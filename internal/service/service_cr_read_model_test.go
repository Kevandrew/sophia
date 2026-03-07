package service

import (
	"testing"

	"sophia/internal/model"
)

func TestBuildCRReadModelUsesBaseRefImpliedParentWhenStoredParentMissing(t *testing.T) {
	t.Parallel()

	parent := seedCR(1, "Parent", seedCROptions{
		Branch:     "cr-parent",
		BaseBranch: "main",
		BaseRef:    "main",
	})
	child := seedCR(2, "Child", seedCROptions{
		Branch:     "cr-child",
		BaseBranch: "main",
		BaseRef:    parent.Branch,
		ParentCRID: 0,
	})

	readModel := buildCRReadModel([]model.CR{*parent, *child})
	children := readModel.childrenOf(parent.ID)
	if len(children) != 1 || children[0].ID != child.ID {
		t.Fatalf("expected child to remain visible under inferred parent, got %#v", children)
	}
}
