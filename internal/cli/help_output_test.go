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
}

func assertHelpContains(t *testing.T, out string, patterns ...string) {
	t.Helper()
	for _, pattern := range patterns {
		if !strings.Contains(out, pattern) {
			t.Fatalf("help output missing %q\noutput=%s", pattern, out)
		}
	}
}
