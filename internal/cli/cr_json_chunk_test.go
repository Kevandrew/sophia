package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sophia/internal/service"
)

func TestTaskChunkListCommandSupportsTextJSONAndPathFilter(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := service.New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

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

func TestTaskChunkShowAndExportCommandsJSON(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := service.New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	if err := os.WriteFile(filepath.Join(dir, "alpha.txt"), []byte("a1\na2\na3\na4\na5\na6\na7\na8\n"), 0o644); err != nil {
		t.Fatalf("write alpha file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "beta.txt"), []byte("b1\nb2\n"), 0o644); err != nil {
		t.Fatalf("write beta file: %v", err)
	}
	runGit(t, dir, "add", "alpha.txt", "beta.txt")
	runGit(t, dir, "commit", "-m", "chore: seed chunk files")

	cr, err := svc.AddCR("Chunk show/export CLI", "inspect and export chunks")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	task, err := svc.AddTask(cr.ID, "feat: show/export chunks")
	if err != nil {
		t.Fatalf("AddTask() error = %v", err)
	}
	intent := "Show and export chunks."
	acceptance := []string{"Commands output valid patch payloads."}
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

	listOut, _, listErr := runCLI(t, dir, "cr", "task", "chunk", "list", "1", "1", "--json")
	if listErr != nil {
		t.Fatalf("chunk list json error = %v\noutput=%s", listErr, listOut)
	}
	listEnv := decodeEnvelope(t, listOut)
	rawChunks, ok := listEnv.Data["chunks"].([]any)
	if !ok || len(rawChunks) < 2 {
		t.Fatalf("expected at least 2 chunks in json output, got %#v", listEnv.Data["chunks"])
	}
	firstChunk, ok := rawChunks[0].(map[string]any)
	if !ok {
		t.Fatalf("expected chunk object, got %#v", rawChunks[0])
	}
	firstChunkID, _ := firstChunk["chunk_id"].(string)
	if strings.TrimSpace(firstChunkID) == "" {
		t.Fatalf("expected first chunk id, got %#v", firstChunk)
	}

	showOut, _, showErr := runCLI(t, dir, "cr", "task", "chunk", "show", "1", "1", firstChunkID, "--json")
	if showErr != nil {
		t.Fatalf("chunk show json error = %v\noutput=%s", showErr, showOut)
	}
	showEnv := decodeEnvelope(t, showOut)
	patch, _ := showEnv.Data["patch"].(string)
	if !strings.Contains(patch, "diff --git") || !strings.Contains(patch, "@@") {
		t.Fatalf("expected patch body from chunk show, got %#v", showEnv.Data["patch"])
	}

	exportPath := filepath.Join(dir, "task-selected.patch")
	secondChunk, ok := rawChunks[1].(map[string]any)
	if !ok {
		t.Fatalf("expected second chunk object, got %#v", rawChunks[1])
	}
	secondChunkID, _ := secondChunk["chunk_id"].(string)
	exportOut, _, exportErr := runCLI(
		t,
		dir,
		"cr", "task", "chunk", "export", "1", "1",
		"--chunk", firstChunkID,
		"--chunk", secondChunkID,
		"--out", exportPath,
		"--json",
	)
	if exportErr != nil {
		t.Fatalf("chunk export json error = %v\noutput=%s", exportErr, exportOut)
	}
	exportEnv := decodeEnvelope(t, exportOut)
	if got, _ := exportEnv.Data["out"].(string); got != exportPath {
		t.Fatalf("expected out path %q, got %#v", exportPath, exportEnv.Data["out"])
	}
	content, readErr := os.ReadFile(exportPath)
	if readErr != nil {
		t.Fatalf("read export patch: %v", readErr)
	}
	if !strings.Contains(string(content), "diff --git") {
		t.Fatalf("expected exported patch content, got %q", string(content))
	}
}

func TestTaskChunkListJSONReturnsPreStagedError(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := service.New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	if err := os.WriteFile(filepath.Join(dir, "alpha.txt"), []byte("a1\na2\n"), 0o644); err != nil {
		t.Fatalf("write alpha file: %v", err)
	}
	runGit(t, dir, "add", "alpha.txt")
	runGit(t, dir, "commit", "-m", "chore: seed chunk file")

	cr, err := svc.AddCR("Chunk pre-staged CLI", "reject staged changes")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	task, err := svc.AddTask(cr.ID, "feat: pre-staged")
	if err != nil {
		t.Fatalf("AddTask() error = %v", err)
	}
	intent := "Pre-staged check."
	acceptance := []string{"chunk commands reject staged index."}
	scope := []string{"alpha.txt"}
	if _, err := svc.SetTaskContract(cr.ID, task.ID, service.TaskContractPatch{
		Intent:             &intent,
		AcceptanceCriteria: &acceptance,
		Scope:              &scope,
	}); err != nil {
		t.Fatalf("SetTaskContract() error = %v", err)
	}

	if err := os.WriteFile(filepath.Join(dir, "alpha.txt"), []byte("a1\na2-edited\n"), 0o644); err != nil {
		t.Fatalf("write alpha modifications: %v", err)
	}
	runGit(t, dir, "add", "alpha.txt")

	out, _, runErr := runCLI(t, dir, "cr", "task", "chunk", "list", "1", "1", "--json")
	if runErr == nil {
		t.Fatalf("expected chunk list pre-staged failure")
	}
	env := decodeEnvelope(t, out)
	if env.OK || env.Error == nil {
		t.Fatalf("expected json error envelope, got %#v", env)
	}
	if env.Error.Code != "pre_staged_changes" {
		t.Fatalf("expected pre_staged_changes code, got %#v", env.Error)
	}
}
