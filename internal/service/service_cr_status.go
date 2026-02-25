package service

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"sophia/internal/model"
)

func (s *Service) WhyCR(id int) (*WhyView, error) {
	statusStore := s.activeStatusStoreProvider()
	cr, err := statusStore.LoadCR(id)
	if err != nil {
		return nil, err
	}
	cr, err = s.ensureCRBaseFieldsPersisted(cr)
	if err != nil {
		return nil, err
	}

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
	statusStore := s.activeStatusStoreProvider()
	statusGit := s.activeStatusGitProvider()
	cr, err := statusStore.LoadCR(id)
	if err != nil {
		return nil, err
	}
	cr, err = s.ensureCRBaseFieldsPersisted(cr)
	if err != nil {
		return nil, err
	}

	currentBranch, _ := statusGit.CurrentBranch()
	statusEntries, err := statusGit.WorkingTreeStatus()
	if err != nil {
		return nil, err
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
		ID:                    cr.ID,
		UID:                   strings.TrimSpace(cr.UID),
		Title:                 cr.Title,
		Status:                cr.Status,
		BaseBranch:            cr.BaseBranch,
		BaseRef:               strings.TrimSpace(cr.BaseRef),
		BaseCommit:            strings.TrimSpace(cr.BaseCommit),
		ParentCRID:            cr.ParentCRID,
		Branch:                cr.Branch,
		CurrentBranch:         currentBranch,
		BranchMatch:           strings.TrimSpace(currentBranch) != "" && currentBranch == cr.Branch,
		ModifiedStagedCount:   modifiedStagedCount,
		UntrackedCount:        untrackedCount,
		Dirty:                 modifiedStagedCount > 0 || untrackedCount > 0,
		TasksTotal:            len(cr.Subtasks),
		TasksOpen:             tasksOpen,
		TasksDone:             tasksDone,
		TasksDelegated:        tasksDelegated,
		TasksDelegatedPending: tasksDelegatedPending,
		ContractComplete:      len(missingFields) == 0,
		ContractMissingFields: missingFields,
		ValidationValid:       true,
		RiskTier:              "-",
	}
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
	}

	return view, nil
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
	for _, field := range missingCRContractFields(cr.Contract, policy.Contract.RequiredFields) {
		errorsOut = append(errorsOut, fmt.Sprintf("missing required contract field: %s", field))
	}
	for _, driftPath := range impact.ScopeDrift {
		errorsOut = append(errorsOut, fmt.Sprintf("scope drift: changed path %q is outside declared contract scope", driftPath))
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
