package cli

import (
	"bytes"
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/spf13/cobra"

	"sophia/internal/model"
	"sophia/internal/service"
)

type crShowMode string

const (
	crShowModePerCR                   crShowMode = "per_cr"
	crShowModeDashboard               crShowMode = "dashboard"
	defaultCRListLimit                           = 200
	defaultCRTimelineLimit                       = 200
	defaultCRShowEventsLimit                     = 20
	defaultCRShowCheckpointsLimit                = 10
	defaultCRShowSSEPollInterval                 = 2 * time.Second
	defaultCRShowSSEKeepaliveInterval            = 15 * time.Second
	defaultCRShowInitialRenderWait               = 2 * time.Second
	defaultCRShowDelegationRuntime               = "mock"
)

type crShowBrowserLaunchState string

const (
	crShowBrowserLaunchNotAttempted crShowBrowserLaunchState = "not_attempted"
	crShowBrowserLaunchStarted      crShowBrowserLaunchState = "started"
	crShowBrowserLaunchFailed       crShowBrowserLaunchState = "failed"
)

type crShowFirstRenderState string

const (
	crShowFirstRenderNotAwaited crShowFirstRenderState = "not_awaited"
	crShowFirstRenderObserved   crShowFirstRenderState = "observed"
	crShowFirstRenderTimedOut   crShowFirstRenderState = "timed_out"
)

type crShowPreviewSession struct {
	server           *crShowServer
	viewURL          string
	templateSource   string
	launchState      crShowBrowserLaunchState
	firstRenderState crShowFirstRenderState
	openErr          string
	closeReason      string
}

func newCRShowCmd() *cobra.Command {
	var (
		asJSON           bool
		noOpen           bool
		eventsLimit      int
		checkpointsLimit int
		forceDashboard   bool
		statusFilter     string
		scopeFilter      string
		riskTierFilter   string
		textFilter       string
		searchFilter     string
		listLimit        int
		timelineLimit    int
	)

	cmd := &cobra.Command{
		Use:   "show [id]",
		Short: "Render a read-only CR report and open it in a localhost browser view",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, err := newServiceForCmd(cmd)
			if err != nil {
				return commandError(cmd, asJSON, err)
			}

			mode, id, selectedHint, err := resolveCRShowMode(svc, args, forceDashboard)
			if err != nil {
				return commandError(cmd, asJSON, err)
			}

			if mode == crShowModePerCR {
				return runCRShowPerCR(cmd, asJSON, noOpen, svc, id, eventsLimit, checkpointsLimit)
			}

			query, err := resolveCRSearchQuery(crSearchCommandFilters{
				status:   statusFilter,
				scope:    scopeFilter,
				riskTier: riskTierFilter,
				text:     textFilter,
				search:   searchFilter,
			})
			if err != nil {
				return commandError(cmd, asJSON, err)
			}
			return runCRShowDashboard(cmd, asJSON, noOpen, svc, query, listLimit, timelineLimit, eventsLimit, checkpointsLimit, selectedHint)
		},
	}

	cmd.Flags().BoolVar(&asJSON, "json", false, "Output in JSON format")
	cmd.Flags().BoolVar(&noOpen, "no-open", false, "Render report without opening a browser tab")
	cmd.Flags().IntVar(&eventsLimit, "events-limit", defaultCRShowEventsLimit, "Maximum recent CR events to include")
	cmd.Flags().IntVar(&checkpointsLimit, "checkpoints-limit", defaultCRShowCheckpointsLimit, "Maximum recent task checkpoints to include")
	cmd.Flags().BoolVar(&forceDashboard, "dashboard", false, "Force dashboard mode instead of per-CR mode")
	cmd.Flags().StringVar(&statusFilter, "status", "", "Dashboard filter by status (in_progress, merged, abandoned)")
	cmd.Flags().StringVar(&scopeFilter, "scope", "", "Dashboard filter by contract scope prefix")
	cmd.Flags().StringVar(&riskTierFilter, "risk-tier", "", "Dashboard filter by risk tier (low, medium, high)")
	cmd.Flags().StringVar(&textFilter, "text", "", "Dashboard text search in title/description/notes/contract")
	cmd.Flags().StringVar(&searchFilter, "search", "", "Alias for --text in dashboard mode")
	cmd.Flags().IntVar(&listLimit, "list-limit", defaultCRListLimit, "Maximum dashboard CR rows to include")
	cmd.Flags().IntVar(&timelineLimit, "timeline-limit", defaultCRTimelineLimit, "Maximum dashboard timeline events to include")
	return cmd
}

func resolveCRShowMode(svc *service.Service, args []string, forceDashboard bool) (crShowMode, int, int, error) {
	if svc == nil {
		return "", 0, 0, fmt.Errorf("service is required")
	}
	if len(args) > 0 {
		id, err := resolveCRIDFromSelector(svc, args[0], "id")
		if err != nil {
			return "", 0, 0, err
		}
		return crShowModePerCR, id, id, nil
	}

	ctx, err := svc.CurrentCR()
	if err == nil && ctx != nil && ctx.CR != nil {
		if forceDashboard {
			return crShowModeDashboard, 0, ctx.CR.ID, nil
		}
		return crShowModePerCR, ctx.CR.ID, ctx.CR.ID, nil
	}
	if err != nil && !errorsIs(err, service.ErrNoActiveCRContext) {
		return "", 0, 0, err
	}

	return crShowModeDashboard, 0, 0, nil
}

func runCRShowPerCR(cmd *cobra.Command, asJSON bool, noOpen bool, svc *service.Service, id int, eventsLimit int, checkpointsLimit int) error {
	if !asJSON {
		fmt.Fprintf(cmd.OutOrStdout(), "Preparing CR %d localhost preview...\n", id)
	}
	view, payload, err := buildCRShowSnapshot(svc, id, eventsLimit, checkpointsLimit)
	if err != nil {
		return commandError(cmd, asJSON, err)
	}
	const templateSource = "embedded:internal/cli/templates/cr_show.html"

	var preview *crShowPreviewSession
	if !noOpen {
		preview, err = startCRShowPreviewSession(
			templateSource,
			func() (string, error) {
				_, _, snapshotErr := buildCRDashboardSnapshot(svc, model.CRSearchQuery{}, defaultCRListLimit, defaultCRTimelineLimit, id)
				if snapshotErr != nil {
					return "", snapshotErr
				}
				return buildCRListHTMLDocument(embeddedCRListHTMLTemplate, buildCRShowBootstrap(crShowModeDashboard, 0))
			},
			func(routeCRID int) (string, error) {
				view, _, snapshotErr := buildCRShowSnapshot(svc, routeCRID, eventsLimit, checkpointsLimit)
				if snapshotErr != nil {
					return "", snapshotErr
				}
				return buildCRShowHTMLDocument(embeddedCRShowHTMLTemplate, buildCRShowBootstrap(crShowModePerCR, view.CR.ID))
			},
			func(r *http.Request) (map[string]any, error) {
				requestQuery, requestSelectedHint := resolveCRShowDashboardRequest(r, model.CRSearchQuery{}, id)
				livePayload, _, snapshotErr := buildCRDashboardSnapshot(svc, requestQuery, defaultCRListLimit, defaultCRTimelineLimit, requestSelectedHint)
				if snapshotErr != nil {
					return nil, snapshotErr
				}
				return livePayload, nil
			},
			func(routeCRID int) (map[string]any, error) {
				_, livePayload, snapshotErr := buildCRShowSnapshot(svc, routeCRID, eventsLimit, checkpointsLimit)
				if snapshotErr != nil {
					return nil, snapshotErr
				}
				return livePayload, nil
			},
			buildCRShowPerCRLaunchHandler(svc, view),
			fmt.Sprintf("/%d", id),
		)
		if err != nil {
			return commandError(cmd, asJSON, fmt.Errorf("start localhost preview: %w", err))
		}
		defer preview.Shutdown()
	} else {
		preview = newCRShowPreviewSession(templateSource)
	}

	if asJSON {
		preview.ObserveFirstRender(defaultCRShowInitialRenderWait)
		return writeJSONSuccess(cmd, map[string]any{
			"cr_id":           id,
			"cr_uid":          view.CR.UID,
			"view_mode":       "localhost_ephemeral",
			"url":             preview.viewURL,
			"template_source": templateSource,
			"warnings":        stringSliceOrEmpty(view.Warnings),
			"open_attempted":  preview.OpenAttempted(),
			"opened":          preview.Opened(),
			"page_served":     preview.PageServed(),
			"close_reason":    preview.closeReason,
			"open_error":      preview.OpenError(),
			"generated_at":    payload["generated_at"],
		})
	}

	fmt.Fprintf(cmd.OutOrStdout(), "CR %d localhost preview prepared.\n", id)
	if strings.TrimSpace(preview.viewURL) != "" {
		fmt.Fprintf(cmd.OutOrStdout(), "Preview URL: %s\n", preview.viewURL)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Template source: %s\n", templateSource)
	if len(view.Warnings) > 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "Warnings:")
		for _, warning := range view.Warnings {
			fmt.Fprintf(cmd.OutOrStdout(), "- %s\n", warning)
		}
	}
	printCRShowPreviewSessionStatus(cmd, preview, "Opened report in your default browser.")
	preview.WaitForClose()
	printCRShowPreviewSessionClose(cmd, preview)
	return nil
}

func runCRShowDashboard(cmd *cobra.Command, asJSON bool, noOpen bool, svc *service.Service, query model.CRSearchQuery, listLimit int, timelineLimit int, eventsLimit int, checkpointsLimit int, selectedHint int) error {
	if !asJSON {
		fmt.Fprintln(cmd.OutOrStdout(), "Preparing CR dashboard localhost preview...")
	}
	payload, selectedCRID, err := buildCRDashboardSnapshot(svc, query, listLimit, timelineLimit, selectedHint)
	if err != nil {
		return commandError(cmd, asJSON, err)
	}
	const templateSource = "embedded:internal/cli/templates/cr_list.html"

	var preview *crShowPreviewSession
	if !noOpen {
		preview, err = startCRShowPreviewSession(
			templateSource,
			func() (string, error) {
				_, _, snapshotErr := buildCRDashboardSnapshot(svc, query, listLimit, timelineLimit, selectedHint)
				if snapshotErr != nil {
					return "", snapshotErr
				}
				return buildCRListHTMLDocument(embeddedCRListHTMLTemplate, buildCRShowBootstrap(crShowModeDashboard, selectedCRID))
			},
			func(routeCRID int) (string, error) {
				view, _, snapshotErr := buildCRShowSnapshot(svc, routeCRID, eventsLimit, checkpointsLimit)
				if snapshotErr != nil {
					return "", snapshotErr
				}
				return buildCRShowHTMLDocument(embeddedCRShowHTMLTemplate, buildCRShowBootstrap(crShowModePerCR, view.CR.ID))
			},
			func(r *http.Request) (map[string]any, error) {
				requestQuery, requestSelectedHint := resolveCRShowDashboardRequest(r, query, selectedHint)
				livePayload, _, snapshotErr := buildCRDashboardSnapshot(svc, requestQuery, listLimit, timelineLimit, requestSelectedHint)
				if snapshotErr != nil {
					return nil, snapshotErr
				}
				return livePayload, nil
			},
			func(routeCRID int) (map[string]any, error) {
				_, livePayload, snapshotErr := buildCRShowSnapshot(svc, routeCRID, eventsLimit, checkpointsLimit)
				if snapshotErr != nil {
					return nil, snapshotErr
				}
				return livePayload, nil
			},
			nil,
			"/",
		)
		if err != nil {
			return commandError(cmd, asJSON, fmt.Errorf("start localhost preview: %w", err))
		}
		defer preview.Shutdown()
	} else {
		preview = newCRShowPreviewSession(templateSource)
	}

	dashboard := mapStringAny(payload["dashboard"])
	filters := mapStringAny(dashboard["filters"])
	counts := mapStringAny(dashboard["counts"])
	selectedValue := any(nil)
	if selectedCRID > 0 {
		selectedValue = selectedCRID
	}

	if asJSON {
		preview.ObserveFirstRender(defaultCRShowInitialRenderWait)
		return writeJSONSuccess(cmd, map[string]any{
			"view_mode":       "localhost_dashboard",
			"url":             preview.viewURL,
			"template_source": templateSource,
			"open_attempted":  preview.OpenAttempted(),
			"opened":          preview.Opened(),
			"page_served":     preview.PageServed(),
			"close_reason":    preview.closeReason,
			"open_error":      preview.OpenError(),
			"generated_at":    payload["generated_at"],
			"selected_cr_id":  selectedValue,
			"filters":         filters,
			"counts":          counts,
		})
	}

	fmt.Fprintln(cmd.OutOrStdout(), "CR dashboard localhost preview prepared.")
	if strings.TrimSpace(preview.viewURL) != "" {
		fmt.Fprintf(cmd.OutOrStdout(), "Preview URL: %s\n", preview.viewURL)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Template source: %s\n", templateSource)
	printCRShowPreviewSessionStatus(cmd, preview, "Opened dashboard in your default browser.")
	preview.WaitForClose()
	printCRShowPreviewSessionClose(cmd, preview)
	return nil
}

func buildCRShowSnapshot(svc *service.Service, id int, eventsLimit int, checkpointsLimit int) (*service.CRPackView, map[string]any, error) {
	view, err := svc.PackCR(id, service.PackOptions{
		EventsLimit:      eventsLimit,
		CheckpointsLimit: checkpointsLimit,
	})
	if err != nil {
		return nil, nil, err
	}
	if view == nil || view.CR == nil {
		return nil, nil, fmt.Errorf("cr %d is unavailable", id)
	}
	payload := crPackToJSONMap(view)
	crPayload := crToJSONMap(view.CR)
	crPayload["parent_cr_id"] = effectiveParentIDFromNativity(view.CR.ParentCRID, view.StackNativity)
	payload["cr"] = crPayload
	payload["delegation_launch"] = delegationLaunchToJSONMap(buildCRShowDelegationLaunchView(view))
	payload["generated_at"] = time.Now().UTC().Format(time.RFC3339)
	return view, payload, nil
}

func buildCRShowPerCRLaunchHandler(svc *service.Service, view *service.CRPackView) crShowLaunchHandler {
	if svc == nil || view == nil || view.CR == nil {
		return nil
	}
	crID := view.CR.ID
	return func(r *http.Request) (map[string]any, int, error) {
		input, err := decodeCRShowDelegationLaunchInput(r)
		if err != nil {
			return nil, http.StatusBadRequest, err
		}
		if input.CRID != 0 && input.CRID != crID {
			return nil, http.StatusBadRequest, fmt.Errorf("launch request targets cr %d but preview is bound to cr %d", input.CRID, crID)
		}
		currentView, err := svc.PackCR(crID, service.PackOptions{})
		if err != nil {
			return nil, http.StatusInternalServerError, err
		}
		if currentView == nil || currentView.CR == nil {
			return nil, http.StatusInternalServerError, fmt.Errorf("cr %d is unavailable", crID)
		}
		launch := buildCRShowDelegationLaunchView(currentView)
		if !launch.Available {
			return nil, http.StatusConflict, errors.New(nonEmpty(launch.Reason, "delegation launch is unavailable"))
		}
		request, selectedTaskIDs, err := buildCRShowDelegationRequest(currentView, input.TaskIDs)
		if err != nil {
			return nil, http.StatusBadRequest, err
		}
		run, err := svc.StartDelegation(crID, request)
		if err != nil {
			return nil, http.StatusInternalServerError, err
		}
		return map[string]any{
			"cr_id":               crID,
			"runtime":             request.Runtime,
			"selected_task_ids":   intSliceOrEmpty(selectedTaskIDs),
			"default_task_ids":    intSliceOrEmpty(launch.DefaultTaskIDs),
			"defaulted_selection": len(input.TaskIDs) == 0,
			"run":                 delegationRunToJSONMap(run),
		}, http.StatusAccepted, nil
	}
}

func newCRShowPreviewSession(templateSource string) *crShowPreviewSession {
	return &crShowPreviewSession{
		templateSource:   templateSource,
		launchState:      crShowBrowserLaunchNotAttempted,
		firstRenderState: crShowFirstRenderNotAwaited,
	}
}

func startCRShowPreviewSession(
	templateSource string,
	renderRoot func() (string, error),
	renderCR func(id int) (string, error),
	snapshotRoot crShowSnapshotRenderer,
	snapshotCR crShowCRSnapshotRenderer,
	launchHandler crShowLaunchHandler,
	openPath string,
) (*crShowPreviewSession, error) {
	server, err := startCRShowServerWithLiveRoutesAndLaunch(renderRoot, renderCR, snapshotRoot, snapshotCR, launchHandler)
	if err != nil {
		return nil, err
	}
	session := newCRShowPreviewSession(templateSource)
	session.server = server
	session.viewURL = server.URL
	targetURL := strings.TrimRight(session.viewURL, "/") + normalizeCRShowOpenPath(openPath)
	if err := openCRShowInBrowser(targetURL); err != nil {
		session.launchState = crShowBrowserLaunchFailed
		session.openErr = err.Error()
		return session, nil
	}
	session.launchState = crShowBrowserLaunchStarted
	return session, nil
}

func normalizeCRShowOpenPath(openPath string) string {
	trimmed := strings.TrimSpace(openPath)
	if trimmed == "" || trimmed == "/" {
		return "/"
	}
	if strings.HasPrefix(trimmed, "/") {
		return trimmed
	}
	return "/" + trimmed
}

func (s *crShowPreviewSession) OpenAttempted() bool {
	return s != nil && s.launchState != crShowBrowserLaunchNotAttempted
}

func (s *crShowPreviewSession) Opened() bool {
	return s != nil && s.launchState == crShowBrowserLaunchStarted
}

func (s *crShowPreviewSession) PageServed() bool {
	return s != nil && s.firstRenderState == crShowFirstRenderObserved
}

func (s *crShowPreviewSession) OpenError() string {
	if s == nil {
		return ""
	}
	if s.openedButNotRendered() {
		return nonEmpty(s.openErr, "browser did not request localhost preview before timeout")
	}
	return s.openErr
}

func (s *crShowPreviewSession) openedButNotRendered() bool {
	return s != nil && s.launchState == crShowBrowserLaunchStarted && s.firstRenderState == crShowFirstRenderTimedOut
}

func (s *crShowPreviewSession) ObserveFirstRender(timeout time.Duration) {
	if s == nil || s.server == nil || s.launchState != crShowBrowserLaunchStarted || s.firstRenderState != crShowFirstRenderNotAwaited {
		return
	}
	if s.server.WaitForFirstRender(timeout) {
		s.firstRenderState = crShowFirstRenderObserved
		return
	}
	s.firstRenderState = crShowFirstRenderTimedOut
}

func (s *crShowPreviewSession) WaitForClose() {
	if s == nil || s.server == nil || s.launchState == crShowBrowserLaunchNotAttempted {
		return
	}
	s.closeReason = waitForCRShowClose(s.server)
}

func (s *crShowPreviewSession) Shutdown() {
	if s == nil || s.server == nil {
		return
	}
	s.server.Shutdown()
}

func printCRShowPreviewSessionStatus(cmd *cobra.Command, session *crShowPreviewSession, openedMessage string) {
	if cmd == nil || session == nil {
		return
	}
	switch session.launchState {
	case crShowBrowserLaunchNotAttempted:
		fmt.Fprintln(cmd.OutOrStdout(), "Browser open skipped (--no-open).")
	case crShowBrowserLaunchFailed:
		fmt.Fprintf(cmd.OutOrStdout(), "Could not open browser automatically: %s\n", nonEmpty(session.openErr, "unknown error"))
		fmt.Fprintln(cmd.OutOrStdout(), "Preview is live. Open the URL manually, then use the page's Close Preview button (or Ctrl+C) to stop the instance.")
	case crShowBrowserLaunchStarted:
		session.ObserveFirstRender(defaultCRShowInitialRenderWait)
		if session.PageServed() {
			fmt.Fprintln(cmd.OutOrStdout(), openedMessage)
		} else {
			fmt.Fprintln(cmd.OutOrStdout(), "Browser launch started. Preview is running; open the URL manually if the browser is slow to attach.")
		}
		fmt.Fprintln(cmd.OutOrStdout(), "Preview is live. Use the page's Close Preview button (or Ctrl+C) to stop the instance.")
	}
}

func printCRShowPreviewSessionClose(cmd *cobra.Command, session *crShowPreviewSession) {
	if cmd == nil || session == nil || strings.TrimSpace(session.closeReason) == "" {
		return
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Preview closed (%s).\n", session.closeReason)
}

func decodeCRShowDelegationLaunchInput(r *http.Request) (crShowDelegationLaunchInput, error) {
	var input crShowDelegationLaunchInput
	if r == nil || r.Body == nil {
		return input, nil
	}
	defer r.Body.Close()
	body, err := io.ReadAll(io.LimitReader(r.Body, 64<<10))
	if err != nil {
		return input, fmt.Errorf("read launch request: %w", err)
	}
	if len(bytes.TrimSpace(body)) == 0 {
		return input, nil
	}
	if err := json.Unmarshal(body, &input); err != nil {
		return input, fmt.Errorf("decode launch request: %w", err)
	}
	return input, nil
}

func buildCRShowDelegationLaunchView(view *service.CRPackView) crShowDelegationLaunchView {
	launch := crShowDelegationLaunchView{
		Runtime:   defaultCRShowDelegationRuntime,
		SkillRefs: []string{"sophia"},
	}
	if view == nil || view.CR == nil {
		launch.Reason = "cr preview payload is unavailable"
		return launch
	}
	for _, task := range view.Tasks {
		launch.AllTaskIDs = append(launch.AllTaskIDs, task.ID)
		if strings.TrimSpace(task.Status) != model.TaskStatusDone {
			launch.OpenTaskIDs = append(launch.OpenTaskIDs, task.ID)
		}
	}
	if len(launch.OpenTaskIDs) > 0 {
		launch.DefaultTaskIDs = append([]int(nil), launch.OpenTaskIDs...)
	} else {
		launch.DefaultTaskIDs = append([]int(nil), launch.AllTaskIDs...)
	}
	switch strings.TrimSpace(view.CR.Status) {
	case model.StatusMerged:
		launch.Reason = "merged crs cannot launch new delegations"
		return launch
	case model.StatusAbandoned:
		launch.Reason = "abandoned crs must be reopened before delegation"
		return launch
	}
	launch.Available = true
	return launch
}

func buildCRShowDelegationRequest(view *service.CRPackView, selectedTaskIDs []int) (model.DelegationRequest, []int, error) {
	launch := buildCRShowDelegationLaunchView(view)
	if !launch.Available {
		return model.DelegationRequest{}, nil, errors.New(nonEmpty(launch.Reason, "delegation launch is unavailable"))
	}
	selection, err := normalizeCRShowDelegationSelection(view, selectedTaskIDs, launch.DefaultTaskIDs)
	if err != nil {
		return model.DelegationRequest{}, nil, err
	}
	request := model.DelegationRequest{
		Runtime:              launch.Runtime,
		TaskIDs:              append([]int(nil), selection...),
		WorkflowInstructions: buildCRShowDelegationWorkflowInstructions(view, selection),
		SkillRefs:            append([]string(nil), launch.SkillRefs...),
		Metadata: map[string]string{
			"source":            "cr_show",
			"launch_surface":    "localhost_preview",
			"cr_id":             strconv.Itoa(view.CR.ID),
			"cr_uid":            strings.TrimSpace(view.CR.UID),
			"selected_task_ids": joinIntCSV(selection),
		},
	}
	return request, selection, nil
}

func normalizeCRShowDelegationSelection(view *service.CRPackView, selectedTaskIDs []int, defaultTaskIDs []int) ([]int, error) {
	if view == nil || view.CR == nil {
		return nil, fmt.Errorf("cr preview payload is unavailable")
	}
	raw := selectedTaskIDs
	if len(raw) == 0 {
		raw = defaultTaskIDs
	}
	if len(raw) == 0 {
		return nil, nil
	}
	taskByID := make(map[int]model.Subtask, len(view.Tasks))
	for _, task := range view.Tasks {
		taskByID[task.ID] = task
	}
	selected := make([]int, 0, len(raw))
	seen := make(map[int]struct{}, len(raw))
	for _, id := range raw {
		if id <= 0 {
			return nil, fmt.Errorf("task ids must be positive integers")
		}
		if _, ok := taskByID[id]; !ok {
			return nil, fmt.Errorf("task %d is not part of cr %d", id, view.CR.ID)
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		selected = append(selected, id)
	}
	return selected, nil
}

func buildCRShowDelegationWorkflowInstructions(view *service.CRPackView, selectedTaskIDs []int) string {
	if view == nil || view.CR == nil {
		return ""
	}
	var b strings.Builder
	writeWorkflowLine := func(line string) {
		line = strings.TrimSpace(line)
		if line == "" {
			return
		}
		b.WriteString(line)
		b.WriteString("\n")
	}
	writeWorkflowLine("You are executing a Sophia delegation launched from `cr show`.")
	writeWorkflowLine("Treat the CR contract as the source of truth for scope, invariants, blast radius, and completion criteria.")
	writeWorkflowLine("Use Sophia locally for additional context or follow-up state, but do not widen scope beyond the contract and selected tasks.")
	writeWorkflowLine("")
	writeWorkflowLine(fmt.Sprintf("CR %d: %s", view.CR.ID, view.CR.Title))
	writeWorkflowLine(fmt.Sprintf("Base branch: %s", nonEmpty(strings.TrimSpace(view.CR.BaseRef), strings.TrimSpace(view.CR.BaseBranch))))
	if why := strings.TrimSpace(view.Contract.Why); why != "" {
		writeWorkflowLine("Why:")
		writeWorkflowLine(why)
	}
	writeWorkflowBullets(&b, "Scope", view.Contract.Scope)
	writeWorkflowBullets(&b, "Non-goals", view.Contract.NonGoals)
	writeWorkflowBullets(&b, "Invariants", view.Contract.Invariants)
	if blast := strings.TrimSpace(view.Contract.BlastRadius); blast != "" {
		writeWorkflowLine("Blast radius:")
		writeWorkflowLine(blast)
	}
	if testPlan := strings.TrimSpace(view.Contract.TestPlan); testPlan != "" {
		writeWorkflowLine("Test plan:")
		writeWorkflowLine(testPlan)
	}
	selectedTasks := selectCRShowDelegationTasks(view.Tasks, selectedTaskIDs)
	if len(selectedTasks) == 0 {
		writeWorkflowLine("Selected task set: none explicitly selected; operate against the CR contract directly.")
	} else {
		writeWorkflowLine("Selected tasks:")
		for _, task := range selectedTasks {
			writeWorkflowLine(fmt.Sprintf("- Task %d: %s", task.ID, task.Title))
			if intent := strings.TrimSpace(task.Contract.Intent); intent != "" {
				writeWorkflowLine("  intent: " + intent)
			}
			if len(task.Contract.Scope) > 0 {
				writeWorkflowLine("  scope: " + strings.Join(task.Contract.Scope, ", "))
			}
			if len(task.Contract.AcceptanceCriteria) > 0 {
				writeWorkflowLine("  acceptance: " + strings.Join(task.Contract.AcceptanceCriteria, " | "))
			}
			if len(task.Contract.AcceptanceChecks) > 0 {
				writeWorkflowLine("  checks: " + strings.Join(task.Contract.AcceptanceChecks, ", "))
			}
		}
	}
	writeWorkflowLine("")
	writeWorkflowLine("Return a concise completion summary and any blockers if the work cannot be completed within the selected scope.")
	return strings.TrimSpace(b.String())
}

func writeWorkflowBullets(b *strings.Builder, label string, values []string) {
	values = normalizeCRShowStringList(values)
	if b == nil || len(values) == 0 {
		return
	}
	b.WriteString(label)
	b.WriteString(":\n")
	for _, value := range values {
		b.WriteString("- ")
		b.WriteString(value)
		b.WriteString("\n")
	}
}

func normalizeCRShowStringList(values []string) []string {
	out := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		out = append(out, trimmed)
	}
	return out
}

func selectCRShowDelegationTasks(tasks []model.Subtask, selectedTaskIDs []int) []model.Subtask {
	if len(selectedTaskIDs) == 0 {
		return nil
	}
	byID := make(map[int]model.Subtask, len(tasks))
	for _, task := range tasks {
		byID[task.ID] = task
	}
	selected := make([]model.Subtask, 0, len(selectedTaskIDs))
	for _, id := range selectedTaskIDs {
		task, ok := byID[id]
		if !ok {
			continue
		}
		selected = append(selected, task)
	}
	return selected
}

func joinIntCSV(values []int) string {
	if len(values) == 0 {
		return ""
	}
	parts := make([]string, 0, len(values))
	for _, value := range values {
		parts = append(parts, strconv.Itoa(value))
	}
	return strings.Join(parts, ",")
}

func buildCRDashboardSnapshot(svc *service.Service, query model.CRSearchQuery, listLimit int, timelineLimit int, selectedHint int) (map[string]any, int, error) {
	if svc == nil {
		return nil, 0, fmt.Errorf("service is required")
	}
	if listLimit <= 0 {
		listLimit = defaultCRListLimit
	}
	if timelineLimit <= 0 {
		timelineLimit = defaultCRTimelineLimit
	}

	readModel, err := svc.LoadCRReadModelForCLI()
	if err != nil {
		return nil, 0, err
	}
	results := searchCRsForDashboard(readModel, query)
	if strings.TrimSpace(query.Status) == "" {
		results = filterDashboardResultsDefaultStatus(results)
	}

	resultByID := make(map[int]model.CRSearchResult, len(results))
	filteredIDs := make(map[int]struct{}, len(results))
	rows := make([]map[string]any, 0, minInt(len(results), listLimit))
	for i, result := range results {
		resultByID[result.ID] = result
		filteredIDs[result.ID] = struct{}{}
		if i >= listLimit {
			continue
		}
		cr, ok := readModel.CRByIDForCLI(result.ID)
		if !ok {
			continue
		}
		rows = append(rows, buildDashboardCRRow(svc, readModel, result, cr))
	}

	selectedCRID := 0
	if selectedHint > 0 {
		if _, ok := filteredIDs[selectedHint]; ok {
			selectedCRID = selectedHint
		}
	}
	if selectedCRID == 0 && len(results) > 0 {
		selectedCRID = results[0].ID
	}

	var selected map[string]any
	if selectedCRID > 0 {
		if cr, ok := readModel.CRByIDForCLI(selectedCRID); ok {
			if result, hasResult := resultByID[selectedCRID]; hasResult {
				selected = buildDashboardSelectedCR(svc, readModel, result, cr)
			} else {
				selected = buildDashboardSelectedCR(svc, readModel, model.CRSearchResult{
					ID:         cr.ID,
					UID:        cr.UID,
					Title:      cr.Title,
					Status:     cr.Status,
					Branch:     cr.Branch,
					BaseBranch: cr.BaseBranch,
					ParentCRID: cr.ParentCRID,
					RiskTier:   nonEmpty(strings.TrimSpace(cr.Contract.RiskTierHint), "-"),
					CreatedAt:  cr.CreatedAt,
					UpdatedAt:  cr.UpdatedAt,
				}, cr)
			}
		}
	}
	if selectedCRID > 0 && selected != nil {
		if status, statusErr := svc.StatusCR(selectedCRID); statusErr == nil && status != nil {
			selected["lifecycle_state"] = nonEmpty(strings.TrimSpace(status.LifecycleState), strings.TrimSpace(status.Status))
			selected["abandoned_at"] = strings.TrimSpace(status.AbandonedAt)
			selected["abandoned_by"] = strings.TrimSpace(status.AbandonedBy)
			selected["abandoned_reason"] = strings.TrimSpace(status.AbandonedReason)
			selected["pr_linkage_state"] = strings.TrimSpace(status.PRLinkageState)
			selected["action_required"] = strings.TrimSpace(status.ActionRequired)
			selected["action_reason"] = strings.TrimSpace(status.ActionReason)
			selected["suggested_commands"] = stringSliceOrEmpty(status.SuggestedCommands)
		}
	}

	timelineItems := make([]dashboardTimelineEntry, 0)
	for _, cr := range readModel.AllCRsForCLI() {
		if _, ok := filteredIDs[cr.ID]; !ok {
			continue
		}
		for _, event := range cr.Events {
			timelineItems = append(timelineItems, dashboardTimelineEntry{
				TS:       event.TS,
				TSParsed: parseRFC3339OrZero(event.TS),
				Type:     event.Type,
				Summary:  event.Summary,
				Actor:    event.Actor,
				Ref:      event.Ref,
				Redacted: event.Redacted,
				CRID:     cr.ID,
				CRUID:    cr.UID,
				CRTitle:  cr.Title,
				CRStatus: cr.Status,
			})
		}
	}
	sort.SliceStable(timelineItems, func(i, j int) bool {
		if timelineItems[i].TSParsed.Equal(timelineItems[j].TSParsed) {
			if timelineItems[i].CRID == timelineItems[j].CRID {
				return timelineItems[i].Type < timelineItems[j].Type
			}
			return timelineItems[i].CRID > timelineItems[j].CRID
		}
		return timelineItems[i].TSParsed.After(timelineItems[j].TSParsed)
	})

	timelineTotal := len(timelineItems)
	if timelineTotal > timelineLimit {
		timelineItems = timelineItems[:timelineLimit]
	}

	timeline := make([]map[string]any, 0, len(timelineItems))
	for _, entry := range timelineItems {
		timeline = append(timeline, map[string]any{
			"ts":        entry.TS,
			"type":      entry.Type,
			"summary":   entry.Summary,
			"actor":     entry.Actor,
			"ref":       entry.Ref,
			"redacted":  entry.Redacted,
			"cr_id":     entry.CRID,
			"cr_uid":    entry.CRUID,
			"cr_title":  entry.CRTitle,
			"cr_status": entry.CRStatus,
		})
	}

	selectedAny := any(nil)
	if selectedCRID > 0 {
		selectedAny = selectedCRID
	}

	payload := map[string]any{
		"generated_at": time.Now().UTC().Format(time.RFC3339),
		"dashboard": map[string]any{
			"selected_cr_id": selectedAny,
			"filters": map[string]any{
				"status":         query.Status,
				"risk_tier":      query.RiskTier,
				"scope":          query.ScopePrefix,
				"text":           query.Text,
				"list_limit":     listLimit,
				"timeline_limit": timelineLimit,
			},
			"counts": map[string]any{
				"list_total":        len(results),
				"list_returned":     len(rows),
				"timeline_total":    timelineTotal,
				"timeline_returned": len(timeline),
			},
		},
		"crs":         rows,
		"timeline":    timeline,
		"selected_cr": selected,
	}

	return payload, selectedCRID, nil
}

func searchCRsForDashboard(readModel *service.CRReadModelView, query model.CRSearchQuery) []model.CRSearchResult {
	if readModel == nil {
		return nil
	}
	results := make([]model.CRSearchResult, 0, len(readModel.AllCRsForCLI()))
	for _, cr := range readModel.AllCRsForCLI() {
		if !service.MatchCRSearchForCLI(cr, query) {
			continue
		}
		tasksOpen, tasksDone, _ := service.CountTaskStatsForCLI(cr.Subtasks)
		riskTier := cr.Contract.RiskTierHint
		if riskTier == "" {
			riskTier = "-"
		}
		results = append(results, model.CRSearchResult{
			ID:         cr.ID,
			UID:        cr.UID,
			Title:      cr.Title,
			Status:     cr.Status,
			Branch:     cr.Branch,
			BaseBranch: cr.BaseBranch,
			ParentCRID: cr.ParentCRID,
			RiskTier:   riskTier,
			TasksTotal: len(cr.Subtasks),
			TasksOpen:  tasksOpen,
			TasksDone:  tasksDone,
			CreatedAt:  cr.CreatedAt,
			UpdatedAt:  cr.UpdatedAt,
		})
	}
	sort.Slice(results, func(i, j int) bool {
		return results[i].ID < results[j].ID
	})
	return results
}

func filterDashboardResultsDefaultStatus(results []model.CRSearchResult) []model.CRSearchResult {
	if len(results) == 0 {
		return results
	}
	filtered := make([]model.CRSearchResult, 0, len(results))
	for _, result := range results {
		if strings.TrimSpace(result.Status) == model.StatusAbandoned {
			continue
		}
		filtered = append(filtered, result)
	}
	return filtered
}

type dashboardTimelineEntry struct {
	TS       string
	TSParsed time.Time
	Type     string
	Summary  string
	Actor    string
	Ref      string
	Redacted bool
	CRID     int
	CRUID    string
	CRTitle  string
	CRStatus string
}

func buildDashboardCRRow(svc *service.Service, readModel *service.CRReadModelView, result model.CRSearchResult, cr model.CR) map[string]any {
	lastEventAt := ""
	if n := len(cr.Events); n > 0 {
		lastEventAt = cr.Events[n-1].TS
	}
	nativity := service.StackNativityView{}
	lineage := []service.StackLineageNodeView{}
	var tree *service.StackTreeNodeView
	if svc != nil {
		nativity = svc.StackNativityForCLIWithReadModel(&cr, readModel)
		lineage = svc.StackLineageForCLIWithReadModel(&cr, readModel)
		tree = svc.StackTreeForCLIWithReadModel(&cr, readModel)
	}
	return map[string]any{
		"id":                  result.ID,
		"uid":                 result.UID,
		"title":               result.Title,
		"status":              result.Status,
		"branch":              result.Branch,
		"base_branch":         result.BaseBranch,
		"base_ref":            cr.BaseRef,
		"base_commit":         cr.BaseCommit,
		"parent_cr_id":        effectiveParentIDFromNativity(result.ParentCRID, nativity),
		"risk_tier":           result.RiskTier,
		"created_at":          result.CreatedAt,
		"updated_at":          result.UpdatedAt,
		"description":         cr.Description,
		"contract_why":        cr.Contract.Why,
		"contract_scope":      stringSliceOrEmpty(cr.Contract.Scope),
		"contract_non_goals":  stringSliceOrEmpty(cr.Contract.NonGoals),
		"contract_invariants": stringSliceOrEmpty(cr.Contract.Invariants),
		"last_event_at":       lastEventAt,
		"stack_nativity":      stackNativityToJSONMap(nativity),
		"stack_lineage":       stackLineageToJSONMaps(lineage),
		"stack_tree":          stackTreeNodeToJSONMap(tree),
		"tasks": map[string]any{
			"total": result.TasksTotal,
			"open":  result.TasksOpen,
			"done":  result.TasksDone,
		},
	}
}

func buildDashboardSelectedCR(svc *service.Service, readModel *service.CRReadModelView, result model.CRSearchResult, cr model.CR) map[string]any {
	return buildDashboardCRRow(svc, readModel, result, cr)
}

func parseRFC3339OrZero(raw string) time.Time {
	v := strings.TrimSpace(raw)
	if v == "" {
		return time.Time{}
	}
	parsed, err := time.Parse(time.RFC3339, v)
	if err != nil {
		return time.Time{}
	}
	return parsed.UTC()
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func mapStringAny(raw any) map[string]any {
	if raw == nil {
		return map[string]any{}
	}
	if out, ok := raw.(map[string]any); ok {
		return out
	}
	return map[string]any{}
}

type crShowServer struct {
	URL        string
	renderedCh chan struct{}
	closedCh   chan string
	once       sync.Once
	closeOnce  sync.Once
	server     *http.Server
	listener   net.Listener
}

type crShowSnapshotRenderer func(r *http.Request) (map[string]any, error)
type crShowCRSnapshotRenderer func(id int) (map[string]any, error)
type crShowLaunchHandler func(r *http.Request) (map[string]any, int, error)
type crShowDelegationLaunchInput struct {
	CRID    int   `json:"cr_id"`
	TaskIDs []int `json:"task_ids"`
}

type crShowDelegationLaunchView struct {
	Available      bool
	Reason         string
	Runtime        string
	DefaultTaskIDs []int
	OpenTaskIDs    []int
	AllTaskIDs     []int
	SkillRefs      []string
}

func buildCRShowBootstrap(mode crShowMode, id int) map[string]any {
	bootstrap := map[string]any{
		"mode":          string(mode),
		"close_url":     "/__sophia_close",
		"snapshot_root": "/__sophia_snapshot",
	}
	switch mode {
	case crShowModePerCR:
		bootstrap["cr_id"] = id
		bootstrap["snapshot_url"] = fmt.Sprintf("/__sophia_snapshot?mode=cr&id=%d", id)
		bootstrap["events_url"] = fmt.Sprintf("/__sophia_events?mode=cr&id=%d", id)
		bootstrap["delegate_launch_url"] = "/__sophia_delegate_launch"
	case crShowModeDashboard:
		if id > 0 {
			bootstrap["selected_cr_id"] = id
		}
		bootstrap["snapshot_url"] = "/__sophia_snapshot?mode=dashboard"
		bootstrap["events_url"] = "/__sophia_events?mode=dashboard"
	}
	return bootstrap
}

func resolveCRShowDashboardRequest(r *http.Request, defaults model.CRSearchQuery, defaultSelectedHint int) (model.CRSearchQuery, int) {
	query := defaults
	selectedHint := defaultSelectedHint
	if r == nil || r.URL == nil {
		return query, selectedHint
	}

	values := r.URL.Query()
	if status, ok := crShowOptionalQueryValue(values, "status"); ok {
		query.Status = status
	}
	if scope, ok := crShowOptionalQueryValue(values, "scope"); ok {
		query.ScopePrefix = scope
	}
	if text, ok := crShowOptionalQueryValue(values, "text"); ok {
		query.Text = text
	}
	if riskTier, ok := crShowOptionalQueryValue(values, "risk_tier"); ok {
		query.RiskTier = riskTier
		if normalizedRiskTier, err := normalizeRiskTierFilter(riskTier); err == nil {
			query.RiskTier = normalizedRiskTier
		}
	}
	if selected, ok := crShowOptionalPositiveIntQueryValue(values, "selected_cr_id", "selected"); ok {
		selectedHint = selected
	}
	return query, selectedHint
}

func crShowOptionalQueryValue(values map[string][]string, key string) (string, bool) {
	if len(values) == 0 {
		return "", false
	}
	raw, ok := values[key]
	if !ok || len(raw) == 0 {
		return "", false
	}
	return strings.TrimSpace(raw[len(raw)-1]), true
}

func crShowOptionalPositiveIntQueryValue(values map[string][]string, keys ...string) (int, bool) {
	for _, key := range keys {
		raw, ok := crShowOptionalQueryValue(values, key)
		if !ok {
			continue
		}
		id, err := strconv.Atoi(raw)
		if err != nil || id <= 0 {
			return 0, true
		}
		return id, true
	}
	return 0, false
}

func startCRShowServer(render func() (string, error)) (*crShowServer, error) {
	return startCRShowServerWithRoutes(render, nil)
}

func startCRShowServerWithRoutes(renderRoot func() (string, error), renderCR func(id int) (string, error)) (*crShowServer, error) {
	return startCRShowServerWithLiveRoutes(renderRoot, renderCR, nil, nil)
}

func startCRShowServerWithLiveRoutes(
	renderRoot func() (string, error),
	renderCR func(id int) (string, error),
	snapshotRoot crShowSnapshotRenderer,
	snapshotCR crShowCRSnapshotRenderer,
) (*crShowServer, error) {
	return startCRShowServerWithLiveRoutesAndLaunch(renderRoot, renderCR, snapshotRoot, snapshotCR, nil)
}

func startCRShowServerWithLiveRoutesAndLaunch(
	renderRoot func() (string, error),
	renderCR func(id int) (string, error),
	snapshotRoot crShowSnapshotRenderer,
	snapshotCR crShowCRSnapshotRenderer,
	launchHandler crShowLaunchHandler,
) (*crShowServer, error) {
	if renderRoot == nil {
		return nil, fmt.Errorf("render callback is required")
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}

	srv := &crShowServer{
		URL:        "http://" + listener.Addr().String(),
		renderedCh: make(chan struct{}, 1),
		closedCh:   make(chan string, 1),
		listener:   listener,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		path := strings.Trim(strings.TrimSpace(r.URL.Path), "/")
		render := renderRoot
		if path != "" {
			if renderCR == nil {
				http.NotFound(w, r)
				return
			}
			id, parseErr := strconv.Atoi(path)
			if parseErr != nil || id <= 0 {
				http.NotFound(w, r)
				return
			}
			render = func() (string, error) {
				return renderCR(id)
			}
		}
		htmlDoc, renderErr := render()
		if renderErr != nil {
			http.Error(w, fmt.Sprintf("render preview: %v", renderErr), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Cache-Control", "no-store, max-age=0")
		w.Header().Set("Pragma", "no-cache")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.WriteString(w, htmlDoc)
		srv.once.Do(func() {
			srv.renderedCh <- struct{}{}
			close(srv.renderedCh)
		})
	})
	mux.HandleFunc("/__sophia_close", func(w http.ResponseWriter, r *http.Request) {
		srv.signalClose("ui_close_button")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	})
	mux.HandleFunc("/__sophia_delegate_launch", func(w http.ResponseWriter, r *http.Request) {
		if launchHandler == nil {
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		payload, statusCode, err := launchHandler(r)
		if statusCode <= 0 {
			statusCode = http.StatusOK
		}
		w.Header().Set("Cache-Control", "no-store, max-age=0")
		w.Header().Set("Pragma", "no-cache")
		w.Header().Set("Content-Type", "application/json")
		if err != nil {
			w.WriteHeader(statusCode)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok": false,
				"error": map[string]any{
					"message": strings.TrimSpace(err.Error()),
				},
			})
			return
		}
		w.WriteHeader(statusCode)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":   true,
			"data": payload,
		})
	})
	mux.HandleFunc("/__sophia_snapshot", func(w http.ResponseWriter, r *http.Request) {
		if snapshotRoot == nil && snapshotCR == nil {
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		renderSnapshot, snapshotErr := resolveCRShowSnapshotRenderer(r, snapshotRoot, snapshotCR)
		if snapshotErr != nil {
			http.Error(w, snapshotErr.Error(), http.StatusBadRequest)
			return
		}
		payload, snapshotErr := renderSnapshot(r)
		if snapshotErr != nil {
			http.Error(w, fmt.Sprintf("render snapshot: %v", snapshotErr), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Cache-Control", "no-store, max-age=0")
		w.Header().Set("Pragma", "no-cache")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(payload)
	})
	mux.HandleFunc("/__sophia_events", func(w http.ResponseWriter, r *http.Request) {
		if snapshotRoot == nil && snapshotCR == nil {
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming unsupported", http.StatusInternalServerError)
			return
		}

		streamSnapshot, streamErr := resolveCRShowSnapshotRenderer(r, snapshotRoot, snapshotCR)
		if streamErr != nil {
			http.Error(w, streamErr.Error(), http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
		w.Header().Set("Pragma", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.WriteHeader(http.StatusOK)
		flusher.Flush()

		sendStreamError := func(err error) error {
			if err == nil {
				return nil
			}
			payload, marshalErr := json.Marshal(map[string]any{
				"message": strings.TrimSpace(err.Error()),
			})
			if marshalErr != nil {
				return marshalErr
			}
			if writeErr := writeSSEEvent(w, "error", payload); writeErr != nil {
				return writeErr
			}
			flusher.Flush()
			return nil
		}

		lastHash := ""
		sendSnapshotIfChanged := func() error {
			payload, snapshotErr := streamSnapshot(r)
			if snapshotErr != nil {
				return snapshotErr
			}
			payloadJSON, marshalErr := json.Marshal(payload)
			if marshalErr != nil {
				return marshalErr
			}
			hash, hashErr := snapshotHash(payload)
			if hashErr != nil {
				return hashErr
			}
			if lastHash == hash {
				return nil
			}
			if writeErr := writeSSEEvent(w, "snapshot", payloadJSON); writeErr != nil {
				return writeErr
			}
			flusher.Flush()
			lastHash = hash
			return nil
		}

		if err := sendSnapshotIfChanged(); err != nil {
			if streamErr := sendStreamError(err); streamErr != nil {
				return
			}
		}

		pollTicker := time.NewTicker(defaultCRShowSSEPollInterval)
		keepaliveTicker := time.NewTicker(defaultCRShowSSEKeepaliveInterval)
		defer pollTicker.Stop()
		defer keepaliveTicker.Stop()

		for {
			select {
			case <-r.Context().Done():
				return
			case <-pollTicker.C:
				if err := sendSnapshotIfChanged(); err != nil {
					if streamErr := sendStreamError(err); streamErr != nil {
						return
					}
				}
			case <-keepaliveTicker.C:
				if _, err := io.WriteString(w, ": keepalive\n\n"); err != nil {
					return
				}
				flusher.Flush()
			}
		}
	})
	mux.HandleFunc("/favicon.ico", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	srv.server = &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		_ = srv.server.Serve(listener)
	}()
	return srv, nil
}

func resolveCRShowSnapshotRenderer(
	r *http.Request,
	snapshotRoot crShowSnapshotRenderer,
	snapshotCR crShowCRSnapshotRenderer,
) (crShowSnapshotRenderer, error) {
	if r == nil {
		return nil, fmt.Errorf("request is required")
	}
	mode := strings.TrimSpace(r.URL.Query().Get("mode"))
	switch mode {
	case "dashboard":
		if snapshotRoot == nil {
			return nil, fmt.Errorf("dashboard stream is unavailable")
		}
		return func(_ *http.Request) (map[string]any, error) {
			return snapshotRoot(r)
		}, nil
	case "cr":
		if snapshotCR == nil {
			return nil, fmt.Errorf("cr stream is unavailable")
		}
		id, err := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("id")))
		if err != nil || id <= 0 {
			return nil, fmt.Errorf("cr stream requires a valid id")
		}
		return func(_ *http.Request) (map[string]any, error) {
			return snapshotCR(id)
		}, nil
	default:
		return nil, fmt.Errorf("invalid stream mode")
	}
}

func normalizeSnapshotForCompare(payload map[string]any) (map[string]any, error) {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	var normalized map[string]any
	if err := json.Unmarshal(encoded, &normalized); err != nil {
		return nil, err
	}
	delete(normalized, "generated_at")
	return normalized, nil
}

func snapshotHash(payload map[string]any) (string, error) {
	normalized, err := normalizeSnapshotForCompare(payload)
	if err != nil {
		return "", err
	}
	encoded, err := json.Marshal(normalized)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:]), nil
}

func writeSSEEvent(w io.Writer, event string, payload []byte) error {
	if strings.TrimSpace(event) != "" {
		if _, err := io.WriteString(w, "event: "+strings.TrimSpace(event)+"\n"); err != nil {
			return err
		}
	}
	for _, line := range bytes.Split(payload, []byte("\n")) {
		if _, err := io.WriteString(w, "data: "+string(line)+"\n"); err != nil {
			return err
		}
	}
	_, err := io.WriteString(w, "\n")
	return err
}

func (s *crShowServer) WaitForFirstRender(timeout time.Duration) bool {
	if s == nil {
		return false
	}
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	select {
	case <-s.renderedCh:
		return true
	case <-time.After(timeout):
		return false
	}
}

func (s *crShowServer) Shutdown() {
	if s == nil || s.server == nil {
		return
	}
	s.signalClose("server_shutdown")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = s.server.Shutdown(ctx)
}

func (s *crShowServer) signalClose(reason string) {
	if s == nil {
		return
	}
	s.closeOnce.Do(func() {
		s.closedCh <- nonEmpty(strings.TrimSpace(reason), "closed")
		close(s.closedCh)
	})
}

func waitForCRShowClose(server *crShowServer) string {
	if server == nil {
		return "closed"
	}
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt)
	defer signal.Stop(sigCh)

	select {
	case reason, ok := <-server.closedCh:
		if !ok {
			return "closed"
		}
		return nonEmpty(strings.TrimSpace(reason), "closed")
	case <-sigCh:
		server.signalClose("interrupt")
		return "interrupt"
	}
}

func openCRShowInBrowser(targetURL string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", targetURL)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", targetURL)
	default:
		cmd = exec.Command("xdg-open", targetURL)
	}

	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

func buildCRShowHTMLDocument(templateHTML string, bootstrap map[string]any) (string, error) {
	title := "Sophia CR Report"
	if id := strings.TrimSpace(stringValue(bootstrap["cr_id"])); id != "" {
		title = fmt.Sprintf("CR-%s Report", id)
	}
	return buildCRShowDocument(templateHTML, bootstrap, title)
}

func buildCRListHTMLDocument(templateHTML string, bootstrap map[string]any) (string, error) {
	return buildCRShowDocument(templateHTML, bootstrap, "Sophia CR Dashboard")
}

func buildCRShowDocument(templateHTML string, bootstrap map[string]any, title string) (string, error) {
	encoded, err := json.Marshal(bootstrap)
	if err != nil {
		return "", err
	}
	var escaped bytes.Buffer
	json.HTMLEscape(&escaped, encoded)

	generatedAt := strings.TrimSpace(stringValue(bootstrap["generated_at"]))
	if generatedAt == "" {
		generatedAt = time.Now().UTC().Format(time.RFC3339)
	}

	doc := strings.TrimSpace(templateHTML)
	if doc == "" {
		return "", fmt.Errorf("template is empty")
	}
	doc = strings.ReplaceAll(doc, "__TITLE__", html.EscapeString(title))
	doc = strings.ReplaceAll(doc, "__GENERATED_AT__", html.EscapeString(generatedAt))
	doc = strings.ReplaceAll(doc, "__BOOTSTRAP_JSON__", escaped.String())
	return doc, nil
}

func stringValue(raw any) string {
	if raw == nil {
		return ""
	}
	switch v := raw.(type) {
	case string:
		return v
	case fmt.Stringer:
		return v.String()
	case int:
		return strconv.Itoa(v)
	case int64:
		return strconv.FormatInt(v, 10)
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	default:
		return fmt.Sprintf("%v", raw)
	}
}

//go:embed templates/cr_show.html
var embeddedCRShowHTMLTemplate string

//go:embed templates/cr_list.html
var embeddedCRListHTMLTemplate string
