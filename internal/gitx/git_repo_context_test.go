package gitx

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestRepoRootAndGitCommonDirAbs(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	runGit(t, dir, "init", "-b", "main")

	subdir := filepath.Join(dir, "a", "b")
	if err := os.MkdirAll(subdir, 0o755); err != nil {
		t.Fatalf("mkdir subdir: %v", err)
	}

	client := New(subdir)
	repoRoot, err := client.RepoRoot()
	if err != nil {
		t.Fatalf("RepoRoot() error = %v", err)
	}
	if !pathsReferToSameLocation(t, repoRoot, dir) {
		t.Fatalf("expected repo root %q, got %q", dir, repoRoot)
	}

	commonDir, err := client.GitCommonDirAbs()
	if err != nil {
		t.Fatalf("GitCommonDirAbs() error = %v", err)
	}
	wantCommon := filepath.Join(dir, ".git")
	if !pathsReferToSameLocation(t, commonDir, wantCommon) {
		t.Fatalf("expected common dir %q, got %q", wantCommon, commonDir)
	}
}

func pathsReferToSameLocation(t *testing.T, a, b string) bool {
	t.Helper()
	aInfo, aErr := os.Stat(a)
	if aErr != nil {
		return false
	}
	bInfo, bErr := os.Stat(b)
	if bErr != nil {
		return false
	}
	return os.SameFile(aInfo, bInfo)
}

func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, string(out))
	}
	return strings.TrimSpace(string(out))
}
