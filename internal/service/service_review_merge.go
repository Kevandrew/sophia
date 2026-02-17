package service

import (
	"fmt"
	"sophia/internal/model"
	"strconv"
	"strings"
)

func (s *Service) ReviewCR(id int) (*Review, error) {
	cr, err := s.store.LoadCR(id)
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
	trust := buildTrustReport(cr, validation, diff)

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
	cr, err := s.store.LoadCR(id)
	if err != nil {
		return "", err
	}
	if _, err := ensureCRUID(cr); err != nil {
		return "", err
	}
	if _, err := s.ensureCRBaseFields(cr, true); err != nil {
		return "", err
	}
	if cr.Status == model.StatusMerged {
		return "", ErrCRAlreadyMerged
	}
	validation, err := s.ValidateCR(id)
	if err != nil {
		return "", err
	}
	overrideReason = strings.TrimSpace(overrideReason)
	if cr.ParentCRID > 0 {
		parent, parentErr := s.store.LoadCR(cr.ParentCRID)
		if parentErr != nil {
			return "", fmt.Errorf("parent cr %d not found: %w", cr.ParentCRID, parentErr)
		}
		if parent.Status != model.StatusMerged && overrideReason == "" && !childDelegatedFromParent(parent, cr.ID) {
			return "", fmt.Errorf("%w: CR %d depends on parent CR %d (%s)", ErrParentCRNotMerged, cr.ID, parent.ID, parent.Status)
		}
	}
	if !validation.Valid && overrideReason == "" {
		return "", fmt.Errorf("%w: %s", ErrCRValidationFailed, strings.Join(validation.Errors, "; "))
	}
	if overrideReason == "" {
		blockers := s.mergeBlockersForCR(cr, validation)
		if len(blockers) > 0 {
			return "", fmt.Errorf("merge blocked: %s", strings.Join(blockers, "; "))
		}
	}
	if dirty, summary, err := s.workingTreeDirtySummary(); err != nil {
		return "", err
	} else if dirty {
		return "", fmt.Errorf("%w: %s", ErrWorkingTreeDirty, summary)
	}
	if !s.git.BranchExists(cr.BaseBranch) {
		return "", fmt.Errorf("base branch %q does not exist", cr.BaseBranch)
	}
	if !s.git.BranchExists(cr.Branch) {
		return "", fmt.Errorf("cr branch %q does not exist", cr.Branch)
	}

	files, err := s.diffNamesForCR(cr)
	if err != nil {
		return "", err
	}

	actor := s.git.Actor()
	mergedAt := s.timestamp()
	msg := buildMergeCommitMessage(cr, actor, mergedAt)
	if err := s.git.MergeNoFF(cr.BaseBranch, cr.Branch, msg); err != nil {
		return "", err
	}

	if !keepBranch {
		if err := s.git.DeleteBranch(cr.Branch, true); err != nil {
			return "", err
		}
	}

	sha, err := s.git.HeadShortSHA()
	if err != nil {
		return "", err
	}

	cr.Status = model.StatusMerged
	cr.UpdatedAt = mergedAt
	cr.MergedAt = mergedAt
	cr.MergedBy = actor
	cr.MergedCommit = sha
	cr.FilesTouchedCount = len(files)
	if overrideReason != "" {
		cr.Events = append(cr.Events, model.Event{
			TS:      mergedAt,
			Actor:   actor,
			Type:    "cr_merge_overridden",
			Summary: fmt.Sprintf("Merged with validation override: %s", overrideReason),
			Ref:     fmt.Sprintf("cr:%d", cr.ID),
			Meta: map[string]string{
				"override_reason":   overrideReason,
				"risk_tier":         nonEmptyTrimmed(validation.Impact.RiskTier, "-"),
				"validation_errors": strconv.Itoa(len(validation.Errors)),
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
		return "", err
	}
	if err := s.backfillChildrenAfterParentMerge(cr); err != nil {
		return "", err
	}
	if err := s.syncDelegatedTasksAfterChildMerge(cr.ID); err != nil {
		return "", err
	}

	return sha, nil
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
		if _, ok := parseCRBranchID(branch); !ok {
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
	id, ok := parseCRBranchID(branch)
	if !ok {
		return nil, ErrNoActiveCRContext
	}
	cr, err := s.store.LoadCR(id)
	if err != nil {
		return nil, err
	}
	if _, err := s.ensureCRBaseFields(cr, true); err != nil {
		return nil, err
	}
	return &CurrentCRContext{Branch: branch, CR: cr}, nil
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
		if err := s.git.CheckoutBranch(cr.Branch); err != nil {
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

	return cr, nil
}
