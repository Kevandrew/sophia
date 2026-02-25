package store

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"sophia/internal/model"
)

func TestInitCreatesLayout(t *testing.T) {
	s := New(t.TempDir())
	if err := s.Init("main", model.MetadataModeLocal); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	if !s.IsInitialized() {
		t.Fatalf("expected store to be initialized")
	}

	cfg, err := s.LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if cfg.BaseBranch != "main" {
		t.Fatalf("expected base branch main, got %q", cfg.BaseBranch)
	}
	if cfg.MetadataMode != model.MetadataModeLocal {
		t.Fatalf("expected metadata mode %q, got %q", model.MetadataModeLocal, cfg.MetadataMode)
	}

	idx, err := s.LoadIndex()
	if err != nil {
		t.Fatalf("LoadIndex() error = %v", err)
	}
	if idx.NextID != 1 {
		t.Fatalf("expected next id 1, got %d", idx.NextID)
	}
}

func TestNextCRIDDeterministic(t *testing.T) {
	s := New(t.TempDir())
	if err := s.Init("main", model.MetadataModeLocal); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	id1, err := s.NextCRID()
	if err != nil {
		t.Fatalf("NextCRID() first call error = %v", err)
	}
	id2, err := s.NextCRID()
	if err != nil {
		t.Fatalf("NextCRID() second call error = %v", err)
	}

	if id1 != 1 || id2 != 2 {
		t.Fatalf("unexpected ids: %d, %d", id1, id2)
	}

	idx, err := s.LoadIndex()
	if err != nil {
		t.Fatalf("LoadIndex() error = %v", err)
	}
	if idx.NextID != 3 {
		t.Fatalf("expected next id 3, got %d", idx.NextID)
	}
}

func TestCRReadWriteRoundTrip(t *testing.T) {
	s := New(t.TempDir())
	if err := s.Init("main", model.MetadataModeLocal); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	cr := &model.CR{
		ID:          1,
		UID:         "cr_test-uid",
		Title:       "Add retries",
		Description: "Improve resilience",
		Status:      model.StatusInProgress,
		BaseBranch:  "main",
		Branch:      "sophia/cr-1",
		Notes:       []string{"started"},
		Subtasks: []model.Subtask{
			{ID: 1, Title: "Code", Status: model.TaskStatusOpen, CreatedAt: "2026-01-01T00:00:00Z", UpdatedAt: "2026-01-01T00:00:00Z", CreatedBy: "user"},
		},
		Events:    []model.Event{{TS: "2026-01-01T00:00:00Z", Actor: "user", Type: "cr_created", Summary: "Created CR 1", Ref: "cr:1"}},
		CreatedAt: "2026-01-01T00:00:00Z",
		UpdatedAt: "2026-01-01T00:00:00Z",
	}

	if err := s.SaveCR(cr); err != nil {
		t.Fatalf("SaveCR() error = %v", err)
	}

	loaded, err := s.LoadCR(1)
	if err != nil {
		t.Fatalf("LoadCR() error = %v", err)
	}
	if loaded.Title != cr.Title || loaded.Description != cr.Description {
		t.Fatalf("loaded CR mismatch: %#v", loaded)
	}
	if loaded.UID != cr.UID {
		t.Fatalf("expected uid %q, got %q", cr.UID, loaded.UID)
	}
	if len(loaded.Subtasks) != 1 || loaded.Subtasks[0].Title != "Code" {
		t.Fatalf("expected one subtask, got %#v", loaded.Subtasks)
	}
	if len(loaded.Events) != 1 || loaded.Events[0].Type != "cr_created" {
		t.Fatalf("expected event round-trip, got %#v", loaded.Events)
	}
}

func TestListCRsSortedByID(t *testing.T) {
	s := New(t.TempDir())
	if err := s.Init("main", model.MetadataModeLocal); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	for _, id := range []int{3, 1, 2} {
		cr := &model.CR{ID: id, Title: "t", Status: model.StatusInProgress, BaseBranch: "main", Branch: "sophia/cr-1", Notes: []string{}, Subtasks: []model.Subtask{}, Events: []model.Event{}, CreatedAt: "2026-01-01T00:00:00Z", UpdatedAt: "2026-01-01T00:00:00Z"}
		if err := s.SaveCR(cr); err != nil {
			t.Fatalf("SaveCR(%d) error = %v", id, err)
		}
	}

	crs, err := s.ListCRs()
	if err != nil {
		t.Fatalf("ListCRs() error = %v", err)
	}
	if len(crs) != 3 {
		t.Fatalf("expected 3 CRs, got %d", len(crs))
	}
	if crs[0].ID != 1 || crs[1].ID != 2 || crs[2].ID != 3 {
		t.Fatalf("expected sorted IDs [1,2,3], got [%d,%d,%d]", crs[0].ID, crs[1].ID, crs[2].ID)
	}
}

func TestLoadOldCRYAMLWithoutContractField(t *testing.T) {
	dir := t.TempDir()
	s := New(dir)
	if err := s.Init("main", model.MetadataModeLocal); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	raw := `id: 1
title: Legacy
description: old schema
status: in_progress
base_branch: main
branch: sophia/cr-1
notes: []
subtasks: []
events: []
created_at: 2026-01-01T00:00:00Z
updated_at: 2026-01-01T00:00:00Z
`
	if err := os.WriteFile(filepath.Join(dir, ".sophia", "cr", "1.yaml"), []byte(raw), 0o644); err != nil {
		t.Fatalf("write legacy cr yaml: %v", err)
	}

	cr, err := s.LoadCR(1)
	if err != nil {
		t.Fatalf("LoadCR() error = %v", err)
	}
	if cr.Contract.Why != "" || len(cr.Contract.Scope) != 0 {
		t.Fatalf("expected empty contract defaults, got %#v", cr.Contract)
	}
}

func TestNewWithSophiaRootUsesExplicitMetadataPath(t *testing.T) {
	repo := t.TempDir()
	metadata := filepath.Join(t.TempDir(), "custom-sophia")
	s := NewWithSophiaRoot(repo, metadata)
	if err := s.Init("main", model.MetadataModeLocal); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if got := s.SophiaDir(); got != metadata {
		t.Fatalf("expected SophiaDir %q, got %q", metadata, got)
	}
	if _, err := os.Stat(filepath.Join(metadata, "config.yaml")); err != nil {
		t.Fatalf("expected config at explicit metadata path: %v", err)
	}
}

func TestLoadCRByUID(t *testing.T) {
	s := New(t.TempDir())
	if err := s.Init("main", model.MetadataModeLocal); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	cr := &model.CR{
		ID:          1,
		UID:         "cr_selector-uid-1",
		Title:       "UID selector",
		Description: "lookup",
		Status:      model.StatusInProgress,
		BaseBranch:  "main",
		Branch:      "cr-1-uid-selector",
		Notes:       []string{},
		Subtasks:    []model.Subtask{},
		Events:      []model.Event{},
		CreatedAt:   "2026-01-01T00:00:00Z",
		UpdatedAt:   "2026-01-01T00:00:00Z",
	}
	if err := s.SaveCR(cr); err != nil {
		t.Fatalf("SaveCR() error = %v", err)
	}

	loaded, err := s.LoadCRByUID(cr.UID)
	if err != nil {
		t.Fatalf("LoadCRByUID() error = %v", err)
	}
	if loaded.ID != cr.ID {
		t.Fatalf("expected id %d, got %d", cr.ID, loaded.ID)
	}
}

func TestLoadCRTypedErrors(t *testing.T) {
	s := New(t.TempDir())
	if err := s.Init("main", model.MetadataModeLocal); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	if _, err := s.LoadCR(99); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound for missing CR id, got %v", err)
	}

	if _, err := s.LoadCRByUID(""); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("expected ErrInvalidArgument for empty uid, got %v", err)
	}

	if _, err := s.LoadCRByUID("cr_missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound for missing uid, got %v", err)
	}
}

func TestWithMutationLockTimeoutAndActionableError(t *testing.T) {
	s := New(t.TempDir())
	if err := s.Init("main", model.MetadataModeLocal); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	lockHeld := make(chan struct{})
	releaseLock := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- s.WithMutationLock(2*time.Second, func() error {
			close(lockHeld)
			<-releaseLock
			return nil
		})
	}()
	<-lockHeld

	err := s.WithMutationLock(150*time.Millisecond, func() error { return nil })
	if !errors.Is(err, ErrMutationLockTimeout) {
		t.Fatalf("expected ErrMutationLockTimeout, got %v", err)
	}
	if !strings.Contains(strings.ToLower(err.Error()), "retry") {
		t.Fatalf("expected actionable retry guidance in lock timeout error, got %v", err)
	}

	close(releaseLock)
	if holdErr := <-done; holdErr != nil {
		t.Fatalf("lock holder returned error: %v", holdErr)
	}
}

func TestWithMutationLockRejectsNilCallback(t *testing.T) {
	s := New(t.TempDir())
	if err := s.Init("main", model.MetadataModeLocal); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if err := s.WithMutationLock(time.Second, nil); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("expected ErrInvalidArgument for nil callback, got %v", err)
	}
}
