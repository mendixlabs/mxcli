// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"os"
	"path/filepath"
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
