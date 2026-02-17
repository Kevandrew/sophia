package service

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"sophia/internal/model"
)

func TestInitInNonGitDirectoryInitializesGitAndSophia(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)

	base, err := svc.Init("main", "")
	if err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if base != "main" {
		t.Fatalf("expected base branch main, got %q", base)
	}

	if _, err := os.Stat(filepath.Join(dir, ".git")); err != nil {
		t.Fatalf("expected .git to exist: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".sophia", "config.yaml")); err != nil {
		t.Fatalf("expected .sophia/config.yaml to exist: %v", err)
	}
}

func TestInitIsIdempotentInExistingRepo(t *testing.T) {
	dir := t.TempDir()
	runGit(t, dir, "init")

	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("first Init() error = %v", err)
	}
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("second Init() error = %v", err)
	}

	idx, err := svc.store.LoadIndex()
	if err != nil {
		t.Fatalf("LoadIndex() error = %v", err)
	}
	if idx.NextID != 1 {
		t.Fatalf("expected next id 1 after idempotent init, got %d", idx.NextID)
	}
}

func TestInitDefaultsToLocalMetadataAndGitIgnoreEntry(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)

	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	cfg, err := svc.store.LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if cfg.MetadataMode != "local" {
		t.Fatalf("expected metadata_mode local, got %q", cfg.MetadataMode)
	}
	gitignore, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
	if err != nil {
		t.Fatalf("read .gitignore: %v", err)
	}
	if !strings.Contains(string(gitignore), ".sophia/") {
		t.Fatalf("expected .gitignore to include .sophia/")
	}

	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("second Init() error = %v", err)
	}
	gitignore2, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
	if err != nil {
		t.Fatalf("read .gitignore after second init: %v", err)
	}
	if strings.Count(string(gitignore2), ".sophia/") != 1 {
		t.Fatalf("expected single .sophia/ entry, got:\n%s", string(gitignore2))
	}
}

func TestAddCRAlignsNextIDWithHistory(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")

	// Simulate existing merged CR history in Git while local index is stale.
	runGit(t, dir, "commit", "--allow-empty", "-m", "[CR-4] Existing merged intent", "-m", "Sophia-CR: 4\nSophia-Intent: Existing merged intent\nSophia-Tasks: 0 completed")
	if err := svc.store.SaveIndex(model.Index{NextID: 1}); err != nil {
		t.Fatalf("SaveIndex() error = %v", err)
	}

	cr, err := svc.AddCR("New intent", "should pick id 5")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	if cr.ID != 5 {
		t.Fatalf("expected CR id 5, got %d", cr.ID)
	}
	if cr.Branch != "sophia/cr-5" {
		t.Fatalf("expected branch sophia/cr-5, got %q", cr.Branch)
	}
}

func TestAddCRCreatesBranchAndCRFile(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	cr, err := svc.AddCR("Bootstrap", "Scaffold CLI")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	if cr.ID != 1 {
		t.Fatalf("expected CR id 1, got %d", cr.ID)
	}
	if cr.Branch != "sophia/cr-1" {
		t.Fatalf("unexpected branch %q", cr.Branch)
	}
	if strings.TrimSpace(cr.UID) == "" {
		t.Fatalf("expected CR uid to be assigned, got %#v", cr)
	}
	if cr.BaseRef != "main" {
		t.Fatalf("expected base ref main, got %q", cr.BaseRef)
	}
	if strings.TrimSpace(cr.BaseCommit) == "" {
		t.Fatalf("expected base commit to be assigned, got %#v", cr)
	}

	branch, err := svc.git.CurrentBranch()
	if err != nil {
		t.Fatalf("CurrentBranch() error = %v", err)
	}
	if branch != cr.Branch {
		t.Fatalf("expected current branch %q, got %q", cr.Branch, branch)
	}

	loaded, err := svc.store.LoadCR(1)
	if err != nil {
		t.Fatalf("LoadCR() error = %v", err)
	}
	if loaded.Title != "Bootstrap" || len(loaded.Events) == 0 || loaded.Events[0].Type != "cr_created" {
		t.Fatalf("unexpected loaded CR: %#v", loaded)
	}
	if loaded.UID != cr.UID {
		t.Fatalf("expected persisted uid %q, got %q", cr.UID, loaded.UID)
	}
	if loaded.BaseRef != cr.BaseRef || loaded.BaseCommit != cr.BaseCommit {
		t.Fatalf("expected persisted base fields, got %#v", loaded)
	}
}

func TestAddCRAssignsDistinctUIDs(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	first, err := svc.AddCR("First", "uid one")
	if err != nil {
		t.Fatalf("AddCR(first) error = %v", err)
	}
	second, err := svc.AddCR("Second", "uid two")
	if err != nil {
		t.Fatalf("AddCR(second) error = %v", err)
	}

	if strings.TrimSpace(first.UID) == "" || strings.TrimSpace(second.UID) == "" {
		t.Fatalf("expected non-empty uids, got first=%q second=%q", first.UID, second.UID)
	}
	if first.UID == second.UID {
		t.Fatalf("expected distinct uids, got %q", first.UID)
	}
}

func TestAddCRWithExplicitBaseRef(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")
	runGit(t, dir, "checkout", "-b", "release")
	if err := os.WriteFile(filepath.Join(dir, "release_base.txt"), []byte("release\n"), 0o644); err != nil {
		t.Fatalf("write release base file: %v", err)
	}
	runGit(t, dir, "add", "release_base.txt")
	runGit(t, dir, "commit", "-m", "feat: release base")
	runGit(t, dir, "checkout", "-B", "main")

	cr, _, err := svc.AddCRWithOptionsWithWarnings("Release-based", "base ref", AddCROptions{BaseRef: "release"})
	if err != nil {
		t.Fatalf("AddCRWithOptionsWithWarnings() error = %v", err)
	}
	if cr.BaseRef != "release" {
		t.Fatalf("expected base ref release, got %q", cr.BaseRef)
	}
	releaseHead, err := svc.git.ResolveRef("release")
	if err != nil {
		t.Fatalf("ResolveRef(release) error = %v", err)
	}
	if cr.BaseCommit != releaseHead {
		t.Fatalf("expected base commit %q, got %q", releaseHead, cr.BaseCommit)
	}
	if _, statErr := os.Stat(filepath.Join(dir, "release_base.txt")); statErr != nil {
		t.Fatalf("expected CR branch from release base to contain file: %v", statErr)
	}
}

func TestAddChildCRUsesParentAnchor(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")

	parent, err := svc.AddCR("Parent", "base for child")
	if err != nil {
		t.Fatalf("AddCR(parent) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "parent.txt"), []byte("parent\n"), 0o644); err != nil {
		t.Fatalf("write parent file: %v", err)
	}
	runGit(t, dir, "add", "parent.txt")
	runGit(t, dir, "commit", "-m", "feat: parent work")
	parentHead, err := svc.git.ResolveRef(parent.Branch)
	if err != nil {
		t.Fatalf("ResolveRef(parent branch) error = %v", err)
	}

	child, _, err := svc.AddCRWithOptionsWithWarnings("Child", "stacked", AddCROptions{ParentCRID: parent.ID})
	if err != nil {
		t.Fatalf("AddCRWithOptionsWithWarnings(child) error = %v", err)
	}
	if child.ParentCRID != parent.ID {
		t.Fatalf("expected parent id %d, got %d", parent.ID, child.ParentCRID)
	}
	if child.BaseRef != parent.Branch {
		t.Fatalf("expected child base_ref %q, got %q", parent.Branch, child.BaseRef)
	}
	if child.BaseCommit != parentHead {
		t.Fatalf("expected child base_commit %q, got %q", parentHead, child.BaseCommit)
	}
}

func TestMergeChildBlockedUntilParentMergedAndParentMergeBackfillsChildBase(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")

	parent, err := svc.AddCR("Parent merge", "must merge first")
	if err != nil {
		t.Fatalf("AddCR(parent) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "parent_merge.txt"), []byte("parent\n"), 0o644); err != nil {
		t.Fatalf("write parent file: %v", err)
	}
	runGit(t, dir, "add", "parent_merge.txt")
	runGit(t, dir, "commit", "-m", "feat: parent merge")
	setValidContract(t, svc, parent.ID)

	child, _, err := svc.AddCRWithOptionsWithWarnings("Child merge", "depends on parent", AddCROptions{ParentCRID: parent.ID})
	if err != nil {
		t.Fatalf("AddCRWithOptionsWithWarnings(child) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "child_merge.txt"), []byte("child\n"), 0o644); err != nil {
		t.Fatalf("write child file: %v", err)
	}
	runGit(t, dir, "add", "child_merge.txt")
	runGit(t, dir, "commit", "-m", "feat: child merge")
	setValidContract(t, svc, child.ID)

	if _, err := svc.MergeCR(child.ID, false, ""); !errors.Is(err, ErrParentCRNotMerged) {
		t.Fatalf("expected ErrParentCRNotMerged, got %v", err)
	}

	if _, err := svc.SwitchCR(parent.ID); err != nil {
		t.Fatalf("SwitchCR(parent) error = %v", err)
	}
	if _, err := svc.MergeCR(parent.ID, false, ""); err != nil {
		t.Fatalf("MergeCR(parent) error = %v", err)
	}

	updatedChild, err := svc.store.LoadCR(child.ID)
	if err != nil {
		t.Fatalf("LoadCR(child) error = %v", err)
	}
	if updatedChild.BaseRef != updatedChild.BaseBranch {
		t.Fatalf("expected child base_ref to reset to base branch, got %q", updatedChild.BaseRef)
	}
	if strings.TrimSpace(updatedChild.BaseCommit) == "" {
		t.Fatalf("expected child base_commit backfilled from parent merge")
	}
}

func TestSetCRBaseAndRestack(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")

	runGit(t, dir, "checkout", "-b", "release")
	if err := os.WriteFile(filepath.Join(dir, "release_stack.txt"), []byte("release\n"), 0o644); err != nil {
		t.Fatalf("write release stack file: %v", err)
	}
	runGit(t, dir, "add", "release_stack.txt")
	runGit(t, dir, "commit", "-m", "feat: release stack")
	runGit(t, dir, "checkout", "-B", "main")

	cr, err := svc.AddCR("Base set", "retarget base")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	updated, err := svc.SetCRBase(cr.ID, "release", false)
	if err != nil {
		t.Fatalf("SetCRBase() error = %v", err)
	}
	if updated.BaseRef != "release" || strings.TrimSpace(updated.BaseCommit) == "" {
		t.Fatalf("unexpected SetCRBase result %#v", updated)
	}

	parent, err := svc.AddCR("Restack parent", "parent")
	if err != nil {
		t.Fatalf("AddCR(parent) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "restack_parent.txt"), []byte("p1\n"), 0o644); err != nil {
		t.Fatalf("write restack parent file: %v", err)
	}
	runGit(t, dir, "add", "restack_parent.txt")
	runGit(t, dir, "commit", "-m", "feat: restack parent 1")

	child, _, err := svc.AddCRWithOptionsWithWarnings("Restack child", "child", AddCROptions{ParentCRID: parent.ID})
	if err != nil {
		t.Fatalf("AddCRWithOptionsWithWarnings(child) error = %v", err)
	}
	if _, err := svc.SwitchCR(parent.ID); err != nil {
		t.Fatalf("SwitchCR(parent) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "restack_parent_2.txt"), []byte("p2\n"), 0o644); err != nil {
		t.Fatalf("write restack parent second file: %v", err)
	}
	runGit(t, dir, "add", "restack_parent_2.txt")
	runGit(t, dir, "commit", "-m", "feat: restack parent 2")

	if _, err := svc.RestackCR(child.ID); err != nil {
		t.Fatalf("RestackCR() error = %v", err)
	}
	reloadedChild, err := svc.store.LoadCR(child.ID)
	if err != nil {
		t.Fatalf("LoadCR(child) error = %v", err)
	}
	parentHead, err := svc.git.ResolveRef(parent.Branch)
	if err != nil {
		t.Fatalf("ResolveRef(parent branch) error = %v", err)
	}
	if reloadedChild.BaseRef != parent.Branch || reloadedChild.BaseCommit != parentHead {
		t.Fatalf("expected child restacked onto parent head, got %#v", reloadedChild)
	}
}

func TestNoteAppendsAndUpdatesCR(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if _, err := svc.AddCR("Bootstrap", "Scaffold CLI"); err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}

	if err := svc.AddNote(1, "Refactored payment client"); err != nil {
		t.Fatalf("AddNote() error = %v", err)
	}

	cr, err := svc.store.LoadCR(1)
	if err != nil {
		t.Fatalf("LoadCR() error = %v", err)
	}
	if len(cr.Notes) != 1 || cr.Notes[0] != "Refactored payment client" {
		t.Fatalf("unexpected notes: %#v", cr.Notes)
	}
	if got := cr.Events[len(cr.Events)-1].Type; got != "note_added" {
		t.Fatalf("expected last event note_added, got %q", got)
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
	if got := cr.Events[len(cr.Events)-1].Type; got != "task_done" {
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
	if lastTwo[0].Type != "task_checkpointed" || lastTwo[1].Type != "task_done" {
		t.Fatalf("expected checkpoint then done events, got %#v", lastTwo)
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
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	cr, err := svc.AddCR("Metadata only", "done without commit")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	task, err := svc.AddTask(cr.ID, "docs: update note")
	if err != nil {
		t.Fatalf("AddTask() error = %v", err)
	}
	setValidTaskContract(t, svc, cr.ID, task.ID)

	sha, err := svc.DoneTaskWithCheckpoint(cr.ID, task.ID, DoneTaskOptions{Checkpoint: false})
	if err != nil {
		t.Fatalf("DoneTaskWithCheckpoint(checkpoint=false) error = %v", err)
	}
	if sha != "" {
		t.Fatalf("expected empty sha for metadata-only completion, got %q", sha)
	}
	loaded, err := svc.store.LoadCR(cr.ID)
	if err != nil {
		t.Fatalf("LoadCR() error = %v", err)
	}
	if loaded.Subtasks[0].Status != model.TaskStatusDone {
		t.Fatalf("expected task done, got %q", loaded.Subtasks[0].Status)
	}
	if loaded.Subtasks[0].CheckpointCommit != "" {
		t.Fatalf("expected no checkpoint commit metadata, got %#v", loaded.Subtasks[0])
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
	if lastTwo[0].Type != "task_checkpointed" {
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

func TestDoneTaskWithCheckpointRequiresExplicitScope(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	cr, err := svc.AddCR("Scope required", "checkpoint scope required")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	task, err := svc.AddTask(cr.ID, "feat: scope required")
	if err != nil {
		t.Fatalf("AddTask() error = %v", err)
	}
	setValidTaskContract(t, svc, cr.ID, task.ID)
	if err := os.WriteFile(filepath.Join(dir, "file.txt"), []byte("x\n"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	_, err = svc.DoneTaskWithCheckpoint(cr.ID, task.ID, DoneTaskOptions{Checkpoint: true})
	if !errors.Is(err, ErrTaskScopeRequired) {
		t.Fatalf("expected ErrTaskScopeRequired, got %v", err)
	}
}

func TestDoneTaskWithCheckpointRejectsInvalidScopePaths(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	cr, err := svc.AddCR("Invalid scope", "reject invalid paths")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	task, err := svc.AddTask(cr.ID, "feat: validate scope")
	if err != nil {
		t.Fatalf("AddTask() error = %v", err)
	}
	setValidTaskContract(t, svc, cr.ID, task.ID)

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
		_, gotErr := svc.DoneTaskWithCheckpoint(cr.ID, task.ID, tc)
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

func TestReviewShowsChangedFilesAndShortStat(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if _, err := svc.AddCR("Bootstrap", "Scaffold CLI"); err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}

	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")

	if err := os.WriteFile(filepath.Join(dir, "feature.txt"), []byte("hello\n"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	runGit(t, dir, "add", "feature.txt")
	runGit(t, dir, "commit", "-m", "feat: add feature")

	review, err := svc.ReviewCR(1)
	if err != nil {
		t.Fatalf("ReviewCR() error = %v", err)
	}
	if len(review.Files) != 1 || review.Files[0] != "feature.txt" {
		t.Fatalf("unexpected files: %#v", review.Files)
	}
	if len(review.NewFiles) != 1 || review.NewFiles[0] != "feature.txt" {
		t.Fatalf("unexpected new files: %#v", review.NewFiles)
	}
	if len(review.ModifiedFiles) != 0 || len(review.DeletedFiles) != 0 {
		t.Fatalf("unexpected modified/deleted categorization: modified=%#v deleted=%#v", review.ModifiedFiles, review.DeletedFiles)
	}
	if !strings.Contains(review.ShortStat, "1 file changed") {
		t.Fatalf("expected shortstat to include file count, got %q", review.ShortStat)
	}
}

func TestMergeCreatesIntentCommitAndMarksMerged(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	cr, err := svc.AddCR("Bootstrap", "Scaffold CLI")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}

	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")

	if _, err := svc.AddTask(cr.ID, "Implement command tree"); err != nil {
		t.Fatalf("AddTask() error = %v", err)
	}
	if err := svc.AddNote(cr.ID, "Added root and cr commands"); err != nil {
		t.Fatalf("AddNote() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "app.txt"), []byte("content\n"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	runGit(t, dir, "add", "app.txt")
	runGit(t, dir, "commit", "-m", "feat: app")
	setValidContract(t, svc, cr.ID)

	sha, err := svc.MergeCR(cr.ID, false, "")
	if err != nil {
		t.Fatalf("MergeCR() error = %v", err)
	}
	if sha == "" {
		t.Fatalf("expected merge sha to be non-empty")
	}

	mergedCR, err := svc.store.LoadCR(cr.ID)
	if err != nil {
		t.Fatalf("LoadCR() error = %v", err)
	}
	if mergedCR.Status != "merged" {
		t.Fatalf("expected status merged, got %q", mergedCR.Status)
	}

	msg := runGit(t, dir, "log", "-1", "--pretty=%B")
	if !strings.Contains(msg, "[CR-1] Bootstrap") {
		t.Fatalf("missing CR subject in commit message: %q", msg)
	}
	for _, section := range []string{"Intent:", "Subtasks:", "Notes:", "Metadata:"} {
		if !strings.Contains(msg, section) {
			t.Fatalf("expected section %q in commit message: %q", section, msg)
		}
	}
	for _, footer := range []string{"Sophia-CR: 1", "Sophia-CR-UID: " + cr.UID, "Sophia-Base-Ref: " + cr.BaseRef, "Sophia-Base-Commit: " + cr.BaseCommit, "Sophia-Intent: Bootstrap", "Sophia-Tasks: 0 completed"} {
		if !strings.Contains(msg, footer) {
			t.Fatalf("expected footer %q in commit message: %q", footer, msg)
		}
	}
	if mergedCR.MergedAt == "" || mergedCR.MergedBy == "" || mergedCR.MergedCommit == "" {
		t.Fatalf("expected merged metadata to be persisted, got %#v", mergedCR)
	}
	if mergedCR.FilesTouchedCount != 1 {
		t.Fatalf("expected files_touched_count=1, got %d", mergedCR.FilesTouchedCount)
	}
	if svc.git.BranchExists(cr.Branch) {
		t.Fatalf("expected branch %q to be deleted by default merge", cr.Branch)
	}
}

func TestMergeKeepBranchPreservesCRBranch(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")

	cr, err := svc.AddCR("Keep branch", "preserve branch")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "keep.txt"), []byte("x\n"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	runGit(t, dir, "add", "keep.txt")
	runGit(t, dir, "commit", "-m", "feat: keep branch")
	setValidContract(t, svc, cr.ID)

	if _, err := svc.MergeCR(cr.ID, true, ""); err != nil {
		t.Fatalf("MergeCR(keepBranch=true) error = %v", err)
	}
	if !svc.git.BranchExists(cr.Branch) {
		t.Fatalf("expected branch %q to remain after keep-branch merge", cr.Branch)
	}
}

func TestActorFallbackIsUnknownWhenGitIdentityMissing(t *testing.T) {
	dir := t.TempDir()
	home := t.TempDir()
	configHome := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", configHome)

	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if _, err := svc.AddCR("Bootstrap", "Scaffold CLI"); err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}

	cr, err := svc.store.LoadCR(1)
	if err != nil {
		t.Fatalf("LoadCR() error = %v", err)
	}
	if len(cr.Events) == 0 {
		t.Fatalf("expected at least one event")
	}
	if cr.Events[0].Actor != "unknown" {
		t.Fatalf("expected actor unknown, got %q", cr.Events[0].Actor)
	}
}

func firstHunkPatchFromDiff(t *testing.T, diff string) string {
	t.Helper()
	diff = strings.TrimSpace(diff)
	if diff == "" {
		t.Fatalf("expected non-empty diff")
	}
	lines := strings.Split(diff, "\n")
	out := make([]string, 0, len(lines))
	hunks := 0
	for _, line := range lines {
		if strings.HasPrefix(line, "@@ ") {
			hunks++
			if hunks > 1 {
				break
			}
		}
		out = append(out, line)
	}
	if hunks == 0 {
		t.Fatalf("expected at least one hunk in diff: %q", diff)
	}
	return strings.Join(out, "\n") + "\n"
}

func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, string(out))
	}
	return strings.TrimSpace(string(out))
}
