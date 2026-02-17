package cli

import (
	"os"
	"path/filepath"
	"testing"

	"sophia/internal/model"
	"sophia/internal/service"
	"sophia/internal/store"
)

func TestCRStatusFromSubdirectoryUsesSharedMetadataFallback(t *testing.T) {
	dir := t.TempDir()
	runGit(t, dir, "init", "-b", "main")
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")
	runGit(t, dir, "commit", "--allow-empty", "-m", "seed")

	shared := localMetadataDirForCLI(t, dir)
	sharedStore := store.NewWithSophiaRoot(dir, shared)
	if err := sharedStore.Init("main", model.MetadataModeLocal); err != nil {
		t.Fatalf("sharedStore.Init() error = %v", err)
	}

	svc := service.New(dir)
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
	commonDir := runGit(t, dir, "rev-parse", "--git-common-dir")
	if !filepath.IsAbs(commonDir) {
		commonDir = filepath.Join(dir, commonDir)
	}
	return filepath.Join(commonDir, "sophia-local")
}
