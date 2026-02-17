package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const validCRPlanYAML = `version: v1
crs:
  - key: parent_refactor
    title: "Parent refactor"
    description: "Parent intent"
    base: "main"
    contract:
      why: "Parent why"
      scope:
        - "internal/service"
      non_goals:
        - "No unrelated refactors"
      invariants:
        - "Compatibility preserved"
      blast_radius: "Service layer"
      test_plan: "go test ./..."
      rollback_plan: "revert"
    tasks:
      - key: split_service
        title: "Split service"
        contract:
          intent: "Split"
          acceptance_criteria:
            - "Service split"
          scope:
            - "internal/service"
        delegate_to:
          - "child_cli"
  - key: child_cli
    title: "Child cli"
    description: "Child intent"
    parent_key: "parent_refactor"
    contract:
      why: "Child why"
      scope:
        - "internal/cli"
      non_goals:
        - "No command semantic changes"
      invariants:
        - "Output stable"
      blast_radius: "CLI layer"
      test_plan: "go test ./internal/cli"
      rollback_plan: "revert"
    tasks:
      - key: split_cli
        title: "Split cli"
        contract:
          intent: "Split cli"
          acceptance_criteria:
            - "CLI split"
          scope:
            - "internal/cli"
`

func TestApplyCRPlanDryRunDoesNotMutate(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	planPath := writePlanFile(t, dir, "plan.yaml", validCRPlanYAML)

	result, err := svc.ApplyCRPlan(ApplyCRPlanOptions{FilePath: planPath, DryRun: true})
	if err != nil {
		t.Fatalf("ApplyCRPlan(dry-run) error = %v", err)
	}
	if !result.DryRun {
		t.Fatalf("expected dry_run result")
	}
	if result.Consumed {
		t.Fatalf("expected consumed=false for dry-run")
	}
	if len(result.CreatedCRs) != 2 {
		t.Fatalf("expected 2 predicted CRs, got %#v", result.CreatedCRs)
	}
	if result.CreatedCRs[0].ID != 1 || result.CreatedCRs[1].ID != 2 {
		t.Fatalf("unexpected predicted IDs: %#v", result.CreatedCRs)
	}
	if len(result.CreatedTasks) != 2 {
		t.Fatalf("expected 2 predicted tasks, got %#v", result.CreatedTasks)
	}
	if len(result.Delegations) != 1 || result.Delegations[0].ChildTaskID != 2 {
		t.Fatalf("unexpected dry-run delegations: %#v", result.Delegations)
	}
	if _, err := os.Stat(planPath); err != nil {
		t.Fatalf("plan file should remain after dry-run: %v", err)
	}
	crs, err := svc.ListCRs()
	if err != nil {
		t.Fatalf("ListCRs() error = %v", err)
	}
	if len(crs) != 0 {
		t.Fatalf("expected no CR mutation on dry-run, got %d", len(crs))
	}
	idx, err := svc.store.LoadIndex()
	if err != nil {
		t.Fatalf("LoadIndex() error = %v", err)
	}
	if idx.NextID != 1 {
		t.Fatalf("expected next id to remain 1, got %d", idx.NextID)
	}
	branch, err := svc.git.CurrentBranch()
	if err != nil {
		t.Fatalf("CurrentBranch() error = %v", err)
	}
	if branch != "main" {
		t.Fatalf("expected branch restored to main, got %q", branch)
	}
}

func TestApplyCRPlanCreatesStackAndConsumesByDefault(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	planPath := writePlanFile(t, dir, "plan.yaml", validCRPlanYAML)

	result, err := svc.ApplyCRPlan(ApplyCRPlanOptions{FilePath: planPath})
	if err != nil {
		t.Fatalf("ApplyCRPlan() error = %v", err)
	}
	if !result.Consumed {
		t.Fatalf("expected plan file consumed by default")
	}
	if _, err := os.Stat(planPath); !os.IsNotExist(err) {
		t.Fatalf("expected plan file removed, stat err=%v", err)
	}
	if len(result.CreatedCRs) != 2 || len(result.CreatedTasks) != 2 || len(result.Delegations) != 1 {
		t.Fatalf("unexpected apply result: %#v", result)
	}

	parentID := findCreatedCRID(t, result, "parent_refactor")
	childID := findCreatedCRID(t, result, "child_cli")
	if parentID <= 0 || childID <= 0 {
		t.Fatalf("expected parent/child IDs in apply result: %#v", result.CreatedCRs)
	}

	stack, err := svc.StackCR(parentID)
	if err != nil {
		t.Fatalf("StackCR() error = %v", err)
	}
	if len(stack.Nodes) != 2 {
		t.Fatalf("expected 2 stack nodes, got %#v", stack.Nodes)
	}

	parentTasks, err := svc.ListTasks(parentID)
	if err != nil {
		t.Fatalf("ListTasks(parent) error = %v", err)
	}
	if len(parentTasks) != 1 {
		t.Fatalf("expected 1 parent task, got %#v", parentTasks)
	}
	if parentTasks[0].Status != "delegated" {
		t.Fatalf("expected parent task delegated, got %#v", parentTasks[0])
	}
	if len(parentTasks[0].Delegations) != 1 {
		t.Fatalf("expected one delegation on parent task, got %#v", parentTasks[0])
	}

	childTasks, err := svc.ListTasks(childID)
	if err != nil {
		t.Fatalf("ListTasks(child) error = %v", err)
	}
	if len(childTasks) != 2 {
		t.Fatalf("expected child explicit+delegated tasks, got %#v", childTasks)
	}
}

func TestApplyCRPlanKeepFilePreservesPlan(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	planPath := writePlanFile(t, dir, "plan.yaml", validCRPlanYAML)

	result, err := svc.ApplyCRPlan(ApplyCRPlanOptions{FilePath: planPath, KeepFile: true})
	if err != nil {
		t.Fatalf("ApplyCRPlan(keep-file) error = %v", err)
	}
	if result.Consumed {
		t.Fatalf("expected consumed=false when keep-file=true")
	}
	if _, err := os.Stat(planPath); err != nil {
		t.Fatalf("expected plan file to remain: %v", err)
	}
}

func TestApplyCRPlanRejectsInvalidVersion(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	planPath := writePlanFile(t, dir, "invalid-version.yaml", strings.Replace(validCRPlanYAML, "version: v1", "version: v2", 1))

	_, err := svc.ApplyCRPlan(ApplyCRPlanOptions{FilePath: planPath, DryRun: true})
	if err == nil || !strings.Contains(err.Error(), "invalid plan version") {
		t.Fatalf("expected invalid version error, got %v", err)
	}
}

func TestApplyCRPlanRejectsUnknownFields(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	invalid := "version: v1\nunknown_field: true\ncrs: []\n"
	planPath := writePlanFile(t, dir, "unknown-field.yaml", invalid)

	_, err := svc.ApplyCRPlan(ApplyCRPlanOptions{FilePath: planPath, DryRun: true})
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "field") {
		t.Fatalf("expected strict field validation error, got %v", err)
	}
}

func TestApplyCRPlanRejectsBaseAndParentConflict(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	invalid := `version: v1
crs:
  - key: p
    title: "Parent"
    description: "x"
  - key: c
    title: "Child"
    description: "x"
    base: "main"
    parent_key: "p"
`
	planPath := writePlanFile(t, dir, "base-parent-conflict.yaml", invalid)

	_, err := svc.ApplyCRPlan(ApplyCRPlanOptions{FilePath: planPath, DryRun: true})
	if err == nil || !strings.Contains(err.Error(), "cannot define both base and parent_key") {
		t.Fatalf("expected base/parent conflict error, got %v", err)
	}
}

func TestApplyCRPlanRejectsParentCycles(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	invalid := `version: v1
crs:
  - key: a
    title: "A"
    description: "x"
    parent_key: "b"
  - key: b
    title: "B"
    description: "x"
    parent_key: "a"
`
	planPath := writePlanFile(t, dir, "cycle.yaml", invalid)

	_, err := svc.ApplyCRPlan(ApplyCRPlanOptions{FilePath: planPath, DryRun: true})
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "cycle") {
		t.Fatalf("expected cycle error, got %v", err)
	}
}

func TestApplyCRPlanRejectsDelegationToNonChild(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	invalid := `version: v1
crs:
  - key: parent
    title: "Parent"
    description: "x"
    tasks:
      - key: t1
        title: "Task"
        delegate_to:
          - "sibling"
  - key: sibling
    title: "Sibling"
    description: "x"
    base: "main"
`
	planPath := writePlanFile(t, dir, "delegation-invalid.yaml", invalid)

	_, err := svc.ApplyCRPlan(ApplyCRPlanOptions{FilePath: planPath, DryRun: true})
	if err == nil || !strings.Contains(err.Error(), "direct child CR") {
		t.Fatalf("expected direct-child delegation error, got %v", err)
	}
}

func TestInitSeedsCRPlanSampleTemplateIdempotently(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() first error = %v", err)
	}
	samplePath := filepath.Join(localMetadataDir(t, dir), "cr-plan.sample.yaml")
	first, err := os.ReadFile(samplePath)
	if err != nil {
		t.Fatalf("read sample template after first init: %v", err)
	}
	if len(first) == 0 {
		t.Fatalf("expected non-empty sample template")
	}

	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() second error = %v", err)
	}
	second, err := os.ReadFile(samplePath)
	if err != nil {
		t.Fatalf("read sample template after second init: %v", err)
	}
	if string(first) != string(second) {
		t.Fatalf("expected idempotent sample template content")
	}
}

func writePlanFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write plan file %s: %v", path, err)
	}
	return path
}

func findCreatedCRID(t *testing.T, result *ApplyCRPlanResult, key string) int {
	t.Helper()
	for _, created := range result.CreatedCRs {
		if created.Key == key {
			return created.ID
		}
	}
	t.Fatalf("created CR key %q not found in %#v", key, result.CreatedCRs)
	return 0
}
