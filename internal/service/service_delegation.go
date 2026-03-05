package service

import (
	"crypto/rand"
	"fmt"
	"sophia/internal/model"
	"sort"
	"strings"
)

func (s *Service) CreateDelegationRun(crID int, request model.DelegationRequest) (*model.DelegationRun, error) {
	var created model.DelegationRun
	if err := s.withMutationLock(func() error {
		cr, err := s.loadCRForMutation(crID)
		if err != nil {
			return err
		}
		normalized, err := normalizeDelegationRequest(cr, request)
		if err != nil {
			return err
		}
		now := s.timestamp()
		actor := s.activeLifecycleGitProvider().Actor()
		runID, err := newDelegationRunID()
		if err != nil {
			return err
		}
		run := model.DelegationRun{
			ID:        runID,
			Status:    model.DelegationRunStatusQueued,
			Request:   normalized,
			Events:    []model.DelegationRunEvent{},
			CreatedAt: now,
			CreatedBy: actor,
			UpdatedAt: now,
		}
		cr.DelegationRuns = append(cr.DelegationRuns, run)
		cr.UpdatedAt = now
		if err := s.activeLifecycleStoreProvider().SaveCR(cr); err != nil {
			return err
		}
		created = cloneDelegationRun(run)
		return nil
	}); err != nil {
		return nil, err
	}
	return &created, nil
}

func (s *Service) AppendDelegationRunEvent(crID int, runID string, event model.DelegationRunEvent) (*model.DelegationRun, error) {
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return nil, fmt.Errorf("delegation run id is required")
	}

	var updated model.DelegationRun
	if err := s.withMutationLock(func() error {
		cr, err := s.loadCRForMutation(crID)
		if err != nil {
			return err
		}
		run, err := findDelegationRunForMutation(cr, runID)
		if err != nil {
			return err
		}
		if isDelegationRunTerminalStatus(run.Status) {
			return fmt.Errorf("delegation run %q is already terminal (%s)", runID, run.Status)
		}
		normalized, err := normalizeDelegationRunEvent(event, len(run.Events)+1, s.timestamp())
		if err != nil {
			return err
		}
		if err := validateDelegationEventTransition(run.Status, normalized.Kind); err != nil {
			return err
		}
		run.Events = append(run.Events, normalized)
		run.UpdatedAt = normalized.TS
		if run.Status == model.DelegationRunStatusQueued && normalized.Kind == model.DelegationEventKindRunStarted {
			run.Status = model.DelegationRunStatusRunning
		}
		cr.UpdatedAt = run.UpdatedAt
		if err := s.activeLifecycleStoreProvider().SaveCR(cr); err != nil {
			return err
		}
		updated = cloneDelegationRun(*run)
		return nil
	}); err != nil {
		return nil, err
	}
	return &updated, nil
}

func (s *Service) FinishDelegationRun(crID int, runID string, result model.DelegationResult) (*model.DelegationRun, error) {
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return nil, fmt.Errorf("delegation run id is required")
	}

	var finished model.DelegationRun
	if err := s.withMutationLock(func() error {
		cr, err := s.loadCRForMutation(crID)
		if err != nil {
			return err
		}
		run, err := findDelegationRunForMutation(cr, runID)
		if err != nil {
			return err
		}
		if isDelegationRunTerminalStatus(run.Status) {
			return fmt.Errorf("delegation run %q is already terminal (%s)", runID, run.Status)
		}
		normalized, err := normalizeDelegationResult(result)
		if err != nil {
			return err
		}
		if err := validateDelegationFinishTransition(run.Status, normalized.Status); err != nil {
			return err
		}
		now := s.timestamp()
		run.Status = normalized.Status
		run.Result = &normalized
		run.UpdatedAt = now
		run.FinishedAt = now
		cr.UpdatedAt = now
		if err := s.activeLifecycleStoreProvider().SaveCR(cr); err != nil {
			return err
		}
		finished = cloneDelegationRun(*run)
		return nil
	}); err != nil {
		return nil, err
	}
	return &finished, nil
}

func (s *Service) GetDelegationRun(crID int, runID string) (*model.DelegationRun, error) {
	cr, err := s.activeLifecycleStoreProvider().LoadCR(crID)
	if err != nil {
		return nil, err
	}
	runID = strings.TrimSpace(runID)
	for _, run := range cr.DelegationRuns {
		if strings.TrimSpace(run.ID) == runID {
			cloned := cloneDelegationRun(run)
			return &cloned, nil
		}
	}
	return nil, fmt.Errorf("delegation run %q not found in cr %d", runID, crID)
}

func (s *Service) ListDelegationRuns(crID int) ([]model.DelegationRun, error) {
	cr, err := s.activeLifecycleStoreProvider().LoadCR(crID)
	if err != nil {
		return nil, err
	}
	runs := make([]model.DelegationRun, 0, len(cr.DelegationRuns))
	for _, run := range cr.DelegationRuns {
		runs = append(runs, cloneDelegationRun(run))
	}
	sort.SliceStable(runs, func(i, j int) bool {
		if runs[i].CreatedAt == runs[j].CreatedAt {
			return runs[i].ID < runs[j].ID
		}
		return runs[i].CreatedAt < runs[j].CreatedAt
	})
	return runs, nil
}

func normalizeDelegationRequest(cr *model.CR, request model.DelegationRequest) (model.DelegationRequest, error) {
	taskIDs, err := normalizeDelegationTaskIDs(request.TaskIDs)
	if err != nil {
		return model.DelegationRequest{}, err
	}
	normalized := model.DelegationRequest{
		Runtime:              strings.TrimSpace(request.Runtime),
		TaskIDs:              taskIDs,
		WorkflowInstructions: strings.TrimSpace(request.WorkflowInstructions),
		SkillRefs:            normalizeStringList(request.SkillRefs),
		Metadata:             normalizeDelegationStringMap(request.Metadata),
	}
	if normalized.Runtime == "" {
		return model.DelegationRequest{}, fmt.Errorf("delegation runtime is required")
	}
	if err := validateDelegationTaskIDs(cr, normalized.TaskIDs); err != nil {
		return model.DelegationRequest{}, err
	}
	if request.IntentSnapshot != nil {
		snapshot := cloneHQIntentSnapshot(request.IntentSnapshot)
		normalized.IntentSnapshot = &snapshot
	} else if cr != nil {
		normalized.IntentSnapshot = canonicalHQIntentSnapshot(cr)
	}
	return normalized, nil
}

func normalizeDelegationTaskIDs(taskIDs []int) ([]int, error) {
	if len(taskIDs) == 0 {
		return nil, nil
	}
	seen := map[int]struct{}{}
	normalized := make([]int, 0, len(taskIDs))
	for _, id := range taskIDs {
		if id <= 0 {
			return nil, fmt.Errorf("delegation task ids must be positive (got %d)", id)
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		normalized = append(normalized, id)
	}
	sort.Ints(normalized)
	return normalized, nil
}

func validateDelegationTaskIDs(cr *model.CR, taskIDs []int) error {
	if len(taskIDs) == 0 || cr == nil {
		return nil
	}
	taskSet := make(map[int]struct{}, len(cr.Subtasks))
	for _, task := range cr.Subtasks {
		taskSet[task.ID] = struct{}{}
	}
	for _, taskID := range taskIDs {
		if _, ok := taskSet[taskID]; !ok {
			return fmt.Errorf("task %d not found in cr %d", taskID, cr.ID)
		}
	}
	return nil
}

func normalizeDelegationRunEvent(event model.DelegationRunEvent, nextID int, now string) (model.DelegationRunEvent, error) {
	normalized := model.DelegationRunEvent{
		ID:      event.ID,
		TS:      strings.TrimSpace(event.TS),
		Kind:    strings.TrimSpace(event.Kind),
		Summary: strings.TrimSpace(event.Summary),
		Message: strings.TrimSpace(event.Message),
		Step:    strings.TrimSpace(event.Step),
		Meta:    normalizeDelegationStringMap(event.Meta),
	}
	if normalized.ID <= 0 {
		normalized.ID = nextID
	}
	if normalized.TS == "" {
		normalized.TS = now
	}
	if !isValidDelegationEventKind(normalized.Kind) {
		return model.DelegationRunEvent{}, fmt.Errorf("invalid delegation event kind %q", event.Kind)
	}
	if normalized.Summary == "" && normalized.Message == "" && normalized.Step == "" {
		return model.DelegationRunEvent{}, fmt.Errorf("delegation event %q must include summary, message, or step", normalized.Kind)
	}
	return normalized, nil
}

func normalizeDelegationResult(result model.DelegationResult) (model.DelegationResult, error) {
	normalized := model.DelegationResult{
		Status:             strings.TrimSpace(result.Status),
		Summary:            strings.TrimSpace(result.Summary),
		FilesChanged:       normalizeStringList(result.FilesChanged),
		ValidationErrors:   normalizeStringList(result.ValidationErrors),
		ValidationWarnings: normalizeStringList(result.ValidationWarnings),
		Blockers:           normalizeStringList(result.Blockers),
		Metadata:           normalizeDelegationStringMap(result.Metadata),
	}
	if !isDelegationRunTerminalStatus(normalized.Status) {
		return model.DelegationResult{}, fmt.Errorf("delegation result status %q must be terminal", result.Status)
	}
	if normalized.Summary == "" {
		normalized.Summary = normalized.Status
	}
	return normalized, nil
}

func normalizeDelegationStringMap(input map[string]string) map[string]string {
	if len(input) == 0 {
		return nil
	}
	keys := make([]string, 0, len(input))
	normalized := make(map[string]string, len(input))
	for key, value := range input {
		k := strings.TrimSpace(key)
		if k == "" {
			continue
		}
		keys = append(keys, k)
		normalized[k] = strings.TrimSpace(value)
	}
	sort.Strings(keys)
	ordered := make(map[string]string, len(keys))
	for _, key := range keys {
		ordered[key] = normalized[key]
	}
	return ordered
}

func isValidDelegationEventKind(kind string) bool {
	switch strings.TrimSpace(kind) {
	case model.DelegationEventKindRunStarted,
		model.DelegationEventKindPlanUpdated,
		model.DelegationEventKindStepStarted,
		model.DelegationEventKindStepCompleted,
		model.DelegationEventKindMessage,
		model.DelegationEventKindCommandStarted,
		model.DelegationEventKindCommandComplete,
		model.DelegationEventKindFileChanged,
		model.DelegationEventKindNeedsInput,
		model.DelegationEventKindBlocked,
		model.DelegationEventKindRunCompleted,
		model.DelegationEventKindRunFailed:
		return true
	default:
		return false
	}
}

func validateDelegationEventTransition(status, kind string) error {
	status = strings.TrimSpace(status)
	kind = strings.TrimSpace(kind)
	switch status {
	case model.DelegationRunStatusQueued:
		if kind != model.DelegationEventKindRunStarted {
			return fmt.Errorf("delegation run must start before recording %q while queued", kind)
		}
		return nil
	case model.DelegationRunStatusRunning:
		switch kind {
		case model.DelegationEventKindRunStarted:
			return fmt.Errorf("delegation run in progress cannot record event %q", kind)
		default:
			return nil
		}
	default:
		return fmt.Errorf("delegation run status %q cannot accept additional events", status)
	}
}

func validateDelegationFinishTransition(currentStatus, finalStatus string) error {
	currentStatus = strings.TrimSpace(currentStatus)
	finalStatus = strings.TrimSpace(finalStatus)
	switch currentStatus {
	case model.DelegationRunStatusQueued:
		if finalStatus == model.DelegationRunStatusCancelled {
			return nil
		}
		return fmt.Errorf("delegation run must start before finishing as %q", finalStatus)
	case model.DelegationRunStatusRunning:
		return nil
	default:
		return fmt.Errorf("delegation run status %q cannot finish as %q", currentStatus, finalStatus)
	}
}

func isDelegationRunTerminalStatus(status string) bool {
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

func findDelegationRunForMutation(cr *model.CR, runID string) (*model.DelegationRun, error) {
	for i := range cr.DelegationRuns {
		if strings.TrimSpace(cr.DelegationRuns[i].ID) == runID {
			return &cr.DelegationRuns[i], nil
		}
	}
	return nil, fmt.Errorf("delegation run %q not found in cr %d", runID, cr.ID)
}

func cloneDelegationRun(run model.DelegationRun) model.DelegationRun {
	cloned := run
	cloned.Request.TaskIDs = append([]int(nil), run.Request.TaskIDs...)
	cloned.Request.SkillRefs = normalizeStringList(run.Request.SkillRefs)
	cloned.Request.Metadata = cloneStringMap(run.Request.Metadata)
	if run.Request.IntentSnapshot != nil {
		snapshot := cloneHQIntentSnapshot(run.Request.IntentSnapshot)
		cloned.Request.IntentSnapshot = &snapshot
	}
	cloned.Events = make([]model.DelegationRunEvent, 0, len(run.Events))
	for _, event := range run.Events {
		eventCopy := event
		eventCopy.Meta = cloneStringMap(event.Meta)
		cloned.Events = append(cloned.Events, eventCopy)
	}
	if run.Result != nil {
		resultCopy := *run.Result
		resultCopy.FilesChanged = normalizeStringList(run.Result.FilesChanged)
		resultCopy.ValidationErrors = normalizeStringList(run.Result.ValidationErrors)
		resultCopy.ValidationWarnings = normalizeStringList(run.Result.ValidationWarnings)
		resultCopy.Blockers = normalizeStringList(run.Result.Blockers)
		resultCopy.Metadata = cloneStringMap(run.Result.Metadata)
		cloned.Result = &resultCopy
	}
	return cloned
}

func cloneHQIntentSnapshot(snapshot *model.HQIntentSnapshot) model.HQIntentSnapshot {
	if snapshot == nil {
		return model.HQIntentSnapshot{}
	}
	cloned := *snapshot
	cloned.Contract.Scope = normalizeStringList(snapshot.Contract.Scope)
	cloned.Contract.NonGoals = normalizeStringList(snapshot.Contract.NonGoals)
	cloned.Contract.Invariants = normalizeStringList(snapshot.Contract.Invariants)
	cloned.Contract.RiskCriticalScopes = normalizeStringList(snapshot.Contract.RiskCriticalScopes)
	cloned.Notes = normalizeStringList(snapshot.Notes)
	cloned.Subtasks = make([]model.HQIntentTaskSnapshot, 0, len(snapshot.Subtasks))
	for _, task := range snapshot.Subtasks {
		taskCopy := task
		taskCopy.Contract.AcceptanceCriteria = normalizeStringList(task.Contract.AcceptanceCriteria)
		taskCopy.Contract.Scope = normalizeStringList(task.Contract.Scope)
		taskCopy.Contract.AcceptanceChecks = normalizeStringList(task.Contract.AcceptanceChecks)
		cloned.Subtasks = append(cloned.Subtasks, taskCopy)
	}
	sort.Slice(cloned.Subtasks, func(i, j int) bool {
		if cloned.Subtasks[i].ID == cloned.Subtasks[j].ID {
			return strings.Compare(cloned.Subtasks[i].Title, cloned.Subtasks[j].Title) < 0
		}
		return cloned.Subtasks[i].ID < cloned.Subtasks[j].ID
	})
	return cloned
}

func newDelegationRunID() (string, error) {
	var raw [8]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", fmt.Errorf("generate delegation run id: %w", err)
	}
	return fmt.Sprintf("dr_%x", raw[:]), nil
}
