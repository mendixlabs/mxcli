// SPDX-License-Identifier: Apache-2.0

package docker

import (
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

// localboot.go boots a Mendix runtime as a plain JVM process (no Docker), for the
// warm local dev loop (`mxcli run --local`). The recipe was reverse-engineered
// this session and is documented in docs/11-proposals/PROPOSAL_mxcli_dev_warm_loop.md:
//
//	java -jar <install>/runtime/launcher/runtimelauncher.jar <deployDir>   (cwd = <install>/runtime)
//	  env: M2EE_ADMIN_PASS, M2EE_ADMIN_PORT, M2EE_ADMIN_LISTEN_ADDRESSES,
//	       MX_INSTALL_PATH=<install>, MX_LOG_LEVEL, plus the FreeType LD_PRELOAD fix.
//	then over the M2EE admin API:
//	  update_appcontainer_configuration  (runtime_port)
//	  update_configuration               (BasePath, RuntimePath, DB, MicroflowConstants)
//	  start -> [database has to be updated -> execute_ddl_commands -> start]
//
// The design-time constant defaults are NOT auto-applied to a standalone runtime;
// they must be passed as MicroflowConstants or the app 530s. mxbuild writes them,
// already resolved, to <deployDir>/model/config.json — readDeploymentConstants
// lifts them from there.

// DBConfig is the database the standalone runtime connects to: either an external
// Postgres (Host/User/Password set) or the runtime's built-in file database
// (Type "HSQLDB", Host/User/Password empty — see applyDatabaseDefaults).
type DBConfig struct {
	Type     string // e.g. "PostgreSQL"
	Host     string // "host:port", e.g. "127.0.0.1:5432"
	Name     string
	User     string
	Password string
}

// IsFileBased reports whether this is the runtime's built-in file database, which
// has no host to reach. It keys on the runtime spelling that applyDatabaseDefaults
// sets (RuntimeDatabaseType), not the raw --db-type flag.
func (c DBConfig) IsFileBased() bool {
	return c.Type == RuntimeDatabaseType(DBTypeHSQLDB)
}

// LocalRuntimeOptions configures StartLocalRuntime.
type LocalRuntimeOptions struct {
	// DeployDir is the deployment directory (the runtime's BasePath). The mxbuild
	// serve Deploy target writes the model/web structure here.
	DeployDir string
	// InstallPath is the mxbuild cache root (MX_INSTALL_PATH); its runtime/ child
	// holds the launcher and the runtime libraries.
	InstallPath string
	// JavaHome is the JDK home used to launch the runtime.
	JavaHome string
	// JavaMajor is the Java release the PROJECT is built for. The runtime must be
	// launched on a JDK that can load what mxbuild compiled: a project built for
	// 25 and launched on 21 fails with UnsupportedClassVersionError. Zero means
	// DefaultJavaMajor.
	JavaMajor int
	// AdminPort is the M2EE admin API port (default 8090).
	AdminPort int
	// AppPort is the HTTP port the app serves on (default 8080).
	AppPort int
	// AdminPass is the M2EE admin password (required).
	AdminPass string
	// ListenAddr is the address the app binds to (default 127.0.0.1). It does
	// NOT affect the M2EE admin API, which is always loopback (see
	// m2eeAdminListenAddr).
	ListenAddr string
	// DTAPMode is D/A/T/P (default "D").
	DTAPMode string
	// ApplicationRootUrl, when set, is the public URL the app is reached at
	// (e.g. https://hub.example.com). Mendix uses it for absolute-URL generation
	// and to accept requests whose Host differs from the listen address — needed
	// when the app is served through an external tunnel/reverse proxy rather than
	// localhost. Empty for a plain local run.
	ApplicationRootUrl string
	// RuntimeSettings are extra update_configuration keys merged into the boot
	// payload (e.g. "Metrics.Registries", "OpenTelemetry._RuntimeSpanFilters").
	// Merged here because the admin action replaces rather than merges.
	RuntimeSettings map[string]any
	// ConstantOverrides are the running configuration's constant values,
	// merged over the defaults mxbuild wrote into the deployment. See
	// mergeConstantOverrides.
	ConstantOverrides map[string]string
	// Trace attaches the bundled OpenTelemetry Java agent to the runtime JVM
	// (traces via the console exporter → the tee'd runtime log). The caller should
	// also set OpenTelemetry._RuntimeSpanFilters via RuntimeSettings — unfiltered
	// per-activity tracing is ~10x slower.
	Trace bool
	// TraceServiceName is OTEL_SERVICE_NAME when Trace is set (default: the app).
	TraceServiceName string
	// TraceOTLPEndpoint, when set, switches the traces exporter from console to
	// OTLP and points it at this collector (e.g. http://127.0.0.1:4318) — needed
	// for flame charts, since the console exporter omits timestamps/parent IDs.
	// Implies Trace. User-set OTEL_* vars still take precedence.
	TraceOTLPEndpoint string
	// DB is the database the runtime connects to.
	DB DBConfig
	// ReadyTimeout bounds how long StartLocalRuntime waits for the admin API
	// (default 90s).
	ReadyTimeout time.Duration
	// Env are extra "KEY=value" entries layered onto the runtime JVM's
	// environment, last-wins over the inherited process environment. Used to hand
	// the runtime a secret that must not be written to disk — the test runner
	// passes its per-run endpoint token this way rather than baking it into the
	// generated Java source, which would land in the user's javasource/ tree.
	Env []string
	// Stdout/Stderr receive progress messages (default os.Stdout/os.Stderr).
	Stdout io.Writer
	Stderr io.Writer
	// RuntimeLogPath, when set, tees the runtime JVM's stdout+stderr to this file
	// (appended across restarts) so the warm loop is debuggable — server-side
	// stack traces and microflow LOG output land somewhere a developer can read.
	// Empty disables the file tee (the in-memory buffer for startup errors is
	// unaffected). Findings #25.
	RuntimeLogPath string
}

// LocalRuntime is a booted standalone runtime process plus its admin connection.
type LocalRuntime struct {
	opts    LocalRuntimeOptions
	cmd     *exec.Cmd
	log     *syncBuffer
	logFile *os.File // open when RuntimeLogPath is set; runtime stdout/stderr tee
	m2ee    M2EEOptions
	ctrl    *RuntimeController
	// bootConfig is the update_configuration payload sent at start; see BootConfig.
	bootConfig map[string]any

	// exited is closed by the reaper when the JVM stops, and exitErr holds what
	// Wait returned. Nothing used to wait on the runtime at all, so an exited
	// JVM stayed an unreaped zombie — and Signal(0), which alive() asked,
	// SUCCEEDS on a zombie. Measured: proc state `Z`, Signal(0) nil; after
	// Wait(), "process already finished". So a runtime that had terminated
	// itself hours earlier still read as alive (mxcli-formula1 FINDINGS §60).
	exitMu  sync.Mutex
	exited  chan struct{}
	exitErr error
}

// watchExit reaps the runtime process and records how it went.
//
// Exactly one goroutine may Wait on a process: a second waiter blocks forever,
// so stopProcess consults this instead of taking its own.
func (rt *LocalRuntime) watchExit() {
	cmd := rt.cmd
	if cmd == nil || cmd.Process == nil {
		return
	}
	rt.exitMu.Lock()
	rt.exited = make(chan struct{})
	rt.exitErr = nil
	done := rt.exited
	rt.exitMu.Unlock()

	go func() {
		err := cmd.Wait()
		rt.exitMu.Lock()
		rt.exitErr = err
		rt.exitMu.Unlock()
		close(done)
	}()
}

// Exited is closed when the runtime process stops, for whatever reason. A
// runtime that was never started yields a nil channel, which blocks forever —
// the right answer for "tell me when this stops".
func (rt *LocalRuntime) Exited() <-chan struct{} {
	rt.exitMu.Lock()
	defer rt.exitMu.Unlock()
	return rt.exited
}

// ExitErr returns what the runtime process exited with: nil for a clean exit or
// while it is still running.
func (rt *LocalRuntime) ExitErr() error {
	rt.exitMu.Lock()
	defer rt.exitMu.Unlock()
	return rt.exitErr
}

// runtimeExitReason picks the cause of a runtime shutdown out of its own log,
// or returns "" when it cannot tell.
//
// The one cause worth naming is the developer licence's maximum run time, which
// is why §60 happened at all: the runtime terminates itself after a few hours
// and the message is right there in the log, having also warned at every boot.
// Measured lifetimes on one machine were 3h52m and 5h07m — not a fixed number,
// and shorter than a working session.
//
// Nothing is guessed. An exit this cannot explain is reported as an exit, since
// inventing a cause is worse than reporting none.
func runtimeExitReason(log string) string {
	if strings.Contains(log, "Maximum run time exceeded") {
		return "the runtime terminated itself: the local standalone runtime uses a " +
			"development licence with a maximum run time (\"Maximum run time exceeded, " +
			"framework is now terminating\"), which it also warns about at boot"
	}
	if strings.Contains(log, "java.lang.OutOfMemoryError") {
		return "the runtime JVM ran out of memory (java.lang.OutOfMemoryError)"
	}
	return ""
}

func (o *LocalRuntimeOptions) applyDefaults() {
	if o.AdminPort == 0 {
		o.AdminPort = 8090
	}
	if o.AppPort == 0 {
		o.AppPort = 8080
	}
	if o.ListenAddr == "" {
		o.ListenAddr = "127.0.0.1"
	}
	if o.DTAPMode == "" {
		o.DTAPMode = "D"
	}
	if o.ReadyTimeout == 0 {
		o.ReadyTimeout = 90 * time.Second
	}
	if o.Stdout == nil {
		o.Stdout = os.Stdout
	}
	if o.Stderr == nil {
		o.Stderr = os.Stderr
	}
}

// runtimeDir is <install>/runtime.
func (o *LocalRuntimeOptions) runtimeDir() string { return filepath.Join(o.InstallPath, "runtime") }

// launcherJar is the runtime launcher jar.
func (o *LocalRuntimeOptions) launcherJar() string {
	return filepath.Join(o.runtimeDir(), "launcher", "runtimelauncher.jar")
}

// jvmArgs builds the JVM argument list for the local runtime.
//
// The two -Dmendix.* system properties mount the runtime's development servlets,
// including /dev/preview_execute_oql — the endpoint `mxcli oql` calls. Docker
// mode passes the same flags via docker-compose (see
// templates/docker-compose.yml); the local boot must set them too, or `mxcli
// oql` against a `mxcli run --local` app fails with "Action not found" and
// silently returns 0 rows (findings #36). `run --local` is always a development
// loop (it forces DTAPMode=D), so enabling live preview unconditionally matches
// what docker mode already does.
func (o *LocalRuntimeOptions) jvmArgs() []string {
	return []string{
		"-Dmendix.live-preview=enabled",
		"-Dmendix.running.locally.by.studiopro=true",
		"-jar", o.launcherJar(), o.DeployDir,
	}
}

// m2eeAdminListenAddr is the address the M2EE admin API binds to, and the
// address mxcli talks to it on. Always loopback, deliberately independent of
// LocalRuntimeOptions.ListenAddr: the admin API is a privileged surface (it
// boots the app, rewrites configuration, runs actions) and widening the app
// bind must not widen it onto the network as a side effect. A wildcard would
// not be a valid client destination anyway — 0.0.0.0 is a bind address, not a
// host, so a client dialling it is not portable.
const m2eeAdminListenAddr = "127.0.0.1"

// localRuntimeEnv builds the environment for the runtime JVM, layered on the
// current process environment. PrepareMxCommand later adds the FreeType fix.
// o.Env is appended last so a caller-supplied value wins over both the inherited
// environment and these defaults.
func localRuntimeEnv(o LocalRuntimeOptions) []string {
	env := append(os.Environ(),
		"M2EE_ADMIN_PASS="+o.AdminPass,
		fmt.Sprintf("M2EE_ADMIN_PORT=%d", o.AdminPort),
		"M2EE_ADMIN_LISTEN_ADDRESSES="+m2eeAdminListenAddr,
		"MX_INSTALL_PATH="+o.InstallPath,
		"MX_LOG_LEVEL=i",
	)
	return append(env, o.Env...)
}

// otelAgentJar locates the OpenTelemetry Java agent bundled with the runtime
// (<runtimeDir>/agents/opentelemetry-javaagent*.jar). The version suffix varies,
// so it is globbed.
func (o *LocalRuntimeOptions) otelAgentJar() (string, error) {
	pattern := filepath.Join(o.runtimeDir(), "agents", "opentelemetry-javaagent*.jar")
	matches, _ := filepath.Glob(pattern)
	if len(matches) == 0 {
		return "", fmt.Errorf("OpenTelemetry agent not found at %s (this runtime may not bundle it)", pattern)
	}
	return matches[0], nil
}

// withTraceEnv layers the OpenTelemetry Java-agent + OTEL_* env onto base. The
// agent is always attached (via JAVA_TOOL_OPTIONS, appended to any existing
// value). The traces exporter defaults to the console (lands in the tee'd
// runtime log) — but when otlpEndpoint is set, it is switched to OTLP and pointed
// at that collector (protocol http/protobuf), which is what flame-chart tools
// need since the console exporter omits timestamps and parent span IDs. Metrics
// and logs exporters default to none. None of these are overridden if the caller
// already set the corresponding OTEL_* var, so a fully hand-rolled env still works.
func withTraceEnv(base []string, agentJar, serviceName, otlpEndpoint string) []string {
	has := func(key string) bool {
		for _, e := range base {
			if strings.HasPrefix(e, key+"=") {
				return true
			}
		}
		return false
	}
	get := func(key string) string {
		for _, e := range base {
			if strings.HasPrefix(e, key+"=") {
				return e[len(key)+1:]
			}
		}
		return ""
	}
	jto := strings.TrimSpace(get("JAVA_TOOL_OPTIONS") + " -javaagent:" + agentJar)

	out := make([]string, 0, len(base)+5)
	for _, e := range base {
		if strings.HasPrefix(e, "JAVA_TOOL_OPTIONS=") {
			continue // replaced below with the agent appended
		}
		out = append(out, e)
	}
	out = append(out, "JAVA_TOOL_OPTIONS="+jto)
	if !has("OTEL_SERVICE_NAME") && serviceName != "" {
		out = append(out, "OTEL_SERVICE_NAME="+serviceName)
	}
	if !has("OTEL_TRACES_EXPORTER") {
		if otlpEndpoint != "" {
			// Export to an OTLP collector so call trees + durations are usable.
			out = append(out, "OTEL_TRACES_EXPORTER=otlp")
			if !has("OTEL_EXPORTER_OTLP_PROTOCOL") {
				out = append(out, "OTEL_EXPORTER_OTLP_PROTOCOL=http/protobuf")
			}
			if !has("OTEL_EXPORTER_OTLP_ENDPOINT") {
				out = append(out, "OTEL_EXPORTER_OTLP_ENDPOINT="+otlpEndpoint)
			}
		} else {
			out = append(out, "OTEL_TRACES_EXPORTER=console")
		}
	}
	if !has("OTEL_METRICS_EXPORTER") {
		out = append(out, "OTEL_METRICS_EXPORTER=none")
	}
	if !has("OTEL_LOGS_EXPORTER") {
		out = append(out, "OTEL_LOGS_EXPORTER=none")
	}
	return out
}

// appContainerParams is the update_appcontainer_configuration payload (which port
// and address the app itself listens on).
func appContainerParams(o LocalRuntimeOptions) map[string]any {
	return map[string]any{
		"runtime_port":             o.AppPort,
		"runtime_listen_addresses": o.ListenAddr,
	}
}

// runtimeConfigParams is the update_configuration payload. constants are the
// app's MicroflowConstants (name -> resolved default); pass an empty map for an
// app with no constants.
func runtimeConfigParams(o LocalRuntimeOptions, constants map[string]string) map[string]any {
	if constants == nil {
		constants = map[string]string{}
	}
	params := map[string]any{
		"BasePath":           o.DeployDir,
		"RuntimePath":        o.runtimeDir(),
		"DTAPMode":           o.DTAPMode,
		"DatabaseType":       o.DB.Type,
		"DatabaseHost":       o.DB.Host,
		"DatabaseName":       o.DB.Name,
		"DatabaseUserName":   o.DB.User,
		"DatabasePassword":   o.DB.Password,
		"MicroflowConstants": constants,
	}
	// Only set ApplicationRootUrl when the app is served behind an external URL;
	// on a plain local run the runtime defaults it from the listen address.
	if o.ApplicationRootUrl != "" {
		params["ApplicationRootUrl"] = o.ApplicationRootUrl
	}
	// Overlay extra runtime settings (e.g. Metrics.Registries,
	// OpenTelemetry._RuntimeSpanFilters) into this SAME payload, because the
	// map below overwrites by key: a caller passing
	// `--runtime-setting MicroflowConstants=…` replaces the constants map built
	// above rather than adding to it, and at boot there is nothing to fall back
	// on for BasePath/DatabaseName (they are not in config.json). Folding
	// everything into mxcli's single boot call is what keeps that safe.
	//
	// The admin action also has no read-back — get_configuration,
	// get_current_configuration, runtime_config and get_current_runtime_status
	// are all "Action not found" on 11.12.1 — so a caller cannot merge by
	// reading first.
	//
	// Measured on 11.12.1, for anyone tempted to drive this API live. Both
	// readings are from a microflow returning two constants over the test
	// endpoint, so they are the app's own view rather than the API's:
	//
	//   - The call is STAGED, not applied. The running app keeps the old value
	//     until the next reload_model, while update_configuration still answers
	//     result:0. "Set a constant on a running app" is therefore the pair.
	//   - MicroflowConstants MERGES onto the running configuration. A payload
	//     carrying one constant left the other at the value an EARLIER
	//     update_configuration had given it — not at its deployment default,
	//     and not blank. Payload shape does not change this: params carrying
	//     only MicroflowConstants behaved the same as the full boot config.
	//
	// The second point is disputed. mxcli-chat FINDINGS §57 reports the opposite
	// on 11.13.0 — a partial map blanking every omitted constant — inferred from
	// a downstream symptom rather than read back. It did not reproduce here on
	// 11.12.1 in either payload shape. Do not rely on either behaviour: ApplyConstants
	// sends the whole resolved chain, which is correct under both readings, and
	// that is the reason this disagreement costs mxcli nothing.
	for k, v := range o.RuntimeSettings {
		params[k] = v
	}
	return params
}

// deploymentConfig mirrors the parts of <deployDir>/model/config.json mxbuild
// writes: a pre-resolved Constants map (name -> default value as a string).
type deploymentConfig struct {
	Constants map[string]string `json:"Constants"`
}

// readDeploymentConstants lifts the resolved constant defaults mxbuild wrote to
// <deployDir>/model/config.json. A standalone runtime does not apply design-time
// defaults itself, so these must be fed back in via update_configuration. Missing
// file / no constants yields an empty map (not an error): an app may have none.
func readDeploymentConstants(deployDir string) (map[string]string, error) {
	path := filepath.Join(deployDir, "model", "config.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]string{}, nil
		}
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	var cfg deploymentConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	if cfg.Constants == nil {
		return map[string]string{}, nil
	}
	return cfg.Constants, nil
}

// mergeConstantOverrides layers a configuration's constant values over the
// defaults mxbuild resolved into the deployment.
//
// The merge direction is the whole point. config.json carries every constant's
// *default*, which is what the runtime needs for the ones a configuration does
// not override; the configuration's values win where both exist. Replacing the
// map instead — the shape `--runtime-setting MicroflowConstants=…` has — drops
// every constant the configuration is silent about, and the app 530s on the
// first microflow that reads one.
func mergeConstantOverrides(defaults, overrides map[string]string) map[string]string {
	if len(overrides) == 0 {
		return defaults
	}
	merged := make(map[string]string, len(defaults)+len(overrides))
	for k, v := range defaults {
		merged[k] = v
	}
	for k, v := range overrides {
		merged[k] = v
	}
	return merged
}

// ensureDataDirs creates the data/{files,tmp,model-upload} directories the
// runtime expects under the deployment dir. m2ee normally creates these; a bare
// serve Deploy / unzipped .mda does not.
func ensureDataDirs(deployDir string) error {
	for _, sub := range []string{"files", "tmp", "model-upload"} {
		if err := os.MkdirAll(filepath.Join(deployDir, "data", sub), 0o755); err != nil {
			return err
		}
	}
	return nil
}

// StartLocalRuntime boots the runtime process, configures it over the admin API,
// and runs the DB-aware start cycle. On success the app is serving on AppPort.
// Call Stop() to shut it down.
func StartLocalRuntime(opts LocalRuntimeOptions) (*LocalRuntime, error) {
	opts.applyDefaults()
	if opts.AdminPass == "" {
		return nil, fmt.Errorf("AdminPass is required")
	}
	if opts.InstallPath == "" {
		return nil, fmt.Errorf("InstallPath is required")
	}
	if opts.JavaHome == "" {
		// The JDK has to match what the project was COMPILED for, not a constant:
		// class files from a newer release cannot be loaded by an older JVM.
		jh, err := resolveJDK(opts.JavaMajor)
		if err != nil {
			return nil, err
		}
		opts.JavaHome = jh
	}
	if _, err := os.Stat(opts.launcherJar()); err != nil {
		return nil, fmt.Errorf("runtime launcher not found at %s (incomplete mxbuild cache?): %w", opts.launcherJar(), err)
	}
	if err := ensureDataDirs(opts.DeployDir); err != nil {
		return nil, fmt.Errorf("creating runtime data directories: %w", err)
	}

	rt := &LocalRuntime{
		opts: opts,
		m2ee: M2EEOptions{
			// The admin API is always loopback, independent of the app bind:
			// see m2eeAdminListenAddr.
			Host:    m2eeAdminListenAddr,
			Port:    opts.AdminPort,
			Token:   opts.AdminPass,
			Direct:  true,
			Timeout: 150 * time.Second,
		},
	}
	rt.ctrl = NewRuntimeController(rt.m2ee)
	// Attach a file log subscriber (post-start, inside Start) so the runtime's
	// application log lands in the same file the JVM stdout/stderr is tee'd to.
	// An absolute path keeps the runtime (cwd = <install>/runtime) and the JVM
	// tee (cwd = mxcli's) pointed at one file. Findings #25.
	if opts.RuntimeLogPath != "" {
		logPath := opts.RuntimeLogPath
		if abs, err := filepath.Abs(logPath); err == nil {
			logPath = abs
		}
		rt.ctrl.LogSubscriberFile = logPath
		rt.ctrl.Stdout = opts.Stdout
	}

	if err := rt.spawnAndConfigure(); err != nil {
		return nil, err
	}
	if _, err := rt.ctrl.Start(); err != nil {
		_ = rt.Stop()
		return nil, fmt.Errorf("starting runtime: %w\n--- runtime output ---\n%s", err, rt.log.String())
	}
	fmt.Fprintf(opts.Stdout, "Runtime started; app serving at %s\n", rt.AppURL())
	return rt, nil
}

// spawnAndConfigure launches (or relaunches) the JVM and applies the admin
// configuration up to but not including start. It is used both for the initial
// boot and for a restart (config is per-process and must be re-applied).
func (rt *LocalRuntime) spawnAndConfigure() error {
	javaExe := JavaExePath(rt.opts.JavaHome)
	cmd := exec.Command(javaExe, rt.opts.jvmArgs()...)
	cmd.Dir = rt.opts.runtimeDir()
	cmd.Env = localRuntimeEnv(rt.opts)
	if rt.opts.Trace {
		jar, err := rt.opts.otelAgentJar()
		if err != nil {
			return fmt.Errorf("--trace: %w", err)
		}
		cmd.Env = withTraceEnv(cmd.Env, jar, rt.opts.TraceServiceName, rt.opts.TraceOTLPEndpoint)
	}
	PrepareMxCommand(cmd) // FreeType LD_PRELOAD workaround, layered on cmd.Env
	setProcessGroup(cmd)  // reap any JVM child on Stop so the port is freed
	// The in-memory buffer always captures output for startup-failure reporting.
	// When RuntimeLogPath is set, also tee stdout+stderr to that file so the
	// runtime's own log (stack traces, microflow LOG output) is readable while
	// the warm loop runs — otherwise it is swallowed (findings #25).
	log := &syncBuffer{}
	var out io.Writer = log
	if rt.opts.RuntimeLogPath != "" {
		if f, err := rt.openRuntimeLog(); err == nil {
			rt.logFile = f
			out = io.MultiWriter(log, f)
		} else {
			fmt.Fprintf(rt.opts.Stdout, "  (could not open runtime log %s: %v)\n", rt.opts.RuntimeLogPath, err)
		}
	}
	cmd.Stdout = out
	cmd.Stderr = out
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("launching runtime JVM: %w", err)
	}
	rt.cmd = cmd
	rt.log = log
	// Reap from the moment it starts, not from Stop: an unreaped exit is what
	// made alive() lie for eight hours (§60).
	rt.watchExit()

	if err := rt.waitAdminReady(rt.opts.ReadyTimeout); err != nil {
		_ = rt.Stop()
		return fmt.Errorf("runtime admin API did not come up: %w\n--- runtime output ---\n%s", err, log.String())
	}

	if _, err := CallM2EE(rt.m2ee, "update_appcontainer_configuration", appContainerParams(rt.opts)); err != nil {
		return fmt.Errorf("update_appcontainer_configuration: %w", err)
	}
	constants, err := readDeploymentConstants(rt.opts.DeployDir)
	if err != nil {
		return err
	}
	constants = mergeConstantOverrides(constants, rt.opts.ConstantOverrides)
	// Kept so a LATER caller can re-send this exact payload with a different
	// constants map. The admin action has no read-back, so the only way to change
	// one setting without guessing at the rest is to have kept the rest.
	rt.bootConfig = runtimeConfigParams(rt.opts, constants)
	if _, err := CallM2EE(rt.m2ee, "update_configuration", rt.bootConfig); err != nil {
		return fmt.Errorf("update_configuration: %w", err)
	}
	return nil
}

// BootConfig is the update_configuration payload this runtime was started with.
//
// It is what makes a live constant change possible from ANOTHER process: that
// process cannot read the configuration back (the admin API has no such action),
// so it re-sends this with MicroflowConstants replaced. See ApplyConstants.
func (rt *LocalRuntime) BootConfig() map[string]any { return rt.bootConfig }

// AdminOptions is how to reach this runtime's admin API.
func (rt *LocalRuntime) AdminOptions() M2EEOptions { return rt.m2ee }

// ApplyConstants changes a running app's constant values: it re-sends a boot
// payload with MicroflowConstants replaced, then reloads the model.
//
// BOTH calls are required, and that is the whole reason this is a function
// rather than a one-liner at the call site. Measured on 11.12.1:
// update_configuration is STAGED — the running app keeps its old values and
// still answers result:0 — and only the next reload_model applies them. Shipping
// just the first call would produce a command that reports success and changes
// nothing, which is the exact failure this feature exists to remove
// (mxcli-chat FINDINGS §33).
//
// It re-sends the whole payload rather than MicroflowConstants alone because
// the admin action offers no read-back to merge against: whatever is not sent is
// simply not in the configuration afterwards.
func ApplyConstants(m2ee M2EEOptions, bootConfig map[string]any, constants map[string]string) error {
	if len(bootConfig) == 0 {
		return fmt.Errorf("no boot configuration to re-send")
	}
	payload := make(map[string]any, len(bootConfig))
	for k, v := range bootConfig {
		payload[k] = v
	}
	if constants == nil {
		constants = map[string]string{}
	}
	payload["MicroflowConstants"] = constants

	if _, err := CallM2EE(m2ee, "update_configuration", payload); err != nil {
		return fmt.Errorf("update_configuration: %w", err)
	}
	if _, err := CallM2EE(m2ee, "reload_model", nil); err != nil {
		return fmt.Errorf("reload_model (the new values are staged but NOT in effect): %w", err)
	}
	return nil
}

// openRuntimeLog opens (creating the parent dir) the runtime log for appending
// and writes a start marker. A prior handle (from an earlier spawn/restart) is
// closed first so the file is reused across restarts rather than leaked.
func (rt *LocalRuntime) openRuntimeLog() (*os.File, error) {
	if rt.logFile != nil {
		_ = rt.logFile.Close()
		rt.logFile = nil
	}
	if err := os.MkdirAll(filepath.Dir(rt.opts.RuntimeLogPath), 0o755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(rt.opts.RuntimeLogPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, err
	}
	fmt.Fprintf(f, "\n=== runtime start %s ===\n", time.Now().Format(time.RFC3339))
	return f, nil
}

// waitAdminReady polls runtime_status until the admin API responds or times out.
func (rt *LocalRuntime) waitAdminReady(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, err := CallM2EE(rt.m2ee, "runtime_status", nil); err == nil {
			return nil
		}
		if !rt.alive() {
			return fmt.Errorf("runtime process exited during startup")
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("timed out after %s", timeout)
}

// Controller returns the runtime controller for applying serve build results.
func (rt *LocalRuntime) Controller() *RuntimeController { return rt.ctrl }

// Restart relaunches the JVM and re-applies configuration (but does not start —
// use it as the ApplyBuild restart callback, which runs Start afterwards).
func (rt *LocalRuntime) Restart() error {
	_ = rt.stopProcess()
	return rt.spawnAndConfigure()
}

// AppURL is the base URL the app serves on.
func (rt *LocalRuntime) AppURL() string {
	return fmt.Sprintf("http://%s:%d/", rt.opts.ListenAddr, rt.opts.AppPort)
}

// HealthOK reports whether the app answers an HTTP request (any status < 500).
func (rt *LocalRuntime) HealthOK() bool {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(rt.AppURL())
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode < 500
}

// Log returns the captured runtime output (for diagnostics).
func (rt *LocalRuntime) Log() string { return rt.log.String() }

// alive reports whether the runtime process is still running.
//
// Signal(0) is only a correct liveness test on POSIX, and even there only
// BECAUSE watchExit reaps: it succeeds on an unreaped zombie — measured, proc
// state `Z`, err nil — and returns "process already finished" once Wait has run.
// Before the reaper existed this function reported a runtime that had terminated
// itself hours earlier as alive (mxcli-formula1 FINDINGS §60). Removing watchExit
// silently breaks that on POSIX. On Windows Signal(0) is not supported at all, so
// processAlive uses WaitForSingleObject instead (see procgroup_windows.go).
func (rt *LocalRuntime) alive() bool {
	if rt.cmd == nil || rt.cmd.Process == nil {
		return false
	}
	return processAlive(rt.cmd.Process)
}

// Stop shuts the runtime down gracefully via the admin API, then terminates the
// process (SIGTERM, SIGKILL after a grace period).
func (rt *LocalRuntime) Stop() error {
	_, _ = CallM2EE(rt.m2ee, "shutdown", nil) // best-effort graceful stop
	return rt.stopProcess()
}

func (rt *LocalRuntime) stopProcess() error {
	if rt.cmd == nil || rt.cmd.Process == nil {
		return nil
	}
	_ = signalProcessGroup(rt.cmd.Process, syscall.SIGTERM)
	// The reaper owns the Wait. A second one blocks forever, so this waits on
	// the channel the reaper closes rather than calling Wait again — and a child
	// that had already exited (the §60 case) is therefore stopped instantly
	// instead of hanging the shutdown for 8 seconds and then some.
	done := rt.Exited()
	if done == nil {
		// Nothing is reaping (a runtime assembled outside spawnAndConfigure):
		// take the wait here so the child is still not left a zombie.
		ch := make(chan error, 1)
		go func() { ch <- rt.cmd.Wait() }()
		closed := make(chan struct{})
		go func() { <-ch; close(closed) }()
		done = closed
	}
	select {
	case <-done:
	case <-time.After(8 * time.Second):
		_ = killProcessGroup(rt.cmd.Process)
		<-done
	}
	rt.cmd = nil
	rt.exitMu.Lock()
	rt.exited, rt.exitErr = nil, nil
	rt.exitMu.Unlock()
	if rt.logFile != nil {
		_ = rt.logFile.Close()
		rt.logFile = nil
	}
	return nil
}
