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
			marker := "[ ]"
			if task.Status == model.TaskStatusDone {
				marker = "[x]"
			}
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

func buildTaskCheckpointMessage(cr *model.CR, task *model.Subtask, scopeMode string, chunkCount int) string {
	taskType := inferTaskCommitType(task.Title)
	subject := fmt.Sprintf("%s(cr-%d/task-%d): %s", taskType, cr.ID, task.ID, strings.TrimSpace(task.Title))
	var b strings.Builder
	b.WriteString(subject)
	b.WriteString("\n\n")
	fmt.Fprintf(&b, "Task: #%d %s\n", task.ID, strings.TrimSpace(task.Title))
	fmt.Fprintf(&b, "CR: %d %s\n\n", cr.ID, strings.TrimSpace(cr.Title))
	fmt.Fprintf(&b, "Sophia-CR: %d\n", cr.ID)
	fmt.Fprintf(&b, "Sophia-CR-UID: %s\n", strings.TrimSpace(cr.UID))
	fmt.Fprintf(&b, "Sophia-Base-Ref: %s\n", nonEmptyTrimmed(cr.BaseRef, cr.BaseBranch))
	fmt.Fprintf(&b, "Sophia-Base-Commit: %s\n", strings.TrimSpace(cr.BaseCommit))
	if cr.ParentCRID > 0 {
		fmt.Fprintf(&b, "Sophia-Parent-CR: %d\n", cr.ParentCRID)
	}
	if strings.TrimSpace(scopeMode) != "" {
		fmt.Fprintf(&b, "Sophia-Task-Scope-Mode: %s\n", strings.TrimSpace(scopeMode))
	}
	if strings.TrimSpace(scopeMode) == "patch_manifest" {
		fmt.Fprintf(&b, "Sophia-Task-Chunk-Count: %d\n", chunkCount)
	}
	fmt.Fprintf(&b, "Sophia-Task: %d\n", task.ID)
	fmt.Fprintf(&b, "Sophia-Intent: %s\n", strings.TrimSpace(cr.Title))
	return b.String()
}

func inferTaskCommitType(taskTitle string) string {
	prefixes := []string{"feat", "fix", "docs", "refactor", "test", "chore", "perf", "build", "ci", "style", "revert"}
	lower := strings.ToLower(strings.TrimSpace(taskTitle))
	for _, prefix := range prefixes {
		token := prefix + ":"
		if strings.HasPrefix(lower, token) || strings.HasPrefix(lower, prefix+" ") {
			return prefix
		}
	}
	return "chore"
}
