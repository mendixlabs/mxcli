// SPDX-License-Identifier: Apache-2.0

//go:build !windows

package docker

import "os"

// linkDir makes link a directory link to target. On POSIX systems that is a
// plain symlink; the Windows variant (linkdir_windows.go) has to cope with the
// symlink privilege, which is why this is a build-tagged pair.
func linkDir(target, link string) error {
	return os.Symlink(target, link)
}
