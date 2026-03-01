package cli

import (
	"os"
	"path/filepath"
	"testing"

	"sophia/internal/service"
)

func TestCREvidenceAddAndShowJSON(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := service.New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if _, err := svc.AddCR("Evidence JSON", "add/show evidence"); err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "evidence.txt"), []byte("artifact\n"), 0o644); err != nil {
		t.Fatalf("write attachment: %v", err)
	}

	out, _, runErr := runCLI(t, dir, "cr", "evidence", "add", "1",
		"--type", "manual_note",
		"--text", "Reviewed parsing edge case.",
		"--scope", "internal/service/service_cr.go",
		"--attachment", "evidence.txt",
		"--json")
	if runErr != nil {
		t.Fatalf("cr evidence add --json error = %v\noutput=%s", runErr, out)
	}
	env := decodeEnvelope(t, out)
	if !env.OK {
		t.Fatalf("expected ok response, got %#v", env)
	}
	evidence, ok := env.Data["evidence"].(map[string]any)
	if !ok {
		t.Fatalf("expected evidence object, got %#v", env.Data["evidence"])
	}
	if evidence["type"] != "manual_note" {
		t.Fatalf("expected manual_note type, got %#v", evidence["type"])
	}

	out, _, runErr = runCLI(t, dir, "cr", "evidence", "show", "1", "--json")
	if runErr != nil {
		t.Fatalf("cr evidence show --json error = %v\noutput=%s", runErr, out)
	}
	env = decodeEnvelope(t, out)
	if !env.OK {
		t.Fatalf("expected ok response, got %#v", env)
	}
	count, ok := env.Data["count"].(float64)
	if !ok || count != 1 {
		t.Fatalf("expected count=1, got %#v", env.Data["count"])
	}
	entries, ok := env.Data["evidence"].([]any)
	if !ok || len(entries) != 1 {
		t.Fatalf("expected one evidence entry, got %#v", env.Data["evidence"])
	}
}

func TestCRReviewJSONIncludesEvidence(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := service.New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if _, err := svc.AddCR("Evidence in Review", "review evidence payload"); err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	if _, err := svc.AddEvidence(1, service.AddEvidenceOptions{
		Type:    "manual_note",
		Summary: "Captured manual verification step.",
	}); err != nil {
		t.Fatalf("AddEvidence() error = %v", err)
	}

	out, _, runErr := runCLI(t, dir, "cr", "review", "1", "--json")
	if runErr != nil {
		t.Fatalf("cr review --json error = %v\noutput=%s", runErr, out)
	}
	env := decodeEnvelope(t, out)
	if !env.OK {
		t.Fatalf("expected ok response, got %#v", env)
	}
	entries, ok := env.Data["evidence"].([]any)
	if !ok || len(entries) != 1 {
		t.Fatalf("expected one review evidence entry, got %#v", env.Data["evidence"])
	}
	entry, ok := entries[0].(map[string]any)
	if !ok {
		t.Fatalf("expected evidence entry object, got %#v", entries[0])
	}
	if entry["type"] != "manual_note" {
		t.Fatalf("expected manual_note type, got %#v", entry["type"])
	}
}
