package service

import (
	"os"
	"path/filepath"
	"testing"

	"sophia/internal/model"
)

func TestValidateCRFailsWhenRequiredContractFieldsMissing(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	cr, err := svc.AddCR("Missing contract", "should fail validation")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}

	report, err := svc.ValidateCR(cr.ID)
	if err != nil {
		t.Fatalf("ValidateCR() error = %v", err)
	}
	if report.Valid {
		t.Fatalf("expected report to be invalid")
	}
	if len(report.Errors) < 7 {
		t.Fatalf("expected required-field errors, got %#v", report.Errors)
	}
}

func TestValidateCRDetectsScopeDriftAsError(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")

	cr, err := svc.AddCR("Scope drift", "drift")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	scope := []string{"internal/service"}
	why := "Contain service changes."
	nonGoals := []string{"No docs changes"}
	invariants := []string{"No API changes"}
	blast := "service only"
	testPlan := "go test ./..."
	rollback := "revert"
	if _, err := svc.SetCRContract(cr.ID, ContractPatch{
		Why:          &why,
		Scope:        &scope,
		NonGoals:     &nonGoals,
		Invariants:   &invariants,
		BlastRadius:  &blast,
		TestPlan:     &testPlan,
		RollbackPlan: &rollback,
	}); err != nil {
		t.Fatalf("SetCRContract() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("drift\n"), 0o644); err != nil {
		t.Fatalf("write README: %v", err)
	}
	runGit(t, dir, "add", "README.md")
	runGit(t, dir, "commit", "-m", "docs: drift")

	report, err := svc.ValidateCR(cr.ID)
	if err != nil {
		t.Fatalf("ValidateCR() error = %v", err)
	}
	if report.Valid {
		t.Fatalf("expected validation failure due to scope drift")
	}
	if !containsAny(report.Errors, "scope drift") {
		t.Fatalf("expected scope drift error, got %#v", report.Errors)
	}
}

func TestValidateCRWarnsOnTaskScopeMismatch(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")

	cr, err := svc.AddCR("Task warning", "task scope warning")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	task, err := svc.AddTask(cr.ID, "docs: update task")
	if err != nil {
		t.Fatalf("AddTask() error = %v", err)
	}
	setValidContract(t, svc, cr.ID)

	if err := os.MkdirAll(filepath.Join(dir, "docs"), 0o755); err != nil {
		t.Fatalf("mkdir docs: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "docs", "in_scope.md"), []byte("ok\n"), 0o644); err != nil {
		t.Fatalf("write docs file: %v", err)
	}
	runGit(t, dir, "add", "docs/in_scope.md")
	runGit(t, dir, "commit", "-m", "docs: in scope")

	loaded, err := svc.store.LoadCR(cr.ID)
	if err != nil {
		t.Fatalf("LoadCR() error = %v", err)
	}
	loaded.Subtasks[task.ID-1].Status = "done"
	loaded.Subtasks[task.ID-1].CheckpointScope = []string{"outside/path.md"}
	if err := svc.store.SaveCR(loaded); err != nil {
		t.Fatalf("SaveCR() error = %v", err)
	}

	scope := []string{"docs"}
	if _, err := svc.SetCRContract(cr.ID, ContractPatch{Scope: &scope}); err != nil {
		t.Fatalf("SetCRContract(scope) error = %v", err)
	}

	report, err := svc.ValidateCR(cr.ID)
	if err != nil {
		t.Fatalf("ValidateCR() error = %v", err)
	}
	if len(report.Warnings) == 0 {
		t.Fatalf("expected task scope warning, got none")
	}
}

func TestValidateCRWarnsOnTaskContractScopeDrift(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")

	cr, err := svc.AddCR("Task contract drift", "warn on drift")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	task, err := svc.AddTask(cr.ID, "feat: drift task")
	if err != nil {
		t.Fatalf("AddTask() error = %v", err)
	}
	setValidContract(t, svc, cr.ID)
	setValidTaskContract(t, svc, cr.ID, task.ID)

	if err := os.WriteFile(filepath.Join(dir, "outside.txt"), []byte("x\n"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	loaded, err := svc.store.LoadCR(cr.ID)
	if err != nil {
		t.Fatalf("LoadCR() error = %v", err)
	}
	loaded.Subtasks[task.ID-1].Status = "done"
	loaded.Subtasks[task.ID-1].CheckpointScope = []string{"outside.txt"}
	loaded.Subtasks[task.ID-1].Contract.Scope = []string{"internal/service"}
	if err := svc.store.SaveCR(loaded); err != nil {
		t.Fatalf("SaveCR() error = %v", err)
	}

	report, err := svc.ValidateCR(cr.ID)
	if err != nil {
		t.Fatalf("ValidateCR() error = %v", err)
	}
	if !containsAny(report.Warnings, "outside task contract scope") {
		t.Fatalf("expected task contract drift warning, got %#v", report.Warnings)
	}
}

func TestValidateCRUsesCheckpointChunksWhenCheckpointScopeMissing(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")

	cr, err := svc.AddCR("Chunk fallback", "validate chunk-only checkpoint metadata")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	taskLegacy, err := svc.AddTask(cr.ID, "feat: legacy checkpoint task")
	if err != nil {
		t.Fatalf("AddTask(legacy) error = %v", err)
	}
	taskChunked, err := svc.AddTask(cr.ID, "feat: chunk checkpoint task")
	if err != nil {
		t.Fatalf("AddTask(chunked) error = %v", err)
	}
	setValidContract(t, svc, cr.ID)
	setValidTaskContract(t, svc, cr.ID, taskLegacy.ID)
	setValidTaskContract(t, svc, cr.ID, taskChunked.ID)

	loaded, err := svc.store.LoadCR(cr.ID)
	if err != nil {
		t.Fatalf("LoadCR() error = %v", err)
	}
	loaded.Subtasks[taskLegacy.ID-1].Status = model.TaskStatusDone
	loaded.Subtasks[taskLegacy.ID-1].CheckpointScope = []string{"internal/service/legacy.go"}

	loaded.Subtasks[taskChunked.ID-1].Status = model.TaskStatusDone
	loaded.Subtasks[taskChunked.ID-1].CheckpointScope = nil
	loaded.Subtasks[taskChunked.ID-1].CheckpointChunks = []model.CheckpointChunk{
		{
			ID:       "chk_1",
			Path:     "outside/chunk.go",
			OldStart: 2,
			OldLines: 1,
			NewStart: 2,
			NewLines: 1,
		},
	}
	loaded.Subtasks[taskChunked.ID-1].Contract.Scope = []string{"internal/service"}
	if err := svc.store.SaveCR(loaded); err != nil {
		t.Fatalf("SaveCR() error = %v", err)
	}

	report, err := svc.ValidateCR(cr.ID)
	if err != nil {
		t.Fatalf("ValidateCR() error = %v", err)
	}
	if !containsAny(report.Warnings, "outside task contract scope") {
		t.Fatalf("expected task contract scope warning from checkpoint_chunks fallback, got %#v", report.Warnings)
	}
}

func TestValidateCRWarnsOnInvalidCheckpointChunkMetadata(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")

	cr, err := svc.AddCR("Chunk validation", "warn on invalid chunk metadata")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	task, err := svc.AddTask(cr.ID, "feat: chunk validation task")
	if err != nil {
		t.Fatalf("AddTask() error = %v", err)
	}
	setValidContract(t, svc, cr.ID)
	setValidTaskContract(t, svc, cr.ID, task.ID)

	loaded, err := svc.store.LoadCR(cr.ID)
	if err != nil {
		t.Fatalf("LoadCR() error = %v", err)
	}
	loaded.Subtasks[task.ID-1].Status = model.TaskStatusDone
	loaded.Subtasks[task.ID-1].CheckpointScope = []string{"internal/service/chunk.go"}
	loaded.Subtasks[task.ID-1].CheckpointChunks = []model.CheckpointChunk{
		{
			ID:       "",
			Path:     "internal/service/chunk.go",
			OldStart: 0,
			OldLines: 1,
			NewStart: 1,
			NewLines: -1,
		},
		{
			ID:       "dup",
			Path:     "internal/service/chunk.go",
			OldStart: 2,
			OldLines: 1,
			NewStart: 2,
			NewLines: 1,
		},
		{
			ID:       "dup",
			Path:     "",
			OldStart: 3,
			OldLines: 1,
			NewStart: 3,
			NewLines: 1,
		},
	}
	if err := svc.store.SaveCR(loaded); err != nil {
		t.Fatalf("SaveCR() error = %v", err)
	}

	report, err := svc.ValidateCR(cr.ID)
	if err != nil {
		t.Fatalf("ValidateCR() error = %v", err)
	}
	if !containsAny(report.Warnings, "missing id") {
		t.Fatalf("expected missing-id warning, got %#v", report.Warnings)
	}
	if !containsAny(report.Warnings, "duplicate checkpoint chunk id") {
		t.Fatalf("expected duplicate-id warning, got %#v", report.Warnings)
	}
	if !containsAny(report.Warnings, "invalid line starts") {
		t.Fatalf("expected invalid line starts warning, got %#v", report.Warnings)
	}
	if !containsAny(report.Warnings, "invalid line counts") {
		t.Fatalf("expected invalid line counts warning, got %#v", report.Warnings)
	}
}
