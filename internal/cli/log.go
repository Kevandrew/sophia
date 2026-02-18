package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newLogCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "log",
		Short:   "Show intent-first CR history",
		Example: "  sophia log",
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, err := newService()
			if err != nil {
				return err
			}
			entries, err := svc.Log()
			if err != nil {
				return err
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
}
