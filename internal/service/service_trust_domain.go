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

func (d *trustDomain) buildReport(cr *model.CR, validation *ValidationReport, diff *diffSummary, requiredCRFields []string) *TrustReport {
	policy := defaultRepoPolicy()
	// Compatibility wrapper for unit tests that exercise dimension scoring in isolation.
	policy.Trust.Checks.Definitions = []model.PolicyTrustCheckDefinition{}
	policy.Trust.ReviewDepth.Low.MinSamples = intPtr(0)
	policy.Trust.ReviewDepth.Low.RequireCriticalScopeCoverage = boolPtr(false)
	policy.Trust.ReviewDepth.Medium.MinSamples = intPtr(0)
	policy.Trust.ReviewDepth.Medium.RequireCriticalScopeCoverage = boolPtr(false)
	policy.Trust.ReviewDepth.High.MinSamples = intPtr(0)
	policy.Trust.ReviewDepth.High.RequireCriticalScopeCoverage = boolPtr(false)
	policy.Trust.Thresholds.Low = floatPtr(trustTrustedMinRatio)
	policy.Trust.Thresholds.Medium = floatPtr(trustTrustedMinRatio)
	policy.Trust.Thresholds.High = floatPtr(trustTrustedMinRatio)
	return d.buildReportWithPolicy(cr, validation, diff, requiredCRFields, policy)
}

func (d *trustDomain) buildReportWithPolicy(cr *model.CR, validation *ValidationReport, diff *diffSummary, requiredCRFields []string, policy *model.RepoPolicy) *TrustReport {
	return d.buildReportWithPolicyAt(cr, validation, diff, requiredCRFields, policy, d.nowUTC())
}

func (d *trustDomain) buildReportWithPolicyAt(cr *model.CR, validation *ValidationReport, diff *diffSummary, requiredCRFields []string, policy *model.RepoPolicy, now time.Time) *TrustReport {
	if cr == nil {
		return &TrustReport{
			Verdict:      trustVerdictUntrusted,
			Score:        0,
			Max:          100,
			AdvisoryOnly: true,
			HardFailures: []string{"CR metadata is missing"},
			Dimensions:   []TrustDimension{},
			RequiredActions: []string{
				"Re-run review on an existing CR.",
			},
			Advisories: []string{},
			Summary:    "Trust evidence unavailable because CR metadata is missing.",
			Gate: TrustGateSummary{
				Enabled: false,
				Applies: false,
				Blocked: false,
				Reason:  "CR metadata is missing.",
			},
		}
	}
	if policy == nil {
		policy = defaultRepoPolicy()
	}
	if now.IsZero() {
		now = d.nowUTC()
	} else {
		now = now.UTC()
	}
	validation, diff, impact, shortStat := normalizeTrustInputs(validation, diff)
	hardFailures, requirements := buildInitialTrustRequirements(cr, validation, requiredCRFields)
	dimensions, score, max, dimensionActions, advisories := evaluateTrustDimensions(cr, validation, impact, diff, shortStat)
	riskTier := normalizedRiskTier(impact.RiskTier)
	reviewDepth := evaluateTrustReviewDepth(cr, policy.Trust, riskTier)
	checkRequirements, checkResults := buildTrustCheckRequirements(cr, policy.Trust, riskTier, now)
	requirements = append(requirements, checkRequirements...)
	requirements = append(requirements, buildReviewDepthRequirement(reviewDepth))
	contractDrift := summarizeTaskContractDrift(cr.Subtasks)
	crContractDrift := summarizeCRContractDrift(cr.ContractDrifts)
	requirements = append(requirements, buildContractDriftRequirement(contractDrift, cr.ID))
	requirements = append(requirements, buildCRContractDriftRequirement(crContractDrift, cr.ID))
	advisories = appendRiskTierAdvisories(advisories, impact, reviewDepth, cr.Contract, diff)
	requiredActions := collectRequiredActions(requirements)

	verdict, summary := selectTrustVerdictForPolicy(score, max, hardFailures, requirements, policy.Trust, riskTier)
	attentionActions := []string{}
	if verdict == trustVerdictNeedsAttention && trustRequirementsSatisfied(requirements) {
		attentionActions = dedupeStrings(dimensionActions)
	}
	gate := buildTrustGateSummary(policy.Trust, riskTier, verdict)

	return &TrustReport{
		Verdict:          verdict,
		Score:            score,
		Max:              max,
		AdvisoryOnly:     !(gate.Enabled && gate.Applies),
		HardFailures:     dedupeStrings(hardFailures),
		Dimensions:       dimensions,
		RequiredActions:  requiredActions,
		Advisories:       advisories,
		Summary:          summary,
		AttentionActions: attentionActions,
		RiskTier:         riskTier,
		Requirements:     requirements,
		CheckResults:     checkResults,
		ReviewDepth:      reviewDepth,
		ContractDrift:    contractDrift,
		CRContractDrift:  crContractDrift,
		Gate:             gate,
	}
}

func (d *trustDomain) trustCheckStatusCR(id int) (*TrustCheckStatusReport, error) {
	if d == nil || d.svc == nil {
		return nil, fmt.Errorf("trust domain is not initialized")
	}
	statusStore := d.svc.activeStatusStoreProvider()
	cr, err := statusStore.LoadCR(id)
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
	statusStore := d.svc.activeStatusStoreProvider()
	cr, err := statusStore.LoadCR(id)
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

	updatedCR, err := statusStore.LoadCR(id)
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
