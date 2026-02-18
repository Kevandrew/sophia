package service

import "testing"

func TestBranchSlugFromTitle(t *testing.T) {
	t.Parallel()

	cases := []struct {
		title string
		want  string
	}{
		{title: "Branch Identity Redesign", want: "branch-identity-redesign"},
		{title: "  multi___space --- title  ", want: "multi-space-title"},
		{title: "日本語", want: "intent"},
		{title: "", want: "intent"},
		{title: "A very long title that should definitely be trimmed once the slug crosses the max length boundary", want: "a-very-long-title-that-should-definitely-be-trim"},
	}

	for _, tc := range cases {
		got := branchSlugFromTitle(tc.title)
		if got != tc.want {
			t.Fatalf("branchSlugFromTitle(%q) = %q, want %q", tc.title, got, tc.want)
		}
		if len(got) > crBranchSlugMaxLen {
			t.Fatalf("slug length exceeded max: got %d", len(got))
		}
	}
}

func TestFormatCRBranchAlias(t *testing.T) {
	t.Parallel()

	branch, err := formatCRBranchAlias(42, "Branch identity redesign", "")
	if err != nil {
		t.Fatalf("formatCRBranchAlias() error = %v", err)
	}
	if branch != "cr-42-branch-identity-redesign" {
		t.Fatalf("unexpected branch %q", branch)
	}

	branch, err = formatCRBranchAlias(42, "Branch identity redesign", "KevAndrew")
	if err != nil {
		t.Fatalf("formatCRBranchAlias(owner) error = %v", err)
	}
	if branch != "kevandrew/cr-42-branch-identity-redesign" {
		t.Fatalf("unexpected owner branch %q", branch)
	}
}

func TestParseCRIDFromBranchName(t *testing.T) {
	t.Parallel()

	cases := []struct {
		branch string
		wantID int
		wantOK bool
	}{
		{branch: "sophia/cr-42", wantID: 42, wantOK: true},
		{branch: "cr-42-branch-identity-redesign", wantID: 42, wantOK: true},
		{branch: "kevandrew/cr-42-branch-identity-redesign", wantID: 42, wantOK: true},
		{branch: "main", wantID: 0, wantOK: false},
		{branch: "feature/whatever", wantID: 0, wantOK: false},
	}

	for _, tc := range cases {
		gotID, gotOK := parseCRIDFromBranchName(tc.branch)
		if gotID != tc.wantID || gotOK != tc.wantOK {
			t.Fatalf("parseCRIDFromBranchName(%q) = (%d, %t), want (%d, %t)", tc.branch, gotID, gotOK, tc.wantID, tc.wantOK)
		}
	}
}
