package cli

import (
	"bytes"
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
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
	crShowActionPublishPR                        = "publish_pr"
	crShowActionPRSync                           = "pr_sync"
	crShowActionRefresh                          = "refresh"
	crShowActionValidate                         = "validate"
	crShowActionReview                           = "review"
	crShowActionMerge                            = "merge"
	crShowActionPull                             = "pull"
	crShowPullModeMetadata                       = "metadata"
	crShowPullModeFFOnly                         = "ff_only"
)

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
	cmd.Flags().StringVar(&statusFilter, "status", "", "Dashboard filter by status (in_progress, merged)")
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
	view, payload, err := buildCRShowSnapshot(svc, id, eventsLimit, checkpointsLimit)
	if err != nil {
		return commandError(cmd, asJSON, err)
	}
	const templateSource = "embedded:internal/cli/templates/cr_show.html"

	openAttempted := !noOpen
	opened := false
	pageServed := false
	openErr := ""
	viewURL := ""
	closeReason := ""
	var server *crShowServer
	if openAttempted {
		server, err = startCRShowServerWithLiveRoutesAndActions(
			func() (string, error) {
				livePayload, _, snapshotErr := buildCRDashboardSnapshot(svc, model.CRSearchQuery{}, defaultCRListLimit, defaultCRTimelineLimit, id)
				if snapshotErr != nil {
					return "", snapshotErr
				}
				return buildCRListHTMLDocument(embeddedCRListHTMLTemplate, livePayload)
			},
			func(routeCRID int) (string, error) {
				_, livePayload, snapshotErr := buildCRShowSnapshot(svc, routeCRID, eventsLimit, checkpointsLimit)
				if snapshotErr != nil {
					return "", snapshotErr
				}
				return buildCRShowHTMLDocument(embeddedCRShowHTMLTemplate, livePayload)
			},
			func() (map[string]any, error) {
				livePayload, _, snapshotErr := buildCRDashboardSnapshot(svc, model.CRSearchQuery{}, defaultCRListLimit, defaultCRTimelineLimit, id)
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
			newCRShowControlActionHandler(svc),
		)
		if err != nil {
			return commandError(cmd, asJSON, fmt.Errorf("start localhost preview: %w", err))
		}
		defer server.Shutdown()
		viewURL = server.URL
		openTarget := fmt.Sprintf("%s/%d", strings.TrimRight(viewURL, "/"), id)
		if err := openCRShowInBrowser(openTarget); err != nil {
			openErr = err.Error()
		} else {
			opened = true
			pageServed = server.WaitForFirstRender(20 * time.Second)
			if !pageServed {
				openErr = nonEmpty(openErr, "browser did not request localhost preview before timeout")
			}
		}
	}

	if asJSON {
		return writeJSONSuccess(cmd, map[string]any{
			"cr_id":           id,
			"cr_uid":          view.CR.UID,
			"view_mode":       "localhost_ephemeral",
			"url":             viewURL,
			"template_source": templateSource,
			"open_attempted":  openAttempted,
			"opened":          opened,
			"page_served":     pageServed,
			"close_reason":    closeReason,
			"open_error":      openErr,
			"generated_at":    payload["generated_at"],
		})
	}

	fmt.Fprintf(cmd.OutOrStdout(), "CR %d localhost preview prepared.\n", id)
	if strings.TrimSpace(viewURL) != "" {
		fmt.Fprintf(cmd.OutOrStdout(), "Preview URL: %s\n", viewURL)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Template source: %s\n", templateSource)
	if noOpen {
		fmt.Fprintln(cmd.OutOrStdout(), "Browser open skipped (--no-open).")
	} else if opened && pageServed {
		fmt.Fprintln(cmd.OutOrStdout(), "Opened report in your default browser.")
		fmt.Fprintln(cmd.OutOrStdout(), "Preview is live. Use the page's Close Preview button (or Ctrl+C) to stop the instance.")
		closeReason = waitForCRShowClose(server)
	} else if opened {
		fmt.Fprintf(cmd.OutOrStdout(), "Browser launch started, but no page request was observed in time.\n")
	} else {
		fmt.Fprintf(cmd.OutOrStdout(), "Could not open browser automatically: %s\n", nonEmpty(openErr, "unknown error"))
	}
	if strings.TrimSpace(closeReason) != "" {
		fmt.Fprintf(cmd.OutOrStdout(), "Preview closed (%s).\n", closeReason)
	}
	return nil
}

func runCRShowDashboard(cmd *cobra.Command, asJSON bool, noOpen bool, svc *service.Service, query model.CRSearchQuery, listLimit int, timelineLimit int, eventsLimit int, checkpointsLimit int, selectedHint int) error {
	payload, selectedCRID, err := buildCRDashboardSnapshot(svc, query, listLimit, timelineLimit, selectedHint)
	if err != nil {
		return commandError(cmd, asJSON, err)
	}
	const templateSource = "embedded:internal/cli/templates/cr_list.html"

	openAttempted := !noOpen
	opened := false
	pageServed := false
	openErr := ""
	viewURL := ""
	closeReason := ""
	var server *crShowServer
	if openAttempted {
		server, err = startCRShowServerWithLiveRoutesAndActions(
			func() (string, error) {
				livePayload, _, snapshotErr := buildCRDashboardSnapshot(svc, query, listLimit, timelineLimit, selectedHint)
				if snapshotErr != nil {
					return "", snapshotErr
				}
				return buildCRListHTMLDocument(embeddedCRListHTMLTemplate, livePayload)
			},
			func(routeCRID int) (string, error) {
				_, livePayload, snapshotErr := buildCRShowSnapshot(svc, routeCRID, eventsLimit, checkpointsLimit)
				if snapshotErr != nil {
					return "", snapshotErr
				}
				return buildCRShowHTMLDocument(embeddedCRShowHTMLTemplate, livePayload)
			},
			func() (map[string]any, error) {
				livePayload, _, snapshotErr := buildCRDashboardSnapshot(svc, query, listLimit, timelineLimit, selectedHint)
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
			newCRShowControlActionHandler(svc),
		)
		if err != nil {
			return commandError(cmd, asJSON, fmt.Errorf("start localhost preview: %w", err))
		}
		defer server.Shutdown()
		viewURL = server.URL
		openTarget := strings.TrimRight(viewURL, "/") + "/"
		if err := openCRShowInBrowser(openTarget); err != nil {
			openErr = err.Error()
		} else {
			opened = true
			pageServed = server.WaitForFirstRender(20 * time.Second)
			if !pageServed {
				openErr = nonEmpty(openErr, "browser did not request localhost preview before timeout")
			}
		}
	}

	dashboard := mapStringAny(payload["dashboard"])
	filters := mapStringAny(dashboard["filters"])
	counts := mapStringAny(dashboard["counts"])
	selectedValue := any(nil)
	if selectedCRID > 0 {
		selectedValue = selectedCRID
	}

	if asJSON {
		return writeJSONSuccess(cmd, map[string]any{
			"view_mode":       "localhost_dashboard",
			"url":             viewURL,
			"template_source": templateSource,
			"open_attempted":  openAttempted,
			"opened":          opened,
			"page_served":     pageServed,
			"close_reason":    closeReason,
			"open_error":      openErr,
			"generated_at":    payload["generated_at"],
			"selected_cr_id":  selectedValue,
			"filters":         filters,
			"counts":          counts,
		})
	}

	fmt.Fprintln(cmd.OutOrStdout(), "CR dashboard localhost preview prepared.")
	if strings.TrimSpace(viewURL) != "" {
		fmt.Fprintf(cmd.OutOrStdout(), "Preview URL: %s\n", viewURL)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Template source: %s\n", templateSource)
	if noOpen {
		fmt.Fprintln(cmd.OutOrStdout(), "Browser open skipped (--no-open).")
	} else if opened && pageServed {
		fmt.Fprintln(cmd.OutOrStdout(), "Opened dashboard in your default browser.")
		fmt.Fprintln(cmd.OutOrStdout(), "Preview is live. Use the page's Close Preview button (or Ctrl+C) to stop the instance.")
		closeReason = waitForCRShowClose(server)
	} else if opened {
		fmt.Fprintf(cmd.OutOrStdout(), "Browser launch started, but no page request was observed in time.\n")
	} else {
		fmt.Fprintf(cmd.OutOrStdout(), "Could not open browser automatically: %s\n", nonEmpty(openErr, "unknown error"))
	}
	if strings.TrimSpace(closeReason) != "" {
		fmt.Fprintf(cmd.OutOrStdout(), "Preview closed (%s).\n", closeReason)
	}
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
	payload["cr"] = crToJSONMap(view.CR)
	payload["generated_at"] = time.Now().UTC().Format(time.RFC3339)
	return view, payload, nil
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

	results, err := svc.SearchCRs(query)
	if err != nil {
		return nil, 0, err
	}
	allCRs, err := svc.ListCRs()
	if err != nil {
		return nil, 0, err
	}

	crByID := make(map[int]model.CR, len(allCRs))
	for _, cr := range allCRs {
		crByID[cr.ID] = cr
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
		cr, ok := crByID[result.ID]
		if !ok {
			continue
		}
		rows = append(rows, buildDashboardCRRow(result, cr))
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
		if cr, ok := crByID[selectedCRID]; ok {
			if result, hasResult := resultByID[selectedCRID]; hasResult {
				selected = buildDashboardSelectedCR(result, cr)
			} else {
				selected = buildDashboardSelectedCR(model.CRSearchResult{
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

	timelineItems := make([]dashboardTimelineEntry, 0)
	for _, cr := range allCRs {
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

func buildDashboardCRRow(result model.CRSearchResult, cr model.CR) map[string]any {
	lastEventAt := ""
	if n := len(cr.Events); n > 0 {
		lastEventAt = cr.Events[n-1].TS
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
		"parent_cr_id":        result.ParentCRID,
		"risk_tier":           result.RiskTier,
		"created_at":          result.CreatedAt,
		"updated_at":          result.UpdatedAt,
		"description":         cr.Description,
		"contract_why":        cr.Contract.Why,
		"contract_scope":      stringSliceOrEmpty(cr.Contract.Scope),
		"contract_non_goals":  stringSliceOrEmpty(cr.Contract.NonGoals),
		"contract_invariants": stringSliceOrEmpty(cr.Contract.Invariants),
		"last_event_at":       lastEventAt,
		"tasks": map[string]any{
			"total": result.TasksTotal,
			"open":  result.TasksOpen,
			"done":  result.TasksDone,
		},
	}
}

func buildDashboardSelectedCR(result model.CRSearchResult, cr model.CR) map[string]any {
	return buildDashboardCRRow(result, cr)
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

type crShowSnapshotRenderer func() (map[string]any, error)
type crShowCRSnapshotRenderer func(id int) (map[string]any, error)
type crShowControlActionHandler func(ctx context.Context, req crShowControlActionRequest) (int, map[string]any)

type crShowControlActionRequest struct {
	Action   string `json:"action"`
	CRID     int    `json:"cr_id"`
	Confirm  bool   `json:"confirm"`
	PullMode string `json:"pull_mode"`
}

func startCRShowServer(render func() (string, error)) (*crShowServer, error) {
	return startCRShowServerWithRoutes(render, nil)
}

func startCRShowServerWithRoutes(renderRoot func() (string, error), renderCR func(id int) (string, error)) (*crShowServer, error) {
	return startCRShowServerWithLiveRoutesAndActions(renderRoot, renderCR, nil, nil, nil)
}

func startCRShowServerWithLiveRoutes(
	renderRoot func() (string, error),
	renderCR func(id int) (string, error),
	snapshotRoot crShowSnapshotRenderer,
	snapshotCR crShowCRSnapshotRenderer,
) (*crShowServer, error) {
	return startCRShowServerWithLiveRoutesAndActions(renderRoot, renderCR, snapshotRoot, snapshotCR, nil)
}

func startCRShowServerWithLiveRoutesAndActions(
	renderRoot func() (string, error),
	renderCR func(id int) (string, error),
	snapshotRoot crShowSnapshotRenderer,
	snapshotCR crShowCRSnapshotRenderer,
	actionHandler crShowControlActionHandler,
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
	mux.HandleFunc("/__sophia_action", func(w http.ResponseWriter, r *http.Request) {
		if actionHandler == nil {
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req crShowControlActionRequest
		decoder := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&req); err != nil {
			writeCRShowActionJSON(w, http.StatusBadRequest, map[string]any{
				"ok":    false,
				"error": "invalid action payload",
				"hint":  strings.TrimSpace(err.Error()),
			})
			return
		}
		var extra map[string]any
		if err := decoder.Decode(&extra); err != io.EOF {
			writeCRShowActionJSON(w, http.StatusBadRequest, map[string]any{
				"ok":    false,
				"error": "invalid action payload",
				"hint":  "payload must be a single JSON object",
			})
			return
		}
		status, payload := actionHandler(r.Context(), req)
		if status == 0 {
			status = http.StatusOK
		}
		if payload == nil {
			payload = map[string]any{}
		}
		if _, ok := payload["ok"]; !ok {
			payload["ok"] = status < http.StatusBadRequest
		}
		writeCRShowActionJSON(w, status, payload)
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
			payload, snapshotErr := streamSnapshot()
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
		return snapshotRoot, nil
	case "cr":
		if snapshotCR == nil {
			return nil, fmt.Errorf("cr stream is unavailable")
		}
		id, err := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("id")))
		if err != nil || id <= 0 {
			return nil, fmt.Errorf("cr stream requires a valid id")
		}
		return func() (map[string]any, error) {
			return snapshotCR(id)
		}, nil
	default:
		return nil, fmt.Errorf("invalid stream mode")
	}
}

func writeCRShowActionJSON(w http.ResponseWriter, status int, payload map[string]any) {
	if status == 0 {
		status = http.StatusOK
	}
	if payload == nil {
		payload = map[string]any{}
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func newCRShowControlActionHandler(svc *service.Service) crShowControlActionHandler {
	if svc == nil {
		return nil
	}
	return func(ctx context.Context, req crShowControlActionRequest) (int, map[string]any) {
		action := normalizeCRShowControlAction(req.Action)
		if action == "" {
			return http.StatusBadRequest, map[string]any{
				"ok":    false,
				"error": "unsupported control action",
				"hint":  "expected publish_pr|pr_sync|refresh|validate|review|merge|pull",
			}
		}

		pullMode := ""
		if action == crShowActionPull {
			mode, modeErr := normalizeCRShowPullMode(req.PullMode)
			if modeErr != nil {
				return http.StatusBadRequest, map[string]any{
					"ok":     false,
					"action": action,
					"error":  modeErr.Error(),
				}
			}
			pullMode = mode
		}

		crID, crIDErr := resolveCRShowActionCRID(svc, req.CRID, action, pullMode)
		if crIDErr != nil {
			return http.StatusBadRequest, map[string]any{
				"ok":     false,
				"action": action,
				"error":  crIDErr.Error(),
				"hint":   "select a CR in the dashboard (or open a specific CR route) before retrying",
			}
		}

		if isCRShowRiskyAction(action) && !req.Confirm {
			return http.StatusConflict, map[string]any{
				"ok":                    false,
				"action":                action,
				"cr_id":                 crShowActionCRIDValue(crID),
				"requires_confirmation": true,
				"summary":               "confirmation required before running control action",
				"preflight":             buildCRShowActionPreflight(svc, action, crID),
			}
		}

		runResult, runErr := runCRShowControlAction(ctx, svc, action, crID, pullMode)
		if runErr != nil {
			status := crShowActionStatusForError(runErr)
			payload := map[string]any{
				"ok":      false,
				"action":  action,
				"cr_id":   crShowActionCRIDValue(crID),
				"summary": "control action failed",
				"error":   strings.TrimSpace(runErr.Error()),
				"hint":    crShowActionHintForError(runErr),
			}
			if runResult != nil {
				if summary, ok := runResult["summary"]; ok {
					payload["summary"] = summary
				}
				if result, ok := runResult["result"]; ok {
					payload["result"] = result
				}
				if mode, ok := runResult["pull_mode"]; ok {
					payload["pull_mode"] = mode
				}
			}
			return status, payload
		}
		if runResult == nil {
			runResult = map[string]any{}
		}
		runResult["ok"] = true
		runResult["action"] = action
		runResult["cr_id"] = crShowActionCRIDValue(crID)
		runResult["executed_at"] = time.Now().UTC().Format(time.RFC3339)
		return http.StatusOK, runResult
	}
}

func normalizeCRShowControlAction(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case crShowActionPublishPR:
		return crShowActionPublishPR
	case crShowActionPRSync:
		return crShowActionPRSync
	case crShowActionRefresh:
		return crShowActionRefresh
	case crShowActionValidate:
		return crShowActionValidate
	case crShowActionReview:
		return crShowActionReview
	case crShowActionMerge:
		return crShowActionMerge
	case crShowActionPull:
		return crShowActionPull
	default:
		return ""
	}
}

func normalizeCRShowPullMode(raw string) (string, error) {
	mode := strings.ToLower(strings.TrimSpace(raw))
	switch mode {
	case "", crShowPullModeMetadata, crShowPullModeFFOnly:
		return mode, nil
	default:
		return "", fmt.Errorf("invalid pull_mode %q (expected metadata|ff_only)", raw)
	}
}

func resolveCRShowActionCRID(svc *service.Service, reqCRID int, action string, pullMode string) (int, error) {
	if reqCRID > 0 {
		return reqCRID, nil
	}
	requiresCR := action != crShowActionPull || strings.TrimSpace(pullMode) == crShowPullModeMetadata
	if !requiresCR {
		return 0, nil
	}
	ctx, err := svc.CurrentCR()
	if err != nil {
		if errorsIs(err, service.ErrNoActiveCRContext) {
			return 0, fmt.Errorf("action %q requires a CR id", action)
		}
		return 0, err
	}
	if ctx == nil || ctx.CR == nil || ctx.CR.ID <= 0 {
		return 0, fmt.Errorf("action %q requires a CR id", action)
	}
	return ctx.CR.ID, nil
}

func buildCRShowActionPreflight(svc *service.Service, action string, crID int) map[string]any {
	preflight := map[string]any{
		"action": action,
	}
	if crID > 0 {
		preflight["cr_id"] = crID
		if statusView, err := svc.StatusCR(crID); err == nil {
			preflight["status"] = crStatusToJSONMap(statusView)
		} else {
			preflight["status_error"] = strings.TrimSpace(err.Error())
		}
	}
	if action == crShowActionMerge && crID > 0 {
		if validationView, err := svc.ValidateCR(crID); err == nil {
			preflight["validation"] = validationToJSONMap(validationView)
		} else {
			preflight["validation_error"] = strings.TrimSpace(err.Error())
		}
	}
	if action == crShowActionPublishPR && crID > 0 {
		if prView, err := svc.PRStatus(crID); err == nil {
			preflight["pr"] = prStatusToJSONMap(prView)
		} else {
			preflight["pr_error"] = strings.TrimSpace(err.Error())
		}
	}
	return preflight
}

func runCRShowControlAction(ctx context.Context, svc *service.Service, action string, crID int, pullMode string) (map[string]any, error) {
	switch action {
	case crShowActionPublishPR:
		prView, err := svc.PROpen(crID, true)
		if err != nil {
			return nil, err
		}
		return map[string]any{
			"summary": "Published PR link for current CR state",
			"result":  prStatusToJSONMap(prView),
		}, nil
	case crShowActionPRSync:
		prView, err := svc.PRSync(crID)
		if err != nil {
			return nil, err
		}
		return map[string]any{
			"summary": "Synced Sophia-managed PR body content",
			"result":  prStatusToJSONMap(prView),
		}, nil
	case crShowActionRefresh:
		view, err := svc.RefreshCR(crID, service.RefreshOptions{Strategy: service.RefreshStrategyAuto})
		if err != nil {
			return nil, err
		}
		return map[string]any{
			"summary": fmt.Sprintf("Refreshed CR %d with strategy %s", crID, strings.TrimSpace(view.Strategy)),
			"result":  crRefreshToJSONMap(view),
		}, nil
	case crShowActionValidate:
		report, err := svc.ValidateCR(crID)
		if err != nil {
			return nil, err
		}
		payload := validationToJSONMap(report)
		if report != nil && !report.Valid {
			return map[string]any{
				"summary": fmt.Sprintf("Validation failed for CR %d", crID),
				"result":  payload,
			}, fmt.Errorf("%w: %d error(s)", service.ErrCRValidationFailed, len(report.Errors))
		}
		return map[string]any{
			"summary": fmt.Sprintf("Validation passed for CR %d", crID),
			"result":  payload,
		}, nil
	case crShowActionReview:
		review, err := svc.ReviewCR(crID)
		if err != nil {
			return nil, err
		}
		return map[string]any{
			"summary": fmt.Sprintf("Built review context for CR %d", crID),
			"result":  reviewToJSONMap(review),
		}, nil
	case crShowActionMerge:
		result, err := svc.MergeCRWithOptions(crID, service.MergeCROptions{
			ApprovePROpen: true,
		})
		if err != nil {
			return nil, err
		}
		payload := map[string]any{
			"cr_id":           crID,
			"merged_commit":   strings.TrimSpace(result.MergedCommit),
			"warnings":        stringSliceOrEmpty(result.Warnings),
			"merge_mode":      strings.TrimSpace(result.MergeMode),
			"pr_url":          strings.TrimSpace(result.PRURL),
			"action":          strings.TrimSpace(result.Action),
			"action_reason":   strings.TrimSpace(result.ActionReason),
			"gate_blocked":    result.GateBlocked,
			"gate_reasons":    stringSliceOrEmpty(result.GateReasons),
			"override_reason": "",
		}
		summary := fmt.Sprintf("Processed merge action for CR %d", crID)
		if strings.TrimSpace(result.MergedCommit) != "" {
			summary = fmt.Sprintf("Merged CR %d as commit %s", crID, strings.TrimSpace(result.MergedCommit))
		} else if strings.TrimSpace(result.MergeMode) == "pr_gate" {
			summary = fmt.Sprintf("Prepared PR-gated merge flow for CR %d", crID)
		}
		return map[string]any{
			"summary": summary,
			"result":  payload,
		}, nil
	case crShowActionPull:
		mode := strings.TrimSpace(pullMode)
		if mode == "" {
			if crID > 0 {
				mode = crShowPullModeMetadata
			} else {
				mode = crShowPullModeFFOnly
			}
		}
		switch mode {
		case crShowPullModeMetadata:
			if crID <= 0 {
				return nil, fmt.Errorf("metadata pull requires a CR id")
			}
			pullResult, err := svc.PullCRFromHQ(strconv.Itoa(crID), false)
			if err != nil {
				return nil, err
			}
			return map[string]any{
				"summary":   fmt.Sprintf("Pulled metadata for CR %d from configured remote", crID),
				"pull_mode": mode,
				"result": map[string]any{
					"local_cr_id":          pullResult.LocalCRID,
					"cr_uid":               pullResult.CRUID,
					"created":              pullResult.Created,
					"updated":              pullResult.Updated,
					"local_ahead":          pullResult.LocalAhead,
					"up_to_date":           pullResult.UpToDate,
					"forced":               pullResult.Forced,
					"upstream_fingerprint": pullResult.UpstreamFingerprint,
				},
			}, nil
		case crShowPullModeFFOnly:
			output, err := runCRShowFFOnlyPull(ctx)
			if err != nil {
				if strings.TrimSpace(output) != "" {
					return nil, fmt.Errorf("%w: %s", err, strings.TrimSpace(output))
				}
				return nil, err
			}
			return map[string]any{
				"summary":   "Updated local branch with fast-forward only pull",
				"pull_mode": mode,
				"result": map[string]any{
					"command": "git pull --ff-only",
					"output":  strings.TrimSpace(output),
				},
			}, nil
		default:
			return nil, fmt.Errorf("invalid pull mode %q", mode)
		}
	default:
		return nil, fmt.Errorf("unsupported action %q", action)
	}
}

func runCRShowFFOnlyPull(ctx context.Context) (string, error) {
	command := exec.CommandContext(ctx, "git", "pull", "--ff-only")
	output, err := command.CombinedOutput()
	return string(output), err
}

func isCRShowRiskyAction(action string) bool {
	return action == crShowActionPublishPR || action == crShowActionMerge
}

func crShowActionCRIDValue(crID int) any {
	if crID <= 0 {
		return nil
	}
	return crID
}

func crShowActionStatusForError(err error) int {
	switch {
	case err == nil:
		return http.StatusOK
	case errorsIs(err, service.ErrNoActiveCRContext):
		return http.StatusBadRequest
	case errorsIs(err, service.ErrWorkingTreeDirty),
		errorsIs(err, service.ErrBranchInOtherWorktree),
		errorsIs(err, service.ErrMergeConflict),
		errorsIs(err, service.ErrMergeInProgress),
		errorsIs(err, service.ErrCRValidationFailed),
		errorsIs(err, service.ErrPRApprovalRequired),
		errorsIs(err, service.ErrPolicyViolation),
		errorsIs(err, service.ErrHQNotConfigured),
		errorsIs(err, service.ErrHQIntentDiverged),
		errorsIs(err, service.ErrHQUpstreamMoved),
		errorsIs(err, service.ErrParentCRRequired):
		return http.StatusConflict
	default:
		return http.StatusInternalServerError
	}
}

func crShowActionHintForError(err error) string {
	switch {
	case err == nil:
		return ""
	case errorsIs(err, service.ErrWorkingTreeDirty):
		return "working tree is dirty; commit/stash local changes and retry"
	case errorsIs(err, service.ErrBranchInOtherWorktree):
		return "target branch is open in another worktree; run action from that worktree path"
	case errorsIs(err, service.ErrPRApprovalRequired):
		return "confirm the action to allow PR creation/open"
	case errorsIs(err, service.ErrHQNotConfigured):
		return "configure a collaboration remote before metadata pull"
	case errorsIs(err, service.ErrHQIntentDiverged), errorsIs(err, service.ErrHQUpstreamMoved):
		return "retry with an explicit force workflow from CLI if remote/local intent diverged"
	default:
		return ""
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

func buildCRShowHTMLDocument(templateHTML string, payload map[string]any) (string, error) {
	title := "Sophia CR Report"
	if crRaw, ok := payload["cr"].(map[string]any); ok {
		crID := "-"
		switch v := crRaw["id"].(type) {
		case int:
			crID = strconv.Itoa(v)
		case int64:
			crID = strconv.FormatInt(v, 10)
		case float64:
			crID = strconv.Itoa(int(v))
		}
		crTitle := strings.TrimSpace(stringValue(crRaw["title"]))
		if crTitle == "" {
			crTitle = "Untitled CR"
		}
		title = fmt.Sprintf("CR-%s %s", crID, crTitle)
	}
	return buildCRShowDocument(templateHTML, payload, title)
}

func buildCRListHTMLDocument(templateHTML string, payload map[string]any) (string, error) {
	title := "Sophia CR Dashboard"
	if selected, ok := payload["selected_cr"].(map[string]any); ok {
		selectedTitle := strings.TrimSpace(stringValue(selected["title"]))
		if selectedTitle != "" {
			title = fmt.Sprintf("Sophia Dashboard · %s", selectedTitle)
		}
	}
	return buildCRShowDocument(templateHTML, payload, title)
}

func buildCRShowDocument(templateHTML string, payload map[string]any, title string) (string, error) {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	var escaped bytes.Buffer
	json.HTMLEscape(&escaped, encoded)

	generatedAt := strings.TrimSpace(stringValue(payload["generated_at"]))
	if generatedAt == "" {
		generatedAt = time.Now().UTC().Format(time.RFC3339)
	}

	doc := strings.TrimSpace(templateHTML)
	if doc == "" {
		return "", fmt.Errorf("template is empty")
	}
	doc = strings.ReplaceAll(doc, "__TITLE__", html.EscapeString(title))
	doc = strings.ReplaceAll(doc, "__GENERATED_AT__", html.EscapeString(generatedAt))
	doc = strings.ReplaceAll(doc, "__PAYLOAD_JSON__", escaped.String())
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
