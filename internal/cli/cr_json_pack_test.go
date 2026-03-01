package cli

import (
	"os"
	"path/filepath"
	"testing"

	"sophia/internal/service"
)

func TestCRPackJSONCommand(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := service.New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	cr, err := svc.AddCR("Pack CLI", "pack command fixture")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	task, err := svc.AddTask(cr.ID, "checkpoint fixture")
	if err != nil {
		t.Fatalf("AddTask() error = %v", err)
	}
	intent := "pack fixture intent"
	acceptance := []string{"pack fixture acceptance"}
	scope := []string{"pack-cli.txt"}
	if _, err := svc.SetTaskContract(cr.ID, task.ID, service.TaskContractPatch{
		Intent:             &intent,
		AcceptanceCriteria: &acceptance,
		Scope:              &scope,
	}); err != nil {
		t.Fatalf("SetTaskContract() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "pack-cli.txt"), []byte("one\n"), 0o644); err != nil {
		t.Fatalf("write pack-cli.txt: %v", err)
	}
	if _, err := svc.DoneTaskWithCheckpoint(cr.ID, task.ID, service.DoneTaskOptions{Checkpoint: true, FromContract: true}); err != nil {
		t.Fatalf("DoneTaskWithCheckpoint() error = %v", err)
	}

	out, _, runErr := runCLI(t, dir, "cr", "pack", "1", "--json", "--events-limit", "1", "--checkpoints-limit", "1")
	if runErr != nil {
		t.Fatalf("cr pack --json error = %v\noutput=%s", runErr, out)
	}
	env := decodeEnvelope(t, out)
	if !env.OK {
		t.Fatalf("expected ok envelope, got %#v", env)
	}
	for _, key := range []string{"cr", "contract", "tasks", "anchors", "status", "recent_events", "events_meta", "recent_checkpoints", "checkpoints_meta", "diff_stat", "validation", "trust"} {
		if _, ok := env.Data[key]; !ok {
			t.Fatalf("expected pack key %q in %#v", key, env.Data)
		}
	}
}

func TestCRPackJSONCommandRejectsNegativeLimits(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := service.New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if _, err := svc.AddCR("Pack negative", "negative limit error"); err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}

	out, _, runErr := runCLI(t, dir, "cr", "pack", "1", "--json", "--events-limit", "-1")
	if runErr == nil {
		t.Fatalf("expected error for negative events limit")
	}
	env := decodeEnvelope(t, out)
	if env.OK || env.Error == nil {
		t.Fatalf("expected structured error envelope, got %#v", env)
	}
}
