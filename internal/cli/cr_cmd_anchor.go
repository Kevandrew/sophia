package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

func newCRRangeCmd() *cobra.Command {
	var asJSON bool

	cmd := &cobra.Command{
		Use:   "range <id>",
		Short: "Show canonical base/head/merge-base anchors for a CR",
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
			view, err := svc.RangeCR(id)
			if err != nil {
				if asJSON {
					return writeJSONError(cmd, err)
				}
				return err
			}
			if asJSON {
				return writeJSONSuccess(cmd, crRangeAnchorsToJSONMap(view))
			}
			fmt.Fprintf(cmd.OutOrStdout(), "CR: %d\n", view.CRID)
			fmt.Fprintf(cmd.OutOrStdout(), "Base: %s\n", nonEmpty(strings.TrimSpace(view.Base), "-"))
			fmt.Fprintf(cmd.OutOrStdout(), "Head: %s\n", nonEmpty(strings.TrimSpace(view.Head), "-"))
			fmt.Fprintf(cmd.OutOrStdout(), "Merge Base: %s\n", nonEmpty(strings.TrimSpace(view.MergeBase), "-"))
			if len(view.Warnings) > 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "\nWarnings:")
				for _, warning := range view.Warnings {
					fmt.Fprintf(cmd.OutOrStdout(), "- %s\n", warning)
				}
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&asJSON, "json", false, "Output in JSON format")
	return cmd
}

func newCRRevParseCmd() *cobra.Command {
	var kind string
	var asJSON bool

	cmd := &cobra.Command{
		Use:   "rev-parse <id>",
		Short: "Resolve one canonical CR anchor to a commit hash",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parsePositiveIntArg(args[0], "id")
			if err != nil {
				if asJSON {
					return writeJSONError(cmd, err)
				}
				return err
			}
			if strings.TrimSpace(kind) == "" {
				err := fmt.Errorf("--kind is required (base|head|merge-base)")
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
			view, err := svc.RevParseCR(id, kind)
			if err != nil {
				if asJSON {
					return writeJSONError(cmd, err)
				}
				return err
			}
			if asJSON {
				return writeJSONSuccess(cmd, crRevParseToJSONMap(view))
			}
			fmt.Fprintln(cmd.OutOrStdout(), strings.TrimSpace(view.Commit))
			return nil
		},
	}

	cmd.Flags().StringVar(&kind, "kind", "", "Anchor kind: base|head|merge-base")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Output in JSON format")
	return cmd
}
