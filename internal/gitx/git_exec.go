package gitx

import (
	"fmt"
	"os/exec"
	"strings"
)

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

func (c *Client) run(args ...string) (string, error) {
	command := strings.Join(args, " ")
	attempts := 0
	for {
		attempts++
		cmd := exec.Command("git", args...)
		cmd.Dir = c.WorkDir
		out, err := cmd.CombinedOutput()
		trimmed := strings.TrimSpace(string(out))
		if err == nil {
			return trimmed, nil
		}

		combinedFailure := strings.ToLower(strings.TrimSpace(trimmed + " " + err.Error()))
		if !containsIndexLockFailure(combinedFailure) {
			if trimmed == "" {
				return "", fmt.Errorf("git %s: %w", command, err)
			}
			return "", fmt.Errorf("git %s: %w: %s", command, err, trimmed)
		}

		retryIdx := attempts - 1
		if retryIdx >= len(indexLockRetryBackoff) {
			return "", IndexLockError{
				Command:     "git " + command,
				Attempts:    attempts,
				LastMessage: trimmed,
			}
		}
		sleepForIndexLockRetry(indexLockRetryBackoff[retryIdx])
	}
}

func containsIndexLockFailure(lowerMessage string) bool {
	lowerMessage = strings.ToLower(strings.TrimSpace(lowerMessage))
	return strings.Contains(lowerMessage, "index.lock")
}

func (c *Client) identityFlags() []string {
	name, _ := c.run("config", "--get", "user.name")
	email, _ := c.run("config", "--get", "user.email")
	if strings.TrimSpace(name) != "" && strings.TrimSpace(email) != "" {
		return []string{}
	}
	return []string{"-c", "user.name=Sophia", "-c", "user.email=sophia@local"}
}
