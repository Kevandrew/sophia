package merge

import (
	"fmt"
	"strconv"
	"strings"
)

type OverrideEvidence struct {
	ValidationErrorCount int
	RiskTier             string
	TrustVerdict         string
	TrustGateBlocked     bool
}

func BuildStatusAdvice(inProgress, targetMatches bool, crID int) []string {
	advice := []string{}
	if inProgress {
		advice = append(advice, fmt.Sprintf("Resolve conflicted files and run sophia cr merge resume %d.", crID))
		advice = append(advice, fmt.Sprintf("Run sophia cr merge abort %d to abandon the in-progress merge.", crID))
		if !targetMatches {
			advice = append(advice, "Current in-progress merge does not match this CR target branch.")
		}
		return advice
	}
	advice = append(advice, "No merge in progress for this CR.")
	return advice
}

func BuildConflictEventMeta(worktreePath, baseBranch, crBranch string, conflictFiles []string, cause error) map[string]string {
	meta := map[string]string{
		"worktree_path":  strings.TrimSpace(worktreePath),
		"base_branch":    strings.TrimSpace(baseBranch),
		"cr_branch":      strings.TrimSpace(crBranch),
		"conflict_count": strconv.Itoa(len(conflictFiles)),
	}
	if len(conflictFiles) > 0 {
		limit := conflictFiles
		if len(limit) > 20 {
			limit = limit[:20]
		}
		meta["conflict_files"] = strings.Join(limit, ",")
	}
	if cause != nil {
		meta["cause"] = strings.TrimSpace(cause.Error())
	}
	return meta
}

func BuildOverrideEventMeta(overrideReason string, evidence OverrideEvidence) map[string]string {
	riskTier := nonEmptyTrimmed(evidence.RiskTier, "-")
	trustVerdict := nonEmptyTrimmed(evidence.TrustVerdict, "-")

	return map[string]string{
		"override_reason":    overrideReason,
		"risk_tier":          riskTier,
		"validation_errors":  strconv.Itoa(evidence.ValidationErrorCount),
		"trust_verdict":      trustVerdict,
		"trust_gate_blocked": strconv.FormatBool(evidence.TrustGateBlocked),
	}
}

func nonEmptyTrimmed(value, fallback string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return fallback
	}
	return trimmed
}
