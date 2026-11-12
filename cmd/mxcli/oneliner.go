// SPDX-License-Identifier: Apache-2.0

package main

import (
	"errors"
	"fmt"
	"github.com/mendixlabs/mxcli/cmd/mxcli/docker"
	"io"
	"os"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/executor"
	"github.com/mendixlabs/mxcli/mdl/langver"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/spf13/cobra"
)

// runCommandLine runs the statements of `mxcli -c` and returns the process exit
// code. Failures are written to errOut.
//
// The failure semantics are those of `mxcli exec` (mendixlabs/mxcli#1218):
//
//   - An empty or whitespace-only -c is an error. It used to fall through to the
//     interactive REPL, which a generator that produced an empty statement list
//     experienced as a hang.
//   - By default the run stops at the first failing statement, read-only or not
//     — a later statement may depend on an earlier one, and a script that
//     carries on past a failure can write on a wrong premise. The error says
//     which statement failed and how many were not run, so the stop is never
//     silent.
//   - --continue-on-error attempts every statement, reports each failure with
//     its statement number, and exits non-zero if any failed.
func runCommandLine(cmd *cobra.Command, commands, projectPath string, continueOnError bool, errOut io.Writer) int {
	if strings.TrimSpace(commands) == "" {
		fmt.Fprintln(errOut, "Error: -c was given no MDL to run. Pass one or more statements, "+
			"e.g. -c \"list modules;\"; run mxcli without -c for the interactive REPL.")
		return 1
	}

	// As in `exec`: a watching dev loop builds as soon as these statements
	// are done (ako/mxcli#2151). Deferred first, so it runs after exec.Close.
	defer docker.BeginModelWrite(projectPath)()

	exec, logger := newLoggedExecutor("batch")
	defer logger.Close()
	defer exec.Close()

	// Suppress status messages when stdout is a pipe so that
	// output can be piped directly to other tools (e.g. mxcli fmt).
	if fi, statErr := os.Stdout.Stat(); statErr == nil && (fi.Mode()&os.ModeCharDevice) == 0 {
		exec.SetQuiet(true)
	}

	// The one-liner's language (freeze decision 6): mdl 1 unless
	// --mdl 0, or a header the commands state themselves. It is what
	// headerless input is read as and what describe writes.
	lang := mdlFlag(cmd)
	if v, written := langver.ScanWrittenHeader(commands); written {
		lang = v
	}
	exec.SetDescribeLanguage(lang)

	// Auto-connect if project specified. CONNECT runs on its own so
	// that a header in the commands stays their first statement.
	if projectPath != "" {
		connectProg, _ := visitor.Build(fmt.Sprintf("CONNECT LOCAL '%s';", visitor.QuoteString(projectPath)))
		if err := exec.ExecuteProgram(connectProg); err != nil {
			fmt.Fprintf(errOut, "Error: %v\n", err)
			return 1
		}
	}

	prog, errs := visitor.BuildSession(commands, lang)
	if len(errs) > 0 {
		for _, err := range errs {
			fmt.Fprintf(errOut, "Parse error: %v\n", err)
		}
		return 1
	}

	if continueOnError {
		res, err := exec.ExecuteProgramContinueOnError(prog, errOut)
		if err != nil && !errors.Is(err, executor.ErrExit) {
			fmt.Fprintf(errOut, "Error: %v\n", err)
			return 1
		}
		if res.Failed > 0 {
			fmt.Fprintf(errOut, "%d statements: %d succeeded, %d failed\n", res.Total, res.Succeeded, res.Failed)
			return 1
		}
		return 0
	}

	stoppedAt, err := exec.ExecuteProgramReportingStop(prog)
	if err == nil || errors.Is(err, executor.ErrExit) {
		return 0
	}
	total := len(prog.Statements)
	if stoppedAt >= 0 && total > 1 {
		fmt.Fprintf(errOut, "Error: statement %d of %d: %v\n", stoppedAt+1, total, err)
		if skipped := total - stoppedAt - 1; skipped > 0 {
			fmt.Fprintf(errOut, "Stopped: %d later statement(s) not run. --continue-on-error runs every statement.\n", skipped)
		}
		return 1
	}
	fmt.Fprintf(errOut, "Error: %v\n", err)
	return 1
}
