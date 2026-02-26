package cli

import (
	"bytes"
	"context"
	"io"
	"os"
	"strings"
	"testing"
)

func runVersionWithBufferStdout(t *testing.T, args ...string) (string, error) {
	t.Helper()
	root := newRootCmd()
	root.SetContext(withServiceRepoRootContext(context.Background(), t.TempDir()))
	var out bytes.Buffer
	var errOut bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errOut)
	err := executeRootCmd(root, append([]string{"version"}, args...))
	return out.String(), err
}

func runVersionWithPipeStdout(t *testing.T, args ...string) (string, error) {
	t.Helper()
	root := newRootCmd()
	root.SetContext(withServiceRepoRootContext(context.Background(), t.TempDir()))
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe() error = %v", err)
	}
	var errOut bytes.Buffer
	root.SetOut(writer)
	root.SetErr(&errOut)
	runErr := executeRootCmd(root, append([]string{"version"}, args...))
	_ = writer.Close()
	output, readErr := io.ReadAll(reader)
	_ = reader.Close()
	if readErr != nil {
		t.Fatalf("ReadAll(pipe) error = %v", readErr)
	}
	return string(output), runErr
}

func TestOutputModeSOPHIAOutputJSON(t *testing.T) {
	t.Setenv(sophiaOutputModeEnv, "json")
	out, runErr := runVersionWithBufferStdout(t)
	if runErr != nil {
		t.Fatalf("version run error = %v\noutput=%s", runErr, out)
	}
	env := decodeEnvelope(t, out)
	if !env.OK {
		t.Fatalf("expected ok envelope, got %#v", env)
	}
}

func TestOutputModeSOPHIAOutputTextOverridesNoTTY(t *testing.T) {
	t.Setenv(sophiaOutputModeEnv, "text")
	out, runErr := runVersionWithPipeStdout(t)
	if runErr != nil {
		t.Fatalf("version run error = %v\noutput=%s", runErr, out)
	}
	if strings.Contains(out, `"ok"`) {
		t.Fatalf("expected text output when SOPHIA_OUTPUT=text, got %q", out)
	}
	if !strings.Contains(out, "version:") {
		t.Fatalf("expected text version output, got %q", out)
	}
}

func TestOutputModeUnsetWithTTYDefaultsText(t *testing.T) {
	t.Setenv(sophiaOutputModeEnv, "")
	out, runErr := runVersionWithBufferStdout(t)
	if runErr != nil {
		t.Fatalf("version run error = %v\noutput=%s", runErr, out)
	}
	if strings.Contains(out, `"ok"`) {
		t.Fatalf("expected text output for default TTY mode, got %q", out)
	}
	if !strings.Contains(out, "version:") {
		t.Fatalf("expected text version output, got %q", out)
	}
}

func TestOutputModeUnsetWithNoTTYDefaultsJSON(t *testing.T) {
	t.Setenv(sophiaOutputModeEnv, "")
	out, runErr := runVersionWithPipeStdout(t)
	if runErr != nil {
		t.Fatalf("version run error = %v\noutput=%s", runErr, out)
	}
	env := decodeEnvelope(t, out)
	if !env.OK {
		t.Fatalf("expected ok envelope for no-TTY default mode, got %#v", env)
	}
}

func TestOutputModeExplicitJSONOverridesEnvText(t *testing.T) {
	t.Setenv(sophiaOutputModeEnv, "text")
	out, runErr := runVersionWithBufferStdout(t, "--json")
	if runErr != nil {
		t.Fatalf("version --json run error = %v\noutput=%s", runErr, out)
	}
	env := decodeEnvelope(t, out)
	if !env.OK {
		t.Fatalf("expected ok envelope, got %#v", env)
	}
}

func TestOutputModeExplicitJSONFalseOverridesEnvJSON(t *testing.T) {
	t.Setenv(sophiaOutputModeEnv, "json")
	out, runErr := runVersionWithBufferStdout(t, "--json=false")
	if runErr != nil {
		t.Fatalf("version --json=false run error = %v\noutput=%s", runErr, out)
	}
	if strings.Contains(out, `"ok"`) {
		t.Fatalf("expected text output when --json=false is explicit, got %q", out)
	}
	if !strings.Contains(out, "version:") {
		t.Fatalf("expected text version output, got %q", out)
	}
}
