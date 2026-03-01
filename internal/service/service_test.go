package service

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"sophia/internal/model"
)

func TestInitInNonGitDirectoryInitializesGitAndSophia(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := New(dir)

	base, err := svc.Init("main", "")
	if err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if base != "main" {
		t.Fatalf("expected base branch main, got %q", base)
	}

	if _, err := os.Stat(filepath.Join(dir, ".git")); err != nil {
		t.Fatalf("expected .git to exist: %v", err)
	}
	metadataDir := localMetadataDir(t, dir)
	if _, err := os.Stat(filepath.Join(metadataDir, "config.yaml")); err != nil {
		t.Fatalf("expected shared metadata config to exist: %v", err)
	}
}

func TestInitIsIdempotentInExistingRepo(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	runGit(t, dir, "init")

	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("first Init() error = %v", err)
	}
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("second Init() error = %v", err)
	}

	idx, err := svc.store.LoadIndex()
	if err != nil {
		t.Fatalf("LoadIndex() error = %v", err)
	}
	if idx.NextID != 1 {
		t.Fatalf("expected next id 1 after idempotent init, got %d", idx.NextID)
	}
}

func TestInitDefaultsToLocalMetadataAndGitIgnoreEntry(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := New(dir)

	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	cfg, err := svc.store.LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if cfg.MetadataMode != "local" {
		t.Fatalf("expected metadata_mode local, got %q", cfg.MetadataMode)
	}
	gitignore, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
	if err != nil {
		t.Fatalf("read .gitignore: %v", err)
	}
	if !strings.Contains(string(gitignore), ".sophia/") {
		t.Fatalf("expected .gitignore to include .sophia/")
	}

	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("second Init() error = %v", err)
	}
	gitignore2, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
	if err != nil {
		t.Fatalf("read .gitignore after second init: %v", err)
	}
	if strings.Count(string(gitignore2), ".sophia/") != 1 {
		t.Fatalf("expected single .sophia/ entry, got:\n%s", string(gitignore2))
	}
}

func TestAddCRAlignsNextIDWithHistory(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	// Simulate existing merged CR history in Git while local index is stale.
	runGit(t, dir, "-c", "user.name=Test User", "-c", "user.email=test@example.com", "commit", "--allow-empty", "-m", "[CR-4] Existing merged intent", "-m", "Sophia-CR: 4\nSophia-Intent: Existing merged intent\nSophia-Tasks: 0 completed")
	if err := svc.store.SaveIndex(model.Index{NextID: 1}); err != nil {
		t.Fatalf("SaveIndex() error = %v", err)
	}

	cr, err := svc.AddCR("New intent", "should pick id 5")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	if cr.ID != 5 {
		t.Fatalf("expected CR id 5, got %d", cr.ID)
	}
	if ok, _ := regexp.MatchString(`^cr-new-intent-(?:[a-z0-9]{4}|[a-z0-9]{6}|[a-z0-9]{8})$`, cr.Branch); !ok {
		t.Fatalf("expected uid-suffixed branch for CR 5, got %q", cr.Branch)
	}
}

func TestAddCRCreatesBranchAndCRFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	cr, err := svc.AddCR("Bootstrap", "Scaffold CLI")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	if cr.ID != 1 {
		t.Fatalf("expected CR id 1, got %d", cr.ID)
	}
	if ok, _ := regexp.MatchString(`^cr-bootstrap-(?:[a-z0-9]{4}|[a-z0-9]{6}|[a-z0-9]{8})$`, cr.Branch); !ok {
		t.Fatalf("unexpected branch %q", cr.Branch)
	}
	if strings.TrimSpace(cr.UID) == "" {
		t.Fatalf("expected CR uid to be assigned, got %#v", cr)
	}
	if cr.BaseRef != "main" {
		t.Fatalf("expected base ref main, got %q", cr.BaseRef)
	}
	if strings.TrimSpace(cr.BaseCommit) == "" {
		t.Fatalf("expected base commit to be assigned, got %#v", cr)
	}

	branch, err := svc.git.CurrentBranch()
	if err != nil {
		t.Fatalf("CurrentBranch() error = %v", err)
	}
	if branch != cr.Branch {
		t.Fatalf("expected current branch %q, got %q", cr.Branch, branch)
	}

	loaded, err := svc.store.LoadCR(1)
	if err != nil {
		t.Fatalf("LoadCR() error = %v", err)
	}
	if loaded.Title != "Bootstrap" || len(loaded.Events) == 0 || loaded.Events[0].Type != model.EventTypeCRCreated {
		t.Fatalf("unexpected loaded CR: %#v", loaded)
	}
	if loaded.UID != cr.UID {
		t.Fatalf("expected persisted uid %q, got %q", cr.UID, loaded.UID)
	}
	if loaded.BaseRef != cr.BaseRef || loaded.BaseCommit != cr.BaseCommit {
		t.Fatalf("expected persisted base fields, got %#v", loaded)
	}
}

func TestAddCRAssignsDistinctUIDs(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	first, err := svc.AddCR("First", "uid one")
	if err != nil {
		t.Fatalf("AddCR(first) error = %v", err)
	}
	second, err := svc.AddCR("Second", "uid two")
	if err != nil {
		t.Fatalf("AddCR(second) error = %v", err)
	}

	if strings.TrimSpace(first.UID) == "" || strings.TrimSpace(second.UID) == "" {
		t.Fatalf("expected non-empty uids, got first=%q second=%q", first.UID, second.UID)
	}
	if first.UID == second.UID {
		t.Fatalf("expected distinct uids, got %q", first.UID)
	}
}

func TestAddCRWithExplicitBaseRef(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	runGit(t, dir, "checkout", "-b", "release")
	if err := os.WriteFile(filepath.Join(dir, "release_base.txt"), []byte("release\n"), 0o644); err != nil {
		t.Fatalf("write release base file: %v", err)
	}
	runGit(t, dir, "add", "release_base.txt")
	runGit(t, dir, "-c", "user.name=Test User", "-c", "user.email=test@example.com", "commit", "-m", "feat: release base")
	runGit(t, dir, "checkout", "-B", "main")

	result, err := svc.AddCRWithOptions("Release-based", "base ref", AddCROptions{BaseRef: "release"})
	if err != nil {
		t.Fatalf("AddCRWithOptions() error = %v", err)
	}
	cr := result.CR
	if result.Warnings == nil {
		t.Fatalf("expected warnings slice to be non-nil")
	}
	if cr.BaseRef != "release" {
		t.Fatalf("expected base ref release, got %q", cr.BaseRef)
	}
	releaseHead, err := svc.git.ResolveRef("release")
	if err != nil {
		t.Fatalf("ResolveRef(release) error = %v", err)
	}
	if cr.BaseCommit != releaseHead {
		t.Fatalf("expected base commit %q, got %q", releaseHead, cr.BaseCommit)
	}
	if _, statErr := os.Stat(filepath.Join(dir, "release_base.txt")); statErr != nil {
		t.Fatalf("expected CR branch from release base to contain file: %v", statErr)
	}
}

func TestAddChildCRUsesParentAnchor(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	parent, err := svc.AddCR("Parent", "base for child")
	if err != nil {
		t.Fatalf("AddCR(parent) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "parent.txt"), []byte("parent\n"), 0o644); err != nil {
		t.Fatalf("write parent file: %v", err)
	}
	runGit(t, dir, "add", "parent.txt")
	runGit(t, dir, "-c", "user.name=Test User", "-c", "user.email=test@example.com", "commit", "-m", "feat: parent work")
	parentHead, err := svc.git.ResolveRef(parent.Branch)
	if err != nil {
		t.Fatalf("ResolveRef(parent branch) error = %v", err)
	}

	childResult, err := svc.AddCRWithOptions("Child", "stacked", AddCROptions{ParentCRID: parent.ID})
	if err != nil {
		t.Fatalf("AddCRWithOptions(child) error = %v", err)
	}
	child := childResult.CR
	if child.ParentCRID != parent.ID {
		t.Fatalf("expected parent id %d, got %d", parent.ID, child.ParentCRID)
	}
	if child.BaseRef != parent.Branch {
		t.Fatalf("expected child base_ref %q, got %q", parent.Branch, child.BaseRef)
	}
	if child.BaseCommit != parentHead {
		t.Fatalf("expected child base_commit %q, got %q", parentHead, child.BaseCommit)
	}
}

func TestMergeChildBlockedUntilParentMergedAndParentMergeBackfillsChildBase(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	parent, err := svc.AddCR("Parent merge", "must merge first")
	if err != nil {
		t.Fatalf("AddCR(parent) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "parent_merge.txt"), []byte("parent\n"), 0o644); err != nil {
		t.Fatalf("write parent file: %v", err)
	}
	runGit(t, dir, "add", "parent_merge.txt")
	runGit(t, dir, "-c", "user.name=Test User", "-c", "user.email=test@example.com", "commit", "-m", "feat: parent merge")
	setValidContract(t, svc, parent.ID)

	child, _, err := svc.AddCRWithOptionsWithWarnings("Child merge", "depends on parent", AddCROptions{ParentCRID: parent.ID})
	if err != nil {
		t.Fatalf("AddCRWithOptionsWithWarnings(child) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "child_merge.txt"), []byte("child\n"), 0o644); err != nil {
		t.Fatalf("write child file: %v", err)
	}
	runGit(t, dir, "add", "child_merge.txt")
	runGit(t, dir, "-c", "user.name=Test User", "-c", "user.email=test@example.com", "commit", "-m", "feat: child merge")
	setValidContract(t, svc, child.ID)

	if _, err := svc.MergeCR(child.ID, false, ""); !errors.Is(err, ErrParentCRNotMerged) {
		t.Fatalf("expected ErrParentCRNotMerged, got %v", err)
	}

	if _, err := svc.SwitchCR(parent.ID); err != nil {
		t.Fatalf("SwitchCR(parent) error = %v", err)
	}
	if _, err := svc.MergeCR(parent.ID, false, ""); err != nil {
		t.Fatalf("MergeCR(parent) error = %v", err)
	}

	updatedChild, err := svc.store.LoadCR(child.ID)
	if err != nil {
		t.Fatalf("LoadCR(child) error = %v", err)
	}
	if updatedChild.BaseRef != updatedChild.BaseBranch {
		t.Fatalf("expected child base_ref to reset to base branch, got %q", updatedChild.BaseRef)
	}
	if strings.TrimSpace(updatedChild.BaseCommit) == "" {
		t.Fatalf("expected child base_commit backfilled from parent merge")
	}
}

func TestSetCRBaseAndRestack(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	runGit(t, dir, "checkout", "-b", "release")
	if err := os.WriteFile(filepath.Join(dir, "release_stack.txt"), []byte("release\n"), 0o644); err != nil {
		t.Fatalf("write release stack file: %v", err)
	}
	runGit(t, dir, "add", "release_stack.txt")
	runGit(t, dir, "-c", "user.name=Test User", "-c", "user.email=test@example.com", "commit", "-m", "feat: release stack")
	runGit(t, dir, "checkout", "-B", "main")

	cr, err := svc.AddCR("Base set", "retarget base")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	updated, err := svc.SetCRBase(cr.ID, "release", false)
	if err != nil {
		t.Fatalf("SetCRBase() error = %v", err)
	}
	if updated.BaseRef != "release" || strings.TrimSpace(updated.BaseCommit) == "" {
		t.Fatalf("unexpected SetCRBase result %#v", updated)
	}

	parent, err := svc.AddCR("Restack parent", "parent")
	if err != nil {
		t.Fatalf("AddCR(parent) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "restack_parent.txt"), []byte("p1\n"), 0o644); err != nil {
		t.Fatalf("write restack parent file: %v", err)
	}
	runGit(t, dir, "add", "restack_parent.txt")
	runGit(t, dir, "-c", "user.name=Test User", "-c", "user.email=test@example.com", "commit", "-m", "feat: restack parent 1")

	child, _, err := svc.AddCRWithOptionsWithWarnings("Restack child", "child", AddCROptions{ParentCRID: parent.ID})
	if err != nil {
		t.Fatalf("AddCRWithOptionsWithWarnings(child) error = %v", err)
	}
	if _, err := svc.SwitchCR(parent.ID); err != nil {
		t.Fatalf("SwitchCR(parent) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "restack_parent_2.txt"), []byte("p2\n"), 0o644); err != nil {
		t.Fatalf("write restack parent second file: %v", err)
	}
	runGit(t, dir, "add", "restack_parent_2.txt")
	runGit(t, dir, "-c", "user.name=Test User", "-c", "user.email=test@example.com", "commit", "-m", "feat: restack parent 2")

	if _, err := svc.RestackCR(child.ID); err != nil {
		t.Fatalf("RestackCR() error = %v", err)
	}
	reloadedChild, err := svc.store.LoadCR(child.ID)
	if err != nil {
		t.Fatalf("LoadCR(child) error = %v", err)
	}
	parentHead, err := svc.git.ResolveRef(parent.Branch)
	if err != nil {
		t.Fatalf("ResolveRef(parent branch) error = %v", err)
	}
	if reloadedChild.BaseRef != parent.Branch || reloadedChild.BaseCommit != parentHead {
		t.Fatalf("expected child restacked onto parent head, got %#v", reloadedChild)
	}
}

func TestNoteAppendsAndUpdatesCR(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if _, err := svc.AddCR("Bootstrap", "Scaffold CLI"); err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}

	if err := svc.AddNote(1, "Refactored payment client"); err != nil {
		t.Fatalf("AddNote() error = %v", err)
	}

	cr, err := svc.store.LoadCR(1)
	if err != nil {
		t.Fatalf("LoadCR() error = %v", err)
	}
	if len(cr.Notes) != 1 || cr.Notes[0] != "Refactored payment client" {
		t.Fatalf("unexpected notes: %#v", cr.Notes)
	}
	if got := cr.Events[len(cr.Events)-1].Type; got != model.EventTypeNoteAdded {
		t.Fatalf("expected last event note_added, got %q", got)
	}
}
