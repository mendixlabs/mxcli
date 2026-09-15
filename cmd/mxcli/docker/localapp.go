// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

// LocalAppOptions configures StartLocalApp.
type LocalAppOptions struct {
	// ProjectPath is the .mpr file.
	ProjectPath string
	// DeployDir is the deployment directory (default <project dir>/deployment).
	DeployDir string
	// AppPort / AdminPort / ServePort default to 8080 / 8090 / 6543.
	AppPort   int
	AdminPort int
	ServePort int
	// AdminPass is the M2EE admin password (defaults to the local-run password).
	AdminPass string
	// DB is the database to connect to; empty fields take the run --local
	// defaults (PostgreSQL at 127.0.0.1:5432, user/password mendix, database
	// name derived from the project file name).
	DB DBConfig
	// EnsureDB provisions the local Postgres + database when missing instead of
	// only checking reachability.
	EnsureDB bool
	// SkipBuild boots against whatever is already in DeployDir.
	SkipBuild bool
	// RuntimeLogPath tees the runtime JVM output and the runtime's own
	// application log to this file.
	RuntimeLogPath string
	// Env are extra "KEY=value" entries for the runtime JVM (see
	// LocalRuntimeOptions.Env) — how a secret reaches the runtime without being
	// written to disk.
	Env []string
	// ConstantOverrides are the constant values this run should use, layered over
	// the defaults mxbuild wrote into deployment/model/config.json.
	//
	// A headless boot needs these for the same reason `run --local` does: the
	// deployment carries only each constant's DEFAULT, so an app booted without
	// them runs as if no configuration existed. Missing here, `mxcli test --local`
	// ran a suite against different values than the same suite under `--attach`,
	// which runs against an app `run --local` booted — silently, since a constant
	// resolving to the wrong value is not an error. See
	// docs/11-proposals/PROPOSAL_constant_values.md and mxcli-chat FINDINGS §33.
	ConstantOverrides map[string]string
	// Stdout/Stderr receive progress messages.
	Stdout io.Writer
	Stderr io.Writer
}

// runtimeOptions is the LocalAppOptions -> LocalRuntimeOptions mapping, split
// out so the forwarding is assertable without booting anything. Every field the
// runtime needs has to appear here; one omitted is invisible until an app runs
// with the wrong configuration.
func (o LocalAppOptions) runtimeOptions(installPath string) LocalRuntimeOptions {
	javaMajor, _ := ProjectJavaMajor(o.ProjectPath)
	return LocalRuntimeOptions{
		DeployDir:         o.DeployDir,
		InstallPath:       installPath,
		JavaMajor:         javaMajor,
		AppPort:           o.AppPort,
		AdminPort:         o.AdminPort,
		AdminPass:         o.AdminPass,
		DB:                o.DB,
		RuntimeLogPath:    o.RuntimeLogPath,
		Env:               o.Env,
		ConstantOverrides: o.ConstantOverrides,
		Stdout:            o.Stdout,
		Stderr:            o.Stderr,
	}
}

// LocalApp is a booted local app: an mxbuild serve server plus the standalone
// runtime it deployed to. It is the Docker-free equivalent of `docker compose
// up` for callers that need an app running and then stopped again.
type LocalApp struct {
	Runtime *LocalRuntime
	// Version is the project's Mendix version.
	Version string
	// RuntimeLogPath is where the runtime log is being written (may be empty).
	RuntimeLogPath string

	serve *ServeServer
}

func (o *LocalAppOptions) applyDefaults() {
	// Resolve the project path before anything is derived from it: DeployDir
	// below, and the runtime's own working directory, both hang off it, and a
	// relative value would leave them relative to whatever cwd the caller
	// happened to have. ServeServer.Build absolutizes too — that is the backstop
	// for MxBuild's own requirement; this is so the paths around it agree.
	if o.ProjectPath != "" && !filepath.IsAbs(o.ProjectPath) {
		if abs, err := filepath.Abs(o.ProjectPath); err == nil {
			o.ProjectPath = abs
		}
	}
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
	if o.Stdout == nil {
		o.Stdout = os.Stdout
	}
	if o.Stderr == nil {
		o.Stderr = os.Stderr
	}
}

// StartLocalApp builds the project with mxbuild and boots the standalone
// runtime against the result — the same sequence as `mxcli run --local`, minus
// everything a headless caller does not need (no web client bundle, no hub, no
// watch loop, no screenshots).
//
// The caller owns the returned app and must Stop it.
func StartLocalApp(opts LocalAppOptions) (*LocalApp, error) {
	opts.applyDefaults()
	w := opts.Stdout

	if err := checkLocalAppPortsFree(opts); err != nil {
		return nil, err
	}
	if err := checkDeployDirIsBuildable(opts); err != nil {
		return nil, err
	}

	// 1. Project version → which mxbuild and runtime to use.
	reader, err := openReadOnly(opts.ProjectPath)
	if err != nil {
		return nil, fmt.Errorf("opening project: %w", err)
	}
	version := reader.ProjectVersion().ProductVersion
	reader.Disconnect()

	// 2. Cache mxbuild + runtime (no-ops when already present).
	if _, err := DownloadMxBuild(version, w); err != nil {
		return nil, fmt.Errorf("setting up mxbuild: %w", err)
	}
	installPath, err := resolveRuntimeInstall(version, w)
	if err != nil {
		return nil, fmt.Errorf("setting up runtime: %w", err)
	}

	// 3. Database.
	if opts.EnsureDB {
		if err := EnsureDatabase(&opts.DB, w); err != nil {
			return nil, fmt.Errorf("ensuring database: %w", err)
		}
	} else if err := pingTCP(opts.DB.Host, 3*time.Second); err != nil {
		return nil, fmt.Errorf("database not reachable at %s: %w\n"+
			"  Pass --ensure-db to provision it, or start Postgres and create the %q database (user %q).",
			opts.DB.Host, err, opts.DB.Name, opts.DB.User)
	}

	app := &LocalApp{Version: version, RuntimeLogPath: opts.RuntimeLogPath}

	// The boot's Gradle packaging removes the browser bundle, and this app is
	// booted headless (tests), so it does not build a replacement — the loss is
	// noticed later by whoever opens a browser onto the `run --local` that is
	// serving the same deployment directory (mxcli-formula1 §35 and §62). The
	// directory is shared because mxbuild shares it, so the bundle is carried
	// across the boot instead.
	restoreWebClient := preserveWebClientBundle(opts.DeployDir)

	// 4. Build, unless the caller is reusing an existing deployment.
	if !opts.SkipBuild {
		fmt.Fprintln(w, "Building project (mxbuild --serve)...")
		serveJavaMajor, _ := ProjectJavaMajor(opts.ProjectPath)
		serve, err := StartServe(ServeOptions{Version: version, JavaMajor: serveJavaMajor, Host: "127.0.0.1", Port: opts.ServePort})
		if err != nil {
			return nil, fmt.Errorf("starting mxbuild serve: %w", err)
		}
		app.serve = serve

		build, err := serve.Build(BuildRequest{Target: TargetDeploy, ProjectFilePath: opts.ProjectPath})
		if err != nil {
			app.Stop()
			return nil, fmt.Errorf("build: %w", err)
		}
		if !build.OK() {
			app.Stop()
			return nil, &BuildFailedError{Result: build}
		}
	}

	// 5. Boot the runtime against the deployment.
	rt, err := StartLocalRuntime(opts.runtimeOptions(installPath))
	if err != nil {
		restoreWebClient()
		app.Stop()
		return nil, err
	}
	app.Runtime = rt
	restoreWebClient()
	return app, nil
}

// Rebuild rebuilds the project through the warm serve server and applies the
// result to the running runtime, returning whether that was a hot reload or a
// restart. This is the warm loop: the serve server keeps the model loaded, so a
// rebuild is ~1s instead of the ~15s cold build, and the runtime is only
// restarted when the build says the metamodel changed.
//
// A restart re-spawns the JVM from the same options, so anything passed via Env
// — notably the test runner's endpoint token — survives it.
//
// Returns an error if the app was started with SkipBuild: there is no serve
// server to rebuild through.
func (a *LocalApp) Rebuild(projectPath string) (ApplyAction, *BuildResult, error) {
	if a.serve == nil {
		return ActionReload, nil, fmt.Errorf("this app was started without a build server (SkipBuild); nothing to rebuild through")
	}
	if a.Runtime == nil {
		return ActionReload, nil, fmt.Errorf("the runtime is not running")
	}
	build, err := a.serve.Build(BuildRequest{Target: TargetDeploy, ProjectFilePath: projectPath})
	if err != nil {
		return ActionReload, nil, err
	}
	if !build.OK() {
		return ActionReload, build, fmt.Errorf("build failed: %s", build.Message)
	}
	action, err := a.Runtime.Controller().ApplyBuild(build, a.Runtime.Restart)
	return action, build, err
}

// ProjectSourceMTime is the newest modification time across a project's model
// source — the change signal a warm loop polls. Exported for callers outside
// this package that run their own watch loop (the test runner).
func ProjectSourceMTime(projectPath string) time.Time { return projectSourceMTime(projectPath) }

// Stop shuts down the runtime and the build server. Safe to call more than once
// and on a partially-started app.
func (a *LocalApp) Stop() error {
	var firstErr error
	if a.Runtime != nil {
		if err := a.Runtime.Stop(); err != nil {
			firstErr = err
		}
		a.Runtime = nil
	}
	if a.serve != nil {
		if err := a.serve.Stop(); err != nil && firstErr == nil {
			firstErr = err
		}
		a.serve = nil
	}
	return firstErr
}

// checkDeployDirIsBuildable refuses a DeployDir that this build will not write
// to.
//
// mxbuild's deploy target writes to `<app dir>/deployment` and takes no option
// to change it — measured, not inferred: `--target=deploy` on a project whose
// deployment/ had just been deleted recreated it there, `mxbuild --help` lists
// no deployment-path flag, and BuildRequest carries none. So DeployDir decides
// where the RUNTIME reads while nothing decides where the BUILD writes, and
// setting them apart points the runtime at a directory nothing populates.
//
// That is what `mxcli test --local` did after it was given a scratch tree of its
// own: the runtime aborted with `Path '…/deployment-test/model/bundles' cannot
// be resolved in base path '…/deployment-test'` for every project, whether or
// not another app was running (mxcli-ledger §150). This states the constraint
// instead, at the point the caller can still act on it.
//
// With SkipBuild there is no build to disagree with, so the caller may boot
// against any tree they have populated themselves.
func checkDeployDirIsBuildable(o LocalAppOptions) error {
	if o.SkipBuild || o.DeployDir == "" || o.ProjectPath == "" {
		return nil
	}
	buildDir := filepath.Join(filepath.Dir(o.ProjectPath), "deployment")
	if filepath.Clean(o.DeployDir) == filepath.Clean(buildDir) {
		return nil
	}
	return fmt.Errorf("cannot boot against %s: mxbuild always writes the deployment to %s "+
		"and has no option to change it, so that directory would be empty.\n"+
		"  Leave DeployDir unset to use the build's own directory, or pass SkipBuild to boot "+
		"against a tree you populated yourself.",
		o.DeployDir, buildDir)
}

// checkLocalAppPortsFree refuses to boot onto a port something is already
// serving — otherwise a stale runtime is silently adopted and the caller reads
// results from an app it did not build.
func checkLocalAppPortsFree(o LocalAppOptions) error {
	ports := []struct {
		port int
		what string
	}{
		{o.AppPort, "app"},
		{o.AdminPort, "runtime admin API"},
	}
	if !o.SkipBuild {
		ports = append(ports, struct {
			port int
			what string
		}{o.ServePort, "mxbuild serve"})
	}
	for _, p := range ports {
		if err := pingTCP(fmt.Sprintf("127.0.0.1:%d", p.port), 300*time.Millisecond); err == nil {
			return fmt.Errorf("port %d (%s) is already in use — stop the running instance first, "+
				"or pass a different port.\n%s",
				p.port, p.what, portCulpritAdvice(p.port, "127.0.0.1", o.AppPort))
		}
	}
	return nil
}
