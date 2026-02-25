package service

import (
	"fmt"
	"time"

	"sophia/internal/model"
)

const harnessTimestamp = "2026-02-25T12:00:00Z"

type seedCROptions struct {
	UID         string
	Description string
	Status      string
	BaseBranch  string
	BaseRef     string
	BaseCommit  string
	Branch      string
	ParentCRID  int
}

func seedCR(id int, title string, opts seedCROptions) *model.CR {
	if id < 1 {
		id = 1
	}
	if title == "" {
		title = fmt.Sprintf("CR %d", id)
	}
	status := opts.Status
	if status == "" {
		status = model.StatusInProgress
	}
	baseBranch := opts.BaseBranch
	if baseBranch == "" {
		baseBranch = "main"
	}
	baseRef := opts.BaseRef
	if baseRef == "" {
		baseRef = baseBranch
	}
	baseCommit := opts.BaseCommit
	if baseCommit == "" {
		baseCommit = "base-sha"
	}
	branch := opts.Branch
	if branch == "" {
		branch = fmt.Sprintf("cr-%d-harness", id)
	}
	uid := opts.UID
	if uid == "" {
		uid = fmt.Sprintf("cr_harness_%03d", id)
	}
	return &model.CR{
		ID:          id,
		UID:         uid,
		Title:       title,
		Description: opts.Description,
		Status:      status,
		BaseBranch:  baseBranch,
		BaseRef:     baseRef,
		BaseCommit:  baseCommit,
		ParentCRID:  opts.ParentCRID,
		Branch:      branch,
		Notes:       []string{},
		Evidence:    []model.EvidenceEntry{},
		Subtasks:    []model.Subtask{},
		Events:      []model.Event{},
		CreatedAt:   harnessTimestamp,
		UpdatedAt:   harnessTimestamp,
	}
}

func seedTask(id int, title, status, actor string) model.Subtask {
	if id < 1 {
		id = 1
	}
	if title == "" {
		title = fmt.Sprintf("Task %d", id)
	}
	if status == "" {
		status = model.TaskStatusOpen
	}
	if actor == "" {
		actor = "Runtime Tester <runtime@test>"
	}
	task := model.Subtask{
		ID:        id,
		Title:     title,
		Status:    status,
		CreatedAt: harnessTimestamp,
		UpdatedAt: harnessTimestamp,
		CreatedBy: actor,
	}
	if status == model.TaskStatusDone {
		task.CompletedAt = harnessTimestamp
		task.CompletedBy = actor
	}
	return task
}

func harnessNow() time.Time {
	return time.Date(2026, time.February, 25, 12, 0, 0, 0, time.UTC)
}
