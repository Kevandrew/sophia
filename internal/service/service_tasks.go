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
	if err := s.withMutationLock(func() error {
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
	if err := s.withMutationLock(func() error {
		var err error
		changed, err = s.setTaskContractUnlocked(crID, taskID, patch)
		return err
	}); err != nil {
		return nil, err
	}
	return changed, nil
}

func (s *Service) setTaskContractUnlocked(crID, taskID int, patch TaskContractPatch) ([]string, error) {
	taskStore := s.activeTaskStoreProvider()
	taskGit := s.activeTaskGitProvider()
	taskMergeGuard := s.activeTaskMergeGuard()

	cr, err := taskStore.LoadCR(crID)
	if err != nil {
		return nil, err
	}
	if guardErr := taskMergeGuard(cr); guardErr != nil {
		return nil, guardErr
	}
	policy, err := s.repoPolicy()
	if err != nil {
		return nil, err
	}
	taskIndex := indexOfTask(cr.Subtasks, taskID)
	if taskIndex < 0 {
		return nil, fmt.Errorf("task %d not found in cr %d", taskID, crID)
	}
	task := &cr.Subtasks[taskIndex]
	beforeContract := servicetasks.CloneTaskContract(task.Contract)

	changed, err := s.applyTaskContractPatch(taskID, &task.Contract, patch, policy)
	if err != nil {
		return nil, err
	}
	if len(changed) == 0 {
		return nil, ErrNoCRChanges
	}

	now := s.timestamp()
	actor := taskGit.Actor()
	changeReason := ""
	if patch.ChangeReason != nil {
		changeReason = strings.TrimSpace(*patch.ChangeReason)
	}

	drift := servicetasks.ApplyTaskContractTransition(task, beforeContract, now, actor, changeReason, pathMatchesScopePrefix)
	cr.UpdatedAt = now
	meta := map[string]string{
		"fields": strings.Join(changed, ","),
	}
	if changeReason != "" {
		meta["change_reason"] = changeReason
	}
	cr.Events = append(cr.Events, model.Event{
		TS:      now,
		Actor:   actor,
		Type:    model.EventTypeTaskContractUpdated,
		Summary: fmt.Sprintf("Updated task %d contract fields: %s", taskID, strings.Join(changed, ",")),
		Ref:     fmt.Sprintf("task:%d", taskID),
		Meta:    meta,
	})
	if drift != nil {
		driftMeta := map[string]string{
			"drift_id":          strconv.Itoa(drift.ID),
			"fields":            strings.Join(drift.Fields, ","),
			"checkpoint_commit": drift.CheckpointCommit,
			"acknowledged":      strconv.FormatBool(drift.Acknowledged),
		}
		if drift.Reason != "" {
			driftMeta["reason"] = drift.Reason
		}
		cr.Events = append(cr.Events, model.Event{
			TS:      now,
			Actor:   actor,
			Type:    model.EventTypeTaskContractDriftRecorded,
			Summary: fmt.Sprintf("Recorded task %d contract drift #%d (%s)", taskID, drift.ID, strings.Join(drift.Fields, ",")),
			Ref:     fmt.Sprintf("task:%d", taskID),
			Meta:    driftMeta,
		})
	}
	if err := taskStore.SaveCR(cr); err != nil {
		return nil, err
	}
	return changed, nil
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
	if err := s.withMutationLock(func() error {
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
	_, chunks, _, err := s.loadWorkingTreeTaskChunks(crID, taskID, paths)
	if err != nil {
		return nil, err
	}
	return chunks, nil
}

func (s *Service) TaskChunkWorkingTreePatch(crID, taskID int, chunkID string, paths []string) (TaskChunk, string, error) {
	chunkID = strings.TrimSpace(chunkID)
	if chunkID == "" {
		return TaskChunk{}, "", fmt.Errorf("chunk id is required")
	}
	files, _, _, err := s.loadWorkingTreeTaskChunks(crID, taskID, paths)
	if err != nil {
		return TaskChunk{}, "", err
	}
	patch, selected, err := buildPatchFromSelectedChunkIDs(files, []string{chunkID})
	if err != nil {
		return TaskChunk{}, "", err
	}
	if len(selected) != 1 {
		return TaskChunk{}, "", fmt.Errorf("chunk %q not found", chunkID)
	}
	chunk := selected[0]
	return TaskChunk{
		ID:       chunk.ID,
		Path:     chunk.Path,
		OldStart: chunk.OldStart,
		OldLines: chunk.OldLines,
		NewStart: chunk.NewStart,
		NewLines: chunk.NewLines,
		Preview:  chunk.Preview,
	}, patch, nil
}

func (s *Service) ExportTaskChunkWorkingTreePatch(crID, taskID int, chunkIDs, paths []string) ([]TaskChunk, string, error) {
	files, _, _, err := s.loadWorkingTreeTaskChunks(crID, taskID, paths)
	if err != nil {
		return nil, "", err
	}
	patch, selected, err := buildPatchFromSelectedChunkIDs(files, chunkIDs)
	if err != nil {
		return nil, "", err
	}
	out := make([]TaskChunk, 0, len(selected))
	for _, chunk := range selected {
		out = append(out, TaskChunk{
			ID:       chunk.ID,
			Path:     chunk.Path,
			OldStart: chunk.OldStart,
			OldLines: chunk.OldLines,
			NewStart: chunk.NewStart,
			NewLines: chunk.NewLines,
			Preview:  chunk.Preview,
		})
	}
	return out, patch, nil
}

func (s *Service) loadWorkingTreeTaskChunks(crID, taskID int, paths []string) ([]parsedPatchFile, []TaskChunk, string, error) {
	taskStore := s.activeTaskStoreProvider()
	taskGit := s.activeTaskGitProvider()
	taskMergeGuard := s.activeTaskMergeGuard()

	cr, err := taskStore.LoadCR(crID)
	if err != nil {
		return nil, nil, "", err
	}
	if guardErr := taskMergeGuard(cr); guardErr != nil {
		return nil, nil, "", guardErr
	}
	if cr.Status != model.StatusInProgress {
		return nil, nil, "", fmt.Errorf("cr %d is not in progress", crID)
	}
	if indexOfTask(cr.Subtasks, taskID) < 0 {
		return nil, nil, "", fmt.Errorf("task %d not found in cr %d", taskID, crID)
	}
	currentBranch, branchErr := taskGit.CurrentBranch()
	if branchErr != nil {
		return nil, nil, "", branchErr
	}
	if currentBranch != cr.Branch {
		return nil, nil, "", fmt.Errorf("task chunk commands requires active CR branch %q, current branch is %q; run `sophia cr switch %d`", cr.Branch, currentBranch, cr.ID)
	}
	hasStaged, stagedErr := taskGit.HasStagedChanges()
	if stagedErr != nil {
		return nil, nil, "", stagedErr
	}
	if hasStaged {
		return nil, nil, "", fmt.Errorf("%w: unstage changes before running task chunk commands", ErrPreStagedChanges)
	}
	normalizedPaths := []string{}
	if len(paths) > 0 {
		normalized, normalizeErr := s.normalizeTaskScopePaths(paths)
		if normalizeErr != nil {
			return nil, nil, "", normalizeErr
		}
		normalizedPaths = normalized
	}
	diff, err := taskGit.WorkingTreeUnifiedDiff(normalizedPaths, 0)
	if err != nil {
		return nil, nil, "", err
	}
	files, err := parsePatchFiles(diff)
	if err != nil {
		return nil, nil, "", fmt.Errorf("parse working tree diff chunks: %w", err)
	}
	chunks := make([]TaskChunk, 0)
	for _, file := range files {
		for _, chunk := range file.Hunks {
			chunks = append(chunks, TaskChunk{
				ID:       chunk.ID,
				Path:     chunk.Path,
				OldStart: chunk.OldStart,
				OldLines: chunk.OldLines,
				NewStart: chunk.NewStart,
				NewLines: chunk.NewLines,
				Preview:  chunk.Preview,
			})
		}
	}
	sort.Slice(chunks, func(i, j int) bool {
		if chunks[i].Path != chunks[j].Path {
			return chunks[i].Path < chunks[j].Path
		}
		if chunks[i].OldStart != chunks[j].OldStart {
			return chunks[i].OldStart < chunks[j].OldStart
		}
		if chunks[i].NewStart != chunks[j].NewStart {
			return chunks[i].NewStart < chunks[j].NewStart
		}
		return chunks[i].ID < chunks[j].ID
	})
	return files, chunks, diff, nil
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
	if err := s.withMutationLock(func() error {
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
	taskStore := s.activeTaskStoreProvider()
	taskGit := s.activeTaskGitProvider()
	taskMergeGuard := s.activeTaskMergeGuard()

	cr, err := taskStore.LoadCR(crID)
	if err != nil {
		return model.Subtask{}, err
	}
	if guardErr := taskMergeGuard(cr); guardErr != nil {
		return model.Subtask{}, guardErr
	}
	if cr.Status != model.StatusInProgress {
		return model.Subtask{}, fmt.Errorf("cr %d is not in progress", crID)
	}
	taskIndex := indexOfTask(cr.Subtasks, taskID)
	if taskIndex < 0 {
		return model.Subtask{}, fmt.Errorf("task %d not found in cr %d", taskID, crID)
	}
	task := &cr.Subtasks[taskIndex]
	if task.Status != model.TaskStatusDone {
		return model.Subtask{}, fmt.Errorf("%w: task %d in cr %d has status %q", ErrTaskNotDone, taskID, crID, task.Status)
	}

	now := s.timestamp()
	actor := taskGit.Actor()
	previousCheckpoint := strings.TrimSpace(task.CheckpointCommit)
	servicetasks.MarkTaskReopened(task, now, opts.ClearCheckpoint)

	meta := map[string]string{
		"checkpoint_cleared": strconv.FormatBool(opts.ClearCheckpoint),
	}
	if previousCheckpoint != "" {
		meta["previous_checkpoint_commit"] = previousCheckpoint
	}
	cr.Events = append(cr.Events, model.Event{
		TS:      now,
		Actor:   actor,
		Type:    model.EventTypeTaskReopened,
		Summary: task.Title,
		Ref:     fmt.Sprintf("task:%d", taskID),
		Meta:    meta,
	})
	cr.UpdatedAt = now
	if err := taskStore.SaveCR(cr); err != nil {
		return model.Subtask{}, err
	}
	return cr.Subtasks[taskIndex], nil
}

func (s *Service) DoneTaskWithCheckpoint(crID, taskID int, opts DoneTaskOptions) (string, error) {
	commitSHA := ""
	if err := s.withMutationLock(func() error {
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
	taskStore := s.activeTaskStoreProvider()
	taskGit := s.activeTaskGitProvider()

	cr, err := taskStore.LoadCR(crID)
	if err != nil {
		return "", err
	}
	if guardErr := s.activeTaskMergeGuard()(cr); guardErr != nil {
		return "", guardErr
	}
	if _, err := ensureCRUID(cr); err != nil {
		return "", err
	}
	if _, err := s.ensureCRBaseFields(cr, false); err != nil {
		return "", err
	}
	policy, err := s.repoPolicy()
	if err != nil {
		return "", err
	}
	if cr.Status != model.StatusInProgress {
		return "", fmt.Errorf("cr %d is not in progress", crID)
	}
	if err := validateDoneTaskOptions(opts); err != nil {
		return "", err
	}

	now := s.timestamp()
	actor := taskGit.Actor()
	taskIndex := indexOfTask(cr.Subtasks, taskID)
	if taskIndex < 0 {
		return "", fmt.Errorf("task %d not found in cr %d", taskID, crID)
	}
	task := &cr.Subtasks[taskIndex]
	if err := ensureTaskReadyForDone(task, taskID, crID, policy); err != nil {
		return "", err
	}
	if opts.DryRun {
		if err := s.previewTaskCheckpointForDone(taskGit, cr, taskIndex, taskID, opts); err != nil {
			return "", err
		}
		return "", nil
	}

	commitSHA, err := s.applyTaskCheckpointForDone(taskGit, cr, task, taskIndex, taskID, now, actor, opts)
	if err != nil {
		return "", err
	}

	servicetasks.MarkTaskDone(task, now, actor)

	taskDoneMeta := map[string]string{
		"checkpoint": strconv.FormatBool(opts.Checkpoint),
	}
	if opts.Checkpoint {
		taskDoneMeta["checkpoint_commit"] = commitSHA
		taskDoneMeta["checkpoint_source"] = model.TaskCheckpointSourceTaskCheckpoint
	} else {
		taskDoneMeta["checkpoint_source"] = model.TaskCheckpointSourceTaskNoCheckpoint
		taskDoneMeta["no_checkpoint_reason"] = strings.TrimSpace(opts.NoCheckpointReason)
	}
	cr.Events = append(cr.Events, model.Event{
		TS:      now,
		Actor:   actor,
		Type:    model.EventTypeTaskDone,
		Summary: task.Title,
		Ref:     fmt.Sprintf("task:%d", taskID),
		Meta:    taskDoneMeta,
	})
	cr.UpdatedAt = now

	if err := taskStore.SaveCR(cr); err != nil {
		return "", err
	}
	return commitSHA, nil
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
	if !opts.Checkpoint {
		return nil
	}

	currentBranch, branchErr := gitProvider.CurrentBranch()
	if branchErr != nil {
		return branchErr
	}
	if currentBranch != cr.Branch {
		return fmt.Errorf("checkpoint requires active CR branch %q, current branch is %q; run `sophia cr switch %d`", cr.Branch, currentBranch, cr.ID)
	}

	preStaged, stagedErr := gitProvider.HasStagedChanges()
	if stagedErr != nil {
		return stagedErr
	}
	if preStaged {
		return fmt.Errorf("%w: unstage changes before running task checkpoint", ErrPreStagedChanges)
	}

	if opts.StageAll {
		dirty, dirtyErr := s.hasTaskWorkingTreeChanges(gitProvider)
		if dirtyErr != nil {
			return dirtyErr
		}
		if !dirty {
			return fmt.Errorf("%w: task %d has no working tree changes", ErrNoTaskChanges, taskID)
		}
		return nil
	}

	if opts.FromContract {
		normalizedScope, normalizeErr := s.normalizeContractScopePrefixes(cr.Subtasks[taskIndex].Contract.Scope)
		if normalizeErr != nil {
			return normalizeErr
		}
		if _, resolveErr := s.resolveTaskCheckpointPathsFromContract(gitProvider, normalizedScope); resolveErr != nil {
			return resolveErr
		}
		return nil
	}

	if strings.TrimSpace(opts.PatchFile) != "" {
		patchPath, pathErr := s.normalizePatchFilePath(opts.PatchFile)
		if pathErr != nil {
			return pathErr
		}
		patchContent, readErr := readPatchManifestContent(patchPath)
		if readErr != nil {
			return fmt.Errorf("read patch file %q: %w", patchFileDisplayName(opts.PatchFile), readErr)
		}
		parsedChunks, parseErr := parsePatchChunks(patchContent)
		if parseErr != nil {
			return fmt.Errorf("%w: parse patch file: %v", ErrInvalidTaskScope, parseErr)
		}
		if len(parsedChunks) == 0 {
			return fmt.Errorf("%w: patch file %q contains no hunks", ErrNoTaskChanges, patchFileDisplayName(opts.PatchFile))
		}
		return nil
	}

	normalizedPaths, normalizeErr := s.normalizeTaskScopePaths(opts.Paths)
	if normalizeErr != nil {
		return normalizeErr
	}
	changedPathFound := false
	for _, scopePath := range normalizedPaths {
		hasChanges, hasErr := gitProvider.PathHasChanges(scopePath)
		if hasErr != nil {
			return hasErr
		}
		if hasChanges {
			changedPathFound = true
			break
		}
	}
	if !changedPathFound {
		return fmt.Errorf("%w: none of the scoped paths have changes", ErrNoTaskChanges)
	}
	return nil
}

func (s *Service) applyTaskCheckpointForDone(gitProvider taskLifecycleGitProvider, cr *model.CR, task *model.Subtask, taskIndex, taskID int, now, actor string, opts DoneTaskOptions) (string, error) {
	if !opts.Checkpoint {
		servicetasks.ApplyNoCheckpointState(task, now, strings.TrimSpace(opts.NoCheckpointReason))
		return "", nil
	}

	currentBranch, branchErr := gitProvider.CurrentBranch()
	if branchErr != nil {
		return "", branchErr
	}
	if currentBranch != cr.Branch {
		return "", fmt.Errorf("checkpoint requires active CR branch %q, current branch is %q; run `sophia cr switch %d`", cr.Branch, currentBranch, cr.ID)
	}

	preStaged, stagedErr := gitProvider.HasStagedChanges()
	if stagedErr != nil {
		return "", stagedErr
	}
	if preStaged {
		return "", fmt.Errorf("%w: unstage changes before running task checkpoint", ErrPreStagedChanges)
	}

	checkpointScope, checkpointChunks, scopeMode, stageErr := s.stageTaskCheckpointForDone(gitProvider, cr, taskIndex, taskID, opts)
	if stageErr != nil {
		return "", stageErr
	}

	hasStaged, stagedErr := gitProvider.HasStagedChanges()
	if stagedErr != nil {
		return "", stagedErr
	}
	if !hasStaged {
		return "", fmt.Errorf("%w: no staged changes after applying scope", ErrNoTaskChanges)
	}

	commitMessage := buildTaskCheckpointMessage(cr, task, scopeMode, len(checkpointChunks))
	if err := gitProvider.Commit(commitMessage); err != nil {
		return "", err
	}
	sha, shaErr := gitProvider.HeadShortSHA()
	if shaErr != nil {
		return "", shaErr
	}

	servicetasks.ApplyCheckpointState(task, now, sha, commitMessage, checkpointScope, checkpointChunks)
	if servicetasks.TaskContractBaselineIsEmpty(task.ContractBaseline) {
		task.ContractBaseline = servicetasks.TaskContractBaselineFromContract(task.Contract, now, actor)
	}

	meta := map[string]string{
		"commit":  sha,
		"message": strings.SplitN(commitMessage, "\n", 2)[0],
		"scope":   strings.Join(checkpointScope, ","),
	}
	if scopeMode == model.TaskScopeModeTaskContract {
		meta["scope_source"] = model.TaskScopeModeTaskContract
	}
	if scopeMode == model.TaskScopeModePatchManifest {
		meta["scope_source"] = model.TaskScopeModePatchManifest
		meta["chunk_count"] = strconv.Itoa(len(checkpointChunks))
	}
	cr.Events = append(cr.Events, model.Event{
		TS:      now,
		Actor:   actor,
		Type:    model.EventTypeTaskCheckpointed,
		Summary: fmt.Sprintf("Checkpointed task %d as %s", taskID, sha),
		Ref:     fmt.Sprintf("task:%d", taskID),
		Meta:    meta,
	})

	return sha, nil
}

func (s *Service) stageTaskCheckpointForDone(gitProvider taskLifecycleGitProvider, cr *model.CR, taskIndex, taskID int, opts DoneTaskOptions) ([]string, []model.CheckpointChunk, string, error) {
	if opts.StageAll {
		dirty, dirtyErr := s.hasTaskWorkingTreeChanges(gitProvider)
		if dirtyErr != nil {
			return nil, nil, "", dirtyErr
		}
		if !dirty {
			return nil, nil, "", fmt.Errorf("%w: task %d has no working tree changes", ErrNoTaskChanges, taskID)
		}
		if err := gitProvider.StageAll(); err != nil {
			return nil, nil, "", err
		}
		return []string{"*"}, nil, model.TaskScopeModeAll, nil
	}

	if opts.FromContract {
		normalizedScope, normalizeErr := s.normalizeContractScopePrefixes(cr.Subtasks[taskIndex].Contract.Scope)
		if normalizeErr != nil {
			return nil, nil, "", normalizeErr
		}
		paths, resolveErr := s.resolveTaskCheckpointPathsFromContract(gitProvider, normalizedScope)
		if resolveErr != nil {
			return nil, nil, "", resolveErr
		}
		if err := gitProvider.StagePaths(paths); err != nil {
			return nil, nil, "", err
		}
		return paths, nil, model.TaskScopeModeTaskContract, nil
	}

	if strings.TrimSpace(opts.PatchFile) != "" {
		patchPath, pathErr := s.normalizePatchFilePath(opts.PatchFile)
		if pathErr != nil {
			return nil, nil, "", pathErr
		}
		patchContent, readErr := readPatchManifestContent(patchPath)
		if readErr != nil {
			return nil, nil, "", fmt.Errorf("read patch file %q: %w", patchFileDisplayName(opts.PatchFile), readErr)
		}
		parsedChunks, parseErr := parsePatchChunks(patchContent)
		if parseErr != nil {
			return nil, nil, "", fmt.Errorf("%w: parse patch file: %v", ErrInvalidTaskScope, parseErr)
		}
		if len(parsedChunks) == 0 {
			return nil, nil, "", fmt.Errorf("%w: patch file %q contains no hunks", ErrNoTaskChanges, patchFileDisplayName(opts.PatchFile))
		}
		if err := gitProvider.ApplyPatchToIndex(patchPath); err != nil {
			return nil, nil, "", err
		}
		checkpointChunks := make([]model.CheckpointChunk, 0, len(parsedChunks))
		for _, chunk := range parsedChunks {
			checkpointChunks = append(checkpointChunks, model.CheckpointChunk{
				ID:       chunk.ID,
				Path:     chunk.Path,
				OldStart: chunk.OldStart,
				OldLines: chunk.OldLines,
				NewStart: chunk.NewStart,
				NewLines: chunk.NewLines,
			})
		}
		return checkpointChunkPaths(parsedChunks), checkpointChunks, model.TaskScopeModePatchManifest, nil
	}

	normalizedPaths, normalizeErr := s.normalizeTaskScopePaths(opts.Paths)
	if normalizeErr != nil {
		return nil, nil, "", normalizeErr
	}

	changedPathFound := false
	for _, scopePath := range normalizedPaths {
		hasChanges, hasErr := gitProvider.PathHasChanges(scopePath)
		if hasErr != nil {
			return nil, nil, "", hasErr
		}
		if hasChanges {
			changedPathFound = true
			break
		}
	}
	if !changedPathFound {
		return nil, nil, "", fmt.Errorf("%w: none of the scoped paths have changes", ErrNoTaskChanges)
	}
	if err := gitProvider.StagePaths(normalizedPaths); err != nil {
		return nil, nil, "", err
	}
	return normalizedPaths, nil, model.TaskScopeModePath, nil
}

func (s *Service) hasTaskWorkingTreeChanges(gitProvider taskLifecycleGitProvider) (bool, error) {
	entries, err := gitProvider.WorkingTreeStatus()
	if err != nil {
		return false, err
	}
	for _, entry := range entries {
		if s.isIgnorableWorktreeEntry(entry) {
			continue
		}
		return true, nil
	}
	return false, nil
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
