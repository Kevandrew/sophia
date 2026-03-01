package cr

import (
	"fmt"
	"sophia/internal/service"
	"strings"
)

type AddOptionsInput struct {
	BaseRef        string
	ParentCRID     int
	SwitchBranch   bool
	BranchAlias    string
	OwnerPrefix    string
	OwnerPrefixSet bool
}

func BuildAddCROptions(in AddOptionsInput) service.AddCROptions {
	return service.AddCROptions{
		BaseRef:        strings.TrimSpace(in.BaseRef),
		ParentCRID:     in.ParentCRID,
		Switch:         in.SwitchBranch,
		NoSwitch:       !in.SwitchBranch,
		BranchAlias:    strings.TrimSpace(in.BranchAlias),
		OwnerPrefix:    strings.TrimSpace(in.OwnerPrefix),
		OwnerPrefixSet: in.OwnerPrefixSet,
	}
}

type TaskDoneFlags struct {
	NoCheckpoint       bool
	NoCheckpointReason string
	StageAll           bool
	FromContract       bool
	ScopePaths         []string
	PatchFile          string
	CommitType         string
}

func ValidateTaskDoneFlags(flags TaskDoneFlags) error {
	trimmedReason := strings.TrimSpace(flags.NoCheckpointReason)
	trimmedPatchFile := strings.TrimSpace(flags.PatchFile)
	trimmedCommitType := strings.TrimSpace(flags.CommitType)
	if flags.NoCheckpoint && (flags.StageAll || flags.FromContract || len(flags.ScopePaths) > 0 || trimmedPatchFile != "") {
		return fmt.Errorf("--no-checkpoint cannot be combined with --from-contract, --path, --patch-file, or --all")
	}
	if flags.NoCheckpoint && trimmedCommitType != "" {
		return fmt.Errorf("--commit-type requires a checkpoint commit and cannot be combined with --no-checkpoint")
	}
	if flags.NoCheckpoint && trimmedReason == "" {
		return fmt.Errorf("--no-checkpoint requires --no-checkpoint-reason")
	}
	if !flags.NoCheckpoint && trimmedReason != "" {
		return fmt.Errorf("--no-checkpoint-reason requires --no-checkpoint")
	}
	if trimmedCommitType != "" && !isValidCommitType(trimmedCommitType) {
		return fmt.Errorf("invalid --commit-type %q (supported: feat, fix, docs, refactor, test, chore, perf, build, ci, style, revert)", trimmedCommitType)
	}
	if flags.NoCheckpoint {
		return nil
	}
	modeCount := 0
	if flags.StageAll {
		modeCount++
	}
	if flags.FromContract {
		modeCount++
	}
	if len(flags.ScopePaths) > 0 {
		modeCount++
	}
	if trimmedPatchFile != "" {
		modeCount++
	}
	if modeCount > 1 {
		return fmt.Errorf("exactly one checkpoint scope mode is required: --from-contract, --path <file> (repeatable), --patch-file <file>, or --all")
	}
	if modeCount == 0 {
		return fmt.Errorf("checkpoint scope required: use --from-contract, --path <file> (repeatable), --patch-file <file>, or --all")
	}
	return nil
}

func BuildTaskDoneOptions(flags TaskDoneFlags) service.DoneTaskOptions {
	return service.DoneTaskOptions{
		Checkpoint:         !flags.NoCheckpoint,
		StageAll:           flags.StageAll,
		FromContract:       flags.FromContract,
		Paths:              append([]string(nil), flags.ScopePaths...),
		PatchFile:          strings.TrimSpace(flags.PatchFile),
		NoCheckpointReason: strings.TrimSpace(flags.NoCheckpointReason),
		CommitType:         strings.TrimSpace(flags.CommitType),
	}
}

func TaskDoneScopeMode(flags TaskDoneFlags) string {
	if flags.NoCheckpoint {
		return "none"
	}
	if flags.StageAll {
		return "all"
	}
	if flags.FromContract {
		return "from_contract"
	}
	if len(flags.ScopePaths) > 0 {
		return "path"
	}
	if strings.TrimSpace(flags.PatchFile) != "" {
		return "patch_file"
	}
	return "unknown"
}

func TaskDoneCheckpointSource(flags TaskDoneFlags) string {
	if flags.NoCheckpoint {
		return "task_no_checkpoint"
	}
	return "task_checkpoint"
}

func isValidCommitType(raw string) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "feat", "fix", "docs", "refactor", "test", "chore", "perf", "build", "ci", "style", "revert":
		return true
	default:
		return false
	}
}
