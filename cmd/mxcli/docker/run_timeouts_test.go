// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"
)

// Reported (mendixlabs/mxcli#1818, mirrored as ako/mxcli#2136): on a 12,900-
// document app the first build hit a fixed 10-minute HTTP limit ("context
// deadline exceeded (Client.Timeout exceeded while awaiting headers)") and the
// runtime start a fixed 150 s one, reported as "cannot connect to Mendix admin
// API ... -- is the app running?" while the after-startup microflow was still
// running. Neither had a flag or an environment variable.

func TestServeBuildTimeout_Resolution(t *testing.T) {
	t.Setenv(buildTimeoutEnv, "")
	if got := serveBuildTimeout(0); got != defaultBuildTimeout {
		t.Errorf("default = %v, want %v", got, defaultBuildTimeout)
	}
	if defaultBuildTimeout <= 10*time.Minute {
		t.Errorf("default build timeout %v is not above the 10m a large app's cold build was measured to need", defaultBuildTimeout)
	}
	t.Setenv(buildTimeoutEnv, "45m")
	if got := serveBuildTimeout(0); got != 45*time.Minute {
		t.Errorf("env = %v, want 45m", got)
	}
	if got := serveBuildTimeout(time.Hour); got != time.Hour {
		t.Errorf("explicit = %v, want 1h (the flag wins over the environment)", got)
	}
	t.Setenv(buildTimeoutEnv, "soon")
	if got := serveBuildTimeout(0); got != defaultBuildTimeout {
		t.Errorf("unparsable env = %v, want the default", got)
	}
}

func TestRuntimeStartTimeout_Resolution(t *testing.T) {
	t.Setenv(startTimeoutEnv, "")
	if got := runtimeStartTimeout(0); got != defaultStartTimeout {
		t.Errorf("default = %v, want %v", got, defaultStartTimeout)
	}
	if defaultStartTimeout <= 150*time.Second {
		t.Errorf("default start timeout %v is not above the 150s an after-startup microflow outran", defaultStartTimeout)
	}
	t.Setenv(startTimeoutEnv, "20m")
	if got := runtimeStartTimeout(0); got != 20*time.Minute {
		t.Errorf("env = %v, want 20m", got)
	}
	if got := runtimeStartTimeout(time.Minute); got != time.Minute {
		t.Errorf("explicit = %v, want 1m", got)
	}
}

// The serve build honours the configured limit, and running out of it names the
// limit and how to raise it rather than only "context deadline exceeded".
func TestServeBuild_HonoursBuildTimeoutAndNamesIt(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(400 * time.Millisecond)
		_, _ = w.Write([]byte(`{"status":"Success"}`))
	}))
	defer srv.Close()
	host, port := hostPort(t, srv.URL)

	s := &ServeServer{Host: host, Port: port, opts: ServeOptions{BuildTimeout: 50 * time.Millisecond}}
	_, err := s.Build(BuildRequest{ProjectFilePath: "/abs/app.mpr"})
	if err == nil {
		t.Fatal("build returned before the server answered: the BuildTimeout was not applied")
	}
	for _, want := range []string{"--build-timeout", buildTimeoutEnv} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("timeout error does not say how to raise the limit (missing %q):\n%v", want, err)
		}
	}
	if !isTimeout(err) {
		t.Errorf("the timeout must stay recognisable to buildRetryingServeTimeout: %v", err)
	}

	s.opts.BuildTimeout = 5 * time.Second
	if _, err := s.Build(BuildRequest{ProjectFilePath: "/abs/app.mpr"}); err != nil {
		t.Fatalf("a build inside a raised limit failed: %v", err)
	}
}

// The start action runs the after-startup microflow, so it is bounded by the
// start timeout, not by the short limit every other admin action has.
func TestRuntimeControllerStart_UsesStartTimeout(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Action string `json:"action"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req.Action == "start" {
			time.Sleep(400 * time.Millisecond) // a slow after-startup action
		}
		_ = json.NewEncoder(w).Encode(M2EEResponse{})
	}))
	defer ts.Close()
	host, port := hostPort(t, ts.URL)
	opts := M2EEOptions{Host: host, Port: port, Token: "t", Direct: true, Timeout: 100 * time.Millisecond}

	c := NewRuntimeController(opts)
	c.StartTimeout = 5 * time.Second
	if _, err := c.Start(); err != nil {
		t.Fatalf("start outran the short admin timeout although StartTimeout allows it: %v", err)
	}

	// And when the start does run out of time, say so — not "is the app running?".
	c.StartTimeout = 50 * time.Millisecond
	_, err := c.Start()
	if err == nil {
		t.Fatal("start returned success past its StartTimeout")
	}
	msg := err.Error()
	if strings.Contains(msg, "is the app running") {
		t.Errorf("a start that ran out of time is reported as an unreachable app:\n%s", msg)
	}
	for _, want := range []string{"did not answer", "--start-timeout", startTimeoutEnv} {
		if !strings.Contains(msg, want) {
			t.Errorf("start timeout error missing %q:\n%s", want, msg)
		}
	}
}

func hostPort(t *testing.T, raw string) (string, int) {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	h, p, err := net.SplitHostPort(u.Host)
	if err != nil {
		t.Fatal(err)
	}
	n, _ := strconv.Atoi(p)
	return h, n
}
