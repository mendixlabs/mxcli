// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

// timeoutErr is what net/http returns when the client's Timeout elapses: a
// *url.Error whose Timeout() is true ("context deadline exceeded (Client.Timeout
// exceeded while awaiting headers)").
type timeoutErr struct{}

func (timeoutErr) Error() string {
	return "context deadline exceeded (Client.Timeout exceeded while awaiting headers)"
}
func (timeoutErr) Timeout() bool   { return true }
func (timeoutErr) Temporary() bool { return true }

func serveTimeout() error {
	return fmt.Errorf("serve build request to http://127.0.0.1:6544: %w",
		&url.Error{Op: "Post", URL: "http://127.0.0.1:6544/build", Err: timeoutErr{}})
}

// A `mxbuild --serve` that stops answering made `mxcli test --local` end in
// "serve build request ... context deadline exceeded", and the next run passed
// with nothing changed (ako/mxcli#1834). The boot's build now restarts the
// serve process and builds once more on a timeout.
func TestBuildRetryingServeTimeout_RetriesOnceOnAFreshProcess(t *testing.T) {
	f := &scriptedServe{build: func(proc, nth int) (*BuildResult, error) {
		if proc == 0 {
			return nil, serveTimeout()
		}
		return buildOK(true), nil
	}}
	var out strings.Builder
	res, err := buildRetryingServeTimeout(f, BuildRequest{Target: TargetDeploy}, &out)
	if err != nil {
		t.Fatalf("err = %v, want the retry's success", err)
	}
	if !res.OK() {
		t.Errorf("result = %+v, want the retry's", res)
	}
	if f.restarts != 1 || len(f.reqs) != 2 {
		t.Errorf("restarts = %d, builds = %d; want one restart and one retry", f.restarts, len(f.reqs))
	}
	if !strings.Contains(out.String(), "did not answer") {
		t.Errorf("the retry was silent: %q", out.String())
	}
}

// Once, not until it works: a second timeout is reported, with the first.
func TestBuildRetryingServeTimeout_SecondTimeoutIsReported(t *testing.T) {
	f := &scriptedServe{build: func(proc, nth int) (*BuildResult, error) { return nil, serveTimeout() }}
	_, err := buildRetryingServeTimeout(f, BuildRequest{}, &strings.Builder{})
	if err == nil || !strings.Contains(err.Error(), "deadline exceeded") {
		t.Fatalf("err = %v, want the timeout", err)
	}
	if f.restarts != 1 || len(f.reqs) != 2 {
		t.Errorf("restarts = %d, builds = %d; want exactly one retry", f.restarts, len(f.reqs))
	}
}

// Anything but a timeout is not retried: a failed build is a verdict, and a
// refused connection is a dead process a restart would only hide. Also the
// control for the retry test.
func TestBuildRetryingServeTimeout_OtherOutcomesAreNotRetried(t *testing.T) {
	cases := map[string]func(int, int) (*BuildResult, error){
		"success": func(int, int) (*BuildResult, error) { return buildOK(true), nil },
		"failure": func(int, int) (*BuildResult, error) {
			return &BuildResult{Status: "Failure", Message: "The project cannot be deployed"}, nil
		},
		"refused": func(int, int) (*BuildResult, error) {
			return nil, &url.Error{Op: "Post", URL: "x", Err: errors.New("connection refused")}
		},
	}
	for name, build := range cases {
		f := &scriptedServe{build: build}
		_, _ = buildRetryingServeTimeout(f, BuildRequest{}, &strings.Builder{})
		if f.restarts != 0 || len(f.reqs) != 1 {
			t.Errorf("%s: restarts = %d, builds = %d; want no retry", name, f.restarts, len(f.reqs))
		}
	}
}

// The fake above is only as good as its resemblance to the real error, so the
// detection is also checked against what net/http actually returns when a
// Client.Timeout elapses, wrapped the way ServeServer.Build wraps it.
func TestIsTimeout_RealClientTimeout(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { <-release }))
	defer srv.Close()
	defer close(release)

	_, err := (&http.Client{Timeout: 20 * time.Millisecond}).Post(srv.URL+"/build", "application/json", nil)
	if err == nil {
		t.Fatal("request did not time out")
	}
	if wrapped := fmt.Errorf("serve build request to %s: %w", srv.URL, err); !isTimeout(wrapped) {
		t.Errorf("isTimeout(%v) = false, want true", wrapped)
	}
}
