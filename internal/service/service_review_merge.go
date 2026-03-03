package service

import (
	"errors"
	"fmt"
	"path/filepath"
	"sophia/internal/gitx"
	"sophia/internal/model"
	"strings"
)

type Review struct {
	CR                 *model.CR
	Contract           model.Contract
	LifecycleState     string
	AbandonedAt        string
	AbandonedBy        string
	AbandonedReason    string
	PRLinkageState     string
	ActionRequired     string
	ActionReason       string
	SuggestedCommands  []string
	Impact             *ImpactReport
	Trust              *TrustReport
	ValidationErrors   []string
	ValidationWarnings []string
	Files              []string
	ShortStat          string
	DiffNumStats       []gitx.DiffNumStat
	NewFiles           []string
	ModifiedFiles      []string
	DeletedFiles       []string
	TestFiles          []string
	DependencyFiles    []string
}

type mergePreflightView struct {
	validation     *ValidationReport
	trust          *TrustReport
	overrideReason string
}

type MergeCROptions struct {
	KeepBranch     bool
	OverrideReason string
	ApprovePROpen  bool
}

type MergeCRResult struct {
	MergedCommit string
	Warnings     []string
	MergeMode    string
	PRURL        string
	Action       string
	ActionReason string
	GateBlocked  bool
	GateReasons  []string
}

type MergeStatusView struct {
	CRID          int
	CRUID         string
	BaseBranch    string
	CRBranch      string
	WorktreePath  string
	InProgress    bool
	ConflictFiles []string
	TargetMatches bool
	MergeHead     string
	Advice        []string
}

func (s *Service) ReviewCR(id int) (*Review, error) {
	mergeStore := s.activeMergeStoreProvider()
	cr, err := mergeStore.LoadCR(id)
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
	trust := s.trustDomainService().buildReportWithPolicy(cr, validation, diff, policy.Contract.RequiredFields, policy)
	lifecycleState := strings.TrimSpace(cr.Status)
	abandonedAt := strings.TrimSpace(cr.AbandonedAt)
	abandonedBy := strings.TrimSpace(cr.AbandonedBy)
	abandonedReason := strings.TrimSpace(cr.AbandonedReason)
	prLinkageState := ""
	actionRequired := ""
	actionReason := ""
	suggestedCommands := []string{}
	if statusView, statusErr := s.StatusCR(id); statusErr == nil && statusView != nil {
		lifecycleState = nonEmptyTrimmed(statusView.LifecycleState, lifecycleState)
		abandonedAt = nonEmptyTrimmed(statusView.AbandonedAt, abandonedAt)
		abandonedBy = nonEmptyTrimmed(statusView.AbandonedBy, abandonedBy)
		abandonedReason = nonEmptyTrimmed(statusView.AbandonedReason, abandonedReason)
		prLinkageState = strings.TrimSpace(statusView.PRLinkageState)
		actionRequired = strings.TrimSpace(statusView.ActionRequired)
		actionReason = strings.TrimSpace(statusView.ActionReason)
		suggestedCommands = cleanAndDedupeStrings(statusView.SuggestedCommands)
	}

	return &Review{
		CR:                 cr,
		Contract:           cr.Contract,
		LifecycleState:     lifecycleState,
		AbandonedAt:        abandonedAt,
		AbandonedBy:        abandonedBy,
		AbandonedReason:    abandonedReason,
		PRLinkageState:     prLinkageState,
		ActionRequired:     actionRequired,
		ActionReason:       actionReason,
		SuggestedCommands:  suggestedCommands,
		Impact:             validation.Impact,
		Trust:              trust,
		ValidationErrors:   append([]string(nil), validation.Errors...),
		ValidationWarnings: append([]string(nil), validation.Warnings...),
		Files:              diff.Files,
		ShortStat:          diff.ShortStat,
		DiffNumStats:       append([]gitx.DiffNumStat(nil), diff.NumStats...),
		NewFiles:           diff.NewFiles,
		ModifiedFiles:      diff.ModifiedFiles,
		DeletedFiles:       diff.DeletedFiles,
		TestFiles:          diff.TestFiles,
		DependencyFiles:    diff.DependencyFiles,
	}, nil
}

func (s *Service) MergeCR(id int, keepBranch bool, overrideReason string) (string, error) {
	result, err := s.MergeCRWithOptions(id, MergeCROptions{
		KeepBranch:     keepBranch,
		OverrideReason: overrideReason,
	})
	if err != nil {
		return "", err
	}
	return result.MergedCommit, nil
}

func (s *Service) MergeCRWithWarnings(id int, keepBranch bool, overrideReason string) (string, []string, error) {
	result, err := s.MergeCRWithOptions(id, MergeCROptions{
		KeepBranch:     keepBranch,
		OverrideReason: overrideReason,
	})
	if err != nil {
		return "", nil, err
	}
	return result.MergedCommit, append([]string(nil), result.Warnings...), nil
}

func (s *Service) MergeCRWithOptions(id int, opts MergeCROptions) (*MergeCRResult, error) {
	policy, policyErr := s.repoPolicy()
	if policyErr != nil {
		return nil, policyErr
	}
	if policyMergeMode(policy) == "pr_gate" {
		var out *MergeCRResult
		if err := s.withMergeMutationLock(func() error {
			var err error
			out, err = s.mergePRGateCRUnlocked(id, opts, policy)
			return err
		}); err != nil {
			return nil, err
		}
		return out, nil
	}
	return s.runMergeOperation(func() (string, []string, error) {
		return s.mergeDomainService().mergeCRWithWarningsUnlocked(id, opts.KeepBranch, opts.OverrideReason)
	})
}

func (s *Service) MergeFinalizeWithOptions(id int, opts MergeCROptions) (*MergeCRResult, error) {
	policy, policyErr := s.repoPolicy()
	if policyErr != nil {
		return nil, policyErr
	}
	if policyMergeMode(policy) != "pr_gate" {
		return s.MergeCRWithOptions(id, opts)
	}
	var out *MergeCRResult
	if err := s.withMergeMutationLock(func() error {
		var err error
		out, err = s.mergePRGateFinalizeUnlocked(id, opts, policy)
		return err
	}); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Service) runMergeOperation(operation func() (string, []string, error)) (*MergeCRResult, error) {
	var (
		sha      string
		warnings []string
	)
	if err := s.withMergeMutationLock(func() error {
		var opErr error
		sha, warnings, opErr = operation()
		return opErr
	}); err != nil {
		return nil, err
	}
	return &MergeCRResult{
		MergedCommit: sha,
		Warnings:     append([]string{}, warnings...),
	}, nil
}

func (s *Service) MergeStatusCR(id int) (*MergeStatusView, error) {
	return s.mergeDomainService().mergeStatusCR(id)
}

func (s *Service) AbortMergeCR(id int) error {
	return s.withMergeMutationLock(func() error {
		return s.abortMergeCRUnlocked(id)
	})
}

func (s *Service) abortMergeCRUnlocked(id int) error {
	return s.mergeDomainService().abortMergeCRUnlocked(id)
}

func (s *Service) ResumeMergeCR(id int, keepBranch bool, overrideReason string) (string, []string, error) {
	result, err := s.ResumeMergeCRWithOptions(id, MergeCROptions{
		KeepBranch:     keepBranch,
		OverrideReason: overrideReason,
	})
	if err != nil {
		return "", nil, err
	}
	return result.MergedCommit, append([]string(nil), result.Warnings...), nil
}

func (s *Service) ResumeMergeCRWithOptions(id int, opts MergeCROptions) (*MergeCRResult, error) {
	return s.runMergeOperation(func() (string, []string, error) {
		return s.mergeDomainService().resumeMergeCRUnlocked(id, opts.KeepBranch, opts.OverrideReason)
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
			if candidate.Status != model.StatusInProgress && candidate.Status != model.StatusAbandoned {
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
	if cr.Status != model.StatusInProgress && cr.Status != model.StatusAbandoned {
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
		if cr.Status == model.StatusAbandoned {
			return nil, fmt.Errorf("branch %q is missing for abandoned CR %d; run sophia cr reopen %d", cr.Branch, cr.ID, cr.ID)
		}
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
	if cr.Status != model.StatusMerged && cr.Status != model.StatusAbandoned {
		return nil, fmt.Errorf("cr %d is neither merged nor abandoned", id)
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
	cr.AbandonedAt = ""
	cr.AbandonedBy = ""
	cr.AbandonedReason = ""
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
