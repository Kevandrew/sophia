package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
)

const (
	defaultSophiaRepo      = "Kevandrew/sophia"
	defaultInstallScript   = "https://sophiahq.com/install.sh"
	defaultGitHubAPIBase   = "https://api.github.com"
)

type releaseInfo struct {
	TagName string `json:"tag_name"`
}

type updateRunResult struct {
	CurrentVersion string
	LatestVersion  string
	TargetVersion  string
	Repo           string
	InstallDir     string
	CheckOnly      bool
	UpdateAvailable bool
	Updated        bool
}

var (
	httpClient               = &http.Client{}
	fetchLatestReleaseTagFn  = fetchLatestReleaseTag
	downloadInstallScriptFn  = downloadInstallScript
	runInstallScriptFn       = runInstallScript
)

func newUpdateCmd() *cobra.Command {
	var asJSON bool
	var checkOnly bool
	var repo string
	var version string
	var installDir string
	var yes bool

	cmd := &cobra.Command{
		Use:     "update",
		Short:   "Check for and install newer Sophia releases",
		Example: "  sophia update --check\n  sophia update\n  sophia update --version v1.0.3 --yes\n  sophia update --json",
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := runUpdateFlow(cmd.Context(), updateOptions{
				CheckOnly:  checkOnly,
				Repo:       repo,
				Version:    version,
				InstallDir: installDir,
				Yes:        yes,
			})
			if err != nil {
				return commandError(cmd, asJSON, err)
			}
			if asJSON {
				return writeJSONSuccess(cmd, map[string]any{
					"current_version": result.CurrentVersion,
					"latest_version":  result.LatestVersion,
					"target_version":  result.TargetVersion,
					"repo":            result.Repo,
					"install_dir":     result.InstallDir,
					"check_only":      result.CheckOnly,
					"update_available": result.UpdateAvailable,
					"updated":         result.Updated,
				})
			}
			if result.CheckOnly {
				if result.UpdateAvailable {
					fmt.Fprintf(cmd.OutOrStdout(), "Update available: current=%s latest=%s\n", nonEmpty(result.CurrentVersion, "unknown"), result.LatestVersion)
				} else {
					fmt.Fprintf(cmd.OutOrStdout(), "Sophia is up to date: %s\n", nonEmpty(result.CurrentVersion, "unknown"))
				}
				return nil
			}
			if result.Updated {
				fmt.Fprintf(cmd.OutOrStdout(), "Sophia updated to %s\n", result.TargetVersion)
				return nil
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Sophia is already at %s\n", nonEmpty(result.CurrentVersion, "unknown"))
			return nil
		},
	}

	cmd.Flags().BoolVar(&checkOnly, "check", false, "Only check for updates without installing")
	cmd.Flags().StringVar(&repo, "repo", "", "GitHub repository in owner/name form (default: Kevandrew/sophia)")
	cmd.Flags().StringVar(&version, "version", "", "Target version/tag to install (default: latest release)")
	cmd.Flags().StringVar(&installDir, "install-dir", "", "Install directory override (forwarded as SOPHIA_INSTALL_DIR)")
	cmd.Flags().BoolVar(&yes, "yes", false, "Apply update without confirmation prompt")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Output in JSON format")
	return cmd
}

type updateOptions struct {
	CheckOnly  bool
	Repo       string
	Version    string
	InstallDir string
	Yes        bool
}

func runUpdateFlow(ctx context.Context, opts updateOptions) (updateRunResult, error) {
	repo := strings.TrimSpace(opts.Repo)
	if repo == "" {
		repo = defaultSophiaRepo
	}
	current := normalizeTag(buildVersion)
	latest, err := fetchLatestReleaseTagFn(ctx, repo)
	if err != nil {
		return updateRunResult{}, err
	}
	target := normalizeTag(opts.Version)
	if target == "" {
		target = latest
	}

	result := updateRunResult{
		CurrentVersion: current,
		LatestVersion:  latest,
		TargetVersion:  target,
		Repo:           repo,
		InstallDir:     strings.TrimSpace(opts.InstallDir),
		CheckOnly:      opts.CheckOnly,
		UpdateAvailable: !versionMatches(current, latest),
	}
	if opts.CheckOnly {
		return result, nil
	}
	if versionMatches(current, target) {
		return result, nil
	}
	if !opts.Yes {
		return updateRunResult{}, fmt.Errorf("--yes is required to apply updates")
	}

	script, err := downloadInstallScriptFn(ctx, defaultInstallScript)
	if err != nil {
		return updateRunResult{}, err
	}
	if err := runInstallScriptFn(ctx, script, target, repo, opts.InstallDir); err != nil {
		return updateRunResult{}, err
	}
	result.Updated = true
	return result, nil
}

func fetchLatestReleaseTag(ctx context.Context, repo string) (string, error) {
	url := fmt.Sprintf("%s/repos/%s/releases/latest", defaultGitHubAPIBase, strings.TrimSpace(repo))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("build latest release request: %w", err)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("resolve latest release tag: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return "", fmt.Errorf("resolve latest release tag: http %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var info releaseInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return "", fmt.Errorf("decode latest release response: %w", err)
	}
	tag := normalizeTag(info.TagName)
	if tag == "" {
		return "", fmt.Errorf("resolve latest release tag: missing tag_name")
	}
	return tag, nil
}

func downloadInstallScript(ctx context.Context, url string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimSpace(url), nil)
	if err != nil {
		return "", fmt.Errorf("build install script request: %w", err)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("download install script: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return "", fmt.Errorf("download install script: http %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read install script: %w", err)
	}
	if strings.TrimSpace(string(body)) == "" {
		return "", fmt.Errorf("download install script: empty body")
	}
	return string(body), nil
}

func runInstallScript(ctx context.Context, scriptBody, version, repo, installDir string) error {
	cmd := exec.CommandContext(ctx, "bash")
	cmd.Stdin = strings.NewReader(scriptBody)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	env := append([]string{}, os.Environ()...)
	if trimmed := strings.TrimSpace(version); trimmed != "" {
		env = append(env, "SOPHIA_VERSION="+trimmed)
	}
	if trimmed := strings.TrimSpace(repo); trimmed != "" {
		env = append(env, "SOPHIA_REPO="+trimmed)
	}
	if trimmed := strings.TrimSpace(installDir); trimmed != "" {
		env = append(env, "SOPHIA_INSTALL_DIR="+trimmed)
	}
	cmd.Env = env
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("run install script: %w", err)
	}
	return nil
}

func normalizeTag(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	if !strings.HasPrefix(trimmed, "v") {
		return "v" + trimmed
	}
	return trimmed
}

func versionMatches(a, b string) bool {
	if strings.TrimSpace(a) == "" || strings.TrimSpace(b) == "" {
		return false
	}
	return normalizeTag(a) == normalizeTag(b)
}
