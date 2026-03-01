package cli

import (
	"os"
	"path/filepath"
	"testing"

	"sophia/internal/service"
)

const cliValidCRPlanYAML = `version: v1
crs:
  - key: parent
    title: "Parent"
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
      blast_radius: "Service"
      test_plan: "go test ./..."
      rollback_plan: "revert"
    tasks:
      - key: parent_task
        title: "Parent task"
        contract:
          intent: "Parent task intent"
          acceptance_criteria:
            - "Parent done"
          scope:
            - "internal/service"
        delegate_to:
          - "child"
  - key: child
    title: "Child"
    description: "Child intent"
    parent_key: "parent"
    contract:
      why: "Child why"
      scope:
        - "internal/cli"
      non_goals:
        - "No command semantic changes"
      invariants:
        - "Output stable"
      blast_radius: "CLI"
      test_plan: "go test ./internal/cli"
      rollback_plan: "revert"
    tasks:
      - key: child_task
        title: "Child task"
        contract:
          intent: "Child task intent"
          acceptance_criteria:
            - "Child done"
          scope:
            - "internal/cli"
`

func TestCRApplyRequiresFileFlag(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := service.New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	_, _, err := runCLI(t, dir, "cr", "apply")
	if err == nil {
		t.Fatalf("expected cr apply without --file to fail")
	}
}

func TestCRApplyDryRunJSONDoesNotMutate(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := service.New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	planPath := writeCLIPlanFile(t, dir, "plan.yaml", cliValidCRPlanYAML)

	out, _, runErr := runCLI(t, dir, "cr", "apply", "--file", "plan.yaml", "--dry-run", "--json")
	if runErr != nil {
		t.Fatalf("cr apply --dry-run --json error = %v\noutput=%s", runErr, out)
	}
	env := decodeEnvelope(t, out)
	if !env.OK {
		t.Fatalf("expected ok envelope, got %#v", env)
	}
	if dryRun, _ := env.Data["dry_run"].(bool); !dryRun {
		t.Fatalf("expected dry_run=true, got %#v", env.Data)
	}
	if consumed, _ := env.Data["consumed"].(bool); consumed {
		t.Fatalf("expected consumed=false on dry-run, got %#v", env.Data)
	}
	if _, err := os.Stat(planPath); err != nil {
		t.Fatalf("expected plan file to remain after dry-run: %v", err)
	}
	crs, err := svc.ListCRs()
	if err != nil {
		t.Fatalf("ListCRs() error = %v", err)
	}
	if len(crs) != 0 {
		t.Fatalf("expected no CRs after dry-run, got %d", len(crs))
	}
}

func TestCRApplyJSONConsumesByDefault(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := service.New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	planPath := writeCLIPlanFile(t, dir, "plan.yaml", cliValidCRPlanYAML)

	out, _, runErr := runCLI(t, dir, "cr", "apply", "--file", "plan.yaml", "--json")
	if runErr != nil {
		t.Fatalf("cr apply --json error = %v\noutput=%s", runErr, out)
	}
	env := decodeEnvelope(t, out)
	if !env.OK {
		t.Fatalf("expected ok envelope, got %#v", env)
	}
	if consumed, _ := env.Data["consumed"].(bool); !consumed {
		t.Fatalf("expected consumed=true by default, got %#v", env.Data)
	}
	if _, err := os.Stat(planPath); !os.IsNotExist(err) {
		t.Fatalf("expected plan file removed, stat err=%v", err)
	}

	stackOut, _, stackErr := runCLI(t, dir, "cr", "stack", "1", "--json")
	if stackErr != nil {
		t.Fatalf("cr stack --json error = %v\noutput=%s", stackErr, stackOut)
	}
	stackEnv := decodeEnvelope(t, stackOut)
	nodes, ok := stackEnv.Data["nodes"].([]any)
	if !ok || len(nodes) != 2 {
		t.Fatalf("expected two stack nodes after apply, got %#v", stackEnv.Data["nodes"])
	}
}

func TestCRApplyKeepFileFlagPreservesPlan(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := service.New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	planPath := writeCLIPlanFile(t, dir, "plan.yaml", cliValidCRPlanYAML)

	out, _, runErr := runCLI(t, dir, "cr", "apply", "--file", "plan.yaml", "--keep-file", "--json")
	if runErr != nil {
		t.Fatalf("cr apply --keep-file --json error = %v\noutput=%s", runErr, out)
	}
	env := decodeEnvelope(t, out)
	if !env.OK {
		t.Fatalf("expected ok envelope, got %#v", env)
	}
	if consumed, _ := env.Data["consumed"].(bool); consumed {
		t.Fatalf("expected consumed=false with --keep-file, got %#v", env.Data)
	}
	if _, err := os.Stat(planPath); err != nil {
		t.Fatalf("expected plan file to remain, got %v", err)
	}
}

func TestCRApplyReportsParseErrors(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := service.New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	writeCLIPlanFile(t, dir, "bad.yaml", "version: v1\nunknown_field: true\ncrs: []\n")

	_, _, runErr := runCLI(t, dir, "cr", "apply", "--file", "bad.yaml")
	if runErr == nil {
		t.Fatalf("expected parse/validation failure for bad.yaml")
	}
}

func writeCLIPlanFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write plan file %s: %v", path, err)
	}
	return path
}
