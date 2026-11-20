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
	"strings"
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
	// MxBuildPath is the mxbuild binary, and is what every local caller sets —
	// resolved once, for this host, by ResolveMxBuildForLocal.
	//
	// When empty, resolution falls back to the cache: the entry for Version if
	// there is one, and the newest cached entry only when no Version is given. It
	// will NOT substitute a different version for a named one; see
	// resolveServeMxBuild.
	MxBuildPath string
	// Version is the Mendix version used to resolve mxbuild when MxBuildPath is
	// empty, and to refuse a cache entry that cannot build this project.
	Version string
	// JavaHome is the JDK home. When empty it is resolved for JavaMajor.
	JavaHome string
	// JavaMajor is the Java release the PROJECT is built for (its Settings >
	// Model > JavaVersion). Zero means DefaultJavaMajor. Mendix 11.14's blank app
	// asks for 25; everything up to 11.13 asks for 21.
	JavaMajor int
	// Host to bind the serve HTTP API (default 127.0.0.1).
	Host string
	// Port for the serve HTTP API (default 6543).
	Port int
	// ReadyTimeout bounds how long StartServe waits for the API (default 90s).
	ReadyTimeout time.Duration
	// BuildTimeout bounds one /build request. Zero falls back to
	// $MXCLI_BUILD_TIMEOUT, then 30m (see serveBuildTimeout).
	BuildTimeout time.Duration
}

// defaultBuildTimeout bounds one serve build when nothing overrides it.
//
// It was a fixed 10 minutes, and the cold build of a 188-module, 12,900-
// document app takes about that long on a laptop, so it passed or failed by
// the run (mendixlabs/mxcli#1818). A build that hangs is caught by the watchdog
// on mxbuild's output, not by this; this only has to outlast a slow honest one.
const defaultBuildTimeout = 30 * time.Minute

// buildTimeoutEnv overrides defaultBuildTimeout (a Go duration, e.g. "45m").
// The --build-timeout flag of `run --local` wins over it.
const buildTimeoutEnv = "MXCLI_BUILD_TIMEOUT"

// serveBuildTimeout resolves the build limit: an explicit value first, then
// $MXCLI_BUILD_TIMEOUT, then the default. An unparsable or non-positive
// environment value is ignored, as for MXCLI_WEB_CLIENT_TIMEOUT.
func serveBuildTimeout(explicit time.Duration) time.Duration {
	return durationSetting(explicit, buildTimeoutEnv, defaultBuildTimeout)
}

// durationSetting is the flag -> environment -> default chain the local loop's
// time limits share.
func durationSetting(explicit time.Duration, env string, def time.Duration) time.Duration {
	if explicit > 0 {
		return explicit
	}
	if v := strings.TrimSpace(os.Getenv(env)); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			return d
		}
	}
	return def
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
	Problems        BuildProblems   `json:"problems"`
	Raw             json.RawMessage `json:"-"`

	// ColdRestart marks a RestartRequired that comes from a fresh serve
	// process's first build with no incremental build to correct it. A first
	// build reports true even for an unchanged model, so here true means "could
	// not tell", not "an entity changed". serveRecovery sets it; mxbuild never
	// sends it. Measured on 11.14.0: every later build in a serve process
	// fails, even for an unchanged model, and the failed response has no
	// restartRequired at all, so this is the only answer a build there gets.
	ColdRestart bool `json:"-"`
}

// BuildProblems is the serve response's problems object. The consistency errors
// are in the inner list; the outer one carries only the summary.
type BuildProblems struct {
	Problems []BuildProblem `json:"problems"`
}

// BuildProblem is one consistency message from a build.
//
// Only the fields anything reads are declared. Severity is the load-bearing one:
// a failing build of a blank 11.13 app returns 18 problems of which 16 are
// warnings and one a deprecation, so printing the response wholesale buries the
// single error that actually stopped the build.
type BuildProblem struct {
	Severity  string          `json:"severity"` // "Error", "Warning", "Deprecation"
	Message   string          `json:"message"`
	ErrorCode string          `json:"errorCode"` // e.g. "CE0109"
	Locations []BuildLocation `json:"locations"`
}

// BuildLocation is where a problem was found. Document is the human name Studio
// Pro would show — `Microflow 'Test_test_3'` — which is what lets a caller
// attribute an error to the document it generated.
type BuildLocation struct {
	Module   string `json:"module"`
	Document string `json:"document"`
	Element  string `json:"element"`
}

// OK reports whether the build succeeded.
func (r *BuildResult) OK() bool { return r.Status == "Success" }

// Errors returns only the problems that failed the build.
func (r *BuildResult) Errors() []BuildProblem {
	var out []BuildProblem
	for _, p := range r.Problems.Problems {
		if strings.EqualFold(p.Severity, "Error") {
			out = append(out, p)
		}
	}
	return out
}

// ErrorSummary renders the build errors one per line, with the code and the
// document each was found in.
//
// This is what a caller should print instead of the raw response body: the body
// is ~200 lines of JSON in which the real error sits among unrelated Atlas
// warnings, and a reader has no way to tell which line failed the build.
// Returns "" when the response carried no structured errors, so a caller can
// fall back rather than print nothing.
func (r *BuildResult) ErrorSummary() string {
	errs := r.Errors()
	if len(errs) == 0 {
		return ""
	}
	var b strings.Builder
	for i, p := range errs {
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString("  ")
		if p.ErrorCode != "" {
			b.WriteString("[" + p.ErrorCode + "] ")
		}
		b.WriteString(p.Message)
		if loc := p.Where(); loc != "" {
			b.WriteString(" — at " + loc)
		}
	}
	return b.String()
}

// Where renders a problem's first location as `Module / Document / Element`,
// skipping the parts the response left empty.
func (p BuildProblem) Where() string {
	if len(p.Locations) == 0 {
		return ""
	}
	l := p.Locations[0]
	parts := make([]string, 0, 3)
	for _, s := range []string{l.Module, l.Document, l.Element} {
		if s != "" {
			parts = append(parts, s)
		}
	}
	return strings.Join(parts, " / ")
}

// ServeServer wraps a long-lived `mxbuild --serve` process and its build API.
type ServeServer struct {
	Host string
	Port int
	cmd  *exec.Cmd
	log  *syncBuffer
	// opts are the resolved options it was started with, so Restart starts
	// the same mxbuild with the same JDK on the same port.
	opts ServeOptions
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

// serveOptionsFor builds the ServeOptions for the local loop's build server.
//
// It exists so that the two local entry points — `run --local` (RunLocal) and
// `test --local` (StartLocalApp) — cannot construct these options differently.
// Each resolves the mxbuild this host can execute (ResolveMxBuildForLocal) and
// then has to hand it on; both used to omit it, so StartServe re-resolved from
// the cache and could start a DIFFERENT mxbuild than the one the caller had just
// chosen. On macOS that was the normal case, not an edge one: the caller picked
// Studio Pro's binary and the serve process got whatever the cache held, which
// is either a Linux download or another version entirely (#1122).
//
// MxBuildPath is the load-bearing field. The rest is plumbing.
func serveOptionsFor(mxbuildPath, version string, javaMajor, servePort int) ServeOptions {
	return ServeOptions{
		MxBuildPath: mxbuildPath,
		Version:     version,
		JavaMajor:   javaMajor,
		Host:        "127.0.0.1",
		Port:        servePort,
	}
}

// resolveServeMxBuild decides which mxbuild `--serve` runs.
//
// A caller-supplied path wins, and is the path every local caller takes. When
// there is none, an exactly-matching cache entry is used — and, crucially, NOT a
// near miss: substituting another version here is how a project pinned to 11.12.2
// came to be built by 11.14.0, a mismatch mxbuild itself only reports minutes
// later, after a cold model load. The unversioned fallback survives because a
// caller that names no version has nothing to be mismatched against.
func resolveServeMxBuild(opts ServeOptions) string {
	if opts.MxBuildPath != "" {
		return opts.MxBuildPath
	}
	if opts.Version != "" {
		return CachedMxBuildPath(opts.Version)
	}
	return AnyCachedMxBuildPath()
}

// noServeMxBuildError explains a failed resolution in the terms the user can act
// on. It names the version that was asked for and what the cache does hold,
// because the two being different is the whole defect — and it does not send a
// macOS user to `setup mxbuild`, which on a host with Studio Pro installed
// reports the path and caches nothing, leaving them exactly where they started
// (#1124).
func noServeMxBuildError(version string) error {
	if version == "" {
		return fmt.Errorf("mxbuild not found; run 'mxcli setup mxbuild -p <app.mpr>' or pass --mxbuild-path")
	}
	msg := fmt.Sprintf("no mxbuild for Mendix %s", version)
	if cached := AnyCachedMxBuildPath(); cached != "" {
		msg += fmt.Sprintf("\n  The cache holds %s, which cannot build a %s project:\n    %s",
			versionFromPath(cached), version, cached)
	}
	return fmt.Errorf("%s\n"+
		"  Install Mendix Studio Pro %[2]s (its bundled mxbuild is used automatically on macOS and Windows),\n"+
		"  point mxcli at one with --mxbuild-path, or on Linux run 'mxcli setup mxbuild --version %[2]s'.", msg, version)
}

// StartServe launches `mxbuild --serve` and blocks until the build API responds.
// Call Stop() to shut it down. The first Build() loads the model (cold, ~10-15s);
// subsequent builds are incremental (~1s).
func StartServe(opts ServeOptions) (*ServeServer, error) {
	mxbuildPath := resolveServeMxBuild(opts)
	if mxbuildPath == "" {
		return nil, noServeMxBuildError(opts.Version)
	}
	if err := verifyMxBuildCache(mxbuildPath); err != nil {
		return nil, err
	}
	// The cache can hold a binary for another OS — the CDN only ships Linux, and
	// Windows keeps one deliberately for Docker builds. Exec'ing it produces a
	// bare "exec format error" naming neither cause nor remedy (#916).
	if err := verifyRunsHere(mxbuildPath); err != nil {
		return nil, err
	}

	javaHome := opts.JavaHome
	if javaHome == "" {
		jh, err := resolveJDK(opts.JavaMajor)
		if err != nil {
			return nil, err
		}
		javaHome = jh
	}
	javaExe := JavaExePath(javaHome)

	host := opts.Host
	if host == "" {
		host = "127.0.0.1"
	}
	port := opts.Port
	if port == 0 {
		port = 6543
	}

	cmd, log := newServeCmd(mxbuildPath, host, port, javaHome, javaExe)
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("starting mxbuild --serve: %w", err)
	}

	resolved := opts
	resolved.MxBuildPath, resolved.JavaHome, resolved.Host, resolved.Port = mxbuildPath, javaHome, host, port
	s := &ServeServer{Host: host, Port: port, cmd: cmd, log: log, opts: resolved}

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

// newServeCmd builds the `mxbuild --serve` command and the buffer its output
// goes to. Split out of StartServe so the process settings — which decide
// whether Stop can return at all — are tested on the command that ships.
func newServeCmd(mxbuildPath, host string, port int, javaHome, javaExe string) (*exec.Cmd, *syncBuffer) {
	cmd := exec.Command(mxbuildPath, "--serve",
		fmt.Sprintf("--host=%s", host),
		fmt.Sprintf("--port=%d", port),
		fmt.Sprintf("--java-home=%s", javaHome),
		fmt.Sprintf("--java-exe-path=%s", javaExe),
	)
	PrepareMxCommand(cmd) // FreeType LD_PRELOAD workaround
	setProcessGroup(cmd)  // reap the JVM grandchild on Stop (mxbuild is a wrapper)
	log := &syncBuffer{}
	cmd.Stdout = log
	cmd.Stderr = log
	cmd.WaitDelay = orphanPipeWaitDelay // a Gradle daemon may hold the pipe (#1396)
	return cmd, log
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
//
// A relative ProjectFilePath is resolved here rather than by each caller.
// MxBuild rejects one outright ("the project file path should be an absolute
// path") and answers with a page of Windows sample requests, which tells a user
// who typed `-p app.mpr` nothing about what to do. `mxcli run` learned to
// absolutize at the CLI layer (findings #17), but that left the requirement
// unenforced for every other caller — and `mxcli test --local` then hit exactly
// the same error through StartLocalApp. Doing it at the one place that talks to
// MxBuild is what stops a third caller finding it again.
func (s *ServeServer) Build(req BuildRequest) (*BuildResult, error) {
	if req.Target == "" {
		req.Target = TargetDeploy
	}
	if req.ProjectFilePath != "" && !filepath.IsAbs(req.ProjectFilePath) {
		abs, err := filepath.Abs(req.ProjectFilePath)
		if err != nil {
			return nil, fmt.Errorf("resolving project path %q: %w", req.ProjectFilePath, err)
		}
		req.ProjectFilePath = abs
	}
	bodyBytes, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshaling build request: %w", err)
	}
	limit := serveBuildTimeout(s.opts.BuildTimeout)
	client := &http.Client{Timeout: limit}
	resp, err := client.Post(s.baseURL()+"/build", "application/json", bytes.NewReader(bodyBytes))
	if err != nil {
		if isTimeout(err) {
			// %w keeps it a net.Error timeout, which buildRetryingServeTimeout
			// recognises.
			return nil, fmt.Errorf("serve build request to %s: mxbuild did not finish the build within %s "+
				"(a large app's cold build can take longer: raise the limit with --build-timeout or $%s): %w",
				s.baseURL(), limit, buildTimeoutEnv, err)
		}
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

// alive reports whether the serve process is still running.
//
// Delegates to processAlive: Signal(0) is a correct liveness test on POSIX but
// returns EWINDOWS on Windows, where it made waitReady() treat a just-started
// mxbuild as dead.
func (s *ServeServer) alive() bool {
	if s.cmd == nil || s.cmd.Process == nil {
		return false
	}
	return processAlive(s.cmd.Process)
}

// Restart replaces the serve process with a fresh one started from the same
// options, in place, so every holder of this *ServeServer — including the
// caller's deferred Stop — follows it. Its first build is cold. See
// serve_recovery.go for why the watch loop needs one.
func (s *ServeServer) Restart() error {
	_ = s.Stop()
	s.cmd = nil // a failed start below must not leave Stop signalling a reaped pid
	next, err := StartServe(s.opts)
	if err != nil {
		return err
	}
	s.cmd, s.log = next.cmd, next.log
	return nil
}

// Log returns the captured mxbuild --serve output (for diagnostics).
func (s *ServeServer) Log() string { return s.log.String() }

// Stop terminates the serve process (SIGTERM, then SIGKILL after a grace period).
func (s *ServeServer) Stop() error {
	if s.cmd == nil || s.cmd.Process == nil {
		return nil
	}
	_ = signalProcessGroup(s.cmd.Process, syscall.SIGTERM)
	done := make(chan error, 1)
	go func() { done <- s.cmd.Wait() }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		_ = killProcessGroup(s.cmd.Process)
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

// buildFailureDetail renders what to show a user after a failed build.
//
// The structured error list when the response carried one, and the raw body only
// as a fallback. Printing the body was the previous behaviour everywhere, and it
// is close to useless: on a blank 11.13 app a single consistency error arrives
// alongside 16 Atlas warnings in ~200 lines of JSON, with nothing marking which
// one stopped the build.
func buildFailureDetail(r *BuildResult) string {
	if r == nil {
		return ""
	}
	if summary := r.ErrorSummary(); summary != "" {
		return summary
	}
	return string(r.Raw)
}

// BuildFailedError is returned when mxbuild rejected the model.
//
// It carries the parsed result so a caller can do something better than print
// the message: `mxcli test` maps each error back to the generated test microflow
// it was found in, which is the difference between "the build failed" and
// "test 3's assertion does not compile".
type BuildFailedError struct {
	Result *BuildResult
}

func (e *BuildFailedError) Error() string {
	msg := "build failed"
	if e.Result != nil && e.Result.Message != "" {
		msg += ": " + e.Result.Message
	}
	if detail := buildFailureDetail(e.Result); detail != "" {
		msg += "\n" + detail
	}
	return msg
}

// BuildErrors returns the consistency errors that failed the build.
func (e *BuildFailedError) BuildErrors() []BuildProblem {
	if e == nil || e.Result == nil {
		return nil
	}
	return e.Result.Errors()
}

// orphanPipeWaitDelay bounds how long Wait waits, after mx/mxbuild has exited,
// for its output pipe to close.
//
// A writer that is not an *os.File makes exec.Cmd create a pipe, and Wait does
// not return until every holder of its write end has closed it. When mxbuild or
// mx starts a NEW Gradle daemon, the daemon inherits that write end and outlives
// its parent — on Windows it is out of taskkill /T's reach once the parent is
// gone — so Wait blocked forever and `test --local` never printed its results or
// wrote its JUnit file (#1396). A reused daemon was started by an earlier run and
// never held this pipe, which is why only a cold Gradle home hung.
//
// The delay runs only once the process has exited, so it never cuts a live build
// short; Wait then closes the pipe and returns exec.ErrWaitDelay.
var orphanPipeWaitDelay = 10 * time.Second
