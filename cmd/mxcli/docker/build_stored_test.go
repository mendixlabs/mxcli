// SPDX-License-Identifier: Apache-2.0

// ako/mxcli#1794: `docker build` (and `docker run` / `docker reload`, which
// build through it) ran `mx update-widgets` on its temporary copy before its
// pre-build check and MxBuild. A project whose stored widgets were stale
// therefore built — from a widget-normalised copy — while `docker check`,
// `run --local` and Studio Pro all reported the CE0463s: the deployment was not
// built from what is committed. The build now checks the project as stored,
// as `docker check` does since #646, and labels each CE0463; the old behaviour
// is --update-widgets.
package docker

import (
	"bytes"
	"io"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildOnCopy_StaleStoredWidgetsFailTheBuild(t *testing.T) {
	mprPath := v1Fixture(t)
	calls := stubStaleWidgets(t, []string{"Page 'Home' / widget 'grid1'"}, nil, nil)
	_, sawPath := stubMxBuild(t)
	t.Setenv("TMPDIR", t.TempDir())
	var out bytes.Buffer
	err := buildOnCopy(buildSteps{ProjectPath: mprPath, MxPath: "mx", OutputDir: filepath.Join(t.TempDir(), "out")}, &out, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "project has errors") {
		t.Fatalf("err = %v, want the stored project's check to fail the build\n%s", err, out.String())
	}
	if *sawPath != "" {
		t.Error("MxBuild ran on a project whose stored widgets are stale")
	}
	// The check ran on the copy as stored; update-widgets only for the
	// re-check that labels the CE0463.
	if got := strings.Join(*calls, ","); !strings.HasPrefix(got, "check,") {
		t.Errorf("first call = %q, want the stored project checked first", got)
	}
	s := out.String() + err.Error()
	for _, want := range []string{"mxcli fix widgets", "--update-widgets"} {
		if !strings.Contains(s, want) {
			t.Errorf("the failure does not name %q:\n%s", want, s)
		}
	}
}

// --update-widgets is the old behaviour: normalise the copy, check it and
// build it, saying so.
func TestBuildOnCopy_UpdateWidgetsBuildsTheNormalisedCopy(t *testing.T) {
	mprPath := v1Fixture(t)
	calls := stubStaleWidgets(t, []string{"Page 'Home' / widget 'grid1'"}, nil, nil)
	_, sawPath := stubMxBuild(t)
	t.Setenv("TMPDIR", t.TempDir())
	var out bytes.Buffer
	if err := buildOnCopy(buildSteps{ProjectPath: mprPath, MxPath: "mx", OutputDir: filepath.Join(t.TempDir(), "out"), UpdateWidgetsFirst: true}, &out, io.Discard); err != nil {
		t.Fatalf("buildOnCopy --update-widgets: %v\n%s", err, out.String())
	}
	if *sawPath == "" {
		t.Error("MxBuild did not run")
	}
	if got := strings.Join(*calls, ","); got != "update-widgets,check" {
		t.Errorf("calls = %q, want update-widgets then check", got)
	}
	if !strings.Contains(out.String(), "mxcli fix widgets") {
		t.Errorf("the build does not say the project keeps its stale widgets:\n%s", out.String())
	}
}

// Control: a clean project builds, with no update-widgets anywhere.
func TestBuildOnCopy_CleanProjectBuildsAsStored(t *testing.T) {
	mprPath := v1Fixture(t)
	calls := stubStaleWidgets(t, nil, nil, nil)
	_, sawPath := stubMxBuild(t)
	t.Setenv("TMPDIR", t.TempDir())
	var out bytes.Buffer
	if err := buildOnCopy(buildSteps{ProjectPath: mprPath, MxPath: "mx", OutputDir: filepath.Join(t.TempDir(), "out")}, &out, io.Discard); err != nil {
		t.Fatalf("buildOnCopy: %v\n%s", err, out.String())
	}
	if *sawPath == "" {
		t.Error("MxBuild did not run")
	}
	if got := strings.Join(*calls, ","); got != "check" {
		t.Errorf("calls = %q, want only the check", got)
	}
}
