// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"
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

func TestLocalRunOptions_DefaultsRespectOverrides(t *testing.T) {
	o := LocalRunOptions{
		ProjectPath: "/proj/App.mpr",
		AppPort:     9000,
		DB:          DBConfig{Host: "db:5432", Name: "custom", User: "u", Password: "p"},
	}
	o.applyDefaults()
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
	t.Setenv("HOME", home)

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
	t.Setenv("HOME", home)
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
