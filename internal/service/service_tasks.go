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

func (s *Service) SetTaskContract(crID, taskID int, patch TaskContractPatch) ([]string, error) {
	cr, err := s.store.LoadCR(crID)
	if err != nil {
		return nil, err
	}
	taskIndex := indexOfTask(cr.Subtasks, taskID)
	if taskIndex < 0 {
		return nil, fmt.Errorf("task %d not found in cr %d", taskID, crID)
	}
	task := &cr.Subtasks[taskIndex]

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
		if !equalStringSlices(task.Contract.Scope, normalized) {
			task.Contract.Scope = normalized
			changed = append(changed, "scope")
		}
	}
	if len(changed) == 0 {
		return nil, ErrNoCRChanges
	}

	now := s.timestamp()
	actor := s.git.Actor()
	task.Contract.UpdatedAt = now
	task.Contract.UpdatedBy = actor
	task.UpdatedAt = now
	cr.UpdatedAt = now
	cr.Events = append(cr.Events, model.Event{
		TS:      now,
		Actor:   actor,
		Type:    "task_contract_updated",
		Summary: fmt.Sprintf("Updated task %d contract fields: %s", taskID, strings.Join(changed, ",")),
		Ref:     fmt.Sprintf("task:%d", taskID),
		Meta: map[string]string{
			"fields": strings.Join(changed, ","),
		},
	})
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
	return &contract, nil
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
		return nil, fmt.Errorf("chunk list requires active CR branch %q, current branch is %q", cr.Branch, currentBranch)
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
	_, err := s.DoneTaskWithCheckpoint(crID, taskID, DoneTaskOptions{Checkpoint: false})
	return err
}

func (s *Service) DoneTaskWithCheckpoint(crID, taskID int, opts DoneTaskOptions) (string, error) {
	cr, err := s.store.LoadCR(crID)
	if err != nil {
		return "", err
	}
	if _, err := ensureCRUID(cr); err != nil {
		return "", err
	}
	if _, err := s.ensureCRBaseFields(cr, true); err != nil {
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
	missingContractFields := missingTaskContractFields(cr.Subtasks[taskIndex].Contract)
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
			return "", fmt.Errorf("checkpoint requires active CR branch %q, current branch is %q", cr.Branch, currentBranch)
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
	}

	cr.Subtasks[taskIndex].Status = model.TaskStatusDone
	cr.Subtasks[taskIndex].UpdatedAt = now
	cr.Subtasks[taskIndex].CompletedAt = now
	cr.Subtasks[taskIndex].CompletedBy = actor

	cr.Events = append(cr.Events, model.Event{
		TS:      now,
		Actor:   actor,
		Type:    "task_done",
		Summary: title,
		Ref:     fmt.Sprintf("task:%d", taskID),
	})
	cr.UpdatedAt = now

	if err := s.store.SaveCR(cr); err != nil {
		return "", err
	}
	return commitSHA, nil
}
