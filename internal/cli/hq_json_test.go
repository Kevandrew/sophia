package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHQConfigAndCRSyncJSON(t *testing.T) {
	dir := t.TempDir()
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if r.URL.Path != "/api/v1/repos/repo-one/crs/cr_remote-1" {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"schema_version": "sophia.hq.v1",
			"cr_uid":         "cr_remote-1",
			"doc": map[string]any{
				"id":          0,
				"uid":         "cr_remote-1",
				"title":       "Remote CR",
				"description": "remote document",
				"status":      "in_progress",
				"base_branch": "main",
				"base_ref":    "main",
				"base_commit": "abc123",
				"branch":      "cr-remote-1",
				"notes":       []any{},
				"evidence":    []any{},
				"contract": map[string]any{
					"why": "remote sync",
				},
				"subtasks":   []any{},
				"events":     []any{},
				"created_at": "2026-02-20T00:00:00Z",
				"updated_at": "2026-02-20T00:00:00Z",
			},
		})
	}))
	defer server.Close()

	if out, _, err := runCLI(t, dir, "init", "--json"); err != nil {
		t.Fatalf("init --json error = %v\noutput=%s", err, out)
	}

	setOut, _, setErr := runCLI(t, dir, "hq", "config", "set", "--repo-id", "repo-one", "--base-url", server.URL, "--json")
	if setErr != nil {
		t.Fatalf("hq config set --json error = %v\noutput=%s", setErr, setOut)
	}
	setEnv := decodeEnvelope(t, setOut)
	if !setEnv.OK {
		t.Fatalf("expected hq config set ok envelope, got %#v", setEnv)
	}

	showOut, _, showErr := runCLI(t, dir, "hq", "config", "show", "--json")
	if showErr != nil {
		t.Fatalf("hq config show --json error = %v\noutput=%s", showErr, showOut)
	}
	showEnv := decodeEnvelope(t, showOut)
	if !showEnv.OK {
		t.Fatalf("expected hq config show ok envelope, got %#v", showEnv)
	}
	if got, _ := showEnv.Data["repo_id"].(string); got != "repo-one" {
		t.Fatalf("expected repo_id repo-one, got %#v", showEnv.Data["repo_id"])
	}
	if got, _ := showEnv.Data["base_url"].(string); got != server.URL {
		t.Fatalf("expected base_url %q, got %#v", server.URL, showEnv.Data["base_url"])
	}

	syncOut, _, syncErr := runCLI(t, dir, "cr", "sync", "cr_remote-1", "--json")
	if syncErr != nil {
		t.Fatalf("cr sync --json error = %v\noutput=%s", syncErr, syncOut)
	}
	syncEnv := decodeEnvelope(t, syncOut)
	if !syncEnv.OK {
		t.Fatalf("expected cr sync ok envelope, got %#v", syncEnv)
	}
	if created, _ := syncEnv.Data["created"].(bool); !created {
		t.Fatalf("expected created=true, got %#v", syncEnv.Data["created"])
	}
	if id, _ := syncEnv.Data["local_cr_id"].(float64); int(id) <= 0 {
		t.Fatalf("expected local_cr_id > 0, got %#v", syncEnv.Data["local_cr_id"])
	}
}

func TestCRSyncJSONBlockedWhenMetadataModeTracked(t *testing.T) {
	dir := t.TempDir()
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)

	if out, _, err := runCLI(t, dir, "init", "--metadata-mode", "tracked", "--json"); err != nil {
		t.Fatalf("init --metadata-mode tracked --json error = %v\noutput=%s", err, out)
	}
	if out, _, err := runCLI(t, dir, "hq", "config", "set", "--repo-id", "repo-one", "--base-url", "https://hq.example", "--json"); err != nil {
		t.Fatalf("hq config set --json error = %v\noutput=%s", err, out)
	}

	out, _, runErr := runCLI(t, dir, "cr", "sync", "cr_remote-1", "--json")
	if runErr == nil {
		t.Fatalf("expected cr sync --json to fail in tracked mode")
	}
	env := decodeEnvelope(t, out)
	if env.OK {
		t.Fatalf("expected non-ok envelope, got %#v", env)
	}
	if env.Error == nil {
		t.Fatalf("expected error payload, got %#v", env)
	}
	if env.Error.Code != "hq_tracked_mode_blocked" {
		t.Fatalf("expected hq_tracked_mode_blocked code, got %#v", env.Error)
	}
}

func TestCRSyncJSONMalformedHQResponse(t *testing.T) {
	dir := t.TempDir()
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if r.URL.Path != "/api/v1/repos/repo-one/crs/cr_remote-bad" {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		// Missing doc/cr on purpose.
		_ = json.NewEncoder(w).Encode(map[string]any{
			"schema_version": "sophia.hq.v1",
			"cr_uid":         "cr_remote-bad",
		})
	}))
	defer server.Close()

	if out, _, err := runCLI(t, dir, "init", "--json"); err != nil {
		t.Fatalf("init --json error = %v\noutput=%s", err, out)
	}
	if out, _, err := runCLI(t, dir, "hq", "config", "set", "--repo-id", "repo-one", "--base-url", server.URL, "--json"); err != nil {
		t.Fatalf("hq config set --json error = %v\noutput=%s", err, out)
	}

	out, _, runErr := runCLI(t, dir, "cr", "sync", "cr_remote-bad", "--json")
	if runErr == nil {
		t.Fatalf("expected cr sync --json to fail for malformed HQ response")
	}
	env := decodeEnvelope(t, out)
	if env.OK {
		t.Fatalf("expected non-ok envelope, got %#v", env)
	}
	if env.Error == nil {
		t.Fatalf("expected error payload, got %#v", env)
	}
	if env.Error.Code != "hq_malformed_response" {
		t.Fatalf("expected hq_malformed_response code, got %#v", env.Error)
	}
}
