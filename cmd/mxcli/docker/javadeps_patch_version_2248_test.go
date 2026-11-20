// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"strings"
	"testing"
)

// ako/mxcli#2248 (mendixlabs/mxcli#1858): after a sync, vendorlib held
// bcprov-jdk18on-1.81.1.jar for a declared org.bouncycastle:bcprov-jdk18on:1.81
// and the check reported it "still missing" — Gradle resolved a newer patch
// (another dependency asks for it), and mx check was clean. A newer patch of
// the same major.minor satisfies the declaration; a different minor still
// does not (TestUnvendoredJarDependencies_WrongVersionVendored).
func TestUnvendoredJarDependencies_NewerPatchSatisfies(t *testing.T) {
	for _, tc := range []struct{ declared, vendored string }{
		{"1.81", "bcprov-jdk18on-1.81.1.jar"},
		{"1.81.0", "bcprov-jdk18on-1.81.1.jar"},
		{"1.81", "bcprov-jdk18on-1.81.jar"}, // control: the exact file
	} {
		dir := t.TempDir()
		writeVendoredJar(t, dir, tc.vendored)
		deps := []JarDependencyRef{{Group: "org.bouncycastle", Artifact: "bcprov-jdk18on", Version: tc.declared}}
		if got := UnvendoredJarDependencies(dir, deps); len(got) != 0 {
			t.Errorf("declared %s with %s vendored: reported missing %v", tc.declared, tc.vendored, got)
		}
	}
	// Control: an older patch, or another minor, is not the declared jar.
	for _, vendored := range []string{"bcprov-jdk18on-1.80.jar", "bcprov-jdk18on-1.82.jar", "bcprov-jdk18on-1.81-beta.jar"} {
		dir := t.TempDir()
		writeVendoredJar(t, dir, vendored)
		deps := []JarDependencyRef{{Group: "org.bouncycastle", Artifact: "bcprov-jdk18on", Version: "1.81.1"}}
		if got := UnvendoredJarDependencies(dir, deps); len(got) != 1 {
			t.Errorf("declared 1.81.1 with %s vendored: got %v, want it missing", vendored, got)
		}
	}
}

// A dependency that is still missing names the version vendorlib does hold,
// so the cause is visible.
func TestDescribeUnvendored_NamesThePresentJar(t *testing.T) {
	dir := t.TempDir()
	writeVendoredJar(t, dir, "commons-lang3-3.14.0.jar")
	got := DescribeUnvendored(dir, []string{"org.apache.commons:commons-lang3:3.12.0", "org.duckdb:duckdb_jdbc:1.5.5.1"})
	if len(got) != 2 || !strings.Contains(got[0], "commons-lang3-3.14.0.jar") || got[1] != "org.duckdb:duckdb_jdbc:1.5.5.1" {
		t.Errorf("got %v", got)
	}
}
