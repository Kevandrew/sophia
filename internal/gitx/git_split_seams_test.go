package gitx

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSplitSeamsComposeAcrossConcernFiles(t *testing.T) {
	dir := t.TempDir()
	runGit(t, dir, "init", "-b", "main")
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")

	if err := os.WriteFile(filepath.Join(dir, "split.txt"), []byte("base\n"), 0o644); err != nil {
		t.Fatalf("write split.txt: %v", err)
	}
	runGit(t, dir, "add", "split.txt")
	runGit(t, dir, "commit", "-m", "feat: seed split test")

	client := New(dir)
	if _, err := client.RepoRoot(); err != nil {
		t.Fatalf("RepoRoot() error = %v", err)
	}
	if _, err := client.GitCommonDirAbs(); err != nil {
		t.Fatalf("GitCommonDirAbs() error = %v", err)
	}
	if _, err := client.WorktreeForBranch("main"); err != nil {
		t.Fatalf("WorktreeForBranch(main) error = %v", err)
	}

	runGit(t, dir, "checkout", "-b", "feature/split-seams")
	if err := os.WriteFile(filepath.Join(dir, "split.txt"), []byte("feature\n"), 0o644); err != nil {
		t.Fatalf("write split.txt on feature: %v", err)
	}
	runGit(t, dir, "add", "split.txt")
	runGit(t, dir, "commit", "-m", "feat: feature split change")

	files, err := client.DiffNames("main", "feature/split-seams")
	if err != nil {
		t.Fatalf("DiffNames() error = %v", err)
	}
	if len(files) == 0 || files[0] != "split.txt" {
		t.Fatalf("expected split.txt in diff names, got %#v", files)
	}

	runGit(t, dir, "checkout", "main")
	base, err := client.MergeBase("main", "feature/split-seams")
	if err != nil {
		t.Fatalf("MergeBase() error = %v", err)
	}
	if base == "" {
		t.Fatalf("expected merge-base hash")
	}

	runGit(t, dir, "checkout", "feature/split-seams")
	if err := os.WriteFile(filepath.Join(dir, "staged.txt"), []byte("stage me\n"), 0o644); err != nil {
		t.Fatalf("write staged.txt: %v", err)
	}
	if err := client.StagePaths([]string{"staged.txt"}); err != nil {
		t.Fatalf("StagePaths() error = %v", err)
	}
	staged, err := client.HasStagedChanges()
	if err != nil {
		t.Fatalf("HasStagedChanges() error = %v", err)
	}
	if !staged {
		t.Fatalf("expected staged changes to be detected")
	}
}
