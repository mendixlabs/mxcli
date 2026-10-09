// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
)

// withBuildVersion sets the build's version variables for one test: ld is what
// -X main.Version would have set ("" for a plain `go build`), v is the
// package default the rest of the code reads.
func withBuildVersion(t *testing.T, ld, v string) {
	t.Helper()
	oldLD, oldV := Version, version
	Version, version = ld, v
	t.Cleanup(func() { Version, version = oldLD, oldV })
}

// stubDevcontainerDownload replaces the downloader for one test and returns a
// pointer to the number of times it was called.
func stubDevcontainerDownload(t *testing.T, fn func(tag, outPath string) error) *int {
	t.Helper()
	old := downloadDevcontainerBinary
	calls := 0
	downloadDevcontainerBinary = func(repo, tag, targetOS, targetArch, outputPath string, w io.Writer) error {
		calls++
		return fn(tag, outputPath)
	}
	t.Cleanup(func() { downloadDevcontainerBinary = old })
	return &calls
}

// TestFetchDevcontainerMxcli_DownloadErrorIsAWarning is the #1365 regression:
// steps 1-6 of `mxcli new` have already produced a complete project, so a
// failed fetch of the Linux binary must not turn the whole command into a
// failure. The function has to return normally with a warning on stderr and
// the hint for fixing it by hand.
func TestFetchDevcontainerMxcli_DownloadErrorIsAWarning(t *testing.T) {
	withBuildVersion(t, "v0.25.0", "v0.25.0")
	calls := stubDevcontainerDownload(t, func(string, string) error {
		return errors.New("HTTP 404")
	})

	var out, errOut bytes.Buffer
	fetchDevcontainerMxcli("/proj/mxcli", &out, &errOut)

	if *calls != 1 {
		t.Fatalf("expected one download attempt, got %d", *calls)
	}
	got := errOut.String()
	if !strings.Contains(got, "  Warning: could not download the Linux mxcli binary for the devcontainer: HTTP 404") {
		t.Errorf("stderr lacks the warning:\n%s", got)
	}
	if strings.Contains(got, "Error:") {
		t.Errorf("a failed fetch must not be reported as an Error:\n%s", got)
	}
	if !strings.Contains(got, "mxcli setup mxcli --output ./mxcli") {
		t.Errorf("stderr lacks the manual-fix hint:\n%s", got)
	}
}

// TestFetchDevcontainerMxcli_DevBuildSkipsDownload: a locally built mxcli
// reports version 0.1.0, whose release tag (v0.1.0) does not exist. Attempting
// the download is a guaranteed 404, so it is skipped with a note instead.
func TestFetchDevcontainerMxcli_DevBuildSkipsDownload(t *testing.T) {
	for _, tc := range []struct{ name, ld, v string }{
		{"plain go build", "", "0.1.0"},
		{"make build without git", "dev", "dev"},
		{"make build, untagged checkout", "4ba1495f2", "4ba1495f2"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			withBuildVersion(t, tc.ld, tc.v)
			calls := stubDevcontainerDownload(t, func(string, string) error { return nil })

			var out, errOut bytes.Buffer
			fetchDevcontainerMxcli("/proj/mxcli", &out, &errOut)

			if *calls != 0 {
				t.Fatalf("dev build must not attempt a download, got %d call(s)", *calls)
			}
			if !strings.Contains(out.String(), "development build") {
				t.Errorf("stdout lacks the dev-build note:\n%s", out.String())
			}
			if !strings.Contains(out.String(), "mxcli setup mxcli --tag") {
				t.Errorf("stdout lacks the hint to fetch a release by tag:\n%s", out.String())
			}
			if errOut.Len() != 0 {
				t.Errorf("a skipped download is not a warning, stderr = %q", errOut.String())
			}
		})
	}
}

// TestFetchDevcontainerMxcli_SuccessUsesReleaseTag: a release build downloads
// the matching tag into the project and prints no warning.
func TestFetchDevcontainerMxcli_SuccessUsesReleaseTag(t *testing.T) {
	for _, tc := range []struct{ name, ld, v, wantTag string }{
		{"release", "v0.25.0", "v0.25.0", "v0.25.0"},
		{"nightly", "nightly-20261002-4ba1495f2", "nightly-20261002-4ba1495f2", "nightly"},
		{"git describe past a tag", "v0.24.0-888-g4ba1495f2", "v0.24.0-888-g4ba1495f2", "v0.24.0"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			withBuildVersion(t, tc.ld, tc.v)
			var gotTag, gotPath string
			calls := stubDevcontainerDownload(t, func(tag, outPath string) error {
				gotTag, gotPath = tag, outPath
				return nil
			})

			var out, errOut bytes.Buffer
			fetchDevcontainerMxcli("/proj/mxcli", &out, &errOut)

			if *calls != 1 || gotTag != tc.wantTag || gotPath != "/proj/mxcli" {
				t.Fatalf("download calls=%d tag=%q path=%q, want 1 / %q / /proj/mxcli", *calls, gotTag, gotPath, tc.wantTag)
			}
			if !strings.Contains(out.String(), "Downloading Linux mxcli ("+tc.wantTag+") for devcontainer") {
				t.Errorf("stdout lacks the download line:\n%s", out.String())
			}
			if errOut.Len() != 0 {
				t.Errorf("success must not write to stderr, got %q", errOut.String())
			}
		})
	}
}
