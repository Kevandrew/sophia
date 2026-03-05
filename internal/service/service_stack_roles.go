package service

import (
	"sort"
	"strings"

	"sophia/internal/model"
)

type aggregateParentAssessment struct {
	IsAggregateParent          bool
	ResolvedDelegatedTaskCount int
	PendingDelegatedTaskCount  int
}

type AggregateParentView struct {
	IsAggregateParent  bool
	ResolvedChildCRIDs []int
	PendingChildCRIDs  []int
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

func (s *Service) aggregateParentViewForCR(cr *model.CR) AggregateParentView {
	view := AggregateParentView{}
	if cr == nil {
		return view
	}
	assessment := assessAggregateParentTasks(cr.Subtasks)
	if !assessment.IsAggregateParent {
		return view
	}
	view.IsAggregateParent = true

	pending := map[int]struct{}{}
	resolved := map[int]struct{}{}
	for _, task := range cr.Subtasks {
		if len(task.Delegations) == 0 {
			continue
		}
		pendingByTask := map[int]struct{}{}
		for _, childID := range s.pendingDelegationChildIDs(task) {
			if childID <= 0 {
				continue
			}
			pendingByTask[childID] = struct{}{}
			pending[childID] = struct{}{}
		}
		for _, delegation := range task.Delegations {
			if delegation.ChildCRID <= 0 {
				continue
			}
			if _, blocked := pendingByTask[delegation.ChildCRID]; blocked {
				continue
			}
			resolved[delegation.ChildCRID] = struct{}{}
		}
	}
	for childID := range pending {
		delete(resolved, childID)
	}
	view.ResolvedChildCRIDs = sortedIntKeys(resolved)
	view.PendingChildCRIDs = sortedIntKeys(pending)
	return view
}

func sortedIntKeys(values map[int]struct{}) []int {
	if len(values) == 0 {
		return []int{}
	}
	out := make([]int, 0, len(values))
	for value := range values {
		out = append(out, value)
	}
	sort.Ints(out)
	return out
}
