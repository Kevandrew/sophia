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

	cmd.Flags().StringVar(&evidenceType, "type", "", "Evidence type (command_run, manual_note, environment, benchmark, reproduction_steps)")
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
