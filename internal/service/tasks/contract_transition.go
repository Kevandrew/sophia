package tasks

import (
	"sophia/internal/model"
	"strings"
)

// ApplyTaskContractTransition applies post-patch task contract domain state updates:
// baseline capture (when checkpointed), drift recording, and contract/task timestamps.
func ApplyTaskContractTransition(
	task *model.Subtask,
	beforeContract model.TaskContract,
	now string,
	actor string,
	changeReason string,
	pathMatchesScopePrefix func(path, scopePrefix string) bool,
) *model.TaskContractDrift {
	if task == nil {
		return nil
	}

	taskHasCheckpoint := strings.TrimSpace(task.CheckpointCommit) != ""
	if taskHasCheckpoint && TaskContractBaselineIsEmpty(task.ContractBaseline) {
		task.ContractBaseline = TaskContractBaselineFromContract(beforeContract, now, actor)
	}

	var drift *model.TaskContractDrift
	if taskHasCheckpoint {
		driftFields := []string{}
		scopeChanged := !equalStringSlices(beforeContract.Scope, task.Contract.Scope)
		if scopeChanged && ScopeWidened(beforeContract.Scope, task.Contract.Scope, pathMatchesScopePrefix) {
			driftFields = append(driftFields, "scope_widened")
		}
		checksChanged := !equalStringSlices(beforeContract.AcceptanceChecks, task.Contract.AcceptanceChecks)
		if checksChanged {
			driftFields = append(driftFields, "acceptance_checks_changed")
		}
		if len(driftFields) > 0 {
			record := model.TaskContractDrift{
				ID:                     NextTaskContractDriftID(task.ContractDrifts),
				TS:                     now,
				Actor:                  actor,
				Fields:                 append([]string(nil), driftFields...),
				BeforeScope:            append([]string(nil), beforeContract.Scope...),
				AfterScope:             append([]string(nil), task.Contract.Scope...),
				BeforeAcceptanceChecks: append([]string(nil), beforeContract.AcceptanceChecks...),
				AfterAcceptanceChecks:  append([]string(nil), task.Contract.AcceptanceChecks...),
				CheckpointCommit:       strings.TrimSpace(task.CheckpointCommit),
				Reason:                 changeReason,
			}
			if changeReason != "" {
				record.Acknowledged = true
				record.AcknowledgedAt = now
				record.AcknowledgedBy = actor
				record.AckReason = changeReason
			}
			task.ContractDrifts = append(task.ContractDrifts, record)
			recordCopy := record
			drift = &recordCopy
		}
	}

	task.Contract.UpdatedAt = now
	task.Contract.UpdatedBy = actor
	task.UpdatedAt = now
	return drift
}

func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
