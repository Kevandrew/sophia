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
	if branch != "cr-branch-identity-redesign-0016" {
		t.Fatalf("unexpected branch %q", branch)
	}

	branch, err = formatCRBranchAlias(42, "Branch identity redesign", "KevAndrew")
	if err != nil {
		t.Fatalf("formatCRBranchAlias(owner) error = %v", err)
	}
	if branch != "kevandrew/cr-branch-identity-redesign-0016" {
		t.Fatalf("unexpected owner branch %q", branch)
	}
}

func TestFormatCRBranchAliasFromUID(t *testing.T) {
	t.Parallel()

	branch, err := formatCRBranchAliasFromUID("Branch identity redesign", "", "cr_c6bec981-b3dc-493d-aa41-897df808126c", 4)
	if err != nil {
		t.Fatalf("formatCRBranchAliasFromUID() error = %v", err)
	}
	if branch != "cr-branch-identity-redesign-c6be" {
		t.Fatalf("unexpected uid branch %q", branch)
	}
}

func TestFormatCRBranchAliasWithFallbackEscalatesSuffixLength(t *testing.T) {
	t.Parallel()

	colliding := "cr-branch-identity-redesign-c6be"
	branch, err := formatCRBranchAliasWithFallback("Branch identity redesign", "", "cr_c6bec981-b3dc-493d-aa41-897df808126c", func(candidate string) bool {
		return candidate == colliding
	})
	if err != nil {
		t.Fatalf("formatCRBranchAliasWithFallback() error = %v", err)
	}
	if branch != "cr-branch-identity-redesign-c6bec9" {
		t.Fatalf("expected suffix-length fallback branch, got %q", branch)
	}
}

func TestFormatCRBranchAliasWithFallbackRejectsFullCollision(t *testing.T) {
	t.Parallel()

	_, err := formatCRBranchAliasWithFallback("Branch identity redesign", "", "cr_c6bec981-b3dc-493d-aa41-897df808126c", func(string) bool {
		return true
	})
	if err == nil {
		t.Fatalf("expected full-collision error when all suffix lengths are exhausted")
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
		{branch: "cr-branch-identity-redesign-c6be", wantID: 0, wantOK: false},
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

func TestDetectCRBranchScheme(t *testing.T) {
	t.Parallel()

	if got := detectCRBranchScheme("cr-branch-identity-redesign-c6be"); got != "human_alias_v2" {
		t.Fatalf("detectCRBranchScheme(v2) = %q", got)
	}
	if got := detectCRBranchScheme("CR-branch-identity-redesign-C6BE"); got != "human_alias_v2" {
		t.Fatalf("detectCRBranchScheme(v2 uppercase) = %q", got)
	}
	if got := detectCRBranchScheme("cr-42-branch-identity-redesign"); got != "human_alias_v1" {
		t.Fatalf("detectCRBranchScheme(v1) = %q", got)
	}
}

func TestValidateExplicitCRBranchAliasRejectsUnsupportedV2SuffixLength(t *testing.T) {
	t.Parallel()

	if _, err := validateExplicitCRBranchAlias("cr-branch-identity-redesign-a1b2c", 1); err == nil {
		t.Fatalf("expected unsupported v2 suffix length to fail")
	}
}

func TestOwnerPrefixFromBranch(t *testing.T) {
	t.Parallel()

	cases := []struct {
		branch string
		want   string
		ok     bool
	}{
		{branch: "team/cr-branch-identity-redesign-c6be", want: "team", ok: true},
		{branch: "Team/cr-branch-identity-redesign-c6be", want: "team", ok: true},
		{branch: "cr-branch-identity-redesign-c6be", want: "", ok: false},
		{branch: "feature/team-cr-branch", want: "", ok: false},
	}

	for _, tc := range cases {
		got, ok := ownerPrefixFromBranch(tc.branch)
		if ok != tc.ok || got != tc.want {
			t.Fatalf("ownerPrefixFromBranch(%q) = (%q,%t), want (%q,%t)", tc.branch, got, ok, tc.want, tc.ok)
		}
	}
}
