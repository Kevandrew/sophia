package cli

import (
	"os"
	"path/filepath"
	"testing"

	"sophia/internal/service"
)

func TestMutationGuardrailJSONIncludesSuggestedAction(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := service.New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	cr, _, err := svc.AddCRWithOptionsWithWarnings("Guardrail", "checkpoint branch guardrail", service.AddCROptions{NoSwitch: true})
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	task, err := svc.AddTask(cr.ID, "task with checkpoint")
	if err != nil {
		t.Fatalf("AddTask() error = %v", err)
	}
	intent := "guardrail task"
	acceptance := []string{"checkpoint should require active branch"}
	scope := []string{"guardrail.txt"}
	if _, err := svc.SetTaskContract(cr.ID, task.ID, service.TaskContractPatch{
		Intent:             &intent,
		AcceptanceCriteria: &acceptance,
		Scope:              &scope,
	}); err != nil {
		t.Fatalf("SetTaskContract() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "guardrail.txt"), []byte("content\n"), 0o644); err != nil {
		t.Fatalf("write guardrail.txt: %v", err)
	}

	out, _, runErr := runCLI(t, dir, "cr", "task", "chunk", "list", "1", "1", "--json")
	if runErr == nil {
		t.Fatalf("expected active branch guardrail error")
	}
	env := decodeEnvelope(t, out)
	if env.OK || env.Error == nil {
		t.Fatalf("expected structured guardrail error envelope, got %#v", env)
	}
	if env.Error.Code != "no_active_cr_context" {
		t.Fatalf("expected no_active_cr_context code, got %#v", env.Error.Code)
	}
	if env.Error.Details == nil {
		t.Fatalf("expected details map with suggested_action, got %#v", env.Error)
	}
	action, _ := env.Error.Details["suggested_action"].(string)
	if action != "sophia cr switch 1" {
		t.Fatalf("expected suggested_action sophia cr switch 1, got %#v", action)
	}
}
