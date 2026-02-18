package cli

import (
	"os"
	"path/filepath"
	"testing"

	"sophia/internal/model"
	"sophia/internal/service"
)

func TestCRDoctorAndReconcileJSON(t *testing.T) {
	dir := t.TempDir()
	svc := service.New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")

	cr, err := svc.AddCR("Integrity JSON", "cli integrity outputs")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	task, err := svc.AddTask(cr.ID, "feat: integrity task")
	if err != nil {
		t.Fatalf("AddTask() error = %v", err)
	}
	intent := "Capture checkpoint footer metadata."
	acceptance := []string{"checkpoint exists"}
	scope := []string{"integrity.txt"}
	if _, err := svc.SetTaskContract(cr.ID, task.ID, service.TaskContractPatch{
		Intent:             &intent,
		AcceptanceCriteria: &acceptance,
		Scope:              &scope,
	}); err != nil {
		t.Fatalf("SetTaskContract() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "integrity.txt"), []byte("v1\n"), 0o644); err != nil {
		t.Fatalf("write integrity.txt: %v", err)
	}
	sha, err := svc.DoneTaskWithCheckpoint(cr.ID, task.ID, service.DoneTaskOptions{Checkpoint: true, FromContract: true})
	if err != nil {
		t.Fatalf("DoneTaskWithCheckpoint() error = %v", err)
	}

	loaded, err := svc.ListTasks(cr.ID)
	if err != nil {
		t.Fatalf("ListTasks() error = %v", err)
	}
	if len(loaded) != 1 || loaded[0].Status != model.TaskStatusDone {
		t.Fatalf("unexpected task state before reconcile: %#v", loaded)
	}
	if _, err := svc.ReopenTask(cr.ID, task.ID, service.ReopenTaskOptions{ClearCheckpoint: true}); err != nil {
		t.Fatalf("ReopenTask() error = %v", err)
	}
	if _, err := svc.DoneTaskWithCheckpoint(cr.ID, task.ID, service.DoneTaskOptions{Checkpoint: false}); err != nil {
		t.Fatalf("DoneTaskWithCheckpoint(no checkpoint) error = %v", err)
	}

	out, _, runErr := runCLI(t, dir, "cr", "doctor", "1", "--json")
	if runErr != nil {
		t.Fatalf("cr doctor --json error = %v\noutput=%s", runErr, out)
	}
	doctorEnv := decodeEnvelope(t, out)
	if !doctorEnv.OK {
		t.Fatalf("expected doctor envelope ok=true, got %#v", doctorEnv)
	}
	for _, key := range []string{"cr_id", "cr_uid", "branch", "base_ref", "findings"} {
		if _, ok := doctorEnv.Data[key]; !ok {
			t.Fatalf("expected doctor key %q in %#v", key, doctorEnv.Data)
		}
	}

	out, _, runErr = runCLI(t, dir, "cr", "reconcile", "1", "--regenerate", "--json")
	if runErr != nil {
		t.Fatalf("cr reconcile --json error = %v\noutput=%s", runErr, out)
	}
	reconcileEnv := decodeEnvelope(t, out)
	if !reconcileEnv.OK {
		t.Fatalf("expected reconcile envelope ok=true, got %#v", reconcileEnv)
	}
	for _, key := range []string{"cr_id", "scan_ref", "relinked", "orphaned", "parent_relinked", "task_results", "findings"} {
		if _, ok := reconcileEnv.Data[key]; !ok {
			t.Fatalf("expected reconcile key %q in %#v", key, reconcileEnv.Data)
		}
	}
	if got := reconcileEnv.Data["relinked"].(float64); got < 1 {
		t.Fatalf("expected reconcile relinked >= 1, got %v (sha=%s)", got, sha)
	}
}
