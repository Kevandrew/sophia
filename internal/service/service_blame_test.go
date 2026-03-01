package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBlameFileUsesSophiaFooterIntent(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")

	if err := os.WriteFile(filepath.Join(dir, "footer.txt"), []byte("footer\n"), 0o644); err != nil {
		t.Fatalf("write footer file: %v", err)
	}
	runGit(t, dir, "add", "footer.txt")
	runGit(t, dir, "commit", "-m", "feat: footer intent", "-m", "Sophia-CR: 42\nSophia-Intent: Footer-owned intent")

	view, err := svc.BlameFile("footer.txt", BlameOptions{})
	if err != nil {
		t.Fatalf("BlameFile() error = %v", err)
	}
	if len(view.Lines) != 1 {
		t.Fatalf("expected one blamed line, got %d", len(view.Lines))
	}
	line := view.Lines[0]
	if !line.HasCR || line.CRID != 42 {
		t.Fatalf("expected CR 42 mapping, got %#v", line)
	}
	if line.Intent != "Footer-owned intent" || line.IntentSource != "sophia_footer" {
		t.Fatalf("expected footer intent/source, got %#v", line)
	}
}

func TestBlameFileFallsBackToCRMetadataIntent(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")

	cr, err := svc.AddCR("Metadata fallback title", "metadata fallback")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "metadata.txt"), []byte("metadata\n"), 0o644); err != nil {
		t.Fatalf("write metadata file: %v", err)
	}
	runGit(t, dir, "add", "metadata.txt")
	runGit(t, dir, "commit", "-m", "chore: metadata fallback", "-m", "Sophia-CR: 1")

	view, err := svc.BlameFile("metadata.txt", BlameOptions{})
	if err != nil {
		t.Fatalf("BlameFile() error = %v", err)
	}
	if len(view.Lines) != 1 {
		t.Fatalf("expected one blamed line, got %d", len(view.Lines))
	}
	line := view.Lines[0]
	if !line.HasCR || line.CRID != cr.ID {
		t.Fatalf("expected CR %d mapping, got %#v", cr.ID, line)
	}
	if line.CRUID != cr.UID {
		t.Fatalf("expected CR uid %q, got %q", cr.UID, line.CRUID)
	}
	if line.Intent != cr.Title || line.IntentSource != "cr_metadata_fallback" {
		t.Fatalf("expected CR metadata fallback intent/source, got %#v", line)
	}
}

func TestBlameFileNonSophiaCommitFallsBackToCommitSummary(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")

	if err := os.WriteFile(filepath.Join(dir, "plain.txt"), []byte("plain\n"), 0o644); err != nil {
		t.Fatalf("write plain file: %v", err)
	}
	runGit(t, dir, "add", "plain.txt")
	runGit(t, dir, "commit", "-m", "docs: plain commit")

	view, err := svc.BlameFile("plain.txt", BlameOptions{})
	if err != nil {
		t.Fatalf("BlameFile() error = %v", err)
	}
	if len(view.Lines) != 1 {
		t.Fatalf("expected one blamed line, got %d", len(view.Lines))
	}
	line := view.Lines[0]
	if line.HasCR {
		t.Fatalf("expected no CR mapping, got %#v", line)
	}
	if line.IntentSource != "commit_summary_fallback" || line.Intent != "docs: plain commit" {
		t.Fatalf("expected commit summary fallback, got %#v", line)
	}
}

func TestBlameFileIncludesUncommittedLines(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")

	path := filepath.Join(dir, "dirty.txt")
	if err := os.WriteFile(path, []byte("a\n"), 0o644); err != nil {
		t.Fatalf("write initial dirty file: %v", err)
	}
	runGit(t, dir, "add", "dirty.txt")
	runGit(t, dir, "commit", "-m", "chore: seed dirty")

	if err := os.WriteFile(path, []byte("a\nb\n"), 0o644); err != nil {
		t.Fatalf("write dirty modifications: %v", err)
	}

	view, err := svc.BlameFile("dirty.txt", BlameOptions{})
	if err != nil {
		t.Fatalf("BlameFile() error = %v", err)
	}
	if len(view.Lines) != 2 {
		t.Fatalf("expected two blamed lines, got %d", len(view.Lines))
	}
	last := view.Lines[1]
	if last.HasCR {
		t.Fatalf("expected no CR mapping for uncommitted line, got %#v", last)
	}
	if !strings.HasPrefix(last.Commit, "0000000") {
		t.Fatalf("expected uncommitted hash prefix, got %#v", last)
	}
}
