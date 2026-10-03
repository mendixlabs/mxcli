// SPDX-License-Identifier: Apache-2.0

//go:build windows

package procalive

import "syscall"

// Alive reports whether pid names a running process.
//
// os.Process.Signal(syscall.Signal(0)) is NOT a liveness test on Windows: Go
// returns EWINDOWS ("not supported by windows") for every signal except Kill, so
// it reports a live process as dead (mendixlabs/mxcli#1284). WaitForSingleObject
// on a SYNCHRONIZE handle is the real check: it returns WAIT_TIMEOUT while the
// process runs and WAIT_OBJECT_0 once it has terminated (even before it is
// reaped). A pid that cannot be opened (no such process, or access denied) reads
// as not alive.
func Alive(pid int) bool {
	if pid <= 0 {
		return false
	}
	h, err := syscall.OpenProcess(syscall.SYNCHRONIZE, false, uint32(pid))
	if err != nil {
		return false
	}
	defer syscall.CloseHandle(h)
	event, err := syscall.WaitForSingleObject(h, 0)
	if err != nil {
		return false
	}
	return event == syscall.WAIT_TIMEOUT
}
