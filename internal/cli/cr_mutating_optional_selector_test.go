package cli

import (
	"os"
	"path/filepath"
	"testing"

	"sophia/internal/service"
)

func TestMutatingCommandsResolveActiveCRWhenSelectorOmitted(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := service.New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	cr, err := svc.AddCR("mutating optional selector", "active branch fallback")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}

	noteOut, _, noteErr := runCLI(t, dir, "cr", "note", "note from active context", "--json")
	if noteErr != nil {
		t.Fatalf("cr note --json error = %v\noutput=%s", noteErr, noteOut)
	}
	noteEnv := decodeEnvelope(t, noteOut)
	if !noteEnv.OK {
		t.Fatalf("expected ok note envelope, got %#v", noteEnv)
	}
	if got := mutatingOptionalSelectorIntField(t, noteEnv.Data, "cr_id"); got != cr.ID {
		t.Fatalf("expected note cr_id %d, got %d", cr.ID, got)
	}

	editOut, _, editErr := runCLI(t, dir, "cr", "edit", "--title", "mutating optional selector updated", "--json")
	if editErr != nil {
		t.Fatalf("cr edit --json error = %v\noutput=%s", editErr, editOut)
	}
	editEnv := decodeEnvelope(t, editOut)
	if !editEnv.OK {
		t.Fatalf("expected ok edit envelope, got %#v", editEnv)
	}
	if got := mutatingOptionalSelectorIntField(t, editEnv.Data, "cr_id"); got != cr.ID {
		t.Fatalf("expected edit cr_id %d, got %d", cr.ID, got)
	}

	contractOut, _, contractErr := runCLI(t, dir, "cr", "contract", "set", "--why", "updated through optional selector", "--json")
	if contractErr != nil {
		t.Fatalf("cr contract set --json error = %v\noutput=%s", contractErr, contractOut)
	}
	contractEnv := decodeEnvelope(t, contractOut)
	if !contractEnv.OK {
		t.Fatalf("expected ok contract envelope, got %#v", contractEnv)
	}
	if got := mutatingOptionalSelectorIntField(t, contractEnv.Data, "cr_id"); got != cr.ID {
		t.Fatalf("expected contract cr_id %d, got %d", cr.ID, got)
	}

	taskAddOut, _, taskAddErr := runCLI(t, dir, "cr", "task", "add", "task via active context", "--json")
	if taskAddErr != nil {
		t.Fatalf("cr task add --json error = %v\noutput=%s", taskAddErr, taskAddOut)
	}
	taskAddEnv := decodeEnvelope(t, taskAddOut)
	if !taskAddEnv.OK {
		t.Fatalf("expected ok task add envelope, got %#v", taskAddEnv)
	}
	if got := mutatingOptionalSelectorIntField(t, taskAddEnv.Data, "cr_id"); got != cr.ID {
		t.Fatalf("expected task add cr_id %d, got %d", cr.ID, got)
	}
	taskMap, ok := taskAddEnv.Data["task"].(map[string]any)
	if !ok {
		t.Fatalf("expected task object, got %#v", taskAddEnv.Data["task"])
	}
	taskID := mutatingOptionalSelectorIntField(t, taskMap, "id")

	taskContractOut, _, taskContractErr := runCLI(
		t,
		dir,
		"cr",
		"task",
		"contract",
		"set",
		"1",
		"--intent", "task intent",
		"--acceptance", "task acceptance",
		"--scope", "task.txt",
		"--json",
	)
	if taskContractErr != nil {
		t.Fatalf("cr task contract set --json error = %v\noutput=%s", taskContractErr, taskContractOut)
	}
	taskContractEnv := decodeEnvelope(t, taskContractOut)
	if !taskContractEnv.OK {
		t.Fatalf("expected ok task contract envelope, got %#v", taskContractEnv)
	}
	if got := mutatingOptionalSelectorIntField(t, taskContractEnv.Data, "cr_id"); got != cr.ID {
		t.Fatalf("expected task contract cr_id %d, got %d", cr.ID, got)
	}
	if gotTask := mutatingOptionalSelectorIntField(t, taskContractEnv.Data, "task_id"); gotTask != taskID {
		t.Fatalf("expected task contract task_id %d, got %d", taskID, gotTask)
	}

	if err := os.WriteFile(filepath.Join(dir, "task.txt"), []byte("task checkpoint\n"), 0o644); err != nil {
		t.Fatalf("write task.txt error = %v", err)
	}
	taskDoneOut, _, taskDoneErr := runCLI(t, dir, "cr", "task", "done", "1", "--path", "task.txt", "--json")
	if taskDoneErr != nil {
		t.Fatalf("cr task done --json error = %v\noutput=%s", taskDoneErr, taskDoneOut)
	}
	taskDoneEnv := decodeEnvelope(t, taskDoneOut)
	if !taskDoneEnv.OK {
		t.Fatalf("expected ok task done envelope, got %#v", taskDoneEnv)
	}
	if got := mutatingOptionalSelectorIntField(t, taskDoneEnv.Data, "cr_id"); got != cr.ID {
		t.Fatalf("expected task done cr_id %d, got %d", cr.ID, got)
	}
	if gotTask := mutatingOptionalSelectorIntField(t, taskDoneEnv.Data, "task_id"); gotTask != taskID {
		t.Fatalf("expected task done task_id %d, got %d", taskID, gotTask)
	}

	reopenOut, _, reopenErr := runCLI(t, dir, "cr", "task", "reopen", "1", "--json")
	if reopenErr != nil {
		t.Fatalf("cr task reopen --json error = %v\noutput=%s", reopenErr, reopenOut)
	}
	reopenEnv := decodeEnvelope(t, reopenOut)
	if !reopenEnv.OK {
		t.Fatalf("expected ok task reopen envelope, got %#v", reopenEnv)
	}
	if got := mutatingOptionalSelectorIntField(t, reopenEnv.Data, "cr_id"); got != cr.ID {
		t.Fatalf("expected task reopen cr_id %d, got %d", cr.ID, got)
	}
	if gotTask := mutatingOptionalSelectorIntField(t, reopenEnv.Data, "task_id"); gotTask != taskID {
		t.Fatalf("expected task reopen task_id %d, got %d", taskID, gotTask)
	}

	evidenceOut, _, evidenceErr := runCLI(
		t,
		dir,
		"cr",
		"evidence",
		"add",
		"--type", "manual_note",
		"--summary", "evidence via active context",
		"--json",
	)
	if evidenceErr != nil {
		t.Fatalf("cr evidence add --json error = %v\noutput=%s", evidenceErr, evidenceOut)
	}
	evidenceEnv := decodeEnvelope(t, evidenceOut)
	if !evidenceEnv.OK {
		t.Fatalf("expected ok evidence envelope, got %#v", evidenceEnv)
	}
	if got := mutatingOptionalSelectorIntField(t, evidenceEnv.Data, "cr_id"); got != cr.ID {
		t.Fatalf("expected evidence cr_id %d, got %d", cr.ID, got)
	}
}

func TestMutatingCommandsWithoutSelectorReturnNoActiveCRContext(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := service.New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if _, _, err := svc.AddCRWithOptionsWithWarnings("mutating optional selector", "no active context", service.AddCROptions{NoSwitch: true}); err != nil {
		t.Fatalf("AddCRWithOptionsWithWarnings() error = %v", err)
	}

	cases := []struct {
		name string
		args []string
	}{
		{name: "note", args: []string{"cr", "note", "note text", "--json"}},
		{name: "edit", args: []string{"cr", "edit", "--title", "updated", "--json"}},
		{name: "contract set", args: []string{"cr", "contract", "set", "--why", "updated", "--json"}},
		{name: "task add", args: []string{"cr", "task", "add", "task title", "--json"}},
		{name: "task contract set", args: []string{"cr", "task", "contract", "set", "1", "--intent", "intent", "--acceptance", "accept", "--scope", "task.txt", "--json"}},
		{name: "task done", args: []string{"cr", "task", "done", "1", "--all", "--json"}},
		{name: "task reopen", args: []string{"cr", "task", "reopen", "1", "--json"}},
		{name: "evidence add", args: []string{"cr", "evidence", "add", "--type", "manual_note", "--summary", "summary", "--json"}},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			out, _, runErr := runCLI(t, dir, tc.args...)
			if runErr == nil {
				t.Fatalf("expected %s to fail without active CR context", tc.name)
			}
			env := decodeEnvelope(t, out)
			if env.OK || env.Error == nil {
				t.Fatalf("expected structured error envelope for %s, got %#v", tc.name, env)
			}
			if env.Error.Code != "no_active_cr_context" {
				t.Fatalf("expected no_active_cr_context for %s, got %q", tc.name, env.Error.Code)
			}
		})
	}
}

func mutatingOptionalSelectorIntField(t *testing.T, data map[string]any, key string) int {
	t.Helper()
	raw, ok := data[key]
	if !ok {
		t.Fatalf("missing key %q in %#v", key, data)
	}
	value, ok := raw.(float64)
	if !ok {
		t.Fatalf("expected numeric key %q, got %#v", key, raw)
	}
	return int(value)
}
