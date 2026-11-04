// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/internal/testutil"
)

// SHOW VERSION printed `MPR Format: v2` for an MPR v1 project — a single .mpr
// with no mprcontents/ folder (ako/mxcli#1553, upstream mendixlabs/mxcli#1621).
// STATUS and the session log's connect record print the same value, so all
// three are asserted here, through the real CLI, on one project of each format.
// The v2 project is the control: it reported v2 before the fix as well.
func TestShowVersionReportsTheStorageFormatOnDisk(t *testing.T) {
	if testing.Short() {
		t.Skip("spawns mxcli against a project")
	}

	v1 := filepath.Join(t.TempDir(), "App.mpr")
	data, err := os.ReadFile(filepath.Join("..", "..", "modelsdk", "mpr", "testdata", "v1-project", "App.mpr"))
	if err != nil {
		t.Fatalf("read v1 fixture: %v", err)
	}
	if err := os.WriteFile(v1, data, 0o644); err != nil {
		t.Fatal(err)
	}

	v2dir := t.TempDir()
	if err := testutil.CopyFixture(v2dir, filepath.Join("..", "..", "testdata", "testapp-views")); err != nil {
		t.Fatalf("copy v2 fixture: %v", err)
	}
	v2 := filepath.Join(v2dir, "TestApp.mpr")

	for _, tc := range []struct {
		name string
		mpr  string
		want string
	}{
		{"v1", v1, "v1"},
		{"v2 control", v2, "v2"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Precondition: the fixture is laid out as the format it stands for.
			_, statErr := os.Stat(filepath.Join(filepath.Dir(tc.mpr), "mprcontents"))
			if hasContents := statErr == nil; hasContents != (tc.want == "v2") {
				t.Fatalf("fixture %s: mprcontents/ present = %v, want it present only for v2", tc.mpr, hasContents)
			}

			res := runMainStreams(t, filepath.Dir(tc.mpr), "-p", tc.mpr, "-c", "show version")
			if res.exitCode != 0 {
				t.Fatalf("show version exited %d\nstdout:\n%s\nstderr:\n%s", res.exitCode, res.stdout, res.stderr)
			}
			if want := "MPR Format:     " + tc.want + "\n"; !strings.Contains(res.stdout, want) {
				t.Errorf("show version: want %q in\n%s", want, res.stdout)
			}

			res = runMainStreams(t, filepath.Dir(tc.mpr), "-p", tc.mpr, "-c", "status")
			if want := "MPR Format: " + tc.want + "\n"; !strings.Contains(res.stdout, want) {
				t.Errorf("status: want %q in\n%s", want, res.stdout)
			}

			// The session log is what `mxcli diag --bundle` ships with a bug report.
			recs := runMainRecords(t, "connect", "-p", tc.mpr, "-c", "show version")
			if len(recs) == 0 {
				t.Fatal("no connect record in the session log")
			}
			for _, r := range recs {
				if got, want := r["mpr_format"], float64(tc.want[1]-'0'); got != want {
					t.Errorf("session log connect record: mpr_format = %v, want %v", got, want)
				}
			}
		})
	}
}
