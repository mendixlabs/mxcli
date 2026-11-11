// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"io"
	"strings"
	"testing"
)

// ako/mxcli#2064: under `run --local --watch` an INCREMENTAL build in a serve
// process that had already built failed with CE0108 "Variable 'Latest' is
// defined but not in scope at this location" on an mxcli-written microflow,
// while `docker check` reported 0 errors and a full build after a serve
// restart applied the same model. The loop stayed on "build #N FAILED" until
// something forced a full build.

func buildCE(code, msg string) *BuildResult {
	return &BuildResult{Status: "Failure", Message: "The project cannot be deployed, because it contains errors.",
		Problems: BuildProblems{Problems: []BuildProblem{{Severity: "Error", ErrorCode: code, Message: msg,
			Locations: []BuildLocation{{Document: "Microflow 'TMP_ReproAct'"}}}}}}
}

const ce0108Msg = "Variable 'Latest' is defined but not in scope at this location."

func TestServeRecovery_IncrementalCE0108IsCheckedOnAFreshProcess(t *testing.T) {
	f := &scriptedServe{build: func(proc, _ int) (*BuildResult, error) {
		if proc == 0 {
			return buildCE("CE0108", ce0108Msg), nil // the stale incremental answer
		}
		return buildOK(true), nil // a full build of the same model
	}}
	var out strings.Builder
	r := &serveRecovery{serve: f, out: &out}
	res, err := r.Build(BuildRequest{ProjectFilePath: "/p/App.mpr"})
	if err != nil {
		t.Fatal(err)
	}
	if !res.OK() {
		t.Fatalf("the full build passed, but the loop reports the incremental build's CE0108: %+v", res)
	}
	if f.restarts != 1 {
		t.Errorf("restarts = %d, want 1", f.restarts)
	}
	if !res.ColdRestart {
		t.Error("the answer is a fresh process's first build; it must be marked ColdRestart")
	}
	if got := out.String(); !strings.Contains(got, "CE0108") || !strings.Contains(got, "full build") {
		t.Errorf("the recheck and its verdict must be reported, got:\n%s", got)
	}
}

// A genuine CE0108 costs one recheck, not one per save while it persists.
func TestServeRecovery_ConfirmedCE0108IsNotRecheckedUntilABuildPasses(t *testing.T) {
	pass := false
	f := &scriptedServe{build: func(_, _ int) (*BuildResult, error) {
		if pass {
			return buildOK(false), nil
		}
		return buildCE("CE0108", ce0108Msg), nil
	}}
	r := &serveRecovery{serve: f, out: io.Discard}
	for i := 0; i < 3; i++ {
		res, err := r.Build(BuildRequest{})
		if err != nil || res.OK() {
			t.Fatalf("build %d: %+v %v, want the genuine CE0108", i, res, err)
		}
	}
	if f.restarts != 1 {
		t.Fatalf("restarts = %d after three saves of one genuine error, want 1", f.restarts)
	}
	pass = true
	if res, _ := r.Build(BuildRequest{}); !res.OK() {
		t.Fatal("a fixed model must build")
	}
	pass = false
	if _, _ = r.Build(BuildRequest{}); f.restarts != 2 {
		t.Fatalf("restarts = %d, want 2: after a passing build a new CE0108 is rechecked again", f.restarts)
	}
}

// Control: an error mxbuild --serve has not been seen to invent is reported as
// it is, at no extra cost.
func TestServeRecovery_OtherModelErrorsAreNotRechecked(t *testing.T) {
	f := &scriptedServe{build: func(_, _ int) (*BuildResult, error) {
		return buildCE("CE0109", "Attribute 'X' does not exist."), nil
	}}
	r := &serveRecovery{serve: f, out: io.Discard}
	if res, _ := r.Build(BuildRequest{}); res.OK() || f.restarts != 0 {
		t.Fatalf("res=%+v restarts=%d, want the failure as it is and no restart", res, f.restarts)
	}
}

// A build that already ran on a fresh process is a full build: its CE0108 is
// the model's.
func TestServeRecovery_FreshProcessCE0108IsNotRechecked(t *testing.T) {
	f := &scriptedServe{build: func(_, _ int) (*BuildResult, error) { return buildCE("CE0108", ce0108Msg), nil }}
	r := &serveRecovery{serve: f, out: io.Discard, restartPending: true}
	if res, _ := r.Build(BuildRequest{}); res.OK() || f.restarts != 1 {
		t.Fatalf("res=%+v restarts=%d, want the failure after the one pending restart", res, f.restarts)
	}
}
