// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

// newTestServe returns a ServeServer wired to an httptest server, so the HTTP
// client (Build) can be tested without spawning mxbuild.
func newTestServe(t *testing.T, handler http.HandlerFunc) *ServeServer {
	t.Helper()
	ts := httptest.NewServer(handler)
	t.Cleanup(ts.Close)
	u, err := url.Parse(ts.URL)
	if err != nil {
		t.Fatalf("parse test URL: %v", err)
	}
	host, portStr, err := net.SplitHostPort(u.Host)
	if err != nil {
		t.Fatalf("split host:port: %v", err)
	}
	port, _ := strconv.Atoi(portStr)
	return &ServeServer{Host: host, Port: port}
}

func TestServeBuild_DeployRestartRequired(t *testing.T) {
	s := newTestServe(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/build" {
			t.Errorf("path = %q, want /build", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Errorf("method = %q, want POST", r.Method)
		}
		var req BuildRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if req.Target != TargetDeploy {
			t.Errorf("target = %q, want Deploy (default)", req.Target)
		}
		if req.ProjectFilePath != "/x/App.mpr" {
			t.Errorf("projectFilePath = %q", req.ProjectFilePath)
		}
		_, _ = w.Write([]byte(`{"restartRequired": true, "status": "Success"}`))
	})

	res, err := s.Build(BuildRequest{ProjectFilePath: "/x/App.mpr"}) // Target empty -> Deploy
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if !res.OK() {
		t.Errorf("OK() = false, status = %q", res.Status)
	}
	if !res.RestartRequired {
		t.Error("RestartRequired = false, want true (domain/view-entity change)")
	}
}

func TestServeBuild_HotReloadable(t *testing.T) {
	s := newTestServe(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"restartRequired": false, "status": "Success"}`))
	})
	res, err := s.Build(BuildRequest{Target: TargetDeploy, ProjectFilePath: "/x/App.mpr"})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if !res.OK() {
		t.Errorf("OK() = false, status = %q", res.Status)
	}
	if res.RestartRequired {
		t.Error("RestartRequired = true, want false (microflow/page change -> reload_model)")
	}
}

func TestServeBuild_Failure(t *testing.T) {
	s := newTestServe(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"status": "Failure", "message": "Specified 'target' is invalid."}`))
	})
	res, err := s.Build(BuildRequest{Target: "Bogus", ProjectFilePath: "/x/App.mpr"})
	if err != nil {
		t.Fatalf("Build should parse the failure envelope, got transport error: %v", err)
	}
	if res.OK() {
		t.Error("OK() = true, want false")
	}
	if res.Message == "" {
		t.Error("Message empty, want the failure message")
	}
}

func TestServeBuild_PackageTargetSendsMdaPath(t *testing.T) {
	s := newTestServe(t, func(w http.ResponseWriter, r *http.Request) {
		var req BuildRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req.Target != TargetPackage {
			t.Errorf("target = %q, want Package", req.Target)
		}
		if req.MdaFilePath != "/out/app.mda" {
			t.Errorf("mdaFilePath = %q, want /out/app.mda", req.MdaFilePath)
		}
		_, _ = w.Write([]byte(`{"restartRequired": true, "status": "Success"}`))
	})
	if _, err := s.Build(BuildRequest{Target: TargetPackage, ProjectFilePath: "/x/App.mpr", MdaFilePath: "/out/app.mda"}); err != nil {
		t.Fatalf("Build: %v", err)
	}
}

func TestVerifyMxBuildCache(t *testing.T) {
	// layout: <cache>/modeler/mxbuild  and  <cache>/runtime
	cache := t.TempDir()
	modeler := filepath.Join(cache, "modeler")
	if err := os.MkdirAll(modeler, 0o755); err != nil {
		t.Fatal(err)
	}
	mxbuild := filepath.Join(modeler, "mxbuild")
	if err := os.WriteFile(mxbuild, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	// runtime/ missing -> error
	if err := verifyMxBuildCache(mxbuild); err == nil {
		t.Error("expected error when runtime/ dir is missing")
	}

	// runtime/ present -> ok
	if err := os.MkdirAll(filepath.Join(cache, "runtime"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := verifyMxBuildCache(mxbuild); err != nil {
		t.Errorf("expected no error when runtime/ present, got %v", err)
	}
}

func TestServeBuild_BadJSONBody(t *testing.T) {
	s := newTestServe(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<html>not json</html>`))
	})
	if _, err := s.Build(BuildRequest{ProjectFilePath: "/x/App.mpr"}); err == nil {
		t.Error("expected an error decoding a non-JSON body")
	}
}
