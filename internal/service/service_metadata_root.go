package service

import (
	"os"
	"path/filepath"
	"strings"

	"sophia/internal/gitx"
	"sophia/internal/store"
)

func (s *Service) bootstrapRepoContext(fallbackRoot string) {
	repoRoot := fallbackRoot
	gitClient := gitx.New(fallbackRoot)

	if gitClient.InRepo() {
		if resolved, err := gitClient.RepoRoot(); err == nil && strings.TrimSpace(resolved) != "" {
			repoRoot = strings.TrimSpace(resolved)
			gitClient = gitx.New(repoRoot)
		}
	}

	s.git = gitClient
	s.store = store.NewWithSophiaRoot(repoRoot, resolveSophiaRootForStartup(repoRoot, gitClient))
}

func resolveSophiaRootForStartup(repoRoot string, gitClient *gitx.Client) string {
	legacy := filepath.Join(repoRoot, ".sophia")
	if isSophiaInitializedAt(legacy) {
		return legacy
	}
	if gitClient == nil || !gitClient.InRepo() {
		return legacy
	}
	commonDir, err := gitClient.GitCommonDirAbs()
	if err != nil || strings.TrimSpace(commonDir) == "" {
		return legacy
	}
	shared := filepath.Join(commonDir, "sophia-local")
	if isSophiaInitializedAt(shared) {
		return shared
	}
	return legacy
}

func isSophiaInitializedAt(path string) bool {
	_, err := os.Stat(filepath.Join(path, "config.yaml"))
	return err == nil
}
