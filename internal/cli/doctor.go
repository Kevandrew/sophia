package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newDoctorCmd() *cobra.Command {
	var limit int
	var asJSON bool

	cmd := &cobra.Command{
		Use:     "doctor",
		Short:   "Run Sophia workflow integrity checks",
		Example: "  sophia doctor\n  sophia doctor --limit 200\n  sophia doctor --json",
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, err := newServiceForCmd(cmd)
			if err != nil {
				if asJSON {
					return writeJSONError(cmd, err)
				}
				return err
			}
			report, err := svc.Doctor(limit)
			if err != nil {
				if asJSON {
					return writeJSONError(cmd, err)
				}
				return err
			}
			if asJSON {
				findings := make([]map[string]any, 0, len(report.Findings))
				for _, finding := range report.Findings {
					findings = append(findings, map[string]any{
						"code":    finding.Code,
						"message": finding.Message,
					})
				}
				return writeJSONSuccess(cmd, map[string]any{
					"base_branch":    report.BaseBranch,
					"current_branch": report.CurrentBranch,
					"working_tree": map[string]any{
						"modified_staged_count": report.ChangedCount,
						"untracked_count":       report.UntrackedCount,
						"dirty":                 report.ChangedCount > 0 || report.UntrackedCount > 0,
					},
					"scanned_commits": report.ScannedCommits,
					"findings":        findings,
				})
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Base branch: %s\n", report.BaseBranch)
			if report.CurrentBranch == "" {
				fmt.Fprintln(cmd.OutOrStdout(), "Current branch: (unknown)")
			} else {
				fmt.Fprintf(cmd.OutOrStdout(), "Current branch: %s\n", report.CurrentBranch)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Working tree: %d modified/staged, %d untracked\n", report.ChangedCount, report.UntrackedCount)
			fmt.Fprintf(cmd.OutOrStdout(), "Scanned commits: %d\n", report.ScannedCommits)

			if len(report.Findings) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "\nDoctor status: OK")
				return nil
			}
			fmt.Fprintln(cmd.OutOrStdout(), "\nFindings:")
			for _, finding := range report.Findings {
				fmt.Fprintf(cmd.OutOrStdout(), "- [%s] %s\n", finding.Code, finding.Message)
			}
			return fmt.Errorf("doctor found %d issue(s)", len(report.Findings))
		},
	}

	cmd.Flags().IntVar(&limit, "limit", 100, "Number of recent base-branch commits to scan")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Output in JSON format")
	return cmd
}
