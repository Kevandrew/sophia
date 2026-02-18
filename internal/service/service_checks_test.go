package service

import (
	"strings"
	"testing"
)

func TestRunTrustChecksCRExecutesRequiredCommands(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")
	writePolicyFileForTest(t, dir, `version: v1
trust:
  mode: advisory
  checks:
    freshness_hours: 24
    definitions:
      - key: smoke_check
        command: "printf 'ok\n'"
        tiers: [low, medium, high]
        allow_exit_codes: [0]
`)

	cr, err := svc.AddCR("trust checks run", "")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}

	report, err := svc.RunTrustChecksCR(cr.ID)
	if err != nil {
		t.Fatalf("RunTrustChecksCR() error = %v", err)
	}
	if report.Executed != 1 {
		t.Fatalf("expected executed=1, got %d", report.Executed)
	}
	if len(report.CheckResults) != 1 {
		t.Fatalf("expected one check result, got %#v", report.CheckResults)
	}
	if report.CheckResults[0].Status != policyTrustCheckStatusPass {
		t.Fatalf("expected pass status, got %#v", report.CheckResults[0])
	}

	evidence, err := svc.ListEvidence(cr.ID)
	if err != nil {
		t.Fatalf("ListEvidence() error = %v", err)
	}
	if len(evidence) == 0 {
		t.Fatalf("expected command_run evidence entry")
	}
	found := false
	for _, entry := range evidence {
		if entry.Type == evidenceTypeCommandRun && strings.HasPrefix(entry.Command, "printf") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected command_run evidence for trust check, got %#v", evidence)
	}
}

func TestTrustCheckStatusMarksStaleEvidence(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")
	writePolicyFileForTest(t, dir, `version: v1
trust:
  mode: advisory
  checks:
    freshness_hours: 1
    definitions:
      - key: stale_check
        command: "echo stale"
        tiers: [low, medium, high]
        allow_exit_codes: [0]
`)

	cr, err := svc.AddCR("trust stale check", "")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	exitCode := 0
	if _, err := svc.AddEvidence(cr.ID, AddEvidenceOptions{
		Type:     evidenceTypeCommandRun,
		Command:  "echo stale",
		ExitCode: &exitCode,
		Summary:  "stale fixture",
	}); err != nil {
		t.Fatalf("AddEvidence() error = %v", err)
	}

	storedCR, err := svc.store.LoadCR(cr.ID)
	if err != nil {
		t.Fatalf("LoadCR() error = %v", err)
	}
	storedCR.Evidence[len(storedCR.Evidence)-1].TS = "2000-01-01T00:00:00Z"
	if err := svc.store.SaveCR(storedCR); err != nil {
		t.Fatalf("SaveCR() error = %v", err)
	}

	report, err := svc.TrustCheckStatusCR(cr.ID)
	if err != nil {
		t.Fatalf("TrustCheckStatusCR() error = %v", err)
	}
	if len(report.CheckResults) != 1 {
		t.Fatalf("expected one check result, got %#v", report.CheckResults)
	}
	if report.CheckResults[0].Status != policyTrustCheckStatusStale {
		t.Fatalf("expected stale status, got %#v", report.CheckResults[0])
	}
}
