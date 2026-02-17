package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"sophia/internal/service"
)

func Execute() error {
	return newRootCmd().Execute()
}

func newRootCmd() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:           "sophia",
		Short:         "Sophia CLI: intent-first workflow over Git",
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	rootCmd.AddCommand(newInitCmd())
	rootCmd.AddCommand(newCRCmd())

	return rootCmd
}

func newInitCmd() *cobra.Command {
	var baseBranch string

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize Sophia metadata in the current repository",
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, err := newService()
			if err != nil {
				return err
			}
			base, err := svc.Init(baseBranch)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Initialized Sophia (base branch: %s)\n", base)
			return nil
		},
	}

	cmd.Flags().StringVar(&baseBranch, "base-branch", "", "Base branch to use for CR merges")
	return cmd
}

func newService() (*service.Service, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("resolve working directory: %w", err)
	}
	return service.New(cwd), nil
}
