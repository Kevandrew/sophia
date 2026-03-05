package service

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sophia/internal/gitx"
	"sophia/internal/model"
)

func TestMergeManagedPRBodyPreservesReviewerTextOutsideManagedBlock(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
	_, err := mergeManagedPRBody("<!-- sophia:managed:start -->\npartial only", "<!-- sophia:managed:start -->\nnew\n<!-- sophia:managed:end -->")
	if err == nil {
		t.Fatalf("expected marker corruption error")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "marker corruption") {
		t.Fatalf("expected marker corruption error, got %v", err)
	}
}

func TestRenderManagedPRBlockIncludesBlastRadiusAndCRDrifts(t *testing.T) {
	t.Parallel()
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
					BeforeScope:  []string{"internal/service"},
					AfterScope:   []string{"internal/service", "internal/store"},
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
	if !strings.Contains(md, "Scope Delta") || !strings.Contains(md, "Added: internal/store") || !strings.Contains(md, "Removed: none") {
		t.Fatalf("expected readable scope delta rendering, got:\n%s", md)
	}
	if strings.Contains(md, "before_scope:") || strings.Contains(md, "| fields:") {
		t.Fatalf("expected legacy pipe-delimited CR drift format to be removed, got:\n%s", md)
	}
}

func TestRenderCRContractDriftSectionOmitsAckReasonWhenEmpty(t *testing.T) {
	t.Parallel()
	b := &strings.Builder{}
	renderCRContractDriftSection(b, []model.CRContractDrift{{
		ID:           1,
		TS:           "2026-03-01T10:00:00Z",
		Fields:       []string{"scope_changed"},
		Reason:       "Scope expanded",
		Acknowledged: true,
	}})
	md := b.String()
	if strings.Contains(md, "Ack Reason:") {
		t.Fatalf("expected Ack Reason line to be omitted when empty, got:\n%s", md)
	}
}

func TestRenderCRContractDriftSectionSortsByTimeThenID(t *testing.T) {
	t.Parallel()
	b := &strings.Builder{}
	renderCRContractDriftSection(b, []model.CRContractDrift{
		{ID: 2, TS: "2026-03-01T10:00:00Z", Fields: []string{"scope"}, Reason: "second"},
		{ID: 1, TS: "2026-03-01T10:00:00Z", Fields: []string{"scope"}, Reason: "first"},
		{ID: 3, TS: "2026-03-01T11:00:00Z", Fields: []string{"scope"}, Reason: "third"},
	})
	md := b.String()
	first := strings.Index(md, "- **Drift ID 1**")
	second := strings.Index(md, "- **Drift ID 2**")
	third := strings.Index(md, "- **Drift ID 3**")
	if first == -1 || second == -1 || third == -1 {
		t.Fatalf("expected all drift markers, got:\n%s", md)
	}
	if !(first < second && second < third) {
		t.Fatalf("expected stable sort by ts then id, got:\n%s", md)
	}
	if strings.Contains(md, "Drift #") {
		t.Fatalf("expected drift labels without # autolink pattern, got:\n%s", md)
	}
}

func TestScopeDeltaAddedAndRemoved(t *testing.T) {
	t.Parallel()
	added, removed := scopeDelta(
		[]string{"internal/cli", "internal/service", "internal/cli"},
		[]string{"internal/service", "internal/store", "internal/store"},
	)
	if strings.Join(added, ",") != "internal/store" {
		t.Fatalf("expected added internal/store, got %#v", added)
	}
	if strings.Join(removed, ",") != "internal/cli" {
		t.Fatalf("expected removed internal/cli, got %#v", removed)
	}
}

func TestScopeDeltaNoChanges(t *testing.T) {
	t.Parallel()
	added, removed := scopeDelta(
		[]string{"internal/service", "internal/model"},
		[]string{"internal/model", "internal/service"},
	)
	if len(added) != 0 || len(removed) != 0 {
		t.Fatalf("expected no delta for same sets, got added=%#v removed=%#v", added, removed)
	}
}

func TestRenderCRContractDriftSectionRendersNoneForEmptyScopeDelta(t *testing.T) {
	t.Parallel()
	b := &strings.Builder{}
	renderCRContractDriftSection(b, []model.CRContractDrift{{
		ID:          1,
		TS:          "2026-03-01T10:00:00Z",
		Fields:      []string{"scope_changed"},
		Reason:      "No-op scope normalization",
		BeforeScope: []string{"internal/model", "internal/service"},
		AfterScope:  []string{"internal/service", "internal/model"},
	}})
	md := b.String()
	if !strings.Contains(md, "- Scope Delta:") || !strings.Contains(md, "- Added: none") || !strings.Contains(md, "- Removed: none") {
		t.Fatalf("expected none values for empty scope delta, got:\n%s", md)
	}
}

func TestRenderManagedPRBlockUsesTaskContractDriftsForScopeDriftSignal(t *testing.T) {
	t.Parallel()
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
	if strings.Contains(md, "| done | Orphan checkpoint only | - | aaaaaaaaaaaa | yes") {
		t.Fatalf("expected orphan-only task to not be marked as contract drift, got:\n%s", md)
	}
	if !strings.Contains(md, "| done | Contract drift task | - | bbbbbbbbbbbb | yes (1) |") {
		t.Fatalf("expected contract-drift task marker, got:\n%s", md)
	}
}

func TestRenderManagedPRBlockIncludesDiffBreakdownWithNumStats(t *testing.T) {
	t.Parallel()
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
	if !strings.Contains(md, "<details><summary>File Breakdown") {
		t.Fatalf("expected file breakdown section, got:\n%s", md)
	}
	if !strings.Contains(md, "| internal/service/service_pr_gate.go | modified | 7 | 3 |") {
		t.Fatalf("expected per-file numstat row, got:\n%s", md)
	}
}

func TestPRApprovalRequiredErrorContracts(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
	args := buildPRMergeArgs(42, true, "abc123")
	got := strings.Join(args, " ")
	want := "pr merge 42 --merge --match-head-commit abc123 --delete-branch"
	if got != want {
		t.Fatalf("unexpected args: got %q want %q", got, want)
	}
}

func TestNormalizeCheckRollupStatePrefersStateField(t *testing.T) {
	t.Parallel()
	got := normalizeCheckRollupState("COMPLETED", "SUCCESS", "FAILURE")
	if got != "FAILURE" {
		t.Fatalf("expected FAILURE, got %q", got)
	}
}

func TestCheckRollupStatePassing(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
	if got := normalizeGHRepoSelector("/tmp/origin.git"); got != "" {
		t.Fatalf("expected empty selector for local path, got %q", got)
	}
}

func TestParseRepoSelectorParts(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
	err := errors.New("GraphQL: Projects (classic) is being deprecated in favor of the new Projects experience. (repository.pullRequest.projectCards)")
	if !isGHProjectCardsSunsetError(err) {
		t.Fatalf("expected projectCards sunset error to be detected")
	}
	if isGHProjectCardsSunsetError(errors.New("some other gh error")) {
		t.Fatalf("expected non-sunset error to be ignored")
	}
}

func TestRenderCheckpointSyncCommentFormat(t *testing.T) {
	t.Parallel()
	task := model.Subtask{
		ID:    3,
		Title: "Enable pr-gate merge mode",
		ContractBaseline: model.TaskContractBaseline{
			Intent:             "Enable PR-gated merge path.",
			AcceptanceCriteria: []string{"PR sync runs before merge finalize", "Gate report is deterministic"},
			Scope:              []string{"internal/service/service_pr_gate.go"},
		},
		CheckpointScope: []string{"internal/service/service_pr_gate.go"},
	}
	got := renderCheckpointSyncComment(task, "abc123def456", "feat: enable merge mode", "task:3:abc123def456")
	expectedParts := []string{
		"### Checkpoint sync: task 3 - Enable pr-gate merge mode",
		"Intent: Enable PR-gated merge path.",
		"Acceptance Criteria:",
		"- PR sync runs before merge finalize",
		"- Gate report is deterministic",
		"| Contract Scope | internal/service/service_pr_gate.go |",
		"| Checkpoint Scope | internal/service/service_pr_gate.go |",
		"Commits in this sync:",
		"- `abc123def456` feat: enable merge mode",
		"<!-- sophia:checkpoint-sync:task:3:abc123def456 -->",
	}
	for _, part := range expectedParts {
		if !strings.Contains(got, part) {
			t.Fatalf("expected comment to contain %q, got:\n%s", part, got)
		}
	}
}

func TestExtractCheckpointSyncCommentKey(t *testing.T) {
	t.Parallel()
	body := "hello\n\n<!-- sophia:checkpoint-sync:task:5:abc123 -->\n"
	if got := extractCheckpointSyncCommentKey(body); got != "task:5:abc123" {
		t.Fatalf("unexpected extracted key: %q", got)
	}
	if got := extractCheckpointSyncCommentKey("no marker"); got != "" {
		t.Fatalf("expected empty key without marker, got %q", got)
	}
}

func TestIndexCheckpointSyncCommentsUsesMarkerKey(t *testing.T) {
	t.Parallel()
	comments := []ghIssueComment{
		{ID: 11, Body: "a\n<!-- sophia:checkpoint-sync:task:1:a1 -->"},
		{ID: 12, Body: "b\n<!-- sophia:checkpoint-sync:task:2:b2 -->"},
		{ID: 13, Body: "without marker"},
	}
	index := indexCheckpointSyncComments(comments)
	if len(index) != 2 {
		t.Fatalf("expected 2 keyed comments, got %d (%#v)", len(index), index)
	}
	if got := index["task:1:a1"].ID; got != 11 {
		t.Fatalf("expected marker task:1:a1 to map to id 11, got %d", got)
	}
	if got := index["task:2:b2"].ID; got != 12 {
		t.Fatalf("expected marker task:2:b2 to map to id 12, got %d", got)
	}
}

func TestParseRevListOutput(t *testing.T) {
	t.Parallel()
	raw := "\nabc123\n\n  def456  \n"
	got := parseRevListOutput(raw)
	if len(got) != 2 || got[0] != "abc123" || got[1] != "def456" {
		t.Fatalf("unexpected rev-list parse: %#v", got)
	}
}

func TestValidateCheckpointStrictOrderAllowsSequential(t *testing.T) {
	t.Parallel()
	missing := []string{"c1", "c2", "c3"}
	pending := []checkpointSyncPending{
		{TaskID: 1, Commit: "c1", MissingIndex: 0},
		{TaskID: 2, Commit: "c2", MissingIndex: 1},
	}
	if err := validateCheckpointStrictOrder(pending, missing); err != nil {
		t.Fatalf("expected sequential checkpoints to pass, got %v", err)
	}
}

func TestValidateCheckpointStrictOrderRejectsMixed(t *testing.T) {
	t.Parallel()
	missing := []string{"extra", "checkpoint"}
	pending := []checkpointSyncPending{
		{TaskID: 2, Commit: "checkpoint", MissingIndex: 1},
	}
	err := validateCheckpointStrictOrder(pending, missing)
	if err == nil {
		t.Fatalf("expected strict order error for mixed commit sequence")
	}
	if !strings.Contains(err.Error(), "clean checkpoint-only branch order") {
		t.Fatalf("unexpected strict order error: %v", err)
	}
}

func TestClassifyPushCommandErrorPermissionDenied(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
	raw := errors.New("git push failed: failed to push some refs to origin")
	err := classifyPushCommandError(raw, "cr-1-branch")
	if errors.Is(err, ErrPushPermissionDenied) {
		t.Fatalf("expected non-permission failure to remain generic, got %v", err)
	}
}

func TestPRStatusReturnsNoLinkedPRActionMetadata(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SOPHIA.yaml"), []byte("version: v1\nmerge:\n  mode: pr_gate\narchive:\n  enabled: false\n"), 0o644); err != nil {
		t.Fatalf("write SOPHIA.yaml: %v", err)
	}
	cr, err := svc.AddCR("PR status metadata", "ensure no-linked payload")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	status, err := svc.PRStatus(cr.ID)
	if err != nil {
		t.Fatalf("PRStatus() error = %v", err)
	}
	if status.LinkageState != prLinkageNoLinkedPR {
		t.Fatalf("expected linkage state %q, got %q", prLinkageNoLinkedPR, status.LinkageState)
	}
	if status.ActionRequired != prActionOpenPR {
		t.Fatalf("expected action_required %q, got %q", prActionOpenPR, status.ActionRequired)
	}
	if len(status.SuggestedCommands) == 0 || status.SuggestedCommands[0] != "sophia cr pr open 1 --approve-open" {
		t.Fatalf("expected suggested open command, got %#v", status.SuggestedCommands)
	}
}

func TestReconcileRemoteMergedPRCompletesDelegatedParentTask(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")

	parent, err := svc.AddCR("Parent refactor", "umbrella intent")
	if err != nil {
		t.Fatalf("AddCR(parent) error = %v", err)
	}
	setValidContract(t, svc, parent.ID)
	parentTask, err := svc.AddTask(parent.ID, "Split service layer")
	if err != nil {
		t.Fatalf("AddTask(parent) error = %v", err)
	}
	setValidTaskContract(t, svc, parent.ID, parentTask.ID)

	if err := os.WriteFile(filepath.Join(dir, "parent.txt"), []byte("parent\n"), 0o644); err != nil {
		t.Fatalf("write parent file: %v", err)
	}
	runGit(t, dir, "add", "parent.txt")
	runGit(t, dir, "commit", "-m", "feat: parent intent commit")

	child, _, err := svc.AddCRWithOptionsWithWarnings("Child slice", "delegated implementation", AddCROptions{ParentCRID: parent.ID})
	if err != nil {
		t.Fatalf("AddCR(child) error = %v", err)
	}
	setValidContract(t, svc, child.ID)
	if _, err := svc.DelegateTaskToChild(parent.ID, parentTask.ID, child.ID); err != nil {
		t.Fatalf("DelegateTaskToChild() error = %v", err)
	}

	if err := os.WriteFile(filepath.Join(dir, "child.txt"), []byte("child\n"), 0o644); err != nil {
		t.Fatalf("write child file: %v", err)
	}
	runGit(t, dir, "add", "child.txt")
	runGit(t, dir, "commit", "-m", "feat: child implementation")

	child, err = svc.store.LoadCR(child.ID)
	if err != nil {
		t.Fatalf("LoadCR(child) error = %v", err)
	}
	head, err := svc.git.ResolveRef("HEAD")
	if err != nil {
		t.Fatalf("RevParse(HEAD) error = %v", err)
	}
	status := &PRStatusView{
		Merged:       true,
		MergedAt:     svc.timestamp(),
		MergedCommit: strings.TrimSpace(head),
	}
	if err := svc.reconcileRemoteMergedPR(child, status); err != nil {
		t.Fatalf("reconcileRemoteMergedPR(child) error = %v", err)
	}

	reloadedParent, err := svc.store.LoadCR(parent.ID)
	if err != nil {
		t.Fatalf("LoadCR(parent) error = %v", err)
	}
	taskIndex := indexOfTask(reloadedParent.Subtasks, parentTask.ID)
	if taskIndex < 0 {
		t.Fatalf("expected parent task %d to exist", parentTask.ID)
	}
	if reloadedParent.Subtasks[taskIndex].Status != model.TaskStatusDone {
		t.Fatalf("expected delegated parent task auto-completed after remote child merge, got %#v", reloadedParent.Subtasks[taskIndex])
	}
	if strings.TrimSpace(reloadedParent.Subtasks[taskIndex].CompletedAt) == "" {
		t.Fatalf("expected delegated parent task completed_at to be set, got %#v", reloadedParent.Subtasks[taskIndex])
	}
}

func TestReconcileRemoteMergedPRWaitsForAllDelegatedChildren(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")

	parent, err := svc.AddCR("Parent refactor", "umbrella intent")
	if err != nil {
		t.Fatalf("AddCR(parent) error = %v", err)
	}
	setValidContract(t, svc, parent.ID)
	parentTask, err := svc.AddTask(parent.ID, "Split service layer")
	if err != nil {
		t.Fatalf("AddTask(parent) error = %v", err)
	}
	setValidTaskContract(t, svc, parent.ID, parentTask.ID)

	if err := os.WriteFile(filepath.Join(dir, "parent.txt"), []byte("parent\n"), 0o644); err != nil {
		t.Fatalf("write parent file: %v", err)
	}
	runGit(t, dir, "add", "parent.txt")
	runGit(t, dir, "commit", "-m", "feat: parent intent commit")

	childOne, _, err := svc.AddCRWithOptionsWithWarnings("Child one", "delegated implementation", AddCROptions{ParentCRID: parent.ID})
	if err != nil {
		t.Fatalf("AddCR(childOne) error = %v", err)
	}
	setValidContract(t, svc, childOne.ID)
	if _, err := svc.DelegateTaskToChild(parent.ID, parentTask.ID, childOne.ID); err != nil {
		t.Fatalf("DelegateTaskToChild(childOne) error = %v", err)
	}

	if err := os.WriteFile(filepath.Join(dir, "child-one.txt"), []byte("child one\n"), 0o644); err != nil {
		t.Fatalf("write child one file: %v", err)
	}
	runGit(t, dir, "add", "child-one.txt")
	runGit(t, dir, "commit", "-m", "feat: child one implementation")
	childOneHead, err := svc.git.ResolveRef("HEAD")
	if err != nil {
		t.Fatalf("RevParse(HEAD child one) error = %v", err)
	}

	childTwo, _, err := svc.AddCRWithOptionsWithWarnings("Child two", "delegated implementation", AddCROptions{ParentCRID: parent.ID})
	if err != nil {
		t.Fatalf("AddCR(childTwo) error = %v", err)
	}
	setValidContract(t, svc, childTwo.ID)
	if _, err := svc.DelegateTaskToChild(parent.ID, parentTask.ID, childTwo.ID); err != nil {
		t.Fatalf("DelegateTaskToChild(childTwo) error = %v", err)
	}

	if err := os.WriteFile(filepath.Join(dir, "child-two.txt"), []byte("child two\n"), 0o644); err != nil {
		t.Fatalf("write child two file: %v", err)
	}
	runGit(t, dir, "add", "child-two.txt")
	runGit(t, dir, "commit", "-m", "feat: child two implementation")
	childTwoHead, err := svc.git.ResolveRef("HEAD")
	if err != nil {
		t.Fatalf("RevParse(HEAD child two) error = %v", err)
	}

	reloadedChildOne, err := svc.store.LoadCR(childOne.ID)
	if err != nil {
		t.Fatalf("LoadCR(childOne) error = %v", err)
	}
	if err := svc.reconcileRemoteMergedPR(reloadedChildOne, &PRStatusView{
		Merged:       true,
		MergedAt:     svc.timestamp(),
		MergedCommit: strings.TrimSpace(childOneHead),
	}); err != nil {
		t.Fatalf("reconcileRemoteMergedPR(childOne) error = %v", err)
	}

	reloadedParent, err := svc.store.LoadCR(parent.ID)
	if err != nil {
		t.Fatalf("LoadCR(parent after child one) error = %v", err)
	}
	taskIndex := indexOfTask(reloadedParent.Subtasks, parentTask.ID)
	if taskIndex < 0 {
		t.Fatalf("expected parent task %d to exist", parentTask.ID)
	}
	if reloadedParent.Subtasks[taskIndex].Status != model.TaskStatusDelegated {
		t.Fatalf("expected parent task to remain delegated until all children merge, got %#v", reloadedParent.Subtasks[taskIndex])
	}

	reloadedChildTwo, err := svc.store.LoadCR(childTwo.ID)
	if err != nil {
		t.Fatalf("LoadCR(childTwo) error = %v", err)
	}
	if err := svc.reconcileRemoteMergedPR(reloadedChildTwo, &PRStatusView{
		Merged:       true,
		MergedAt:     svc.timestamp(),
		MergedCommit: strings.TrimSpace(childTwoHead),
	}); err != nil {
		t.Fatalf("reconcileRemoteMergedPR(childTwo) error = %v", err)
	}

	reloadedParent, err = svc.store.LoadCR(parent.ID)
	if err != nil {
		t.Fatalf("LoadCR(parent after child two) error = %v", err)
	}
	taskIndex = indexOfTask(reloadedParent.Subtasks, parentTask.ID)
	if taskIndex < 0 {
		t.Fatalf("expected parent task %d to exist", parentTask.ID)
	}
	if reloadedParent.Subtasks[taskIndex].Status != model.TaskStatusDone {
		t.Fatalf("expected parent task done after all delegated children merge, got %#v", reloadedParent.Subtasks[taskIndex])
	}
}

func TestPRStatusReturnsStaleWhenLinkedPRNotFound(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SOPHIA.yaml"), []byte("version: v1\nmerge:\n  mode: pr_gate\narchive:\n  enabled: false\n"), 0o644); err != nil {
		t.Fatalf("write SOPHIA.yaml: %v", err)
	}
	cr, err := svc.AddCR("PR stale not found", "status path")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	loaded, err := svc.store.LoadCR(cr.ID)
	if err != nil {
		t.Fatalf("LoadCR() error = %v", err)
	}
	loaded.PR.Number = 42
	loaded.PR.Repo = "acme/repo"
	if err := svc.store.SaveCR(loaded); err != nil {
		t.Fatalf("SaveCR() error = %v", err)
	}
	installFakeGHCommand(t, "echo 'no pull requests found for branch \"x\"' >&2\nexit 1\n")

	status, err := svc.PRStatus(cr.ID)
	if err != nil {
		t.Fatalf("PRStatus() error = %v", err)
	}
	if status.LinkageState != prLinkagePRNotFound {
		t.Fatalf("expected linkage state %q, got %q", prLinkagePRNotFound, status.LinkageState)
	}
	if status.ActionRequired != prActionReconcilePR {
		t.Fatalf("expected action_required %q, got %q", prActionReconcilePR, status.ActionRequired)
	}
	if !status.GateBlocked {
		t.Fatalf("expected gate to be blocked")
	}
	reloaded, err := svc.store.LoadCR(cr.ID)
	if err != nil {
		t.Fatalf("LoadCR() error = %v", err)
	}
	if reloaded.Status != model.StatusInProgress {
		t.Fatalf("expected stale detection to keep CR in_progress, got %q", reloaded.Status)
	}
	if strings.TrimSpace(reloaded.MergedCommit) != "" || strings.TrimSpace(reloaded.MergedAt) != "" {
		t.Fatalf("expected stale detection to avoid silent merge mutation, got merged_at=%q merged_commit=%q", reloaded.MergedAt, reloaded.MergedCommit)
	}
}

func TestPRStatusClosedUnmergedDoesNotSilentlyMutateCRLifecycle(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SOPHIA.yaml"), []byte("version: v1\nmerge:\n  mode: pr_gate\narchive:\n  enabled: false\n"), 0o644); err != nil {
		t.Fatalf("write SOPHIA.yaml: %v", err)
	}
	cr, err := svc.AddCR("PR stale closed", "status path")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	loaded, err := svc.store.LoadCR(cr.ID)
	if err != nil {
		t.Fatalf("LoadCR() error = %v", err)
	}
	loaded.PR.Number = 90
	loaded.PR.Repo = "acme/repo"
	if err := svc.store.SaveCR(loaded); err != nil {
		t.Fatalf("SaveCR() error = %v", err)
	}
	installFakeGHCommand(t, "#!/bin/sh\nif [ \"$1\" = \"-R\" ]; then\n  shift\n  shift\nfi\nif [ \"$1\" = \"pr\" ] && [ \"$2\" = \"view\" ]; then\n  echo '{\"number\":90,\"url\":\"https://github.com/acme/repo/pull/90\",\"state\":\"CLOSED\",\"isDraft\":false,\"headRefOid\":\"abc123\",\"headRefName\":\"cr-1-pr-stale-closed\",\"baseRefName\":\"main\",\"author\":{\"login\":\"bot\"},\"latestReviews\":[],\"statusCheckRollup\":[]}'\n  exit 0\nfi\necho \"unexpected gh args: $@\" >&2\nexit 1\n")

	status, err := svc.PRStatus(cr.ID)
	if err != nil {
		t.Fatalf("PRStatus() error = %v", err)
	}
	if status.LinkageState != prLinkageClosed {
		t.Fatalf("expected linkage state %q, got %q", prLinkageClosed, status.LinkageState)
	}
	reloaded, err := svc.store.LoadCR(cr.ID)
	if err != nil {
		t.Fatalf("LoadCR() after PRStatus error = %v", err)
	}
	if reloaded.Status != model.StatusInProgress {
		t.Fatalf("expected closed PR detection to keep CR in_progress, got %q", reloaded.Status)
	}
	if strings.TrimSpace(reloaded.MergedCommit) != "" || strings.TrimSpace(reloaded.MergedAt) != "" {
		t.Fatalf("expected no silent merged transition, got merged_at=%q merged_commit=%q", reloaded.MergedAt, reloaded.MergedCommit)
	}
	if strings.TrimSpace(reloaded.AbandonedAt) != "" {
		t.Fatalf("expected no silent abandon/archive transition, got abandoned_at=%q", reloaded.AbandonedAt)
	}
}

func TestPRReadyBlockedWhenNoTaskCheckpointCommits(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SOPHIA.yaml"), []byte("version: v1\nmerge:\n  mode: pr_gate\narchive:\n  enabled: false\n"), 0o644); err != nil {
		t.Fatalf("write SOPHIA.yaml: %v", err)
	}
	cr, err := svc.AddCR("PR ready blocked", "no checkpoint path")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	loaded, err := svc.store.LoadCR(cr.ID)
	if err != nil {
		t.Fatalf("LoadCR() error = %v", err)
	}
	loaded.PR.Number = 52
	loaded.PR.Repo = "acme/repo"
	if err := svc.store.SaveCR(loaded); err != nil {
		t.Fatalf("SaveCR() error = %v", err)
	}

	ghInvokedMarker := filepath.Join(t.TempDir(), "gh-invoked")
	installFakeGHCommand(t, fmt.Sprintf(strings.Join([]string{
		"#!/bin/sh",
		"if [ \"$1\" = \"-R\" ]; then",
		"  shift",
		"  shift",
		"fi",
		"printf '%%s\\n' \"$*\" >> %q",
		"if [ \"$1\" = \"pr\" ] && [ \"$2\" = \"ready\" ]; then",
		"  echo 'unexpected gh pr ready call' >&2",
		"  exit 1",
		"fi",
		"echo '{\"number\":52,\"url\":\"https://github.com/acme/repo/pull/52\",\"state\":\"OPEN\",\"isDraft\":true,\"headRefOid\":\"abc123\",\"headRefName\":\"cr-1-pr-ready-blocked\",\"baseRefName\":\"main\",\"author\":{\"login\":\"bot\"},\"latestReviews\":[],\"statusCheckRollup\":[]}'",
		"exit 0",
	}, "\n"), ghInvokedMarker))

	status, err := svc.PRReady(cr.ID)
	if status != nil {
		t.Fatalf("expected nil status when readiness is blocked, got %#v", status)
	}
	if err == nil {
		t.Fatalf("expected PRReady() to return blocked error")
	}
	var blocked *PRReadyBlockedError
	if !errors.As(err, &blocked) {
		t.Fatalf("expected PRReadyBlockedError, got %T (%v)", err, err)
	}
	if blocked.ReasonCode != prReadyBlockedReasonNoCheckpoints {
		t.Fatalf("expected reason code %q, got %q", prReadyBlockedReasonNoCheckpoints, blocked.ReasonCode)
	}
	if strings.TrimSpace(blocked.Reason) == "" {
		t.Fatalf("expected non-empty blocked reason")
	}
	if _, statErr := os.Stat(ghInvokedMarker); !os.IsNotExist(statErr) {
		t.Fatalf("expected gh to remain uninvoked, stat err=%v", statErr)
	}
}

func TestPRReadySucceedsWhenTaskCheckpointCommitExists(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SOPHIA.yaml"), []byte("version: v1\nmerge:\n  mode: pr_gate\narchive:\n  enabled: false\n"), 0o644); err != nil {
		t.Fatalf("write SOPHIA.yaml: %v", err)
	}
	cr, err := svc.AddCR("PR ready allowed", "checkpoint path")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	loaded, err := svc.store.LoadCR(cr.ID)
	if err != nil {
		t.Fatalf("LoadCR() error = %v", err)
	}
	loaded.PR.Number = 64
	loaded.PR.Repo = "acme/repo"
	loaded.Subtasks = []model.Subtask{
		{
			ID:               1,
			Title:            "Implement checkpointed task",
			Status:           model.TaskStatusDone,
			CheckpointCommit: "abc123",
		},
	}
	if err := svc.store.SaveCR(loaded); err != nil {
		t.Fatalf("SaveCR() error = %v", err)
	}

	readyCalledMarker := filepath.Join(t.TempDir(), "ready-called")
	installFakeGHCommand(t, fmt.Sprintf(strings.Join([]string{
		"#!/bin/sh",
		"if [ \"$1\" = \"-R\" ]; then",
		"  shift",
		"  shift",
		"fi",
		"if [ \"$1\" = \"pr\" ] && [ \"$2\" = \"ready\" ]; then",
		"  printf 'called' > %q",
		"  exit 0",
		"fi",
		"if [ \"$1\" = \"pr\" ] && [ \"$2\" = \"view\" ]; then",
		"  echo '{\"number\":64,\"url\":\"https://github.com/acme/repo/pull/64\",\"state\":\"OPEN\",\"isDraft\":false,\"headRefOid\":\"def456\",\"headRefName\":\"cr-1-pr-ready-allowed\",\"baseRefName\":\"main\",\"author\":{\"login\":\"bot\"},\"latestReviews\":[],\"statusCheckRollup\":[]}'",
		"  exit 0",
		"fi",
		"echo \"unexpected gh args: $@\" >&2",
		"exit 1",
	}, "\n"), readyCalledMarker))

	status, err := svc.PRReady(cr.ID)
	if err != nil {
		t.Fatalf("PRReady() error = %v", err)
	}
	if status == nil {
		t.Fatalf("expected status from PRReady()")
	}
	if status.Draft {
		t.Fatalf("expected ready transition to return non-draft status")
	}
	if _, statErr := os.Stat(readyCalledMarker); statErr != nil {
		t.Fatalf("expected gh pr ready invocation marker, stat err=%v", statErr)
	}
}

func TestClassifyPRLinkageStatusMarksClosedUnmergedPRsStale(t *testing.T) {
	t.Parallel()
	svc := &Service{}
	cr := &model.CR{
		ID:         8,
		Branch:     "cr-8-intent",
		BaseRef:    "main",
		BaseBranch: "main",
	}
	status := &PRStatusView{
		Number: 19,
		State:  "CLOSED",
	}
	svc.classifyPRLinkageStatus(cr, status)
	if status.LinkageState != prLinkageClosed {
		t.Fatalf("expected linkage state %q, got %q", prLinkageClosed, status.LinkageState)
	}
	if status.ActionRequired != prActionReopenPR {
		t.Fatalf("expected action_required %q, got %q", prActionReopenPR, status.ActionRequired)
	}
	if !status.GateBlocked {
		t.Fatalf("expected closed-unmerged PR to block gate")
	}
	if len(status.SuggestedCommands) < 2 {
		t.Fatalf("expected stale suggested commands, got %#v", status.SuggestedCommands)
	}
}

func TestClassifyPRLinkageStatusMarksDraftPRActionMetadata(t *testing.T) {
	t.Parallel()
	svc := &Service{}
	cr := &model.CR{
		ID:         12,
		Branch:     "cr-12-target",
		BaseRef:    "main",
		BaseBranch: "main",
	}
	status := &PRStatusView{
		Number:      41,
		State:       "OPEN",
		Draft:       true,
		HeadRefName: "cr-12-target",
		BaseRefName: "main",
	}
	svc.classifyPRLinkageStatus(cr, status)
	if status.ActionRequired != prActionReadyPR {
		t.Fatalf("expected draft action_required=%q, got %q", prActionReadyPR, status.ActionRequired)
	}
	if !strings.Contains(status.ActionReason, "draft") {
		t.Fatalf("expected draft action reason, got %q", status.ActionReason)
	}
	if len(status.SuggestedCommands) == 0 || status.SuggestedCommands[0] != "sophia cr pr ready 12" {
		t.Fatalf("expected draft suggested ready command, got %#v", status.SuggestedCommands)
	}
}

func TestClassifyPRLinkageStatusMarksMismatchStale(t *testing.T) {
	t.Parallel()
	svc := &Service{}
	cr := &model.CR{
		ID:         11,
		Branch:     "cr-11-target",
		BaseRef:    "main",
		BaseBranch: "main",
	}
	status := &PRStatusView{
		Number:      33,
		State:       "OPEN",
		HeadRefName: "different-branch",
		BaseRefName: "develop",
	}
	svc.classifyPRLinkageStatus(cr, status)
	if status.LinkageState != prLinkageMismatch {
		t.Fatalf("expected linkage state %q, got %q", prLinkageMismatch, status.LinkageState)
	}
	if status.ActionRequired != prActionReconcilePR {
		t.Fatalf("expected action_required %q, got %q", prActionReconcilePR, status.ActionRequired)
	}
	if !strings.Contains(status.ActionReason, "base ref mismatch") || !strings.Contains(status.ActionReason, "head ref mismatch") {
		t.Fatalf("expected mismatch reason details, got %q", status.ActionReason)
	}
	if !status.GateBlocked {
		t.Fatalf("expected mismatch to block gate")
	}
}

func TestPRReconcileRelinkLinksMatchingPRAndRecordsEvent(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SOPHIA.yaml"), []byte("version: v1\nmerge:\n  mode: pr_gate\narchive:\n  enabled: false\n"), 0o644); err != nil {
		t.Fatalf("write SOPHIA.yaml: %v", err)
	}
	cr, err := svc.AddCR("PR reconcile relink", "status path")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	loaded, err := svc.store.LoadCR(cr.ID)
	if err != nil {
		t.Fatalf("LoadCR() error = %v", err)
	}
	loaded.PR.Number = 404
	loaded.PR.Repo = "acme/repo"
	if err := svc.store.SaveCR(loaded); err != nil {
		t.Fatalf("SaveCR() error = %v", err)
	}
	installFakeGHCommand(t, "#!/bin/sh\nif [ \"$1\" = \"-R\" ]; then\n  shift\n  shift\nfi\nif [ \"$1\" = \"pr\" ] && [ \"$2\" = \"list\" ]; then\n  echo '[{\"number\":77,\"url\":\"https://github.com/acme/repo/pull/77\",\"state\":\"OPEN\",\"isDraft\":true,\"headRefOid\":\"abc123\",\"headRefName\":\"cr-1-pr-reconcile-relink\",\"baseRefName\":\"main\",\"updatedAt\":\"2026-03-03T00:00:00Z\"}]'\n  exit 0\nfi\nif [ \"$1\" = \"pr\" ] && [ \"$2\" = \"view\" ]; then\n  echo '{\"number\":77,\"url\":\"https://github.com/acme/repo/pull/77\",\"state\":\"OPEN\",\"isDraft\":true,\"headRefOid\":\"abc123\",\"headRefName\":\"cr-1-pr-reconcile-relink\",\"baseRefName\":\"main\",\"author\":{\"login\":\"bot\"},\"latestReviews\":[],\"statusCheckRollup\":[]}'\n  exit 0\nfi\necho \"unexpected gh args: $@\" >&2\nexit 1\n")

	view, err := svc.PRReconcile(cr.ID, prReconcileModeRelink)
	if err != nil {
		t.Fatalf("PRReconcile(relink) error = %v", err)
	}
	if !view.Mutated {
		t.Fatalf("expected relink to mutate linkage")
	}
	if view.AfterPRNumber != 77 {
		t.Fatalf("expected relinked PR number 77, got %d", view.AfterPRNumber)
	}

	reloaded, err := svc.store.LoadCR(cr.ID)
	if err != nil {
		t.Fatalf("LoadCR() after reconcile error = %v", err)
	}
	if reloaded.PR.Number != 77 {
		t.Fatalf("expected stored PR number 77, got %d", reloaded.PR.Number)
	}
	found := false
	for _, event := range reloaded.Events {
		if event.Type == model.EventTypeCRReconciled && event.Meta["mode"] == prReconcileModeRelink {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected reconcile event with relink mode, got %#v", reloaded.Events)
	}
}

func TestPRUnreadyCloseReopenLifecycleOperations(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SOPHIA.yaml"), []byte("version: v1\nmerge:\n  mode: pr_gate\narchive:\n  enabled: false\n"), 0o644); err != nil {
		t.Fatalf("write SOPHIA.yaml: %v", err)
	}
	cr, err := svc.AddCR("PR lifecycle commands", "status path")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	loaded, err := svc.store.LoadCR(cr.ID)
	if err != nil {
		t.Fatalf("LoadCR() error = %v", err)
	}
	loaded.PR.Number = 64
	loaded.PR.Repo = "acme/repo"
	loaded.PR.State = "OPEN"
	loaded.PR.Draft = false
	branchName := strings.TrimSpace(loaded.Branch)
	if branchName == "" {
		branchName = "cr-1-pr-lifecycle"
	}
	if err := svc.store.SaveCR(loaded); err != nil {
		t.Fatalf("SaveCR() error = %v", err)
	}
	installFakeGHCommand(t, fmt.Sprintf(strings.Join([]string{
		"#!/bin/sh",
		"if [ \"$1\" = \"-R\" ]; then",
		"  shift",
		"  shift",
		"fi",
		"if [ \"$1\" = \"pr\" ] && [ \"$2\" = \"ready\" ] && [ \"$4\" = \"--undo\" ]; then",
		"  printf 'draft' > \"$TMPDIR/sophia-pr-state-64\"",
		"  exit 0",
		"fi",
		"if [ \"$1\" = \"pr\" ] && [ \"$2\" = \"close\" ]; then",
		"  printf 'closed' > \"$TMPDIR/sophia-pr-state-64\"",
		"  exit 0",
		"fi",
		"if [ \"$1\" = \"pr\" ] && [ \"$2\" = \"reopen\" ]; then",
		"  printf 'open' > \"$TMPDIR/sophia-pr-state-64\"",
		"  exit 0",
		"fi",
		"if [ \"$1\" = \"pr\" ] && [ \"$2\" = \"view\" ]; then",
		"  state_file=\"$TMPDIR/sophia-pr-state-64\"",
		"  current='open'",
		"  if [ -f \"$state_file\" ]; then",
		"    current=$(cat \"$state_file\")",
		"  fi",
		"  if [ \"$current\" = \"draft\" ]; then",
		"    echo '{\"number\":64,\"url\":\"https://github.com/acme/repo/pull/64\",\"state\":\"OPEN\",\"isDraft\":true,\"headRefOid\":\"abc123\",\"headRefName\":\"%s\",\"baseRefName\":\"main\",\"author\":{\"login\":\"bot\"},\"latestReviews\":[],\"statusCheckRollup\":[]}'",
		"    exit 0",
		"  fi",
		"  if [ \"$current\" = \"closed\" ]; then",
		"    echo '{\"number\":64,\"url\":\"https://github.com/acme/repo/pull/64\",\"state\":\"CLOSED\",\"isDraft\":false,\"headRefOid\":\"abc123\",\"headRefName\":\"%s\",\"baseRefName\":\"main\",\"author\":{\"login\":\"bot\"},\"latestReviews\":[],\"statusCheckRollup\":[]}'",
		"    exit 0",
		"  fi",
		"  echo '{\"number\":64,\"url\":\"https://github.com/acme/repo/pull/64\",\"state\":\"OPEN\",\"isDraft\":false,\"headRefOid\":\"abc123\",\"headRefName\":\"%s\",\"baseRefName\":\"main\",\"author\":{\"login\":\"bot\"},\"latestReviews\":[],\"statusCheckRollup\":[]}'",
		"  exit 0",
		"fi",
		"echo \"unexpected gh args: $@\" >&2",
		"exit 1",
	}, "\n"), branchName, branchName, branchName))

	unready, err := svc.PRUnready(cr.ID)
	if err != nil {
		t.Fatalf("PRUnready() error = %v", err)
	}
	if !unready.Draft {
		t.Fatalf("expected unready result to be draft")
	}

	closed, err := svc.PRClose(cr.ID)
	if err != nil {
		t.Fatalf("PRClose() error = %v", err)
	}
	if got := strings.ToUpper(strings.TrimSpace(closed.State)); got != "CLOSED" {
		t.Fatalf("expected closed state, got %q", got)
	}
	if closed.ActionRequired != prActionReopenPR {
		t.Fatalf("expected closed action_required=%q, got %q", prActionReopenPR, closed.ActionRequired)
	}

	reopened, err := svc.PRReopen(cr.ID)
	if err != nil {
		t.Fatalf("PRReopen() error = %v", err)
	}
	if got := strings.ToUpper(strings.TrimSpace(reopened.State)); got != "OPEN" {
		t.Fatalf("expected reopened state OPEN, got %q", got)
	}

	reloaded, err := svc.store.LoadCR(cr.ID)
	if err != nil {
		t.Fatalf("LoadCR() after lifecycle ops error = %v", err)
	}
	if strings.ToUpper(strings.TrimSpace(reloaded.PR.State)) != "OPEN" {
		t.Fatalf("expected stored PR state OPEN after reopen, got %q", reloaded.PR.State)
	}
	eventTypes := map[string]int{}
	for _, event := range reloaded.Events {
		eventTypes[event.Type]++
	}
	if eventTypes[model.EventTypeCRReconciled] < 3 {
		t.Fatalf("expected at least three CRReconciled lifecycle events, got %#v", eventTypes)
	}
}

func TestPRReconcileReopenRunsProviderReopen(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SOPHIA.yaml"), []byte("version: v1\nmerge:\n  mode: pr_gate\narchive:\n  enabled: false\n"), 0o644); err != nil {
		t.Fatalf("write SOPHIA.yaml: %v", err)
	}
	cr, err := svc.AddCR("PR reconcile reopen", "status path")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	loaded, err := svc.store.LoadCR(cr.ID)
	if err != nil {
		t.Fatalf("LoadCR() error = %v", err)
	}
	loaded.PR.Number = 55
	loaded.PR.Repo = "acme/repo"
	if err := svc.store.SaveCR(loaded); err != nil {
		t.Fatalf("SaveCR() error = %v", err)
	}
	installFakeGHCommand(t, "#!/bin/sh\nif [ \"$1\" = \"-R\" ]; then\n  shift\n  shift\nfi\nif [ \"$1\" = \"pr\" ] && [ \"$2\" = \"reopen\" ]; then\n  exit 0\nfi\nif [ \"$1\" = \"pr\" ] && [ \"$2\" = \"view\" ]; then\n  echo '{\"number\":55,\"url\":\"https://github.com/acme/repo/pull/55\",\"state\":\"OPEN\",\"isDraft\":false,\"headRefOid\":\"abc123\",\"headRefName\":\"cr-1-pr-reconcile-reopen\",\"baseRefName\":\"main\",\"author\":{\"login\":\"bot\"},\"latestReviews\":[],\"statusCheckRollup\":[]}'\n  exit 0\nfi\necho \"unexpected gh args: $@\" >&2\nexit 1\n")

	view, err := svc.PRReconcile(cr.ID, prReconcileModeReopen)
	if err != nil {
		t.Fatalf("PRReconcile(reopen) error = %v", err)
	}
	if view.Action != "reopened" {
		t.Fatalf("expected action reopened, got %q", view.Action)
	}

	reloaded, err := svc.store.LoadCR(cr.ID)
	if err != nil {
		t.Fatalf("LoadCR() after reconcile error = %v", err)
	}
	if strings.ToUpper(strings.TrimSpace(reloaded.PR.State)) != "OPEN" {
		t.Fatalf("expected stored PR state OPEN, got %q", reloaded.PR.State)
	}
}

func TestPRReconcileCreateCreatesDraftPR(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SOPHIA.yaml"), []byte("version: v1\nmerge:\n  mode: pr_gate\narchive:\n  enabled: false\n"), 0o644); err != nil {
		t.Fatalf("write SOPHIA.yaml: %v", err)
	}

	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("seed\n"), 0o644); err != nil {
		t.Fatalf("write README.md: %v", err)
	}
	runGit(t, dir, "add", "README.md")
	runGit(t, dir, "commit", "-m", "seed")
	remoteDir := filepath.Join(t.TempDir(), "origin.git")
	runGit(t, dir, "init", "--bare", remoteDir)
	runGit(t, dir, "remote", "add", "origin", remoteDir)
	runGit(t, dir, "push", "-u", "origin", "main")

	cr, err := svc.AddCR("PR reconcile create", "status path")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	installFakeGHCommand(t, "#!/bin/sh\nif [ \"$1\" = \"-R\" ]; then\n  shift\n  shift\nfi\nif [ \"$1\" = \"pr\" ] && [ \"$2\" = \"create\" ]; then\n  echo 'https://github.com/acme/repo/pull/9'\n  exit 0\nfi\nif [ \"$1\" = \"pr\" ] && [ \"$2\" = \"view\" ]; then\n  echo '{\"number\":9,\"url\":\"https://github.com/acme/repo/pull/9\",\"state\":\"OPEN\",\"isDraft\":true,\"headRefOid\":\"abc123\",\"headRefName\":\"cr-1-pr-reconcile-create\",\"baseRefName\":\"main\",\"author\":{\"login\":\"bot\"},\"latestReviews\":[],\"statusCheckRollup\":[]}'\n  exit 0\nfi\necho \"unexpected gh args: $@\" >&2\nexit 1\n")

	view, err := svc.PRReconcile(cr.ID, prReconcileModeCreate)
	if err != nil {
		t.Fatalf("PRReconcile(create) error = %v", err)
	}
	if view.Action != "created" || view.AfterPRNumber != 9 {
		t.Fatalf("expected created action with PR #9, got action=%q number=%d", view.Action, view.AfterPRNumber)
	}
}

func installFakeGHCommand(t *testing.T, body string) {
	t.Helper()
	binDir := t.TempDir()
	ghPath := filepath.Join(binDir, "gh")
	script := body
	if !strings.HasPrefix(script, "#!/bin/sh") {
		script = "#!/bin/sh\n" + script
	}
	if err := os.WriteFile(ghPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake gh: %v", err)
	}
	currentPath := os.Getenv("PATH")
	if strings.TrimSpace(currentPath) == "" {
		t.Setenv("PATH", binDir)
		return
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+currentPath)
}

func TestStageArchiveForPRGateSkipsWhenArchiveDisabled(t *testing.T) {
	t.Parallel()
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

func TestStageArchiveForPRGateWritesV2ArchiveWhenFullDiffEnabled(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := New(dir)
	mergeGit := newFakeMergeGit("Test User <test@example.com>", "cr-61-test")
	mergeGit.resolve["main"] = "base123"
	mergeGit.resolve["cr-61-test"] = "head123"
	mergeGit.diffCached = []gitx.FileChange{
		{Path: "internal/service/archive.go"},
	}
	mergeGit.diffNumStat = []gitx.DiffNumStat{
		{Path: "internal/service/archive.go", Insertions: intPtr(3), Deletions: intPtr(1)},
	}
	mergeGit.diffPatch = "diff --git a/internal/service/archive.go b/internal/service/archive.go\nindex 1111111..2222222 100644\n--- a/internal/service/archive.go\n+++ b/internal/service/archive.go\n@@ -1 +1 @@\n-old\n+new\n"
	svc.overrideMergeRuntimeProvidersForTests(mergeGit, nil, nil)

	policy := &model.RepoPolicy{
		Merge: model.PolicyMerge{
			Mode: "pr_gate",
		},
		Archive: model.PolicyArchive{
			Enabled:          boolPtr(true),
			Path:             ".sophia-tracked/cr",
			Format:           "yaml",
			IncludeFullDiffs: boolPtr(true),
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
	archivePath := filepath.Join(dir, ".sophia-tracked", "cr", "cr-61.v1.yaml")
	content, err := os.ReadFile(archivePath)
	if err != nil {
		t.Fatalf("read archive path %s: %v", archivePath, err)
	}
	body := string(content)
	if !strings.Contains(body, "schema_version: sophia.cr_archive.v2") {
		t.Fatalf("expected archive v2 schema in staged archive:\n%s", body)
	}
	if !strings.Contains(body, "full_diff:") {
		t.Fatalf("expected full_diff payload in staged archive:\n%s", body)
	}
	if !strings.Contains(body, "encoding: git_unified_patch") {
		t.Fatalf("expected full diff encoding in staged archive:\n%s", body)
	}
	if mergeGit.Calls("Commit") != 1 {
		t.Fatalf("expected one archive staging commit, got %d", mergeGit.Calls("Commit"))
	}
}

func TestStageArchiveForPRGateFullDiffGuardrailBlocksCommit(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := New(dir)
	mergeGit := newFakeMergeGit("Test User <test@example.com>", "cr-62-test")
	mergeGit.resolve["main"] = "base123"
	mergeGit.resolve["cr-62-test"] = "head123"
	mergeGit.diffCached = []gitx.FileChange{
		{Path: "oversize.txt"},
	}
	mergeGit.diffNumStat = []gitx.DiffNumStat{
		{Path: "oversize.txt", Insertions: intPtr(1), Deletions: intPtr(0)},
	}
	mergeGit.diffPatch = strings.Repeat("x", archiveFullDiffMaxBytes+1024)
	svc.overrideMergeRuntimeProvidersForTests(mergeGit, nil, nil)

	policy := &model.RepoPolicy{
		Merge: model.PolicyMerge{
			Mode: "pr_gate",
		},
		Archive: model.PolicyArchive{
			Enabled:          boolPtr(true),
			Path:             ".sophia-tracked/cr",
			Format:           "yaml",
			IncludeFullDiffs: boolPtr(true),
		},
	}
	cr := &model.CR{
		ID:         62,
		BaseBranch: "main",
		Branch:     "cr-62-test",
	}
	err := svc.stageArchiveForPRGate(cr, policy)
	if err == nil {
		t.Fatalf("expected guardrail error")
	}
	if !strings.Contains(err.Error(), "archive full diff exceeds byte limit") {
		t.Fatalf("expected deterministic guardrail error, got %v", err)
	}
	if mergeGit.Calls("StagePaths") != 0 {
		t.Fatalf("expected no staged paths on guardrail failure, got %d", mergeGit.Calls("StagePaths"))
	}
	if mergeGit.Calls("Commit") != 0 {
		t.Fatalf("expected no commit on guardrail failure, got %d", mergeGit.Calls("Commit"))
	}
	archivePath := filepath.Join(dir, ".sophia-tracked", "cr", "cr-62.v1.yaml")
	if _, statErr := os.Stat(archivePath); !os.IsNotExist(statErr) {
		t.Fatalf("expected no archive file on guardrail failure, stat err=%v", statErr)
	}
}

func TestStageArchiveForPRGateBranchOwnerConflictIncludesDetails(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := New(dir)
	mergeGit := newFakeMergeGit("Test User <test@example.com>", "main")
	mergeGit.worktrees["cr-63-test"] = &gitx.Worktree{
		Path:   filepath.Join(t.TempDir(), "wt-owner"),
		Branch: "cr-63-test",
	}
	svc.overrideMergeRuntimeProvidersForTests(mergeGit, nil, func(root string) mergeRuntimeGit {
		return mergeGit
	})

	policy := &model.RepoPolicy{
		Merge: model.PolicyMerge{
			Mode: "pr_gate",
		},
		Archive: model.PolicyArchive{
			Enabled: boolPtr(true),
			Path:    ".sophia-tracked/cr",
			Format:  "yaml",
		},
	}
	cr := &model.CR{
		ID:         63,
		BaseBranch: "main",
		Branch:     "cr-63-test",
	}
	err := svc.stageArchiveForPRGate(cr, policy)
	if err == nil {
		t.Fatalf("expected branch ownership conflict error")
	}
	if !errors.Is(err, ErrBranchInOtherWorktree) {
		t.Fatalf("expected ErrBranchInOtherWorktree, got %v", err)
	}
	var details *BranchInOtherWorktreeError
	if !errors.As(err, &details) {
		t.Fatalf("expected BranchInOtherWorktreeError details, got %T", err)
	}
	if details.CRID != cr.ID {
		t.Fatalf("expected cr id %d, got %d", cr.ID, details.CRID)
	}
	if details.Operation != "pr_stage_archive" {
		t.Fatalf("expected operation pr_stage_archive, got %q", details.Operation)
	}
	if details.OwnerWorktreePath != mergeGit.worktrees["cr-63-test"].Path {
		t.Fatalf("expected owner worktree path %q, got %q", mergeGit.worktrees["cr-63-test"].Path, details.OwnerWorktreePath)
	}
	if !strings.Contains(details.SuggestedCommand, "sophia cr pr sync 63") {
		t.Fatalf("expected suggested command to target cr pr sync 63, got %q", details.SuggestedCommand)
	}
}

func TestPushBranchIfNeededPushesLocalAheadCommit(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	runGit(t, dir, "init", "-b", "main")
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("seed\n"), 0o644); err != nil {
		t.Fatalf("write README: %v", err)
	}
	runGit(t, dir, "add", "README.md")
	runGit(t, dir, "commit", "-m", "seed")

	remoteDir := filepath.Join(t.TempDir(), "origin.git")
	runGit(t, dir, "init", "--bare", remoteDir)
	runGit(t, dir, "remote", "add", "origin", remoteDir)
	runGit(t, dir, "push", "-u", "origin", "main")

	branch := "feature/push-sync"
	runGit(t, dir, "checkout", "-b", branch)
	if err := os.WriteFile(filepath.Join(dir, "feature.txt"), []byte("one\n"), 0o644); err != nil {
		t.Fatalf("write feature.txt: %v", err)
	}
	runGit(t, dir, "add", "feature.txt")
	runGit(t, dir, "commit", "-m", "feat: seed feature")
	runGit(t, dir, "push", "-u", "origin", branch)

	if err := os.WriteFile(filepath.Join(dir, "feature.txt"), []byte("one\ntwo\n"), 0o644); err != nil {
		t.Fatalf("update feature.txt: %v", err)
	}
	runGit(t, dir, "add", "feature.txt")
	runGit(t, dir, "commit", "-m", "feat: local ahead commit")

	svc := New(dir)
	if err := svc.pushBranchIfNeeded(&model.CR{Branch: branch}); err != nil {
		t.Fatalf("pushBranchIfNeeded() error = %v", err)
	}

	localHead := runGit(t, dir, "rev-parse", branch)
	remoteHeadLine := runGit(t, dir, "ls-remote", "--heads", "origin", branch)
	parts := strings.Fields(remoteHeadLine)
	if len(parts) == 0 {
		t.Fatalf("expected remote head output, got %q", remoteHeadLine)
	}
	if parts[0] != localHead {
		t.Fatalf("expected remote head %s to match local head %s", parts[0], localHead)
	}
}

func TestResolveCommitOIDExpandsShortSHA(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	runGit(t, dir, "init", "-b", "main")
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("seed\n"), 0o644); err != nil {
		t.Fatalf("write README: %v", err)
	}
	runGit(t, dir, "add", "README.md")
	runGit(t, dir, "commit", "-m", "seed")

	full := runGit(t, dir, "rev-parse", "HEAD")
	short := runGit(t, dir, "rev-parse", "--short", "HEAD")
	if len(short) >= len(full) {
		t.Fatalf("expected short SHA, got short=%q full=%q", short, full)
	}

	svc := New(dir)
	got, err := svc.resolveCommitOID(short)
	if err != nil {
		t.Fatalf("resolveCommitOID() error = %v", err)
	}
	if got != full {
		t.Fatalf("expected full SHA %q, got %q", full, got)
	}
}
