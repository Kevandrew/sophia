package service

import (
	"path/filepath"
	"testing"
	"time"

	"sophia/internal/gitx"
	"sophia/internal/model"
	"sophia/internal/store"
)

type runtimeHarnessOptions struct {
	RepoRoot string
	Now      time.Time
	Actor    string
	Branch   string
	Config   model.Config
	Index    model.Index
	CRs      []*model.CR
}

type runtimeHarness struct {
	Service      *Service
	Store        *fakeCRStore
	LifecycleGit *fakeLifecycleGit
	StatusGit    *fakeStatusGit
	MergeGit     *fakeMergeGit
	TaskGit      *fakeTaskGit
	Now          time.Time
	Actor        string
	Branch       string
}

func harnessService(t *testing.T, opts runtimeHarnessOptions) *runtimeHarness {
	t.Helper()

	repoRoot := opts.RepoRoot
	if repoRoot == "" {
		repoRoot = t.TempDir()
	}
	now := opts.Now
	if now.IsZero() {
		now = harnessNow()
	}
	actor := opts.Actor
	if actor == "" {
		actor = "Runtime Tester <runtime@test>"
	}
	branch := opts.Branch
	if branch == "" {
		branch = "cr-runtime-harness"
	}

	fakeStore := newFakeCRStore()
	fakeStore.sophiaDir = filepath.Join(repoRoot, ".sophia-harness")
	if opts.Config.Version != "" || opts.Config.BaseBranch != "" || opts.Config.MetadataMode != "" {
		fakeStore.config = opts.Config
		if fakeStore.config.Version == "" {
			fakeStore.config.Version = "v0"
		}
		if fakeStore.config.BaseBranch == "" {
			fakeStore.config.BaseBranch = "main"
		}
		if fakeStore.config.MetadataMode == "" {
			fakeStore.config.MetadataMode = model.MetadataModeLocal
		}
	}
	if opts.Index.NextID > 0 {
		fakeStore.index = opts.Index
	}
	for _, cr := range opts.CRs {
		fakeStore.SeedCR(cr)
	}

	lifecycleGit := newFakeLifecycleGit(actor, branch)
	lifecycleGit.branchExists[branch] = true
	lifecycleGit.branchExists["main"] = true
	lifecycleGit.resolve["HEAD"] = "head-sha"
	lifecycleGit.resolve[branch] = "head-sha"
	lifecycleGit.resolve["main"] = "base-sha"

	statusGit := newFakeStatusGit(actor, branch)
	statusGit.resolve["HEAD"] = "head-sha"
	statusGit.resolve[branch] = "head-sha"
	statusGit.resolve["main"] = "base-sha"

	mergeGit := newFakeMergeGit(actor, branch)
	mergeGit.branchExists[branch] = true
	mergeGit.branchExists["main"] = true
	mergeGit.resolve["HEAD"] = "head-sha"
	mergeGit.resolve[branch] = "head-sha"
	mergeGit.resolve["main"] = "base-sha"

	taskGit := newFakeTaskGit(actor, branch)
	taskGit.headShortSHA = "head-sha"

	svc := &Service{
		store:    store.NewWithSophiaRoot(repoRoot, filepath.Join(repoRoot, ".sophia-harness-real")),
		git:      gitx.New(repoRoot),
		repoRoot: repoRoot,
		now:      func() time.Time { return now },
	}
	svc.overrideLifecycleRuntimeProvidersForTests(lifecycleGit, fakeStore)
	svc.overrideStatusRuntimeProvidersForTests(statusGit, fakeStore)
	svc.overrideMergeRuntimeProvidersForTests(mergeGit, fakeStore, func(root string) mergeRuntimeGit {
		fork := mergeGit.clone()
		if root != "" {
			fork.currentBranch = branch
		}
		return fork
	})
	svc.overrideTaskRuntimeProvidersForTests(taskGit, fakeStore)
	svc.overrideTaskMergeGuardForTests(func(*model.CR) error { return nil })

	return &runtimeHarness{
		Service:      svc,
		Store:        fakeStore,
		LifecycleGit: lifecycleGit,
		StatusGit:    statusGit,
		MergeGit:     mergeGit,
		TaskGit:      taskGit,
		Now:          now,
		Actor:        actor,
		Branch:       branch,
	}
}

func TestHarnessServiceWiresRuntimeProviders(t *testing.T) {
	t.Parallel()
	h := harnessService(t, runtimeHarnessOptions{})

	if got := h.Service.activeLifecycleStoreProvider(); got != h.Store {
		t.Fatalf("expected lifecycle store provider to be harness store")
	}
	if got := h.Service.activeStatusStoreProvider(); got != h.Store {
		t.Fatalf("expected status store provider to be harness store")
	}
	if got := h.Service.activeMergeStoreProvider(); got != h.Store {
		t.Fatalf("expected merge store provider to be harness store")
	}
	if got := h.Service.activeTaskStoreProvider(); got != h.Store {
		t.Fatalf("expected task store provider to be harness store")
	}

	if got := h.Service.activeLifecycleGitProvider(); got != h.LifecycleGit {
		t.Fatalf("expected lifecycle git provider to be harness fake")
	}
	if got := h.Service.activeStatusGitProvider(); got != h.StatusGit {
		t.Fatalf("expected status git provider to be harness fake")
	}
	if got := h.Service.activeMergeGitProvider(); got != h.MergeGit {
		t.Fatalf("expected merge git provider to be harness fake")
	}
	if got := h.Service.activeTaskGitProvider(); got != h.TaskGit {
		t.Fatalf("expected task git provider to be harness fake")
	}
}

func TestHarnessServiceMergeFactoryReturnsIndependentAdapter(t *testing.T) {
	t.Parallel()
	h := harnessService(t, runtimeHarnessOptions{})
	factory := h.Service.activeMergeGitFactory()
	if factory == nil {
		t.Fatalf("expected non-nil merge git factory")
	}
	first, ok := factory(t.TempDir()).(*fakeMergeGit)
	if !ok {
		t.Fatalf("expected fake merge git, got %T", factory(t.TempDir()))
	}
	second, ok := factory(t.TempDir()).(*fakeMergeGit)
	if !ok {
		t.Fatalf("expected fake merge git, got %T", factory(t.TempDir()))
	}
	if first == second {
		t.Fatalf("expected independent factory instances")
	}
	if first.actor != h.Actor || second.actor != h.Actor {
		t.Fatalf("expected factory clones to preserve actor")
	}
}
