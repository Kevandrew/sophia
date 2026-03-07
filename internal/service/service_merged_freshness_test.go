package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sophia/internal/model"
)

func TestStatusCRMergedDoesNotSuggestRefreshAfterBaseMoves(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")

	cr, err := svc.AddCR("Merged freshness", "merged CRs should not go stale after base moves")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	setValidContract(t, svc, cr.ID)
	task, err := svc.AddTask(cr.ID, "merged freshness task")
	if err != nil {
		t.Fatalf("AddTask() error = %v", err)
	}
	mustSetTaskContractForDiff(t, svc, cr.ID, task.ID, []string{"merged-freshness.txt"})
	if err := os.WriteFile(filepath.Join(dir, "merged-freshness.txt"), []byte("ok\n"), 0o644); err != nil {
		t.Fatalf("write merged-freshness.txt: %v", err)
	}
	if _, err := svc.DoneTaskWithCheckpoint(cr.ID, task.ID, DoneTaskOptions{Checkpoint: true, FromContract: true}); err != nil {
		t.Fatalf("DoneTaskWithCheckpoint() error = %v", err)
	}

	mergedSHA, err := svc.MergeCR(cr.ID, false, "")
	if err != nil {
		t.Fatalf("MergeCR() error = %v", err)
	}

	if err := os.WriteFile(filepath.Join(dir, "post-merge.txt"), []byte("later\n"), 0o644); err != nil {
		t.Fatalf("write post-merge.txt: %v", err)
	}
	runGit(t, dir, "add", "post-merge.txt")
	runGit(t, dir, "commit", "-m", "chore: move main after merge")

	status, err := svc.StatusCR(cr.ID)
	if err != nil {
		t.Fatalf("StatusCR() error = %v", err)
	}
	if status.Status != model.StatusMerged {
		t.Fatalf("expected merged status, got %q", status.Status)
	}
	if status.FreshnessState != "current" {
		t.Fatalf("expected merged freshness_state=current, got %q", status.FreshnessState)
	}
	if len(status.FreshnessSuggestedCommands) != 0 {
		t.Fatalf("expected no refresh suggestion for merged CR, got %#v", status.FreshnessSuggestedCommands)
	}

	reloaded, err := svc.store.LoadCR(cr.ID)
	if err != nil {
		t.Fatalf("LoadCR() error = %v", err)
	}
	if !strings.HasPrefix(strings.TrimSpace(reloaded.BaseCommit), strings.TrimSpace(mergedSHA)) && !strings.HasPrefix(strings.TrimSpace(mergedSHA), strings.TrimSpace(reloaded.BaseCommit)) {
		t.Fatalf("expected merged base_commit %q to match merged sha %q", reloaded.BaseCommit, mergedSHA)
	}
}

func TestPRStatusRemoteMergedNormalizesBaseCommit(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SOPHIA.yaml"), []byte("version: v1\nmerge:\n  mode: pr_gate\narchive:\n  enabled: false\n"), 0o644); err != nil {
		t.Fatalf("write SOPHIA.yaml: %v", err)
	}
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")

	cr, err := svc.AddCR("Remote merged freshness", "remote merge reconciliation should normalize base anchors")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	loaded, err := svc.store.LoadCR(cr.ID)
	if err != nil {
		t.Fatalf("LoadCR() error = %v", err)
	}
	loaded.PR.Number = 42
	loaded.PR.Repo = "acme/repo"
	if err := svc.store.SaveCR(loaded); err != nil {
		t.Fatalf("SaveCR() error = %v", err)
	}

	head := strings.TrimSpace(runGit(t, dir, "rev-parse", "HEAD"))
	installFakeGHCommandForMergedFreshness(t, dir, "#!/bin/sh\nif [ \"$1\" = \"-R\" ]; then\n  shift\n  shift\nfi\nif [ \"$1\" = \"pr\" ] && [ \"$2\" = \"view\" ]; then\n  echo '{\"number\":42,\"url\":\"https://github.com/acme/repo/pull/42\",\"state\":\"MERGED\",\"isDraft\":false,\"headRefOid\":\""+head+"\",\"headRefName\":\""+strings.TrimSpace(loaded.Branch)+"\",\"baseRefName\":\"main\",\"mergedAt\":\"2026-03-07T19:20:00Z\",\"mergeCommit\":{\"oid\":\""+head+"\"},\"author\":{\"login\":\"bot\"},\"latestReviews\":[],\"statusCheckRollup\":[]}'\n  exit 0\nfi\necho \"unexpected gh args: $@\" >&2\nexit 1\n")

	if _, err := svc.PRStatus(cr.ID); err != nil {
		t.Fatalf("PRStatus() error = %v", err)
	}

	reloaded, err := svc.store.LoadCR(cr.ID)
	if err != nil {
		t.Fatalf("LoadCR() after PRStatus error = %v", err)
	}
	if reloaded.Status != model.StatusMerged {
		t.Fatalf("expected merged status, got %q", reloaded.Status)
	}
	if strings.TrimSpace(reloaded.BaseCommit) != head {
		t.Fatalf("expected base_commit %q, got %q", head, reloaded.BaseCommit)
	}
	if strings.TrimSpace(reloaded.MergedCommit) != head {
		t.Fatalf("expected merged_commit %q, got %q", head, reloaded.MergedCommit)
	}

	status, err := svc.StatusCR(cr.ID)
	if err != nil {
		t.Fatalf("StatusCR() error = %v", err)
	}
	if status.FreshnessState != "current" {
		t.Fatalf("expected merged freshness_state=current, got %q", status.FreshnessState)
	}
	if len(status.FreshnessSuggestedCommands) != 0 {
		t.Fatalf("expected no merged refresh suggestion, got %#v", status.FreshnessSuggestedCommands)
	}
}

func installFakeGHCommandForMergedFreshness(t *testing.T, dir, body string) {
	t.Helper()

	path := filepath.Join(dir, "gh")
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatalf("write fake gh: %v", err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func hasCRFindingCode(findings []CRDoctorFinding, code string) bool {
	for _, finding := range findings {
		if strings.TrimSpace(finding.Code) == code {
			return true
		}
	}
	return false
}

func TestDoctorCRMergedKeepBranchDoesNotFlagBaseCommitFindings(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")

	cr, err := svc.AddCR("Merged doctor keep-branch", "merged CRs with retained branches should not look corrupt")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	setValidContract(t, svc, cr.ID)
	task, err := svc.AddTask(cr.ID, "merged doctor task")
	if err != nil {
		t.Fatalf("AddTask() error = %v", err)
	}
	mustSetTaskContractForDiff(t, svc, cr.ID, task.ID, []string{"merged-doctor.txt"})
	if err := os.WriteFile(filepath.Join(dir, "merged-doctor.txt"), []byte("ok\n"), 0o644); err != nil {
		t.Fatalf("write merged-doctor.txt: %v", err)
	}
	if _, err := svc.DoneTaskWithCheckpoint(cr.ID, task.ID, DoneTaskOptions{Checkpoint: true, FromContract: true}); err != nil {
		t.Fatalf("DoneTaskWithCheckpoint() error = %v", err)
	}
	if _, err := svc.MergeCR(cr.ID, true, ""); err != nil {
		t.Fatalf("MergeCR() error = %v", err)
	}

	report, err := svc.DoctorCR(cr.ID)
	if err != nil {
		t.Fatalf("DoctorCR() error = %v", err)
	}
	if hasCRFindingCode(report.Findings, "base_commit_drift") {
		t.Fatalf("expected merged CR to avoid base_commit_drift, got %#v", report.Findings)
	}
	if hasCRFindingCode(report.Findings, "base_commit_unreachable") {
		t.Fatalf("expected merged CR to avoid base_commit_unreachable, got %#v", report.Findings)
	}
}

func TestPRStatusAlreadyMergedPreservesMergeProvenanceWhileHealingBaseCommit(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SOPHIA.yaml"), []byte("version: v1\nmerge:\n  mode: pr_gate\narchive:\n  enabled: false\n"), 0o644); err != nil {
		t.Fatalf("write SOPHIA.yaml: %v", err)
	}
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")

	cr, err := svc.AddCR("Already merged heal", "healing should not rewrite merged provenance")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	loaded, err := svc.store.LoadCR(cr.ID)
	if err != nil {
		t.Fatalf("LoadCR() error = %v", err)
	}
	head := strings.TrimSpace(runGit(t, dir, "rev-parse", "HEAD"))
	loaded.Status = model.StatusMerged
	loaded.MergedAt = "2026-03-07T18:00:00Z"
	loaded.MergedBy = "Original Merger <merge@example.com>"
	loaded.MergedCommit = head
	loaded.BaseCommit = "deadbeef"
	loaded.PR.Number = 43
	loaded.PR.Repo = "acme/repo"
	if err := svc.store.SaveCR(loaded); err != nil {
		t.Fatalf("SaveCR() error = %v", err)
	}
	installFakeGHCommandForMergedFreshness(t, dir, "#!/bin/sh\nif [ \"$1\" = \"-R\" ]; then\n  shift\n  shift\nfi\nif [ \"$1\" = \"pr\" ] && [ \"$2\" = \"view\" ]; then\n  echo '{\"number\":43,\"url\":\"https://github.com/acme/repo/pull/43\",\"state\":\"MERGED\",\"isDraft\":false,\"headRefOid\":\""+head+"\",\"headRefName\":\""+strings.TrimSpace(loaded.Branch)+"\",\"baseRefName\":\"main\",\"mergedAt\":\"2026-03-07T19:20:00Z\",\"mergeCommit\":{\"oid\":\""+head+"\"},\"author\":{\"login\":\"bot\"},\"latestReviews\":[],\"statusCheckRollup\":[]}'\n  exit 0\nfi\necho \"unexpected gh args: $@\" >&2\nexit 1\n")

	if _, err := svc.PRStatus(cr.ID); err != nil {
		t.Fatalf("PRStatus() error = %v", err)
	}

	reloaded, err := svc.store.LoadCR(cr.ID)
	if err != nil {
		t.Fatalf("LoadCR() after PRStatus error = %v", err)
	}
	if strings.TrimSpace(reloaded.MergedBy) != "Original Merger <merge@example.com>" {
		t.Fatalf("expected merged_by to remain original, got %q", reloaded.MergedBy)
	}
	if strings.TrimSpace(reloaded.MergedAt) != "2026-03-07T18:00:00Z" {
		t.Fatalf("expected merged_at to remain original, got %q", reloaded.MergedAt)
	}
	if strings.TrimSpace(reloaded.BaseCommit) != head {
		t.Fatalf("expected healed base_commit %q, got %q", head, reloaded.BaseCommit)
	}
}

func TestPRStatusMergedFallbackHeadDoesNotClobberExistingBaseAnchor(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SOPHIA.yaml"), []byte("version: v1\nmerge:\n  mode: pr_gate\narchive:\n  enabled: false\n"), 0o644); err != nil {
		t.Fatalf("write SOPHIA.yaml: %v", err)
	}
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")

	cr, err := svc.AddCR("Merged fallback head", "fallback head oid should not replace canonical base anchors")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	loaded, err := svc.store.LoadCR(cr.ID)
	if err != nil {
		t.Fatalf("LoadCR() error = %v", err)
	}
	head := strings.TrimSpace(runGit(t, dir, "rev-parse", "HEAD"))
	loaded.Status = model.StatusMerged
	loaded.MergedAt = "2026-03-07T18:00:00Z"
	loaded.MergedBy = "Original Merger <merge@example.com>"
	loaded.MergedCommit = "cafebabe"
	loaded.BaseCommit = head
	loaded.PR.Number = 44
	loaded.PR.Repo = "acme/repo"
	if err := svc.store.SaveCR(loaded); err != nil {
		t.Fatalf("SaveCR() error = %v", err)
	}
	installFakeGHCommandForMergedFreshness(t, dir, "#!/bin/sh\nif [ \"$1\" = \"-R\" ]; then\n  shift\n  shift\nfi\nif [ \"$1\" = \"pr\" ] && [ \"$2\" = \"view\" ]; then\n  echo '{\"number\":44,\"url\":\"https://github.com/acme/repo/pull/44\",\"state\":\"MERGED\",\"isDraft\":false,\"headRefOid\":\"feedface\",\"headRefName\":\"cr-1-merged-fallback-head\",\"baseRefName\":\"main\",\"mergedAt\":\"2026-03-07T19:20:00Z\",\"author\":{\"login\":\"bot\"},\"latestReviews\":[],\"statusCheckRollup\":[]}'\n  exit 0\nfi\necho \"unexpected gh args: $@\" >&2\nexit 1\n")

	if _, err := svc.PRStatus(cr.ID); err != nil {
		t.Fatalf("PRStatus() error = %v", err)
	}

	reloaded, err := svc.store.LoadCR(cr.ID)
	if err != nil {
		t.Fatalf("LoadCR() after PRStatus error = %v", err)
	}
	if strings.TrimSpace(reloaded.BaseCommit) != head {
		t.Fatalf("expected existing base_commit %q to remain unchanged, got %q", head, reloaded.BaseCommit)
	}
	if strings.TrimSpace(reloaded.MergedCommit) != "cafebabe" {
		t.Fatalf("expected existing merged_commit to remain unchanged under fallback-only status, got %q", reloaded.MergedCommit)
	}
}
