// SPDX-License-Identifier: Apache-2.0

// `mxcli new --version X` silently produced a project at a DIFFERENT version.
//
// ResolveMxForNewProject delegated to ResolveMxForVersion, whose last resort is
// AnyCachedMxPath() — any cached mx, of any version. That fallback is defensible for
// check/build, where the project already exists and its version is a preference. It is
// wrong for `new`, where the requested version is not a preference but the definition
// of the output: `mx create-project` stamps the project with ITS OWN version, so a
// mismatched binary produces a project the user did not ask for.
//
// Observed: with only 11.6.6 cached, `mxcli new --version 11.12.2` printed
// "Resolving MxBuild 11.12.2..." and produced a Mendix 11.6.6 project, with no
// warning. `mxcli setup mxbuild --version 11.12.2` downloaded it fine, so the CDN was
// never the problem — the resolver just never asked.
package docker

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mendixlabs/mxcli/internal/testutil"
)

// cacheFakeMx plants a fake cached mx binary for each version under a fake HOME.
func cacheFakeMx(t *testing.T, home string, versions ...string) map[string]string {
	t.Helper()
	paths := make(map[string]string, len(versions))
	for _, v := range versions {
		modelerDir := filepath.Join(home, ".mxcli", "mxbuild", v, "modeler")
		if err := os.MkdirAll(modelerDir, 0755); err != nil {
			t.Fatal(err)
		}
		bin := filepath.Join(modelerDir, mxBinaryName())
		if err := os.WriteFile(bin, []byte("fake"), 0755); err != nil {
			t.Fatal(err)
		}
		paths[v] = bin
	}
	return paths
}

func isolateResolution(t *testing.T, home string) {
	t.Helper()
	testutil.SetHome(t, home)
	setTestApplicationsDir(t, t.TempDir()) // no real macOS Studio Pro
	t.Setenv("PATH", t.TempDir())          // no mx on PATH
}

// TestLocalMxForVersion_RefusesAnotherVersion is the bug: a cached 11.6.6 must not be
// offered up when 11.12.2 was requested.
func TestLocalMxForVersion_RefusesAnotherVersion(t *testing.T) {
	home := t.TempDir()
	isolateResolution(t, home)
	cacheFakeMx(t, home, "11.6.6")

	if got := localMxForVersion("11.12.2"); got != "" {
		t.Errorf("localMxForVersion(11.12.2) = %q with only 11.6.6 cached; a different "+
			"version is not a substitute — mx create-project stamps the project with its "+
			"own version, so this silently produces the wrong project", got)
	}
}

// TestLocalMxForVersion_AcceptsExactMatch: an exact match must still be reused, so a
// cached version is not re-downloaded.
func TestLocalMxForVersion_AcceptsExactMatch(t *testing.T) {
	home := t.TempDir()
	isolateResolution(t, home)
	want := cacheFakeMx(t, home, "11.6.6", "11.12.2")["11.12.2"]

	if got := localMxForVersion("11.12.2"); got != want {
		t.Errorf("localMxForVersion(11.12.2) = %q, want the exact cached binary %q", got, want)
	}
}

// TestLocalMxForVersion_EmptyVersion: with no version requested there is nothing to
// match, so nothing local qualifies and the caller must decide.
func TestLocalMxForVersion_EmptyVersion(t *testing.T) {
	home := t.TempDir()
	isolateResolution(t, home)
	cacheFakeMx(t, home, "11.6.6")

	if got := localMxForVersion(""); got != "" {
		t.Errorf("localMxForVersion(\"\") = %q, want \"\" — no requested version means no match", got)
	}
}

// TestResolveMxForNewProject_UsesExactCachedVersion covers the wiring: the exact match
// short-circuits before any download is attempted.
func TestResolveMxForNewProject_UsesExactCachedVersion(t *testing.T) {
	home := t.TempDir()
	isolateResolution(t, home)
	want := cacheFakeMx(t, home, "11.6.6", "11.12.2")["11.12.2"]

	got, err := ResolveMxForNewProject("11.12.2", os.Stderr)
	if err != nil {
		t.Fatalf("ResolveMxForNewProject: %v", err)
	}
	if got != want {
		t.Errorf("ResolveMxForNewProject(11.12.2) = %q, want %q", got, want)
	}
}
