// SPDX-License-Identifier: Apache-2.0

package testrunner

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// fakeEndpoint stands in for the handler the Java action registers, enforcing
// the same token gate. It lets the Go side of the contract — header name,
// routes, response shape, status codes — be tested without a Mendix runtime.
type fakeEndpoint struct {
	token string
	flows map[string]runResponse
	// seenTokens records what each request presented, so a test can assert the
	// client actually sends the token rather than the server merely allowing it.
	seenTokens []string
	// rollbackParams records the rollback query parameter of each run request.
	rollbackParams []string
}

func (f *fakeEndpoint) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		presented := r.Header.Get(endpointTokenHeader)
		f.seenTokens = append(f.seenTokens, presented)
		w.Header().Set("Content-Type", "application/json")

		if presented != f.token {
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprint(w, `{"error":"unauthorized"}`)
			return
		}

		switch {
		case strings.HasSuffix(r.URL.Path, "/list"):
			names := []string{}
			for name := range f.flows {
				if p := r.URL.Query().Get("prefix"); p == "" || strings.HasPrefix(name, p) {
					names = append(names, name)
				}
			}
			json.NewEncoder(w).Encode(listResponse{Microflows: names})
		case strings.HasSuffix(r.URL.Path, "/run"):
			f.rollbackParams = append(f.rollbackParams, r.URL.Query().Get("rollback"))
			mf := r.URL.Query().Get("mf")
			resp, ok := f.flows[mf]
			if !ok {
				w.WriteHeader(http.StatusNotFound)
				fmt.Fprintf(w, `{"error":"unknown microflow","mf":%q}`, mf)
				return
			}
			json.NewEncoder(w).Encode(resp)
		default:
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprint(w, `{"error":"no such route"}`)
		}
	})
}

// newFakeEndpoint starts the fake and returns a client pointed at it.
func newFakeEndpoint(t *testing.T, token string, flows map[string]runResponse) (*fakeEndpoint, *endpointClient) {
	t.Helper()
	fake := &fakeEndpoint{token: token, flows: flows}
	srv := httptest.NewServer(fake.handler())
	t.Cleanup(srv.Close)

	c := newEndpointClient(0, token)
	c.baseURL = srv.URL + "/" + endpointPath
	return fake, c
}

func TestClientSendsTheToken(t *testing.T) {
	fake, c := newFakeEndpoint(t, "s3cret", map[string]runResponse{
		testFlowPrefix + "test_1": {OK: true, Result: verdictPass},
	})

	if _, err := c.list(); err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(fake.seenTokens) != 1 || fake.seenTokens[0] != "s3cret" {
		t.Errorf("server saw tokens %q, want one request presenting %q", fake.seenTokens, "s3cret")
	}
}

// TestClientReportsAnUnauthorizedGateClearly pins that a rejected token is
// reported as a token problem, not as a JSON decoding failure — the gate
// rejecting mxcli is a bug in how the token was passed, and the message has to
// say so.
func TestClientReportsAnUnauthorizedGateClearly(t *testing.T) {
	_, c := newFakeEndpoint(t, "the-real-token", nil)
	c.token = "the-wrong-token"

	_, err := c.list()
	if err == nil {
		t.Fatal("list with a wrong token succeeded")
	}
	if !strings.Contains(err.Error(), "rejected the token") {
		t.Errorf("error %q does not explain that the token was rejected", err)
	}
}

func TestClientListFiltersToTestFlows(t *testing.T) {
	_, c := newFakeEndpoint(t, "t", map[string]runResponse{
		testFlowPrefix + "test_1":   {},
		"MyModule.SomethingElse":    {},
		testFlowPrefix + "test_222": {},
	})

	names, err := c.list()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	for _, n := range names {
		if !strings.HasPrefix(n, testFlowPrefix) {
			t.Errorf("list returned a non-test microflow: %q", n)
		}
	}
	if len(names) != 2 {
		t.Errorf("got %d test microflows, want 2: %q", len(names), names)
	}
}

func TestToResult(t *testing.T) {
	tc := TestCase{ID: "test_1", Name: "a test"}

	tests := []struct {
		name       string
		resp       runResponse
		wantStatus TestStatus
		wantMsg    string
	}{
		{
			name:       "pass",
			resp:       runResponse{OK: true, Result: verdictPass, DurationMicros: 1500},
			wantStatus: StatusPass,
		},
		{
			name:       "assertion failure is a FAIL",
			resp:       runResponse{OK: true, Result: verdictFailPrefix + "expected $r = 'x'"},
			wantStatus: StatusFail,
			wantMsg:    "expected $r = 'x'",
		},
		{
			// The verdict the generated handler used to return (#1874).
			name:       "a thrown test microflow is a FAIL naming the error",
			resp:       runResponse{OK: false, Error: "NullPointerException"},
			wantStatus: StatusFail,
			wantMsg:    "exception during execution: NullPointerException",
		},
		{
			name:       "a throw with no message still says something",
			resp:       runResponse{OK: false},
			wantStatus: StatusFail,
			wantMsg:    "exception during execution: microflow threw, but the runtime reported no message",
		},
		{
			name:       "an unrecognised verdict is an ERROR",
			resp:       runResponse{OK: true, Result: "who knows"},
			wantStatus: StatusError,
			wantMsg:    `unrecognised verdict from the test microflow: "who knows"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := toResult(tc, &tt.resp)
			if got.Status != tt.wantStatus {
				t.Errorf("status = %v, want %v", got.Status, tt.wantStatus)
			}
			if got.Message != tt.wantMsg {
				t.Errorf("message = %q, want %q", got.Message, tt.wantMsg)
			}
			if got.ID != tc.ID || got.Name != tc.Name {
				t.Errorf("identity not carried over: got %q/%q", got.ID, got.Name)
			}
		})
	}
}

// TestToResult_ThrownTestMicroflow pins @throws: the error leaves the test
// microflow, so the comparison with the annotation is the runner's (#1874).
// Measured on 11.16.1 against the generated handler it replaces: the same PASS
// and the same FAIL text, `actual: For input string: "abc"`.
func TestToResult_ThrownTestMicroflow(t *testing.T) {
	thrown := &runResponse{OK: false, Error: `For input string: "abc"`}
	for _, c := range []struct {
		throws, wantMsg string
		bare            bool
		want            TestStatus
	}{
		{throws: "abc", want: StatusPass},
		{bare: true, want: StatusPass},
		{throws: "no such text", want: StatusFail,
			wantMsg: `expected an error containing 'no such text', actual: For input string: "abc"`},
	} {
		tc := TestCase{ID: "t", Name: "n", Throws: c.throws}
		if c.bare {
			tc.ExpectsThrow = true
		}
		got := toResult(tc, thrown)
		if got.Status != c.want || got.Message != c.wantMsg {
			t.Errorf("@throws %q: got %v %q, want %v %q", c.throws, got.Status, got.Message, c.want, c.wantMsg)
		}
	}
}

func TestToResultCarriesDuration(t *testing.T) {
	got := toResult(TestCase{ID: "test_1"}, &runResponse{OK: true, Result: verdictPass, DurationMicros: 2500})
	if got.Duration != 2500*time.Microsecond {
		t.Errorf("duration = %v, want 2.5ms", got.Duration)
	}
}

// TestClientUsesNoProxy pins that the loopback call cannot be diverted. An
// HTTP_PROXY in the environment is common in container and CI images, and would
// otherwise send the token to the proxy.
func TestClientUsesNoProxy(t *testing.T) {
	c := newEndpointClient(8081, "tok")
	tr, ok := c.http.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("transport is %T, want *http.Transport", c.http.Transport)
	}
	if tr.Proxy != nil {
		t.Error("the client honours a proxy; the token could leave the machine")
	}
}

func TestWaitReadyGivesUp(t *testing.T) {
	// Port 1 on loopback: nothing listens, and connections fail fast.
	c := newEndpointClient(1, "tok")
	start := time.Now()
	err := c.waitReady(600 * time.Millisecond)
	if err == nil {
		t.Fatal("waitReady succeeded against a dead port")
	}
	if elapsed := time.Since(start); elapsed > 10*time.Second {
		t.Errorf("waitReady took %v; it should honour its timeout", elapsed)
	}
	if !strings.Contains(err.Error(), "did not come up") {
		t.Errorf("error %q does not explain the endpoint never came up", err)
	}
}

// TestClientSendsTheRollbackParameter pins that the runner's per-test decision
// actually reaches the endpoint. Without the parameter the endpoint commits, and
// a test annotated for rollback would silently leave its data behind.
func TestClientSendsTheRollbackParameter(t *testing.T) {
	fake, c := newFakeEndpoint(t, "tok", map[string]runResponse{
		testFlowPrefix + "test_1": {OK: true, Result: verdictPass},
	})

	if _, err := c.run(testFlowPrefix+"test_1", true); err != nil {
		t.Fatalf("run with rollback: %v", err)
	}
	if _, err := c.run(testFlowPrefix+"test_1", false); err != nil {
		t.Fatalf("run without rollback: %v", err)
	}

	if len(fake.rollbackParams) != 2 {
		t.Fatalf("server saw %d run requests, want 2", len(fake.rollbackParams))
	}
	if fake.rollbackParams[0] != "1" {
		t.Errorf("rollback run sent rollback=%q, want \"1\"", fake.rollbackParams[0])
	}
	if fake.rollbackParams[1] != "" {
		t.Errorf("non-rollback run sent rollback=%q, want it absent", fake.rollbackParams[1])
	}
}
