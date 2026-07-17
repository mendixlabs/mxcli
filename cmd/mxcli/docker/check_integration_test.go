// SPDX-License-Identifier: Apache-2.0

//go:build integration

package docker

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/mendixlabs/mxcli/sdk/mpr"
)

// TestCheck_PreservesMPRv2StorageFormat is the end-to-end guard for
// mendixlabs/mxcli#763. `mx update-widgets`, which Check runs before `mx check`,
// rewrites an MPRv2 project into the self-contained MPRv1 format (inlining every
// unit into the .mpr and deleting mprcontents/). Check must leave the on-disk
// storage format exactly as it found it. Both mx 11.9.0 (the version the CI
// integration job installs) and 11.12.1 were observed to perform this conversion,
// so on a v2 project this test genuinely exercises the bug: without the snapshot/
// restore fix the project would be MPRv1 after Check.
//
// Requires a resolvable mx (provided by the CI integration job's
// `mxcli setup mxbuild`); skips otherwise. Uses `mx create-project`, whose output
// is MPRv2, as the fixture — the same scaffolding the executor roundtrip suite uses.
func TestCheck_PreservesMPRv2StorageFormat(t *testing.T) {
	mxPath, err := ResolveMx("")
	if err != nil {
		t.Skipf("mx not resolvable: %v", err)
	}

	// Scaffold a fresh project. `mx create-project` (no template arg) writes App.mpr
	// into the working directory and produces MPRv2 storage.
	dir := t.TempDir()
	scaffold := exec.Command(mxPath, "create-project")
	scaffold.Dir = dir
	if out, err := scaffold.CombinedOutput(); err != nil {
		t.Skipf("mx create-project failed, cannot scaffold fixture: %v\n%s", err, out)
	}
	mprPath := filepath.Join(dir, "App.mpr")
	if _, err := os.Stat(mprPath); err != nil {
		t.Skipf("mx create-project did not produce App.mpr: %v", err)
	}

	// Precondition: the fixture must be MPRv2, or the test proves nothing.
	if v := mprStorageVersion(t, mprPath); v != mpr.MPRVersionV2 {
		t.Skipf("scaffolded project is %v, not MPRv2 — nothing to protect", v)
	}

	var stdout, stderr bytes.Buffer
	if err := Check(CheckOptions{
		ProjectPath: mprPath,
		Stdout:      &stdout,
		Stderr:      &stderr,
	}); err != nil {
		t.Fatalf("Check failed: %v\nstdout:\n%s\nstderr:\n%s", err, stdout.String(), stderr.String())
	}

	// Postcondition: still MPRv2. Without the fix, update-widgets would have left
	// it MPRv1.
	if v := mprStorageVersion(t, mprPath); v != mpr.MPRVersionV2 {
		t.Errorf("Check converted the project to %v; the MPRv2 storage format must be preserved (#763)", v)
	}
	if _, err := os.Stat(filepath.Join(dir, "mprcontents")); err != nil {
		t.Errorf("mprcontents/ missing after Check, storage format was not preserved: %v", err)
	}
}

// mprStorageVersion opens the .mpr and returns its detected storage-format version.
func mprStorageVersion(t *testing.T, mprPath string) mpr.MPRVersion {
	t.Helper()
	reader, err := mpr.Open(mprPath)
	if err != nil {
		t.Fatalf("mpr.Open(%s): %v", mprPath, err)
	}
	defer reader.Close()
	return reader.Version()
}
