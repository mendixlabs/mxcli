// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func testLocalOpts() LocalRuntimeOptions {
	o := LocalRuntimeOptions{
		DeployDir:   "/app/deployment",
		InstallPath: "/cache/11.12.1",
		JavaHome:    "/jdk21",
		AdminPass:   "secret",
		DB: DBConfig{
			Type: "PostgreSQL", Host: "127.0.0.1:5432",
			Name: "appdb", User: "mendix", Password: "mendix",
		},
	}
	o.applyDefaults()
	return o
}

func TestApplyDefaults(t *testing.T) {
	var o LocalRuntimeOptions
	o.applyDefaults()
	if o.AdminPort != 8090 || o.AppPort != 8080 {
		t.Errorf("ports = %d/%d, want 8090/8080", o.AdminPort, o.AppPort)
	}
	if o.ListenAddr != "127.0.0.1" {
		t.Errorf("ListenAddr = %q", o.ListenAddr)
	}
	if o.DTAPMode != "D" {
		t.Errorf("DTAPMode = %q, want D", o.DTAPMode)
	}
	if o.ReadyTimeout != 90*time.Second {
		t.Errorf("ReadyTimeout = %v", o.ReadyTimeout)
	}
	if o.Stdout == nil || o.Stderr == nil {
		t.Error("Stdout/Stderr should default to non-nil")
	}
}

func TestPathHelpers(t *testing.T) {
	o := testLocalOpts()
	if got := o.runtimeDir(); got != filepath.FromSlash("/cache/11.12.1/runtime") {
		t.Errorf("runtimeDir = %q", got)
	}
	if got := o.launcherJar(); got != filepath.FromSlash("/cache/11.12.1/runtime/launcher/runtimelauncher.jar") {
		t.Errorf("launcherJar = %q", got)
	}
}

func TestJVMArgs(t *testing.T) {
	o := testLocalOpts()
	args := o.jvmArgs()
	joined := strings.Join(args, " ")
	// The live-preview dev flags must be present so `mxcli oql` can reach a
	// `run --local` app via /dev/preview_execute_oql (findings #36).
	for _, want := range []string{
		"-Dmendix.live-preview=enabled",
		"-Dmendix.running.locally.by.studiopro=true",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("jvmArgs missing %q; got %v", want, args)
		}
	}
	// -jar and its two positional args (launcher jar + deploy dir) must still be
	// last, in order, so the JVM launches the runtime.
	if len(args) < 3 || args[len(args)-3] != "-jar" {
		t.Fatalf("jvmArgs must end with -jar <launcher> <deploydir>; got %v", args)
	}
	if args[len(args)-2] != o.launcherJar() {
		t.Errorf("launcher jar = %q, want %q", args[len(args)-2], o.launcherJar())
	}
	if args[len(args)-1] != o.DeployDir {
		t.Errorf("deploy dir = %q, want %q", args[len(args)-1], o.DeployDir)
	}
}

func TestLocalRuntimeEnv(t *testing.T) {
	o := testLocalOpts()
	env := localRuntimeEnv(o)
	want := map[string]bool{
		"M2EE_ADMIN_PASS=secret":                false,
		"M2EE_ADMIN_PORT=8090":                  false,
		"M2EE_ADMIN_LISTEN_ADDRESSES=127.0.0.1": false,
		"MX_INSTALL_PATH=/cache/11.12.1":        false,
		"MX_LOG_LEVEL=i":                        false,
	}
	for _, e := range env {
		if _, ok := want[e]; ok {
			want[e] = true
		}
	}
	for k, seen := range want {
		if !seen {
			t.Errorf("env missing %q", k)
		}
	}
}

func TestAppContainerParams(t *testing.T) {
	p := appContainerParams(testLocalOpts())
	if p["runtime_port"] != 8080 {
		t.Errorf("runtime_port = %v, want 8080", p["runtime_port"])
	}
	if p["runtime_listen_addresses"] != "127.0.0.1" {
		t.Errorf("runtime_listen_addresses = %v", p["runtime_listen_addresses"])
	}
}

func TestRuntimeConfigParams(t *testing.T) {
	o := testLocalOpts()
	consts := map[string]string{"Mod.Key": "val"}
	p := runtimeConfigParams(o, consts)
	checks := map[string]any{
		"BasePath":         "/app/deployment",
		"RuntimePath":      filepath.FromSlash("/cache/11.12.1/runtime"),
		"DTAPMode":         "D",
		"DatabaseType":     "PostgreSQL",
		"DatabaseHost":     "127.0.0.1:5432",
		"DatabaseName":     "appdb",
		"DatabaseUserName": "mendix",
		"DatabasePassword": "mendix",
	}
	for k, want := range checks {
		if p[k] != want {
			t.Errorf("%s = %v, want %v", k, p[k], want)
		}
	}
	mc, ok := p["MicroflowConstants"].(map[string]string)
	if !ok || mc["Mod.Key"] != "val" {
		t.Errorf("MicroflowConstants = %v", p["MicroflowConstants"])
	}
}

func TestRuntimeConfigParams_NilConstants(t *testing.T) {
	p := runtimeConfigParams(testLocalOpts(), nil)
	mc, ok := p["MicroflowConstants"].(map[string]string)
	if !ok || len(mc) != 0 {
		t.Errorf("MicroflowConstants should be an empty (non-nil) map, got %v", p["MicroflowConstants"])
	}
}

func TestPrivateDBPasswordRuntimeConfig(t *testing.T) {
	const password = " test-only-private-db-password "
	for _, supplied := range []string{"", password} {
		run := LocalRunOptions{ProjectPath: "App.mpr", DB: DBConfig{Password: supplied}}
		if err := run.applyDatabaseDefaults(); err != nil {
			t.Fatal("could not apply database defaults")
		}
		want := supplied
		if want == "" {
			want = "mendix"
		}
		runtime := testLocalOpts()
		runtime.DB = run.DB
		if runtimeConfigParams(runtime, nil)["DatabasePassword"] != want {
			t.Fatal("password did not reach runtime configuration")
		}
		if strings.Contains(strings.Join(runtime.jvmArgs(), " "), password) ||
			strings.Contains(strings.Join(localRuntimeEnv(runtime), " "), password) {
			t.Fatal("database password appeared in runtime argv or environment")
		}
	}
}

func TestRuntimeConfigParams_ApplicationRootUrl(t *testing.T) {
	// Absent by default (plain local run): the runtime derives it from the listen
	// address, so we must not pin it.
	if _, ok := runtimeConfigParams(testLocalOpts(), nil)["ApplicationRootUrl"]; ok {
		t.Error("ApplicationRootUrl must be absent when not set")
	}
	// Present when serving behind a hub, so the SPA works under the public origin.
	o := testLocalOpts()
	o.ApplicationRootUrl = "https://hub.example.com"
	if got := runtimeConfigParams(o, nil)["ApplicationRootUrl"]; got != "https://hub.example.com" {
		t.Errorf("ApplicationRootUrl = %v, want https://hub.example.com", got)
	}
}

func TestRuntimeConfigParams_HSQLDB(t *testing.T) {
	o := testLocalOpts()
	o.DB = DBConfig{Type: "HSQLDB", Name: "appdb"}
	p := runtimeConfigParams(o, nil)
	checks := map[string]any{
		"DatabaseType":     "HSQLDB",
		"DatabaseHost":     "",
		"DatabaseName":     "appdb",
		"DatabaseUserName": "",
		"DatabasePassword": "",
	}
	for k, want := range checks {
		if p[k] != want {
			t.Errorf("%s = %v, want %v", k, p[k], want)
		}
	}
}

func TestReadDeploymentConstants(t *testing.T) {
	dir := t.TempDir()
	modelDir := filepath.Join(dir, "model")
	if err := os.MkdirAll(modelDir, 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := `{"Configuration":{"DatabaseType":"HSQLDB"},"Constants":{"A.X":"1","B.Y":"two"},"AdminPassword":"1"}`
	if err := os.WriteFile(filepath.Join(modelDir, "config.json"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := readDeploymentConstants(dir)
	if err != nil {
		t.Fatalf("readDeploymentConstants: %v", err)
	}
	if got["A.X"] != "1" || got["B.Y"] != "two" || len(got) != 2 {
		t.Errorf("constants = %v", got)
	}
}

func TestReadDeploymentConstants_Missing(t *testing.T) {
	// No config.json -> empty map, no error (an app may legitimately have none).
	got, err := readDeploymentConstants(t.TempDir())
	if err != nil {
		t.Fatalf("expected no error for a missing config.json, got %v", err)
	}
	if len(got) != 0 {
		t.Errorf("constants = %v, want empty", got)
	}
}

func TestReadDeploymentConstants_BadJSON(t *testing.T) {
	dir := t.TempDir()
	modelDir := filepath.Join(dir, "model")
	_ = os.MkdirAll(modelDir, 0o755)
	_ = os.WriteFile(filepath.Join(modelDir, "config.json"), []byte("{not json"), 0o644)
	if _, err := readDeploymentConstants(dir); err == nil {
		t.Error("expected an error for malformed config.json")
	}
}

func TestEnsureDataDirs(t *testing.T) {
	dir := t.TempDir()
	if err := ensureDataDirs(dir); err != nil {
		t.Fatalf("ensureDataDirs: %v", err)
	}
	for _, sub := range []string{"files", "tmp", "model-upload"} {
		p := filepath.Join(dir, "data", sub)
		if fi, err := os.Stat(p); err != nil || !fi.IsDir() {
			t.Errorf("missing data dir %s", p)
		}
	}
}

func TestStartLocalRuntime_Validation(t *testing.T) {
	// Missing AdminPass.
	if _, err := StartLocalRuntime(LocalRuntimeOptions{InstallPath: "/x"}); err == nil {
		t.Error("expected error for missing AdminPass")
	}
	// Missing InstallPath.
	if _, err := StartLocalRuntime(LocalRuntimeOptions{AdminPass: "p"}); err == nil {
		t.Error("expected error for missing InstallPath")
	}
	// Launcher jar not present.
	if _, err := StartLocalRuntime(LocalRuntimeOptions{AdminPass: "p", InstallPath: t.TempDir()}); err == nil {
		t.Error("expected error when the launcher jar is absent")
	}
}

// TestOpenRuntimeLog verifies the runtime-log tee creates the parent dir, appends
// across restarts (reusing the file, closing the prior handle), and writes a
// start marker each spawn. Findings #25.
func TestOpenRuntimeLog(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "nested", ".mxcli", "runtime.log")
	rt := &LocalRuntime{opts: LocalRuntimeOptions{RuntimeLogPath: logPath}}

	// First spawn: opens, writes marker, then some "runtime" output.
	f1, err := rt.openRuntimeLog()
	if err != nil {
		t.Fatalf("openRuntimeLog: %v", err)
	}
	rt.logFile = f1
	if _, err := f1.WriteString("first-run line\n"); err != nil {
		t.Fatalf("write: %v", err)
	}

	// Second spawn (a restart): must reuse the same file (append) and close f1.
	f2, err := rt.openRuntimeLog()
	if err != nil {
		t.Fatalf("openRuntimeLog restart: %v", err)
	}
	rt.logFile = f2
	if _, err := f2.WriteString("second-run line\n"); err != nil {
		t.Fatalf("write 2: %v", err)
	}
	_ = f2.Close()

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	s := string(data)
	for _, want := range []string{"first-run line", "second-run line", "=== runtime start"} {
		if !strings.Contains(s, want) {
			t.Errorf("runtime log missing %q; got:\n%s", want, s)
		}
	}
	// Two start markers (one per spawn) → append, not truncate.
	if n := strings.Count(s, "=== runtime start"); n != 2 {
		t.Errorf("expected 2 start markers (append across restarts), got %d", n)
	}
	// Writing to the first (now-closed) handle must fail — it was closed on reopen.
	if _, err := f1.WriteString("x"); err == nil {
		t.Error("expected write to the closed prior handle to fail")
	}
}
