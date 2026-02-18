package gitx

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
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

func (c *Client) RepoRoot() (string, error) {
	out, err := c.run("rev-parse", "--show-toplevel")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

func (c *Client) GitCommonDir() (string, error) {
	out, err := c.run("rev-parse", "--git-common-dir")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

func (c *Client) GitCommonDirAbs() (string, error) {
	gitCommonDir, err := c.GitCommonDir()
	if err != nil {
		return "", err
	}
	if filepath.IsAbs(gitCommonDir) {
		return gitCommonDir, nil
	}
	return filepath.Join(c.WorkDir, gitCommonDir), nil
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

func (c *Client) ResolveRef(ref string) (string, error) {
	out, err := c.run("rev-parse", "--verify", ref)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

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

func (c *Client) DiffNames(baseBranch, branch string) ([]string, error) {
	out, err := c.run("diff", "--name-only", baseBranch+"..."+branch)
	if err != nil {
		return nil, err
	}
	return parseDiffNames(out), nil
}

func (c *Client) DiffNamesBetween(fromRef, toRef string) ([]string, error) {
	out, err := c.run("diff", "--name-only", fromRef, toRef)
	if err != nil {
		return nil, err
	}
	return parseDiffNames(out), nil
}

func (c *Client) DiffPatchBetween(fromRef, toRef string, paths []string, unified int) (string, error) {
	args := []string{"diff"}
	if unified >= 0 {
		args = append(args, fmt.Sprintf("--unified=%d", unified))
	}
	args = append(args, fromRef, toRef)
	if len(paths) > 0 {
		args = append(args, "--")
		args = append(args, paths...)
	}
	out, err := c.run(args...)
	if err != nil {
		return "", err
	}
	return out, nil
}

func (c *Client) WorkingTreeUnifiedDiff(paths []string, unified int) (string, error) {
	args := []string{"diff", fmt.Sprintf("--unified=%d", unified)}
	if len(paths) > 0 {
		args = append(args, "--")
		args = append(args, paths...)
	}
	out, err := c.run(args...)
	if err != nil {
		return "", err
	}
	return out, nil
}

func parseDiffNames(out string) []string {
	if strings.TrimSpace(out) == "" {
		return []string{}
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
	return res
}

func (c *Client) DiffNameStatus(baseBranch, branch string) ([]FileChange, error) {
	return c.diffNameStatusWithArgs(baseBranch + "..." + branch)
}

func (c *Client) DiffNameStatusBetween(fromRef, toRef string) ([]FileChange, error) {
	return c.diffNameStatusWithArgs(fromRef, toRef)
}

func (c *Client) diffNameStatusWithArgs(revs ...string) ([]FileChange, error) {
	args := append([]string{"diff", "--name-status"}, revs...)
	out, err := c.run(args...)
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
	return c.diffShortStatWithArgs(baseBranch + "..." + branch)
}

func (c *Client) DiffShortStatBetween(fromRef, toRef string) (string, error) {
	return c.diffShortStatWithArgs(fromRef, toRef)
}

func (c *Client) diffShortStatWithArgs(revs ...string) (string, error) {
	args := append([]string{"diff", "--shortstat"}, revs...)
	out, err := c.run(args...)
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

func (c *Client) CommitByHash(hash string) (*Commit, error) {
	hash = strings.TrimSpace(hash)
	if hash == "" {
		return nil, fmt.Errorf("commit hash cannot be empty")
	}
	out, err := c.run("show", "-s", "--format=%H%x1f%aN <%aE>%x1f%aI%x1f%s%x1f%b", hash)
	if err != nil {
		return nil, err
	}
	parts := strings.SplitN(out, "\x1f", 5)
	if len(parts) < 5 {
		return nil, fmt.Errorf("unexpected git show output for commit %q", hash)
	}
	return &Commit{
		Hash:    strings.TrimSpace(parts[0]),
		Author:  strings.TrimSpace(parts[1]),
		When:    strings.TrimSpace(parts[2]),
		Subject: strings.TrimSpace(parts[3]),
		Body:    strings.TrimSpace(parts[4]),
	}, nil
}

func (c *Client) CommitFiles(hash string) ([]string, error) {
	hash = strings.TrimSpace(hash)
	if hash == "" {
		return nil, fmt.Errorf("commit hash cannot be empty")
	}
	out, err := c.run("show", "--pretty=format:", "--name-only", hash)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(out) == "" {
		return []string{}, nil
	}
	seen := map[string]struct{}{}
	files := []string{}
	for _, line := range strings.Split(out, "\n") {
		path := strings.TrimSpace(line)
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

func (c *Client) CommitPatch(hash string) (string, error) {
	hash = strings.TrimSpace(hash)
	if hash == "" {
		return "", fmt.Errorf("commit hash cannot be empty")
	}
	out, err := c.run("show", "--format=", "--patch", hash)
	if err != nil {
		return "", err
	}
	return out, nil
}

func (c *Client) RangeDiff(oldRange, newRange string) (string, error) {
	oldRange = strings.TrimSpace(oldRange)
	newRange = strings.TrimSpace(newRange)
	if oldRange == "" || newRange == "" {
		return "", fmt.Errorf("old and new ranges are required")
	}
	oldBase, oldTip, oldOK := splitRange(oldRange)
	newBase, newTip, newOK := splitRange(newRange)

	// Prefer the 3-argument form when ranges share a base so empty-side comparisons
	// (for example base..base vs base..HEAD) still return a structured mapping.
	args := []string{"range-diff", "--no-color"}
	if oldOK && newOK && oldBase == newBase {
		args = append(args, oldBase, oldTip, newTip)
	} else {
		args = append(args, oldRange, newRange)
	}

	out, err := c.run(args...)
	if err != nil {
		return "", err
	}
	return out, nil
}

func splitRange(rng string) (string, string, bool) {
	parts := strings.SplitN(strings.TrimSpace(rng), "..", 2)
	if len(parts) != 2 {
		return "", "", false
	}
	base := strings.TrimSpace(parts[0])
	tip := strings.TrimSpace(parts[1])
	if base == "" || tip == "" {
		return "", "", false
	}
	return base, tip, true
}

func (c *Client) BlameLinePorcelain(path string, rev string, ranges []BlameRange) ([]BlameLine, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, fmt.Errorf("path cannot be empty")
	}

	args := buildBlameArgs(path, rev, ranges)
	out, err := c.run(args...)
	if err != nil {
		return nil, err
	}
	return parseBlamePorcelain(out)
}

func buildBlameArgs(path string, rev string, ranges []BlameRange) []string {
	args := []string{"blame", "--line-porcelain"}
	for _, r := range ranges {
		args = append(args, "-L", fmt.Sprintf("%d,%d", r.Start, r.End))
	}
	if strings.TrimSpace(rev) != "" {
		args = append(args, strings.TrimSpace(rev))
	}
	args = append(args, "--", path)
	return args
}

var blameHeaderPattern = regexp.MustCompile(`^([0-9a-f]{40})\s+(\d+)\s+(\d+)(?:\s+(\d+))?$`)

func parseBlamePorcelain(raw string) ([]BlameLine, error) {
	if strings.TrimSpace(raw) == "" {
		return []BlameLine{}, nil
	}
	lines := strings.Split(strings.ReplaceAll(raw, "\r\n", "\n"), "\n")
	res := make([]BlameLine, 0)

	for i := 0; i < len(lines); {
		line := lines[i]
		if strings.TrimSpace(line) == "" {
			i++
			continue
		}
		header := blameHeaderPattern.FindStringSubmatch(strings.TrimSpace(line))
		if len(header) != 5 {
			return nil, fmt.Errorf("invalid blame porcelain header %q", line)
		}

		origLine, err := strconv.Atoi(header[2])
		if err != nil {
			return nil, fmt.Errorf("parse blame orig line %q: %w", header[2], err)
		}
		finalLine, err := strconv.Atoi(header[3])
		if err != nil {
			return nil, fmt.Errorf("parse blame final line %q: %w", header[3], err)
		}

		entry := BlameLine{
			CommitHash: header[1],
			OrigLine:   origLine,
			FinalLine:  finalLine,
		}
		i++

		var authorTimeRaw string
		var authorTZRaw string
		for i < len(lines) {
			metaLine := lines[i]
			if strings.HasPrefix(metaLine, "\t") {
				entry.Text = strings.TrimPrefix(metaLine, "\t")
				i++
				break
			}
			if blameHeaderPattern.MatchString(strings.TrimSpace(metaLine)) {
				break
			}
			if metaLine == "" {
				i++
				continue
			}
			key, value, ok := strings.Cut(metaLine, " ")
			if !ok {
				i++
				continue
			}
			switch key {
			case "author":
				entry.Author = strings.TrimSpace(value)
			case "author-mail":
				entry.AuthorMail = strings.Trim(strings.TrimSpace(value), "<>")
			case "author-time":
				authorTimeRaw = strings.TrimSpace(value)
			case "author-tz":
				authorTZRaw = strings.TrimSpace(value)
			case "summary":
				entry.Summary = strings.TrimSpace(value)
			}
			i++
		}
		entry.AuthorTime = parseAuthorTimestamp(authorTimeRaw, authorTZRaw)
		res = append(res, entry)
	}
	return res, nil
}

func parseAuthorTimestamp(unixSecondsRaw string, tzRaw string) string {
	unixSecondsRaw = strings.TrimSpace(unixSecondsRaw)
	if unixSecondsRaw == "" {
		return ""
	}
	seconds, err := strconv.ParseInt(unixSecondsRaw, 10, 64)
	if err != nil {
		return ""
	}
	loc := time.UTC
	if len(tzRaw) == 5 && (strings.HasPrefix(tzRaw, "+") || strings.HasPrefix(tzRaw, "-")) {
		sign := 1
		if strings.HasPrefix(tzRaw, "-") {
			sign = -1
		}
		hours, hErr := strconv.Atoi(tzRaw[1:3])
		minutes, mErr := strconv.Atoi(tzRaw[3:5])
		if hErr == nil && mErr == nil {
			offset := sign * ((hours * 60 * 60) + (minutes * 60))
			loc = time.FixedZone(tzRaw, offset)
		}
	}
	return time.Unix(seconds, 0).In(loc).Format(time.RFC3339)
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

func (c *Client) ListWorktrees() ([]Worktree, error) {
	out, err := c.run("worktree", "list", "--porcelain")
	if err != nil {
		return nil, err
	}
	return parseWorktreeListPorcelain(c.WorkDir, out), nil
}

func (c *Client) WorktreeForBranch(branch string) (*Worktree, error) {
	branch = strings.TrimSpace(branch)
	if branch == "" {
		return nil, fmt.Errorf("branch cannot be empty")
	}
	worktrees, err := c.ListWorktrees()
	if err != nil {
		return nil, err
	}
	for i := range worktrees {
		if worktrees[i].Branch == branch {
			wt := worktrees[i]
			return &wt, nil
		}
	}
	return nil, nil
}

func parseWorktreeListPorcelain(workDir, raw string) []Worktree {
	raw = strings.ReplaceAll(raw, "\r\n", "\n")
	blocks := strings.Split(raw, "\n\n")
	res := make([]Worktree, 0, len(blocks))
	for _, block := range blocks {
		block = strings.TrimSpace(block)
		if block == "" {
			continue
		}
		wt := Worktree{}
		for _, line := range strings.Split(block, "\n") {
			line = strings.TrimSpace(line)
			switch {
			case strings.HasPrefix(line, "worktree "):
				wt.Path = strings.TrimSpace(strings.TrimPrefix(line, "worktree "))
				if wt.Path != "" && !filepath.IsAbs(wt.Path) {
					wt.Path = filepath.Join(workDir, wt.Path)
				}
			case strings.HasPrefix(line, "HEAD "):
				wt.Head = strings.TrimSpace(strings.TrimPrefix(line, "HEAD "))
			case strings.HasPrefix(line, "branch "):
				rawBranch := strings.TrimSpace(strings.TrimPrefix(line, "branch "))
				wt.Branch = strings.TrimPrefix(rawBranch, "refs/heads/")
			}
		}
		if strings.TrimSpace(wt.Path) == "" {
			continue
		}
		res = append(res, wt)
	}
	sort.Slice(res, func(i, j int) bool {
		return res[i].Path < res[j].Path
	})
	return res
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
