package service

import (
	"os"
	"path/filepath"
	"testing"

	"sophia/internal/model"
	"sophia/internal/store"
)

func TestServiceNewFallsBackToSharedLocalMetadata(t *testing.T) {
	dir := t.TempDir()
	runGit(t, dir, "init", "-b", "main")
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")
	runGit(t, dir, "commit", "--allow-empty", "-m", "seed")

	shared := localMetadataDir(t, dir)
	sharedStore := store.NewWithSophiaRoot(dir, shared)
	if err := sharedStore.Init("main", model.MetadataModeLocal); err != nil {
		t.Fatalf("sharedStore.Init() error = %v", err)
	}

	if _, err := os.Stat(filepath.Join(dir, ".sophia", "config.yaml")); !os.IsNotExist(err) {
		t.Fatalf("expected legacy .sophia metadata to be absent, err=%v", err)
	}

	svc := New(dir)
	if !pathsReferToSameLocation(t, svc.store.SophiaDir(), shared) {
		t.Fatalf("expected shared metadata path %q, got %q", shared, svc.store.SophiaDir())
	}
	if _, err := svc.store.LoadConfig(); err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
}

func TestServiceNewResolvesRepoRootFromSubdirectory(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	subdir := filepath.Join(dir, "internal", "nested")
	if err := os.MkdirAll(subdir, 0o755); err != nil {
		t.Fatalf("MkdirAll(subdir) error = %v", err)
	}

	nestedSvc := New(subdir)
	want := filepath.Join(dir, ".sophia")
	if !pathsReferToSameLocation(t, nestedSvc.store.SophiaDir(), want) {
		t.Fatalf("expected nested service metadata path %q, got %q", want, nestedSvc.store.SophiaDir())
	}
	if _, err := nestedSvc.store.LoadConfig(); err != nil {
		t.Fatalf("nested LoadConfig() error = %v", err)
	}
}

func TestServiceNewPrefersLegacyMetadataWhenInitialized(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	shared := localMetadataDir(t, dir)
	sharedStore := store.NewWithSophiaRoot(dir, shared)
	if err := sharedStore.Init("main", model.MetadataModeLocal); err != nil {
		t.Fatalf("sharedStore.Init() error = %v", err)
	}

	reloaded := New(dir)
	want := filepath.Join(dir, ".sophia")
	if !pathsReferToSameLocation(t, reloaded.store.SophiaDir(), want) {
		t.Fatalf("expected legacy metadata path %q, got %q", want, reloaded.store.SophiaDir())
	}
}

func pathsReferToSameLocation(t *testing.T, a, b string) bool {
	t.Helper()
	aInfo, aErr := os.Stat(a)
	if aErr != nil {
		return false
	}
	bInfo, bErr := os.Stat(b)
	if bErr != nil {
		return false
	}
	return os.SameFile(aInfo, bInfo)
}
