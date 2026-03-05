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

func TestCRChildAddWithoutActiveContextReturnsActionableError(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := service.New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	_, _, runErr := runCLI(t, dir, "cr", "child", "add", "Child without context")
	if runErr == nil {
		t.Fatalf("expected child add to fail without active CR context")
	}
	if !strings.Contains(runErr.Error(), "current branch is not a CR branch; run `sophia cr switch <id>` or use `sophia cr add <title> --parent <id>`") {
		t.Fatalf("expected actionable no-active-context guidance, got %v", runErr)
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

func TestCRStatusAndStackJSONIncludeAggregateParentFields(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := service.New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	parent, err := svc.AddCR("Parent", "aggregate json")
	if err != nil {
		t.Fatalf("AddCR(parent) error = %v", err)
	}
	setValidContractCLI(t, svc, parent.ID)
	taskResolved, err := svc.AddTask(parent.ID, "resolved child")
	if err != nil {
		t.Fatalf("AddTask(resolved) error = %v", err)
	}
	setValidTaskContractCLI(t, svc, parent.ID, taskResolved.ID)
	taskPending, err := svc.AddTask(parent.ID, "pending child")
	if err != nil {
		t.Fatalf("AddTask(pending) error = %v", err)
	}
	setValidTaskContractCLI(t, svc, parent.ID, taskPending.ID)

	childResolved, _, err := svc.AddCRWithOptionsWithWarnings("Resolved child", "delegated implementation", service.AddCROptions{ParentCRID: parent.ID})
	if err != nil {
		t.Fatalf("AddCR(childResolved) error = %v", err)
	}
	setValidContractCLI(t, svc, childResolved.ID)
	childPending, _, err := svc.AddCRWithOptionsWithWarnings("Pending child", "delegated implementation", service.AddCROptions{ParentCRID: parent.ID})
	if err != nil {
		t.Fatalf("AddCR(childPending) error = %v", err)
	}
	setValidContractCLI(t, svc, childPending.ID)

	if _, err := svc.DelegateTaskToChild(parent.ID, taskResolved.ID, childResolved.ID); err != nil {
		t.Fatalf("DelegateTaskToChild(resolved) error = %v", err)
	}
	if _, err := svc.DelegateTaskToChild(parent.ID, taskPending.ID, childPending.ID); err != nil {
		t.Fatalf("DelegateTaskToChild(pending) error = %v", err)
	}

	out, _, runErr := runCLI(t, dir, "cr", "status", strconv.Itoa(parent.ID), "--json")
	if runErr != nil {
		t.Fatalf("cr status --json error = %v\noutput=%s", runErr, out)
	}
	env := decodeEnvelope(t, out)
	aggregate, ok := env.Data["aggregate_parent"].(map[string]any)
	if !ok {
		t.Fatalf("expected aggregate_parent object, got %#v", env.Data["aggregate_parent"])
	}
	if enabled, _ := aggregate["enabled"].(bool); !enabled {
		t.Fatalf("expected aggregate parent enabled, got %#v", aggregate)
	}
	if resolvedCount, _ := aggregate["resolved_count"].(float64); resolvedCount != 0 {
		t.Fatalf("expected resolved_count 0, got %#v", aggregate)
	}
	if pendingCount, _ := aggregate["pending_count"].(float64); pendingCount != 2 {
		t.Fatalf("expected pending_count 2, got %#v", aggregate)
	}

	out, _, runErr = runCLI(t, dir, "cr", "stack", strconv.Itoa(parent.ID), "--json")
	if runErr != nil {
		t.Fatalf("cr stack --json error = %v\noutput=%s", runErr, out)
	}
	env = decodeEnvelope(t, out)
	nodes, ok := env.Data["nodes"].([]any)
	if !ok || len(nodes) == 0 {
		t.Fatalf("expected stack nodes, got %#v", env.Data["nodes"])
	}
	root, ok := nodes[0].(map[string]any)
	if !ok {
		t.Fatalf("expected root node map, got %#v", nodes[0])
	}
	if enabled, _ := root["aggregate_parent"].(bool); !enabled {
		t.Fatalf("expected aggregate_parent true on root node, got %#v", root)
	}
	if resolved, ok := root["aggregate_resolved_children"].([]any); !ok || len(resolved) != 0 {
		t.Fatalf("expected zero resolved children on root node, got %#v", root)
	}
	if pending, ok := root["aggregate_pending_children"].([]any); !ok || len(pending) != 2 {
		t.Fatalf("expected two pending children on root node, got %#v", root)
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
