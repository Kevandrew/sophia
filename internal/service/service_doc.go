package service

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strings"

	"sophia/internal/model"
)

const crDocSchemaV1 = model.CRDocSchemaV1

func canonicalCRDoc(cr *model.CR) *CRDoc {
	if cr == nil {
		return nil
	}
	doc := &CRDoc{
		ID:                cr.ID,
		UID:               strings.TrimSpace(cr.UID),
		Title:             cr.Title,
		Description:       cr.Description,
		Status:            cr.Status,
		BaseBranch:        cr.BaseBranch,
		BaseRef:           strings.TrimSpace(cr.BaseRef),
		BaseCommit:        strings.TrimSpace(cr.BaseCommit),
		ParentCRID:        cr.ParentCRID,
		Branch:            cr.Branch,
		Notes:             append([]string(nil), cr.Notes...),
		Evidence:          append([]model.EvidenceEntry(nil), cr.Evidence...),
		Contract:          cloneContract(cr.Contract),
		Subtasks:          cloneSubtasks(cr.Subtasks),
		Events:            make([]CRDocEvent, 0, len(cr.Events)),
		MergedAt:          strings.TrimSpace(cr.MergedAt),
		MergedBy:          strings.TrimSpace(cr.MergedBy),
		MergedCommit:      strings.TrimSpace(cr.MergedCommit),
		FilesTouchedCount: cr.FilesTouchedCount,
		CreatedAt:         cr.CreatedAt,
		UpdatedAt:         cr.UpdatedAt,
	}
	if doc.Notes == nil {
		doc.Notes = []string{}
	}
	if doc.Evidence == nil {
		doc.Evidence = []model.EvidenceEntry{}
	}
	if doc.Subtasks == nil {
		doc.Subtasks = []model.Subtask{}
	}
	for _, event := range cr.Events {
		doc.Events = append(doc.Events, canonicalCRDocEvent(event))
	}
	if doc.Events == nil {
		doc.Events = []CRDocEvent{}
	}
	return doc
}

func canonicalCRDocEvent(event model.Event) CRDocEvent {
	out := CRDocEvent{
		TS:              event.TS,
		Actor:           event.Actor,
		Type:            event.Type,
		Summary:         event.Summary,
		Ref:             strings.TrimSpace(event.Ref),
		Redacted:        event.Redacted,
		RedactionReason: strings.TrimSpace(event.RedactionReason),
		Meta:            make([]CRDocMetaEntry, 0, len(event.Meta)),
	}
	if len(event.Meta) > 0 {
		keys := make([]string, 0, len(event.Meta))
		for key := range event.Meta {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			out.Meta = append(out.Meta, CRDocMetaEntry{
				Key:   key,
				Value: event.Meta[key],
			})
		}
	}
	if out.Meta == nil {
		out.Meta = []CRDocMetaEntry{}
	}
	return out
}

func fingerprintCRDoc(doc *CRDoc) (string, error) {
	if doc == nil {
		return "", nil
	}
	payload, err := json.Marshal(doc)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:]), nil
}

func cloneContract(contract model.Contract) model.Contract {
	out := contract
	out.Scope = append([]string(nil), contract.Scope...)
	out.NonGoals = append([]string(nil), contract.NonGoals...)
	out.Invariants = append([]string(nil), contract.Invariants...)
	out.RiskCriticalScopes = append([]string(nil), contract.RiskCriticalScopes...)
	return out
}

func cloneSubtasks(tasks []model.Subtask) []model.Subtask {
	if len(tasks) == 0 {
		return []model.Subtask{}
	}
	out := make([]model.Subtask, 0, len(tasks))
	for _, task := range tasks {
		copyTask := task
		copyTask.CheckpointScope = append([]string(nil), task.CheckpointScope...)
		copyTask.CheckpointChunks = append([]model.CheckpointChunk(nil), task.CheckpointChunks...)
		copyTask.Delegations = append([]model.TaskDelegation(nil), task.Delegations...)
		copyTask.Contract = cloneTaskContract(task.Contract)
		copyTask.ContractBaseline = cloneTaskContractBaseline(task.ContractBaseline)
		copyTask.ContractDrifts = cloneTaskContractDrifts(task.ContractDrifts)
		out = append(out, copyTask)
	}
	return out
}

func cloneTaskContractBaseline(baseline model.TaskContractBaseline) model.TaskContractBaseline {
	out := baseline
	out.AcceptanceCriteria = append([]string(nil), baseline.AcceptanceCriteria...)
	out.Scope = append([]string(nil), baseline.Scope...)
	out.AcceptanceChecks = append([]string(nil), baseline.AcceptanceChecks...)
	return out
}

func cloneTaskContractDrifts(drifts []model.TaskContractDrift) []model.TaskContractDrift {
	if len(drifts) == 0 {
		return []model.TaskContractDrift{}
	}
	out := make([]model.TaskContractDrift, 0, len(drifts))
	for _, drift := range drifts {
		copyDrift := drift
		copyDrift.Fields = append([]string(nil), drift.Fields...)
		copyDrift.BeforeScope = append([]string(nil), drift.BeforeScope...)
		copyDrift.AfterScope = append([]string(nil), drift.AfterScope...)
		copyDrift.BeforeAcceptanceChecks = append([]string(nil), drift.BeforeAcceptanceChecks...)
		copyDrift.AfterAcceptanceChecks = append([]string(nil), drift.AfterAcceptanceChecks...)
		out = append(out, copyDrift)
	}
	return out
}
