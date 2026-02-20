package service

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHQSyncStatusNotConfigured(t *testing.T) {
	dir := t.TempDir()
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)

	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if _, err := svc.AddCR("HQ status", "fixture"); err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}

	view, err := svc.HQSyncStatusCR(1)
	if err != nil {
		t.Fatalf("HQSyncStatusCR() error = %v", err)
	}
	if view.Configured {
		t.Fatalf("expected configured=false, got %#v", view)
	}
	if view.State != "not_configured" {
		t.Fatalf("expected state=not_configured, got %#v", view.State)
	}
	if view.RemoteChecked || view.RemoteExists {
		t.Fatalf("expected no remote check when not configured, got %#v", view)
	}
}

func TestHQSyncStatusStates(t *testing.T) {
	type fixture struct {
		name   string
		setup  func(t *testing.T, svc *Service, crID int, remoteFP *string)
		want   string
		checks func(t *testing.T, view *HQSyncStatusView)
	}

	mkServer := func(t *testing.T, repoID string, uid string, remoteFP *string, missing bool) *httptest.Server {
		t.Helper()
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			want := "/api/v1/repos/" + repoID + "/crs/" + uid
			if r.URL.Path != want {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			if missing {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"schema_version": "sophia.hq.v1",
				"cr_uid":         uid,
				"cr_fingerprint": strings.TrimSpace(*remoteFP),
			})
		}))
	}

	cases := []fixture{
		{
			name: "remote_missing",
			setup: func(t *testing.T, svc *Service, crID int, remoteFP *string) {
				t.Helper()
				// Nothing else: server returns 404.
			},
			want: "remote_missing",
			checks: func(t *testing.T, view *HQSyncStatusView) {
				t.Helper()
				if view.RemoteExists {
					t.Fatalf("expected remote_exists=false, got %#v", view)
				}
				if len(view.SuggestedActions) == 0 {
					t.Fatalf("expected suggested_actions, got %#v", view)
				}
			},
		},
		{
			name: "unlinked",
			setup: func(t *testing.T, svc *Service, crID int, remoteFP *string) {
				t.Helper()
				// Upstream fingerprint empty by default.
			},
			want: "unlinked",
			checks: func(t *testing.T, view *HQSyncStatusView) {
				t.Helper()
				if !view.RemoteExists {
					t.Fatalf("expected remote_exists=true, got %#v", view)
				}
				if view.Linked {
					t.Fatalf("expected linked=false, got %#v", view)
				}
			},
		},
		{
			name: "up_to_date",
			setup: func(t *testing.T, svc *Service, crID int, remoteFP *string) {
				t.Helper()
				cr, err := svc.store.LoadCR(crID)
				if err != nil {
					t.Fatalf("LoadCR() error = %v", err)
				}
				fp, err := fingerprintHQIntentCR(cr)
				if err != nil {
					t.Fatalf("fingerprintHQIntentCR() error = %v", err)
				}
				*remoteFP = fp
				cr.HQ.UpstreamFingerprint = fp
				if err := svc.store.SaveCR(cr); err != nil {
					t.Fatalf("SaveCR() error = %v", err)
				}
			},
			want: "up_to_date",
		},
		{
			name: "local_ahead",
			setup: func(t *testing.T, svc *Service, crID int, remoteFP *string) {
				t.Helper()
				cr, err := svc.store.LoadCR(crID)
				if err != nil {
					t.Fatalf("LoadCR() error = %v", err)
				}
				// Remote and upstream match baseline.
				*remoteFP = "upstream"
				cr.HQ.UpstreamFingerprint = "upstream"
				// Change local intent so local fingerprint differs.
				cr.Contract.Why = "local changed"
				if err := svc.store.SaveCR(cr); err != nil {
					t.Fatalf("SaveCR() error = %v", err)
				}
			},
			want: "local_ahead",
		},
		{
			name: "remote_ahead",
			setup: func(t *testing.T, svc *Service, crID int, remoteFP *string) {
				t.Helper()
				cr, err := svc.store.LoadCR(crID)
				if err != nil {
					t.Fatalf("LoadCR() error = %v", err)
				}
				fp, err := fingerprintHQIntentCR(cr)
				if err != nil {
					t.Fatalf("fingerprintHQIntentCR() error = %v", err)
				}
				cr.HQ.UpstreamFingerprint = fp
				if err := svc.store.SaveCR(cr); err != nil {
					t.Fatalf("SaveCR() error = %v", err)
				}
				*remoteFP = "remote-changed"
			},
			want: "remote_ahead",
		},
		{
			name: "diverged",
			setup: func(t *testing.T, svc *Service, crID int, remoteFP *string) {
				t.Helper()
				cr, err := svc.store.LoadCR(crID)
				if err != nil {
					t.Fatalf("LoadCR() error = %v", err)
				}
				cr.HQ.UpstreamFingerprint = "upstream"
				cr.Contract.Why = "local changed"
				if err := svc.store.SaveCR(cr); err != nil {
					t.Fatalf("SaveCR() error = %v", err)
				}
				*remoteFP = "remote changed"
			},
			want: "diverged",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			configHome := t.TempDir()
			t.Setenv("XDG_CONFIG_HOME", configHome)

			svc := New(dir)
			if _, err := svc.Init("main", ""); err != nil {
				t.Fatalf("Init() error = %v", err)
			}
			cr, err := svc.AddCR("HQ status", "fixture")
			if err != nil {
				t.Fatalf("AddCR() error = %v", err)
			}

			repoID := "repo-one"
			remoteFP := "remote-fp"
			ts := mkServer(t, repoID, cr.UID, &remoteFP, tc.name == "remote_missing")
			defer ts.Close()

			baseURL := ts.URL
			if _, err := svc.SetHQConfig(HQConfigSetOptions{RepoID: &repoID, BaseURL: &baseURL}); err != nil {
				t.Fatalf("SetHQConfig() error = %v", err)
			}

			tc.setup(t, svc, cr.ID, &remoteFP)

			view, err := svc.HQSyncStatusCR(cr.ID)
			if err != nil {
				t.Fatalf("HQSyncStatusCR() error = %v", err)
			}
			if view.State != tc.want {
				t.Fatalf("expected state %q, got %#v", tc.want, view)
			}
			if tc.checks != nil {
				tc.checks(t, view)
			}
		})
	}
}
