// SPDX-License-Identifier: Apache-2.0

package main

import (
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"

	"github.com/mendixlabs/mxcli/internal/projectlock"
	"github.com/mendixlabs/mxcli/mdl/ast"
)

// exitProjectBusy is the exit code of a run that gave up waiting for another
// mxcli process to finish writing the project: EX_TEMPFAIL, "try again
// later", so a script or an agent can tell it from a failed statement
// (ako/mxcli#2326).
const exitProjectBusy = 75

// readOnlyStatementPrefixes are the statement types that never write the
// model: a program of only these runs without the project's write lock, so
// agents reading one project in parallel do not queue behind each other.
var readOnlyStatementPrefixes = []string{
	"Show", "Describe", "Select", "Search", "Help", "Exit", "Status", "Lint",
	"Connect", "Disconnect", "Set", "Refresh", "SQLConnect", "SQLDisconnect",
	"SQLConnections", "SQLQuery", "SQLShow", "SQLDescribe",
}

// programWritesModel reports whether prog has a statement that may write the
// project. Unknown statement types count as writes: an extra wait is a cost,
// a missing lock is the failure this guards against.
func programWritesModel(prog *ast.Program) bool {
	if prog == nil {
		return false
	}
	for _, s := range prog.Statements {
		if s == nil {
			continue
		}
		name := reflect.TypeOf(s).String()
		name = name[strings.LastIndexByte(name, '.')+1:]
		readOnly := false
		for _, p := range readOnlyStatementPrefixes {
			if strings.HasPrefix(name, p) {
				readOnly = true
				break
			}
		}
		if !readOnly {
			return true
		}
	}
	return false
}

// acquireProjectWriteLock takes the project's write lock for a run that
// writes it, waiting for another mxcli process that holds it. It returns the
// release, or the exit code to stop with (and has said why on errOut).
func acquireProjectWriteLock(projectPath, purpose string, errOut io.Writer) (release func(), exitCode int) {
	release, err := projectlock.Acquire(projectPath, projectlock.Options{Purpose: purpose})
	if err == nil {
		return release, 0
	}
	var busy *projectlock.BusyError
	if errors.As(err, &busy) {
		fmt.Fprintf(errOut, "Error: %v\nNothing was written.\n", err)
		return nil, exitProjectBusy
	}
	fmt.Fprintf(errOut, "Error: take the project's write lock: %v\n", err)
	return nil, 1
}
