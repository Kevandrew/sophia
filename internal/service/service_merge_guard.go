package service

import (
	"fmt"
	"sophia/internal/model"
	"strings"
)

func (s *Service) ensureNoMergeInProgressInCurrentWorktree() error {
	return s.ensureNoMergeInProgressForGit(s.activeMergeGitProvider(), strings.TrimSpace(s.git.WorkDir), 0)
}

func (s *Service) ensureNoMergeInProgressForCR(cr *model.CR) error {
	if cr == nil {
		return nil
	}
	mergeGit, worktreePath, err := s.effectiveMergeGitForCR(cr)
	if err != nil {
		return err
	}
	return s.ensureNoMergeInProgressForGit(mergeGit, worktreePath, cr.ID)
}

func (s *Service) ensureNoMergeInProgressForGit(gitClient mergeRuntimeGit, worktreePath string, crID int) error {
	if gitClient == nil {
		return nil
	}
	inProgress, err := gitClient.IsMergeInProgress()
	if err != nil {
		return err
	}
	if !inProgress {
		return nil
	}
	conflictFiles, conflictErr := gitClient.MergeConflictFiles()
	if conflictErr != nil {
		conflictFiles = []string{}
	}
	resolvedPath := strings.TrimSpace(worktreePath)
	if resolvedPath == "" {
		resolvedPath = "(unknown worktree)"
	}
	summary := fmt.Sprintf("%s: unresolved merge detected in %s", ErrMergeInProgress, nonEmptyTrimmed(resolvedPath, "(unknown worktree)"))
	if crID > 0 {
		summary = fmt.Sprintf("%s; run sophia cr merge status %d, then sophia cr merge abort %d or resolve conflicts and run sophia cr merge resume %d", summary, crID, crID, crID)
	}
	return &MergeInProgressError{
		WorktreePath:  resolvedPath,
		ConflictFiles: conflictFiles,
		Summary:       summary,
	}
}
