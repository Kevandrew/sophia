package service

import (
	"errors"
	"fmt"
	"regexp"
	"sophia/internal/gitx"
	"sophia/internal/model"
	"sophia/internal/store"
	"strings"
	"time"
)

var (
	ErrCRAlreadyMerged        = errors.New("cr is already merged")
	ErrNoActiveCRContext      = errors.New("current branch is not a CR branch")
	ErrWorkingTreeDirty       = errors.New("working tree is dirty")
	ErrNoCRChanges            = errors.New("no CR changes provided")
	ErrCRValidationFailed     = errors.New("cr validation failed")
	ErrParentCRNotMerged      = errors.New("parent cr is not merged")
	ErrParentCRRequired       = errors.New("cr has no parent")
	ErrAlreadyRedacted        = errors.New("target is already redacted")
	ErrNoTaskChanges          = errors.New("no task checkpoint changes found")
	ErrTaskScopeRequired      = errors.New("checkpoint scope is required (use --patch-file, --path, --from-contract, or --all)")
	ErrInvalidTaskScope       = errors.New("invalid task checkpoint scope")
	ErrPreStagedChanges       = errors.New("staged changes already exist before checkpoint")
	ErrTaskContractIncomplete = errors.New("task contract is incomplete")
	ErrNoTaskScopeMatches     = errors.New("no changed files match task contract scope")
	ErrTaskDelegated          = errors.New("task is delegated")
	ErrTaskNotDone            = errors.New("task is not done")
)

var (
	crBranchPattern      = regexp.MustCompile(`^sophia/cr-(\d+)$`)
	crSubjectPattern     = regexp.MustCompile(`^\[CR-(\d+)\]\s*(.*)$`)
	crFooterPattern      = regexp.MustCompile(`(?m)^Sophia-CR:\s*\d+\s*$`)
	legacyPersistPattern = regexp.MustCompile(`^chore:\s*persist CR-\d+\s+merged metadata$`)
	footerCRIDPattern    = regexp.MustCompile(`(?m)^Sophia-CR:\s*(\d+)\s*$`)
	footerCRUIDPattern   = regexp.MustCompile(`(?m)^Sophia-CR-UID:\s*(\S+)\s*$`)
	footerBaseRefPattern = regexp.MustCompile(`(?m)^Sophia-Base-Ref:\s*(.+)\s*$`)
	footerBaseSHApattern = regexp.MustCompile(`(?m)^Sophia-Base-Commit:\s*(\S+)\s*$`)
	footerParentPattern  = regexp.MustCompile(`(?m)^Sophia-Parent-CR:\s*(\d+)\s*$`)
	hunkHeaderPattern    = regexp.MustCompile(`^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@`)
	footerIntentPattern  = regexp.MustCompile(`(?m)^Sophia-Intent:\s*(.+)\s*$`)
)

const redactedPlaceholder = "[REDACTED]"

type Service struct {
	store *store.Store
	git   *gitx.Client
	now   func() time.Time
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
	BaseRef    string
	ParentCRID int
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

type CRHistory struct {
	CRID        int
	Title       string
	Status      string
	Description string
	Notes       []HistoryNote
	Events      []HistoryEvent
}

type DoneTaskOptions struct {
	Checkpoint   bool
	StageAll     bool
	Paths        []string
	FromContract bool
	PatchFile    string
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

type RiskSignal struct {
	Code    string
	Summary string
	Points  int
}

type ImpactReport struct {
	CRID                      int
	CRUID                     string
	BaseRef                   string
	BaseCommit                string
	ParentCRID                int
	RiskTierHint              string
	RiskTierFloorApplied      bool
	MatchedRiskCriticalScopes []string
	FilesChanged              int
	NewFiles                  []string
	ModifiedFiles             []string
	DeletedFiles              []string
	TestFiles                 []string
	DependencyFiles           []string
	ScopeDrift                []string
	TaskScopeWarnings         []string
	TaskContractWarnings      []string
	TaskChunkWarnings         []string
	Signals                   []RiskSignal
	RiskScore                 int
	RiskTier                  string
}

type ValidationReport struct {
	Valid    bool
	Errors   []string
	Warnings []string
	Impact   *ImpactReport
}

type TrustDimension struct {
	Code            string
	Label           string
	Score           int
	Max             int
	Reasons         []string
	RequiredActions []string
}

type TrustReport struct {
	Verdict         string
	Score           int
	Max             int
	AdvisoryOnly    bool
	HardFailures    []string
	Dimensions      []TrustDimension
	RequiredActions []string
	Summary         string
}

type TaskContractPatch struct {
	Intent             *string
	AcceptanceCriteria *[]string
	Scope              *[]string
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

func New(root string) *Service {
	return &Service{
		store: store.New(root),
		git:   gitx.New(root),
		now:   time.Now,
	}
}

func (s *Service) Init(baseBranch, metadataMode string) (string, error) {
	if !s.git.InRepo() {
		if err := s.git.InitRepo(); err != nil {
			return "", fmt.Errorf("initialize git repository: %w", err)
		}
	}

	wasInitialized := s.store.IsInitialized()
	existingMode := ""
	effectiveBase := strings.TrimSpace(baseBranch)
	if effectiveBase == "" && wasInitialized {
		cfg, err := s.store.LoadConfig()
		if err == nil {
			if strings.TrimSpace(cfg.BaseBranch) != "" {
				effectiveBase = cfg.BaseBranch
			}
			existingMode = cfg.MetadataMode
		}
	}
	effectiveMode := strings.TrimSpace(metadataMode)
	if effectiveMode == "" {
		effectiveMode = strings.TrimSpace(existingMode)
	}
	if effectiveMode == "" {
		effectiveMode = model.MetadataModeLocal
	}
	if !isValidMetadataMode(effectiveMode) {
		return "", fmt.Errorf("invalid metadata mode %q (expected local or tracked)", effectiveMode)
	}
	if effectiveBase == "" {
		currentBranch, err := s.git.CurrentBranch()
		if err == nil && strings.TrimSpace(currentBranch) != "" {
			effectiveBase = currentBranch
		}
	}
	if effectiveBase == "" {
		effectiveBase = "main"
	}

	if err := s.git.EnsureBaseBranch(effectiveBase); err != nil {
		return "", fmt.Errorf("prepare base branch %q: %w", effectiveBase, err)
	}

	configBase := ""
	if !wasInitialized {
		configBase = effectiveBase
	} else if strings.TrimSpace(baseBranch) != "" {
		configBase = effectiveBase
	}
	configMode := ""
	if !wasInitialized {
		configMode = effectiveMode
	} else if strings.TrimSpace(metadataMode) != "" {
		configMode = effectiveMode
	}
	if err := s.store.Init(configBase, configMode); err != nil {
		return "", err
	}
	if err := ensureCRPlanSample(s.store.SophiaDir()); err != nil {
		return "", err
	}
	if effectiveMode == model.MetadataModeLocal {
		if err := ensureGitIgnoreEntry(s.git.WorkDir, ".sophia/"); err != nil {
			return "", err
		}
	}

	return effectiveBase, nil
}
