// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/cmd/mxcli/docker"
)

// ako/mxcli#2028: installing MCP Client 4.4.0 headless copied its three managed
// Java dependency declarations and downloaded none of them (measured on a fresh
// 11.12.6 app: `sync-java-deps --check` reported 3 of 3 missing). mx check
// passed; the build failed compiling the module's Java actions. The install now
// runs the sync Studio Pro would have run. These tests stub the read and the
// sync: no mx, no Maven.

var mcpClientJars = []docker.JarDependencyRef{
	{Group: "io.modelcontextprotocol.sdk", Artifact: "mcp", Version: "2.0.0"},
	{Group: "com.fasterxml.jackson.core", Artifact: "jackson-databind", Version: "[2.21.2,3)"},
}

func stubJarSync(t *testing.T, deps []docker.JarDependencyRef, sync func(dir string) error) (mpr string, calls *int) {
	t.Helper()
	dir := t.TempDir()
	mpr = filepath.Join(dir, "App.mpr")
	n := 0
	origRead, origSync := postInstallDeclaredJars, postInstallSyncJars
	postInstallDeclaredJars = func(string) ([]docker.JarDependencyRef, string, error) { return deps, "11.12.6", nil }
	postInstallSyncJars = func(projectPath, mxPath, version string, w io.Writer) error {
		n++
		return sync(filepath.Dir(projectPath))
	}
	t.Cleanup(func() { postInstallDeclaredJars, postInstallSyncJars = origRead, origSync })
	return mpr, &n
}

func vendor(dir string, jars ...string) error {
	if err := os.MkdirAll(filepath.Join(dir, "vendorlib"), 0o755); err != nil {
		return err
	}
	for _, j := range jars {
		if err := os.WriteFile(filepath.Join(dir, "vendorlib", j), []byte("jar"), 0o644); err != nil {
			return err
		}
	}
	return nil
}

func TestInstallSyncsDeclaredJarDependencies(t *testing.T) {
	mpr, calls := stubJarSync(t, mcpClientJars, func(dir string) error {
		return vendor(dir, "mcp-2.0.0.jar", "jackson-databind-2.23.0.jar")
	})
	var out bytes.Buffer
	syncInstalledJarDependencies(&out, mpr)
	if *calls != 1 {
		t.Fatalf("sync ran %d time(s), want 1\n%s", *calls, out.String())
	}
	if s := out.String(); !strings.Contains(s, "2 downloaded into vendorlib/") || strings.Contains(s, "Warning") {
		t.Errorf("want a clean download report, got:\n%s", s)
	}

	// Control: everything already vendored, nothing to run.
	out.Reset()
	syncInstalledJarDependencies(&out, mpr)
	if *calls != 1 || out.Len() != 0 {
		t.Errorf("all vendored: sync ran again (%d) or printed:\n%s", *calls, out.String())
	}
}

func TestInstallJarSyncFailureNamesTheRetry(t *testing.T) {
	mpr, _ := stubJarSync(t, mcpClientJars, func(string) error {
		return errors.New("Failed to find some hosts")
	})
	var out bytes.Buffer
	syncInstalledJarDependencies(&out, mpr)
	s := out.String()
	for _, want := range []string{"Warning", "package … does not exist", "mxcli sync-java-deps -p " + mpr} {
		if !strings.Contains(s, want) {
			t.Errorf("output lacks %q:\n%s", want, s)
		}
	}
}

// A module that declares nothing: no sync, no output.
func TestInstallWithoutJarDependenciesIsQuiet(t *testing.T) {
	mpr, calls := stubJarSync(t, nil, func(string) error { return nil })
	var out bytes.Buffer
	syncInstalledJarDependencies(&out, mpr)
	if *calls != 0 || out.Len() != 0 {
		t.Errorf("sync ran %d time(s), printed:\n%s", *calls, out.String())
	}
}
