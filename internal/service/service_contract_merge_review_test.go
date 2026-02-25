package service

import (
	"errors"
	"os"
	"path/filepath"
	"sophia/internal/model"
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
	riskScopes := []string{"internal/service"}
	riskHint := "high"
	riskRationale := "service boundary changes are high risk"
	if _, err := svc.SetCRContract(cr.ID, ContractPatch{
		RiskCriticalScopes: &riskScopes,
		RiskTierHint:       &riskHint,
		RiskRationale:      &riskRationale,
	}); err != nil {
		t.Fatalf("SetCRContract(risk hints) error = %v", err)
	}

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
	for _, code := range []string{"critical_scope_hint", "dependency_changes", "deletions", "no_test_changes"} {
		if !containsSignal(impact.Signals, code) {
			t.Fatalf("expected risk signal %q, got %#v", code, impact.Signals)
		}
	}
	if impact.RiskTierHint != "high" {
		t.Fatalf("expected risk tier hint high, got %q", impact.RiskTierHint)
	}
	if impact.RiskTierFloorApplied {
		t.Fatalf("expected no floor application when computed tier already high, got %#v", impact)
	}
	if containsSignal(impact.Signals, "risk_tier_hint_floor") {
		t.Fatalf("did not expect risk_tier_hint_floor signal when computed tier already high, got %#v", impact.Signals)
	}
	if len(impact.MatchedRiskCriticalScopes) != 1 || impact.MatchedRiskCriticalScopes[0] != "internal/service" {
		t.Fatalf("unexpected matched risk scopes: %#v", impact.MatchedRiskCriticalScopes)
	}
}

func TestImpactCRDoesNotUseRepoHardcodedCriticalPathSignals(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")

	cr, err := svc.AddCR("No hardcoded critical path", "risk scoring should be contract-driven")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	setValidContract(t, svc, cr.ID)

	if err := os.MkdirAll(filepath.Join(dir, "internal", "service"), 0o755); err != nil {
		t.Fatalf("mkdir internal/service: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "internal", "service", "x.go"), []byte("package service\n"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	runGit(t, dir, "add", "internal/service/x.go")
	runGit(t, dir, "commit", "-m", "feat: touch service path")

	impact, err := svc.ImpactCR(cr.ID)
	if err != nil {
		t.Fatalf("ImpactCR() error = %v", err)
	}
	if containsSignal(impact.Signals, "critical_paths") {
		t.Fatalf("did not expect legacy critical_paths signal, got %#v", impact.Signals)
	}
	if containsSignal(impact.Signals, "critical_scope_hint") {
		t.Fatalf("did not expect critical_scope_hint without contract risk scopes, got %#v", impact.Signals)
	}
}

func TestImpactCRRiskTierHintNoFloorWhenHintLowerOrEqual(t *testing.T) {
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
	runGit(t, dir, "commit", "-m", "chore: seed")

	cr, err := svc.AddCR("Hint no floor", "high from heuristics")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	setValidContract(t, svc, cr.ID)
	hint := "medium"
	if _, err := svc.SetCRContract(cr.ID, ContractPatch{RiskTierHint: &hint}); err != nil {
		t.Fatalf("SetCRContract(risk hint) error = %v", err)
	}

	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module tmp\n"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	runGit(t, dir, "rm", "delete_me.txt")
	runGit(t, dir, "add", "go.mod")
	runGit(t, dir, "commit", "-m", "feat: dependency + deletion")

	impact, err := svc.ImpactCR(cr.ID)
	if err != nil {
		t.Fatalf("ImpactCR() error = %v", err)
	}
	if impact.RiskTier != "medium" && impact.RiskTier != "high" {
		t.Fatalf("expected computed tier >= medium, got %#v", impact)
	}
	if impact.RiskTierFloorApplied {
		t.Fatalf("expected no floor application when hint <= computed tier, got %#v", impact)
	}
	if containsSignal(impact.Signals, "risk_tier_hint_floor") {
		t.Fatalf("did not expect risk_tier_hint_floor signal, got %#v", impact.Signals)
	}
}

func TestImpactCRRiskTierHintRaisesLowToMediumFloor(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")

	cr, err := svc.AddCR("Hint floor medium", "low -> medium floor")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	setValidContract(t, svc, cr.ID)
	hint := "medium"
	if _, err := svc.SetCRContract(cr.ID, ContractPatch{RiskTierHint: &hint}); err != nil {
		t.Fatalf("SetCRContract(risk hint) error = %v", err)
	}

	if err := os.WriteFile(filepath.Join(dir, "simple.txt"), []byte("x\n"), 0o644); err != nil {
		t.Fatalf("write simple file: %v", err)
	}
	runGit(t, dir, "add", "simple.txt")
	runGit(t, dir, "commit", "-m", "feat: simple change")

	impact, err := svc.ImpactCR(cr.ID)
	if err != nil {
		t.Fatalf("ImpactCR() error = %v", err)
	}
	if impact.RiskTier != "medium" {
		t.Fatalf("expected medium risk tier after floor, got %#v", impact)
	}
	if impact.RiskScore < 3 {
		t.Fatalf("expected score >= 3 after medium floor, got %#v", impact)
	}
	if !impact.RiskTierFloorApplied {
		t.Fatalf("expected floor applied, got %#v", impact)
	}
	if !containsSignal(impact.Signals, "risk_tier_hint_floor") {
		t.Fatalf("expected risk_tier_hint_floor signal, got %#v", impact.Signals)
	}
}

func TestImpactCRRiskTierHintRaisesMediumToHighFloor(t *testing.T) {
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
	runGit(t, dir, "commit", "-m", "chore: seed")

	cr, err := svc.AddCR("Hint floor high", "medium -> high floor")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	setValidContract(t, svc, cr.ID)
	hint := "high"
	if _, err := svc.SetCRContract(cr.ID, ContractPatch{RiskTierHint: &hint}); err != nil {
		t.Fatalf("SetCRContract(risk hint) error = %v", err)
	}

	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module tmp\n"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	runGit(t, dir, "rm", "delete_me.txt")
	runGit(t, dir, "add", "go.mod")
	runGit(t, dir, "commit", "-m", "feat: medium-risk change")

	impact, err := svc.ImpactCR(cr.ID)
	if err != nil {
		t.Fatalf("ImpactCR() error = %v", err)
	}
	if impact.RiskTier != "high" {
		t.Fatalf("expected high risk tier after floor, got %#v", impact)
	}
	if impact.RiskScore < 7 {
		t.Fatalf("expected score >= 7 after high floor, got %#v", impact)
	}
	if !impact.RiskTierFloorApplied {
		t.Fatalf("expected floor applied, got %#v", impact)
	}
	if !containsSignal(impact.Signals, "risk_tier_hint_floor") {
		t.Fatalf("expected risk_tier_hint_floor signal, got %#v", impact.Signals)
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
		if event.Type == model.EventTypeCRMergeOverridden {
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
