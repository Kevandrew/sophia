package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"sophia/internal/service"
)

func newCRCurrentCmd() *cobra.Command {
	var asJSON bool

	cmd := &cobra.Command{
		Use:   "current",
		Short: "Show the active CR context for the current branch",
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, err := newService()
			if err != nil {
				return commandError(cmd, asJSON, err)
			}
			ctx, err := svc.CurrentCR()
			if err != nil {
				if errorsIs(err, service.ErrNoActiveCRContext) {
					fmt.Fprintln(cmd.OutOrStdout(), "No active CR context on current branch.")
				}
				return commandError(cmd, asJSON, err)
			}
			if asJSON {
				return writeJSONSuccess(cmd, map[string]any{
					"branch": ctx.Branch,
					"cr": map[string]any{
						"id":           ctx.CR.ID,
						"uid":          ctx.CR.UID,
						"title":        ctx.CR.Title,
						"status":       ctx.CR.Status,
						"base_branch":  ctx.CR.BaseBranch,
						"base_ref":     ctx.CR.BaseRef,
						"base_commit":  ctx.CR.BaseCommit,
						"parent_cr_id": ctx.CR.ParentCRID,
						"branch":       ctx.CR.Branch,
					},
				})
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Branch: %s\n", ctx.Branch)
			fmt.Fprintf(cmd.OutOrStdout(), "CR: %d\n", ctx.CR.ID)
			fmt.Fprintf(cmd.OutOrStdout(), "Title: %s\n", ctx.CR.Title)
			fmt.Fprintf(cmd.OutOrStdout(), "Status: %s\n", ctx.CR.Status)
			fmt.Fprintf(cmd.OutOrStdout(), "Base: %s\n", ctx.CR.BaseBranch)
			fmt.Fprintf(cmd.OutOrStdout(), "Base Ref: %s\n", nonEmpty(ctx.CR.BaseRef, "(missing)"))
			fmt.Fprintf(cmd.OutOrStdout(), "Base Commit: %s\n", nonEmpty(ctx.CR.BaseCommit, "(missing)"))
			fmt.Fprintf(cmd.OutOrStdout(), "Parent CR: %d\n", ctx.CR.ParentCRID)
			return nil
		},
	}

	cmd.Flags().BoolVar(&asJSON, "json", false, "Output in JSON format")
	return cmd
}

func newCRSwitchCmd() *cobra.Command {
	var asJSON bool

	cmd := &cobra.Command{
		Use:   "switch <id>",
		Short: "Switch to the branch for a CR",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withParsedIDAndService(cmd, asJSON, args[0], "id", func(id int, svc *service.Service) error {
				cr, err := svc.SwitchCR(id)
				if err != nil {
					if !asJSON && errorsIs(err, service.ErrWorkingTreeDirty) {
						fmt.Fprintln(cmd.OutOrStdout(), "Working tree is dirty. Commit changes or run `git stash`, then retry.")
					} else if !asJSON && errorsIs(err, service.ErrBranchInOtherWorktree) {
						fmt.Fprintln(cmd.OutOrStdout(), "Target branch is already checked out in another worktree. Run this command from that worktree path.")
					}
					return commandError(cmd, asJSON, err)
				}
				if asJSON {
					return writeJSONSuccess(cmd, map[string]any{
						"cr_id":  cr.ID,
						"branch": cr.Branch,
					})
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Switched to CR %d branch %s\n", cr.ID, cr.Branch)
				return nil
			})
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "Output in JSON format")
	return cmd
}

func newCRReopenCmd() *cobra.Command {
	var asJSON bool

	cmd := &cobra.Command{
		Use:   "reopen <id>",
		Short: "Reopen a merged CR and switch to its branch",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withParsedIDAndService(cmd, asJSON, args[0], "id", func(id int, svc *service.Service) error {
				cr, err := svc.ReopenCR(id)
				if err != nil {
					if !asJSON && errorsIs(err, service.ErrWorkingTreeDirty) {
						fmt.Fprintln(cmd.OutOrStdout(), "Working tree is dirty. Commit changes or run `git stash`, then retry.")
					} else if !asJSON && errorsIs(err, service.ErrBranchInOtherWorktree) {
						fmt.Fprintln(cmd.OutOrStdout(), "Target branch is already checked out in another worktree. Reopen from that worktree path.")
					}
					return commandError(cmd, asJSON, err)
				}
				if asJSON {
					return writeJSONSuccess(cmd, map[string]any{
						"cr_id":  cr.ID,
						"branch": cr.Branch,
						"status": cr.Status,
					})
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Reopened CR %d on branch %s\n", cr.ID, cr.Branch)
				return nil
			})
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "Output in JSON format")
	return cmd
}

func newCRBaseCmd() *cobra.Command {
	baseCmd := &cobra.Command{
		Use:   "base",
		Short: "Manage per-CR base ref settings",
	}
	baseCmd.AddCommand(newCRBaseSetCmd())
	return baseCmd
}

func newCRBaseSetCmd() *cobra.Command {
	var ref string
	var rebase bool
	var asJSON bool

	cmd := &cobra.Command{
		Use:   "set <id>",
		Short: "Set a CR base ref with optional immediate rebase",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ref = strings.TrimSpace(ref)
			if ref == "" {
				err := fmt.Errorf("--ref is required")
				return commandError(cmd, asJSON, err)
			}
			return withParsedIDAndService(cmd, asJSON, args[0], "id", func(id int, svc *service.Service) error {
				cr, err := svc.SetCRBase(id, ref, rebase)
				if err != nil {
					return commandError(cmd, asJSON, err)
				}
				if asJSON {
					return writeJSONSuccess(cmd, map[string]any{
						"cr_id":       cr.ID,
						"base_ref":    cr.BaseRef,
						"base_commit": cr.BaseCommit,
						"rebased":     rebase,
					})
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Updated CR %d base to %s (%s)\n", cr.ID, cr.BaseRef, nonEmpty(cr.BaseCommit, "-"))
				return nil
			})
		},
	}

	cmd.Flags().StringVar(&ref, "ref", "", "Git ref to use as CR base")
	cmd.Flags().BoolVar(&rebase, "rebase", false, "Rebase CR branch onto the new base ref")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Output in JSON format")
	return cmd
}

func newCRRestackCmd() *cobra.Command {
	var asJSON bool

	cmd := &cobra.Command{
		Use:   "restack <id>",
		Short: "Restack a child CR onto its parent effective head",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withParsedIDAndService(cmd, asJSON, args[0], "id", func(id int, svc *service.Service) error {
				cr, err := svc.RestackCR(id)
				if err != nil {
					return commandError(cmd, asJSON, err)
				}
				if asJSON {
					return writeJSONSuccess(cmd, map[string]any{
						"cr_id":       cr.ID,
						"base_ref":    cr.BaseRef,
						"base_commit": cr.BaseCommit,
					})
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Restacked CR %d onto base %s (%s)\n", cr.ID, nonEmpty(cr.BaseRef, "-"), nonEmpty(cr.BaseCommit, "-"))
				return nil
			})
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "Output in JSON format")
	return cmd
}

func newCRRefreshCmd() *cobra.Command {
	var strategy string
	var dryRun bool
	var asJSON bool

	cmd := &cobra.Command{
		Use:   "refresh <id>",
		Short: "Refresh a CR onto latest base/parent with an explicit strategy",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withParsedIDAndService(cmd, asJSON, args[0], "id", func(id int, svc *service.Service) error {
				view, err := svc.RefreshCR(id, service.RefreshOptions{
					Strategy: strategy,
					DryRun:   dryRun,
				})
				if err != nil {
					return commandError(cmd, asJSON, err)
				}
				if asJSON {
					return writeJSONSuccess(cmd, crRefreshToJSONMap(view))
				}

				action := "Refreshed"
				if view.DryRun {
					action = "Would refresh"
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%s CR %d using strategy %s\n", action, view.CRID, view.Strategy)
				fmt.Fprintf(cmd.OutOrStdout(), "Base Ref: %s\n", nonEmpty(strings.TrimSpace(view.BaseRef), "-"))
				fmt.Fprintf(cmd.OutOrStdout(), "Target Ref: %s\n", nonEmpty(strings.TrimSpace(view.TargetRef), "-"))
				fmt.Fprintf(cmd.OutOrStdout(), "Before Head: %s\n", nonEmpty(strings.TrimSpace(view.BeforeHead), "-"))
				if strings.TrimSpace(view.AfterHead) != "" {
					fmt.Fprintf(cmd.OutOrStdout(), "After Head: %s\n", strings.TrimSpace(view.AfterHead))
				}
				if len(view.Warnings) > 0 {
					fmt.Fprintln(cmd.OutOrStdout(), "Warnings:")
					for _, warning := range view.Warnings {
						fmt.Fprintf(cmd.OutOrStdout(), "- %s\n", warning)
					}
				}
				return nil
			})
		},
	}

	cmd.Flags().StringVar(&strategy, "strategy", service.RefreshStrategyAuto, "Refresh strategy: auto|restack|rebase")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Preview strategy/target without mutating branch history")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Output in JSON format")
	return cmd
}

func newCRNoteCmd() *cobra.Command {
	var asJSON bool

	cmd := &cobra.Command{
		Use:   "note <id> <note>",
		Short: "Append a note to a change request",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withParsedIDAndService(cmd, asJSON, args[0], "id", func(id int, svc *service.Service) error {
				if err := svc.AddNote(id, args[1]); err != nil {
					return commandError(cmd, asJSON, err)
				}
				if asJSON {
					return writeJSONSuccess(cmd, map[string]any{
						"cr_id": id,
						"note":  args[1],
					})
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Added note to CR %d\n", id)
				return nil
			})
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "Output in JSON format")
	return cmd
}
