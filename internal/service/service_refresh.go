package service

import (
	"fmt"
	"sophia/internal/model"
	"strings"
)

type refreshPlanStep struct {
	CRID     int
	Depth    int
	Strategy string
	Cascaded bool
}

func (s *Service) RefreshCR(id int, opts RefreshOptions) (*CRRefreshView, error) {
	var view *CRRefreshView
	if err := s.withMutationLock(func() error {
		var err error
		view, err = s.refreshCRUnlocked(id, opts)
		return err
	}); err != nil {
		return nil, err
	}
	return view, nil
}

func (s *Service) refreshCRUnlocked(id int, opts RefreshOptions) (*CRRefreshView, error) {
	lifecycleStore := s.activeLifecycleStoreProvider()
	cr, err := lifecycleStore.LoadCR(id)
	if err != nil {
		return nil, err
	}
	if guardErr := s.ensureNoMergeInProgressForCR(cr); guardErr != nil {
		return nil, guardErr
	}
	if cr.Status != model.StatusInProgress {
		return nil, fmt.Errorf("cr %d is not in progress", id)
	}
	if _, err := s.ensureCRBaseFields(cr, false); err != nil {
		return nil, err
	}

	strategy, err := normalizeRefreshStrategy(opts.Strategy)
	if err != nil {
		return nil, err
	}
	readModel, err := s.loadCRReadModel()
	if err != nil {
		return nil, err
	}
	warnings := []string{}
	if strategy == RefreshStrategyAuto {
		if effectiveParentCRID(*cr, readModel.all) > 0 {
			strategy = RefreshStrategyRestack
		} else {
			strategy = RefreshStrategyRebase
		}
		warnings = append(warnings, fmt.Sprintf("auto-selected strategy: %s", strategy))
	}
	plan := s.buildRefreshPlan(cr, readModel, strategy)

	view := &CRRefreshView{
		CRID:     cr.ID,
		Strategy: strategy,
		DryRun:   opts.DryRun,
		Applied:  false,
		Warnings: warnings,
	}
	if len(plan) > 1 {
		view.CascadeCount = len(plan) - 1
		if opts.DryRun {
			view.Warnings = append(view.Warnings, fmt.Sprintf("parent refresh will cascade to %d descendant child CR(s)", view.CascadeCount))
		} else {
			view.Warnings = append(view.Warnings, fmt.Sprintf("parent refresh cascaded to %d descendant child CR(s)", view.CascadeCount))
		}
	}
	view.Entries = make([]CRRefreshEntryView, 0, len(plan))

	for _, step := range plan {
		entry, stepErr := s.refreshPlanStepUnlocked(step, opts.DryRun)
		if stepErr != nil {
			if step.Cascaded {
				return nil, fmt.Errorf("refresh cascade failed for child CR %d: %w", step.CRID, stepErr)
			}
			return nil, stepErr
		}
		view.Entries = append(view.Entries, *entry)
	}
	if len(view.Entries) > 0 {
		root := view.Entries[0]
		view.BaseRef = root.BaseRef
		view.TargetRef = root.TargetRef
		view.BeforeHead = root.BeforeHead
		view.AfterHead = root.AfterHead
	}
	view.Applied = !opts.DryRun
	return view, nil
}

func (s *Service) buildRefreshPlan(cr *model.CR, readModel *crReadModel, strategy string) []refreshPlanStep {
	if cr == nil {
		return nil
	}
	plan := []refreshPlanStep{{
		CRID:     cr.ID,
		Depth:    0,
		Strategy: strategy,
	}}
	if cr.ParentCRID > 0 || readModel == nil {
		return plan
	}
	children := readModel.childrenOf(cr.ID)
	if len(children) == 0 {
		return plan
	}
	var appendChildren func(parentID, depth int)
	appendChildren = func(parentID, depth int) {
		for _, child := range readModel.childrenOf(parentID) {
			plan = append(plan, refreshPlanStep{
				CRID:     child.ID,
				Depth:    depth,
				Strategy: RefreshStrategyRestack,
				Cascaded: true,
			})
			appendChildren(child.ID, depth+1)
		}
	}
	appendChildren(cr.ID, 1)
	return plan
}

func (s *Service) refreshPlanStepUnlocked(step refreshPlanStep, dryRun bool) (*CRRefreshEntryView, error) {
	lifecycleStore := s.activeLifecycleStoreProvider()
	lifecycleGit := s.activeLifecycleGitProvider()
	cr, err := lifecycleStore.LoadCR(step.CRID)
	if err != nil {
		return nil, err
	}
	if guardErr := s.ensureNoMergeInProgressForCR(cr); guardErr != nil {
		return nil, guardErr
	}
	if cr.Status != model.StatusInProgress {
		return nil, fmt.Errorf("cr %d is not in progress", cr.ID)
	}
	if _, err := s.ensureCRBaseFields(cr, false); err != nil {
		return nil, err
	}
	entry := &CRRefreshEntryView{
		CRID:       cr.ID,
		ParentCRID: cr.ParentCRID,
		Branch:     strings.TrimSpace(cr.Branch),
		Strategy:   step.Strategy,
		Depth:      step.Depth,
		Cascaded:   step.Cascaded,
		BaseRef:    strings.TrimSpace(nonEmptyTrimmed(cr.BaseRef, cr.BaseBranch)),
	}
	if lifecycleGit.BranchExists(cr.Branch) {
		if resolved, resolveErr := lifecycleGit.ResolveRef(cr.Branch); resolveErr == nil {
			entry.BeforeHead = strings.TrimSpace(resolved)
		}
	}

	switch step.Strategy {
	case RefreshStrategyRestack:
		allCRs, listErr := lifecycleStore.ListCRs()
		if listErr != nil {
			return nil, listErr
		}
		parentID := effectiveParentCRID(*cr, allCRs)
		if parentID <= 0 {
			return nil, ErrParentCRRequired
		}
		entry.ParentCRID = parentID
		parent, parentErr := lifecycleStore.LoadCR(parentID)
		if parentErr != nil {
			return nil, parentErr
		}
		switch {
		case parent.Status == model.StatusInProgress && lifecycleGit.BranchExists(parent.Branch):
			entry.TargetRef = strings.TrimSpace(parent.Branch)
			entry.BaseRef = strings.TrimSpace(parent.Branch)
		case parent.Status == model.StatusMerged && strings.TrimSpace(parent.MergedCommit) != "":
			entry.TargetRef = strings.TrimSpace(parent.MergedCommit)
			entry.BaseRef = strings.TrimSpace(nonEmptyTrimmed(cr.BaseBranch, cr.BaseRef))
		default:
			return nil, fmt.Errorf("parent CR %d has no restack anchor", parent.ID)
		}
		if dryRun {
			return entry, nil
		}
		updated, restackErr := s.restackCRUnlocked(step.CRID)
		if restackErr != nil {
			return nil, restackErr
		}
		entry.BaseRef = strings.TrimSpace(nonEmptyTrimmed(updated.BaseRef, updated.BaseBranch))
	case RefreshStrategyRebase:
		entry.TargetRef = strings.TrimSpace(nonEmptyTrimmed(cr.BaseRef, cr.BaseBranch))
		if entry.TargetRef == "" {
			return nil, fmt.Errorf("cr %d has no base ref for rebase refresh", cr.ID)
		}
		entry.BaseRef = entry.TargetRef
		if dryRun {
			return entry, nil
		}
		updated, rebaseErr := s.setCRBaseUnlocked(step.CRID, entry.TargetRef, true)
		if rebaseErr != nil {
			return nil, rebaseErr
		}
		entry.BaseRef = strings.TrimSpace(nonEmptyTrimmed(updated.BaseRef, updated.BaseBranch))
	default:
		return nil, fmt.Errorf("unsupported refresh strategy %q", step.Strategy)
	}

	if lifecycleGit.BranchExists(cr.Branch) {
		if resolved, resolveErr := lifecycleGit.ResolveRef(cr.Branch); resolveErr == nil {
			entry.AfterHead = strings.TrimSpace(resolved)
		}
	}
	return entry, nil
}

func normalizeRefreshStrategy(raw string) (string, error) {
	strategy := strings.TrimSpace(strings.ToLower(raw))
	if strategy == "" {
		return RefreshStrategyAuto, nil
	}
	switch strategy {
	case RefreshStrategyAuto, RefreshStrategyRestack, RefreshStrategyRebase:
		return strategy, nil
	default:
		return "", fmt.Errorf("invalid --strategy %q (expected auto|restack|rebase)", raw)
	}
}
