package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sophia/internal/service"
)

func TestCRExportWritesBundleFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := service.New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	cr, err := svc.AddCR("Export CLI", "bundle file output")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}

	why := "Export bundles for deterministic ingestion."
	scope := []string{"internal"}
	nonGoals := []string{"no upload"}
	invariants := []string{"deterministic output"}
	blast := "cli and service only"
	testPlan := "go test ./..."
	rollback := "revert"
	if _, err := svc.SetCRContract(cr.ID, service.ContractPatch{
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

	task, err := svc.AddTask(cr.ID, "feat: export fixture")
	if err != nil {
		t.Fatalf("AddTask() error = %v", err)
	}
	intent := "Create checkpoint for export diff."
	acceptance := []string{"checkpoint exists"}
	taskScope := []string{"export_cli.txt"}
	if _, err := svc.SetTaskContract(cr.ID, task.ID, service.TaskContractPatch{
		Intent:             &intent,
		AcceptanceCriteria: &acceptance,
		Scope:              &taskScope,
	}); err != nil {
		t.Fatalf("SetTaskContract() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "export_cli.txt"), []byte("cli\n"), 0o644); err != nil {
		t.Fatalf("write export_cli.txt: %v", err)
	}
	if _, err := svc.DoneTaskWithCheckpoint(cr.ID, task.ID, service.DoneTaskOptions{Checkpoint: true, FromContract: true}); err != nil {
		t.Fatalf("DoneTaskWithCheckpoint() error = %v", err)
	}

	outPath := filepath.Join(dir, "artifacts", "cr-1.bundle.json")
	out, _, runErr := runCLI(t, dir, "cr", "export", "1", "--format", "json", "--include", "diffs", "--out", outPath)
	if runErr != nil {
		t.Fatalf("cr export error = %v\noutput=%s", runErr, out)
	}
	if !strings.Contains(out, "Exported CR 1 bundle") {
		t.Fatalf("expected export success output, got %q", out)
	}

	raw, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read exported bundle: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("decode exported json: %v", err)
	}
	if got := strings.TrimSpace(payload["schema_version"].(string)); got == "" {
		t.Fatalf("expected schema_version, got %#v", payload)
	}
	includes, ok := payload["includes"].([]any)
	if !ok || len(includes) != 1 || includes[0].(string) != "diffs" {
		t.Fatalf("expected includes=[diffs], got %#v", payload["includes"])
	}
	if taskDiffs, ok := payload["task_diffs"].([]any); !ok || len(taskDiffs) < 1 {
		t.Fatalf("expected task_diffs in export payload, got %#v", payload["task_diffs"])
	}
}

func TestCRExportSupportsYAMLAndNDJSONOutput(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := service.New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	cr, err := svc.AddCR("Export formats CLI", "bundle file output")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}

	why := "Export bundles for deterministic ingestion."
	scope := []string{"internal"}
	nonGoals := []string{"no upload"}
	invariants := []string{"deterministic output"}
	blast := "cli and service only"
	testPlan := "go test ./..."
	rollback := "revert"
	if _, err := svc.SetCRContract(cr.ID, service.ContractPatch{
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

	for _, tc := range []struct {
		format string
		ext    string
	}{
		{format: "yaml", ext: ".yaml"},
		{format: "ndjson", ext: ".ndjson"},
	} {
		outPath := filepath.Join(dir, "artifacts", "cr-1.bundle"+tc.ext)
		out, _, runErr := runCLI(t, dir, "cr", "export", "1", "--format", tc.format, "--out", outPath)
		if runErr != nil {
			t.Fatalf("cr export (%s) error = %v\noutput=%s", tc.format, runErr, out)
		}
		if !strings.Contains(out, "Exported CR 1 bundle") {
			t.Fatalf("expected export success output for %s, got %q", tc.format, out)
		}
		raw, err := os.ReadFile(outPath)
		if err != nil {
			t.Fatalf("read exported %s bundle: %v", tc.format, err)
		}
		if len(raw) == 0 {
			t.Fatalf("expected non-empty %s export payload", tc.format)
		}
	}
}

func TestCRExportJSONEnvelopeIncludesPayloadTextForNonJSONFormats(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := service.New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	cr, err := svc.AddCR("Export JSON envelope", "non-json payload in --json envelope")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	why := "Export bundle envelope."
	scope := []string{"internal"}
	nonGoals := []string{"none"}
	invariants := []string{"stable"}
	blast := "cli only"
	testPlan := "go test ./..."
	rollback := "revert"
	if _, err := svc.SetCRContract(cr.ID, service.ContractPatch{
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

	out, _, runErr := runCLI(t, dir, "cr", "export", "1", "--format", "yaml", "--json")
	if runErr != nil {
		t.Fatalf("cr export --format yaml --json error = %v\noutput=%s", runErr, out)
	}
	env := decodeEnvelope(t, out)
	if !env.OK {
		t.Fatalf("expected ok envelope, got %#v", env)
	}
	if payloadText, ok := env.Data["payload_text"].(string); !ok || strings.TrimSpace(payloadText) == "" {
		t.Fatalf("expected non-empty payload_text for non-json format, got %#v", env.Data["payload_text"])
	}
	if _, ok := env.Data["bundle"]; !ok {
		t.Fatalf("expected bundle object in envelope, got %#v", env.Data)
	}
}
