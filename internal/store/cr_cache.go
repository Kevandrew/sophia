package store

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"sophia/internal/model"
)

type crFileFingerprint struct {
	Size    int64
	ModTime int64
}

type crMetadataCache struct {
	loaded       bool
	fingerprints map[string]crFileFingerprint
	crs          []model.CR
	byID         map[int]model.CR
}

func (s *Store) cachedCRByID(id int) (*model.CR, error) {
	s.crFilesMu.Lock()
	defer s.crFilesMu.Unlock()
	if err := s.refreshCRCacheLocked(); err != nil {
		return nil, err
	}
	s.cacheMu.RLock()
	defer s.cacheMu.RUnlock()
	cr, ok := s.crCache.byID[id]
	if !ok {
		return nil, NotFoundError{Resource: "cr", Value: fmt.Sprintf("%d", id)}
	}
	copyCR := cloneCR(cr)
	return &copyCR, nil
}

func (s *Store) cachedCRs() ([]model.CR, error) {
	s.crFilesMu.Lock()
	defer s.crFilesMu.Unlock()
	if err := s.refreshCRCacheLocked(); err != nil {
		return nil, err
	}
	s.cacheMu.RLock()
	defer s.cacheMu.RUnlock()
	return cloneCRSlice(s.crCache.crs), nil
}

func (s *Store) refreshCRCache() error {
	if err := s.EnsureInitialized(); err != nil {
		return err
	}
	s.crFilesMu.Lock()
	defer s.crFilesMu.Unlock()
	return s.refreshCRCacheLocked()
}

func (s *Store) refreshCRCacheLocked() error {
	paths, fingerprints, changed, err := s.scanCRFingerprints()
	if err != nil {
		return err
	}

	s.cacheMu.RLock()
	loaded := s.crCache.loaded
	s.cacheMu.RUnlock()
	if loaded && !changed {
		return nil
	}

	crs := make([]model.CR, 0, len(paths))
	byID := make(map[int]model.CR, len(paths))
	for _, path := range paths {
		var cr model.CR
		if err := s.readYAML(path, &cr); err != nil {
			return err
		}
		normalizeCRCollections(&cr)
		crs = append(crs, cr)
		byID[cr.ID] = cr
	}
	sort.Slice(crs, func(i, j int) bool {
		return crs[i].ID < crs[j].ID
	})

	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()
	s.crCache.loaded = true
	s.crCache.fingerprints = fingerprints
	s.crCache.crs = crs
	s.crCache.byID = byID
	return nil
}

func (s *Store) scanCRFingerprints() ([]string, map[string]crFileFingerprint, bool, error) {
	matches, err := filepath.Glob(filepath.Join(s.CRDir(), "*.yaml"))
	if err != nil {
		return nil, nil, false, fmt.Errorf("list cr files: %w", err)
	}
	sort.Strings(matches)

	fingerprints := make(map[string]crFileFingerprint, len(matches))
	for _, path := range matches {
		info, err := os.Stat(path)
		if err != nil {
			return nil, nil, false, fmt.Errorf("stat %s: %w", path, err)
		}
		fingerprints[path] = crFileFingerprint{
			Size:    info.Size(),
			ModTime: info.ModTime().UnixNano(),
		}
	}

	s.cacheMu.RLock()
	defer s.cacheMu.RUnlock()
	if !s.crCache.loaded {
		return matches, fingerprints, true, nil
	}
	if len(fingerprints) != len(s.crCache.fingerprints) {
		return matches, fingerprints, true, nil
	}
	for path, fp := range fingerprints {
		cached, ok := s.crCache.fingerprints[path]
		if !ok || cached != fp {
			return matches, fingerprints, true, nil
		}
	}
	return matches, fingerprints, false, nil
}

func (s *Store) invalidateCRCache() {
	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()
	s.crCache = crMetadataCache{}
}

func normalizeCRCollections(cr *model.CR) {
	if cr == nil {
		return
	}
	if cr.Notes == nil {
		cr.Notes = []string{}
	}
	if cr.Evidence == nil {
		cr.Evidence = []model.EvidenceEntry{}
	}
	if cr.Subtasks == nil {
		cr.Subtasks = []model.Subtask{}
	}
	if cr.Events == nil {
		cr.Events = []model.Event{}
	}
	if cr.DelegationRuns == nil {
		cr.DelegationRuns = []model.DelegationRun{}
	}
	if cr.PR.CheckpointCommentKeys == nil {
		cr.PR.CheckpointCommentKeys = []string{}
	}
	if cr.PR.CheckpointSyncKeys == nil {
		cr.PR.CheckpointSyncKeys = []string{}
	}
}

func cloneCRSlice(crs []model.CR) []model.CR {
	if len(crs) == 0 {
		return []model.CR{}
	}
	out := make([]model.CR, 0, len(crs))
	for _, cr := range crs {
		out = append(out, cloneCR(cr))
	}
	return out
}

func cloneCR(cr model.CR) model.CR {
	out := cr
	out.Notes = append([]string(nil), cr.Notes...)
	out.Evidence = cloneEvidenceEntries(cr.Evidence)
	out.DelegationRuns = cloneDelegationRuns(cr.DelegationRuns)
	out.Contract = cloneContract(cr.Contract)
	out.ContractBaseline = cloneCRContractBaseline(cr.ContractBaseline)
	out.ContractDrifts = cloneCRContractDrifts(cr.ContractDrifts)
	out.Subtasks = cloneSubtasks(cr.Subtasks)
	out.Events = cloneEvents(cr.Events)
	out.HQ = cloneCRHQState(cr.HQ)
	out.PR = cloneCRPRLink(cr.PR)
	return out
}

func cloneEvidenceEntries(entries []model.EvidenceEntry) []model.EvidenceEntry {
	if len(entries) == 0 {
		return []model.EvidenceEntry{}
	}
	out := make([]model.EvidenceEntry, 0, len(entries))
	for _, entry := range entries {
		copyEntry := entry
		copyEntry.Attachments = append([]string(nil), entry.Attachments...)
		if entry.ExitCode != nil {
			code := *entry.ExitCode
			copyEntry.ExitCode = &code
		}
		out = append(out, copyEntry)
	}
	return out
}

func cloneDelegationRuns(runs []model.DelegationRun) []model.DelegationRun {
	if len(runs) == 0 {
		return []model.DelegationRun{}
	}
	out := make([]model.DelegationRun, 0, len(runs))
	for _, run := range runs {
		copyRun := run
		copyRun.Request = cloneDelegationRequest(run.Request)
		copyRun.Events = cloneDelegationRunEvents(run.Events)
		if run.Result != nil {
			result := cloneDelegationResult(*run.Result)
			copyRun.Result = &result
		}
		out = append(out, copyRun)
	}
	return out
}

func cloneDelegationRequest(request model.DelegationRequest) model.DelegationRequest {
	out := request
	out.TaskIDs = append([]int(nil), request.TaskIDs...)
	out.SkillRefs = append([]string(nil), request.SkillRefs...)
	if len(request.Metadata) > 0 {
		out.Metadata = make(map[string]string, len(request.Metadata))
		for k, v := range request.Metadata {
			out.Metadata[k] = v
		}
	}
	if request.IntentSnapshot != nil {
		snapshot := cloneHQIntentSnapshot(*request.IntentSnapshot)
		out.IntentSnapshot = &snapshot
	}
	return out
}

func cloneHQIntentSnapshot(snapshot model.HQIntentSnapshot) model.HQIntentSnapshot {
	out := snapshot
	out.Contract = cloneHQIntentContractSnapshot(snapshot.Contract)
	out.Notes = append([]string(nil), snapshot.Notes...)
	if len(snapshot.Subtasks) > 0 {
		out.Subtasks = make([]model.HQIntentTaskSnapshot, 0, len(snapshot.Subtasks))
		for _, task := range snapshot.Subtasks {
			copyTask := task
			copyTask.Contract = cloneHQIntentTaskContractSnapshot(task.Contract)
			out.Subtasks = append(out.Subtasks, copyTask)
		}
	}
	return out
}

func cloneHQIntentContractSnapshot(contract model.HQIntentContractSnapshot) model.HQIntentContractSnapshot {
	out := contract
	out.Scope = append([]string(nil), contract.Scope...)
	out.NonGoals = append([]string(nil), contract.NonGoals...)
	out.Invariants = append([]string(nil), contract.Invariants...)
	out.RiskCriticalScopes = append([]string(nil), contract.RiskCriticalScopes...)
	return out
}

func cloneHQIntentTaskContractSnapshot(contract model.HQIntentTaskContractSnapshot) model.HQIntentTaskContractSnapshot {
	out := contract
	out.AcceptanceCriteria = append([]string(nil), contract.AcceptanceCriteria...)
	out.Scope = append([]string(nil), contract.Scope...)
	out.AcceptanceChecks = append([]string(nil), contract.AcceptanceChecks...)
	return out
}

func cloneDelegationRunEvents(events []model.DelegationRunEvent) []model.DelegationRunEvent {
	if len(events) == 0 {
		return []model.DelegationRunEvent{}
	}
	out := make([]model.DelegationRunEvent, 0, len(events))
	for _, event := range events {
		copyEvent := event
		if len(event.Meta) > 0 {
			copyEvent.Meta = make(map[string]string, len(event.Meta))
			for k, v := range event.Meta {
				copyEvent.Meta[k] = v
			}
		}
		out = append(out, copyEvent)
	}
	return out
}

func cloneDelegationResult(result model.DelegationResult) model.DelegationResult {
	out := result
	out.FilesChanged = append([]string(nil), result.FilesChanged...)
	out.ValidationErrors = append([]string(nil), result.ValidationErrors...)
	out.ValidationWarnings = append([]string(nil), result.ValidationWarnings...)
	out.Blockers = append([]string(nil), result.Blockers...)
	if len(result.Metadata) > 0 {
		out.Metadata = make(map[string]string, len(result.Metadata))
		for k, v := range result.Metadata {
			out.Metadata[k] = v
		}
	}
	return out
}

func cloneContract(contract model.Contract) model.Contract {
	out := contract
	out.Scope = append([]string(nil), contract.Scope...)
	out.NonGoals = append([]string(nil), contract.NonGoals...)
	out.Invariants = append([]string(nil), contract.Invariants...)
	out.RiskCriticalScopes = append([]string(nil), contract.RiskCriticalScopes...)
	return out
}

func cloneCRContractBaseline(baseline model.CRContractBaseline) model.CRContractBaseline {
	out := baseline
	out.Scope = append([]string(nil), baseline.Scope...)
	return out
}

func cloneCRContractDrifts(drifts []model.CRContractDrift) []model.CRContractDrift {
	if len(drifts) == 0 {
		return []model.CRContractDrift{}
	}
	out := make([]model.CRContractDrift, 0, len(drifts))
	for _, drift := range drifts {
		copyDrift := drift
		copyDrift.Fields = append([]string(nil), drift.Fields...)
		copyDrift.BeforeScope = append([]string(nil), drift.BeforeScope...)
		copyDrift.AfterScope = append([]string(nil), drift.AfterScope...)
		out = append(out, copyDrift)
	}
	return out
}

func cloneSubtasks(tasks []model.Subtask) []model.Subtask {
	if len(tasks) == 0 {
		return []model.Subtask{}
	}
	out := make([]model.Subtask, 0, len(tasks))
	for _, task := range tasks {
		copyTask := task
		copyTask.CheckpointScope = append([]string(nil), task.CheckpointScope...)
		copyTask.CheckpointChunks = append([]model.CheckpointChunk(nil), task.CheckpointChunks...)
		copyTask.Delegations = append([]model.TaskDelegation(nil), task.Delegations...)
		copyTask.Contract = cloneTaskContract(task.Contract)
		copyTask.ContractBaseline = cloneTaskContractBaseline(task.ContractBaseline)
		copyTask.ContractDrifts = cloneTaskContractDrifts(task.ContractDrifts)
		out = append(out, copyTask)
	}
	return out
}

func cloneTaskContract(contract model.TaskContract) model.TaskContract {
	out := contract
	out.AcceptanceCriteria = append([]string(nil), contract.AcceptanceCriteria...)
	out.Scope = append([]string(nil), contract.Scope...)
	out.AcceptanceChecks = append([]string(nil), contract.AcceptanceChecks...)
	return out
}

func cloneTaskContractBaseline(baseline model.TaskContractBaseline) model.TaskContractBaseline {
	out := baseline
	out.AcceptanceCriteria = append([]string(nil), baseline.AcceptanceCriteria...)
	out.Scope = append([]string(nil), baseline.Scope...)
	out.AcceptanceChecks = append([]string(nil), baseline.AcceptanceChecks...)
	return out
}

func cloneTaskContractDrifts(drifts []model.TaskContractDrift) []model.TaskContractDrift {
	if len(drifts) == 0 {
		return []model.TaskContractDrift{}
	}
	out := make([]model.TaskContractDrift, 0, len(drifts))
	for _, drift := range drifts {
		copyDrift := drift
		copyDrift.Fields = append([]string(nil), drift.Fields...)
		copyDrift.BeforeScope = append([]string(nil), drift.BeforeScope...)
		copyDrift.AfterScope = append([]string(nil), drift.AfterScope...)
		copyDrift.BeforeAcceptanceChecks = append([]string(nil), drift.BeforeAcceptanceChecks...)
		copyDrift.AfterAcceptanceChecks = append([]string(nil), drift.AfterAcceptanceChecks...)
		out = append(out, copyDrift)
	}
	return out
}

func cloneEvents(events []model.Event) []model.Event {
	if len(events) == 0 {
		return []model.Event{}
	}
	out := make([]model.Event, 0, len(events))
	for _, event := range events {
		copyEvent := event
		if len(event.Meta) > 0 {
			copyEvent.Meta = make(map[string]string, len(event.Meta))
			for k, v := range event.Meta {
				copyEvent.Meta[k] = v
			}
		}
		out = append(out, copyEvent)
	}
	return out
}

func cloneCRHQState(state model.CRHQState) model.CRHQState {
	out := state
	if state.UpstreamIntent != nil {
		intent := cloneHQIntentSnapshot(*state.UpstreamIntent)
		out.UpstreamIntent = &intent
	}
	return out
}

func cloneCRPRLink(link model.CRPRLink) model.CRPRLink {
	out := link
	out.CheckpointCommentKeys = append([]string(nil), link.CheckpointCommentKeys...)
	out.CheckpointSyncKeys = append([]string(nil), link.CheckpointSyncKeys...)
	return out
}
