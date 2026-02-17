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
	ratio := trustScoreRatio(report.Score, report.Max)
	if ratio < trustAttentionMinRatio || ratio >= trustTrustedMinRatio {
		t.Fatalf("expected needs_attention ratio in [%.2f, %.2f), got %.3f (score=%d max=%d)", trustAttentionMinRatio, trustTrustedMinRatio, ratio, report.Score, report.Max)
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

func TestSelectTrustVerdictUsesRatioThresholds(t *testing.T) {
	cases := []struct {
		name         string
		score        int
		max          int
		hardFailures []string
		wantVerdict  string
	}{
		{name: "trusted at threshold", score: 85, max: 100, wantVerdict: trustVerdictTrusted},
		{name: "trusted above threshold with larger max", score: 94, max: 110, wantVerdict: trustVerdictTrusted},
		{name: "needs attention below trusted threshold", score: 84, max: 100, wantVerdict: trustVerdictNeedsAttention},
		{name: "needs attention at attention threshold", score: 60, max: 100, wantVerdict: trustVerdictNeedsAttention},
		{name: "untrusted below attention threshold", score: 59, max: 100, wantVerdict: trustVerdictUntrusted},
		{name: "hard failure forces untrusted", score: 100, max: 100, hardFailures: []string{"validation failed"}, wantVerdict: trustVerdictUntrusted},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotVerdict, _ := selectTrustVerdict(tc.score, tc.max, tc.hardFailures)
			if gotVerdict != tc.wantVerdict {
				t.Fatalf("selectTrustVerdict(%d,%d,%v) verdict=%q, want %q", tc.score, tc.max, tc.hardFailures, gotVerdict, tc.wantVerdict)
			}
		})
	}
}

func TestParseShortStatMetrics(t *testing.T) {
	cases := []struct {
		name     string
		input    string
		expected shortStatMetrics
	}{
		{
			name:  "full shortstat",
			input: "21 files changed, 995 insertions(+), 70 deletions(-)",
			expected: shortStatMetrics{
				FilesChanged: 21,
				Insertions:   995,
				Deletions:    70,
			},
		},
		{
			name:  "single file insertion only",
			input: "1 file changed, 1 insertion(+)",
			expected: shortStatMetrics{
				FilesChanged: 1,
				Insertions:   1,
				Deletions:    0,
			},
		},
		{
			name:  "derived shortstat format",
			input: "3 file(s) changed (derived from task checkpoint scope)",
			expected: shortStatMetrics{
				FilesChanged: 3,
				Insertions:   0,
				Deletions:    0,
			},
		},
		{
			name:     "empty shortstat",
			input:    "",
			expected: shortStatMetrics{},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := parseShortStatMetrics(tc.input)
			if got != tc.expected {
				t.Fatalf("parseShortStatMetrics(%q)=%#v, want %#v", tc.input, got, tc.expected)
			}
		})
	}
}

func TestTrustReportAppliesChangeMagnitudePenalties(t *testing.T) {
	cr := &model.CR{
		Contract: validTrustContract(),
		Subtasks: []model.Subtask{
			{ID: 1, Status: model.TaskStatusDone, CheckpointCommit: "abc1234"},
		},
	}
	report := buildTrustReport(cr, &ValidationReport{
		Impact: &ImpactReport{
			FilesChanged: 21,
			RiskTier:     "high",
			Signals:      []RiskSignal{{Code: "large_change_set", Points: 2}},
		},
	}, &diffSummary{
		Files:     []string{"a.go", "b.go"},
		TestFiles: []string{"a_test.go"},
		ShortStat: "21 files changed, 995 insertions(+), 70 deletions(-)",
	})

	dimension := trustDimensionByCode(t, report, "change_magnitude")
	if dimension.Score != 4 {
		t.Fatalf("expected change_magnitude score 4, got %d", dimension.Score)
	}
	if !containsAny(dimension.Reasons, "large file surface") {
		t.Fatalf("expected large file surface reason, got %#v", dimension.Reasons)
	}
	if !containsAny(dimension.Reasons, "high insertion volume") {
		t.Fatalf("expected insertion reason, got %#v", dimension.Reasons)
	}
	if !containsAny(dimension.Reasons, "high-risk tier with broad change surface") {
		t.Fatalf("expected high-risk/broad-surface reason, got %#v", dimension.Reasons)
	}
}

func TestTrustReportHighRiskWithoutSpecializedEvidenceNeedsAttention(t *testing.T) {
	cr := &model.CR{
		Contract: validTrustContract(),
		Subtasks: []model.Subtask{
			{ID: 1, Status: model.TaskStatusDone, CheckpointCommit: "abc1234"},
		},
	}
	report := buildTrustReport(cr, &ValidationReport{
		Impact: &ImpactReport{
			FilesChanged:              4,
			RiskTier:                  "high",
			MatchedRiskCriticalScopes: []string{"internal/service/service_trust.go"},
			Signals:                   []RiskSignal{{Code: "critical_scope_hint", Points: 3}},
			TestFiles:                 []string{"internal/service/service_trust_test.go"},
		},
	}, &diffSummary{
		Files:     []string{"internal/service/service_trust.go", "internal/service/service_trust_test.go"},
		TestFiles: []string{"internal/service/service_trust_test.go"},
		ShortStat: "2 files changed, 20 insertions(+), 3 deletions(-)",
	})

	if report.Verdict != trustVerdictNeedsAttention {
		t.Fatalf("expected needs_attention verdict for high-risk without specialized evidence, got %q", report.Verdict)
	}
	if !containsAny(report.RequiredActions, "specialized high-risk evidence") {
		t.Fatalf("expected specialized high-risk action, got %#v", report.RequiredActions)
	}
	if !containsAny(report.RequiredActions, "Spot-check critical scopes") {
		t.Fatalf("expected spot-check required action, got %#v", report.RequiredActions)
	}
}

func TestTrustReportHighRiskWithSpecializedEvidenceCanBeTrusted(t *testing.T) {
	cr := &model.CR{
		Contract: validTrustContract(),
		Subtasks: []model.Subtask{
			{ID: 1, Status: model.TaskStatusDone, CheckpointCommit: "abc1234"},
		},
	}
	report := buildTrustReport(cr, &ValidationReport{
		Impact: &ImpactReport{
			FilesChanged:              4,
			RiskTier:                  "high",
			MatchedRiskCriticalScopes: []string{"internal/service/service_trust.go"},
			Signals:                   []RiskSignal{{Code: "critical_scope_hint", Points: 3}},
			TestFiles:                 []string{"internal/service/worktree_integration_test.go"},
		},
	}, &diffSummary{
		Files:     []string{"internal/service/service_trust.go", "internal/service/worktree_integration_test.go"},
		TestFiles: []string{"internal/service/worktree_integration_test.go"},
		ShortStat: "2 files changed, 20 insertions(+), 3 deletions(-)",
	})

	if report.Verdict != trustVerdictTrusted {
		t.Fatalf("expected trusted verdict when specialized evidence exists, got %q", report.Verdict)
	}
}

func TestTrustDimensionsKeepCodesAndUseUpdatedLabels(t *testing.T) {
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
			Signals:      []RiskSignal{{Code: "large_change_set", Points: 2}},
		},
	}, &diffSummary{
		Files:     []string{"internal/service/a.go", "internal/service/a_test.go"},
		TestFiles: []string{"internal/service/a_test.go"},
		ShortStat: "2 files changed, 10 insertions(+), 2 deletions(-)",
	})

	expected := map[string]string{
		"contract_quality":    "Contract Completeness",
		"scope_discipline":    "Scope Alignment",
		"task_proof_chain":    "Checkpoint Coverage",
		"risk_accountability": "Risk Declaration",
		"change_magnitude":    "Change Magnitude",
		"validation_health":   "Validation Status",
		"test_evidence":       "Test Touch Signals",
	}
	for code, label := range expected {
		dimension := trustDimensionByCode(t, report, code)
		if dimension.Label != label {
			t.Fatalf("expected label %q for code %q, got %q", label, code, dimension.Label)
		}
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
