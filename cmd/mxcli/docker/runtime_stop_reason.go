// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// runtime_stop_reason.go says, in one line, why a local runtime stopped — for
// `run status` and run-state.json (ako/mxcli#1542).
//
// The reason is in the runtime's application log, which only exists because
// mxcli attaches a file log subscriber writing .mxcli/runtime.log: a standalone
// runtime has no subscriber of its own, so "Maximum run time exceeded" never
// reaches the JVM's stdout, which is all LocalRuntime.Log() holds. Reading the
// in-memory output alone found the licence in a unit test and never in a run.

// RuntimeStoppedError ends a run whose Mendix runtime stopped without being
// asked to: the trial licence's time limit, a crash, a kill.
type RuntimeStoppedError struct {
	// Reason is one line for `run status` and run-state.json, such as "trial
	// licence time limit reached at 18:43:16 (runtime.log)"; "" when neither
	// log says why.
	Reason string
	msg    string
}

func (e *RuntimeStoppedError) Error() string { return e.msg }

// NewRuntimeStoppedError records both the status reason and the full error message.
func NewRuntimeStoppedError(reason, message string) *RuntimeStoppedError {
	return &RuntimeStoppedError{Reason: reason, msg: message}
}

// runtimeStartMarker begins each boot's section of the runtime log (see
// openRuntimeLog).
const runtimeStartMarker = "=== runtime start "

// runtimeLogTailBytes bounds how much of the runtime log is read: a reason is
// in the last lines, and the file grows for as long as the project is run.
const runtimeLogTailBytes = 256 << 10

// currentRuntimeLog returns the runtime log written since the newest boot
// began, or "" when it cannot be read. The file is appended across restarts,
// and an earlier boot's licence stop must not be blamed for this one.
func currentRuntimeLog(path string) string {
	if path == "" {
		return ""
	}
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	if info, err := f.Stat(); err == nil && info.Size() > runtimeLogTailBytes {
		if _, err := f.Seek(info.Size()-runtimeLogTailBytes, io.SeekStart); err != nil {
			return ""
		}
	}
	body, err := io.ReadAll(f)
	if err != nil {
		return ""
	}
	text := string(body)
	if i := strings.LastIndex(text, runtimeStartMarker); i >= 0 {
		text = text[i:]
	}
	return text
}

// RuntimeLogStopReason is stopReason over the current boot of the runtime log
// at path — for a run that could not record its own end (it was killed), whose
// log may still say why the app went.
func RuntimeLogStopReason(path string) string {
	return stopReason(currentRuntimeLog(path), filepath.Base(path))
}

var (
	// logClockRe finds the time of day on a runtime log line.
	logClockRe = regexp.MustCompile(`(?:^|[^0-9T:])(\d{2}:\d{2}:\d{2})(?:[.,]\d+)?\b`)
	// logErrorRe is a Mendix log line at ERROR or CRITICAL: "LEVEL - Node: message".
	logErrorRe = regexp.MustCompile(`\b(?:ERROR|CRITICAL)\s+-\s+(.+)$`)
)

// licenceStopMarkers are what the runtime logs when the trial licence ends the
// run. The boot warning ("...terminated when the maximum time is exceeded!")
// is deliberately not one: it is printed at every start.
var licenceStopMarkers = []string{"Maximum run time exceeded", "checkRuntimeDuration"}

// stopReason picks the cause of a runtime stop out of log text, naming source
// as where it was read; "" when nothing in it says. The known causes come
// first; otherwise the last ERROR line is reported as exactly that, not as a
// cause — inventing one is worse than naming none.
func stopReason(text, source string) string {
	if strings.TrimSpace(text) == "" {
		return ""
	}
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		for _, m := range licenceStopMarkers {
			if strings.Contains(lines[i], m) {
				return "trial licence time limit reached" + clockNear(lines, i) + " (" + source + ")"
			}
		}
	}
	for i := len(lines) - 1; i >= 0; i-- {
		if strings.Contains(lines[i], "java.lang.OutOfMemoryError") {
			return "the runtime JVM ran out of memory" + clockNear(lines, i) + " (" + source + ")"
		}
	}
	for i := len(lines) - 1; i >= 0; i-- {
		if m := logErrorRe.FindStringSubmatch(lines[i]); m != nil {
			msg := strings.TrimSpace(m[1])
			if len(msg) > 160 {
				msg = msg[:160] + "…"
			}
			return "last runtime error" + clockNear(lines, i) + ": " + msg + " (" + source + ")"
		}
	}
	return ""
}

// clockNear is " at HH:MM:SS" from line i, or from the few lines above it — a
// stack frame carries no time, the line that logged it does — or "".
func clockNear(lines []string, i int) string {
	for j := i; j >= 0 && j > i-6; j-- {
		if strings.HasPrefix(lines[j], runtimeStartMarker) {
			break
		}
		if m := logClockRe.FindStringSubmatch(lines[j]); m != nil {
			return " at " + m[1]
		}
	}
	return ""
}
