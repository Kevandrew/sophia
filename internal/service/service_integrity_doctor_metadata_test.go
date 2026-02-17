package service

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDoctorFlagsTrackedSophiaMetadataInLocalMode(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")
	if err := os.MkdirAll(filepath.Join(dir, ".sophia"), 0o755); err != nil {
		t.Fatalf("mkdir .sophia: %v", err)
	}
	sharedConfig := filepath.Join(localMetadataDir(t, dir), "config.yaml")
	configBytes, readErr := os.ReadFile(sharedConfig)
	if readErr != nil {
		t.Fatalf("read shared config: %v", readErr)
	}
	if err := os.WriteFile(filepath.Join(dir, ".sophia", "config.yaml"), configBytes, 0o644); err != nil {
		t.Fatalf("write legacy .sophia/config.yaml: %v", err)
	}
	runGit(t, dir, "add", "-f", ".sophia/config.yaml")
	runGit(t, dir, "commit", "-m", "chore: track local metadata")

	report, err := svc.Doctor(100)
	if err != nil {
		t.Fatalf("Doctor() error = %v", err)
	}
	if !hasFindingCode(report.Findings, "tracked_sophia_metadata") {
		t.Fatalf("expected tracked_sophia_metadata finding, got %#v", report.Findings)
	}
}

func TestDoctorFlagsStaleMergedBranches(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")

	cr, err := svc.AddCR("Merged CR", "stale branch")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "stale.txt"), []byte("x\n"), 0o644); err != nil {
		t.Fatalf("write stale.txt: %v", err)
	}
	runGit(t, dir, "add", "stale.txt")
	runGit(t, dir, "commit", "-m", "feat: stale branch")
	setValidContract(t, svc, cr.ID)
	if _, err := svc.MergeCR(cr.ID, true, ""); err != nil {
		t.Fatalf("MergeCR(keep=true) error = %v", err)
	}

	report, err := svc.Doctor(100)
	if err != nil {
		t.Fatalf("Doctor() error = %v", err)
	}
	if !hasFindingCode(report.Findings, "stale_merged_branches") {
		t.Fatalf("expected stale_merged_branches finding, got %#v", report.Findings)
	}
}

func TestDoctorIgnoresLegacyPersistChoreCommit(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")
	if err := os.WriteFile(filepath.Join(dir, "legacy.txt"), []byte("x\n"), 0o644); err != nil {
		t.Fatalf("write legacy.txt: %v", err)
	}
	runGit(t, dir, "add", "legacy.txt")
	runGit(t, dir, "commit", "-m", "chore: persist CR-9 merged metadata")

	report, err := svc.Doctor(100)
	if err != nil {
		t.Fatalf("Doctor() error = %v", err)
	}
	if hasFindingCode(report.Findings, "untied_base_commits") {
		t.Fatalf("expected legacy persist commit to be ignored, got %#v", report.Findings)
	}
}

func TestLogFallsBackToGitWhenLocalMetadataMissing(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")

	cr, err := svc.AddCR("Fallback CR", "from git log")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "fallback.txt"), []byte("x\n"), 0o644); err != nil {
		t.Fatalf("write fallback.txt: %v", err)
	}
	runGit(t, dir, "add", "fallback.txt")
	runGit(t, dir, "commit", "-m", "feat: fallback")
	setValidContract(t, svc, cr.ID)
	if _, err := svc.MergeCR(cr.ID, false, ""); err != nil {
		t.Fatalf("MergeCR() error = %v", err)
	}

	if err := os.RemoveAll(svc.store.SophiaDir()); err != nil {
		t.Fatalf("remove metadata dir: %v", err)
	}
	entries, err := svc.Log()
	if err != nil {
		t.Fatalf("Log() fallback error = %v", err)
	}
	if len(entries) == 0 {
		t.Fatalf("expected git-derived log entries")
	}
	if entries[0].ID != cr.ID || entries[0].Status != "merged" {
		t.Fatalf("unexpected fallback entry: %#v", entries[0])
	}
}
