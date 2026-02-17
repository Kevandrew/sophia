package cli

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"sophia/internal/model"
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
			cr, err := svc.AddCR(args[0], description)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Created CR %d on branch %s\n", cr.ID, cr.Branch)
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

			fmt.Fprintf(cmd.OutOrStdout(), "\nFiles Changed:\n")
			if len(review.Files) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "- (none)")
			} else {
				for _, file := range review.Files {
					fmt.Fprintf(cmd.OutOrStdout(), "- %s\n", file)
				}
			}
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
