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
	return taskCmd
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
			fmt.Fprintln(cmd.OutOrStdout(), "TASK_ID\tSTATUS\tTITLE")
			for _, task := range tasks {
				fmt.Fprintf(cmd.OutOrStdout(), "%d\t%s\t%s\n", task.ID, task.Status, task.Title)
			}
			return nil
		},
	}
}

func newCRTaskDoneCmd() *cobra.Command {
	return &cobra.Command{
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
			if err := svc.DoneTask(crID, taskID); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Marked task %d done in CR %d\n", taskID, crID)
			return nil
		},
	}
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
			return nil
		},
	}
}

func newCRMergeCmd() *cobra.Command {
	var deleteBranch bool

	cmd := &cobra.Command{
		Use:   "merge <id>",
		Short: "Squash merge a CR into its base branch",
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
			sha, err := svc.MergeCR(id, deleteBranch)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Merged CR %d as commit %s\n", id, sha)
			return nil
		},
	}

	cmd.Flags().BoolVar(&deleteBranch, "delete-branch", false, "Delete CR branch after merge")
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
