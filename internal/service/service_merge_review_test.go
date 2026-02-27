package service

import (
	"os"
	"path/filepath"
	"sophia/internal/model"
	"strings"
	"testing"
)

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

func TestMergeStatusCRUsesRuntimeProviders(t *testing.T) {
	cr := seedCR(1, "runtime merge status", seedCROptions{Branch: "cr-1-runtime"})
	h := harnessService(t, runtimeHarnessOptions{Branch: cr.Branch, CRs: []*model.CR{cr}})

	h.MergeGit.mergeInProg = true
	h.MergeGit.mergeHeadSHA = "cr-head-sha"
	h.MergeGit.resolve[cr.Branch] = "cr-head-sha"
	h.MergeGit.mergeFiles = []string{"conflict.txt"}

	status, err := h.Service.MergeStatusCR(cr.ID)
	if err != nil {
		t.Fatalf("MergeStatusCR() error = %v", err)
	}
	if !status.InProgress {
		t.Fatalf("expected in-progress merge status")
	}
	if !status.TargetMatches {
		t.Fatalf("expected target branch head to match merge head")
	}
	if status.MergeHead != "cr-head-sha" {
		t.Fatalf("expected merge head cr-head-sha, got %q", status.MergeHead)
	}
	if len(status.ConflictFiles) != 1 || status.ConflictFiles[0] != "conflict.txt" {
		t.Fatalf("unexpected conflict files: %#v", status.ConflictFiles)
	}

	if got := h.Store.Calls("LoadCR"); got < 1 {
		t.Fatalf("expected merge status to load CR from runtime store, got %d calls", got)
	}
	if got := h.MergeGit.Calls("IsMergeInProgress"); got < 1 {
		t.Fatalf("expected merge status to query runtime git in-progress state, got %d calls", got)
	}
	if got := h.MergeGit.Calls("MergeHeadSHA"); got < 1 {
		t.Fatalf("expected merge status to query runtime git merge head, got %d calls", got)
	}
	if got := h.MergeGit.Calls("MergeConflictFiles"); got < 1 {
		t.Fatalf("expected merge status to query runtime git conflict files, got %d calls", got)
	}
	if got := h.MergeGit.Calls("ResolveRef"); got < 1 {
		t.Fatalf("expected merge status to resolve CR branch head via runtime git, got %d calls", got)
	}
}
