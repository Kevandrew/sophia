package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func newCRCheckpointFixture(t *testing.T) (*Service, int, int) {
	t.Helper()
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")

	cr, err := svc.AddCR("CR contract drift", "freeze and drift flow")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}

	why := "Baseline capture test"
	scope := []string{"scoped-a.txt"}
	nonGoals := []string{"none"}
	invariants := []string{"keep behavior"}
	blast := "local"
	testPlan := "go test ./..."
	rollback := "revert"
	if _, err := svc.SetCRContract(cr.ID, ContractPatch{
		Why:          &why,
		Scope:        &scope,
		NonGoals:     &nonGoals,
		Invariants:   &invariants,
		BlastRadius:  &blast,
		TestPlan:     &testPlan,
		RollbackPlan: &rollback,
	}); err != nil {
		t.Fatalf("SetCRContract() error = %v", err)
	}

	task, err := svc.AddTask(cr.ID, "checkpoint task")
	if err != nil {
		t.Fatalf("AddTask() error = %v", err)
	}
	intent := "checkpoint"
	acceptance := []string{"checkpoint exists"}
	taskScope := []string{"scoped-a.txt"}
	if _, err := svc.SetTaskContract(cr.ID, task.ID, TaskContractPatch{
		Intent:             &intent,
		AcceptanceCriteria: &acceptance,
		Scope:              &taskScope,
	}); err != nil {
		t.Fatalf("SetTaskContract() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "scoped-a.txt"), []byte("a\n"), 0o644); err != nil {
		t.Fatalf("write scoped-a.txt: %v", err)
	}
	if _, err := svc.DoneTaskWithCheckpoint(cr.ID, task.ID, DoneTaskOptions{Checkpoint: true, Paths: []string{"scoped-a.txt"}}); err != nil {
		t.Fatalf("DoneTaskWithCheckpoint() error = %v", err)
	}
	return svc, cr.ID, task.ID
}

func TestFirstCheckpointCapturesCRContractBaselineOnce(t *testing.T) {
	t.Parallel()
	svc, crID, _ := newCRCheckpointFixture(t)
	cr, err := svc.store.LoadCR(crID)
	if err != nil {
		t.Fatalf("LoadCR() error = %v", err)
	}
	firstBaseline := cr.ContractBaseline
	if strings.TrimSpace(firstBaseline.CapturedAt) == "" || len(firstBaseline.Scope) == 0 {
		t.Fatalf("expected CR baseline to be captured, got %#v", firstBaseline)
	}

	updatedScope := []string{"scoped-a.txt", "scoped-b.txt"}
	reason := "include follow-up file"
	if _, err := svc.SetCRContract(crID, ContractPatch{Scope: &updatedScope, ChangeReason: &reason}); err != nil {
		t.Fatalf("SetCRContract(with reason) error = %v", err)
	}

	task, err := svc.AddTask(crID, "second checkpoint task")
	if err != nil {
		t.Fatalf("AddTask(second) error = %v", err)
	}
	intent := "checkpoint second"
	acceptance := []string{"checkpoint exists"}
	taskScope := []string{"scoped-b.txt"}
	if _, err := svc.SetTaskContract(crID, task.ID, TaskContractPatch{
		Intent:             &intent,
		AcceptanceCriteria: &acceptance,
		Scope:              &taskScope,
	}); err != nil {
		t.Fatalf("SetTaskContract(second) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(svc.git.WorkDir, "scoped-b.txt"), []byte("b\n"), 0o644); err != nil {
		t.Fatalf("write scoped-b.txt: %v", err)
	}
	if _, err := svc.DoneTaskWithCheckpoint(crID, task.ID, DoneTaskOptions{Checkpoint: true, Paths: []string{"scoped-b.txt"}}); err != nil {
		t.Fatalf("DoneTaskWithCheckpoint(second) error = %v", err)
	}

	reloaded, err := svc.store.LoadCR(crID)
	if err != nil {
		t.Fatalf("LoadCR(reloaded) error = %v", err)
	}
	if reloaded.ContractBaseline.CapturedAt != firstBaseline.CapturedAt {
		t.Fatalf("expected baseline captured_at to remain unchanged, before=%q after=%q", firstBaseline.CapturedAt, reloaded.ContractBaseline.CapturedAt)
	}
	if strings.Join(reloaded.ContractBaseline.Scope, ",") != strings.Join(firstBaseline.Scope, ",") {
		t.Fatalf("expected baseline scope unchanged, before=%#v after=%#v", firstBaseline.Scope, reloaded.ContractBaseline.Scope)
	}
}

func TestSetCRContractScopeAfterFreezeRequiresReasonAndRecordsDrift(t *testing.T) {
	t.Parallel()
	svc, crID, _ := newCRCheckpointFixture(t)

	updatedScope := []string{"scoped-a.txt", "scoped-b.txt"}
	if _, err := svc.SetCRContract(crID, ContractPatch{Scope: &updatedScope}); err == nil {
		t.Fatalf("expected missing change reason error")
	} else if !strings.Contains(strings.ToLower(err.Error()), "change reason") {
		t.Fatalf("expected change reason error, got %v", err)
	}

	reason := "expanded scope for follow-up"
	if _, err := svc.SetCRContract(crID, ContractPatch{Scope: &updatedScope, ChangeReason: &reason}); err != nil {
		t.Fatalf("SetCRContract(with reason) error = %v", err)
	}
	cr, err := svc.store.LoadCR(crID)
	if err != nil {
		t.Fatalf("LoadCR() error = %v", err)
	}
	if len(cr.ContractDrifts) != 1 {
		t.Fatalf("expected one CR drift record, got %#v", cr.ContractDrifts)
	}
	drift := cr.ContractDrifts[0]
	if len(drift.Fields) != 1 || drift.Fields[0] != "scope_changed" {
		t.Fatalf("expected scope_changed drift field, got %#v", drift.Fields)
	}
	if strings.TrimSpace(drift.Reason) != reason {
		t.Fatalf("expected drift reason %q, got %#v", reason, drift)
	}
	if drift.Acknowledged {
		t.Fatalf("expected unacknowledged drift by default, got %#v", drift)
	}
}

func TestMergeBlockedByUnacknowledgedCRContractDriftUntilAcked(t *testing.T) {
	t.Parallel()
	svc, crID, _ := newCRCheckpointFixture(t)

	updatedScope := []string{"scoped-a.txt", "scoped-b.txt"}
	reason := "scope changed after first checkpoint"
	if _, err := svc.SetCRContract(crID, ContractPatch{Scope: &updatedScope, ChangeReason: &reason}); err != nil {
		t.Fatalf("SetCRContract(with reason) error = %v", err)
	}
	task, err := svc.AddTask(crID, "checkpoint scoped-b")
	if err != nil {
		t.Fatalf("AddTask() error = %v", err)
	}
	intent := "checkpoint b"
	acceptance := []string{"scoped-b is checkpointed"}
	taskScope := []string{"scoped-b.txt"}
	if _, err := svc.SetTaskContract(crID, task.ID, TaskContractPatch{
		Intent:             &intent,
		AcceptanceCriteria: &acceptance,
		Scope:              &taskScope,
	}); err != nil {
		t.Fatalf("SetTaskContract() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(svc.git.WorkDir, "scoped-b.txt"), []byte("b\n"), 0o644); err != nil {
		t.Fatalf("write scoped-b.txt: %v", err)
	}
	if _, err := svc.DoneTaskWithCheckpoint(crID, task.ID, DoneTaskOptions{Checkpoint: true, Paths: []string{"scoped-b.txt"}}); err != nil {
		t.Fatalf("DoneTaskWithCheckpoint() error = %v", err)
	}

	if _, err := svc.MergeCR(crID, false, ""); err == nil {
		t.Fatalf("expected merge blocker for unacknowledged CR drift")
	}

	drifts, err := svc.ListCRContractDrifts(crID)
	if err != nil {
		t.Fatalf("ListCRContractDrifts() error = %v", err)
	}
	if len(drifts) != 1 {
		t.Fatalf("expected one drift before ack, got %#v", drifts)
	}
	if _, err := svc.AckCRContractDrift(crID, drifts[0].ID, "accepted during review"); err != nil {
		t.Fatalf("AckCRContractDrift() error = %v", err)
	}

	if _, err := svc.MergeCR(crID, false, ""); err != nil {
		t.Fatalf("MergeCR() after drift ack error = %v", err)
	}
}

func TestScopeSourceSwitchesToContractAfterCRDriftAck(t *testing.T) {
	svc, crID, _ := newCRCheckpointFixture(t)

	updatedScope := []string{"scoped-a.txt", "scoped-b.txt"}
	reason := "scope changed after first checkpoint"
	if _, err := svc.SetCRContract(crID, ContractPatch{Scope: &updatedScope, ChangeReason: &reason}); err != nil {
		t.Fatalf("SetCRContract(with reason) error = %v", err)
	}
	task, err := svc.AddTask(crID, "checkpoint scoped-b")
	if err != nil {
		t.Fatalf("AddTask() error = %v", err)
	}
	intent := "checkpoint b"
	acceptance := []string{"scoped-b is checkpointed"}
	taskScope := []string{"scoped-b.txt"}
	if _, err := svc.SetTaskContract(crID, task.ID, TaskContractPatch{
		Intent:             &intent,
		AcceptanceCriteria: &acceptance,
		Scope:              &taskScope,
	}); err != nil {
		t.Fatalf("SetTaskContract() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(svc.git.WorkDir, "scoped-b.txt"), []byte("b\n"), 0o644); err != nil {
		t.Fatalf("write scoped-b.txt: %v", err)
	}
	if _, err := svc.DoneTaskWithCheckpoint(crID, task.ID, DoneTaskOptions{Checkpoint: true, Paths: []string{"scoped-b.txt"}}); err != nil {
		t.Fatalf("DoneTaskWithCheckpoint() error = %v", err)
	}

	beforeAck, err := svc.ValidateCR(crID)
	if err != nil {
		t.Fatalf("ValidateCR(before ack) error = %v", err)
	}
	if beforeAck.Impact == nil || beforeAck.Impact.ScopeSource != "contract_baseline" {
		t.Fatalf("expected baseline scope source before ack, got %#v", beforeAck.Impact)
	}
	if !containsAny(beforeAck.Errors, "outside frozen contract baseline scope") {
		t.Fatalf("expected baseline scope drift error before ack, got %#v", beforeAck.Errors)
	}

	drifts, err := svc.ListCRContractDrifts(crID)
	if err != nil {
		t.Fatalf("ListCRContractDrifts() error = %v", err)
	}
	if len(drifts) != 1 {
		t.Fatalf("expected one drift before ack, got %#v", drifts)
	}
	if _, err := svc.AckCRContractDrift(crID, drifts[0].ID, "accepted during review"); err != nil {
		t.Fatalf("AckCRContractDrift() error = %v", err)
	}

	afterAck, err := svc.ValidateCR(crID)
	if err != nil {
		t.Fatalf("ValidateCR(after ack) error = %v", err)
	}
	if afterAck.Impact == nil || afterAck.Impact.ScopeSource != "contract_scope" {
		t.Fatalf("expected declared contract scope source after ack, got %#v", afterAck.Impact)
	}
	if containsAny(afterAck.Errors, "outside frozen contract baseline scope") {
		t.Fatalf("did not expect baseline scope drift error after ack, got %#v", afterAck.Errors)
	}
	if containsAny(afterAck.Warnings, "scope drift is evaluated against frozen contract baseline scope (first checkpoint)") {
		t.Fatalf("did not expect baseline warning after ack, got %#v", afterAck.Warnings)
	}
}

func TestSetCRContractWithOptionsReportsCRDriftWorkflowMetadata(t *testing.T) {
	svc, crID, _ := newCRCheckpointFixture(t)
	updatedScope := []string{"scoped-a.txt", "scoped-b.txt"}
	reason := "expand declared scope"

	result, err := svc.SetCRContractWithOptions(crID, ContractPatch{
		Scope:        &updatedScope,
		ChangeReason: &reason,
	}, SetCRContractOptions{})
	if err != nil {
		t.Fatalf("SetCRContractWithOptions() error = %v", err)
	}
	if result.AlreadyApplied {
		t.Fatalf("expected update to apply, got %#v", result)
	}
	if !result.ScopeChanged || !result.BaselineFrozen {
		t.Fatalf("expected scope_changed=true and baseline_frozen=true, got %#v", result)
	}
	if !result.DriftRecorded || result.DriftID <= 0 || !result.DriftAckRequired {
		t.Fatalf("expected recorded drift metadata, got %#v", result)
	}
}

func TestSetCRContractWithOptionsDryRunReportsDeterministicCRDriftMetadata(t *testing.T) {
	svc, crID, _ := newCRCheckpointFixture(t)
	updatedScope := []string{"scoped-a.txt", "scoped-b.txt"}
	reason := "expand declared scope"

	result, err := svc.SetCRContractWithOptions(crID, ContractPatch{
		Scope:        &updatedScope,
		ChangeReason: &reason,
	}, SetCRContractOptions{DryRun: true})
	if err != nil {
		t.Fatalf("SetCRContractWithOptions(dry-run) error = %v", err)
	}
	if !result.DryRun {
		t.Fatalf("expected dry_run=true, got %#v", result)
	}
	if !result.ScopeChanged || !result.BaselineFrozen {
		t.Fatalf("expected scope_changed=true and baseline_frozen=true in dry-run, got %#v", result)
	}
	if result.DriftRecorded || result.DriftID != 0 || result.DriftAckRequired {
		t.Fatalf("expected no recorded drift metadata in dry-run, got %#v", result)
	}

	drifts, err := svc.ListCRContractDrifts(crID)
	if err != nil {
		t.Fatalf("ListCRContractDrifts() error = %v", err)
	}
	if len(drifts) != 0 {
		t.Fatalf("expected dry-run to avoid recording drift, got %#v", drifts)
	}
}

func TestSetCRContractUpdatesCRUpdatedAt(t *testing.T) {
	t.Parallel()
	svc, crID, _ := newCRCheckpointFixture(t)
	before, err := svc.store.LoadCR(crID)
	if err != nil {
		t.Fatalf("LoadCR(before) error = %v", err)
	}
	beforeUpdatedAt := strings.TrimSpace(before.UpdatedAt)
	if beforeUpdatedAt == "" {
		t.Fatalf("expected non-empty CR updated_at before mutation")
	}
	beforeTS, err := time.Parse(time.RFC3339, beforeUpdatedAt)
	if err != nil {
		t.Fatalf("parse before updated_at: %v", err)
	}
	svc.now = func() time.Time { return beforeTS.Add(2 * time.Second).UTC() }

	why := "updated why text"
	if _, err := svc.SetCRContract(crID, ContractPatch{Why: &why}); err != nil {
		t.Fatalf("SetCRContract() error = %v", err)
	}
	after, err := svc.store.LoadCR(crID)
	if err != nil {
		t.Fatalf("LoadCR(after) error = %v", err)
	}
	afterUpdatedAt := strings.TrimSpace(after.UpdatedAt)
	if afterUpdatedAt == "" || afterUpdatedAt == beforeUpdatedAt {
		t.Fatalf("expected CR updated_at to advance, before=%q after=%q", beforeUpdatedAt, afterUpdatedAt)
	}
}
