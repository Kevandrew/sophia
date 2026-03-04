package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"sophia/internal/service"
)

func TestPRGateMergeJSONReturnsActionRequiredWhenPROpenApprovalMissing(t *testing.T) {
	dir := setupCLIPRGateRepo(t)
	installFakeGH(t, "echo '[]'; exit 0")

	out, _, runErr := runCLI(t, dir, "cr", "merge", "1", "--json")
	if runErr != nil {
		t.Fatalf("cr merge --json error = %v\noutput=%s", runErr, out)
	}
	env := decodeEnvelope(t, out)
	if !env.OK {
		t.Fatalf("expected ok envelope, got %#v", env)
	}
	actionRequired, ok := env.Data["action_required"].(map[string]any)
	if !ok {
		t.Fatalf("expected action_required object, got %#v", env.Data["action_required"])
	}
	if got, _ := actionRequired["type"].(string); got != "agent_approval" {
		t.Fatalf("expected action_required.type=agent_approval, got %#v", actionRequired["type"])
	}
	if got, _ := actionRequired["name"].(string); got != "open_pr" {
		t.Fatalf("expected action_required.name=open_pr, got %#v", actionRequired["name"])
	}
	if got, _ := actionRequired["approve_flag"].(string); got != "--approve-pr-open" {
		t.Fatalf("expected action_required.approve_flag=--approve-pr-open, got %#v", actionRequired["approve_flag"])
	}
}

func TestPRGateOpenJSONReturnsActionRequiredWhenApprovalMissing(t *testing.T) {
	dir := setupCLIPRGateRepo(t)
	installFakeGH(t, "echo '[]'; exit 0")

	out, _, runErr := runCLI(t, dir, "cr", "pr", "open", "1", "--json")
	if runErr != nil {
		t.Fatalf("cr pr open --json error = %v\noutput=%s", runErr, out)
	}
	env := decodeEnvelope(t, out)
	if !env.OK {
		t.Fatalf("expected ok envelope, got %#v", env)
	}
	actionRequired, ok := env.Data["action_required"].(map[string]any)
	if !ok {
		t.Fatalf("expected action_required object, got %#v", env.Data["action_required"])
	}
	if got, _ := actionRequired["type"].(string); got != "agent_approval" {
		t.Fatalf("expected action_required.type=agent_approval, got %#v", actionRequired["type"])
	}
	if got, _ := actionRequired["name"].(string); got != "open_pr" {
		t.Fatalf("expected action_required.name=open_pr, got %#v", actionRequired["name"])
	}
	if got, _ := actionRequired["approve_flag"].(string); got != "--approve-open" {
		t.Fatalf("expected action_required.approve_flag=--approve-open, got %#v", actionRequired["approve_flag"])
	}
}

func TestPRReadyJSONReturnsActionRequiredWhenNoTaskCheckpointsExist(t *testing.T) {
	dir := setupCLIPRGateRepo(t)
	installFakeGH(t, strings.Join([]string{
		"if [ \"$1\" = \"pr\" ] && [ \"$2\" = \"list\" ]; then",
		"  echo '[{\"number\":27,\"url\":\"https://github.com/acme/repo/pull/27\",\"state\":\"OPEN\",\"isDraft\":true,\"headRefOid\":\"abc123\",\"headRefName\":\"cr-1-pr-gate-json\",\"baseRefName\":\"main\",\"updatedAt\":\"2026-03-03T00:00:00Z\"}]'",
		"  exit 0",
		"fi",
		"if [ \"$1\" = \"pr\" ] && [ \"$2\" = \"view\" ]; then",
		"  echo '{\"number\":27,\"url\":\"https://github.com/acme/repo/pull/27\",\"state\":\"OPEN\",\"isDraft\":true,\"headRefOid\":\"abc123\",\"headRefName\":\"cr-1-pr-gate-json\",\"baseRefName\":\"main\",\"author\":{\"login\":\"bot\"},\"latestReviews\":[],\"statusCheckRollup\":[]}'",
		"  exit 0",
		"fi",
		"if [ \"$1\" = \"pr\" ] && [ \"$2\" = \"ready\" ]; then",
		"  echo 'unexpected ready transition' >&2",
		"  exit 1",
		"fi",
		"echo \"unexpected gh args: $@\" >&2",
		"exit 1",
	}, "\n"))

	out, _, runErr := runCLI(t, dir, "cr", "pr", "reconcile", "1", "--mode", "relink", "--json")
	if runErr != nil {
		t.Fatalf("cr pr reconcile --mode relink --json error = %v\noutput=%s", runErr, out)
	}

	out, _, runErr = runCLI(t, dir, "cr", "pr", "ready", "1", "--json")
	if runErr != nil {
		t.Fatalf("cr pr ready --json error = %v\noutput=%s", runErr, out)
	}
	env := decodeEnvelope(t, out)
	if !env.OK {
		t.Fatalf("expected ok envelope, got %#v", env)
	}
	actionRequired, ok := env.Data["action_required"].(map[string]any)
	if !ok {
		t.Fatalf("expected action_required object, got %#v", env.Data["action_required"])
	}
	if got, _ := actionRequired["name"].(string); got != "ready_pr_blocked" {
		t.Fatalf("expected action_required.name=ready_pr_blocked, got %#v", actionRequired["name"])
	}
	if got, _ := actionRequired["reason_code"].(string); got != "pre_implementation_no_checkpoints" {
		t.Fatalf("expected action_required.reason_code=pre_implementation_no_checkpoints, got %#v", actionRequired["reason_code"])
	}
	suggested, ok := actionRequired["suggested_commands"].([]any)
	if !ok || len(suggested) == 0 {
		t.Fatalf("expected suggested_commands array, got %#v", actionRequired["suggested_commands"])
	}
	foundTaskList := false
	for _, command := range suggested {
		if strings.TrimSpace(fmt.Sprint(command)) == "sophia cr task list 1" {
			foundTaskList = true
			break
		}
	}
	if !foundTaskList {
		t.Fatalf("expected suggested_commands to include `sophia cr task list 1`, got %#v", suggested)
	}
}

func TestPRGateMergeJSONReturnsGHAuthRequiredErrorDetails(t *testing.T) {
	dir := setupCLIPRGateRepo(t)
	installFakeGH(t, "echo 'error: not logged into github.com. Run gh auth login.' >&2; exit 4")

	out, _, runErr := runCLI(t, dir, "cr", "merge", "1", "--approve-pr-open", "--json")
	if runErr == nil {
		t.Fatalf("expected cr merge --json to fail for gh auth error")
	}
	env := decodeEnvelope(t, out)
	if env.OK || env.Error == nil {
		t.Fatalf("expected non-ok envelope with error, got %#v", env)
	}
	if env.Error.Code != "gh_auth_required" {
		t.Fatalf("expected gh_auth_required code, got %#v", env.Error.Code)
	}
	actionRequired := requireActionRequiredDetails(t, env.Error.Details)
	if got, _ := actionRequired["name"].(string); got != "gh_auth_login" {
		t.Fatalf("expected action_required.name=gh_auth_login, got %#v", actionRequired["name"])
	}
	if got, _ := actionRequired["suggested_command"].(string); got != "gh auth login" {
		t.Fatalf("expected suggested command gh auth login, got %#v", actionRequired["suggested_command"])
	}
}

func TestPRGateMergeJSONReturnsPRPermissionDeniedErrorDetails(t *testing.T) {
	dir := setupCLIPRGateRepo(t)
	installFakeGH(t, "echo 'GraphQL: Resource not accessible by integration' >&2; exit 1")

	out, _, runErr := runCLI(t, dir, "cr", "merge", "1", "--approve-pr-open", "--json")
	if runErr == nil {
		t.Fatalf("expected cr merge --json to fail for gh permission error")
	}
	env := decodeEnvelope(t, out)
	if env.OK || env.Error == nil {
		t.Fatalf("expected non-ok envelope with error, got %#v", env)
	}
	if env.Error.Code != "pr_permission_denied" {
		t.Fatalf("expected pr_permission_denied code, got %#v", env.Error.Code)
	}
	actionRequired := requireActionRequiredDetails(t, env.Error.Details)
	if got, _ := actionRequired["name"].(string); got != "request_reviewer_merge" {
		t.Fatalf("expected action_required.name=request_reviewer_merge, got %#v", actionRequired["name"])
	}
}

func TestPRGateMergeJSONReturnsPushPermissionDeniedErrorDetails(t *testing.T) {
	dir := setupCLIPRGateRepo(t)
	installFakeGH(t, "echo '[]'; exit 0")
	installFakeGitPushDenied(t)

	out, _, runErr := runCLI(t, dir, "cr", "merge", "1", "--approve-pr-open", "--json")
	if runErr == nil {
		t.Fatalf("expected cr merge --json to fail for push denial")
	}
	env := decodeEnvelope(t, out)
	if env.OK || env.Error == nil {
		t.Fatalf("expected non-ok envelope with error, got %#v", env)
	}
	if env.Error.Code != "push_permission_denied" {
		t.Fatalf("expected push_permission_denied code, got %#v", env.Error.Code)
	}
	actionRequired := requireActionRequiredDetails(t, env.Error.Details)
	if got, _ := actionRequired["name"].(string); got != "request_push_access" {
		t.Fatalf("expected action_required.name=request_push_access, got %#v", actionRequired["name"])
	}
}

func TestPRStatusJSONIncludesLinkageActionMetadataWhenNoLinkedPR(t *testing.T) {
	dir := setupCLIPRGateRepo(t)
	installFakeGH(t, "echo '[]'; exit 0")

	out, _, runErr := runCLI(t, dir, "cr", "pr", "status", "1", "--json")
	if runErr != nil {
		t.Fatalf("cr pr status --json error = %v\noutput=%s", runErr, out)
	}
	env := decodeEnvelope(t, out)
	if !env.OK {
		t.Fatalf("expected ok envelope, got %#v", env)
	}
	if got, _ := env.Data["linkage_state"].(string); got != "no_linked_pr" {
		t.Fatalf("expected linkage_state=no_linked_pr, got %#v", env.Data["linkage_state"])
	}
	if got, _ := env.Data["action_required"].(string); got != "open_pr" {
		t.Fatalf("expected action_required=open_pr, got %#v", env.Data["action_required"])
	}
	suggested, ok := env.Data["suggested_commands"].([]any)
	if !ok || len(suggested) == 0 {
		t.Fatalf("expected suggested_commands list, got %#v", env.Data["suggested_commands"])
	}
}

func TestCRStatusJSONIncludesPRLinkageMetadataWhenNoLinkedPR(t *testing.T) {
	dir := setupCLIPRGateRepo(t)
	installFakeGH(t, "echo '[]'; exit 0")

	out, _, runErr := runCLI(t, dir, "cr", "status", "1", "--json")
	if runErr != nil {
		t.Fatalf("cr status --json error = %v\noutput=%s", runErr, out)
	}
	env := decodeEnvelope(t, out)
	if !env.OK {
		t.Fatalf("expected ok envelope, got %#v", env)
	}
	if got, _ := env.Data["pr_linkage_state"].(string); got != "no_linked_pr" {
		t.Fatalf("expected pr_linkage_state=no_linked_pr, got %#v", env.Data["pr_linkage_state"])
	}
	if got, _ := env.Data["action_required"].(string); got != "open_pr" {
		t.Fatalf("expected action_required=open_pr, got %#v", env.Data["action_required"])
	}
}

func TestCRReviewJSONIncludesLifecycleActionMetadataWhenNoLinkedPR(t *testing.T) {
	dir := setupCLIPRGateRepo(t)
	installFakeGH(t, "echo '[]'; exit 0")

	out, _, runErr := runCLI(t, dir, "cr", "review", "1", "--json")
	if runErr != nil {
		t.Fatalf("cr review --json error = %v\noutput=%s", runErr, out)
	}
	env := decodeEnvelope(t, out)
	if !env.OK {
		t.Fatalf("expected ok envelope, got %#v", env)
	}
	crMap, ok := env.Data["cr"].(map[string]any)
	if !ok {
		t.Fatalf("expected cr object, got %#v", env.Data["cr"])
	}
	if got, _ := crMap["pr_linkage_state"].(string); got != "no_linked_pr" {
		t.Fatalf("expected pr_linkage_state=no_linked_pr, got %#v", crMap["pr_linkage_state"])
	}
	if got, _ := crMap["action_required"].(string); got != "open_pr" {
		t.Fatalf("expected action_required=open_pr, got %#v", crMap["action_required"])
	}
}

func TestPRReconcileJSONRequiresExplicitMode(t *testing.T) {
	dir := setupCLIPRGateRepo(t)

	out, _, runErr := runCLI(t, dir, "cr", "pr", "reconcile", "1", "--json")
	if runErr == nil {
		t.Fatalf("expected cr pr reconcile --json without mode to fail, output=%s", out)
	}
	env := decodeEnvelope(t, out)
	if env.OK || env.Error == nil {
		t.Fatalf("expected non-ok envelope with error, got %#v", env)
	}
	if env.Error.Code != "invalid_argument" {
		t.Fatalf("expected invalid_argument code, got %#v", env.Error.Code)
	}
}

func TestPRReconcileRelinkJSONReturnsOutcome(t *testing.T) {
	dir := setupCLIPRGateRepo(t)
	installFakeGH(t, strings.Join([]string{
		"if [ \"$1\" = \"-R\" ]; then",
		"  shift",
		"  shift",
		"fi",
		"if [ \"$1\" = \"pr\" ] && [ \"$2\" = \"list\" ]; then",
		"  echo '[{\"number\":7,\"url\":\"https://github.com/acme/repo/pull/7\",\"state\":\"OPEN\",\"isDraft\":true,\"headRefOid\":\"abc123\",\"headRefName\":\"x\",\"baseRefName\":\"main\",\"updatedAt\":\"2026-03-03T00:00:00Z\"}]'",
		"  exit 0",
		"fi",
		"if [ \"$1\" = \"pr\" ] && [ \"$2\" = \"view\" ]; then",
		"  echo '{\"number\":7,\"url\":\"https://github.com/acme/repo/pull/7\",\"state\":\"OPEN\",\"isDraft\":true,\"headRefOid\":\"abc123\",\"headRefName\":\"x\",\"baseRefName\":\"main\",\"author\":{\"login\":\"bot\"},\"latestReviews\":[],\"statusCheckRollup\":[]}'",
		"  exit 0",
		"fi",
		"echo '[]'; exit 0",
	}, "\n"))

	out, _, runErr := runCLI(t, dir, "cr", "pr", "reconcile", "1", "--mode", "relink", "--json")
	if runErr != nil {
		t.Fatalf("cr pr reconcile --mode relink --json error = %v\noutput=%s", runErr, out)
	}
	env := decodeEnvelope(t, out)
	if !env.OK {
		t.Fatalf("expected ok envelope, got %#v", env)
	}
	if got, _ := env.Data["mode"].(string); got != "relink" {
		t.Fatalf("expected mode=relink, got %#v", env.Data["mode"])
	}
	if got, _ := env.Data["action"].(string); got != "relinked" {
		t.Fatalf("expected action=relinked, got %#v", env.Data["action"])
	}
	if got := int(env.Data["after_pr_number"].(float64)); got != 7 {
		t.Fatalf("expected after_pr_number=7, got %d", got)
	}
}

func TestPRReconcileCreateJSONReturnsOutcome(t *testing.T) {
	dir := setupCLIPRGateRepo(t)
	installFakeGH(t, strings.Join([]string{
		"if [ \"$1\" = \"-R\" ]; then",
		"  shift",
		"  shift",
		"fi",
		"if [ \"$1\" = \"pr\" ] && [ \"$2\" = \"create\" ]; then",
		"  echo 'https://github.com/acme/repo/pull/9'",
		"  exit 0",
		"fi",
		"if [ \"$1\" = \"pr\" ] && [ \"$2\" = \"view\" ]; then",
		"  echo '{\"number\":9,\"url\":\"https://github.com/acme/repo/pull/9\",\"state\":\"OPEN\",\"isDraft\":true,\"headRefOid\":\"abc123\",\"headRefName\":\"x\",\"baseRefName\":\"main\",\"author\":{\"login\":\"bot\"},\"latestReviews\":[],\"statusCheckRollup\":[]}'",
		"  exit 0",
		"fi",
		"echo '[]'; exit 0",
	}, "\n"))

	out, _, runErr := runCLI(t, dir, "cr", "pr", "reconcile", "1", "--mode", "create", "--json")
	if runErr != nil {
		t.Fatalf("cr pr reconcile --mode create --json error = %v\noutput=%s", runErr, out)
	}
	env := decodeEnvelope(t, out)
	if !env.OK {
		t.Fatalf("expected ok envelope, got %#v", env)
	}
	if got, _ := env.Data["action"].(string); got != "created" {
		t.Fatalf("expected action=created, got %#v", env.Data["action"])
	}
	if got := int(env.Data["after_pr_number"].(float64)); got != 9 {
		t.Fatalf("expected after_pr_number=9, got %d", got)
	}
}

func TestPRLifecycleCommandsJSONReturnUpdatedStates(t *testing.T) {
	dir := setupCLIPRGateRepo(t)
	installFakeGHLifecycleScript(t)

	out, _, runErr := runCLI(t, dir, "cr", "pr", "reconcile", "1", "--mode", "relink", "--json")
	if runErr != nil {
		t.Fatalf("cr pr reconcile --mode relink --json error = %v\noutput=%s", runErr, out)
	}
	env := decodeEnvelope(t, out)
	if !env.OK {
		t.Fatalf("expected ok envelope for relink, got %#v", env)
	}

	out, _, runErr = runCLI(t, dir, "cr", "pr", "unready", "1", "--json")
	if runErr != nil {
		t.Fatalf("cr pr unready --json error = %v\noutput=%s", runErr, out)
	}
	env = decodeEnvelope(t, out)
	if !env.OK {
		t.Fatalf("expected ok envelope for unready, got %#v", env)
	}
	if draft, _ := env.Data["draft"].(bool); !draft {
		t.Fatalf("expected draft=true after unready, got %#v", env.Data["draft"])
	}

	out, _, runErr = runCLI(t, dir, "cr", "pr", "close", "1", "--json")
	if runErr != nil {
		t.Fatalf("cr pr close --json error = %v\noutput=%s", runErr, out)
	}
	env = decodeEnvelope(t, out)
	if !env.OK {
		t.Fatalf("expected ok envelope for close, got %#v", env)
	}
	if got, _ := env.Data["state"].(string); strings.ToUpper(strings.TrimSpace(got)) != "CLOSED" {
		t.Fatalf("expected state=CLOSED after close, got %#v", env.Data["state"])
	}
	if got, _ := env.Data["action_required"].(string); got != "reopen_pr" {
		t.Fatalf("expected action_required=reopen_pr for closed PR, got %#v", env.Data["action_required"])
	}

	out, _, runErr = runCLI(t, dir, "cr", "pr", "reopen", "1", "--json")
	if runErr != nil {
		t.Fatalf("cr pr reopen --json error = %v\noutput=%s", runErr, out)
	}
	env = decodeEnvelope(t, out)
	if !env.OK {
		t.Fatalf("expected ok envelope for reopen, got %#v", env)
	}
	if got, _ := env.Data["state"].(string); strings.ToUpper(strings.TrimSpace(got)) != "OPEN" {
		t.Fatalf("expected state=OPEN after reopen, got %#v", env.Data["state"])
	}
}

func requireActionRequiredDetails(t *testing.T, details map[string]any) map[string]any {
	t.Helper()
	if details == nil {
		t.Fatalf("expected details payload")
	}
	actionRequired, ok := details["action_required"].(map[string]any)
	if !ok {
		t.Fatalf("expected action_required in details, got %#v", details)
	}
	if got, _ := actionRequired["type"].(string); got != "manual" {
		t.Fatalf("expected action_required.type=manual, got %#v", actionRequired["type"])
	}
	return actionRequired
}

func setupCLIPRGateRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	svc := service.New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SOPHIA.yaml"), []byte("version: v1\nmerge:\n  mode: pr_gate\narchive:\n  enabled: false\n"), 0o644); err != nil {
		t.Fatalf("write SOPHIA.yaml: %v", err)
	}
	cr, err := svc.AddCR("PR gate JSON", "json envelope coverage")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	setValidContractCLI(t, svc, cr.ID)
	originPath := filepath.Join(t.TempDir(), "origin.git")
	runGit(t, dir, "init", "--bare", originPath)
	runGit(t, dir, "remote", "add", "origin", originPath)
	return dir
}

func installFakeGH(t *testing.T, body string) {
	t.Helper()
	binDir := t.TempDir()
	ghPath := filepath.Join(binDir, "gh")
	script := fmt.Sprintf("#!/bin/sh\nif [ \"$1\" = \"pr\" ] && [ \"$2\" = \"list\" ]; then\n%s\nfi\n%s\n", body, body)
	if err := os.WriteFile(ghPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake gh: %v", err)
	}
	prependPath(t, binDir)
}

func installFakeGitPushDenied(t *testing.T) {
	t.Helper()
	realGit, err := exec.LookPath("git")
	if err != nil {
		t.Fatalf("resolve git path: %v", err)
	}
	binDir := t.TempDir()
	gitPath := filepath.Join(binDir, "git")
	script := strings.Join([]string{
		"#!/bin/sh",
		"if [ \"$1\" = \"push\" ]; then",
		"  echo \"remote: Permission to test/repo.git denied to user.\" >&2",
		"  echo \"fatal: unable to access 'https://github.com/test/repo.git/': The requested URL returned error: 403\" >&2",
		"  exit 1",
		"fi",
		fmt.Sprintf("exec %s \"$@\"", realGit),
	}, "\n") + "\n"
	if err := os.WriteFile(gitPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake git: %v", err)
	}
	prependPath(t, binDir)
}

func installFakeGHLifecycleScript(t *testing.T) {
	t.Helper()
	binDir := t.TempDir()
	ghPath := filepath.Join(binDir, "gh")
	script := strings.Join([]string{
		"#!/bin/sh",
		"state_file=\"${TMPDIR:-/tmp}/sophia-pr-state-12\"",
		"if [ \"$1\" = \"-R\" ]; then",
		"  shift",
		"  shift",
		"fi",
		"if [ \"$1\" = \"pr\" ] && [ \"$2\" = \"list\" ]; then",
		"  echo '[{\"number\":12,\"url\":\"https://github.com/acme/repo/pull/12\",\"state\":\"OPEN\",\"isDraft\":false,\"headRefOid\":\"abc123\",\"headRefName\":\"cr-1-pr-lifecycle\",\"baseRefName\":\"main\",\"updatedAt\":\"2026-03-03T00:00:00Z\"}]'",
		"  exit 0",
		"fi",
		"if [ \"$1\" = \"pr\" ] && [ \"$2\" = \"ready\" ] && [ \"$4\" = \"--undo\" ]; then",
		"  printf 'draft' > \"$state_file\"",
		"  exit 0",
		"fi",
		"if [ \"$1\" = \"pr\" ] && [ \"$2\" = \"close\" ]; then",
		"  printf 'closed' > \"$state_file\"",
		"  exit 0",
		"fi",
		"if [ \"$1\" = \"pr\" ] && [ \"$2\" = \"reopen\" ]; then",
		"  printf 'open' > \"$state_file\"",
		"  exit 0",
		"fi",
		"if [ \"$1\" = \"pr\" ] && [ \"$2\" = \"view\" ]; then",
		"  current='open'",
		"  if [ -f \"$state_file\" ]; then",
		"    current=$(cat \"$state_file\")",
		"  fi",
		"  if [ \"$current\" = \"draft\" ]; then",
		"    echo '{\"number\":12,\"url\":\"https://github.com/acme/repo/pull/12\",\"state\":\"OPEN\",\"isDraft\":true,\"headRefOid\":\"abc123\",\"headRefName\":\"cr-1-pr-lifecycle\",\"baseRefName\":\"main\",\"author\":{\"login\":\"bot\"},\"latestReviews\":[],\"statusCheckRollup\":[]}'",
		"    exit 0",
		"  fi",
		"  if [ \"$current\" = \"closed\" ]; then",
		"    echo '{\"number\":12,\"url\":\"https://github.com/acme/repo/pull/12\",\"state\":\"CLOSED\",\"isDraft\":false,\"headRefOid\":\"abc123\",\"headRefName\":\"cr-1-pr-lifecycle\",\"baseRefName\":\"main\",\"author\":{\"login\":\"bot\"},\"latestReviews\":[],\"statusCheckRollup\":[]}'",
		"    exit 0",
		"  fi",
		"  echo '{\"number\":12,\"url\":\"https://github.com/acme/repo/pull/12\",\"state\":\"OPEN\",\"isDraft\":false,\"headRefOid\":\"abc123\",\"headRefName\":\"cr-1-pr-lifecycle\",\"baseRefName\":\"main\",\"author\":{\"login\":\"bot\"},\"latestReviews\":[],\"statusCheckRollup\":[]}'",
		"  exit 0",
		"fi",
		"echo \"unexpected gh args: $@\" >&2",
		"exit 1",
	}, "\n") + "\n"
	if err := os.WriteFile(ghPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake gh lifecycle script: %v", err)
	}
	prependPath(t, binDir)
}

func prependPath(t *testing.T, prefix string) {
	t.Helper()
	current := os.Getenv("PATH")
	if strings.TrimSpace(current) == "" {
		t.Setenv("PATH", prefix)
		return
	}
	t.Setenv("PATH", prefix+string(os.PathListSeparator)+current)
}
