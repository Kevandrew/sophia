package service

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRefreshCRAutoRebaseForRootCR(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")

	cr, err := svc.AddCR("Refresh root", "auto strategy should rebase")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "root-refresh.txt"), []byte("root\n"), 0o644); err != nil {
		t.Fatalf("write root-refresh.txt: %v", err)
	}
	runGit(t, dir, "add", "root-refresh.txt")
	runGit(t, dir, "commit", "-m", "feat: root refresh change")

	if err := svc.git.CheckoutBranch("main"); err != nil {
		t.Fatalf("CheckoutBranch(main) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main-refresh.txt"), []byte("main\n"), 0o644); err != nil {
		t.Fatalf("write main-refresh.txt: %v", err)
	}
	runGit(t, dir, "add", "main-refresh.txt")
	runGit(t, dir, "commit", "-m", "feat: main refresh anchor")

	dryRun, err := svc.RefreshCR(cr.ID, RefreshOptions{DryRun: true})
	if err != nil {
		t.Fatalf("RefreshCR(dry-run) error = %v", err)
	}
	if dryRun.Strategy != RefreshStrategyRebase {
		t.Fatalf("expected auto strategy rebase, got %#v", dryRun)
	}
	if dryRun.Applied {
		t.Fatalf("expected dry-run applied=false, got %#v", dryRun)
	}

	view, err := svc.RefreshCR(cr.ID, RefreshOptions{})
	if err != nil {
		t.Fatalf("RefreshCR() error = %v", err)
	}
	if !view.Applied || view.Strategy != RefreshStrategyRebase {
		t.Fatalf("expected applied rebase, got %#v", view)
	}
}

func TestRefreshCRAutoRestackForChildCR(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")

	parent, err := svc.AddCR("Parent refresh", "parent")
	if err != nil {
		t.Fatalf("AddCR(parent) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "parent-refresh.txt"), []byte("p1\n"), 0o644); err != nil {
		t.Fatalf("write parent file: %v", err)
	}
	runGit(t, dir, "add", "parent-refresh.txt")
	runGit(t, dir, "commit", "-m", "feat: parent refresh 1")

	child, _, err := svc.AddCRWithOptionsWithWarnings("Child refresh", "child", AddCROptions{ParentCRID: parent.ID, Switch: true})
	if err != nil {
		t.Fatalf("AddCR(child) error = %v", err)
	}
	if err := svc.git.CheckoutBranch(parent.Branch); err != nil {
		t.Fatalf("CheckoutBranch(parent) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "parent-refresh-2.txt"), []byte("p2\n"), 0o644); err != nil {
		t.Fatalf("write parent second file: %v", err)
	}
	runGit(t, dir, "add", "parent-refresh-2.txt")
	runGit(t, dir, "commit", "-m", "feat: parent refresh 2")
	if err := svc.git.CheckoutBranch("main"); err != nil {
		t.Fatalf("CheckoutBranch(main) error = %v", err)
	}

	dryRun, err := svc.RefreshCR(child.ID, RefreshOptions{DryRun: true})
	if err != nil {
		t.Fatalf("RefreshCR(child dry-run) error = %v", err)
	}
	if dryRun.Strategy != RefreshStrategyRestack {
		t.Fatalf("expected child auto strategy restack, got %#v", dryRun)
	}
	if !containsString(dryRun.Warnings, "auto-selected strategy: restack") {
		t.Fatalf("expected auto strategy warning, got %#v", dryRun.Warnings)
	}
}
