package service

import (
	"errors"
	"fmt"
	"os"
	"sophia/internal/model"
	"sort"
	"strconv"
	"strings"
)

func (s *Service) AddTask(crID int, title string) (*model.Subtask, error) {
	if strings.TrimSpace(title) == "" {
		return nil, errors.New("task title cannot be empty")
	}
	cr, err := s.store.LoadCR(crID)
	if err != nil {
		return nil, err
	}
	if guardErr := s.ensureNoMergeInProgressForCR(cr); guardErr != nil {
		return nil, guardErr
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
	cr.Events = append(cr.Events, model.Event{
		TS:      now,
		Actor:   actor,
		Type:    "task_added",
		Summary: title,
		Ref:     fmt.Sprintf("task:%d", newTaskID),
	})
	cr.UpdatedAt = now
	if err := s.store.SaveCR(cr); err != nil {
		return nil, err
	}
	return &task, nil
}

func cloneTaskContract(contract model.TaskContract) model.TaskContract {
	out := contract
	out.AcceptanceCriteria = append([]string(nil), contract.AcceptanceCriteria...)
	out.Scope = append([]string(nil), contract.Scope...)
	out.AcceptanceChecks = append([]string(nil), contract.AcceptanceChecks...)
	return out
}

func normalizeAcceptanceCheckKeys(values []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, raw := range values {
		key := strings.TrimSpace(raw)
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}

func taskContractBaselineIsEmpty(baseline model.TaskContractBaseline) bool {
	return strings.TrimSpace(baseline.CapturedAt) == "" &&
		strings.TrimSpace(baseline.CapturedBy) == "" &&
		strings.TrimSpace(baseline.Intent) == "" &&
		len(baseline.AcceptanceCriteria) == 0 &&
		len(baseline.Scope) == 0 &&
		len(baseline.AcceptanceChecks) == 0
}

func taskContractBaselineFromContract(contract model.TaskContract, capturedAt, capturedBy string) model.TaskContractBaseline {
	return model.TaskContractBaseline{
		CapturedAt:         capturedAt,
		CapturedBy:         capturedBy,
		Intent:             strings.TrimSpace(contract.Intent),
		AcceptanceCriteria: append([]string(nil), contract.AcceptanceCriteria...),
		Scope:              append([]string(nil), contract.Scope...),
		AcceptanceChecks:   append([]string(nil), contract.AcceptanceChecks...),
	}
}

func nextTaskContractDriftID(drifts []model.TaskContractDrift) int {
	maxID := 0
	for _, drift := range drifts {
		if drift.ID > maxID {
			maxID = drift.ID
		}
	}
	return maxID + 1
}

func scopeWidened(beforeScope, afterScope []string) bool {
	if len(afterScope) == 0 {
		return false
	}
	if len(beforeScope) == 0 {
		return true
	}
	for _, next := range afterScope {
		covered := false
		for _, prev := range beforeScope {
			if pathMatchesScopePrefix(next, prev) {
				covered = true
				break
			}
		}
		if !covered {
			return true
		}
	}
	return false
}

func taskAcceptanceCheckPolicyMap(policy *model.RepoPolicy) map[string]struct{} {
	out := map[string]struct{}{}
	if policy == nil {
		return out
	}
	for _, check := range policy.Trust.Checks.Definitions {
		key := strings.TrimSpace(check.Key)
		if key == "" {
			continue
		}
		out[key] = struct{}{}
	}
	return out
}

func validateTaskAcceptanceCheckKeys(taskID int, checks []string, policy *model.RepoPolicy) error {
	allowed := taskAcceptanceCheckPolicyMap(policy)
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
	cr, err := s.store.LoadCR(crID)
	if err != nil {
		return nil, err
	}
	if guardErr := s.ensureNoMergeInProgressForCR(cr); guardErr != nil {
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
	beforeContract := cloneTaskContract(task.Contract)

	changed := []string{}
	if patch.Intent != nil {
		normalized := strings.TrimSpace(*patch.Intent)
		if task.Contract.Intent != normalized {
			task.Contract.Intent = normalized
			changed = append(changed, "intent")
		}
	}
	if patch.AcceptanceCriteria != nil {
		normalized := normalizeNonEmptyStringList(*patch.AcceptanceCriteria)
		if !equalStringSlices(task.Contract.AcceptanceCriteria, normalized) {
			task.Contract.AcceptanceCriteria = normalized
			changed = append(changed, "acceptance_criteria")
		}
	}
	if patch.Scope != nil {
		normalized, normalizeErr := s.normalizeContractScopePrefixes(*patch.Scope)
		if normalizeErr != nil {
			return nil, normalizeErr
		}
		if scopeErr := enforceScopeAllowlist(normalized, policy.Scope.AllowedPrefixes, "task contract scope"); scopeErr != nil {
			return nil, scopeErr
		}
		if !equalStringSlices(task.Contract.Scope, normalized) {
			task.Contract.Scope = normalized
			changed = append(changed, "scope")
		}
	}
	if patch.AcceptanceChecks != nil {
		normalized := normalizeAcceptanceCheckKeys(*patch.AcceptanceChecks)
		if validateErr := validateTaskAcceptanceCheckKeys(taskID, normalized, policy); validateErr != nil {
			return nil, validateErr
		}
		if !equalStringSlices(task.Contract.AcceptanceChecks, normalized) {
			task.Contract.AcceptanceChecks = normalized
			changed = append(changed, "acceptance_checks")
		}
	}
	if len(changed) == 0 {
		return nil, ErrNoCRChanges
	}

	now := s.timestamp()
	actor := s.git.Actor()
	changeReason := ""
	if patch.ChangeReason != nil {
		changeReason = strings.TrimSpace(*patch.ChangeReason)
	}

	taskHasCheckpoint := strings.TrimSpace(task.CheckpointCommit) != ""
	if taskHasCheckpoint && taskContractBaselineIsEmpty(task.ContractBaseline) {
		task.ContractBaseline = taskContractBaselineFromContract(beforeContract, now, actor)
	}

	var drift *model.TaskContractDrift
	if taskHasCheckpoint {
		driftFields := []string{}
		scopeChanged := !equalStringSlices(beforeContract.Scope, task.Contract.Scope)
		if scopeChanged && scopeWidened(beforeContract.Scope, task.Contract.Scope) {
			driftFields = append(driftFields, "scope_widened")
		}
		checksChanged := !equalStringSlices(beforeContract.AcceptanceChecks, task.Contract.AcceptanceChecks)
		if checksChanged {
			driftFields = append(driftFields, "acceptance_checks_changed")
		}
		if len(driftFields) > 0 {
			record := model.TaskContractDrift{
				ID:                     nextTaskContractDriftID(task.ContractDrifts),
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
			drift = &record
		}
	}

	task.Contract.UpdatedAt = now
	task.Contract.UpdatedBy = actor
	task.UpdatedAt = now
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
		Type:    "task_contract_updated",
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
			Type:    "task_contract_drift_recorded",
			Summary: fmt.Sprintf("Recorded task %d contract drift #%d (%s)", taskID, drift.ID, strings.Join(drift.Fields, ",")),
			Ref:     fmt.Sprintf("task:%d", taskID),
			Meta:    driftMeta,
		})
	}
	if err := s.store.SaveCR(cr); err != nil {
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
	cr, err := s.store.LoadCR(crID)
	if err != nil {
		return nil, err
	}
	if guardErr := s.ensureNoMergeInProgressForCR(cr); guardErr != nil {
		return nil, guardErr
	}
	taskIndex := indexOfTask(cr.Subtasks, taskID)
	if taskIndex < 0 {
		return nil, fmt.Errorf("task %d not found in cr %d", taskID, crID)
	}
	task := &cr.Subtasks[taskIndex]
	driftIndex := -1
	for i := range task.ContractDrifts {
		if task.ContractDrifts[i].ID == driftID {
			driftIndex = i
			break
		}
	}
	if driftIndex < 0 {
		return nil, fmt.Errorf("task %d drift %d not found in cr %d", taskID, driftID, crID)
	}

	now := s.timestamp()
	actor := s.git.Actor()
	task.ContractDrifts[driftIndex].Acknowledged = true
	task.ContractDrifts[driftIndex].AcknowledgedAt = now
	task.ContractDrifts[driftIndex].AcknowledgedBy = actor
	task.ContractDrifts[driftIndex].AckReason = reason
	task.UpdatedAt = now
	cr.UpdatedAt = now
	cr.Events = append(cr.Events, model.Event{
		TS:      now,
		Actor:   actor,
		Type:    "task_contract_drift_acknowledged",
		Summary: fmt.Sprintf("Acknowledged task %d contract drift #%d", taskID, driftID),
		Ref:     fmt.Sprintf("task:%d", taskID),
		Meta: map[string]string{
			"drift_id": strconv.Itoa(driftID),
			"reason":   reason,
		},
	})
	if err := s.store.SaveCR(cr); err != nil {
		return nil, err
	}
	ack := task.ContractDrifts[driftIndex]
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
	cr, err := s.store.LoadCR(crID)
	if err != nil {
		return nil, err
	}
	if cr.Status != model.StatusInProgress {
		return nil, fmt.Errorf("cr %d is not in progress", crID)
	}
	if indexOfTask(cr.Subtasks, taskID) < 0 {
		return nil, fmt.Errorf("task %d not found in cr %d", taskID, crID)
	}
	currentBranch, branchErr := s.git.CurrentBranch()
	if branchErr != nil {
		return nil, branchErr
	}
	if currentBranch != cr.Branch {
		return nil, fmt.Errorf("chunk list requires active CR branch %q, current branch is %q; run `sophia cr switch %d`", cr.Branch, currentBranch, cr.ID)
	}
	normalizedPaths := []string{}
	if len(paths) > 0 {
		normalized, normalizeErr := s.normalizeTaskScopePaths(paths)
		if normalizeErr != nil {
			return nil, normalizeErr
		}
		normalizedPaths = normalized
	}
	diff, err := s.git.WorkingTreeUnifiedDiff(normalizedPaths, 0)
	if err != nil {
		return nil, err
	}
	parsed, err := parsePatchChunks(diff)
	if err != nil {
		return nil, fmt.Errorf("parse working tree diff chunks: %w", err)
	}
	chunks := make([]TaskChunk, 0, len(parsed))
	for _, chunk := range parsed {
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
	return chunks, nil
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
	cr, err := s.store.LoadCR(crID)
	if err != nil {
		return nil, err
	}
	if guardErr := s.ensureNoMergeInProgressForCR(cr); guardErr != nil {
		return nil, guardErr
	}
	if cr.Status != model.StatusInProgress {
		return nil, fmt.Errorf("cr %d is not in progress", crID)
	}
	taskIndex := indexOfTask(cr.Subtasks, taskID)
	if taskIndex < 0 {
		return nil, fmt.Errorf("task %d not found in cr %d", taskID, crID)
	}
	task := &cr.Subtasks[taskIndex]
	if task.Status != model.TaskStatusDone {
		return nil, fmt.Errorf("%w: task %d in cr %d has status %q", ErrTaskNotDone, taskID, crID, task.Status)
	}

	now := s.timestamp()
	actor := s.git.Actor()
	previousCheckpoint := strings.TrimSpace(task.CheckpointCommit)

	task.Status = model.TaskStatusOpen
	task.UpdatedAt = now
	task.CompletedAt = ""
	task.CompletedBy = ""
	if opts.ClearCheckpoint {
		task.CheckpointCommit = ""
		task.CheckpointAt = ""
		task.CheckpointMessage = ""
		task.CheckpointScope = nil
		task.CheckpointChunks = nil
		task.CheckpointOrphan = false
		task.CheckpointReason = ""
		task.CheckpointSource = ""
		task.CheckpointSyncAt = ""
	}

	meta := map[string]string{
		"checkpoint_cleared": strconv.FormatBool(opts.ClearCheckpoint),
	}
	if previousCheckpoint != "" {
		meta["previous_checkpoint_commit"] = previousCheckpoint
	}
	cr.Events = append(cr.Events, model.Event{
		TS:      now,
		Actor:   actor,
		Type:    "task_reopened",
		Summary: task.Title,
		Ref:     fmt.Sprintf("task:%d", taskID),
		Meta:    meta,
	})
	cr.UpdatedAt = now
	if err := s.store.SaveCR(cr); err != nil {
		return nil, err
	}

	reopened := cr.Subtasks[taskIndex]
	return &reopened, nil
}

func (s *Service) DoneTaskWithCheckpoint(crID, taskID int, opts DoneTaskOptions) (string, error) {
	cr, err := s.store.LoadCR(crID)
	if err != nil {
		return "", err
	}
	if guardErr := s.ensureNoMergeInProgressForCR(cr); guardErr != nil {
		return "", guardErr
	}
	if _, err := ensureCRUID(cr); err != nil {
		return "", err
	}
	if _, err := s.ensureCRBaseFields(cr, true); err != nil {
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
	actor := s.git.Actor()
	found := false
	title := ""
	taskIndex := -1
	for i := range cr.Subtasks {
		if cr.Subtasks[i].ID == taskID {
			found = true
			title = cr.Subtasks[i].Title
			taskIndex = i
			break
		}
	}
	if !found {
		return "", fmt.Errorf("task %d not found in cr %d", taskID, crID)
	}
	if cr.Subtasks[taskIndex].Status == model.TaskStatusDelegated {
		return "", fmt.Errorf("%w: task %d in cr %d is delegated to child CRs", ErrTaskDelegated, taskID, crID)
	}
	missingContractFields := missingTaskContractFields(cr.Subtasks[taskIndex].Contract, policy.TaskContract.RequiredFields)
	if len(missingContractFields) > 0 {
		return "", fmt.Errorf("%w: task %d missing %s", ErrTaskContractIncomplete, taskID, strings.Join(missingContractFields, ","))
	}

	commitSHA := ""
	if opts.Checkpoint {
		currentBranch, branchErr := s.git.CurrentBranch()
		if branchErr != nil {
			return "", branchErr
		}
		if currentBranch != cr.Branch {
			return "", fmt.Errorf("checkpoint requires active CR branch %q, current branch is %q; run `sophia cr switch %d`", cr.Branch, currentBranch, cr.ID)
		}

		preStaged, stagedErr := s.git.HasStagedChanges()
		if stagedErr != nil {
			return "", stagedErr
		}
		if preStaged {
			return "", fmt.Errorf("%w: unstage changes before running task checkpoint", ErrPreStagedChanges)
		}

		checkpointScope := []string{}
		checkpointChunks := []model.CheckpointChunk{}
		scopeMode := ""
		if opts.StageAll {
			dirty, _, dirtyErr := s.workingTreeDirtySummary()
			if dirtyErr != nil {
				return "", dirtyErr
			}
			if !dirty {
				return "", fmt.Errorf("%w: task %d has no working tree changes", ErrNoTaskChanges, taskID)
			}
			if err := s.git.StageAll(); err != nil {
				return "", err
			}
			checkpointScope = []string{"*"}
			scopeMode = "all"
		} else if opts.FromContract {
			normalizedScope, normalizeErr := s.normalizeContractScopePrefixes(cr.Subtasks[taskIndex].Contract.Scope)
			if normalizeErr != nil {
				return "", normalizeErr
			}
			paths, resolveErr := s.resolveTaskCheckpointPathsFromContract(normalizedScope)
			if resolveErr != nil {
				return "", resolveErr
			}
			if err := s.git.StagePaths(paths); err != nil {
				return "", err
			}
			checkpointScope = paths
			scopeMode = "task_contract"
		} else if strings.TrimSpace(opts.PatchFile) != "" {
			patchPath, pathErr := s.normalizePatchFilePath(opts.PatchFile)
			if pathErr != nil {
				return "", pathErr
			}
			patchContent, readErr := os.ReadFile(patchPath)
			if readErr != nil {
				return "", fmt.Errorf("read patch file %q: %w", opts.PatchFile, readErr)
			}
			parsedChunks, parseErr := parsePatchChunks(string(patchContent))
			if parseErr != nil {
				return "", fmt.Errorf("%w: parse patch file: %v", ErrInvalidTaskScope, parseErr)
			}
			if len(parsedChunks) == 0 {
				return "", fmt.Errorf("%w: patch file %q contains no hunks", ErrNoTaskChanges, opts.PatchFile)
			}
			if err := s.git.ApplyPatchToIndex(patchPath); err != nil {
				return "", err
			}
			checkpointScope = checkpointChunkPaths(parsedChunks)
			checkpointChunks = make([]model.CheckpointChunk, 0, len(parsedChunks))
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
			scopeMode = "patch_manifest"
		} else {
			normalizedPaths, normalizeErr := s.normalizeTaskScopePaths(opts.Paths)
			if normalizeErr != nil {
				return "", normalizeErr
			}

			changedPathFound := false
			for _, scopePath := range normalizedPaths {
				hasChanges, hasErr := s.git.PathHasChanges(scopePath)
				if hasErr != nil {
					return "", hasErr
				}
				if hasChanges {
					changedPathFound = true
					break
				}
			}
			if !changedPathFound {
				return "", fmt.Errorf("%w: none of the scoped paths have changes", ErrNoTaskChanges)
			}
			if err := s.git.StagePaths(normalizedPaths); err != nil {
				return "", err
			}
			checkpointScope = normalizedPaths
			scopeMode = "path"
		}

		hasStaged, stagedErr := s.git.HasStagedChanges()
		if stagedErr != nil {
			return "", stagedErr
		}
		if !hasStaged {
			return "", fmt.Errorf("%w: no staged changes after applying scope", ErrNoTaskChanges)
		}

		commitMessage := buildTaskCheckpointMessage(cr, &cr.Subtasks[taskIndex], scopeMode, len(checkpointChunks))
		if err := s.git.Commit(commitMessage); err != nil {
			return "", err
		}
		sha, shaErr := s.git.HeadShortSHA()
		if shaErr != nil {
			return "", shaErr
		}
		commitSHA = sha
		cr.Subtasks[taskIndex].CheckpointCommit = sha
		cr.Subtasks[taskIndex].CheckpointAt = now
		cr.Subtasks[taskIndex].CheckpointMessage = commitMessage
		cr.Subtasks[taskIndex].CheckpointScope = append([]string(nil), checkpointScope...)
		cr.Subtasks[taskIndex].CheckpointChunks = append([]model.CheckpointChunk(nil), checkpointChunks...)
		cr.Subtasks[taskIndex].CheckpointOrphan = false
		cr.Subtasks[taskIndex].CheckpointReason = ""
		cr.Subtasks[taskIndex].CheckpointSource = "task_checkpoint"
		cr.Subtasks[taskIndex].CheckpointSyncAt = now
		if taskContractBaselineIsEmpty(cr.Subtasks[taskIndex].ContractBaseline) {
			cr.Subtasks[taskIndex].ContractBaseline = taskContractBaselineFromContract(cr.Subtasks[taskIndex].Contract, now, actor)
		}
		cr.Events = append(cr.Events, model.Event{
			TS:      now,
			Actor:   actor,
			Type:    "task_checkpointed",
			Summary: fmt.Sprintf("Checkpointed task %d as %s", taskID, sha),
			Ref:     fmt.Sprintf("task:%d", taskID),
			Meta: map[string]string{
				"commit":  sha,
				"message": strings.SplitN(commitMessage, "\n", 2)[0],
				"scope":   strings.Join(checkpointScope, ","),
			},
		})
		if opts.FromContract {
			cr.Events[len(cr.Events)-1].Meta["scope_source"] = "task_contract"
		}
		if strings.TrimSpace(opts.PatchFile) != "" {
			cr.Events[len(cr.Events)-1].Meta["scope_source"] = "patch_manifest"
			cr.Events[len(cr.Events)-1].Meta["chunk_count"] = strconv.Itoa(len(checkpointChunks))
		}
	} else {
		reason := strings.TrimSpace(opts.NoCheckpointReason)
		cr.Subtasks[taskIndex].CheckpointCommit = ""
		cr.Subtasks[taskIndex].CheckpointAt = now
		cr.Subtasks[taskIndex].CheckpointMessage = ""
		cr.Subtasks[taskIndex].CheckpointScope = []string{}
		cr.Subtasks[taskIndex].CheckpointChunks = []model.CheckpointChunk{}
		cr.Subtasks[taskIndex].CheckpointOrphan = false
		cr.Subtasks[taskIndex].CheckpointReason = reason
		cr.Subtasks[taskIndex].CheckpointSource = "task_no_checkpoint"
		cr.Subtasks[taskIndex].CheckpointSyncAt = now
	}

	cr.Subtasks[taskIndex].Status = model.TaskStatusDone
	cr.Subtasks[taskIndex].UpdatedAt = now
	cr.Subtasks[taskIndex].CompletedAt = now
	cr.Subtasks[taskIndex].CompletedBy = actor

	taskDoneMeta := map[string]string{
		"checkpoint": strconv.FormatBool(opts.Checkpoint),
	}
	if opts.Checkpoint {
		taskDoneMeta["checkpoint_commit"] = commitSHA
		taskDoneMeta["checkpoint_source"] = "task_checkpoint"
	} else {
		taskDoneMeta["checkpoint_source"] = "task_no_checkpoint"
		taskDoneMeta["no_checkpoint_reason"] = strings.TrimSpace(opts.NoCheckpointReason)
	}
	cr.Events = append(cr.Events, model.Event{
		TS:      now,
		Actor:   actor,
		Type:    "task_done",
		Summary: title,
		Ref:     fmt.Sprintf("task:%d", taskID),
		Meta:    taskDoneMeta,
	})
	cr.UpdatedAt = now

	if err := s.store.SaveCR(cr); err != nil {
		return "", err
	}
	return commitSHA, nil
}
