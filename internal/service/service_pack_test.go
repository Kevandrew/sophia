package service

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestPackCRAggregatesAndAppliesLimits(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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

func TestPackCRInProgressMissingBranchUsesMetadataOnlyFallback(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")

	result, err := svc.AddCRWithOptions("Pack metadata-only", "allow show/pack fallback when branch is missing", AddCROptions{NoSwitch: true})
	if err != nil {
		t.Fatalf("AddCRWithOptions() error = %v", err)
	}
	if result == nil || result.CR == nil {
		t.Fatalf("expected CR payload")
	}

	cr, err := svc.store.LoadCR(result.CR.ID)
	if err != nil {
		t.Fatalf("LoadCR() error = %v", err)
	}
	cr.BaseRef = "refs/heads/missing-parent-ref"
	cr.BaseCommit = ""
	cr.UpdatedAt = svc.timestamp()
	if err := svc.store.SaveCR(cr); err != nil {
		t.Fatalf("SaveCR() error = %v", err)
	}

	runGit(t, dir, "branch", "-D", result.CR.Branch)

	view, err := svc.PackCR(result.CR.ID, PackOptions{})
	if err != nil {
		t.Fatalf("PackCR() error = %v", err)
	}
	if view == nil || view.Anchors == nil {
		t.Fatalf("expected pack anchors in metadata-only fallback")
	}
	if strings.TrimSpace(view.Anchors.Base) == "" || strings.TrimSpace(view.Anchors.Head) == "" {
		t.Fatalf("expected non-empty anchors, got %#v", view.Anchors)
	}
	if view.Anchors.Base != view.Anchors.Head {
		t.Fatalf("expected metadata-only fallback head==base, got base=%q head=%q", view.Anchors.Base, view.Anchors.Head)
	}
	foundMetadataOnly := false
	for _, warning := range view.Warnings {
		if strings.Contains(strings.ToLower(warning), "metadata-only") {
			foundMetadataOnly = true
			break
		}
	}
	if !foundMetadataOnly {
		t.Fatalf("expected metadata-only fallback warning, got %#v", view.Warnings)
	}
}

func TestPackCRAbandonedMissingBranchUsesMetadataOnlyFallback(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")

	cr, err := svc.AddCR("Pack abandoned metadata-only", "allow show/pack fallback when abandoned branch is missing")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	if _, err := svc.AbandonCR(cr.ID, CRAbandonOptions{Reason: "testing"}); err != nil {
		t.Fatalf("AbandonCR() error = %v", err)
	}
	if svc.git.BranchExists(cr.Branch) {
		t.Fatalf("expected abandoned CR branch %q to be removed", cr.Branch)
	}

	view, err := svc.PackCR(cr.ID, PackOptions{})
	if err != nil {
		t.Fatalf("PackCR() error = %v", err)
	}
	if view == nil || view.Anchors == nil {
		t.Fatalf("expected pack anchors in metadata-only fallback")
	}
	if view.Anchors.Base != view.Anchors.Head {
		t.Fatalf("expected metadata-only fallback head==base, got base=%q head=%q", view.Anchors.Base, view.Anchors.Head)
	}
	if !containsStringCaseInsensitive(view.Warnings, "metadata-only") {
		t.Fatalf("expected metadata-only warning, got %#v", view.Warnings)
	}
}

func TestPackCRMetadataFallbackWarnsWhenCheckpointCommitsExist(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")

	cr, err := svc.AddCR("Pack checkpoint warning", "warn when fallback may hide orphaned commits")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	setValidContract(t, svc, cr.ID)
	task, err := svc.AddTask(cr.ID, "checkpoint task")
	if err != nil {
		t.Fatalf("AddTask() error = %v", err)
	}
	mustSetTaskContractForDiff(t, svc, cr.ID, task.ID, []string{"pack-checkpoint-warning.txt"})
	if err := os.WriteFile(filepath.Join(dir, "pack-checkpoint-warning.txt"), []byte("checkpoint\n"), 0o644); err != nil {
		t.Fatalf("write pack-checkpoint-warning.txt: %v", err)
	}
	if _, err := svc.DoneTaskWithCheckpoint(cr.ID, task.ID, DoneTaskOptions{Checkpoint: true, FromContract: true}); err != nil {
		t.Fatalf("DoneTaskWithCheckpoint() error = %v", err)
	}

	runGit(t, dir, "checkout", "main")
	runGit(t, dir, "branch", "-D", cr.Branch)

	view, err := svc.PackCR(cr.ID, PackOptions{})
	if err != nil {
		t.Fatalf("PackCR() error = %v", err)
	}
	if !containsStringCaseInsensitive(view.Warnings, "orphaned implementation commits") {
		t.Fatalf("expected orphaned implementation warning, got %#v", view.Warnings)
	}
}

func TestPackCRIncludesStackTreeAndLineage(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")

	parent, err := svc.AddCR("Stack parent", "aggregate parent pack")
	if err != nil {
		t.Fatalf("AddCR(parent) error = %v", err)
	}
	setValidContract(t, svc, parent.ID)
	parentTask, err := svc.AddTask(parent.ID, "delegate child")
	if err != nil {
		t.Fatalf("AddTask(parent) error = %v", err)
	}
	setValidTaskContract(t, svc, parent.ID, parentTask.ID)

	child, _, err := svc.AddCRWithOptionsWithWarnings("Stack child", "first child", AddCROptions{ParentCRID: parent.ID})
	if err != nil {
		t.Fatalf("AddCR(child) error = %v", err)
	}
	setValidContract(t, svc, child.ID)
	if _, err := svc.DelegateTaskToChild(parent.ID, parentTask.ID, child.ID); err != nil {
		t.Fatalf("DelegateTaskToChild() error = %v", err)
	}

	grandchild, _, err := svc.AddCRWithOptionsWithWarnings("Stack grandchild", "nested child", AddCROptions{ParentCRID: child.ID})
	if err != nil {
		t.Fatalf("AddCR(grandchild) error = %v", err)
	}
	setValidContract(t, svc, grandchild.ID)

	parentView, err := svc.PackCR(parent.ID, PackOptions{})
	if err != nil {
		t.Fatalf("PackCR(parent) error = %v", err)
	}
	if parentView.StackTree == nil {
		t.Fatalf("expected parent stack tree")
	}
	if parentView.StackTree.ID != parent.ID {
		t.Fatalf("expected root tree id %d, got %#v", parent.ID, parentView.StackTree)
	}
	if len(parentView.StackTree.Children) != 1 {
		t.Fatalf("expected one child node, got %#v", parentView.StackTree.Children)
	}
	childNode := parentView.StackTree.Children[0]
	if childNode.ID != child.ID {
		t.Fatalf("expected child node %d, got %#v", child.ID, childNode)
	}
	if childNode.ResolutionState != "pending" {
		t.Fatalf("expected child resolution pending, got %#v", childNode.ResolutionState)
	}
	if len(childNode.Children) != 1 || childNode.Children[0].ID != grandchild.ID {
		t.Fatalf("expected grandchild nested under child, got %#v", childNode.Children)
	}

	childView, err := svc.PackCR(child.ID, PackOptions{})
	if err != nil {
		t.Fatalf("PackCR(child) error = %v", err)
	}
	if len(childView.StackLineage) != 1 {
		t.Fatalf("expected single parent lineage entry, got %#v", childView.StackLineage)
	}
	if childView.StackLineage[0].ID != parent.ID {
		t.Fatalf("expected lineage parent %d, got %#v", parent.ID, childView.StackLineage)
	}
	if childView.StackTree == nil || childView.StackTree.ID != child.ID {
		t.Fatalf("expected child-local stack tree, got %#v", childView.StackTree)
	}
	if len(childView.StackTree.Children) != 1 || childView.StackTree.Children[0].ID != grandchild.ID {
		t.Fatalf("expected child tree to include grandchild, got %#v", childView.StackTree.Children)
	}
	if childView.StackLineage[0].Role != "aggregate_parent" {
		t.Fatalf("expected lineage role aggregate_parent, got %#v", childView.StackLineage[0])
	}
	if got := strconv.Itoa(childView.StackTree.Children[0].ParentCRID); got != strconv.Itoa(child.ID) {
		t.Fatalf("expected grandchild parent id %d, got %#v", child.ID, childView.StackTree.Children[0].ParentCRID)
	}
}
