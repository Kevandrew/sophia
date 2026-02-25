package service

import (
	"fmt"
	"os"
	"path/filepath"
	"sophia/internal/model"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestMutationLockPathUsesGitCommonDir(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", "tracked"); err != nil {
		t.Fatalf("Init(tracked) error = %v", err)
	}

	commonDir := runGit(t, dir, "rev-parse", "--git-common-dir")
	if !filepath.IsAbs(commonDir) {
		commonDir = filepath.Join(dir, commonDir)
	}
	want := filepath.Join(commonDir, "sophia-mutation.lock")
	got := svc.mutationLockPath()
	wantDir, wantEvalErr := filepath.EvalSymlinks(filepath.Dir(want))
	gotDir, gotEvalErr := filepath.EvalSymlinks(filepath.Dir(got))
	if wantEvalErr == nil && gotEvalErr == nil {
		wantInfo, wantStatErr := os.Stat(wantDir)
		gotInfo, gotStatErr := os.Stat(gotDir)
		if wantStatErr == nil && gotStatErr == nil && !os.SameFile(wantInfo, gotInfo) {
			t.Fatalf("expected mutation lock path directory %q, got %q", wantDir, gotDir)
		}
	} else if filepath.Clean(filepath.Dir(got)) != filepath.Clean(filepath.Dir(want)) {
		t.Fatalf("expected mutation lock path %q, got %q", want, got)
	}
	if filepath.Base(got) != "sophia-mutation.lock" {
		t.Fatalf("expected mutation lock filename sophia-mutation.lock, got %q", filepath.Base(got))
	}
	if strings.Contains(got, string(filepath.Separator)+".sophia"+string(filepath.Separator)) {
		t.Fatalf("expected lock path to be common-dir shared, got %q", got)
	}
}

func TestSetCRContractWaitsForSharedMutationLock(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	cr, err := svc.AddCR("lock wait", "")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}

	lockHeld := make(chan struct{})
	release := make(chan struct{})
	go func() {
		_ = svc.store.WithMutationLockPath(svc.mutationLockPath(), 2*time.Second, func() error {
			close(lockHeld)
			<-release
			return nil
		})
	}()
	<-lockHeld

	why := "updated under lock"
	start := time.Now()
	done := make(chan error, 1)
	go func() {
		_, setErr := svc.SetCRContract(cr.ID, ContractPatch{Why: &why})
		done <- setErr
	}()
	time.Sleep(200 * time.Millisecond)
	close(release)
	if err := <-done; err != nil {
		t.Fatalf("SetCRContract() error = %v", err)
	}
	if waited := time.Since(start); waited < 180*time.Millisecond {
		t.Fatalf("expected SetCRContract to wait for lock release, elapsed=%s", waited)
	}
}

func TestConcurrentCRMutationsPreserveNotesAndContractEvents(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	cr, err := svc.AddCR("concurrent cr mutations", "")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}

	const workers = 20
	start := make(chan struct{})
	errCh := make(chan error, workers*2)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			<-start
			worker := New(dir)
			if addErr := worker.AddNote(cr.ID, fmt.Sprintf("note-%02d", idx)); addErr != nil {
				errCh <- addErr
				return
			}
			why := fmt.Sprintf("why-%02d", idx)
			_, setErr := worker.SetCRContract(cr.ID, ContractPatch{Why: &why})
			errCh <- setErr
		}(i)
	}

	close(start)
	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			t.Fatalf("concurrent CR mutation error = %v", err)
		}
	}

	stored, err := svc.store.LoadCR(cr.ID)
	if err != nil {
		t.Fatalf("LoadCR() error = %v", err)
	}
	if len(stored.Notes) != workers {
		t.Fatalf("expected %d notes after concurrent AddNote calls, got %d", workers, len(stored.Notes))
	}
	noteEvents := 0
	contractEvents := 0
	for _, event := range stored.Events {
		switch event.Type {
		case model.EventTypeNoteAdded:
			noteEvents++
		case model.EventTypeContractUpdated:
			contractEvents++
		}
	}
	if noteEvents != workers {
		t.Fatalf("expected %d note_added events, got %d", workers, noteEvents)
	}
	if contractEvents != workers {
		t.Fatalf("expected %d contract_updated events, got %d", workers, contractEvents)
	}
	if strings.TrimSpace(stored.Contract.Why) == "" {
		t.Fatalf("expected non-empty final contract why after concurrent updates")
	}
}

func TestConcurrentEditAndContractSetMutationsPreserveAllEvents(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	cr, err := svc.AddCR("concurrent mixed mutations", "")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}

	const workers = 20
	start := make(chan struct{})
	errCh := make(chan error, workers*2)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			<-start
			worker := New(dir)
			if _, editErr := worker.EditCR(cr.ID, strPtr(fmt.Sprintf("title-%02d", idx)), nil); editErr != nil {
				errCh <- editErr
				return
			}
			why := fmt.Sprintf("why-%02d", idx)
			_, setErr := worker.SetCRContract(cr.ID, ContractPatch{Why: &why})
			errCh <- setErr
		}(i)
	}

	close(start)
	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			t.Fatalf("concurrent mixed mutation error = %v", err)
		}
	}

	stored, err := svc.store.LoadCR(cr.ID)
	if err != nil {
		t.Fatalf("LoadCR() error = %v", err)
	}
	amendedEvents := 0
	contractEvents := 0
	for _, event := range stored.Events {
		switch event.Type {
		case model.EventTypeCRAmended:
			amendedEvents++
		case model.EventTypeContractUpdated:
			contractEvents++
		}
	}
	if amendedEvents != workers {
		t.Fatalf("expected %d cr_amended events, got %d", workers, amendedEvents)
	}
	if contractEvents != workers {
		t.Fatalf("expected %d contract_updated events, got %d", workers, contractEvents)
	}
	if strings.TrimSpace(stored.Title) == "" {
		t.Fatalf("expected non-empty final title after edits")
	}
}

func strPtr(v string) *string {
	return &v
}
