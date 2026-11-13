// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"fmt"
	"os"
	osexec "os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/mendixlabs/mxcli/cmd/mxcli/projectreport"
	"github.com/mendixlabs/mxcli/cmd/mxcli/testrunner"
	"github.com/spf13/cobra"
)

// Evidence collection shared by the tests, performance and traceability
// reports: microflow test sources and results, journey logs, and the spans the
// running app exported while the journeys ran.

func evidenceFlags(cmd *cobra.Command, withTests bool) {
	if withTests {
		cmd.Flags().StringSlice("tests", nil, "Test files or directories (.test.mdl / .test.md); default: tests/ beside the project")
		cmd.Flags().String("junit", "", "Read microflow test results from this JUnit file (as `mxcli test --junit` writes it)")
		cmd.Flags().Bool("run", false, "Run the microflow tests first (`mxcli test <tests> --junit …`), then report")
		cmd.Flags().StringArray("test-arg", nil, "Extra argument for `mxcli test` with --run (repeatable), e.g. --test-arg=--local")
	}
	cmd.Flags().String("journeys", "", "Journey log (JSONL): one record per journey or step with its time window")
	cmd.Flags().StringSlice("spans", nil, "Span files or directories from `mxcli report collect-spans` (JSONL, OTLP JSON or .pb)")
	cmd.Flags().String("journeys-cmd", "", "Run this command (via sh -c) with a span collector listening; it writes its journey log to $MXCLI_JOURNEY_LOG")
	cmd.Flags().String("otlp-listen", "127.0.0.1:4318", "With --journeys-cmd: where the span collector listens (point `run --local --trace-otlp` here)")
	cmd.Flags().Duration("flush", 8*time.Second, "With --journeys-cmd: how long to keep collecting after the command ends (the agent exports in batches)")
	cmd.Flags().String("database", "", "The database state the journeys ran on (e.g. \"fresh seed 2026-11-12\"); overrides the journey log's")
}

// evidence is what the flags produced.
type evidence struct {
	sources  []projectreport.TestSource
	results  []projectreport.TestResult
	journeys *projectreport.JourneyLog
	spans    []projectreport.Span
	warnings []string
}

func collectEvidence(cmd *cobra.Command, projectPath string, withTests bool) (*evidence, error) {
	ev := &evidence{}
	if withTests {
		paths, _ := cmd.Flags().GetStringSlice("tests")
		if len(paths) == 0 {
			if def := filepath.Join(filepath.Dir(projectPath), "tests"); dirExists(def) {
				paths = []string{def}
			}
		}
		paths = resolveTestPaths(paths, projectPath)
		for _, p := range paths {
			var suite *testrunner.TestSuite
			var err error
			if dirExists(p) {
				suite, err = testrunner.ParseTestDir(p)
			} else {
				suite, err = testrunner.ParseTestFile(p)
			}
			if err != nil {
				return nil, fmt.Errorf("reading tests %s: %w", p, err)
			}
			for _, fe := range suite.FileErrors {
				ev.warnings = append(ev.warnings, fmt.Sprintf("Test file `%s` could not be parsed: %v", fe.Path, fe.Err))
			}
			for _, tc := range suite.Tests {
				ev.sources = append(ev.sources, projectreport.TestSource{
					Name: tc.Name, File: relTo(projectPath, tc.SourceFile),
					Calls: projectreport.CallsInMDL(tc.MDL, tc.Setups), Assertions: tc.AssertionCount(),
				})
			}
		}
		junit, _ := cmd.Flags().GetString("junit")
		if run, _ := cmd.Flags().GetBool("run"); run {
			if len(paths) == 0 {
				return nil, fmt.Errorf("--run: no tests found (pass --tests <dir>)")
			}
			if junit == "" {
				junit = filepath.Join(os.TempDir(), fmt.Sprintf("mxcli-report-junit-%d.xml", os.Getpid()))
				defer os.Remove(junit)
			}
			extra, _ := cmd.Flags().GetStringArray("test-arg")
			if err := runMxcliTest(projectPath, paths, junit, extra); err != nil {
				ev.warnings = append(ev.warnings, "`mxcli test` did not finish cleanly: "+err.Error())
			}
		}
		if junit != "" {
			res, err := projectreport.ReadJUnit(junit)
			if err != nil {
				return nil, err
			}
			ev.results = res
		}
	}

	journeysPath, _ := cmd.Flags().GetString("journeys")
	spanPaths, _ := cmd.Flags().GetStringSlice("spans")
	if jc, _ := cmd.Flags().GetString("journeys-cmd"); jc != "" {
		listen, _ := cmd.Flags().GetString("otlp-listen")
		flush, _ := cmd.Flags().GetDuration("flush")
		logPath, spansPath, err := runJourneysCollected(jc, listen, flush)
		if err != nil {
			return nil, err
		}
		journeysPath = logPath
		spanPaths = append(spanPaths, spansPath)
	}
	if journeysPath != "" {
		log, err := projectreport.ReadJourneyLog(journeysPath)
		if err != nil {
			return nil, err
		}
		ev.journeys = log
	}
	if len(spanPaths) > 0 {
		spans, err := projectreport.ReadSpans(spanPaths)
		if err != nil {
			return nil, err
		}
		ev.spans = spans
	}
	if db, _ := cmd.Flags().GetString("database"); db != "" {
		if ev.journeys == nil {
			ev.journeys = &projectreport.JourneyLog{}
		}
		ev.journeys.Database = db
	}
	return ev, nil
}

// runMxcliTest runs this binary's own `test` command, so the report uses
// exactly the runner (and flags) a user would.
func runMxcliTest(projectPath string, paths []string, junit string, extra []string) error {
	self, err := os.Executable()
	if err != nil {
		return err
	}
	args := append([]string{"test"}, paths...)
	args = append(args, "-p", projectPath, "--junit", junit)
	args = append(args, extra...)
	fmt.Fprintf(os.Stderr, "Running: mxcli %s\n", strings.Join(args, " "))
	c := osexec.Command(self, args...)
	c.Stdout, c.Stderr = os.Stderr, os.Stderr
	return c.Run()
}

// runJourneysCollected starts a span collector, runs the journey command with
// $MXCLI_JOURNEY_LOG and $MXCLI_OTLP_ENDPOINT set, keeps collecting for the
// flush period after it ends, and returns the journey log and span file.
func runJourneysCollected(command, listen string, flush time.Duration) (string, string, error) {
	dir, err := os.MkdirTemp("", "mxcli-journeys-")
	if err != nil {
		return "", "", err
	}
	logPath := filepath.Join(dir, "journeys.jsonl")
	spansPath := filepath.Join(dir, "spans.jsonl")
	col, err := projectreport.StartCollector(listen, spansPath)
	if err != nil {
		return "", "", err
	}
	fmt.Fprintf(os.Stderr, "Span collector on http://%s (the app must run with --trace-otlp http://%s)\n", col.Addr(), col.Addr())
	c := osexec.Command("sh", "-c", command)
	c.Env = append(os.Environ(), "MXCLI_JOURNEY_LOG="+logPath, "MXCLI_OTLP_ENDPOINT=http://"+col.Addr())
	c.Stdout, c.Stderr = os.Stderr, os.Stderr
	runErr := c.Run()
	fmt.Fprintf(os.Stderr, "Journeys finished; collecting spans for %s more…\n", flush)
	time.Sleep(flush)
	n := col.Count()
	if err := col.Close(); err != nil {
		return "", "", err
	}
	fmt.Fprintf(os.Stderr, "Collected %d span(s) into %s\n", n, spansPath)
	if _, err := os.Stat(logPath); err != nil {
		return "", "", fmt.Errorf("the journey command wrote no log to $MXCLI_JOURNEY_LOG (%v); it must append one JSON line per journey or step", runErr)
	}
	if runErr != nil {
		fmt.Fprintf(os.Stderr, "Warning: journey command exited with %v; reporting what it logged\n", runErr)
	}
	return logPath, spansPath, nil
}

// testRunnerLeftovers names test-runner files the project carries outside a
// run: the after-startup swap and uncommitted MxTest files. A commit taken while
// `mxcli test` runs captures the injected module (ako/mxcli#2223).
func testRunnerLeftovers(projectPath string) []string {
	var out []string
	if w := testrunner.LeftoverEndpointWarning(projectPath); w != "" {
		out = append(out, "Test-runner leftover: "+firstLineOf(w))
	}
	dir := filepath.Dir(projectPath)
	c := osexec.Command("git", "status", "--porcelain", "--untracked-files=all")
	c.Dir = dir
	if b, err := c.Output(); err == nil {
		var hits []string
		for _, line := range strings.Split(string(b), "\n") {
			if strings.Contains(strings.ToLower(line), "mxtest") {
				hits = append(hits, strings.TrimSpace(line))
			}
		}
		if len(hits) > 0 {
			if len(hits) > 5 {
				hits = append(hits[:5], fmt.Sprintf("… and %d more", len(hits)-5))
			}
			out = append(out, "Uncommitted test-runner files (the `MxTest` module, its Java action or theme files) are in the working tree: "+
				strings.Join(hits, "; ")+". Do not commit while `mxcli test` runs — the runner injects them for the run and removes them after.")
		}
	}
	return out
}

func dirExists(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.IsDir()
}

func relTo(projectPath, p string) string {
	if r, err := filepath.Rel(filepath.Dir(projectPath), p); err == nil && !strings.HasPrefix(r, "..") {
		return filepath.ToSlash(r)
	}
	return p
}

// ---------------------------------------------------------------- tests

var reportTestsCmd = newReportKindCmd(reportKindSpec{
	name:  "tests",
	short: "Test results plus measured microflow coverage per module",
	long: `A test report a team can read in a minute: what the microflow tests and the
browser journeys check, whether they passed, and which microflows they reach,
per module, with the gaps listed.

Coverage has five categories per microflow: reached by both layers, by
microflow tests only (static: what each test calls, through the call graph),
by journeys only (runtime: microflow spans inside a journey's time window), run
only at startup or on a schedule (not a gap), or by neither.

Journeys: run the app with 'mxcli run --local --trace-otlp http://127.0.0.1:4318',
then either pass --journeys-cmd "<command>" (this command collects spans while
it runs; the script appends its journey log to $MXCLI_JOURNEY_LOG), or collect
yourself with 'mxcli report collect-spans' and pass --journeys and --spans.

Warns when the working tree carries test-runner files: never commit while
'mxcli test' runs. Annotation keys: gap:<Module>, journey:<name>.`,
	example: `  mxcli report tests -p app.mpr --run -o tests.html
  mxcli report tests -p app.mpr --junit results.xml --journeys journeys.jsonl --spans spans.jsonl -o tests.html
  mxcli report tests -p app.mpr --junit results.xml --journeys-cmd "npx playwright test" --database "fresh seed"`,
	flags: func(cmd *cobra.Command) {
		scopeFlags(cmd)
		evidenceFlags(cmd, true)
	},
	collect: func(cmd *cobra.Command, projectPath string) (*projectreport.Envelope, error) {
		warnings := testRunnerLeftovers(projectPath)
		for _, w := range warnings {
			fmt.Fprintln(os.Stderr, "Warning: "+w)
		}
		ev, err := collectEvidence(cmd, projectPath, true)
		if err != nil {
			return nil, err
		}
		warnings = append(warnings, ev.warnings...)
		exec, cleanup, err := openReportCatalog(projectPath, "full")
		if err != nil {
			return nil, err
		}
		defer cleanup()
		data, err := projectreport.CollectTests(exec.Catalog(), projectreport.TestsInput{
			Scope: scopeOptions(cmd), Sources: ev.sources, Results: ev.results,
			Journeys: ev.journeys, Spans: ev.spans, StartupFlows: settingsMicroflows(exec),
		})
		if err != nil {
			return nil, err
		}
		if len(ev.sources) == 0 && len(ev.results) == 0 {
			warnings = append(warnings, "No microflow tests were found (no tests/ beside the project, no --tests, no --junit).")
		}
		return projectreport.NewEnvelope("tests", reportProjectName(projectPath), reportNow(), version, data,
			projectreport.TestsMethod, projectreport.TestsLimits, warnings)
	},
	render: projectreport.RenderTests,
})

var reportCollectSpansCmd = &cobra.Command{
	Use:   "collect-spans",
	Short: "Receive OpenTelemetry spans from a running app into a JSONL file",
	Long: `A minimal OTLP/HTTP trace receiver for the tests, performance and traceability
reports. Start it, run the app with 'mxcli run --local --trace-otlp
http://127.0.0.1:4318', drive the journeys, then stop it with Ctrl-C. Each span
is appended to the output file as it arrives (one JSON object per line), so the
file can be read while it is still collecting.

Accepts protobuf (what the runtime's agent sends) and JSON, gzip or plain.`,
	Example: `  mxcli report collect-spans -o spans.jsonl
  mxcli report collect-spans --listen 127.0.0.1:4318 -o spans.jsonl --duration 10m`,
	Args: cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		listen, _ := cmd.Flags().GetString("listen")
		out, _ := cmd.Flags().GetString("output")
		dur, _ := cmd.Flags().GetDuration("duration")
		col, err := projectreport.StartCollector(listen, out)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "Collecting spans on http://%s into %s (Ctrl-C to stop)\n", col.Addr(), out)
		waitForStop(dur)
		n := col.Count()
		_ = col.Close()
		fmt.Fprintf(os.Stderr, "Collected %d span(s)\n", n)
	},
}

func init() {
	reportCollectSpansCmd.Flags().String("listen", "127.0.0.1:4318", "Address to listen on")
	reportCollectSpansCmd.Flags().StringP("output", "o", "spans.jsonl", "File to append spans to")
	reportCollectSpansCmd.Flags().Duration("duration", 0, "Stop after this long (default: until Ctrl-C)")
	reportCmd.AddCommand(reportTestsCmd, reportCollectSpansCmd)
}

// waitForStop blocks until Ctrl-C / SIGTERM, or for d when d > 0.
func waitForStop(d time.Duration) {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if d > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, d)
		defer cancel()
	}
	<-ctx.Done()
}
