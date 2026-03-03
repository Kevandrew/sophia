package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"sophia/internal/service"
)

func TestCRImportAndPatchJSONCommands(t *testing.T) {
	t.Parallel()
	sourceDir := t.TempDir()
	sourceSvc := service.New(sourceDir)
	if _, err := sourceSvc.Init("main", ""); err != nil {
		t.Fatalf("Init(source) error = %v", err)
	}

	sourceCR, err := sourceSvc.AddCR("CLI collab", "source")
	if err != nil {
		t.Fatalf("AddCR(source) error = %v", err)
	}
	setServiceContract(t, sourceSvc, sourceCR.ID)
	bundle, payload, err := sourceSvc.ExportCRBundle(sourceCR.ID, service.ExportCROptions{Format: "json"})
	if err != nil {
		t.Fatalf("ExportCRBundle() error = %v", err)
	}

	targetDir := t.TempDir()
	targetSvc := service.New(targetDir)
	if _, err := targetSvc.Init("main", ""); err != nil {
		t.Fatalf("Init(target) error = %v", err)
	}

	bundlePath := filepath.Join(targetDir, "bundle.json")
	if err := os.WriteFile(bundlePath, payload, 0o644); err != nil {
		t.Fatalf("write bundle: %v", err)
	}

	importOut, _, importErr := runCLI(t, targetDir, "cr", "import", "--file", bundlePath, "--mode", "create", "--json")
	if importErr != nil {
		t.Fatalf("cr import --json error = %v\nout=%s", importErr, importOut)
	}
	importEnv := decodeEnvelope(t, importOut)
	if !importEnv.OK {
		t.Fatalf("expected import ok envelope, got %#v", importEnv)
	}
	localID, ok := importEnv.Data["local_cr_id"].(float64)
	if !ok || int(localID) <= 0 {
		t.Fatalf("expected local_cr_id in import data, got %#v", importEnv.Data)
	}
	uid := bundle.CRUID

	patchPayload := map[string]any{
		"schema_version": "sophia.cr_patch.v1",
		"target": map[string]any{
			"cr_uid": uid,
		},
		"ops": []any{
			map[string]any{
				"op":   "add_note",
				"text": "cli-json-note",
			},
		},
	}
	rawPatch, err := json.Marshal(patchPayload)
	if err != nil {
		t.Fatalf("Marshal(patch) error = %v", err)
	}
	patchPath := filepath.Join(targetDir, "patch.json")
	if err := os.WriteFile(patchPath, rawPatch, 0o644); err != nil {
		t.Fatalf("write patch: %v", err)
	}

	applyOut, _, applyErr := runCLI(t, targetDir, "cr", "patch", "apply", strconv.Itoa(int(localID)), "--file", patchPath, "--json")
	if applyErr != nil {
		t.Fatalf("cr patch apply --json error = %v\nout=%s", applyErr, applyOut)
	}
	applyEnv := decodeEnvelope(t, applyOut)
	if !applyEnv.OK {
		t.Fatalf("expected patch apply ok envelope, got %#v", applyEnv)
	}
	appliedOps, ok := applyEnv.Data["applied_ops"].([]any)
	if !ok || len(appliedOps) != 1 {
		t.Fatalf("expected one applied op, got %#v", applyEnv.Data["applied_ops"])
	}

	previewOut, _, previewErr := runCLI(t, targetDir, "cr", "patch", "preview", strconv.Itoa(int(localID)), "--file", patchPath, "--json")
	if previewErr != nil {
		t.Fatalf("cr patch preview --json error = %v\nout=%s", previewErr, previewOut)
	}
	previewEnv := decodeEnvelope(t, previewOut)
	if !previewEnv.OK {
		t.Fatalf("expected patch preview ok envelope, got %#v", previewEnv)
	}
	preview, ok := previewEnv.Data["preview"].(bool)
	if !ok || !preview {
		t.Fatalf("expected preview=true, got %#v", previewEnv.Data["preview"])
	}
}

func TestCRPatchPreviewJSONIncludesV2ConflictDetails(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := service.New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	cr, err := svc.AddCR("CLI v2 conflict", "preview conflict details")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	setServiceContract(t, svc, cr.ID)
	task, err := svc.AddTask(cr.ID, "task-one")
	if err != nil {
		t.Fatalf("AddTask() error = %v", err)
	}

	patchPayload := map[string]any{
		"schema_version": "sophia.cr_patch.v2",
		"target": map[string]any{
			"cr_uid": cr.UID,
		},
		"ops": []any{
			map[string]any{
				"op":      "delete_task",
				"task_id": task.ID,
				"before":  "wrong-title",
			},
		},
	}
	rawPatch, err := json.Marshal(patchPayload)
	if err != nil {
		t.Fatalf("Marshal(patch) error = %v", err)
	}
	patchPath := filepath.Join(dir, "patch-v2-conflict.json")
	if err := os.WriteFile(patchPath, rawPatch, 0o644); err != nil {
		t.Fatalf("write patch: %v", err)
	}

	out, _, runErr := runCLI(t, dir, "cr", "patch", "preview", strconv.Itoa(cr.ID), "--file", patchPath, "--json")
	if runErr == nil {
		t.Fatalf("expected preview command to fail with conflict")
	}
	env := decodeEnvelope(t, out)
	if env.OK {
		t.Fatalf("expected non-ok envelope, got %#v", env)
	}
	if env.Error == nil || env.Error.Code != "patch_conflict" {
		t.Fatalf("expected patch_conflict code, got %#v", env.Error)
	}
	conflicts, ok := env.Error.Details["conflicts"].([]any)
	if !ok || len(conflicts) == 0 {
		t.Fatalf("expected conflict details, got %#v", env.Error.Details)
	}
	first, ok := conflicts[0].(map[string]any)
	if !ok {
		t.Fatalf("expected conflict object, got %#v", conflicts[0])
	}
	if gotOp, _ := first["op"].(string); gotOp != "delete_task" {
		t.Fatalf("expected delete_task conflict op, got %#v", first)
	}
	if gotField, _ := first["field"].(string); gotField != "before" {
		t.Fatalf("expected before conflict field, got %#v", first)
	}
}

func TestCRImportMergeJSONIncludesDeterministicSummary(t *testing.T) {
	t.Parallel()
	sourceDir := t.TempDir()
	sourceSvc := service.New(sourceDir)
	if _, err := sourceSvc.Init("main", ""); err != nil {
		t.Fatalf("Init(source) error = %v", err)
	}
	sourceCR, err := sourceSvc.AddCR("CLI merge JSON", "source")
	if err != nil {
		t.Fatalf("AddCR(source) error = %v", err)
	}
	setServiceContract(t, sourceSvc, sourceCR.ID)
	_, payload, err := sourceSvc.ExportCRBundle(sourceCR.ID, service.ExportCROptions{Format: "json"})
	if err != nil {
		t.Fatalf("ExportCRBundle(source) error = %v", err)
	}

	targetDir := t.TempDir()
	targetSvc := service.New(targetDir)
	if _, err := targetSvc.Init("main", ""); err != nil {
		t.Fatalf("Init(target) error = %v", err)
	}
	bundlePath := filepath.Join(targetDir, "bundle.json")
	if err := os.WriteFile(bundlePath, payload, 0o644); err != nil {
		t.Fatalf("write bundle: %v", err)
	}
	if _, _, err := runCLI(t, targetDir, "cr", "import", "--file", bundlePath, "--mode", "create", "--json"); err != nil {
		t.Fatalf("cr import create --json error = %v", err)
	}

	if err := sourceSvc.AddNote(sourceCR.ID, "remote-merge-note"); err != nil {
		t.Fatalf("AddNote(source) error = %v", err)
	}
	_, updatedPayload, err := sourceSvc.ExportCRBundle(sourceCR.ID, service.ExportCROptions{Format: "json"})
	if err != nil {
		t.Fatalf("ExportCRBundle(updated source) error = %v", err)
	}
	if err := os.WriteFile(bundlePath, updatedPayload, 0o644); err != nil {
		t.Fatalf("rewrite bundle: %v", err)
	}

	out, _, runErr := runCLI(t, targetDir, "cr", "import", "--file", bundlePath, "--mode", "merge", "--json")
	if runErr != nil {
		t.Fatalf("cr import merge --json error = %v\nout=%s", runErr, out)
	}
	env := decodeEnvelope(t, out)
	if !env.OK {
		t.Fatalf("expected merge ok envelope, got %#v", env)
	}
	if merged, _ := env.Data["merged"].(bool); !merged {
		t.Fatalf("expected merged=true, got %#v", env.Data["merged"])
	}
	if applied, _ := env.Data["applied"].(bool); !applied {
		t.Fatalf("expected applied=true, got %#v", env.Data["applied"])
	}
	if conflicts, _ := env.Data["conflict_count"].(float64); int(conflicts) != 0 {
		t.Fatalf("expected conflict_count=0, got %#v", env.Data["conflict_count"])
	}
	changedFields, ok := env.Data["changed_fields"].([]any)
	if !ok || len(changedFields) == 0 {
		t.Fatalf("expected non-empty changed_fields, got %#v", env.Data["changed_fields"])
	}
	taskSummary, ok := env.Data["task_summary"].(map[string]any)
	if !ok {
		t.Fatalf("expected task_summary map, got %#v", env.Data["task_summary"])
	}
	if _, ok := taskSummary["added"].(float64); !ok {
		t.Fatalf("expected task_summary.added, got %#v", taskSummary)
	}
}

func TestCRImportMergePreviewCreateJSONLeavesIDUnset(t *testing.T) {
	t.Parallel()
	sourceDir := t.TempDir()
	sourceSvc := service.New(sourceDir)
	if _, err := sourceSvc.Init("main", ""); err != nil {
		t.Fatalf("Init(source) error = %v", err)
	}
	sourceCR, err := sourceSvc.AddCR("CLI merge preview create", "source")
	if err != nil {
		t.Fatalf("AddCR(source) error = %v", err)
	}
	setServiceContract(t, sourceSvc, sourceCR.ID)
	_, payload, err := sourceSvc.ExportCRBundle(sourceCR.ID, service.ExportCROptions{Format: "json"})
	if err != nil {
		t.Fatalf("ExportCRBundle(source) error = %v", err)
	}

	targetDir := t.TempDir()
	targetSvc := service.New(targetDir)
	if _, err := targetSvc.Init("main", ""); err != nil {
		t.Fatalf("Init(target) error = %v", err)
	}
	bundlePath := filepath.Join(targetDir, "bundle.json")
	if err := os.WriteFile(bundlePath, payload, 0o644); err != nil {
		t.Fatalf("write bundle: %v", err)
	}

	previewOut, _, previewErr := runCLI(t, targetDir, "cr", "import", "--file", bundlePath, "--mode", "merge", "--preview", "--json")
	if previewErr != nil {
		t.Fatalf("cr import merge --preview --json error = %v\nout=%s", previewErr, previewOut)
	}
	previewEnv := decodeEnvelope(t, previewOut)
	if !previewEnv.OK {
		t.Fatalf("expected preview ok envelope, got %#v", previewEnv)
	}
	if preview, _ := previewEnv.Data["preview"].(bool); !preview {
		t.Fatalf("expected preview=true, got %#v", previewEnv.Data["preview"])
	}
	if created, _ := previewEnv.Data["created"].(bool); !created {
		t.Fatalf("expected created=true on preview create, got %#v", previewEnv.Data["created"])
	}
	if localID, _ := previewEnv.Data["local_cr_id"].(float64); int(localID) != 0 {
		t.Fatalf("expected local_cr_id=0 on preview create, got %#v", previewEnv.Data["local_cr_id"])
	}
	if applied, _ := previewEnv.Data["applied"].(bool); applied {
		t.Fatalf("expected applied=false on preview create, got %#v", previewEnv.Data["applied"])
	}

	createOut, _, createErr := runCLI(t, targetDir, "cr", "import", "--file", bundlePath, "--mode", "create", "--json")
	if createErr != nil {
		t.Fatalf("expected create import to succeed after preview, err=%v out=%s", createErr, createOut)
	}
	createEnv := decodeEnvelope(t, createOut)
	if !createEnv.OK {
		t.Fatalf("expected create ok envelope, got %#v", createEnv)
	}
	if localID, _ := createEnv.Data["local_cr_id"].(float64); int(localID) != 1 {
		t.Fatalf("expected first persisted id to be 1, got %#v", createEnv.Data["local_cr_id"])
	}
}

func TestCRImportMergeJSONConflictIncludesStructuredDetails(t *testing.T) {
	t.Parallel()
	sourceDir := t.TempDir()
	sourceSvc := service.New(sourceDir)
	if _, err := sourceSvc.Init("main", ""); err != nil {
		t.Fatalf("Init(source) error = %v", err)
	}
	sourceCR, err := sourceSvc.AddCR("CLI merge conflict", "source")
	if err != nil {
		t.Fatalf("AddCR(source) error = %v", err)
	}
	setServiceContract(t, sourceSvc, sourceCR.ID)
	_, payload, err := sourceSvc.ExportCRBundle(sourceCR.ID, service.ExportCROptions{Format: "json"})
	if err != nil {
		t.Fatalf("ExportCRBundle(source) error = %v", err)
	}

	targetDir := t.TempDir()
	targetSvc := service.New(targetDir)
	if _, err := targetSvc.Init("main", ""); err != nil {
		t.Fatalf("Init(target) error = %v", err)
	}
	bundlePath := filepath.Join(targetDir, "bundle.json")
	if err := os.WriteFile(bundlePath, payload, 0o644); err != nil {
		t.Fatalf("write bundle: %v", err)
	}
	importOut, _, importErr := runCLI(t, targetDir, "cr", "import", "--file", bundlePath, "--mode", "create", "--json")
	if importErr != nil {
		t.Fatalf("cr import create --json error = %v\nout=%s", importErr, importOut)
	}
	importEnv := decodeEnvelope(t, importOut)
	localID, ok := importEnv.Data["local_cr_id"].(float64)
	if !ok {
		t.Fatalf("expected local_cr_id, got %#v", importEnv.Data)
	}

	localTitle := "Local title override"
	if _, err := targetSvc.EditCR(int(localID), &localTitle, nil); err != nil {
		t.Fatalf("EditCR(target) error = %v", err)
	}
	remoteTitle := "Remote title override"
	if _, err := sourceSvc.EditCR(sourceCR.ID, &remoteTitle, nil); err != nil {
		t.Fatalf("EditCR(source) error = %v", err)
	}
	_, updatedPayload, err := sourceSvc.ExportCRBundle(sourceCR.ID, service.ExportCROptions{Format: "json"})
	if err != nil {
		t.Fatalf("ExportCRBundle(updated source) error = %v", err)
	}
	if err := os.WriteFile(bundlePath, updatedPayload, 0o644); err != nil {
		t.Fatalf("rewrite bundle: %v", err)
	}

	out, _, runErr := runCLI(t, targetDir, "cr", "import", "--file", bundlePath, "--mode", "merge", "--json")
	if runErr == nil {
		t.Fatalf("expected merge command to fail on conflict")
	}
	env := decodeEnvelope(t, out)
	if env.OK {
		t.Fatalf("expected non-ok envelope, got %#v", env)
	}
	if env.Error == nil || env.Error.Code != "import_merge_conflict" {
		t.Fatalf("expected import_merge_conflict code, got %#v", env.Error)
	}
	if conflicts, _ := env.Error.Details["conflict_count"].(float64); int(conflicts) == 0 {
		t.Fatalf("expected conflict_count > 0, got %#v", env.Error.Details["conflict_count"])
	}
	conflictList, ok := env.Error.Details["conflicts"].([]any)
	if !ok || len(conflictList) == 0 {
		t.Fatalf("expected conflicts array in details, got %#v", env.Error.Details)
	}
}

func setServiceContract(t *testing.T, svc *service.Service, crID int) {
	t.Helper()
	why := "Contract"
	scope := []string{"."}
	nonGoals := []string{"none"}
	invariants := []string{"stable"}
	blast := "small"
	testPlan := "go test ./..."
	rollback := "revert"
	if _, err := svc.SetCRContract(crID, service.ContractPatch{
		Why:          &why,
		Scope:        &scope,
		NonGoals:     &nonGoals,
		Invariants:   &invariants,
		BlastRadius:  &blast,
		TestPlan:     &testPlan,
		RollbackPlan: &rollback,
	}); err != nil {
		t.Fatalf("SetCRContract() error = %v", err)
	}
}
