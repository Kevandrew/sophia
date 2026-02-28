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

	out, _, runErr := runCLI(t, dir, "cr", "merge", "1", "--json")
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

func prependPath(t *testing.T, prefix string) {
	t.Helper()
	current := os.Getenv("PATH")
	if strings.TrimSpace(current) == "" {
		t.Setenv("PATH", prefix)
		return
	}
	t.Setenv("PATH", prefix+string(os.PathListSeparator)+current)
}
