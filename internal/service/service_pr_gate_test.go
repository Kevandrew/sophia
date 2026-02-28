package service

import (
	"errors"
	"strings"
	"testing"

	"sophia/internal/gitx"
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

func TestRenderManagedPRBlockIncludesBlastRadiusAndCRDrifts(t *testing.T) {
	review := &Review{
		CR: &model.CR{
			Title:      "Feature rollout",
			Branch:     "feature-rollout",
			BaseRef:    "main",
			BaseBranch: "main",
			Contract: model.Contract{
				Why:          "Improve rollout safety",
				BlastRadius:  "Affects deployment workflow and release bot comments.",
				RollbackPlan: "Revert merge commit.",
			},
			ContractDrifts: []model.CRContractDrift{
				{
					TS:           "2026-03-01T10:00:00Z",
					Fields:       []string{"scope"},
					Reason:       "Expanded for release metadata path",
					Acknowledged: true,
					AckReason:    "Reviewer accepted scope widening",
				},
			},
		},
		ShortStat: "2 files changed, 10 insertions(+), 1 deletion(-)",
	}

	md := renderManagedPRBlock(review)
	if !strings.Contains(md, "### Blast Radius") {
		t.Fatalf("expected blast radius section, got:\n%s", md)
	}
	if !strings.Contains(md, "Affects deployment workflow and release bot comments.") {
		t.Fatalf("expected blast radius content, got:\n%s", md)
	}
	if !strings.Contains(md, "### CR Contract Drifts") {
		t.Fatalf("expected CR contract drift section, got:\n%s", md)
	}
	if !strings.Contains(md, "Expanded for release metadata path") || !strings.Contains(md, "Reviewer accepted scope widening") {
		t.Fatalf("expected CR drift reason and ack reason, got:\n%s", md)
	}
}

func TestRenderManagedPRBlockUsesTaskContractDriftsForScopeDriftSignal(t *testing.T) {
	review := &Review{
		CR: &model.CR{
			Title:      "Task drift signal",
			Branch:     "task-drift",
			BaseRef:    "main",
			BaseBranch: "main",
			Contract: model.Contract{
				Why:          "Validate task drift rendering",
				BlastRadius:  "Renderer output only.",
				RollbackPlan: "Revert.",
			},
			Subtasks: []model.Subtask{
				{
					ID:               1,
					Title:            "Orphan checkpoint only",
					Status:           model.TaskStatusDone,
					CheckpointCommit: "aaaaaaaaaaaa",
					CheckpointOrphan: true,
				},
				{
					ID:               2,
					Title:            "Contract drift task",
					Status:           model.TaskStatusDone,
					CheckpointCommit: "bbbbbbbbbbbb",
					ContractDrifts: []model.TaskContractDrift{
						{
							TS:     "2026-03-01T11:00:00Z",
							Fields: []string{"scope"},
							Reason: "Scope widened for tests",
						},
					},
				},
			},
		},
		ShortStat: "1 file changed, 2 insertions(+), 0 deletions(-)",
	}

	md := renderManagedPRBlock(review)
	if strings.Contains(md, "| done | Orphan checkpoint only | - | - | aaaaaaaaaaaa | yes") {
		t.Fatalf("expected orphan-only task to not be marked as contract drift, got:\n%s", md)
	}
	if !strings.Contains(md, "| done | Contract drift task | - | - | bbbbbbbbbbbb | yes (1) |") {
		t.Fatalf("expected contract-drift task marker, got:\n%s", md)
	}
}

func TestRenderManagedPRBlockIncludesDiffBreakdownWithNumStats(t *testing.T) {
	ins := 7
	del := 3
	review := &Review{
		CR: &model.CR{
			Title:      "Diff table",
			Branch:     "diff-table",
			BaseRef:    "main",
			BaseBranch: "main",
			Contract: model.Contract{
				Why:          "Show per-file stats",
				BlastRadius:  "PR rendering.",
				RollbackPlan: "Revert.",
			},
		},
		ShortStat: "1 file changed, 7 insertions(+), 3 deletions(-)",
		Files:     []string{"internal/service/service_pr_gate.go"},
		DiffNumStats: []gitx.DiffNumStat{
			{
				Path:       "internal/service/service_pr_gate.go",
				Insertions: &ins,
				Deletions:  &del,
			},
		},
	}

	md := renderManagedPRBlock(review)
	if !strings.Contains(md, "#### File Breakdown") {
		t.Fatalf("expected file breakdown section, got:\n%s", md)
	}
	if !strings.Contains(md, "| internal/service/service_pr_gate.go | modified | 7 | 3 |") {
		t.Fatalf("expected per-file numstat row, got:\n%s", md)
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

func TestEvaluatePRGateBlocksWhenChecksRequiredButMissing(t *testing.T) {
	policy := &model.RepoPolicy{
		Merge: model.PolicyMerge{
			RequirePassingChecks: boolPtr(true),
		},
	}
	status := &PRStatusView{
		ChecksObserved: false,
		ChecksPassing:  false,
	}
	blocked, reasons := evaluatePRGate(policy, status)
	if !blocked {
		t.Fatalf("expected gate to be blocked")
	}
	found := false
	for _, reason := range reasons {
		if strings.Contains(reason, "not reported") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected missing-checks reason, got %#v", reasons)
	}
}

func TestParsePRNumberFromURL(t *testing.T) {
	if got := parsePRNumberFromURL("https://github.com/acme/repo/pull/123"); got != 123 {
		t.Fatalf("expected 123, got %d", got)
	}
	if got := parsePRNumberFromURL("https://github.com/acme/repo/pull/123/"); got != 123 {
		t.Fatalf("expected 123 with trailing slash, got %d", got)
	}
	if got := parsePRNumberFromURL("https://github.com/acme/repo/pull/not-a-number"); got != 0 {
		t.Fatalf("expected 0 for invalid URL, got %d", got)
	}
}

func TestBuildPRMergeArgsRespectsDeleteBranch(t *testing.T) {
	withDelete := buildPRMergeArgs(42, true, "")
	if strings.Join(withDelete, " ") != "pr merge 42 --merge --delete-branch" {
		t.Fatalf("unexpected args with delete: %#v", withDelete)
	}
	withoutDelete := buildPRMergeArgs(42, false, "")
	if strings.Join(withoutDelete, " ") != "pr merge 42 --merge" {
		t.Fatalf("unexpected args without delete: %#v", withoutDelete)
	}
}

func TestBuildPRMergeArgsIncludesMatchHeadCommit(t *testing.T) {
	args := buildPRMergeArgs(42, true, "abc123")
	got := strings.Join(args, " ")
	want := "pr merge 42 --merge --match-head-commit abc123 --delete-branch"
	if got != want {
		t.Fatalf("unexpected args: got %q want %q", got, want)
	}
}

func TestNormalizeCheckRollupStatePrefersStateField(t *testing.T) {
	got := normalizeCheckRollupState("COMPLETED", "SUCCESS", "FAILURE")
	if got != "FAILURE" {
		t.Fatalf("expected FAILURE, got %q", got)
	}
}

func TestCheckRollupStatePassing(t *testing.T) {
	if !checkRollupStatePassing("SUCCESS") {
		t.Fatalf("expected SUCCESS to be passing")
	}
	if checkRollupStatePassing("FAILURE") {
		t.Fatalf("expected FAILURE to be non-passing")
	}
	if checkRollupStatePassing("PENDING") {
		t.Fatalf("expected PENDING to be non-passing")
	}
}

func TestNormalizeGHRepoSelectorParsesCommonRemoteFormats(t *testing.T) {
	cases := map[string]string{
		"https://github.com/acme/repo.git":           "acme/repo",
		"git@github.com:acme/repo.git":               "acme/repo",
		"https://github.example.com/acme/repo":       "github.example.com/acme/repo",
		"ssh://git@github.example.com/acme/repo.git": "github.example.com/acme/repo",
		"github.example.com/acme/repo":               "github.example.com/acme/repo",
	}
	for input, want := range cases {
		if got := normalizeGHRepoSelector(input); got != want {
			t.Fatalf("normalizeGHRepoSelector(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestNormalizeGHRepoSelectorRejectsLocalRemotePath(t *testing.T) {
	if got := normalizeGHRepoSelector("/tmp/origin.git"); got != "" {
		t.Fatalf("expected empty selector for local path, got %q", got)
	}
}

func TestParseRepoSelectorParts(t *testing.T) {
	host, owner, repo, ok := parseRepoSelectorParts("acme/repo")
	if !ok || host != "" || owner != "acme" || repo != "repo" {
		t.Fatalf("unexpected parse for owner/repo: ok=%t host=%q owner=%q repo=%q", ok, host, owner, repo)
	}
	host, owner, repo, ok = parseRepoSelectorParts("github.example.com/acme/repo")
	if !ok || host != "github.example.com" || owner != "acme" || repo != "repo" {
		t.Fatalf("unexpected parse for host/owner/repo: ok=%t host=%q owner=%q repo=%q", ok, host, owner, repo)
	}
	if _, _, _, ok = parseRepoSelectorParts(""); ok {
		t.Fatalf("expected empty selector parse to fail")
	}
}

func TestIsGHProjectCardsSunsetError(t *testing.T) {
	err := errors.New("GraphQL: Projects (classic) is being deprecated in favor of the new Projects experience. (repository.pullRequest.projectCards)")
	if !isGHProjectCardsSunsetError(err) {
		t.Fatalf("expected projectCards sunset error to be detected")
	}
	if isGHProjectCardsSunsetError(errors.New("some other gh error")) {
		t.Fatalf("expected non-sunset error to be ignored")
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

func TestClassifyPushCommandErrorDoesNotMisclassifyNonPermissionFailures(t *testing.T) {
	raw := errors.New("git push failed: failed to push some refs to origin")
	err := classifyPushCommandError(raw, "cr-1-branch")
	if errors.Is(err, ErrPushPermissionDenied) {
		t.Fatalf("expected non-permission failure to remain generic, got %v", err)
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
