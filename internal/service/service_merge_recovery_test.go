package service

import (
	"errors"
	"os"
	"path/filepath"
	"sophia/internal/model"
	"testing"
)

// Integration coverage: merge recovery depends on real git conflict state transitions.
func TestMergeConflictReturnsStructuredErrorAndStatus(t *testing.T) {
	t.Parallel()
	svc, cr, _ := setupMergeConflictScenario(t)

	_, _, err := svc.MergeCRWithWarnings(cr.ID, false, "")
	if err == nil {
		t.Fatalf("expected merge conflict error")
	}
	if !errors.Is(err, ErrMergeConflict) {
		t.Fatalf("expected ErrMergeConflict, got %v", err)
	}
	var conflictErr *MergeConflictError
	if !errors.As(err, &conflictErr) {
		t.Fatalf("expected *MergeConflictError, got %T", err)
	}
	if conflictErr.CRID != cr.ID {
		t.Fatalf("expected conflict CRID %d, got %d", cr.ID, conflictErr.CRID)
	}
	if len(conflictErr.ConflictFiles) == 0 || conflictErr.ConflictFiles[0] != "conflict.txt" {
		t.Fatalf("expected conflict files to include conflict.txt, got %#v", conflictErr.ConflictFiles)
	}
	if conflictErr.WorktreePath == "" {
		t.Fatalf("expected worktree path in conflict error")
	}

	status, statusErr := svc.MergeStatusCR(cr.ID)
	if statusErr != nil {
		t.Fatalf("MergeStatusCR() error = %v", statusErr)
	}
	if !status.InProgress {
		t.Fatalf("expected merge status in progress")
	}
	if !status.TargetMatches {
		t.Fatalf("expected merge target match for CR %d", cr.ID)
	}
	if len(status.ConflictFiles) == 0 || status.ConflictFiles[0] != "conflict.txt" {
		t.Fatalf("expected status conflict files to include conflict.txt, got %#v", status.ConflictFiles)
	}

	loaded, loadErr := svc.store.LoadCR(cr.ID)
	if loadErr != nil {
		t.Fatalf("LoadCR() error = %v", loadErr)
	}
	foundConflictEvent := false
	for _, event := range loaded.Events {
		if event.Type == model.EventTypeCRMergeConflict {
			foundConflictEvent = true
			break
		}
	}
	if !foundConflictEvent {
		t.Fatalf("expected cr_merge_conflict event")
	}

}

func TestAbortMergeClearsStateAndRecordsEvent(t *testing.T) {
	t.Parallel()
	svc, cr, _ := setupMergeConflictScenario(t)

	if _, _, err := svc.MergeCRWithWarnings(cr.ID, false, ""); err == nil {
		t.Fatalf("expected merge conflict")
	}
	if err := svc.AbortMergeCR(cr.ID); err != nil {
		t.Fatalf("AbortMergeCR() error = %v", err)
	}
	status, err := svc.MergeStatusCR(cr.ID)
	if err != nil {
		t.Fatalf("MergeStatusCR() error = %v", err)
	}
	if status.InProgress {
		t.Fatalf("expected merge state cleared after abort")
	}

	loaded, loadErr := svc.store.LoadCR(cr.ID)
	if loadErr != nil {
		t.Fatalf("LoadCR() error = %v", loadErr)
	}
	foundAbortEvent := false
	for _, event := range loaded.Events {
		if event.Type == model.EventTypeCRMergeAborted {
			foundAbortEvent = true
			break
		}
	}
	if !foundAbortEvent {
		t.Fatalf("expected cr_merge_aborted event")
	}
}

func TestResumeMergeFinalizesAfterManualResolution(t *testing.T) {
	t.Parallel()
	svc, cr, dir := setupMergeConflictScenario(t)

	if _, _, err := svc.MergeCRWithWarnings(cr.ID, false, ""); err == nil {
		t.Fatalf("expected merge conflict")
	}

	if err := os.WriteFile(filepath.Join(dir, "conflict.txt"), []byte("resolved\n"), 0o644); err != nil {
		t.Fatalf("write resolved file: %v", err)
	}
	runGit(t, dir, "add", "conflict.txt")

	result, err := svc.ResumeMergeCRWithOptions(cr.ID, MergeCROptions{KeepBranch: false})
	if err != nil {
		t.Fatalf("ResumeMergeCRWithOptions() error = %v", err)
	}
	sha := result.MergedCommit
	warnings := result.Warnings
	if sha == "" {
		t.Fatalf("expected resumed merge sha")
	}
	if len(warnings) != 0 {
		t.Fatalf("unexpected resume warnings: %#v", warnings)
	}

	merged, loadErr := svc.store.LoadCR(cr.ID)
	if loadErr != nil {
		t.Fatalf("LoadCR() error = %v", loadErr)
	}
	if merged.Status != "merged" {
		t.Fatalf("expected merged status, got %q", merged.Status)
	}
	foundResumedEvent := false
	foundMergedEvent := false
	for _, event := range merged.Events {
		if event.Type == model.EventTypeCRMergeResumed {
			foundResumedEvent = true
		}
		if event.Type == model.EventTypeCRMerged {
			foundMergedEvent = true
		}
	}
	if !foundResumedEvent {
		t.Fatalf("expected cr_merge_resumed event")
	}
	if !foundMergedEvent {
		t.Fatalf("expected cr_merged event")
	}
}

func TestMutatingCommandsBlockedDuringMergeInProgress(t *testing.T) {
	t.Parallel()
	svc, cr, _ := setupMergeConflictScenario(t)

	if _, _, err := svc.MergeCRWithWarnings(cr.ID, false, ""); err == nil {
		t.Fatalf("expected merge conflict")
	}

	err := svc.AddNote(cr.ID, "should be blocked")
	if err == nil {
		t.Fatalf("expected merge in progress error")
	}
	if !errors.Is(err, ErrMergeInProgress) {
		t.Fatalf("expected ErrMergeInProgress, got %v", err)
	}
	var inProgressErr *MergeInProgressError
	if !errors.As(err, &inProgressErr) {
		t.Fatalf("expected *MergeInProgressError, got %T", err)
	}
	if len(inProgressErr.ConflictFiles) == 0 || inProgressErr.ConflictFiles[0] != "conflict.txt" {
		t.Fatalf("expected conflict files in merge-in-progress error, got %#v", inProgressErr.ConflictFiles)
	}
}

func TestAbortAndResumeFailWhenNoMergeInProgress(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")

	cr, err := svc.AddCR("No merge in progress", "abort/resume failure path")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	setValidContract(t, svc, cr.ID)

	if err := svc.AbortMergeCR(cr.ID); !errors.Is(err, ErrNoMergeInProgress) {
		t.Fatalf("expected ErrNoMergeInProgress from abort, got %v", err)
	}
	if _, _, err := svc.ResumeMergeCR(cr.ID, false, ""); !errors.Is(err, ErrNoMergeInProgress) {
		t.Fatalf("expected ErrNoMergeInProgress from resume, got %v", err)
	}
}

func setupMergeConflictScenario(t *testing.T) (*Service, *model.CR, string) {
	t.Helper()

	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")

	if err := os.WriteFile(filepath.Join(dir, "conflict.txt"), []byte("base\n"), 0o644); err != nil {
		t.Fatalf("write base file: %v", err)
	}
	runGit(t, dir, "add", "conflict.txt")
	runGit(t, dir, "commit", "-m", "chore: seed conflict")

	cr, err := svc.AddCR("Merge conflict recovery", "exercise conflict flow")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	setValidContract(t, svc, cr.ID)

	if err := os.WriteFile(filepath.Join(dir, "conflict.txt"), []byte("from-cr\n"), 0o644); err != nil {
		t.Fatalf("write CR side file: %v", err)
	}
	runGit(t, dir, "add", "conflict.txt")
	runGit(t, dir, "commit", "-m", "feat: CR side change")

	runGit(t, dir, "checkout", "main")
	if err := os.WriteFile(filepath.Join(dir, "conflict.txt"), []byte("from-main\n"), 0o644); err != nil {
		t.Fatalf("write base side file: %v", err)
	}
	runGit(t, dir, "add", "conflict.txt")
	runGit(t, dir, "commit", "-m", "feat: base side change")
	runGit(t, dir, "checkout", cr.Branch)

	return svc, cr, dir
}
