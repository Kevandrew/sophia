package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sophia/internal/model"
)

func TestAddChildCRFromCurrentUsesActiveCRAsParent(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	parent, err := svc.AddCR("Parent", "active context")
	if err != nil {
		t.Fatalf("AddCR(parent) error = %v", err)
	}
	child, _, err := svc.AddChildCRFromCurrent("Child", "created from current")
	if err != nil {
		t.Fatalf("AddChildCRFromCurrent() error = %v", err)
	}
	if child.ParentCRID != parent.ID {
		t.Fatalf("expected parent id %d, got %d", parent.ID, child.ParentCRID)
	}
	if child.BaseRef != parent.Branch {
		t.Fatalf("expected child base_ref %q, got %q", parent.Branch, child.BaseRef)
	}
}

func TestDelegateFlowAllowsChildMergeBeforeParentAndAutoCompletesParentTask(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")

	parent, err := svc.AddCR("Parent refactor", "umbrella intent")
	if err != nil {
		t.Fatalf("AddCR(parent) error = %v", err)
	}
	setValidContract(t, svc, parent.ID)
	parentTask, err := svc.AddTask(parent.ID, "Split service layer")
	if err != nil {
		t.Fatalf("AddTask(parent) error = %v", err)
	}
	setValidTaskContract(t, svc, parent.ID, parentTask.ID)

	if err := os.WriteFile(filepath.Join(dir, "parent.txt"), []byte("parent\n"), 0o644); err != nil {
		t.Fatalf("write parent file: %v", err)
	}
	runGit(t, dir, "add", "parent.txt")
	runGit(t, dir, "commit", "-m", "feat: parent intent commit")

	child, _, err := svc.AddCRWithOptionsWithWarnings("Child slice", "delegated implementation", AddCROptions{ParentCRID: parent.ID})
	if err != nil {
		t.Fatalf("AddCR(child) error = %v", err)
	}
	setValidContract(t, svc, child.ID)

	delegateResult, err := svc.DelegateTaskToChild(parent.ID, parentTask.ID, child.ID)
	if err != nil {
		t.Fatalf("DelegateTaskToChild() error = %v", err)
	}
	if delegateResult.ChildTaskID <= 0 {
		t.Fatalf("expected child task id > 0, got %#v", delegateResult)
	}

	if err := os.WriteFile(filepath.Join(dir, "child.txt"), []byte("child\n"), 0o644); err != nil {
		t.Fatalf("write child file: %v", err)
	}
	runGit(t, dir, "add", "child.txt")
	runGit(t, dir, "commit", "-m", "feat: child implementation")

	if _, err := svc.MergeCR(parent.ID, false, ""); err == nil || !strings.Contains(err.Error(), "delegated to unmerged child CR") {
		t.Fatalf("expected parent merge blocked by delegated child, got %v", err)
	}

	if _, err := svc.MergeCR(child.ID, false, ""); err != nil {
		t.Fatalf("MergeCR(child) should succeed before parent when delegated, got %v", err)
	}

	reloadedParent, err := svc.store.LoadCR(parent.ID)
	if err != nil {
		t.Fatalf("LoadCR(parent) error = %v", err)
	}
	taskIndex := indexOfTask(reloadedParent.Subtasks, parentTask.ID)
	if taskIndex < 0 {
		t.Fatalf("expected parent task %d to exist", parentTask.ID)
	}
	if reloadedParent.Subtasks[taskIndex].Status != model.TaskStatusDone {
		t.Fatalf("expected delegated parent task auto-completed, got %#v", reloadedParent.Subtasks[taskIndex])
	}
	if strings.TrimSpace(reloadedParent.Subtasks[taskIndex].CompletedAt) == "" {
		t.Fatalf("expected delegated parent task completed_at to be set, got %#v", reloadedParent.Subtasks[taskIndex])
	}

	if _, err := svc.MergeCR(parent.ID, false, ""); err != nil {
		t.Fatalf("MergeCR(parent) after child merge error = %v", err)
	}
}

func TestUndelegateTaskReturnsDelegatedTaskToOpenWhenLastLinkRemoved(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	parent, err := svc.AddCR("Parent", "for undelegate")
	if err != nil {
		t.Fatalf("AddCR(parent) error = %v", err)
	}
	task, err := svc.AddTask(parent.ID, "Task to delegate")
	if err != nil {
		t.Fatalf("AddTask(parent) error = %v", err)
	}
	setValidTaskContract(t, svc, parent.ID, task.ID)
	setValidContract(t, svc, parent.ID)

	child, _, err := svc.AddCRWithOptionsWithWarnings("Child", "for undelegate", AddCROptions{ParentCRID: parent.ID})
	if err != nil {
		t.Fatalf("AddCR(child) error = %v", err)
	}
	if _, err := svc.DelegateTaskToChild(parent.ID, task.ID, child.ID); err != nil {
		t.Fatalf("DelegateTaskToChild() error = %v", err)
	}
	if _, err := svc.UndelegateTaskFromChild(parent.ID, task.ID, child.ID); err != nil {
		t.Fatalf("UndelegateTaskFromChild() error = %v", err)
	}

	reloadedParent, err := svc.store.LoadCR(parent.ID)
	if err != nil {
		t.Fatalf("LoadCR(parent) error = %v", err)
	}
	taskIndex := indexOfTask(reloadedParent.Subtasks, task.ID)
	if taskIndex < 0 {
		t.Fatalf("expected parent task to exist")
	}
	parentTask := reloadedParent.Subtasks[taskIndex]
	if parentTask.Status != model.TaskStatusOpen {
		t.Fatalf("expected parent task status open after undelegate, got %#v", parentTask)
	}
	if len(parentTask.Delegations) != 0 {
		t.Fatalf("expected no delegations after undelegate, got %#v", parentTask.Delegations)
	}
}

func TestStackCRIncludesChildAndDelegationBlockers(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	parent, err := svc.AddCR("Parent", "stack root")
	if err != nil {
		t.Fatalf("AddCR(parent) error = %v", err)
	}
	setValidContract(t, svc, parent.ID)
	parentTask, err := svc.AddTask(parent.ID, "delegate me")
	if err != nil {
		t.Fatalf("AddTask(parent) error = %v", err)
	}
	setValidTaskContract(t, svc, parent.ID, parentTask.ID)

	child, _, err := svc.AddCRWithOptionsWithWarnings("Child", "stack child", AddCROptions{ParentCRID: parent.ID})
	if err != nil {
		t.Fatalf("AddCR(child) error = %v", err)
	}
	setValidContract(t, svc, child.ID)
	if _, err := svc.DelegateTaskToChild(parent.ID, parentTask.ID, child.ID); err != nil {
		t.Fatalf("DelegateTaskToChild() error = %v", err)
	}

	stack, err := svc.StackCR(parent.ID)
	if err != nil {
		t.Fatalf("StackCR() error = %v", err)
	}
	if stack.RootCRID != parent.ID {
		t.Fatalf("expected root %d, got %d", parent.ID, stack.RootCRID)
	}
	if len(stack.Nodes) != 2 {
		t.Fatalf("expected 2 nodes, got %#v", stack.Nodes)
	}
	if stack.Nodes[0].ID != parent.ID || stack.Nodes[0].Depth != 0 {
		t.Fatalf("unexpected parent node %#v", stack.Nodes[0])
	}
	if !stack.Nodes[0].MergeBlocked {
		t.Fatalf("expected parent node merge blocked while delegated child unmerged, got %#v", stack.Nodes[0])
	}
	if stack.Nodes[1].ID != child.ID || stack.Nodes[1].Depth != 1 {
		t.Fatalf("unexpected child node %#v", stack.Nodes[1])
	}
}
