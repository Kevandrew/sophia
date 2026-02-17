package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"sophia/internal/model"
)

func newCRReviewCmd() *cobra.Command {
	var asJSON bool

	cmd := &cobra.Command{
		Use:   "review <id>",
		Short: "Show intent-first CR review context",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parsePositiveIntArg(args[0], "id")
			if err != nil {
				if asJSON {
					return writeJSONError(cmd, err)
				}
				return err
			}
			svc, err := newService()
			if err != nil {
				if asJSON {
					return writeJSONError(cmd, err)
				}
				return err
			}
			review, err := svc.ReviewCR(id)
			if err != nil {
				if asJSON {
					return writeJSONError(cmd, err)
				}
				return err
			}
			if asJSON {
				return writeJSONSuccess(cmd, reviewToJSONMap(review))
			}

			fmt.Fprintf(cmd.OutOrStdout(), "CR %d: %s\n", review.CR.ID, review.CR.Title)
			fmt.Fprintf(cmd.OutOrStdout(), "Status: %s\n", review.CR.Status)
			fmt.Fprintf(cmd.OutOrStdout(), "Base: %s\n", review.CR.BaseBranch)
			fmt.Fprintf(cmd.OutOrStdout(), "Base Ref: %s\n", nonEmpty(review.CR.BaseRef, "(missing)"))
			fmt.Fprintf(cmd.OutOrStdout(), "Base Commit: %s\n", nonEmpty(review.CR.BaseCommit, "(missing)"))
			fmt.Fprintf(cmd.OutOrStdout(), "Parent CR: %d\n", review.CR.ParentCRID)
			fmt.Fprintf(cmd.OutOrStdout(), "Branch: %s\n", review.CR.Branch)
			fmt.Fprintf(cmd.OutOrStdout(), "\nIntent:\n%s\n", nonEmpty(review.CR.Description, "(none)"))
			fmt.Fprintf(cmd.OutOrStdout(), "\nContract:\n")
			fmt.Fprintf(cmd.OutOrStdout(), "- why: %s\n", nonEmpty(strings.TrimSpace(review.Contract.Why), "(missing)"))
			printInlineList(cmd, "scope", review.Contract.Scope)
			printInlineList(cmd, "non_goals", review.Contract.NonGoals)
			printInlineList(cmd, "invariants", review.Contract.Invariants)
			fmt.Fprintf(cmd.OutOrStdout(), "- blast_radius: %s\n", nonEmpty(strings.TrimSpace(review.Contract.BlastRadius), "(missing)"))
			fmt.Fprintf(cmd.OutOrStdout(), "- test_plan: %s\n", nonEmpty(strings.TrimSpace(review.Contract.TestPlan), "(missing)"))
			fmt.Fprintf(cmd.OutOrStdout(), "- rollback_plan: %s\n", nonEmpty(strings.TrimSpace(review.Contract.RollbackPlan), "(missing)"))

			fmt.Fprintf(cmd.OutOrStdout(), "\nSubtasks:\n")
			if len(review.CR.Subtasks) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "- (none)")
			} else {
				for _, task := range review.CR.Subtasks {
					marker := "[ ]"
					switch task.Status {
					case model.TaskStatusDone:
						marker = "[x]"
					case model.TaskStatusDelegated:
						marker = "[~]"
					}
					fmt.Fprintf(cmd.OutOrStdout(), "- %s #%d %s\n", marker, task.ID, task.Title)
				}
			}

			fmt.Fprintf(cmd.OutOrStdout(), "\nNotes:\n")
			if len(review.CR.Notes) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "- (none)")
			} else {
				for _, note := range review.CR.Notes {
					fmt.Fprintf(cmd.OutOrStdout(), "- %s\n", note)
				}
			}

			printListSection(cmd, "New Files", review.NewFiles)
			printListSection(cmd, "Modified Files", review.ModifiedFiles)
			printListSection(cmd, "Deleted Files", review.DeletedFiles)
			printListSection(cmd, "Test Files Touched", review.TestFiles)
			printListSection(cmd, "Dependency Files Touched", review.DependencyFiles)
			printListSection(cmd, "Files Changed", review.Files)
			fmt.Fprintf(cmd.OutOrStdout(), "\nDiff Stat:\n%s\n", review.ShortStat)
			printImpactSection(cmd, review.Impact)
			printStringSection(cmd, "Errors", review.ValidationErrors)
			printStringSection(cmd, "Warnings", review.ValidationWarnings)
			return nil
		},
	}

	cmd.Flags().BoolVar(&asJSON, "json", false, "Output in JSON format")
	return cmd
}

func newCRMergeCmd() *cobra.Command {
	var keepBranch bool
	var deleteBranch bool
	var overrideReason string

	cmd := &cobra.Command{
		Use:   "merge <id>",
		Short: "Create one intent merge commit and merge CR branch into base",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parsePositiveIntArg(args[0], "id")
			if err != nil {
				return err
			}
			svc, err := newService()
			if err != nil {
				return err
			}
			if deleteBranch {
				keepBranch = false
			}
			sha, err := svc.MergeCR(id, keepBranch, overrideReason)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Merged CR %d as commit %s\n", id, sha)
			return nil
		},
	}

	cmd.Flags().BoolVar(&keepBranch, "keep-branch", false, "Keep CR branch after merge (default deletes merged branch)")
	cmd.Flags().BoolVar(&deleteBranch, "delete-branch", false, "Deprecated: branch deletion is now the default")
	cmd.Flags().StringVar(&overrideReason, "override-reason", "", "Bypass validation failures with an audited reason")
	return cmd
}
