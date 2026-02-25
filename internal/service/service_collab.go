package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"
	servicecollab "sophia/internal/service/collab"
	"sort"
	"strings"

	"sophia/internal/model"
	"sophia/internal/store"
)

const (
	patchSchemaV1     = model.CRPatchSchemaV1
	importModeCreate  = "create"
	importModeReplace = "replace"
)

type ImportCRBundleOptions struct {
	FilePath string
	Mode     string
}

type ImportCRBundleResult struct {
	LocalCRID     int    `json:"local_cr_id"`
	CRUID         string `json:"cr_uid"`
	CRFingerprint string `json:"cr_fingerprint"`
	Created       bool   `json:"created"`
	Replaced      bool   `json:"replaced"`
}

type CRPatch struct {
	SchemaVersion string            `json:"schema_version"`
	Target        CRPatchTarget     `json:"target"`
	Base          CRPatchBase       `json:"base"`
	Ops           []json.RawMessage `json:"ops"`
	Meta          CRPatchMeta       `json:"meta,omitempty"`
}

type CRPatchTarget struct {
	CRUID string `json:"cr_uid"`
}

type CRPatchBase struct {
	CRFingerprint string `json:"cr_fingerprint"`
	ExportedAt    string `json:"exported_at,omitempty"`
}

type CRPatchMeta struct {
	Author  string `json:"author,omitempty"`
	Tool    string `json:"tool,omitempty"`
	Message string `json:"message,omitempty"`
}

type CRPatchApplyResult struct {
	CRID            int               `json:"cr_id"`
	CRUID           string            `json:"cr_uid"`
	BaseFingerprint string            `json:"base_fingerprint"`
	NewFingerprint  string            `json:"new_fingerprint"`
	AppliedOps      []int             `json:"applied_ops"`
	SkippedOps      []int             `json:"skipped_ops"`
	Conflicts       []CRPatchConflict `json:"conflicts"`
	Warnings        []string          `json:"warnings"`
	Preview         bool              `json:"preview"`
}

type CRPatchConflict struct {
	OpIndex  int    `json:"op_index"`
	Op       string `json:"op"`
	Field    string `json:"field"`
	Message  string `json:"message"`
	Expected any    `json:"expected,omitempty"`
	Current  any    `json:"current,omitempty"`
}

type PatchConflictError struct {
	Result *CRPatchApplyResult
}

func (e *PatchConflictError) Error() string {
	if e == nil || e.Result == nil {
		return "patch apply conflicts detected"
	}
	return fmt.Sprintf("patch apply conflicts detected (%d conflict(s))", len(e.Result.Conflicts))
}

func (e *PatchConflictError) Details() map[string]any {
	if e == nil || e.Result == nil {
		return map[string]any{}
	}
	return map[string]any{
		"cr_id":            e.Result.CRID,
		"cr_uid":           e.Result.CRUID,
		"base_fingerprint": e.Result.BaseFingerprint,
		"new_fingerprint":  e.Result.NewFingerprint,
		"applied_ops":      append([]int(nil), e.Result.AppliedOps...),
		"skipped_ops":      append([]int(nil), e.Result.SkippedOps...),
		"warnings":         append([]string(nil), e.Result.Warnings...),
		"conflicts":        append([]CRPatchConflict(nil), e.Result.Conflicts...),
		"preview":          e.Result.Preview,
	}
}

type rawPatchOp struct {
	Op string `json:"op"`
}

type patchSetFieldOp struct {
	Op     string           `json:"op"`
	Field  string           `json:"field"`
	Before *json.RawMessage `json:"before,omitempty"`
	After  json.RawMessage  `json:"after"`
}

type patchAddNoteOp struct {
	Op   string `json:"op"`
	Text string `json:"text"`
}

type patchAddTaskOp struct {
	Op            string `json:"op"`
	Title         string `json:"title"`
	ClientTaskKey string `json:"client_task_key,omitempty"`
}

type patchValueChange struct {
	Before *json.RawMessage `json:"before,omitempty"`
	After  json.RawMessage  `json:"after"`
}

type patchTaskContractChanges struct {
	Intent             *patchValueChange `json:"intent,omitempty"`
	AcceptanceCriteria *patchValueChange `json:"acceptance_criteria,omitempty"`
	Scope              *patchValueChange `json:"scope,omitempty"`
	AcceptanceChecks   *patchValueChange `json:"acceptance_checks,omitempty"`
}

type patchUpdateTaskChanges struct {
	Title    *patchValueChange         `json:"title,omitempty"`
	Status   *patchValueChange         `json:"status,omitempty"`
	Contract *patchTaskContractChanges `json:"contract,omitempty"`
}

type patchUpdateTaskOp struct {
	Op      string                 `json:"op"`
	TaskID  int                    `json:"task_id"`
	Changes patchUpdateTaskChanges `json:"changes"`
}

type patchSetContractChanges struct {
	Why                *patchValueChange `json:"why,omitempty"`
	Scope              *patchValueChange `json:"scope,omitempty"`
	NonGoals           *patchValueChange `json:"non_goals,omitempty"`
	Invariants         *patchValueChange `json:"invariants,omitempty"`
	BlastRadius        *patchValueChange `json:"blast_radius,omitempty"`
	RiskCriticalScopes *patchValueChange `json:"risk_critical_scopes,omitempty"`
	RiskTierHint       *patchValueChange `json:"risk_tier_hint,omitempty"`
	RiskRationale      *patchValueChange `json:"risk_rationale,omitempty"`
	TestPlan           *patchValueChange `json:"test_plan,omitempty"`
	RollbackPlan       *patchValueChange `json:"rollback_plan,omitempty"`
}

type patchSetContractOp struct {
	Op      string                  `json:"op"`
	Changes patchSetContractChanges `json:"changes"`
}

func (s *Service) ImportCRBundle(opts ImportCRBundleOptions) (*ImportCRBundleResult, error) {
	var result *ImportCRBundleResult
	if err := s.withMutationLock(func() error {
		var err error
		result, err = s.importCRBundleUnlocked(opts)
		return err
	}); err != nil {
		return nil, err
	}
	return result, nil
}

func (s *Service) importCRBundleUnlocked(opts ImportCRBundleOptions) (*ImportCRBundleResult, error) {
	if err := s.store.EnsureInitialized(); err != nil {
		return nil, err
	}
	path := strings.TrimSpace(opts.FilePath)
	if path == "" {
		return nil, fmt.Errorf("--file is required")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read import bundle %q: %w", path, err)
	}
	var bundle CRExportBundle
	if err := json.Unmarshal(raw, &bundle); err != nil {
		return nil, fmt.Errorf("decode import bundle: %w", err)
	}
	if strings.TrimSpace(bundle.SchemaVersion) != exportSchemaV1 {
		return nil, fmt.Errorf("invalid bundle schema_version %q (expected %s)", strings.TrimSpace(bundle.SchemaVersion), exportSchemaV1)
	}

	imported, err := crFromImportBundle(&bundle)
	if err != nil {
		return nil, err
	}
	uid := strings.TrimSpace(imported.UID)
	if uid == "" {
		return nil, fmt.Errorf("bundle is missing cr uid")
	}

	mode, modeErr := normalizeImportMode(opts.Mode)
	if modeErr != nil {
		return nil, modeErr
	}

	existing, existingErr := s.store.LoadCRByUID(uid)
	created := false
	replaced := false
	switch {
	case existingErr == nil:
		if mode == importModeCreate {
			return nil, fmt.Errorf("cr uid %q already exists locally (id %d)", uid, existing.ID)
		}
		imported.ID = existing.ID
		created = false
		replaced = true
	case existingErr != nil:
		if !errors.Is(existingErr, store.ErrNotFound) {
			return nil, existingErr
		}
		newID, nextErr := s.store.NextCRID()
		if nextErr != nil {
			return nil, nextErr
		}
		imported.ID = newID
		created = true
		replaced = false
	}

	if err := s.store.SaveCR(imported); err != nil {
		return nil, err
	}
	if err := s.syncCRRef(imported); err != nil {
		return nil, err
	}
	doc := canonicalCRDoc(imported)
	fingerprint, fpErr := fingerprintCRDoc(doc)
	if fpErr != nil {
		return nil, fpErr
	}
	return &ImportCRBundleResult{
		LocalCRID:     imported.ID,
		CRUID:         uid,
		CRFingerprint: fingerprint,
		Created:       created,
		Replaced:      replaced,
	}, nil
}

func crFromImportBundle(bundle *CRExportBundle) (*model.CR, error) {
	if bundle == nil {
		return nil, fmt.Errorf("bundle is required")
	}
	if bundle.Doc != nil {
		if strings.TrimSpace(bundle.DocSchemaVersion) != crDocSchemaV1 {
			return nil, fmt.Errorf("invalid doc schema_version %q (expected %s)", strings.TrimSpace(bundle.DocSchemaVersion), crDocSchemaV1)
		}
		return crFromDoc(bundle.Doc), nil
	}
	if bundle.CR != nil {
		copyCR := *bundle.CR
		return &copyCR, nil
	}
	return nil, fmt.Errorf("bundle has no importable cr document")
}

func crFromDoc(doc *CRDoc) *model.CR {
	if doc == nil {
		return &model.CR{}
	}
	out := &model.CR{
		ID:                doc.ID,
		UID:               strings.TrimSpace(doc.UID),
		Title:             doc.Title,
		Description:       doc.Description,
		Status:            doc.Status,
		BaseBranch:        doc.BaseBranch,
		BaseRef:           strings.TrimSpace(doc.BaseRef),
		BaseCommit:        strings.TrimSpace(doc.BaseCommit),
		ParentCRID:        doc.ParentCRID,
		Branch:            doc.Branch,
		Notes:             append([]string(nil), doc.Notes...),
		Evidence:          append([]model.EvidenceEntry(nil), doc.Evidence...),
		Contract:          cloneContract(doc.Contract),
		Subtasks:          cloneSubtasks(doc.Subtasks),
		Events:            make([]model.Event, 0, len(doc.Events)),
		MergedAt:          strings.TrimSpace(doc.MergedAt),
		MergedBy:          strings.TrimSpace(doc.MergedBy),
		MergedCommit:      strings.TrimSpace(doc.MergedCommit),
		FilesTouchedCount: doc.FilesTouchedCount,
		CreatedAt:         doc.CreatedAt,
		UpdatedAt:         doc.UpdatedAt,
	}
	for _, event := range doc.Events {
		out.Events = append(out.Events, model.Event{
			TS:              event.TS,
			Actor:           event.Actor,
			Type:            event.Type,
			Summary:         event.Summary,
			Ref:             strings.TrimSpace(event.Ref),
			Redacted:        event.Redacted,
			RedactionReason: strings.TrimSpace(event.RedactionReason),
			Meta:            metaMapFromEntries(event.Meta),
		})
	}
	if out.Notes == nil {
		out.Notes = []string{}
	}
	if out.Evidence == nil {
		out.Evidence = []model.EvidenceEntry{}
	}
	if out.Subtasks == nil {
		out.Subtasks = []model.Subtask{}
	}
	if out.Events == nil {
		out.Events = []model.Event{}
	}
	return out
}

func metaMapFromEntries(entries []CRDocMetaEntry) map[string]string {
	if len(entries) == 0 {
		return map[string]string{}
	}
	out := map[string]string{}
	for _, entry := range entries {
		key := strings.TrimSpace(entry.Key)
		if key == "" {
			continue
		}
		out[key] = entry.Value
	}
	return out
}

func (s *Service) ApplyCRPatch(selector string, patchBytes []byte, force bool, preview bool) (*CRPatchApplyResult, error) {
	var result *CRPatchApplyResult
	if err := s.withMutationLock(func() error {
		var err error
		result, err = s.applyCRPatchUnlocked(selector, patchBytes, force, preview)
		return err
	}); err != nil {
		return nil, err
	}
	return result, nil
}

func (s *Service) applyCRPatchUnlocked(selector string, patchBytes []byte, force bool, preview bool) (*CRPatchApplyResult, error) {
	patch, err := parseCRPatch(patchBytes)
	if err != nil {
		return nil, err
	}
	cr, err := s.resolvePatchTargetCR(selector, patch.Target)
	if err != nil {
		return nil, err
	}
	baseDoc := canonicalCRDoc(cr)
	baseFingerprint, fpErr := fingerprintCRDoc(baseDoc)
	if fpErr != nil {
		return nil, fpErr
	}
	out := &CRPatchApplyResult{
		CRID:            cr.ID,
		CRUID:           strings.TrimSpace(cr.UID),
		BaseFingerprint: baseFingerprint,
		AppliedOps:      []int{},
		SkippedOps:      []int{},
		Conflicts:       []CRPatchConflict{},
		Warnings:        []string{},
		Preview:         preview,
	}
	if expected := strings.TrimSpace(patch.Base.CRFingerprint); expected != "" && expected != baseFingerprint {
		out.Warnings = append(out.Warnings, fmt.Sprintf("patch base fingerprint mismatch: patch=%s current=%s", expected, baseFingerprint))
	}

	working := *cr
	working.Contract = cloneContract(cr.Contract)
	working.Subtasks = cloneSubtasks(cr.Subtasks)
	working.Notes = append([]string(nil), cr.Notes...)
	working.Evidence = append([]model.EvidenceEntry(nil), cr.Evidence...)
	working.Events = append([]model.Event(nil), cr.Events...)

	noteHashes := map[string]struct{}{}
	for _, note := range working.Notes {
		noteHashes[noteHash(note)] = struct{}{}
	}

	for idx, raw := range patch.Ops {
		applied, skipped, conflicts, warnings, opErr := s.applySinglePatchOp(&working, raw, idx+1, noteHashes, force)
		out.Conflicts = append(out.Conflicts, conflicts...)
		out.Warnings = append(out.Warnings, warnings...)
		if opErr != nil {
			return nil, opErr
		}
		if applied {
			out.AppliedOps = append(out.AppliedOps, idx+1)
		}
		if skipped {
			out.SkippedOps = append(out.SkippedOps, idx+1)
		}
	}

	newDoc := canonicalCRDoc(&working)
	newFingerprint, newErr := fingerprintCRDoc(newDoc)
	if newErr != nil {
		return nil, newErr
	}
	out.NewFingerprint = newFingerprint

	if len(out.Conflicts) > 0 {
		return out, &PatchConflictError{Result: out}
	}
	if preview || len(out.AppliedOps) == 0 {
		return out, nil
	}

	now := s.timestamp()
	actor := s.git.Actor()
	working.UpdatedAt = now
	working.Events = append(working.Events, model.Event{
		TS:      now,
		Actor:   actor,
		Type:    model.EventTypePatchApplied,
		Summary: fmt.Sprintf("Applied collaboration patch (%d op(s))", len(out.AppliedOps)),
		Ref:     fmt.Sprintf("cr:%d", working.ID),
		Meta: map[string]string{
			"schema_version": patch.SchemaVersion,
			"applied_ops":    fmt.Sprintf("%d", len(out.AppliedOps)),
		},
	})
	if err := s.store.SaveCR(&working); err != nil {
		return nil, err
	}
	return out, nil
}

func parseCRPatch(payload []byte) (*CRPatch, error) {
	var patch CRPatch
	if err := json.Unmarshal(payload, &patch); err != nil {
		return nil, fmt.Errorf("decode patch: %w", err)
	}
	if strings.TrimSpace(patch.SchemaVersion) != patchSchemaV1 {
		return nil, fmt.Errorf("invalid patch schema_version %q (expected %s)", strings.TrimSpace(patch.SchemaVersion), patchSchemaV1)
	}
	if len(patch.Ops) == 0 {
		return nil, fmt.Errorf("patch must include at least one op")
	}
	return &patch, nil
}

func (s *Service) resolvePatchTargetCR(selector string, target CRPatchTarget) (*model.CR, error) {
	uid := strings.TrimSpace(target.CRUID)
	if strings.TrimSpace(selector) == "" {
		if uid == "" {
			return nil, fmt.Errorf("patch requires selector or target.cr_uid")
		}
		return s.store.LoadCRByUID(uid)
	}
	id, err := s.ResolveCRID(selector)
	if err != nil {
		return nil, err
	}
	cr, err := s.store.LoadCR(id)
	if err != nil {
		return nil, err
	}
	if uid != "" && strings.TrimSpace(cr.UID) != uid {
		return nil, fmt.Errorf("patch target cr_uid %q does not match selected CR uid %q", uid, strings.TrimSpace(cr.UID))
	}
	return cr, nil
}

func noteHash(note string) string {
	return servicecollab.NoteHash(note)
}

func addPatchConflict(out *[]CRPatchConflict, opIndex int, op, field, message string, expected, current any) {
	*out = append(*out, CRPatchConflict{
		OpIndex:  opIndex,
		Op:       op,
		Field:    field,
		Message:  message,
		Expected: expected,
		Current:  current,
	})
}

func (s *Service) applySinglePatchOp(cr *model.CR, raw json.RawMessage, opIndex int, noteHashes map[string]struct{}, force bool) (bool, bool, []CRPatchConflict, []string, error) {
	var header rawPatchOp
	if err := json.Unmarshal(raw, &header); err != nil {
		return false, false, nil, nil, fmt.Errorf("decode patch op #%d: %w", opIndex, err)
	}
	opName := strings.TrimSpace(header.Op)
	if opName == "" {
		return false, false, nil, nil, fmt.Errorf("patch op #%d missing op", opIndex)
	}
	switch opName {
	case "set_field":
		return s.applyPatchSetField(cr, raw, opIndex, force)
	case "add_note":
		return s.applyPatchAddNote(cr, raw, opIndex, noteHashes)
	case "add_task":
		return s.applyPatchAddTask(cr, raw, opIndex)
	case "update_task":
		return s.applyPatchUpdateTask(cr, raw, opIndex, force)
	case "set_contract":
		return s.applyPatchSetContract(cr, raw, opIndex, force)
	default:
		return false, false, nil, nil, fmt.Errorf("unsupported patch op %q at index %d", opName, opIndex)
	}
}

func (s *Service) applyPatchSetField(cr *model.CR, raw json.RawMessage, opIndex int, force bool) (bool, bool, []CRPatchConflict, []string, error) {
	var op patchSetFieldOp
	if err := json.Unmarshal(raw, &op); err != nil {
		return false, false, nil, nil, fmt.Errorf("decode set_field op #%d: %w", opIndex, err)
	}
	field := strings.TrimSpace(op.Field)
	if field == "" {
		return false, false, nil, nil, fmt.Errorf("set_field op #%d missing field", opIndex)
	}
	current, currentErr := readCRField(cr, field)
	if currentErr != nil {
		return false, false, nil, nil, currentErr
	}
	beforeValue, hasBefore, beforeErr := decodeCRFieldValue(field, op.Before)
	if beforeErr != nil {
		return false, false, nil, nil, fmt.Errorf("set_field op #%d before decode: %w", opIndex, beforeErr)
	}
	afterValue, _, afterErr := decodeCRFieldValue(field, &op.After)
	if afterErr != nil {
		return false, false, nil, nil, fmt.Errorf("set_field op #%d after decode: %w", opIndex, afterErr)
	}

	conflicts := []CRPatchConflict{}
	warnings := []string{}
	if !hasBefore && !force {
		addPatchConflict(&conflicts, opIndex, op.Op, field, "before is required unless --force is used", nil, current)
		return false, false, conflicts, warnings, nil
	}
	if hasBefore && !reflect.DeepEqual(current, beforeValue) {
		if !force {
			addPatchConflict(&conflicts, opIndex, op.Op, field, "before does not match current value", beforeValue, current)
			return false, false, conflicts, warnings, nil
		}
		warnings = append(warnings, fmt.Sprintf("op #%d %s: force applied despite before mismatch", opIndex, field))
	}
	if reflect.DeepEqual(current, afterValue) {
		return false, true, conflicts, warnings, nil
	}
	if err := writeCRField(cr, field, afterValue); err != nil {
		return false, false, nil, nil, err
	}
	return true, false, conflicts, warnings, nil
}

func (s *Service) applyPatchAddNote(cr *model.CR, raw json.RawMessage, opIndex int, noteHashes map[string]struct{}) (bool, bool, []CRPatchConflict, []string, error) {
	var op patchAddNoteOp
	if err := json.Unmarshal(raw, &op); err != nil {
		return false, false, nil, nil, fmt.Errorf("decode add_note op #%d: %w", opIndex, err)
	}
	note := strings.TrimSpace(op.Text)
	if note == "" {
		return false, false, nil, nil, fmt.Errorf("add_note op #%d missing text", opIndex)
	}
	hash := noteHash(note)
	if _, ok := noteHashes[hash]; ok {
		return false, true, nil, nil, nil
	}
	noteHashes[hash] = struct{}{}
	cr.Notes = append(cr.Notes, note)
	return true, false, nil, nil, nil
}

func (s *Service) applyPatchAddTask(cr *model.CR, raw json.RawMessage, opIndex int) (bool, bool, []CRPatchConflict, []string, error) {
	var op patchAddTaskOp
	if err := json.Unmarshal(raw, &op); err != nil {
		return false, false, nil, nil, fmt.Errorf("decode add_task op #%d: %w", opIndex, err)
	}
	title := strings.TrimSpace(op.Title)
	if title == "" {
		return false, false, nil, nil, fmt.Errorf("add_task op #%d missing title", opIndex)
	}
	now := s.timestamp()
	actor := s.git.Actor()
	task := model.Subtask{
		ID:        nextTaskID(cr.Subtasks),
		Title:     title,
		Status:    model.TaskStatusOpen,
		CreatedAt: now,
		UpdatedAt: now,
		CreatedBy: actor,
	}
	cr.Subtasks = append(cr.Subtasks, task)
	return true, false, nil, nil, nil
}

func (s *Service) applyPatchUpdateTask(cr *model.CR, raw json.RawMessage, opIndex int, force bool) (bool, bool, []CRPatchConflict, []string, error) {
	var op patchUpdateTaskOp
	if err := json.Unmarshal(raw, &op); err != nil {
		return false, false, nil, nil, fmt.Errorf("decode update_task op #%d: %w", opIndex, err)
	}
	if op.TaskID <= 0 {
		return false, false, nil, nil, fmt.Errorf("update_task op #%d missing task_id", opIndex)
	}
	taskIdx := -1
	for i := range cr.Subtasks {
		if cr.Subtasks[i].ID == op.TaskID {
			taskIdx = i
			break
		}
	}
	if taskIdx < 0 {
		conflicts := []CRPatchConflict{}
		addPatchConflict(&conflicts, opIndex, op.Op, "task_id", "task does not exist", op.TaskID, nil)
		return false, false, conflicts, nil, nil
	}
	task := &cr.Subtasks[taskIdx]
	applied := false
	skipped := true
	conflicts := []CRPatchConflict{}
	warnings := []string{}

	applyString := func(field string, change *patchValueChange, read func() string, write func(string) error) error {
		if change == nil {
			return nil
		}
		skipped = false
		before, hasBefore, err := decodeStringChange(change.Before)
		if err != nil {
			return err
		}
		after, err := decodeStringRaw(change.After)
		if err != nil {
			return err
		}
		current := read()
		if !hasBefore && !force {
			addPatchConflict(&conflicts, opIndex, op.Op, field, "before is required unless --force is used", nil, current)
			return nil
		}
		if hasBefore && current != before {
			if !force {
				addPatchConflict(&conflicts, opIndex, op.Op, field, "before does not match current value", before, current)
				return nil
			}
			warnings = append(warnings, fmt.Sprintf("op #%d %s: force applied despite before mismatch", opIndex, field))
		}
		if current == after {
			return nil
		}
		if err := write(after); err != nil {
			return err
		}
		applied = true
		return nil
	}

	if err := applyString("task.title", op.Changes.Title, func() string { return task.Title }, func(v string) error {
		task.Title = v
		return nil
	}); err != nil {
		return false, false, nil, nil, err
	}
	if err := applyString("task.status", op.Changes.Status, func() string { return task.Status }, func(v string) error {
		switch v {
		case model.TaskStatusOpen, model.TaskStatusDone, model.TaskStatusDelegated:
			task.Status = v
			return nil
		default:
			return fmt.Errorf("update_task op #%d invalid status %q", opIndex, v)
		}
	}); err != nil {
		return false, false, nil, nil, err
	}

	if op.Changes.Contract != nil {
		skipped = false
		if err := applyString("task.contract.intent", op.Changes.Contract.Intent, func() string { return task.Contract.Intent }, func(v string) error {
			task.Contract.Intent = v
			return nil
		}); err != nil {
			return false, false, nil, nil, err
		}
		if err := applyStringSliceChange(opIndex, op.Op, "task.contract.acceptance_criteria", op.Changes.Contract.AcceptanceCriteria, &task.Contract.AcceptanceCriteria, force, &conflicts, &warnings, &applied); err != nil {
			return false, false, nil, nil, err
		}
		if err := applyStringSliceChange(opIndex, op.Op, "task.contract.scope", op.Changes.Contract.Scope, &task.Contract.Scope, force, &conflicts, &warnings, &applied); err != nil {
			return false, false, nil, nil, err
		}
		if err := applyStringSliceChange(opIndex, op.Op, "task.contract.acceptance_checks", op.Changes.Contract.AcceptanceChecks, &task.Contract.AcceptanceChecks, force, &conflicts, &warnings, &applied); err != nil {
			return false, false, nil, nil, err
		}
	}
	if applied {
		task.UpdatedAt = s.timestamp()
	}
	if skipped {
		return false, true, conflicts, warnings, nil
	}
	return applied, false, conflicts, warnings, nil
}

func (s *Service) applyPatchSetContract(cr *model.CR, raw json.RawMessage, opIndex int, force bool) (bool, bool, []CRPatchConflict, []string, error) {
	var op patchSetContractOp
	if err := json.Unmarshal(raw, &op); err != nil {
		return false, false, nil, nil, fmt.Errorf("decode set_contract op #%d: %w", opIndex, err)
	}
	applied := false
	skipped := true
	conflicts := []CRPatchConflict{}
	warnings := []string{}
	contract := &cr.Contract

	applyString := func(field string, change *patchValueChange, read func() string, write func(string) error) error {
		if change == nil {
			return nil
		}
		skipped = false
		before, hasBefore, err := decodeStringChange(change.Before)
		if err != nil {
			return err
		}
		after, err := decodeStringRaw(change.After)
		if err != nil {
			return err
		}
		current := read()
		if !hasBefore && !force {
			addPatchConflict(&conflicts, opIndex, op.Op, field, "before is required unless --force is used", nil, current)
			return nil
		}
		if hasBefore && current != before {
			if !force {
				addPatchConflict(&conflicts, opIndex, op.Op, field, "before does not match current value", before, current)
				return nil
			}
			warnings = append(warnings, fmt.Sprintf("op #%d %s: force applied despite before mismatch", opIndex, field))
		}
		if current == after {
			return nil
		}
		if err := write(after); err != nil {
			return err
		}
		applied = true
		return nil
	}

	if err := applyString("contract.why", op.Changes.Why, func() string { return contract.Why }, func(v string) error {
		contract.Why = v
		return nil
	}); err != nil {
		return false, false, nil, nil, err
	}
	if err := applyString("contract.blast_radius", op.Changes.BlastRadius, func() string { return contract.BlastRadius }, func(v string) error {
		contract.BlastRadius = v
		return nil
	}); err != nil {
		return false, false, nil, nil, err
	}
	if err := applyString("contract.risk_tier_hint", op.Changes.RiskTierHint, func() string { return contract.RiskTierHint }, func(v string) error {
		normalized, normErr := normalizeRiskTierHint(v)
		if normErr != nil {
			return normErr
		}
		contract.RiskTierHint = normalized
		return nil
	}); err != nil {
		return false, false, nil, nil, err
	}
	if err := applyString("contract.risk_rationale", op.Changes.RiskRationale, func() string { return contract.RiskRationale }, func(v string) error {
		contract.RiskRationale = v
		return nil
	}); err != nil {
		return false, false, nil, nil, err
	}
	if err := applyString("contract.test_plan", op.Changes.TestPlan, func() string { return contract.TestPlan }, func(v string) error {
		contract.TestPlan = v
		return nil
	}); err != nil {
		return false, false, nil, nil, err
	}
	if err := applyString("contract.rollback_plan", op.Changes.RollbackPlan, func() string { return contract.RollbackPlan }, func(v string) error {
		contract.RollbackPlan = v
		return nil
	}); err != nil {
		return false, false, nil, nil, err
	}
	if err := applyStringSliceChange(opIndex, op.Op, "contract.scope", op.Changes.Scope, &contract.Scope, force, &conflicts, &warnings, &applied); err != nil {
		return false, false, nil, nil, err
	}
	if err := applyStringSliceChange(opIndex, op.Op, "contract.non_goals", op.Changes.NonGoals, &contract.NonGoals, force, &conflicts, &warnings, &applied); err != nil {
		return false, false, nil, nil, err
	}
	if err := applyStringSliceChange(opIndex, op.Op, "contract.invariants", op.Changes.Invariants, &contract.Invariants, force, &conflicts, &warnings, &applied); err != nil {
		return false, false, nil, nil, err
	}
	if err := applyStringSliceChange(opIndex, op.Op, "contract.risk_critical_scopes", op.Changes.RiskCriticalScopes, &contract.RiskCriticalScopes, force, &conflicts, &warnings, &applied); err != nil {
		return false, false, nil, nil, err
	}

	if skipped {
		return false, true, conflicts, warnings, nil
	}
	if applied {
		contract.UpdatedAt = s.timestamp()
		contract.UpdatedBy = s.git.Actor()
	}
	return applied, false, conflicts, warnings, nil
}

func applyStringSliceChange(opIndex int, op, field string, change *patchValueChange, target *[]string, force bool, conflicts *[]CRPatchConflict, warnings *[]string, applied *bool) error {
	if change == nil {
		return nil
	}
	before, hasBefore, err := decodeStringSliceChange(change.Before)
	if err != nil {
		return err
	}
	after, err := decodeStringSliceRaw(change.After)
	if err != nil {
		return err
	}
	current := append([]string(nil), *target...)
	sort.Strings(current)
	sortedBefore := append([]string(nil), before...)
	sort.Strings(sortedBefore)
	sortedAfter := append([]string(nil), after...)
	sort.Strings(sortedAfter)
	if !hasBefore && !force {
		addPatchConflict(conflicts, opIndex, op, field, "before is required unless --force is used", nil, current)
		return nil
	}
	if hasBefore && !reflect.DeepEqual(current, sortedBefore) {
		if !force {
			addPatchConflict(conflicts, opIndex, op, field, "before does not match current value", sortedBefore, current)
			return nil
		}
		*warnings = append(*warnings, fmt.Sprintf("op #%d %s: force applied despite before mismatch", opIndex, field))
	}
	if reflect.DeepEqual(current, sortedAfter) {
		return nil
	}
	*target = append([]string(nil), sortedAfter...)
	*applied = true
	return nil
}

func decodeStringChange(raw *json.RawMessage) (string, bool, error) {
	return servicecollab.DecodeStringChange(raw)
}

func decodeStringRaw(raw json.RawMessage) (string, error) {
	return servicecollab.DecodeStringRaw(raw)
}

func decodeStringSliceChange(raw *json.RawMessage) ([]string, bool, error) {
	return servicecollab.DecodeStringSliceChange(raw)
}

func decodeStringSliceRaw(raw json.RawMessage) ([]string, error) {
	return servicecollab.DecodeStringSliceRaw(raw)
}

func readCRField(cr *model.CR, field string) (any, error) {
	return servicecollab.ReadCRField(cr, field)
}

func decodeCRFieldValue(field string, raw *json.RawMessage) (any, bool, error) {
	return servicecollab.DecodeCRFieldValue(field, raw)
}

func writeCRField(cr *model.CR, field string, value any) error {
	return servicecollab.WriteCRField(cr, field, value)
}
