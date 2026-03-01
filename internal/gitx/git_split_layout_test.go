package gitx

import "testing"

func TestSplitConcernOwnershipIsCompleteAndNonOverlapping(t *testing.T) {
	t.Parallel()
	concerns := SplitConcernOwnership()
	if len(concerns) == 0 {
		t.Fatalf("expected non-empty concern ownership map")
	}

	seenConcern := map[string]struct{}{}
	seenFile := map[string]struct{}{}
	for _, concern := range concerns {
		if concern.Concern == "" {
			t.Fatalf("concern id cannot be empty: %#v", concern)
		}
		if concern.TargetFile == "" {
			t.Fatalf("target file cannot be empty for concern %q", concern.Concern)
		}
		if len(concern.Responsibilities) == 0 {
			t.Fatalf("responsibilities cannot be empty for concern %q", concern.Concern)
		}
		if _, exists := seenConcern[concern.Concern]; exists {
			t.Fatalf("duplicate concern id %q", concern.Concern)
		}
		if _, exists := seenFile[concern.TargetFile]; exists {
			t.Fatalf("duplicate target file %q", concern.TargetFile)
		}
		seenConcern[concern.Concern] = struct{}{}
		seenFile[concern.TargetFile] = struct{}{}
	}
}

func TestSplitSharedHelperPlacementIsUnique(t *testing.T) {
	t.Parallel()
	placements := SplitSharedHelperPlacement()
	if len(placements) == 0 {
		t.Fatalf("expected non-empty shared helper placement map")
	}

	seen := map[string]struct{}{}
	for _, placement := range placements {
		if placement.Helper == "" {
			t.Fatalf("helper cannot be empty: %#v", placement)
		}
		if placement.OwnerFile == "" {
			t.Fatalf("owner file cannot be empty for helper %q", placement.Helper)
		}
		if _, exists := seen[placement.Helper]; exists {
			t.Fatalf("helper %q assigned multiple times", placement.Helper)
		}
		seen[placement.Helper] = struct{}{}
	}
}
