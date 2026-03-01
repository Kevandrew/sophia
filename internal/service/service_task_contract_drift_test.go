package service

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writePolicyWithChecks(t *testing.T, dir string, keys ...string) {
	t.Helper()
	var b strings.Builder
	b.WriteString("version: v1\n")
	if len(keys) == 0 {
		if err := os.WriteFile(filepath.Join(dir, "SOPHIA.yaml"), []byte(b.String()), 0o644); err != nil {
			t.Fatalf("write policy: %v", err)
		}
		return
	}
	b.WriteString("trust:\n")
	b.WriteString("  checks:\n")
	b.WriteString("    definitions:\n")
	for _, key := range keys {
		fmt.Fprintf(&b, "      - key: %s\n", key)
		fmt.Fprintf(&b, "        command: echo %s\n", key)
		b.WriteString("        tiers: [low, medium, high]\n")
		b.WriteString("        allow_exit_codes: [0]\n")
	}
	if err := os.WriteFile(filepath.Join(dir, "SOPHIA.yaml"), []byte(b.String()), 0o644); err != nil {
		t.Fatalf("write policy: %v", err)
	}
}

func newCheckpointedTaskFixture(t *testing.T, checks []string) (*Service, int, int) {
	t.Helper()
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")
	writePolicyWithChecks(t, dir, "unit_tests", "lint")

	cr, err := svc.AddCR("Task drift", "record drift")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	setValidContract(t, svc, cr.ID)
	task, err := svc.AddTask(cr.ID, "Task with checkpoint")
	if err != nil {
		t.Fatalf("AddTask() error = %v", err)
	}
	intent := "Lock task contract before checkpoint."
	acceptance := []string{"Checkpoint captures scoped files."}
	scope := []string{"scoped.txt"}
	patch := TaskContractPatch{
		Intent:             &intent,
		AcceptanceCriteria: &acceptance,
		Scope:              &scope,
	}
	if len(checks) > 0 {
		copied := append([]string(nil), checks...)
		patch.AcceptanceChecks = &copied
	}
	if _, err := svc.SetTaskContract(cr.ID, task.ID, patch); err != nil {
		t.Fatalf("SetTaskContract() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "scoped.txt"), []byte("checkpoint\n"), 0o644); err != nil {
		t.Fatalf("write scoped file: %v", err)
	}
	if _, err := svc.DoneTaskWithCheckpoint(cr.ID, task.ID, DoneTaskOptions{Checkpoint: true, Paths: []string{"scoped.txt"}}); err != nil {
		t.Fatalf("DoneTaskWithCheckpoint() error = %v", err)
	}
	return svc, cr.ID, task.ID
}

func TestDoneTaskCheckpointCapturesTaskContractBaseline(t *testing.T) {
	t.Parallel()
	svc, crID, taskID := newCheckpointedTaskFixture(t, []string{"unit_tests"})

	cr, err := svc.store.LoadCR(crID)
	if err != nil {
		t.Fatalf("LoadCR() error = %v", err)
	}
	task := cr.Subtasks[indexOfTask(cr.Subtasks, taskID)]
	if strings.TrimSpace(task.ContractBaseline.CapturedAt) == "" {
		t.Fatalf("expected baseline captured_at, got %#v", task.ContractBaseline)
	}
	if task.ContractBaseline.Intent != task.Contract.Intent {
		t.Fatalf("expected baseline intent %q, got %q", task.Contract.Intent, task.ContractBaseline.Intent)
	}
	if len(task.ContractBaseline.AcceptanceChecks) != 1 || task.ContractBaseline.AcceptanceChecks[0] != "unit_tests" {
		t.Fatalf("expected baseline acceptance checks, got %#v", task.ContractBaseline.AcceptanceChecks)
	}
}

func TestSetTaskContractRecordsScopeWidenDriftAndSupportsAck(t *testing.T) {
	t.Parallel()
	svc, crID, taskID := newCheckpointedTaskFixture(t, nil)

	scope := []string{"."}
	if _, err := svc.SetTaskContract(crID, taskID, TaskContractPatch{Scope: &scope}); err != nil {
		t.Fatalf("SetTaskContract() error = %v", err)
	}
	cr, err := svc.store.LoadCR(crID)
	if err != nil {
		t.Fatalf("LoadCR() error = %v", err)
	}
	task := cr.Subtasks[indexOfTask(cr.Subtasks, taskID)]
	if len(task.ContractDrifts) != 1 {
		t.Fatalf("expected 1 drift record, got %#v", task.ContractDrifts)
	}
	drift := task.ContractDrifts[0]
	if !containsAny(drift.Fields, "scope_widened") {
		t.Fatalf("expected scope_widened drift, got %#v", drift.Fields)
	}
	if drift.Acknowledged {
		t.Fatalf("expected unacknowledged drift, got %#v", drift)
	}

	acked, err := svc.AckTaskContractDrift(crID, taskID, drift.ID, "scope had to expand")
	if err != nil {
		t.Fatalf("AckTaskContractDrift() error = %v", err)
	}
	if !acked.Acknowledged || strings.TrimSpace(acked.AckReason) == "" {
		t.Fatalf("expected acknowledged drift after ack, got %#v", acked)
	}
}

func TestSetTaskContractChangeReasonAutoAcknowledgesDrift(t *testing.T) {
	t.Parallel()
	svc, crID, taskID := newCheckpointedTaskFixture(t, []string{"unit_tests"})

	checks := []string{"lint"}
	reason := "switch task proof to lint gate"
	if _, err := svc.SetTaskContract(crID, taskID, TaskContractPatch{AcceptanceChecks: &checks, ChangeReason: &reason}); err != nil {
		t.Fatalf("SetTaskContract() error = %v", err)
	}
	cr, err := svc.store.LoadCR(crID)
	if err != nil {
		t.Fatalf("LoadCR() error = %v", err)
	}
	task := cr.Subtasks[indexOfTask(cr.Subtasks, taskID)]
	if len(task.ContractDrifts) != 1 {
		t.Fatalf("expected drift record, got %#v", task.ContractDrifts)
	}
	drift := task.ContractDrifts[0]
	if !containsAny(drift.Fields, "acceptance_checks_changed") {
		t.Fatalf("expected acceptance_checks_changed drift, got %#v", drift.Fields)
	}
	if !drift.Acknowledged || strings.TrimSpace(drift.AckReason) != reason {
		t.Fatalf("expected auto-acknowledged drift with reason, got %#v", drift)
	}
}

func TestValidateCRFailsWhenAcceptanceCheckKeyMissingFromPolicy(t *testing.T) {
	t.Parallel()
	svc, crID, taskID := newCheckpointedTaskFixture(t, []string{"unit_tests"})
	writePolicyWithChecks(t, svc.git.WorkDir)

	report, err := svc.ValidateCR(crID)
	if err != nil {
		t.Fatalf("ValidateCR() error = %v", err)
	}
	if report.Valid {
		t.Fatalf("expected validation failure when acceptance check key is missing from policy")
	}
	if !containsAny(report.Errors, fmt.Sprintf("task %d acceptance_checks", taskID)) {
		t.Fatalf("expected acceptance check validation error, got %#v", report.Errors)
	}
}

func TestSetTaskContractRejectsUnknownAcceptanceCheckKey(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	writePolicyWithChecks(t, dir, "unit_tests")
	cr, err := svc.AddCR("Check key", "validate acceptance key")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	task, err := svc.AddTask(cr.ID, "Task")
	if err != nil {
		t.Fatalf("AddTask() error = %v", err)
	}
	intent := "Task intent"
	acceptance := []string{"Task acceptance"}
	scope := []string{"."}
	checks := []string{"does_not_exist"}
	_, err = svc.SetTaskContract(cr.ID, task.ID, TaskContractPatch{
		Intent:             &intent,
		AcceptanceCriteria: &acceptance,
		Scope:              &scope,
		AcceptanceChecks:   &checks,
	})
	if err == nil {
		t.Fatalf("expected acceptance check validation error")
	}
	if !strings.Contains(err.Error(), "acceptance_checks key") {
		t.Fatalf("expected acceptance check key error, got %v", err)
	}
}
