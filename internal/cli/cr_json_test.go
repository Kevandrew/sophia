package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sophia/internal/service"
)

func TestCRJSONCommandsReturnEnvelope(t *testing.T) {
	dir := t.TempDir()
	svc := service.New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")

	cr, err := svc.AddCR("JSON CR", "json rationale")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	task, err := svc.AddTask(cr.ID, "feat: json outputs")
	if err != nil {
		t.Fatalf("AddTask() error = %v", err)
	}
	why := "Provide machine-readable command output."
	scope := []string{"feature.txt"}
	nonGoals := []string{"No orchestration"}
	invariants := []string{"Git remains source of truth"}
	blast := "CLI output surface only"
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
	intent := "Task contract for JSON test."
	acceptance := []string{"Task contract show has data."}
	taskScope := []string{"feature.txt"}
	if _, err := svc.SetTaskContract(cr.ID, task.ID, service.TaskContractPatch{
		Intent:             &intent,
		AcceptanceCriteria: &acceptance,
		Scope:              &taskScope,
	}); err != nil {
		t.Fatalf("SetTaskContract() error = %v", err)
	}

	if err := os.WriteFile(filepath.Join(dir, "feature.txt"), []byte("json\n"), 0o644); err != nil {
		t.Fatalf("write feature file: %v", err)
	}
	runGit(t, dir, "add", "feature.txt")
	runGit(t, dir, "commit", "-m", "feat: json fixture")

	cases := []struct {
		args []string
		keys []string
	}{
		{args: []string{"cr", "current", "--json"}, keys: []string{"branch", "cr"}},
		{args: []string{"cr", "task", "list", "1", "--json"}, keys: []string{"cr_id", "tasks"}},
		{args: []string{"cr", "task", "chunk", "list", "1", "1", "--json"}, keys: []string{"cr_id", "task_id", "chunks"}},
		{args: []string{"cr", "task", "contract", "show", "1", "1", "--json"}, keys: []string{"cr_id", "task_id", "task_contract"}},
		{args: []string{"cr", "why", "1", "--json"}, keys: []string{"cr_uid", "base_ref", "base_commit", "parent_cr_id", "effective_why", "source"}},
		{args: []string{"cr", "status", "1", "--json"}, keys: []string{"id", "uid", "base_ref", "base_commit", "parent_cr_id", "parent_status", "title", "working_tree", "validation", "merge_blocked"}},
		{args: []string{"cr", "impact", "1", "--json"}, keys: []string{"cr_id", "cr_uid", "base_ref", "base_commit", "parent_cr_id", "risk_tier", "risk_score", "risk_tier_hint", "risk_tier_floor_applied", "matched_risk_critical_scopes"}},
		{args: []string{"cr", "review", "1", "--json"}, keys: []string{"cr", "impact", "validation_errors", "validation_warnings"}},
		{args: []string{"cr", "validate", "1", "--json"}, keys: []string{"valid", "errors", "warnings", "impact"}},
	}

	for _, tc := range cases {
		out, _, runErr := runCLI(t, dir, tc.args...)
		if runErr != nil {
			t.Fatalf("%q error = %v\noutput=%s", strings.Join(tc.args, " "), runErr, out)
		}
		env := decodeEnvelope(t, out)
		if !env.OK {
			t.Fatalf("%q expected ok envelope, got %#v", strings.Join(tc.args, " "), env)
		}
		for _, key := range tc.keys {
			if _, ok := env.Data[key]; !ok {
				t.Fatalf("%q expected data key %q in %#v", strings.Join(tc.args, " "), key, env.Data)
			}
		}
	}

	out, _, runErr := runCLI(t, dir, "cr", "current", "--json")
	if runErr != nil {
		t.Fatalf("cr current --json error = %v\noutput=%s", runErr, out)
	}
	currentEnv := decodeEnvelope(t, out)
	crData, ok := currentEnv.Data["cr"].(map[string]any)
	if !ok {
		t.Fatalf("expected current.cr object, got %#v", currentEnv.Data["cr"])
	}
	uid, _ := crData["uid"].(string)
	if strings.TrimSpace(uid) == "" {
		t.Fatalf("expected current.cr.uid to be non-empty, got %#v", crData)
	}

	out, _, runErr = runCLI(t, dir, "cr", "review", "1", "--json")
	if runErr != nil {
		t.Fatalf("cr review --json error = %v\noutput=%s", runErr, out)
	}
	reviewEnv := decodeEnvelope(t, out)
	reviewCR, ok := reviewEnv.Data["cr"].(map[string]any)
	if !ok {
		t.Fatalf("expected review.cr object, got %#v", reviewEnv.Data["cr"])
	}
	reviewUID, _ := reviewCR["uid"].(string)
	if strings.TrimSpace(reviewUID) == "" {
		t.Fatalf("expected review.cr.uid to be non-empty, got %#v", reviewCR)
	}
	reviewSubtasks, ok := reviewEnv.Data["subtasks"].([]any)
	if !ok || len(reviewSubtasks) == 0 {
		t.Fatalf("expected review subtasks array, got %#v", reviewEnv.Data["subtasks"])
	}
	firstSubtask, ok := reviewSubtasks[0].(map[string]any)
	if !ok {
		t.Fatalf("expected review subtask object, got %#v", reviewSubtasks[0])
	}
	if _, ok := firstSubtask["checkpoint_chunks"]; !ok {
		t.Fatalf("expected review subtask checkpoint_chunks field, got %#v", firstSubtask)
	}
}

func TestValidateJSONReturnsStructuredErrorWhenInvalid(t *testing.T) {
	dir := t.TempDir()
	svc := service.New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	cr, err := svc.AddCR("Invalid validate", "missing contract")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	if cr.ID != 1 {
		t.Fatalf("expected CR id 1, got %d", cr.ID)
	}

	out, _, runErr := runCLI(t, dir, "cr", "validate", "1", "--json")
	if runErr == nil {
		t.Fatalf("expected validate --json to fail for invalid CR")
	}
	env := decodeEnvelope(t, out)
	if env.OK {
		t.Fatalf("expected non-ok envelope, got %#v", env)
	}
	if env.Error == nil {
		t.Fatalf("expected structured error payload, got %#v", env)
	}
	if env.Error.Code != "validation_failed" {
		t.Fatalf("expected validation_failed code, got %#v", env.Error)
	}
}
