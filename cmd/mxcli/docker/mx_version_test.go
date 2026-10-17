// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"os"
	"path/filepath"
	"testing"
)

// writeMx makes <root>/<verDir>/modeler/mx(.exe) and, when deps is not empty, the
// mx.deps.json beside it. It returns the path of the binary.
func writeMx(t *testing.T, root, verDir, deps string) string {
	t.Helper()
	dir := filepath.Join(root, verDir, "modeler")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(dir, mxBinaryName())
	if err := os.WriteFile(bin, []byte("binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	if deps != "" {
		if err := os.WriteFile(filepath.Join(dir, "mx.deps.json"), []byte(deps), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return bin
}

// depsFor is the shape of the mx.deps.json Studio Pro ships: the target key names
// the assembly and its version, with a literal "\n" escape after the version
// (measured on 10.24.9 and 11.12.5).
func depsFor(version string) string {
	return `{"targets": {".NETCoreApp,Version=v10.0": {"mx/` + version + `\n": {"dependencies": {"Mendix.Common": "9.9.9"}}}}}`
}

func TestMxBinaryVersion_ReadsMxDepsJSONWhateverTheFolderIsCalled(t *testing.T) {
	// A folder named after no version (a PATH install, a renamed copy): the
	// deps file still says which Mendix version the binary is.
	bin := writeMx(t, t.TempDir(), "tools", depsFor("11.12.5"))
	if got := MxBinaryVersion(bin); got != "11.12.5" {
		t.Errorf("MxBinaryVersion = %q, want 11.12.5", got)
	}
}

func TestMxBinaryVersion_DepsFileWinsOverTheFolderName(t *testing.T) {
	// The folder says 11.6.3, the binary says 11.12.5: believe the binary.
	bin := writeMx(t, t.TempDir(), "11.6.3", depsFor("11.12.5"))
	if got := MxBinaryVersion(bin); got != "11.12.5" {
		t.Errorf("MxBinaryVersion = %q, want 11.12.5", got)
	}
}

func TestMxBinaryVersion_FallsBackToTheVersionFolder(t *testing.T) {
	// No deps file (an older Studio Pro): ~/.mxcli/mxbuild/<ver>/modeler/mx and
	// Program Files/Mendix/<ver>/modeler/mx both name the version.
	for _, dir := range []string{"11.6.3", "10.24.9.81004"} {
		bin := writeMx(t, t.TempDir(), dir, "")
		if got := MxBinaryVersion(bin); got != dir {
			t.Errorf("MxBinaryVersion(%s) = %q, want %q", dir, got, dir)
		}
	}
}

func TestMxBinaryVersion_UnknownWhenNothingSaysSo(t *testing.T) {
	for _, tc := range []struct{ name, dir, deps string }{
		{"no deps file, folder is not a version", "tools", ""},
		{"deps file without an mx entry", "tools", `{"targets": {"x": {"other/1.2.3": {}}}}`},
		{"folder looks like a version but is not a release", "v2", ""},
		{"two-part folder", "11.12", ""},
	} {
		bin := writeMx(t, t.TempDir(), tc.dir, tc.deps)
		if got := MxBinaryVersion(bin); got != "" {
			t.Errorf("%s: MxBinaryVersion = %q, want unknown", tc.name, got)
		}
	}
	if got := MxBinaryVersion(filepath.Join(t.TempDir(), "nowhere", "modeler", "mx")); got != "" {
		t.Errorf("a binary that does not exist: %q, want unknown", got)
	}
}

func TestSameMendixRelease(t *testing.T) {
	for _, tc := range []struct {
		a, b string
		want bool
	}{
		{"11.12.5", "11.12.5", true},
		{"11.12.5", "11.12.1", false},
		{"11.12.5", "11.6.5", false},
		{"11.12.5", "10.12.5", false},
		{"10.24.9", "10.24.9.81004", true}, // build number is not part of the release
		{"10.24.9.81004", "10.24.9.99999", true},
		{"11.12.5", "", false},
		{"", "", false}, // nothing known is never a match
		{"11.12", "11.12.5", false},
		{"11.12.x", "11.12.5", false},
	} {
		if got := SameMendixRelease(tc.a, tc.b); got != tc.want {
			t.Errorf("SameMendixRelease(%q, %q) = %v, want %v", tc.a, tc.b, got, tc.want)
		}
	}
}
