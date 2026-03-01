package gitx

import "testing"

func TestParseWorktreeListPorcelain(t *testing.T) {
	t.Parallel()
	raw := "worktree /tmp/repo\nHEAD aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\nbranch refs/heads/main\n\n" +
		"worktree /tmp/repo-wt\nHEAD bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb\nbranch refs/heads/sophia/cr-25\n\n" +
		"worktree /tmp/repo-detached\nHEAD cccccccccccccccccccccccccccccccccccccccc\ndetached\n"

	parsed := parseWorktreeListPorcelain("/tmp/repo", raw)
	if len(parsed) != 3 {
		t.Fatalf("expected 3 worktrees, got %#v", parsed)
	}

	byPath := map[string]Worktree{}
	for _, wt := range parsed {
		byPath[wt.Path] = wt
	}
	if byPath["/tmp/repo-wt"].Branch != "sophia/cr-25" {
		t.Fatalf("unexpected parsed branch worktree: %#v", byPath["/tmp/repo-wt"])
	}
	if byPath["/tmp/repo-detached"].Branch != "" {
		t.Fatalf("detached worktree should not have branch value: %#v", byPath["/tmp/repo-detached"])
	}
}
