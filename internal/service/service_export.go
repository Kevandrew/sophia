package service

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"sophia/internal/model"
)

const (
	exportFormatJSON   = "json"
	exportIncludeDiffs = "diffs"
	exportSchemaV1     = "sophia.cr_bundle.v1"
)

func (s *Service) ExportCRBundle(id int, opts ExportCROptions) (*CRExportBundle, []byte, error) {
	format := strings.ToLower(strings.TrimSpace(opts.Format))
	if format == "" {
		format = exportFormatJSON
	}
	if format != exportFormatJSON {
		return nil, nil, fmt.Errorf("unsupported export format %q (supported: json)", format)
	}

	includes, err := normalizeExportIncludes(opts.Include)
	if err != nil {
		return nil, nil, err
	}

	review, err := s.ReviewCR(id)
	if err != nil {
		return nil, nil, err
	}
	if review == nil || review.CR == nil {
		return nil, nil, fmt.Errorf("cr %d is unavailable", id)
	}

	crPath := s.store.CRPath(id)
	rawCRYAML, readErr := os.ReadFile(crPath)
	if readErr != nil {
		return nil, nil, fmt.Errorf("read cr yaml %s: %w", crPath, readErr)
	}

	checkpoints := make([]CRExportCheckpoint, 0, len(review.CR.Subtasks))
	referencedCommitSet := map[string]struct{}{}
	baseCommit := strings.TrimSpace(review.CR.BaseCommit)
	if baseCommit != "" {
		referencedCommitSet[baseCommit] = struct{}{}
	}
	mergedCommit := strings.TrimSpace(review.CR.MergedCommit)
	if mergedCommit != "" {
		referencedCommitSet[mergedCommit] = struct{}{}
	}

	for _, task := range review.CR.Subtasks {
		commit := strings.TrimSpace(task.CheckpointCommit)
		if commit != "" {
			referencedCommitSet[commit] = struct{}{}
		}
		scope := append([]string(nil), task.CheckpointScope...)
		chunks := append([]model.CheckpointChunk(nil), task.CheckpointChunks...)
		checkpoints = append(checkpoints, CRExportCheckpoint{
			TaskID:  task.ID,
			Title:   task.Title,
			Status:  task.Status,
			Commit:  commit,
			At:      strings.TrimSpace(task.CheckpointAt),
			Message: strings.TrimSpace(task.CheckpointMessage),
			Scope:   scope,
			Chunks:  chunks,
			Source:  strings.TrimSpace(task.CheckpointSource),
			Orphan:  task.CheckpointOrphan,
			Reason:  strings.TrimSpace(task.CheckpointReason),
		})
	}
	sort.Slice(checkpoints, func(i, j int) bool { return checkpoints[i].TaskID < checkpoints[j].TaskID })

	referencedCommits := make([]string, 0, len(referencedCommitSet))
	for commit := range referencedCommitSet {
		referencedCommits = append(referencedCommits, commit)
	}
	sort.Strings(referencedCommits)

	validation := &ValidationReport{
		Valid:    len(review.ValidationErrors) == 0,
		Errors:   append([]string(nil), review.ValidationErrors...),
		Warnings: append([]string(nil), review.ValidationWarnings...),
		Impact:   review.Impact,
	}

	bundle := &CRExportBundle{
		SchemaVersion: exportSchemaV1,
		Format:        format,
		CR:            review.CR,
		CRYAML:        string(rawCRYAML),
		Evidence:      append([]model.EvidenceEntry(nil), review.CR.Evidence...),
		Derived: CRExportDerived{
			FilesChanged:    append([]string(nil), review.Files...),
			NewFiles:        append([]string(nil), review.NewFiles...),
			ModifiedFiles:   append([]string(nil), review.ModifiedFiles...),
			DeletedFiles:    append([]string(nil), review.DeletedFiles...),
			TestFiles:       append([]string(nil), review.TestFiles...),
			DependencyFiles: append([]string(nil), review.DependencyFiles...),
			DiffStat:        strings.TrimSpace(review.ShortStat),
			Impact:          review.Impact,
			Trust:           review.Trust,
			Validation:      validation,
		},
		Checkpoints:       checkpoints,
		ReferencedCommits: referencedCommits,
		Includes:          includes,
		Warnings:          []string{},
	}

	if includesContain(includes, exportIncludeDiffs) {
		taskDiffs, warnings := s.exportTaskDiffs(review.CR.Subtasks)
		bundle.TaskDiffs = taskDiffs
		bundle.Warnings = append(bundle.Warnings, warnings...)
	}

	payload, err := json.MarshalIndent(bundle, "", "  ")
	if err != nil {
		return nil, nil, fmt.Errorf("marshal export bundle: %w", err)
	}
	return bundle, payload, nil
}

func normalizeExportIncludes(raw []string) ([]string, error) {
	if len(raw) == 0 {
		return []string{}, nil
	}
	allowed := map[string]struct{}{
		exportIncludeDiffs: {},
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		normalized := strings.ToLower(strings.TrimSpace(item))
		if normalized == "" {
			continue
		}
		if _, ok := allowed[normalized]; !ok {
			return nil, fmt.Errorf("unsupported --include value %q (supported: diffs)", item)
		}
		if _, exists := seen[normalized]; exists {
			continue
		}
		seen[normalized] = struct{}{}
		out = append(out, normalized)
	}
	sort.Strings(out)
	return out, nil
}

func includesContain(items []string, target string) bool {
	target = strings.TrimSpace(target)
	if target == "" {
		return false
	}
	for _, item := range items {
		if strings.TrimSpace(item) == target {
			return true
		}
	}
	return false
}

func (s *Service) exportTaskDiffs(tasks []model.Subtask) ([]CRExportTaskDiff, []string) {
	diffs := make([]CRExportTaskDiff, 0, len(tasks))
	warnings := []string{}
	for _, task := range tasks {
		commit := strings.TrimSpace(task.CheckpointCommit)
		if commit == "" {
			continue
		}
		files, filesErr := s.git.CommitFiles(commit)
		if filesErr != nil {
			warnings = append(warnings, fmt.Sprintf("task #%d: unable to list files for checkpoint %s: %v", task.ID, shortHash(commit), filesErr))
		}
		patch, patchErr := s.git.CommitPatch(commit)
		if patchErr != nil {
			warnings = append(warnings, fmt.Sprintf("task #%d: unable to render patch for checkpoint %s: %v", task.ID, shortHash(commit), patchErr))
		}
		diffs = append(diffs, CRExportTaskDiff{
			TaskID: task.ID,
			Title:  task.Title,
			Commit: commit,
			Files:  files,
			Patch:  patch,
		})
	}
	sort.Slice(diffs, func(i, j int) bool { return diffs[i].TaskID < diffs[j].TaskID })
	return diffs, warnings
}
