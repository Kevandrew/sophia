package service

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sophia/internal/model"

	"gopkg.in/yaml.v3"
)

func TestExportCRBundleDeterministicJSON(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")

	cr, err := svc.AddCR("Export bundle", "deterministic export")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	setValidContract(t, svc, cr.ID)

	task, err := svc.AddTask(cr.ID, "feat: export fixture")
	if err != nil {
		t.Fatalf("AddTask() error = %v", err)
	}
	intent := "Create checkpoint commit for export references."
	acceptance := []string{"checkpoint exists"}
	scope := []string{"export.txt"}
	if _, err := svc.SetTaskContract(cr.ID, task.ID, TaskContractPatch{
		Intent:             &intent,
		AcceptanceCriteria: &acceptance,
		Scope:              &scope,
	}); err != nil {
		t.Fatalf("SetTaskContract() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "export.txt"), []byte("export\n"), 0o644); err != nil {
		t.Fatalf("write export.txt: %v", err)
	}
	if _, err := svc.DoneTaskWithCheckpoint(cr.ID, task.ID, DoneTaskOptions{Checkpoint: true, FromContract: true}); err != nil {
		t.Fatalf("DoneTaskWithCheckpoint() error = %v", err)
	}

	bundle1, payload1, err := svc.ExportCRBundle(cr.ID, ExportCROptions{Format: "json"})
	if err != nil {
		t.Fatalf("ExportCRBundle(first) error = %v", err)
	}
	bundle2, payload2, err := svc.ExportCRBundle(cr.ID, ExportCROptions{Format: "json"})
	if err != nil {
		t.Fatalf("ExportCRBundle(second) error = %v", err)
	}
	if !bytes.Equal(payload1, payload2) {
		t.Fatalf("expected deterministic payload bytes\nfirst=%s\nsecond=%s", string(payload1), string(payload2))
	}
	if bundle1.SchemaVersion != exportSchemaV1 {
		t.Fatalf("expected schema %q, got %#v", exportSchemaV1, bundle1)
	}
	if len(bundle1.Checkpoints) != 1 || strings.TrimSpace(bundle1.Checkpoints[0].Commit) == "" {
		t.Fatalf("expected checkpoint metadata in export, got %#v", bundle1.Checkpoints)
	}
	if len(bundle1.ReferencedCommits) == 0 {
		t.Fatalf("expected referenced commits in export, got %#v", bundle1)
	}
	if bundle2.SchemaVersion != bundle1.SchemaVersion {
		t.Fatalf("expected matching schemas, got %q vs %q", bundle1.SchemaVersion, bundle2.SchemaVersion)
	}
}

func TestExportCRBundleIncludesTaskDiffs(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")

	cr, err := svc.AddCR("Export diffs", "include task patches")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	setValidContract(t, svc, cr.ID)

	task, err := svc.AddTask(cr.ID, "feat: diff export fixture")
	if err != nil {
		t.Fatalf("AddTask() error = %v", err)
	}
	intent := "Create task checkpoint patch."
	acceptance := []string{"patch renderable"}
	scope := []string{"diff.txt"}
	if _, err := svc.SetTaskContract(cr.ID, task.ID, TaskContractPatch{
		Intent:             &intent,
		AcceptanceCriteria: &acceptance,
		Scope:              &scope,
	}); err != nil {
		t.Fatalf("SetTaskContract() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "diff.txt"), []byte("diff\n"), 0o644); err != nil {
		t.Fatalf("write diff.txt: %v", err)
	}
	if _, err := svc.DoneTaskWithCheckpoint(cr.ID, task.ID, DoneTaskOptions{Checkpoint: true, FromContract: true}); err != nil {
		t.Fatalf("DoneTaskWithCheckpoint() error = %v", err)
	}

	bundle, _, err := svc.ExportCRBundle(cr.ID, ExportCROptions{Format: "json", Include: []string{"diffs"}})
	if err != nil {
		t.Fatalf("ExportCRBundle(include diffs) error = %v", err)
	}
	if len(bundle.TaskDiffs) != 1 {
		t.Fatalf("expected one task diff, got %#v", bundle.TaskDiffs)
	}
	if !containsString(bundle.TaskDiffs[0].Files, "diff.txt") {
		t.Fatalf("expected diff.txt in task diff files, got %#v", bundle.TaskDiffs[0])
	}
	if !strings.Contains(bundle.TaskDiffs[0].Patch, "diff --git") {
		t.Fatalf("expected patch body in task diff, got %#v", bundle.TaskDiffs[0])
	}
}

func TestExportCRBundleRejectsInvalidInclude(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")
	cr, err := svc.AddCR("Export invalid", "invalid include")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	setValidContract(t, svc, cr.ID)

	if _, _, err := svc.ExportCRBundle(cr.ID, ExportCROptions{Format: "json", Include: []string{"unknown"}}); err == nil {
		t.Fatalf("expected invalid include error")
	}
}

func TestExportCRBundleSupportsYAMLAndNDJSONDeterministic(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")

	cr, err := svc.AddCR("Export formats", "yaml and ndjson")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	setValidContract(t, svc, cr.ID)

	task, err := svc.AddTask(cr.ID, "feat: format export fixture")
	if err != nil {
		t.Fatalf("AddTask() error = %v", err)
	}
	intent := "Create checkpoint commit for multi-format export."
	acceptance := []string{"checkpoint exists"}
	scope := []string{"formats.txt"}
	if _, err := svc.SetTaskContract(cr.ID, task.ID, TaskContractPatch{
		Intent:             &intent,
		AcceptanceCriteria: &acceptance,
		Scope:              &scope,
	}); err != nil {
		t.Fatalf("SetTaskContract() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "formats.txt"), []byte("formats\n"), 0o644); err != nil {
		t.Fatalf("write formats.txt: %v", err)
	}
	if _, err := svc.DoneTaskWithCheckpoint(cr.ID, task.ID, DoneTaskOptions{Checkpoint: true, FromContract: true}); err != nil {
		t.Fatalf("DoneTaskWithCheckpoint() error = %v", err)
	}

	for _, format := range []string{"yaml", "ndjson"} {
		_, payloadA, err := svc.ExportCRBundle(cr.ID, ExportCROptions{Format: format, Include: []string{"diffs"}})
		if err != nil {
			t.Fatalf("ExportCRBundle(%s A) error = %v", format, err)
		}
		_, payloadB, err := svc.ExportCRBundle(cr.ID, ExportCROptions{Format: format, Include: []string{"diffs"}})
		if err != nil {
			t.Fatalf("ExportCRBundle(%s B) error = %v", format, err)
		}
		if !bytes.Equal(payloadA, payloadB) {
			t.Fatalf("expected deterministic %s payload bytes", format)
		}
		if len(payloadA) == 0 {
			t.Fatalf("expected non-empty %s payload", format)
		}
		if format == "yaml" {
			var decoded map[string]any
			if err := yaml.Unmarshal(payloadA, &decoded); err != nil {
				t.Fatalf("decode yaml export: %v", err)
			}
			if got, _ := decoded["schema_version"].(string); got != exportSchemaV1 {
				t.Fatalf("expected schema %q, got %#v", exportSchemaV1, decoded["schema_version"])
			}
		}
		if format == "ndjson" {
			lines := strings.Split(strings.TrimSpace(string(payloadA)), "\n")
			if len(lines) < 2 {
				t.Fatalf("expected multiple ndjson records, got %d", len(lines))
			}
			var first map[string]any
			if err := json.Unmarshal([]byte(lines[0]), &first); err != nil {
				t.Fatalf("decode ndjson meta line: %v", err)
			}
			if got, _ := first["type"].(string); got != "meta" {
				t.Fatalf("expected first ndjson record type meta, got %#v", first["type"])
			}
		}
	}
}

func TestExportCRBundleRichIncludesPopulateSections(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")

	cr, err := svc.AddCR("Export includes", "rich include sections")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	setValidContract(t, svc, cr.ID)
	if _, err := svc.AddEvidence(cr.ID, AddEvidenceOptions{
		Type:    "manual_note",
		Summary: "evidence note",
	}); err != nil {
		t.Fatalf("AddEvidence() error = %v", err)
	}

	task, err := svc.AddTask(cr.ID, "feat: include fixture")
	if err != nil {
		t.Fatalf("AddTask() error = %v", err)
	}
	intent := "Create checkpoint for include projection."
	acceptance := []string{"checkpoint exists"}
	scope := []string{"include.txt"}
	if _, err := svc.SetTaskContract(cr.ID, task.ID, TaskContractPatch{
		Intent:             &intent,
		AcceptanceCriteria: &acceptance,
		Scope:              &scope,
	}); err != nil {
		t.Fatalf("SetTaskContract() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "include.txt"), []byte("include\n"), 0o644); err != nil {
		t.Fatalf("write include.txt: %v", err)
	}
	if _, err := svc.DoneTaskWithCheckpoint(cr.ID, task.ID, DoneTaskOptions{Checkpoint: true, FromContract: true}); err != nil {
		t.Fatalf("DoneTaskWithCheckpoint() error = %v", err)
	}

	include := []string{"diffs", "evidence", "events", "anchors", "checkpoints", "trust", "validation"}
	bundle, _, err := svc.ExportCRBundle(cr.ID, ExportCROptions{Format: "json", Include: include})
	if err != nil {
		t.Fatalf("ExportCRBundle() error = %v", err)
	}
	if bundle.Sections == nil {
		t.Fatalf("expected sections projection for rich includes")
	}
	if len(bundle.Sections.TaskDiffs) == 0 {
		t.Fatalf("expected sections.task_diffs")
	}
	if len(bundle.Sections.Evidence) == 0 {
		t.Fatalf("expected sections.evidence")
	}
	if len(bundle.Sections.Events) == 0 {
		t.Fatalf("expected sections.events")
	}
	if bundle.Sections.Anchors == nil {
		t.Fatalf("expected sections.anchors")
	}
	if len(bundle.Sections.Checkpoints) == 0 {
		t.Fatalf("expected sections.checkpoints")
	}
	if bundle.Sections.Trust == nil {
		t.Fatalf("expected sections.trust")
	}
	if bundle.Sections.Validation == nil {
		t.Fatalf("expected sections.validation")
	}
}

func TestExportCRBundleRejectsInvalidFormat(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	cr, err := svc.AddCR("Export invalid format", "invalid format")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	setValidContract(t, svc, cr.ID)

	if _, _, err := svc.ExportCRBundle(cr.ID, ExportCROptions{Format: "toml"}); err == nil {
		t.Fatalf("expected invalid format error")
	}
}

func TestExportCRBundleYAMLCRUsesSnakeCaseKeys(t *testing.T) {
	t.Parallel()
	bundle := &CRExportBundle{
		SchemaVersion:    exportSchemaV1,
		Format:           exportFormatYAML,
		CRUID:            "cr_yaml_shape",
		CRFingerprint:    "fp",
		DocSchemaVersion: crDocSchemaV1,
		CR: &model.CR{
			ID:          1,
			UID:         "cr_yaml_shape",
			Title:       "shape",
			Description: "shape",
			Status:      model.StatusInProgress,
			BaseBranch:  "main",
			Branch:      "cr-shape",
		},
		CRYAML: strings.TrimSpace(`
id: 1
uid: cr_yaml_shape
title: shape
description: shape
status: in_progress
base_branch: main
branch: cr-shape
notes: []
subtasks: []
events: []
created_at: "2026-03-04T00:00:00Z"
updated_at: "2026-03-04T00:00:00Z"
`) + "\n",
	}
	payload, err := marshalExportBundleYAML(bundle)
	if err != nil {
		t.Fatalf("marshalExportBundleYAML() error = %v", err)
	}
	var decoded map[string]any
	if err := yaml.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("decode yaml export: %v", err)
	}
	crValue, ok := decoded["cr"]
	if !ok {
		t.Fatalf("expected yaml payload to include cr object")
	}
	crMap, ok := normalizeYAMLValue(crValue).(map[string]any)
	if !ok {
		t.Fatalf("expected cr to decode into map, got %#v", crValue)
	}
	if _, ok := crMap["base_branch"]; !ok {
		t.Fatalf("expected snake_case key base_branch in cr payload, got keys=%v", mapKeys(crMap))
	}
	if _, ok := crMap["BaseBranch"]; ok {
		t.Fatalf("did not expect CamelCase key BaseBranch in cr payload, got keys=%v", mapKeys(crMap))
	}
	if got, ok := crMap["id"].(int); !ok || got != 1 {
		t.Fatalf("expected cr.id integer value 1, got %#v", crMap["id"])
	}
}

func TestMarshalExportBundleYAMLRejectsInvalidCRYAML(t *testing.T) {
	t.Parallel()
	bundle := &CRExportBundle{
		SchemaVersion: exportSchemaV1,
		Format:        exportFormatYAML,
		CRUID:         "cr_bad_yaml",
		CRFingerprint: "fp",
		CRYAML:        "id: [not-valid\n",
	}
	if _, err := marshalExportBundleYAML(bundle); err == nil {
		t.Fatalf("expected marshalExportBundleYAML to fail for invalid cr_yaml")
	}
}

func TestDecodeExportBundleNDJSONAllowsLargeRecord(t *testing.T) {
	t.Parallel()
	largeCRYAML := strings.Repeat("x", 17*1024*1024)
	lines := [][]byte{
		mustMarshalNDJSONLine(t, map[string]any{
			"type":               "meta",
			"schema_version":     exportSchemaV1,
			"format":             exportFormatNDJSON,
			"cr_uid":             "cr_large_line",
			"cr_fingerprint":     "fp",
			"doc_schema_version": crDocSchemaV1,
		}),
		mustMarshalNDJSONLine(t, map[string]any{
			"type": "doc",
			"value": map[string]any{
				"id":          1,
				"uid":         "cr_large_line",
				"title":       "large ndjson",
				"description": "test",
				"status":      "in_progress",
				"base_branch": "main",
				"branch":      "cr-large",
				"notes":       []string{},
				"subtasks":    []any{},
				"events":      []any{},
				"created_at":  "2026-03-04T00:00:00Z",
				"updated_at":  "2026-03-04T00:00:00Z",
			},
		}),
		mustMarshalNDJSONLine(t, map[string]any{
			"type":  "cr_yaml",
			"value": largeCRYAML,
		}),
	}
	raw := bytes.Join(lines, []byte{'\n'})
	raw = append(raw, '\n')

	bundle, err := decodeExportBundleNDJSON(raw)
	if err != nil {
		t.Fatalf("decodeExportBundleNDJSON() error = %v", err)
	}
	if bundle == nil {
		t.Fatalf("expected non-nil bundle")
	}
	if got := len(bundle.CRYAML); got != len(largeCRYAML) {
		t.Fatalf("expected large cr_yaml length %d, got %d", len(largeCRYAML), got)
	}
}

func TestDecodeExportBundleNDJSONRejectsOversizedRecord(t *testing.T) {
	t.Parallel()
	oversizedCRYAML := strings.Repeat("x", exportNDJSONMaxLineBytes)
	lines := [][]byte{
		mustMarshalNDJSONLine(t, map[string]any{
			"type":               "meta",
			"schema_version":     exportSchemaV1,
			"format":             exportFormatNDJSON,
			"cr_uid":             "cr_oversized_line",
			"cr_fingerprint":     "fp",
			"doc_schema_version": crDocSchemaV1,
		}),
		mustMarshalNDJSONLine(t, map[string]any{
			"type":  "cr_yaml",
			"value": oversizedCRYAML,
		}),
	}
	raw := bytes.Join(lines, []byte{'\n'})
	raw = append(raw, '\n')

	if _, err := decodeExportBundleNDJSON(raw); err == nil {
		t.Fatalf("expected oversized ndjson line error")
	} else if !strings.Contains(err.Error(), "record exceeds max line bytes") {
		t.Fatalf("expected deterministic oversized record error, got %v", err)
	}
}

func mustMarshalNDJSONLine(t *testing.T, value map[string]any) []byte {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal ndjson line: %v", err)
	}
	return raw
}

func mapKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	return keys
}

func TestNDJSONRecordDecoderRegistryCoversMarshallerTypes(t *testing.T) {
	t.Parallel()
	bundle := &CRExportBundle{
		SchemaVersion:    exportSchemaV1,
		Format:           exportFormatNDJSON,
		CRUID:            "cr_registry_decoder",
		CRFingerprint:    "fp",
		DocSchemaVersion: crDocSchemaV1,
		Doc: &CRDoc{
			ID:          1,
			UID:         "cr_registry_decoder",
			Title:       "registry decoder",
			Description: "decoder coverage",
			Status:      model.StatusInProgress,
			BaseBranch:  "main",
			Branch:      "cr-registry-decoder",
		},
		CR: &model.CR{
			ID:          1,
			UID:         "cr_registry_decoder",
			Title:       "registry decoder",
			Description: "decoder coverage",
			Status:      model.StatusInProgress,
			BaseBranch:  "main",
			Branch:      "cr-registry-decoder",
		},
		CRYAML: "id: 1\n",
		Derived: CRExportDerived{
			FilesChanged: []string{"internal/service/service_export.go"},
		},
		Anchors:           &CRExportAnchors{BaseRef: "main"},
		Checkpoints:       []CRExportCheckpoint{{TaskID: 1, Title: "task"}},
		ReferencedCommits: []string{"abc123"},
		Evidence:          []model.EvidenceEntry{{Type: "command_run", Summary: "ok"}},
		TaskDiffs:         []CRExportTaskDiff{{TaskID: 1, Title: "task", Commit: "abc123"}},
		Sections:          &CRExportSections{TaskDiffs: []CRExportTaskDiff{{TaskID: 1, Title: "task", Commit: "abc123"}}},
	}
	raw, err := marshalExportBundleNDJSON(bundle)
	if err != nil {
		t.Fatalf("marshalExportBundleNDJSON() error = %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	if len(lines) == 0 {
		t.Fatalf("expected ndjson records")
	}
	seenTypes := make(map[string]struct{}, len(lines))
	for _, line := range lines {
		var payload map[string]json.RawMessage
		if err := json.Unmarshal([]byte(line), &payload); err != nil {
			t.Fatalf("decode ndjson line: %v", err)
		}
		recordType, err := ndjsonRecordType(payload)
		if err != nil {
			t.Fatalf("ndjsonRecordType() error: %v", err)
		}
		decoder, ok := ndjsonRecordDecoders[recordType]
		if !ok {
			t.Fatalf("missing ndjson decoder for record type %q", recordType)
		}
		if decoder.decode == nil {
			t.Fatalf("nil ndjson decoder for record type %q", recordType)
		}
		seenTypes[recordType] = struct{}{}
	}
	if len(seenTypes) != len(ndjsonRecordDecoders) {
		t.Fatalf("decoder registry size mismatch: seen=%d registered=%d", len(seenTypes), len(ndjsonRecordDecoders))
	}
	meta := ndjsonRecordDecoders["meta"]
	if !meta.marksMeta {
		t.Fatalf("meta decoder must mark hasMeta")
	}
}
