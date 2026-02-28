package service

import (
	"fmt"
	"sophia/internal/model"
	servicetasks "sophia/internal/service/tasks"
	servicetrust "sophia/internal/service/trust"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	trustVerdictTrusted        = "trusted"
	trustVerdictNeedsAttention = "needs_attention"
	trustVerdictUntrusted      = "untrusted"
	trustTrustedMinRatio       = 0.85
	trustAttentionMinRatio     = 0.60
)

func buildTrustReport(cr *model.CR, validation *ValidationReport, diff *diffSummary, requiredCRFields []string) *TrustReport {
	return newTrustDomain(nil).buildReport(cr, validation, diff, requiredCRFields)
}

func normalizeTrustInputs(validation *ValidationReport, diff *diffSummary) (*ValidationReport, *diffSummary, *ImpactReport, shortStatMetrics) {
	if validation == nil {
		validation = &ValidationReport{}
	}
	if diff == nil {
		diff = &diffSummary{}
	}
	shortStat := parseShortStatMetrics(diff.ShortStat)
	impact := validation.Impact
	if impact == nil {
		impact = &ImpactReport{FilesChanged: len(diff.Files)}
	}
	if impact.FilesChanged == 0 {
		if len(diff.Files) > 0 {
			impact.FilesChanged = len(diff.Files)
		} else if shortStat.FilesChanged > 0 {
			impact.FilesChanged = shortStat.FilesChanged
		}
	}
	return validation, diff, impact, shortStat
}

func newTrustRequirement(key, title string, satisfied bool, reason, action, source string) TrustRequirement {
	return TrustRequirement{
		Key:       key,
		Title:     title,
		Satisfied: satisfied,
		Reason:    reason,
		Action:    action,
		Source:    source,
	}
}

func newTaskTrustRequirement(key, title string, satisfied bool, reason, action string, taskID int, source string) TrustRequirement {
	requirement := newTrustRequirement(key, title, satisfied, reason, action, source)
	requirement.TaskID = taskID
	return requirement
}

func buildInitialTrustRequirements(cr *model.CR, validation *ValidationReport, requiredCRFields []string) ([]string, []TrustRequirement) {
	hardFailures := []string{}
	requirements := []TrustRequirement{}
	if len(validation.Errors) > 0 {
		hardFailures = append(hardFailures, fmt.Sprintf("validation errors present (%d)", len(validation.Errors)))
		requirements = append(requirements, newTrustRequirement(
			"validation_clean",
			"Validation has no errors",
			false,
			fmt.Sprintf("%d validation error(s) present.", len(validation.Errors)),
			"Resolve all validation errors before trusting review data.",
			"validation",
		))
	} else {
		requirements = append(requirements, newTrustRequirement(
			"validation_clean",
			"Validation has no errors",
			true,
			"No validation errors.",
			"",
			"validation",
		))
	}
	missingContractFields := missingCRContractFields(cr.Contract, requiredCRFields)
	if len(missingContractFields) > 0 {
		hardFailures = append(hardFailures, fmt.Sprintf("missing required contract fields: %s", strings.Join(missingContractFields, ", ")))
		requirements = append(requirements, newTrustRequirement(
			"contract_required_fields",
			"CR required contract fields are complete",
			false,
			fmt.Sprintf("Missing fields: %s.", strings.Join(missingContractFields, ", ")),
			fmt.Sprintf("Complete required contract fields: %s.", strings.Join(missingContractFields, ", ")),
			"contract_required_fields",
		))
	} else {
		requirements = append(requirements, newTrustRequirement(
			"contract_required_fields",
			"CR required contract fields are complete",
			true,
			"All required contract fields are present.",
			"",
			"contract_required_fields",
		))
	}
	unjustifiedNoCheckpointTasks := listUnjustifiedDoneTasksWithoutCheckpoint(cr.Subtasks)
	checkpointExceptionRequirement := newTrustRequirement(
		"task_checkpoint_exception_justified",
		"Done tasks without checkpoints include explicit rationale",
		len(unjustifiedNoCheckpointTasks) == 0,
		"All done tasks are backed by checkpoint commits or explicit no-checkpoint reasons.",
		"",
		"task_proof_chain",
	)
	if len(unjustifiedNoCheckpointTasks) > 0 {
		checkpointExceptionRequirement.Reason = fmt.Sprintf("Done task(s) missing checkpoint rationale: %s.", formatTaskIDList(unjustifiedNoCheckpointTasks))
		checkpointExceptionRequirement.Action = fmt.Sprintf("Record rationale with `sophia cr task done %d <task-id> --no-checkpoint --no-checkpoint-reason \"...\"` or create scoped checkpoints.", cr.ID)
	}
	requirements = append(requirements, checkpointExceptionRequirement)
	return hardFailures, requirements
}

func evaluateTrustDimensions(cr *model.CR, validation *ValidationReport, impact *ImpactReport, diff *diffSummary, shortStat shortStatMetrics) ([]TrustDimension, int, int, []string, []string) {
	dimensions := []TrustDimension{
		buildContractQualityDimension(cr.Contract),
		buildScopeDisciplineDimension(impact),
		buildTaskProofChainDimension(cr.Subtasks),
		buildRiskAccountabilityDimension(cr.Contract, impact, diff),
		buildChangeMagnitudeDimension(impact, shortStat),
		buildValidationHealthDimension(validation),
		buildTestEvidenceDimension(cr.Contract, diff),
	}
	score := 0
	max := 0
	dimensionActions := []string{}
	advisories := []string{}
	for i := range dimensions {
		dimensions[i].Score = clampMin(dimensions[i].Score, 0)
		if dimensions[i].Score > dimensions[i].Max {
			dimensions[i].Score = dimensions[i].Max
		}
		dimensions[i].Reasons = dedupeStrings(dimensions[i].Reasons)
		dimensions[i].RequiredActions = dedupeStrings(dimensions[i].RequiredActions)
		dimensionActions = append(dimensionActions, dimensions[i].RequiredActions...)
		score += dimensions[i].Score
		max += dimensions[i].Max
		for _, action := range dimensions[i].RequiredActions {
			if isTrustGatingAction(action) {
				continue
			}
			advisories = append(advisories, action)
		}
	}
	return dimensions, score, max, dimensionActions, dedupeStrings(advisories)
}

func buildTrustCheckRequirements(cr *model.CR, trust model.PolicyTrust, riskTier string, now time.Time) ([]TrustRequirement, []TrustCheckResult) {
	taskAcceptanceRequirements := requiredTaskAcceptanceChecks(cr.Subtasks)
	taskChecksByKey := taskAcceptanceCheckTaskMap(taskAcceptanceRequirements)
	checkResults := evaluateTrustChecks(cr.Evidence, trust, riskTier, taskChecksByKey, now)
	checkResultsByKey := map[string]TrustCheckResult{}
	for _, check := range checkResults {
		checkResultsByKey[check.Key] = check
	}
	requirements := make([]TrustRequirement, 0, len(checkResults)+len(taskAcceptanceRequirements))
	for _, check := range checkResults {
		satisfied := check.Status == policyTrustCheckStatusPass
		action := ""
		if !satisfied {
			action = fmt.Sprintf("Run required check %q (%s) and record a passing fresh result.", check.Key, check.Command)
		}
		requirements = append(requirements, newTrustRequirement(
			"check:"+check.Key,
			fmt.Sprintf("Required check %q is passing and fresh", check.Key),
			satisfied,
			check.Reason,
			action,
			"policy_check",
		))
	}
	for _, req := range taskAcceptanceRequirements {
		check, found := checkResultsByKey[req.Key]
		satisfied := found && check.Status == policyTrustCheckStatusPass
		reason := fmt.Sprintf("Check %q has no recorded status.", req.Key)
		if found {
			reason = check.Reason
		}
		action := ""
		if !satisfied {
			action = fmt.Sprintf("Task #%d requires check key %q to pass; run `sophia cr check run %d` or record fresh passing evidence.", req.TaskID, req.Key, cr.ID)
		}
		requirements = append(requirements, newTaskTrustRequirement(
			fmt.Sprintf("task:%d:check:%s", req.TaskID, req.Key),
			fmt.Sprintf("Task #%d acceptance check %q is passing and fresh", req.TaskID, req.Key),
			satisfied,
			reason,
			action,
			req.TaskID,
			"task_acceptance_check",
		))
	}
	return requirements, checkResults
}

func buildReviewDepthRequirement(reviewDepth TrustReviewDepthResult) TrustRequirement {
	requirement := newTrustRequirement(
		"review_depth",
		"Review-depth sampling requirement is satisfied",
		reviewDepth.Satisfied,
		fmt.Sprintf("Samples: %d/%d.", reviewDepth.SampleCount, reviewDepth.RequiredSamples),
		"",
		"review_depth_policy",
	)
	if !reviewDepth.Satisfied {
		requirement.Action = fmt.Sprintf("Add review_sample evidence entries until at least %d sample(s) are recorded.", reviewDepth.RequiredSamples)
	}
	if reviewDepth.RequireCriticalScopeCoverage && len(reviewDepth.MissingCriticalScopes) > 0 {
		requirement.Reason = fmt.Sprintf("Missing critical scope coverage for: %s.", strings.Join(reviewDepth.MissingCriticalScopes, ", "))
		if requirement.Action == "" {
			requirement.Action = "Add review_sample evidence with scope prefixes covering each risk_critical_scope."
		}
	}
	return requirement
}

func buildContractDriftRequirement(contractDrift TaskContractDriftSummary, crID int) TrustRequirement {
	requirement := newTrustRequirement(
		"contract_drift_acknowledged",
		"Task contract drift records are acknowledged",
		contractDrift.Unacknowledged == 0,
		"No unacknowledged task contract drift records.",
		"",
		"contract_drift",
	)
	if contractDrift.Unacknowledged > 0 {
		requirement.Reason = fmt.Sprintf("%d unacknowledged drift record(s) remain across task(s): %s.", contractDrift.Unacknowledged, formatTaskIDList(contractDrift.UnacknowledgedTasks))
		requirement.Action = fmt.Sprintf("Acknowledge drift records with `sophia cr task contract drift ack %d <task-id> <drift-id> --reason \"...\"`.", crID)
	}
	return requirement
}

func buildCRContractDriftRequirement(contractDrift CRContractDriftSummary, crID int) TrustRequirement {
	requirement := newTrustRequirement(
		"cr_contract_drift_acknowledged",
		"CR contract drift records are acknowledged",
		contractDrift.Unacknowledged == 0,
		"No unacknowledged CR contract drift records.",
		"",
		"cr_contract_drift",
	)
	if contractDrift.Unacknowledged > 0 {
		requirement.Reason = fmt.Sprintf("%d unacknowledged CR contract drift record(s) remain: %s.", contractDrift.Unacknowledged, formatDriftIDList(contractDrift.UnacknowledgedDriftIDs))
		requirement.Action = fmt.Sprintf("Acknowledge CR drift records with `sophia cr contract drift ack %d <drift-id> --reason \"...\"`.", crID)
	}
	return requirement
}

func appendRiskTierAdvisories(advisories []string, impact *ImpactReport, reviewDepth TrustReviewDepthResult, contract model.Contract, diff *diffSummary) []string {
	if strings.EqualFold(strings.TrimSpace(impact.RiskTier), "high") && len(impact.MatchedRiskCriticalScopes) > 0 {
		advisories = append(advisories, fmt.Sprintf("Spot-check critical scopes: %s.", strings.Join(impact.MatchedRiskCriticalScopes, ", ")))
	}
	if reviewDepth.RequireCriticalScopeCoverage && len(contract.RiskCriticalScopes) == 0 {
		advisories = append(advisories, "Declare risk_critical_scopes in the CR contract to enforce high-tier critical-scope coverage checks.")
	}
	if strings.EqualFold(strings.TrimSpace(impact.RiskTier), "high") &&
		len(impact.MatchedRiskCriticalScopes) > 0 &&
		!hasSpecializedHighRiskEvidence(impact, diff) {
		advisories = append(advisories, "Add specialized high-risk evidence (integration/worktree/doctor/repair coverage) to increase confidence.")
	}
	return dedupeStrings(advisories)
}

func collectRequiredActions(requirements []TrustRequirement) []string {
	actions := []string{}
	for _, requirement := range requirements {
		if requirement.Satisfied || strings.TrimSpace(requirement.Action) == "" {
			continue
		}
		actions = append(actions, requirement.Action)
	}
	return dedupeStrings(actions)
}

func selectTrustVerdict(score, max int, hardFailures []string) (string, string) {
	ratio := trustScoreRatio(score, max)
	switch {
	case len(hardFailures) > 0 || ratio < trustAttentionMinRatio:
		return trustVerdictUntrusted, "Trust evidence is insufficient; perform deeper review and resolve required actions."
	case ratio >= trustTrustedMinRatio:
		return trustVerdictTrusted, "Trust evidence is strong; diff deep-dive can be optional."
	default:
		return trustVerdictNeedsAttention, "Trust evidence has gaps; address required actions before treating diffs as optional."
	}
}

func selectTrustVerdictForPolicy(score, max int, hardFailures []string, requirements []TrustRequirement, trust model.PolicyTrust, riskTier string) (string, string) {
	if len(hardFailures) > 0 {
		return trustVerdictUntrusted, "Trust evidence is insufficient; validation/contract requirements are failing."
	}
	for _, requirement := range requirements {
		if !requirement.Satisfied {
			return trustVerdictUntrusted, "Trust evidence requirements are unsatisfied; complete required actions before treating review as trusted."
		}
	}
	ratio := trustScoreRatio(score, max)
	threshold := trustThresholdForTier(trust, riskTier)
	if ratio >= threshold {
		return trustVerdictTrusted, "Trust evidence is strong and policy requirements are satisfied."
	}
	return trustVerdictNeedsAttention, "Trust requirements are satisfied, but confidence score is below the trust threshold."
}

func trustRequirementsSatisfied(requirements []TrustRequirement) bool {
	for _, requirement := range requirements {
		if !requirement.Satisfied {
			return false
		}
	}
	return true
}

func trustScoreRatio(score, max int) float64 {
	return servicetrust.TrustScoreRatio(score, max)
}

func isTrustGatingAction(action string) bool {
	lower := strings.ToLower(strings.TrimSpace(action))
	if lower == "" {
		return false
	}
	if strings.Contains(lower, "validation error") {
		return true
	}
	if strings.Contains(lower, "required contract field") {
		return true
	}
	if strings.Contains(lower, "required contract fields") {
		return true
	}
	return false
}

func trustThresholdForTier(trust model.PolicyTrust, riskTier string) float64 {
	return servicetrust.TrustThresholdForTier(trust, riskTier, defaultTrustThresholdLow, defaultTrustThresholdMedium, defaultTrustThresholdHigh)
}

func normalizedRiskTier(raw string) string {
	return servicetrust.NormalizeRiskTier(raw)
}

func evaluateTrustChecks(evidence []model.EvidenceEntry, trust model.PolicyTrust, riskTier string, taskRequired map[string][]int, now time.Time) []TrustCheckResult {
	required := requiredTrustCheckDefinitions(trust.Checks.Definitions, riskTier)
	riskRequiredKeys := map[string]struct{}{}
	for _, definition := range required {
		riskRequiredKeys[strings.TrimSpace(definition.Key)] = struct{}{}
	}
	definitionsByKey := map[string]model.PolicyTrustCheckDefinition{}
	for _, definition := range trust.Checks.Definitions {
		key := strings.TrimSpace(definition.Key)
		if key == "" {
			continue
		}
		definitionsByKey[key] = definition
	}
	for key := range taskRequired {
		if _, exists := definitionsByKey[key]; !exists {
			continue
		}
		alreadyRequired := false
		for _, current := range required {
			if strings.TrimSpace(current.Key) == key {
				alreadyRequired = true
				break
			}
		}
		if alreadyRequired {
			continue
		}
		required = append(required, definitionsByKey[key])
	}
	sort.Slice(required, func(i, j int) bool {
		return required[i].Key < required[j].Key
	})
	freshnessHours := intValueOrDefault(trust.Checks.FreshnessHours, defaultTrustCheckFreshnessHours)
	results := make([]TrustCheckResult, 0, len(required))
	for _, definition := range required {
		result := evaluateTrustCheckResult(evidence, definition, freshnessHours, now)
		key := strings.TrimSpace(definition.Key)
		result.RequiredByTaskIDs = normalizeIntList(taskRequired[key])
		_, riskRequired := riskRequiredKeys[key]
		taskRequiredFlag := len(result.RequiredByTaskIDs) > 0
		switch {
		case riskRequired && taskRequiredFlag:
			result.Sources = []string{"risk_tier_policy", "task_acceptance_check"}
		case riskRequired:
			result.Sources = []string{"risk_tier_policy"}
		case taskRequiredFlag:
			result.Sources = []string{"task_acceptance_check"}
		default:
			result.Sources = []string{}
		}
		results = append(results, result)
	}
	return results
}

func requiredTrustCheckDefinitions(definitions []model.PolicyTrustCheckDefinition, riskTier string) []model.PolicyTrustCheckDefinition {
	tier := normalizedRiskTier(riskTier)
	required := []model.PolicyTrustCheckDefinition{}
	for _, definition := range definitions {
		if stringSliceContains(definition.Tiers, tier) {
			required = append(required, definition)
		}
	}
	return required
}

func evaluateTrustCheckResult(evidence []model.EvidenceEntry, definition model.PolicyTrustCheckDefinition, freshnessHours int, now time.Time) TrustCheckResult {
	result := TrustCheckResult{
		Key:            definition.Key,
		Command:        definition.Command,
		Required:       true,
		Status:         policyTrustCheckStatusMissing,
		Reason:         "No command_run evidence found.",
		AllowExitCodes: append([]int(nil), definition.AllowExitCodes...),
		FreshnessHours: freshnessHours,
	}
	entry, found := latestTrustCheckEvidence(evidence, definition.Command)
	if !found {
		return result
	}
	result.LastRunAt = strings.TrimSpace(entry.TS)
	if entry.ExitCode != nil {
		exit := *entry.ExitCode
		result.ExitCode = &exit
	}
	entryTime := parseRFC3339OrZero(entry.TS)
	if entryTime.IsZero() {
		result.Status = policyTrustCheckStatusStale
		result.Reason = "Latest evidence timestamp is invalid."
		return result
	}
	if now.Sub(entryTime.UTC()) > time.Duration(freshnessHours)*time.Hour {
		result.Status = policyTrustCheckStatusStale
		result.Reason = fmt.Sprintf("Latest check run is older than %d hour(s).", freshnessHours)
		return result
	}
	if entry.ExitCode == nil {
		result.Status = policyTrustCheckStatusFail
		result.Reason = "Latest check run is missing exit code."
		return result
	}
	if containsInt(definition.AllowExitCodes, *entry.ExitCode) {
		result.Status = policyTrustCheckStatusPass
		result.Reason = fmt.Sprintf("Latest check run passed with exit code %d.", *entry.ExitCode)
		return result
	}
	result.Status = policyTrustCheckStatusFail
	result.Reason = fmt.Sprintf("Latest check run exit code %d is not allowed.", *entry.ExitCode)
	return result
}

func latestTrustCheckEvidence(evidence []model.EvidenceEntry, command string) (model.EvidenceEntry, bool) {
	var (
		best     model.EvidenceEntry
		bestTime time.Time
		found    bool
	)
	for _, entry := range evidence {
		if strings.TrimSpace(entry.Type) != evidenceTypeCommandRun {
			continue
		}
		if strings.TrimSpace(entry.Command) != strings.TrimSpace(command) {
			continue
		}
		current := parseRFC3339OrZero(entry.TS)
		if !found || current.After(bestTime) {
			best = entry
			bestTime = current
			found = true
		}
	}
	return best, found
}

func evaluateTrustReviewDepth(cr *model.CR, trust model.PolicyTrust, riskTier string) TrustReviewDepthResult {
	out := TrustReviewDepthResult{
		RiskTier: normalizedRiskTier(riskTier),
	}
	reviewTier := trust.ReviewDepth.Low
	switch out.RiskTier {
	case "high":
		reviewTier = trust.ReviewDepth.High
	case "medium":
		reviewTier = trust.ReviewDepth.Medium
	}

	out.RequiredSamples = intValueOrDefault(reviewTier.MinSamples, 0)
	out.RequireCriticalScopeCoverage = boolValueOrDefault(reviewTier.RequireCriticalScopeCoverage, false)
	sampleScopes := []string{}
	for _, entry := range cr.Evidence {
		if strings.TrimSpace(entry.Type) != evidenceTypeReviewSample {
			continue
		}
		out.SampleCount++
		scope := strings.TrimSpace(entry.Scope)
		if scope != "" {
			sampleScopes = append(sampleScopes, scope)
		}
	}
	out.Satisfied = out.SampleCount >= out.RequiredSamples
	if out.RequireCriticalScopeCoverage && len(cr.Contract.RiskCriticalScopes) > 0 {
		covered := []string{}
		missing := []string{}
		for _, critical := range cr.Contract.RiskCriticalScopes {
			if sampleScopesMatchCriticalScope(sampleScopes, critical) {
				covered = append(covered, critical)
				continue
			}
			missing = append(missing, critical)
		}
		out.CoveredCriticalScopes = dedupeStrings(covered)
		out.MissingCriticalScopes = dedupeStrings(missing)
		out.Satisfied = out.Satisfied && len(out.MissingCriticalScopes) == 0
	}
	return out
}

func sampleScopesMatchCriticalScope(sampleScopes []string, criticalScope string) bool {
	return servicetrust.SampleScopesMatchCriticalScope(sampleScopes, criticalScope, pathMatchesScopePrefix)
}

func buildTrustGateSummary(trust model.PolicyTrust, riskTier, verdict string) TrustGateSummary {
	summary := TrustGateSummary{
		Enabled: false,
		Applies: false,
		Blocked: false,
		Reason:  "Trust gate is disabled.",
	}
	if !policyTrustGateEnabled(trust) {
		return summary
	}
	summary.Enabled = true
	if !trustGateAppliesToRiskTier(trust, riskTier) {
		summary.Reason = fmt.Sprintf("Trust gate enabled but not configured for risk tier %q.", normalizedRiskTier(riskTier))
		return summary
	}
	summary.Applies = true
	minVerdict := strings.TrimSpace(trust.Gate.MinVerdict)
	if minVerdict == "" {
		minVerdict = trustVerdictTrusted
	}
	if trustVerdictRank(verdict) < trustVerdictRank(minVerdict) {
		summary.Blocked = true
		summary.Reason = fmt.Sprintf("Trust gate blocked merge: verdict %q is below minimum %q.", verdict, minVerdict)
		return summary
	}
	summary.Reason = "Trust gate satisfied."
	return summary
}

func policyTrustGateEnabled(trust model.PolicyTrust) bool {
	return strings.EqualFold(strings.TrimSpace(trust.Mode), policyTrustModeGate) && servicetrust.BoolValueOrDefault(trust.Gate.Enabled, false)
}

func trustGateAppliesToRiskTier(trust model.PolicyTrust, riskTier string) bool {
	return servicetrust.TrustGateAppliesToRiskTier(trust.Gate.ApplyRiskTiers, defaultTrustGateApplyRiskTiers, riskTier)
}

func trustVerdictRank(verdict string) int {
	return servicetrust.TrustVerdictRank(verdict, trustVerdictTrusted, trustVerdictNeedsAttention)
}

func boolValueOrDefault(value *bool, fallback bool) bool {
	return servicetrust.BoolValueOrDefault(value, fallback)
}

func intValueOrDefault(value *int, fallback int) int {
	return servicetrust.IntValueOrDefault(value, fallback)
}

func containsInt(values []int, target int) bool {
	return servicetrust.ContainsInt(values, target)
}

func stringSliceContains(values []string, target string) bool {
	return servicetrust.StringSliceContains(values, target)
}

type shortStatMetrics = servicetrust.ShortStatMetrics

func parseShortStatMetrics(shortStat string) shortStatMetrics {
	return servicetrust.ParseShortStatMetrics(shortStat)
}

func effectiveFilesChanged(impact *ImpactReport, diff *diffSummary, shortStat shortStatMetrics) int {
	impactFilesChanged := 0
	if impact != nil {
		impactFilesChanged = impact.FilesChanged
	}
	diffFilesChanged := 0
	if diff != nil {
		diffFilesChanged = len(diff.Files)
	}
	return servicetrust.EffectiveFilesChanged(impactFilesChanged, diffFilesChanged, shortStat)
}

func buildContractQualityDimension(contract model.Contract) TrustDimension {
	dimension := TrustDimension{
		Code:            "contract_quality",
		Label:           "Contract Completeness",
		Score:           20,
		Max:             20,
		Reasons:         []string{},
		RequiredActions: []string{},
	}
	checks := []struct {
		label string
		value string
	}{
		{label: "why", value: contract.Why},
		{label: "blast_radius", value: contract.BlastRadius},
		{label: "test_plan", value: contract.TestPlan},
		{label: "rollback_plan", value: contract.RollbackPlan},
	}
	for _, check := range checks {
		if isWeakTrustText(check.value) {
			dimension.Score -= 3
			dimension.Reasons = append(dimension.Reasons, fmt.Sprintf("%s is weak", check.label))
			dimension.RequiredActions = append(dimension.RequiredActions, fmt.Sprintf("Strengthen contract field %q with concrete, non-placeholder detail.", check.label))
		}
	}
	if len(normalizeNonEmptyStringList(contract.NonGoals)) == 0 {
		dimension.Score -= 2
		dimension.Reasons = append(dimension.Reasons, "non_goals missing")
		dimension.RequiredActions = append(dimension.RequiredActions, "Add at least one explicit non-goal to bound intent.")
	}
	if len(normalizeNonEmptyStringList(contract.Invariants)) == 0 {
		dimension.Score -= 2
		dimension.Reasons = append(dimension.Reasons, "invariants missing")
		dimension.RequiredActions = append(dimension.RequiredActions, "Add at least one invariant that must remain true.")
	}
	return dimension
}

func buildScopeDisciplineDimension(impact *ImpactReport) TrustDimension {
	dimension := TrustDimension{
		Code:            "scope_discipline",
		Label:           "Scope Alignment",
		Score:           20,
		Max:             20,
		Reasons:         []string{},
		RequiredActions: []string{},
	}
	if impact == nil {
		return dimension
	}

	scopePenalty := minInt(12, 3*len(impact.ScopeDrift))
	if scopePenalty > 0 {
		dimension.Score -= scopePenalty
		dimension.Reasons = append(dimension.Reasons, fmt.Sprintf("%d scope drift file(s)", len(impact.ScopeDrift)))
		dimension.RequiredActions = append(dimension.RequiredActions, "Align changed files with declared contract scope or update scope intentionally.")
	}
	taskScopePenalty := minInt(4, 2*len(impact.TaskScopeWarnings))
	if taskScopePenalty > 0 {
		dimension.Score -= taskScopePenalty
		dimension.Reasons = append(dimension.Reasons, fmt.Sprintf("%d task scope warning(s)", len(impact.TaskScopeWarnings)))
		dimension.RequiredActions = append(dimension.RequiredActions, "Fix task checkpoint scope warnings by narrowing staged paths or task scopes.")
	}
	taskContractPenalty := minInt(4, 2*len(impact.TaskContractWarnings))
	if taskContractPenalty > 0 {
		dimension.Score -= taskContractPenalty
		dimension.Reasons = append(dimension.Reasons, fmt.Sprintf("%d task contract warning(s)", len(impact.TaskContractWarnings)))
		dimension.RequiredActions = append(dimension.RequiredActions, "Resolve task contract drift between checkpoint scope and task contract scope.")
	}
	return dimension
}

func buildTaskProofChainDimension(tasks []model.Subtask) TrustDimension {
	dimension := TrustDimension{
		Code:            "task_proof_chain",
		Label:           "Checkpoint Coverage",
		Score:           20,
		Max:             20,
		Reasons:         []string{},
		RequiredActions: []string{},
	}
	if len(tasks) == 0 {
		dimension.Score = 12
		dimension.Reasons = append(dimension.Reasons, "no explicit tasks")
		dimension.RequiredActions = append(dimension.RequiredActions, "Add task contracts and scoped task checkpoints to strengthen evidence chain.")
		return dimension
	}

	tasksDone := 0
	unjustifiedMissingCheckpoints := 0
	delegatedPending := false
	for _, task := range tasks {
		if task.Status == model.TaskStatusDone {
			tasksDone++
			if strings.TrimSpace(task.CheckpointCommit) == "" && strings.TrimSpace(task.CheckpointReason) == "" {
				unjustifiedMissingCheckpoints++
			}
		}
		if task.Status == model.TaskStatusDelegated {
			delegatedPending = true
		}
	}
	if tasksDone == 0 {
		dimension.Score -= 4
		dimension.Reasons = append(dimension.Reasons, "no completed tasks")
		dimension.RequiredActions = append(dimension.RequiredActions, "Complete at least one scoped task checkpoint to prove implementation progress.")
	}
	checkpointPenalty := minInt(8, 2*unjustifiedMissingCheckpoints)
	if checkpointPenalty > 0 {
		dimension.Score -= checkpointPenalty
		dimension.Reasons = append(dimension.Reasons, fmt.Sprintf("%d done task(s) missing checkpoint commit without rationale", unjustifiedMissingCheckpoints))
		dimension.RequiredActions = append(dimension.RequiredActions, "Add checkpoint commits (or explicit rationale) for done tasks missing proof commits.")
	}
	if delegatedPending {
		dimension.Score -= 3
		dimension.Reasons = append(dimension.Reasons, "delegated tasks still pending")
		dimension.RequiredActions = append(dimension.RequiredActions, "Resolve pending delegated tasks before relying on trust verdict.")
	}
	return dimension
}

func buildRiskAccountabilityDimension(contract model.Contract, impact *ImpactReport, diff *diffSummary) TrustDimension {
	dimension := TrustDimension{
		Code:            "risk_accountability",
		Label:           "Risk Declaration",
		Score:           15,
		Max:             15,
		Reasons:         []string{},
		RequiredActions: []string{},
	}
	riskTierHint := strings.TrimSpace(contract.RiskTierHint)
	if riskTierHint != "" && strings.TrimSpace(contract.RiskRationale) == "" {
		dimension.Score -= 4
		dimension.Reasons = append(dimension.Reasons, "risk_tier_hint set without risk_rationale")
		dimension.RequiredActions = append(dimension.RequiredActions, "Add risk_rationale when setting risk_tier_hint.")
	}
	if impact == nil {
		return dimension
	}
	if strings.EqualFold(strings.TrimSpace(impact.RiskTier), "high") && !hasDependencyOrTestEvidence(impact, diff) {
		dimension.Score -= 3
		dimension.Reasons = append(dimension.Reasons, "high risk tier lacks dependency/test evidence")
		dimension.RequiredActions = append(dimension.RequiredActions, "Document or include dependency/test evidence supporting high-risk changes.")
	}
	filesChanged := effectiveFilesChanged(impact, diff, shortStatMetrics{})
	if filesChanged > 0 && len(impact.Signals) == 0 {
		dimension.Score -= 2
		dimension.Reasons = append(dimension.Reasons, "files changed but no risk signals")
		dimension.RequiredActions = append(dimension.RequiredActions, "Clarify risk-critical scopes/rationale so risk signals reflect actual change impact.")
	}
	return dimension
}

func buildValidationHealthDimension(validation *ValidationReport) TrustDimension {
	dimension := TrustDimension{
		Code:            "validation_health",
		Label:           "Validation Status",
		Score:           15,
		Max:             15,
		Reasons:         []string{},
		RequiredActions: []string{},
	}
	if validation == nil {
		return dimension
	}
	if len(validation.Errors) > 0 {
		dimension.Score = 0
		dimension.Reasons = append(dimension.Reasons, fmt.Sprintf("%d validation error(s)", len(validation.Errors)))
		dimension.RequiredActions = append(dimension.RequiredActions, "Resolve validation errors before treating CR metadata as trusted evidence.")
		return dimension
	}
	warningPenalty := minInt(10, 2*len(validation.Warnings))
	if warningPenalty > 0 {
		dimension.Score -= warningPenalty
		dimension.Reasons = append(dimension.Reasons, fmt.Sprintf("%d validation warning(s)", len(validation.Warnings)))
		dimension.RequiredActions = append(dimension.RequiredActions, "Reduce validation warnings to improve trust confidence.")
	}
	return dimension
}

func buildChangeMagnitudeDimension(impact *ImpactReport, shortStat shortStatMetrics) TrustDimension {
	dimension := TrustDimension{
		Code:            "change_magnitude",
		Label:           "Change Magnitude",
		Score:           10,
		Max:             10,
		Reasons:         []string{},
		RequiredActions: []string{},
	}
	riskTier := ""
	if impact != nil {
		riskTier = strings.TrimSpace(impact.RiskTier)
	}
	filesChanged := effectiveFilesChanged(impact, nil, shortStat)
	if filesChanged >= 15 {
		dimension.Score -= 2
		dimension.Reasons = append(dimension.Reasons, fmt.Sprintf("large file surface (%d files changed)", filesChanged))
		dimension.RequiredActions = append(dimension.RequiredActions, "Split or justify broad change surface for reviewer confidence.")
	}
	if filesChanged >= 25 {
		dimension.Score -= 2
		dimension.Reasons = append(dimension.Reasons, "very large file surface (>=25 files changed)")
		dimension.RequiredActions = append(dimension.RequiredActions, "Consider splitting intent into stacked CRs for reviewability.")
	}
	if shortStat.Insertions >= 500 {
		dimension.Score -= 2
		dimension.Reasons = append(dimension.Reasons, fmt.Sprintf("high insertion volume (%d)", shortStat.Insertions))
		dimension.RequiredActions = append(dimension.RequiredActions, "Provide explicit rationale for high insertion volume.")
	}
	if shortStat.Deletions >= 200 {
		dimension.Score -= 1
		dimension.Reasons = append(dimension.Reasons, fmt.Sprintf("high deletion volume (%d)", shortStat.Deletions))
		dimension.RequiredActions = append(dimension.RequiredActions, "Call out rollback considerations for high deletion volume.")
	}
	if strings.EqualFold(riskTier, "high") && filesChanged >= 15 {
		dimension.Score -= 2
		dimension.Reasons = append(dimension.Reasons, "high-risk tier with broad change surface")
		dimension.RequiredActions = append(dimension.RequiredActions, "Add focused reviewer notes for high-risk broad-surface changes.")
	}
	return dimension
}

func buildTestEvidenceDimension(contract model.Contract, diff *diffSummary) TrustDimension {
	dimension := TrustDimension{
		Code:            "test_evidence",
		Label:           "Test Touch Signals",
		Score:           10,
		Max:             10,
		Reasons:         []string{},
		RequiredActions: []string{},
	}
	if diff == nil {
		diff = &diffSummary{}
	}
	if isWeakTrustText(contract.TestPlan) {
		dimension.Score -= 4
		dimension.Reasons = append(dimension.Reasons, "test_plan is weak or missing")
		dimension.RequiredActions = append(dimension.RequiredActions, "Provide a concrete test_plan for the declared blast radius.")
	}
	nonTestChanges := len(diff.Files) > len(diff.TestFiles)
	if nonTestChanges && len(diff.TestFiles) == 0 {
		dimension.Score -= 4
		dimension.Reasons = append(dimension.Reasons, "non-test changes without test file updates")
		dimension.RequiredActions = append(dimension.RequiredActions, "Add or update tests for non-test code changes, or document why tests are unchanged.")
	}
	if len(diff.DependencyFiles) > 0 && len(diff.TestFiles) == 0 {
		dimension.Score -= 2
		dimension.Reasons = append(dimension.Reasons, "dependency changes without test evidence")
		dimension.RequiredActions = append(dimension.RequiredActions, "Provide test evidence when dependency files change.")
	}
	return dimension
}

func hasSpecializedHighRiskEvidence(impact *ImpactReport, diff *diffSummary) bool {
	testPaths := map[string]struct{}{}
	if impact != nil {
		for _, path := range impact.TestFiles {
			trimmed := strings.TrimSpace(path)
			if trimmed != "" {
				testPaths[trimmed] = struct{}{}
			}
		}
	}
	if diff != nil {
		for _, path := range diff.TestFiles {
			trimmed := strings.TrimSpace(path)
			if trimmed != "" {
				testPaths[trimmed] = struct{}{}
			}
		}
	}
	for path := range testPaths {
		normalized := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(path), "\\", "/"))
		switch {
		case strings.Contains(normalized, "/integration/"):
			return true
		case strings.HasSuffix(normalized, "_integration_test.go"):
			return true
		case strings.Contains(normalized, "worktree"):
			return true
		case strings.Contains(normalized, "doctor"):
			return true
		case strings.Contains(normalized, "repair"):
			return true
		}
	}
	return false
}

func hasDependencyOrTestEvidence(impact *ImpactReport, diff *diffSummary) bool {
	if impact != nil {
		if len(impact.DependencyFiles) > 0 || len(impact.TestFiles) > 0 {
			return true
		}
		for _, signal := range impact.Signals {
			if signal.Code == "dependency_changes" {
				return true
			}
		}
	}
	if diff != nil {
		return len(diff.DependencyFiles) > 0 || len(diff.TestFiles) > 0
	}
	return false
}

func isWeakTrustText(value string) bool {
	return servicetrust.IsWeakTrustText(value)
}

type taskAcceptanceCheckRequirement struct {
	TaskID int
	Key    string
}

func listUnjustifiedDoneTasksWithoutCheckpoint(tasks []model.Subtask) []int {
	out := []int{}
	for _, task := range tasks {
		if task.Status != model.TaskStatusDone {
			continue
		}
		if strings.TrimSpace(task.CheckpointCommit) != "" {
			continue
		}
		if strings.TrimSpace(task.CheckpointReason) != "" {
			continue
		}
		out = append(out, task.ID)
	}
	sort.Ints(out)
	return out
}

func requiredTaskAcceptanceChecks(tasks []model.Subtask) []taskAcceptanceCheckRequirement {
	out := []taskAcceptanceCheckRequirement{}
	seen := map[string]struct{}{}
	for _, task := range tasks {
		if task.Status != model.TaskStatusDone {
			continue
		}
		for _, key := range servicetasks.NormalizeAcceptanceCheckKeys(task.Contract.AcceptanceChecks) {
			marker := fmt.Sprintf("%d:%s", task.ID, key)
			if _, ok := seen[marker]; ok {
				continue
			}
			seen[marker] = struct{}{}
			out = append(out, taskAcceptanceCheckRequirement{TaskID: task.ID, Key: key})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].TaskID == out[j].TaskID {
			return out[i].Key < out[j].Key
		}
		return out[i].TaskID < out[j].TaskID
	})
	return out
}

func taskAcceptanceCheckTaskMap(requirements []taskAcceptanceCheckRequirement) map[string][]int {
	out := map[string][]int{}
	for _, requirement := range requirements {
		out[requirement.Key] = append(out[requirement.Key], requirement.TaskID)
	}
	for key, ids := range out {
		out[key] = normalizeIntList(ids)
	}
	return out
}

func summarizeTaskContractDrift(tasks []model.Subtask) TaskContractDriftSummary {
	summary := TaskContractDriftSummary{
		TasksWithDrift:      []int{},
		UnacknowledgedTasks: []int{},
	}
	tasksWithDrift := map[int]struct{}{}
	unackedTasks := map[int]struct{}{}
	for _, task := range tasks {
		if len(task.ContractDrifts) == 0 {
			continue
		}
		tasksWithDrift[task.ID] = struct{}{}
		for _, drift := range task.ContractDrifts {
			summary.Total++
			if drift.Acknowledged {
				continue
			}
			summary.Unacknowledged++
			unackedTasks[task.ID] = struct{}{}
		}
	}
	for taskID := range tasksWithDrift {
		summary.TasksWithDrift = append(summary.TasksWithDrift, taskID)
	}
	for taskID := range unackedTasks {
		summary.UnacknowledgedTasks = append(summary.UnacknowledgedTasks, taskID)
	}
	sort.Ints(summary.TasksWithDrift)
	sort.Ints(summary.UnacknowledgedTasks)
	return summary
}

func formatTaskIDList(taskIDs []int) string {
	if len(taskIDs) == 0 {
		return "(none)"
	}
	parts := make([]string, 0, len(taskIDs))
	for _, taskID := range normalizeIntList(taskIDs) {
		parts = append(parts, strconv.Itoa(taskID))
	}
	return strings.Join(parts, ", ")
}

func formatDriftIDList(driftIDs []int) string {
	if len(driftIDs) == 0 {
		return "(none)"
	}
	parts := make([]string, 0, len(driftIDs))
	for _, driftID := range normalizeIntList(driftIDs) {
		parts = append(parts, strconv.Itoa(driftID))
	}
	return strings.Join(parts, ", ")
}

func clampMin(value, min int) int {
	if value < min {
		return min
	}
	return value
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
