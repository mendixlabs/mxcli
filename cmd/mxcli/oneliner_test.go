// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/internal/testutil"
)

// runOneLinerForTest runs `mxcli -c <commands>` without a project and returns
// the exit code and everything written to the error stream.
func runOneLinerForTest(t *testing.T, commands string, continueOnError bool) (int, string, string) {
	t.Helper()
	testutil.SetHome(t, t.TempDir()) // the session log is not the developer's
	var errOut bytes.Buffer
	var code int
	out, _ := captureStdout(t, func() error {
		code = runCommandLine(rootCmd, commands, "", continueOnError, &errOut)
		return nil
	})
	return code, out, errOut.String()
}

// mendixlabs/mxcli#1218 (1): `-c ""` fell through to the interactive REPL,
// which hangs a generator that spawned mxcli with an open stdin. Empty or
// whitespace-only input is an error, and nothing waits on stdin.
func TestOneLinerEmptyIsAnError(t *testing.T) {
	for _, c := range []string{"", "   ", "\n\t"} {
		code, _, errOut := runOneLinerForTest(t, c, false)
		if code == 0 {
			t.Errorf("-c %q: exit 0, want an error", c)
		}
		if !strings.Contains(errOut, "-c was given no MDL") {
			t.Errorf("-c %q: error output %q does not say the command was empty", c, errOut)
		}
	}
}

// mendixlabs/mxcli#1218 (2): a failing statement in a ;-separated -c stopped
// the run with nothing saying the later statements were skipped. The default
// stays fail-fast (as exec) but the stop is reported with the statement's
// position; --continue-on-error runs the rest.
//
// No project: `describe entity` fails (not connected) while `show features for
// version` needs none, so statement 3 is observable.
func TestOneLinerFailureSemantics(t *testing.T) {
	const batch = "show features for version 10.0; describe entity String; show features for version 11.0;"

	code, out, errOut := runOneLinerForTest(t, batch, false)
	if code == 0 {
		t.Fatalf("exit 0 with a failing statement; stderr %q", errOut)
	}
	if !strings.Contains(out, "Features for Mendix 10.0") {
		t.Errorf("statement 1 did not run (control): %q", out)
	}
	if strings.Contains(out, "Features for Mendix 11.0") {
		t.Errorf("fail-fast ran statement 3 after statement 2 failed")
	}
	for _, want := range []string{"statement 2 of 3", "1 later statement(s) not run", "--continue-on-error"} {
		if !strings.Contains(errOut, want) {
			t.Errorf("stderr %q does not contain %q", errOut, want)
		}
	}

	code, out, errOut = runOneLinerForTest(t, batch, true)
	if code == 0 {
		t.Errorf("--continue-on-error: exit 0 although statement 2 failed")
	}
	if !strings.Contains(out, "Features for Mendix 11.0") {
		t.Errorf("--continue-on-error did not run statement 3: %q", out)
	}
	if !strings.Contains(errOut, "statement 2:") || !strings.Contains(errOut, "1 failed") {
		t.Errorf("--continue-on-error stderr %q does not report statement 2 and the tally", errOut)
	}
}
