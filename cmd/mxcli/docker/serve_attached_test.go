// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"io"
	"os"
	"path/filepath"
	"testing"
)

// hostServe is the dev loop's serve process as an attach sees it: every build
// is a later build in that process, so it leaves the prebuilt bundle without
// pages. Restarting it is not the attach's to do.
type hostServe struct {
	t      *testing.T
	deploy string
	builds int
}

func (h *hostServe) Build(BuildRequest) (*BuildResult, error) {
	h.builds++
	writeIncompleteBundle(h.t, h.deploy)
	return buildOK(false), nil
}

func (h *hostServe) Restart() error {
	h.t.Fatal("the attach restarted the dev loop's mxbuild --serve, which it does not own")
	return nil
}

// privateServe is a serve process the attach started: its first build is
// complete, a later one is not, as with any serve process.
type privateServe struct {
	scriptedServe
	stops int
}

func (p *privateServe) Stop() error { p.stops++; return nil }

func newPrivateServe(t *testing.T, deploy string) *privateServe {
	return &privateServe{scriptedServe: scriptedServe{build: func(_, nth int) (*BuildResult, error) {
		if nth == 1 {
			writeCompleteBundle(t, deploy)
			return buildOK(true), nil
		}
		writeIncompleteBundle(t, deploy)
		return buildOK(false), nil
	}}}
}

func attachedServeFor(host serveProcess, deploy string, start func() (ownedServe, error)) *AttachedServe {
	proc := &borrowedServe{host: host, start: start, out: io.Discard}
	return &AttachedServe{rec: &serveRecovery{serve: proc, deployDir: deploy, out: io.Discard}, proc: proc}
}

func pagesIn(t *testing.T, deploy string) int {
	t.Helper()
	pages, _ := filepath.Glob(filepath.Join(deploy, "web", "dist", "pages", "*.js"))
	return len(pages)
}

// ako/mxcli#1917: `test --attach` built through the dev loop's serve process,
// whose later builds leave dist without pages, and the app answered 404 on
// every page afterwards. The build now completes the bundle on a serve process
// of the attach's own, and leaves the dev loop's alone.
func TestAttachedServe_CompletesTheBundleOnItsOwnServe(t *testing.T) {
	deploy := prebuiltDeployment(t)
	host := &hostServe{t: t, deploy: deploy}
	var own *privateServe
	started := 0
	a := attachedServeFor(host, deploy, func() (ownedServe, error) {
		started++
		own = newPrivateServe(t, deploy)
		return own, nil
	})

	res, err := a.Build(BuildRequest{ProjectFilePath: "/p/App.mpr"})
	if err != nil || !res.OK() {
		t.Fatalf("Build = %+v, %v", res, err)
	}
	if pagesIn(t, deploy) == 0 {
		t.Fatal("dist/pages is empty after the attach's build — the hosting app 404s on every page")
	}
	if host.builds != 1 || started != 1 {
		t.Fatalf("host builds = %d, private serves started = %d; want the host tried once, then one private serve", host.builds, started)
	}
	if res.RestartRequired {
		t.Fatal("RestartRequired must be the incremental build's answer, not the cold one's")
	}

	// The cleanup build lands on the private serve, where it is a later build
	// too: recovered by restarting the private serve, never the host's.
	if res, err := a.Build(BuildRequest{ProjectFilePath: "/p/App.mpr"}); err != nil || !res.OK() {
		t.Fatalf("second Build = %+v, %v", res, err)
	}
	if pagesIn(t, deploy) == 0 {
		t.Fatal("dist/pages is empty after the attach's cleanup build")
	}
	if host.builds != 1 || own.restarts != 1 {
		t.Fatalf("host builds = %d, private restarts = %d; want the cleanup on the private serve, restarted once", host.builds, own.restarts)
	}

	a.Close()
	if own.stops != 1 {
		t.Fatalf("private serve stopped %d times on Close, want 1", own.stops)
	}
}

// Mendix 11.14.0: the host's build fails outright, even for an unchanged model,
// and the failure carries no restartRequired (measured over mxbuild's /build
// API). The private serve's cold build is then the only answer, and it asks for
// a restart whatever changed: the result says it is that answer, so the attach
// can refuse without blaming an entity.
func TestAttachedServe_FailedHostBuildLeavesOnlyAColdAnswer(t *testing.T) {
	deploy := prebuiltDeployment(t)
	config := filepath.Join(deploy, "web", "rollup.config.mjs")
	host := &scriptedServe{build: func(_, _ int) (*BuildResult, error) {
		return &BuildResult{Status: "Failure", Message: "Compilation of the app bundle failed.",
			Raw: []byte(`{"problems":{"errors":[{"details":"Error [ERR_MODULE_NOT_FOUND]: Cannot find module '` +
				config + `' imported from rollup-runner.mjs"}]},"status":"Failure"}`)}, nil
	}}
	var own *privateServe
	a := attachedServeFor(host, deploy, func() (ownedServe, error) {
		own = newPrivateServe(t, deploy)
		return own, nil
	})
	res, err := a.Build(BuildRequest{ProjectFilePath: "/p/App.mpr"})
	if err != nil || !res.OK() {
		t.Fatalf("Build = %+v, %v", res, err)
	}
	if !res.RestartRequired || !res.ColdRestart {
		t.Fatalf("want the cold build's restartRequired, marked ColdRestart; got %+v", res)
	}
	if host.restarts != 0 || own == nil {
		t.Fatalf("host restarts = %d, private serve started = %v; want only a private one", host.restarts, own != nil)
	}
	if pagesIn(t, deploy) == 0 {
		t.Fatal("dist/pages is empty after the attach's build")
	}
	a.Close()
}

// Control: a host build that leaves the bundle complete is all there is — no
// private serve process is started for it.
func TestAttachedServe_CompleteHostBuildStartsNothing(t *testing.T) {
	deploy := prebuiltDeployment(t)
	host := &scriptedServe{build: func(_, _ int) (*BuildResult, error) {
		writeCompleteBundle(t, deploy)
		return buildOK(false), nil
	}}
	a := attachedServeFor(host, deploy, func() (ownedServe, error) {
		t.Fatal("started a private serve process for a complete build")
		return nil, nil
	})
	if res, err := a.Build(BuildRequest{}); err != nil || !res.OK() {
		t.Fatalf("Build = %+v, %v", res, err)
	}
	if _, err := os.Stat(filepath.Join(deploy, "web", "dist", "widgets.css")); err != nil {
		t.Fatal(err)
	}
	a.Close() // nothing to stop
}
