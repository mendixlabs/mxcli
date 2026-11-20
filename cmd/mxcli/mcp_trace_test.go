// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/mendixlabs/mxcli/internal/testutil"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// mendixlabs/mxcli#1684 (ako/mxcli#1833): "`--mcp-trace` lists no tool call",
// read as proof that `exec --mcp` never reached Studio Pro. The calls were
// made; the trace could not show them. The executor got the tracer and printed
// each statement, but the MCP backend the factory built never did
// (mcpbackend.WithTracer had no caller), so its client printed no call lines.

// fakeStudioPro answers the PED tools a `create constant` makes and records
// the tool names it was asked for.
func fakeStudioPro(t *testing.T) (*httptest.Server, func() []string) {
	t.Helper()
	var mu sync.Mutex
	var calls []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req struct {
			ID     *int   `json:"id"`
			Method string `json:"method"`
			Params struct {
				Name string `json:"name"`
			} `json:"params"`
		}
		_ = json.Unmarshal(body, &req)
		w.Header().Set("Mcp-Session-Id", "test")
		if req.ID == nil {
			w.WriteHeader(http.StatusAccepted)
			return
		}
		var result any
		switch req.Method {
		case "initialize":
			result = map[string]any{"protocolVersion": "2025-06-18",
				"serverInfo": map[string]any{"name": "fake-studio-pro", "version": "1.0.0"}}
		case "tools/list":
			result = map[string]any{"tools": []any{}}
		case "tools/call":
			mu.Lock()
			calls = append(calls, req.Params.Name)
			mu.Unlock()
			text := "SUCCESS"
			if req.Params.Name == "ped_check_errors" {
				text = "No errors found."
			}
			result = map[string]any{"isError": false, "content": []map[string]any{{"type": "text", "text": text}}}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": result})
	}))
	t.Cleanup(srv.Close)
	return srv, func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), calls...)
	}
}

// withMCPFlags sets the --mcp globals for one test.
func withMCPFlags(t *testing.T, url, dial string, trace bool) {
	t.Helper()
	saved := []any{globalMCPURL, globalMCPDial, globalMCPTrace, globalMCPVerbose}
	globalMCPURL, globalMCPDial, globalMCPTrace, globalMCPVerbose = url, dial, trace, false
	t.Cleanup(func() {
		globalMCPURL, globalMCPDial = saved[0].(string), saved[1].(string)
		globalMCPTrace, globalMCPVerbose = saved[2].(bool), saved[3].(bool)
	})
}

// stderrOf runs fn with os.Stderr captured; the tracers read os.Stderr when
// they are built, so the executor must be built inside fn.
func stderrOf(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	saved := os.Stderr
	os.Stderr = w
	var buf bytes.Buffer
	done := make(chan struct{})
	go func() { _, _ = io.Copy(&buf, r); close(done) }()
	defer func() {
		os.Stderr = saved
		_ = w.Close()
		<-done
	}()
	fn()
	_ = w.Close()
	<-done
	os.Stderr = saved
	return buf.String()
}

func TestMCPTrace_ListsThePEDCallsOfAWrite(t *testing.T) {
	srv, calls := fakeStudioPro(t)
	withMCPFlags(t, srv.URL+"/mcp", strings.TrimPrefix(srv.URL, "http://"), true)
	dst := t.TempDir()
	if err := testutil.CopyFixture(dst, "../../testdata/expr-checker"); err != nil {
		t.Fatalf("copy fixture: %v", err)
	}

	var out bytes.Buffer
	stderr := stderrOf(t, func() {
		exec, logger := newLoggedExecutorTo("test", &out)
		defer logger.Close()
		defer exec.Close()
		prog, errs := visitor.Build("CONNECT LOCAL '" + visitor.QuoteString(filepath.Join(dst, "minimal.mpr")) + "';\n" +
			"create constant MyFirstModule.TraceProbe (Type: String, DefaultValue: 'x');")
		if len(errs) > 0 {
			t.Fatal(errs)
		}
		for _, stmt := range prog.Statements {
			if err := exec.Execute(stmt); err != nil {
				t.Fatalf("%v\n%s", err, out.String())
			}
		}
	})

	got := calls()
	if len(got) == 0 || !contains(got, "ped_create_document") {
		t.Fatalf("the fake Studio Pro received %v, want a ped_create_document", got)
	}
	if !strings.Contains(stderr, "▸ CreateConstantStmt") {
		t.Fatalf("--mcp-trace printed no statement header, so tracing is off altogether:\n%s", stderr)
	}
	for _, tool := range []string{"ped_create_document", "ped_check_errors"} {
		if !strings.Contains(stderr, "→ "+tool) {
			t.Errorf("--mcp-trace does not list %s, which Studio Pro received (%v):\n%s", tool, got, stderr)
		}
	}
}

func contains(xs []string, x string) bool {
	for _, s := range xs {
		if s == x {
			return true
		}
	}
	return false
}
