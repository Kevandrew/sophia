package cli

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"sophia/internal/service"
)

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

func TestCRAddDefaultsToNoSwitchAndSupportsSwitchFlag(t *testing.T) {
	dir := t.TempDir()
	svc := service.New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	out, _, runErr := runCLI(t, dir, "cr", "add", "No switch default")
	if runErr != nil {
		t.Fatalf("cr add default error = %v\noutput=%s", runErr, out)
	}
	if !strings.Contains(out, "Run: sophia cr switch 1") {
		t.Fatalf("expected switch guidance in output, got %q", out)
	}
	current := runGit(t, dir, "branch", "--show-current")
	if current != "main" {
		t.Fatalf("expected to remain on main, got %q", current)
	}

	out, _, runErr = runCLI(t, dir, "cr", "add", "Switch now", "--switch")
	if runErr != nil {
		t.Fatalf("cr add --switch error = %v\noutput=%s", runErr, out)
	}
	if !strings.Contains(out, "Active branch: ") {
		t.Fatalf("expected active branch output, got %q", out)
	}
	activePrefix := "Active branch: "
	activeIdx := strings.Index(out, activePrefix)
	activeBranch := ""
	if activeIdx >= 0 {
		remaining := strings.TrimSpace(out[activeIdx+len(activePrefix):])
		fields := strings.Fields(remaining)
		if len(fields) > 0 {
			activeBranch = fields[0]
		}
	}
	if !strings.Contains(activeBranch, "cr-2-") {
		t.Fatalf("expected CR-2 branch alias, got %q", activeBranch)
	}
	current = runGit(t, dir, "branch", "--show-current")
	if current != activeBranch {
		t.Fatalf("expected switched branch %q, got %q", activeBranch, current)
	}
}

func TestCRAddSupportsOwnerPrefixAndExplicitBranchAlias(t *testing.T) {
	dir := t.TempDir()
	if _, _, initErr := runCLI(t, dir, "init", "--base-branch", "main", "--branch-owner-prefix", "kevandrew"); initErr != nil {
		t.Fatalf("init with owner prefix error = %v", initErr)
	}

	out, _, runErr := runCLI(t, dir, "cr", "add", "Prefix default", "--switch")
	if runErr != nil {
		t.Fatalf("cr add with owner-prefix default error = %v\noutput=%s", runErr, out)
	}
	if !strings.Contains(out, "Active branch: kevandrew/cr-1-") {
		t.Fatalf("expected owner-prefixed active branch output, got %q", out)
	}

	runGit(t, dir, "checkout", "main")
	out, _, runErr = runCLI(t, dir, "cr", "add", "Alias explicit", "--branch-alias", "kevandrew/cr-2-explicit", "--switch")
	if runErr != nil {
		t.Fatalf("cr add with explicit alias error = %v\noutput=%s", runErr, out)
	}
	if !strings.Contains(out, "Active branch: kevandrew/cr-2-explicit") {
		t.Fatalf("expected explicit alias output, got %q", out)
	}

	_, _, runErr = runCLI(t, dir, "cr", "add", "Conflict", "--branch-alias", "cr-3-conflict", "--owner-prefix", "foo")
	if runErr == nil || !strings.Contains(runErr.Error(), "--branch-alias and --owner-prefix cannot be combined") {
		t.Fatalf("expected branch-alias/owner-prefix conflict error, got %v", runErr)
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

func TestCRRefreshCommandAutoStrategy(t *testing.T) {
	dir := t.TempDir()
	svc := service.New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")

	cr, err := svc.AddCR("Refresh root", "auto refresh rebase")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "refresh-root.txt"), []byte("root\n"), 0o644); err != nil {
		t.Fatalf("write refresh-root.txt: %v", err)
	}
	runGit(t, dir, "add", "refresh-root.txt")
	runGit(t, dir, "commit", "-m", "feat: root change")
	runGit(t, dir, "checkout", "main")
	if err := os.WriteFile(filepath.Join(dir, "refresh-main.txt"), []byte("main\n"), 0o644); err != nil {
		t.Fatalf("write refresh-main.txt: %v", err)
	}
	runGit(t, dir, "add", "refresh-main.txt")
	runGit(t, dir, "commit", "-m", "feat: main change")

	dryRunOut, _, dryRunErr := runCLI(t, dir, "cr", "refresh", strconv.Itoa(cr.ID), "--dry-run", "--json")
	if dryRunErr != nil {
		t.Fatalf("cr refresh --dry-run --json error = %v\noutput=%s", dryRunErr, dryRunOut)
	}
	dryEnv := decodeEnvelope(t, dryRunOut)
	if !dryEnv.OK {
		t.Fatalf("expected dry-run envelope ok, got %#v", dryEnv)
	}
	if strategy, _ := dryEnv.Data["strategy"].(string); strategy != service.RefreshStrategyRebase {
		t.Fatalf("expected auto strategy rebase for root CR, got %#v", dryEnv.Data["strategy"])
	}

	applyOut, _, applyErr := runCLI(t, dir, "cr", "refresh", strconv.Itoa(cr.ID), "--json")
	if applyErr != nil {
		t.Fatalf("cr refresh --json error = %v\noutput=%s", applyErr, applyOut)
	}
	applyEnv := decodeEnvelope(t, applyOut)
	if !applyEnv.OK {
		t.Fatalf("expected refresh envelope ok, got %#v", applyEnv)
	}
	applied, ok := applyEnv.Data["applied"].(bool)
	if !ok || !applied {
		t.Fatalf("expected applied=true, got %#v", applyEnv.Data["applied"])
	}
}
