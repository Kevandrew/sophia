package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sophia/internal/service"
)

func TestCommandsResolveRepoFromSubdirectory(t *testing.T) {
	dir := t.TempDir()
	svc := service.New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if _, err := svc.AddCR("Subdir CR", "repo root resolution"); err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}

	nested := filepath.Join(dir, "nested", "child")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatalf("mkdir nested: %v", err)
	}

	out, _, err := runCLI(t, nested, "cr", "list")
	if err != nil {
		t.Fatalf("cr list from nested dir error = %v\noutput=%s", err, out)
	}
	if !strings.Contains(out, "Subdir CR") {
		t.Fatalf("expected CR listing from nested dir, got %q", out)
	}
}
