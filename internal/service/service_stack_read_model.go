package service

import (
	"sort"
	"strings"

	"sophia/internal/model"
)

type StackLineageNodeView struct {
	ID        int
	UID       string
	Title     string
	Status    string
	Branch    string
	BaseRef   string
	Depth     int
	Role      string
	RoleLabel string
}

type StackTreeNodeView struct {
	ID                    int
	UID                   string
	Title                 string
	Status                string
	Branch                string
	BaseRef               string
	ParentCRID            int
	Depth                 int
	Role                  string
	RoleLabel             string
	IsChild               bool
	IsRootParent          bool
	IsAggregateParent     bool
	TasksTotal            int
	TasksOpen             int
	TasksDone             int
	TasksDelegated        int
	TasksDelegatedPending int
	ChildCount            int
	ResolvedChildCount    int
	PendingChildCount     int
	ResolutionState       string
	Children              []StackTreeNodeView
}

func (s *Service) stackLineageForCR(cr *model.CR) []StackLineageNodeView {
	if s == nil || cr == nil || cr.ParentCRID <= 0 {
		return nil
	}
	allCRs, err := s.store.ListCRs()
	if err != nil {
		return nil
	}
	byID := make(map[int]model.CR, len(allCRs))
	for _, candidate := range allCRs {
		byID[candidate.ID] = candidate
	}

	lineage := make([]StackLineageNodeView, 0)
	visited := map[int]struct{}{}
	currentID := cr.ParentCRID
	for currentID > 0 {
		if _, seen := visited[currentID]; seen {
			break
		}
		visited[currentID] = struct{}{}
		parent, ok := byID[currentID]
		if !ok {
			break
		}
		nativity := s.stackNativityForCR(&parent)
		lineage = append(lineage, StackLineageNodeView{
			ID:        parent.ID,
			UID:       strings.TrimSpace(parent.UID),
			Title:     strings.TrimSpace(parent.Title),
			Status:    strings.TrimSpace(parent.Status),
			Branch:    strings.TrimSpace(parent.Branch),
			BaseRef:   nonEmptyTrimmed(parent.BaseRef, parent.BaseBranch),
			Depth:     len(lineage),
			Role:      nativity.Role,
			RoleLabel: nativity.RoleLabel,
		})
		currentID = parent.ParentCRID
	}
	if len(lineage) == 0 {
		return nil
	}
	for i, j := 0, len(lineage)-1; i < j; i, j = i+1, j-1 {
		lineage[i], lineage[j] = lineage[j], lineage[i]
	}
	for i := range lineage {
		lineage[i].Depth = i
	}
	return lineage
}

func (s *Service) stackTreeForCR(cr *model.CR) *StackTreeNodeView {
	if s == nil || cr == nil {
		return nil
	}
	allCRs, err := s.store.ListCRs()
	if err != nil {
		return nil
	}
	childrenByParent := make(map[int][]model.CR)
	for _, candidate := range allCRs {
		if candidate.ParentCRID > 0 {
			childrenByParent[candidate.ParentCRID] = append(childrenByParent[candidate.ParentCRID], candidate)
		}
	}
	if len(childrenByParent[cr.ID]) == 0 {
		return nil
	}
	sort.SliceStable(childrenByParent[cr.ID], func(i, j int) bool {
		if childrenByParent[cr.ID][i].ID == childrenByParent[cr.ID][j].ID {
			return childrenByParent[cr.ID][i].Title < childrenByParent[cr.ID][j].Title
		}
		return childrenByParent[cr.ID][i].ID < childrenByParent[cr.ID][j].ID
	})
	root := s.buildStackTreeNode(*cr, 0, childrenByParent)
	return &root
}

func (s *Service) buildStackTreeNode(cr model.CR, depth int, childrenByParent map[int][]model.CR) StackTreeNodeView {
	nativity := s.stackNativityForCR(&cr)
	aggregate := s.aggregateParentViewForCR(&cr)
	assessment := assessAggregateParentTasks(cr.Subtasks)
	node := StackTreeNodeView{
		ID:                    cr.ID,
		UID:                   strings.TrimSpace(cr.UID),
		Title:                 strings.TrimSpace(cr.Title),
		Status:                strings.TrimSpace(cr.Status),
		Branch:                strings.TrimSpace(cr.Branch),
		BaseRef:               nonEmptyTrimmed(cr.BaseRef, cr.BaseBranch),
		ParentCRID:            cr.ParentCRID,
		Depth:                 depth,
		Role:                  nativity.Role,
		RoleLabel:             nativity.RoleLabel,
		IsChild:               nativity.IsChild,
		IsRootParent:          nativity.IsRootParent,
		IsAggregateParent:     nativity.IsAggregateParent,
		TasksTotal:            len(cr.Subtasks),
		TasksOpen:             countTasksByStatus(cr.Subtasks, model.TaskStatusOpen),
		TasksDone:             countTasksByStatus(cr.Subtasks, model.TaskStatusDone),
		TasksDelegated:        countTasksByStatus(cr.Subtasks, model.TaskStatusDelegated),
		TasksDelegatedPending: assessment.PendingDelegatedTaskCount,
		ChildCount:            len(childrenByParent[cr.ID]),
		ResolvedChildCount:    len(aggregate.ResolvedChildCRIDs),
		PendingChildCount:     len(aggregate.PendingChildCRIDs),
	}

	if children := childrenByParent[cr.ID]; len(children) > 0 {
		sort.SliceStable(children, func(i, j int) bool {
			if children[i].ID == children[j].ID {
				return children[i].Title < children[j].Title
			}
			return children[i].ID < children[j].ID
		})
		node.Children = make([]StackTreeNodeView, 0, len(children))
		for _, child := range children {
			childNode := s.buildStackTreeNode(child, depth+1, childrenByParent)
			childNode.ResolutionState = childResolutionState(aggregate, child.ID)
			node.Children = append(node.Children, childNode)
		}
	}
	return node
}

func childResolutionState(parent AggregateParentView, childID int) string {
	for _, id := range parent.PendingChildCRIDs {
		if id == childID {
			return "pending"
		}
	}
	for _, id := range parent.ResolvedChildCRIDs {
		if id == childID {
			return "resolved"
		}
	}
	return ""
}

func countTasksByStatus(tasks []model.Subtask, status string) int {
	count := 0
	for _, task := range tasks {
		if strings.TrimSpace(task.Status) == strings.TrimSpace(status) {
			count++
		}
	}
	return count
}
