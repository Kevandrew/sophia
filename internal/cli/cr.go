package cli

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"sophia/internal/model"
	"sophia/internal/service"
)

func newCRCmd() *cobra.Command {
	crCmd := &cobra.Command{
		Use:   "cr",
		Short: "Manage change requests",
	}

	crCmd.AddCommand(newCRAddCmd())
	crCmd.AddCommand(newCRListCmd())
	crCmd.AddCommand(newCRNoteCmd())
	crCmd.AddCommand(newCRReviewCmd())
	crCmd.AddCommand(newCRMergeCmd())
	crCmd.AddCommand(newCRTaskCmd())
	crCmd.AddCommand(newCRCurrentCmd())
	crCmd.AddCommand(newCRSwitchCmd())
	crCmd.AddCommand(newCRReopenCmd())
	crCmd.AddCommand(newCREditCmd())
	crCmd.AddCommand(newCRContractCmd())
	crCmd.AddCommand(newCRImpactCmd())
	crCmd.AddCommand(newCRValidateCmd())
	crCmd.AddCommand(newCRRedactCmd())
	crCmd.AddCommand(newCRHistoryCmd())

	return crCmd
}

func newCRAddCmd() *cobra.Command {
	var description string

	cmd := &cobra.Command{
		Use:   "add <title>",
		Short: "Create a new change request",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, err := newService()
			if err != nil {
				return err
			}
			cr, warnings, err := svc.AddCRWithWarnings(args[0], description)
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
			if cmd.Flags().Changed("test-plan") {
				v := testPlan
				patch.TestPlan = &v
			}
			if cmd.Flags().Changed("rollback-plan") {
				v := rollbackPlan
				patch.RollbackPlan = &v
			}
			if patch.Why == nil && patch.Scope == nil && patch.NonGoals == nil && patch.Invariants == nil && patch.BlastRadius == nil && patch.TestPlan == nil && patch.RollbackPlan == nil {
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
			fmt.Fprintf(cmd.OutOrStdout(), "- test_plan: %s\n", nonEmpty(strings.TrimSpace(contract.TestPlan), "(missing)"))
			fmt.Fprintf(cmd.OutOrStdout(), "- rollback_plan: %s\n", nonEmpty(strings.TrimSpace(contract.RollbackPlan), "(missing)"))
			fmt.Fprintf(cmd.OutOrStdout(), "- updated_at: %s\n", nonEmpty(strings.TrimSpace(contract.UpdatedAt), "(never)"))
			fmt.Fprintf(cmd.OutOrStdout(), "- updated_by: %s\n", nonEmpty(strings.TrimSpace(contract.UpdatedBy), "(never)"))
			return nil
		},
	}
}

func newCRImpactCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "impact <id>",
		Short: "Show deterministic impact and risk summary for a CR",
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
			impact, err := svc.ImpactCR(id)
			if err != nil {
				return err
			}
			printImpactSection(cmd, impact)
			return nil
		},
	}
}

func newCRValidateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "validate <id>",
		Short: "Validate CR contract completeness, scope drift, and risk signals",
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
			report, err := svc.ValidateCR(id)
			if err != nil {
				return err
			}
			if err := svc.RecordCRValidation(id, report); err != nil {
				return err
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
	return &cobra.Command{
		Use:   "current",
		Short: "Show the active CR context for the current branch",
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, err := newService()
			if err != nil {
				return err
			}
			ctx, err := svc.CurrentCR()
			if err != nil {
				if errorsIs(err, service.ErrNoActiveCRContext) {
					fmt.Fprintln(cmd.OutOrStdout(), "No active CR context on current branch.")
					return err
				}
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Branch: %s\n", ctx.Branch)
			fmt.Fprintf(cmd.OutOrStdout(), "CR: %d\n", ctx.CR.ID)
			fmt.Fprintf(cmd.OutOrStdout(), "Title: %s\n", ctx.CR.Title)
			fmt.Fprintf(cmd.OutOrStdout(), "Status: %s\n", ctx.CR.Status)
			fmt.Fprintf(cmd.OutOrStdout(), "Base: %s\n", ctx.CR.BaseBranch)
			return nil
		},
	}
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
				}
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Reopened CR %d on branch %s\n", cr.ID, cr.Branch)
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

func newCRTaskCmd() *cobra.Command {
	taskCmd := &cobra.Command{
		Use:   "task",
		Short: "Manage CR subtasks",
	}
	taskCmd.AddCommand(newCRTaskAddCmd())
	taskCmd.AddCommand(newCRTaskListCmd())
	taskCmd.AddCommand(newCRTaskDoneCmd())
	taskCmd.AddCommand(newCRTaskContractCmd())
	return taskCmd
}

func newCRTaskContractCmd() *cobra.Command {
	contractCmd := &cobra.Command{
		Use:   "contract",
		Short: "Manage task-level contract fields",
	}
	contractCmd.AddCommand(newCRTaskContractSetCmd())
	contractCmd.AddCommand(newCRTaskContractShowCmd())
	return contractCmd
}

func newCRTaskContractSetCmd() *cobra.Command {
	var intent string
	var acceptance []string
	var scope []string

	cmd := &cobra.Command{
		Use:   "set <cr-id> <task-id>",
		Short: "Set/update task contract fields",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			crID, err := parsePositiveIntArg(args[0], "cr-id")
			if err != nil {
				return err
			}
			taskID, err := parsePositiveIntArg(args[1], "task-id")
			if err != nil {
				return err
			}

			patch := service.TaskContractPatch{}
			if cmd.Flags().Changed("intent") {
				v := intent
				patch.Intent = &v
			}
			if cmd.Flags().Changed("acceptance") {
				v := append([]string(nil), acceptance...)
				patch.AcceptanceCriteria = &v
			}
			if cmd.Flags().Changed("scope") {
				v := append([]string(nil), scope...)
				patch.Scope = &v
			}
			if patch.Intent == nil && patch.AcceptanceCriteria == nil && patch.Scope == nil {
				return fmt.Errorf("provide at least one of --intent, --acceptance, or --scope")
			}

			svc, err := newService()
			if err != nil {
				return err
			}
			changed, err := svc.SetTaskContract(crID, taskID, patch)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Updated CR %d task %d contract fields: %s\n", crID, taskID, strings.Join(changed, ", "))
			return nil
		},
	}

	cmd.Flags().StringVar(&intent, "intent", "", "Task intent statement")
	cmd.Flags().StringArrayVar(&acceptance, "acceptance", nil, "Task acceptance criterion (repeatable)")
	cmd.Flags().StringArrayVar(&scope, "scope", nil, "Task scope prefix (repeatable)")
	return cmd
}

func newCRTaskContractShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <cr-id> <task-id>",
		Short: "Show task contract fields",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			crID, err := parsePositiveIntArg(args[0], "cr-id")
			if err != nil {
				return err
			}
			taskID, err := parsePositiveIntArg(args[1], "task-id")
			if err != nil {
				return err
			}
			svc, err := newService()
			if err != nil {
				return err
			}
			contract, err := svc.GetTaskContract(crID, taskID)
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "Task Contract:")
			fmt.Fprintf(cmd.OutOrStdout(), "- intent: %s\n", nonEmpty(strings.TrimSpace(contract.Intent), "(missing)"))
			printValueList(cmd, "acceptance_criteria", contract.AcceptanceCriteria)
			printValueList(cmd, "scope", contract.Scope)
			fmt.Fprintf(cmd.OutOrStdout(), "- updated_at: %s\n", nonEmpty(strings.TrimSpace(contract.UpdatedAt), "(never)"))
			fmt.Fprintf(cmd.OutOrStdout(), "- updated_by: %s\n", nonEmpty(strings.TrimSpace(contract.UpdatedBy), "(never)"))
			return nil
		},
	}
}

func newCRTaskAddCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "add <cr-id> <title>",
		Short: "Add a subtask to a CR",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			crID, err := parsePositiveIntArg(args[0], "cr-id")
			if err != nil {
				return err
			}
			svc, err := newService()
			if err != nil {
				return err
			}
			task, err := svc.AddTask(crID, args[1])
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Added task %d to CR %d\n", task.ID, crID)
			return nil
		},
	}
}

func newCRTaskListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list <cr-id>",
		Short: "List subtasks for a CR",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			crID, err := parsePositiveIntArg(args[0], "cr-id")
			if err != nil {
				return err
			}
			svc, err := newService()
			if err != nil {
				return err
			}
			tasks, err := svc.ListTasks(crID)
			if err != nil {
				return err
			}
			if len(tasks) == 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "No tasks found for CR %d.\n", crID)
				return nil
			}
			fmt.Fprintln(cmd.OutOrStdout(), "TASK_ID\tSTATUS\tCHECKPOINT\tTITLE")
			for _, task := range tasks {
				checkpoint := "-"
				if strings.TrimSpace(task.CheckpointCommit) != "" {
					checkpoint = task.CheckpointCommit
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%d\t%s\t%s\t%s\n", task.ID, task.Status, checkpoint, task.Title)
			}
			return nil
		},
	}
}

func newCRTaskDoneCmd() *cobra.Command {
	var noCheckpoint bool
	var stageAll bool
	var scopePaths []string

	cmd := &cobra.Command{
		Use:   "done <cr-id> <task-id>",
		Short: "Mark a subtask as done",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			crID, err := parsePositiveIntArg(args[0], "cr-id")
			if err != nil {
				return err
			}
			taskID, err := parsePositiveIntArg(args[1], "task-id")
			if err != nil {
				return err
			}
			svc, err := newService()
			if err != nil {
				return err
			}
			if noCheckpoint && (stageAll || len(scopePaths) > 0) {
				return fmt.Errorf("--no-checkpoint cannot be combined with --path or --all")
			}
			if !noCheckpoint {
				if stageAll && len(scopePaths) > 0 {
					return fmt.Errorf("--all cannot be combined with --path")
				}
				if !stageAll && len(scopePaths) == 0 {
					return fmt.Errorf("checkpoint scope required: use --path <file> (repeatable) or --all")
				}
			}
			opts := service.DoneTaskOptions{
				Checkpoint: !noCheckpoint,
				StageAll:   stageAll,
				Paths:      append([]string(nil), scopePaths...),
			}
			sha, err := svc.DoneTaskWithCheckpoint(crID, taskID, opts)
			if err != nil {
				return err
			}
			if noCheckpoint {
				fmt.Fprintf(cmd.OutOrStdout(), "Marked task %d done in CR %d (no checkpoint)\n", taskID, crID)
				return nil
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Marked task %d done in CR %d with checkpoint %s\n", taskID, crID, nonEmpty(sha, "-"))
			return nil
		},
	}

	cmd.Flags().BoolVar(&noCheckpoint, "no-checkpoint", false, "Mark task done without creating a checkpoint commit")
	cmd.Flags().BoolVar(&stageAll, "all", false, "Checkpoint by staging all changes explicitly")
	cmd.Flags().StringArrayVar(&scopePaths, "path", nil, "Checkpoint scope path (repo-relative file, repeatable)")
	return cmd
}

func newCRReviewCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "review <id>",
		Short: "Show intent-first CR review context",
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
			review, err := svc.ReviewCR(id)
			if err != nil {
				return err
			}

			fmt.Fprintf(cmd.OutOrStdout(), "CR %d: %s\n", review.CR.ID, review.CR.Title)
			fmt.Fprintf(cmd.OutOrStdout(), "Status: %s\n", review.CR.Status)
			fmt.Fprintf(cmd.OutOrStdout(), "Base: %s\n", review.CR.BaseBranch)
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
					if task.Status == model.TaskStatusDone {
						marker = "[x]"
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

func printListSection(cmd *cobra.Command, title string, items []string) {
	fmt.Fprintf(cmd.OutOrStdout(), "\n%s:\n", title)
	if len(items) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "- (none)")
		return
	}
	for _, item := range items {
		fmt.Fprintf(cmd.OutOrStdout(), "- %s\n", item)
	}
}

func printStringSection(cmd *cobra.Command, title string, items []string) {
	fmt.Fprintf(cmd.OutOrStdout(), "\n%s:\n", title)
	if len(items) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "- (none)")
		return
	}
	for _, item := range items {
		fmt.Fprintf(cmd.OutOrStdout(), "- %s\n", item)
	}
}

func printValueList(cmd *cobra.Command, label string, values []string) {
	if len(values) == 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "- %s: (missing)\n", label)
		return
	}
	fmt.Fprintf(cmd.OutOrStdout(), "- %s:\n", label)
	for _, value := range values {
		fmt.Fprintf(cmd.OutOrStdout(), "  - %s\n", value)
	}
}

func printInlineList(cmd *cobra.Command, label string, values []string) {
	if len(values) == 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "- %s: (missing)\n", label)
		return
	}
	fmt.Fprintf(cmd.OutOrStdout(), "- %s: %s\n", label, strings.Join(values, ", "))
}

func printImpactSection(cmd *cobra.Command, impact *service.ImpactReport) {
	fmt.Fprintln(cmd.OutOrStdout(), "\nImpact:")
	if impact == nil {
		fmt.Fprintln(cmd.OutOrStdout(), "- (none)")
		return
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Risk Tier: %s\n", nonEmpty(strings.TrimSpace(impact.RiskTier), "-"))
	fmt.Fprintf(cmd.OutOrStdout(), "Risk Score: %d\n", impact.RiskScore)
	fmt.Fprintf(cmd.OutOrStdout(), "Files Changed: %d\n", impact.FilesChanged)
	printListSection(cmd, "Scope Drift", impact.ScopeDrift)
	printListSection(cmd, "Task Scope Warnings", impact.TaskScopeWarnings)
	printListSection(cmd, "Task Contract Warnings", impact.TaskContractWarnings)
	fmt.Fprintln(cmd.OutOrStdout(), "\nRisk Signals:")
	if len(impact.Signals) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "- (none)")
		return
	}
	for _, signal := range impact.Signals {
		fmt.Fprintf(cmd.OutOrStdout(), "- [%s] +%d %s\n", signal.Code, signal.Points, signal.Summary)
	}
}

func parsePositiveIntArg(raw string, name string) (int, error) {
	id, err := strconv.Atoi(raw)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("invalid %s %q", name, raw)
	}
	return id, nil
}

func nonEmpty(v, fallback string) string {
	if strings.TrimSpace(v) == "" {
		return fallback
	}
	return v
}

func errorsIs(err error, target error) bool {
	return errors.Is(err, target)
}
