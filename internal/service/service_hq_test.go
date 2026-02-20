package service

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sophia/internal/model"
)

func TestHQConfigResolutionUsesRepoOverridesThenGlobalDefaults(t *testing.T) {
	dir := t.TempDir()
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)

	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	globalRemote := "hq-global"
	globalRepo := "repo-global"
	globalBaseURL := "https://global.example"
	if _, err := svc.SetHQConfig(HQConfigSetOptions{
		Global:      true,
		RemoteAlias: &globalRemote,
		RepoID:      &globalRepo,
		BaseURL:     &globalBaseURL,
	}); err != nil {
		t.Fatalf("SetHQConfig(global) error = %v", err)
	}

	repoRepoID := "repo-local"
	repoBaseURL := "https://repo.example"
	if _, err := svc.SetHQConfig(HQConfigSetOptions{
		RepoID:  &repoRepoID,
		BaseURL: &repoBaseURL,
	}); err != nil {
		t.Fatalf("SetHQConfig(repo) error = %v", err)
	}

	view, err := svc.GetHQConfig()
	if err != nil {
		t.Fatalf("GetHQConfig() error = %v", err)
	}
	if view.RemoteAlias != globalRemote {
		t.Fatalf("expected remote alias %q, got %q", globalRemote, view.RemoteAlias)
	}
	if view.RepoID != repoRepoID {
		t.Fatalf("expected repo id %q, got %q", repoRepoID, view.RepoID)
	}
	if view.BaseURL != repoBaseURL {
		t.Fatalf("expected base url %q, got %q", repoBaseURL, view.BaseURL)
	}
	if view.TokenPresent {
		t.Fatalf("expected token_present=false")
	}
}

func TestHQLoginLogoutPersistsTokenInUserConfig(t *testing.T) {
	dir := t.TempDir()
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)

	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	alias, err := svc.HQLogin("", "token-123")
	if err != nil {
		t.Fatalf("HQLogin() error = %v", err)
	}
	if alias != defaultHQRemoteAlias {
		t.Fatalf("expected default alias %q, got %q", defaultHQRemoteAlias, alias)
	}

	credPath, err := svc.hqCredentialPath()
	if err != nil {
		t.Fatalf("hqCredentialPath() error = %v", err)
	}
	info, err := os.Stat(credPath)
	if err != nil {
		t.Fatalf("Stat(credentials) error = %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("expected credentials mode 0600, got %#o", info.Mode().Perm())
	}

	cfg, err := svc.GetHQConfig()
	if err != nil {
		t.Fatalf("GetHQConfig() error = %v", err)
	}
	if !cfg.TokenPresent {
		t.Fatalf("expected token_present=true after login")
	}

	if _, err := svc.HQLogout(""); err != nil {
		t.Fatalf("HQLogout() error = %v", err)
	}
	cfg, err = svc.GetHQConfig()
	if err != nil {
		t.Fatalf("GetHQConfig(after logout) error = %v", err)
	}
	if cfg.TokenPresent {
		t.Fatalf("expected token_present=false after logout")
	}
}

func TestHQWriteAndSyncBlockedInTrackedMetadataMode(t *testing.T) {
	dir := t.TempDir()
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)

	svc := New(dir)
	if _, err := svc.Init("main", "tracked"); err != nil {
		t.Fatalf("Init(tracked) error = %v", err)
	}

	repoID := "repo-tracked"
	baseURL := "https://hq.example"
	if _, err := svc.SetHQConfig(HQConfigSetOptions{RepoID: &repoID, BaseURL: &baseURL}); err != nil {
		t.Fatalf("SetHQConfig() error = %v", err)
	}

	if _, err := svc.HQAddCRNote("cr_uid-1", "note"); !errors.Is(err, ErrHQTrackedModeBlocked) {
		t.Fatalf("expected ErrHQTrackedModeBlocked for note add, got %v", err)
	}
	if _, err := svc.SyncCRFromHQ("cr_uid-1"); !errors.Is(err, ErrHQTrackedModeBlocked) {
		t.Fatalf("expected ErrHQTrackedModeBlocked for sync, got %v", err)
	}
}

func TestSyncCRFromHQUpsertsByUID(t *testing.T) {
	dir := t.TempDir()
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)

	title := "Remote title one"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if r.URL.Path != "/api/v1/repos/repo-one/crs/cr_remote-1" {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		doc := map[string]any{
			"id":          0,
			"uid":         "cr_remote-1",
			"title":       title,
			"description": "synced from hq",
			"status":      "in_progress",
			"base_branch": "main",
			"base_ref":    "main",
			"base_commit": "abc123",
			"branch":      "cr-remote-1",
			"notes":       []string{},
			"evidence":    []any{},
			"contract": map[string]any{
				"why": "HQ sync test",
			},
			"subtasks":   []any{},
			"events":     []any{},
			"created_at": "2026-02-20T00:00:00Z",
			"updated_at": "2026-02-20T00:00:00Z",
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"schema_version": "sophia.hq.v1",
			"cr_uid":         "cr_remote-1",
			"doc":            doc,
		})
	}))
	defer server.Close()

	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	repoID := "repo-one"
	baseURL := server.URL
	if _, err := svc.SetHQConfig(HQConfigSetOptions{RepoID: &repoID, BaseURL: &baseURL}); err != nil {
		t.Fatalf("SetHQConfig() error = %v", err)
	}

	first, err := svc.SyncCRFromHQ("cr_remote-1")
	if err != nil {
		t.Fatalf("SyncCRFromHQ(first) error = %v", err)
	}
	if !first.Created || first.Replaced {
		t.Fatalf("expected first sync created=true replaced=false, got %#v", first)
	}
	loaded, err := svc.store.LoadCRByUID("cr_remote-1")
	if err != nil {
		t.Fatalf("LoadCRByUID() error = %v", err)
	}
	if loaded.Title != title {
		t.Fatalf("expected title %q, got %q", title, loaded.Title)
	}

	title = "Remote title two"
	second, err := svc.SyncCRFromHQ("cr_remote-1")
	if err != nil {
		t.Fatalf("SyncCRFromHQ(second) error = %v", err)
	}
	if second.Created || !second.Replaced {
		t.Fatalf("expected second sync created=false replaced=true, got %#v", second)
	}
	if second.LocalCRID != first.LocalCRID {
		t.Fatalf("expected same local id %d, got %d", first.LocalCRID, second.LocalCRID)
	}
	loaded, err = svc.store.LoadCRByUID("cr_remote-1")
	if err != nil {
		t.Fatalf("LoadCRByUID(after replace) error = %v", err)
	}
	if loaded.Title != title {
		t.Fatalf("expected replaced title %q, got %q", title, loaded.Title)
	}
}

func TestHQListCRsSupportsItemsWithSingleRequest(t *testing.T) {
	dir := t.TempDir()
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)

	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if r.URL.Path != "/api/v1/repos/repo-one/crs" {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"schema_version": "sophia.hq.v1",
			"items": []map[string]any{
				{
					"uid":    "cr_remote-3",
					"title":  "Remote summary",
					"status": "in_progress",
				},
			},
		})
	}))
	defer server.Close()

	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	repoID := "repo-one"
	baseURL := server.URL
	if _, err := svc.SetHQConfig(HQConfigSetOptions{RepoID: &repoID, BaseURL: &baseURL}); err != nil {
		t.Fatalf("SetHQConfig() error = %v", err)
	}

	summaries, err := svc.HQListCRs()
	if err != nil {
		t.Fatalf("HQListCRs() error = %v", err)
	}
	if requests != 1 {
		t.Fatalf("expected 1 request, got %d", requests)
	}
	if len(summaries) != 1 {
		t.Fatalf("expected 1 summary, got %d", len(summaries))
	}
	if summaries[0].UID != "cr_remote-3" {
		t.Fatalf("expected uid cr_remote-3, got %q", summaries[0].UID)
	}
	if summaries[0].Title != "Remote summary" {
		t.Fatalf("expected title Remote summary, got %q", summaries[0].Title)
	}
}

func TestHQAddCRNoteUsesPatchMutationEndpoint(t *testing.T) {
	dir := t.TempDir()
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)

	var received model.HQPatchApplyRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			if r.URL.Path != "/api/v1/repos/repo-one/crs/cr_remote-2" {
				http.Error(w, "not found", http.StatusNotFound)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"schema_version": "sophia.hq.v1",
				"cr_uid":         "cr_remote-2",
				"cr_fingerprint": "fp_remote-2",
				"doc": map[string]any{
					"id":          0,
					"uid":         "cr_remote-2",
					"title":       "Remote CR",
					"description": "remote document",
					"status":      "in_progress",
					"base_branch": "main",
					"base_ref":    "main",
					"base_commit": "abc123",
					"branch":      "cr-remote-2",
					"notes":       []any{},
					"evidence":    []any{},
					"contract": map[string]any{
						"why": "remote note",
					},
					"subtasks":   []any{},
					"events":     []any{},
					"created_at": "2026-02-20T00:00:00Z",
					"updated_at": "2026-02-20T00:00:00Z",
				},
			})
			return
		case http.MethodPost:
			if r.URL.Path != "/api/v1/repos/repo-one/crs/cr_remote-2/patch" {
				http.Error(w, "not found", http.StatusNotFound)
				return
			}
			if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"schema_version": "sophia.hq.v1",
				"cr_uid":         "cr_remote-2",
				"applied_ops":    []int{0},
			})
			return
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
	}))
	defer server.Close()

	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	repoID := "repo-one"
	baseURL := server.URL
	if _, err := svc.SetHQConfig(HQConfigSetOptions{RepoID: &repoID, BaseURL: &baseURL}); err != nil {
		t.Fatalf("SetHQConfig() error = %v", err)
	}
	result, err := svc.HQAddCRNote("cr_remote-2", "hello")
	if err != nil {
		t.Fatalf("HQAddCRNote() error = %v", err)
	}
	if len(result.AppliedOps) != 1 {
		t.Fatalf("expected one applied op, got %#v", result.AppliedOps)
	}
	if received.SchemaVersion != model.HQSchemaV1 {
		t.Fatalf("expected request schema %q, got %q", model.HQSchemaV1, received.SchemaVersion)
	}
	if received.Patch.SchemaVersion != model.CRPatchSchemaV1 {
		t.Fatalf("expected patch schema %q, got %q", model.CRPatchSchemaV1, received.Patch.SchemaVersion)
	}
	if received.Patch.Base.CRFingerprint != "fp_remote-2" {
		t.Fatalf("expected patch base fingerprint %q, got %q", "fp_remote-2", received.Patch.Base.CRFingerprint)
	}
	if len(received.Patch.Ops) != 1 {
		t.Fatalf("expected one patch op, got %d", len(received.Patch.Ops))
	}
	if !strings.Contains(string(received.Patch.Ops[0]), "\"add_note\"") {
		t.Fatalf("expected add_note op payload, got %s", string(received.Patch.Ops[0]))
	}
}

func TestHQCredentialPathUsesUserConfigHome(t *testing.T) {
	dir := t.TempDir()
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)

	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	path, err := svc.hqCredentialPath()
	if err != nil {
		t.Fatalf("hqCredentialPath() error = %v", err)
	}
	if !strings.HasPrefix(path, filepath.Join(configHome, "sophia")) {
		t.Fatalf("expected credentials path under %s, got %s", filepath.Join(configHome, "sophia"), path)
	}
}
