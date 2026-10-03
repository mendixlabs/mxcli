// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/mendixlabs/mxcli/internal/testutil"
)

func TestResolveMxBuild_ExplicitPath(t *testing.T) {
	dir := t.TempDir()
	fakeBin := filepath.Join(dir, "mxbuild")
	if runtime.GOOS == "windows" {
		fakeBin += ".exe"
	}
	os.WriteFile(fakeBin, []byte("fake"), 0755)

	result, err := resolveMxBuild(fakeBin)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != fakeBin {
		t.Errorf("expected %s, got %s", fakeBin, result)
	}
}

func TestResolveMxBuild_ExplicitDir_FindsBinaryInRoot(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, mxbuildBinaryName())
	os.WriteFile(bin, []byte("fake"), 0755)

	result, err := resolveMxBuild(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != bin {
		t.Errorf("expected %s, got %s", bin, result)
	}
}

func TestResolveMxBuild_ExplicitDir_FindsBinaryInModeler(t *testing.T) {
	dir := t.TempDir()
	modelerDir := filepath.Join(dir, "modeler")
	os.MkdirAll(modelerDir, 0755)
	bin := filepath.Join(modelerDir, mxbuildBinaryName())
	os.WriteFile(bin, []byte("fake"), 0755)

	result, err := resolveMxBuild(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != bin {
		t.Errorf("expected %s, got %s", bin, result)
	}
}

func TestResolveMxBuild_ExplicitDir_NoBinaryInside(t *testing.T) {
	dir := t.TempDir()
	_, err := resolveMxBuild(dir)
	if err == nil {
		t.Error("expected error for directory without mxbuild binary")
	}
}

func TestResolveMxBuild_ExplicitPathNotFound(t *testing.T) {
	_, err := resolveMxBuild("/nonexistent/mxbuild")
	if err == nil {
		t.Error("expected error for nonexistent explicit path")
	}
}

func TestResolveMxBuild_NoExplicitPath_FallsThrough(t *testing.T) {
	// Without mxbuild in PATH or known locations, this should error
	_, err := resolveMxBuild("")
	if err == nil {
		// It's possible mxbuild is actually installed; skip in that case
		t.Skip("mxbuild found on system")
	}
}

func TestIsJDK_InvalidPath(t *testing.T) {
	for _, major := range []int{21, 25} {
		if isJDK("/nonexistent/jdk", major) {
			t.Errorf("expected false for nonexistent path (major %d)", major)
		}
	}
}

func TestMxbuildSearchPaths_NonEmpty(t *testing.T) {
	paths := mxbuildSearchPaths()
	if len(paths) == 0 {
		t.Error("expected non-empty search paths")
	}
}

func TestJdkSearchPaths_NonEmpty(t *testing.T) {
	paths := jdkSearchPaths(21)
	if len(paths) == 0 {
		t.Error("expected non-empty search paths")
	}
}

// --- Platform-aware resolution tests ---

func TestWindowsProgramDirs_UsesEnvVars(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("windows-only test")
	}
	dirs := windowsProgramDirs()
	if len(dirs) == 0 {
		t.Fatal("expected at least one program directory")
	}
	// Every returned dir must be an absolute path
	for _, d := range dirs {
		if !filepath.IsAbs(d) {
			t.Errorf("expected absolute path, got %s", d)
		}
	}
}

func TestWindowsProgramDirs_NoDuplicates(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("windows-only test")
	}
	dirs := windowsProgramDirs()
	seen := map[string]bool{}
	for _, d := range dirs {
		if seen[d] {
			t.Errorf("duplicate directory: %s", d)
		}
		seen[d] = true
	}
}

func TestWindowsProgramDirs_SystemDriveFallback(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("windows-only test")
	}
	// Save and clear PROGRAMFILES vars, keep SystemDrive
	saved := map[string]string{}
	for _, env := range []string{"PROGRAMFILES", "PROGRAMW6432", "PROGRAMFILES(X86)"} {
		saved[env] = os.Getenv(env)
		t.Setenv(env, "")
	}
	sysDrive := os.Getenv("SystemDrive")
	if sysDrive == "" {
		t.Skip("SystemDrive not set")
	}

	dirs := windowsProgramDirs()
	if len(dirs) == 0 {
		t.Fatal("expected SystemDrive fallback to produce directories")
	}
	// Should contain paths derived from SystemDrive
	found := false
	for _, d := range dirs {
		if filepath.HasPrefix(d, sysDrive) {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected dirs to contain paths under %s, got %v", sysDrive, dirs)
	}
}

func TestMendixSearchPaths_NoHardcodedDriveLetter(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("windows-only test")
	}
	for _, binary := range []string{"mxbuild.exe", "mx.exe"} {
		paths := mendixSearchPaths(binary)
		for _, p := range paths {
			// Paths must NOT start with a hardcoded "C:\"
			if len(p) >= 3 && p[0] == 'C' && p[1] == ':' {
				sysDrive := os.Getenv("SystemDrive")
				if sysDrive != "" && sysDrive != "C:" {
					t.Errorf("mendixSearchPaths(%q) contains hardcoded C: path: %s (SystemDrive=%s)", binary, p, sysDrive)
				}
			}
		}
	}
}

func TestJdkSearchPaths_NoHardcodedDriveLetter(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("windows-only test")
	}
	paths := jdkSearchPaths(21)
	for _, p := range paths {
		if len(p) >= 3 && p[0] == 'C' && p[1] == ':' {
			sysDrive := os.Getenv("SystemDrive")
			if sysDrive != "" && sysDrive != "C:" {
				t.Errorf("jdkSearchPaths contains hardcoded C: path: %s (SystemDrive=%s)", p, sysDrive)
			}
		}
	}
}

func TestResolveStudioProDir_Linux(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("linux-only test")
	}
	if dir := ResolveStudioProDir("11.6.4"); dir != "" {
		t.Errorf("expected empty on linux, got %s", dir)
	}
}

func TestResolveStudioProDir_MacOS_FakeInstall(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("macOS-only test")
	}
	// Simulate a Studio Pro .app bundle in a temp Applications directory.
	// resolveStudioProDirMacOS is hard-coded to /Applications, so we test
	// it indirectly by confirming that a non-existent version returns "".
	if dir := resolveStudioProDirMacOS("0.0.0"); dir != "" {
		t.Errorf("expected empty for non-existent version, got %s", dir)
	}
}

func TestResolveStudioProDir_FakeInstall(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("windows-only test")
	}
	// Create a fake Studio Pro layout in a temp dir
	tmpDir := t.TempDir()
	fakeVersion := "99.0.0"
	modelerDir := filepath.Join(tmpDir, "Mendix", fakeVersion, "modeler")
	os.MkdirAll(modelerDir, 0755)
	os.WriteFile(filepath.Join(modelerDir, "mxbuild.exe"), []byte("fake"), 0755)

	// Point PROGRAMFILES to our temp dir
	t.Setenv("PROGRAMFILES", tmpDir)

	dir := ResolveStudioProDir(fakeVersion)
	expected := filepath.Join(tmpDir, "Mendix", fakeVersion)
	if dir != expected {
		t.Errorf("expected %s, got %s", expected, dir)
	}
}

func TestResolveStudioProDir_VersionNotInstalled(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("windows-only test")
	}
	dir := ResolveStudioProDir("99.99.99")
	if dir != "" {
		t.Errorf("expected empty for non-existent version, got %s", dir)
	}
}

func TestResolveMxBuild_PrefersStudioProOverCache(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("windows-only test")
	}
	// Create fake Studio Pro install
	tmpDir := t.TempDir()
	modelerDir := filepath.Join(tmpDir, "Mendix", "11.0.0", "modeler")
	os.MkdirAll(modelerDir, 0755)
	studioBin := filepath.Join(modelerDir, "mxbuild.exe")
	os.WriteFile(studioBin, []byte("studio"), 0755)

	// Create fake cached Linux binary
	cacheDir := t.TempDir()
	cacheBin := filepath.Join(cacheDir, "modeler", "mxbuild")
	os.MkdirAll(filepath.Dir(cacheBin), 0755)
	os.WriteFile(cacheBin, []byte("linux-elf"), 0755)

	// Override PROGRAMFILES so resolveMxBuild finds our fake Studio Pro
	t.Setenv("PROGRAMFILES", tmpDir)
	// Clear others to avoid interference
	t.Setenv("PROGRAMW6432", "")
	t.Setenv("PROGRAMFILES(X86)", "")

	result, err := resolveMxBuild("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != studioBin {
		t.Errorf("expected Studio Pro binary %s, got %s (should prefer Studio Pro over cache)", studioBin, result)
	}
}

func TestResolveMxBuild_PrefersExactCachedVersion(t *testing.T) {
	dir := t.TempDir()
	testutil.SetHome(t, dir)
	setTestApplicationsDir(t, t.TempDir()) // prevent real macOS Studio Pro from matching
	// Point PATH at an empty temp dir (rather than clearing it) so exec.LookPath
	// still works for any other testing infrastructure but can't find mxbuild.
	t.Setenv("PATH", t.TempDir())

	versions := []string{"9.24.40.80973", "11.6.3", "11.9.0"}
	var expected string
	for _, version := range versions {
		modelerDir := filepath.Join(dir, ".mxcli", "mxbuild", version, "modeler")
		if err := os.MkdirAll(modelerDir, 0755); err != nil {
			t.Fatal(err)
		}
		bin := filepath.Join(modelerDir, mxbuildBinaryName())
		if err := os.WriteFile(bin, []byte("fake"), 0755); err != nil {
			t.Fatal(err)
		}
		if version == "11.9.0" {
			expected = bin
		}
	}

	result, err := resolveMxBuild("", "11.9.0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != expected {
		t.Errorf("expected exact cached version %s, got %s", expected, result)
	}
}
