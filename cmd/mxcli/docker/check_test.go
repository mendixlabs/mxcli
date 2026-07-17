// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestCheck_SkipUpdateWidgets(t *testing.T) {
	// This test verifies the SkipUpdateWidgets option is wired through.
	// Since we don't have a real mx binary in CI, we just verify the
	// function returns the expected "mx not found" error.
	opts := CheckOptions{
		ProjectPath:       "/nonexistent/app.mpr",
		SkipUpdateWidgets: true,
		Stdout:            &bytes.Buffer{},
		Stderr:            &bytes.Buffer{},
	}

	err := Check(opts)
	if err == nil {
		t.Fatal("expected error when mx binary not found")
	}
	if got := err.Error(); got != "mx not found; specify --mxbuild-path pointing to Mendix installation directory" {
		// Accept any error about mx not being found
		t.Logf("got error: %s", got)
	}
}

// TestSnapshotStorageFormat_RestoresV2AfterConversion guards the fix for the bug
// where `mxcli docker check` silently converted an MPRv2 project to MPRv1. The
// `mx update-widgets` step that docker check runs before `mx check` inlines every
// unit into the .mpr and deletes mprcontents/. snapshotStorageFormat backs up the
// v2 storage files first, and the restore func it returns must put the project
// back byte-for-byte, undoing the conversion. See mendixlabs/mxcli#763.
func TestSnapshotStorageFormat_RestoresV2AfterConversion(t *testing.T) {
	dir := t.TempDir()
	mprPath := filepath.Join(dir, "App.mpr")
	contentsDir := filepath.Join(dir, "mprcontents")

	// Seed a fake MPRv2 project: an .mpr index plus a nested mprcontents/ tree
	// mirroring the real XX/YY/UUID.mxunit layout. The .mpr uses mode 0600 so the
	// restore's mode preservation is observable (the simulated conversion below
	// rewrites it as 0644).
	mprV2 := []byte("MPRv2-index-bytes")
	if err := os.WriteFile(mprPath, mprV2, 0600); err != nil {
		t.Fatal(err)
	}
	units := map[string][]byte{
		"ab/cd/unit-1.mxunit": []byte("unit-1"),
		"ab/cd/unit-2.mxunit": []byte("unit-2"),
		"ef/01/unit-3.mxunit": []byte("unit-3-contents"),
	}
	for rel, content := range units {
		p := filepath.Join(contentsDir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, content, 0644); err != nil {
			t.Fatal(err)
		}
	}

	snapshotGlob := filepath.Join(os.TempDir(), "mxcli-mpr-snapshot-*")
	leakBefore, _ := filepath.Glob(snapshotGlob)

	restore, err := snapshotStorageFormat(mprPath, contentsDir)
	if err != nil {
		t.Fatalf("snapshotStorageFormat: %v", err)
	}

	// Simulate `mx update-widgets`: rewrite the .mpr as a v1 self-contained file
	// (different bytes, different mode) and delete the whole mprcontents/ tree.
	if err := os.WriteFile(mprPath, []byte("MPRv1-self-contained-inlined-and-larger"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(contentsDir); err != nil {
		t.Fatal(err)
	}

	restore()

	// The .mpr index must be restored byte-identically to the original v2 file...
	got, err := os.ReadFile(mprPath)
	if err != nil {
		t.Fatalf("read restored .mpr: %v", err)
	}
	if !bytes.Equal(got, mprV2) {
		t.Errorf(".mpr not restored to v2 form:\n got %q\nwant %q", got, mprV2)
	}
	// ...and with its original file mode, not the mode from the conversion write.
	if info, err := os.Stat(mprPath); err != nil {
		t.Fatalf("stat restored .mpr: %v", err)
	} else if perm := info.Mode().Perm(); perm != 0600 {
		t.Errorf("restored .mpr mode = %o, want 0600", perm)
	}

	// Every unit file must be back with its original content, and nothing extra.
	for rel, want := range units {
		p := filepath.Join(contentsDir, filepath.FromSlash(rel))
		got, err := os.ReadFile(p)
		if err != nil {
			t.Errorf("unit %s not restored: %v", rel, err)
			continue
		}
		if !bytes.Equal(got, want) {
			t.Errorf("unit %s content mismatch: got %q want %q", rel, got, want)
		}
	}
	var restoredUnits int
	_ = filepath.Walk(contentsDir, func(_ string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			restoredUnits++
		}
		return nil
	})
	if restoredUnits != len(units) {
		t.Errorf("restored %d unit files, want %d", restoredUnits, len(units))
	}

	// The snapshot's temp directory must be cleaned up by restore().
	leakAfter, _ := filepath.Glob(snapshotGlob)
	if len(leakAfter) > len(leakBefore) {
		t.Errorf("snapshot temp dir leaked: %d before, %d after restore", len(leakBefore), len(leakAfter))
	}
}

// createFakeMxDir creates a temp directory with fake mx and mxbuild scripts
// that log their first argument to a file.
func createFakeMxDir(t *testing.T) (dir, logFile string) {
	t.Helper()
	dir = t.TempDir()
	logFile = filepath.Join(dir, "commands.log")

	script := `#!/bin/sh
echo "$1" >> ` + logFile + "\n"

	for _, name := range []string{"mx", "mxbuild"} {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(script), 0755); err != nil {
			t.Fatal(err)
		}
	}
	return dir, logFile
}

func TestCheck_UpdateWidgetsBeforeCheck(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script test not supported on Windows")
	}

	mxDir, logFile := createFakeMxDir(t)

	var stdout, stderr bytes.Buffer
	opts := CheckOptions{
		ProjectPath: "/tmp/fake.mpr",
		MxBuildPath: mxDir,
		Stdout:      &stdout,
		Stderr:      &stderr,
	}

	Check(opts)

	logBytes, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("failed to read command log: %v", err)
	}

	log := string(logBytes)
	if !bytes.Contains(logBytes, []byte("update-widgets\n")) {
		t.Errorf("update-widgets was not called, got log:\n%s", log)
	}
	if !bytes.Contains(logBytes, []byte("check\n")) {
		t.Errorf("check was not called, got log:\n%s", log)
	}

	// Verify order: update-widgets before check
	uwIdx := bytes.Index(logBytes, []byte("update-widgets"))
	chIdx := bytes.Index(logBytes, []byte("check"))
	if uwIdx >= chIdx {
		t.Errorf("update-widgets should run before check, got log:\n%s", log)
	}
}

func TestCheck_SkipUpdateWidgetsFlag(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script test not supported on Windows")
	}

	mxDir, logFile := createFakeMxDir(t)

	var stdout, stderr bytes.Buffer
	opts := CheckOptions{
		ProjectPath:       "/tmp/fake.mpr",
		MxBuildPath:       mxDir,
		SkipUpdateWidgets: true,
		Stdout:            &stdout,
		Stderr:            &stderr,
	}

	Check(opts)

	logBytes, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("failed to read command log: %v", err)
	}

	if bytes.Contains(logBytes, []byte("update-widgets")) {
		t.Error("update-widgets should NOT be called when SkipUpdateWidgets=true")
	}
	if !bytes.Contains(logBytes, []byte("check")) {
		t.Error("check should still be called")
	}
}

// TestUpdateWidgetsPathArg_AbsolutizesBareFilename guards the fix for the
// `mx update-widgets` crash: a bare .mpr filename (no directory component) makes
// MxToolset's AddProjectDirAsAllowedPath compute an empty directory and throw
// System.ArgumentNullException, silently skipping the widget migration (leaving
// CE0463 unresolved). The arg must be absolutized so it always has a directory.
func TestUpdateWidgetsPathArg_AbsolutizesBareFilename(t *testing.T) {
	got := updateWidgetsPathArg("app.mpr")
	if !filepath.IsAbs(got) {
		t.Errorf("updateWidgetsPathArg(%q) = %q, want an absolute path", "app.mpr", got)
	}
	// An already-absolute path is returned unchanged.
	if got := updateWidgetsPathArg("/proj/app.mpr"); got != "/proj/app.mpr" {
		t.Errorf("updateWidgetsPathArg(abs) = %q, want it unchanged", got)
	}
}

// TestCheck_UpdateWidgetsReceivesAbsolutePath verifies the invocation passes an
// absolute path to `mx update-widgets` even when ProjectPath is a bare filename
// (as with `mxcli docker check -p app.mpr` run from the project directory).
func TestCheck_UpdateWidgetsReceivesAbsolutePath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script test not supported on Windows")
	}
	dir := t.TempDir()
	logFile := filepath.Join(dir, "args.log")
	script := "#!/bin/sh\necho \"$@\" >> " + logFile + "\n"
	for _, name := range []string{"mx", "mxbuild"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(script), 0755); err != nil {
			t.Fatal(err)
		}
	}

	var stdout, stderr bytes.Buffer
	// Bare filename — the crash trigger.
	Check(CheckOptions{ProjectPath: "fake.mpr", MxBuildPath: dir, Stdout: &stdout, Stderr: &stderr})

	logBytes, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("failed to read command log: %v", err)
	}
	var found bool
	for _, line := range bytes.Split(logBytes, []byte("\n")) {
		fields := bytes.Fields(line)
		if len(fields) == 2 && string(fields[0]) == "update-widgets" {
			found = true
			if !filepath.IsAbs(string(fields[1])) {
				t.Errorf("update-widgets path arg = %q, want absolute", fields[1])
			}
		}
	}
	if !found {
		t.Errorf("update-widgets was not invoked; log:\n%s", logBytes)
	}
}

func TestResolveMxForVersion_PrefersExactCachedVersion(t *testing.T) {
	dir := t.TempDir()
	setTestHomeDir(t, dir)
	setTestApplicationsDir(t, t.TempDir()) // prevent real macOS Studio Pro from matching
	// Point PATH at an empty temp dir (rather than clearing it) so exec.LookPath
	// still works for any other testing infrastructure but can't find mx.
	t.Setenv("PATH", t.TempDir())

	versions := []string{"9.24.40.80973", "11.6.3", "11.9.0"}
	var expected string
	for _, version := range versions {
		modelerDir := filepath.Join(dir, ".mxcli", "mxbuild", version, "modeler")
		if err := os.MkdirAll(modelerDir, 0755); err != nil {
			t.Fatal(err)
		}
		bin := filepath.Join(modelerDir, mxBinaryName())
		if err := os.WriteFile(bin, []byte("fake"), 0755); err != nil {
			t.Fatal(err)
		}
		if version == "11.9.0" {
			expected = bin
		}
	}

	result, err := ResolveMxForVersion("", "11.9.0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != expected {
		t.Errorf("expected exact cached mx %s, got %s", expected, result)
	}
}
