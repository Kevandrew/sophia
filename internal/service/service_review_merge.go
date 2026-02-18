package service

import (
	"fmt"
	"path/filepath"
	"sophia/internal/model"
	"strconv"
	"strings"
)

func (s *Service) ReviewCR(id int) (*Review, error) {
	cr, err := s.store.LoadCR(id)
	if err != nil {
		return nil, err
	}
	policy, err := s.repoPolicy()
	if err != nil {
		return nil, err
	}
	diff, err := s.summarizeCRDiff(cr)
	if err != nil {
		return nil, err
	}
	validation, err := s.ValidateCR(id)
	if err != nil {
		return nil, err
	}
	trust := buildTrustReport(cr, validation, diff, policy.Contract.RequiredFields)

	return &Review{
		CR:                 cr,
		Contract:           cr.Contract,
		Impact:             validation.Impact,
		Trust:              trust,
		ValidationErrors:   append([]string(nil), validation.Errors...),
		ValidationWarnings: append([]string(nil), validation.Warnings...),
		Files:              diff.Files,
		ShortStat:          diff.ShortStat,
		NewFiles:           diff.NewFiles,
		ModifiedFiles:      diff.ModifiedFiles,
		DeletedFiles:       diff.DeletedFiles,
		TestFiles:          diff.TestFiles,
		DependencyFiles:    diff.DependencyFiles,
	}, nil
}

func (s *Service) MergeCR(id int, keepBranch bool, overrideReason string) (string, error) {
	sha, _, err := s.MergeCRWithWarnings(id, keepBranch, overrideReason)
	return sha, err
}

func (s *Service) MergeCRWithWarnings(id int, keepBranch bool, overrideReason string) (string, []string, error) {
	warnings := []string{}
	cr, err := s.store.LoadCR(id)
	if err != nil {
		return "", warnings, err
	}
	if _, err := ensureCRUID(cr); err != nil {
		return "", warnings, err
	}
	if _, err := s.ensureCRBaseFields(cr, true); err != nil {
		return "", warnings, err
	}
	if guardErr := s.ensureNoMergeInProgressForCR(cr); guardErr != nil {
		return "", warnings, guardErr
	}
	if cr.Status == model.StatusMerged {
		return "", warnings, ErrCRAlreadyMerged
	}
	policy, err := s.repoPolicy()
	if err != nil {
		return "", warnings, err
	}
	validation, err := s.ValidateCR(id)
	if err != nil {
		return "", warnings, err
	}
	overrideReason = strings.TrimSpace(overrideReason)
	if overrideReason != "" && !policyAllowsMergeOverride(policy) {
		return "", warnings, fmt.Errorf("%w: merge override is disabled by repository policy", ErrPolicyViolation)
	}
	if cr.ParentCRID > 0 {
		parent, parentErr := s.store.LoadCR(cr.ParentCRID)
		if parentErr != nil {
			return "", warnings, fmt.Errorf("parent cr %d not found: %w", cr.ParentCRID, parentErr)
		}
		if parent.Status != model.StatusMerged && overrideReason == "" && !childDelegatedFromParent(parent, cr.ID) {
			return "", warnings, fmt.Errorf("%w: CR %d depends on parent CR %d (%s)", ErrParentCRNotMerged, cr.ID, parent.ID, parent.Status)
		}
	}
	if !validation.Valid && overrideReason == "" {
		return "", warnings, fmt.Errorf("%w: %s", ErrCRValidationFailed, strings.Join(validation.Errors, "; "))
	}
	if overrideReason == "" {
		blockers := s.mergeBlockersForCR(cr, validation)
		if len(blockers) > 0 {
			return "", warnings, fmt.Errorf("merge blocked: %s", strings.Join(blockers, "; "))
		}
	}
	if !s.git.BranchExists(cr.BaseBranch) {
		return "", warnings, fmt.Errorf("base branch %q does not exist", cr.BaseBranch)
	}
	if !s.git.BranchExists(cr.Branch) {
		return "", warnings, fmt.Errorf("cr branch %q does not exist", cr.Branch)
	}

	mergeGit, worktreePath, err := s.effectiveMergeGitForCR(cr)
	if err != nil {
		return "", warnings, err
	}
	if dirty, summary, err := s.workingTreeDirtySummaryFor(mergeGit); err != nil {
		return "", warnings, err
	} else if dirty {
		return "", warnings, fmt.Errorf("%w: %s", ErrWorkingTreeDirty, summary)
	}
	currentBranch, err := mergeGit.CurrentBranch()
	if err != nil {
		return "", warnings, err
	}
	if strings.TrimSpace(currentBranch) != strings.TrimSpace(cr.BaseBranch) {
		if err := mergeGit.CheckoutBranch(cr.BaseBranch); err != nil {
			return "", warnings, err
		}
	}

	actor := mergeGit.Actor()
	mergedAt := s.timestamp()
	msg := buildMergeCommitMessage(cr, actor, mergedAt)
	if err := mergeGit.MergeNoFFOnCurrentBranch(cr.Branch, msg); err != nil {
		conflictFiles, conflictErr := mergeGit.MergeConflictFiles()
		if conflictErr != nil {
			return "", warnings, err
		}
		inProgress, progressErr := mergeGit.IsMergeInProgress()
		if progressErr != nil {
			return "", warnings, err
		}
		if inProgress {
			if persistErr := s.recordMergeConflictEvent(cr, actor, worktreePath, conflictFiles, err); persistErr != nil {
				return "", warnings, persistErr
			}
			return "", warnings, &MergeConflictError{
				CRID:          cr.ID,
				BaseBranch:    cr.BaseBranch,
				CRBranch:      cr.Branch,
				WorktreePath:  worktreePath,
				ConflictFiles: conflictFiles,
				Cause:         err,
			}
		}
		return "", warnings, err
	}

	if !keepBranch {
		crOwner, ownerErr := s.branchOwnerWorktree(cr.Branch)
		if ownerErr != nil {
			return "", warnings, ownerErr
		}
		if crOwner != nil {
			warnings = append(warnings, fmt.Sprintf("Kept branch %s because it is checked out in worktree %s", cr.Branch, crOwner.Path))
		} else if err := s.git.DeleteBranch(cr.Branch, true); err != nil {
			return "", warnings, err
		}
	}

	sha, err := mergeGit.HeadShortSHA()
	if err != nil {
		return "", warnings, err
	}

	if err := s.finalizeCRMergedState(cr, validation, overrideReason, actor, mergedAt, sha, false); err != nil {
		return "", warnings, err
	}

	return sha, warnings, nil
}

func (s *Service) MergeStatusCR(id int) (*MergeStatusView, error) {
	cr, err := s.store.LoadCR(id)
	if err != nil {
		return nil, err
	}
	if _, err := s.ensureCRBaseFields(cr, true); err != nil {
		return nil, err
	}
	mergeGit, worktreePath, err := s.effectiveMergeGitForCR(cr)
	if err != nil {
		return nil, err
	}
	inProgress, err := mergeGit.IsMergeInProgress()
	if err != nil {
		return nil, err
	}
	mergeHead, err := mergeGit.MergeHeadSHA()
	if err != nil {
		return nil, err
	}
	conflictFiles, err := mergeGit.MergeConflictFiles()
	if err != nil {
		return nil, err
	}

	targetMatches := false
	if inProgress && strings.TrimSpace(mergeHead) != "" {
		targetHead, resolveErr := mergeGit.ResolveRef(cr.Branch)
		if resolveErr == nil && strings.TrimSpace(targetHead) != "" {
			targetMatches = strings.TrimSpace(targetHead) == strings.TrimSpace(mergeHead)
		}
	}
	advice := []string{}
	if inProgress {
		advice = append(advice, fmt.Sprintf("Resolve conflicted files and run sophia cr merge resume %d.", cr.ID))
		advice = append(advice, fmt.Sprintf("Run sophia cr merge abort %d to abandon the in-progress merge.", cr.ID))
		if !targetMatches {
			advice = append(advice, "Current in-progress merge does not match this CR target branch.")
		}
	} else {
		advice = append(advice, "No merge in progress for this CR.")
	}

	return &MergeStatusView{
		CRID:          cr.ID,
		CRUID:         strings.TrimSpace(cr.UID),
		BaseBranch:    cr.BaseBranch,
		CRBranch:      cr.Branch,
		WorktreePath:  worktreePath,
		InProgress:    inProgress,
		ConflictFiles: conflictFiles,
		TargetMatches: targetMatches,
		MergeHead:     strings.TrimSpace(mergeHead),
		Advice:        advice,
	}, nil
}

func (s *Service) AbortMergeCR(id int) error {
	status, err := s.MergeStatusCR(id)
	if err != nil {
		return err
	}
	if !status.InProgress {
		return &NoMergeInProgressError{
			WorktreePath: status.WorktreePath,
			Summary:      fmt.Sprintf("%s: no merge in progress for CR %d", ErrNoMergeInProgress, id),
		}
	}
	if !status.TargetMatches {
		return &MergeInProgressError{
			WorktreePath:  status.WorktreePath,
			ConflictFiles: status.ConflictFiles,
			Summary:       fmt.Sprintf("%s: in-progress merge in %s does not target CR %d", ErrMergeInProgress, status.WorktreePath, id),
		}
	}
	cr, err := s.store.LoadCR(id)
	if err != nil {
		return err
	}
	mergeGit, _, err := s.effectiveMergeGitForCR(cr)
	if err != nil {
		return err
	}
	if err := mergeGit.MergeAbort(); err != nil {
		return err
	}
	now := s.timestamp()
	actor := s.git.Actor()
	cr.UpdatedAt = now
	cr.Events = append(cr.Events, model.Event{
		TS:      now,
		Actor:   actor,
		Type:    "cr_merge_aborted",
		Summary: fmt.Sprintf("Aborted in-progress merge for CR %d", cr.ID),
		Ref:     fmt.Sprintf("cr:%d", cr.ID),
		Meta: map[string]string{
			"worktree_path":  status.WorktreePath,
			"conflict_count": strconv.Itoa(len(status.ConflictFiles)),
		},
	})
	return s.store.SaveCR(cr)
}

func (s *Service) ResumeMergeCR(id int, keepBranch bool, overrideReason string) (string, []string, error) {
	status, err := s.MergeStatusCR(id)
	if err != nil {
		return "", nil, err
	}
	if !status.InProgress {
		return "", nil, &NoMergeInProgressError{
			WorktreePath: status.WorktreePath,
			Summary:      fmt.Sprintf("%s: no merge in progress for CR %d", ErrNoMergeInProgress, id),
		}
	}
	if !status.TargetMatches {
		return "", nil, &MergeInProgressError{
			WorktreePath:  status.WorktreePath,
			ConflictFiles: status.ConflictFiles,
			Summary:       fmt.Sprintf("%s: in-progress merge in %s does not target CR %d", ErrMergeInProgress, status.WorktreePath, id),
		}
	}

	cr, err := s.store.LoadCR(id)
	if err != nil {
		return "", nil, err
	}
	if cr.Status == model.StatusMerged {
		return "", nil, ErrCRAlreadyMerged
	}
	policy, err := s.repoPolicy()
	if err != nil {
		return "", nil, err
	}
	validation, err := s.ValidateCR(id)
	if err != nil {
		return "", nil, err
	}
	overrideReason = strings.TrimSpace(overrideReason)
	if overrideReason != "" && !policyAllowsMergeOverride(policy) {
		return "", nil, fmt.Errorf("%w: merge override is disabled by repository policy", ErrPolicyViolation)
	}

	mergeGit, _, err := s.effectiveMergeGitForCR(cr)
	if err != nil {
		return "", nil, err
	}
	if err := mergeGit.MergeContinue(); err != nil {
		return "", nil, err
	}
	warnings := []string{}
	if !keepBranch {
		crOwner, ownerErr := s.branchOwnerWorktree(cr.Branch)
		if ownerErr != nil {
			return "", warnings, ownerErr
		}
		if crOwner != nil {
			warnings = append(warnings, fmt.Sprintf("Kept branch %s because it is checked out in worktree %s", cr.Branch, crOwner.Path))
		} else if err := s.git.DeleteBranch(cr.Branch, true); err != nil {
			return "", warnings, err
		}
	}
	sha, err := mergeGit.HeadShortSHA()
	if err != nil {
		return "", warnings, err
	}
	mergedAt := s.timestamp()
	actor := mergeGit.Actor()
	if err := s.finalizeCRMergedState(cr, validation, overrideReason, actor, mergedAt, sha, true); err != nil {
		return "", warnings, err
	}
	return sha, warnings, nil
}

func (s *Service) finalizeCRMergedState(cr *model.CR, validation *ValidationReport, overrideReason, actor, mergedAt, sha string, resumed bool) error {
	if count, err := s.git.ChangedFileCount(sha); err == nil && count > 0 {
		cr.FilesTouchedCount = count
	} else if validation != nil && validation.Impact != nil && validation.Impact.FilesChanged > 0 {
		cr.FilesTouchedCount = validation.Impact.FilesChanged
	} else {
		files, diffErr := s.diffNamesForCR(cr)
		if diffErr == nil {
			cr.FilesTouchedCount = len(files)
		} else if err != nil {
			return err
		}
	}
	cr.Status = model.StatusMerged
	cr.UpdatedAt = mergedAt
	cr.MergedAt = mergedAt
	cr.MergedBy = actor
	cr.MergedCommit = sha
	if resumed {
		cr.Events = append(cr.Events, model.Event{
			TS:      mergedAt,
			Actor:   actor,
			Type:    "cr_merge_resumed",
			Summary: fmt.Sprintf("Resumed in-progress merge for CR %d", cr.ID),
			Ref:     fmt.Sprintf("cr:%d", cr.ID),
		})
	}
	if strings.TrimSpace(overrideReason) != "" {
		riskTier := "-"
		validationErrors := "0"
		if validation != nil {
			validationErrors = strconv.Itoa(len(validation.Errors))
			if validation.Impact != nil {
				riskTier = nonEmptyTrimmed(validation.Impact.RiskTier, "-")
			}
		}
		cr.Events = append(cr.Events, model.Event{
			TS:      mergedAt,
			Actor:   actor,
			Type:    "cr_merge_overridden",
			Summary: fmt.Sprintf("Merged with validation override: %s", overrideReason),
			Ref:     fmt.Sprintf("cr:%d", cr.ID),
			Meta: map[string]string{
				"override_reason":   overrideReason,
				"risk_tier":         riskTier,
				"validation_errors": validationErrors,
			},
		})
	}
	cr.Events = append(cr.Events, model.Event{
		TS:      mergedAt,
		Actor:   actor,
		Type:    "cr_merged",
		Summary: fmt.Sprintf("Merged CR %d", cr.ID),
		Ref:     fmt.Sprintf("cr:%d", cr.ID),
	})
	if err := s.store.SaveCR(cr); err != nil {
		return err
	}
	if err := s.syncCRRef(cr); err != nil {
		return err
	}
	if err := s.backfillChildrenAfterParentMerge(cr); err != nil {
		return err
	}
	if err := s.syncDelegatedTasksAfterChildMerge(cr.ID); err != nil {
		return err
	}
	return nil
}

func (s *Service) recordMergeConflictEvent(cr *model.CR, actor, worktreePath string, conflictFiles []string, cause error) error {
	now := s.timestamp()
	meta := map[string]string{
		"worktree_path":  strings.TrimSpace(worktreePath),
		"base_branch":    strings.TrimSpace(cr.BaseBranch),
		"cr_branch":      strings.TrimSpace(cr.Branch),
		"conflict_count": strconv.Itoa(len(conflictFiles)),
	}
	if len(conflictFiles) > 0 {
		limit := conflictFiles
		if len(limit) > 20 {
			limit = limit[:20]
		}
		meta["conflict_files"] = strings.Join(limit, ",")
	}
	if cause != nil {
		meta["cause"] = strings.TrimSpace(cause.Error())
	}
	cr.Events = append(cr.Events, model.Event{
		TS:      now,
		Actor:   actor,
		Type:    "cr_merge_conflict",
		Summary: fmt.Sprintf("Merge conflict while merging CR %d", cr.ID),
		Ref:     fmt.Sprintf("cr:%d", cr.ID),
		Meta:    meta,
	})
	cr.UpdatedAt = now
	return s.store.SaveCR(cr)
}

func (s *Service) Doctor(limit int) (*DoctorReport, error) {
	if err := s.store.EnsureInitialized(); err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 100
	}
	cfg, err := s.store.LoadConfig()
	if err != nil {
		return nil, err
	}

	report := &DoctorReport{BaseBranch: cfg.BaseBranch, Findings: []DoctorFinding{}}
	branch, err := s.git.CurrentBranch()
	if err == nil {
		report.CurrentBranch = branch
		if _, ctxErr := s.resolveCRFromBranch(branch); ctxErr != nil {
			report.Findings = append(report.Findings, DoctorFinding{
				Code:    "non_cr_branch",
				Message: fmt.Sprintf("current branch %q is not a CR branch", branch),
			})
		}
	}

	statusEntries, err := s.git.WorkingTreeStatus()
	if err != nil {
		return nil, err
	}
	for _, entry := range statusEntries {
		if entry.Code == "??" {
			report.UntrackedCount++
		} else {
			report.ChangedCount++
		}
	}
	if report.UntrackedCount > 0 || report.ChangedCount > 0 {
		report.Findings = append(report.Findings, DoctorFinding{
			Code:    "dirty_worktree",
			Message: fmt.Sprintf("working tree has %d modified/staged and %d untracked paths", report.ChangedCount, report.UntrackedCount),
		})
	}

	if cfg.MetadataMode == model.MetadataModeLocal {
		trackedSophia, trackedErr := s.git.TrackedFiles(".sophia")
		if trackedErr == nil && len(trackedSophia) > 0 {
			report.Findings = append(report.Findings, DoctorFinding{
				Code:    "tracked_sophia_metadata",
				Message: fmt.Sprintf("%d tracked path(s) found under .sophia in local metadata mode", len(trackedSophia)),
			})
		}
		if strings.TrimSpace(s.sharedLocalSophiaDir) != "" &&
			filepath.Clean(s.store.SophiaDir()) != filepath.Clean(s.legacySophiaDir) &&
			pathExists(s.legacySophiaDir) {
			report.Findings = append(report.Findings, DoctorFinding{
				Code:    "legacy_local_metadata",
				Message: fmt.Sprintf("legacy local metadata path still exists at %s", s.legacySophiaDir),
			})
		}
	}

	crs, err := s.store.ListCRs()
	if err != nil {
		return nil, err
	}
	stale := make([]string, 0)
	for _, cr := range crs {
		if cr.Status == model.StatusMerged && s.git.BranchExists(cr.Branch) {
			stale = append(stale, cr.Branch)
		}
	}
	if len(stale) > 0 {
		preview := stale
		if len(preview) > 5 {
			preview = preview[:5]
		}
		report.Findings = append(report.Findings, DoctorFinding{
			Code:    "stale_merged_branches",
			Message: fmt.Sprintf("%d merged CR branch(es) still present (latest: %s)", len(stale), strings.Join(preview, ", ")),
		})
	}

	commits, err := s.git.RecentCommits(cfg.BaseBranch, limit)
	if err != nil {
		return nil, err
	}
	report.ScannedCommits = len(commits)
	untied := make([]string, 0)
	for _, commit := range commits {
		if strings.HasPrefix(commit.Subject, "chore: bootstrap base branch for Sophia") {
			continue
		}
		if legacyPersistPattern.MatchString(strings.TrimSpace(commit.Subject)) {
			continue
		}
		if commitTiedToCR(commit.Subject, commit.Body) {
			continue
		}
		untied = append(untied, fmt.Sprintf("%s %s", shortHash(commit.Hash), commit.Subject))
	}
	if len(untied) > 0 {
		preview := untied
		if len(preview) > 5 {
			preview = preview[:5]
		}
		report.Findings = append(report.Findings, DoctorFinding{
			Code:    "untied_base_commits",
			Message: fmt.Sprintf("%d base-branch commit(s) not tied to a CR (latest: %s)", len(untied), strings.Join(preview, "; ")),
		})
	}

	return report, nil
}

func (s *Service) CurrentCR() (*CurrentCRContext, error) {
	branch, err := s.git.CurrentBranch()
	if err != nil {
		return nil, err
	}
	cr, err := s.resolveCRFromBranch(branch)
	if err != nil {
		return nil, err
	}
	if _, err := s.ensureCRBaseFields(cr, true); err != nil {
		return nil, err
	}
	return &CurrentCRContext{Branch: branch, CR: cr}, nil
}

func (s *Service) resolveCRFromBranch(branch string) (*model.CR, error) {
	trimmedBranch := strings.TrimSpace(branch)
	if trimmedBranch == "" {
		return nil, ErrNoActiveCRContext
	}

	crs, err := s.store.ListCRs()
	if err == nil {
		for _, candidate := range crs {
			if candidate.Status != model.StatusInProgress {
				continue
			}
			if strings.TrimSpace(candidate.Branch) != trimmedBranch {
				continue
			}
			crCopy := candidate
			return &crCopy, nil
		}
	}

	id, ok := parseCRBranchID(trimmedBranch)
	if !ok {
		return nil, ErrNoActiveCRContext
	}
	cr, err := s.store.LoadCR(id)
	if err != nil {
		return nil, ErrNoActiveCRContext
	}
	if cr.Status != model.StatusInProgress {
		return nil, ErrNoActiveCRContext
	}
	return cr, nil
}

func (s *Service) SwitchCR(id int) (*model.CR, error) {
	if dirty, summary, err := s.workingTreeDirtySummary(); err != nil {
		return nil, err
	} else if dirty {
		return nil, fmt.Errorf("%w: %s", ErrWorkingTreeDirty, summary)
	}

	cr, err := s.store.LoadCR(id)
	if err != nil {
		return nil, err
	}
	if _, err := s.ensureCRBaseFields(cr, true); err != nil {
		return nil, err
	}
	if s.git.BranchExists(cr.Branch) {
		owner, ownerErr := s.branchOwnerWorktree(cr.Branch)
		if ownerErr != nil {
			return nil, ownerErr
		}
		if owner != nil && !s.isCurrentWorktreePath(owner.Path) {
			return nil, fmt.Errorf("%w: branch %q is checked out in worktree %q", ErrBranchInOtherWorktree, cr.Branch, owner.Path)
		}
		if err := s.git.CheckoutBranch(cr.Branch); err != nil {
			return nil, err
		}
		if err := s.syncCRRef(cr); err != nil {
			return nil, err
		}
		return cr, nil
	}

	if cr.Status != model.StatusInProgress {
		return nil, fmt.Errorf("branch %q is missing for merged CR %d; run sophia cr reopen %d", cr.Branch, cr.ID, cr.ID)
	}
	baseAnchor, err := s.resolveCRBaseAnchor(cr)
	if err != nil {
		return nil, err
	}
	if err := s.git.CreateBranchFrom(cr.Branch, baseAnchor); err != nil {
		return nil, err
	}
	if err := s.syncCRRef(cr); err != nil {
		return nil, err
	}
	return cr, nil
}

func (s *Service) ReopenCR(id int) (*model.CR, error) {
	if dirty, summary, err := s.workingTreeDirtySummary(); err != nil {
		return nil, err
	} else if dirty {
		return nil, fmt.Errorf("%w: %s", ErrWorkingTreeDirty, summary)
	}

	cr, err := s.store.LoadCR(id)
	if err != nil {
		return nil, err
	}
	if _, err := s.ensureCRBaseFields(cr, true); err != nil {
		return nil, err
	}
	if cr.Status != model.StatusMerged {
		return nil, fmt.Errorf("cr %d is not merged", id)
	}
	if s.git.BranchExists(cr.Branch) {
		owner, ownerErr := s.branchOwnerWorktree(cr.Branch)
		if ownerErr != nil {
			return nil, ownerErr
		}
		if owner != nil && !s.isCurrentWorktreePath(owner.Path) {
			return nil, fmt.Errorf("%w: branch %q is checked out in worktree %q", ErrBranchInOtherWorktree, cr.Branch, owner.Path)
		}
		if err := s.git.CheckoutBranch(cr.Branch); err != nil {
			return nil, err
		}
	} else {
		baseAnchor, err := s.resolveCRBaseAnchor(cr)
		if err != nil {
			return nil, err
		}
		if err := s.git.CreateBranchFrom(cr.Branch, baseAnchor); err != nil {
			return nil, err
		}
	}

	now := s.timestamp()
	actor := s.git.Actor()
	cr.Status = model.StatusInProgress
	cr.UpdatedAt = now
	cr.Events = append(cr.Events, model.Event{
		TS:      now,
		Actor:   actor,
		Type:    "cr_reopened",
		Summary: fmt.Sprintf("Reopened CR %d", cr.ID),
		Ref:     fmt.Sprintf("cr:%d", cr.ID),
	})
	if err := s.store.SaveCR(cr); err != nil {
		return nil, err
	}
	if err := s.syncCRRef(cr); err != nil {
		return nil, err
	}

	return cr, nil
}
