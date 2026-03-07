package cli

import (
	"strings"
	"testing"
)

func TestRootHelpStartPath(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	out, _, err := runCLI(t, dir, "--help")
	if err != nil {
		t.Fatalf("root --help error = %v\noutput=%s", err, out)
	}
	assertHelpContains(t, out,
		"Start Here:",
		"sophia cr add \"<title>\" --description \"<why>\"",
		"sophia cr switch <cr-id>",
		"sophia cr review <cr-id>",
		"sophia cr merge <cr-id>",
		"Optional explicit setup:",
		"sophia init",
	)
}

func TestCRHelpNavigationMap(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	out, _, err := runCLI(t, dir, "cr", "--help")
	if err != nil {
		t.Fatalf("cr --help error = %v\noutput=%s", err, out)
	}
	assertHelpContains(t, out,
		"Change-request commands grouped by intent:",
		"Navigation:",
		"Intake and planning:",
		"Implementation lenses:",
		"range, rev-parse, pack",
		"Merge and recovery:",
		"refresh",
		"sophia cr add \"Worktree-safe parsing\"",
	)
}

func TestMergeAndTaskDoneHelpExamples(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	mergeOut, _, mergeErr := runCLI(t, dir, "cr", "merge", "--help")
	if mergeErr != nil {
		t.Fatalf("cr merge --help error = %v\noutput=%s", mergeErr, mergeOut)
	}
	assertHelpContains(t, mergeOut,
		"sophia cr merge [id]",
		"sophia cr merge status 25",
		"sophia cr merge resume 25",
		"sophia cr merge abort 25",
	)

	doneOut, _, doneErr := runCLI(t, dir, "cr", "task", "done", "--help")
	if doneErr != nil {
		t.Fatalf("cr task done --help error = %v\noutput=%s", doneErr, doneOut)
	}
	assertHelpContains(t, doneOut,
		"sophia cr task done [<cr-id>] <task-id>",
		"sophia cr task done 25 1 --commit-type fix --from-contract",
		"sophia cr task done 25 1 --patch-file /tmp/task1.patch",
		"sophia cr task done 25 1 --no-checkpoint --no-checkpoint-reason \"metadata-only task\"",
	)

	contractSetOut, _, contractSetErr := runCLI(t, dir, "cr", "task", "contract", "set", "--help")
	if contractSetErr != nil {
		t.Fatalf("cr task contract set --help error = %v\noutput=%s", contractSetErr, contractSetOut)
	}
	assertHelpContains(t, contractSetOut,
		"sophia cr task contract set [<cr-id>] <task-id>",
	)

	refreshOut, _, refreshErr := runCLI(t, dir, "cr", "refresh", "--help")
	if refreshErr != nil {
		t.Fatalf("cr refresh --help error = %v\noutput=%s", refreshErr, refreshOut)
	}
	assertHelpContains(t, refreshOut,
		"parent CR also refreshes descendant child CRs in stack order by default",
		"Refreshing a child CR remains local to that child.",
	)
}

func TestLifecycleLeafHelpExplainsPurposeAndNextStep(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	addOut, _, addErr := runCLI(t, dir, "cr", "add", "--help")
	if addErr != nil {
		t.Fatalf("cr add --help error = %v\noutput=%s", addErr, addOut)
	}
	assertHelpContains(t, addOut,
		"Open a new CR intent.",
		"then switch to the CR branch",
		"sophia cr add \"Add retry jitter\" --description \"Reduce synchronized retries\" --switch",
	)

	contractOut, _, contractErr := runCLI(t, dir, "cr", "contract", "set", "--help")
	if contractErr != nil {
		t.Fatalf("cr contract set --help error = %v\noutput=%s", contractErr, contractOut)
	}
	assertHelpContains(t, contractOut,
		"Use this immediately after opening a CR",
		"test plan, and rollback plan explicit",
	)

	statusOut, _, statusErr := runCLI(t, dir, "cr", "status", "--help")
	if statusErr != nil {
		t.Fatalf("cr status --help error = %v\noutput=%s", statusErr, statusOut)
	}
	assertHelpContains(t, statusOut,
		"Use this before mutating, refreshing, or merging an existing CR.",
		"sophia cr status 25 --json",
	)

	taskAddOut, _, taskAddErr := runCLI(t, dir, "cr", "task", "add", "--help")
	if taskAddErr != nil {
		t.Fatalf("cr task add --help error = %v\noutput=%s", taskAddErr, taskAddOut)
	}
	assertHelpContains(t, taskAddOut,
		"Add a checkpoint-sized subtask",
		"sophia cr task add 25 \"Implement bounded jitter strategy\"",
	)

	taskContractOut, _, taskContractErr := runCLI(t, dir, "cr", "task", "contract", "set", "--help")
	if taskContractErr != nil {
		t.Fatalf("cr task contract set --help error = %v\noutput=%s", taskContractErr, taskContractOut)
	}
	assertHelpContains(t, taskContractOut,
		"Use this before `task done`.",
		"scope narrow enough for a single checkpoint",
		"sophia cr task contract set 25 1 --intent \"Bound retry jitter\"",
	)

	prOpenOut, _, prOpenErr := runCLI(t, dir, "cr", "pr", "open", "--help")
	if prOpenErr != nil {
		t.Fatalf("cr pr open --help error = %v\noutput=%s", prOpenErr, prOpenOut)
	}
	assertHelpContains(t, prOpenOut,
		"Use this after local implementation, validation, and review",
		"sophia cr pr open 25 --approve-open",
	)

	prReadyOut, _, prReadyErr := runCLI(t, dir, "cr", "pr", "ready", "--help")
	if prReadyErr != nil {
		t.Fatalf("cr pr ready --help error = %v\noutput=%s", prReadyErr, prReadyOut)
	}
	assertHelpContains(t, prReadyOut,
		"Use this only for explicit reviewer handoff",
		"Keep the PR in draft",
		"sophia cr pr ready 25 --json",
	)
}

func assertHelpContains(t *testing.T, out string, patterns ...string) {
	t.Helper()
	for _, pattern := range patterns {
		if !strings.Contains(out, pattern) {
			t.Fatalf("help output missing %q\noutput=%s", pattern, out)
		}
	}
}
