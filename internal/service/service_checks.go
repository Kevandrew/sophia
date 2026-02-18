package service

import "fmt"

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
