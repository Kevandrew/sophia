package service

import (
	"sophia/internal/model"
	"testing"
)

func TestTrustReportValidationErrorsHardFail(t *testing.T) {
	cr := &model.CR{
		Contract: validTrustContract(),
	}
	report := buildTrustReport(cr, &ValidationReport{
		Errors: []string{"scope drift detected"},
		Impact: &ImpactReport{
			FilesChanged: 1,
			RiskTier:     "low",
			Signals:      []RiskSignal{{Code: "large_change_set", Points: 2}},
		},
	}, &diffSummary{
		Files: []string{"internal/service/a.go"},
	})

	if report.Verdict != trustVerdictUntrusted {
		t.Fatalf("expected untrusted verdict, got %q", report.Verdict)
	}
	if !containsAny(report.HardFailures, "validation errors present") {
		t.Fatalf("expected validation hard failure, got %#v", report.HardFailures)
	}
}

func TestTrustReportTrustedWhenEvidenceStrong(t *testing.T) {
	cr := &model.CR{
		Contract: validTrustContract(),
		Subtasks: []model.Subtask{
			{ID: 1, Status: model.TaskStatusDone, CheckpointCommit: "abc1234"},
		},
	}
	report := buildTrustReport(cr, &ValidationReport{
		Impact: &ImpactReport{
			FilesChanged: 2,
			RiskTier:     "low",
			Signals: []RiskSignal{
				{Code: "large_change_set", Points: 2},
			},
		},
	}, &diffSummary{
		Files:     []string{"internal/service/a.go", "internal/service/a_test.go"},
		TestFiles: []string{"internal/service/a_test.go"},
	})

	if report.Verdict != trustVerdictTrusted {
		t.Fatalf("expected trusted verdict, got %q (score=%d)", report.Verdict, report.Score)
	}
	if report.Score < 85 {
		t.Fatalf("expected trusted score >= 85, got %d", report.Score)
	}
	if len(report.HardFailures) != 0 {
		t.Fatalf("expected no hard failures, got %#v", report.HardFailures)
	}
}

func TestTrustReportWarningHeavyNeedsAttention(t *testing.T) {
	cr := &model.CR{
		Contract: validTrustContract(),
	}
	report := buildTrustReport(cr, &ValidationReport{
		Warnings: []string{"scope warning 1", "scope warning 2", "scope warning 3"},
		Impact: &ImpactReport{
			FilesChanged: 1,
			RiskTier:     "medium",
			ScopeDrift:   []string{"internal/service/a.go"},
		},
	}, &diffSummary{
		Files: []string{"internal/service/a.go"},
	})

	if report.Verdict != trustVerdictNeedsAttention {
		t.Fatalf("expected needs_attention verdict, got %q (score=%d)", report.Verdict, report.Score)
	}
	if report.Score < 60 || report.Score > 84 {
		t.Fatalf("expected needs_attention score in [60,84], got %d", report.Score)
	}
	if len(report.HardFailures) != 0 {
		t.Fatalf("expected no hard failures, got %#v", report.HardFailures)
	}
}

func TestTrustReportAppliesWeakContractTextPenalties(t *testing.T) {
	cr := &model.CR{
		Contract: model.Contract{
			Why:          "todo",
			Scope:        []string{"internal/service"},
			BlastRadius:  "n/a",
			TestPlan:     "...",
			RollbackPlan: "none",
		},
	}
	report := buildTrustReport(cr, &ValidationReport{
		Impact: &ImpactReport{
			FilesChanged: 1,
			RiskTier:     "low",
			Signals:      []RiskSignal{{Code: "large_change_set", Points: 2}},
		},
	}, &diffSummary{
		Files: []string{"internal/service/a.go"},
	})

	dimension := trustDimensionByCode(t, report, "contract_quality")
	if dimension.Score != 4 {
		t.Fatalf("expected contract_quality score 4, got %d", dimension.Score)
	}
	if !containsAny(dimension.Reasons, "why is weak") {
		t.Fatalf("expected weak why reason, got %#v", dimension.Reasons)
	}
}

func TestTrustReportPenalizesDependencyChangesWithoutTests(t *testing.T) {
	cr := &model.CR{
		Contract: validTrustContract(),
	}
	report := buildTrustReport(cr, &ValidationReport{
		Impact: &ImpactReport{
			FilesChanged: 1,
			RiskTier:     "medium",
			Signals:      []RiskSignal{{Code: "dependency_changes", Points: 3}},
		},
	}, &diffSummary{
		Files:           []string{"go.mod"},
		DependencyFiles: []string{"go.mod"},
	})

	dimension := trustDimensionByCode(t, report, "test_evidence")
	if dimension.Score != 4 {
		t.Fatalf("expected test_evidence score 4, got %d", dimension.Score)
	}
	if !containsAny(dimension.Reasons, "dependency changes without test evidence") {
		t.Fatalf("expected dependency/no-tests reason, got %#v", dimension.Reasons)
	}
}

func TestTrustReportPenalizesDelegatedPendingTasks(t *testing.T) {
	cr := &model.CR{
		Contract: validTrustContract(),
		Subtasks: []model.Subtask{
			{ID: 1, Status: model.TaskStatusDone, CheckpointCommit: "abc1234"},
			{ID: 2, Status: model.TaskStatusDelegated},
		},
	}
	report := buildTrustReport(cr, &ValidationReport{
		Impact: &ImpactReport{
			FilesChanged: 2,
			RiskTier:     "low",
			Signals:      []RiskSignal{{Code: "large_change_set", Points: 2}},
		},
	}, &diffSummary{
		Files:     []string{"internal/service/a.go", "internal/service/a_test.go"},
		TestFiles: []string{"internal/service/a_test.go"},
	})

	dimension := trustDimensionByCode(t, report, "task_proof_chain")
	if dimension.Score != 17 {
		t.Fatalf("expected task_proof_chain score 17, got %d", dimension.Score)
	}
	if !containsAny(dimension.Reasons, "delegated tasks still pending") {
		t.Fatalf("expected delegated pending reason, got %#v", dimension.Reasons)
	}
}

func TestTrustReportNoTaskCanStillBeNeedsAttention(t *testing.T) {
	cr := &model.CR{
		Contract: validTrustContract(),
	}
	report := buildTrustReport(cr, &ValidationReport{
		Warnings: []string{"minor warning"},
		Impact: &ImpactReport{
			FilesChanged: 1,
			RiskTier:     "medium",
			ScopeDrift:   []string{"internal/service/a.go"},
		},
	}, &diffSummary{
		Files: []string{"internal/service/a.go"},
	})

	if report.Verdict != trustVerdictNeedsAttention {
		t.Fatalf("expected needs_attention verdict, got %q (score=%d)", report.Verdict, report.Score)
	}
	if len(report.HardFailures) != 0 {
		t.Fatalf("expected no hard failures, got %#v", report.HardFailures)
	}
}

func validTrustContract() model.Contract {
	return model.Contract{
		Why:          "Deliver deterministic trust evidence so review can be metadata-first.",
		Scope:        []string{"internal/service"},
		NonGoals:     []string{"No merge gate changes in this CR."},
		Invariants:   []string{"Validation remains deterministic and additive."},
		BlastRadius:  "Review output and evidence scoring paths only.",
		TestPlan:     "Run go test ./... and go vet ./... before merge.",
		RollbackPlan: "Revert the trust-envelope merge commit.",
	}
}

func trustDimensionByCode(t *testing.T, report *TrustReport, code string) TrustDimension {
	t.Helper()
	for _, dimension := range report.Dimensions {
		if dimension.Code == code {
			return dimension
		}
	}
	t.Fatalf("missing trust dimension %q in %#v", code, report.Dimensions)
	return TrustDimension{}
}
