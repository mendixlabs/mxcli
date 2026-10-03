// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/internal/testutil"
)

// The local loop resolves mxbuild once (ResolveMxBuildForLocal) and then started
// mxbuild --serve from a SECOND resolver inside StartServe, which walked the
// cache on its own. The two answers differed, and the serve one won — so a Mac
// with Studio Pro installed built with whatever the cache happened to hold.
// These tests pin both halves of the fix. (issue #1122)

// plantCacheEntry writes <home>/.mxcli/mxbuild/<version>/modeler/mxbuild plus the
// runtime/ sibling verifyMxBuildCache requires, so resolution reaches its verdict
// rather than tripping the cache-completeness guard first.
func plantCacheEntry(t *testing.T, home, version string) string {
	t.Helper()
	base := filepath.Join(home, ".mxcli", "mxbuild", version)
	if err := os.MkdirAll(filepath.Join(base, "runtime"), 0o755); err != nil {
		t.Fatal(err)
	}
	return writeBinary(t, filepath.Join(base, "modeler"), "mxbuild", elfMagic)
}

// TestStartServeRefusesVersionSubstitution is the reporter's shape 1. With no
// cache entry for the project's version, StartServe used to fall through to
// AnyCachedMxBuildPath — the NEWEST cached version, whatever it was — and hand it
// to mxbuild, which rejected it several minutes later with "Project version
// '11.12.2' does not exactly match MxBuild version '11.14.0'".
//
// A known version that is not cached must fail HERE, naming both versions.
func TestStartServeRefusesVersionSubstitution(t *testing.T) {
	home := t.TempDir()
	testutil.SetHome(t, home)
	plantCacheEntry(t, home, "11.14.0")

	_, err := StartServe(ServeOptions{Version: "11.12.2"})
	if err == nil {
		t.Fatal("a cache holding only 11.14.0 must not satisfy a request for 11.12.2")
	}
	for _, want := range []string{"11.12.2", "11.14.0"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error should name %q so the mismatch is visible; got:\n%v", want, err)
		}
	}
}

// TestStartServeRefusesModelerlessCacheDir covers how the cache comes to be in
// that state, which is not an accident of the user's setup: step 2 of the same
// command creates <cache>/<version>/ to hold the runtime symlink, and on macOS
// nothing ever puts a modeler/ beside it (mxbuild comes from Studio Pro). So the
// half-populated directory that defeats CachedMxBuildPath is mxcli's own work.
func TestStartServeRefusesModelerlessCacheDir(t *testing.T) {
	home := t.TempDir()
	testutil.SetHome(t, home)
	plantCacheEntry(t, home, "11.14.0")

	// What ensureMxBuildRuntimeSibling leaves behind for the project's version.
	runtimeSrc := filepath.Join(home, ".mxcli", "runtime", "11.12.2", "runtime")
	if err := os.MkdirAll(runtimeSrc, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := ensureMxBuildRuntimeSibling("11.12.2", io.Discard); err != nil {
		t.Fatal(err)
	}
	if got := CachedMxBuildPath("11.12.2"); got != "" {
		t.Fatalf("precondition: the cache dir must have no modeler entry, got %q", got)
	}

	_, err := StartServe(ServeOptions{Version: "11.12.2"})
	if err == nil {
		t.Fatal("a modeler-less cache dir must not silently resolve to another version")
	}
	// Asserting only that this fails proves nothing: the old code substituted
	// 11.14.0 and then failed anyway, one exec later, with "exec format error".
	// The verdict has to be reached HERE, naming the version that is missing.
	if !strings.Contains(err.Error(), "no mxbuild for Mendix 11.12.2") {
		t.Errorf("resolution must refuse up front, not fail later on a substituted binary; got:\n%v", err)
	}
}

// TestStartServeUsesAnyCachedOnlyWithoutAVersion keeps the fallback available
// where it is not a guess: a caller that names no version has nothing to be
// mismatched against.
func TestStartServeUsesAnyCachedOnlyWithoutAVersion(t *testing.T) {
	home := t.TempDir()
	testutil.SetHome(t, home)
	planted := plantCacheEntry(t, home, "11.14.0")

	if got := resolveServeMxBuild(ServeOptions{}); got != planted {
		t.Errorf("with no version requested, resolution should fall back to the newest cached entry %q, got %q", planted, got)
	}
}

// TestServeOptionsForCarriesResolvedMxBuild is the call-site half. Both local
// entry points resolve mxbuild for this host and then build a ServeOptions; the
// bug was that the resolved path was dropped on the floor at that exact step
// (the web-client bundler received it, the build server did not). Constructing
// the options through one function is what stops the two from drifting again.
func TestServeOptionsForCarriesResolvedMxBuild(t *testing.T) {
	opts := serveOptionsFor("/Applications/Mendix Studio Pro 11.12.2.app/Contents/modeler/mxbuild", "11.12.2", 21, 6543)

	if opts.MxBuildPath == "" {
		t.Fatal("the resolved mxbuild must reach StartServe; an empty MxBuildPath sends it back to the cache")
	}
	if opts.Version != "11.12.2" || opts.JavaMajor != 21 || opts.Port != 6543 {
		t.Errorf("unexpected options: %+v", opts)
	}
}
