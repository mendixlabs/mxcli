// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"errors"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"strings"
)

// settle.go runs one deploy build against a freshly created project so that the
// sources MxBuild generates are already in their post-build shape.
//
// mxcli-todo findings #7: the template ships `javascriptsource/*/actions/*.js`
// (and the matching Java stubs) in a slightly older shape, and MxBuild rewrites
// every one of them on the first build — banner, `import { Big } from "big.js"`,
// `async function X()` -> `export async function X()`. They are *tracked* files,
// so a fresh clone goes dirty on the first build (48 of them in a blank Mendix
// 11.12 app) and stays dirty until someone commits build output they did not
// write. `mx check` does not do this; only a build does. Doing that build while
// the project is still being created means the first commit already holds the
// settled form.

// SettleGeneratedSources runs `mxbuild --target=deploy` once against
// projectPath. It is best-effort by contract: every failure is returned for the
// caller to report as a warning, never as a reason to fail project creation.
//
// mxPath is the `mx` binary already resolved by the caller — mxbuild lives
// beside it — and version is used to find a cached download when it does not.
// An explicit mxbuildPath pins the build to that tool, without cache fallback.
func SettleGeneratedSources(projectPath, mxPath, mxbuildPath, version string, w io.Writer) error {
	if mxbuildPath == "" {
		mxbuildPath = resolveMxBuildForSettle(mxPath, version)
	}
	if mxbuildPath == "" {
		return fmt.Errorf("mxbuild not found next to %s or in the cache for %s", mxPath, version)
	}
	javaMajor, _ := ProjectJavaMajor(projectPath)
	javaHome, err := resolveJDK(javaMajor)
	if err != nil {
		return fmt.Errorf("no JDK %d available: %w", javaMajor, err)
	}

	cmd := settleCommand(mxbuildPath, javaHome, projectPath)

	// MxBuild is very chatty and this build is incidental to what the user asked
	// for, so its output is only worth showing when it fails.
	out := &syncBuffer{}
	cmd.Stdout = out
	cmd.Stderr = out
	if err := runWithWatchdog(cmd, mxbuildWatchdogLimits(), javaHome); err != nil {
		var werr *WatchdogError
		if errors.As(err, &werr) {
			return err // already carries the tail and the java home
		}
		return classifySettleFailure(err, out.String())
	}
	fmt.Fprintln(w, "  Generated sources are in their post-build form.")
	return nil
}

// settleCommand builds the one-shot deploy build SettleGeneratedSources runs
// (`mxcli new` step 6) — without a Gradle daemon, which hung this step the same
// way it hung the dependency sync (ako/mxcli#1340).
func settleCommand(mxbuildPath, javaHome, projectPath string) *exec.Cmd {
	cmd := exec.Command(mxbuildPath,
		"--target=deploy",
		fmt.Sprintf("--java-home=%s", javaHome),
		fmt.Sprintf("--java-exe-path=%s", JavaExePath(javaHome)),
		projectPath,
	)
	cmd.Dir = filepath.Dir(projectPath)
	prepareOneShotMxCommand(cmd) // FreeType LD_PRELOAD workaround + no Gradle daemon
	return cmd
}

// ModelErrorsError is a build mxbuild refused because the MODEL has errors —
// as opposed to one that could not run (no JDK, a killed JVM). The difference
// matters to `mxcli new`: the first means the project it just created does not
// build, which is a failed scaffold, not a warning (mendixlabs/mxcli#1433).
type ModelErrorsError struct {
	// Errors are mxbuild's "ERROR at <location>: <message>" lines.
	Errors []string
}

func (e *ModelErrorsError) Error() string {
	return fmt.Sprintf("mxbuild refused the project: %d model error(s):\n  %s",
		len(e.Errors), strings.Join(e.Errors, "\n  "))
}

// classifySettleFailure turns a failed settle build into an error, typed as a
// *ModelErrorsError when mxbuild's output lists model errors. mxbuild prints
// each as "ERROR at <location>: <message>" after "The project cannot be
// deployed, because it contains errors." (measured on 10.24.0).
func classifySettleFailure(runErr error, output string) error {
	var modelErrs []string
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "ERROR at ") {
			modelErrs = append(modelErrs, line)
		}
	}
	if len(modelErrs) > 0 {
		return &ModelErrorsError{Errors: modelErrs}
	}
	return fmt.Errorf("mxbuild failed: %w\n%s", runErr, lastLines(output, 20))
}

// resolveMxBuildForSettle finds mxbuild beside the resolved mx binary (the
// Studio Pro / CDN layout puts them in the same directory), falling back to the
// version's download cache.
func resolveMxBuildForSettle(mxPath, version string) string {
	if mxPath != "" {
		if found := findMxBuildInDir(filepath.Dir(mxPath)); found != "" {
			return found
		}
	}
	return CachedMxBuildPath(version)
}

// lastLines returns the final n lines of s — enough to show why a build failed
// without pasting the whole log into a creation summary.
func lastLines(s string, n int) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}
