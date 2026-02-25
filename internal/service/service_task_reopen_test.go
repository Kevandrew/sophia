package service

import (
	"errors"
	"strings"
	"testing"

	"sophia/internal/model"
)

func TestReopenTaskUnlockedPreservesCheckpointByDefault(t *testing.T) {
	cr := seedCR(1, "task reopen", seedCROptions{Branch: "cr-1-runtime"})
	task := seedTask(1, "done task", model.TaskStatusDone, "Before")
	task.CheckpointCommit = "abc1234"
	task.CheckpointAt = harnessTimestamp
	cr.Subtasks = []model.Subtask{task}

	h := harnessService(t, runtimeHarnessOptions{Branch: cr.Branch, CRs: []*model.CR{cr}})
	reopened, err := h.Service.reopenTaskUnlocked(cr.ID, task.ID, ReopenTaskOptions{})
	if err != nil {
		t.Fatalf("reopenTaskUnlocked() error = %v", err)
	}
	if reopened.Status != model.TaskStatusOpen {
		t.Fatalf("expected reopened task status %q, got %q", model.TaskStatusOpen, reopened.Status)
	}
	if reopened.CheckpointCommit != "abc1234" {
		t.Fatalf("expected checkpoint commit preserved, got %q", reopened.CheckpointCommit)
	}
	if reopened.CompletedAt != "" || reopened.CompletedBy != "" {
		t.Fatalf("expected completion identity cleared, got at=%q by=%q", reopened.CompletedAt, reopened.CompletedBy)
	}

	if got := h.Store.Calls("SaveCR"); got != 1 {
		t.Fatalf("expected exactly 1 save call, got %d", got)
	}
	stored, err := h.Store.LoadCR(cr.ID)
	if err != nil {
		t.Fatalf("LoadCR() error = %v", err)
	}
	last := stored.Events[len(stored.Events)-1]
	if last.Type != model.EventTypeTaskReopened {
		t.Fatalf("expected %q event, got %q", model.EventTypeTaskReopened, last.Type)
	}
	if last.Meta["checkpoint_cleared"] != "false" {
		t.Fatalf("expected checkpoint_cleared=false, got %#v", last.Meta)
	}
	if last.Meta["previous_checkpoint_commit"] != "abc1234" {
		t.Fatalf("expected previous checkpoint commit in meta, got %#v", last.Meta)
	}
	if last.Actor != h.Actor {
		t.Fatalf("expected runtime actor, got %q", last.Actor)
	}
}

func TestReopenTaskUnlockedClearsCheckpointWhenRequested(t *testing.T) {
	cr := seedCR(1, "task reopen", seedCROptions{Branch: "cr-1-runtime"})
	task := seedTask(1, "done task", model.TaskStatusDone, "Before")
	task.CheckpointCommit = "abc1234"
	task.CheckpointAt = harnessTimestamp
	task.CheckpointMessage = "message"
	task.CheckpointScope = []string{"internal/service"}
	task.CheckpointChunks = []model.CheckpointChunk{{ID: "chunk-1", Path: "internal/service/x.go"}}
	cr.Subtasks = []model.Subtask{task}

	h := harnessService(t, runtimeHarnessOptions{Branch: cr.Branch, CRs: []*model.CR{cr}})
	reopened, err := h.Service.reopenTaskUnlocked(cr.ID, task.ID, ReopenTaskOptions{ClearCheckpoint: true})
	if err != nil {
		t.Fatalf("reopenTaskUnlocked(clear) error = %v", err)
	}
	if reopened.CheckpointCommit != "" || reopened.CheckpointAt != "" || reopened.CheckpointMessage != "" {
		t.Fatalf("expected checkpoint metadata cleared, got %#v", reopened)
	}
	if len(reopened.CheckpointScope) != 0 || len(reopened.CheckpointChunks) != 0 {
		t.Fatalf("expected checkpoint scope/chunks cleared, got scope=%v chunks=%v", reopened.CheckpointScope, reopened.CheckpointChunks)
	}
	stored, err := h.Store.LoadCR(cr.ID)
	if err != nil {
		t.Fatalf("LoadCR() error = %v", err)
	}
	last := stored.Events[len(stored.Events)-1]
	if last.Meta["checkpoint_cleared"] != "true" {
		t.Fatalf("expected checkpoint_cleared=true, got %#v", last.Meta)
	}
}

func TestReopenTaskUnlockedFailsWhenTaskIsNotDone(t *testing.T) {
	cr := seedCR(1, "task reopen", seedCROptions{Branch: "cr-1-runtime"})
	cr.Subtasks = []model.Subtask{seedTask(1, "open task", model.TaskStatusOpen, "Before")}

	h := harnessService(t, runtimeHarnessOptions{Branch: cr.Branch, CRs: []*model.CR{cr}})
	_, err := h.Service.reopenTaskUnlocked(cr.ID, 1, ReopenTaskOptions{})
	if !errors.Is(err, ErrTaskNotDone) {
		t.Fatalf("expected ErrTaskNotDone, got %v", err)
	}
}

func TestSetTaskContractUnlockedUsesRuntimeProviders(t *testing.T) {
	cr := seedCR(1, "task contract", seedCROptions{Branch: "cr-1-runtime"})
	cr.Subtasks = []model.Subtask{seedTask(1, "task", model.TaskStatusOpen, "Before")}

	h := harnessService(t, runtimeHarnessOptions{Branch: cr.Branch, CRs: []*model.CR{cr}})
	intent := "Use seam actor"
	changed, err := h.Service.setTaskContractUnlocked(cr.ID, 1, TaskContractPatch{Intent: &intent})
	if err != nil {
		t.Fatalf("setTaskContractUnlocked() error = %v", err)
	}
	if len(changed) != 1 || changed[0] != "intent" {
		t.Fatalf("unexpected changed fields: %#v", changed)
	}
	if got := h.Store.Calls("SaveCR"); got != 1 {
		t.Fatalf("expected 1 save call, got %d", got)
	}
	stored, err := h.Store.LoadCR(cr.ID)
	if err != nil {
		t.Fatalf("LoadCR() error = %v", err)
	}
	gotTask := stored.Subtasks[0]
	if gotTask.Contract.Intent != intent {
		t.Fatalf("expected intent %q, got %q", intent, gotTask.Contract.Intent)
	}
	if gotTask.Contract.UpdatedBy != h.Actor {
		t.Fatalf("expected contract UpdatedBy from runtime git actor, got %q", gotTask.Contract.UpdatedBy)
	}
	last := stored.Events[len(stored.Events)-1]
	if last.Type != model.EventTypeTaskContractUpdated {
		t.Fatalf("expected event %q, got %q", model.EventTypeTaskContractUpdated, last.Type)
	}
	if last.Actor != h.Actor {
		t.Fatalf("expected event actor from runtime git provider, got %q", last.Actor)
	}
}

func TestLoadWorkingTreeTaskChunksUsesRuntimeProviders(t *testing.T) {
	cr := seedCR(1, "task chunks", seedCROptions{Branch: "cr-1-runtime"})
	cr.Subtasks = []model.Subtask{seedTask(1, "chunk task", model.TaskStatusOpen, "Before")}

	h := harnessService(t, runtimeHarnessOptions{Branch: cr.Branch, CRs: []*model.CR{cr}})
	h.TaskGit.diff = strings.Join([]string{
		"diff --git a/runtime.txt b/runtime.txt",
		"--- a/runtime.txt",
		"+++ b/runtime.txt",
		"@@ -1 +1 @@",
		"-old",
		"+new",
		"",
	}, "\n")

	chunks, err := h.Service.ListTaskChunks(cr.ID, 1, nil)
	if err != nil {
		t.Fatalf("ListTaskChunks() error = %v", err)
	}
	if len(chunks) != 1 {
		t.Fatalf("expected one chunk from runtime provider diff, got %#v", chunks)
	}
	if chunks[0].Path != "runtime.txt" {
		t.Fatalf("expected chunk path runtime.txt, got %q", chunks[0].Path)
	}
	if got := h.TaskGit.Calls("WorkingTreeUnifiedDiff"); got != 1 {
		t.Fatalf("expected runtime task git diff call, got %d", got)
	}
}
