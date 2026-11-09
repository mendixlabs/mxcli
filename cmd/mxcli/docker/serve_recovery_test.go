// SPDX-License-Identifier: Apache-2.0

// `run --local --watch` on Mendix 11.12.5: the first hot reload after any change
// leaves deployment/web/dist holding only index.js and chunks/, so every page
// 404s until the loop is restarted (mendixlabs/mxcli#1580).
//
// The defect is in mxbuild --serve, and the measurements that place it there
// were taken by driving its /build API directly, no watch loop involved, on a
// blank 11.12.5 app:
//
//	build                          restartRequired   dist after it
//	1  cold                        true              13 pages, widgets.css, 5.6 KB index.js
//	2  same process, NO change     false             0 pages, no widgets.css, 683 KB index.js
//	3  same process, rm web/dist   false             same as 2
//	4  fresh process               true              complete again
//
// So no in-process step recovers it; a fresh serve process does, and only its
// restartRequired is untrustworthy (true even for an unchanged model).
package docker

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// prebuiltDeployment writes an 11.12.5/11.14-shaped deployment: mxbuild wrote
// web/dist itself, and there is no rollup config and no web/pages source.
func prebuiltDeployment(t *testing.T) string {
	t.Helper()
	deploy := t.TempDir()
	writeCompleteBundle(t, deploy)
	return deploy
}

func writeCompleteBundle(t *testing.T, deploy string) {
	t.Helper()
	dist := filepath.Join(deploy, "web", "dist")
	if err := os.RemoveAll(dist); err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{"index.js", "widgets.css", "chunks/a.js",
		"pages/MyFirstModule.Home_Web.js", "pages/Administration.Account_Overview.js"} {
		p := filepath.Join(dist, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("//"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

// writeIncompleteBundle is what mxbuild 11.12.5's second serve build leaves:
// dist replaced by a monolithic index.js and one chunk, no pages, no widgets.css.
func writeIncompleteBundle(t *testing.T, deploy string) {
	t.Helper()
	dist := filepath.Join(deploy, "web", "dist")
	if err := os.RemoveAll(dist); err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{"index.js", "chunks/b.js"} {
		p := filepath.Join(dist, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("// monolithic"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

// scriptedServe is a serve process whose builds are scripted. Each build is run by
// the process generation it lands on: proc 0 is the one the loop started with,
// and every Restart begins a new one.
type scriptedServe struct {
	build      func(proc, nth int) (*BuildResult, error) // nth build within proc, from 1
	proc       int
	nth        int
	restarts   int
	restartErr error
	reqs       []BuildRequest
}

func (f *scriptedServe) Build(req BuildRequest) (*BuildResult, error) {
	f.nth++
	f.reqs = append(f.reqs, req)
	return f.build(f.proc, f.nth)
}

func (f *scriptedServe) Restart() error {
	f.restarts++
	if f.restartErr != nil {
		return f.restartErr
	}
	f.proc++
	f.nth = 0
	return nil
}

func buildOK(restart bool) *BuildResult {
	return &BuildResult{Status: "Success", RestartRequired: restart}
}

// The reported symptom: a successful reload that leaves dist without pages.
func TestServeRecovery_IncompletePrebuiltBundleRestartsServe(t *testing.T) {
	deploy := prebuiltDeployment(t)
	f := &scriptedServe{build: func(proc, nth int) (*BuildResult, error) {
		if nth == 1 && proc == 0 {
			writeIncompleteBundle(t, deploy) // a second build in the boot's process
			return buildOK(false), nil
		}
		writeCompleteBundle(t, deploy) // the first build of a fresh process
		return buildOK(true), nil
	}}
	var out strings.Builder
	r := &serveRecovery{serve: f, deployDir: deploy, out: &out}

	res, err := r.Build(BuildRequest{ProjectFilePath: "/p/App.mpr"})
	if err != nil || !res.OK() {
		t.Fatalf("Build = %+v, %v", res, err)
	}
	if _, err := os.Stat(filepath.Join(deploy, "web", "dist", "pages", "MyFirstModule.Home_Web.js")); err != nil {
		t.Fatalf("dist/pages must be complete after the build — every page 404s otherwise: %v", err)
	}
	if f.restarts != 1 {
		t.Fatalf("restarts = %d, want 1 (a fresh serve process is the only thing that rebuilds dist completely)", f.restarts)
	}
	// The fresh process says restartRequired for an unchanged model; the
	// incremental build's answer is the one that knows what changed. Taking the
	// cold one would restart the runtime — and drop every session — per change.
	if res.RestartRequired || res.ColdRestart {
		t.Fatalf("RestartRequired must be the incremental build's (false), not the cold build's: %+v", res)
	}
	if got := out.String(); !strings.Contains(got, "dist/pages") || !strings.Contains(got, "mxbuild --serve") {
		t.Fatalf("the recovery must be reported in one line naming what was lost, got:\n%s", got)
	}
}

// Control: a build that keeps the bundle complete costs nothing extra.
func TestServeRecovery_CompleteBundleNoRestart(t *testing.T) {
	deploy := prebuiltDeployment(t)
	f := &scriptedServe{build: func(_, _ int) (*BuildResult, error) { return buildOK(false), nil }}
	r := &serveRecovery{serve: f, deployDir: deploy, out: io.Discard}
	if _, err := r.Build(BuildRequest{}); err != nil {
		t.Fatal(err)
	}
	if f.restarts != 0 {
		t.Fatalf("restarts = %d, want 0", f.restarts)
	}
}

// The rollup shape (Mendix 11.13 and earlier) has its dist written by mxcli's
// own bundler, not by the serve build; missing pages there are
// recoverMissingPages' business, and a serve restart would not help.
func TestServeRecovery_RollupShapeIsNotServesBusiness(t *testing.T) {
	deploy := prebuiltDeployment(t)
	if err := os.WriteFile(filepath.Join(deploy, "web", "rollup.config.mjs"), []byte("export default {}"), 0o600); err != nil {
		t.Fatal(err)
	}
	f := &scriptedServe{build: func(_, _ int) (*BuildResult, error) {
		writeIncompleteBundle(t, deploy)
		return buildOK(false), nil
	}}
	r := &serveRecovery{serve: f, deployDir: deploy, out: io.Discard}
	if _, err := r.Build(BuildRequest{}); err != nil {
		t.Fatal(err)
	}
	if f.restarts != 0 {
		t.Fatalf("restarts = %d, want 0 for the rollup shape", f.restarts)
	}
}

// headless (mxcli test --watch): nothing serves the client, so an incomplete
// prebuilt bundle after a successful build is not a defect to recover from —
// recovering it would restart mxbuild --serve (and rebuild, and reboot the
// runtime) on every single test-file edit for a symptom the test runner never
// observes.
func TestServeRecovery_HeadlessSkipsPrebuiltRestart(t *testing.T) {
	deploy := prebuiltDeployment(t)
	f := &scriptedServe{build: func(_, _ int) (*BuildResult, error) {
		writeIncompleteBundle(t, deploy) // the same symptom IncompletePrebuiltBundle recovers from
		return buildOK(false), nil
	}}
	r := &serveRecovery{serve: f, deployDir: deploy, out: io.Discard, headless: true}
	res, err := r.Build(BuildRequest{})
	if err != nil || !res.OK() {
		t.Fatalf("Build = %+v, %v", res, err)
	}
	if f.restarts != 0 {
		t.Fatalf("restarts = %d, want 0: headless must not pay for a restart only a browser-facing bundle needs", f.restarts)
	}
}

// The same serve process FAILS a change that regenerates a page: it wants
// web/pages/<Page>.js, which its own first build did not leave behind. Measured
// on 11.12.5 as well as 11.14. A fresh process builds it.
func TestServeRecovery_MissingClientSourceFailureRestartsServe(t *testing.T) {
	deploy := prebuiltDeployment(t)
	pagePath := filepath.Join(deploy, "web", "pages", "MyFirstModule.Home_Web.js")
	f := &scriptedServe{build: func(proc, _ int) (*BuildResult, error) {
		if proc == 0 {
			msg := "One or more errors occurred. (Could not find a part of the path '" + pagePath + "'.)"
			return &BuildResult{Status: "Failure", Message: msg}, nil
		}
		return buildOK(true), nil
	}}
	r := &serveRecovery{serve: f, deployDir: deploy, out: io.Discard}
	res, err := r.Build(BuildRequest{})
	if err != nil || !res.OK() {
		t.Fatalf("a page change must build on a fresh serve process, got %+v, %v", res, err)
	}
	if !res.RestartRequired {
		t.Fatal("with no successful incremental build to say otherwise, the cold build's restartRequired stands")
	}
	// ...but it is a first build's answer, true whatever changed, and a caller
	// that cannot restart the runtime (test --attach) must be able to say so
	// rather than claim an entity changed.
	if !res.ColdRestart {
		t.Fatal("the cold build's restartRequired must be marked ColdRestart")
	}
	if f.restarts != 1 {
		t.Fatalf("restarts = %d, want 1", f.restarts)
	}
}

// A serve process that could not be restarted must not be left for every later
// change to fail against: the next build starts one first.
func TestServeRecovery_FailedRestartIsRetriedOnNextBuild(t *testing.T) {
	deploy := prebuiltDeployment(t)
	f := &scriptedServe{restartErr: errors.New("port in use"), build: func(proc, nth int) (*BuildResult, error) {
		if proc == 0 {
			writeIncompleteBundle(t, deploy)
			return buildOK(false), nil
		}
		writeCompleteBundle(t, deploy)
		return buildOK(false), nil
	}}
	r := &serveRecovery{serve: f, deployDir: deploy, out: io.Discard}
	if _, err := r.Build(BuildRequest{}); err == nil || !strings.Contains(err.Error(), "port in use") {
		t.Fatalf("a failed restart must be reported, got %v", err)
	}
	f.restartErr = nil
	before := f.restarts
	res, err := r.Build(BuildRequest{})
	if err != nil || !res.OK() {
		t.Fatalf("second Build = %+v, %v", res, err)
	}
	if f.restarts != before+1 {
		t.Fatalf("the next build must start a serve process first, restarts %d -> %d", before, f.restarts)
	}
	if !res.ColdRestart {
		t.Fatal("the first build of the process just started is a cold answer and must be marked ColdRestart")
	}
}

// themeCompileFailure is the first Sass failure in a serve process, as
// mxbuild 11.12.5 reports it.
func themeCompileFailure() *BuildResult {
	return &BuildResult{Status: "Failure", Message: "An error occurred while compiling Theme files",
		Raw: []byte(`{"problems":{"errors":[{"message":"An error occurred while compiling Theme files","details":"Error: Undefined variable.\n  .probe2 { color: $does-not-exist; }"}],"problems":[]}}`)}
}

// staleThemeFailure is every later build in that process, after the SCSS is
// fixed: CE6093 quoting the FIRST error (mendixlabs/mxcli#1577).
func staleThemeFailure() *BuildResult {
	return &BuildResult{Status: "Failure", Message: "The project cannot be deployed, because it contains errors.",
		Problems: BuildProblems{Problems: []BuildProblem{{Severity: "Error", ErrorCode: "CE6093",
			Message: "The theme could not be compiled:\nError: Undefined variable.\n  .probe2 { color: $does-not-exist; }"}}}}
}

// The reported symptom, measured on 11.12.5 over mxbuild's /build API:
//
//	B  broken scss                 Failure  An error occurred while compiling Theme files
//	C  fixed scss, same process    Failure  CE6093, quoting B's error
//	D  again, same process         Failure  CE6093, quoting B's error
//	E  fresh process               Success
func TestServeRecovery_StaleThemeErrorRestartsServe(t *testing.T) {
	deploy := prebuiltDeployment(t)
	f := &scriptedServe{build: func(proc, nth int) (*BuildResult, error) {
		switch {
		case proc == 0 && nth == 1:
			return themeCompileFailure(), nil // the user broke the SCSS
		case proc == 0:
			return staleThemeFailure(), nil // ...and fixed it; the process still says no
		}
		return buildOK(true), nil
	}}
	var out strings.Builder
	r := &serveRecovery{serve: f, deployDir: deploy, out: &out}

	res, err := r.Build(BuildRequest{})
	if err != nil || res.OK() {
		t.Fatalf("the first theme failure is genuine and must be reported, got %+v, %v", res, err)
	}
	if f.restarts != 0 {
		t.Fatalf("a first theme failure must not restart the serve process (restarts = %d)", f.restarts)
	}

	res, err = r.Build(BuildRequest{})
	if err != nil || !res.OK() {
		t.Fatalf("after the SCSS is fixed the build must succeed on a fresh serve process, got %+v, %v", res, err)
	}
	if f.restarts != 1 {
		t.Fatalf("restarts = %d, want 1", f.restarts)
	}
	if got := out.String(); !strings.Contains(got, "theme") || !strings.Contains(got, "mxbuild --serve") {
		t.Fatalf("the restart must be reported in one line, got:\n%s", got)
	}
}

// A theme error that is still there after the restart is the user's, and is
// reported from the fresh process — whose message is current, unlike the stale
// one. One restart per build, never a loop.
func TestServeRecovery_GenuineThemeErrorSurvivesOneRestart(t *testing.T) {
	deploy := prebuiltDeployment(t)
	f := &scriptedServe{build: func(proc, nth int) (*BuildResult, error) {
		if proc == 0 && nth > 1 {
			return staleThemeFailure(), nil
		}
		return themeCompileFailure(), nil
	}}
	r := &serveRecovery{serve: f, deployDir: deploy, out: io.Discard}
	_, _ = r.Build(BuildRequest{})
	res, err := r.Build(BuildRequest{})
	if err != nil || res.OK() || res.Message != themeCompileFailure().Message {
		t.Fatalf("the fresh process's own theme failure must be returned, got %+v, %v", res, err)
	}
	if f.restarts != 1 {
		t.Fatalf("restarts = %d, want exactly 1", f.restarts)
	}
}

// Control: a theme failure after a SUCCESSFUL build is a fresh error, not a
// stale one — the process compiled the theme in between.
func TestServeRecovery_ThemeFailureAfterSuccessIsNotStale(t *testing.T) {
	deploy := prebuiltDeployment(t)
	f := &scriptedServe{build: func(_, nth int) (*BuildResult, error) {
		switch nth {
		case 1:
			return themeCompileFailure(), nil
		case 2:
			return buildOK(false), nil
		}
		return themeCompileFailure(), nil
	}}
	r := &serveRecovery{serve: f, deployDir: deploy, out: io.Discard}
	for i := 0; i < 3; i++ {
		_, _ = r.Build(BuildRequest{})
	}
	if f.restarts != 0 {
		t.Fatalf("restarts = %d, want 0: no build here followed a theme failure in the same process", f.restarts)
	}
}
