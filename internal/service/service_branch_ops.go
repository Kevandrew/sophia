package service

import (
	"fmt"
	"sophia/internal/model"
	"strings"
)

type BranchFormatView struct {
	ID          int
	Title       string
	OwnerPrefix string
	Branch      string
}

type BranchResolveView struct {
	InputBranch string
	CR          *model.CR
}

type BranchMigrateView struct {
	CRID    int
	UID     string
	From    string
	To      string
	Applied bool
	Changed bool
}

func (s *Service) FormatCRBranch(id int, title, ownerPrefix string) (*BranchFormatView, error) {
	branch, err := formatCRBranchAlias(id, title, ownerPrefix)
	if err != nil {
		return nil, err
	}
	normalizedOwner, err := normalizeCRBranchOwnerPrefix(ownerPrefix)
	if err != nil {
		return nil, err
	}
	return &BranchFormatView{
		ID:          id,
		Title:       strings.TrimSpace(title),
		OwnerPrefix: normalizedOwner,
		Branch:      branch,
	}, nil
}

func (s *Service) ResolveCRBranch(branch string) (*BranchResolveView, error) {
	candidate := strings.TrimSpace(branch)
	if candidate == "" {
		current, err := s.git.CurrentBranch()
		if err != nil {
			return nil, err
		}
		candidate = strings.TrimSpace(current)
	}
	cr, err := s.resolveCRFromBranch(candidate)
	if err != nil {
		return nil, err
	}
	if _, err := s.ensureCRBaseFields(cr, true); err != nil {
		return nil, err
	}
	return &BranchResolveView{
		InputBranch: candidate,
		CR:          cr,
	}, nil
}

func (s *Service) MigrateCRBranch(id int, dryRun bool) (*BranchMigrateView, error) {
	cr, err := s.store.LoadCR(id)
	if err != nil {
		return nil, err
	}
	if cr.Status != model.StatusInProgress {
		return nil, fmt.Errorf("cr %d is not in progress", id)
	}
	cfg, err := s.store.LoadConfig()
	if err != nil {
		return nil, err
	}
	target, err := formatCRBranchAlias(cr.ID, cr.Title, cfg.BranchOwnerPrefix)
	if err != nil {
		return nil, err
	}

	view := &BranchMigrateView{
		CRID:    cr.ID,
		UID:     strings.TrimSpace(cr.UID),
		From:    cr.Branch,
		To:      target,
		Changed: strings.TrimSpace(cr.Branch) != strings.TrimSpace(target),
		Applied: false,
	}
	if !view.Changed || dryRun {
		return view, nil
	}

	if s.git.BranchExists(target) {
		return nil, fmt.Errorf("target branch %q already exists", target)
	}
	if !s.git.BranchExists(cr.Branch) {
		return nil, fmt.Errorf("source branch %q is missing", cr.Branch)
	}
	owner, ownerErr := s.branchOwnerWorktree(cr.Branch)
	if ownerErr != nil {
		return nil, ownerErr
	}
	if owner != nil && !s.isCurrentWorktreePath(owner.Path) {
		return nil, fmt.Errorf("%w: branch %q is checked out in worktree %q", ErrBranchInOtherWorktree, cr.Branch, owner.Path)
	}
	if err := s.git.RenameBranch(cr.Branch, target); err != nil {
		return nil, err
	}

	cr.Branch = target
	cr.UpdatedAt = s.timestamp()
	if err := s.store.SaveCR(cr); err != nil {
		return nil, err
	}
	if err := s.syncCRRef(cr); err != nil {
		return nil, err
	}
	view.Applied = true
	return view, nil
}
