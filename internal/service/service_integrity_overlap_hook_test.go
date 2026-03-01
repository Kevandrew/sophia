package service

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestAddCRWithWarningsReportsOverlap(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")

	_, err := svc.AddCR("Billing CR", "billing work")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "billing"), 0o755); err != nil {
		t.Fatalf("mkdir billing: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "billing", "a.txt"), []byte("a\n"), 0o644); err != nil {
		t.Fatalf("write billing/a.txt: %v", err)
	}
	runGit(t, dir, "add", "billing/a.txt")
	runGit(t, dir, "commit", "-m", "feat: billing a")

	runGit(t, dir, "checkout", "main")
	runGit(t, dir, "checkout", "-b", "exploratory")
	if err := os.MkdirAll(filepath.Join(dir, "billing"), 0o755); err != nil {
		t.Fatalf("mkdir billing on exploratory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "billing", "b.txt"), []byte("b\n"), 0o644); err != nil {
		t.Fatalf("write billing/b.txt: %v", err)
	}
	runGit(t, dir, "add", "billing/b.txt")
	runGit(t, dir, "commit", "-m", "feat: exploratory billing")

	result, err := svc.AddCRWithOptions("New billing CR", "another billing change", AddCROptions{Switch: true})
	if err != nil {
		t.Fatalf("AddCRWithOptions() error = %v", err)
	}
	warnings := result.Warnings
	if len(warnings) == 0 {
		t.Fatalf("expected overlap warnings")
	}
	joined := strings.Join(warnings, "\n")
	if !strings.Contains(joined, "CR-1") || !strings.Contains(joined, "/billing") {
		t.Fatalf("unexpected overlap warnings: %#v", warnings)
	}
}

func TestInstallHookBlocksBaseBranchCommitUnlessNoVerify(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")

	hookPath, err := svc.InstallHook(false)
	if err != nil {
		t.Fatalf("InstallHook() error = %v", err)
	}
	hookContent, err := os.ReadFile(hookPath)
	if err != nil {
		t.Fatalf("read hook: %v", err)
	}
	if !strings.Contains(string(hookContent), "SOPHIA_MANAGED_PRE_COMMIT") {
		t.Fatalf("expected Sophia marker in hook")
	}

	if err := os.WriteFile(filepath.Join(dir, "blocked.txt"), []byte("x\n"), 0o644); err != nil {
		t.Fatalf("write blocked.txt: %v", err)
	}
	runGit(t, dir, "add", "blocked.txt")

	cmd := exec.Command("git", "commit", "-m", "feat: blocked by hook")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected commit to fail due to hook, output: %s", string(out))
	}

	runGit(t, dir, "commit", "--no-verify", "-m", "feat: bypass hook")
}
