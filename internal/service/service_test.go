package service

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitInNonGitDirectoryInitializesGitAndSophia(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)

	base, err := svc.Init("main")
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
	if _, err := svc.Init("main"); err != nil {
		t.Fatalf("first Init() error = %v", err)
	}
	if _, err := svc.Init("main"); err != nil {
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

func TestAddCRCreatesBranchAndCRFile(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main"); err != nil {
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
}

func TestNoteAppendsAndUpdatesCR(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main"); err != nil {
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
	if _, err := svc.Init("main"); err != nil {
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

func TestReviewShowsChangedFilesAndShortStat(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main"); err != nil {
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
	if _, err := svc.Init("main"); err != nil {
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

	sha, err := svc.MergeCR(cr.ID, false)
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
	for _, footer := range []string{"Sophia-CR: 1", "Sophia-Intent: Bootstrap", "Sophia-Tasks: 0 completed"} {
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
}

func TestActorFallbackIsUnknownWhenGitIdentityMissing(t *testing.T) {
	dir := t.TempDir()
	home := t.TempDir()
	configHome := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", configHome)

	svc := New(dir)
	if _, err := svc.Init("main"); err != nil {
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
