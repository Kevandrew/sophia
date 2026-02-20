package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

func newCRSyncCmd() *cobra.Command {
	var asJSON bool

	cmd := &cobra.Command{
		Use:   "sync <cr-uid>",
		Short: "Fetch latest CR doc from HQ and refresh local CR state by UID",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			uid := strings.TrimSpace(args[0])
			if uid == "" {
				return commandError(cmd, asJSON, fmt.Errorf("cr uid is required"))
			}
			svc, err := newService()
			if err != nil {
				return commandError(cmd, asJSON, err)
			}
			result, err := svc.SyncCRFromHQ(uid)
			if err != nil {
				return commandError(cmd, asJSON, err)
			}
			if asJSON {
				return writeJSONSuccess(cmd, map[string]any{
					"local_cr_id": result.LocalCRID,
					"cr_uid":      result.CRUID,
					"created":     result.Created,
					"replaced":    result.Replaced,
				})
			}
			action := "updated"
			switch {
			case result.Created:
				action = "created"
			case result.Replaced:
				action = "replaced"
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Synced HQ CR %s into local CR %d (%s)\n", result.CRUID, result.LocalCRID, action)
			return nil
		},
	}

	cmd.Flags().BoolVar(&asJSON, "json", false, "Output in JSON format")
	return cmd
}
