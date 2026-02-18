package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"sophia/internal/service"
)

func newCRDiffCmd() *cobra.Command {
	var taskID int
	var criticalOnly bool
	var asJSON bool

	cmd := &cobra.Command{
		Use:   "diff <id>",
		Short: "Show deterministic CR/task diff lenses",
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
			view, err := svc.DiffCR(id, service.CRDiffOptions{
				TaskID:       taskID,
				CriticalOnly: criticalOnly,
			})
			if err != nil {
				if asJSON {
					return writeJSONError(cmd, err)
				}
				return err
			}
			if asJSON {
				return writeJSONSuccess(cmd, crDiffToJSONMap(view))
			}
			printCRDiffView(cmd, view)
			return nil
		},
	}

	cmd.Flags().IntVar(&taskID, "task", 0, "Task id lens within the CR diff")
	cmd.Flags().BoolVar(&criticalOnly, "critical", false, "Filter to contract risk_critical_scopes only")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Output in JSON format")
	return cmd
}

func newCRTaskDiffCmd() *cobra.Command {
	var chunksOnly bool
	var asJSON bool

	cmd := &cobra.Command{
		Use:   "diff <cr-id> <task-id>",
		Short: "Show deterministic diff lens for one task",
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
			view, err := svc.DiffTask(crID, taskID, service.TaskDiffOptions{ChunksOnly: chunksOnly})
			if err != nil {
				if asJSON {
					return writeJSONError(cmd, err)
				}
				return err
			}
			if asJSON {
				return writeJSONSuccess(cmd, crDiffToJSONMap(view))
			}
			printCRDiffView(cmd, view)
			return nil
		},
	}

	cmd.Flags().BoolVar(&chunksOnly, "chunks", false, "Render output as chunk-centric view")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Output in JSON format")
	return cmd
}

func newCRTaskChunkDiffCmd() *cobra.Command {
	var asJSON bool

	cmd := &cobra.Command{
		Use:   "diff <cr-id> <task-id> <chunk-id>",
		Short: "Show deterministic diff view for a specific checkpoint chunk",
		Args:  cobra.ExactArgs(3),
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
			chunkID := strings.TrimSpace(args[2])
			if chunkID == "" {
				err := fmt.Errorf("chunk-id cannot be empty")
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
			view, err := svc.DiffTaskChunk(crID, taskID, chunkID)
			if err != nil {
				if asJSON {
					return writeJSONError(cmd, err)
				}
				return err
			}
			if asJSON {
				return writeJSONSuccess(cmd, crDiffToJSONMap(view))
			}
			printCRDiffView(cmd, view)
			return nil
		},
	}

	cmd.Flags().BoolVar(&asJSON, "json", false, "Output in JSON format")
	return cmd
}

func printCRDiffView(cmd *cobra.Command, view *service.CRDiffView) {
	if view == nil {
		fmt.Fprintln(cmd.OutOrStdout(), "No diff view available.")
		return
	}
	fmt.Fprintf(cmd.OutOrStdout(), "CR Diff View\n")
	fmt.Fprintf(cmd.OutOrStdout(), "Mode: %s\n", nonEmpty(strings.TrimSpace(view.Mode), "-"))
	fmt.Fprintf(cmd.OutOrStdout(), "CR: %d\n", view.CRID)
	if view.TaskID > 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "Task: %d\n", view.TaskID)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Base Ref: %s\n", nonEmpty(strings.TrimSpace(view.BaseRef), "-"))
	fmt.Fprintf(cmd.OutOrStdout(), "Base Commit: %s\n", nonEmpty(strings.TrimSpace(view.BaseCommit), "-"))
	fmt.Fprintf(cmd.OutOrStdout(), "Target Ref: %s\n", nonEmpty(strings.TrimSpace(view.TargetRef), "-"))
	fmt.Fprintf(cmd.OutOrStdout(), "Critical Only: %t\n", view.CriticalOnly)
	fmt.Fprintf(cmd.OutOrStdout(), "Chunks Only: %t\n", view.ChunksOnly)
	fmt.Fprintf(cmd.OutOrStdout(), "Fallback Used: %t\n", view.FallbackUsed)
	if strings.TrimSpace(view.FallbackReason) != "" {
		fmt.Fprintf(cmd.OutOrStdout(), "Fallback Reason: %s\n", view.FallbackReason)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Files Changed: %d\n", view.FilesChanged)
	fmt.Fprintf(cmd.OutOrStdout(), "Diff Stat: %s\n", nonEmpty(strings.TrimSpace(view.ShortStat), "-"))
	if len(view.Warnings) > 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "\nWarnings:")
		for _, warning := range view.Warnings {
			fmt.Fprintf(cmd.OutOrStdout(), "- %s\n", warning)
		}
	}
	fmt.Fprintln(cmd.OutOrStdout(), "\nFiles:")
	if len(view.Files) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "- (none)")
		return
	}
	for _, file := range view.Files {
		fmt.Fprintf(cmd.OutOrStdout(), "- %s\n", file.Path)
		if len(file.Hunks) == 0 {
			fmt.Fprintln(cmd.OutOrStdout(), "  (no hunks)")
			continue
		}
		for _, hunk := range file.Hunks {
			fmt.Fprintf(cmd.OutOrStdout(), "  - %s old=%d,%d new=%d,%d source=%s\n", nonEmpty(strings.TrimSpace(hunk.ChunkID), "-"), hunk.OldStart, hunk.OldLines, hunk.NewStart, hunk.NewLines, nonEmpty(strings.TrimSpace(hunk.Source), "-"))
			if strings.TrimSpace(hunk.Preview) != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "    preview: %s\n", strings.TrimSpace(hunk.Preview))
			}
		}
	}
}

func newCRRangeDiffCmd() *cobra.Command {
	var fromRef string
	var toRef string
	var sinceLastCheckpoint bool
	var asJSON bool

	cmd := &cobra.Command{
		Use:   "rangediff <id>",
		Short: "Show deterministic range-diff view for a CR",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parsePositiveIntArg(args[0], "id")
			if err != nil {
				if asJSON {
					return writeJSONError(cmd, err)
				}
				return err
			}
			if sinceLastCheckpoint && strings.TrimSpace(fromRef) != "" {
				err := fmt.Errorf("--from and --since-last-checkpoint are mutually exclusive")
				if asJSON {
					return writeJSONError(cmd, err)
				}
				return err
			}
			if !sinceLastCheckpoint && strings.TrimSpace(fromRef) == "" {
				err := fmt.Errorf("either --from or --since-last-checkpoint is required")
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
			view, err := svc.RangeDiffCR(id, service.RangeDiffOptions{
				FromRef:             strings.TrimSpace(fromRef),
				ToRef:               strings.TrimSpace(toRef),
				SinceLastCheckpoint: sinceLastCheckpoint,
			})
			if err != nil {
				if asJSON {
					return writeJSONError(cmd, err)
				}
				return err
			}
			if asJSON {
				return writeJSONSuccess(cmd, rangeDiffToJSONMap(view))
			}
			printRangeDiffView(cmd, view)
			return nil
		},
	}

	cmd.Flags().StringVar(&fromRef, "from", "", "Source anchor ref")
	cmd.Flags().StringVar(&toRef, "to", "", "Target anchor ref (default: CR branch/merged commit)")
	cmd.Flags().BoolVar(&sinceLastCheckpoint, "since-last-checkpoint", false, "Use latest done checkpoint commit as --from anchor")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Output in JSON format")
	return cmd
}

func newCRTaskRangeDiffCmd() *cobra.Command {
	var fromRef string
	var toRef string
	var sinceLastCheckpoint bool
	var asJSON bool

	cmd := &cobra.Command{
		Use:   "rangediff <cr-id> <task-id>",
		Short: "Show deterministic range-diff view for one task",
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
			if sinceLastCheckpoint && strings.TrimSpace(fromRef) != "" {
				err := fmt.Errorf("--from and --since-last-checkpoint are mutually exclusive")
				if asJSON {
					return writeJSONError(cmd, err)
				}
				return err
			}
			if !sinceLastCheckpoint && strings.TrimSpace(fromRef) == "" {
				err := fmt.Errorf("either --from or --since-last-checkpoint is required")
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
			view, err := svc.RangeDiffTask(crID, taskID, service.RangeDiffOptions{
				FromRef:             strings.TrimSpace(fromRef),
				ToRef:               strings.TrimSpace(toRef),
				SinceLastCheckpoint: sinceLastCheckpoint,
			})
			if err != nil {
				if asJSON {
					return writeJSONError(cmd, err)
				}
				return err
			}
			if asJSON {
				return writeJSONSuccess(cmd, rangeDiffToJSONMap(view))
			}
			printRangeDiffView(cmd, view)
			return nil
		},
	}

	cmd.Flags().StringVar(&fromRef, "from", "", "Source anchor ref")
	cmd.Flags().StringVar(&toRef, "to", "", "Target anchor ref (default: CR branch/merged commit)")
	cmd.Flags().BoolVar(&sinceLastCheckpoint, "since-last-checkpoint", false, "Use latest done checkpoint commit as --from anchor")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Output in JSON format")
	return cmd
}

func printRangeDiffView(cmd *cobra.Command, view *service.RangeDiffView) {
	if view == nil {
		fmt.Fprintln(cmd.OutOrStdout(), "No range-diff view available.")
		return
	}
	fmt.Fprintln(cmd.OutOrStdout(), "RangeDiff View")
	fmt.Fprintf(cmd.OutOrStdout(), "CR: %d\n", view.CRID)
	if view.TaskID > 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "Task: %d\n", view.TaskID)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "From: %s\n", nonEmpty(strings.TrimSpace(view.FromRef), "-"))
	fmt.Fprintf(cmd.OutOrStdout(), "To: %s\n", nonEmpty(strings.TrimSpace(view.ToRef), "-"))
	fmt.Fprintf(cmd.OutOrStdout(), "Base: %s\n", nonEmpty(strings.TrimSpace(view.BaseRef), "-"))
	fmt.Fprintf(cmd.OutOrStdout(), "Old Range: %s\n", nonEmpty(strings.TrimSpace(view.OldRange), "-"))
	fmt.Fprintf(cmd.OutOrStdout(), "New Range: %s\n", nonEmpty(strings.TrimSpace(view.NewRange), "-"))
	fmt.Fprintf(cmd.OutOrStdout(), "Diff Stat: %s\n", nonEmpty(strings.TrimSpace(view.ShortStat), "-"))
	printListSection(cmd, "Files Changed", view.FilesChanged)
	fmt.Fprintln(cmd.OutOrStdout(), "\nCommit Mapping:")
	if len(view.Mapping) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "- (none)")
	} else {
		for _, row := range view.Mapping {
			fmt.Fprintf(cmd.OutOrStdout(), "- old=(%s %s) relation=%s new=(%s %s) subject=%s\n",
				nonEmpty(strings.TrimSpace(row.OldIndex), "-"),
				nonEmpty(strings.TrimSpace(row.OldCommit), "-"),
				nonEmpty(strings.TrimSpace(row.Relation), "-"),
				nonEmpty(strings.TrimSpace(row.NewIndex), "-"),
				nonEmpty(strings.TrimSpace(row.NewCommit), "-"),
				nonEmpty(strings.TrimSpace(row.Subject), "-"))
		}
	}
	if len(view.Warnings) > 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "\nWarnings:")
		for _, warning := range view.Warnings {
			fmt.Fprintf(cmd.OutOrStdout(), "- %s\n", warning)
		}
	}
}
