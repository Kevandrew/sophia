package service

import (
	"os"
	"path/filepath"
	"sophia/internal/model"
	"testing"
)

func TestReopenTaskIntegrationPreservesCheckpointByDefault(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")

	cr, err := svc.AddCR("Reopen preserve", "integration coverage for reopen wiring")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	task, err := svc.AddTask(cr.ID, "feat: add reopen support")
	if err != nil {
		t.Fatalf("AddTask() error = %v", err)
	}
	setValidTaskContract(t, svc, cr.ID, task.ID)
	if err := os.WriteFile(filepath.Join(dir, "reopen-preserve.txt"), []byte("x\n"), 0o644); err != nil {
		t.Fatalf("write reopen fixture: %v", err)
	}

	sha, err := svc.DoneTaskWithCheckpoint(cr.ID, task.ID, DoneTaskOptions{Checkpoint: true, StageAll: true})
	if err != nil {
		t.Fatalf("DoneTaskWithCheckpoint() error = %v", err)
	}

	reopened, err := svc.ReopenTask(cr.ID, task.ID, ReopenTaskOptions{})
	if err != nil {
		t.Fatalf("ReopenTask() error = %v", err)
	}
	if reopened.Status != model.TaskStatusOpen {
		t.Fatalf("expected reopened status %q, got %q", model.TaskStatusOpen, reopened.Status)
	}
	if reopened.CheckpointCommit != sha {
		t.Fatalf("expected checkpoint commit %q preserved, got %q", sha, reopened.CheckpointCommit)
	}

	loaded, err := svc.store.LoadCR(cr.ID)
	if err != nil {
		t.Fatalf("LoadCR() error = %v", err)
	}
	last := loaded.Events[len(loaded.Events)-1]
	if last.Type != model.EventTypeTaskReopened {
		t.Fatalf("expected %q event, got %q", model.EventTypeTaskReopened, last.Type)
	}
}

