package service

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDoctorFindingsForUntiedBaseCommitAndDirtyWorktree(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")

	if err := os.WriteFile(filepath.Join(dir, "raw.txt"), []byte("raw\n"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	runGit(t, dir, "add", "raw.txt")
	runGit(t, dir, "commit", "-m", "feat: raw base commit")

	report, err := svc.Doctor(100)
	if err != nil {
		t.Fatalf("Doctor() error = %v", err)
	}
	if !hasFindingCode(report.Findings, "non_cr_branch") {
		t.Fatalf("expected non_cr_branch finding, got %#v", report.Findings)
	}
	if !hasFindingCode(report.Findings, "untied_base_commits") {
		t.Fatalf("expected untied_base_commits finding, got %#v", report.Findings)
	}

	if err := os.WriteFile(filepath.Join(dir, "dirty.tmp"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write dirty file: %v", err)
	}
	report, err = svc.Doctor(100)
	if err != nil {
		t.Fatalf("Doctor() second call error = %v", err)
	}
	if !hasFindingCode(report.Findings, "dirty_worktree") {
		t.Fatalf("expected dirty_worktree finding, got %#v", report.Findings)
	}
}

func TestCurrentSwitchAndReopenCRWorkflow(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")

	cr1, err := svc.AddCR("CR1", "first")
	if err != nil {
		t.Fatalf("AddCR #1 error = %v", err)
	}
	ctx, err := svc.CurrentCR()
	if err != nil {
		t.Fatalf("CurrentCR() error = %v", err)
	}
	if ctx.CR.ID != cr1.ID {
		t.Fatalf("expected current CR %d, got %d", cr1.ID, ctx.CR.ID)
	}

	if err := os.WriteFile(filepath.Join(dir, "feature.txt"), []byte("v1\n"), 0o644); err != nil {
		t.Fatalf("write feature file: %v", err)
	}
	runGit(t, dir, "add", "feature.txt")
	runGit(t, dir, "commit", "-m", "feat: cr1 content")
	setValidContract(t, svc, cr1.ID)

	runGit(t, dir, "checkout", "-b", "scratch")
	_, err = svc.CurrentCR()
	if err == nil {
		t.Fatalf("expected CurrentCR() error on non-CR branch")
	}

	if err := os.WriteFile(filepath.Join(dir, "dirty.tmp"), []byte("dirty"), 0o644); err != nil {
		t.Fatalf("write dirty file: %v", err)
	}
	if _, err := svc.SwitchCR(cr1.ID); err == nil {
		t.Fatalf("expected SwitchCR() to fail on dirty worktree")
	}
	if err := os.Remove(filepath.Join(dir, "dirty.tmp")); err != nil {
		t.Fatalf("remove dirty file: %v", err)
	}
	if _, err := svc.SwitchCR(cr1.ID); err != nil {
		t.Fatalf("SwitchCR() clean error = %v", err)
	}

	if _, err := svc.MergeCR(cr1.ID, false, ""); err != nil {
		t.Fatalf("MergeCR() error = %v", err)
	}

	reopened, err := svc.ReopenCR(cr1.ID)
	if err != nil {
		t.Fatalf("ReopenCR() error = %v", err)
	}
	if reopened.Status != "in_progress" {
		t.Fatalf("expected reopened status in_progress, got %q", reopened.Status)
	}
	if !svc.git.BranchExists(cr1.Branch) {
		t.Fatalf("expected branch %q to exist after reopen", cr1.Branch)
	}
	if got := runGit(t, dir, "branch", "--show-current"); got != cr1.Branch {
		t.Fatalf("expected current branch %q, got %q", cr1.Branch, got)
	}

}

func TestLogShowsMergedThenActive(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")

	cr1, err := svc.AddCR("Merged CR", "one")
	if err != nil {
		t.Fatalf("AddCR #1 error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "one.txt"), []byte("1\n"), 0o644); err != nil {
		t.Fatalf("write one.txt: %v", err)
	}
	runGit(t, dir, "add", "one.txt")
	runGit(t, dir, "commit", "-m", "feat: one")
	setValidContract(t, svc, cr1.ID)
	if _, err := svc.MergeCR(cr1.ID, false, ""); err != nil {
		t.Fatalf("MergeCR() error = %v", err)
	}

	_, err = svc.AddCR("Active CR", "two")
	if err != nil {
		t.Fatalf("AddCR #2 error = %v", err)
	}

	entries, err := svc.Log()
	if err != nil {
		t.Fatalf("Log() error = %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if entries[0].ID != cr1.ID || entries[0].Status != "merged" {
		t.Fatalf("expected merged CR first, got %#v", entries[0])
	}
	if entries[0].Who == "-" {
		t.Fatalf("expected merged entry to include actor")
	}
	if entries[0].FilesTouched == "-" {
		t.Fatalf("expected merged entry to include files touched")
	}
	if entries[1].Status != "in_progress" {
		t.Fatalf("expected active entry second, got %#v", entries[1])
	}
}

func TestReviewCategorizationAndSignals(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")

	if err := os.WriteFile(filepath.Join(dir, "mod.txt"), []byte("before\n"), 0o644); err != nil {
		t.Fatalf("write mod.txt: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "remove.txt"), []byte("remove\n"), 0o644); err != nil {
		t.Fatalf("write remove.txt: %v", err)
	}
	runGit(t, dir, "add", "mod.txt", "remove.txt")
	runGit(t, dir, "commit", "-m", "chore: base files")

	cr, err := svc.AddCR("Review scope", "categorize files")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}

	if err := os.WriteFile(filepath.Join(dir, "mod.txt"), []byte("after\n"), 0o644); err != nil {
		t.Fatalf("rewrite mod.txt: %v", err)
	}
	runGit(t, dir, "rm", "remove.txt")
	if err := os.WriteFile(filepath.Join(dir, "feature.txt"), []byte("feature\n"), 0o644); err != nil {
		t.Fatalf("write feature.txt: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "unit_test.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("write unit_test.go: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module tmp\n"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	runGit(t, dir, "add", "mod.txt", "feature.txt", "unit_test.go", "go.mod")
	runGit(t, dir, "commit", "-m", "feat: review scope")

	review, err := svc.ReviewCR(cr.ID)
	if err != nil {
		t.Fatalf("ReviewCR() error = %v", err)
	}
	if !containsString(review.NewFiles, "feature.txt") || !containsString(review.NewFiles, "unit_test.go") || !containsString(review.NewFiles, "go.mod") {
		t.Fatalf("unexpected new files: %#v", review.NewFiles)
	}
	if !containsString(review.ModifiedFiles, "mod.txt") {
		t.Fatalf("expected mod.txt in modified files: %#v", review.ModifiedFiles)
	}
	if !containsString(review.DeletedFiles, "remove.txt") {
		t.Fatalf("expected remove.txt in deleted files: %#v", review.DeletedFiles)
	}
	if !containsString(review.TestFiles, "unit_test.go") {
		t.Fatalf("expected unit_test.go in test files: %#v", review.TestFiles)
	}
	if !containsString(review.DependencyFiles, "go.mod") {
		t.Fatalf("expected go.mod in dependency files: %#v", review.DependencyFiles)
	}
}

func TestReviewValidateAndCheckFlowWithTrustDomainExtraction(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	writePolicyFileForTest(t, dir, `version: v1
trust:
  mode: advisory
  checks:
    freshness_hours: 24
    definitions:
      - key: smoke_check
        command: "printf 'ok\n'"
        tiers: [low, medium, high]
        allow_exit_codes: [0]
`)
	cr, err := svc.AddCR("trust integration", "review/validate/check integration")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	setValidContract(t, svc, cr.ID)

	validation, err := svc.ValidateCR(cr.ID)
	if err != nil {
		t.Fatalf("ValidateCR() error = %v", err)
	}
	if !validation.Valid {
		t.Fatalf("expected validation valid, got %#v", validation)
	}

	status, err := svc.TrustCheckStatusCR(cr.ID)
	if err != nil {
		t.Fatalf("TrustCheckStatusCR() error = %v", err)
	}
	if status.CheckMode != "required" {
		t.Fatalf("expected check mode required, got %q", status.CheckMode)
	}
	if len(status.CheckResults) != 1 || status.CheckResults[0].Status != policyTrustCheckStatusMissing {
		t.Fatalf("expected one missing check result before run, got %#v", status.CheckResults)
	}

	runReport, err := svc.RunTrustChecksCR(cr.ID)
	if err != nil {
		t.Fatalf("RunTrustChecksCR() error = %v", err)
	}
	if runReport.Executed != 1 {
		t.Fatalf("expected one executed check, got %d", runReport.Executed)
	}
	if len(runReport.CheckResults) != 1 || runReport.CheckResults[0].Status != policyTrustCheckStatusPass {
		t.Fatalf("expected one passing check result after run, got %#v", runReport.CheckResults)
	}

	review, err := svc.ReviewCR(cr.ID)
	if err != nil {
		t.Fatalf("ReviewCR() error = %v", err)
	}
	if review.Trust == nil {
		t.Fatalf("expected non-nil trust report")
	}
	if len(review.Trust.CheckResults) != 1 || review.Trust.CheckResults[0].Status != policyTrustCheckStatusPass {
		t.Fatalf("expected review trust check result to remain pass, got %#v", review.Trust.CheckResults)
	}
}
