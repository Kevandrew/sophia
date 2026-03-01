package gitx

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// tparallel:serial-exception
// This file mutates package-level retry hooks and must remain serial.
func TestRunRetriesIndexLockAndSucceeds(t *testing.T) {
	repo := t.TempDir()
	runGit(t, repo, "init", "-b", "main")
	if err := os.WriteFile(filepath.Join(repo, "f.txt"), []byte("x\n"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	runGit(t, repo, "add", "f.txt")
	runGit(t, repo, "commit", "-m", "feat: fixture")

	lockPath := filepath.Join(repo, ".git", "index.lock")
	if err := os.WriteFile(lockPath, []byte("lock"), 0o644); err != nil {
		t.Fatalf("write index.lock: %v", err)
	}

	origBackoff := indexLockRetryBackoff
	indexLockRetryBackoff = []time.Duration{10 * time.Millisecond, 20 * time.Millisecond, 40 * time.Millisecond}
	defer func() {
		indexLockRetryBackoff = origBackoff
	}()

	go func() {
		time.Sleep(25 * time.Millisecond)
		_ = os.Remove(lockPath)
	}()

	client := New(repo)
	if _, err := client.run("add", "f.txt"); err != nil {
		t.Fatalf("expected add to succeed after index.lock retry, got %v", err)
	}
}

func TestRunReturnsActionableIndexLockErrorAfterBoundedRetries(t *testing.T) {
	repo := t.TempDir()
	runGit(t, repo, "init", "-b", "main")
	if err := os.WriteFile(filepath.Join(repo, "f.txt"), []byte("x\n"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	lockPath := filepath.Join(repo, ".git", "index.lock")
	if err := os.WriteFile(lockPath, []byte("lock"), 0o644); err != nil {
		t.Fatalf("write index.lock: %v", err)
	}

	origBackoff := indexLockRetryBackoff
	indexLockRetryBackoff = []time.Duration{1 * time.Millisecond, 1 * time.Millisecond}
	defer func() {
		indexLockRetryBackoff = origBackoff
	}()

	client := New(repo)
	_, err := client.run("add", "f.txt")
	if !errors.Is(err, ErrIndexLock) {
		t.Fatalf("expected ErrIndexLock, got %v", err)
	}
	if !strings.Contains(strings.ToLower(err.Error()), "retry") {
		t.Fatalf("expected actionable retry guidance, got %v", err)
	}
}

func TestRunDoesNotRetryNonLockFailures(t *testing.T) {
	repo := t.TempDir()
	runGit(t, repo, "init", "-b", "main")
	client := New(repo)

	origBackoff := indexLockRetryBackoff
	origSleep := sleepForIndexLockRetry
	indexLockRetryBackoff = []time.Duration{time.Second, time.Second}
	sleepCalls := 0
	sleepForIndexLockRetry = func(_ time.Duration) {
		sleepCalls++
	}
	defer func() {
		indexLockRetryBackoff = origBackoff
		sleepForIndexLockRetry = origSleep
	}()

	_, err := client.run("definitely-not-a-git-subcommand")
	if err == nil {
		t.Fatalf("expected non-lock git failure")
	}
	if errors.Is(err, ErrIndexLock) {
		t.Fatalf("did not expect ErrIndexLock for non-lock failure, got %v", err)
	}
	if sleepCalls != 0 {
		t.Fatalf("expected non-lock failure to skip retries, observed %d sleeps", sleepCalls)
	}
}
