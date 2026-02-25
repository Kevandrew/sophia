package service

import (
	"errors"
	"fmt"
	"path/filepath"
	"sophia/internal/gitx"
	"sophia/internal/model"
	servicemerge "sophia/internal/service/merge"
	"strconv"
	"strings"
)

type mergePreflightView struct {
	validation     *ValidationReport
	trust          *TrustReport
	overrideReason string
}

func (s *Service) ReviewCR(id int) (*Review, error) {
	cr, err := s.store.LoadCR(id)
	if err != nil {
		return nil, err
	}
	policy, err := s.repoPolicy()
	if err != nil {
		return nil, err
	}
	validation, err := s.ValidateCR(id)
	if err != nil {
		return nil, err
	}
	diff, err := s.summarizeCRDiffWithPolicy(cr, policy)
	if err != nil {
		if errors.Is(err, ErrCRBranchContextUnavailable) {
			diff = &diffSummary{}
		} else {
			return nil, err
		}
	}
	trust := buildTrustReportWithPolicy(cr, validation, diff, policy.Contract.RequiredFields, policy)

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
	var (
		sha      string
		warnings []string
	)
	if err := s.withMutationLock(func() error {
		var mergeErr error
		sha, warnings, mergeErr = s.mergeCRWithWarningsUnlocked(id, keepBranch, overrideReason)
		return mergeErr
	}); err != nil {
		return "", nil, err
	}
	return sha, warnings, nil
}

func (s *Service) mergeCRWithWarningsUnlocked(id int, keepBranch bool, overrideReason string) (string, []string, error) {
	warnings := []string{}
	cr, err := s.store.LoadCR(id)
	if err != nil {
		return "", warnings, err
	}
	if _, err := ensureCRUID(cr); err != nil {
		return "", warnings, err
	}
	if _, err := s.ensureCRBaseFields(cr, false); err != nil {
		return "", warnings, err
	}
	if guardErr := s.ensureNoMergeInProgressForCR(cr); guardErr != nil {
		return "", warnings, guardErr
	}
	if cr.Status == model.StatusMerged {
		return "", warnings, ErrCRAlreadyMerged
	}
	preflight, err := s.prepareMergePreflight(id, cr, overrideReason, true)
	if err != nil {
		return "", warnings, err
	}
	if !s.git.BranchExists(cr.BaseBranch) {
		return "", warnings, fmt.Errorf("base branch %q does not exist", cr.BaseBranch)
	}
	if !s.git.BranchExists(cr.Branch) {
		return "", warnings, fmt.Errorf("cr branch %q does not exist", cr.Branch)
	}
	archiveConfig, archiveEnabled, err := s.resolveMergeArchiveConfig()
	if err != nil {
		return "", warnings, err
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
	baseParent, err := mergeGit.ResolveRef("HEAD")
	if err != nil {
		return "", warnings, err
	}
	if err := mergeGit.MergeNoFFNoCommitOnCurrentBranch(cr.Branch, msg); err != nil {
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
	inProgress, err := mergeGit.IsMergeInProgress()
	if err != nil {
		return "", warnings, err
	}
	if inProgress {
		if archiveEnabled {
			mergeHead, mergeHeadErr := mergeGit.MergeHeadSHA()
			if mergeHeadErr != nil {
				return "", warnings, mergeHeadErr
			}
			if strings.TrimSpace(mergeHead) == "" {
				return "", warnings, fmt.Errorf("unable to determine merge head for CR %d archive generation", cr.ID)
			}
			if err := s.writeAutomaticCRArchiveForMerge(mergeGit, cr, archiveConfig, actor, mergedAt, baseParent, mergeHead, false); err != nil {
				return "", warnings, err
			}
		}
		if err := mergeGit.Commit(msg); err != nil {
			return "", warnings, err
		}
	} else if archiveEnabled {
		warnings = append(warnings, fmt.Sprintf("Skipped archive write for CR %d because merge produced no new commit", cr.ID))
	}

	branchWarnings, err := s.maybeDeleteMergedCRBranch(cr.Branch, keepBranch)
	if err != nil {
		return "", warnings, err
	}
	warnings = append(warnings, branchWarnings...)

	sha, err := mergeGit.HeadShortSHA()
	if err != nil {
		return "", warnings, err
	}

	if err := s.finalizeCRMergedState(cr, preflight.validation, preflight.trust, preflight.overrideReason, actor, mergedAt, sha, false); err != nil {
		return "", warnings, err
	}

	return sha, warnings, nil
}

func (s *Service) MergeStatusCR(id int) (*MergeStatusView, error) {
	cr, err := s.store.LoadCR(id)
	if err != nil {
		return nil, err
	}
	if _, err := s.ensureCRBaseFields(cr, false); err != nil {
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
	advice := servicemerge.BuildStatusAdvice(inProgress, targetMatches, cr.ID)

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
	return s.withMutationLock(func() error {
		return s.abortMergeCRUnlocked(id)
	})
}

func (s *Service) abortMergeCRUnlocked(id int) error {
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
	return s.appendCRMutationEventAndSave(cr, model.Event{
		TS:      now,
		Actor:   actor,
		Type:    model.EventTypeCRMergeAborted,
		Summary: fmt.Sprintf("Aborted in-progress merge for CR %d", cr.ID),
		Ref:     fmt.Sprintf("cr:%d", cr.ID),
		Meta: map[string]string{
			"worktree_path":  status.WorktreePath,
			"conflict_count": strconv.Itoa(len(status.ConflictFiles)),
		},
	})
}

func (s *Service) ResumeMergeCR(id int, keepBranch bool, overrideReason string) (string, []string, error) {
	var (
		sha      string
		warnings []string
	)
	if err := s.withMutationLock(func() error {
		var resumeErr error
		sha, warnings, resumeErr = s.resumeMergeCRUnlocked(id, keepBranch, overrideReason)
		return resumeErr
	}); err != nil {
		return "", nil, err
	}
	return sha, warnings, nil
}

func (s *Service) resumeMergeCRUnlocked(id int, keepBranch bool, overrideReason string) (string, []string, error) {
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
	if len(status.ConflictFiles) > 0 {
		return "", nil, &MergeInProgressError{
			WorktreePath:  status.WorktreePath,
			ConflictFiles: status.ConflictFiles,
			Summary:       fmt.Sprintf("%s: unresolved conflicts remain for CR %d", ErrMergeInProgress, id),
		}
	}

	cr, err := s.store.LoadCR(id)
	if err != nil {
		return "", nil, err
	}
	if cr.Status == model.StatusMerged {
		return "", nil, ErrCRAlreadyMerged
	}
	preflight, err := s.prepareMergePreflight(id, cr, overrideReason, false)
	if err != nil {
		return "", nil, err
	}
	archiveConfig, archiveEnabled, err := s.resolveMergeArchiveConfig()
	if err != nil {
		return "", nil, err
	}

	mergeGit, _, err := s.effectiveMergeGitForCR(cr)
	if err != nil {
		return "", nil, err
	}
	actor := mergeGit.Actor()
	mergedAt := s.timestamp()
	if archiveEnabled {
		baseParent, baseParentErr := mergeGit.ResolveRef("HEAD")
		if baseParentErr != nil {
			return "", nil, baseParentErr
		}
		mergeHead, mergeHeadErr := mergeGit.MergeHeadSHA()
		if mergeHeadErr != nil {
			return "", nil, mergeHeadErr
		}
		if strings.TrimSpace(mergeHead) == "" {
			return "", nil, fmt.Errorf("unable to determine merge head for CR %d archive generation", cr.ID)
		}
		if err := s.writeAutomaticCRArchiveForMerge(mergeGit, cr, archiveConfig, actor, mergedAt, baseParent, mergeHead, true); err != nil {
			return "", nil, err
		}
	}
	if err := mergeGit.MergeContinue(); err != nil {
		return "", nil, err
	}
	warnings := []string{}
	branchWarnings, err := s.maybeDeleteMergedCRBranch(cr.Branch, keepBranch)
	if err != nil {
		return "", warnings, err
	}
	warnings = append(warnings, branchWarnings...)
	sha, err := mergeGit.HeadShortSHA()
	if err != nil {
		return "", warnings, err
	}
	if err := s.finalizeCRMergedState(cr, preflight.validation, preflight.trust, preflight.overrideReason, actor, mergedAt, sha, true); err != nil {
		return "", warnings, err
	}
	return sha, warnings, nil
}

func (s *Service) resolveMergeArchiveConfig() (model.PolicyArchive, bool, error) {
	archiveConfig, err := s.archivePolicyConfig()
	if err != nil {
		return model.PolicyArchive{}, false, err
	}
	enabled := archivePolicyEnabled(archiveConfig)
	if enabled {
		if err := s.requireArchiveConfigSupported(archiveConfig); err != nil {
			return model.PolicyArchive{}, false, err
		}
	}
	return archiveConfig, enabled, nil
}

func (s *Service) maybeDeleteMergedCRBranch(branch string, keepBranch bool) ([]string, error) {
	if keepBranch {
		return nil, nil
	}
	crOwner, ownerErr := s.branchOwnerWorktree(branch)
	if ownerErr != nil {
		return nil, ownerErr
	}
	if crOwner != nil {
		return []string{fmt.Sprintf("Kept branch %s because it is checked out in worktree %s", branch, crOwner.Path)}, nil
	}
	if err := s.git.DeleteBranch(branch, true); err != nil {
		return nil, err
	}
	return nil, nil
}

func (s *Service) writeAutomaticCRArchiveForMerge(mergeGit *gitx.Client, cr *model.CR, archiveConfig model.PolicyArchive, actor, mergedAt, baseParent, crParent string, reuseExisting bool) error {
	if mergeGit == nil {
		return fmt.Errorf("merge git client is required for archive generation")
	}
	if cr == nil {
		return fmt.Errorf("cr is required for archive generation")
	}
	workDir := strings.TrimSpace(mergeGit.WorkDir)
	if workDir == "" {
		return fmt.Errorf("merge worktree path is required for archive generation")
	}
	archivePath := archiveRevisionPath(filepath.Join(workDir, archiveConfig.Path), cr.ID, 1)
	if reuseExisting {
		exists, err := archiveFileExists(archivePath)
		if err != nil {
			return err
		}
		if exists {
			relativeArchivePath, relErr := relativeToRootPath(workDir, archivePath)
			if relErr != nil {
				return relErr
			}
			return mergeGit.StagePaths([]string{relativeArchivePath})
		}
	}
	gitSummary, summaryErr := buildArchiveGitSummaryFromCachedDiff(mergeGit, baseParent, crParent)
	if summaryErr != nil {
		return summaryErr
	}
	archiveCR := *cr
	archiveCR.Status = model.StatusMerged
	archiveCR.MergedAt = mergedAt
	archiveCR.MergedBy = actor
	archive := buildCRArchiveDocument(&archiveCR, 1, "", mergedAt, gitSummary)
	payload, payloadErr := marshalCRArchiveYAML(archive)
	if payloadErr != nil {
		return payloadErr
	}
	if err := writeArchivePayload(archivePath, payload); err != nil {
		return err
	}
	relativeArchivePath, relErr := relativeToRootPath(workDir, archivePath)
	if relErr != nil {
		return relErr
	}
	return mergeGit.StagePaths([]string{relativeArchivePath})
}

func (s *Service) prepareMergePreflight(id int, cr *model.CR, overrideReason string, enforceParentState bool) (*mergePreflightView, error) {
	policy, err := s.repoPolicy()
	if err != nil {
		return nil, err
	}
	validation, err := s.ValidateCR(id)
	if err != nil {
		return nil, err
	}
	diff, err := s.summarizeCRDiffWithPolicy(cr, policy)
	if err != nil {
		return nil, err
	}
	trust := buildTrustReportWithPolicy(cr, validation, diff, policy.Contract.RequiredFields, policy)
	trimmedOverride := strings.TrimSpace(overrideReason)
	if trimmedOverride != "" && !policyAllowsMergeOverride(policy) {
		return nil, fmt.Errorf("%w: merge override is disabled by repository policy", ErrPolicyViolation)
	}
	if enforceParentState && cr.ParentCRID > 0 {
		parent, parentErr := s.store.LoadCR(cr.ParentCRID)
		if parentErr != nil {
			return nil, fmt.Errorf("parent cr %d not found: %w", cr.ParentCRID, parentErr)
		}
		if parent.Status != model.StatusMerged && trimmedOverride == "" && !childDelegatedFromParent(parent, cr.ID) {
			return nil, fmt.Errorf("%w: CR %d depends on parent CR %d (%s)", ErrParentCRNotMerged, cr.ID, parent.ID, parent.Status)
		}
	}
	if !validation.Valid && trimmedOverride == "" {
		return nil, fmt.Errorf("%w: %s", ErrCRValidationFailed, strings.Join(validation.Errors, "; "))
	}
	if trimmedOverride == "" && trust.Gate.Blocked {
		return nil, fmt.Errorf("merge blocked: %s", strings.TrimSpace(trust.Gate.Reason))
	}
	if trimmedOverride == "" {
		blockers := s.mergeBlockersForCR(cr, validation)
		if len(blockers) > 0 {
			return nil, fmt.Errorf("merge blocked: %s", strings.Join(blockers, "; "))
		}
	}
	return &mergePreflightView{
		validation:     validation,
		trust:          trust,
		overrideReason: trimmedOverride,
	}, nil
}

func (s *Service) finalizeCRMergedState(cr *model.CR, validation *ValidationReport, trust *TrustReport, overrideReason, actor, mergedAt, sha string, resumed bool) error {
	if count, err := s.git.ChangedFileCount(sha); err == nil && count > 0 {
		cr.FilesTouchedCount = count
	} else if validation != nil && validation.Impact != nil && validation.Impact.FilesChanged > 0 {
		cr.FilesTouchedCount = validation.Impact.FilesChanged
	} else {
		files, diffErr := s.diffNamesForCR(cr)
		if diffErr == nil {
			cr.FilesTouchedCount = len(files)
		} else {
			return diffErr
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
			Type:    model.EventTypeCRMergeResumed,
			Summary: fmt.Sprintf("Resumed in-progress merge for CR %d", cr.ID),
			Ref:     fmt.Sprintf("cr:%d", cr.ID),
		})
	}
	if strings.TrimSpace(overrideReason) != "" {
		evidence := servicemerge.OverrideEvidence{}
		if validation != nil {
			evidence.ValidationErrorCount = len(validation.Errors)
			if validation.Impact != nil {
				evidence.RiskTier = validation.Impact.RiskTier
			}
		}
		if trust != nil {
			evidence.TrustVerdict = trust.Verdict
			evidence.TrustGateBlocked = trust.Gate.Blocked
		}
		cr.Events = append(cr.Events, model.Event{
			TS:      mergedAt,
			Actor:   actor,
			Type:    model.EventTypeCRMergeOverridden,
			Summary: fmt.Sprintf("Merged with validation override: %s", overrideReason),
			Ref:     fmt.Sprintf("cr:%d", cr.ID),
			Meta:    servicemerge.BuildOverrideEventMeta(overrideReason, evidence),
		})
	}
	cr.Events = append(cr.Events, model.Event{
		TS:      mergedAt,
		Actor:   actor,
		Type:    model.EventTypeCRMerged,
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
	meta := servicemerge.BuildConflictEventMeta(worktreePath, cr.BaseBranch, cr.Branch, conflictFiles, cause)
	return s.appendCRMutationEventAndSave(cr, model.Event{
		TS:      now,
		Actor:   actor,
		Type:    model.EventTypeCRMergeConflict,
		Summary: fmt.Sprintf("Merge conflict while merging CR %d", cr.ID),
		Ref:     fmt.Sprintf("cr:%d", cr.ID),
		Meta:    meta,
	})
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
	if _, policyWarnings, policyErr := s.repoPolicyWithWarnings(); policyErr == nil && len(policyWarnings) > 0 {
		report.Findings = append(report.Findings, DoctorFinding{
			Code:    "policy_unknown_fields",
			Message: strings.Join(policyWarnings, "; "),
		})
	}
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
	cr, err = s.ensureCRBaseFieldsPersisted(cr)
	if err != nil {
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
	var cr *model.CR
	if err := s.withMutationLock(func() error {
		var err error
		cr, err = s.switchCRUnlocked(id)
		return err
	}); err != nil {
		return nil, err
	}
	return cr, nil
}

func (s *Service) switchCRUnlocked(id int) (*model.CR, error) {
	if dirty, summary, err := s.workingTreeDirtySummary(); err != nil {
		return nil, err
	} else if dirty {
		return nil, fmt.Errorf("%w: %s", ErrWorkingTreeDirty, summary)
	}

	cr, err := s.store.LoadCR(id)
	if err != nil {
		return nil, err
	}
	if _, err := s.ensureCRBaseFields(cr, false); err != nil {
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
		return nil, fmt.Errorf("create CR branch %q from %q: %w", cr.Branch, strings.TrimSpace(baseAnchor), err)
	}
	resolvedBaseAnchor := strings.TrimSpace(baseAnchor)
	if resolvedBaseAnchor != "" && strings.TrimSpace(cr.BaseCommit) != resolvedBaseAnchor {
		cr.BaseCommit = resolvedBaseAnchor
		cr.UpdatedAt = s.timestamp()
		if err := s.store.SaveCR(cr); err != nil {
			return nil, err
		}
	}
	if err := s.syncCRRef(cr); err != nil {
		return nil, err
	}
	return cr, nil
}

func (s *Service) ReopenCR(id int) (*model.CR, error) {
	var cr *model.CR
	if err := s.withMutationLock(func() error {
		var err error
		cr, err = s.reopenCRUnlocked(id)
		return err
	}); err != nil {
		return nil, err
	}
	return cr, nil
}

func (s *Service) reopenCRUnlocked(id int) (*model.CR, error) {
	if dirty, summary, err := s.workingTreeDirtySummary(); err != nil {
		return nil, err
	} else if dirty {
		return nil, fmt.Errorf("%w: %s", ErrWorkingTreeDirty, summary)
	}

	cr, err := s.store.LoadCR(id)
	if err != nil {
		return nil, err
	}
	if _, err := s.ensureCRBaseFields(cr, false); err != nil {
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
		Type:    model.EventTypeCRReopened,
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
