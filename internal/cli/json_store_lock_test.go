package cli

import (
	"testing"

	"sophia/internal/store"
)

func TestJSONErrorCodeMutationLockTimeout(t *testing.T) {
	t.Parallel()
	if got := jsonErrorCode(store.MutationLockTimeoutError{Path: ".sophia/mutation.lock"}); got != "resource_busy" {
		t.Fatalf("jsonErrorCode(store.MutationLockTimeoutError) = %q, want resource_busy", got)
	}
}
