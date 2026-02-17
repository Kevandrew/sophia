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
			"id":                task.ID,
			"title":             task.Title,
			"status":            task.Status,
			"checkpoint_commit": task.CheckpointCommit,
			"checkpoint_at":     task.CheckpointAt,
			"checkpoint_scope":  task.CheckpointScope,
			"checkpoint_chunks": chunkMaps,
			"delegations":       delegationMaps,
		})
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
		"new_files":           review.NewFiles,
		"modified_files":      review.ModifiedFiles,
		"deleted_files":       review.DeletedFiles,
		"test_files_touched":  review.TestFiles,
		"dependency_files":    review.DependencyFiles,
		"files_changed":       review.Files,
		"diff_stat":           review.ShortStat,
		"impact":              impactToJSONMap(review.Impact),
		"validation_errors":   review.ValidationErrors,
		"validation_warnings": review.ValidationWarnings,
	}
}
