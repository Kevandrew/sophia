package cli

import (
	"fmt"
	"sophia/internal/model"
	"strings"

	"github.com/spf13/cobra"
)

func newCRBranchCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "branch",
		Short: "Inspect and manage CR branch aliases",
	}
	cmd.AddCommand(newCRBranchShowCmd())
	cmd.AddCommand(newCRBranchResolveCmd())
	cmd.AddCommand(newCRBranchFormatCmd())
	cmd.AddCommand(newCRBranchMigrateCmd())
	return cmd
}

func newCRBranchShowCmd() *cobra.Command {
	var asJSON bool

	cmd := &cobra.Command{
		Use:   "show <id>",
		Short: "Show branch identity for a CR",
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
			crs, err := svc.ListCRs()
			if err != nil {
				if asJSON {
					return writeJSONError(cmd, err)
				}
				return err
			}
			var selected *model.CR
			for i := range crs {
				if crs[i].ID == id {
					selected = &crs[i]
					break
				}
			}
			if selected == nil {
				err := fmt.Errorf("cr %d not found", id)
				if asJSON {
					return writeJSONError(cmd, err)
				}
				return err
			}
			if asJSON {
				return writeJSONSuccess(cmd, map[string]any{
					"cr": map[string]any{
						"id":     selected.ID,
						"uid":    selected.UID,
						"title":  selected.Title,
						"status": selected.Status,
						"branch": selected.Branch,
					},
				})
			}
			fmt.Fprintf(cmd.OutOrStdout(), "CR: %d\n", selected.ID)
			fmt.Fprintf(cmd.OutOrStdout(), "UID: %s\n", nonEmpty(strings.TrimSpace(selected.UID), "-"))
			fmt.Fprintf(cmd.OutOrStdout(), "Title: %s\n", selected.Title)
			fmt.Fprintf(cmd.OutOrStdout(), "Status: %s\n", selected.Status)
			fmt.Fprintf(cmd.OutOrStdout(), "Branch: %s\n", selected.Branch)
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "Output in JSON format")
	return cmd
}

func newCRBranchResolveCmd() *cobra.Command {
	var branch string
	var asJSON bool

	cmd := &cobra.Command{
		Use:   "resolve",
		Short: "Resolve a branch to its CR context",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, err := newServiceForCmd(cmd)
			if err != nil {
				if asJSON {
					return writeJSONError(cmd, err)
				}
				return err
			}
			view, err := svc.ResolveCRBranch(branch)
			if err != nil {
				if asJSON {
					return writeJSONError(cmd, err)
				}
				return err
			}
			if asJSON {
				return writeJSONSuccess(cmd, map[string]any{
					"input_branch": view.InputBranch,
					"cr":           crToJSONMap(view.CR),
				})
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Input branch: %s\n", view.InputBranch)
			fmt.Fprintf(cmd.OutOrStdout(), "Resolved CR: %d\n", view.CR.ID)
			fmt.Fprintf(cmd.OutOrStdout(), "Title: %s\n", view.CR.Title)
			fmt.Fprintf(cmd.OutOrStdout(), "Stored branch: %s\n", view.CR.Branch)
			return nil
		},
	}
	cmd.Flags().StringVar(&branch, "branch", "", "Branch name to resolve (defaults to current branch)")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Output in JSON format")
	return cmd
}

func newCRBranchFormatCmd() *cobra.Command {
	var id int
	var uid string
	var title string
	var ownerPrefix string
	var asJSON bool

	cmd := &cobra.Command{
		Use:   "format",
		Short: "Format a CR branch alias from uid/title",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if id <= 0 && strings.TrimSpace(uid) == "" {
				err := fmt.Errorf("provide --uid or an existing --id")
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
			view, err := svc.FormatCRBranch(id, title, ownerPrefix, uid, cmd.Flags().Changed("owner-prefix"))
			if err != nil {
				if asJSON {
					return writeJSONError(cmd, err)
				}
				return err
			}
			if asJSON {
				return writeJSONSuccess(cmd, map[string]any{
					"id":           view.ID,
					"uid":          view.UID,
					"title":        view.Title,
					"owner_prefix": view.OwnerPrefix,
					"branch":       view.Branch,
				})
			}
			fmt.Fprintln(cmd.OutOrStdout(), view.Branch)
			return nil
		},
	}
	cmd.Flags().IntVar(&id, "id", 0, "Existing CR id")
	cmd.Flags().StringVar(&uid, "uid", "", "CR uid (required unless --id resolves to an existing CR)")
	cmd.Flags().StringVar(&title, "title", "", "CR title (required unless inferred from --id)")
	cmd.Flags().StringVar(&ownerPrefix, "owner-prefix", "", "Optional owner prefix for generated alias")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Output in JSON format")
	return cmd
}

func newCRBranchMigrateCmd() *cobra.Command {
	var all bool
	var dryRun bool
	var asJSON bool

	cmd := &cobra.Command{
		Use:   "migrate [id]",
		Short: "Migrate existing CR branch aliases to the current format",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, err := newServiceForCmd(cmd)
			if err != nil {
				if asJSON {
					return writeJSONError(cmd, err)
				}
				return err
			}
			results := []map[string]any{}
			runOne := func(id int) error {
				view, migrateErr := svc.MigrateCRBranch(id, dryRun)
				if migrateErr != nil {
					return migrateErr
				}
				results = append(results, map[string]any{
					"cr_id":   view.CRID,
					"uid":     view.UID,
					"from":    view.From,
					"to":      view.To,
					"changed": view.Changed,
					"applied": view.Applied,
				})
				return nil
			}

			switch {
			case all:
				crs, listErr := svc.ListCRs()
				if listErr != nil {
					if asJSON {
						return writeJSONError(cmd, listErr)
					}
					return listErr
				}
				for _, cr := range crs {
					if cr.Status != model.StatusInProgress {
						continue
					}
					if err := runOne(cr.ID); err != nil {
						if asJSON {
							return writeJSONError(cmd, err)
						}
						return err
					}
				}
			default:
				if len(args) != 1 {
					err := fmt.Errorf("provide <id> or use --all")
					if asJSON {
						return writeJSONError(cmd, err)
					}
					return err
				}
				id, parseErr := parsePositiveIntArg(args[0], "id")
				if parseErr != nil {
					if asJSON {
						return writeJSONError(cmd, parseErr)
					}
					return parseErr
				}
				if err := runOne(id); err != nil {
					if asJSON {
						return writeJSONError(cmd, err)
					}
					return err
				}
			}

			if asJSON {
				return writeJSONSuccess(cmd, map[string]any{
					"dry_run": dryRun,
					"count":   len(results),
					"items":   results,
				})
			}
			if len(results) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No in-progress CRs to migrate.")
				return nil
			}
			for _, item := range results {
				fmt.Fprintf(cmd.OutOrStdout(), "CR %v: %v -> %v (changed=%v applied=%v)\n",
					item["cr_id"], item["from"], item["to"], item["changed"], item["applied"])
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&all, "all", false, "Migrate all in-progress CR branches")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Preview migration without renaming branches")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Output in JSON format")
	return cmd
}
