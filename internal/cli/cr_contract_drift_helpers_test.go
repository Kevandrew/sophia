package cli

import (
	"reflect"
	"testing"
)

func TestScopeDeltaNormalizesDedupesAndSorts(t *testing.T) {
	added, removed := scopeDelta(
		[]string{" docs", "internal/cli", "docs", ""},
		[]string{"internal/cli", "internal/service", "internal/service "},
	)
	if !reflect.DeepEqual(added, []string{"internal/service"}) {
		t.Fatalf("expected added [internal/service], got %#v", added)
	}
	if !reflect.DeepEqual(removed, []string{"docs"}) {
		t.Fatalf("expected removed [docs], got %#v", removed)
	}
}
