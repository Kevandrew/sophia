package service

import (
	"sophia/internal/model"
	servicetasks "sophia/internal/service/tasks"
	"strings"
)

func (s *Service) applyCRContractPatch(contract *model.Contract, patch ContractPatch, policy *model.RepoPolicy) ([]string, error) {
	changed := make([]string, 0, 10)
	if patch.Why != nil {
		normalized := strings.TrimSpace(*patch.Why)
		if contract.Why != normalized {
			contract.Why = normalized
			changed = append(changed, "why")
		}
	}
	if patch.Scope != nil {
		scope, err := s.normalizeContractScopePrefixes(*patch.Scope)
		if err != nil {
			return nil, err
		}
		if err := enforceScopeAllowlist(scope, policy.Scope.AllowedPrefixes, "cr contract scope"); err != nil {
			return nil, err
		}
		if !equalStringSlices(contract.Scope, scope) {
			contract.Scope = scope
			changed = append(changed, "scope")
		}
	}
	if patch.NonGoals != nil {
		normalized := normalizeNonEmptyStringList(*patch.NonGoals)
		if !equalStringSlices(contract.NonGoals, normalized) {
			contract.NonGoals = normalized
			changed = append(changed, "non_goals")
		}
	}
	if patch.Invariants != nil {
		normalized := normalizeNonEmptyStringList(*patch.Invariants)
		if !equalStringSlices(contract.Invariants, normalized) {
			contract.Invariants = normalized
			changed = append(changed, "invariants")
		}
	}
	if patch.BlastRadius != nil {
		normalized := strings.TrimSpace(*patch.BlastRadius)
		if contract.BlastRadius != normalized {
			contract.BlastRadius = normalized
			changed = append(changed, "blast_radius")
		}
	}
	if patch.RiskCriticalScopes != nil {
		scopes, err := s.normalizeContractScopePrefixes(*patch.RiskCriticalScopes)
		if err != nil {
			return nil, err
		}
		if !equalStringSlices(contract.RiskCriticalScopes, scopes) {
			contract.RiskCriticalScopes = scopes
			changed = append(changed, "risk_critical_scopes")
		}
	}
	if patch.RiskTierHint != nil {
		tierHint, err := normalizeRiskTierHint(*patch.RiskTierHint)
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(contract.RiskTierHint) != tierHint {
			contract.RiskTierHint = tierHint
			changed = append(changed, "risk_tier_hint")
		}
	}
	if patch.RiskRationale != nil {
		normalized := strings.TrimSpace(*patch.RiskRationale)
		if contract.RiskRationale != normalized {
			contract.RiskRationale = normalized
			changed = append(changed, "risk_rationale")
		}
	}
	if patch.TestPlan != nil {
		normalized := strings.TrimSpace(*patch.TestPlan)
		if contract.TestPlan != normalized {
			contract.TestPlan = normalized
			changed = append(changed, "test_plan")
		}
	}
	if patch.RollbackPlan != nil {
		normalized := strings.TrimSpace(*patch.RollbackPlan)
		if contract.RollbackPlan != normalized {
			contract.RollbackPlan = normalized
			changed = append(changed, "rollback_plan")
		}
	}
	return changed, nil
}

func (s *Service) applyTaskContractPatch(taskID int, contract *model.TaskContract, patch TaskContractPatch, policy *model.RepoPolicy) ([]string, error) {
	changed := make([]string, 0, 4)
	if patch.Intent != nil {
		normalized := strings.TrimSpace(*patch.Intent)
		if contract.Intent != normalized {
			contract.Intent = normalized
			changed = append(changed, "intent")
		}
	}
	if patch.AcceptanceCriteria != nil {
		normalized := normalizeNonEmptyStringList(*patch.AcceptanceCriteria)
		if !equalStringSlices(contract.AcceptanceCriteria, normalized) {
			contract.AcceptanceCriteria = normalized
			changed = append(changed, "acceptance_criteria")
		}
	}
	if patch.Scope != nil {
		normalized, err := s.normalizeContractScopePrefixes(*patch.Scope)
		if err != nil {
			return nil, err
		}
		if err := enforceScopeAllowlist(normalized, policy.Scope.AllowedPrefixes, "task contract scope"); err != nil {
			return nil, err
		}
		if !equalStringSlices(contract.Scope, normalized) {
			contract.Scope = normalized
			changed = append(changed, "scope")
		}
	}
	if patch.AcceptanceChecks != nil {
		normalized := servicetasks.NormalizeAcceptanceCheckKeys(*patch.AcceptanceChecks)
		if err := validateTaskAcceptanceCheckKeys(taskID, normalized, policy); err != nil {
			return nil, err
		}
		if !equalStringSlices(contract.AcceptanceChecks, normalized) {
			contract.AcceptanceChecks = normalized
			changed = append(changed, "acceptance_checks")
		}
	}
	return changed, nil
}
