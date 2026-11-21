// SPDX-License-Identifier: Apache-2.0

package testrunner

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// A run without a Docker daemon failed only at `docker compose up`, after it
// had generated .docker/ (and built into it) in the project — a Docker setup
// the user never had, left in their git status (ako/mxcli#2474). It now asks
// the daemon first and leaves the project untouched.
func TestNoDockerDaemonLeavesTheProjectUntouched_2474(t *testing.T) {
	dir := t.TempDir()
	mpr := filepath.Join(dir, "App.mpr")
	if err := os.WriteFile(mpr, []byte("not a real project"), 0o644); err != nil {
		t.Fatal(err)
	}
	prev := dockerDaemonReachable
	t.Cleanup(func() { dockerDaemonReachable = prev })

	dockerDaemonReachable = func() error { return errors.New("cannot connect to the Docker daemon") }
	_, err := runDockerAndCapture(RunOptions{ProjectPath: mpr, SkipBuild: true}, time.Second, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "--local") {
		t.Fatalf("err = %v, want the daemon failure with the --local hint", err)
	}
	if _, statErr := os.Stat(filepath.Join(dir, ".docker")); !os.IsNotExist(statErr) {
		t.Errorf("a run without a Docker daemon left .docker/ in the project (stat: %v)", statErr)
	}

	// Control: with a daemon the stack is generated, so the check above is
	// what kept the folder out, not a project the generator refuses.
	dockerDaemonReachable = func() error { return nil }
	_, _ = runDockerAndCapture(RunOptions{ProjectPath: mpr, SkipBuild: true}, time.Second, io.Discard)
	if _, statErr := os.Stat(filepath.Join(dir, ".docker")); statErr != nil {
		t.Errorf("control: with a daemon the run generated no .docker/ (stat: %v); the test proves nothing", statErr)
	}
}
