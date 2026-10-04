// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"os"
	"testing"
)

// TestMain keeps every test in this package out of the developer's real
// ~/.mxcli/logs.
//
// A test that runs a command in-process (runCheckFiles, runCommandLine, ...)
// reaches diaglog.Init through newLoggedExecutorTo. Init appends session_start
// and session_end lines to ~/.mxcli/logs/mxcli-<date>.log and then runs
// cleanOldLogs on that directory, so on every OS a plain `go test` wrote into
// the developer's own diagnostics log and could delete their older log files.
//
// MXCLI_LOG_DIR is diaglog's own hook for this ("keeps a test out of the user's
// real log directory"), but only the two tests that spawn a subprocess set it.
// Setting it once here covers every in-process test, present and future,
// instead of each one having to remember a per-test Setenv. A developer who
// has pointed MXCLI_LOG_DIR somewhere on purpose keeps that choice.
//
// Child processes inherit the variable, which is what they want; the two
// per-test overrides (json_output_purity_test.go, session_start_test.go) give
// their child a directory of its own to assert on and stay as they are.
func TestMain(m *testing.M) {
	if os.Getenv("MXCLI_LOG_DIR") != "" {
		os.Exit(m.Run())
	}

	logDir, err := os.MkdirTemp("", "mxcli-test-logs-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "TestMain: cannot create a temporary log directory: %v\n", err)
		os.Exit(1)
	}
	if err := os.Setenv("MXCLI_LOG_DIR", logDir); err != nil {
		os.RemoveAll(logDir)
		fmt.Fprintf(os.Stderr, "TestMain: cannot set MXCLI_LOG_DIR: %v\n", err)
		os.Exit(1)
	}

	code := m.Run()
	// diaglog holds the day's log file open until the process ends, so on
	// Windows the removal can fail; a leftover temp directory is harmless.
	_ = os.RemoveAll(logDir)
	os.Exit(code)
}
