package service

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sophia/internal/model"
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

func TestStatusCRMarksMovedBaseRefAsStale(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	cr, err := svc.AddCR("Status freshness", "base movement should surface refresh guidance")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}

	runGit(t, dir, "checkout", "main")
	if err := os.WriteFile(filepath.Join(dir, "freshness.txt"), []byte("fresh\n"), 0o644); err != nil {
		t.Fatalf("write freshness.txt: %v", err)
	}
	runGit(t, dir, "add", "freshness.txt")
	runGit(t, dir, "commit", "-m", "feat: move base ref")

	status, err := svc.StatusCR(cr.ID)
	if err != nil {
		t.Fatalf("StatusCR() error = %v", err)
	}
	if status.FreshnessState != "stale" {
		t.Fatalf("expected freshness_state=stale, got %q", status.FreshnessState)
	}
	if !strings.Contains(status.FreshnessReason, "moved") {
		t.Fatalf("expected freshness reason to mention moved base ref, got %q", status.FreshnessReason)
	}
	if len(status.FreshnessSuggestedCommands) != 1 || status.FreshnessSuggestedCommands[0] != "sophia cr refresh 1" {
		t.Fatalf("expected refresh suggestion, got %#v", status.FreshnessSuggestedCommands)
	}
}

func TestStatusCRUsesInferredParentageWhenStoredParentMissing(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	parent, err := svc.AddCR("Parent status", "parent remains the effective base owner")
	if err != nil {
		t.Fatalf("AddCR(parent) error = %v", err)
	}
	child, _, err := svc.AddCRWithOptionsWithWarnings("Child status", "child keeps base_ref to parent", AddCROptions{ParentCRID: parent.ID})
	if err != nil {
		t.Fatalf("AddCR(child) error = %v", err)
	}

	loadedChild, err := svc.store.LoadCR(child.ID)
	if err != nil {
		t.Fatalf("LoadCR(child) error = %v", err)
	}
	loadedChild.ParentCRID = 0
	if err := svc.store.SaveCR(loadedChild); err != nil {
		t.Fatalf("SaveCR(clear parent) error = %v", err)
	}

	status, err := svc.StatusCR(child.ID)
	if err != nil {
		t.Fatalf("StatusCR() error = %v", err)
	}
	if status.ParentCRID != parent.ID {
		t.Fatalf("expected inferred parent %d, got %#v", parent.ID, status)
	}
	if status.ParentStatus != model.StatusInProgress {
		t.Fatalf("expected parent status %q, got %#v", model.StatusInProgress, status)
	}
	foundParentBlocker := false
	for _, blocker := range status.MergeBlockers {
		if strings.Contains(blocker, "depends on parent CR") {
			foundParentBlocker = true
			break
		}
	}
	if !foundParentBlocker {
		t.Fatalf("expected parent dependency blocker, got %#v", status.MergeBlockers)
	}
}
