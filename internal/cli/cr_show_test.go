package cli

import (
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

func TestCRShowServerSupportsCloseEndpoint(t *testing.T) {
	t.Parallel()
	server, err := startCRShowServer("<!doctype html><html><body>ok</body></html>")
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
