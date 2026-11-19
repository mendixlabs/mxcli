// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// ako/mxcli#2360: `run status/stop/wait/restart` left through os.Exit, which
// skips runCLI's session close, so even a clean exit wrote no session_end and
// `diag loop-report` counted 189 `run wait` calls as vanished processes with
// 0.0s between them, while an hour of waiting went unmeasured.

func TestExitProcess_ClosesTheSessionWithTheCode_Issue2360(t *testing.T) {
	var closed, exited []int
	defer func(c func(int), e func(int)) { closeSession, osExit = c, e }(closeSession, osExit)
	closeSession = func(code int) { closed = append(closed, code) }
	osExit = func(code int) { exited = append(exited, code) }

	exitProcess(0)
	exitProcess(3)
	if len(closed) != 2 || closed[0] != 0 || closed[1] != 3 {
		t.Errorf("session closed with %v, want [0 3]", closed)
	}
	if len(exited) != 2 || exited[0] != 0 || exited[1] != 3 {
		t.Errorf("exited with %v, want [0 3]", exited)
	}
}

// The run commands are where an agent's loop spends its waiting time; none of
// them may leave without closing the session.
func TestRunCommandsDoNotCallOsExit_Issue2360(t *testing.T) {
	raw := regexp.MustCompile(`\bos\.Exit\(`)
	for _, f := range []string{"cmd_run.go", "cmd_run_lifecycle.go"} {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for i, line := range strings.Split(string(b), "\n") {
			if raw.MatchString(line) && !strings.HasPrefix(strings.TrimSpace(line), "//") {
				t.Errorf("%s:%d calls os.Exit, which skips the session close: use exitProcess\n\t%s", f, i+1, strings.TrimSpace(line))
			}
		}
	}
}
