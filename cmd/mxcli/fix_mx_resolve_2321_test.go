// SPDX-License-Identifier: Apache-2.0

package main

import (
	"errors"
	"strings"
	"testing"
)

// ako/mxcli#2321: `mxcli fix widgets` on macOS ran an mx of another version
// than the project's — ResolveMxForVersion treats the version as a preference
// and falls back to an mx on PATH or the newest install — and `mx
// update-widgets` refused: "Project is in different Mendix version - 11.12.2"
// (exit 4). update-widgets only works on its own version, so fix needs that
// exact mx, or says which one it needs.

func TestResolveFixMx_ExactLocalVersion_Issue2321(t *testing.T) {
	exact := func(v string) string {
		if v == "11.12.2" {
			return "/Applications/Studio Pro 11.12.2.app/modeler/mx"
		}
		return ""
	}
	got, err := resolveFixMx("", "11.12.2", "darwin", exact, nil)
	if err != nil || got != "/Applications/Studio Pro 11.12.2.app/modeler/mx" {
		t.Errorf("got %q, %v; want the exact-version mx", got, err)
	}
}

// No exact mx on macOS: a downloaded Linux mx cannot run there, so the refusal
// names the version and the ways to provide it — not a mismatched mx.
func TestResolveFixMx_MacWithoutExactVersionSaysWhich_Issue2321(t *testing.T) {
	none := func(string) string { return "" }
	download := func(string) (string, error) { t.Fatal("must not download a Linux mx on macOS"); return "", nil }
	_, err := resolveFixMx("", "11.12.2", "darwin", none, download)
	if err == nil {
		t.Fatal("resolved an mx although none of 11.12.2 exists")
	}
	for _, want := range []string{"11.12.2", "--mx", "different Mendix version"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q lacks %q", err, want)
		}
	}
}

// On Linux the exact version is downloaded, as `mxcli new` does.
func TestResolveFixMx_LinuxDownloadsTheExactVersion_Issue2321(t *testing.T) {
	none := func(string) string { return "" }
	var asked string
	download := func(v string) (string, error) { asked = v; return "/cache/" + v + "/mx", nil }
	got, err := resolveFixMx("", "11.12.2", "linux", none, download)
	if err != nil || asked != "11.12.2" || got != "/cache/11.12.2/mx" {
		t.Errorf("got %q, %v (downloaded %q); want the 11.12.2 download", got, err, asked)
	}
	failing := func(string) (string, error) { return "", errors.New("offline") }
	if _, err := resolveFixMx("", "11.12.2", "linux", none, failing); err == nil || !strings.Contains(err.Error(), "11.12.2") {
		t.Errorf("failed download: %v", err)
	}
}

// CONTROL: an explicit --mx is used as given.
func TestResolveFixMx_ExplicitOverride_Issue2321(t *testing.T) {
	got, err := resolveFixMx("/opt/mxbuild/modeler/mx", "11.12.2", "darwin", func(string) string { return "" }, nil)
	if err != nil || got != "/opt/mxbuild/modeler/mx" {
		t.Errorf("got %q, %v", got, err)
	}
}
