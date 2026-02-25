package service

import (
	"errors"
	"sophia/internal/model"
	"testing"
)

func TestActiveTaskMergeGuardDefaultsToServiceMergeGuard(t *testing.T) {
	svc := &Service{}
	guard := svc.activeTaskMergeGuard()
	if guard == nil {
		t.Fatalf("expected non-nil merge guard")
	}
}

func TestActiveTaskMergeGuardUsesOverride(t *testing.T) {
	expected := errors.New("guard hit")
	svc := &Service{}
	svc.overrideTaskMergeGuardForTests(func(*model.CR) error { return expected })

	if err := svc.activeTaskMergeGuard()(&model.CR{}); !errors.Is(err, expected) {
		t.Fatalf("expected override guard error %v, got %v", expected, err)
	}
}

