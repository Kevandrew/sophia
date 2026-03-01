package cli

import (
	"strings"
	"testing"

	"sophia/internal/service"
)

func TestMissingArgTextErrorShowsUsageAndExample(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	_, stderr, runErr := runCLI(t, dir, "cr", "task", "done")
	if runErr == nil {
		t.Fatalf("expected missing-arg command to fail")
	}
	if !strings.Contains(stderr, "Usage:") {
		t.Fatalf("expected usage block in stderr, got %q", stderr)
	}
	if !strings.Contains(stderr, "Example:") {
		t.Fatalf("expected example block in stderr, got %q", stderr)
	}
	if !strings.Contains(stderr, "sophia cr task done --help") {
		t.Fatalf("expected concrete next-step help invocation, got %q", stderr)
	}
}

func TestMissingArgJSONErrorIsStructuredAndActionable(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	out, _, runErr := runCLI(t, dir, "cr", "task", "done", "--json")
	if runErr == nil {
		t.Fatalf("expected missing-arg JSON command to fail")
	}
	env := decodeEnvelope(t, out)
	if env.OK {
		t.Fatalf("expected non-ok JSON envelope, got %#v", env)
	}
	if env.Error == nil || env.Error.Code != "invalid_argument" {
		t.Fatalf("expected invalid_argument JSON code, got %#v", env.Error)
	}
	action, _ := env.Error.Details["suggested_action"].(string)
	if strings.TrimSpace(action) == "" || !strings.Contains(action, "--help") {
		t.Fatalf("expected suggested_action with --help, got %#v", env.Error.Details)
	}
}

func TestMissingArgJSONErrorWithEqualsFlagIsStructured(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	out, _, runErr := runCLI(t, dir, "cr", "task", "done", "--json=true")
	if runErr == nil {
		t.Fatalf("expected missing-arg JSON command to fail")
	}
	env := decodeEnvelope(t, out)
	if env.OK {
		t.Fatalf("expected non-ok JSON envelope, got %#v", env)
	}
	if env.Error == nil || env.Error.Code != "invalid_argument" {
		t.Fatalf("expected invalid_argument JSON code, got %#v", env.Error)
	}
}

func TestRuntimeTextErrorsDoNotDumpUsageNoise(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	_, stderr, runErr := runCLI(t, dir, "cr", "status", "1")
	if runErr == nil {
		t.Fatalf("expected runtime error when sophia is not initialized")
	}
	if strings.Contains(stderr, "Usage:") {
		t.Fatalf("did not expect usage dump for runtime error, got %q", stderr)
	}
}

func TestTaskDoneRejectsInvalidCommitType(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := service.New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	cr, err := svc.AddCR("Invalid commit type", "validation")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	task, err := svc.AddTask(cr.ID, "Unprefixed task")
	if err != nil {
		t.Fatalf("AddTask() error = %v", err)
	}
	intent := "Implement"
	acceptance := []string{"done"}
	scope := []string{"."}
	if _, err := svc.SetTaskContract(cr.ID, task.ID, service.TaskContractPatch{
		Intent:             &intent,
		AcceptanceCriteria: &acceptance,
		Scope:              &scope,
	}); err != nil {
		t.Fatalf("SetTaskContract() error = %v", err)
	}

	stdout, stderr, runErr := runCLI(t, dir, "cr", "task", "done", "1", "1", "--all", "--commit-type", "invalidtype")
	if runErr == nil {
		t.Fatalf("expected invalid commit type to fail")
	}
	combined := stdout + "\n" + stderr + "\n" + runErr.Error()
	if !strings.Contains(combined, "invalid --commit-type") {
		t.Fatalf("expected invalid commit type error, got stdout=%q stderr=%q err=%v", stdout, stderr, runErr)
	}
}
