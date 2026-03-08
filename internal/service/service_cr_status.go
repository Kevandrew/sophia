package service

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"sophia/internal/model"
)

type WhyView struct {
	CRID              int
	CRUID             string
	BaseRef           string
	BaseCommit        string
	ParentCRID        int
	EffectiveWhy      string
	Source            string
	Description       string
	ContractWhy       string
	ContractUpdatedAt string
	ContractUpdatedBy string
}

type CRStatusView struct {
	ID                         int
	UID                        string
	Title                      string
	Status                     string
	BaseBranch                 string
	BaseRef                    string
	BaseCommit                 string
	ParentCRID                 int
	ParentStatus               string
	Branch                     string
	CurrentBranch              string
	BranchMatch                bool
	OwnerWorktreePath          string
	CurrentWorktreePath        string
	OwnerIsCurrentWorktree     bool
	CheckedOutInOtherWorktree  bool
	SuggestedWorktreeCommand   string
	ModifiedStagedCount        int
	UntrackedCount             int
	Dirty                      bool
	TasksTotal                 int
	TasksOpen                  int
	TasksDone                  int
	TasksDelegated             int
	TasksDelegatedPending      int
	IsAggregateParent          bool
	AggregateResolvedChildren  []int
	AggregatePendingChildren   []int
	ContractComplete           bool
	ContractMissingFields      []string
	ValidationValid            bool
	ValidationErrors           int
	ValidationWarnings         int
	RiskTier                   string
	RiskScore                  int
	MergeBlocked               bool
	MergeBlockers              []string
	LifecycleState             string
	AbandonedAt                string
	AbandonedBy                string
	AbandonedReason            string
	PRLinkageState             string
	ActionRequired             string
	ActionReason               string
	SuggestedCommands          []string
	FreshnessState             string
	FreshnessReason            string
	FreshnessSuggestedCommands []string
}

func (s *Service) WhyCR(id int) (*WhyView, error) {
	statusStore := s.activeStatusStoreProvider()
	statusGit := s.activeStatusGitProvider()
	cr, err := statusStore.LoadCR(id)
	if err != nil {
		return nil, err
	}
	cr, err = s.ensureCRBaseFieldsPersistedWithProviders(cr, statusStore, statusGit)
	if err != nil {
		return nil, err
	}
	allCRs, err := s.store.ListCRs()
	if err != nil {
		return nil, err
	}
	readModel := buildCRReadModel(allCRs)
	normalizedCR := readModel.normalizeCR(*cr)
	cr = &normalizedCR

	description := strings.TrimSpace(cr.Description)
	contractWhy := strings.TrimSpace(cr.Contract.Why)
	effectiveWhy := ""
	source := "missing"
	switch {
	case contractWhy != "":
		effectiveWhy = contractWhy
		source = "contract_why"
	case description != "":
		effectiveWhy = description
		source = "description"
	}

	return &WhyView{
		CRID:              cr.ID,
		CRUID:             strings.TrimSpace(cr.UID),
		BaseRef:           strings.TrimSpace(cr.BaseRef),
		BaseCommit:        strings.TrimSpace(cr.BaseCommit),
		ParentCRID:        cr.ParentCRID,
		EffectiveWhy:      effectiveWhy,
		Source:            source,
		Description:       description,
		ContractWhy:       contractWhy,
		ContractUpdatedAt: strings.TrimSpace(cr.Contract.UpdatedAt),
		ContractUpdatedBy: strings.TrimSpace(cr.Contract.UpdatedBy),
	}, nil
}

func (s *Service) StatusCR(id int) (*CRStatusView, error) {
	s.ensurePRGateReconciledInStatus(id)
	statusStore := s.activeStatusStoreProvider()
	statusGit := s.activeStatusGitProvider()
	cr, err := statusStore.LoadCR(id)
	if err != nil {
		return nil, err
	}
	cr, err = s.ensureCRBaseFieldsPersistedWithProviders(cr, statusStore, statusGit)
	if err != nil {
		return nil, err
	}
	allCRs, err := s.store.ListCRs()
	if err != nil {
		return nil, err
	}
	readModel := buildCRReadModel(allCRs)
	normalizedCR := readModel.normalizeCR(*cr)
	cr = &normalizedCR

	currentBranch, _ := statusGit.CurrentBranch()
	statusEntries, err := statusGit.WorkingTreeStatus()
	if err != nil {
		return nil, err
	}
	worktreeContext, err := s.resolveBranchWorktreeContext(cr.ID, cr.Branch, fmt.Sprintf("sophia cr switch %d", cr.ID))
	if err != nil {
		worktreeContext = &branchWorktreeContext{
			CurrentWorktreePath: strings.TrimSpace(s.git.WorkDir),
		}
	}

	modifiedStagedCount := 0
	untrackedCount := 0
	for _, entry := range statusEntries {
		if entry.Code == "??" {
			untrackedCount++
			continue
		}
		modifiedStagedCount++
	}

	tasksOpen := 0
	tasksDone := 0
	tasksDelegated := 0
	tasksDelegatedPending := 0
	for _, task := range cr.Subtasks {
		switch task.Status {
		case model.TaskStatusDone:
			tasksDone++
		case model.TaskStatusDelegated:
			tasksDelegated++
			if len(s.pendingDelegationChildIDs(task)) > 0 {
				tasksDelegatedPending++
			}
		default:
			tasksOpen++
		}
	}

	policy, err := s.repoPolicy()
	if err != nil {
		return nil, err
	}
	missingFields := missingCRContractFields(cr.Contract, policy.Contract.RequiredFields)
	view := &CRStatusView{
		ID:                        cr.ID,
		UID:                       strings.TrimSpace(cr.UID),
		Title:                     cr.Title,
		Status:                    cr.Status,
		BaseBranch:                cr.BaseBranch,
		BaseRef:                   strings.TrimSpace(cr.BaseRef),
		BaseCommit:                strings.TrimSpace(cr.BaseCommit),
		ParentCRID:                cr.ParentCRID,
		Branch:                    cr.Branch,
		CurrentBranch:             currentBranch,
		BranchMatch:               strings.TrimSpace(currentBranch) != "" && currentBranch == cr.Branch,
		OwnerWorktreePath:         worktreeContext.OwnerWorktreePath,
		CurrentWorktreePath:       worktreeContext.CurrentWorktreePath,
		OwnerIsCurrentWorktree:    worktreeContext.OwnerIsCurrentWorktree,
		CheckedOutInOtherWorktree: worktreeContext.CheckedOutInOtherWorktree,
		SuggestedWorktreeCommand:  worktreeContext.SuggestedCommand,
		ModifiedStagedCount:       modifiedStagedCount,
		UntrackedCount:            untrackedCount,
		Dirty:                     modifiedStagedCount > 0 || untrackedCount > 0,
		TasksTotal:                len(cr.Subtasks),
		TasksOpen:                 tasksOpen,
		TasksDone:                 tasksDone,
		TasksDelegated:            tasksDelegated,
		TasksDelegatedPending:     tasksDelegatedPending,
		ContractComplete:          len(missingFields) == 0,
		ContractMissingFields:     missingFields,
		LifecycleState:            strings.TrimSpace(cr.Status),
		AbandonedAt:               strings.TrimSpace(cr.AbandonedAt),
		AbandonedBy:               strings.TrimSpace(cr.AbandonedBy),
		AbandonedReason:           strings.TrimSpace(cr.AbandonedReason),
		ValidationValid:           true,
		RiskTier:                  "-",
		FreshnessState:            "unknown",
	}
	baseRef := strings.TrimSpace(nonEmptyTrimmed(cr.BaseRef, cr.BaseBranch))
	baseCommit := strings.TrimSpace(cr.BaseCommit)
	switch {
	case cr.Status == model.StatusMerged:
		mergedAnchor := strings.TrimSpace(nonEmptyTrimmed(cr.MergedCommit, baseCommit))
		switch {
		case baseRef == "":
			view.FreshnessReason = "Merged CR has no recorded base ref."
		case mergedAnchor == "":
			view.FreshnessReason = fmt.Sprintf("Merged CR base ref %q has no recorded merge/base anchor.", baseRef)
		case s.mergedHistoricalParentRefMissing(cr):
			view.FreshnessState = "current"
			view.FreshnessReason = fmt.Sprintf("CR is merged; historical parent branch %q has been cleaned up after merge.", baseRef)
		default:
			resolvedBase, resolveErr := statusGit.ResolveRef(baseRef)
			if resolveErr != nil {
				view.FreshnessReason = fmt.Sprintf("Unable to resolve merged CR base ref %q: %v", baseRef, resolveErr)
				break
			}
			resolvedBase = strings.TrimSpace(resolvedBase)
			if resolvedBase == "" {
				view.FreshnessReason = fmt.Sprintf("Unable to resolve merged CR base ref %q.", baseRef)
				break
			}
			view.FreshnessState = "current"
			view.FreshnessReason = fmt.Sprintf("CR is merged; freshness checks no longer apply after merge into %q at %s.", baseRef, shortHash(mergedAnchor))
		}
	case baseRef == "":
		view.FreshnessReason = "CR has no recorded base ref."
	case baseCommit == "":
		view.FreshnessReason = fmt.Sprintf("CR base ref %q has no recorded base commit yet.", baseRef)
	default:
		resolvedBase, resolveErr := statusGit.ResolveRef(baseRef)
		if resolveErr != nil {
			view.FreshnessReason = fmt.Sprintf("Unable to resolve base ref %q: %v", baseRef, resolveErr)
			break
		}
		resolvedBase = strings.TrimSpace(resolvedBase)
		if resolvedBase == "" {
			view.FreshnessReason = fmt.Sprintf("Unable to resolve base ref %q.", baseRef)
			break
		}
		if resolvedBase != baseCommit {
			view.FreshnessState = "stale"
			view.FreshnessReason = fmt.Sprintf("Base ref %q moved from %s to %s.", baseRef, shortHash(baseCommit), shortHash(resolvedBase))
			view.FreshnessSuggestedCommands = dedupeStrings([]string{fmt.Sprintf("sophia cr refresh %d", cr.ID)})
		} else {
			view.FreshnessState = "current"
			view.FreshnessReason = fmt.Sprintf("Base ref %q still matches recorded base commit %s.", baseRef, shortHash(resolvedBase))
		}
	}
	aggregateParent := s.aggregateParentViewForCR(cr)
	view.IsAggregateParent = aggregateParent.IsAggregateParent
	view.AggregateResolvedChildren = append([]int(nil), aggregateParent.ResolvedChildCRIDs...)
	view.AggregatePendingChildren = append([]int(nil), aggregateParent.PendingChildCRIDs...)
	if cr.ParentCRID > 0 {
		parent, parentErr := statusStore.LoadCR(cr.ParentCRID)
		if parentErr != nil {
			view.ParentStatus = "missing"
		} else {
			view.ParentStatus = parent.Status
		}
	}

	if cr.Status == model.StatusInProgress {
		report, validateErr := s.ValidateCR(id)
		if validateErr != nil {
			return nil, validateErr
		}
		view.ValidationValid = report.Valid
		view.ValidationErrors = len(report.Errors)
		view.ValidationWarnings = len(report.Warnings)
		view.MergeBlockers = s.mergeBlockersForCR(cr, report)
		view.MergeBlocked = len(view.MergeBlockers) > 0
		if report.Impact != nil {
			view.RiskTier = nonEmptyTrimmed(report.Impact.RiskTier, "-")
			view.RiskScore = report.Impact.RiskScore
		}
		if policyMergeMode(policy) == "pr_gate" {
			if prStatus, prStatusErr := s.PRStatus(id); prStatusErr == nil && prStatus != nil {
				view.PRLinkageState = strings.TrimSpace(prStatus.LinkageState)
				view.ActionRequired = strings.TrimSpace(prStatus.ActionRequired)
				view.ActionReason = strings.TrimSpace(prStatus.ActionReason)
				view.SuggestedCommands = dedupeStrings(append([]string(nil), prStatus.SuggestedCommands...))
				if prStatus.GateBlocked {
					view.MergeBlocked = true
					view.MergeBlockers = dedupeStrings(append(view.MergeBlockers, prStatus.GateReasons...))
				}
			}
		}
	}
	if cr.Status == model.StatusAbandoned {
		view.MergeBlocked = true
		view.MergeBlockers = dedupeStrings(append(view.MergeBlockers, fmt.Sprintf("CR %d is abandoned; run `sophia cr reopen %d` to resume", cr.ID, cr.ID)))
		view.ActionRequired = "reopen_cr"
		view.ActionReason = fmt.Sprintf("CR %d is abandoned", cr.ID)
		view.SuggestedCommands = dedupeStrings(append(view.SuggestedCommands, fmt.Sprintf("sophia cr reopen %d", cr.ID)))
	}

	return view, nil
}

func (s *Service) mergedHistoricalParentRefMissing(cr *model.CR) bool {
	if s == nil || cr == nil || strings.TrimSpace(cr.Status) != model.StatusMerged {
		return false
	}
	baseRef := strings.TrimSpace(cr.BaseRef)
	if baseRef == "" || s.git.BranchExists(baseRef) {
		return false
	}
	allCRs, err := s.store.ListCRs()
	if err != nil {
		return false
	}
	parentID := expectedParentCRIDFromBaseRef(baseRef, cr.ID, allCRs)
	if parentID <= 0 && cr.ParentCRID > 0 {
		parentID = cr.ParentCRID
	}
	if parentID <= 0 {
		return false
	}
	for _, candidate := range allCRs {
		if candidate.ID != parentID {
			continue
		}
		return strings.TrimSpace(candidate.Status) == model.StatusMerged &&
			strings.TrimSpace(candidate.Branch) == baseRef
	}
	return false
}

func (s *Service) ImpactCR(id int) (*ImpactReport, error) {
	statusStore := s.activeStatusStoreProvider()
	cr, err := statusStore.LoadCR(id)
	if err != nil {
		return nil, err
	}
	policy, policyWarnings, err := s.repoPolicyWithWarnings()
	if err != nil {
		return nil, err
	}
	diff, err := s.summarizeCRDiffWithPolicy(cr, policy)
	if err != nil {
		if errors.Is(err, ErrCRBranchContextUnavailable) {
			diff = s.summarizeCRDiffFromTaskCheckpoints(cr, policy)
			impact := buildImpactReport(cr, diff, policy)
			impact.Warnings = append([]string(nil), policyWarnings...)
			impact.Warnings = append(impact.Warnings, "branch context unavailable; impact derived from CR metadata only")
			impact.Warnings = dedupeStrings(impact.Warnings)
			return impact, nil
		}
		return nil, err
	}
	impact := buildImpactReport(cr, diff, policy)
	impact.Warnings = append(impact.Warnings, policyWarnings...)
	impact.Warnings = dedupeStrings(impact.Warnings)
	return impact, nil
}

func (s *Service) ValidateCR(id int) (*ValidationReport, error) {
	statusStore := s.activeStatusStoreProvider()
	cr, err := statusStore.LoadCR(id)
	if err != nil {
		return nil, err
	}
	policy, policyWarnings, err := s.repoPolicyWithWarnings()
	if err != nil {
		return nil, err
	}
	diff, err := s.summarizeCRDiffWithPolicy(cr, policy)
	if err != nil {
		if errors.Is(err, ErrCRBranchContextUnavailable) {
			return s.validateCRWithoutDiffContext(cr, policy, policyWarnings), nil
		}
		return nil, err
	}
	impact := buildImpactReport(cr, diff, policy)

	errorsOut := make([]string, 0)
	scopeLabel := "declared contract scope"
	if strings.TrimSpace(impact.ScopeSource) == "contract_baseline" {
		scopeLabel = "frozen contract baseline scope"
	}
	for _, field := range missingCRContractFields(cr.Contract, policy.Contract.RequiredFields) {
		errorsOut = append(errorsOut, fmt.Sprintf("missing required contract field: %s", field))
	}
	for _, driftPath := range impact.ScopeDrift {
		errorsOut = append(errorsOut, fmt.Sprintf("scope drift: changed path %q is outside %s", driftPath, scopeLabel))
	}
	errorsOut = append(errorsOut, policyScopeViolationErrors(cr, policy.Scope.AllowedPrefixes)...)
	for _, task := range cr.Subtasks {
		if validateErr := validateTaskAcceptanceCheckKeys(task.ID, task.Contract.AcceptanceChecks, policy); validateErr != nil {
			errorsOut = append(errorsOut, validateErr.Error())
		}
	}
	errorsOut = dedupeStrings(errorsOut)
	sort.Strings(errorsOut)

	warnings := append([]string(nil), policyWarnings...)
	if strings.TrimSpace(impact.ScopeSource) == "contract_baseline" {
		warnings = append(warnings, "scope drift is evaluated against frozen contract baseline scope (first checkpoint)")
	}
	warnings = append(warnings, impact.TaskScopeWarnings...)
	warnings = append(warnings, impact.TaskContractWarnings...)
	warnings = append(warnings, impact.TaskChunkWarnings...)
	warnings = dedupeStrings(warnings)
	return &ValidationReport{
		Valid:    len(errorsOut) == 0,
		Errors:   errorsOut,
		Warnings: warnings,
		Impact:   impact,
	}, nil
}

func (s *Service) validateCRWithoutDiffContext(cr *model.CR, policy *model.RepoPolicy, policyWarnings []string) *ValidationReport {
	impact := buildImpactReport(cr, &diffSummary{}, policy)
	errorsOut := []string{
		fmt.Sprintf("unable to validate CR diff because branch context is unavailable for %q", strings.TrimSpace(cr.Branch)),
	}
	for _, field := range missingCRContractFields(cr.Contract, policy.Contract.RequiredFields) {
		errorsOut = append(errorsOut, fmt.Sprintf("missing required contract field: %s", field))
	}
	errorsOut = append(errorsOut, policyScopeViolationErrors(cr, policy.Scope.AllowedPrefixes)...)
	for _, task := range cr.Subtasks {
		if validateErr := validateTaskAcceptanceCheckKeys(task.ID, task.Contract.AcceptanceChecks, policy); validateErr != nil {
			errorsOut = append(errorsOut, validateErr.Error())
		}
	}
	errorsOut = dedupeStrings(errorsOut)
	sort.Strings(errorsOut)

	warnings := append([]string(nil), policyWarnings...)
	warnings = append(warnings, "branch context unavailable; validation derived from CR metadata only")
	if strings.TrimSpace(impact.ScopeSource) == "contract_baseline" {
		warnings = append(warnings, "scope drift source: frozen contract baseline scope (first checkpoint)")
	}
	warnings = append(warnings, impact.TaskScopeWarnings...)
	warnings = append(warnings, impact.TaskContractWarnings...)
	warnings = append(warnings, impact.TaskChunkWarnings...)
	warnings = dedupeStrings(warnings)

	return &ValidationReport{
		Valid:    false,
		Errors:   errorsOut,
		Warnings: warnings,
		Impact:   impact,
	}
}

func (s *Service) RecordCRValidation(id int, report *ValidationReport) error {
	return s.withMutationLock(func() error {
		return s.recordCRValidationUnlocked(id, report)
	})
}

func (s *Service) recordCRValidationUnlocked(id int, report *ValidationReport) error {
	statusStore := s.activeStatusStoreProvider()
	statusGit := s.activeStatusGitProvider()
	if report == nil {
		return errors.New("validation report is required")
	}
	cr, err := statusStore.LoadCR(id)
	if err != nil {
		return err
	}
	now := s.timestamp()
	actor := statusGit.Actor()
	status := "passed"
	if !report.Valid {
		status = "failed"
	}
	meta := map[string]string{
		"risk_tier":           "-",
		"validation_errors":   strconv.Itoa(len(report.Errors)),
		"validation_warnings": strconv.Itoa(len(report.Warnings)),
	}
	if report.Impact != nil {
		meta["risk_tier"] = nonEmptyTrimmed(report.Impact.RiskTier, "-")
	}
	cr.UpdatedAt = now
	cr.Events = append(cr.Events, model.Event{
		TS:      now,
		Actor:   actor,
		Type:    model.EventTypeCRValidated,
		Summary: fmt.Sprintf("Validation %s with %d error(s) and %d warning(s)", status, len(report.Errors), len(report.Warnings)),
		Ref:     fmt.Sprintf("cr:%d", id),
		Meta:    meta,
	})
	return statusStore.SaveCR(cr)
}
