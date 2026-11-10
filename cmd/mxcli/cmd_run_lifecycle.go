// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/mendixlabs/mxcli/cmd/mxcli/docker"
	"github.com/mendixlabs/mxcli/cmd/mxcli/testrunner"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// cmd_run_lifecycle.go makes `mxcli run --local` a background service an agent
// can drive in one call per question: `--detach` to start it, and `run status`,
// `run wait`, `run stop`, `run restart` to manage it.
//
// Before this, every one of those questions was a hand-rolled shell loop —
// nohup and a log file to start, `until grep -q 'App is running'` to learn it was
// up, `grep -c applied` counting to learn a change had landed, pkill and pgrep
// loops to stop it — and they took about a quarter of all tool calls in measured
// app-building sessions. See docs/11-proposals/PROPOSAL_agent_loop_efficiency.md.
// Each command here answers from .mxcli/run-state.json (docker/runstate.go),
// which the run itself keeps current, and exits with a code a `&&` chain can act
// on.

// Exit codes shared by the lifecycle commands. Distinct on purpose, so
// `mxcli exec x.mdl && mxcli run wait` is a valid one-call chain and a caller
// can tell a broken change from a slow one.
const (
	runExitOK         = 0 // running / applied / stopped as asked
	runExitFailed     = 1 // the change, or the boot, failed
	runExitTimeout    = 2 // no result in time
	runExitNotRunning = 3 // nothing is serving this project
	runExitStarting   = 4 // status only: booting, not yet serving
)

// envDetachedChild marks the process a `--detach` parent started, so it runs
// the loop instead of detaching again.
const envDetachedChild = "MXCLI_RUN_DETACHED"

// detachBootLimit bounds how long `--detach` waits for the boot. A first boot
// can download mxbuild and a runtime, so it is generous; past it the parent
// returns and the run keeps booting in the background.
var detachBootLimit = 20 * time.Minute

// statePollInterval is how often the lifecycle commands re-read the state file.
var statePollInterval = 250 * time.Millisecond

// stopGrace is how long `run stop` lets the run tear itself down — the runtime's
// own shutdown, then mxbuild's and the bundler's — before it kills.
var stopGrace = 60 * time.Second

var runStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "One line: is the local app running, where, and how did the last change go",
	Long: `Print one line about the 'mxcli run --local' serving this project: running or
stopped, its URL, pid and uptime, the last build generation and how it was
applied (reload or restart) or why it failed, and whether a change on disk is
still waiting to be applied.

A state file whose process is gone is reported as stopped, and says so. A run
whose runtime stopped on its own (the trial licence's time limit, a crash) says
why when its runtime log does:

  stopped: trial licence time limit reached at 18:43:16 (runtime.log); restart with mxcli run restart -p app.mpr

Check this first when .mxcli/run-local.json is missing or the admin API answers
401/403: that file is removed when the run ends, and a command that cannot find
it falls back to default credentials. A live run whose run-local.json is gone is
reported as running, with the missing file named.

Exit codes: 0 running, 3 not running, 4 still starting.`,
	Example: `  mxcli run status -p app.mpr
  mxcli run status -p app.mpr || mxcli run --local --watch --detach -p app.mpr`,
	Run: func(cmd *cobra.Command, args []string) {
		projectPath := lifecycleProject(cmd)
		os.Exit(runStatus(projectPath, os.Stdout))
	},
}

var runStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the local app and every process it started, then return",
	Long: `Stop the 'mxcli run --local' serving this project: the runtime, mxbuild --serve
and the web client bundler. It asks the run to shut down (so it tears down its
own children and removes anything it installed, like --test-endpoint), waits
until they are gone, and for a --detach run then checks the run's whole process
session, so no orphaned runtimelauncher or mxbuild JVM is left behind — even
from a run that was killed with -9 and never cleaned up. A --test-endpoint the
run could not remove itself (on Windows the stop is always a forced one) is
removed from the project here, from what the run recorded when it installed it.

Stopping when nothing runs is a no-op that exits 0.

Never use pkill -f 'mxcli run' for this: the pattern also matches the shell it
runs in.`,
	Example: `  mxcli run stop -p app.mpr`,
	Run: func(cmd *cobra.Command, args []string) {
		projectPath := lifecycleProject(cmd)
		os.Exit(runStop(projectPath, os.Stdout))
	},
}

var runWaitCmd = &cobra.Command{
	Use:   "wait",
	Short: "Block until the last model change is applied (or failed); --ready waits for boot",
	Long: `Block until the watch loop has applied the current state of the model — by a hot
reload or a runtime restart — or has failed to, then print one result line:

  applied: build #3 via reload in 1.4s
  failed: build #4 (build): Build failed
    [CE0109] … — at Module / Page 'Customer_Edit'

It is race-free: the model's source time is read when wait starts, and the
first build made from that source or newer is the answer. A change the loop
already applied before wait was called is reported at once, so

  mxcli exec change.mdl -p app.mpr && mxcli run wait -p app.mpr

is a correct one-call chain. With nothing changed since the last build it
reports that build. --since N instead waits for any build newer than N.

--ready waits for the app to finish booting instead.

Exit codes: 0 applied (or ready), 1 failed, 2 timeout, 3 not running. A run
started without --watch never applies a change: wait says so and exits 1.`,
	Example: `  mxcli exec change.mdl -p app.mpr && mxcli run wait -p app.mpr
  mxcli run wait --ready -p app.mpr --timeout 10m`,
	Run: func(cmd *cobra.Command, args []string) {
		projectPath := lifecycleProject(cmd)
		timeout, _ := cmd.Flags().GetDuration("timeout")
		ready, _ := cmd.Flags().GetBool("ready")
		since, _ := cmd.Flags().GetInt("since")
		if ready {
			os.Exit(runWaitReady(projectPath, timeout, os.Stdout))
		}
		os.Exit(runWait(projectPath, timeout, since, os.Stdout))
	},
}

var runRestartCmd = &cobra.Command{
	Use:   "restart",
	Short: "Stop the local app and start it again, detached, with the same arguments",
	Long: `Stop the 'mxcli run --local' serving this project (as 'mxcli run stop') and
start it again in the background with the arguments it was started with (as
'mxcli run --local … --detach'). Returns once the app is up, or failed to boot.

The arguments come from the project's run state, so this also restarts a run
that has already stopped or crashed. To change a flag, stop it and start a new
one: mxcli run stop -p app.mpr && mxcli run --local --watch --detach -p app.mpr …

Secrets written literally on the command line (--constant values, --db-password,
--db-admin-password, --screenshot-password, --hub-secret, --debug-pass) are not
stored with the run, so a run started with them cannot be restarted: restart says
so and changes nothing. Written as @env:VAR or @file:path they are stored as that
reference and resolved again on restart, in the environment of the restarting
shell.`,
	Example: `  mxcli run restart -p app.mpr`,
	Run: func(cmd *cobra.Command, args []string) {
		projectPath := lifecycleProject(cmd)
		st, err := docker.ReadRunState(projectPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(runExitFailed)
		}
		noteForeignRunState(projectPath, os.Stderr)
		if st == nil || len(st.Args) == 0 {
			fmt.Fprintf(os.Stderr, "nothing to restart: no run of this project is recorded\n"+
				"  start one: mxcli run --local --watch --detach -p %s\n", projectPath)
			os.Exit(runExitNotRunning)
		}
		// Secrets given on the command line are not stored with the run, so a
		// restart would hand the app the placeholder instead of the value. Say so
		// before stopping anything.
		if hasRedactedArgs(st.Args) {
			fmt.Fprint(os.Stderr, restartSecretRefusal(projectPath))
			os.Exit(runExitFailed)
		}
		if code := runStop(projectPath, os.Stdout); code != runExitOK {
			os.Exit(code)
		}
		os.Exit(runDetach(projectPath, st.Args, os.Stdout))
	},
}

func init() {
	runWaitCmd.Flags().Duration("timeout", 5*time.Minute, "Give up after this long (exit 2)")
	runWaitCmd.Flags().Bool("ready", false, "Wait for the app to finish booting instead of for a change")
	runWaitCmd.Flags().Int("since", 0, "Wait for a build generation newer than this one, instead of for the current source")
	runCmd.AddCommand(runStatusCmd, runStopCmd, runWaitCmd, runRestartCmd)
}

// lifecycleProject resolves -p (or the discovered project) to an absolute path.
func lifecycleProject(cmd *cobra.Command) string {
	projectPath, _ := cmd.Flags().GetString("project")
	if projectPath == "" {
		fmt.Fprintln(os.Stderr, "Error: --project (-p) is required")
		os.Exit(runExitFailed)
	}
	if abs, err := filepath.Abs(projectPath); err == nil {
		projectPath = abs
	}
	return projectPath
}

// restartSecretRefusal is why `run restart` refuses a run whose stored
// arguments had a literal secret redacted, and what to do instead.
//
// It used to suggest the PGPASSWORD environment variable, which the runtime's
// database password never reads (only --ensure-db's psql does): a run started
// that way connected as the default password. A reference is what restart can
// resolve again.
func restartSecretRefusal(projectPath string) string {
	return fmt.Sprintf("cannot restart: this run was started with a secret written literally on the command line "+
		"(a --constant value, --db-password, ...), and it is not stored.\n"+
		"  Stop it and start it again with the secret as a reference, which is stored and resolved again on restart:\n"+
		"    mxcli run stop -p %s\n"+
		"    mxcli run --local --detach -p %s --db-password @env:MX_DB_PASSWORD ...   (or @file:path)\n",
		projectPath, projectPath)
}

// --- detach ------------------------------------------------------------

// runFlagLookup finds a `run` flag by name. It is set in init rather than
// calling runCmd.Flag directly, because runCmd's Run refers to detachArgs and
// a direct reference would be an initialization cycle.
var runFlagLookup = func(string) *pflag.Flag { return nil }

func init() {
	runFlagLookup = func(name string) *pflag.Flag { return runCmd.Flag(name) }
}

// detachArgs turns the parent's arguments into the child's: --detach removed,
// and the project pinned to an absolute path so the child (and a later
// `run restart` from another directory) runs the same project.
func detachArgs(args []string, projectPath string) []string {
	var out []string
	args = absoluteFileRefArgs(args)
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--detach" || strings.HasPrefix(a, "--detach="):
			continue
		case a == "-p" || a == "--project":
			i++ // drop the value too
			continue
		case strings.HasPrefix(a, "--project="), strings.HasPrefix(a, "-p") && len(a) > 2:
			continue // --project=X, -p=X, -pX
		}
		out = append(out, a)
		// A flag that takes a value keeps it, even when the value looks like
		// one of the flags dropped above (`--db-password -pX`). From
		// mendixlabs/mxcli#1761.
		if name, ok := strings.CutPrefix(a, "--"); ok && i+1 < len(args) {
			if f := runFlagLookup(name); f != nil && f.NoOptDefVal == "" {
				i++
				out = append(out, args[i])
			}
		}
	}
	return append(out, "-p", projectPath)
}

// noteForeignRunState says so when the run state in this project's .mxcli/ was
// written for another project, which a copied folder brings along. Every reader
// treats such a state as no run at all (docker.ReadRunState), so a command
// that goes on to report "not running" is not leaving the user guessing why.
func noteForeignRunState(projectPath string, out io.Writer) {
	if st := docker.ForeignRunState(projectPath); st != nil {
		fmt.Fprintf(out, "note: .mxcli/run-state.json here belongs to the run of %s (pid %d), not this project; ignoring it\n",
			st.Project, st.PID)
	}
}

// refuseIfRunning returns an error when a live run already serves the project
// (other than this process).
func refuseIfRunning(projectPath string) error {
	st, err := docker.ReadRunState(projectPath)
	if err != nil || st == nil {
		return nil
	}
	if st.PID == os.Getpid() || !docker.PidAlive(st.PID) {
		return nil
	}
	if st.Phase != docker.PhaseStarting && st.Phase != docker.PhaseRunning {
		return nil
	}
	where := st.URL
	if where == "" {
		where = "starting"
	}
	return fmt.Errorf("already running for this project: %s (pid %d)\n"+
		"  mxcli run status -p %s     # what it is doing\n"+
		"  mxcli run stop -p %s       # stop it first, or\n"+
		"  mxcli run restart -p %s    # stop and start it again with the same arguments",
		where, st.PID, projectPath, projectPath, projectPath)
}

// runDetach starts `mxcli <args>` as a detached background process and returns
// once its app is serving (0) or its boot failed (1).
func runDetach(projectPath string, args []string, out io.Writer) int {
	if err := refuseIfRunning(projectPath); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return runExitFailed
	}
	// A previous detached run that was killed without cleaning up leaves its
	// children in its session; they hold the ports this boot needs. They are
	// provably ours (same session, started after it), so reap them rather than
	// fail on a port the user would then have to go and free by hand.
	if prev, _ := docker.ReadRunState(projectPath); prev != nil && prev.Detached && !docker.PidAlive(prev.PID) {
		if n := reapSession(prev); n > 0 {
			fmt.Fprintf(out, "reaped %d leftover process(es) of a previous run (pid %d)\n", n, prev.PID)
		}
	}

	self, err := os.Executable()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: locating mxcli: %v\n", err)
		return runExitFailed
	}
	logPath := docker.RunLogPath(projectPath)
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return runExitFailed
	}
	logf, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return runExitFailed
	}
	defer logf.Close()

	child := exec.Command(self, args...)
	child.Stdout, child.Stderr = logf, logf
	child.Env = append(os.Environ(), envDetachedChild+"=1")
	setDetachAttrs(child)
	if err := child.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: starting the background run: %v\n", err)
		return runExitFailed
	}
	pid := child.Process.Pid
	exited := make(chan error, 1)
	go func() { exited <- child.Wait() }()

	deadline := time.Now().Add(detachBootLimit)
	for {
		st, _ := docker.ReadRunState(projectPath)
		if st != nil && st.PID == pid {
			switch st.Phase {
			case docker.PhaseRunning:
				fmt.Fprint(out, detachedReadyLine(st))
				printLogWarnings(out, logPath, 5)
				return runExitOK
			case docker.PhaseFailed, docker.PhaseStopped:
				printBootFailure(out, st.Error, logPath)
				return runExitFailed
			}
		}
		select {
		case err := <-exited:
			// The child is gone. Re-read once: it may have published its
			// failure just before exiting.
			if st, _ := docker.ReadRunState(projectPath); st != nil && st.PID == pid && st.Error != "" {
				printBootFailure(out, st.Error, logPath)
			} else {
				msg := "the background run exited before the app was ready"
				if err != nil {
					msg += " (" + err.Error() + ")"
				}
				printBootFailure(out, msg, logPath)
			}
			return runExitFailed
		case <-time.After(statePollInterval):
		}
		if time.Now().After(deadline) {
			fmt.Fprintf(out, "still starting after %s (pid %d, log %s); it continues in the background:\n"+
				"  mxcli run wait --ready -p %s\n", detachBootLimit, pid, displayPath(logPath), projectPath)
			return runExitTimeout
		}
	}
}

// detachedReadyLine is the single line --detach prints on success.
func detachedReadyLine(st *docker.RunState) string {
	mode := "no watch"
	if st.Watch {
		mode = "watch"
	}
	return fmt.Sprintf("running: %s (pid %d, %s, log %s)\n", st.URL, st.PID, mode, displayPath(st.Log))
}

// printBootFailure prints why a boot failed and the end of the run log, which
// is where mxbuild's and the runtime's own explanation is.
func printBootFailure(out io.Writer, reason, logPath string) {
	fmt.Fprintf(out, "failed: %s\n", firstLineOf(reason))
	tail := tailLines(logPath, 15)
	if len(tail) > 0 {
		fmt.Fprintf(out, "  last lines of %s:\n", displayPath(logPath))
		for _, l := range tail {
			fmt.Fprintf(out, "    %s\n", l)
		}
	}
}

// printLogWarnings surfaces the boot's warnings, which a detached run would
// otherwise bury in its log (a missing client bundle, a hub that refused).
func printLogWarnings(out io.Writer, logPath string, max int) {
	f, err := os.Open(logPath)
	if err != nil {
		return
	}
	defer f.Close()
	n := 0
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		l := strings.TrimSpace(sc.Text())
		if strings.HasPrefix(l, "Warning:") {
			if n == max {
				fmt.Fprintf(out, "  … more warnings in %s\n", displayPath(logPath))
				return
			}
			fmt.Fprintf(out, "  %s\n", l)
			n++
		}
	}
}

// --- status ------------------------------------------------------------

func runStatus(projectPath string, out io.Writer) int {
	noteForeignRunState(projectPath, out)
	st, err := docker.ReadRunState(projectPath)
	if err != nil {
		fmt.Fprintf(out, "unknown: %v\n", err)
		return runExitFailed
	}
	line, code := statusLine(st, liveStatusProbes(), docker.SourceMTimeOf(projectPath, st), time.Now())
	fmt.Fprintln(out, line)
	return code
}

// runStatusSummary is the line `mxcli run status -p projectPath` would print,
// for an error elsewhere to quote; "" when no run of the project is recorded.
func runStatusSummary(projectPath string) string {
	st, err := docker.ReadRunState(projectPath)
	if err != nil || st == nil {
		return ""
	}
	line, _ := statusLine(st, liveStatusProbes(), docker.SourceMTimeOf(projectPath, st), time.Now())
	return line
}

// statusProbes are what statusLine asks of the system, injected so its rules
// are testable without processes. A nil probe has nothing to say.
type statusProbes struct {
	alive     func(int) bool
	leftovers func(*docker.RunState) int
	// logReason reads why the runtime stopped from the run's runtime log.
	logReason func(*docker.RunState) string
	// handshake reports whether the run's .mxcli/run-local.json exists.
	handshake func(*docker.RunState) bool
}

// liveStatusProbes asks the real system.
func liveStatusProbes() statusProbes {
	return statusProbes{
		alive:     docker.PidAlive,
		leftovers: leftoverCount,
		logReason: func(st *docker.RunState) string { return docker.RuntimeLogStopReason(st.RuntimeLog) },
		handshake: func(st *docker.RunState) bool {
			if st.Project == "" {
				return true // cannot tell; do not cry wolf
			}
			_, err := os.Stat(devLoopHandshakePath(st.Project))
			return err == nil
		},
	}
}

// probesWithAlive is liveStatusProbes with liveness already decided by the
// caller, so one snapshot gives one answer.
func probesWithAlive(alive bool) statusProbes {
	p := liveStatusProbes()
	p.alive = func(int) bool { return alive }
	return p
}

func (p statusProbes) reasonFromLog(st *docker.RunState) string {
	if p.logReason == nil || st.RuntimeLog == "" {
		return ""
	}
	return p.logReason(st)
}

func (p statusProbes) handshakePublished(st *docker.RunState) bool {
	return p.handshake == nil || p.handshake(st)
}

// restartHint is the command that brings a stopped run back with the same
// arguments.
func restartHint(st *docker.RunState) string {
	if st.Project == "" {
		return "mxcli run restart"
	}
	return "mxcli run restart -p " + st.Project
}

// leftoverCount is how many processes of a dead detached run are still alive.
func leftoverCount(st *docker.RunState) int {
	if st == nil || !st.Detached {
		return 0
	}
	return len(docker.SessionMembers(st.PID, st.Started))
}

// statusLine renders the one status line and its exit code. Its inputs are
// injected so the staleness rules are testable without processes.
//
// A run that stopped on its own says why when anything recorded it — the run
// state's reason, else the runtime log — because a caller that only learns
// "stopped" (or, worse, finds run-local.json gone) goes looking at its
// credentials instead (ako/mxcli#1542).
func statusLine(st *docker.RunState, p statusProbes, source, now time.Time) (string, int) {
	if st == nil {
		return "stopped: no run recorded for this project", runExitNotRunning
	}
	live := p.alive(st.PID)
	if !live && (st.Phase == docker.PhaseStarting || st.Phase == docker.PhaseRunning) {
		line := fmt.Sprintf("stopped: pid %d exited without cleaning up (it was %s", st.PID, st.Phase)
		if st.URL != "" {
			line += " at " + st.URL
		}
		line += ")"
		if reason := p.reasonFromLog(st); reason != "" {
			line += "; its runtime log says: " + reason
		}
		if p.leftovers != nil {
			if n := p.leftovers(st); n > 0 {
				line += fmt.Sprintf("; %d leftover process(es) still running — mxcli run stop reaps them", n)
			}
		}
		return line, runExitNotRunning
	}
	switch st.Phase {
	case docker.PhaseFailed:
		return fmt.Sprintf("stopped: boot failed — %s (log %s)", firstLineOf(st.Error), displayPath(st.Log)), runExitNotRunning
	case docker.PhaseStopped:
		reason := st.Reason
		if reason == "" && st.Error != "" {
			// A run recorded by an older mxcli, or one that ended on another
			// error: the runtime log may still know.
			reason = p.reasonFromLog(st)
		}
		if reason != "" {
			return "stopped: " + reason + "; restart with " + restartHint(st), runExitNotRunning
		}
		if st.Error != "" {
			return "stopped: " + firstLineOf(st.Error), runExitNotRunning
		}
		return "stopped", runExitNotRunning
	case docker.PhaseStarting:
		return fmt.Sprintf("starting: pid %d, %s so far (log %s)", st.PID, now.Sub(st.Started).Round(time.Second), displayPath(st.Log)), runExitStarting
	}

	parts := []string{fmt.Sprintf("pid %d", st.PID), "up " + now.Sub(st.Ready).Round(time.Second).String()}
	if st.Watch {
		parts = append(parts, "watch")
	} else {
		parts = append(parts, "no watch")
	}
	if l := st.Last; l != nil {
		ago := now.Sub(l.At).Round(time.Second)
		switch {
		case l.OK && l.Action == "boot":
			parts = append(parts, fmt.Sprintf("build #%d (boot)", l.Gen))
		case l.OK:
			parts = append(parts, fmt.Sprintf("build #%d applied via %s %s ago", l.Gen, l.Action, ago))
		default:
			parts = append(parts, fmt.Sprintf("build #%d FAILED (%s) %s ago: %s — mxcli run wait prints the errors",
				l.Gen, l.Action, ago, firstLineOf(l.Message)))
		}
	}
	covered := time.Time{}
	if st.Last != nil {
		covered = st.Last.Source
	}
	switch {
	case st.Building:
		parts = append(parts, fmt.Sprintf("building #%d", st.Gen))
	case source.After(covered) && st.Watch:
		parts = append(parts, "change pending")
	case source.After(covered):
		parts = append(parts, "changes on disk NOT applied (no --watch): mxcli run restart applies them")
	}
	line := fmt.Sprintf("running: %s (%s)", st.URL, strings.Join(parts, ", "))
	if !p.handshakePublished(st) {
		// The app is up, but the file other commands find it by is gone — so
		// they fall back to default credentials and meet a 403 (#1542).
		line += "; but .mxcli/run-local.json is MISSING, so oql, log and constant --apply cannot " +
			"reach its admin API — " + restartHint(st) + " publishes it again"
	}
	return line, runExitOK
}

// --- wait --------------------------------------------------------------

func runWait(projectPath string, timeout time.Duration, since int, out io.Writer) int {
	// The run's own signal, without the theme scaffold its builds wrote: judged
	// by the unfiltered one, a hosted dev loop's MxTest scaffold was a change no
	// build would ever cover, and the wait ran into its timeout.
	first, _ := docker.ReadRunState(projectPath)
	target := docker.SourceMTimeOf(projectPath, first)
	deadline := time.Now().Add(timeout)
	for {
		st, err := docker.ReadRunState(projectPath)
		if err != nil {
			fmt.Fprintf(out, "failed: %v\n", err)
			return runExitFailed
		}
		alive := st != nil && docker.PidAlive(st.PID)
		verdict, outcome := docker.EvaluateWait(st, alive, target, since)
		switch verdict {
		case docker.WaitDone:
			if outcome.OK && outcome.Action == "boot" {
				fmt.Fprintf(out, "up to date: no change since the boot (build #%d)\n", outcome.Gen)
				return runExitOK
			}
			fmt.Fprint(out, docker.FormatOutcome(outcome, 10))
			if outcome.OK {
				return runExitOK
			}
			return runExitFailed
		case docker.WaitBootFailed:
			printBootFailure(out, "boot failed: "+st.Error, st.Log)
			return runExitFailed
		case docker.WaitNotRunning:
			line, _ := statusLine(st, probesWithAlive(alive), target, time.Now())
			fmt.Fprintf(out, "not running: %s\n", strings.TrimPrefix(line, "stopped: "))
			return runExitNotRunning
		case docker.WaitNotWatching:
			fmt.Fprintf(out, "failed: this run was started without --watch, so it never applies a change\n"+
				"  apply it with: mxcli run restart -p %s\n", projectPath)
			return runExitFailed
		}
		if time.Now().After(deadline) {
			what := "no build has picked the change up"
			if st != nil && st.Building {
				what = fmt.Sprintf("build #%d is still in progress", st.Gen)
			}
			fmt.Fprintf(out, "timeout: after %s %s (mxcli run status -p %s)\n", timeout, what, projectPath)
			return runExitTimeout
		}
		time.Sleep(statePollInterval)
	}
}

func runWaitReady(projectPath string, timeout time.Duration, out io.Writer) int {
	deadline := time.Now().Add(timeout)
	for {
		st, err := docker.ReadRunState(projectPath)
		if err != nil {
			fmt.Fprintf(out, "failed: %v\n", err)
			return runExitFailed
		}
		alive := st != nil && docker.PidAlive(st.PID)
		switch docker.ReadyVerdict(st, alive) {
		case docker.WaitDone:
			fmt.Fprint(out, detachedReadyLine(st))
			return runExitOK
		case docker.WaitBootFailed:
			printBootFailure(out, st.Error, st.Log)
			return runExitFailed
		case docker.WaitNotRunning:
			line, _ := statusLine(st, probesWithAlive(alive), time.Time{}, time.Now())
			fmt.Fprintf(out, "not running: %s\n", strings.TrimPrefix(line, "stopped: "))
			return runExitNotRunning
		}
		if time.Now().After(deadline) {
			fmt.Fprintf(out, "timeout: still starting after %s (log %s)\n", timeout, displayPath(st.Log))
			return runExitTimeout
		}
		time.Sleep(statePollInterval)
	}
}

// --- stop --------------------------------------------------------------

func runStop(projectPath string, out io.Writer) int {
	noteForeignRunState(projectPath, out)
	st, err := docker.ReadRunState(projectPath)
	if err != nil {
		fmt.Fprintf(out, "failed: %v\n", err)
		return runExitFailed
	}
	if st == nil {
		fmt.Fprintln(out, "stopped (nothing was running)")
		return runExitOK
	}
	start := time.Now()
	wasAlive := docker.PidAlive(st.PID)
	if wasAlive {
		// Ask first: on SIGTERM the run tears down its own children and undoes
		// what it installed (the --test-endpoint module, the debugger session).
		_ = terminateProcess(st.PID)
		if !waitGone(func() bool { return docker.PidAlive(st.PID) }, stopGrace) {
			_ = killProcess(st.PID)
			waitGone(func() bool { return docker.PidAlive(st.PID) }, 5*time.Second)
		}
	}
	reaped := reapSession(st)

	if st.Phase == docker.PhaseStarting || st.Phase == docker.PhaseRunning {
		// The run records its own exit when it shuts down cleanly; this covers a
		// run that could not (killed, or already dead).
		if cur, _ := docker.ReadRunState(projectPath); cur != nil && cur.PID == st.PID &&
			(cur.Phase == docker.PhaseStarting || cur.Phase == docker.PhaseRunning) {
			cur.Phase, cur.Ended, cur.Building = docker.PhaseStopped, time.Now(), false
			_ = docker.WriteRunState(projectPath, cur)
		}
		// And its handshake, which only it could withdraw (ako/mxcli#1542).
		withdrawDevLoopHandshake(projectPath, st.PID)
	}

	if left := remaining(st); len(left) > 0 {
		fmt.Fprintf(out, "failed: %d process(es) of pid %d's run are still alive: %v\n", len(left), st.PID, left)
		return runExitFailed
	}
	// A run stopped without its teardown — always on Windows, where the stop is
	// taskkill /F, and after the SIGKILL fallback above — leaves --test-endpoint
	// in the project; the run recorded how to take it out (ako/mxcli#1917).
	if _, err := testrunner.RestoreAbandonedEndpoint(projectPath, out); err != nil {
		fmt.Fprintf(out, "failed: %v\n", err)
		return runExitFailed
	}
	extra := ""
	if reaped > 0 {
		extra = fmt.Sprintf(", reaped %d leftover process(es)", reaped)
	}
	if wasAlive {
		fmt.Fprintf(out, "stopped: pid %d in %s%s\n", st.PID, time.Since(start).Round(100*time.Millisecond), extra)
	} else {
		fmt.Fprintf(out, "stopped (was not running%s)\n", extra)
	}
	return runExitOK
}

// reapSession terminates whatever is left of a detached run's session and
// returns how many processes it had to stop. A run that shut down cleanly
// leaves nothing, so this is normally a no-op; it is the guarantee for one that
// did not.
func reapSession(st *docker.RunState) int {
	if st == nil || !st.Detached {
		return 0
	}
	members := docker.SessionMembers(st.PID, st.Started)
	if len(members) == 0 {
		return 0
	}
	for _, pid := range members {
		_ = terminateProcess(pid)
	}
	anyAlive := func() bool {
		for _, pid := range members {
			if docker.PidAlive(pid) {
				return true
			}
		}
		return false
	}
	if !waitGone(anyAlive, 10*time.Second) {
		for _, pid := range members {
			_ = killProcess(pid)
		}
		waitGone(anyAlive, 5*time.Second)
	}
	return len(members)
}

// remaining lists what is still alive of a run: its leader and, for a detached
// run, its session.
func remaining(st *docker.RunState) []int {
	var left []int
	if docker.PidAlive(st.PID) {
		left = append(left, st.PID)
	}
	if st.Detached {
		left = append(left, docker.SessionMembers(st.PID, st.Started)...)
	}
	return left
}

// waitGone polls until alive() is false or limit passes; true when gone.
func waitGone(alive func() bool, limit time.Duration) bool {
	deadline := time.Now().Add(limit)
	for alive() {
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(100 * time.Millisecond)
	}
	return true
}

// --- helpers -----------------------------------------------------------

func firstLineOf(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

// tailLines returns the last n non-empty lines of a file.
func tailLines(path string, n int) []string {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var lines []string
	for _, l := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(l) != "" {
			lines = append(lines, strings.TrimRight(l, "\r"))
		}
	}
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return lines
}

// displayPath shortens a path to be relative to the working directory when it
// is below it.
func displayPath(p string) string {
	if p == "" {
		return p
	}
	if wd, err := os.Getwd(); err == nil {
		if rel, err := filepath.Rel(wd, p); err == nil && !strings.HasPrefix(rel, "..") {
			return rel
		}
	}
	return p
}
