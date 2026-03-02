package cli

import (
	"strings"
	"testing"

	"sophia/internal/service"
)

func TestCoreReadOnlyCommandsResolveActiveCRWhenSelectorOmitted(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := service.New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	cr, err := svc.AddCR("optional selector", "active branch fallback")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	setRequiredCRContractForOptionalSelectorTest(t, svc, cr.ID)

	tests := []struct {
		name   string
		args   []string
		assert func(t *testing.T, env envelope, wantID int)
	}{
		{
			name: "status",
			args: []string{"cr", "status", "--json"},
			assert: func(t *testing.T, env envelope, wantID int) {
				t.Helper()
				got := int(jsonNumberField(t, env.Data, "id"))
				if got != wantID {
					t.Fatalf("expected id %d, got %d", wantID, got)
				}
			},
		},
		{
			name: "why",
			args: []string{"cr", "why", "--json"},
			assert: func(t *testing.T, env envelope, wantID int) {
				t.Helper()
				got := int(jsonNumberField(t, env.Data, "cr_id"))
				if got != wantID {
					t.Fatalf("expected cr_id %d, got %d", wantID, got)
				}
			},
		},
		{
			name: "impact",
			args: []string{"cr", "impact", "--json"},
			assert: func(t *testing.T, env envelope, wantID int) {
				t.Helper()
				got := int(jsonNumberField(t, env.Data, "cr_id"))
				if got != wantID {
					t.Fatalf("expected cr_id %d, got %d", wantID, got)
				}
			},
		},
		{
			name: "validate",
			args: []string{"cr", "validate", "--json"},
			assert: func(t *testing.T, env envelope, _ int) {
				t.Helper()
				if valid, ok := env.Data["valid"].(bool); !ok || !valid {
					t.Fatalf("expected valid=true, got %#v", env.Data["valid"])
				}
			},
		},
		{
			name: "review",
			args: []string{"cr", "review", "--json"},
			assert: func(t *testing.T, env envelope, wantID int) {
				t.Helper()
				crData, ok := env.Data["cr"].(map[string]any)
				if !ok {
					t.Fatalf("expected cr object in review payload, got %#v", env.Data["cr"])
				}
				got := int(jsonNumberField(t, crData, "id"))
				if got != wantID {
					t.Fatalf("expected review.cr.id %d, got %d", wantID, got)
				}
			},
		},
		{
			name: "show",
			args: []string{"cr", "show", "--json", "--no-open"},
			assert: func(t *testing.T, env envelope, wantID int) {
				t.Helper()
				got := int(jsonNumberField(t, env.Data, "cr_id"))
				if got != wantID {
					t.Fatalf("expected cr_id %d, got %d", wantID, got)
				}
				if opened, ok := env.Data["opened"].(bool); !ok || opened {
					t.Fatalf("expected opened=false with --no-open, got %#v", env.Data["opened"])
				}
			},
		},
		{
			name: "doctor",
			args: []string{"cr", "doctor", "--json"},
			assert: func(t *testing.T, env envelope, wantID int) {
				t.Helper()
				got := int(jsonNumberField(t, env.Data, "cr_id"))
				if got != wantID {
					t.Fatalf("expected cr_id %d, got %d", wantID, got)
				}
			},
		},
		{
			name: "check status",
			args: []string{"cr", "check", "status", "--json"},
			assert: func(t *testing.T, env envelope, wantID int) {
				t.Helper()
				got := int(jsonNumberField(t, env.Data, "cr_id"))
				if got != wantID {
					t.Fatalf("expected cr_id %d, got %d", wantID, got)
				}
			},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			out, _, runErr := runCLI(t, dir, tc.args...)
			if runErr != nil {
				t.Fatalf("%s error = %v\noutput=%s", tc.name, runErr, out)
			}
			env := decodeEnvelope(t, out)
			if !env.OK {
				t.Fatalf("expected ok envelope for %s, got %#v", tc.name, env)
			}
			tc.assert(t, env, cr.ID)
		})
	}
}

func TestCoreReadOnlyCommandsWithoutSelectorReturnNoActiveCRContext(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := service.New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if _, _, err := svc.AddCRWithOptionsWithWarnings("optional selector", "no active context", service.AddCROptions{NoSwitch: true}); err != nil {
		t.Fatalf("AddCRWithOptionsWithWarnings() error = %v", err)
	}

	tests := []struct {
		name string
		args []string
	}{
		{name: "status", args: []string{"cr", "status", "--json"}},
		{name: "why", args: []string{"cr", "why", "--json"}},
		{name: "impact", args: []string{"cr", "impact", "--json"}},
		{name: "validate", args: []string{"cr", "validate", "--json"}},
		{name: "review", args: []string{"cr", "review", "--json"}},
		{name: "doctor", args: []string{"cr", "doctor", "--json"}},
		{name: "show", args: []string{"cr", "show", "--json", "--no-open"}},
		{name: "check status", args: []string{"cr", "check", "status", "--json"}},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			out, _, runErr := runCLI(t, dir, tc.args...)
			if runErr == nil {
				t.Fatalf("expected %s to fail without active CR context", tc.name)
			}
			env := decodeEnvelope(t, out)
			if env.OK || env.Error == nil {
				t.Fatalf("expected structured error envelope for %s, got %#v", tc.name, env)
			}
			if env.Error.Code != "no_active_cr_context" {
				t.Fatalf("expected no_active_cr_context for %s, got %q", tc.name, env.Error.Code)
			}
			if strings.Contains(strings.ToLower(env.Error.Message), "accepts 1 arg") {
				t.Fatalf("expected context error for %s, got arity message %q", tc.name, env.Error.Message)
			}
		})
	}
}

func setRequiredCRContractForOptionalSelectorTest(t *testing.T, svc *service.Service, crID int) {
	t.Helper()
	why := "validate optional selector behavior"
	scope := []string{"internal/cli"}
	nonGoals := []string{"no mutation command changes"}
	invariants := []string{"structured errors remain stable"}
	blastRadius := "read-only command selector resolution"
	testPlan := "go test ./internal/cli/..."
	rollback := "revert merge commit"
	if _, err := svc.SetCRContract(crID, service.ContractPatch{
		Why:          &why,
		Scope:        &scope,
		NonGoals:     &nonGoals,
		Invariants:   &invariants,
		BlastRadius:  &blastRadius,
		TestPlan:     &testPlan,
		RollbackPlan: &rollback,
	}); err != nil {
		t.Fatalf("SetCRContract() error = %v", err)
	}
}

func jsonNumberField(t *testing.T, data map[string]any, key string) float64 {
	t.Helper()
	raw, ok := data[key]
	if !ok {
		t.Fatalf("missing key %q in %#v", key, data)
	}
	value, ok := raw.(float64)
	if !ok {
		t.Fatalf("expected numeric key %q, got %#v", key, raw)
	}
	return value
}
