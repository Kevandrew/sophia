package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newHookCmd() *cobra.Command {
	hookCmd := &cobra.Command{
		Use:     "hook",
		Short:   "Manage Sophia local git hooks",
		Example: "  sophia hook install\n  sophia hook install --force-overwrite",
	}
	hookCmd.AddCommand(newHookInstallCmd())
	return hookCmd
}

func newHookInstallCmd() *cobra.Command {
	var forceOverwrite bool
	var asJSON bool

	cmd := &cobra.Command{
		Use:     "install",
		Short:   "Install Sophia pre-commit guard hook",
		Example: "  sophia hook install\n  sophia hook install --force-overwrite\n  sophia hook install --json",
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, err := newService()
			if err != nil {
				if asJSON {
					return writeJSONError(cmd, err)
				}
				return err
			}
			hookPath, err := svc.InstallHook(forceOverwrite)
			if err != nil {
				if asJSON {
					return writeJSONError(cmd, err)
				}
				return err
			}
			if asJSON {
				return writeJSONSuccess(cmd, map[string]any{
					"hook_path":       hookPath,
					"force_overwrite": forceOverwrite,
					"bypass_hint":     "git commit --no-verify",
				})
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Installed Sophia pre-commit hook: %s\n", hookPath)
			fmt.Fprintln(cmd.OutOrStdout(), "Bypass intentionally with: git commit --no-verify")
			return nil
		},
	}

	cmd.Flags().BoolVar(&forceOverwrite, "force-overwrite", false, "Overwrite an existing non-Sophia pre-commit hook")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Output in JSON format")
	return cmd
}
