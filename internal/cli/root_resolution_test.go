package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sophia/internal/model"
	"sophia/internal/service"
	"sophia/internal/store"
)

func TestCRStatusFromSubdirectoryUsesSharedMetadataFallback(t *testing.T) {
	dir := t.TempDir()
	svc := service.New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	shared := localMetadataDirForCLI(t, dir)
	sharedStore := store.NewWithSophiaRoot(dir, shared)
	if err := sharedStore.Init("main", model.MetadataModeLocal); err != nil {
		t.Fatalf("sharedStore.Init() error = %v", err)
	}

	cr, err := svc.AddCR("Shared metadata CR", "regression fixture")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}

	subdir := filepath.Join(dir, "nested", "folder")
	if err := os.MkdirAll(subdir, 0o755); err != nil {
		t.Fatalf("MkdirAll(subdir) error = %v", err)
	}

	out, _, runErr := runCLI(t, subdir, "cr", "status", "1", "--json")
	if runErr != nil {
		t.Fatalf("cr status --json from subdir error = %v\noutput=%s", runErr, out)
	}
	env := decodeEnvelope(t, out)
	if !env.OK {
		t.Fatalf("expected ok envelope, got %#v", env)
	}
	gotID, ok := env.Data["id"].(float64)
	if !ok || int(gotID) != cr.ID {
		t.Fatalf("expected id %d, got %#v", cr.ID, env.Data["id"])
	}
}

func localMetadataDirForCLI(t *testing.T, dir string) string {
	t.Helper()
	commonDir := filepath.Join(dir, ".git")
	info, err := os.Stat(commonDir)
	if err != nil {
		t.Fatalf("stat .git: %v", err)
	}
	if !info.IsDir() {
		content, readErr := os.ReadFile(commonDir)
		if readErr != nil {
			t.Fatalf("read .git file: %v", readErr)
		}
		line := strings.TrimSpace(string(content))
		if !strings.HasPrefix(line, "gitdir:") {
			t.Fatalf("unexpected .git file format: %q", line)
		}
		commonDir = strings.TrimSpace(strings.TrimPrefix(line, "gitdir:"))
		if !filepath.IsAbs(commonDir) {
			commonDir = filepath.Join(dir, commonDir)
		}
	}
	return filepath.Join(commonDir, "sophia-local")
}
