package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"sophia/internal/service"
)

var cliCWDMu sync.Mutex

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
		{args: []string{"cr", "task", "contract", "show", "1", "1", "--json"}, keys: []string{"cr_id", "task_id", "task_contract"}},
		{args: []string{"cr", "why", "1", "--json"}, keys: []string{"effective_why", "source"}},
		{args: []string{"cr", "status", "1", "--json"}, keys: []string{"id", "title", "working_tree", "validation", "merge_blocked"}},
		{args: []string{"cr", "impact", "1", "--json"}, keys: []string{"cr_id", "risk_tier", "risk_score"}},
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

func TestTaskDoneFlagConflictsWithFromContract(t *testing.T) {
	dir := t.TempDir()
	svc := service.New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	cr, err := svc.AddCR("Task flags", "conflict checks")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	task, err := svc.AddTask(cr.ID, "feat: flag conflicts")
	if err != nil {
		t.Fatalf("AddTask() error = %v", err)
	}
	intent := "Set up task for conflict checks."
	acceptance := []string{"Flag conflicts are rejected."}
	scope := []string{"feature.txt"}
	if _, err := svc.SetTaskContract(cr.ID, task.ID, service.TaskContractPatch{
		Intent:             &intent,
		AcceptanceCriteria: &acceptance,
		Scope:              &scope,
	}); err != nil {
		t.Fatalf("SetTaskContract() error = %v", err)
	}

	_, _, err = runCLI(t, dir, "cr", "task", "done", "1", "1", "--no-checkpoint", "--from-contract")
	if err == nil || !strings.Contains(err.Error(), "--no-checkpoint cannot be combined") {
		t.Fatalf("expected --no-checkpoint conflict error, got %v", err)
	}

	_, _, err = runCLI(t, dir, "cr", "task", "done", "1", "1", "--from-contract", "--all")
	if err == nil || !strings.Contains(err.Error(), "exactly one checkpoint scope mode is required") {
		t.Fatalf("expected exclusivity conflict error, got %v", err)
	}
}

func runCLI(t *testing.T, dir string, args ...string) (string, string, error) {
	t.Helper()
	cliCWDMu.Lock()
	defer cliCWDMu.Unlock()

	orig, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir(%s) error = %v", dir, err)
	}
	defer func() {
		_ = os.Chdir(orig)
	}()

	root := newRootCmd()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs(args)
	err = root.Execute()
	return stdout.String(), stderr.String(), err
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

type envelope struct {
	OK    bool                  `json:"ok"`
	Data  map[string]any        `json:"data"`
	Error *envelopeErrorPayload `json:"error,omitempty"`
}

type envelopeErrorPayload struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func decodeEnvelope(t *testing.T, raw string) envelope {
	t.Helper()
	var env envelope
	if err := json.Unmarshal([]byte(raw), &env); err != nil {
		t.Fatalf("decode envelope: %v\nraw=%s", err, raw)
	}
	return env
}
