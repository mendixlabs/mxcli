// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// Running the app goes through a Studio Pro 11.16 handler, not a tool: the
// server advertises execute_handler and query_handlers, and mendix_app (run,
// stop, status) is one of the handlers behind them. Studio Pro 11.12 has
// neither tool, so every call here is gated on their presence.

const (
	mendixAppHandler = "mendix_app"
	// appRunTimeoutSeconds is the longest wait mendix_app accepts (schema max
	// 120). A timeout does not cancel the run; status still reports it.
	appRunTimeoutSeconds = 120
	// appLogPath is where Studio Pro serves the runtime log (read_file).
	appLogPath = "/app-logs/app.log"
)

// AppStatus is mendix_app's status reply, measured on 11.16.0:
// {"running":false,"launching":false,"appUrl":"http://localhost:8080/"}.
type AppStatus struct {
	Running   bool   `json:"running"`
	Launching bool   `json:"launching"`
	AppURL    string `json:"appUrl"`
}

// requireHandler reports an actionable error unless the server exposes the
// named handler. Presence is asked once per session with query_handlers
// getDetails, which execute_handler's own description requires before a call.
func (b *Backend) requireHandler(name string) error {
	if b.client == nil || !b.client.HasTool("execute_handler") || !b.client.HasTool("query_handlers") {
		return fmt.Errorf("%s needs Studio Pro 11.16+ (execute_handler); this Studio Pro's MCP server has no handlers", name)
	}
	if ok, cached := b.handlers[name]; cached {
		if !ok {
			return fmt.Errorf("this Studio Pro offers no %s handler", name)
		}
		return nil
	}
	res, err := b.client.CallTool("query_handlers", map[string]any{
		"operation": "getDetails",
		"names":     []string{name},
	})
	if err != nil {
		return err
	}
	text := pedStripReminder(res.Text)
	if res.IsError {
		return fmt.Errorf("query_handlers: %s", text)
	}
	var details struct {
		Handlers []struct {
			Name string `json:"name"`
		} `json:"handlers"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(text)), &details); err != nil {
		return fmt.Errorf("query_handlers: parsing result: %w", err)
	}
	found := false
	for _, h := range details.Handlers {
		found = found || h.Name == name
	}
	if b.handlers == nil {
		b.handlers = map[string]bool{}
	}
	b.handlers[name] = found
	if !found {
		return fmt.Errorf("this Studio Pro offers no %s handler", name)
	}
	return nil
}

// executeHandler runs a handler and returns its reply text. A handler that
// throws answers "Error executing tool handler: …", which CallTool already
// classifies as an error.
func (b *Backend) executeHandler(name string, payload map[string]any) (string, error) {
	if err := b.requireHandler(name); err != nil {
		return "", err
	}
	res, err := b.client.CallTool("execute_handler", map[string]any{"name": name, "payload": payload})
	if err != nil {
		return "", err
	}
	text := pedStripReminder(res.Text)
	if res.IsError {
		return "", fmt.Errorf("%s: %s", name, strings.TrimSpace(text))
	}
	return text, nil
}

func parseAppStatus(text string) (*AppStatus, bool) {
	var s AppStatus
	if err := json.Unmarshal([]byte(strings.TrimSpace(text)), &s); err != nil {
		return nil, false
	}
	return &s, true
}

// GetAppStatus reports whether the app is running or launching, and its URL,
// without disturbing a running session.
func (b *Backend) GetAppStatus() (*AppStatus, error) {
	text, err := b.executeHandler(mendixAppHandler, map[string]any{"action": "status"})
	if err != nil {
		return nil, err
	}
	s, ok := parseAppStatus(text)
	if !ok {
		return nil, fmt.Errorf("%s status: unexpected reply: %s", mendixAppHandler, strings.TrimSpace(text))
	}
	return s, nil
}

// RunApp starts (or restarts) the app — Studio Pro's Run button — and waits up
// to timeoutSeconds (0 means appRunTimeoutSeconds) for it to become ready.
//
// mendix_app refuses to run a project with model errors and lists them instead.
// Its run reply has not been observed live, so a reply that is not a status is
// settled by looking: the run took iff status then says running or launching.
func (b *Backend) RunApp(timeoutSeconds int) (*AppStatus, error) {
	if timeoutSeconds <= 0 {
		timeoutSeconds = appRunTimeoutSeconds
	}
	text, err := b.executeHandler(mendixAppHandler, map[string]any{"action": "run", "timeoutSeconds": timeoutSeconds})
	if err != nil {
		return nil, fmt.Errorf("%w (runtime log: read_file %s)", err, appLogPath)
	}
	if s, ok := parseAppStatus(text); ok {
		return s, nil
	}
	if s, serr := b.GetAppStatus(); serr == nil && (s.Running || s.Launching) {
		return s, nil
	}
	return nil, fmt.Errorf("%s run: %s (runtime log: read_file %s)", mendixAppHandler, strings.TrimSpace(text), appLogPath)
}

// StopApp stops the running app — Studio Pro's Stop button.
func (b *Backend) StopApp() error {
	_, err := b.executeHandler(mendixAppHandler, map[string]any{"action": "stop"})
	return err
}

// writeAppStatus prints one line on where the app stands after a run.
func writeAppStatus(w io.Writer, s *AppStatus) {
	switch {
	case s.Running:
		fmt.Fprintf(w, "app running at %s\n", s.AppURL)
	case s.Launching:
		fmt.Fprintf(w, "app still launching at %s (check again with mendix_app status)\n", s.AppURL)
	default:
		fmt.Fprintf(w, "app is not running (runtime log: read_file %s)\n", appLogPath)
	}
}
