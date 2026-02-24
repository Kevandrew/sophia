package service

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"slices"
	"sophia/internal/model"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

const repoPolicyFileName = "SOPHIA.yaml"

var (
	defaultCRRequiredContractFields = []string{
		"why",
		"scope",
		"non_goals",
		"invariants",
		"blast_radius",
		"test_plan",
		"rollback_plan",
	}
	defaultTaskRequiredContractFields = []string{
		"intent",
		"acceptance_criteria",
		"scope",
	}
	defaultScopeAllowedPrefixes = []string{"."}
	defaultTestSuffixes         = []string{
		"_test.go",
		".spec.js",
		".spec.ts",
		".spec.jsx",
		".spec.tsx",
		".test.js",
		".test.ts",
		".test.jsx",
		".test.tsx",
	}
	defaultTestPathContains = []string{
		"/test/",
		"/tests/",
	}
	defaultDependencyFileNames = []string{
		"go.mod",
		"go.sum",
		"package.json",
		"package-lock.json",
		"pnpm-lock.yaml",
		"yarn.lock",
		"cargo.toml",
		"cargo.lock",
		"requirements.txt",
		"poetry.lock",
	}
	defaultTrustGateApplyRiskTiers = []string{"high"}
	defaultTrustCheckTiers         = []string{"low", "medium", "high"}

	defaultArchiveEnabled          = true
	defaultArchivePath             = ".sophia-tracked/cr"
	defaultArchiveFormat           = "yaml"
	defaultArchiveIncludeFullDiffs = false
)

const (
	policyTrustModeAdvisory = "advisory"
	policyTrustModeGate     = "gate"

	policyTrustCheckStatusPass    = "pass"
	policyTrustCheckStatusFail    = "fail"
	policyTrustCheckStatusMissing = "missing"
	policyTrustCheckStatusStale   = "stale"

	defaultTrustThresholdLow    = 0.85
	defaultTrustThresholdMedium = 0.90
	defaultTrustThresholdHigh   = 0.95

	defaultTrustCheckFreshnessHours = 24

	defaultTrustReviewLowMinSamples    = 0
	defaultTrustReviewMediumMinSamples = 0
	defaultTrustReviewHighMinSamples   = 0
)

func defaultRepoPolicy() *model.RepoPolicy {
	return &model.RepoPolicy{
		Version: "v1",
		Contract: model.PolicyContract{
			RequiredFields: append([]string(nil), defaultCRRequiredContractFields...),
		},
		TaskContract: model.PolicyTaskContract{
			RequiredFields: append([]string(nil), defaultTaskRequiredContractFields...),
		},
		Scope: model.PolicyScope{
			AllowedPrefixes: append([]string(nil), defaultScopeAllowedPrefixes...),
		},
		Classification: model.PolicyClassification{
			Test: model.PolicyClassificationTest{
				Suffixes:     append([]string(nil), defaultTestSuffixes...),
				PathContains: append([]string(nil), defaultTestPathContains...),
			},
			Dependency: model.PolicyClassificationDependency{
				FileNames: append([]string(nil), defaultDependencyFileNames...),
			},
		},
		Merge: model.PolicyMerge{
			AllowOverride: boolPtr(true),
		},
		Archive: model.PolicyArchive{
			Enabled:          boolPtr(defaultArchiveEnabled),
			Path:             defaultArchivePath,
			Format:           defaultArchiveFormat,
			IncludeFullDiffs: boolPtr(defaultArchiveIncludeFullDiffs),
		},
		Trust: *defaultPolicyTrust(),
	}
}

func boolPtr(v bool) *bool {
	return &v
}

func intPtr(v int) *int {
	return &v
}

func floatPtr(v float64) *float64 {
	return &v
}

func defaultPolicyTrust() *model.PolicyTrust {
	return &model.PolicyTrust{
		Mode: policyTrustModeAdvisory,
		Gate: model.PolicyTrustGate{
			Enabled:        boolPtr(false),
			ApplyRiskTiers: append([]string(nil), defaultTrustGateApplyRiskTiers...),
			MinVerdict:     trustVerdictTrusted,
		},
		Thresholds: model.PolicyTrustThresholds{
			Low:    floatPtr(defaultTrustThresholdLow),
			Medium: floatPtr(defaultTrustThresholdMedium),
			High:   floatPtr(defaultTrustThresholdHigh),
		},
		Checks: model.PolicyTrustChecks{
			FreshnessHours: intPtr(defaultTrustCheckFreshnessHours),
			Definitions:    []model.PolicyTrustCheckDefinition{},
		},
		ReviewDepth: model.PolicyTrustReviewDepth{
			Low: model.PolicyTrustReviewTier{
				MinSamples:                   intPtr(defaultTrustReviewLowMinSamples),
				RequireCriticalScopeCoverage: boolPtr(false),
			},
			Medium: model.PolicyTrustReviewTier{
				MinSamples:                   intPtr(defaultTrustReviewMediumMinSamples),
				RequireCriticalScopeCoverage: boolPtr(false),
			},
			High: model.PolicyTrustReviewTier{
				MinSamples:                   intPtr(defaultTrustReviewHighMinSamples),
				RequireCriticalScopeCoverage: boolPtr(false),
			},
		},
	}
}

func (s *Service) repoPolicy() (*model.RepoPolicy, error) {
	repoRoot := strings.TrimSpace(s.repoRoot)
	if repoRoot == "" {
		repoRoot = strings.TrimSpace(s.git.WorkDir)
	}
	path := filepath.Join(repoRoot, repoPolicyFileName)
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return defaultRepoPolicy(), nil
		}
		return nil, fmt.Errorf("%w: stat %s: %v", ErrPolicyInvalid, path, err)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("%w: read %s: %v", ErrPolicyInvalid, path, err)
	}
	var parsed model.RepoPolicy
	decoder := yaml.NewDecoder(bytes.NewReader(content))
	decoder.KnownFields(true)
	if err := decoder.Decode(&parsed); err != nil {
		return nil, fmt.Errorf("%w: parse %s: %v", ErrPolicyInvalid, path, err)
	}
	var extra any
	if err := decoder.Decode(&extra); err == nil {
		return nil, fmt.Errorf("%w: parse %s: multiple YAML documents are not supported", ErrPolicyInvalid, path)
	} else if err != io.EOF {
		return nil, fmt.Errorf("%w: parse %s: %v", ErrPolicyInvalid, path, err)
	}

	normalized, err := s.normalizeRepoPolicy(&parsed)
	if err != nil {
		return nil, err
	}
	return normalized, nil
}

func (s *Service) normalizeRepoPolicy(input *model.RepoPolicy) (*model.RepoPolicy, error) {
	if input == nil {
		return defaultRepoPolicy(), nil
	}
	if strings.TrimSpace(input.Version) != "v1" {
		return nil, fmt.Errorf("%w: invalid version %q (expected v1)", ErrPolicyInvalid, strings.TrimSpace(input.Version))
	}

	normalized := defaultRepoPolicy()

	crFields, err := normalizePolicyRequiredFields(input.Contract.RequiredFields, map[string]struct{}{
		"why":                  {},
		"scope":                {},
		"non_goals":            {},
		"invariants":           {},
		"blast_radius":         {},
		"risk_critical_scopes": {},
		"risk_tier_hint":       {},
		"risk_rationale":       {},
		"test_plan":            {},
		"rollback_plan":        {},
	}, "contract.required_fields")
	if err != nil {
		return nil, err
	}
	if len(crFields) > 0 {
		normalized.Contract.RequiredFields = crFields
	}

	taskFields, err := normalizePolicyRequiredFields(input.TaskContract.RequiredFields, map[string]struct{}{
		"intent":              {},
		"acceptance_criteria": {},
		"scope":               {},
	}, "task_contract.required_fields")
	if err != nil {
		return nil, err
	}
	if len(taskFields) > 0 {
		normalized.TaskContract.RequiredFields = taskFields
	}

	if len(input.Scope.AllowedPrefixes) > 0 {
		allowed, normalizeErr := s.normalizeContractScopePrefixes(input.Scope.AllowedPrefixes)
		if normalizeErr != nil {
			return nil, fmt.Errorf("%w: scope.allowed_prefixes invalid: %v", ErrPolicyInvalid, normalizeErr)
		}
		normalized.Scope.AllowedPrefixes = allowed
	}

	if len(input.Classification.Test.Suffixes) > 0 {
		normalized.Classification.Test.Suffixes = normalizePolicyStringList(input.Classification.Test.Suffixes, true)
	}
	if len(input.Classification.Test.PathContains) > 0 {
		normalized.Classification.Test.PathContains = normalizePolicyStringList(input.Classification.Test.PathContains, true)
	}
	if len(input.Classification.Dependency.FileNames) > 0 {
		normalized.Classification.Dependency.FileNames = normalizePolicyStringList(input.Classification.Dependency.FileNames, true)
	}

	if input.Merge.AllowOverride != nil {
		normalized.Merge.AllowOverride = boolPtr(*input.Merge.AllowOverride)
	}
	archive, archiveErr := normalizePolicyArchive(input.Archive)
	if archiveErr != nil {
		return nil, archiveErr
	}
	normalized.Archive = archive
	trust, trustErr := normalizePolicyTrust(input.Trust)
	if trustErr != nil {
		return nil, trustErr
	}
	normalized.Trust = trust
	return normalized, nil
}

func normalizePolicyArchive(input model.PolicyArchive) (model.PolicyArchive, error) {
	normalized := model.PolicyArchive{
		Enabled:          boolPtr(defaultArchiveEnabled),
		Path:             defaultArchivePath,
		Format:           defaultArchiveFormat,
		IncludeFullDiffs: boolPtr(defaultArchiveIncludeFullDiffs),
	}
	if input.Enabled != nil {
		normalized.Enabled = boolPtr(*input.Enabled)
	}
	if strings.TrimSpace(input.Path) != "" {
		pathValue, pathErr := normalizePolicyArchivePath(input.Path)
		if pathErr != nil {
			return model.PolicyArchive{}, pathErr
		}
		normalized.Path = pathValue
	}
	if strings.TrimSpace(input.Format) != "" {
		format := strings.ToLower(strings.TrimSpace(input.Format))
		if format != defaultArchiveFormat {
			return model.PolicyArchive{}, fmt.Errorf("%w: archive.format %q is invalid (expected yaml)", ErrPolicyInvalid, input.Format)
		}
		normalized.Format = format
	}
	if input.IncludeFullDiffs != nil {
		normalized.IncludeFullDiffs = boolPtr(*input.IncludeFullDiffs)
	}
	return normalized, nil
}

func normalizePolicyArchivePath(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	slashPath := strings.ReplaceAll(trimmed, "\\", "/")
	if slashPath == "" {
		return "", fmt.Errorf("%w: archive.path cannot be empty", ErrPolicyInvalid)
	}
	if filepath.IsAbs(trimmed) || strings.HasPrefix(slashPath, "/") {
		return "", fmt.Errorf("%w: archive.path %q must be repo-relative", ErrPolicyInvalid, raw)
	}
	if strings.ContainsAny(slashPath, "*?[]{}") {
		return "", fmt.Errorf("%w: archive.path %q must be an exact path (no glob patterns)", ErrPolicyInvalid, raw)
	}
	cleaned := path.Clean(slashPath)
	if cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", fmt.Errorf("%w: archive.path %q escapes repository root", ErrPolicyInvalid, raw)
	}
	if cleaned != slashPath {
		return "", fmt.Errorf("%w: archive.path %q must be normalized", ErrPolicyInvalid, raw)
	}
	return cleaned, nil
}

func normalizePolicyRequiredFields(values []string, allowed map[string]struct{}, label string) ([]string, error) {
	normalized := []string{}
	seen := map[string]struct{}{}
	for _, raw := range values {
		candidate := strings.ToLower(strings.TrimSpace(raw))
		if candidate == "" {
			continue
		}
		if _, ok := allowed[candidate]; !ok {
			return nil, fmt.Errorf("%w: %s contains unsupported field %q", ErrPolicyInvalid, label, raw)
		}
		if _, ok := seen[candidate]; ok {
			continue
		}
		seen[candidate] = struct{}{}
		normalized = append(normalized, candidate)
	}
	sort.Strings(normalized)
	return normalized, nil
}

func normalizePolicyStringList(values []string, lower bool) []string {
	normalized := []string{}
	seen := map[string]struct{}{}
	for _, raw := range values {
		candidate := strings.TrimSpace(raw)
		if lower {
			candidate = strings.ToLower(candidate)
		}
		if candidate == "" {
			continue
		}
		if _, ok := seen[candidate]; ok {
			continue
		}
		seen[candidate] = struct{}{}
		normalized = append(normalized, candidate)
	}
	sort.Strings(normalized)
	return normalized
}

func normalizePolicyTrust(input model.PolicyTrust) (model.PolicyTrust, error) {
	normalized := *defaultPolicyTrust()

	mode, err := normalizePolicyTrustMode(input.Mode)
	if err != nil {
		return model.PolicyTrust{}, err
	}
	normalized.Mode = mode

	if input.Gate.Enabled != nil {
		normalized.Gate.Enabled = boolPtr(*input.Gate.Enabled)
	}
	if len(input.Gate.ApplyRiskTiers) > 0 {
		tiers, tierErr := normalizePolicyRiskTiers(input.Gate.ApplyRiskTiers, "trust.gate.apply_risk_tiers")
		if tierErr != nil {
			return model.PolicyTrust{}, tierErr
		}
		normalized.Gate.ApplyRiskTiers = tiers
	}
	if strings.TrimSpace(input.Gate.MinVerdict) != "" {
		minVerdict, minVerdictErr := normalizePolicyTrustMinVerdict(input.Gate.MinVerdict)
		if minVerdictErr != nil {
			return model.PolicyTrust{}, minVerdictErr
		}
		normalized.Gate.MinVerdict = minVerdict
	}

	if input.Thresholds.Low != nil {
		if thresholdErr := validatePolicyTrustThreshold(*input.Thresholds.Low, "trust.thresholds.low"); thresholdErr != nil {
			return model.PolicyTrust{}, thresholdErr
		}
		normalized.Thresholds.Low = floatPtr(*input.Thresholds.Low)
	}
	if input.Thresholds.Medium != nil {
		if thresholdErr := validatePolicyTrustThreshold(*input.Thresholds.Medium, "trust.thresholds.medium"); thresholdErr != nil {
			return model.PolicyTrust{}, thresholdErr
		}
		normalized.Thresholds.Medium = floatPtr(*input.Thresholds.Medium)
	}
	if input.Thresholds.High != nil {
		if thresholdErr := validatePolicyTrustThreshold(*input.Thresholds.High, "trust.thresholds.high"); thresholdErr != nil {
			return model.PolicyTrust{}, thresholdErr
		}
		normalized.Thresholds.High = floatPtr(*input.Thresholds.High)
	}
	if *normalized.Thresholds.Low > *normalized.Thresholds.Medium || *normalized.Thresholds.Medium > *normalized.Thresholds.High {
		return model.PolicyTrust{}, fmt.Errorf("%w: trust.thresholds must satisfy low <= medium <= high", ErrPolicyInvalid)
	}

	if input.Checks.FreshnessHours != nil {
		if *input.Checks.FreshnessHours <= 0 {
			return model.PolicyTrust{}, fmt.Errorf("%w: trust.checks.freshness_hours must be > 0", ErrPolicyInvalid)
		}
		normalized.Checks.FreshnessHours = intPtr(*input.Checks.FreshnessHours)
	}
	checks, checksErr := normalizePolicyTrustCheckDefinitions(input.Checks.Definitions)
	if checksErr != nil {
		return model.PolicyTrust{}, checksErr
	}
	if input.Checks.Definitions != nil {
		normalized.Checks.Definitions = checks
	}

	lowTier, lowErr := normalizePolicyTrustReviewTier(input.ReviewDepth.Low, normalized.ReviewDepth.Low, "trust.review_depth.low")
	if lowErr != nil {
		return model.PolicyTrust{}, lowErr
	}
	mediumTier, mediumErr := normalizePolicyTrustReviewTier(input.ReviewDepth.Medium, normalized.ReviewDepth.Medium, "trust.review_depth.medium")
	if mediumErr != nil {
		return model.PolicyTrust{}, mediumErr
	}
	highTier, highErr := normalizePolicyTrustReviewTier(input.ReviewDepth.High, normalized.ReviewDepth.High, "trust.review_depth.high")
	if highErr != nil {
		return model.PolicyTrust{}, highErr
	}
	normalized.ReviewDepth.Low = lowTier
	normalized.ReviewDepth.Medium = mediumTier
	normalized.ReviewDepth.High = highTier

	return normalized, nil
}

func normalizePolicyTrustReviewTier(input model.PolicyTrustReviewTier, fallback model.PolicyTrustReviewTier, path string) (model.PolicyTrustReviewTier, error) {
	out := model.PolicyTrustReviewTier{
		MinSamples:                   intPtr(*fallback.MinSamples),
		RequireCriticalScopeCoverage: boolPtr(*fallback.RequireCriticalScopeCoverage),
	}
	if input.MinSamples != nil {
		if *input.MinSamples < 0 {
			return model.PolicyTrustReviewTier{}, fmt.Errorf("%w: %s.min_samples must be >= 0", ErrPolicyInvalid, path)
		}
		out.MinSamples = intPtr(*input.MinSamples)
	}
	if input.RequireCriticalScopeCoverage != nil {
		out.RequireCriticalScopeCoverage = boolPtr(*input.RequireCriticalScopeCoverage)
	}
	return out, nil
}

func normalizePolicyTrustCheckDefinitions(values []model.PolicyTrustCheckDefinition) ([]model.PolicyTrustCheckDefinition, error) {
	out := make([]model.PolicyTrustCheckDefinition, 0, len(values))
	seen := map[string]struct{}{}
	for idx, raw := range values {
		path := fmt.Sprintf("trust.checks.definitions[%d]", idx)
		key := strings.TrimSpace(raw.Key)
		if key == "" {
			return nil, fmt.Errorf("%w: %s.key cannot be empty", ErrPolicyInvalid, path)
		}
		if _, ok := seen[key]; ok {
			return nil, fmt.Errorf("%w: duplicate %s.key %q", ErrPolicyInvalid, path, key)
		}
		seen[key] = struct{}{}
		command := strings.TrimSpace(raw.Command)
		if command == "" {
			return nil, fmt.Errorf("%w: %s.command cannot be empty", ErrPolicyInvalid, path)
		}

		tiers := append([]string(nil), defaultTrustCheckTiers...)
		if len(raw.Tiers) > 0 {
			normalizedTiers, tierErr := normalizePolicyRiskTiers(raw.Tiers, path+".tiers")
			if tierErr != nil {
				return nil, tierErr
			}
			tiers = normalizedTiers
		}

		allowCodes := normalizeIntList(raw.AllowExitCodes)
		if len(allowCodes) == 0 {
			allowCodes = []int{0}
		}

		out = append(out, model.PolicyTrustCheckDefinition{
			Key:            key,
			Command:        command,
			Tiers:          tiers,
			AllowExitCodes: allowCodes,
		})
	}
	return out, nil
}

func normalizePolicyRiskTiers(values []string, label string) ([]string, error) {
	normalized := []string{}
	seen := map[string]struct{}{}
	for _, raw := range values {
		tier := strings.ToLower(strings.TrimSpace(raw))
		if tier == "" {
			continue
		}
		switch tier {
		case "low", "medium", "high":
		default:
			return nil, fmt.Errorf("%w: %s contains invalid tier %q", ErrPolicyInvalid, label, raw)
		}
		if _, ok := seen[tier]; ok {
			continue
		}
		seen[tier] = struct{}{}
		normalized = append(normalized, tier)
	}
	sort.Strings(normalized)
	return normalized, nil
}

func normalizePolicyTrustMode(raw string) (string, error) {
	mode := strings.ToLower(strings.TrimSpace(raw))
	if mode == "" {
		return policyTrustModeAdvisory, nil
	}
	switch mode {
	case policyTrustModeAdvisory, policyTrustModeGate:
		return mode, nil
	default:
		return "", fmt.Errorf("%w: trust.mode %q is invalid (expected advisory or gate)", ErrPolicyInvalid, raw)
	}
}

func normalizePolicyTrustMinVerdict(raw string) (string, error) {
	value := strings.ToLower(strings.TrimSpace(raw))
	if value == "" {
		return trustVerdictTrusted, nil
	}
	switch value {
	case trustVerdictTrusted, trustVerdictNeedsAttention:
		return value, nil
	default:
		return "", fmt.Errorf("%w: trust.gate.min_verdict %q is invalid (expected trusted or needs_attention)", ErrPolicyInvalid, raw)
	}
}

func validatePolicyTrustThreshold(value float64, label string) error {
	if value <= 0 || value > 1 {
		return fmt.Errorf("%w: %s must be within (0,1], got %.6f", ErrPolicyInvalid, label, value)
	}
	return nil
}

func normalizeIntList(values []int) []int {
	if len(values) == 0 {
		return []int{}
	}
	seen := map[int]struct{}{}
	normalized := make([]int, 0, len(values))
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		normalized = append(normalized, value)
	}
	sort.Ints(normalized)
	return normalized
}

func enforceScopeAllowlist(scopeValues, allowedPrefixes []string, fieldLabel string) error {
	if len(scopeValues) == 0 || len(allowedPrefixes) == 0 {
		return nil
	}
	violations := []string{}
	for _, scope := range scopeValues {
		match := false
		for _, allowed := range allowedPrefixes {
			if pathMatchesScopePrefix(scope, allowed) {
				match = true
				break
			}
		}
		if !match {
			violations = append(violations, scope)
		}
	}
	if len(violations) == 0 {
		return nil
	}
	sort.Strings(violations)
	violations = slices.Compact(violations)
	return fmt.Errorf("%w: %s outside scope.allowed_prefixes: %s", ErrPolicyViolation, fieldLabel, strings.Join(violations, ", "))
}

func policyScopeViolationErrors(cr *model.CR, allowedPrefixes []string) []string {
	if cr == nil || len(allowedPrefixes) == 0 {
		return []string{}
	}
	errs := []string{}
	for _, scope := range cr.Contract.Scope {
		if scopeAllowedByPolicy(scope, allowedPrefixes) {
			continue
		}
		errs = append(errs, fmt.Sprintf("policy scope violation: cr contract scope %q is outside scope.allowed_prefixes", scope))
	}
	for _, task := range cr.Subtasks {
		for _, scope := range task.Contract.Scope {
			if scopeAllowedByPolicy(scope, allowedPrefixes) {
				continue
			}
			errs = append(errs, fmt.Sprintf("policy scope violation: task #%d contract scope %q is outside scope.allowed_prefixes", task.ID, scope))
		}
	}
	sort.Strings(errs)
	return dedupeStrings(errs)
}

func scopeAllowedByPolicy(scope string, allowedPrefixes []string) bool {
	for _, allowed := range allowedPrefixes {
		if pathMatchesScopePrefix(scope, allowed) {
			return true
		}
	}
	return false
}

func policyAllowsMergeOverride(policy *model.RepoPolicy) bool {
	if policy == nil || policy.Merge.AllowOverride == nil {
		return true
	}
	return *policy.Merge.AllowOverride
}
