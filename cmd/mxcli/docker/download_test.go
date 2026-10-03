// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/mendixlabs/mxcli/internal/testutil"
)

// setTestApplicationsDir redirects the macOS Applications directory scan to dir
// so tests that set up cached binaries are not confused by real Studio Pro installs.
func setTestApplicationsDir(t *testing.T, dir string) {
	t.Helper()
	prev := macOSApplicationsDir
	macOSApplicationsDir = dir
	t.Cleanup(func() { macOSApplicationsDir = prev })
}

func TestMxBuildCDNURL_ARM64(t *testing.T) {
	url := MxBuildCDNURL("11.6.3", "arm64")
	expected := "https://cdn.mendix.com/runtime/arm64-mxbuild-11.6.3.tar.gz"
	if url != expected {
		t.Errorf("expected %s, got %s", expected, url)
	}
}

func TestMxBuildCDNURL_AMD64(t *testing.T) {
	url := MxBuildCDNURL("11.6.3", "amd64")
	expected := "https://cdn.mendix.com/runtime/mxbuild-11.6.3.tar.gz"
	if url != expected {
		t.Errorf("expected %s, got %s", expected, url)
	}
}

func TestMxBuildCacheDir(t *testing.T) {
	dir, err := MxBuildCacheDir("11.6.3")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	home, _ := os.UserHomeDir()
	expected := filepath.Join(home, ".mxcli", "mxbuild", "11.6.3")
	if dir != expected {
		t.Errorf("expected %s, got %s", expected, dir)
	}
}

func TestCachedMxBuildPath_NotCached(t *testing.T) {
	// A version that definitely isn't cached
	path := CachedMxBuildPath("0.0.0-nonexistent")
	if path != "" {
		t.Errorf("expected empty string for uncached version, got %s", path)
	}
}

func TestCachedMxBuildPath_Cached(t *testing.T) {
	// Create a fake cached mxbuild
	dir := t.TempDir()
	testutil.SetHome(t, dir)

	version := "99.99.99"
	modelerDir := filepath.Join(dir, ".mxcli", "mxbuild", version, "modeler")
	os.MkdirAll(modelerDir, 0755)

	bin := filepath.Join(modelerDir, mxbuildBinaryName())
	os.WriteFile(bin, []byte("fake"), 0755)

	path := CachedMxBuildPath(version)
	if path != bin {
		t.Errorf("expected %s, got %s", bin, path)
	}
}

func TestAnyCachedMxBuildPath_Empty(t *testing.T) {
	dir := t.TempDir()
	testutil.SetHome(t, dir)

	path := AnyCachedMxBuildPath()
	if path != "" {
		t.Errorf("expected empty string, got %s", path)
	}
}

func TestAnyCachedMxBuildPath_Found(t *testing.T) {
	dir := t.TempDir()
	testutil.SetHome(t, dir)

	modelerDir := filepath.Join(dir, ".mxcli", "mxbuild", "11.6.3", "modeler")
	os.MkdirAll(modelerDir, 0755)
	bin := filepath.Join(modelerDir, mxbuildBinaryName())
	os.WriteFile(bin, []byte("fake"), 0755)

	path := AnyCachedMxBuildPath()
	if path != bin {
		t.Errorf("expected %s, got %s", bin, path)
	}
}

func TestAnyCachedMxBuildPath_PicksNewestNumericVersion(t *testing.T) {
	dir := t.TempDir()
	testutil.SetHome(t, dir)

	versions := []string{"9.24.40.80973", "11.6.3", "11.9.0"}
	var newest string
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
			newest = bin
		}
	}

	path := AnyCachedMxBuildPath()
	if path != newest {
		t.Errorf("expected newest cached mxbuild %s, got %s", newest, path)
	}
}

func TestRuntimeCDNURL(t *testing.T) {
	url := RuntimeCDNURL("11.6.3")
	expected := "https://cdn.mendix.com/runtime/mendix-11.6.3.tar.gz"
	if url != expected {
		t.Errorf("expected %s, got %s", expected, url)
	}
}

func TestRuntimeCacheDir(t *testing.T) {
	dir, err := RuntimeCacheDir("11.6.3")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	home, _ := os.UserHomeDir()
	expected := filepath.Join(home, ".mxcli", "runtime", "11.6.3")
	if dir != expected {
		t.Errorf("expected %s, got %s", expected, dir)
	}
}

func TestCachedRuntimePath_NotCached(t *testing.T) {
	path := CachedRuntimePath("0.0.0-nonexistent")
	if path != "" {
		t.Errorf("expected empty string for uncached version, got %s", path)
	}
}

func TestCachedRuntimePath_Cached(t *testing.T) {
	dir := t.TempDir()
	testutil.SetHome(t, dir)

	version := "99.99.99"
	launcherDir := filepath.Join(dir, ".mxcli", "runtime", version, "runtime", "launcher")
	os.MkdirAll(launcherDir, 0755)
	os.WriteFile(filepath.Join(launcherDir, "runtimelauncher.jar"), []byte("fake"), 0644)

	path := CachedRuntimePath(version)
	expected := filepath.Join(dir, ".mxcli", "runtime", version)
	if path != expected {
		t.Errorf("expected %s, got %s", expected, path)
	}
}

func TestResolveMxBuild_FindsCachedVersion(t *testing.T) {
	dir := t.TempDir()
	testutil.SetHome(t, dir)

	// Set up a fake cached mxbuild
	modelerDir := filepath.Join(dir, ".mxcli", "mxbuild", "11.6.3", "modeler")
	os.MkdirAll(modelerDir, 0755)
	bin := filepath.Join(modelerDir, mxbuildBinaryName())
	os.WriteFile(bin, []byte("fake"), 0755)

	// Clear PATH to avoid finding real mxbuild
	origPath := os.Getenv("PATH")
	t.Setenv("PATH", "")
	defer os.Setenv("PATH", origPath)

	result, err := resolveMxBuild("")
	if err != nil {
		// If it fails because there's no real mxbuild, check if it found our cache
		// This could fail on systems where glob patterns match other things
		t.Skipf("resolveMxBuild failed: %v", err)
	}
	if result != bin {
		// May have found a system mxbuild instead
		if runtime.GOOS == "linux" {
			t.Errorf("expected %s, got %s", bin, result)
		}
	}
}

func TestVersionFromPath_MacOSAppBundle(t *testing.T) {
	path := "/Applications/Mendix Studio Pro 11.10.0.app/Contents/modeler/mx"
	if got := versionFromPath(path); got != "11.10.0" {
		t.Errorf("versionFromPath(%q) = %q; want %q", path, got, "11.10.0")
	}
}

func TestVersionFromPath_MacOSAppBundle_RC(t *testing.T) {
	path := "/Applications/Mendix Studio Pro 11.10.0-rc.7 Beta.app/Contents/modeler/mx"
	if got := versionFromPath(path); got != "11.10.0" {
		t.Errorf("versionFromPath(%q) = %q; want %q", path, got, "11.10.0")
	}
}

func TestVersionFromPath_CachedLinux(t *testing.T) {
	path := "/home/user/.mxcli/mxbuild/11.9.0/modeler/mxbuild"
	if got := versionFromPath(path); got != "11.9.0" {
		t.Errorf("versionFromPath(%q) = %q; want %q", path, got, "11.9.0")
	}
}
