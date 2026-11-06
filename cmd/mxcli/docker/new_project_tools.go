// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

// ResolveNewProjectTools selects the exact installed pair requested by
// --mxbuild-path. An invalid selection must not fall back to another installation.
func ResolveNewProjectTools(explicitPath, version string) (mxPath, mxbuildPath string, err error) {
	if explicitPath == "" {
		return "", "", fmt.Errorf("--mxbuild-path must name an installed toolchain")
	}
	parts, ok := parseVersionParts(version)
	if !ok || (len(parts) != 3 && len(parts) != 4) {
		return "", "", fmt.Errorf("invalid Mendix version %q: expected three or four numeric parts", version)
	}
	// Preserve link/.. semantics before Abs or directory joins clean the path.
	resolvedPath, err := filepath.EvalSymlinks(explicitPath)
	if err != nil {
		return "", "", fmt.Errorf("resolve --mxbuild-path %s: %w", explicitPath, err)
	}
	mxbuildPath, err = resolveMxBuild(resolvedPath, version)
	if err != nil {
		return "", "", err
	}
	// Resolve aliases before deriving the sibling mx; both commands also need
	// absolute paths because they run from project directories.
	mxbuildPath, err = filepath.Abs(mxbuildPath)
	if err != nil {
		return "", "", err
	}
	mxbuildPath, err = filepath.EvalSymlinks(mxbuildPath)
	if err != nil {
		return "", "", err
	}
	mxPath = filepath.Join(filepath.Dir(mxbuildPath), mxBinaryName())
	for _, path := range []string{mxPath, mxbuildPath} {
		if _, err := exec.LookPath(path); err != nil {
			return "", "", fmt.Errorf("selected tool is not executable at %s: %w", path, err)
		}
		if err := verifyRunsHere(path); err != nil {
			return "", "", err
		}
	}
	actual := MxBinaryVersion(mxPath)
	if actual == "" {
		return "", "", fmt.Errorf("cannot determine Mendix version of selected mx at %s", mxPath)
	}
	if !SameMendixRelease(actual, version) ||
		(len(parts) == 4 && strings.Count(actual, ".") == 3 && actual != version) {
		return "", "", fmt.Errorf("selected mx at %s is Mendix %s, requested %s", mxPath, actual, version)
	}
	// Some installations publish only the release in mx.deps.json. A requested
	// build number is also checked against the project after create-project.
	return mxPath, mxbuildPath, nil
}
