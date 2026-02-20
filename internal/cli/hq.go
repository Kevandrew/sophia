package cli

import "github.com/spf13/cobra"

func newHQCmd() *cobra.Command {
	hqCmd := &cobra.Command{
		Use:   "hq",
		Short: "Manage SophiaHQ remote integration",
	}
	hqCmd.AddCommand(newHQConfigCmd())
	hqCmd.AddCommand(newHQLoginCmd())
	hqCmd.AddCommand(newHQLogoutCmd())
	hqCmd.AddCommand(newHQCRCmd())
	return hqCmd
}
