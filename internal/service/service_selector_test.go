package service

import (
	"strings"
	"testing"
)

func TestResolveCRIDByUID(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	cr, err := svc.AddCR("Resolve UID", "selector test")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	got, err := svc.ResolveCRIDByUID(cr.UID)
	if err != nil {
		t.Fatalf("ResolveCRIDByUID() error = %v", err)
	}
	if got != cr.ID {
		t.Fatalf("expected id %d, got %d", cr.ID, got)
	}
}

func TestResolveCRIDSupportsIDUIDAndAlias(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	cr, err := svc.AddCR("Resolve selector", "selector variants")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}

	for _, selector := range []string{
		"1",
		cr.UID,
		cr.Branch,
	} {
		got, resolveErr := svc.ResolveCRID(selector)
		if resolveErr != nil {
			t.Fatalf("ResolveCRID(%q) error = %v", selector, resolveErr)
		}
		if got != cr.ID {
			t.Fatalf("ResolveCRID(%q) expected %d, got %d", selector, cr.ID, got)
		}
	}
}

func TestResolveCRIDRejectsUnknownBranchLikeSelector(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if _, err := svc.AddCR("Resolve selector", "selector variants"); err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}

	_, err := svc.ResolveCRID("cr-missing-alias-a1b2")
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "not found") {
		t.Fatalf("expected branch-like selector not found error, got %v", err)
	}
}
