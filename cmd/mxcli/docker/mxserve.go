// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"time"
)

// mxbuild --serve is an incremental build server: it keeps the model loaded and
// rebuilds only what changed, so a warm build is ~1s vs ~30-60s for a one-shot
// `mxbuild` invocation. It speaks HTTP: POST /build with a JSON request, and the
// response carries `status` plus `restartRequired` (the signal for whether a
// runtime restart is needed vs a hot `reload_model`). See
// docs/11-proposals/PROPOSAL_mxcli_dev_warm_loop.md.

// ServeOptions configures StartServe.
type ServeOptions struct {
	// MxBuildPath is the mxbuild binary. When empty it is resolved from Version
	// (CachedMxBuildPath) or the newest cached mxbuild (AnyCachedMxBuildPath).
	MxBuildPath string
	// Version is the Mendix version used to resolve mxbuild when MxBuildPath is empty.
	Version string
	// JavaHome is the JDK 21 home. When empty it is resolved via resolveJDK21().
	JavaHome string
	// Host to bind the serve HTTP API (default 127.0.0.1).
	Host string
	// Port for the serve HTTP API (default 6543).
	Port int
	// ReadyTimeout bounds how long StartServe waits for the API (default 90s).
	ReadyTimeout time.Duration
}

// ServeTarget is the build target for a serve request.
type ServeTarget string

const (
	// TargetDeploy writes the deployment structure directly into the project's
	// deployment dir (no .mda) — the form the runtime reads for reload_model.
	TargetDeploy ServeTarget = "Deploy"
	// TargetPackage produces an .mda at MdaFilePath.
	TargetPackage ServeTarget = "Package"
)

// BuildRequest mirrors the mxbuild serve /build request schema.
type BuildRequest struct {
	Target               ServeTarget `json:"target"`
	ProjectFilePath      string      `json:"projectFilePath"`
	MdaFilePath          string      `json:"mdaFilePath,omitempty"`
	UseLooseVersionCheck bool        `json:"useLooseVersionCheck,omitempty"`
}

// BuildResult is the parsed serve /build response. Raw holds the full body for
// callers that need startup_metrics or other fields.
type BuildResult struct {
	Status          string          `json:"status"`
	RestartRequired bool            `json:"restartRequired"`
	Message         string          `json:"message"`
	Raw             json.RawMessage `json:"-"`
}

// OK reports whether the build succeeded.
func (r *BuildResult) OK() bool { return r.Status == "Success" }

// ServeServer wraps a long-lived `mxbuild --serve` process and its build API.
type ServeServer struct {
	Host string
	Port int
	cmd  *exec.Cmd
	log  *syncBuffer
}

// verifyMxBuildCache checks the mxbuild cache is complete: the sibling runtime/
// directory (which holds the Mendix API libraries on the javac classpath) must
// exist next to modeler/. Without it, Java compilation fails with a cryptic
// "package com.mendix.* does not exist" — in both serve and one-shot builds. A
// normal `mxcli setup mxbuild` / DownloadMxBuild extracts both dirs; this guards
// against a partially-populated cache. mxbuildPath is <cache>/modeler/mxbuild,
// so the runtime dir is <cache>/runtime.
func verifyMxBuildCache(mxbuildPath string) error {
	runtimeDir := filepath.Join(filepath.Dir(filepath.Dir(mxbuildPath)), "runtime")
	if fi, err := os.Stat(runtimeDir); err != nil || !fi.IsDir() {
		return fmt.Errorf("mxbuild cache incomplete: missing runtime directory %s "+
			"(Java compilation needs the Mendix runtime libraries) -- re-run 'mxcli setup mxbuild -p <app.mpr>'", runtimeDir)
	}
	return nil
}

// StartServe launches `mxbuild --serve` and blocks until the build API responds.
// Call Stop() to shut it down. The first Build() loads the model (cold, ~10-15s);
// subsequent builds are incremental (~1s).
func StartServe(opts ServeOptions) (*ServeServer, error) {
	mxbuildPath := opts.MxBuildPath
	if mxbuildPath == "" && opts.Version != "" {
		mxbuildPath = CachedMxBuildPath(opts.Version)
	}
	if mxbuildPath == "" {
		mxbuildPath = AnyCachedMxBuildPath()
	}
	if mxbuildPath == "" {
		return nil, fmt.Errorf("mxbuild not found; run 'mxcli setup mxbuild -p <app.mpr>' or pass ServeOptions.MxBuildPath")
	}
	if err := verifyMxBuildCache(mxbuildPath); err != nil {
		return nil, err
	}

	javaHome := opts.JavaHome
	if javaHome == "" {
		jh, err := resolveJDK21()
		if err != nil {
			return nil, err
		}
		javaHome = jh
	}
	javaExe := filepath.Join(javaHome, "bin", "java")

	host := opts.Host
	if host == "" {
		host = "127.0.0.1"
	}
	port := opts.Port
	if port == 0 {
		port = 6543
	}

	cmd := exec.Command(mxbuildPath, "--serve",
		fmt.Sprintf("--host=%s", host),
		fmt.Sprintf("--port=%d", port),
		fmt.Sprintf("--java-home=%s", javaHome),
		fmt.Sprintf("--java-exe-path=%s", javaExe),
	)
	PrepareMxCommand(cmd) // FreeType LD_PRELOAD workaround
	log := &syncBuffer{}
	cmd.Stdout = log
	cmd.Stderr = log

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("starting mxbuild --serve: %w", err)
	}

	s := &ServeServer{Host: host, Port: port, cmd: cmd, log: log}

	readyTimeout := opts.ReadyTimeout
	if readyTimeout == 0 {
		readyTimeout = 90 * time.Second
	}
	if err := s.waitReady(readyTimeout); err != nil {
		_ = s.Stop()
		return nil, fmt.Errorf("mxbuild --serve did not become ready: %w\n--- mxbuild output ---\n%s", err, s.log.String())
	}
	return s, nil
}

func (s *ServeServer) baseURL() string { return fmt.Sprintf("http://%s:%d", s.Host, s.Port) }

// waitReady polls the build endpoint until it responds (an empty POST yields a
// JSON validation error, which still proves the HTTP server is up).
func (s *ServeServer) waitReady(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	client := &http.Client{Timeout: 3 * time.Second}
	for time.Now().Before(deadline) {
		resp, err := client.Post(s.baseURL()+"/build", "application/json", bytes.NewReader([]byte("{}")))
		if err == nil {
			resp.Body.Close()
			return nil
		}
		if !s.alive() {
			return fmt.Errorf("mxbuild --serve exited during startup")
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("timed out after %s", timeout)
}

// Build sends a build request to the warm server and parses the result. The
// caller inspects RestartRequired to decide reload_model vs restart.
func (s *ServeServer) Build(req BuildRequest) (*BuildResult, error) {
	if req.Target == "" {
		req.Target = TargetDeploy
	}
	bodyBytes, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshaling build request: %w", err)
	}
	// Cold builds can take ~60s; keep a generous timeout.
	client := &http.Client{Timeout: 10 * time.Minute}
	resp, err := client.Post(s.baseURL()+"/build", "application/json", bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("serve build request to %s: %w", s.baseURL(), err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading serve response: %w", err)
	}
	var res BuildResult
	if err := json.Unmarshal(raw, &res); err != nil {
		return nil, fmt.Errorf("decoding serve response (HTTP %d): %w -- body: %s", resp.StatusCode, err, string(raw))
	}
	res.Raw = raw
	return &res, nil
}

// alive reports whether the serve process is still running (Linux: signal 0).
func (s *ServeServer) alive() bool {
	if s.cmd == nil || s.cmd.Process == nil {
		return false
	}
	return s.cmd.Process.Signal(syscall.Signal(0)) == nil
}

// Log returns the captured mxbuild --serve output (for diagnostics).
func (s *ServeServer) Log() string { return s.log.String() }

// Stop terminates the serve process (SIGTERM, then SIGKILL after a grace period).
func (s *ServeServer) Stop() error {
	if s.cmd == nil || s.cmd.Process == nil {
		return nil
	}
	_ = s.cmd.Process.Signal(syscall.SIGTERM)
	done := make(chan error, 1)
	go func() { done <- s.cmd.Wait() }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		_ = s.cmd.Process.Kill()
		<-done
	}
	return nil
}

// syncBuffer is a goroutine-safe bytes.Buffer for capturing subprocess output
// while it is read concurrently for diagnostics.
type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (w *syncBuffer) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.b.Write(p)
}

func (w *syncBuffer) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.b.String()
}
