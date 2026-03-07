package service

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRefreshCRParentCascadeRelinksChildFromBaseRefInRealRepo(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")

	parent, err := svc.AddCR("Parent refresh", "real refresh cascade")
	if err != nil {
		t.Fatalf("AddCR(parent) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "parent.txt"), []byte("parent\n"), 0o644); err != nil {
		t.Fatalf("write parent.txt: %v", err)
	}
	runGit(t, dir, "add", "parent.txt")
	runGit(t, dir, "commit", "-m", "feat: parent work")

	child, _, err := svc.AddCRWithOptionsWithWarnings("Child refresh", "real child refresh", AddCROptions{ParentCRID: parent.ID})
	if err != nil {
		t.Fatalf("AddCR(child) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "child.txt"), []byte("child\n"), 0o644); err != nil {
		t.Fatalf("write child.txt: %v", err)
	}
	runGit(t, dir, "add", "child.txt")
	runGit(t, dir, "commit", "-m", "feat: child work")

	loadedChild, err := svc.store.LoadCR(child.ID)
	if err != nil {
		t.Fatalf("LoadCR(child) error = %v", err)
	}
	loadedChild.ParentCRID = 0
	if err := svc.store.SaveCR(loadedChild); err != nil {
		t.Fatalf("SaveCR(clear parent) error = %v", err)
	}

	runGit(t, dir, "checkout", "main")
	if err := os.WriteFile(filepath.Join(dir, "main.txt"), []byte("main\n"), 0o644); err != nil {
		t.Fatalf("write main.txt: %v", err)
	}
	runGit(t, dir, "add", "main.txt")
	runGit(t, dir, "commit", "-m", "feat: advance main")

	view, err := svc.RefreshCR(parent.ID, RefreshOptions{})
	if err != nil {
		t.Fatalf("RefreshCR(parent) error = %v", err)
	}
	if !view.Applied || view.CascadeCount != 1 || len(view.Entries) != 2 {
		t.Fatalf("expected parent refresh plus one cascaded child, got %#v", view)
	}
	if view.Entries[1].CRID != child.ID || view.Entries[1].Strategy != RefreshStrategyRestack {
		t.Fatalf("expected cascaded child restack entry, got %#v", view.Entries[1])
	}

	reloadedChild, err := svc.store.LoadCR(child.ID)
	if err != nil {
		t.Fatalf("LoadCR(child after refresh) error = %v", err)
	}
	if reloadedChild.ParentCRID != parent.ID {
		t.Fatalf("expected child parent relinked to %d, got %#v", parent.ID, reloadedChild)
	}
	if reloadedChild.BaseRef != parent.Branch {
		t.Fatalf("expected child base_ref %q after cascade, got %#v", parent.Branch, reloadedChild)
	}
	parentHead := runGit(t, dir, "rev-parse", "--verify", parent.Branch)
	if reloadedChild.BaseCommit != parentHead {
		t.Fatalf("expected child base_commit %q, got %#v", parentHead, reloadedChild)
	}
	childHead := runGit(t, dir, "rev-parse", "--verify", child.Branch)
	if childHead == "" {
		t.Fatalf("expected child branch head after refresh")
	}
	if !gitRefContainsCommit(t, dir, child.Branch, parentHead) {
		t.Fatalf("expected child branch %q to contain refreshed parent head %q", child.Branch, parentHead)
	}
}
