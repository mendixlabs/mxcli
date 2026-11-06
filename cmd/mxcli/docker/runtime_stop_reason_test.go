// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ako/mxcli#1542: "One of the two times, the runtime had stopped itself on the
// trial licence's time limit (`LicenseUtil.checkRuntimeDuration`)" — and the
// callers got no reason.
//
// The licence message is an APPLICATION log line. A standalone runtime has no
// log subscriber until mxcli attaches the file subscriber that writes
// .mxcli/runtime.log, so the line lands in that file and never on the JVM's
// stdout — which is all LocalRuntime.Log() holds. Reading the reason from
// Log() alone (§60) worked in its unit test, on a string, and never in a run.

// licenceStopLog is a runtime.log across two boots: the first stopped on the
// licence, the second (the run in question) too, hours later.
const licenceStopLog = "\n=== runtime start 2026-10-26T10:00:00Z ===\n" +
	"Picked up JAVA_TOOL_OPTIONS\n" +
	"2026-10-26 10:00:20.101 WARNING - LicenseService: The runtime has been started using a trial licence,\n" +
	"          the framework will be terminated when the maximum time is exceeded!\n" +
	"\n=== runtime start 2026-10-26T14:51:02Z ===\n" +
	"Picked up JAVA_TOOL_OPTIONS\n" +
	"2026-10-26 14:51:20.101 WARNING - LicenseService: The runtime has been started using a trial licence,\n" +
	"          the framework will be terminated when the maximum time is exceeded!\n" +
	"2026-10-26 16:02:11.007 INFO - Core: an ordinary line\n" +
	"2026-10-26 18:43:16.412 CRITICAL - LicenseService: Maximum run time exceeded, framework is now terminating\n" +
	"\tat com.mendix.basis.license.LicenseUtil.checkRuntimeDuration(LicenseUtil.java:212)\n" +
	"2026-10-26 18:43:16.530 INFO - Core: Mendix Runtime is shutting down\n"

func writeRuntimeLog(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), ".mxcli", "runtime.log")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestRuntimeStoppedError_ReadsTheReasonFromTheRuntimeLog(t *testing.T) {
	logPath := writeRuntimeLog(t, licenceStopLog)
	// What the JVM itself printed: the banner, and nothing the app logged.
	mem := &syncBuffer{}
	_, _ = mem.Write([]byte("Picked up JAVA_TOOL_OPTIONS\nMendix runtime launcher started\n"))
	rt := &LocalRuntime{log: mem, opts: LocalRuntimeOptions{RuntimeLogPath: logPath}}

	err := runtimeStoppedError(rt, false)
	msg := err.Error()
	if !strings.Contains(strings.ToLower(msg), "licence") {
		t.Fatalf("the runtime log says the trial licence ended the run, but the error does not:\n%s", msg)
	}
	stopped, ok := err.(*RuntimeStoppedError)
	if !ok {
		t.Fatalf("err = %T, want *RuntimeStoppedError", err)
	}
	if want := "trial licence time limit reached at 18:43:16 (runtime.log)"; stopped.Reason != want {
		t.Errorf("Reason = %q, want %q", stopped.Reason, want)
	}
}

func TestRuntimeStoppedError_PasswordInputGuidance(t *testing.T) {
	for _, tc := range []struct {
		name  string
		stdin bool
	}{{"legacy", false}, {"stdin", true}} {
		t.Run(tc.name, func(t *testing.T) {
			rt := &LocalRuntime{log: &syncBuffer{}, exited: make(chan struct{}),
				opts: LocalRuntimeOptions{RuntimeLogPath: writeRuntimeLog(t, licenceStopLog)}}
			close(rt.exited)
			err := watchAndApply(LocalRunOptions{DBPasswordStdin: tc.stdin, PollInterval: time.Hour, Stdout: io.Discard},
				nil, rt, nil, "", time.Time{})
			stopped, ok := err.(*RuntimeStoppedError)
			if !ok || stopped.Reason != "trial licence time limit reached at 18:43:16 (runtime.log)" {
				t.Fatal("runtime exit lost its typed stop reason")
			}
			message := err.Error()
			if !tc.stdin {
				if !strings.HasSuffix(message, "Restart it: mxcli run restart -p <app.mpr>, or re-run.") {
					t.Fatal("legacy runtime exit guidance changed")
				}
				return
			}
			for _, want := range []string{"new foreground run", "--db-password-stdin", "fresh stdin"} {
				if !strings.Contains(message, want) {
					t.Error("runtime exit did not explain how to restart with fresh private input")
				}
			}
			if strings.Contains(message, "mxcli run restart") {
				t.Error("runtime exit recommends unsupported private-input replay")
			}
		})
	}
}

func TestRuntimeLogStopReason(t *testing.T) {
	cases := []struct{ name, log, want string }{
		{"licence, timed from the line above the stack frame", licenceStopLog,
			"trial licence time limit reached at 18:43:16 (runtime.log)"},
		// Only the newest boot counts: the file is appended across restarts.
		{"an earlier boot's licence stop is not this one's",
			licenceStopLog + "\n=== runtime start 2026-10-26T19:00:00Z ===\n2026-10-26 19:00:05.000 INFO - Core: started\n", ""},
		// The boot warning is printed at every start; it is not a stop.
		{"boot warning is not a stop",
			"\n=== runtime start 2026-10-26T10:00:00Z ===\nWARNING - LicenseService: the framework will be terminated when the maximum time is exceeded!\n", ""},
		{"out of memory",
			"\n=== runtime start x ===\n2026-10-26 12:00:01.000 ERROR - Core: boom\nException in thread \"main\" java.lang.OutOfMemoryError: Java heap space\n",
			"the runtime JVM ran out of memory at 12:00:01 (runtime.log)"},
		{"last error, reported as such",
			"\n=== runtime start x ===\n2026-10-26 12:00:01.000 ERROR - Connector: first\n2026-10-26 12:30:02.500 CRITICAL - Core: Could not connect to the database\n2026-10-26 12:30:03.000 INFO - Core: bye\n",
			"last runtime error at 12:30:02: Core: Could not connect to the database (runtime.log)"},
		{"nothing to say", "\n=== runtime start x ===\n2026-10-26 12:00:01.000 INFO - Core: fine\n", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := RuntimeLogStopReason(writeRuntimeLog(t, tc.log)); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
	if got := RuntimeLogStopReason(filepath.Join(t.TempDir(), "missing.log")); got != "" {
		t.Errorf("a missing log must give no reason, got %q", got)
	}
}

// Expected behaviour 2, verbatim: "`run-state.json` records it (`phase:
// stopped`, `ended` set)". The reason rides on the error the run returns.
func TestRunStateRecorder_RuntimeStopRecordsTheReason(t *testing.T) {
	r, published := newTestRecorder(t)
	r.Ready("http://127.0.0.1:8080/", BuildOutcome{Gen: 1, OK: true, Action: "boot"})
	r.Exit(&RuntimeStoppedError{Reason: "trial licence time limit reached at 18:43:16 (runtime.log)", msg: "the app stopped"})
	last := (*published)[len(*published)-1]
	if last.Phase != PhaseStopped || last.Ended.IsZero() {
		t.Fatalf("published %+v, want phase stopped with ended set", last)
	}
	if last.Reason != "trial licence time limit reached at 18:43:16 (runtime.log)" {
		t.Errorf("reason = %q", last.Reason)
	}

	// A stop whose logs say nothing still says the runtime stopped by itself.
	r2, published2 := newTestRecorder(t)
	r2.Ready("http://127.0.0.1:8080/", BuildOutcome{Gen: 1, OK: true, Action: "boot"})
	r2.Exit(&RuntimeStoppedError{msg: "the app stopped"})
	if got := (*published2)[len(*published2)-1].Reason; !strings.Contains(got, "stopped on its own") {
		t.Errorf("reason = %q", got)
	}

	// CONTROL: a requested stop has no reason to give.
	r3, published3 := newTestRecorder(t)
	r3.Ready("http://127.0.0.1:8080/", BuildOutcome{Gen: 1, OK: true, Action: "boot"})
	r3.Exit(nil)
	if got := (*published3)[len(*published3)-1].Reason; got != "" {
		t.Errorf("a requested stop recorded reason %q", got)
	}
}
