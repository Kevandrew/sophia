package service

import (
	"fmt"
	"sophia/internal/model"
	"strings"
)

func (s *Service) RefreshCR(id int, opts RefreshOptions) (*CRRefreshView, error) {
	cr, err := s.store.LoadCR(id)
	if err != nil {
		return nil, err
	}
	if guardErr := s.ensureNoMergeInProgressForCR(cr); guardErr != nil {
		return nil, guardErr
	}
	if cr.Status != model.StatusInProgress {
		return nil, fmt.Errorf("cr %d is not in progress", id)
	}
	if _, err := s.ensureCRBaseFields(cr, true); err != nil {
		return nil, err
	}

	strategy, err := normalizeRefreshStrategy(opts.Strategy)
	if err != nil {
		return nil, err
	}
	warnings := []string{}
	if strategy == RefreshStrategyAuto {
		if cr.ParentCRID > 0 {
			strategy = RefreshStrategyRestack
		} else {
			strategy = RefreshStrategyRebase
		}
		warnings = append(warnings, fmt.Sprintf("auto-selected strategy: %s", strategy))
	}

	beforeHead := ""
	if s.git.BranchExists(cr.Branch) {
		if resolved, resolveErr := s.git.ResolveRef(cr.Branch); resolveErr == nil {
			beforeHead = strings.TrimSpace(resolved)
		}
	}

	view := &CRRefreshView{
		CRID:       cr.ID,
		Strategy:   strategy,
		DryRun:     opts.DryRun,
		Applied:    false,
		BaseRef:    strings.TrimSpace(nonEmptyTrimmed(cr.BaseRef, cr.BaseBranch)),
		BeforeHead: beforeHead,
		Warnings:   warnings,
	}

	switch strategy {
	case RefreshStrategyRestack:
		if cr.ParentCRID <= 0 {
			return nil, ErrParentCRRequired
		}
		parent, parentErr := s.store.LoadCR(cr.ParentCRID)
		if parentErr != nil {
			return nil, parentErr
		}
		targetRef := ""
		switch {
		case parent.Status == model.StatusInProgress && s.git.BranchExists(parent.Branch):
			targetRef = parent.Branch
		case parent.Status == model.StatusMerged && strings.TrimSpace(parent.MergedCommit) != "":
			targetRef = strings.TrimSpace(parent.MergedCommit)
		default:
			return nil, fmt.Errorf("parent CR %d has no restack anchor", parent.ID)
		}
		view.TargetRef = targetRef
		if opts.DryRun {
			return view, nil
		}
		updated, restackErr := s.RestackCR(id)
		if restackErr != nil {
			return nil, restackErr
		}
		view.Applied = true
		view.BaseRef = strings.TrimSpace(nonEmptyTrimmed(updated.BaseRef, updated.BaseBranch))
	case RefreshStrategyRebase:
		targetRef := strings.TrimSpace(nonEmptyTrimmed(cr.BaseRef, cr.BaseBranch))
		if targetRef == "" {
			return nil, fmt.Errorf("cr %d has no base ref for rebase refresh", cr.ID)
		}
		view.TargetRef = targetRef
		if opts.DryRun {
			return view, nil
		}
		updated, baseErr := s.SetCRBase(id, targetRef, true)
		if baseErr != nil {
			return nil, baseErr
		}
		view.Applied = true
		view.BaseRef = strings.TrimSpace(nonEmptyTrimmed(updated.BaseRef, updated.BaseBranch))
	default:
		return nil, fmt.Errorf("unsupported refresh strategy %q", strategy)
	}

	if s.git.BranchExists(cr.Branch) {
		if resolved, resolveErr := s.git.ResolveRef(cr.Branch); resolveErr == nil {
			view.AfterHead = strings.TrimSpace(resolved)
		}
	}
	return view, nil
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
