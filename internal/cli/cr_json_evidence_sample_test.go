package cli

import (
	"testing"

	"sophia/internal/service"
)

func TestCREvidenceSampleAddAndListJSON(t *testing.T) {
	dir := t.TempDir()
	svc := service.New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if _, err := svc.AddCR("sample evidence", "review sample wrappers"); err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}

	out, _, runErr := runCLI(t, dir, "cr", "evidence", "sample", "add", "1", "--scope", "internal/service", "--summary", "Spot-checked merge gate path", "--json")
	if runErr != nil {
		t.Fatalf("cr evidence sample add --json error = %v\noutput=%s", runErr, out)
	}
	env := decodeEnvelope(t, out)
	if !env.OK {
		t.Fatalf("expected ok envelope from sample add, got %#v", env)
	}
	evidence, ok := env.Data["evidence"].(map[string]any)
	if !ok {
		t.Fatalf("expected evidence object, got %#v", env.Data["evidence"])
	}
	if evidenceType, ok := evidence["type"].(string); !ok || evidenceType != "review_sample" {
		t.Fatalf("expected review_sample type, got %#v", evidence["type"])
	}

	_, _, err := runCLI(t, dir, "cr", "evidence", "add", "1", "--type", "manual_note", "--summary", "non-sample note", "--json")
	if err != nil {
		t.Fatalf("cr evidence add manual_note --json error = %v", err)
	}

	out, _, runErr = runCLI(t, dir, "cr", "evidence", "sample", "list", "1", "--json")
	if runErr != nil {
		t.Fatalf("cr evidence sample list --json error = %v\noutput=%s", runErr, out)
	}
	env = decodeEnvelope(t, out)
	if !env.OK {
		t.Fatalf("expected ok envelope from sample list, got %#v", env)
	}
	count, ok := env.Data["count"].(float64)
	if !ok || count != 1 {
		t.Fatalf("expected sample count 1, got %#v", env.Data["count"])
	}
	samples, ok := env.Data["samples"].([]any)
	if !ok || len(samples) != 1 {
		t.Fatalf("expected one sample entry, got %#v", env.Data["samples"])
	}
}
