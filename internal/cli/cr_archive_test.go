package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sophia/internal/service"
)

func TestCRArchiveWriteAndAppendJSON(t *testing.T) {
	dir := setupCLIMergedCRNoArchiveRepo(t)

	writeOut, _, writeErr := runCLI(t, dir, "cr", "archive", "write", "1", "--json")
	if writeErr != nil {
		t.Fatalf("cr archive write --json error = %v\noutput=%s", writeErr, writeOut)
	}
	writeEnv := decodeEnvelope(t, writeOut)
	if !writeEnv.OK {
		t.Fatalf("expected write envelope ok, got %#v", writeEnv)
	}
	if revision, ok := writeEnv.Data["revision"].(float64); !ok || int(revision) != 1 {
		t.Fatalf("expected revision=1, got %#v", writeEnv.Data["revision"])
	}
	if path, ok := writeEnv.Data["path"].(string); !ok || !strings.Contains(path, ".sophia-tracked/cr/cr-1.v1.yaml") {
		t.Fatalf("expected archive path, got %#v", writeEnv.Data["path"])
	}
	if _, ok := writeEnv.Data["config"].(map[string]any); !ok {
		t.Fatalf("expected archive config in payload, got %#v", writeEnv.Data["config"])
	}

	appendOut, _, appendErr := runCLI(t, dir, "cr", "archive", "append", "1", "--reason", "fix metadata", "--json")
	if appendErr != nil {
		t.Fatalf("cr archive append --json error = %v\noutput=%s", appendErr, appendOut)
	}
	appendEnv := decodeEnvelope(t, appendOut)
	if !appendEnv.OK {
		t.Fatalf("expected append envelope ok, got %#v", appendEnv)
	}
	if revision, ok := appendEnv.Data["revision"].(float64); !ok || int(revision) != 2 {
		t.Fatalf("expected revision=2, got %#v", appendEnv.Data["revision"])
	}
	if reason, ok := appendEnv.Data["reason"].(string); !ok || reason != "fix metadata" {
		t.Fatalf("expected append reason in payload, got %#v", appendEnv.Data["reason"])
	}
}

func TestCRArchiveBackfillDryRunAndCommitJSON(t *testing.T) {
	dir := setupCLIMergedCRNoArchiveRepo(t)
	svc := service.New(dir)

	cr2, err := svc.AddCR("Archive fixture 2", "cli archive test")
	if err != nil {
		t.Fatalf("AddCR(2) error = %v", err)
	}
	setCLIValidContract(t, svc, cr2.ID)
	if err := os.WriteFile(filepath.Join(dir, "second.txt"), []byte("two\n"), 0o644); err != nil {
		t.Fatalf("write second.txt: %v", err)
	}
	runGit(t, dir, "add", "second.txt")
	runGit(t, dir, "commit", "-m", "feat: archive fixture two")
	if _, _, err := svc.MergeCRWithWarnings(cr2.ID, false, ""); err != nil {
		t.Fatalf("MergeCRWithWarnings(2) error = %v", err)
	}

	dryOut, _, dryErr := runCLI(t, dir, "cr", "archive", "backfill", "--json")
	if dryErr != nil {
		t.Fatalf("cr archive backfill --json dry-run error = %v\noutput=%s", dryErr, dryOut)
	}
	dryEnv := decodeEnvelope(t, dryOut)
	if !dryEnv.OK {
		t.Fatalf("expected dry-run envelope ok, got %#v", dryEnv)
	}
	if dryRun, ok := dryEnv.Data["dry_run"].(bool); !ok || !dryRun {
		t.Fatalf("expected dry_run=true, got %#v", dryEnv.Data["dry_run"])
	}
	missing, ok := dryEnv.Data["missing_cr_ids"].([]any)
	if !ok || len(missing) != 2 {
		t.Fatalf("expected 2 missing ids, got %#v", dryEnv.Data["missing_cr_ids"])
	}

	commitOut, _, commitErr := runCLI(t, dir, "cr", "archive", "backfill", "--commit", "--json")
	if commitErr != nil {
		t.Fatalf("cr archive backfill --commit --json error = %v\noutput=%s", commitErr, commitOut)
	}
	commitEnv := decodeEnvelope(t, commitOut)
	if !commitEnv.OK {
		t.Fatalf("expected commit envelope ok, got %#v", commitEnv)
	}
	if committed, ok := commitEnv.Data["committed"].(bool); !ok || !committed {
		t.Fatalf("expected committed=true, got %#v", commitEnv.Data["committed"])
	}
}

func setupCLIMergedCRNoArchiveRepo(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	svc := service.New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")
	if err := os.WriteFile(filepath.Join(dir, "SOPHIA.yaml"), []byte("version: v1\narchive:\n  enabled: false\n"), 0o644); err != nil {
		t.Fatalf("write SOPHIA.yaml: %v", err)
	}
	runGit(t, dir, "add", "SOPHIA.yaml")
	runGit(t, dir, "commit", "-m", "chore: disable archive for fixture")

	cr, err := svc.AddCR("Archive fixture", "cli archive test")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	setCLIValidContract(t, svc, cr.ID)
	if err := os.WriteFile(filepath.Join(dir, "archive.txt"), []byte("one\n"), 0o644); err != nil {
		t.Fatalf("write archive.txt: %v", err)
	}
	runGit(t, dir, "add", "archive.txt")
	runGit(t, dir, "commit", "-m", "feat: archive fixture one")
	if _, _, err := svc.MergeCRWithWarnings(cr.ID, false, ""); err != nil {
		t.Fatalf("MergeCRWithWarnings() error = %v", err)
	}

	return dir
}
