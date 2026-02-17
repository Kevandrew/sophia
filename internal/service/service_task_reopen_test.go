package service

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReopenTaskPreservesCheckpointByDefault(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")

	cr, err := svc.AddCR("Reopen preserve", "preserve checkpoint metadata")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	task, err := svc.AddTask(cr.ID, "feat: add reopen support")
	if err != nil {
		t.Fatalf("AddTask() error = %v", err)
	}
	setValidTaskContract(t, svc, cr.ID, task.ID)
	if err := os.WriteFile(filepath.Join(dir, "reopen-preserve.txt"), []byte("x\n"), 0o644); err != nil {
		t.Fatalf("write reopen file: %v", err)
	}

	sha, err := svc.DoneTaskWithCheckpoint(cr.ID, task.ID, DoneTaskOptions{Checkpoint: true, StageAll: true})
	if err != nil {
		t.Fatalf("DoneTaskWithCheckpoint() error = %v", err)
	}

	reopened, err := svc.ReopenTask(cr.ID, task.ID, ReopenTaskOptions{})
	if err != nil {
		t.Fatalf("ReopenTask() error = %v", err)
	}
	if reopened.Status != "open" {
		t.Fatalf("expected reopened status open, got %q", reopened.Status)
	}
	if reopened.CheckpointCommit != sha {
		t.Fatalf("expected checkpoint commit %q preserved, got %q", sha, reopened.CheckpointCommit)
	}
	if reopened.CompletedAt != "" || reopened.CompletedBy != "" {
		t.Fatalf("expected completion identity cleared, got completed_at=%q completed_by=%q", reopened.CompletedAt, reopened.CompletedBy)
	}

	loaded, err := svc.store.LoadCR(cr.ID)
	if err != nil {
		t.Fatalf("LoadCR() error = %v", err)
	}
	last := loaded.Events[len(loaded.Events)-1]
	if last.Type != "task_reopened" {
		t.Fatalf("expected task_reopened event, got %q", last.Type)
	}
	if last.Meta["checkpoint_cleared"] != "false" {
		t.Fatalf("expected checkpoint_cleared=false, got %#v", last.Meta)
	}
	if last.Meta["previous_checkpoint_commit"] != sha {
		t.Fatalf("expected previous checkpoint commit %q in meta, got %#v", sha, last.Meta)
	}
}

func TestReopenTaskWithClearCheckpointClearsCheckpointFields(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")

	cr, err := svc.AddCR("Reopen clear", "clear checkpoint metadata")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	task, err := svc.AddTask(cr.ID, "feat: checkpoint then clear")
	if err != nil {
		t.Fatalf("AddTask() error = %v", err)
	}
	setValidTaskContract(t, svc, cr.ID, task.ID)
	if err := os.WriteFile(filepath.Join(dir, "reopen-clear.txt"), []byte("x\n"), 0o644); err != nil {
		t.Fatalf("write reopen file: %v", err)
	}

	sha, err := svc.DoneTaskWithCheckpoint(cr.ID, task.ID, DoneTaskOptions{Checkpoint: true, StageAll: true})
	if err != nil {
		t.Fatalf("DoneTaskWithCheckpoint() error = %v", err)
	}

	reopened, err := svc.ReopenTask(cr.ID, task.ID, ReopenTaskOptions{ClearCheckpoint: true})
	if err != nil {
		t.Fatalf("ReopenTask() error = %v", err)
	}
	if reopened.Status != "open" {
		t.Fatalf("expected reopened status open, got %q", reopened.Status)
	}
	if reopened.CheckpointCommit != "" || reopened.CheckpointAt != "" || reopened.CheckpointMessage != "" {
		t.Fatalf("expected checkpoint fields cleared, got %#v", reopened)
	}
	if len(reopened.CheckpointScope) != 0 || len(reopened.CheckpointChunks) != 0 {
		t.Fatalf("expected checkpoint scope/chunks cleared, got scope=%#v chunks=%#v", reopened.CheckpointScope, reopened.CheckpointChunks)
	}

	loaded, err := svc.store.LoadCR(cr.ID)
	if err != nil {
		t.Fatalf("LoadCR() error = %v", err)
	}
	last := loaded.Events[len(loaded.Events)-1]
	if last.Type != "task_reopened" {
		t.Fatalf("expected task_reopened event, got %q", last.Type)
	}
	if last.Meta["checkpoint_cleared"] != "true" {
		t.Fatalf("expected checkpoint_cleared=true, got %#v", last.Meta)
	}
	if last.Meta["previous_checkpoint_commit"] != sha {
		t.Fatalf("expected previous checkpoint commit %q in meta, got %#v", sha, last.Meta)
	}
}

func TestReopenTaskFailsWhenTaskIsNotDone(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	cr, err := svc.AddCR("Reopen not done", "reject non-done task")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	task, err := svc.AddTask(cr.ID, "chore: still open")
	if err != nil {
		t.Fatalf("AddTask() error = %v", err)
	}

	_, err = svc.ReopenTask(cr.ID, task.ID, ReopenTaskOptions{})
	if !errors.Is(err, ErrTaskNotDone) {
		t.Fatalf("expected ErrTaskNotDone, got %v", err)
	}
}

func TestReopenTaskFailsWhenCRIsNotInProgress(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	cr, err := svc.AddCR("Reopen merged", "reject merged CR")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	task, err := svc.AddTask(cr.ID, "chore: done")
	if err != nil {
		t.Fatalf("AddTask() error = %v", err)
	}
	setValidTaskContract(t, svc, cr.ID, task.ID)
	if err := svc.DoneTask(cr.ID, task.ID); err != nil {
		t.Fatalf("DoneTask() error = %v", err)
	}

	loaded, err := svc.store.LoadCR(cr.ID)
	if err != nil {
		t.Fatalf("LoadCR() error = %v", err)
	}
	loaded.Status = "merged"
	if err := svc.store.SaveCR(loaded); err != nil {
		t.Fatalf("SaveCR() error = %v", err)
	}

	_, err = svc.ReopenTask(cr.ID, task.ID, ReopenTaskOptions{})
	if err == nil || !strings.Contains(err.Error(), "is not in progress") {
		t.Fatalf("expected CR in-progress error, got %v", err)
	}
}
