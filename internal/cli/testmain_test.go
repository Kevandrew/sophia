package cli

import (
	"os"
	"testing"
)

// Set deterministic git identity for all CLI tests so fixtures don't require
// per-repo config subprocess setup.
func TestMain(m *testing.M) {
	_ = os.Setenv("GIT_AUTHOR_NAME", "Test User")
	_ = os.Setenv("GIT_AUTHOR_EMAIL", "test@example.com")
	_ = os.Setenv("GIT_COMMITTER_NAME", "Test User")
	_ = os.Setenv("GIT_COMMITTER_EMAIL", "test@example.com")
	os.Exit(m.Run())
}
