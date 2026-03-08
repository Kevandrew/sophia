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

func TestStackReadSurfacesUseInferredParentageWhenStoredParentMissing(t *testing.T) {
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
	grandchild := seedCR(3, "Grandchild", seedCROptions{
		Branch:     "cr-grandchild",
		BaseBranch: "main",
		BaseRef:    child.Branch,
		ParentCRID: 0,
	})

	readModel := buildCRReadModel([]model.CR{*parent, *child, *grandchild})
	svc := New(t.TempDir())

	nativity := svc.stackNativityForCRWithReadModel(child, readModel)
	if !nativity.IsChild || nativity.ParentCRID != parent.ID {
		t.Fatalf("expected inferred child nativity under parent %d, got %#v", parent.ID, nativity)
	}

	lineage := svc.stackLineageForCRWithReadModel(grandchild, readModel)
	if len(lineage) != 2 || lineage[0].ID != parent.ID || lineage[1].ID != child.ID {
		t.Fatalf("expected inferred lineage parent->child, got %#v", lineage)
	}

	tree := svc.stackTreeForCRWithReadModel(parent, readModel)
	if tree == nil || len(tree.Children) != 1 {
		t.Fatalf("expected inferred stack tree child under parent, got %#v", tree)
	}
	if tree.Children[0].ParentCRID != parent.ID {
		t.Fatalf("expected normalized child parent %d, got %#v", parent.ID, tree.Children[0])
	}
	if len(tree.Children[0].Children) != 1 || tree.Children[0].Children[0].ParentCRID != child.ID {
		t.Fatalf("expected normalized grandchild parent %d, got %#v", child.ID, tree.Children[0].Children)
	}
}
