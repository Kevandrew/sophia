package cli

import (
	"testing"

	"sophia/internal/service"
)

func TestCRList_JSONOutput(t *testing.T) {
	dir := t.TempDir()
	svc := service.New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if _, err := svc.AddCR("Test CR for list", "test description"); err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}

	stdout, _, err := runCLI(t, dir, "cr", "list", "--json")
	if err != nil {
		t.Fatalf("cr list --json error = %v", err)
	}

	env := decodeEnvelope(t, stdout)
	if !env.OK {
		t.Fatalf("expected ok response")
	}
	if _, ok := env.Data["count"]; !ok {
		t.Fatalf("expected count field")
	}
	if _, ok := env.Data["results"]; !ok {
		t.Fatalf("expected results field")
	}
}

func TestCRList_FilterByStatus(t *testing.T) {
	dir := t.TempDir()
	svc := service.New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if _, err := svc.AddCR("Test CR", "test description"); err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}

	stdout, _, err := runCLI(t, dir, "cr", "list", "--status", "in_progress", "--json")
	if err != nil {
		t.Fatalf("cr list --status error = %v", err)
	}

	env := decodeEnvelope(t, stdout)
	if !env.OK {
		t.Fatalf("expected ok response")
	}

	results, ok := env.Data["results"].([]any)
	if !ok {
		t.Fatalf("expected results array")
	}

	for _, r := range results {
		cr, ok := r.(map[string]any)
		if !ok {
			t.Fatalf("expected result object")
		}
		if cr["status"] != "in_progress" {
			t.Fatalf("expected status in_progress, got %v", cr["status"])
		}
	}
}

func TestCRSearch_Positive(t *testing.T) {
	dir := t.TempDir()
	svc := service.New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if _, err := svc.AddCR("Searchable Test CR", "unique description for search"); err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}

	stdout, _, err := runCLI(t, dir, "cr", "search", "unique", "--json")
	if err != nil {
		t.Fatalf("cr search error = %v", err)
	}

	env := decodeEnvelope(t, stdout)
	if !env.OK {
		t.Fatalf("expected ok response")
	}

	count, ok := env.Data["count"].(float64)
	if !ok || count < 1 {
		t.Fatalf("expected at least 1 result, got %v", count)
	}
}

func TestCRSearch_NoResults(t *testing.T) {
	dir := t.TempDir()
	svc := service.New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if _, err := svc.AddCR("Test CR", "test description"); err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}

	stdout, _, err := runCLI(t, dir, "cr", "search", "xyznonexistent123", "--json")
	if err != nil {
		t.Fatalf("cr search error = %v", err)
	}

	env := decodeEnvelope(t, stdout)
	if !env.OK {
		t.Fatalf("expected ok response")
	}

	count, ok := env.Data["count"].(float64)
	if !ok || count != 0 {
		t.Fatalf("expected 0 results, got %v", count)
	}
}

func TestCRSearch_FilterCombination(t *testing.T) {
	dir := t.TempDir()
	svc := service.New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if _, err := svc.AddCR("Combined Filter CR", "test"); err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}

	stdout, _, err := runCLI(t, dir, "cr", "search", "--status", "in_progress", "--text", "Combined", "--json")
	if err != nil {
		t.Fatalf("cr search error = %v", err)
	}

	env := decodeEnvelope(t, stdout)
	if !env.OK {
		t.Fatalf("expected ok response")
	}

	results, ok := env.Data["results"].([]any)
	if !ok {
		t.Fatalf("expected results array")
	}

	for _, r := range results {
		cr, ok := r.(map[string]any)
		if !ok {
			t.Fatalf("expected result object")
		}
		if cr["status"] != "in_progress" {
			t.Fatalf("expected status in_progress, got %v", cr["status"])
		}
	}
}
