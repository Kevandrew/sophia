package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"sophia/internal/service"
)

func newCRPackCmd() *cobra.Command {
	var (
		asJSON           bool
		eventsLimit      int
		checkpointsLimit int
	)

	cmd := &cobra.Command{
		Use:   "pack <id>",
		Short: "Pack CR context for one-call agent refresh",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parsePositiveIntArg(args[0], "id")
			if err != nil {
				if asJSON {
					return writeJSONError(cmd, err)
				}
				return err
			}
			svc, err := newServiceForCmd(cmd)
			if err != nil {
				if asJSON {
					return writeJSONError(cmd, err)
				}
				return err
			}
			view, err := svc.PackCR(id, service.PackOptions{
				EventsLimit:      eventsLimit,
				CheckpointsLimit: checkpointsLimit,
			})
			if err != nil {
				if asJSON {
					return writeJSONError(cmd, err)
				}
				return err
			}
			if asJSON {
				return writeJSONSuccess(cmd, crPackToJSONMap(view))
			}
			printCRPackView(cmd, view)
			return nil
		},
	}

	cmd.Flags().BoolVar(&asJSON, "json", false, "Output in JSON format")
	cmd.Flags().IntVar(&eventsLimit, "events-limit", 20, "Maximum recent CR events to include")
	cmd.Flags().IntVar(&checkpointsLimit, "checkpoints-limit", 10, "Maximum recent task checkpoints to include")
	return cmd
}

func printCRPackView(cmd *cobra.Command, view *service.CRPackView) {
	if view == nil || view.CR == nil {
		fmt.Fprintln(cmd.OutOrStdout(), "No pack data available.")
		return
	}
	fmt.Fprintf(cmd.OutOrStdout(), "CR Pack\n")
	fmt.Fprintf(cmd.OutOrStdout(), "CR: %d %s\n", view.CR.ID, nonEmpty(strings.TrimSpace(view.CR.Title), "-"))
	fmt.Fprintf(cmd.OutOrStdout(), "Status: %s\n", nonEmpty(strings.TrimSpace(view.CR.Status), "-"))
	if view.Anchors != nil {
		fmt.Fprintf(cmd.OutOrStdout(), "Base: %s\n", nonEmpty(strings.TrimSpace(view.Anchors.Base), "-"))
		fmt.Fprintf(cmd.OutOrStdout(), "Head: %s\n", nonEmpty(strings.TrimSpace(view.Anchors.Head), "-"))
		fmt.Fprintf(cmd.OutOrStdout(), "Merge Base: %s\n", nonEmpty(strings.TrimSpace(view.Anchors.MergeBase), "-"))
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Diff Stat: %s\n", nonEmpty(strings.TrimSpace(view.DiffStat), "-"))
	fmt.Fprintf(cmd.OutOrStdout(), "Events: %d/%d\n", view.EventsMeta.Returned, view.EventsMeta.Total)
	fmt.Fprintf(cmd.OutOrStdout(), "Checkpoints: %d/%d\n", view.CheckpointsMeta.Returned, view.CheckpointsMeta.Total)
	if len(view.Warnings) > 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "\nWarnings:")
		for _, warning := range view.Warnings {
			fmt.Fprintf(cmd.OutOrStdout(), "- %s\n", warning)
		}
	}
}
