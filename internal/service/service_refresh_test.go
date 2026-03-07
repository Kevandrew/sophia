package service

import (
	"testing"

	"sophia/internal/model"
)

// fake-eligible: refresh auto-rebase selection is lifecycle orchestration logic.
func TestRefreshCRAutoRebaseForRootCR(t *testing.T) {
	t.Parallel()
	root := seedCR(1, "Refresh root", seedCROptions{
		Branch:     "cr-refresh-root",
		BaseBranch: "main",
		BaseRef:    "main",
		BaseCommit: "main-old",
	})
	h := harnessService(t, runtimeHarnessOptions{
		Branch: "cr-refresh-root",
		CRs:    []*model.CR{root},
	})
	h.LifecycleGit.SeedBranch(root.Branch, true)
	h.LifecycleGit.SeedResolve(root.Branch, "root-head-before")
	h.LifecycleGit.SeedResolve("main", "main-head-new")

	dryRun, err := h.Service.RefreshCR(root.ID, RefreshOptions{DryRun: true})
	if err != nil {
		t.Fatalf("RefreshCR(dry-run) error = %v", err)
	}
	if dryRun.Strategy != RefreshStrategyRebase {
		t.Fatalf("expected auto strategy rebase, got %#v", dryRun)
	}
	if dryRun.Applied {
		t.Fatalf("expected dry-run applied=false, got %#v", dryRun)
	}

	view, err := h.Service.RefreshCR(root.ID, RefreshOptions{})
	if err != nil {
		t.Fatalf("RefreshCR() error = %v", err)
	}
	if !view.Applied || view.Strategy != RefreshStrategyRebase {
		t.Fatalf("expected applied rebase, got %#v", view)
	}
	if h.MergeGit.Calls("RebaseBranchOnto") != 1 {
		t.Fatalf("expected one rebase call, got %d", h.MergeGit.Calls("RebaseBranchOnto"))
	}
	updated, err := h.Store.LoadCR(root.ID)
	if err != nil {
		t.Fatalf("LoadCR() error = %v", err)
	}
	if updated.BaseRef != "main" || updated.BaseCommit != "main-head-new" {
		t.Fatalf("expected refreshed base fields, got %#v", updated)
	}
}

// fake-eligible: child refresh auto-restack behavior is validated via runtime fakes.
func TestRefreshCRAutoRestackForChildCR(t *testing.T) {
	t.Parallel()
	parent := seedCR(1, "Parent refresh", seedCROptions{
		Branch:     "cr-parent-refresh",
		BaseBranch: "main",
		BaseRef:    "main",
		BaseCommit: "main-head-old",
	})
	child := seedCR(2, "Child refresh", seedCROptions{
		Branch:     "cr-child-refresh",
		BaseBranch: "main",
		BaseRef:    parent.Branch,
		BaseCommit: "parent-head-old",
		ParentCRID: parent.ID,
	})
	h := harnessService(t, runtimeHarnessOptions{
		Branch: "cr-child-refresh",
		CRs:    []*model.CR{parent, child},
	})
	h.LifecycleGit.SeedBranch(parent.Branch, true)
	h.LifecycleGit.SeedBranch(child.Branch, true)
	h.LifecycleGit.SeedResolve(parent.Branch, "parent-head-new")
	h.LifecycleGit.SeedResolve(child.Branch, "child-head-before")

	dryRun, err := h.Service.RefreshCR(child.ID, RefreshOptions{DryRun: true})
	if err != nil {
		t.Fatalf("RefreshCR(child dry-run) error = %v", err)
	}
	if dryRun.Strategy != RefreshStrategyRestack {
		t.Fatalf("expected child auto strategy restack, got %#v", dryRun)
	}
	if !containsString(dryRun.Warnings, "auto-selected strategy: restack") {
		t.Fatalf("expected auto strategy warning, got %#v", dryRun.Warnings)
	}
	view, err := h.Service.RefreshCR(child.ID, RefreshOptions{})
	if err != nil {
		t.Fatalf("RefreshCR(child) error = %v", err)
	}
	if !view.Applied || view.Strategy != RefreshStrategyRestack {
		t.Fatalf("expected applied restack, got %#v", view)
	}
	if h.MergeGit.Calls("RebaseBranchOnto") != 1 {
		t.Fatalf("expected one restack rebase, got %d", h.MergeGit.Calls("RebaseBranchOnto"))
	}
	updated, err := h.Store.LoadCR(child.ID)
	if err != nil {
		t.Fatalf("LoadCR(child) error = %v", err)
	}
	if updated.BaseRef != parent.Branch || updated.BaseCommit != "parent-head-new" {
		t.Fatalf("expected child restacked onto parent, got %#v", updated)
	}
}

// fake-eligible: root parent refresh orchestration only; descendant order and target selection come from runtime fakes.
func TestRefreshCRParentCascadeRefreshesDescendants(t *testing.T) {
	t.Parallel()
	root := seedCR(1, "Refresh root parent", seedCROptions{
		Branch:     "cr-refresh-root-parent",
		BaseBranch: "main",
		BaseRef:    "main",
		BaseCommit: "main-head-old",
	})
	child := seedCR(2, "Refresh child", seedCROptions{
		Branch:     "cr-refresh-child",
		BaseBranch: "main",
		BaseRef:    root.Branch,
		BaseCommit: "root-head-old",
		ParentCRID: root.ID,
	})
	grandchild := seedCR(3, "Refresh grandchild", seedCROptions{
		Branch:     "cr-refresh-grandchild",
		BaseBranch: "main",
		BaseRef:    child.Branch,
		BaseCommit: "child-head-old",
		ParentCRID: child.ID,
	})
	h := harnessService(t, runtimeHarnessOptions{
		Branch: root.Branch,
		CRs:    []*model.CR{root, child, grandchild},
	})
	h.LifecycleGit.SeedBranch(root.Branch, true)
	h.LifecycleGit.SeedBranch(child.Branch, true)
	h.LifecycleGit.SeedBranch(grandchild.Branch, true)
	h.LifecycleGit.SeedResolve(root.Branch, "root-head-new")
	h.LifecycleGit.SeedResolve(child.Branch, "child-head-new")
	h.LifecycleGit.SeedResolve(grandchild.Branch, "grandchild-head-before")
	h.LifecycleGit.SeedResolve("main", "main-head-new")

	dryRun, err := h.Service.RefreshCR(root.ID, RefreshOptions{DryRun: true})
	if err != nil {
		t.Fatalf("RefreshCR(parent dry-run) error = %v", err)
	}
	if dryRun.CascadeCount != 2 {
		t.Fatalf("expected cascade_count=2, got %#v", dryRun.CascadeCount)
	}
	if len(dryRun.Entries) != 3 {
		t.Fatalf("expected three refresh entries, got %#v", dryRun.Entries)
	}
	if dryRun.Entries[1].CRID != child.ID || dryRun.Entries[1].Strategy != RefreshStrategyRestack {
		t.Fatalf("expected child restack entry, got %#v", dryRun.Entries[1])
	}
	if dryRun.Entries[2].CRID != grandchild.ID || dryRun.Entries[2].Strategy != RefreshStrategyRestack {
		t.Fatalf("expected grandchild restack entry, got %#v", dryRun.Entries[2])
	}

	view, err := h.Service.RefreshCR(root.ID, RefreshOptions{})
	if err != nil {
		t.Fatalf("RefreshCR(parent) error = %v", err)
	}
	if !view.Applied || view.CascadeCount != 2 {
		t.Fatalf("expected applied parent cascade, got %#v", view)
	}
	if h.MergeGit.Calls("RebaseBranchOnto") != 3 {
		t.Fatalf("expected three rebases for parent cascade, got %d", h.MergeGit.Calls("RebaseBranchOnto"))
	}

	updatedRoot, err := h.Store.LoadCR(root.ID)
	if err != nil {
		t.Fatalf("LoadCR(root) error = %v", err)
	}
	if updatedRoot.BaseRef != "main" || updatedRoot.BaseCommit != "main-head-new" {
		t.Fatalf("expected root rebased onto main, got %#v", updatedRoot)
	}
	updatedChild, err := h.Store.LoadCR(child.ID)
	if err != nil {
		t.Fatalf("LoadCR(child) error = %v", err)
	}
	if updatedChild.BaseRef != root.Branch || updatedChild.BaseCommit != "root-head-new" {
		t.Fatalf("expected child restacked onto refreshed root, got %#v", updatedChild)
	}
	updatedGrandchild, err := h.Store.LoadCR(grandchild.ID)
	if err != nil {
		t.Fatalf("LoadCR(grandchild) error = %v", err)
	}
	if updatedGrandchild.BaseRef != child.Branch || updatedGrandchild.BaseCommit != "child-head-new" {
		t.Fatalf("expected grandchild restacked onto refreshed child, got %#v", updatedGrandchild)
	}
}

// fake-eligible: child refresh scope only; ensures no sibling or ancestor cascade is introduced.
func TestRefreshCRChildRemainsLocalToThatChild(t *testing.T) {
	t.Parallel()
	parent := seedCR(1, "Parent refresh", seedCROptions{
		Branch:     "cr-parent-refresh-locality",
		BaseBranch: "main",
		BaseRef:    "main",
		BaseCommit: "main-head-old",
	})
	child := seedCR(2, "Target child refresh", seedCROptions{
		Branch:     "cr-target-child-refresh",
		BaseBranch: "main",
		BaseRef:    parent.Branch,
		BaseCommit: "parent-head-old",
		ParentCRID: parent.ID,
	})
	sibling := seedCR(3, "Sibling refresh", seedCROptions{
		Branch:     "cr-sibling-refresh",
		BaseBranch: "main",
		BaseRef:    parent.Branch,
		BaseCommit: "parent-head-old",
		ParentCRID: parent.ID,
	})
	h := harnessService(t, runtimeHarnessOptions{
		Branch: child.Branch,
		CRs:    []*model.CR{parent, child, sibling},
	})
	h.LifecycleGit.SeedBranch(parent.Branch, true)
	h.LifecycleGit.SeedBranch(child.Branch, true)
	h.LifecycleGit.SeedBranch(sibling.Branch, true)
	h.LifecycleGit.SeedResolve(parent.Branch, "parent-head-new")
	h.LifecycleGit.SeedResolve(child.Branch, "child-head-before")
	h.LifecycleGit.SeedResolve(sibling.Branch, "sibling-head-before")

	view, err := h.Service.RefreshCR(child.ID, RefreshOptions{})
	if err != nil {
		t.Fatalf("RefreshCR(child) error = %v", err)
	}
	if !view.Applied || view.CascadeCount != 0 || len(view.Entries) != 1 {
		t.Fatalf("expected local child refresh only, got %#v", view)
	}
	if h.MergeGit.Calls("RebaseBranchOnto") != 1 {
		t.Fatalf("expected one local child rebase, got %d", h.MergeGit.Calls("RebaseBranchOnto"))
	}
	updatedSibling, err := h.Store.LoadCR(sibling.ID)
	if err != nil {
		t.Fatalf("LoadCR(sibling) error = %v", err)
	}
	if updatedSibling.BaseRef != parent.Branch || updatedSibling.BaseCommit != "parent-head-old" {
		t.Fatalf("expected sibling to remain untouched, got %#v", updatedSibling)
	}
}
