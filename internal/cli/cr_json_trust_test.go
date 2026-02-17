package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sophia/internal/service"
)

func TestCRReviewJSONIncludesTrustEnvelope(t *testing.T) {
	dir := t.TempDir()
	svc := service.New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")

	cr, err := svc.AddCR("Trust JSON", "trust envelope json output")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	why := "Provide deterministic trust metadata for review confidence."
	scope := []string{"feature.txt"}
	nonGoals := []string{"No merge gating changes in this CR."}
	invariants := []string{"Existing review JSON envelope stays compatible."}
	blast := "Review and JSON output only."
	testPlan := "Run go test ./... and go vet ./..."
	rollback := "Revert CR merge commit."
	if _, err := svc.SetCRContract(cr.ID, service.ContractPatch{
		Why:          &why,
		Scope:        &scope,
		NonGoals:     &nonGoals,
		Invariants:   &invariants,
		BlastRadius:  &blast,
		TestPlan:     &testPlan,
		RollbackPlan: &rollback,
	}); err != nil {
		t.Fatalf("SetCRContract() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "feature.txt"), []byte("trust\n"), 0o644); err != nil {
		t.Fatalf("write feature file: %v", err)
	}
	runGit(t, dir, "add", "feature.txt")
	runGit(t, dir, "commit", "-m", "feat: trust fixture")

	out, _, runErr := runCLI(t, dir, "cr", "review", "1", "--json")
	if runErr != nil {
		t.Fatalf("cr review --json error = %v\noutput=%s", runErr, out)
	}
	env := decodeEnvelope(t, out)
	trust, ok := env.Data["trust"].(map[string]any)
	if !ok {
		t.Fatalf("expected trust object, got %#v", env.Data["trust"])
	}
	verdict, _ := trust["verdict"].(string)
	if strings.TrimSpace(verdict) == "" {
		t.Fatalf("expected trust verdict, got %#v", trust)
	}
	if _, ok := trust["score"]; !ok {
		t.Fatalf("expected trust score key, got %#v", trust)
	}
	if _, ok := trust["advisory_only"]; !ok {
		t.Fatalf("expected advisory_only key, got %#v", trust)
	}
	dimensions, ok := trust["dimensions"].([]any)
	if !ok || len(dimensions) == 0 {
		t.Fatalf("expected trust dimensions array, got %#v", trust["dimensions"])
	}
	gotCodes := map[string]string{}
	for _, entry := range dimensions {
		dimension, ok := entry.(map[string]any)
		if !ok {
			t.Fatalf("expected dimension object, got %#v", entry)
		}
		code, _ := dimension["code"].(string)
		label, _ := dimension["label"].(string)
		if strings.TrimSpace(code) != "" {
			gotCodes[code] = label
		}
	}
	for _, code := range []string{
		"contract_quality",
		"scope_discipline",
		"task_proof_chain",
		"risk_accountability",
		"change_magnitude",
		"validation_health",
		"test_evidence",
	} {
		if _, ok := gotCodes[code]; !ok {
			t.Fatalf("expected trust dimension code %q, got %#v", code, gotCodes)
		}
	}
	if gotCodes["contract_quality"] != "Contract Completeness" {
		t.Fatalf("expected updated contract_quality label, got %q", gotCodes["contract_quality"])
	}
	if gotCodes["change_magnitude"] != "Change Magnitude" {
		t.Fatalf("expected change_magnitude label, got %q", gotCodes["change_magnitude"])
	}
}

func TestCRReviewTextIncludesTrustSection(t *testing.T) {
	dir := t.TempDir()
	svc := service.New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")

	cr, err := svc.AddCR("Trust Text", "trust envelope text output")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	why := "Show trust verdict and required actions directly in review output."
	scope := []string{"trust_text_fixture.go"}
	nonGoals := []string{"No remote features."}
	invariants := []string{"CR metadata remains additive and deterministic."}
	blast := "Review formatting output only."
	testPlan := "Run go test ./... and go vet ./..."
	rollback := "Revert review formatting commit."
	if _, err := svc.SetCRContract(cr.ID, service.ContractPatch{
		Why:          &why,
		Scope:        &scope,
		NonGoals:     &nonGoals,
		Invariants:   &invariants,
		BlastRadius:  &blast,
		TestPlan:     &testPlan,
		RollbackPlan: &rollback,
	}); err != nil {
		t.Fatalf("SetCRContract() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "trust_text_fixture.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("write fixture file: %v", err)
	}
	runGit(t, dir, "add", "trust_text_fixture.go")
	runGit(t, dir, "commit", "-m", "feat: trust text fixture")

	out, _, runErr := runCLI(t, dir, "cr", "review", "1")
	if runErr != nil {
		t.Fatalf("cr review error = %v\noutput=%s", runErr, out)
	}
	for _, required := range []string{"Trust:", "Verdict:", "Score:", "Advisory Only:", "Dimensions:", "Contract Completeness", "Change Magnitude"} {
		if !strings.Contains(out, required) {
			t.Fatalf("expected review output to contain %q, got:\n%s", required, out)
		}
	}
}
