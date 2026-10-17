// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"os"
	"path/filepath"
	"regexp"
)

// mxDepsVersion finds the version of the mx assembly in the mx.deps.json that
// Studio Pro and the mxbuild download ship beside the binary: the target key is
// "mx/<version>" followed by a "\n" escape (measured on 10.24.9 and 11.12.5).
var mxDepsVersion = regexp.MustCompile(`"mx/(\d+\.\d+\.\d+(?:\.\d+)?)[^"]*"`)

// MxBinaryVersion reports the Mendix version of the mx binary at mxPath, or ""
// when it cannot be told.
//
// ResolveMxForVersion prefers an mx on PATH over the one for the project's
// version and falls back to any cached one, so the binary it returns is not
// necessarily the version asked for; callers that must not mix versions use this
// to find out. The binary's own mx.deps.json is read first, because it holds for
// any install layout (a PATH entry, a renamed folder). The folder name is the
// fallback — ~/.mxcli/mxbuild/<version>/modeler/mx and
// Program Files/Mendix/<version>/modeler/mx both carry it — and only when it is a
// dotted version of at least three parts. Symbolic links are followed first.
func MxBinaryVersion(mxPath string) string {
	if resolved, err := filepath.EvalSymlinks(mxPath); err == nil {
		mxPath = resolved
	}
	if data, err := os.ReadFile(filepath.Join(filepath.Dir(mxPath), "mx.deps.json")); err == nil {
		if m := mxDepsVersion.FindSubmatch(data); m != nil {
			return string(m[1])
		}
	}
	if v := versionFromPath(mxPath); isReleaseVersion(v) {
		return v
	}
	return ""
}

// SameMendixRelease reports whether two Mendix versions are the same release:
// major, minor and patch equal. The build number a Studio Pro folder or a project
// stamp may add ("10.24.9.81004") is not part of the release. An empty or
// malformed version matches nothing.
func SameMendixRelease(a, b string) bool {
	pa, okA := parseVersionParts(a)
	pb, okB := parseVersionParts(b)
	if !okA || !okB || len(pa) < 3 || len(pb) < 3 {
		return false
	}
	return pa[0] == pb[0] && pa[1] == pb[1] && pa[2] == pb[2]
}

func isReleaseVersion(v string) bool {
	parts, ok := parseVersionParts(v)
	return ok && len(parts) >= 3
}
