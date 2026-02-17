package service

import (
	"fmt"
	"path/filepath"
	"sophia/internal/gitx"
	"strings"
)

func (s *Service) branchOwnerWorktree(branch string) (*gitx.Worktree, error) {
	branch = strings.TrimSpace(branch)
	if branch == "" {
		return nil, nil
	}
	return s.git.WorktreeForBranch(branch)
}

func (s *Service) isCurrentWorktreePath(path string) bool {
	if strings.TrimSpace(path) == "" {
		return false
	}
	currentAbs, currentErr := filepath.Abs(s.git.WorkDir)
	otherAbs, otherErr := filepath.Abs(path)
	if currentErr != nil || otherErr != nil {
		return filepath.Clean(s.git.WorkDir) == filepath.Clean(path)
	}
	return filepath.Clean(currentAbs) == filepath.Clean(otherAbs)
}

func (s *Service) gitClientForBranch(branch string) (*gitx.Client, *gitx.Worktree, error) {
	owner, err := s.branchOwnerWorktree(branch)
	if err != nil {
		return nil, nil, err
	}
	if owner != nil && !s.isCurrentWorktreePath(owner.Path) {
		return gitx.New(owner.Path), owner, nil
	}
	return s.git, owner, nil
}

func (s *Service) rebaseBranchOnto(branch, ontoRef string) error {
	branch = strings.TrimSpace(branch)
	ontoRef = strings.TrimSpace(ontoRef)
	if branch == "" || ontoRef == "" {
		return fmt.Errorf("branch and onto ref are required")
	}

	rebaseGit, owner, err := s.gitClientForBranch(branch)
	if err != nil {
		return err
	}
	if dirty, summary, err := s.workingTreeDirtySummaryFor(rebaseGit); err != nil {
		return err
	} else if dirty {
		return fmt.Errorf("%w: %s", ErrWorkingTreeDirty, summary)
	}

	if owner != nil && !s.isCurrentWorktreePath(owner.Path) {
		currentBranch, branchErr := rebaseGit.CurrentBranch()
		if branchErr != nil {
			return branchErr
		}
		if strings.TrimSpace(currentBranch) != branch {
			return fmt.Errorf("%w: branch %q is checked out in worktree %q", ErrBranchInOtherWorktree, branch, owner.Path)
		}
		return rebaseGit.RebaseCurrentBranchOnto(ontoRef)
	}
	return rebaseGit.RebaseBranchOnto(branch, ontoRef)
}
