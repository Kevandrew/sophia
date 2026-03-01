package service

import (
	"sophia/internal/model"
	"testing"
)

func TestMatchCRSearch_Status(t *testing.T) {
	t.Parallel()
	cr := model.CR{Status: model.StatusInProgress}
	if !matchCRSearch(cr, model.CRSearchQuery{Status: model.StatusInProgress}) {
		t.Error("expected match for same status")
	}
	if matchCRSearch(cr, model.CRSearchQuery{Status: model.StatusMerged}) {
		t.Error("expected no match for different status")
	}
}

func TestMatchCRSearch_RiskTier(t *testing.T) {
	t.Parallel()
	cr := model.CR{Contract: model.Contract{RiskTierHint: "high"}}
	if !matchCRSearch(cr, model.CRSearchQuery{RiskTier: "high"}) {
		t.Error("expected match for same risk tier")
	}
	if !matchCRSearch(cr, model.CRSearchQuery{RiskTier: "HIGH"}) {
		t.Error("expected match for case-insensitive risk tier")
	}
	if matchCRSearch(cr, model.CRSearchQuery{RiskTier: "low"}) {
		t.Error("expected no match for different risk tier")
	}
	cr2 := model.CR{Contract: model.Contract{RiskTierHint: ""}}
	if matchCRSearch(cr2, model.CRSearchQuery{RiskTier: "high"}) {
		t.Error("expected no match when risk tier empty but filter set")
	}
}

func TestMatchCRSearch_ScopePrefix(t *testing.T) {
	t.Parallel()
	cr := model.CR{Contract: model.Contract{Scope: []string{"internal/cli", "internal/service"}}}
	if !matchCRSearch(cr, model.CRSearchQuery{ScopePrefix: "internal/cli"}) {
		t.Error("expected match for exact scope")
	}
	if !matchCRSearch(cr, model.CRSearchQuery{ScopePrefix: "internal/"}) {
		t.Error("expected match for scope prefix")
	}
	if !matchCRSearch(cr, model.CRSearchQuery{ScopePrefix: "INTERNAL/"}) {
		t.Error("expected match for case-insensitive scope prefix")
	}
	if matchCRSearch(cr, model.CRSearchQuery{ScopePrefix: "cmd/"}) {
		t.Error("expected no match for different scope")
	}
}

func TestMatchCRSearch_Text(t *testing.T) {
	t.Parallel()
	cr := model.CR{
		Title:       "CR index and search primitives",
		Description: "Discovery improvements",
		Contract:    model.Contract{Why: "Enable discovery", BlastRadius: "CLI commands"},
		Notes:       []string{"Note about search"},
	}
	if !matchCRSearch(cr, model.CRSearchQuery{Text: "index"}) {
		t.Error("expected match in title")
	}
	if !matchCRSearch(cr, model.CRSearchQuery{Text: "discovery"}) {
		t.Error("expected match in description")
	}
	if !matchCRSearch(cr, model.CRSearchQuery{Text: "enable"}) {
		t.Error("expected match in contract.why")
	}
	if !matchCRSearch(cr, model.CRSearchQuery{Text: "commands"}) {
		t.Error("expected match in contract.blast_radius")
	}
	if !matchCRSearch(cr, model.CRSearchQuery{Text: "note"}) {
		t.Error("expected match in notes")
	}
	if !matchCRSearch(cr, model.CRSearchQuery{Text: "INDEX"}) {
		t.Error("expected case-insensitive match")
	}
	if matchCRSearch(cr, model.CRSearchQuery{Text: "nonexistent"}) {
		t.Error("expected no match for missing text")
	}
}

func TestMatchCRSearch_Combined(t *testing.T) {
	t.Parallel()
	cr := model.CR{
		Status:   model.StatusInProgress,
		Title:    "CR index and search primitives",
		Contract: model.Contract{RiskTierHint: "low", Scope: []string{"internal/cli"}},
	}
	if !matchCRSearch(cr, model.CRSearchQuery{
		Status:      model.StatusInProgress,
		Text:        "index",
		RiskTier:    "low",
		ScopePrefix: "internal/",
	}) {
		t.Error("expected match when all filters match")
	}
	if matchCRSearch(cr, model.CRSearchQuery{
		Status:      model.StatusInProgress,
		RiskTier:    "high",
		ScopePrefix: "internal/",
	}) {
		t.Error("expected no match when one filter fails")
	}
}

func TestCountTaskStats(t *testing.T) {
	t.Parallel()
	tasks := []model.Subtask{
		{Status: model.TaskStatusOpen},
		{Status: model.TaskStatusDone},
		{Status: model.TaskStatusDelegated},
		{Status: model.TaskStatusOpen},
	}
	open, done, delegated := countTaskStats(tasks)
	if open != 2 {
		t.Errorf("expected 2 open tasks, got %d", open)
	}
	if done != 1 {
		t.Errorf("expected 1 done task, got %d", done)
	}
	if delegated != 1 {
		t.Errorf("expected 1 delegated task, got %d", delegated)
	}
}
