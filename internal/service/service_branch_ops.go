package service

import (
	"fmt"
	"sophia/internal/model"
	"strings"
)

type BranchFormatView struct {
	ID          int
	UID         string
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

func (s *Service) FormatCRBranch(id int, title, ownerPrefix, uid string, ownerPrefixSet bool) (*BranchFormatView, error) {
	var existing *model.CR
	if id > 0 {
		cr, err := s.store.LoadCR(id)
		if err != nil && strings.TrimSpace(uid) == "" && strings.TrimSpace(title) == "" {
			return nil, err
		}
		if err == nil {
			existing = cr
		}
	}
	if existing != nil && strings.TrimSpace(title) == "" && strings.TrimSpace(uid) == "" && !ownerPrefixSet {
		normalizedOwner := ""
		if inferred, ok := ownerPrefixFromBranch(existing.Branch); ok {
			normalizedOwner = inferred
		}
		return &BranchFormatView{
			ID:          id,
			UID:         strings.TrimSpace(existing.UID),
			Title:       strings.TrimSpace(existing.Title),
			OwnerPrefix: normalizedOwner,
			Branch:      strings.TrimSpace(existing.Branch),
		}, nil
	}

	resolvedTitle := strings.TrimSpace(title)
	if resolvedTitle == "" && existing != nil {
		resolvedTitle = strings.TrimSpace(existing.Title)
	}
	if resolvedTitle == "" {
		return nil, fmt.Errorf("--title is required")
	}

	resolvedUID := strings.TrimSpace(uid)
	if resolvedUID == "" && existing != nil {
		resolvedUID = strings.TrimSpace(existing.UID)
	}
	resolvedOwner := strings.TrimSpace(ownerPrefix)
	if !ownerPrefixSet && resolvedOwner == "" {
		if existing != nil {
			if inferred, ok := ownerPrefixFromBranch(existing.Branch); ok {
				resolvedOwner = inferred
			}
		}
		if resolvedOwner == "" {
			if cfg, cfgErr := s.store.LoadConfig(); cfgErr == nil {
				resolvedOwner = strings.TrimSpace(cfg.BranchOwnerPrefix)
			}
		}
	}
	if resolvedUID == "" {
		if id <= 0 {
			return nil, fmt.Errorf("--uid is required (or supply an existing --id)")
		}
		branch, err := formatCRBranchAlias(id, resolvedTitle, resolvedOwner)
		if err != nil {
			return nil, err
		}
		normalizedOwner, err := normalizeCRBranchOwnerPrefix(resolvedOwner)
		if err != nil {
			return nil, err
		}
		return &BranchFormatView{
			ID:          id,
			UID:         "",
			Title:       resolvedTitle,
			OwnerPrefix: normalizedOwner,
			Branch:      branch,
		}, nil
	}
	resolvedUID, err := normalizeCRUID(resolvedUID)
	if err != nil {
		return nil, err
	}

	existingBranch := ""
	if existing != nil {
		existingBranch = strings.TrimSpace(existing.Branch)
	}
	branch, err := formatCRBranchAliasWithFallback(resolvedTitle, resolvedOwner, resolvedUID, func(candidate string) bool {
		if existingBranch != "" && strings.TrimSpace(candidate) == existingBranch {
			return false
		}
		return s.git.BranchExists(candidate)
	})
	if err != nil {
		return nil, err
	}

	normalizedOwner, err := normalizeCRBranchOwnerPrefix(resolvedOwner)
	if err != nil {
		return nil, err
	}
	return &BranchFormatView{
		ID:          id,
		UID:         resolvedUID,
		Title:       resolvedTitle,
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
	if _, err := ensureCRUID(cr); err != nil {
		return nil, err
	}
	if cr.Status != model.StatusInProgress {
		return nil, fmt.Errorf("cr %d is not in progress", id)
	}
	cfg, err := s.store.LoadConfig()
	if err != nil {
		return nil, err
	}
	target, err := formatCRBranchAliasWithFallback(cr.Title, cfg.BranchOwnerPrefix, cr.UID, func(candidate string) bool {
		if strings.TrimSpace(candidate) == strings.TrimSpace(cr.Branch) {
			return false
		}
		return s.git.BranchExists(candidate)
	})
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
