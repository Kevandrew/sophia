package cli

import (
	"strings"
	"testing"

	"sophia/internal/service"
)

func TestReadCommandsResolveByExplicitCRIDOffBranch(t *testing.T) {
	dir := t.TempDir()
	svc := service.New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	_, _, err := svc.AddCRWithOptionsWithWarnings("Read context", "read by id off branch", service.AddCROptions{NoSwitch: true})
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}

	cases := [][]string{
		{"cr", "status", "1", "--json"},
		{"cr", "diff", "1", "--json"},
		{"cr", "impact", "1", "--json"},
		{"cr", "review", "1", "--json"},
	}
	for _, args := range cases {
		out, _, runErr := runCLI(t, dir, args...)
		if runErr != nil {
			t.Fatalf("%q error = %v\noutput=%s", strings.Join(args, " "), runErr, out)
		}
		env := decodeEnvelope(t, out)
		if !env.OK {
			t.Fatalf("%q expected ok envelope, got %#v", strings.Join(args, " "), env)
		}
	}
}
