package store

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"sophia/internal/model"
)

func TestInitCreatesLayout(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
		DelegationRuns: []model.DelegationRun{
			{
				ID:        "dr_roundtrip",
				Status:    model.DelegationRunStatusRunning,
				CreatedAt: "2026-01-01T00:00:00Z",
				CreatedBy: "user",
				UpdatedAt: "2026-01-01T00:01:00Z",
				Request: model.DelegationRequest{
					Runtime:   "mock",
					TaskIDs:   []int{1},
					SkillRefs: []string{"/Users/example/.agents/skills/sophia/SKILL.md"},
					IntentSnapshot: &model.HQIntentSnapshot{
						Title: "Add retries",
						Contract: model.HQIntentContractSnapshot{
							Why:   "Improve resilience",
							Scope: []string{"internal/service"},
						},
					},
				},
				Events: []model.DelegationRunEvent{
					{ID: 1, TS: "2026-01-01T00:00:10Z", Kind: model.DelegationEventKindRunStarted, Summary: "started"},
				},
				Result: &model.DelegationResult{
					Status:       model.DelegationRunStatusCompleted,
					Summary:      "complete",
					FilesChanged: []string{"internal/service/retries.go"},
				},
			},
		},
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
	if len(loaded.DelegationRuns) != 1 {
		t.Fatalf("expected delegation run round-trip, got %#v", loaded.DelegationRuns)
	}
	if got := loaded.DelegationRuns[0].Request.Runtime; got != "mock" {
		t.Fatalf("expected delegation runtime mock, got %q", got)
	}
	if got := loaded.DelegationRuns[0].Result; got == nil || got.Status != model.DelegationRunStatusCompleted {
		t.Fatalf("expected completed delegation result, got %#v", loaded.DelegationRuns[0].Result)
	}
}

func TestListCRsSortedByID(t *testing.T) {
	t.Parallel()
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

func TestListCRsUsesCachedMetadataWhenFilesAreUnchanged(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	s := New(dir)
	if err := s.Init("main", model.MetadataModeLocal); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	cr := &model.CR{
		ID:          1,
		Title:       "cached list",
		Description: "cache",
		Status:      model.StatusInProgress,
		BaseBranch:  "main",
		Branch:      "cr-cache",
		Notes:       []string{},
		Subtasks:    []model.Subtask{},
		Events:      []model.Event{},
		CreatedAt:   "2026-01-01T00:00:00Z",
		UpdatedAt:   "2026-01-01T00:00:00Z",
	}
	if err := s.SaveCR(cr); err != nil {
		t.Fatalf("SaveCR() error = %v", err)
	}

	if _, err := s.ListCRs(); err != nil {
		t.Fatalf("ListCRs() initial error = %v", err)
	}

	crPath := filepath.Join(dir, ".sophia", "cr", "1.yaml")
	info, err := os.Stat(crPath)
	if err != nil {
		t.Fatalf("stat cr file: %v", err)
	}
	if err := os.Chmod(crPath, 0o000); err != nil {
		t.Fatalf("chmod 000 cr file: %v", err)
	}
	defer func() {
		_ = os.Chmod(crPath, 0o644)
	}()

	crs, err := s.ListCRs()
	if err != nil {
		t.Fatalf("ListCRs() cached error = %v", err)
	}
	if len(crs) != 1 || crs[0].Title != "cached list" {
		t.Fatalf("expected cached list result, got %#v", crs)
	}

	if got, err := os.Stat(crPath); err != nil {
		t.Fatalf("restat cr file: %v", err)
	} else if !got.ModTime().Equal(info.ModTime()) {
		t.Fatalf("expected chmod not to invalidate mtime-based cache check")
	}
}

func TestListCRsRefreshesCacheWhenCRFileChanges(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	s := New(dir)
	if err := s.Init("main", model.MetadataModeLocal); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	cr := &model.CR{
		ID:          1,
		Title:       "before",
		Description: "cache",
		Status:      model.StatusInProgress,
		BaseBranch:  "main",
		Branch:      "cr-cache",
		Notes:       []string{},
		Subtasks:    []model.Subtask{},
		Events:      []model.Event{},
		CreatedAt:   "2026-01-01T00:00:00Z",
		UpdatedAt:   "2026-01-01T00:00:00Z",
	}
	if err := s.SaveCR(cr); err != nil {
		t.Fatalf("SaveCR() error = %v", err)
	}
	if _, err := s.ListCRs(); err != nil {
		t.Fatalf("ListCRs() initial error = %v", err)
	}

	loaded, err := s.LoadCR(1)
	if err != nil {
		t.Fatalf("LoadCR() error = %v", err)
	}
	loaded.Title = "after"
	loaded.UpdatedAt = "2026-01-01T00:01:00Z"
	time.Sleep(2 * time.Millisecond)
	if err := s.SaveCR(loaded); err != nil {
		t.Fatalf("SaveCR(updated) error = %v", err)
	}

	crs, err := s.ListCRs()
	if err != nil {
		t.Fatalf("ListCRs() refreshed error = %v", err)
	}
	if len(crs) != 1 || crs[0].Title != "after" {
		t.Fatalf("expected refreshed title after cache invalidation, got %#v", crs)
	}
}

func TestLoadOldCRYAMLWithoutContractField(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
	s := New(t.TempDir())
	if err := s.Init("main", model.MetadataModeLocal); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if err := s.WithMutationLock(time.Second, nil); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("expected ErrInvalidArgument for nil callback, got %v", err)
	}
}

func TestWithMutationLockPathRejectsEmptyPath(t *testing.T) {
	t.Parallel()
	s := New(t.TempDir())
	if err := s.Init("main", model.MetadataModeLocal); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if err := s.WithMutationLockPath(" ", time.Second, func() error { return nil }); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("expected ErrInvalidArgument for empty lock path, got %v", err)
	}
}

func TestConcurrentSaveCRDoesNotRaceOnTempRename(t *testing.T) {
	s := New(t.TempDir())
	if err := s.Init("main", model.MetadataModeLocal); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	cr := &model.CR{
		ID:          1,
		UID:         "cr_concurrent-save",
		Title:       "base",
		Description: "base",
		Status:      model.StatusInProgress,
		BaseBranch:  "main",
		Branch:      "sophia/cr-1",
		Notes:       []string{},
		Subtasks:    []model.Subtask{},
		Events:      []model.Event{},
		CreatedAt:   "2026-01-01T00:00:00Z",
		UpdatedAt:   "2026-01-01T00:00:00Z",
	}
	if err := s.SaveCR(cr); err != nil {
		t.Fatalf("SaveCR(seed) error = %v", err)
	}

	const workers = 24
	start := make(chan struct{})
	errCh := make(chan error, workers)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			<-start
			loaded, err := s.LoadCR(1)
			if err != nil {
				errCh <- err
				return
			}
			loaded.Title = fmt.Sprintf("title-%02d", idx)
			errCh <- s.SaveCR(loaded)
		}(i)
	}
	close(start)
	wg.Wait()
	close(errCh)

	for err := range errCh {
		if err != nil {
			t.Fatalf("concurrent SaveCR() error = %v", err)
		}
	}
}

func TestConcurrentLoadAndListCRsStayAvailableDuringSave(t *testing.T) {
	s := New(t.TempDir())
	if err := s.Init("main", model.MetadataModeLocal); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	cr := &model.CR{
		ID:          1,
		UID:         "cr_concurrent-readers",
		Title:       "base",
		Description: "base",
		Status:      model.StatusInProgress,
		BaseBranch:  "main",
		Branch:      "sophia/cr-1",
		Notes:       []string{},
		Subtasks:    []model.Subtask{},
		Events:      []model.Event{},
		CreatedAt:   "2026-01-01T00:00:00Z",
		UpdatedAt:   "2026-01-01T00:00:00Z",
	}
	if err := s.SaveCR(cr); err != nil {
		t.Fatalf("SaveCR(seed) error = %v", err)
	}

	const (
		writers    = 8
		readers    = 8
		iterations = 20
	)
	start := make(chan struct{})
	errCh := make(chan error, writers+readers)
	var wg sync.WaitGroup

	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			<-start
			for iter := 0; iter < iterations; iter++ {
				loaded, err := s.LoadCR(1)
				if err != nil {
					errCh <- fmt.Errorf("writer load iteration %d: %w", iter, err)
					return
				}
				loaded.Title = fmt.Sprintf("writer-%02d-%02d", idx, iter)
				if err := s.SaveCR(loaded); err != nil {
					errCh <- fmt.Errorf("writer save iteration %d: %w", iter, err)
					return
				}
			}
		}(i)
	}

	for i := 0; i < readers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			for iter := 0; iter < iterations; iter++ {
				loaded, err := s.LoadCR(1)
				if err != nil {
					errCh <- fmt.Errorf("reader load iteration %d: %w", iter, err)
					return
				}
				if loaded.ID != 1 {
					errCh <- fmt.Errorf("reader load iteration %d: unexpected id %d", iter, loaded.ID)
					return
				}
				crs, err := s.ListCRs()
				if err != nil {
					errCh <- fmt.Errorf("reader list iteration %d: %w", iter, err)
					return
				}
				if len(crs) != 1 || crs[0].ID != 1 {
					errCh <- fmt.Errorf("reader list iteration %d: unexpected CR list %#v", iter, crs)
					return
				}
			}
		}()
	}

	close(start)
	wg.Wait()
	close(errCh)

	for err := range errCh {
		if err != nil {
			t.Fatalf("concurrent read/write error = %v", err)
		}
	}
}
