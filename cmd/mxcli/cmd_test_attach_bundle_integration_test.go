//go:build integration && linux

// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/cmd/mxcli/docker"
)

// TestTestAttach_KeepsTheHostingAppsPageBundles: `mxcli test --attach` builds
// its test microflows into the app `run --local --test-endpoint` is serving.
// That build went to the run's own mxbuild --serve, where on the prebuilt-client
// Mendix versions (11.12.5+) any build after a process's first leaves web/dist
// without its page modules (mendixlabs/mxcli#1580) — so after a green attach
// the app answered 404 on dist/pages/*.js and every page showed the generic
// error dialog until it was restarted (ako/mxcli#1917, mendixlabs/mxcli#1723).
//
//	MXCLI_RUNLIFE_VERSION=11.12.6 go test -tags integration ./cmd/mxcli -run KeepsTheHostingApps -v
//
// The control is the same page module before the attach, which the boot's
// first (complete) build serves with a 200.
//
// On Mendix 11.14.0 the attach cannot apply at all: every later build in a serve
// process fails, even for an unchanged model, and the failed response carries no
// restartRequired (measured over mxbuild's /build API; 11.15.0 builds them and
// answers false for a microflow). The only answer is a cold build's, which asks
// for a restart whatever changed, so the attach is refused and must say why
// rather than claim an entity changed. Its pages must survive all the same. When
// an 11.14 mxbuild builds incrementally again, that branch fails and should go.
func TestTestAttach_KeepsTheHostingAppsPageBundles(t *testing.T) {
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
	work, err := os.MkdirTemp("", "mxta")
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
	coldAnswerOnly := projectAtLeast(t, mpr, 11, 14) && !projectAtLeast(t, mpr, 11, 15)

	const appPort, adminPort, servePort = 8487, 8497, 6943
	for _, p := range []int{appPort, adminPort, servePort} {
		if portAnswers(p) {
			t.Skipf("port %d is in use", p)
		}
	}
	run := func(args ...string) (string, error) {
		cmd := exec.Command(bin, args...)
		cmd.Dir = work
		var out bytes.Buffer
		cmd.Stdout, cmd.Stderr = &out, &out
		err := cmd.Run()
		return out.String(), err
	}
	mxcli := func(args ...string) string {
		out, err := run(args...)
		if err != nil {
			t.Fatalf("mxcli %v: %v\n%s", args, err, out)
		}
		return out
	}
	t.Cleanup(func() { _ = exec.Command(bin, "run", "stop", "-p", mpr).Run() })

	mxcli("run", "--local", "--test-endpoint", "--detach", "--db-type", "hsqldb",
		"--app-port", fmt.Sprint(appPort), "--admin-port", fmt.Sprint(adminPort),
		"--serve-port", fmt.Sprint(servePort), "-p", mpr)

	pages, _ := filepath.Glob(filepath.Join(work, "deployment", "web", "dist", "pages", "*.js"))
	if len(pages) == 0 {
		t.Skip("this Mendix version does not prebuild page modules into web/dist; nothing to lose")
	}
	pageURL := fmt.Sprintf("http://127.0.0.1:%d/dist/pages/%s", appPort, filepath.Base(pages[0]))
	status := func() int {
		resp, err := http.Get(pageURL)
		if err != nil {
			t.Fatalf("GET %s: %v", pageURL, err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}
	if got := status(); got != http.StatusOK {
		t.Fatalf("control: %s answers %d before the attach, want 200", pageURL, got)
	}

	test := filepath.Join(work, "trivial.test.mdl")
	src := "mdl 1;\n\n/**\n * @test Trivial assertion\n * @expect $Ok = true\n */\ndeclare $Ok boolean = true;\n/\n"
	if err := os.WriteFile(test, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := run("test", test, "-p", mpr, "--attach", "--skip-app-startup", "--require-assertions")
	switch {
	case coldAnswerOnly:
		if err == nil || !strings.Contains(out, "could not build this change incrementally, and the full build") ||
			strings.Contains(out, "an entity or association changed") {
			t.Fatalf("Mendix 11.14: want the attach refused for want of an incremental answer, not for an entity "+
				"change; if this mxbuild builds incrementally again, drop the 11.14 branch (err %v):\n%s", err, out)
		}
	case err != nil || !strings.Contains(out, "Passed: 1"):
		t.Fatalf("the attach run did not pass (err %v):\n%s", err, out)
	}

	if got := status(); got != http.StatusOK {
		left, _ := filepath.Glob(filepath.Join(work, "deployment", "web", "dist", "pages", "*.js"))
		t.Errorf("%s answers %d after test --attach, want 200 (page modules left in dist/pages: %d)\nattach said:\n%s",
			pageURL, got, len(left), out)
	}
}
