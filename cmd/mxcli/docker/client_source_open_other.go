// SPDX-License-Identifier: Apache-2.0

//go:build !windows

package docker

import "os"

// openClientSource opens a file under deployment/web for reading. On POSIX an
// open file does not stop anyone deleting it; see the Windows build.
func openClientSource(path string) (*os.File, error) { return os.Open(path) }
