package cli

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"sophia/internal/model"
	"sophia/internal/service"
)

func printStringListSection(cmd *cobra.Command, title string, items []string) {
	fmt.Fprintf(cmd.OutOrStdout(), "\n%s:\n", title)
	if len(items) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "- (none)")
		return
	}
	for _, item := range items {
		fmt.Fprintf(cmd.OutOrStdout(), "- %s\n", item)
	}
}

func printListSection(cmd *cobra.Command, title string, items []string) {
	printStringListSection(cmd, title, items)
}

func printStringSection(cmd *cobra.Command, title string, items []string) {
	printStringListSection(cmd, title, items)
}

func printValueList(cmd *cobra.Command, label string, values []string) {
	if len(values) == 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "- %s: (missing)\n", label)
		return
	}
	fmt.Fprintf(cmd.OutOrStdout(), "- %s:\n", label)
	for _, value := range values {
		fmt.Fprintf(cmd.OutOrStdout(), "  - %s\n", value)
	}
}

func printInlineList(cmd *cobra.Command, label string, values []string) {
	if len(values) == 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "- %s: (missing)\n", label)
		return
	}
	fmt.Fprintf(cmd.OutOrStdout(), "- %s: %s\n", label, strings.Join(values, ", "))
}

func printImpactSection(cmd *cobra.Command, impact *service.ImpactReport) {
	fmt.Fprintln(cmd.OutOrStdout(), "\nImpact:")
	if impact == nil {
		fmt.Fprintln(cmd.OutOrStdout(), "- (none)")
		return
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Risk Tier: %s\n", nonEmpty(strings.TrimSpace(impact.RiskTier), "-"))
	fmt.Fprintf(cmd.OutOrStdout(), "Risk Score: %d\n", impact.RiskScore)
	fmt.Fprintf(cmd.OutOrStdout(), "Files Changed: %d\n", impact.FilesChanged)
	fmt.Fprintf(cmd.OutOrStdout(), "Scope Source: %s\n", nonEmpty(strings.TrimSpace(impact.ScopeSource), "contract_scope"))
	printListSection(cmd, "Warnings", impact.Warnings)
	printListSection(cmd, "Scope Drift", impact.ScopeDrift)
	printListSection(cmd, "Task Scope Warnings", impact.TaskScopeWarnings)
	printListSection(cmd, "Task Contract Warnings", impact.TaskContractWarnings)
	printListSection(cmd, "Task Chunk Warnings", impact.TaskChunkWarnings)
	fmt.Fprintln(cmd.OutOrStdout(), "\nRisk Signals:")
	if len(impact.Signals) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "- (none)")
		return
	}
	for _, signal := range impact.Signals {
		fmt.Fprintf(cmd.OutOrStdout(), "- [%s] +%d %s\n", signal.Code, signal.Points, signal.Summary)
	}
}

func printTrustSection(cmd *cobra.Command, trust *service.TrustReport) {
	fmt.Fprintln(cmd.OutOrStdout(), "\nTrust:")
	if trust == nil {
		fmt.Fprintln(cmd.OutOrStdout(), "- (none)")
		return
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Verdict: %s\n", nonEmpty(strings.TrimSpace(trust.Verdict), "-"))
	fmt.Fprintf(cmd.OutOrStdout(), "Score: %d/%d\n", trust.Score, trust.Max)
	fmt.Fprintf(cmd.OutOrStdout(), "Risk Tier: %s\n", nonEmpty(strings.TrimSpace(trust.RiskTier), "-"))
	fmt.Fprintf(cmd.OutOrStdout(), "Advisory Only: %t\n", trust.AdvisoryOnly)
	printStringSection(cmd, "Hard Failures", trust.HardFailures)
	fmt.Fprintln(cmd.OutOrStdout(), "\nDimensions:")
	if len(trust.Dimensions) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "- (none)")
	} else {
		for _, dimension := range trust.Dimensions {
			label := nonEmpty(strings.TrimSpace(dimension.Label), dimension.Code)
			fmt.Fprintf(cmd.OutOrStdout(), "- [%s] %s: %d/%d\n", dimension.Code, label, dimension.Score, dimension.Max)
			if len(dimension.Reasons) > 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "  reasons: %s\n", strings.Join(dimension.Reasons, "; "))
			}
			if len(dimension.RequiredActions) > 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "  required_actions: %s\n", strings.Join(dimension.RequiredActions, "; "))
			}
		}
	}
	printStringSection(cmd, "Required Actions", trust.RequiredActions)
	printStringSection(cmd, "Attention Actions", trust.AttentionActions)
	printStringSection(cmd, "Advisories", trust.Advisories)
	printTrustRequirements(cmd, trust.Requirements)
	printTrustCheckResults(cmd, trust.CheckResults)
	fmt.Fprintln(cmd.OutOrStdout(), "\nReview Depth:")
	fmt.Fprintf(cmd.OutOrStdout(), "- risk_tier: %s\n", nonEmpty(strings.TrimSpace(trust.ReviewDepth.RiskTier), "-"))
	fmt.Fprintf(cmd.OutOrStdout(), "- required_samples: %d\n", trust.ReviewDepth.RequiredSamples)
	fmt.Fprintf(cmd.OutOrStdout(), "- sample_count: %d\n", trust.ReviewDepth.SampleCount)
	fmt.Fprintf(cmd.OutOrStdout(), "- require_critical_scope_coverage: %t\n", trust.ReviewDepth.RequireCriticalScopeCoverage)
	fmt.Fprintf(cmd.OutOrStdout(), "- satisfied: %t\n", trust.ReviewDepth.Satisfied)
	if len(trust.ReviewDepth.CoveredCriticalScopes) > 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "- covered_critical_scopes: %s\n", strings.Join(trust.ReviewDepth.CoveredCriticalScopes, ", "))
	}
	if len(trust.ReviewDepth.MissingCriticalScopes) > 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "- missing_critical_scopes: %s\n", strings.Join(trust.ReviewDepth.MissingCriticalScopes, ", "))
	}
	fmt.Fprintln(cmd.OutOrStdout(), "\nContract Drift:")
	fmt.Fprintf(cmd.OutOrStdout(), "- total: %d\n", trust.ContractDrift.Total)
	fmt.Fprintf(cmd.OutOrStdout(), "- unacknowledged: %d\n", trust.ContractDrift.Unacknowledged)
	if len(trust.ContractDrift.TasksWithDrift) > 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "- tasks_with_drift: %v\n", trust.ContractDrift.TasksWithDrift)
	}
	if len(trust.ContractDrift.UnacknowledgedTasks) > 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "- unacknowledged_tasks: %v\n", trust.ContractDrift.UnacknowledgedTasks)
	}
	fmt.Fprintln(cmd.OutOrStdout(), "\nCR Contract Drift:")
	fmt.Fprintf(cmd.OutOrStdout(), "- total: %d\n", trust.CRContractDrift.Total)
	fmt.Fprintf(cmd.OutOrStdout(), "- unacknowledged: %d\n", trust.CRContractDrift.Unacknowledged)
	if len(trust.CRContractDrift.DriftIDs) > 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "- drift_ids: %v\n", trust.CRContractDrift.DriftIDs)
	}
	if len(trust.CRContractDrift.UnacknowledgedDriftIDs) > 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "- unacknowledged_drift_ids: %v\n", trust.CRContractDrift.UnacknowledgedDriftIDs)
	}
	fmt.Fprintln(cmd.OutOrStdout(), "\nGate:")
	fmt.Fprintf(cmd.OutOrStdout(), "- enabled: %t\n", trust.Gate.Enabled)
	fmt.Fprintf(cmd.OutOrStdout(), "- applies: %t\n", trust.Gate.Applies)
	fmt.Fprintf(cmd.OutOrStdout(), "- blocked: %t\n", trust.Gate.Blocked)
	if strings.TrimSpace(trust.Gate.Reason) != "" {
		fmt.Fprintf(cmd.OutOrStdout(), "- reason: %s\n", trust.Gate.Reason)
	}
	if strings.TrimSpace(trust.Summary) != "" {
		fmt.Fprintf(cmd.OutOrStdout(), "\nSummary:\n%s\n", trust.Summary)
	}
}

func printTrustRequirements(cmd *cobra.Command, requirements []service.TrustRequirement) {
	fmt.Fprintln(cmd.OutOrStdout(), "\nRequirements:")
	if len(requirements) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "- (none)")
		return
	}
	for _, requirement := range requirements {
		status := "unsatisfied"
		if requirement.Satisfied {
			status = "satisfied"
		}
		fmt.Fprintf(cmd.OutOrStdout(), "- [%s] %s (%s)\n", requirement.Key, nonEmpty(strings.TrimSpace(requirement.Title), "-"), status)
		if strings.TrimSpace(requirement.Reason) != "" {
			fmt.Fprintf(cmd.OutOrStdout(), "  reason: %s\n", requirement.Reason)
		}
		if requirement.TaskID > 0 {
			fmt.Fprintf(cmd.OutOrStdout(), "  task_id: %d\n", requirement.TaskID)
		}
		if strings.TrimSpace(requirement.Source) != "" {
			fmt.Fprintf(cmd.OutOrStdout(), "  source: %s\n", requirement.Source)
		}
		if !requirement.Satisfied && strings.TrimSpace(requirement.Action) != "" {
			fmt.Fprintf(cmd.OutOrStdout(), "  action: %s\n", requirement.Action)
		}
	}
}

func printTrustCheckResults(cmd *cobra.Command, checks []service.TrustCheckResult) {
	fmt.Fprintln(cmd.OutOrStdout(), "\nCheck Results:")
	if len(checks) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "- (none)")
		return
	}
	for _, check := range checks {
		fmt.Fprintf(cmd.OutOrStdout(), "- [%s] %s -> %s\n", check.Key, nonEmpty(strings.TrimSpace(check.Command), "-"), nonEmpty(strings.TrimSpace(check.Status), "-"))
		if strings.TrimSpace(check.Reason) != "" {
			fmt.Fprintf(cmd.OutOrStdout(), "  reason: %s\n", check.Reason)
		}
		if strings.TrimSpace(check.LastRunAt) != "" {
			fmt.Fprintf(cmd.OutOrStdout(), "  last_run_at: %s\n", check.LastRunAt)
		}
		if check.ExitCode != nil {
			fmt.Fprintf(cmd.OutOrStdout(), "  exit_code: %d\n", *check.ExitCode)
		}
		if len(check.RequiredByTaskIDs) > 0 {
			fmt.Fprintf(cmd.OutOrStdout(), "  required_by_task_ids: %v\n", check.RequiredByTaskIDs)
		}
		if len(check.Sources) > 0 {
			fmt.Fprintf(cmd.OutOrStdout(), "  sources: %s\n", strings.Join(check.Sources, ", "))
		}
	}
}

func parsePositiveIntArg(raw string, name string) (int, error) {
	id, err := strconv.Atoi(raw)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("invalid %s %q", name, raw)
	}
	return id, nil
}

func parseIDAndService(cmd *cobra.Command, rawID string, argName string) (int, *service.Service, error) {
	id, err := parsePositiveIntArg(rawID, argName)
	if err != nil {
		return 0, nil, err
	}
	svc, err := newServiceForCmd(cmd)
	if err != nil {
		return 0, nil, err
	}
	return id, svc, nil
}

func parseOptionalCRIDAndService(cmd *cobra.Command, args []string, argName string) (int, *service.Service, error) {
	svc, err := newServiceForCmd(cmd)
	if err != nil {
		return 0, nil, err
	}
	if len(args) > 0 {
		id, resolveErr := resolveCRIDFromSelector(svc, args[0], argName)
		if resolveErr != nil {
			return 0, nil, resolveErr
		}
		return id, svc, nil
	}
	id, err := resolveCurrentCRID(svc)
	if err != nil {
		return 0, nil, err
	}
	return id, svc, nil
}

func parseOptionalCRTaskIDsAndService(cmd *cobra.Command, args []string, crArgName, taskArgName string) (int, int, *service.Service, error) {
	svc, err := newServiceForCmd(cmd)
	if err != nil {
		return 0, 0, nil, err
	}
	switch len(args) {
	case 1:
		taskID, parseErr := parsePositiveIntArg(args[0], taskArgName)
		if parseErr != nil {
			return 0, 0, nil, parseErr
		}
		crID, resolveErr := resolveCurrentCRID(svc)
		if resolveErr != nil {
			return 0, 0, nil, resolveErr
		}
		return crID, taskID, svc, nil
	case 2:
		crID, resolveErr := resolveCRIDFromSelector(svc, args[0], crArgName)
		if resolveErr != nil {
			return 0, 0, nil, resolveErr
		}
		taskID, parseErr := parsePositiveIntArg(args[1], taskArgName)
		if parseErr != nil {
			return 0, 0, nil, parseErr
		}
		return crID, taskID, svc, nil
	default:
		return 0, 0, nil, fmt.Errorf("invalid arguments: expected <task-id> or <cr-id|uid> <task-id>")
	}
}

func resolveCRIDFromSelector(svc *service.Service, rawSelector string, argName string) (int, error) {
	if svc == nil {
		return 0, fmt.Errorf("service is required to resolve %s", argName)
	}
	selector := strings.TrimSpace(rawSelector)
	if selector == "" {
		return 0, fmt.Errorf("invalid %s %q", argName, rawSelector)
	}
	id, err := svc.ResolveCRID(selector)
	if err != nil {
		return 0, err
	}
	if id <= 0 {
		return 0, fmt.Errorf("invalid %s %q", argName, rawSelector)
	}
	return id, nil
}

func resolveCurrentCRID(svc *service.Service) (int, error) {
	if svc == nil {
		return 0, fmt.Errorf("service is required to resolve current CR context")
	}
	ctx, err := svc.CurrentCR()
	if err != nil {
		if errorsIs(err, service.ErrNoActiveCRContext) {
			return 0, fmt.Errorf("no CR selector provided and current branch has no active CR context; pass <id|uid> or run `sophia cr switch <id>`: %w", err)
		}
		return 0, err
	}
	if ctx == nil || ctx.CR == nil || ctx.CR.ID <= 0 {
		return 0, fmt.Errorf("failed to resolve active CR from current branch")
	}
	return ctx.CR.ID, nil
}

func parseCRTaskIDsAndService(cmd *cobra.Command, rawCRID, rawTaskID string) (int, int, *service.Service, error) {
	crID, err := parsePositiveIntArg(rawCRID, "cr-id")
	if err != nil {
		return 0, 0, nil, err
	}
	taskID, err := parsePositiveIntArg(rawTaskID, "task-id")
	if err != nil {
		return 0, 0, nil, err
	}
	svc, err := newServiceForCmd(cmd)
	if err != nil {
		return 0, 0, nil, err
	}
	return crID, taskID, svc, nil
}

func nonEmpty(v, fallback string) string {
	if strings.TrimSpace(v) == "" {
		return fallback
	}
	return v
}

func errorsIs(err error, target error) bool {
	return errors.Is(err, target)
}

func normalizeCRStatusFilter(raw string) (string, error) {
	value := strings.ToLower(strings.TrimSpace(raw))
	if value == "" {
		return "", nil
	}
	switch value {
	case model.StatusInProgress, model.StatusMerged:
		return value, nil
	default:
		return "", fmt.Errorf("invalid --status %q (expected %s or %s)", raw, model.StatusInProgress, model.StatusMerged)
	}
}

func normalizeRiskTierFilter(raw string) (string, error) {
	value := strings.ToLower(strings.TrimSpace(raw))
	if value == "" {
		return "", nil
	}
	switch value {
	case "low", "medium", "high":
		return value, nil
	default:
		return "", fmt.Errorf("invalid --risk-tier %q (expected low, medium, or high)", raw)
	}
}

func commandUsesCRSelectorAsFirstArg(cmd *cobra.Command) bool {
	if cmd == nil {
		return false
	}
	fields := strings.Fields(strings.TrimSpace(cmd.Use))
	if len(fields) < 2 {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(fields[1])) {
	case "<id>", "<cr-id>", "[id]", "[cr-id]":
		return true
	default:
		return false
	}
}

func rewriteCRSelectorArg(cmd *cobra.Command, args []string) error {
	if cmd == nil || len(args) == 0 {
		return nil
	}
	if !commandUsesCRSelectorAsFirstArg(cmd) {
		return nil
	}
	raw := strings.TrimSpace(args[0])
	if raw == "" {
		return nil
	}
	if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
		return nil
	}
	svc, err := newServiceForCmd(cmd)
	if err != nil {
		return err
	}
	resolved, err := svc.ResolveCRID(raw)
	if err != nil {
		return err
	}
	args[0] = strconv.Itoa(resolved)
	return nil
}

func dedupeStringValues(values []string) []string {
	if len(values) == 0 {
		return []string{}
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		out = append(out, trimmed)
	}
	return out
}
