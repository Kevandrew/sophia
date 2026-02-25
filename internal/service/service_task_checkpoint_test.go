package service

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sophia/internal/model"
)

func TestPatchFileDisplayNameUsesBaseName(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "task.patch")
	if got := patchFileDisplayName(path); got != "task.patch" {
		t.Fatalf("expected base filename, got %q", got)
	}
}

func TestReadPatchManifestContentRejectsOversize(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "huge.patch")
	payload := bytes.Repeat([]byte("x"), maxPatchManifestBytes+1)
	if err := os.WriteFile(path, payload, 0o644); err != nil {
		t.Fatalf("write huge patch: %v", err)
	}
	if _, err := readPatchManifestContent(path); err == nil {
		t.Fatalf("expected oversize patch manifest error")
	}
}

func TestTaskAddAndDonePreservesOrderAndStatus(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if _, err := svc.AddCR("Bootstrap", "Scaffold CLI"); err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}

	t1, err := svc.AddTask(1, "Implement CLI")
	if err != nil {
		t.Fatalf("AddTask() #1 error = %v", err)
	}
	t2, err := svc.AddTask(1, "Add tests")
	if err != nil {
		t.Fatalf("AddTask() #2 error = %v", err)
	}
	setValidTaskContract(t, svc, 1, t1.ID)
	if err := svc.DoneTask(1, t1.ID); err != nil {
		t.Fatalf("DoneTask() error = %v", err)
	}

	tasks, err := svc.ListTasks(1)
	if err != nil {
		t.Fatalf("ListTasks() error = %v", err)
	}
	if len(tasks) != 2 {
		t.Fatalf("expected 2 tasks, got %d", len(tasks))
	}
	if tasks[0].ID != t1.ID || tasks[1].ID != t2.ID {
		t.Fatalf("task order changed: %#v", tasks)
	}
	if tasks[0].Status != "done" {
		t.Fatalf("expected task 1 done, got %q", tasks[0].Status)
	}

	cr, err := svc.store.LoadCR(1)
	if err != nil {
		t.Fatalf("LoadCR() error = %v", err)
	}
	if got := cr.Events[len(cr.Events)-1].Type; got != model.EventTypeTaskDone {
		t.Fatalf("expected last event task_done, got %q", got)
	}
}

func TestDoneTaskWithCheckpointCreatesCommit(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")

	cr, err := svc.AddCR("Checkpoint CR", "checkpoint behavior")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	task, err := svc.AddTask(cr.ID, "feat: implement checkpoint workflow")
	if err != nil {
		t.Fatalf("AddTask() error = %v", err)
	}
	setValidTaskContract(t, svc, cr.ID, task.ID)
	if err := os.WriteFile(filepath.Join(dir, "checkpoint.txt"), []byte("checkpoint\n"), 0o644); err != nil {
		t.Fatalf("write checkpoint file: %v", err)
	}

	sha, err := svc.DoneTaskWithCheckpoint(cr.ID, task.ID, DoneTaskOptions{Checkpoint: true, StageAll: true})
	if err != nil {
		t.Fatalf("DoneTaskWithCheckpoint() error = %v", err)
	}
	if sha == "" {
		t.Fatalf("expected checkpoint sha")
	}

	msg := runGit(t, dir, "log", "-1", "--pretty=%B")
	if !strings.Contains(msg, "feat(cr-1/task-1): feat: implement checkpoint workflow") {
		t.Fatalf("unexpected checkpoint subject: %q", msg)
	}
	for _, footer := range []string{"Sophia-CR: 1", "Sophia-CR-UID: " + cr.UID, "Sophia-Base-Ref: " + cr.BaseRef, "Sophia-Base-Commit: " + cr.BaseCommit, "Sophia-Task: 1", "Sophia-Intent: Checkpoint CR"} {
		if !strings.Contains(msg, footer) {
			t.Fatalf("expected checkpoint footer %q in message: %q", footer, msg)
		}
	}

	loaded, err := svc.store.LoadCR(cr.ID)
	if err != nil {
		t.Fatalf("LoadCR() error = %v", err)
	}
	if loaded.Subtasks[0].Status != model.TaskStatusDone {
		t.Fatalf("expected task done, got %q", loaded.Subtasks[0].Status)
	}
	if loaded.Subtasks[0].CheckpointCommit == "" || loaded.Subtasks[0].CheckpointAt == "" {
		t.Fatalf("expected checkpoint metadata on task, got %#v", loaded.Subtasks[0])
	}
	lastTwo := loaded.Events[len(loaded.Events)-2:]
	if lastTwo[0].Type != model.EventTypeTaskCheckpointed || lastTwo[1].Type != model.EventTypeTaskDone {
		t.Fatalf("expected checkpoint then done events, got %#v", lastTwo)
	}
}

func TestDoneTaskWithCheckpointAfterReopenUsesV2Suffix(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")

	cr, err := svc.AddCR("Checkpoint retry CR", "retry behavior")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	task, err := svc.AddTask(cr.ID, "feat: implement checkpoint retry workflow")
	if err != nil {
		t.Fatalf("AddTask() error = %v", err)
	}
	setValidTaskContract(t, svc, cr.ID, task.ID)
	path := filepath.Join(dir, "checkpoint-retry.txt")
	if err := os.WriteFile(path, []byte("first\n"), 0o644); err != nil {
		t.Fatalf("write checkpoint file: %v", err)
	}

	if _, err := svc.DoneTaskWithCheckpoint(cr.ID, task.ID, DoneTaskOptions{Checkpoint: true, StageAll: true}); err != nil {
		t.Fatalf("DoneTaskWithCheckpoint(first) error = %v", err)
	}
	if _, err := svc.ReopenTask(cr.ID, task.ID, ReopenTaskOptions{}); err != nil {
		t.Fatalf("ReopenTask() error = %v", err)
	}
	if err := os.WriteFile(path, []byte("second\n"), 0o644); err != nil {
		t.Fatalf("write checkpoint file second pass: %v", err)
	}

	if _, err := svc.DoneTaskWithCheckpoint(cr.ID, task.ID, DoneTaskOptions{Checkpoint: true, StageAll: true}); err != nil {
		t.Fatalf("DoneTaskWithCheckpoint(second) error = %v", err)
	}
	msg := runGit(t, dir, "log", "-1", "--pretty=%B")
	if !strings.Contains(msg, "feat(cr-1/task-1v2): feat: implement checkpoint retry workflow") {
		t.Fatalf("unexpected reopened checkpoint subject: %q", msg)
	}
}

func TestDoneTaskWithCheckpointAfterTwoReopensUsesV3Suffix(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")

	cr, err := svc.AddCR("Checkpoint v3 CR", "third attempt behavior")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	task, err := svc.AddTask(cr.ID, "fix: checkpoint third attempt")
	if err != nil {
		t.Fatalf("AddTask() error = %v", err)
	}
	setValidTaskContract(t, svc, cr.ID, task.ID)
	path := filepath.Join(dir, "checkpoint-v3.txt")
	if err := os.WriteFile(path, []byte("first\n"), 0o644); err != nil {
		t.Fatalf("write checkpoint file: %v", err)
	}

	if _, err := svc.DoneTaskWithCheckpoint(cr.ID, task.ID, DoneTaskOptions{Checkpoint: true, StageAll: true}); err != nil {
		t.Fatalf("DoneTaskWithCheckpoint(first) error = %v", err)
	}
	if _, err := svc.ReopenTask(cr.ID, task.ID, ReopenTaskOptions{}); err != nil {
		t.Fatalf("ReopenTask(first) error = %v", err)
	}
	if err := os.WriteFile(path, []byte("second\n"), 0o644); err != nil {
		t.Fatalf("write checkpoint file second pass: %v", err)
	}
	if _, err := svc.DoneTaskWithCheckpoint(cr.ID, task.ID, DoneTaskOptions{Checkpoint: true, StageAll: true}); err != nil {
		t.Fatalf("DoneTaskWithCheckpoint(second) error = %v", err)
	}
	if _, err := svc.ReopenTask(cr.ID, task.ID, ReopenTaskOptions{}); err != nil {
		t.Fatalf("ReopenTask(second) error = %v", err)
	}
	if err := os.WriteFile(path, []byte("third\n"), 0o644); err != nil {
		t.Fatalf("write checkpoint file third pass: %v", err)
	}

	if _, err := svc.DoneTaskWithCheckpoint(cr.ID, task.ID, DoneTaskOptions{Checkpoint: true, StageAll: true}); err != nil {
		t.Fatalf("DoneTaskWithCheckpoint(third) error = %v", err)
	}
	msg := runGit(t, dir, "log", "-1", "--pretty=%B")
	if !strings.Contains(msg, "fix(cr-1/task-1v3): fix: checkpoint third attempt") {
		t.Fatalf("unexpected v3 checkpoint subject: %q", msg)
	}
}

func TestDoneTaskWithCheckpointAfterNoCheckpointReopenUsesV2Suffix(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")

	cr, err := svc.AddCR("No checkpoint reopen CR", "no-checkpoint reopen behavior")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	task, err := svc.AddTask(cr.ID, "chore: no-checkpoint reopen suffix")
	if err != nil {
		t.Fatalf("AddTask() error = %v", err)
	}
	setValidTaskContract(t, svc, cr.ID, task.ID)

	if _, err := svc.DoneTaskWithCheckpoint(cr.ID, task.ID, DoneTaskOptions{
		Checkpoint:         false,
		NoCheckpointReason: "metadata-only first completion",
	}); err != nil {
		t.Fatalf("DoneTaskWithCheckpoint(no checkpoint) error = %v", err)
	}
	if _, err := svc.ReopenTask(cr.ID, task.ID, ReopenTaskOptions{}); err != nil {
		t.Fatalf("ReopenTask() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "checkpoint-no-checkpoint.txt"), []byte("now committed\n"), 0o644); err != nil {
		t.Fatalf("write checkpoint file: %v", err)
	}

	if _, err := svc.DoneTaskWithCheckpoint(cr.ID, task.ID, DoneTaskOptions{Checkpoint: true, StageAll: true}); err != nil {
		t.Fatalf("DoneTaskWithCheckpoint(with checkpoint) error = %v", err)
	}
	msg := runGit(t, dir, "log", "-1", "--pretty=%B")
	if !strings.Contains(msg, "chore(cr-1/task-1v2): chore: no-checkpoint reopen suffix") {
		t.Fatalf("unexpected v2 checkpoint subject after no-checkpoint reopen: %q", msg)
	}
}

func TestDoneTaskWithCheckpointNoChangesKeepsTaskOpen(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	cr, err := svc.AddCR("No change CR", "no changes")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	task, err := svc.AddTask(cr.ID, "feat: no-op task")
	if err != nil {
		t.Fatalf("AddTask() error = %v", err)
	}
	setValidTaskContract(t, svc, cr.ID, task.ID)

	if _, err := svc.DoneTaskWithCheckpoint(cr.ID, task.ID, DoneTaskOptions{Checkpoint: true, StageAll: true}); !errors.Is(err, ErrNoTaskChanges) {
		t.Fatalf("expected ErrNoTaskChanges, got %v", err)
	}

	loaded, err := svc.store.LoadCR(cr.ID)
	if err != nil {
		t.Fatalf("LoadCR() error = %v", err)
	}
	if loaded.Subtasks[0].Status != model.TaskStatusOpen {
		t.Fatalf("expected task to remain open, got %q", loaded.Subtasks[0].Status)
	}
}

func TestDoneTaskWithNoCheckpointIsMetadataOnly(t *testing.T) {
	cr := seedCR(1, "Metadata only", seedCROptions{
		Description: "done without commit",
		Branch:      "cr-1-runtime",
	})
	task := seedTask(1, "docs: update note", model.TaskStatusOpen, "")
	task.Contract = model.TaskContract{
		Intent:             "Finish metadata-only task.",
		AcceptanceCriteria: []string{"Task can be marked done without commit."},
		Scope:              []string{"internal/service"},
	}
	cr.Subtasks = []model.Subtask{task}
	h := harnessService(t, runtimeHarnessOptions{Branch: cr.Branch, CRs: []*model.CR{cr}})

	sha, err := h.Service.doneTaskWithCheckpointUnlocked(cr.ID, task.ID, DoneTaskOptions{
		Checkpoint:         false,
		NoCheckpointReason: "metadata-only completion",
	})
	if err != nil {
		t.Fatalf("doneTaskWithCheckpointUnlocked(checkpoint=false) error = %v", err)
	}
	if sha != "" {
		t.Fatalf("expected empty sha for metadata-only completion, got %q", sha)
	}
	loaded, err := h.Store.LoadCR(cr.ID)
	if err != nil {
		t.Fatalf("LoadCR() error = %v", err)
	}
	if loaded.Subtasks[0].Status != model.TaskStatusDone {
		t.Fatalf("expected task done, got %q", loaded.Subtasks[0].Status)
	}
	if loaded.Subtasks[0].CheckpointCommit != "" {
		t.Fatalf("expected no checkpoint commit metadata, got %#v", loaded.Subtasks[0])
	}
	if loaded.Subtasks[0].CheckpointSource != "task_no_checkpoint" {
		t.Fatalf("expected checkpoint_source task_no_checkpoint, got %#v", loaded.Subtasks[0].CheckpointSource)
	}
	if strings.TrimSpace(loaded.Subtasks[0].CheckpointReason) == "" {
		t.Fatalf("expected checkpoint_reason for no-checkpoint completion")
	}
}

func TestDoneTaskWithNoCheckpointRequiresReason(t *testing.T) {
	cr := seedCR(1, "Metadata reason", seedCROptions{
		Description: "require no-checkpoint rationale",
		Branch:      "cr-1-runtime",
	})
	cr.Subtasks = []model.Subtask{seedTask(1, "docs: metadata completion", model.TaskStatusOpen, "")}
	h := harnessService(t, runtimeHarnessOptions{Branch: cr.Branch, CRs: []*model.CR{cr}})

	_, err := h.Service.doneTaskWithCheckpointUnlocked(cr.ID, 1, DoneTaskOptions{Checkpoint: false})
	if err == nil || !strings.Contains(err.Error(), "--no-checkpoint requires --no-checkpoint-reason") {
		t.Fatalf("expected no-checkpoint reason error, got %v", err)
	}
}

func TestDoneTaskWithCheckpointRequiresCRBranch(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	cr, err := svc.AddCR("Branch guard", "require branch")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	task, err := svc.AddTask(cr.ID, "fix: branch guard")
	if err != nil {
		t.Fatalf("AddTask() error = %v", err)
	}
	setValidTaskContract(t, svc, cr.ID, task.ID)
	runGit(t, dir, "checkout", "main")
	if err := os.WriteFile(filepath.Join(dir, "branch.txt"), []byte("x\n"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	if _, err := svc.DoneTaskWithCheckpoint(cr.ID, task.ID, DoneTaskOptions{Checkpoint: true, StageAll: true}); err == nil {
		t.Fatalf("expected branch context error")
	}
}

func TestDoneTaskWithCheckpointScopesToSelectedPaths(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")

	cr, err := svc.AddCR("Scoped checkpoint", "scope only selected files")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	task, err := svc.AddTask(cr.ID, "feat: scoped staging")
	if err != nil {
		t.Fatalf("AddTask() error = %v", err)
	}
	setValidTaskContract(t, svc, cr.ID, task.ID)

	if err := os.WriteFile(filepath.Join(dir, "scoped.txt"), []byte("scoped\n"), 0o644); err != nil {
		t.Fatalf("write scoped file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "unscoped.txt"), []byte("unscoped\n"), 0o644); err != nil {
		t.Fatalf("write unscoped file: %v", err)
	}

	sha, err := svc.DoneTaskWithCheckpoint(cr.ID, task.ID, DoneTaskOptions{
		Checkpoint: true,
		Paths:      []string{"scoped.txt"},
	})
	if err != nil {
		t.Fatalf("DoneTaskWithCheckpoint() error = %v", err)
	}
	if sha == "" {
		t.Fatalf("expected checkpoint sha")
	}

	commitFiles := runGit(t, dir, "show", "--pretty=format:", "--name-only", "-1")
	if !strings.Contains(commitFiles, "scoped.txt") {
		t.Fatalf("expected scoped.txt in checkpoint commit, got %q", commitFiles)
	}
	if strings.Contains(commitFiles, "unscoped.txt") {
		t.Fatalf("did not expect unscoped.txt in checkpoint commit, got %q", commitFiles)
	}

	status := runGit(t, dir, "status", "--porcelain")
	if !strings.Contains(status, "?? unscoped.txt") {
		t.Fatalf("expected unscoped.txt to remain uncommitted, status=%q", status)
	}

	loaded, err := svc.store.LoadCR(cr.ID)
	if err != nil {
		t.Fatalf("LoadCR() error = %v", err)
	}
	if len(loaded.Subtasks[0].CheckpointScope) != 1 || loaded.Subtasks[0].CheckpointScope[0] != "scoped.txt" {
		t.Fatalf("expected checkpoint_scope [scoped.txt], got %#v", loaded.Subtasks[0].CheckpointScope)
	}
}

func TestDoneTaskWithCheckpointFromContractScopesToChangedFiles(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")

	cr, err := svc.AddCR("Contract-scoped checkpoint", "scope from task contract")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	task, err := svc.AddTask(cr.ID, "feat: contract scoped staging")
	if err != nil {
		t.Fatalf("AddTask() error = %v", err)
	}
	intent := "Scope checkpoint to task contract prefixes."
	acceptance := []string{"Only in-scope files are checkpointed."}
	scope := []string{"scoped"}
	if _, err := svc.SetTaskContract(cr.ID, task.ID, TaskContractPatch{
		Intent:             &intent,
		AcceptanceCriteria: &acceptance,
		Scope:              &scope,
	}); err != nil {
		t.Fatalf("SetTaskContract() error = %v", err)
	}

	if err := os.MkdirAll(filepath.Join(dir, "scoped"), 0o755); err != nil {
		t.Fatalf("mkdir scoped: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "unscoped"), 0o755); err != nil {
		t.Fatalf("mkdir unscoped: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "scoped", "in.txt"), []byte("in\n"), 0o644); err != nil {
		t.Fatalf("write scoped file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "unscoped", "out.txt"), []byte("out\n"), 0o644); err != nil {
		t.Fatalf("write unscoped file: %v", err)
	}

	sha, err := svc.DoneTaskWithCheckpoint(cr.ID, task.ID, DoneTaskOptions{
		Checkpoint:   true,
		FromContract: true,
	})
	if err != nil {
		t.Fatalf("DoneTaskWithCheckpoint() error = %v", err)
	}
	if sha == "" {
		t.Fatalf("expected checkpoint sha")
	}

	commitFiles := runGit(t, dir, "show", "--pretty=format:", "--name-only", "-1")
	if !strings.Contains(commitFiles, "scoped/in.txt") {
		t.Fatalf("expected scoped/in.txt in checkpoint commit, got %q", commitFiles)
	}
	if strings.Contains(commitFiles, "unscoped/out.txt") {
		t.Fatalf("did not expect unscoped/out.txt in checkpoint commit, got %q", commitFiles)
	}

	status := runGit(t, dir, "status", "--porcelain")
	if !strings.Contains(status, "?? unscoped/") {
		t.Fatalf("expected unscoped changes to remain uncommitted, status=%q", status)
	}

	loaded, err := svc.store.LoadCR(cr.ID)
	if err != nil {
		t.Fatalf("LoadCR() error = %v", err)
	}
	if len(loaded.Subtasks[0].CheckpointScope) != 1 || loaded.Subtasks[0].CheckpointScope[0] != "scoped/in.txt" {
		t.Fatalf("expected checkpoint scope from contract path, got %#v", loaded.Subtasks[0].CheckpointScope)
	}
	lastTwo := loaded.Events[len(loaded.Events)-2:]
	if lastTwo[0].Type != model.EventTypeTaskCheckpointed {
		t.Fatalf("expected task_checkpointed event, got %#v", lastTwo)
	}
	if lastTwo[0].Meta["scope_source"] != "task_contract" {
		t.Fatalf("expected scope_source=task_contract, got %#v", lastTwo[0].Meta)
	}
}

func TestDoneTaskWithCheckpointFromContractNoMatchesFails(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	cr, err := svc.AddCR("No contract matches", "contract scope has no matching changes")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	task, err := svc.AddTask(cr.ID, "feat: no matching in-scope files")
	if err != nil {
		t.Fatalf("AddTask() error = %v", err)
	}
	intent := "Require contract-scoped file matches."
	acceptance := []string{"Fail when no changed files match scope."}
	scope := []string{"scoped"}
	if _, err := svc.SetTaskContract(cr.ID, task.ID, TaskContractPatch{
		Intent:             &intent,
		AcceptanceCriteria: &acceptance,
		Scope:              &scope,
	}); err != nil {
		t.Fatalf("SetTaskContract() error = %v", err)
	}

	if err := os.WriteFile(filepath.Join(dir, "outside.txt"), []byte("outside\n"), 0o644); err != nil {
		t.Fatalf("write outside file: %v", err)
	}
	_, err = svc.DoneTaskWithCheckpoint(cr.ID, task.ID, DoneTaskOptions{
		Checkpoint:   true,
		FromContract: true,
	})
	if !errors.Is(err, ErrNoTaskScopeMatches) {
		t.Fatalf("expected ErrNoTaskScopeMatches, got %v", err)
	}
}

func TestDoneTaskWithCheckpointPatchFileScopesToSelectedHunks(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")

	if err := os.WriteFile(filepath.Join(dir, "chunked.txt"), []byte("l1\nl2\nl3\nl4\nl5\nl6\nl7\nl8\n"), 0o644); err != nil {
		t.Fatalf("write seed file: %v", err)
	}
	runGit(t, dir, "add", "chunked.txt")
	runGit(t, dir, "commit", "-m", "chore: seed chunk file")

	cr, err := svc.AddCR("Patch scoped checkpoint", "stage selected hunks")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	task, err := svc.AddTask(cr.ID, "feat: patch-scoped staging")
	if err != nil {
		t.Fatalf("AddTask() error = %v", err)
	}
	setValidTaskContract(t, svc, cr.ID, task.ID)

	if err := os.WriteFile(filepath.Join(dir, "chunked.txt"), []byte("l1\nl2-edited\nl3\nl4\nl5\nl6\nl7-edited\nl8\n"), 0o644); err != nil {
		t.Fatalf("write modified file: %v", err)
	}

	fullPatch := runGit(t, dir, "diff", "--unified=0", "chunked.txt")
	partialPatch := firstHunkPatchFromDiff(t, fullPatch)
	patchPath := filepath.Join(dir, "task.patch")
	if err := os.WriteFile(patchPath, []byte(partialPatch), 0o644); err != nil {
		t.Fatalf("write patch file: %v", err)
	}

	sha, err := svc.DoneTaskWithCheckpoint(cr.ID, task.ID, DoneTaskOptions{
		Checkpoint: true,
		PatchFile:  patchPath,
	})
	if err != nil {
		t.Fatalf("DoneTaskWithCheckpoint() error = %v", err)
	}
	if sha == "" {
		t.Fatalf("expected checkpoint sha")
	}

	msg := runGit(t, dir, "log", "-1", "--pretty=%B")
	if !strings.Contains(msg, "Sophia-Task-Scope-Mode: patch_manifest") {
		t.Fatalf("expected patch scope footer in checkpoint message: %q", msg)
	}
	if !strings.Contains(msg, "Sophia-Task-Chunk-Count: 1") {
		t.Fatalf("expected patch chunk count footer in checkpoint message: %q", msg)
	}

	remaining := runGit(t, dir, "diff", "--unified=0", "chunked.txt")
	if !strings.Contains(remaining, "+l7-edited") {
		t.Fatalf("expected second hunk to remain unstaged/uncommitted, diff=%q", remaining)
	}
	if strings.Contains(remaining, "+l2-edited") {
		t.Fatalf("expected first hunk to be committed, diff=%q", remaining)
	}

	loaded, err := svc.store.LoadCR(cr.ID)
	if err != nil {
		t.Fatalf("LoadCR() error = %v", err)
	}
	if len(loaded.Subtasks[0].CheckpointScope) != 1 || loaded.Subtasks[0].CheckpointScope[0] != "chunked.txt" {
		t.Fatalf("expected checkpoint scope [chunked.txt], got %#v", loaded.Subtasks[0].CheckpointScope)
	}
	if len(loaded.Subtasks[0].CheckpointChunks) != 1 {
		t.Fatalf("expected one checkpoint chunk, got %#v", loaded.Subtasks[0].CheckpointChunks)
	}
	chunk := loaded.Subtasks[0].CheckpointChunks[0]
	if chunk.ID == "" || chunk.Path != "chunked.txt" {
		t.Fatalf("expected chunk metadata with id/path, got %#v", chunk)
	}
	lastTwo := loaded.Events[len(loaded.Events)-2:]
	if got := lastTwo[0].Meta["scope_source"]; got != "patch_manifest" {
		t.Fatalf("expected scope_source patch_manifest, got %q", got)
	}
	if got := lastTwo[0].Meta["chunk_count"]; got != "1" {
		t.Fatalf("expected chunk_count 1, got %q", got)
	}
}

func TestDoneTaskWithCheckpointPatchFileMalformedFails(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")

	if err := os.WriteFile(filepath.Join(dir, "chunked.txt"), []byte("l1\n"), 0o644); err != nil {
		t.Fatalf("write seed file: %v", err)
	}
	runGit(t, dir, "add", "chunked.txt")
	runGit(t, dir, "commit", "-m", "chore: seed chunk file")

	cr, err := svc.AddCR("Patch malformed", "reject malformed patch input")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	task, err := svc.AddTask(cr.ID, "feat: reject malformed patch")
	if err != nil {
		t.Fatalf("AddTask() error = %v", err)
	}
	setValidTaskContract(t, svc, cr.ID, task.ID)

	patchPath := filepath.Join(dir, "broken.patch")
	if err := os.WriteFile(patchPath, []byte("not a valid patch\n"), 0o644); err != nil {
		t.Fatalf("write patch file: %v", err)
	}

	_, err = svc.DoneTaskWithCheckpoint(cr.ID, task.ID, DoneTaskOptions{
		Checkpoint: true,
		PatchFile:  patchPath,
	})
	if err == nil {
		t.Fatalf("expected malformed patch failure")
	}
}

func TestListTaskChunksReturnsSortedChunksAndPathFilter(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")

	if err := os.WriteFile(filepath.Join(dir, "alpha.txt"), []byte("a1\na2\na3\na4\na5\na6\na7\na8\n"), 0o644); err != nil {
		t.Fatalf("write alpha file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "beta.txt"), []byte("b1\nb2\n"), 0o644); err != nil {
		t.Fatalf("write beta file: %v", err)
	}
	runGit(t, dir, "add", "alpha.txt", "beta.txt")
	runGit(t, dir, "commit", "-m", "chore: seed files")

	cr, err := svc.AddCR("Chunk list", "show chunk candidates")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	task, err := svc.AddTask(cr.ID, "feat: list chunks")
	if err != nil {
		t.Fatalf("AddTask() error = %v", err)
	}
	setValidTaskContract(t, svc, cr.ID, task.ID)

	if err := os.WriteFile(filepath.Join(dir, "alpha.txt"), []byte("a1\na2-edited\na3\na4\na5\na6\na7-edited\na8\n"), 0o644); err != nil {
		t.Fatalf("write alpha modifications: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "beta.txt"), []byte("b1\nb2-edited\n"), 0o644); err != nil {
		t.Fatalf("write beta modifications: %v", err)
	}

	chunks, err := svc.ListTaskChunks(cr.ID, task.ID, nil)
	if err != nil {
		t.Fatalf("ListTaskChunks() error = %v", err)
	}
	if len(chunks) != 3 {
		t.Fatalf("expected 3 chunks, got %#v", chunks)
	}
	if chunks[0].Path != "alpha.txt" || chunks[1].Path != "alpha.txt" || chunks[2].Path != "beta.txt" {
		t.Fatalf("expected path-sorted chunks, got %#v", chunks)
	}
	for _, chunk := range chunks {
		if chunk.ID == "" {
			t.Fatalf("expected chunk id, got %#v", chunk)
		}
		if strings.TrimSpace(chunk.Preview) == "" {
			t.Fatalf("expected chunk preview, got %#v", chunk)
		}
	}

	filtered, err := svc.ListTaskChunks(cr.ID, task.ID, []string{"beta.txt"})
	if err != nil {
		t.Fatalf("ListTaskChunks(filtered) error = %v", err)
	}
	if len(filtered) != 1 || filtered[0].Path != "beta.txt" {
		t.Fatalf("expected one beta chunk, got %#v", filtered)
	}
}

func TestTaskChunkWorkingTreePatchIsApplyable(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")

	if err := os.WriteFile(filepath.Join(dir, "alpha.txt"), []byte("a1\na2\na3\na4\na5\na6\na7\na8\n"), 0o644); err != nil {
		t.Fatalf("write alpha file: %v", err)
	}
	runGit(t, dir, "add", "alpha.txt")
	runGit(t, dir, "commit", "-m", "chore: seed file")

	cr, err := svc.AddCR("Chunk show", "patch output for one chunk")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	task, err := svc.AddTask(cr.ID, "feat: chunk show")
	if err != nil {
		t.Fatalf("AddTask() error = %v", err)
	}
	setValidTaskContract(t, svc, cr.ID, task.ID)

	if err := os.WriteFile(filepath.Join(dir, "alpha.txt"), []byte("a1\na2-edited\na3\na4\na5\na6\na7-edited\na8\n"), 0o644); err != nil {
		t.Fatalf("write alpha modifications: %v", err)
	}
	chunks, err := svc.ListTaskChunks(cr.ID, task.ID, nil)
	if err != nil {
		t.Fatalf("ListTaskChunks() error = %v", err)
	}
	if len(chunks) < 1 {
		t.Fatalf("expected at least one chunk, got %#v", chunks)
	}

	chunk, patch, err := svc.TaskChunkWorkingTreePatch(cr.ID, task.ID, chunks[0].ID, nil)
	if err != nil {
		t.Fatalf("TaskChunkWorkingTreePatch() error = %v", err)
	}
	if chunk.ID != chunks[0].ID {
		t.Fatalf("expected chunk id %q, got %q", chunks[0].ID, chunk.ID)
	}
	if !strings.Contains(patch, "diff --git") || !strings.Contains(patch, "@@") {
		t.Fatalf("expected full patch output, got %q", patch)
	}

	patchPath := filepath.Join(dir, "single.patch")
	if err := os.WriteFile(patchPath, []byte(patch), 0o644); err != nil {
		t.Fatalf("write patch file: %v", err)
	}
	runGit(t, dir, "apply", "--cached", "--unidiff-zero", "--recount", patchPath)
	staged := runGit(t, dir, "diff", "--cached", "--name-only")
	if !strings.Contains(staged, "alpha.txt") {
		t.Fatalf("expected alpha.txt staged after patch apply, got %q", staged)
	}
}

func TestExportTaskChunkWorkingTreePatchAcrossFiles(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")

	if err := os.WriteFile(filepath.Join(dir, "alpha.txt"), []byte("a1\na2\na3\na4\na5\na6\na7\na8\n"), 0o644); err != nil {
		t.Fatalf("write alpha file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "beta.txt"), []byte("b1\nb2\n"), 0o644); err != nil {
		t.Fatalf("write beta file: %v", err)
	}
	runGit(t, dir, "add", "alpha.txt", "beta.txt")
	runGit(t, dir, "commit", "-m", "chore: seed files")

	cr, err := svc.AddCR("Chunk export", "patch output for multiple chunks")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	task, err := svc.AddTask(cr.ID, "feat: chunk export")
	if err != nil {
		t.Fatalf("AddTask() error = %v", err)
	}
	setValidTaskContract(t, svc, cr.ID, task.ID)

	if err := os.WriteFile(filepath.Join(dir, "alpha.txt"), []byte("a1\na2-edited\na3\na4\na5\na6\na7-edited\na8\n"), 0o644); err != nil {
		t.Fatalf("write alpha modifications: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "beta.txt"), []byte("b1\nb2-edited\n"), 0o644); err != nil {
		t.Fatalf("write beta modifications: %v", err)
	}
	chunks, err := svc.ListTaskChunks(cr.ID, task.ID, nil)
	if err != nil {
		t.Fatalf("ListTaskChunks() error = %v", err)
	}
	var alphaID, betaID string
	for _, chunk := range chunks {
		if chunk.Path == "alpha.txt" && alphaID == "" {
			alphaID = chunk.ID
		}
		if chunk.Path == "beta.txt" && betaID == "" {
			betaID = chunk.ID
		}
	}
	if alphaID == "" || betaID == "" {
		t.Fatalf("expected chunk ids for alpha and beta, got %#v", chunks)
	}

	selected, patch, err := svc.ExportTaskChunkWorkingTreePatch(cr.ID, task.ID, []string{alphaID, betaID}, nil)
	if err != nil {
		t.Fatalf("ExportTaskChunkWorkingTreePatch() error = %v", err)
	}
	if len(selected) != 2 {
		t.Fatalf("expected 2 selected chunks, got %#v", selected)
	}
	if !strings.Contains(patch, "diff --git a/alpha.txt b/alpha.txt") || !strings.Contains(patch, "diff --git a/beta.txt b/beta.txt") {
		t.Fatalf("expected multi-file patch, got %q", patch)
	}

	patchPath := filepath.Join(dir, "selected.patch")
	if err := os.WriteFile(patchPath, []byte(patch), 0o644); err != nil {
		t.Fatalf("write patch file: %v", err)
	}
	runGit(t, dir, "apply", "--cached", "--unidiff-zero", "--recount", patchPath)
	staged := strings.TrimSpace(runGit(t, dir, "diff", "--cached", "--name-only"))
	if staged != "alpha.txt\nbeta.txt" && staged != "beta.txt\nalpha.txt" {
		t.Fatalf("expected alpha.txt and beta.txt staged, got %q", staged)
	}
}

func TestTaskChunkCommandsRejectPreStagedChanges(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")

	if err := os.WriteFile(filepath.Join(dir, "alpha.txt"), []byte("a1\na2\n"), 0o644); err != nil {
		t.Fatalf("write alpha file: %v", err)
	}
	runGit(t, dir, "add", "alpha.txt")
	runGit(t, dir, "commit", "-m", "chore: seed file")

	cr, err := svc.AddCR("Chunk pre-staged", "reject staged changes")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	task, err := svc.AddTask(cr.ID, "feat: pre-staged check")
	if err != nil {
		t.Fatalf("AddTask() error = %v", err)
	}
	setValidTaskContract(t, svc, cr.ID, task.ID)

	if err := os.WriteFile(filepath.Join(dir, "alpha.txt"), []byte("a1\na2-edited\n"), 0o644); err != nil {
		t.Fatalf("write alpha modifications: %v", err)
	}
	runGit(t, dir, "add", "alpha.txt")

	if _, err := svc.ListTaskChunks(cr.ID, task.ID, nil); !errors.Is(err, ErrPreStagedChanges) {
		t.Fatalf("expected ErrPreStagedChanges from ListTaskChunks, got %v", err)
	}
	if _, _, err := svc.TaskChunkWorkingTreePatch(cr.ID, task.ID, "chk_missing", nil); !errors.Is(err, ErrPreStagedChanges) {
		t.Fatalf("expected ErrPreStagedChanges from TaskChunkWorkingTreePatch, got %v", err)
	}
	if _, _, err := svc.ExportTaskChunkWorkingTreePatch(cr.ID, task.ID, []string{"chk_missing"}, nil); !errors.Is(err, ErrPreStagedChanges) {
		t.Fatalf("expected ErrPreStagedChanges from ExportTaskChunkWorkingTreePatch, got %v", err)
	}
}

func TestDoneTaskWithCheckpointRequiresExplicitScope(t *testing.T) {
	cr := seedCR(1, "Scope required", seedCROptions{
		Description: "checkpoint scope required",
		Branch:      "cr-1-runtime",
	})
	cr.Subtasks = []model.Subtask{seedTask(1, "feat: scope required", model.TaskStatusOpen, "")}
	h := harnessService(t, runtimeHarnessOptions{Branch: cr.Branch, CRs: []*model.CR{cr}})

	_, err := h.Service.doneTaskWithCheckpointUnlocked(cr.ID, 1, DoneTaskOptions{Checkpoint: true})
	if !errors.Is(err, ErrTaskScopeRequired) {
		t.Fatalf("expected ErrTaskScopeRequired, got %v", err)
	}
}

func TestDoneTaskWithCheckpointRejectsInvalidScopePaths(t *testing.T) {
	cr := seedCR(1, "Invalid scope", seedCROptions{
		Description: "reject invalid paths",
		Branch:      "cr-1-runtime",
	})
	task := seedTask(1, "feat: validate scope", model.TaskStatusOpen, "")
	task.Contract = model.TaskContract{
		Intent:             "Validate checkpoint scope arguments.",
		AcceptanceCriteria: []string{"Invalid scope options fail fast."},
		Scope:              []string{"internal/service"},
	}
	cr.Subtasks = []model.Subtask{task}
	h := harnessService(t, runtimeHarnessOptions{Branch: cr.Branch, CRs: []*model.CR{cr}})

	cases := []DoneTaskOptions{
		{Checkpoint: true, Paths: []string{""}},
		{Checkpoint: true, Paths: []string{"/tmp/a.txt"}},
		{Checkpoint: true, Paths: []string{"../escape.txt"}},
		{Checkpoint: true, Paths: []string{"a/../b.txt"}},
		{Checkpoint: true, Paths: []string{"*.go"}},
		{Checkpoint: true, Paths: []string{"dup.txt", "dup.txt"}},
		{Checkpoint: true, StageAll: true, FromContract: true},
		{Checkpoint: true, FromContract: true, Paths: []string{"x.txt"}},
		{Checkpoint: true, StageAll: true, PatchFile: "task.patch"},
		{Checkpoint: true, FromContract: true, PatchFile: "task.patch"},
		{Checkpoint: true, Paths: []string{"x.txt"}, PatchFile: "task.patch"},
		{Checkpoint: false, Paths: []string{"x.txt"}},
		{Checkpoint: false, StageAll: true},
		{Checkpoint: false, FromContract: true},
		{Checkpoint: false, PatchFile: "task.patch"},
	}

	for _, tc := range cases {
		_, gotErr := h.Service.doneTaskWithCheckpointUnlocked(cr.ID, 1, tc)
		if !errors.Is(gotErr, ErrInvalidTaskScope) {
			t.Fatalf("expected ErrInvalidTaskScope for options %#v, got %v", tc, gotErr)
		}
	}
}

func TestDoneTaskWithCheckpointRejectsPreStagedChanges(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")

	cr, err := svc.AddCR("Pre-staged guard", "fail on existing staged changes")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	task, err := svc.AddTask(cr.ID, "feat: pre-staged guard")
	if err != nil {
		t.Fatalf("AddTask() error = %v", err)
	}
	setValidTaskContract(t, svc, cr.ID, task.ID)

	if err := os.WriteFile(filepath.Join(dir, "already-staged.txt"), []byte("staged\n"), 0o644); err != nil {
		t.Fatalf("write already-staged file: %v", err)
	}
	runGit(t, dir, "add", "already-staged.txt")
	if err := os.WriteFile(filepath.Join(dir, "scoped.txt"), []byte("scoped\n"), 0o644); err != nil {
		t.Fatalf("write scoped file: %v", err)
	}

	_, err = svc.DoneTaskWithCheckpoint(cr.ID, task.ID, DoneTaskOptions{
		Checkpoint: true,
		Paths:      []string{"scoped.txt"},
	})
	if !errors.Is(err, ErrPreStagedChanges) {
		t.Fatalf("expected ErrPreStagedChanges, got %v", err)
	}

	loaded, err := svc.store.LoadCR(cr.ID)
	if err != nil {
		t.Fatalf("LoadCR() error = %v", err)
	}
	if loaded.Subtasks[0].Status != model.TaskStatusOpen {
		t.Fatalf("expected task to remain open, got %q", loaded.Subtasks[0].Status)
	}
}

func TestDoneTaskWithCheckpointScopedPathWithoutChangesFails(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	cr, err := svc.AddCR("No scoped changes", "scope has no changes")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	task, err := svc.AddTask(cr.ID, "feat: scoped no changes")
	if err != nil {
		t.Fatalf("AddTask() error = %v", err)
	}
	setValidTaskContract(t, svc, cr.ID, task.ID)
	if err := os.WriteFile(filepath.Join(dir, "other.txt"), []byte("other\n"), 0o644); err != nil {
		t.Fatalf("write other file: %v", err)
	}

	_, err = svc.DoneTaskWithCheckpoint(cr.ID, task.ID, DoneTaskOptions{
		Checkpoint: true,
		Paths:      []string{"target.txt"},
	})
	if !errors.Is(err, ErrNoTaskChanges) {
		t.Fatalf("expected ErrNoTaskChanges, got %v", err)
	}
}
