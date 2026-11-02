// SPDX-License-Identifier: Apache-2.0

package testrunner

import (
	"bufio"
	"fmt"
	"io"
	"strings"
	"time"
)

// TestResult represents the outcome of a single test case.
type TestResult struct {
	ID       string        // Test ID
	Name     string        // Test name
	Status   TestStatus    // Pass, Fail, Skip, Error
	Message  string        // Failure/skip message
	Duration time.Duration // Execution time
	// Assertions is how many assertions the test made. A PASS with zero of them
	// says only that the body did not throw, and the output has to make that
	// visible — a suite whose green cannot be told apart from a vacuous one is
	// the failure this whole area exists to prevent.
	Assertions int
	// SourceFile is the test file the case came from, so a multi-file run's
	// report can say where a failure lives.
	SourceFile string
}

// newResult starts a result from its test case, carrying across everything the
// case already knows. Every construction site goes through this, so a new field
// on TestResult cannot be populated in one path and silently missing in another.
func newResult(tc TestCase) TestResult {
	return TestResult{
		ID:         tc.ID,
		Name:       tc.Name,
		Assertions: tc.AssertionCount(),
		SourceFile: tc.SourceFile,
	}
}

// TestStatus represents the outcome status of a test.
type TestStatus int

const (
	StatusPass TestStatus = iota
	StatusFail
	StatusSkip
	StatusError
)

func (s TestStatus) String() string {
	switch s {
	case StatusPass:
		return "PASS"
	case StatusFail:
		return "FAIL"
	case StatusSkip:
		return "SKIP"
	case StatusError:
		return "ERROR"
	default:
		return "UNKNOWN"
	}
}

// SuiteResult holds the results of an entire test suite execution.
type SuiteResult struct {
	Name     string
	Tests    []TestResult
	Duration time.Duration
	Started  time.Time
	// BuildFailed is set when MxBuild rejected a generated test microflow, so no
	// test ran: the ERROR rows carry the build errors and every other row is a
	// SKIP. PrintResults repeats those errors above the summary.
	BuildFailed bool
}

// PassCount returns the number of passing tests.
func (sr *SuiteResult) PassCount() int {
	n := 0
	for _, t := range sr.Tests {
		if t.Status == StatusPass {
			n++
		}
	}
	return n
}

// FailCount returns the number of failing tests.
func (sr *SuiteResult) FailCount() int {
	n := 0
	for _, t := range sr.Tests {
		if t.Status == StatusFail || t.Status == StatusError {
			n++
		}
	}
	return n
}

// resultNote renders the parenthetical after a test's name: how long it took and
// how much it actually asserted.
//
// The assertion count is here rather than buried in a --verbose mode because the
// whole lesson of the silent-pass defect is that a suite's ordinary output has to
// distinguish a test that asserted from one that did not.
func resultNote(t TestResult) string {
	var parts []string
	if t.Duration > 0 {
		parts = append(parts, t.Duration.Round(time.Millisecond).String())
	}
	switch {
	case t.Status == StatusSkip || t.Status == StatusError:
		// Neither reached its assertions, so a count would say nothing.
	case t.Assertions == 0:
		parts = append(parts, "no assertions")
	case t.Assertions == 1:
		parts = append(parts, "1 assertion")
	default:
		parts = append(parts, fmt.Sprintf("%d assertions", t.Assertions))
	}
	return strings.Join(parts, ", ")
}

// VacuousCount returns the number of tests that reached a verdict without
// asserting anything — a pass that means only "the body did not throw".
func (sr *SuiteResult) VacuousCount() int {
	n := 0
	for _, t := range sr.Tests {
		if t.Assertions == 0 && (t.Status == StatusPass || t.Status == StatusFail) {
			n++
		}
	}
	return n
}

// ErrorCount returns the number of tests that did not reach a verdict — an
// uncompilable @expect, a missing microflow, a failed request.
//
// It is reported separately from FailCount in the summary line. A suite whose
// output cannot distinguish "this assertion is false" from "this assertion was
// never evaluated" is how the silent-pass defect stayed invisible for weeks.
func (sr *SuiteResult) ErrorCount() int {
	n := 0
	for _, t := range sr.Tests {
		if t.Status == StatusError {
			n++
		}
	}
	return n
}

// SkipCount returns the number of skipped tests.
func (sr *SuiteResult) SkipCount() int {
	n := 0
	for _, t := range sr.Tests {
		if t.Status == StatusSkip {
			n++
		}
	}
	return n
}

// AllPassed returns true if all tests passed.
func (sr *SuiteResult) AllPassed() bool {
	return sr.FailCount() == 0
}

// ParseLogResults parses structured MXTEST: log lines from runtime output
// and matches them to the test suite's test cases.
// ParseLogResults assembles a suite result from the runtime log the
// after-startup runner leaves behind (the Docker path, and --local
// --legacy-runner).
//
// requireAssertions is threaded in rather than applied by the caller afterwards
// because the pre-run verdicts have to be decided in the same place as the
// log-derived ones: a test that is an ERROR before it runs must not also pick up
// a PASS from the log. It reached this function late — it was honoured only by
// the endpoint runner, so `mxcli test -p app.mpr --require-assertions` without
// --local accepted the flag and did nothing (issue #926).
func ParseLogResults(logReader io.Reader, suite *TestSuite, requireAssertions bool) *SuiteResult {
	result := &SuiteResult{
		Name:    suite.Name,
		Started: time.Now(),
	}

	// Build a map of test cases by ID for quick lookup
	testMap := make(map[string]*TestCase)
	for i := range suite.Tests {
		testMap[suite.Tests[i].ID] = &suite.Tests[i]
	}

	// Track which tests we've seen results for
	resultMap := make(map[string]*TestResult)
	runTimes := make(map[string]time.Time)
	// failures counts each test's FAIL and ERROR lines; see applyVerdictLine.
	failures := make(map[string]int)

	scanner := bufio.NewScanner(logReader)
	for scanner.Scan() {
		line := scanner.Text()

		// Find MXTEST: protocol lines.
		// Runtime logs look like: "MXTEST: MXTEST:PASS:test_1" where the first
		// "MXTEST:" is the log node and the second is our protocol prefix.
		// We search for specific protocol actions to avoid matching the log node.
		protocol := ""
		for _, action := range []string{"MXTEST:START:", "MXTEST:RUN:", "MXTEST:PASS:", "MXTEST:FAIL:", "MXTEST:ERROR:", "MXTEST:SKIP:", "MXTEST:END:"} {
			if idx := strings.Index(line, action); idx >= 0 {
				protocol = line[idx:]
				break
			}
		}
		if protocol == "" {
			continue
		}

		parts := strings.SplitN(protocol, ":", 4) // MXTEST:TYPE:id[:message]

		if len(parts) < 3 {
			continue
		}

		action := parts[1]
		id := parts[2]

		switch action {
		case "START":
			result.Started = time.Now()

		case "RUN":
			runTimes[id] = time.Now()
			// Extract name from RUN line: MXTEST:RUN:id:name
			name := id
			if len(parts) >= 4 {
				name = parts[3]
			}
			// Not the zero status, which is PASS. A test that logs RUN and no
			// verdict did not finish: an error left the runner flow, which
			// stops it here — a call whose own handler re-raises does exactly
			// that (ako/mxcli#1448). Its PASS or FAIL line replaces this.
			// A RUN line starts the test's verdict afresh, so a failure that
			// sticks (applyVerdictLine) cannot outlive the run it belongs to.
			delete(failures, id)
			resultMap[id] = &TestResult{
				ID:      id,
				Name:    name,
				Status:  StatusError,
				Message: "started and reported no verdict: an error left the test runner here, which stopped it (the runtime log has the error)",
			}

		case "PASS", "FAIL", "ERROR", "SKIP":
			// ERROR is a setup that threw: the test never ran, so it is neither a
			// pass nor a failure. Only the monolithic runner reports this way —
			// the endpoint carries the same outcome back as a SETUP verdict.
			msg := ""
			if len(parts) >= 4 {
				msg = parts[3]
			}
			r, ok := resultMap[id]
			if !ok {
				r = &TestResult{ID: id, Name: id}
				resultMap[id] = r
			}
			if action == "PASS" || action == "FAIL" {
				if t, ok := runTimes[id]; ok {
					r.Duration = time.Since(t)
				}
			}
			applyVerdictLine(r, verdictStatus[action], msg, failures)

		case "END":
			result.Duration = time.Since(result.Started)
		}
	}

	// Collect results in test order
	for _, tc := range suite.Tests {
		// Verdicts that are settled before the test runs. A test whose @expect
		// did not compile was never generated, so the log has nothing to say
		// about it; report the parse error rather than the generic "not
		// executed". Same for a vacuous test under --require-assertions.
		if res, bad := preRunResult(tc, requireAssertions); bad {
			result.Tests = append(result.Tests, res)
			continue
		}
		if r, ok := resultMap[tc.ID]; ok {
			// The log carries a status and a duration; everything else is known
			// from the test case and is stamped on here so this path reports the
			// same fields as the endpoint path.
			res := newResult(tc)
			if tc.Name == "" {
				res.Name = r.Name
			}
			res.Status = r.Status
			res.Message = r.Message
			if more := failures[tc.ID] - 1; more > 0 {
				noun := "failures"
				if more == 1 {
					noun = "failure"
				}
				res.Message += fmt.Sprintf(" (and %d more %s in this test)", more, noun)
			}
			res.Duration = r.Duration
			result.Tests = append(result.Tests, res)
		} else {
			// Test was not executed — mark as error
			res := newResult(tc)
			res.Status = StatusError
			res.Message = "Test was not executed (runtime may have crashed before reaching it)"
			result.Tests = append(result.Tests, res)
		}
	}

	return result
}

// verdictStatus maps a log protocol verdict action to the status it reports.
var verdictStatus = map[string]TestStatus{
	"PASS":  StatusPass,
	"FAIL":  StatusFail,
	"ERROR": StatusError,
	"SKIP":  StatusSkip,
}

// applyVerdictLine records one verdict line of the after-startup runner's log
// against a test's result. failures counts each test's FAIL and ERROR lines.
//
// A test can log more than one verdict line: writeExpectAssertion logs PASS or
// FAIL for every @expect on its own, and the skipped-assertions guard logs a
// FAIL after them. A failure is therefore sticky — once a test has a FAIL or an
// ERROR, no later line clears it, and the first failure's message is the one
// reported. When every line simply replaced the last, a test whose first
// assertion failed and whose second held reported PASS with the failure message
// erased, and `mxcli test` exited 0 (ako/mxcli#1753). The rule lives here rather
// than in the generated flow so it also holds for a log written by a runner
// generated before it.
//
// This is the endpoint runner's semantics: there an assertion is evaluated only
// while $Verdict is still PASS, and the skipped-assertions guard does not
// replace an existing failure, so its first failure is what it returns.
//
// The ERROR a RUN line leaves as a placeholder is not a failure line; the
// test's own verdict replaces it.
func applyVerdictLine(r *TestResult, status TestStatus, msg string, failures map[string]int) {
	failed := status == StatusFail || status == StatusError
	if failed {
		failures[r.ID]++
	}
	if failures[r.ID] > 1 || (!failed && failures[r.ID] > 0) {
		return
	}
	r.Status = status
	r.Message = msg
}

// PrintResults writes a human-readable summary to the writer.
func PrintResults(w io.Writer, result *SuiteResult, color bool) {
	fmt.Fprintf(w, "\n")
	fmt.Fprintf(w, "Test Results: %s\n", result.Name)
	fmt.Fprintf(w, "%s\n", strings.Repeat("=", 60))

	for _, t := range result.Tests {
		var statusStr string
		if color {
			switch t.Status {
			case StatusPass:
				statusStr = "\033[32mPASS\033[0m"
			case StatusFail:
				statusStr = "\033[31mFAIL\033[0m"
			case StatusSkip:
				statusStr = "\033[33mSKIP\033[0m"
			case StatusError:
				statusStr = "\033[31mERROR\033[0m"
			}
		} else {
			statusStr = t.Status.String()
		}

		fmt.Fprintf(w, "  %s  %s", statusStr, t.Name)
		if note := resultNote(t); note != "" {
			fmt.Fprintf(w, " (%s)", note)
		}
		fmt.Fprintln(w)

		if t.Message != "" && t.Status != StatusPass {
			fmt.Fprintf(w, "         %s\n", t.Message)
		}
	}

	// A vacuous test is not a failure, but it must never be silent: after the
	// @expect fix the cheapest way back to a green suite is to delete the
	// assertion, and that must not look like a repair.
	if n := result.VacuousCount(); n > 0 {
		fmt.Fprintf(w, "%s\n", strings.Repeat("-", 60))
		fmt.Fprintf(w, "%d test(s) asserted nothing beyond \"did not throw\". "+
			"Run with --require-assertions to make that an error.\n", n)
	}

	printBuildFailure(w, result)

	fmt.Fprintf(w, "%s\n", strings.Repeat("-", 60))
	errors := result.ErrorCount()
	fmt.Fprintf(w, "Total: %d  Passed: %d  Failed: %d",
		len(result.Tests), result.PassCount(), result.FailCount()-errors)
	if errors > 0 {
		fmt.Fprintf(w, "  Errors: %d", errors)
	}
	fmt.Fprintf(w, "  Skipped: %d", result.SkipCount())
	if result.Duration > 0 {
		fmt.Fprintf(w, "  Time: %s", result.Duration.Round(time.Millisecond))
	}
	fmt.Fprintln(w)

	if result.AllPassed() {
		if color {
			fmt.Fprintf(w, "\033[32mAll tests passed.\033[0m\n")
		} else {
			fmt.Fprintf(w, "All tests passed.\n")
		}
	} else {
		if color {
			fmt.Fprintf(w, "\033[31mSome tests failed.\033[0m\n")
		} else {
			fmt.Fprintf(w, "Some tests failed.\n")
		}
	}
}

// printBuildFailure repeats a failed build's errors directly above the summary.
//
// The ERROR rows already carry them, but a suite with one test that does not
// build has every other test as a SKIP row, and the error was one line among a
// hundred identical "not run" lines: the run ended in "Passed: 0, Errors: 1,
// Skipped: 108" and read as if every test had failed for no reason
// (ako/mxcli#1834). The summary is where the reader's eye lands, so the cause
// goes right above it.
func printBuildFailure(w io.Writer, result *SuiteResult) {
	if !result.BuildFailed {
		return
	}
	var failed []TestResult
	for _, t := range result.Tests {
		if t.Status == StatusError {
			failed = append(failed, t)
		}
	}
	if len(failed) == 0 {
		return
	}
	fmt.Fprintf(w, "%s\n", strings.Repeat("-", 60))
	fmt.Fprintf(w, "Build failed: MxBuild rejected %d test(s), so no test ran.\n", len(failed))
	for _, t := range failed {
		where := t.Name
		if t.SourceFile != "" {
			where += " (" + t.SourceFile + ")"
		}
		fmt.Fprintf(w, "  %s\n    %s\n", where, strings.TrimPrefix(t.Message, buildProblemPrefix))
	}
}

// assertionErrorResult turns a test whose assertions could not be compiled into
// an ERROR result.
//
// This is the fail-closed rule the whole annotation pipeline is built around: an
// assertion the runner cannot evaluate must never be able to report a pass. The
// original implementation dropped such a line during parsing, and a test with no
// assertions left passes as long as it does not throw — so a suite could report
// green while asserting nothing. ERROR is counted with the failures, so the run's
// exit code is non-zero.
func assertionErrorResult(tc TestCase) (TestResult, bool) {
	if len(tc.AssertionErrors) == 0 {
		return TestResult{}, false
	}
	res := newResult(tc)
	res.Status = StatusError
	res.Message = strings.Join(tc.AssertionErrors, "; ")
	return res, true
}

// vacuousResult turns a test that asserts nothing into an ERROR, but only when
// the run asked for that.
//
// It is opt-in because a smoke test — "this microflow runs without throwing" —
// is a legitimate thing to write, and is documented as such. What is not
// legitimate is a suite in which nobody can tell the two apart, and the summary
// line handles that unconditionally. This flag is for a project that has decided
// every test must assert.
func vacuousResult(tc TestCase, require bool) (TestResult, bool) {
	if !require || tc.AssertionCount() > 0 {
		return TestResult{}, false
	}
	res := newResult(tc)
	res.Status = StatusError
	res.Message = "the test has no assertions — it can only report that the body did not throw " +
		"(add an @expect, or drop --require-assertions)"
	return res, true
}

// preRunResult returns the verdict a test earns before it is executed at all —
// the ones that depend only on the parsed test case, never on the run.
//
// Both result-assembly loops go through here, and TestPreRunVerdictsHaveOneCallSite
// pins that they are its only callers. That is the actual fix for #926: the
// helpers below were correct and unit-tested throughout, but only the endpoint
// runner called vacuousResult, so --require-assertions was silently inert on the
// log runner. A verdict reachable from one runner and not the other is the
// defect; a single call site is what prevents the next one.
func preRunResult(tc TestCase, requireAssertions bool) (TestResult, bool) {
	if res, bad := assertionErrorResult(tc); bad {
		return res, true
	}
	return vacuousResult(tc, requireAssertions)
}
