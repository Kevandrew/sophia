package tasks

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNormalizeScopePathsAcceptsRepoRelativeFiles(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	file := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	paths, err := NormalizeScopePaths(dir, []string{"a.txt"})
	if err != nil {
		t.Fatalf("NormalizeScopePaths error: %v", err)
	}
	if len(paths) != 1 || paths[0] != "a.txt" {
		t.Fatalf("paths = %#v, want [a.txt]", paths)
	}
}

func TestNormalizeScopePathsRejectsDirectories(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if _, err := NormalizeScopePaths(dir, []string{"sub"}); err == nil {
		t.Fatalf("expected directory rejection")
	}
}

func TestNormalizePatchFilePathRejectsMissing(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if _, err := NormalizePatchFilePath(dir, "missing.patch"); err == nil {
		t.Fatalf("expected missing file error")
	}
}
