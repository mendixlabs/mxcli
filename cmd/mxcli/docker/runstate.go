// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/mendixlabs/mxcli/internal/pathutil"
)

// runstate.go is the contract between a running `mxcli run --local` and the
// commands that manage it from outside: `run status`, `run wait`, `run stop` and
// `run restart`.
//
// It exists because an agent driving the warm loop had no way to ask it anything.
// Measured over real app-building transcripts, a quarter of all tool calls went to
// managing the app by hand — nohup to start it, `until grep -q 'App is running'`
// to learn it had booted, `grep -c applied` loops to learn a change had landed,
// pkill plus pgrep loops to stop it — and every one of those calls re-reads the
// whole context (docs/11-proposals/PROPOSAL_agent_loop_efficiency.md). Each
// question those loops answer by scraping a log is answered here by a field.
//
// The file is written only by the run process, always by atomic rename, so a
// reader sees either the previous state or the next one and never a torn write.

// RunStateName is the state file under <projectDir>/.mxcli/.
const RunStateName = "run-state.json"

// RunLogName is where a detached run's own stdout/stderr go.
const RunLogName = "run.log"

// Run phases.
const (
	PhaseStarting = "starting"
	PhaseRunning  = "running"
	PhaseFailed   = "failed"  // never became ready
	PhaseStopped  = "stopped" // was running, then ended
)

// RunStatePath is <projectDir>/.mxcli/run-state.json.
func RunStatePath(projectPath string) string {
	return filepath.Join(filepath.Dir(projectPath), ".mxcli", RunStateName)
}

// RunLogPath is <projectDir>/.mxcli/run.log.
func RunLogPath(projectPath string) string {
	return filepath.Join(filepath.Dir(projectPath), ".mxcli", RunLogName)
}

// BuildOutcome is the result of one served build generation: the boot build
// (generation 1) or a rebuild the watch loop applied, or tried to.
type BuildOutcome struct {
	Gen int  `json:"gen"`
	OK  bool `json:"ok"`
	// Action is "boot", "reload" or "restart" for a success; for a failure it is
	// the stage that failed ("build", "apply", "client").
	Action     string `json:"action"`
	DurationMs int64  `json:"durationMs"`
	// Message is the one-line summary (the serve build's message, or the error).
	Message string `json:"message,omitempty"`
	// Errors are the build's consistency errors, one rendered line each
	// ("[CE0109] … — at Module / Document"), plus any explanation mxcli adds for a
	// failure it recognises (the 11.14 second-build defect).
	Errors []string `json:"errors,omitempty"`
	// Source is the model+theme source mtime this build was made from. It is what
	// makes `run wait` race-free: a change is covered by the first outcome whose
	// Source is not older than the change, whether that outcome landed before
	// `wait` was called or after.
	Source time.Time `json:"source"`
	At     time.Time `json:"at"`
}

// Duration is DurationMs as a time.Duration.
func (o BuildOutcome) Duration() time.Duration { return time.Duration(o.DurationMs) * time.Millisecond }

// RunState is the published state of one `mxcli run --local`.
type RunState struct {
	Project string `json:"project"`
	// PID is the mxcli process hosting the loop. Staleness is judged by it.
	PID int `json:"pid"`
	// Detached is true for `--detach`: the process leads its own session, so
	// every child it ever started — mxbuild's JVM, the runtime, the bundler — is
	// findable by session id even after the leader is gone.
	Detached bool `json:"detached"`
	// Args are the command-line arguments after the program name, minus
	// --detach, so `run restart` can start the same loop again.
	Args       []string  `json:"args"`
	Phase      string    `json:"phase"`
	Watch      bool      `json:"watch"`
	URL        string    `json:"url,omitempty"`
	AppPort    int       `json:"appPort"`
	AdminPort  int       `json:"adminPort"`
	ServePort  int       `json:"servePort"`
	Log        string    `json:"log,omitempty"`
	RuntimeLog string    `json:"runtimeLog,omitempty"`
	Started    time.Time `json:"started"`
	Ready      time.Time `json:"ready,omitempty"`
	Ended      time.Time `json:"ended,omitempty"`
	// Error is why the run failed or stopped on its own.
	Error string `json:"error,omitempty"`
	// Reason is, for an app whose runtime stopped on its own, why — one line,
	// from the runtime's log when it says ("trial licence time limit reached at
	// 18:43:16 (runtime.log)"). It is written in the same update that sets
	// phase stopped and ended, just before the run withdraws run-local.json, so
	// a reader of either file sees one state (ako/mxcli#1542).
	Reason string `json:"reason,omitempty"`
	// Gen is the newest build generation started; Building says whether it is
	// still in progress, and BuildingSource is the source mtime it is building.
	Gen            int       `json:"gen"`
	Building       bool      `json:"building"`
	BuildingSource time.Time `json:"buildingSource,omitempty"`
	// Scaffold is the theme source the run's builds wrote themselves, by
	// themesource/ module folder (see buildScaffold): files there no newer than
	// the time are build output, not an edit. Recorded so a reader of this file
	// (`run status`, `run wait`) judges "change pending" by the signal the
	// watcher uses — SourceMTimeOf.
	Scaffold map[string]time.Time `json:"scaffold,omitempty"`
	// HeldBy is the process holding the watcher (a `test --attach` changing the
	// project for its run, watchhold.go); 0 when nothing does.
	HeldBy int `json:"heldBy,omitempty"`
	// Last is the newest finished generation.
	Last    *BuildOutcome `json:"last,omitempty"`
	Updated time.Time     `json:"updated"`
}

// ReadRunState reads a project's state file. A missing file is (nil, nil), and
// so is one that was written for another project: copying an app folder copies
// .mxcli/ with it, and a state that names the original's pid must neither block
// the copy nor be acted on (`run stop` would signal the original's process).
// ForeignRunState says whether that is what was skipped.
func ReadRunState(projectPath string) (*RunState, error) {
	st, err := readRunStateFile(projectPath)
	if st != nil && !pathutil.SameProject(st.Project, projectPath) {
		return nil, nil
	}
	return st, err
}

// ForeignRunState returns the state in projectPath's .mxcli/ when it was
// written for a different project, so a command can say that it is ignoring it.
func ForeignRunState(projectPath string) *RunState {
	st, err := readRunStateFile(projectPath)
	if err != nil || st == nil || pathutil.SameProject(st.Project, projectPath) {
		return nil
	}
	return st
}

func readRunStateFile(projectPath string) (*RunState, error) {
	body, err := os.ReadFile(RunStatePath(projectPath))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var st RunState
	if err := json.Unmarshal(body, &st); err != nil {
		return nil, fmt.Errorf("%s is not valid JSON: %w", RunStatePath(projectPath), err)
	}
	return &st, nil
}

// WriteRunState publishes st atomically (write to a temp file, then rename).
// The file is 0600: Args can carry --db-password or a --constant value.
func WriteRunState(projectPath string, st *RunState) error {
	path := RunStatePath(projectPath)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	body, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	tmp := fmt.Sprintf("%s.%d.tmp", path, os.Getpid())
	if err := os.WriteFile(tmp, body, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}

// RunStateRecorder is what RunLocal reports its progress to. Every method is
// safe on a nil receiver, so the loop needs no branch when nobody is listening
// (the test runner boots its own app without one).
type RunStateRecorder struct {
	mu      sync.Mutex
	project string
	st      RunState
	now     func() time.Time
	// write is WriteRunState, injectable for tests.
	write func(string, *RunState) error
	// Warn receives a failure to publish (best-effort: never fatal to the run).
	Warn func(error)
}

// NewRunStateRecorder starts a state for a run that is beginning now, and
// publishes it in the "starting" phase.
func NewRunStateRecorder(projectPath string, pid int, detached, watch bool, args []string, log string) *RunStateRecorder {
	return newRunStateRecorder(projectPath, pid, detached, watch, args, log, time.Now, WriteRunState)
}

func newRunStateRecorder(projectPath string, pid int, detached, watch bool, args []string, log string,
	now func() time.Time, write func(string, *RunState) error) *RunStateRecorder {
	r := &RunStateRecorder{project: projectPath, now: now, write: write}
	t := r.now()
	r.st = RunState{
		Project: projectPath, PID: pid, Detached: detached, Args: args,
		Phase: PhaseStarting, Watch: watch, Log: log, Started: t, Updated: t,
	}
	r.publish()
	return r
}

// State returns a copy of the current state.
func (r *RunStateRecorder) State() RunState {
	if r == nil {
		return RunState{}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.st
}

// update applies f and publishes the result. The write happens under the lock
// so two updates can never publish out of order.
func (r *RunStateRecorder) update(f func(*RunState)) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	f(&r.st)
	r.st.Updated = r.now()
	r.publishLocked()
}

func (r *RunStateRecorder) publish() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.publishLocked()
}

func (r *RunStateRecorder) publishLocked() {
	cp := r.st
	if err := r.write(r.project, &cp); err != nil && r.Warn != nil {
		r.Warn(err)
	}
}

// Ports records the ports the loop serves on, once they are resolved.
func (r *RunStateRecorder) Ports(app, admin, serve int, runtimeLog string) {
	r.update(func(s *RunState) {
		s.AppPort, s.AdminPort, s.ServePort, s.RuntimeLog = app, admin, serve, runtimeLog
	})
}

// Scaffold records the theme scaffold the run's builds have written.
func (r *RunStateRecorder) Scaffold(s buildScaffold) {
	cp := make(map[string]time.Time, len(s))
	for k, v := range s {
		cp[k] = v
	}
	r.update(func(st *RunState) { st.Scaffold = cp })
}

// Covered records that the latest build covers source up to t without a new
// build: the watcher found the model it already serves (a hold released over an
// unchanged model). Without it `run status` read the project's newer mtimes as a
// change pending, and `run wait` waited for a build that was never coming.
func (r *RunStateRecorder) Covered(t time.Time) {
	r.update(func(st *RunState) {
		if st.Last != nil && t.After(st.Last.Source) {
			st.Last.Source = t
		}
	})
}

// Held records which process holds the watcher, or 0 when the hold ended.
func (r *RunStateRecorder) Held(pid int) {
	r.update(func(st *RunState) { st.HeldBy = pid })
}

// BuildStarted records that generation gen is being built from source.
func (r *RunStateRecorder) BuildStarted(gen int, source time.Time) {
	r.update(func(s *RunState) {
		s.Gen, s.Building, s.BuildingSource = gen, true, source
	})
}

// BuildFinished records the outcome of the generation in progress.
func (r *RunStateRecorder) BuildFinished(o BuildOutcome) {
	r.update(func(s *RunState) {
		if o.At.IsZero() {
			o.At = r.now()
		}
		s.Building = false
		s.Last = &o
	})
}

// Ready records that the app is serving at url. The boot build (generation 1)
// is the first outcome.
func (r *RunStateRecorder) Ready(url string, boot BuildOutcome) {
	r.update(func(s *RunState) {
		now := r.now()
		if boot.At.IsZero() {
			boot.At = now
		}
		s.Phase, s.URL, s.Ready = PhaseRunning, url, now
		s.Building = false
		s.Last = &boot
	})
}

// Exit records how the run ended: nil is a requested stop; an error before
// readiness is a failed boot; an error after it is an app that stopped on its
// own — and when that is because the runtime stopped, why.
func (r *RunStateRecorder) Exit(err error) {
	r.update(func(s *RunState) {
		s.Ended = r.now()
		s.Building = false
		if err != nil {
			s.Error = err.Error()
		}
		var stopped *RuntimeStoppedError
		if errors.As(err, &stopped) {
			s.Reason = stopped.Reason
			if s.Reason == "" {
				s.Reason = "the Mendix runtime stopped on its own (its log does not say why)"
			}
		}
		if s.Phase == PhaseStarting && err != nil {
			s.Phase = PhaseFailed
			return
		}
		s.Phase = PhaseStopped
	})
}

// --- wait ---------------------------------------------------------------

// WaitVerdict is what one look at the state says about a pending `run wait`.
type WaitVerdict int

const (
	// WaitPending: keep waiting.
	WaitPending WaitVerdict = iota
	// WaitDone: the outcome returned covers the change (OK or not).
	WaitDone
	// WaitBootFailed: the run never became ready.
	WaitBootFailed
	// WaitNotRunning: no live run serves this project.
	WaitNotRunning
	// WaitNotWatching: the source changed, but the run is not watching, so the
	// change will never be applied by it.
	WaitNotWatching
)

// EvaluateWait decides a `run wait` from one snapshot of the state.
//
// target is the model+theme source mtime observed when `wait` started; the
// change is covered by the newest outcome whose Source is not older than it.
// Comparing source times rather than counting generations is what makes this
// race-free: a change the watch loop already applied between the agent's write
// and the `wait` call is covered by the outcome that is already there, so it is
// reported at once instead of waiting for a next change that never comes.
//
// since > 0 instead waits for any generation newer than since — for a caller
// that captured the generation before making a change.
func EvaluateWait(st *RunState, alive bool, target time.Time, since int) (WaitVerdict, *BuildOutcome) {
	if st == nil {
		return WaitNotRunning, nil
	}
	if st.Phase == PhaseFailed {
		return WaitBootFailed, nil
	}
	if !alive || st.Phase == PhaseStopped {
		return WaitNotRunning, nil
	}
	if st.Phase == PhaseStarting || st.Last == nil {
		return WaitPending, nil
	}
	if since > 0 {
		if st.Last.Gen > since {
			return WaitDone, st.Last
		}
	} else if !st.Last.Source.Before(target) {
		return WaitDone, st.Last
	}
	if !st.Watch {
		return WaitNotWatching, nil
	}
	return WaitPending, nil
}

// ReadyVerdict decides `run wait --ready` from one snapshot.
func ReadyVerdict(st *RunState, alive bool) WaitVerdict {
	switch {
	case st == nil:
		return WaitNotRunning
	case st.Phase == PhaseFailed:
		return WaitBootFailed
	case !alive || st.Phase == PhaseStopped:
		return WaitNotRunning
	case st.Phase == PhaseRunning:
		return WaitDone
	}
	return WaitPending
}

// FormatOutcome renders an outcome as `run wait` prints it: one line for a
// success, the line plus at most maxLines error lines for a failure.
func FormatOutcome(o *BuildOutcome, maxLines int) string {
	if o == nil {
		return ""
	}
	if o.OK {
		line := fmt.Sprintf("applied: build #%d via %s in %s", o.Gen, o.Action, o.Duration().Round(100*time.Millisecond))
		if o.Action == ActionRestart.String() {
			line += " (runtime restarted; browser sessions dropped)"
		}
		return line + "\n"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "failed: build #%d (%s): %s\n", o.Gen, o.Action, firstLine(strings.TrimSpace(o.Message)))
	n := 0
	for _, e := range o.Errors {
		for _, l := range strings.Split(strings.TrimRight(e, "\n"), "\n") {
			if strings.TrimSpace(l) == "" {
				continue
			}
			if n == maxLines {
				b.WriteString("  … (more in the run log)\n")
				return b.String()
			}
			b.WriteString("  " + strings.TrimLeft(l, " ") + "\n")
			n++
		}
	}
	return b.String()
}

// SourceMTime is the --watch change signal (model + theme source), exported so
// `run wait` measures a change with the same clock the watch loop builds from.
func SourceMTime(projectPath string) time.Time { return sourceMTime(projectPath) }

// SourceMTimeOf is the change signal of the run st describes: SourceMTime
// without the theme scaffold its builds wrote, which is what its watcher
// compares. A hosted dev loop (`--test-endpoint`) scaffolds themesource/mxtest/
// at boot, and judged by the unfiltered signal `run status` reported "change
// pending" for as long as it ran, a change the watcher rightly never built.
func SourceMTimeOf(projectPath string, st *RunState) time.Time {
	if st == nil || len(st.Scaffold) == 0 {
		return sourceMTime(projectPath)
	}
	return sourceMTimeExcept(projectPath, buildScaffold(st.Scaffold))
}

// buildErrorLines turns a failed build into the lines a waiter is shown.
func buildErrorLines(b *BuildResult, hint string) []string {
	var out []string
	if b != nil {
		for _, p := range b.Errors() {
			line := p.Message
			if p.ErrorCode != "" {
				line = "[" + p.ErrorCode + "] " + line
			}
			if loc := p.Where(); loc != "" {
				line += " — at " + loc
			}
			out = append(out, line)
		}
		if len(out) == 0 {
			out = append(out, serveErrorDetails(b.Raw)...)
		}
		if len(out) == 0 {
			if raw := strings.TrimSpace(string(b.Raw)); raw != "" && raw != b.Message {
				if len(raw) > 2000 {
					raw = raw[:2000] + "…"
				}
				out = append(out, raw)
			}
		}
	}
	if hint = strings.TrimSpace(hint); hint != "" {
		out = append(out, hint)
	}
	return out
}

// serveErrorDetails extracts the problems.errors list a serve build returns for
// a failure that is not a consistency error — a theme that does not compile, a
// bundler that cannot start. Each carries the real cause in "details" (the SCSS
// compiler's "Undefined variable … main.scss 4:18"), which is what a waiter
// needs instead of the JSON envelope around it.
func serveErrorDetails(raw []byte) []string {
	var body struct {
		Problems struct {
			Errors []struct {
				Message string `json:"message"`
				Details string `json:"details"`
			} `json:"errors"`
		} `json:"problems"`
	}
	if len(raw) == 0 || json.Unmarshal(raw, &body) != nil {
		return nil
	}
	var out []string
	for _, e := range body.Problems.Errors {
		line := strings.TrimSpace(e.Details)
		if line == "" {
			line = strings.TrimSpace(e.Message)
		}
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}
