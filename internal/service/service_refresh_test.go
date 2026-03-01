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
