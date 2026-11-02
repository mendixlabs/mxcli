// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"strings"
	"testing"
)

// The 11.16 surface that carries handlers; 11.12 advertises neither tool.
var handlerTools = map[string][]string{
	"query_handlers":  {"operation", "keyword", "names"},
	"execute_handler": {"name", "payload"},
}

// mendixAppDetails is the shape of 11.16.0's getDetails reply (handlers-detail
// probe), trimmed to what requireHandler reads.
const mendixAppDetails = `{"handlers":[{"name":"mendix_app","description":"Controls the Mendix application runtime."}],"errors":[]}`

// fakeMendixApp answers query_handlers and execute_handler mendix_app with the
// given reply per action.
func fakeMendixApp(t *testing.T, replies map[string]string) *fakePED {
	f := newFakePED(t, func(name string, args map[string]any) (string, bool) {
		switch name {
		case "query_handlers":
			return mendixAppDetails, false
		case "execute_handler":
			if args["name"] != "mendix_app" {
				return "Error executing tool handler: Handler '" + args["name"].(string) + "' not found.", false
			}
			payload, _ := args["payload"].(map[string]any)
			action, _ := payload["action"].(string)
			if r, ok := replies[action]; ok {
				return r, false
			}
		}
		return "unexpected " + name, true
	})
	f.tools = handlerTools
	return f
}

func TestGetAppStatus_ParsesMeasuredReply(t *testing.T) {
	f := fakeMendixApp(t, map[string]string{
		"status": `{"running":false,"launching":false,"appUrl":"http://localhost:8080/"}`,
	})
	b := &Backend{client: f.connectClient(t)}

	s, err := b.GetAppStatus()
	if err != nil {
		t.Fatalf("GetAppStatus: %v", err)
	}
	if s.Running || s.Launching || s.AppURL != "http://localhost:8080/" {
		t.Errorf("status = %+v", s)
	}
	q, ok := f.callByName("query_handlers")
	if !ok || q.Args["operation"] != "getDetails" {
		t.Fatalf("handler presence must be asked with getDetails first, got %+v", q)
	}
	call, _ := f.callByName("execute_handler")
	if payload, _ := call.Args["payload"].(map[string]any); payload["action"] != "status" {
		t.Errorf("payload = %v, want action status", call.Args["payload"])
	}
}

func TestRunApp_SendsTimeoutAndReportsURL(t *testing.T) {
	f := fakeMendixApp(t, map[string]string{
		"run": `{"running":true,"launching":false,"appUrl":"http://localhost:8080/"}`,
	})
	b := &Backend{client: f.connectClient(t)}

	s, err := b.RunApp(0)
	if err != nil {
		t.Fatalf("RunApp: %v", err)
	}
	if !s.Running {
		t.Errorf("status = %+v, want running", s)
	}
	call, _ := f.callByName("execute_handler")
	payload, _ := call.Args["payload"].(map[string]any)
	if payload["action"] != "run" || payload["timeoutSeconds"] != float64(appRunTimeoutSeconds) {
		t.Errorf("payload = %v, want run with timeoutSeconds %d", payload, appRunTimeoutSeconds)
	}
	var sb strings.Builder
	writeAppStatus(&sb, s)
	if sb.String() != "app running at http://localhost:8080/\n" {
		t.Errorf("report = %q", sb.String())
	}
}

// mendix_app refuses to run a project with model errors and lists them instead;
// that text is not a status, and status then shows nothing running.
func TestRunApp_RefusalIsAnError(t *testing.T) {
	f := fakeMendixApp(t, map[string]string{
		"run":    "The app cannot be run: the project has 2 errors.",
		"status": `{"running":false,"launching":false,"appUrl":"http://localhost:8080/"}`,
	})
	b := &Backend{client: f.connectClient(t)}

	_, err := b.RunApp(0)
	if err == nil || !strings.Contains(err.Error(), "the project has 2 errors") || !strings.Contains(err.Error(), appLogPath) {
		t.Fatalf("a refused run should carry the reply and point at the log, got: %v", err)
	}
}

// A run reply that is not a status but did start the app is settled by status.
func TestRunApp_UnrecognisedReplySettledByStatus(t *testing.T) {
	f := fakeMendixApp(t, map[string]string{
		"run":    "Started.",
		"status": `{"running":false,"launching":true,"appUrl":"http://localhost:8080/"}`,
	})
	b := &Backend{client: f.connectClient(t)}

	s, err := b.RunApp(0)
	if err != nil {
		t.Fatalf("RunApp: %v", err)
	}
	if !s.Launching {
		t.Errorf("status = %+v, want launching", s)
	}
}

func TestStopApp_HandlerFailureSurfaces(t *testing.T) {
	f := fakeMendixApp(t, map[string]string{"stop": handlerSessionError})
	b := &Backend{client: f.connectClient(t)}

	if err := b.StopApp(); err == nil || !strings.Contains(err.Error(), "Session 'mcp-server' not found") {
		t.Fatalf("a handler that throws must be an error, got: %v", err)
	}
}

func TestAppHandler_AbsentOn1112(t *testing.T) {
	f := newFakePED(t, nil)
	f.tools = map[string][]string{"ped_create_document": {"documents"}} // 11.12 surface
	b := &Backend{client: f.connectClient(t)}

	if _, err := b.RunApp(0); err == nil || !strings.Contains(err.Error(), "needs Studio Pro 11.16+") {
		t.Fatalf("without execute_handler RunApp should name the version, got: %v", err)
	}
	if len(f.calls) != 0 {
		t.Errorf("no tool may be called on a server without handlers, got %+v", f.calls)
	}
}

func TestAppHandler_NotOffered(t *testing.T) {
	f := newFakePED(t, func(name string, _ map[string]any) (string, bool) {
		return `{"handlers":[],"errors":[]}`, false
	})
	f.tools = handlerTools
	b := &Backend{client: f.connectClient(t)}

	if _, err := b.GetAppStatus(); err == nil || !strings.Contains(err.Error(), "no mendix_app handler") {
		t.Fatalf("got: %v", err)
	}
	if _, ok := f.callByName("execute_handler"); ok {
		t.Error("execute_handler must not be called for a handler query_handlers did not list")
	}
	// Cached: a second call asks nothing more.
	n := len(f.calls)
	_, _ = b.GetAppStatus()
	if len(f.calls) != n {
		t.Errorf("handler presence should be cached for the session")
	}
}

func TestCheckProject(t *testing.T) {
	listing := "Listing problems 1-2 (out of 2). Check ID: 6\n" +
		"'MyFirstModule.Zz_BrokenEnum' (Enumerations$Enumeration):\n" +
		"- [error] Missing enumeration values. (at locations: -)\n" +
		"'MyFirstModule' (DomainModels$DomainModel):\n" +
		"- [error] Duplicate name\n"
	f := newFakePED(t, func(string, map[string]any) (string, bool) { return listing, false })
	f.tools = map[string][]string{"ped_check_errors": {"filters", "pagination"}}
	b := &Backend{client: f.connectClient(t)}

	got, err := b.CheckProject()
	if err != nil {
		t.Fatalf("CheckProject: %v", err)
	}
	if len(got) != 4 || got[0] != "'MyFirstModule.Zz_BrokenEnum' (Enumerations$Enumeration):" {
		t.Fatalf("listing = %q", got)
	}
	call, _ := f.callByName("ped_check_errors")
	if len(call.Args) != 0 {
		t.Errorf("a project-wide check sends no filters, got %v", call.Args)
	}
	var sb strings.Builder
	writeCheckReport(&sb, got)
	if !strings.Contains(sb.String(), "  - [error] Duplicate name") {
		t.Errorf("report:\n%s", sb.String())
	}
}

func TestCheckProject_NeedsFilters(t *testing.T) {
	f := newFakePED(t, nil)
	f.tools = map[string][]string{"ped_check_errors": {"documents"}} // up to 11.14
	b := &Backend{client: f.connectClient(t)}

	if _, err := b.CheckProject(); err == nil || !strings.Contains(err.Error(), "11.15+") {
		t.Fatalf("got: %v", err)
	}
}
