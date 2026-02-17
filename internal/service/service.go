package service

import (
	"errors"
	"fmt"
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
)

var (
	crBranchPattern  = regexp.MustCompile(`^sophia/cr-(\d+)$`)
	crSubjectPattern = regexp.MustCompile(`^\[CR-(\d+)\]`)
	crFooterPattern  = regexp.MustCompile(`(?m)^Sophia-CR:\s*\d+\s*$`)
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

func (s *Service) Init(baseBranch string) (string, error) {
	if !s.git.InRepo() {
		if err := s.git.InitRepo(); err != nil {
			return "", fmt.Errorf("initialize git repository: %w", err)
		}
	}

	wasInitialized := s.store.IsInitialized()
	effectiveBase := strings.TrimSpace(baseBranch)
	if effectiveBase == "" && wasInitialized {
		cfg, err := s.store.LoadConfig()
		if err == nil && strings.TrimSpace(cfg.BaseBranch) != "" {
			effectiveBase = cfg.BaseBranch
		}
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
	if err := s.store.Init(configBase); err != nil {
		return "", err
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
	cr, err := s.store.LoadCR(crID)
	if err != nil {
		return err
	}
	now := s.timestamp()
	actor := s.git.Actor()
	found := false
	title := ""
	for i := range cr.Subtasks {
		if cr.Subtasks[i].ID == taskID {
			found = true
			title = cr.Subtasks[i].Title
			cr.Subtasks[i].Status = model.TaskStatusDone
			cr.Subtasks[i].UpdatedAt = now
			cr.Subtasks[i].CompletedAt = now
			cr.Subtasks[i].CompletedBy = actor
			break
		}
	}
	if !found {
		return fmt.Errorf("task %d not found in cr %d", taskID, crID)
	}

	cr.Events = append(cr.Events, model.Event{
		TS:      now,
		Actor:   actor,
		Type:    "task_done",
		Summary: title,
		Ref:     fmt.Sprintf("task:%d", taskID),
	})
	cr.UpdatedAt = now

	return s.store.SaveCR(cr)
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

func (s *Service) MergeCR(id int, deleteBranch bool) (string, error) {
	cr, err := s.store.LoadCR(id)
	if err != nil {
		return "", err
	}
	if cr.Status == model.StatusMerged {
		return "", ErrCRAlreadyMerged
	}

	files, err := s.git.DiffNames(cr.BaseBranch, cr.Branch)
	if err != nil {
		return "", err
	}

	actor := s.git.Actor()
	mergedAt := s.timestamp()
	msg := buildMergeCommitMessage(cr, actor, mergedAt)
	if err := s.git.SquashMerge(cr.BaseBranch, cr.Branch, msg); err != nil {
		return "", err
	}

	if deleteBranch {
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
	if err != nil {
		return nil, err
	}

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
	return res, nil
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
		if entry.Code == "??" {
			untracked++
		} else {
			changed++
		}
	}
	return true, fmt.Sprintf("%d modified/staged and %d untracked paths; commit or stash before switching", changed, untracked), nil
}

func nonEmptyTrimmed(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return strings.TrimSpace(value)
}

func (s *Service) timestamp() string {
	return s.now().UTC().Format(time.RFC3339)
}
