package service

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"sophia/internal/model"

	"gopkg.in/yaml.v3"
)

const (
	exportFormatHintAuto = model.CRBundleFormatAuto
	exportFormatJSON     = model.CRBundleFormatJSON
	exportFormatYAML     = model.CRBundleFormatYAML
	exportFormatNDJSON   = model.CRBundleFormatNDJSON

	exportIncludeDiffs       = model.CRBundleIncludeDiffs
	exportIncludeEvidence    = model.CRBundleIncludeEvidence
	exportIncludeEvents      = model.CRBundleIncludeEvents
	exportIncludeAnchors     = model.CRBundleIncludeAnchors
	exportIncludeCheckpoints = model.CRBundleIncludeCheckpoints
	exportIncludeTrust       = model.CRBundleIncludeTrust
	exportIncludeValidation  = model.CRBundleIncludeValidation

	exportSchemaV1 = model.CRBundleSchemaV1
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
	Sections          *CRExportSections     `json:"sections,omitempty"`
	Warnings          []string              `json:"warnings,omitempty"`
}

type CRExportSections struct {
	TaskDiffs   []CRExportTaskDiff    `json:"task_diffs,omitempty"`
	Evidence    []model.EvidenceEntry `json:"evidence,omitempty"`
	Events      []CRDocEvent          `json:"events,omitempty"`
	Anchors     *CRExportAnchors      `json:"anchors,omitempty"`
	Checkpoints []CRExportCheckpoint  `json:"checkpoints,omitempty"`
	Trust       *TrustReport          `json:"trust,omitempty"`
	Validation  *ValidationReport     `json:"validation,omitempty"`
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
	format, err := normalizeExportFormat(opts.Format)
	if err != nil {
		return nil, nil, err
	}

	includes, err := normalizeExportIncludeValues(opts.Include)
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

	bundle.Sections = buildExportSections(bundle, includes)

	payload, err := marshalExportPayload(bundle, format)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal export bundle: %w", err)
	}
	return bundle, payload, nil
}

func normalizeExportFormat(raw string) (string, error) {
	format := strings.ToLower(strings.TrimSpace(raw))
	if format == "" {
		return exportFormatJSON, nil
	}
	switch format {
	case exportFormatJSON, exportFormatYAML, exportFormatNDJSON:
		return format, nil
	default:
		return "", fmt.Errorf("unsupported export format %q (supported: %s)", raw, strings.Join(supportedExportFormats(), ","))
	}
}

func supportedExportFormats() []string {
	return []string{exportFormatJSON, exportFormatYAML, exportFormatNDJSON}
}

func supportedExportIncludes() []string {
	return []string{
		exportIncludeAnchors,
		exportIncludeCheckpoints,
		exportIncludeDiffs,
		exportIncludeEvidence,
		exportIncludeEvents,
		exportIncludeTrust,
		exportIncludeValidation,
	}
}

func normalizeExportIncludeValues(raw []string) ([]string, error) {
	if len(raw) == 0 {
		return []string{}, nil
	}
	allowed := map[string]struct{}{
		exportIncludeDiffs:       {},
		exportIncludeEvidence:    {},
		exportIncludeEvents:      {},
		exportIncludeAnchors:     {},
		exportIncludeCheckpoints: {},
		exportIncludeTrust:       {},
		exportIncludeValidation:  {},
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		normalized := strings.ToLower(strings.TrimSpace(item))
		if normalized == "" {
			continue
		}
		if _, ok := allowed[normalized]; !ok {
			return nil, fmt.Errorf("unsupported --include value %q (supported: %s)", item, strings.Join(supportedExportIncludes(), ","))
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

func normalizeExportIncludeMetadata(raw []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		normalized := strings.ToLower(strings.TrimSpace(item))
		if normalized == "" {
			continue
		}
		if _, exists := seen[normalized]; exists {
			continue
		}
		seen[normalized] = struct{}{}
		out = append(out, normalized)
	}
	sort.Strings(out)
	return out
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

func buildExportSections(bundle *CRExportBundle, includes []string) *CRExportSections {
	if bundle == nil || len(includes) == 0 {
		return nil
	}
	sections := &CRExportSections{}
	hasContent := false

	if includesContain(includes, exportIncludeDiffs) {
		sections.TaskDiffs = append([]CRExportTaskDiff(nil), bundle.TaskDiffs...)
		hasContent = true
	}
	if includesContain(includes, exportIncludeEvidence) {
		sections.Evidence = append([]model.EvidenceEntry(nil), bundle.Evidence...)
		hasContent = true
	}
	if includesContain(includes, exportIncludeEvents) {
		if bundle.Doc != nil {
			sections.Events = append([]CRDocEvent(nil), bundle.Doc.Events...)
		} else {
			sections.Events = []CRDocEvent{}
		}
		hasContent = true
	}
	if includesContain(includes, exportIncludeAnchors) {
		sections.Anchors = bundle.Anchors
		hasContent = true
	}
	if includesContain(includes, exportIncludeCheckpoints) {
		sections.Checkpoints = append([]CRExportCheckpoint(nil), bundle.Checkpoints...)
		hasContent = true
	}
	if includesContain(includes, exportIncludeTrust) {
		sections.Trust = bundle.Derived.Trust
		hasContent = true
	}
	if includesContain(includes, exportIncludeValidation) {
		sections.Validation = bundle.Derived.Validation
		hasContent = true
	}
	if !hasContent {
		return nil
	}
	return sections
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

func marshalExportPayload(bundle *CRExportBundle, format string) ([]byte, error) {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case exportFormatJSON:
		return marshalExportBundleJSON(bundle)
	case exportFormatYAML:
		return marshalExportBundleYAML(bundle)
	case exportFormatNDJSON:
		return marshalExportBundleNDJSON(bundle)
	default:
		return nil, fmt.Errorf("unsupported export format %q", format)
	}
}

func marshalExportBundleJSON(bundle *CRExportBundle) ([]byte, error) {
	return json.MarshalIndent(bundle, "", "  ")
}

func marshalExportBundleYAML(bundle *CRExportBundle) ([]byte, error) {
	jsonPayload, err := marshalExportBundleJSON(bundle)
	if err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(jsonPayload))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, fmt.Errorf("decode json export payload: %w", err)
	}
	value = normalizeExportYAMLValue(value)
	root := jsonValueToYAMLNode(value)
	if root == nil {
		root = &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!null", Value: "null"}
	}
	document := &yaml.Node{
		Kind:    yaml.DocumentNode,
		Content: []*yaml.Node{root},
	}
	return yaml.Marshal(document)
}

func jsonValueToYAMLNode(value any) *yaml.Node {
	switch typed := value.(type) {
	case map[string]any:
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		node := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		for _, key := range keys {
			node.Content = append(node.Content,
				&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key},
				jsonValueToYAMLNode(typed[key]),
			)
		}
		return node
	case []any:
		node := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
		for _, item := range typed {
			node.Content = append(node.Content, jsonValueToYAMLNode(item))
		}
		return node
	case json.Number:
		literal := typed.String()
		tag := "!!int"
		if strings.ContainsAny(literal, ".eE") {
			tag = "!!float"
		}
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: tag, Value: literal}
	case string:
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: typed}
	case bool:
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!bool", Value: strconv.FormatBool(typed)}
	case nil:
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!null", Value: "null"}
	case float64:
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!float", Value: strconv.FormatFloat(typed, 'g', -1, 64)}
	default:
		encoded, err := json.Marshal(typed)
		if err != nil {
			return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: fmt.Sprintf("%v", typed)}
		}
		var fallback any
		if err := json.Unmarshal(encoded, &fallback); err != nil {
			return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: fmt.Sprintf("%v", typed)}
		}
		return jsonValueToYAMLNode(fallback)
	}
}

func normalizeExportYAMLValue(value any) any {
	root, ok := value.(map[string]any)
	if !ok {
		return value
	}
	rawCRYAML, _ := root["cr_yaml"].(string)
	if strings.TrimSpace(rawCRYAML) == "" {
		return root
	}
	var crValue any
	if err := yaml.Unmarshal([]byte(rawCRYAML), &crValue); err != nil {
		return root
	}
	root["cr"] = normalizeYAMLValue(crValue)
	return root
}

func marshalExportBundleNDJSON(bundle *CRExportBundle) ([]byte, error) {
	records := make([]map[string]any, 0, 12)
	if bundle == nil {
		return []byte{}, nil
	}
	records = append(records, map[string]any{
		"type":               "meta",
		"schema_version":     bundle.SchemaVersion,
		"format":             bundle.Format,
		"cr_uid":             bundle.CRUID,
		"cr_fingerprint":     bundle.CRFingerprint,
		"doc_schema_version": bundle.DocSchemaVersion,
		"includes":           append([]string(nil), bundle.Includes...),
		"warnings":           append([]string(nil), bundle.Warnings...),
	})
	if bundle.Doc != nil {
		records = append(records, map[string]any{"type": "doc", "value": bundle.Doc})
	}
	if bundle.CR != nil {
		records = append(records, map[string]any{"type": "cr", "value": bundle.CR})
	}
	records = append(records, map[string]any{"type": "cr_yaml", "value": bundle.CRYAML})
	records = append(records, map[string]any{"type": "derived", "value": bundle.Derived})
	if bundle.Anchors != nil {
		records = append(records, map[string]any{"type": "anchors", "value": bundle.Anchors})
	}
	records = append(records, map[string]any{"type": "checkpoints", "value": append([]CRExportCheckpoint(nil), bundle.Checkpoints...)})
	records = append(records, map[string]any{"type": "referenced_commits", "value": append([]string(nil), bundle.ReferencedCommits...)})
	records = append(records, map[string]any{"type": "evidence", "value": append([]model.EvidenceEntry(nil), bundle.Evidence...)})
	if len(bundle.TaskDiffs) > 0 {
		records = append(records, map[string]any{"type": "task_diffs", "value": append([]CRExportTaskDiff(nil), bundle.TaskDiffs...)})
	}
	if bundle.Sections != nil {
		records = append(records, map[string]any{"type": "sections", "value": bundle.Sections})
	}

	var out bytes.Buffer
	for _, record := range records {
		line, err := json.Marshal(record)
		if err != nil {
			return nil, err
		}
		out.Write(line)
		out.WriteByte('\n')
	}
	return out.Bytes(), nil
}

func normalizeImportBundleFormat(raw string) (string, error) {
	format := strings.ToLower(strings.TrimSpace(raw))
	if format == "" {
		return exportFormatHintAuto, nil
	}
	switch format {
	case exportFormatHintAuto, exportFormatJSON, exportFormatYAML, exportFormatNDJSON:
		return format, nil
	default:
		return "", fmt.Errorf("unsupported import bundle format %q (supported: %s)", raw, strings.Join([]string{exportFormatHintAuto, exportFormatJSON, exportFormatYAML, exportFormatNDJSON}, ","))
	}
}

func decodeExportBundlePayload(raw []byte, path, formatHint string) (*CRExportBundle, error) {
	format, err := normalizeImportBundleFormat(formatHint)
	if err != nil {
		return nil, err
	}
	trimmedPath := strings.TrimSpace(path)
	if format != exportFormatHintAuto {
		return decodeExportBundleByFormat(raw, format)
	}
	errs := make([]string, 0, 3)
	for _, candidate := range candidateExportFormatsForAutoDetect(trimmedPath) {
		bundle, decodeErr := decodeExportBundleByFormat(raw, candidate)
		if decodeErr == nil {
			return bundle, nil
		}
		errs = append(errs, fmt.Sprintf("%s: %v", candidate, decodeErr))
	}
	return nil, fmt.Errorf("unable to decode bundle using auto-detected formats (%s)", strings.Join(errs, " | "))
}

func candidateExportFormatsForAutoDetect(path string) []string {
	switch strings.ToLower(strings.TrimSpace(filepath.Ext(path))) {
	case ".yaml", ".yml":
		return []string{exportFormatYAML, exportFormatJSON, exportFormatNDJSON}
	case ".ndjson":
		return []string{exportFormatNDJSON, exportFormatJSON, exportFormatYAML}
	case ".json":
		return []string{exportFormatJSON, exportFormatYAML, exportFormatNDJSON}
	default:
		return []string{exportFormatJSON, exportFormatYAML, exportFormatNDJSON}
	}
}

func decodeExportBundleByFormat(raw []byte, format string) (*CRExportBundle, error) {
	expected := strings.ToLower(strings.TrimSpace(format))
	switch strings.ToLower(strings.TrimSpace(format)) {
	case exportFormatJSON:
		bundle, err := decodeExportBundleJSON(raw)
		if err != nil {
			return nil, err
		}
		return ensureDecodedBundleFormat(bundle, expected)
	case exportFormatYAML:
		bundle, err := decodeExportBundleYAML(raw)
		if err != nil {
			return nil, err
		}
		return ensureDecodedBundleFormat(bundle, expected)
	case exportFormatNDJSON:
		bundle, err := decodeExportBundleNDJSON(raw)
		if err != nil {
			return nil, err
		}
		return ensureDecodedBundleFormat(bundle, expected)
	default:
		return nil, fmt.Errorf("unsupported format %q", format)
	}
}

func decodeExportBundleJSON(raw []byte) (*CRExportBundle, error) {
	var bundle CRExportBundle
	if err := json.Unmarshal(raw, &bundle); err != nil {
		return nil, fmt.Errorf("decode as json: %w", err)
	}
	finalizeDecodedBundle(&bundle, exportFormatJSON)
	if strings.TrimSpace(bundle.SchemaVersion) == "" {
		return nil, fmt.Errorf("decode as json: missing schema_version")
	}
	return &bundle, nil
}

func decodeExportBundleYAML(raw []byte) (*CRExportBundle, error) {
	var value any
	if err := yaml.Unmarshal(raw, &value); err != nil {
		return nil, fmt.Errorf("decode as yaml: %w", err)
	}
	normalized := normalizeYAMLValue(value)
	jsonPayload, err := json.Marshal(normalized)
	if err != nil {
		return nil, fmt.Errorf("decode as yaml: normalize payload: %w", err)
	}
	var bundle CRExportBundle
	if err := json.Unmarshal(jsonPayload, &bundle); err != nil {
		return nil, fmt.Errorf("decode as yaml: re-decode normalized payload: %w", err)
	}
	finalizeDecodedBundle(&bundle, exportFormatYAML)
	if strings.TrimSpace(bundle.SchemaVersion) == "" {
		return nil, fmt.Errorf("decode as yaml: missing schema_version")
	}
	return &bundle, nil
}

func normalizeYAMLValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(typed))
		for key, item := range typed {
			out[key] = normalizeYAMLValue(item)
		}
		return out
	case map[any]any:
		out := make(map[string]any, len(typed))
		for key, item := range typed {
			out[fmt.Sprintf("%v", key)] = normalizeYAMLValue(item)
		}
		return out
	case []any:
		out := make([]any, len(typed))
		for i, item := range typed {
			out[i] = normalizeYAMLValue(item)
		}
		return out
	default:
		return typed
	}
}

func decodeExportBundleNDJSON(raw []byte) (*CRExportBundle, error) {
	reader := bufio.NewReader(bytes.NewReader(raw))
	var bundle CRExportBundle
	hasMeta := false
	lineNo := 0
	seenSingleton := map[string]struct{}{}
	for {
		line, err := reader.ReadString('\n')
		if err != nil && err != io.EOF {
			return nil, fmt.Errorf("scan ndjson payload: %w", err)
		}
		if len(line) == 0 && err == io.EOF {
			break
		}
		lineNo++
		line = strings.TrimSpace(line)
		if line == "" {
			if err == io.EOF {
				break
			}
			continue
		}
		var payload map[string]json.RawMessage
		if err := json.Unmarshal([]byte(line), &payload); err != nil {
			return nil, fmt.Errorf("decode ndjson line %d: %w", lineNo, err)
		}
		recordType, err := ndjsonRecordType(payload)
		if err != nil {
			return nil, fmt.Errorf("decode ndjson line %d: %w", lineNo, err)
		}
		switch recordType {
		case "meta":
			if _, exists := seenSingleton[recordType]; exists {
				return nil, fmt.Errorf("decode ndjson line %d: duplicate record type %q", lineNo, recordType)
			}
			if err := decodeNDJSONMeta(payload, &bundle); err != nil {
				return nil, fmt.Errorf("decode ndjson line %d: %w", lineNo, err)
			}
			seenSingleton[recordType] = struct{}{}
			hasMeta = true
		case "doc":
			if _, exists := seenSingleton[recordType]; exists {
				return nil, fmt.Errorf("decode ndjson line %d: duplicate record type %q", lineNo, recordType)
			}
			doc, err := decodeNDJSONValue[CRDoc](payload)
			if err != nil {
				return nil, fmt.Errorf("decode ndjson line %d: %w", lineNo, err)
			}
			bundle.Doc = &doc
			seenSingleton[recordType] = struct{}{}
		case "cr":
			if _, exists := seenSingleton[recordType]; exists {
				return nil, fmt.Errorf("decode ndjson line %d: duplicate record type %q", lineNo, recordType)
			}
			cr, err := decodeNDJSONValue[model.CR](payload)
			if err != nil {
				return nil, fmt.Errorf("decode ndjson line %d: %w", lineNo, err)
			}
			bundle.CR = &cr
			seenSingleton[recordType] = struct{}{}
		case "cr_yaml":
			if _, exists := seenSingleton[recordType]; exists {
				return nil, fmt.Errorf("decode ndjson line %d: duplicate record type %q", lineNo, recordType)
			}
			value, err := decodeNDJSONValue[string](payload)
			if err != nil {
				return nil, fmt.Errorf("decode ndjson line %d: %w", lineNo, err)
			}
			bundle.CRYAML = value
			seenSingleton[recordType] = struct{}{}
		case "derived":
			if _, exists := seenSingleton[recordType]; exists {
				return nil, fmt.Errorf("decode ndjson line %d: duplicate record type %q", lineNo, recordType)
			}
			value, err := decodeNDJSONValue[CRExportDerived](payload)
			if err != nil {
				return nil, fmt.Errorf("decode ndjson line %d: %w", lineNo, err)
			}
			bundle.Derived = value
			seenSingleton[recordType] = struct{}{}
		case "anchors":
			if _, exists := seenSingleton[recordType]; exists {
				return nil, fmt.Errorf("decode ndjson line %d: duplicate record type %q", lineNo, recordType)
			}
			value, err := decodeNDJSONValue[CRExportAnchors](payload)
			if err != nil {
				return nil, fmt.Errorf("decode ndjson line %d: %w", lineNo, err)
			}
			bundle.Anchors = &value
			seenSingleton[recordType] = struct{}{}
		case "checkpoints":
			if _, exists := seenSingleton[recordType]; exists {
				return nil, fmt.Errorf("decode ndjson line %d: duplicate record type %q", lineNo, recordType)
			}
			value, err := decodeNDJSONValue[[]CRExportCheckpoint](payload)
			if err != nil {
				return nil, fmt.Errorf("decode ndjson line %d: %w", lineNo, err)
			}
			bundle.Checkpoints = value
			seenSingleton[recordType] = struct{}{}
		case "referenced_commits":
			if _, exists := seenSingleton[recordType]; exists {
				return nil, fmt.Errorf("decode ndjson line %d: duplicate record type %q", lineNo, recordType)
			}
			value, err := decodeNDJSONValue[[]string](payload)
			if err != nil {
				return nil, fmt.Errorf("decode ndjson line %d: %w", lineNo, err)
			}
			bundle.ReferencedCommits = value
			seenSingleton[recordType] = struct{}{}
		case "evidence":
			if _, exists := seenSingleton[recordType]; exists {
				return nil, fmt.Errorf("decode ndjson line %d: duplicate record type %q", lineNo, recordType)
			}
			value, err := decodeNDJSONValue[[]model.EvidenceEntry](payload)
			if err != nil {
				return nil, fmt.Errorf("decode ndjson line %d: %w", lineNo, err)
			}
			bundle.Evidence = value
			seenSingleton[recordType] = struct{}{}
		case "task_diffs":
			if _, exists := seenSingleton[recordType]; exists {
				return nil, fmt.Errorf("decode ndjson line %d: duplicate record type %q", lineNo, recordType)
			}
			value, err := decodeNDJSONValue[[]CRExportTaskDiff](payload)
			if err != nil {
				return nil, fmt.Errorf("decode ndjson line %d: %w", lineNo, err)
			}
			bundle.TaskDiffs = value
			seenSingleton[recordType] = struct{}{}
		case "sections":
			if _, exists := seenSingleton[recordType]; exists {
				return nil, fmt.Errorf("decode ndjson line %d: duplicate record type %q", lineNo, recordType)
			}
			value, err := decodeNDJSONValue[CRExportSections](payload)
			if err != nil {
				return nil, fmt.Errorf("decode ndjson line %d: %w", lineNo, err)
			}
			bundle.Sections = &value
			seenSingleton[recordType] = struct{}{}
		default:
			return nil, fmt.Errorf("decode ndjson line %d: unsupported record type %q", lineNo, recordType)
		}
		if err == io.EOF {
			break
		}
	}
	if !hasMeta {
		return nil, fmt.Errorf("decode as ndjson: missing meta record")
	}
	finalizeDecodedBundle(&bundle, exportFormatNDJSON)
	if strings.TrimSpace(bundle.SchemaVersion) == "" {
		return nil, fmt.Errorf("decode as ndjson: missing schema_version")
	}
	return &bundle, nil
}

func ndjsonRecordType(payload map[string]json.RawMessage) (string, error) {
	rawType, ok := payload["type"]
	if !ok {
		return "", fmt.Errorf("missing \"type\"")
	}
	var recordType string
	if err := json.Unmarshal(rawType, &recordType); err != nil {
		return "", fmt.Errorf("decode \"type\": %w", err)
	}
	recordType = strings.TrimSpace(recordType)
	if recordType == "" {
		return "", fmt.Errorf("empty \"type\"")
	}
	return recordType, nil
}

func decodeNDJSONMeta(payload map[string]json.RawMessage, bundle *CRExportBundle) error {
	if bundle == nil {
		return fmt.Errorf("bundle is required")
	}
	type metaRecord struct {
		SchemaVersion    string   `json:"schema_version"`
		Format           string   `json:"format"`
		CRUID            string   `json:"cr_uid"`
		CRFingerprint    string   `json:"cr_fingerprint"`
		DocSchemaVersion string   `json:"doc_schema_version"`
		Includes         []string `json:"includes"`
		Warnings         []string `json:"warnings"`
	}
	var meta metaRecord
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(raw, &meta); err != nil {
		return err
	}
	bundle.SchemaVersion = strings.TrimSpace(meta.SchemaVersion)
	bundle.Format = strings.ToLower(strings.TrimSpace(meta.Format))
	bundle.CRUID = strings.TrimSpace(meta.CRUID)
	bundle.CRFingerprint = strings.TrimSpace(meta.CRFingerprint)
	bundle.DocSchemaVersion = strings.TrimSpace(meta.DocSchemaVersion)
	bundle.Includes = append([]string(nil), meta.Includes...)
	bundle.Warnings = append([]string(nil), meta.Warnings...)
	return nil
}

func decodeNDJSONValue[T any](payload map[string]json.RawMessage) (T, error) {
	var zero T
	raw, ok := payload["value"]
	if !ok {
		return zero, fmt.Errorf("missing \"value\"")
	}
	var out T
	if err := json.Unmarshal(raw, &out); err != nil {
		return zero, err
	}
	return out, nil
}

func finalizeDecodedBundle(bundle *CRExportBundle, detectedFormat string) {
	if bundle == nil {
		return
	}
	bundle.Format = strings.ToLower(strings.TrimSpace(bundle.Format))
	if bundle.Format == "" {
		bundle.Format = strings.TrimSpace(detectedFormat)
	}
	bundle.Includes = normalizeExportIncludeMetadata(bundle.Includes)
	if bundle.Warnings == nil {
		bundle.Warnings = []string{}
	}
}

func ensureDecodedBundleFormat(bundle *CRExportBundle, expected string) (*CRExportBundle, error) {
	if bundle == nil {
		return nil, fmt.Errorf("bundle is required")
	}
	declared := strings.ToLower(strings.TrimSpace(bundle.Format))
	if declared == "" {
		return bundle, nil
	}
	if strings.ToLower(strings.TrimSpace(expected)) != declared {
		return nil, fmt.Errorf("decoded bundle format %q does not match requested format %q", declared, expected)
	}
	return bundle, nil
}
