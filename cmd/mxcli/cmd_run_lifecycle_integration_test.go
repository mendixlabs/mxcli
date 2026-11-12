//go:build integration && linux

// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mendixlabs/mxcli/cmd/mxcli/docker"
	modelsdkbackend "github.com/mendixlabs/mxcli/mdl/backend/modelsdk"
)

// TestRunLifecycle_DetachStatusExecWaitStop drives the background-service
// commands the way an agent does, against a real mxbuild and runtime:
//
//	run --local --watch --detach   -> returns once the app serves
//	run status                     -> one line, exit 0
//	exec <page change> && run wait -> "applied: build #2 via reload"
//	run stop                       -> nothing left in the run's session, ports free
//
// and the boot race: a change made while the app is still booting must be
// built once it is up, not folded into the boot's baseline and lost.
//
//	MXCLI_RUNLIFE_VERSION=11.13.0 go test -tags integration ./cmd/mxcli -run RunLifecycle -v
//
// Uses the built-in HSQLDB database, so it needs no PostgreSQL. Skips when no
// mxbuild is cached. ResolveMxForVersion falls back to any cached version, so the
// nightly runs this against each matrix leg's own version — which is how it
// found that --watch could not start on 10.24 / 11.6.
//
// On the Mendix versions that bundle the client themselves (11.14+ here), a
// build in an mxbuild --serve process deletes the client source it wrote, so the
// next build in that process failed (11.14) or bundled no pages (11.15+), and
// mxcli answered every change with a serve restart and a cold build. Since
// ako/mxcli#2150 it keeps that source between builds (client_source_keep.go), so
// a page change is a reload on every version again, and step 4 asserts the run
// log shows no serve restart. Every fallback that restarts the serve says
// "restarting mxbuild --serve" (serve_recovery.go): a failure naming it means the
// keep did not hold what the build needed.
func TestRunLifecycle_DetachStatusExecWaitStop(t *testing.T) {
	version := os.Getenv("MXCLI_RUNLIFE_VERSION")
	if version == "" {
		version = "11.13.0"
	}
	mxPath, err := docker.ResolveMxForVersion("", version)
	if err != nil {
		t.Skipf("mxbuild %s is not cached: %v", version, err)
	}
	bin := os.Getenv("MXCLI_BIN")
	if bin == "" {
		bin = filepath.Join(t.TempDir(), "mxcli")
		build := exec.Command("go", "build", "-o", bin, ".")
		if out, err := build.CombinedOutput(); err != nil {
			t.Fatalf("go build: %v\n%s", err, out)
		}
	}
	work, err := os.MkdirTemp("", "mxrl")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(work) }) // registered first, so it runs last
	create := exec.Command(mxPath, "create-project")
	create.Dir = work
	if out, err := create.CombinedOutput(); err != nil {
		t.Skipf("mx create-project: %v\n%s", err, out)
	}
	mpr := filepath.Join(work, "App.mpr")

	const appPort, adminPort, servePort = 8287, 8297, 6743
	for _, p := range []int{appPort, adminPort, servePort} {
		if portAnswers(p) {
			t.Skipf("port %d is in use", p)
		}
	}
	mxcli := func(args ...string) (string, int) {
		cmd := exec.Command(bin, args...)
		cmd.Dir = work
		var out bytes.Buffer
		cmd.Stdout = &out
		cmd.Stderr = &bytes.Buffer{} // the banner and "Using project"
		err := cmd.Run()
		code := 0
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			code = ee.ExitCode()
		} else if err != nil {
			t.Fatalf("mxcli %v: %v", args, err)
		}
		return out.String(), code
	}
	runArgs := []string{"run", "--local", "--watch", "--detach", "--db-type", "hsqldb",
		"--app-port", fmt.Sprint(appPort), "--admin-port", fmt.Sprint(adminPort),
		"--serve-port", fmt.Sprint(servePort), "-p", mpr}
	t.Cleanup(func() { _ = exec.Command(bin, "run", "stop", "-p", mpr).Run() })

	page := func(name, text string) string {
		f := filepath.Join(work, name+".mdl")
		mdl := fmt.Sprintf("mdl 1;\ncreate or modify page MyFirstModule.%s (title: '%s', layout: Atlas_Core.Atlas_Default) {\n  dynamictext t1 (content: '%s')\n};\n", name, name, text)
		if err := os.WriteFile(f, []byte(mdl), 0o644); err != nil {
			t.Fatal(err)
		}
		return f
	}

	// wantBuild2 checks a `run wait` that should report build #2 applied and,
	// unless orDirect, that mxcli did not restart mxbuild --serve to get there: on
	// 11.14+ that restart is the fallback for client source the keep did not hold,
	// a cold build per change instead of an incremental one. orDirect drops that
	// requirement, for a step whose claim is only that build #2 ran.
	wantBuild2 := func(step, out string, code int, orDirect bool) {
		t.Helper()
		if code != 0 || !strings.HasPrefix(out, "applied: build #2") {
			t.Fatalf("%s: exit %d, %q\n%s", step, code, out, tailOf(docker.RunLogPath(mpr)))
		}
		if !orDirect {
			log, _ := os.ReadFile(docker.RunLogPath(mpr))
			if strings.Contains(string(log), "restarting mxbuild --serve") {
				t.Fatalf("%s: build #2 applied only after mxcli restarted mxbuild --serve; the client source kept "+
					"between builds (client_source_keep.go, ako/mxcli#2150) should let it build incrementally\n%s",
					step, tailOf(docker.RunLogPath(mpr)))
			}
		}
	}

	// 1. detach: one line, exit 0, and the app answers.
	out, code := mxcli(runArgs...)
	if code != 0 || !strings.HasPrefix(out, "running: http://127.0.0.1:8287/ (pid ") || strings.Count(out, "\n") != 1 {
		t.Fatalf("detach: exit %d, output %q\n%s", code, out, tailOf(docker.RunLogPath(mpr)))
	}
	st, _ := docker.ReadRunState(mpr)
	if st == nil || !st.Detached {
		t.Fatalf("state after detach: %+v", st)
	}
	runPID, started := st.PID, st.Started

	// 2. a second detach is refused, and names the way out.
	if _, code := mxcli(runArgs...); code == 0 {
		t.Fatal("second detach for the same project was not refused")
	}

	// 3. status.
	out, code = mxcli("run", "status", "-p", mpr)
	if code != 0 || !strings.HasPrefix(out, "running: ") {
		t.Fatalf("status: exit %d, %q", code, out)
	}

	// 4. exec a page change, then wait — the one-call chain.
	if out, code := mxcli("exec", page("Hello", "one"), "-p", mpr); code != 0 {
		t.Fatalf("exec: exit %d, %s", code, out)
	}
	out, code = mxcli("run", "wait", "-p", mpr, "--timeout", "3m")
	wantBuild2("wait", out, code, false)
	if !strings.HasPrefix(out, "applied: build #2 via reload") {
		t.Fatalf("wait: applied, but not via reload: %q", out)
	}
	// Waiting again with nothing changed reports the same build at once.
	out, code = mxcli("run", "wait", "-p", mpr, "--timeout", "5s")
	wantBuild2("second wait", out, code, false)

	// 5. stop: every process of the run is gone, and the ports are free.
	out, code = mxcli("run", "stop", "-p", mpr)
	if code != 0 || !strings.HasPrefix(out, "stopped: pid ") {
		t.Fatalf("stop: exit %d, %q", code, out)
	}
	if left := docker.SessionMembers(runPID, started); len(left) != 0 {
		t.Fatalf("processes left after stop: %v", left)
	}
	for _, p := range []int{appPort, adminPort, servePort} {
		if portAnswers(p) {
			t.Errorf("port %d still answers after stop", p)
		}
	}
	if out, code := mxcli("run", "status", "-p", mpr); code != 3 || !strings.HasPrefix(out, "stopped") {
		t.Errorf("status after stop: exit %d, %q", code, out)
	}
	if out, code := mxcli("run", "stop", "-p", mpr); code != 0 {
		t.Errorf("second stop must be a no-op: exit %d, %q", code, out)
	}

	// 6. the boot race: change the model while the app is still booting.
	bg := exec.Command(bin, runArgs...)
	bg.Dir = work
	if err := bg.Start(); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Minute)
	for {
		st, _ := docker.ReadRunState(mpr)
		if st != nil && st.PID != runPID && st.Phase == docker.PhaseStarting && st.Building {
			break // the boot build's source time is recorded
		}
		if time.Now().After(deadline) {
			t.Fatal("never saw the boot build start")
		}
		time.Sleep(50 * time.Millisecond)
	}
	if out, code := mxcli("exec", page("DuringBoot", "two"), "-p", mpr); code != 0 {
		t.Fatalf("exec during boot: exit %d, %s", code, out)
	}
	if err := bg.Wait(); err != nil {
		t.Fatalf("detach (boot race): %v\n%s", err, tailOf(docker.RunLogPath(mpr)))
	}
	// What this step claims is that the change is not lost: build #2 applies;
	// "nothing to apply" never passes. A serve restart is not ruled out here: the
	// change lands while the boot build runs, and how much of it that build had
	// already read, and so what the keep holds, is timing the test does not control.
	out, code = mxcli("run", "wait", "-p", mpr, "--timeout", "2m")
	wantBuild2("a change made during the boot", out, code, true)
	if out, code := mxcli("run", "stop", "-p", mpr); code != 0 {
		t.Fatalf("final stop: exit %d, %q", code, out)
	}
}

// projectAtLeast reports whether the project at mpr is Mendix major.minor or newer.
func projectAtLeast(t *testing.T, mpr string, major, minor int) bool {
	t.Helper()
	b := modelsdkbackend.New()
	if err := b.ConnectReadOnly(mpr); err != nil {
		t.Fatalf("reading project version: %v", err)
	}
	defer b.Disconnect()
	pv := b.ProjectVersion()
	if pv == nil {
		t.Fatal("project has no version")
	}
	return pv.IsAtLeast(major, minor)
}

func portAnswers(port int) bool {
	c, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 300*time.Millisecond)
	if err != nil {
		return false
	}
	c.Close()
	return true
}

func tailOf(path string) string {
	return "--- " + path + " ---\n" + strings.Join(tailLines(path, 30), "\n")
}
