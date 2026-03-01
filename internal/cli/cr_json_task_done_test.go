package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sophia/internal/service"
)

func TestTaskDoneFlagConflictsWithFromContract(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := service.New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	cr, err := svc.AddCR("Task flags", "conflict checks")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	task, err := svc.AddTask(cr.ID, "feat: flag conflicts")
	if err != nil {
		t.Fatalf("AddTask() error = %v", err)
	}
	intent := "Set up task for conflict checks."
	acceptance := []string{"Flag conflicts are rejected."}
	scope := []string{"feature.txt"}
	if _, err := svc.SetTaskContract(cr.ID, task.ID, service.TaskContractPatch{
		Intent:             &intent,
		AcceptanceCriteria: &acceptance,
		Scope:              &scope,
	}); err != nil {
		t.Fatalf("SetTaskContract() error = %v", err)
	}

	_, _, err = runCLI(t, dir, "cr", "task", "done", "1", "1", "--no-checkpoint", "--from-contract")
	if err == nil || !strings.Contains(err.Error(), "--no-checkpoint cannot be combined") {
		t.Fatalf("expected --no-checkpoint conflict error, got %v", err)
	}

	_, _, err = runCLI(t, dir, "cr", "task", "done", "1", "1", "--from-contract", "--all")
	if err == nil || !strings.Contains(err.Error(), "exactly one checkpoint scope mode is required") {
		t.Fatalf("expected exclusivity conflict error, got %v", err)
	}

	_, _, err = runCLI(t, dir, "cr", "task", "done", "1", "1", "--patch-file", "task.patch", "--path", "feature.txt")
	if err == nil || !strings.Contains(err.Error(), "exactly one checkpoint scope mode is required") {
		t.Fatalf("expected --patch-file exclusivity conflict error, got %v", err)
	}

	_, _, err = runCLI(t, dir, "cr", "task", "done", "1", "1", "--no-checkpoint", "--patch-file", "task.patch")
	if err == nil || !strings.Contains(err.Error(), "--no-checkpoint cannot be combined") {
		t.Fatalf("expected --no-checkpoint + --patch-file conflict error, got %v", err)
	}

	_, _, err = runCLI(t, dir, "cr", "task", "done", "1", "1", "--no-checkpoint")
	if err == nil || !strings.Contains(err.Error(), "--no-checkpoint requires --no-checkpoint-reason") {
		t.Fatalf("expected --no-checkpoint reason requirement error, got %v", err)
	}
}

func TestTaskDonePatchFileSuccess(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := service.New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	if err := os.WriteFile(filepath.Join(dir, "chunked.txt"), []byte("l1\nl2\nl3\nl4\nl5\nl6\nl7\nl8\n"), 0o644); err != nil {
		t.Fatalf("write seed file: %v", err)
	}
	runGit(t, dir, "add", "chunked.txt")
	runGit(t, dir, "commit", "-m", "chore: seed chunked file")

	cr, err := svc.AddCR("Patch file CLI", "task done patch mode")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	task, err := svc.AddTask(cr.ID, "feat: checkpoint patch file")
	if err != nil {
		t.Fatalf("AddTask() error = %v", err)
	}
	intent := "Checkpoint only selected hunks."
	acceptance := []string{"Patch-file mode stages selected hunks."}
	scope := []string{"chunked.txt"}
	if _, err := svc.SetTaskContract(cr.ID, task.ID, service.TaskContractPatch{
		Intent:             &intent,
		AcceptanceCriteria: &acceptance,
		Scope:              &scope,
	}); err != nil {
		t.Fatalf("SetTaskContract() error = %v", err)
	}

	if err := os.WriteFile(filepath.Join(dir, "chunked.txt"), []byte("l1\nl2-edited\nl3\nl4\nl5\nl6\nl7-edited\nl8\n"), 0o644); err != nil {
		t.Fatalf("write modified file: %v", err)
	}
	patch := runGit(t, dir, "diff", "--unified=0", "chunked.txt")
	patch = firstHunkPatchFromDiff(t, patch)
	if err := os.WriteFile(filepath.Join(dir, "task.patch"), []byte(patch), 0o644); err != nil {
		t.Fatalf("write patch file: %v", err)
	}

	out, _, runErr := runCLI(t, dir, "cr", "task", "done", "1", "1", "--patch-file", "task.patch")
	if runErr != nil {
		t.Fatalf("cr task done --patch-file error = %v\noutput=%s", runErr, out)
	}
	if !strings.Contains(out, "Marked task 1 done in CR 1 with checkpoint") {
		t.Fatalf("unexpected output: %q", out)
	}
}
