// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/model"
)

// runlocal.go is the `mxcli run --local` orchestrator: a warm, Docker-free dev
// loop. It keeps a mxbuild --serve process and a standalone runtime hot, so a
// model change rebuilds incrementally (~1s) and is applied by a hot reload_model
// (page/microflow/text) or a runtime restart (entity/view/association) — the
// serve build's restartRequired flag decides which. See
// docs/11-proposals/PROPOSAL_mxcli_dev_warm_loop.md.

// LocalAppInfo is what a running local app exposes to another process: the
// loopback ports it serves on, and the admin password needed to drive its M2EE
// API. The admin password is separate from any application-level credential —
// conflating the two is an authentication failure at the first admin call.
type LocalAppInfo struct {
	AppPort   int
	AdminPort int
	ServePort int
	AdminPass string
	// BootConfig is the update_configuration payload the runtime was started
	// with. A caller wanting to change ONE setting on the running app re-sends
	// this with that key replaced — the admin API has no read-back, so anything
	// not re-sent is simply gone from the configuration.
	BootConfig map[string]any
}

// LocalRunOptions configures RunLocal.
type LocalRunOptions struct {
	// ProjectPath is the .mpr file.
	ProjectPath string
	// ConstantOverrides are the constant values of the configuration this run
	// represents (qualified name -> value), merged over the defaults mxbuild
	// wrote into the deployment. Empty means "every constant keeps its default",
	// which is what a local run always did before — see runconstants.go.
	ConstantOverrides map[string]string
	// DeployDir is where the serve Deploy target writes (default <projectDir>/deployment).
	DeployDir string
	// MxBuildPath overrides mxbuild resolution (optional).
	MxBuildPath string
	// AppPort / AdminPort / ServePort default to 8080 / 8090 / 6543.
	AppPort   int
	AdminPort int
	ServePort int
	// AdminPass is the M2EE admin password (default is a fixed local-dev value).
	AdminPass string
	// DB is the Postgres the runtime connects to (devcontainer defaults applied).
	DB DBConfig
	// Hub, when set, is the URL of an mxcli tunnel-hub (e.g. https://hub.example.com).
	// The app stays running here; a chisel client reverse-tunnels it out to the hub
	// so it is reachable in a browser at the hub URL. Implies a local run. The
	// runtime boots with ApplicationRootUrl = Hub so the SPA works under that origin.
	Hub string
	// HubSecret is the shared auth secret for the hub ("user:pass"), matching the
	// hub's --secret. Optional but recommended.
	HubSecret string
	// HubKey is a per-user tunnel-hub API key (from `mxcli auth hub login` or
	// MXCLI_HUB_KEY) presented to an authenticated hub as X-Hub-Key; it stamps the
	// preview's owner. Empty for open self-hosted hubs.
	HubKey string
	// Hub identity (multi-tenant hub): these drive the assigned subdomain and the
	// hub overview grouping. Blank Project/Branch are auto-detected from the .mpr
	// name and git.
	HubPrefix   string // optional hostname namespace (org/solution/team/env)
	HubProject  string // override the auto-detected project name
	HubSolution string // grouping for multi-app solutions
	HubBranch   string // override the auto-detected git branch
	HubWorktree string // distinguish worktrees of one branch
	HubSession  string // override the auto-detected Claude Code session id
	// Watch keeps running, rebuilding+applying on every project change.
	Watch bool
	// EnsureDB provisions the local Postgres + app database if missing (otherwise
	// the DB must already exist). Intended for a fresh session / SessionStart boot.
	EnsureDB bool
	// SetupOnly does the idempotent prerequisites (cache mxbuild+runtime, ensure
	// the database) and returns without booting serve/runtime — the non-blocking
	// bring-up a SessionStart hook runs each session. The agent then runs the full
	// (blocking) loop on demand.
	SetupOnly bool
	// PollInterval is how often Watch checks for changes (default 1s).
	PollInterval time.Duration
	// Screenshot, when set, captures a PNG of the app after boot and after each
	// applied change (requires the Playwright CLI + a browser).
	Screenshot bool
	// ScreenshotPath is where the PNG is written (default <projectDir>/.mxcli/run-local.png).
	// With multiple ScreenshotURLs, it is the base name and each page gets a
	// per-page suffix (run-local-<page>.png).
	ScreenshotPath string
	// ScreenshotURLs are the pages to shoot each change: full http(s) URLs, or
	// paths relative to the app root (e.g. "/p/customers"). Empty -> the app root.
	// More than one produces a screenshot set (one PNG per page).
	ScreenshotURLs []string
	// ScreenshotUser/Password log in once (Mendix form auth) and reuse the session
	// for every screenshot, so pages behind login render authenticated.
	ScreenshotUser     string
	ScreenshotPassword string
	// screenshotStorage is the resolved Playwright storage-state file (from login);
	// internal, set during boot.
	screenshotStorage string
	// RuntimeLogPath tees the Mendix runtime's stdout+stderr to a file so the
	// warm loop is debuggable (server stack traces, microflow LOG output).
	// Default <projectDir>/.mxcli/runtime.log; set to "-" to disable. Findings #25.
	RuntimeLogPath string
	// Debug enables the runtime microflow debugger at boot and starts a session,
	// caching its token under <projectDir>/.mxcli so `mxcli debug break/paused/…`
	// (in another terminal, same -p) works immediately. Enabling with no
	// breakpoints does not change runtime behaviour — only a set breakpoint pauses.
	Debug bool
	// DebugPass is the debugger password used when Debug is set (default "mxdebug").
	DebugPass string
	// Metrics registers a Prometheus meter registry at boot; the runtime then
	// serves /prometheus on the admin port. Sugar for a Metrics.Registries setting.
	Metrics bool
	// Trace attaches the bundled OpenTelemetry Java agent (traces → the runtime
	// log via the console exporter) and applies default span filters.
	Trace bool
	// TraceService is OTEL_SERVICE_NAME under --trace (default: the .mpr name).
	TraceService string
	// TraceOTLP, when set, exports traces to this OTLP collector endpoint
	// (e.g. http://127.0.0.1:4318) instead of the console — needed for flame
	// charts, since the console exporter omits timestamps/parent span IDs.
	// Implies Trace.
	TraceOTLP string
	// Env are extra "KEY=value" entries for the runtime JVM (see
	// LocalRuntimeOptions.Env) — how a secret reaches the runtime without being
	// written to disk. `--test-endpoint` passes the test-endpoint token this way.
	Env []string
	// OnReady, when set, is called once the app is serving, with everything a
	// second process needs to drive it. Used by `--test-endpoint` to publish its
	// handshake only after there is something for `mxcli test --attach` to
	// connect to.
	OnReady func(LocalAppInfo)
	// RuntimeSettings are raw "Key=Value" runtime settings merged into the boot
	// update_configuration payload (Value is parsed as JSON, else a string), e.g.
	// 'Metrics.Registries=[{"type":"otlp"}]' or
	// 'OpenTelemetry._RuntimeSpanFilters=["Loop","Gateway"]'.
	RuntimeSettings []string
	Stdout          io.Writer
	Stderr          io.Writer
}

// defaultLocalAdminPass is the admin password for a local dev runtime. The admin
// API binds to 127.0.0.1 only, so a fixed value is acceptable for local use.
const defaultLocalAdminPass = "mxcli-local-dev"

func (o *LocalRunOptions) applyDefaults() {
	if o.DeployDir == "" {
		o.DeployDir = filepath.Join(filepath.Dir(o.ProjectPath), "deployment")
	}
	if o.AppPort == 0 {
		o.AppPort = 8080
	}
	if o.AdminPort == 0 {
		o.AdminPort = 8090
	}
	if o.ServePort == 0 {
		o.ServePort = 6543
	}
	if o.AdminPass == "" {
		o.AdminPass = defaultLocalAdminPass
	}
	if o.PollInterval == 0 {
		o.PollInterval = time.Second
	}
	if o.DB.Type == "" {
		o.DB.Type = "PostgreSQL"
	}
	if o.DB.Host == "" {
		o.DB.Host = "127.0.0.1:5432"
	}
	if o.DB.User == "" {
		o.DB.User = "mendix"
	}
	if o.DB.Password == "" {
		o.DB.Password = "mendix"
	}
	if o.DB.Name == "" {
		o.DB.Name = deriveDBName(o.ProjectPath)
	}
	if o.ScreenshotPath == "" {
		o.ScreenshotPath = filepath.Join(filepath.Dir(o.ProjectPath), ".mxcli", "run-local.png")
	}
	if o.RuntimeLogPath == "" {
		o.RuntimeLogPath = filepath.Join(filepath.Dir(o.ProjectPath), ".mxcli", "runtime.log")
	}
	if o.Debug && o.DebugPass == "" {
		o.DebugPass = "mxdebug"
	}
	if o.Stdout == nil {
		o.Stdout = os.Stdout
	}
	if o.Stderr == nil {
		o.Stderr = os.Stderr
	}
}

// defaultOtelSpanFilters are the internal runtime spans suppressed under --trace.
// Unfiltered per-activity tracing is ~10x slower; these bring it near baseline
// while keeping the microflow-level spans (findings — OpenTelemetry).
var defaultOtelSpanFilters = []any{"CreateOrChangeVariable", "Loop", "Gateway", "RetrieveFromCache"}

// buildRuntimeSettings turns --metrics/--trace + repeatable --runtime-setting
// Key=Value into the map merged into the boot update_configuration payload. A
// Value that parses as JSON is used as-is; otherwise it is a plain string.
// --metrics adds a Prometheus registry and --trace adds default span filters,
// each unless the corresponding key was already set explicitly.
func buildRuntimeSettings(metrics, trace bool, raw []string) (map[string]any, error) {
	out := map[string]any{}
	for _, s := range raw {
		k, v, err := parseRuntimeSetting(s)
		if err != nil {
			return nil, err
		}
		out[k] = v
	}
	if metrics {
		if _, ok := out["Metrics.Registries"]; !ok {
			out["Metrics.Registries"] = []any{map[string]any{"type": "prometheus"}}
		}
	}
	if trace {
		if _, ok := out["OpenTelemetry._RuntimeSpanFilters"]; !ok {
			out["OpenTelemetry._RuntimeSpanFilters"] = defaultOtelSpanFilters
		}
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}

// parseRuntimeSetting splits "Key=Value"; Value is parsed as JSON when possible
// (so arrays/objects/numbers/bools pass through typed), else kept as a string.
func parseRuntimeSetting(s string) (string, any, error) {
	i := strings.IndexByte(s, '=')
	if i <= 0 {
		return "", nil, fmt.Errorf("invalid --runtime-setting %q (want Key=Value)", s)
	}
	key := strings.TrimSpace(s[:i])
	rawVal := s[i+1:]
	var v any
	if json.Unmarshal([]byte(rawVal), &v) == nil {
		return key, v, nil
	}
	return key, rawVal, nil
}

// deriveDBName turns a project file name into a safe Postgres database name:
// lowercased, non-alphanumerics collapsed to underscores, leading digit prefixed.
// configuredApplicationRootURL returns the ApplicationRootUrl set on the
// project's server configuration, plus the name of the configuration it came
// from. Empty when no configuration sets one, which is the default.
//
// The model has no "active configuration" marker — Studio Pro remembers the
// selection per developer — so the one named "Default" wins, falling back to
// the first configuration that actually sets a URL. A settings read failure is
// not fatal: the run simply proceeds without a root URL, exactly as before.
func configuredApplicationRootURL(reader backend.FullBackend) (rootURL, configName string) {
	settings, err := reader.GetProjectSettings()
	if err != nil {
		return "", ""
	}
	return applicationRootURLFrom(settings)
}

// applicationRootURLFrom implements the selection rule over already-read
// settings, so it is testable without a project on disk.
func applicationRootURLFrom(settings *model.ProjectSettings) (rootURL, configName string) {
	if settings == nil || settings.Configuration == nil {
		return "", ""
	}
	first, firstName := "", ""
	for _, cfg := range settings.Configuration.Configurations {
		if cfg == nil || cfg.ApplicationRootUrl == "" {
			continue
		}
		if strings.EqualFold(cfg.Name, "Default") {
			return cfg.ApplicationRootUrl, cfg.Name
		}
		if first == "" {
			first, firstName = cfg.ApplicationRootUrl, cfg.Name
		}
	}
	return first, firstName
}

// customHostRootURL reports whether an ApplicationRootUrl names a host worth
// telling the runtime about.
//
// A blank Mendix app already ships `http://localhost:8080/`, so "set" does not
// mean "chosen". Passing that back would be a behaviour change for every
// existing project — and an actively wrong one under --app-port, where the
// stock value names a port the app is not serving on. A loopback host also adds
// nothing: with no ApplicationRootUrl the runtime derives one from the listen
// address, which is the same thing. So only a real host name — the case this
// exists for, giving each app in a solution its own name — is honoured.
func customHostRootURL(rootURL string) bool {
	if rootURL == "" {
		return false
	}
	u, err := url.Parse(rootURL)
	if err != nil || u.Host == "" {
		return false
	}
	host := strings.ToLower(u.Hostname())
	if host == "localhost" || host == "::1" {
		return false
	}
	if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
		return false
	}
	return true
}

// urlPort returns the explicit port of a URL, or "" when it has none.
func urlPort(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return u.Port()
}

// DeriveDBName is the local-run database name for a project: the .mpr file name
// lowercased and sanitised to a legal identifier. Exported so callers that boot
// their own local app (e.g. the test runner, which appends a suffix to keep test
// data out of the dev database) derive the same base name.
func DeriveDBName(projectPath string) string { return deriveDBName(projectPath) }

func deriveDBName(projectPath string) string {
	base := strings.TrimSuffix(filepath.Base(projectPath), filepath.Ext(projectPath))
	var b strings.Builder
	for _, r := range strings.ToLower(base) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	name := strings.Trim(b.String(), "_")
	if name == "" {
		return "mxlocal"
	}
	if name[0] >= '0' && name[0] <= '9' {
		name = "db_" + name
	}
	return name
}

// ensureMxBuildRuntimeSibling makes the downloaded runtime available as a
// runtime/ sibling of the mxbuild cache's modeler/ dir. mxbuild's Deploy/serve
// javac step resolves the Mendix API from there; without it compilation fails
// with "package com.mendix.* does not exist". It is a no-op if the sibling
// already exists (symlink or real dir). Mirrors ensurePADFiles' link pattern.
func ensureMxBuildRuntimeSibling(version string, w io.Writer) error {
	mxbuildDir, err := MxBuildCacheDir(version)
	if err != nil {
		return err
	}
	dst := filepath.Join(mxbuildDir, "runtime")
	if _, err := os.Stat(dst); err == nil {
		return nil // already present (bundled or linked previously)
	}
	runtimeDir, err := RuntimeCacheDir(version)
	if err != nil {
		return err
	}
	src := filepath.Join(runtimeDir, "runtime")
	if _, err := os.Stat(src); err != nil {
		return fmt.Errorf("runtime not found at %s (run 'mxcli setup mxbuild' / DownloadRuntime first): %w", src, err)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	if err := os.Symlink(src, dst); err != nil {
		return fmt.Errorf("linking runtime into mxbuild cache: %w", err)
	}
	fmt.Fprintf(w, "  Linked runtime into mxbuild cache: %s -> %s\n", dst, src)
	return nil
}

// pingTCP dials host (host:port) to check reachability within timeout.
func pingTCP(hostPort string, timeout time.Duration) error {
	conn, err := net.DialTimeout("tcp", hostPort, timeout)
	if err != nil {
		return err
	}
	_ = conn.Close()
	return nil
}

// checkTargetPortsFree refuses to boot when any of the loop's ports is already
// answering, which almost always means a previous `mxcli run --local` (or a
// stray mxbuild --serve / runtime) is still alive. Without this guard the boot
// "succeeds" against the stale process: the readiness probes only check that the
// port answers (StartServe.waitReady / runtime waitAdminReady), so mxcli adopts
// the old serve/runtime, the fresh JVM that failed to bind is torn down by
// defer, and the old process keeps serving stale output — reading exactly like a
// stale cache. Refusing with an actionable message turns that silent failure
// into a clear one. We only *detect* here (never kill): reaping someone else's
// process is the user's call.
func checkTargetPortsFree(o LocalRunOptions) error {
	host := "127.0.0.1"
	type p struct {
		port int
		role string
		flag string
	}
	for _, c := range []p{
		{o.AppPort, "app", "--app-port"},
		{o.AdminPort, "admin API", "--admin-port"},
		{o.ServePort, "mxbuild serve", "--serve-port"},
	} {
		hostPort := fmt.Sprintf("%s:%d", host, c.port)
		if err := pingTCP(hostPort, 500*time.Millisecond); err == nil {
			return fmt.Errorf("port %d (%s) is already in use.\n"+
				"  A stale process is silently adopted otherwise, so edits appear to do nothing (looks like a stale cache — it isn't).\n"+
				"%s"+
				"  Or run on different ports with %s (and --admin-port/--serve-port).",
				c.port, c.role, portCulpritAdvice(c.port, host, o.AppPort), c.flag)
		}
	}
	return nil
}

// webClientSourceMTime returns the newest mtime of the browser-client *source*
// under <deployDir>/web, excluding the rollup output (dist/) and the build log.
// A page/widget/theme edit bumps it (and needs a client re-bundle); a
// microflow/entity-only edit does not. Zero time if web/ is absent.
func webClientSourceMTime(deployDir string) time.Time {
	webDir := filepath.Join(deployDir, "web")
	var newest time.Time
	_ = filepath.WalkDir(webDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if d.Name() == "dist" {
				return fs.SkipDir
			}
			return nil
		}
		if info, err := d.Info(); err == nil && info.ModTime().After(newest) {
			newest = info.ModTime()
		}
		return nil
	})
	return newest
}

// projectSourceMTime returns the newest mtime of the Mendix model *source*: the
// .mpr file (v1 stores everything here) plus the mprcontents/ document tree (v2).
// It is the change signal for Watch. Watching only the model source — not the
// whole project dir — makes it immune to build-output churn: the serve/mxbuild
// build rewrites theme-cache/, .mendix-cache/, and deployment/ on every run, and
// screenshots land in .mxcli/, none of which must re-trigger the watcher.
func projectSourceMTime(projectPath string) time.Time {
	var newest time.Time
	if fi, err := os.Stat(projectPath); err == nil {
		newest = fi.ModTime()
	}
	mprcontents := filepath.Join(filepath.Dir(projectPath), "mprcontents")
	_ = filepath.WalkDir(mprcontents, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if !d.IsDir() {
			if info, err := d.Info(); err == nil && info.ModTime().After(newest) {
				newest = info.ModTime()
			}
		}
		return nil
	})
	return newest
}

// themeSourceMTime returns the newest mtime of the app's *theme source* — the
// SCSS/CSS/JS the designer edits under theme/ (app-level: main.scss,
// custom-variables.scss, …) and themesource/<module>/web/ (per-module). These
// live outside the .mpr/mprcontents tree, so projectSourceMTime never sees them;
// without this a theme edit triggers no rebuild even under --watch, and the app
// keeps serving the old styles (the "SCSS cache" symptom — really a missing
// watch). mxbuild --serve recompiles the theme on the next /build, so surfacing
// the edit as a change signal is all that's needed. Walk is mtime-polling (same
// as projectSourceMTime), so it is reliable on container filesystems where
// inotify is not — no watcher fd involved.
func themeSourceMTime(projectPath string) time.Time {
	dir := filepath.Dir(projectPath)
	var newest time.Time
	for _, sub := range []string{"theme", "themesource"} {
		root := filepath.Join(dir, sub)
		_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			switch strings.ToLower(filepath.Ext(path)) {
			case ".scss", ".css", ".js", ".json":
				if info, err := d.Info(); err == nil && info.ModTime().After(newest) {
					newest = info.ModTime()
				}
			}
			return nil
		})
	}
	return newest
}

// sourceMTime is the combined --watch change signal: the newer of the model
// source (projectSourceMTime) and the theme source (themeSourceMTime). Either a
// model edit or a theme edit re-triggers the build.
func sourceMTime(projectPath string) time.Time {
	m := projectSourceMTime(projectPath)
	if t := themeSourceMTime(projectPath); t.After(m) {
		return t
	}
	return m
}

// RunLocal boots the warm local dev loop: resolve tooling, start mxbuild --serve
// and a standalone runtime, do the first build+apply, then (with Watch) rebuild
// and hot-apply on every project change until interrupted.
func RunLocal(opts LocalRunOptions) error {
	opts.applyDefaults()
	w, stderr := opts.Stdout, opts.Stderr

	// 0. Refuse fast if the loop's ports are already taken (a stale run/serve/
	// runtime). Skipped for SetupOnly, which never boots a server. This is the
	// single-instance guard: without it a stale process is silently adopted and
	// keeps serving old output.
	if !opts.SetupOnly {
		if err := checkTargetPortsFree(opts); err != nil {
			return err
		}
	}

	// 1. Detect the project's Mendix version.
	fmt.Fprintln(w, "Detecting project version...")
	reader, err := openReadOnly(opts.ProjectPath)
	if err != nil {
		return fmt.Errorf("opening project: %w", err)
	}
	pv := reader.ProjectVersion()
	// Read the configured application root URL while the project is open — the
	// app's own configuration is where a custom host name belongs (App Settings ->
	// Configurations in Studio Pro, `alter settings` in MDL), versioned with the
	// app rather than repeated on every command line.
	modelRootURL, modelRootConfig := configuredApplicationRootURL(reader)
	// Managed Java dependencies are declared in the model but resolved by a
	// separate step; collect them here so the boot can vendor any that are
	// missing (mxcli-formula1 findings #12).
	declaredJars := declaredJarDependencies(reader)
	reader.Disconnect()
	version := pv.ProductVersion
	fmt.Fprintf(w, "  Mendix version: %s\n", version)

	// 2. Ensure mxbuild + runtime are cached, and linked for the serve javac step.
	fmt.Fprintln(w, "Ensuring MxBuild and runtime are available...")
	// Resolve what this host can actually execute: Studio Pro before the cache
	// on macOS/Windows, and honour --mxbuild-path, which the local path used to
	// ignore (#916).
	mxbuildPath, err := ResolveMxBuildForLocal(opts.MxBuildPath, version, w)
	if err != nil {
		return fmt.Errorf("setting up mxbuild: %w", err)
	}
	installPath, err := resolveRuntimeInstall(version, w)
	if err != nil {
		return fmt.Errorf("setting up runtime: %w", err)
	}

	// 3. Vendor any declared Java dependency that is not in vendorlib/. MxBuild
	// does not resolve these (a full build emits no dependencies block), so
	// without this the app builds green and throws "no driver found" at runtime.
	// Best-effort: it needs network, and a project whose jars are already
	// vendored — the common case — does no work at all.
	if missing := UnvendoredJarDependencies(filepath.Dir(opts.ProjectPath), declaredJars); len(missing) > 0 {
		fmt.Fprintf(w, "Resolving %d managed Java dependency/dependencies (%s)...\n",
			len(missing), strings.Join(missing, ", "))
		if err := SyncJavaDependencies(opts.ProjectPath, "", version, w); err != nil {
			fmt.Fprintf(stderr, "  Warning: could not resolve Java dependencies: %v\n", err)
			fmt.Fprintln(stderr, "  The app will build, but code needing those jars fails at runtime.")
			fmt.Fprintf(stderr, "  Retry with: mxcli sync-java-deps -p %s\n", opts.ProjectPath)
		}
	}

	// 4. Ensure the database is available. With --ensure-db, provision it (start
	// local Postgres + create the role/db if missing); otherwise just check
	// reachability and point the user at --ensure-db.
	if opts.EnsureDB {
		fmt.Fprintln(w, "Ensuring database...")
		if err := EnsureDatabase(&opts.DB, w); err != nil {
			return fmt.Errorf("ensuring database: %w", err)
		}
	} else if err := pingTCP(opts.DB.Host, 3*time.Second); err != nil {
		return fmt.Errorf("database not reachable at %s: %w\n"+
			"  Pass --ensure-db to provision it, or start Postgres and create the '%s' database (user %q).",
			opts.DB.Host, err, opts.DB.Name, opts.DB.User)
	}

	// Setup-only: prerequisites are ready (mxbuild+runtime cached, database up).
	// Stop here without booting — this is the non-blocking SessionStart bring-up.
	if opts.SetupOnly {
		fmt.Fprintf(w, "Setup complete: MxBuild %s + runtime cached, database %q ready.\n", version, opts.DB.Name)
		fmt.Fprintln(w, "Run 'mxcli run --local -p <app.mpr>' to boot the warm dev loop.")
		return nil
	}

	// 5. Start the warm build server.
	fmt.Fprintln(w, "Starting mxbuild --serve...")
	javaMajor, _ := ProjectJavaMajor(opts.ProjectPath)
	serve, err := StartServe(ServeOptions{
		Version:   version,
		JavaMajor: javaMajor,
		Host:      "127.0.0.1",
		Port:      opts.ServePort,
	})
	if err != nil {
		return fmt.Errorf("starting mxbuild serve: %w", err)
	}
	defer serve.Stop()

	// 5. First build (cold — loads the model).
	fmt.Fprintln(w, "Building (first build is cold, ~10-15s)...")
	build, err := serve.Build(BuildRequest{Target: TargetDeploy, ProjectFilePath: opts.ProjectPath})
	if err != nil {
		return fmt.Errorf("initial build: %w", err)
	}
	if !build.OK() {
		return fmt.Errorf("initial build failed: %s\n%s", build.Message, buildFailureDetail(build))
	}

	// 5b. Bundle the browser client (web/dist). The serve Deploy target writes the
	// client source but not the rollup bundle, so without this the app 404s on
	// /dist/index.js and renders blank. In --watch we keep an incremental bundler
	// hot (warm rollup graph + polling file detection -> ~3-4s re-bundles);
	// otherwise a single one-shot build (~7s cold) suffices.
	var watcher *WebClientWatcher
	if opts.Watch {
		fmt.Fprintln(w, "Starting incremental web client bundler...")
		// A nil watcher is not a failure: on Mendix 11.14+ mxbuild's serve build
		// writes web/dist itself, so there is no bundler to keep hot. Every
		// watcher method is nil-safe, so the watch loop needs no branch.
		watcher, err = StartWebClientWatch(WebClientOptions{DeployDir: opts.DeployDir, MxBuildPath: mxbuildPath, Stdout: w})
		if err != nil {
			return fmt.Errorf("starting web client bundler: %w", err)
		}
		defer watcher.Stop()
	} else {
		fmt.Fprintln(w, "Bundling web client...")
		if err := BuildWebClient(WebClientOptions{DeployDir: opts.DeployDir, MxBuildPath: mxbuildPath, Stdout: w}); err != nil {
			return fmt.Errorf("bundling web client: %w", err)
		}
	}

	// 5c. With --hub, register with the hub first (before boot) so we know the
	// assigned public URL — the runtime must boot with ApplicationRootUrl set to it
	// (else the SPA/originURI misbehave across origins). A multi-tenant hub hands
	// back a per-preview subdomain + reverse port; a single-app hub falls back to
	// the hub URL itself.
	var hubReg *HubRegistration
	appRootURL := ""
	if opts.Hub != "" {
		meta := DetectHubMeta(opts.ProjectPath, HubMeta{
			Prefix: opts.HubPrefix, Project: opts.HubProject, Solution: opts.HubSolution,
			Branch: opts.HubBranch, Worktree: opts.HubWorktree, Session: opts.HubSession,
		})
		fmt.Fprintf(w, "Registering with hub %s...\n", opts.Hub)
		hubReg, err = RegisterWithHub(opts.Hub, opts.HubSecret, opts.HubKey, meta, opts.AppPort)
		if err != nil {
			// A failed registration (unreachable hub, stale/rejected key) must cost
			// only the public preview URL — not the whole run. Degrade to a normal
			// local run so the app still boots and is reachable on localhost.
			// (findings #27, #31C)
			fmt.Fprintf(stderr, "Warning: hub registration failed (%v); continuing local-only — the app runs on localhost but has no public preview URL.\n", err)
			if opts.HubKey == "" {
				fmt.Fprintf(stderr, "  hint: this hub requires auth; set MXCLI_HUB_KEY or run 'mxcli auth hub login'.\n")
			}
			hubReg = nil
			err = nil
		} else {
			appRootURL = hubReg.URL
		}
	}

	// No hub URL: fall back to the one configured in the project. This is what
	// makes "give each app its own host name" work for a local run — Mendix needs
	// to know the URL it is reached at to generate absolute URLs (OIDC/SAML
	// redirect URIs, deep links) that point at the host name rather than the
	// listen address. A hub assignment wins, since that URL is the one actually
	// serving the app.
	if appRootURL == "" && customHostRootURL(modelRootURL) {
		appRootURL = modelRootURL
		fmt.Fprintf(w, "Application root URL from configuration %q: %s\n", modelRootConfig, appRootURL)
		if port := urlPort(appRootURL); port != "" && port != fmt.Sprint(opts.AppPort) {
			fmt.Fprintf(stderr, "Warning: configuration %q says port %s but the app is serving on %d — "+
				"absolute URLs will point at %s. Update the configuration or pass --app-port %s.\n",
				modelRootConfig, port, opts.AppPort, appRootURL, port)
		}
	}

	// 6. Boot the runtime against the fresh deployment. Tee the runtime's own
	// stdout/stderr to a log file so server-side errors are debuggable ("-"
	// disables). (findings #25)
	runtimeLog := opts.RuntimeLogPath
	if runtimeLog == "-" {
		runtimeLog = ""
	}
	runtimeSettings, err := buildRuntimeSettings(opts.Metrics, opts.Trace, opts.RuntimeSettings)
	if err != nil {
		return err
	}
	traceService := opts.TraceService
	if opts.Trace && traceService == "" {
		traceService = strings.TrimSuffix(filepath.Base(opts.ProjectPath), filepath.Ext(opts.ProjectPath))
	}
	rt, err := StartLocalRuntime(LocalRuntimeOptions{
		DeployDir:   opts.DeployDir,
		InstallPath: installPath,
		// JavaHome left empty: StartLocalRuntime resolves a JDK for JavaMajor,
		// which must match what mxbuild compiled the project for.
		JavaMajor:          javaMajor,
		AppPort:            opts.AppPort,
		AdminPort:          opts.AdminPort,
		AdminPass:          opts.AdminPass,
		ApplicationRootUrl: appRootURL,
		DB:                 opts.DB,
		RuntimeLogPath:     runtimeLog,
		RuntimeSettings:    runtimeSettings,
		Trace:              opts.Trace,
		TraceServiceName:   traceService,
		TraceOTLPEndpoint:  opts.TraceOTLP,
		ConstantOverrides:  opts.ConstantOverrides,
		Env:                opts.Env,
		Stdout:             w,
		Stderr:             stderr,
	})
	if err != nil {
		return err
	}
	defer rt.Stop()

	// 6b. The boot's Gradle `package` pass repopulates deployment/web, and when
	// Gradle had work to do it takes dist/ with it — deleting the bundle step 5b
	// wrote seconds ago. Verify after the boot, because before it proves nothing
	// (mxcli-formula1 §35). Costs a stat when the bundle survived.
	if _, err := EnsureWebClientBundle(WebClientOptions{
		DeployDir: opts.DeployDir, MxBuildPath: mxbuildPath, Stdout: w,
	}); err != nil {
		// The app is up and its services answer; only the browser is broken. Say so
		// and keep running rather than tearing down a working runtime.
		fmt.Fprintf(stderr, "Warning: %v\n", err)
	}

	if opts.OnReady != nil {
		opts.OnReady(LocalAppInfo{
			AppPort:    opts.AppPort,
			AdminPort:  opts.AdminPort,
			ServePort:  opts.ServePort,
			AdminPass:  opts.AdminPass,
			BootConfig: rt.BootConfig(),
		})
	}

	fmt.Fprintf(w, "\nApp is running at %s\n", rt.AppURL())
	// The local runtime boots with the live-preview dev flags (see
	// LocalRuntimeOptions.jvmArgs), so `mxcli oql` can query it directly — and it
	// now defaults to the local admin password, so no M2EE_ADMIN_PASS is needed
	// (findings #36).
	fmt.Fprintf(w, "Query data:  mxcli oql -p %s \"SELECT ...\"\n", opts.ProjectPath)
	if runtimeLog != "" {
		fmt.Fprintf(w, "Runtime log: %s\n", runtimeLog)
	}
	if opts.Metrics {
		// The Prometheus registry is served from the admin port (loopback).
		fmt.Fprintf(w, "Metrics (Prometheus): http://127.0.0.1:%d/prometheus\n", opts.AdminPort)
	}
	if opts.Trace {
		switch {
		case opts.TraceOTLP != "":
			fmt.Fprintf(w, "Tracing enabled (OpenTelemetry, service %q); spans -> OTLP %s\n", traceService, opts.TraceOTLP)
		case runtimeLog != "":
			fmt.Fprintf(w, "Tracing enabled (OpenTelemetry, service %q); spans -> %s\n", traceService, runtimeLog)
		default:
			fmt.Fprintf(w, "Tracing enabled (OpenTelemetry, service %q); spans go to the console only (pass --runtime-log to capture them, or --trace-otlp <url> for a collector)\n", traceService)
		}
	}

	// 6·debug: --debug enables the microflow debugger and starts a session now, so
	// breakpoints can be set from another terminal without a separate
	// `mxcli debug enable`. No breakpoints exist yet, so nothing pauses; the token
	// is cached under <projectDir>/.mxcli for the `mxcli debug …` commands.
	if opts.Debug {
		dbg := NewDebuggerClient(DebuggerOptions{
			Admin:     rt.m2ee,
			AppURL:    rt.AppURL(),
			DebugPass: opts.DebugPass,
			TokenPath: filepath.Join(filepath.Dir(opts.ProjectPath), ".mxcli", "debug-session.token"),
		})
		if err := dbg.Enable(); err != nil {
			fmt.Fprintf(stderr, "  (debugger not enabled: %v)\n", err)
		} else if _, err := dbg.StartSession(); err != nil {
			fmt.Fprintf(stderr, "  (debugger enabled but starting a session failed: %v)\n", err)
		} else {
			fmt.Fprintf(w, "Debugger enabled. Set a breakpoint from another terminal:\n")
			fmt.Fprintf(w, "  mxcli debug break <Module.Flow> --activity <#n|caption> -p %s\n", opts.ProjectPath)
			// Best-effort: turn the debugger back off on shutdown so a breakpoint
			// can't be left pausing requests. (The runtime dies with the process
			// anyway; this also clears the cached session token.)
			defer func() { _ = dbg.Disable() }()
		}
	}

	// 6a. With --hub, open a reverse tunnel so the app is reachable in a browser at
	// its public URL, and heartbeat so it shows as available in the hub overview.
	// The app stays here; only live HTTP flows through the tunnel (nothing pushed).
	if hubReg != nil {
		tunnel, err := StartTunnel(TunnelOptions{
			HubURL:     hubReg.ControlURL,
			LocalPort:  opts.AppPort,
			RemotePort: hubReg.ReversePort,
			Secret:     hubReg.TunnelAuth,
			PublicURL:  hubReg.URL,
			Stdout:     w,
		})
		if err != nil {
			return fmt.Errorf("starting hub tunnel: %w", err)
		}
		// The heartbeat re-registers if the hub restarts; when that lands on a new
		// reverse port, restart the tunnel to the new port. Guard the handle since
		// the callback runs on the heartbeat goroutine.
		var tunMu sync.Mutex
		defer func() { tunMu.Lock(); tunnel.Stop(); tunMu.Unlock() }()

		hb := StartHeartbeat(hubReg, func(reg *HubRegistration) {
			tunMu.Lock()
			defer tunMu.Unlock()
			tunnel.Stop()
			nt, err := StartTunnel(TunnelOptions{
				HubURL:     reg.ControlURL,
				LocalPort:  opts.AppPort,
				RemotePort: reg.ReversePort,
				Secret:     reg.TunnelAuth,
				PublicURL:  reg.URL,
				Stdout:     w,
			})
			if err != nil {
				fmt.Fprintf(stderr, "hub re-registered but restarting the tunnel failed: %v\n", err)
				return
			}
			tunnel = nt
			fmt.Fprintf(w, "Re-registered with hub after restart; preview available at %s\n", reg.URL)
		})
		defer hb.Stop()

		fmt.Fprintf(w, "Preview available at %s\n", hubReg.URL)
	}

	// 6b. If screenshot auth was requested, log in once and reuse the session for
	// every screenshot (pages behind login render authenticated).
	if opts.Screenshot && opts.ScreenshotUser != "" {
		storage := filepath.Join(filepath.Dir(opts.ScreenshotPath), "run-local-storage.json")
		fmt.Fprintf(w, "Logging in as %q for authenticated screenshots...\n", opts.ScreenshotUser)
		if err := LoginAndSaveStorage(LoginOptions{
			AppURL: rt.AppURL(), Username: opts.ScreenshotUser, Password: opts.ScreenshotPassword,
			StoragePath: storage, MxBuildPath: mxbuildPath, RuntimeLogPath: runtimeLog,
		}); err != nil {
			fmt.Fprintf(stderr, "  screenshot login failed (continuing unauthenticated): %v\n", err)
		} else {
			opts.screenshotStorage = storage
		}
	}
	maybeScreenshot(opts, rt)

	// 7. Stay up until interrupted. With --watch, rebuild + hot-apply on every
	// project change; otherwise just keep the runtime serving.
	if opts.Watch {
		return watchAndApply(opts, serve, rt, watcher, mxbuildPath)
	}
	fmt.Fprintln(w, "(run with --watch to rebuild and hot-apply on changes; Ctrl-C to stop)")
	if waitForInterruptOrExit(rt.Exited()) {
		return runtimeStoppedError(rt)
	}
	fmt.Fprintln(w, "\nShutting down...")
	return nil
}

// runtimeStoppedError reports a runtime that stopped without being asked to.
//
// It is an ERROR, not a message: `run` returning 0 after the app has gone is
// what let a supervisor conclude everything was fine, and what made "the hub URL
// still answers" the only evidence available. A non-zero exit is what any
// supervisor — a shell loop, systemd, a container restart policy — already knows
// how to act on.
func runtimeStoppedError(rt *LocalRuntime) error {
	detail := ""
	if reason := runtimeExitReason(rt.Log()); reason != "" {
		detail = "\n  " + reason
	}
	if err := rt.ExitErr(); err != nil {
		detail += "\n  the runtime process exited with: " + err.Error()
	}
	return fmt.Errorf("the app stopped: the Mendix runtime is no longer running%s\n"+
		"  mxcli is exiting rather than staying up over a dead app — a tunnel or reverse proxy\n"+
		"  keeps answering after the runtime is gone, so \"the URL responds\" is not evidence\n"+
		"  the app is alive. Re-run to restart it.", detail)
}

// maybeScreenshot captures the app (best-effort) when --screenshot is set. A
// failure is reported but never aborts the loop — the app is still running.
func maybeScreenshot(opts LocalRunOptions, rt *LocalRuntime) {
	if !opts.Screenshot {
		return
	}
	if err := os.MkdirAll(filepath.Dir(opts.ScreenshotPath), 0o755); err != nil {
		fmt.Fprintf(opts.Stderr, "  screenshot skipped: %v\n", err)
		return
	}
	// Empty list -> the app root; multiple pages -> a screenshot set (one PNG each).
	targets := opts.ScreenshotURLs
	if len(targets) == 0 {
		targets = []string{""}
	}
	multi := len(targets) > 1
	for _, t := range targets {
		out := opts.ScreenshotPath
		if multi {
			out = screenshotOutName(opts.ScreenshotPath, t)
		}
		if err := CaptureScreenshot(ScreenshotOptions{
			URL:      resolveScreenshotURL(rt.AppURL(), t),
			OutPath:  out,
			WaitMs:   4000,
			FullPage: true,
			Viewport: "1280,800",
			Storage:  opts.screenshotStorage,
		}); err != nil {
			fmt.Fprintf(opts.Stderr, "  screenshot skipped (%s): %v\n", pageLabel(t), err)
			continue
		}
		fmt.Fprintf(opts.Stdout, "  screenshot -> %s\n", out)
	}
}

// resolveScreenshotURL resolves a --screenshot-url value against the app root:
// empty -> the root; a full http(s) URL -> as-is; anything else -> treated as a
// path relative to the app root (so "/p/customers" deep-links a specific page).
func resolveScreenshotURL(appURL, urlOrPath string) string {
	if urlOrPath == "" {
		return appURL
	}
	if strings.HasPrefix(urlOrPath, "http://") || strings.HasPrefix(urlOrPath, "https://") {
		return urlOrPath
	}
	return strings.TrimRight(appURL, "/") + "/" + strings.TrimLeft(urlOrPath, "/")
}

// pageLabel is a short human label for a screenshot target ("home" for the root).
func pageLabel(urlOrPath string) string {
	if urlOrPath == "" {
		return "home"
	}
	return urlOrPath
}

// screenshotOutName derives a per-page output path from the base path and a
// target, so a screenshot set writes distinct files: run-local.png +
// "/p/customers" -> run-local-p-customers.png; the app root -> run-local-home.png.
func screenshotOutName(basePath, urlOrPath string) string {
	ext := filepath.Ext(basePath)             // ".png"
	stem := strings.TrimSuffix(basePath, ext) // ".../run-local"
	slug := slugifyPage(urlOrPath)
	return stem + "-" + slug + ext
}

// slugifyPage turns a page URL/path into a filesystem-safe slug.
func slugifyPage(urlOrPath string) string {
	s := urlOrPath
	// Drop scheme://host for full URLs, keep the path.
	if i := strings.Index(s, "://"); i >= 0 {
		if j := strings.IndexByte(s[i+3:], '/'); j >= 0 {
			s = s[i+3+j:]
		} else {
			s = ""
		}
	}
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		} else {
			b.WriteByte('-')
		}
	}
	slug := strings.Trim(b.String(), "-")
	// collapse runs of '-'
	for strings.Contains(slug, "--") {
		slug = strings.ReplaceAll(slug, "--", "-")
	}
	if slug == "" {
		return "home"
	}
	return slug
}

// resolveRuntimeInstall returns the directory to use as MX_INSTALL_PATH (its
// runtime/ child holds the launcher and Mendix libraries), downloading the
// runtime only when nothing usable is cached. It also makes the runtime visible
// to mxbuild's serve javac step as a runtime/ sibling of modeler/.
//
// Preference order:
//  1. the dedicated runtime cache (~/.mxcli/runtime/{v}), if it has the launcher;
//  2. a runtime/ already inside the mxbuild cache (bundled or previously linked);
//  3. download the runtime, then link it into the mxbuild cache.
func resolveRuntimeInstall(version string, w io.Writer) (string, error) {
	if p := CachedRuntimePath(version); p != "" {
		if err := ensureMxBuildRuntimeSibling(version, w); err != nil {
			return "", err
		}
		return p, nil
	}
	mxbuildDir, err := MxBuildCacheDir(version)
	if err != nil {
		return "", err
	}
	launcher := filepath.Join(mxbuildDir, "runtime", "launcher", "runtimelauncher.jar")
	if fi, err := os.Stat(launcher); err == nil && !fi.IsDir() {
		// The mxbuild cache already carries a usable runtime/; use it directly.
		return mxbuildDir, nil
	}
	if _, err := DownloadRuntime(version, w); err != nil {
		return "", err
	}
	if err := ensureMxBuildRuntimeSibling(version, w); err != nil {
		return "", err
	}
	return CachedRuntimePath(version), nil
}

// waitForInterruptOrExit blocks until the process receives SIGINT or SIGTERM, or
// the runtime stops on its own. It returns true when the runtime was the cause.
//
// Waiting on the signal ALONE is what turned a runtime that terminated itself
// into an eight-hour outage nobody was told about: the hub tunnel outlives the
// app, so the preview URL kept answering while `mxcli run` sat here (see
// mxcli-formula1 FINDINGS §60). The local runtime has a bounded lifetime by
// design — its development licence has a maximum run time — so this is the
// expected ending of a long run, not an edge case.
//
// A nil exited channel (no runtime to watch) blocks forever, which is what a
// nil channel does and the right answer here.
func waitForInterruptOrExit(exited <-chan struct{}) (runtimeStopped bool) {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigCh)
	select {
	case <-sigCh:
		return false
	case <-exited:
		return true
	}
}

// clientProbeWindow bounds how long ensureClientServed waits for the app to serve
// the freshly-restarted bundle before treating it as missing. A var so tests can
// shrink it.
var clientProbeWindow = 5 * time.Second

// clientBundlePresent reports whether the rollup output exists on disk.
func clientBundlePresent(deployDir string) bool {
	_, err := os.Stat(filepath.Join(deployDir, "web", "dist", "index.js"))
	return err == nil
}

// clientBundleServed reports whether the app actually serves /dist/index.js with
// a 200 (the runtime 404s it when the bundle is missing, rendering only the
// <noscript> shell). appURL is expected to end with "/".
func clientBundleServed(appURL string) bool {
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(appURL + "dist/index.js")
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.StatusCode == http.StatusOK
}

// clientBundleServedWithin polls clientBundleServed until it succeeds or the
// window elapses (the just-restarted runtime may need a beat before static
// serving is live).
func clientBundleServedWithin(appURL string, window time.Duration) bool {
	deadline := time.Now().Add(window)
	for {
		if clientBundleServed(appURL) {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(300 * time.Millisecond)
	}
}

// ensureClientServed guarantees the browser bundle is actually being served after
// an apply, recovering if it is not. Under --watch, a structural model change has
// the serve Deploy build rewrite web/ source and clear web/dist, while the
// incremental bundler may report "success" for an intermediate bundle that does
// not match the final source — so the restarted runtime can end up 404ing
// /dist/index.js and booting only the <noscript> shell. When the bundle is not
// present+served, fall back to the reliable synchronous one-shot bundle (the same
// path the non-watch boot uses) and re-probe. A no-op when the bundle is already
// served (a pure model reload never touches web/dist).
func ensureClientServed(deployDir, appURL, mxbuildPath string, out io.Writer) error {
	// A dangling chunk is checked FIRST, because the index.js probe cannot see it:
	// the entry point is served with a 200 while a chunk it imports is missing, so
	// the apply is reported as successful and the page dies in the browser with
	// the runtime's generic error dialog. See danglingClientChunks.
	if dangling := danglingClientChunks(deployDir); len(dangling) > 0 {
		fmt.Fprintf(out, "  client bundle inconsistent — index.js imports %d chunk(s) that are not on disk (%s); re-bundling web client...\n",
			len(dangling), strings.Join(dangling, ", "))
		if err := BuildWebClient(WebClientOptions{DeployDir: deployDir, MxBuildPath: mxbuildPath, Stdout: out}); err != nil {
			return fmt.Errorf("web client re-bundle: %w", err)
		}
		if still := danglingClientChunks(deployDir); len(still) > 0 {
			return fmt.Errorf("client bundle still references missing chunks after re-bundle: %s",
				strings.Join(still, ", "))
		}
	}
	if clientBundlePresent(deployDir) && clientBundleServedWithin(appURL, clientProbeWindow) {
		return nil
	}
	fmt.Fprintln(out, "  /dist/index.js not served after apply; re-bundling web client...")
	if err := BuildWebClient(WebClientOptions{DeployDir: deployDir, MxBuildPath: mxbuildPath, Stdout: out}); err != nil {
		return fmt.Errorf("web client re-bundle: %w", err)
	}
	if !clientBundleServedWithin(appURL, clientProbeWindow) {
		return fmt.Errorf("web/dist/index.js still not served after re-bundle")
	}
	return nil
}

// sourceSettleWindow is how long the model source must stop changing before a
// rebuild starts. Two poll intervals: long enough that the gaps between an
// exec's individual unit writes do not read as "finished", short enough that a
// single editor save still rebuilds promptly.
const sourceSettleWindow = 2

// settleSource waits until the model source stops changing, and returns the
// mtime it settled at (or the zero time if interrupted).
//
// The wait is unbounded on purpose: a long exec is exactly the case this exists
// for, and rebuilding a project mid-write is worse than rebuilding it late. It
// returns as soon as the source has been quiet for sourceSettleWindow polls, so
// an ordinary single-file save costs one extra poll.
func settleSource(projectPath string, seen time.Time, poll time.Duration, sigCh <-chan os.Signal) time.Time {
	return settleSourceWith(projectPath, seen, sigCh, func() <-chan time.Time { return time.After(poll) })
}

// settleSourceWith is settleSource with the poll timer injected, so a test can
// assert how many polls a quiet source costs rather than how long it took.
//
// The distinction is not cosmetic. `tick` is a LOWER bound — time.After
// guarantees at least the duration and says nothing about the upper one — so a
// test that bounds elapsed wall-clock as a multiple of it fails on a loaded CI
// runner with no defect present, which is what this seam exists to stop.
// Each call must return a freshly-armed channel: the window is "quiet for N
// consecutive polls", so reusing one channel would collapse the wait.
func settleSourceWith(projectPath string, seen time.Time, sigCh <-chan os.Signal, tick func() <-chan time.Time) time.Time {
	quiet := 0
	for {
		select {
		case <-sigCh:
			return time.Time{}
		case <-tick():
		}
		now := sourceMTime(projectPath)
		if now.After(seen) {
			seen = now
			quiet = 0
			continue
		}
		if quiet++; quiet >= sourceSettleWindow {
			return seen
		}
	}
}

// watchAndApply polls the project for changes and applies each rebuild until the
// user interrupts (Ctrl-C). StartLocalRuntime already resolved the JVM; here we
// only rebuild via serve and let the RuntimeController decide reload vs restart.
func watchAndApply(opts LocalRunOptions, serve *ServeServer, rt *LocalRuntime, watcher *WebClientWatcher, mxbuildPath string) error {
	w := opts.Stdout

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	fmt.Fprintln(w, "Watching model + theme source for changes (serving build #1; Ctrl-C to stop)...")
	last := sourceMTime(opts.ProjectPath)
	// gen is the served build generation — a monotonic counter surfaced on every
	// apply so "did my change take?" is answerable from the log without guessing.
	// The initial boot build is generation 1.
	gen := 1
	ticker := time.NewTicker(opts.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-sigCh:
			fmt.Fprintln(w, "\nShutting down...")
			return nil
		case <-rt.Exited():
			// Same reason as the non-watch path: a runtime that stops on its own
			// must end the run rather than leave it watching for edits to apply
			// to nothing (FINDINGS §60). A restart the controller performs
			// re-arms this channel, so only an unasked-for exit lands here.
			return runtimeStoppedError(rt)
		case <-ticker.C:
			now := sourceMTime(opts.ProjectPath)
			if !now.After(last) {
				continue
			}
			// Wait for the writer to finish before reading the model.
			//
			// An `mxcli exec` of a real script rewrites the .mpr and many
			// mprcontents/*.mxunit files over several seconds. Building on the
			// first mtime bump deploys whatever happens to be on disk at that
			// instant — a half-applied model — and the escape hatch used to be
			// "run the script again". Byte-idempotent exec closed that: the
			// second run writes nothing, so nothing re-triggers the watcher and
			// the stale build is what you are left with, with no way out the tool
			// offers. Settling first means the watcher can only ever observe a
			// model that has stopped changing.
			now = settleSource(opts.ProjectPath, now, opts.PollInterval, sigCh)
			if now.IsZero() {
				// Interrupted while settling.
				fmt.Fprintln(w, "\nShutting down...")
				return nil
			}
			last = now
			gen++
			fmt.Fprintf(w, "Change detected, rebuilding (build #%d)...\n", gen)
			start := time.Now()

			// Capture the client-bundle generation and the web-source mtime before
			// the serve build, so we can tell whether the change plausibly touched
			// client source and, if so, wait for the incremental re-bundle.
			genBefore := watcher.Generation()
			webBefore := webClientSourceMTime(opts.DeployDir)

			build, err := serve.Build(BuildRequest{Target: TargetDeploy, ProjectFilePath: opts.ProjectPath})
			if err != nil {
				fmt.Fprintf(opts.Stderr, "  build error: %v\n", err)
				continue
			}
			if !build.OK() {
				// Surface the full serve response, not just the generic message —
				// it carries the real detail (e.g. the SCSS compiler's
				// "Expected expression. _x.scss 180:35" or which model errors),
				// matching the cold-build path. (findings #15, #23)
				fmt.Fprintf(opts.Stderr, "  build failed: %s\n", build.Message)
				if raw := strings.TrimSpace(string(build.Raw)); raw != "" && raw != build.Message {
					fmt.Fprintf(opts.Stderr, "    %s\n", raw)
				}
				// One failure shape is not the user's model: on Mendix 11.14+ the
				// first build in a serve process does not leave the deployment in a
				// state its own incremental build can continue from, so every rebuild
				// fails on paths inside deployment/ and reads as a corrupt deployment.
				fmt.Fprint(opts.Stderr, legacyClientBuildHint(opts.DeployDir, build.Message, string(build.Raw)))
				continue
			}
			// If the serve build touched web/ source, wait (briefly) for the
			// incremental bundler to re-bundle. WaitForRebuild settles out cleanly if
			// no rebuild materializes (the touched file isn't a rollup input — e.g. a
			// microflow edit that rewrites a web metadata file but no page/widget), so
			// this never hangs. A pure model change skips the wait entirely.
			bundled := false
			if webClientSourceMTime(opts.DeployDir).After(webBefore) {
				// Detection is a reliable ~1s with polling, so a 2.5s settle is ample
				// margin to catch a rebuild that's going to start, while keeping the
				// no-rebuild case (a model edit that only grazed web/) snappy.
				bundled, err = watcher.WaitForRebuild(genBefore, 2500*time.Millisecond, 90*time.Second)
				if err != nil {
					fmt.Fprintf(opts.Stderr, "  web client rebuild failed: %v\n", err)
					continue
				}
			}

			action, err := rt.Controller().ApplyBuild(build, rt.Restart)
			if err != nil {
				fmt.Fprintf(opts.Stderr, "  apply (%s) failed: %v\n", action, err)
				continue
			}
			// Gate the "applied" report on the browser bundle actually being served:
			// a structural change can leave the restarted runtime 404ing
			// /dist/index.js (see ensureClientServed). This recovers before we tell
			// the user the build is live.
			if err := ensureClientServed(opts.DeployDir, rt.AppURL(), mxbuildPath, opts.Stdout); err != nil {
				fmt.Fprintf(opts.Stderr, "  client bundle not served after apply: %v\n", err)
				continue
			}
			client := ""
			if bundled {
				client = fmt.Sprintf(", client re-bundled (gen %d)", watcher.Generation())
			}
			fmt.Fprintf(w, "  build #%d applied via %s in %s%s -> %s\n", gen, action, time.Since(start).Round(time.Millisecond), client, rt.AppURL())
			maybeScreenshot(opts, rt)
			// Refresh the baseline AFTER the apply so an edit made mid-build is
			// still caught on the next tick.
			last = sourceMTime(opts.ProjectPath)
		}
	}
}

// declaredJarDependencies collects the managed Java dependency coordinates the
// model declares, across every module.
func declaredJarDependencies(reader backend.FullBackend) []JarDependencyRef {
	all, err := reader.ListModuleSettings()
	if err != nil {
		return nil
	}
	var out []JarDependencyRef
	for _, ms := range all {
		if ms == nil {
			continue
		}
		for _, d := range ms.JarDependencies {
			out = append(out, JarDependencyRef{
				Group: d.GroupID, Artifact: d.ArtifactID, Version: d.Version,
			})
		}
	}
	return out
}
