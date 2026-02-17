package store

import (
	"testing"

	"sophia/internal/model"
)

func TestInitCreatesLayout(t *testing.T) {
	s := New(t.TempDir())
	if err := s.Init("main"); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	if !s.IsInitialized() {
		t.Fatalf("expected store to be initialized")
	}

	cfg, err := s.LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if cfg.BaseBranch != "main" {
		t.Fatalf("expected base branch main, got %q", cfg.BaseBranch)
	}

	idx, err := s.LoadIndex()
	if err != nil {
		t.Fatalf("LoadIndex() error = %v", err)
	}
	if idx.NextID != 1 {
		t.Fatalf("expected next id 1, got %d", idx.NextID)
	}
}

func TestNextCRIDDeterministic(t *testing.T) {
	s := New(t.TempDir())
	if err := s.Init("main"); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	id1, err := s.NextCRID()
	if err != nil {
		t.Fatalf("NextCRID() first call error = %v", err)
	}
	id2, err := s.NextCRID()
	if err != nil {
		t.Fatalf("NextCRID() second call error = %v", err)
	}

	if id1 != 1 || id2 != 2 {
		t.Fatalf("unexpected ids: %d, %d", id1, id2)
	}

	idx, err := s.LoadIndex()
	if err != nil {
		t.Fatalf("LoadIndex() error = %v", err)
	}
	if idx.NextID != 3 {
		t.Fatalf("expected next id 3, got %d", idx.NextID)
	}
}

func TestCRReadWriteRoundTrip(t *testing.T) {
	s := New(t.TempDir())
	if err := s.Init("main"); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	cr := &model.CR{
		ID:          1,
		Title:       "Add retries",
		Description: "Improve resilience",
		Status:      model.StatusInProgress,
		BaseBranch:  "main",
		Branch:      "sophia/cr-1",
		Notes:       []string{"started"},
		Subtasks: []model.Subtask{
			{ID: 1, Title: "Code", Status: model.TaskStatusOpen, CreatedAt: "2026-01-01T00:00:00Z", UpdatedAt: "2026-01-01T00:00:00Z", CreatedBy: "user"},
		},
		Events:    []model.Event{{TS: "2026-01-01T00:00:00Z", Actor: "user", Type: "cr_created", Summary: "Created CR 1", Ref: "cr:1"}},
		CreatedAt: "2026-01-01T00:00:00Z",
		UpdatedAt: "2026-01-01T00:00:00Z",
	}

	if err := s.SaveCR(cr); err != nil {
		t.Fatalf("SaveCR() error = %v", err)
	}

	loaded, err := s.LoadCR(1)
	if err != nil {
		t.Fatalf("LoadCR() error = %v", err)
	}
	if loaded.Title != cr.Title || loaded.Description != cr.Description {
		t.Fatalf("loaded CR mismatch: %#v", loaded)
	}
	if len(loaded.Subtasks) != 1 || loaded.Subtasks[0].Title != "Code" {
		t.Fatalf("expected one subtask, got %#v", loaded.Subtasks)
	}
	if len(loaded.Events) != 1 || loaded.Events[0].Type != "cr_created" {
		t.Fatalf("expected event round-trip, got %#v", loaded.Events)
	}
}

func TestListCRsSortedByID(t *testing.T) {
	s := New(t.TempDir())
	if err := s.Init("main"); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	for _, id := range []int{3, 1, 2} {
		cr := &model.CR{ID: id, Title: "t", Status: model.StatusInProgress, BaseBranch: "main", Branch: "sophia/cr-1", Notes: []string{}, Subtasks: []model.Subtask{}, Events: []model.Event{}, CreatedAt: "2026-01-01T00:00:00Z", UpdatedAt: "2026-01-01T00:00:00Z"}
		if err := s.SaveCR(cr); err != nil {
			t.Fatalf("SaveCR(%d) error = %v", id, err)
		}
	}

	crs, err := s.ListCRs()
	if err != nil {
		t.Fatalf("ListCRs() error = %v", err)
	}
	if len(crs) != 3 {
		t.Fatalf("expected 3 CRs, got %d", len(crs))
	}
	if crs[0].ID != 1 || crs[1].ID != 2 || crs[2].ID != 3 {
		t.Fatalf("expected sorted IDs [1,2,3], got [%d,%d,%d]", crs[0].ID, crs[1].ID, crs[2].ID)
	}
}
