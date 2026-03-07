package cli

import (
	"bufio"
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"sophia/internal/model"
	"sophia/internal/service"
)

func newCRReviewCmd() *cobra.Command {
	var asJSON bool

	cmd := &cobra.Command{
		Use:   "review [id]",
		Short: "Show intent-first CR review context",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withOptionalCRIDAndService(cmd, asJSON, args, "id", func(id int, svc *service.Service) error {
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
				printInlineList(cmd, "risk_critical_scopes", review.Contract.RiskCriticalScopes)
				fmt.Fprintf(cmd.OutOrStdout(), "- risk_tier_hint: %s\n", nonEmpty(strings.TrimSpace(review.Contract.RiskTierHint), "(none)"))
				fmt.Fprintf(cmd.OutOrStdout(), "- risk_rationale: %s\n", nonEmpty(strings.TrimSpace(review.Contract.RiskRationale), "(none)"))
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

				fmt.Fprintf(cmd.OutOrStdout(), "\nEvidence:\n")
				if len(review.CR.Evidence) == 0 {
					fmt.Fprintln(cmd.OutOrStdout(), "- (none)")
				} else {
					for i, entry := range review.CR.Evidence {
						fmt.Fprintf(cmd.OutOrStdout(), "- #%d %s %s: %s\n", i+1, nonEmpty(strings.TrimSpace(entry.TS), "-"), nonEmpty(strings.TrimSpace(entry.Type), "-"), nonEmpty(strings.TrimSpace(entry.Summary), "-"))
						if strings.TrimSpace(entry.Scope) != "" {
							fmt.Fprintf(cmd.OutOrStdout(), "  scope: %s\n", entry.Scope)
						}
						if strings.TrimSpace(entry.Command) != "" {
							fmt.Fprintf(cmd.OutOrStdout(), "  command: %s\n", entry.Command)
						}
						if entry.ExitCode != nil {
							fmt.Fprintf(cmd.OutOrStdout(), "  exit_code: %d\n", *entry.ExitCode)
						}
						if strings.TrimSpace(entry.OutputHash) != "" {
							fmt.Fprintf(cmd.OutOrStdout(), "  output_hash: %s\n", entry.OutputHash)
						}
						if len(entry.Attachments) > 0 {
							fmt.Fprintf(cmd.OutOrStdout(), "  attachments: %s\n", strings.Join(entry.Attachments, ", "))
						}
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
				printTrustSection(cmd, review.Trust)
				printStringSection(cmd, "Errors", review.ValidationErrors)
				printStringSection(cmd, "Warnings", review.ValidationWarnings)
				fmt.Fprintf(cmd.OutOrStdout(), "\nFreshness: %s\n", nonEmpty(strings.TrimSpace(review.FreshnessState), "unknown"))
				fmt.Fprintf(cmd.OutOrStdout(), "Freshness Reason: %s\n", nonEmpty(strings.TrimSpace(review.FreshnessReason), "-"))
				printNextStepsSection(cmd, reviewNextStepView(review))
				return nil
			})
		},
	}

	cmd.Flags().BoolVar(&asJSON, "json", false, "Output in JSON format")
	return cmd
}

func newCRMergeCmd() *cobra.Command {
	var keepBranch bool
	var deleteBranch bool
	var overrideReason string
	var approvePROpen bool
	var asJSON bool

	cmd := &cobra.Command{
		Use:     "merge [id]",
		Short:   "Merge a CR and recover from merge conflicts",
		Long:    "Merge a CR selected by id/uid or active branch context. If conflicts occur, use the merge subcommands to inspect status, resume after resolution, or abort cleanly.",
		Example: "  sophia cr merge 25\n  sophia cr merge 25 --keep-branch\n  sophia cr merge 25 --override-reason \"Emergency prod fix\"\n  sophia cr merge status 25\n  sophia cr merge resume 25\n  sophia cr merge abort 25",
		Args:    cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withOptionalCRIDAndService(cmd, asJSON, args, "id", func(id int, svc *service.Service) error {
				if deleteBranch {
					keepBranch = false
				}
				result, err := svc.MergeCRWithOptions(id, service.MergeCROptions{
					KeepBranch:     keepBranch,
					OverrideReason: overrideReason,
					ApprovePROpen:  approvePROpen,
				})
				if err != nil {
					if asJSON && errors.Is(err, service.ErrPRApprovalRequired) {
						return writeJSONSuccess(cmd, map[string]any{
							"cr_id":      id,
							"merge_mode": "pr_gate",
							"action_required": map[string]any{
								"type":         "agent_approval",
								"name":         "open_pr",
								"reason":       "approve PR create/open to proceed",
								"approve_flag": "--approve-pr-open",
							},
						})
					}
					if !asJSON && errors.Is(err, service.ErrPRApprovalRequired) && !approvePROpen {
						fmt.Fprint(cmd.OutOrStdout(), "Want me to create/open the PR now? [y/N]: ")
						reader := bufio.NewReader(cmd.InOrStdin())
						line, _ := reader.ReadString('\n')
						answer := strings.ToLower(strings.TrimSpace(line))
						if answer == "y" || answer == "yes" {
							approvePROpen = true
							result, err = svc.MergeCRWithOptions(id, service.MergeCROptions{
								KeepBranch:     keepBranch,
								OverrideReason: overrideReason,
								ApprovePROpen:  true,
							})
						}
					}
				}
				if err != nil {
					if asJSON {
						return writeJSONError(cmd, err)
					}
					return err
				}
				sha := result.MergedCommit
				warnings := append([]string(nil), result.Warnings...)
				if asJSON {
					return writeJSONSuccess(cmd, map[string]any{
						"cr_id":           id,
						"merged_commit":   sha,
						"warnings":        stringSliceOrEmpty(warnings),
						"keep_branch":     keepBranch,
						"override_reason": strings.TrimSpace(overrideReason),
						"merge_mode":      strings.TrimSpace(result.MergeMode),
						"pr_url":          strings.TrimSpace(result.PRURL),
						"action":          strings.TrimSpace(result.Action),
						"action_reason":   strings.TrimSpace(result.ActionReason),
						"gate_blocked":    result.GateBlocked,
						"gate_reasons":    stringSliceOrEmpty(result.GateReasons),
					})
				}
				if strings.TrimSpace(sha) != "" {
					fmt.Fprintf(cmd.OutOrStdout(), "Merged CR %d as commit %s\n", id, sha)
				} else if strings.TrimSpace(result.MergeMode) == "pr_gate" {
					fmt.Fprintf(cmd.OutOrStdout(), "PR-gated merge flow prepared for CR %d\n", id)
					if strings.TrimSpace(result.PRURL) != "" {
						fmt.Fprintf(cmd.OutOrStdout(), "PR: %s\n", result.PRURL)
					}
					if strings.TrimSpace(result.Action) != "" {
						fmt.Fprintf(cmd.OutOrStdout(), "Action: %s\n", result.Action)
					}
					if strings.TrimSpace(result.ActionReason) != "" {
						fmt.Fprintf(cmd.OutOrStdout(), "Reason: %s\n", result.ActionReason)
					}
					if result.GateBlocked {
						printListSection(cmd, "Gate Reasons", result.GateReasons)
					}
				} else {
					fmt.Fprintf(cmd.OutOrStdout(), "Processed merge command for CR %d\n", id)
				}
				for _, warning := range warnings {
					fmt.Fprintf(cmd.OutOrStdout(), "Warning: %s\n", warning)
				}
				return nil
			})
		},
	}

	cmd.Flags().BoolVar(&keepBranch, "keep-branch", false, "Keep CR branch after merge (default deletes merged branch)")
	cmd.Flags().BoolVar(&deleteBranch, "delete-branch", false, "Deprecated: branch deletion is now the default")
	cmd.Flags().StringVar(&overrideReason, "override-reason", "", "Bypass validation failures with an audited reason")
	cmd.Flags().BoolVar(&approvePROpen, "approve-pr-open", false, "Approve opening/creating PR when running merge in pr_gate mode")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Output in JSON format")
	cmd.AddCommand(newCRMergeStatusCmd())
	cmd.AddCommand(newCRMergeAbortCmd())
	cmd.AddCommand(newCRMergeResumeCmd())
	cmd.AddCommand(newCRMergeFinalizeCmd())
	return cmd
}

func newCRMergeFinalizeCmd() *cobra.Command {
	var keepBranch bool
	var overrideReason string
	var asJSON bool

	cmd := &cobra.Command{
		Use:   "finalize [id]",
		Short: "Finalize PR-gated merge after approvals/checks pass",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withOptionalCRIDAndService(cmd, asJSON, args, "id", func(id int, svc *service.Service) error {
				result, err := svc.MergeFinalizeWithOptions(id, service.MergeCROptions{
					KeepBranch:     keepBranch,
					OverrideReason: overrideReason,
				})
				if err != nil {
					if asJSON {
						return writeJSONError(cmd, err)
					}
					return err
				}
				if asJSON {
					return writeJSONSuccess(cmd, map[string]any{
						"cr_id":           id,
						"merged_commit":   strings.TrimSpace(result.MergedCommit),
						"merge_mode":      strings.TrimSpace(result.MergeMode),
						"pr_url":          strings.TrimSpace(result.PRURL),
						"action":          strings.TrimSpace(result.Action),
						"action_reason":   strings.TrimSpace(result.ActionReason),
						"gate_blocked":    result.GateBlocked,
						"gate_reasons":    stringSliceOrEmpty(result.GateReasons),
						"warnings":        stringSliceOrEmpty(result.Warnings),
						"override_reason": strings.TrimSpace(overrideReason),
					})
				}
				if strings.TrimSpace(result.MergedCommit) != "" {
					fmt.Fprintf(cmd.OutOrStdout(), "Finalized merge for CR %d as commit %s\n", id, result.MergedCommit)
				} else {
					fmt.Fprintf(cmd.OutOrStdout(), "Finalize result for CR %d: %s\n", id, nonEmpty(strings.TrimSpace(result.Action), "done"))
				}
				if strings.TrimSpace(result.PRURL) != "" {
					fmt.Fprintf(cmd.OutOrStdout(), "PR: %s\n", result.PRURL)
				}
				for _, warning := range result.Warnings {
					fmt.Fprintf(cmd.OutOrStdout(), "Warning: %s\n", warning)
				}
				return nil
			})
		},
	}

	cmd.Flags().BoolVar(&keepBranch, "keep-branch", false, "Keep CR branch after merge (default deletes merged branch)")
	cmd.Flags().StringVar(&overrideReason, "override-reason", "", "Bypass validation failures with an audited reason")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Output in JSON format")
	return cmd
}

func newCRMergeStatusCmd() *cobra.Command {
	var asJSON bool

	cmd := &cobra.Command{
		Use:     "status <id>",
		Short:   "Show merge-in-progress status for a CR",
		Example: "  sophia cr merge status 25\n  sophia cr merge status 25 --json",
		Args:    cobra.ExactArgs(1),
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
			status, err := svc.MergeStatusCR(id)
			if err != nil {
				if asJSON {
					return writeJSONError(cmd, err)
				}
				return err
			}
			if asJSON {
				return writeJSONSuccess(cmd, mergeStatusToJSONMap(status))
			}
			fmt.Fprintf(cmd.OutOrStdout(), "CR %d merge status\n", status.CRID)
			fmt.Fprintf(cmd.OutOrStdout(), "CR UID: %s\n", nonEmpty(strings.TrimSpace(status.CRUID), "-"))
			fmt.Fprintf(cmd.OutOrStdout(), "Base Branch: %s\n", status.BaseBranch)
			fmt.Fprintf(cmd.OutOrStdout(), "CR Branch: %s\n", status.CRBranch)
			fmt.Fprintf(cmd.OutOrStdout(), "Worktree: %s\n", status.WorktreePath)
			fmt.Fprintf(cmd.OutOrStdout(), "In Progress: %t\n", status.InProgress)
			fmt.Fprintf(cmd.OutOrStdout(), "Target Matches: %t\n", status.TargetMatches)
			fmt.Fprintf(cmd.OutOrStdout(), "Merge Head: %s\n", nonEmpty(strings.TrimSpace(status.MergeHead), "-"))
			printListSection(cmd, "Conflict Files", status.ConflictFiles)
			printListSection(cmd, "Advice", status.Advice)
			return nil
		},
	}

	cmd.Flags().BoolVar(&asJSON, "json", false, "Output in JSON format")
	return cmd
}

func newCRMergeAbortCmd() *cobra.Command {
	var asJSON bool

	cmd := &cobra.Command{
		Use:     "abort <id>",
		Short:   "Abort an in-progress merge for a CR",
		Example: "  sophia cr merge abort 25\n  sophia cr merge abort 25 --json",
		Args:    cobra.ExactArgs(1),
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
			if err := svc.AbortMergeCR(id); err != nil {
				if asJSON {
					return writeJSONError(cmd, err)
				}
				return err
			}
			if asJSON {
				return writeJSONSuccess(cmd, map[string]any{
					"cr_id":   id,
					"aborted": true,
				})
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Aborted in-progress merge for CR %d\n", id)
			return nil
		},
	}

	cmd.Flags().BoolVar(&asJSON, "json", false, "Output in JSON format")
	return cmd
}

func newCRMergeResumeCmd() *cobra.Command {
	var keepBranch bool
	var deleteBranch bool
	var overrideReason string
	var asJSON bool

	cmd := &cobra.Command{
		Use:     "resume <id>",
		Short:   "Resume an in-progress merge for a CR after resolving conflicts",
		Example: "  sophia cr merge resume 25\n  sophia cr merge resume 25 --keep-branch",
		Args:    cobra.ExactArgs(1),
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
			if deleteBranch {
				keepBranch = false
			}
			result, err := svc.ResumeMergeCRWithOptions(id, service.MergeCROptions{
				KeepBranch:     keepBranch,
				OverrideReason: overrideReason,
			})
			if err != nil {
				if asJSON {
					return writeJSONError(cmd, err)
				}
				return err
			}
			sha := result.MergedCommit
			warnings := append([]string(nil), result.Warnings...)
			if asJSON {
				return writeJSONSuccess(cmd, map[string]any{
					"cr_id":           id,
					"merged_commit":   sha,
					"warnings":        stringSliceOrEmpty(warnings),
					"keep_branch":     keepBranch,
					"override_reason": strings.TrimSpace(overrideReason),
				})
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Resumed merge for CR %d as commit %s\n", id, sha)
			for _, warning := range warnings {
				fmt.Fprintf(cmd.OutOrStdout(), "Warning: %s\n", warning)
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&keepBranch, "keep-branch", false, "Keep CR branch after merge (default deletes merged branch)")
	cmd.Flags().BoolVar(&deleteBranch, "delete-branch", false, "Deprecated: branch deletion is now the default")
	cmd.Flags().StringVar(&overrideReason, "override-reason", "", "Bypass validation failures with an audited reason")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Output in JSON format")
	return cmd
}
