package service

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"sophia/internal/model"
)

func TestAddChildCRFromCurrentUsesActiveCRAsParent(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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

	childMergeSHA, err := svc.MergeCR(child.ID, false, "")
	if err != nil {
		t.Fatalf("MergeCR(child) should succeed before parent when delegated, got %v", err)
	}
	if currentBranch := runGit(t, dir, "branch", "--show-current"); currentBranch != parent.Branch {
		t.Fatalf("expected child merge to leave current branch on parent %q, got %q", parent.Branch, currentBranch)
	}
	if got := runGit(t, dir, "rev-parse", "--verify", parent.Branch); strings.TrimSpace(got) == "" {
		t.Fatalf("expected parent branch %q to remain after child merge", parent.Branch)
	}
	if !gitRefContainsCommit(t, dir, parent.Branch, childMergeSHA) {
		t.Fatalf("expected parent branch %q to contain child merge commit %q", parent.Branch, childMergeSHA)
	}
	if gitRefContainsCommit(t, dir, parent.BaseBranch, childMergeSHA) {
		t.Fatalf("expected base branch %q not to contain child merge commit %q before parent merge", parent.BaseBranch, childMergeSHA)
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

func gitRefContainsCommit(t *testing.T, dir, ref, commit string) bool {
	t.Helper()
	cmd := exec.Command("git", "merge-base", "--is-ancestor", strings.TrimSpace(commit), strings.TrimSpace(ref))
	cmd.Dir = dir
	if err := cmd.Run(); err == nil {
		return true
	} else if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
		return false
	} else if err != nil {
		t.Fatalf("git merge-base --is-ancestor %s %s failed: %v", commit, ref, err)
	}
	return false
}

func TestUndelegateTaskReturnsDelegatedTaskToOpenWhenLastLinkRemoved(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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

func TestStackCRUsesInferredParentageWhenStoredParentMissing(t *testing.T) {
	t.Parallel()
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

	child, _, err := svc.AddCRWithOptionsWithWarnings("Child", "stack child", AddCROptions{ParentCRID: parent.ID})
	if err != nil {
		t.Fatalf("AddCR(child) error = %v", err)
	}
	setValidContract(t, svc, child.ID)

	loadedChild, err := svc.store.LoadCR(child.ID)
	if err != nil {
		t.Fatalf("LoadCR(child) error = %v", err)
	}
	loadedChild.ParentCRID = 0
	if err := svc.store.SaveCR(loadedChild); err != nil {
		t.Fatalf("SaveCR(clear parent) error = %v", err)
	}

	stack, err := svc.StackCR(parent.ID)
	if err != nil {
		t.Fatalf("StackCR() error = %v", err)
	}
	if stack.RootCRID != parent.ID {
		t.Fatalf("expected root %d, got %#v", parent.ID, stack)
	}
	if len(stack.Nodes) != 2 {
		t.Fatalf("expected 2 stack nodes, got %#v", stack.Nodes)
	}
	if stack.Nodes[1].ID != child.ID || stack.Nodes[1].ParentCRID != parent.ID {
		t.Fatalf("expected inferred parent %d for child node, got %#v", parent.ID, stack.Nodes[1])
	}
	if len(stack.Nodes[0].Children) != 1 || stack.Nodes[0].Children[0] != child.ID {
		t.Fatalf("expected child listed under parent node, got %#v", stack.Nodes[0])
	}
}

func TestRepairDelegatedParentTaskStateRepairsMergedAggregateParent(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	parent, err := svc.AddCR("Parent", "aggregate parent")
	if err != nil {
		t.Fatalf("AddCR(parent) error = %v", err)
	}
	setValidContract(t, svc, parent.ID)
	parentTask, err := svc.AddTask(parent.ID, "delegated child work")
	if err != nil {
		t.Fatalf("AddTask(parent) error = %v", err)
	}
	setValidTaskContract(t, svc, parent.ID, parentTask.ID)

	child, _, err := svc.AddCRWithOptionsWithWarnings("Child", "delegated implementation", AddCROptions{ParentCRID: parent.ID})
	if err != nil {
		t.Fatalf("AddCR(child) error = %v", err)
	}
	setValidContract(t, svc, child.ID)
	if _, err := svc.DelegateTaskToChild(parent.ID, parentTask.ID, child.ID); err != nil {
		t.Fatalf("DelegateTaskToChild() error = %v", err)
	}

	parentCR, err := svc.store.LoadCR(parent.ID)
	if err != nil {
		t.Fatalf("LoadCR(parent) error = %v", err)
	}
	childCR, err := svc.store.LoadCR(child.ID)
	if err != nil {
		t.Fatalf("LoadCR(child) error = %v", err)
	}
	childCR.Status = model.StatusMerged
	childCR.MergedAt = svc.timestamp()
	childCR.MergedBy = "tester"
	childCR.MergedCommit = "childmerged"
	if err := svc.store.SaveCR(childCR); err != nil {
		t.Fatalf("SaveCR(child merged) error = %v", err)
	}

	taskIndex := indexOfTask(parentCR.Subtasks, parentTask.ID)
	if taskIndex < 0 {
		t.Fatalf("expected parent task %d", parentTask.ID)
	}
	parentCR.Status = model.StatusMerged
	parentCR.MergedAt = svc.timestamp()
	parentCR.MergedBy = "tester"
	parentCR.MergedCommit = "parentmerged"
	parentCR.Subtasks[taskIndex].Status = model.TaskStatusDelegated
	parentCR.Subtasks[taskIndex].CompletedAt = ""
	parentCR.Subtasks[taskIndex].CompletedBy = ""
	if err := svc.store.SaveCR(parentCR); err != nil {
		t.Fatalf("SaveCR(parent stale merged) error = %v", err)
	}

	if err := svc.repairDelegatedParentTaskState(); err != nil {
		t.Fatalf("repairDelegatedParentTaskState() error = %v", err)
	}

	repairedParent, err := svc.store.LoadCR(parent.ID)
	if err != nil {
		t.Fatalf("LoadCR(repaired parent) error = %v", err)
	}
	repairedTask := repairedParent.Subtasks[taskIndex]
	if repairedTask.Status != model.TaskStatusDone {
		t.Fatalf("expected repaired task done, got %#v", repairedTask)
	}
	if strings.TrimSpace(repairedTask.CompletedAt) == "" || strings.TrimSpace(repairedTask.CompletedBy) == "" {
		t.Fatalf("expected repaired completion metadata, got %#v", repairedTask)
	}
	if len(repairedTask.Delegations) != 1 || repairedTask.Delegations[0].ChildCRID != child.ID {
		t.Fatalf("expected delegation linkage preserved, got %#v", repairedTask.Delegations)
	}
}

func TestRepairFromGitDoesNotAutoCompleteDelegatedParentAfterPartialChildLanding(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")

	parent, err := svc.AddCR("Parent", "delegated parent")
	if err != nil {
		t.Fatalf("AddCR(parent) error = %v", err)
	}
	setValidContract(t, svc, parent.ID)
	parentTask, err := svc.AddTask(parent.ID, "Delegated child work")
	if err != nil {
		t.Fatalf("AddTask(parent) error = %v", err)
	}
	setValidTaskContract(t, svc, parent.ID, parentTask.ID)

	child, _, err := svc.AddCRWithOptionsWithWarnings("Child", "delegated implementation", AddCROptions{ParentCRID: parent.ID})
	if err != nil {
		t.Fatalf("AddCR(child) error = %v", err)
	}
	setValidContract(t, svc, child.ID)
	delegateResult, err := svc.DelegateTaskToChild(parent.ID, parentTask.ID, child.ID)
	if err != nil {
		t.Fatalf("DelegateTaskToChild() error = %v", err)
	}

	if err := os.WriteFile(filepath.Join(dir, "child.txt"), []byte("stage one\n"), 0o644); err != nil {
		t.Fatalf("write child.txt stage one: %v", err)
	}
	runGit(t, dir, "add", "child.txt")
	runGit(t, dir, "commit", "-m", "feat: delegated slice")
	checkpointCommit := runGit(t, dir, "rev-parse", "--verify", "HEAD")
	runGit(t, dir, "branch", "child-landing", checkpointCommit)

	childCR, err := svc.store.LoadCR(child.ID)
	if err != nil {
		t.Fatalf("LoadCR(child) error = %v", err)
	}
	delegatedIdx := indexOfTask(childCR.Subtasks, delegateResult.ChildTaskID)
	if delegatedIdx < 0 {
		t.Fatalf("expected delegated child task %d", delegateResult.ChildTaskID)
	}
	childCR.Subtasks[delegatedIdx].Status = model.TaskStatusDone
	childCR.Subtasks[delegatedIdx].CompletedAt = harnessTimestamp
	childCR.Subtasks[delegatedIdx].CompletedBy = "Test User <test@example.com>"
	childCR.Subtasks[delegatedIdx].CheckpointCommit = checkpointCommit
	childCR.Subtasks[delegatedIdx].CheckpointSource = model.TaskCheckpointSourceTaskCheckpoint
	childCR.Subtasks = append(childCR.Subtasks, model.Subtask{
		ID:        delegateResult.ChildTaskID + 1,
		Title:     "Remaining child work",
		Status:    model.TaskStatusOpen,
		CreatedAt: harnessTimestamp,
		UpdatedAt: harnessTimestamp,
		CreatedBy: "Test User <test@example.com>",
	})
	if err := svc.store.SaveCR(childCR); err != nil {
		t.Fatalf("SaveCR(child) error = %v", err)
	}

	if err := os.WriteFile(filepath.Join(dir, "child.txt"), []byte("stage one\nstage two\n"), 0o644); err != nil {
		t.Fatalf("write child.txt stage two: %v", err)
	}
	runGit(t, dir, "add", "child.txt")
	runGit(t, dir, "commit", "-m", "feat: unfinished child follow-up")

	runGit(t, dir, "checkout", "main")
	runGit(t, dir, "merge", "--ff-only", "child-landing")

	report, err := svc.RepairFromGit("main", true)
	if err != nil {
		t.Fatalf("RepairFromGit(refresh) error = %v", err)
	}
	if containsInt(report.RepairedCRIDs, child.ID) {
		t.Fatalf("expected child CR %d to remain in progress, repaired set = %#v", child.ID, report.RepairedCRIDs)
	}

	reloadedChild, err := svc.store.LoadCR(child.ID)
	if err != nil {
		t.Fatalf("LoadCR(reloaded child) error = %v", err)
	}
	if reloadedChild.Status != model.StatusInProgress {
		t.Fatalf("expected child to remain in progress, got %#v", reloadedChild)
	}

	reloadedParent, err := svc.store.LoadCR(parent.ID)
	if err != nil {
		t.Fatalf("LoadCR(reloaded parent) error = %v", err)
	}
	parentTaskIndex := indexOfTask(reloadedParent.Subtasks, parentTask.ID)
	if parentTaskIndex < 0 {
		t.Fatalf("expected parent task %d", parentTask.ID)
	}
	if reloadedParent.Subtasks[parentTaskIndex].Status != model.TaskStatusDelegated {
		t.Fatalf("expected parent delegated task to remain delegated, got %#v", reloadedParent.Subtasks[parentTaskIndex])
	}
}

func TestStatusCRMarksAggregateParentResolvedAndPendingChildren(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	parent, err := svc.AddCR("Parent", "aggregate status")
	if err != nil {
		t.Fatalf("AddCR(parent) error = %v", err)
	}
	setValidContract(t, svc, parent.ID)

	taskResolved, err := svc.AddTask(parent.ID, "resolved delegate")
	if err != nil {
		t.Fatalf("AddTask(resolved) error = %v", err)
	}
	setValidTaskContract(t, svc, parent.ID, taskResolved.ID)
	taskPending, err := svc.AddTask(parent.ID, "pending delegate")
	if err != nil {
		t.Fatalf("AddTask(pending) error = %v", err)
	}
	setValidTaskContract(t, svc, parent.ID, taskPending.ID)

	childResolved, _, err := svc.AddCRWithOptionsWithWarnings("Child resolved", "delegated implementation", AddCROptions{ParentCRID: parent.ID})
	if err != nil {
		t.Fatalf("AddCR(childResolved) error = %v", err)
	}
	setValidContract(t, svc, childResolved.ID)
	childPending, _, err := svc.AddCRWithOptionsWithWarnings("Child pending", "delegated implementation", AddCROptions{ParentCRID: parent.ID})
	if err != nil {
		t.Fatalf("AddCR(childPending) error = %v", err)
	}
	setValidContract(t, svc, childPending.ID)

	if _, err := svc.DelegateTaskToChild(parent.ID, taskResolved.ID, childResolved.ID); err != nil {
		t.Fatalf("DelegateTaskToChild(resolved) error = %v", err)
	}
	if _, err := svc.DelegateTaskToChild(parent.ID, taskPending.ID, childPending.ID); err != nil {
		t.Fatalf("DelegateTaskToChild(pending) error = %v", err)
	}

	childResolvedCR, err := svc.store.LoadCR(childResolved.ID)
	if err != nil {
		t.Fatalf("LoadCR(childResolved) error = %v", err)
	}
	childResolvedCR.Status = model.StatusMerged
	childResolvedCR.MergedAt = svc.timestamp()
	childResolvedCR.MergedBy = "tester"
	childResolvedCR.MergedCommit = "childresolved"
	if err := svc.store.SaveCR(childResolvedCR); err != nil {
		t.Fatalf("SaveCR(childResolved merged) error = %v", err)
	}
	if err := svc.syncDelegatedTasksAfterChildMerge(childResolved.ID); err != nil {
		t.Fatalf("syncDelegatedTasksAfterChildMerge() error = %v", err)
	}

	status, err := svc.StatusCR(parent.ID)
	if err != nil {
		t.Fatalf("StatusCR() error = %v", err)
	}
	if !status.IsAggregateParent {
		t.Fatalf("expected aggregate parent status, got %#v", status)
	}
	if got := joinIntIDs(status.AggregateResolvedChildren); got != strconv.Itoa(childResolved.ID) {
		t.Fatalf("expected resolved children [%d], got %v", childResolved.ID, status.AggregateResolvedChildren)
	}
	if got := joinIntIDs(status.AggregatePendingChildren); got != strconv.Itoa(childPending.ID) {
		t.Fatalf("expected pending children [%d], got %v", childPending.ID, status.AggregatePendingChildren)
	}

	stack, err := svc.StackCR(parent.ID)
	if err != nil {
		t.Fatalf("StackCR() error = %v", err)
	}
	if len(stack.Nodes) == 0 || !stack.Nodes[0].IsAggregateParent {
		t.Fatalf("expected aggregate parent node, got %#v", stack.Nodes)
	}
	if got := joinIntIDs(stack.Nodes[0].ResolvedChildCRIDs); got != strconv.Itoa(childResolved.ID) {
		t.Fatalf("expected stack resolved children [%d], got %v", childResolved.ID, stack.Nodes[0].ResolvedChildCRIDs)
	}
	if got := joinIntIDs(stack.Nodes[0].PendingChildCRIDs); got != strconv.Itoa(childPending.ID) {
		t.Fatalf("expected stack pending children [%d], got %v", childPending.ID, stack.Nodes[0].PendingChildCRIDs)
	}
}

func TestMergeFinalizeBlocksAggregateParentWithDelegatedChildrenPendingEvenWhenPRGateChecksPass(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SOPHIA.yaml"), []byte("version: v1\nmerge:\n  mode: pr_gate\narchive:\n  enabled: false\n"), 0o644); err != nil {
		t.Fatalf("write SOPHIA.yaml: %v", err)
	}

	parent, err := svc.AddCR("Aggregate finalize blocked", "delegated child still pending")
	if err != nil {
		t.Fatalf("AddCR(parent) error = %v", err)
	}
	loaded, err := svc.store.LoadCR(parent.ID)
	if err != nil {
		t.Fatalf("LoadCR(parent) error = %v", err)
	}
	loaded.PR.Number = 88
	loaded.PR.Repo = "acme/repo"
	loaded.Subtasks = []model.Subtask{{
		ID:     1,
		Title:  "Delegated child work",
		Status: model.TaskStatusDelegated,
		Delegations: []model.TaskDelegation{
			{ChildCRID: 401, ChildTaskID: 1},
		},
	}}
	if err := svc.store.SaveCR(loaded); err != nil {
		t.Fatalf("SaveCR(parent) error = %v", err)
	}

	svc.overrideGHRunnerForTests(func(_ string, args ...string) (string, error) {
		if len(args) >= 3 && args[0] == "pr" && args[1] == "view" {
			payload := map[string]any{
				"number":      88,
				"url":         "https://github.com/acme/repo/pull/88",
				"state":       "OPEN",
				"isDraft":     false,
				"headRefOid":  "abc123",
				"headRefName": strings.TrimSpace(parent.Branch),
				"baseRefName": "main",
				"author":      map[string]any{"login": "author"},
				"latestReviews": []map[string]any{
					{"state": "APPROVED", "author": map[string]any{"login": "reviewer"}},
				},
				"statusCheckRollup": []map[string]any{
					{"status": "COMPLETED", "conclusion": "SUCCESS"},
				},
			}
			raw, err := json.Marshal(payload)
			if err != nil {
				return "", err
			}
			return string(raw), nil
		}
		if len(args) >= 3 && args[0] == "pr" && args[1] == "merge" {
			t.Fatalf("expected finalize to block before gh pr merge, got args=%v", args)
		}
		return "", nil
	})

	_, err = svc.MergeFinalizeWithOptions(parent.ID, MergeCROptions{})
	if err == nil || !strings.Contains(err.Error(), "delegated child CRs pending") {
		t.Fatalf("expected delegated-child finalize block, got %v", err)
	}
}
