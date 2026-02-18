package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"sophia/internal/service"
)

func newCRTaskCmd() *cobra.Command {
	taskCmd := &cobra.Command{
		Use:   "task",
		Short: "Manage CR subtasks",
	}
	taskCmd.AddCommand(newCRTaskAddCmd())
	taskCmd.AddCommand(newCRTaskListCmd())
	taskCmd.AddCommand(newCRTaskDoneCmd())
	taskCmd.AddCommand(newCRTaskReopenCmd())
	taskCmd.AddCommand(newCRTaskDiffCmd())
	taskCmd.AddCommand(newCRTaskRangeDiffCmd())
	taskCmd.AddCommand(newCRTaskDelegateCmd())
	taskCmd.AddCommand(newCRTaskUndelegateCmd())
	taskCmd.AddCommand(newCRTaskChunkCmd())
	taskCmd.AddCommand(newCRTaskContractCmd())
	return taskCmd
}

func newCRTaskChunkCmd() *cobra.Command {
	chunkCmd := &cobra.Command{
		Use:   "chunk",
		Short: "Inspect task checkpoint chunks",
	}
	chunkCmd.AddCommand(newCRTaskChunkListCmd())
	chunkCmd.AddCommand(newCRTaskChunkDiffCmd())
	return chunkCmd
}

func newCRTaskChunkListCmd() *cobra.Command {
	var asJSON bool
	var scopePaths []string

	cmd := &cobra.Command{
		Use:   "list <cr-id> <task-id>",
		Short: "List chunk candidates for task checkpointing",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			crID, err := parsePositiveIntArg(args[0], "cr-id")
			if err != nil {
				if asJSON {
					return writeJSONError(cmd, err)
				}
				return err
			}
			taskID, err := parsePositiveIntArg(args[1], "task-id")
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
			chunks, err := svc.ListTaskChunks(crID, taskID, append([]string(nil), scopePaths...))
			if err != nil {
				if asJSON {
					return writeJSONError(cmd, err)
				}
				return err
			}
			if asJSON {
				chunkMaps := make([]map[string]any, 0, len(chunks))
				for _, chunk := range chunks {
					chunkMaps = append(chunkMaps, chunkToJSONMap(chunk))
				}
				return writeJSONSuccess(cmd, map[string]any{
					"cr_id":   crID,
					"task_id": taskID,
					"chunks":  chunkMaps,
				})
			}
			if len(chunks) == 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "No chunks found for task %d in CR %d.\n", taskID, crID)
				return nil
			}
			fmt.Fprintln(cmd.OutOrStdout(), "CHUNK_ID\tPATH\tOLD\tNEW\tPREVIEW")
			for _, chunk := range chunks {
				oldRange := fmt.Sprintf("%d,%d", chunk.OldStart, chunk.OldLines)
				newRange := fmt.Sprintf("%d,%d", chunk.NewStart, chunk.NewLines)
				fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\t%s\t%s\t%s\n", chunk.ID, chunk.Path, oldRange, newRange, strings.TrimSpace(chunk.Preview))
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&asJSON, "json", false, "Output in JSON format")
	cmd.Flags().StringArrayVar(&scopePaths, "path", nil, "Filter chunk listing to repo-relative file path(s)")
	return cmd
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
	var asJSON bool

	cmd := &cobra.Command{
		Use:   "show <cr-id> <task-id>",
		Short: "Show task contract fields",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			crID, err := parsePositiveIntArg(args[0], "cr-id")
			if err != nil {
				if asJSON {
					return writeJSONError(cmd, err)
				}
				return err
			}
			taskID, err := parsePositiveIntArg(args[1], "task-id")
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
			contract, err := svc.GetTaskContract(crID, taskID)
			if err != nil {
				if asJSON {
					return writeJSONError(cmd, err)
				}
				return err
			}
			if asJSON {
				return writeJSONSuccess(cmd, map[string]any{
					"cr_id":   crID,
					"task_id": taskID,
					"task_contract": map[string]any{
						"intent":              contract.Intent,
						"acceptance_criteria": contract.AcceptanceCriteria,
						"scope":               contract.Scope,
						"updated_at":          contract.UpdatedAt,
						"updated_by":          contract.UpdatedBy,
					},
				})
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

	cmd.Flags().BoolVar(&asJSON, "json", false, "Output in JSON format")
	return cmd
}

func newCRTaskDelegateCmd() *cobra.Command {
	var childID int

	cmd := &cobra.Command{
		Use:   "delegate <cr-id> <task-id> --child <child-cr-id>",
		Short: "Delegate a parent task to a child CR",
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
			if childID <= 0 {
				return fmt.Errorf("--child must be >= 1")
			}
			svc, err := newService()
			if err != nil {
				return err
			}
			result, err := svc.DelegateTaskToChild(crID, taskID, childID)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Delegated CR %d task %d to child CR %d task %d (parent status: %s)\n", crID, result.ParentTaskID, result.ChildCRID, result.ChildTaskID, result.ParentTaskStatus)
			return nil
		},
	}

	cmd.Flags().IntVar(&childID, "child", 0, "Child CR id")
	return cmd
}

func newCRTaskUndelegateCmd() *cobra.Command {
	var childID int

	cmd := &cobra.Command{
		Use:   "undelegate <cr-id> <task-id> --child <child-cr-id>",
		Short: "Remove one delegation link from a parent task",
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
			if childID <= 0 {
				return fmt.Errorf("--child must be >= 1")
			}
			svc, err := newService()
			if err != nil {
				return err
			}
			result, err := svc.UndelegateTaskFromChild(crID, taskID, childID)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Removed %d delegation(s) from CR %d task %d to child CR %d (parent status: %s)\n", result.RemovedDelegation, crID, result.ParentTaskID, childID, result.ParentTaskStatus)
			return nil
		},
	}

	cmd.Flags().IntVar(&childID, "child", 0, "Child CR id")
	return cmd
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
	var asJSON bool

	cmd := &cobra.Command{
		Use:   "list <cr-id>",
		Short: "List subtasks for a CR",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			crID, err := parsePositiveIntArg(args[0], "cr-id")
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
			tasks, err := svc.ListTasks(crID)
			if err != nil {
				if asJSON {
					return writeJSONError(cmd, err)
				}
				return err
			}
			if asJSON {
				return writeJSONSuccess(cmd, map[string]any{
					"cr_id": crID,
					"tasks": tasks,
				})
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

	cmd.Flags().BoolVar(&asJSON, "json", false, "Output in JSON format")
	return cmd
}

func newCRTaskDoneCmd() *cobra.Command {
	var noCheckpoint bool
	var stageAll bool
	var fromContract bool
	var scopePaths []string
	var patchFile string

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
			if noCheckpoint && (stageAll || fromContract || len(scopePaths) > 0 || strings.TrimSpace(patchFile) != "") {
				return fmt.Errorf("--no-checkpoint cannot be combined with --from-contract, --path, --patch-file, or --all")
			}
			if !noCheckpoint {
				modeCount := 0
				if stageAll {
					modeCount++
				}
				if fromContract {
					modeCount++
				}
				if len(scopePaths) > 0 {
					modeCount++
				}
				if strings.TrimSpace(patchFile) != "" {
					modeCount++
				}
				if modeCount > 1 {
					return fmt.Errorf("exactly one checkpoint scope mode is required: --from-contract, --path <file> (repeatable), --patch-file <file>, or --all")
				}
				if modeCount == 0 {
					return fmt.Errorf("checkpoint scope required: use --from-contract, --path <file> (repeatable), --patch-file <file>, or --all")
				}
			}
			opts := service.DoneTaskOptions{
				Checkpoint:   !noCheckpoint,
				StageAll:     stageAll,
				FromContract: fromContract,
				Paths:        append([]string(nil), scopePaths...),
				PatchFile:    strings.TrimSpace(patchFile),
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
	cmd.Flags().BoolVar(&fromContract, "from-contract", false, "Checkpoint by staging changed files that match task contract scope")
	cmd.Flags().StringArrayVar(&scopePaths, "path", nil, "Checkpoint scope path (repo-relative file, repeatable)")
	cmd.Flags().StringVar(&patchFile, "patch-file", "", "Checkpoint scope patch manifest file")
	return cmd
}

func newCRTaskReopenCmd() *cobra.Command {
	var clearCheckpoint bool
	var asJSON bool

	cmd := &cobra.Command{
		Use:   "reopen <cr-id> <task-id>",
		Short: "Reopen a completed subtask",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			crID, err := parsePositiveIntArg(args[0], "cr-id")
			if err != nil {
				if asJSON {
					return writeJSONError(cmd, err)
				}
				return err
			}
			taskID, err := parsePositiveIntArg(args[1], "task-id")
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
			task, err := svc.ReopenTask(crID, taskID, service.ReopenTaskOptions{
				ClearCheckpoint: clearCheckpoint,
			})
			if err != nil {
				if asJSON {
					return writeJSONError(cmd, err)
				}
				return err
			}
			if asJSON {
				return writeJSONSuccess(cmd, map[string]any{
					"cr_id":              crID,
					"task_id":            taskID,
					"status":             task.Status,
					"checkpoint_commit":  strings.TrimSpace(task.CheckpointCommit),
					"checkpoint_cleared": clearCheckpoint,
				})
			}
			if clearCheckpoint {
				fmt.Fprintf(cmd.OutOrStdout(), "Reopened task %d in CR %d and cleared checkpoint metadata\n", taskID, crID)
				return nil
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Reopened task %d in CR %d\n", taskID, crID)
			return nil
		},
	}

	cmd.Flags().BoolVar(&clearCheckpoint, "clear-checkpoint", false, "Clear checkpoint metadata while reopening task")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Output in JSON format")
	return cmd
}
