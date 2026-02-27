package service

import (
	"fmt"
	"strings"
)

func (e *MergeConflictError) Error() string {
	if e == nil {
		return ErrMergeConflict.Error()
	}
	conflictCount := len(e.ConflictFiles)
	summary := fmt.Sprintf("merge conflict while merging %s into %s", nonEmptyTrimmed(e.CRBranch, "(unknown branch)"), nonEmptyTrimmed(e.BaseBranch, "(unknown base)"))
	if conflictCount > 0 {
		summary = fmt.Sprintf("%s (%d conflicted file(s))", summary, conflictCount)
	}
	if e.Cause == nil {
		return summary
	}
	return fmt.Sprintf("%s: %v", summary, e.Cause)
}

func (e *MergeConflictError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

func (e *MergeConflictError) Is(target error) bool {
	return target == ErrMergeConflict
}

func (e *MergeConflictError) Details() map[string]any {
	if e == nil {
		return map[string]any{}
	}
	return map[string]any{
		"cr_id":          e.CRID,
		"base_branch":    strings.TrimSpace(e.BaseBranch),
		"cr_branch":      strings.TrimSpace(e.CRBranch),
		"worktree_path":  strings.TrimSpace(e.WorktreePath),
		"conflict_files": append([]string(nil), e.ConflictFiles...),
	}
}

func (e *MergeInProgressError) Error() string {
	if e == nil {
		return ErrMergeInProgress.Error()
	}
	if strings.TrimSpace(e.Summary) != "" {
		return e.Summary
	}
	if len(e.ConflictFiles) > 0 {
		return fmt.Sprintf("merge in progress at %s with %d conflicted file(s)", nonEmptyTrimmed(e.WorktreePath, "(unknown worktree)"), len(e.ConflictFiles))
	}
	return fmt.Sprintf("merge in progress at %s", nonEmptyTrimmed(e.WorktreePath, "(unknown worktree)"))
}

func (e *MergeInProgressError) Is(target error) bool {
	return target == ErrMergeInProgress
}

func (e *MergeInProgressError) Details() map[string]any {
	if e == nil {
		return map[string]any{}
	}
	return map[string]any{
		"worktree_path":  strings.TrimSpace(e.WorktreePath),
		"conflict_files": append([]string(nil), e.ConflictFiles...),
	}
}

func (e *NoMergeInProgressError) Error() string {
	if e == nil {
		return ErrNoMergeInProgress.Error()
	}
	if strings.TrimSpace(e.Summary) != "" {
		return e.Summary
	}
	return fmt.Sprintf("no merge in progress at %s", nonEmptyTrimmed(e.WorktreePath, "(unknown worktree)"))
}

func (e *NoMergeInProgressError) Is(target error) bool {
	return target == ErrNoMergeInProgress
}

func (e *NoMergeInProgressError) Details() map[string]any {
	if e == nil {
		return map[string]any{}
	}
	return map[string]any{
		"worktree_path": strings.TrimSpace(e.WorktreePath),
	}
}
