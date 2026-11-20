// SPDX-License-Identifier: Apache-2.0

package testrunner

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// endpointClient talks to the test endpoint registered inside the running app.
type endpointClient struct {
	baseURL string
	token   string
	http    *http.Client
}

// newEndpointClient returns a client for the app serving on port.
//
// The transport deliberately takes no proxy: the address is always loopback, and
// an HTTP_PROXY in the environment (this is common in container and CI images)
// would otherwise send the token to the proxy.
func newEndpointClient(port int, token string) *endpointClient {
	return &endpointClient{
		baseURL: fmt.Sprintf("http://127.0.0.1:%d/%s", port, endpointPath),
		token:   token,
		http: &http.Client{
			Timeout: 10 * time.Minute,
			Transport: &http.Transport{
				Proxy:               nil,
				DialContext:         (&net.Dialer{Timeout: 5 * time.Second}).DialContext,
				TLSHandshakeTimeout: 5 * time.Second,
			},
		},
	}
}

// runResponse is the endpoint's reply to a run request.
type runResponse struct {
	MF             string `json:"mf"`
	OK             bool   `json:"ok"`
	DurationMicros int64  `json:"durationMicros"`
	Result         string `json:"result"`
	Error          string `json:"error"`
	// RollbackRequested echoes back whether the runner asked for a rollback, so a
	// runner talking to an older endpoint that ignores the parameter can tell.
	RollbackRequested bool `json:"rollbackRequested"`
	// RolledBack reports that the transaction was actually rolled back.
	RolledBack bool `json:"rolledBack"`
	// RollbackError is why it was not.
	RollbackError string `json:"rollbackError"`
}

// listResponse is the endpoint's reply to a list request.
type listResponse struct {
	Microflows []string `json:"microflows"`
	Error      string   `json:"error"`
}

// get performs one authenticated GET and decodes the JSON body into out.
func (c *endpointClient) get(route string, params url.Values, out any) error {
	u := c.baseURL + route
	if len(params) > 0 {
		u += "?" + params.Encode()
	}
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	req.Header.Set(endpointTokenHeader, c.token)

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return fmt.Errorf("reading response: %w", err)
	}

	// 401/403 mean the gate rejected us, which is a bug in how mxcli passed the
	// token rather than anything to do with the tests. Say so plainly instead of
	// letting it surface as an unmarshalling error.
	switch resp.StatusCode {
	case http.StatusUnauthorized:
		return fmt.Errorf("test endpoint rejected the token (is another app serving port %s?)", portOf(c.baseURL))
	case http.StatusForbidden:
		return fmt.Errorf("test endpoint refused the request: %s", strings.TrimSpace(string(body)))
	}

	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("decoding response (HTTP %d): %w: %s", resp.StatusCode, err, truncate(string(body), 200))
	}
	return nil
}

// exec sends one compiled test to the endpoint's exec route (interp.go) and
// returns its reply, in the run route's shape. A POST, so the test travels in
// the body: a query string has a length limit and lands in access logs.
func (c *endpointClient) exec(p *execProgram) (*runResponse, error) {
	body, err := json.Marshal(p)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(http.MethodPost, c.baseURL+endpointExecRoute, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set(endpointTokenHeader, c.token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}
	var rr runResponse
	if resp.StatusCode != http.StatusOK {
		var e struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(data, &e)
		if e.Error == "" {
			e.Error = truncate(strings.TrimSpace(string(data)), 200)
		}
		return nil, fmt.Errorf("the test endpoint refused the test (HTTP %d): %s", resp.StatusCode, e.Error)
	}
	if err := json.Unmarshal(data, &rr); err != nil {
		return nil, fmt.Errorf("decoding response: %w: %s", err, truncate(string(data), 200))
	}
	return &rr, nil
}

// ping reports whether the endpoint is up and accepting our token.
func (c *endpointClient) ping() error {
	var lr listResponse
	return c.get("list", nil, &lr)
}

// list returns the test microflows the running app knows about.
func (c *endpointClient) list() ([]string, error) {
	var lr listResponse
	if err := c.get("list", url.Values{"prefix": {testFlowPrefix}}, &lr); err != nil {
		return nil, err
	}
	if lr.Error != "" {
		return nil, fmt.Errorf("%s", lr.Error)
	}
	return lr.Microflows, nil
}

// run executes one test microflow and returns the endpoint's reply. With
// rollback set, the endpoint wraps the call in a transaction it rolls back, so
// the test's database writes do not survive.
func (c *endpointClient) run(mf string, rollback bool) (*runResponse, error) {
	params := url.Values{"mf": {mf}}
	if rollback {
		params.Set("rollback", "1")
	}
	var rr runResponse
	if err := c.get("run", params, &rr); err != nil {
		return nil, err
	}
	return &rr, nil
}

// waitReady polls until the endpoint answers or the deadline passes. The runtime
// reports itself started before the after-startup action has necessarily
// finished registering the handler, so a first call can legitimately 404.
func (c *endpointClient) waitReady(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var last error
	for {
		if err := c.ping(); err == nil {
			return nil
		} else {
			last = err
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("test endpoint did not come up within %s: %w", timeout, last)
		}
		time.Sleep(200 * time.Millisecond)
	}
}

// toResult maps an endpoint reply onto the test's result.
//
// Three outcomes are distinguished, and the distinction matters when reading a
// failing run: the test decided it failed (an assertion), the microflow threw
// (StatusError — the test did not reach a verdict), or the verdict came back in
// a shape this runner does not recognise.
func toResult(tc TestCase, rr *runResponse) TestResult {
	res := newResult(tc)
	res.Duration = time.Duration(rr.DurationMicros) * time.Microsecond
	switch {
	case !rr.OK:
		res.Status = StatusError
		res.Message = rr.Error
		if res.Message == "" {
			res.Message = "microflow threw, but the runtime reported no message"
		}
	case rr.Result == verdictPass:
		res.Status = StatusPass
	case strings.HasPrefix(rr.Result, verdictFailPrefix):
		res.Status = StatusFail
		res.Message = strings.TrimPrefix(rr.Result, verdictFailPrefix)
	case strings.HasPrefix(rr.Result, verdictSetupPrefix):
		// The setup threw, so the test never ran — an ERROR, not a FAIL.
		res.Status = StatusError
		res.Message = "setup microflow " + strings.TrimPrefix(rr.Result, verdictSetupPrefix) +
			" failed, so the test did not run"
	default:
		res.Status = StatusError
		res.Message = fmt.Sprintf("unrecognised verdict from the test microflow: %q", truncate(rr.Result, 200))
	}
	return res
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// portOf pulls the port back out of a base URL for error messages.
func portOf(baseURL string) string {
	u, err := url.Parse(baseURL)
	if err != nil {
		return "?"
	}
	return u.Port()
}
