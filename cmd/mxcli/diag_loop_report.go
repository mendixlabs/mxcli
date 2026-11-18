// SPDX-License-Identifier: Apache-2.0

// diag_loop_report.go answers "where did this session's mxcli calls go?".
//
// It exists because the cost of an agent-driven session is dominated by the
// NUMBER of tool calls, not by their output: every model call re-reads the whole
// conversation, so total cost is calls x conversation size, and when a removed
// call takes its tool output with it the total falls with the square of the call
// count. A measured comparison of the same class of app built with mxcli vs on
// Vercel put the gap at 4.25x the model calls and 5.4x the conversation re-read.
// See docs/11-proposals/PROPOSAL_agent_loop_efficiency.md.
//
// Every mxcli invocation already writes a session_start record naming its argv
// (mdl/diaglog), so the distribution of calls is on disk and nobody has looked.
// This turns "the loop feels expensive" into a ranked list, and — run before and
// after a change — into evidence that the change worked.
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/mendixlabs/mxcli/mdl/diaglog"
	"github.com/spf13/cobra"
)

// logRecord is one JSON Lines entry. Only the fields this report reads are
// declared; diaglog writes more.
type logRecord struct {
	Time             time.Time `json:"time"`
	Msg              string    `json:"msg"`
	Args             []string  `json:"args"`
	Mode             string    `json:"mode"`
	CommandsExecuted int       `json:"commands_executed"`
	ErrorsCount      int       `json:"errors_count"`
	PID              int       `json:"pid"`
	// ParentPID is the mxcli process that spawned this one, when one did. It is
	// a string because diaglog writes "" for a top-level run (ako/mxcli#629).
	ParentPID string `json:"parent_pid"`
	// ExitCode is on session_end, and only when non-zero. Absent means zero:
	// before mxcli closed the session on its error path, a session_end could
	// only be written by a run that succeeded, so the default reads correctly
	// on older logs too.
	ExitCode int `json:"exit_code"`
}

// invocation is one mxcli process: a session_start and the session_end that
// closes it, if there was one.
type invocation struct {
	Verb     string
	Script   string
	Start    time.Time
	End      time.Time
	Ended    bool
	Commands int
	Errors   int
	PID      int
	// Spawned marks a run mxcli started itself. `mxcli test` runs three before a
	// single test executes, so counting them as calls the agent made overstates
	// the loop and understates `test` (ako/mxcli#629).
	Spawned bool
	// ExitCode is the code the process left with, for one that closed.
	ExitCode int
	// LastSeen is the newest session_alive heartbeat from this process. For a
	// run that never closed it is the only MEASURED evidence of how long it
	// lasted — a lower bound, since the process died somewhere after it.
	LastSeen time.Time
}

// Floor is the measured lower bound on an unclosed run's duration: how long it
// was still heartbeating. Zero when it died before its first heartbeat.
func (i invocation) Floor() time.Duration {
	if i.Ended || i.LastSeen.IsZero() {
		return 0
	}
	return i.LastSeen.Sub(i.Start)
}

// Duration is wall time for an invocation that closed. An invocation that did
// not close has no knowable duration — see Ended.
func (i invocation) Duration() time.Duration { return i.End.Sub(i.Start) }

// verbStats aggregates one command verb.
type verbStats struct {
	Verb string `json:"verb"`
	// Count is invocations of this verb; Unclosed is how many of them wrote no
	// session_end — the report's only evidence of a non-zero exit. Deliberately
	// NOT named Failed, and there is no per-verb error field either: the two
	// populations are disjoint (see loopReport.StatementErrors) and one name for
	// two meanings is how a report starts lying.
	Count    int `json:"count"`
	Unclosed int `json:"unclosed"`
	// ExitedNonZero is runs that closed and reported a non-zero exit code.
	// Unlike Unclosed these carry a measured duration, which is the point:
	// a run that failed still spent the time it spent.
	ExitedNonZero int           `json:"exited_nonzero"`
	TotalDur      time.Duration `json:"-"`
	TotalSec      float64       `json:"total_seconds"`
	MedianMS      int64         `json:"median_ms"`
	// FloorSec is the summed measured lower bound of this verb's unclosed runs:
	// how long each was still heartbeating. Measured, not extrapolated.
	FloorSec float64 `json:"unclosed_floor_seconds"`
	// UnmeasuredSec is an ESTIMATE for the unclosed runs that left no heartbeat
	// at all, at this verb's median closed duration — the only figure here that
	// is not a measurement. Zero when no run of this verb closed, because there
	// is then nothing to extrapolate from and a guess would be worse than the
	// admission.
	UnmeasuredSec float64 `json:"unmeasured_estimate_seconds"`
	// unheard is the unclosed runs with no heartbeat — the only ones the
	// estimate above covers. Not emitted: it is working state, and the two
	// figures it splits are both reported.
	unheard int
}

// loopReport is the whole analysis, and is what --json emits.
type loopReport struct {
	Invocations int `json:"invocations"`
	// StatementErrors counts runs that CLOSED while their summary reported at
	// least one failed statement — in practice `exec --continue-on-error`, which
	// is the only path that keeps going after one. It is NOT the failure count:
	// a run that exits non-zero exits through os.Exit and writes no summary at
	// all, so it lands in Unclosed instead.
	//
	// It was called `failed` until ako/mxcli#620. Measured against a real
	// 442-invocation log it read 0 throughout while real non-zero exits were
	// happening, so anyone reading the JSON alongside `unclosed: 10` drew the
	// wrong conclusion from a field that was doing its job. Naming it after what
	// it measures is the fix; making it mean "failed" would take an exit-code
	// path through all ~250 os.Exit sites.
	StatementErrors int `json:"runs_with_statement_errors"`
	// Unclosed is runs with no session_end — a process that VANISHED: killed
	// with SIGKILL, or exited through an os.Exit inside a command. A run that
	// merely returned an error now closes and is counted in ExitedNonZero
	// instead, with its duration (item 2d).
	Unclosed int `json:"unclosed"`
	// ExitedNonZero is runs that closed with a non-zero exit code. Measured,
	// duration included.
	ExitedNonZero int `json:"runs_exited_nonzero"`
	// Spawned is runs mxcli started itself, excluded from every other figure
	// here. They are real processes and their time is already inside their
	// parent's, so counting them again would double it (ako/mxcli#629).
	Spawned     int     `json:"spawned_by_mxcli"`
	WallSeconds float64 `json:"wall_seconds"`
	// UnclosedFloorSeconds is how long the unclosed runs were MEASURED to still
	// be running, from their heartbeats. A lower bound: each died somewhere
	// after its last one.
	UnclosedFloorSeconds float64 `json:"unclosed_floor_seconds"`
	// UnmeasuredSeconds estimates the unclosed runs that left no heartbeat, at
	// their verb's median. It is NEVER added into WallSeconds, which stays a
	// pure measurement — the figure a before/after comparison is read from.
	// Printing the three side by side is the fix for item 2d: the restart bill
	// was invisible because the only number on the page was computed over the
	// minority of runs that closed.
	UnmeasuredSeconds float64     `json:"unmeasured_estimate_seconds"`
	ByVerb            []verbStats `json:"by_verb"`
	CheckExecDup      int         `json:"check_then_exec_pairs"`
	Span              string      `json:"span"`
}

// parseLogRecords reads JSON Lines, skipping anything that does not parse. A
// truncated final line is normal (a process killed mid-write), so a bad line is
// not an error.
func parseLogRecords(lines []string) []logRecord {
	var out []logRecord
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var r logRecord
		if err := json.Unmarshal([]byte(line), &r); err != nil {
			continue
		}
		out = append(out, r)
	}
	return out
}

// invocationVerb resolves an argv to the cobra command path it invoked, so the
// verb list maintains itself: a renamed or added command is picked up with no
// change here. Falls back to the session's mode when argv names no subcommand,
// which is how the REPL and the one-shot `-c` form appear.
func invocationVerb(args []string, mode string) string {
	if len(args) > 1 {
		if cmd, _, err := rootCmd.Find(args[1:]); err == nil && cmd != nil && cmd != rootCmd {
			return strings.TrimPrefix(cmd.CommandPath(), rootCmd.Name()+" ")
		}
	}
	switch mode {
	case "batch":
		return "-c (one-shot)"
	case "repl":
		return "REPL"
	case "":
		return "(unknown)"
	default:
		return mode
	}
}

// invocationScript returns the first positional argument after the verb — the
// script a check or exec was given. Empty when there is none.
func invocationScript(args []string) string {
	for i := 1; i < len(args); i++ {
		a := args[i]
		if strings.HasPrefix(a, "-") {
			// A flag that takes a value consumes the next token. Treating every
			// flag as valued would swallow a positional after a boolean flag,
			// but mxcli's scripts are passed before their flags in every
			// documented form, so the first positional is reached first.
			continue
		}
		if strings.HasSuffix(a, ".mdl") {
			return a
		}
	}
	return ""
}

// buildInvocations segments the record stream into processes.
//
// Only session_start carries a pid, so sessions are delimited by their own
// start records rather than correlated by pid: an invocation runs until the
// next session_end or the next session_start, whichever comes first. That is
// exact for sequential invocations, which is what an agent loop produces, and
// would mis-attribute two mxcli processes running CONCURRENTLY. The report says
// so rather than pretending otherwise.
func buildInvocations(records []logRecord) []invocation {
	var out []invocation
	open := map[int]int{} // pid -> index of its open invocation
	cur := -1
	for _, r := range records {
		switch r.Msg {
		case "session_start":
			out = append(out, invocation{
				Verb:    invocationVerb(r.Args, r.Mode),
				Script:  invocationScript(r.Args),
				Start:   r.Time,
				PID:     r.PID,
				Spawned: r.ParentPID != "",
			})
			cur = len(out) - 1
			if r.PID != 0 {
				open[r.PID] = cur
			}
		case "session_alive":
			// Pairs by pid like session_end. A heartbeat is the only thing a
			// process that is killed, or that exits through an os.Exit inside a
			// command, leaves behind about its duration.
			if r.PID == 0 {
				continue
			}
			if i, ok := open[r.PID]; ok && r.Time.After(out[i].LastSeen) {
				out[i].LastSeen = r.Time
			}
		case "session_end":
			// Pair by pid when the log has one. `mxcli test` spawns three mxcli
			// processes before a single test runs, so their session_ends arrive
			// between the parent's start and its own end; closing "the most
			// recent open invocation" hands the parent's close to a child and
			// reports the parent as a failure. Measured: every `test` run showed
			// as unclosed although all its tests passed (ako/mxcli#629).
			i := cur
			if r.PID != 0 {
				j, ok := open[r.PID]
				if !ok {
					continue // an end whose start is outside this window
				}
				i = j
				delete(open, r.PID)
			}
			if i >= 0 && !out[i].Ended {
				out[i].End = r.Time
				out[i].Ended = true
				out[i].Commands = r.CommandsExecuted
				out[i].Errors = r.ErrorsCount
				out[i].ExitCode = r.ExitCode
			}
		}
	}
	return out
}

// analyzeLoop is the whole report, as a pure function of the records.
func analyzeLoop(records []logRecord) loopReport {
	invs := buildInvocations(records)

	rep := loopReport{}
	durs := map[string][]time.Duration{}
	stats := map[string]*verbStats{}

	for _, inv := range invs {
		// The report never counts itself. Since ako/mxcli#617 every command is
		// recorded from startSession, which excludes `diag` for this reason;
		// the filter stays as the second guard, because a report whose numbers
		// depend on one exclusion staying in place would drift silently.
		if inv.Verb == "diag" || strings.HasPrefix(inv.Verb, "diag ") {
			continue
		}
		// A run mxcli started itself is not a call anyone made. `mxcli test`
		// spawns `-c DESCRIBE SETTINGS`, `-c SHOW MODULES` and an `exec` of the
		// generated runner before the first test executes, so a session of 5
		// test runs carried 15 phantom entries in the table the report exists to
		// rank. Counted on its own line instead (ako/mxcli#629).
		if inv.Spawned {
			rep.Spawned++
			continue
		}
		s, ok := stats[inv.Verb]
		if !ok {
			s = &verbStats{Verb: inv.Verb}
			stats[inv.Verb] = s
		}
		s.Count++

		// An invocation with no session_end is a process that VANISHED: killed,
		// or exited through an os.Exit inside a command (deferred calls do not
		// run then). A command that returns an error no longer lands here —
		// main() closes the session on that path — which is what gives a failed
		// run a duration. What is left genuinely has none: the process did not
		// get to write one, and the log cannot say when it stopped. Its time is
		// estimated below rather than dropped.
		if !inv.Ended {
			s.Unclosed++
			rep.Unclosed++
			if f := inv.Floor(); f > 0 {
				s.FloorSec += f.Seconds()
				rep.UnclosedFloorSeconds += f.Seconds()
			} else {
				// No heartbeat: it died inside the first interval, or the log
				// predates heartbeats. Only this remainder is extrapolated.
				s.unheard++
			}
			continue
		}
		if inv.Errors > 0 {
			rep.StatementErrors++
		}
		if inv.ExitCode != 0 {
			s.ExitedNonZero++
			rep.ExitedNonZero++
		}
		d := inv.Duration()
		s.TotalDur += d
		durs[inv.Verb] = append(durs[inv.Verb], d)
		rep.WallSeconds += d.Seconds()
	}

	for verb, s := range stats {
		ds := durs[verb]
		sort.Slice(ds, func(i, j int) bool { return ds[i] < ds[j] })
		if len(ds) > 0 {
			s.MedianMS = ds[len(ds)/2].Milliseconds()
			// The median, not the mean: a boot that was killed early and one
			// that ran to a timeout are both in the unclosed set, and the mean
			// of the closed runs is the more easily skewed of the two. Only the
			// runs that left NO heartbeat are extrapolated — the rest carry a
			// measured floor instead.
			s.UnmeasuredSec = float64(s.unheard) * ds[len(ds)/2].Seconds()
		}
		s.TotalSec = s.TotalDur.Seconds()
		rep.UnmeasuredSeconds += s.UnmeasuredSec
		rep.ByVerb = append(rep.ByVerb, *s)
	}
	// Most invocations first; ties by name so the output is stable.
	sort.Slice(rep.ByVerb, func(i, j int) bool {
		if rep.ByVerb[i].Count != rep.ByVerb[j].Count {
			return rep.ByVerb[i].Count > rep.ByVerb[j].Count
		}
		return rep.ByVerb[i].Verb < rep.ByVerb[j].Verb
	})

	for _, s := range stats {
		rep.Invocations += s.Count
	}
	rep.CheckExecDup = countCheckThenExec(invs)
	if len(invs) > 0 {
		rep.Span = invs[0].Start.Format(time.RFC3339) + " .. " +
			invs[len(invs)-1].Start.Format(time.RFC3339)
	}
	return rep
}

// countCheckThenExec counts a `check` immediately followed by an `exec` of the
// SAME script — the two-call form the generated CLAUDE.md used to teach.
//
// It is deliberately not called "waste": since ako/mxcli#607 `exec` resolves
// references itself, so what the extra call still buys is project-conflict
// detection (a plain CREATE over an existing document), which exec does not do.
// The number is here to be weighed, not to be eliminated on sight.
func countCheckThenExec(invs []invocation) int {
	n := 0
	for i := 0; i+1 < len(invs); i++ {
		a, b := invs[i], invs[i+1]
		if a.Verb == "check" && b.Verb == "exec" && a.Script != "" && a.Script == b.Script {
			n++
		}
	}
	return n
}

// readLogLines reads every log file in dir, oldest first, so the records come
// back in time order across a multi-day retention window.
func readLogLines(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasPrefix(e.Name(), "mxcli-") && strings.HasSuffix(e.Name(), ".log") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names) // mxcli-YYYY-MM-DD.log sorts chronologically

	var lines []string
	for _, name := range names {
		f, err := os.Open(filepath.Join(dir, name))
		if err != nil {
			continue
		}
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024) // argv can be long
		for sc.Scan() {
			lines = append(lines, sc.Text())
		}
		f.Close()
	}
	return lines, nil
}

func renderLoopReport(rep loopReport, w io.Writer) {
	fmt.Fprintf(w, "mxcli invocations: %d", rep.Invocations)
	if rep.Span != "" {
		fmt.Fprintf(w, "   (%s)", rep.Span)
	}
	fmt.Fprintln(w)
	if rep.Invocations == 0 {
		fmt.Fprintln(w, "\nNo session records found. Logging is on unless MXCLI_LOG=0,")
		fmt.Fprintln(w, "and logs are kept for 7 days.")
		return
	}
	fmt.Fprintf(w, "Wall time in mxcli: %.1fs measured, across the runs that closed\n", rep.WallSeconds)
	if rep.Unclosed > 0 {
		// The headline fix for item 2d. The measured total was the only number
		// on the page, and it is computed over the runs that closed — 4 of 30
		// `run` invocations on one measured project, so it read as 27% of the
		// session's mxcli time when the real figure was several times that.
		// Printing the estimate beside it, named as an estimate, is what makes
		// the restart bill visible without asserting a number nobody measured.
		if rep.UnclosedFloorSeconds > 0 {
			fmt.Fprintf(w, "Unclosed, at least: %.1fs more, measured from the heartbeats those\n"+
				"               runs wrote before they died (a lower bound: each stopped\n"+
				"               somewhere after its last one)\n", rep.UnclosedFloorSeconds)
		}
		if rep.UnmeasuredSeconds > 0 {
			fmt.Fprintf(w, "Not measured:       ~%.1fs more, estimated for the run(s) that left no\n"+
				"               heartbeat, at their command's median. An ESTIMATE, and\n"+
				"               deliberately not added to the measured total above\n",
				rep.UnmeasuredSeconds)
		}
		if rep.UnclosedFloorSeconds == 0 && rep.UnmeasuredSeconds == 0 {
			fmt.Fprintf(w, "Not measured:       %d run(s) that never closed, with no heartbeat and\n"+
				"               no closed run of the same command to estimate from\n", rep.Unclosed)
		}
		fmt.Fprintf(w, "Did not close: %d (a process that vanished: killed, or an os.Exit\n"+
			"               inside a command. A run that returns an error closes and is\n"+
			"               counted below instead, with its time)\n", rep.Unclosed)
	}
	// Measured, unlike the unclosed runs above: these ended and said so.
	if rep.ExitedNonZero > 0 {
		fmt.Fprintf(w, "Exited non-zero: %d (ran to the end and failed — their wall time is\n"+
			"               in the measured total)\n", rep.ExitedNonZero)
	}
	// Printed only when non-zero. It is near-always zero, and a "0" next to the
	// unclosed count reads as "nothing failed" — which is the opposite of what
	// the two numbers together mean.
	if rep.StatementErrors > 0 {
		fmt.Fprintf(w, "Finished with failed statements: %d (ran to the end and reported\n"+
			"               errors — `exec --continue-on-error`. Separate from the\n"+
			"               unclosed runs above, which exited instead)\n", rep.StatementErrors)
	}

	if rep.Spawned > 0 {
		fmt.Fprintf(w, "Started by mxcli itself: %d (test, new, eval and the LSP run mxcli;\n"+
			"               excluded above — their time is already inside the run\n"+
			"               that started them)\n", rep.Spawned)
	}

	fmt.Fprintln(w, "\nBy command, most calls first:")
	fmt.Fprintf(w, "  %-20s %6s %8s %6s %9s %8s %9s %9s\n",
		"COMMAND", "CALLS", "UNCLOSED", "FAIL", "TOTAL", "MEDIAN", "+FLOOR", "+EST")
	for _, s := range rep.ByVerb {
		fmt.Fprintf(w, "  %-20s %6d %8d %6d %8.1fs %7dms %8.1fs %8.1fs\n",
			s.Verb, s.Count, s.Unclosed, s.ExitedNonZero,
			s.TotalSec, s.MedianMS, s.FloorSec, s.UnmeasuredSec)
	}
	fmt.Fprintln(w, "  TOTAL and +FLOOR are measured — the second from the unclosed runs'")
	fmt.Fprintln(w, "  heartbeats. +EST extrapolates only the unclosed runs that left none,")
	fmt.Fprintln(w, "  at this verb's median, so a command with no closed run reports 0.")

	if rep.CheckExecDup > 0 {
		fmt.Fprintf(w, "\n`check` immediately followed by `exec` of the same script: %d\n", rep.CheckExecDup)
		fmt.Fprintln(w, "  Since #607 exec resolves references itself, so the second call buys")
		fmt.Fprintln(w, "  project-conflict detection and the no-apply preflight. Worth weighing,")
		fmt.Fprintln(w, "  not eliminating on sight.")
	}

	fmt.Fprintln(w, "\nWhat this cannot tell you:")
	fmt.Fprintln(w, "  - model calls. One bash call can run several mxcli commands, and the")
	fmt.Fprintln(w, "    agent's other calls are not here at all. This counts mxcli processes.")
	fmt.Fprintln(w, "  - output size. Nothing records how many bytes a command printed, which")
	fmt.Fprintln(w, "    is the other half of the bill.")
	fmt.Fprintln(w, "  - reloads vs restarts. A long-running `run --local` is one invocation")
	fmt.Fprintln(w, "    however many times it hot-applies a change.")
	fmt.Fprintf(w, "  - exactly when a killed process stopped. Nothing is written at the kill,\n"+
		"    so an unclosed run is bounded by its last heartbeat (every %s) and the\n"+
		"    remainder is unknowable.\n", diaglog.HeartbeatInterval)
}

var diagLoopReportCmd = &cobra.Command{
	Use:   "loop-report",
	Short: "Report where this project's mxcli calls went, from the session logs",
	Long: `Summarise the session logs as a per-command call count, wall time and exit health.

Every mxcli invocation writes a session record naming its argv, so the shape of
an agent's loop is already on disk. This reports it: which commands were run,
how often, how long they took, and how many did not exit cleanly.

A run that is killed, or that leaves through a bare os.Exit inside a command,
writes no summary record, so it is counted as "did not close" rather than as a
failure — the report says which it is measuring rather than presenting one as
the other. The run commands (run, run status/stop/wait/restart) close their
session on every exit.

It counts mxcli PROCESSES, not model calls — one shell command can run several.
Read it to find which command dominates a loop, and re-run it after a change to
see whether the loop actually moved.

Examples:
  mxcli diag loop-report
  mxcli diag loop-report --json
  mxcli diag loop-report --log-dir ./collected-logs
`,
	Args: cobra.NoArgs,
	Run: func(cmd *cobra.Command, _ []string) {
		dir, _ := cmd.Flags().GetString("log-dir")
		if dir == "" {
			dir = diaglog.LogDir()
		}
		asJSON, _ := cmd.Flags().GetBool("json")

		lines, err := readLogLines(dir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading logs from %s: %v\n", dir, err)
			os.Exit(1)
		}
		rep := analyzeLoop(parseLogRecords(lines))

		if asJSON {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			if err := enc.Encode(rep); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			return
		}
		renderLoopReport(rep, os.Stdout)
	},
}

func init() {
	diagLoopReportCmd.Flags().Bool("json", false, "Emit the report as JSON")
	diagLoopReportCmd.Flags().String("log-dir", "", "Read logs from this directory instead of ~/.mxcli/logs")
	diagCmd.AddCommand(diagLoopReportCmd)
}
