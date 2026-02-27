package cli

import (
	"bytes"
	"context"
	"encoding/json"
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

func runCLI(t *testing.T, dir string, args ...string) (string, string, error) {
	return runCLIWithOptions(t, dir, cliRunOptions{}, args...)
}

type cliRunOptions struct {
	factory serviceFactory
}

func runCLIWithOptions(t *testing.T, dir string, opts cliRunOptions, args ...string) (string, string, error) {
	t.Helper()

	root := newRootCmd()
	ctx := withServiceRepoRootContext(context.Background(), dir)
	if opts.factory != nil {
		ctx = withServiceFactoryContext(ctx, opts.factory)
	}
	root.SetContext(ctx)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	err := executeRootCmd(root, args)
	return stdout.String(), stderr.String(), err
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

type envelope struct {
	OK    bool                  `json:"ok"`
	Data  map[string]any        `json:"data"`
	Error *envelopeErrorPayload `json:"error,omitempty"`
}

type envelopeErrorPayload struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"`
}

func decodeEnvelope(t *testing.T, raw string) envelope {
	t.Helper()
	var env envelope
	if err := json.Unmarshal([]byte(raw), &env); err != nil {
		t.Fatalf("decode envelope: %v\nraw=%s", err, raw)
	}
	return env
}

func requireJSONArrayField(t *testing.T, payload map[string]any, key string) []any {
	t.Helper()
	value, ok := payload[key]
	if !ok {
		t.Fatalf("expected key %q in payload %#v", key, payload)
	}
	items, ok := value.([]any)
	if !ok {
		t.Fatalf("expected %q to be array, got %#v", key, value)
	}
	return items
}
