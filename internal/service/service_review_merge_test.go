package service

import (
	"os"
	"path/filepath"
	"testing"

	"sophia/internal/model"
)

func TestReviewCRIncludesLifecycleActionsWhenNoLinkedPR(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SOPHIA.yaml"), []byte("version: v1\nmerge:\n  mode: pr_gate\narchive:\n  enabled: false\n"), 0o644); err != nil {
		t.Fatalf("write SOPHIA.yaml: %v", err)
	}
	cr, err := svc.AddCR("Review lifecycle metadata", "ensure review carries action guidance")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}

	review, err := svc.ReviewCR(cr.ID)
	if err != nil {
		t.Fatalf("ReviewCR() error = %v", err)
	}
	if review.LifecycleState != model.StatusInProgress {
		t.Fatalf("expected lifecycle_state=%q, got %q", model.StatusInProgress, review.LifecycleState)
	}
	if review.PRLinkageState != prLinkageNoLinkedPR {
		t.Fatalf("expected pr_linkage_state=%q, got %q", prLinkageNoLinkedPR, review.PRLinkageState)
	}
	if review.ActionRequired != prActionOpenPR {
		t.Fatalf("expected action_required=%q, got %q", prActionOpenPR, review.ActionRequired)
	}
	if len(review.SuggestedCommands) == 0 {
		t.Fatalf("expected suggested command guidance in review payload")
	}
}

func TestReviewCRIncludesAbandonmentMetadataAndReopenAction(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SOPHIA.yaml"), []byte("version: v1\nmerge:\n  mode: pr_gate\narchive:\n  enabled: false\n"), 0o644); err != nil {
		t.Fatalf("write SOPHIA.yaml: %v", err)
	}
	cr, err := svc.AddCR("Review abandon metadata", "ensure abandon guidance in review")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	if _, err := svc.AbandonCR(cr.ID, CRAbandonOptions{Reason: "no longer needed"}); err != nil {
		t.Fatalf("AbandonCR() error = %v", err)
	}

	review, err := svc.ReviewCR(cr.ID)
	if err != nil {
		t.Fatalf("ReviewCR() error = %v", err)
	}
	if review.LifecycleState != model.StatusAbandoned {
		t.Fatalf("expected lifecycle_state=%q, got %q", model.StatusAbandoned, review.LifecycleState)
	}
	if review.AbandonedReason != "no longer needed" {
		t.Fatalf("expected abandoned_reason to round-trip, got %q", review.AbandonedReason)
	}
	if review.ActionRequired != "reopen_cr" {
		t.Fatalf("expected action_required=reopen_cr, got %q", review.ActionRequired)
	}
}
