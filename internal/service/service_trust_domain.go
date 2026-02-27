package service

import (
	"fmt"
	"sophia/internal/model"
	"sort"
	"strings"
	"time"
)

type trustDomain struct {
	svc *Service
	now func() time.Time
}

func newTrustDomain(svc *Service) *trustDomain {
	domain := &trustDomain{}
	domain.bind(svc)
	return domain
}

func (d *trustDomain) bind(svc *Service) {
	d.svc = svc
	d.now = resolveTrustNowFunc(svc)
}

func resolveTrustNowFunc(svc *Service) func() time.Time {
	if svc != nil && svc.now != nil {
		return svc.now
	}
	return time.Now
}

func (d *trustDomain) nowUTC() time.Time {
	if d == nil || d.now == nil {
		return time.Now().UTC()
	}
	return d.now().UTC()
}

func (s *Service) trustDomainService() *trustDomain {
	if s == nil {
		return newTrustDomain(nil)
	}
	if s.trustSvc == nil {
		s.trustSvc = newTrustDomain(s)
	} else {
		s.trustSvc.bind(s)
	}
	return s.trustSvc
}

func (d *trustDomain) buildReportWithPolicy(cr *model.CR, validation *ValidationReport, diff *diffSummary, requiredCRFields []string, policy *model.RepoPolicy) *TrustReport {
	return buildTrustReportWithPolicyAt(cr, validation, diff, requiredCRFields, policy, d.nowUTC())
}

func (d *trustDomain) trustCheckStatusCR(id int) (*TrustCheckStatusReport, error) {
	if d == nil || d.svc == nil {
		return nil, fmt.Errorf("trust domain is not initialized")
	}
	cr, err := d.svc.store.LoadCR(id)
	if err != nil {
		return nil, err
	}
	policy, err := d.svc.repoPolicy()
	if err != nil {
		return nil, err
	}
	diff, err := d.svc.summarizeCRDiff(cr)
	if err != nil {
		return nil, err
	}
	validation, err := d.svc.ValidateCR(id)
	if err != nil {
		return nil, err
	}
	trust := d.buildReportWithPolicy(cr, validation, diff, policy.Contract.RequiredFields, policy)
	checkMode, requiredCount, guidance := trustCheckModeAndGuidance(trust.CheckResults)
	return &TrustCheckStatusReport{
		CRID:               cr.ID,
		CRUID:              cr.UID,
		RiskTier:           trust.RiskTier,
		Requirements:       append([]TrustRequirement(nil), trust.Requirements...),
		CheckResults:       append([]TrustCheckResult(nil), trust.CheckResults...),
		FreshnessHours:     intValueOrDefault(policy.Trust.Checks.FreshnessHours, defaultTrustCheckFreshnessHours),
		CheckMode:          checkMode,
		RequiredCheckCount: requiredCount,
		Guidance:           guidance,
	}, nil
}

func (d *trustDomain) runTrustChecksCR(id int) (*TrustCheckRunReport, error) {
	if d == nil || d.svc == nil {
		return nil, fmt.Errorf("trust domain is not initialized")
	}
	cr, err := d.svc.store.LoadCR(id)
	if err != nil {
		return nil, err
	}
	policy, err := d.svc.repoPolicy()
	if err != nil {
		return nil, err
	}
	validation, err := d.svc.ValidateCR(id)
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
		if _, addErr := d.svc.AddEvidence(id, AddEvidenceOptions{
			Type:    evidenceTypeCommandRun,
			Command: definition.Command,
			Capture: true,
			Summary: fmt.Sprintf("Policy trust check %q", definition.Key),
		}); addErr != nil {
			return nil, addErr
		}
	}

	updatedCR, err := d.svc.store.LoadCR(id)
	if err != nil {
		return nil, err
	}
	updatedDiff, err := d.svc.summarizeCRDiff(updatedCR)
	if err != nil {
		return nil, err
	}
	updatedValidation, err := d.svc.ValidateCR(id)
	if err != nil {
		return nil, err
	}
	trust := d.buildReportWithPolicy(updatedCR, updatedValidation, updatedDiff, policy.Contract.RequiredFields, policy)
	checkMode, requiredCount, guidance := trustCheckModeAndGuidance(trust.CheckResults)
	return &TrustCheckRunReport{
		CRID:               updatedCR.ID,
		CRUID:              updatedCR.UID,
		RiskTier:           trust.RiskTier,
		Requirements:       append([]TrustRequirement(nil), trust.Requirements...),
		CheckResults:       append([]TrustCheckResult(nil), trust.CheckResults...),
		Executed:           len(requiredChecks),
		CheckMode:          checkMode,
		RequiredCheckCount: requiredCount,
		Guidance:           guidance,
	}, nil
}

func trustCheckModeAndGuidance(checkResults []TrustCheckResult) (string, int, []string) {
	requiredCount := len(checkResults)
	if requiredCount == 0 {
		return "none", 0, []string{
			"No checks are currently required for this CR.",
			"Define trust.checks.definitions in SOPHIA.yaml to require executable checks by risk tier.",
			"Done-task acceptance checks can also require check keys via `sophia cr task contract set --acceptance-check <key>`.",
		}
	}
	return "required", requiredCount, []string{}
}
