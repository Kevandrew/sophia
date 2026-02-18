package service

import (
	"fmt"
	"sort"
	"strings"

	"sophia/internal/model"
)

func (s *Service) DiffCR(id int, opts CRDiffOptions) (*CRDiffView, error) {
	cr, err := s.store.LoadCR(id)
	if err != nil {
		return nil, err
	}
	if _, err := s.ensureCRBaseFields(cr, true); err != nil {
		return nil, err
	}
	if opts.TaskID > 0 {
		view, err := s.diffTaskFromCR(cr, opts.TaskID, TaskDiffOptions{CriticalOnly: opts.CriticalOnly})
		if err != nil {
			return nil, err
		}
		view.Mode = "task"
		return view, nil
	}

	fromRef, targetRef, warnings, err := s.crDiffAnchors(cr)
	if err != nil {
		return nil, err
	}
	patch, err := s.git.DiffPatchBetween(fromRef, targetRef, nil, 3)
	if err != nil {
		return nil, err
	}
	parsed, err := parsePatchChunks(patch)
	if err != nil {
		return nil, fmt.Errorf("parse CR patch: %w", err)
	}
	files := buildDiffFiles(parsed, "cr_range")
	shortStat, statErr := s.git.DiffShortStatBetween(fromRef, targetRef)
	if statErr != nil {
		shortStat = fmt.Sprintf("%d file(s) changed", len(files))
	}
	view := &CRDiffView{
		CRID:         cr.ID,
		Mode:         "cr",
		CriticalOnly: opts.CriticalOnly,
		BaseRef:      strings.TrimSpace(nonEmptyTrimmed(cr.BaseRef, cr.BaseBranch)),
		BaseCommit:   fromRef,
		TargetRef:    targetRef,
		Files:        files,
		FilesChanged: len(files),
		ShortStat:    shortStat,
		Warnings:     append([]string(nil), warnings...),
	}
	if opts.CriticalOnly {
		applyCriticalFilter(view, cr.Contract.RiskCriticalScopes)
	}
	return view, nil
}

func (s *Service) DiffTask(crID, taskID int, opts TaskDiffOptions) (*CRDiffView, error) {
	cr, err := s.store.LoadCR(crID)
	if err != nil {
		return nil, err
	}
	if _, err := s.ensureCRBaseFields(cr, true); err != nil {
		return nil, err
	}
	return s.diffTaskFromCR(cr, taskID, opts)
}

func (s *Service) diffTaskFromCR(cr *model.CR, taskID int, opts TaskDiffOptions) (*CRDiffView, error) {
	if cr == nil {
		return nil, fmt.Errorf("cr is required")
	}
	taskIdx := indexOfTask(cr.Subtasks, taskID)
	if taskIdx < 0 {
		return nil, fmt.Errorf("task %d not found in cr %d", taskID, cr.ID)
	}
	task := cr.Subtasks[taskIdx]
	view := &CRDiffView{
		CRID:         cr.ID,
		TaskID:       task.ID,
		Mode:         "task",
		ChunksOnly:   opts.ChunksOnly,
		CriticalOnly: opts.CriticalOnly,
		BaseRef:      strings.TrimSpace(cr.BaseRef),
		BaseCommit:   strings.TrimSpace(cr.BaseCommit),
		Warnings:     []string{},
	}

	commit := strings.TrimSpace(task.CheckpointCommit)
	if commit != "" {
		patch, err := s.git.CommitPatch(commit)
		if err == nil {
			parsed, parseErr := parsePatchChunks(patch)
			if parseErr != nil {
				return nil, fmt.Errorf("parse task checkpoint patch: %w", parseErr)
			}
			view.TargetRef = commit
			view.Files = buildDiffFiles(parsed, "checkpoint_commit")
			view.FilesChanged = len(view.Files)
			if shortStat, statErr := s.git.DiffShortStatBetween(commit+"^", commit); statErr == nil {
				view.ShortStat = shortStat
			} else {
				view.ShortStat = fmt.Sprintf("%d file(s) changed", view.FilesChanged)
			}
			if opts.CriticalOnly {
				applyCriticalFilter(view, cr.Contract.RiskCriticalScopes)
			}
			return view, nil
		}
	}

	if len(task.CheckpointScope) == 0 {
		return nil, fmt.Errorf("task %d has no checkpoint commit and no checkpoint_scope for fallback diff", task.ID)
	}
	fromRef, targetRef, anchorWarnings, err := s.crDiffAnchors(cr)
	if err != nil {
		return nil, err
	}
	paths := normalizeNonEmptyStringList(task.CheckpointScope)
	patch, err := s.git.DiffPatchBetween(fromRef, targetRef, paths, 3)
	if err != nil {
		return nil, err
	}
	parsed, err := parsePatchChunks(patch)
	if err != nil {
		return nil, fmt.Errorf("parse task fallback patch: %w", err)
	}
	view.Mode = "task"
	view.TargetRef = targetRef
	view.Files = buildDiffFiles(parsed, "task_scope_fallback")
	view.FilesChanged = len(view.Files)
	view.ShortStat = fmt.Sprintf("%d file(s) changed (task checkpoint scope fallback)", view.FilesChanged)
	view.FallbackUsed = true
	view.FallbackReason = "task checkpoint commit unavailable; using CR range filtered by task checkpoint_scope"
	view.Warnings = append(view.Warnings, anchorWarnings...)
	view.Warnings = append(view.Warnings, view.FallbackReason)
	if opts.CriticalOnly {
		applyCriticalFilter(view, cr.Contract.RiskCriticalScopes)
	}
	return view, nil
}

func (s *Service) DiffTaskChunk(crID, taskID int, chunkID string) (*CRDiffView, error) {
	chunkID = strings.TrimSpace(chunkID)
	if chunkID == "" {
		return nil, fmt.Errorf("chunk id is required")
	}
	cr, err := s.store.LoadCR(crID)
	if err != nil {
		return nil, err
	}
	if _, err := s.ensureCRBaseFields(cr, true); err != nil {
		return nil, err
	}
	taskIdx := indexOfTask(cr.Subtasks, taskID)
	if taskIdx < 0 {
		return nil, fmt.Errorf("task %d not found in cr %d", taskID, crID)
	}
	task := cr.Subtasks[taskIdx]
	commit := strings.TrimSpace(task.CheckpointCommit)
	if commit == "" {
		return nil, fmt.Errorf("task %d has no checkpoint commit for chunk diff", taskID)
	}
	patch, err := s.git.CommitPatch(commit)
	if err != nil {
		return nil, err
	}
	parsed, err := parsePatchChunks(patch)
	if err != nil {
		return nil, fmt.Errorf("parse task checkpoint patch: %w", err)
	}
	if len(parsed) == 0 {
		return nil, fmt.Errorf("no checkpoint chunks available in commit %s", shortHash(commit))
	}

	selected := parsedPatchChunk{}
	found := false
	source := "checkpoint_chunk_derived"
	for _, meta := range task.CheckpointChunks {
		if strings.TrimSpace(meta.ID) != chunkID {
			continue
		}
		for _, chunk := range parsed {
			if chunk.ID == chunkID {
				selected = chunk
				found = true
				source = "checkpoint_chunk_metadata"
				break
			}
		}
		if found {
			break
		}
		for _, chunk := range parsed {
			if strings.TrimSpace(chunk.Path) == strings.TrimSpace(meta.Path) &&
				chunk.OldStart == meta.OldStart &&
				chunk.NewStart == meta.NewStart {
				selected = chunk
				found = true
				source = "checkpoint_chunk_metadata_fallback"
				break
			}
		}
		if found {
			break
		}
	}
	if !found {
		for _, chunk := range parsed {
			if chunk.ID == chunkID {
				selected = chunk
				found = true
				source = "checkpoint_chunk_derived"
				break
			}
		}
	}
	if !found {
		return nil, fmt.Errorf("chunk %q not found for task %d; run `sophia cr task chunk list %d %d` to inspect chunk ids", chunkID, taskID, crID, taskID)
	}

	hunk := DiffHunkView{
		ChunkID:  selected.ID,
		Path:     selected.Path,
		OldStart: selected.OldStart,
		OldLines: selected.OldLines,
		NewStart: selected.NewStart,
		NewLines: selected.NewLines,
		Header:   selected.Header,
		Preview:  selected.Preview,
		Source:   source,
	}
	view := &CRDiffView{
		CRID:         cr.ID,
		TaskID:       task.ID,
		Mode:         "chunk",
		ChunksOnly:   true,
		BaseRef:      strings.TrimSpace(cr.BaseRef),
		BaseCommit:   strings.TrimSpace(cr.BaseCommit),
		TargetRef:    commit,
		Files:        []DiffFileView{{Path: selected.Path, Hunks: []DiffHunkView{hunk}}},
		FilesChanged: 1,
		ShortStat:    "1 file(s) changed (chunk view)",
		Warnings:     []string{},
	}
	if source != "checkpoint_chunk_metadata" {
		view.FallbackUsed = true
		view.FallbackReason = "chunk metadata incomplete; derived chunk mapping from checkpoint patch"
		view.Warnings = append(view.Warnings, view.FallbackReason)
	}
	return view, nil
}

func (s *Service) crDiffAnchors(cr *model.CR) (string, string, []string, error) {
	if cr == nil {
		return "", "", nil, fmt.Errorf("cr is required")
	}
	anchors, err := s.resolveCRAnchors(cr)
	if err != nil {
		return "", "", nil, err
	}
	return anchors.baseCommit, anchors.headCommit, append([]string(nil), anchors.warnings...), nil
}

func buildDiffFiles(chunks []parsedPatchChunk, source string) []DiffFileView {
	byPath := map[string][]DiffHunkView{}
	for _, chunk := range chunks {
		path := strings.TrimSpace(chunk.Path)
		if path == "" {
			continue
		}
		byPath[path] = append(byPath[path], DiffHunkView{
			ChunkID:  chunk.ID,
			Path:     path,
			OldStart: chunk.OldStart,
			OldLines: chunk.OldLines,
			NewStart: chunk.NewStart,
			NewLines: chunk.NewLines,
			Header:   chunk.Header,
			Preview:  chunk.Preview,
			Source:   source,
		})
	}
	paths := make([]string, 0, len(byPath))
	for path := range byPath {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	files := make([]DiffFileView, 0, len(paths))
	for _, path := range paths {
		hunks := byPath[path]
		sort.Slice(hunks, func(i, j int) bool {
			if hunks[i].Path != hunks[j].Path {
				return hunks[i].Path < hunks[j].Path
			}
			if hunks[i].OldStart != hunks[j].OldStart {
				return hunks[i].OldStart < hunks[j].OldStart
			}
			if hunks[i].NewStart != hunks[j].NewStart {
				return hunks[i].NewStart < hunks[j].NewStart
			}
			return hunks[i].ChunkID < hunks[j].ChunkID
		})
		files = append(files, DiffFileView{Path: path, Hunks: hunks})
	}
	return files
}

func applyCriticalFilter(view *CRDiffView, criticalScopes []string) {
	if view == nil {
		return
	}
	scopes := normalizeNonEmptyStringList(criticalScopes)
	if len(scopes) == 0 {
		view.Files = []DiffFileView{}
		view.FilesChanged = 0
		view.ShortStat = "0 file(s) changed (critical scope filter)"
		view.Warnings = append(view.Warnings, "no risk_critical_scopes configured; critical diff is empty")
		return
	}
	filtered := make([]DiffFileView, 0, len(view.Files))
	for _, file := range view.Files {
		include := false
		for _, scope := range scopes {
			if pathMatchesScopePrefix(file.Path, scope) {
				include = true
				break
			}
		}
		if include {
			filtered = append(filtered, file)
		}
	}
	view.Files = filtered
	view.FilesChanged = len(filtered)
	view.ShortStat = fmt.Sprintf("%d file(s) changed (critical scope filter)", len(filtered))
}
