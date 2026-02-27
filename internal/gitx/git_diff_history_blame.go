package gitx

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

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

func (c *Client) DiffNameStatusCached() ([]FileChange, error) {
	return c.diffNameStatusWithArgs("--cached")
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

func (c *Client) DiffShortStatCached() (string, error) {
	return c.diffShortStatWithArgs("--cached")
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

func (c *Client) DiffNumStatBetween(fromRef, toRef string) ([]DiffNumStat, error) {
	return c.diffNumStatWithArgs(fromRef, toRef)
}

func (c *Client) DiffNumStatCached() ([]DiffNumStat, error) {
	return c.diffNumStatWithArgs("--cached")
}

func (c *Client) diffNumStatWithArgs(revs ...string) ([]DiffNumStat, error) {
	args := append([]string{"diff", "--numstat"}, revs...)
	out, err := c.run(args...)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(out) == "" {
		return []DiffNumStat{}, nil
	}
	stats := make([]DiffNumStat, 0)
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) < 3 {
			continue
		}
		path := strings.TrimSpace(parts[2])
		if path == "" {
			continue
		}
		row := DiffNumStat{
			Path: path,
		}
		addRaw := strings.TrimSpace(parts[0])
		delRaw := strings.TrimSpace(parts[1])
		if addRaw == "-" || delRaw == "-" {
			row.Binary = true
		} else {
			addVal, addErr := strconv.Atoi(addRaw)
			delVal, delErr := strconv.Atoi(delRaw)
			if addErr != nil || delErr != nil {
				continue
			}
			row.Insertions = &addVal
			row.Deletions = &delVal
		}
		stats = append(stats, row)
	}
	sort.Slice(stats, func(i, j int) bool {
		return stats[i].Path < stats[j].Path
	})
	return stats, nil
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
