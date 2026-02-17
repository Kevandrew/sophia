package service

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"sophia/internal/gitx"
	"sophia/internal/model"
	"sophia/internal/store"
)

var ErrCRAlreadyMerged = errors.New("cr is already merged")

type Service struct {
	store *store.Store
	git   *gitx.Client
	now   func() time.Time
}

type Review struct {
	CR        *model.CR
	Files     []string
	ShortStat string
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
	if strings.TrimSpace(title) == "" {
		return nil, errors.New("title cannot be empty")
	}
	if err := s.store.EnsureInitialized(); err != nil {
		return nil, err
	}

	cfg, err := s.store.LoadConfig()
	if err != nil {
		return nil, err
	}

	if err := s.git.EnsureBaseBranch(cfg.BaseBranch); err != nil {
		return nil, fmt.Errorf("ensure base branch: %w", err)
	}
	if err := s.git.EnsureBootstrapCommit("chore: bootstrap base branch for Sophia"); err != nil {
		return nil, fmt.Errorf("ensure bootstrap commit: %w", err)
	}

	id, err := s.store.NextCRID()
	if err != nil {
		return nil, err
	}

	branch := fmt.Sprintf("sophia/cr-%d", id)
	if s.git.BranchExists(branch) {
		return nil, fmt.Errorf("branch %q already exists", branch)
	}
	if err := s.git.CreateBranch(branch); err != nil {
		return nil, err
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
		return nil, err
	}

	return cr, nil
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
	files, err := s.git.DiffNames(cr.BaseBranch, cr.Branch)
	if err != nil {
		return nil, err
	}
	shortStat, err := s.git.DiffShortStat(cr.BaseBranch, cr.Branch)
	if err != nil {
		return nil, err
	}
	return &Review{
		CR:        cr,
		Files:     files,
		ShortStat: shortStat,
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

	cr.Status = model.StatusMerged
	cr.UpdatedAt = mergedAt
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

	sha, err := s.git.HeadShortSHA()
	if err != nil {
		return "", err
	}
	return sha, nil
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
	return b.String()
}

func (s *Service) timestamp() string {
	return s.now().UTC().Format(time.RFC3339)
}
