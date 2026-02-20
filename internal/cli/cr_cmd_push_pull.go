package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

func newCRPushCmd() *cobra.Command {
	var force bool
	var asJSON bool

	cmd := &cobra.Command{
		Use:   "push [<id|uid>]",
		Short: "Push local CR intent metadata to the configured collaboration remote",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			selector := ""
			if len(args) == 1 {
				selector = strings.TrimSpace(args[0])
			}
			svc, err := newService()
			if err != nil {
				return commandError(cmd, asJSON, err)
			}
			result, err := svc.PushCRToHQ(selector, force)
			if err != nil {
				return commandError(cmd, asJSON, err)
			}
			if asJSON {
				return writeJSONSuccess(cmd, map[string]any{
					"local_cr_id":          result.LocalCRID,
					"cr_uid":               result.CRUID,
					"created_remote":       result.CreatedRemote,
					"updated_remote":       result.UpdatedRemote,
					"noop":                 result.Noop,
					"forced":               result.Forced,
					"upstream_fingerprint": result.UpstreamFingerprint,
					"warnings":             stringSliceOrEmpty(result.Warnings),
				})
			}
			switch {
			case result.CreatedRemote:
				fmt.Fprintf(cmd.OutOrStdout(), "Pushed local CR %d to remote as %s (created)\n", result.LocalCRID, result.CRUID)
			case result.UpdatedRemote:
				fmt.Fprintf(cmd.OutOrStdout(), "Pushed local CR %d to remote %s (updated)\n", result.LocalCRID, result.CRUID)
			default:
				fmt.Fprintf(cmd.OutOrStdout(), "Remote CR %s is up to date\n", result.CRUID)
			}
			if len(result.Warnings) > 0 {
				for _, warning := range result.Warnings {
					fmt.Fprintf(cmd.OutOrStdout(), "warning: %s\n", warning)
				}
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "Push even when upstream moved or local upstream link is missing")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Output in JSON format")
	return cmd
}

func newCRPullCmd() *cobra.Command {
	var force bool
	var asJSON bool

	cmd := &cobra.Command{
		Use:   "pull [<id|uid>]",
		Short: "Pull latest CR intent metadata from the configured collaboration remote",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			selector := ""
			if len(args) == 1 {
				selector = strings.TrimSpace(args[0])
			}
			svc, err := newService()
			if err != nil {
				return commandError(cmd, asJSON, err)
			}
			result, err := svc.PullCRFromHQ(selector, force)
			if err != nil {
				return commandError(cmd, asJSON, err)
			}
			if asJSON {
				return writeJSONSuccess(cmd, map[string]any{
					"local_cr_id":          result.LocalCRID,
					"cr_uid":               result.CRUID,
					"created":              result.Created,
					"updated":              result.Updated,
					"local_ahead":          result.LocalAhead,
					"up_to_date":           result.UpToDate,
					"forced":               result.Forced,
					"upstream_fingerprint": result.UpstreamFingerprint,
				})
			}
			switch {
			case result.Created:
				fmt.Fprintf(cmd.OutOrStdout(), "Pulled remote CR %s into new local CR %d\n", result.CRUID, result.LocalCRID)
			case result.Updated:
				fmt.Fprintf(cmd.OutOrStdout(), "Pulled remote CR %s into local CR %d (updated)\n", result.CRUID, result.LocalCRID)
			case result.LocalAhead:
				fmt.Fprintf(cmd.OutOrStdout(), "Local CR %d is ahead of remote CR %s\n", result.LocalCRID, result.CRUID)
			default:
				fmt.Fprintf(cmd.OutOrStdout(), "Local CR %d is up to date with remote CR %s\n", result.LocalCRID, result.CRUID)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "Accept remote intent as canonical when pull would otherwise conflict")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Output in JSON format")
	return cmd
}
