package service

import "testing"

func TestAddCRWithOptionsSupportsNoSwitchAndSwitch(t *testing.T) {
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
