package json

import (
	"errors"
	"testing"
)

func TestHandledErrorRoundTrip(t *testing.T) {
	base := errors.New("boom")
	wrapped := MarkHandled(base)
	if wrapped == nil {
		t.Fatalf("MarkHandled returned nil")
	}
	if !IsHandled(wrapped) {
		t.Fatalf("expected handled error")
	}
}

func TestBranchIdentityToMapParsesHumanAliasV2(t *testing.T) {
	m := BranchIdentityToMap("team/cr-refactor-4a06", "cr_uid")
	if got := m["scheme"]; got != "human_alias_v2" {
		t.Fatalf("scheme = %v, want human_alias_v2", got)
	}
	if got := m["slug"]; got != "refactor" {
		t.Fatalf("slug = %v, want refactor", got)
	}
	if got := m["uid_suffix"]; got != "4a06" {
		t.Fatalf("uid_suffix = %v, want 4a06", got)
	}
	if got := m["owner_prefix"]; got != "team" {
		t.Fatalf("owner_prefix = %v, want team", got)
	}
}
