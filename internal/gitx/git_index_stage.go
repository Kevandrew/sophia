package gitx

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

var (
	ErrIndexLock = errors.New("git index lock busy")

	indexLockRetryBackoff = []time.Duration{
		50 * time.Millisecond,
		100 * time.Millisecond,
		200 * time.Millisecond,
	}
	sleepForIndexLockRetry = time.Sleep
)

type IndexLockError struct {
	Command     string
	Attempts    int
	LastMessage string
}

func (e IndexLockError) Error() string {
	command := strings.TrimSpace(e.Command)
	if command == "" {
		command = "git <command>"
	}
	attempts := e.Attempts
	if attempts < 1 {
		attempts = 1
	}
	base := fmt.Sprintf("git index.lock blocked command `%s`; retried %d time(s) and still failed", command, attempts)
	if strings.TrimSpace(e.LastMessage) == "" {
		return base + "; wait for other git processes (or clear stale index.lock) and retry"
	}
	return fmt.Sprintf("%s: %s; wait for other git processes (or clear stale index.lock) and retry", base, strings.TrimSpace(e.LastMessage))
}

func (e IndexLockError) Is(target error) bool {
	return target == ErrIndexLock
}

func (c *Client) StageAll() error {
	_, err := c.run("add", "-A")
	return err
}

func (c *Client) StagePaths(paths []string) error {
	if len(paths) == 0 {
		return fmt.Errorf("no paths provided for staging")
	}
	args := []string{"add", "-A", "--"}
	args = append(args, paths...)
	_, err := c.run(args...)
	return err
}

func (c *Client) ApplyPatchToIndex(patchPath string) error {
	_, err := c.run("apply", "--cached", "--unidiff-zero", "--recount", patchPath)
	return err
}

func (c *Client) HasStagedChanges() (bool, error) {
	cmd := exec.Command("git", "diff", "--cached", "--quiet")
	cmd.Dir = c.WorkDir
	err := cmd.Run()
	if err == nil {
		return false, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
		return true, nil
	}
	return false, fmt.Errorf("git diff --cached --quiet: %w", err)
}

func (c *Client) PathHasChanges(path string) (bool, error) {
	out, err := c.run("status", "--porcelain=v1", "--untracked-files=all", "--", path)
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(out) != "", nil
}
