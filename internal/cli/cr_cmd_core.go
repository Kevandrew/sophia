package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"sophia/internal/service"
)

func newCRAddCmd() *cobra.Command {
	var description string
	var baseRef string
	var parentID int

	cmd := &cobra.Command{
		Use:   "add <title>",
		Short: "Create a new change request",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, err := newService()
			if err != nil {
				return err
			}
			if parentID < 0 {
				return fmt.Errorf("--parent must be >= 1")
			}
			opts := service.AddCROptions{
				BaseRef:    strings.TrimSpace(baseRef),
				ParentCRID: parentID,
			}
			cr, warnings, err := svc.AddCRWithOptionsWithWarnings(args[0], description, opts)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Created CR %d on branch %s\n", cr.ID, cr.Branch)
			if len(warnings) > 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "Overlap warnings:")
				for _, warning := range warnings {
					fmt.Fprintf(cmd.OutOrStdout(), "- %s\n", warning)
				}
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&description, "description", "", "Description/rationale for the CR")
	cmd.Flags().StringVar(&baseRef, "base", "", "Base Git ref for this CR")
	cmd.Flags().IntVar(&parentID, "parent", 0, "Parent CR id for stacked workflow")
	return cmd
}

func newCRApplyCmd() *cobra.Command {
	var filePath string
	var dryRun bool
	var keepFile bool
	var asJSON bool

	cmd := &cobra.Command{
		Use:   "apply",
		Short: "Apply a strict YAML CR plan",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(filePath) == "" {
				err := fmt.Errorf("--file is required")
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
			result, err := svc.ApplyCRPlan(service.ApplyCRPlanOptions{
				FilePath: filePath,
				DryRun:   dryRun,
				KeepFile: keepFile,
			})
			if err != nil {
				if asJSON {
					return writeJSONError(cmd, err)
				}
				return err
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

	cmd := &cobra.Command{
		Use:   "add <title>",
		Short: "Create a child CR from the current CR",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, err := newService()
			if err != nil {
				return err
			}
			cr, warnings, err := svc.AddChildCRFromCurrent(args[0], description)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Created child CR %d on branch %s\n", cr.ID, cr.Branch)
			if len(warnings) > 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "Overlap warnings:")
				for _, warning := range warnings {
					fmt.Fprintf(cmd.OutOrStdout(), "- %s\n", warning)
				}
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&description, "description", "", "Description/rationale for the child CR")
	return cmd
}

func newCRListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all change requests",
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, err := newService()
			if err != nil {
				return err
			}
			crs, err := svc.ListCRs()
			if err != nil {
				return err
			}
			if len(crs) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No CRs found.")
				return nil
			}
			fmt.Fprintln(cmd.OutOrStdout(), "ID\tSTATUS\tBRANCH\tTITLE")
			for _, cr := range crs {
				fmt.Fprintf(cmd.OutOrStdout(), "%d\t%s\t%s\t%s\n", cr.ID, cr.Status, cr.Branch, cr.Title)
			}
			return nil
		},
	}
}

func newCRStackCmd() *cobra.Command {
	var asJSON bool

	cmd := &cobra.Command{
		Use:   "stack [id]",
		Short: "Show stack topology and merge blockers for related CRs",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, err := newService()
			if err != nil {
				if asJSON {
					return writeJSONError(cmd, err)
				}
				return err
			}

			var stack *service.StackView
			if len(args) == 0 {
				stack, err = svc.StackCurrentCR()
			} else {
				id, parseErr := parsePositiveIntArg(args[0], "id")
				if parseErr != nil {
					if asJSON {
						return writeJSONError(cmd, parseErr)
					}
					return parseErr
				}
				stack, err = svc.StackCR(id)
			}
			if err != nil {
				if asJSON {
					return writeJSONError(cmd, err)
				}
				return err
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
		Use:   "why <id>",
		Short: "Show the rationale for why a CR exists",
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
			why, err := svc.WhyCR(id)
			if err != nil {
				if asJSON {
					return writeJSONError(cmd, err)
				}
				return err
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
		},
	}

	cmd.Flags().BoolVar(&asJSON, "json", false, "Output in JSON format")
	return cmd
}

func newCRStatusCmd() *cobra.Command {
	var asJSON bool

	cmd := &cobra.Command{
		Use:   "status <id>",
		Short: "Show CR merge-readiness and workspace status",
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
			status, err := svc.StatusCR(id)
			if err != nil {
				if asJSON {
					return writeJSONError(cmd, err)
				}
				return err
			}
			if asJSON {
				return writeJSONSuccess(cmd, map[string]any{
					"id":            status.ID,
					"uid":           status.UID,
					"title":         status.Title,
					"status":        status.Status,
					"base":          status.BaseBranch,
					"base_ref":      status.BaseRef,
					"base_commit":   status.BaseCommit,
					"parent_cr_id":  status.ParentCRID,
					"parent_status": status.ParentStatus,
					"branch":        status.Branch,
					"branch_context": map[string]any{
						"current_branch": status.CurrentBranch,
						"branch_match":   status.BranchMatch,
					},
					"working_tree": map[string]any{
						"modified_staged_count": status.ModifiedStagedCount,
						"untracked_count":       status.UntrackedCount,
						"dirty":                 status.Dirty,
					},
					"tasks": map[string]any{
						"total":             status.TasksTotal,
						"open":              status.TasksOpen,
						"done":              status.TasksDone,
						"delegated":         status.TasksDelegated,
						"delegated_pending": status.TasksDelegatedPending,
					},
					"contract": map[string]any{
						"complete":       status.ContractComplete,
						"missing_fields": status.ContractMissingFields,
					},
					"validation": map[string]any{
						"valid":    status.ValidationValid,
						"errors":   status.ValidationErrors,
						"warnings": status.ValidationWarnings,
						"risk": map[string]any{
							"tier":  status.RiskTier,
							"score": status.RiskScore,
						},
					},
					"merge_blocked":  status.MergeBlocked,
					"merge_blockers": status.MergeBlockers,
				})
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
			return nil
		},
	}

	cmd.Flags().BoolVar(&asJSON, "json", false, "Output in JSON format")
	return cmd
}

func newCREditCmd() *cobra.Command {
	var title string
	var description string

	cmd := &cobra.Command{
		Use:   "edit <id>",
		Short: "Edit CR title/description with audit trail",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parsePositiveIntArg(args[0], "id")
			if err != nil {
				return err
			}

			titleChanged := cmd.Flags().Changed("title")
			descriptionChanged := cmd.Flags().Changed("description")
			if !titleChanged && !descriptionChanged {
				return fmt.Errorf("provide at least one of --title or --description")
			}

			var titlePtr *string
			var descriptionPtr *string
			if titleChanged {
				titlePtr = &title
			}
			if descriptionChanged {
				descriptionPtr = &description
			}

			svc, err := newService()
			if err != nil {
				return err
			}
			changedFields, err := svc.EditCR(id, titlePtr, descriptionPtr)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Updated CR %d fields: %s\n", id, strings.Join(changedFields, ", "))
			return nil
		},
	}

	cmd.Flags().StringVar(&title, "title", "", "New CR title")
	cmd.Flags().StringVar(&description, "description", "", "New CR description")
	return cmd
}

func newCRContractCmd() *cobra.Command {
	contractCmd := &cobra.Command{
		Use:   "contract",
		Short: "Manage CR intent contract fields",
	}
	contractCmd.AddCommand(newCRContractSetCmd())
	contractCmd.AddCommand(newCRContractShowCmd())
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

	cmd := &cobra.Command{
		Use:   "set <id>",
		Short: "Set/update CR intent contract fields",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parsePositiveIntArg(args[0], "id")
			if err != nil {
				return err
			}

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
			if patch.Why == nil && patch.Scope == nil && patch.NonGoals == nil && patch.Invariants == nil && patch.BlastRadius == nil && patch.RiskCriticalScopes == nil && patch.RiskTierHint == nil && patch.RiskRationale == nil && patch.TestPlan == nil && patch.RollbackPlan == nil {
				return fmt.Errorf("provide at least one contract field flag")
			}

			svc, err := newService()
			if err != nil {
				return err
			}
			changed, err := svc.SetCRContract(id, patch)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Updated CR %d contract fields: %s\n", id, strings.Join(changed, ", "))
			return nil
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
	return cmd
}

func newCRContractShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <id>",
		Short: "Show CR intent contract fields",
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
			contract, err := svc.GetCRContract(id)
			if err != nil {
				return err
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
			return nil
		},
	}
}

func newCRImpactCmd() *cobra.Command {
	var asJSON bool

	cmd := &cobra.Command{
		Use:   "impact <id>",
		Short: "Show deterministic impact and risk summary for a CR",
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
			impact, err := svc.ImpactCR(id)
			if err != nil {
				if asJSON {
					return writeJSONError(cmd, err)
				}
				return err
			}
			if asJSON {
				return writeJSONSuccess(cmd, impactToJSONMap(impact))
			}
			printImpactSection(cmd, impact)
			return nil
		},
	}

	cmd.Flags().BoolVar(&asJSON, "json", false, "Output in JSON format")
	return cmd
}

func newCRValidateCmd() *cobra.Command {
	var asJSON bool

	cmd := &cobra.Command{
		Use:   "validate <id>",
		Short: "Validate CR contract completeness, scope drift, and risk signals",
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
			report, err := svc.ValidateCR(id)
			if err != nil {
				if asJSON {
					return writeJSONError(cmd, err)
				}
				return err
			}
			if err := svc.RecordCRValidation(id, report); err != nil {
				if asJSON {
					return writeJSONError(cmd, err)
				}
				return err
			}
			if asJSON {
				if !report.Valid {
					return writeJSONError(cmd, fmt.Errorf("validation failed with %d error(s): %s", len(report.Errors), strings.Join(report.Errors, "; ")))
				}
				return writeJSONSuccess(cmd, map[string]any{
					"valid":    report.Valid,
					"errors":   report.Errors,
					"warnings": report.Warnings,
					"impact":   impactToJSONMap(report.Impact),
				})
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
			if !report.Valid {
				return fmt.Errorf("validation failed with %d error(s)", len(report.Errors))
			}
			fmt.Fprintln(cmd.OutOrStdout(), "Validation status: OK")
			return nil
		},
	}

	cmd.Flags().BoolVar(&asJSON, "json", false, "Output in JSON format")
	return cmd
}

func newCRRedactCmd() *cobra.Command {
	var noteIndex int
	var eventIndex int
	var reason string

	cmd := &cobra.Command{
		Use:   "redact <id>",
		Short: "Redact CR note/event payload with audit event",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parsePositiveIntArg(args[0], "id")
			if err != nil {
				return err
			}
			reason = strings.TrimSpace(reason)
			if reason == "" {
				return fmt.Errorf("--reason is required")
			}

			noteChanged := cmd.Flags().Changed("note-index")
			eventChanged := cmd.Flags().Changed("event-index")
			if noteChanged == eventChanged {
				return fmt.Errorf("provide exactly one of --note-index or --event-index")
			}

			svc, err := newService()
			if err != nil {
				return err
			}
			if noteChanged {
				if err := svc.RedactCRNote(id, noteIndex, reason); err != nil {
					return err
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Redacted note #%d in CR %d\n", noteIndex, id)
				return nil
			}
			if err := svc.RedactCREvent(id, eventIndex, reason); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Redacted event #%d in CR %d\n", eventIndex, id)
			return nil
		},
	}

	cmd.Flags().IntVar(&noteIndex, "note-index", 0, "1-based note index to redact")
	cmd.Flags().IntVar(&eventIndex, "event-index", 0, "1-based event index to redact")
	cmd.Flags().StringVar(&reason, "reason", "", "Redaction reason (required)")
	return cmd
}

func newCRHistoryCmd() *cobra.Command {
	var showRedacted bool

	cmd := &cobra.Command{
		Use:   "history <id>",
		Short: "Show CR notes/events timeline with indices",
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
			history, err := svc.HistoryCR(id, showRedacted)
			if err != nil {
				return err
			}

			fmt.Fprintf(cmd.OutOrStdout(), "CR %d: %s\n", history.CRID, history.Title)
			fmt.Fprintf(cmd.OutOrStdout(), "Status: %s\n", history.Status)
			fmt.Fprintf(cmd.OutOrStdout(), "Intent: %s\n", nonEmpty(history.Description, "(none)"))

			fmt.Fprintln(cmd.OutOrStdout(), "\nNotes:")
			if len(history.Notes) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "- (none)")
			} else {
				for _, note := range history.Notes {
					suffix := ""
					if note.Redacted {
						suffix = " [redacted]"
					}
					fmt.Fprintf(cmd.OutOrStdout(), "- #%d %s%s\n", note.Index, note.Text, suffix)
				}
			}

			fmt.Fprintln(cmd.OutOrStdout(), "\nEvents:")
			if len(history.Events) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "- (none)")
			} else {
				for _, event := range history.Events {
					suffix := ""
					if event.Redacted {
						suffix = " [redacted]"
					}
					fmt.Fprintf(cmd.OutOrStdout(), "- #%d %s %s %s: %s%s\n", event.Index, event.TS, event.Type, event.Actor, event.Summary, suffix)
					if strings.TrimSpace(event.Ref) != "" {
						fmt.Fprintf(cmd.OutOrStdout(), "  ref: %s\n", event.Ref)
					}
					if showRedacted && strings.TrimSpace(event.RedactionReason) != "" {
						fmt.Fprintf(cmd.OutOrStdout(), "  redaction_reason: %s\n", event.RedactionReason)
					}
				}
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&showRedacted, "show-redacted", false, "Show redaction metadata (payload remains redacted)")
	return cmd
}

func newCRCurrentCmd() *cobra.Command {
	var asJSON bool

	cmd := &cobra.Command{
		Use:   "current",
		Short: "Show the active CR context for the current branch",
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, err := newService()
			if err != nil {
				if asJSON {
					return writeJSONError(cmd, err)
				}
				return err
			}
			ctx, err := svc.CurrentCR()
			if err != nil {
				if asJSON {
					return writeJSONError(cmd, err)
				}
				if errorsIs(err, service.ErrNoActiveCRContext) {
					fmt.Fprintln(cmd.OutOrStdout(), "No active CR context on current branch.")
					return err
				}
				return err
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
	return &cobra.Command{
		Use:   "switch <id>",
		Short: "Switch to the branch for a CR",
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
			cr, err := svc.SwitchCR(id)
			if err != nil {
				if errorsIs(err, service.ErrWorkingTreeDirty) {
					fmt.Fprintln(cmd.OutOrStdout(), "Working tree is dirty. Commit changes or run `git stash`, then retry.")
				} else if errorsIs(err, service.ErrBranchInOtherWorktree) {
					fmt.Fprintln(cmd.OutOrStdout(), "Target branch is already checked out in another worktree. Run this command from that worktree path.")
				}
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Switched to CR %d branch %s\n", cr.ID, cr.Branch)
			return nil
		},
	}
}

func newCRReopenCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "reopen <id>",
		Short: "Reopen a merged CR and switch to its branch",
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
			cr, err := svc.ReopenCR(id)
			if err != nil {
				if errorsIs(err, service.ErrWorkingTreeDirty) {
					fmt.Fprintln(cmd.OutOrStdout(), "Working tree is dirty. Commit changes or run `git stash`, then retry.")
				} else if errorsIs(err, service.ErrBranchInOtherWorktree) {
					fmt.Fprintln(cmd.OutOrStdout(), "Target branch is already checked out in another worktree. Reopen from that worktree path.")
				}
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Reopened CR %d on branch %s\n", cr.ID, cr.Branch)
			return nil
		},
	}
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

	cmd := &cobra.Command{
		Use:   "set <id>",
		Short: "Set a CR base ref with optional immediate rebase",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parsePositiveIntArg(args[0], "id")
			if err != nil {
				return err
			}
			ref = strings.TrimSpace(ref)
			if ref == "" {
				return fmt.Errorf("--ref is required")
			}

			svc, err := newService()
			if err != nil {
				return err
			}
			cr, err := svc.SetCRBase(id, ref, rebase)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Updated CR %d base to %s (%s)\n", cr.ID, cr.BaseRef, nonEmpty(cr.BaseCommit, "-"))
			return nil
		},
	}

	cmd.Flags().StringVar(&ref, "ref", "", "Git ref to use as CR base")
	cmd.Flags().BoolVar(&rebase, "rebase", false, "Rebase CR branch onto the new base ref")
	return cmd
}

func newCRRestackCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "restack <id>",
		Short: "Restack a child CR onto its parent effective head",
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
			cr, err := svc.RestackCR(id)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Restacked CR %d onto base %s (%s)\n", cr.ID, nonEmpty(cr.BaseRef, "-"), nonEmpty(cr.BaseCommit, "-"))
			return nil
		},
	}
}

func newCRNoteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "note <id> <note>",
		Short: "Append a note to a change request",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parsePositiveIntArg(args[0], "id")
			if err != nil {
				return err
			}
			svc, err := newService()
			if err != nil {
				return err
			}
			if err := svc.AddNote(id, args[1]); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Added note to CR %d\n", id)
			return nil
		},
	}
}
