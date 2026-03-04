package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sophia/internal/service"
)

func TestCommandsResolveRepoFromSubdirectory(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := service.New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if _, err := svc.AddCR("Subdir CR", "repo root resolution"); err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}

	nested := filepath.Join(dir, "nested", "child")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatalf("mkdir nested: %v", err)
	}

	out, _, err := runCLI(t, nested, "cr", "list")
	if err != nil {
		t.Fatalf("cr list from nested dir error = %v\noutput=%s", err, out)
	}
	if !strings.Contains(out, "Subdir CR") {
		t.Fatalf("expected CR listing from nested dir, got %q", out)
	}
}

func TestCRWhereResolvesOwnerWorktreeAcrossLinkedWorktrees(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := service.New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	cr, err := svc.AddCR("Locate owner", "where command")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}

	runGit(t, dir, "checkout", "main")
	wtDir := filepath.Join(t.TempDir(), "wt-where-cli")
	runGit(t, dir, "worktree", "add", wtDir, cr.Branch)

	mainOut, _, mainErr := runCLI(t, dir, "cr", "where", "1", "--json")
	if mainErr != nil {
		t.Fatalf("cr where --json (main) error = %v\noutput=%s", mainErr, mainOut)
	}
	mainEnv := decodeEnvelope(t, mainOut)
	if !mainEnv.OK {
		t.Fatalf("expected ok envelope for main worktree where, got %#v", mainEnv)
	}
	mainOwner, _ := mainEnv.Data["owner_worktree_path"].(string)
	if !samePathForTest(mainOwner, wtDir) {
		t.Fatalf("expected owner worktree %q, got %q", wtDir, mainOwner)
	}
	if ownerIsCurrent, _ := mainEnv.Data["owner_is_current_worktree"].(bool); ownerIsCurrent {
		t.Fatalf("expected owner_is_current_worktree=false in main worktree")
	}
	if checkedElsewhere, _ := mainEnv.Data["checked_out_in_other_worktree"].(bool); !checkedElsewhere {
		t.Fatalf("expected checked_out_in_other_worktree=true in main worktree")
	}

	ownerOut, _, ownerErr := runCLI(t, wtDir, "cr", "where", "1", "--json")
	if ownerErr != nil {
		t.Fatalf("cr where --json (owner worktree) error = %v\noutput=%s", ownerErr, ownerOut)
	}
	ownerEnv := decodeEnvelope(t, ownerOut)
	if !ownerEnv.OK {
		t.Fatalf("expected ok envelope for owner worktree where, got %#v", ownerEnv)
	}
	ownerPath, _ := ownerEnv.Data["owner_worktree_path"].(string)
	if !samePathForTest(ownerPath, wtDir) {
		t.Fatalf("expected owner worktree %q, got %q", wtDir, ownerPath)
	}
	if ownerIsCurrent, _ := ownerEnv.Data["owner_is_current_worktree"].(bool); !ownerIsCurrent {
		t.Fatalf("expected owner_is_current_worktree=true in owner worktree")
	}
	if checkedElsewhere, _ := ownerEnv.Data["checked_out_in_other_worktree"].(bool); checkedElsewhere {
		t.Fatalf("expected checked_out_in_other_worktree=false in owner worktree")
	}
}

func TestCRStatusJSONIncludesWorktreeOwnershipSignals(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := service.New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	cr, err := svc.AddCR("Status ownership", "status branch context")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}

	statusCurrentOut, _, statusCurrentErr := runCLI(t, dir, "cr", "status", "1", "--json")
	if statusCurrentErr != nil {
		t.Fatalf("cr status --json (current owner) error = %v\noutput=%s", statusCurrentErr, statusCurrentOut)
	}
	statusCurrentEnv := decodeEnvelope(t, statusCurrentOut)
	branchContext, _ := statusCurrentEnv.Data["branch_context"].(map[string]any)
	if ownerIsCurrent, _ := branchContext["owner_is_current_worktree"].(bool); !ownerIsCurrent {
		t.Fatalf("expected owner_is_current_worktree=true on active CR branch, got %#v", branchContext["owner_is_current_worktree"])
	}
	if checkedElsewhere, _ := branchContext["checked_out_in_other_worktree"].(bool); checkedElsewhere {
		t.Fatalf("expected checked_out_in_other_worktree=false on active CR branch, got %#v", branchContext["checked_out_in_other_worktree"])
	}

	runGit(t, dir, "checkout", "main")
	wtDir := filepath.Join(t.TempDir(), "wt-status-cli")
	runGit(t, dir, "worktree", "add", wtDir, cr.Branch)
	statusOtherOut, _, statusOtherErr := runCLI(t, dir, "cr", "status", "1", "--json")
	if statusOtherErr != nil {
		t.Fatalf("cr status --json (other owner) error = %v\noutput=%s", statusOtherErr, statusOtherOut)
	}
	statusOtherEnv := decodeEnvelope(t, statusOtherOut)
	branchContext, _ = statusOtherEnv.Data["branch_context"].(map[string]any)
	if ownerPath, _ := branchContext["owner_worktree_path"].(string); !samePathForTest(ownerPath, wtDir) {
		t.Fatalf("expected owner_worktree_path=%q, got %#v", wtDir, branchContext["owner_worktree_path"])
	}
	if ownerIsCurrent, _ := branchContext["owner_is_current_worktree"].(bool); ownerIsCurrent {
		t.Fatalf("expected owner_is_current_worktree=false when branch owned elsewhere, got %#v", branchContext["owner_is_current_worktree"])
	}
	if checkedElsewhere, _ := branchContext["checked_out_in_other_worktree"].(bool); !checkedElsewhere {
		t.Fatalf("expected checked_out_in_other_worktree=true when branch owned elsewhere, got %#v", branchContext["checked_out_in_other_worktree"])
	}

	runGit(t, dir, "worktree", "remove", wtDir, "--force")
	statusMissingOut, _, statusMissingErr := runCLI(t, dir, "cr", "status", "1", "--json")
	if statusMissingErr != nil {
		t.Fatalf("cr status --json (not checked out) error = %v\noutput=%s", statusMissingErr, statusMissingOut)
	}
	statusMissingEnv := decodeEnvelope(t, statusMissingOut)
	branchContext, _ = statusMissingEnv.Data["branch_context"].(map[string]any)
	if ownerPath, _ := branchContext["owner_worktree_path"].(string); strings.TrimSpace(ownerPath) != "" {
		t.Fatalf("expected empty owner_worktree_path when branch is not checked out, got %#v", branchContext["owner_worktree_path"])
	}
	if ownerIsCurrent, _ := branchContext["owner_is_current_worktree"].(bool); ownerIsCurrent {
		t.Fatalf("expected owner_is_current_worktree=false when no owner exists, got %#v", branchContext["owner_is_current_worktree"])
	}
	if checkedElsewhere, _ := branchContext["checked_out_in_other_worktree"].(bool); checkedElsewhere {
		t.Fatalf("expected checked_out_in_other_worktree=false when no owner exists, got %#v", branchContext["checked_out_in_other_worktree"])
	}
}

func samePathForTest(a, b string) bool {
	a = strings.TrimSpace(a)
	b = strings.TrimSpace(b)
	if a == "" || b == "" {
		return a == b
	}
	aresolved, aerr := filepath.EvalSymlinks(a)
	bresolved, berr := filepath.EvalSymlinks(b)
	if aerr == nil {
		a = aresolved
	}
	if berr == nil {
		b = bresolved
	}
	return filepath.Clean(a) == filepath.Clean(b)
}
