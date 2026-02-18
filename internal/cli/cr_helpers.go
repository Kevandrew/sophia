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

func printListSection(cmd *cobra.Command, title string, items []string) {
	fmt.Fprintf(cmd.OutOrStdout(), "\n%s:\n", title)
	if len(items) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "- (none)")
		return
	}
	for _, item := range items {
		fmt.Fprintf(cmd.OutOrStdout(), "- %s\n", item)
	}
}

func printStringSection(cmd *cobra.Command, title string, items []string) {
	fmt.Fprintf(cmd.OutOrStdout(), "\n%s:\n", title)
	if len(items) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "- (none)")
		return
	}
	for _, item := range items {
		fmt.Fprintf(cmd.OutOrStdout(), "- %s\n", item)
	}
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
	printStringSection(cmd, "Advisories", trust.Advisories)
	if strings.TrimSpace(trust.Summary) != "" {
		fmt.Fprintf(cmd.OutOrStdout(), "\nSummary:\n%s\n", trust.Summary)
	}
}

func parsePositiveIntArg(raw string, name string) (int, error) {
	id, err := strconv.Atoi(raw)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("invalid %s %q", name, raw)
	}
	return id, nil
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
	svc, err := newService()
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
