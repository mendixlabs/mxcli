// SPDX-License-Identifier: Apache-2.0

// mendixlabs/mxcli#808: `mx update-widgets` rewrites an MPRv2 project into the
// self-contained MPRv1 format — it inlines every unit into the .mpr and deletes
// mprcontents/. #763 / PR #764 protected the invocation in check.go with a
// snapshot/restore, but build.go carried its own unprotected copy of the same
// invocation, so `mxcli docker build`, `docker run` and `docker reload` still
// converted the project silently while reporting success.
//
// The fix makes the protection a property of the operation rather than of one
// call site: runUpdateWidgets snapshots the v2 storage, runs the step, and returns
// the restore func for the caller to defer. These tests pin that behaviour, so a
// third call site cannot reintroduce the bug by forgetting the snapshot.
package docker

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/types"
)

// storageVersion reports the .mpr's detected on-disk storage format.
func storageVersion(t *testing.T, mprPath string) types.MPRVersion {
	t.Helper()
	reader, err := openReadOnly(mprPath)
	if err != nil {
		t.Fatalf("openReadOnly(%s): %v", mprPath, err)
	}
	defer reader.Disconnect()
	return reader.Version()
}

// v2Fixture copies the MPRv2 test project (.mpr index + mprcontents/ tree) into a
// temp dir, so a test may convert it without mutating shared testdata.
func v2Fixture(t *testing.T) string {
	t.Helper()
	dst := t.TempDir()
	if err := os.CopyFS(dst, os.DirFS("../../../testdata/expr-checker")); err != nil {
		t.Fatalf("copy v2 fixture: %v", err)
	}
	p := filepath.Join(dst, "minimal.mpr")
	if v := storageVersion(t, p); v != types.MPRVersionV2 {
		t.Fatalf("fixture is %v, not MPRv2 — this test would prove nothing", v)
	}
	return p
}

// v1Fixture copies the single-file MPRv1 test project into a temp dir.
func v1Fixture(t *testing.T) string {
	t.Helper()
	dst := t.TempDir()
	if err := os.CopyFS(dst, os.DirFS("../../../sdk/mpr/testdata/v1-project")); err != nil {
		t.Fatalf("copy v1 fixture: %v", err)
	}
	p := filepath.Join(dst, "App.mpr")
	if v := storageVersion(t, p); v != types.MPRVersionV1 {
		t.Fatalf("fixture is %v, not MPRv1", v)
	}
	return p
}

// stubUpdateWidgets swaps the mx invocation for the duration of a test.
func stubUpdateWidgets(t *testing.T, fn func(mxPath, pathArg string, w, stderr io.Writer) error) {
	t.Helper()
	prev := updateWidgetsCmd
	updateWidgetsCmd = fn
	t.Cleanup(func() { updateWidgetsCmd = prev })
}

// convertToV1 mimics what `mx update-widgets` does to an MPRv2 project: rewrite
// the .mpr as a self-contained v1 file and delete the mprcontents/ tree.
func convertToV1(t *testing.T, mprPath string) {
	t.Helper()
	if err := os.WriteFile(mprPath, []byte("MPRv1-self-contained-inlined"), 0644); err != nil {
		t.Fatalf("simulating conversion (.mpr): %v", err)
	}
	if err := os.RemoveAll(filepath.Join(filepath.Dir(mprPath), "mprcontents")); err != nil {
		t.Fatalf("simulating conversion (mprcontents): %v", err)
	}
}

// TestRunUpdateWidgets_RestoresV2AfterConversion is the core regression: whatever
// update-widgets does to the on-disk format, the restore must undo it.
func TestRunUpdateWidgets_RestoresV2AfterConversion(t *testing.T) {
	mprPath := v2Fixture(t)
	contentsDir := filepath.Join(filepath.Dir(mprPath), "mprcontents")

	orig, err := os.ReadFile(mprPath)
	if err != nil {
		t.Fatal(err)
	}

	ran := false
	stubUpdateWidgets(t, func(_, _ string, _, _ io.Writer) error {
		ran = true
		convertToV1(t, mprPath)
		return nil
	})

	var out bytes.Buffer
	restore := runUpdateWidgets("mx", mprPath, &out, io.Discard)
	if !ran {
		t.Fatal("update-widgets was not run on a project whose format could be snapshotted")
	}

	// Before restore the caller's check/build step still sees the normalized model —
	// that is the whole reason restore is deferred rather than run immediately.
	if _, err := os.Stat(contentsDir); !os.IsNotExist(err) {
		t.Fatalf("fixture setup wrong: mprcontents/ should be gone at this point (err=%v)", err)
	}

	restore()

	if got, err := os.ReadFile(mprPath); err != nil {
		t.Fatalf("read restored .mpr: %v", err)
	} else if !bytes.Equal(got, orig) {
		t.Errorf(".mpr not restored: got %d bytes, want the original %d (#808)", len(got), len(orig))
	}
	if v := storageVersion(t, mprPath); v != types.MPRVersionV2 {
		t.Errorf("project left as %v; MPRv2 storage must be preserved (#808)", v)
	}
	if entries, err := os.ReadDir(contentsDir); err != nil {
		t.Errorf("mprcontents/ not restored: %v", err)
	} else if len(entries) == 0 {
		t.Error("mprcontents/ restored but empty")
	}
}

// TestRunUpdateWidgets_V1NeedsNoSnapshot: a v1 project is already single-file, so
// there is nothing to protect — the step must still run, and restore must be a
// harmless no-op rather than clobbering the project.
func TestRunUpdateWidgets_V1NeedsNoSnapshot(t *testing.T) {
	mprPath := v1Fixture(t)
	orig, err := os.ReadFile(mprPath)
	if err != nil {
		t.Fatal(err)
	}

	ran := false
	stubUpdateWidgets(t, func(_, _ string, _, _ io.Writer) error {
		ran = true
		return nil
	})

	var out bytes.Buffer
	restore := runUpdateWidgets("mx", mprPath, &out, io.Discard)
	if !ran {
		t.Error("update-widgets was skipped on an MPRv1 project, which needs no protection")
	}
	if restore == nil {
		t.Fatal("restore is nil — callers defer it unconditionally")
	}
	restore()

	if got, err := os.ReadFile(mprPath); err != nil {
		t.Fatalf("read .mpr: %v", err)
	} else if !bytes.Equal(got, orig) {
		t.Error("restore modified an MPRv1 project it never snapshotted")
	}
}

// TestRunUpdateWidgets_SkipsStepWhenSnapshotFails is the fail-safe: if the storage
// cannot be backed up, running update-widgets risks an unrecoverable conversion.
// A CE0463 false positive is the lesser evil, so the step must not run at all.
func TestRunUpdateWidgets_SkipsStepWhenSnapshotFails(t *testing.T) {
	mprPath := v2Fixture(t)
	// Remove mprcontents/ so the snapshot's directory copy fails. The project still
	// reads as v2 (detection keys off the absent Unit.Contents column), so this is
	// the "v2, but cannot be protected" case rather than a v1 project.
	if err := os.RemoveAll(filepath.Join(filepath.Dir(mprPath), "mprcontents")); err != nil {
		t.Fatal(err)
	}
	if v := storageVersion(t, mprPath); v != types.MPRVersionV2 {
		t.Fatalf("fixture no longer reads as MPRv2 (%v) — test premise broken", v)
	}

	ran := false
	stubUpdateWidgets(t, func(_, _ string, _, _ io.Writer) error {
		ran = true
		return nil
	})

	var out bytes.Buffer
	restore := runUpdateWidgets("mx", mprPath, &out, io.Discard)
	restore()

	if ran {
		t.Error("update-widgets ran without a usable snapshot — risks an unrecoverable v2->v1 conversion (#808)")
	}
	if !bytes.Contains(out.Bytes(), []byte("skipping update-widgets")) {
		t.Errorf("the skip must be reported to the user, got:\n%s", out.String())
	}
}

// TestRunUpdateWidgets_RestoresWhenStepFails: a failed update-widgets is non-fatal,
// but it may still have converted the project before failing, so the snapshot must
// be restored (and its temp dir cleaned up) on that path too.
func TestRunUpdateWidgets_RestoresWhenStepFails(t *testing.T) {
	mprPath := v2Fixture(t)

	stubUpdateWidgets(t, func(_, _ string, _, _ io.Writer) error {
		convertToV1(t, mprPath)
		return io.ErrUnexpectedEOF
	})

	var out bytes.Buffer
	restore := runUpdateWidgets("mx", mprPath, &out, io.Discard)
	restore()

	if v := storageVersion(t, mprPath); v != types.MPRVersionV2 {
		t.Errorf("project left as %v after a failed update-widgets; the snapshot must still be restored (#808)", v)
	}
}
