//go:build integration
// +build integration

package cli

import (
	"bufio"
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"sophia/internal/model"
	"sophia/internal/service"
)

func TestBuildCRShowSnapshotIncludesDelegationHistory(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := service.New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	cr, err := svc.AddCRWithOptions("Delegation status snapshot", "include persisted runs in snapshot", service.AddCROptions{NoSwitch: true})
	if err != nil {
		t.Fatalf("AddCRWithOptions() error = %v", err)
	}

	running, err := svc.CreateDelegationRun(cr.CR.ID, model.DelegationRequest{Runtime: "mock"})
	if err != nil {
		t.Fatalf("CreateDelegationRun(running) error = %v", err)
	}
	if _, err := svc.AppendDelegationRunEvent(cr.CR.ID, running.ID, model.DelegationRunEvent{
		Kind:    model.DelegationEventKindRunStarted,
		Summary: "run started",
	}); err != nil {
		t.Fatalf("AppendDelegationRunEvent(run_started) error = %v", err)
	}
	completed, err := svc.CreateDelegationRun(cr.CR.ID, model.DelegationRequest{Runtime: "mock"})
	if err != nil {
		t.Fatalf("CreateDelegationRun(completed) error = %v", err)
	}
	if _, err := svc.AppendDelegationRunEvent(cr.CR.ID, completed.ID, model.DelegationRunEvent{
		Kind:    model.DelegationEventKindRunStarted,
		Summary: "completed run started",
	}); err != nil {
		t.Fatalf("AppendDelegationRunEvent(completed run_started) error = %v", err)
	}
	if _, err := svc.FinishDelegationRun(cr.CR.ID, completed.ID, model.DelegationResult{
		Status:  model.DelegationRunStatusCompleted,
		Summary: "completed successfully",
	}); err != nil {
		t.Fatalf("FinishDelegationRun(completed) error = %v", err)
	}
	failed, err := svc.CreateDelegationRun(cr.CR.ID, model.DelegationRequest{Runtime: "mock"})
	if err != nil {
		t.Fatalf("CreateDelegationRun(failed) error = %v", err)
	}
	if _, err := svc.AppendDelegationRunEvent(cr.CR.ID, failed.ID, model.DelegationRunEvent{
		Kind:    model.DelegationEventKindRunStarted,
		Summary: "failed run started",
	}); err != nil {
		t.Fatalf("AppendDelegationRunEvent(failed run_started) error = %v", err)
	}
	if _, err := svc.FinishDelegationRun(cr.CR.ID, failed.ID, model.DelegationResult{
		Status:  model.DelegationRunStatusFailed,
		Summary: "failed run",
	}); err != nil {
		t.Fatalf("FinishDelegationRun(failed) error = %v", err)
	}

	_, payload, err := buildCRShowSnapshot(svc, cr.CR.ID, defaultCRShowEventsLimit, defaultCRShowCheckpointsLimit)
	if err != nil {
		t.Fatalf("buildCRShowSnapshot() error = %v", err)
	}
	delegation := mapStringAny(payload["delegation"])
	counts := mapStringAny(delegation["counts"])
	if got := requireIntField(t, counts, "total"); got != 3 {
		t.Fatalf("expected 3 total runs, got %d", got)
	}
	if got := requireIntField(t, counts, "running"); got != 1 {
		t.Fatalf("expected 1 running run, got %d", got)
	}
	currentRun := mapStringAny(delegation["current_run"])
	if got, _ := currentRun["status"].(string); got != model.DelegationRunStatusRunning {
		t.Fatalf("expected current run to be running, got %#v", currentRun["status"])
	}
	recentRuns := requireObjectArrayField(t, delegation, "recent_runs")
	if len(recentRuns) != 3 {
		t.Fatalf("expected 3 recent runs, got %#v", recentRuns)
	}
}

func TestBuildCRShowSnapshotWithNoDelegationRuns(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := service.New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	cr, err := svc.AddCRWithOptions("Delegation empty snapshot", "no runs yet", service.AddCROptions{NoSwitch: true})
	if err != nil {
		t.Fatalf("AddCRWithOptions() error = %v", err)
	}

	_, payload, err := buildCRShowSnapshot(svc, cr.CR.ID, defaultCRShowEventsLimit, defaultCRShowCheckpointsLimit)
	if err != nil {
		t.Fatalf("buildCRShowSnapshot() error = %v", err)
	}
	delegation := mapStringAny(payload["delegation"])
	counts := mapStringAny(delegation["counts"])
	if got := requireIntField(t, counts, "total"); got != 0 {
		t.Fatalf("expected 0 total runs, got %d", got)
	}
	currentRun := mapStringAny(delegation["current_run"])
	if len(currentRun) != 0 {
		t.Fatalf("expected empty current run, got %#v", currentRun)
	}
	recentRuns := requireObjectArrayField(t, delegation, "recent_runs")
	if len(recentRuns) != 0 {
		t.Fatalf("expected no recent runs, got %#v", recentRuns)
	}
}

func requireIntField(t *testing.T, payload map[string]any, key string) int {
	t.Helper()
	value, ok := payload[key]
	if !ok {
		t.Fatalf("expected key %q in payload %#v", key, payload)
	}
	switch typed := value.(type) {
	case int:
		return typed
	case float64:
		return int(typed)
	default:
		t.Fatalf("expected numeric key %q, got %#v", key, value)
	}
	return 0
}

func requireObjectArrayField(t *testing.T, payload map[string]any, key string) []map[string]any {
	t.Helper()
	value, ok := payload[key]
	if !ok {
		t.Fatalf("expected key %q in payload %#v", key, payload)
	}
	switch typed := value.(type) {
	case []map[string]any:
		return typed
	case []any:
		out := make([]map[string]any, 0, len(typed))
		for _, item := range typed {
			obj, ok := item.(map[string]any)
			if !ok {
				t.Fatalf("expected %q array to contain objects, got %#v", key, item)
			}
			out = append(out, obj)
		}
		return out
	default:
		t.Fatalf("expected %q to be object array, got %#v", key, value)
	}
	return nil
}

func TestCRShowServerSSEEmitsOnDelegationStateChange(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := service.New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	cr, err := svc.AddCRWithOptions("Delegation live snapshot", "sse emits delegation changes", service.AddCROptions{NoSwitch: true})
	if err != nil {
		t.Fatalf("AddCRWithOptions() error = %v", err)
	}

	server, err := startCRShowServerWithLiveRoutesAndLaunch(
		func() (string, error) { return "<!doctype html><html><body>dashboard</body></html>", nil },
		func(id int) (string, error) {
			_, payload, snapshotErr := buildCRShowSnapshot(svc, id, defaultCRShowEventsLimit, defaultCRShowCheckpointsLimit)
			if snapshotErr != nil {
				return "", snapshotErr
			}
			return buildCRShowHTMLDocument(embeddedCRShowHTMLTemplate, payload)
		},
		nil,
		func(id int) (map[string]any, error) {
			_, payload, snapshotErr := buildCRShowSnapshot(svc, id, defaultCRShowEventsLimit, defaultCRShowCheckpointsLimit)
			if snapshotErr != nil {
				return nil, snapshotErr
			}
			return payload, nil
		},
		nil,
	)
	if err != nil {
		t.Fatalf("startCRShowServerWithLiveRoutesAndLaunch() error = %v", err)
	}
	defer server.Shutdown()

	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/__sophia_events?mode=cr&id=1", nil)
	if err != nil {
		t.Fatalf("NewRequestWithContext() error = %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /__sophia_events error = %v", err)
	}
	defer resp.Body.Close()

	reader := bufio.NewReader(resp.Body)
	_, firstData, firstErr := readSSEEvent(reader)
	if firstErr != nil {
		t.Fatalf("readSSEEvent(first) error = %v", firstErr)
	}
	if !strings.Contains(firstData, "\"total\":0") {
		t.Fatalf("expected initial delegation count 0, got %q", firstData)
	}

	go func() {
		time.Sleep(250 * time.Millisecond)
		run, createErr := svc.CreateDelegationRun(cr.CR.ID, model.DelegationRequest{Runtime: "mock"})
		if createErr != nil {
			return
		}
		_, _ = svc.AppendDelegationRunEvent(cr.CR.ID, run.ID, model.DelegationRunEvent{
			Kind:    model.DelegationEventKindRunStarted,
			Summary: "streamed run started",
		})
	}()

	_, secondData, secondErr := readSSEEvent(reader)
	if secondErr != nil {
		t.Fatalf("readSSEEvent(second) error = %v", secondErr)
	}
	if !strings.Contains(secondData, "\"total\":1") {
		t.Fatalf("expected updated delegation count 1, got %q", secondData)
	}
	if !strings.Contains(secondData, "\"status\":\"running\"") {
		t.Fatalf("expected running delegation in updated snapshot, got %q", secondData)
	}
}
