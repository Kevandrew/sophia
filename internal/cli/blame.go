package cli

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"sophia/internal/service"
)

func newBlameCmd() *cobra.Command {
	var rev string
	var lineRanges []string
	var asJSON bool

	cmd := &cobra.Command{
		Use:   "blame <path>",
		Short: "Show Sophia-enriched line attribution for a file",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ranges, err := parseBlameRangeFlags(lineRanges)
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

			view, err := svc.BlameFile(args[0], service.BlameOptions{Rev: rev, Ranges: ranges})
			if err != nil {
				if asJSON {
					return writeJSONError(cmd, err)
				}
				return err
			}

			if asJSON {
				return writeJSONSuccess(cmd, blameViewToJSONMap(view))
			}
			printBlameTable(cmd, view)
			return nil
		},
	}

	cmd.Flags().StringVar(&rev, "rev", "", "Git revision to blame (default: working tree against HEAD)")
	cmd.Flags().StringArrayVarP(&lineRanges, "lines", "L", nil, "Line range start,end (repeatable)")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Output in JSON format")
	return cmd
}

func parseBlameRangeFlags(raw []string) ([]service.BlameRange, error) {
	if len(raw) == 0 {
		return []service.BlameRange{}, nil
	}
	ranges := make([]service.BlameRange, 0, len(raw))
	for _, value := range raw {
		trimmed := strings.TrimSpace(value)
		parts := strings.Split(trimmed, ",")
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid --lines value %q (expected start,end)", value)
		}
		start, err := strconv.Atoi(strings.TrimSpace(parts[0]))
		if err != nil {
			return nil, fmt.Errorf("invalid --lines value %q (expected start,end)", value)
		}
		end, err := strconv.Atoi(strings.TrimSpace(parts[1]))
		if err != nil {
			return nil, fmt.Errorf("invalid --lines value %q (expected start,end)", value)
		}
		if start <= 0 || end <= 0 {
			return nil, fmt.Errorf("invalid --lines value %q (line values must be >= 1)", value)
		}
		if start > end {
			return nil, fmt.Errorf("invalid --lines value %q (start must be <= end)", value)
		}
		ranges = append(ranges, service.BlameRange{Start: start, End: end})
	}
	return ranges, nil
}

func printBlameTable(cmd *cobra.Command, view *service.BlameView) {
	fmt.Fprintln(cmd.OutOrStdout(), "LINE\tAUTHOR\tDATE\tSHA\tCR\tINTENT\tCODE")
	for _, line := range view.Lines {
		date := "-"
		if len(line.AuthorTime) >= 10 {
			date = line.AuthorTime[:10]
		}
		cr := "-"
		if line.HasCR && line.CRID > 0 {
			cr = strconv.Itoa(line.CRID)
		}
		text := strings.ReplaceAll(line.Text, "\t", "    ")
		fmt.Fprintf(
			cmd.OutOrStdout(),
			"%d\t%s\t%s\t%s\t%s\t%s\t%s\n",
			line.Line,
			nonEmpty(strings.TrimSpace(line.Author), "-"),
			date,
			nonEmpty(strings.TrimSpace(line.Commit), "-"),
			cr,
			nonEmpty(strings.TrimSpace(line.Intent), "(none)"),
			truncateForBlameTable(text, 120),
		)
	}
}

func truncateForBlameTable(text string, maxChars int) string {
	if maxChars <= 0 {
		return ""
	}
	runes := []rune(text)
	if len(runes) <= maxChars {
		return text
	}
	return strings.TrimRight(string(runes[:maxChars]), " ") + "..."
}

func blameViewToJSONMap(view *service.BlameView) map[string]any {
	ranges := make([]map[string]any, 0, len(view.Ranges))
	for _, r := range view.Ranges {
		ranges = append(ranges, map[string]any{
			"start": r.Start,
			"end":   r.End,
		})
	}

	lines := make([]map[string]any, 0, len(view.Lines))
	for _, line := range view.Lines {
		var crID any
		if line.HasCR {
			crID = line.CRID
		}
		lines = append(lines, map[string]any{
			"line":          line.Line,
			"commit":        line.Commit,
			"author":        line.Author,
			"author_email":  line.AuthorEmail,
			"author_time":   line.AuthorTime,
			"cr_id":         crID,
			"cr_uid":        line.CRUID,
			"intent":        line.Intent,
			"intent_source": line.IntentSource,
			"summary":       line.Summary,
			"text":          line.Text,
		})
	}

	return map[string]any{
		"path":   view.Path,
		"rev":    view.Rev,
		"ranges": ranges,
		"lines":  lines,
	}
}
