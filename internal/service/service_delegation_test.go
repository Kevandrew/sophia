package service

import (
	"testing"

	"sophia/internal/model"
)

func TestCreateDelegationRunDefaultsIntentSnapshotAndQueuedStatus(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	cr, err := svc.AddCRWithOptions("Delegation fixture", "seed delegation run", AddCROptions{NoSwitch: true})
	if err != nil {
		t.Fatalf("AddCRWithOptions() error = %v", err)
	}
	task, err := svc.AddTask(cr.CR.ID, "delegate me")
	if err != nil {
		t.Fatalf("AddTask() error = %v", err)
	}

	run, err := svc.CreateDelegationRun(cr.CR.ID, model.DelegationRequest{
		Runtime: "mock",
		TaskIDs: []int{task.ID, task.ID},
	})
	if err != nil {
		t.Fatalf("CreateDelegationRun() error = %v", err)
	}
	if run.Status != model.DelegationRunStatusQueued {
		t.Fatalf("expected queued status, got %#v", run)
	}
	if run.Request.IntentSnapshot == nil || run.Request.IntentSnapshot.Title != cr.CR.Title {
		t.Fatalf("expected default intent snapshot, got %#v", run.Request.IntentSnapshot)
	}
	if len(run.Request.TaskIDs) != 1 || run.Request.TaskIDs[0] != task.ID {
		t.Fatalf("expected normalized task ids, got %#v", run.Request.TaskIDs)
	}

	reloaded, err := svc.store.LoadCR(cr.CR.ID)
	if err != nil {
		t.Fatalf("LoadCR() error = %v", err)
	}
	if len(reloaded.DelegationRuns) != 1 {
		t.Fatalf("expected persisted delegation run, got %#v", reloaded.DelegationRuns)
	}
}

func TestCreateDelegationRunRejectsUnknownTaskID(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	cr, err := svc.AddCRWithOptions("Delegation fixture", "seed delegation run", AddCROptions{NoSwitch: true})
	if err != nil {
		t.Fatalf("AddCRWithOptions() error = %v", err)
	}

	if _, err := svc.CreateDelegationRun(cr.CR.ID, model.DelegationRequest{
		Runtime: "mock",
		TaskIDs: []int{99},
	}); err == nil {
		t.Fatalf("expected unknown task id error")
	}
}

func TestCreateDelegationRunRejectsNonPositiveTaskID(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	cr, err := svc.AddCRWithOptions("Delegation fixture", "seed delegation run", AddCROptions{NoSwitch: true})
	if err != nil {
		t.Fatalf("AddCRWithOptions() error = %v", err)
	}

	if _, err := svc.CreateDelegationRun(cr.CR.ID, model.DelegationRequest{
		Runtime: "mock",
		TaskIDs: []int{0},
	}); err == nil {
		t.Fatalf("expected non-positive task id error")
	}
}

func TestAppendDelegationRunEventTransitionsRunToRunning(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	cr, err := svc.AddCRWithOptions("Delegation fixture", "seed delegation run", AddCROptions{NoSwitch: true})
	if err != nil {
		t.Fatalf("AddCRWithOptions() error = %v", err)
	}
	run, err := svc.CreateDelegationRun(cr.CR.ID, model.DelegationRequest{Runtime: "mock"})
	if err != nil {
		t.Fatalf("CreateDelegationRun() error = %v", err)
	}

	updated, err := svc.AppendDelegationRunEvent(cr.CR.ID, run.ID, model.DelegationRunEvent{
		Kind:    model.DelegationEventKindRunStarted,
		Summary: "worker started",
	})
	if err != nil {
		t.Fatalf("AppendDelegationRunEvent() error = %v", err)
	}
	if updated.Status != model.DelegationRunStatusRunning {
		t.Fatalf("expected running status, got %#v", updated)
	}
	if len(updated.Events) != 1 || updated.Events[0].ID != 1 {
		t.Fatalf("expected normalized event id, got %#v", updated.Events)
	}
}

func TestAppendDelegationRunEventRejectsQueuedNonStartEvent(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	cr, err := svc.AddCRWithOptions("Delegation fixture", "seed delegation run", AddCROptions{NoSwitch: true})
	if err != nil {
		t.Fatalf("AddCRWithOptions() error = %v", err)
	}
	run, err := svc.CreateDelegationRun(cr.CR.ID, model.DelegationRequest{Runtime: "mock"})
	if err != nil {
		t.Fatalf("CreateDelegationRun() error = %v", err)
	}

	if _, err := svc.AppendDelegationRunEvent(cr.CR.ID, run.ID, model.DelegationRunEvent{
		Kind:    model.DelegationEventKindMessage,
		Message: "premature output",
	}); err == nil {
		t.Fatalf("expected queued non-start event to fail")
	}
}

func TestFinishDelegationRunStoresTerminalResult(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	cr, err := svc.AddCRWithOptions("Delegation fixture", "seed delegation run", AddCROptions{NoSwitch: true})
	if err != nil {
		t.Fatalf("AddCRWithOptions() error = %v", err)
	}
	run, err := svc.CreateDelegationRun(cr.CR.ID, model.DelegationRequest{Runtime: "mock"})
	if err != nil {
		t.Fatalf("CreateDelegationRun() error = %v", err)
	}
	if _, err := svc.AppendDelegationRunEvent(cr.CR.ID, run.ID, model.DelegationRunEvent{
		Kind:    model.DelegationEventKindRunStarted,
		Summary: "worker started",
	}); err != nil {
		t.Fatalf("AppendDelegationRunEvent(start) error = %v", err)
	}

	finished, err := svc.FinishDelegationRun(cr.CR.ID, run.ID, model.DelegationResult{
		Status:       model.DelegationRunStatusCompleted,
		Summary:      "done",
		FilesChanged: []string{"internal/service/service_delegation.go"},
	})
	if err != nil {
		t.Fatalf("FinishDelegationRun() error = %v", err)
	}
	if finished.Result == nil || finished.Result.Status != model.DelegationRunStatusCompleted {
		t.Fatalf("expected completed result, got %#v", finished.Result)
	}
	if finished.FinishedAt == "" {
		t.Fatalf("expected finished_at to be set, got %#v", finished)
	}

	loaded, err := svc.GetDelegationRun(cr.CR.ID, run.ID)
	if err != nil {
		t.Fatalf("GetDelegationRun() error = %v", err)
	}
	if loaded.Result == nil || len(loaded.Result.FilesChanged) != 1 {
		t.Fatalf("expected persisted result, got %#v", loaded.Result)
	}
}

func TestDelegationRunReadsUseLifecycleStoreOverride(t *testing.T) {
	t.Parallel()
	cr := seedCR(42, "Delegation fixture", seedCROptions{Branch: "cr-42-delegation"})
	cr.DelegationRuns = []model.DelegationRun{
		{
			ID:        "dr_seeded",
			Status:    model.DelegationRunStatusRunning,
			Request:   model.DelegationRequest{Runtime: "mock"},
			Events:    []model.DelegationRunEvent{{ID: 1, TS: harnessTimestamp, Kind: model.DelegationEventKindRunStarted, Summary: "started"}},
			CreatedAt: harnessTimestamp,
			CreatedBy: "Runtime Tester <runtime@test>",
			UpdatedAt: harnessTimestamp,
		},
	}

	h := harnessService(t, runtimeHarnessOptions{Branch: cr.Branch, CRs: []*model.CR{cr}})

	run, err := h.Service.GetDelegationRun(cr.ID, "dr_seeded")
	if err != nil {
		t.Fatalf("GetDelegationRun() error = %v", err)
	}
	if run.ID != "dr_seeded" {
		t.Fatalf("expected seeded run id, got %#v", run)
	}

	runs, err := h.Service.ListDelegationRuns(cr.ID)
	if err != nil {
		t.Fatalf("ListDelegationRuns() error = %v", err)
	}
	if len(runs) != 1 || runs[0].ID != "dr_seeded" {
		t.Fatalf("expected seeded runs from lifecycle store override, got %#v", runs)
	}
	if got := h.Store.Calls("LoadCR"); got < 2 {
		t.Fatalf("expected lifecycle store override to service reads, got %d LoadCR calls", got)
	}
}

func TestFinishDelegationRunRejectsQueuedCompletion(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	cr, err := svc.AddCRWithOptions("Delegation fixture", "seed delegation run", AddCROptions{NoSwitch: true})
	if err != nil {
		t.Fatalf("AddCRWithOptions() error = %v", err)
	}
	run, err := svc.CreateDelegationRun(cr.CR.ID, model.DelegationRequest{Runtime: "mock"})
	if err != nil {
		t.Fatalf("CreateDelegationRun() error = %v", err)
	}

	if _, err := svc.FinishDelegationRun(cr.CR.ID, run.ID, model.DelegationResult{
		Status:  model.DelegationRunStatusCompleted,
		Summary: "done",
	}); err == nil {
		t.Fatalf("expected queued completion to fail")
	}
}

func TestFinishDelegationRunAllowsQueuedCancellation(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	cr, err := svc.AddCRWithOptions("Delegation fixture", "seed delegation run", AddCROptions{NoSwitch: true})
	if err != nil {
		t.Fatalf("AddCRWithOptions() error = %v", err)
	}
	run, err := svc.CreateDelegationRun(cr.CR.ID, model.DelegationRequest{Runtime: "mock"})
	if err != nil {
		t.Fatalf("CreateDelegationRun() error = %v", err)
	}

	finished, err := svc.FinishDelegationRun(cr.CR.ID, run.ID, model.DelegationResult{
		Status:  model.DelegationRunStatusCancelled,
		Summary: "cancelled before start",
	})
	if err != nil {
		t.Fatalf("FinishDelegationRun(cancelled) error = %v", err)
	}
	if finished.Status != model.DelegationRunStatusCancelled {
		t.Fatalf("expected cancelled status, got %#v", finished)
	}
}

func TestAppendDelegationRunEventRejectsTerminalRun(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	cr, err := svc.AddCRWithOptions("Delegation fixture", "seed delegation run", AddCROptions{NoSwitch: true})
	if err != nil {
		t.Fatalf("AddCRWithOptions() error = %v", err)
	}
	run, err := svc.CreateDelegationRun(cr.CR.ID, model.DelegationRequest{Runtime: "mock"})
	if err != nil {
		t.Fatalf("CreateDelegationRun() error = %v", err)
	}
	if _, err := svc.AppendDelegationRunEvent(cr.CR.ID, run.ID, model.DelegationRunEvent{
		Kind:    model.DelegationEventKindRunStarted,
		Summary: "worker started",
	}); err != nil {
		t.Fatalf("AppendDelegationRunEvent(start) error = %v", err)
	}
	if _, err := svc.FinishDelegationRun(cr.CR.ID, run.ID, model.DelegationResult{
		Status:  model.DelegationRunStatusFailed,
		Summary: "failed",
	}); err != nil {
		t.Fatalf("FinishDelegationRun() error = %v", err)
	}

	if _, err := svc.AppendDelegationRunEvent(cr.CR.ID, run.ID, model.DelegationRunEvent{
		Kind:    model.DelegationEventKindMessage,
		Message: "too late",
	}); err == nil {
		t.Fatalf("expected append on terminal run to fail")
	}
}
