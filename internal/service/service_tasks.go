package service

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sophia/internal/model"
	servicetasks "sophia/internal/service/tasks"
	"sort"
	"strconv"
	"strings"
)

func (s *Service) AddTask(crID int, title string) (*model.Subtask, error) {
	if strings.TrimSpace(title) == "" {
		return nil, errors.New("task title cannot be empty")
	}
	var added model.Subtask
	if err := s.withTaskMutationLock(func() error {
		cr, err := s.loadCRForMutation(crID)
		if err != nil {
			return err
		}
		newTaskID := nextTaskID(cr.Subtasks)
		now := s.timestamp()
		actor := s.git.Actor()
		task := model.Subtask{
			ID:        newTaskID,
			Title:     title,
			Status:    model.TaskStatusOpen,
			CreatedAt: now,
			UpdatedAt: now,
			CreatedBy: actor,
		}
		cr.Subtasks = append(cr.Subtasks, task)
		if err := s.appendCRMutationEventAndSave(cr, model.Event{
			TS:      now,
			Actor:   actor,
			Type:    model.EventTypeTaskAdded,
			Summary: title,
			Ref:     fmt.Sprintf("task:%d", newTaskID),
		}); err != nil {
			return err
		}
		added = task
		return nil
	}); err != nil {
		return nil, err
	}
	return &added, nil
}

func validateTaskAcceptanceCheckKeys(taskID int, checks []string, policy *model.RepoPolicy) error {
	allowed := servicetasks.TaskAcceptanceCheckPolicyMap(policy)
	if len(allowed) == 0 && len(checks) > 0 {
		return fmt.Errorf("%w: task %d acceptance_checks configured but trust.checks.definitions is empty", ErrPolicyViolation, taskID)
	}
	for _, key := range checks {
		if _, ok := allowed[key]; ok {
			continue
		}
		return fmt.Errorf("%w: task %d acceptance_checks key %q not found in trust.checks.definitions", ErrPolicyViolation, taskID, key)
	}
	return nil
}

func (s *Service) SetTaskContract(crID, taskID int, patch TaskContractPatch) ([]string, error) {
	var changed []string
	if err := s.withTaskMutationLock(func() error {
		var err error
		changed, err = s.setTaskContractUnlocked(crID, taskID, patch)
		return err
	}); err != nil {
		return nil, err
	}
	return changed, nil
}

func (s *Service) setTaskContractUnlocked(crID, taskID int, patch TaskContractPatch) ([]string, error) {
	return s.taskLifecycleDomainService().setTaskContractUnlocked(crID, taskID, patch)
}

func (s *Service) GetTaskContract(crID, taskID int) (*model.TaskContract, error) {
	cr, err := s.store.LoadCR(crID)
	if err != nil {
		return nil, err
	}
	taskIndex := indexOfTask(cr.Subtasks, taskID)
	if taskIndex < 0 {
		return nil, fmt.Errorf("task %d not found in cr %d", taskID, crID)
	}
	contract := cr.Subtasks[taskIndex].Contract
	contract.AcceptanceCriteria = append([]string(nil), contract.AcceptanceCriteria...)
	contract.Scope = append([]string(nil), contract.Scope...)
	contract.AcceptanceChecks = append([]string(nil), contract.AcceptanceChecks...)
	return &contract, nil
}

func (s *Service) ListTaskContractDrifts(crID, taskID int) ([]model.TaskContractDrift, error) {
	cr, err := s.store.LoadCR(crID)
	if err != nil {
		return nil, err
	}
	taskIndex := indexOfTask(cr.Subtasks, taskID)
	if taskIndex < 0 {
		return nil, fmt.Errorf("task %d not found in cr %d", taskID, crID)
	}
	drifts := append([]model.TaskContractDrift(nil), cr.Subtasks[taskIndex].ContractDrifts...)
	sort.Slice(drifts, func(i, j int) bool {
		if drifts[i].ID == drifts[j].ID {
			return drifts[i].TS < drifts[j].TS
		}
		return drifts[i].ID < drifts[j].ID
	})
	return drifts, nil
}

func (s *Service) AckTaskContractDrift(crID, taskID, driftID int, reason string) (*model.TaskContractDrift, error) {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return nil, fmt.Errorf("ack reason is required")
	}
	var ack model.TaskContractDrift
	if err := s.withTaskMutationLock(func() error {
		cr, err := s.loadCRForMutation(crID)
		if err != nil {
			return err
		}
		taskIndex := indexOfTask(cr.Subtasks, taskID)
		if taskIndex < 0 {
			return fmt.Errorf("task %d not found in cr %d", taskID, crID)
		}
		task := &cr.Subtasks[taskIndex]
		driftIndex := indexOfDrift(task.ContractDrifts, driftID)
		if driftIndex < 0 {
			return fmt.Errorf("task %d drift %d not found in cr %d", taskID, driftID, crID)
		}

		now := s.timestamp()
		actor := s.git.Actor()
		task.ContractDrifts[driftIndex].Acknowledged = true
		task.ContractDrifts[driftIndex].AcknowledgedAt = now
		task.ContractDrifts[driftIndex].AcknowledgedBy = actor
		task.ContractDrifts[driftIndex].AckReason = reason
		task.UpdatedAt = now
		if err := s.appendCRMutationEventAndSave(cr, model.Event{
			TS:      now,
			Actor:   actor,
			Type:    model.EventTypeTaskContractDriftAcknowledged,
			Summary: fmt.Sprintf("Acknowledged task %d contract drift #%d", taskID, driftID),
			Ref:     fmt.Sprintf("task:%d", taskID),
			Meta: map[string]string{
				"drift_id": strconv.Itoa(driftID),
				"reason":   reason,
			},
		}); err != nil {
			return err
		}
		ack = task.ContractDrifts[driftIndex]
		return nil
	}); err != nil {
		return nil, err
	}
	ack.Fields = append([]string(nil), ack.Fields...)
	ack.BeforeScope = append([]string(nil), ack.BeforeScope...)
	ack.AfterScope = append([]string(nil), ack.AfterScope...)
	ack.BeforeAcceptanceChecks = append([]string(nil), ack.BeforeAcceptanceChecks...)
	ack.AfterAcceptanceChecks = append([]string(nil), ack.AfterAcceptanceChecks...)
	return &ack, nil
}

func (s *Service) ListTasks(crID int) ([]model.Subtask, error) {
	cr, err := s.store.LoadCR(crID)
	if err != nil {
		return nil, err
	}
	tasks := append([]model.Subtask(nil), cr.Subtasks...)
	sort.Slice(tasks, func(i, j int) bool {
		return tasks[i].ID < tasks[j].ID
	})
	return tasks, nil
}

func (s *Service) ListTaskChunks(crID, taskID int, paths []string) ([]TaskChunk, error) {
	return s.taskLifecycleDomainService().listTaskChunks(crID, taskID, paths)
}

func (s *Service) TaskChunkWorkingTreePatch(crID, taskID int, chunkID string, paths []string) (TaskChunk, string, error) {
	return s.taskLifecycleDomainService().taskChunkWorkingTreePatch(crID, taskID, chunkID, paths)
}

func (s *Service) ExportTaskChunkWorkingTreePatch(crID, taskID int, chunkIDs, paths []string) ([]TaskChunk, string, error) {
	return s.taskLifecycleDomainService().exportTaskChunkWorkingTreePatch(crID, taskID, chunkIDs, paths)
}

func (s *Service) loadWorkingTreeTaskChunks(crID, taskID int, paths []string) ([]parsedPatchFile, []TaskChunk, string, error) {
	return s.taskLifecycleDomainService().loadWorkingTreeTaskChunks(crID, taskID, paths)
}

func (s *Service) DoneTask(crID, taskID int) error {
	_, err := s.DoneTaskWithCheckpoint(crID, taskID, DoneTaskOptions{
		Checkpoint:         false,
		NoCheckpointReason: "task marked done without checkpoint via service API",
	})
	return err
}

// Reopen preserves checkpoint evidence by default unless the caller explicitly clears it.
func (s *Service) ReopenTask(crID, taskID int, opts ReopenTaskOptions) (*model.Subtask, error) {
	var reopened model.Subtask
	if err := s.withTaskMutationLock(func() error {
		var err error
		reopened, err = s.reopenTaskUnlocked(crID, taskID, opts)
		if err != nil {
			return err
		}
		return nil
	}); err != nil {
		return nil, err
	}
	return &reopened, nil
}

func (s *Service) reopenTaskUnlocked(crID, taskID int, opts ReopenTaskOptions) (model.Subtask, error) {
	return s.taskLifecycleDomainService().reopenTaskUnlocked(crID, taskID, opts)
}

func (s *Service) DoneTaskWithCheckpoint(crID, taskID int, opts DoneTaskOptions) (string, error) {
	commitSHA := ""
	if err := s.withTaskMutationLock(func() error {
		sha, doneErr := s.doneTaskWithCheckpointUnlocked(crID, taskID, opts)
		if doneErr != nil {
			return doneErr
		}
		commitSHA = sha
		return nil
	}); err != nil {
		return "", err
	}
	return commitSHA, nil
}

func (s *Service) doneTaskWithCheckpointUnlocked(crID, taskID int, opts DoneTaskOptions) (string, error) {
	return s.taskLifecycleDomainService().doneTaskWithCheckpointUnlocked(crID, taskID, opts)
}

func ensureTaskReadyForDone(task *model.Subtask, taskID, crID int, policy *model.RepoPolicy) error {
	if task == nil {
		return fmt.Errorf("task %d not found in cr %d", taskID, crID)
	}
	if task.Status == model.TaskStatusDelegated {
		return fmt.Errorf("%w: task %d in cr %d is delegated to child CRs", ErrTaskDelegated, taskID, crID)
	}
	missingContractFields := missingTaskContractFields(task.Contract, policy.TaskContract.RequiredFields)
	if len(missingContractFields) > 0 {
		return fmt.Errorf("%w: task %d missing %s", ErrTaskContractIncomplete, taskID, strings.Join(missingContractFields, ","))
	}
	return nil
}

func (s *Service) previewTaskCheckpointForDone(gitProvider taskLifecycleGitProvider, cr *model.CR, taskIndex, taskID int, opts DoneTaskOptions) error {
	return s.taskLifecycleDomainService().previewTaskCheckpointForDone(gitProvider, cr, taskIndex, taskID, opts)
}

func (s *Service) applyTaskCheckpointForDone(gitProvider taskLifecycleGitProvider, cr *model.CR, task *model.Subtask, taskIndex, taskID int, now, actor string, opts DoneTaskOptions) (string, error) {
	return s.taskLifecycleDomainService().applyTaskCheckpointForDone(gitProvider, cr, task, taskIndex, taskID, now, actor, opts)
}

func (s *Service) stageTaskCheckpointForDone(gitProvider taskLifecycleGitProvider, cr *model.CR, taskIndex, taskID int, opts DoneTaskOptions) ([]string, []model.CheckpointChunk, string, error) {
	return s.taskLifecycleDomainService().stageTaskCheckpointForDone(gitProvider, cr, taskIndex, taskID, opts)
}

func (s *Service) hasTaskWorkingTreeChanges(gitProvider taskLifecycleGitProvider) (bool, error) {
	return s.taskLifecycleDomainService().hasTaskWorkingTreeChanges(gitProvider)
}

const maxPatchManifestBytes = 8 * 1024 * 1024

func readPatchManifestContent(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	data, err := io.ReadAll(io.LimitReader(file, maxPatchManifestBytes+1))
	if err != nil {
		return "", err
	}
	if len(data) > maxPatchManifestBytes {
		return "", fmt.Errorf("patch manifest exceeds %d bytes", maxPatchManifestBytes)
	}
	return string(data), nil
}

func patchFileDisplayName(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "(patch-file)"
	}
	return filepath.Base(trimmed)
}
