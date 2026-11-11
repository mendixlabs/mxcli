// SPDX-License-Identifier: Apache-2.0

// mxbuild --serve deletes the client source a build wrote, so its next build
// bundles a client without pages and the watch loop pays a cold build per
// change (ako/mxcli#2150). The measurements are in client_source_keep.go; the
// fake below is that behaviour: a build writes the source of the documents it
// regenerates, bundles whatever source is in web/, and deletes all of it.
package docker

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

// fakePrebuiltMxbuild is a serve process of the prebuilt shape.
type fakePrebuiltMxbuild struct {
	t      *testing.T
	deploy string
	keep   *clientSourceKeep // nil: nothing is watching, so nothing to wait for
	model  map[string]string // page name -> content, the model now
	built  map[string]string // what this process last generated
	// failBundle makes the bundler step fail when a restored source file has
	// this content.
	failBundle string
	// hide names a page whose source is deleted before any sweep can see it.
	hide string
}

func (m *fakePrebuiltMxbuild) restartProcess() { m.built = nil }

func (m *fakePrebuiltMxbuild) build() *BuildResult {
	m.t.Helper()
	web := filepath.Join(m.deploy, "web")
	cold := m.built == nil
	if cold {
		m.built = map[string]string{}
	}
	keepWrite(m.t, filepath.Join(web, "index.html"), "<html>") // never deleted
	for name, content := range m.model {
		if cold || m.built[name] != content {
			if name != m.hide {
				keepWrite(m.t, filepath.Join(web, "pages", name+".js"), content)
			}
			m.built[name] = content
		}
	}
	keepWrite(m.t, filepath.Join(web, "package.json"), "{}")
	m.waitForSweep()

	// The bundler: every source file present becomes a page module.
	dist := filepath.Join(web, "dist")
	_ = os.RemoveAll(dist)
	keepWrite(m.t, filepath.Join(dist, "index.js"), "//")
	entries, _ := os.ReadDir(filepath.Join(web, "pages"))
	for _, e := range entries {
		b, _ := os.ReadFile(filepath.Join(web, "pages", e.Name()))
		if m.failBundle != "" && string(b) == m.failBundle {
			_ = os.RemoveAll(dist)
			return &BuildResult{Status: "Failure", Message: bundleCompileMessage}
		}
		keepWrite(m.t, filepath.Join(dist, "pages", e.Name()), string(b))
	}
	if m.hide != "" && cold {
		keepWrite(m.t, filepath.Join(dist, "pages", m.hide+".js"), m.model[m.hide])
	}
	if len(entries) > 0 {
		keepWrite(m.t, filepath.Join(dist, "widgets.css"), "/**/")
	}
	_ = os.RemoveAll(filepath.Join(web, "pages"))
	_ = os.Remove(filepath.Join(web, "package.json"))
	return &BuildResult{Status: "Success", RestartRequired: cold}
}

// waitForSweep blocks until a sweep that started after the source was written
// has finished. Counting sweeps rather than sleeping keeps the test off the
// wall clock.
func (m *fakePrebuiltMxbuild) waitForSweep() {
	if m.keep == nil || m.keep.off {
		return
	}
	target := m.keep.sweeps.Load() + 2
	deadline := time.Now().Add(10 * time.Second)
	for m.keep.sweeps.Load() < target {
		if time.Now().After(deadline) {
			m.t.Fatal("the keep never swept web/ during the build")
		}
		time.Sleep(time.Millisecond)
	}
}

func keepWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func distPages(t *testing.T, deploy string) map[string]string {
	t.Helper()
	out := map[string]string{}
	entries, _ := os.ReadDir(filepath.Join(deploy, "web", "dist", "pages"))
	for _, e := range entries {
		b, _ := os.ReadFile(filepath.Join(deploy, "web", "dist", "pages", e.Name()))
		out[strings.TrimSuffix(e.Name(), ".js")] = string(b)
	}
	return out
}

// newKeptLoop boots a fake serve process the way RunLocal does — the boot
// build watched by the keep — and returns the recovery the watch loop builds
// through.
func newKeptLoop(t *testing.T, withKeep bool) (*fakePrebuiltMxbuild, *scriptedServe, *serveRecovery, *strings.Builder) {
	t.Helper()
	deploy := t.TempDir()
	m := &fakePrebuiltMxbuild{t: t, deploy: deploy, model: map[string]string{"Mod.Home": "home v1", "Mod.Other": "other v1"}}
	var keep *clientSourceKeep
	if withKeep {
		keep = newClientSourceKeep(deploy)
		t.Cleanup(keep.Close)
		m.keep = keep
	}
	f := &scriptedServe{build: func(proc, nth int) (*BuildResult, error) {
		if nth == 1 && proc > 0 {
			m.restartProcess()
		}
		return m.build(), nil
	}}
	stop := keep.watch()
	boot := m.build()
	stop(boot.OK())
	out := &strings.Builder{}
	return m, f, &serveRecovery{serve: f, deployDir: deploy, out: out, keep: keep}, out
}

// The fix: with the source put back, a change builds on the running serve
// process and the bundle is complete, the edited page included.
func TestClientSourceKeep_IncrementalBuildKeepsTheBundleComplete(t *testing.T) {
	m, f, r, out := newKeptLoop(t, true)

	m.model["Mod.Home"] = "home v2"
	res, err := r.Build(BuildRequest{ProjectFilePath: "/p/App.mpr"})
	if err != nil || !res.OK() {
		t.Fatalf("Build = %+v, %v", res, err)
	}
	if f.restarts != 0 {
		t.Fatalf("restarts = %d, want 0: the incremental build had its client source back\n%s", f.restarts, out)
	}
	if res.RestartRequired {
		t.Fatalf("a page change must not ask for a runtime restart: %+v", res)
	}
	want := map[string]string{"Mod.Home": "home v2", "Mod.Other": "other v1"}
	if got := distPages(t, m.deploy); len(got) != 2 || got["Mod.Home"] != want["Mod.Home"] || got["Mod.Other"] != want["Mod.Other"] {
		t.Fatalf("dist/pages = %v, want %v", got, want)
	}

	// A page added by an incremental build is kept too, so the build after it
	// still bundles it.
	m.model["Mod.New"] = "new v1"
	if _, err := r.Build(BuildRequest{ProjectFilePath: "/p/App.mpr"}); err != nil {
		t.Fatal(err)
	}
	m.model["Mod.Home"] = "home v3"
	if _, err := r.Build(BuildRequest{ProjectFilePath: "/p/App.mpr"}); err != nil {
		t.Fatal(err)
	}
	if got := distPages(t, m.deploy); got["Mod.New"] != "new v1" || got["Mod.Home"] != "home v3" || f.restarts != 0 {
		t.Fatalf("after adding a page then editing another: dist/pages = %v, restarts = %d", got, f.restarts)
	}
	if _, err := os.Stat(filepath.Join(m.deploy, "web", "pages")); !os.IsNotExist(err) {
		t.Fatalf("web/pages must be gone again after the build, as mxbuild leaves it: %v", err)
	}
}

// Control: the same loop without the keep is the reported behaviour — every
// change restarts mxbuild --serve.
func TestClientSourceKeep_WithoutItEveryChangeRestartsServe(t *testing.T) {
	_, f, r, _ := newKeptLoop(t, false)

	// A change that regenerates no page, such as a microflow edit.
	if _, err := r.Build(BuildRequest{ProjectFilePath: "/p/App.mpr"}); err != nil {
		t.Fatal(err)
	}
	if f.restarts != 1 {
		t.Fatalf("restarts = %d, want 1: without the kept source the incremental bundle has no pages", f.restarts)
	}
}

// A page whose source the sweeps missed is in this build's bundle and would be
// absent from the next one. The keep is emptied, so the next build is a full
// one and the page is not lost.
func TestClientSourceKeep_MissedSourceFallsBackToAFullBuild(t *testing.T) {
	deploy := t.TempDir()
	m := &fakePrebuiltMxbuild{t: t, deploy: deploy, hide: "Mod.Missed",
		model: map[string]string{"Mod.Home": "home v1", "Mod.Missed": "missed v1"}}
	keep := newClientSourceKeep(deploy)
	t.Cleanup(keep.Close)
	m.keep = keep
	f := &scriptedServe{build: func(proc, nth int) (*BuildResult, error) {
		if nth == 1 && proc > 0 {
			m.restartProcess()
			m.hide = "" // the fresh build's source is caught
		}
		return m.build(), nil
	}}
	stop := keep.watch()
	stop(m.build().OK())
	r := &serveRecovery{serve: f, deployDir: deploy, out: &strings.Builder{}, keep: keep}

	// A change that regenerates no page, such as a microflow edit.
	if _, err := r.Build(BuildRequest{ProjectFilePath: "/p/App.mpr"}); err != nil {
		t.Fatal(err)
	}
	got := distPages(t, deploy)
	if got["Mod.Missed"] != "missed v1" || got["Mod.Home"] != "home v1" {
		t.Fatalf("dist/pages = %v: the page whose source was missed must still be served", got)
	}
	if f.restarts == 0 {
		t.Fatal("a keep that missed a page's source must give way to a full build")
	}
}

// Kept source the bundler cannot compile is replaced by a full build rather
// than reported as the user's error.
func TestClientSourceKeep_BundlerFailureOnKeptSourceRebuildsFresh(t *testing.T) {
	m, f, r, out := newKeptLoop(t, true)
	m.failBundle = "other v1" // Mod.Other is unchanged, so its source is the restored copy

	m.model["Mod.Home"] = "home v2"
	f.build = func(proc, nth int) (*BuildResult, error) {
		if nth == 1 && proc > 0 {
			m.restartProcess()
			m.failBundle = ""
		}
		return m.build(), nil
	}
	res, err := r.Build(BuildRequest{ProjectFilePath: "/p/App.mpr"})
	if err != nil || !res.OK() {
		t.Fatalf("Build = %+v, %v\n%s", res, err, out)
	}
	if f.restarts != 1 {
		t.Fatalf("restarts = %d, want 1", f.restarts)
	}
	if got := distPages(t, m.deploy); got["Mod.Home"] != "home v2" || got["Mod.Other"] != "other v1" {
		t.Fatalf("dist/pages = %v", got)
	}
}

// A failed build must not leave restored source behind in web/: planWebClient
// reads web/ to decide which client the deployment has.
func TestClientSourceKeep_FailedBuildTakesRestoredSourceOutAgain(t *testing.T) {
	deploy := t.TempDir()
	k := newClientSourceKeep(deploy)
	t.Cleanup(k.Close)
	web := filepath.Join(deploy, "web")

	stop := k.watch()
	keepWrite(t, filepath.Join(web, "pages", "A.js"), "a")
	keepWrite(t, filepath.Join(web, "index.html"), "<html>")
	k.sweep(map[string]bool{})
	_ = os.RemoveAll(filepath.Join(web, "pages"))
	stop(true)

	if got := k.files(); len(got) != 1 || got[0] != "pages/A.js" {
		t.Fatalf("kept = %v, want only pages/A.js: index.html survived the build, so it is not the keep's to restore", got)
	}
	if n := k.restore(); n != 1 {
		t.Fatalf("restore = %d, want 1", n)
	}
	stop = k.watch()
	stop(false) // the build failed before deleting anything
	if _, err := os.Stat(filepath.Join(web, "pages", "A.js")); !os.IsNotExist(err) {
		t.Fatalf("restored source must be removed after a failed build: %v", err)
	}
	if got := k.files(); len(got) != 1 {
		t.Fatalf("a failed build must leave the keep as it was, got %v", got)
	}
}

// A Mendix version that leaves its client source in place needs no keep, and
// the keep stops sweeping.
func TestClientSourceKeep_SwitchesOffWhenNothingIsDeleted(t *testing.T) {
	deploy := t.TempDir()
	k := newClientSourceKeep(deploy)
	t.Cleanup(k.Close)
	web := filepath.Join(deploy, "web")

	stop := k.watch()
	keepWrite(t, filepath.Join(web, "pages", "A.js"), "a")
	keepWrite(t, filepath.Join(web, "rollup.config.mjs"), "//")
	stop(true) // both files are still there

	if !k.off {
		kept := k.files()
		sort.Strings(kept)
		t.Fatalf("the keep must switch off when the build deleted nothing it wrote; kept = %v", kept)
	}
	if n := k.restore(); n != 0 {
		t.Fatalf("restore = %d on a switched-off keep", n)
	}
}

// A nil keep is what every caller without one passes.
func TestClientSourceKeep_NilIsANoOp(t *testing.T) {
	var k *clientSourceKeep
	k.reset()
	k.Close()
	k.watch()(true)
	if k.restore() != 0 || k.files() != nil || k.missingPageSource() != nil {
		t.Fatal("nil keep must do nothing")
	}
}

// On Windows a sweep can hold web/ open while mxbuild recreates it, and the
// build fails with a sharing violation. The keep gives way: the build is run
// again without it, and it stays off.
func TestClientSourceKeep_WebDirInUseDisablesTheKeepAndRebuilds(t *testing.T) {
	m, f, r, out := newKeptLoop(t, true)
	inner := f.build
	failed := false
	f.build = func(proc, nth int) (*BuildResult, error) {
		if !failed {
			failed = true
			return &BuildResult{Status: "Failure", Message: webDirLockedMessage}, nil
		}
		return inner(proc, nth)
	}

	res, err := r.Build(BuildRequest{ProjectFilePath: "/p/App.mpr"})
	if err != nil || !res.OK() {
		t.Fatalf("Build = %+v, %v\n%s", res, err, out)
	}
	if r.keep.active() {
		t.Fatal("the keep must stay off once it has collided with a build")
	}
	if len(r.keep.files()) != 0 {
		t.Fatalf("a disabled keep must hold nothing, got %v", r.keep.files())
	}
	if got := distPages(t, m.deploy); len(got) != 2 {
		t.Fatalf("dist/pages = %v, want both pages (from the full build the loop falls back to)", got)
	}
	if !strings.Contains(out.String(), "deployment/web was in use") {
		t.Fatalf("the fallback must be reported, got:\n%s", out)
	}
	// Control for the report: a build that fails some other way is not retried.
	calls := len(f.reqs)
	f.build = func(int, int) (*BuildResult, error) {
		return &BuildResult{Status: "Failure", Message: "CE0001 something else"}, nil
	}
	if res, _ := r.Build(BuildRequest{ProjectFilePath: "/p/App.mpr"}); res.OK() || len(f.reqs) != calls+1 {
		t.Fatalf("an unrelated failure must be returned as it is, after one build; builds = %d", len(f.reqs)-calls)
	}
}

// The copy the sweeps make reads the same bytes on every platform.
func TestCopyClientSource(t *testing.T) {
	dir := t.TempDir()
	src, dst := filepath.Join(dir, "a.js"), filepath.Join(dir, "b.js")
	keepWrite(t, src, "export default 1")
	if err := copyClientSource(src, dst); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(dst); string(b) != "export default 1" {
		t.Fatalf("copy = %q", b)
	}
	if err := copyClientSource(filepath.Join(dir, "gone.js"), dst); err == nil {
		t.Fatal("a source mxbuild already deleted must be an error, not an empty copy")
	}
}
