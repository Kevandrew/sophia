package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPackCRAggregatesAndAppliesLimits(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")

	cr, err := svc.AddCR("Pack aggregate", "aggregate one-call context")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	if err := svc.AddNote(cr.ID, "first note"); err != nil {
		t.Fatalf("AddNote(first) error = %v", err)
	}
	if err := svc.AddNote(cr.ID, "second note"); err != nil {
		t.Fatalf("AddNote(second) error = %v", err)
	}

	task1, err := svc.AddTask(cr.ID, "checkpoint one")
	if err != nil {
		t.Fatalf("AddTask(task1) error = %v", err)
	}
	mustSetTaskContractForDiff(t, svc, cr.ID, task1.ID, []string{"pack1.txt"})
	if err := os.WriteFile(filepath.Join(dir, "pack1.txt"), []byte("one\n"), 0o644); err != nil {
		t.Fatalf("write pack1.txt: %v", err)
	}
	if _, err := svc.DoneTaskWithCheckpoint(cr.ID, task1.ID, DoneTaskOptions{Checkpoint: true, FromContract: true}); err != nil {
		t.Fatalf("DoneTaskWithCheckpoint(task1) error = %v", err)
	}

	task2, err := svc.AddTask(cr.ID, "checkpoint two")
	if err != nil {
		t.Fatalf("AddTask(task2) error = %v", err)
	}
	mustSetTaskContractForDiff(t, svc, cr.ID, task2.ID, []string{"pack2.txt"})
	if err := os.WriteFile(filepath.Join(dir, "pack2.txt"), []byte("two\n"), 0o644); err != nil {
		t.Fatalf("write pack2.txt: %v", err)
	}
	if _, err := svc.DoneTaskWithCheckpoint(cr.ID, task2.ID, DoneTaskOptions{Checkpoint: true, FromContract: true}); err != nil {
		t.Fatalf("DoneTaskWithCheckpoint(task2) error = %v", err)
	}

	view, err := svc.PackCR(cr.ID, PackOptions{EventsLimit: 2, CheckpointsLimit: 1})
	if err != nil {
		t.Fatalf("PackCR() error = %v", err)
	}
	if view == nil || view.CR == nil {
		t.Fatalf("expected pack payload")
	}
	if view.EventsMeta.Returned != 2 {
		t.Fatalf("expected 2 recent events, got %#v", view.EventsMeta)
	}
	if view.EventsMeta.Total <= view.EventsMeta.Returned || view.EventsMeta.Truncated == 0 {
		t.Fatalf("expected event truncation metadata, got %#v", view.EventsMeta)
	}
	if len(view.RecentCheckpoints) != 1 {
		t.Fatalf("expected 1 recent checkpoint, got %#v", view.RecentCheckpoints)
	}
	if view.CheckpointsMeta.Total != 2 || view.CheckpointsMeta.Truncated != 1 {
		t.Fatalf("expected checkpoint truncation metadata, got %#v", view.CheckpointsMeta)
	}
	if view.RecentCheckpoints[0].TaskID != task2.ID {
		t.Fatalf("expected latest checkpoint task %d, got %#v", task2.ID, view.RecentCheckpoints[0])
	}
	if view.Anchors == nil || strings.TrimSpace(view.Anchors.Base) == "" || strings.TrimSpace(view.Anchors.Head) == "" {
		t.Fatalf("expected non-empty anchors, got %#v", view.Anchors)
	}
	if strings.TrimSpace(view.DiffStat) == "" {
		t.Fatalf("expected diff stat in pack view")
	}
}

func TestPackCRMergedFallbackUsesCanonicalRef(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")

	cr, err := svc.AddCR("Pack merged", "pack merged fallback")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	setValidContract(t, svc, cr.ID)
	task, err := svc.AddTask(cr.ID, "pack merged checkpoint")
	if err != nil {
		t.Fatalf("AddTask() error = %v", err)
	}
	mustSetTaskContractForDiff(t, svc, cr.ID, task.ID, []string{"pack-merged.txt"})
	if err := os.WriteFile(filepath.Join(dir, "pack-merged.txt"), []byte("merged\n"), 0o644); err != nil {
		t.Fatalf("write pack-merged.txt: %v", err)
	}
	if _, err := svc.DoneTaskWithCheckpoint(cr.ID, task.ID, DoneTaskOptions{Checkpoint: true, FromContract: true}); err != nil {
		t.Fatalf("DoneTaskWithCheckpoint() error = %v", err)
	}
	mergedSHA, err := svc.MergeCR(cr.ID, false, "")
	if err != nil {
		t.Fatalf("MergeCR() error = %v", err)
	}
	if svc.git.BranchExists(cr.Branch) {
		t.Fatalf("expected merged CR branch deleted")
	}

	view, err := svc.PackCR(cr.ID, PackOptions{})
	if err != nil {
		t.Fatalf("PackCR(merged) error = %v", err)
	}
	if view.Anchors == nil {
		t.Fatalf("expected anchors in pack view")
	}
	if !strings.HasPrefix(view.Anchors.Head, mergedSHA) && !strings.HasPrefix(mergedSHA, view.Anchors.Head) {
		t.Fatalf("expected pack head %s to match merged commit %s", view.Anchors.Head, mergedSHA)
	}
	if !containsString(view.Warnings, "CR branch is unavailable; using canonical CR ref as head anchor") {
		t.Fatalf("expected canonical-ref fallback warning, got %#v", view.Warnings)
	}
}
