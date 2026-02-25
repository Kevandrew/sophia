package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

func newRepairCmd() *cobra.Command {
	var baseBranch string
	var refresh bool
	var asJSON bool

	cmd := &cobra.Command{
		Use:     "repair",
		Short:   "Rebuild local Sophia CR metadata from Git commit history",
		Example: "  sophia repair\n  sophia repair --base-branch main\n  sophia repair --base-branch release/2026-q1 --refresh\n  sophia repair --json",
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, err := newServiceForCmd(cmd)
			if err != nil {
				if asJSON {
					return writeJSONError(cmd, err)
				}
				return err
			}
			report, err := svc.RepairFromGit(baseBranch, refresh)
			if err != nil {
				if asJSON {
					return writeJSONError(cmd, err)
				}
				return err
			}
			if asJSON {
				return writeJSONSuccess(cmd, map[string]any{
					"base_branch":     report.BaseBranch,
					"scanned":         report.Scanned,
					"imported":        report.Imported,
					"updated":         report.Updated,
					"skipped":         report.Skipped,
					"highest_cr_id":   report.HighestCRID,
					"next_id":         report.NextID,
					"repaired_cr_ids": intSliceOrEmpty(report.RepairedCRIDs),
					"refreshed":       refresh,
				})
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Repaired Sophia metadata from base branch %s\n", report.BaseBranch)
			fmt.Fprintf(cmd.OutOrStdout(), "Scanned commits: %d\n", report.Scanned)
			fmt.Fprintf(cmd.OutOrStdout(), "Imported CRs: %d\n", report.Imported)
			fmt.Fprintf(cmd.OutOrStdout(), "Updated CRs: %d\n", report.Updated)
			fmt.Fprintf(cmd.OutOrStdout(), "Skipped CRs: %d\n", report.Skipped)
			fmt.Fprintf(cmd.OutOrStdout(), "Highest CR id: %d\n", report.HighestCRID)
			fmt.Fprintf(cmd.OutOrStdout(), "Next CR id: %d\n", report.NextID)
			if len(report.RepairedCRIDs) > 0 {
				ids := make([]string, 0, len(report.RepairedCRIDs))
				for _, id := range report.RepairedCRIDs {
					ids = append(ids, fmt.Sprintf("%d", id))
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Repaired IDs: %s\n", strings.Join(ids, ", "))
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&baseBranch, "base-branch", "", "Base branch to scan for CR commits")
	cmd.Flags().BoolVar(&refresh, "refresh", false, "Update existing merged CR files from Git (in-progress CRs remain untouched)")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Output in JSON format")
	return cmd
}
