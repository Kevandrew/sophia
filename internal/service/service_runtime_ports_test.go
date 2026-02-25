package service

import (
	"path/filepath"
	"sophia/internal/gitx"
	"sophia/internal/store"
	"testing"
)

func TestRuntimeProvidersDefaultToServiceDependencies(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)

	if got := svc.activeLifecycleStoreProvider(); got != svc.store {
		t.Fatalf("expected lifecycle store default to service store")
	}
	if got := svc.activeLifecycleGitProvider(); got != svc.git {
		t.Fatalf("expected lifecycle git default to service git")
	}
	if got := svc.activeStatusStoreProvider(); got != svc.store {
		t.Fatalf("expected status store default to service store")
	}
	if got := svc.activeStatusGitProvider(); got != svc.git {
		t.Fatalf("expected status git default to service git")
	}
	if got := svc.activeMergeStoreProvider(); got != svc.store {
		t.Fatalf("expected merge store default to service store")
	}
	if got := svc.activeMergeGitProvider(); got != svc.git {
		t.Fatalf("expected merge git default to service git")
	}
	factory := svc.activeMergeGitFactory()
	if factory == nil {
		t.Fatalf("expected non-nil merge git factory")
	}
	created := factory(dir)
	if created == nil {
		t.Fatalf("expected merge git factory to create a client")
	}
	if _, ok := created.(*gitx.Client); !ok {
		t.Fatalf("expected default merge factory to return *gitx.Client, got %T", created)
	}
}

func TestRuntimeProvidersUseOverrides(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)

	altRoot := t.TempDir()
	altStore := store.NewWithSophiaRoot(altRoot, filepath.Join(altRoot, ".sophia-alt"))
	altGit := gitx.New(altRoot)

	svc.overrideLifecycleRuntimeProvidersForTests(altGit, altStore)
	if got := svc.activeLifecycleStoreProvider(); got != altStore {
		t.Fatalf("expected lifecycle store override")
	}
	if got := svc.activeLifecycleGitProvider(); got != altGit {
		t.Fatalf("expected lifecycle git override")
	}

	svc.overrideStatusRuntimeProvidersForTests(altGit, altStore)
	if got := svc.activeStatusStoreProvider(); got != altStore {
		t.Fatalf("expected status store override")
	}
	if got := svc.activeStatusGitProvider(); got != altGit {
		t.Fatalf("expected status git override")
	}

	factoryCalled := false
	svc.overrideMergeRuntimeProvidersForTests(altGit, altStore, func(root string) mergeRuntimeGit {
		factoryCalled = true
		return altGit
	})
	if got := svc.activeMergeStoreProvider(); got != altStore {
		t.Fatalf("expected merge store override")
	}
	if got := svc.activeMergeGitProvider(); got != altGit {
		t.Fatalf("expected merge git override")
	}
	if got := svc.activeMergeGitFactory()(altRoot); got != altGit {
		t.Fatalf("expected merge git factory override")
	}
	if !factoryCalled {
		t.Fatalf("expected merge git factory override to be called")
	}
}
