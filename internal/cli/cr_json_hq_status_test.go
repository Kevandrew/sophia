package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"sophia/internal/service"
)

func TestCRStatusHQJSONNotConfiguredReturnsOK(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	svc := service.New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if _, err := svc.AddCR("HQ status", "fixture"); err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}

	out, _, runErr := runCLI(t, dir, "cr", "status", "1", "--hq", "--json")
	if runErr != nil {
		t.Fatalf("cr status --hq --json error = %v\noutput=%s", runErr, out)
	}
	env := decodeEnvelope(t, out)
	if !env.OK {
		t.Fatalf("expected ok envelope, got %#v", env)
	}
	hq, ok := env.Data["hq_sync"].(map[string]any)
	if !ok {
		t.Fatalf("expected hq_sync object, got %#v", env.Data["hq_sync"])
	}
	if configured, _ := hq["configured"].(bool); configured {
		t.Fatalf("expected configured=false, got %#v", hq["configured"])
	}
	if state, _ := hq["state"].(string); state != "not_configured" {
		t.Fatalf("expected state=not_configured, got %#v", hq["state"])
	}
}

func TestCRStatusHQJSONConfiguredPopulatesRemoteFields(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	svc := service.New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	cr, err := svc.AddCR("HQ status", "fixture")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		want := "/api/v1/repos/repo-one/crs/" + cr.UID
		if r.URL.Path != want {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"schema_version":     "sophia.hq.v1",
			"cr_uid":             cr.UID,
			"cr_fingerprint":     "remote-fp",
			"doc_schema_version": "sophia.cr_doc.v1",
			"doc":                map[string]any{"id": cr.ID, "uid": cr.UID, "title": cr.Title, "description": cr.Description, "status": cr.Status},
		})
	}))
	defer ts.Close()

	repoID := "repo-one"
	baseURL := ts.URL
	if _, err := svc.SetHQConfig(service.HQConfigSetOptions{RepoID: &repoID, BaseURL: &baseURL}); err != nil {
		t.Fatalf("SetHQConfig() error = %v", err)
	}

	out, _, runErr := runCLI(t, dir, "cr", "status", cr.UID, "--hq", "--json")
	if runErr != nil {
		t.Fatalf("cr status <uid> --hq --json error = %v\noutput=%s", runErr, out)
	}
	env := decodeEnvelope(t, out)
	if !env.OK {
		t.Fatalf("expected ok envelope, got %#v", env)
	}
	hq, ok := env.Data["hq_sync"].(map[string]any)
	if !ok {
		t.Fatalf("expected hq_sync object, got %#v", env.Data["hq_sync"])
	}
	if configured, _ := hq["configured"].(bool); !configured {
		t.Fatalf("expected configured=true, got %#v", hq["configured"])
	}
	if checked, _ := hq["remote_checked"].(bool); !checked {
		t.Fatalf("expected remote_checked=true, got %#v", hq["remote_checked"])
	}
	if exists, _ := hq["remote_exists"].(bool); !exists {
		t.Fatalf("expected remote_exists=true, got %#v", hq["remote_exists"])
	}
	if fp, _ := hq["remote_fingerprint"].(string); fp == "" {
		t.Fatalf("expected remote_fingerprint non-empty, got %#v", hq["remote_fingerprint"])
	}
	if state, _ := hq["state"].(string); state != "unlinked" {
		t.Fatalf("expected state=unlinked (no upstream link yet), got %#v", hq["state"])
	}
}
