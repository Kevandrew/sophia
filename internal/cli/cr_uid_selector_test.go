package cli

import (
	"strings"
	"testing"

	"sophia/internal/service"
)

func TestCRCommandsAcceptUIDSelectorArgument(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := service.New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	cr, err := svc.AddCR("UID selector", "cli selector")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}

	out, _, runErr := runCLI(t, dir, "cr", "status", cr.UID, "--json")
	if runErr != nil {
		t.Fatalf("cr status <uid> --json error = %v\noutput=%s", runErr, out)
	}
	env := decodeEnvelope(t, out)
	if !env.OK {
		t.Fatalf("expected ok envelope, got %#v", env)
	}
	if got, _ := env.Data["id"].(float64); int(got) != cr.ID {
		t.Fatalf("expected resolved id %d, got %#v", cr.ID, env.Data["id"])
	}
}

func TestCRCommandUIDSelectorNotFoundReturnsError(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := service.New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if _, err := svc.AddCR("UID selector", "cli selector"); err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}

	_, _, runErr := runCLI(t, dir, "cr", "status", "cr_missing-uid", "--json")
	if runErr == nil || !strings.Contains(strings.ToLower(runErr.Error()), "not found") {
		t.Fatalf("expected uid not found error, got %v", runErr)
	}
}
