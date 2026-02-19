package service

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"sophia/internal/model"
)

func TestExportIncludesFingerprintDeterministic(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")

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
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")

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
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")

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
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")

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

func TestImportCreateAndReplaceByUID(t *testing.T) {
	sourceDir := t.TempDir()
	sourceSvc := New(sourceDir)
	if _, err := sourceSvc.Init("main", ""); err != nil {
		t.Fatalf("source Init() error = %v", err)
	}
	runGit(t, sourceDir, "config", "user.name", "Test User")
	runGit(t, sourceDir, "config", "user.email", "test@example.com")

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
	runGit(t, targetDir, "config", "user.name", "Test User")
	runGit(t, targetDir, "config", "user.email", "test@example.com")

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

func mustCRFingerprint(t *testing.T, cr *model.CR) string {
	t.Helper()
	doc := canonicalCRDoc(cr)
	fingerprint, err := fingerprintCRDoc(doc)
	if err != nil {
		t.Fatalf("fingerprintCRDoc() error = %v", err)
	}
	return fingerprint
}
