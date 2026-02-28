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
	ID                int                      `json:"id"`
	UID               string                   `json:"uid,omitempty"`
	Title             string                   `json:"title"`
	Description       string                   `json:"description"`
	Status            string                   `json:"status"`
	BaseBranch        string                   `json:"base_branch"`
	BaseRef           string                   `json:"base_ref,omitempty"`
	BaseCommit        string                   `json:"base_commit,omitempty"`
	ParentCRID        int                      `json:"parent_cr_id,omitempty"`
	Branch            string                   `json:"branch"`
	Notes             []string                 `json:"notes"`
	Evidence          []model.EvidenceEntry    `json:"evidence,omitempty"`
	Contract          model.Contract           `json:"contract,omitempty"`
	ContractBaseline  model.CRContractBaseline `json:"contract_baseline,omitempty"`
	ContractDrifts    []model.CRContractDrift  `json:"contract_drifts,omitempty"`
	Subtasks          []model.Subtask          `json:"subtasks"`
	Events            []CRDocEvent             `json:"events"`
	MergedAt          string                   `json:"merged_at,omitempty"`
	MergedBy          string                   `json:"merged_by,omitempty"`
	MergedCommit      string                   `json:"merged_commit,omitempty"`
	FilesTouchedCount int                      `json:"files_touched_count,omitempty"`
	CreatedAt         string                   `json:"created_at"`
	UpdatedAt         string                   `json:"updated_at"`
}

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
	doc := canonicalCRDoc(review.CR)
	fingerprint, fpErr := fingerprintCRDoc(doc)
	if fpErr != nil {
		return nil, nil, fmt.Errorf("fingerprint cr doc: %w", fpErr)
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
	var anchors *CRExportAnchors
	if resolved, anchorErr := s.resolveCRAnchors(review.CR); anchorErr == nil && resolved != nil {
		anchors = &CRExportAnchors{
			BaseRef:    strings.TrimSpace(resolved.baseRef),
			BaseCommit: strings.TrimSpace(resolved.baseCommit),
			HeadRef:    strings.TrimSpace(resolved.headRef),
			HeadCommit: strings.TrimSpace(resolved.headCommit),
			MergeBase:  strings.TrimSpace(resolved.mergeBase),
		}
	}

	bundle := &CRExportBundle{
		SchemaVersion:    exportSchemaV1,
		Format:           format,
		CRUID:            strings.TrimSpace(review.CR.UID),
		CRFingerprint:    fingerprint,
		DocSchemaVersion: crDocSchemaV1,
		Doc:              doc,
		Anchors:          anchors,
		CR:               review.CR,
		CRYAML:           string(rawCRYAML),
		Evidence:         append([]model.EvidenceEntry(nil), review.CR.Evidence...),
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
