package service

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"sophia/internal/model"
)

func TestPROpenBlocksChildPRWhenBranchHasNoDiffFromParentBase(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")
	if err := os.WriteFile(filepath.Join(dir, "SOPHIA.yaml"), []byte("version: v1\nmerge:\n  mode: pr_gate\narchive:\n  enabled: false\n"), 0o644); err != nil {
		t.Fatalf("write SOPHIA.yaml: %v", err)
	}

	parent, err := svc.AddCR("Parent publish", "aggregate parent")
	if err != nil {
		t.Fatalf("AddCR(parent) error = %v", err)
	}
	child, _, err := svc.AddCRWithOptionsWithWarnings("Child publish", "already integrated", AddCROptions{ParentCRID: parent.ID})
	if err != nil {
		t.Fatalf("AddCR(child) error = %v", err)
	}

	err = nil
	_, err = svc.PROpen(child.ID, true)
	if !errors.Is(err, ErrPRNoDiffToBase) {
		t.Fatalf("expected ErrPRNoDiffToBase, got %v", err)
	}
	detailer, ok := err.(interface{ Details() map[string]any })
	if !ok {
		t.Fatalf("expected detailed action-required error")
	}
	details := detailer.Details()
	actionRequired, ok := details["action_required"].(map[string]any)
	if !ok {
		t.Fatalf("expected action_required details, got %#v", details)
	}
	if got, _ := actionRequired["name"].(string); got != "publish_aggregate_parent" {
		t.Fatalf("expected aggregate parent publish guidance, got %#v", actionRequired)
	}
	if got, _ := details["suggested_command"].(string); got != "sophia cr pr open 1 --approve-open" {
		t.Fatalf("expected parent publish suggestion, got %#v", details)
	}
}

func TestPROpenBlocksChildPRWhenBranchIsNotAheadOfParentBase(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")
	if err := os.WriteFile(filepath.Join(dir, "SOPHIA.yaml"), []byte("version: v1\nmerge:\n  mode: pr_gate\narchive:\n  enabled: false\n"), 0o644); err != nil {
		t.Fatalf("write SOPHIA.yaml: %v", err)
	}

	parent, err := svc.AddCR("Parent publish ahead", "aggregate parent")
	if err != nil {
		t.Fatalf("AddCR(parent) error = %v", err)
	}
	child, _, err := svc.AddCRWithOptionsWithWarnings("Child publish behind", "base moved independently", AddCROptions{ParentCRID: parent.ID})
	if err != nil {
		t.Fatalf("AddCR(child) error = %v", err)
	}

	runGit(t, dir, "checkout", parent.Branch)
	if err := os.WriteFile(filepath.Join(dir, "parent-only.txt"), []byte("parent ahead\n"), 0o644); err != nil {
		t.Fatalf("write parent-only.txt: %v", err)
	}
	runGit(t, dir, "add", "parent-only.txt")
	runGit(t, dir, "commit", "-m", "feat: advance parent base only")
	runGit(t, dir, "checkout", child.Branch)

	_, err = svc.PROpen(child.ID, true)
	if !errors.Is(err, ErrPRNoDiffToBase) {
		t.Fatalf("expected ErrPRNoDiffToBase for branch not ahead of base, got %v", err)
	}
}

func TestPROpenBlocksWhenBaseRefCannotBeResolvedLocally(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")
	if err := os.WriteFile(filepath.Join(dir, "SOPHIA.yaml"), []byte("version: v1\nmerge:\n  mode: pr_gate\narchive:\n  enabled: false\n"), 0o644); err != nil {
		t.Fatalf("write SOPHIA.yaml: %v", err)
	}

	parent, err := svc.AddCR("Parent missing base", "aggregate parent")
	if err != nil {
		t.Fatalf("AddCR(parent) error = %v", err)
	}
	child, _, err := svc.AddCRWithOptionsWithWarnings("Child missing base", "base ref deleted locally", AddCROptions{ParentCRID: parent.ID})
	if err != nil {
		t.Fatalf("AddCR(child) error = %v", err)
	}

	runGit(t, dir, "branch", "-D", parent.Branch)

	_, err = svc.PROpen(child.ID, true)
	var actionErr *PRActionRequiredError
	if !errors.As(err, &actionErr) {
		t.Fatalf("expected PRActionRequiredError, got %T %v", err, err)
	}
	details := actionErr.Details()
	actionRequired, ok := details["action_required"].(map[string]any)
	if !ok {
		t.Fatalf("expected action_required details, got %#v", details)
	}
	if got, _ := actionRequired["name"].(string); got != "refresh_cr_base" {
		t.Fatalf("expected refresh_cr_base guidance, got %#v", actionRequired)
	}
	if got, _ := details["suggested_command"].(string); got != "sophia cr refresh 2" {
		t.Fatalf("expected refresh suggestion for missing base, got %#v", details)
	}
}

func TestPRReconcileCreateBlocksWhenBranchIsNotAheadOfBase(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")
	if err := os.WriteFile(filepath.Join(dir, "SOPHIA.yaml"), []byte("version: v1\nmerge:\n  mode: pr_gate\narchive:\n  enabled: false\n"), 0o644); err != nil {
		t.Fatalf("write SOPHIA.yaml: %v", err)
	}

	parent, err := svc.AddCR("Parent reconcile ahead", "aggregate parent")
	if err != nil {
		t.Fatalf("AddCR(parent) error = %v", err)
	}
	child, _, err := svc.AddCRWithOptionsWithWarnings("Child reconcile behind", "base moved independently", AddCROptions{ParentCRID: parent.ID})
	if err != nil {
		t.Fatalf("AddCR(child) error = %v", err)
	}

	runGit(t, dir, "checkout", parent.Branch)
	if err := os.WriteFile(filepath.Join(dir, "parent-reconcile-only.txt"), []byte("parent ahead\n"), 0o644); err != nil {
		t.Fatalf("write parent-reconcile-only.txt: %v", err)
	}
	runGit(t, dir, "add", "parent-reconcile-only.txt")
	runGit(t, dir, "commit", "-m", "feat: advance parent base only for reconcile")
	runGit(t, dir, "checkout", child.Branch)

	_, err = svc.PRReconcile(child.ID, prReconcileModeCreate)
	if !errors.Is(err, ErrPRNoDiffToBase) {
		t.Fatalf("expected ErrPRNoDiffToBase for reconcile create, got %v", err)
	}
}

func TestPRReconcileCreateBlocksWhenBaseRefCannotBeResolvedLocally(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")
	if err := os.WriteFile(filepath.Join(dir, "SOPHIA.yaml"), []byte("version: v1\nmerge:\n  mode: pr_gate\narchive:\n  enabled: false\n"), 0o644); err != nil {
		t.Fatalf("write SOPHIA.yaml: %v", err)
	}

	parent, err := svc.AddCR("Parent reconcile missing base", "aggregate parent")
	if err != nil {
		t.Fatalf("AddCR(parent) error = %v", err)
	}
	child, _, err := svc.AddCRWithOptionsWithWarnings("Child reconcile missing base", "base ref deleted locally", AddCROptions{ParentCRID: parent.ID})
	if err != nil {
		t.Fatalf("AddCR(child) error = %v", err)
	}

	runGit(t, dir, "branch", "-D", parent.Branch)

	_, err = svc.PRReconcile(child.ID, prReconcileModeCreate)
	var actionErr *PRActionRequiredError
	if !errors.As(err, &actionErr) {
		t.Fatalf("expected PRActionRequiredError, got %T %v", err, err)
	}
	if got, _ := actionErr.Details()["suggested_command"].(string); got != "sophia cr refresh 2" {
		t.Fatalf("expected refresh guidance for reconcile create missing-base guard, got %#v", actionErr.Details())
	}
}

func TestPRReadyBlocksAggregateParentWithDelegatedChildrenPendingUsingStackGuidance(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SOPHIA.yaml"), []byte("version: v1\nmerge:\n  mode: pr_gate\narchive:\n  enabled: false\n"), 0o644); err != nil {
		t.Fatalf("write SOPHIA.yaml: %v", err)
	}

	cr, err := svc.AddCR("Aggregate parent blocked", "delegated children pending")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	loaded, err := svc.store.LoadCR(cr.ID)
	if err != nil {
		t.Fatalf("LoadCR() error = %v", err)
	}
	loaded.PR.Number = 71
	loaded.Subtasks = []model.Subtask{{
		ID:     1,
		Title:  "Delegated child work",
		Status: model.TaskStatusDelegated,
		Delegations: []model.TaskDelegation{
			{ChildCRID: 401, ChildTaskID: 1},
		},
	}}
	if err := svc.store.SaveCR(loaded); err != nil {
		t.Fatalf("SaveCR() error = %v", err)
	}

	_, err = svc.PRReady(cr.ID)
	blocked, ok := err.(*PRReadyBlockedError)
	if !ok {
		t.Fatalf("expected PRReadyBlockedError, got %T %v", err, err)
	}
	if blocked.ReasonCode != prReadyBlockedReasonDelegatedChildrenPending {
		t.Fatalf("expected delegated-children-pending reason, got %#v", blocked)
	}
	if len(blocked.SuggestedCommands) == 0 || blocked.SuggestedCommands[0] != "sophia cr stack 1" {
		t.Fatalf("expected stack-first guidance, got %#v", blocked.SuggestedCommands)
	}
}
