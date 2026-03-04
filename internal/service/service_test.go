package service

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"

	"sophia/internal/gitx"
	"sophia/internal/model"
)

// integration-required: verifies real repo initialization and on-disk metadata creation.
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

// integration-required: validates idempotent init behavior against a real git repo.
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

// integration-required: asserts filesystem-level defaults and .gitignore writes.
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

// integration-required: verifies AddCR can lazily bootstrap metadata without explicit init.
func TestAddCRBootstrapsLocalMetadataInInitializedGitRepo(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	runGit(t, dir, "init", "-b", "main")

	svc := New(dir)
	result, err := svc.AddCRWithOptions("No init CR", "bootstrap on first add", AddCROptions{NoSwitch: true})
	if err != nil {
		t.Fatalf("AddCRWithOptions() error = %v", err)
	}
	if !result.Bootstrap.Triggered {
		t.Fatalf("expected bootstrap to be triggered, got %#v", result.Bootstrap)
	}
	if result.Bootstrap.MetadataMode != model.MetadataModeLocal {
		t.Fatalf("expected bootstrap metadata mode %q, got %q", model.MetadataModeLocal, result.Bootstrap.MetadataMode)
	}
	if result.Bootstrap.BaseBranch != "main" {
		t.Fatalf("expected bootstrap base branch main, got %q", result.Bootstrap.BaseBranch)
	}
	if _, err := os.Stat(filepath.Join(localMetadataDir(t, dir), "config.yaml")); err != nil {
		t.Fatalf("expected local metadata config to exist: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".sophia", "config.yaml")); !os.IsNotExist(err) {
		t.Fatalf("expected legacy tracked metadata to be absent, err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "SOPHIA.yaml")); !os.IsNotExist(err) {
		t.Fatalf("expected SOPHIA.yaml to be absent, err=%v", err)
	}
}

// integration-required: ensures bootstrap signal is only emitted on first add after lazy init.
func TestAddCRLazyBootstrapIsIdempotentAcrossSubsequentAdds(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	runGit(t, dir, "init", "-b", "main")

	svc := New(dir)
	first, err := svc.AddCRWithOptions("First no-init", "bootstrap", AddCROptions{NoSwitch: true})
	if err != nil {
		t.Fatalf("AddCRWithOptions(first) error = %v", err)
	}
	second, err := svc.AddCRWithOptions("Second no-init", "no re-bootstrap", AddCROptions{NoSwitch: true})
	if err != nil {
		t.Fatalf("AddCRWithOptions(second) error = %v", err)
	}
	if !first.Bootstrap.Triggered {
		t.Fatalf("expected first AddCR to trigger bootstrap, got %#v", first.Bootstrap)
	}
	if second.Bootstrap.Triggered {
		t.Fatalf("expected second AddCR not to trigger bootstrap, got %#v", second.Bootstrap)
	}
}

// fake-eligible: lifecycle ID-floor logic only; no real git plumbing semantics required.
func TestAddCRAlignsNextIDWithHistory(t *testing.T) {
	t.Parallel()
	h := harnessService(t, runtimeHarnessOptions{
		Branch: "main",
		Index:  model.Index{NextID: 1},
	})
	h.LifecycleGit.SeedLocalBranches("main")
	h.LifecycleGit.SeedRecentCommits(
		gitx.Commit{
			Subject: "[CR-4] Existing merged intent",
			Body:    "Sophia-CR: 4\nSophia-Intent: Existing merged intent\nSophia-Tasks: 0 completed",
		},
	)

	cr, err := h.Service.AddCR("New intent", "should pick id 5")
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

// integration-required: asserts real branch checkout and persisted CR YAML behavior.
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

// integration-required: keeps end-to-end AddCR flow coverage with real runtime wiring.
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

func TestAddCRWarningParityAcrossWrappers(t *testing.T) {
	t.Parallel()
	runCase := func(t *testing.T, name string, run func(*Service) ([]string, error)) []string {
		t.Helper()
		dir := t.TempDir()
		svc := New(dir)
		if _, err := svc.Init("main", ""); err != nil {
			t.Fatalf("Init() error = %v", err)
		}
		runGit(t, dir, "config", "user.name", "Test User")
		runGit(t, dir, "config", "user.email", "test@example.com")

		if _, err := svc.AddCR("Billing CR", "billing work"); err != nil {
			t.Fatalf("AddCR() error = %v", err)
		}
		if err := os.MkdirAll(filepath.Join(dir, "billing"), 0o755); err != nil {
			t.Fatalf("mkdir billing: %v", err)
		}
		if err := os.WriteFile(filepath.Join(dir, "billing", "a.txt"), []byte("a\n"), 0o644); err != nil {
			t.Fatalf("write billing/a.txt: %v", err)
		}
		runGit(t, dir, "add", "billing/a.txt")
		runGit(t, dir, "commit", "-m", "feat: billing a")

		runGit(t, dir, "checkout", "main")
		runGit(t, dir, "checkout", "-b", "exploratory")
		if err := os.MkdirAll(filepath.Join(dir, "billing"), 0o755); err != nil {
			t.Fatalf("mkdir billing on exploratory: %v", err)
		}
		if err := os.WriteFile(filepath.Join(dir, "billing", "b.txt"), []byte("b\n"), 0o644); err != nil {
			t.Fatalf("write billing/b.txt: %v", err)
		}
		runGit(t, dir, "add", "billing/b.txt")
		runGit(t, dir, "commit", "-m", "feat: exploratory billing")

		warnings, err := run(svc)
		if err != nil {
			t.Fatalf("%s() error = %v", name, err)
		}
		if len(warnings) == 0 {
			t.Fatalf("%s() expected warnings", name)
		}
		joined := strings.Join(warnings, "\n")
		if !strings.Contains(joined, "CR-1") || !strings.Contains(joined, "/billing") {
			t.Fatalf("%s() unexpected warnings: %#v", name, warnings)
		}
		out := append([]string(nil), warnings...)
		sort.Strings(out)
		return out
	}

	optionsWarnings := runCase(t, "AddCRWithOptions", func(svc *Service) ([]string, error) {
		result, err := svc.AddCRWithOptions("New billing CR", "another billing change", AddCROptions{})
		if err != nil {
			return nil, err
		}
		return result.Warnings, nil
	})
	optionsWithWarningsWarnings := runCase(t, "AddCRWithOptionsWithWarnings", func(svc *Service) ([]string, error) {
		_, warnings, err := svc.AddCRWithOptionsWithWarnings("New billing CR", "another billing change", AddCROptions{})
		if err != nil {
			return nil, err
		}
		return warnings, nil
	})
	withWarnings := runCase(t, "AddCRWithWarnings", func(svc *Service) ([]string, error) {
		_, warnings, err := svc.AddCRWithWarnings("New billing CR", "another billing change")
		if err != nil {
			return nil, err
		}
		return warnings, nil
	})

	if !reflect.DeepEqual(optionsWarnings, optionsWithWarningsWarnings) {
		t.Fatalf("warnings mismatch between AddCRWithOptions and AddCRWithOptionsWithWarnings:\noptions=%#v\nwith_warnings=%#v", optionsWarnings, optionsWithWarningsWarnings)
	}
	if !reflect.DeepEqual(optionsWarnings, withWarnings) {
		t.Fatalf("warnings mismatch between AddCRWithOptions and AddCRWithWarnings:\noptions=%#v\nwith_warnings=%#v", optionsWarnings, withWarnings)
	}
}

// fake-eligible: base-ref resolution and lifecycle metadata decisions are runtime-logic only.
func TestAddCRWithExplicitBaseRef(t *testing.T) {
	t.Parallel()
	h := harnessService(t, runtimeHarnessOptions{Branch: "main"})
	h.LifecycleGit.SeedBranch("release", true)
	h.LifecycleGit.SeedResolve("release", "release-head-sha")

	result, err := h.Service.AddCRWithOptions("Release-based", "base ref", AddCROptions{BaseRef: "release"})
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
	releaseHead := "release-head-sha"
	if cr.BaseCommit != releaseHead {
		t.Fatalf("expected base commit %q, got %q", releaseHead, cr.BaseCommit)
	}
}

// fake-eligible: parent-anchor selection is a lifecycle decision, not git command fidelity.
func TestAddChildCRUsesParentAnchor(t *testing.T) {
	t.Parallel()
	h := harnessService(t, runtimeHarnessOptions{Branch: "main"})
	parent, err := h.Service.AddCR("Parent", "base for child")
	if err != nil {
		t.Fatalf("AddCR(parent) error = %v", err)
	}
	h.LifecycleGit.SeedResolve(parent.Branch, "parent-head-sha")
	parentHead := "parent-head-sha"

	childResult, err := h.Service.AddCRWithOptions("Child", "stacked", AddCROptions{ParentCRID: parent.ID})
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

// integration-required: merge gate + parent merge backfill relies on real merge/runtime behavior.
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

// fake-eligible: SetCRBase/Restack branch-target decisions are validated via runtime fakes.
func TestSetCRBaseAndRestack(t *testing.T) {
	t.Parallel()
	baseCR := seedCR(1, "Base set", seedCROptions{
		Branch:     "cr-base-set",
		BaseBranch: "main",
		BaseRef:    "main",
		BaseCommit: "main-head-sha",
	})
	parent := seedCR(2, "Restack parent", seedCROptions{
		Branch:     "cr-restack-parent",
		BaseBranch: "main",
		BaseRef:    "main",
		BaseCommit: "main-head-sha",
	})
	child := seedCR(3, "Restack child", seedCROptions{
		Branch:     "cr-restack-child",
		BaseBranch: "main",
		BaseRef:    parent.Branch,
		BaseCommit: "parent-head-old",
		ParentCRID: parent.ID,
	})
	h := harnessService(t, runtimeHarnessOptions{
		Branch: "main",
		CRs:    []*model.CR{baseCR, parent, child},
	})
	h.LifecycleGit.SeedBranch("release", true)
	h.LifecycleGit.SeedBranch(baseCR.Branch, true)
	h.LifecycleGit.SeedBranch(parent.Branch, true)
	h.LifecycleGit.SeedBranch(child.Branch, true)
	h.LifecycleGit.SeedResolve("release", "release-head-sha")
	h.LifecycleGit.SeedResolve(parent.Branch, "parent-head-new")

	updated, err := h.Service.SetCRBase(baseCR.ID, "release", false)
	if err != nil {
		t.Fatalf("SetCRBase() error = %v", err)
	}
	if updated.BaseRef != "release" || updated.BaseCommit != "release-head-sha" {
		t.Fatalf("unexpected SetCRBase result %#v", updated)
	}
	if h.MergeGit.Calls("RebaseBranchOnto") != 0 {
		t.Fatalf("expected SetCRBase(rebase=false) not to rebase")
	}
	if _, err := h.Service.RestackCR(child.ID); err != nil {
		t.Fatalf("RestackCR() error = %v", err)
	}
	reloadedChild, err := h.Store.LoadCR(child.ID)
	if err != nil {
		t.Fatalf("LoadCR(child) error = %v", err)
	}
	if reloadedChild.BaseRef != parent.Branch || reloadedChild.BaseCommit != "parent-head-new" {
		t.Fatalf("expected child restacked onto parent head, got %#v", reloadedChild)
	}
	if h.MergeGit.Calls("RebaseBranchOnto") != 1 {
		t.Fatalf("expected one rebase call from restack, got %d", h.MergeGit.Calls("RebaseBranchOnto"))
	}
}

// integration-required: keeps end-to-end CR persistence/event append coverage.
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
