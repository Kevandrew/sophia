package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

func newCRSyncCmd() *cobra.Command {
	var force bool
	var replace bool
	var asJSON bool

	cmd := &cobra.Command{
		Use:   "sync [<id|uid>]",
		Short: "Sync CR intent metadata from the collaboration remote (alias of pull by default)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			selector := ""
			if len(args) == 1 {
				selector = strings.TrimSpace(args[0])
			}
			svc, err := newServiceForCmd(cmd)
			if err != nil {
				return commandError(cmd, asJSON, err)
			}
			if replace {
				uid := selector
				if uid != "" {
					if id, parseErr := svc.ResolveCRID(uid); parseErr == nil {
						review, reviewErr := svc.ReviewCR(id)
						if reviewErr != nil {
							return commandError(cmd, asJSON, reviewErr)
						}
						if review == nil || review.CR == nil {
							return commandError(cmd, asJSON, fmt.Errorf("cr %d is unavailable", id))
						}
						if strings.TrimSpace(review.CR.UID) == "" {
							return commandError(cmd, asJSON, fmt.Errorf("cr %d has empty uid", id))
						}
						uid = strings.TrimSpace(review.CR.UID)
					}
				}
				if uid == "" {
					ctx, currentErr := svc.CurrentCR()
					if currentErr != nil {
						return commandError(cmd, asJSON, fmt.Errorf("replace mode requires <id|uid> or an active CR branch: %w", currentErr))
					}
					uid = strings.TrimSpace(ctx.CR.UID)
				}
				uid = strings.TrimSpace(uid)
				if uid == "" {
					return commandError(cmd, asJSON, fmt.Errorf("replace mode requires a CR uid"))
				}
				result, pullErr := svc.SyncCRFromHQ(uid)
				if pullErr != nil {
					return commandError(cmd, asJSON, pullErr)
				}
				if asJSON {
					return writeJSONSuccess(cmd, map[string]any{
						"local_cr_id": result.LocalCRID,
						"cr_uid":      result.CRUID,
						"created":     result.Created,
						"replaced":    result.Replaced,
						"mode":        "replace",
					})
				}
				action := "updated"
				switch {
				case result.Created:
					action = "created"
				case result.Replaced:
					action = "replaced"
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Synced remote CR %s into local CR %d (%s, replace mode)\n", result.CRUID, result.LocalCRID, action)
				return nil
			}

			result, pullErr := svc.PullCRFromHQ(selector, force)
			if pullErr != nil {
				return commandError(cmd, asJSON, pullErr)
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
					"mode":                 "pull",
				})
			}
			switch {
			case result.Created:
				fmt.Fprintf(cmd.OutOrStdout(), "Synced remote CR %s into new local CR %d\n", result.CRUID, result.LocalCRID)
			case result.Updated:
				fmt.Fprintf(cmd.OutOrStdout(), "Synced remote CR %s into local CR %d (updated)\n", result.CRUID, result.LocalCRID)
			case result.LocalAhead:
				fmt.Fprintf(cmd.OutOrStdout(), "Local CR %d is ahead of remote CR %s\n", result.LocalCRID, result.CRUID)
			default:
				fmt.Fprintf(cmd.OutOrStdout(), "Local CR %d is up to date with remote CR %s\n", result.LocalCRID, result.CRUID)
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&force, "force", false, "Accept remote intent as canonical when merge-safe pull would conflict")
	cmd.Flags().BoolVar(&replace, "replace", false, "Use legacy destructive replace behavior")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Output in JSON format")
	return cmd
}
