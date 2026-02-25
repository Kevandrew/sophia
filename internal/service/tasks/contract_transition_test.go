package tasks

import (
	"sophia/internal/model"
	"strings"
	"testing"
)

func TestApplyTaskContractTransitionRecordsDriftAndBaseline(t *testing.T) {
	task := &model.Subtask{
		ID:               1,
		Status:           model.TaskStatusDone,
		CheckpointCommit: "abc1234",
		Contract: model.TaskContract{
			Intent:             "new intent",
			AcceptanceCriteria: []string{"new acceptance"},
			Scope:              []string{"."},
			AcceptanceChecks:   []string{"unit_tests"},
		},
	}
	before := model.TaskContract{
		Intent:             "old intent",
		AcceptanceCriteria: []string{"old acceptance"},
		Scope:              []string{"internal/service"},
		AcceptanceChecks:   []string{"lint"},
	}

	drift := ApplyTaskContractTransition(task, before, "2026-02-25T12:00:00Z", "Tester", "scope widened", func(path, scopePrefix string) bool {
		return strings.HasPrefix(path, scopePrefix)
	})
	if drift == nil {
		t.Fatalf("expected drift to be recorded")
	}
	if len(task.ContractDrifts) != 1 {
		t.Fatalf("expected one drift record, got %#v", task.ContractDrifts)
	}
	if task.ContractBaseline.Intent != "old intent" {
		t.Fatalf("expected baseline from pre-change contract, got %#v", task.ContractBaseline)
	}
	if !task.ContractDrifts[0].Acknowledged || task.ContractDrifts[0].AckReason != "scope widened" {
		t.Fatalf("expected acknowledged drift from change reason, got %#v", task.ContractDrifts[0])
	}
}

func TestApplyTaskContractTransitionWithoutCheckpointSkipsDrift(t *testing.T) {
	task := &model.Subtask{
		ID:     1,
		Status: model.TaskStatusOpen,
		Contract: model.TaskContract{
			Intent: "intent",
			Scope:  []string{"internal/service"},
		},
	}
	before := task.Contract

	drift := ApplyTaskContractTransition(task, before, "2026-02-25T12:00:00Z", "Tester", "", func(path, scopePrefix string) bool {
		return strings.HasPrefix(path, scopePrefix)
	})
	if drift != nil {
		t.Fatalf("expected no drift without checkpoint, got %#v", drift)
	}
	if len(task.ContractDrifts) != 0 {
		t.Fatalf("expected no drift records, got %#v", task.ContractDrifts)
	}
	if task.Contract.UpdatedAt == "" || task.Contract.UpdatedBy == "" {
		t.Fatalf("expected contract update metadata to be set, got %#v", task.Contract)
	}
}
