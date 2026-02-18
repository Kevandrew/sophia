package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"sophia/internal/service"
)

func newCRExportCmd() *cobra.Command {
	var format string
	var include []string
	var outPath string

	cmd := &cobra.Command{
		Use:   "export <id>",
		Short: "Export a canonical CR bundle artifact",
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
			bundle, payload, err := svc.ExportCRBundle(id, service.ExportCROptions{
				Format:  format,
				Include: include,
			})
			if err != nil {
				return err
			}

			outPath = strings.TrimSpace(outPath)
			if outPath != "" {
				if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
					return fmt.Errorf("create export directory: %w", err)
				}
				if err := os.WriteFile(outPath, payload, 0o644); err != nil {
					return fmt.Errorf("write export bundle: %w", err)
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Exported CR %d bundle to %s\n", id, outPath)
				fmt.Fprintf(cmd.OutOrStdout(), "Schema: %s\n", bundle.SchemaVersion)
				fmt.Fprintf(cmd.OutOrStdout(), "Includes: %s\n", nonEmpty(strings.Join(bundle.Includes, ","), "(none)"))
				return nil
			}

			if len(payload) > 0 {
				_, _ = cmd.OutOrStdout().Write(payload)
				_, _ = cmd.OutOrStdout().Write([]byte("\n"))
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&format, "format", "json", "Export format (currently: json)")
	cmd.Flags().StringSliceVar(&include, "include", nil, "Optional sections to include (supported: diffs)")
	cmd.Flags().StringVar(&outPath, "out", "", "Output path for bundle file (stdout when omitted)")
	return cmd
}
