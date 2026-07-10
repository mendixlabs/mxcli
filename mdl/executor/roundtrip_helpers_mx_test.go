// SPDX-License-Identifier: Apache-2.0

//go:build integration

package executor

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/cmd/mxcli/docker"
)

func TestNewestVersionedPath_PicksNewestNumericVersion(t *testing.T) {
	t.Parallel()

	paths := []string{
		"/tmp/.mxcli/mxbuild/11.9.0/modeler/mx",
		"/tmp/.mxcli/mxbuild/11.6.3/modeler/mx",
		"/tmp/.mxcli/mxbuild/9.24.40.80973/modeler/mx",
	}

	got := docker.NewestVersionedPath(paths)
	want := "/tmp/.mxcli/mxbuild/11.9.0/modeler/mx"
	if got != want {
		t.Fatalf("docker.NewestVersionedPath() = %q, want %q", got, want)
	}
}

func TestFindMxBinary_PrefersPathOverCachedDownloads(t *testing.T) {
	home := t.TempDir()
	pathDir := filepath.Join(home, "bin")
	if err := os.MkdirAll(pathDir, 0755); err != nil {
		t.Fatalf("mkdir path dir: %v", err)
	}

	pathMx := filepath.Join(pathDir, "mx")
	if err := os.WriteFile(pathMx, []byte("#!/bin/sh\nexit 0\n"), 0755); err != nil {
		t.Fatalf("write PATH mx: %v", err)
	}

	for _, version := range []string{"11.9.0", "11.6.3", "9.24.40.80973"} {
		cacheMx := filepath.Join(home, ".mxcli", "mxbuild", version, "modeler", "mx")
		if err := os.MkdirAll(filepath.Dir(cacheMx), 0755); err != nil {
			t.Fatalf("mkdir cache dir: %v", err)
		}
		if err := os.WriteFile(cacheMx, []byte("#!/bin/sh\nexit 0\n"), 0755); err != nil {
			t.Fatalf("write cached mx: %v", err)
		}
	}

	t.Setenv("HOME", home)
	t.Setenv("PATH", pathDir)
	t.Setenv("MX_BINARY", "")

	if got := findMxBinary(); got != pathMx {
		t.Fatalf("findMxBinary() = %q, want PATH binary %q", got, pathMx)
	}
}
