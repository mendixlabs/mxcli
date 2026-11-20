// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/mendixlabs/mxcli/cmd/mxcli/docker"
)

// marketplace_javadeps.go resolves the managed Java dependencies a module
// install or update leaves undownloaded (ako/mxcli#2028).
//
// A Marketplace module can declare managed Java dependencies — MCP Client 4.4.0
// declares io.modelcontextprotocol.sdk:mcp:2.0.0, mcp-json-jackson2 and
// jackson-databind. Studio Pro downloads them into vendorlib/ when it imports
// the module; a headless install copied the declarations and nothing else, so
// `mx check` passed and the next build failed with some 180 "package … does
// not exist" javac errors. The sync is the step Studio Pro would have run.

// Seams for tests: reading the declared dependencies and running the sync.
var (
	postInstallDeclaredJars = declaredJarDependencies
	postInstallSyncJars     = docker.SyncJavaDependencies
)

// syncInstalledJarDependencies downloads the declared managed Java
// dependencies that are not in vendorlib/. Best-effort, like the other
// post-install repairs: the install succeeded, so a failure is a warning that
// names the command to run.
func syncInstalledJarDependencies(out io.Writer, mprPath string) {
	deps, version, err := postInstallDeclaredJars(mprPath)
	if err != nil {
		fmt.Fprintf(out, "\n  Warning: could not read the project's managed Java dependencies: %v\n", err)
		fmt.Fprintf(out, "  Check with: mxcli sync-java-deps -p %s --check\n", mprPath)
		return
	}
	dir := filepath.Dir(mprPath)
	missing := docker.UnvendoredJarDependencies(dir, deps)
	if len(missing) == 0 {
		return
	}
	fmt.Fprintf(out, "\n  Downloading %d managed Java dependency/dependencies into vendorlib/ (%s)...\n",
		len(missing), strings.Join(missing, ", "))
	err = postInstallSyncJars(mprPath, "", version, out)
	if err == nil {
		if still := docker.UnvendoredJarDependencies(dir, deps); len(still) > 0 {
			err = fmt.Errorf("the sync finished but %s is still missing from vendorlib/", strings.Join(docker.DescribeUnvendored(dir, still), ", "))
		}
	}
	if err != nil {
		fmt.Fprintf(out, "  Warning: the managed Java dependencies were not downloaded: %v\n", err)
		fmt.Fprintln(out, "  Until they are, building the app fails compiling the module's Java actions (\"package … does not exist\").")
		fmt.Fprintf(out, "  Retry with: mxcli sync-java-deps -p %s\n", mprPath)
		return
	}
	fmt.Fprintf(out, "    managed Java dependencies: %d downloaded into vendorlib/\n", len(missing))
}
