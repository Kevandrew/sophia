package cli

import (
	"strings"
	"testing"
)

func TestVersionCommandDefaults(t *testing.T) {
	t.Parallel()
	SetBuildInfo("dev", "unknown", "unknown")

	out, _, err := runCLI(t, t.TempDir(), "version")
	if err != nil {
		t.Fatalf("version command returned error: %v\noutput=%s", err, out)
	}
	assertHelpContains(t, out,
		"version: dev",
		"commit: unknown",
		"build_date: unknown",
	)
}

func TestVersionCommandInjectedBuildInfo(t *testing.T) {
	t.Parallel()
	SetBuildInfo("v1.2.3", "abc1234", "2026-02-19T11:00:00Z")

	out, _, err := runCLI(t, t.TempDir(), "version")
	if err != nil {
		t.Fatalf("version command returned error: %v\noutput=%s", err, out)
	}
	assertHelpContains(t, out,
		"version: v1.2.3",
		"commit: abc1234",
		"build_date: 2026-02-19T11:00:00Z",
	)

	jsonOut, _, jsonErr := runCLI(t, t.TempDir(), "version", "--json")
	if jsonErr != nil {
		t.Fatalf("version --json returned error: %v\noutput=%s", jsonErr, jsonOut)
	}
	env := decodeEnvelope(t, jsonOut)
	if !env.OK {
		t.Fatalf("expected ok envelope, got %#v", env)
	}
	if got := strings.TrimSpace(env.Data["version"].(string)); got != "v1.2.3" {
		t.Fatalf("expected version v1.2.3, got %q", got)
	}
	if got := strings.TrimSpace(env.Data["commit"].(string)); got != "abc1234" {
		t.Fatalf("expected commit abc1234, got %q", got)
	}
	if got := strings.TrimSpace(env.Data["build_date"].(string)); got != "2026-02-19T11:00:00Z" {
		t.Fatalf("expected build date 2026-02-19T11:00:00Z, got %q", got)
	}
}
