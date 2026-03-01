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
