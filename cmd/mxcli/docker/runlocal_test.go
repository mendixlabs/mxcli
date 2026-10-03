// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mendixlabs/mxcli/internal/testutil"
	"github.com/mendixlabs/mxcli/model"
)

func TestDeriveDBName(t *testing.T) {
	cases := map[string]string{
		"/path/App1112.mpr": "app1112",
		"/path/My App.mpr":  "my_app",
		"/x/Sales-CRM.mpr":  "sales_crm",
		"/x/123Numbers.mpr": "db_123numbers",
		"/x/__weird__.mpr":  "weird",
		"/x/.mpr":           "mxlocal",
	}
	for in, want := range cases {
		if got := deriveDBName(in); got != want {
			t.Errorf("deriveDBName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestLocalRunOptions_Defaults(t *testing.T) {
	o := LocalRunOptions{ProjectPath: "/proj/App1112.mpr"}
	o.applyDefaults()
	if err := o.applyDatabaseDefaults(); err != nil {
		t.Fatalf("applyDatabaseDefaults: %v", err)
	}
	if o.DeployDir != filepath.FromSlash("/proj/deployment") {
		t.Errorf("DeployDir = %q", o.DeployDir)
	}
	if o.AppPort != 8080 || o.AdminPort != 8090 || o.ServePort != 6543 {
		t.Errorf("ports = %d/%d/%d", o.AppPort, o.AdminPort, o.ServePort)
	}
	if o.AdminPass != defaultLocalAdminPass {
		t.Errorf("AdminPass = %q", o.AdminPass)
	}
	if o.DB.Type != "PostgreSQL" || o.DB.Host != "127.0.0.1:5432" || o.DB.User != "mendix" || o.DB.Password != "mendix" {
		t.Errorf("DB defaults = %+v", o.DB)
	}
	if o.DB.Name != "app1112" {
		t.Errorf("DB.Name = %q, want app1112", o.DB.Name)
	}
	if o.PollInterval != time.Second {
		t.Errorf("PollInterval = %v", o.PollInterval)
	}
}

func TestLocalRunOptions_DebugPassDefault(t *testing.T) {
	// --debug with no password defaults to "mxdebug".
	o := LocalRunOptions{ProjectPath: "/proj/App.mpr", Debug: true}
	o.applyDefaults()
	if o.DebugPass != "mxdebug" {
		t.Errorf("DebugPass = %q, want mxdebug", o.DebugPass)
	}
	// An explicit password is preserved.
	o2 := LocalRunOptions{ProjectPath: "/proj/App.mpr", Debug: true, DebugPass: "secret"}
	o2.applyDefaults()
	if o2.DebugPass != "secret" {
		t.Errorf("DebugPass override lost: %q", o2.DebugPass)
	}
	// Without --debug, no password is set.
	o3 := LocalRunOptions{ProjectPath: "/proj/App.mpr"}
	o3.applyDefaults()
	if o3.DebugPass != "" {
		t.Errorf("DebugPass should stay empty without --debug, got %q", o3.DebugPass)
	}
}

func TestBuildRuntimeSettings(t *testing.T) {
	// --metrics alone → a Prometheus registry.
	s, err := buildRuntimeSettings(true, false, nil)
	if err != nil {
		t.Fatalf("buildRuntimeSettings: %v", err)
	}
	regs, ok := s["Metrics.Registries"].([]any)
	if !ok || len(regs) != 1 {
		t.Fatalf("Metrics.Registries = %v, want one registry", s["Metrics.Registries"])
	}
	if m, _ := regs[0].(map[string]any); m["type"] != "prometheus" {
		t.Errorf("registry = %v, want type prometheus", regs[0])
	}

	// --runtime-setting with a JSON array value passes through typed, and an
	// explicit Metrics.Registries is not overridden by --metrics.
	s, err = buildRuntimeSettings(true, false, []string{
		`OpenTelemetry._RuntimeSpanFilters=["Loop","Gateway"]`,
		`Metrics.Registries=[{"type":"otlp"}]`,
	})
	if err != nil {
		t.Fatalf("buildRuntimeSettings: %v", err)
	}
	filters, ok := s["OpenTelemetry._RuntimeSpanFilters"].([]any)
	if !ok || len(filters) != 2 || filters[0] != "Loop" {
		t.Errorf("span filters = %v", s["OpenTelemetry._RuntimeSpanFilters"])
	}
	if regs, _ := s["Metrics.Registries"].([]any); len(regs) != 1 || regs[0].(map[string]any)["type"] != "otlp" {
		t.Errorf("explicit Metrics.Registries should win over --metrics, got %v", s["Metrics.Registries"])
	}

	// --trace adds the default span filters…
	s, _ = buildRuntimeSettings(false, true, nil)
	if f, _ := s["OpenTelemetry._RuntimeSpanFilters"].([]any); len(f) != len(defaultOtelSpanFilters) {
		t.Errorf("--trace should add %d default span filters, got %v", len(defaultOtelSpanFilters), s["OpenTelemetry._RuntimeSpanFilters"])
	}
	// …but an explicit filter set wins over --trace defaults.
	s, _ = buildRuntimeSettings(false, true, []string{`OpenTelemetry._RuntimeSpanFilters=["Only"]`})
	if f, _ := s["OpenTelemetry._RuntimeSpanFilters"].([]any); len(f) != 1 || f[0] != "Only" {
		t.Errorf("explicit span filters should win over --trace, got %v", s["OpenTelemetry._RuntimeSpanFilters"])
	}

	// A non-JSON value stays a plain string.
	s, _ = buildRuntimeSettings(false, false, []string{"DTAPMode=A"})
	if s["DTAPMode"] != "A" {
		t.Errorf("DTAPMode = %v, want string A", s["DTAPMode"])
	}

	// Nothing requested → nil (no overlay).
	if s, _ := buildRuntimeSettings(false, false, nil); s != nil {
		t.Errorf("want nil for no settings, got %v", s)
	}

	// Malformed setting errors.
	if _, err := buildRuntimeSettings(false, false, []string{"noequals"}); err == nil {
		t.Error("want error for a setting without '='")
	}
}

func TestWithTraceEnv(t *testing.T) {
	base := []string{"PATH=/bin", "JAVA_TOOL_OPTIONS=-Xmx512m"}
	env := withTraceEnv(base, "/agents/otel.jar", "sudoku", "")

	get := func(key string) (string, bool) {
		for _, e := range env {
			if strings.HasPrefix(e, key+"=") {
				return e[len(key)+1:], true
			}
		}
		return "", false
	}
	// Agent appended to the existing JAVA_TOOL_OPTIONS (not duplicated).
	jto, _ := get("JAVA_TOOL_OPTIONS")
	if !strings.Contains(jto, "-Xmx512m") || !strings.Contains(jto, "-javaagent:/agents/otel.jar") {
		t.Errorf("JAVA_TOOL_OPTIONS = %q, want existing + agent", jto)
	}
	n := 0
	for _, e := range env {
		if strings.HasPrefix(e, "JAVA_TOOL_OPTIONS=") {
			n++
		}
	}
	if n != 1 {
		t.Errorf("JAVA_TOOL_OPTIONS appears %d times, want 1", n)
	}
	if v, _ := get("OTEL_SERVICE_NAME"); v != "sudoku" {
		t.Errorf("OTEL_SERVICE_NAME = %q, want sudoku", v)
	}
	if v, _ := get("OTEL_TRACES_EXPORTER"); v != "console" {
		t.Errorf("OTEL_TRACES_EXPORTER = %q, want console", v)
	}

	// A user-provided OTEL_* is respected (not overridden).
	env = withTraceEnv([]string{"OTEL_TRACES_EXPORTER=otlp"}, "/agents/otel.jar", "svc", "")
	if v, _ := get2(env, "OTEL_TRACES_EXPORTER"); v != "otlp" {
		t.Errorf("user OTEL_TRACES_EXPORTER should win, got %q", v)
	}

	// An OTLP endpoint switches the traces exporter to otlp and wires the
	// protocol + endpoint (for flame charts). (sudoku tracing follow-up)
	env = withTraceEnv([]string{"PATH=/bin"}, "/agents/otel.jar", "svc", "http://127.0.0.1:4318")
	if v, _ := get2(env, "OTEL_TRACES_EXPORTER"); v != "otlp" {
		t.Errorf("OTEL_TRACES_EXPORTER = %q, want otlp", v)
	}
	if v, _ := get2(env, "OTEL_EXPORTER_OTLP_PROTOCOL"); v != "http/protobuf" {
		t.Errorf("OTEL_EXPORTER_OTLP_PROTOCOL = %q, want http/protobuf", v)
	}
	if v, _ := get2(env, "OTEL_EXPORTER_OTLP_ENDPOINT"); v != "http://127.0.0.1:4318" {
		t.Errorf("OTEL_EXPORTER_OTLP_ENDPOINT = %q, want the endpoint", v)
	}

	// A user-set exporter still wins even when an OTLP endpoint is passed.
	env = withTraceEnv([]string{"OTEL_TRACES_EXPORTER=console"}, "/agents/otel.jar", "svc", "http://127.0.0.1:4318")
	if v, _ := get2(env, "OTEL_TRACES_EXPORTER"); v != "console" {
		t.Errorf("user OTEL_TRACES_EXPORTER should win over --trace-otlp, got %q", v)
	}
	if _, ok := get2(env, "OTEL_EXPORTER_OTLP_ENDPOINT"); ok {
		t.Error("endpoint should not be set when the user pinned a non-otlp exporter")
	}
}

func get2(env []string, key string) (string, bool) {
	for _, e := range env {
		if strings.HasPrefix(e, key+"=") {
			return e[len(key)+1:], true
		}
	}
	return "", false
}

func TestRuntimeConfigParams_OverlaysSettings(t *testing.T) {
	o := LocalRuntimeOptions{
		DeployDir: "/d", DB: DBConfig{Type: "PostgreSQL", Name: "app"},
		RuntimeSettings: map[string]any{"Metrics.Registries": []any{map[string]any{"type": "prometheus"}}},
	}
	p := runtimeConfigParams(o, nil)
	// Base keys still present…
	if p["DatabaseName"] != "app" {
		t.Errorf("DatabaseName lost: %v", p["DatabaseName"])
	}
	// …and the overlay merged in (not a separate replace call).
	if _, ok := p["Metrics.Registries"]; !ok {
		t.Errorf("Metrics.Registries not merged into config params: %v", p)
	}
}

func TestLocalRunOptions_DefaultsRespectOverrides(t *testing.T) {
	o := LocalRunOptions{
		ProjectPath: "/proj/App.mpr",
		AppPort:     9000,
		DB:          DBConfig{Host: "db:5432", Name: "custom", User: "u", Password: "p"},
	}
	o.applyDefaults()
	if err := o.applyDatabaseDefaults(); err != nil {
		t.Fatalf("applyDatabaseDefaults: %v", err)
	}
	if o.AppPort != 9000 {
		t.Errorf("AppPort override lost: %d", o.AppPort)
	}
	if o.DB.Host != "db:5432" || o.DB.Name != "custom" || o.DB.User != "u" || o.DB.Password != "p" {
		t.Errorf("DB overrides lost: %+v", o.DB)
	}
}

func TestEnsureMxBuildRuntimeSibling(t *testing.T) {
	// Point the cache roots at a temp HOME so we don't touch the real cache.
	home := t.TempDir()
	testutil.SetHome(t, home)

	version := "99.99.99"
	// Build the runtime cache with a runtime/ dir.
	runtimeCache, _ := RuntimeCacheDir(version)
	realRuntime := filepath.Join(runtimeCache, "runtime")
	if err := os.MkdirAll(realRuntime, 0o755); err != nil {
		t.Fatal(err)
	}
	// mxbuild cache exists (modeler/) but has no runtime/ sibling yet.
	mxbuildCache, _ := MxBuildCacheDir(version)
	if err := os.MkdirAll(filepath.Join(mxbuildCache, "modeler"), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := ensureMxBuildRuntimeSibling(version, io.Discard); err != nil {
		t.Fatalf("ensureMxBuildRuntimeSibling: %v", err)
	}
	link := filepath.Join(mxbuildCache, "runtime")
	if _, err := os.Stat(link); err != nil {
		t.Errorf("runtime sibling not created: %v", err)
	}
	// Idempotent second call.
	if err := ensureMxBuildRuntimeSibling(version, io.Discard); err != nil {
		t.Errorf("second call should be a no-op, got %v", err)
	}
}

func TestEnsureMxBuildRuntimeSibling_MissingSource(t *testing.T) {
	home := t.TempDir()
	testutil.SetHome(t, home)
	version := "99.99.98"
	mxbuildCache, _ := MxBuildCacheDir(version)
	_ = os.MkdirAll(filepath.Join(mxbuildCache, "modeler"), 0o755)
	// No runtime cache -> error.
	if err := ensureMxBuildRuntimeSibling(version, io.Discard); err == nil {
		t.Error("expected error when the runtime source is absent")
	}
}

func TestProjectSourceMTime(t *testing.T) {
	dir := t.TempDir()
	// Build-output/cache dirs the serve/mxbuild build churns — must be ignored.
	for _, d := range []string{"deployment", "theme-cache", ".mendix-cache", ".mxcli"} {
		_ = os.MkdirAll(filepath.Join(dir, d), 0o755)
	}
	mprcontents := filepath.Join(dir, "mprcontents", "ab")
	_ = os.MkdirAll(mprcontents, 0o755)

	mpr := filepath.Join(dir, "App.mpr")
	if err := os.WriteFile(mpr, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	doc := filepath.Join(mprcontents, "doc.mxunit")
	_ = os.WriteFile(doc, []byte("d"), 0o644)

	base := projectSourceMTime(mpr)
	if base.IsZero() {
		t.Fatal("expected a non-zero source mtime")
	}

	// Churn in every build-output/cache dir must NOT advance the signal.
	future := time.Now().Add(time.Hour)
	for _, f := range []string{
		filepath.Join(dir, "deployment", "model.mdp"),
		filepath.Join(dir, "theme-cache", "theme.compiled.css"),
		filepath.Join(dir, ".mendix-cache", "x"),
		filepath.Join(dir, ".mxcli", "run-local.png"),
	} {
		_ = os.WriteFile(f, []byte("y"), 0o644)
		_ = os.Chtimes(f, future, future)
	}
	if projectSourceMTime(mpr).After(base) {
		t.Error("build-output/cache churn must not advance the source signal")
	}

	// An edit to the .mpr MUST advance the signal.
	_ = os.Chtimes(mpr, future, future)
	if !projectSourceMTime(mpr).After(base) {
		t.Error(".mpr change should advance the signal")
	}

	// An edit under mprcontents/ (v2 documents) MUST advance the signal.
	base2 := projectSourceMTime(mpr)
	future2 := time.Now().Add(2 * time.Hour)
	_ = os.Chtimes(doc, future2, future2)
	if !projectSourceMTime(mpr).After(base2) {
		t.Error("mprcontents/ change should advance the signal")
	}
}

func TestWebClientSourceMTime_ExcludesDist(t *testing.T) {
	dep := t.TempDir()
	web := filepath.Join(dep, "web")
	dist := filepath.Join(web, "dist")
	_ = os.MkdirAll(filepath.Join(web, "pages"), 0o755)
	_ = os.MkdirAll(dist, 0o755)

	src := filepath.Join(web, "pages", "Home.js")
	_ = os.WriteFile(src, []byte("x"), 0o644)
	base := webClientSourceMTime(dep)
	if base.IsZero() {
		t.Fatal("expected a non-zero web source mtime")
	}

	// A newer file under dist/ must NOT advance the source mtime.
	future := time.Now().Add(time.Hour)
	df := filepath.Join(dist, "index.js")
	_ = os.WriteFile(df, []byte("y"), 0o644)
	_ = os.Chtimes(df, future, future)
	if webClientSourceMTime(dep).After(base) {
		t.Error("dist/ changes must be excluded from the web source mtime")
	}

	// A newer source file MUST advance it.
	_ = os.Chtimes(src, future, future)
	if !webClientSourceMTime(dep).After(base) {
		t.Error("a client source change should advance the web source mtime")
	}
}

func TestResolveScreenshotURL(t *testing.T) {
	app := "http://127.0.0.1:8080/"
	cases := map[string]string{
		"":                      app,
		"/p/customers":          "http://127.0.0.1:8080/p/customers",
		"p/customers":           "http://127.0.0.1:8080/p/customers",
		"http://host:9000/x":    "http://host:9000/x",
		"https://example.com/a": "https://example.com/a",
	}
	for in, want := range cases {
		if got := resolveScreenshotURL(app, in); got != want {
			t.Errorf("resolveScreenshotURL(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSlugifyPage(t *testing.T) {
	cases := map[string]string{
		"":                       "home",
		"/":                      "home",
		"/p/customers":           "p-customers",
		"p/Customer_Overview":    "p-customer-overview",
		"http://127.0.0.1:8080/": "home",
		"http://h:8080/p/orders": "p-orders",
		"/p/a//b":                "p-a-b",
	}
	for in, want := range cases {
		if got := slugifyPage(in); got != want {
			t.Errorf("slugifyPage(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestScreenshotOutName(t *testing.T) {
	base := filepath.FromSlash("/x/.mxcli/run-local.png")
	cases := map[string]string{
		"/p/customers": filepath.FromSlash("/x/.mxcli/run-local-p-customers.png"),
		"":             filepath.FromSlash("/x/.mxcli/run-local-home.png"),
	}
	for in, want := range cases {
		if got := screenshotOutName(base, in); got != want {
			t.Errorf("screenshotOutName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestPageLabel(t *testing.T) {
	if pageLabel("") != "home" {
		t.Errorf("pageLabel(\"\") = %q, want home", pageLabel(""))
	}
	if pageLabel("/p/x") != "/p/x" {
		t.Errorf("pageLabel(/p/x) = %q", pageLabel("/p/x"))
	}
}

func TestThemeSourceMTime_WatchesThemeAndThemesource(t *testing.T) {
	dir := t.TempDir()
	mpr := filepath.Join(dir, "App.mpr")
	if err := os.WriteFile(mpr, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	// App-level theme and a per-module theme source.
	themeMain := filepath.Join(dir, "theme", "web", "main.scss")
	moduleScss := filepath.Join(dir, "themesource", "travel", "web", "main.scss")
	for _, f := range []string{themeMain, moduleScss} {
		_ = os.MkdirAll(filepath.Dir(f), 0o755)
		if err := os.WriteFile(f, []byte("// scss"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	base := themeSourceMTime(mpr)
	if base.IsZero() {
		t.Fatal("expected a non-zero theme source mtime")
	}

	// Editing the app-level main.scss MUST advance the signal.
	future := time.Now().Add(time.Hour)
	_ = os.Chtimes(themeMain, future, future)
	if !themeSourceMTime(mpr).After(base) {
		t.Error("theme/web/main.scss change should advance the theme signal")
	}

	// Editing a per-module theme source MUST advance the signal too.
	base2 := themeSourceMTime(mpr)
	future2 := time.Now().Add(2 * time.Hour)
	_ = os.Chtimes(moduleScss, future2, future2)
	if !themeSourceMTime(mpr).After(base2) {
		t.Error("themesource/<module>/web/main.scss change should advance the theme signal")
	}

	// sourceMTime combines model + theme: a theme edit advances the combined signal
	// even when the model is untouched (the core of the Problem-3 fix).
	combinedBase := sourceMTime(mpr)
	future3 := time.Now().Add(3 * time.Hour)
	_ = os.Chtimes(themeMain, future3, future3)
	if !sourceMTime(mpr).After(combinedBase) {
		t.Error("a theme-only edit should advance the combined watch signal")
	}
}

func TestCheckTargetPortsFree(t *testing.T) {
	// A run whose ports are all free must pass.
	opts := LocalRunOptions{}
	opts.applyDefaults()
	// Move ports to almost-certainly-free high values so a real serve/8080 on the
	// box doesn't make the test flaky.
	opts.AppPort, opts.AdminPort, opts.ServePort = 5, 6, 7 // privileged/unused; nothing listens
	if err := checkTargetPortsFree(opts); err != nil {
		t.Errorf("expected free ports to pass, got: %v", err)
	}

	// Occupy a port, point AppPort at it, and expect a refusal that names it.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	busy := ln.Addr().(*net.TCPAddr).Port
	opts.AppPort = busy
	err = checkTargetPortsFree(opts)
	if err == nil {
		t.Fatalf("expected refusal when port %d is occupied", busy)
	}
	if want := fmt.Sprintf("port %d", busy); !strings.Contains(err.Error(), want) {
		t.Errorf("error should name the busy port %q; got: %v", want, err)
	}
	if !strings.Contains(err.Error(), "already in use") {
		t.Errorf("error should explain the port is in use; got: %v", err)
	}
}

// The app's host name belongs in the project's configuration (App Settings ->
// Configurations), not on the command line. These pin the selection rule.
func TestApplicationRootURLFrom(t *testing.T) {
	cfg := func(name, url string) *model.ServerConfiguration {
		return &model.ServerConfiguration{Name: name, ApplicationRootUrl: url}
	}
	settings := func(cfgs ...*model.ServerConfiguration) *model.ProjectSettings {
		return &model.ProjectSettings{Configuration: &model.ConfigurationSettings{Configurations: cfgs}}
	}

	tests := []struct {
		name       string
		in         *model.ProjectSettings
		wantURL    string
		wantConfig string
	}{
		{"no settings at all", nil, "", ""},
		{"no configuration part", &model.ProjectSettings{}, "", ""},
		{"no configurations", settings(), "", ""},
		{"configuration without a URL", settings(cfg("Default", "")), "", ""},
		{
			"single configuration",
			settings(cfg("Default", "http://backend.local:8080/")),
			"http://backend.local:8080/", "Default",
		},
		{
			// No "active configuration" marker exists in the model, so Default wins
			// wherever it sits in the list.
			"Default wins over others",
			settings(cfg("Acceptance", "https://acc.example.com/"), cfg("Default", "http://backend.local:8080/")),
			"http://backend.local:8080/", "Default",
		},
		{"Default match is case-insensitive", settings(cfg("default", "http://x/")), "http://x/", "default"},
		{
			"first one that sets a URL when there is no Default",
			settings(cfg("Local", ""), cfg("Test", "https://test.example.com/"), cfg("Acc", "https://acc.example.com/")),
			"https://test.example.com/", "Test",
		},
		{"nil entries are skipped", settings(nil, cfg("Default", "http://y/")), "http://y/", "Default"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			url, name := applicationRootURLFrom(tt.in)
			if url != tt.wantURL || name != tt.wantConfig {
				t.Errorf("got (%q, %q), want (%q, %q)", url, name, tt.wantURL, tt.wantConfig)
			}
		})
	}
}

// A blank Mendix app already sets ApplicationRootUrl to http://localhost:8080/,
// so honouring every configured value would change behaviour for every existing
// project — and name the wrong port under --app-port. Only a real host name is
// worth passing to the runtime.
func TestCustomHostRootURL(t *testing.T) {
	tests := map[string]bool{
		"":                                  false,
		"http://localhost:8080/":            false, // the stock value in a blank app
		"http://LOCALHOST:8080/":            false,
		"http://127.0.0.1:8080/":            false,
		"http://127.0.0.2:8080/":            false,
		"http://[::1]:8080/":                false,
		"not a url":                         false,
		"http://backend.local:8080/":        true,
		"https://app.example.com/":          true,
		"http://app.127.0.0.1.nip.io:8080/": true, // a name, even if it resolves to loopback
	}
	for in, want := range tests {
		if got := customHostRootURL(in); got != want {
			t.Errorf("customHostRootURL(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestURLPort(t *testing.T) {
	tests := map[string]string{
		"http://backend.local:8080/": "8080",
		"https://app.example.com/":   "",
		"":                           "",
	}
	for in, want := range tests {
		if got := urlPort(in); got != want {
			t.Errorf("urlPort(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestLocalRunOptions_DatabaseDefaults_HSQLDB(t *testing.T) {
	o := LocalRunOptions{ProjectPath: "/proj/App1112.mpr", DB: DBConfig{Type: "hsqldb"}}
	o.applyDefaults()
	if err := o.applyDatabaseDefaults(); err != nil {
		t.Fatalf("applyDatabaseDefaults: %v", err)
	}
	if o.DB.Type != "HSQLDB" {
		t.Errorf("Type = %q, want HSQLDB", o.DB.Type)
	}
	if o.DB.Host != "" || o.DB.User != "" || o.DB.Password != "" {
		t.Errorf("HSQLDB must carry no host/credentials, got %+v", o.DB)
	}
	if o.DB.Name != "app1112" {
		t.Errorf("DB.Name = %q, want app1112", o.DB.Name)
	}
}

func TestLocalRunOptions_DatabaseDefaults_HSQLDBRejectsConnectionFlags(t *testing.T) {
	for _, db := range []DBConfig{
		{Type: "hsqldb", Host: "127.0.0.1:5432"},
		{Type: "hsqldb", User: "u"},
		{Type: "hsqldb", Password: "p"},
	} {
		o := LocalRunOptions{ProjectPath: "/proj/App.mpr", DB: db}
		o.applyDefaults()
		if err := o.applyDatabaseDefaults(); err == nil {
			t.Errorf("hsqldb with %+v: want an error", db)
		}
	}
}

func TestLocalRunOptions_DatabaseDefaults_HSQLDBRejectsEnsureDB(t *testing.T) {
	o := LocalRunOptions{ProjectPath: "/proj/App.mpr", DB: DBConfig{Type: "hsqldb"}, EnsureDB: true}
	o.applyDefaults()
	if err := o.applyDatabaseDefaults(); err == nil {
		t.Error("hsqldb + --ensure-db: want an error")
	}
}

func TestLocalRunOptions_DatabaseDefaults_UnknownType(t *testing.T) {
	o := LocalRunOptions{ProjectPath: "/proj/App.mpr", DB: DBConfig{Type: "mysql"}}
	o.applyDefaults()
	if err := o.applyDatabaseDefaults(); err == nil {
		t.Error("unknown db type: want an error")
	}
}

func TestDBConfig_IsFileBased(t *testing.T) {
	if !(DBConfig{Type: "HSQLDB"}).IsFileBased() {
		t.Error("HSQLDB should be file-based")
	}
	if (DBConfig{Type: "PostgreSQL"}).IsFileBased() {
		t.Error("PostgreSQL is not file-based")
	}
	if (DBConfig{Type: "hsqldb"}).IsFileBased() {
		t.Error("the check must use the runtime spelling set by applyDatabaseDefaults, not the raw flag")
	}
}
