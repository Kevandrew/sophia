package service

import (
	"fmt"
	"sophia/internal/model"
	"sync"
	"testing"
)

func TestConcurrentAddTaskMutationsPreserveAllUpdates(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	cr, err := svc.AddCR("concurrent task adds", "")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}

	const workers = 24
	start := make(chan struct{})
	errCh := make(chan error, workers)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			<-start
			worker := New(dir)
			_, addErr := worker.AddTask(cr.ID, fmt.Sprintf("task-%02d", idx))
			errCh <- addErr
		}(i)
	}

	close(start)
	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			t.Fatalf("concurrent AddTask() error = %v", err)
		}
	}

	tasks, err := svc.ListTasks(cr.ID)
	if err != nil {
		t.Fatalf("ListTasks() error = %v", err)
	}
	if len(tasks) != workers {
		t.Fatalf("expected %d tasks after concurrent adds, got %d", workers, len(tasks))
	}
	seen := map[int]struct{}{}
	for _, task := range tasks {
		if _, ok := seen[task.ID]; ok {
			t.Fatalf("duplicate task id detected: %d", task.ID)
		}
		seen[task.ID] = struct{}{}
	}
}

func TestConcurrentSetTaskContractMutationsRecordEveryUpdateEvent(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	cr, err := svc.AddCR("concurrent task contract", "")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	task, err := svc.AddTask(cr.ID, "contract target")
	if err != nil {
		t.Fatalf("AddTask() error = %v", err)
	}

	const workers = 20
	start := make(chan struct{})
	errCh := make(chan error, workers)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			<-start
			intent := fmt.Sprintf("intent-%02d", idx)
			worker := New(dir)
			_, setErr := worker.SetTaskContract(cr.ID, task.ID, TaskContractPatch{
				Intent: &intent,
			})
			errCh <- setErr
		}(i)
	}

	close(start)
	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			t.Fatalf("concurrent SetTaskContract() error = %v", err)
		}
	}

	stored, err := svc.store.LoadCR(cr.ID)
	if err != nil {
		t.Fatalf("LoadCR() error = %v", err)
	}
	updates := 0
	for _, event := range stored.Events {
		if event.Type == model.EventTypeTaskContractUpdated {
			updates++
		}
	}
	if updates != workers {
		t.Fatalf("expected %d task_contract_updated events, got %d", workers, updates)
	}
}
