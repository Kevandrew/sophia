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

func (e *BranchInOtherWorktreeError) Error() string {
	if e == nil {
		return ErrBranchInOtherWorktree.Error()
	}
	branch := nonEmptyTrimmed(e.Branch, "(unknown branch)")
	owner := nonEmptyTrimmed(e.OwnerWorktreePath, "(unknown worktree)")
	summary := fmt.Sprintf("%s: branch %q is checked out in worktree %q", ErrBranchInOtherWorktree, branch, owner)
	if op := strings.TrimSpace(e.Operation); op != "" {
		summary = fmt.Sprintf("%s during %s", summary, op)
	}
	return summary
}

func (e *BranchInOtherWorktreeError) Is(target error) bool {
	return target == ErrBranchInOtherWorktree
}

func (e *BranchInOtherWorktreeError) Details() map[string]any {
	if e == nil {
		return map[string]any{}
	}
	out := map[string]any{
		"branch":                strings.TrimSpace(e.Branch),
		"owner_worktree_path":   strings.TrimSpace(e.OwnerWorktreePath),
		"current_worktree_path": strings.TrimSpace(e.CurrentWorktreePath),
		"operation":             strings.TrimSpace(e.Operation),
		"suggested_command":     strings.TrimSpace(e.SuggestedCommand),
	}
	if e.CRID > 0 {
		out["cr_id"] = e.CRID
	}
	return out
}

func (e *PRReadyBlockedError) Error() string {
	if e == nil {
		return "pr ready is blocked"
	}
	reason := strings.TrimSpace(e.Reason)
	if reason == "" {
		reason = "keep PR draft until implementation checkpoints exist"
	}
	return fmt.Sprintf("pr ready is blocked: %s", reason)
}

func (e *PRReadyBlockedError) Details() map[string]any {
	if e == nil {
		return map[string]any{}
	}
	reasonCode := strings.TrimSpace(e.ReasonCode)
	if reasonCode == "" {
		reasonCode = prReadyBlockedReasonNoCheckpoints
	}
	reason := strings.TrimSpace(e.Reason)
	if reason == "" {
		reason = "CR has no task checkpoint commits yet; keep PR draft until implementation checkpoints exist."
	}
	suggestedCommands := cleanAndDedupeStrings(e.SuggestedCommands)
	return map[string]any{
		"cr_id":              e.CRID,
		"reason_code":        reasonCode,
		"reason":             reason,
		"suggested_commands": suggestedCommands,
		"action_required": map[string]any{
			"type":               "manual",
			"name":               prActionReadyBlocked,
			"reason_code":        reasonCode,
			"reason":             reason,
			"suggested_commands": suggestedCommands,
		},
	}
}
