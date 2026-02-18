package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"sophia/internal/model"
	"sophia/internal/service"
)

func newCREvidenceCmd() *cobra.Command {
	evidenceCmd := &cobra.Command{
		Use:   "evidence",
		Short: "Manage CR evidence ledger entries",
	}
	evidenceCmd.AddCommand(newCREvidenceAddCmd())
	evidenceCmd.AddCommand(newCREvidenceShowCmd())
	evidenceCmd.AddCommand(newCREvidenceSampleCmd())
	return evidenceCmd
}

func newCREvidenceAddCmd() *cobra.Command {
	var evidenceType string
	var scope string
	var summary string
	var text string
	var command string
	var capture bool
	var exitCode int
	var attachments []string
	var asJSON bool

	cmd := &cobra.Command{
		Use:   "add <id>",
		Short: "Add an evidence ledger entry to a CR",
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

			combinedSummary := strings.TrimSpace(summary)
			if combinedSummary == "" {
				combinedSummary = strings.TrimSpace(text)
			}
			var exitCodePtr *int
			if cmd.Flags().Changed("exit-code") {
				value := exitCode
				exitCodePtr = &value
			}
			entry, err := svc.AddEvidence(id, service.AddEvidenceOptions{
				Type:        evidenceType,
				Scope:       scope,
				Summary:     combinedSummary,
				Command:     command,
				Capture:     capture,
				ExitCode:    exitCodePtr,
				Attachments: append([]string(nil), attachments...),
			})
			if err != nil {
				if asJSON {
					return writeJSONError(cmd, err)
				}
				return err
			}
			if asJSON {
				return writeJSONSuccess(cmd, map[string]any{
					"cr_id":    id,
					"evidence": evidenceEntryToJSONMap(*entry),
				})
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Added %s evidence to CR %d\n", entry.Type, id)
			return nil
		},
	}

	cmd.Flags().StringVar(&evidenceType, "type", "", "Evidence type (command_run, manual_note, environment, benchmark, reproduction_steps, review_sample)")
	cmd.Flags().StringVar(&scope, "scope", "", "Optional evidence scope path/prefix")
	cmd.Flags().StringVar(&summary, "summary", "", "Evidence summary text")
	cmd.Flags().StringVar(&text, "text", "", "Alias for --summary (useful for manual_note)")
	cmd.Flags().StringVar(&command, "cmd", "", "Command text for command_run evidence")
	cmd.Flags().BoolVar(&capture, "capture", false, "Execute --cmd and capture exit code/hash/summary")
	cmd.Flags().IntVar(&exitCode, "exit-code", 0, "Optional exit code when adding evidence without --capture")
	cmd.Flags().StringArrayVar(&attachments, "attachment", nil, "Optional attachment path (repeatable)")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Output in JSON format")
	_ = cmd.MarkFlagRequired("type")
	return cmd
}

func newCREvidenceShowCmd() *cobra.Command {
	var asJSON bool

	cmd := &cobra.Command{
		Use:   "show <id>",
		Short: "Show evidence ledger entries for a CR",
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
			entries, err := svc.ListEvidence(id)
			if err != nil {
				if asJSON {
					return writeJSONError(cmd, err)
				}
				return err
			}
			if asJSON {
				items := make([]map[string]any, 0, len(entries))
				for _, entry := range entries {
					items = append(items, evidenceEntryToJSONMap(entry))
				}
				return writeJSONSuccess(cmd, map[string]any{
					"cr_id":    id,
					"count":    len(items),
					"evidence": items,
				})
			}
			if len(entries) == 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "No evidence entries found for CR %d.\n", id)
				return nil
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Evidence for CR %d:\n", id)
			for i, entry := range entries {
				fmt.Fprintf(cmd.OutOrStdout(), "- #%d %s %s %s\n", i+1, nonEmpty(strings.TrimSpace(entry.TS), "-"), nonEmpty(strings.TrimSpace(entry.Type), "-"), nonEmpty(strings.TrimSpace(entry.Summary), "-"))
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
			return nil
		},
	}

	cmd.Flags().BoolVar(&asJSON, "json", false, "Output in JSON format")
	return cmd
}

func evidenceEntryToJSONMap(entry model.EvidenceEntry) map[string]any {
	var exitCode any = nil
	if entry.ExitCode != nil {
		exitCode = *entry.ExitCode
	}
	return map[string]any{
		"ts":          entry.TS,
		"actor":       entry.Actor,
		"type":        entry.Type,
		"scope":       entry.Scope,
		"command":     entry.Command,
		"exit_code":   exitCode,
		"output_hash": entry.OutputHash,
		"summary":     entry.Summary,
		"attachments": append([]string(nil), entry.Attachments...),
	}
}

func newCREvidenceSampleCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "sample",
		Short: "Add or list review_sample evidence entries",
	}
	cmd.AddCommand(newCREvidenceSampleAddCmd())
	cmd.AddCommand(newCREvidenceSampleListCmd())
	return cmd
}

func newCREvidenceSampleAddCmd() *cobra.Command {
	var scope string
	var summary string
	var attachments []string
	var asJSON bool

	cmd := &cobra.Command{
		Use:   "add <id>",
		Short: "Add a review_sample evidence entry",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parsePositiveIntArg(args[0], "id")
			if err != nil {
				if asJSON {
					return writeJSONError(cmd, err)
				}
				return err
			}
			if strings.TrimSpace(scope) == "" {
				err := fmt.Errorf("--scope is required")
				if asJSON {
					return writeJSONError(cmd, err)
				}
				return err
			}
			if strings.TrimSpace(summary) == "" {
				err := fmt.Errorf("--summary is required")
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
			entry, err := svc.AddEvidence(id, service.AddEvidenceOptions{
				Type:        "review_sample",
				Scope:       strings.TrimSpace(scope),
				Summary:     strings.TrimSpace(summary),
				Attachments: append([]string(nil), attachments...),
			})
			if err != nil {
				if asJSON {
					return writeJSONError(cmd, err)
				}
				return err
			}
			if asJSON {
				return writeJSONSuccess(cmd, map[string]any{
					"cr_id":    id,
					"evidence": evidenceEntryToJSONMap(*entry),
				})
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Added review_sample evidence to CR %d for scope %s\n", id, strings.TrimSpace(scope))
			return nil
		},
	}
	cmd.Flags().StringVar(&scope, "scope", "", "Review sample scope path/prefix")
	cmd.Flags().StringVar(&summary, "summary", "", "Review sample summary")
	cmd.Flags().StringArrayVar(&attachments, "attachment", nil, "Optional attachment path (repeatable)")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Output in JSON format")
	return cmd
}

func newCREvidenceSampleListCmd() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "list <id>",
		Short: "List review_sample evidence entries for a CR",
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
			entries, err := svc.ListEvidence(id)
			if err != nil {
				if asJSON {
					return writeJSONError(cmd, err)
				}
				return err
			}
			samples := make([]model.EvidenceEntry, 0, len(entries))
			for _, entry := range entries {
				if strings.TrimSpace(entry.Type) != "review_sample" {
					continue
				}
				samples = append(samples, entry)
			}
			if asJSON {
				items := make([]map[string]any, 0, len(samples))
				for _, entry := range samples {
					items = append(items, evidenceEntryToJSONMap(entry))
				}
				return writeJSONSuccess(cmd, map[string]any{
					"cr_id":   id,
					"count":   len(items),
					"samples": items,
				})
			}
			if len(samples) == 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "No review_sample evidence entries found for CR %d.\n", id)
				return nil
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Review samples for CR %d:\n", id)
			for i, entry := range samples {
				fmt.Fprintf(cmd.OutOrStdout(), "- #%d %s %s\n", i+1, nonEmpty(strings.TrimSpace(entry.Scope), "-"), nonEmpty(strings.TrimSpace(entry.Summary), "-"))
				if strings.TrimSpace(entry.TS) != "" {
					fmt.Fprintf(cmd.OutOrStdout(), "  ts: %s\n", entry.TS)
				}
				if len(entry.Attachments) > 0 {
					fmt.Fprintf(cmd.OutOrStdout(), "  attachments: %s\n", strings.Join(entry.Attachments, ", "))
				}
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "Output in JSON format")
	return cmd
}
