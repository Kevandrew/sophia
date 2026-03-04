package service

import (
	"strings"
	"testing"
)

func TestAddCRWithOptionsSupportsNoSwitchAndSwitch(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	crNoSwitch, _, err := svc.AddCRWithOptionsWithWarnings("No switch", "stay on base branch", AddCROptions{NoSwitch: true})
	if err != nil {
		t.Fatalf("AddCRWithOptionsWithWarnings(no-switch) error = %v", err)
	}
	current, err := svc.git.CurrentBranch()
	if err != nil {
		t.Fatalf("CurrentBranch() error = %v", err)
	}
	if current != "main" {
		t.Fatalf("expected current branch main after no-switch add, got %q", current)
	}
	if !svc.git.BranchExists(crNoSwitch.Branch) {
		t.Fatalf("expected CR branch %q to exist", crNoSwitch.Branch)
	}

	crSwitch, _, err := svc.AddCRWithOptionsWithWarnings("Switch", "switch to CR branch", AddCROptions{Switch: true})
	if err != nil {
		t.Fatalf("AddCRWithOptionsWithWarnings(switch) error = %v", err)
	}
	current, err = svc.git.CurrentBranch()
	if err != nil {
		t.Fatalf("CurrentBranch() error = %v", err)
	}
	if current != crSwitch.Branch {
		t.Fatalf("expected switched branch %q, got %q", crSwitch.Branch, current)
	}
}

func TestAddCRUsesConfiguredOwnerPrefix(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.InitWithOptions(InitOptions{
		BaseBranch:        "main",
		BranchOwnerPrefix: "KevAndrew",
	}); err != nil {
		t.Fatalf("InitWithOptions() error = %v", err)
	}

	cr, err := svc.AddCR("Owner prefix", "configured prefix")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	if !strings.HasPrefix(cr.Branch, "kevandrew/") {
		t.Fatalf("expected owner-prefixed branch, got %q", cr.Branch)
	}
}

func TestAddCRWithExplicitBranchAlias(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	cr, _, err := svc.AddCRWithOptionsWithWarnings("Alias override", "explicit branch", AddCROptions{
		BranchAlias: "kevandrew/cr-1-alias-override",
	})
	if err != nil {
		t.Fatalf("AddCRWithOptionsWithWarnings() error = %v", err)
	}
	if cr.Branch != "kevandrew/cr-1-alias-override" {
		t.Fatalf("expected explicit alias branch, got %q", cr.Branch)
	}

	crV2, _, err := svc.AddCRWithOptionsWithWarnings("Alias v2", "explicit branch", AddCROptions{
		BranchAlias: "cr-explicit-alias-a1b2",
	})
	if err != nil {
		t.Fatalf("AddCRWithOptionsWithWarnings(v2 alias) error = %v", err)
	}
	if crV2.Branch != "cr-explicit-alias-a1b2" {
		t.Fatalf("expected explicit v2 alias branch, got %q", crV2.Branch)
	}
}

func TestAddCRRejectsInvalidAliasCombinations(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	if _, _, err := svc.AddCRWithOptionsWithWarnings("Bad parent", "negative", AddCROptions{
		ParentCRID: -1,
	}); err == nil || !strings.Contains(err.Error(), "--parent must be >= 1") {
		t.Fatalf("expected parent lower bound error, got %v", err)
	}

	if _, _, err := svc.AddCRWithOptionsWithWarnings("Bad combo", "conflict", AddCROptions{
		BaseRef:    "main",
		ParentCRID: 1,
	}); err == nil || !strings.Contains(err.Error(), "--base and --parent cannot be combined") {
		t.Fatalf("expected --base/--parent conflict error, got %v", err)
	}

	if _, _, err := svc.AddCRWithOptionsWithWarnings("Bad alias", "mismatch", AddCROptions{
		BranchAlias: "cr-99-not-this-id",
	}); err == nil {
		t.Fatalf("expected alias id mismatch error")
	}

	if _, _, err := svc.AddCRWithOptionsWithWarnings("Bad combo", "conflict", AddCROptions{
		BranchAlias:    "cr-1-bad-combo",
		OwnerPrefix:    "kevandrew",
		OwnerPrefixSet: true,
	}); err == nil || !strings.Contains(err.Error(), "--branch-alias and --owner-prefix cannot be combined") {
		t.Fatalf("expected branch-alias/owner-prefix conflict error, got %v", err)
	}

	if _, _, err := svc.AddCRWithOptionsWithWarnings("Bad v2 suffix", "unsupported length", AddCROptions{
		BranchAlias: "cr-bad-suffix-a1b2c",
	}); err == nil {
		t.Fatalf("expected v2 suffix length validation error")
	}
}
