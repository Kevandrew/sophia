package service

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sophia/internal/gitx"
	"sophia/internal/model"
	"sophia/internal/store"
	"strings"
	"time"
)

func (s *Service) bootstrapRepoContext(fallbackRoot string) {
	repoRoot := fallbackRoot
	gitClient := s.git
	if gitClient == nil {
		gitClient = gitx.New(fallbackRoot)
	}
	if gitClient.InRepo() {
		if resolved, err := gitClient.RepoRoot(); err == nil && strings.TrimSpace(resolved) != "" {
			repoRoot = strings.TrimSpace(resolved)
			gitClient = gitx.New(repoRoot)
		}
	}

	s.repoRoot = repoRoot
	s.git = gitClient
	s.legacySophiaDir = filepath.Join(repoRoot, ".sophia")
	s.sharedLocalSophiaDir = ""
	if gitClient.InRepo() {
		if commonDir, err := gitClient.GitCommonDirAbs(); err == nil {
			s.sharedLocalSophiaDir = filepath.Join(commonDir, "sophia-local")
		}
	}

	sophiaDir := s.resolveSophiaRootForStartup()
	s.store = store.NewWithSophiaRoot(repoRoot, sophiaDir)
}

func (s *Service) resolveSophiaRootForStartup() string {
	legacy := strings.TrimSpace(s.legacySophiaDir)
	shared := strings.TrimSpace(s.sharedLocalSophiaDir)
	if legacy == "" {
		return legacy
	}
	if shared == "" {
		return legacy
	}

	legacyCfg, legacyOK := loadConfigIfInitialized(s.repoRoot, legacy)
	if legacyOK && strings.TrimSpace(legacyCfg.MetadataMode) == model.MetadataModeTracked {
		return legacy
	}

	sharedStore := store.NewWithSophiaRoot(s.repoRoot, shared)
	if sharedStore.IsInitialized() {
		if legacyOK {
			_ = s.reconcileLegacyLocalMetadata(shared, legacy)
			_ = backupLegacySophiaDir(legacy)
		}
		return shared
	}

	if legacyOK {
		if err := s.migrateLegacyLocalMetadata(shared, legacy); err == nil {
			return shared
		}
		return legacy
	}

	return shared
}

func loadConfigIfInitialized(repoRoot, sophiaDir string) (model.Config, bool) {
	st := store.NewWithSophiaRoot(repoRoot, sophiaDir)
	if !st.IsInitialized() {
		return model.Config{}, false
	}
	cfg, err := st.LoadConfig()
	if err != nil {
		return model.Config{}, false
	}
	return cfg, true
}

func (s *Service) setStoreSophiaDir(sophiaDir string) {
	s.store = store.NewWithSophiaRoot(s.repoRoot, sophiaDir)
}

func (s *Service) migrateLegacyLocalMetadata(sharedDir, legacyDir string) error {
	if !pathExists(legacyDir) {
		return nil
	}
	if pathExists(sharedDir) {
		if err := s.reconcileLegacyLocalMetadata(sharedDir, legacyDir); err != nil {
			return err
		}
		return backupLegacySophiaDir(legacyDir)
	}

	if err := os.MkdirAll(filepath.Dir(sharedDir), 0o755); err != nil {
		return fmt.Errorf("create shared metadata parent: %w", err)
	}
	if err := copyDirRecursive(legacyDir, sharedDir); err != nil {
		return err
	}
	return backupLegacySophiaDir(legacyDir)
}

func (s *Service) reconcileLegacyLocalMetadata(sharedDir, legacyDir string) error {
	sharedStore := store.NewWithSophiaRoot(s.repoRoot, sharedDir)
	legacyStore := store.NewWithSophiaRoot(s.repoRoot, legacyDir)

	var legacyCfg model.Config
	if legacyStore.IsInitialized() {
		cfg, err := legacyStore.LoadConfig()
		if err == nil {
			legacyCfg = cfg
		}
	}

	if !sharedStore.IsInitialized() {
		base := strings.TrimSpace(legacyCfg.BaseBranch)
		if base == "" {
			base = "main"
		}
		if err := sharedStore.Init(base, model.MetadataModeLocal); err != nil {
			return err
		}
	}

	if legacyStore.IsInitialized() {
		legacyCRs, err := legacyStore.ListCRs()
		if err == nil {
			for i := range legacyCRs {
				cr := legacyCRs[i]
				if _, loadErr := sharedStore.LoadCR(cr.ID); loadErr == nil {
					continue
				}
				copyCR := cr
				if saveErr := sharedStore.SaveCR(&copyCR); saveErr != nil {
					return saveErr
				}
			}
		}
	}

	nextID := 1
	if idx, err := sharedStore.LoadIndex(); err == nil && idx.NextID > nextID {
		nextID = idx.NextID
	}
	if legacyStore.IsInitialized() {
		if idx, err := legacyStore.LoadIndex(); err == nil && idx.NextID > nextID {
			nextID = idx.NextID
		}
	}
	if sharedCRs, err := sharedStore.ListCRs(); err == nil {
		highest := 0
		for _, cr := range sharedCRs {
			if cr.ID > highest {
				highest = cr.ID
			}
		}
		if highest+1 > nextID {
			nextID = highest + 1
		}
	}
	if err := sharedStore.SaveIndex(model.Index{NextID: nextID}); err != nil {
		return err
	}

	if strings.TrimSpace(legacyCfg.BaseBranch) != "" {
		cfg, err := sharedStore.LoadConfig()
		if err == nil {
			if strings.TrimSpace(cfg.BaseBranch) == "" {
				cfg.BaseBranch = strings.TrimSpace(legacyCfg.BaseBranch)
			}
			cfg.MetadataMode = model.MetadataModeLocal
			if saveErr := sharedStore.SaveConfig(cfg); saveErr != nil {
				return saveErr
			}
		}
	}

	legacySample := filepath.Join(legacyDir, "cr-plan.sample.yaml")
	sharedSample := filepath.Join(sharedDir, "cr-plan.sample.yaml")
	if pathExists(legacySample) && !pathExists(sharedSample) {
		if err := copyFile(legacySample, sharedSample, 0o644); err != nil {
			return err
		}
	}

	return nil
}

func backupLegacySophiaDir(legacyDir string) error {
	if !pathExists(legacyDir) {
		return nil
	}
	backupPath := legacyDir + ".migrated." + time.Now().UTC().Format("20060102T150405Z")
	if err := os.Rename(legacyDir, backupPath); err != nil {
		return fmt.Errorf("backup legacy metadata %q -> %q: %w", legacyDir, backupPath, err)
	}
	return nil
}

func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func copyDirRecursive(srcDir, dstDir string) error {
	if err := os.MkdirAll(dstDir, 0o755); err != nil {
		return fmt.Errorf("create destination metadata dir: %w", err)
	}
	return filepath.WalkDir(srcDir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		dstPath := filepath.Join(dstDir, rel)
		if d.IsDir() {
			return os.MkdirAll(dstPath, 0o755)
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		mode := info.Mode() & fs.ModePerm
		if mode == 0 {
			mode = 0o644
		}
		return copyFile(path, dstPath, mode)
	})
}

func copyFile(src, dst string, perm fs.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open %q: %w", src, err)
	}
	defer in.Close()

	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return fmt.Errorf("create parent %q: %w", dst, err)
	}
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, perm)
	if err != nil {
		return fmt.Errorf("open %q: %w", dst, err)
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return fmt.Errorf("copy %q -> %q: %w", src, dst, err)
	}
	if err := out.Close(); err != nil {
		return fmt.Errorf("close %q: %w", dst, err)
	}
	return nil
}
