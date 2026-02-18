package cli

import (
	"encoding/json"
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
	var asJSON bool

	cmd := &cobra.Command{
		Use:   "export <id>",
		Short: "Export a canonical CR bundle artifact",
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
			bundle, payload, err := svc.ExportCRBundle(id, service.ExportCROptions{
				Format:  format,
				Include: include,
			})
			if err != nil {
				if asJSON {
					return writeJSONError(cmd, err)
				}
				return err
			}

			outPath = strings.TrimSpace(outPath)
			if outPath != "" {
				if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
					if asJSON {
						return writeJSONError(cmd, fmt.Errorf("create export directory: %w", err))
					}
					return fmt.Errorf("create export directory: %w", err)
				}
				if err := os.WriteFile(outPath, payload, 0o644); err != nil {
					if asJSON {
						return writeJSONError(cmd, fmt.Errorf("write export bundle: %w", err))
					}
					return fmt.Errorf("write export bundle: %w", err)
				}
				if asJSON {
					return writeJSONSuccess(cmd, map[string]any{
						"cr_id":          id,
						"format":         bundle.Format,
						"schema_version": bundle.SchemaVersion,
						"includes":       stringSliceOrEmpty(bundle.Includes),
						"out":            outPath,
						"bytes_written":  len(payload),
					})
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Exported CR %d bundle to %s\n", id, outPath)
				fmt.Fprintf(cmd.OutOrStdout(), "Schema: %s\n", bundle.SchemaVersion)
				fmt.Fprintf(cmd.OutOrStdout(), "Includes: %s\n", nonEmpty(strings.Join(bundle.Includes, ","), "(none)"))
				return nil
			}
			if asJSON {
				var decoded any
				if err := json.Unmarshal(payload, &decoded); err != nil {
					return writeJSONError(cmd, fmt.Errorf("decode export payload: %w", err))
				}
				return writeJSONSuccess(cmd, map[string]any{
					"cr_id":          id,
					"format":         bundle.Format,
					"schema_version": bundle.SchemaVersion,
					"includes":       stringSliceOrEmpty(bundle.Includes),
					"bundle":         decoded,
				})
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
	cmd.Flags().BoolVar(&asJSON, "json", false, "Output in JSON format")
	return cmd
}
