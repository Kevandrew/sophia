package cli

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"sophia/internal/service"
)

func TestCRAddRejectsBaseAndParentTogether(t *testing.T) {
	t.Parallel()
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

func TestCRAddRejectsNegativeParent(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := service.New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	_, _, err := runCLI(t, dir, "cr", "add", "Bad parent", "--parent", "-1")
	if err == nil || !strings.Contains(err.Error(), "--parent must be >= 1") {
		t.Fatalf("expected --parent lower bound error, got %v", err)
	}
}

func TestRootHelpMentionsCRAddSwitchDefault(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	out, _, runErr := runCLI(t, dir, "--help")
	if runErr != nil {
		t.Fatalf("root --help error = %v\noutput=%s", runErr, out)
	}
	if !strings.Contains(out, "(default: stays on current branch; pass --switch to hop immediately)") {
		t.Fatalf("expected root help to describe cr add switch default, got %q", out)
	}
}

func TestCRAddDefaultsToNoSwitchAndSupportsSwitchFlag(t *testing.T) {
	t.Parallel()
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
	if strings.Contains(out, "Bootstrapped local Sophia metadata") {
		t.Fatalf("did not expect bootstrap notice in initialized repo output, got %q", out)
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
	branchPattern := regexp.MustCompile(`^cr-switch-now-(?:[a-z0-9]{4}|[a-z0-9]{6}|[a-z0-9]{8})$`)
	if !branchPattern.MatchString(activeBranch) {
		t.Fatalf("expected uid-suffixed branch alias, got %q", activeBranch)
	}
	current = runGit(t, dir, "branch", "--show-current")
	if current != activeBranch {
		t.Fatalf("expected switched branch %q, got %q", activeBranch, current)
	}
}

func TestCRAddJSONSwitchSemanticsRemainCompatible(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := service.New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	out, _, runErr := runCLI(t, dir, "cr", "add", "JSON default no switch", "--json")
	if runErr != nil {
		t.Fatalf("cr add --json error = %v\noutput=%s", runErr, out)
	}
	env := decodeEnvelope(t, out)
	if !env.OK {
		t.Fatalf("expected ok envelope from cr add --json, got %#v", env)
	}
	if switched, ok := env.Data["switched"].(bool); !ok || switched {
		t.Fatalf("expected switched=false for default add, got %#v", env.Data["switched"])
	}
	current := runGit(t, dir, "branch", "--show-current")
	if current != "main" {
		t.Fatalf("expected to remain on main for default json add, got %q", current)
	}

	out, _, runErr = runCLI(t, dir, "cr", "add", "JSON switch", "--switch", "--json")
	if runErr != nil {
		t.Fatalf("cr add --switch --json error = %v\noutput=%s", runErr, out)
	}
	env = decodeEnvelope(t, out)
	if !env.OK {
		t.Fatalf("expected ok envelope from cr add --switch --json, got %#v", env)
	}
	if switched, ok := env.Data["switched"].(bool); !ok || !switched {
		t.Fatalf("expected switched=true for --switch json add, got %#v", env.Data["switched"])
	}
	crData, ok := env.Data["cr"].(map[string]any)
	if !ok {
		t.Fatalf("expected cr payload object, got %#v", env.Data["cr"])
	}
	branch, _ := crData["branch"].(string)
	if strings.TrimSpace(branch) == "" {
		t.Fatalf("expected branch in cr payload, got %#v", crData)
	}
	current = runGit(t, dir, "branch", "--show-current")
	if current != branch {
		t.Fatalf("expected switched branch %q, got %q", branch, current)
	}
}

func TestCRChildAddDefaultsToNoSwitchAndSupportsSwitchFlag(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := service.New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	parentOut, _, parentErr := runCLI(t, dir, "cr", "add", "Parent context", "--switch")
	if parentErr != nil {
		t.Fatalf("parent cr add --switch error = %v\noutput=%s", parentErr, parentOut)
	}
	parentBranch := runGit(t, dir, "branch", "--show-current")
	if strings.TrimSpace(parentBranch) == "" || parentBranch == "main" {
		t.Fatalf("expected to be switched to parent CR branch, got %q", parentBranch)
	}

	out, _, runErr := runCLI(t, dir, "cr", "child", "add", "Child no switch")
	if runErr != nil {
		t.Fatalf("child add default error = %v\noutput=%s", runErr, out)
	}
	if !strings.Contains(out, "Run: sophia cr switch 2") {
		t.Fatalf("expected switch guidance for child add output, got %q", out)
	}
	current := runGit(t, dir, "branch", "--show-current")
	if current != parentBranch {
		t.Fatalf("expected to remain on parent branch %q, got %q", parentBranch, current)
	}

	out, _, runErr = runCLI(t, dir, "cr", "child", "add", "Child switch", "--switch")
	if runErr != nil {
		t.Fatalf("child add --switch error = %v\noutput=%s", runErr, out)
	}
	if !strings.Contains(out, "Active branch: ") {
		t.Fatalf("expected active branch output for child add --switch, got %q", out)
	}
	current = runGit(t, dir, "branch", "--show-current")
	if current == parentBranch || current == "main" {
		t.Fatalf("expected switch to child branch, got %q", current)
	}
}

func TestCRAddNoInitShowsBootstrapNoticeOnce(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	runGit(t, dir, "init", "-b", "main")

	firstOut, _, firstErr := runCLI(t, dir, "cr", "add", "No init first")
	if firstErr != nil {
		t.Fatalf("first cr add error = %v\noutput=%s", firstErr, firstOut)
	}
	if !strings.Contains(firstOut, "Bootstrapped local Sophia metadata") {
		t.Fatalf("expected bootstrap notice in first output, got %q", firstOut)
	}

	secondOut, _, secondErr := runCLI(t, dir, "cr", "add", "No init second")
	if secondErr != nil {
		t.Fatalf("second cr add error = %v\noutput=%s", secondErr, secondOut)
	}
	if strings.Contains(secondOut, "Bootstrapped local Sophia metadata") {
		t.Fatalf("did not expect bootstrap notice in second output, got %q", secondOut)
	}
}

func TestCRAddSupportsOwnerPrefixAndExplicitBranchAlias(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if _, _, initErr := runCLI(t, dir, "init", "--base-branch", "main", "--branch-owner-prefix", "kevandrew"); initErr != nil {
		t.Fatalf("init with owner prefix error = %v", initErr)
	}

	out, _, runErr := runCLI(t, dir, "cr", "add", "Prefix default", "--switch")
	if runErr != nil {
		t.Fatalf("cr add with owner-prefix default error = %v\noutput=%s", runErr, out)
	}
	if !strings.Contains(out, "Active branch: kevandrew/cr-prefix-default-") {
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

	runGit(t, dir, "checkout", "main")
	out, _, runErr = runCLI(t, dir, "cr", "add", "Alias explicit v2", "--branch-alias", "cr-alias-explicit-v2-a1b2", "--switch")
	if runErr != nil {
		t.Fatalf("cr add with explicit v2 alias error = %v\noutput=%s", runErr, out)
	}
	if !strings.Contains(out, "Active branch: cr-alias-explicit-v2-a1b2") {
		t.Fatalf("expected explicit v2 alias output, got %q", out)
	}

	_, _, runErr = runCLI(t, dir, "cr", "add", "Conflict", "--branch-alias", "cr-3-conflict", "--owner-prefix", "foo")
	if runErr == nil || !strings.Contains(runErr.Error(), "--branch-alias and --owner-prefix cannot be combined") {
		t.Fatalf("expected branch-alias/owner-prefix conflict error, got %v", runErr)
	}

	_, _, runErr = runCLI(t, dir, "cr", "add", "Bad v2 suffix", "--branch-alias", "cr-bad-suffix-a1b2c")
	if runErr == nil {
		t.Fatalf("expected invalid v2 suffix length error")
	}
}

func TestCRBranchFormatSupportsExistingIDAndExplicitUID(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := service.New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	if _, _, runErr := runCLI(t, dir, "cr", "add", "Format target"); runErr != nil {
		t.Fatalf("cr add error = %v", runErr)
	}

	statusOut, _, statusErr := runCLI(t, dir, "cr", "status", "1", "--json")
	if statusErr != nil {
		t.Fatalf("cr status --json error = %v\noutput=%s", statusErr, statusOut)
	}
	statusEnv := decodeEnvelope(t, statusOut)
	existingBranch, _ := statusEnv.Data["branch"].(string)
	existingUID, _ := statusEnv.Data["uid"].(string)
	if strings.TrimSpace(existingBranch) == "" || strings.TrimSpace(existingUID) == "" {
		t.Fatalf("expected existing branch and uid, got %#v", statusEnv.Data)
	}

	formatOut, _, formatErr := runCLI(t, dir, "cr", "branch", "format", "--id", "1", "--json")
	if formatErr != nil {
		t.Fatalf("cr branch format --id error = %v\noutput=%s", formatErr, formatOut)
	}
	formatEnv := decodeEnvelope(t, formatOut)
	formattedBranch, _ := formatEnv.Data["branch"].(string)
	formattedUID, _ := formatEnv.Data["uid"].(string)
	if formattedBranch != existingBranch || formattedUID != existingUID {
		t.Fatalf("expected formatted branch/uid to match existing CR, got branch=%q uid=%q", formattedBranch, formattedUID)
	}

	formatOut, _, formatErr = runCLI(t, dir, "cr", "branch", "format", "--uid", "cr_c6bec981-b3dc-493d-aa41-897df808126c", "--title", "Format target", "--json")
	if formatErr != nil {
		t.Fatalf("cr branch format --uid error = %v\noutput=%s", formatErr, formatOut)
	}
	formatEnv = decodeEnvelope(t, formatOut)
	formattedBranch, _ = formatEnv.Data["branch"].(string)
	if ok, _ := regexp.MatchString(`^cr-format-target-c6be(?:[a-z0-9]{2}|[a-z0-9]{4})?$`, formattedBranch); !ok {
		t.Fatalf("expected uid-derived c6be suffix (with collision fallback), got %q", formattedBranch)
	}

	formatOut, _, formatErr = runCLI(t, dir, "cr", "branch", "format", "--id", "99", "--title", "Future preview", "--json")
	if formatErr != nil {
		t.Fatalf("cr branch format preview error = %v\noutput=%s", formatErr, formatOut)
	}
	formatEnv = decodeEnvelope(t, formatOut)
	formattedBranch, _ = formatEnv.Data["branch"].(string)
	if ok, _ := regexp.MatchString(`^cr-future-preview-[a-z0-9]{4}$`, formattedBranch); !ok {
		t.Fatalf("expected id-preview branch alias, got %q", formattedBranch)
	}
}

func TestCRBranchFormatPreservesOwnerPrefixForExistingCR(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if _, _, initErr := runCLI(t, dir, "init", "--base-branch", "main", "--branch-owner-prefix", "team"); initErr != nil {
		t.Fatalf("init with owner prefix error = %v", initErr)
	}

	if _, _, addErr := runCLI(t, dir, "cr", "add", "Owner format target"); addErr != nil {
		t.Fatalf("cr add error = %v", addErr)
	}
	statusOut, _, statusErr := runCLI(t, dir, "cr", "status", "1", "--json")
	if statusErr != nil {
		t.Fatalf("cr status --json error = %v\noutput=%s", statusErr, statusOut)
	}
	statusEnv := decodeEnvelope(t, statusOut)
	existingBranch, _ := statusEnv.Data["branch"].(string)
	if !strings.HasPrefix(existingBranch, "team/") {
		t.Fatalf("expected owner-prefixed stored branch, got %q", existingBranch)
	}

	formatOut, _, formatErr := runCLI(t, dir, "cr", "branch", "format", "--id", "1", "--json")
	if formatErr != nil {
		t.Fatalf("cr branch format --id error = %v\noutput=%s", formatErr, formatOut)
	}
	formatEnv := decodeEnvelope(t, formatOut)
	formattedBranch, _ := formatEnv.Data["branch"].(string)
	if formattedBranch != existingBranch {
		t.Fatalf("expected formatted branch %q to match stored owner-prefixed branch %q", formattedBranch, existingBranch)
	}
}

func TestCRSwitchJSONWorktreeConflictIncludesStructuredDetails(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := service.New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")

	cr, err := svc.AddCR("Switch conflict details", "json branch conflict")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	runGit(t, dir, "checkout", "main")
	wtDir := filepath.Join(t.TempDir(), "wt-switch-json")
	runGit(t, dir, "worktree", "add", wtDir, cr.Branch)

	out, _, runErr := runCLI(t, dir, "cr", "switch", "1", "--json")
	if runErr == nil {
		t.Fatalf("expected switch conflict error, output=%s", out)
	}
	env := decodeEnvelope(t, out)
	if env.OK || env.Error == nil {
		t.Fatalf("expected structured error envelope, got %#v", env)
	}
	if env.Error.Code != "branch_in_other_worktree" {
		t.Fatalf("expected branch_in_other_worktree code, got %#v", env.Error.Code)
	}
	if env.Error.Details == nil {
		t.Fatalf("expected error details for branch conflict, got %#v", env.Error)
	}
	if gotID, _ := env.Error.Details["cr_id"].(float64); int(gotID) != 1 {
		t.Fatalf("expected cr_id=1 in details, got %#v", env.Error.Details["cr_id"])
	}
	if branch, _ := env.Error.Details["branch"].(string); branch != cr.Branch {
		t.Fatalf("expected branch %q in details, got %#v", cr.Branch, env.Error.Details["branch"])
	}
	if ownerPath, _ := env.Error.Details["owner_worktree_path"].(string); !samePathForTest(ownerPath, wtDir) {
		t.Fatalf("expected owner_worktree_path %q, got %#v", wtDir, env.Error.Details["owner_worktree_path"])
	}
	if operation, _ := env.Error.Details["operation"].(string); operation != "cr_switch" {
		t.Fatalf("expected operation=cr_switch, got %#v", env.Error.Details["operation"])
	}
	suggested, _ := env.Error.Details["suggested_command"].(string)
	if !strings.Contains(suggested, "sophia cr switch 1") {
		t.Fatalf("expected suggested switch command in details, got %#v", env.Error.Details["suggested_command"])
	}
}

func TestCRBaseSetAndRestackCommands(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := service.New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

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
	t.Parallel()
	dir := t.TempDir()
	svc := service.New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

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
