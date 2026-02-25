package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sophia/internal/service"
)

func TestCRDiffJSONCommands(t *testing.T) {
	dir := t.TempDir()
	svc := service.New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	cr, err := svc.AddCR("Diff JSON", "json diff commands")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	riskCritical := []string{"critical"}
	if _, err := svc.SetCRContract(cr.ID, service.ContractPatch{RiskCriticalScopes: &riskCritical}); err != nil {
		t.Fatalf("SetCRContract() error = %v", err)
	}
	task, err := svc.AddTask(cr.ID, "feat: diff task fixture")
	if err != nil {
		t.Fatalf("AddTask() error = %v", err)
	}
	intent := "task diff fixture"
	acceptance := []string{"checkpoint created"}
	scope := []string{"critical/file.txt"}
	if _, err := svc.SetTaskContract(cr.ID, task.ID, service.TaskContractPatch{
		Intent:             &intent,
		AcceptanceCriteria: &acceptance,
		Scope:              &scope,
	}); err != nil {
		t.Fatalf("SetTaskContract() error = %v", err)
	}

	if err := os.MkdirAll(filepath.Join(dir, "critical"), 0o755); err != nil {
		t.Fatalf("mkdir critical: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "critical", "file.txt"), []byte("diff\n"), 0o644); err != nil {
		t.Fatalf("write diff file: %v", err)
	}
	if _, err := svc.DoneTaskWithCheckpoint(cr.ID, task.ID, service.DoneTaskOptions{Checkpoint: true, FromContract: true}); err != nil {
		t.Fatalf("DoneTaskWithCheckpoint() error = %v", err)
	}

	taskView, err := svc.DiffTask(cr.ID, task.ID, service.TaskDiffOptions{})
	if err != nil {
		t.Fatalf("DiffTask() error = %v", err)
	}
	if len(taskView.Files) == 0 || len(taskView.Files[0].Hunks) == 0 {
		t.Fatalf("expected task hunks, got %#v", taskView)
	}
	chunkID := taskView.Files[0].Hunks[0].ChunkID

	cases := []struct {
		args []string
		keys []string
	}{
		{args: []string{"cr", "diff", "1", "--json"}, keys: []string{"cr_id", "mode", "files", "files_changed", "fallback_used"}},
		{args: []string{"cr", "diff", "1", "--task", "1", "--json"}, keys: []string{"cr_id", "task_id", "mode", "files", "short_stat"}},
		{args: []string{"cr", "diff", "1", "--critical", "--json"}, keys: []string{"cr_id", "critical_only", "files", "warnings"}},
		{args: []string{"cr", "task", "diff", "1", "1", "--json"}, keys: []string{"cr_id", "task_id", "mode", "files"}},
		{args: []string{"cr", "task", "chunk", "diff", "1", "1", chunkID, "--json"}, keys: []string{"cr_id", "task_id", "mode", "chunks_only", "files"}},
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

	out, _, runErr := runCLI(t, dir, "cr", "task", "chunk", "diff", "1", "1", "chk_missing", "--json")
	if runErr == nil {
		t.Fatalf("expected chunk diff error for unknown chunk id")
	}
	env := decodeEnvelope(t, out)
	if env.OK || env.Error == nil {
		t.Fatalf("expected structured error envelope for bad chunk id, got %#v", env)
	}
}
