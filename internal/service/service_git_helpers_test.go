package service

import (
	"os/exec"
	"strings"
	"testing"
)

func firstHunkPatchFromDiff(t *testing.T, diff string) string {
	t.Helper()
	diff = strings.TrimSpace(diff)
	if diff == "" {
		t.Fatalf("expected non-empty diff")
	}
	lines := strings.Split(diff, "\n")
	out := make([]string, 0, len(lines))
	hunks := 0
	for _, line := range lines {
		if strings.HasPrefix(line, "@@ ") {
			hunks++
			if hunks > 1 {
				break
			}
		}
		out = append(out, line)
	}
	if hunks == 0 {
		t.Fatalf("expected at least one hunk in diff: %q", diff)
	}
	return strings.Join(out, "\n") + "\n"
}

func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, string(out))
	}
	return strings.TrimSpace(string(out))
}
