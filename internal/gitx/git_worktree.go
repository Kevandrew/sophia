package gitx

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

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
