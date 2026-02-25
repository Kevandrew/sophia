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
	if strings.TrimSpace(baseRef) != "" && parentCRID > 0 {
		return errors.New("--base and --parent cannot be combined")
	}
	if strings.TrimSpace(branchAlias) != "" && ownerPrefixSet {
		return errors.New("--branch-alias and --owner-prefix cannot be combined")
	}
	return nil
}

func ShouldSwitch(noSwitch, switchFlag bool) bool {
	switchBranch := true
	if noSwitch {
		switchBranch = false
	}
	if switchFlag {
		switchBranch = true
	}
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
