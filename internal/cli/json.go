package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	clijson "sophia/internal/cli/json"
	"sophia/internal/store"
	"strings"

	"github.com/spf13/cobra"

	"sophia/internal/service"
)

func markHandled(err error) error {
	return clijson.MarkHandled(err)
}

func IsHandledError(err error) bool {
	return clijson.IsHandled(err)
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
			Details: jsonErrorDetails(cmd, err),
		},
	})
	return markHandled(err)
}

func commandError(cmd *cobra.Command, asJSON bool, err error) error {
	if err == nil {
		return nil
	}
	if asJSON {
		return writeJSONError(cmd, err)
	}
	return err
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
	case errors.Is(err, service.ErrHQNotConfigured):
		return "hq_not_configured"
	case errors.Is(err, service.ErrHQRepoIDRequired):
		return "hq_repo_id_required"
	case errors.Is(err, service.ErrHQTrackedModeBlocked):
		return "hq_tracked_mode_blocked"
	case errors.Is(err, service.ErrHQRemoteMalformedResponse):
		return "hq_malformed_response"
	case errors.Is(err, service.ErrHQIntentDiverged):
		return "hq_intent_diverged"
	case errors.Is(err, service.ErrHQUpstreamMoved):
		return "hq_upstream_moved"
	case errors.Is(err, service.ErrHQPatchConflict):
		return "hq_patch_conflict"
	case errors.Is(err, service.ErrHQTaskSyncUnsupported):
		return "hq_task_sync_unsupported"
	case errors.Is(err, store.ErrNotFound):
		return "not_found"
	case errors.Is(err, store.ErrInvalidArgument):
		return "invalid_argument"
	case errors.Is(err, store.ErrMutationLockTimeout):
		return "resource_busy"
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

func jsonErrorDetails(cmd *cobra.Command, err error) any {
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
	if action := suggestedActionForError(cmd, err); strings.TrimSpace(action) != "" {
		return map[string]any{
			"suggested_action": action,
		}
	}
	return nil
}

func suggestedActionForError(cmd *cobra.Command, err error) string {
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
		if svc, svcErr := newServiceForCmd(cmd); svcErr == nil {
			if id, resolveErr := svc.ResolveCRID(branch); resolveErr == nil && id > 0 {
				return fmt.Sprintf("sophia cr switch %d", id)
			}
		}
		return "sophia cr switch <id>"
	}
	if strings.Contains(strings.ToLower(msg), "requires active cr branch") {
		return "sophia cr switch <id>"
	}
	return ""
}
