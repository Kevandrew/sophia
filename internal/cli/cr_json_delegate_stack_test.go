package cli

import (
	"strconv"
	"strings"
	"testing"

	"sophia/internal/service"
)

func TestCRChildAddAndStackJSON(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := service.New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	parent, err := svc.AddCR("Parent", "stack root")
	if err != nil {
		t.Fatalf("AddCR(parent) error = %v", err)
	}
	out, _, runErr := runCLI(t, dir, "cr", "child", "add", "Child", "--description", "stack child")
	if runErr != nil {
		t.Fatalf("cr child add error = %v\noutput=%s", runErr, out)
	}
	if !strings.Contains(out, "Created child CR") {
		t.Fatalf("unexpected child add output: %q", out)
	}

	out, _, runErr = runCLI(t, dir, "cr", "stack", strconv.Itoa(parent.ID), "--json")
	if runErr != nil {
		t.Fatalf("cr stack --json error = %v\noutput=%s", runErr, out)
	}
	env := decodeEnvelope(t, out)
	if !env.OK {
		t.Fatalf("expected ok envelope, got %#v", env)
	}
	nodes, ok := env.Data["nodes"].([]any)
	if !ok || len(nodes) != 2 {
		t.Fatalf("expected two stack nodes, got %#v", env.Data["nodes"])
	}
}

func TestCRTaskDelegateAndUndelegateCommands(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := service.New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	parent, err := svc.AddCR("Parent", "delegation test")
	if err != nil {
		t.Fatalf("AddCR(parent) error = %v", err)
	}
	setValidContractCLI(t, svc, parent.ID)
	parentTask, err := svc.AddTask(parent.ID, "delegate task")
	if err != nil {
		t.Fatalf("AddTask(parent) error = %v", err)
	}
	setValidTaskContractCLI(t, svc, parent.ID, parentTask.ID)

	child, _, err := svc.AddCRWithOptionsWithWarnings("Child", "delegated child", service.AddCROptions{ParentCRID: parent.ID})
	if err != nil {
		t.Fatalf("AddCR(child) error = %v", err)
	}
	setValidContractCLI(t, svc, child.ID)

	out, _, runErr := runCLI(t, dir, "cr", "task", "delegate", strconv.Itoa(parent.ID), strconv.Itoa(parentTask.ID), "--child", strconv.Itoa(child.ID))
	if runErr != nil {
		t.Fatalf("cr task delegate error = %v\noutput=%s", runErr, out)
	}
	if !strings.Contains(out, "Delegated CR") {
		t.Fatalf("unexpected delegate output: %q", out)
	}

	out, _, runErr = runCLI(t, dir, "cr", "status", strconv.Itoa(parent.ID), "--json")
	if runErr != nil {
		t.Fatalf("cr status --json error = %v\noutput=%s", runErr, out)
	}
	env := decodeEnvelope(t, out)
	tasks, ok := env.Data["tasks"].(map[string]any)
	if !ok {
		t.Fatalf("expected tasks object, got %#v", env.Data["tasks"])
	}
	delegated, _ := tasks["delegated"].(float64)
	if delegated != 1 {
		t.Fatalf("expected delegated count 1, got %#v", tasks)
	}

	out, _, runErr = runCLI(t, dir, "cr", "task", "undelegate", strconv.Itoa(parent.ID), strconv.Itoa(parentTask.ID), "--child", strconv.Itoa(child.ID))
	if runErr != nil {
		t.Fatalf("cr task undelegate error = %v\noutput=%s", runErr, out)
	}
	if !strings.Contains(out, "Removed 1 delegation") {
		t.Fatalf("unexpected undelegate output: %q", out)
	}
}

func setValidContractCLI(t *testing.T, svc *service.Service, crID int) {
	t.Helper()
	why := "why"
	scope := []string{"."}
	nonGoals := []string{"n"}
	invariants := []string{"i"}
	blast := "b"
	testPlan := "go test ./..."
	rollback := "revert"
	if _, err := svc.SetCRContract(crID, service.ContractPatch{
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
}

func setValidTaskContractCLI(t *testing.T, svc *service.Service, crID, taskID int) {
	t.Helper()
	intent := "intent"
	acceptance := []string{"a"}
	scope := []string{"."}
	if _, err := svc.SetTaskContract(crID, taskID, service.TaskContractPatch{
		Intent:             &intent,
		AcceptanceCriteria: &acceptance,
		Scope:              &scope,
	}); err != nil {
		t.Fatalf("SetTaskContract() error = %v", err)
	}
}
