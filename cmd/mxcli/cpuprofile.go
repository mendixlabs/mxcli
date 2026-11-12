// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"os"
	"runtime/pprof"
)

// cpuProfileEnv names a file to write a CPU profile of this command to, for
// `go tool pprof`. Unset, nothing is profiled.
const cpuProfileEnv = "MXCLI_CPUPROFILE"

// startCPUProfile starts profiling when cpuProfileEnv is set and returns the
// function that stops it and closes the file.
func startCPUProfile() (stop func()) {
	path := os.Getenv(cpuProfileEnv)
	if path == "" {
		return func() {}
	}
	f, err := os.Create(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: %s: %v\n", cpuProfileEnv, err)
		return func() {}
	}
	if err := pprof.StartCPUProfile(f); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: %s: %v\n", cpuProfileEnv, err)
		f.Close()
		return func() {}
	}
	return func() {
		pprof.StopCPUProfile()
		f.Close()
	}
}
