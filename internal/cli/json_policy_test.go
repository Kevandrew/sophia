package cli

import (
	"fmt"
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
