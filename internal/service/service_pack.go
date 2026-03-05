package service

import (
	"fmt"
	"sort"
	"strings"

	"sophia/internal/model"
)

const (
	defaultPackEventsLimit      = 20
	defaultPackCheckpointsLimit = 10
)

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
	DelegationRuns    []model.DelegationRun
	StackNativity     StackNativityView
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

func (s *Service) PackCR(id int, opts PackOptions) (*CRPackView, error) {
	eventsLimit, checkpointsLimit, err := normalizePackOptions(opts)
	if err != nil {
		return nil, err
	}
	review, err := s.ReviewCR(id)
	if err != nil {
		return nil, err
	}
	if review == nil || review.CR == nil {
		return nil, fmt.Errorf("cr %d is unavailable", id)
	}
	resolvedAnchors, err := s.resolveCRAnchorsWithOptions(review.CR, CRAnchorResolveOptions{AllowMetadataOnlyHeadFallback: true})
	if err != nil {
		return nil, err
	}
	anchors := &CRRangeAnchorsView{
		CRID:      review.CR.ID,
		Base:      resolvedAnchors.baseCommit,
		Head:      resolvedAnchors.headCommit,
		MergeBase: resolvedAnchors.mergeBase,
		Warnings:  append([]string(nil), resolvedAnchors.warnings...),
	}
	status, err := s.StatusCR(id)
	if err != nil {
		return nil, err
	}
	if status != nil {
		status.LifecycleState = nonEmptyTrimmed(status.LifecycleState, strings.TrimSpace(review.CR.Status))
	}

	events, eventsMeta := selectRecentEvents(review.CR.Events, eventsLimit)
	checkpoints, checkpointsMeta := selectRecentCheckpoints(review.CR.Subtasks, checkpointsLimit)
	validation := &ValidationReport{
		Valid:    len(review.ValidationErrors) == 0,
		Errors:   append([]string(nil), review.ValidationErrors...),
		Warnings: append([]string(nil), review.ValidationWarnings...),
		Impact:   review.Impact,
	}

	warnings := append([]string(nil), anchors.Warnings...)
	if eventsMeta.Truncated > 0 {
		warnings = appendUniqueString(warnings, fmt.Sprintf("events truncated: %d hidden", eventsMeta.Truncated))
	}
	if checkpointsMeta.Truncated > 0 {
		warnings = appendUniqueString(warnings, fmt.Sprintf("checkpoints truncated: %d hidden", checkpointsMeta.Truncated))
	}

	tasks := append([]model.Subtask(nil), review.CR.Subtasks...)
	return &CRPackView{
		CR:                review.CR,
		Contract:          review.CR.Contract,
		Tasks:             tasks,
		DelegationRuns:    cloneDelegationRunsForPack(review.CR.DelegationRuns),
		StackNativity:     s.stackNativityForCR(review.CR),
		Anchors:           anchors,
		Status:            status,
		RecentEvents:      events,
		EventsMeta:        eventsMeta,
		RecentCheckpoints: checkpoints,
		CheckpointsMeta:   checkpointsMeta,
		DiffStat:          strings.TrimSpace(review.ShortStat),
		FilesChanged:      append([]string(nil), review.Files...),
		Impact:            review.Impact,
		Validation:        validation,
		Trust:             review.Trust,
		Warnings:          warnings,
	}, nil
}

func cloneDelegationRunsForPack(runs []model.DelegationRun) []model.DelegationRun {
	if len(runs) == 0 {
		return nil
	}
	cloned := make([]model.DelegationRun, 0, len(runs))
	for _, run := range runs {
		cloned = append(cloned, cloneDelegationRun(run))
	}
	sort.SliceStable(cloned, func(i, j int) bool {
		ti := parseRFC3339OrZero(cloned[i].UpdatedAt)
		tj := parseRFC3339OrZero(cloned[j].UpdatedAt)
		if !ti.Equal(tj) {
			return tj.Before(ti)
		}
		return cloned[i].ID > cloned[j].ID
	})
	return cloned
}

func normalizePackOptions(opts PackOptions) (int, int, error) {
	if opts.EventsLimit < 0 {
		return 0, 0, fmt.Errorf("--events-limit must be >= 0")
	}
	if opts.CheckpointsLimit < 0 {
		return 0, 0, fmt.Errorf("--checkpoints-limit must be >= 0")
	}
	eventsLimit := opts.EventsLimit
	if eventsLimit == 0 {
		eventsLimit = defaultPackEventsLimit
	}
	checkpointsLimit := opts.CheckpointsLimit
	if checkpointsLimit == 0 {
		checkpointsLimit = defaultPackCheckpointsLimit
	}
	return eventsLimit, checkpointsLimit, nil
}

func selectRecentEvents(events []model.Event, limit int) ([]model.Event, PackSliceMeta) {
	total := len(events)
	if limit < 0 {
		limit = 0
	}
	if limit > total {
		limit = total
	}
	out := make([]model.Event, 0, limit)
	for i := total - 1; i >= total-limit; i-- {
		if i < 0 {
			break
		}
		out = append(out, events[i])
	}
	return out, PackSliceMeta{
		Total:     total,
		Returned:  len(out),
		Truncated: maxInt(total-len(out), 0),
	}
}

func selectRecentCheckpoints(tasks []model.Subtask, limit int) ([]CRPackCheckpoint, PackSliceMeta) {
	checkpoints := make([]CRPackCheckpoint, 0, len(tasks))
	for _, task := range tasks {
		commit := strings.TrimSpace(task.CheckpointCommit)
		if commit == "" {
			continue
		}
		checkpoints = append(checkpoints, CRPackCheckpoint{
			TaskID:  task.ID,
			Title:   task.Title,
			Status:  task.Status,
			Commit:  commit,
			At:      strings.TrimSpace(task.CheckpointAt),
			Message: strings.TrimSpace(task.CheckpointMessage),
			Scope:   append([]string(nil), task.CheckpointScope...),
			Source:  strings.TrimSpace(task.CheckpointSource),
			Orphan:  task.CheckpointOrphan,
			Reason:  strings.TrimSpace(task.CheckpointReason),
		})
	}
	sort.Slice(checkpoints, func(i, j int) bool {
		ti := parseRFC3339OrZero(checkpoints[i].At)
		tj := parseRFC3339OrZero(checkpoints[j].At)
		if !ti.Equal(tj) {
			return tj.Before(ti)
		}
		if checkpoints[i].TaskID != checkpoints[j].TaskID {
			return checkpoints[i].TaskID > checkpoints[j].TaskID
		}
		return checkpoints[i].Commit > checkpoints[j].Commit
	})

	total := len(checkpoints)
	if limit < 0 {
		limit = 0
	}
	if limit > total {
		limit = total
	}
	selected := append([]CRPackCheckpoint(nil), checkpoints[:limit]...)
	return selected, PackSliceMeta{
		Total:     total,
		Returned:  len(selected),
		Truncated: maxInt(total-len(selected), 0),
	}
}

func appendUniqueString(items []string, value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return items
	}
	for _, existing := range items {
		if strings.TrimSpace(existing) == value {
			return items
		}
	}
	return append(items, value)
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
