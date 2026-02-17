package service

import (
	"os"
	"path/filepath"
	"testing"

	"sophia/internal/model"
	"sophia/internal/store"
)

func TestLegacyLocalMetadataMigratesToSharedStoreWithBackup(t *testing.T) {
	dir := t.TempDir()
	runGit(t, dir, "init", "-b", "main")
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")
	runGit(t, dir, "commit", "--allow-empty", "-m", "seed")

	legacyDir := filepath.Join(dir, ".sophia")
	legacyStore := store.NewWithSophiaRoot(dir, legacyDir)
	if err := legacyStore.Init("main", model.MetadataModeLocal); err != nil {
		t.Fatalf("legacy Init() error = %v", err)
	}
	if err := legacyStore.SaveIndex(model.Index{NextID: 7}); err != nil {
		t.Fatalf("legacy SaveIndex() error = %v", err)
	}
	legacyCR := &model.CR{
		ID:          1,
		UID:         "cr_fixture",
		Title:       "legacy",
		Description: "legacy",
		Status:      model.StatusInProgress,
		BaseBranch:  "main",
		Branch:      "sophia/cr-1",
		Notes:       []string{},
		Subtasks:    []model.Subtask{},
		Events:      []model.Event{},
		CreatedAt:   "2026-01-01T00:00:00Z",
		UpdatedAt:   "2026-01-01T00:00:00Z",
	}
	if err := legacyStore.SaveCR(legacyCR); err != nil {
		t.Fatalf("legacy SaveCR() error = %v", err)
	}

	svc := New(dir)
	sharedDir := localMetadataDir(t, dir)
	if !pathsReferToSameLocation(t, svc.store.SophiaDir(), sharedDir) {
		t.Fatalf("expected service store at shared path %q, got %q", sharedDir, svc.store.SophiaDir())
	}
	if _, err := os.Stat(filepath.Join(sharedDir, "config.yaml")); err != nil {
		t.Fatalf("expected shared config.yaml: %v", err)
	}
	if _, err := os.Stat(filepath.Join(sharedDir, "cr", "1.yaml")); err != nil {
		t.Fatalf("expected migrated CR file: %v", err)
	}
	idx, err := svc.store.LoadIndex()
	if err != nil {
		t.Fatalf("LoadIndex(shared) error = %v", err)
	}
	if idx.NextID < 7 {
		t.Fatalf("expected next_id preserved from legacy index, got %d", idx.NextID)
	}

	backups, err := filepath.Glob(filepath.Join(dir, ".sophia.migrated.*"))
	if err != nil {
		t.Fatalf("glob backups error = %v", err)
	}
	if len(backups) != 1 {
		t.Fatalf("expected one legacy backup, got %#v", backups)
	}
}

func TestLegacyAndSharedMetadataConflictReconcilesIntoSharedStore(t *testing.T) {
	dir := t.TempDir()
	runGit(t, dir, "init", "-b", "main")
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")
	runGit(t, dir, "commit", "--allow-empty", "-m", "seed")

	sharedDir := localMetadataDir(t, dir)
	legacyDir := filepath.Join(dir, ".sophia")
	sharedStore := store.NewWithSophiaRoot(dir, sharedDir)
	legacyStore := store.NewWithSophiaRoot(dir, legacyDir)
	if err := sharedStore.Init("main", model.MetadataModeLocal); err != nil {
		t.Fatalf("shared Init() error = %v", err)
	}
	if err := legacyStore.Init("main", model.MetadataModeLocal); err != nil {
		t.Fatalf("legacy Init() error = %v", err)
	}
	if err := sharedStore.SaveIndex(model.Index{NextID: 2}); err != nil {
		t.Fatalf("shared SaveIndex() error = %v", err)
	}
	if err := legacyStore.SaveIndex(model.Index{NextID: 10}); err != nil {
		t.Fatalf("legacy SaveIndex() error = %v", err)
	}
	if err := sharedStore.SaveCR(&model.CR{ID: 1, Title: "shared", Description: "shared", Status: model.StatusInProgress, BaseBranch: "main", Branch: "sophia/cr-1", Notes: []string{}, Subtasks: []model.Subtask{}, Events: []model.Event{}, CreatedAt: "2026-01-01T00:00:00Z", UpdatedAt: "2026-01-01T00:00:00Z"}); err != nil {
		t.Fatalf("shared SaveCR() error = %v", err)
	}
	if err := legacyStore.SaveCR(&model.CR{ID: 9, Title: "legacy", Description: "legacy", Status: model.StatusInProgress, BaseBranch: "main", Branch: "sophia/cr-9", Notes: []string{}, Subtasks: []model.Subtask{}, Events: []model.Event{}, CreatedAt: "2026-01-01T00:00:00Z", UpdatedAt: "2026-01-01T00:00:00Z"}); err != nil {
		t.Fatalf("legacy SaveCR() error = %v", err)
	}

	svc := New(dir)
	if !pathsReferToSameLocation(t, svc.store.SophiaDir(), sharedDir) {
		t.Fatalf("expected shared store path, got %q", svc.store.SophiaDir())
	}
	if _, err := svc.store.LoadCR(1); err != nil {
		t.Fatalf("expected shared CR 1 retained: %v", err)
	}
	if _, err := svc.store.LoadCR(9); err != nil {
		t.Fatalf("expected legacy CR 9 imported: %v", err)
	}
	idx, err := svc.store.LoadIndex()
	if err != nil {
		t.Fatalf("LoadIndex() error = %v", err)
	}
	if idx.NextID < 10 {
		t.Fatalf("expected reconciled next_id >= 10, got %d", idx.NextID)
	}

	backups, err := filepath.Glob(filepath.Join(dir, ".sophia.migrated.*"))
	if err != nil {
		t.Fatalf("glob backups error = %v", err)
	}
	if len(backups) != 1 {
		t.Fatalf("expected one legacy backup after reconciliation, got %#v", backups)
	}
}
