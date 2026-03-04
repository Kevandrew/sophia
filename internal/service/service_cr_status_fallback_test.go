package service

import (
	"errors"
	"strings"
	"testing"
)

func TestStatusCRBranchContextUnavailableReturnsValidationBlockers(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	cr, err := svc.AddCR("Status fallback", "missing branch context should not hard-fail")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}

	runGit(t, dir, "checkout", "main")
	runGit(t, dir, "branch", "-D", cr.Branch)

	status, err := svc.StatusCR(cr.ID)
	if err != nil {
		t.Fatalf("StatusCR() error = %v", err)
	}
	if status.ValidationValid {
		t.Fatalf("expected invalid status when branch context is unavailable")
	}
	if status.ValidationErrors == 0 {
		t.Fatalf("expected validation errors in fallback mode")
	}
	if !status.MergeBlocked {
		t.Fatalf("expected merge blocked in fallback mode")
	}
	if strings.TrimSpace(status.OwnerWorktreePath) != "" {
		t.Fatalf("expected empty owner worktree path when branch context is unavailable, got %q", status.OwnerWorktreePath)
	}
	if status.CheckedOutInOtherWorktree {
		t.Fatalf("expected checked_out_in_other_worktree=false without owner worktree")
	}

	foundValidationBlocker := false
	foundBranchUnavailable := false
	for _, blocker := range status.MergeBlockers {
		if strings.HasPrefix(blocker, "validation: ") {
			foundValidationBlocker = true
		}
		if strings.Contains(blocker, "branch context is unavailable") {
			foundBranchUnavailable = true
		}
	}
	if !foundValidationBlocker {
		t.Fatalf("expected at least one validation-prefixed merge blocker, got %#v", status.MergeBlockers)
	}
	if !foundBranchUnavailable {
		t.Fatalf("expected branch-context-unavailable merge blocker, got %#v", status.MergeBlockers)
	}
}

func TestStatusCRIgnoresWorktreeResolutionErrors(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	cr, err := svc.AddCR("Status worktree fallback", "worktree metadata should be best-effort")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}

	mergeGit := newFakeMergeGit("Test User <test@example.com>", cr.Branch)
	mergeGit.worktreeErr[cr.Branch] = errors.New("worktree list unavailable")
	svc.overrideMergeRuntimeProvidersForTests(mergeGit, nil, nil)

	status, err := svc.StatusCR(cr.ID)
	if err != nil {
		t.Fatalf("StatusCR() error = %v", err)
	}
	if strings.TrimSpace(status.OwnerWorktreePath) != "" {
		t.Fatalf("expected empty owner worktree path on worktree resolution error, got %q", status.OwnerWorktreePath)
	}
	if status.OwnerIsCurrentWorktree {
		t.Fatalf("expected owner_is_current_worktree=false on worktree resolution error")
	}
	if status.CheckedOutInOtherWorktree {
		t.Fatalf("expected checked_out_in_other_worktree=false on worktree resolution error")
	}
}
