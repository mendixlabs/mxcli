// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// serve_recovery.go recovers the watch loop from state a long-lived
// `mxbuild --serve` process keeps and only a fresh process clears.
//
// On Mendix versions where mxbuild writes web/dist itself (the prebuilt shape:
// measured on 11.12.5, and 11.14), the FIRST build in a serve process is the
// only complete one. It leaves no web/pages, web/layouts or bundler config
// behind, so every later build in the same process either
//
//   - fails, when the change regenerates a page or layout ("Could not find a part
//     of the path '…/web/pages/X.js'") — the shape legacyClientBuildHint names;
//     or
//   - SUCCEEDS with an incomplete bundle, for every other change and for no
//     change at all: dist/ is replaced by a monolithic index.js and one chunk,
//     with no pages/ and no widgets.css, so the browser 404s on every page while
//     the loop reports "applied via reload" (mendixlabs/mxcli#1580).
//
// Measured by driving mxbuild 11.12.5's /build API directly on a blank app:
//
//	build                          restartRequired   dist after it
//	1  cold                        true              13 pages, widgets.css, 5.6 KB index.js
//	2  same process, NO change     false             0 pages, no widgets.css, 683 KB index.js
//	3  same process, rm web/dist   false             same as 2
//	4  same process, rm web/       Failure           -
//	5  fresh process               true              complete again
//
// So nothing done to the deployment from outside helps, and a fresh process
// always does. The cost of one is a ~5s start plus a cold ~18s build per change,
// against a 4s reload that serves no pages — correct beats fast.
//
// The same process also keeps a theme error past its fix, in every shape
// (mendixlabs/mxcli#1577). Measured on 11.12.5 over the same API:
//
//	broken scss                 Failure  An error occurred while compiling Theme files
//	fixed scss, same process    Failure  CE6093 The theme could not be compiled, quoting the OLD error
//	again, same process         Failure  the same
//	fresh process               Success
//
// Running the bundled Dart Sass on the generated entry point at the second step
// compiles cleanly, so the project is fine and the error is process state.

// serveProcess is the part of *ServeServer the watch loop's recovery drives — a
// seam so the recovery is testable without an mxbuild.
type serveProcess interface {
	Build(BuildRequest) (*BuildResult, error)
	Restart() error
}

// serveRecovery runs the watch loop's builds, restarting the serve process and
// building once more when a build's outcome is one only a fresh process fixes.
type serveRecovery struct {
	serve     serveProcess
	deployDir string
	out       io.Writer

	// restartPending is set when a restart failed: the process it was meant to
	// replace is gone, so the next build starts one first instead of failing
	// against nothing on every later change.
	restartPending bool

	// themeFailed is set while the CURRENT serve process's last theme compile
	// failed. A theme failure then is not trustworthy: the process may be
	// reporting the earlier error rather than the source on disk.
	themeFailed bool

	// headless: nothing serves the client (the test runner), so a build that
	// succeeds with an incomplete web/dist is a good build and is not repeated.
	headless bool
}

func (r *serveRecovery) Build(req BuildRequest) (*BuildResult, error) {
	freshProcess := r.restartPending
	if r.restartPending {
		fmt.Fprintln(r.out, "  starting mxbuild --serve (the last restart failed)...")
		if err := r.restart(); err != nil {
			return nil, err
		}
	}
	before, prebuilt := readPrebuiltBundle(r.deployDir)

	res, err := r.serve.Build(req)
	if err != nil {
		return res, err
	}
	if freshProcess && res.OK() {
		res.ColdRestart = true // the first build of the process just started
	}
	staleTheme := r.themeFailed && isThemeFailure(res)
	r.observe(res)

	if staleTheme {
		fmt.Fprintln(r.out, "  the theme error may be stale — mxbuild --serve keeps reporting its first Sass error "+
			"(CE6093) after the SCSS is fixed; restarting mxbuild --serve to compile the theme afresh...")
		// The fresh process's answer stands either way: a theme error it
		// reports is the source's own, and current.
		return r.rebuildFresh(req)
	}

	if res.OK() {
		if !prebuilt || r.headless {
			return res, nil
		}
		after, _ := readPrebuiltBundle(r.deployDir)
		lost := before.lostIn(after)
		if len(lost) == 0 {
			return res, nil
		}
		fmt.Fprintf(r.out, "  mxbuild --serve's incremental build left web/dist without %s "+
			"(this Mendix version bundles the client completely only in a serve process's first build); "+
			"restarting mxbuild --serve for a full build...\n", strings.Join(lost, " and "))
		fresh, err := r.rebuildFresh(req)
		if err != nil || !fresh.OK() {
			return fresh, err
		}
		// A fresh process's first build reports restartRequired even for an
		// unchanged model (measured: true on every cold build), because it has
		// no previous build to compare with. The incremental build that just
		// succeeded does, and its model is the one the cold build rewrote
		// byte-for-byte; taking the cold answer would restart the runtime — and
		// drop every browser session — on every change.
		fresh.RestartRequired, fresh.ColdRestart = res.RestartRequired, false
		return fresh, nil
	}

	if legacyClientBuildHint(r.deployDir, res.Message, string(res.Raw)) != "" {
		fmt.Fprintln(r.out, "  mxbuild --serve cannot build this change incrementally (it wants client source its "+
			"first build does not leave behind); restarting mxbuild --serve for a full build...")
		// No incremental build succeeded, so the cold build's restartRequired
		// is the only answer there is, and it is marked ColdRestart: a caller
		// that cannot restart the runtime must not read it as "an entity
		// changed".
		return r.rebuildFresh(req)
	}
	return res, nil
}

// rebuildFresh replaces the serve process and builds once on the new one. Its
// RestartRequired is a first build's, so it is marked ColdRestart; a caller
// that has an incremental answer replaces both.
func (r *serveRecovery) rebuildFresh(req BuildRequest) (*BuildResult, error) {
	if err := r.restart(); err != nil {
		return nil, err
	}
	res, err := r.serve.Build(req)
	if err == nil {
		r.observe(res)
		if res != nil && res.OK() {
			res.ColdRestart = true
		}
	}
	return res, err
}

func (r *serveRecovery) restart() error {
	r.themeFailed = false // whatever the old process held went with it
	if err := r.serve.Restart(); err != nil {
		r.restartPending = true
		return fmt.Errorf("restarting mxbuild --serve: %w", err)
	}
	r.restartPending = false
	return nil
}

// observe records what a build says about the current process's theme state.
// A non-theme failure leaves it as it was: the process still holds whatever it
// held.
func (r *serveRecovery) observe(res *BuildResult) {
	switch {
	case res == nil:
	case res.OK():
		r.themeFailed = false
	case isThemeFailure(res):
		r.themeFailed = true
	}
}

// themeCompileMessage is mxbuild's message for a theme that failed to compile
// in this build; CE6093 is the consistency error a later build reports for it.
const themeCompileMessage = "An error occurred while compiling Theme files"

// isThemeFailure reports whether a failed build failed on the theme.
func isThemeFailure(res *BuildResult) bool {
	if res == nil || res.OK() {
		return false
	}
	if strings.Contains(res.Message, themeCompileMessage) {
		return true
	}
	for _, p := range res.Errors() {
		if p.ErrorCode == "CE6093" || strings.Contains(p.Message, themeCompileMessage) {
			return true
		}
	}
	return false
}

// prebuiltBundle is what a prebuilt-shape web/dist holds that the incremental
// serve build loses.
type prebuiltBundle struct {
	pages      int  // page modules under dist/pages
	widgetsCSS bool // dist/widgets.css
}

// readPrebuiltBundle reads the bundle when the deployment is the prebuilt shape,
// and reports false for every other shape: in the rollup shape dist is written
// by mxcli's own bundler (recoverMissingPages' business), and the classic client
// has no dist at all.
func readPrebuiltBundle(deployDir string) (prebuiltBundle, bool) {
	if planWebClient(deployDir) != webClientPrebuilt {
		return prebuiltBundle{}, false
	}
	dist := filepath.Join(deployDir, "web", "dist")
	var b prebuiltBundle
	_ = filepath.WalkDir(filepath.Join(dist, "pages"), func(path string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.HasSuffix(d.Name(), ".js") {
			b.pages++
		}
		return nil
	})
	if fi, err := os.Stat(filepath.Join(dist, "widgets.css")); err == nil && !fi.IsDir() {
		b.widgetsCSS = true
	}
	return b, true
}

// lostIn names what the bundle had before a build and no longer has. Only a
// total loss counts: a change that deletes one page legitimately shrinks
// dist/pages, while every page vanishing at once is the incomplete bundle.
func (b prebuiltBundle) lostIn(after prebuiltBundle) []string {
	var lost []string
	if b.pages > 0 && after.pages == 0 {
		lost = append(lost, fmt.Sprintf("dist/pages (%d page modules)", b.pages))
	}
	if b.widgetsCSS && !after.widgetsCSS {
		lost = append(lost, "dist/widgets.css")
	}
	// widgets.css alone going is not this defect's signature — an app whose
	// last styled pluggable widget was just removed loses it legitimately.
	if b.pages == 0 || after.pages > 0 {
		return nil
	}
	return lost
}
