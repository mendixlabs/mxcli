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

// DBConfig is the external Postgres the standalone runtime connects to.
type DBConfig struct {
	Type     string // e.g. "PostgreSQL"
	Host     string // "host:port", e.g. "127.0.0.1:5432"
	Name     string
	User     string
	Password string
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
	// AdminPort is the M2EE admin API port (default 8090).
	AdminPort int
	// AppPort is the HTTP port the app serves on (default 8080).
	AppPort int
	// AdminPass is the M2EE admin password (required).
	AdminPass string
	// ListenAddr binds both the admin API and the app (default 127.0.0.1).
	ListenAddr string
	// DTAPMode is D/A/T/P (default "D").
	DTAPMode string
	// DB is the database the runtime connects to.
	DB DBConfig
	// ReadyTimeout bounds how long StartLocalRuntime waits for the admin API
	// (default 90s).
	ReadyTimeout time.Duration
	// Stdout/Stderr receive progress messages (default os.Stdout/os.Stderr).
	Stdout io.Writer
	Stderr io.Writer
}

// LocalRuntime is a booted standalone runtime process plus its admin connection.
type LocalRuntime struct {
	opts LocalRuntimeOptions
	cmd  *exec.Cmd
	log  *syncBuffer
	m2ee M2EEOptions
	ctrl *RuntimeController
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

// localRuntimeEnv builds the environment for the runtime JVM, layered on the
// current process environment. PrepareMxCommand later adds the FreeType fix.
func localRuntimeEnv(o LocalRuntimeOptions) []string {
	return append(os.Environ(),
		"M2EE_ADMIN_PASS="+o.AdminPass,
		fmt.Sprintf("M2EE_ADMIN_PORT=%d", o.AdminPort),
		"M2EE_ADMIN_LISTEN_ADDRESSES="+o.ListenAddr,
		"MX_INSTALL_PATH="+o.InstallPath,
		"MX_LOG_LEVEL=i",
	)
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
	return map[string]any{
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
		// Mendix 11 needs JDK 21. Version-aware selection (9/10) is a follow-up;
		// the local loop targets 11.x for now.
		jh, err := resolveJDK21()
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
			Host:    opts.ListenAddr,
			Port:    opts.AdminPort,
			Token:   opts.AdminPass,
			Direct:  true,
			Timeout: 150 * time.Second,
		},
	}
	rt.ctrl = NewRuntimeController(rt.m2ee)

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
	javaExe := filepath.Join(rt.opts.JavaHome, "bin", "java")
	cmd := exec.Command(javaExe, "-jar", rt.opts.launcherJar(), rt.opts.DeployDir)
	cmd.Dir = rt.opts.runtimeDir()
	cmd.Env = localRuntimeEnv(rt.opts)
	PrepareMxCommand(cmd) // FreeType LD_PRELOAD workaround, layered on cmd.Env
	log := &syncBuffer{}
	cmd.Stdout = log
	cmd.Stderr = log
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("launching runtime JVM: %w", err)
	}
	rt.cmd = cmd
	rt.log = log

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
	if _, err := CallM2EE(rt.m2ee, "update_configuration", runtimeConfigParams(rt.opts, constants)); err != nil {
		return fmt.Errorf("update_configuration: %w", err)
	}
	return nil
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

func (rt *LocalRuntime) alive() bool {
	if rt.cmd == nil || rt.cmd.Process == nil {
		return false
	}
	return rt.cmd.Process.Signal(syscall.Signal(0)) == nil
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
	_ = rt.cmd.Process.Signal(syscall.SIGTERM)
	done := make(chan error, 1)
	go func() { done <- rt.cmd.Wait() }()
	select {
	case <-done:
	case <-time.After(8 * time.Second):
		_ = rt.cmd.Process.Kill()
		<-done
	}
	rt.cmd = nil
	return nil
}
