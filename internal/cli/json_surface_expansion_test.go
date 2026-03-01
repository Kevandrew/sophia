package cli

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"sophia/internal/service"
)

func TestRootCommandsSupportJSON(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	out, _, runErr := runCLI(t, dir, "init", "--json")
	if runErr != nil {
		t.Fatalf("init --json error = %v\noutput=%s", runErr, out)
	}
	env := decodeEnvelope(t, out)
	if !env.OK {
		t.Fatalf("expected ok envelope from init --json, got %#v", env)
	}
	if _, ok := env.Data["base_branch"]; !ok {
		t.Fatalf("expected base_branch in init payload, got %#v", env.Data)
	}
	svc := service.New(dir)
	if _, _, err := svc.AddCRWithOptionsWithWarnings("Doctor fixture", "seed command history", service.AddCROptions{NoSwitch: true}); err != nil {
		t.Fatalf("AddCRWithOptionsWithWarnings() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "doctor_fixture.txt"), []byte("fixture\n"), 0o644); err != nil {
		t.Fatalf("write doctor fixture: %v", err)
	}

	out, _, runErr = runCLI(t, dir, "hook", "install", "--json")
	if runErr != nil {
		t.Fatalf("hook install --json error = %v\noutput=%s", runErr, out)
	}
	env = decodeEnvelope(t, out)
	if !env.OK {
		t.Fatalf("expected ok envelope from hook install --json, got %#v", env)
	}
	if _, ok := env.Data["hook_path"]; !ok {
		t.Fatalf("expected hook_path in hook install payload, got %#v", env.Data)
	}

	out, _, runErr = runCLI(t, dir, "doctor", "--json")
	if runErr != nil {
		t.Fatalf("doctor --json error = %v\noutput=%s", runErr, out)
	}
	env = decodeEnvelope(t, out)
	if !env.OK {
		t.Fatalf("expected ok envelope from doctor --json, got %#v", env)
	}
	if _, ok := env.Data["findings"]; !ok {
		t.Fatalf("expected findings in doctor payload, got %#v", env.Data)
	}

	out, _, runErr = runCLI(t, dir, "log", "--json")
	if runErr != nil {
		t.Fatalf("log --json error = %v\noutput=%s", runErr, out)
	}
	env = decodeEnvelope(t, out)
	if !env.OK {
		t.Fatalf("expected ok envelope from log --json, got %#v", env)
	}
	if _, ok := env.Data["entries"]; !ok {
		t.Fatalf("expected entries in log payload, got %#v", env.Data)
	}

	out, _, runErr = runCLI(t, dir, "repair", "--json")
	if runErr != nil {
		t.Fatalf("repair --json error = %v\noutput=%s", runErr, out)
	}
	env = decodeEnvelope(t, out)
	if !env.OK {
		t.Fatalf("expected ok envelope from repair --json, got %#v", env)
	}
	if _, ok := env.Data["repaired_cr_ids"]; !ok {
		t.Fatalf("expected repaired_cr_ids in repair payload, got %#v", env.Data)
	}
}

func TestValidateRecordFlagControlsEventRecording(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := service.New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	cr, err := svc.AddCR("Validate record gate", "record behavior")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}

	why := "Ensure validate default is read-only."
	scope := []string{"validate_gate.txt"}
	nonGoals := []string{"No behavior change outside validate"}
	invariants := []string{"validation remains deterministic"}
	blast := "validation command only"
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

	before, err := svc.HistoryCR(cr.ID, true)
	if err != nil {
		t.Fatalf("HistoryCR(before) error = %v", err)
	}

	out, _, runErr := runCLI(t, dir, "cr", "validate", "1", "--json")
	if runErr != nil {
		t.Fatalf("validate --json error = %v\noutput=%s", runErr, out)
	}
	env := decodeEnvelope(t, out)
	if !env.OK {
		t.Fatalf("expected ok validate envelope, got %#v", env)
	}
	if recorded, ok := env.Data["recorded"].(bool); !ok || recorded {
		t.Fatalf("expected recorded=false by default, got %#v", env.Data["recorded"])
	}

	afterDefault, err := svc.HistoryCR(cr.ID, true)
	if err != nil {
		t.Fatalf("HistoryCR(after default validate) error = %v", err)
	}
	if len(afterDefault.Events) != len(before.Events) {
		t.Fatalf("expected validate without --record not to append events; before=%d after=%d", len(before.Events), len(afterDefault.Events))
	}

	out, _, runErr = runCLI(t, dir, "cr", "validate", "1", "--record", "--json")
	if runErr != nil {
		t.Fatalf("validate --record --json error = %v\noutput=%s", runErr, out)
	}
	env = decodeEnvelope(t, out)
	if !env.OK {
		t.Fatalf("expected ok validate --record envelope, got %#v", env)
	}
	if recorded, ok := env.Data["recorded"].(bool); !ok || !recorded {
		t.Fatalf("expected recorded=true with --record, got %#v", env.Data["recorded"])
	}

	afterRecord, err := svc.HistoryCR(cr.ID, true)
	if err != nil {
		t.Fatalf("HistoryCR(after record validate) error = %v", err)
	}
	if len(afterRecord.Events) != len(before.Events)+1 {
		t.Fatalf("expected validate --record to append exactly one event; before=%d after=%d", len(before.Events), len(afterRecord.Events))
	}
	last := afterRecord.Events[len(afterRecord.Events)-1]
	if last.Type != "cr_validated" {
		t.Fatalf("expected last event type cr_validated, got %#v", last)
	}
}

func TestCRListSearchFilterValidationAndAlias(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := service.New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if _, err := svc.AddCR("Alias Search CR", "alias coverage"); err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}

	out, _, runErr := runCLI(t, dir, "cr", "list", "--status", "bad", "--json")
	if runErr == nil {
		t.Fatalf("expected invalid status filter to fail")
	}
	env := decodeEnvelope(t, out)
	if env.Error == nil || env.Error.Code != "invalid_argument" {
		t.Fatalf("expected invalid_argument error for status filter, got %#v", env.Error)
	}

	out, _, runErr = runCLI(t, dir, "cr", "search", "--risk-tier", "bad", "--json")
	if runErr == nil {
		t.Fatalf("expected invalid risk-tier filter to fail")
	}
	env = decodeEnvelope(t, out)
	if env.Error == nil || env.Error.Code != "invalid_argument" {
		t.Fatalf("expected invalid_argument error for risk-tier filter, got %#v", env.Error)
	}

	out, _, runErr = runCLI(t, dir, "cr", "list", "--search", "Alias", "--json")
	if runErr != nil {
		t.Fatalf("cr list --search --json error = %v\noutput=%s", runErr, out)
	}
	env = decodeEnvelope(t, out)
	if !env.OK {
		t.Fatalf("expected ok envelope for list --search, got %#v", env)
	}
	if count, ok := env.Data["count"].(float64); !ok || count < 1 {
		t.Fatalf("expected at least one list --search result, got %#v", env.Data["count"])
	}

	out, _, runErr = runCLI(t, dir, "cr", "search", "alpha", "--search", "beta", "--json")
	if runErr == nil {
		t.Fatalf("expected query/--search mismatch to fail")
	}
	env = decodeEnvelope(t, out)
	if env.Error == nil || env.Error.Code != "invalid_argument" {
		t.Fatalf("expected invalid_argument for query mismatch, got %#v", env.Error)
	}
}

func TestTaskListJSONUsesSnakeCaseKeys(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := service.New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	cr, err := svc.AddCR("Task list case", "snake_case")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	if _, err := svc.AddTask(cr.ID, "task one"); err != nil {
		t.Fatalf("AddTask() error = %v", err)
	}

	out, _, runErr := runCLI(t, dir, "cr", "task", "list", "1", "--json")
	if runErr != nil {
		t.Fatalf("task list --json error = %v\noutput=%s", runErr, out)
	}
	env := decodeEnvelope(t, out)
	if !env.OK {
		t.Fatalf("expected ok envelope from task list --json, got %#v", env)
	}
	tasks, ok := env.Data["tasks"].([]any)
	if !ok || len(tasks) == 0 {
		t.Fatalf("expected tasks array, got %#v", env.Data["tasks"])
	}
	first, ok := tasks[0].(map[string]any)
	if !ok {
		t.Fatalf("expected first task object, got %#v", tasks[0])
	}
	if _, ok := first["checkpoint_commit"]; !ok {
		t.Fatalf("expected snake_case checkpoint_commit key, got %#v", first)
	}
	if _, ok := first["CheckpointCommit"]; ok {
		t.Fatalf("expected PascalCase key to be absent, got %#v", first)
	}
}

func TestCRMutationCommandsSupportJSON(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := service.New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	if _, err := svc.AddCR("Mutation JSON", "json coverage"); err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}

	out, _, runErr := runCLI(t, dir, "cr", "add", "Second JSON CR", "--description", "second", "--json")
	if runErr != nil {
		t.Fatalf("cr add --json error = %v\noutput=%s", runErr, out)
	}
	env := decodeEnvelope(t, out)
	if !env.OK {
		t.Fatalf("expected ok envelope from cr add --json, got %#v", env)
	}

	out, _, runErr = runCLI(t, dir, "cr", "note", "1", "first note", "--json")
	if runErr != nil {
		t.Fatalf("cr note --json error = %v\noutput=%s", runErr, out)
	}
	env = decodeEnvelope(t, out)
	if !env.OK {
		t.Fatalf("expected ok envelope from cr note --json, got %#v", env)
	}

	out, _, runErr = runCLI(t, dir, "cr", "edit", "1", "--title", "Mutation JSON Updated", "--json")
	if runErr != nil {
		t.Fatalf("cr edit --json error = %v\noutput=%s", runErr, out)
	}
	env = decodeEnvelope(t, out)
	if !env.OK {
		t.Fatalf("expected ok envelope from cr edit --json, got %#v", env)
	}

	out, _, runErr = runCLI(t, dir, "cr", "contract", "set", "1",
		"--why", "machine readability",
		"--scope", "internal/cli",
		"--non-goal", "no protocol changes",
		"--invariant", "git remains source of truth",
		"--blast-radius", "cli only",
		"--test-plan", "go test ./...",
		"--rollback-plan", "revert",
		"--json")
	if runErr != nil {
		t.Fatalf("cr contract set --json error = %v\noutput=%s", runErr, out)
	}
	env = decodeEnvelope(t, out)
	if !env.OK {
		t.Fatalf("expected ok envelope from contract set --json, got %#v", env)
	}

	out, _, runErr = runCLI(t, dir, "cr", "contract", "show", "1", "--json")
	if runErr != nil {
		t.Fatalf("cr contract show --json error = %v\noutput=%s", runErr, out)
	}
	env = decodeEnvelope(t, out)
	if !env.OK {
		t.Fatalf("expected ok envelope from contract show --json, got %#v", env)
	}
	if _, ok := env.Data["contract"]; !ok {
		t.Fatalf("expected contract payload, got %#v", env.Data)
	}

	out, _, runErr = runCLI(t, dir, "cr", "history", "1", "--json")
	if runErr != nil {
		t.Fatalf("cr history --json error = %v\noutput=%s", runErr, out)
	}
	env = decodeEnvelope(t, out)
	if !env.OK {
		t.Fatalf("expected ok envelope from history --json, got %#v", env)
	}

	out, _, runErr = runCLI(t, dir, "cr", "redact", "1", "--note-index", "1", "--reason", "cleanup", "--json")
	if runErr != nil {
		t.Fatalf("cr redact --json error = %v\noutput=%s", runErr, out)
	}
	env = decodeEnvelope(t, out)
	if !env.OK {
		t.Fatalf("expected ok envelope from redact --json, got %#v", env)
	}

	out, _, runErr = runCLI(t, dir, "cr", "switch", "1", "--json")
	if runErr != nil {
		t.Fatalf("cr switch --json error = %v\noutput=%s", runErr, out)
	}
	env = decodeEnvelope(t, out)
	if !env.OK {
		t.Fatalf("expected ok envelope from switch --json, got %#v", env)
	}

	out, _, runErr = runCLI(t, dir, "cr", "base", "set", "1", "--ref", "main", "--json")
	if runErr != nil {
		t.Fatalf("cr base set --json error = %v\noutput=%s", runErr, out)
	}
	env = decodeEnvelope(t, out)
	if !env.OK {
		t.Fatalf("expected ok envelope from base set --json, got %#v", env)
	}

	out, _, runErr = runCLI(t, dir, "cr", "task", "add", "1", "delegated task", "--json")
	if runErr != nil {
		t.Fatalf("cr task add --json error = %v\noutput=%s", runErr, out)
	}
	env = decodeEnvelope(t, out)
	if !env.OK {
		t.Fatalf("expected ok envelope from task add --json, got %#v", env)
	}
	taskMap, ok := env.Data["task"].(map[string]any)
	if !ok {
		t.Fatalf("expected task payload object, got %#v", env.Data["task"])
	}
	taskIDFloat, ok := taskMap["id"].(float64)
	if !ok {
		t.Fatalf("expected numeric task id, got %#v", taskMap["id"])
	}
	taskID := int(taskIDFloat)

	out, _, runErr = runCLI(t, dir, "cr", "task", "contract", "set", "1", strconv.Itoa(taskID), "--intent", "delegate", "--acceptance", "child linked", "--scope", "internal/cli", "--json")
	if runErr != nil {
		t.Fatalf("cr task contract set --json error = %v\noutput=%s", runErr, out)
	}
	env = decodeEnvelope(t, out)
	if !env.OK {
		t.Fatalf("expected ok envelope from task contract set --json, got %#v", env)
	}

	out, _, runErr = runCLI(t, dir, "cr", "child", "add", "child delegate target", "--json")
	if runErr != nil {
		t.Fatalf("cr child add --json error = %v\noutput=%s", runErr, out)
	}
	env = decodeEnvelope(t, out)
	if !env.OK {
		t.Fatalf("expected ok envelope from child add --json, got %#v", env)
	}
	childCR, ok := env.Data["cr"].(map[string]any)
	if !ok {
		t.Fatalf("expected child cr object, got %#v", env.Data["cr"])
	}
	childIDFloat, ok := childCR["id"].(float64)
	if !ok {
		t.Fatalf("expected child CR id, got %#v", childCR["id"])
	}
	childID := int(childIDFloat)

	out, _, runErr = runCLI(t, dir, "cr", "task", "delegate", "1", strconv.Itoa(taskID), "--child", strconv.Itoa(childID), "--json")
	if runErr != nil {
		t.Fatalf("cr task delegate --json error = %v\noutput=%s", runErr, out)
	}
	env = decodeEnvelope(t, out)
	if !env.OK {
		t.Fatalf("expected ok envelope from task delegate --json, got %#v", env)
	}

	out, _, runErr = runCLI(t, dir, "cr", "task", "undelegate", "1", strconv.Itoa(taskID), "--child", strconv.Itoa(childID), "--json")
	if runErr != nil {
		t.Fatalf("cr task undelegate --json error = %v\noutput=%s", runErr, out)
	}
	env = decodeEnvelope(t, out)
	if !env.OK {
		t.Fatalf("expected ok envelope from task undelegate --json, got %#v", env)
	}

	out, _, runErr = runCLI(t, dir, "cr", "restack", strconv.Itoa(childID), "--json")
	if runErr != nil {
		t.Fatalf("cr restack --json error = %v\noutput=%s", runErr, out)
	}
	env = decodeEnvelope(t, out)
	if !env.OK {
		t.Fatalf("expected ok envelope from restack --json, got %#v", env)
	}

	out, _, runErr = runCLI(t, dir, "cr", "task", "add", "1", "done task", "--json")
	if runErr != nil {
		t.Fatalf("cr task add(second) --json error = %v\noutput=%s", runErr, out)
	}
	env = decodeEnvelope(t, out)
	if !env.OK {
		t.Fatalf("expected ok envelope from second task add --json, got %#v", env)
	}
	secondTaskMap, ok := env.Data["task"].(map[string]any)
	if !ok {
		t.Fatalf("expected second task payload object, got %#v", env.Data["task"])
	}
	secondTaskIDFloat, ok := secondTaskMap["id"].(float64)
	if !ok {
		t.Fatalf("expected second task id, got %#v", secondTaskMap["id"])
	}
	secondTaskID := int(secondTaskIDFloat)
	out, _, runErr = runCLI(t, dir, "cr", "task", "contract", "set", "1", strconv.Itoa(secondTaskID), "--intent", "done", "--acceptance", "task completed", "--scope", "internal/cli", "--json")
	if runErr != nil {
		t.Fatalf("cr task contract set(second) --json error = %v\noutput=%s", runErr, out)
	}
	env = decodeEnvelope(t, out)
	if !env.OK {
		t.Fatalf("expected ok envelope from second task contract set --json, got %#v", env)
	}

	out, _, runErr = runCLI(t, dir, "cr", "task", "done", "1", strconv.Itoa(secondTaskID), "--no-checkpoint", "--no-checkpoint-reason", "metadata-only completion", "--json")
	if runErr != nil {
		t.Fatalf("cr task done --json error = %v\noutput=%s", runErr, out)
	}
	env = decodeEnvelope(t, out)
	if !env.OK {
		t.Fatalf("expected ok envelope from task done --json, got %#v", env)
	}
	if source, ok := env.Data["checkpoint_source"].(string); !ok || source != "task_no_checkpoint" {
		t.Fatalf("expected checkpoint_source task_no_checkpoint, got %#v", env.Data["checkpoint_source"])
	}
	if reason, ok := env.Data["no_checkpoint_reason"].(string); !ok || strings.TrimSpace(reason) == "" {
		t.Fatalf("expected no_checkpoint_reason, got %#v", env.Data["no_checkpoint_reason"])
	}

	outPath := filepath.Join(dir, "artifacts", "cr-1.bundle.json")
	out, _, runErr = runCLI(t, dir, "cr", "export", "1", "--json")
	if runErr != nil {
		t.Fatalf("cr export --json error = %v\noutput=%s", runErr, out)
	}
	env = decodeEnvelope(t, out)
	if !env.OK {
		t.Fatalf("expected ok envelope from export --json, got %#v", env)
	}
	if _, ok := env.Data["bundle"]; !ok {
		t.Fatalf("expected bundle payload in export --json data, got %#v", env.Data)
	}

	out, _, runErr = runCLI(t, dir, "cr", "export", "1", "--out", outPath, "--json")
	if runErr != nil {
		t.Fatalf("cr export --out --json error = %v\noutput=%s", runErr, out)
	}
	env = decodeEnvelope(t, out)
	if !env.OK {
		t.Fatalf("expected ok envelope from export --out --json, got %#v", env)
	}
	if _, err := os.Stat(outPath); err != nil {
		t.Fatalf("expected export output file at %s: %v", outPath, err)
	}
}

func TestCRTaskDoneJSONIncludesCommitType(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := service.New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	if _, err := svc.AddCR("Task done commit type", "json commit type"); err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	out, _, runErr := runCLI(t, dir, "cr", "switch", "1", "--json")
	if runErr != nil {
		t.Fatalf("cr switch --json error = %v\noutput=%s", runErr, out)
	}
	env := decodeEnvelope(t, out)
	if !env.OK {
		t.Fatalf("expected ok envelope from switch --json, got %#v", env)
	}

	out, _, runErr = runCLI(t, dir, "cr", "task", "add", "1", "Unprefixed task title", "--json")
	if runErr != nil {
		t.Fatalf("cr task add --json error = %v\noutput=%s", runErr, out)
	}
	env = decodeEnvelope(t, out)
	if !env.OK {
		t.Fatalf("expected ok envelope from task add --json, got %#v", env)
	}
	taskMap, ok := env.Data["task"].(map[string]any)
	if !ok {
		t.Fatalf("expected task payload object, got %#v", env.Data["task"])
	}
	taskIDFloat, ok := taskMap["id"].(float64)
	if !ok {
		t.Fatalf("expected numeric task id, got %#v", taskMap["id"])
	}
	taskID := int(taskIDFloat)

	out, _, runErr = runCLI(t, dir, "cr", "task", "contract", "set", "1", strconv.Itoa(taskID), "--intent", "Implement task checkpoint", "--acceptance", "done", "--scope", ".", "--json")
	if runErr != nil {
		t.Fatalf("cr task contract set --json error = %v\noutput=%s", runErr, out)
	}
	env = decodeEnvelope(t, out)
	if !env.OK {
		t.Fatalf("expected ok envelope from task contract set --json, got %#v", env)
	}

	if err := os.WriteFile(filepath.Join(dir, "commit_type.json"), []byte("checkpoint\n"), 0o644); err != nil {
		t.Fatalf("write commit type fixture: %v", err)
	}

	out, _, runErr = runCLI(t, dir, "cr", "task", "done", "1", strconv.Itoa(taskID), "--all", "--commit-type", "fix", "--json")
	if runErr != nil {
		t.Fatalf("cr task done --json error = %v\noutput=%s", runErr, out)
	}
	env = decodeEnvelope(t, out)
	if !env.OK {
		t.Fatalf("expected ok envelope from task done --json, got %#v", env)
	}
	if commitType, ok := env.Data["commit_type"].(string); !ok || commitType != "fix" {
		t.Fatalf("expected commit_type fix, got %#v", env.Data["commit_type"])
	}
}
