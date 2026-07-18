// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestResolveNodeTooling(t *testing.T) {
	cache := t.TempDir()
	toolsNode := filepath.Join(cache, "modeler", "tools", "node")
	// platform dir matching this host (that's what findNodeBinary prefers)
	exe := "node"
	if runtime.GOOS == "windows" {
		exe = "node.exe"
	}
	archAlias := map[string]string{"amd64": "x64", "arm64": "arm64", "386": "x86"}
	arch := archAlias[runtime.GOARCH]
	if arch == "" {
		arch = runtime.GOARCH
	}
	osAlias := map[string]string{"darwin": "darwin", "linux": "linux", "windows": "win"}
	goos := osAlias[runtime.GOOS]
	if goos == "" {
		goos = runtime.GOOS
	}
	platDir := filepath.Join(toolsNode, goos+"-"+arch)
	if err := os.MkdirAll(platDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(platDir, exe), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(toolsNode, "rollup-runner.mjs"), []byte("// runner\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	mxbuildPath := filepath.Join(cache, "modeler", "mxbuild")
	nodeBin, runner, err := resolveNodeTooling(mxbuildPath)
	if err != nil {
		t.Fatalf("resolveNodeTooling: %v", err)
	}
	if nodeBin != filepath.Join(platDir, exe) {
		t.Errorf("nodeBin = %q", nodeBin)
	}
	if runner != filepath.Join(toolsNode, "rollup-runner.mjs") {
		t.Errorf("runner = %q", runner)
	}
}

func TestResolveNodeTooling_GlobFallback(t *testing.T) {
	// A cache for a different platform: findNodeBinary falls back to any */node.
	cache := t.TempDir()
	toolsNode := filepath.Join(cache, "modeler", "tools", "node")
	other := filepath.Join(toolsNode, "some-other-plat")
	if err := os.MkdirAll(other, 0o755); err != nil {
		t.Fatal(err)
	}
	exe := "node"
	if runtime.GOOS == "windows" {
		exe = "node.exe"
	}
	_ = os.WriteFile(filepath.Join(other, exe), []byte("x"), 0o755)
	_ = os.WriteFile(filepath.Join(toolsNode, "rollup-runner.mjs"), []byte("x"), 0o644)

	nodeBin, _, err := resolveNodeTooling(filepath.Join(cache, "modeler", "mxbuild"))
	if err != nil {
		t.Fatalf("resolveNodeTooling: %v", err)
	}
	if nodeBin != filepath.Join(other, exe) {
		t.Errorf("nodeBin = %q, want glob fallback %q", nodeBin, filepath.Join(other, exe))
	}
}

func TestResolveNodeTooling_MissingRunner(t *testing.T) {
	cache := t.TempDir()
	_ = os.MkdirAll(filepath.Join(cache, "modeler", "tools", "node"), 0o755)
	if _, _, err := resolveNodeTooling(filepath.Join(cache, "modeler", "mxbuild")); err == nil {
		t.Error("expected error when rollup-runner.mjs is absent")
	}
}

func TestBuildWebClient_NoRollupConfig(t *testing.T) {
	// web/ without rollup.config.mjs -> clear error (serve build not run yet).
	dep := t.TempDir()
	_ = os.MkdirAll(filepath.Join(dep, "web"), 0o755)
	err := BuildWebClient(WebClientOptions{DeployDir: dep, MxBuildPath: "/x/modeler/mxbuild"})
	if err == nil {
		t.Error("expected error when rollup.config.mjs is missing")
	}
}
