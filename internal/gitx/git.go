package gitx

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
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

type DiffNumStat struct {
	Path       string
	Insertions *int
	Deletions  *int
	Binary     bool
}

type StatusEntry struct {
	Code string
	Path string
}

type BlameRange struct {
	Start int
	End   int
}

type BlameLine struct {
	CommitHash string
	OrigLine   int
	FinalLine  int
	Author     string
	AuthorMail string
	AuthorTime string
	Summary    string
	Text       string
}

type Commit struct {
	Hash    string
	Author  string
	When    string
	Subject string
	Body    string
}

type Worktree struct {
	Path   string
	Head   string
	Branch string
}

func New(workDir string) *Client {
	return &Client{WorkDir: workDir}
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

func (c *Client) RefExists(ref string) bool {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return false
	}
	_, err := c.run("show-ref", "--verify", "--quiet", ref)
	return err == nil
}

func (c *Client) ResolveSymbolicRef(ref string) (string, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", fmt.Errorf("ref cannot be empty")
	}
	out, err := c.run("symbolic-ref", "--quiet", ref)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

func (c *Client) SetSymbolicRef(ref, target string) error {
	ref = strings.TrimSpace(ref)
	target = strings.TrimSpace(target)
	if ref == "" {
		return fmt.Errorf("ref cannot be empty")
	}
	if target == "" {
		return fmt.Errorf("target cannot be empty")
	}
	_, err := c.run("symbolic-ref", ref, target)
	return err
}

func (c *Client) UpdateRef(ref, target string) error {
	ref = strings.TrimSpace(ref)
	target = strings.TrimSpace(target)
	if ref == "" {
		return fmt.Errorf("ref cannot be empty")
	}
	if target == "" {
		return fmt.Errorf("target cannot be empty")
	}
	_, err := c.run("update-ref", "--no-deref", ref, target)
	return err
}

func (c *Client) DeleteRef(ref string) error {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return fmt.Errorf("ref cannot be empty")
	}
	if !c.RefExists(ref) {
		return nil
	}
	_, err := c.run("update-ref", "-d", ref)
	return err
}

func (c *Client) ListRefs(prefix string) ([]string, error) {
	prefix = strings.TrimSpace(prefix)
	args := []string{"for-each-ref", "--format=%(refname)"}
	if prefix != "" {
		args = append(args, prefix)
	}
	out, err := c.run(args...)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(out) == "" {
		return []string{}, nil
	}
	lines := strings.Split(out, "\n")
	refs := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		refs = append(refs, line)
	}
	sort.Strings(refs)
	return refs, nil
}

func (c *Client) EnsureBaseBranch(baseBranch string) error {
	baseBranch = strings.TrimSpace(baseBranch)
	if baseBranch == "" {
		return fmt.Errorf("base branch cannot be empty")
	}
	return c.EnsureBranchExists(baseBranch)
}

func (c *Client) EnsureBranchExists(branch string) error {
	branch = strings.TrimSpace(branch)
	if branch == "" {
		return fmt.Errorf("branch cannot be empty")
	}
	if c.BranchExists(branch) {
		return nil
	}
	if !c.HasCommit() {
		_, err := c.run("checkout", "-B", branch)
		return err
	}
	_, err := c.run("branch", branch, "HEAD")
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

func (c *Client) CreateBranchFrom(branch, ref string) error {
	_, err := c.run("checkout", "-b", branch, ref)
	return err
}

func (c *Client) CreateBranchAt(branch, ref string) error {
	_, err := c.run("branch", branch, ref)
	return err
}

func (c *Client) RenameBranch(oldBranch, newBranch string) error {
	_, err := c.run("branch", "-m", oldBranch, newBranch)
	return err
}

func (c *Client) ResolveRef(ref string) (string, error) {
	out, err := c.run("rev-parse", "--verify", ref)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
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

func (c *Client) Commit(message string) error {
	args := c.identityFlags()
	args = append(args, "commit", "-m", message)
	_, err := c.run(args...)
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

func (c *Client) DeleteBranch(branch string, force bool) error {
	if force {
		_, err := c.run("branch", "-D", branch)
		return err
	}
	_, err := c.run("branch", "-d", branch)
	return err
}

func (c *Client) HeadShortSHA() (string, error) {
	out, err := c.run("rev-parse", "--short", "HEAD")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}
