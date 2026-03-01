package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sophia/internal/model"
)

func TestRepairFromGitRebuildsCRsAndRealignsIndex(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")

	runGit(t, dir, "commit", "--allow-empty",
		"-m", "[CR-2] Existing intent",
		"-m", "Intent:\nRecovered why\n\nSubtasks:\n- [x] #1 Do thing\n\nNotes:\n- recovered note\n\nMetadata:\n- actor: Test User <test@example.com>\n- merged_at: 2026-02-17T00:00:00Z\n\nSophia-CR: 2\nSophia-CR-UID: cr_fixture-uid-2\nSophia-Base-Ref: release/2026-q1\nSophia-Base-Commit: deadbeefcafebabe\nSophia-Branch: kevandrew/cr-2-existing-intent\nSophia-Branch-Scheme: human_alias_v1\nSophia-Parent-CR: 1\nSophia-Intent: Existing intent\nSophia-Tasks: 1 completed",
	)

	if err := svc.store.SaveIndex(model.Index{NextID: 1}); err != nil {
		t.Fatalf("SaveIndex() error = %v", err)
	}
	if err := os.RemoveAll(filepath.Join(svc.store.SophiaDir(), "cr")); err != nil {
		t.Fatalf("remove cr dir: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(svc.store.SophiaDir(), "cr"), 0o755); err != nil {
		t.Fatalf("recreate cr dir: %v", err)
	}

	report, err := svc.RepairFromGit("main", false)
	if err != nil {
		t.Fatalf("RepairFromGit() error = %v", err)
	}
	if report.Imported < 1 || report.HighestCRID < 2 || report.NextID < 3 {
		t.Fatalf("unexpected repair report: %#v", report)
	}

	repaired, err := svc.store.LoadCR(2)
	if err != nil {
		t.Fatalf("LoadCR(2) error = %v", err)
	}
	if repaired.Status != "merged" || repaired.Title != "Existing intent" {
		t.Fatalf("unexpected repaired CR: %#v", repaired)
	}
	if repaired.UID != "cr_fixture-uid-2" {
		t.Fatalf("expected repaired UID from footer, got %#v", repaired.UID)
	}
	if repaired.BaseRef != "release/2026-q1" || repaired.BaseCommit != "deadbeefcafebabe" || repaired.ParentCRID != 1 {
		t.Fatalf("expected repaired base/parent metadata from footers, got %#v", repaired)
	}
	if repaired.Branch != "kevandrew/cr-2-existing-intent" {
		t.Fatalf("expected repaired branch from footer, got %#v", repaired.Branch)
	}
	if len(repaired.Notes) != 1 || repaired.Notes[0] != "recovered note" {
		t.Fatalf("unexpected repaired notes: %#v", repaired.Notes)
	}
	if len(repaired.Subtasks) != 1 || repaired.Subtasks[0].Status != model.TaskStatusDone {
		t.Fatalf("unexpected repaired subtasks: %#v", repaired.Subtasks)
	}

	nextCR, err := svc.AddCR("Next intent", "after repair")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	if nextCR.ID != 3 {
		t.Fatalf("expected next CR id 3, got %d", nextCR.ID)
	}
}

func TestRepairBackfillsMissingUIDOnExistingCRMetadata(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")

	cr, err := svc.AddCR("UID backfill", "repair should set uid")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}

	loaded, err := svc.store.LoadCR(cr.ID)
	if err != nil {
		t.Fatalf("LoadCR() error = %v", err)
	}
	loaded.UID = ""
	loaded.BaseRef = ""
	loaded.BaseCommit = ""
	if err := svc.store.SaveCR(loaded); err != nil {
		t.Fatalf("SaveCR(clear uid/base) error = %v", err)
	}

	if _, err := svc.RepairFromGit("main", false); err != nil {
		t.Fatalf("RepairFromGit() error = %v", err)
	}

	repaired, err := svc.store.LoadCR(cr.ID)
	if err != nil {
		t.Fatalf("LoadCR(repaired) error = %v", err)
	}
	if strings.TrimSpace(repaired.UID) == "" {
		t.Fatalf("expected repair to backfill uid, got %#v", repaired)
	}
	if strings.TrimSpace(repaired.BaseRef) == "" || strings.TrimSpace(repaired.BaseCommit) == "" {
		t.Fatalf("expected repair to backfill base metadata, got %#v", repaired)
	}
}

func TestLegacyAndChunkCheckpointMetadataCoexistence(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")

	cr, err := svc.AddCR("Metadata coexistence", "legacy and chunk checkpoint data")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	legacyTask, err := svc.AddTask(cr.ID, "feat: legacy scope task")
	if err != nil {
		t.Fatalf("AddTask(legacy) error = %v", err)
	}
	chunkTask, err := svc.AddTask(cr.ID, "feat: chunk scope task")
	if err != nil {
		t.Fatalf("AddTask(chunk) error = %v", err)
	}
	setValidContract(t, svc, cr.ID)
	setValidTaskContract(t, svc, cr.ID, legacyTask.ID)
	setValidTaskContract(t, svc, cr.ID, chunkTask.ID)

	loaded, err := svc.store.LoadCR(cr.ID)
	if err != nil {
		t.Fatalf("LoadCR() error = %v", err)
	}
	loaded.Subtasks[legacyTask.ID-1].Status = model.TaskStatusDone
	loaded.Subtasks[legacyTask.ID-1].CheckpointScope = []string{"internal/service/legacy.go"}

	loaded.Subtasks[chunkTask.ID-1].Status = model.TaskStatusDone
	loaded.Subtasks[chunkTask.ID-1].CheckpointScope = nil
	loaded.Subtasks[chunkTask.ID-1].CheckpointChunks = []model.CheckpointChunk{
		{
			ID:       "chk_mixed",
			Path:     "internal/service/chunk.go",
			OldStart: 10,
			OldLines: 1,
			NewStart: 10,
			NewLines: 1,
		},
	}
	if err := svc.store.SaveCR(loaded); err != nil {
		t.Fatalf("SaveCR() error = %v", err)
	}

	reloaded, err := svc.store.LoadCR(cr.ID)
	if err != nil {
		t.Fatalf("LoadCR(reloaded) error = %v", err)
	}
	if len(reloaded.Subtasks[legacyTask.ID-1].CheckpointScope) != 1 {
		t.Fatalf("expected legacy checkpoint_scope preserved, got %#v", reloaded.Subtasks[legacyTask.ID-1])
	}
	if len(reloaded.Subtasks[chunkTask.ID-1].CheckpointChunks) != 1 {
		t.Fatalf("expected checkpoint_chunks preserved, got %#v", reloaded.Subtasks[chunkTask.ID-1])
	}

	report, err := svc.ValidateCR(cr.ID)
	if err != nil {
		t.Fatalf("ValidateCR() error = %v", err)
	}
	if !report.Valid {
		t.Fatalf("expected valid report for coexistence fixture, got errors=%#v warnings=%#v", report.Errors, report.Warnings)
	}
}

func TestRepairFromGitLegacyCommitWithoutBaseOrParentFootersStillReconstructs(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")

	runGit(t, dir, "commit", "--allow-empty",
		"-m", "[CR-2] Legacy intent",
		"-m", "Intent:\nLegacy why\n\nSubtasks:\n- [x] #1 Legacy task\n\nNotes:\n- legacy note\n\nMetadata:\n- actor: Test User <test@example.com>\n- merged_at: 2026-02-17T00:00:00Z\n\nSophia-CR: 2\nSophia-Intent: Legacy intent\nSophia-Tasks: 1 completed",
	)
	if err := os.RemoveAll(filepath.Join(svc.store.SophiaDir(), "cr")); err != nil {
		t.Fatalf("remove cr dir: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(svc.store.SophiaDir(), "cr"), 0o755); err != nil {
		t.Fatalf("recreate cr dir: %v", err)
	}

	if _, err := svc.RepairFromGit("main", false); err != nil {
		t.Fatalf("RepairFromGit() error = %v", err)
	}
	repaired, err := svc.store.LoadCR(2)
	if err != nil {
		t.Fatalf("LoadCR(2) error = %v", err)
	}
	if repaired.ParentCRID != 0 {
		t.Fatalf("expected missing parent footer to default to 0, got %#v", repaired)
	}
	if strings.TrimSpace(repaired.BaseRef) == "" || strings.TrimSpace(repaired.BaseCommit) == "" {
		t.Fatalf("expected base metadata backfilled for legacy repair, got %#v", repaired)
	}
}
