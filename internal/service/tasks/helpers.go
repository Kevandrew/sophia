package tasks

import (
	"sophia/internal/model"
	"sort"
	"strings"
)

func CloneTaskContract(contract model.TaskContract) model.TaskContract {
	out := contract
	out.AcceptanceCriteria = append([]string(nil), contract.AcceptanceCriteria...)
	out.Scope = append([]string(nil), contract.Scope...)
	out.AcceptanceChecks = append([]string(nil), contract.AcceptanceChecks...)
	return out
}

func NormalizeAcceptanceCheckKeys(values []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, raw := range values {
		key := strings.TrimSpace(raw)
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}

func TaskContractBaselineIsEmpty(baseline model.TaskContractBaseline) bool {
	return strings.TrimSpace(baseline.CapturedAt) == "" &&
		strings.TrimSpace(baseline.CapturedBy) == "" &&
		strings.TrimSpace(baseline.Intent) == "" &&
		len(baseline.AcceptanceCriteria) == 0 &&
		len(baseline.Scope) == 0 &&
		len(baseline.AcceptanceChecks) == 0
}

func TaskContractBaselineFromContract(contract model.TaskContract, capturedAt, capturedBy string) model.TaskContractBaseline {
	return model.TaskContractBaseline{
		CapturedAt:         capturedAt,
		CapturedBy:         capturedBy,
		Intent:             strings.TrimSpace(contract.Intent),
		AcceptanceCriteria: append([]string(nil), contract.AcceptanceCriteria...),
		Scope:              append([]string(nil), contract.Scope...),
		AcceptanceChecks:   append([]string(nil), contract.AcceptanceChecks...),
	}
}

func NextTaskContractDriftID(drifts []model.TaskContractDrift) int {
	maxID := 0
	for _, drift := range drifts {
		if drift.ID > maxID {
			maxID = drift.ID
		}
	}
	return maxID + 1
}

func ScopeWidened(beforeScope, afterScope []string, pathMatchesScopePrefix func(path, scopePrefix string) bool) bool {
	if len(afterScope) == 0 {
		return false
	}
	if len(beforeScope) == 0 {
		return true
	}
	for _, next := range afterScope {
		covered := false
		for _, prev := range beforeScope {
			if pathMatchesScopePrefix(next, prev) {
				covered = true
				break
			}
		}
		if !covered {
			return true
		}
	}
	return false
}

func TaskAcceptanceCheckPolicyMap(policy *model.RepoPolicy) map[string]struct{} {
	out := map[string]struct{}{}
	if policy == nil {
		return out
	}
	for _, check := range policy.Trust.Checks.Definitions {
		key := strings.TrimSpace(check.Key)
		if key == "" {
			continue
		}
		out[key] = struct{}{}
	}
	return out
}
