// SPDX-License-Identifier: Apache-2.0

//go:build integration

package docker

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// The end-to-end guard two projects independently asked for after
// `mxcli test --local` stopped working for every project (mxcli-ledger §150,
// mxcli-sudoku §51):
//
//	Error: local runtime: runtime admin API did not come up:
//	       runtime process exited during startup
//	java.lang.IllegalArgumentException: Path
//	  '…/.mxcli/deployment-test/model/bundles' cannot be resolved in base path
//	  '…/.mxcli/deployment-test'
//
// The boot was pointed at a directory the build never wrote to. mxbuild deploys
// to `<app dir>/deployment` and has no option to move it, so redirecting the
// runtime alone left it reading an empty tree.
//
// Both reports drew the same lesson, and it is about the tests rather than the
// code: the four unit tests covering this asserted that the OPTION was set, was
// under .mxcli/, was per-project and was not the dev loop's — and every one
// passed against a build that could not start. Sudoku §51 names the remedy
// exactly: "an end-to-end assertion that one `mxcli test --local` run leaves a
// model/ directory where the runtime is told to look."
//
// So this asserts the ARTEFACT, not the option: after a real build, the
// directory the runtime will be booted against contains what the runtime needs.
// It runs the build only — booting a JVM and a database is the test runner's own
// integration territory, and the mismatch is fully visible one step earlier.
//
// Requires a resolvable mxbuild (the CI integration job installs one); skips
// otherwise.
func TestLocalApp_BuildPopulatesTheDirectoryTheRuntimeBootsAgainst(t *testing.T) {
	mprPath := os.Getenv("MXCLI_IT_PROJECT")
	if mprPath == "" {
		// No fixture given: scaffold one. `mx create-project` produces a project at
		// the INSTALLED mxbuild's version, whose JDK may not be present — hence the
		// override above, which lets this run against a project the machine can
		// actually build. A test that only ever skips proves nothing.
		mxPath, err := ResolveMx("")
		if err != nil {
			t.Skipf("mx not resolvable and MXCLI_IT_PROJECT unset: %v", err)
		}
		// NOT t.TempDir(): it names the directory after the test function, and
		// Mendix's toolset rejects the result with PathTooLongException — which
		// this test would then report as a skip. A short path is the fix.
		dir, err := os.MkdirTemp("", "mxapp")
		if err != nil {
			t.Fatal(err)
		}
		defer os.RemoveAll(dir)

		scaffold := exec.Command(mxPath, "create-project")
		scaffold.Dir = dir
		if out, err := scaffold.CombinedOutput(); err != nil {
			t.Skipf("mx create-project failed, cannot scaffold fixture: %v\n%s", err, out)
		}
		mprPath = filepath.Join(dir, "App.mpr")
	}
	if _, err := os.Stat(mprPath); err != nil {
		t.Skipf("no project fixture at %s: %v", mprPath, err)
	}

	// The options a headless caller (the test runner) boots with, defaults applied
	// exactly as StartLocalApp applies them.
	opts := LocalAppOptions{ProjectPath: mprPath, Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}}
	opts.applyDefaults()

	// The guard StartLocalApp runs before it builds anything. A DeployDir the
	// build will not write to is refused rather than booted against.
	if err := checkDeployDirIsBuildable(opts); err != nil {
		t.Fatalf("the default options are not buildable: %v", err)
	}

	reader, err := openReadOnly(mprPath)
	if err != nil {
		t.Skipf("cannot open project: %v", err)
	}
	version := reader.ProjectVersion().ProductVersion
	reader.Disconnect()

	serveJavaMajor, _ := ProjectJavaMajor(mprPath)
	serve, err := StartServe(ServeOptions{
		Version: version, JavaMajor: serveJavaMajor, Host: "127.0.0.1", Port: 6547,
	})
	if err != nil {
		t.Skipf("cannot start mxbuild serve: %v", err)
	}
	defer serve.Stop()

	build, err := serve.Build(BuildRequest{Target: TargetDeploy, ProjectFilePath: mprPath})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if !build.OK() {
		t.Fatalf("build failed: %s", build.Message)
	}

	// The assertion the unit tests could not make: the runtime's BasePath holds a
	// model. `model/bundles` is the exact path the JVM named when this broke.
	for _, want := range []string{"model", filepath.Join("model", "bundles")} {
		path := filepath.Join(opts.DeployDir, want)
		if _, err := os.Stat(path); err != nil {
			t.Errorf("after a build, %s does not exist — the runtime boots against %s "+
				"and would exit during startup: %v", path, opts.DeployDir, err)
		}
	}
}
