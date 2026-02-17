package service

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"sophia/internal/model"
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

func TestAddCRWithWarningsReportsOverlap(t *testing.T) {
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

	_, warnings, err := svc.AddCRWithWarnings("New billing CR", "another billing change")
	if err != nil {
		t.Fatalf("AddCRWithWarnings() error = %v", err)
	}
	if len(warnings) == 0 {
		t.Fatalf("expected overlap warnings")
	}
	joined := strings.Join(warnings, "\n")
	if !strings.Contains(joined, "CR-1") || !strings.Contains(joined, "/billing") {
		t.Fatalf("unexpected overlap warnings: %#v", warnings)
	}
}

func TestInstallHookBlocksBaseBranchCommitUnlessNoVerify(t *testing.T) {
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

func TestDoctorFlagsTrackedSophiaMetadataInLocalMode(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")
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

	if err := os.RemoveAll(filepath.Join(dir, ".sophia")); err != nil {
		t.Fatalf("remove .sophia: %v", err)
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

func TestRepairFromGitRebuildsCRsAndRealignsIndex(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")

	runGit(t, dir, "commit", "--allow-empty",
		"-m", "[CR-2] Existing intent",
		"-m", "Intent:\nRecovered why\n\nSubtasks:\n- [x] #1 Do thing\n\nNotes:\n- recovered note\n\nMetadata:\n- actor: Test User <test@example.com>\n- merged_at: 2026-02-17T00:00:00Z\n\nSophia-CR: 2\nSophia-CR-UID: cr_fixture-uid-2\nSophia-Base-Ref: release/2026-q1\nSophia-Base-Commit: deadbeefcafebabe\nSophia-Parent-CR: 1\nSophia-Intent: Existing intent\nSophia-Tasks: 1 completed",
	)

	if err := svc.store.SaveIndex(model.Index{NextID: 1}); err != nil {
		t.Fatalf("SaveIndex() error = %v", err)
	}
	if err := os.RemoveAll(filepath.Join(dir, ".sophia", "cr")); err != nil {
		t.Fatalf("remove cr dir: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(dir, ".sophia", "cr"), 0o755); err != nil {
		t.Fatalf("recreate cr dir: %v", err)
	}

	report, err := svc.RepairFromGit("main", false)
	if err != nil {
		t.Fatalf("RepairFromGit() error = %v", err)
	}
	if report.Imported < 1 || report.HighestCRID < 2 || report.NextID < 3 {
		t.Fatalf("unexpected repair report: %#v", report)
	}

	repaired, err := svc.store.LoadCR(2)
	if err != nil {
		t.Fatalf("LoadCR(2) error = %v", err)
	}
	if repaired.Status != "merged" || repaired.Title != "Existing intent" {
		t.Fatalf("unexpected repaired CR: %#v", repaired)
	}
	if repaired.UID != "cr_fixture-uid-2" {
		t.Fatalf("expected repaired UID from footer, got %#v", repaired.UID)
	}
	if repaired.BaseRef != "release/2026-q1" || repaired.BaseCommit != "deadbeefcafebabe" || repaired.ParentCRID != 1 {
		t.Fatalf("expected repaired base/parent metadata from footers, got %#v", repaired)
	}
	if len(repaired.Notes) != 1 || repaired.Notes[0] != "recovered note" {
		t.Fatalf("unexpected repaired notes: %#v", repaired.Notes)
	}
	if len(repaired.Subtasks) != 1 || repaired.Subtasks[0].Status != model.TaskStatusDone {
		t.Fatalf("unexpected repaired subtasks: %#v", repaired.Subtasks)
	}

	nextCR, err := svc.AddCR("Next intent", "after repair")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	if nextCR.ID != 3 {
		t.Fatalf("expected next CR id 3, got %d", nextCR.ID)
	}
}

func TestRepairBackfillsMissingUIDOnExistingCRMetadata(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")

	cr, err := svc.AddCR("UID backfill", "repair should set uid")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}

	loaded, err := svc.store.LoadCR(cr.ID)
	if err != nil {
		t.Fatalf("LoadCR() error = %v", err)
	}
	loaded.UID = ""
	loaded.BaseRef = ""
	loaded.BaseCommit = ""
	if err := svc.store.SaveCR(loaded); err != nil {
		t.Fatalf("SaveCR(clear uid/base) error = %v", err)
	}

	if _, err := svc.RepairFromGit("main", false); err != nil {
		t.Fatalf("RepairFromGit() error = %v", err)
	}

	repaired, err := svc.store.LoadCR(cr.ID)
	if err != nil {
		t.Fatalf("LoadCR(repaired) error = %v", err)
	}
	if strings.TrimSpace(repaired.UID) == "" {
		t.Fatalf("expected repair to backfill uid, got %#v", repaired)
	}
	if strings.TrimSpace(repaired.BaseRef) == "" || strings.TrimSpace(repaired.BaseCommit) == "" {
		t.Fatalf("expected repair to backfill base metadata, got %#v", repaired)
	}
}

func hasFindingCode(findings []DoctorFinding, code string) bool {
	for _, finding := range findings {
		if finding.Code == code {
			return true
		}
	}
	return false
}

func containsString(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}
