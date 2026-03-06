package store

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"

	"sophia/internal/model"
)

var ErrNotInitialized = errors.New("sophia is not initialized in this repository")
var ErrNotFound = errors.New("resource not found")
var ErrInvalidArgument = errors.New("invalid argument")
var ErrMutationLockTimeout = errors.New("timed out acquiring mutation lock")

type NotFoundError struct {
	Resource string
	Value    string
}

func (e NotFoundError) Error() string {
	resource := strings.TrimSpace(e.Resource)
	value := strings.TrimSpace(e.Value)
	switch {
	case resource == "" && value == "":
		return "resource not found"
	case value == "":
		return fmt.Sprintf("%s not found", resource)
	default:
		return fmt.Sprintf("%s %q not found", resource, value)
	}
}

func (e NotFoundError) Is(target error) bool {
	return target == ErrNotFound
}

type InvalidArgumentError struct {
	Argument string
	Message  string
}

func (e InvalidArgumentError) Error() string {
	argument := strings.TrimSpace(e.Argument)
	message := strings.TrimSpace(e.Message)
	switch {
	case argument == "" && message == "":
		return "invalid argument"
	case argument == "":
		return message
	case message == "":
		return fmt.Sprintf("invalid %s", argument)
	default:
		return fmt.Sprintf("invalid %s: %s", argument, message)
	}
}

func (e InvalidArgumentError) Is(target error) bool {
	return target == ErrInvalidArgument
}

type MutationLockTimeoutError struct {
	Path    string
	Timeout time.Duration
}

func (e MutationLockTimeoutError) Error() string {
	lockPath := strings.TrimSpace(e.Path)
	if lockPath == "" {
		lockPath = "mutation.lock"
	}
	timeout := e.Timeout
	if timeout <= 0 {
		timeout = 0
	}
	return fmt.Sprintf("unable to acquire Sophia mutation lock %q within %s; another mutation is likely in progress, retry shortly", lockPath, timeout)
}

func (e MutationLockTimeoutError) Is(target error) bool {
	return target == ErrMutationLockTimeout
}

type Store struct {
	Root       string
	SophiaRoot string
	cacheMu    sync.RWMutex
	crCache    crMetadataCache
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

	s.invalidateCRCache()
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
	return s.cachedCRByID(id)
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
	if cr.PR.CheckpointCommentKeys == nil {
		cr.PR.CheckpointCommentKeys = []string{}
	}
	if cr.PR.CheckpointSyncKeys == nil {
		cr.PR.CheckpointSyncKeys = []string{}
	}
	if err := os.MkdirAll(s.CRDir(), 0o755); err != nil {
		return fmt.Errorf("ensure cr directory: %w", err)
	}
	if err := s.writeYAMLAtomic(s.CRPath(cr.ID), cr); err != nil {
		return err
	}
	s.invalidateCRCache()
	return nil
}

func (s *Store) LoadCRByUID(uid string) (*model.CR, error) {
	if err := s.EnsureInitialized(); err != nil {
		return nil, err
	}
	needle := strings.TrimSpace(uid)
	if needle == "" {
		return nil, InvalidArgumentError{Argument: "cr uid", Message: "cannot be empty"}
	}
	crs, err := s.cachedCRs()
	if err != nil {
		return nil, err
	}
	matches := make([]model.CR, 0, 1)
	for _, cr := range crs {
		if strings.TrimSpace(cr.UID) != needle {
			continue
		}
		matches = append(matches, cr)
	}
	switch len(matches) {
	case 0:
		return nil, NotFoundError{Resource: "cr uid", Value: needle}
	case 1:
		cr := matches[0]
		return &cr, nil
	default:
		ids := make([]string, 0, len(matches))
		for _, m := range matches {
			ids = append(ids, fmt.Sprintf("%d", m.ID))
		}
		sort.Strings(ids)
		return nil, fmt.Errorf("cr uid %q is ambiguous across ids: %s", needle, strings.Join(ids, ","))
	}
}

func (s *Store) ListCRs() ([]model.CR, error) {
	return s.cachedCRs()
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
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create directory for %s: %w", path, err)
	}
	tmpFile, err := os.CreateTemp(dir, filepath.Base(path)+".*.tmp")
	if err != nil {
		return fmt.Errorf("create temp file for %s: %w", path, err)
	}
	tmp := tmpFile.Name()
	if _, err := tmpFile.Write(data); err != nil {
		_ = tmpFile.Close()
		_ = os.Remove(tmp)
		return fmt.Errorf("write temp file for %s: %w", path, err)
	}
	if err := tmpFile.Close(); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("close temp file for %s: %w", path, err)
	}
	if err := os.Chmod(tmp, 0o644); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("chmod temp file for %s: %w", path, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("rename temp file for %s: %w", path, err)
	}
	return nil
}

func (s *Store) mutationLockPath() string {
	return filepath.Join(s.SophiaDir(), "mutation.lock")
}

func (s *Store) WithMutationLock(timeout time.Duration, fn func() error) error {
	return s.WithMutationLockPath(s.mutationLockPath(), timeout, fn)
}

func (s *Store) WithMutationLockPath(lockPath string, timeout time.Duration, fn func() error) error {
	if fn == nil {
		return InvalidArgumentError{Argument: "mutation callback", Message: "cannot be nil"}
	}
	lockPath = strings.TrimSpace(lockPath)
	if lockPath == "" {
		return InvalidArgumentError{Argument: "mutation lock path", Message: "cannot be empty"}
	}
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o755); err != nil {
		return fmt.Errorf("create mutation lock directory for %s: %w", lockPath, err)
	}
	deadline := time.Now().Add(timeout)
	for {
		lockFile, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err == nil {
			_, _ = fmt.Fprintf(lockFile, "pid=%d\nacquired_at=%s\n", os.Getpid(), time.Now().UTC().Format(time.RFC3339Nano))
			_ = lockFile.Close()
			defer func() {
				_ = os.Remove(lockPath)
			}()
			return fn()
		}
		if !errors.Is(err, os.ErrExist) {
			return fmt.Errorf("acquire mutation lock %q: %w", lockPath, err)
		}
		if time.Now().After(deadline) {
			return MutationLockTimeoutError{
				Path:    lockPath,
				Timeout: timeout,
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
}
