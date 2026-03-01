package service

import (
	"fmt"
	"sophia/internal/model"
	servicetasks "sophia/internal/service/tasks"
	"sort"
	"strconv"
	"strings"
)

type taskLifecycleDomain struct {
	svc *Service
}

func newTaskLifecycleDomain(svc *Service) *taskLifecycleDomain {
	domain := &taskLifecycleDomain{}
	domain.bind(svc)
	return domain
}

func (d *taskLifecycleDomain) bind(svc *Service) {
	d.svc = svc
}

func (s *Service) taskLifecycleDomainService() *taskLifecycleDomain {
	if s == nil {
		return newTaskLifecycleDomain(nil)
	}
	if s.taskLifecycleSvc == nil {
		s.taskLifecycleSvc = newTaskLifecycleDomain(s)
	} else {
		s.taskLifecycleSvc.bind(s)
	}
	return s.taskLifecycleSvc
}

func (d *taskLifecycleDomain) setTaskContractUnlocked(crID, taskID int, patch TaskContractPatch) ([]string, error) {
	if d == nil || d.svc == nil {
		return nil, fmt.Errorf("task lifecycle domain is not initialized")
	}
	taskStore := d.svc.activeTaskStoreProvider()
	taskGit := d.svc.activeTaskGitProvider()
	taskMergeGuard := d.svc.activeTaskMergeGuard()

	cr, err := taskStore.LoadCR(crID)
	if err != nil {
		return nil, err
	}
	if guardErr := taskMergeGuard(cr); guardErr != nil {
		return nil, guardErr
	}
	policy, err := d.svc.repoPolicy()
	if err != nil {
		return nil, err
	}
	taskIndex := indexOfTask(cr.Subtasks, taskID)
	if taskIndex < 0 {
		return nil, fmt.Errorf("task %d not found in cr %d", taskID, crID)
	}
	task := &cr.Subtasks[taskIndex]
	beforeContract := servicetasks.CloneTaskContract(task.Contract)

	changed, err := d.svc.applyTaskContractPatch(taskID, &task.Contract, patch, policy)
	if err != nil {
		return nil, err
	}
	if len(changed) == 0 {
		return nil, ErrNoCRChanges
	}

	now := d.svc.timestamp()
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

func (d *taskLifecycleDomain) listTaskChunks(crID, taskID int, paths []string) ([]TaskChunk, error) {
	_, chunks, _, err := d.loadWorkingTreeTaskChunks(crID, taskID, paths)
	if err != nil {
		return nil, err
	}
	return chunks, nil
}

func (d *taskLifecycleDomain) taskChunkWorkingTreePatch(crID, taskID int, chunkID string, paths []string) (TaskChunk, string, error) {
	chunkID = strings.TrimSpace(chunkID)
	if chunkID == "" {
		return TaskChunk{}, "", fmt.Errorf("chunk id is required")
	}
	files, _, _, err := d.loadWorkingTreeTaskChunks(crID, taskID, paths)
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

func (d *taskLifecycleDomain) exportTaskChunkWorkingTreePatch(crID, taskID int, chunkIDs, paths []string) ([]TaskChunk, string, error) {
	files, _, _, err := d.loadWorkingTreeTaskChunks(crID, taskID, paths)
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

func (d *taskLifecycleDomain) loadWorkingTreeTaskChunks(crID, taskID int, paths []string) ([]parsedPatchFile, []TaskChunk, string, error) {
	if d == nil || d.svc == nil {
		return nil, nil, "", fmt.Errorf("task lifecycle domain is not initialized")
	}
	taskStore := d.svc.activeTaskStoreProvider()
	taskGit := d.svc.activeTaskGitProvider()
	taskMergeGuard := d.svc.activeTaskMergeGuard()

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
		normalized, normalizeErr := d.svc.normalizeTaskScopePaths(paths)
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

func (d *taskLifecycleDomain) reopenTaskUnlocked(crID, taskID int, opts ReopenTaskOptions) (model.Subtask, error) {
	if d == nil || d.svc == nil {
		return model.Subtask{}, fmt.Errorf("task lifecycle domain is not initialized")
	}
	taskStore := d.svc.activeTaskStoreProvider()
	taskGit := d.svc.activeTaskGitProvider()
	taskMergeGuard := d.svc.activeTaskMergeGuard()

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

	now := d.svc.timestamp()
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

func (d *taskLifecycleDomain) doneTaskWithCheckpointUnlocked(crID, taskID int, opts DoneTaskOptions) (string, error) {
	if d == nil || d.svc == nil {
		return "", fmt.Errorf("task lifecycle domain is not initialized")
	}
	taskStore := d.svc.activeTaskStoreProvider()
	taskGit := d.svc.activeTaskGitProvider()

	cr, err := taskStore.LoadCR(crID)
	if err != nil {
		return "", err
	}
	if guardErr := d.svc.activeTaskMergeGuard()(cr); guardErr != nil {
		return "", guardErr
	}
	if _, err := ensureCRUID(cr); err != nil {
		return "", err
	}
	if _, err := d.svc.ensureCRBaseFields(cr, false); err != nil {
		return "", err
	}
	policy, err := d.svc.repoPolicy()
	if err != nil {
		return "", err
	}
	if cr.Status != model.StatusInProgress {
		return "", fmt.Errorf("cr %d is not in progress", crID)
	}
	if err := validateDoneTaskOptions(opts); err != nil {
		return "", err
	}

	now := d.svc.timestamp()
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
		if err := d.previewTaskCheckpointForDone(taskGit, cr, taskIndex, taskID, opts); err != nil {
			return "", err
		}
		return "", nil
	}

	commitSHA, err := d.applyTaskCheckpointForDone(taskGit, cr, task, taskIndex, taskID, now, actor, opts)
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

func (d *taskLifecycleDomain) previewTaskCheckpointForDone(gitProvider taskLifecycleGitProvider, cr *model.CR, taskIndex, taskID int, opts DoneTaskOptions) error {
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
		dirty, dirtyErr := d.hasTaskWorkingTreeChanges(gitProvider)
		if dirtyErr != nil {
			return dirtyErr
		}
		if !dirty {
			return fmt.Errorf("%w: task %d has no working tree changes", ErrNoTaskChanges, taskID)
		}
		return nil
	}

	if opts.FromContract {
		normalizedScope, normalizeErr := d.svc.normalizeContractScopePrefixes(cr.Subtasks[taskIndex].Contract.Scope)
		if normalizeErr != nil {
			return normalizeErr
		}
		if _, resolveErr := d.svc.resolveTaskCheckpointPathsFromContract(gitProvider, normalizedScope); resolveErr != nil {
			return resolveErr
		}
		return nil
	}

	if strings.TrimSpace(opts.PatchFile) != "" {
		patchPath, pathErr := d.svc.normalizePatchFilePath(opts.PatchFile)
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

	normalizedPaths, normalizeErr := d.svc.normalizeTaskScopePaths(opts.Paths)
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

func (d *taskLifecycleDomain) applyTaskCheckpointForDone(gitProvider taskLifecycleGitProvider, cr *model.CR, task *model.Subtask, taskIndex, taskID int, now, actor string, opts DoneTaskOptions) (string, error) {
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

	checkpointScope, checkpointChunks, scopeMode, stageErr := d.stageTaskCheckpointForDone(gitProvider, cr, taskIndex, taskID, opts)
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

	commitMessage := buildTaskCheckpointMessage(cr, task, strings.TrimSpace(opts.CommitType), scopeMode, len(checkpointChunks))
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
	if crContractBaselineIsEmpty(cr.ContractBaseline) {
		cr.ContractBaseline = crContractBaselineFromScope(cr.Contract.Scope, now, actor)
		cr.Events = append(cr.Events, model.Event{
			TS:      now,
			Actor:   actor,
			Type:    model.EventTypeCRContractBaselineCaptured,
			Summary: fmt.Sprintf("Captured CR contract scope baseline at first checkpoint (task %d)", taskID),
			Ref:     fmt.Sprintf("cr:%d", cr.ID),
			Meta: map[string]string{
				"task_id":           strconv.Itoa(taskID),
				"checkpoint_commit": sha,
				"scope":             strings.Join(cr.ContractBaseline.Scope, ","),
			},
		})
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

func (d *taskLifecycleDomain) stageTaskCheckpointForDone(gitProvider taskLifecycleGitProvider, cr *model.CR, taskIndex, taskID int, opts DoneTaskOptions) ([]string, []model.CheckpointChunk, string, error) {
	if opts.StageAll {
		dirty, dirtyErr := d.hasTaskWorkingTreeChanges(gitProvider)
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
		normalizedScope, normalizeErr := d.svc.normalizeContractScopePrefixes(cr.Subtasks[taskIndex].Contract.Scope)
		if normalizeErr != nil {
			return nil, nil, "", normalizeErr
		}
		paths, resolveErr := d.svc.resolveTaskCheckpointPathsFromContract(gitProvider, normalizedScope)
		if resolveErr != nil {
			return nil, nil, "", resolveErr
		}
		if err := gitProvider.StagePaths(paths); err != nil {
			return nil, nil, "", err
		}
		return paths, nil, model.TaskScopeModeTaskContract, nil
	}

	if strings.TrimSpace(opts.PatchFile) != "" {
		patchPath, pathErr := d.svc.normalizePatchFilePath(opts.PatchFile)
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

	normalizedPaths, normalizeErr := d.svc.normalizeTaskScopePaths(opts.Paths)
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

func (d *taskLifecycleDomain) hasTaskWorkingTreeChanges(gitProvider taskLifecycleGitProvider) (bool, error) {
	if d == nil || d.svc == nil {
		return false, fmt.Errorf("task lifecycle domain is not initialized")
	}
	entries, err := gitProvider.WorkingTreeStatus()
	if err != nil {
		return false, err
	}
	for _, entry := range entries {
		if d.svc.isIgnorableWorktreeEntry(entry) {
			continue
		}
		return true, nil
	}
	return false, nil
}
