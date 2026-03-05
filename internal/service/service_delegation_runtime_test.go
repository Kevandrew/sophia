package service

import (
	"testing"

	"sophia/internal/model"
)

type spyDelegationRuntime struct {
	startCalls   int
	cancelCalls  int
	cancelReason string
}

func (s *spyDelegationRuntime) Start(_ DelegationRuntimeRunContext, reporter DelegationRuntimeReporter) error {
	s.startCalls++
	return reporter.Event(DelegationRuntimeProgress{
		Kind:    model.DelegationEventKindRunStarted,
		Summary: "spy runtime started",
	})
}

func (s *spyDelegationRuntime) Cancel(_ DelegationRuntimeRunContext, reason string) error {
	s.cancelCalls++
	s.cancelReason = reason
	return nil
}

func TestStartDelegationUsesMockRuntimeAndPersistsCompletedResult(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	cr, err := svc.AddCRWithOptions("Delegation runtime fixture", "exercise runtime contract", AddCROptions{NoSwitch: true})
	if err != nil {
		t.Fatalf("AddCRWithOptions() error = %v", err)
	}
	task, err := svc.AddTask(cr.CR.ID, "runtime task")
	if err != nil {
		t.Fatalf("AddTask() error = %v", err)
	}

	run, err := svc.StartDelegation(cr.CR.ID, model.DelegationRequest{
		Runtime: "mock",
		TaskIDs: []int{task.ID},
		Metadata: map[string]string{
			"mock_steps":         "hydrate context,emit progress",
			"mock_files_changed": "internal/service/service_delegation_runtime.go,internal/service/service_delegation_runtime_test.go",
		},
	})
	if err != nil {
		t.Fatalf("StartDelegation() error = %v", err)
	}
	if run.Status != model.DelegationRunStatusCompleted {
		t.Fatalf("expected completed run, got %#v", run)
	}
	if run.Result == nil || len(run.Result.FilesChanged) != 2 {
		t.Fatalf("expected completed result with files changed, got %#v", run.Result)
	}
	if len(run.Events) < 6 {
		t.Fatalf("expected progress events to be recorded, got %#v", run.Events)
	}
	if got := run.Events[0].Kind; got != model.DelegationEventKindRunStarted {
		t.Fatalf("expected first event kind %q, got %q", model.DelegationEventKindRunStarted, got)
	}
	if got := run.Events[len(run.Events)-1].Kind; got != model.DelegationEventKindRunCompleted {
		t.Fatalf("expected terminal progress event %q, got %q", model.DelegationEventKindRunCompleted, got)
	}

	reloaded, err := svc.GetDelegationRun(cr.CR.ID, run.ID)
	if err != nil {
		t.Fatalf("GetDelegationRun() error = %v", err)
	}
	if reloaded.Result == nil || reloaded.Result.Status != model.DelegationRunStatusCompleted {
		t.Fatalf("expected persisted completed result, got %#v", reloaded.Result)
	}
}

func TestStartDelegationSupportsBlockedMockOutcome(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	cr, err := svc.AddCRWithOptions("Delegation runtime fixture", "exercise blocked runtime contract", AddCROptions{NoSwitch: true})
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
		t.Fatalf("StartDelegation() error = %v", err)
	}
	if run.Status != model.DelegationRunStatusBlocked {
		t.Fatalf("expected blocked run, got %#v", run)
	}
	if run.Result == nil || len(run.Result.Blockers) != 1 || run.Result.Blockers[0] != "needs operator input" {
		t.Fatalf("expected blocker result, got %#v", run.Result)
	}
	foundBlocked := false
	for _, event := range run.Events {
		if event.Kind == model.DelegationEventKindBlocked {
			foundBlocked = true
			break
		}
	}
	if !foundBlocked {
		t.Fatalf("expected blocked progress event, got %#v", run.Events)
	}
}

func TestStartDelegationRejectsUnknownRuntimeWithoutPersistingRun(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	cr, err := svc.AddCRWithOptions("Delegation runtime fixture", "exercise unknown runtime", AddCROptions{NoSwitch: true})
	if err != nil {
		t.Fatalf("AddCRWithOptions() error = %v", err)
	}

	if _, err := svc.StartDelegation(cr.CR.ID, model.DelegationRequest{Runtime: "missing"}); err == nil {
		t.Fatalf("expected unknown runtime error")
	}
	runs, err := svc.ListDelegationRuns(cr.CR.ID)
	if err != nil {
		t.Fatalf("ListDelegationRuns() error = %v", err)
	}
	if len(runs) != 0 {
		t.Fatalf("expected no persisted runs for rejected runtime, got %#v", runs)
	}
}

func TestCancelDelegationUsesRuntimeContract(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	cr, err := svc.AddCRWithOptions("Delegation runtime fixture", "exercise cancellation path", AddCROptions{NoSwitch: true})
	if err != nil {
		t.Fatalf("AddCRWithOptions() error = %v", err)
	}
	spy := &spyDelegationRuntime{}
	svc.overrideDelegationRuntimesForTests(map[string]DelegationRuntime{
		"spy": spy,
	})

	run, err := svc.StartDelegation(cr.CR.ID, model.DelegationRequest{Runtime: "spy"})
	if err != nil {
		t.Fatalf("StartDelegation() error = %v", err)
	}
	if run.Status != model.DelegationRunStatusRunning {
		t.Fatalf("expected running run before cancel, got %#v", run)
	}

	cancelled, err := svc.CancelDelegation(cr.CR.ID, run.ID, "operator requested stop")
	if err != nil {
		t.Fatalf("CancelDelegation() error = %v", err)
	}
	if spy.startCalls != 1 {
		t.Fatalf("expected one runtime start call, got %d", spy.startCalls)
	}
	if spy.cancelCalls != 1 || spy.cancelReason != "operator requested stop" {
		t.Fatalf("expected cancel call with reason, got calls=%d reason=%q", spy.cancelCalls, spy.cancelReason)
	}
	if cancelled.Status != model.DelegationRunStatusCancelled {
		t.Fatalf("expected cancelled run, got %#v", cancelled)
	}
	if cancelled.Result == nil || cancelled.Result.Summary != "operator requested stop" {
		t.Fatalf("expected cancelled result summary, got %#v", cancelled.Result)
	}
}
