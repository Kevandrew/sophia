package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sophia/internal/model"
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

func TestAddCRDefaultSwitchParityAcrossWrappers(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		run  func(*Service, string, string) (*model.CR, error)
	}{
		{
			name: "AddCR",
			run: func(svc *Service, title, description string) (*model.CR, error) {
				return svc.AddCR(title, description)
			},
		},
		{
			name: "AddCRWithWarnings",
			run: func(svc *Service, title, description string) (*model.CR, error) {
				cr, _, err := svc.AddCRWithWarnings(title, description)
				return cr, err
			},
		},
		{
			name: "AddCRWithOptions",
			run: func(svc *Service, title, description string) (*model.CR, error) {
				result, err := svc.AddCRWithOptions(title, description, AddCROptions{})
				if err != nil {
					return nil, err
				}
				return result.CR, nil
			},
		},
		{
			name: "AddCRWithOptionsWithWarnings",
			run: func(svc *Service, title, description string) (*model.CR, error) {
				cr, _, err := svc.AddCRWithOptionsWithWarnings(title, description, AddCROptions{})
				return cr, err
			},
		},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			svc := New(dir)
			if _, err := svc.Init("main", ""); err != nil {
				t.Fatalf("Init() error = %v", err)
			}

			cr, err := tc.run(svc, "default switch "+tc.name, "wrapper parity")
			if err != nil {
				t.Fatalf("%s() error = %v", tc.name, err)
			}

			current, err := svc.git.CurrentBranch()
			if err != nil {
				t.Fatalf("CurrentBranch() error = %v", err)
			}
			if current != cr.Branch {
				t.Fatalf("expected switched branch %q, got %q", cr.Branch, current)
			}
		})
	}
}

func TestAddCRWithOptionsNormalizesLegacySwitchCombinations(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		opts       AddCROptions
		wantSwitch bool
	}{
		{
			name:       "legacy conflicting bools remain switch",
			opts:       AddCROptions{Switch: true, NoSwitch: true},
			wantSwitch: true,
		},
		{
			name:       "legacy no flags defaults to switch",
			opts:       AddCROptions{},
			wantSwitch: true,
		},
		{
			name:       "legacy explicit no_switch remains no switch",
			opts:       AddCROptions{NoSwitch: true},
			wantSwitch: false,
		},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			svc := New(dir)
			if _, err := svc.Init("main", ""); err != nil {
				t.Fatalf("Init() error = %v", err)
			}

			cr, _, err := svc.AddCRWithOptionsWithWarnings("normalize "+tc.name, "switch semantics", tc.opts)
			if err != nil {
				t.Fatalf("AddCRWithOptionsWithWarnings() error = %v", err)
			}
			current, err := svc.git.CurrentBranch()
			if err != nil {
				t.Fatalf("CurrentBranch() error = %v", err)
			}

			if tc.wantSwitch {
				if current != cr.Branch {
					t.Fatalf("expected switched branch %q, got %q", cr.Branch, current)
				}
				return
			}
			if current != "main" {
				t.Fatalf("expected current branch main when no-switch, got %q", current)
			}
		})
	}
}

func TestNormalizeCLIAndServiceAddOptionsDefaultsByEntrypoint(t *testing.T) {
	t.Parallel()

	cliDefaults := NormalizeCLIAddCROptions(AddCROptions{})
	if cliDefaults.Switch || !cliDefaults.NoSwitch {
		t.Fatalf("expected CLI defaults (Switch=false, NoSwitch=true), got (%t,%t)", cliDefaults.Switch, cliDefaults.NoSwitch)
	}
	cliSwitch := NormalizeCLIAddCROptions(AddCROptions{Switch: true})
	if !cliSwitch.Switch || cliSwitch.NoSwitch {
		t.Fatalf("expected CLI --switch normalization to keep switch=true, got (%t,%t)", cliSwitch.Switch, cliSwitch.NoSwitch)
	}

	serviceDefaults := normalizeServiceAddCROptions(AddCROptions{})
	if !serviceDefaults.Switch || serviceDefaults.NoSwitch {
		t.Fatalf("expected service defaults (Switch=true, NoSwitch=false), got (%t,%t)", serviceDefaults.Switch, serviceDefaults.NoSwitch)
	}
}

func TestAddChildCRFromCurrentDefaultsToSwitch(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	parent, err := svc.AddCR("Parent switch default", "anchor")
	if err != nil {
		t.Fatalf("AddCR(parent) error = %v", err)
	}
	if current, err := svc.git.CurrentBranch(); err != nil || current != parent.Branch {
		t.Fatalf("expected current branch %q after AddCR, got %q (err=%v)", parent.Branch, current, err)
	}

	child, _, err := svc.AddChildCRFromCurrent("Child switch default", "inherits parent context")
	if err != nil {
		t.Fatalf("AddChildCRFromCurrent() error = %v", err)
	}
	current, err := svc.git.CurrentBranch()
	if err != nil {
		t.Fatalf("CurrentBranch() error = %v", err)
	}
	if current != child.Branch {
		t.Fatalf("expected AddChildCRFromCurrent to switch to child branch %q, got %q", child.Branch, current)
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

func TestAddCRRejectsNegativeParentWithoutLazyBootstrap(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	runGit(t, dir, "init", "-b", "main")

	svc := New(dir)
	if svc.store.IsInitialized() {
		t.Fatalf("expected uninitialized metadata store before invalid add")
	}

	if _, _, err := svc.AddCRWithOptionsWithWarnings("Bad parent", "negative", AddCROptions{
		ParentCRID: -1,
	}); err == nil || !strings.Contains(err.Error(), "--parent must be >= 1") {
		t.Fatalf("expected parent lower bound error, got %v", err)
	}

	if svc.store.IsInitialized() {
		t.Fatalf("expected invalid add request to avoid lazy bootstrap")
	}
	if _, err := os.Stat(filepath.Join(localMetadataDir(t, dir), "config.yaml")); !os.IsNotExist(err) {
		t.Fatalf("expected shared metadata config to remain absent, err=%v", err)
	}
}
