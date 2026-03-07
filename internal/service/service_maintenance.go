package service

import (
	"errors"
	"fmt"
	"sophia/internal/model"
	"sophia/internal/store"
	"sort"
	"strconv"
	"strings"
	"time"
)

func (s *Service) Log() ([]LogEntry, error) {
	crs, err := s.store.ListCRs()
	if err == nil && len(crs) > 0 {
		return buildLogEntriesFromCRs(crs), nil
	}
	if err != nil && !errors.Is(err, store.ErrNotInitialized) {
		return nil, err
	}
	return s.logFromGit(200)
}

func buildLogEntriesFromCRs(crs []model.CR) []LogEntry {
	type stampedEntry struct {
		entry LogEntry
		ts    time.Time
	}
	merged := make([]stampedEntry, 0)
	active := make([]stampedEntry, 0)

	for _, cr := range crs {
		if cr.Status == model.StatusMerged {
			when := cr.MergedAt
			if when == "" {
				when = cr.UpdatedAt
			}
			filesTouched := "-"
			if cr.MergedAt != "" || cr.MergedCommit != "" {
				filesTouched = strconv.Itoa(cr.FilesTouchedCount)
			}
			entry := stampedEntry{
				entry: LogEntry{
					ID:           cr.ID,
					Title:        cr.Title,
					Status:       cr.Status,
					Who:          nonEmptyTrimmed(cr.MergedBy, "-"),
					When:         when,
					FilesTouched: filesTouched,
				},
				ts: parseRFC3339OrZero(when),
			}
			merged = append(merged, entry)
			continue
		}
		entry := stampedEntry{
			entry: LogEntry{
				ID:           cr.ID,
				Title:        cr.Title,
				Status:       cr.Status,
				Who:          "-",
				When:         cr.UpdatedAt,
				FilesTouched: "-",
			},
			ts: parseRFC3339OrZero(cr.UpdatedAt),
		}
		active = append(active, entry)
	}

	sort.Slice(merged, func(i, j int) bool {
		return merged[i].ts.After(merged[j].ts)
	})
	sort.Slice(active, func(i, j int) bool {
		return active[i].ts.After(active[j].ts)
	})

	res := make([]LogEntry, 0, len(merged)+len(active))
	for _, item := range merged {
		res = append(res, item.entry)
	}
	for _, item := range active {
		res = append(res, item.entry)
	}
	return res
}

func (s *Service) logFromGit(limit int) ([]LogEntry, error) {
	branch := s.git.DefaultBranch()
	commits, err := s.git.RecentCommits(branch, limit)
	if err != nil {
		return nil, err
	}
	entries := make([]LogEntry, 0)
	seen := map[int]struct{}{}
	for _, commit := range commits {
		id, ok := crIDFromSubjectOrBody(commit.Subject, commit.Body)
		if !ok {
			continue
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}

		filesCount, countErr := s.git.ChangedFileCount(commit.Hash)
		filesTouched := "-"
		if countErr == nil {
			filesTouched = strconv.Itoa(filesCount)
		}
		entries = append(entries, LogEntry{
			ID:           id,
			Title:        titleFromSubjectOrBody(commit.Subject, commit.Body),
			Status:       model.StatusMerged,
			Who:          nonEmptyTrimmed(commit.Author, "-"),
			When:         nonEmptyTrimmed(commit.When, "-"),
			FilesTouched: filesTouched,
		})
	}
	return entries, nil
}

func (s *Service) RepairFromGit(baseBranch string, refresh bool) (*RepairReport, error) {
	targetBase := strings.TrimSpace(baseBranch)
	if targetBase == "" {
		if s.store.IsInitialized() {
			cfg, err := s.store.LoadConfig()
			if err == nil && strings.TrimSpace(cfg.BaseBranch) != "" {
				targetBase = strings.TrimSpace(cfg.BaseBranch)
			}
		}
	}
	if targetBase == "" {
		targetBase = s.git.DefaultBranch()
	}
	if !s.git.BranchExists(targetBase) {
		return nil, fmt.Errorf("base branch %q does not exist", targetBase)
	}

	mode := model.MetadataModeLocal
	if s.store.IsInitialized() {
		if cfg, err := s.store.LoadConfig(); err == nil && strings.TrimSpace(cfg.MetadataMode) != "" {
			mode = cfg.MetadataMode
		}
	}
	if err := s.store.Init(targetBase, mode); err != nil {
		return nil, err
	}

	existingMap := map[int]*model.CR{}
	if existing, err := s.store.ListCRs(); err == nil {
		for i := range existing {
			cr := existing[i]
			c := cr
			existingMap[cr.ID] = &c
		}
	}

	commits, err := s.git.RecentCommits(targetBase, 5000)
	if err != nil {
		return nil, err
	}

	report := &RepairReport{
		BaseBranch:    targetBase,
		Scanned:       len(commits),
		RepairedCRIDs: []int{},
		Warnings:      []string{},
	}

	uidBackfilledSet := map[int]struct{}{}
	for id, existing := range existingMap {
		changedUID, uidErr := ensureCRUID(existing)
		if uidErr != nil {
			return nil, uidErr
		}
		changedBase, baseErr := s.ensureCRBaseFields(existing, false)
		if baseErr != nil {
			return nil, baseErr
		}
		if !changedUID && !changedBase {
			continue
		}
		if strings.TrimSpace(existing.CreatedAt) == "" {
			report.Warnings = appendUniqueString(report.Warnings, fmt.Sprintf("CR %d has empty created_at; repair preserved unknown chronology while backfilling metadata", id))
		}
		existing.UpdatedAt = s.timestamp()
		if err := s.store.SaveCR(existing); err != nil {
			return nil, err
		}
		report.Updated++
		uidBackfilledSet[id] = struct{}{}
	}

	repairedSet := map[int]struct{}{}
	for _, commit := range commits {
		id, ok := crIDFromSubjectOrBody(commit.Subject, commit.Body)
		if !ok {
			continue
		}

		existing, exists := existingMap[id]
		if exists && existing.Status == model.StatusInProgress && !refresh {
			report.Skipped++
			continue
		}

		description := intentFromCommitBody(commit.Body)
		notes := notesFromCommitBody(commit.Body)
		when, timestampSource := normalizeRepairCommitTimestamp(commit.When)
		subtasks := subtasksFromCommitBody(commit.Body, when, commit.Author)
		actor := nonEmptyTrimmed(commit.Author, "unknown")
		filesTouched := 0
		if count, countErr := s.git.ChangedFileCount(commit.Hash); countErr == nil {
			filesTouched = count
		}
		if timestampSource != "commit_author_time" {
			report.Warnings = appendUniqueString(report.Warnings, fmt.Sprintf("CR %d commit %s has %s timestamp metadata; chronology fields remain explicit/unknown", id, shortHash(commit.Hash), timestampSource))
		}

		title := titleFromSubjectOrBody(commit.Subject, commit.Body)
		branch := strings.TrimSpace(branchFromBody(commit.Body))
		if branch == "" {
			branch = legacyCRBranchName(id)
			if alias, aliasErr := formatCRBranchAlias(id, title, ""); aliasErr == nil {
				branch = alias
			}
		}

		cr := &model.CR{
			ID:                id,
			UID:               crUIDFromBody(commit.Body),
			Title:             title,
			Description:       description,
			Status:            model.StatusMerged,
			BaseBranch:        targetBase,
			BaseRef:           baseRefFromBody(commit.Body),
			BaseCommit:        baseCommitFromBody(commit.Body),
			ParentCRID:        parentCRIDFromBody(commit.Body),
			Branch:            branch,
			Notes:             notes,
			Subtasks:          subtasks,
			Events:            []model.Event{},
			MergedAt:          when,
			MergedBy:          actor,
			MergedCommit:      shortHash(commit.Hash),
			FilesTouchedCount: filesTouched,
			CreatedAt:         when,
			UpdatedAt:         when,
		}

		if exists {
			cr = mergeRecoveredCR(existing, cr)
			if strings.TrimSpace(cr.CreatedAt) == "" {
				report.Warnings = appendUniqueString(report.Warnings, fmt.Sprintf("CR %d repaired without created_at; chronology remains unknown", id))
			}
			cr.Events = append([]model.Event{}, existing.Events...)
			if _, backfilled := uidBackfilledSet[id]; !backfilled {
				report.Updated++
			}
		} else {
			report.Imported++
		}
		if _, uidErr := ensureCRUID(cr); uidErr != nil {
			return nil, uidErr
		}
		if _, baseErr := s.ensureCRBaseFields(cr, false); baseErr != nil {
			return nil, baseErr
		}
		eventMeta := map[string]string{
			"repair_timestamp_source": timestampSource,
		}
		if strings.TrimSpace(cr.CreatedAt) == "" {
			eventMeta["repair_created_at_state"] = "unknown"
		}
		if strings.TrimSpace(cr.MergedAt) == "" {
			eventMeta["repair_merged_at_state"] = "unknown"
		}
		cr.Events = append(cr.Events, model.Event{
			TS:      s.timestamp(),
			Actor:   s.git.Actor(),
			Type:    model.EventTypeCRRepaired,
			Summary: fmt.Sprintf("Repaired CR %d from git commit %s", id, shortHash(commit.Hash)),
			Ref:     fmt.Sprintf("cr:%d", id),
			Meta:    eventMeta,
		})

		if err := s.store.SaveCR(cr); err != nil {
			return nil, err
		}
		existingMap[id] = cr
		if id > report.HighestCRID {
			report.HighestCRID = id
		}
		if _, seen := repairedSet[id]; !seen {
			repairedSet[id] = struct{}{}
			report.RepairedCRIDs = append(report.RepairedCRIDs, id)
		}
	}

	allCRs := make([]model.CR, 0, len(existingMap))
	for _, cr := range existingMap {
		if cr == nil {
			continue
		}
		allCRs = append(allCRs, *cr)
	}
	if err := s.repairDelegatedParentTaskState(); err != nil {
		return nil, err
	}
	allCRs = allCRs[:0]
	refreshedCRs, listErr := s.store.ListCRs()
	if listErr != nil {
		return nil, listErr
	}
	for _, cr := range refreshedCRs {
		allCRs = append(allCRs, cr)
	}
	if err := s.syncAllCRRefs(allCRs); err != nil {
		return nil, err
	}

	if err := s.ensureNextCRIDFloor(targetBase); err != nil {
		return nil, err
	}
	idx, err := s.store.LoadIndex()
	if err != nil {
		return nil, err
	}
	report.NextID = idx.NextID
	sort.Ints(report.RepairedCRIDs)
	return report, nil
}

func mergeRecoveredCR(existing, recovered *model.CR) *model.CR {
	if recovered == nil {
		return existing
	}
	if existing == nil {
		return recovered
	}

	merged := *recovered
	if strings.TrimSpace(merged.UID) == "" {
		merged.UID = strings.TrimSpace(existing.UID)
	}
	if strings.TrimSpace(merged.Description) == "" {
		merged.Description = strings.TrimSpace(existing.Description)
	}
	if strings.TrimSpace(merged.BaseRef) == "" {
		merged.BaseRef = strings.TrimSpace(existing.BaseRef)
	}
	if strings.TrimSpace(merged.BaseCommit) == "" {
		merged.BaseCommit = strings.TrimSpace(existing.BaseCommit)
	}
	if merged.ParentCRID <= 0 {
		merged.ParentCRID = existing.ParentCRID
	}
	if strings.TrimSpace(merged.Branch) == "" {
		merged.Branch = strings.TrimSpace(existing.Branch)
	}
	if len(merged.Notes) == 0 && len(existing.Notes) > 0 {
		merged.Notes = append([]string(nil), existing.Notes...)
	}
	if len(merged.Evidence) == 0 && len(existing.Evidence) > 0 {
		merged.Evidence = append([]model.EvidenceEntry(nil), existing.Evidence...)
	}
	if len(merged.DelegationRuns) == 0 && len(existing.DelegationRuns) > 0 {
		merged.DelegationRuns = append([]model.DelegationRun(nil), existing.DelegationRuns...)
	}
	merged.Contract = mergeRecoveredContract(existing.Contract, merged.Contract)
	if isEmptyCRContractBaseline(merged.ContractBaseline) {
		merged.ContractBaseline = existing.ContractBaseline
	}
	if len(merged.ContractDrifts) == 0 && len(existing.ContractDrifts) > 0 {
		merged.ContractDrifts = append([]model.CRContractDrift(nil), existing.ContractDrifts...)
	}
	merged.Subtasks = mergeRecoveredSubtasks(existing.Subtasks, merged.Subtasks)
	merged.HQ = mergeRecoveredHQState(existing.HQ, merged.HQ)
	merged.PR = mergeRecoveredPRLink(existing.PR, merged.PR)
	if strings.TrimSpace(merged.CreatedAt) == "" {
		merged.CreatedAt = strings.TrimSpace(existing.CreatedAt)
	}
	if strings.TrimSpace(merged.MergedAt) == "" {
		merged.MergedAt = strings.TrimSpace(existing.MergedAt)
	}
	if strings.TrimSpace(merged.MergedBy) == "" {
		merged.MergedBy = strings.TrimSpace(existing.MergedBy)
	}
	if strings.TrimSpace(merged.MergedCommit) == "" {
		merged.MergedCommit = strings.TrimSpace(existing.MergedCommit)
	}
	if merged.FilesTouchedCount == 0 {
		merged.FilesTouchedCount = existing.FilesTouchedCount
	}
	return &merged
}

func mergeRecoveredContract(existing, recovered model.Contract) model.Contract {
	merged := recovered
	if strings.TrimSpace(merged.Why) == "" {
		merged.Why = strings.TrimSpace(existing.Why)
	}
	if len(merged.Scope) == 0 && len(existing.Scope) > 0 {
		merged.Scope = append([]string(nil), existing.Scope...)
	}
	if len(merged.NonGoals) == 0 && len(existing.NonGoals) > 0 {
		merged.NonGoals = append([]string(nil), existing.NonGoals...)
	}
	if len(merged.Invariants) == 0 && len(existing.Invariants) > 0 {
		merged.Invariants = append([]string(nil), existing.Invariants...)
	}
	if strings.TrimSpace(merged.BlastRadius) == "" {
		merged.BlastRadius = strings.TrimSpace(existing.BlastRadius)
	}
	if len(merged.RiskCriticalScopes) == 0 && len(existing.RiskCriticalScopes) > 0 {
		merged.RiskCriticalScopes = append([]string(nil), existing.RiskCriticalScopes...)
	}
	if strings.TrimSpace(merged.RiskTierHint) == "" {
		merged.RiskTierHint = strings.TrimSpace(existing.RiskTierHint)
	}
	if strings.TrimSpace(merged.RiskRationale) == "" {
		merged.RiskRationale = strings.TrimSpace(existing.RiskRationale)
	}
	if strings.TrimSpace(merged.TestPlan) == "" {
		merged.TestPlan = strings.TrimSpace(existing.TestPlan)
	}
	if strings.TrimSpace(merged.RollbackPlan) == "" {
		merged.RollbackPlan = strings.TrimSpace(existing.RollbackPlan)
	}
	if strings.TrimSpace(merged.UpdatedAt) == "" {
		merged.UpdatedAt = strings.TrimSpace(existing.UpdatedAt)
	}
	if strings.TrimSpace(merged.UpdatedBy) == "" {
		merged.UpdatedBy = strings.TrimSpace(existing.UpdatedBy)
	}
	return merged
}

func mergeRecoveredSubtasks(existing, recovered []model.Subtask) []model.Subtask {
	if len(existing) == 0 {
		return recovered
	}
	if len(recovered) == 0 {
		return append([]model.Subtask(nil), existing...)
	}

	existingByID := make(map[int]model.Subtask, len(existing))
	used := make(map[int]struct{}, len(recovered))
	for _, task := range existing {
		existingByID[task.ID] = task
	}

	merged := make([]model.Subtask, 0, len(existing))
	for _, recoveredTask := range recovered {
		if existingTask, ok := existingByID[recoveredTask.ID]; ok {
			merged = append(merged, mergeRecoveredSubtask(existingTask, recoveredTask))
			used[recoveredTask.ID] = struct{}{}
			continue
		}
		merged = append(merged, recoveredTask)
	}
	for _, existingTask := range existing {
		if _, ok := used[existingTask.ID]; ok {
			continue
		}
		merged = append(merged, existingTask)
	}
	sort.SliceStable(merged, func(i, j int) bool {
		return merged[i].ID < merged[j].ID
	})
	return merged
}

func mergeRecoveredSubtask(existing, recovered model.Subtask) model.Subtask {
	merged := recovered
	if strings.TrimSpace(merged.Title) == "" {
		merged.Title = strings.TrimSpace(existing.Title)
	}
	if strings.TrimSpace(merged.Status) == "" {
		merged.Status = strings.TrimSpace(existing.Status)
	}
	if strings.TrimSpace(merged.CreatedAt) == "" {
		merged.CreatedAt = strings.TrimSpace(existing.CreatedAt)
	}
	if strings.TrimSpace(merged.UpdatedAt) == "" {
		merged.UpdatedAt = strings.TrimSpace(existing.UpdatedAt)
	}
	if strings.TrimSpace(merged.CompletedAt) == "" {
		merged.CompletedAt = strings.TrimSpace(existing.CompletedAt)
	}
	if strings.TrimSpace(merged.CreatedBy) == "" {
		merged.CreatedBy = strings.TrimSpace(existing.CreatedBy)
	}
	if strings.TrimSpace(merged.CompletedBy) == "" {
		merged.CompletedBy = strings.TrimSpace(existing.CompletedBy)
	}
	if strings.TrimSpace(merged.CheckpointCommit) == "" {
		merged.CheckpointCommit = strings.TrimSpace(existing.CheckpointCommit)
	}
	if strings.TrimSpace(merged.CheckpointAt) == "" {
		merged.CheckpointAt = strings.TrimSpace(existing.CheckpointAt)
	}
	if strings.TrimSpace(merged.CheckpointMessage) == "" {
		merged.CheckpointMessage = strings.TrimSpace(existing.CheckpointMessage)
	}
	if len(merged.CheckpointScope) == 0 && len(existing.CheckpointScope) > 0 {
		merged.CheckpointScope = append([]string(nil), existing.CheckpointScope...)
	}
	if len(merged.CheckpointChunks) == 0 && len(existing.CheckpointChunks) > 0 {
		merged.CheckpointChunks = append([]model.CheckpointChunk(nil), existing.CheckpointChunks...)
	}
	if !merged.CheckpointOrphan {
		merged.CheckpointOrphan = existing.CheckpointOrphan
	}
	if strings.TrimSpace(merged.CheckpointReason) == "" {
		merged.CheckpointReason = strings.TrimSpace(existing.CheckpointReason)
	}
	if strings.TrimSpace(merged.CheckpointSource) == "" {
		merged.CheckpointSource = strings.TrimSpace(existing.CheckpointSource)
	}
	if strings.TrimSpace(merged.CheckpointSyncAt) == "" {
		merged.CheckpointSyncAt = strings.TrimSpace(existing.CheckpointSyncAt)
	}
	if len(merged.Delegations) == 0 && len(existing.Delegations) > 0 {
		merged.Delegations = append([]model.TaskDelegation(nil), existing.Delegations...)
	}
	merged.Contract = mergeRecoveredTaskContract(existing.Contract, merged.Contract)
	if isEmptyTaskContractBaseline(merged.ContractBaseline) {
		merged.ContractBaseline = existing.ContractBaseline
	}
	if len(merged.ContractDrifts) == 0 && len(existing.ContractDrifts) > 0 {
		merged.ContractDrifts = append([]model.TaskContractDrift(nil), existing.ContractDrifts...)
	}
	return merged
}

func mergeRecoveredTaskContract(existing, recovered model.TaskContract) model.TaskContract {
	merged := recovered
	if strings.TrimSpace(merged.Intent) == "" {
		merged.Intent = strings.TrimSpace(existing.Intent)
	}
	if len(merged.AcceptanceCriteria) == 0 && len(existing.AcceptanceCriteria) > 0 {
		merged.AcceptanceCriteria = append([]string(nil), existing.AcceptanceCriteria...)
	}
	if len(merged.Scope) == 0 && len(existing.Scope) > 0 {
		merged.Scope = append([]string(nil), existing.Scope...)
	}
	if len(merged.AcceptanceChecks) == 0 && len(existing.AcceptanceChecks) > 0 {
		merged.AcceptanceChecks = append([]string(nil), existing.AcceptanceChecks...)
	}
	if strings.TrimSpace(merged.UpdatedAt) == "" {
		merged.UpdatedAt = strings.TrimSpace(existing.UpdatedAt)
	}
	if strings.TrimSpace(merged.UpdatedBy) == "" {
		merged.UpdatedBy = strings.TrimSpace(existing.UpdatedBy)
	}
	return merged
}

func mergeRecoveredHQState(existing, recovered model.CRHQState) model.CRHQState {
	merged := recovered
	if strings.TrimSpace(merged.RemoteAlias) == "" {
		merged.RemoteAlias = strings.TrimSpace(existing.RemoteAlias)
	}
	if strings.TrimSpace(merged.RepoID) == "" {
		merged.RepoID = strings.TrimSpace(existing.RepoID)
	}
	if strings.TrimSpace(merged.UpstreamFingerprint) == "" {
		merged.UpstreamFingerprint = strings.TrimSpace(existing.UpstreamFingerprint)
	}
	if merged.UpstreamIntent == nil && existing.UpstreamIntent != nil {
		intentCopy := *existing.UpstreamIntent
		merged.UpstreamIntent = &intentCopy
	}
	if strings.TrimSpace(merged.LastPullAt) == "" {
		merged.LastPullAt = strings.TrimSpace(existing.LastPullAt)
	}
	if strings.TrimSpace(merged.LastPushAt) == "" {
		merged.LastPushAt = strings.TrimSpace(existing.LastPushAt)
	}
	return merged
}

func mergeRecoveredPRLink(existing, recovered model.CRPRLink) model.CRPRLink {
	merged := recovered
	if strings.TrimSpace(merged.Provider) == "" {
		merged.Provider = strings.TrimSpace(existing.Provider)
	}
	if strings.TrimSpace(merged.Repo) == "" {
		merged.Repo = strings.TrimSpace(existing.Repo)
	}
	if merged.Number <= 0 {
		merged.Number = existing.Number
	}
	if strings.TrimSpace(merged.URL) == "" {
		merged.URL = strings.TrimSpace(existing.URL)
	}
	if strings.TrimSpace(merged.State) == "" {
		merged.State = strings.TrimSpace(existing.State)
	}
	if !merged.Draft {
		merged.Draft = existing.Draft
	}
	if strings.TrimSpace(merged.LastHeadSHA) == "" {
		merged.LastHeadSHA = strings.TrimSpace(existing.LastHeadSHA)
	}
	if strings.TrimSpace(merged.LastBaseRef) == "" {
		merged.LastBaseRef = strings.TrimSpace(existing.LastBaseRef)
	}
	if strings.TrimSpace(merged.LastBodyHash) == "" {
		merged.LastBodyHash = strings.TrimSpace(existing.LastBodyHash)
	}
	if strings.TrimSpace(merged.LastSyncedAt) == "" {
		merged.LastSyncedAt = strings.TrimSpace(existing.LastSyncedAt)
	}
	if strings.TrimSpace(merged.LastStatusCheckedAt) == "" {
		merged.LastStatusCheckedAt = strings.TrimSpace(existing.LastStatusCheckedAt)
	}
	if strings.TrimSpace(merged.LastMergedAt) == "" {
		merged.LastMergedAt = strings.TrimSpace(existing.LastMergedAt)
	}
	if strings.TrimSpace(merged.LastMergedCommit) == "" {
		merged.LastMergedCommit = strings.TrimSpace(existing.LastMergedCommit)
	}
	if len(merged.CheckpointCommentKeys) == 0 && len(existing.CheckpointCommentKeys) > 0 {
		merged.CheckpointCommentKeys = append([]string(nil), existing.CheckpointCommentKeys...)
	}
	if len(merged.CheckpointSyncKeys) == 0 && len(existing.CheckpointSyncKeys) > 0 {
		merged.CheckpointSyncKeys = append([]string(nil), existing.CheckpointSyncKeys...)
	}
	if !merged.AwaitingOpenApproval {
		merged.AwaitingOpenApproval = existing.AwaitingOpenApproval
	}
	if strings.TrimSpace(merged.AwaitingOpenApprovalNote) == "" {
		merged.AwaitingOpenApprovalNote = strings.TrimSpace(existing.AwaitingOpenApprovalNote)
	}
	return merged
}

func isEmptyTaskContractBaseline(baseline model.TaskContractBaseline) bool {
	return strings.TrimSpace(baseline.CapturedAt) == "" &&
		strings.TrimSpace(baseline.CapturedBy) == "" &&
		strings.TrimSpace(baseline.Intent) == "" &&
		len(baseline.AcceptanceCriteria) == 0 &&
		len(baseline.Scope) == 0 &&
		len(baseline.AcceptanceChecks) == 0
}

func isEmptyCRContractBaseline(baseline model.CRContractBaseline) bool {
	return strings.TrimSpace(baseline.CapturedAt) == "" &&
		strings.TrimSpace(baseline.CapturedBy) == "" &&
		len(baseline.Scope) == 0
}

func normalizeRepairCommitTimestamp(raw string) (string, string) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", "missing"
	}
	if parseRFC3339OrZero(trimmed).IsZero() {
		return "", "invalid"
	}
	return trimmed, "commit_author_time"
}

func (s *Service) InstallHook(forceOverwrite bool) (string, error) {
	if err := s.store.EnsureInitialized(); err != nil {
		return "", err
	}
	cfg, err := s.store.LoadConfig()
	if err != nil {
		return "", err
	}
	return s.git.InstallPreCommitHook(cfg.BaseBranch, forceOverwrite)
}
