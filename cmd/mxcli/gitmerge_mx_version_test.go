// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// stubMergeMx makes the mx lookup return mxPath, reported as Mendix version
// (the project fixtures are 11.13.0), and restores the real lookup afterwards.
func stubMergeMx(t *testing.T, mxPath, version string) *[]string {
	t.Helper()
	origResolve, origVersion := resolveMxForMerge, mergeMxVersion
	t.Cleanup(func() { resolveMxForMerge, mergeMxVersion = origResolve, origVersion })
	var asked []string
	resolveMxForMerge = func(mxbuildPath, preferred string) (string, error) {
		asked = append(asked, preferred)
		return mxPath, nil
	}
	mergeMxVersion = func(string) string { return version }
	return &asked
}

func assertMergeAborted(t *testing.T, dir, before string) {
	t.Helper()
	if got := commitSubject(t, dir); got != before {
		t.Errorf("HEAD moved despite the refusal: %q", got)
	}
	if _, err := gitOut(dir, "rev-parse", "--verify", "--quiet", "MERGE_HEAD"); err == nil {
		t.Error("MERGE_HEAD still present: the refusal must abort the merge")
	}
	if uu := gitOutT(t, dir, "diff", "--name-only", "--diff-filter=U"); strings.TrimSpace(uu) != "" {
		t.Errorf("working tree not restored:\n%s", uu)
	}
}

// With only another Mendix version's mx resolvable (a cached download, an mx
// on PATH), the model merge must not run that engine: it would write this
// project's model with another version's merge. Refuse, abort, and say how to
// get the right one.
func TestGitMerge_RefusesAnotherVersionsMx(t *testing.T) {
	dir, mprPath := modelConflictRepo(t)
	doctorEnv(t, dir)
	before := commitSubject(t, dir)
	asked := stubMergeMx(t, "/cache/mxbuild/11.6.3/modeler/mx", "11.6.3")

	var out, stderr bytes.Buffer
	err := mergeRun(mprPath, "side", "", "", "", false, mxRunnersFor("", &stderr), &out)
	if err == nil {
		t.Fatalf("want a refusal, the merge went through:\n%s", out.String())
	}
	for _, want := range []string{
		"is Mendix 11.6.3, but the project is Mendix 11.13.0",
		"mxcli setup mxbuild --version 11.13.0",
		"--mxbuild-path",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("refusal lacks %q:\n%v", want, err)
		}
	}
	if len(*asked) == 0 || (*asked)[0] != "11.13.0" {
		t.Errorf("mx was not resolved for the project's version: %v", *asked)
	}
	assertMergeAborted(t, dir, before)
}

// An mx whose version cannot be told is refused too: nothing shows it is the
// project's release.
func TestGitMerge_RefusesMxOfUnknownVersion(t *testing.T) {
	dir, mprPath := modelConflictRepo(t)
	doctorEnv(t, dir)
	before := commitSubject(t, dir)
	stubMergeMx(t, "/usr/local/bin/mx", "")

	var out, stderr bytes.Buffer
	err := mergeRun(mprPath, "side", "", "", "", false, mxRunnersFor("", &stderr), &out)
	if err == nil || !strings.Contains(err.Error(), "cannot tell which Mendix version /usr/local/bin/mx is") {
		t.Fatalf("want the unknown-version refusal, got %v\n%s", err, out.String())
	}
	assertMergeAborted(t, dir, before)
}

// --mxbuild-path is the deliberate choice: a mismatch is warned about once and
// the engine runs. The fake mx exits 129 so the run ends in the existing
// engine-failure abort, which proves it was invoked.
func TestGitMerge_MxbuildPathOverridesVersionCheck(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake mx is a shell script")
	}
	dir, mprPath := modelConflictRepo(t)
	doctorEnv(t, dir)
	before := commitSubject(t, dir)
	mx := filepath.Join(t.TempDir(), "mx")
	if err := os.WriteFile(mx, []byte("#!/bin/sh\necho crashed\nexit 129\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	stubMergeMx(t, mx, "11.6.3")

	var out, stderr bytes.Buffer
	err := mergeRun(mprPath, "side", "", "", "", false, mxRunnersFor(filepath.Dir(mx), &stderr), &out)
	if err == nil || !strings.Contains(err.Error(), "exit 129") {
		t.Fatalf("want the fake engine to have run (exit 129), got %v\n%s", err, out.String())
	}
	warn := stderr.String()
	if !strings.Contains(warn, "is Mendix 11.6.3, but the project is Mendix 11.13.0") ||
		!strings.Contains(warn, "going on because --mxbuild-path was given") {
		t.Errorf("want the mismatch warned about, stderr:\n%s", warn)
	}
	if n := strings.Count(warn, "warning:"); n != 1 {
		t.Errorf("want one warning, got %d:\n%s", n, warn)
	}
	assertMergeAborted(t, dir, before)
}

func TestMergeMxProblem(t *testing.T) {
	orig := mergeMxVersion
	t.Cleanup(func() { mergeMxVersion = orig })
	for _, tc := range []struct {
		name, project, mx, want string
	}{
		{"same release", "11.13.0", "11.13.0", ""},
		{"build number ignored", "11.13.0.81004", "11.13.0", ""},
		{"other patch", "11.13.0", "11.13.1", "is Mendix 11.13.1, but the project is Mendix 11.13.0"},
		{"unknown mx", "11.13.0", "", "cannot tell which Mendix version"},
		{"unknown project is not checked", "", "11.6.3", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mergeMxVersion = func(string) string { return tc.mx }
			got := mergeMxProblem("/x/mx", tc.project)
			if tc.want == "" && got != "" || tc.want != "" && !strings.Contains(got, tc.want) {
				t.Errorf("mergeMxProblem(%q, mx %q) = %q, want %q", tc.project, tc.mx, got, tc.want)
			}
		})
	}
}
