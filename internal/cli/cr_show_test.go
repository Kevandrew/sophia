package cli

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

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
