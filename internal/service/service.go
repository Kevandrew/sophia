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
	store                *store.Store
	git                  *gitx.Client
	trustSvc             *trustDomain
	mergeSvc             *mergeDomain
	taskLifecycleSvc     *taskLifecycleDomain
	taskStore            taskLifecycleStoreProvider
	taskGit              taskLifecycleGitProvider
	taskMergeGuard       func(*model.CR) error
	lifecycleStore       lifecycleRuntimeStore
	lifecycleGit         lifecycleRuntimeGit
	statusStore          statusRuntimeStore
	statusGit            statusRuntimeGit
	mergeStore           mergeRuntimeStore
	mergeGit             mergeRuntimeGit
	mergeGitFactory      mergeRuntimeGitFactory
	lifecycleStoreCustom bool
	lifecycleGitCustom   bool
	statusStoreCustom    bool
	statusGitCustom      bool
	mergeStoreCustom     bool
	mergeGitCustom       bool
	mergeFactoryCustom   bool
	now                  func() time.Time
	repoRoot             string
	legacySophiaDir      string
	sharedLocalSophiaDir string
}

type Review struct {
	CR                 *model.CR
	Contract           model.Contract
	Impact             *ImpactReport
	Trust              *TrustReport
	ValidationErrors   []string
	ValidationWarnings []string
	Files              []string
	ShortStat          string
	NewFiles           []string
	ModifiedFiles      []string
	DeletedFiles       []string
	TestFiles          []string
	DependencyFiles    []string
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

type CRDoctorFinding struct {
	Code    string
	Message string
	TaskID  int
	Commit  string
}

type CRDoctorReport struct {
	CRID             int
	CRUID            string
	Branch           string
	BranchExists     bool
	BranchHead       string
	BaseRef          string
	BaseCommit       string
	ResolvedBaseRef  string
	ParentCRID       int
	ExpectedParentID int
	Findings         []CRDoctorFinding
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
}

type InitOptions struct {
	BaseBranch        string
	MetadataMode      string
	BranchOwnerPrefix string
}

type ReconcileCROptions struct {
	Regenerate bool
}

type ReconcileTaskResult struct {
	TaskID           int
	Title            string
	Status           string
	PreviousCommit   string
	CurrentCommit    string
	Action           string
	Reason           string
	Source           string
	CheckpointAt     string
	CheckpointOrphan bool
}

type ReconcileCRReport struct {
	CRID             int
	CRUID            string
	Branch           string
	BranchExists     bool
	PreviousParentID int
	CurrentParentID  int
	ParentRelinked   bool
	ScanRef          string
	ScannedCommits   int
	Relinked         int
	Orphaned         int
	ClearedOrphans   int
	Regenerated      bool
	FilesChanged     int
	DiffStat         string
	Warnings         []string
	Findings         []CRDoctorFinding
	TaskResults      []ReconcileTaskResult
}

type ExportCROptions struct {
	Format  string
	Include []string
}

type CRExportBundle struct {
	SchemaVersion     string                `json:"schema_version"`
	Format            string                `json:"format"`
	CRUID             string                `json:"cr_uid"`
	CRFingerprint     string                `json:"cr_fingerprint"`
	DocSchemaVersion  string                `json:"doc_schema_version"`
	Doc               *CRDoc                `json:"doc,omitempty"`
	Anchors           *CRExportAnchors      `json:"anchors,omitempty"`
	CR                *model.CR             `json:"cr"`
	CRYAML            string                `json:"cr_yaml"`
	Evidence          []model.EvidenceEntry `json:"evidence"`
	Derived           CRExportDerived       `json:"derived"`
	Checkpoints       []CRExportCheckpoint  `json:"checkpoints"`
	ReferencedCommits []string              `json:"referenced_commits"`
	Includes          []string              `json:"includes,omitempty"`
	TaskDiffs         []CRExportTaskDiff    `json:"task_diffs,omitempty"`
	Warnings          []string              `json:"warnings,omitempty"`
}

type CRExportDerived struct {
	FilesChanged    []string          `json:"files_changed"`
	NewFiles        []string          `json:"new_files"`
	ModifiedFiles   []string          `json:"modified_files"`
	DeletedFiles    []string          `json:"deleted_files"`
	TestFiles       []string          `json:"test_files"`
	DependencyFiles []string          `json:"dependency_files"`
	DiffStat        string            `json:"diff_stat"`
	Impact          *ImpactReport     `json:"impact"`
	Trust           *TrustReport      `json:"trust"`
	Validation      *ValidationReport `json:"validation"`
}

type CRExportCheckpoint struct {
	TaskID  int                     `json:"task_id"`
	Title   string                  `json:"title"`
	Status  string                  `json:"status"`
	Commit  string                  `json:"commit,omitempty"`
	At      string                  `json:"at,omitempty"`
	Message string                  `json:"message,omitempty"`
	Scope   []string                `json:"scope,omitempty"`
	Chunks  []model.CheckpointChunk `json:"chunks,omitempty"`
	Source  string                  `json:"source,omitempty"`
	Orphan  bool                    `json:"orphan,omitempty"`
	Reason  string                  `json:"reason,omitempty"`
}

type CRExportTaskDiff struct {
	TaskID int      `json:"task_id"`
	Title  string   `json:"title"`
	Commit string   `json:"commit"`
	Files  []string `json:"files,omitempty"`
	Patch  string   `json:"patch,omitempty"`
}

type CRExportAnchors struct {
	BaseRef    string `json:"base_ref,omitempty"`
	BaseCommit string `json:"base_commit,omitempty"`
	HeadRef    string `json:"head_ref,omitempty"`
	HeadCommit string `json:"head_commit,omitempty"`
	MergeBase  string `json:"merge_base,omitempty"`
}

type CRDocMetaEntry struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type CRDocEvent struct {
	TS              string           `json:"ts"`
	Actor           string           `json:"actor"`
	Type            string           `json:"type"`
	Summary         string           `json:"summary"`
	Ref             string           `json:"ref,omitempty"`
	Redacted        bool             `json:"redacted,omitempty"`
	RedactionReason string           `json:"redaction_reason,omitempty"`
	Meta            []CRDocMetaEntry `json:"meta,omitempty"`
}

type CRDoc struct {
	ID                int                   `json:"id"`
	UID               string                `json:"uid,omitempty"`
	Title             string                `json:"title"`
	Description       string                `json:"description"`
	Status            string                `json:"status"`
	BaseBranch        string                `json:"base_branch"`
	BaseRef           string                `json:"base_ref,omitempty"`
	BaseCommit        string                `json:"base_commit,omitempty"`
	ParentCRID        int                   `json:"parent_cr_id,omitempty"`
	Branch            string                `json:"branch"`
	Notes             []string              `json:"notes"`
	Evidence          []model.EvidenceEntry `json:"evidence,omitempty"`
	Contract          model.Contract        `json:"contract,omitempty"`
	Subtasks          []model.Subtask       `json:"subtasks"`
	Events            []CRDocEvent          `json:"events"`
	MergedAt          string                `json:"merged_at,omitempty"`
	MergedBy          string                `json:"merged_by,omitempty"`
	MergedCommit      string                `json:"merged_commit,omitempty"`
	FilesTouchedCount int                   `json:"files_touched_count,omitempty"`
	CreatedAt         string                `json:"created_at"`
	UpdatedAt         string                `json:"updated_at"`
}

type WhyView struct {
	CRID              int
	CRUID             string
	BaseRef           string
	BaseCommit        string
	ParentCRID        int
	EffectiveWhy      string
	Source            string
	Description       string
	ContractWhy       string
	ContractUpdatedAt string
	ContractUpdatedBy string
}

type CRStatusView struct {
	ID                    int
	UID                   string
	Title                 string
	Status                string
	BaseBranch            string
	BaseRef               string
	BaseCommit            string
	ParentCRID            int
	ParentStatus          string
	Branch                string
	CurrentBranch         string
	BranchMatch           bool
	ModifiedStagedCount   int
	UntrackedCount        int
	Dirty                 bool
	TasksTotal            int
	TasksOpen             int
	TasksDone             int
	TasksDelegated        int
	TasksDelegatedPending int
	ContractComplete      bool
	ContractMissingFields []string
	ValidationValid       bool
	ValidationErrors      int
	ValidationWarnings    int
	RiskTier              string
	RiskScore             int
	MergeBlocked          bool
	MergeBlockers         []string
}

type MergeStatusView struct {
	CRID          int
	CRUID         string
	BaseBranch    string
	CRBranch      string
	WorktreePath  string
	InProgress    bool
	ConflictFiles []string
	TargetMatches bool
	MergeHead     string
	Advice        []string
}

type TaskDelegationView struct {
	ChildCRID   int
	ChildCRUID  string
	ChildTaskID int
	ChildStatus string
	LinkedAt    string
	LinkedBy    string
}

type DelegateTaskResult struct {
	ParentTaskID     int
	ParentTaskStatus string
	ChildTaskID      int
	ChildCRID        int
}

type UndelegateTaskResult struct {
	ParentTaskID      int
	ParentTaskStatus  string
	RemovedDelegation int
}

type StackNodeView struct {
	ID                    int
	UID                   string
	ParentCRID            int
	Title                 string
	Status                string
	Branch                string
	Depth                 int
	Children              []int
	MergeBlocked          bool
	MergeBlockers         []string
	TasksTotal            int
	TasksOpen             int
	TasksDone             int
	TasksDelegated        int
	TasksDelegatedPending int
}

type StackView struct {
	RootCRID  int
	FocusCRID int
	Nodes     []StackNodeView
}

type HistoryNote struct {
	Index    int
	Text     string
	Redacted bool
}

type HistoryEvent struct {
	Index           int
	TS              string
	Actor           string
	Type            string
	Summary         string
	Ref             string
	Redacted        bool
	RedactionReason string
	Meta            map[string]string
}

type HistoryEvidence struct {
	Index       int
	TS          string
	Actor       string
	Type        string
	Scope       string
	Command     string
	ExitCode    *int
	OutputHash  string
	Summary     string
	Attachments []string
}

type CRHistory struct {
	CRID        int
	Title       string
	Status      string
	Description string
	Notes       []HistoryNote
	Evidence    []HistoryEvidence
	Events      []HistoryEvent
}

type AddEvidenceOptions struct {
	Type        string
	Scope       string
	Summary     string
	Command     string
	Capture     bool
	ExitCode    *int
	Attachments []string
}

type DoneTaskOptions struct {
	Checkpoint         bool
	StageAll           bool
	Paths              []string
	FromContract       bool
	PatchFile          string
	NoCheckpointReason string
	DryRun             bool
}

type ReopenTaskOptions struct {
	ClearCheckpoint bool
}

type ContractPatch struct {
	Why                *string
	Scope              *[]string
	NonGoals           *[]string
	Invariants         *[]string
	BlastRadius        *string
	RiskCriticalScopes *[]string
	RiskTierHint       *string
	RiskRationale      *string
	TestPlan           *string
	RollbackPlan       *string
}

type SetCRContractOptions struct {
	DryRun bool
}

type SetCRContractResult struct {
	ChangedFields  []string
	AlreadyApplied bool
	DryRun         bool
}

type ValidationReport struct {
	Valid    bool
	Errors   []string
	Warnings []string
	Impact   *ImpactReport
}

type TaskContractDriftSummary struct {
	Total               int
	Unacknowledged      int
	TasksWithDrift      []int
	UnacknowledgedTasks []int
}

type TaskContractPatch struct {
	Intent             *string
	AcceptanceCriteria *[]string
	Scope              *[]string
	AcceptanceChecks   *[]string
	ChangeReason       *string
}

type TaskChunk struct {
	ID       string
	Path     string
	OldStart int
	OldLines int
	NewStart int
	NewLines int
	Preview  string
}

type CRDiffOptions struct {
	TaskID       int
	CriticalOnly bool
}

type TaskDiffOptions struct {
	ChunksOnly   bool
	CriticalOnly bool
}

type DiffHunkView struct {
	ChunkID  string
	Path     string
	OldStart int
	OldLines int
	NewStart int
	NewLines int
	Header   string
	Preview  string
	Source   string
}

type DiffFileView struct {
	Path  string
	Hunks []DiffHunkView
}

type CRDiffView struct {
	CRID           int
	TaskID         int
	Mode           string
	CriticalOnly   bool
	ChunksOnly     bool
	BaseRef        string
	BaseCommit     string
	TargetRef      string
	Files          []DiffFileView
	FilesChanged   int
	ShortStat      string
	FallbackUsed   bool
	FallbackReason string
	Warnings       []string
}

type RangeDiffOptions struct {
	FromRef             string
	ToRef               string
	SinceLastCheckpoint bool
}

const (
	CRAnchorKindBase      = "base"
	CRAnchorKindHead      = "head"
	CRAnchorKindMergeBase = "merge-base"
)

type CRRangeAnchorsView struct {
	CRID      int
	Base      string
	Head      string
	MergeBase string
	Warnings  []string
}

type CRRevParseView struct {
	CRID     int
	Kind     string
	Commit   string
	Warnings []string
}

type PackOptions struct {
	EventsLimit      int
	CheckpointsLimit int
}

type PackSliceMeta struct {
	Total     int
	Returned  int
	Truncated int
}

type CRPackCheckpoint struct {
	TaskID  int
	Title   string
	Status  string
	Commit  string
	At      string
	Message string
	Scope   []string
	Source  string
	Orphan  bool
	Reason  string
}

type CRPackView struct {
	CR                *model.CR
	Contract          model.Contract
	Tasks             []model.Subtask
	Anchors           *CRRangeAnchorsView
	Status            *CRStatusView
	RecentEvents      []model.Event
	EventsMeta        PackSliceMeta
	RecentCheckpoints []CRPackCheckpoint
	CheckpointsMeta   PackSliceMeta
	DiffStat          string
	FilesChanged      []string
	Impact            *ImpactReport
	Validation        *ValidationReport
	Trust             *TrustReport
	Warnings          []string
}

type RangeDiffCommitMap struct {
	OldIndex  string
	OldCommit string
	Relation  string
	NewIndex  string
	NewCommit string
	Subject   string
}

type RangeDiffView struct {
	CRID         int
	TaskID       int
	FromRef      string
	ToRef        string
	BaseRef      string
	OldRange     string
	NewRange     string
	Mapping      []RangeDiffCommitMap
	FilesChanged []string
	ShortStat    string
	Warnings     []string
}

type BlameRange struct {
	Start int
	End   int
}

type BlameOptions struct {
	Rev    string
	Ranges []BlameRange
}

type BlameLineView struct {
	Line         int
	Commit       string
	Author       string
	AuthorEmail  string
	AuthorTime   string
	CRID         int
	HasCR        bool
	CRUID        string
	Intent       string
	IntentSource string
	Summary      string
	Text         string
}

type BlameView struct {
	Path   string
	Rev    string
	Ranges []BlameRange
	Lines  []BlameLineView
}

type diffSummary struct {
	Files           []string
	ShortStat       string
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
		currentBranch, err := s.git.CurrentBranch()
		if err == nil && strings.TrimSpace(currentBranch) != "" {
			resolution.effectiveBase = currentBranch
		}
	}
	if resolution.effectiveBase == "" {
		resolution.effectiveBase = "main"
	}
	return resolution, nil
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
