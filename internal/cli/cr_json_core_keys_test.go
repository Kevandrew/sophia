package cli

import (
	"strings"
	"testing"

	"sophia/internal/service"
)

func TestCoreCRJSONOutputsIncludeRequiredKeys(t *testing.T) {
	dir := t.TempDir()
	svc := service.New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	cr, err := svc.AddCR("Core JSON keys", "compat guard")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	setValidContractCLI(t, svc, cr.ID)

	cases := []struct {
		name string
		args []string
		keys []string
	}{
		{
			name: "impact",
			args: []string{"cr", "impact", "1", "--json"},
			keys: []string{"cr_id", "risk_tier", "risk_score", "files_changed", "warnings"},
		},
		{
			name: "review",
			args: []string{"cr", "review", "1", "--json"},
			keys: []string{"cr", "impact", "trust", "validation_errors", "validation_warnings"},
		},
		{
			name: "validate",
			args: []string{"cr", "validate", "1", "--json"},
			keys: []string{"valid", "errors", "warnings", "impact"},
		},
		{
			name: "status",
			args: []string{"cr", "status", "1", "--json"},
			keys: []string{"id", "uid", "title", "working_tree", "validation", "merge_blocked"},
		},
		{
			name: "check status",
			args: []string{"cr", "check", "status", "1", "--json"},
			keys: []string{"cr_id", "check_mode", "required_check_count", "check_results", "guidance"},
		},
	}

	for _, tc := range cases {
		out, _, runErr := runCLI(t, dir, tc.args...)
		if runErr != nil {
			t.Fatalf("%s (%q) error = %v\noutput=%s", tc.name, strings.Join(tc.args, " "), runErr, out)
		}
		env := decodeEnvelope(t, out)
		if !env.OK {
			t.Fatalf("%s expected ok envelope, got %#v", tc.name, env)
		}
		for _, key := range tc.keys {
			if _, ok := env.Data[key]; !ok {
				t.Fatalf("%s expected key %q in payload, got %#v", tc.name, key, env.Data)
			}
		}
	}
}
