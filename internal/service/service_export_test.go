package service

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExportCRBundleDeterministicJSON(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")

	cr, err := svc.AddCR("Export bundle", "deterministic export")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	setValidContract(t, svc, cr.ID)

	task, err := svc.AddTask(cr.ID, "feat: export fixture")
	if err != nil {
		t.Fatalf("AddTask() error = %v", err)
	}
	intent := "Create checkpoint commit for export references."
	acceptance := []string{"checkpoint exists"}
	scope := []string{"export.txt"}
	if _, err := svc.SetTaskContract(cr.ID, task.ID, TaskContractPatch{
		Intent:             &intent,
		AcceptanceCriteria: &acceptance,
		Scope:              &scope,
	}); err != nil {
		t.Fatalf("SetTaskContract() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "export.txt"), []byte("export\n"), 0o644); err != nil {
		t.Fatalf("write export.txt: %v", err)
	}
	if _, err := svc.DoneTaskWithCheckpoint(cr.ID, task.ID, DoneTaskOptions{Checkpoint: true, FromContract: true}); err != nil {
		t.Fatalf("DoneTaskWithCheckpoint() error = %v", err)
	}

	bundle1, payload1, err := svc.ExportCRBundle(cr.ID, ExportCROptions{Format: "json"})
	if err != nil {
		t.Fatalf("ExportCRBundle(first) error = %v", err)
	}
	bundle2, payload2, err := svc.ExportCRBundle(cr.ID, ExportCROptions{Format: "json"})
	if err != nil {
		t.Fatalf("ExportCRBundle(second) error = %v", err)
	}
	if !bytes.Equal(payload1, payload2) {
		t.Fatalf("expected deterministic payload bytes\nfirst=%s\nsecond=%s", string(payload1), string(payload2))
	}
	if bundle1.SchemaVersion != exportSchemaV1 {
		t.Fatalf("expected schema %q, got %#v", exportSchemaV1, bundle1)
	}
	if len(bundle1.Checkpoints) != 1 || strings.TrimSpace(bundle1.Checkpoints[0].Commit) == "" {
		t.Fatalf("expected checkpoint metadata in export, got %#v", bundle1.Checkpoints)
	}
	if len(bundle1.ReferencedCommits) == 0 {
		t.Fatalf("expected referenced commits in export, got %#v", bundle1)
	}
	if bundle2.SchemaVersion != bundle1.SchemaVersion {
		t.Fatalf("expected matching schemas, got %q vs %q", bundle1.SchemaVersion, bundle2.SchemaVersion)
	}
}

func TestExportCRBundleIncludesTaskDiffs(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")

	cr, err := svc.AddCR("Export diffs", "include task patches")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	setValidContract(t, svc, cr.ID)

	task, err := svc.AddTask(cr.ID, "feat: diff export fixture")
	if err != nil {
		t.Fatalf("AddTask() error = %v", err)
	}
	intent := "Create task checkpoint patch."
	acceptance := []string{"patch renderable"}
	scope := []string{"diff.txt"}
	if _, err := svc.SetTaskContract(cr.ID, task.ID, TaskContractPatch{
		Intent:             &intent,
		AcceptanceCriteria: &acceptance,
		Scope:              &scope,
	}); err != nil {
		t.Fatalf("SetTaskContract() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "diff.txt"), []byte("diff\n"), 0o644); err != nil {
		t.Fatalf("write diff.txt: %v", err)
	}
	if _, err := svc.DoneTaskWithCheckpoint(cr.ID, task.ID, DoneTaskOptions{Checkpoint: true, FromContract: true}); err != nil {
		t.Fatalf("DoneTaskWithCheckpoint() error = %v", err)
	}

	bundle, _, err := svc.ExportCRBundle(cr.ID, ExportCROptions{Format: "json", Include: []string{"diffs"}})
	if err != nil {
		t.Fatalf("ExportCRBundle(include diffs) error = %v", err)
	}
	if len(bundle.TaskDiffs) != 1 {
		t.Fatalf("expected one task diff, got %#v", bundle.TaskDiffs)
	}
	if !containsString(bundle.TaskDiffs[0].Files, "diff.txt") {
		t.Fatalf("expected diff.txt in task diff files, got %#v", bundle.TaskDiffs[0])
	}
	if !strings.Contains(bundle.TaskDiffs[0].Patch, "diff --git") {
		t.Fatalf("expected patch body in task diff, got %#v", bundle.TaskDiffs[0])
	}
}

func TestExportCRBundleRejectsInvalidInclude(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")
	cr, err := svc.AddCR("Export invalid", "invalid include")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	setValidContract(t, svc, cr.ID)

	if _, _, err := svc.ExportCRBundle(cr.ID, ExportCROptions{Format: "json", Include: []string{"unknown"}}); err == nil {
		t.Fatalf("expected invalid include error")
	}
}
