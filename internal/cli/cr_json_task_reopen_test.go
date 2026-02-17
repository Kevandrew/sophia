package cli

import (
	"os"
	"path/filepath"
	"testing"

	"sophia/internal/service"
)

func TestCRTaskReopenJSONSuccess(t *testing.T) {
	dir := t.TempDir()
	svc := service.New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")

	cr, err := svc.AddCR("Task reopen json", "json mode")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	task, err := svc.AddTask(cr.ID, "feat: reopen json")
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
	if err := os.WriteFile(filepath.Join(dir, "reopen-json.txt"), []byte("x\n"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	sha, err := svc.DoneTaskWithCheckpoint(cr.ID, task.ID, service.DoneTaskOptions{Checkpoint: true, StageAll: true})
	if err != nil {
		t.Fatalf("DoneTaskWithCheckpoint() error = %v", err)
	}

	out, _, runErr := runCLI(t, dir, "cr", "task", "reopen", "1", "1", "--json")
	if runErr != nil {
		t.Fatalf("cr task reopen --json error = %v\noutput=%s", runErr, out)
	}
	env := decodeEnvelope(t, out)
	if !env.OK {
		t.Fatalf("expected ok envelope, got %#v", env)
	}
	if got := int(env.Data["cr_id"].(float64)); got != 1 {
		t.Fatalf("expected cr_id=1, got %d", got)
	}
	if got := int(env.Data["task_id"].(float64)); got != 1 {
		t.Fatalf("expected task_id=1, got %d", got)
	}
	if got := env.Data["status"].(string); got != "open" {
		t.Fatalf("expected status=open, got %q", got)
	}
	if got := env.Data["checkpoint_cleared"].(bool); got {
		t.Fatalf("expected checkpoint_cleared=false, got true")
	}
	if got := env.Data["checkpoint_commit"].(string); got != sha {
		t.Fatalf("expected checkpoint_commit=%q, got %q", sha, got)
	}
}

func TestCRTaskReopenJSONTaskNotDoneErrorCode(t *testing.T) {
	dir := t.TempDir()
	svc := service.New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if _, err := svc.AddCR("Task reopen json error", "open task should fail"); err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	if _, err := svc.AddTask(1, "chore: still open"); err != nil {
		t.Fatalf("AddTask() error = %v", err)
	}

	out, _, runErr := runCLI(t, dir, "cr", "task", "reopen", "1", "1", "--json")
	if runErr == nil {
		t.Fatalf("expected reopen error for non-done task, output=%s", out)
	}
	env := decodeEnvelope(t, out)
	if env.OK {
		t.Fatalf("expected non-ok envelope, got %#v", env)
	}
	if env.Error == nil {
		t.Fatalf("expected error payload, got %#v", env)
	}
	if env.Error.Code != "task_not_done" {
		t.Fatalf("expected error code task_not_done, got %q", env.Error.Code)
	}
}
