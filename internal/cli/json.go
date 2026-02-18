package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/spf13/cobra"

	"sophia/internal/service"
)

type handledError struct {
	err error
}

func (h *handledError) Error() string {
	if h == nil || h.err == nil {
		return ""
	}
	return h.err.Error()
}

func (h *handledError) Unwrap() error {
	if h == nil {
		return nil
	}
	return h.err
}

func markHandled(err error) error {
	if err == nil {
		return nil
	}
	return &handledError{err: err}
}

func IsHandledError(err error) bool {
	var handled *handledError
	return errors.As(err, &handled)
}

type jsonEnvelope struct {
	OK    bool       `json:"ok"`
	Data  any        `json:"data,omitempty"`
	Error *jsonError `json:"error,omitempty"`
}

type jsonError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details any    `json:"details,omitempty"`
}

var (
	requiredActiveBranchPattern = regexp.MustCompile(`active CR branch "([^"]+)"`)
	crBranchIDPattern           = regexp.MustCompile(`^sophia/cr-(\d+)$`)
)

func writeJSONSuccess(cmd *cobra.Command, payload any) error {
	enc := json.NewEncoder(cmd.OutOrStdout())
	enc.SetIndent("", "  ")
	return enc.Encode(jsonEnvelope{
		OK:   true,
		Data: payload,
	})
}

func writeJSONError(cmd *cobra.Command, err error) error {
	enc := json.NewEncoder(cmd.OutOrStdout())
	enc.SetIndent("", "  ")
	_ = enc.Encode(jsonEnvelope{
		OK: false,
		Error: &jsonError{
			Code:    jsonErrorCode(err),
			Message: err.Error(),
			Details: jsonErrorDetails(err),
		},
	})
	return markHandled(err)
}

func jsonErrorCode(err error) string {
	switch {
	case errors.Is(err, service.ErrPolicyInvalid):
		return "policy_invalid"
	case errors.Is(err, service.ErrPolicyViolation):
		return "policy_violation"
	case errors.Is(err, service.ErrMergeConflict):
		return "merge_conflict"
	case errors.Is(err, service.ErrMergeInProgress):
		return "merge_in_progress"
	case errors.Is(err, service.ErrNoMergeInProgress):
		return "no_merge_in_progress"
	case errors.Is(err, service.ErrNoActiveCRContext):
		return "no_active_cr_context"
	case errors.Is(err, service.ErrBranchInOtherWorktree):
		return "branch_in_other_worktree"
	case errors.Is(err, service.ErrWorkingTreeDirty):
		return "working_tree_dirty"
	case errors.Is(err, service.ErrNoCRChanges):
		return "no_changes"
	case errors.Is(err, service.ErrCRValidationFailed):
		return "validation_failed"
	case errors.Is(err, service.ErrAlreadyRedacted):
		return "already_redacted"
	case errors.Is(err, service.ErrNoTaskChanges):
		return "no_task_changes"
	case errors.Is(err, service.ErrTaskScopeRequired):
		return "task_scope_required"
	case errors.Is(err, service.ErrInvalidTaskScope):
		return "invalid_task_scope"
	case errors.Is(err, service.ErrPreStagedChanges):
		return "pre_staged_changes"
	case errors.Is(err, service.ErrTaskContractIncomplete):
		return "task_contract_incomplete"
	case errors.Is(err, service.ErrNoTaskScopeMatches):
		return "no_task_scope_matches"
	case errors.Is(err, service.ErrTaskNotDone):
		return "task_not_done"
	}
	lower := strings.ToLower(strings.TrimSpace(err.Error()))
	switch {
	case strings.Contains(lower, "requires active cr branch"):
		return "no_active_cr_context"
	case strings.Contains(lower, "not found"):
		return "not_found"
	case strings.Contains(lower, "validation failed"):
		return "validation_failed"
	case strings.Contains(lower, "invalid"):
		return "invalid_argument"
	}
	return "internal_error"
}

func jsonErrorDetails(err error) any {
	type detailer interface {
		Details() map[string]any
	}
	var withDetails detailer
	if errors.As(err, &withDetails) {
		details := withDetails.Details()
		if len(details) > 0 {
			return details
		}
	}
	if action := suggestedActionForError(err); strings.TrimSpace(action) != "" {
		return map[string]any{
			"suggested_action": action,
		}
	}
	return nil
}

func suggestedActionForError(err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, service.ErrNoActiveCRContext) {
		return "sophia cr switch <id>"
	}
	msg := strings.TrimSpace(err.Error())
	if msg == "" {
		return ""
	}
	matches := requiredActiveBranchPattern.FindStringSubmatch(msg)
	if len(matches) == 2 {
		branch := strings.TrimSpace(matches[1])
		idMatches := crBranchIDPattern.FindStringSubmatch(branch)
		if len(idMatches) == 2 {
			return fmt.Sprintf("sophia cr switch %s", strings.TrimSpace(idMatches[1]))
		}
		return "sophia cr switch <id>"
	}
	if strings.Contains(strings.ToLower(msg), "requires active cr branch") {
		return "sophia cr switch <id>"
	}
	return ""
}
