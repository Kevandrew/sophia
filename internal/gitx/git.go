package gitx

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

const sophiaManagedHookMarker = "SOPHIA_MANAGED_PRE_COMMIT"

type Client struct {
	WorkDir string
}

type FileChange struct {
	Status  string
	OldPath string
	Path    string
}

type StatusEntry struct {
	Code string
	Path string
}

type Commit struct {
	Hash    string
	Author  string
	When    string
	Subject string
	Body    string
}

func New(workDir string) *Client {
	return &Client{WorkDir: workDir}
}

func (c *Client) InRepo() bool {
	out, err := c.run("rev-parse", "--is-inside-work-tree")
	return err == nil && strings.TrimSpace(out) == "true"
}

func (c *Client) InitRepo() error {
	_, err := c.run("init")
	return err
}

func (c *Client) CurrentBranch() (string, error) {
	out, err := c.run("symbolic-ref", "--quiet", "--short", "HEAD")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

func (c *Client) DefaultBranch() string {
	if c.BranchExists("main") {
		return "main"
	}
	if c.BranchExists("master") {
		return "master"
	}
	return "main"
}

func (c *Client) HasCommit() bool {
	_, err := c.run("rev-parse", "--verify", "HEAD")
	return err == nil
}

func (c *Client) BranchExists(branch string) bool {
	_, err := c.run("show-ref", "--verify", "--quiet", "refs/heads/"+branch)
	return err == nil
}

func (c *Client) EnsureBaseBranch(baseBranch string) error {
	if c.BranchExists(baseBranch) {
		return c.CheckoutBranch(baseBranch)
	}
	_, err := c.run("checkout", "-B", baseBranch)
	return err
}

func (c *Client) CheckoutBranch(branch string) error {
	_, err := c.run("checkout", branch)
	return err
}

func (c *Client) CreateBranch(branch string) error {
	_, err := c.run("checkout", "-b", branch)
	return err
}

func (c *Client) DiffNames(baseBranch, branch string) ([]string, error) {
	out, err := c.run("diff", "--name-only", baseBranch+"..."+branch)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(out) == "" {
		return []string{}, nil
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	res := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			res = append(res, line)
		}
	}
	sort.Strings(res)
	return res, nil
}

func (c *Client) DiffNameStatus(baseBranch, branch string) ([]FileChange, error) {
	out, err := c.run("diff", "--name-status", baseBranch+"..."+branch)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(out) == "" {
		return []FileChange{}, nil
	}

	changes := make([]FileChange, 0)
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) < 2 {
			continue
		}
		statusToken := parts[0]
		status := string(statusToken[0])

		fc := FileChange{Status: status}
		switch {
		case status == "R" || status == "C":
			if len(parts) >= 3 {
				fc.OldPath = strings.TrimSpace(parts[1])
				fc.Path = strings.TrimSpace(parts[2])
			} else {
				fc.Path = strings.TrimSpace(parts[len(parts)-1])
			}
		default:
			fc.Path = strings.TrimSpace(parts[1])
		}
		if fc.Path != "" {
			changes = append(changes, fc)
		}
	}
	return changes, nil
}

func (c *Client) DiffShortStat(baseBranch, branch string) (string, error) {
	out, err := c.run("diff", "--shortstat", baseBranch+"..."+branch)
	if err != nil {
		return "", err
	}
	stat := strings.TrimSpace(out)
	if stat == "" {
		return "0 files changed, 0 insertions(+), 0 deletions(-)", nil
	}
	return stat, nil
}

func (c *Client) WorkingTreeStatus() ([]StatusEntry, error) {
	cmd := exec.Command("git", "status", "--porcelain=v1", "--untracked-files=all")
	cmd.Dir = c.WorkDir
	raw, err := cmd.CombinedOutput()
	if err != nil {
		trimmed := strings.TrimSpace(string(raw))
		if trimmed == "" {
			return nil, fmt.Errorf("git status --porcelain=v1 --untracked-files=all: %w", err)
		}
		return nil, fmt.Errorf("git status --porcelain=v1 --untracked-files=all: %w: %s", err, trimmed)
	}
	out := string(raw)
	if strings.TrimSpace(out) == "" {
		return []StatusEntry{}, nil
	}

	entries := make([]StatusEntry, 0)
	for _, line := range strings.Split(strings.TrimRight(out, "\n"), "\n") {
		if len(line) < 3 {
			continue
		}
		code := line[0:2]
		path := strings.TrimSpace(line[3:])
		if idx := strings.LastIndex(path, " -> "); idx >= 0 {
			path = strings.TrimSpace(path[idx+4:])
		}
		entries = append(entries, StatusEntry{Code: code, Path: path})
	}
	return entries, nil
}

func (c *Client) RecentCommits(branch string, limit int) ([]Commit, error) {
	if limit <= 0 {
		limit = 100
	}
	out, err := c.run("log", branch, "--first-parent", "-n", strconv.Itoa(limit), "--pretty=format:%H%x1f%aN <%aE>%x1f%aI%x1f%s%x1f%b%x1e")
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(out) == "" {
		return []Commit{}, nil
	}

	records := strings.Split(out, "\x1e")
	commits := make([]Commit, 0, len(records))
	for _, record := range records {
		record = strings.TrimSpace(record)
		if record == "" {
			continue
		}
		parts := strings.Split(record, "\x1f")
		if len(parts) < 5 {
			continue
		}
		commits = append(commits, Commit{
			Hash:    strings.TrimSpace(parts[0]),
			Author:  strings.TrimSpace(parts[1]),
			When:    strings.TrimSpace(parts[2]),
			Subject: strings.TrimSpace(parts[3]),
			Body:    strings.TrimSpace(parts[4]),
		})
	}
	return commits, nil
}

func (c *Client) MergeBase(baseBranch, branch string) (string, error) {
	out, err := c.run("merge-base", baseBranch, branch)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

func (c *Client) MergeNoFF(baseBranch, branch, message string) error {
	if err := c.CheckoutBranch(baseBranch); err != nil {
		return err
	}
	args := c.identityFlags()
	args = append(args, "merge", "--no-ff", branch, "-m", message)
	_, err := c.run(args...)
	return err
}

func (c *Client) ResetSoft(target string) error {
	_, err := c.run("reset", "--soft", target)
	return err
}

func (c *Client) Commit(message string) error {
	args := c.identityFlags()
	args = append(args, "commit", "-m", message)
	_, err := c.run(args...)
	return err
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

func (c *Client) MergeFFOnly(baseBranch, branch string) error {
	if err := c.CheckoutBranch(baseBranch); err != nil {
		return err
	}
	_, err := c.run("merge", "--ff-only", branch)
	return err
}

func (c *Client) TrackedFiles(pathspec string) ([]string, error) {
	args := []string{"ls-files"}
	if strings.TrimSpace(pathspec) != "" {
		args = append(args, "--", pathspec)
	}
	out, err := c.run(args...)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(out) == "" {
		return []string{}, nil
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	paths := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			paths = append(paths, line)
		}
	}
	sort.Strings(paths)
	return paths, nil
}

func (c *Client) LocalBranches(prefix string) ([]string, error) {
	out, err := c.run("for-each-ref", "--format=%(refname:short)", "refs/heads/"+prefix+"*")
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(out) == "" {
		return []string{}, nil
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	branches := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			branches = append(branches, line)
		}
	}
	sort.Strings(branches)
	return branches, nil
}

func (c *Client) ChangedFileCount(hash string) (int, error) {
	out, err := c.run("show", "--pretty=format:", "--name-only", hash)
	if err != nil {
		return 0, err
	}
	if strings.TrimSpace(out) == "" {
		return 0, nil
	}
	seen := map[string]struct{}{}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		seen[line] = struct{}{}
	}
	return len(seen), nil
}

func (c *Client) GitDir() (string, error) {
	out, err := c.run("rev-parse", "--git-dir")
	if err != nil {
		return "", err
	}
	gitDir := strings.TrimSpace(out)
	if filepath.IsAbs(gitDir) {
		return gitDir, nil
	}
	return filepath.Join(c.WorkDir, gitDir), nil
}

func (c *Client) InstallPreCommitHook(baseBranch string, forceOverwrite bool) (string, error) {
	if strings.TrimSpace(baseBranch) == "" {
		return "", fmt.Errorf("base branch cannot be empty")
	}
	gitDir, err := c.GitDir()
	if err != nil {
		return "", err
	}
	hookPath := filepath.Join(gitDir, "hooks", "pre-commit")
	if err := os.MkdirAll(filepath.Dir(hookPath), 0o755); err != nil {
		return "", fmt.Errorf("create hooks directory: %w", err)
	}

	existing, readErr := os.ReadFile(hookPath)
	if readErr == nil {
		existingText := string(existing)
		if !forceOverwrite && !strings.Contains(existingText, sophiaManagedHookMarker) {
			return "", fmt.Errorf("existing pre-commit hook is not Sophia-managed; rerun with --force-overwrite")
		}
	} else if !os.IsNotExist(readErr) {
		return "", fmt.Errorf("read existing hook: %w", readErr)
	}

	script := fmt.Sprintf("#!/usr/bin/env sh\n# %s\nbase_branch=%q\ncurrent_branch=\"$(git symbolic-ref --quiet --short HEAD 2>/dev/null || true)\"\nif [ \"$current_branch\" = \"$base_branch\" ]; then\n  echo \"Sophia guard: commits to $base_branch are blocked. Use a CR branch or bypass with --no-verify.\" >&2\n  exit 1\nfi\nexit 0\n", sophiaManagedHookMarker, baseBranch)
	if err := os.WriteFile(hookPath, []byte(script), 0o755); err != nil {
		return "", fmt.Errorf("write pre-commit hook: %w", err)
	}
	return hookPath, nil
}

func (c *Client) EnsureBootstrapCommit(message string) error {
	if c.HasCommit() {
		return nil
	}
	args := c.identityFlags()
	args = append(args, "commit", "--allow-empty", "-m", message)
	_, err := c.run(args...)
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

func (c *Client) DeleteBranch(branch string, force bool) error {
	if force {
		_, err := c.run("branch", "-D", branch)
		return err
	}
	_, err := c.run("branch", "-d", branch)
	return err
}

func (c *Client) Actor() string {
	name, _ := c.run("config", "--get", "user.name")
	email, _ := c.run("config", "--get", "user.email")
	name = strings.TrimSpace(name)
	email = strings.TrimSpace(email)

	if name == "" && email == "" {
		return "unknown"
	}
	if name == "" {
		return email
	}
	if email == "" {
		return name
	}
	return fmt.Sprintf("%s <%s>", name, email)
}

func (c *Client) HeadShortSHA() (string, error) {
	out, err := c.run("rev-parse", "--short", "HEAD")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

func (c *Client) run(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = c.WorkDir
	out, err := cmd.CombinedOutput()
	trimmed := strings.TrimSpace(string(out))
	if err != nil {
		if trimmed == "" {
			return "", fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
		}
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, trimmed)
	}
	return trimmed, nil
}

func (c *Client) identityFlags() []string {
	name, _ := c.run("config", "--get", "user.name")
	email, _ := c.run("config", "--get", "user.email")
	if strings.TrimSpace(name) != "" && strings.TrimSpace(email) != "" {
		return []string{}
	}
	return []string{"-c", "user.name=Sophia", "-c", "user.email=sophia@local"}
}
