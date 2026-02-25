package tasks

import "sophia/internal/model"

func ApplyCheckpointState(task *model.Subtask, now, commit, message string, scope []string, chunks []model.CheckpointChunk) {
	if task == nil {
		return
	}
	task.CheckpointCommit = commit
	task.CheckpointAt = now
	task.CheckpointMessage = message
	task.CheckpointScope = append([]string(nil), scope...)
	task.CheckpointChunks = append([]model.CheckpointChunk(nil), chunks...)
	task.CheckpointOrphan = false
	task.CheckpointReason = ""
	task.CheckpointSource = model.TaskCheckpointSourceTaskCheckpoint
	task.CheckpointSyncAt = now
}

func ApplyNoCheckpointState(task *model.Subtask, now, reason string) {
	if task == nil {
		return
	}
	task.CheckpointCommit = ""
	task.CheckpointAt = now
	task.CheckpointMessage = ""
	task.CheckpointScope = []string{}
	task.CheckpointChunks = []model.CheckpointChunk{}
	task.CheckpointOrphan = false
	task.CheckpointReason = reason
	task.CheckpointSource = model.TaskCheckpointSourceTaskNoCheckpoint
	task.CheckpointSyncAt = now
}

func ClearCheckpointState(task *model.Subtask) {
	if task == nil {
		return
	}
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

func MarkTaskDone(task *model.Subtask, now, actor string) {
	if task == nil {
		return
	}
	task.Status = model.TaskStatusDone
	task.UpdatedAt = now
	task.CompletedAt = now
	task.CompletedBy = actor
}

func MarkTaskReopened(task *model.Subtask, now string, clearCheckpoint bool) {
	if task == nil {
		return
	}
	task.Status = model.TaskStatusOpen
	task.UpdatedAt = now
	task.CompletedAt = ""
	task.CompletedBy = ""
	if clearCheckpoint {
		ClearCheckpointState(task)
	}
}
