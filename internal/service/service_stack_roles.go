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

type StackNativityView struct {
	Role               string
	RoleLabel          string
	IsChild            bool
	IsRootParent       bool
	IsAggregateParent  bool
	ParentCRID         int
	ParentTitle        string
	ParentBranch       string
	ParentStatus       string
	ChildCRIDs         []int
	ResolvedChildCRIDs []int
	PendingChildCRIDs  []int
	ChildCount         int
	ResolvedChildCount int
	PendingChildCount  int
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

func (s *Service) stackNativityForCR(cr *model.CR) StackNativityView {
	view := StackNativityView{
		Role:      "standalone",
		RoleLabel: "Standalone CR",
	}
	if cr == nil {
		return view
	}

	allCRs, err := s.store.ListCRs()
	if err != nil {
		return view
	}
	children := make([]int, 0)
	for _, candidate := range allCRs {
		if candidate.ParentCRID == cr.ID {
			children = append(children, candidate.ID)
		}
	}
	sort.Ints(children)

	aggregate := s.aggregateParentViewForCR(cr)
	view.IsAggregateParent = aggregate.IsAggregateParent
	view.ResolvedChildCRIDs = append([]int(nil), aggregate.ResolvedChildCRIDs...)
	view.PendingChildCRIDs = append([]int(nil), aggregate.PendingChildCRIDs...)
	view.ResolvedChildCount = len(view.ResolvedChildCRIDs)
	view.PendingChildCount = len(view.PendingChildCRIDs)
	view.ChildCRIDs = append([]int(nil), children...)
	view.ChildCount = len(children)

	if cr.ParentCRID > 0 {
		view.IsChild = true
		view.ParentCRID = cr.ParentCRID
		view.Role = "child"
		view.RoleLabel = "Child CR"
		if parent, err := s.store.LoadCR(cr.ParentCRID); err == nil && parent != nil {
			view.ParentTitle = strings.TrimSpace(parent.Title)
			view.ParentBranch = strings.TrimSpace(parent.Branch)
			view.ParentStatus = strings.TrimSpace(parent.Status)
		}
		return view
	}

	if len(children) > 0 {
		view.IsRootParent = true
		view.Role = "root_parent"
		view.RoleLabel = "Root Parent"
	}
	if aggregate.IsAggregateParent {
		view.Role = "aggregate_parent"
		view.RoleLabel = "Aggregate Parent"
		if !view.IsRootParent && len(children) > 0 {
			view.IsRootParent = true
		}
		return view
	}
	if view.IsRootParent {
		return view
	}
	return view
}

func (s *Service) StackNativityForCLI(cr *model.CR) StackNativityView {
	return s.stackNativityForCR(cr)
}

func (s *Service) StackLineageForCLI(cr *model.CR) []StackLineageNodeView {
	return s.stackLineageForCR(cr)
}

func (s *Service) StackTreeForCLI(cr *model.CR) *StackTreeNodeView {
	return s.stackTreeForCR(cr)
}
