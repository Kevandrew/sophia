package cli

import (
	"fmt"
	"testing"

	"sophia/internal/service"
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
