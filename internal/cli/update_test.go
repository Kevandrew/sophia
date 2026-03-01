package cli

import (
	"context"
	"strings"
	"testing"
)

func restoreUpdateDeps() func() {
	origFetch := fetchLatestReleaseTagFn
	origDownload := downloadInstallScriptFn
	origRun := runInstallScriptFn
	origVersion := buildVersion
	origCommit := buildCommit
	origDate := buildDate
	return func() {
		fetchLatestReleaseTagFn = origFetch
		downloadInstallScriptFn = origDownload
		runInstallScriptFn = origRun
		SetBuildInfo(origVersion, origCommit, origDate)
	}
}

func TestUpdateCheckJSONReportsAvailability(t *testing.T) {
	defer restoreUpdateDeps()()
	SetBuildInfo("v1.0.0", "abc", "2026-01-01T00:00:00Z")
	fetchLatestReleaseTagFn = func(ctx context.Context, repo string) (string, error) {
		return "v1.1.0", nil
	}

	out, _, err := runCLI(t, t.TempDir(), "update", "--check", "--json")
	if err != nil {
		t.Fatalf("update --check --json error = %v\noutput=%s", err, out)
	}
	env := decodeEnvelope(t, out)
	if !env.OK {
		t.Fatalf("expected ok envelope, got %#v", env)
	}
	if got, _ := env.Data["latest_version"].(string); got != "v1.1.0" {
		t.Fatalf("latest_version = %q, want v1.1.0", got)
	}
	if got, _ := env.Data["current_version"].(string); got != "v1.0.0" {
		t.Fatalf("current_version = %q, want v1.0.0", got)
	}
	available, _ := env.Data["update_available"].(bool)
	if !available {
		t.Fatalf("expected update_available=true, got %#v", env.Data["update_available"])
	}
}

func TestUpdateApplyRequiresYes(t *testing.T) {
	defer restoreUpdateDeps()()
	SetBuildInfo("v1.0.0", "abc", "2026-01-01T00:00:00Z")
	fetchLatestReleaseTagFn = func(ctx context.Context, repo string) (string, error) {
		return "v1.1.0", nil
	}

	out, _, err := runCLI(t, t.TempDir(), "update")
	if err == nil {
		t.Fatalf("expected update without --yes to fail")
	}
	if !strings.Contains(err.Error(), "--yes is required") {
		t.Fatalf("expected --yes error, got %v\noutput=%s", err, out)
	}
}

func TestUpdateApplyRunsInstallScript(t *testing.T) {
	defer restoreUpdateDeps()()
	SetBuildInfo("v1.0.0", "abc", "2026-01-01T00:00:00Z")
	fetchLatestReleaseTagFn = func(ctx context.Context, repo string) (string, error) {
		return "v1.2.0", nil
	}
	downloadInstallScriptFn = func(ctx context.Context, url string) (string, error) {
		return "echo install", nil
	}
	var ran bool
	var gotVersion string
	var gotRepo string
	var gotInstallDir string
	runInstallScriptFn = func(ctx context.Context, scriptBody, version, repo, installDir string) error {
		ran = true
		gotVersion = version
		gotRepo = repo
		gotInstallDir = installDir
		if strings.TrimSpace(scriptBody) == "" {
			t.Fatalf("expected non-empty script body")
		}
		return nil
	}

	out, _, err := runCLI(t, t.TempDir(), "update", "--yes", "--repo", "Kevandrew/sophia", "--install-dir", "/tmp/bin", "--json")
	if err != nil {
		t.Fatalf("update --yes --json error = %v\noutput=%s", err, out)
	}
	if !ran {
		t.Fatalf("expected install script runner to be invoked")
	}
	if gotVersion != "v1.2.0" {
		t.Fatalf("version = %q, want v1.2.0", gotVersion)
	}
	if gotRepo != "Kevandrew/sophia" {
		t.Fatalf("repo = %q, want Kevandrew/sophia", gotRepo)
	}
	if gotInstallDir != "/tmp/bin" {
		t.Fatalf("install_dir = %q, want /tmp/bin", gotInstallDir)
	}
	env := decodeEnvelope(t, out)
	if !env.OK {
		t.Fatalf("expected ok envelope, got %#v", env)
	}
	updated, _ := env.Data["updated"].(bool)
	if !updated {
		t.Fatalf("expected updated=true, got %#v", env.Data["updated"])
	}
}

