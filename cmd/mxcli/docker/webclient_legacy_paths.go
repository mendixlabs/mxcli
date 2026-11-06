// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// webclient_legacy_paths.go recognises one mxbuild failure that only ever
// appears under --watch, and only on Mendix 11.14+.
//
// On 11.14 the FIRST build in an `mxbuild --serve` process succeeds and every
// SUBSEQUENT build in that process fails, because the first one does not leave
// the deployment in a state its own incremental build can continue from. Two
// artifacts are missing, and which one the build dies on depends on how far it
// gets before it needs them:
//
//	the bundler's config file   web/rollup.config.mjs or web/rspack.config.mjs
//	the per-document client     web/pages/, web/layouts/
//
// Measured against mxbuild 11.14.0 driven directly over its HTTP API — no
// mxcli in the picture — on a blank app, POSTing the SAME /build request twice
// with the model untouched between them:
//
//	build 1   Success
//	build 2   Failure — ERR_MODULE_NOT_FOUND for web/rollup.config.mjs,
//	                    imported from mxbuild's own tools/node/rollup-runner.mjs
//
// It is not the app's choice of bundler. With App Settings > Runtime > App
// bundler flipped to Rspack the shape is identical, naming the other file:
// "Failed to load Rspack configuration file … web/rspack.config.mjs". So
// switching bundlers is not a workaround.
//
// Nor is restoring the deleted config, which was worth ruling out because the
// config IS recoverable: it appears on disk for ~1.5s mid-build and carries
// nothing model-specific (no page list, no widget list, no hashes; its input is
// just index.js, with page discovery delegated to a rollup plugin at build
// time), so it could be captured and put back the way `mxcli fix widgets`
// harvests mxbuild's own output. Measured, that rescues only the case nobody
// needs:
//
//	build   model      state                     result
//	1       -          cold                      Success
//	2,3     unchanged  config restored           Success
//	2'      CHANGED    config restored           Failure - missing web/pages/*.js
//	3'      CHANGED    config restored + dirs    Failure - widget export
//
// So there are two regressions, not one. The config deletion is the visible,
// recoverable half; underneath it the per-document client export expects
// deployment state 11.14's cold build no longer produces, and supplying the
// missing directories only moves the failure to exporting two pluggable widgets
// that ship with a blank app. Across 2' and 3', web/dist/index.js never moved
// off its cold-build timestamp. Nothing done to the deployment fixes that half.
// A fresh serve process does — its first build is the one that succeeds — so
// the watch loop now restarts mxbuild --serve on this shape (serve_recovery.go,
// which also measured it on 11.12.5), and this hint is only printed when the
// build on the fresh process fails as well.
//
// The controls that place this in mxbuild rather than here: a one-shot
// `mxbuild --target=deploy` run TWICE into the same deployment directory
// succeeds both times (so the 11.14 deployment shape is not the trigger — a
// fresh process is happy with the directory a serve process chokes on), and
// mxcli's /build request carries exactly the four fields mxbuild advertises in
// its own error response.
//
// What mxcli can do is not let the failure read as the user's model being
// broken. The message names absolute paths inside deployment/ and nothing
// about why they are missing, so the natural response is `rm -rf deployment/`
// — which costs a cold build and changes nothing, because the next second
// build fails the same way.

// legacyClientDirs are the per-document client output directories a pre-11.14
// serve Deploy build creates and 11.14+ does not.
var legacyClientDirs = []string{"pages", "layouts"}

// bundlerConfigs are the bundler config files 11.14's first serve build does
// not leave behind. One per bundler, because the app can be set to either and
// both fail the same way.
var bundlerConfigs = []string{"rollup.config.mjs", "rspack.config.mjs"}

// legacyClientBuildHint explains an incremental build that failed on something
// this Mendix version's first build did not leave behind. It returns "" for
// every other failure.
//
// message is the serve response's message; raw is the full body, which is
// where the bundler-config failure puts its detail (the message there is only
// "Compilation of the app bundle failed"). The check is on the failure's own
// shape rather than on the Mendix version, so a future mxbuild that fixes this
// goes quiet on its own and no unrelated build failure is explained away.
func legacyClientBuildHint(deployDir, message, raw string) string {
	if missing := missingClientDirs(deployDir, message); len(missing) > 0 {
		return hintText("this build wants " + strings.Join(missing, " and ") +
			", which this Mendix version's first build does not create")
	}
	if name := missingBundlerConfig(deployDir, message+raw); name != "" {
		return hintText("mxbuild's own bundler cannot find web/" + name +
			", which its own first build does not leave behind")
	}
	return ""
}

// missingClientDirs returns the per-document client directories the failure
// names and the deployment genuinely lacks.
func missingClientDirs(deployDir, message string) []string {
	// macOS and Linux builds of mxbuild report the same failure with the resource
	// key instead of the English text: "AggregateException_ctor_DefaultMessage
	// (IO_PathNotFound_Path, <path>)" (measured on 11.15.0, macOS).
	byKey := strings.Contains(message, "IO_PathNotFound_Path")
	if !strings.Contains(message, "Could not find a part of the path") && !byKey {
		return nil
	}
	var named []string
	for _, dir := range legacyClientDirs {
		if !strings.Contains(message, filepath.Join("web", dir)+string(filepath.Separator)) {
			continue
		}
		if _, err := os.Stat(filepath.Join(deployDir, "web", dir)); os.IsNotExist(err) {
			named = append(named, "web/"+dir+"/")
		} else if byKey {
			// The key form names a file, not a directory, and the directory can exist
			// (an earlier mxbuild left some pages) while the file the build wants is
			// not there: the same first-build-only state.
			named = append(named, "web/"+dir+"/ files")
		}
	}
	return named
}

// missingBundlerConfig returns the bundler config file the failure names and
// the deployment genuinely lacks, or "".
func missingBundlerConfig(deployDir, body string) string {
	if !strings.Contains(body, "ERR_MODULE_NOT_FOUND") {
		return ""
	}
	for _, name := range bundlerConfigs {
		if !strings.Contains(body, name) {
			continue
		}
		if _, err := os.Stat(filepath.Join(deployDir, "web", name)); os.IsNotExist(err) {
			return name
		}
	}
	return ""
}

func hintText(what string) string {
	return fmt.Sprintf(
		"  This is not a problem with your model, and deployment/ is not corrupt:\n"+
			"  %s.\n"+
			"  On Mendix 11.12.5 and 11.14 the first build in a serve process succeeds and\n"+
			"  every later one fails this way — measured with the model untouched between two\n"+
			"  identical builds, and with either app bundler, so switching bundlers does not help.\n"+
			"  Deleting deployment/ does not either: the next second build fails the same way.\n"+
			"  mxcli already retried on a fresh mxbuild --serve; if this persists, drop --watch\n"+
			"  and restart per change:\n"+
			"    mxcli run --local --screenshot\n",
		what)
}
