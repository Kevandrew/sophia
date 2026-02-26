package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sophia/internal/service"
)

func runCLIWithStdin(t *testing.T, dir string, stdin string, args ...string) (string, string, error) {
	t.Helper()
	root := newRootCmd()
	root.SetContext(withServiceRepoRootContext(context.Background(), dir))
	root.SetIn(strings.NewReader(stdin))
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	err := executeRootCmd(root, args)
	return stdout.String(), stderr.String(), err
}

func TestCRApplyStdinDryRunJSONMatchesFileMode(t *testing.T) {
	dir := t.TempDir()
	svc := service.New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	writeCLIPlanFile(t, dir, "plan.yaml", cliValidCRPlanYAML)

	stdinOut, _, stdinErr := runCLIWithStdin(t, dir, cliValidCRPlanYAML, "cr", "apply", "--stdin", "--dry-run", "--json")
	if stdinErr != nil {
		t.Fatalf("cr apply --stdin --dry-run --json error = %v\noutput=%s", stdinErr, stdinOut)
	}
	stdinEnv := decodeEnvelope(t, stdinOut)
	if !stdinEnv.OK {
		t.Fatalf("expected ok stdin envelope, got %#v", stdinEnv)
	}

	fileOut, _, fileErr := runCLI(t, dir, "cr", "apply", "--file", "plan.yaml", "--dry-run", "--json")
	if fileErr != nil {
		t.Fatalf("cr apply --file --dry-run --json error = %v\noutput=%s", fileErr, fileOut)
	}
	fileEnv := decodeEnvelope(t, fileOut)
	if !fileEnv.OK {
		t.Fatalf("expected ok file envelope, got %#v", fileEnv)
	}

	if dryRun, _ := stdinEnv.Data["dry_run"].(bool); !dryRun {
		t.Fatalf("expected stdin dry_run=true, got %#v", stdinEnv.Data["dry_run"])
	}
	if consumed, _ := stdinEnv.Data["consumed"].(bool); consumed {
		t.Fatalf("expected stdin consumed=false, got %#v", stdinEnv.Data["consumed"])
	}
	stdinCreated, ok := stdinEnv.Data["created_crs"].([]any)
	if !ok {
		t.Fatalf("expected stdin created_crs array, got %#v", stdinEnv.Data["created_crs"])
	}
	fileCreated, ok := fileEnv.Data["created_crs"].([]any)
	if !ok {
		t.Fatalf("expected file created_crs array, got %#v", fileEnv.Data["created_crs"])
	}
	if len(stdinCreated) != len(fileCreated) {
		t.Fatalf("expected stdin and file created_crs to match, got stdin=%#v file=%#v", stdinEnv.Data["created_crs"], fileEnv.Data["created_crs"])
	}
}

func TestCRApplyStdinMutuallyExclusiveWithFile(t *testing.T) {
	dir := t.TempDir()
	svc := service.New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	writeCLIPlanFile(t, dir, "plan.yaml", cliValidCRPlanYAML)

	out, _, runErr := runCLIWithStdin(t, dir, cliValidCRPlanYAML, "cr", "apply", "--stdin", "--file", "plan.yaml", "--json")
	if runErr == nil {
		t.Fatalf("expected --stdin + --file to fail")
	}
	env := decodeEnvelope(t, out)
	if env.OK || env.Error == nil {
		t.Fatalf("expected structured error envelope, got %#v", env)
	}
	if env.Error.Code != "invalid_argument" {
		t.Fatalf("expected invalid_argument code, got %#v", env.Error)
	}
}

func TestCRApplyStdinInvalidYAMLFails(t *testing.T) {
	dir := t.TempDir()
	svc := service.New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	out, _, runErr := runCLIWithStdin(t, dir, "version: v1\ncrs:\n  - bad\n", "cr", "apply", "--stdin", "--json")
	if runErr == nil {
		t.Fatalf("expected invalid stdin yaml to fail")
	}
	env := decodeEnvelope(t, out)
	if env.OK || env.Error == nil {
		t.Fatalf("expected structured error envelope, got %#v", env)
	}
}

func TestCRContractSetDryRunAndAlreadyAppliedJSON(t *testing.T) {
	dir := t.TempDir()
	svc := service.New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	cr, err := svc.AddCR("Contract dry-run", "contract no-op signals")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	before, err := svc.GetCRContract(cr.ID)
	if err != nil {
		t.Fatalf("GetCRContract(before) error = %v", err)
	}

	dryOut, _, dryErr := runCLI(t, dir, "cr", "contract", "set", "1", "--why", "new why", "--dry-run", "--json")
	if dryErr != nil {
		t.Fatalf("cr contract set --dry-run --json error = %v\noutput=%s", dryErr, dryOut)
	}
	dryEnv := decodeEnvelope(t, dryOut)
	if !dryEnv.OK {
		t.Fatalf("expected ok dry-run envelope, got %#v", dryEnv)
	}
	if dryRun, _ := dryEnv.Data["dry_run"].(bool); !dryRun {
		t.Fatalf("expected dry_run=true, got %#v", dryEnv.Data["dry_run"])
	}
	if alreadyApplied, _ := dryEnv.Data["already_applied"].(bool); alreadyApplied {
		t.Fatalf("expected already_applied=false on dry-run change, got %#v", dryEnv.Data["already_applied"])
	}

	afterDryRun, err := svc.GetCRContract(cr.ID)
	if err != nil {
		t.Fatalf("GetCRContract(after dry-run) error = %v", err)
	}
	if afterDryRun.Why != before.Why {
		t.Fatalf("expected dry-run to avoid mutation, before=%q after=%q", before.Why, afterDryRun.Why)
	}

	firstOut, _, firstErr := runCLI(t, dir, "cr", "contract", "set", "1", "--why", "new why", "--json")
	if firstErr != nil {
		t.Fatalf("first cr contract set --json error = %v\noutput=%s", firstErr, firstOut)
	}
	firstEnv := decodeEnvelope(t, firstOut)
	if !firstEnv.OK {
		t.Fatalf("expected ok first contract set envelope, got %#v", firstEnv)
	}
	if alreadyApplied, _ := firstEnv.Data["already_applied"].(bool); alreadyApplied {
		t.Fatalf("expected already_applied=false on first set, got %#v", firstEnv.Data["already_applied"])
	}

	secondOut, _, secondErr := runCLI(t, dir, "cr", "contract", "set", "1", "--why", "new why", "--json")
	if secondErr != nil {
		t.Fatalf("second cr contract set --json error = %v\noutput=%s", secondErr, secondOut)
	}
	secondEnv := decodeEnvelope(t, secondOut)
	if !secondEnv.OK {
		t.Fatalf("expected ok second contract set envelope, got %#v", secondEnv)
	}
	if alreadyApplied, _ := secondEnv.Data["already_applied"].(bool); !alreadyApplied {
		t.Fatalf("expected already_applied=true on second set, got %#v", secondEnv.Data["already_applied"])
	}
}

func TestCRTaskDoneDryRunDoesNotCreateCommit(t *testing.T) {
	dir := t.TempDir()
	svc := service.New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "task.txt"), []byte("one\n"), 0o644); err != nil {
		t.Fatalf("write seed file: %v", err)
	}
	runGit(t, dir, "add", "task.txt")
	runGit(t, dir, "commit", "-m", "seed task file")

	cr, err := svc.AddCR("Task dry-run", "preview task checkpoint")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	task, err := svc.AddTask(cr.ID, "dry-run done")
	if err != nil {
		t.Fatalf("AddTask() error = %v", err)
	}
	intent := "Dry-run checkpoint preview"
	acceptance := []string{"No commit created during dry-run"}
	scope := []string{"task.txt"}
	if _, err := svc.SetTaskContract(cr.ID, task.ID, service.TaskContractPatch{
		Intent:             &intent,
		AcceptanceCriteria: &acceptance,
		Scope:              &scope,
	}); err != nil {
		t.Fatalf("SetTaskContract() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "task.txt"), []byte("two\n"), 0o644); err != nil {
		t.Fatalf("write modified file: %v", err)
	}
	headBefore := runGit(t, dir, "rev-parse", "HEAD")

	out, _, runErr := runCLI(t, dir, "cr", "task", "done", "1", "1", "--from-contract", "--dry-run", "--json")
	if runErr != nil {
		t.Fatalf("cr task done --dry-run --json error = %v\noutput=%s", runErr, out)
	}
	env := decodeEnvelope(t, out)
	if !env.OK {
		t.Fatalf("expected ok envelope, got %#v", env)
	}
	if dryRun, _ := env.Data["dry_run"].(bool); !dryRun {
		t.Fatalf("expected dry_run=true, got %#v", env.Data["dry_run"])
	}

	headAfter := runGit(t, dir, "rev-parse", "HEAD")
	if headAfter != headBefore {
		t.Fatalf("expected no commit on dry-run, before=%s after=%s", headBefore, headAfter)
	}
	tasks, err := svc.ListTasks(cr.ID)
	if err != nil {
		t.Fatalf("ListTasks() error = %v", err)
	}
	if tasks[0].Status != "open" {
		t.Fatalf("expected task status open after dry-run, got %q", tasks[0].Status)
	}
}
