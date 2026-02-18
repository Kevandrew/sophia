package cli

import (
	"os"
	"path/filepath"
	"testing"

	"sophia/internal/service"
)

func TestCRCheckRunAndStatusJSON(t *testing.T) {
	dir := t.TempDir()
	svc := service.New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")
	if err := os.WriteFile(filepath.Join(dir, "SOPHIA.yaml"), []byte(`version: v1
trust:
  mode: advisory
  checks:
    freshness_hours: 24
    definitions:
      - key: smoke
        command: "printf 'ok\n'"
        tiers: [low, medium, high]
        allow_exit_codes: [0]
`), 0o644); err != nil {
		t.Fatalf("write policy file: %v", err)
	}
	if _, err := svc.AddCR("check json", "check surfaces"); err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}

	out, _, runErr := runCLI(t, dir, "cr", "check", "status", "1", "--json")
	if runErr != nil {
		t.Fatalf("cr check status --json error = %v\noutput=%s", runErr, out)
	}
	env := decodeEnvelope(t, out)
	if !env.OK {
		t.Fatalf("expected ok envelope, got %#v", env)
	}
	results, ok := env.Data["check_results"].([]any)
	if !ok || len(results) != 1 {
		t.Fatalf("expected single check result, got %#v", env.Data["check_results"])
	}
	mode, ok := env.Data["check_mode"].(string)
	if !ok || mode != "required" {
		t.Fatalf("expected check_mode=required, got %#v", env.Data["check_mode"])
	}
	requiredCount, ok := env.Data["required_check_count"].(float64)
	if !ok || requiredCount != 1 {
		t.Fatalf("expected required_check_count=1, got %#v", env.Data["required_check_count"])
	}
	guidance, ok := env.Data["guidance"].([]any)
	if !ok || len(guidance) != 0 {
		t.Fatalf("expected empty guidance when checks are required, got %#v", env.Data["guidance"])
	}

	out, _, runErr = runCLI(t, dir, "cr", "check", "run", "1", "--json")
	if runErr != nil {
		t.Fatalf("cr check run --json error = %v\noutput=%s", runErr, out)
	}
	env = decodeEnvelope(t, out)
	if !env.OK {
		t.Fatalf("expected ok envelope, got %#v", env)
	}
	executed, ok := env.Data["executed"].(float64)
	if !ok || executed != 1 {
		t.Fatalf("expected executed=1, got %#v", env.Data["executed"])
	}
	mode, ok = env.Data["check_mode"].(string)
	if !ok || mode != "required" {
		t.Fatalf("expected check_mode=required, got %#v", env.Data["check_mode"])
	}
	requiredCount, ok = env.Data["required_check_count"].(float64)
	if !ok || requiredCount != 1 {
		t.Fatalf("expected required_check_count=1, got %#v", env.Data["required_check_count"])
	}
	guidance, ok = env.Data["guidance"].([]any)
	if !ok || len(guidance) != 0 {
		t.Fatalf("expected empty guidance when checks are required, got %#v", env.Data["guidance"])
	}
}

func TestCRCheckStatusJSONNoRequiredChecks(t *testing.T) {
	dir := t.TempDir()
	svc := service.New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if _, err := svc.AddCR("check json no-op", "check surfaces no required checks"); err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}

	out, _, runErr := runCLI(t, dir, "cr", "check", "status", "1", "--json")
	if runErr != nil {
		t.Fatalf("cr check status --json error = %v\noutput=%s", runErr, out)
	}
	env := decodeEnvelope(t, out)
	if !env.OK {
		t.Fatalf("expected ok envelope, got %#v", env)
	}
	mode, ok := env.Data["check_mode"].(string)
	if !ok || mode != "none" {
		t.Fatalf("expected check_mode=none, got %#v", env.Data["check_mode"])
	}
	requiredCount, ok := env.Data["required_check_count"].(float64)
	if !ok || requiredCount != 0 {
		t.Fatalf("expected required_check_count=0, got %#v", env.Data["required_check_count"])
	}
	checkResults, ok := env.Data["check_results"].([]any)
	if !ok || len(checkResults) != 0 {
		t.Fatalf("expected empty check_results, got %#v", env.Data["check_results"])
	}
	guidance, ok := env.Data["guidance"].([]any)
	if !ok || len(guidance) == 0 {
		t.Fatalf("expected non-empty guidance for no-op check mode, got %#v", env.Data["guidance"])
	}
}
