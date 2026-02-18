package service

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRepoPolicyDefaultsWhenFileMissing(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if err := os.Remove(filepath.Join(dir, repoPolicyFileName)); err != nil {
		t.Fatalf("remove policy file: %v", err)
	}

	policy, err := svc.repoPolicy()
	if err != nil {
		t.Fatalf("repoPolicy() error = %v", err)
	}
	if strings.TrimSpace(policy.Version) != "v1" {
		t.Fatalf("expected version v1, got %#v", policy)
	}
	if len(policy.Contract.RequiredFields) == 0 || !containsAny(policy.Contract.RequiredFields, "why") {
		t.Fatalf("expected default contract required fields, got %#v", policy.Contract.RequiredFields)
	}
	if len(policy.TaskContract.RequiredFields) == 0 || !containsAny(policy.TaskContract.RequiredFields, "intent") {
		t.Fatalf("expected default task required fields, got %#v", policy.TaskContract.RequiredFields)
	}
	if len(policy.Scope.AllowedPrefixes) == 0 || policy.Scope.AllowedPrefixes[0] != "." {
		t.Fatalf("expected default allowed prefix '.', got %#v", policy.Scope.AllowedPrefixes)
	}
	if !policyAllowsMergeOverride(policy) {
		t.Fatalf("expected default merge override to be allowed")
	}
}

func TestRepoPolicyRejectsUnknownField(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	writePolicyFileForTest(t, dir, `version: v1
unknown_key: true
`)

	_, err := svc.repoPolicy()
	if !errors.Is(err, ErrPolicyInvalid) {
		t.Fatalf("expected ErrPolicyInvalid, got %v", err)
	}
}

func TestRepoPolicyRejectsInvalidRequiredFieldEnum(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	writePolicyFileForTest(t, dir, `version: v1
contract:
  required_fields:
    - why
    - not_a_real_field
`)

	_, err := svc.repoPolicy()
	if !errors.Is(err, ErrPolicyInvalid) {
		t.Fatalf("expected ErrPolicyInvalid, got %v", err)
	}
}

func TestRepoPolicyRejectsInvalidAllowedPrefix(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	writePolicyFileForTest(t, dir, `version: v1
scope:
  allowed_prefixes:
    - ../outside
`)

	_, err := svc.repoPolicy()
	if !errors.Is(err, ErrPolicyInvalid) {
		t.Fatalf("expected ErrPolicyInvalid, got %v", err)
	}
}

func TestInitSeedsRepoPolicyFileIdempotently(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() first error = %v", err)
	}
	policyPath := filepath.Join(dir, repoPolicyFileName)
	first, err := os.ReadFile(policyPath)
	if err != nil {
		t.Fatalf("read policy after first init: %v", err)
	}
	if strings.TrimSpace(string(first)) == "" {
		t.Fatalf("expected non-empty policy file")
	}
	if strings.TrimSpace(string(first)) != strings.TrimSpace(repoPolicyTemplate) {
		t.Fatalf("expected seeded policy to match deterministic template")
	}

	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() second error = %v", err)
	}
	second, err := os.ReadFile(policyPath)
	if err != nil {
		t.Fatalf("read policy after second init: %v", err)
	}
	if string(first) != string(second) {
		t.Fatalf("expected idempotent policy content")
	}
}

func TestInitDoesNotOverwriteExistingRepoPolicyFile(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() first error = %v", err)
	}
	custom := strings.TrimSpace(`version: v1
merge:
  allow_override: false
`) + "\n"
	policyPath := filepath.Join(dir, repoPolicyFileName)
	if err := os.WriteFile(policyPath, []byte(custom), 0o644); err != nil {
		t.Fatalf("write custom policy: %v", err)
	}

	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() second error = %v", err)
	}
	got, err := os.ReadFile(policyPath)
	if err != nil {
		t.Fatalf("read policy after second init: %v", err)
	}
	if string(got) != custom {
		t.Fatalf("expected existing policy file to be preserved, got:\n%s", string(got))
	}
}

func TestPolicyCustomCRRequiredFieldsAffectValidate(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	writePolicyFileForTest(t, dir, `version: v1
contract:
  required_fields:
    - why
`)
	cr, err := svc.AddCR("policy required fields", "")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}

	report, err := svc.ValidateCR(cr.ID)
	if err != nil {
		t.Fatalf("ValidateCR() error = %v", err)
	}
	if report.Valid {
		t.Fatalf("expected validation failure when required why is missing")
	}
	if len(report.Errors) != 1 || !containsAny(report.Errors, "missing required contract field: why") {
		t.Fatalf("expected only missing why error, got %#v", report.Errors)
	}
}

func TestPolicyCustomTaskRequiredFieldsAffectTaskDone(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	writePolicyFileForTest(t, dir, `version: v1
task_contract:
  required_fields:
    - intent
`)
	cr, err := svc.AddCR("task policy", "")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	task, err := svc.AddTask(cr.ID, "task")
	if err != nil {
		t.Fatalf("AddTask() error = %v", err)
	}

	err = svc.DoneTask(cr.ID, task.ID)
	if !errors.Is(err, ErrTaskContractIncomplete) {
		t.Fatalf("expected ErrTaskContractIncomplete, got %v", err)
	}
	if !containsAny([]string{err.Error()}, "intent") {
		t.Fatalf("expected incomplete reason to mention intent, got %v", err)
	}

	intent := "Task intent"
	if _, err := svc.SetTaskContract(cr.ID, task.ID, TaskContractPatch{Intent: &intent}); err != nil {
		t.Fatalf("SetTaskContract(intent only) error = %v", err)
	}
	if err := svc.DoneTask(cr.ID, task.ID); err != nil {
		t.Fatalf("DoneTask() with policy-minimum contract error = %v", err)
	}
}

func TestPolicyScopeAllowlistBlocksContractWrites(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	writePolicyFileForTest(t, dir, `version: v1
scope:
  allowed_prefixes:
    - internal/service
`)
	cr, err := svc.AddCR("scope policy", "")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	outside := []string{"cmd"}
	if _, err := svc.SetCRContract(cr.ID, ContractPatch{Scope: &outside}); !errors.Is(err, ErrPolicyViolation) {
		t.Fatalf("expected ErrPolicyViolation for CR scope, got %v", err)
	}
	task, err := svc.AddTask(cr.ID, "scope task")
	if err != nil {
		t.Fatalf("AddTask() error = %v", err)
	}
	intent := "task"
	acceptance := []string{"ok"}
	if _, err := svc.SetTaskContract(cr.ID, task.ID, TaskContractPatch{
		Intent:             &intent,
		AcceptanceCriteria: &acceptance,
		Scope:              &outside,
	}); !errors.Is(err, ErrPolicyViolation) {
		t.Fatalf("expected ErrPolicyViolation for task scope, got %v", err)
	}
}

func TestPolicyScopeAllowlistAppearsInValidateForExistingData(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	cr, err := svc.AddCR("existing scope", "")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	setValidContract(t, svc, cr.ID)
	task, err := svc.AddTask(cr.ID, "task")
	if err != nil {
		t.Fatalf("AddTask() error = %v", err)
	}
	setValidTaskContract(t, svc, cr.ID, task.ID)
	writePolicyFileForTest(t, dir, `version: v1
scope:
  allowed_prefixes:
    - internal/service
`)

	report, err := svc.ValidateCR(cr.ID)
	if err != nil {
		t.Fatalf("ValidateCR() error = %v", err)
	}
	if report.Valid {
		t.Fatalf("expected validation failure due to policy scope violations")
	}
	if !containsAny(report.Errors, "policy scope violation: cr contract scope") {
		t.Fatalf("expected CR contract policy scope violation, got %#v", report.Errors)
	}
	if !containsAny(report.Errors, "policy scope violation: task #1 contract scope") {
		t.Fatalf("expected task contract policy scope violation, got %#v", report.Errors)
	}
}

func TestPolicyScopeAllowlistBlocksPlanApplyContractScopes(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	writePolicyFileForTest(t, dir, `version: v1
scope:
  allowed_prefixes:
    - internal/service
`)
	planPath := writePolicyPlanFileForTest(t, dir, `version: v1
crs:
  - key: cr1
    title: "Scoped"
    description: "x"
    contract:
      scope:
        - cmd
`)

	_, err := svc.ApplyCRPlan(ApplyCRPlanOptions{FilePath: planPath, DryRun: true})
	if !errors.Is(err, ErrPolicyViolation) {
		t.Fatalf("expected ErrPolicyViolation, got %v", err)
	}
}

func TestPolicyClassificationOverridesDiffClassification(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")
	writePolicyFileForTest(t, dir, `version: v1
classification:
  test:
    suffixes:
      - ".scenario"
    path_contains:
      - "/qa/"
  dependency:
    file_names:
      - "deps.lock"
`)

	cr, err := svc.AddCR("classification policy", "")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	setValidContract(t, svc, cr.ID)

	if err := os.WriteFile(filepath.Join(dir, "unit_test.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("write unit_test.go: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "run.scenario"), []byte("scenario\n"), 0o644); err != nil {
		t.Fatalf("write run.scenario: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example\n"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "deps.lock"), []byte("deps\n"), 0o644); err != nil {
		t.Fatalf("write deps.lock: %v", err)
	}
	runGit(t, dir, "add", "unit_test.go", "run.scenario", "go.mod", "deps.lock")
	runGit(t, dir, "commit", "-m", "feat: policy classification")

	impact, err := svc.ImpactCR(cr.ID)
	if err != nil {
		t.Fatalf("ImpactCR() error = %v", err)
	}
	if !containsString(impact.TestFiles, "run.scenario") {
		t.Fatalf("expected run.scenario in test files, got %#v", impact.TestFiles)
	}
	if containsString(impact.TestFiles, "unit_test.go") {
		t.Fatalf("did not expect unit_test.go in test files with override policy, got %#v", impact.TestFiles)
	}
	if !containsString(impact.DependencyFiles, "deps.lock") {
		t.Fatalf("expected deps.lock in dependency files, got %#v", impact.DependencyFiles)
	}
	if containsString(impact.DependencyFiles, "go.mod") {
		t.Fatalf("did not expect go.mod in dependency files with override policy, got %#v", impact.DependencyFiles)
	}
}

func TestPolicyMergeAllowOverrideFalseBlocksMergeOverride(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")
	writePolicyFileForTest(t, dir, `version: v1
merge:
  allow_override: false
`)

	cr, err := svc.AddCR("override policy", "")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "override-policy.txt"), []byte("x\n"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	runGit(t, dir, "add", "override-policy.txt")
	runGit(t, dir, "commit", "-m", "feat: override policy test")

	_, err = svc.MergeCR(cr.ID, false, "emergency")
	if !errors.Is(err, ErrPolicyViolation) {
		t.Fatalf("expected ErrPolicyViolation, got %v", err)
	}
}

func TestPolicyMergeAllowOverrideFalseBlocksResumeOverride(t *testing.T) {
	svc, cr, dir := setupMergeConflictScenario(t)

	if _, _, err := svc.MergeCRWithWarnings(cr.ID, false, ""); err == nil {
		t.Fatalf("expected merge conflict")
	}
	writePolicyFileForTest(t, dir, `version: v1
merge:
  allow_override: false
`)

	if err := os.WriteFile(filepath.Join(dir, "conflict.txt"), []byte("resolved\n"), 0o644); err != nil {
		t.Fatalf("write resolved file: %v", err)
	}
	runGit(t, dir, "add", "conflict.txt")

	_, _, err := svc.ResumeMergeCR(cr.ID, false, "emergency")
	if !errors.Is(err, ErrPolicyViolation) {
		t.Fatalf("expected ErrPolicyViolation, got %v", err)
	}
}

func writePolicyFileForTest(t *testing.T, dir, content string) {
	t.Helper()
	path := filepath.Join(dir, repoPolicyFileName)
	normalized := strings.TrimSpace(content) + "\n"
	if err := os.WriteFile(path, []byte(normalized), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func writePolicyPlanFileForTest(t *testing.T, dir, content string) string {
	t.Helper()
	path := filepath.Join(dir, "plan-policy.yaml")
	if err := os.WriteFile(path, []byte(strings.TrimSpace(content)+"\n"), 0o644); err != nil {
		t.Fatalf("write plan file %s: %v", path, err)
	}
	return path
}
