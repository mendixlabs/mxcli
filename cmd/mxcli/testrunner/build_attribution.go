// SPDX-License-Identifier: Apache-2.0

// Attributing a failed build back to the test that caused it.
//
// An @expect the runner cannot compile is already an ERROR at parse time. What
// is left is everything only MxBuild decides — an undefined variable (CE0109), a
// String compared to a number (CE0117) — where nothing upstream of the build has
// the information to object.
//
// Checking those earlier was tried and abandoned. The microflow validator tracks
// variables where they are ASSIGNED, and reusing that model to check READS
// produces false refusals: `$latestHttpResponse` is a Mendix system variable that
// no MDL statement declares, and a loop iterator is only registered when the
// list's type is known. Both refuse a microflow `mx check` accepts at 0 errors.
// The scope model is right for the bar it was built for and wrong for this one,
// so the build stays the authority and this file makes its verdict legible.
//
// When that happens the build fails, the runtime never boots, and the run
// produces no test results at all. The failure is real; what was missing is
// saying which test caused it. MxBuild locates every problem
// (`module` + `document`), and the test microflows are generated with names this
// package chose, so the mapping back is exact rather than a guess.
//
// ako/mxcli-sudoku FINDINGS #46, follow-up.
package testrunner

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/mendixlabs/mxcli/cmd/mxcli/docker"
)

// buildFailure is the error every path in this package returns for a build
// MxBuild rejected.
//
// The type is load-bearing, not decoration. MxBuild puts the same sentence in
// Message for every failing build — "The project cannot be deployed, because it
// contains errors." — so an error carrying only that cannot tell "your test does
// not compile" from "an unrelated document in the project is broken". Two paths
// built their error with fmt.Errorf and threw the parsed problems away, which is
// the whole of mendixlabs/mxcli#1104's first half: the detail was already in
// hand and discarded one line before it was needed.
func buildFailure(build *docker.BuildResult) error {
	return &docker.BuildFailedError{Result: build}
}

// resultsForBuildFailure turns a failed build into something the reader can act
// on, and passes any other error through untouched.
//
// A build error belonging to a generated test microflow is that test's problem:
// it becomes an ERROR row and the rest become SKIP. Anything else is in the
// project, and the returned error names the documents so the reader is not sent
// looking through their own model for a message that came from mxcli's.
//
// Shared by both runners on purpose. It was wired into the --local boot only, so
// --attach and every rebuild under --watch reported the bare sentence.
func resultsForBuildFailure(err error, suite *TestSuite) (*SuiteResult, error) {
	var bf *docker.BuildFailedError
	if !errors.As(err, &bf) {
		return nil, err
	}
	if results := resultsFromFailedBuild(bf.BuildErrors(), suite); results != nil {
		return &SuiteResult{Name: suite.Name, Tests: results, Started: time.Now(), BuildFailed: true}, nil
	}
	_, other := attributeBuildProblems(bf.BuildErrors(), suite)
	return nil, fmt.Errorf("%w%s", err, buildFailureHint(other))
}

// testFlowDocumentPattern matches the `document` MxBuild reports for a generated
// test microflow.
//
// MxBuild names the document WITHOUT its module — `Microflow 'Test_test_3'`,
// with the module carried separately in the same location — so this matches the
// bare name and the module is checked alongside it. Anchored on the generated
// prefix, so an error in the user's own microflow is never blamed on a test.
var testFlowDocumentPattern = regexp.MustCompile(`^Microflow '(Test_[^']*)'$`)

// attributeBuildProblems splits a failed build's errors into those belonging to
// a generated test microflow and those that do not.
//
// The second group matters as much as the first: an error in the user's own
// model also fails the build, and reporting it as a test failure would send them
// looking in the wrong place.
func attributeBuildProblems(problems []docker.BuildProblem, suite *TestSuite) (map[string][]docker.BuildProblem, []docker.BuildProblem) {
	byTest := map[string][]docker.BuildProblem{}
	var other []docker.BuildProblem

	// Keyed on the BARE document name MxBuild reports, not the qualified one.
	known := map[string]string{} // "Test_test_3" -> test ID
	if suite != nil {
		for _, tc := range suite.Tests {
			known[strings.TrimPrefix(testFlowName(tc), mxTestModule+".")] = tc.ID
		}
	}

	for _, p := range problems {
		id := ""
		for _, loc := range p.Locations {
			if !strings.EqualFold(loc.Module, mxTestModule) {
				continue
			}
			m := testFlowDocumentPattern.FindStringSubmatch(loc.Document)
			if m == nil {
				continue
			}
			if tid, ok := known[m[1]]; ok {
				id = tid
				break
			}
		}
		if id == "" {
			other = append(other, p)
			continue
		}
		byTest[id] = append(byTest[id], p)
	}
	return byTest, other
}

// buildProblemMessage renders one test's build errors for its result row.
func buildProblemMessage(problems []docker.BuildProblem) string {
	parts := make([]string, 0, len(problems))
	for _, p := range problems {
		msg := p.Message
		if p.ErrorCode != "" {
			msg = p.ErrorCode + ": " + msg
		}
		if p.Locations != nil && p.Where() != "" {
			// Only the element is worth repeating here — the module and document
			// are the test itself, which the row already names.
			if el := p.Locations[0].Element; el != "" {
				msg += " (at " + el + ")"
			}
		}
		parts = append(parts, msg)
	}
	return buildProblemPrefix + strings.Join(parts, "; ")
}

// buildProblemPrefix starts the message of a test MxBuild rejected.
const buildProblemPrefix = "the assertion could not be built: "

// resultsFromFailedBuild turns a failed build into one result per test, so a run
// that cannot boot still reports something per test instead of nothing at all.
//
// A test MxBuild blamed becomes an ERROR carrying the consistency message. Every
// other test becomes a SKIP: they were not run and must not be counted as
// passing — the whole point of this area is that a test framework never reports
// green for something it did not evaluate.
//
// Returns nil when no error could be attributed to a test, so the caller reports
// the build failure as it always did rather than inventing per-test rows for a
// problem in the user's own model.
func resultsFromFailedBuild(problems []docker.BuildProblem, suite *TestSuite) []TestResult {
	byTest, _ := attributeBuildProblems(problems, suite)
	if len(byTest) == 0 || suite == nil {
		return nil
	}

	results := make([]TestResult, 0, len(suite.Tests))
	for _, tc := range suite.Tests {
		r := newResult(tc)
		if ps, ok := byTest[tc.ID]; ok {
			r.Status = StatusError
			r.Message = buildProblemMessage(ps)
		} else {
			r.Status = StatusSkip
			r.Message = "not run: another test in this run failed to build"
		}
		results = append(results, r)
	}
	return results
}

// buildFailureHint is appended to the error when a build failure could not be
// attributed to any test, which means it is in the project rather than in the
// suite — or in one of the documents the run generates besides its tests.
//
// The errors themselves are already rendered by BuildFailedError.Error(), so
// this says what the reader cannot work out from them: that none of them belongs
// to a test in this run, and — for a generated microflow no current test owns —
// that it is a leftover from an earlier run, with the command to remove it. That
// last case is the one that fails every subsequent run of every test file until
// someone finds it by hand (mendixlabs/mxcli#1104).
//
// An error in the endpoint documents is mxcli's own, and says so. CE0114 in
// MxTest.RegisterEndpoint used to be reported as "in the project", which sent
// the reader through their own model for a defect in a microflow they never
// wrote (ako/mxcli#1654).
func buildFailureHint(other []docker.BuildProblem) string {
	if len(other) == 0 {
		return ""
	}
	generated, project := splitRunnerDocuments(other)
	var b strings.Builder
	for _, name := range generated {
		b.WriteString(fmt.Sprintf("\n  %s is not part of your project: mxcli generated it for this run, %s."+
			" An error in it is a defect in mxcli, not in your project or your tests — please report it"+
			" (`mxcli diag --bundle` collects what is needed).", name, runnerDocuments[name]))
		if name == endpointStartupFlow {
			b.WriteString("\n  Until it is fixed, --skip-app-startup runs the tests without the call to your after-startup microflow.")
		}
	}
	if len(project) == 0 {
		return b.String()
	}
	if len(generated) > 0 {
		b.WriteString("\n  The other errors are in the project, not in this run's tests.")
	} else {
		b.WriteString("\n  These errors are in the project, not in this run's tests.")
	}
	for _, name := range leftoverFlowsIn(project) {
		b.WriteString(fmt.Sprintf(
			"\n  %s is a microflow an earlier `mxcli test` run left behind. Remove it with:"+
				"\n    mxcli -p <project.mpr> -c \"DROP MICROFLOW %s\"", name, name))
	}
	return b.String()
}

// runnerDocuments are the documents an endpoint run generates besides its test
// microflows, with what each is for. Their names are fixed, so an error located
// in one is the run's own whatever else is in the MxTest module.
var runnerDocuments = map[string]string{
	endpointStartupFlow:    "to register the test endpoint at startup and then run your after-startup microflow",
	endpointRegisterAction: "to register the test endpoint",
	errorMessageFlow:       "to read a raised error's message for the test microflows",
}

// runnerDocumentPattern matches the `document` MxBuild reports — the document's
// kind and its bare name, `Microflow 'RegisterEndpoint'` or
// `Java action 'RegisterTestEndpoint'`; the module is carried separately.
var runnerDocumentPattern = regexp.MustCompile(`^[A-Za-z ]+ '([^']*)'$`)

// splitRunnerDocuments separates the problems located in a runner document from
// the rest, naming each runner document once, in the order MxBuild reported it.
func splitRunnerDocuments(problems []docker.BuildProblem) (generated []string, rest []docker.BuildProblem) {
	seen := map[string]bool{}
	for _, p := range problems {
		name := ""
		for _, loc := range p.Locations {
			if !strings.EqualFold(loc.Module, mxTestModule) {
				continue
			}
			if m := runnerDocumentPattern.FindStringSubmatch(loc.Document); m != nil {
				if q := mxTestModule + "." + m[1]; runnerDocuments[q] != "" {
					name = q
					break
				}
			}
		}
		if name == "" {
			rest = append(rest, p)
			continue
		}
		if !seen[name] {
			seen[name] = true
			generated = append(generated, name)
		}
	}
	return generated, rest
}

// leftoverFlowsIn names the generated test microflows among a set of build
// problems, deduplicated and in the order MxBuild reported them.
//
// A document matching the generated prefix, in the generated module, that no
// test in this run owns, can only have come from an earlier run: the names are
// positional and nothing else in the project is allowed to use them.
func leftoverFlowsIn(problems []docker.BuildProblem) []string {
	var names []string
	seen := map[string]bool{}
	for _, p := range problems {
		for _, loc := range p.Locations {
			if !strings.EqualFold(loc.Module, mxTestModule) {
				continue
			}
			m := testFlowDocumentPattern.FindStringSubmatch(loc.Document)
			if m == nil {
				continue
			}
			name := mxTestModule + "." + m[1]
			if !seen[name] {
				seen[name] = true
				names = append(names, name)
			}
		}
	}
	return names
}
