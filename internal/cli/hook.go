package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newHookCmd() *cobra.Command {
	hookCmd := &cobra.Command{
		Use:   "hook",
		Short: "Manage Sophia local git hooks",
	}
	hookCmd.AddCommand(newHookInstallCmd())
	return hookCmd
}

func newHookInstallCmd() *cobra.Command {
	var forceOverwrite bool

	cmd := &cobra.Command{
		Use:   "install",
		Short: "Install Sophia pre-commit guard hook",
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, err := newService()
			if err != nil {
				return err
			}
			hookPath, err := svc.InstallHook(forceOverwrite)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Installed Sophia pre-commit hook: %s\n", hookPath)
			fmt.Fprintln(cmd.OutOrStdout(), "Bypass intentionally with: git commit --no-verify")
			return nil
		},
	}

	cmd.Flags().BoolVar(&forceOverwrite, "force-overwrite", false, "Overwrite an existing non-Sophia pre-commit hook")
	return cmd
}
