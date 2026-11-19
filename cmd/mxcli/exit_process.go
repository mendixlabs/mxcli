// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"

	"github.com/mendixlabs/mxcli/mdl/diaglog"
)

// exitProcess ends the process with code after closing the diagnostic
// session, which records the run's duration and exit code. A bare os.Exit
// skips runCLI's close, so `diag loop-report` counted the run as a vanished
// process with no time at all — every `run status/stop/wait/restart`, clean
// exits included (ako/mxcli#2360).
func exitProcess(code int) {
	closeSession(code)
	osExit(code)
}

// Seams for tests.
var (
	closeSession = diaglog.CloseCurrentWithExit
	osExit       = os.Exit
)
