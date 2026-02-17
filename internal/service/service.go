package service

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"sophia/internal/gitx"
	"sophia/internal/model"
	"sophia/internal/store"
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
	ContractComplete      bool
	ContractMissingFields []string
	ValidationValid       bool
	ValidationErrors      int
	ValidationWarnings    int
	RiskTier              string
	RiskScore             int
	MergeBlocked          bool
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

type ContractPatch struct {
	Why          *string
	Scope        *[]string
	NonGoals     *[]string
	Invariants   *[]string
	BlastRadius  *string
	TestPlan     *string
	RollbackPlan *string
}

type RiskSignal struct {
	Code    string
	Summary string
	Points  int
}

type ImpactReport struct {
	CRID                 int
	CRUID                string
	BaseRef              string
	BaseCommit           string
	ParentCRID           int
	FilesChanged         int
	NewFiles             []string
	ModifiedFiles        []string
	DeletedFiles         []string
	TestFiles            []string
	DependencyFiles      []string
	ScopeDrift           []string
	TaskScopeWarnings    []string
	TaskContractWarnings []string
	Signals              []RiskSignal
	RiskScore            int
	RiskTier             string
}

type ValidationReport struct {
	Valid    bool
	Errors   []string
	Warnings []string
	Impact   *ImpactReport
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
	if effectiveMode == model.MetadataModeLocal {
		if err := ensureGitIgnoreEntry(s.git.WorkDir, ".sophia/"); err != nil {
			return "", err
		}
	}

	return effectiveBase, nil
}

func (s *Service) AddCR(title, description string) (*model.CR, error) {
	cr, _, err := s.AddCRWithOptionsWithWarnings(title, description, AddCROptions{})
	return cr, err
}

func (s *Service) AddCRWithWarnings(title, description string) (*model.CR, []string, error) {
	return s.AddCRWithOptionsWithWarnings(title, description, AddCROptions{})
}

func (s *Service) AddCRWithOptionsWithWarnings(title, description string, opts AddCROptions) (*model.CR, []string, error) {
	if strings.TrimSpace(title) == "" {
		return nil, nil, errors.New("title cannot be empty")
	}
	if strings.TrimSpace(opts.BaseRef) != "" && opts.ParentCRID > 0 {
		return nil, nil, errors.New("--base and --parent cannot be combined")
	}
	if err := s.store.EnsureInitialized(); err != nil {
		return nil, nil, err
	}

	cfg, err := s.store.LoadConfig()
	if err != nil {
		return nil, nil, err
	}

	currentBranch, _ := s.git.CurrentBranch()
	referenceDirs := map[string]struct{}{}
	if strings.TrimSpace(currentBranch) != "" && currentBranch != cfg.BaseBranch && s.git.BranchExists(currentBranch) && s.git.BranchExists(cfg.BaseBranch) {
		files, diffErr := s.git.DiffNames(cfg.BaseBranch, currentBranch)
		if diffErr == nil {
			referenceDirs = topLevelDirs(files)
		}
	}

	if err := s.git.EnsureBaseBranch(cfg.BaseBranch); err != nil {
		return nil, nil, fmt.Errorf("ensure base branch: %w", err)
	}
	if err := s.git.EnsureBootstrapCommit("chore: bootstrap base branch for Sophia"); err != nil {
		return nil, nil, fmt.Errorf("ensure bootstrap commit: %w", err)
	}
	if err := s.ensureNextCRIDFloor(cfg.BaseBranch); err != nil {
		return nil, nil, fmt.Errorf("align cr id sequence: %w", err)
	}

	baseRef := strings.TrimSpace(opts.BaseRef)
	baseCommit := ""
	parentID := 0
	if opts.ParentCRID > 0 {
		parent, err := s.store.LoadCR(opts.ParentCRID)
		if err != nil {
			return nil, nil, err
		}
		ref, commit, err := s.parentBaseAnchor(parent)
		if err != nil {
			return nil, nil, err
		}
		baseRef = ref
		baseCommit = commit
		parentID = parent.ID
	}
	if baseRef == "" {
		baseRef = cfg.BaseBranch
	}
	if strings.TrimSpace(baseCommit) == "" {
		resolved, err := s.git.ResolveRef(baseRef)
		if err != nil {
			return nil, nil, fmt.Errorf("resolve base ref %q: %w", baseRef, err)
		}
		baseCommit = resolved
	}

	id, err := s.store.NextCRID()
	if err != nil {
		return nil, nil, err
	}
	uid, err := newCRUID()
	if err != nil {
		return nil, nil, err
	}

	branch := fmt.Sprintf("sophia/cr-%d", id)
	if s.git.BranchExists(branch) {
		return nil, nil, fmt.Errorf("branch %q already exists", branch)
	}
	if err := s.git.CreateBranchFrom(branch, baseCommit); err != nil {
		return nil, nil, err
	}

	now := s.timestamp()
	actor := s.git.Actor()
	cr := &model.CR{
		ID:          id,
		UID:         uid,
		Title:       title,
		Description: description,
		Status:      model.StatusInProgress,
		BaseBranch:  cfg.BaseBranch,
		BaseRef:     baseRef,
		BaseCommit:  baseCommit,
		ParentCRID:  parentID,
		Branch:      branch,
		Notes:       []string{},
		Subtasks:    []model.Subtask{},
		Events: []model.Event{
			{
				TS:      now,
				Actor:   actor,
				Type:    "cr_created",
				Summary: fmt.Sprintf("Created CR %d", id),
				Ref:     fmt.Sprintf("cr:%d", id),
			},
		},
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := s.store.SaveCR(cr); err != nil {
		return nil, nil, err
	}

	warnings := s.computeOverlapWarnings(referenceDirs, cr.ID)
	return cr, warnings, nil
}

func (s *Service) ListCRs() ([]model.CR, error) {
	crs, err := s.store.ListCRs()
	if err != nil {
		return nil, err
	}
	sort.Slice(crs, func(i, j int) bool {
		return crs[i].ID < crs[j].ID
	})
	return crs, nil
}

func (s *Service) AddNote(id int, note string) error {
	if strings.TrimSpace(note) == "" {
		return errors.New("note cannot be empty")
	}
	cr, err := s.store.LoadCR(id)
	if err != nil {
		return err
	}
	now := s.timestamp()
	actor := s.git.Actor()
	cr.Notes = append(cr.Notes, note)
	cr.Events = append(cr.Events, model.Event{
		TS:      now,
		Actor:   actor,
		Type:    "note_added",
		Summary: note,
		Ref:     fmt.Sprintf("cr:%d", id),
	})
	cr.UpdatedAt = now
	return s.store.SaveCR(cr)
}

func (s *Service) EditCR(id int, newTitle, newDescription *string) ([]string, error) {
	cr, err := s.store.LoadCR(id)
	if err != nil {
		return nil, err
	}

	changedFields := make([]string, 0, 2)
	if newTitle != nil && cr.Title != *newTitle {
		cr.Title = *newTitle
		changedFields = append(changedFields, "title")
	}
	if newDescription != nil && cr.Description != *newDescription {
		cr.Description = *newDescription
		changedFields = append(changedFields, "description")
	}
	if len(changedFields) == 0 {
		return nil, ErrNoCRChanges
	}

	now := s.timestamp()
	actor := s.git.Actor()
	cr.UpdatedAt = now
	cr.Events = append(cr.Events, model.Event{
		TS:      now,
		Actor:   actor,
		Type:    "cr_amended",
		Summary: fmt.Sprintf("Amended CR fields: %s", strings.Join(changedFields, ",")),
		Ref:     fmt.Sprintf("cr:%d", id),
		Meta: map[string]string{
			"fields": strings.Join(changedFields, ","),
		},
	})
	if err := s.store.SaveCR(cr); err != nil {
		return nil, err
	}
	return changedFields, nil
}

func (s *Service) SetCRContract(id int, patch ContractPatch) ([]string, error) {
	cr, err := s.store.LoadCR(id)
	if err != nil {
		return nil, err
	}
	changed := []string{}
	if patch.Why != nil {
		if cr.Contract.Why != strings.TrimSpace(*patch.Why) {
			cr.Contract.Why = strings.TrimSpace(*patch.Why)
			changed = append(changed, "why")
		}
	}
	if patch.Scope != nil {
		scope, scopeErr := s.normalizeContractScopePrefixes(*patch.Scope)
		if scopeErr != nil {
			return nil, scopeErr
		}
		if !equalStringSlices(cr.Contract.Scope, scope) {
			cr.Contract.Scope = scope
			changed = append(changed, "scope")
		}
	}
	if patch.NonGoals != nil {
		normalized := normalizeNonEmptyStringList(*patch.NonGoals)
		if !equalStringSlices(cr.Contract.NonGoals, normalized) {
			cr.Contract.NonGoals = normalized
			changed = append(changed, "non_goals")
		}
	}
	if patch.Invariants != nil {
		normalized := normalizeNonEmptyStringList(*patch.Invariants)
		if !equalStringSlices(cr.Contract.Invariants, normalized) {
			cr.Contract.Invariants = normalized
			changed = append(changed, "invariants")
		}
	}
	if patch.BlastRadius != nil {
		normalized := strings.TrimSpace(*patch.BlastRadius)
		if cr.Contract.BlastRadius != normalized {
			cr.Contract.BlastRadius = normalized
			changed = append(changed, "blast_radius")
		}
	}
	if patch.TestPlan != nil {
		normalized := strings.TrimSpace(*patch.TestPlan)
		if cr.Contract.TestPlan != normalized {
			cr.Contract.TestPlan = normalized
			changed = append(changed, "test_plan")
		}
	}
	if patch.RollbackPlan != nil {
		normalized := strings.TrimSpace(*patch.RollbackPlan)
		if cr.Contract.RollbackPlan != normalized {
			cr.Contract.RollbackPlan = normalized
			changed = append(changed, "rollback_plan")
		}
	}
	if len(changed) == 0 {
		return nil, ErrNoCRChanges
	}

	now := s.timestamp()
	actor := s.git.Actor()
	cr.Contract.UpdatedAt = now
	cr.Contract.UpdatedBy = actor
	cr.UpdatedAt = now
	cr.Events = append(cr.Events, model.Event{
		TS:      now,
		Actor:   actor,
		Type:    "contract_updated",
		Summary: fmt.Sprintf("Updated contract fields: %s", strings.Join(changed, ",")),
		Ref:     fmt.Sprintf("cr:%d", id),
		Meta: map[string]string{
			"fields": strings.Join(changed, ","),
		},
	})
	if err := s.store.SaveCR(cr); err != nil {
		return nil, err
	}
	return changed, nil
}

func (s *Service) GetCRContract(id int) (*model.Contract, error) {
	cr, err := s.store.LoadCR(id)
	if err != nil {
		return nil, err
	}
	contract := cr.Contract
	contract.Scope = append([]string(nil), contract.Scope...)
	contract.NonGoals = append([]string(nil), contract.NonGoals...)
	contract.Invariants = append([]string(nil), contract.Invariants...)
	return &contract, nil
}

func (s *Service) SetCRBase(id int, ref string, rebase bool) (*model.CR, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return nil, errors.New("base ref cannot be empty")
	}
	cr, err := s.store.LoadCR(id)
	if err != nil {
		return nil, err
	}
	if cr.Status != model.StatusInProgress {
		return nil, fmt.Errorf("cr %d is not in progress", id)
	}
	baseCommit, err := s.git.ResolveRef(ref)
	if err != nil {
		return nil, fmt.Errorf("resolve base ref %q: %w", ref, err)
	}
	if rebase {
		if dirty, summary, err := s.workingTreeDirtySummary(); err != nil {
			return nil, err
		} else if dirty {
			return nil, fmt.Errorf("%w: %s", ErrWorkingTreeDirty, summary)
		}
		if !s.git.BranchExists(cr.Branch) {
			return nil, fmt.Errorf("cr branch %q does not exist", cr.Branch)
		}
		if err := s.git.RebaseBranchOnto(cr.Branch, ref); err != nil {
			return nil, err
		}
	}

	now := s.timestamp()
	actor := s.git.Actor()
	cr.BaseRef = ref
	cr.BaseCommit = strings.TrimSpace(baseCommit)
	cr.ParentCRID = 0
	cr.UpdatedAt = now
	cr.Events = append(cr.Events, model.Event{
		TS:      now,
		Actor:   actor,
		Type:    "cr_base_updated",
		Summary: fmt.Sprintf("Updated CR base to %s", ref),
		Ref:     fmt.Sprintf("cr:%d", cr.ID),
		Meta: map[string]string{
			"base_ref":    cr.BaseRef,
			"base_commit": cr.BaseCommit,
			"rebase":      strconv.FormatBool(rebase),
		},
	})
	if err := s.store.SaveCR(cr); err != nil {
		return nil, err
	}
	return cr, nil
}

func (s *Service) RestackCR(id int) (*model.CR, error) {
	cr, err := s.store.LoadCR(id)
	if err != nil {
		return nil, err
	}
	if cr.Status != model.StatusInProgress {
		return nil, fmt.Errorf("cr %d is not in progress", id)
	}
	if cr.ParentCRID <= 0 {
		return nil, ErrParentCRRequired
	}
	if dirty, summary, err := s.workingTreeDirtySummary(); err != nil {
		return nil, err
	} else if dirty {
		return nil, fmt.Errorf("%w: %s", ErrWorkingTreeDirty, summary)
	}
	if !s.git.BranchExists(cr.Branch) {
		return nil, fmt.Errorf("cr branch %q does not exist", cr.Branch)
	}

	parent, err := s.store.LoadCR(cr.ParentCRID)
	if err != nil {
		return nil, err
	}
	targetRef := ""
	switch {
	case parent.Status == model.StatusInProgress && s.git.BranchExists(parent.Branch):
		targetRef = parent.Branch
	case parent.Status == model.StatusMerged && strings.TrimSpace(parent.MergedCommit) != "":
		targetRef = strings.TrimSpace(parent.MergedCommit)
	default:
		return nil, fmt.Errorf("parent CR %d has no restack anchor", parent.ID)
	}

	if err := s.git.RebaseBranchOnto(cr.Branch, targetRef); err != nil {
		return nil, err
	}
	targetCommit, err := s.git.ResolveRef(targetRef)
	if err != nil {
		return nil, err
	}

	cr.BaseCommit = strings.TrimSpace(targetCommit)
	if parent.Status == model.StatusMerged {
		cr.BaseRef = cr.BaseBranch
	} else {
		cr.BaseRef = parent.Branch
	}
	now := s.timestamp()
	cr.UpdatedAt = now
	cr.Events = append(cr.Events, model.Event{
		TS:      now,
		Actor:   s.git.Actor(),
		Type:    "cr_restacked",
		Summary: fmt.Sprintf("Restacked CR %d onto parent CR %d", cr.ID, parent.ID),
		Ref:     fmt.Sprintf("cr:%d", cr.ID),
		Meta: map[string]string{
			"parent_cr":   strconv.Itoa(parent.ID),
			"target_ref":  targetRef,
			"base_ref":    cr.BaseRef,
			"base_commit": cr.BaseCommit,
		},
	})
	if err := s.store.SaveCR(cr); err != nil {
		return nil, err
	}
	return cr, nil
}

func (s *Service) WhyCR(id int) (*WhyView, error) {
	cr, err := s.store.LoadCR(id)
	if err != nil {
		return nil, err
	}
	if _, err := s.ensureCRBaseFields(cr, true); err != nil {
		return nil, err
	}

	description := strings.TrimSpace(cr.Description)
	contractWhy := strings.TrimSpace(cr.Contract.Why)
	effectiveWhy := ""
	source := "missing"
	switch {
	case contractWhy != "":
		effectiveWhy = contractWhy
		source = "contract_why"
	case description != "":
		effectiveWhy = description
		source = "description"
	}

	return &WhyView{
		CRID:              cr.ID,
		CRUID:             strings.TrimSpace(cr.UID),
		BaseRef:           strings.TrimSpace(cr.BaseRef),
		BaseCommit:        strings.TrimSpace(cr.BaseCommit),
		ParentCRID:        cr.ParentCRID,
		EffectiveWhy:      effectiveWhy,
		Source:            source,
		Description:       description,
		ContractWhy:       contractWhy,
		ContractUpdatedAt: strings.TrimSpace(cr.Contract.UpdatedAt),
		ContractUpdatedBy: strings.TrimSpace(cr.Contract.UpdatedBy),
	}, nil
}

func (s *Service) StatusCR(id int) (*CRStatusView, error) {
	cr, err := s.store.LoadCR(id)
	if err != nil {
		return nil, err
	}
	if _, err := s.ensureCRBaseFields(cr, true); err != nil {
		return nil, err
	}

	currentBranch, _ := s.git.CurrentBranch()
	statusEntries, err := s.git.WorkingTreeStatus()
	if err != nil {
		return nil, err
	}

	modifiedStagedCount := 0
	untrackedCount := 0
	for _, entry := range statusEntries {
		if entry.Code == "??" {
			untrackedCount++
			continue
		}
		modifiedStagedCount++
	}

	tasksOpen := 0
	tasksDone := 0
	for _, task := range cr.Subtasks {
		if task.Status == model.TaskStatusDone {
			tasksDone++
			continue
		}
		tasksOpen++
	}

	missingFields := missingCRContractFields(cr.Contract)
	view := &CRStatusView{
		ID:                    cr.ID,
		UID:                   strings.TrimSpace(cr.UID),
		Title:                 cr.Title,
		Status:                cr.Status,
		BaseBranch:            cr.BaseBranch,
		BaseRef:               strings.TrimSpace(cr.BaseRef),
		BaseCommit:            strings.TrimSpace(cr.BaseCommit),
		ParentCRID:            cr.ParentCRID,
		Branch:                cr.Branch,
		CurrentBranch:         currentBranch,
		BranchMatch:           strings.TrimSpace(currentBranch) != "" && currentBranch == cr.Branch,
		ModifiedStagedCount:   modifiedStagedCount,
		UntrackedCount:        untrackedCount,
		Dirty:                 modifiedStagedCount > 0 || untrackedCount > 0,
		TasksTotal:            len(cr.Subtasks),
		TasksOpen:             tasksOpen,
		TasksDone:             tasksDone,
		ContractComplete:      len(missingFields) == 0,
		ContractMissingFields: missingFields,
		ValidationValid:       true,
		RiskTier:              "-",
	}
	if cr.ParentCRID > 0 {
		parent, parentErr := s.store.LoadCR(cr.ParentCRID)
		if parentErr != nil {
			view.ParentStatus = "missing"
		} else {
			view.ParentStatus = parent.Status
		}
	}

	if cr.Status == model.StatusInProgress {
		report, validateErr := s.ValidateCR(id)
		if validateErr != nil {
			return nil, validateErr
		}
		view.ValidationValid = report.Valid
		view.ValidationErrors = len(report.Errors)
		view.ValidationWarnings = len(report.Warnings)
		view.MergeBlocked = !report.Valid
		if report.Impact != nil {
			view.RiskTier = nonEmptyTrimmed(report.Impact.RiskTier, "-")
			view.RiskScore = report.Impact.RiskScore
		}
	}

	return view, nil
}

func (s *Service) ImpactCR(id int) (*ImpactReport, error) {
	cr, err := s.store.LoadCR(id)
	if err != nil {
		return nil, err
	}
	diff, err := s.summarizeCRDiff(cr)
	if err != nil {
		return nil, err
	}
	impact := buildImpactReport(cr, diff)
	return impact, nil
}

func (s *Service) ValidateCR(id int) (*ValidationReport, error) {
	cr, err := s.store.LoadCR(id)
	if err != nil {
		return nil, err
	}
	diff, err := s.summarizeCRDiff(cr)
	if err != nil {
		return nil, err
	}
	impact := buildImpactReport(cr, diff)

	errorsOut := make([]string, 0)
	for _, field := range missingCRContractFields(cr.Contract) {
		errorsOut = append(errorsOut, fmt.Sprintf("missing required contract field: %s", field))
	}
	for _, driftPath := range impact.ScopeDrift {
		errorsOut = append(errorsOut, fmt.Sprintf("scope drift: changed path %q is outside declared contract scope", driftPath))
	}

	warnings := append([]string(nil), impact.TaskScopeWarnings...)
	warnings = append(warnings, impact.TaskContractWarnings...)
	return &ValidationReport{
		Valid:    len(errorsOut) == 0,
		Errors:   errorsOut,
		Warnings: warnings,
		Impact:   impact,
	}, nil
}

func (s *Service) RecordCRValidation(id int, report *ValidationReport) error {
	if report == nil {
		return errors.New("validation report is required")
	}
	cr, err := s.store.LoadCR(id)
	if err != nil {
		return err
	}
	now := s.timestamp()
	actor := s.git.Actor()
	status := "passed"
	if !report.Valid {
		status = "failed"
	}
	meta := map[string]string{
		"risk_tier":           "-",
		"validation_errors":   strconv.Itoa(len(report.Errors)),
		"validation_warnings": strconv.Itoa(len(report.Warnings)),
	}
	if report.Impact != nil {
		meta["risk_tier"] = nonEmptyTrimmed(report.Impact.RiskTier, "-")
	}
	cr.UpdatedAt = now
	cr.Events = append(cr.Events, model.Event{
		TS:      now,
		Actor:   actor,
		Type:    "cr_validated",
		Summary: fmt.Sprintf("Validation %s with %d error(s) and %d warning(s)", status, len(report.Errors), len(report.Warnings)),
		Ref:     fmt.Sprintf("cr:%d", id),
		Meta:    meta,
	})
	return s.store.SaveCR(cr)
}

func (s *Service) RedactCRNote(id, noteIndex int, reason string) error {
	if strings.TrimSpace(reason) == "" {
		return errors.New("redaction reason cannot be empty")
	}
	cr, err := s.store.LoadCR(id)
	if err != nil {
		return err
	}

	idx, err := oneBasedIndex(noteIndex, len(cr.Notes), "note")
	if err != nil {
		return err
	}
	if cr.Notes[idx] == redactedPlaceholder {
		return ErrAlreadyRedacted
	}
	cr.Notes[idx] = redactedPlaceholder

	now := s.timestamp()
	actor := s.git.Actor()
	cr.UpdatedAt = now
	cr.Events = append(cr.Events, model.Event{
		TS:              now,
		Actor:           actor,
		Type:            "cr_redacted",
		Summary:         fmt.Sprintf("Redacted note #%d", noteIndex),
		Ref:             fmt.Sprintf("note:%d", noteIndex),
		RedactionReason: reason,
		Meta: map[string]string{
			"target": fmt.Sprintf("note:%d", noteIndex),
			"reason": reason,
		},
	})
	return s.store.SaveCR(cr)
}

func (s *Service) RedactCREvent(id, eventIndex int, reason string) error {
	if strings.TrimSpace(reason) == "" {
		return errors.New("redaction reason cannot be empty")
	}
	cr, err := s.store.LoadCR(id)
	if err != nil {
		return err
	}

	idx, err := oneBasedIndex(eventIndex, len(cr.Events), "event")
	if err != nil {
		return err
	}
	if cr.Events[idx].Redacted || cr.Events[idx].Summary == redactedPlaceholder {
		return ErrAlreadyRedacted
	}

	cr.Events[idx].Summary = redactedPlaceholder
	cr.Events[idx].Redacted = true
	cr.Events[idx].RedactionReason = reason
	if cr.Events[idx].Meta == nil {
		cr.Events[idx].Meta = map[string]string{}
	}
	cr.Events[idx].Meta["redacted_via"] = fmt.Sprintf("event:%d", eventIndex)

	now := s.timestamp()
	actor := s.git.Actor()
	cr.UpdatedAt = now
	cr.Events = append(cr.Events, model.Event{
		TS:              now,
		Actor:           actor,
		Type:            "cr_redacted",
		Summary:         fmt.Sprintf("Redacted event #%d", eventIndex),
		Ref:             fmt.Sprintf("event:%d", eventIndex),
		RedactionReason: reason,
		Meta: map[string]string{
			"target": fmt.Sprintf("event:%d", eventIndex),
			"reason": reason,
		},
	})
	return s.store.SaveCR(cr)
}

func (s *Service) HistoryCR(id int, showRedacted bool) (*CRHistory, error) {
	cr, err := s.store.LoadCR(id)
	if err != nil {
		return nil, err
	}

	history := &CRHistory{
		CRID:        cr.ID,
		Title:       cr.Title,
		Status:      cr.Status,
		Description: cr.Description,
		Notes:       make([]HistoryNote, 0, len(cr.Notes)),
		Events:      make([]HistoryEvent, 0, len(cr.Events)),
	}

	for i, note := range cr.Notes {
		redacted := note == redactedPlaceholder
		text := note
		if redacted {
			text = redactedPlaceholder
		}
		history.Notes = append(history.Notes, HistoryNote{
			Index:    i + 1,
			Text:     text,
			Redacted: redacted,
		})
	}

	for i, event := range cr.Events {
		summary := event.Summary
		redacted := event.Redacted || summary == redactedPlaceholder
		if redacted {
			summary = redactedPlaceholder
		}
		reason := ""
		if showRedacted {
			reason = event.RedactionReason
		}
		meta := map[string]string(nil)
		if showRedacted && len(event.Meta) > 0 {
			meta = cloneStringMap(event.Meta)
		}
		history.Events = append(history.Events, HistoryEvent{
			Index:           i + 1,
			TS:              event.TS,
			Actor:           event.Actor,
			Type:            event.Type,
			Summary:         summary,
			Ref:             event.Ref,
			Redacted:        redacted,
			RedactionReason: reason,
			Meta:            meta,
		})
	}

	return history, nil
}

func (s *Service) AddTask(crID int, title string) (*model.Subtask, error) {
	if strings.TrimSpace(title) == "" {
		return nil, errors.New("task title cannot be empty")
	}
	cr, err := s.store.LoadCR(crID)
	if err != nil {
		return nil, err
	}
	newTaskID := nextTaskID(cr.Subtasks)
	now := s.timestamp()
	actor := s.git.Actor()
	task := model.Subtask{
		ID:        newTaskID,
		Title:     title,
		Status:    model.TaskStatusOpen,
		CreatedAt: now,
		UpdatedAt: now,
		CreatedBy: actor,
	}
	cr.Subtasks = append(cr.Subtasks, task)
	cr.Events = append(cr.Events, model.Event{
		TS:      now,
		Actor:   actor,
		Type:    "task_added",
		Summary: title,
		Ref:     fmt.Sprintf("task:%d", newTaskID),
	})
	cr.UpdatedAt = now
	if err := s.store.SaveCR(cr); err != nil {
		return nil, err
	}
	return &task, nil
}

func (s *Service) SetTaskContract(crID, taskID int, patch TaskContractPatch) ([]string, error) {
	cr, err := s.store.LoadCR(crID)
	if err != nil {
		return nil, err
	}
	taskIndex := indexOfTask(cr.Subtasks, taskID)
	if taskIndex < 0 {
		return nil, fmt.Errorf("task %d not found in cr %d", taskID, crID)
	}
	task := &cr.Subtasks[taskIndex]

	changed := []string{}
	if patch.Intent != nil {
		normalized := strings.TrimSpace(*patch.Intent)
		if task.Contract.Intent != normalized {
			task.Contract.Intent = normalized
			changed = append(changed, "intent")
		}
	}
	if patch.AcceptanceCriteria != nil {
		normalized := normalizeNonEmptyStringList(*patch.AcceptanceCriteria)
		if !equalStringSlices(task.Contract.AcceptanceCriteria, normalized) {
			task.Contract.AcceptanceCriteria = normalized
			changed = append(changed, "acceptance_criteria")
		}
	}
	if patch.Scope != nil {
		normalized, normalizeErr := s.normalizeContractScopePrefixes(*patch.Scope)
		if normalizeErr != nil {
			return nil, normalizeErr
		}
		if !equalStringSlices(task.Contract.Scope, normalized) {
			task.Contract.Scope = normalized
			changed = append(changed, "scope")
		}
	}
	if len(changed) == 0 {
		return nil, ErrNoCRChanges
	}

	now := s.timestamp()
	actor := s.git.Actor()
	task.Contract.UpdatedAt = now
	task.Contract.UpdatedBy = actor
	task.UpdatedAt = now
	cr.UpdatedAt = now
	cr.Events = append(cr.Events, model.Event{
		TS:      now,
		Actor:   actor,
		Type:    "task_contract_updated",
		Summary: fmt.Sprintf("Updated task %d contract fields: %s", taskID, strings.Join(changed, ",")),
		Ref:     fmt.Sprintf("task:%d", taskID),
		Meta: map[string]string{
			"fields": strings.Join(changed, ","),
		},
	})
	if err := s.store.SaveCR(cr); err != nil {
		return nil, err
	}
	return changed, nil
}

func (s *Service) GetTaskContract(crID, taskID int) (*model.TaskContract, error) {
	cr, err := s.store.LoadCR(crID)
	if err != nil {
		return nil, err
	}
	taskIndex := indexOfTask(cr.Subtasks, taskID)
	if taskIndex < 0 {
		return nil, fmt.Errorf("task %d not found in cr %d", taskID, crID)
	}
	contract := cr.Subtasks[taskIndex].Contract
	contract.AcceptanceCriteria = append([]string(nil), contract.AcceptanceCriteria...)
	contract.Scope = append([]string(nil), contract.Scope...)
	return &contract, nil
}

func (s *Service) ListTasks(crID int) ([]model.Subtask, error) {
	cr, err := s.store.LoadCR(crID)
	if err != nil {
		return nil, err
	}
	tasks := append([]model.Subtask(nil), cr.Subtasks...)
	sort.Slice(tasks, func(i, j int) bool {
		return tasks[i].ID < tasks[j].ID
	})
	return tasks, nil
}

func (s *Service) ListTaskChunks(crID, taskID int, paths []string) ([]TaskChunk, error) {
	cr, err := s.store.LoadCR(crID)
	if err != nil {
		return nil, err
	}
	if cr.Status != model.StatusInProgress {
		return nil, fmt.Errorf("cr %d is not in progress", crID)
	}
	if indexOfTask(cr.Subtasks, taskID) < 0 {
		return nil, fmt.Errorf("task %d not found in cr %d", taskID, crID)
	}
	currentBranch, branchErr := s.git.CurrentBranch()
	if branchErr != nil {
		return nil, branchErr
	}
	if currentBranch != cr.Branch {
		return nil, fmt.Errorf("chunk list requires active CR branch %q, current branch is %q", cr.Branch, currentBranch)
	}
	normalizedPaths := []string{}
	if len(paths) > 0 {
		normalized, normalizeErr := s.normalizeTaskScopePaths(paths)
		if normalizeErr != nil {
			return nil, normalizeErr
		}
		normalizedPaths = normalized
	}
	diff, err := s.git.WorkingTreeUnifiedDiff(normalizedPaths, 0)
	if err != nil {
		return nil, err
	}
	parsed, err := parsePatchChunks(diff)
	if err != nil {
		return nil, fmt.Errorf("parse working tree diff chunks: %w", err)
	}
	chunks := make([]TaskChunk, 0, len(parsed))
	for _, chunk := range parsed {
		chunks = append(chunks, TaskChunk{
			ID:       chunk.ID,
			Path:     chunk.Path,
			OldStart: chunk.OldStart,
			OldLines: chunk.OldLines,
			NewStart: chunk.NewStart,
			NewLines: chunk.NewLines,
			Preview:  chunk.Preview,
		})
	}
	sort.Slice(chunks, func(i, j int) bool {
		if chunks[i].Path != chunks[j].Path {
			return chunks[i].Path < chunks[j].Path
		}
		if chunks[i].OldStart != chunks[j].OldStart {
			return chunks[i].OldStart < chunks[j].OldStart
		}
		if chunks[i].NewStart != chunks[j].NewStart {
			return chunks[i].NewStart < chunks[j].NewStart
		}
		return chunks[i].ID < chunks[j].ID
	})
	return chunks, nil
}

func (s *Service) DoneTask(crID, taskID int) error {
	_, err := s.DoneTaskWithCheckpoint(crID, taskID, DoneTaskOptions{Checkpoint: false})
	return err
}

func (s *Service) DoneTaskWithCheckpoint(crID, taskID int, opts DoneTaskOptions) (string, error) {
	cr, err := s.store.LoadCR(crID)
	if err != nil {
		return "", err
	}
	if _, err := ensureCRUID(cr); err != nil {
		return "", err
	}
	if _, err := s.ensureCRBaseFields(cr, true); err != nil {
		return "", err
	}
	if cr.Status != model.StatusInProgress {
		return "", fmt.Errorf("cr %d is not in progress", crID)
	}
	if err := validateDoneTaskOptions(opts); err != nil {
		return "", err
	}

	now := s.timestamp()
	actor := s.git.Actor()
	found := false
	title := ""
	taskIndex := -1
	for i := range cr.Subtasks {
		if cr.Subtasks[i].ID == taskID {
			found = true
			title = cr.Subtasks[i].Title
			taskIndex = i
			break
		}
	}
	if !found {
		return "", fmt.Errorf("task %d not found in cr %d", taskID, crID)
	}
	missingContractFields := missingTaskContractFields(cr.Subtasks[taskIndex].Contract)
	if len(missingContractFields) > 0 {
		return "", fmt.Errorf("%w: task %d missing %s", ErrTaskContractIncomplete, taskID, strings.Join(missingContractFields, ","))
	}

	commitSHA := ""
	if opts.Checkpoint {
		currentBranch, branchErr := s.git.CurrentBranch()
		if branchErr != nil {
			return "", branchErr
		}
		if currentBranch != cr.Branch {
			return "", fmt.Errorf("checkpoint requires active CR branch %q, current branch is %q", cr.Branch, currentBranch)
		}

		preStaged, stagedErr := s.git.HasStagedChanges()
		if stagedErr != nil {
			return "", stagedErr
		}
		if preStaged {
			return "", fmt.Errorf("%w: unstage changes before running task checkpoint", ErrPreStagedChanges)
		}

		checkpointScope := []string{}
		checkpointChunks := []model.CheckpointChunk{}
		scopeMode := ""
		if opts.StageAll {
			dirty, _, dirtyErr := s.workingTreeDirtySummary()
			if dirtyErr != nil {
				return "", dirtyErr
			}
			if !dirty {
				return "", fmt.Errorf("%w: task %d has no working tree changes", ErrNoTaskChanges, taskID)
			}
			if err := s.git.StageAll(); err != nil {
				return "", err
			}
			checkpointScope = []string{"*"}
			scopeMode = "all"
		} else if opts.FromContract {
			normalizedScope, normalizeErr := s.normalizeContractScopePrefixes(cr.Subtasks[taskIndex].Contract.Scope)
			if normalizeErr != nil {
				return "", normalizeErr
			}
			paths, resolveErr := s.resolveTaskCheckpointPathsFromContract(normalizedScope)
			if resolveErr != nil {
				return "", resolveErr
			}
			if err := s.git.StagePaths(paths); err != nil {
				return "", err
			}
			checkpointScope = paths
			scopeMode = "task_contract"
		} else if strings.TrimSpace(opts.PatchFile) != "" {
			patchPath, pathErr := s.normalizePatchFilePath(opts.PatchFile)
			if pathErr != nil {
				return "", pathErr
			}
			patchContent, readErr := os.ReadFile(patchPath)
			if readErr != nil {
				return "", fmt.Errorf("read patch file %q: %w", opts.PatchFile, readErr)
			}
			parsedChunks, parseErr := parsePatchChunks(string(patchContent))
			if parseErr != nil {
				return "", fmt.Errorf("%w: parse patch file: %v", ErrInvalidTaskScope, parseErr)
			}
			if len(parsedChunks) == 0 {
				return "", fmt.Errorf("%w: patch file %q contains no hunks", ErrNoTaskChanges, opts.PatchFile)
			}
			if err := s.git.ApplyPatchToIndex(patchPath); err != nil {
				return "", err
			}
			checkpointScope = checkpointChunkPaths(parsedChunks)
			checkpointChunks = make([]model.CheckpointChunk, 0, len(parsedChunks))
			for _, chunk := range parsedChunks {
				checkpointChunks = append(checkpointChunks, model.CheckpointChunk{
					ID:       chunk.ID,
					Path:     chunk.Path,
					OldStart: chunk.OldStart,
					OldLines: chunk.OldLines,
					NewStart: chunk.NewStart,
					NewLines: chunk.NewLines,
				})
			}
			scopeMode = "patch_manifest"
		} else {
			normalizedPaths, normalizeErr := s.normalizeTaskScopePaths(opts.Paths)
			if normalizeErr != nil {
				return "", normalizeErr
			}

			changedPathFound := false
			for _, scopePath := range normalizedPaths {
				hasChanges, hasErr := s.git.PathHasChanges(scopePath)
				if hasErr != nil {
					return "", hasErr
				}
				if hasChanges {
					changedPathFound = true
					break
				}
			}
			if !changedPathFound {
				return "", fmt.Errorf("%w: none of the scoped paths have changes", ErrNoTaskChanges)
			}
			if err := s.git.StagePaths(normalizedPaths); err != nil {
				return "", err
			}
			checkpointScope = normalizedPaths
			scopeMode = "path"
		}

		hasStaged, stagedErr := s.git.HasStagedChanges()
		if stagedErr != nil {
			return "", stagedErr
		}
		if !hasStaged {
			return "", fmt.Errorf("%w: no staged changes after applying scope", ErrNoTaskChanges)
		}

		commitMessage := buildTaskCheckpointMessage(cr, &cr.Subtasks[taskIndex], scopeMode, len(checkpointChunks))
		if err := s.git.Commit(commitMessage); err != nil {
			return "", err
		}
		sha, shaErr := s.git.HeadShortSHA()
		if shaErr != nil {
			return "", shaErr
		}
		commitSHA = sha
		cr.Subtasks[taskIndex].CheckpointCommit = sha
		cr.Subtasks[taskIndex].CheckpointAt = now
		cr.Subtasks[taskIndex].CheckpointMessage = commitMessage
		cr.Subtasks[taskIndex].CheckpointScope = append([]string(nil), checkpointScope...)
		cr.Subtasks[taskIndex].CheckpointChunks = append([]model.CheckpointChunk(nil), checkpointChunks...)
		cr.Events = append(cr.Events, model.Event{
			TS:      now,
			Actor:   actor,
			Type:    "task_checkpointed",
			Summary: fmt.Sprintf("Checkpointed task %d as %s", taskID, sha),
			Ref:     fmt.Sprintf("task:%d", taskID),
			Meta: map[string]string{
				"commit":  sha,
				"message": strings.SplitN(commitMessage, "\n", 2)[0],
				"scope":   strings.Join(checkpointScope, ","),
			},
		})
		if opts.FromContract {
			cr.Events[len(cr.Events)-1].Meta["scope_source"] = "task_contract"
		}
		if strings.TrimSpace(opts.PatchFile) != "" {
			cr.Events[len(cr.Events)-1].Meta["scope_source"] = "patch_manifest"
			cr.Events[len(cr.Events)-1].Meta["chunk_count"] = strconv.Itoa(len(checkpointChunks))
		}
	}

	cr.Subtasks[taskIndex].Status = model.TaskStatusDone
	cr.Subtasks[taskIndex].UpdatedAt = now
	cr.Subtasks[taskIndex].CompletedAt = now
	cr.Subtasks[taskIndex].CompletedBy = actor

	cr.Events = append(cr.Events, model.Event{
		TS:      now,
		Actor:   actor,
		Type:    "task_done",
		Summary: title,
		Ref:     fmt.Sprintf("task:%d", taskID),
	})
	cr.UpdatedAt = now

	if err := s.store.SaveCR(cr); err != nil {
		return "", err
	}
	return commitSHA, nil
}

func (s *Service) ReviewCR(id int) (*Review, error) {
	cr, err := s.store.LoadCR(id)
	if err != nil {
		return nil, err
	}
	diff, err := s.summarizeCRDiff(cr)
	if err != nil {
		return nil, err
	}
	validation, err := s.ValidateCR(id)
	if err != nil {
		return nil, err
	}

	return &Review{
		CR:                 cr,
		Contract:           cr.Contract,
		Impact:             validation.Impact,
		ValidationErrors:   append([]string(nil), validation.Errors...),
		ValidationWarnings: append([]string(nil), validation.Warnings...),
		Files:              diff.Files,
		ShortStat:          diff.ShortStat,
		NewFiles:           diff.NewFiles,
		ModifiedFiles:      diff.ModifiedFiles,
		DeletedFiles:       diff.DeletedFiles,
		TestFiles:          diff.TestFiles,
		DependencyFiles:    diff.DependencyFiles,
	}, nil
}

func (s *Service) MergeCR(id int, keepBranch bool, overrideReason string) (string, error) {
	cr, err := s.store.LoadCR(id)
	if err != nil {
		return "", err
	}
	if _, err := ensureCRUID(cr); err != nil {
		return "", err
	}
	if _, err := s.ensureCRBaseFields(cr, true); err != nil {
		return "", err
	}
	if cr.Status == model.StatusMerged {
		return "", ErrCRAlreadyMerged
	}
	validation, err := s.ValidateCR(id)
	if err != nil {
		return "", err
	}
	overrideReason = strings.TrimSpace(overrideReason)
	if cr.ParentCRID > 0 {
		parent, parentErr := s.store.LoadCR(cr.ParentCRID)
		if parentErr != nil {
			return "", fmt.Errorf("parent cr %d not found: %w", cr.ParentCRID, parentErr)
		}
		if parent.Status != model.StatusMerged && overrideReason == "" {
			return "", fmt.Errorf("%w: CR %d depends on parent CR %d (%s)", ErrParentCRNotMerged, cr.ID, parent.ID, parent.Status)
		}
	}
	if !validation.Valid && overrideReason == "" {
		return "", fmt.Errorf("%w: %s", ErrCRValidationFailed, strings.Join(validation.Errors, "; "))
	}
	if dirty, summary, err := s.workingTreeDirtySummary(); err != nil {
		return "", err
	} else if dirty {
		return "", fmt.Errorf("%w: %s", ErrWorkingTreeDirty, summary)
	}
	if !s.git.BranchExists(cr.BaseBranch) {
		return "", fmt.Errorf("base branch %q does not exist", cr.BaseBranch)
	}
	if !s.git.BranchExists(cr.Branch) {
		return "", fmt.Errorf("cr branch %q does not exist", cr.Branch)
	}

	files, err := s.diffNamesForCR(cr)
	if err != nil {
		return "", err
	}

	actor := s.git.Actor()
	mergedAt := s.timestamp()
	msg := buildMergeCommitMessage(cr, actor, mergedAt)
	if err := s.git.MergeNoFF(cr.BaseBranch, cr.Branch, msg); err != nil {
		return "", err
	}

	if !keepBranch {
		if err := s.git.DeleteBranch(cr.Branch, true); err != nil {
			return "", err
		}
	}

	sha, err := s.git.HeadShortSHA()
	if err != nil {
		return "", err
	}

	cr.Status = model.StatusMerged
	cr.UpdatedAt = mergedAt
	cr.MergedAt = mergedAt
	cr.MergedBy = actor
	cr.MergedCommit = sha
	cr.FilesTouchedCount = len(files)
	if overrideReason != "" {
		cr.Events = append(cr.Events, model.Event{
			TS:      mergedAt,
			Actor:   actor,
			Type:    "cr_merge_overridden",
			Summary: fmt.Sprintf("Merged with validation override: %s", overrideReason),
			Ref:     fmt.Sprintf("cr:%d", cr.ID),
			Meta: map[string]string{
				"override_reason":   overrideReason,
				"risk_tier":         nonEmptyTrimmed(validation.Impact.RiskTier, "-"),
				"validation_errors": strconv.Itoa(len(validation.Errors)),
			},
		})
	}
	cr.Events = append(cr.Events, model.Event{
		TS:      mergedAt,
		Actor:   actor,
		Type:    "cr_merged",
		Summary: fmt.Sprintf("Merged CR %d", cr.ID),
		Ref:     fmt.Sprintf("cr:%d", cr.ID),
	})
	if err := s.store.SaveCR(cr); err != nil {
		return "", err
	}
	if err := s.backfillChildrenAfterParentMerge(cr); err != nil {
		return "", err
	}

	return sha, nil
}

func (s *Service) Doctor(limit int) (*DoctorReport, error) {
	if err := s.store.EnsureInitialized(); err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 100
	}
	cfg, err := s.store.LoadConfig()
	if err != nil {
		return nil, err
	}

	report := &DoctorReport{BaseBranch: cfg.BaseBranch, Findings: []DoctorFinding{}}
	branch, err := s.git.CurrentBranch()
	if err == nil {
		report.CurrentBranch = branch
		if _, ok := parseCRBranchID(branch); !ok {
			report.Findings = append(report.Findings, DoctorFinding{
				Code:    "non_cr_branch",
				Message: fmt.Sprintf("current branch %q is not a CR branch", branch),
			})
		}
	}

	statusEntries, err := s.git.WorkingTreeStatus()
	if err != nil {
		return nil, err
	}
	for _, entry := range statusEntries {
		if entry.Code == "??" {
			report.UntrackedCount++
		} else {
			report.ChangedCount++
		}
	}
	if report.UntrackedCount > 0 || report.ChangedCount > 0 {
		report.Findings = append(report.Findings, DoctorFinding{
			Code:    "dirty_worktree",
			Message: fmt.Sprintf("working tree has %d modified/staged and %d untracked paths", report.ChangedCount, report.UntrackedCount),
		})
	}

	if cfg.MetadataMode == model.MetadataModeLocal {
		trackedSophia, trackedErr := s.git.TrackedFiles(".sophia")
		if trackedErr == nil && len(trackedSophia) > 0 {
			report.Findings = append(report.Findings, DoctorFinding{
				Code:    "tracked_sophia_metadata",
				Message: fmt.Sprintf("%d tracked path(s) found under .sophia in local metadata mode", len(trackedSophia)),
			})
		}
	}

	crs, err := s.store.ListCRs()
	if err != nil {
		return nil, err
	}
	stale := make([]string, 0)
	for _, cr := range crs {
		if cr.Status == model.StatusMerged && s.git.BranchExists(cr.Branch) {
			stale = append(stale, cr.Branch)
		}
	}
	if len(stale) > 0 {
		preview := stale
		if len(preview) > 5 {
			preview = preview[:5]
		}
		report.Findings = append(report.Findings, DoctorFinding{
			Code:    "stale_merged_branches",
			Message: fmt.Sprintf("%d merged CR branch(es) still present (latest: %s)", len(stale), strings.Join(preview, ", ")),
		})
	}

	commits, err := s.git.RecentCommits(cfg.BaseBranch, limit)
	if err != nil {
		return nil, err
	}
	report.ScannedCommits = len(commits)
	untied := make([]string, 0)
	for _, commit := range commits {
		if strings.HasPrefix(commit.Subject, "chore: bootstrap base branch for Sophia") {
			continue
		}
		if legacyPersistPattern.MatchString(strings.TrimSpace(commit.Subject)) {
			continue
		}
		if commitTiedToCR(commit.Subject, commit.Body) {
			continue
		}
		untied = append(untied, fmt.Sprintf("%s %s", shortHash(commit.Hash), commit.Subject))
	}
	if len(untied) > 0 {
		preview := untied
		if len(preview) > 5 {
			preview = preview[:5]
		}
		report.Findings = append(report.Findings, DoctorFinding{
			Code:    "untied_base_commits",
			Message: fmt.Sprintf("%d base-branch commit(s) not tied to a CR (latest: %s)", len(untied), strings.Join(preview, "; ")),
		})
	}

	return report, nil
}

func (s *Service) CurrentCR() (*CurrentCRContext, error) {
	branch, err := s.git.CurrentBranch()
	if err != nil {
		return nil, err
	}
	id, ok := parseCRBranchID(branch)
	if !ok {
		return nil, ErrNoActiveCRContext
	}
	cr, err := s.store.LoadCR(id)
	if err != nil {
		return nil, err
	}
	if _, err := s.ensureCRBaseFields(cr, true); err != nil {
		return nil, err
	}
	return &CurrentCRContext{Branch: branch, CR: cr}, nil
}

func (s *Service) SwitchCR(id int) (*model.CR, error) {
	if dirty, summary, err := s.workingTreeDirtySummary(); err != nil {
		return nil, err
	} else if dirty {
		return nil, fmt.Errorf("%w: %s", ErrWorkingTreeDirty, summary)
	}

	cr, err := s.store.LoadCR(id)
	if err != nil {
		return nil, err
	}
	if _, err := s.ensureCRBaseFields(cr, true); err != nil {
		return nil, err
	}
	if s.git.BranchExists(cr.Branch) {
		if err := s.git.CheckoutBranch(cr.Branch); err != nil {
			return nil, err
		}
		return cr, nil
	}

	if cr.Status != model.StatusInProgress {
		return nil, fmt.Errorf("branch %q is missing for merged CR %d; run sophia cr reopen %d", cr.Branch, cr.ID, cr.ID)
	}
	baseAnchor, err := s.resolveCRBaseAnchor(cr)
	if err != nil {
		return nil, err
	}
	if err := s.git.CreateBranchFrom(cr.Branch, baseAnchor); err != nil {
		return nil, err
	}
	return cr, nil
}

func (s *Service) ReopenCR(id int) (*model.CR, error) {
	if dirty, summary, err := s.workingTreeDirtySummary(); err != nil {
		return nil, err
	} else if dirty {
		return nil, fmt.Errorf("%w: %s", ErrWorkingTreeDirty, summary)
	}

	cr, err := s.store.LoadCR(id)
	if err != nil {
		return nil, err
	}
	if _, err := s.ensureCRBaseFields(cr, true); err != nil {
		return nil, err
	}
	if cr.Status != model.StatusMerged {
		return nil, fmt.Errorf("cr %d is not merged", id)
	}
	if s.git.BranchExists(cr.Branch) {
		if err := s.git.CheckoutBranch(cr.Branch); err != nil {
			return nil, err
		}
	} else {
		baseAnchor, err := s.resolveCRBaseAnchor(cr)
		if err != nil {
			return nil, err
		}
		if err := s.git.CreateBranchFrom(cr.Branch, baseAnchor); err != nil {
			return nil, err
		}
	}

	now := s.timestamp()
	actor := s.git.Actor()
	cr.Status = model.StatusInProgress
	cr.UpdatedAt = now
	cr.Events = append(cr.Events, model.Event{
		TS:      now,
		Actor:   actor,
		Type:    "cr_reopened",
		Summary: fmt.Sprintf("Reopened CR %d", cr.ID),
		Ref:     fmt.Sprintf("cr:%d", cr.ID),
	})
	if err := s.store.SaveCR(cr); err != nil {
		return nil, err
	}

	return cr, nil
}

func (s *Service) Log() ([]LogEntry, error) {
	crs, err := s.store.ListCRs()
	if err == nil && len(crs) > 0 {
		return buildLogEntriesFromCRs(crs), nil
	}
	if err != nil && !errors.Is(err, store.ErrNotInitialized) {
		return nil, err
	}
	return s.logFromGit(200)
}

func buildLogEntriesFromCRs(crs []model.CR) []LogEntry {
	type stampedEntry struct {
		entry LogEntry
		ts    time.Time
	}
	merged := make([]stampedEntry, 0)
	active := make([]stampedEntry, 0)

	for _, cr := range crs {
		if cr.Status == model.StatusMerged {
			when := cr.MergedAt
			if when == "" {
				when = cr.UpdatedAt
			}
			filesTouched := "-"
			if cr.MergedAt != "" || cr.MergedCommit != "" {
				filesTouched = strconv.Itoa(cr.FilesTouchedCount)
			}
			entry := stampedEntry{
				entry: LogEntry{
					ID:           cr.ID,
					Title:        cr.Title,
					Status:       cr.Status,
					Who:          nonEmptyTrimmed(cr.MergedBy, "-"),
					When:         when,
					FilesTouched: filesTouched,
				},
				ts: parseRFC3339OrZero(when),
			}
			merged = append(merged, entry)
			continue
		}
		entry := stampedEntry{
			entry: LogEntry{
				ID:           cr.ID,
				Title:        cr.Title,
				Status:       cr.Status,
				Who:          "-",
				When:         cr.UpdatedAt,
				FilesTouched: "-",
			},
			ts: parseRFC3339OrZero(cr.UpdatedAt),
		}
		active = append(active, entry)
	}

	sort.Slice(merged, func(i, j int) bool {
		return merged[i].ts.After(merged[j].ts)
	})
	sort.Slice(active, func(i, j int) bool {
		return active[i].ts.After(active[j].ts)
	})

	res := make([]LogEntry, 0, len(merged)+len(active))
	for _, item := range merged {
		res = append(res, item.entry)
	}
	for _, item := range active {
		res = append(res, item.entry)
	}
	return res
}

func (s *Service) logFromGit(limit int) ([]LogEntry, error) {
	branch := s.git.DefaultBranch()
	commits, err := s.git.RecentCommits(branch, limit)
	if err != nil {
		return nil, err
	}
	entries := make([]LogEntry, 0)
	seen := map[int]struct{}{}
	for _, commit := range commits {
		id, ok := crIDFromSubjectOrBody(commit.Subject, commit.Body)
		if !ok {
			continue
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}

		filesCount, countErr := s.git.ChangedFileCount(commit.Hash)
		filesTouched := "-"
		if countErr == nil {
			filesTouched = strconv.Itoa(filesCount)
		}
		entries = append(entries, LogEntry{
			ID:           id,
			Title:        titleFromSubjectOrBody(commit.Subject, commit.Body),
			Status:       model.StatusMerged,
			Who:          nonEmptyTrimmed(commit.Author, "-"),
			When:         nonEmptyTrimmed(commit.When, "-"),
			FilesTouched: filesTouched,
		})
	}
	return entries, nil
}

func (s *Service) RepairFromGit(baseBranch string, refresh bool) (*RepairReport, error) {
	targetBase := strings.TrimSpace(baseBranch)
	if targetBase == "" {
		if s.store.IsInitialized() {
			cfg, err := s.store.LoadConfig()
			if err == nil && strings.TrimSpace(cfg.BaseBranch) != "" {
				targetBase = strings.TrimSpace(cfg.BaseBranch)
			}
		}
	}
	if targetBase == "" {
		targetBase = s.git.DefaultBranch()
	}
	if !s.git.BranchExists(targetBase) {
		return nil, fmt.Errorf("base branch %q does not exist", targetBase)
	}

	mode := model.MetadataModeLocal
	if s.store.IsInitialized() {
		if cfg, err := s.store.LoadConfig(); err == nil && strings.TrimSpace(cfg.MetadataMode) != "" {
			mode = cfg.MetadataMode
		}
	}
	if err := s.store.Init(targetBase, mode); err != nil {
		return nil, err
	}

	existingMap := map[int]*model.CR{}
	if existing, err := s.store.ListCRs(); err == nil {
		for i := range existing {
			cr := existing[i]
			c := cr
			existingMap[cr.ID] = &c
		}
	}

	commits, err := s.git.RecentCommits(targetBase, 5000)
	if err != nil {
		return nil, err
	}

	report := &RepairReport{
		BaseBranch:    targetBase,
		Scanned:       len(commits),
		RepairedCRIDs: []int{},
	}

	uidBackfilledSet := map[int]struct{}{}
	for id, existing := range existingMap {
		changedUID, uidErr := ensureCRUID(existing)
		if uidErr != nil {
			return nil, uidErr
		}
		changedBase, baseErr := s.ensureCRBaseFields(existing, false)
		if baseErr != nil {
			return nil, baseErr
		}
		if !changedUID && !changedBase {
			continue
		}
		if strings.TrimSpace(existing.CreatedAt) == "" {
			existing.CreatedAt = s.timestamp()
		}
		existing.UpdatedAt = s.timestamp()
		if err := s.store.SaveCR(existing); err != nil {
			return nil, err
		}
		report.Updated++
		uidBackfilledSet[id] = struct{}{}
	}

	repairedSet := map[int]struct{}{}
	for _, commit := range commits {
		id, ok := crIDFromSubjectOrBody(commit.Subject, commit.Body)
		if !ok {
			continue
		}

		existing, exists := existingMap[id]
		if exists && existing.Status == model.StatusInProgress && !refresh {
			report.Skipped++
			continue
		}

		description := intentFromCommitBody(commit.Body)
		notes := notesFromCommitBody(commit.Body)
		subtasks := subtasksFromCommitBody(commit.Body, commit.When, commit.Author)
		when := nonEmptyTrimmed(commit.When, s.timestamp())
		actor := nonEmptyTrimmed(commit.Author, "unknown")
		filesTouched := 0
		if count, countErr := s.git.ChangedFileCount(commit.Hash); countErr == nil {
			filesTouched = count
		}

		cr := &model.CR{
			ID:                id,
			UID:               crUIDFromBody(commit.Body),
			Title:             titleFromSubjectOrBody(commit.Subject, commit.Body),
			Description:       description,
			Status:            model.StatusMerged,
			BaseBranch:        targetBase,
			BaseRef:           baseRefFromBody(commit.Body),
			BaseCommit:        baseCommitFromBody(commit.Body),
			ParentCRID:        parentCRIDFromBody(commit.Body),
			Branch:            fmt.Sprintf("sophia/cr-%d", id),
			Notes:             notes,
			Subtasks:          subtasks,
			Events:            []model.Event{},
			MergedAt:          when,
			MergedBy:          actor,
			MergedCommit:      shortHash(commit.Hash),
			FilesTouchedCount: filesTouched,
			CreatedAt:         when,
			UpdatedAt:         when,
		}

		if exists {
			if strings.TrimSpace(cr.UID) == "" {
				cr.UID = strings.TrimSpace(existing.UID)
			}
			if strings.TrimSpace(cr.BaseRef) == "" {
				cr.BaseRef = strings.TrimSpace(existing.BaseRef)
			}
			if strings.TrimSpace(cr.BaseCommit) == "" {
				cr.BaseCommit = strings.TrimSpace(existing.BaseCommit)
			}
			if cr.ParentCRID <= 0 {
				cr.ParentCRID = existing.ParentCRID
			}
			cr.CreatedAt = existing.CreatedAt
			if strings.TrimSpace(cr.CreatedAt) == "" {
				cr.CreatedAt = when
			}
			cr.Events = append([]model.Event{}, existing.Events...)
			if _, backfilled := uidBackfilledSet[id]; !backfilled {
				report.Updated++
			}
		} else {
			report.Imported++
		}
		if _, uidErr := ensureCRUID(cr); uidErr != nil {
			return nil, uidErr
		}
		if _, baseErr := s.ensureCRBaseFields(cr, false); baseErr != nil {
			return nil, baseErr
		}
		cr.Events = append(cr.Events, model.Event{
			TS:      s.timestamp(),
			Actor:   s.git.Actor(),
			Type:    "cr_repaired",
			Summary: fmt.Sprintf("Repaired CR %d from git commit %s", id, shortHash(commit.Hash)),
			Ref:     fmt.Sprintf("cr:%d", id),
		})

		if err := s.store.SaveCR(cr); err != nil {
			return nil, err
		}
		existingMap[id] = cr
		if id > report.HighestCRID {
			report.HighestCRID = id
		}
		if _, seen := repairedSet[id]; !seen {
			repairedSet[id] = struct{}{}
			report.RepairedCRIDs = append(report.RepairedCRIDs, id)
		}
	}

	if err := s.ensureNextCRIDFloor(targetBase); err != nil {
		return nil, err
	}
	idx, err := s.store.LoadIndex()
	if err != nil {
		return nil, err
	}
	report.NextID = idx.NextID
	sort.Ints(report.RepairedCRIDs)
	return report, nil
}

func (s *Service) InstallHook(forceOverwrite bool) (string, error) {
	if err := s.store.EnsureInitialized(); err != nil {
		return "", err
	}
	cfg, err := s.store.LoadConfig()
	if err != nil {
		return "", err
	}
	return s.git.InstallPreCommitHook(cfg.BaseBranch, forceOverwrite)
}

func nextTaskID(tasks []model.Subtask) int {
	maxID := 0
	for _, task := range tasks {
		if task.ID > maxID {
			maxID = task.ID
		}
	}
	return maxID + 1
}

func indexOfTask(tasks []model.Subtask, taskID int) int {
	for i := range tasks {
		if tasks[i].ID == taskID {
			return i
		}
	}
	return -1
}

func buildMergeCommitMessage(cr *model.CR, actor, mergedAt string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "[CR-%d] %s\n\n", cr.ID, cr.Title)

	b.WriteString("Intent:\n")
	if strings.TrimSpace(cr.Description) == "" {
		b.WriteString("(none)\n\n")
	} else {
		b.WriteString(cr.Description)
		b.WriteString("\n\n")
	}

	b.WriteString("Subtasks:\n")
	if len(cr.Subtasks) == 0 {
		b.WriteString("- (none)\n\n")
	} else {
		for _, task := range cr.Subtasks {
			marker := "[ ]"
			if task.Status == model.TaskStatusDone {
				marker = "[x]"
			}
			fmt.Fprintf(&b, "- %s #%d %s\n", marker, task.ID, task.Title)
		}
		b.WriteString("\n")
	}

	b.WriteString("Notes:\n")
	if len(cr.Notes) == 0 {
		b.WriteString("- (none)\n\n")
	} else {
		for _, note := range cr.Notes {
			fmt.Fprintf(&b, "- %s\n", note)
		}
		b.WriteString("\n")
	}

	b.WriteString("Metadata:\n")
	fmt.Fprintf(&b, "- actor: %s\n", actor)
	fmt.Fprintf(&b, "- merged_at: %s\n", mergedAt)
	b.WriteString("\n")
	fmt.Fprintf(&b, "Sophia-CR: %d\n", cr.ID)
	fmt.Fprintf(&b, "Sophia-CR-UID: %s\n", strings.TrimSpace(cr.UID))
	fmt.Fprintf(&b, "Sophia-Base-Ref: %s\n", nonEmptyTrimmed(cr.BaseRef, cr.BaseBranch))
	fmt.Fprintf(&b, "Sophia-Base-Commit: %s\n", strings.TrimSpace(cr.BaseCommit))
	if cr.ParentCRID > 0 {
		fmt.Fprintf(&b, "Sophia-Parent-CR: %d\n", cr.ParentCRID)
	}
	fmt.Fprintf(&b, "Sophia-Intent: %s\n", cr.Title)
	fmt.Fprintf(&b, "Sophia-Tasks: %d completed\n", completedTasks(cr.Subtasks))
	return b.String()
}

func completedTasks(tasks []model.Subtask) int {
	count := 0
	for _, task := range tasks {
		if task.Status == model.TaskStatusDone {
			count++
		}
	}
	return count
}

func buildTaskCheckpointMessage(cr *model.CR, task *model.Subtask, scopeMode string, chunkCount int) string {
	taskType := inferTaskCommitType(task.Title)
	subject := fmt.Sprintf("%s(cr-%d/task-%d): %s", taskType, cr.ID, task.ID, strings.TrimSpace(task.Title))
	var b strings.Builder
	b.WriteString(subject)
	b.WriteString("\n\n")
	fmt.Fprintf(&b, "Task: #%d %s\n", task.ID, strings.TrimSpace(task.Title))
	fmt.Fprintf(&b, "CR: %d %s\n\n", cr.ID, strings.TrimSpace(cr.Title))
	fmt.Fprintf(&b, "Sophia-CR: %d\n", cr.ID)
	fmt.Fprintf(&b, "Sophia-CR-UID: %s\n", strings.TrimSpace(cr.UID))
	fmt.Fprintf(&b, "Sophia-Base-Ref: %s\n", nonEmptyTrimmed(cr.BaseRef, cr.BaseBranch))
	fmt.Fprintf(&b, "Sophia-Base-Commit: %s\n", strings.TrimSpace(cr.BaseCommit))
	if cr.ParentCRID > 0 {
		fmt.Fprintf(&b, "Sophia-Parent-CR: %d\n", cr.ParentCRID)
	}
	if strings.TrimSpace(scopeMode) != "" {
		fmt.Fprintf(&b, "Sophia-Task-Scope-Mode: %s\n", strings.TrimSpace(scopeMode))
	}
	if strings.TrimSpace(scopeMode) == "patch_manifest" {
		fmt.Fprintf(&b, "Sophia-Task-Chunk-Count: %d\n", chunkCount)
	}
	fmt.Fprintf(&b, "Sophia-Task: %d\n", task.ID)
	fmt.Fprintf(&b, "Sophia-Intent: %s\n", strings.TrimSpace(cr.Title))
	return b.String()
}

func inferTaskCommitType(taskTitle string) string {
	prefixes := []string{"feat", "fix", "docs", "refactor", "test", "chore", "perf", "build", "ci", "style", "revert"}
	lower := strings.ToLower(strings.TrimSpace(taskTitle))
	for _, prefix := range prefixes {
		token := prefix + ":"
		if strings.HasPrefix(lower, token) || strings.HasPrefix(lower, prefix+" ") {
			return prefix
		}
	}
	return "chore"
}

func (s *Service) summarizeCRDiff(cr *model.CR) (*diffSummary, error) {
	if _, err := s.ensureCRBaseFields(cr, true); err != nil {
		return nil, err
	}
	var (
		changes   []gitx.FileChange
		shortStat string
		err       error
	)
	switch {
	case s.git.BranchExists(cr.Branch):
		changes, err = s.diffNameStatusForCR(cr)
		if err != nil {
			return nil, err
		}
		shortStat, err = s.diffShortStatForCR(cr)
		if err != nil {
			return nil, err
		}
	case cr.Status == model.StatusMerged:
		changes, shortStat, err = s.summarizeMergedCRDiff(cr)
		if err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("unable to summarize CR %d diff: missing branch context (%q, %q)", cr.ID, cr.BaseBranch, cr.Branch)
	}

	files := make([]string, 0, len(changes))
	newFiles := []string{}
	modifiedFiles := []string{}
	deletedFiles := []string{}
	testFiles := []string{}
	depFiles := []string{}
	seenTest := map[string]struct{}{}
	seenDep := map[string]struct{}{}

	for _, change := range changes {
		changePath := strings.TrimSpace(change.Path)
		if changePath == "" {
			continue
		}
		files = append(files, changePath)
		switch change.Status {
		case "A":
			newFiles = append(newFiles, changePath)
		case "D":
			deletedFiles = append(deletedFiles, changePath)
		default:
			modifiedFiles = append(modifiedFiles, changePath)
		}
		if isTestFile(changePath) {
			if _, ok := seenTest[changePath]; !ok {
				seenTest[changePath] = struct{}{}
				testFiles = append(testFiles, changePath)
			}
		}
		if isDependencyFile(changePath) {
			if _, ok := seenDep[changePath]; !ok {
				seenDep[changePath] = struct{}{}
				depFiles = append(depFiles, changePath)
			}
		}
	}

	sort.Strings(files)
	sort.Strings(newFiles)
	sort.Strings(modifiedFiles)
	sort.Strings(deletedFiles)
	sort.Strings(testFiles)
	sort.Strings(depFiles)

	return &diffSummary{
		Files:           files,
		ShortStat:       shortStat,
		NewFiles:        newFiles,
		ModifiedFiles:   modifiedFiles,
		DeletedFiles:    deletedFiles,
		TestFiles:       testFiles,
		DependencyFiles: depFiles,
	}, nil
}

func (s *Service) summarizeMergedCRDiff(cr *model.CR) ([]gitx.FileChange, string, error) {
	mergedCommit := strings.TrimSpace(cr.MergedCommit)
	var mergeDiffErr error
	if mergedCommit != "" {
		baseRef := mergedCommit + "^1"
		changes, err := s.git.DiffNameStatusBetween(baseRef, mergedCommit)
		if err != nil {
			mergeDiffErr = err
		} else {
			shortStat, statErr := s.git.DiffShortStatBetween(baseRef, mergedCommit)
			if statErr == nil {
				return changes, shortStat, nil
			}
			mergeDiffErr = statErr
		}
	}

	derivedChanges := deriveChangesFromTaskCheckpointScopes(cr.Subtasks)
	if len(derivedChanges) > 0 {
		shortStat := fmt.Sprintf("%d file(s) changed (derived from task checkpoint scope)", len(derivedChanges))
		return derivedChanges, shortStat, nil
	}

	if mergeDiffErr != nil {
		return nil, "", fmt.Errorf("unable to summarize merged CR %d diff: %w", cr.ID, mergeDiffErr)
	}
	return nil, "", fmt.Errorf("unable to summarize merged CR %d diff: merged commit and task checkpoint scope are unavailable", cr.ID)
}

func (s *Service) ensureCRBaseFields(cr *model.CR, persist bool) (bool, error) {
	if cr == nil {
		return false, errors.New("cr cannot be nil")
	}
	changed := false
	if strings.TrimSpace(cr.BaseBranch) == "" {
		cfg, err := s.store.LoadConfig()
		if err != nil {
			return false, err
		}
		cr.BaseBranch = cfg.BaseBranch
		changed = true
	}
	if strings.TrimSpace(cr.BaseRef) == "" {
		cr.BaseRef = cr.BaseBranch
		changed = true
	}
	if strings.TrimSpace(cr.BaseCommit) == "" && strings.TrimSpace(cr.BaseRef) != "" {
		if resolved, err := s.git.ResolveRef(cr.BaseRef); err == nil && strings.TrimSpace(resolved) != "" {
			cr.BaseCommit = strings.TrimSpace(resolved)
			changed = true
		}
	}
	if changed && persist {
		cr.UpdatedAt = s.timestamp()
		if err := s.store.SaveCR(cr); err != nil {
			return false, err
		}
	}
	return changed, nil
}

func (s *Service) resolveCRBaseAnchor(cr *model.CR) (string, error) {
	if strings.TrimSpace(cr.BaseCommit) != "" {
		return strings.TrimSpace(cr.BaseCommit), nil
	}
	if strings.TrimSpace(cr.BaseRef) != "" {
		resolved, err := s.git.ResolveRef(cr.BaseRef)
		if err != nil {
			return "", fmt.Errorf("resolve base ref %q: %w", cr.BaseRef, err)
		}
		return strings.TrimSpace(resolved), nil
	}
	if strings.TrimSpace(cr.BaseBranch) != "" {
		resolved, err := s.git.ResolveRef(cr.BaseBranch)
		if err != nil {
			return "", fmt.Errorf("resolve base branch %q: %w", cr.BaseBranch, err)
		}
		return strings.TrimSpace(resolved), nil
	}
	return "", errors.New("cr has no base anchor")
}

func (s *Service) diffNameStatusForCR(cr *model.CR) ([]gitx.FileChange, error) {
	if strings.TrimSpace(cr.BaseCommit) != "" {
		return s.git.DiffNameStatusBetween(strings.TrimSpace(cr.BaseCommit), cr.Branch)
	}
	baseRef := nonEmptyTrimmed(cr.BaseRef, cr.BaseBranch)
	return s.git.DiffNameStatus(baseRef, cr.Branch)
}

func (s *Service) diffShortStatForCR(cr *model.CR) (string, error) {
	if strings.TrimSpace(cr.BaseCommit) != "" {
		return s.git.DiffShortStatBetween(strings.TrimSpace(cr.BaseCommit), cr.Branch)
	}
	baseRef := nonEmptyTrimmed(cr.BaseRef, cr.BaseBranch)
	return s.git.DiffShortStat(baseRef, cr.Branch)
}

func (s *Service) diffNamesForCR(cr *model.CR) ([]string, error) {
	if strings.TrimSpace(cr.BaseCommit) != "" {
		return s.git.DiffNamesBetween(strings.TrimSpace(cr.BaseCommit), cr.Branch)
	}
	baseRef := nonEmptyTrimmed(cr.BaseRef, cr.BaseBranch)
	return s.git.DiffNames(baseRef, cr.Branch)
}

func (s *Service) parentBaseAnchor(parent *model.CR) (string, string, error) {
	if parent == nil {
		return "", "", errors.New("parent cr is required")
	}
	if _, err := s.ensureCRBaseFields(parent, true); err != nil {
		return "", "", err
	}

	if parent.Status == model.StatusInProgress && s.git.BranchExists(parent.Branch) {
		sha, err := s.git.ResolveRef(parent.Branch)
		if err != nil {
			return "", "", err
		}
		return parent.Branch, strings.TrimSpace(sha), nil
	}
	if parent.Status == model.StatusMerged {
		if strings.TrimSpace(parent.MergedCommit) != "" {
			sha, err := s.git.ResolveRef(parent.MergedCommit)
			if err == nil {
				return parent.BaseBranch, strings.TrimSpace(sha), nil
			}
			return parent.BaseBranch, strings.TrimSpace(parent.MergedCommit), nil
		}
		if strings.TrimSpace(parent.BaseCommit) != "" {
			return nonEmptyTrimmed(parent.BaseRef, parent.BaseBranch), strings.TrimSpace(parent.BaseCommit), nil
		}
	}
	anchorRef := nonEmptyTrimmed(parent.BaseRef, parent.BaseBranch)
	if strings.TrimSpace(anchorRef) == "" {
		return "", "", fmt.Errorf("parent CR %d has no base ref", parent.ID)
	}
	sha, err := s.git.ResolveRef(anchorRef)
	if err != nil {
		return "", "", err
	}
	return anchorRef, strings.TrimSpace(sha), nil
}

func (s *Service) backfillChildrenAfterParentMerge(parent *model.CR) error {
	if parent == nil || parent.ID <= 0 {
		return nil
	}
	crs, err := s.store.ListCRs()
	if err != nil {
		return err
	}
	resolvedMergeCommit := strings.TrimSpace(parent.MergedCommit)
	if resolvedMergeCommit != "" {
		if resolved, resolveErr := s.git.ResolveRef(resolvedMergeCommit); resolveErr == nil {
			resolvedMergeCommit = strings.TrimSpace(resolved)
		}
	}
	for i := range crs {
		child := crs[i]
		if child.ParentCRID != parent.ID || child.Status != model.StatusInProgress {
			continue
		}
		changed := false
		if strings.TrimSpace(child.BaseRef) != strings.TrimSpace(child.BaseBranch) {
			child.BaseRef = child.BaseBranch
			changed = true
		}
		if strings.TrimSpace(resolvedMergeCommit) != "" && strings.TrimSpace(child.BaseCommit) != resolvedMergeCommit {
			child.BaseCommit = resolvedMergeCommit
			changed = true
		}
		if !changed {
			continue
		}
		now := s.timestamp()
		child.UpdatedAt = now
		child.Events = append(child.Events, model.Event{
			TS:      now,
			Actor:   s.git.Actor(),
			Type:    "cr_parent_merged",
			Summary: fmt.Sprintf("Updated base anchor from merged parent CR %d", parent.ID),
			Ref:     fmt.Sprintf("cr:%d", child.ID),
			Meta: map[string]string{
				"parent_cr":   strconv.Itoa(parent.ID),
				"base_ref":    child.BaseRef,
				"base_commit": child.BaseCommit,
			},
		})
		if err := s.store.SaveCR(&child); err != nil {
			return err
		}
	}
	return nil
}

func buildImpactReport(cr *model.CR, diff *diffSummary) *ImpactReport {
	scope := append([]string(nil), cr.Contract.Scope...)
	scopeDrift := findScopeDrift(diff.Files, scope)
	taskScopeWarnings := findTaskScopeWarnings(cr.Subtasks, scope)
	taskContractWarnings := findTaskContractWarnings(cr.Subtasks)

	signals := []RiskSignal{}
	riskScore := 0
	addSignal := func(code, summary string, points int) {
		if points <= 0 {
			return
		}
		signals = append(signals, RiskSignal{Code: code, Summary: summary, Points: points})
		riskScore += points
	}

	criticalPrefixes := []string{"internal/service/", "internal/store/", "internal/gitx/", "cmd/"}
	criticalTouched := []string{}
	for _, file := range diff.Files {
		for _, prefix := range criticalPrefixes {
			if strings.HasPrefix(file, prefix) {
				criticalTouched = append(criticalTouched, prefix)
				break
			}
		}
	}
	criticalTouched = dedupeStrings(criticalTouched)
	if len(criticalTouched) > 0 {
		addSignal("critical_paths", fmt.Sprintf("critical paths touched: %s", strings.Join(criticalTouched, ", ")), 3)
	}
	if len(diff.DependencyFiles) > 0 {
		addSignal("dependency_changes", fmt.Sprintf("%d dependency file(s) changed", len(diff.DependencyFiles)), 2)
	}
	if len(diff.DeletedFiles) > 0 {
		addSignal("deletions", fmt.Sprintf("%d deleted file(s)", len(diff.DeletedFiles)), 2)
	}
	if len(diff.Files) > 20 {
		addSignal("large_change_set", fmt.Sprintf("%d files changed", len(diff.Files)), 2)
	}
	nonTestChanges := len(diff.Files) > len(diff.TestFiles)
	if nonTestChanges && len(diff.TestFiles) == 0 {
		addSignal("no_test_changes", "non-test changes detected without test file updates", 1)
	}
	if len(scopeDrift) > 0 {
		addSignal("scope_drift", fmt.Sprintf("%d file(s) outside declared scope", len(scopeDrift)), 2)
	}

	riskTier := "low"
	switch {
	case riskScore >= 7:
		riskTier = "high"
	case riskScore >= 3:
		riskTier = "medium"
	}

	return &ImpactReport{
		CRID:                 cr.ID,
		CRUID:                strings.TrimSpace(cr.UID),
		BaseRef:              strings.TrimSpace(cr.BaseRef),
		BaseCommit:           strings.TrimSpace(cr.BaseCommit),
		ParentCRID:           cr.ParentCRID,
		FilesChanged:         len(diff.Files),
		NewFiles:             append([]string(nil), diff.NewFiles...),
		ModifiedFiles:        append([]string(nil), diff.ModifiedFiles...),
		DeletedFiles:         append([]string(nil), diff.DeletedFiles...),
		TestFiles:            append([]string(nil), diff.TestFiles...),
		DependencyFiles:      append([]string(nil), diff.DependencyFiles...),
		ScopeDrift:           scopeDrift,
		TaskScopeWarnings:    taskScopeWarnings,
		TaskContractWarnings: taskContractWarnings,
		Signals:              signals,
		RiskScore:            riskScore,
		RiskTier:             riskTier,
	}
}

func findScopeDrift(changedFiles, scopePrefixes []string) []string {
	if len(changedFiles) == 0 {
		return []string{}
	}
	if len(scopePrefixes) == 0 {
		return append([]string(nil), changedFiles...)
	}
	drift := []string{}
	for _, changedPath := range changedFiles {
		inScope := false
		for _, scopePrefix := range scopePrefixes {
			if pathMatchesScopePrefix(changedPath, scopePrefix) {
				inScope = true
				break
			}
		}
		if !inScope {
			drift = append(drift, changedPath)
		}
	}
	sort.Strings(drift)
	return drift
}

func findTaskScopeWarnings(tasks []model.Subtask, scopePrefixes []string) []string {
	if len(scopePrefixes) == 0 {
		return []string{}
	}
	warnings := []string{}
	for _, task := range tasks {
		if task.Status != model.TaskStatusDone || len(task.CheckpointScope) == 0 {
			continue
		}
		for _, scopedPath := range task.CheckpointScope {
			if strings.TrimSpace(scopedPath) == "" || scopedPath == "*" {
				continue
			}
			inScope := false
			for _, scopePrefix := range scopePrefixes {
				if pathMatchesScopePrefix(scopedPath, scopePrefix) {
					inScope = true
					break
				}
			}
			if !inScope {
				warnings = append(warnings, fmt.Sprintf("task #%d checkpoint scope %q is outside contract scope", task.ID, scopedPath))
			}
		}
	}
	sort.Strings(warnings)
	return dedupeStrings(warnings)
}

func findTaskContractWarnings(tasks []model.Subtask) []string {
	warnings := []string{}
	for _, task := range tasks {
		if task.Status != model.TaskStatusDone {
			continue
		}
		missing := missingTaskContractFields(task.Contract)
		if len(missing) > 0 {
			warnings = append(warnings, fmt.Sprintf("task #%d is done but missing contract fields: %s", task.ID, strings.Join(missing, ",")))
		}
		if len(task.Contract.Scope) == 0 || len(task.CheckpointScope) == 0 {
			continue
		}
		for _, scopedPath := range task.CheckpointScope {
			if strings.TrimSpace(scopedPath) == "" || scopedPath == "*" {
				continue
			}
			inScope := false
			for _, taskScope := range task.Contract.Scope {
				if pathMatchesScopePrefix(scopedPath, taskScope) {
					inScope = true
					break
				}
			}
			if !inScope {
				warnings = append(warnings, fmt.Sprintf("task #%d checkpoint scope %q is outside task contract scope", task.ID, scopedPath))
			}
		}
	}
	sort.Strings(warnings)
	return dedupeStrings(warnings)
}

func deriveChangesFromTaskCheckpointScopes(tasks []model.Subtask) []gitx.FileChange {
	seen := map[string]struct{}{}
	changes := make([]gitx.FileChange, 0)
	for _, task := range tasks {
		for _, scopedPath := range task.CheckpointScope {
			scopedPath = strings.TrimSpace(scopedPath)
			if scopedPath == "" || scopedPath == "*" {
				continue
			}
			if _, ok := seen[scopedPath]; ok {
				continue
			}
			seen[scopedPath] = struct{}{}
			changes = append(changes, gitx.FileChange{
				Status: "M",
				Path:   scopedPath,
			})
		}
	}
	sort.Slice(changes, func(i, j int) bool {
		return changes[i].Path < changes[j].Path
	})
	return changes
}

func missingCRContractFields(contract model.Contract) []string {
	missing := []string{}
	if strings.TrimSpace(contract.Why) == "" {
		missing = append(missing, "why")
	}
	if len(contract.Scope) == 0 {
		missing = append(missing, "scope")
	}
	if len(normalizeNonEmptyStringList(contract.NonGoals)) == 0 {
		missing = append(missing, "non_goals")
	}
	if len(normalizeNonEmptyStringList(contract.Invariants)) == 0 {
		missing = append(missing, "invariants")
	}
	if strings.TrimSpace(contract.BlastRadius) == "" {
		missing = append(missing, "blast_radius")
	}
	if strings.TrimSpace(contract.TestPlan) == "" {
		missing = append(missing, "test_plan")
	}
	if strings.TrimSpace(contract.RollbackPlan) == "" {
		missing = append(missing, "rollback_plan")
	}
	return missing
}

func missingTaskContractFields(contract model.TaskContract) []string {
	missing := []string{}
	if strings.TrimSpace(contract.Intent) == "" {
		missing = append(missing, "intent")
	}
	if len(normalizeNonEmptyStringList(contract.AcceptanceCriteria)) == 0 {
		missing = append(missing, "acceptance_criteria")
	}
	if len(contract.Scope) == 0 {
		missing = append(missing, "scope")
	}
	return missing
}

func pathMatchesScopePrefix(candidatePath, scopePrefix string) bool {
	candidatePath = strings.TrimSpace(candidatePath)
	scopePrefix = strings.TrimSpace(scopePrefix)
	if candidatePath == "" || scopePrefix == "" {
		return false
	}
	if scopePrefix == "." {
		return true
	}
	if candidatePath == scopePrefix {
		return true
	}
	return strings.HasPrefix(candidatePath, scopePrefix+"/")
}

func (s *Service) computeOverlapWarnings(referenceDirs map[string]struct{}, skipCRID int) []string {
	if len(referenceDirs) == 0 {
		return nil
	}
	crs, err := s.store.ListCRs()
	if err != nil {
		return nil
	}
	warnings := make([]string, 0)
	for _, cr := range crs {
		if cr.ID == skipCRID || cr.Status != model.StatusInProgress {
			continue
		}
		if !s.git.BranchExists(cr.Branch) || !s.git.BranchExists(cr.BaseBranch) {
			continue
		}
		files, diffErr := s.git.DiffNames(cr.BaseBranch, cr.Branch)
		if diffErr != nil {
			continue
		}
		dirs := topLevelDirs(files)
		for dir := range referenceDirs {
			if _, ok := dirs[dir]; ok {
				warnings = append(warnings, fmt.Sprintf("Potential overlap: CR-%d also touches /%s", cr.ID, dir))
			}
		}
	}
	sort.Strings(warnings)
	return dedupeStrings(warnings)
}

func topLevelDirs(paths []string) map[string]struct{} {
	res := map[string]struct{}{}
	for _, path := range paths {
		path = strings.TrimSpace(path)
		if path == "" {
			continue
		}
		first := path
		if idx := strings.Index(path, "/"); idx >= 0 {
			first = path[:idx]
		}
		if strings.TrimSpace(first) == "" {
			continue
		}
		res[first] = struct{}{}
	}
	return res
}

func isTestFile(path string) bool {
	lower := strings.ToLower(path)
	if strings.HasSuffix(lower, "_test.go") {
		return true
	}
	if strings.Contains(lower, "/test/") || strings.Contains(lower, "/tests/") {
		return true
	}
	for _, suffix := range []string{".spec.js", ".spec.ts", ".spec.jsx", ".spec.tsx", ".test.js", ".test.ts", ".test.jsx", ".test.tsx"} {
		if strings.HasSuffix(lower, suffix) {
			return true
		}
	}
	return false
}

func isDependencyFile(path string) bool {
	names := map[string]struct{}{
		"go.mod":            {},
		"go.sum":            {},
		"package.json":      {},
		"package-lock.json": {},
		"pnpm-lock.yaml":    {},
		"yarn.lock":         {},
		"cargo.toml":        {},
		"cargo.lock":        {},
		"requirements.txt":  {},
		"poetry.lock":       {},
	}
	lower := strings.ToLower(path)
	parts := strings.Split(lower, "/")
	if len(parts) == 0 {
		return false
	}
	_, ok := names[parts[len(parts)-1]]
	return ok
}

func commitTiedToCR(subject, body string) bool {
	if crSubjectPattern.MatchString(strings.TrimSpace(subject)) {
		return true
	}
	return crFooterPattern.MatchString(body)
}

func crIDFromSubjectOrBody(subject, body string) (int, bool) {
	if matches := crSubjectPattern.FindStringSubmatch(strings.TrimSpace(subject)); len(matches) >= 2 {
		id, err := strconv.Atoi(strings.TrimSpace(matches[1]))
		if err == nil && id > 0 {
			return id, true
		}
	}
	matches := footerCRIDPattern.FindStringSubmatch(body)
	if len(matches) != 2 {
		return 0, false
	}
	id, err := strconv.Atoi(strings.TrimSpace(matches[1]))
	if err != nil || id <= 0 {
		return 0, false
	}
	return id, true
}

func crUIDFromBody(body string) string {
	matches := footerCRUIDPattern.FindStringSubmatch(body)
	if len(matches) != 2 {
		return ""
	}
	return strings.TrimSpace(matches[1])
}

func baseRefFromBody(body string) string {
	matches := footerBaseRefPattern.FindStringSubmatch(body)
	if len(matches) != 2 {
		return ""
	}
	return strings.TrimSpace(matches[1])
}

func baseCommitFromBody(body string) string {
	matches := footerBaseSHApattern.FindStringSubmatch(body)
	if len(matches) != 2 {
		return ""
	}
	return strings.TrimSpace(matches[1])
}

func parentCRIDFromBody(body string) int {
	matches := footerParentPattern.FindStringSubmatch(body)
	if len(matches) != 2 {
		return 0
	}
	id, err := strconv.Atoi(strings.TrimSpace(matches[1]))
	if err != nil || id <= 0 {
		return 0
	}
	return id
}

func titleFromSubjectOrBody(subject, body string) string {
	if matches := crSubjectPattern.FindStringSubmatch(strings.TrimSpace(subject)); len(matches) >= 3 {
		title := strings.TrimSpace(matches[2])
		if title != "" {
			return title
		}
	}
	if matches := footerIntentPattern.FindStringSubmatch(body); len(matches) == 2 {
		title := strings.TrimSpace(matches[1])
		if title != "" {
			return title
		}
	}
	return "(unknown)"
}

func sectionFromCommitBody(body, section string) string {
	body = strings.ReplaceAll(body, "\r\n", "\n")
	needle := section + ":\n"
	start := strings.Index(body, needle)
	if start < 0 {
		return ""
	}
	rest := body[start+len(needle):]
	marker := "\n\n"
	if idx := strings.Index(rest, marker); idx >= 0 {
		return strings.TrimSpace(rest[:idx])
	}
	return strings.TrimSpace(rest)
}

func intentFromCommitBody(body string) string {
	section := sectionFromCommitBody(body, "Intent")
	if strings.EqualFold(strings.TrimSpace(section), "(none)") {
		return ""
	}
	return section
}

func notesFromCommitBody(body string) []string {
	section := sectionFromCommitBody(body, "Notes")
	if section == "" || strings.EqualFold(strings.TrimSpace(section), "- (none)") {
		return []string{}
	}
	res := make([]string, 0)
	for _, line := range strings.Split(section, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "- ") {
			continue
		}
		note := strings.TrimSpace(strings.TrimPrefix(line, "- "))
		if note == "" || strings.EqualFold(note, "(none)") {
			continue
		}
		res = append(res, note)
	}
	return res
}

func subtasksFromCommitBody(body, when, actor string) []model.Subtask {
	section := sectionFromCommitBody(body, "Subtasks")
	if section == "" || strings.EqualFold(strings.TrimSpace(section), "- (none)") {
		return []model.Subtask{}
	}
	res := make([]model.Subtask, 0)
	for _, line := range strings.Split(section, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "- [") {
			continue
		}
		open := strings.HasPrefix(line, "- [ ]")
		done := strings.HasPrefix(line, "- [x]") || strings.HasPrefix(line, "- [X]")
		if !open && !done {
			continue
		}
		rest := strings.TrimSpace(line[5:])
		taskID := len(res) + 1
		title := rest
		if strings.HasPrefix(rest, "#") {
			parts := strings.SplitN(rest, " ", 2)
			if len(parts) == 2 {
				if parsed, err := strconv.Atoi(strings.TrimPrefix(parts[0], "#")); err == nil && parsed > 0 {
					taskID = parsed
				}
				title = strings.TrimSpace(parts[1])
			}
		}
		status := model.TaskStatusOpen
		completedAt := ""
		completedBy := ""
		if done {
			status = model.TaskStatusDone
			completedAt = when
			completedBy = actor
		}
		res = append(res, model.Subtask{
			ID:          taskID,
			Title:       title,
			Status:      status,
			CreatedAt:   when,
			UpdatedAt:   when,
			CompletedAt: completedAt,
			CreatedBy:   actor,
			CompletedBy: completedBy,
		})
	}
	sort.Slice(res, func(i, j int) bool { return res[i].ID < res[j].ID })
	return res
}

func parseCRBranchID(branch string) (int, bool) {
	matches := crBranchPattern.FindStringSubmatch(strings.TrimSpace(branch))
	if len(matches) != 2 {
		return 0, false
	}
	id, err := strconv.Atoi(matches[1])
	if err != nil || id <= 0 {
		return 0, false
	}
	return id, true
}

func shortHash(hash string) string {
	hash = strings.TrimSpace(hash)
	if len(hash) > 7 {
		return hash[:7]
	}
	return hash
}

func newCRUID() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", fmt.Errorf("generate cr uid: %w", err)
	}
	// RFC 4122 variant/version bits for compatibility with UUID tooling.
	raw[6] = (raw[6] & 0x0f) | 0x40
	raw[8] = (raw[8] & 0x3f) | 0x80
	return fmt.Sprintf("cr_%x-%x-%x-%x-%x", raw[0:4], raw[4:6], raw[6:8], raw[8:10], raw[10:16]), nil
}

func ensureCRUID(cr *model.CR) (bool, error) {
	if cr == nil {
		return false, errors.New("cr cannot be nil")
	}
	if strings.TrimSpace(cr.UID) != "" {
		return false, nil
	}
	uid, err := newCRUID()
	if err != nil {
		return false, err
	}
	cr.UID = uid
	return true, nil
}

func parseRFC3339OrZero(raw string) time.Time {
	t, err := time.Parse(time.RFC3339, strings.TrimSpace(raw))
	if err != nil {
		return time.Time{}
	}
	return t
}

func dedupeStrings(values []string) []string {
	seen := map[string]struct{}{}
	res := make([]string, 0, len(values))
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		res = append(res, value)
	}
	return res
}

func validateDoneTaskOptions(opts DoneTaskOptions) error {
	if !opts.Checkpoint {
		if opts.StageAll || opts.FromContract || len(opts.Paths) > 0 || strings.TrimSpace(opts.PatchFile) != "" {
			return fmt.Errorf("%w: --no-checkpoint cannot be combined with --from-contract, --path, --patch-file, or --all", ErrInvalidTaskScope)
		}
		return nil
	}
	modes := 0
	if opts.StageAll {
		modes++
	}
	if opts.FromContract {
		modes++
	}
	if len(opts.Paths) > 0 {
		modes++
	}
	if strings.TrimSpace(opts.PatchFile) != "" {
		modes++
	}
	if modes > 1 {
		return fmt.Errorf("%w: exactly one of --all, --from-contract, --path, or --patch-file must be provided", ErrInvalidTaskScope)
	}
	if modes == 0 {
		return ErrTaskScopeRequired
	}
	return nil
}

func (s *Service) resolveTaskCheckpointPathsFromContract(scopePrefixes []string) ([]string, error) {
	statusEntries, err := s.git.WorkingTreeStatus()
	if err != nil {
		return nil, err
	}
	matches := make([]string, 0)
	seen := map[string]struct{}{}
	for _, entry := range statusEntries {
		candidate := strings.TrimSpace(entry.Path)
		if candidate == "" {
			continue
		}
		inScope := false
		for _, prefix := range scopePrefixes {
			if pathMatchesScopePrefix(candidate, prefix) {
				inScope = true
				break
			}
		}
		if !inScope {
			continue
		}
		if _, ok := seen[candidate]; ok {
			continue
		}
		seen[candidate] = struct{}{}
		matches = append(matches, candidate)
	}
	if len(matches) == 0 {
		return nil, ErrNoTaskScopeMatches
	}
	sort.Strings(matches)
	return matches, nil
}

func (s *Service) normalizeTaskScopePaths(paths []string) ([]string, error) {
	normalized := make([]string, 0, len(paths))
	seen := map[string]struct{}{}
	for _, raw := range paths {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			return nil, fmt.Errorf("%w: empty path", ErrInvalidTaskScope)
		}

		slashPath := strings.ReplaceAll(trimmed, "\\", "/")
		if filepath.IsAbs(trimmed) || strings.HasPrefix(slashPath, "/") {
			return nil, fmt.Errorf("%w: path %q must be repo-relative", ErrInvalidTaskScope, raw)
		}
		if strings.ContainsAny(slashPath, "*?[]{}") {
			return nil, fmt.Errorf("%w: path %q must be exact (no glob patterns)", ErrInvalidTaskScope, raw)
		}

		cleaned := path.Clean(slashPath)
		if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
			return nil, fmt.Errorf("%w: path %q escapes repository root", ErrInvalidTaskScope, raw)
		}
		if cleaned != slashPath {
			return nil, fmt.Errorf("%w: path %q must be normalized", ErrInvalidTaskScope, raw)
		}

		absPath := filepath.Join(s.git.WorkDir, filepath.FromSlash(cleaned))
		if info, statErr := os.Stat(absPath); statErr == nil && info.IsDir() {
			return nil, fmt.Errorf("%w: path %q is a directory; select files only", ErrInvalidTaskScope, raw)
		}
		if _, exists := seen[cleaned]; exists {
			return nil, fmt.Errorf("%w: duplicate path %q", ErrInvalidTaskScope, raw)
		}
		seen[cleaned] = struct{}{}
		normalized = append(normalized, cleaned)
	}
	return normalized, nil
}

func (s *Service) normalizePatchFilePath(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", fmt.Errorf("%w: patch file path is required", ErrInvalidTaskScope)
	}
	patchPath := trimmed
	if !filepath.IsAbs(patchPath) {
		patchPath = filepath.Join(s.git.WorkDir, patchPath)
	}
	info, err := os.Stat(patchPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("%w: patch file %q does not exist", ErrInvalidTaskScope, raw)
		}
		return "", fmt.Errorf("%w: patch file %q: %v", ErrInvalidTaskScope, raw, err)
	}
	if info.IsDir() {
		return "", fmt.Errorf("%w: patch file %q is a directory", ErrInvalidTaskScope, raw)
	}
	return patchPath, nil
}

func parsePatchChunks(diff string) ([]parsedPatchChunk, error) {
	diff = strings.ReplaceAll(diff, "\r\n", "\n")
	if strings.TrimSpace(diff) == "" {
		return []parsedPatchChunk{}, nil
	}

	lines := strings.Split(diff, "\n")
	chunks := make([]parsedPatchChunk, 0)
	currentPath := ""
	currentHeader := ""
	currentBody := []string{}

	flush := func() error {
		if currentHeader == "" {
			return nil
		}
		if strings.TrimSpace(currentPath) == "" {
			return fmt.Errorf("chunk header %q is missing file path", currentHeader)
		}
		oldStart, oldLines, newStart, newLines, err := parseHunkHeader(currentHeader)
		if err != nil {
			return err
		}
		body := strings.Join(currentBody, "\n")
		chunks = append(chunks, parsedPatchChunk{
			ID:       chunkIDFor(currentPath, currentHeader, body),
			Path:     currentPath,
			OldStart: oldStart,
			OldLines: oldLines,
			NewStart: newStart,
			NewLines: newLines,
			Header:   currentHeader,
			Body:     body,
			Preview:  chunkPreview(currentBody),
		})
		currentHeader = ""
		currentBody = nil
		return nil
	}

	for _, rawLine := range lines {
		line := strings.TrimSuffix(rawLine, "\r")
		switch {
		case strings.HasPrefix(line, "diff --git "):
			if err := flush(); err != nil {
				return nil, err
			}
			currentPath = pathFromDiffHeader(line)
		case strings.HasPrefix(line, "+++ "):
			nextPath := pathFromPatchLine(line)
			if nextPath != "" {
				currentPath = nextPath
			}
		case strings.HasPrefix(line, "@@ "):
			if err := flush(); err != nil {
				return nil, err
			}
			currentHeader = line
			currentBody = []string{}
		default:
			if currentHeader != "" {
				currentBody = append(currentBody, line)
			}
		}
	}
	if err := flush(); err != nil {
		return nil, err
	}
	return chunks, nil
}

func pathFromDiffHeader(line string) string {
	parts := strings.Fields(strings.TrimSpace(line))
	if len(parts) < 4 {
		return ""
	}
	return stripDiffPathPrefix(parts[3])
}

func pathFromPatchLine(line string) string {
	parts := strings.Fields(strings.TrimSpace(line))
	if len(parts) < 2 {
		return ""
	}
	if parts[1] == "/dev/null" {
		return ""
	}
	return stripDiffPathPrefix(parts[1])
}

func stripDiffPathPrefix(raw string) string {
	raw = strings.Trim(raw, "\"")
	switch {
	case strings.HasPrefix(raw, "a/"):
		return strings.TrimPrefix(raw, "a/")
	case strings.HasPrefix(raw, "b/"):
		return strings.TrimPrefix(raw, "b/")
	default:
		return raw
	}
}

func parseHunkHeader(line string) (int, int, int, int, error) {
	matches := hunkHeaderPattern.FindStringSubmatch(strings.TrimSpace(line))
	if len(matches) != 5 {
		return 0, 0, 0, 0, fmt.Errorf("invalid hunk header %q", line)
	}
	oldStart, err := strconv.Atoi(matches[1])
	if err != nil {
		return 0, 0, 0, 0, fmt.Errorf("invalid old start in hunk header %q", line)
	}
	oldLines := 1
	if strings.TrimSpace(matches[2]) != "" {
		oldLines, err = strconv.Atoi(matches[2])
		if err != nil {
			return 0, 0, 0, 0, fmt.Errorf("invalid old line count in hunk header %q", line)
		}
	}
	newStart, err := strconv.Atoi(matches[3])
	if err != nil {
		return 0, 0, 0, 0, fmt.Errorf("invalid new start in hunk header %q", line)
	}
	newLines := 1
	if strings.TrimSpace(matches[4]) != "" {
		newLines, err = strconv.Atoi(matches[4])
		if err != nil {
			return 0, 0, 0, 0, fmt.Errorf("invalid new line count in hunk header %q", line)
		}
	}
	return oldStart, oldLines, newStart, newLines, nil
}

func chunkIDFor(path, header, body string) string {
	sum := sha256.Sum256([]byte(path + "\n" + header + "\n" + body))
	return "chk_" + hex.EncodeToString(sum[:8])
}

func chunkPreview(lines []string) string {
	if len(lines) == 0 {
		return ""
	}
	snippets := make([]string, 0, 2)
	for _, line := range lines {
		if strings.HasPrefix(line, "+") || strings.HasPrefix(line, "-") {
			snippets = append(snippets, strings.TrimSpace(line))
		}
		if len(snippets) >= 2 {
			break
		}
	}
	if len(snippets) == 0 {
		snippets = append(snippets, strings.TrimSpace(lines[0]))
	}
	return strings.Join(snippets, " | ")
}

func checkpointChunkPaths(chunks []parsedPatchChunk) []string {
	seen := map[string]struct{}{}
	paths := make([]string, 0, len(chunks))
	for _, chunk := range chunks {
		p := strings.TrimSpace(chunk.Path)
		if p == "" {
			continue
		}
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		paths = append(paths, p)
	}
	sort.Strings(paths)
	return paths
}

func (s *Service) normalizeContractScopePrefixes(prefixes []string) ([]string, error) {
	normalized := make([]string, 0, len(prefixes))
	seen := map[string]struct{}{}
	for _, raw := range prefixes {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			return nil, fmt.Errorf("%w: empty scope prefix", ErrInvalidTaskScope)
		}
		slashPath := strings.ReplaceAll(trimmed, "\\", "/")
		if filepath.IsAbs(trimmed) || strings.HasPrefix(slashPath, "/") {
			return nil, fmt.Errorf("%w: scope prefix %q must be repo-relative", ErrInvalidTaskScope, raw)
		}
		if strings.ContainsAny(slashPath, "*?[]{}") {
			return nil, fmt.Errorf("%w: scope prefix %q must be exact prefix (no glob patterns)", ErrInvalidTaskScope, raw)
		}
		cleaned := path.Clean(slashPath)
		if cleaned == ".." || strings.HasPrefix(cleaned, "../") {
			return nil, fmt.Errorf("%w: scope prefix %q escapes repository root", ErrInvalidTaskScope, raw)
		}
		if cleaned != slashPath {
			return nil, fmt.Errorf("%w: scope prefix %q must be normalized", ErrInvalidTaskScope, raw)
		}
		if _, ok := seen[cleaned]; ok {
			return nil, fmt.Errorf("%w: duplicate scope prefix %q", ErrInvalidTaskScope, raw)
		}
		seen[cleaned] = struct{}{}
		normalized = append(normalized, cleaned)
	}
	sort.Strings(normalized)
	return normalized, nil
}

func normalizeNonEmptyStringList(values []string) []string {
	res := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		res = append(res, trimmed)
	}
	return dedupeStrings(res)
}

func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func oneBasedIndex(input, length int, label string) (int, error) {
	if input <= 0 {
		return 0, fmt.Errorf("%s index must be >= 1", label)
	}
	idx := input - 1
	if idx >= length {
		return 0, fmt.Errorf("%s index %d out of range", label, input)
	}
	return idx, nil
}

func cloneStringMap(src map[string]string) map[string]string {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]string, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func (s *Service) workingTreeDirtySummary() (bool, string, error) {
	entries, err := s.git.WorkingTreeStatus()
	if err != nil {
		return false, "", err
	}
	if len(entries) == 0 {
		return false, "", nil
	}
	untracked := 0
	changed := 0
	for _, entry := range entries {
		if s.isIgnorableWorktreeEntry(entry) {
			continue
		}
		if entry.Code == "??" {
			untracked++
		} else {
			changed++
		}
	}
	if changed == 0 && untracked == 0 {
		return false, "", nil
	}
	return true, fmt.Sprintf("%d modified/staged and %d untracked paths; commit or stash before switching", changed, untracked), nil
}

func nonEmptyTrimmed(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return strings.TrimSpace(value)
}

func isValidMetadataMode(mode string) bool {
	switch strings.TrimSpace(mode) {
	case model.MetadataModeLocal, model.MetadataModeTracked:
		return true
	default:
		return false
	}
}

func ensureGitIgnoreEntry(root, entry string) error {
	entry = strings.TrimSpace(entry)
	if entry == "" {
		return nil
	}

	path := filepath.Join(root, ".gitignore")
	content, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("read .gitignore: %w", err)
	}

	existing := string(content)
	lines := strings.Split(existing, "\n")
	for _, line := range lines {
		if strings.TrimSpace(line) == entry {
			return nil
		}
	}

	var b strings.Builder
	if strings.TrimSpace(existing) != "" {
		b.WriteString(strings.TrimRight(existing, "\n"))
		b.WriteString("\n")
	}
	b.WriteString(entry)
	b.WriteString("\n")

	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		return fmt.Errorf("write .gitignore: %w", err)
	}
	return nil
}

func (s *Service) ensureNextCRIDFloor(baseBranch string) error {
	idx, err := s.store.LoadIndex()
	if err != nil {
		return err
	}
	maxID := 0

	crs, err := s.store.ListCRs()
	if err == nil {
		for _, cr := range crs {
			if cr.ID > maxID {
				maxID = cr.ID
			}
		}
	}

	branches, err := s.git.LocalBranches("sophia/cr-")
	if err == nil {
		for _, branch := range branches {
			if id, ok := parseCRBranchID(branch); ok && id > maxID {
				maxID = id
			}
		}
	}

	if strings.TrimSpace(baseBranch) != "" {
		commits, err := s.git.RecentCommits(baseBranch, 5000)
		if err == nil {
			for _, commit := range commits {
				if id, ok := crIDFromSubjectOrBody(commit.Subject, commit.Body); ok && id > maxID {
					maxID = id
				}
			}
		}
	}

	required := maxID + 1
	if required < 1 {
		required = 1
	}
	if idx.NextID >= required {
		return nil
	}
	idx.NextID = required
	return s.store.SaveIndex(idx)
}

func (s *Service) timestamp() string {
	return s.now().UTC().Format(time.RFC3339)
}

func (s *Service) isIgnorableWorktreeEntry(entry gitx.StatusEntry) bool {
	if entry.Code != "??" {
		return false
	}
	if strings.TrimSpace(entry.Path) != ".gitignore" {
		return false
	}
	data, err := os.ReadFile(filepath.Join(s.git.WorkDir, ".gitignore"))
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || line == ".sophia/" {
			continue
		}
		return false
	}
	return true
}
