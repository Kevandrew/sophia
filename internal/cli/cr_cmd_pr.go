package cli

import (
	"errors"
	"fmt"
	"sophia/internal/service"
	"strings"

	"github.com/spf13/cobra"
)

func newCRPRCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "pr",
		Short: "Manage PR publishing/sync for PR-gated merge mode",
	}
	cmd.AddCommand(newCRPRContextCmd())
	cmd.AddCommand(newCRPRDraftCmd())
	cmd.AddCommand(newCRPROpenCmd())
	cmd.AddCommand(newCRPRSyncCmd())
	cmd.AddCommand(newCRPRStatusCmd())
	cmd.AddCommand(newCRPRReadyCmd())
	return cmd
}

func newCRPRContextCmd() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "context [id]",
		Short: "Render deterministic PR context markdown from CR intent and evidence",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withOptionalCRIDAndService(cmd, asJSON, args, "id", func(id int, svc *service.Service) error {
				view, err := svc.PRContext(id)
				if err != nil {
					return commandError(cmd, asJSON, err)
				}
				if asJSON {
					return writeJSONSuccess(cmd, map[string]any{
						"cr_id":     view.CRID,
						"cr_uid":    view.CRUID,
						"title":     view.Title,
						"pr_title":  view.PRTitle,
						"branch":    view.Branch,
						"base_ref":  view.BaseRef,
						"markdown":  view.Markdown,
						"body_hash": view.BodyHash,
						"warnings":  stringSliceOrEmpty(view.Warnings),
					})
				}
				fmt.Fprintln(cmd.OutOrStdout(), view.Markdown)
				return nil
			})
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "Output in JSON format")
	return cmd
}

func newCRPRDraftCmd() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "draft [id]",
		Short: "Alias of pr context for draft body generation",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withOptionalCRIDAndService(cmd, asJSON, args, "id", func(id int, svc *service.Service) error {
				view, err := svc.PRDraft(id)
				if err != nil {
					return commandError(cmd, asJSON, err)
				}
				if asJSON {
					return writeJSONSuccess(cmd, map[string]any{"cr_id": view.CRID, "markdown": view.Markdown, "body_hash": view.BodyHash, "warnings": stringSliceOrEmpty(view.Warnings)})
				}
				fmt.Fprintln(cmd.OutOrStdout(), view.Markdown)
				return nil
			})
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "Output in JSON format")
	return cmd
}

func newCRPROpenCmd() *cobra.Command {
	var asJSON bool
	var approve bool
	cmd := &cobra.Command{
		Use:   "open [id]",
		Short: "Open or update linked draft PR for a CR",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withOptionalCRIDAndService(cmd, asJSON, args, "id", func(id int, svc *service.Service) error {
				status, err := svc.PROpen(id, approve)
				if err != nil {
					if asJSON && errors.Is(err, service.ErrPRApprovalRequired) {
						return writeJSONSuccess(cmd, map[string]any{
							"cr_id": id,
							"action_required": map[string]any{
								"type":         "agent_approval",
								"name":         "open_pr",
								"reason":       "approve PR create/open to proceed",
								"approve_flag": "--approve-open",
							},
						})
					}
					return commandError(cmd, asJSON, err)
				}
				if asJSON {
					return writeJSONSuccess(cmd, prStatusToJSONMap(status))
				}
				fmt.Fprintf(cmd.OutOrStdout(), "PR #%d: %s\n", status.Number, nonEmpty(strings.TrimSpace(status.URL), "(missing url)"))
				return nil
			})
		},
	}
	cmd.Flags().BoolVar(&approve, "approve-open", false, "Approve creating/opening a new PR when none is linked")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Output in JSON format")
	return cmd
}

func newCRPRSyncCmd() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "sync [id]",
		Short: "Sync Sophia-managed section of linked PR",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withOptionalCRIDAndService(cmd, asJSON, args, "id", func(id int, svc *service.Service) error {
				status, err := svc.PRSync(id)
				if err != nil {
					return commandError(cmd, asJSON, err)
				}
				if asJSON {
					return writeJSONSuccess(cmd, prStatusToJSONMap(status))
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Synced PR #%d: %s\n", status.Number, status.URL)
				return nil
			})
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "Output in JSON format")
	return cmd
}

func newCRPRStatusCmd() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "status [id]",
		Short: "Show linked PR status and gate evaluation",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withOptionalCRIDAndService(cmd, asJSON, args, "id", func(id int, svc *service.Service) error {
				status, err := svc.PRStatus(id)
				if err != nil {
					return commandError(cmd, asJSON, err)
				}
				if asJSON {
					return writeJSONSuccess(cmd, prStatusToJSONMap(status))
				}
				fmt.Fprintf(cmd.OutOrStdout(), "PR #%d: %s\n", status.Number, nonEmpty(status.URL, "(none)"))
				fmt.Fprintf(cmd.OutOrStdout(), "State: %s\n", nonEmpty(status.State, "-"))
				fmt.Fprintf(cmd.OutOrStdout(), "Gate Blocked: %t\n", status.GateBlocked)
				printListSection(cmd, "Gate Reasons", status.GateReasons)
				return nil
			})
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "Output in JSON format")
	return cmd
}

func newCRPRReadyCmd() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "ready [id]",
		Short: "Mark linked PR ready for review",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withOptionalCRIDAndService(cmd, asJSON, args, "id", func(id int, svc *service.Service) error {
				status, err := svc.PRReady(id)
				if err != nil {
					return commandError(cmd, asJSON, err)
				}
				if asJSON {
					return writeJSONSuccess(cmd, prStatusToJSONMap(status))
				}
				fmt.Fprintf(cmd.OutOrStdout(), "PR #%d marked ready\n", status.Number)
				return nil
			})
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "Output in JSON format")
	return cmd
}

func prStatusToJSONMap(status *service.PRStatusView) map[string]any {
	if status == nil {
		return map[string]any{}
	}
	return map[string]any{
		"cr_id":                status.CRID,
		"cr_uid":               status.CRUID,
		"provider":             status.Provider,
		"repo":                 status.Repo,
		"number":               status.Number,
		"url":                  status.URL,
		"state":                status.State,
		"draft":                status.Draft,
		"review_decision":      status.ReviewDecision,
		"merged":               status.Merged,
		"merged_at":            status.MergedAt,
		"merged_commit":        status.MergedCommit,
		"checks_passing":       status.ChecksPassing,
		"approvals":            status.Approvals,
		"non_author_approvals": status.NonAuthorApprovals,
		"gate_blocked":         status.GateBlocked,
		"gate_reasons":         stringSliceOrEmpty(status.GateReasons),
		"action_required":      status.ActionRequired,
		"action_reason":        status.ActionReason,
		"warnings":             stringSliceOrEmpty(status.Warnings),
	}
}
