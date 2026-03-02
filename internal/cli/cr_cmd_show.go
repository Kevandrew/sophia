package cli

import (
	"bytes"
	"context"
	_ "embed"
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
	crShowModePerCR        crShowMode = "per_cr"
	crShowModeDashboard    crShowMode = "dashboard"
	defaultCRListLimit                = 200
	defaultCRTimelineLimit            = 200
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
			return runCRShowDashboard(cmd, asJSON, noOpen, svc, query, listLimit, timelineLimit, selectedHint)
		},
	}

	cmd.Flags().BoolVar(&asJSON, "json", false, "Output in JSON format")
	cmd.Flags().BoolVar(&noOpen, "no-open", false, "Render report without opening a browser tab")
	cmd.Flags().IntVar(&eventsLimit, "events-limit", 20, "Maximum recent CR events to include")
	cmd.Flags().IntVar(&checkpointsLimit, "checkpoints-limit", 10, "Maximum recent task checkpoints to include")
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
		server, err = startCRShowServer(func() (string, error) {
			_, livePayload, snapshotErr := buildCRShowSnapshot(svc, id, eventsLimit, checkpointsLimit)
			if snapshotErr != nil {
				return "", snapshotErr
			}
			return buildCRShowHTMLDocument(embeddedCRShowHTMLTemplate, livePayload)
		})
		if err != nil {
			return commandError(cmd, asJSON, fmt.Errorf("start localhost preview: %w", err))
		}
		defer server.Shutdown()
		viewURL = server.URL
		if err := openCRShowInBrowser(viewURL); err != nil {
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

func runCRShowDashboard(cmd *cobra.Command, asJSON bool, noOpen bool, svc *service.Service, query model.CRSearchQuery, listLimit int, timelineLimit int, selectedHint int) error {
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
		server, err = startCRShowServer(func() (string, error) {
			livePayload, _, snapshotErr := buildCRDashboardSnapshot(svc, query, listLimit, timelineLimit, selectedHint)
			if snapshotErr != nil {
				return "", snapshotErr
			}
			return buildCRListHTMLDocument(embeddedCRListHTMLTemplate, livePayload)
		})
		if err != nil {
			return commandError(cmd, asJSON, fmt.Errorf("start localhost preview: %w", err))
		}
		defer server.Shutdown()
		viewURL = server.URL
		if err := openCRShowInBrowser(viewURL); err != nil {
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

func startCRShowServer(render func() (string, error)) (*crShowServer, error) {
	if render == nil {
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
