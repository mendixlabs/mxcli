// SPDX-License-Identifier: Apache-2.0

//go:build unix

package main

import (
	"os"
	"runtime"
	"syscall"
)

// maxRSSMB is the process's peak resident memory in MiB.
func maxRSSMB(ps *os.ProcessState) float64 {
	ru, ok := ps.SysUsage().(*syscall.Rusage)
	if !ok {
		return 0
	}
	// ru_maxrss is kilobytes on Linux and bytes on macOS.
	if runtime.GOOS == "darwin" {
		return float64(ru.Maxrss) / (1 << 20)
	}
	return float64(ru.Maxrss) / (1 << 10)
}
