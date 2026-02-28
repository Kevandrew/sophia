package service

import (
	"errors"
	"strings"
	"testing"

	"sophia/internal/model"
)

func TestMergeManagedPRBodyPreservesReviewerTextOutsideManagedBlock(t *testing.T) {
	existing := strings.Join([]string{
		"Reviewer context above",
		"",
		"<!-- sophia:managed:start -->",
		"old managed",
		"<!-- sophia:managed:end -->",
		"",
		"Reviewer notes below",
	}, "\n")
	managed := strings.Join([]string{
		"<!-- sophia:managed:start -->",
		"new managed",
		"<!-- sophia:managed:end -->",
	}, "\n")

	merged, err := mergeManagedPRBody(existing, managed)
	if err != nil {
		t.Fatalf("mergeManagedPRBody() error = %v", err)
	}
	if !strings.Contains(merged, "Reviewer context above") || !strings.Contains(merged, "Reviewer notes below") {
		t.Fatalf("expected reviewer-authored sections to be preserved; got:\n%s", merged)
	}
	if !strings.Contains(merged, "new managed") || strings.Contains(merged, "old managed") {
		t.Fatalf("expected managed section replacement only; got:\n%s", merged)
	}
}

func TestMergeManagedPRBodyRejectsMarkerCorruption(t *testing.T) {
	_, err := mergeManagedPRBody("<!-- sophia:managed:start -->\npartial only", "<!-- sophia:managed:start -->\nnew\n<!-- sophia:managed:end -->")
	if err == nil {
		t.Fatalf("expected marker corruption error")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "marker corruption") {
		t.Fatalf("expected marker corruption error, got %v", err)
	}
}

func TestPRApprovalRequiredErrorContracts(t *testing.T) {
	err := &PRApprovalRequiredError{
		CRID:   61,
		Branch: "cr-61-test",
		Reason: "approve PR create/open to proceed",
	}
	if !errors.Is(err, ErrPRApprovalRequired) {
		t.Fatalf("expected errors.Is(err, ErrPRApprovalRequired) to be true")
	}
	details := err.Details()
	if got, _ := details["action"].(string); got != "open_pr" {
		t.Fatalf("expected action=open_pr, got %#v", details["action"])
	}
	if got, _ := details["approve_flag"].(string); got != "--approve-pr-open" {
		t.Fatalf("expected approve_flag, got %#v", details["approve_flag"])
	}
}

func TestEvaluatePRGateHonorsPolicyRequirements(t *testing.T) {
	policy := &model.RepoPolicy{
		Merge: model.PolicyMerge{
			RequiredApprovals:        intPtr(2),
			RequireNonAuthorApproval: boolPtr(true),
			RequireReadyForReview:    boolPtr(true),
			RequirePassingChecks:     boolPtr(true),
		},
	}
	status := &PRStatusView{
		Draft:              true,
		Approvals:          1,
		NonAuthorApprovals: 0,
		ChecksPassing:      false,
	}
	blocked, reasons := evaluatePRGate(policy, status)
	if !blocked {
		t.Fatalf("expected gate to be blocked")
	}
	if len(reasons) != 4 {
		t.Fatalf("expected 4 blocking reasons, got %#v", reasons)
	}
}

func TestClassifyGHCommandErrorAuthRequired(t *testing.T) {
	raw := errors.New("gh pr list failed: not logged into github.com. Run gh auth login")
	err := classifyGHCommandError(raw, []string{"pr", "list"})
	if !errors.Is(err, ErrGHAuthRequired) {
		t.Fatalf("expected ErrGHAuthRequired, got %v", err)
	}
	detailer, ok := err.(interface{ Details() map[string]any })
	if !ok {
		t.Fatalf("expected detailed error")
	}
	details := detailer.Details()
	actionRequired, ok := details["action_required"].(map[string]any)
	if !ok {
		t.Fatalf("expected action_required details, got %#v", details)
	}
	if got, _ := actionRequired["name"].(string); got != "gh_auth_login" {
		t.Fatalf("expected action_required.name=gh_auth_login, got %#v", actionRequired["name"])
	}
}

func TestClassifyPushCommandErrorPermissionDenied(t *testing.T) {
	raw := errors.New("git push failed: remote: Permission to repo denied")
	err := classifyPushCommandError(raw, "cr-1-branch")
	if !errors.Is(err, ErrPushPermissionDenied) {
		t.Fatalf("expected ErrPushPermissionDenied, got %v", err)
	}
	detailer, ok := err.(interface{ Details() map[string]any })
	if !ok {
		t.Fatalf("expected detailed error")
	}
	details := detailer.Details()
	actionRequired, ok := details["action_required"].(map[string]any)
	if !ok {
		t.Fatalf("expected action_required details, got %#v", details)
	}
	if got, _ := actionRequired["name"].(string); got != "request_push_access" {
		t.Fatalf("expected action_required.name=request_push_access, got %#v", actionRequired["name"])
	}
}

func TestStageArchiveForPRGateSkipsWhenArchiveDisabled(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	policy := &model.RepoPolicy{
		Merge: model.PolicyMerge{
			Mode: "pr_gate",
		},
		Archive: model.PolicyArchive{
			Enabled: boolPtr(false),
		},
	}
	cr := &model.CR{
		ID:         61,
		BaseBranch: "main",
		Branch:     "cr-61-test",
	}
	if err := svc.stageArchiveForPRGate(cr, policy); err != nil {
		t.Fatalf("stageArchiveForPRGate() error = %v", err)
	}
}
