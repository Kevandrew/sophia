package store

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"sophia/internal/model"
)

var ErrNotInitialized = errors.New("sophia is not initialized in this repository")

type Store struct {
	Root       string
	SophiaRoot string
}

func New(root string) *Store {
	return &Store{Root: root}
}

func NewWithSophiaRoot(root, sophiaRoot string) *Store {
	return &Store{
		Root:       root,
		SophiaRoot: filepath.Clean(sophiaRoot),
	}
}

func (s *Store) SophiaDir() string {
	if strings.TrimSpace(s.SophiaRoot) != "" {
		return filepath.Clean(s.SophiaRoot)
	}
	return filepath.Join(s.Root, ".sophia")
}

func (s *Store) CRDir() string {
	return filepath.Join(s.SophiaDir(), "cr")
}

func (s *Store) ConfigPath() string {
	return filepath.Join(s.SophiaDir(), "config.yaml")
}

func (s *Store) IndexPath() string {
	return filepath.Join(s.SophiaDir(), "index.yaml")
}

func (s *Store) CRPath(id int) string {
	return filepath.Join(s.CRDir(), fmt.Sprintf("%d.yaml", id))
}

func (s *Store) IsInitialized() bool {
	_, err := os.Stat(s.ConfigPath())
	return err == nil
}

func (s *Store) EnsureInitialized() error {
	if s.IsInitialized() {
		return nil
	}
	return ErrNotInitialized
}

func (s *Store) Init(baseBranch, metadataMode string) error {
	if err := os.MkdirAll(s.CRDir(), 0o755); err != nil {
		return fmt.Errorf("create .sophia layout: %w", err)
	}

	cfg := model.Config{}
	if _, err := os.Stat(s.ConfigPath()); err == nil {
		existing, loadErr := s.LoadConfig()
		if loadErr != nil {
			return loadErr
		}
		cfg = existing
		if existing.Version == "" {
			cfg.Version = "v0"
		}
		if baseBranch != "" {
			cfg.BaseBranch = baseBranch
		}
		if cfg.BaseBranch == "" {
			cfg.BaseBranch = "main"
		}
		if metadataMode != "" {
			cfg.MetadataMode = metadataMode
		}
		if cfg.MetadataMode == "" {
			cfg.MetadataMode = model.MetadataModeLocal
		}
	} else {
		cfg.Version = "v0"
		if baseBranch == "" {
			baseBranch = "main"
		}
		cfg.BaseBranch = baseBranch
		if metadataMode == "" {
			metadataMode = model.MetadataModeLocal
		}
		cfg.MetadataMode = metadataMode
	}
	if err := s.writeYAMLAtomic(s.ConfigPath(), cfg); err != nil {
		return err
	}

	if _, err := os.Stat(s.IndexPath()); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			if err := s.writeYAMLAtomic(s.IndexPath(), model.Index{NextID: 1}); err != nil {
				return err
			}
		} else {
			return fmt.Errorf("stat index file: %w", err)
		}
	}

	return nil
}

func (s *Store) LoadConfig() (model.Config, error) {
	if err := s.EnsureInitialized(); err != nil {
		return model.Config{}, err
	}
	var cfg model.Config
	if err := s.readYAML(s.ConfigPath(), &cfg); err != nil {
		return model.Config{}, err
	}
	if cfg.Version == "" {
		cfg.Version = "v0"
	}
	if cfg.BaseBranch == "" {
		cfg.BaseBranch = "main"
	}
	if cfg.MetadataMode == "" {
		cfg.MetadataMode = model.MetadataModeLocal
	}
	return cfg, nil
}

func (s *Store) SaveConfig(cfg model.Config) error {
	if err := s.EnsureInitialized(); err != nil {
		return err
	}
	if cfg.Version == "" {
		cfg.Version = "v0"
	}
	if cfg.BaseBranch == "" {
		cfg.BaseBranch = "main"
	}
	if cfg.MetadataMode == "" {
		cfg.MetadataMode = model.MetadataModeLocal
	}
	return s.writeYAMLAtomic(s.ConfigPath(), cfg)
}

func (s *Store) LoadIndex() (model.Index, error) {
	if err := s.EnsureInitialized(); err != nil {
		return model.Index{}, err
	}
	var idx model.Index
	if err := s.readYAML(s.IndexPath(), &idx); err != nil {
		return model.Index{}, err
	}
	if idx.NextID < 1 {
		idx.NextID = 1
	}
	return idx, nil
}

func (s *Store) SaveIndex(idx model.Index) error {
	if err := s.EnsureInitialized(); err != nil {
		return err
	}
	if idx.NextID < 1 {
		idx.NextID = 1
	}
	return s.writeYAMLAtomic(s.IndexPath(), idx)
}

func (s *Store) NextCRID() (int, error) {
	idx, err := s.LoadIndex()
	if err != nil {
		return 0, err
	}
	id := idx.NextID
	idx.NextID++
	if err := s.SaveIndex(idx); err != nil {
		return 0, err
	}
	return id, nil
}

func (s *Store) LoadCR(id int) (*model.CR, error) {
	if err := s.EnsureInitialized(); err != nil {
		return nil, err
	}
	path := s.CRPath(id)
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("cr %d not found", id)
		}
		return nil, fmt.Errorf("stat cr file: %w", err)
	}
	var cr model.CR
	if err := s.readYAML(path, &cr); err != nil {
		return nil, err
	}
	if cr.Notes == nil {
		cr.Notes = []string{}
	}
	if cr.Evidence == nil {
		cr.Evidence = []model.EvidenceEntry{}
	}
	if cr.Subtasks == nil {
		cr.Subtasks = []model.Subtask{}
	}
	if cr.Events == nil {
		cr.Events = []model.Event{}
	}
	return &cr, nil
}

func (s *Store) SaveCR(cr *model.CR) error {
	if err := s.EnsureInitialized(); err != nil {
		return err
	}
	if cr == nil {
		return errors.New("cr cannot be nil")
	}
	if cr.Notes == nil {
		cr.Notes = []string{}
	}
	if cr.Evidence == nil {
		cr.Evidence = []model.EvidenceEntry{}
	}
	if cr.Subtasks == nil {
		cr.Subtasks = []model.Subtask{}
	}
	if cr.Events == nil {
		cr.Events = []model.Event{}
	}
	if err := os.MkdirAll(s.CRDir(), 0o755); err != nil {
		return fmt.Errorf("ensure cr directory: %w", err)
	}
	return s.writeYAMLAtomic(s.CRPath(cr.ID), cr)
}

func (s *Store) ListCRs() ([]model.CR, error) {
	if err := s.EnsureInitialized(); err != nil {
		return nil, err
	}
	matches, err := filepath.Glob(filepath.Join(s.CRDir(), "*.yaml"))
	if err != nil {
		return nil, fmt.Errorf("list cr files: %w", err)
	}
	crs := make([]model.CR, 0, len(matches))
	for _, path := range matches {
		var cr model.CR
		if err := s.readYAML(path, &cr); err != nil {
			return nil, err
		}
		if cr.Notes == nil {
			cr.Notes = []string{}
		}
		if cr.Evidence == nil {
			cr.Evidence = []model.EvidenceEntry{}
		}
		if cr.Subtasks == nil {
			cr.Subtasks = []model.Subtask{}
		}
		if cr.Events == nil {
			cr.Events = []model.Event{}
		}
		crs = append(crs, cr)
	}
	sort.Slice(crs, func(i, j int) bool {
		return crs[i].ID < crs[j].ID
	})
	return crs, nil
}

func (s *Store) readYAML(path string, into any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	if err := yaml.Unmarshal(data, into); err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}
	return nil
}

func (s *Store) writeYAMLAtomic(path string, v any) error {
	data, err := yaml.Marshal(v)
	if err != nil {
		return fmt.Errorf("marshal yaml for %s: %w", path, err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create directory for %s: %w", path, err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("write temp file for %s: %w", path, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("rename temp file for %s: %w", path, err)
	}
	return nil
}
