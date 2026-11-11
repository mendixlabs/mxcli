// SPDX-License-Identifier: Apache-2.0

//go:build windows

package docker

import (
	"os"
	"syscall"
)

// openClientSource opens a file under deployment/web for reading without
// stopping mxbuild from deleting or replacing it meanwhile.
//
// os.Open shares a file for reading and writing but not for deletion, so a copy
// in progress made mxbuild's own cleanup of that file fail with "being used by
// another process" — the failure the web client bundler caused by holding
// web/ open (#1342). With FILE_SHARE_DELETE the delete goes through and the
// file disappears when this handle closes.
func openClientSource(path string) (*os.File, error) {
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	h, err := syscall.CreateFile(p, syscall.GENERIC_READ,
		syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE|syscall.FILE_SHARE_DELETE,
		nil, syscall.OPEN_EXISTING, syscall.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		return nil, &os.PathError{Op: "open", Path: path, Err: err}
	}
	return os.NewFile(uintptr(h), path), nil
}
