package service

import (
	"fmt"
	"sophia/internal/model"
	"strings"
)

func nextTaskID(tasks []model.Subtask) int {
	maxID := 0
	for _, task := range tasks {
		if task.ID > maxID {
			maxID = task.ID
		}
	}
	return maxID + 1
}

func indexOfTask(tasks []model.Subtask, taskID int) int {
	for i := range tasks {
		if tasks[i].ID == taskID {
			return i
		}
	}
	return -1
}

func indexOfDrift(drifts []model.TaskContractDrift, driftID int) int {
	for i := range drifts {
		if drifts[i].ID == driftID {
			return i
		}
	}
	return -1
}

func indexOfCRDrift(drifts []model.CRContractDrift, driftID int) int {
	for i := range drifts {
		if drifts[i].ID == driftID {
			return i
		}
	}
	return -1
}

func buildMergeCommitMessage(cr *model.CR, actor, mergedAt string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "[CR-%d] %s\n\n", cr.ID, cr.Title)

	b.WriteString("Intent:\n")
	if strings.TrimSpace(cr.Description) == "" {
		b.WriteString("(none)\n\n")
	} else {
		b.WriteString(cr.Description)
		b.WriteString("\n\n")
	}

	b.WriteString("Subtasks:\n")
	if len(cr.Subtasks) == 0 {
		b.WriteString("- (none)\n\n")
	} else {
		for _, task := range cr.Subtasks {
			marker := taskStatusMarker(task.Status)
			fmt.Fprintf(&b, "- %s #%d %s\n", marker, task.ID, task.Title)
		}
		b.WriteString("\n")
	}

	b.WriteString("Notes:\n")
	if len(cr.Notes) == 0 {
		b.WriteString("- (none)\n\n")
	} else {
		for _, note := range cr.Notes {
			fmt.Fprintf(&b, "- %s\n", note)
		}
		b.WriteString("\n")
	}

	b.WriteString("Metadata:\n")
	fmt.Fprintf(&b, "- actor: %s\n", actor)
	fmt.Fprintf(&b, "- merged_at: %s\n", mergedAt)
	b.WriteString("\n")
	fmt.Fprintf(&b, "Sophia-CR: %d\n", cr.ID)
	fmt.Fprintf(&b, "Sophia-CR-UID: %s\n", strings.TrimSpace(cr.UID))
	fmt.Fprintf(&b, "Sophia-Base-Ref: %s\n", nonEmptyTrimmed(cr.BaseRef, cr.BaseBranch))
	fmt.Fprintf(&b, "Sophia-Base-Commit: %s\n", strings.TrimSpace(cr.BaseCommit))
	fmt.Fprintf(&b, "Sophia-Branch: %s\n", strings.TrimSpace(cr.Branch))
	fmt.Fprintf(&b, "Sophia-Branch-Scheme: %s\n", detectCRBranchScheme(cr.Branch))
	if cr.ParentCRID > 0 {
		fmt.Fprintf(&b, "Sophia-Parent-CR: %d\n", cr.ParentCRID)
	}
	fmt.Fprintf(&b, "Sophia-Intent: %s\n", cr.Title)
	fmt.Fprintf(&b, "Sophia-Tasks: %d completed\n", completedTasks(cr.Subtasks))
	return b.String()
}

func completedTasks(tasks []model.Subtask) int {
	count := 0
	for _, task := range tasks {
		if task.Status == model.TaskStatusDone {
			count++
		}
	}
	return count
}

func taskStatusMarker(status string) string {
	switch strings.TrimSpace(status) {
	case model.TaskStatusDone:
		return "[x]"
	case model.TaskStatusDelegated:
		return "[~]"
	default:
		return "[ ]"
	}
}

func buildTaskCheckpointMessage(cr *model.CR, task *model.Subtask, commitTypeOverride, scopeMode string, chunkCount int) string {
	taskType := resolveTaskCommitType(task, commitTypeOverride)
	attempt := taskCheckpointAttempt(cr, task.ID)
	taskIdentifier := fmt.Sprintf("task-%d", task.ID)
	if attempt > 1 {
		taskIdentifier = fmt.Sprintf("task-%dv%d", task.ID, attempt)
	}
	subject := fmt.Sprintf("%s(cr-%d/%s): %s", taskType, cr.ID, taskIdentifier, strings.TrimSpace(task.Title))
	var b strings.Builder
	b.WriteString(subject)
	b.WriteString("\n\n")
	fmt.Fprintf(&b, "Task: #%d %s\n", task.ID, strings.TrimSpace(task.Title))
	fmt.Fprintf(&b, "CR: %d %s\n\n", cr.ID, strings.TrimSpace(cr.Title))
	fmt.Fprintf(&b, "Sophia-CR: %d\n", cr.ID)
	fmt.Fprintf(&b, "Sophia-CR-UID: %s\n", strings.TrimSpace(cr.UID))
	fmt.Fprintf(&b, "Sophia-Base-Ref: %s\n", nonEmptyTrimmed(cr.BaseRef, cr.BaseBranch))
	fmt.Fprintf(&b, "Sophia-Base-Commit: %s\n", strings.TrimSpace(cr.BaseCommit))
	fmt.Fprintf(&b, "Sophia-Branch: %s\n", strings.TrimSpace(cr.Branch))
	fmt.Fprintf(&b, "Sophia-Branch-Scheme: %s\n", detectCRBranchScheme(cr.Branch))
	if cr.ParentCRID > 0 {
		fmt.Fprintf(&b, "Sophia-Parent-CR: %d\n", cr.ParentCRID)
	}
	if strings.TrimSpace(scopeMode) != "" {
		fmt.Fprintf(&b, "Sophia-Task-Scope-Mode: %s\n", strings.TrimSpace(scopeMode))
	}
	if strings.TrimSpace(scopeMode) == model.TaskScopeModePatchManifest {
		fmt.Fprintf(&b, "Sophia-Task-Chunk-Count: %d\n", chunkCount)
	}
	fmt.Fprintf(&b, "Sophia-Task: %d\n", task.ID)
	fmt.Fprintf(&b, "Sophia-Intent: %s\n", strings.TrimSpace(cr.Title))
	return b.String()
}

func taskCheckpointAttempt(cr *model.CR, taskID int) int {
	attempt := 1
	if cr == nil || taskID <= 0 {
		return attempt
	}
	taskRef := fmt.Sprintf("task:%d", taskID)
	for _, event := range cr.Events {
		if event.Type == model.EventTypeTaskReopened && event.Ref == taskRef {
			attempt++
		}
	}
	return attempt
}

func inferTaskCommitType(taskTitle string) string {
	if inferred, ok := inferTaskCommitTypeToken(taskTitle); ok {
		return inferred
	}
	return "chore"
}

func resolveTaskCommitType(task *model.Subtask, commitTypeOverride string) string {
	if normalized, ok := normalizeTaskCommitTypeToken(commitTypeOverride); ok {
		return normalized
	}
	if task == nil {
		return "chore"
	}
	if inferred, ok := inferTaskCommitTypeToken(task.Title); ok {
		return inferred
	}
	if inferred, ok := inferTaskCommitTypeToken(task.Contract.Intent); ok {
		return inferred
	}
	return "chore"
}

func inferTaskCommitTypeToken(raw string) (string, bool) {
	lower := strings.ToLower(strings.TrimSpace(raw))
	if lower == "" {
		return "", false
	}
	for _, token := range []string{"feat", "fix", "docs", "refactor", "test", "chore", "perf", "build", "ci", "style", "revert"} {
		if strings.HasPrefix(lower, token+":") || strings.HasPrefix(lower, token+" ") {
			return token, true
		}
	}
	return "", false
}

func normalizeTaskCommitTypeToken(raw string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "feat", "fix", "docs", "refactor", "test", "chore", "perf", "build", "ci", "style", "revert":
		return strings.ToLower(strings.TrimSpace(raw)), true
	default:
		return "", false
	}
}
