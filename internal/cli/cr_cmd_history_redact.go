package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

func newCRRedactCmd() *cobra.Command {
	var noteIndex int
	var eventIndex int
	var reason string
	var asJSON bool

	cmd := &cobra.Command{
		Use:   "redact <id>",
		Short: "Redact CR note/event payload with audit event",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parsePositiveIntArg(args[0], "id")
			if err != nil {
				return commandError(cmd, asJSON, err)
			}
			reason = strings.TrimSpace(reason)
			if reason == "" {
				err := fmt.Errorf("--reason is required")
				return commandError(cmd, asJSON, err)
			}

			noteChanged := cmd.Flags().Changed("note-index")
			eventChanged := cmd.Flags().Changed("event-index")
			if noteChanged == eventChanged {
				err := fmt.Errorf("provide exactly one of --note-index or --event-index")
				return commandError(cmd, asJSON, err)
			}

			svc, err := newService()
			if err != nil {
				return commandError(cmd, asJSON, err)
			}
			if noteChanged {
				if err := svc.RedactCRNote(id, noteIndex, reason); err != nil {
					return commandError(cmd, asJSON, err)
				}
				if asJSON {
					return writeJSONSuccess(cmd, map[string]any{
						"cr_id":  id,
						"target": "note",
						"index":  noteIndex,
						"reason": reason,
					})
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Redacted note #%d in CR %d\n", noteIndex, id)
				return nil
			}
			if err := svc.RedactCREvent(id, eventIndex, reason); err != nil {
				return commandError(cmd, asJSON, err)
			}
			if asJSON {
				return writeJSONSuccess(cmd, map[string]any{
					"cr_id":  id,
					"target": "event",
					"index":  eventIndex,
					"reason": reason,
				})
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Redacted event #%d in CR %d\n", eventIndex, id)
			return nil
		},
	}

	cmd.Flags().IntVar(&noteIndex, "note-index", 0, "1-based note index to redact")
	cmd.Flags().IntVar(&eventIndex, "event-index", 0, "1-based event index to redact")
	cmd.Flags().StringVar(&reason, "reason", "", "Redaction reason (required)")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Output in JSON format")
	return cmd
}

func newCRHistoryCmd() *cobra.Command {
	var showRedacted bool
	var asJSON bool

	cmd := &cobra.Command{
		Use:   "history <id>",
		Short: "Show CR notes/events timeline with indices",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, svc, err := parseIDAndService(args[0], "id")
			if err != nil {
				return commandError(cmd, asJSON, err)
			}
			history, err := svc.HistoryCR(id, showRedacted)
			if err != nil {
				return commandError(cmd, asJSON, err)
			}
			if asJSON {
				return writeJSONSuccess(cmd, historyToJSONMap(history, showRedacted))
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

			fmt.Fprintln(cmd.OutOrStdout(), "\nEvidence:")
			if len(history.Evidence) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "- (none)")
			} else {
				for _, evidence := range history.Evidence {
					fmt.Fprintf(cmd.OutOrStdout(), "- #%d %s %s %s: %s\n", evidence.Index, nonEmpty(strings.TrimSpace(evidence.TS), "-"), nonEmpty(strings.TrimSpace(evidence.Type), "-"), nonEmpty(strings.TrimSpace(evidence.Actor), "-"), nonEmpty(strings.TrimSpace(evidence.Summary), "-"))
					if strings.TrimSpace(evidence.Scope) != "" {
						fmt.Fprintf(cmd.OutOrStdout(), "  scope: %s\n", evidence.Scope)
					}
					if strings.TrimSpace(evidence.Command) != "" {
						fmt.Fprintf(cmd.OutOrStdout(), "  command: %s\n", evidence.Command)
					}
					if evidence.ExitCode != nil {
						fmt.Fprintf(cmd.OutOrStdout(), "  exit_code: %d\n", *evidence.ExitCode)
					}
					if strings.TrimSpace(evidence.OutputHash) != "" {
						fmt.Fprintf(cmd.OutOrStdout(), "  output_hash: %s\n", evidence.OutputHash)
					}
					if len(evidence.Attachments) > 0 {
						fmt.Fprintf(cmd.OutOrStdout(), "  attachments: %s\n", strings.Join(evidence.Attachments, ", "))
					}
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
	cmd.Flags().BoolVar(&asJSON, "json", false, "Output in JSON format")
	return cmd
}
