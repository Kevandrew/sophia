package service

import (
	"fmt"
	"sophia/internal/model"
	"strings"
)

const (
	trustVerdictTrusted        = "trusted"
	trustVerdictNeedsAttention = "needs_attention"
	trustVerdictUntrusted      = "untrusted"
)

func buildTrustReport(cr *model.CR, validation *ValidationReport, diff *diffSummary) *TrustReport {
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
			Summary: "Trust evidence unavailable because CR metadata is missing.",
		}
	}
	if validation == nil {
		validation = &ValidationReport{}
	}
	if diff == nil {
		diff = &diffSummary{}
	}
	impact := validation.Impact
	if impact == nil {
		impact = &ImpactReport{FilesChanged: len(diff.Files)}
	}

	hardFailures := []string{}
	requiredActions := []string{}
	if len(validation.Errors) > 0 {
		hardFailures = append(hardFailures, fmt.Sprintf("validation errors present (%d)", len(validation.Errors)))
		requiredActions = append(requiredActions, "Resolve all validation errors before trusting review data.")
	}
	if missing := missingCRContractFields(cr.Contract); len(missing) > 0 {
		hardFailures = append(hardFailures, fmt.Sprintf("missing required contract fields: %s", strings.Join(missing, ", ")))
		requiredActions = append(requiredActions, fmt.Sprintf("Complete required contract fields: %s.", strings.Join(missing, ", ")))
	}

	dimensions := []TrustDimension{
		buildContractQualityDimension(cr.Contract),
		buildScopeDisciplineDimension(impact),
		buildTaskProofChainDimension(cr.Subtasks),
		buildRiskAccountabilityDimension(cr.Contract, impact, diff),
		buildValidationHealthDimension(validation),
		buildTestEvidenceDimension(cr.Contract, diff),
	}

	score := 0
	max := 0
	for i := range dimensions {
		dimensions[i].Score = clampMin(dimensions[i].Score, 0)
		if dimensions[i].Score > dimensions[i].Max {
			dimensions[i].Score = dimensions[i].Max
		}
		dimensions[i].Reasons = dedupeStrings(dimensions[i].Reasons)
		dimensions[i].RequiredActions = dedupeStrings(dimensions[i].RequiredActions)
		score += dimensions[i].Score
		max += dimensions[i].Max
		requiredActions = append(requiredActions, dimensions[i].RequiredActions...)
	}
	requiredActions = dedupeStrings(requiredActions)

	verdict := trustVerdictNeedsAttention
	summary := "Trust evidence has gaps; address required actions before treating diffs as optional."
	switch {
	case len(hardFailures) > 0 || score < 60:
		verdict = trustVerdictUntrusted
		summary = "Trust evidence is insufficient; perform deeper review and resolve required actions."
	case score >= 85:
		verdict = trustVerdictTrusted
		summary = "Trust evidence is strong; diff deep-dive can be optional."
	}

	return &TrustReport{
		Verdict:         verdict,
		Score:           score,
		Max:             max,
		AdvisoryOnly:    true,
		HardFailures:    dedupeStrings(hardFailures),
		Dimensions:      dimensions,
		RequiredActions: requiredActions,
		Summary:         summary,
	}
}

func buildContractQualityDimension(contract model.Contract) TrustDimension {
	dimension := TrustDimension{
		Code:            "contract_quality",
		Label:           "Contract Quality",
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
		Label:           "Scope Discipline",
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
		Label:           "Task Proof Chain",
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
	missingCheckpoints := 0
	delegatedPending := false
	for _, task := range tasks {
		if task.Status == model.TaskStatusDone {
			tasksDone++
			if strings.TrimSpace(task.CheckpointCommit) == "" {
				missingCheckpoints++
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
	checkpointPenalty := minInt(8, 2*missingCheckpoints)
	if checkpointPenalty > 0 {
		dimension.Score -= checkpointPenalty
		dimension.Reasons = append(dimension.Reasons, fmt.Sprintf("%d done task(s) missing checkpoint commit", missingCheckpoints))
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
		Label:           "Risk Accountability",
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
	filesChanged := impact.FilesChanged
	if filesChanged == 0 && diff != nil {
		filesChanged = len(diff.Files)
	}
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
		Label:           "Validation Health",
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

func buildTestEvidenceDimension(contract model.Contract, diff *diffSummary) TrustDimension {
	dimension := TrustDimension{
		Code:            "test_evidence",
		Label:           "Test Evidence",
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
