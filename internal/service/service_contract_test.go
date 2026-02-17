package service

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSetAndGetCRContractRoundTrip(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	cr, err := svc.AddCR("Contract", "roundtrip")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}

	why := "Reduce review bottlenecks."
	scope := []string{"internal/service", "cmd"}
	nonGoals := []string{"No cloud sync"}
	invariants := []string{"Git remains source of truth"}
	blastRadius := "CLI and service layer only."
	testPlan := "go test ./... && go vet ./..."
	rollback := "Revert CR merge commit."

	changed, err := svc.SetCRContract(cr.ID, ContractPatch{
		Why:          &why,
		Scope:        &scope,
		NonGoals:     &nonGoals,
		Invariants:   &invariants,
		BlastRadius:  &blastRadius,
		TestPlan:     &testPlan,
		RollbackPlan: &rollback,
	})
	if err != nil {
		t.Fatalf("SetCRContract() error = %v", err)
	}
	if len(changed) != 7 {
		t.Fatalf("expected 7 changed fields, got %#v", changed)
	}

	got, err := svc.GetCRContract(cr.ID)
	if err != nil {
		t.Fatalf("GetCRContract() error = %v", err)
	}
	if got.Why != why || got.BlastRadius != blastRadius || got.TestPlan != testPlan || got.RollbackPlan != rollback {
		t.Fatalf("unexpected scalar contract values: %#v", got)
	}
	if len(got.Scope) != 2 || got.Scope[0] != "cmd" || got.Scope[1] != "internal/service" {
		t.Fatalf("unexpected scope values: %#v", got.Scope)
	}
	if len(got.NonGoals) != 1 || got.NonGoals[0] != nonGoals[0] {
		t.Fatalf("unexpected non-goals: %#v", got.NonGoals)
	}
	if len(got.Invariants) != 1 || got.Invariants[0] != invariants[0] {
		t.Fatalf("unexpected invariants: %#v", got.Invariants)
	}
	if strings.TrimSpace(got.UpdatedAt) == "" || strings.TrimSpace(got.UpdatedBy) == "" {
		t.Fatalf("expected contract update audit fields, got %#v", got)
	}
}

func TestSetCRContractPartialUpdateOnlyMutatesTargetFields(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	cr, err := svc.AddCR("Contract", "partial")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	setValidContract(t, svc, cr.ID)

	why := "Updated intent rationale."
	changed, err := svc.SetCRContract(cr.ID, ContractPatch{Why: &why})
	if err != nil {
		t.Fatalf("SetCRContract(partial) error = %v", err)
	}
	if len(changed) != 1 || changed[0] != "why" {
		t.Fatalf("expected only why to change, got %#v", changed)
	}

	got, err := svc.GetCRContract(cr.ID)
	if err != nil {
		t.Fatalf("GetCRContract() error = %v", err)
	}
	if got.Why != why {
		t.Fatalf("expected why %q, got %q", why, got.Why)
	}
	if len(got.Scope) == 0 || got.TestPlan == "" {
		t.Fatalf("expected existing fields to remain populated, got %#v", got)
	}
}

func TestWhyCRUsesContractThenDescriptionThenMissing(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	cr, err := svc.AddCR("Why precedence", "fallback description")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}

	view, err := svc.WhyCR(cr.ID)
	if err != nil {
		t.Fatalf("WhyCR() error = %v", err)
	}
	if view.Source != "description" || view.EffectiveWhy != "fallback description" {
		t.Fatalf("expected description fallback, got %#v", view)
	}

	why := "contract why wins"
	if _, err := svc.SetCRContract(cr.ID, ContractPatch{Why: &why}); err != nil {
		t.Fatalf("SetCRContract(why) error = %v", err)
	}
	view, err = svc.WhyCR(cr.ID)
	if err != nil {
		t.Fatalf("WhyCR() error = %v", err)
	}
	if view.Source != "contract_why" || view.EffectiveWhy != why {
		t.Fatalf("expected contract why precedence, got %#v", view)
	}

	emptyDescCR, err := svc.AddCR("No why", "")
	if err != nil {
		t.Fatalf("AddCR(empty desc) error = %v", err)
	}
	view, err = svc.WhyCR(emptyDescCR.ID)
	if err != nil {
		t.Fatalf("WhyCR(empty) error = %v", err)
	}
	if view.Source != "missing" || view.EffectiveWhy != "" {
		t.Fatalf("expected missing why source, got %#v", view)
	}
}

func TestStatusCRReflectsReadinessAndWorkspaceState(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	cr, err := svc.AddCR("Status view", "status details")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	setValidContract(t, svc, cr.ID)

	task1, err := svc.AddTask(cr.ID, "feat: complete one task")
	if err != nil {
		t.Fatalf("AddTask() #1 error = %v", err)
	}
	task2, err := svc.AddTask(cr.ID, "feat: leave one open")
	if err != nil {
		t.Fatalf("AddTask() #2 error = %v", err)
	}
	setValidTaskContract(t, svc, cr.ID, task1.ID)
	setValidTaskContract(t, svc, cr.ID, task2.ID)
	if err := svc.DoneTask(cr.ID, task1.ID); err != nil {
		t.Fatalf("DoneTask() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "untracked.txt"), []byte("x\n"), 0o644); err != nil {
		t.Fatalf("write untracked file: %v", err)
	}

	status, err := svc.StatusCR(cr.ID)
	if err != nil {
		t.Fatalf("StatusCR() error = %v", err)
	}
	if status.ID != cr.ID || status.Title != cr.Title {
		t.Fatalf("unexpected status identity: %#v", status)
	}
	if !status.BranchMatch {
		t.Fatalf("expected branch match on active CR branch, got %#v", status)
	}
	if !status.Dirty || status.UntrackedCount == 0 {
		t.Fatalf("expected dirty workspace with untracked file, got %#v", status)
	}
	if status.TasksTotal != 2 || status.TasksDone != 1 || status.TasksOpen != 1 {
		t.Fatalf("unexpected task progress counts: %#v", status)
	}
	if !status.ContractComplete || len(status.ContractMissingFields) != 0 {
		t.Fatalf("expected complete contract, got %#v", status)
	}
	if !status.ValidationValid || status.ValidationErrors != 0 || status.MergeBlocked {
		t.Fatalf("expected merge-ready validation summary, got %#v", status)
	}
}

func TestSetAndGetTaskContractRoundTrip(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	cr, err := svc.AddCR("Task contract", "roundtrip")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	task, err := svc.AddTask(cr.ID, "Implement task contract")
	if err != nil {
		t.Fatalf("AddTask() error = %v", err)
	}

	intent := "Ensure task intent is explicit."
	acceptance := []string{"Task can be completed safely."}
	scope := []string{"internal/service"}
	changed, err := svc.SetTaskContract(cr.ID, task.ID, TaskContractPatch{
		Intent:             &intent,
		AcceptanceCriteria: &acceptance,
		Scope:              &scope,
	})
	if err != nil {
		t.Fatalf("SetTaskContract() error = %v", err)
	}
	if len(changed) != 3 {
		t.Fatalf("expected 3 changed fields, got %#v", changed)
	}

	got, err := svc.GetTaskContract(cr.ID, task.ID)
	if err != nil {
		t.Fatalf("GetTaskContract() error = %v", err)
	}
	if got.Intent != intent || len(got.AcceptanceCriteria) != 1 || len(got.Scope) != 1 {
		t.Fatalf("unexpected task contract values: %#v", got)
	}
	if strings.TrimSpace(got.UpdatedAt) == "" || strings.TrimSpace(got.UpdatedBy) == "" {
		t.Fatalf("expected task contract update audit fields, got %#v", got)
	}
}

func TestDoneTaskBlockedWhenTaskContractMissing(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	cr, err := svc.AddCR("Missing task contract", "block done")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	task, err := svc.AddTask(cr.ID, "Task without contract")
	if err != nil {
		t.Fatalf("AddTask() error = %v", err)
	}

	err = svc.DoneTask(cr.ID, task.ID)
	if !errors.Is(err, ErrTaskContractIncomplete) {
		t.Fatalf("expected ErrTaskContractIncomplete, got %v", err)
	}
}

func TestValidateCRFailsWhenRequiredContractFieldsMissing(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	cr, err := svc.AddCR("Missing contract", "should fail validation")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}

	report, err := svc.ValidateCR(cr.ID)
	if err != nil {
		t.Fatalf("ValidateCR() error = %v", err)
	}
	if report.Valid {
		t.Fatalf("expected report to be invalid")
	}
	if len(report.Errors) < 7 {
		t.Fatalf("expected required-field errors, got %#v", report.Errors)
	}
}

func TestValidateCRDetectsScopeDriftAsError(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")

	cr, err := svc.AddCR("Scope drift", "drift")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	scope := []string{"internal/service"}
	why := "Contain service changes."
	nonGoals := []string{"No docs changes"}
	invariants := []string{"No API changes"}
	blast := "service only"
	testPlan := "go test ./..."
	rollback := "revert"
	if _, err := svc.SetCRContract(cr.ID, ContractPatch{
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
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("drift\n"), 0o644); err != nil {
		t.Fatalf("write README: %v", err)
	}
	runGit(t, dir, "add", "README.md")
	runGit(t, dir, "commit", "-m", "docs: drift")

	report, err := svc.ValidateCR(cr.ID)
	if err != nil {
		t.Fatalf("ValidateCR() error = %v", err)
	}
	if report.Valid {
		t.Fatalf("expected validation failure due to scope drift")
	}
	if !containsAny(report.Errors, "scope drift") {
		t.Fatalf("expected scope drift error, got %#v", report.Errors)
	}
}

func TestValidateCRWarnsOnTaskScopeMismatch(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")

	cr, err := svc.AddCR("Task warning", "task scope warning")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	task, err := svc.AddTask(cr.ID, "docs: update task")
	if err != nil {
		t.Fatalf("AddTask() error = %v", err)
	}
	setValidContract(t, svc, cr.ID)

	if err := os.MkdirAll(filepath.Join(dir, "docs"), 0o755); err != nil {
		t.Fatalf("mkdir docs: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "docs", "in_scope.md"), []byte("ok\n"), 0o644); err != nil {
		t.Fatalf("write docs file: %v", err)
	}
	runGit(t, dir, "add", "docs/in_scope.md")
	runGit(t, dir, "commit", "-m", "docs: in scope")

	loaded, err := svc.store.LoadCR(cr.ID)
	if err != nil {
		t.Fatalf("LoadCR() error = %v", err)
	}
	loaded.Subtasks[task.ID-1].Status = "done"
	loaded.Subtasks[task.ID-1].CheckpointScope = []string{"outside/path.md"}
	if err := svc.store.SaveCR(loaded); err != nil {
		t.Fatalf("SaveCR() error = %v", err)
	}

	scope := []string{"docs"}
	if _, err := svc.SetCRContract(cr.ID, ContractPatch{Scope: &scope}); err != nil {
		t.Fatalf("SetCRContract(scope) error = %v", err)
	}

	report, err := svc.ValidateCR(cr.ID)
	if err != nil {
		t.Fatalf("ValidateCR() error = %v", err)
	}
	if len(report.Warnings) == 0 {
		t.Fatalf("expected task scope warning, got none")
	}
}

func TestValidateCRWarnsOnTaskContractScopeDrift(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")

	cr, err := svc.AddCR("Task contract drift", "warn on drift")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	task, err := svc.AddTask(cr.ID, "feat: drift task")
	if err != nil {
		t.Fatalf("AddTask() error = %v", err)
	}
	setValidContract(t, svc, cr.ID)
	setValidTaskContract(t, svc, cr.ID, task.ID)

	if err := os.WriteFile(filepath.Join(dir, "outside.txt"), []byte("x\n"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	loaded, err := svc.store.LoadCR(cr.ID)
	if err != nil {
		t.Fatalf("LoadCR() error = %v", err)
	}
	loaded.Subtasks[task.ID-1].Status = "done"
	loaded.Subtasks[task.ID-1].CheckpointScope = []string{"outside.txt"}
	loaded.Subtasks[task.ID-1].Contract.Scope = []string{"internal/service"}
	if err := svc.store.SaveCR(loaded); err != nil {
		t.Fatalf("SaveCR() error = %v", err)
	}

	report, err := svc.ValidateCR(cr.ID)
	if err != nil {
		t.Fatalf("ValidateCR() error = %v", err)
	}
	if !containsAny(report.Warnings, "outside task contract scope") {
		t.Fatalf("expected task contract drift warning, got %#v", report.Warnings)
	}
}

func TestImpactCRAppliesRiskSignalsDeterministically(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")

	if err := os.WriteFile(filepath.Join(dir, "delete_me.txt"), []byte("base\n"), 0o644); err != nil {
		t.Fatalf("write base file: %v", err)
	}
	runGit(t, dir, "add", "delete_me.txt")
	runGit(t, dir, "commit", "-m", "chore: base file")

	cr, err := svc.AddCR("Impact", "risk scoring")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	setValidContract(t, svc, cr.ID)

	if err := os.MkdirAll(filepath.Join(dir, "internal", "service"), 0o755); err != nil {
		t.Fatalf("mkdir internal/service: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "internal", "service", "x.go"), []byte("package service\n"), 0o644); err != nil {
		t.Fatalf("write critical file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module tmp\n"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	runGit(t, dir, "rm", "delete_me.txt")
	runGit(t, dir, "add", "internal/service/x.go", "go.mod")
	runGit(t, dir, "commit", "-m", "feat: risky change")

	impact, err := svc.ImpactCR(cr.ID)
	if err != nil {
		t.Fatalf("ImpactCR() error = %v", err)
	}
	if impact.RiskTier != "high" {
		t.Fatalf("expected high risk tier, got %q (score=%d)", impact.RiskTier, impact.RiskScore)
	}
	for _, code := range []string{"critical_paths", "dependency_changes", "deletions", "no_test_changes"} {
		if !containsSignal(impact.Signals, code) {
			t.Fatalf("expected risk signal %q, got %#v", code, impact.Signals)
		}
	}
}

func TestMergeCRBlockedWithoutOverrideWhenValidationFails(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")

	cr, err := svc.AddCR("Blocked merge", "validation should fail")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "blocked.txt"), []byte("x\n"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	runGit(t, dir, "add", "blocked.txt")
	runGit(t, dir, "commit", "-m", "feat: blocked")

	_, err = svc.MergeCR(cr.ID, false, "")
	if !errors.Is(err, ErrCRValidationFailed) {
		t.Fatalf("expected ErrCRValidationFailed, got %v", err)
	}
}

func TestMergeCROverridePersistsAuditEvent(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")

	cr, err := svc.AddCR("Override merge", "intent")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "override.txt"), []byte("x\n"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	runGit(t, dir, "add", "override.txt")
	runGit(t, dir, "commit", "-m", "feat: override")

	sha, err := svc.MergeCR(cr.ID, false, "emergency hotfix")
	if err != nil {
		t.Fatalf("MergeCR(override) error = %v", err)
	}
	if sha == "" {
		t.Fatalf("expected merge sha")
	}

	loaded, err := svc.store.LoadCR(cr.ID)
	if err != nil {
		t.Fatalf("LoadCR() error = %v", err)
	}
	found := false
	for _, event := range loaded.Events {
		if event.Type == "cr_merge_overridden" {
			found = true
			if event.Meta["override_reason"] != "emergency hotfix" {
				t.Fatalf("unexpected override reason meta: %#v", event.Meta)
			}
			break
		}
	}
	if !found {
		t.Fatalf("expected cr_merge_overridden event, got %#v", loaded.Events)
	}
}

func containsSignal(signals []RiskSignal, code string) bool {
	for _, signal := range signals {
		if signal.Code == code {
			return true
		}
	}
	return false
}

func containsAny(items []string, needle string) bool {
	for _, item := range items {
		if strings.Contains(item, needle) {
			return true
		}
	}
	return false
}
