package cli

import (
	"os"
	"path/filepath"
	"sophia/internal/service"
	"strings"
	"testing"
)

func setupCRContractDriftCLIFixture(t *testing.T, dir string) *service.Service {
	t.Helper()
	svc := service.New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")

	cr, err := svc.AddCR("CLI CR drift", "json flow")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	why := "why"
	scope := []string{"a.txt"}
	nonGoals := []string{"none"}
	invariants := []string{"keep"}
	blast := "local"
	testPlan := "go test ./..."
	rollback := "revert"
	if _, err := svc.SetCRContract(cr.ID, service.ContractPatch{
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
	task, err := svc.AddTask(cr.ID, "checkpoint")
	if err != nil {
		t.Fatalf("AddTask() error = %v", err)
	}
	intent := "intent"
	acceptance := []string{"accept"}
	taskScope := []string{"a.txt"}
	if _, err := svc.SetTaskContract(cr.ID, task.ID, service.TaskContractPatch{
		Intent:             &intent,
		AcceptanceCriteria: &acceptance,
		Scope:              &taskScope,
	}); err != nil {
		t.Fatalf("SetTaskContract() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a\n"), 0o644); err != nil {
		t.Fatalf("write a.txt: %v", err)
	}
	if _, err := svc.DoneTaskWithCheckpoint(cr.ID, task.ID, service.DoneTaskOptions{Checkpoint: true, Paths: []string{"a.txt"}}); err != nil {
		t.Fatalf("DoneTaskWithCheckpoint() error = %v", err)
	}
	return svc
}

func TestCRContractDriftCommandsAndChangeReasonJSON(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	_ = setupCRContractDriftCLIFixture(t, dir)

	out, _, runErr := runCLI(t, dir, "cr", "contract", "set", "1", "--scope", "a.txt", "--scope", "b.txt", "--json")
	if runErr == nil {
		t.Fatalf("expected contract set to fail without --change-reason after freeze")
	}
	errEnv := decodeEnvelope(t, out)
	if errEnv.OK {
		t.Fatalf("expected error envelope, got %#v", errEnv)
	}

	out, _, runErr = runCLI(t, dir, "cr", "contract", "set", "1", "--scope", "a.txt", "--scope", "b.txt", "--change-reason", "widen scope", "--json")
	if runErr != nil {
		t.Fatalf("cr contract set --change-reason --json error = %v\noutput=%s", runErr, out)
	}
	env := decodeEnvelope(t, out)
	if !env.OK {
		t.Fatalf("expected ok envelope from contract set with reason, got %#v", env)
	}
	if scopeChanged, _ := env.Data["scope_changed"].(bool); !scopeChanged {
		t.Fatalf("expected scope_changed=true, got %#v", env.Data["scope_changed"])
	}
	if baselineFrozen, _ := env.Data["baseline_frozen"].(bool); !baselineFrozen {
		t.Fatalf("expected baseline_frozen=true, got %#v", env.Data["baseline_frozen"])
	}
	if driftRecorded, _ := env.Data["drift_recorded"].(bool); !driftRecorded {
		t.Fatalf("expected drift_recorded=true, got %#v", env.Data["drift_recorded"])
	}
	if driftAckRequired, _ := env.Data["drift_ack_required"].(bool); !driftAckRequired {
		t.Fatalf("expected drift_ack_required=true, got %#v", env.Data["drift_ack_required"])
	}
	if driftID, ok := env.Data["drift_id"].(float64); !ok || int(driftID) <= 0 {
		t.Fatalf("expected drift_id > 0, got %#v", env.Data["drift_id"])
	}

	out, _, runErr = runCLI(t, dir, "cr", "contract", "drift", "list", "1", "--json")
	if runErr != nil {
		t.Fatalf("cr contract drift list --json error = %v\noutput=%s", runErr, out)
	}
	env = decodeEnvelope(t, out)
	if !env.OK {
		t.Fatalf("expected ok envelope from drift list, got %#v", env)
	}
	drifts := requireJSONArrayField(t, env.Data, "drifts")
	if len(drifts) != 1 {
		t.Fatalf("expected one CR drift record, got %#v", drifts)
	}

	out, _, runErr = runCLI(t, dir, "cr", "contract", "drift", "ack", "1", "1", "--reason", "accepted", "--json")
	if runErr != nil {
		t.Fatalf("cr contract drift ack --json error = %v\noutput=%s", runErr, out)
	}
	env = decodeEnvelope(t, out)
	if !env.OK {
		t.Fatalf("expected ok envelope from drift ack, got %#v", env)
	}

	out, _, runErr = runCLI(t, dir, "cr", "contract", "show", "1", "--json")
	if runErr != nil {
		t.Fatalf("cr contract show --json error = %v\noutput=%s", runErr, out)
	}
	env = decodeEnvelope(t, out)
	if !env.OK {
		t.Fatalf("expected ok envelope from contract show, got %#v", env)
	}
	if _, ok := env.Data["contract_baseline"]; !ok {
		t.Fatalf("expected contract_baseline in show payload, got %#v", env.Data)
	}
	if _, ok := env.Data["contract_drifts"]; !ok {
		t.Fatalf("expected contract_drifts in show payload, got %#v", env.Data)
	}
}

func TestCRContractSetAndDriftListTextGuidance(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	_ = setupCRContractDriftCLIFixture(t, dir)

	out, _, runErr := runCLI(t, dir, "cr", "contract", "set", "1", "--scope", "a.txt", "--scope", "b.txt", "--change-reason", "widen scope")
	if runErr != nil {
		t.Fatalf("cr contract set text error = %v\noutput=%s", runErr, out)
	}
	if !strings.Contains(out, "CR contract scope drift was recorded and must be acknowledged before merge.") {
		t.Fatalf("expected drift guidance message, output=%s", out)
	}
	if !strings.Contains(out, "sophia cr contract drift list 1") {
		t.Fatalf("expected drift list guidance command, output=%s", out)
	}
	if !strings.Contains(out, "sophia cr contract drift ack 1 1 --reason") {
		t.Fatalf("expected drift ack guidance command, output=%s", out)
	}

	out, _, runErr = runCLI(t, dir, "cr", "contract", "drift", "list", "1")
	if runErr != nil {
		t.Fatalf("cr contract drift list text error = %v\noutput=%s", runErr, out)
	}
	if !strings.Contains(out, "reason: widen scope") {
		t.Fatalf("expected drift reason in text output, output=%s", out)
	}
	if !strings.Contains(out, "scope_added: b.txt") {
		t.Fatalf("expected scope delta added in text output, output=%s", out)
	}
	if !strings.Contains(out, "scope_removed: (none)") {
		t.Fatalf("expected scope delta removed in text output, output=%s", out)
	}
	if !strings.Contains(out, "next: sophia cr contract drift ack 1 1 --reason") {
		t.Fatalf("expected remediation command in text output, output=%s", out)
	}
}
