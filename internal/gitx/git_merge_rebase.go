package gitx

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
)

func (c *Client) RebaseBranchOnto(branch, ontoRef string) error {
	if err := c.CheckoutBranch(branch); err != nil {
		return err
	}
	return c.RebaseCurrentBranchOnto(ontoRef)
}

func (c *Client) RebaseCurrentBranchOnto(ontoRef string) error {
	_, err := c.run("rebase", ontoRef)
	return err
}

func (c *Client) IsMergeInProgress() (bool, error) {
	cmd := exec.Command("git", "rev-parse", "-q", "--verify", "MERGE_HEAD")
	cmd.Dir = c.WorkDir
	raw, err := cmd.CombinedOutput()
	if err == nil {
		return strings.TrimSpace(string(raw)) != "", nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
		return false, nil
	}
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" {
		return false, fmt.Errorf("git rev-parse -q --verify MERGE_HEAD: %w", err)
	}
	return false, fmt.Errorf("git rev-parse -q --verify MERGE_HEAD: %w: %s", err, trimmed)
}

func (c *Client) MergeHeadSHA() (string, error) {
	cmd := exec.Command("git", "rev-parse", "-q", "--verify", "MERGE_HEAD")
	cmd.Dir = c.WorkDir
	raw, err := cmd.CombinedOutput()
	if err == nil {
		return strings.TrimSpace(string(raw)), nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
		return "", nil
	}
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" {
		return "", fmt.Errorf("git rev-parse -q --verify MERGE_HEAD: %w", err)
	}
	return "", fmt.Errorf("git rev-parse -q --verify MERGE_HEAD: %w: %s", err, trimmed)
}

func (c *Client) MergeConflictFiles() ([]string, error) {
	entries, err := c.WorkingTreeStatus()
	if err != nil {
		return nil, err
	}
	unmergedCodes := map[string]struct{}{
		"UU": {},
		"AA": {},
		"DD": {},
		"AU": {},
		"UA": {},
		"DU": {},
		"UD": {},
	}
	seen := map[string]struct{}{}
	files := make([]string, 0)
	for _, entry := range entries {
		code := strings.TrimSpace(entry.Code)
		if _, ok := unmergedCodes[code]; !ok {
			continue
		}
		path := strings.TrimSpace(entry.Path)
		if path == "" {
			continue
		}
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}
		files = append(files, path)
	}
	sort.Strings(files)
	return files, nil
}

func (c *Client) MergeAbort() error {
	_, err := c.run("merge", "--abort")
	return err
}

func (c *Client) MergeContinue() error {
	cmd := exec.Command("git", "merge", "--continue")
	cmd.Dir = c.WorkDir
	cmd.Env = append(os.Environ(), "GIT_EDITOR=true")
	raw, err := cmd.CombinedOutput()
	trimmed := strings.TrimSpace(string(raw))
	if err != nil {
		if trimmed == "" {
			return fmt.Errorf("git merge --continue: %w", err)
		}
		return fmt.Errorf("git merge --continue: %w: %s", err, trimmed)
	}
	return nil
}

func (c *Client) MergeBase(baseBranch, branch string) (string, error) {
	out, err := c.run("merge-base", baseBranch, branch)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

func (c *Client) IsAncestor(ancestor, descendant string) (bool, error) {
	ancestor = strings.TrimSpace(ancestor)
	descendant = strings.TrimSpace(descendant)
	if ancestor == "" || descendant == "" {
		return false, fmt.Errorf("ancestor and descendant refs are required")
	}
	cmd := exec.Command("git", "merge-base", "--is-ancestor", ancestor, descendant)
	cmd.Dir = c.WorkDir
	raw, err := cmd.CombinedOutput()
	if err == nil {
		return true, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
		return false, nil
	}
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" {
		return false, fmt.Errorf("git merge-base --is-ancestor %s %s: %w", ancestor, descendant, err)
	}
	return false, fmt.Errorf("git merge-base --is-ancestor %s %s: %w: %s", ancestor, descendant, err, trimmed)
}

func (c *Client) MergeNoFF(baseBranch, branch, message string) error {
	if err := c.CheckoutBranch(baseBranch); err != nil {
		return err
	}
	return c.MergeNoFFOnCurrentBranch(branch, message)
}

func (c *Client) MergeNoFFOnCurrentBranch(branch, message string) error {
	args := c.identityFlags()
	args = append(args, "merge", "--no-ff", branch, "-m", message)
	_, err := c.run(args...)
	return err
}

func (c *Client) MergeNoFFNoCommitOnCurrentBranch(branch, message string) error {
	args := c.identityFlags()
	args = append(args, "merge", "--no-ff", "--no-commit", branch, "-m", message)
	_, err := c.run(args...)
	return err
}

func (c *Client) CommitParents(ref string) ([]string, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return nil, fmt.Errorf("ref is required")
	}
	out, err := c.run("rev-list", "--parents", "-n", "1", ref)
	if err != nil {
		return nil, err
	}
	fields := strings.Fields(strings.TrimSpace(out))
	if len(fields) == 0 {
		return []string{}, nil
	}
	parents := make([]string, 0, len(fields)-1)
	for _, parent := range fields[1:] {
		parent = strings.TrimSpace(parent)
		if parent == "" {
			continue
		}
		parents = append(parents, parent)
	}
	return parents, nil
}

func (c *Client) ResetSoft(target string) error {
	_, err := c.run("reset", "--soft", target)
	return err
}

func (c *Client) MergeFFOnly(baseBranch, branch string) error {
	if err := c.CheckoutBranch(baseBranch); err != nil {
		return err
	}
	_, err := c.run("merge", "--ff-only", branch)
	return err
}

func (c *Client) SquashMerge(baseBranch, branch, message string) error {
	if err := c.CheckoutBranch(baseBranch); err != nil {
		return err
	}
	if _, err := c.run("merge", "--squash", branch); err != nil {
		return err
	}
	args := c.identityFlags()
	args = append(args, "commit", "-m", message)
	_, err := c.run(args...)
	return err
}
