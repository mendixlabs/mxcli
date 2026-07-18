// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"
)

// webclient.go builds the browser client bundle (web/dist/index.js) for a
// deployment. mxbuild's serve Deploy target writes the client *source*
// (web/index.js, web/pages/*, web/widgets/*) and a self-contained
// web/rollup.config.mjs, but it does NOT run the rollup step that bundles them
// into web/dist/. A standalone-served app therefore 404s on /dist/index.js and
// renders blank. This runs mxbuild's own bundled rollup runner to close that gap.
//
// The runner (modeler/tools/node/rollup-runner.mjs) with NODE_ENV=production does
// a one-shot rollup() + bundle.write(config.output) into web/dist. It resolves
// `rollup` from tools/node/node_modules (relative to the runner) and loads
// rollup.config.mjs from its working directory (the deployment web dir).

// WebClientOptions configures BuildWebClient.
type WebClientOptions struct {
	// DeployDir is the deployment directory; its web/ child holds the client
	// source and rollup.config.mjs, and receives web/dist/.
	DeployDir string
	// MxBuildPath is <cache>/modeler/mxbuild; the node tooling is resolved from
	// its sibling tools/node directory.
	MxBuildPath string
	// Timeout bounds the bundle build (default 5m).
	Timeout time.Duration
	// Stdout receives a short progress line (default discarded).
	Stdout io.Writer
}

// resolveNodeTooling returns the bundled node binary and rollup-runner.mjs paths
// from an mxbuild binary path (<cache>/modeler/mxbuild -> <cache>/modeler/tools/node).
func resolveNodeTooling(mxbuildPath string) (nodeBin, runner string, err error) {
	toolsNode := filepath.Join(filepath.Dir(mxbuildPath), "tools", "node")
	runner = filepath.Join(toolsNode, "rollup-runner.mjs")
	if _, err := os.Stat(runner); err != nil {
		return "", "", fmt.Errorf("rollup runner not found at %s (incomplete mxbuild?): %w", runner, err)
	}
	nodeBin = findNodeBinary(toolsNode)
	if nodeBin == "" {
		return "", "", fmt.Errorf("bundled node binary not found under %s", toolsNode)
	}
	return nodeBin, runner, nil
}

// findNodeBinary locates mxbuild's bundled node under tools/node/<platform>/.
// It prefers the GOOS/GOARCH-matched directory, then falls back to any platform
// dir containing a node binary (so a cache for one arch still resolves cleanly).
func findNodeBinary(toolsNode string) string {
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
	// Preferred exact match, then common alternates, then a glob fallback.
	candidates := []string{
		filepath.Join(toolsNode, goos+"-"+arch, exe),
		filepath.Join(toolsNode, exe),
	}
	for _, c := range candidates {
		if fi, err := os.Stat(c); err == nil && !fi.IsDir() {
			return c
		}
	}
	matches, _ := filepath.Glob(filepath.Join(toolsNode, "*", exe))
	for _, m := range matches {
		if fi, err := os.Stat(m); err == nil && !fi.IsDir() {
			return m
		}
	}
	return ""
}

// BuildWebClient bundles the deployment's browser client into web/dist. It is a
// no-op-safe prerequisite for serving a rendered app; call it after each serve
// Deploy build. Returns an error if the web dir, tooling, or rollup build fails.
func BuildWebClient(opts WebClientOptions) error {
	w := opts.Stdout
	if w == nil {
		w = io.Discard
	}
	webDir := filepath.Join(opts.DeployDir, "web")
	if fi, err := os.Stat(filepath.Join(webDir, "rollup.config.mjs")); err != nil || fi.IsDir() {
		return fmt.Errorf("no rollup.config.mjs in %s (run a serve Deploy build first)", webDir)
	}
	nodeBin, runner, err := resolveNodeTooling(opts.MxBuildPath)
	if err != nil {
		return err
	}
	timeout := opts.Timeout
	if timeout == 0 {
		timeout = 5 * time.Minute
	}

	start := time.Now()
	cmd := exec.Command(nodeBin, runner)
	cmd.Dir = webDir
	cmd.Env = append(os.Environ(),
		"NODE_ENV=production",
		"MX_WEB_CLIENT_BUILD_LOG="+filepath.Join(opts.DeployDir, "log", "web-client-build.log"),
	)
	log := &syncBuffer{}
	cmd.Stdout = log
	cmd.Stderr = log

	done := make(chan error, 1)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("launching web client build: %w", err)
	}
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		if err != nil {
			return fmt.Errorf("web client build failed: %w\n%s", err, log.String())
		}
	case <-time.After(timeout):
		_ = cmd.Process.Kill()
		<-done
		return fmt.Errorf("web client build timed out after %s", timeout)
	}

	dist := filepath.Join(webDir, "dist", "index.js")
	if _, err := os.Stat(dist); err != nil {
		return fmt.Errorf("web client build reported success but %s is missing:\n%s", dist, log.String())
	}
	fmt.Fprintf(w, "  Web client bundled in %s\n", time.Since(start).Round(time.Millisecond))
	return nil
}
