package service

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"sophia/internal/model"
)

var rangeDiffLinePattern = regexp.MustCompile(`^(\S+):\s+(\S+)\s+([<>=!])\s+(\S+):\s+(\S+)\s+(.*)$`)

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

func (s *Service) RangeDiffCR(id int, opts RangeDiffOptions) (*RangeDiffView, error) {
	cr, err := s.store.LoadCR(id)
	if err != nil {
		return nil, err
	}
	if _, err := s.ensureCRBaseFields(cr, false); err != nil {
		return nil, err
	}
	fromRef, toRef, warnings, err := s.resolveRangeDiffAnchors(cr, nil, opts)
	if err != nil {
		return nil, err
	}
	return s.buildRangeDiffView(cr, nil, fromRef, toRef, warnings)
}

func (s *Service) RangeDiffTask(crID, taskID int, opts RangeDiffOptions) (*RangeDiffView, error) {
	cr, err := s.store.LoadCR(crID)
	if err != nil {
		return nil, err
	}
	if _, err := s.ensureCRBaseFields(cr, false); err != nil {
		return nil, err
	}
	taskIdx := indexOfTask(cr.Subtasks, taskID)
	if taskIdx < 0 {
		return nil, fmt.Errorf("task %d not found in cr %d", taskID, crID)
	}
	task := cr.Subtasks[taskIdx]
	fromRef, toRef, warnings, err := s.resolveRangeDiffAnchors(cr, &task, opts)
	if err != nil {
		return nil, err
	}
	return s.buildRangeDiffView(cr, &task, fromRef, toRef, warnings)
}

func (s *Service) buildRangeDiffView(cr *model.CR, task *model.Subtask, fromRef, toRef string, warnings []string) (*RangeDiffView, error) {
	baseRef, err := s.git.MergeBase(fromRef, toRef)
	if err != nil {
		return nil, fmt.Errorf("compute merge-base(%s, %s): %w", fromRef, toRef, err)
	}
	oldRange := fmt.Sprintf("%s..%s", baseRef, fromRef)
	newRange := fmt.Sprintf("%s..%s", baseRef, toRef)

	out, err := s.git.RangeDiff(oldRange, newRange)
	if err != nil {
		return nil, err
	}
	mapping, parseWarnings := parseRangeDiffMappings(out)
	warnings = append(warnings, parseWarnings...)

	filesChanged, filesErr := s.git.DiffNamesBetween(fromRef, toRef)
	if filesErr != nil {
		warnings = append(warnings, fmt.Sprintf("unable to list changed files between %s and %s: %v", shortHash(fromRef), shortHash(toRef), filesErr))
		filesChanged = []string{}
	}
	shortStat, statErr := s.git.DiffShortStatBetween(fromRef, toRef)
	if statErr != nil {
		warnings = append(warnings, fmt.Sprintf("unable to compute shortstat between %s and %s: %v", shortHash(fromRef), shortHash(toRef), statErr))
		shortStat = fmt.Sprintf("%d file(s) changed", len(filesChanged))
	}
	sort.Strings(filesChanged)

	view := &RangeDiffView{
		CRID:         cr.ID,
		FromRef:      fromRef,
		ToRef:        toRef,
		BaseRef:      baseRef,
		OldRange:     oldRange,
		NewRange:     newRange,
		Mapping:      mapping,
		FilesChanged: filesChanged,
		ShortStat:    shortStat,
		Warnings:     warnings,
	}
	if task != nil {
		view.TaskID = task.ID
	}
	return view, nil
}

func (s *Service) resolveRangeDiffAnchors(cr *model.CR, task *model.Subtask, opts RangeDiffOptions) (string, string, []string, error) {
	warnings := []string{}
	fromRaw := strings.TrimSpace(opts.FromRef)
	toRaw := strings.TrimSpace(opts.ToRef)
	if opts.SinceLastCheckpoint && fromRaw != "" {
		return "", "", nil, fmt.Errorf("--from and --since-last-checkpoint are mutually exclusive")
	}
	if !opts.SinceLastCheckpoint && fromRaw == "" {
		return "", "", nil, fmt.Errorf("either --from or --since-last-checkpoint is required")
	}

	fromResolved := ""
	if opts.SinceLastCheckpoint {
		latest, err := latestDoneCheckpointCommit(cr, task)
		if err != nil {
			return "", "", nil, err
		}
		fromResolved = latest
	} else {
		resolved, err := s.git.ResolveRef(fromRaw)
		if err != nil {
			return "", "", nil, fmt.Errorf("resolve --from ref %q: %w", fromRaw, err)
		}
		fromResolved = strings.TrimSpace(resolved)
	}

	toResolved := ""
	if toRaw != "" {
		resolved, err := s.git.ResolveRef(toRaw)
		if err != nil {
			return "", "", nil, fmt.Errorf("resolve --to ref %q: %w", toRaw, err)
		}
		toResolved = strings.TrimSpace(resolved)
	} else {
		anchors, err := s.resolveCRAnchors(cr)
		if err != nil {
			return "", "", nil, err
		}
		toResolved = strings.TrimSpace(anchors.headCommit)
		warnings = append(warnings, anchors.warnings...)
	}

	return fromResolved, toResolved, warnings, nil
}

func latestDoneCheckpointCommit(cr *model.CR, task *model.Subtask) (string, error) {
	type checkpointAnchor struct {
		commit string
		at     string
		taskID int
	}
	anchors := []checkpointAnchor{}
	appendTask := func(candidate model.Subtask) {
		if candidate.Status != model.TaskStatusDone {
			return
		}
		commit := strings.TrimSpace(candidate.CheckpointCommit)
		if commit == "" {
			return
		}
		anchors = append(anchors, checkpointAnchor{
			commit: commit,
			at:     strings.TrimSpace(candidate.CheckpointAt),
			taskID: candidate.ID,
		})
	}

	if task != nil {
		appendTask(*task)
		if len(anchors) == 0 {
			return "", fmt.Errorf("task %d has no done checkpoint commit for --since-last-checkpoint", task.ID)
		}
		return anchors[0].commit, nil
	}

	for _, subtask := range cr.Subtasks {
		appendTask(subtask)
	}
	if len(anchors) == 0 {
		return "", fmt.Errorf("cr %d has no done checkpoint commit for --since-last-checkpoint", cr.ID)
	}
	sort.Slice(anchors, func(i, j int) bool {
		ti := parseRFC3339OrZero(anchors[i].at)
		tj := parseRFC3339OrZero(anchors[j].at)
		if !ti.Equal(tj) {
			return tj.Before(ti)
		}
		if anchors[i].taskID != anchors[j].taskID {
			return anchors[i].taskID > anchors[j].taskID
		}
		return anchors[i].commit > anchors[j].commit
	})
	return anchors[0].commit, nil
}

func parseRangeDiffMappings(raw string) ([]RangeDiffCommitMap, []string) {
	raw = strings.ReplaceAll(raw, "\r\n", "\n")
	if strings.TrimSpace(raw) == "" {
		return []RangeDiffCommitMap{}, []string{"range-diff produced no commit mapping rows"}
	}
	lines := strings.Split(raw, "\n")
	rows := make([]RangeDiffCommitMap, 0, len(lines))
	warnings := []string{}
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		matches := rangeDiffLinePattern.FindStringSubmatch(trimmed)
		if len(matches) != 7 {
			warnings = append(warnings, fmt.Sprintf("unparsed range-diff line: %s", trimmed))
			continue
		}
		rows = append(rows, RangeDiffCommitMap{
			OldIndex:  normalizeRangeToken(matches[1]),
			OldCommit: normalizeRangeToken(matches[2]),
			Relation:  strings.TrimSpace(matches[3]),
			NewIndex:  normalizeRangeToken(matches[4]),
			NewCommit: normalizeRangeToken(matches[5]),
			Subject:   strings.TrimSpace(matches[6]),
		})
	}
	if len(rows) == 0 {
		warnings = append(warnings, "range-diff mapping parser returned no structured rows")
	}
	return rows, warnings
}

func normalizeRangeToken(token string) string {
	token = strings.TrimSpace(token)
	switch token {
	case "-", "-------":
		return ""
	default:
		return token
	}
}
