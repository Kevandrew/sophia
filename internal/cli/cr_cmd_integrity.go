package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"sophia/internal/service"
)

func newCRDoctorCmd() *cobra.Command {
	var asJSON bool

	cmd := &cobra.Command{
		Use:   "doctor <id>",
		Short: "Run CR-scoped integrity checks for checkpoint/base metadata",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parsePositiveIntArg(args[0], "id")
			if err != nil {
				if asJSON {
					return writeJSONError(cmd, err)
				}
				return err
			}
			svc, err := newService()
			if err != nil {
				if asJSON {
					return writeJSONError(cmd, err)
				}
				return err
			}
			report, err := svc.DoctorCR(id)
			if err != nil {
				if asJSON {
					return writeJSONError(cmd, err)
				}
				return err
			}
			if asJSON {
				return writeJSONSuccess(cmd, crDoctorToJSONMap(report))
			}

			fmt.Fprintf(cmd.OutOrStdout(), "CR %d doctor\n", report.CRID)
			fmt.Fprintf(cmd.OutOrStdout(), "CR UID: %s\n", nonEmpty(strings.TrimSpace(report.CRUID), "-"))
			fmt.Fprintf(cmd.OutOrStdout(), "Branch: %s (exists=%t)\n", report.Branch, report.BranchExists)
			fmt.Fprintf(cmd.OutOrStdout(), "Branch Head: %s\n", nonEmpty(strings.TrimSpace(report.BranchHead), "-"))
			fmt.Fprintf(cmd.OutOrStdout(), "Base Ref: %s\n", nonEmpty(strings.TrimSpace(report.BaseRef), "-"))
			fmt.Fprintf(cmd.OutOrStdout(), "Base Commit: %s\n", nonEmpty(strings.TrimSpace(report.BaseCommit), "-"))
			fmt.Fprintf(cmd.OutOrStdout(), "Resolved Base Ref: %s\n", nonEmpty(strings.TrimSpace(report.ResolvedBaseRef), "-"))
			fmt.Fprintf(cmd.OutOrStdout(), "Parent CR: %d (expected from base_ref=%d)\n", report.ParentCRID, report.ExpectedParentID)
			if len(report.Findings) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "\nCR doctor status: OK")
				return nil
			}
			fmt.Fprintln(cmd.OutOrStdout(), "\nFindings:")
			for _, finding := range report.Findings {
				taskSuffix := ""
				if finding.TaskID > 0 {
					taskSuffix = fmt.Sprintf(" task=%d", finding.TaskID)
				}
				commitSuffix := ""
				if strings.TrimSpace(finding.Commit) != "" {
					commitSuffix = fmt.Sprintf(" commit=%s", finding.Commit)
				}
				fmt.Fprintf(cmd.OutOrStdout(), "- [%s]%s%s %s\n", finding.Code, taskSuffix, commitSuffix, finding.Message)
			}
			return fmt.Errorf("cr doctor found %d issue(s)", len(report.Findings))
		},
	}

	cmd.Flags().BoolVar(&asJSON, "json", false, "Output in JSON format")
	return cmd
}

func newCRReconcileCmd() *cobra.Command {
	var regenerate bool
	var asJSON bool

	cmd := &cobra.Command{
		Use:   "reconcile <id>",
		Short: "Reconcile task checkpoint metadata from commit footers",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parsePositiveIntArg(args[0], "id")
			if err != nil {
				if asJSON {
					return writeJSONError(cmd, err)
				}
				return err
			}
			svc, err := newService()
			if err != nil {
				if asJSON {
					return writeJSONError(cmd, err)
				}
				return err
			}
			report, err := svc.ReconcileCR(id, service.ReconcileCROptions{Regenerate: regenerate})
			if err != nil {
				if asJSON {
					return writeJSONError(cmd, err)
				}
				return err
			}
			if asJSON {
				return writeJSONSuccess(cmd, reconcileCRToJSONMap(report))
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Reconciled CR %d\n", report.CRID)
			fmt.Fprintf(cmd.OutOrStdout(), "Scan Ref: %s (%d commits scanned)\n", nonEmpty(strings.TrimSpace(report.ScanRef), "-"), report.ScannedCommits)
			fmt.Fprintf(cmd.OutOrStdout(), "Parent Relinked: %t (%d -> %d)\n", report.ParentRelinked, report.PreviousParentID, report.CurrentParentID)
			fmt.Fprintf(cmd.OutOrStdout(), "Relinked: %d\n", report.Relinked)
			fmt.Fprintf(cmd.OutOrStdout(), "Orphaned: %d\n", report.Orphaned)
			fmt.Fprintf(cmd.OutOrStdout(), "Cleared Orphans: %d\n", report.ClearedOrphans)
			fmt.Fprintf(cmd.OutOrStdout(), "Regenerated: %t\n", report.Regenerated)
			if report.Regenerated {
				fmt.Fprintf(cmd.OutOrStdout(), "Files Changed: %d\n", report.FilesChanged)
				fmt.Fprintf(cmd.OutOrStdout(), "Diff Stat: %s\n", nonEmpty(strings.TrimSpace(report.DiffStat), "-"))
			}
			if len(report.Warnings) > 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "\nWarnings:")
				for _, warning := range report.Warnings {
					fmt.Fprintf(cmd.OutOrStdout(), "- %s\n", warning)
				}
			}
			if len(report.TaskResults) > 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "\nTask Results:")
				for _, result := range report.TaskResults {
					fmt.Fprintf(cmd.OutOrStdout(), "- #%d %s: %s", result.TaskID, result.Status, result.Action)
					if strings.TrimSpace(result.Reason) != "" {
						fmt.Fprintf(cmd.OutOrStdout(), " (%s)", result.Reason)
					}
					fmt.Fprintln(cmd.OutOrStdout())
				}
			}
			if len(report.Findings) > 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "\nRemaining Findings: %d\n", len(report.Findings))
			} else {
				fmt.Fprintln(cmd.OutOrStdout(), "\nRemaining Findings: 0")
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&regenerate, "regenerate", false, "Regenerate derived diff metadata while reconciling")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Output in JSON format")
	return cmd
}
