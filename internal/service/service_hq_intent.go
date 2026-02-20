package service

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"reflect"
	"sort"
	"strconv"
	"strings"

	"sophia/internal/model"
)

type HQIntentFieldConflict struct {
	Field    string `json:"field"`
	Upstream any    `json:"upstream,omitempty"`
	Local    any    `json:"local,omitempty"`
	Remote   any    `json:"remote,omitempty"`
}

func canonicalHQIntentSnapshot(cr *model.CR) *model.HQIntentSnapshot {
	if cr == nil {
		return nil
	}
	doc := &model.HQIntentSnapshot{
		Title:       strings.TrimSpace(cr.Title),
		Description: strings.TrimSpace(cr.Description),
		Status:      strings.TrimSpace(cr.Status),
		Contract:    canonicalHQIntentContract(cr.Contract),
		Notes:       normalizeStringList(cr.Notes),
		Subtasks:    make([]model.HQIntentTaskSnapshot, 0, len(cr.Subtasks)),
	}
	for _, task := range cr.Subtasks {
		item := model.HQIntentTaskSnapshot{
			ID:       task.ID,
			Title:    strings.TrimSpace(task.Title),
			Status:   strings.TrimSpace(task.Status),
			Contract: canonicalHQIntentTaskContract(task.Contract),
		}
		doc.Subtasks = append(doc.Subtasks, item)
	}
	sort.Slice(doc.Subtasks, func(i, j int) bool {
		return doc.Subtasks[i].ID < doc.Subtasks[j].ID
	})
	return doc
}

func canonicalHQIntentContract(contract model.Contract) model.HQIntentContractSnapshot {
	return model.HQIntentContractSnapshot{
		Why:                strings.TrimSpace(contract.Why),
		Scope:              normalizeStringList(contract.Scope),
		NonGoals:           normalizeStringList(contract.NonGoals),
		Invariants:         normalizeStringList(contract.Invariants),
		BlastRadius:        strings.TrimSpace(contract.BlastRadius),
		RiskCriticalScopes: normalizeStringList(contract.RiskCriticalScopes),
		RiskTierHint:       strings.TrimSpace(contract.RiskTierHint),
		RiskRationale:      strings.TrimSpace(contract.RiskRationale),
		TestPlan:           strings.TrimSpace(contract.TestPlan),
		RollbackPlan:       strings.TrimSpace(contract.RollbackPlan),
	}
}

func canonicalHQIntentTaskContract(contract model.TaskContract) model.HQIntentTaskContractSnapshot {
	return model.HQIntentTaskContractSnapshot{
		Intent:             strings.TrimSpace(contract.Intent),
		AcceptanceCriteria: normalizeStringList(contract.AcceptanceCriteria),
		Scope:              normalizeStringList(contract.Scope),
		AcceptanceChecks:   normalizeStringList(contract.AcceptanceChecks),
	}
}

func normalizeStringList(items []string) []string {
	if len(items) == 0 {
		return []string{}
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		v := strings.TrimSpace(item)
		if v == "" {
			continue
		}
		out = append(out, v)
	}
	if len(out) == 0 {
		return []string{}
	}
	return out
}

func fingerprintHQIntentSnapshot(doc *model.HQIntentSnapshot) (string, error) {
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

func fingerprintHQIntentCR(cr *model.CR) (string, error) {
	return fingerprintHQIntentSnapshot(canonicalHQIntentSnapshot(cr))
}

func flattenHQIntentFields(doc *model.HQIntentSnapshot) map[string]any {
	out := map[string]any{}
	if doc == nil {
		return out
	}
	out["cr.title"] = doc.Title
	out["cr.description"] = doc.Description
	out["cr.status"] = doc.Status
	out["cr.contract.why"] = doc.Contract.Why
	out["cr.contract.scope"] = append([]string(nil), doc.Contract.Scope...)
	out["cr.contract.non_goals"] = append([]string(nil), doc.Contract.NonGoals...)
	out["cr.contract.invariants"] = append([]string(nil), doc.Contract.Invariants...)
	out["cr.contract.blast_radius"] = doc.Contract.BlastRadius
	out["cr.contract.risk_critical_scopes"] = append([]string(nil), doc.Contract.RiskCriticalScopes...)
	out["cr.contract.risk_tier_hint"] = doc.Contract.RiskTierHint
	out["cr.contract.risk_rationale"] = doc.Contract.RiskRationale
	out["cr.contract.test_plan"] = doc.Contract.TestPlan
	out["cr.contract.rollback_plan"] = doc.Contract.RollbackPlan
	out["cr.notes"] = append([]string(nil), doc.Notes...)

	taskByID := map[int]model.HQIntentTaskSnapshot{}
	taskIDs := make([]int, 0, len(doc.Subtasks))
	for _, task := range doc.Subtasks {
		taskIDs = append(taskIDs, task.ID)
		taskByID[task.ID] = task
	}
	sort.Ints(taskIDs)
	out["cr.tasks.ids"] = append([]int(nil), taskIDs...)
	for _, id := range taskIDs {
		task := taskByID[id]
		prefix := "cr.tasks." + strconv.Itoa(id)
		out[prefix+".title"] = task.Title
		out[prefix+".status"] = task.Status
		out[prefix+".contract.intent"] = task.Contract.Intent
		out[prefix+".contract.acceptance_criteria"] = append([]string(nil), task.Contract.AcceptanceCriteria...)
		out[prefix+".contract.scope"] = append([]string(nil), task.Contract.Scope...)
		out[prefix+".contract.acceptance_checks"] = append([]string(nil), task.Contract.AcceptanceChecks...)
	}
	return out
}

func diffHQIntentFields(local, remote *model.HQIntentSnapshot) ([]string, []HQIntentFieldConflict) {
	localFields := flattenHQIntentFields(local)
	remoteFields := flattenHQIntentFields(remote)

	keys := map[string]struct{}{}
	for key := range localFields {
		keys[key] = struct{}{}
	}
	for key := range remoteFields {
		keys[key] = struct{}{}
	}
	sorted := make([]string, 0, len(keys))
	for key := range keys {
		sorted = append(sorted, key)
	}
	sort.Strings(sorted)

	changed := []string{}
	conflicts := []HQIntentFieldConflict{}
	for _, key := range sorted {
		lv, lok := localFields[key]
		rv, rok := remoteFields[key]
		if !lok {
			lv = nil
		}
		if !rok {
			rv = nil
		}
		if reflect.DeepEqual(lv, rv) {
			continue
		}
		changed = append(changed, key)
		conflicts = append(conflicts, HQIntentFieldConflict{
			Field:  key,
			Local:  lv,
			Remote: rv,
		})
	}
	return changed, conflicts
}

func diffHQIntentFields3(upstream, local, remote *model.HQIntentSnapshot) ([]string, []string, []HQIntentFieldConflict) {
	upstreamFields := flattenHQIntentFields(upstream)
	localFields := flattenHQIntentFields(local)
	remoteFields := flattenHQIntentFields(remote)

	keys := map[string]struct{}{}
	for key := range upstreamFields {
		keys[key] = struct{}{}
	}
	for key := range localFields {
		keys[key] = struct{}{}
	}
	for key := range remoteFields {
		keys[key] = struct{}{}
	}
	sorted := make([]string, 0, len(keys))
	for key := range keys {
		sorted = append(sorted, key)
	}
	sort.Strings(sorted)

	localChanged := []string{}
	remoteChanged := []string{}
	conflicts := []HQIntentFieldConflict{}
	for _, key := range sorted {
		uv := upstreamFields[key]
		lv := localFields[key]
		rv := remoteFields[key]
		localDiffers := !reflect.DeepEqual(lv, uv)
		remoteDiffers := !reflect.DeepEqual(rv, uv)
		if localDiffers {
			localChanged = append(localChanged, key)
		}
		if remoteDiffers {
			remoteChanged = append(remoteChanged, key)
		}
		if localDiffers && remoteDiffers && !reflect.DeepEqual(lv, rv) {
			conflicts = append(conflicts, HQIntentFieldConflict{
				Field:    key,
				Upstream: uv,
				Local:    lv,
				Remote:   rv,
			})
		}
	}
	return localChanged, remoteChanged, conflicts
}
