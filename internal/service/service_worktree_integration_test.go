package service

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWorktreeSharedLocalMetadataAndCRIDSequence(t *testing.T) {
	dir := t.TempDir()
	svcMain := New(dir)
	if _, err := svcMain.Init("main", ""); err != nil {
		t.Fatalf("Init(main) error = %v", err)
	}
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")

	first, err := svcMain.AddCR("Main CR", "from main worktree")
	if err != nil {
		t.Fatalf("AddCR(main) error = %v", err)
	}
	if first.ID != 1 {
		t.Fatalf("expected first CR id 1, got %d", first.ID)
	}

	runGit(t, dir, "checkout", "main")
	wtDir := filepath.Join(t.TempDir(), "wt2")
	runGit(t, dir, "worktree", "add", wtDir, "-b", "feature/wt2", "main")

	svcWT := New(wtDir)
	crs, err := svcWT.ListCRs()
	if err != nil {
		t.Fatalf("ListCRs(worktree) error = %v", err)
	}
	if len(crs) != 1 || crs[0].ID != 1 {
		t.Fatalf("expected shared metadata to include CR 1, got %#v", crs)
	}

	second, err := svcWT.AddCR("WT CR", "from secondary worktree")
	if err != nil {
		t.Fatalf("AddCR(worktree) error = %v", err)
	}
	if second.ID != 2 {
		t.Fatalf("expected second CR id 2, got %d", second.ID)
	}
}

func TestInitInSecondaryWorktreeDoesNotRequireBaseCheckout(t *testing.T) {
	dir := t.TempDir()
	runGit(t, dir, "init", "-b", "main")
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")
	if err := os.WriteFile(filepath.Join(dir, "seed.txt"), []byte("seed\n"), 0o644); err != nil {
		t.Fatalf("write seed: %v", err)
	}
	runGit(t, dir, "add", "seed.txt")
	runGit(t, dir, "commit", "-m", "seed")

	wtDir := filepath.Join(t.TempDir(), "wt-init")
	runGit(t, dir, "worktree", "add", wtDir, "-b", "feature/wt-init", "main")

	svcWT := New(wtDir)
	if _, err := svcWT.Init("main", ""); err != nil {
		t.Fatalf("Init(worktree, base=main) error = %v", err)
	}
}

func TestSwitchCRFailsWithBranchOwnerPathWhenCheckedOutElsewhere(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")

	cr, err := svc.AddCR("Switch ownership", "ownership test")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	runGit(t, dir, "checkout", "main")

	wtDir := filepath.Join(t.TempDir(), "wt-switch")
	runGit(t, dir, "worktree", "add", wtDir, cr.Branch)

	_, err = svc.SwitchCR(cr.ID)
	if err == nil || !errors.Is(err, ErrBranchInOtherWorktree) {
		t.Fatalf("expected ErrBranchInOtherWorktree, got %v", err)
	}
	if !strings.Contains(err.Error(), wtDir) {
		t.Fatalf("expected owner worktree path in error, got %v", err)
	}
}

func TestMergeCRUsesBaseOwnerWorktreeAndWarnsWhenCRBranchOwnedElsewhere(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")

	cr, err := svc.AddCR("Merge ownership", "merge ownership test")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "merge.txt"), []byte("x\n"), 0o644); err != nil {
		t.Fatalf("write merge.txt: %v", err)
	}
	runGit(t, dir, "add", "merge.txt")
	runGit(t, dir, "commit", "-m", "feat: merge ownership")
	setValidContract(t, svc, cr.ID)

	baseWT := filepath.Join(t.TempDir(), "wt-main-owner")
	runGit(t, dir, "worktree", "add", baseWT, "main")

	sha, warnings, err := svc.MergeCRWithWarnings(cr.ID, false, "")
	if err != nil {
		t.Fatalf("MergeCRWithWarnings() error = %v", err)
	}
	if strings.TrimSpace(sha) == "" {
		t.Fatalf("expected non-empty merge sha")
	}
	if len(warnings) == 0 {
		t.Fatalf("expected keep-branch warning when CR branch is checked out elsewhere")
	}

	merged, loadErr := svc.store.LoadCR(cr.ID)
	if loadErr != nil {
		t.Fatalf("LoadCR(merged) error = %v", loadErr)
	}
	if merged.Status != "merged" {
		t.Fatalf("expected merged status, got %q", merged.Status)
	}
}
