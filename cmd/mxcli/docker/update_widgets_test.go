// SPDX-License-Identifier: Apache-2.0

// mendixlabs/mxcli#808: `mx update-widgets` rewrites an MPRv2 project into the
// self-contained MPRv1 format — it inlines every unit into the .mpr and deletes
// mprcontents/. #763 / PR #764 protected the invocation in check.go with a
// snapshot/restore, but build.go carried its own unprotected copy of the same
// invocation, so `mxcli docker build`, `docker run` and `docker reload` still
// converted the project silently while reporting success.
//
// That snapshot/restore (runUpdateWidgets) is gone: since ako/mxcli#951 and #961
// both `docker check` and `docker build` run update-widgets on a temporary copy,
// which protects an MPRv1 project as well (check_readonly_test.go,
// build_readonly_test.go). The fixtures below are shared with those tests.
package docker

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mendixlabs/mxcli/internal/testutil"
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
	if err := testutil.CopyFixture(dst, "../../../testdata/expr-checker"); err != nil {
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
	if err := testutil.CopyFixture(dst, "../../../modelsdk/mpr/testdata/v1-project"); err != nil {
		t.Fatalf("copy v1 fixture: %v", err)
	}
	p := filepath.Join(dst, "App.mpr")
	if v := storageVersion(t, p); v != types.MPRVersionV1 {
		t.Fatalf("fixture is %v, not MPRv1", v)
	}
	return p
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
