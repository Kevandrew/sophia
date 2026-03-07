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
	installFakeGHCommandForMergedFreshness(t, dir, "#!/bin/sh\nif [ \"$1\" = \"-R\" ]; then\n  shift\n  shift\nfi\nif [ \"$1\" = \"pr\" ] && [ \"$2\" = \"view\" ]; then\n  echo '{\"number\":42,\"url\":\"https://github.com/acme/repo/pull/42\",\"state\":\"MERGED\",\"isDraft\":false,\"headRefOid\":\""+head+"\",\"headRefName\":\"cr-1-remote-merged-freshness\",\"baseRefName\":\"main\",\"mergedAt\":\"2026-03-07T19:20:00Z\",\"mergeCommit\":{\"oid\":\""+head+"\"},\"author\":{\"login\":\"bot\"},\"latestReviews\":[],\"statusCheckRollup\":[]}'\n  exit 0\nfi\necho \"unexpected gh args: $@\" >&2\nexit 1\n")

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
