//go:build integration && linux

// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/mendixlabs/mxcli/cmd/mxcli/docker"
)

// TestRunStop_RestoresTestEndpointOfAKilledRun: `run --test-endpoint` puts the
// MxTest module, its after-startup microflow and themesource/mxtest/ into the
// project and removes them on its own way out. On Windows `run stop` cannot ask
// for that way out: it ends the run with `taskkill /F`, the teardown never runs,
// and the project is left carrying the endpoint, which the next build reports as
// CE1613 once MxTest is dropped (ako/mxcli#1917, mendixlabs/mxcli#1723). A
// SIGKILL here is the same stop, so the test kills the run outright and asks
// `run stop` to leave the project as it was before the run.
//
//	MXCLI_RUNLIFE_VERSION=11.12.6 go test -tags integration ./cmd/mxcli -run RestoresTestEndpoint -v
//
// The control is the plain `run stop` of a live run (SIGTERM), which already
// restored the project before this fix; the killed run did not.
func TestRunStop_RestoresTestEndpointOfAKilledRun(t *testing.T) {
	version := os.Getenv("MXCLI_RUNLIFE_VERSION")
	if version == "" {
		version = "11.12.6"
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
	work, err := os.MkdirTemp("", "mxte")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(work) })
	create := exec.Command(mxPath, "create-project")
	create.Dir = work
	if out, err := create.CombinedOutput(); err != nil {
		t.Skipf("mx create-project: %v\n%s", err, out)
	}
	mpr := filepath.Join(work, "App.mpr")

	const appPort, adminPort, servePort = 8387, 8397, 6843
	for _, p := range []int{appPort, adminPort, servePort} {
		if portAnswers(p) {
			t.Skipf("port %d is in use", p)
		}
	}
	mxcli := func(args ...string) string {
		cmd := exec.Command(bin, args...)
		cmd.Dir = work
		var out bytes.Buffer
		cmd.Stdout, cmd.Stderr = &out, &out
		if err := cmd.Run(); err != nil {
			t.Fatalf("mxcli %v: %v\n%s", args, err, out.String())
		}
		return out.String()
	}
	t.Cleanup(func() { _ = exec.Command(bin, "run", "stop", "-p", mpr).Run() })

	before := projectFingerprint(t, work)

	mxcli("run", "--local", "--test-endpoint", "--detach", "--db-type", "hsqldb",
		"--app-port", fmt.Sprint(appPort), "--admin-port", fmt.Sprint(adminPort),
		"--serve-port", fmt.Sprint(servePort), "-p", mpr)
	if !strings.Contains(mxcli("-p", mpr, "-c", "describe settings"), "MxTest.RegisterEndpoint") {
		t.Fatal("precondition: the running app's project does not carry the test endpoint")
	}

	st, err := docker.ReadRunState(mpr)
	if err != nil || st == nil {
		t.Fatalf("reading the run state: %v", err)
	}
	// What taskkill /F does on Windows: no signal the run can handle.
	if err := syscall.Kill(st.PID, syscall.SIGKILL); err != nil {
		t.Fatalf("killing the run: %v", err)
	}
	waitGone(func() bool { return docker.PidAlive(st.PID) }, 10*time.Second)

	out := mxcli("run", "stop", "-p", mpr)

	settings := mxcli("-p", mpr, "-c", "describe settings")
	if strings.Contains(settings, "MxTest") {
		t.Errorf("after-startup still points at the test endpoint after run stop:\n%s\nrun stop said:\n%s", settings, out)
	}
	if strings.Contains(mxcli("-p", mpr, "-c", "show modules"), "MxTest") {
		t.Errorf("module MxTest is still in the project after run stop; run stop said:\n%s", out)
	}
	if after := projectFingerprint(t, work); after != before {
		t.Errorf("the project differs from before the run:\n%s\nrun stop said:\n%s", diffFingerprints(before, after), out)
	}

	// Control: a live run stopped normally (SIGTERM) removes its own endpoint,
	// and with it the record, so nothing is left for a later stop to act on.
	mxcli("run", "--local", "--test-endpoint", "--detach", "--db-type", "hsqldb",
		"--app-port", fmt.Sprint(appPort), "--admin-port", fmt.Sprint(adminPort),
		"--serve-port", fmt.Sprint(servePort), "-p", mpr)
	out = mxcli("run", "stop", "-p", mpr)
	if strings.Contains(out, "left in the project") {
		t.Errorf("a graceful stop left its endpoint for run stop to remove:\n%s", out)
	}
	if _, err := os.Stat(filepath.Join(work, ".mxcli", "test-endpoint-restore.json")); !os.IsNotExist(err) {
		t.Errorf("the restore record survives a graceful stop (stat: %v)", err)
	}
	if after := projectFingerprint(t, work); after != before {
		t.Errorf("a graceful stop left the project changed:\n%s", diffFingerprints(before, after))
	}
}

// projectFingerprint lists the files `run --test-endpoint` may touch — the
// .mpr, mprcontents/, themesource/mxtest and javasource/mxtest — with a hash of
// each, one per line. What mxbuild regenerates on any build (javascriptsource,
// deployment/) is left out: it changes without the endpoint too.
func projectFingerprint(t *testing.T, dir string) string {
	t.Helper()
	var lines []string
	add := func(path string) {
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		rel, _ := filepath.Rel(dir, path)
		lines = append(lines, fmt.Sprintf("%s %x", rel, sha256.Sum256(b)))
	}
	add(filepath.Join(dir, "App.mpr"))
	for _, sub := range []string{"mprcontents", "themesource/mxtest", "javasource/mxtest"} {
		root := filepath.Join(dir, sub)
		if _, err := os.Stat(root); err != nil {
			continue
		}
		_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err == nil && !d.IsDir() {
				add(p)
			}
			return nil
		})
	}
	return strings.Join(lines, "\n")
}

func diffFingerprints(before, after string) string {
	in := func(s string) map[string]bool {
		m := map[string]bool{}
		for _, l := range strings.Split(s, "\n") {
			m[l] = true
		}
		return m
	}
	b, a := in(before), in(after)
	var out []string
	for l := range b {
		if !a[l] {
			out = append(out, "- "+l)
		}
	}
	for l := range a {
		if !b[l] {
			out = append(out, "+ "+l)
		}
	}
	return strings.Join(out, "\n")
}
