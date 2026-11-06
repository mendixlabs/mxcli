// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/mendixlabs/mxcli/cmd/mxcli/docker"
)

func TestDetachArgs_DropsDetachAndPinsTheProject(t *testing.T) {
	for _, in := range [][]string{
		{"run", "--local", "--watch", "--detach", "-p", "app.mpr"},
		{"run", "--local", "--watch", "--detach=true", "--project", "app.mpr"},
		{"run", "--local", "--watch", "-p=app.mpr", "--detach"},
		{"run", "--local", "--watch", "--project=app.mpr"},
		{"run", "--local", "--watch", "-papp.mpr"},
	} {
		got := detachArgs(in, "/abs/app.mpr")
		want := []string{"run", "--local", "--watch", "-p", "/abs/app.mpr"}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("detachArgs(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestDetachArgs_PreservesConsumedFlagValues(t *testing.T) {
	for _, flag := range []string{"--screenshot-password", "--db-password", "--engine"} {
		for _, value := range []string{"--detach", "--detach=false", "--project", "--project=other.mpr", "-p", "-pother.mpr", "--"} {
			in := []string{"run", "--local", flag, value, "--db-password-stdin", "--detach", "-p", "app.mpr"}
			want := []string{"run", "--local", flag, value, "--db-password-stdin", "-p", "/abs/app.mpr"}
			if got := detachArgs(in, "/abs/app.mpr"); !reflect.DeepEqual(got, want) {
				t.Errorf("argument recording changed the value consumed by %s", flag)
			}
		}
	}
	in := []string{"run", "--local", "--detach", "--", "--detach", "--project=literal"}
	want := []string{"run", "--local", "-p", "/abs/app.mpr", "--", "--detach", "--project=literal"}
	if got := detachArgs(in, "/abs/app.mpr"); !reflect.DeepEqual(got, want) {
		t.Error("argument recording reinterpreted positional arguments after --")
	}
}

func alwaysAlive(int) bool { return true }
func neverAlive(int) bool  { return false }
func noLeftovers(*docker.RunState) int {
	return 0
}

func TestStatusLine(t *testing.T) {
	now := time.Unix(10_000, 0)
	src := time.Unix(9_000, 0)
	running := func(mut func(*docker.RunState)) *docker.RunState {
		st := &docker.RunState{PID: 77, Phase: docker.PhaseRunning, Watch: true, URL: "http://127.0.0.1:8080/",
			Started: now.Add(-2 * time.Minute), Ready: now.Add(-time.Minute), Log: "/p/.mxcli/run.log",
			Last: &docker.BuildOutcome{Gen: 1, OK: true, Action: "boot", Source: src, At: now.Add(-time.Minute)}}
		if mut != nil {
			mut(st)
		}
		return st
	}
	cases := []struct {
		name      string
		st        *docker.RunState
		alive     func(int) bool
		source    time.Time
		leftovers func(*docker.RunState) int
		wantCode  int
		want      []string
	}{
		{"nothing recorded", nil, neverAlive, src, noLeftovers, runExitNotRunning, []string{"stopped: no run recorded"}},
		{"running, boot only", running(nil), alwaysAlive, src, noLeftovers, runExitOK,
			[]string{"running: http://127.0.0.1:8080/ (pid 77, up 1m0s, watch, build #1 (boot))"}},
		{"stale: process gone", running(nil), neverAlive, src, func(*docker.RunState) int { return 2 }, runExitNotRunning,
			[]string{"stopped: pid 77 exited without cleaning up", "2 leftover process(es)", "mxcli run stop"}},
		{"starting", running(func(s *docker.RunState) { s.Phase = docker.PhaseStarting; s.Last = nil }), alwaysAlive, src, noLeftovers, runExitStarting,
			[]string{"starting: pid 77, 2m0s so far"}},
		{"boot failed", running(func(s *docker.RunState) { s.Phase = docker.PhaseFailed; s.Error = "initial build failed: X\nmore" }), neverAlive, src, noLeftovers, runExitNotRunning,
			[]string{"stopped: boot failed — initial build failed: X ("}},
		{"building", running(func(s *docker.RunState) { s.Building = true; s.Gen = 2 }), alwaysAlive, src.Add(time.Second), noLeftovers, runExitOK,
			[]string{"building #2"}},
		{"change pending", running(nil), alwaysAlive, src.Add(time.Second), noLeftovers, runExitOK, []string{"change pending"}},
		{"not watching, change on disk", running(func(s *docker.RunState) { s.Watch = false }), alwaysAlive, src.Add(time.Second), noLeftovers, runExitOK,
			[]string{"no watch", "NOT applied", "mxcli run restart"}},
		{"last build failed", running(func(s *docker.RunState) {
			s.Last = &docker.BuildOutcome{Gen: 3, Action: "build", Message: "Build failed", Source: src, At: now.Add(-5 * time.Second)}
		}), alwaysAlive, src, noLeftovers, runExitOK, []string{"build #3 FAILED (build) 5s ago: Build failed", "mxcli run wait"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			line, code := statusLine(c.st, statusProbes{alive: c.alive, leftovers: c.leftovers}, c.source, now)
			if code != c.wantCode {
				t.Errorf("exit %d, want %d (%s)", code, c.wantCode, line)
			}
			if strings.Contains(line, "\n") {
				t.Errorf("status must be one line: %q", line)
			}
			for _, w := range c.want {
				if !strings.Contains(line, w) {
					t.Errorf("%q does not contain %q", line, w)
				}
			}
		})
	}
}

// scratchProject is an empty project directory with a model file whose mtime
// is the source time `run wait` reads.
func scratchProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	mpr := filepath.Join(dir, "App.mpr")
	if err := os.WriteFile(mpr, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	return mpr
}

func fastPolling(t *testing.T) {
	t.Helper()
	old := statePollInterval
	statePollInterval = 10 * time.Millisecond
	t.Cleanup(func() { statePollInterval = old })
}

// liveState is a running watch-mode state owned by this (live) test process.
func liveState(mpr string, last *docker.BuildOutcome) *docker.RunState {
	return &docker.RunState{Project: mpr, PID: os.Getpid(), Phase: docker.PhaseRunning, Watch: true,
		URL: "http://127.0.0.1:8080/", Gen: last.Gen, Last: last}
}

// The race `run wait` must not lose: the change is applied by the watch loop
// BEFORE wait is called. It must report that build at once, not wait for a
// next one that never comes.
func TestRunWait_ChangeAppliedBeforeWaitStartsIsReportedAtOnce(t *testing.T) {
	fastPolling(t)
	mpr := scratchProject(t)
	src := docker.SourceMTime(mpr)
	if err := docker.WriteRunState(mpr, liveState(mpr, &docker.BuildOutcome{Gen: 2, OK: true, Action: "reload", DurationMs: 1200, Source: src})); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	start := time.Now()
	code := runWait(mpr, 2*time.Second, 0, &out)
	if code != runExitOK || out.String() != "applied: build #2 via reload in 1.2s\n" {
		t.Fatalf("exit %d, output %q", code, out.String())
	}
	if time.Since(start) > time.Second {
		t.Errorf("took %s: an applied change must not wait", time.Since(start))
	}
}

// A change written after the last build is NOT covered by it: wait blocks
// until the loop publishes a build of that source, then reports it.
func TestRunWait_BlocksUntilTheChangeIsBuiltAndReportsFailure(t *testing.T) {
	fastPolling(t)
	mpr := scratchProject(t)
	old := docker.SourceMTime(mpr).Add(-time.Minute)
	if err := docker.WriteRunState(mpr, liveState(mpr, &docker.BuildOutcome{Gen: 2, OK: true, Action: "reload", Source: old})); err != nil {
		t.Fatal(err)
	}
	go func() {
		time.Sleep(150 * time.Millisecond)
		st := liveState(mpr, &docker.BuildOutcome{Gen: 3, OK: false, Action: "build", Message: "Build failed",
			Errors: []string{"[CE0109] No attribute selected — at M / Page 'P'"}, Source: docker.SourceMTime(mpr)})
		_ = docker.WriteRunState(mpr, st)
	}()
	var out bytes.Buffer
	code := runWait(mpr, 5*time.Second, 0, &out)
	if code != runExitFailed {
		t.Fatalf("exit %d, want %d; output %q", code, runExitFailed, out.String())
	}
	if !strings.HasPrefix(out.String(), "failed: build #3 (build): Build failed\n  [CE0109]") {
		t.Errorf("output %q", out.String())
	}
}

func TestRunWait_TimeoutAndNotRunningHaveTheirOwnExitCodes(t *testing.T) {
	fastPolling(t)
	mpr := scratchProject(t)
	old := docker.SourceMTime(mpr).Add(-time.Minute)
	st := liveState(mpr, &docker.BuildOutcome{Gen: 2, OK: true, Action: "reload", Source: old})
	st.Building = true
	_ = docker.WriteRunState(mpr, st)
	var out bytes.Buffer
	if code := runWait(mpr, 100*time.Millisecond, 0, &out); code != runExitTimeout || !strings.Contains(out.String(), "still in progress") {
		t.Errorf("timeout: exit %d, output %q", code, out.String())
	}

	st.PID = deadPID(t)
	_ = docker.WriteRunState(mpr, st)
	out.Reset()
	if code := runWait(mpr, time.Second, 0, &out); code != runExitNotRunning || !strings.HasPrefix(out.String(), "not running:") {
		t.Errorf("stale state: exit %d, output %q", code, out.String())
	}

	st.PID = os.Getpid()
	st.Watch, st.Building = false, false
	_ = docker.WriteRunState(mpr, st)
	out.Reset()
	if code := runWait(mpr, time.Second, 0, &out); code != runExitFailed || !strings.Contains(out.String(), "without --watch") {
		t.Errorf("not watching: exit %d, output %q", code, out.String())
	}
}

func TestRunStop_NothingRunningIsANoOp(t *testing.T) {
	mpr := scratchProject(t)
	var out bytes.Buffer
	if code := runStop(mpr, &out); code != runExitOK || out.String() != "stopped (nothing was running)\n" {
		t.Errorf("exit %d, output %q", code, out.String())
	}
	_ = docker.WriteRunState(mpr, &docker.RunState{PID: deadPID(t), Phase: docker.PhaseStopped})
	out.Reset()
	if code := runStop(mpr, &out); code != runExitOK || !strings.HasPrefix(out.String(), "stopped (was not running") {
		t.Errorf("stale: exit %d, output %q", code, out.String())
	}
}

func TestRefuseIfRunning(t *testing.T) {
	mpr := scratchProject(t)
	if err := refuseIfRunning(mpr); err != nil {
		t.Fatalf("no state: %v", err)
	}
	other := startSleeper(t)
	_ = docker.WriteRunState(mpr, &docker.RunState{PID: other, Phase: docker.PhaseRunning, URL: "http://127.0.0.1:8080/"})
	err := refuseIfRunning(mpr)
	if err == nil || !strings.Contains(err.Error(), "already running") || !strings.Contains(err.Error(), "mxcli run stop") {
		t.Fatalf("live run: %v", err)
	}
	_ = docker.WriteRunState(mpr, &docker.RunState{PID: deadPID(t), Phase: docker.PhaseRunning})
	if err := refuseIfRunning(mpr); err != nil {
		t.Errorf("a stale state must not block a new run: %v", err)
	}
}

// deadPID returns the pid of a process that has exited and been reaped.
func deadPID(t *testing.T) int {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("uses POSIX true/sleep")
	}
	cmd := exec.Command("true")
	if err := cmd.Run(); err != nil {
		t.Skipf("cannot run true: %v", err)
	}
	return cmd.Process.Pid
}

// startSleeper starts a live process that is not this one.
func startSleeper(t *testing.T) int {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("uses POSIX true/sleep")
	}
	cmd := exec.Command("sleep", "30")
	if err := cmd.Start(); err != nil {
		t.Skipf("cannot start sleep: %v", err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
	return cmd.Process.Pid
}
