package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newLogCmd() *cobra.Command {
	var asJSON bool

	cmd := &cobra.Command{
		Use:     "log",
		Short:   "Show intent-first CR history",
		Example: "  sophia log\n  sophia log --json",
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, err := newService()
			if err != nil {
				if asJSON {
					return writeJSONError(cmd, err)
				}
				return err
			}
			entries, err := svc.Log()
			if err != nil {
				if asJSON {
					return writeJSONError(cmd, err)
				}
				return err
			}
			if asJSON {
				items := make([]map[string]any, 0, len(entries))
				for _, entry := range entries {
					items = append(items, map[string]any{
						"id":            entry.ID,
						"title":         entry.Title,
						"status":        entry.Status,
						"who":           entry.Who,
						"when":          entry.When,
						"files_touched": entry.FilesTouched,
					})
				}
				return writeJSONSuccess(cmd, map[string]any{
					"count":   len(items),
					"entries": items,
				})
			}
			if len(entries) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No CR history found.")
				return nil
			}
			fmt.Fprintln(cmd.OutOrStdout(), "CR\tSTATUS\tWHEN\tWHO\tFILES\tTITLE")
			for _, entry := range entries {
				fmt.Fprintf(cmd.OutOrStdout(), "%d\t%s\t%s\t%s\t%s\t%s\n", entry.ID, entry.Status, entry.When, entry.Who, entry.FilesTouched, entry.Title)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "Output in JSON format")
	return cmd
}
