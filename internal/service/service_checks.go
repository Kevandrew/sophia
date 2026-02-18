package service

import (
	"fmt"
	"sophia/internal/model"
	"sort"
	"strings"
)

func (s *Service) TrustCheckStatusCR(id int) (*TrustCheckStatusReport, error) {
	cr, err := s.store.LoadCR(id)
	if err != nil {
		return nil, err
	}
	policy, err := s.repoPolicy()
	if err != nil {
		return nil, err
	}
	diff, err := s.summarizeCRDiff(cr)
	if err != nil {
		return nil, err
	}
	validation, err := s.ValidateCR(id)
	if err != nil {
		return nil, err
	}
	trust := buildTrustReportWithPolicy(cr, validation, diff, policy.Contract.RequiredFields, policy)
	return &TrustCheckStatusReport{
		CRID:           cr.ID,
		CRUID:          cr.UID,
		RiskTier:       trust.RiskTier,
		Requirements:   append([]TrustRequirement(nil), trust.Requirements...),
		CheckResults:   append([]TrustCheckResult(nil), trust.CheckResults...),
		FreshnessHours: intValueOrDefault(policy.Trust.Checks.FreshnessHours, defaultTrustCheckFreshnessHours),
	}, nil
}

func (s *Service) RunTrustChecksCR(id int) (*TrustCheckRunReport, error) {
	cr, err := s.store.LoadCR(id)
	if err != nil {
		return nil, err
	}
	policy, err := s.repoPolicy()
	if err != nil {
		return nil, err
	}
	validation, err := s.ValidateCR(id)
	if err != nil {
		return nil, err
	}
	impact := validation.Impact
	riskTier := "low"
	if impact != nil {
		riskTier = normalizedRiskTier(impact.RiskTier)
	}
	requiredChecks := requiredTrustCheckDefinitions(policy.Trust.Checks.Definitions, riskTier)
	taskRequirements := requiredTaskAcceptanceChecks(cr.Subtasks)
	taskChecksByKey := taskAcceptanceCheckTaskMap(taskRequirements)
	definitionsByKey := map[string]model.PolicyTrustCheckDefinition{}
	for _, definition := range policy.Trust.Checks.Definitions {
		key := strings.TrimSpace(definition.Key)
		if key == "" {
			continue
		}
		definitionsByKey[key] = definition
	}
	for key := range taskChecksByKey {
		definition, ok := definitionsByKey[key]
		if !ok {
			continue
		}
		alreadyRequired := false
		for _, current := range requiredChecks {
			if strings.TrimSpace(current.Key) == key {
				alreadyRequired = true
				break
			}
		}
		if alreadyRequired {
			continue
		}
		requiredChecks = append(requiredChecks, definition)
	}
	sort.Slice(requiredChecks, func(i, j int) bool {
		return requiredChecks[i].Key < requiredChecks[j].Key
	})
	for _, definition := range requiredChecks {
		if _, addErr := s.AddEvidence(id, AddEvidenceOptions{
			Type:    evidenceTypeCommandRun,
			Command: definition.Command,
			Capture: true,
			Summary: fmt.Sprintf("Policy trust check %q", definition.Key),
		}); addErr != nil {
			return nil, addErr
		}
	}

	updatedCR, err := s.store.LoadCR(id)
	if err != nil {
		return nil, err
	}
	updatedDiff, err := s.summarizeCRDiff(updatedCR)
	if err != nil {
		return nil, err
	}
	updatedValidation, err := s.ValidateCR(id)
	if err != nil {
		return nil, err
	}
	trust := buildTrustReportWithPolicy(updatedCR, updatedValidation, updatedDiff, policy.Contract.RequiredFields, policy)
	return &TrustCheckRunReport{
		CRID:         updatedCR.ID,
		CRUID:        updatedCR.UID,
		RiskTier:     trust.RiskTier,
		Requirements: append([]TrustRequirement(nil), trust.Requirements...),
		CheckResults: append([]TrustCheckResult(nil), trust.CheckResults...),
		Executed:     len(requiredChecks),
	}, nil
}
