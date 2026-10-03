// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/mendixlabs/mxcli/cmd/mxcli/docker"
	"github.com/mendixlabs/mxcli/cmd/mxcli/hubauth"
	"github.com/mendixlabs/mxcli/cmd/mxcli/testrunner"
	"github.com/spf13/cobra"
)

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Run a Mendix app locally in a warm dev loop",
	Long: `Run a Mendix app with a warm, Docker-free dev loop (--local).

'mxcli run --local' keeps a mxbuild --serve process and a standalone Mendix
runtime hot. The first build is cold (~10-15s); after that a model change
rebuilds incrementally (~1s) and is applied without a full restart:

  - page / microflow / text change  -> hot reload_model (no restart)
  - entity / view / association      -> runtime restart (metamodel is
                                        reconciled only at startup)

The serve build reports which is needed, so the right action is chosen
automatically. With --watch, mxcli rebuilds and hot-applies on every change.

Requirements:
  - A JDK matching the project's Settings > Model > JavaVersion (21 up to Mendix
    11.13, 25 from 11.14)
  - A reachable PostgreSQL (the devcontainer provides one); the database must
    already exist. Defaults: 127.0.0.1:5432, user 'mendix', db from the project
    name. Override with --db-host/--db-name/--db-user/--db-password.
  - --db-type hsqldb runs against the built-in file database instead of
    PostgreSQL, so no database server is needed. Its data lives under the
    project's deployment directory and is meant for local development only.

With --hub, the running app is exposed in a browser at a public URL through an
mxcli tunnel-hub, without leaving this machine: a tunnel client reverse-tunnels
the local app out over 443, and the runtime boots with ApplicationRootUrl set to
the hub URL so the app works under that origin. --hub implies --local.

--hub is available in the Linux build only. The tunnel exists to get a preview
out of a Linux container, so shipping it in the Windows and macOS binaries would
only get them flagged by endpoint security for a capability they never use. On
those platforms --hub fails with an explanatory message; run mxcli inside the
project's devcontainer to use it.

The Mendix runtime log — server-side stack traces and your microflow LOG
output — is written to <projectDir>/.mxcli/runtime.log so a server-side error
is debuggable (the browser only shows a generic dialog). mxcli both tees the
runtime JVM's stdout/stderr to the file and attaches a Mendix file log
subscriber after start, so the application log lands there too (a standalone
runtime attaches no subscriber by default). The path is printed at boot;
override with --runtime-log <path>, or "-" to disable.

With --test-endpoint, the app hosts mxcli's token-guarded test endpoint, so
'mxcli test <files> -p <app.mpr> --attach' runs a suite against this already-warm
app instead of booting a runtime of its own — a couple of seconds instead of ~30.
The endpoint has to be installed before the boot (its handler is registered by
the after-startup microflow, which only runs at startup), so it cannot be added
to an app that is already up. Your project's own after-startup microflow is
chained, not displaced, so the app still boots the way you expect. The endpoint
is removed and the project restored when the app stops.

Two things to know: tests then run against THIS app's database, not a scratch
one, and while the app is up its model carries a microflow-executing endpoint —
guarded by a per-run token, loopback-only, and limited to the generated
MxTest.Test_* microflows, but present. Leave the flag off for a normal dev loop.

With --debug, the microflow debugger is enabled at boot and a session is started,
so 'mxcli debug break/paused/step/continue' works from another terminal (use the
same -p). No breakpoints exist until you set one, so --debug alone does not change
runtime behaviour; it is turned back off on shutdown.

With --metrics, a Prometheus meter registry is registered at boot and the runtime
serves metrics at http://127.0.0.1:<admin-port>/prometheus. With --trace, the
bundled OpenTelemetry agent is attached (spans -> the runtime log via the console
exporter) with default span filters (unfiltered tracing is ~10x slower). The
console exporter omits timestamps and parent span IDs, so for flame charts pass
--trace-otlp <endpoint> to export to an OTLP collector (e.g. http://127.0.0.1:4318)
instead. Use --runtime-setting Key=Value (repeatable) to merge any other runtime setting into
the boot config (the admin config action replaces rather than merges, so mxcli
folds these into its single boot call), e.g. a different Metrics.Registries type or
custom OpenTelemetry span filters.

Examples:
  mxcli run --local -p app.mpr
  mxcli run --local -p app.mpr --watch
  mxcli run --local -p app.mpr --test-endpoint   # then: mxcli test tests/ -p app.mpr --attach
  mxcli run --local -p app.mpr --debug          # then: mxcli debug break … -p app.mpr
  mxcli run --local -p app.mpr --app-port 8081 --db-name myapp
  mxcli run --hub https://hub.example.com -p app.mpr            # browser preview
  mxcli run --hub https://hub.example.com --hub-secret u:pass -p app.mpr --watch
  mxcli auth hub login && mxcli run --hub https://hub.mxcli.org -p app.mpr  # authed hub
`,
	Run: func(cmd *cobra.Command, args []string) {
		local, _ := cmd.Flags().GetBool("local")
		hub, _ := cmd.Flags().GetString("hub")
		hubSecret, _ := cmd.Flags().GetString("hub-secret")
		hubPrefix, _ := cmd.Flags().GetString("hub-prefix")
		hubProject, _ := cmd.Flags().GetString("hub-project")
		hubSolution, _ := cmd.Flags().GetString("hub-solution")
		hubBranch, _ := cmd.Flags().GetString("hub-branch")
		hubWorktree, _ := cmd.Flags().GetString("hub-worktree")
		hubSession, _ := cmd.Flags().GetString("hub-session")
		// --hub is a cross-cutting ingress and implies the local serving path (the
		// only serving mode wired today; a future PAD path will accept --hub too).
		hubKey := ""
		if hub != "" {
			// Fail here rather than after booting the app: on a build without the
			// tunnel (everything but Linux — ADR-0009) --hub can never succeed, and
			// the user should learn that before waiting out a runtime start.
			if !docker.TunnelSupported() {
				fmt.Fprintf(os.Stderr, "Error: %v\n", docker.ErrTunnelUnsupported)
				os.Exit(1)
			}
			local = true
			// Present a per-user hub API key to an authenticated hub (MXCLI_HUB_KEY
			// env → ~/.mxcli/auth.json). Empty for open hubs; the shared --hub-secret
			// still applies.
			hubKey = hubauth.ResolveKey(hub)
		}
		if !local {
			fmt.Fprintln(os.Stderr, "Error: only --local is supported for now (use 'mxcli docker run' for the container workflow)")
			os.Exit(1)
		}
		projectPath, _ := cmd.Flags().GetString("project")
		if projectPath == "" {
			fmt.Fprintln(os.Stderr, "Error: --project (-p) is required")
			os.Exit(1)
		}
		// MxBuild requires an absolute project path; resolve a relative -p here
		// rather than surfacing MxBuild's raw "path should be an absolute path"
		// error (findings #17).
		if abs, err := filepath.Abs(projectPath); err == nil {
			projectPath = abs
		}

		watch, _ := cmd.Flags().GetBool("watch")
		testEndpoint, _ := cmd.Flags().GetBool("test-endpoint")
		ensureDB, _ := cmd.Flags().GetBool("ensure-db")
		setupOnly, _ := cmd.Flags().GetBool("setup")
		appPort, _ := cmd.Flags().GetInt("app-port")
		adminPort, _ := cmd.Flags().GetInt("admin-port")
		servePort, _ := cmd.Flags().GetInt("serve-port")
		listenAddr, _ := cmd.Flags().GetString("listen-addr")
		mxbuildPath, _ := cmd.Flags().GetString("mxbuild-path")
		dbType, _ := cmd.Flags().GetString("db-type")
		dbHost, _ := cmd.Flags().GetString("db-host")
		dbName, _ := cmd.Flags().GetString("db-name")
		dbUser, _ := cmd.Flags().GetString("db-user")
		dbPassword, _ := cmd.Flags().GetString("db-password")
		screenshot, _ := cmd.Flags().GetBool("screenshot")
		pageCheck, _ := cmd.Flags().GetBool("page-check")
		screenshotPath, _ := cmd.Flags().GetString("screenshot-path")
		screenshotURLs, _ := cmd.Flags().GetStringArray("screenshot-url")
		screenshotUser, _ := cmd.Flags().GetString("screenshot-user")
		screenshotPassword, _ := cmd.Flags().GetString("screenshot-password")
		runtimeLog, _ := cmd.Flags().GetString("runtime-log")
		debug, _ := cmd.Flags().GetBool("debug")
		debugPass, _ := cmd.Flags().GetString("debug-pass")
		metrics, _ := cmd.Flags().GetBool("metrics")
		runtimeSettings, _ := cmd.Flags().GetStringArray("runtime-setting")
		trace, _ := cmd.Flags().GetBool("trace")
		traceService, _ := cmd.Flags().GetString("trace-service")
		traceOTLP, _ := cmd.Flags().GetString("trace-otlp")
		if traceOTLP != "" {
			trace = true // --trace-otlp implies --trace
		}

		// Constant values set per configuration are not in the deployment's
		// config.json (mxbuild writes each constant's default there), so without
		// this the app runs with defaults while the model says otherwise —
		// silently. See runconstants.go.
		configuration, _ := cmd.Flags().GetString("configuration")
		constantArgs, _ := cmd.Flags().GetStringArray("constant")
		constantFlags, err := parseConstantFlags(constantArgs)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		overrides, err := constantChainFor(projectPath, configuration, constantFlags)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		reportConstantChain(os.Stdout, overrides)

		opts := docker.LocalRunOptions{
			ProjectPath:        projectPath,
			ConstantOverrides:  overrides.Values,
			Hub:                hub,
			HubSecret:          hubSecret,
			HubKey:             hubKey,
			HubPrefix:          hubPrefix,
			HubProject:         hubProject,
			HubSolution:        hubSolution,
			HubBranch:          hubBranch,
			HubWorktree:        hubWorktree,
			HubSession:         hubSession,
			AppPort:            appPort,
			AdminPort:          adminPort,
			ServePort:          servePort,
			ListenAddr:         listenAddr,
			MxBuildPath:        mxbuildPath,
			Watch:              watch,
			EnsureDB:           ensureDB,
			SetupOnly:          setupOnly,
			Screenshot:         screenshot,
			PageCheck:          pageCheck,
			ScreenshotPath:     screenshotPath,
			ScreenshotURLs:     screenshotURLs,
			ScreenshotUser:     screenshotUser,
			ScreenshotPassword: screenshotPassword,
			RuntimeLogPath:     runtimeLog,
			Debug:              debug,
			DebugPass:          debugPass,
			Metrics:            metrics,
			RuntimeSettings:    runtimeSettings,
			Trace:              trace,
			TraceService:       traceService,
			TraceOTLP:          traceOTLP,
			DB: docker.DBConfig{
				Type:     dbType,
				Host:     dbHost,
				Name:     dbName,
				User:     dbUser,
				Password: dbPassword,
			},
			Stdout: os.Stdout,
			Stderr: os.Stderr,
		}

		// --test-endpoint installs the token-guarded test endpoint into the project
		// so `mxcli test --attach` can run tests against this app without booting
		// its own runtime. It must be installed before the boot (the handler is
		// registered by the after-startup microflow, which only runs at startup)
		// and removed on the way out.
		var hosted *testrunner.HostedEndpoint
		if testEndpoint {
			if setupOnly {
				fmt.Fprintln(os.Stderr, "Error: --test-endpoint has nothing to do with --setup (which never boots the app)")
				os.Exit(1)
			}
			var err error
			hosted, err = testrunner.InstallHostedEndpoint(projectPath, os.Stdout)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			// Ctrl-C is the normal way to stop a dev loop, and it must not leave the
			// endpoint in the project. RunLocal returns on SIGINT, so the deferred
			// removal runs — but os.Exit below would skip it, hence the explicit
			// removal on the error path too. Remove is idempotent.
			defer hosted.Remove()
			opts.Env = append(opts.Env, hosted.Env...)
			opts.OnReady = func(info docker.LocalAppInfo) {
				if err := hosted.Publish(info); err != nil {
					fmt.Fprintf(os.Stderr, "Warning: could not publish the test-endpoint handshake: %v\n", err)
				}
			}
		}

		// Publish the dev-loop handshake so another mxcli process can reach this
		// app — `mxcli constant set --apply` needs the admin port and the payload
		// the runtime was booted with. Chained rather than assigned, because
		// --test-endpoint may already have installed an OnReady of its own.
		previousOnReady := opts.OnReady
		opts.OnReady = func(info docker.LocalAppInfo) {
			if previousOnReady != nil {
				previousOnReady(info)
			}
			if err := writeDevLoopHandshake(projectPath, devLoopHandshake{
				Project:    projectPath,
				PID:        os.Getpid(),
				AppPort:    info.AppPort,
				AdminPort:  info.AdminPort,
				AdminPass:  info.AdminPass,
				BootConfig: info.BootConfig,
				Started:    time.Now(),
			}); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: could not publish the dev-loop handshake: %v\n", err)
			}
		}
		defer removeDevLoopHandshake(projectPath)

		if err := docker.RunLocal(opts); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			// os.Exit skips deferred calls, so remove explicitly here.
			hosted.Remove()
			removeDevLoopHandshake(projectPath)
			os.Exit(1)
		}
	},
}

func init() {
	runCmd.Flags().String("configuration", "",
		"Which project configuration's constant values to run with (default: the only one, or \"Default\")")
	runCmd.Flags().StringArray("constant", nil,
		"Set a constant for THIS RUN only: Module.Name=value (repeatable). Wins over the configuration and is never written to the project. The value is visible in shell history and in `ps` — see docs/11-proposals/PROPOSAL_constant_values.md")
	runCmd.Flags().Bool("local", false, "Run locally without Docker (warm serve + standalone runtime)")
	runCmd.Flags().String("hub", "", "Expose the running app in a browser via your own mxcli tunnel-hub URL (e.g. https://hub.example.com). Implies --local; the app stays local and is reverse-tunnelled out")
	runCmd.Flags().String("hub-secret", "", "Shared auth secret for --hub (\"user:pass\"), matching the hub's --secret")
	runCmd.Flags().String("hub-prefix", "", "Optional subdomain prefix on the hub (org/solution/team/env): <prefix>-<project>-<branch>")
	runCmd.Flags().String("hub-project", "", "Project name for the hub subdomain + overview (default: the .mpr name)")
	runCmd.Flags().String("hub-solution", "", "Solution name to group this app under in the hub overview (multi-app solutions)")
	runCmd.Flags().String("hub-branch", "", "Branch for the hub subdomain + overview (default: the git branch)")
	runCmd.Flags().String("hub-worktree", "", "Worktree label to distinguish multiple worktrees of one branch")
	runCmd.Flags().String("hub-session", "", "Session id to group this preview under in the hub overview (default: CLAUDE_CODE_REMOTE_SESSION_ID / MXCLI_HUB_SESSION)")
	runCmd.Flags().Bool("watch", false, "Rebuild and hot-apply on every project change")
	runCmd.Flags().Bool("test-endpoint", false, "Host mxcli's token-guarded test endpoint so 'mxcli test --attach' can run tests against this app without booting its own runtime (removed on exit)")
	runCmd.Flags().Bool("ensure-db", false, "Provision the local Postgres + app database if missing (fresh-session bootstrap)")
	runCmd.Flags().Bool("setup", false, "Prepare prerequisites (cache MxBuild+runtime, ensure DB) and exit without booting — for a SessionStart hook")
	runCmd.Flags().Int("app-port", 0, "HTTP port for the app (default 8080)")
	runCmd.Flags().Int("admin-port", 0, "M2EE admin API port (default 8090)")
	runCmd.Flags().String("listen-addr", "", "Address the app binds to (default 127.0.0.1, loopback only). Set 0.0.0.0 to reach the app from other machines on the network, and pair it with a reachable root URL — either an ApplicationRootUrl naming a real host in the model, or --runtime-setting ApplicationRootUrl=http://<host>:<port>/ — because with none the runtime derives its root URL from this address and 0.0.0.0 is not a host a browser can ask for. The M2EE admin API is unaffected and always stays on 127.0.0.1.")
	runCmd.Flags().Int("serve-port", 0, "mxbuild --serve port (default 6543)")
	runCmd.Flags().String("mxbuild-path", "", "Path to the mxbuild to build with, overriding resolution (Studio Pro's bundled mxbuild on macOS/Windows, the cached CDN download on Linux)")
	runCmd.Flags().String("db-type", "", "Database type for a local run: postgresql (default) or hsqldb (the runtime's built-in file database — no server, no --db-host/--db-user/--db-password, data under <project>/deployment/data/database/hsqldb/)")
	runCmd.Flags().String("db-host", "", "Database host:port (IPv6: [::1]:5432; default 127.0.0.1:5432)")
	runCmd.Flags().String("db-name", "", "Database name (default derived from the project name)")
	runCmd.Flags().String("db-user", "", "Database user (default mendix)")
	runCmd.Flags().String("db-password", "", "Database password (default mendix)")
	runCmd.Flags().Bool("screenshot", false, "Capture a Playwright screenshot after boot and each applied change")
	runCmd.Flags().Bool("page-check", false, "Print a text verdict for each page (title, headings, error banners, row count, console errors) instead of reading a screenshot — far cheaper, and it reports console errors a PNG cannot show")
	runCmd.Flags().String("screenshot-path", "", "Screenshot output PNG (default <projectDir>/.mxcli/run-local.png)")
	runCmd.Flags().StringArray("screenshot-url", nil, "Page to screenshot: a full URL or a path relative to the app root, e.g. /p/customers (default the app root). Repeat for a multi-page set.")
	runCmd.Flags().String("screenshot-user", "", "Log in with this user before screenshotting (for pages behind login)")
	runCmd.Flags().String("screenshot-password", "", "Password for --screenshot-user")
	runCmd.Flags().String("runtime-log", "", "Write the Mendix runtime log (server stack traces + microflow LOG output) to this file for debugging (default <projectDir>/.mxcli/runtime.log; \"-\" to disable)")
	runCmd.Flags().Bool("debug", false, "Enable the microflow debugger at boot and start a session, so 'mxcli debug break/paused/…' works from another terminal (no breakpoints = no behaviour change)")
	runCmd.Flags().String("debug-pass", "", "Debugger password when --debug is set (default \"mxdebug\")")
	runCmd.Flags().Bool("metrics", false, "Register a Prometheus meter registry at boot; the runtime serves metrics at http://127.0.0.1:<admin-port>/prometheus")
	runCmd.Flags().StringArray("runtime-setting", nil, "Extra runtime setting Key=Value merged into the boot configuration (Value parsed as JSON when possible), e.g. --runtime-setting 'OpenTelemetry._RuntimeSpanFilters=[\"Loop\",\"Gateway\"]'. Repeatable.")
	runCmd.Flags().Bool("trace", false, "Enable OpenTelemetry tracing: attach the bundled agent (console exporter → the runtime log) and apply default span filters (unfiltered tracing is ~10x slower)")
	runCmd.Flags().String("trace-service", "", "OTEL_SERVICE_NAME under --trace (default: the .mpr name)")
	runCmd.Flags().String("trace-otlp", "", "Export traces to this OTLP collector endpoint (e.g. http://127.0.0.1:4318) instead of the console — needed for flame charts (the console exporter omits timestamps/parent IDs). Implies --trace")
	rootCmd.AddCommand(runCmd)
}
