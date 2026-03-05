package cli

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"

	"sophia/internal/model"
	"sophia/internal/service"
)

func TestBuildCRShowSnapshotIncludesDelegationLaunchPayload(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := service.New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	cr, err := svc.AddCRWithOptions("Delegation launch snapshot", "preview launch payload", service.AddCROptions{NoSwitch: true})
	if err != nil {
		t.Fatalf("AddCRWithOptions() error = %v", err)
	}
	taskOne, err := svc.AddTask(cr.CR.ID, "first task")
	if err != nil {
		t.Fatalf("AddTask(first) error = %v", err)
	}
	taskTwo, err := svc.AddTask(cr.CR.ID, "second task")
	if err != nil {
		t.Fatalf("AddTask(second) error = %v", err)
	}

	_, payload, err := buildCRShowSnapshot(svc, cr.CR.ID, defaultCRShowEventsLimit, defaultCRShowCheckpointsLimit)
	if err != nil {
		t.Fatalf("buildCRShowSnapshot() error = %v", err)
	}
	launch := mapStringAny(payload["delegation_launch"])
	if available, _ := launch["available"].(bool); !available {
		t.Fatalf("expected available launch payload, got %#v", launch)
	}
	if runtime, _ := launch["runtime"].(string); runtime != defaultCRShowDelegationRuntime {
		t.Fatalf("expected runtime %q, got %#v", defaultCRShowDelegationRuntime, launch["runtime"])
	}
	defaultTaskIDs := requireIntSliceField(t, launch, "default_task_ids")
	if len(defaultTaskIDs) != 2 {
		t.Fatalf("expected 2 default task ids, got %#v", defaultTaskIDs)
	}
	if got := defaultTaskIDs[0]; got != taskOne.ID {
		t.Fatalf("expected first default task id %d, got %d", taskOne.ID, got)
	}
	if got := defaultTaskIDs[1]; got != taskTwo.ID {
		t.Fatalf("expected second default task id %d, got %d", taskTwo.ID, got)
	}
}

func requireIntSliceField(t *testing.T, payload map[string]any, key string) []int {
	t.Helper()
	value, ok := payload[key]
	if !ok {
		t.Fatalf("expected key %q in payload %#v", key, payload)
	}
	switch typed := value.(type) {
	case []int:
		return append([]int(nil), typed...)
	case []any:
		out := make([]int, 0, len(typed))
		for _, item := range typed {
			switch v := item.(type) {
			case float64:
				out = append(out, int(v))
			case int:
				out = append(out, v)
			default:
				t.Fatalf("expected %q array to contain numeric values, got %#v", key, item)
			}
		}
		return out
	default:
		t.Fatalf("expected %q to be int array, got %#v", key, value)
	}
	return nil
}

func TestCRShowLaunchRouteStartsDelegationRun(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := service.New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	cr, err := svc.AddCRWithOptions("Delegation launch route", "launch mock runtime from localhost preview", service.AddCROptions{NoSwitch: true})
	if err != nil {
		t.Fatalf("AddCRWithOptions() error = %v", err)
	}
	task, err := svc.AddTask(cr.CR.ID, "launch target")
	if err != nil {
		t.Fatalf("AddTask() error = %v", err)
	}
	view, _, err := buildCRShowSnapshot(svc, cr.CR.ID, defaultCRShowEventsLimit, defaultCRShowCheckpointsLimit)
	if err != nil {
		t.Fatalf("buildCRShowSnapshot() error = %v", err)
	}

	server, err := startCRShowServerWithLiveRoutesAndLaunch(
		func() (string, error) { return "<!doctype html><html><body>ok</body></html>", nil },
		nil,
		nil,
		nil,
		buildCRShowPerCRLaunchHandler(svc, view),
	)
	if err != nil {
		t.Fatalf("startCRShowServerWithLiveRoutesAndLaunch() error = %v", err)
	}
	defer server.Shutdown()

	body := bytes.NewBufferString(`{"cr_id":1}`)
	resp, err := http.Post(server.URL+"/__sophia_delegate_launch", "application/json", body)
	if err != nil {
		t.Fatalf("POST /__sophia_delegate_launch error = %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("expected status %d, got %d", http.StatusAccepted, resp.StatusCode)
	}
	var envelope envelope
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !envelope.OK {
		t.Fatalf("expected ok envelope, got %#v", envelope)
	}
	data := envelope.Data
	if runtime, _ := data["runtime"].(string); runtime != defaultCRShowDelegationRuntime {
		t.Fatalf("expected runtime %q, got %#v", defaultCRShowDelegationRuntime, data["runtime"])
	}
	selectedTaskIDs := requireJSONArrayField(t, data, "selected_task_ids")
	if len(selectedTaskIDs) != 1 || int(selectedTaskIDs[0].(float64)) != task.ID {
		t.Fatalf("expected selected task id %d, got %#v", task.ID, selectedTaskIDs)
	}
	run := mapStringAny(data["run"])
	if status, _ := run["status"].(string); status != model.DelegationRunStatusCompleted {
		t.Fatalf("expected completed run, got %#v", run)
	}

	runs, err := svc.ListDelegationRuns(cr.CR.ID)
	if err != nil {
		t.Fatalf("ListDelegationRuns() error = %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("expected one persisted delegation run, got %#v", runs)
	}
}

func TestCRShowLaunchRouteRejectsMismatchedCRID(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := service.New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	cr, err := svc.AddCRWithOptions("Delegation launch mismatch", "reject mismatched cr ids", service.AddCROptions{NoSwitch: true})
	if err != nil {
		t.Fatalf("AddCRWithOptions() error = %v", err)
	}
	view, _, err := buildCRShowSnapshot(svc, cr.CR.ID, defaultCRShowEventsLimit, defaultCRShowCheckpointsLimit)
	if err != nil {
		t.Fatalf("buildCRShowSnapshot() error = %v", err)
	}

	server, err := startCRShowServerWithLiveRoutesAndLaunch(
		func() (string, error) { return "<!doctype html><html><body>ok</body></html>", nil },
		nil,
		nil,
		nil,
		buildCRShowPerCRLaunchHandler(svc, view),
	)
	if err != nil {
		t.Fatalf("startCRShowServerWithLiveRoutesAndLaunch() error = %v", err)
	}
	defer server.Shutdown()

	resp, err := http.Post(server.URL+"/__sophia_delegate_launch", "application/json", bytes.NewBufferString(`{"cr_id":999}`))
	if err != nil {
		t.Fatalf("POST /__sophia_delegate_launch error = %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, resp.StatusCode)
	}
}

func TestCRShowLaunchRouteRejectsUnavailableCR(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := service.New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	cr, err := svc.AddCRWithOptions("Delegation launch unavailable", "reject abandoned cr launch", service.AddCROptions{Switch: true})
	if err != nil {
		t.Fatalf("AddCRWithOptions() error = %v", err)
	}
	if _, err := svc.AbandonCR(cr.CR.ID, service.CRAbandonOptions{Reason: "deferred"}); err != nil {
		t.Fatalf("AbandonCR() error = %v", err)
	}
	view, _, err := buildCRShowSnapshot(svc, cr.CR.ID, defaultCRShowEventsLimit, defaultCRShowCheckpointsLimit)
	if err != nil {
		t.Fatalf("buildCRShowSnapshot() error = %v", err)
	}

	server, err := startCRShowServerWithLiveRoutesAndLaunch(
		func() (string, error) { return "<!doctype html><html><body>ok</body></html>", nil },
		nil,
		nil,
		nil,
		buildCRShowPerCRLaunchHandler(svc, view),
	)
	if err != nil {
		t.Fatalf("startCRShowServerWithLiveRoutesAndLaunch() error = %v", err)
	}
	defer server.Shutdown()

	resp, err := http.Post(server.URL+"/__sophia_delegate_launch", "application/json", bytes.NewBufferString(`{"cr_id":1}`))
	if err != nil {
		t.Fatalf("POST /__sophia_delegate_launch error = %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("expected status %d, got %d", http.StatusConflict, resp.StatusCode)
	}
	var envelope envelope
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if envelope.OK {
		t.Fatalf("expected failed envelope, got %#v", envelope)
	}
	if envelope.Error == nil || envelope.Error.Message == "" {
		t.Fatalf("expected error message, got %#v", envelope.Error)
	}
}
