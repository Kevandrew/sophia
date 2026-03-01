package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sophia/internal/service"
)

func TestCRTaskReopenTextHappyPath(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := service.New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	cr, err := svc.AddCR("Task reopen", "text mode")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	task, err := svc.AddTask(cr.ID, "chore: reopen me")
	if err != nil {
		t.Fatalf("AddTask() error = %v", err)
	}
	intent := "Allow reopen flow."
	acceptance := []string{"Task can be reopened from done to open."}
	scope := []string{"."}
	if _, err := svc.SetTaskContract(cr.ID, task.ID, service.TaskContractPatch{
		Intent:             &intent,
		AcceptanceCriteria: &acceptance,
		Scope:              &scope,
	}); err != nil {
		t.Fatalf("SetTaskContract() error = %v", err)
	}
	if err := svc.DoneTask(cr.ID, task.ID); err != nil {
		t.Fatalf("DoneTask() error = %v", err)
	}

	out, _, runErr := runCLI(t, dir, "cr", "task", "reopen", "1", "1")
	if runErr != nil {
		t.Fatalf("cr task reopen error = %v\noutput=%s", runErr, out)
	}
	if !strings.Contains(out, "Reopened task 1 in CR 1") {
		t.Fatalf("unexpected output: %q", out)
	}

	tasks, err := svc.ListTasks(cr.ID)
	if err != nil {
		t.Fatalf("ListTasks() error = %v", err)
	}
	if tasks[0].Status != "open" {
		t.Fatalf("expected reopened task status open, got %q", tasks[0].Status)
	}
}

func TestCRTaskReopenTextClearCheckpoint(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := service.New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	cr, err := svc.AddCR("Task reopen clear", "clear checkpoint")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	task, err := svc.AddTask(cr.ID, "feat: reopen and clear")
	if err != nil {
		t.Fatalf("AddTask() error = %v", err)
	}
	intent := "Allow reopen flow."
	acceptance := []string{"Task can be reopened from done to open."}
	scope := []string{"."}
	if _, err := svc.SetTaskContract(cr.ID, task.ID, service.TaskContractPatch{
		Intent:             &intent,
		AcceptanceCriteria: &acceptance,
		Scope:              &scope,
	}); err != nil {
		t.Fatalf("SetTaskContract() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "reopen-cli.txt"), []byte("x\n"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if _, err := svc.DoneTaskWithCheckpoint(cr.ID, task.ID, service.DoneTaskOptions{Checkpoint: true, StageAll: true}); err != nil {
		t.Fatalf("DoneTaskWithCheckpoint() error = %v", err)
	}

	out, _, runErr := runCLI(t, dir, "cr", "task", "reopen", "1", "1", "--clear-checkpoint")
	if runErr != nil {
		t.Fatalf("cr task reopen --clear-checkpoint error = %v\noutput=%s", runErr, out)
	}
	if !strings.Contains(out, "and cleared checkpoint metadata") {
		t.Fatalf("unexpected output: %q", out)
	}

	tasks, err := svc.ListTasks(cr.ID)
	if err != nil {
		t.Fatalf("ListTasks() error = %v", err)
	}
	taskState := tasks[0]
	if taskState.CheckpointCommit != "" || taskState.CheckpointMessage != "" || taskState.CheckpointAt != "" {
		t.Fatalf("expected checkpoint fields cleared, got %#v", taskState)
	}
}

func TestCRTaskReopenTextInvalidState(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := service.New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if _, err := svc.AddCR("Task reopen error", "open task should fail"); err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	if _, err := svc.AddTask(1, "chore: still open"); err != nil {
		t.Fatalf("AddTask() error = %v", err)
	}

	out, _, runErr := runCLI(t, dir, "cr", "task", "reopen", "1", "1")
	if runErr == nil {
		t.Fatalf("expected reopen error for non-done task, output=%s", out)
	}
	if !strings.Contains(runErr.Error(), "task is not done") {
		t.Fatalf("expected task-not-done message, got %v", runErr)
	}
}
