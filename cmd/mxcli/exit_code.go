// SPDX-License-Identifier: Apache-2.0

package main

import "errors"

// exitDecisionNeeded is the exit code of a command that stopped, without
// failing, on a decision a person must take: `git merge` on a model conflict
// the engine leaves open, or on text conflicts left for manual resolution
// (ako/mxcli#2240). It is not 2 on purpose: across mxcli, 2 is a usage error
// (a refused --json, a bad --v1 value, conflicting flags), and a script must
// be able to tell "you called it wrong" from "someone has to decide".
const exitDecisionNeeded = 3

// exitCodeError carries the process exit code of a command error that is not
// a plain failure. runCLI reads it, so the run still closes its diagnostics
// session (os.Exit from inside a command skips that).
type exitCodeError struct {
	code int
	err  error
}

func (e *exitCodeError) Error() string { return e.err.Error() }
func (e *exitCodeError) Unwrap() error { return e.err }

// decisionNeeded marks err as a stop for a human decision (exit 3).
func decisionNeeded(err error) error {
	return &exitCodeError{code: exitDecisionNeeded, err: err}
}

// exitCodeFor is the process exit code for a command's error: 0 for none, the
// carried code for an exitCodeError, 1 for anything else.
func exitCodeFor(err error) int {
	if err == nil {
		return 0
	}
	var e *exitCodeError
	if errors.As(err, &e) {
		return e.code
	}
	return 1
}
