package service

import (
	"fmt"
	"sophia/internal/model"
	"strings"
)

const delegationRuntimeNameMock = "mock"

type DelegationRuntime interface {
	Start(DelegationRuntimeRunContext, DelegationRuntimeReporter) error
	Cancel(DelegationRuntimeRunContext, string) error
}

type DelegationRuntimeRunContext struct {
	CRID int
	Run  model.DelegationRun
}

type DelegationRuntimeProgress struct {
	Kind    string
	Summary string
	Message string
	Step    string
	Meta    map[string]string
}

type DelegationRuntimeReporter interface {
	Event(DelegationRuntimeProgress) error
	Finish(model.DelegationResult) error
}

type delegationRuntimeReporter struct {
	service  *Service
	crID     int
	runID    string
	finished bool
}

type mockDelegationRuntime struct{}

type mockDelegationPlan struct {
	outcome      string
	planSummary  string
	steps        []string
	message      string
	filesChanged []string
	blockers     []string
}

func (s *Service) StartDelegation(crID int, request model.DelegationRequest) (*model.DelegationRun, error) {
	runtime, err := s.activeDelegationRuntime(request.Runtime)
	if err != nil {
		return nil, err
	}
	run, err := s.CreateDelegationRun(crID, request)
	if err != nil {
		return nil, err
	}
	reporter := &delegationRuntimeReporter{
		service: s,
		crID:    crID,
		runID:   run.ID,
	}
	ctx := DelegationRuntimeRunContext{
		CRID: crID,
		Run:  cloneDelegationRun(*run),
	}
	if err := runtime.Start(ctx, reporter); err != nil {
		if !reporter.finished {
			failure := model.DelegationResult{
				Status:  model.DelegationRunStatusFailed,
				Summary: fmt.Sprintf("runtime %q failed to start: %v", run.Request.Runtime, err),
				Metadata: map[string]string{
					"runtime": run.Request.Runtime,
				},
			}
			if _, finishErr := s.FinishDelegationRun(crID, run.ID, failure); finishErr != nil {
				return nil, fmt.Errorf("start delegation runtime: %w (mark failed: %v)", err, finishErr)
			}
		}
		return nil, err
	}
	return s.GetDelegationRun(crID, run.ID)
}

func (s *Service) CancelDelegation(crID int, runID, reason string) (*model.DelegationRun, error) {
	run, err := s.GetDelegationRun(crID, runID)
	if err != nil {
		return nil, err
	}
	if isDelegationRunTerminalStatus(run.Status) {
		return nil, fmt.Errorf("delegation run %q is already terminal (%s)", runID, run.Status)
	}
	runtime, err := s.activeDelegationRuntime(run.Request.Runtime)
	if err != nil {
		return nil, err
	}
	trimmedReason := strings.TrimSpace(reason)
	if err := runtime.Cancel(DelegationRuntimeRunContext{CRID: crID, Run: cloneDelegationRun(*run)}, trimmedReason); err != nil {
		return nil, err
	}
	result := model.DelegationResult{
		Status:  model.DelegationRunStatusCancelled,
		Summary: "delegation cancelled",
	}
	if trimmedReason != "" {
		result.Summary = trimmedReason
		result.Metadata = map[string]string{"reason": trimmedReason}
	}
	return s.FinishDelegationRun(crID, runID, result)
}

func (s *Service) activeDelegationRuntime(name string) (DelegationRuntime, error) {
	key := strings.TrimSpace(name)
	if key == "" {
		return nil, fmt.Errorf("delegation runtime is required")
	}
	if !s.delegationRuntimesCustom || s.delegationRuntimes == nil {
		s.delegationRuntimes = defaultDelegationRuntimes()
	}
	runtime, ok := s.delegationRuntimes[key]
	if !ok || runtime == nil {
		return nil, fmt.Errorf("delegation runtime %q is not registered", key)
	}
	return runtime, nil
}

func (s *Service) overrideDelegationRuntimesForTests(runtimes map[string]DelegationRuntime) {
	s.delegationRuntimes = cloneDelegationRuntimeMap(runtimes)
	s.delegationRuntimesCustom = runtimes != nil
}

func (r *delegationRuntimeReporter) Event(progress DelegationRuntimeProgress) error {
	if r.finished {
		return fmt.Errorf("delegation run %q is already finished", r.runID)
	}
	_, err := r.service.AppendDelegationRunEvent(r.crID, r.runID, model.DelegationRunEvent{
		Kind:    strings.TrimSpace(progress.Kind),
		Summary: strings.TrimSpace(progress.Summary),
		Message: strings.TrimSpace(progress.Message),
		Step:    strings.TrimSpace(progress.Step),
		Meta:    cloneStringMap(progress.Meta),
	})
	return err
}

func (r *delegationRuntimeReporter) Finish(result model.DelegationResult) error {
	if r.finished {
		return fmt.Errorf("delegation run %q is already finished", r.runID)
	}
	r.finished = true
	_, err := r.service.FinishDelegationRun(r.crID, r.runID, result)
	return err
}

func defaultDelegationRuntimes() map[string]DelegationRuntime {
	return map[string]DelegationRuntime{
		delegationRuntimeNameMock: mockDelegationRuntime{},
	}
}

func cloneDelegationRuntimeMap(src map[string]DelegationRuntime) map[string]DelegationRuntime {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]DelegationRuntime, len(src))
	for key, runtime := range src {
		trimmed := strings.TrimSpace(key)
		if trimmed == "" || runtime == nil {
			continue
		}
		dst[trimmed] = runtime
	}
	return dst
}

func (mockDelegationRuntime) Start(ctx DelegationRuntimeRunContext, reporter DelegationRuntimeReporter) error {
	plan := buildMockDelegationPlan(ctx.Run.Request)
	if err := reporter.Event(DelegationRuntimeProgress{
		Kind:    model.DelegationEventKindRunStarted,
		Summary: "mock runtime started",
	}); err != nil {
		return err
	}
	if err := reporter.Event(DelegationRuntimeProgress{
		Kind:    model.DelegationEventKindPlanUpdated,
		Summary: plan.planSummary,
		Step:    firstMockDelegationStep(plan.steps),
	}); err != nil {
		return err
	}
	for _, step := range plan.steps {
		if err := reporter.Event(DelegationRuntimeProgress{
			Kind:    model.DelegationEventKindStepStarted,
			Summary: fmt.Sprintf("starting %s", step),
			Step:    step,
		}); err != nil {
			return err
		}
		if plan.message != "" {
			if err := reporter.Event(DelegationRuntimeProgress{
				Kind:    model.DelegationEventKindMessage,
				Summary: plan.message,
				Message: plan.message,
				Step:    step,
			}); err != nil {
				return err
			}
		}
		if err := reporter.Event(DelegationRuntimeProgress{
			Kind:    model.DelegationEventKindStepCompleted,
			Summary: fmt.Sprintf("completed %s", step),
			Step:    step,
		}); err != nil {
			return err
		}
	}
	switch plan.outcome {
	case model.DelegationRunStatusCompleted:
		for _, path := range plan.filesChanged {
			if err := reporter.Event(DelegationRuntimeProgress{
				Kind:    model.DelegationEventKindFileChanged,
				Summary: fmt.Sprintf("changed %s", path),
				Meta:    map[string]string{"path": path},
			}); err != nil {
				return err
			}
		}
		if err := reporter.Event(DelegationRuntimeProgress{
			Kind:    model.DelegationEventKindRunCompleted,
			Summary: "mock runtime completed",
		}); err != nil {
			return err
		}
		return reporter.Finish(model.DelegationResult{
			Status:       model.DelegationRunStatusCompleted,
			Summary:      "mock runtime completed successfully",
			FilesChanged: append([]string(nil), plan.filesChanged...),
			Metadata: map[string]string{
				"runtime": delegationRuntimeNameMock,
			},
		})
	case model.DelegationRunStatusBlocked:
		blockerSummary := "mock runtime blocked"
		if len(plan.blockers) > 0 {
			blockerSummary = plan.blockers[0]
		}
		if err := reporter.Event(DelegationRuntimeProgress{
			Kind:    model.DelegationEventKindBlocked,
			Summary: blockerSummary,
			Message: blockerSummary,
		}); err != nil {
			return err
		}
		return reporter.Finish(model.DelegationResult{
			Status:   model.DelegationRunStatusBlocked,
			Summary:  blockerSummary,
			Blockers: append([]string(nil), plan.blockers...),
			Metadata: map[string]string{
				"runtime": delegationRuntimeNameMock,
			},
		})
	default:
		if err := reporter.Event(DelegationRuntimeProgress{
			Kind:    model.DelegationEventKindRunFailed,
			Summary: "mock runtime failed",
			Message: "mock runtime failed",
		}); err != nil {
			return err
		}
		return reporter.Finish(model.DelegationResult{
			Status:  model.DelegationRunStatusFailed,
			Summary: "mock runtime failed",
			Metadata: map[string]string{
				"runtime": delegationRuntimeNameMock,
			},
		})
	}
}

func (mockDelegationRuntime) Cancel(DelegationRuntimeRunContext, string) error {
	return nil
}

func buildMockDelegationPlan(request model.DelegationRequest) mockDelegationPlan {
	metadata := normalizeDelegationStringMap(request.Metadata)
	outcome := strings.ToLower(strings.TrimSpace(metadata["mock_outcome"]))
	switch outcome {
	case model.DelegationRunStatusBlocked, model.DelegationRunStatusFailed:
	default:
		outcome = model.DelegationRunStatusCompleted
	}
	planSummary := strings.TrimSpace(metadata["mock_plan"])
	if planSummary == "" {
		planSummary = "mock runtime plan prepared"
	}
	message := strings.TrimSpace(metadata["mock_message"])
	if message == "" {
		message = "mock runtime executing delegation steps"
	}
	steps := parseMockDelegationCSV(metadata["mock_steps"])
	if len(steps) == 0 {
		steps = []string{"hydrate context", "apply deterministic mock work"}
	}
	filesChanged := parseMockDelegationCSV(metadata["mock_files_changed"])
	if len(filesChanged) == 0 {
		filesChanged = []string{"internal/service/service_delegation_runtime.go"}
	}
	blockers := parseMockDelegationCSV(metadata["mock_blockers"])
	if outcome == model.DelegationRunStatusBlocked && len(blockers) == 0 {
		blockers = []string{"mock runtime reported a blocker"}
	}
	return mockDelegationPlan{
		outcome:      outcome,
		planSummary:  planSummary,
		steps:        steps,
		message:      message,
		filesChanged: filesChanged,
		blockers:     blockers,
	}
}

func parseMockDelegationCSV(input string) []string {
	if strings.TrimSpace(input) == "" {
		return nil
	}
	return normalizeStringList(strings.Split(input, ","))
}

func firstMockDelegationStep(steps []string) string {
	if len(steps) == 0 {
		return ""
	}
	return steps[0]
}
