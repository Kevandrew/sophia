package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sophia/internal/service"
)

func TestCRRangeDiffJSONCommands(t *testing.T) {
	dir := t.TempDir()
	svc := service.New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")

	cr, err := svc.AddCR("Range JSON", "json rangediff commands")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	task, err := svc.AddTask(cr.ID, "feat: rangediff task fixture")
	if err != nil {
		t.Fatalf("AddTask() error = %v", err)
	}
	intent := "task rangediff fixture"
	acceptance := []string{"checkpoint created"}
	scope := []string{"range.txt"}
	if _, err := svc.SetTaskContract(cr.ID, task.ID, service.TaskContractPatch{
		Intent:             &intent,
		AcceptanceCriteria: &acceptance,
		Scope:              &scope,
	}); err != nil {
		t.Fatalf("SetTaskContract() error = %v", err)
	}

	if err := os.WriteFile(filepath.Join(dir, "range.txt"), []byte("one\n"), 0o644); err != nil {
		t.Fatalf("write range.txt#1: %v", err)
	}
	fromSHA, err := svc.DoneTaskWithCheckpoint(cr.ID, task.ID, service.DoneTaskOptions{Checkpoint: true, FromContract: true})
	if err != nil {
		t.Fatalf("DoneTaskWithCheckpoint() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "range.txt"), []byte("two\n"), 0o644); err != nil {
		t.Fatalf("write range.txt#2: %v", err)
	}
	runGit(t, dir, "add", "range.txt")
	runGit(t, dir, "commit", "-m", "feat: second range commit")

	cases := []struct {
		args []string
		keys []string
	}{
		{args: []string{"cr", "rangediff", "1", "--from", fromSHA, "--to", "HEAD", "--json"}, keys: []string{"cr_id", "from_ref", "to_ref", "mapping", "files_changed", "short_stat"}},
		{args: []string{"cr", "task", "rangediff", "1", "1", "--since-last-checkpoint", "--json"}, keys: []string{"cr_id", "task_id", "old_range", "new_range", "mapping"}},
	}
	for _, tc := range cases {
		out, _, runErr := runCLI(t, dir, tc.args...)
		if runErr != nil {
			t.Fatalf("%q error = %v\noutput=%s", strings.Join(tc.args, " "), runErr, out)
		}
		env := decodeEnvelope(t, out)
		if !env.OK {
			t.Fatalf("%q expected ok envelope, got %#v", strings.Join(tc.args, " "), env)
		}
		for _, key := range tc.keys {
			if _, ok := env.Data[key]; !ok {
				t.Fatalf("%q expected data key %q in %#v", strings.Join(tc.args, " "), key, env.Data)
			}
		}
	}

	out, _, runErr := runCLI(t, dir, "cr", "rangediff", "1", "--from", fromSHA, "--since-last-checkpoint", "--json")
	if runErr == nil {
		t.Fatalf("expected mutually exclusive anchor flag error")
	}
	env := decodeEnvelope(t, out)
	if env.OK || env.Error == nil {
		t.Fatalf("expected structured error envelope for mutually exclusive anchor flags, got %#v", env)
	}
}
