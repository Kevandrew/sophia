package service

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"sophia/internal/model"
)

func TestExportIncludesFingerprintDeterministic(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	cr, err := svc.AddCR("Collab export", "fingerprint test")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	setValidContract(t, svc, cr.ID)

	first, _, err := svc.ExportCRBundle(cr.ID, ExportCROptions{Format: "json"})
	if err != nil {
		t.Fatalf("ExportCRBundle(first) error = %v", err)
	}
	second, _, err := svc.ExportCRBundle(cr.ID, ExportCROptions{Format: "json"})
	if err != nil {
		t.Fatalf("ExportCRBundle(second) error = %v", err)
	}
	if first.CRFingerprint == "" {
		t.Fatalf("expected non-empty cr_fingerprint")
	}
	if first.CRFingerprint != second.CRFingerprint {
		t.Fatalf("expected deterministic fingerprint %q, got %q", first.CRFingerprint, second.CRFingerprint)
	}
	if first.DocSchemaVersion != crDocSchemaV1 {
		t.Fatalf("expected doc schema %q, got %q", crDocSchemaV1, first.DocSchemaVersion)
	}
	if first.Doc == nil || first.Doc.UID == "" {
		t.Fatalf("expected doc payload with uid, got %#v", first.Doc)
	}
}

func TestPatchApplyNonOverlappingChangesAutoMerge(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	cr, err := svc.AddCR("Patch merge", "non-overlap")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	setValidContract(t, svc, cr.ID)
	loaded, err := svc.store.LoadCR(cr.ID)
	if err != nil {
		t.Fatalf("LoadCR() error = %v", err)
	}
	patch := map[string]any{
		"schema_version": patchSchemaV1,
		"target": map[string]any{
			"cr_uid": loaded.UID,
		},
		"base": map[string]any{
			"cr_fingerprint": mustCRFingerprint(t, loaded),
		},
		"ops": []any{
			map[string]any{
				"op": "set_contract",
				"changes": map[string]any{
					"why": map[string]any{
						"before": loaded.Contract.Why,
						"after":  "New collaboration why",
					},
				},
			},
			map[string]any{
				"op":   "add_note",
				"text": "From HQ suggestion",
			},
		},
	}
	payload, err := json.Marshal(patch)
	if err != nil {
		t.Fatalf("Marshal(patch) error = %v", err)
	}
	result, err := svc.ApplyCRPatch(strconv.Itoa(cr.ID), payload, false, false)
	if err != nil {
		t.Fatalf("ApplyCRPatch() error = %v", err)
	}
	if len(result.Conflicts) != 0 {
		t.Fatalf("expected no conflicts, got %#v", result.Conflicts)
	}
	if len(result.AppliedOps) != 2 {
		t.Fatalf("expected two applied ops, got %#v", result.AppliedOps)
	}
	reloaded, err := svc.store.LoadCR(cr.ID)
	if err != nil {
		t.Fatalf("LoadCR(reloaded) error = %v", err)
	}
	if reloaded.Contract.Why != "New collaboration why" {
		t.Fatalf("expected updated why, got %q", reloaded.Contract.Why)
	}
	if len(reloaded.Notes) == 0 || reloaded.Notes[len(reloaded.Notes)-1] != "From HQ suggestion" {
		t.Fatalf("expected note append, got %#v", reloaded.Notes)
	}
}

func TestPatchApplyConflictsOnStaleBefore(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	cr, err := svc.AddCR("Patch conflict", "stale before")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	setValidContract(t, svc, cr.ID)
	loaded, err := svc.store.LoadCR(cr.ID)
	if err != nil {
		t.Fatalf("LoadCR() error = %v", err)
	}

	updatedWhy := "changed locally first"
	if _, err := svc.SetCRContract(cr.ID, ContractPatch{Why: &updatedWhy}); err != nil {
		t.Fatalf("SetCRContract() error = %v", err)
	}

	patch := map[string]any{
		"schema_version": patchSchemaV1,
		"target": map[string]any{
			"cr_uid": loaded.UID,
		},
		"ops": []any{
			map[string]any{
				"op":     "set_field",
				"field":  "cr.title",
				"before": loaded.Title,
				"after":  "new title",
			},
			map[string]any{
				"op": "set_contract",
				"changes": map[string]any{
					"why": map[string]any{
						"before": loaded.Contract.Why,
						"after":  "patch why",
					},
				},
			},
		},
	}
	payload, err := json.Marshal(patch)
	if err != nil {
		t.Fatalf("Marshal(patch) error = %v", err)
	}
	_, err = svc.ApplyCRPatch(strconv.Itoa(cr.ID), payload, false, false)
	if err == nil {
		t.Fatalf("expected conflict error")
	}
	var conflictErr *PatchConflictError
	if !errors.As(err, &conflictErr) {
		t.Fatalf("expected PatchConflictError, got %T (%v)", err, err)
	}
	if conflictErr.Result == nil || len(conflictErr.Result.Conflicts) == 0 {
		t.Fatalf("expected non-empty conflicts in result, got %#v", conflictErr.Result)
	}
}

func TestPatchApplyDedupNotes(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	cr, err := svc.AddCR("Patch notes", "dedupe")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	setValidContract(t, svc, cr.ID)
	loaded, err := svc.store.LoadCR(cr.ID)
	if err != nil {
		t.Fatalf("LoadCR() error = %v", err)
	}

	patch := map[string]any{
		"schema_version": patchSchemaV1,
		"target": map[string]any{
			"cr_uid": loaded.UID,
		},
		"ops": []any{
			map[string]any{
				"op":   "add_note",
				"text": "dedupe-note",
			},
		},
	}
	payload, err := json.Marshal(patch)
	if err != nil {
		t.Fatalf("Marshal(patch) error = %v", err)
	}
	first, err := svc.ApplyCRPatch(strconv.Itoa(cr.ID), payload, false, false)
	if err != nil {
		t.Fatalf("ApplyCRPatch(first) error = %v", err)
	}
	second, err := svc.ApplyCRPatch(strconv.Itoa(cr.ID), payload, false, false)
	if err != nil {
		t.Fatalf("ApplyCRPatch(second) error = %v", err)
	}
	if len(first.AppliedOps) != 1 {
		t.Fatalf("expected first apply op, got %#v", first.AppliedOps)
	}
	if len(second.SkippedOps) != 1 {
		t.Fatalf("expected second skip op, got %#v", second.SkippedOps)
	}
	reloaded, err := svc.store.LoadCR(cr.ID)
	if err != nil {
		t.Fatalf("LoadCR(reloaded) error = %v", err)
	}
	count := 0
	for _, note := range reloaded.Notes {
		if note == "dedupe-note" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("expected note count 1, got %d (%#v)", count, reloaded.Notes)
	}
}

func TestPatchApplyAcceptsSchemaV2ForV1CompatibleOps(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	cr, err := svc.AddCR("Patch schema v2", "compat ops")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	setValidContract(t, svc, cr.ID)
	loaded, err := svc.store.LoadCR(cr.ID)
	if err != nil {
		t.Fatalf("LoadCR() error = %v", err)
	}

	patch := map[string]any{
		"schema_version": patchSchemaV2,
		"target": map[string]any{
			"cr_uid": loaded.UID,
		},
		"ops": []any{
			map[string]any{
				"op":   "add_note",
				"text": "v2 note",
			},
		},
	}
	payload, err := json.Marshal(patch)
	if err != nil {
		t.Fatalf("Marshal(patch) error = %v", err)
	}
	result, err := svc.ApplyCRPatch(strconv.Itoa(cr.ID), payload, false, false)
	if err != nil {
		t.Fatalf("ApplyCRPatch() error = %v", err)
	}
	if len(result.AppliedOps) != 1 {
		t.Fatalf("expected one applied op, got %#v", result.AppliedOps)
	}
}

func TestPatchApplyRejectsUnknownSchemaVersion(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	cr, err := svc.AddCR("Patch schema unknown", "reject unknown version")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	setValidContract(t, svc, cr.ID)
	loaded, err := svc.store.LoadCR(cr.ID)
	if err != nil {
		t.Fatalf("LoadCR() error = %v", err)
	}

	patch := map[string]any{
		"schema_version": "sophia.cr_patch.v999",
		"target": map[string]any{
			"cr_uid": loaded.UID,
		},
		"ops": []any{
			map[string]any{
				"op":   "add_note",
				"text": "ignored",
			},
		},
	}
	payload, err := json.Marshal(patch)
	if err != nil {
		t.Fatalf("Marshal(patch) error = %v", err)
	}
	_, err = svc.ApplyCRPatch(strconv.Itoa(cr.ID), payload, false, false)
	if err == nil {
		t.Fatalf("expected unknown schema version error")
	}
	if !strings.Contains(err.Error(), "invalid patch schema_version") {
		t.Fatalf("expected schema version error, got %v", err)
	}
}

func TestPatchApplyRejectsV2OnlyOpsInV1Schema(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	cr, err := svc.AddCR("Patch schema v1 gate", "reject v2 only ops")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	setValidContract(t, svc, cr.ID)
	loaded, err := svc.store.LoadCR(cr.ID)
	if err != nil {
		t.Fatalf("LoadCR() error = %v", err)
	}

	patch := map[string]any{
		"schema_version": patchSchemaV1,
		"target": map[string]any{
			"cr_uid": loaded.UID,
		},
		"ops": []any{
			map[string]any{
				"op":        "delete_note",
				"note_hash": "abc",
			},
		},
	}
	payload, err := json.Marshal(patch)
	if err != nil {
		t.Fatalf("Marshal(patch) error = %v", err)
	}
	_, err = svc.ApplyCRPatch(strconv.Itoa(cr.ID), payload, false, false)
	if err == nil {
		t.Fatalf("expected schema/op compatibility error")
	}
	if !strings.Contains(err.Error(), "does not support op") {
		t.Fatalf("expected schema op support error, got %v", err)
	}
}

func TestPatchApplyDeleteNoteWithStableBeforeMatch(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	cr, err := svc.AddCR("Patch delete note", "delete note op")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	setValidContract(t, svc, cr.ID)
	if err := svc.AddNote(cr.ID, "delete-me"); err != nil {
		t.Fatalf("AddNote() error = %v", err)
	}
	loaded, err := svc.store.LoadCR(cr.ID)
	if err != nil {
		t.Fatalf("LoadCR() error = %v", err)
	}

	patch := map[string]any{
		"schema_version": patchSchemaV2,
		"target": map[string]any{
			"cr_uid": loaded.UID,
		},
		"ops": []any{
			map[string]any{
				"op":        "delete_note",
				"note_hash": noteHash("delete-me"),
				"before":    "delete-me",
			},
		},
	}
	payload, err := json.Marshal(patch)
	if err != nil {
		t.Fatalf("Marshal(patch) error = %v", err)
	}
	result, err := svc.ApplyCRPatch(strconv.Itoa(cr.ID), payload, false, false)
	if err != nil {
		t.Fatalf("ApplyCRPatch() error = %v", err)
	}
	if len(result.AppliedOps) != 1 {
		t.Fatalf("expected one applied op, got %#v", result.AppliedOps)
	}
	reloaded, err := svc.store.LoadCR(cr.ID)
	if err != nil {
		t.Fatalf("LoadCR(reloaded) error = %v", err)
	}
	if len(reloaded.Notes) != 0 {
		t.Fatalf("expected note deleted, got %#v", reloaded.Notes)
	}
}

func TestPatchApplyDeleteTaskWithStableBeforeMatch(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	cr, err := svc.AddCR("Patch delete task", "delete task op")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	setValidContract(t, svc, cr.ID)
	task, err := svc.AddTask(cr.ID, "delete-task")
	if err != nil {
		t.Fatalf("AddTask() error = %v", err)
	}
	loaded, err := svc.store.LoadCR(cr.ID)
	if err != nil {
		t.Fatalf("LoadCR() error = %v", err)
	}

	patch := map[string]any{
		"schema_version": patchSchemaV2,
		"target": map[string]any{
			"cr_uid": loaded.UID,
		},
		"ops": []any{
			map[string]any{
				"op":      "delete_task",
				"task_id": task.ID,
				"before":  task.Title,
			},
		},
	}
	payload, err := json.Marshal(patch)
	if err != nil {
		t.Fatalf("Marshal(patch) error = %v", err)
	}
	result, err := svc.ApplyCRPatch(strconv.Itoa(cr.ID), payload, false, false)
	if err != nil {
		t.Fatalf("ApplyCRPatch() error = %v", err)
	}
	if len(result.AppliedOps) != 1 {
		t.Fatalf("expected one applied op, got %#v", result.AppliedOps)
	}
	reloaded, err := svc.store.LoadCR(cr.ID)
	if err != nil {
		t.Fatalf("LoadCR(reloaded) error = %v", err)
	}
	if len(reloaded.Subtasks) != 0 {
		t.Fatalf("expected task deleted, got %#v", reloaded.Subtasks)
	}
}

func TestPatchApplyReorderTaskDeterministic(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	cr, err := svc.AddCR("Patch reorder", "reorder tasks")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	setValidContract(t, svc, cr.ID)
	t1, err := svc.AddTask(cr.ID, "task-1")
	if err != nil {
		t.Fatalf("AddTask(1) error = %v", err)
	}
	t2, err := svc.AddTask(cr.ID, "task-2")
	if err != nil {
		t.Fatalf("AddTask(2) error = %v", err)
	}
	t3, err := svc.AddTask(cr.ID, "task-3")
	if err != nil {
		t.Fatalf("AddTask(3) error = %v", err)
	}
	loaded, err := svc.store.LoadCR(cr.ID)
	if err != nil {
		t.Fatalf("LoadCR() error = %v", err)
	}

	patch := map[string]any{
		"schema_version": patchSchemaV2,
		"target": map[string]any{
			"cr_uid": loaded.UID,
		},
		"ops": []any{
			map[string]any{
				"op":       "reorder_task",
				"before":   []int{t1.ID, t2.ID, t3.ID},
				"task_ids": []int{t3.ID, t1.ID, t2.ID},
			},
		},
	}
	payload, err := json.Marshal(patch)
	if err != nil {
		t.Fatalf("Marshal(patch) error = %v", err)
	}
	result, err := svc.ApplyCRPatch(strconv.Itoa(cr.ID), payload, false, false)
	if err != nil {
		t.Fatalf("ApplyCRPatch() error = %v", err)
	}
	if len(result.AppliedOps) != 1 {
		t.Fatalf("expected one applied op, got %#v", result.AppliedOps)
	}
	reloaded, err := svc.store.LoadCR(cr.ID)
	if err != nil {
		t.Fatalf("LoadCR(reloaded) error = %v", err)
	}
	gotOrder := currentTaskOrder(reloaded.Subtasks)
	expectedOrder := []int{t3.ID, t1.ID, t2.ID}
	if len(gotOrder) != len(expectedOrder) || gotOrder[0] != expectedOrder[0] || gotOrder[1] != expectedOrder[1] || gotOrder[2] != expectedOrder[2] {
		t.Fatalf("expected task order %#v, got %#v", expectedOrder, gotOrder)
	}
}

func TestPatchApplyReorderTaskInvalidPayloadReturnsConflict(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	cr, err := svc.AddCR("Patch reorder invalid", "invalid reorder payload")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	setValidContract(t, svc, cr.ID)
	t1, err := svc.AddTask(cr.ID, "task-1")
	if err != nil {
		t.Fatalf("AddTask(1) error = %v", err)
	}
	t2, err := svc.AddTask(cr.ID, "task-2")
	if err != nil {
		t.Fatalf("AddTask(2) error = %v", err)
	}
	loaded, err := svc.store.LoadCR(cr.ID)
	if err != nil {
		t.Fatalf("LoadCR() error = %v", err)
	}

	patch := map[string]any{
		"schema_version": patchSchemaV2,
		"target": map[string]any{
			"cr_uid": loaded.UID,
		},
		"ops": []any{
			map[string]any{
				"op":       "reorder_task",
				"before":   []int{t1.ID, t2.ID},
				"task_ids": []int{t1.ID, t1.ID},
			},
		},
	}
	payload, err := json.Marshal(patch)
	if err != nil {
		t.Fatalf("Marshal(patch) error = %v", err)
	}
	_, err = svc.ApplyCRPatch(strconv.Itoa(cr.ID), payload, false, false)
	if err == nil {
		t.Fatalf("expected conflict error")
	}
	var conflictErr *PatchConflictError
	if !errors.As(err, &conflictErr) {
		t.Fatalf("expected PatchConflictError, got %T (%v)", err, err)
	}
	if conflictErr.Result == nil || len(conflictErr.Result.Conflicts) == 0 {
		t.Fatalf("expected structured conflicts, got %#v", conflictErr.Result)
	}
	reloaded, err := svc.store.LoadCR(cr.ID)
	if err != nil {
		t.Fatalf("LoadCR(reloaded) error = %v", err)
	}
	if got := currentTaskOrder(reloaded.Subtasks); len(got) != 2 || got[0] != t1.ID || got[1] != t2.ID {
		t.Fatalf("expected task order unchanged, got %#v", got)
	}
}

func TestPatchApplySetFieldSupportsStatus(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	cr, err := svc.AddCR("Patch status", "set_field status")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	setValidContract(t, svc, cr.ID)
	loaded, err := svc.store.LoadCR(cr.ID)
	if err != nil {
		t.Fatalf("LoadCR() error = %v", err)
	}

	patch := map[string]any{
		"schema_version": patchSchemaV2,
		"target": map[string]any{
			"cr_uid": loaded.UID,
		},
		"ops": []any{
			map[string]any{
				"op":     "set_field",
				"field":  "cr.status",
				"before": model.StatusInProgress,
				"after":  model.StatusAbandoned,
			},
		},
	}
	payload, err := json.Marshal(patch)
	if err != nil {
		t.Fatalf("Marshal(patch) error = %v", err)
	}
	if _, err := svc.ApplyCRPatch(strconv.Itoa(cr.ID), payload, false, false); err != nil {
		t.Fatalf("ApplyCRPatch() error = %v", err)
	}
	reloaded, err := svc.store.LoadCR(cr.ID)
	if err != nil {
		t.Fatalf("LoadCR(reloaded) error = %v", err)
	}
	if reloaded.Status != model.StatusAbandoned {
		t.Fatalf("expected status updated to %q, got %q", model.StatusAbandoned, reloaded.Status)
	}
}

func TestPatchApplySetFieldTypeMismatchIsDeterministic(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	cr, err := svc.AddCR("Patch type mismatch", "set_field type mismatch")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	setValidContract(t, svc, cr.ID)
	loaded, err := svc.store.LoadCR(cr.ID)
	if err != nil {
		t.Fatalf("LoadCR() error = %v", err)
	}

	patch := map[string]any{
		"schema_version": patchSchemaV2,
		"target": map[string]any{
			"cr_uid": loaded.UID,
		},
		"ops": []any{
			map[string]any{
				"op":     "set_field",
				"field":  "cr.parent_cr_id",
				"before": nil,
				"after":  "not-an-int",
			},
		},
	}
	payload, err := json.Marshal(patch)
	if err != nil {
		t.Fatalf("Marshal(patch) error = %v", err)
	}
	_, err = svc.ApplyCRPatch(strconv.Itoa(cr.ID), payload, false, false)
	if err == nil {
		t.Fatalf("expected type mismatch error")
	}
	if !strings.Contains(err.Error(), "after decode") {
		t.Fatalf("expected typed decode error, got %v", err)
	}
}

func TestPatchApplyConflictsDoNotPartiallyWriteForV2Ops(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	cr, err := svc.AddCR("Patch no partial writes", "conflicts block all writes")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	setValidContract(t, svc, cr.ID)
	task, err := svc.AddTask(cr.ID, "task-1")
	if err != nil {
		t.Fatalf("AddTask() error = %v", err)
	}
	loaded, err := svc.store.LoadCR(cr.ID)
	if err != nil {
		t.Fatalf("LoadCR() error = %v", err)
	}

	patch := map[string]any{
		"schema_version": patchSchemaV2,
		"target": map[string]any{
			"cr_uid": loaded.UID,
		},
		"ops": []any{
			map[string]any{
				"op":   "add_note",
				"text": "would-have-been-added",
			},
			map[string]any{
				"op":      "delete_task",
				"task_id": task.ID,
				"before":  "wrong-title",
			},
		},
	}
	payload, err := json.Marshal(patch)
	if err != nil {
		t.Fatalf("Marshal(patch) error = %v", err)
	}
	_, err = svc.ApplyCRPatch(strconv.Itoa(cr.ID), payload, false, false)
	if err == nil {
		t.Fatalf("expected conflict error")
	}
	var conflictErr *PatchConflictError
	if !errors.As(err, &conflictErr) {
		t.Fatalf("expected PatchConflictError, got %T (%v)", err, err)
	}
	reloaded, err := svc.store.LoadCR(cr.ID)
	if err != nil {
		t.Fatalf("LoadCR(reloaded) error = %v", err)
	}
	if len(reloaded.Notes) != 0 {
		t.Fatalf("expected notes unchanged on conflict, got %#v", reloaded.Notes)
	}
	if len(reloaded.Subtasks) != 1 || reloaded.Subtasks[0].ID != task.ID {
		t.Fatalf("expected tasks unchanged on conflict, got %#v", reloaded.Subtasks)
	}
}

func TestImportCreateAndReplaceByUID(t *testing.T) {
	t.Parallel()
	sourceDir := t.TempDir()
	sourceSvc := New(sourceDir)
	if _, err := sourceSvc.Init("main", ""); err != nil {
		t.Fatalf("source Init() error = %v", err)
	}

	sourceCR, err := sourceSvc.AddCR("Import source", "bundle source")
	if err != nil {
		t.Fatalf("source AddCR() error = %v", err)
	}
	setValidContract(t, sourceSvc, sourceCR.ID)
	bundle, payload, err := sourceSvc.ExportCRBundle(sourceCR.ID, ExportCROptions{Format: "json"})
	if err != nil {
		t.Fatalf("source ExportCRBundle() error = %v", err)
	}

	targetDir := t.TempDir()
	targetSvc := New(targetDir)
	if _, err := targetSvc.Init("main", ""); err != nil {
		t.Fatalf("target Init() error = %v", err)
	}

	bundlePath := filepath.Join(targetDir, "import.bundle.json")
	if err := os.WriteFile(bundlePath, payload, 0o644); err != nil {
		t.Fatalf("write bundle file: %v", err)
	}
	createResult, err := targetSvc.ImportCRBundle(ImportCRBundleOptions{FilePath: bundlePath, Mode: "create"})
	if err != nil {
		t.Fatalf("ImportCRBundle(create) error = %v", err)
	}
	if !createResult.Created || createResult.Replaced {
		t.Fatalf("expected created=true replaced=false, got %#v", createResult)
	}

	bundle.Doc.Title = "Import source updated"
	updatedPayload, err := json.Marshal(bundle)
	if err != nil {
		t.Fatalf("Marshal(updated bundle) error = %v", err)
	}
	if err := os.WriteFile(bundlePath, updatedPayload, 0o644); err != nil {
		t.Fatalf("rewrite bundle file: %v", err)
	}
	replaceResult, err := targetSvc.ImportCRBundle(ImportCRBundleOptions{FilePath: bundlePath, Mode: "replace"})
	if err != nil {
		t.Fatalf("ImportCRBundle(replace) error = %v", err)
	}
	if replaceResult.LocalCRID != createResult.LocalCRID {
		t.Fatalf("expected replace to preserve local id %d, got %d", createResult.LocalCRID, replaceResult.LocalCRID)
	}
	reloaded, err := targetSvc.store.LoadCR(createResult.LocalCRID)
	if err != nil {
		t.Fatalf("LoadCR(imported) error = %v", err)
	}
	if reloaded.Title != "Import source updated" {
		t.Fatalf("expected replaced title, got %q", reloaded.Title)
	}
}

func TestStatusCRImportedMetadataOnlyDoesNotFail(t *testing.T) {
	t.Parallel()
	sourceDir := t.TempDir()
	sourceSvc := New(sourceDir)
	if _, err := sourceSvc.Init("main", ""); err != nil {
		t.Fatalf("source Init() error = %v", err)
	}
	sourceCR, err := sourceSvc.AddCR("Imported status fallback", "status should not hard-fail")
	if err != nil {
		t.Fatalf("source AddCR() error = %v", err)
	}
	setValidContract(t, sourceSvc, sourceCR.ID)
	_, payload, err := sourceSvc.ExportCRBundle(sourceCR.ID, ExportCROptions{Format: "json"})
	if err != nil {
		t.Fatalf("source ExportCRBundle() error = %v", err)
	}

	targetDir := t.TempDir()
	targetSvc := New(targetDir)
	if _, err := targetSvc.Init("main", ""); err != nil {
		t.Fatalf("target Init() error = %v", err)
	}

	bundlePath := filepath.Join(targetDir, "import.bundle.json")
	if err := os.WriteFile(bundlePath, payload, 0o644); err != nil {
		t.Fatalf("write bundle file: %v", err)
	}
	createResult, err := targetSvc.ImportCRBundle(ImportCRBundleOptions{FilePath: bundlePath, Mode: "create"})
	if err != nil {
		t.Fatalf("ImportCRBundle(create) error = %v", err)
	}

	status, err := targetSvc.StatusCR(createResult.LocalCRID)
	if err != nil {
		t.Fatalf("StatusCR(imported) error = %v", err)
	}
	if status.ValidationValid {
		t.Fatalf("expected imported metadata-only status to be non-valid until branch context is available")
	}
	if status.ValidationErrors == 0 {
		t.Fatalf("expected validation errors for missing branch context, got %#v", status)
	}
	if !status.MergeBlocked {
		t.Fatalf("expected merge blocked when status is metadata-only")
	}
	found := false
	for _, blocker := range status.MergeBlockers {
		if strings.Contains(strings.ToLower(blocker), "branch context is unavailable") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected missing branch context merge blocker, got %#v", status.MergeBlockers)
	}
}

func TestValidateCRImportedMetadataOnlyReturnsStructuredReport(t *testing.T) {
	t.Parallel()
	sourceDir := t.TempDir()
	sourceSvc := New(sourceDir)
	if _, err := sourceSvc.Init("main", ""); err != nil {
		t.Fatalf("source Init() error = %v", err)
	}
	sourceCR, err := sourceSvc.AddCR("Imported validate fallback", "validate should not hard-fail")
	if err != nil {
		t.Fatalf("source AddCR() error = %v", err)
	}
	setValidContract(t, sourceSvc, sourceCR.ID)
	_, payload, err := sourceSvc.ExportCRBundle(sourceCR.ID, ExportCROptions{Format: "json"})
	if err != nil {
		t.Fatalf("source ExportCRBundle() error = %v", err)
	}

	targetDir := t.TempDir()
	targetSvc := New(targetDir)
	if _, err := targetSvc.Init("main", ""); err != nil {
		t.Fatalf("target Init() error = %v", err)
	}
	bundlePath := filepath.Join(targetDir, "import.bundle.json")
	if err := os.WriteFile(bundlePath, payload, 0o644); err != nil {
		t.Fatalf("write bundle file: %v", err)
	}
	createResult, err := targetSvc.ImportCRBundle(ImportCRBundleOptions{FilePath: bundlePath, Mode: "create"})
	if err != nil {
		t.Fatalf("ImportCRBundle(create) error = %v", err)
	}

	report, err := targetSvc.ValidateCR(createResult.LocalCRID)
	if err != nil {
		t.Fatalf("ValidateCR(imported) error = %v", err)
	}
	if report.Valid {
		t.Fatalf("expected metadata-only validate result to be non-valid")
	}
	if len(report.Errors) == 0 {
		t.Fatalf("expected validation errors for missing branch context, got %#v", report)
	}
	foundWarning := false
	for _, warning := range report.Warnings {
		if strings.Contains(strings.ToLower(warning), "branch context unavailable") {
			foundWarning = true
			break
		}
	}
	if !foundWarning {
		t.Fatalf("expected branch context warning, got %#v", report.Warnings)
	}
}

func TestImpactCRImportedMetadataOnlyReturnsStructuredReport(t *testing.T) {
	t.Parallel()
	sourceDir := t.TempDir()
	sourceSvc := New(sourceDir)
	if _, err := sourceSvc.Init("main", ""); err != nil {
		t.Fatalf("source Init() error = %v", err)
	}
	sourceCR, err := sourceSvc.AddCR("Imported impact fallback", "impact should not hard-fail")
	if err != nil {
		t.Fatalf("source AddCR() error = %v", err)
	}
	setValidContract(t, sourceSvc, sourceCR.ID)
	_, payload, err := sourceSvc.ExportCRBundle(sourceCR.ID, ExportCROptions{Format: "json"})
	if err != nil {
		t.Fatalf("source ExportCRBundle() error = %v", err)
	}

	targetDir := t.TempDir()
	targetSvc := New(targetDir)
	if _, err := targetSvc.Init("main", ""); err != nil {
		t.Fatalf("target Init() error = %v", err)
	}
	bundlePath := filepath.Join(targetDir, "import.bundle.json")
	if err := os.WriteFile(bundlePath, payload, 0o644); err != nil {
		t.Fatalf("write bundle file: %v", err)
	}
	createResult, err := targetSvc.ImportCRBundle(ImportCRBundleOptions{FilePath: bundlePath, Mode: "create"})
	if err != nil {
		t.Fatalf("ImportCRBundle(create) error = %v", err)
	}

	report, err := targetSvc.ImpactCR(createResult.LocalCRID)
	if err != nil {
		t.Fatalf("ImpactCR(imported) error = %v", err)
	}
	if report == nil {
		t.Fatalf("expected impact report payload")
	}
	foundWarning := false
	for _, warning := range report.Warnings {
		if strings.Contains(strings.ToLower(warning), "branch context unavailable") {
			foundWarning = true
			break
		}
	}
	if !foundWarning {
		t.Fatalf("expected branch context warning, got %#v", report.Warnings)
	}
}

func TestImpactCRImportedMetadataOnlyDerivesChangesFromTaskCheckpointScope(t *testing.T) {
	t.Parallel()
	sourceDir := t.TempDir()
	sourceSvc := New(sourceDir)
	if _, err := sourceSvc.Init("main", ""); err != nil {
		t.Fatalf("source Init() error = %v", err)
	}
	sourceCR, err := sourceSvc.AddCR("Imported impact derived scope", "impact should use checkpoint scope fallback")
	if err != nil {
		t.Fatalf("source AddCR() error = %v", err)
	}
	setValidContract(t, sourceSvc, sourceCR.ID)

	loadedSourceCR, err := sourceSvc.store.LoadCR(sourceCR.ID)
	if err != nil {
		t.Fatalf("source LoadCR() error = %v", err)
	}
	now := sourceSvc.timestamp()
	actor := sourceSvc.git.Actor()
	loadedSourceCR.Subtasks = append(loadedSourceCR.Subtasks, model.Subtask{
		ID:          1,
		Title:       "done task with checkpoint scope",
		Status:      model.TaskStatusDone,
		CreatedAt:   now,
		UpdatedAt:   now,
		CompletedAt: now,
		CreatedBy:   actor,
		CompletedBy: actor,
		CheckpointScope: []string{
			"docs/spec.md",
		},
		Contract: model.TaskContract{
			Intent:             "record fallback scope",
			AcceptanceCriteria: []string{"impact derives changed paths"},
			Scope:              []string{"docs/"},
		},
	})
	loadedSourceCR.UpdatedAt = now
	if err := sourceSvc.store.SaveCR(loadedSourceCR); err != nil {
		t.Fatalf("source SaveCR() error = %v", err)
	}

	_, payload, err := sourceSvc.ExportCRBundle(sourceCR.ID, ExportCROptions{Format: "json"})
	if err != nil {
		t.Fatalf("source ExportCRBundle() error = %v", err)
	}

	targetDir := t.TempDir()
	targetSvc := New(targetDir)
	if _, err := targetSvc.Init("main", ""); err != nil {
		t.Fatalf("target Init() error = %v", err)
	}
	bundlePath := filepath.Join(targetDir, "import.bundle.json")
	if err := os.WriteFile(bundlePath, payload, 0o644); err != nil {
		t.Fatalf("write bundle file: %v", err)
	}
	createResult, err := targetSvc.ImportCRBundle(ImportCRBundleOptions{FilePath: bundlePath, Mode: "create"})
	if err != nil {
		t.Fatalf("ImportCRBundle(create) error = %v", err)
	}

	report, err := targetSvc.ImpactCR(createResult.LocalCRID)
	if err != nil {
		t.Fatalf("ImpactCR(imported) error = %v", err)
	}
	if report == nil {
		t.Fatalf("expected impact report payload")
	}
	if report.FilesChanged != 1 {
		t.Fatalf("expected fallback files_changed=1 from checkpoint scope, got %d (%#v)", report.FilesChanged, report)
	}
	if len(report.ModifiedFiles) != 1 || report.ModifiedFiles[0] != "docs/spec.md" {
		t.Fatalf("expected fallback modified_files [docs/spec.md], got %#v", report.ModifiedFiles)
	}
}

func TestReviewCRImportedMetadataOnlyDoesNotFail(t *testing.T) {
	t.Parallel()
	sourceDir := t.TempDir()
	sourceSvc := New(sourceDir)
	if _, err := sourceSvc.Init("main", ""); err != nil {
		t.Fatalf("source Init() error = %v", err)
	}
	sourceCR, err := sourceSvc.AddCR("Imported review fallback", "review should not hard-fail")
	if err != nil {
		t.Fatalf("source AddCR() error = %v", err)
	}
	setValidContract(t, sourceSvc, sourceCR.ID)
	_, payload, err := sourceSvc.ExportCRBundle(sourceCR.ID, ExportCROptions{Format: "json"})
	if err != nil {
		t.Fatalf("source ExportCRBundle() error = %v", err)
	}

	targetDir := t.TempDir()
	targetSvc := New(targetDir)
	if _, err := targetSvc.Init("main", ""); err != nil {
		t.Fatalf("target Init() error = %v", err)
	}
	bundlePath := filepath.Join(targetDir, "import.bundle.json")
	if err := os.WriteFile(bundlePath, payload, 0o644); err != nil {
		t.Fatalf("write bundle file: %v", err)
	}
	createResult, err := targetSvc.ImportCRBundle(ImportCRBundleOptions{FilePath: bundlePath, Mode: "create"})
	if err != nil {
		t.Fatalf("ImportCRBundle(create) error = %v", err)
	}

	review, err := targetSvc.ReviewCR(createResult.LocalCRID)
	if err != nil {
		t.Fatalf("ReviewCR(imported) error = %v", err)
	}
	if review == nil {
		t.Fatalf("expected review payload")
	}
	if len(review.Files) != 0 {
		t.Fatalf("expected empty file list for metadata-only review, got %#v", review.Files)
	}
	foundValidationWarning := false
	for _, warning := range review.ValidationWarnings {
		if strings.Contains(strings.ToLower(warning), "branch context unavailable") {
			foundValidationWarning = true
			break
		}
	}
	if !foundValidationWarning {
		t.Fatalf("expected validation warning for metadata-only review, got %#v", review.ValidationWarnings)
	}
}

func TestSwitchCRImportedMetadataOnlyFallsBackToLocalBase(t *testing.T) {
	t.Parallel()
	sourceDir := t.TempDir()
	sourceSvc := New(sourceDir)
	if _, err := sourceSvc.Init("main", ""); err != nil {
		t.Fatalf("source Init() error = %v", err)
	}
	sourceCR, err := sourceSvc.AddCR("Imported switch fallback", "switch should use local base anchor")
	if err != nil {
		t.Fatalf("source AddCR() error = %v", err)
	}
	setValidContract(t, sourceSvc, sourceCR.ID)
	_, payload, err := sourceSvc.ExportCRBundle(sourceCR.ID, ExportCROptions{Format: "json"})
	if err != nil {
		t.Fatalf("source ExportCRBundle() error = %v", err)
	}

	targetDir := t.TempDir()
	targetSvc := New(targetDir)
	if _, err := targetSvc.Init("main", ""); err != nil {
		t.Fatalf("target Init() error = %v", err)
	}
	runGit(t, targetDir, "-c", "user.name=Test User", "-c", "user.email=test@example.com", "commit", "--allow-empty", "-m", "target base commit")

	bundlePath := filepath.Join(targetDir, "import.bundle.json")
	if err := os.WriteFile(bundlePath, payload, 0o644); err != nil {
		t.Fatalf("write bundle file: %v", err)
	}
	createResult, err := targetSvc.ImportCRBundle(ImportCRBundleOptions{FilePath: bundlePath, Mode: "create"})
	if err != nil {
		t.Fatalf("ImportCRBundle(create) error = %v", err)
	}
	if err := os.Remove(bundlePath); err != nil {
		t.Fatalf("remove bundle file before switch: %v", err)
	}

	crBefore, err := targetSvc.store.LoadCR(createResult.LocalCRID)
	if err != nil {
		t.Fatalf("LoadCR(before switch) error = %v", err)
	}
	if targetSvc.git.BranchExists(crBefore.Branch) {
		t.Fatalf("expected imported branch %q to be absent before switch", crBefore.Branch)
	}

	switched, err := targetSvc.SwitchCR(createResult.LocalCRID)
	if err != nil {
		t.Fatalf("SwitchCR(imported) error = %v", err)
	}
	current, err := targetSvc.git.CurrentBranch()
	if err != nil {
		t.Fatalf("CurrentBranch() error = %v", err)
	}
	if current != switched.Branch {
		t.Fatalf("expected current branch %q, got %q", switched.Branch, current)
	}
	if !targetSvc.git.BranchExists(switched.Branch) {
		t.Fatalf("expected switched branch %q to exist", switched.Branch)
	}

	mainHead, err := targetSvc.git.ResolveRef("main")
	if err != nil {
		t.Fatalf("ResolveRef(main) error = %v", err)
	}
	reloaded, err := targetSvc.store.LoadCR(createResult.LocalCRID)
	if err != nil {
		t.Fatalf("LoadCR(after switch) error = %v", err)
	}
	if strings.TrimSpace(reloaded.BaseCommit) != strings.TrimSpace(mainHead) {
		t.Fatalf("expected BaseCommit to be rewritten to local base anchor %q, got %q", mainHead, reloaded.BaseCommit)
	}
}

func TestSwitchCRImportedMetadataOnlyWithoutLocalAnchorReturnsActionableError(t *testing.T) {
	t.Parallel()
	sourceDir := t.TempDir()
	sourceSvc := New(sourceDir)
	if _, err := sourceSvc.Init("main", ""); err != nil {
		t.Fatalf("source Init() error = %v", err)
	}
	sourceCR, err := sourceSvc.AddCR("Imported switch failure", "no local base anchor")
	if err != nil {
		t.Fatalf("source AddCR() error = %v", err)
	}
	setValidContract(t, sourceSvc, sourceCR.ID)
	_, payload, err := sourceSvc.ExportCRBundle(sourceCR.ID, ExportCROptions{Format: "json"})
	if err != nil {
		t.Fatalf("source ExportCRBundle() error = %v", err)
	}

	targetDir := t.TempDir()
	targetSvc := New(targetDir)
	if _, err := targetSvc.Init("main", ""); err != nil {
		t.Fatalf("target Init() error = %v", err)
	}

	bundlePath := filepath.Join(targetDir, "import.bundle.json")
	if err := os.WriteFile(bundlePath, payload, 0o644); err != nil {
		t.Fatalf("write bundle file: %v", err)
	}
	createResult, err := targetSvc.ImportCRBundle(ImportCRBundleOptions{FilePath: bundlePath, Mode: "create"})
	if err != nil {
		t.Fatalf("ImportCRBundle(create) error = %v", err)
	}
	if err := os.Remove(bundlePath); err != nil {
		t.Fatalf("remove bundle file before switch: %v", err)
	}

	_, err = targetSvc.SwitchCR(createResult.LocalCRID)
	if err == nil {
		t.Fatalf("expected SwitchCR() to fail when no local base anchor exists")
	}
	msg := strings.ToLower(err.Error())
	if !strings.Contains(msg, "unable to resolve cr") || !strings.Contains(msg, "base anchor") {
		t.Fatalf("expected actionable base-anchor error, got %v", err)
	}
	if strings.Contains(msg, "unable to read tree") {
		t.Fatalf("expected no raw git tree error, got %v", err)
	}
}

func mustCRFingerprint(t *testing.T, cr *model.CR) string {
	t.Helper()
	doc := canonicalCRDoc(cr)
	fingerprint, err := fingerprintCRDoc(doc)
	if err != nil {
		t.Fatalf("fingerprintCRDoc() error = %v", err)
	}
	return fingerprint
}
