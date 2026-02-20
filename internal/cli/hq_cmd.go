package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"sophia/internal/model"
	"sophia/internal/service"
)

func newHQConfigCmd() *cobra.Command {
	configCmd := &cobra.Command{
		Use:   "config",
		Short: "Show or set HQ remote configuration",
	}
	configCmd.AddCommand(newHQConfigShowCmd())
	configCmd.AddCommand(newHQConfigSetCmd())
	return configCmd
}

func newHQConfigShowCmd() *cobra.Command {
	var asJSON bool

	cmd := &cobra.Command{
		Use:   "show",
		Short: "Show resolved HQ config (repo overrides + global defaults)",
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, err := newService()
			if err != nil {
				return commandError(cmd, asJSON, err)
			}
			cfg, err := svc.GetHQConfig()
			if err != nil {
				return commandError(cmd, asJSON, err)
			}
			if asJSON {
				return writeJSONSuccess(cmd, map[string]any{
					"remote_alias":  cfg.RemoteAlias,
					"repo_id":       cfg.RepoID,
					"base_url":      cfg.BaseURL,
					"metadata_mode": cfg.MetadataMode,
					"token_present": cfg.TokenPresent,
					"repo_config": map[string]any{
						"remote_alias": cfg.RepoConfig.RemoteAlias,
						"repo_id":      cfg.RepoConfig.RepoID,
						"base_url":     cfg.RepoConfig.BaseURL,
					},
					"global_config": map[string]any{
						"remote_alias": cfg.GlobalConfig.RemoteAlias,
						"repo_id":      cfg.GlobalConfig.RepoID,
						"base_url":     cfg.GlobalConfig.BaseURL,
					},
				})
			}
			fmt.Fprintln(cmd.OutOrStdout(), "HQ Config:")
			fmt.Fprintf(cmd.OutOrStdout(), "- remote_alias: %s\n", nonEmpty(cfg.RemoteAlias, "-"))
			fmt.Fprintf(cmd.OutOrStdout(), "- repo_id: %s\n", nonEmpty(cfg.RepoID, "(unset)"))
			fmt.Fprintf(cmd.OutOrStdout(), "- base_url: %s\n", nonEmpty(cfg.BaseURL, "-"))
			fmt.Fprintf(cmd.OutOrStdout(), "- metadata_mode: %s\n", nonEmpty(cfg.MetadataMode, "-"))
			fmt.Fprintf(cmd.OutOrStdout(), "- token_present: %t\n", cfg.TokenPresent)
			fmt.Fprintln(cmd.OutOrStdout(), "- repo_config:")
			fmt.Fprintf(cmd.OutOrStdout(), "  - remote_alias: %s\n", nonEmpty(cfg.RepoConfig.RemoteAlias, "(unset)"))
			fmt.Fprintf(cmd.OutOrStdout(), "  - repo_id: %s\n", nonEmpty(cfg.RepoConfig.RepoID, "(unset)"))
			fmt.Fprintf(cmd.OutOrStdout(), "  - base_url: %s\n", nonEmpty(cfg.RepoConfig.BaseURL, "(unset)"))
			fmt.Fprintln(cmd.OutOrStdout(), "- global_config:")
			fmt.Fprintf(cmd.OutOrStdout(), "  - remote_alias: %s\n", nonEmpty(cfg.GlobalConfig.RemoteAlias, "(unset)"))
			fmt.Fprintf(cmd.OutOrStdout(), "  - repo_id: %s\n", nonEmpty(cfg.GlobalConfig.RepoID, "(unset)"))
			fmt.Fprintf(cmd.OutOrStdout(), "  - base_url: %s\n", nonEmpty(cfg.GlobalConfig.BaseURL, "(unset)"))
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "Output in JSON format")
	return cmd
}

func newHQConfigSetCmd() *cobra.Command {
	var remoteAlias string
	var repoID string
	var baseURL string
	var global bool
	var asJSON bool

	cmd := &cobra.Command{
		Use:   "set",
		Short: "Set HQ config values",
		RunE: func(cmd *cobra.Command, args []string) error {
			opts := service.HQConfigSetOptions{
				Global: global,
			}
			if cmd.Flags().Changed("remote") {
				v := remoteAlias
				opts.RemoteAlias = &v
			}
			if cmd.Flags().Changed("repo-id") {
				v := repoID
				opts.RepoID = &v
			}
			if cmd.Flags().Changed("base-url") {
				v := baseURL
				opts.BaseURL = &v
			}
			if opts.RemoteAlias == nil && opts.RepoID == nil && opts.BaseURL == nil {
				return commandError(cmd, asJSON, fmt.Errorf("provide at least one of --remote, --repo-id, or --base-url"))
			}
			svc, err := newService()
			if err != nil {
				return commandError(cmd, asJSON, err)
			}
			changed, err := svc.SetHQConfig(opts)
			if err != nil {
				return commandError(cmd, asJSON, err)
			}
			target := "repo"
			if global {
				target = "global"
			}
			if asJSON {
				return writeJSONSuccess(cmd, map[string]any{
					"scope":          target,
					"changed_fields": stringSliceOrEmpty(changed),
				})
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Updated HQ %s config fields: %s\n", target, strings.Join(changed, ", "))
			return nil
		},
	}
	cmd.Flags().StringVar(&remoteAlias, "remote", "", "HQ remote alias")
	cmd.Flags().StringVar(&repoID, "repo-id", "", "HQ repo identity")
	cmd.Flags().StringVar(&baseURL, "base-url", "", "HQ base URL")
	cmd.Flags().BoolVar(&global, "global", false, "Write per-user global defaults instead of repo-local config")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Output in JSON format")
	return cmd
}

func newHQLoginCmd() *cobra.Command {
	var remoteAlias string
	var token string
	var tokenStdin bool
	var asJSON bool

	cmd := &cobra.Command{
		Use:   "login",
		Short: "Store an HQ token for the selected remote",
		RunE: func(cmd *cobra.Command, args []string) error {
			if tokenStdin && strings.TrimSpace(token) != "" {
				return commandError(cmd, asJSON, fmt.Errorf("use either --token or --token-stdin"))
			}
			if tokenStdin {
				raw, err := io.ReadAll(cmd.InOrStdin())
				if err != nil {
					return commandError(cmd, asJSON, err)
				}
				token = strings.TrimSpace(string(raw))
			}
			if strings.TrimSpace(token) == "" {
				return commandError(cmd, asJSON, fmt.Errorf("provide --token or --token-stdin"))
			}
			svc, err := newService()
			if err != nil {
				return commandError(cmd, asJSON, err)
			}
			alias, err := svc.HQLogin(remoteAlias, token)
			if err != nil {
				return commandError(cmd, asJSON, err)
			}
			if asJSON {
				return writeJSONSuccess(cmd, map[string]any{
					"remote_alias": alias,
				})
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Stored HQ token for remote %s\n", alias)
			return nil
		},
	}
	cmd.Flags().StringVar(&remoteAlias, "remote", "", "HQ remote alias (defaults to resolved HQ remote)")
	cmd.Flags().StringVar(&token, "token", "", "HQ personal access token")
	cmd.Flags().BoolVar(&tokenStdin, "token-stdin", false, "Read HQ token from stdin")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Output in JSON format")
	return cmd
}

func newHQLogoutCmd() *cobra.Command {
	var remoteAlias string
	var asJSON bool

	cmd := &cobra.Command{
		Use:   "logout",
		Short: "Remove stored token for an HQ remote",
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, err := newService()
			if err != nil {
				return commandError(cmd, asJSON, err)
			}
			alias, err := svc.HQLogout(remoteAlias)
			if err != nil {
				return commandError(cmd, asJSON, err)
			}
			if asJSON {
				return writeJSONSuccess(cmd, map[string]any{
					"remote_alias": alias,
				})
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Removed HQ token for remote %s\n", alias)
			return nil
		},
	}
	cmd.Flags().StringVar(&remoteAlias, "remote", "", "HQ remote alias (defaults to resolved HQ remote)")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Output in JSON format")
	return cmd
}

func newHQCRCmd() *cobra.Command {
	crCmd := &cobra.Command{
		Use:   "cr",
		Short: "Read and mutate CR discussions/contracts via HQ remote",
	}
	crCmd.AddCommand(newHQCRListCmd())
	crCmd.AddCommand(newHQCRShowCmd())
	crCmd.AddCommand(newHQCRNoteCmd())
	crCmd.AddCommand(newHQCRContractCmd())
	return crCmd
}

func newHQCRListCmd() *cobra.Command {
	var asJSON bool

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List CR summaries from HQ for the configured repo identity",
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, err := newService()
			if err != nil {
				return commandError(cmd, asJSON, err)
			}
			summaries, err := svc.HQListCRs()
			if err != nil {
				return commandError(cmd, asJSON, err)
			}
			if asJSON {
				items := make([]map[string]any, 0, len(summaries))
				for _, summary := range summaries {
					items = append(items, hqSummaryToJSONMap(summary))
				}
				return writeJSONSuccess(cmd, map[string]any{
					"count":     len(items),
					"summaries": items,
				})
			}
			if len(summaries) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No HQ CRs found.")
				return nil
			}
			fmt.Fprintln(cmd.OutOrStdout(), "HQ CRs:")
			for _, summary := range summaries {
				fmt.Fprintf(cmd.OutOrStdout(), "- %s [%s] %s\n", nonEmpty(summary.UID, "-"), nonEmpty(summary.Status, "-"), nonEmpty(summary.Title, "-"))
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "Output in JSON format")
	return cmd
}

func hqSummaryToJSONMap(summary model.HQCRSummary) map[string]any {
	return map[string]any{
		"uid":         summary.UID,
		"title":       summary.Title,
		"status":      summary.Status,
		"branch":      summary.Branch,
		"base_branch": summary.BaseBranch,
		"updated_at":  summary.UpdatedAt,
	}
}

func newHQCRShowCmd() *cobra.Command {
	var asJSON bool

	cmd := &cobra.Command{
		Use:   "show <cr-uid>",
		Short: "Show one remote CR by UID",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			uid := strings.TrimSpace(args[0])
			if uid == "" {
				return commandError(cmd, asJSON, fmt.Errorf("cr uid is required"))
			}
			svc, err := newService()
			if err != nil {
				return commandError(cmd, asJSON, err)
			}
			detail, err := svc.HQGetCR(uid)
			if err != nil {
				return commandError(cmd, asJSON, err)
			}
			if asJSON {
				return writeJSONSuccess(cmd, map[string]any{
					"uid":            detail.UID,
					"cr_fingerprint": detail.Fingerprint,
					"cr":             crToJSONMap(detail.CR),
				})
			}
			fmt.Fprintf(cmd.OutOrStdout(), "HQ CR %s\n", detail.UID)
			fmt.Fprintf(cmd.OutOrStdout(), "Title: %s\n", nonEmpty(detail.CR.Title, "-"))
			fmt.Fprintf(cmd.OutOrStdout(), "Status: %s\n", nonEmpty(detail.CR.Status, "-"))
			fmt.Fprintf(cmd.OutOrStdout(), "Fingerprint: %s\n", nonEmpty(detail.Fingerprint, "-"))
			fmt.Fprintf(cmd.OutOrStdout(), "Notes: %d\n", len(detail.CR.Notes))
			fmt.Fprintf(cmd.OutOrStdout(), "Tasks: %d\n", len(detail.CR.Subtasks))
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "Output in JSON format")
	return cmd
}

func newHQCRNoteCmd() *cobra.Command {
	noteCmd := &cobra.Command{
		Use:   "note",
		Short: "Mutate remote CR notes via patch semantics",
	}
	noteCmd.AddCommand(newHQCRNoteAddCmd())
	return noteCmd
}

func newHQCRNoteAddCmd() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "add <cr-uid> <note>",
		Short: "Append a note to a remote CR",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			uid := strings.TrimSpace(args[0])
			note := strings.TrimSpace(args[1])
			svc, err := newService()
			if err != nil {
				return commandError(cmd, asJSON, err)
			}
			result, err := svc.HQAddCRNote(uid, note)
			if err != nil {
				return commandError(cmd, asJSON, err)
			}
			if asJSON {
				return writeJSONSuccess(cmd, hqPatchResponseToJSONMap(result))
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Remote note appended for %s (applied=%d skipped=%d conflicts=%d)\n", uid, len(result.AppliedOps), len(result.SkippedOps), len(result.Conflicts))
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "Output in JSON format")
	return cmd
}

func newHQCRContractCmd() *cobra.Command {
	contractCmd := &cobra.Command{
		Use:   "contract",
		Short: "Mutate remote CR contract fields via patch semantics",
	}
	contractCmd.AddCommand(newHQCRContractSetCmd())
	return contractCmd
}

func newHQCRContractSetCmd() *cobra.Command {
	var why string
	var scope []string
	var nonGoals []string
	var invariants []string
	var blastRadius string
	var riskCriticalScopes []string
	var riskTierHint string
	var riskRationale string
	var testPlan string
	var rollbackPlan string
	var asJSON bool

	cmd := &cobra.Command{
		Use:   "set <cr-uid>",
		Short: "Set remote CR contract fields through HQ patch mutation",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			uid := strings.TrimSpace(args[0])
			if uid == "" {
				return commandError(cmd, asJSON, fmt.Errorf("cr uid is required"))
			}
			patch := service.ContractPatch{}
			if cmd.Flags().Changed("why") {
				v := why
				patch.Why = &v
			}
			if cmd.Flags().Changed("scope") {
				v := append([]string(nil), scope...)
				patch.Scope = &v
			}
			if cmd.Flags().Changed("non-goal") {
				v := append([]string(nil), nonGoals...)
				patch.NonGoals = &v
			}
			if cmd.Flags().Changed("invariant") {
				v := append([]string(nil), invariants...)
				patch.Invariants = &v
			}
			if cmd.Flags().Changed("blast-radius") {
				v := blastRadius
				patch.BlastRadius = &v
			}
			if cmd.Flags().Changed("risk-critical-scope") {
				v := append([]string(nil), riskCriticalScopes...)
				patch.RiskCriticalScopes = &v
			}
			if cmd.Flags().Changed("risk-tier-hint") {
				v := riskTierHint
				patch.RiskTierHint = &v
			}
			if cmd.Flags().Changed("risk-rationale") {
				v := riskRationale
				patch.RiskRationale = &v
			}
			if cmd.Flags().Changed("test-plan") {
				v := testPlan
				patch.TestPlan = &v
			}
			if cmd.Flags().Changed("rollback-plan") {
				v := rollbackPlan
				patch.RollbackPlan = &v
			}
			if patch.Why == nil && patch.Scope == nil && patch.NonGoals == nil && patch.Invariants == nil && patch.BlastRadius == nil && patch.RiskCriticalScopes == nil && patch.RiskTierHint == nil && patch.RiskRationale == nil && patch.TestPlan == nil && patch.RollbackPlan == nil {
				return commandError(cmd, asJSON, fmt.Errorf("provide at least one contract field flag"))
			}
			svc, err := newService()
			if err != nil {
				return commandError(cmd, asJSON, err)
			}
			result, err := svc.HQSetCRContract(uid, patch)
			if err != nil {
				return commandError(cmd, asJSON, err)
			}
			if asJSON {
				return writeJSONSuccess(cmd, hqPatchResponseToJSONMap(result))
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Remote contract updated for %s (applied=%d skipped=%d conflicts=%d)\n", uid, len(result.AppliedOps), len(result.SkippedOps), len(result.Conflicts))
			return nil
		},
	}

	cmd.Flags().StringVar(&why, "why", "", "Intent rationale")
	cmd.Flags().StringArrayVar(&scope, "scope", nil, "Repo-relative scope prefix (repeatable)")
	cmd.Flags().StringArrayVar(&nonGoals, "non-goal", nil, "Explicit non-goal (repeatable)")
	cmd.Flags().StringArrayVar(&invariants, "invariant", nil, "Invariant that must hold (repeatable)")
	cmd.Flags().StringVar(&blastRadius, "blast-radius", "", "Expected blast radius")
	cmd.Flags().StringArrayVar(&riskCriticalScopes, "risk-critical-scope", nil, "CR-authored critical scope prefix for impact scoring (repeatable)")
	cmd.Flags().StringVar(&riskTierHint, "risk-tier-hint", "", "Optional risk tier hint floor (low, medium, high)")
	cmd.Flags().StringVar(&riskRationale, "risk-rationale", "", "Optional rationale for risk hint choices")
	cmd.Flags().StringVar(&testPlan, "test-plan", "", "Planned validation/testing approach")
	cmd.Flags().StringVar(&rollbackPlan, "rollback-plan", "", "Rollback strategy")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Output in JSON format")
	return cmd
}

func hqPatchResponseToJSONMap(result *model.HQPatchApplyResponse) map[string]any {
	if result == nil {
		return map[string]any{}
	}
	conflicts := make([]map[string]any, 0, len(result.Conflicts))
	for _, conflict := range result.Conflicts {
		conflicts = append(conflicts, map[string]any{
			"op_index": conflict.OpIndex,
			"op":       conflict.Op,
			"field":    conflict.Field,
			"message":  conflict.Message,
		})
	}
	return map[string]any{
		"schema_version": result.SchemaVersion,
		"cr_uid":         result.CRUID,
		"cr_fingerprint": result.CRFingerprint,
		"applied_ops":    append([]int(nil), result.AppliedOps...),
		"skipped_ops":    append([]int(nil), result.SkippedOps...),
		"warnings":       append([]string(nil), result.Warnings...),
		"conflicts":      conflicts,
	}
}
