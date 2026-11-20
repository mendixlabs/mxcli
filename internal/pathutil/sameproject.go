// SPDX-License-Identifier: Apache-2.0

package pathutil

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// SameProject reports whether two project paths (.mpr files) name the same
// project directory.
//
// The state files under <projectDir>/.mxcli/ (run-state.json, run-local.json,
// test-endpoint.json) record the project they were written for, and a copy of
// the folder carries them along. A reader that trusts such a file by pid alone
// would refuse to start the copy, or signal the original's process. SameProject
// is the check that tells the two apart.
//
// Symlinks are resolved on both sides (macOS /var vs /private/var, a linked
// workspace), and when both directories exist they are compared by identity, so
// a case-insensitive volume cannot call one directory two. A path that cannot
// be resolved falls back to the cleaned path, compared case-insensitively on
// macOS and Windows. A recorded path that is empty comes from a file written
// before the field existed and is taken to be this project's.
func SameProject(recorded, current string) bool {
	if recorded == "" {
		return true
	}
	rd, cd := filepath.Dir(recorded), filepath.Dir(current)
	if ri, err := os.Stat(rd); err == nil {
		if ci, err := os.Stat(cd); err == nil && os.SameFile(ri, ci) {
			return true
		}
	}
	return foldPath(resolvePath(rd)) == foldPath(resolvePath(cd))
}

// resolvePath is the absolute, symlink-free, cleaned form of p where the file
// system can say, and the cleaned form otherwise.
func resolvePath(p string) string {
	if abs, err := filepath.Abs(p); err == nil {
		p = abs
	}
	if real, err := filepath.EvalSymlinks(p); err == nil {
		p = real
	}
	return filepath.Clean(p)
}

func foldPath(p string) string {
	if runtime.GOOS == "darwin" || runtime.GOOS == "windows" {
		return strings.ToLower(p)
	}
	return p
}
