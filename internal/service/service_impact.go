package service

import (
	"fmt"
	"sophia/internal/gitx"
	"sophia/internal/model"
	"sort"
	"strings"
)

func buildImpactReport(cr *model.CR, diff *diffSummary, policy *model.RepoPolicy) *ImpactReport {
	scope, scopeSource := scopeReferenceForCR(cr)
	scopeDrift := findScopeDrift(diff.Files, scope)
	taskScopeWarnings := findTaskScopeWarnings(cr.Subtasks, scope)
	taskRequiredFields := append([]string(nil), defaultTaskRequiredContractFields...)
	if policy != nil && len(policy.TaskContract.RequiredFields) > 0 {
		taskRequiredFields = append([]string(nil), policy.TaskContract.RequiredFields...)
	}
	taskContractWarnings := findTaskContractWarnings(cr.Subtasks, taskRequiredFields)
	taskChunkWarnings := findTaskChunkWarnings(cr.Subtasks)
	riskTierHint, _ := normalizeRiskTierHint(cr.Contract.RiskTierHint)
	matchedRiskCriticalScopes := findMatchedScopePrefixes(diff.Files, cr.Contract.RiskCriticalScopes)

	signals := []RiskSignal{}
	riskScore := 0
	addSignal := func(code, summary string, points int) {
		if points <= 0 {
			return
		}
		signals = append(signals, RiskSignal{Code: code, Summary: summary, Points: points})
		riskScore += points
	}

	if len(matchedRiskCriticalScopes) > 0 {
		addSignal("critical_scope_hint", fmt.Sprintf("contract critical scopes touched: %s", strings.Join(matchedRiskCriticalScopes, ", ")), 3)
	}
	if len(diff.DependencyFiles) > 0 {
		addSignal("dependency_changes", fmt.Sprintf("%d dependency file(s) changed", len(diff.DependencyFiles)), 2)
	}
	if len(diff.DeletedFiles) > 0 {
		addSignal("deletions", fmt.Sprintf("%d deleted file(s)", len(diff.DeletedFiles)), 2)
	}
	if len(diff.Files) > 20 {
		addSignal("large_change_set", fmt.Sprintf("%d files changed", len(diff.Files)), 2)
	}
	nonTestChanges := len(diff.Files) > len(diff.TestFiles)
	if nonTestChanges && len(diff.TestFiles) == 0 {
		addSignal("no_test_changes", "non-test changes detected without test file updates", 1)
	}
	if len(scopeDrift) > 0 {
		addSignal("scope_drift", fmt.Sprintf("%d file(s) outside declared scope", len(scopeDrift)), 2)
	}

	riskTierFloorApplied := false
	riskTier := riskTierFromScore(riskScore)
	if riskTierRank(riskTierHint) > riskTierRank(riskTier) {
		floor := riskFloorScoreForTier(riskTierHint)
		if floor > riskScore {
			delta := floor - riskScore
			addSignal("risk_tier_hint_floor", fmt.Sprintf("raised risk score to %s floor (+%d)", riskTierHint, delta), delta)
		}
		riskTier = riskTierHint
		riskTierFloorApplied = true
	}

	return &ImpactReport{
		CRID:                      cr.ID,
		CRUID:                     strings.TrimSpace(cr.UID),
		BaseRef:                   strings.TrimSpace(cr.BaseRef),
		BaseCommit:                strings.TrimSpace(cr.BaseCommit),
		ParentCRID:                cr.ParentCRID,
		ScopeSource:               scopeSource,
		RiskTierHint:              riskTierHint,
		RiskTierFloorApplied:      riskTierFloorApplied,
		MatchedRiskCriticalScopes: matchedRiskCriticalScopes,
		FilesChanged:              len(diff.Files),
		NewFiles:                  append([]string(nil), diff.NewFiles...),
		ModifiedFiles:             append([]string(nil), diff.ModifiedFiles...),
		DeletedFiles:              append([]string(nil), diff.DeletedFiles...),
		TestFiles:                 append([]string(nil), diff.TestFiles...),
		DependencyFiles:           append([]string(nil), diff.DependencyFiles...),
		ScopeDrift:                scopeDrift,
		TaskScopeWarnings:         taskScopeWarnings,
		TaskContractWarnings:      taskContractWarnings,
		TaskChunkWarnings:         taskChunkWarnings,
		Signals:                   signals,
		RiskScore:                 riskScore,
		RiskTier:                  riskTier,
	}
}

func scopeReferenceForCR(cr *model.CR) ([]string, string) {
	if cr != nil && !crContractBaselineIsEmpty(cr.ContractBaseline) {
		return append([]string(nil), cr.ContractBaseline.Scope...), "contract_baseline"
	}
	if cr == nil {
		return []string{}, "contract_scope"
	}
	return append([]string(nil), cr.Contract.Scope...), "contract_scope"
}

func findMatchedScopePrefixes(changedFiles, scopePrefixes []string) []string {
	if len(changedFiles) == 0 || len(scopePrefixes) == 0 {
		return []string{}
	}
	normalizedScopes := make([]string, 0, len(scopePrefixes))
	seenScopes := map[string]struct{}{}
	for _, rawScope := range scopePrefixes {
		scope := strings.TrimSpace(rawScope)
		if scope == "" {
			continue
		}
		if _, exists := seenScopes[scope]; exists {
			continue
		}
		seenScopes[scope] = struct{}{}
		normalizedScopes = append(normalizedScopes, scope)
	}
	sort.Strings(normalizedScopes)
	matched := []string{}
	for _, scope := range normalizedScopes {
		for _, changedPath := range changedFiles {
			if pathMatchesScopePrefix(changedPath, scope) {
				matched = append(matched, scope)
				break
			}
		}
	}
	return matched
}

func findScopeDrift(changedFiles, scopePrefixes []string) []string {
	if len(changedFiles) == 0 {
		return []string{}
	}
	if len(scopePrefixes) == 0 {
		drift := make([]string, 0, len(changedFiles))
		for _, changedPath := range changedFiles {
			if isScopeDriftExcludedPath(changedPath) {
				continue
			}
			drift = append(drift, changedPath)
		}
		sort.Strings(drift)
		return drift
	}
	drift := []string{}
	for _, changedPath := range changedFiles {
		if isScopeDriftExcludedPath(changedPath) {
			continue
		}
		inScope := false
		for _, scopePrefix := range scopePrefixes {
			if pathMatchesScopePrefix(changedPath, scopePrefix) {
				inScope = true
				break
			}
		}
		if !inScope {
			drift = append(drift, changedPath)
		}
	}
	sort.Strings(drift)
	return drift
}

func isScopeDriftExcludedPath(path string) bool {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return false
	}
	return trimmed == archiveTrackedPrefix || strings.HasPrefix(trimmed, archiveTrackedPrefix+"/")
}

func findTaskScopeWarnings(tasks []model.Subtask, scopePrefixes []string) []string {
	if len(scopePrefixes) == 0 {
		return []string{}
	}
	warnings := []string{}
	for _, task := range tasks {
		if task.Status != model.TaskStatusDone {
			continue
		}
		for _, scopedPath := range taskCheckpointPaths(task) {
			if strings.TrimSpace(scopedPath) == "" || scopedPath == "*" {
				continue
			}
			inScope := false
			for _, scopePrefix := range scopePrefixes {
				if pathMatchesScopePrefix(scopedPath, scopePrefix) {
					inScope = true
					break
				}
			}
			if !inScope {
				warnings = append(warnings, fmt.Sprintf("task #%d checkpoint scope %q is outside contract scope", task.ID, scopedPath))
			}
		}
	}
	sort.Strings(warnings)
	return dedupeStrings(warnings)
}

func findTaskContractWarnings(tasks []model.Subtask, requiredTaskFields []string) []string {
	warnings := []string{}
	for _, task := range tasks {
		if task.Status != model.TaskStatusDone {
			continue
		}
		missing := missingTaskContractFields(task.Contract, requiredTaskFields)
		if len(missing) > 0 {
			warnings = append(warnings, fmt.Sprintf("task #%d is done but missing contract fields: %s", task.ID, strings.Join(missing, ",")))
		}
		checkpointPaths := taskCheckpointPaths(task)
		if len(task.Contract.Scope) == 0 || len(checkpointPaths) == 0 {
			continue
		}
		for _, scopedPath := range checkpointPaths {
			if strings.TrimSpace(scopedPath) == "" || scopedPath == "*" {
				continue
			}
			inScope := false
			for _, taskScope := range task.Contract.Scope {
				if pathMatchesScopePrefix(scopedPath, taskScope) {
					inScope = true
					break
				}
			}
			if !inScope {
				warnings = append(warnings, fmt.Sprintf("task #%d checkpoint scope %q is outside task contract scope", task.ID, scopedPath))
			}
		}
	}
	sort.Strings(warnings)
	return dedupeStrings(warnings)
}

func deriveChangesFromTaskCheckpointScopes(tasks []model.Subtask) []gitx.FileChange {
	seen := map[string]struct{}{}
	changes := make([]gitx.FileChange, 0)
	for _, task := range tasks {
		for _, scopedPath := range taskCheckpointPaths(task) {
			scopedPath = strings.TrimSpace(scopedPath)
			if scopedPath == "" || scopedPath == "*" {
				continue
			}
			if _, ok := seen[scopedPath]; ok {
				continue
			}
			seen[scopedPath] = struct{}{}
			changes = append(changes, gitx.FileChange{
				Status: "M",
				Path:   scopedPath,
			})
		}
	}
	sort.Slice(changes, func(i, j int) bool {
		return changes[i].Path < changes[j].Path
	})
	return changes
}

func findTaskChunkWarnings(tasks []model.Subtask) []string {
	warnings := []string{}
	for _, task := range tasks {
		if task.Status != model.TaskStatusDone || len(task.CheckpointChunks) == 0 {
			continue
		}
		seenChunkIDs := map[string]struct{}{}
		seenScopePaths := map[string]struct{}{}
		for _, scopePath := range task.CheckpointScope {
			trimmed := strings.TrimSpace(scopePath)
			if trimmed == "" || trimmed == "*" {
				continue
			}
			seenScopePaths[trimmed] = struct{}{}
		}
		for _, chunk := range task.CheckpointChunks {
			if strings.TrimSpace(chunk.ID) == "" {
				warnings = append(warnings, fmt.Sprintf("task #%d has checkpoint chunk with missing id", task.ID))
			} else {
				if _, exists := seenChunkIDs[chunk.ID]; exists {
					warnings = append(warnings, fmt.Sprintf("task #%d has duplicate checkpoint chunk id %q", task.ID, chunk.ID))
				}
				seenChunkIDs[chunk.ID] = struct{}{}
			}
			if strings.TrimSpace(chunk.Path) == "" {
				warnings = append(warnings, fmt.Sprintf("task #%d has checkpoint chunk %q with missing path", task.ID, chunk.ID))
			} else if len(seenScopePaths) > 0 {
				if _, inScope := seenScopePaths[chunk.Path]; !inScope {
					warnings = append(warnings, fmt.Sprintf("task #%d checkpoint chunk %q path %q is not present in checkpoint_scope", task.ID, chunk.ID, chunk.Path))
				}
			}
			if chunk.OldStart <= 0 || chunk.NewStart <= 0 {
				warnings = append(warnings, fmt.Sprintf("task #%d checkpoint chunk %q has invalid line starts", task.ID, chunk.ID))
			}
			if chunk.OldLines < 0 || chunk.NewLines < 0 {
				warnings = append(warnings, fmt.Sprintf("task #%d checkpoint chunk %q has invalid line counts", task.ID, chunk.ID))
			}
		}
	}
	sort.Strings(warnings)
	return dedupeStrings(warnings)
}

func taskCheckpointPaths(task model.Subtask) []string {
	if len(task.CheckpointScope) > 0 {
		return append([]string(nil), task.CheckpointScope...)
	}
	if len(task.CheckpointChunks) == 0 {
		return []string{}
	}
	paths := make([]string, 0, len(task.CheckpointChunks))
	seen := map[string]struct{}{}
	for _, chunk := range task.CheckpointChunks {
		path := strings.TrimSpace(chunk.Path)
		if path == "" {
			continue
		}
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return paths
}

func missingCRContractFields(contract model.Contract, requiredFields []string) []string {
	if len(requiredFields) == 0 {
		requiredFields = defaultCRRequiredContractFields
	}
	missing := []string{}
	for _, field := range requiredFields {
		switch field {
		case "why":
			if strings.TrimSpace(contract.Why) == "" {
				missing = append(missing, field)
			}
		case "scope":
			if len(contract.Scope) == 0 {
				missing = append(missing, field)
			}
		case "non_goals":
			if len(normalizeNonEmptyStringList(contract.NonGoals)) == 0 {
				missing = append(missing, field)
			}
		case "invariants":
			if len(normalizeNonEmptyStringList(contract.Invariants)) == 0 {
				missing = append(missing, field)
			}
		case "blast_radius":
			if strings.TrimSpace(contract.BlastRadius) == "" {
				missing = append(missing, field)
			}
		case "risk_critical_scopes":
			if len(contract.RiskCriticalScopes) == 0 {
				missing = append(missing, field)
			}
		case "risk_tier_hint":
			if strings.TrimSpace(contract.RiskTierHint) == "" {
				missing = append(missing, field)
			}
		case "risk_rationale":
			if strings.TrimSpace(contract.RiskRationale) == "" {
				missing = append(missing, field)
			}
		case "test_plan":
			if strings.TrimSpace(contract.TestPlan) == "" {
				missing = append(missing, field)
			}
		case "rollback_plan":
			if strings.TrimSpace(contract.RollbackPlan) == "" {
				missing = append(missing, field)
			}
		}
	}
	sort.Strings(missing)
	missing = dedupeStrings(missing)
	return missing
}

func missingTaskContractFields(contract model.TaskContract, requiredFields []string) []string {
	if len(requiredFields) == 0 {
		requiredFields = defaultTaskRequiredContractFields
	}
	missing := []string{}
	for _, field := range requiredFields {
		switch field {
		case "intent":
			if strings.TrimSpace(contract.Intent) == "" {
				missing = append(missing, field)
			}
		case "acceptance_criteria":
			if len(normalizeNonEmptyStringList(contract.AcceptanceCriteria)) == 0 {
				missing = append(missing, field)
			}
		case "scope":
			if len(contract.Scope) == 0 {
				missing = append(missing, field)
			}
		}
	}
	sort.Strings(missing)
	missing = dedupeStrings(missing)
	return missing
}

func pathMatchesScopePrefix(candidatePath, scopePrefix string) bool {
	candidatePath = strings.TrimSpace(candidatePath)
	scopePrefix = strings.TrimSpace(scopePrefix)
	if candidatePath == "" || scopePrefix == "" {
		return false
	}
	if scopePrefix == "." {
		return true
	}
	if candidatePath == scopePrefix {
		return true
	}
	return strings.HasPrefix(candidatePath, scopePrefix+"/")
}

func (s *Service) computeOverlapWarnings(referenceDirs map[string]struct{}, skipCRID int) []string {
	if len(referenceDirs) == 0 {
		return nil
	}
	crs, err := s.store.ListCRs()
	if err != nil {
		return nil
	}
	warnings := make([]string, 0)
	for _, cr := range crs {
		if cr.ID == skipCRID || cr.Status != model.StatusInProgress {
			continue
		}
		if !s.git.BranchExists(cr.Branch) || !s.git.BranchExists(cr.BaseBranch) {
			continue
		}
		files, diffErr := s.git.DiffNames(cr.BaseBranch, cr.Branch)
		if diffErr != nil {
			continue
		}
		dirs := topLevelDirs(files)
		for dir := range referenceDirs {
			if _, ok := dirs[dir]; ok {
				warnings = append(warnings, fmt.Sprintf("Potential overlap: CR-%d also touches /%s", cr.ID, dir))
			}
		}
	}
	sort.Strings(warnings)
	return dedupeStrings(warnings)
}

func topLevelDirs(paths []string) map[string]struct{} {
	res := map[string]struct{}{}
	for _, path := range paths {
		path = strings.TrimSpace(path)
		if path == "" {
			continue
		}
		first := path
		if idx := strings.Index(path, "/"); idx >= 0 {
			first = path[:idx]
		}
		if strings.TrimSpace(first) == "" {
			continue
		}
		res[first] = struct{}{}
	}
	return res
}

func isTestFile(path string, policy *model.RepoPolicy) bool {
	suffixes := defaultTestSuffixes
	pathContains := defaultTestPathContains
	if policy != nil {
		if len(policy.Classification.Test.Suffixes) > 0 {
			suffixes = policy.Classification.Test.Suffixes
		}
		if len(policy.Classification.Test.PathContains) > 0 {
			pathContains = policy.Classification.Test.PathContains
		}
	}
	lower := strings.ToLower(path)
	for _, contains := range pathContains {
		if strings.TrimSpace(contains) != "" && strings.Contains(lower, strings.ToLower(strings.TrimSpace(contains))) {
			return true
		}
	}
	for _, suffix := range suffixes {
		if strings.HasSuffix(lower, suffix) {
			return true
		}
	}
	return false
}

func isDependencyFile(path string, policy *model.RepoPolicy) bool {
	fileNames := defaultDependencyFileNames
	if policy != nil && len(policy.Classification.Dependency.FileNames) > 0 {
		fileNames = policy.Classification.Dependency.FileNames
	}
	names := map[string]struct{}{}
	for _, fileName := range fileNames {
		trimmed := strings.ToLower(strings.TrimSpace(fileName))
		if trimmed == "" {
			continue
		}
		names[trimmed] = struct{}{}
	}
	lower := strings.ToLower(path)
	parts := strings.Split(lower, "/")
	if len(parts) == 0 {
		return false
	}
	_, ok := names[parts[len(parts)-1]]
	return ok
}
