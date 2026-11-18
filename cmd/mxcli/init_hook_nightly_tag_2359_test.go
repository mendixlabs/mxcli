// SPDX-License-Identifier: Apache-2.0

package main

import (
	"strings"
	"testing"
)

// ako/mxcli#2359: with a nightly stamp, the bootstrap script told the user to
// "Set MXCLI_TAG to nightly-20261117-b7dc2f863 or newer", which downloaded a
// 404: only the rolling `nightly` tag existed. Dated nightlies are now
// published, but only the most recent ones are kept, so the advice says so
// and names `nightly` as the way to the current build.
func TestBootstrapScript_NightlyStampAdviceIsDownloadable_Issue2359(t *testing.T) {
	requireSh(t)
	e := newBootEnv(t, "nightly-21000101-deadbee") // newer than the fake download
	out := e.run(t)
	if !strings.Contains(out, "is still older than this project's tooling") {
		t.Fatalf("no staleness warning; output:\n%s", out)
	}
	for _, want := range []string{"MXCLI_TAG=nightly-21000101-deadbee", "MXCLI_TAG=nightly", "most recent"} {
		if !strings.Contains(out, want) {
			t.Errorf("advice lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "or newer (or 'nightly')") {
		t.Errorf("still the release-version advice for a nightly stamp:\n%s", out)
	}
}

// CONTROL: a release stamp keeps the release advice. (A nightly stand-in is
// not provably older than a release stamp, so this reads the script itself.)
func TestBootstrapScript_ReleaseStampAdviceUnchanged_Issue2359(t *testing.T) {
	if !strings.Contains(renderBootstrapScript("App.mpr"), "or newer (or 'nightly') and re-run this script.") {
		t.Error("the release-stamp advice is gone from the bootstrap script")
	}
}
