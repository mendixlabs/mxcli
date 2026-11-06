// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// mxcli-todo findings #7: mxbuild sits beside the mx binary `mxcli new` already
// resolved, so the settle build should not need a second download — including on
// macOS/Windows, where the resolved mx comes from a Studio Pro install that no
// version cache knows about.
func TestResolveMxBuildForSettle_NextToMx(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("binary names differ on Windows; the Linux/macOS layout is what this checks")
	}
	dir := t.TempDir()
	mxPath := filepath.Join(dir, "mx")
	mxbuildPath := filepath.Join(dir, "mxbuild")
	for _, p := range []string{mxPath, mxbuildPath} {
		if err := os.WriteFile(p, []byte("#!/bin/sh\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	if got := resolveMxBuildForSettle(mxPath, "11.12.1"); got != mxbuildPath {
		t.Errorf("got %q, want %q", got, mxbuildPath)
	}
}

// With no mxbuild beside mx and nothing cached for the version, there is nothing
// to run — the caller must get "" and report a warning, not run a random binary.
func TestResolveMxBuildForSettle_NotFound(t *testing.T) {
	dir := t.TempDir()
	mxPath := filepath.Join(dir, "mx")
	if err := os.WriteFile(mxPath, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	// A version string no cache directory can match.
	if got := resolveMxBuildForSettle(mxPath, "0.0.0-not-a-version"); got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

func TestLastLines(t *testing.T) {
	tests := []struct {
		name string
		in   string
		n    int
		want string
	}{
		{"fewer lines than asked for", "a\nb\n", 5, "a\nb"},
		{"truncates to the tail", "a\nb\nc\nd\n", 2, "c\nd"},
		{"empty", "", 3, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := lastLines(tt.in, tt.n); got != tt.want {
				t.Errorf("lastLines(%q, %d) = %q, want %q", tt.in, tt.n, got, tt.want)
			}
		})
	}
}

// A settle build that cannot run must return an error the caller can print as a
// warning — never panic, and never take the project creation down with it.
func TestSettleGeneratedSources_NoMxBuild(t *testing.T) {
	dir := t.TempDir()
	mpr := filepath.Join(dir, "App.mpr")
	if err := os.WriteFile(mpr, []byte("not a real project"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := SettleGeneratedSources(mpr, filepath.Join(dir, "mx"), "", "0.0.0-not-a-version", os.Stdout)
	if err == nil {
		t.Fatal("expected an error when mxbuild cannot be found")
	}
	if !strings.Contains(err.Error(), "mxbuild") {
		t.Errorf("error should name what is missing, got: %v", err)
	}
}

func TestSettleGeneratedSources_ExplicitBuildDoesNotFallBack(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test executables use POSIX scripts")
	}
	home := t.TempDir()
	isolateResolution(t, home)
	jdk := filepath.Join(home, "jdk")
	if err := os.MkdirAll(filepath.Join(jdk, "bin"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(jdk, "bin", "java"), []byte("#!/bin/sh\necho 'openjdk version \"21\"'\n"), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("JAVA_HOME", jdk)
	mx, fallback := fakeNewProjectTools(t, filepath.Join(home, "fallback"), "11.13.0")
	_, selected := fakeNewProjectTools(t, filepath.Join(home, "selected"), "11.13.0")
	for _, path := range []string{fallback, selected} {
		if err := os.WriteFile(path, []byte("#!/bin/sh\nprintf '%s\\n' \"$0\" \"$@\" > \"$MXCLI_TEST_BUILD_RECORD\"\n"), 0755); err != nil {
			t.Fatal(err)
		}
	}
	project := filepath.Join(home, "App.mpr")
	if err := os.WriteFile(project, []byte("not a real project"), 0644); err != nil {
		t.Fatal(err)
	}
	record := filepath.Join(home, "build-record")
	t.Setenv("MXCLI_TEST_BUILD_RECORD", record)
	for _, tc := range []struct{ name, explicit, want string }{
		{"selected", selected, selected},
		{"missing selected", filepath.Join(home, "missing", mxbuildBinaryName()), ""},
		{"default sibling control", "", fallback},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_ = os.Remove(record)
			err := SettleGeneratedSources(project, mx, tc.explicit, "11.13.0", io.Discard)
			got, readErr := os.ReadFile(record)
			if tc.want == "" {
				if err == nil || !os.IsNotExist(readErr) {
					t.Fatalf("missing selected tool must fail without running fallback: err=%v record=%q read=%v", err, got, readErr)
				}
				return
			}
			if err != nil || readErr != nil || !strings.HasPrefix(string(got), tc.want+"\n--target=deploy\n") || !strings.Contains(string(got), project) {
				t.Fatalf("wrong build tool or args: err=%v record=%q read=%v", err, got, readErr)
			}
		})
	}
}

// The tail of the settle build mxbuild 10.24.0 printed for a `mxcli new` project
// whose scaffold layout named Atlas 4 design properties (mendixlabs/mxcli#1433).
const mxbuildModelErrorsOutput = `Executing step 'Check prerequisites'
Checking for errors...
Executing step 'Clean up'
Cleaning up...
ERROR: The project cannot be deployed, because it contains errors.
ERROR at MyFirstModule, Layout 'App_Default', Container 'topbarContent': Design property Flex container is not supported by your theme. Right-click to see more options.
ERROR at MyFirstModule, Layout 'App_Default', Container 'topbarContent': Design property Align items Y is not supported by your theme. Right-click to see more options.
BUILD FAILED
`

// mendixlabs/mxcli#1433: a settle build that fails because the MODEL has errors
// is not the same failure as a missing JDK. It means the project mxcli just
// created does not build, and the caller has to be able to tell, so it is a
// typed error carrying the model errors rather than a flat string.
func TestClassifySettleFailure_ModelErrors(t *testing.T) {
	err := classifySettleFailure(errors.New("exit status 3"), mxbuildModelErrorsOutput)
	var merr *ModelErrorsError
	if !errors.As(err, &merr) {
		t.Fatalf("a build refused for model errors must be a *ModelErrorsError, got %T: %v", err, err)
	}
	if len(merr.Errors) != 2 {
		t.Errorf("want the 2 model errors, got %d: %q", len(merr.Errors), merr.Errors)
	}
	if !strings.Contains(err.Error(), "Flex container") {
		t.Errorf("the error must name what is wrong, got: %v", err)
	}
}

// Anything else (a JVM crash, a missing tool) stays an ordinary error: the
// project may well be fine.
func TestClassifySettleFailure_OtherFailure(t *testing.T) {
	err := classifySettleFailure(errors.New("exit status 1"), "Error: could not start the JVM\n")
	var merr *ModelErrorsError
	if errors.As(err, &merr) {
		t.Fatalf("a non-model failure was classified as model errors: %v", err)
	}
	if err == nil || !strings.Contains(err.Error(), "could not start the JVM") {
		t.Errorf("the error must carry the output tail, got: %v", err)
	}
}
