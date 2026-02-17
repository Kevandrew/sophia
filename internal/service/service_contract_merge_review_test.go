package service

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestImpactCRAppliesRiskSignalsDeterministically(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")

	if err := os.WriteFile(filepath.Join(dir, "delete_me.txt"), []byte("base\n"), 0o644); err != nil {
		t.Fatalf("write base file: %v", err)
	}
	runGit(t, dir, "add", "delete_me.txt")
	runGit(t, dir, "commit", "-m", "chore: base file")

	cr, err := svc.AddCR("Impact", "risk scoring")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	setValidContract(t, svc, cr.ID)

	if err := os.MkdirAll(filepath.Join(dir, "internal", "service"), 0o755); err != nil {
		t.Fatalf("mkdir internal/service: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "internal", "service", "x.go"), []byte("package service\n"), 0o644); err != nil {
		t.Fatalf("write critical file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module tmp\n"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	runGit(t, dir, "rm", "delete_me.txt")
	runGit(t, dir, "add", "internal/service/x.go", "go.mod")
	runGit(t, dir, "commit", "-m", "feat: risky change")

	impact, err := svc.ImpactCR(cr.ID)
	if err != nil {
		t.Fatalf("ImpactCR() error = %v", err)
	}
	if impact.RiskTier != "high" {
		t.Fatalf("expected high risk tier, got %q (score=%d)", impact.RiskTier, impact.RiskScore)
	}
	for _, code := range []string{"critical_paths", "dependency_changes", "deletions", "no_test_changes"} {
		if !containsSignal(impact.Signals, code) {
			t.Fatalf("expected risk signal %q, got %#v", code, impact.Signals)
		}
	}
}

func TestMergeCRBlockedWithoutOverrideWhenValidationFails(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")

	cr, err := svc.AddCR("Blocked merge", "validation should fail")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "blocked.txt"), []byte("x\n"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	runGit(t, dir, "add", "blocked.txt")
	runGit(t, dir, "commit", "-m", "feat: blocked")

	_, err = svc.MergeCR(cr.ID, false, "")
	if !errors.Is(err, ErrCRValidationFailed) {
		t.Fatalf("expected ErrCRValidationFailed, got %v", err)
	}
}

func TestMergeCROverridePersistsAuditEvent(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")

	cr, err := svc.AddCR("Override merge", "intent")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "override.txt"), []byte("x\n"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	runGit(t, dir, "add", "override.txt")
	runGit(t, dir, "commit", "-m", "feat: override")

	sha, err := svc.MergeCR(cr.ID, false, "emergency hotfix")
	if err != nil {
		t.Fatalf("MergeCR(override) error = %v", err)
	}
	if sha == "" {
		t.Fatalf("expected merge sha")
	}

	loaded, err := svc.store.LoadCR(cr.ID)
	if err != nil {
		t.Fatalf("LoadCR() error = %v", err)
	}
	found := false
	for _, event := range loaded.Events {
		if event.Type == "cr_merge_overridden" {
			found = true
			if event.Meta["override_reason"] != "emergency hotfix" {
				t.Fatalf("unexpected override reason meta: %#v", event.Meta)
			}
			break
		}
	}
	if !found {
		t.Fatalf("expected cr_merge_overridden event, got %#v", loaded.Events)
	}
}

func TestReviewAndValidateWorkForMergedCRAfterBranchDeletion(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")

	cr, err := svc.AddCR("Merged fallback", "ensure merged review works")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	setValidContract(t, svc, cr.ID)

	if err := os.WriteFile(filepath.Join(dir, "merged_review.txt"), []byte("ok\n"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	runGit(t, dir, "add", "merged_review.txt")
	runGit(t, dir, "commit", "-m", "feat: merged review fallback")

	if _, err := svc.MergeCR(cr.ID, false, ""); err != nil {
		t.Fatalf("MergeCR() error = %v", err)
	}
	if svc.git.BranchExists(cr.Branch) {
		t.Fatalf("expected branch %q to be deleted by default merge", cr.Branch)
	}

	review, err := svc.ReviewCR(cr.ID)
	if err != nil {
		t.Fatalf("ReviewCR() error = %v", err)
	}
	if !containsAny(review.Files, "merged_review.txt") {
		t.Fatalf("expected merged file in review output, got %#v", review.Files)
	}
	if strings.TrimSpace(review.ShortStat) == "" {
		t.Fatalf("expected non-empty short stat for merged review")
	}

	report, err := svc.ValidateCR(cr.ID)
	if err != nil {
		t.Fatalf("ValidateCR() error = %v", err)
	}
	if !report.Valid {
		t.Fatalf("expected merged CR validation to pass, got errors=%#v", report.Errors)
	}
	if report.Impact == nil || report.Impact.FilesChanged == 0 {
		t.Fatalf("expected impact summary for merged CR, got %#v", report.Impact)
	}
	if strings.TrimSpace(report.Impact.CRUID) == "" {
		t.Fatalf("expected impact CRUID to be populated, got %#v", report.Impact)
	}
}
