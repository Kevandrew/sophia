package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sophia/internal/service"
)

func TestTaskChunkListCommandSupportsTextJSONAndPathFilter(t *testing.T) {
	dir := t.TempDir()
	svc := service.New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")

	if err := os.WriteFile(filepath.Join(dir, "alpha.txt"), []byte("a1\na2\na3\na4\na5\na6\na7\na8\n"), 0o644); err != nil {
		t.Fatalf("write alpha file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "beta.txt"), []byte("b1\nb2\n"), 0o644); err != nil {
		t.Fatalf("write beta file: %v", err)
	}
	runGit(t, dir, "add", "alpha.txt", "beta.txt")
	runGit(t, dir, "commit", "-m", "chore: seed chunk files")

	cr, err := svc.AddCR("Chunk list CLI", "inspect task chunks")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	task, err := svc.AddTask(cr.ID, "feat: list chunk candidates")
	if err != nil {
		t.Fatalf("AddTask() error = %v", err)
	}
	intent := "List chunks for patch selection."
	acceptance := []string{"Chunk list command returns deterministic chunks."}
	scope := []string{"alpha.txt", "beta.txt"}
	if _, err := svc.SetTaskContract(cr.ID, task.ID, service.TaskContractPatch{
		Intent:             &intent,
		AcceptanceCriteria: &acceptance,
		Scope:              &scope,
	}); err != nil {
		t.Fatalf("SetTaskContract() error = %v", err)
	}

	if err := os.WriteFile(filepath.Join(dir, "alpha.txt"), []byte("a1\na2-edited\na3\na4\na5\na6\na7-edited\na8\n"), 0o644); err != nil {
		t.Fatalf("write alpha modifications: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "beta.txt"), []byte("b1\nb2-edited\n"), 0o644); err != nil {
		t.Fatalf("write beta modifications: %v", err)
	}

	textOut, _, textErr := runCLI(t, dir, "cr", "task", "chunk", "list", "1", "1")
	if textErr != nil {
		t.Fatalf("chunk list text error = %v\noutput=%s", textErr, textOut)
	}
	if !strings.Contains(textOut, "CHUNK_ID\tPATH\tOLD\tNEW\tPREVIEW") {
		t.Fatalf("expected table header in text output, got %q", textOut)
	}

	jsonOut, _, jsonErr := runCLI(t, dir, "cr", "task", "chunk", "list", "1", "1", "--json")
	if jsonErr != nil {
		t.Fatalf("chunk list json error = %v\noutput=%s", jsonErr, jsonOut)
	}
	env := decodeEnvelope(t, jsonOut)
	if !env.OK {
		t.Fatalf("expected ok envelope, got %#v", env)
	}
	rawChunks, ok := env.Data["chunks"].([]any)
	if !ok || len(rawChunks) != 3 {
		t.Fatalf("expected 3 chunks in json output, got %#v", env.Data["chunks"])
	}
	firstChunk, ok := rawChunks[0].(map[string]any)
	if !ok {
		t.Fatalf("expected chunk object, got %#v", rawChunks[0])
	}
	if _, ok := firstChunk["chunk_id"]; !ok {
		t.Fatalf("expected snake_case chunk_id key, got %#v", firstChunk)
	}

	filterOut, _, filterErr := runCLI(t, dir, "cr", "task", "chunk", "list", "1", "1", "--path", "beta.txt", "--json")
	if filterErr != nil {
		t.Fatalf("chunk list filtered json error = %v\noutput=%s", filterErr, filterOut)
	}
	filterEnv := decodeEnvelope(t, filterOut)
	filterChunks, ok := filterEnv.Data["chunks"].([]any)
	if !ok || len(filterChunks) != 1 {
		t.Fatalf("expected 1 filtered chunk, got %#v", filterEnv.Data["chunks"])
	}
}
