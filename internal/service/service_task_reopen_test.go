package service

import (
	"errors"
	"fmt"
	"sophia/internal/gitx"
	"sophia/internal/model"
	"testing"
	"time"
)

type stubTaskRuntimeStore struct {
	cr       *model.CR
	loadErr  error
	saveErr  error
	loadHits int
	saveHits int
}

func (s *stubTaskRuntimeStore) LoadCR(id int) (*model.CR, error) {
	if s.loadErr != nil {
		return nil, s.loadErr
	}
	s.loadHits++
	if s.cr == nil || s.cr.ID != id {
		return nil, fmt.Errorf("cr %d not found", id)
	}
	return cloneRemoteCR(s.cr), nil
}

func (s *stubTaskRuntimeStore) SaveCR(cr *model.CR) error {
	if s.saveErr != nil {
		return s.saveErr
	}
	s.saveHits++
	s.cr = cloneRemoteCR(cr)
	return nil
}

type stubTaskRuntimeGit struct {
	actor string
}

func (g *stubTaskRuntimeGit) Actor() string { return g.actor }

func (g *stubTaskRuntimeGit) CurrentBranch() (string, error) { return "", nil }

func (g *stubTaskRuntimeGit) HasStagedChanges() (bool, error) { return false, nil }

func (g *stubTaskRuntimeGit) StageAll() error { return nil }

func (g *stubTaskRuntimeGit) StagePaths(paths []string) error { return nil }

func (g *stubTaskRuntimeGit) ApplyPatchToIndex(patchPath string) error { return nil }

func (g *stubTaskRuntimeGit) PathHasChanges(path string) (bool, error) { return false, nil }

func (g *stubTaskRuntimeGit) WorkingTreeStatus() ([]gitx.StatusEntry, error) { return nil, nil }

func (g *stubTaskRuntimeGit) Commit(msg string) error { return nil }

func (g *stubTaskRuntimeGit) HeadShortSHA() (string, error) { return "", nil }

func testTaskRuntimeService(t *testing.T, cr *model.CR) (*Service, *stubTaskRuntimeStore) {
	t.Helper()
	now := time.Date(2026, time.February, 25, 12, 0, 0, 0, time.UTC)
	store := &stubTaskRuntimeStore{cr: cloneRemoteCR(cr)}
	git := &stubTaskRuntimeGit{actor: "Runtime Tester <runtime@test>"}
	svc := &Service{
		repoRoot: t.TempDir(),
		now:      func() time.Time { return now },
	}
	svc.overrideTaskRuntimeProvidersForTests(git, store)
	svc.overrideTaskMergeGuardForTests(func(_ *model.CR) error { return nil })
	return svc, store
}

func TestReopenTaskUnlockedPreservesCheckpointByDefault(t *testing.T) {
	cr := &model.CR{
		ID:        1,
		Status:    model.StatusInProgress,
		CreatedAt: "2026-02-25T00:00:00Z",
		UpdatedAt: "2026-02-25T00:00:00Z",
		Subtasks: []model.Subtask{{
			ID:               1,
			Title:            "done task",
			Status:           model.TaskStatusDone,
			CreatedAt:        "2026-02-25T00:00:00Z",
			UpdatedAt:        "2026-02-25T00:00:00Z",
			CompletedAt:      "2026-02-25T00:00:00Z",
			CompletedBy:      "Before",
			CheckpointCommit: "abc1234",
			CheckpointAt:     "2026-02-25T00:00:00Z",
		}},
	}
	svc, store := testTaskRuntimeService(t, cr)

	reopened, err := svc.reopenTaskUnlocked(1, 1, ReopenTaskOptions{})
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

	if store.saveHits != 1 {
		t.Fatalf("expected exactly 1 save call, got %d", store.saveHits)
	}
	last := store.cr.Events[len(store.cr.Events)-1]
	if last.Type != model.EventTypeTaskReopened {
		t.Fatalf("expected %q event, got %q", model.EventTypeTaskReopened, last.Type)
	}
	if last.Meta["checkpoint_cleared"] != "false" {
		t.Fatalf("expected checkpoint_cleared=false, got %#v", last.Meta)
	}
	if last.Meta["previous_checkpoint_commit"] != "abc1234" {
		t.Fatalf("expected previous checkpoint commit in meta, got %#v", last.Meta)
	}
	if last.Actor != "Runtime Tester <runtime@test>" {
		t.Fatalf("expected runtime actor, got %q", last.Actor)
	}
}

func TestReopenTaskUnlockedClearsCheckpointWhenRequested(t *testing.T) {
	cr := &model.CR{
		ID:        1,
		Status:    model.StatusInProgress,
		CreatedAt: "2026-02-25T00:00:00Z",
		UpdatedAt: "2026-02-25T00:00:00Z",
		Subtasks: []model.Subtask{{
			ID:                1,
			Title:             "done task",
			Status:            model.TaskStatusDone,
			CheckpointCommit:  "abc1234",
			CheckpointAt:      "2026-02-25T00:00:00Z",
			CheckpointMessage: "message",
			CheckpointScope:   []string{"internal/service"},
			CheckpointChunks: []model.CheckpointChunk{{
				ID: "chunk-1",
			}},
		}},
	}
	svc, store := testTaskRuntimeService(t, cr)

	reopened, err := svc.reopenTaskUnlocked(1, 1, ReopenTaskOptions{ClearCheckpoint: true})
	if err != nil {
		t.Fatalf("reopenTaskUnlocked(clear) error = %v", err)
	}
	if reopened.CheckpointCommit != "" || reopened.CheckpointAt != "" || reopened.CheckpointMessage != "" {
		t.Fatalf("expected checkpoint metadata cleared, got %#v", reopened)
	}
	if len(reopened.CheckpointScope) != 0 || len(reopened.CheckpointChunks) != 0 {
		t.Fatalf("expected checkpoint scope/chunks cleared, got scope=%v chunks=%v", reopened.CheckpointScope, reopened.CheckpointChunks)
	}
	last := store.cr.Events[len(store.cr.Events)-1]
	if last.Meta["checkpoint_cleared"] != "true" {
		t.Fatalf("expected checkpoint_cleared=true, got %#v", last.Meta)
	}
}

func TestReopenTaskUnlockedFailsWhenTaskIsNotDone(t *testing.T) {
	cr := &model.CR{
		ID:     1,
		Status: model.StatusInProgress,
		Subtasks: []model.Subtask{{
			ID:     1,
			Title:  "open task",
			Status: model.TaskStatusOpen,
		}},
	}
	svc, _ := testTaskRuntimeService(t, cr)

	_, err := svc.reopenTaskUnlocked(1, 1, ReopenTaskOptions{})
	if !errors.Is(err, ErrTaskNotDone) {
		t.Fatalf("expected ErrTaskNotDone, got %v", err)
	}
}

func TestSetTaskContractUnlockedUsesRuntimeProviders(t *testing.T) {
	cr := &model.CR{
		ID:        1,
		Status:    model.StatusInProgress,
		CreatedAt: "2026-02-25T00:00:00Z",
		UpdatedAt: "2026-02-25T00:00:00Z",
		Subtasks: []model.Subtask{{
			ID:        1,
			Title:     "task",
			Status:    model.TaskStatusOpen,
			CreatedAt: "2026-02-25T00:00:00Z",
			UpdatedAt: "2026-02-25T00:00:00Z",
		}},
	}
	svc, store := testTaskRuntimeService(t, cr)
	intent := "Use seam actor"

	changed, err := svc.setTaskContractUnlocked(1, 1, TaskContractPatch{
		Intent: &intent,
	})
	if err != nil {
		t.Fatalf("setTaskContractUnlocked() error = %v", err)
	}
	if len(changed) != 1 || changed[0] != "intent" {
		t.Fatalf("unexpected changed fields: %#v", changed)
	}
	if store.saveHits != 1 {
		t.Fatalf("expected 1 save call, got %d", store.saveHits)
	}
	gotTask := store.cr.Subtasks[0]
	if gotTask.Contract.Intent != intent {
		t.Fatalf("expected intent %q, got %q", intent, gotTask.Contract.Intent)
	}
	if gotTask.Contract.UpdatedBy != "Runtime Tester <runtime@test>" {
		t.Fatalf("expected contract UpdatedBy from runtime git actor, got %q", gotTask.Contract.UpdatedBy)
	}
	last := store.cr.Events[len(store.cr.Events)-1]
	if last.Type != model.EventTypeTaskContractUpdated {
		t.Fatalf("expected event %q, got %q", model.EventTypeTaskContractUpdated, last.Type)
	}
	if last.Actor != "Runtime Tester <runtime@test>" {
		t.Fatalf("expected event actor from runtime git provider, got %q", last.Actor)
	}
}
