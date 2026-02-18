package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"sophia/internal/model"
	"sophia/internal/service"
)

func newCRTaskCmd() *cobra.Command {
	taskCmd := &cobra.Command{
		Use:     "task",
		Short:   "Manage CR subtasks",
		Long:    "Task commands are the checkpoint layer for implementation progress. Define task contracts first, then complete tasks with explicit checkpoint scope.",
		Example: "  sophia cr task add 25 \"Implement merge status parser\"\n  sophia cr task contract set 25 1 --intent \"Parse merge state\" --acceptance \"status command reports conflict files\" --scope internal/service\n  sophia cr task done 25 1 --from-contract\n  sophia cr task reopen 25 1 --clear-checkpoint",
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
				return commandError(cmd, asJSON, err)
			}
			taskID, err := parsePositiveIntArg(args[1], "task-id")
			if err != nil {
				return commandError(cmd, asJSON, err)
			}
			svc, err := newService()
			if err != nil {
				return commandError(cmd, asJSON, err)
			}
			chunks, err := svc.ListTaskChunks(crID, taskID, append([]string(nil), scopePaths...))
			if err != nil {
				return commandError(cmd, asJSON, err)
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
	contractCmd.AddCommand(newCRTaskContractDriftCmd())
	return contractCmd
}

func newCRTaskContractSetCmd() *cobra.Command {
	var intent string
	var acceptance []string
	var scope []string
	var acceptanceChecks []string
	var changeReason string
	var asJSON bool

	cmd := &cobra.Command{
		Use:   "set <cr-id> <task-id>",
		Short: "Set/update task contract fields",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			crID, err := parsePositiveIntArg(args[0], "cr-id")
			if err != nil {
				return commandError(cmd, asJSON, err)
			}
			taskID, err := parsePositiveIntArg(args[1], "task-id")
			if err != nil {
				return commandError(cmd, asJSON, err)
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
			if cmd.Flags().Changed("acceptance-check") {
				v := append([]string(nil), acceptanceChecks...)
				patch.AcceptanceChecks = &v
			}
			if cmd.Flags().Changed("change-reason") {
				v := changeReason
				patch.ChangeReason = &v
			}
			if patch.Intent == nil && patch.AcceptanceCriteria == nil && patch.Scope == nil && patch.AcceptanceChecks == nil {
				err := fmt.Errorf("provide at least one of --intent, --acceptance, --scope, or --acceptance-check")
				return commandError(cmd, asJSON, err)
			}

			svc, err := newService()
			if err != nil {
				return commandError(cmd, asJSON, err)
			}
			changed, err := svc.SetTaskContract(crID, taskID, patch)
			if err != nil {
				return commandError(cmd, asJSON, err)
			}
			if asJSON {
				return writeJSONSuccess(cmd, map[string]any{
					"cr_id":          crID,
					"task_id":        taskID,
					"changed_fields": stringSliceOrEmpty(changed),
				})
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Updated CR %d task %d contract fields: %s\n", crID, taskID, strings.Join(changed, ", "))
			return nil
		},
	}

	cmd.Flags().StringVar(&intent, "intent", "", "Task intent statement")
	cmd.Flags().StringArrayVar(&acceptance, "acceptance", nil, "Task acceptance criterion (repeatable)")
	cmd.Flags().StringArrayVar(&scope, "scope", nil, "Task scope prefix (repeatable)")
	cmd.Flags().StringArrayVar(&acceptanceChecks, "acceptance-check", nil, "Policy trust check key required for task acceptance (repeatable)")
	cmd.Flags().StringVar(&changeReason, "change-reason", "", "Reason for post-checkpoint contract change (used to acknowledge drift)")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Output in JSON format")
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
				return commandError(cmd, asJSON, err)
			}
			taskID, err := parsePositiveIntArg(args[1], "task-id")
			if err != nil {
				return commandError(cmd, asJSON, err)
			}
			svc, err := newService()
			if err != nil {
				return commandError(cmd, asJSON, err)
			}
			contract, err := svc.GetTaskContract(crID, taskID)
			if err != nil {
				return commandError(cmd, asJSON, err)
			}
			if asJSON {
				return writeJSONSuccess(cmd, map[string]any{
					"cr_id":   crID,
					"task_id": taskID,
					"task_contract": map[string]any{
						"intent":              contract.Intent,
						"acceptance_criteria": contract.AcceptanceCriteria,
						"scope":               contract.Scope,
						"acceptance_checks":   contract.AcceptanceChecks,
						"updated_at":          contract.UpdatedAt,
						"updated_by":          contract.UpdatedBy,
					},
				})
			}
			fmt.Fprintln(cmd.OutOrStdout(), "Task Contract:")
			fmt.Fprintf(cmd.OutOrStdout(), "- intent: %s\n", nonEmpty(strings.TrimSpace(contract.Intent), "(missing)"))
			printValueList(cmd, "acceptance_criteria", contract.AcceptanceCriteria)
			printValueList(cmd, "scope", contract.Scope)
			printValueList(cmd, "acceptance_checks", contract.AcceptanceChecks)
			fmt.Fprintf(cmd.OutOrStdout(), "- updated_at: %s\n", nonEmpty(strings.TrimSpace(contract.UpdatedAt), "(never)"))
			fmt.Fprintf(cmd.OutOrStdout(), "- updated_by: %s\n", nonEmpty(strings.TrimSpace(contract.UpdatedBy), "(never)"))
			return nil
		},
	}

	cmd.Flags().BoolVar(&asJSON, "json", false, "Output in JSON format")
	return cmd
}

func newCRTaskContractDriftCmd() *cobra.Command {
	driftCmd := &cobra.Command{
		Use:   "drift",
		Short: "Inspect and acknowledge task contract drift records",
	}
	driftCmd.AddCommand(newCRTaskContractDriftListCmd())
	driftCmd.AddCommand(newCRTaskContractDriftAckCmd())
	return driftCmd
}

func newCRTaskContractDriftListCmd() *cobra.Command {
	var asJSON bool

	cmd := &cobra.Command{
		Use:   "list <cr-id> <task-id>",
		Short: "List drift records for a task contract",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			crID, err := parsePositiveIntArg(args[0], "cr-id")
			if err != nil {
				return commandError(cmd, asJSON, err)
			}
			taskID, err := parsePositiveIntArg(args[1], "task-id")
			if err != nil {
				return commandError(cmd, asJSON, err)
			}
			svc, err := newService()
			if err != nil {
				return commandError(cmd, asJSON, err)
			}
			drifts, err := svc.ListTaskContractDrifts(crID, taskID)
			if err != nil {
				return commandError(cmd, asJSON, err)
			}
			if asJSON {
				return writeJSONSuccess(cmd, map[string]any{
					"cr_id":   crID,
					"task_id": taskID,
					"drifts":  taskContractDriftsToJSON(drifts),
				})
			}
			if len(drifts) == 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "No contract drift records for CR %d task %d.\n", crID, taskID)
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

func newCRTaskContractDriftAckCmd() *cobra.Command {
	var reason string
	var asJSON bool

	cmd := &cobra.Command{
		Use:   "ack <cr-id> <task-id> <drift-id>",
		Short: "Acknowledge a task contract drift record",
		Args:  cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			crID, err := parsePositiveIntArg(args[0], "cr-id")
			if err != nil {
				return commandError(cmd, asJSON, err)
			}
			taskID, err := parsePositiveIntArg(args[1], "task-id")
			if err != nil {
				return commandError(cmd, asJSON, err)
			}
			driftID, err := parsePositiveIntArg(args[2], "drift-id")
			if err != nil {
				return commandError(cmd, asJSON, err)
			}
			if strings.TrimSpace(reason) == "" {
				err := fmt.Errorf("--reason is required")
				return commandError(cmd, asJSON, err)
			}
			svc, err := newService()
			if err != nil {
				return commandError(cmd, asJSON, err)
			}
			drift, err := svc.AckTaskContractDrift(crID, taskID, driftID, reason)
			if err != nil {
				return commandError(cmd, asJSON, err)
			}
			if asJSON {
				return writeJSONSuccess(cmd, map[string]any{
					"cr_id":   crID,
					"task_id": taskID,
					"drift":   taskContractDriftToJSON(*drift),
				})
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Acknowledged CR %d task %d drift %d.\n", crID, taskID, driftID)
			return nil
		},
	}

	cmd.Flags().StringVar(&reason, "reason", "", "Acknowledgement reason")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Output in JSON format")
	return cmd
}

func taskContractDriftsToJSON(drifts []model.TaskContractDrift) []map[string]any {
	out := make([]map[string]any, 0, len(drifts))
	for _, drift := range drifts {
		out = append(out, taskContractDriftToJSON(drift))
	}
	return out
}

func taskContractDriftToJSON(drift model.TaskContractDrift) map[string]any {
	return map[string]any{
		"id":                       drift.ID,
		"ts":                       drift.TS,
		"actor":                    drift.Actor,
		"fields":                   stringSliceOrEmpty(drift.Fields),
		"before_scope":             stringSliceOrEmpty(drift.BeforeScope),
		"after_scope":              stringSliceOrEmpty(drift.AfterScope),
		"before_acceptance_checks": stringSliceOrEmpty(drift.BeforeAcceptanceChecks),
		"after_acceptance_checks":  stringSliceOrEmpty(drift.AfterAcceptanceChecks),
		"checkpoint_commit":        drift.CheckpointCommit,
		"reason":                   drift.Reason,
		"acknowledged":             drift.Acknowledged,
		"acknowledged_at":          drift.AcknowledgedAt,
		"acknowledged_by":          drift.AcknowledgedBy,
		"ack_reason":               drift.AckReason,
	}
}

func newCRTaskDelegateCmd() *cobra.Command {
	var childID int
	var asJSON bool

	cmd := &cobra.Command{
		Use:   "delegate <cr-id> <task-id> --child <child-cr-id>",
		Short: "Delegate a parent task to a child CR",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			crID, err := parsePositiveIntArg(args[0], "cr-id")
			if err != nil {
				return commandError(cmd, asJSON, err)
			}
			taskID, err := parsePositiveIntArg(args[1], "task-id")
			if err != nil {
				return commandError(cmd, asJSON, err)
			}
			if childID <= 0 {
				err := fmt.Errorf("--child must be >= 1")
				return commandError(cmd, asJSON, err)
			}
			svc, err := newService()
			if err != nil {
				return commandError(cmd, asJSON, err)
			}
			result, err := svc.DelegateTaskToChild(crID, taskID, childID)
			if err != nil {
				return commandError(cmd, asJSON, err)
			}
			if asJSON {
				return writeJSONSuccess(cmd, map[string]any{
					"cr_id":              crID,
					"task_id":            taskID,
					"child_cr_id":        result.ChildCRID,
					"child_task_id":      result.ChildTaskID,
					"parent_task_id":     result.ParentTaskID,
					"parent_task_status": result.ParentTaskStatus,
				})
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Delegated CR %d task %d to child CR %d task %d (parent status: %s)\n", crID, result.ParentTaskID, result.ChildCRID, result.ChildTaskID, result.ParentTaskStatus)
			return nil
		},
	}

	cmd.Flags().IntVar(&childID, "child", 0, "Child CR id")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Output in JSON format")
	return cmd
}

func newCRTaskUndelegateCmd() *cobra.Command {
	var childID int
	var asJSON bool

	cmd := &cobra.Command{
		Use:   "undelegate <cr-id> <task-id> --child <child-cr-id>",
		Short: "Remove one delegation link from a parent task",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			crID, err := parsePositiveIntArg(args[0], "cr-id")
			if err != nil {
				return commandError(cmd, asJSON, err)
			}
			taskID, err := parsePositiveIntArg(args[1], "task-id")
			if err != nil {
				return commandError(cmd, asJSON, err)
			}
			if childID <= 0 {
				err := fmt.Errorf("--child must be >= 1")
				return commandError(cmd, asJSON, err)
			}
			svc, err := newService()
			if err != nil {
				return commandError(cmd, asJSON, err)
			}
			result, err := svc.UndelegateTaskFromChild(crID, taskID, childID)
			if err != nil {
				return commandError(cmd, asJSON, err)
			}
			if asJSON {
				return writeJSONSuccess(cmd, map[string]any{
					"cr_id":               crID,
					"task_id":             taskID,
					"child_cr_id":         childID,
					"parent_task_id":      result.ParentTaskID,
					"parent_task_status":  result.ParentTaskStatus,
					"removed_delegations": result.RemovedDelegation,
				})
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Removed %d delegation(s) from CR %d task %d to child CR %d (parent status: %s)\n", result.RemovedDelegation, crID, result.ParentTaskID, childID, result.ParentTaskStatus)
			return nil
		},
	}

	cmd.Flags().IntVar(&childID, "child", 0, "Child CR id")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Output in JSON format")
	return cmd
}

func newCRTaskAddCmd() *cobra.Command {
	var asJSON bool

	cmd := &cobra.Command{
		Use:   "add <cr-id> <title>",
		Short: "Add a subtask to a CR",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			crID, err := parsePositiveIntArg(args[0], "cr-id")
			if err != nil {
				return commandError(cmd, asJSON, err)
			}
			svc, err := newService()
			if err != nil {
				return commandError(cmd, asJSON, err)
			}
			task, err := svc.AddTask(crID, args[1])
			if err != nil {
				return commandError(cmd, asJSON, err)
			}
			if asJSON {
				return writeJSONSuccess(cmd, map[string]any{
					"cr_id": crID,
					"task":  taskToJSONMap(*task),
				})
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Added task %d to CR %d\n", task.ID, crID)
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "Output in JSON format")
	return cmd
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
				return commandError(cmd, asJSON, err)
			}
			svc, err := newService()
			if err != nil {
				return commandError(cmd, asJSON, err)
			}
			tasks, err := svc.ListTasks(crID)
			if err != nil {
				return commandError(cmd, asJSON, err)
			}
			if asJSON {
				taskMaps := make([]map[string]any, 0, len(tasks))
				for _, task := range tasks {
					taskMaps = append(taskMaps, taskToJSONMap(task))
				}
				return writeJSONSuccess(cmd, map[string]any{
					"cr_id": crID,
					"tasks": taskMaps,
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

type taskDoneFlags struct {
	noCheckpoint       bool
	noCheckpointReason string
	stageAll           bool
	fromContract       bool
	scopePaths         []string
	patchFile          string
}

func validateTaskDoneFlags(flags taskDoneFlags) error {
	trimmedReason := strings.TrimSpace(flags.noCheckpointReason)
	trimmedPatchFile := strings.TrimSpace(flags.patchFile)
	if flags.noCheckpoint && (flags.stageAll || flags.fromContract || len(flags.scopePaths) > 0 || trimmedPatchFile != "") {
		return fmt.Errorf("--no-checkpoint cannot be combined with --from-contract, --path, --patch-file, or --all")
	}
	if flags.noCheckpoint && trimmedReason == "" {
		return fmt.Errorf("--no-checkpoint requires --no-checkpoint-reason")
	}
	if !flags.noCheckpoint && trimmedReason != "" {
		return fmt.Errorf("--no-checkpoint-reason requires --no-checkpoint")
	}
	if flags.noCheckpoint {
		return nil
	}
	modeCount := 0
	if flags.stageAll {
		modeCount++
	}
	if flags.fromContract {
		modeCount++
	}
	if len(flags.scopePaths) > 0 {
		modeCount++
	}
	if trimmedPatchFile != "" {
		modeCount++
	}
	if modeCount > 1 {
		return fmt.Errorf("exactly one checkpoint scope mode is required: --from-contract, --path <file> (repeatable), --patch-file <file>, or --all")
	}
	if modeCount == 0 {
		return fmt.Errorf("checkpoint scope required: use --from-contract, --path <file> (repeatable), --patch-file <file>, or --all")
	}
	return nil
}

func buildTaskDoneOptions(flags taskDoneFlags) service.DoneTaskOptions {
	return service.DoneTaskOptions{
		Checkpoint:         !flags.noCheckpoint,
		StageAll:           flags.stageAll,
		FromContract:       flags.fromContract,
		Paths:              append([]string(nil), flags.scopePaths...),
		PatchFile:          strings.TrimSpace(flags.patchFile),
		NoCheckpointReason: strings.TrimSpace(flags.noCheckpointReason),
	}
}

func taskDoneScopeMode(flags taskDoneFlags) string {
	if flags.noCheckpoint {
		return "none"
	}
	if flags.stageAll {
		return "all"
	}
	if flags.fromContract {
		return "from_contract"
	}
	if len(flags.scopePaths) > 0 {
		return "path"
	}
	if strings.TrimSpace(flags.patchFile) != "" {
		return "patch_file"
	}
	return "unknown"
}

func taskDoneCheckpointSource(flags taskDoneFlags) string {
	if flags.noCheckpoint {
		return "task_no_checkpoint"
	}
	return "task_checkpoint"
}

func writeTaskDoneResult(cmd *cobra.Command, asJSON bool, crID, taskID int, sha string, flags taskDoneFlags) error {
	if asJSON {
		return writeJSONSuccess(cmd, map[string]any{
			"cr_id":                crID,
			"task_id":              taskID,
			"checkpoint":           !flags.noCheckpoint,
			"checkpoint_commit":    strings.TrimSpace(sha),
			"scope_mode":           taskDoneScopeMode(flags),
			"scope_paths":          stringSliceOrEmpty(flags.scopePaths),
			"patch_file":           strings.TrimSpace(flags.patchFile),
			"no_checkpoint_reason": strings.TrimSpace(flags.noCheckpointReason),
			"checkpoint_source":    taskDoneCheckpointSource(flags),
		})
	}
	if flags.noCheckpoint {
		fmt.Fprintf(cmd.OutOrStdout(), "Marked task %d done in CR %d (no checkpoint): %s\n", taskID, crID, strings.TrimSpace(flags.noCheckpointReason))
		return nil
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Marked task %d done in CR %d with checkpoint %s\n", taskID, crID, nonEmpty(sha, "-"))
	return nil
}

func newCRTaskDoneCmd() *cobra.Command {
	var noCheckpoint bool
	var noCheckpointReason string
	var stageAll bool
	var fromContract bool
	var scopePaths []string
	var patchFile string
	var asJSON bool

	cmd := &cobra.Command{
		Use:     "done <cr-id> <task-id>",
		Short:   "Mark a subtask as done",
		Long:    "Complete a task with one explicit checkpoint scope mode. Prefer --from-contract once task contract scope is defined.",
		Example: "  sophia cr task done 25 1 --from-contract\n  sophia cr task done 25 1 --path internal/service/service.go --path internal/service/service_test.go\n  sophia cr task done 25 1 --patch-file /tmp/task1.patch\n  sophia cr task done 25 1 --all\n  sophia cr task done 25 1 --no-checkpoint --no-checkpoint-reason \"metadata-only task\"",
		Args:    cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			crID, err := parsePositiveIntArg(args[0], "cr-id")
			if err != nil {
				return commandError(cmd, asJSON, err)
			}
			taskID, err := parsePositiveIntArg(args[1], "task-id")
			if err != nil {
				return commandError(cmd, asJSON, err)
			}
			flags := taskDoneFlags{
				noCheckpoint:       noCheckpoint,
				noCheckpointReason: noCheckpointReason,
				stageAll:           stageAll,
				fromContract:       fromContract,
				scopePaths:         append([]string(nil), scopePaths...),
				patchFile:          patchFile,
			}
			svc, err := newService()
			if err != nil {
				return commandError(cmd, asJSON, err)
			}
			if err := validateTaskDoneFlags(flags); err != nil {
				return commandError(cmd, asJSON, err)
			}
			opts := buildTaskDoneOptions(flags)
			sha, err := svc.DoneTaskWithCheckpoint(crID, taskID, opts)
			if err != nil {
				return commandError(cmd, asJSON, err)
			}
			return writeTaskDoneResult(cmd, asJSON, crID, taskID, sha, flags)
		},
	}

	cmd.Flags().BoolVar(&noCheckpoint, "no-checkpoint", false, "Mark task done without creating a checkpoint commit")
	cmd.Flags().StringVar(&noCheckpointReason, "no-checkpoint-reason", "", "Reason for metadata-only completion when using --no-checkpoint")
	cmd.Flags().BoolVar(&stageAll, "all", false, "Checkpoint by staging all changes explicitly")
	cmd.Flags().BoolVar(&fromContract, "from-contract", false, "Checkpoint by staging changed files that match task contract scope")
	cmd.Flags().StringArrayVar(&scopePaths, "path", nil, "Checkpoint scope path (repo-relative file, repeatable)")
	cmd.Flags().StringVar(&patchFile, "patch-file", "", "Checkpoint scope patch manifest file")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Output in JSON format")
	return cmd
}

func newCRTaskReopenCmd() *cobra.Command {
	var clearCheckpoint bool
	var asJSON bool

	cmd := &cobra.Command{
		Use:     "reopen <cr-id> <task-id>",
		Short:   "Reopen a completed subtask",
		Example: "  sophia cr task reopen 25 1\n  sophia cr task reopen 25 1 --clear-checkpoint\n  sophia cr task reopen 25 1 --json",
		Args:    cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			crID, err := parsePositiveIntArg(args[0], "cr-id")
			if err != nil {
				return commandError(cmd, asJSON, err)
			}
			taskID, err := parsePositiveIntArg(args[1], "task-id")
			if err != nil {
				return commandError(cmd, asJSON, err)
			}
			svc, err := newService()
			if err != nil {
				return commandError(cmd, asJSON, err)
			}
			task, err := svc.ReopenTask(crID, taskID, service.ReopenTaskOptions{
				ClearCheckpoint: clearCheckpoint,
			})
			if err != nil {
				return commandError(cmd, asJSON, err)
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
