package cli

import "sophia/internal/service"

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
		"risk_tier_hint":               impact.RiskTierHint,
		"risk_tier_floor_applied":      impact.RiskTierFloorApplied,
		"matched_risk_critical_scopes": impact.MatchedRiskCriticalScopes,
		"files_changed":                impact.FilesChanged,
		"new_files":                    impact.NewFiles,
		"modified_files":               impact.ModifiedFiles,
		"deleted_files":                impact.DeletedFiles,
		"test_files":                   impact.TestFiles,
		"dependency_files":             impact.DependencyFiles,
		"scope_drift":                  impact.ScopeDrift,
		"task_scope_warnings":          impact.TaskScopeWarnings,
		"task_contract_warnings":       impact.TaskContractWarnings,
		"task_chunk_warnings":          impact.TaskChunkWarnings,
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
		chunkMaps := make([]map[string]any, 0, len(task.CheckpointChunks))
		for _, chunk := range task.CheckpointChunks {
			chunkMaps = append(chunkMaps, map[string]any{
				"chunk_id":  chunk.ID,
				"path":      chunk.Path,
				"old_start": chunk.OldStart,
				"old_lines": chunk.OldLines,
				"new_start": chunk.NewStart,
				"new_lines": chunk.NewLines,
			})
		}
		delegationMaps := make([]map[string]any, 0, len(task.Delegations))
		for _, delegation := range task.Delegations {
			delegationMaps = append(delegationMaps, map[string]any{
				"child_cr_id":   delegation.ChildCRID,
				"child_cr_uid":  delegation.ChildCRUID,
				"child_task_id": delegation.ChildTaskID,
				"linked_at":     delegation.LinkedAt,
				"linked_by":     delegation.LinkedBy,
			})
		}
		subtasks = append(subtasks, map[string]any{
			"id":                 task.ID,
			"title":              task.Title,
			"status":             task.Status,
			"checkpoint_commit":  task.CheckpointCommit,
			"checkpoint_at":      task.CheckpointAt,
			"checkpoint_orphan":  task.CheckpointOrphan,
			"checkpoint_reason":  task.CheckpointReason,
			"checkpoint_source":  task.CheckpointSource,
			"checkpoint_sync_at": task.CheckpointSyncAt,
			"checkpoint_scope":   task.CheckpointScope,
			"checkpoint_chunks":  chunkMaps,
			"delegations":        delegationMaps,
		})
	}
	evidence := make([]map[string]any, 0, len(review.CR.Evidence))
	for _, entry := range review.CR.Evidence {
		evidence = append(evidence, evidenceEntryToJSONMap(entry))
	}
	return map[string]any{
		"cr": map[string]any{
			"id":           review.CR.ID,
			"uid":          review.CR.UID,
			"title":        review.CR.Title,
			"status":       review.CR.Status,
			"base_branch":  review.CR.BaseBranch,
			"base_ref":     review.CR.BaseRef,
			"base_commit":  review.CR.BaseCommit,
			"parent_cr_id": review.CR.ParentCRID,
			"branch":       review.CR.Branch,
			"intent":       review.CR.Description,
		},
		"contract": map[string]any{
			"why":                  review.Contract.Why,
			"scope":                review.Contract.Scope,
			"non_goals":            review.Contract.NonGoals,
			"invariants":           review.Contract.Invariants,
			"blast_radius":         review.Contract.BlastRadius,
			"risk_critical_scopes": review.Contract.RiskCriticalScopes,
			"risk_tier_hint":       review.Contract.RiskTierHint,
			"risk_rationale":       review.Contract.RiskRationale,
			"test_plan":            review.Contract.TestPlan,
			"rollback_plan":        review.Contract.RollbackPlan,
		},
		"subtasks":            subtasks,
		"notes":               review.CR.Notes,
		"evidence":            evidence,
		"new_files":           review.NewFiles,
		"modified_files":      review.ModifiedFiles,
		"deleted_files":       review.DeletedFiles,
		"test_files_touched":  review.TestFiles,
		"dependency_files":    review.DependencyFiles,
		"files_changed":       review.Files,
		"diff_stat":           review.ShortStat,
		"impact":              impactToJSONMap(review.Impact),
		"trust":               trustToJSONMap(review.Trust),
		"validation_errors":   review.ValidationErrors,
		"validation_warnings": review.ValidationWarnings,
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
	return map[string]any{
		"verdict":          trust.Verdict,
		"score":            trust.Score,
		"max":              trust.Max,
		"advisory_only":    trust.AdvisoryOnly,
		"hard_failures":    trust.HardFailures,
		"dimensions":       dimensions,
		"required_actions": trust.RequiredActions,
		"advisories":       trust.Advisories,
		"summary":          trust.Summary,
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
		"errors":   report.Errors,
		"warnings": report.Warnings,
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
		"conflict_files": status.ConflictFiles,
		"target_matches": status.TargetMatches,
		"merge_head":     status.MergeHead,
		"advice":         status.Advice,
	}
}

func crStatusToJSONMap(status *service.CRStatusView) map[string]any {
	if status == nil {
		return map[string]any{}
	}
	return map[string]any{
		"id":            status.ID,
		"uid":           status.UID,
		"title":         status.Title,
		"status":        status.Status,
		"base":          status.BaseBranch,
		"base_ref":      status.BaseRef,
		"base_commit":   status.BaseCommit,
		"parent_cr_id":  status.ParentCRID,
		"parent_status": status.ParentStatus,
		"branch":        status.Branch,
		"branch_context": map[string]any{
			"current_branch": status.CurrentBranch,
			"branch_match":   status.BranchMatch,
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
		"contract": map[string]any{
			"complete":       status.ContractComplete,
			"missing_fields": status.ContractMissingFields,
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
		"merge_blocked":  status.MergeBlocked,
		"merge_blockers": status.MergeBlockers,
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
		"warnings":    view.Warnings,
	}
}

func applyPlanToJSONMap(result *service.ApplyCRPlanResult) map[string]any {
	if result == nil {
		return map[string]any{}
	}
	createdCRs := make([]map[string]any, 0, len(result.CreatedCRs))
	for _, created := range result.CreatedCRs {
		createdCRs = append(createdCRs, map[string]any{
			"key":          created.Key,
			"id":           created.ID,
			"uid":          created.UID,
			"branch":       created.Branch,
			"parent_cr_id": created.ParentCRID,
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
		"planned_operations": result.PlannedOperations,
		"created_crs":        createdCRs,
		"created_tasks":      createdTasks,
		"delegations":        delegations,
		"warnings":           result.Warnings,
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
		"warnings":           report.Warnings,
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
		"warnings":        view.Warnings,
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
		"warnings":      view.Warnings,
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
		"warnings":   view.Warnings,
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
		"warnings": view.Warnings,
	}
}

func crPackToJSONMap(view *service.CRPackView) map[string]any {
	if view == nil || view.CR == nil {
		return map[string]any{}
	}
	tasks := make([]map[string]any, 0, len(view.Tasks))
	for _, task := range view.Tasks {
		chunks := make([]map[string]any, 0, len(task.CheckpointChunks))
		for _, chunk := range task.CheckpointChunks {
			chunks = append(chunks, map[string]any{
				"chunk_id":  chunk.ID,
				"path":      chunk.Path,
				"old_start": chunk.OldStart,
				"old_lines": chunk.OldLines,
				"new_start": chunk.NewStart,
				"new_lines": chunk.NewLines,
			})
		}
		tasks = append(tasks, map[string]any{
			"id":                 task.ID,
			"title":              task.Title,
			"status":             task.Status,
			"checkpoint_commit":  task.CheckpointCommit,
			"checkpoint_at":      task.CheckpointAt,
			"checkpoint_message": task.CheckpointMessage,
			"checkpoint_scope":   task.CheckpointScope,
			"checkpoint_source":  task.CheckpointSource,
			"checkpoint_orphan":  task.CheckpointOrphan,
			"checkpoint_reason":  task.CheckpointReason,
			"checkpoint_chunks":  chunks,
			"contract": map[string]any{
				"intent":              task.Contract.Intent,
				"acceptance_criteria": task.Contract.AcceptanceCriteria,
				"scope":               task.Contract.Scope,
				"updated_at":          task.Contract.UpdatedAt,
				"updated_by":          task.Contract.UpdatedBy,
			},
		})
	}

	events := make([]map[string]any, 0, len(view.RecentEvents))
	for _, event := range view.RecentEvents {
		events = append(events, map[string]any{
			"ts":      event.TS,
			"actor":   event.Actor,
			"type":    event.Type,
			"summary": event.Summary,
			"ref":     event.Ref,
			"meta":    event.Meta,
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
			"warnings":   view.Anchors.Warnings,
		}
	}

	return map[string]any{
		"cr": map[string]any{
			"id":            view.CR.ID,
			"uid":           view.CR.UID,
			"title":         view.CR.Title,
			"description":   view.CR.Description,
			"status":        view.CR.Status,
			"base_branch":   view.CR.BaseBranch,
			"base_ref":      view.CR.BaseRef,
			"base_commit":   view.CR.BaseCommit,
			"parent_cr_id":  view.CR.ParentCRID,
			"branch":        view.CR.Branch,
			"merged_at":     view.CR.MergedAt,
			"merged_by":     view.CR.MergedBy,
			"merged_commit": view.CR.MergedCommit,
			"created_at":    view.CR.CreatedAt,
			"updated_at":    view.CR.UpdatedAt,
		},
		"contract": map[string]any{
			"why":                  view.Contract.Why,
			"scope":                view.Contract.Scope,
			"non_goals":            view.Contract.NonGoals,
			"invariants":           view.Contract.Invariants,
			"blast_radius":         view.Contract.BlastRadius,
			"risk_critical_scopes": view.Contract.RiskCriticalScopes,
			"risk_tier_hint":       view.Contract.RiskTierHint,
			"risk_rationale":       view.Contract.RiskRationale,
			"test_plan":            view.Contract.TestPlan,
			"rollback_plan":        view.Contract.RollbackPlan,
			"updated_at":           view.Contract.UpdatedAt,
			"updated_by":           view.Contract.UpdatedBy,
		},
		"tasks":              tasks,
		"anchors":            anchors,
		"status":             crStatusToJSONMap(view.Status),
		"recent_events":      events,
		"events_meta":        packSliceMetaToJSONMap(view.EventsMeta),
		"recent_checkpoints": checkpoints,
		"checkpoints_meta":   packSliceMetaToJSONMap(view.CheckpointsMeta),
		"diff_stat":          view.DiffStat,
		"files_changed":      view.FilesChanged,
		"impact":             impactToJSONMap(view.Impact),
		"validation":         validationToJSONMap(view.Validation),
		"trust":              trustToJSONMap(view.Trust),
		"warnings":           view.Warnings,
	}
}

func packSliceMetaToJSONMap(meta service.PackSliceMeta) map[string]any {
	return map[string]any{
		"total":     meta.Total,
		"returned":  meta.Returned,
		"truncated": meta.Truncated,
	}
}
