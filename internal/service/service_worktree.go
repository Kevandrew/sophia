package service

import (
	"fmt"
	"path/filepath"
	"sophia/internal/gitx"
	"sophia/internal/model"
	"strings"
)

type CRWhereView struct {
	CRID                      int
	CRUID                     string
	Title                     string
	Branch                    string
	CurrentWorktreePath       string
	OwnerWorktreePath         string
	OwnerIsCurrentWorktree    bool
	CheckedOutInOtherWorktree bool
	SuggestedCommand          string
}

type branchWorktreeContext struct {
	CurrentWorktreePath       string
	OwnerWorktreePath         string
	OwnerIsCurrentWorktree    bool
	CheckedOutInOtherWorktree bool
	SuggestedCommand          string
}

func (s *Service) branchOwnerWorktree(branch string) (*gitx.Worktree, error) {
	mergeGit := s.activeMergeGitProvider()
	branch = strings.TrimSpace(branch)
	if branch == "" {
		return nil, nil
	}
	return mergeGit.WorktreeForBranch(branch)
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

func (s *Service) gitClientForBranch(branch string) (mergeRuntimeGit, *gitx.Worktree, error) {
	owner, err := s.branchOwnerWorktree(branch)
	if err != nil {
		return nil, nil, err
	}
	if owner != nil && !s.isCurrentWorktreePath(owner.Path) {
		return s.activeMergeGitFactory()(owner.Path), owner, nil
	}
	return s.activeMergeGitProvider(), owner, nil
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
			return s.newBranchInOtherWorktreeError(0, branch, owner.Path, "rebase_branch", "")
		}
		return rebaseGit.RebaseCurrentBranchOnto(ontoRef)
	}
	return rebaseGit.RebaseBranchOnto(branch, ontoRef)
}

func (s *Service) WhereCR(id int) (*CRWhereView, error) {
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
	worktree, err := s.resolveBranchWorktreeContext(cr.ID, cr.Branch, fmt.Sprintf("sophia cr switch %d", cr.ID))
	if err != nil {
		return nil, err
	}
	return &CRWhereView{
		CRID:                      cr.ID,
		CRUID:                     strings.TrimSpace(cr.UID),
		Title:                     strings.TrimSpace(cr.Title),
		Branch:                    strings.TrimSpace(cr.Branch),
		CurrentWorktreePath:       worktree.CurrentWorktreePath,
		OwnerWorktreePath:         worktree.OwnerWorktreePath,
		OwnerIsCurrentWorktree:    worktree.OwnerIsCurrentWorktree,
		CheckedOutInOtherWorktree: worktree.CheckedOutInOtherWorktree,
		SuggestedCommand:          worktree.SuggestedCommand,
	}, nil
}

func (s *Service) resolveBranchWorktreeContext(crID int, branch, command string) (*branchWorktreeContext, error) {
	branch = strings.TrimSpace(branch)
	command = strings.TrimSpace(command)
	if command == "" {
		if crID > 0 {
			command = fmt.Sprintf("sophia cr switch %d", crID)
		} else if branch != "" {
			command = branchResolveCommand(branch)
		}
	}
	currentPath := strings.TrimSpace(s.git.WorkDir)
	owner, err := s.branchOwnerWorktree(branch)
	if err != nil {
		return nil, err
	}
	ownerPath := ""
	ownerIsCurrent := false
	if owner != nil {
		ownerPath = strings.TrimSpace(owner.Path)
		ownerIsCurrent = s.isCurrentWorktreePath(owner.Path)
	}
	checkedOutInOtherWorktree := ownerPath != "" && !ownerIsCurrent
	return &branchWorktreeContext{
		CurrentWorktreePath:       currentPath,
		OwnerWorktreePath:         ownerPath,
		OwnerIsCurrentWorktree:    ownerIsCurrent,
		CheckedOutInOtherWorktree: checkedOutInOtherWorktree,
		SuggestedCommand:          withWorktreePathPrefix(ownerPath, command),
	}, nil
}

func (s *Service) newBranchInOtherWorktreeError(crID int, branch, ownerPath, operation, command string) error {
	branch = strings.TrimSpace(branch)
	ownerPath = strings.TrimSpace(ownerPath)
	if crID <= 0 && branch != "" {
		if resolvedCRID, err := s.ResolveCRID(branch); err == nil && resolvedCRID > 0 {
			crID = resolvedCRID
		}
	}
	command = strings.TrimSpace(command)
	if command == "" {
		if crID > 0 {
			command = fmt.Sprintf("sophia cr switch %d", crID)
		} else if branch != "" {
			command = branchResolveCommand(branch)
		}
	}
	return &BranchInOtherWorktreeError{
		CRID:                crID,
		Branch:              branch,
		OwnerWorktreePath:   ownerPath,
		CurrentWorktreePath: strings.TrimSpace(s.git.WorkDir),
		Operation:           strings.TrimSpace(operation),
		SuggestedCommand:    withWorktreePathPrefix(ownerPath, command),
	}
}

func withWorktreePathPrefix(worktreePath, command string) string {
	worktreePath = strings.TrimSpace(worktreePath)
	command = strings.TrimSpace(command)
	if command == "" {
		return ""
	}
	if worktreePath == "" {
		return command
	}
	return fmt.Sprintf("cd %s && %s", shellQuote(worktreePath), command)
}

func branchResolveCommand(branch string) string {
	branch = strings.TrimSpace(branch)
	if branch == "" {
		return "sophia cr branch resolve"
	}
	return "sophia cr branch resolve --branch " + shellQuote(branch)
}

func shellQuote(value string) string {
	if value == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(value, "'", `'"'"'`) + "'"
}

func (s *Service) effectiveMergeGitForCR(cr *model.CR) (mergeRuntimeGit, string, error) {
	if cr == nil {
		return nil, "", fmt.Errorf("cr is required")
	}
	mergeGit := s.activeMergeGitProvider()
	worktreePath := strings.TrimSpace(s.git.WorkDir)
	targetRef := s.mergeTargetRef(cr)
	baseOwner, err := s.branchOwnerWorktree(targetRef)
	if err != nil {
		return nil, "", err
	}
	if baseOwner != nil && !s.isCurrentWorktreePath(baseOwner.Path) {
		mergeGit = s.activeMergeGitFactory()(baseOwner.Path)
		worktreePath = strings.TrimSpace(baseOwner.Path)
	}
	if strings.TrimSpace(worktreePath) == "" {
		worktreePath = strings.TrimSpace(s.git.WorkDir)
	}
	return mergeGit, worktreePath, nil
}

func (s *Service) mergeTargetRef(cr *model.CR) string {
	if cr == nil {
		return ""
	}
	return strings.TrimSpace(nonEmptyTrimmed(cr.BaseRef, cr.BaseBranch))
}
