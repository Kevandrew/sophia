package service

import (
	"sort"
	"strings"

	"sophia/internal/model"
)

func (s *Service) SearchCRs(query model.CRSearchQuery) ([]model.CRSearchResult, error) {
	crs, err := s.store.ListCRs()
	if err != nil {
		return nil, err
	}

	var results []model.CRSearchResult
	for _, cr := range crs {
		if !matchCRSearch(cr, query) {
			continue
		}

		tasksOpen, tasksDone, _ := countTaskStats(cr.Subtasks)
		riskTier := cr.Contract.RiskTierHint
		if riskTier == "" {
			riskTier = "-"
		}

		results = append(results, model.CRSearchResult{
			ID:         cr.ID,
			UID:        cr.UID,
			Title:      cr.Title,
			Status:     cr.Status,
			Branch:     cr.Branch,
			BaseBranch: cr.BaseBranch,
			ParentCRID: cr.ParentCRID,
			RiskTier:   riskTier,
			TasksTotal: len(cr.Subtasks),
			TasksOpen:  tasksOpen,
			TasksDone:  tasksDone,
			CreatedAt:  cr.CreatedAt,
			UpdatedAt:  cr.UpdatedAt,
		})
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].ID < results[j].ID
	})

	return results, nil
}

func matchCRSearch(cr model.CR, query model.CRSearchQuery) bool {
	if strings.TrimSpace(query.Status) != "" && cr.Status != query.Status {
		return false
	}

	if strings.TrimSpace(query.RiskTier) != "" {
		crTier := strings.ToLower(strings.TrimSpace(cr.Contract.RiskTierHint))
		if crTier == "" {
			crTier = "-"
		}
		if crTier != strings.ToLower(strings.TrimSpace(query.RiskTier)) {
			return false
		}
	}

	if strings.TrimSpace(query.ScopePrefix) != "" {
		if !hasScopePrefix(cr.Contract.Scope, query.ScopePrefix) {
			return false
		}
	}

	if strings.TrimSpace(query.Text) != "" {
		if !matchCRText(cr, strings.ToLower(strings.TrimSpace(query.Text))) {
			return false
		}
	}

	return true
}

func hasScopePrefix(scopes []string, prefix string) bool {
	prefix = strings.ToLower(strings.TrimSpace(prefix))
	for _, s := range scopes {
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(s)), prefix) {
			return true
		}
	}
	return false
}

func matchCRText(cr model.CR, text string) bool {
	if strings.Contains(strings.ToLower(cr.Title), text) {
		return true
	}
	if strings.Contains(strings.ToLower(cr.Description), text) {
		return true
	}
	if strings.Contains(strings.ToLower(cr.Contract.Why), text) {
		return true
	}
	if strings.Contains(strings.ToLower(cr.Contract.BlastRadius), text) {
		return true
	}
	for _, note := range cr.Notes {
		if strings.Contains(strings.ToLower(note), text) {
			return true
		}
	}
	return false
}

func countTaskStats(tasks []model.Subtask) (open, done, delegated int) {
	for _, t := range tasks {
		switch t.Status {
		case model.TaskStatusDone:
			done++
		case model.TaskStatusDelegated:
			delegated++
		default:
			open++
		}
	}
	return
}
