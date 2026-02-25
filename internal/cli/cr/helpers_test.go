package cr

import "testing"

func TestBuildAddCROptionsNormalizesInputs(t *testing.T) {
	opts := BuildAddCROptions(AddOptionsInput{
		BaseRef:        "  origin/main  ",
		ParentCRID:     7,
		SwitchBranch:   true,
		BranchAlias:    "  feat-x  ",
		OwnerPrefix:    "  van  ",
		OwnerPrefixSet: true,
	})

	if opts.BaseRef != "origin/main" {
		t.Fatalf("BaseRef = %q, want origin/main", opts.BaseRef)
	}
	if !opts.Switch || opts.NoSwitch {
		t.Fatalf("switch flags = (%t,%t), want (true,false)", opts.Switch, opts.NoSwitch)
	}
	if opts.BranchAlias != "feat-x" {
		t.Fatalf("BranchAlias = %q, want feat-x", opts.BranchAlias)
	}
	if opts.OwnerPrefix != "van" || !opts.OwnerPrefixSet {
		t.Fatalf("owner fields = (%q,%t), want (van,true)", opts.OwnerPrefix, opts.OwnerPrefixSet)
	}
}

func TestValidateTaskDoneFlagsRejectsInvalidCombinations(t *testing.T) {
	err := ValidateTaskDoneFlags(TaskDoneFlags{
		NoCheckpoint: true,
		FromContract: true,
	})
	if err == nil {
		t.Fatalf("expected conflict error for --no-checkpoint + --from-contract")
	}

	err = ValidateTaskDoneFlags(TaskDoneFlags{
		NoCheckpoint: true,
	})
	if err == nil {
		t.Fatalf("expected reason requirement for --no-checkpoint")
	}
}

func TestTaskDoneScopeModeAndCheckpointSource(t *testing.T) {
	flags := TaskDoneFlags{
		NoCheckpoint: true,
	}
	if got := TaskDoneScopeMode(flags); got != "none" {
		t.Fatalf("TaskDoneScopeMode(no-checkpoint) = %q, want none", got)
	}
	if got := TaskDoneCheckpointSource(flags); got != "task_no_checkpoint" {
		t.Fatalf("TaskDoneCheckpointSource(no-checkpoint) = %q, want task_no_checkpoint", got)
	}
}
