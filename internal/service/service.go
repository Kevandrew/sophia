package service

import (
	"fmt"
	"sophia/internal/gitx"
	"sophia/internal/model"
	"sophia/internal/store"
	"strings"
	"time"
)

type Service struct {
	store                    *store.Store
	git                      *gitx.Client
	trustSvc                 *trustDomain
	mergeSvc                 *mergeDomain
	taskLifecycleSvc         *taskLifecycleDomain
	taskStore                taskLifecycleStoreProvider
	taskGit                  taskLifecycleGitProvider
	taskMergeGuard           func(*model.CR) error
	lifecycleStore           lifecycleRuntimeStore
	lifecycleGit             lifecycleRuntimeGit
	statusStore              statusRuntimeStore
	statusGit                statusRuntimeGit
	mergeStore               mergeRuntimeStore
	mergeGit                 mergeRuntimeGit
	mergeGitFactory          mergeRuntimeGitFactory
	delegationRuntimes       map[string]DelegationRuntime
	lifecycleStoreCustom     bool
	lifecycleGitCustom       bool
	statusStoreCustom        bool
	statusGitCustom          bool
	mergeStoreCustom         bool
	mergeGitCustom           bool
	mergeFactoryCustom       bool
	delegationRuntimesCustom bool
	now                      func() time.Time
	repoRoot                 string
	legacySophiaDir          string
	sharedLocalSophiaDir     string
}

type MergeConflictError struct {
	CRID          int
	BaseBranch    string
	CRBranch      string
	WorktreePath  string
	ConflictFiles []string
	Cause         error
}

type MergeInProgressError struct {
	WorktreePath  string
	ConflictFiles []string
	Summary       string
}

type NoMergeInProgressError struct {
	WorktreePath string
	Summary      string
}

type BranchInOtherWorktreeError struct {
	CRID                int
	Branch              string
	OwnerWorktreePath   string
	CurrentWorktreePath string
	Operation           string
	SuggestedCommand    string
}

type DoctorFinding struct {
	Code    string
	Message string
}

type DoctorReport struct {
	CurrentBranch  string
	BaseBranch     string
	UntrackedCount int
	ChangedCount   int
	ScannedCommits int
	Findings       []DoctorFinding
}

type CurrentCRContext struct {
	Branch string
	CR     *model.CR
}

type AddCROptions struct {
	BaseRef        string
	ParentCRID     int
	Switch         bool
	NoSwitch       bool
	BranchAlias    string
	OwnerPrefix    string
	OwnerPrefixSet bool
	UIDOverride    string
}

type AddCRBootstrapInfo struct {
	Triggered    bool
	BaseBranch   string
	MetadataMode string
	SophiaDir    string
}

const (
	RefreshStrategyAuto    = "auto"
	RefreshStrategyRestack = "restack"
	RefreshStrategyRebase  = "rebase"
)

type RefreshOptions struct {
	Strategy string
	DryRun   bool
}

type CRRefreshView struct {
	CRID       int
	Strategy   string
	DryRun     bool
	Applied    bool
	BaseRef    string
	TargetRef  string
	BeforeHead string
	AfterHead  string
	Warnings   []string
}

type LogEntry struct {
	ID           int
	Title        string
	Status       string
	Who          string
	When         string
	FilesTouched string
}

type RepairReport struct {
	BaseBranch    string
	Scanned       int
	Imported      int
	Updated       int
	Skipped       int
	NextID        int
	HighestCRID   int
	RepairedCRIDs []int
	Warnings      []string
}

type InitOptions struct {
	BaseBranch        string
	MetadataMode      string
	BranchOwnerPrefix string
}

type diffSummary struct {
	Files           []string
	ShortStat       string
	NumStats        []gitx.DiffNumStat
	NewFiles        []string
	ModifiedFiles   []string
	DeletedFiles    []string
	TestFiles       []string
	DependencyFiles []string
}

type parsedPatchChunk struct {
	ID       string
	Path     string
	OldStart int
	OldLines int
	NewStart int
	NewLines int
	Header   string
	Body     string
	Preview  string
}

type parsedPatchFile struct {
	Path        string
	HeaderLines []string
	Hunks       []parsedPatchChunk
}

func New(root string) *Service {
	svc := &Service{
		git: gitx.New(root),
		now: time.Now,
	}
	svc.bootstrapRepoContext(root)
	svc.composeRuntimePorts()
	return svc
}

func (s *Service) Init(baseBranch, metadataMode string) (string, error) {
	return s.InitWithOptions(InitOptions{
		BaseBranch:   baseBranch,
		MetadataMode: metadataMode,
	})
}

type initResolution struct {
	requestedBase  string
	requestedMode  string
	effectiveBase  string
	effectiveMode  string
	wasInitialized bool
}

func (s *Service) InitWithOptions(opts InitOptions) (string, error) {
	if err := s.ensureInitRepoContext(); err != nil {
		return "", err
	}
	s.applyRequestedMetadataModeForInit(opts.MetadataMode)
	resolution, err := s.resolveInitInputs(opts.BaseBranch, opts.MetadataMode)
	if err != nil {
		return "", err
	}
	resolution.wasInitialized = s.applyEffectiveMetadataModeForInit(resolution.effectiveMode, resolution.wasInitialized)
	if err := s.initializeStoreForInit(resolution); err != nil {
		return "", err
	}
	if err := s.finalizeInitArtifacts(resolution.effectiveMode, opts.BranchOwnerPrefix); err != nil {
		return "", err
	}
	return resolution.effectiveBase, nil
}

func (s *Service) ensureInitRepoContext() error {
	if !s.git.InRepo() {
		if err := s.git.InitRepo(); err != nil {
			return fmt.Errorf("initialize git repository: %w", err)
		}
	}
	s.bootstrapRepoContext(s.git.WorkDir)
	s.composeRuntimePorts()
	return nil
}

func (s *Service) applyRequestedMetadataModeForInit(metadataMode string) {
	switch strings.TrimSpace(metadataMode) {
	case model.MetadataModeTracked:
		s.setStoreSophiaDir(s.legacySophiaDir)
	case model.MetadataModeLocal:
		if strings.TrimSpace(s.sharedLocalSophiaDir) != "" {
			if err := s.migrateLegacyLocalMetadata(s.sharedLocalSophiaDir, s.legacySophiaDir); err == nil {
				s.setStoreSophiaDir(s.sharedLocalSophiaDir)
			} else {
				s.setStoreSophiaDir(s.legacySophiaDir)
			}
		} else {
			s.setStoreSophiaDir(s.legacySophiaDir)
		}
	}
	s.composeRuntimePorts()
}

func (s *Service) resolveInitInputs(baseBranch, metadataMode string) (initResolution, error) {
	resolution := initResolution{
		requestedBase:  strings.TrimSpace(baseBranch),
		requestedMode:  strings.TrimSpace(metadataMode),
		effectiveBase:  strings.TrimSpace(baseBranch),
		wasInitialized: s.store.IsInitialized(),
	}
	existingMode := ""
	if resolution.effectiveBase == "" && resolution.wasInitialized {
		cfg, err := s.store.LoadConfig()
		if err == nil {
			if strings.TrimSpace(cfg.BaseBranch) != "" {
				resolution.effectiveBase = cfg.BaseBranch
			}
			existingMode = cfg.MetadataMode
		}
	}
	resolution.effectiveMode = resolution.requestedMode
	if resolution.effectiveMode == "" {
		resolution.effectiveMode = strings.TrimSpace(existingMode)
	}
	if resolution.effectiveMode == "" {
		resolution.effectiveMode = model.MetadataModeLocal
	}
	if !isValidMetadataMode(resolution.effectiveMode) {
		return initResolution{}, fmt.Errorf("invalid metadata mode %q (expected local or tracked)", resolution.effectiveMode)
	}
	if resolution.effectiveBase == "" {
		resolution.effectiveBase = s.inferDefaultBaseBranch()
	}
	return resolution, nil
}

func (s *Service) inferDefaultBaseBranch() string {
	if s.git != nil {
		if symbolic, err := s.git.ResolveSymbolicRef("refs/remotes/origin/HEAD"); err == nil {
			const remotePrefix = "refs/remotes/origin/"
			if strings.HasPrefix(symbolic, remotePrefix) {
				branch := strings.TrimSpace(strings.TrimPrefix(symbolic, remotePrefix))
				if branch != "" {
					return branch
				}
			}
		}
	}
	if s.git != nil && s.git.BranchExists("main") {
		return "main"
	}
	if s.git != nil && s.git.BranchExists("master") {
		return "master"
	}
	if s.git != nil {
		currentBranch, err := s.git.CurrentBranch()
		if err == nil && strings.TrimSpace(currentBranch) != "" {
			return strings.TrimSpace(currentBranch)
		}
	}
	return "main"
}

func (s *Service) applyEffectiveMetadataModeForInit(effectiveMode string, fallbackInitialized bool) bool {
	switch effectiveMode {
	case model.MetadataModeTracked:
		s.setStoreSophiaDir(s.legacySophiaDir)
		s.composeRuntimePorts()
		return s.store.IsInitialized()
	case model.MetadataModeLocal:
		if strings.TrimSpace(s.sharedLocalSophiaDir) != "" {
			if err := s.migrateLegacyLocalMetadata(s.sharedLocalSophiaDir, s.legacySophiaDir); err == nil {
				s.setStoreSophiaDir(s.sharedLocalSophiaDir)
			}
			s.composeRuntimePorts()
			return s.store.IsInitialized()
		}
	}
	s.composeRuntimePorts()
	return fallbackInitialized
}

func (s *Service) composeRuntimePorts() {
	if s.store != nil {
		if !s.lifecycleStoreCustom || s.lifecycleStore == nil {
			s.lifecycleStore = s.store
		}
		if !s.statusStoreCustom || s.statusStore == nil {
			s.statusStore = s.store
		}
		if !s.mergeStoreCustom || s.mergeStore == nil {
			s.mergeStore = s.store
		}
	}
	if s.git != nil {
		if !s.lifecycleGitCustom || s.lifecycleGit == nil {
			s.lifecycleGit = s.git
		}
		if !s.statusGitCustom || s.statusGit == nil {
			s.statusGit = s.git
		}
		if !s.mergeGitCustom || s.mergeGit == nil {
			s.mergeGit = s.git
		}
	}
	if !s.mergeFactoryCustom || s.mergeGitFactory == nil {
		s.mergeGitFactory = func(root string) mergeRuntimeGit {
			return gitx.New(root)
		}
	}
}

func (s *Service) initializeStoreForInit(resolution initResolution) error {
	if err := s.git.EnsureBranchExists(resolution.effectiveBase); err != nil {
		return fmt.Errorf("prepare base branch %q: %w", resolution.effectiveBase, err)
	}
	configBase := ""
	if !resolution.wasInitialized || resolution.requestedBase != "" {
		configBase = resolution.effectiveBase
	}
	configMode := ""
	if !resolution.wasInitialized || resolution.requestedMode != "" {
		configMode = resolution.effectiveMode
	}
	return s.store.Init(configBase, configMode)
}

func (s *Service) finalizeInitArtifacts(effectiveMode string, branchOwnerPrefix string) error {
	if err := ensureCRPlanSample(s.store.SophiaDir()); err != nil {
		return err
	}
	if err := ensureRepoPolicyFile(s.repoRoot); err != nil {
		return err
	}
	if effectiveMode == model.MetadataModeLocal {
		if err := ensureGitIgnoreEntry(s.git.WorkDir, ".sophia/"); err != nil {
			return err
		}
	}
	trimmedPrefix := strings.TrimSpace(branchOwnerPrefix)
	if trimmedPrefix == "" {
		return nil
	}
	normalizedPrefix, prefixErr := normalizeCRBranchOwnerPrefix(trimmedPrefix)
	if prefixErr != nil {
		return prefixErr
	}
	cfg, cfgErr := s.store.LoadConfig()
	if cfgErr != nil {
		return cfgErr
	}
	cfg.BranchOwnerPrefix = normalizedPrefix
	return s.store.SaveConfig(cfg)
}

func (s *Service) Config() (model.Config, error) {
	return s.store.LoadConfig()
}

func (s *Service) ensureLazyLocalBootstrapForCRMutation() (AddCRBootstrapInfo, error) {
	info := AddCRBootstrapInfo{}
	if s.lifecycleStoreCustom || s.lifecycleGitCustom {
		return info, nil
	}
	if s.store != nil && s.store.IsInitialized() {
		return info, nil
	}

	if s.git != nil && !s.git.InRepo() {
		return info, store.ErrNotInitialized
	}

	// Rebind to the active repository root before deciding bootstrap target metadata path.
	s.bootstrapRepoContext(s.git.WorkDir)
	s.composeRuntimePorts()
	if s.store != nil && s.store.IsInitialized() {
		return info, nil
	}

	baseBranch := s.inferDefaultBaseBranch()
	if err := s.store.Init(baseBranch, model.MetadataModeLocal); err != nil {
		return info, err
	}
	cfg, err := s.store.LoadConfig()
	if err != nil {
		return info, err
	}
	info.Triggered = true
	info.BaseBranch = strings.TrimSpace(cfg.BaseBranch)
	info.MetadataMode = strings.TrimSpace(cfg.MetadataMode)
	info.SophiaDir = s.store.SophiaDir()
	return info, nil
}
