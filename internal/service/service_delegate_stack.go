package service

import (
	"fmt"
	"sophia/internal/model"
	"sort"
	"strconv"
	"strings"
)

func (s *Service) AddChildCRFromCurrent(title, description string) (*model.CR, []string, error) {
	ctx, err := s.CurrentCR()
	if err != nil {
		return nil, nil, err
	}
	return s.AddCRWithOptionsWithWarnings(title, description, AddCROptions{ParentCRID: ctx.CR.ID, Switch: true})
}

func (s *Service) DelegateTaskToChild(parentCRID, taskID, childCRID int) (*DelegateTaskResult, error) {
	var result *DelegateTaskResult
	if err := s.withMutationLock(func() error {
		var err error
		result, err = s.delegateTaskToChildUnlocked(parentCRID, taskID, childCRID)
		return err
	}); err != nil {
		return nil, err
	}
	return result, nil
}

func (s *Service) delegateTaskToChildUnlocked(parentCRID, taskID, childCRID int) (*DelegateTaskResult, error) {
	parent, err := s.store.LoadCR(parentCRID)
	if err != nil {
		return nil, err
	}
	if guardErr := s.ensureNoMergeInProgressForCR(parent); guardErr != nil {
		return nil, guardErr
	}
	if parent.Status != model.StatusInProgress {
		return nil, fmt.Errorf("parent cr %d is not in progress", parentCRID)
	}
	child, err := s.store.LoadCR(childCRID)
	if err != nil {
		return nil, err
	}
	if guardErr := s.ensureNoMergeInProgressForCR(child); guardErr != nil {
		return nil, guardErr
	}
	if child.Status != model.StatusInProgress {
		return nil, fmt.Errorf("child cr %d is not in progress", childCRID)
	}
	if child.ParentCRID != parent.ID {
		return nil, fmt.Errorf("child cr %d is not a direct child of parent cr %d", child.ID, parent.ID)
	}
	policy, err := s.repoPolicy()
	if err != nil {
		return nil, err
	}

	parentTaskIndex := indexOfTask(parent.Subtasks, taskID)
	if parentTaskIndex < 0 {
		return nil, fmt.Errorf("task %d not found in cr %d", taskID, parentCRID)
	}
	parentTask := &parent.Subtasks[parentTaskIndex]
	if parentTask.Status == model.TaskStatusDone {
		return nil, fmt.Errorf("task %d in cr %d is already done", taskID, parentCRID)
	}
	if missing := missingTaskContractFields(parentTask.Contract, policy.TaskContract.RequiredFields); len(missing) > 0 {
		return nil, fmt.Errorf("%w: task %d missing %s", ErrTaskContractIncomplete, taskID, strings.Join(missing, ","))
	}
	for _, delegation := range parentTask.Delegations {
		if delegation.ChildCRID == child.ID {
			return nil, fmt.Errorf("task %d in cr %d is already delegated to child cr %d", taskID, parentCRID, childCRID)
		}
	}

	now := s.timestamp()
	actor := s.git.Actor()
	childTaskID := nextTaskID(child.Subtasks)
	childTask := model.Subtask{
		ID:        childTaskID,
		Title:     parentTask.Title,
		Status:    model.TaskStatusOpen,
		CreatedAt: now,
		UpdatedAt: now,
		CreatedBy: actor,
		Contract:  parentTask.Contract,
	}
	childTask.Contract.UpdatedAt = now
	childTask.Contract.UpdatedBy = actor
	child.Subtasks = append(child.Subtasks, childTask)
	child.Events = append(child.Events, model.Event{
		TS:      now,
		Actor:   actor,
		Type:    model.EventTypeTaskDelegationReceived,
		Summary: fmt.Sprintf("Received delegated task from CR %d task %d", parent.ID, parentTask.ID),
		Ref:     fmt.Sprintf("task:%d", childTaskID),
		Meta: map[string]string{
			"parent_cr":   strconv.Itoa(parent.ID),
			"parent_task": strconv.Itoa(parentTask.ID),
		},
	})
	child.UpdatedAt = now

	parentTask.Delegations = append(parentTask.Delegations, model.TaskDelegation{
		ChildCRID:   child.ID,
		ChildCRUID:  strings.TrimSpace(child.UID),
		ChildTaskID: childTaskID,
		LinkedAt:    now,
		LinkedBy:    actor,
	})
	parentTask.Status = model.TaskStatusDelegated
	parentTask.UpdatedAt = now
	parentTask.CompletedAt = ""
	parentTask.CompletedBy = ""
	parent.Events = append(parent.Events, model.Event{
		TS:      now,
		Actor:   actor,
		Type:    model.EventTypeTaskDelegated,
		Summary: fmt.Sprintf("Delegated task %d to child CR %d", parentTask.ID, child.ID),
		Ref:     fmt.Sprintf("task:%d", parentTask.ID),
		Meta: map[string]string{
			"child_cr":   strconv.Itoa(child.ID),
			"child_task": strconv.Itoa(childTaskID),
		},
	})
	parent.UpdatedAt = now

	if err := s.store.SaveCR(child); err != nil {
		return nil, err
	}
	if err := s.store.SaveCR(parent); err != nil {
		return nil, err
	}

	return &DelegateTaskResult{
		ParentTaskID:     parentTask.ID,
		ParentTaskStatus: parentTask.Status,
		ChildTaskID:      childTaskID,
		ChildCRID:        child.ID,
	}, nil
}

func (s *Service) UndelegateTaskFromChild(parentCRID, taskID, childCRID int) (*UndelegateTaskResult, error) {
	var result *UndelegateTaskResult
	if err := s.withMutationLock(func() error {
		var err error
		result, err = s.undelegateTaskFromChildUnlocked(parentCRID, taskID, childCRID)
		return err
	}); err != nil {
		return nil, err
	}
	return result, nil
}

func (s *Service) undelegateTaskFromChildUnlocked(parentCRID, taskID, childCRID int) (*UndelegateTaskResult, error) {
	parent, err := s.store.LoadCR(parentCRID)
	if err != nil {
		return nil, err
	}
	if guardErr := s.ensureNoMergeInProgressForCR(parent); guardErr != nil {
		return nil, guardErr
	}
	taskIndex := indexOfTask(parent.Subtasks, taskID)
	if taskIndex < 0 {
		return nil, fmt.Errorf("task %d not found in cr %d", taskID, parentCRID)
	}
	task := &parent.Subtasks[taskIndex]

	filtered := make([]model.TaskDelegation, 0, len(task.Delegations))
	removed := 0
	for _, delegation := range task.Delegations {
		if delegation.ChildCRID == childCRID {
			removed++
			continue
		}
		filtered = append(filtered, delegation)
	}
	if removed == 0 {
		return nil, fmt.Errorf("task %d in cr %d is not delegated to child cr %d", taskID, parentCRID, childCRID)
	}

	now := s.timestamp()
	actor := s.git.Actor()
	task.Delegations = filtered
	if len(task.Delegations) == 0 && task.Status == model.TaskStatusDelegated {
		task.Status = model.TaskStatusOpen
		task.CompletedAt = ""
		task.CompletedBy = ""
	}
	task.UpdatedAt = now
	parent.Events = append(parent.Events, model.Event{
		TS:      now,
		Actor:   actor,
		Type:    model.EventTypeTaskUndelegated,
		Summary: fmt.Sprintf("Removed delegation from task %d to child CR %d", taskID, childCRID),
		Ref:     fmt.Sprintf("task:%d", taskID),
		Meta: map[string]string{
			"child_cr": strconv.Itoa(childCRID),
			"removed":  strconv.Itoa(removed),
		},
	})
	parent.UpdatedAt = now
	if err := s.store.SaveCR(parent); err != nil {
		return nil, err
	}

	return &UndelegateTaskResult{
		ParentTaskID:      task.ID,
		ParentTaskStatus:  task.Status,
		RemovedDelegation: removed,
	}, nil
}

func (s *Service) StackCR(id int) (*StackView, error) {
	focus, err := s.store.LoadCR(id)
	if err != nil {
		return nil, err
	}
	crs, err := s.store.ListCRs()
	if err != nil {
		return nil, err
	}
	byID := map[int]model.CR{}
	for _, cr := range crs {
		byID[cr.ID] = cr
	}

	root := focus.ID
	for {
		current, ok := byID[root]
		if !ok || current.ParentCRID <= 0 {
			break
		}
		parent, ok := byID[current.ParentCRID]
		if !ok {
			break
		}
		root = parent.ID
	}

	children := map[int][]int{}
	for _, cr := range crs {
		if cr.ParentCRID <= 0 {
			continue
		}
		children[cr.ParentCRID] = append(children[cr.ParentCRID], cr.ID)
	}
	for parentID := range children {
		sort.Ints(children[parentID])
	}

	nodes := make([]StackNodeView, 0)
	var visit func(crID, depth int) error
	visit = func(crID, depth int) error {
		cr, ok := byID[crID]
		if !ok {
			return nil
		}
		status, err := s.StatusCR(cr.ID)
		if err != nil {
			return err
		}
		node := StackNodeView{
			ID:                    cr.ID,
			UID:                   strings.TrimSpace(cr.UID),
			ParentCRID:            cr.ParentCRID,
			Title:                 cr.Title,
			Status:                cr.Status,
			Branch:                cr.Branch,
			Depth:                 depth,
			Children:              append([]int(nil), children[cr.ID]...),
			MergeBlocked:          status.MergeBlocked,
			MergeBlockers:         append([]string(nil), status.MergeBlockers...),
			TasksTotal:            status.TasksTotal,
			TasksOpen:             status.TasksOpen,
			TasksDone:             status.TasksDone,
			TasksDelegated:        status.TasksDelegated,
			TasksDelegatedPending: status.TasksDelegatedPending,
		}
		nodes = append(nodes, node)
		for _, childID := range children[cr.ID] {
			if err := visit(childID, depth+1); err != nil {
				return err
			}
		}
		return nil
	}
	if err := visit(root, 0); err != nil {
		return nil, err
	}

	return &StackView{
		RootCRID:  root,
		FocusCRID: focus.ID,
		Nodes:     nodes,
	}, nil
}

func (s *Service) StackCurrentCR() (*StackView, error) {
	ctx, err := s.CurrentCR()
	if err != nil {
		return nil, err
	}
	return s.StackCR(ctx.CR.ID)
}

func (s *Service) mergeBlockersForCR(cr *model.CR, validation *ValidationReport) []string {
	if cr == nil {
		return []string{"unknown CR"}
	}
	blockers := make([]string, 0)
	if validation != nil {
		for _, validationErr := range validation.Errors {
			blockers = append(blockers, fmt.Sprintf("validation: %s", validationErr))
		}
	}
	if cr.ParentCRID > 0 {
		parent, err := s.store.LoadCR(cr.ParentCRID)
		switch {
		case err != nil:
			blockers = append(blockers, fmt.Sprintf("parent cr %d is missing", cr.ParentCRID))
		case parent.Status != model.StatusMerged && !childDelegatedFromParent(parent, cr.ID):
			blockers = append(blockers, fmt.Sprintf("CR %d depends on parent CR %d (%s)", cr.ID, parent.ID, parent.Status))
		}
	}
	for _, task := range cr.Subtasks {
		if len(task.Delegations) == 0 {
			continue
		}
		pending := s.pendingDelegationChildIDs(task)
		if len(pending) == 0 {
			continue
		}
		blockers = append(blockers, fmt.Sprintf("task #%d delegated to unmerged child CR(s): %s", task.ID, joinIntIDs(pending)))
	}
	return dedupeStrings(blockers)
}

func childDelegatedFromParent(parent *model.CR, childCRID int) bool {
	if parent == nil || childCRID <= 0 {
		return false
	}
	for _, task := range parent.Subtasks {
		for _, delegation := range task.Delegations {
			if delegation.ChildCRID == childCRID {
				return true
			}
		}
	}
	return false
}

func (s *Service) pendingDelegationChildIDs(task model.Subtask) []int {
	if len(task.Delegations) == 0 {
		return []int{}
	}
	pending := []int{}
	seen := map[int]struct{}{}
	for _, delegation := range task.Delegations {
		if delegation.ChildCRID <= 0 {
			continue
		}
		if _, exists := seen[delegation.ChildCRID]; exists {
			continue
		}
		seen[delegation.ChildCRID] = struct{}{}

		child, err := s.store.LoadCR(delegation.ChildCRID)
		if err != nil || child.Status != model.StatusMerged {
			pending = append(pending, delegation.ChildCRID)
		}
	}
	sort.Ints(pending)
	return pending
}

func (s *Service) syncDelegatedTasksAfterChildMerge(childCRID int) error {
	if childCRID <= 0 {
		return nil
	}
	crs, err := s.store.ListCRs()
	if err != nil {
		return err
	}
	statusByID := make(map[int]string, len(crs))
	for _, cr := range crs {
		statusByID[cr.ID] = cr.Status
	}

	now := s.timestamp()
	actor := s.git.Actor()
	for i := range crs {
		cr := crs[i]
		if cr.Status != model.StatusInProgress {
			continue
		}

		changed := false
		for taskIndex := range cr.Subtasks {
			task := &cr.Subtasks[taskIndex]
			if len(task.Delegations) == 0 {
				continue
			}
			referencesMergedChild := false
			pending := make([]int, 0)
			seen := map[int]struct{}{}
			for _, delegation := range task.Delegations {
				childID := delegation.ChildCRID
				if childID <= 0 {
					continue
				}
				if childID == childCRID {
					referencesMergedChild = true
				}
				if _, exists := seen[childID]; exists {
					continue
				}
				seen[childID] = struct{}{}
				if statusByID[childID] != model.StatusMerged {
					pending = append(pending, childID)
				}
			}
			if !referencesMergedChild || len(pending) > 0 || task.Status == model.TaskStatusDone {
				continue
			}

			task.Status = model.TaskStatusDone
			task.UpdatedAt = now
			task.CompletedAt = now
			task.CompletedBy = actor
			cr.Events = append(cr.Events, model.Event{
				TS:      now,
				Actor:   actor,
				Type:    model.EventTypeTaskDoneAuto,
				Summary: fmt.Sprintf("Auto-completed delegated task %d after child CR merge", task.ID),
				Ref:     fmt.Sprintf("task:%d", task.ID),
				Meta: map[string]string{
					"source":   "delegation_merge",
					"child_cr": strconv.Itoa(childCRID),
				},
			})
			changed = true
		}
		if !changed {
			continue
		}
		cr.UpdatedAt = now
		if err := s.store.SaveCR(&cr); err != nil {
			return err
		}
	}
	return nil
}

func joinIntIDs(ids []int) string {
	if len(ids) == 0 {
		return "-"
	}
	parts := make([]string, 0, len(ids))
	for _, id := range ids {
		parts = append(parts, strconv.Itoa(id))
	}
	return strings.Join(parts, ",")
}
