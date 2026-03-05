package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"sophia/internal/service"
)

func newCRDelegateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delegate",
		Short: "Inspect CR delegation runs",
	}
	cmd.AddCommand(newCRDelegateListCmd())
	cmd.AddCommand(newCRDelegateShowCmd())
	return cmd
}

func newCRDelegateListCmd() *cobra.Command {
	var asJSON bool

	cmd := &cobra.Command{
		Use:   "list <id>",
		Short: "List delegation runs for a CR",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withParsedIDAndService(cmd, asJSON, args[0], "id", func(id int, svc *service.Service) error {
				runs, err := svc.ListDelegationRuns(id)
				if err != nil {
					return commandError(cmd, asJSON, err)
				}
				if asJSON {
					items := make([]map[string]any, 0, len(runs))
					for _, run := range runs {
						items = append(items, delegationRunToJSONMap(&run))
					}
					return writeJSONSuccess(cmd, map[string]any{
						"cr_id": id,
						"count": len(items),
						"runs":  items,
					})
				}
				if len(runs) == 0 {
					fmt.Fprintf(cmd.OutOrStdout(), "No delegation runs found for CR %d.\n", id)
					return nil
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Delegation runs for CR %d:\n", id)
				for _, run := range runs {
					fmt.Fprintf(cmd.OutOrStdout(), "- %s %s runtime=%s tasks=%s updated=%s\n",
						nonEmpty(strings.TrimSpace(run.ID), "-"),
						nonEmpty(strings.TrimSpace(run.Status), "-"),
						nonEmpty(strings.TrimSpace(run.Request.Runtime), "-"),
						formatDelegationTaskIDs(run.Request.TaskIDs),
						nonEmpty(strings.TrimSpace(run.UpdatedAt), "-"),
					)
					if run.Result != nil && strings.TrimSpace(run.Result.Summary) != "" {
						fmt.Fprintf(cmd.OutOrStdout(), "  result: %s\n", run.Result.Summary)
					}
				}
				return nil
			})
		},
	}

	cmd.Flags().BoolVar(&asJSON, "json", false, "Output in JSON format")
	return cmd
}

func newCRDelegateShowCmd() *cobra.Command {
	var asJSON bool

	cmd := &cobra.Command{
		Use:   "show <id> <run-id>",
		Short: "Show one delegation run for a CR",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withParsedIDAndService(cmd, asJSON, args[0], "id", func(id int, svc *service.Service) error {
				run, err := svc.GetDelegationRun(id, args[1])
				if err != nil {
					return commandError(cmd, asJSON, err)
				}
				if asJSON {
					return writeJSONSuccess(cmd, map[string]any{
						"cr_id": id,
						"run":   delegationRunToJSONMap(run),
					})
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Delegation Run: %s\n", nonEmpty(strings.TrimSpace(run.ID), "-"))
				fmt.Fprintf(cmd.OutOrStdout(), "Status: %s\n", nonEmpty(strings.TrimSpace(run.Status), "-"))
				fmt.Fprintf(cmd.OutOrStdout(), "Runtime: %s\n", nonEmpty(strings.TrimSpace(run.Request.Runtime), "-"))
				fmt.Fprintf(cmd.OutOrStdout(), "Tasks: %s\n", formatDelegationTaskIDs(run.Request.TaskIDs))
				fmt.Fprintf(cmd.OutOrStdout(), "Created: %s by %s\n", nonEmpty(strings.TrimSpace(run.CreatedAt), "-"), nonEmpty(strings.TrimSpace(run.CreatedBy), "-"))
				fmt.Fprintf(cmd.OutOrStdout(), "Updated: %s\n", nonEmpty(strings.TrimSpace(run.UpdatedAt), "-"))
				if strings.TrimSpace(run.FinishedAt) != "" {
					fmt.Fprintf(cmd.OutOrStdout(), "Finished: %s\n", run.FinishedAt)
				}
				if run.Result != nil {
					fmt.Fprintf(cmd.OutOrStdout(), "Result: %s\n", nonEmpty(strings.TrimSpace(run.Result.Summary), nonEmpty(strings.TrimSpace(run.Result.Status), "-")))
					if len(run.Result.FilesChanged) > 0 {
						fmt.Fprintf(cmd.OutOrStdout(), "Files changed: %s\n", strings.Join(run.Result.FilesChanged, ", "))
					}
					if len(run.Result.ValidationErrors) > 0 {
						fmt.Fprintf(cmd.OutOrStdout(), "Validation errors: %s\n", strings.Join(run.Result.ValidationErrors, "; "))
					}
					if len(run.Result.ValidationWarnings) > 0 {
						fmt.Fprintf(cmd.OutOrStdout(), "Validation warnings: %s\n", strings.Join(run.Result.ValidationWarnings, "; "))
					}
					if len(run.Result.Blockers) > 0 {
						fmt.Fprintf(cmd.OutOrStdout(), "Blockers: %s\n", strings.Join(run.Result.Blockers, "; "))
					}
				}
				if len(run.Events) == 0 {
					fmt.Fprintln(cmd.OutOrStdout(), "Events: none")
					return nil
				}
				fmt.Fprintln(cmd.OutOrStdout(), "Events:")
				for _, event := range run.Events {
					label := strings.TrimSpace(event.Summary)
					if label == "" {
						label = strings.TrimSpace(event.Message)
					}
					if label == "" {
						label = strings.TrimSpace(event.Step)
					}
					fmt.Fprintf(cmd.OutOrStdout(), "- #%d %s %s\n", event.ID, nonEmpty(strings.TrimSpace(event.Kind), "-"), nonEmpty(label, "-"))
				}
				return nil
			})
		},
	}

	cmd.Flags().BoolVar(&asJSON, "json", false, "Output in JSON format")
	return cmd
}

func formatDelegationTaskIDs(ids []int) string {
	if len(ids) == 0 {
		return "-"
	}
	parts := make([]string, 0, len(ids))
	for _, id := range ids {
		parts = append(parts, fmt.Sprintf("%d", id))
	}
	return strings.Join(parts, ",")
}
