package cli

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"sophia/internal/model"
	"sophia/internal/service"
)

func TestCRShowJSONUsesLocalhostPreview(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := service.New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if _, _, err := svc.AddCRWithOptionsWithWarnings("Show report", "read-only report fixture", service.AddCROptions{NoSwitch: true}); err != nil {
		t.Fatalf("AddCRWithOptionsWithWarnings() error = %v", err)
	}

	out, _, runErr := runCLI(t, dir, "cr", "show", "1", "--json", "--no-open")
	if runErr != nil {
		t.Fatalf("cr show --json error = %v\noutput=%s", runErr, out)
	}
	env := decodeEnvelope(t, out)
	if !env.OK {
		t.Fatalf("expected ok envelope, got %#v", env)
	}

	if got := int(jsonNumberField(t, env.Data, "cr_id")); got != 1 {
		t.Fatalf("expected cr_id=1, got %d", got)
	}
	if got, _ := env.Data["view_mode"].(string); got != "localhost_ephemeral" {
		t.Fatalf("expected localhost_ephemeral view_mode, got %#v", env.Data["view_mode"])
	}
	if _, exists := env.Data["path"]; exists {
		t.Fatalf("did not expect path field in payload, got %#v", env.Data["path"])
	}
	if gotURL, _ := env.Data["url"].(string); strings.TrimSpace(gotURL) != "" {
		t.Fatalf("expected empty url with --no-open, got %q", gotURL)
	}
	if opened, ok := env.Data["opened"].(bool); !ok || opened {
		t.Fatalf("expected opened=false with --no-open, got %#v", env.Data["opened"])
	}
	if pageServed, ok := env.Data["page_served"].(bool); !ok || pageServed {
		t.Fatalf("expected page_served=false with --no-open, got %#v", env.Data["page_served"])
	}
}

func TestCRShowJSONMissingInProgressBranchFallsBackToMetadataOnlyPreview(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := service.New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	cr, _, err := svc.AddCRWithOptionsWithWarnings("Show metadata-only fallback", "render without strict head anchor", service.AddCROptions{NoSwitch: true})
	if err != nil {
		t.Fatalf("AddCRWithOptionsWithWarnings() error = %v", err)
	}
	runGit(t, dir, "branch", "-D", cr.Branch)

	out, _, runErr := runCLI(t, dir, "cr", "show", "1", "--json", "--no-open")
	if runErr != nil {
		t.Fatalf("cr show --json --no-open error = %v\noutput=%s", runErr, out)
	}
	env := decodeEnvelope(t, out)
	if !env.OK {
		t.Fatalf("expected ok envelope, got %#v", env)
	}
	warnings := requireJSONArrayField(t, env.Data, "warnings")
	if len(warnings) == 0 {
		t.Fatalf("expected metadata-only fallback warnings, got %#v", warnings)
	}
	found := false
	for _, raw := range warnings {
		warning, _ := raw.(string)
		if strings.Contains(strings.ToLower(warning), "metadata-only") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected metadata-only warning, got %#v", warnings)
	}
}

func TestCRShowJSONMissingAbandonedBranchFallsBackToMetadataOnlyPreview(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := service.New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	cr, _, err := svc.AddCRWithOptionsWithWarnings("Show abandoned metadata-only fallback", "render abandoned CR without strict head anchor", service.AddCROptions{Switch: true})
	if err != nil {
		t.Fatalf("AddCRWithOptionsWithWarnings() error = %v", err)
	}
	if _, err := svc.AbandonCR(cr.ID, service.CRAbandonOptions{Reason: "testing fallback"}); err != nil {
		t.Fatalf("AbandonCR() error = %v", err)
	}

	out, _, runErr := runCLI(t, dir, "cr", "show", "1", "--json", "--no-open")
	if runErr != nil {
		t.Fatalf("cr show --json --no-open error = %v\noutput=%s", runErr, out)
	}
	env := decodeEnvelope(t, out)
	if !env.OK {
		t.Fatalf("expected ok envelope, got %#v", env)
	}
	warnings := requireJSONArrayField(t, env.Data, "warnings")
	if len(warnings) == 0 {
		t.Fatalf("expected metadata-only fallback warnings, got %#v", warnings)
	}
	found := false
	for _, raw := range warnings {
		warning, _ := raw.(string)
		if strings.Contains(strings.ToLower(warning), "metadata-only") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected metadata-only warning, got %#v", warnings)
	}
}

func TestCRShowJSONWithoutContextFallsBackToDashboard(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := service.New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if _, _, err := svc.AddCRWithOptionsWithWarnings("Dashboard report", "dashboard fallback fixture", service.AddCROptions{NoSwitch: true}); err != nil {
		t.Fatalf("AddCRWithOptionsWithWarnings() error = %v", err)
	}

	out, _, runErr := runCLI(t, dir, "cr", "show", "--json", "--no-open")
	if runErr != nil {
		t.Fatalf("cr show --json --no-open error = %v\noutput=%s", runErr, out)
	}
	env := decodeEnvelope(t, out)
	if !env.OK {
		t.Fatalf("expected ok envelope, got %#v", env)
	}
	if got, _ := env.Data["view_mode"].(string); got != "localhost_dashboard" {
		t.Fatalf("expected localhost_dashboard view_mode, got %#v", env.Data["view_mode"])
	}
	if got, _ := env.Data["template_source"].(string); got != "embedded:internal/cli/templates/cr_list.html" {
		t.Fatalf("expected cr_list template source, got %#v", env.Data["template_source"])
	}
	if _, exists := env.Data["selected_cr_id"]; !exists {
		t.Fatalf("expected selected_cr_id in dashboard envelope, got %#v", env.Data)
	}
}

func TestCRShowJSONDashboardFlagForcesDashboardMode(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := service.New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if _, _, err := svc.AddCRWithOptionsWithWarnings("Dashboard report", "dashboard forced fixture", service.AddCROptions{Switch: true}); err != nil {
		t.Fatalf("AddCRWithOptionsWithWarnings() error = %v", err)
	}

	out, _, runErr := runCLI(t, dir, "cr", "show", "--dashboard", "--json", "--no-open")
	if runErr != nil {
		t.Fatalf("cr show --dashboard --json --no-open error = %v\noutput=%s", runErr, out)
	}
	env := decodeEnvelope(t, out)
	if !env.OK {
		t.Fatalf("expected ok envelope, got %#v", env)
	}
	if got, _ := env.Data["view_mode"].(string); got != "localhost_dashboard" {
		t.Fatalf("expected localhost_dashboard view_mode, got %#v", env.Data["view_mode"])
	}
}

func TestBuildCRShowSnapshotIncludesStaleAndAbandonedLifecycleMetadata(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := service.New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SOPHIA.yaml"), []byte("version: v1\nmerge:\n  mode: pr_gate\narchive:\n  enabled: false\n"), 0o644); err != nil {
		t.Fatalf("write SOPHIA.yaml: %v", err)
	}
	cr, err := svc.AddCR("Show stale lifecycle", "snapshot metadata")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")
	if err := os.WriteFile(filepath.Join(dir, "show-lifecycle.txt"), []byte("seed\n"), 0o644); err != nil {
		t.Fatalf("write show-lifecycle.txt: %v", err)
	}
	runGit(t, dir, "add", "show-lifecycle.txt")
	runGit(t, dir, "commit", "-m", "feat: seed show lifecycle fixture")

	_, payload, err := buildCRShowSnapshot(svc, cr.ID, defaultCRShowEventsLimit, defaultCRShowCheckpointsLimit)
	if err != nil {
		t.Fatalf("buildCRShowSnapshot() error = %v", err)
	}
	statusMap := mapStringAny(payload["status"])
	if got, _ := statusMap["pr_linkage_state"].(string); got != "no_linked_pr" {
		t.Fatalf("expected pr_linkage_state=no_linked_pr, got %#v", statusMap["pr_linkage_state"])
	}
	if got, _ := statusMap["action_required"].(string); got != "open_pr" {
		t.Fatalf("expected action_required=open_pr, got %#v", statusMap["action_required"])
	}

	if _, err := svc.AbandonCR(cr.ID, service.CRAbandonOptions{Reason: "deferred"}); err != nil {
		t.Fatalf("AbandonCR() error = %v", err)
	}
	payload, selectedID, err := buildCRDashboardSnapshot(svc, model.CRSearchQuery{}, defaultCRListLimit, defaultCRTimelineLimit, cr.ID)
	if err != nil {
		t.Fatalf("buildCRDashboardSnapshot() after abandon error = %v", err)
	}
	if selectedID != 0 {
		t.Fatalf("expected no selected id after default dashboard filtering, got %d", selectedID)
	}
	selected := mapStringAny(payload["selected_cr"])
	if len(selected) != 0 {
		t.Fatalf("expected selected_cr to be empty when abandoned is default-filtered, got %#v", selected)
	}
	rows, ok := payload["crs"].([]map[string]any)
	if !ok {
		t.Fatalf("expected crs payload array, got %#v", payload["crs"])
	}
	if len(rows) != 0 {
		t.Fatalf("expected no dashboard rows after default filtering, got %d", len(rows))
	}
	timeline, ok := payload["timeline"].([]map[string]any)
	if !ok {
		t.Fatalf("expected timeline payload array, got %#v", payload["timeline"])
	}
	if len(timeline) != 0 {
		t.Fatalf("expected no timeline entries after default filtering, got %d", len(timeline))
	}

	payload, selectedID, err = buildCRDashboardSnapshot(svc, model.CRSearchQuery{Status: model.StatusAbandoned}, defaultCRListLimit, defaultCRTimelineLimit, cr.ID)
	if err != nil {
		t.Fatalf("buildCRDashboardSnapshot() with abandoned filter error = %v", err)
	}
	if selectedID != cr.ID {
		t.Fatalf("expected selected id %d when status=abandoned, got %d", cr.ID, selectedID)
	}
	selected = mapStringAny(payload["selected_cr"])
	if got, _ := selected["status"].(string); got != "abandoned" {
		t.Fatalf("expected selected status=abandoned, got %#v", selected["status"])
	}
	if got, _ := selected["abandoned_reason"].(string); got != "deferred" {
		t.Fatalf("expected abandoned_reason=deferred, got %#v", selected["abandoned_reason"])
	}
	if got, _ := selected["action_required"].(string); got != "reopen_cr" {
		t.Fatalf("expected action_required=reopen_cr for abandoned CR, got %#v", selected["action_required"])
	}
}

func TestCRShowDashboardJSONDefaultExcludesAbandonedInCounts(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := service.New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if _, _, err := svc.AddCRWithOptionsWithWarnings("Dashboard active CR", "default dashboard result", service.AddCROptions{NoSwitch: true}); err != nil {
		t.Fatalf("AddCRWithOptionsWithWarnings(active) error = %v", err)
	}
	abandoned, _, err := svc.AddCRWithOptionsWithWarnings("Dashboard abandoned CR", "explicit abandoned status filter result", service.AddCROptions{NoSwitch: true})
	if err != nil {
		t.Fatalf("AddCRWithOptionsWithWarnings(abandoned) error = %v", err)
	}
	if _, err := svc.AbandonCR(abandoned.ID, service.CRAbandonOptions{Reason: "deferred"}); err != nil {
		t.Fatalf("AbandonCR() error = %v", err)
	}

	out, _, runErr := runCLI(t, dir, "cr", "show", "--dashboard", "--json", "--no-open")
	if runErr != nil {
		t.Fatalf("cr show --dashboard --json --no-open error = %v\noutput=%s", runErr, out)
	}
	env := decodeEnvelope(t, out)
	if !env.OK {
		t.Fatalf("expected ok envelope, got %#v", env)
	}
	countsRaw, ok := env.Data["counts"]
	if !ok {
		t.Fatalf("expected counts in response payload, got %#v", env.Data)
	}
	counts, ok := countsRaw.(map[string]any)
	if !ok {
		t.Fatalf("expected counts object, got %#v", countsRaw)
	}
	if got := int(jsonNumberField(t, counts, "list_total")); got != 1 {
		t.Fatalf("expected list_total=1 with default abandoned exclusion, got %d", got)
	}

	out, _, runErr = runCLI(t, dir, "cr", "show", "--dashboard", "--status", "abandoned", "--json", "--no-open")
	if runErr != nil {
		t.Fatalf("cr show --dashboard --status abandoned --json --no-open error = %v\noutput=%s", runErr, out)
	}
	env = decodeEnvelope(t, out)
	if !env.OK {
		t.Fatalf("expected ok envelope for --status abandoned, got %#v", env)
	}
	countsRaw, ok = env.Data["counts"]
	if !ok {
		t.Fatalf("expected counts in abandoned response payload, got %#v", env.Data)
	}
	counts, ok = countsRaw.(map[string]any)
	if !ok {
		t.Fatalf("expected counts object for abandoned response, got %#v", countsRaw)
	}
	if got := int(jsonNumberField(t, counts, "list_total")); got != 1 {
		t.Fatalf("expected list_total=1 for status=abandoned, got %d", got)
	}
}

func TestBuildCRDashboardSnapshotIncludesSelectedCRLifecycleActions(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := service.New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SOPHIA.yaml"), []byte("version: v1\nmerge:\n  mode: pr_gate\narchive:\n  enabled: false\n"), 0o644); err != nil {
		t.Fatalf("write SOPHIA.yaml: %v", err)
	}
	cr, err := svc.AddCR("Dashboard lifecycle", "selected CR metadata")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}

	payload, selectedID, err := buildCRDashboardSnapshot(svc, model.CRSearchQuery{}, defaultCRListLimit, defaultCRTimelineLimit, cr.ID)
	if err != nil {
		t.Fatalf("buildCRDashboardSnapshot() error = %v", err)
	}
	if selectedID != cr.ID {
		t.Fatalf("expected selected id %d, got %d", cr.ID, selectedID)
	}
	selected := mapStringAny(payload["selected_cr"])
	if got, _ := selected["pr_linkage_state"].(string); got != "no_linked_pr" {
		t.Fatalf("expected selected pr_linkage_state=no_linked_pr, got %#v", selected["pr_linkage_state"])
	}
	if got, _ := selected["action_required"].(string); got != "open_pr" {
		t.Fatalf("expected selected action_required=open_pr, got %#v", selected["action_required"])
	}
}

func TestCRShowTemplateIsSingleFileWithInlineAssets(t *testing.T) {
	t.Parallel()
	doc, err := buildCRShowHTMLDocument(embeddedCRShowHTMLTemplate, map[string]any{
		"generated_at": "2026-03-03T00:00:00Z",
		"cr": map[string]any{
			"id":    101,
			"title": "Template inline test",
		},
	})
	if err != nil {
		t.Fatalf("buildCRShowHTMLDocument() error = %v", err)
	}

	for _, required := range []string{
		"<style>",
		"<script id=\"cr-show-data\" type=\"application/json\">",
		"Raw JSON Payload",
		"read-only local report",
		"id=\"close-preview-btn\"",
		"EventSource",
		"/__sophia_events?mode=cr&id=",
	} {
		if !strings.Contains(doc, required) {
			t.Fatalf("expected generated html to contain %q", required)
		}
	}
	for _, forbidden := range []string{
		"<script src=\"http://",
		"<script src=\"https://",
	} {
		if strings.Contains(doc, forbidden) {
			t.Fatalf("expected no remote script dependency in template; found %q", forbidden)
		}
	}
}

func TestCRListTemplateIsSingleFileWithInlineAssets(t *testing.T) {
	t.Parallel()
	doc, err := buildCRListHTMLDocument(embeddedCRListHTMLTemplate, map[string]any{
		"generated_at": "2026-03-03T00:00:00Z",
		"dashboard": map[string]any{
			"selected_cr_id": 101,
		},
		"selected_cr": map[string]any{
			"id":    101,
			"title": "Dashboard template test",
		},
	})
	if err != nil {
		t.Fatalf("buildCRListHTMLDocument() error = %v", err)
	}

	for _, required := range []string{
		"<style>",
		"<script id=\"cr-list-data\" type=\"application/json\">",
		"Raw JSON Payload",
		"read-only local report",
		"id=\"close-preview-btn\"",
		"EventSource",
		"/__sophia_events?mode=dashboard",
	} {
		if !strings.Contains(doc, required) {
			t.Fatalf("expected generated html to contain %q", required)
		}
	}
	for _, forbidden := range []string{
		"<script src=\"http://",
		"<script src=\"https://",
	} {
		if strings.Contains(doc, forbidden) {
			t.Fatalf("expected no remote script dependency in template; found %q", forbidden)
		}
	}
}

func TestCRShowServerSupportsCloseEndpoint(t *testing.T) {
	t.Parallel()
	server, err := startCRShowServer(func() (string, error) {
		return "<!doctype html><html><body>ok</body></html>", nil
	})
	if err != nil {
		t.Fatalf("startCRShowServer() error = %v", err)
	}
	defer server.Shutdown()

	resp, err := http.Get(server.URL + "/")
	if err != nil {
		t.Fatalf("GET / error = %v", err)
	}
	_, _ = io.ReadAll(resp.Body)
	_ = resp.Body.Close()

	if !server.WaitForFirstRender(2 * time.Second) {
		t.Fatalf("expected WaitForFirstRender to observe first render")
	}

	req, err := http.NewRequest(http.MethodPost, server.URL+"/__sophia_close", nil)
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}
	closeResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /__sophia_close error = %v", err)
	}
	_, _ = io.ReadAll(closeResp.Body)
	_ = closeResp.Body.Close()

	select {
	case reason := <-server.closedCh:
		if reason != "ui_close_button" {
			t.Fatalf("expected ui_close_button close reason, got %q", reason)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for close signal")
	}
}

func TestCRShowServerRendersFreshDocumentPerRequest(t *testing.T) {
	t.Parallel()
	renderCount := 0
	server, err := startCRShowServer(func() (string, error) {
		renderCount++
		return fmt.Sprintf("<!doctype html><html><body>render-%d</body></html>", renderCount), nil
	})
	if err != nil {
		t.Fatalf("startCRShowServer() error = %v", err)
	}
	defer server.Shutdown()

	respOne, err := http.Get(server.URL + "/")
	if err != nil {
		t.Fatalf("GET #1 error = %v", err)
	}
	bodyOneRaw, _ := io.ReadAll(respOne.Body)
	_ = respOne.Body.Close()
	bodyOne := string(bodyOneRaw)

	respTwo, err := http.Get(server.URL + "/")
	if err != nil {
		t.Fatalf("GET #2 error = %v", err)
	}
	bodyTwoRaw, _ := io.ReadAll(respTwo.Body)
	_ = respTwo.Body.Close()
	bodyTwo := string(bodyTwoRaw)

	if bodyOne == bodyTwo {
		t.Fatalf("expected fresh document per request, got identical body %q", bodyOne)
	}
	if !strings.Contains(bodyOne, "render-1") || !strings.Contains(bodyTwo, "render-2") {
		t.Fatalf("unexpected render bodies: bodyOne=%q bodyTwo=%q", bodyOne, bodyTwo)
	}
}

func TestCRShowServerSupportsDashboardAndCRRoutes(t *testing.T) {
	t.Parallel()
	server, err := startCRShowServerWithRoutes(
		func() (string, error) {
			return "<!doctype html><html><body>dashboard</body></html>", nil
		},
		func(id int) (string, error) {
			return fmt.Sprintf("<!doctype html><html><body>cr-%d</body></html>", id), nil
		},
	)
	if err != nil {
		t.Fatalf("startCRShowServerWithRoutes() error = %v", err)
	}
	defer server.Shutdown()

	respDashboard, err := http.Get(server.URL + "/")
	if err != nil {
		t.Fatalf("GET / error = %v", err)
	}
	bodyDashboardRaw, _ := io.ReadAll(respDashboard.Body)
	_ = respDashboard.Body.Close()
	if respDashboard.StatusCode != http.StatusOK {
		t.Fatalf("expected dashboard status 200, got %d", respDashboard.StatusCode)
	}
	bodyDashboard := string(bodyDashboardRaw)
	if !strings.Contains(bodyDashboard, "dashboard") {
		t.Fatalf("expected dashboard body, got %q", bodyDashboard)
	}

	respCR, err := http.Get(server.URL + "/42")
	if err != nil {
		t.Fatalf("GET /42 error = %v", err)
	}
	bodyCRRaw, _ := io.ReadAll(respCR.Body)
	_ = respCR.Body.Close()
	if respCR.StatusCode != http.StatusOK {
		t.Fatalf("expected /42 status 200, got %d", respCR.StatusCode)
	}
	bodyCR := string(bodyCRRaw)
	if !strings.Contains(bodyCR, "cr-42") {
		t.Fatalf("expected cr route body, got %q", bodyCR)
	}

	respMissing, err := http.Get(server.URL + "/bad-route")
	if err != nil {
		t.Fatalf("GET /bad-route error = %v", err)
	}
	_, _ = io.ReadAll(respMissing.Body)
	_ = respMissing.Body.Close()
	if respMissing.StatusCode != http.StatusNotFound {
		t.Fatalf("expected /bad-route status 404, got %d", respMissing.StatusCode)
	}
}

func TestCRShowServerSSEInitialSnapshot(t *testing.T) {
	t.Parallel()
	server, err := startCRShowServerWithLiveRoutes(
		func() (string, error) { return "<!doctype html><html><body>dashboard</body></html>", nil },
		func(id int) (string, error) {
			return fmt.Sprintf("<!doctype html><html><body>cr-%d</body></html>", id), nil
		},
		func() (map[string]any, error) {
			return map[string]any{"mode": "dashboard", "generated_at": "2026-03-03T00:00:00Z"}, nil
		},
		func(id int) (map[string]any, error) {
			return map[string]any{"mode": "cr", "cr_id": id, "generated_at": "2026-03-03T00:00:00Z"}, nil
		},
	)
	if err != nil {
		t.Fatalf("startCRShowServerWithLiveRoutes() error = %v", err)
	}
	defer server.Shutdown()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/__sophia_events?mode=dashboard", nil)
	if err != nil {
		t.Fatalf("NewRequestWithContext() error = %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /__sophia_events error = %v", err)
	}
	defer resp.Body.Close()

	if got := resp.Header.Get("Content-Type"); !strings.Contains(got, "text/event-stream") {
		t.Fatalf("expected text/event-stream, got %q", got)
	}

	eventName, eventData, readErr := readSSEEvent(bufio.NewReader(resp.Body))
	if readErr != nil {
		t.Fatalf("readSSEEvent() error = %v", readErr)
	}
	if eventName != "snapshot" {
		t.Fatalf("expected snapshot event, got %q", eventName)
	}
	if !strings.Contains(eventData, "\"mode\":\"dashboard\"") {
		t.Fatalf("expected dashboard payload, got %q", eventData)
	}
}

func TestCRShowServerSSEUsesCRRouteSnapshot(t *testing.T) {
	t.Parallel()
	server, err := startCRShowServerWithLiveRoutes(
		func() (string, error) { return "<!doctype html><html><body>dashboard</body></html>", nil },
		func(id int) (string, error) {
			return fmt.Sprintf("<!doctype html><html><body>cr-%d</body></html>", id), nil
		},
		func() (map[string]any, error) {
			return map[string]any{"mode": "dashboard", "generated_at": "2026-03-03T00:00:00Z"}, nil
		},
		func(id int) (map[string]any, error) {
			return map[string]any{"mode": "cr", "cr_id": id, "generated_at": "2026-03-03T00:00:00Z"}, nil
		},
	)
	if err != nil {
		t.Fatalf("startCRShowServerWithLiveRoutes() error = %v", err)
	}
	defer server.Shutdown()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/__sophia_events?mode=cr&id=42", nil)
	if err != nil {
		t.Fatalf("NewRequestWithContext() error = %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /__sophia_events?mode=cr&id=42 error = %v", err)
	}
	defer resp.Body.Close()

	eventName, eventData, readErr := readSSEEvent(bufio.NewReader(resp.Body))
	if readErr != nil {
		t.Fatalf("readSSEEvent() error = %v", readErr)
	}
	if eventName != "snapshot" {
		t.Fatalf("expected snapshot event, got %q", eventName)
	}
	if !strings.Contains(eventData, "\"cr_id\":42") {
		t.Fatalf("expected cr_id 42 payload, got %q", eventData)
	}
}

func TestCRShowServerSSENoChangeNoDuplicate(t *testing.T) {
	t.Parallel()
	server, err := startCRShowServerWithLiveRoutes(
		func() (string, error) { return "<!doctype html><html><body>dashboard</body></html>", nil },
		nil,
		func() (map[string]any, error) {
			return map[string]any{"stable": true, "generated_at": "2026-03-03T00:00:00Z"}, nil
		},
		nil,
	)
	if err != nil {
		t.Fatalf("startCRShowServerWithLiveRoutes() error = %v", err)
	}
	defer server.Shutdown()

	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/__sophia_events?mode=dashboard", nil)
	if err != nil {
		t.Fatalf("NewRequestWithContext() error = %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /__sophia_events error = %v", err)
	}
	defer resp.Body.Close()

	reader := bufio.NewReader(resp.Body)
	if _, _, readErr := readSSEEvent(reader); readErr != nil {
		t.Fatalf("readSSEEvent(first) error = %v", readErr)
	}
	if _, _, readErr := readSSEEvent(reader); readErr == nil {
		t.Fatalf("expected no duplicate snapshot before poll interval")
	}
}

func TestCRShowServerSSEEmitsOnChange(t *testing.T) {
	t.Parallel()
	var calls int32
	server, err := startCRShowServerWithLiveRoutes(
		func() (string, error) { return "<!doctype html><html><body>dashboard</body></html>", nil },
		nil,
		func() (map[string]any, error) {
			n := atomic.AddInt32(&calls, 1)
			version := "v1"
			if n >= 2 {
				version = "v2"
			}
			return map[string]any{
				"version":      version,
				"generated_at": fmt.Sprintf("2026-03-03T00:00:%02dZ", n),
			}, nil
		},
		nil,
	)
	if err != nil {
		t.Fatalf("startCRShowServerWithLiveRoutes() error = %v", err)
	}
	defer server.Shutdown()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/__sophia_events?mode=dashboard", nil)
	if err != nil {
		t.Fatalf("NewRequestWithContext() error = %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /__sophia_events error = %v", err)
	}
	defer resp.Body.Close()

	reader := bufio.NewReader(resp.Body)
	_, firstData, firstErr := readSSEEvent(reader)
	if firstErr != nil {
		t.Fatalf("readSSEEvent(first) error = %v", firstErr)
	}
	if !strings.Contains(firstData, "\"version\":\"v1\"") {
		t.Fatalf("expected initial v1 payload, got %q", firstData)
	}

	_, secondData, secondErr := readSSEEvent(reader)
	if secondErr != nil {
		t.Fatalf("readSSEEvent(second) error = %v", secondErr)
	}
	if !strings.Contains(secondData, "\"version\":\"v2\"") {
		t.Fatalf("expected changed v2 payload, got %q", secondData)
	}
}

func TestCRShowServerSSEIgnoresGeneratedAt(t *testing.T) {
	t.Parallel()
	var calls int32
	server, err := startCRShowServerWithLiveRoutes(
		func() (string, error) { return "<!doctype html><html><body>dashboard</body></html>", nil },
		nil,
		func() (map[string]any, error) {
			n := atomic.AddInt32(&calls, 1)
			return map[string]any{
				"stable_key":   "same-value",
				"generated_at": fmt.Sprintf("2026-03-03T00:00:%02dZ", n),
			}, nil
		},
		nil,
	)
	if err != nil {
		t.Fatalf("startCRShowServerWithLiveRoutes() error = %v", err)
	}
	defer server.Shutdown()

	ctx, cancel := context.WithTimeout(context.Background(), 2500*time.Millisecond)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/__sophia_events?mode=dashboard", nil)
	if err != nil {
		t.Fatalf("NewRequestWithContext() error = %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /__sophia_events error = %v", err)
	}
	defer resp.Body.Close()

	reader := bufio.NewReader(resp.Body)
	if _, _, readErr := readSSEEvent(reader); readErr != nil {
		t.Fatalf("readSSEEvent(first) error = %v", readErr)
	}
	if _, _, readErr := readSSEEvent(reader); readErr == nil {
		t.Fatalf("expected no second snapshot when only generated_at changes")
	}
}

func readSSEEvent(reader *bufio.Reader) (string, string, error) {
	if reader == nil {
		return "", "", fmt.Errorf("reader is required")
	}
	eventName := ""
	dataLines := make([]string, 0)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return "", "", err
		}
		trimmed := strings.TrimRight(line, "\r\n")
		if trimmed == "" {
			if eventName == "" && len(dataLines) == 0 {
				continue
			}
			return eventName, strings.Join(dataLines, "\n"), nil
		}
		if strings.HasPrefix(trimmed, ":") {
			continue
		}
		if strings.HasPrefix(trimmed, "event:") {
			eventName = strings.TrimSpace(strings.TrimPrefix(trimmed, "event:"))
			continue
		}
		if strings.HasPrefix(trimmed, "data:") {
			dataLines = append(dataLines, strings.TrimSpace(strings.TrimPrefix(trimmed, "data:")))
		}
	}
}
