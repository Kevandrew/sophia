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
		Long:          "Sophia is an intent-first workflow over Git.\n\nStart Here:\n  1. sophia init\n  2. sophia cr add \"<title>\" --description \"<why>\"\n  3. sophia cr task add <cr-id> \"<task>\"\n  4. sophia cr task done <cr-id> <task-id> --from-contract\n  5. sophia cr validate <cr-id>\n  6. sophia cr review <cr-id>\n  7. sophia cr merge <cr-id>\n\nFor command discovery, use help top-down:\n  sophia --help\n  sophia cr --help\n  sophia cr <command> --help",
		Example:       "  sophia init\n  sophia cr add \"Add billing retries\" --description \"Reduce transient failure loops\"\n  sophia cr contract set 12 --why \"Retry policy drift\" --scope internal/service\n  sophia cr task add 12 \"Add jittered backoff\"\n  sophia cr task done 12 1 --from-contract\n  sophia cr review 12\n  sophia cr merge 12",
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	rootCmd.AddCommand(newInitCmd())
	rootCmd.AddCommand(newBlameCmd())
	rootCmd.AddCommand(newCRCmd())
	rootCmd.AddCommand(newDoctorCmd())
	rootCmd.AddCommand(newLogCmd())
	rootCmd.AddCommand(newRepairCmd())
	rootCmd.AddCommand(newHookCmd())

	return rootCmd
}

func newInitCmd() *cobra.Command {
	var baseBranch string
	var metadataMode string

	cmd := &cobra.Command{
		Use:     "init",
		Short:   "Initialize Sophia metadata in the current repository",
		Example: "  sophia init\n  sophia init --base-branch main\n  sophia init --metadata-mode tracked",
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, err := newService()
			if err != nil {
				return err
			}
			base, err := svc.Init(baseBranch, metadataMode)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Initialized Sophia (base branch: %s)\n", base)
			return nil
		},
	}

	cmd.Flags().StringVar(&baseBranch, "base-branch", "", "Base branch to use for CR merges")
	cmd.Flags().StringVar(&metadataMode, "metadata-mode", "", "Metadata mode: local or tracked (default: local)")
	return cmd
}

func newService() (*service.Service, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("resolve working directory: %w", err)
	}
	return service.New(cwd), nil
}
