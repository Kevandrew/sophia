package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"sophia/internal/service"
	"sophia/internal/store"
)

type serviceContextKey string

const (
	serviceRepoRootContextKey serviceContextKey = "service_repo_root"
	serviceFactoryContextKey  serviceContextKey = "service_factory"
)

type serviceFactory func(repoRoot string) *service.Service

func Execute() error {
	root := newRootCmd()
	return executeRootCmd(root, os.Args[1:])
}

func executeRootCmd(root *cobra.Command, args []string) error {
	if root == nil {
		return nil
	}
	root.SetArgs(args)
	if err := root.Execute(); err != nil {
		if IsHandledError(err) {
			return err
		}
		if handled := maybeHandleArgumentUsageError(root, err, args); handled != nil {
			return handled
		}
		return err
	}
	return nil
}

func newRootCmd() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:           "sophia",
		Short:         "Sophia CLI: intent-first workflow over Git",
		Long:          "Sophia is an intent-first workflow over Git.\n\nStart Here:\n  1. sophia init\n  2. sophia cr add \"<title>\" --description \"<why>\"\n  3. sophia cr switch <cr-id>\n  4. sophia cr task add <cr-id> \"<task>\"\n  5. sophia cr task done <cr-id> <task-id> --from-contract\n  6. sophia cr validate <cr-id>\n  7. sophia cr review <cr-id>\n  8. sophia cr merge <cr-id>\n\nFor command discovery, use help top-down:\n  sophia --help\n  sophia cr --help\n  sophia cr <command> --help",
		Example:       "  sophia init\n  sophia cr add \"Add billing retries\" --description \"Reduce transient failure loops\"\n  sophia cr switch 12\n  sophia cr contract set 12 --why \"Retry policy drift\" --scope internal/service\n  sophia cr task add 12 \"Add jittered backoff\"\n  sophia cr task done 12 1 --from-contract\n  sophia cr review 12\n  sophia cr merge 12",
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	rootCmd.AddCommand(newInitCmd())
	rootCmd.AddCommand(newVersionCmd())
	rootCmd.AddCommand(newBlameCmd())
	rootCmd.AddCommand(newCRCmd())
	rootCmd.AddCommand(newDoctorCmd())
	rootCmd.AddCommand(newLogCmd())
	rootCmd.AddCommand(newRepairCmd())
	rootCmd.AddCommand(newHookCmd())
	rootCmd.AddCommand(newHQCmd())

	return rootCmd
}

func newInitCmd() *cobra.Command {
	var baseBranch string
	var metadataMode string
	var branchOwnerPrefix string
	var asJSON bool

	cmd := &cobra.Command{
		Use:     "init",
		Short:   "Initialize Sophia metadata in the current repository",
		Example: "  sophia init\n  sophia init --base-branch main\n  sophia init --metadata-mode tracked\n  sophia init --json",
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, err := newServiceForCmd(cmd)
			if err != nil {
				if asJSON {
					return writeJSONError(cmd, err)
				}
				return err
			}
			base, err := svc.InitWithOptions(service.InitOptions{
				BaseBranch:        baseBranch,
				MetadataMode:      metadataMode,
				BranchOwnerPrefix: branchOwnerPrefix,
			})
			if err != nil {
				if asJSON {
					return writeJSONError(cmd, err)
				}
				return err
			}
			currentPrefix := ""
			if cfg, cfgErr := svc.Config(); cfgErr == nil {
				currentPrefix = cfg.BranchOwnerPrefix
			}
			if asJSON {
				return writeJSONSuccess(cmd, map[string]any{
					"base_branch":         base,
					"branch_owner_prefix": currentPrefix,
				})
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Initialized Sophia (base branch: %s)\n", base)
			return nil
		},
	}

	cmd.Flags().StringVar(&baseBranch, "base-branch", "", "Base branch to use for CR merges")
	cmd.Flags().StringVar(&metadataMode, "metadata-mode", "", "Metadata mode: local or tracked (default: local)")
	cmd.Flags().StringVar(&branchOwnerPrefix, "branch-owner-prefix", "", "Default owner prefix for generated branch aliases")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Output in JSON format")
	return cmd
}

func withServiceRepoRootContext(ctx context.Context, repoRoot string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, serviceRepoRootContextKey, strings.TrimSpace(repoRoot))
}

func withServiceFactoryContext(ctx context.Context, factory serviceFactory) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, serviceFactoryContextKey, factory)
}

func newServiceForCmd(cmd *cobra.Command) (*service.Service, error) {
	if cmd != nil {
		if repoRoot, ok := repoRootFromCommandContext(cmd); ok && strings.TrimSpace(repoRoot) != "" {
			if factory, hasFactory := serviceFactoryFromCommandContext(cmd); hasFactory {
				return factory(strings.TrimSpace(repoRoot)), nil
			}
			return service.New(strings.TrimSpace(repoRoot)), nil
		}
	}
	return newService()
}

func newService() (*service.Service, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("resolve working directory: %w", err)
	}
	return service.New(cwd), nil
}

func repoRootFromCommandContext(cmd *cobra.Command) (string, bool) {
	if cmd == nil {
		return "", false
	}
	ctx := cmd.Context()
	if ctx == nil {
		return "", false
	}
	value := ctx.Value(serviceRepoRootContextKey)
	if value == nil {
		return "", false
	}
	root, ok := value.(string)
	return root, ok
}

func serviceFactoryFromCommandContext(cmd *cobra.Command) (serviceFactory, bool) {
	if cmd == nil {
		return nil, false
	}
	ctx := cmd.Context()
	if ctx == nil {
		return nil, false
	}
	value := ctx.Value(serviceFactoryContextKey)
	if value == nil {
		return nil, false
	}
	factory, ok := value.(serviceFactory)
	return factory, ok && factory != nil
}

func resolvePathForCmd(cmd *cobra.Command, rawPath string) string {
	trimmed := strings.TrimSpace(rawPath)
	if trimmed == "" || filepath.IsAbs(trimmed) {
		return trimmed
	}
	root, ok := repoRootFromCommandContext(cmd)
	if !ok || strings.TrimSpace(root) == "" {
		return trimmed
	}
	return filepath.Join(strings.TrimSpace(root), trimmed)
}

type usageArgumentError struct {
	cause       error
	commandPath string
	useLine     string
	example     string
}

func (e *usageArgumentError) Error() string {
	if e == nil || e.cause == nil {
		return ""
	}
	return e.cause.Error()
}

func (e *usageArgumentError) Unwrap() error {
	if e == nil {
		return nil
	}
	return store.InvalidArgumentError{
		Argument: "arguments",
		Message:  e.Error(),
	}
}

func (e *usageArgumentError) Details() map[string]any {
	if e == nil {
		return nil
	}
	suggestedAction := strings.TrimSpace(e.commandPath)
	if suggestedAction != "" {
		suggestedAction += " --help"
	} else {
		suggestedAction = "sophia --help"
	}
	details := map[string]any{
		"suggested_action": suggestedAction,
	}
	if strings.TrimSpace(e.useLine) != "" {
		details["usage"] = strings.TrimSpace(e.useLine)
	}
	if strings.TrimSpace(e.example) != "" {
		details["example"] = strings.TrimSpace(e.example)
	}
	return details
}

func maybeHandleArgumentUsageError(root *cobra.Command, err error, argv []string) error {
	if !isArgumentArityError(err) {
		return nil
	}
	cmd := resolveCommandForArgError(root, argv)
	if cmd == nil {
		cmd = root
	}
	usageErr := buildUsageArgumentError(cmd, err)
	if argsContainJSONFlag(argv) {
		return writeJSONError(cmd, usageErr)
	}
	renderActionableUsageError(cmd, err, usageErr)
	return markHandled(err)
}

func isArgumentArityError(err error) bool {
	if err == nil {
		return false
	}
	lower := strings.ToLower(strings.TrimSpace(err.Error()))
	if lower == "" {
		return false
	}
	return strings.Contains(lower, "arg(s)") &&
		(strings.Contains(lower, "accepts") || strings.Contains(lower, "requires"))
}

func argsContainJSONFlag(argv []string) bool {
	for _, arg := range argv {
		trimmed := strings.TrimSpace(arg)
		if trimmed == "--json" {
			return true
		}
		if !strings.HasPrefix(trimmed, "--json=") {
			continue
		}
		raw := strings.TrimSpace(strings.TrimPrefix(trimmed, "--json="))
		if enabled, err := strconv.ParseBool(raw); err == nil && enabled {
			return true
		}
	}
	return false
}

func resolveCommandForArgError(root *cobra.Command, argv []string) *cobra.Command {
	if root == nil {
		return nil
	}
	if len(argv) == 0 {
		return root
	}
	if cmd, _, err := root.Find(argv); err == nil && cmd != nil {
		return cmd
	}
	for i := len(argv) - 1; i > 0; i-- {
		if strings.HasPrefix(strings.TrimSpace(argv[i]), "-") {
			continue
		}
		if cmd, _, err := root.Find(argv[:i]); err == nil && cmd != nil {
			return cmd
		}
	}
	return root
}

func buildUsageArgumentError(cmd *cobra.Command, cause error) *usageArgumentError {
	commandPath := "sophia"
	useLine := "sophia [command]"
	example := "sophia --help"
	if cmd != nil {
		if path := strings.TrimSpace(cmd.CommandPath()); path != "" {
			commandPath = path
		}
		if line := strings.TrimSpace(cmd.UseLine()); line != "" {
			useLine = line
		}
		if ex := firstCommandExample(cmd); ex != "" {
			example = ex
		} else {
			example = commandPath + " --help"
		}
	}
	return &usageArgumentError{
		cause:       cause,
		commandPath: commandPath,
		useLine:     useLine,
		example:     example,
	}
}

func firstCommandExample(cmd *cobra.Command) string {
	if cmd == nil {
		return ""
	}
	raw := strings.TrimSpace(cmd.Example)
	if raw == "" {
		return ""
	}
	for _, line := range strings.Split(raw, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		return trimmed
	}
	return ""
}

func renderActionableUsageError(cmd *cobra.Command, cause error, usageErr *usageArgumentError) {
	if cmd == nil {
		return
	}
	errOut := cmd.ErrOrStderr()
	fmt.Fprintf(errOut, "Error: %s\n", strings.TrimSpace(cause.Error()))
	if usageErr == nil {
		return
	}
	fmt.Fprintf(errOut, "\nUsage:\n  %s\n", strings.TrimSpace(usageErr.useLine))
	fmt.Fprintf(errOut, "\nExample:\n  %s\n", strings.TrimSpace(usageErr.example))
	fmt.Fprintf(errOut, "\nNext step:\n  %s --help\n", strings.TrimSpace(usageErr.commandPath))
}
