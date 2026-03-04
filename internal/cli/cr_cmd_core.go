package cli

import (
	"fmt"
	"io"
	clicr "sophia/internal/cli/cr"
	"strings"

	"github.com/spf13/cobra"

	"sophia/internal/model"
	"sophia/internal/service"
)

type crAddRenderOptions struct {
	switchBranch    bool
	childMode       bool
	includeParentID bool
	parentCRID      int
}

func buildAddCROptions(baseRef string, parentCRID int, switchBranch bool, branchAlias string, ownerPrefix string, ownerPrefixSet bool) service.AddCROptions {
	return clicr.BuildAddCROptions(clicr.AddOptionsInput{
		BaseRef:        baseRef,
		ParentCRID:     parentCRID,
		SwitchBranch:   switchBranch,
		BranchAlias:    branchAlias,
		OwnerPrefix:    ownerPrefix,
		OwnerPrefixSet: ownerPrefixSet,
	})
}

func runCRAddFlow(
	cmd *cobra.Command,
	asJSON bool,
	svc *service.Service,
	title string,
	description string,
	opts service.AddCROptions,
	renderOpts crAddRenderOptions,
) error {
	result, err := svc.AddCRWithOptions(title, description, opts)
	if err != nil {
		return commandError(cmd, asJSON, err)
	}
	cr := result.CR
	warnings := append([]string(nil), result.Warnings...)
	bootstrap := result.Bootstrap
	if asJSON {
		payload := map[string]any{
			"cr":        crToJSONMap(cr),
			"warnings":  stringSliceOrEmpty(warnings),
			"switched":  renderOpts.switchBranch,
			"bootstrap": addCRBootstrapToJSONMap(bootstrap),
		}
		if renderOpts.includeParentID {
			payload["parent_cr_id"] = renderOpts.parentCRID
		}
		return writeJSONSuccess(cmd, payload)
	}

	prefix := "Created CR"
	if renderOpts.childMode {
		prefix = "Created child CR"
	}
	fmt.Fprintf(cmd.OutOrStdout(), "%s %d on branch %s\n", prefix, cr.ID, cr.Branch)
	if renderOpts.switchBranch {
		fmt.Fprintf(cmd.OutOrStdout(), "Active branch: %s\n", cr.Branch)
	} else {
		fmt.Fprintf(cmd.OutOrStdout(), "Run: sophia cr switch %d\n", cr.ID)
	}
	if bootstrap.Triggered {
		fmt.Fprintf(cmd.OutOrStdout(), "Bootstrapped local Sophia metadata (base: %s, mode: %s)\n", nonEmpty(bootstrap.BaseBranch, "-"), nonEmpty(bootstrap.MetadataMode, "-"))
	}
	if len(warnings) > 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "Overlap warnings:")
		for _, warning := range warnings {
			fmt.Fprintf(cmd.OutOrStdout(), "- %s\n", warning)
		}
	}
	return nil
}

func newCRAddCmd() *cobra.Command {
	var description string
	var baseRef string
	var parentID int
	var switchBranch bool
	var branchAlias string
	var ownerPrefix string
	var asJSON bool

	cmd := &cobra.Command{
		Use:   "add <title>",
		Short: "Create a new change request",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, err := newServiceForCmd(cmd)
			if err != nil {
				return commandError(cmd, asJSON, err)
			}
			opts := buildAddCROptions(baseRef, parentID, switchBranch, branchAlias, ownerPrefix, cmd.Flags().Changed("owner-prefix"))
			return runCRAddFlow(cmd, asJSON, svc, args[0], description, opts, crAddRenderOptions{
				switchBranch: switchBranch,
			})
		},
	}

	cmd.Flags().StringVar(&description, "description", "", "Description/rationale for the CR")
	cmd.Flags().StringVar(&baseRef, "base", "", "Base Git ref for this CR")
	cmd.Flags().IntVar(&parentID, "parent", 0, "Parent CR id for stacked workflow")
	cmd.Flags().BoolVar(&switchBranch, "switch", false, "Switch to the CR branch immediately after creation")
	cmd.Flags().StringVar(&branchAlias, "branch-alias", "", "Explicit branch alias (cr-<id>-<slug> or cr-<slug>-<uid4|uid6|uid8>)")
	cmd.Flags().StringVar(&ownerPrefix, "owner-prefix", "", "Optional owner prefix for generated branch alias")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Output in JSON format")
	return cmd
}

func newCRApplyCmd() *cobra.Command {
	var filePath string
	var stdin bool
	var dryRun bool
	var keepFile bool
	var asJSON bool

	cmd := &cobra.Command{
		Use:   "apply",
		Short: "Apply a strict YAML CR plan",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if stdin && strings.TrimSpace(filePath) != "" {
				err := fmt.Errorf("invalid arguments: --stdin and --file are mutually exclusive")
				return commandError(cmd, asJSON, err)
			}
			if !stdin && strings.TrimSpace(filePath) == "" {
				err := fmt.Errorf("invalid arguments: one of --file or --stdin is required")
				return commandError(cmd, asJSON, err)
			}
			svc, err := newServiceForCmd(cmd)
			if err != nil {
				return commandError(cmd, asJSON, err)
			}
			applyOpts := service.ApplyCRPlanOptions{
				FilePath: resolvePathForCmd(cmd, filePath),
				DryRun:   dryRun,
				KeepFile: keepFile,
			}
			if stdin {
				input, readErr := io.ReadAll(cmd.InOrStdin())
				if readErr != nil {
					return commandError(cmd, asJSON, fmt.Errorf("read stdin plan: %w", readErr))
				}
				applyOpts.FilePath = ""
				applyOpts.PlanYAML = string(input)
				applyOpts.SourceName = "stdin"
				applyOpts.KeepFile = true
			}
			result, err := svc.ApplyCRPlan(applyOpts)
			if err != nil {
				return commandError(cmd, asJSON, err)
			}
			if asJSON {
				return writeJSONSuccess(cmd, applyPlanToJSONMap(result))
			}

			mode := "apply"
			if dryRun {
				mode = "dry-run"
			}
			fmt.Fprintf(cmd.OutOrStdout(), "CR plan %s completed for %s\n", mode, result.FilePath)
			fmt.Fprintf(cmd.OutOrStdout(), "Consumed: %t\n", result.Consumed)
			fmt.Fprintln(cmd.OutOrStdout(), "\nPlanned Operations:")
			if len(result.PlannedOperations) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "- (none)")
			} else {
				for _, op := range result.PlannedOperations {
					fmt.Fprintf(cmd.OutOrStdout(), "- %s\n", op)
				}
			}
			fmt.Fprintln(cmd.OutOrStdout(), "\nCreated CRs:")
			if len(result.CreatedCRs) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "- (none)")
			} else {
				for _, created := range result.CreatedCRs {
					fmt.Fprintf(cmd.OutOrStdout(), "- key=%s id=%d uid=%s branch=%s parent_cr_id=%d\n", created.Key, created.ID, nonEmpty(created.UID, "-"), created.Branch, created.ParentCRID)
				}
			}
			fmt.Fprintln(cmd.OutOrStdout(), "\nCreated Tasks:")
			if len(result.CreatedTasks) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "- (none)")
			} else {
				for _, created := range result.CreatedTasks {
					fmt.Fprintf(cmd.OutOrStdout(), "- cr_key=%s task_key=%s task_id=%d\n", created.CRKey, created.TaskKey, created.TaskID)
				}
			}
			fmt.Fprintln(cmd.OutOrStdout(), "\nDelegations:")
			if len(result.Delegations) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "- (none)")
			} else {
				for _, delegation := range result.Delegations {
					fmt.Fprintf(cmd.OutOrStdout(), "- parent_cr_key=%s parent_task_key=%s child_cr_key=%s child_task_id=%d\n", delegation.ParentCRKey, delegation.ParentTaskKey, delegation.ChildCRKey, delegation.ChildTaskID)
				}
			}
			fmt.Fprintln(cmd.OutOrStdout(), "\nWarnings:")
			if len(result.Warnings) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "- (none)")
			} else {
				for _, warning := range result.Warnings {
					fmt.Fprintf(cmd.OutOrStdout(), "- %s\n", warning)
				}
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&filePath, "file", "", "Path to YAML plan file")
	cmd.Flags().BoolVar(&stdin, "stdin", false, "Read YAML plan from stdin")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Validate and preview plan without mutating repository state")
	cmd.Flags().BoolVar(&keepFile, "keep-file", false, "Keep source plan file after successful apply")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Output in JSON format")
	return cmd
}

func newCRChildCmd() *cobra.Command {
	childCmd := &cobra.Command{
		Use:   "child",
		Short: "Manage child CRs from the active CR context",
	}
	childCmd.AddCommand(newCRChildAddCmd())
	return childCmd
}

func newCRChildAddCmd() *cobra.Command {
	var description string
	var switchBranch bool
	var branchAlias string
	var ownerPrefix string
	var asJSON bool

	cmd := &cobra.Command{
		Use:   "add <title>",
		Short: "Create a child CR from the current CR",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, err := newServiceForCmd(cmd)
			if err != nil {
				return commandError(cmd, asJSON, err)
			}
			ctx, ctxErr := svc.CurrentCR()
			if ctxErr != nil {
				if errorsIs(ctxErr, service.ErrNoActiveCRContext) {
					ctxErr = fmt.Errorf("current branch is not a CR branch; run `sophia cr switch <id>` or use `sophia cr add <title> --parent <id>`")
				}
				return commandError(cmd, asJSON, ctxErr)
			}
			opts := buildAddCROptions("", ctx.CR.ID, switchBranch, branchAlias, ownerPrefix, cmd.Flags().Changed("owner-prefix"))
			return runCRAddFlow(cmd, asJSON, svc, args[0], description, opts, crAddRenderOptions{
				switchBranch:    switchBranch,
				childMode:       true,
				includeParentID: true,
				parentCRID:      ctx.CR.ID,
			})
		},
	}

	cmd.Flags().StringVar(&description, "description", "", "Description/rationale for the child CR")
	cmd.Flags().BoolVar(&switchBranch, "switch", false, "Switch to the child CR branch immediately after creation")
	cmd.Flags().StringVar(&branchAlias, "branch-alias", "", "Explicit branch alias (cr-<id>-<slug> or cr-<slug>-<uid4|uid6|uid8>)")
	cmd.Flags().StringVar(&ownerPrefix, "owner-prefix", "", "Optional owner prefix for generated branch alias")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Output in JSON format")
	return cmd
}

type crSearchCommandFilters struct {
	status          string
	scope           string
	riskTier        string
	text            string
	search          string
	positionalQuery string
	hasQueryArg     bool
}

func resolveCRSearchQuery(filters crSearchCommandFilters) (model.CRSearchQuery, error) {
	normalizedStatus, err := normalizeCRStatusFilter(filters.status)
	if err != nil {
		return model.CRSearchQuery{}, err
	}
	normalizedRiskTier, err := normalizeRiskTierFilter(filters.riskTier)
	if err != nil {
		return model.CRSearchQuery{}, err
	}

	searchText := strings.TrimSpace(filters.text)
	searchFlag := strings.TrimSpace(filters.search)
	if searchText != "" && searchFlag != "" && searchText != searchFlag {
		return model.CRSearchQuery{}, fmt.Errorf("invalid argument: --text and --search must match when both are provided")
	}
	if searchFlag != "" {
		searchText = searchFlag
	}
	if filters.hasQueryArg {
		argText := strings.TrimSpace(filters.positionalQuery)
		if searchText != "" && searchText != argText {
			return model.CRSearchQuery{}, fmt.Errorf("invalid argument: positional query and --text/--search must match when both are provided")
		}
		searchText = argText
	}

	return model.CRSearchQuery{
		Status:      normalizedStatus,
		ScopePrefix: filters.scope,
		RiskTier:    normalizedRiskTier,
		Text:        searchText,
	}, nil
}

func runCRSearchQuery(cmd *cobra.Command, query model.CRSearchQuery) ([]model.CRSearchResult, error) {
	svc, err := newServiceForCmd(cmd)
	if err != nil {
		return nil, err
	}
	return svc.SearchCRs(query)
}

func renderCRSearchResults(cmd *cobra.Command, asJSON bool, results []model.CRSearchResult, includeFoundHeading bool) error {
	if asJSON {
		items := make([]map[string]any, 0, len(results))
		for _, result := range results {
			items = append(items, crSearchResultToJSONMap(result))
		}
		return writeJSONSuccess(cmd, map[string]any{
			"count":   len(results),
			"results": items,
		})
	}
	if len(results) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "No CRs found.")
		return nil
	}
	if includeFoundHeading {
		fmt.Fprintf(cmd.OutOrStdout(), "Found %d CR(s):\n", len(results))
	}
	fmt.Fprintln(cmd.OutOrStdout(), "ID\tSTATUS\tRISK\tBRANCH\tTITLE")
	for _, result := range results {
		fmt.Fprintf(cmd.OutOrStdout(), "%d\t%s\t%s\t%s\t%s\n", result.ID, result.Status, result.RiskTier, result.Branch, result.Title)
	}
	return nil
}

func newCRListCmd() *cobra.Command {
	var status string
	var scope string
	var riskTier string
	var text string
	var search string
	var asJSON bool

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all change requests",
		RunE: func(cmd *cobra.Command, args []string) error {
			query, err := resolveCRSearchQuery(crSearchCommandFilters{
				status:   status,
				scope:    scope,
				riskTier: riskTier,
				text:     text,
				search:   search,
			})
			if err != nil {
				return commandError(cmd, asJSON, err)
			}
			results, err := runCRSearchQuery(cmd, query)
			if err != nil {
				return commandError(cmd, asJSON, err)
			}
			return renderCRSearchResults(cmd, asJSON, results, false)
		},
	}

	cmd.Flags().StringVar(&status, "status", "", "Filter by status (in_progress, merged)")
	cmd.Flags().StringVar(&scope, "scope", "", "Filter by contract scope prefix")
	cmd.Flags().StringVar(&riskTier, "risk-tier", "", "Filter by risk tier (low, medium, high)")
	cmd.Flags().StringVar(&text, "text", "", "Search in title, description, notes, contract")
	cmd.Flags().StringVar(&search, "search", "", "Alias for --text")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Output in JSON format")
	return cmd
}

func newCRSearchCmd() *cobra.Command {
	var status string
	var scope string
	var riskTier string
	var text string
	var search string
	var asJSON bool

	cmd := &cobra.Command{
		Use:   "search [query]",
		Short: "Search change requests by text and filters",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			filters := crSearchCommandFilters{
				status:   status,
				scope:    scope,
				riskTier: riskTier,
				text:     text,
				search:   search,
			}
			if len(args) > 0 {
				filters.hasQueryArg = true
				filters.positionalQuery = args[0]
			}
			query, err := resolveCRSearchQuery(filters)
			if err != nil {
				return commandError(cmd, asJSON, err)
			}
			results, err := runCRSearchQuery(cmd, query)
			if err != nil {
				return commandError(cmd, asJSON, err)
			}
			return renderCRSearchResults(cmd, asJSON, results, true)
		},
	}

	cmd.Flags().StringVar(&status, "status", "", "Filter by status (in_progress, merged)")
	cmd.Flags().StringVar(&scope, "scope", "", "Filter by contract scope prefix")
	cmd.Flags().StringVar(&riskTier, "risk-tier", "", "Filter by risk tier (low, medium, high)")
	cmd.Flags().StringVar(&text, "text", "", "Search in title, description, notes, contract")
	cmd.Flags().StringVar(&search, "search", "", "Alias for --text")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Output in JSON format")
	return cmd
}

func newCRStackCmd() *cobra.Command {
	var asJSON bool

	cmd := &cobra.Command{
		Use:   "stack [id]",
		Short: "Show stack topology and merge blockers for related CRs",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, err := newServiceForCmd(cmd)
			if err != nil {
				return commandError(cmd, asJSON, err)
			}

			var stack *service.StackView
			if len(args) == 0 {
				stack, err = svc.StackCurrentCR()
			} else {
				id, parseErr := parsePositiveIntArg(args[0], "id")
				if parseErr != nil {
					return commandError(cmd, asJSON, parseErr)
				}
				stack, err = svc.StackCR(id)
			}
			if err != nil {
				return commandError(cmd, asJSON, err)
			}

			if asJSON {
				nodes := make([]map[string]any, 0, len(stack.Nodes))
				for _, node := range stack.Nodes {
					nodes = append(nodes, map[string]any{
						"id":                      node.ID,
						"uid":                     node.UID,
						"parent_cr_id":            node.ParentCRID,
						"title":                   node.Title,
						"status":                  node.Status,
						"branch":                  node.Branch,
						"depth":                   node.Depth,
						"children":                node.Children,
						"merge_blocked":           node.MergeBlocked,
						"merge_blockers":          node.MergeBlockers,
						"tasks_total":             node.TasksTotal,
						"tasks_open":              node.TasksOpen,
						"tasks_done":              node.TasksDone,
						"tasks_delegated":         node.TasksDelegated,
						"tasks_delegated_pending": node.TasksDelegatedPending,
					})
				}
				return writeJSONSuccess(cmd, map[string]any{
					"root_cr_id":  stack.RootCRID,
					"focus_cr_id": stack.FocusCRID,
					"nodes":       nodes,
				})
			}

			fmt.Fprintf(cmd.OutOrStdout(), "CR Stack (root=%d, focus=%d)\n", stack.RootCRID, stack.FocusCRID)
			for _, node := range stack.Nodes {
				indent := strings.Repeat("  ", node.Depth)
				fmt.Fprintf(cmd.OutOrStdout(), "%s- CR %d [%s] %s\n", indent, node.ID, node.Status, node.Title)
				fmt.Fprintf(cmd.OutOrStdout(), "%s  branch=%s tasks=%d open=%d delegated=%d(%d pending) done=%d merge_blocked=%t\n", indent, node.Branch, node.TasksTotal, node.TasksOpen, node.TasksDelegated, node.TasksDelegatedPending, node.TasksDone, node.MergeBlocked)
				if len(node.MergeBlockers) > 0 {
					for _, blocker := range node.MergeBlockers {
						fmt.Fprintf(cmd.OutOrStdout(), "%s  blocker: %s\n", indent, blocker)
					}
				}
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&asJSON, "json", false, "Output in JSON format")
	return cmd
}

func newCRWhyCmd() *cobra.Command {
	var asJSON bool

	cmd := &cobra.Command{
		Use:   "why [id]",
		Short: "Show the rationale for why a CR exists",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withOptionalCRIDAndService(cmd, asJSON, args, "id", func(id int, svc *service.Service) error {
				why, err := svc.WhyCR(id)
				if err != nil {
					return commandError(cmd, asJSON, err)
				}
				if asJSON {
					return writeJSONSuccess(cmd, map[string]any{
						"cr_id":               why.CRID,
						"cr_uid":              why.CRUID,
						"base_ref":            why.BaseRef,
						"base_commit":         why.BaseCommit,
						"parent_cr_id":        why.ParentCRID,
						"effective_why":       why.EffectiveWhy,
						"source":              why.Source,
						"description":         why.Description,
						"contract_why":        why.ContractWhy,
						"contract_updated_at": why.ContractUpdatedAt,
						"contract_updated_by": why.ContractUpdatedBy,
					})
				}

				fmt.Fprintf(cmd.OutOrStdout(), "CR %d Why:\n", why.CRID)
				fmt.Fprintf(cmd.OutOrStdout(), "- effective_why: %s\n", nonEmpty(why.EffectiveWhy, "(missing)"))
				fmt.Fprintf(cmd.OutOrStdout(), "- source: %s\n", nonEmpty(why.Source, "missing"))
				fmt.Fprintf(cmd.OutOrStdout(), "- description: %s\n", nonEmpty(why.Description, "(missing)"))
				fmt.Fprintf(cmd.OutOrStdout(), "- contract_why: %s\n", nonEmpty(why.ContractWhy, "(missing)"))
				fmt.Fprintf(cmd.OutOrStdout(), "- base_ref: %s\n", nonEmpty(why.BaseRef, "(missing)"))
				fmt.Fprintf(cmd.OutOrStdout(), "- base_commit: %s\n", nonEmpty(why.BaseCommit, "(missing)"))
				fmt.Fprintf(cmd.OutOrStdout(), "- parent_cr_id: %d\n", why.ParentCRID)
				fmt.Fprintf(cmd.OutOrStdout(), "- contract_updated_at: %s\n", nonEmpty(why.ContractUpdatedAt, "(never)"))
				fmt.Fprintf(cmd.OutOrStdout(), "- contract_updated_by: %s\n", nonEmpty(why.ContractUpdatedBy, "(never)"))
				return nil
			})
		},
	}

	cmd.Flags().BoolVar(&asJSON, "json", false, "Output in JSON format")
	return cmd
}

func newCRStatusCmd() *cobra.Command {
	var asJSON bool
	var includeHQ bool

	cmd := &cobra.Command{
		Use:   "status [id]",
		Short: "Show CR merge-readiness and workspace status",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withOptionalCRIDAndService(cmd, asJSON, args, "id", func(id int, svc *service.Service) error {
				status, err := svc.StatusCR(id)
				if err != nil {
					return commandError(cmd, asJSON, err)
				}
				var hqStatus *service.HQSyncStatusView
				if includeHQ {
					hqStatus, err = svc.HQSyncStatusCR(id)
					if err != nil {
						return commandError(cmd, asJSON, err)
					}
				}
				if asJSON {
					payload := crStatusToJSONMap(status)
					if includeHQ {
						payload["hq_sync"] = hqSyncStatusToJSONMap(hqStatus)
					}
					return writeJSONSuccess(cmd, payload)
				}

				fmt.Fprintf(cmd.OutOrStdout(), "CR %d: %s\n", status.ID, status.Title)
				fmt.Fprintf(cmd.OutOrStdout(), "Status: %s\n", status.Status)
				fmt.Fprintf(cmd.OutOrStdout(), "Base: %s\n", status.BaseBranch)
				fmt.Fprintf(cmd.OutOrStdout(), "Base Ref: %s\n", nonEmpty(status.BaseRef, "(missing)"))
				fmt.Fprintf(cmd.OutOrStdout(), "Base Commit: %s\n", nonEmpty(status.BaseCommit, "(missing)"))
				fmt.Fprintf(cmd.OutOrStdout(), "Parent CR: %d (%s)\n", status.ParentCRID, nonEmpty(status.ParentStatus, "-"))
				fmt.Fprintf(cmd.OutOrStdout(), "Branch: %s\n", status.Branch)
				fmt.Fprintf(cmd.OutOrStdout(), "Current Branch: %s\n", nonEmpty(status.CurrentBranch, "(unknown)"))
				fmt.Fprintf(cmd.OutOrStdout(), "Branch Match: %t\n", status.BranchMatch)
				fmt.Fprintf(cmd.OutOrStdout(), "Current Worktree: %s\n", nonEmpty(strings.TrimSpace(status.CurrentWorktreePath), "(unknown)"))
				fmt.Fprintf(cmd.OutOrStdout(), "Owner Worktree: %s\n", nonEmpty(strings.TrimSpace(status.OwnerWorktreePath), "(not checked out in any worktree)"))
				fmt.Fprintf(cmd.OutOrStdout(), "Owner Is Current: %t\n", status.OwnerIsCurrentWorktree)
				fmt.Fprintf(cmd.OutOrStdout(), "Checked Out In Other Worktree: %t\n", status.CheckedOutInOtherWorktree)
				if status.CheckedOutInOtherWorktree && strings.TrimSpace(status.SuggestedWorktreeCommand) != "" {
					fmt.Fprintf(cmd.OutOrStdout(), "Suggested Command: %s\n", status.SuggestedWorktreeCommand)
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Working Tree: %d modified/staged, %d untracked (dirty=%t)\n", status.ModifiedStagedCount, status.UntrackedCount, status.Dirty)
				fmt.Fprintf(cmd.OutOrStdout(), "Tasks: %d total, %d open, %d delegated (%d pending), %d done\n", status.TasksTotal, status.TasksOpen, status.TasksDelegated, status.TasksDelegatedPending, status.TasksDone)
				fmt.Fprintf(cmd.OutOrStdout(), "Contract Complete: %t\n", status.ContractComplete)
				if len(status.ContractMissingFields) == 0 {
					fmt.Fprintln(cmd.OutOrStdout(), "Contract Missing Fields: (none)")
				} else {
					fmt.Fprintf(cmd.OutOrStdout(), "Contract Missing Fields: %s\n", strings.Join(status.ContractMissingFields, ", "))
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Validation: valid=%t errors=%d warnings=%d risk=%s/%d\n", status.ValidationValid, status.ValidationErrors, status.ValidationWarnings, status.RiskTier, status.RiskScore)
				fmt.Fprintf(cmd.OutOrStdout(), "Merge Blocked: %t\n", status.MergeBlocked)
				if len(status.MergeBlockers) == 0 {
					fmt.Fprintln(cmd.OutOrStdout(), "Merge Blockers: (none)")
				} else {
					fmt.Fprintln(cmd.OutOrStdout(), "Merge Blockers:")
					for _, blocker := range status.MergeBlockers {
						fmt.Fprintf(cmd.OutOrStdout(), "- %s\n", blocker)
					}
				}
				if strings.TrimSpace(status.PRLinkageState) != "" {
					fmt.Fprintf(cmd.OutOrStdout(), "PR Linkage State: %s\n", status.PRLinkageState)
				}
				if strings.TrimSpace(status.ActionRequired) != "" {
					fmt.Fprintf(cmd.OutOrStdout(), "Action Required: %s\n", status.ActionRequired)
					fmt.Fprintf(cmd.OutOrStdout(), "Action Reason: %s\n", nonEmpty(status.ActionReason, "-"))
					if len(status.SuggestedCommands) == 0 {
						fmt.Fprintln(cmd.OutOrStdout(), "Suggested Commands: (none)")
					} else {
						fmt.Fprintln(cmd.OutOrStdout(), "Suggested Commands:")
						for _, suggested := range status.SuggestedCommands {
							fmt.Fprintf(cmd.OutOrStdout(), "- %s\n", suggested)
						}
					}
				}
				if includeHQ && hqStatus != nil {
					fmt.Fprintln(cmd.OutOrStdout(), "\nHQ Sync:")
					fmt.Fprintf(cmd.OutOrStdout(), "- configured: %t\n", hqStatus.Configured)
					fmt.Fprintf(cmd.OutOrStdout(), "- base_url: %s\n", nonEmpty(hqStatus.BaseURL, "(missing)"))
					fmt.Fprintf(cmd.OutOrStdout(), "- repo_id: %s\n", nonEmpty(hqStatus.RepoID, "(missing)"))
					fmt.Fprintf(cmd.OutOrStdout(), "- remote_alias: %s\n", nonEmpty(hqStatus.RemoteAlias, "(missing)"))
					fmt.Fprintf(cmd.OutOrStdout(), "- has_token: %t\n", hqStatus.HasToken)
					fmt.Fprintf(cmd.OutOrStdout(), "- linked: %t\n", hqStatus.Linked)
					fmt.Fprintf(cmd.OutOrStdout(), "- state: %s\n", nonEmpty(hqStatus.State, "(missing)"))
					if len(hqStatus.SuggestedActions) == 0 {
						fmt.Fprintln(cmd.OutOrStdout(), "- suggested_actions: (none)")
					} else {
						fmt.Fprintln(cmd.OutOrStdout(), "- suggested_actions:")
						for _, action := range hqStatus.SuggestedActions {
							fmt.Fprintf(cmd.OutOrStdout(), "  - %s\n", action)
						}
					}
				}
				return nil
			})
		},
	}

	cmd.Flags().BoolVar(&asJSON, "json", false, "Output in JSON format")
	cmd.Flags().BoolVar(&includeHQ, "hq", false, "Include HQ intent sync status")
	return cmd
}

func newCREditCmd() *cobra.Command {
	var title string
	var description string
	var asJSON bool

	cmd := &cobra.Command{
		Use:   "edit [id]",
		Short: "Edit CR title/description with audit trail",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			titleChanged := cmd.Flags().Changed("title")
			descriptionChanged := cmd.Flags().Changed("description")
			if !titleChanged && !descriptionChanged {
				err := fmt.Errorf("provide at least one of --title or --description")
				return commandError(cmd, asJSON, err)
			}

			var titlePtr *string
			var descriptionPtr *string
			if titleChanged {
				titlePtr = &title
			}
			if descriptionChanged {
				descriptionPtr = &description
			}
			return withOptionalCRIDAndService(cmd, asJSON, args, "id", func(id int, svc *service.Service) error {
				changedFields, err := svc.EditCR(id, titlePtr, descriptionPtr)
				if err != nil {
					return commandError(cmd, asJSON, err)
				}
				if asJSON {
					return writeJSONSuccess(cmd, map[string]any{
						"cr_id":          id,
						"changed_fields": stringSliceOrEmpty(changedFields),
					})
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Updated CR %d fields: %s\n", id, strings.Join(changedFields, ", "))
				return nil
			})
		},
	}

	cmd.Flags().StringVar(&title, "title", "", "New CR title")
	cmd.Flags().StringVar(&description, "description", "", "New CR description")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Output in JSON format")
	return cmd
}

func newCRContractCmd() *cobra.Command {
	contractCmd := &cobra.Command{
		Use:   "contract",
		Short: "Manage CR intent contract fields",
	}
	contractCmd.AddCommand(newCRContractSetCmd())
	contractCmd.AddCommand(newCRContractShowCmd())
	contractCmd.AddCommand(newCRContractDriftCmd())
	return contractCmd
}

func newCRContractSetCmd() *cobra.Command {
	var why string
	var scope []string
	var nonGoals []string
	var invariants []string
	var blastRadius string
	var riskCriticalScopes []string
	var riskTierHint string
	var riskRationale string
	var testPlan string
	var rollbackPlan string
	var changeReason string
	var dryRun bool
	var asJSON bool

	cmd := &cobra.Command{
		Use:     "set [id]",
		Short:   "Set/update CR intent contract fields",
		Example: "  sophia cr contract set 25 --why \"Reduce merge churn\" --scope internal/service --scope internal/cli\n  sophia cr contract set 25 --risk-critical-scope internal/service --risk-tier-hint medium --risk-rationale \"Touches merge behavior\"\n  sophia cr contract set 25 --test-plan \"go test ./... && go vet ./...\" --rollback-plan \"Revert [CR-25] merge commit\"",
		Args:    cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			patch := service.ContractPatch{}
			if cmd.Flags().Changed("why") {
				v := why
				patch.Why = &v
			}
			if cmd.Flags().Changed("scope") {
				v := append([]string(nil), scope...)
				patch.Scope = &v
			}
			if cmd.Flags().Changed("non-goal") {
				v := append([]string(nil), nonGoals...)
				patch.NonGoals = &v
			}
			if cmd.Flags().Changed("invariant") {
				v := append([]string(nil), invariants...)
				patch.Invariants = &v
			}
			if cmd.Flags().Changed("blast-radius") {
				v := blastRadius
				patch.BlastRadius = &v
			}
			if cmd.Flags().Changed("risk-critical-scope") {
				v := append([]string(nil), riskCriticalScopes...)
				patch.RiskCriticalScopes = &v
			}
			if cmd.Flags().Changed("risk-tier-hint") {
				v := riskTierHint
				patch.RiskTierHint = &v
			}
			if cmd.Flags().Changed("risk-rationale") {
				v := riskRationale
				patch.RiskRationale = &v
			}
			if cmd.Flags().Changed("test-plan") {
				v := testPlan
				patch.TestPlan = &v
			}
			if cmd.Flags().Changed("rollback-plan") {
				v := rollbackPlan
				patch.RollbackPlan = &v
			}
			if cmd.Flags().Changed("change-reason") {
				v := changeReason
				patch.ChangeReason = &v
			}
			if patch.Why == nil && patch.Scope == nil && patch.NonGoals == nil && patch.Invariants == nil && patch.BlastRadius == nil && patch.RiskCriticalScopes == nil && patch.RiskTierHint == nil && patch.RiskRationale == nil && patch.TestPlan == nil && patch.RollbackPlan == nil {
				err := fmt.Errorf("provide at least one contract field flag")
				return commandError(cmd, asJSON, err)
			}
			return withOptionalCRIDAndService(cmd, asJSON, args, "id", func(id int, svc *service.Service) error {
				result, err := svc.SetCRContractWithOptions(id, patch, service.SetCRContractOptions{
					DryRun: dryRun,
				})
				if err != nil {
					return commandError(cmd, asJSON, err)
				}
				if asJSON {
					return writeJSONSuccess(cmd, map[string]any{
						"cr_id":           id,
						"changed_fields":  stringSliceOrEmpty(result.ChangedFields),
						"already_applied": result.AlreadyApplied,
						"dry_run":         dryRun,
					})
				}
				if result.AlreadyApplied {
					if dryRun {
						fmt.Fprintf(cmd.OutOrStdout(), "Dry-run: CR %d contract already matches requested values\n", id)
						return nil
					}
					fmt.Fprintf(cmd.OutOrStdout(), "CR %d contract already matches requested values\n", id)
					return nil
				}
				if dryRun {
					fmt.Fprintf(cmd.OutOrStdout(), "Dry-run: CR %d contract update valid; fields would change: %s\n", id, strings.Join(result.ChangedFields, ", "))
					return nil
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Updated CR %d contract fields: %s\n", id, strings.Join(result.ChangedFields, ", "))
				return nil
			})
		},
	}

	cmd.Flags().StringVar(&why, "why", "", "Intent rationale")
	cmd.Flags().StringArrayVar(&scope, "scope", nil, "Repo-relative scope prefix (repeatable)")
	cmd.Flags().StringArrayVar(&nonGoals, "non-goal", nil, "Explicit non-goal (repeatable)")
	cmd.Flags().StringArrayVar(&invariants, "invariant", nil, "Invariant that must hold (repeatable)")
	cmd.Flags().StringVar(&blastRadius, "blast-radius", "", "Expected blast radius")
	cmd.Flags().StringArrayVar(&riskCriticalScopes, "risk-critical-scope", nil, "CR-authored critical scope prefix for impact scoring (repeatable)")
	cmd.Flags().StringVar(&riskTierHint, "risk-tier-hint", "", "Optional risk tier hint floor (low, medium, high)")
	cmd.Flags().StringVar(&riskRationale, "risk-rationale", "", "Optional rationale for risk hint choices")
	cmd.Flags().StringVar(&testPlan, "test-plan", "", "Planned validation/testing approach")
	cmd.Flags().StringVar(&rollbackPlan, "rollback-plan", "", "Rollback strategy")
	cmd.Flags().StringVar(&changeReason, "change-reason", "", "Reason for post-checkpoint scope change")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Validate contract changes without mutating CR metadata")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Output in JSON format")
	return cmd
}

func newCRContractShowCmd() *cobra.Command {
	var asJSON bool

	cmd := &cobra.Command{
		Use:     "show <id>",
		Short:   "Show CR intent contract fields",
		Example: "  sophia cr contract show 25",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, svc, err := parseIDAndService(cmd, args[0], "id")
			if err != nil {
				return commandError(cmd, asJSON, err)
			}
			contract, err := svc.GetCRContract(id)
			if err != nil {
				return commandError(cmd, asJSON, err)
			}
			baseline, err := svc.GetCRContractBaseline(id)
			if err != nil {
				return commandError(cmd, asJSON, err)
			}
			drifts, err := svc.ListCRContractDrifts(id)
			if err != nil {
				return commandError(cmd, asJSON, err)
			}
			if asJSON {
				return writeJSONSuccess(cmd, map[string]any{
					"cr_id":             id,
					"contract":          contractToJSONMap(*contract),
					"contract_baseline": crContractBaselineToJSONMap(*baseline),
					"contract_drifts":   crContractDriftMaps(drifts),
				})
			}
			fmt.Fprintln(cmd.OutOrStdout(), "Contract:")
			fmt.Fprintf(cmd.OutOrStdout(), "- why: %s\n", nonEmpty(strings.TrimSpace(contract.Why), "(missing)"))
			printValueList(cmd, "scope", contract.Scope)
			printValueList(cmd, "non_goals", contract.NonGoals)
			printValueList(cmd, "invariants", contract.Invariants)
			fmt.Fprintf(cmd.OutOrStdout(), "- blast_radius: %s\n", nonEmpty(strings.TrimSpace(contract.BlastRadius), "(missing)"))
			printValueList(cmd, "risk_critical_scopes", contract.RiskCriticalScopes)
			fmt.Fprintf(cmd.OutOrStdout(), "- risk_tier_hint: %s\n", nonEmpty(strings.TrimSpace(contract.RiskTierHint), "(none)"))
			fmt.Fprintf(cmd.OutOrStdout(), "- risk_rationale: %s\n", nonEmpty(strings.TrimSpace(contract.RiskRationale), "(none)"))
			fmt.Fprintf(cmd.OutOrStdout(), "- test_plan: %s\n", nonEmpty(strings.TrimSpace(contract.TestPlan), "(missing)"))
			fmt.Fprintf(cmd.OutOrStdout(), "- rollback_plan: %s\n", nonEmpty(strings.TrimSpace(contract.RollbackPlan), "(missing)"))
			fmt.Fprintf(cmd.OutOrStdout(), "- updated_at: %s\n", nonEmpty(strings.TrimSpace(contract.UpdatedAt), "(never)"))
			fmt.Fprintf(cmd.OutOrStdout(), "- updated_by: %s\n", nonEmpty(strings.TrimSpace(contract.UpdatedBy), "(never)"))
			fmt.Fprintf(cmd.OutOrStdout(), "- baseline_captured_at: %s\n", nonEmpty(strings.TrimSpace(baseline.CapturedAt), "(not captured)"))
			if len(baseline.Scope) > 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "- baseline_scope: %s\n", strings.Join(baseline.Scope, ", "))
			}
			fmt.Fprintf(cmd.OutOrStdout(), "- contract_drifts: %d\n", len(drifts))
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "Output in JSON format")
	return cmd
}

func newCRContractDriftCmd() *cobra.Command {
	driftCmd := &cobra.Command{
		Use:   "drift",
		Short: "Inspect and acknowledge CR contract drift records",
	}
	driftCmd.AddCommand(newCRContractDriftListCmd())
	driftCmd.AddCommand(newCRContractDriftAckCmd())
	return driftCmd
}

func newCRContractDriftListCmd() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "list <id>",
		Short: "List CR contract drift records",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, svc, err := parseIDAndService(cmd, args[0], "id")
			if err != nil {
				return commandError(cmd, asJSON, err)
			}
			drifts, err := svc.ListCRContractDrifts(id)
			if err != nil {
				return commandError(cmd, asJSON, err)
			}
			if asJSON {
				return writeJSONSuccess(cmd, map[string]any{
					"cr_id":  id,
					"drifts": crContractDriftMaps(drifts),
				})
			}
			if len(drifts) == 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "No CR contract drift records for CR %d.\n", id)
				return nil
			}
			fmt.Fprintln(cmd.OutOrStdout(), "DRIFT_ID\tFIELDS\tACKNOWLEDGED\tTS")
			for _, drift := range drifts {
				fmt.Fprintf(cmd.OutOrStdout(), "%d\t%s\t%t\t%s\n", drift.ID, strings.Join(drift.Fields, ","), drift.Acknowledged, drift.TS)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "Output in JSON format")
	return cmd
}

func newCRContractDriftAckCmd() *cobra.Command {
	var reason string
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "ack <id> <drift-id>",
		Short: "Acknowledge a CR contract drift record",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parsePositiveIntArg(args[0], "id")
			if err != nil {
				return commandError(cmd, asJSON, err)
			}
			driftID, err := parsePositiveIntArg(args[1], "drift-id")
			if err != nil {
				return commandError(cmd, asJSON, err)
			}
			if strings.TrimSpace(reason) == "" {
				err := fmt.Errorf("--reason is required")
				return commandError(cmd, asJSON, err)
			}
			svc, err := newServiceForCmd(cmd)
			if err != nil {
				return commandError(cmd, asJSON, err)
			}
			drift, err := svc.AckCRContractDrift(id, driftID, reason)
			if err != nil {
				return commandError(cmd, asJSON, err)
			}
			if asJSON {
				return writeJSONSuccess(cmd, map[string]any{
					"cr_id": id,
					"drift": crContractDriftToJSONMap(*drift),
				})
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Acknowledged CR %d contract drift %d.\n", id, driftID)
			return nil
		},
	}
	cmd.Flags().StringVar(&reason, "reason", "", "Acknowledgement reason")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Output in JSON format")
	return cmd
}

func newCRImpactCmd() *cobra.Command {
	var asJSON bool

	cmd := &cobra.Command{
		Use:   "impact [id]",
		Short: "Show deterministic impact and risk summary for a CR",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withOptionalCRIDAndService(cmd, asJSON, args, "id", func(id int, svc *service.Service) error {
				impact, err := svc.ImpactCR(id)
				if err != nil {
					return commandError(cmd, asJSON, err)
				}
				if asJSON {
					return writeJSONSuccess(cmd, impactToJSONMap(impact))
				}
				printImpactSection(cmd, impact)
				return nil
			})
		},
	}

	cmd.Flags().BoolVar(&asJSON, "json", false, "Output in JSON format")
	return cmd
}

func newCRValidateCmd() *cobra.Command {
	var asJSON bool
	var record bool

	cmd := &cobra.Command{
		Use:   "validate [id]",
		Short: "Validate CR contract completeness, scope drift, and risk signals",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withOptionalCRIDAndService(cmd, asJSON, args, "id", func(id int, svc *service.Service) error {
				report, err := svc.ValidateCR(id)
				if err != nil {
					return commandError(cmd, asJSON, err)
				}
				review, err := svc.ReviewCR(id)
				if err != nil {
					return commandError(cmd, asJSON, err)
				}
				if record {
					if err := svc.RecordCRValidation(id, report); err != nil {
						return commandError(cmd, asJSON, err)
					}
				}
				trustWarnings := []string{}
				trustRequirementsUnsatisfied := []string{}
				if review.Trust != nil {
					for _, requirement := range review.Trust.Requirements {
						if requirement.Satisfied {
							continue
						}
						trustRequirementsUnsatisfied = append(trustRequirementsUnsatisfied, requirement.Key)
						if strings.TrimSpace(requirement.Reason) != "" {
							trustWarnings = append(trustWarnings, requirement.Reason)
						}
					}
				}
				trustWarnings = dedupeStringValues(trustWarnings)
				trustRequirementsUnsatisfied = dedupeStringValues(trustRequirementsUnsatisfied)
				if asJSON {
					if !report.Valid {
						return writeJSONError(cmd, fmt.Errorf("validation failed with %d error(s): %s", len(report.Errors), strings.Join(report.Errors, "; ")))
					}
					payload := validationToJSONMap(report)
					payload["recorded"] = record
					payload["trust"] = trustToJSONMap(review.Trust)
					payload["trust_warnings"] = trustWarnings
					payload["trust_requirements_unsatisfied"] = trustRequirementsUnsatisfied
					return writeJSONSuccess(cmd, payload)
				}

				fmt.Fprintln(cmd.OutOrStdout(), "Contract:")
				if report.Valid {
					fmt.Fprintln(cmd.OutOrStdout(), "- status: complete")
				} else {
					fmt.Fprintln(cmd.OutOrStdout(), "- status: incomplete")
				}
				printImpactSection(cmd, report.Impact)
				printStringSection(cmd, "Errors", report.Errors)
				printStringSection(cmd, "Warnings", report.Warnings)
				printTrustSection(cmd, review.Trust)
				fmt.Fprintf(cmd.OutOrStdout(), "\nRecorded: %t\n", record)
				if !report.Valid {
					return fmt.Errorf("validation failed with %d error(s)", len(report.Errors))
				}
				fmt.Fprintln(cmd.OutOrStdout(), "Validation status: OK")
				return nil
			})
		},
	}

	cmd.Flags().BoolVar(&record, "record", false, "Record validation event in CR history")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Output in JSON format")
	return cmd
}
