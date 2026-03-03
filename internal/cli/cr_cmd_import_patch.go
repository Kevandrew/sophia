package cli

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"sophia/internal/service"
)

func newCRImportCmd() *cobra.Command {
	var filePath string
	var mode string
	var asJSON bool

	cmd := &cobra.Command{
		Use:   "import",
		Short: "Import a CR bundle artifact into local Sophia metadata",
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, err := newServiceForCmd(cmd)
			if err != nil {
				return commandError(cmd, asJSON, err)
			}
			bundlePath := resolvePathForCmd(cmd, filePath)
			result, err := svc.ImportCRBundle(service.ImportCRBundleOptions{
				FilePath: bundlePath,
				Mode:     mode,
			})
			if err != nil {
				return commandError(cmd, asJSON, err)
			}
			if asJSON {
				return writeJSONSuccess(cmd, map[string]any{
					"local_cr_id":    result.LocalCRID,
					"cr_uid":         result.CRUID,
					"cr_fingerprint": result.CRFingerprint,
					"created":        result.Created,
					"replaced":       result.Replaced,
				})
			}
			action := "created"
			if result.Replaced {
				action = "replaced"
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Imported CR %d (%s) from bundle (%s)\n", result.LocalCRID, result.CRUID, action)
			return nil
		},
	}

	cmd.Flags().StringVar(&filePath, "file", "", "Path to exported CR bundle JSON file")
	cmd.Flags().StringVar(&mode, "mode", "create", "Import mode: create or replace")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Output in JSON format")
	return cmd
}

func newCRPatchCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "patch",
		Short: "Apply or preview collaboration patches against a CR",
	}
	cmd.AddCommand(newCRPatchApplyCmd(false))
	cmd.AddCommand(newCRPatchApplyCmd(true))
	return cmd
}

func newCRPatchApplyCmd(preview bool) *cobra.Command {
	var filePath string
	var force bool
	var asJSON bool

	use := "apply <id>"
	short := "Apply a collaboration patch file to a CR"
	if preview {
		use = "preview <id>"
		short = "Preview patch application against a CR without writes"
	}
	cmd := &cobra.Command{
		Use:   use,
		Short: short,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			selector := strings.TrimSpace(args[0])
			if selector == "" {
				return commandError(cmd, asJSON, fmt.Errorf("cr selector is required"))
			}
			filePath = strings.TrimSpace(filePath)
			if filePath == "" {
				return commandError(cmd, asJSON, fmt.Errorf("--file is required"))
			}
			filePath = resolvePathForCmd(cmd, filePath)
			payload, readErr := os.ReadFile(filePath)
			if readErr != nil {
				return commandError(cmd, asJSON, fmt.Errorf("read patch file %q: %w", filePath, readErr))
			}
			svc, err := newServiceForCmd(cmd)
			if err != nil {
				return commandError(cmd, asJSON, err)
			}
			var result *service.CRPatchApplyResult
			if preview {
				result, err = svc.PreviewCRPatch(selector, payload, force)
			} else {
				result, err = svc.ApplyCRPatch(selector, payload, force, false)
			}
			if err != nil {
				if !asJSON && result != nil {
					printCRPatchApplyResult(cmd, result, preview)
					if len(result.Conflicts) > 0 {
						var conflictErr *service.PatchConflictError
						if errors.As(err, &conflictErr) {
							fmt.Fprintln(cmd.OutOrStdout(), "\nConflicts:")
							for _, conflict := range result.Conflicts {
								fmt.Fprintf(cmd.OutOrStdout(), "- op #%d [%s] %s: %s\n", conflict.OpIndex, conflict.Op, conflict.Field, conflict.Message)
							}
						}
					}
				}
				if asJSON && result != nil {
					return writeJSONError(cmd, err)
				}
				return commandError(cmd, asJSON, err)
			}
			if asJSON {
				return writeJSONSuccess(cmd, crPatchApplyResultToJSON(result))
			}
			printCRPatchApplyResult(cmd, result, preview)
			return nil
		},
	}

	cmd.Flags().StringVar(&filePath, "file", "", "Path to patch JSON file")
	cmd.Flags().BoolVar(&force, "force", false, "Force apply even when before values mismatch")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Output in JSON format")
	return cmd
}

func printCRPatchApplyResult(cmd *cobra.Command, result *service.CRPatchApplyResult, preview bool) {
	if result == nil {
		return
	}
	verb := "Applied"
	if preview {
		verb = "Previewed"
	}
	fmt.Fprintf(cmd.OutOrStdout(), "%s patch for CR %d (%s)\n", verb, result.CRID, result.CRUID)
	fmt.Fprintf(cmd.OutOrStdout(), "Applied ops: %d\n", len(result.AppliedOps))
	fmt.Fprintf(cmd.OutOrStdout(), "Skipped ops: %d\n", len(result.SkippedOps))
	fmt.Fprintf(cmd.OutOrStdout(), "Conflicts: %d\n", len(result.Conflicts))
	if len(result.Warnings) > 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "\nWarnings:")
		for _, warning := range result.Warnings {
			fmt.Fprintf(cmd.OutOrStdout(), "- %s\n", warning)
		}
	}
}
