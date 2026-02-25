package trust

import (
	"regexp"
	"sophia/internal/model"
	"strconv"
	"strings"
)

var leadingIntPattern = regexp.MustCompile(`^\d+`)

type ShortStatMetrics struct {
	FilesChanged int
	Insertions   int
	Deletions    int
}

func NormalizeRiskTier(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "high":
		return "high"
	case "medium":
		return "medium"
	default:
		return "low"
	}
}

func TrustScoreRatio(score, max int) float64 {
	if max <= 0 {
		return 0
	}
	return float64(score) / float64(max)
}

func BoolValueOrDefault(value *bool, fallback bool) bool {
	if value == nil {
		return fallback
	}
	return *value
}

func IntValueOrDefault(value *int, fallback int) int {
	if value == nil {
		return fallback
	}
	return *value
}

func FloatValueOrDefault(value *float64, fallback float64) float64 {
	if value == nil {
		return fallback
	}
	return *value
}

func ContainsInt(values []int, target int) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func StringSliceContains(values []string, target string) bool {
	for _, value := range values {
		if strings.EqualFold(strings.TrimSpace(value), strings.TrimSpace(target)) {
			return true
		}
	}
	return false
}

func ParseShortStatMetrics(shortStat string) ShortStatMetrics {
	metrics := ShortStatMetrics{}
	trimmed := strings.TrimSpace(shortStat)
	if trimmed == "" {
		return metrics
	}
	for _, rawPart := range strings.Split(trimmed, ",") {
		part := strings.ToLower(strings.TrimSpace(rawPart))
		if part == "" {
			continue
		}
		value := ParseLeadingInt(part)
		switch {
		case strings.Contains(part, "file") && strings.Contains(part, "changed"):
			metrics.FilesChanged = value
		case strings.Contains(part, "insertion"):
			metrics.Insertions = value
		case strings.Contains(part, "deletion"):
			metrics.Deletions = value
		}
	}
	return metrics
}

func ParseLeadingInt(input string) int {
	match := leadingIntPattern.FindString(strings.TrimSpace(input))
	if strings.TrimSpace(match) == "" {
		return 0
	}
	value, err := strconv.Atoi(match)
	if err != nil {
		return 0
	}
	return value
}

func EffectiveFilesChanged(impactFilesChanged int, diffFilesChanged int, shortStat ShortStatMetrics) int {
	if impactFilesChanged > 0 {
		return impactFilesChanged
	}
	if diffFilesChanged > 0 {
		return diffFilesChanged
	}
	if shortStat.FilesChanged > 0 {
		return shortStat.FilesChanged
	}
	return 0
}

func TrustThresholdForTier(trust model.PolicyTrust, riskTier string, defaultLow, defaultMedium, defaultHigh float64) float64 {
	switch NormalizeRiskTier(riskTier) {
	case "high":
		return FloatValueOrDefault(trust.Thresholds.High, defaultHigh)
	case "medium":
		return FloatValueOrDefault(trust.Thresholds.Medium, defaultMedium)
	default:
		return FloatValueOrDefault(trust.Thresholds.Low, defaultLow)
	}
}

func TrustVerdictRank(verdict, trustedVerdict, attentionVerdict string) int {
	switch strings.ToLower(strings.TrimSpace(verdict)) {
	case strings.ToLower(strings.TrimSpace(trustedVerdict)):
		return 2
	case strings.ToLower(strings.TrimSpace(attentionVerdict)):
		return 1
	default:
		return 0
	}
}

func TrustGateAppliesToRiskTier(applyRiskTiers []string, defaultApplyRiskTiers []string, riskTier string) bool {
	tiers := applyRiskTiers
	if len(tiers) == 0 {
		tiers = defaultApplyRiskTiers
	}
	return StringSliceContains(tiers, NormalizeRiskTier(riskTier))
}

func SampleScopesMatchCriticalScope(sampleScopes []string, criticalScope string, pathMatchesScopePrefix func(path, scopePrefix string) bool) bool {
	critical := strings.TrimSpace(criticalScope)
	if critical == "" {
		return false
	}
	for _, sample := range sampleScopes {
		scope := strings.TrimSpace(sample)
		if scope == "" {
			continue
		}
		if pathMatchesScopePrefix(critical, scope) || pathMatchesScopePrefix(scope, critical) {
			return true
		}
	}
	return false
}

func IsWeakTrustText(value string) bool {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return true
	}
	if len([]rune(trimmed)) < 20 {
		return true
	}
	normalized := strings.ToLower(trimmed)
	for _, token := range strings.Fields(normalized) {
		switch token {
		case "todo", "tbd", "n/a", "na", "none", "...":
			return true
		}
	}
	switch normalized {
	case "n/a", "na", "none", "todo", "tbd", "...":
		return true
	default:
		return false
	}
}
