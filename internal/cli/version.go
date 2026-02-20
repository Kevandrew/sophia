package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

var (
	buildVersion = "dev"
	buildCommit  = "unknown"
	buildDate    = "unknown"
)

func SetBuildInfo(version, commit, date string) {
	buildVersion = normalizeBuildValue(version, "dev")
	buildCommit = normalizeBuildValue(commit, "unknown")
	buildDate = normalizeBuildValue(date, "unknown")
}

func newVersionCmd() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:     "version",
		Short:   "Print Sophia version metadata",
		Example: "  sophia version\n  sophia version --json",
		RunE: func(cmd *cobra.Command, args []string) error {
			payload := map[string]string{
				"version":    buildVersion,
				"commit":     buildCommit,
				"build_date": buildDate,
			}
			if asJSON {
				return writeJSONSuccess(cmd, payload)
			}
			fmt.Fprintf(
				cmd.OutOrStdout(),
				"version: %s\ncommit: %s\nbuild_date: %s\n",
				buildVersion,
				buildCommit,
				buildDate,
			)
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "Output in JSON format")
	return cmd
}

func normalizeBuildValue(value, fallback string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return fallback
	}
	return trimmed
}
