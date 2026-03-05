package cli

import (
	"strconv"
	"strings"
	"testing"

	"sophia/internal/model"
	"sophia/internal/service"
)

func TestCRDelegateListAndShowJSON(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := service.New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	cr, err := svc.AddCRWithOptions("Delegation inspect fixture", "inspect completed and failed runs", service.AddCROptions{NoSwitch: true})
	if err != nil {
		t.Fatalf("AddCRWithOptions() error = %v", err)
	}
	task, err := svc.AddTask(cr.CR.ID, "delegate me")
	if err != nil {
		t.Fatalf("AddTask() error = %v", err)
	}

	completed, err := svc.StartDelegation(cr.CR.ID, model.DelegationRequest{
		Runtime: "mock",
		TaskIDs: []int{task.ID},
		Metadata: map[string]string{
			"mock_files_changed": "internal/cli/cr_cmd_delegate.go",
		},
	})
	if err != nil {
		t.Fatalf("StartDelegation(completed) error = %v", err)
	}
	failed, err := svc.StartDelegation(cr.CR.ID, model.DelegationRequest{
		Runtime: "mock",
		Metadata: map[string]string{
			"mock_outcome": model.DelegationRunStatusFailed,
		},
	})
	if err != nil {
		t.Fatalf("StartDelegation(failed) error = %v", err)
	}

	out, _, runErr := runCLI(t, dir, "cr", "delegate", "list", strconv.Itoa(cr.CR.ID), "--json")
	if runErr != nil {
		t.Fatalf("cr delegate list --json error = %v\noutput=%s", runErr, out)
	}
	env := decodeEnvelope(t, out)
	if !env.OK {
		t.Fatalf("expected ok envelope, got %#v", env)
	}
	if got, _ := env.Data["count"].(float64); int(got) != 2 {
		t.Fatalf("expected count 2, got %#v", env.Data)
	}
	runs := requireJSONArrayField(t, env.Data, "runs")
	if len(runs) != 2 {
		t.Fatalf("expected two runs, got %#v", runs)
	}
	firstRun := mapStringAny(runs[0])
	secondRun := mapStringAny(runs[1])
	if firstRun["id"] != completed.ID || secondRun["id"] != failed.ID {
		t.Fatalf("expected completed then failed runs, got %#v", runs)
	}

	out, _, runErr = runCLI(t, dir, "cr", "delegate", "show", strconv.Itoa(cr.CR.ID), completed.ID, "--json")
	if runErr != nil {
		t.Fatalf("cr delegate show --json error = %v\noutput=%s", runErr, out)
	}
	env = decodeEnvelope(t, out)
	if !env.OK {
		t.Fatalf("expected ok envelope, got %#v", env)
	}
	run := mapStringAny(env.Data["run"])
	if got, _ := run["status"].(string); got != model.DelegationRunStatusCompleted {
		t.Fatalf("expected completed status, got %#v", run)
	}
	request := mapStringAny(run["request"])
	taskIDs := requireJSONArrayField(t, request, "task_ids")
	if len(taskIDs) != 1 || int(taskIDs[0].(float64)) != task.ID {
		t.Fatalf("expected selected task id %d, got %#v", task.ID, taskIDs)
	}
	result := mapStringAny(run["result"])
	if got, _ := result["status"].(string); got != model.DelegationRunStatusCompleted {
		t.Fatalf("expected completed result status, got %#v", result)
	}
	events := requireJSONArrayField(t, run, "events")
	if len(events) == 0 {
		t.Fatalf("expected persisted run events, got %#v", run)
	}
}

func TestCRDelegateShowTextIncludesResultSummary(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := service.New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	cr, err := svc.AddCRWithOptions("Delegation inspect fixture", "inspect blocked run", service.AddCROptions{NoSwitch: true})
	if err != nil {
		t.Fatalf("AddCRWithOptions() error = %v", err)
	}

	run, err := svc.StartDelegation(cr.CR.ID, model.DelegationRequest{
		Runtime: "mock",
		Metadata: map[string]string{
			"mock_outcome":  model.DelegationRunStatusBlocked,
			"mock_blockers": "needs operator input",
		},
	})
	if err != nil {
		t.Fatalf("StartDelegation(blocked) error = %v", err)
	}

	out, _, runErr := runCLI(t, dir, "cr", "delegate", "show", strconv.Itoa(cr.CR.ID), run.ID)
	if runErr != nil {
		t.Fatalf("cr delegate show error = %v\noutput=%s", runErr, out)
	}
	if !strings.Contains(out, "Status: blocked") {
		t.Fatalf("expected blocked status in output, got %q", out)
	}
	if !strings.Contains(out, "Blockers: needs operator input") {
		t.Fatalf("expected blocker summary in output, got %q", out)
	}
}
