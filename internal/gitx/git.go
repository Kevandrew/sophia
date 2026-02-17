package gitx

import (
	"fmt"
	"os/exec"
	"strings"
)

type Client struct {
	WorkDir string
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
	return res, nil
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
