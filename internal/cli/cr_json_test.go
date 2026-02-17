package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"

	"sophia/internal/service"
)

var cliCWDMu sync.Mutex

func TestCRJSONCommandsReturnEnvelope(t *testing.T) {
	dir := t.TempDir()
	svc := service.New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")

	cr, err := svc.AddCR("JSON CR", "json rationale")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	task, err := svc.AddTask(cr.ID, "feat: json outputs")
	if err != nil {
		t.Fatalf("AddTask() error = %v", err)
	}
	why := "Provide machine-readable command output."
	scope := []string{"feature.txt"}
	nonGoals := []string{"No orchestration"}
	invariants := []string{"Git remains source of truth"}
	blast := "CLI output surface only"
	testPlan := "go test ./..."
	rollback := "revert"
	if _, err := svc.SetCRContract(cr.ID, service.ContractPatch{
		Why:          &why,
		Scope:        &scope,
		NonGoals:     &nonGoals,
		Invariants:   &invariants,
		BlastRadius:  &blast,
		TestPlan:     &testPlan,
		RollbackPlan: &rollback,
	}); err != nil {
		t.Fatalf("SetCRContract() error = %v", err)
	}
	intent := "Task contract for JSON test."
	acceptance := []string{"Task contract show has data."}
	taskScope := []string{"feature.txt"}
	if _, err := svc.SetTaskContract(cr.ID, task.ID, service.TaskContractPatch{
		Intent:             &intent,
		AcceptanceCriteria: &acceptance,
		Scope:              &taskScope,
	}); err != nil {
		t.Fatalf("SetTaskContract() error = %v", err)
	}

	if err := os.WriteFile(filepath.Join(dir, "feature.txt"), []byte("json\n"), 0o644); err != nil {
		t.Fatalf("write feature file: %v", err)
	}
	runGit(t, dir, "add", "feature.txt")
	runGit(t, dir, "commit", "-m", "feat: json fixture")

	cases := []struct {
		args []string
		keys []string
	}{
		{args: []string{"cr", "current", "--json"}, keys: []string{"branch", "cr"}},
		{args: []string{"cr", "task", "list", "1", "--json"}, keys: []string{"cr_id", "tasks"}},
		{args: []string{"cr", "task", "chunk", "list", "1", "1", "--json"}, keys: []string{"cr_id", "task_id", "chunks"}},
		{args: []string{"cr", "task", "contract", "show", "1", "1", "--json"}, keys: []string{"cr_id", "task_id", "task_contract"}},
		{args: []string{"cr", "why", "1", "--json"}, keys: []string{"cr_uid", "base_ref", "base_commit", "parent_cr_id", "effective_why", "source"}},
		{args: []string{"cr", "status", "1", "--json"}, keys: []string{"id", "uid", "base_ref", "base_commit", "parent_cr_id", "parent_status", "title", "working_tree", "validation", "merge_blocked"}},
		{args: []string{"cr", "impact", "1", "--json"}, keys: []string{"cr_id", "cr_uid", "base_ref", "base_commit", "parent_cr_id", "risk_tier", "risk_score"}},
		{args: []string{"cr", "review", "1", "--json"}, keys: []string{"cr", "impact", "validation_errors", "validation_warnings"}},
		{args: []string{"cr", "validate", "1", "--json"}, keys: []string{"valid", "errors", "warnings", "impact"}},
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

	out, _, runErr := runCLI(t, dir, "cr", "current", "--json")
	if runErr != nil {
		t.Fatalf("cr current --json error = %v\noutput=%s", runErr, out)
	}
	currentEnv := decodeEnvelope(t, out)
	crData, ok := currentEnv.Data["cr"].(map[string]any)
	if !ok {
		t.Fatalf("expected current.cr object, got %#v", currentEnv.Data["cr"])
	}
	uid, _ := crData["uid"].(string)
	if strings.TrimSpace(uid) == "" {
		t.Fatalf("expected current.cr.uid to be non-empty, got %#v", crData)
	}

	out, _, runErr = runCLI(t, dir, "cr", "review", "1", "--json")
	if runErr != nil {
		t.Fatalf("cr review --json error = %v\noutput=%s", runErr, out)
	}
	reviewEnv := decodeEnvelope(t, out)
	reviewCR, ok := reviewEnv.Data["cr"].(map[string]any)
	if !ok {
		t.Fatalf("expected review.cr object, got %#v", reviewEnv.Data["cr"])
	}
	reviewUID, _ := reviewCR["uid"].(string)
	if strings.TrimSpace(reviewUID) == "" {
		t.Fatalf("expected review.cr.uid to be non-empty, got %#v", reviewCR)
	}
	reviewSubtasks, ok := reviewEnv.Data["subtasks"].([]any)
	if !ok || len(reviewSubtasks) == 0 {
		t.Fatalf("expected review subtasks array, got %#v", reviewEnv.Data["subtasks"])
	}
	firstSubtask, ok := reviewSubtasks[0].(map[string]any)
	if !ok {
		t.Fatalf("expected review subtask object, got %#v", reviewSubtasks[0])
	}
	if _, ok := firstSubtask["checkpoint_chunks"]; !ok {
		t.Fatalf("expected review subtask checkpoint_chunks field, got %#v", firstSubtask)
	}
}

func TestValidateJSONReturnsStructuredErrorWhenInvalid(t *testing.T) {
	dir := t.TempDir()
	svc := service.New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	cr, err := svc.AddCR("Invalid validate", "missing contract")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	if cr.ID != 1 {
		t.Fatalf("expected CR id 1, got %d", cr.ID)
	}

	out, _, runErr := runCLI(t, dir, "cr", "validate", "1", "--json")
	if runErr == nil {
		t.Fatalf("expected validate --json to fail for invalid CR")
	}
	env := decodeEnvelope(t, out)
	if env.OK {
		t.Fatalf("expected non-ok envelope, got %#v", env)
	}
	if env.Error == nil {
		t.Fatalf("expected structured error payload, got %#v", env)
	}
	if env.Error.Code != "validation_failed" {
		t.Fatalf("expected validation_failed code, got %#v", env.Error)
	}
}

func TestTaskDoneFlagConflictsWithFromContract(t *testing.T) {
	dir := t.TempDir()
	svc := service.New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	cr, err := svc.AddCR("Task flags", "conflict checks")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	task, err := svc.AddTask(cr.ID, "feat: flag conflicts")
	if err != nil {
		t.Fatalf("AddTask() error = %v", err)
	}
	intent := "Set up task for conflict checks."
	acceptance := []string{"Flag conflicts are rejected."}
	scope := []string{"feature.txt"}
	if _, err := svc.SetTaskContract(cr.ID, task.ID, service.TaskContractPatch{
		Intent:             &intent,
		AcceptanceCriteria: &acceptance,
		Scope:              &scope,
	}); err != nil {
		t.Fatalf("SetTaskContract() error = %v", err)
	}

	_, _, err = runCLI(t, dir, "cr", "task", "done", "1", "1", "--no-checkpoint", "--from-contract")
	if err == nil || !strings.Contains(err.Error(), "--no-checkpoint cannot be combined") {
		t.Fatalf("expected --no-checkpoint conflict error, got %v", err)
	}

	_, _, err = runCLI(t, dir, "cr", "task", "done", "1", "1", "--from-contract", "--all")
	if err == nil || !strings.Contains(err.Error(), "exactly one checkpoint scope mode is required") {
		t.Fatalf("expected exclusivity conflict error, got %v", err)
	}

	_, _, err = runCLI(t, dir, "cr", "task", "done", "1", "1", "--patch-file", "task.patch", "--path", "feature.txt")
	if err == nil || !strings.Contains(err.Error(), "exactly one checkpoint scope mode is required") {
		t.Fatalf("expected --patch-file exclusivity conflict error, got %v", err)
	}

	_, _, err = runCLI(t, dir, "cr", "task", "done", "1", "1", "--no-checkpoint", "--patch-file", "task.patch")
	if err == nil || !strings.Contains(err.Error(), "--no-checkpoint cannot be combined") {
		t.Fatalf("expected --no-checkpoint + --patch-file conflict error, got %v", err)
	}
}

func TestTaskDonePatchFileSuccess(t *testing.T) {
	dir := t.TempDir()
	svc := service.New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")

	if err := os.WriteFile(filepath.Join(dir, "chunked.txt"), []byte("l1\nl2\nl3\nl4\nl5\nl6\nl7\nl8\n"), 0o644); err != nil {
		t.Fatalf("write seed file: %v", err)
	}
	runGit(t, dir, "add", "chunked.txt")
	runGit(t, dir, "commit", "-m", "chore: seed chunked file")

	cr, err := svc.AddCR("Patch file CLI", "task done patch mode")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	task, err := svc.AddTask(cr.ID, "feat: checkpoint patch file")
	if err != nil {
		t.Fatalf("AddTask() error = %v", err)
	}
	intent := "Checkpoint only selected hunks."
	acceptance := []string{"Patch-file mode stages selected hunks."}
	scope := []string{"chunked.txt"}
	if _, err := svc.SetTaskContract(cr.ID, task.ID, service.TaskContractPatch{
		Intent:             &intent,
		AcceptanceCriteria: &acceptance,
		Scope:              &scope,
	}); err != nil {
		t.Fatalf("SetTaskContract() error = %v", err)
	}

	if err := os.WriteFile(filepath.Join(dir, "chunked.txt"), []byte("l1\nl2-edited\nl3\nl4\nl5\nl6\nl7-edited\nl8\n"), 0o644); err != nil {
		t.Fatalf("write modified file: %v", err)
	}
	patch := runGit(t, dir, "diff", "--unified=0", "chunked.txt")
	patch = firstHunkPatchFromDiff(t, patch)
	if err := os.WriteFile(filepath.Join(dir, "task.patch"), []byte(patch), 0o644); err != nil {
		t.Fatalf("write patch file: %v", err)
	}

	out, _, runErr := runCLI(t, dir, "cr", "task", "done", "1", "1", "--patch-file", "task.patch")
	if runErr != nil {
		t.Fatalf("cr task done --patch-file error = %v\noutput=%s", runErr, out)
	}
	if !strings.Contains(out, "Marked task 1 done in CR 1 with checkpoint") {
		t.Fatalf("unexpected output: %q", out)
	}
}

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

func TestCRAddRejectsBaseAndParentTogether(t *testing.T) {
	dir := t.TempDir()
	svc := service.New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	_, _, err := runCLI(t, dir, "cr", "add", "Conflict", "--base", "main", "--parent", "1")
	if err == nil || !strings.Contains(err.Error(), "--base and --parent cannot be combined") {
		t.Fatalf("expected --base/--parent conflict error, got %v", err)
	}
}

func TestCRBaseSetAndRestackCommands(t *testing.T) {
	dir := t.TempDir()
	svc := service.New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")

	baseCR, err := svc.AddCR("Base set", "cli base set")
	if err != nil {
		t.Fatalf("AddCR(base) error = %v", err)
	}
	out, _, runErr := runCLI(t, dir, "cr", "base", "set", "1", "--ref", "main")
	if runErr != nil {
		t.Fatalf("cr base set error = %v\noutput=%s", runErr, out)
	}
	if !strings.Contains(out, "Updated CR 1 base") {
		t.Fatalf("unexpected base set output: %q", out)
	}
	if _, err := svc.SwitchCR(baseCR.ID); err != nil {
		t.Fatalf("SwitchCR(base) error = %v", err)
	}

	parent, err := svc.AddCR("Parent", "for restack")
	if err != nil {
		t.Fatalf("AddCR(parent) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "parent.txt"), []byte("p1\n"), 0o644); err != nil {
		t.Fatalf("write parent file: %v", err)
	}
	runGit(t, dir, "add", "parent.txt")
	runGit(t, dir, "commit", "-m", "feat: parent")

	child, _, err := svc.AddCRWithOptionsWithWarnings("Child", "for restack", service.AddCROptions{ParentCRID: parent.ID})
	if err != nil {
		t.Fatalf("AddCR(child) error = %v", err)
	}
	if _, err := svc.SwitchCR(parent.ID); err != nil {
		t.Fatalf("SwitchCR(parent) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "parent2.txt"), []byte("p2\n"), 0o644); err != nil {
		t.Fatalf("write parent second file: %v", err)
	}
	runGit(t, dir, "add", "parent2.txt")
	runGit(t, dir, "commit", "-m", "feat: parent update")

	out, _, runErr = runCLI(t, dir, "cr", "restack", strconv.Itoa(child.ID))
	if runErr != nil {
		t.Fatalf("cr restack error = %v\noutput=%s", runErr, out)
	}
	if !strings.Contains(out, "Restacked CR") {
		t.Fatalf("unexpected restack output: %q", out)
	}
}

func firstHunkPatchFromDiff(t *testing.T, diff string) string {
	t.Helper()
	diff = strings.TrimSpace(diff)
	if diff == "" {
		t.Fatalf("expected non-empty diff")
	}
	lines := strings.Split(diff, "\n")
	out := make([]string, 0, len(lines))
	hunks := 0
	for _, line := range lines {
		if strings.HasPrefix(line, "@@ ") {
			hunks++
			if hunks > 1 {
				break
			}
		}
		out = append(out, line)
	}
	if hunks == 0 {
		t.Fatalf("expected at least one hunk in diff: %q", diff)
	}
	return strings.Join(out, "\n") + "\n"
}

func runCLI(t *testing.T, dir string, args ...string) (string, string, error) {
	t.Helper()
	cliCWDMu.Lock()
	defer cliCWDMu.Unlock()

	orig, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir(%s) error = %v", dir, err)
	}
	defer func() {
		_ = os.Chdir(orig)
	}()

	root := newRootCmd()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs(args)
	err = root.Execute()
	return stdout.String(), stderr.String(), err
}

func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, string(out))
	}
	return strings.TrimSpace(string(out))
}

type envelope struct {
	OK    bool                  `json:"ok"`
	Data  map[string]any        `json:"data"`
	Error *envelopeErrorPayload `json:"error,omitempty"`
}

type envelopeErrorPayload struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func decodeEnvelope(t *testing.T, raw string) envelope {
	t.Helper()
	var env envelope
	if err := json.Unmarshal([]byte(raw), &env); err != nil {
		t.Fatalf("decode envelope: %v\nraw=%s", err, raw)
	}
	return env
}
