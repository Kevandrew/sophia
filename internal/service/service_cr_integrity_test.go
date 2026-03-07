package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sophia/internal/model"
)

func TestDoctorCRFindingsCoverCheckpointAndBaseDrift(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")

	cr, err := svc.AddCR("Integrity", "doctor checks")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	task1, err := svc.AddTask(cr.ID, "task one")
	if err != nil {
		t.Fatalf("AddTask(task1) error = %v", err)
	}
	task2, err := svc.AddTask(cr.ID, "task two")
	if err != nil {
		t.Fatalf("AddTask(task2) error = %v", err)
	}
	task3, err := svc.AddTask(cr.ID, "task three")
	if err != nil {
		t.Fatalf("AddTask(task3) error = %v", err)
	}
	task4, err := svc.AddTask(cr.ID, "task four")
	if err != nil {
		t.Fatalf("AddTask(task4) error = %v", err)
	}

	if err := os.WriteFile(filepath.Join(dir, "cr-work.txt"), []byte("work\n"), 0o644); err != nil {
		t.Fatalf("write cr-work.txt: %v", err)
	}
	runGit(t, dir, "add", "cr-work.txt")
	runGit(t, dir, "commit", "-m", "feat: cr work commit")
	crCommit := runGit(t, dir, "rev-parse", "HEAD")

	runGit(t, dir, "checkout", "main")
	if err := os.WriteFile(filepath.Join(dir, "main-work.txt"), []byte("main\n"), 0o644); err != nil {
		t.Fatalf("write main-work.txt: %v", err)
	}
	runGit(t, dir, "add", "main-work.txt")
	runGit(t, dir, "commit", "-m", "feat: main moved")
	mainCommit := runGit(t, dir, "rev-parse", "HEAD")
	runGit(t, dir, "checkout", cr.Branch)

	loaded, err := svc.store.LoadCR(cr.ID)
	if err != nil {
		t.Fatalf("LoadCR() error = %v", err)
	}
	for i := range loaded.Subtasks {
		loaded.Subtasks[i].Status = model.TaskStatusDone
	}
	loaded.Subtasks[indexOfTask(loaded.Subtasks, task1.ID)].CheckpointCommit = crCommit
	loaded.Subtasks[indexOfTask(loaded.Subtasks, task2.ID)].CheckpointCommit = crCommit
	loaded.Subtasks[indexOfTask(loaded.Subtasks, task3.ID)].CheckpointCommit = mainCommit
	loaded.Subtasks[indexOfTask(loaded.Subtasks, task4.ID)].CheckpointCommit = ""
	if err := svc.store.SaveCR(loaded); err != nil {
		t.Fatalf("SaveCR() error = %v", err)
	}

	report, err := svc.DoctorCR(cr.ID)
	if err != nil {
		t.Fatalf("DoctorCR() error = %v", err)
	}
	assertCRFindingCode(t, report.Findings, "duplicate_checkpoint_commit")
	assertCRFindingCode(t, report.Findings, "checkpoint_unreachable")
	assertCRFindingCode(t, report.Findings, "done_task_missing_checkpoint")
	assertCRFindingCode(t, report.Findings, "base_commit_drift")
}

func TestSetCRBasePreservesParentLinkWhenBaseRefPointsToParentBranch(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")

	parent, err := svc.AddCR("Parent", "parent")
	if err != nil {
		t.Fatalf("AddCR(parent) error = %v", err)
	}
	child, _, err := svc.AddCRWithOptionsWithWarnings("Child", "child", AddCROptions{ParentCRID: parent.ID})
	if err != nil {
		t.Fatalf("AddCRWithOptionsWithWarnings(child) error = %v", err)
	}
	if _, err := svc.SetCRBase(child.ID, parent.Branch, false); err != nil {
		t.Fatalf("SetCRBase() error = %v", err)
	}
	reloaded, err := svc.store.LoadCR(child.ID)
	if err != nil {
		t.Fatalf("LoadCR(child) error = %v", err)
	}
	if reloaded.ParentCRID != parent.ID {
		t.Fatalf("expected parent linkage preserved, got %#v", reloaded)
	}
	report, err := svc.DoctorCR(child.ID)
	if err != nil {
		t.Fatalf("DoctorCR() error = %v", err)
	}
	for _, finding := range report.Findings {
		if finding.Code == "parent_base_ref_mismatch" {
			t.Fatalf("expected no parent/base ref mismatch after base set, got %#v", report.Findings)
		}
	}
}

func TestReconcileCRRelinksAndOrphansCheckpoints(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")

	cr, err := svc.AddCR("Reconcile", "relink task checkpoints")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	task1, err := svc.AddTask(cr.ID, "feat: task one")
	if err != nil {
		t.Fatalf("AddTask(task1) error = %v", err)
	}
	task2, err := svc.AddTask(cr.ID, "feat: task two")
	if err != nil {
		t.Fatalf("AddTask(task2) error = %v", err)
	}
	mustSetTaskContract(t, svc, cr.ID, task1.ID, "checkpoint task one", []string{"task1 complete"}, []string{"task1.txt"})
	mustSetTaskContract(t, svc, cr.ID, task2.ID, "checkpoint task two", []string{"task2 complete"}, []string{"task2.txt"})

	if err := os.WriteFile(filepath.Join(dir, "task1.txt"), []byte("task1\n"), 0o644); err != nil {
		t.Fatalf("write task1.txt: %v", err)
	}
	task1SHA, err := svc.DoneTaskWithCheckpoint(cr.ID, task1.ID, DoneTaskOptions{Checkpoint: true, FromContract: true})
	if err != nil {
		t.Fatalf("DoneTaskWithCheckpoint(task1) error = %v", err)
	}

	runGit(t, dir, "checkout", "main")
	if err := os.WriteFile(filepath.Join(dir, "main-shift.txt"), []byte("shift\n"), 0o644); err != nil {
		t.Fatalf("write main-shift.txt: %v", err)
	}
	runGit(t, dir, "add", "main-shift.txt")
	runGit(t, dir, "commit", "-m", "feat: shift main for unreachable checkpoint")
	mainSHA := runGit(t, dir, "rev-parse", "HEAD")
	runGit(t, dir, "checkout", cr.Branch)

	loaded, err := svc.store.LoadCR(cr.ID)
	if err != nil {
		t.Fatalf("LoadCR() error = %v", err)
	}
	idx1 := indexOfTask(loaded.Subtasks, task1.ID)
	idx2 := indexOfTask(loaded.Subtasks, task2.ID)
	loaded.Subtasks[idx1].Status = model.TaskStatusDone
	loaded.Subtasks[idx1].CheckpointCommit = ""
	loaded.Subtasks[idx1].CheckpointAt = ""
	loaded.Subtasks[idx1].CheckpointMessage = ""
	loaded.Subtasks[idx1].CheckpointSource = ""
	loaded.Subtasks[idx1].CheckpointSyncAt = ""
	loaded.Subtasks[idx2].Status = model.TaskStatusDone
	loaded.Subtasks[idx2].CheckpointCommit = mainSHA
	loaded.Subtasks[idx2].CheckpointSource = "task_checkpoint"
	if err := svc.store.SaveCR(loaded); err != nil {
		t.Fatalf("SaveCR() error = %v", err)
	}

	report, err := svc.ReconcileCR(cr.ID, ReconcileCROptions{Regenerate: true})
	if err != nil {
		t.Fatalf("ReconcileCR() error = %v", err)
	}
	if report.Relinked < 1 {
		t.Fatalf("expected at least one relinked task, got %#v", report)
	}
	if report.Orphaned < 1 {
		t.Fatalf("expected at least one orphaned task, got %#v", report)
	}
	if !report.Regenerated {
		t.Fatalf("expected regenerated=true, got %#v", report)
	}

	reloaded, err := svc.store.LoadCR(cr.ID)
	if err != nil {
		t.Fatalf("LoadCR() after reconcile error = %v", err)
	}
	task1State := reloaded.Subtasks[indexOfTask(reloaded.Subtasks, task1.ID)]
	if !strings.HasPrefix(strings.TrimSpace(task1State.CheckpointCommit), strings.TrimSpace(task1SHA)) &&
		!strings.HasPrefix(strings.TrimSpace(task1SHA), strings.TrimSpace(task1State.CheckpointCommit)) {
		t.Fatalf("expected task1 checkpoint relinked to %s, got %#v", task1SHA, task1State)
	}
	if task1State.CheckpointOrphan {
		t.Fatalf("expected task1 checkpoint not orphaned, got %#v", task1State)
	}
	task2State := reloaded.Subtasks[indexOfTask(reloaded.Subtasks, task2.ID)]
	if !task2State.CheckpointOrphan {
		t.Fatalf("expected task2 checkpoint orphaned, got %#v", task2State)
	}
	if strings.TrimSpace(task2State.CheckpointReason) == "" {
		t.Fatalf("expected task2 orphan reason, got %#v", task2State)
	}
	if reloaded.FilesTouchedCount <= 0 {
		t.Fatalf("expected regenerated files_touched_count > 0, got %d", reloaded.FilesTouchedCount)
	}
}

func TestReconcileCRRelinksParentFromBaseRef(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")

	parent, err := svc.AddCR("Parent", "stack parent")
	if err != nil {
		t.Fatalf("AddCR(parent) error = %v", err)
	}
	child, _, err := svc.AddCRWithOptionsWithWarnings("Child", "stack child", AddCROptions{ParentCRID: parent.ID})
	if err != nil {
		t.Fatalf("AddCRWithOptionsWithWarnings(child) error = %v", err)
	}
	loaded, err := svc.store.LoadCR(child.ID)
	if err != nil {
		t.Fatalf("LoadCR(child) error = %v", err)
	}
	loaded.ParentCRID = 0
	if err := svc.store.SaveCR(loaded); err != nil {
		t.Fatalf("SaveCR(clear parent) error = %v", err)
	}

	report, err := svc.ReconcileCR(child.ID, ReconcileCROptions{})
	if err != nil {
		t.Fatalf("ReconcileCR() error = %v", err)
	}
	if !report.ParentRelinked {
		t.Fatalf("expected parent relink in report, got %#v", report)
	}
	if report.PreviousParentID != 0 || report.CurrentParentID != parent.ID {
		t.Fatalf("expected parent 0 -> %d, got %#v", parent.ID, report)
	}

	reloaded, err := svc.store.LoadCR(child.ID)
	if err != nil {
		t.Fatalf("LoadCR() error = %v", err)
	}
	if reloaded.ParentCRID != parent.ID {
		t.Fatalf("expected child parent_cr_id=%d after reconcile, got %d", parent.ID, reloaded.ParentCRID)
	}
}

func mustSetTaskContract(t *testing.T, svc *Service, crID, taskID int, intent string, acceptance []string, scope []string) {
	t.Helper()
	if _, err := svc.SetTaskContract(crID, taskID, TaskContractPatch{
		Intent:             &intent,
		AcceptanceCriteria: &acceptance,
		Scope:              &scope,
	}); err != nil {
		t.Fatalf("SetTaskContract(cr=%d task=%d) error = %v", crID, taskID, err)
	}
}

func assertCRFindingCode(t *testing.T, findings []CRDoctorFinding, code string) {
	t.Helper()
	for _, finding := range findings {
		if finding.Code == code {
			return
		}
	}
	t.Fatalf("expected finding code %q, got %#v", code, findings)
}
