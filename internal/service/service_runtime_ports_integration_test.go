package service

import (
	"sophia/internal/gitx"
	"sophia/internal/model"
	"sophia/internal/store"
	"testing"
)

type spyStatusRuntimeStore struct {
	*store.Store
	loadCRCount     int
	loadConfigCount int
	saveCRCount     int
}

func (s *spyStatusRuntimeStore) LoadCR(id int) (*model.CR, error) {
	s.loadCRCount++
	return s.Store.LoadCR(id)
}

func (s *spyStatusRuntimeStore) LoadConfig() (model.Config, error) {
	s.loadConfigCount++
	return s.Store.LoadConfig()
}

func (s *spyStatusRuntimeStore) SaveCR(cr *model.CR) error {
	s.saveCRCount++
	return s.Store.SaveCR(cr)
}

type spyStatusRuntimeGit struct {
	*gitx.Client
	resolveRefCount int
}

func (g *spyStatusRuntimeGit) ResolveRef(ref string) (string, error) {
	g.resolveRefCount++
	return g.Client.ResolveRef(ref)
}

type spyLifecycleRuntimeStore struct {
	*store.Store
	loadCRCount int
}

func (s *spyLifecycleRuntimeStore) LoadCR(id int) (*model.CR, error) {
	s.loadCRCount++
	return s.Store.LoadCR(id)
}

type spyLifecycleRuntimeGit struct {
	*gitx.Client
	resolveRefCount   int
	branchExistsCount int
}

func (g *spyLifecycleRuntimeGit) ResolveRef(ref string) (string, error) {
	g.resolveRefCount++
	return g.Client.ResolveRef(ref)
}

func (g *spyLifecycleRuntimeGit) BranchExists(branch string) bool {
	g.branchExistsCount++
	return g.Client.BranchExists(branch)
}

func TestWhyCRUsesStatusRuntimeProvidersForBaseFieldHydration(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	cr, err := svc.AddCR("status runtime hydration", "exercise status runtime providers")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}

	loaded, err := svc.store.LoadCR(cr.ID)
	if err != nil {
		t.Fatalf("LoadCR() error = %v", err)
	}
	loaded.BaseBranch = ""
	loaded.BaseRef = ""
	loaded.BaseCommit = ""
	if err := svc.store.SaveCR(loaded); err != nil {
		t.Fatalf("SaveCR() error = %v", err)
	}

	statusStore := &spyStatusRuntimeStore{Store: svc.store}
	statusGit := &spyStatusRuntimeGit{Client: svc.git}
	svc.overrideStatusRuntimeProvidersForTests(statusGit, statusStore)

	view, err := svc.WhyCR(cr.ID)
	if err != nil {
		t.Fatalf("WhyCR() error = %v", err)
	}
	if view.BaseRef == "" || view.BaseCommit == "" {
		t.Fatalf("expected hydrated base fields, got %#v", view)
	}
	if statusStore.loadConfigCount == 0 {
		t.Fatalf("expected status runtime store LoadConfig() during base hydration")
	}
	if statusStore.saveCRCount == 0 {
		t.Fatalf("expected status runtime store SaveCR() during base hydration persistence")
	}
	if statusGit.resolveRefCount == 0 {
		t.Fatalf("expected status runtime git ResolveRef() during base hydration")
	}
}

func TestAddCRWithParentUsesLifecycleRuntimeProvidersForBaseAnchor(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	parent, err := svc.AddCR("parent runtime anchor", "exercise lifecycle runtime providers")
	if err != nil {
		t.Fatalf("AddCR(parent) error = %v", err)
	}

	lifecycleStore := &spyLifecycleRuntimeStore{Store: svc.store}
	lifecycleGit := &spyLifecycleRuntimeGit{Client: svc.git}
	svc.overrideLifecycleRuntimeProvidersForTests(lifecycleGit, lifecycleStore)

	child, _, err := svc.AddCRWithOptionsWithWarnings("child runtime anchor", "uses parent anchor", AddCROptions{
		ParentCRID: parent.ID,
		Switch:     false,
	})
	if err != nil {
		t.Fatalf("AddCRWithOptionsWithWarnings(child) error = %v", err)
	}
	if child.ParentCRID != parent.ID {
		t.Fatalf("expected child parent_cr_id=%d, got %d", parent.ID, child.ParentCRID)
	}
	if lifecycleStore.loadCRCount == 0 {
		t.Fatalf("expected lifecycle runtime store LoadCR() usage")
	}
	if lifecycleGit.branchExistsCount == 0 {
		t.Fatalf("expected lifecycle runtime git BranchExists() usage")
	}
	if lifecycleGit.resolveRefCount == 0 {
		t.Fatalf("expected lifecycle runtime git ResolveRef() usage")
	}
}
