package service

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
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

func TestPushCRToHQCreatesRemoteViaUpsert(t *testing.T) {
	dir := t.TempDir()
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)

	var (
		getCount  int
		putCount  int
		upsertReq model.HQUpsertCRRequest
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			getCount++
			http.Error(w, "not found", http.StatusNotFound)
			return
		case http.MethodPut:
			putCount++
			if err := json.NewDecoder(r.Body).Decode(&upsertReq); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"schema_version": "sophia.hq.v1",
				"cr_uid":         "cr_remote_push_create",
				"cr_fingerprint": "fp_remote_created",
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
	cr, err := svc.AddCR("push create", "local to remote")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	repoID := "repo-one"
	baseURL := server.URL
	if _, err := svc.SetHQConfig(HQConfigSetOptions{RepoID: &repoID, BaseURL: &baseURL}); err != nil {
		t.Fatalf("SetHQConfig() error = %v", err)
	}

	result, err := svc.PushCRToHQ(strconv.Itoa(cr.ID), false)
	if err != nil {
		t.Fatalf("PushCRToHQ() error = %v", err)
	}
	if !result.CreatedRemote || result.UpdatedRemote {
		t.Fatalf("expected created_remote=true updated_remote=false, got %#v", result)
	}
	if getCount != 1 || putCount != 1 {
		t.Fatalf("expected one GET and one PUT, got GET=%d PUT=%d", getCount, putCount)
	}
	if upsertReq.SchemaVersion != model.HQSchemaV1 {
		t.Fatalf("expected schema_version %q, got %q", model.HQSchemaV1, upsertReq.SchemaVersion)
	}
	if upsertReq.DocSchemaVersion != model.CRDocSchemaV1 {
		t.Fatalf("expected doc_schema_version %q, got %q", model.CRDocSchemaV1, upsertReq.DocSchemaVersion)
	}

	updated, err := svc.store.LoadCR(cr.ID)
	if err != nil {
		t.Fatalf("LoadCR() error = %v", err)
	}
	if updated.HQ.UpstreamFingerprint != "fp_remote_created" {
		t.Fatalf("expected upstream fingerprint fp_remote_created, got %q", updated.HQ.UpstreamFingerprint)
	}
	if strings.TrimSpace(updated.HQ.LastPushAt) == "" {
		t.Fatalf("expected LastPushAt to be set")
	}
}

func TestPushCRToHQNoopDoesNotUpdateLastPullAt(t *testing.T) {
	dir := t.TempDir()
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)

	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	cr, err := svc.AddCR("push noop", "local matches remote")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	loaded, err := svc.store.LoadCR(cr.ID)
	if err != nil {
		t.Fatalf("LoadCR() error = %v", err)
	}
	loaded.HQ.LastPullAt = "2026-02-20T00:00:00Z"
	baseRemote := cloneRemoteCR(loaded)
	remoteFP, err := fingerprintHQIntentCR(baseRemote)
	if err != nil {
		t.Fatalf("fingerprintHQIntentCR(remote) error = %v", err)
	}
	loaded.HQ.UpstreamFingerprint = remoteFP
	if err := svc.store.SaveCR(loaded); err != nil {
		t.Fatalf("SaveCR() error = %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		respondRemoteCR(t, w, baseRemote, remoteFP)
	}))
	defer server.Close()

	repoID := "repo-one"
	baseURL := server.URL
	if _, err := svc.SetHQConfig(HQConfigSetOptions{RepoID: &repoID, BaseURL: &baseURL}); err != nil {
		t.Fatalf("SetHQConfig() error = %v", err)
	}

	if _, err := svc.PushCRToHQ(strconv.Itoa(cr.ID), false); err != nil {
		t.Fatalf("PushCRToHQ() error = %v", err)
	}
	after, err := svc.store.LoadCR(cr.ID)
	if err != nil {
		t.Fatalf("LoadCR(after) error = %v", err)
	}
	if after.HQ.LastPullAt != "2026-02-20T00:00:00Z" {
		t.Fatalf("expected LastPullAt unchanged, got %q", after.HQ.LastPullAt)
	}
	if strings.TrimSpace(after.HQ.LastPushAt) == "" {
		t.Fatalf("expected LastPushAt to be set")
	}
}

func TestPushCRToHQReturnsPatchConflictError(t *testing.T) {
	dir := t.TempDir()
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)

	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	cr, err := svc.AddCR("push conflict", "server will report conflict")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	loaded, err := svc.store.LoadCR(cr.ID)
	if err != nil {
		t.Fatalf("LoadCR() error = %v", err)
	}

	remote := cloneRemoteCR(loaded)
	remote.Title = "Remote baseline"
	remoteFP, err := fingerprintHQIntentCR(remote)
	if err != nil {
		t.Fatalf("fingerprintHQIntentCR(remote) error = %v", err)
	}
	loaded.HQ.UpstreamFingerprint = remoteFP
	loaded.Title = "Local changed"
	if err := svc.store.SaveCR(loaded); err != nil {
		t.Fatalf("SaveCR() error = %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			respondRemoteCR(t, w, remote, remoteFP)
			return
		case http.MethodPost:
			if !strings.HasSuffix(r.URL.Path, "/patch") {
				http.Error(w, "not found", http.StatusNotFound)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"schema_version": "sophia.hq.v1",
				"cr_uid":         strings.TrimSpace(remote.UID),
				"cr_fingerprint": "fp_remote_after",
				"conflicts": []map[string]any{
					{"op_index": 0, "op": "set_field", "field": "cr.title", "message": "before mismatch"},
				},
			})
			return
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
	}))
	defer server.Close()

	repoID := "repo-one"
	baseURL := server.URL
	if _, err := svc.SetHQConfig(HQConfigSetOptions{RepoID: &repoID, BaseURL: &baseURL}); err != nil {
		t.Fatalf("SetHQConfig() error = %v", err)
	}

	_, err = svc.PushCRToHQ(strconv.Itoa(cr.ID), false)
	if err == nil {
		t.Fatalf("expected PushCRToHQ() to return patch conflict error")
	}
	if !errors.Is(err, ErrHQPatchConflict) {
		t.Fatalf("expected ErrHQPatchConflict, got %T (%v)", err, err)
	}
	var conflictErr *HQPatchConflictError
	if !errors.As(err, &conflictErr) {
		t.Fatalf("expected HQPatchConflictError, got %T", err)
	}
	if conflictErr.ApplyResult == nil || len(conflictErr.ApplyResult.Conflicts) == 0 {
		t.Fatalf("expected conflict details, got %#v", conflictErr)
	}
}

func TestPushCRToHQPublishesNewTaskWithUpdate(t *testing.T) {
	dir := t.TempDir()
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)

	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	cr, err := svc.AddCR("push new task", "local has additional task")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	if _, err := svc.AddTask(cr.ID, "remote task"); err != nil {
		t.Fatalf("AddTask(remote) error = %v", err)
	}
	newTask, err := svc.AddTask(cr.ID, "local-only task")
	if err != nil {
		t.Fatalf("AddTask(local) error = %v", err)
	}
	loaded, err := svc.store.LoadCR(cr.ID)
	if err != nil {
		t.Fatalf("LoadCR() error = %v", err)
	}
	for i := range loaded.Subtasks {
		if loaded.Subtasks[i].ID != newTask.ID {
			continue
		}
		loaded.Subtasks[i].Status = model.TaskStatusDone
		loaded.Subtasks[i].Contract.Intent = "intent"
		loaded.Subtasks[i].Contract.Scope = []string{"internal/service"}
	}
	remote := cloneRemoteCR(loaded)
	remote.Subtasks = remote.Subtasks[:1]
	remoteFP, err := fingerprintHQIntentCR(remote)
	if err != nil {
		t.Fatalf("fingerprintHQIntentCR(remote) error = %v", err)
	}
	loaded.HQ.UpstreamFingerprint = remoteFP
	if err := svc.store.SaveCR(loaded); err != nil {
		t.Fatalf("SaveCR() error = %v", err)
	}

	var received model.HQPatchApplyRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			respondRemoteCR(t, w, remote, remoteFP)
			return
		case http.MethodPost:
			if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"schema_version": "sophia.hq.v1",
				"cr_uid":         strings.TrimSpace(remote.UID),
				"cr_fingerprint": "fp_remote_after",
			})
			return
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
	}))
	defer server.Close()

	repoID := "repo-one"
	baseURL := server.URL
	if _, err := svc.SetHQConfig(HQConfigSetOptions{RepoID: &repoID, BaseURL: &baseURL}); err != nil {
		t.Fatalf("SetHQConfig() error = %v", err)
	}

	result, err := svc.PushCRToHQ(strconv.Itoa(cr.ID), false)
	if err != nil {
		t.Fatalf("PushCRToHQ() error = %v", err)
	}
	if !result.UpdatedRemote || result.CreatedRemote {
		t.Fatalf("expected updated_remote=true created_remote=false, got %#v", result)
	}
	if len(received.Patch.Ops) < 2 {
		t.Fatalf("expected at least 2 patch ops, got %d", len(received.Patch.Ops))
	}
	if !strings.Contains(string(received.Patch.Ops[0]), "\"add_task\"") {
		t.Fatalf("expected first op add_task, got %s", string(received.Patch.Ops[0]))
	}
	foundUpdate := false
	for _, op := range received.Patch.Ops {
		if strings.Contains(string(op), "\"update_task\"") {
			foundUpdate = true
			break
		}
	}
	if !foundUpdate {
		t.Fatalf("expected an update_task op for new task, ops=%v", received.Patch.Ops)
	}
	if len(result.Warnings) != 0 {
		t.Fatalf("expected no warnings, got %#v", result.Warnings)
	}
}

func TestPushCRToHQEncodesTaskDeleteAndReorderWithSchemaV2(t *testing.T) {
	dir := t.TempDir()
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)

	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	cr, err := svc.AddCR("push reorder delete", "local removes and reorders")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	if _, err := svc.AddTask(cr.ID, "task-1"); err != nil {
		t.Fatalf("AddTask(1) error = %v", err)
	}
	if _, err := svc.AddTask(cr.ID, "task-2"); err != nil {
		t.Fatalf("AddTask(2) error = %v", err)
	}
	if _, err := svc.AddTask(cr.ID, "task-3"); err != nil {
		t.Fatalf("AddTask(3) error = %v", err)
	}
	loaded, err := svc.store.LoadCR(cr.ID)
	if err != nil {
		t.Fatalf("LoadCR() error = %v", err)
	}
	remote := cloneRemoteCR(loaded)
	remoteFP, err := fingerprintHQIntentCR(remote)
	if err != nil {
		t.Fatalf("fingerprintHQIntentCR(remote) error = %v", err)
	}

	byID := map[int]model.Subtask{}
	for _, task := range loaded.Subtasks {
		byID[task.ID] = task
	}
	loaded.Subtasks = []model.Subtask{byID[3], byID[1]}
	loaded.HQ.UpstreamFingerprint = remoteFP
	if err := svc.store.SaveCR(loaded); err != nil {
		t.Fatalf("SaveCR() error = %v", err)
	}

	var received model.HQPatchApplyRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			respondRemoteCR(t, w, remote, remoteFP)
			return
		case http.MethodPost:
			if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"schema_version": "sophia.hq.v1",
				"cr_uid":         strings.TrimSpace(remote.UID),
				"cr_fingerprint": "fp_remote_after",
			})
			return
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
	}))
	defer server.Close()

	repoID := "repo-one"
	baseURL := server.URL
	if _, err := svc.SetHQConfig(HQConfigSetOptions{RepoID: &repoID, BaseURL: &baseURL}); err != nil {
		t.Fatalf("SetHQConfig() error = %v", err)
	}

	if _, err := svc.PushCRToHQ(strconv.Itoa(cr.ID), false); err != nil {
		t.Fatalf("PushCRToHQ() error = %v", err)
	}
	if received.Patch.SchemaVersion != model.CRPatchSchemaV2 {
		t.Fatalf("expected patch schema %q, got %q", model.CRPatchSchemaV2, received.Patch.SchemaVersion)
	}
	foundDelete := false
	foundReorder := false
	for _, op := range received.Patch.Ops {
		raw := string(op)
		if strings.Contains(raw, "\"delete_task\"") {
			foundDelete = true
		}
		if strings.Contains(raw, "\"reorder_task\"") {
			foundReorder = true
		}
	}
	if !foundDelete {
		t.Fatalf("expected delete_task op, ops=%v", received.Patch.Ops)
	}
	if !foundReorder {
		t.Fatalf("expected reorder_task op, ops=%v", received.Patch.Ops)
	}
}

func TestPushCRToHQEncodesNoteDeleteWithSchemaV2(t *testing.T) {
	dir := t.TempDir()
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)

	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	cr, err := svc.AddCR("push note delete", "local removes note")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	if err := svc.AddNote(cr.ID, "keep"); err != nil {
		t.Fatalf("AddNote(keep) error = %v", err)
	}
	if err := svc.AddNote(cr.ID, "remove"); err != nil {
		t.Fatalf("AddNote(remove) error = %v", err)
	}
	loaded, err := svc.store.LoadCR(cr.ID)
	if err != nil {
		t.Fatalf("LoadCR() error = %v", err)
	}
	remote := cloneRemoteCR(loaded)
	remoteFP, err := fingerprintHQIntentCR(remote)
	if err != nil {
		t.Fatalf("fingerprintHQIntentCR(remote) error = %v", err)
	}
	loaded.Notes = []string{"keep"}
	loaded.HQ.UpstreamFingerprint = remoteFP
	if err := svc.store.SaveCR(loaded); err != nil {
		t.Fatalf("SaveCR() error = %v", err)
	}

	var received model.HQPatchApplyRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			respondRemoteCR(t, w, remote, remoteFP)
			return
		case http.MethodPost:
			if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"schema_version": "sophia.hq.v1",
				"cr_uid":         strings.TrimSpace(remote.UID),
				"cr_fingerprint": "fp_remote_after",
			})
			return
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
	}))
	defer server.Close()

	repoID := "repo-one"
	baseURL := server.URL
	if _, err := svc.SetHQConfig(HQConfigSetOptions{RepoID: &repoID, BaseURL: &baseURL}); err != nil {
		t.Fatalf("SetHQConfig() error = %v", err)
	}

	if _, err := svc.PushCRToHQ(strconv.Itoa(cr.ID), false); err != nil {
		t.Fatalf("PushCRToHQ() error = %v", err)
	}
	if received.Patch.SchemaVersion != model.CRPatchSchemaV2 {
		t.Fatalf("expected patch schema %q, got %q", model.CRPatchSchemaV2, received.Patch.SchemaVersion)
	}
	foundDeleteNote := false
	for _, op := range received.Patch.Ops {
		if strings.Contains(string(op), "\"delete_note\"") {
			foundDeleteNote = true
			break
		}
	}
	if !foundDeleteNote {
		t.Fatalf("expected delete_note op, ops=%v", received.Patch.Ops)
	}
}

func TestPushCRToHQReturnsDeterministicUnsupportedTaskSyncDetails(t *testing.T) {
	dir := t.TempDir()
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)

	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	cr, err := svc.AddCR("push unsupported task sync", "non-contiguous local task ids")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	if _, err := svc.AddTask(cr.ID, "task-1"); err != nil {
		t.Fatalf("AddTask(1) error = %v", err)
	}
	if _, err := svc.AddTask(cr.ID, "task-2"); err != nil {
		t.Fatalf("AddTask(2) error = %v", err)
	}
	loaded, err := svc.store.LoadCR(cr.ID)
	if err != nil {
		t.Fatalf("LoadCR() error = %v", err)
	}
	remote := cloneRemoteCR(loaded)
	remoteFP, err := fingerprintHQIntentCR(remote)
	if err != nil {
		t.Fatalf("fingerprintHQIntentCR(remote) error = %v", err)
	}

	loaded.Subtasks[1].ID = 4
	loaded.HQ.UpstreamFingerprint = remoteFP
	if err := svc.store.SaveCR(loaded); err != nil {
		t.Fatalf("SaveCR() error = %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		respondRemoteCR(t, w, remote, remoteFP)
	}))
	defer server.Close()

	repoID := "repo-one"
	baseURL := server.URL
	if _, err := svc.SetHQConfig(HQConfigSetOptions{RepoID: &repoID, BaseURL: &baseURL}); err != nil {
		t.Fatalf("SetHQConfig() error = %v", err)
	}

	_, err = svc.PushCRToHQ(strconv.Itoa(cr.ID), false)
	if err == nil {
		t.Fatalf("expected PushCRToHQ to fail with unsupported task sync")
	}
	if !errors.Is(err, ErrHQTaskSyncUnsupported) {
		t.Fatalf("expected ErrHQTaskSyncUnsupported, got %T (%v)", err, err)
	}
	var unsupported *HQTaskSyncUnsupportedError
	if !errors.As(err, &unsupported) {
		t.Fatalf("expected HQTaskSyncUnsupportedError, got %T", err)
	}
	if unsupported.RemoteMaxTaskID != 2 {
		t.Fatalf("expected remote max task id 2, got %d", unsupported.RemoteMaxTaskID)
	}
	if len(unsupported.MissingLocalTask) != 1 || unsupported.MissingLocalTask[0] != 4 {
		t.Fatalf("expected missing local task ids [4], got %#v", unsupported.MissingLocalTask)
	}
}

func TestPushCRToHQRefusesWhenUpstreamMoved(t *testing.T) {
	dir := t.TempDir()
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)

	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	cr, err := svc.AddCR("push moved", "upstream changed")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	loaded, err := svc.store.LoadCR(cr.ID)
	if err != nil {
		t.Fatalf("LoadCR() error = %v", err)
	}
	loaded.HQ.UpstreamFingerprint = "fp_old"
	if err := svc.store.SaveCR(loaded); err != nil {
		t.Fatalf("SaveCR() error = %v", err)
	}

	remote := cloneRemoteCR(loaded)
	remote.Title = "Remote changed title"
	remoteFP, err := fingerprintHQIntentCR(remote)
	if err != nil {
		t.Fatalf("fingerprintHQIntentCR(remote) error = %v", err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		respondRemoteCR(t, w, remote, remoteFP)
	}))
	defer server.Close()

	repoID := "repo-one"
	baseURL := server.URL
	if _, err := svc.SetHQConfig(HQConfigSetOptions{RepoID: &repoID, BaseURL: &baseURL}); err != nil {
		t.Fatalf("SetHQConfig() error = %v", err)
	}

	_, err = svc.PushCRToHQ(strconv.Itoa(cr.ID), false)
	if err == nil {
		t.Fatalf("expected PushCRToHQ to fail when upstream moved")
	}
	if !errors.Is(err, ErrHQUpstreamMoved) {
		t.Fatalf("expected ErrHQUpstreamMoved, got %T (%v)", err, err)
	}
	var moved *HQUpstreamMovedError
	if !errors.As(err, &moved) {
		t.Fatalf("expected HQUpstreamMovedError, got %T", err)
	}
	if moved.RemoteFingerprint != remoteFP {
		t.Fatalf("expected remote fingerprint %q, got %q", remoteFP, moved.RemoteFingerprint)
	}
}

func TestPullCRFromHQFastForwardUpdatesLocalIntent(t *testing.T) {
	dir := t.TempDir()
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)

	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	cr, err := svc.AddCR("pull baseline", "baseline")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	loaded, err := svc.store.LoadCR(cr.ID)
	if err != nil {
		t.Fatalf("LoadCR() error = %v", err)
	}
	baseFP, err := fingerprintHQIntentCR(loaded)
	if err != nil {
		t.Fatalf("fingerprintHQIntentCR(local) error = %v", err)
	}
	loaded.HQ.UpstreamFingerprint = baseFP
	if err := svc.store.SaveCR(loaded); err != nil {
		t.Fatalf("SaveCR() error = %v", err)
	}

	remote := cloneRemoteCR(loaded)
	remote.Title = "Remote updated title"
	remoteFP, err := fingerprintHQIntentCR(remote)
	if err != nil {
		t.Fatalf("fingerprintHQIntentCR(remote) error = %v", err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		respondRemoteCR(t, w, remote, remoteFP)
	}))
	defer server.Close()

	repoID := "repo-one"
	baseURL := server.URL
	if _, err := svc.SetHQConfig(HQConfigSetOptions{RepoID: &repoID, BaseURL: &baseURL}); err != nil {
		t.Fatalf("SetHQConfig() error = %v", err)
	}

	result, err := svc.PullCRFromHQ(strconv.Itoa(cr.ID), false)
	if err != nil {
		t.Fatalf("PullCRFromHQ() error = %v", err)
	}
	if !result.Updated || result.LocalAhead || result.UpToDate {
		t.Fatalf("expected updated pull result, got %#v", result)
	}
	after, err := svc.store.LoadCR(cr.ID)
	if err != nil {
		t.Fatalf("LoadCR(after pull) error = %v", err)
	}
	if after.Title != "Remote updated title" {
		t.Fatalf("expected title to be updated from remote, got %q", after.Title)
	}
	if after.HQ.UpstreamFingerprint != remoteFP {
		t.Fatalf("expected upstream fingerprint %q, got %q", remoteFP, after.HQ.UpstreamFingerprint)
	}
}

func TestPullCRFromHQDivergedReturnsConflictDetails(t *testing.T) {
	dir := t.TempDir()
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)

	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	cr, err := svc.AddCR("diverge baseline", "baseline")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	loaded, err := svc.store.LoadCR(cr.ID)
	if err != nil {
		t.Fatalf("LoadCR() error = %v", err)
	}
	baseFP, err := fingerprintHQIntentCR(loaded)
	if err != nil {
		t.Fatalf("fingerprintHQIntentCR(base) error = %v", err)
	}
	loaded.HQ.UpstreamFingerprint = baseFP
	loaded.Title = "Local changed title"
	if err := svc.store.SaveCR(loaded); err != nil {
		t.Fatalf("SaveCR(local changed) error = %v", err)
	}

	remote := cloneRemoteCR(cr)
	remote.Title = "Remote changed title"
	remoteFP, err := fingerprintHQIntentCR(remote)
	if err != nil {
		t.Fatalf("fingerprintHQIntentCR(remote) error = %v", err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		respondRemoteCR(t, w, remote, remoteFP)
	}))
	defer server.Close()

	repoID := "repo-one"
	baseURL := server.URL
	if _, err := svc.SetHQConfig(HQConfigSetOptions{RepoID: &repoID, BaseURL: &baseURL}); err != nil {
		t.Fatalf("SetHQConfig() error = %v", err)
	}

	_, err = svc.PullCRFromHQ(strconv.Itoa(cr.ID), false)
	if err == nil {
		t.Fatalf("expected PullCRFromHQ() to return divergence error")
	}
	if !errors.Is(err, ErrHQIntentDiverged) {
		t.Fatalf("expected ErrHQIntentDiverged, got %T (%v)", err, err)
	}
	var diverged *HQIntentDivergedError
	if !errors.As(err, &diverged) {
		t.Fatalf("expected HQIntentDivergedError, got %T", err)
	}
	if len(diverged.Conflicts) == 0 {
		t.Fatalf("expected non-empty conflicts")
	}
	foundTitle := false
	for _, conflict := range diverged.Conflicts {
		if conflict.Field == "cr.title" {
			foundTitle = true
			break
		}
	}
	if !foundTitle {
		t.Fatalf("expected cr.title conflict, got %#v", diverged.Conflicts)
	}
}

func TestPullCRFromHQPreservesCheckpointMetadata(t *testing.T) {
	dir := t.TempDir()
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)

	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	cr, err := svc.AddCR("pull checkpoint preserve", "baseline")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	task, err := svc.AddTask(cr.ID, "Task one")
	if err != nil {
		t.Fatalf("AddTask() error = %v", err)
	}
	loaded, err := svc.store.LoadCR(cr.ID)
	if err != nil {
		t.Fatalf("LoadCR() error = %v", err)
	}
	for i := range loaded.Subtasks {
		if loaded.Subtasks[i].ID != task.ID {
			continue
		}
		loaded.Subtasks[i].CheckpointCommit = "abc123"
		loaded.Subtasks[i].CheckpointSource = "task_checkpoint"
		loaded.Subtasks[i].CheckpointScope = []string{"internal/service"}
	}
	baseFP, err := fingerprintHQIntentCR(loaded)
	if err != nil {
		t.Fatalf("fingerprintHQIntentCR(base) error = %v", err)
	}
	loaded.HQ.UpstreamFingerprint = baseFP
	if err := svc.store.SaveCR(loaded); err != nil {
		t.Fatalf("SaveCR() error = %v", err)
	}

	remote := cloneRemoteCR(loaded)
	for i := range remote.Subtasks {
		if remote.Subtasks[i].ID != task.ID {
			continue
		}
		remote.Subtasks[i].Title = "Task one remote updated"
		remote.Subtasks[i].CheckpointCommit = ""
		remote.Subtasks[i].CheckpointSource = ""
		remote.Subtasks[i].CheckpointScope = nil
	}
	remoteFP, err := fingerprintHQIntentCR(remote)
	if err != nil {
		t.Fatalf("fingerprintHQIntentCR(remote) error = %v", err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		respondRemoteCR(t, w, remote, remoteFP)
	}))
	defer server.Close()

	repoID := "repo-one"
	baseURL := server.URL
	if _, err := svc.SetHQConfig(HQConfigSetOptions{RepoID: &repoID, BaseURL: &baseURL}); err != nil {
		t.Fatalf("SetHQConfig() error = %v", err)
	}

	if _, err := svc.PullCRFromHQ(strconv.Itoa(cr.ID), false); err != nil {
		t.Fatalf("PullCRFromHQ() error = %v", err)
	}
	after, err := svc.store.LoadCR(cr.ID)
	if err != nil {
		t.Fatalf("LoadCR(after pull) error = %v", err)
	}
	for _, st := range after.Subtasks {
		if st.ID != task.ID {
			continue
		}
		if st.Title != "Task one remote updated" {
			t.Fatalf("expected remote task title update, got %q", st.Title)
		}
		if st.CheckpointCommit != "abc123" {
			t.Fatalf("expected checkpoint commit preserved, got %q", st.CheckpointCommit)
		}
		if st.CheckpointSource != "task_checkpoint" {
			t.Fatalf("expected checkpoint source preserved, got %q", st.CheckpointSource)
		}
		return
	}
	t.Fatalf("expected task %d after pull", task.ID)
}

func respondRemoteCR(t *testing.T, w http.ResponseWriter, cr *model.CR, fingerprint string) {
	t.Helper()
	doc := canonicalCRDoc(cr)
	raw, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("marshal remote doc: %v", err)
	}
	var decoded any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("decode remote doc: %v", err)
	}
	_ = json.NewEncoder(w).Encode(map[string]any{
		"schema_version": "sophia.hq.v1",
		"cr_uid":         strings.TrimSpace(cr.UID),
		"cr_fingerprint": strings.TrimSpace(fingerprint),
		"doc":            decoded,
	})
}
