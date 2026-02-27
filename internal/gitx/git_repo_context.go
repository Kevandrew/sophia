package gitx

import (
	"path/filepath"
	"strings"
)

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
