// SPDX-License-Identifier: Apache-2.0

//go:build !windows

package procalive

import (
	"os"
	"syscall"
)

// Alive reports whether pid names a running process. On POSIX, signal 0 is
// delivered to no one but still performs the existence and permission checks,
// so it succeeds for a live process. It also succeeds for an unreaped zombie,
// which is why a caller that owns the child must reap it (cmd.Wait) before
// asking.
func Alive(pid int) bool {
	if pid <= 0 {
		return false
	}
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return p.Signal(syscall.Signal(0)) == nil
}
