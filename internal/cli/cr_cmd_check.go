package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

func newCRCheckCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "check",
		Short: "Run and inspect trust policy checks for a CR",
	}
	cmd.AddCommand(newCRCheckRunCmd())
	cmd.AddCommand(newCRCheckStatusCmd())
	return cmd
}

func newCRCheckRunCmd() *cobra.Command {
	var asJSON bool

	cmd := &cobra.Command{
		Use:   "run <id>",
		Short: "Run required trust checks for a CR",
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
			report, err := svc.RunTrustChecksCR(id)
			if err != nil {
				if asJSON {
					return writeJSONError(cmd, err)
				}
				return err
			}
			if asJSON {
				return writeJSONSuccess(cmd, trustCheckRunToJSONMap(report))
			}
			fmt.Fprintf(cmd.OutOrStdout(), "CR %d trust check run complete.\n", report.CRID)
			fmt.Fprintf(cmd.OutOrStdout(), "Executed: %d\n", report.Executed)
			fmt.Fprintf(cmd.OutOrStdout(), "Risk Tier: %s\n", nonEmpty(strings.TrimSpace(report.RiskTier), "-"))
			printTrustRequirements(cmd, report.Requirements)
			printTrustCheckResults(cmd, report.CheckResults)
			return nil
		},
	}

	cmd.Flags().BoolVar(&asJSON, "json", false, "Output in JSON format")
	return cmd
}

func newCRCheckStatusCmd() *cobra.Command {
	var asJSON bool

	cmd := &cobra.Command{
		Use:   "status <id>",
		Short: "Show trust check status for a CR",
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
			report, err := svc.TrustCheckStatusCR(id)
			if err != nil {
				if asJSON {
					return writeJSONError(cmd, err)
				}
				return err
			}
			if asJSON {
				return writeJSONSuccess(cmd, trustCheckStatusToJSONMap(report))
			}
			fmt.Fprintf(cmd.OutOrStdout(), "CR %d trust check status.\n", report.CRID)
			fmt.Fprintf(cmd.OutOrStdout(), "Risk Tier: %s\n", nonEmpty(strings.TrimSpace(report.RiskTier), "-"))
			fmt.Fprintf(cmd.OutOrStdout(), "Freshness Hours: %d\n", report.FreshnessHours)
			printTrustRequirements(cmd, report.Requirements)
			printTrustCheckResults(cmd, report.CheckResults)
			return nil
		},
	}

	cmd.Flags().BoolVar(&asJSON, "json", false, "Output in JSON format")
	return cmd
}
