package service

import "testing"

func TestReopenTaskDoesNotIntroduceMergeBlocker(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	cr, err := svc.AddCR("Merge blocker regression", "open/reopened tasks should not add merge blocker")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	setValidContract(t, svc, cr.ID)
	task, err := svc.AddTask(cr.ID, "chore: done then reopen")
	if err != nil {
		t.Fatalf("AddTask() error = %v", err)
	}
	setValidTaskContract(t, svc, cr.ID, task.ID)
	if err := svc.DoneTask(cr.ID, task.ID); err != nil {
		t.Fatalf("DoneTask() error = %v", err)
	}
	if _, err := svc.ReopenTask(cr.ID, task.ID, ReopenTaskOptions{}); err != nil {
		t.Fatalf("ReopenTask() error = %v", err)
	}

	status, err := svc.StatusCR(cr.ID)
	if err != nil {
		t.Fatalf("StatusCR() error = %v", err)
	}
	if status.TasksOpen != 1 {
		t.Fatalf("expected one open task after reopen, got %d", status.TasksOpen)
	}
	if status.MergeBlocked {
		t.Fatalf("expected merge not blocked by reopened/open task, blockers=%#v", status.MergeBlockers)
	}
}
