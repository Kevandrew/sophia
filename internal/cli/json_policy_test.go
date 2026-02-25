package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sophia/internal/service"
	"sophia/internal/store"
)

func TestJSONErrorCodePolicyErrors(t *testing.T) {
	if got := jsonErrorCode(service.ErrPolicyInvalid); got != "policy_invalid" {
		t.Fatalf("jsonErrorCode(ErrPolicyInvalid) = %q, want policy_invalid", got)
	}
	if got := jsonErrorCode(service.ErrPolicyViolation); got != "policy_violation" {
		t.Fatalf("jsonErrorCode(ErrPolicyViolation) = %q, want policy_violation", got)
	}
	if got := jsonErrorCode(fmt.Errorf("wrapped: %w", service.ErrPolicyInvalid)); got != "policy_invalid" {
		t.Fatalf("jsonErrorCode(wrapped ErrPolicyInvalid) = %q, want policy_invalid", got)
	}
	if got := jsonErrorCode(fmt.Errorf("wrapped: %w", service.ErrPolicyViolation)); got != "policy_violation" {
		t.Fatalf("jsonErrorCode(wrapped ErrPolicyViolation) = %q, want policy_violation", got)
	}
}

func TestJSONErrorCodeStoreTypedErrors(t *testing.T) {
	if got := jsonErrorCode(store.NotFoundError{Resource: "cr", Value: "123"}); got != "not_found" {
		t.Fatalf("jsonErrorCode(store.NotFoundError) = %q, want not_found", got)
	}
	if got := jsonErrorCode(fmt.Errorf("wrapped: %w", store.NotFoundError{Resource: "cr uid", Value: "cr_abc"})); got != "not_found" {
		t.Fatalf("jsonErrorCode(wrapped store.NotFoundError) = %q, want not_found", got)
	}
	if got := jsonErrorCode(store.InvalidArgumentError{Argument: "cr uid", Message: "cannot be empty"}); got != "invalid_argument" {
		t.Fatalf("jsonErrorCode(store.InvalidArgumentError) = %q, want invalid_argument", got)
	}
	if got := jsonErrorCode(fmt.Errorf("wrapped: %w", store.InvalidArgumentError{Argument: "selector", Message: "cannot be empty"})); got != "invalid_argument" {
		t.Fatalf("jsonErrorCode(wrapped store.InvalidArgumentError) = %q, want invalid_argument", got)
	}
}

func TestValidateAndDoctorJSONSurfacePolicyUnknownFieldWarnings(t *testing.T) {
	dir := t.TempDir()
	svc := service.New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	cr, err := svc.AddCR("policy warning json", "warning surface")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	setValidContractCLI(t, svc, cr.ID)
	if err := os.WriteFile(filepath.Join(dir, "SOPHIA.yaml"), []byte("version: v1\nunknown_key: true\n"), 0o644); err != nil {
		t.Fatalf("write SOPHIA.yaml: %v", err)
	}

	validateOut, _, validateErr := runCLI(t, dir, "cr", "validate", "1", "--json")
	if validateErr != nil {
		t.Fatalf("cr validate --json error = %v\noutput=%s", validateErr, validateOut)
	}
	validateEnv := decodeEnvelope(t, validateOut)
	if !validateEnv.OK {
		t.Fatalf("expected ok validate envelope, got %#v", validateEnv)
	}
	warnings, ok := validateEnv.Data["warnings"].([]any)
	if !ok {
		t.Fatalf("expected warnings array, got %#v", validateEnv.Data["warnings"])
	}
	foundUnknownWarning := false
	for _, raw := range warnings {
		text, _ := raw.(string)
		if strings.Contains(text, `unknown field "unknown_key"`) {
			foundUnknownWarning = true
			break
		}
	}
	if !foundUnknownWarning {
		t.Fatalf("expected unknown-field warning in validate JSON, got %#v", warnings)
	}

	doctorOut, _, doctorErr := runCLI(t, dir, "doctor", "--json")
	if doctorErr != nil {
		t.Fatalf("doctor --json error = %v\noutput=%s", doctorErr, doctorOut)
	}
	doctorEnv := decodeEnvelope(t, doctorOut)
	if !doctorEnv.OK {
		t.Fatalf("expected ok doctor envelope, got %#v", doctorEnv)
	}
	doctorFindings, ok := doctorEnv.Data["findings"].([]any)
	if !ok {
		t.Fatalf("expected doctor findings array, got %#v", doctorEnv.Data["findings"])
	}
	foundPolicyFinding := false
	for _, raw := range doctorFindings {
		finding, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if code, _ := finding["code"].(string); code == "policy_unknown_fields" {
			foundPolicyFinding = true
			break
		}
	}
	if !foundPolicyFinding {
		t.Fatalf("expected policy_unknown_fields in doctor findings, got %#v", doctorFindings)
	}
}
