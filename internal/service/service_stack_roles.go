package service

import (
	"strings"

	"sophia/internal/model"
)

type aggregateParentAssessment struct {
	IsAggregateParent          bool
	ResolvedDelegatedTaskCount int
	PendingDelegatedTaskCount  int
}

func assessAggregateParentTasks(tasks []model.Subtask) aggregateParentAssessment {
	assessment := aggregateParentAssessment{}
	if len(tasks) == 0 {
		return assessment
	}

	hasDelegatedTasks := false
	for _, task := range tasks {
		if strings.TrimSpace(task.CheckpointCommit) != "" {
			return aggregateParentAssessment{}
		}
		if len(task.Delegations) > 0 {
			hasDelegatedTasks = true
			switch task.Status {
			case model.TaskStatusDone:
				assessment.ResolvedDelegatedTaskCount++
			case model.TaskStatusDelegated:
				assessment.PendingDelegatedTaskCount++
			default:
				return aggregateParentAssessment{}
			}
			continue
		}
		if task.Status == model.TaskStatusDone &&
			strings.TrimSpace(task.CheckpointReason) != "" &&
			strings.TrimSpace(task.CheckpointSource) == model.TaskCheckpointSourceTaskNoCheckpoint {
			continue
		}
		return aggregateParentAssessment{}
	}

	if !hasDelegatedTasks {
		return aggregateParentAssessment{}
	}
	assessment.IsAggregateParent = true
	return assessment
}

func hasAggregateParentImplementationProof(cr *model.CR) bool {
	if cr == nil {
		return false
	}
	assessment := assessAggregateParentTasks(cr.Subtasks)
	return assessment.IsAggregateParent && assessment.PendingDelegatedTaskCount == 0 && assessment.ResolvedDelegatedTaskCount > 0
}
