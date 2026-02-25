package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sophia/internal/service"
)

func TestCRContractSetAndShowSupportsRiskHintFields(t *testing.T) {
	dir := t.TempDir()
	svc := service.New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	cr, err := svc.AddCR("Risk hints", "contract fields")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	_ = cr

	out, _, runErr := runCLI(
		t,
		dir,
		"cr", "contract", "set", "1",
		"--risk-critical-scope", "internal/service",
		"--risk-critical-scope", "cmd",
		"--risk-tier-hint", "HIGH",
		"--risk-rationale", "touches key runtime paths",
	)
	if runErr != nil {
		t.Fatalf("cr contract set risk fields error = %v\noutput=%s", runErr, out)
	}
	if !strings.Contains(out, "risk_critical_scopes") || !strings.Contains(out, "risk_tier_hint") {
		t.Fatalf("expected changed fields to include risk hints, got %q", out)
	}

	out, _, runErr = runCLI(t, dir, "cr", "contract", "show", "1")
	if runErr != nil {
		t.Fatalf("cr contract show error = %v\noutput=%s", runErr, out)
	}
	for _, want := range []string{
		"risk_critical_scopes",
		"cmd",
		"internal/service",
		"risk_tier_hint: high",
		"risk_rationale: touches key runtime paths",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected contract show output to contain %q, got:\n%s", want, out)
		}
	}
}

func TestCRImpactJSONIncludesRiskHintProvenanceFields(t *testing.T) {
	dir := t.TempDir()
	svc := service.New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	cr, err := svc.AddCR("Impact JSON risk hints", "json fields")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	setValidContractCLI(t, svc, cr.ID)
	riskScopes := []string{"internal/service"}
	riskHint := "high"
	if _, err := svc.SetCRContract(cr.ID, service.ContractPatch{
		RiskCriticalScopes: &riskScopes,
		RiskTierHint:       &riskHint,
	}); err != nil {
		t.Fatalf("SetCRContract(risk hints) error = %v", err)
	}

	if err := os.MkdirAll(filepath.Join(dir, "internal", "service"), 0o755); err != nil {
		t.Fatalf("mkdir internal/service: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "internal", "service", "x.go"), []byte("package service\n"), 0o644); err != nil {
		t.Fatalf("write risk scoped file: %v", err)
	}
	runGit(t, dir, "add", "internal/service/x.go")
	runGit(t, dir, "commit", "-m", "feat: risk scoped change")

	out, _, runErr := runCLI(t, dir, "cr", "impact", "1", "--json")
	if runErr != nil {
		t.Fatalf("cr impact --json error = %v\noutput=%s", runErr, out)
	}
	env := decodeEnvelope(t, out)
	if !env.OK {
		t.Fatalf("expected ok impact envelope, got %#v", env)
	}
	if _, ok := env.Data["risk_tier_hint"]; !ok {
		t.Fatalf("expected risk_tier_hint in impact JSON, got %#v", env.Data)
	}
	if _, ok := env.Data["risk_tier_floor_applied"]; !ok {
		t.Fatalf("expected risk_tier_floor_applied in impact JSON, got %#v", env.Data)
	}
	matched, ok := env.Data["matched_risk_critical_scopes"].([]any)
	if !ok {
		t.Fatalf("expected matched_risk_critical_scopes array, got %#v", env.Data["matched_risk_critical_scopes"])
	}
	if len(matched) != 1 || matched[0] != "internal/service" {
		t.Fatalf("expected matched risk scope [internal/service], got %#v", matched)
	}
}
