package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sophia/internal/model"
)

func TestRepairFromGitPromotesReachableChildCRMergedThroughParentHistory(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")

	parent, err := svc.AddCR("Parent merge", "repair should recover descendants")
	if err != nil {
		t.Fatalf("AddCR(parent) error = %v", err)
	}
	child, _, err := svc.AddCRWithOptionsWithWarnings("Child merge", "reachable through parent merge", AddCROptions{ParentCRID: parent.ID})
	if err != nil {
		t.Fatalf("AddCR(child) error = %v", err)
	}

	if err := os.WriteFile(filepath.Join(dir, "child.txt"), []byte("child\n"), 0o644); err != nil {
		t.Fatalf("write child.txt: %v", err)
	}
	runGit(t, dir, "add", "child.txt")
	runGit(t, dir, "commit", "-m", "feat: child implementation")
	childHead := runGit(t, dir, "rev-parse", "--verify", "HEAD")

	loadedChild, err := svc.store.LoadCR(child.ID)
	if err != nil {
		t.Fatalf("LoadCR(child) error = %v", err)
	}
	loadedChild.Subtasks = []model.Subtask{{
		ID:               1,
		Title:            "Child implementation",
		Status:           model.TaskStatusDone,
		CreatedAt:        harnessTimestamp,
		UpdatedAt:        harnessTimestamp,
		CreatedBy:        "Test User <test@example.com>",
		CompletedAt:      harnessTimestamp,
		CompletedBy:      "Test User <test@example.com>",
		CheckpointCommit: childHead,
		CheckpointSource: "task_checkpoint",
	}}
	if err := svc.store.SaveCR(loadedChild); err != nil {
		t.Fatalf("SaveCR(child) error = %v", err)
	}

	runGit(t, dir, "checkout", parent.Branch)
	runGit(t, dir, "merge", "--ff-only", child.Branch)
	runGit(t, dir, "checkout", "main")
	runGit(t, dir, "merge", "--no-ff", parent.Branch,
		"-m", "[CR-1] Parent merge",
		"-m", strings.Join([]string{
			"Intent:",
			"Parent merge",
			"",
			"Subtasks:",
			"- [x] #1 Parent merge",
			"",
			"Metadata:",
			"- actor: Test User <test@example.com>",
			"- merged_at: 2026-03-07T00:00:00Z",
			"",
			"Sophia-CR: 1",
			"Sophia-Intent: Parent merge",
			"Sophia-Tasks: 1 completed",
			"Sophia-Branch: " + parent.Branch,
			"Sophia-Base-Ref: main",
		}, "\n"),
	)

	reloadedChild, err := svc.store.LoadCR(child.ID)
	if err != nil {
		t.Fatalf("LoadCR(child pre-repair) error = %v", err)
	}
	if reloadedChild.Status != model.StatusInProgress {
		t.Fatalf("expected child to still be in progress before repair, got %#v", reloadedChild)
	}

	report, err := svc.RepairFromGit("main", true)
	if err != nil {
		t.Fatalf("RepairFromGit(refresh) error = %v", err)
	}
	if !containsInt(report.RepairedCRIDs, child.ID) {
		t.Fatalf("expected child id in repaired set, got %#v", report.RepairedCRIDs)
	}

	repairedChild, err := svc.store.LoadCR(child.ID)
	if err != nil {
		t.Fatalf("LoadCR(child repaired) error = %v", err)
	}
	if repairedChild.Status != model.StatusMerged {
		t.Fatalf("expected child promoted to merged, got %#v", repairedChild)
	}
	if strings.TrimSpace(repairedChild.MergedCommit) == "" {
		t.Fatalf("expected merged commit populated after promotion, got %#v", repairedChild)
	}
	if len(repairedChild.Events) == 0 || repairedChild.Events[len(repairedChild.Events)-1].Meta["repair_merge_promotion"] != "reachable_from_base" {
		t.Fatalf("expected repair merge promotion event, got %#v", repairedChild.Events)
	}
}
