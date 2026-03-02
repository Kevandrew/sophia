package cli

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"html"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/spf13/cobra"

	"sophia/internal/service"
)

func newCRShowCmd() *cobra.Command {
	var (
		asJSON           bool
		noOpen           bool
		eventsLimit      int
		checkpointsLimit int
	)

	cmd := &cobra.Command{
		Use:   "show [id]",
		Short: "Render a read-only CR report and open it in a localhost browser view",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withOptionalCRIDAndService(cmd, asJSON, args, "id", func(id int, svc *service.Service) error {
				view, err := svc.PackCR(id, service.PackOptions{
					EventsLimit:      eventsLimit,
					CheckpointsLimit: checkpointsLimit,
				})
				if err != nil {
					return commandError(cmd, asJSON, err)
				}
				if view == nil || view.CR == nil {
					return commandError(cmd, asJSON, fmt.Errorf("cr %d is unavailable", id))
				}

				payload := crPackToJSONMap(view)
				payload["cr"] = crToJSONMap(view.CR)
				payload["generated_at"] = time.Now().UTC().Format(time.RFC3339)
				const templateSource = "embedded:internal/cli/templates/cr_show.html"
				htmlDoc, err := buildCRShowHTMLDocument(embeddedCRShowHTMLTemplate, payload)
				if err != nil {
					return commandError(cmd, asJSON, fmt.Errorf("build CR show HTML: %w", err))
				}

				openAttempted := !noOpen
				opened := false
				pageServed := false
				openErr := ""
				viewURL := ""
				closeReason := ""
				var server *crShowServer
				if openAttempted {
					server, err = startCRShowServer(htmlDoc)
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
			})
		},
	}

	cmd.Flags().BoolVar(&asJSON, "json", false, "Output in JSON format")
	cmd.Flags().BoolVar(&noOpen, "no-open", false, "Render report without opening a browser tab")
	cmd.Flags().IntVar(&eventsLimit, "events-limit", 20, "Maximum recent CR events to include")
	cmd.Flags().IntVar(&checkpointsLimit, "checkpoints-limit", 10, "Maximum recent task checkpoints to include")
	return cmd
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

func startCRShowServer(htmlDoc string) (*crShowServer, error) {
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
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(htmlDoc))
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
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	var escaped bytes.Buffer
	json.HTMLEscape(&escaped, encoded)

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
