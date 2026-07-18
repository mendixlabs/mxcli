// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"fmt"
	"io"
	"io/fs"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/mendixlabs/mxcli/sdk/mpr"
)

// runlocal.go is the `mxcli run --local` orchestrator: a warm, Docker-free dev
// loop. It keeps a mxbuild --serve process and a standalone runtime hot, so a
// model change rebuilds incrementally (~1s) and is applied by a hot reload_model
// (page/microflow/text) or a runtime restart (entity/view/association) — the
// serve build's restartRequired flag decides which. See
// docs/11-proposals/PROPOSAL_mxcli_dev_warm_loop.md.

// LocalRunOptions configures RunLocal.
type LocalRunOptions struct {
	// ProjectPath is the .mpr file.
	ProjectPath string
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
	Stdout            io.Writer
	Stderr            io.Writer
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
	if o.Stdout == nil {
		o.Stdout = os.Stdout
	}
	if o.Stderr == nil {
		o.Stderr = os.Stderr
	}
}

// deriveDBName turns a project file name into a safe Postgres database name:
// lowercased, non-alphanumerics collapsed to underscores, leading digit prefixed.
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

// RunLocal boots the warm local dev loop: resolve tooling, start mxbuild --serve
// and a standalone runtime, do the first build+apply, then (with Watch) rebuild
// and hot-apply on every project change until interrupted.
func RunLocal(opts LocalRunOptions) error {
	opts.applyDefaults()
	w, stderr := opts.Stdout, opts.Stderr

	// 1. Detect the project's Mendix version.
	fmt.Fprintln(w, "Detecting project version...")
	reader, err := mpr.Open(opts.ProjectPath)
	if err != nil {
		return fmt.Errorf("opening project: %w", err)
	}
	pv := reader.ProjectVersion()
	reader.Close()
	version := pv.ProductVersion
	fmt.Fprintf(w, "  Mendix version: %s\n", version)

	// 2. Ensure mxbuild + runtime are cached, and linked for the serve javac step.
	fmt.Fprintln(w, "Ensuring MxBuild and runtime are available...")
	mxbuildPath, err := DownloadMxBuild(version, w)
	if err != nil {
		return fmt.Errorf("setting up mxbuild: %w", err)
	}
	installPath, err := resolveRuntimeInstall(version, w)
	if err != nil {
		return fmt.Errorf("setting up runtime: %w", err)
	}

	// 3. Ensure the database is available. With --ensure-db, provision it (start
	// local Postgres + create the role/db if missing); otherwise just check
	// reachability and point the user at --ensure-db.
	if opts.EnsureDB {
		fmt.Fprintln(w, "Ensuring database...")
		if err := EnsureDatabase(opts.DB, w); err != nil {
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

	// 4. Start the warm build server.
	fmt.Fprintln(w, "Starting mxbuild --serve...")
	serve, err := StartServe(ServeOptions{
		Version: version,
		Host:    "127.0.0.1",
		Port:    opts.ServePort,
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
		return fmt.Errorf("initial build failed: %s\n%s", build.Message, string(build.Raw))
	}

	// 5b. Bundle the browser client (web/dist). The serve Deploy target writes the
	// client source but not the rollup bundle, so without this the app 404s on
	// /dist/index.js and renders blank. In --watch we keep an incremental bundler
	// hot (warm rollup graph + polling file detection -> ~3-4s re-bundles);
	// otherwise a single one-shot build (~7s cold) suffices.
	var watcher *WebClientWatcher
	if opts.Watch {
		fmt.Fprintln(w, "Starting incremental web client bundler...")
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

	// 6. Boot the runtime against the fresh deployment.
	rt, err := StartLocalRuntime(LocalRuntimeOptions{
		DeployDir:   opts.DeployDir,
		InstallPath: installPath,
		// JavaHome left empty: StartLocalRuntime resolves JDK 21.
		AppPort:   opts.AppPort,
		AdminPort: opts.AdminPort,
		AdminPass: opts.AdminPass,
		DB:        opts.DB,
		Stdout:    w,
		Stderr:    stderr,
	})
	if err != nil {
		return err
	}
	defer rt.Stop()

	fmt.Fprintf(w, "\nApp is running at %s\n", rt.AppURL())

	// 6b. If screenshot auth was requested, log in once and reuse the session for
	// every screenshot (pages behind login render authenticated).
	if opts.Screenshot && opts.ScreenshotUser != "" {
		storage := filepath.Join(filepath.Dir(opts.ScreenshotPath), "run-local-storage.json")
		fmt.Fprintf(w, "Logging in as %q for authenticated screenshots...\n", opts.ScreenshotUser)
		if err := LoginAndSaveStorage(LoginOptions{
			AppURL: rt.AppURL(), Username: opts.ScreenshotUser, Password: opts.ScreenshotPassword,
			StoragePath: storage, MxBuildPath: mxbuildPath,
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
		return watchAndApply(opts, serve, rt, watcher)
	}
	fmt.Fprintln(w, "(run with --watch to rebuild and hot-apply on changes; Ctrl-C to stop)")
	waitForInterrupt()
	fmt.Fprintln(w, "\nShutting down...")
	return nil
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

// waitForInterrupt blocks until the process receives SIGINT or SIGTERM.
func waitForInterrupt() {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigCh)
	<-sigCh
}

// watchAndApply polls the project for changes and applies each rebuild until the
// user interrupts (Ctrl-C). StartLocalRuntime already resolved the JVM; here we
// only rebuild via serve and let the RuntimeController decide reload vs restart.
func watchAndApply(opts LocalRunOptions, serve *ServeServer, rt *LocalRuntime, watcher *WebClientWatcher) error {
	w := opts.Stdout

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	fmt.Fprintln(w, "Watching for changes (Ctrl-C to stop)...")
	last := projectSourceMTime(opts.ProjectPath)
	ticker := time.NewTicker(opts.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-sigCh:
			fmt.Fprintln(w, "\nShutting down...")
			return nil
		case <-ticker.C:
			now := projectSourceMTime(opts.ProjectPath)
			if !now.After(last) {
				continue
			}
			last = now
			fmt.Fprintln(w, "Change detected, rebuilding...")
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
				fmt.Fprintf(opts.Stderr, "  build failed: %s\n", build.Message)
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
			client := ""
			if bundled {
				client = ", client re-bundled"
			}
			fmt.Fprintf(w, "  applied via %s in %s%s -> %s\n", action, time.Since(start).Round(time.Millisecond), client, rt.AppURL())
			maybeScreenshot(opts, rt)
			// Refresh the baseline AFTER the apply so an edit made mid-build is
			// still caught on the next tick.
			last = projectSourceMTime(opts.ProjectPath)
		}
	}
}
