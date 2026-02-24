package cli

import (
	"testing"

	"sophia/internal/service"
)

func TestCRStatusJSONIncludesBranchIdentity(t *testing.T) {
	dir := t.TempDir()
	svc := service.New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if _, err := svc.AddCR("Branch identity", "json payload"); err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}

	out, _, runErr := runCLI(t, dir, "cr", "status", "1", "--json")
	if runErr != nil {
		t.Fatalf("cr status --json error = %v\noutput=%s", runErr, out)
	}
	env := decodeEnvelope(t, out)
	if !env.OK {
		t.Fatalf("expected ok envelope, got %#v", env)
	}
	identity, ok := env.Data["branch_identity"].(map[string]any)
	if !ok {
		t.Fatalf("expected branch_identity object in status payload, got %#v", env.Data["branch_identity"])
	}
	if scheme, _ := identity["scheme"].(string); scheme != "human_alias_v2" {
		t.Fatalf("expected human_alias_v2 scheme, got %#v", identity["scheme"])
	}
}

func TestCRListJSONIncludesBranchIdentity(t *testing.T) {
	dir := t.TempDir()
	svc := service.New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if _, err := svc.AddCR("Branch identity", "json payload"); err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}

	out, _, runErr := runCLI(t, dir, "cr", "list", "--json")
	if runErr != nil {
		t.Fatalf("cr list --json error = %v\noutput=%s", runErr, out)
	}
	env := decodeEnvelope(t, out)
	if !env.OK {
		t.Fatalf("expected ok envelope, got %#v", env)
	}
	results, ok := env.Data["results"].([]any)
	if !ok || len(results) == 0 {
		t.Fatalf("expected results array, got %#v", env.Data["results"])
	}
	item, ok := results[0].(map[string]any)
	if !ok {
		t.Fatalf("expected first result object, got %#v", results[0])
	}
	if _, ok := item["branch_identity"].(map[string]any); !ok {
		t.Fatalf("expected branch_identity in list result, got %#v", item["branch_identity"])
	}
}
