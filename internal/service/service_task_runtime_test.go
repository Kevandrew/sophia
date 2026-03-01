package service

import (
	"errors"
	"sophia/internal/model"
	"testing"
)

func TestActiveTaskMergeGuardDefaultsToServiceMergeGuard(t *testing.T) {
	t.Parallel()
	svc := &Service{}
	guard := svc.activeTaskMergeGuard()
	if guard == nil {
		t.Fatalf("expected non-nil merge guard")
	}
}

func TestActiveTaskMergeGuardUsesOverride(t *testing.T) {
	t.Parallel()
	expected := errors.New("guard hit")
	svc := &Service{}
	svc.overrideTaskMergeGuardForTests(func(*model.CR) error { return expected })

	if err := svc.activeTaskMergeGuard()(&model.CR{}); !errors.Is(err, expected) {
		t.Fatalf("expected override guard error %v, got %v", expected, err)
	}
}

func TestDoneTaskWithCheckpointNoCheckpointUsesRuntimeProviders(t *testing.T) {
	t.Parallel()
	cr := seedCR(1, "runtime done", seedCROptions{Branch: "cr-1-runtime"})
	task := seedTask(1, "done task", model.TaskStatusOpen, "Before")
	task.Contract = model.TaskContract{
		Intent:             "close task with explicit no-checkpoint reason",
		AcceptanceCriteria: []string{"task can be marked done"},
		Scope:              []string{"internal/service"},
	}
	cr.Subtasks = []model.Subtask{task}

	h := harnessService(t, runtimeHarnessOptions{Branch: cr.Branch, CRs: []*model.CR{cr}})
	reason := "task has no code changes"
	sha, err := h.Service.DoneTaskWithCheckpoint(cr.ID, task.ID, DoneTaskOptions{
		Checkpoint:         false,
		NoCheckpointReason: reason,
	})
	if err != nil {
		t.Fatalf("DoneTaskWithCheckpoint() error = %v", err)
	}
	if sha != "" {
		t.Fatalf("expected empty checkpoint sha for no-checkpoint mode, got %q", sha)
	}

	if got := h.Store.Calls("SaveCR"); got != 1 {
		t.Fatalf("expected 1 SaveCR call, got %d", got)
	}
	if got := h.TaskGit.Calls("Actor"); got < 1 {
		t.Fatalf("expected task runtime git Actor() to be called at least once, got %d", got)
	}
	if got := h.TaskGit.Calls("Commit"); got != 0 {
		t.Fatalf("expected no checkpoint commit call, got %d", got)
	}

	stored, err := h.Store.LoadCR(cr.ID)
	if err != nil {
		t.Fatalf("LoadCR() error = %v", err)
	}
	done := stored.Subtasks[0]
	if done.Status != model.TaskStatusDone {
		t.Fatalf("expected task status %q, got %q", model.TaskStatusDone, done.Status)
	}
	if done.CompletedBy != h.Actor {
		t.Fatalf("expected completed_by from runtime actor %q, got %q", h.Actor, done.CompletedBy)
	}
	if done.CheckpointSource != model.TaskCheckpointSourceTaskNoCheckpoint {
		t.Fatalf("expected checkpoint source %q, got %q", model.TaskCheckpointSourceTaskNoCheckpoint, done.CheckpointSource)
	}
	if done.CheckpointReason != reason {
		t.Fatalf("expected checkpoint reason %q, got %q", reason, done.CheckpointReason)
	}

	last := stored.Events[len(stored.Events)-1]
	if last.Type != model.EventTypeTaskDone {
		t.Fatalf("expected event %q, got %q", model.EventTypeTaskDone, last.Type)
	}
	if last.Actor != h.Actor {
		t.Fatalf("expected event actor from runtime provider, got %q", last.Actor)
	}
	if last.Meta["checkpoint_source"] != model.TaskCheckpointSourceTaskNoCheckpoint {
		t.Fatalf("expected task_done metadata checkpoint_source=%q, got %#v", model.TaskCheckpointSourceTaskNoCheckpoint, last.Meta)
	}
}
