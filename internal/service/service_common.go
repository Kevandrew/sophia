package service

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sophia/internal/gitx"
	"sophia/internal/model"
	"sort"
	"strings"
	"time"
)

func (s *Service) normalizeContractScopePrefixes(prefixes []string) ([]string, error) {
	normalized := make([]string, 0, len(prefixes))
	seen := map[string]struct{}{}
	for _, raw := range prefixes {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			return nil, fmt.Errorf("%w: empty scope prefix", ErrInvalidTaskScope)
		}
		slashPath := strings.ReplaceAll(trimmed, "\\", "/")
		if filepath.IsAbs(trimmed) || strings.HasPrefix(slashPath, "/") {
			return nil, fmt.Errorf("%w: scope prefix %q must be repo-relative", ErrInvalidTaskScope, raw)
		}
		if strings.ContainsAny(slashPath, "*?[]{}") {
			return nil, fmt.Errorf("%w: scope prefix %q must be exact prefix (no glob patterns)", ErrInvalidTaskScope, raw)
		}
		cleaned := path.Clean(slashPath)
		if cleaned == ".." || strings.HasPrefix(cleaned, "../") {
			return nil, fmt.Errorf("%w: scope prefix %q escapes repository root", ErrInvalidTaskScope, raw)
		}
		if cleaned != slashPath {
			return nil, fmt.Errorf("%w: scope prefix %q must be normalized", ErrInvalidTaskScope, raw)
		}
		if _, ok := seen[cleaned]; ok {
			return nil, fmt.Errorf("%w: duplicate scope prefix %q", ErrInvalidTaskScope, raw)
		}
		seen[cleaned] = struct{}{}
		normalized = append(normalized, cleaned)
	}
	sort.Strings(normalized)
	return normalized, nil
}

func normalizeNonEmptyStringList(values []string) []string {
	res := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		res = append(res, trimmed)
	}
	return dedupeStrings(res)
}

func normalizeRiskTierHint(raw string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(raw))
	switch normalized {
	case "":
		return "", nil
	case "low", "medium", "high":
		return normalized, nil
	default:
		return "", fmt.Errorf("invalid risk tier hint %q (expected low, medium, or high)", raw)
	}
}

func riskTierRank(tier string) int {
	switch strings.ToLower(strings.TrimSpace(tier)) {
	case "high":
		return 3
	case "medium":
		return 2
	case "low":
		return 1
	default:
		return 0
	}
}

func riskTierFromScore(score int) string {
	switch {
	case score >= 7:
		return "high"
	case score >= 3:
		return "medium"
	default:
		return "low"
	}
}

func riskFloorScoreForTier(tier string) int {
	switch strings.ToLower(strings.TrimSpace(tier)) {
	case "high":
		return 7
	case "medium":
		return 3
	default:
		return 0
	}
}

func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func oneBasedIndex(input, length int, label string) (int, error) {
	if input <= 0 {
		return 0, fmt.Errorf("%s index must be >= 1", label)
	}
	idx := input - 1
	if idx >= length {
		return 0, fmt.Errorf("%s index %d out of range", label, input)
	}
	return idx, nil
}

func cloneStringMap(src map[string]string) map[string]string {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]string, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func (s *Service) workingTreeDirtySummary() (bool, string, error) {
	return s.workingTreeDirtySummaryFor(s.git)
}

func (s *Service) workingTreeDirtySummaryFor(gitClient *gitx.Client) (bool, string, error) {
	if gitClient == nil {
		return false, "", nil
	}
	entries, err := gitClient.WorkingTreeStatus()
	if err != nil {
		return false, "", err
	}
	if len(entries) == 0 {
		return false, "", nil
	}
	untracked := 0
	changed := 0
	for _, entry := range entries {
		if s.isIgnorableWorktreeEntry(entry) {
			continue
		}
		if entry.Code == "??" {
			untracked++
		} else {
			changed++
		}
	}
	if changed == 0 && untracked == 0 {
		return false, "", nil
	}
	return true, fmt.Sprintf("%d modified/staged and %d untracked paths; commit or stash before switching", changed, untracked), nil
}

func nonEmptyTrimmed(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return strings.TrimSpace(value)
}

func isValidMetadataMode(mode string) bool {
	switch strings.TrimSpace(mode) {
	case model.MetadataModeLocal, model.MetadataModeTracked:
		return true
	default:
		return false
	}
}

func ensureGitIgnoreEntry(root, entry string) error {
	entry = strings.TrimSpace(entry)
	if entry == "" {
		return nil
	}

	path := filepath.Join(root, ".gitignore")
	content, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("read .gitignore: %w", err)
	}

	existing := string(content)
	lines := strings.Split(existing, "\n")
	for _, line := range lines {
		if strings.TrimSpace(line) == entry {
			return nil
		}
	}

	var b strings.Builder
	if strings.TrimSpace(existing) != "" {
		b.WriteString(strings.TrimRight(existing, "\n"))
		b.WriteString("\n")
	}
	b.WriteString(entry)
	b.WriteString("\n")

	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		return fmt.Errorf("write .gitignore: %w", err)
	}
	return nil
}

const crPlanSampleTemplate = `version: v1
crs:
  - key: parent_refactor
    title: "Decouple large service file"
    description: "Reduce service surface size and isolate responsibilities."
    base: "main"
    contract:
      why: "Define the primary intent and decision boundary."
      scope:
        - "internal/service"
        - "internal/cli"
      non_goals:
        - "No unrelated refactors."
      invariants:
        - "Existing command behavior remains compatible."
      blast_radius: "Service and CLI wiring for this refactor scope."
      risk_critical_scopes:
        - "internal/service"
      risk_tier_hint: "medium"
      risk_rationale: "Core workflow touched."
      test_plan: "go test ./... && go vet ./..."
      rollback_plan: "Revert CR merge commit."
    tasks:
      - key: split_service
        title: "Split service responsibilities"
        contract:
          intent: "Extract focused service components."
          acceptance_criteria:
            - "Responsibilities are separated with passing tests."
          scope:
            - "internal/service"
        delegate_to:
          - "child_cli"
  - key: child_cli
    title: "Split CLI command wiring"
    description: "Child implementation slice."
    parent_key: "parent_refactor"
    contract:
      why: "Keep CLI command wiring maintainable and testable."
      scope:
        - "internal/cli"
      non_goals:
        - "No new command semantics."
      invariants:
        - "Existing command outputs remain stable."
      blast_radius: "CLI command constructors and wiring."
      test_plan: "go test ./internal/cli ./internal/service"
      rollback_plan: "Revert CR merge commit."
    tasks:
      - key: split_cli
        title: "Split CLI command files"
        contract:
          intent: "Move command handlers into focused files."
          acceptance_criteria:
            - "CLI tests pass and command wiring remains stable."
          scope:
            - "internal/cli"
`

func ensureCRPlanSample(sophiaDir string) error {
	path := filepath.Join(sophiaDir, "cr-plan.sample.yaml")
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("stat %s: %w", path, err)
	}
	if err := os.WriteFile(path, []byte(crPlanSampleTemplate), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

func (s *Service) ensureNextCRIDFloor(baseBranch string) error {
	idx, err := s.store.LoadIndex()
	if err != nil {
		return err
	}
	maxID := 0

	crs, err := s.store.ListCRs()
	if err == nil {
		for _, cr := range crs {
			if cr.ID > maxID {
				maxID = cr.ID
			}
		}
	}

	branches, err := s.git.LocalBranches("sophia/cr-")
	if err == nil {
		for _, branch := range branches {
			if id, ok := parseCRBranchID(branch); ok && id > maxID {
				maxID = id
			}
		}
	}

	if strings.TrimSpace(baseBranch) != "" {
		commits, err := s.git.RecentCommits(baseBranch, 5000)
		if err == nil {
			for _, commit := range commits {
				if id, ok := crIDFromSubjectOrBody(commit.Subject, commit.Body); ok && id > maxID {
					maxID = id
				}
			}
		}
	}

	required := maxID + 1
	if required < 1 {
		required = 1
	}
	if idx.NextID >= required {
		return nil
	}
	idx.NextID = required
	return s.store.SaveIndex(idx)
}

func (s *Service) timestamp() string {
	return s.now().UTC().Format(time.RFC3339)
}

func (s *Service) isIgnorableWorktreeEntry(entry gitx.StatusEntry) bool {
	if entry.Code != "??" {
		return false
	}
	if strings.TrimSpace(entry.Path) != ".gitignore" {
		return false
	}
	data, err := os.ReadFile(filepath.Join(s.git.WorkDir, ".gitignore"))
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || line == ".sophia/" {
			continue
		}
		return false
	}
	return true
}
