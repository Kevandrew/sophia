package cli

import (
	"strings"

	clijson "sophia/internal/cli/json"
	"sophia/internal/model"
	"sophia/internal/service"
)

func stringSliceOrEmpty(in []string) []string {
	return clijson.StringSliceOrEmpty(in)
}

func intSliceOrEmpty(in []int) []int {
	return clijson.IntSliceOrEmpty(in)
}

func mapStringStringOrEmpty(in map[string]string) map[string]string {
	return clijson.MapStringStringOrEmpty(in)
}

func addCRBootstrapToJSONMap(info service.AddCRBootstrapInfo) map[string]any {
	return map[string]any{
		"triggered":     info.Triggered,
		"base_branch":   info.BaseBranch,
		"metadata_mode": info.MetadataMode,
		"sophia_dir":    info.SophiaDir,
	}
}

func branchIdentityToJSONMap(branch, uid string) map[string]any {
	return clijson.BranchIdentityToMap(branch, uid)
}

func checkpointChunkModelToJSONMap(chunk model.CheckpointChunk) map[string]any {
	return map[string]any{
		"id":        chunk.ID,
		"path":      chunk.Path,
		"old_start": chunk.OldStart,
		"old_lines": chunk.OldLines,
		"new_start": chunk.NewStart,
		"new_lines": chunk.NewLines,
	}
}

func checkpointChunkMaps(chunks []model.CheckpointChunk) []map[string]any {
	out := make([]map[string]any, 0, len(chunks))
	for _, chunk := range chunks {
		out = append(out, checkpointChunkModelToJSONMap(chunk))
	}
	return out
}

func checkpointChunkToDiffJSONMap(chunk model.CheckpointChunk) map[string]any {
	return map[string]any{
		"chunk_id":  chunk.ID,
		"path":      chunk.Path,
		"old_start": chunk.OldStart,
		"old_lines": chunk.OldLines,
		"new_start": chunk.NewStart,
		"new_lines": chunk.NewLines,
	}
}

func checkpointChunkDiffMaps(chunks []model.CheckpointChunk) []map[string]any {
	out := make([]map[string]any, 0, len(chunks))
	for _, chunk := range chunks {
		out = append(out, checkpointChunkToDiffJSONMap(chunk))
	}
	return out
}

func taskDelegationToJSONMap(delegation model.TaskDelegation) map[string]any {
	return map[string]any{
		"child_cr_id":   delegation.ChildCRID,
		"child_cr_uid":  delegation.ChildCRUID,
		"child_task_id": delegation.ChildTaskID,
		"linked_at":     delegation.LinkedAt,
		"linked_by":     delegation.LinkedBy,
	}
}

func taskDelegationMaps(delegations []model.TaskDelegation) []map[string]any {
	out := make([]map[string]any, 0, len(delegations))
	for _, delegation := range delegations {
		out = append(out, taskDelegationToJSONMap(delegation))
	}
	return out
}

func delegationRunEventToJSONMap(event model.DelegationRunEvent) map[string]any {
	return map[string]any{
		"id":      event.ID,
		"ts":      event.TS,
		"kind":    event.Kind,
		"summary": event.Summary,
		"message": event.Message,
		"step":    event.Step,
		"meta":    mapStringStringOrEmpty(event.Meta),
	}
}

func delegationRunEventMaps(events []model.DelegationRunEvent) []map[string]any {
	out := make([]map[string]any, 0, len(events))
	for _, event := range events {
		out = append(out, delegationRunEventToJSONMap(event))
	}
	return out
}

func delegationResultToJSONMap(result *model.DelegationResult) map[string]any {
	if result == nil {
		return map[string]any{}
	}
	return map[string]any{
		"status":              result.Status,
		"summary":             result.Summary,
		"files_changed":       stringSliceOrEmpty(result.FilesChanged),
		"validation_errors":   stringSliceOrEmpty(result.ValidationErrors),
		"validation_warnings": stringSliceOrEmpty(result.ValidationWarnings),
		"blockers":            stringSliceOrEmpty(result.Blockers),
		"metadata":            mapStringStringOrEmpty(result.Metadata),
	}
}

func delegationRunToJSONMap(run *model.DelegationRun) map[string]any {
	if run == nil {
		return map[string]any{}
	}
	return map[string]any{
		"id":          run.ID,
		"status":      run.Status,
		"created_at":  run.CreatedAt,
		"created_by":  run.CreatedBy,
		"updated_at":  run.UpdatedAt,
		"finished_at": run.FinishedAt,
		"request": map[string]any{
			"runtime":               run.Request.Runtime,
			"task_ids":              intSliceOrEmpty(run.Request.TaskIDs),
			"workflow_instructions": run.Request.WorkflowInstructions,
			"skill_refs":            stringSliceOrEmpty(run.Request.SkillRefs),
			"metadata":              mapStringStringOrEmpty(run.Request.Metadata),
		},
		"events": delegationRunEventMaps(run.Events),
		"result": delegationResultToJSONMap(run.Result),
	}
}

func delegationLaunchToJSONMap(launch crShowDelegationLaunchView) map[string]any {
	return map[string]any{
		"available":        launch.Available,
		"reason":           launch.Reason,
		"runtime":          launch.Runtime,
		"default_task_ids": intSliceOrEmpty(launch.DefaultTaskIDs),
		"open_task_ids":    intSliceOrEmpty(launch.OpenTaskIDs),
		"all_task_ids":     intSliceOrEmpty(launch.AllTaskIDs),
		"skill_refs":       stringSliceOrEmpty(launch.SkillRefs),
	}
}

func stackNativityToJSONMap(view service.StackNativityView) map[string]any {
	return map[string]any{
		"role":                  view.Role,
		"role_label":            view.RoleLabel,
		"is_child":              view.IsChild,
		"is_root_parent":        view.IsRootParent,
		"is_aggregate_parent":   view.IsAggregateParent,
		"parent_cr_id":          view.ParentCRID,
		"parent_title":          view.ParentTitle,
		"parent_branch":         view.ParentBranch,
		"parent_status":         view.ParentStatus,
		"child_cr_ids":          intSliceOrEmpty(view.ChildCRIDs),
		"resolved_child_cr_ids": intSliceOrEmpty(view.ResolvedChildCRIDs),
		"pending_child_cr_ids":  intSliceOrEmpty(view.PendingChildCRIDs),
		"child_count":           view.ChildCount,
		"resolved_child_count":  view.ResolvedChildCount,
		"pending_child_count":   view.PendingChildCount,
	}
}

func stackLineageToJSONMaps(lineage []service.StackLineageNodeView) []map[string]any {
	if len(lineage) == 0 {
		return []map[string]any{}
	}
	out := make([]map[string]any, 0, len(lineage))
	for _, node := range lineage {
		out = append(out, map[string]any{
			"id":         node.ID,
			"uid":        node.UID,
			"title":      node.Title,
			"status":     node.Status,
			"branch":     node.Branch,
			"base_ref":   node.BaseRef,
			"depth":      node.Depth,
			"role":       node.Role,
			"role_label": node.RoleLabel,
		})
	}
	return out
}

func stackTreeNodeToJSONMap(node *service.StackTreeNodeView) map[string]any {
	if node == nil {
		return map[string]any{}
	}
	children := make([]map[string]any, 0, len(node.Children))
	for i := range node.Children {
		child := node.Children[i]
		children = append(children, stackTreeNodeToJSONMap(&child))
	}
	return map[string]any{
		"id":                      node.ID,
		"uid":                     node.UID,
		"title":                   node.Title,
		"status":                  node.Status,
		"branch":                  node.Branch,
		"base_ref":                node.BaseRef,
		"parent_cr_id":            node.ParentCRID,
		"depth":                   node.Depth,
		"role":                    node.Role,
		"role_label":              node.RoleLabel,
		"is_child":                node.IsChild,
		"is_root_parent":          node.IsRootParent,
		"is_aggregate_parent":     node.IsAggregateParent,
		"tasks_total":             node.TasksTotal,
		"tasks_open":              node.TasksOpen,
		"tasks_done":              node.TasksDone,
		"tasks_delegated":         node.TasksDelegated,
		"tasks_delegated_pending": node.TasksDelegatedPending,
		"child_count":             node.ChildCount,
		"resolved_child_count":    node.ResolvedChildCount,
		"pending_child_count":     node.PendingChildCount,
		"resolution_state":        node.ResolutionState,
		"children":                children,
	}
}

func delegationSnapshotToJSONMap(runs []model.DelegationRun) map[string]any {
	recentRuns := make([]map[string]any, 0, len(runs))
	currentRun := map[string]any{}
	total := len(runs)
	running := 0
	terminal := 0
	for _, run := range runs {
		if isDelegationRunTerminal(run.Status) {
			terminal++
		} else {
			running++
			if len(currentRun) == 0 {
				currentRun = delegationRunToJSONMap(&run)
			}
		}
		recentRuns = append(recentRuns, delegationRunToJSONMap(&run))
	}
	return map[string]any{
		"current_run": currentRun,
		"recent_runs": recentRuns,
		"counts": map[string]any{
			"total":    total,
			"running":  running,
			"terminal": terminal,
		},
	}
}

func isDelegationRunTerminal(status string) bool {
	switch strings.TrimSpace(status) {
	case model.DelegationRunStatusCompleted,
		model.DelegationRunStatusFailed,
		model.DelegationRunStatusBlocked,
		model.DelegationRunStatusCancelled:
		return true
	default:
		return false
	}
}

func taskContractFieldsToJSONMap(contract model.TaskContract) map[string]any {
	return map[string]any{
		"intent":              contract.Intent,
		"acceptance_criteria": stringSliceOrEmpty(contract.AcceptanceCriteria),
		"scope":               stringSliceOrEmpty(contract.Scope),
		"acceptance_checks":   stringSliceOrEmpty(contract.AcceptanceChecks),
	}
}

func taskContractToJSONMap(contract model.TaskContract) map[string]any {
	out := taskContractFieldsToJSONMap(contract)
	out["updated_at"] = contract.UpdatedAt
	out["updated_by"] = contract.UpdatedBy
	return out
}

func taskContractBaselineToJSONMap(baseline model.TaskContractBaseline) map[string]any {
	return map[string]any{
		"captured_at":         baseline.CapturedAt,
		"captured_by":         baseline.CapturedBy,
		"intent":              baseline.Intent,
		"acceptance_criteria": stringSliceOrEmpty(baseline.AcceptanceCriteria),
		"scope":               stringSliceOrEmpty(baseline.Scope),
		"acceptance_checks":   stringSliceOrEmpty(baseline.AcceptanceChecks),
	}
}

func taskContractDriftModelToJSONMap(drift model.TaskContractDrift) map[string]any {
	return map[string]any{
		"id":                       drift.ID,
		"ts":                       drift.TS,
		"actor":                    drift.Actor,
		"fields":                   stringSliceOrEmpty(drift.Fields),
		"before_scope":             stringSliceOrEmpty(drift.BeforeScope),
		"after_scope":              stringSliceOrEmpty(drift.AfterScope),
		"before_acceptance_checks": stringSliceOrEmpty(drift.BeforeAcceptanceChecks),
		"after_acceptance_checks":  stringSliceOrEmpty(drift.AfterAcceptanceChecks),
		"checkpoint_commit":        drift.CheckpointCommit,
		"reason":                   drift.Reason,
		"acknowledged":             drift.Acknowledged,
		"acknowledged_at":          drift.AcknowledgedAt,
		"acknowledged_by":          drift.AcknowledgedBy,
		"ack_reason":               drift.AckReason,
	}
}

func taskContractDriftMaps(drifts []model.TaskContractDrift) []map[string]any {
	out := make([]map[string]any, 0, len(drifts))
	for _, drift := range drifts {
		out = append(out, taskContractDriftModelToJSONMap(drift))
	}
	return out
}

type taskJSONProjectionOptions struct {
	includeCheckpointMessage bool
	includeCheckpointSyncAt  bool
	includeDelegations       bool
	includeContract          bool
}

func projectTaskJSON(task model.Subtask, opts taskJSONProjectionOptions) map[string]any {
	out := map[string]any{
		"id":                task.ID,
		"title":             task.Title,
		"status":            task.Status,
		"checkpoint_commit": task.CheckpointCommit,
		"checkpoint_at":     task.CheckpointAt,
		"checkpoint_scope":  stringSliceOrEmpty(task.CheckpointScope),
		"checkpoint_source": task.CheckpointSource,
		"checkpoint_orphan": task.CheckpointOrphan,
		"checkpoint_reason": task.CheckpointReason,
		"checkpoint_chunks": checkpointChunkDiffMaps(task.CheckpointChunks),
	}
	if opts.includeCheckpointMessage {
		out["checkpoint_message"] = task.CheckpointMessage
	}
	if opts.includeCheckpointSyncAt {
		out["checkpoint_sync_at"] = task.CheckpointSyncAt
	}
	if opts.includeDelegations {
		out["delegations"] = taskDelegationMaps(task.Delegations)
	}
	if opts.includeContract {
		out["contract"] = taskContractToJSONMap(task.Contract)
		out["contract_baseline"] = taskContractBaselineToJSONMap(task.ContractBaseline)
		out["contract_drifts"] = taskContractDriftMaps(task.ContractDrifts)
	}
	return out
}

func taskToJSONMap(task model.Subtask) map[string]any {
	return map[string]any{
		"id":                 task.ID,
		"title":              task.Title,
		"status":             task.Status,
		"created_at":         task.CreatedAt,
		"updated_at":         task.UpdatedAt,
		"completed_at":       task.CompletedAt,
		"created_by":         task.CreatedBy,
		"completed_by":       task.CompletedBy,
		"checkpoint_commit":  task.CheckpointCommit,
		"checkpoint_at":      task.CheckpointAt,
		"checkpoint_message": task.CheckpointMessage,
		"checkpoint_scope":   stringSliceOrEmpty(task.CheckpointScope),
		"checkpoint_chunks":  checkpointChunkMaps(task.CheckpointChunks),
		"checkpoint_orphan":  task.CheckpointOrphan,
		"checkpoint_reason":  task.CheckpointReason,
		"checkpoint_source":  task.CheckpointSource,
		"checkpoint_sync_at": task.CheckpointSyncAt,
		"delegations":        taskDelegationMaps(task.Delegations),
		"task_contract":      taskContractToJSONMap(task.Contract),
		"contract_baseline":  taskContractBaselineToJSONMap(task.ContractBaseline),
		"contract_drifts":    taskContractDriftMaps(task.ContractDrifts),
	}
}

func contractFieldsToJSONMap(contract model.Contract) map[string]any {
	return map[string]any{
		"why":                  contract.Why,
		"scope":                stringSliceOrEmpty(contract.Scope),
		"non_goals":            stringSliceOrEmpty(contract.NonGoals),
		"invariants":           stringSliceOrEmpty(contract.Invariants),
		"blast_radius":         contract.BlastRadius,
		"risk_critical_scopes": stringSliceOrEmpty(contract.RiskCriticalScopes),
		"risk_tier_hint":       contract.RiskTierHint,
		"risk_rationale":       contract.RiskRationale,
		"test_plan":            contract.TestPlan,
		"rollback_plan":        contract.RollbackPlan,
	}
}

func contractToJSONMap(contract model.Contract) map[string]any {
	out := contractFieldsToJSONMap(contract)
	out["updated_at"] = contract.UpdatedAt
	out["updated_by"] = contract.UpdatedBy
	return out
}

func crContractBaselineToJSONMap(baseline model.CRContractBaseline) map[string]any {
	return map[string]any{
		"captured_at": baseline.CapturedAt,
		"captured_by": baseline.CapturedBy,
		"scope":       stringSliceOrEmpty(baseline.Scope),
	}
}

func crContractDriftToJSONMap(drift model.CRContractDrift) map[string]any {
	return map[string]any{
		"id":              drift.ID,
		"ts":              drift.TS,
		"actor":           drift.Actor,
		"fields":          stringSliceOrEmpty(drift.Fields),
		"before_scope":    stringSliceOrEmpty(drift.BeforeScope),
		"after_scope":     stringSliceOrEmpty(drift.AfterScope),
		"reason":          drift.Reason,
		"acknowledged":    drift.Acknowledged,
		"acknowledged_at": drift.AcknowledgedAt,
		"acknowledged_by": drift.AcknowledgedBy,
		"ack_reason":      drift.AckReason,
	}
}

func crContractDriftMaps(drifts []model.CRContractDrift) []map[string]any {
	out := make([]map[string]any, 0, len(drifts))
	for _, drift := range drifts {
		out = append(out, crContractDriftToJSONMap(drift))
	}
	return out
}

func setCRContractResultToJSONMap(crID int, result *service.SetCRContractResult) map[string]any {
	if result == nil {
		return map[string]any{
			"cr_id":              crID,
			"changed_fields":     []string{},
			"already_applied":    false,
			"dry_run":            false,
			"scope_changed":      false,
			"baseline_frozen":    false,
			"drift_recorded":     false,
			"drift_id":           0,
			"drift_ack_required": false,
		}
	}
	return map[string]any{
		"cr_id":              crID,
		"changed_fields":     stringSliceOrEmpty(result.ChangedFields),
		"already_applied":    result.AlreadyApplied,
		"dry_run":            result.DryRun,
		"scope_changed":      result.ScopeChanged,
		"baseline_frozen":    result.BaselineFrozen,
		"drift_recorded":     result.DriftRecorded,
		"drift_id":           result.DriftID,
		"drift_ack_required": result.DriftAckRequired,
	}
}

func prReadyBlockedToJSONMap(crID int, err *service.PRReadyBlockedError) map[string]any {
	if err == nil {
		return map[string]any{
			"cr_id": crID,
			"action_required": map[string]any{
				"type":               "manual",
				"name":               "ready_pr_blocked",
				"reason_code":        "pre_implementation_no_checkpoints",
				"reason":             "",
				"suggested_commands": []string{},
			},
		}
	}
	reasonCode := err.ReasonCode
	if reasonCode == "" {
		reasonCode = "pre_implementation_no_checkpoints"
	}
	reason := err.Reason
	if strings.TrimSpace(reason) == "" {
		reason = "CR has no task checkpoint commits yet; keep PR draft until implementation checkpoints exist."
	}
	return map[string]any{
		"cr_id": crID,
		"action_required": map[string]any{
			"type":               "manual",
			"name":               "ready_pr_blocked",
			"reason_code":        reasonCode,
			"reason":             reason,
			"suggested_commands": stringSliceOrEmpty(err.SuggestedCommands),
		},
	}
}

func prLinkToJSONMap(pr model.CRPRLink) map[string]any {
	return map[string]any{
		"provider":                    pr.Provider,
		"repo":                        pr.Repo,
		"number":                      pr.Number,
		"url":                         pr.URL,
		"state":                       pr.State,
		"draft":                       pr.Draft,
		"last_head_sha":               pr.LastHeadSHA,
		"last_base_ref":               pr.LastBaseRef,
		"last_body_hash":              pr.LastBodyHash,
		"last_synced_at":              pr.LastSyncedAt,
		"last_status_checked_at":      pr.LastStatusCheckedAt,
		"last_merged_at":              pr.LastMergedAt,
		"last_merged_commit":          pr.LastMergedCommit,
		"checkpoint_comment_keys":     stringSliceOrEmpty(pr.CheckpointCommentKeys),
		"checkpoint_sync_keys":        stringSliceOrEmpty(pr.CheckpointSyncKeys),
		"awaiting_open_approval":      pr.AwaitingOpenApproval,
		"awaiting_open_approval_note": pr.AwaitingOpenApprovalNote,
	}
}

func crSearchResultToJSONMap(result model.CRSearchResult) map[string]any {
	return map[string]any{
		"id":              result.ID,
		"uid":             result.UID,
		"title":           result.Title,
		"status":          result.Status,
		"branch":          result.Branch,
		"branch_identity": branchIdentityToJSONMap(result.Branch, result.UID),
		"base_branch":     result.BaseBranch,
		"parent_cr_id":    result.ParentCRID,
		"risk_tier":       result.RiskTier,
		"tasks": map[string]int{
			"total": result.TasksTotal,
			"open":  result.TasksOpen,
			"done":  result.TasksDone,
		},
		"created_at": result.CreatedAt,
		"updated_at": result.UpdatedAt,
	}
}

func crToJSONMap(cr *model.CR) map[string]any {
	if cr == nil {
		return map[string]any{}
	}
	subtasks := make([]map[string]any, 0, len(cr.Subtasks))
	for _, task := range cr.Subtasks {
		subtasks = append(subtasks, taskToJSONMap(task))
	}
	evidence := make([]map[string]any, 0, len(cr.Evidence))
	for _, entry := range cr.Evidence {
		evidence = append(evidence, evidenceEntryToJSONMap(entry))
	}
	events := make([]map[string]any, 0, len(cr.Events))
	for _, event := range cr.Events {
		events = append(events, map[string]any{
			"ts":               event.TS,
			"actor":            event.Actor,
			"type":             event.Type,
			"summary":          event.Summary,
			"ref":              event.Ref,
			"redacted":         event.Redacted,
			"redaction_reason": event.RedactionReason,
			"meta":             mapStringStringOrEmpty(event.Meta),
		})
	}
	return map[string]any{
		"id":                  cr.ID,
		"uid":                 cr.UID,
		"title":               cr.Title,
		"description":         cr.Description,
		"status":              cr.Status,
		"lifecycle_state":     cr.Status,
		"base_branch":         cr.BaseBranch,
		"base_ref":            cr.BaseRef,
		"base_commit":         cr.BaseCommit,
		"parent_cr_id":        cr.ParentCRID,
		"branch":              cr.Branch,
		"branch_identity":     branchIdentityToJSONMap(cr.Branch, cr.UID),
		"notes":               stringSliceOrEmpty(cr.Notes),
		"evidence":            evidence,
		"contract":            contractToJSONMap(cr.Contract),
		"contract_baseline":   crContractBaselineToJSONMap(cr.ContractBaseline),
		"contract_drifts":     crContractDriftMaps(cr.ContractDrifts),
		"subtasks":            subtasks,
		"events":              events,
		"merged_at":           cr.MergedAt,
		"merged_by":           cr.MergedBy,
		"merged_commit":       cr.MergedCommit,
		"abandoned_at":        cr.AbandonedAt,
		"abandoned_by":        cr.AbandonedBy,
		"abandoned_reason":    cr.AbandonedReason,
		"files_touched_count": cr.FilesTouchedCount,
		"pr":                  prLinkToJSONMap(cr.PR),
		"created_at":          cr.CreatedAt,
		"updated_at":          cr.UpdatedAt,
	}
}

func historyToJSONMap(history *service.CRHistory, showRedacted bool) map[string]any {
	if history == nil {
		return map[string]any{}
	}
	notes := make([]map[string]any, 0, len(history.Notes))
	for _, note := range history.Notes {
		notes = append(notes, map[string]any{
			"index":    note.Index,
			"text":     note.Text,
			"redacted": note.Redacted,
		})
	}
	evidence := make([]map[string]any, 0, len(history.Evidence))
	for _, entry := range history.Evidence {
		evidence = append(evidence, map[string]any{
			"index":       entry.Index,
			"ts":          entry.TS,
			"actor":       entry.Actor,
			"type":        entry.Type,
			"scope":       entry.Scope,
			"command":     entry.Command,
			"output_hash": entry.OutputHash,
			"summary":     entry.Summary,
			"attachments": stringSliceOrEmpty(entry.Attachments),
			"redacted":    false,
			"exit_code": func() any {
				if entry.ExitCode == nil {
					return nil
				}
				return *entry.ExitCode
			}(),
		})
	}
	events := make([]map[string]any, 0, len(history.Events))
	for _, event := range history.Events {
		events = append(events, map[string]any{
			"index":            event.Index,
			"ts":               event.TS,
			"actor":            event.Actor,
			"type":             event.Type,
			"summary":          event.Summary,
			"ref":              event.Ref,
			"redacted":         event.Redacted,
			"redaction_reason": event.RedactionReason,
			"meta":             mapStringStringOrEmpty(event.Meta),
		})
	}
	return map[string]any{
		"cr_id":         history.CRID,
		"title":         history.Title,
		"status":        history.Status,
		"description":   history.Description,
		"show_redacted": showRedacted,
		"notes":         notes,
		"evidence":      evidence,
		"events":        events,
	}
}

func impactToJSONMap(impact *service.ImpactReport) map[string]any {
	if impact == nil {
		return map[string]any{}
	}
	signals := make([]map[string]any, 0, len(impact.Signals))
	for _, signal := range impact.Signals {
		signals = append(signals, map[string]any{
			"code":    signal.Code,
			"summary": signal.Summary,
			"points":  signal.Points,
		})
	}
	return map[string]any{
		"cr_id":                        impact.CRID,
		"cr_uid":                       impact.CRUID,
		"base_ref":                     impact.BaseRef,
		"base_commit":                  impact.BaseCommit,
		"parent_cr_id":                 impact.ParentCRID,
		"scope_source":                 impact.ScopeSource,
		"risk_tier_hint":               impact.RiskTierHint,
		"risk_tier_floor_applied":      impact.RiskTierFloorApplied,
		"matched_risk_critical_scopes": stringSliceOrEmpty(impact.MatchedRiskCriticalScopes),
		"files_changed":                impact.FilesChanged,
		"new_files":                    stringSliceOrEmpty(impact.NewFiles),
		"modified_files":               stringSliceOrEmpty(impact.ModifiedFiles),
		"deleted_files":                stringSliceOrEmpty(impact.DeletedFiles),
		"test_files":                   stringSliceOrEmpty(impact.TestFiles),
		"dependency_files":             stringSliceOrEmpty(impact.DependencyFiles),
		"warnings":                     stringSliceOrEmpty(impact.Warnings),
		"scope_drift":                  stringSliceOrEmpty(impact.ScopeDrift),
		"task_scope_warnings":          stringSliceOrEmpty(impact.TaskScopeWarnings),
		"task_contract_warnings":       stringSliceOrEmpty(impact.TaskContractWarnings),
		"task_chunk_warnings":          stringSliceOrEmpty(impact.TaskChunkWarnings),
		"risk_signals":                 signals,
		"risk_score":                   impact.RiskScore,
		"risk_tier":                    impact.RiskTier,
	}
}

func chunkToJSONMap(chunk service.TaskChunk) map[string]any {
	return map[string]any{
		"chunk_id":  chunk.ID,
		"path":      chunk.Path,
		"old_start": chunk.OldStart,
		"old_lines": chunk.OldLines,
		"new_start": chunk.NewStart,
		"new_lines": chunk.NewLines,
		"preview":   chunk.Preview,
	}
}

func reviewToJSONMap(review *service.Review) map[string]any {
	if review == nil || review.CR == nil {
		return map[string]any{}
	}
	subtasks := make([]map[string]any, 0, len(review.CR.Subtasks))
	for _, task := range review.CR.Subtasks {
		subtasks = append(subtasks, projectTaskJSON(task, taskJSONProjectionOptions{
			includeCheckpointSyncAt: true,
			includeDelegations:      true,
		}))
	}
	evidence := make([]map[string]any, 0, len(review.CR.Evidence))
	for _, entry := range review.CR.Evidence {
		evidence = append(evidence, evidenceEntryToJSONMap(entry))
	}
	return map[string]any{
		"cr": map[string]any{
			"id":                 review.CR.ID,
			"uid":                review.CR.UID,
			"title":              review.CR.Title,
			"status":             review.CR.Status,
			"lifecycle_state":    review.LifecycleState,
			"abandoned_at":       review.AbandonedAt,
			"abandoned_by":       review.AbandonedBy,
			"abandoned_reason":   review.AbandonedReason,
			"base_branch":        review.CR.BaseBranch,
			"base_ref":           review.CR.BaseRef,
			"base_commit":        review.CR.BaseCommit,
			"parent_cr_id":       review.CR.ParentCRID,
			"branch":             review.CR.Branch,
			"branch_identity":    branchIdentityToJSONMap(review.CR.Branch, review.CR.UID),
			"intent":             review.CR.Description,
			"pr":                 prLinkToJSONMap(review.CR.PR),
			"pr_linkage_state":   review.PRLinkageState,
			"action_required":    review.ActionRequired,
			"action_reason":      review.ActionReason,
			"suggested_commands": stringSliceOrEmpty(review.SuggestedCommands),
		},
		"contract":            contractFieldsToJSONMap(review.Contract),
		"subtasks":            subtasks,
		"notes":               stringSliceOrEmpty(review.CR.Notes),
		"evidence":            evidence,
		"new_files":           stringSliceOrEmpty(review.NewFiles),
		"modified_files":      stringSliceOrEmpty(review.ModifiedFiles),
		"deleted_files":       stringSliceOrEmpty(review.DeletedFiles),
		"test_files_touched":  stringSliceOrEmpty(review.TestFiles),
		"dependency_files":    stringSliceOrEmpty(review.DependencyFiles),
		"files_changed":       stringSliceOrEmpty(review.Files),
		"diff_stat":           review.ShortStat,
		"impact":              impactToJSONMap(review.Impact),
		"trust":               trustToJSONMap(review.Trust),
		"validation_errors":   stringSliceOrEmpty(review.ValidationErrors),
		"validation_warnings": stringSliceOrEmpty(review.ValidationWarnings),
	}
}

func trustToJSONMap(trust *service.TrustReport) map[string]any {
	if trust == nil {
		return map[string]any{}
	}
	dimensions := make([]map[string]any, 0, len(trust.Dimensions))
	for _, dimension := range trust.Dimensions {
		dimensions = append(dimensions, map[string]any{
			"code":             dimension.Code,
			"label":            dimension.Label,
			"score":            dimension.Score,
			"max":              dimension.Max,
			"reasons":          dimension.Reasons,
			"required_actions": dimension.RequiredActions,
		})
	}
	requirements := make([]map[string]any, 0, len(trust.Requirements))
	for _, requirement := range trust.Requirements {
		requirements = append(requirements, trustRequirementToJSONMap(requirement))
	}
	checkResults := make([]map[string]any, 0, len(trust.CheckResults))
	for _, check := range trust.CheckResults {
		checkResults = append(checkResults, trustCheckResultToJSONMap(check))
	}
	return map[string]any{
		"verdict":           trust.Verdict,
		"score":             trust.Score,
		"max":               trust.Max,
		"advisory_only":     trust.AdvisoryOnly,
		"risk_tier":         trust.RiskTier,
		"hard_failures":     stringSliceOrEmpty(trust.HardFailures),
		"dimensions":        dimensions,
		"requirements":      requirements,
		"check_results":     checkResults,
		"review_depth":      trustReviewDepthToJSONMap(trust.ReviewDepth),
		"contract_drift":    trustContractDriftSummaryToJSONMap(trust.ContractDrift),
		"cr_contract_drift": trustCRContractDriftSummaryToJSONMap(trust.CRContractDrift),
		"gate":              trustGateToJSONMap(trust.Gate),
		"required_actions":  stringSliceOrEmpty(trust.RequiredActions),
		"attention_actions": stringSliceOrEmpty(trust.AttentionActions),
		"advisories":        stringSliceOrEmpty(trust.Advisories),
		"summary":           trust.Summary,
	}
}

func trustRequirementToJSONMap(requirement service.TrustRequirement) map[string]any {
	return map[string]any{
		"key":       requirement.Key,
		"title":     requirement.Title,
		"satisfied": requirement.Satisfied,
		"reason":    requirement.Reason,
		"action":    requirement.Action,
		"task_id":   requirement.TaskID,
		"source":    requirement.Source,
	}
}

func trustCheckResultToJSONMap(check service.TrustCheckResult) map[string]any {
	var exitCode any
	if check.ExitCode != nil {
		exitCode = *check.ExitCode
	}
	return map[string]any{
		"key":                  check.Key,
		"command":              check.Command,
		"required":             check.Required,
		"status":               check.Status,
		"reason":               check.Reason,
		"allow_exit_codes":     append([]int(nil), check.AllowExitCodes...),
		"exit_code":            exitCode,
		"last_run_at":          check.LastRunAt,
		"freshness_hours":      check.FreshnessHours,
		"required_by_task_ids": intSliceOrEmpty(check.RequiredByTaskIDs),
		"sources":              stringSliceOrEmpty(check.Sources),
	}
}

func trustReviewDepthToJSONMap(depth service.TrustReviewDepthResult) map[string]any {
	return map[string]any{
		"risk_tier":                       depth.RiskTier,
		"required_samples":                depth.RequiredSamples,
		"sample_count":                    depth.SampleCount,
		"require_critical_scope_coverage": depth.RequireCriticalScopeCoverage,
		"covered_critical_scopes":         stringSliceOrEmpty(depth.CoveredCriticalScopes),
		"missing_critical_scopes":         stringSliceOrEmpty(depth.MissingCriticalScopes),
		"satisfied":                       depth.Satisfied,
	}
}

func trustGateToJSONMap(gate service.TrustGateSummary) map[string]any {
	return map[string]any{
		"enabled": gate.Enabled,
		"applies": gate.Applies,
		"blocked": gate.Blocked,
		"reason":  gate.Reason,
	}
}

func trustContractDriftSummaryToJSONMap(summary service.TaskContractDriftSummary) map[string]any {
	return map[string]any{
		"total":                summary.Total,
		"unacknowledged":       summary.Unacknowledged,
		"tasks_with_drift":     intSliceOrEmpty(summary.TasksWithDrift),
		"unacknowledged_tasks": intSliceOrEmpty(summary.UnacknowledgedTasks),
	}
}

func trustCRContractDriftSummaryToJSONMap(summary service.CRContractDriftSummary) map[string]any {
	return map[string]any{
		"total":                    summary.Total,
		"unacknowledged":           summary.Unacknowledged,
		"drift_ids":                intSliceOrEmpty(summary.DriftIDs),
		"unacknowledged_drift_ids": intSliceOrEmpty(summary.UnacknowledgedDriftIDs),
	}
}

func trustCheckStatusToJSONMap(report *service.TrustCheckStatusReport) map[string]any {
	if report == nil {
		return map[string]any{}
	}
	requirements := make([]map[string]any, 0, len(report.Requirements))
	for _, requirement := range report.Requirements {
		requirements = append(requirements, trustRequirementToJSONMap(requirement))
	}
	checkResults := make([]map[string]any, 0, len(report.CheckResults))
	for _, check := range report.CheckResults {
		checkResults = append(checkResults, trustCheckResultToJSONMap(check))
	}
	return map[string]any{
		"cr_id":                report.CRID,
		"cr_uid":               report.CRUID,
		"risk_tier":            report.RiskTier,
		"freshness_hours":      report.FreshnessHours,
		"check_mode":           report.CheckMode,
		"required_check_count": report.RequiredCheckCount,
		"requirements":         requirements,
		"check_results":        checkResults,
		"guidance":             stringSliceOrEmpty(report.Guidance),
	}
}

func trustCheckRunToJSONMap(report *service.TrustCheckRunReport) map[string]any {
	if report == nil {
		return map[string]any{}
	}
	requirements := make([]map[string]any, 0, len(report.Requirements))
	for _, requirement := range report.Requirements {
		requirements = append(requirements, trustRequirementToJSONMap(requirement))
	}
	checkResults := make([]map[string]any, 0, len(report.CheckResults))
	for _, check := range report.CheckResults {
		checkResults = append(checkResults, trustCheckResultToJSONMap(check))
	}
	return map[string]any{
		"cr_id":                report.CRID,
		"cr_uid":               report.CRUID,
		"risk_tier":            report.RiskTier,
		"executed":             report.Executed,
		"check_mode":           report.CheckMode,
		"required_check_count": report.RequiredCheckCount,
		"requirements":         requirements,
		"check_results":        checkResults,
		"guidance":             stringSliceOrEmpty(report.Guidance),
	}
}

func validationToJSONMap(report *service.ValidationReport) map[string]any {
	if report == nil {
		return map[string]any{
			"valid":    false,
			"errors":   []string{},
			"warnings": []string{},
			"impact":   map[string]any{},
		}
	}
	return map[string]any{
		"valid":    report.Valid,
		"errors":   stringSliceOrEmpty(report.Errors),
		"warnings": stringSliceOrEmpty(report.Warnings),
		"impact":   impactToJSONMap(report.Impact),
	}
}

func mergeStatusToJSONMap(status *service.MergeStatusView) map[string]any {
	if status == nil {
		return map[string]any{}
	}
	return map[string]any{
		"cr_id":          status.CRID,
		"cr_uid":         status.CRUID,
		"base_branch":    status.BaseBranch,
		"cr_branch":      status.CRBranch,
		"worktree_path":  status.WorktreePath,
		"in_progress":    status.InProgress,
		"conflict_files": stringSliceOrEmpty(status.ConflictFiles),
		"target_matches": status.TargetMatches,
		"merge_head":     status.MergeHead,
		"advice":         stringSliceOrEmpty(status.Advice),
	}
}

func crStatusToJSONMap(status *service.CRStatusView) map[string]any {
	if status == nil {
		return map[string]any{}
	}
	return map[string]any{
		"id":              status.ID,
		"uid":             status.UID,
		"title":           status.Title,
		"status":          status.Status,
		"base":            status.BaseBranch,
		"base_ref":        status.BaseRef,
		"base_commit":     status.BaseCommit,
		"parent_cr_id":    status.ParentCRID,
		"parent_status":   status.ParentStatus,
		"branch":          status.Branch,
		"branch_identity": branchIdentityToJSONMap(status.Branch, status.UID),
		"branch_context": map[string]any{
			"current_branch":                status.CurrentBranch,
			"current_worktree_path":         status.CurrentWorktreePath,
			"branch_match":                  status.BranchMatch,
			"owner_worktree_path":           status.OwnerWorktreePath,
			"owner_is_current_worktree":     status.OwnerIsCurrentWorktree,
			"checked_out_in_other_worktree": status.CheckedOutInOtherWorktree,
		},
		"working_tree": map[string]any{
			"modified_staged_count": status.ModifiedStagedCount,
			"untracked_count":       status.UntrackedCount,
			"dirty":                 status.Dirty,
		},
		"tasks": map[string]any{
			"total":             status.TasksTotal,
			"open":              status.TasksOpen,
			"done":              status.TasksDone,
			"delegated":         status.TasksDelegated,
			"delegated_pending": status.TasksDelegatedPending,
		},
		"aggregate_parent": map[string]any{
			"enabled":           status.IsAggregateParent,
			"resolved_children": intSliceOrEmpty(status.AggregateResolvedChildren),
			"pending_children":  intSliceOrEmpty(status.AggregatePendingChildren),
			"resolved_count":    len(status.AggregateResolvedChildren),
			"pending_count":     len(status.AggregatePendingChildren),
		},
		"contract": map[string]any{
			"complete":       status.ContractComplete,
			"missing_fields": stringSliceOrEmpty(status.ContractMissingFields),
		},
		"validation": map[string]any{
			"valid":    status.ValidationValid,
			"errors":   status.ValidationErrors,
			"warnings": status.ValidationWarnings,
			"risk": map[string]any{
				"tier":  status.RiskTier,
				"score": status.RiskScore,
			},
		},
		"merge_blocked":      status.MergeBlocked,
		"merge_blockers":     stringSliceOrEmpty(status.MergeBlockers),
		"pr_linkage_state":   status.PRLinkageState,
		"action_required":    status.ActionRequired,
		"action_reason":      status.ActionReason,
		"suggested_commands": stringSliceOrEmpty(status.SuggestedCommands),
	}
}

func hqSyncStatusToJSONMap(view *service.HQSyncStatusView) map[string]any {
	if view == nil {
		return map[string]any{}
	}
	return map[string]any{
		"configured":           view.Configured,
		"base_url":             view.BaseURL,
		"repo_id":              view.RepoID,
		"remote_alias":         view.RemoteAlias,
		"has_token":            view.HasToken,
		"linked":               view.Linked,
		"local_fingerprint":    view.LocalFingerprint,
		"upstream_fingerprint": view.UpstreamFingerprint,
		"remote_exists":        view.RemoteExists,
		"remote_checked":       view.RemoteChecked,
		"remote_fingerprint":   view.RemoteFingerprint,
		"state":                view.State,
		"suggested_actions":    stringSliceOrEmpty(view.SuggestedActions),
	}
}

func crRefreshToJSONMap(view *service.CRRefreshView) map[string]any {
	if view == nil {
		return map[string]any{}
	}
	return map[string]any{
		"cr_id":       view.CRID,
		"strategy":    view.Strategy,
		"dry_run":     view.DryRun,
		"applied":     view.Applied,
		"base_ref":    view.BaseRef,
		"target_ref":  view.TargetRef,
		"before_head": view.BeforeHead,
		"after_head":  view.AfterHead,
		"warnings":    stringSliceOrEmpty(view.Warnings),
	}
}

func applyPlanToJSONMap(result *service.ApplyCRPlanResult) map[string]any {
	if result == nil {
		return map[string]any{}
	}
	createdCRs := make([]map[string]any, 0, len(result.CreatedCRs))
	for _, created := range result.CreatedCRs {
		createdCRs = append(createdCRs, map[string]any{
			"key":             created.Key,
			"id":              created.ID,
			"uid":             created.UID,
			"branch":          created.Branch,
			"branch_identity": branchIdentityToJSONMap(created.Branch, created.UID),
			"parent_cr_id":    created.ParentCRID,
		})
	}
	createdTasks := make([]map[string]any, 0, len(result.CreatedTasks))
	for _, created := range result.CreatedTasks {
		createdTasks = append(createdTasks, map[string]any{
			"cr_key":   created.CRKey,
			"task_key": created.TaskKey,
			"task_id":  created.TaskID,
		})
	}
	delegations := make([]map[string]any, 0, len(result.Delegations))
	for _, delegation := range result.Delegations {
		delegations = append(delegations, map[string]any{
			"parent_cr_key":   delegation.ParentCRKey,
			"parent_task_key": delegation.ParentTaskKey,
			"child_cr_key":    delegation.ChildCRKey,
			"child_task_id":   delegation.ChildTaskID,
		})
	}
	return map[string]any{
		"file":               result.FilePath,
		"dry_run":            result.DryRun,
		"consumed":           result.Consumed,
		"planned_operations": stringSliceOrEmpty(result.PlannedOperations),
		"created_crs":        createdCRs,
		"created_tasks":      createdTasks,
		"delegations":        delegations,
		"warnings":           stringSliceOrEmpty(result.Warnings),
	}
}

func crDoctorToJSONMap(report *service.CRDoctorReport) map[string]any {
	if report == nil {
		return map[string]any{}
	}
	findings := make([]map[string]any, 0, len(report.Findings))
	for _, finding := range report.Findings {
		findings = append(findings, map[string]any{
			"code":    finding.Code,
			"message": finding.Message,
			"task_id": finding.TaskID,
			"commit":  finding.Commit,
		})
	}
	return map[string]any{
		"cr_id":                 report.CRID,
		"cr_uid":                report.CRUID,
		"branch":                report.Branch,
		"branch_identity":       branchIdentityToJSONMap(report.Branch, report.CRUID),
		"branch_exists":         report.BranchExists,
		"branch_head":           report.BranchHead,
		"base_ref":              report.BaseRef,
		"base_commit":           report.BaseCommit,
		"resolved_base_ref":     report.ResolvedBaseRef,
		"parent_cr_id":          report.ParentCRID,
		"expected_parent_cr_id": report.ExpectedParentID,
		"findings":              findings,
	}
}

func reconcileCRToJSONMap(report *service.ReconcileCRReport) map[string]any {
	if report == nil {
		return map[string]any{}
	}
	findings := make([]map[string]any, 0, len(report.Findings))
	for _, finding := range report.Findings {
		findings = append(findings, map[string]any{
			"code":    finding.Code,
			"message": finding.Message,
			"task_id": finding.TaskID,
			"commit":  finding.Commit,
		})
	}
	taskResults := make([]map[string]any, 0, len(report.TaskResults))
	for _, result := range report.TaskResults {
		taskResults = append(taskResults, map[string]any{
			"task_id":           result.TaskID,
			"title":             result.Title,
			"status":            result.Status,
			"previous_commit":   result.PreviousCommit,
			"current_commit":    result.CurrentCommit,
			"action":            result.Action,
			"reason":            result.Reason,
			"source":            result.Source,
			"checkpoint_at":     result.CheckpointAt,
			"checkpoint_orphan": result.CheckpointOrphan,
		})
	}
	return map[string]any{
		"cr_id":              report.CRID,
		"cr_uid":             report.CRUID,
		"branch":             report.Branch,
		"branch_identity":    branchIdentityToJSONMap(report.Branch, report.CRUID),
		"branch_exists":      report.BranchExists,
		"previous_parent_id": report.PreviousParentID,
		"current_parent_id":  report.CurrentParentID,
		"parent_relinked":    report.ParentRelinked,
		"scan_ref":           report.ScanRef,
		"scanned_commits":    report.ScannedCommits,
		"relinked":           report.Relinked,
		"orphaned":           report.Orphaned,
		"cleared_orphans":    report.ClearedOrphans,
		"regenerated":        report.Regenerated,
		"files_changed":      report.FilesChanged,
		"diff_stat":          report.DiffStat,
		"warnings":           stringSliceOrEmpty(report.Warnings),
		"findings":           findings,
		"task_results":       taskResults,
	}
}

func crDiffToJSONMap(view *service.CRDiffView) map[string]any {
	if view == nil {
		return map[string]any{}
	}
	files := make([]map[string]any, 0, len(view.Files))
	for _, file := range view.Files {
		hunks := make([]map[string]any, 0, len(file.Hunks))
		for _, hunk := range file.Hunks {
			hunks = append(hunks, map[string]any{
				"chunk_id":  hunk.ChunkID,
				"path":      hunk.Path,
				"old_start": hunk.OldStart,
				"old_lines": hunk.OldLines,
				"new_start": hunk.NewStart,
				"new_lines": hunk.NewLines,
				"header":    hunk.Header,
				"preview":   hunk.Preview,
				"source":    hunk.Source,
			})
		}
		files = append(files, map[string]any{
			"path":  file.Path,
			"hunks": hunks,
		})
	}
	return map[string]any{
		"cr_id":           view.CRID,
		"task_id":         view.TaskID,
		"mode":            view.Mode,
		"critical_only":   view.CriticalOnly,
		"chunks_only":     view.ChunksOnly,
		"base_ref":        view.BaseRef,
		"base_commit":     view.BaseCommit,
		"target_ref":      view.TargetRef,
		"files":           files,
		"files_changed":   view.FilesChanged,
		"short_stat":      view.ShortStat,
		"fallback_used":   view.FallbackUsed,
		"fallback_reason": view.FallbackReason,
		"warnings":        stringSliceOrEmpty(view.Warnings),
	}
}

func rangeDiffToJSONMap(view *service.RangeDiffView) map[string]any {
	if view == nil {
		return map[string]any{}
	}
	mapping := make([]map[string]any, 0, len(view.Mapping))
	for _, row := range view.Mapping {
		mapping = append(mapping, map[string]any{
			"old_index":  row.OldIndex,
			"old_commit": row.OldCommit,
			"relation":   row.Relation,
			"new_index":  row.NewIndex,
			"new_commit": row.NewCommit,
			"subject":    row.Subject,
		})
	}
	return map[string]any{
		"cr_id":         view.CRID,
		"task_id":       view.TaskID,
		"from_ref":      view.FromRef,
		"to_ref":        view.ToRef,
		"base_ref":      view.BaseRef,
		"old_range":     view.OldRange,
		"new_range":     view.NewRange,
		"mapping":       mapping,
		"files_changed": view.FilesChanged,
		"short_stat":    view.ShortStat,
		"warnings":      stringSliceOrEmpty(view.Warnings),
	}
}

func crRangeAnchorsToJSONMap(view *service.CRRangeAnchorsView) map[string]any {
	if view == nil {
		return map[string]any{}
	}
	return map[string]any{
		"cr_id":      view.CRID,
		"base":       view.Base,
		"head":       view.Head,
		"merge_base": view.MergeBase,
		"warnings":   stringSliceOrEmpty(view.Warnings),
	}
}

func crRevParseToJSONMap(view *service.CRRevParseView) map[string]any {
	if view == nil {
		return map[string]any{}
	}
	return map[string]any{
		"cr_id":    view.CRID,
		"kind":     view.Kind,
		"commit":   view.Commit,
		"warnings": stringSliceOrEmpty(view.Warnings),
	}
}

func crPackToJSONMap(view *service.CRPackView) map[string]any {
	if view == nil || view.CR == nil {
		return map[string]any{}
	}
	tasks := make([]map[string]any, 0, len(view.Tasks))
	for _, task := range view.Tasks {
		tasks = append(tasks, projectTaskJSON(task, taskJSONProjectionOptions{
			includeCheckpointMessage: true,
			includeContract:          true,
		}))
	}

	events := make([]map[string]any, 0, len(view.RecentEvents))
	for _, event := range view.RecentEvents {
		events = append(events, map[string]any{
			"ts":      event.TS,
			"actor":   event.Actor,
			"type":    event.Type,
			"summary": event.Summary,
			"ref":     event.Ref,
			"meta":    mapStringStringOrEmpty(event.Meta),
		})
	}

	checkpoints := make([]map[string]any, 0, len(view.RecentCheckpoints))
	for _, checkpoint := range view.RecentCheckpoints {
		checkpoints = append(checkpoints, map[string]any{
			"task_id": checkpoint.TaskID,
			"title":   checkpoint.Title,
			"status":  checkpoint.Status,
			"commit":  checkpoint.Commit,
			"at":      checkpoint.At,
			"message": checkpoint.Message,
			"scope":   checkpoint.Scope,
			"source":  checkpoint.Source,
			"orphan":  checkpoint.Orphan,
			"reason":  checkpoint.Reason,
		})
	}

	anchors := map[string]any{}
	if view.Anchors != nil {
		anchors = map[string]any{
			"base":       view.Anchors.Base,
			"head":       view.Anchors.Head,
			"merge_base": view.Anchors.MergeBase,
			"warnings":   stringSliceOrEmpty(view.Anchors.Warnings),
		}
	}

	return map[string]any{
		"cr": map[string]any{
			"id":               view.CR.ID,
			"uid":              view.CR.UID,
			"title":            view.CR.Title,
			"description":      view.CR.Description,
			"status":           view.CR.Status,
			"lifecycle_state":  view.CR.Status,
			"base_branch":      view.CR.BaseBranch,
			"base_ref":         view.CR.BaseRef,
			"base_commit":      view.CR.BaseCommit,
			"parent_cr_id":     view.CR.ParentCRID,
			"branch":           view.CR.Branch,
			"branch_identity":  branchIdentityToJSONMap(view.CR.Branch, view.CR.UID),
			"merged_at":        view.CR.MergedAt,
			"merged_by":        view.CR.MergedBy,
			"merged_commit":    view.CR.MergedCommit,
			"abandoned_at":     view.CR.AbandonedAt,
			"abandoned_by":     view.CR.AbandonedBy,
			"abandoned_reason": view.CR.AbandonedReason,
			"created_at":       view.CR.CreatedAt,
			"updated_at":       view.CR.UpdatedAt,
		},
		"contract":           contractToJSONMap(view.Contract),
		"tasks":              tasks,
		"delegation":         delegationSnapshotToJSONMap(view.DelegationRuns),
		"stack_nativity":     stackNativityToJSONMap(view.StackNativity),
		"stack_lineage":      stackLineageToJSONMaps(view.StackLineage),
		"stack_tree":         stackTreeNodeToJSONMap(view.StackTree),
		"anchors":            anchors,
		"status":             crStatusToJSONMap(view.Status),
		"recent_events":      events,
		"events_meta":        packSliceMetaToJSONMap(view.EventsMeta),
		"recent_checkpoints": checkpoints,
		"checkpoints_meta":   packSliceMetaToJSONMap(view.CheckpointsMeta),
		"diff_stat":          view.DiffStat,
		"files_changed":      stringSliceOrEmpty(view.FilesChanged),
		"impact":             impactToJSONMap(view.Impact),
		"validation":         validationToJSONMap(view.Validation),
		"trust":              trustToJSONMap(view.Trust),
		"warnings":           stringSliceOrEmpty(view.Warnings),
	}
}

func packSliceMetaToJSONMap(meta service.PackSliceMeta) map[string]any {
	return map[string]any{
		"total":     meta.Total,
		"returned":  meta.Returned,
		"truncated": meta.Truncated,
	}
}

func crPatchApplyResultToJSON(result *service.CRPatchApplyResult) map[string]any {
	if result == nil {
		return map[string]any{}
	}
	conflicts := make([]map[string]any, 0, len(result.Conflicts))
	for _, conflict := range result.Conflicts {
		conflicts = append(conflicts, map[string]any{
			"op_index": conflict.OpIndex,
			"op":       conflict.Op,
			"field":    conflict.Field,
			"message":  conflict.Message,
			"expected": conflict.Expected,
			"current":  conflict.Current,
		})
	}
	return map[string]any{
		"cr_id":            result.CRID,
		"cr_uid":           result.CRUID,
		"base_fingerprint": result.BaseFingerprint,
		"new_fingerprint":  result.NewFingerprint,
		"applied_ops":      append([]int(nil), result.AppliedOps...),
		"skipped_ops":      append([]int(nil), result.SkippedOps...),
		"warnings":         stringSliceOrEmpty(result.Warnings),
		"conflicts":        conflicts,
		"preview":          result.Preview,
	}
}
