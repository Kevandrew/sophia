package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newDoctorCmd() *cobra.Command {
	var limit int

	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Run Sophia workflow integrity checks",
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, err := newService()
			if err != nil {
				return err
			}
			report, err := svc.Doctor(limit)
			if err != nil {
				return err
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
	return cmd
}
