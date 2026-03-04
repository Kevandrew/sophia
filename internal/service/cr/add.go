package cr

import (
	"errors"
	"fmt"
	"strings"

	"sophia/internal/model"
)

type BuildInput struct {
	ID          int
	UID         string
	Title       string
	Description string
	BaseBranch  string
	BaseRef     string
	BaseCommit  string
	ParentCRID  int
	Branch      string
	Now         string
	Actor       string
}

func ValidateAddRequest(title, baseRef string, parentCRID int, branchAlias string, ownerPrefixSet bool) error {
	if strings.TrimSpace(title) == "" {
		return errors.New("title cannot be empty")
	}
	if parentCRID < 0 {
		return errors.New("--parent must be >= 1")
	}
	if strings.TrimSpace(baseRef) != "" && parentCRID > 0 {
		return errors.New("--base and --parent cannot be combined")
	}
	if strings.TrimSpace(branchAlias) != "" && ownerPrefixSet {
		return errors.New("--branch-alias and --owner-prefix cannot be combined")
	}
	return nil
}

const (
	SwitchModeSwitch   = "switch"
	SwitchModeNoSwitch = "no_switch"
)

func NormalizeSwitchFlags(mode string, noSwitch, switchFlag bool) (string, bool, bool) {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case SwitchModeSwitch:
		return SwitchModeSwitch, false, true
	case SwitchModeNoSwitch:
		return SwitchModeNoSwitch, true, false
	}
	if switchFlag {
		return SwitchModeSwitch, false, true
	}
	if noSwitch {
		return SwitchModeNoSwitch, true, false
	}
	// Backward-compatible default for service-level AddCRWithOptions callers.
	return SwitchModeSwitch, false, true
}

func ShouldSwitch(noSwitch, switchFlag bool) bool {
	_, _, switchBranch := NormalizeSwitchFlags("", noSwitch, switchFlag)
	return switchBranch
}

func BuildCR(input BuildInput) *model.CR {
	return &model.CR{
		ID:          input.ID,
		UID:         input.UID,
		Title:       input.Title,
		Description: input.Description,
		Status:      model.StatusInProgress,
		BaseBranch:  input.BaseBranch,
		BaseRef:     input.BaseRef,
		BaseCommit:  input.BaseCommit,
		ParentCRID:  input.ParentCRID,
		Branch:      input.Branch,
		Notes:       []string{},
		Evidence:    []model.EvidenceEntry{},
		Subtasks:    []model.Subtask{},
		Events: []model.Event{
			{
				TS:      input.Now,
				Actor:   input.Actor,
				Type:    model.EventTypeCRCreated,
				Summary: fmt.Sprintf("Created CR %d", input.ID),
				Ref:     fmt.Sprintf("cr:%d", input.ID),
			},
		},
		CreatedAt: input.Now,
		UpdatedAt: input.Now,
	}
}
