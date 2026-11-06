// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func fakeNewProjectTools(t *testing.T, dir, version string) (string, string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	dir, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	mx := filepath.Join(dir, mxBinaryName())
	build := filepath.Join(dir, mxbuildBinaryName())
	for _, path := range []string{mx, build} {
		if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 90\n"), 0755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "mx.deps.json"), []byte(`{"targets":{"mx/`+version+`":{}}}`), 0644); err != nil {
		t.Fatal(err)
	}
	return mx, build
}

func TestResolveNewProjectTools_ExactInstallation(t *testing.T) {
	home := t.TempDir()
	isolateResolution(t, home)
	cacheFakeMx(t, home, "10.24.25", "10.24.25.999999")
	// Same-release installations can contain different builds, including betas.
	_, other := fakeNewProjectTools(t, filepath.Join(macOSApplicationsDir, "Mendix Studio Pro 10.24.25 Beta.app", "Contents", "modeler"), "10.24.25.999999")
	t.Setenv("PATH", filepath.Dir(other))
	mx, build := fakeNewProjectTools(t, filepath.Join(macOSApplicationsDir, "Mendix Studio Pro 10.24.25.app", "Contents", "modeler"), "10.24.25.122571")
	if got := localMxForVersion("10.24.25"); got == "" || got == mx {
		t.Fatalf("control: default resolution must find the other installation/cache, got %q", got)
	}
	for _, path := range []string{build, filepath.Dir(build), filepath.Dir(filepath.Dir(build))} {
		gotMx, gotBuild, err := ResolveNewProjectTools(path, "10.24.25")
		if err != nil {
			t.Fatal(err)
		}
		if gotMx != mx || gotBuild != build {
			t.Fatalf("resolved (%q, %q), want selected pair (%q, %q)", gotMx, gotBuild, mx, build)
		}
	}
}

func TestResolveNewProjectTools_WrongPlatform(t *testing.T) {
	_, build := fakeNewProjectTools(t, t.TempDir(), "11.13.0")
	header := []byte{0x7f, 'E', 'L', 'F'}
	if runtime.GOOS == "linux" {
		header = []byte{'M', 'Z', 0, 0}
	}
	if err := os.WriteFile(build, header, 0755); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ResolveNewProjectTools(build, "11.13.0"); err == nil {
		t.Fatal("a binary for another operating system must not be accepted")
	}
}

func TestResolveNewProjectTools_SymlinkKeepsSelectedPair(t *testing.T) {
	for _, foreignMx := range []bool{false, true} {
		name := "alias without mx"
		if foreignMx {
			name = "alias with foreign same-release mx"
		}
		t.Run(name, func(t *testing.T) {
			home := t.TempDir()
			isolateResolution(t, home)
			mx, build := fakeNewProjectTools(t, filepath.Join(home, "chosen", "modeler"), "11.13.0")
			aliases := filepath.Join(home, "aliases")
			if err := os.MkdirAll(aliases, 0755); err != nil {
				t.Fatal(err)
			}
			if foreignMx {
				_, foreignBuild := fakeNewProjectTools(t, aliases, "11.13.0")
				if err := os.Remove(foreignBuild); err != nil {
					t.Fatal(err)
				}
			}
			alias := filepath.Join(aliases, mxbuildBinaryName())
			if err := os.Symlink(build, alias); err != nil {
				if runtime.GOOS == "windows" {
					t.Skipf("symlink creation requires privileges: %v", err)
				}
				t.Fatal(err)
			}
			wantMx, err := filepath.EvalSymlinks(mx)
			if err != nil {
				t.Fatal(err)
			}
			wantBuild, err := filepath.EvalSymlinks(build)
			if err != nil {
				t.Fatal(err)
			}
			gotMx, gotBuild, err := ResolveNewProjectTools(alias, "11.13.0")
			if err != nil || gotMx != wantMx || gotBuild != wantBuild {
				t.Fatalf("resolved (%q, %q), err=%v; want selected pair (%q, %q)", gotMx, gotBuild, err, wantMx, wantBuild)
			}
		})
	}
}

func TestResolveNewProjectTools_RefusesInvalidOverride(t *testing.T) {
	for _, tc := range []struct{ name, actual, requested, remove string }{
		{"wrong release", "11.12.0", "11.13.0", ""},
		{"wrong build", "10.24.25.122571", "10.24.25.999999", ""},
		{"unknown version", "not-a-version", "11.13.0", ""},
		{"missing mx", "11.13.0", "11.13.0", "mx"},
		{"missing mxbuild", "11.13.0", "11.13.0", "mxbuild"},
		{"missing requested version", "11.13.0", "", ""},
		{"partial requested version", "11.13.0", "11.13", ""},
		{"too many version parts", "11.13.0", "11.13.0.1.2", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			isolateResolution(t, home)
			cacheFakeMx(t, home, tc.requested)
			mx, build := fakeNewProjectTools(t, filepath.Join(home, "chosen"), tc.actual)
			if tc.remove != "" {
				path := build
				if tc.remove == "mx" {
					path = mx
				}
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
			}
			if _, _, err := ResolveNewProjectTools(build, tc.requested); err == nil {
				t.Fatal("invalid explicit tools must fail without using another installation or cache")
			}
		})
	}
}

func TestResolveNewProjectTools_VersionForms(t *testing.T) {
	for _, tc := range []struct{ actual, requested string }{
		{"10.24.25.122571", "10.24.25.122571"},
		// Only a three-part assembly version is available in some installations;
		// the created project's full stamp checks the requested build later.
		{"10.24.25", "10.24.25.122571"},
	} {
		_, build := fakeNewProjectTools(t, t.TempDir(), tc.actual)
		if _, _, err := ResolveNewProjectTools(build, tc.requested); err != nil {
			t.Fatal(err)
		}
	}
}

func TestResolveNewProjectTools_NotExecutable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not use Unix executable permissions")
	}
	mx, build := fakeNewProjectTools(t, t.TempDir(), "11.13.0")
	for _, path := range []string{mx, build} {
		t.Run(filepath.Base(path), func(t *testing.T) {
			if err := os.Chmod(path, 0644); err != nil {
				t.Fatal(err)
			}
			defer os.Chmod(path, 0755)
			if _, _, err := ResolveNewProjectTools(build, "11.13.0"); err == nil {
				t.Fatal("non-executable tool must be refused")
			}
		})
	}
}
