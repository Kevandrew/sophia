package service

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func TestDiffCRDeterministicAndCriticalFilter(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")

	cr, err := svc.AddCR("Diff CR", "deterministic CR diff view")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	critical := []string{"critical"}
	if _, err := svc.SetCRContract(cr.ID, ContractPatch{RiskCriticalScopes: &critical}); err != nil {
		t.Fatalf("SetCRContract() error = %v", err)
	}

	if err := os.MkdirAll(filepath.Join(dir, "critical"), 0o755); err != nil {
		t.Fatalf("mkdir critical: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "other"), 0o755); err != nil {
		t.Fatalf("mkdir other: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "critical", "a.txt"), []byte("a\n"), 0o644); err != nil {
		t.Fatalf("write critical/a.txt: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "other", "b.txt"), []byte("b\n"), 0o644); err != nil {
		t.Fatalf("write other/b.txt: %v", err)
	}
	runGit(t, dir, "add", "critical/a.txt", "other/b.txt")
	runGit(t, dir, "commit", "-m", "feat: add deterministic diff fixtures")

	first, err := svc.DiffCR(cr.ID, CRDiffOptions{})
	if err != nil {
		t.Fatalf("DiffCR(first) error = %v", err)
	}
	second, err := svc.DiffCR(cr.ID, CRDiffOptions{})
	if err != nil {
		t.Fatalf("DiffCR(second) error = %v", err)
	}
	if first.FilesChanged != 2 || second.FilesChanged != 2 {
		t.Fatalf("expected 2 changed files, got first=%d second=%d", first.FilesChanged, second.FilesChanged)
	}
	if flattenDiffFiles(first.Files) != flattenDiffFiles(second.Files) {
		t.Fatalf("expected deterministic file/hunk ordering, got first=%q second=%q", flattenDiffFiles(first.Files), flattenDiffFiles(second.Files))
	}

	criticalView, err := svc.DiffCR(cr.ID, CRDiffOptions{CriticalOnly: true})
	if err != nil {
		t.Fatalf("DiffCR(critical) error = %v", err)
	}
	if criticalView.FilesChanged != 1 {
		t.Fatalf("expected one critical file, got %#v", criticalView)
	}
	if len(criticalView.Files) != 1 || criticalView.Files[0].Path != "critical/a.txt" {
		t.Fatalf("expected only critical/a.txt in critical view, got %#v", criticalView.Files)
	}
}

func TestDiffTaskCheckpointAndFallback(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")

	cr, err := svc.AddCR("Task diff", "task checkpoint + fallback")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	task, err := svc.AddTask(cr.ID, "feat: task diff fixture")
	if err != nil {
		t.Fatalf("AddTask() error = %v", err)
	}
	mustSetTaskContractForDiff(t, svc, cr.ID, task.ID, []string{"task.txt"})
	if err := os.WriteFile(filepath.Join(dir, "task.txt"), []byte("task\n"), 0o644); err != nil {
		t.Fatalf("write task.txt: %v", err)
	}
	if _, err := svc.DoneTaskWithCheckpoint(cr.ID, task.ID, DoneTaskOptions{Checkpoint: true, FromContract: true}); err != nil {
		t.Fatalf("DoneTaskWithCheckpoint() error = %v", err)
	}

	view, err := svc.DiffTask(cr.ID, task.ID, TaskDiffOptions{})
	if err != nil {
		t.Fatalf("DiffTask(checkpoint) error = %v", err)
	}
	if view.FallbackUsed {
		t.Fatalf("expected direct checkpoint diff, got fallback view %#v", view)
	}
	if view.FilesChanged == 0 {
		t.Fatalf("expected changed files in task diff, got %#v", view)
	}

	reloaded, err := svc.store.LoadCR(cr.ID)
	if err != nil {
		t.Fatalf("LoadCR() error = %v", err)
	}
	idx := indexOfTask(reloaded.Subtasks, task.ID)
	reloaded.Subtasks[idx].CheckpointCommit = ""
	if err := svc.store.SaveCR(reloaded); err != nil {
		t.Fatalf("SaveCR() error = %v", err)
	}

	fallbackView, err := svc.DiffTask(cr.ID, task.ID, TaskDiffOptions{})
	if err != nil {
		t.Fatalf("DiffTask(fallback) error = %v", err)
	}
	if !fallbackView.FallbackUsed {
		t.Fatalf("expected fallback diff, got %#v", fallbackView)
	}
	if strings.TrimSpace(fallbackView.FallbackReason) == "" {
		t.Fatalf("expected fallback reason, got %#v", fallbackView)
	}
}

func TestDiffTaskChunkMetadataAndDerivedFallback(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")

	if err := os.WriteFile(filepath.Join(dir, "chunked.txt"), []byte("alpha\nbeta\ngamma\n"), 0o644); err != nil {
		t.Fatalf("write base chunked.txt: %v", err)
	}
	runGit(t, dir, "add", "chunked.txt")
	runGit(t, dir, "commit", "-m", "chore: base chunk file")

	cr, err := svc.AddCR("Chunk diff", "task chunk diff fixture")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	task, err := svc.AddTask(cr.ID, "feat: chunked task")
	if err != nil {
		t.Fatalf("AddTask() error = %v", err)
	}
	mustSetTaskContractForDiff(t, svc, cr.ID, task.ID, []string{"chunked.txt"})

	if err := os.WriteFile(filepath.Join(dir, "chunked.txt"), []byte("alpha-1\nbeta\ngamma-1\n"), 0o644); err != nil {
		t.Fatalf("write updated chunked.txt: %v", err)
	}
	fullPatch := runGit(t, dir, "diff", "--unified=0", "chunked.txt")
	firstHunkPatch := firstHunkPatchFromDiff(t, fullPatch)
	patchPath := filepath.Join(dir, "task.patch")
	if err := os.WriteFile(patchPath, []byte(firstHunkPatch), 0o644); err != nil {
		t.Fatalf("write patch file: %v", err)
	}
	if _, err := svc.DoneTaskWithCheckpoint(cr.ID, task.ID, DoneTaskOptions{Checkpoint: true, PatchFile: patchPath}); err != nil {
		t.Fatalf("DoneTaskWithCheckpoint(patch) error = %v", err)
	}

	tasks, err := svc.ListTasks(cr.ID)
	if err != nil {
		t.Fatalf("ListTasks() error = %v", err)
	}
	idx := indexOfTask(tasks, task.ID)
	if idx < 0 || len(tasks[idx].CheckpointChunks) == 0 {
		t.Fatalf("expected checkpoint chunk metadata, got %#v", tasks)
	}
	metaChunkID := tasks[idx].CheckpointChunks[0].ID
	metaView, err := svc.DiffTaskChunk(cr.ID, task.ID, metaChunkID)
	if err != nil {
		t.Fatalf("DiffTaskChunk(metadata) error = %v", err)
	}
	if metaView.FilesChanged != 1 {
		t.Fatalf("expected one file in metadata chunk diff, got %#v", metaView)
	}

	taskView, err := svc.DiffTask(cr.ID, task.ID, TaskDiffOptions{})
	if err != nil {
		t.Fatalf("DiffTask() error = %v", err)
	}
	if len(taskView.Files) == 0 || len(taskView.Files[0].Hunks) == 0 {
		t.Fatalf("expected task diff hunks for derived chunk id, got %#v", taskView)
	}
	derivedChunkID := taskView.Files[0].Hunks[0].ChunkID

	reloaded, err := svc.store.LoadCR(cr.ID)
	if err != nil {
		t.Fatalf("LoadCR() error = %v", err)
	}
	reloadedTaskIdx := indexOfTask(reloaded.Subtasks, task.ID)
	reloaded.Subtasks[reloadedTaskIdx].CheckpointChunks = nil
	if err := svc.store.SaveCR(reloaded); err != nil {
		t.Fatalf("SaveCR() error = %v", err)
	}

	derivedView, err := svc.DiffTaskChunk(cr.ID, task.ID, derivedChunkID)
	if err != nil {
		t.Fatalf("DiffTaskChunk(derived) error = %v", err)
	}
	if !derivedView.FallbackUsed {
		t.Fatalf("expected chunk derived fallback, got %#v", derivedView)
	}
	if strings.TrimSpace(derivedView.FallbackReason) == "" {
		t.Fatalf("expected derived fallback reason, got %#v", derivedView)
	}
}

func TestDiffTaskFallbackRequiresCheckpointScope(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")

	cr, err := svc.AddCR("Task fallback error", "require checkpoint scope")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	task, err := svc.AddTask(cr.ID, "feat: no checkpoint")
	if err != nil {
		t.Fatalf("AddTask() error = %v", err)
	}
	mustSetTaskContractForDiff(t, svc, cr.ID, task.ID, []string{"missing.txt"})
	if _, err := svc.DoneTaskWithCheckpoint(cr.ID, task.ID, DoneTaskOptions{
		Checkpoint:         false,
		NoCheckpointReason: "metadata-only fallback test",
	}); err != nil {
		t.Fatalf("DoneTaskWithCheckpoint(no checkpoint) error = %v", err)
	}

	if _, err := svc.DiffTask(cr.ID, task.ID, TaskDiffOptions{}); err == nil || !strings.Contains(err.Error(), "checkpoint_scope") {
		t.Fatalf("expected checkpoint_scope fallback error, got %v", err)
	}
}

func flattenDiffFiles(files []DiffFileView) string {
	parts := make([]string, 0)
	for _, file := range files {
		chunks := make([]string, 0, len(file.Hunks))
		for _, hunk := range file.Hunks {
			chunks = append(chunks, hunk.ChunkID)
		}
		sort.Strings(chunks)
		parts = append(parts, file.Path+":"+strings.Join(chunks, ","))
	}
	sort.Strings(parts)
	return strings.Join(parts, "|")
}

func mustSetTaskContractForDiff(t *testing.T, svc *Service, crID, taskID int, scope []string) {
	t.Helper()
	intent := "task diff contract"
	acceptance := []string{"task diff acceptance"}
	if _, err := svc.SetTaskContract(crID, taskID, TaskContractPatch{
		Intent:             &intent,
		AcceptanceCriteria: &acceptance,
		Scope:              &scope,
	}); err != nil {
		t.Fatalf("SetTaskContract(cr=%d task=%d) error = %v", crID, taskID, err)
	}
}
