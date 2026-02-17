package service

import (
	"errors"
	"fmt"
	"os"
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
	ErrCRAlreadyMerged   = errors.New("cr is already merged")
	ErrNoActiveCRContext = errors.New("current branch is not a CR branch")
	ErrWorkingTreeDirty  = errors.New("working tree is dirty")
	ErrNoCRChanges       = errors.New("no CR changes provided")
	ErrAlreadyRedacted   = errors.New("target is already redacted")
	ErrNoTaskChanges     = errors.New("no task checkpoint changes found")
)

var (
	crBranchPattern      = regexp.MustCompile(`^sophia/cr-(\d+)$`)
	crSubjectPattern     = regexp.MustCompile(`^\[CR-(\d+)\]\s*(.*)$`)
	crFooterPattern      = regexp.MustCompile(`(?m)^Sophia-CR:\s*\d+\s*$`)
	legacyPersistPattern = regexp.MustCompile(`^chore:\s*persist CR-\d+\s+merged metadata$`)
	footerCRIDPattern    = regexp.MustCompile(`(?m)^Sophia-CR:\s*(\d+)\s*$`)
	footerIntentPattern  = regexp.MustCompile(`(?m)^Sophia-Intent:\s*(.+)\s*$`)
)

const redactedPlaceholder = "[REDACTED]"

type Service struct {
	store *store.Store
	git   *gitx.Client
	now   func() time.Time
}

type Review struct {
	CR              *model.CR
	Files           []string
	ShortStat       string
	NewFiles        []string
	ModifiedFiles   []string
	DeletedFiles    []string
	TestFiles       []string
	DependencyFiles []string
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
	cr, _, err := s.AddCRWithWarnings(title, description)
	return cr, err
}

func (s *Service) AddCRWithWarnings(title, description string) (*model.CR, []string, error) {
	if strings.TrimSpace(title) == "" {
		return nil, nil, errors.New("title cannot be empty")
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

	id, err := s.store.NextCRID()
	if err != nil {
		return nil, nil, err
	}

	branch := fmt.Sprintf("sophia/cr-%d", id)
	if s.git.BranchExists(branch) {
		return nil, nil, fmt.Errorf("branch %q already exists", branch)
	}
	if err := s.git.CreateBranch(branch); err != nil {
		return nil, nil, err
	}

	now := s.timestamp()
	actor := s.git.Actor()
	cr := &model.CR{
		ID:          id,
		Title:       title,
		Description: description,
		Status:      model.StatusInProgress,
		BaseBranch:  cfg.BaseBranch,
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

func (s *Service) DoneTask(crID, taskID int) error {
	_, err := s.DoneTaskWithCheckpoint(crID, taskID, false)
	return err
}

func (s *Service) DoneTaskWithCheckpoint(crID, taskID int, checkpoint bool) (string, error) {
	cr, err := s.store.LoadCR(crID)
	if err != nil {
		return "", err
	}
	if cr.Status != model.StatusInProgress {
		return "", fmt.Errorf("cr %d is not in progress", crID)
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

	commitSHA := ""
	if checkpoint {
		currentBranch, branchErr := s.git.CurrentBranch()
		if branchErr != nil {
			return "", branchErr
		}
		if currentBranch != cr.Branch {
			return "", fmt.Errorf("checkpoint requires active CR branch %q, current branch is %q", cr.Branch, currentBranch)
		}
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
		commitMessage := buildTaskCheckpointMessage(cr, &cr.Subtasks[taskIndex])
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
		cr.Events = append(cr.Events, model.Event{
			TS:      now,
			Actor:   actor,
			Type:    "task_checkpointed",
			Summary: fmt.Sprintf("Checkpointed task %d as %s", taskID, sha),
			Ref:     fmt.Sprintf("task:%d", taskID),
			Meta: map[string]string{
				"commit":  sha,
				"message": strings.SplitN(commitMessage, "\n", 2)[0],
			},
		})
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
	changes, err := s.git.DiffNameStatus(cr.BaseBranch, cr.Branch)
	if err != nil {
		return nil, err
	}
	shortStat, err := s.git.DiffShortStat(cr.BaseBranch, cr.Branch)
	if err != nil {
		return nil, err
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
		path := change.Path
		if path == "" {
			continue
		}
		files = append(files, path)
		switch change.Status {
		case "A":
			newFiles = append(newFiles, path)
		case "D":
			deletedFiles = append(deletedFiles, path)
		default:
			modifiedFiles = append(modifiedFiles, path)
		}
		if isTestFile(path) {
			if _, ok := seenTest[path]; !ok {
				seenTest[path] = struct{}{}
				testFiles = append(testFiles, path)
			}
		}
		if isDependencyFile(path) {
			if _, ok := seenDep[path]; !ok {
				seenDep[path] = struct{}{}
				depFiles = append(depFiles, path)
			}
		}
	}

	sort.Strings(files)
	sort.Strings(newFiles)
	sort.Strings(modifiedFiles)
	sort.Strings(deletedFiles)
	sort.Strings(testFiles)
	sort.Strings(depFiles)

	return &Review{
		CR:              cr,
		Files:           files,
		ShortStat:       shortStat,
		NewFiles:        newFiles,
		ModifiedFiles:   modifiedFiles,
		DeletedFiles:    deletedFiles,
		TestFiles:       testFiles,
		DependencyFiles: depFiles,
	}, nil
}

func (s *Service) MergeCR(id int, keepBranch bool) (string, error) {
	cr, err := s.store.LoadCR(id)
	if err != nil {
		return "", err
	}
	if cr.Status == model.StatusMerged {
		return "", ErrCRAlreadyMerged
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

	files, err := s.git.DiffNames(cr.BaseBranch, cr.Branch)
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
	if s.git.BranchExists(cr.Branch) {
		if err := s.git.CheckoutBranch(cr.Branch); err != nil {
			return nil, err
		}
		return cr, nil
	}

	if cr.Status != model.StatusInProgress {
		return nil, fmt.Errorf("branch %q is missing for merged CR %d; run sophia cr reopen %d", cr.Branch, cr.ID, cr.ID)
	}
	if err := s.git.EnsureBaseBranch(cr.BaseBranch); err != nil {
		return nil, err
	}
	if err := s.git.CreateBranch(cr.Branch); err != nil {
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
	if cr.Status != model.StatusMerged {
		return nil, fmt.Errorf("cr %d is not merged", id)
	}
	if s.git.BranchExists(cr.Branch) {
		if err := s.git.CheckoutBranch(cr.Branch); err != nil {
			return nil, err
		}
	} else {
		if err := s.git.EnsureBaseBranch(cr.BaseBranch); err != nil {
			return nil, err
		}
		if err := s.git.CreateBranch(cr.Branch); err != nil {
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
			Title:             titleFromSubjectOrBody(commit.Subject, commit.Body),
			Description:       description,
			Status:            model.StatusMerged,
			BaseBranch:        targetBase,
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
			cr.CreatedAt = existing.CreatedAt
			if strings.TrimSpace(cr.CreatedAt) == "" {
				cr.CreatedAt = when
			}
			cr.Events = append([]model.Event{}, existing.Events...)
			report.Updated++
		} else {
			report.Imported++
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

func buildTaskCheckpointMessage(cr *model.CR, task *model.Subtask) string {
	taskType := inferTaskCommitType(task.Title)
	subject := fmt.Sprintf("%s(cr-%d/task-%d): %s", taskType, cr.ID, task.ID, strings.TrimSpace(task.Title))
	var b strings.Builder
	b.WriteString(subject)
	b.WriteString("\n\n")
	fmt.Fprintf(&b, "Task: #%d %s\n", task.ID, strings.TrimSpace(task.Title))
	fmt.Fprintf(&b, "CR: %d %s\n\n", cr.ID, strings.TrimSpace(cr.Title))
	fmt.Fprintf(&b, "Sophia-CR: %d\n", cr.ID)
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
