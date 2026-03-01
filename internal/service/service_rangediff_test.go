package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRangeDiffCRFromToAnchors(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")

	cr, err := svc.AddCR("Range diff", "explicit from/to")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a1\n"), 0o644); err != nil {
		t.Fatalf("write a.txt: %v", err)
	}
	runGit(t, dir, "add", "a.txt")
	runGit(t, dir, "commit", "-m", "feat: commit one")
	from := runGit(t, dir, "rev-parse", "HEAD")

	if err := os.WriteFile(filepath.Join(dir, "b.txt"), []byte("b1\n"), 0o644); err != nil {
		t.Fatalf("write b.txt: %v", err)
	}
	runGit(t, dir, "add", "b.txt")
	runGit(t, dir, "commit", "-m", "feat: commit two")
	to := runGit(t, dir, "rev-parse", "HEAD")

	view, err := svc.RangeDiffCR(cr.ID, RangeDiffOptions{FromRef: from, ToRef: to})
	if err != nil {
		t.Fatalf("RangeDiffCR() error = %v", err)
	}
	if !strings.HasPrefix(view.FromRef, from) && !strings.HasPrefix(from, view.FromRef) {
		t.Fatalf("expected from anchor %s, got %#v", from, view)
	}
	if !containsString(view.FilesChanged, "b.txt") {
		t.Fatalf("expected b.txt in files changed, got %#v", view.FilesChanged)
	}
	if len(view.Mapping) == 0 {
		t.Fatalf("expected range-diff commit mapping rows, got %#v", view)
	}
}

func TestRangeDiffCRSinceLastCheckpointUsesLatestDoneTask(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")

	cr, err := svc.AddCR("Range since", "latest checkpoint")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	task1, err := svc.AddTask(cr.ID, "feat: task one")
	if err != nil {
		t.Fatalf("AddTask(task1) error = %v", err)
	}
	task2, err := svc.AddTask(cr.ID, "feat: task two")
	if err != nil {
		t.Fatalf("AddTask(task2) error = %v", err)
	}
	mustSetTaskContractForDiff(t, svc, cr.ID, task1.ID, []string{"one.txt"})
	mustSetTaskContractForDiff(t, svc, cr.ID, task2.ID, []string{"two.txt"})

	if err := os.WriteFile(filepath.Join(dir, "one.txt"), []byte("one\n"), 0o644); err != nil {
		t.Fatalf("write one.txt: %v", err)
	}
	_, err = svc.DoneTaskWithCheckpoint(cr.ID, task1.ID, DoneTaskOptions{Checkpoint: true, FromContract: true})
	if err != nil {
		t.Fatalf("DoneTaskWithCheckpoint(task1) error = %v", err)
	}

	if err := os.WriteFile(filepath.Join(dir, "two.txt"), []byte("two\n"), 0o644); err != nil {
		t.Fatalf("write two.txt: %v", err)
	}
	task2SHA, err := svc.DoneTaskWithCheckpoint(cr.ID, task2.ID, DoneTaskOptions{Checkpoint: true, FromContract: true})
	if err != nil {
		t.Fatalf("DoneTaskWithCheckpoint(task2) error = %v", err)
	}

	view, err := svc.RangeDiffCR(cr.ID, RangeDiffOptions{SinceLastCheckpoint: true})
	if err != nil {
		t.Fatalf("RangeDiffCR(since-last-checkpoint) error = %v", err)
	}
	if !strings.HasPrefix(view.FromRef, task2SHA) && !strings.HasPrefix(task2SHA, view.FromRef) {
		t.Fatalf("expected latest checkpoint anchor %s, got %#v", task2SHA, view)
	}
}

func TestRangeDiffCRSinceLastCheckpointRequiresDoneCheckpoint(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")

	cr, err := svc.AddCR("Range missing checkpoint", "no done checkpoints")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	if _, err := svc.RangeDiffCR(cr.ID, RangeDiffOptions{SinceLastCheckpoint: true}); err == nil || !strings.Contains(err.Error(), "no done checkpoint commit") {
		t.Fatalf("expected no checkpoint anchor error, got %v", err)
	}
}

func TestRangeDiffCRMergedBranchFallbackUsesMergedCommit(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")

	cr, err := svc.AddCR("Range merged fallback", "merged branch fallback")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	setValidContract(t, svc, cr.ID)
	task, err := svc.AddTask(cr.ID, "feat: merged fallback task")
	if err != nil {
		t.Fatalf("AddTask() error = %v", err)
	}
	mustSetTaskContractForDiff(t, svc, cr.ID, task.ID, []string{"merged.txt"})
	if err := os.WriteFile(filepath.Join(dir, "merged.txt"), []byte("merged\n"), 0o644); err != nil {
		t.Fatalf("write merged.txt: %v", err)
	}
	taskSHA, err := svc.DoneTaskWithCheckpoint(cr.ID, task.ID, DoneTaskOptions{Checkpoint: true, FromContract: true})
	if err != nil {
		t.Fatalf("DoneTaskWithCheckpoint() error = %v", err)
	}
	mergedSHA, err := svc.MergeCR(cr.ID, false, "")
	if err != nil {
		t.Fatalf("MergeCR() error = %v", err)
	}
	if svc.git.BranchExists(cr.Branch) {
		t.Fatalf("expected merged branch deleted for fallback test")
	}

	view, err := svc.RangeDiffCR(cr.ID, RangeDiffOptions{FromRef: taskSHA})
	if err != nil {
		t.Fatalf("RangeDiffCR(merged fallback) error = %v", err)
	}
	if !strings.HasPrefix(view.ToRef, mergedSHA) && !strings.HasPrefix(mergedSHA, view.ToRef) {
		t.Fatalf("expected --to anchor merged commit %s, got %#v", mergedSHA, view)
	}
	if len(view.Warnings) == 0 {
		t.Fatalf("expected fallback warning for merged commit anchor, got %#v", view)
	}
}
