package service

import (
	"testing"

	"sophia/internal/model"
)

func TestFingerprintHQIntentCRIgnoresVolatileLocalFields(t *testing.T) {
	cr := &model.CR{
		ID:          1,
		UID:         "cr_intent",
		Title:       "Title",
		Description: "Desc",
		Status:      model.StatusInProgress,
		Contract: model.Contract{
			Why:        "why",
			Scope:      []string{"internal/service"},
			NonGoals:   []string{"no"},
			Invariants: []string{"inv"},
		},
		Notes: []string{"note 1"},
		Subtasks: []model.Subtask{
			{
				ID:     1,
				Title:  "Task",
				Status: model.TaskStatusOpen,
				Contract: model.TaskContract{
					Intent:             "task intent",
					AcceptanceCriteria: []string{"ac1"},
					Scope:              []string{"internal/cli"},
				},
				CheckpointCommit: "abc123",
				CheckpointScope:  []string{"internal/service"},
			},
		},
		Events: []model.Event{
			{Type: "created", Summary: "created"},
		},
		Evidence: []model.EvidenceEntry{
			{Type: "command", Summary: "evidence"},
		},
	}
	first, err := fingerprintHQIntentCR(cr)
	if err != nil {
		t.Fatalf("fingerprintHQIntentCR(first) error = %v", err)
	}

	cr.Events = append(cr.Events, model.Event{Type: "updated", Summary: "updated"})
	cr.Subtasks[0].CheckpointCommit = "def456"
	cr.Subtasks[0].CheckpointScope = []string{"internal/model"}
	second, err := fingerprintHQIntentCR(cr)
	if err != nil {
		t.Fatalf("fingerprintHQIntentCR(second) error = %v", err)
	}
	if first != second {
		t.Fatalf("expected volatile-field edits to keep intent fingerprint stable: %q != %q", first, second)
	}
}

func TestFingerprintHQIntentCRIgnoresUID(t *testing.T) {
	cr := &model.CR{
		ID:          1,
		UID:         "cr_one",
		Title:       "Title",
		Description: "Desc",
		Status:      model.StatusInProgress,
		Contract: model.Contract{
			Why: "why",
		},
	}
	first, err := fingerprintHQIntentCR(cr)
	if err != nil {
		t.Fatalf("fingerprintHQIntentCR(first) error = %v", err)
	}
	cr.UID = "cr_two"
	second, err := fingerprintHQIntentCR(cr)
	if err != nil {
		t.Fatalf("fingerprintHQIntentCR(second) error = %v", err)
	}
	if first != second {
		t.Fatalf("expected uid change to not affect intent fingerprint: %q != %q", first, second)
	}
}

func TestFingerprintHQIntentCRChangesForIntentEdits(t *testing.T) {
	cr := &model.CR{
		ID:          1,
		UID:         "cr_intent_2",
		Title:       "Title",
		Description: "Desc",
		Status:      model.StatusInProgress,
		Contract: model.Contract{
			Why: "why",
		},
		Notes: []string{"note 1"},
		Subtasks: []model.Subtask{
			{
				ID:     1,
				Title:  "Task",
				Status: model.TaskStatusOpen,
				Contract: model.TaskContract{
					Intent: "task intent",
				},
			},
		},
	}
	first, err := fingerprintHQIntentCR(cr)
	if err != nil {
		t.Fatalf("fingerprintHQIntentCR(first) error = %v", err)
	}

	cr.Contract.Why = "new why"
	second, err := fingerprintHQIntentCR(cr)
	if err != nil {
		t.Fatalf("fingerprintHQIntentCR(second) error = %v", err)
	}
	if first == second {
		t.Fatalf("expected contract why change to alter intent fingerprint")
	}
}
