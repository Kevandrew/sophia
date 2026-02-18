package cli

import (
	"os"
	"path/filepath"
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
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")

	_, err := svc.AddCR("Read context", "read by id off branch")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "read.txt"), []byte("one\n"), 0o644); err != nil {
		t.Fatalf("write read.txt: %v", err)
	}
	runGit(t, dir, "add", "read.txt")
	runGit(t, dir, "commit", "-m", "feat: read fixture")
	runGit(t, dir, "checkout", "main")

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
