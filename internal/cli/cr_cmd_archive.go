package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"sophia/internal/model"
	"sophia/internal/service"
)

func newCRArchiveCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "archive",
		Short: "Generate append-only CR archive artifacts",
	}
	cmd.AddCommand(newCRArchiveWriteCmd())
	cmd.AddCommand(newCRArchiveAppendCmd())
	cmd.AddCommand(newCRArchiveBackfillCmd())
	cmd.AddCommand(newCRArchiveAbandonCmd())
	cmd.AddCommand(newCRArchiveResumeCmd())
	return cmd
}

func newCRArchiveWriteCmd() *cobra.Command {
	var outPath string
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "write <id>",
		Short: "Write the next archive revision for a merged CR",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parsePositiveIntArg(args[0], "id")
			if err != nil {
				return commandError(cmd, asJSON, err)
			}
			svc, err := newServiceForCmd(cmd)
			if err != nil {
				return commandError(cmd, asJSON, err)
			}
			view, err := svc.WriteCRArchive(id, service.CRArchiveWriteOptions{
				OutPath: resolvePathForCmd(cmd, outPath),
			})
			if err != nil {
				return commandError(cmd, asJSON, err)
			}
			if asJSON {
				return writeJSONSuccess(cmd, crArchiveWriteToJSONMap(view))
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Wrote CR %d archive revision v%d to %s\n", view.CRID, view.Revision, view.Path)
			return nil
		},
	}
	cmd.Flags().StringVar(&outPath, "out", "", "Output file override (defaults to configured archive path)")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Output in JSON format")
	return cmd
}

func newCRArchiveAppendCmd() *cobra.Command {
	var outPath string
	var reason string
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "append <id>",
		Short: "Write a correction archive revision (vN+1) with a reason",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parsePositiveIntArg(args[0], "id")
			if err != nil {
				return commandError(cmd, asJSON, err)
			}
			reason = strings.TrimSpace(reason)
			if reason == "" {
				return commandError(cmd, asJSON, fmt.Errorf("--reason is required"))
			}
			svc, err := newServiceForCmd(cmd)
			if err != nil {
				return commandError(cmd, asJSON, err)
			}
			view, err := svc.WriteCRArchive(id, service.CRArchiveWriteOptions{
				OutPath: resolvePathForCmd(cmd, outPath),
				Reason:  reason,
			})
			if err != nil {
				return commandError(cmd, asJSON, err)
			}
			if asJSON {
				return writeJSONSuccess(cmd, crArchiveWriteToJSONMap(view))
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Appended CR %d archive revision v%d to %s\n", view.CRID, view.Revision, view.Path)
			fmt.Fprintf(cmd.OutOrStdout(), "Reason: %s\n", reason)
			return nil
		},
	}
	cmd.Flags().StringVar(&outPath, "out", "", "Output file override (defaults to configured archive path)")
	cmd.Flags().StringVar(&reason, "reason", "", "Reason for archive append revision")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Output in JSON format")
	return cmd
}

func newCRArchiveBackfillCmd() *cobra.Command {
	var commit bool
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "backfill",
		Short: "Create missing v1 archive artifacts for merged CRs",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, err := newServiceForCmd(cmd)
			if err != nil {
				return commandError(cmd, asJSON, err)
			}
			view, err := svc.BackfillCRArchives(service.CRArchiveBackfillOptions{
				Commit: commit,
			})
			if err != nil {
				return commandError(cmd, asJSON, err)
			}
			if asJSON {
				return writeJSONSuccess(cmd, crArchiveBackfillToJSONMap(view))
			}
			if view.DryRun {
				fmt.Fprintf(cmd.OutOrStdout(), "Backfill dry-run: %d merged CR(s) scanned, %d missing v1 archive(s)\n", view.ScannedMerged, len(view.MissingCRIDs))
				for _, id := range view.MissingCRIDs {
					fmt.Fprintf(cmd.OutOrStdout(), "- CR %d\n", id)
				}
				return nil
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Backfill complete: wrote %d archive file(s)\n", len(view.WrittenPaths))
			if strings.TrimSpace(view.CommitSHA) != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "Commit: %s\n", view.CommitSHA)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&commit, "commit", false, "Write missing v1 archives and create a single commit")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Output in JSON format")
	return cmd
}

func newCRArchiveAbandonCmd() *cobra.Command {
	var asJSON bool
	var reason string
	cmd := &cobra.Command{
		Use:   "abandon <id>",
		Short: "Mark an in-progress CR as abandoned without merging",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parsePositiveIntArg(args[0], "id")
			if err != nil {
				return commandError(cmd, asJSON, err)
			}
			svc, err := newServiceForCmd(cmd)
			if err != nil {
				return commandError(cmd, asJSON, err)
			}
			view, err := svc.AbandonCR(id, service.CRAbandonOptions{Reason: strings.TrimSpace(reason)})
			if err != nil {
				return commandError(cmd, asJSON, err)
			}
			if asJSON {
				return writeJSONSuccess(cmd, crAbandonToJSONMap(view))
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Abandoned CR %d\n", view.CRID)
			fmt.Fprintf(cmd.OutOrStdout(), "Reason: %s\n", nonEmpty(view.AbandonedReason, "-"))
			fmt.Fprintf(cmd.OutOrStdout(), "Next: %s\n", strings.Join(view.SuggestedCommands, " or "))
			return nil
		},
	}
	cmd.Flags().StringVar(&reason, "reason", "", "Reason for abandoning this CR")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Output in JSON format")
	return cmd
}

func newCRArchiveResumeCmd() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "resume <id>",
		Short: "Resume an abandoned CR (alias of cr reopen)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parsePositiveIntArg(args[0], "id")
			if err != nil {
				return commandError(cmd, asJSON, err)
			}
			svc, err := newServiceForCmd(cmd)
			if err != nil {
				return commandError(cmd, asJSON, err)
			}
			cr, err := svc.ReopenCR(id)
			if err != nil {
				return commandError(cmd, asJSON, err)
			}
			if asJSON {
				return writeJSONSuccess(cmd, map[string]any{
					"cr_id":  cr.ID,
					"status": cr.Status,
					"branch": cr.Branch,
				})
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Resumed CR %d on branch %s\n", cr.ID, cr.Branch)
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "Output in JSON format")
	return cmd
}

func archivePolicyToJSONMap(config model.PolicyArchive) map[string]any {
	enabled := false
	if config.Enabled != nil {
		enabled = *config.Enabled
	}
	includeFullDiffs := false
	if config.IncludeFullDiffs != nil {
		includeFullDiffs = *config.IncludeFullDiffs
	}
	return map[string]any{
		"enabled":            enabled,
		"path":               config.Path,
		"format":             config.Format,
		"include_full_diffs": includeFullDiffs,
	}
}

func archiveGitSummaryToJSONMap(summary model.CRArchiveGitSummary) map[string]any {
	rows := make([]map[string]any, 0, len(summary.DiffStat.Files))
	for _, row := range summary.DiffStat.Files {
		item := map[string]any{
			"path":   row.Path,
			"binary": row.Binary,
		}
		if row.Insertions != nil {
			item["insertions"] = *row.Insertions
		}
		if row.Deletions != nil {
			item["deletions"] = *row.Deletions
		}
		rows = append(rows, item)
	}
	return map[string]any{
		"base_parent":   summary.BaseParent,
		"cr_parent":     summary.CRParent,
		"files_changed": stringSliceOrEmpty(summary.FilesChanged),
		"diffstat": map[string]any{
			"summary": summary.DiffStat.Summary,
			"files":   rows,
		},
	}
}

func archiveFullDiffToJSONMap(fullDiff *model.CRArchiveFullDiff) map[string]any {
	if fullDiff == nil {
		return map[string]any{}
	}
	return map[string]any{
		"encoding": fullDiff.Encoding,
		"bytes":    fullDiff.Bytes,
	}
}

func crArchiveWriteToJSONMap(view *service.CRArchiveWriteView) map[string]any {
	if view == nil {
		return map[string]any{}
	}
	out := map[string]any{
		"cr_id":       view.CRID,
		"cr_uid":      view.CRUID,
		"revision":    view.Revision,
		"path":        view.Path,
		"bytes":       view.Bytes,
		"reason":      view.Archive.Reason,
		"config":      archivePolicyToJSONMap(view.Config),
		"git_summary": archiveGitSummaryToJSONMap(view.GitSummary),
		"schema":      view.Archive.SchemaVersion,
		"archived_at": view.Archive.ArchivedAt,
		"notice":      view.Archive.Notice,
	}
	if view.Archive.FullDiff != nil {
		out["full_diff"] = archiveFullDiffToJSONMap(view.Archive.FullDiff)
	}
	return out
}

func crArchiveBackfillToJSONMap(view *service.CRArchiveBackfillView) map[string]any {
	if view == nil {
		return map[string]any{}
	}
	return map[string]any{
		"scanned_merged": view.ScannedMerged,
		"missing_cr_ids": intSliceOrEmpty(view.MissingCRIDs),
		"written_paths":  stringSliceOrEmpty(view.WrittenPaths),
		"committed":      view.Committed,
		"commit_sha":     view.CommitSHA,
		"dry_run":        view.DryRun,
		"config":         archivePolicyToJSONMap(view.Config),
	}
}

func crAbandonToJSONMap(view *service.CRAbandonView) map[string]any {
	if view == nil {
		return map[string]any{}
	}
	return map[string]any{
		"cr_id":              view.CRID,
		"cr_uid":             view.CRUID,
		"status":             view.Status,
		"abandoned_at":       view.AbandonedAt,
		"abandoned_by":       view.AbandonedBy,
		"abandoned_reason":   view.AbandonedReason,
		"action_required":    view.ActionRequired,
		"action_reason":      view.ActionReason,
		"suggested_commands": stringSliceOrEmpty(view.SuggestedCommands),
	}
}
