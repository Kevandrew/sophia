package service

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
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
	}
}

func boolPtr(v bool) *bool {
	return &v
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
	return normalized, nil
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
