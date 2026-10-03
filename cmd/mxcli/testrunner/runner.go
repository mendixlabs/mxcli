// SPDX-License-Identifier: Apache-2.0

package testrunner

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/mendixlabs/mxcli/cmd/mxcli/docker"
)

const (
	// mxTestModule is the module Run generates the test runner into.
	mxTestModule = "MxTest"
	// mxTestRunner is the generated after-startup microflow.
	mxTestRunner = "MxTest.TestRunner"
)

// RunOptions configures the test runner.
type RunOptions struct {
	// ProjectPath is the path to the .mpr file.
	ProjectPath string

	// TestFiles is the list of test file paths to run.
	TestFiles []string

	// SkipBuild skips the MxBuild step (reuse existing deployment).
	SkipBuild bool

	// Local runs the app with mxcli's own local runtime (`run --local`) instead
	// of a Docker container, and drives the tests over the test endpoint.
	Local bool

	// LegacyRunner forces the after-startup mechanism on a local run — the suite
	// compiled into one startup microflow, results read back from the runtime
	// log. An escape hatch for the case where the endpoint misbehaves; the
	// Docker path uses this mechanism regardless.
	LegacyRunner bool

	// Watch keeps the runtime and the build server up and re-runs the suite on
	// every change to a test file or to the project's model, until interrupted.
	// Requires the test endpoint, so it is incompatible with LegacyRunner and
	// with the Docker path — both of which can only re-run by restarting.
	Watch bool

	// Attach runs against an app already started with
	// `mxcli run --local --test-endpoint`, skipping the boot entirely. The tests
	// then run against that app's database rather than a scratch one.
	Attach bool

	// SkipAppStartup stops the project's own after-startup microflow from running
	// during a --local test run.
	//
	// It normally does run: the generated startup flow registers the endpoint and
	// then chains it, so tests see the app in the state it actually boots into.
	// Set this when the suite wants an empty, deterministic baseline instead —
	// e.g. the app seeds demo data at startup and the tests are asserting on
	// counts.
	SkipAppStartup bool

	// ConstantOverrides are the constant values the app should run with, layered
	// over the defaults in the deployment. Resolved by the caller from the
	// project's configuration (and, in future, the higher layers of
	// docs/11-proposals/PROPOSAL_constant_values.md) so the runner stays a
	// carrier rather than a second place that decides precedence.
	//
	// Only --local uses these: --attach runs against an app someone else booted
	// and inherits ITS constants, and the Docker path configures the container.
	ConstantOverrides map[string]string

	// MxBuildPath overrides mxbuild resolution for a --local run, as
	// `run --local --mxbuild-path` does. Empty means resolve: Studio Pro's
	// bundled mxbuild on macOS/Windows, the cached CDN download on Linux.
	MxBuildPath string

	// DBType selects the database a --local run boots against, as
	// `run --local --db-type` does: empty or "postgresql" for the scratch
	// PostgreSQL this command has always used, "hsqldb" for the runtime's
	// built-in file database — no server, and nothing to provision. Pass it
	// through ResolveTestDBType rather than reading it raw, so it arrives in the
	// runtime's upper-case spelling and an unknown value is refused at the
	// command line instead of after a boot.
	DBType string

	// Timeout for runtime startup and test execution.
	Timeout time.Duration

	// JUnitOutput is the path for JUnit XML output (empty = no file output).
	JUnitOutput string

	// RequireAssertions turns a test that asserts nothing into an ERROR.
	//
	// Off by default: a smoke test — "this microflow runs without throwing" — is
	// a legitimate thing to write. The summary line reports vacuous tests either
	// way; this is for a project that has decided every test must assert.
	RequireAssertions bool

	// Verbose shows all runtime log output.
	Verbose bool

	// Color enables colored console output.
	Color bool

	// Stdout for output messages.
	Stdout io.Writer

	// Stderr for error output.
	Stderr io.Writer
}

// Run executes the test suite.
//
// There are two mechanisms, and which one is used follows from opts.Local:
//
//   - Local runs go through the test endpoint (runEndpoint). Boot registers an
//     HTTP handler and nothing else; each test is then invoked by name against a
//     runtime that stays up, and returns its verdict in the response.
//   - Docker runs go through the after-startup runner (runAfterStartup), which
//     compiles the suite into the project's after-startup microflow, restarts the
//     container, and recovers results from its log.
//
// The endpoint is the better mechanism — a re-run is an HTTP call rather than a
// restart, a failing test is a result rather than a failed boot, and results are
// returned rather than scraped. It is confined to the local path because it
// needs to hand the runtime a secret through its environment and to reach it on
// loopback, neither of which is wired through docker-compose yet.
func Run(opts RunOptions) (*SuiteResult, error) {
	w := opts.Stdout
	if w == nil {
		w = os.Stdout
	}
	stderr := opts.Stderr
	if stderr == nil {
		stderr = os.Stderr
	}

	if err := validateOptions(opts); err != nil {
		return nil, err
	}

	timeout := opts.Timeout
	if timeout == 0 {
		timeout = 5 * time.Minute
	}

	// Resolve the project path up front so everything derived from it — the
	// runtime log path, the deployment directory, the paths named in error
	// messages — is absolute and agrees.
	if opts.ProjectPath != "" && !filepath.IsAbs(opts.ProjectPath) {
		if abs, err := filepath.Abs(opts.ProjectPath); err == nil {
			opts.ProjectPath = abs
		}
	}

	fmt.Fprintln(w, "Parsing test files...")
	suite, err := parseTestFiles(opts.TestFiles)
	if err != nil {
		return nil, fmt.Errorf("parsing test files: %w", err)
	}
	fmt.Fprintf(w, "  Found %d test(s) in %d file(s)\n", len(suite.Tests), suite.FilesRead)
	for _, fe := range suite.FileErrors {
		fmt.Fprintf(w, "  ERROR %s: %v\n", fe.Path, fe.Err)
	}

	if len(suite.Tests) == 0 {
		// A directory whose every file is malformed is not an empty directory,
		// and must not be reported as one (#903).
		if len(suite.FileErrors) > 0 {
			return nil, fmt.Errorf("no tests could be parsed: %d file(s) failed to parse", len(suite.FileErrors))
		}
		return nil, emptySuiteError(opts.TestFiles)
	}

	result, err := dispatchRun(opts, suite, timeout, w)
	if result != nil {
		// Appended after execution so an unparseable file lands in the summary,
		// the JUnit report and the exit code alongside the tests that did run.
		result.Tests = append(result.Tests, suiteFileErrorResults(suite.FileErrors)...)
	}
	return result, err
}

// dispatchRun hands the suite to the runner the options select.
func dispatchRun(opts RunOptions, suite *TestSuite, timeout time.Duration, w io.Writer) (*SuiteResult, error) {
	if opts.Attach {
		return runAttached(opts, suite, timeout, w)
	}
	if opts.Local && !opts.LegacyRunner {
		return runEndpoint(opts, suite, timeout, w)
	}
	// @verify needs a seam after each test's call, and a reachable admin API to
	// query — neither of which the after-startup runner has: its tests execute
	// during boot and its results are recovered from the log. Refuse rather than
	// run the suite with those assertions quietly skipped.
	if err := rejectVerifyOnLegacyRunner(suite); err != nil {
		return nil, err
	}
	return runAfterStartup(opts, suite, timeout, w)
}

// rejectVerifyOnLegacyRunner refuses a suite the after-startup runner cannot
// fully evaluate.
func rejectVerifyOnLegacyRunner(suite *TestSuite) error {
	for _, tc := range suite.Tests {
		if len(tc.Verify) > 0 {
			return fmt.Errorf(
				"test %q uses @verify, which the after-startup runner cannot evaluate: "+
					"its tests run during boot, so there is no point at which to query the app. "+
					"Run with --local (the default test endpoint) or --attach", tc.Name)
		}
	}
	return nil
}

// validateOptions rejects combinations that cannot work, with a message that
// says what to do instead. Watching depends on re-invoking tests without a
// restart, which only the test endpoint can do.
func validateOptions(opts RunOptions) error {
	if opts.Attach {
		if opts.LegacyRunner {
			return fmt.Errorf("--attach cannot be combined with --legacy-runner: the after-startup runner can only run tests by restarting, which is what attaching avoids")
		}
		if opts.SkipBuild {
			return fmt.Errorf("--attach cannot be combined with --skip-build: the attached app must be rebuilt to pick up the test microflows")
		}
		// --attach implies a local app; requiring --local as well would be noise.
		return nil
	}
	if !opts.Watch {
		return nil
	}
	if !opts.Local {
		return fmt.Errorf("--watch requires --local: the Docker path can only re-run tests by restarting the container")
	}
	if opts.LegacyRunner {
		return fmt.Errorf("--watch cannot be combined with --legacy-runner: the after-startup runner can only re-run tests by restarting the runtime")
	}
	if opts.SkipBuild {
		return fmt.Errorf("--watch cannot be combined with --skip-build: watching exists to rebuild on every change")
	}
	return nil
}

// runEndpoint injects the test endpoint plus one microflow per test, boots the
// app once, and drives the suite over HTTP.
func runEndpoint(opts RunOptions, suite *TestSuite, timeout time.Duration, w io.Writer) (*SuiteResult, error) {
	token, err := newEndpointToken()
	if err != nil {
		return nil, err
	}

	// Capture what cleanup will need to restore, before touching anything. This
	// must succeed: without it cleanup cannot tell an existing MxTest module from
	// the one it is about to create, nor restore the original after-startup — and
	// the generated startup flow needs to know what to chain.
	state, err := captureProjectState(opts.ProjectPath)
	if err != nil {
		return nil, fmt.Errorf("capturing project state: %w", err)
	}

	fmt.Fprintln(w, "Generating test endpoint and test microflows...")
	chain := state.afterStartup
	if opts.SkipAppStartup {
		chain = ""
	}
	endpointMDL := GenerateEndpointMDL(chain)
	flowsMDL := GenerateTestFlows(suite)

	if opts.Verbose {
		fmt.Fprintln(w, "--- Generated MDL ---")
		fmt.Fprintln(w, endpointMDL)
		fmt.Fprintln(w, flowsMDL)
		fmt.Fprintln(w, "--- End MDL ---")
	}

	fmt.Fprintln(w, "Injecting test endpoint into project...")

	// From here on the project is modified, so every exit runs cleanup.
	//
	// cleanupSuite, not the suite captured here: --watch re-parses on every
	// change, so by the time cleanup runs the set of injected test microflows may
	// differ from the one injected at boot. Dropping the wrong list would leave
	// generated microflows in the user's project.
	cleanupSuite := suite
	finish := func(result *SuiteResult, runErr error) (*SuiteResult, error) {
		fmt.Fprintln(w, "Cleaning up...")
		cleanupErr := cleanupEndpoint(opts.ProjectPath, state, cleanupSuite, w)
		removeGeneratedJavaSource(opts.ProjectPath, w)
		restoreProjectFile(state, cleanupErr, w)
		reportCleanup(w, cleanupErr, survivingTestFlows(opts.ProjectPath))
		if cleanupErr == nil {
			fmt.Fprintln(w, "  project restored")
		}
		if runErr != nil {
			return nil, runErr
		}
		if cleanupErr != nil {
			return result, fmt.Errorf("cleanup failed, project left modified: %w", cleanupErr)
		}
		return result, nil
	}

	if err := execMDLScript(opts.ProjectPath, endpointMDL, "mxtest-endpoint-*.mdl"); err != nil {
		return finish(nil, fmt.Errorf("injecting test endpoint: %w", err))
	}
	if err := execMDLScript(opts.ProjectPath, flowsMDL, "mxtest-flows-*.mdl"); err != nil {
		return finish(nil, fmt.Errorf("injecting test microflows: %w", err))
	}
	for _, cmd := range setupCommands(endpointStartupFlow) {
		if err := execMxcliCmd(opts.ProjectPath, cmd); err != nil {
			return finish(nil, fmt.Errorf("preparing project for the test run (%s): %w", cmd, err))
		}
	}
	fmt.Fprintln(w, "  "+describeStartup(state.afterStartup, opts.SkipAppStartup))

	// --watch keeps the runtime and the build server up and re-runs on every
	// change, so it owns the loop — including printing each run's results, which
	// it must do before cleanup rather than after. It reports each re-injected
	// suite back so cleanup drops what is actually in the project.
	if opts.Watch {
		return runEndpointWatch(opts, suite, token, timeout, w, finish, func(s *TestSuite) { cleanupSuite = s })
	}

	result, err := runViaEndpoint(opts, suite, token, timeout, w)
	if err != nil {
		return finish(nil, err)
	}

	result, err = finish(result, nil)
	if result != nil {
		PrintResults(w, result, opts.Color)
		if jerr := writeJUnit(opts, result, w); jerr != nil && err == nil {
			err = jerr
		}
	}
	return result, err
}

// runAfterStartup is the original mechanism: compile the suite into the
// after-startup microflow, restart the runtime, and read results out of the log.
// Always the Docker path, and the local path under --legacy-runner.
func runAfterStartup(opts RunOptions, suite *TestSuite, timeout time.Duration, w io.Writer) (*SuiteResult, error) {
	fmt.Fprintln(w, "Generating test runner microflow...")
	runnerMDL := GenerateTestRunner(suite)

	if opts.Verbose {
		fmt.Fprintln(w, "--- Generated MDL ---")
		fmt.Fprintln(w, runnerMDL)
		fmt.Fprintln(w, "--- End MDL ---")
	}

	// Save original settings and inject the test runner.
	fmt.Fprintln(w, "Injecting test runner into project...")
	state, err := captureProjectState(opts.ProjectPath)
	if err != nil {
		return nil, fmt.Errorf("capturing project state: %w", err)
	}

	if err := execMDLScript(opts.ProjectPath, runnerMDL, "mxtest-runner-*.mdl"); err != nil {
		return nil, fmt.Errorf("injecting test runner: %w", err)
	}

	// Set after-startup microflow
	for _, cmd := range setupCommands(mxTestRunner) {
		if err := execMxcliCmd(opts.ProjectPath, cmd); err != nil {
			return nil, fmt.Errorf("preparing project for the test run (%s): %w", cmd, err)
		}
	}
	fmt.Fprintf(w, "  After-startup set to %s\n", mxTestRunner)

	var logOutput string
	if opts.Local {
		logOutput, err = runLocalAndCapture(opts, timeout, w)
	} else {
		logOutput, err = runDockerAndCapture(opts, timeout, w)
	}
	if err != nil {
		cleanupErr := cleanup(opts.ProjectPath, state, w)
		restoreProjectFile(state, cleanupErr, w)
		reportCleanup(w, cleanupErr, survivingTestFlows(opts.ProjectPath))
		return nil, err
	}

	fmt.Fprintln(w, "Parsing test results...")
	result := ParseLogResults(strings.NewReader(logOutput), suite, opts.RequireAssertions)

	fmt.Fprintln(w, "Cleaning up...")
	cleanupErr := cleanup(opts.ProjectPath, state, w)
	restoreProjectFile(state, cleanupErr, w)
	reportCleanup(w, cleanupErr, survivingTestFlows(opts.ProjectPath))

	PrintResults(w, result, opts.Color)

	if err := writeJUnit(opts, result, w); err != nil {
		return result, err
	}

	// A failed cleanup leaves the project modified, so the run must not be
	// reported as clean even when every test passed.
	if cleanupErr != nil {
		return result, fmt.Errorf("cleanup failed, project left modified: %w", cleanupErr)
	}
	return result, nil
}

// writeJUnit writes the JUnit XML report when one was asked for.
func writeJUnit(opts RunOptions, result *SuiteResult, w io.Writer) error {
	if opts.JUnitOutput == "" {
		return nil
	}
	f, err := os.Create(opts.JUnitOutput)
	if err != nil {
		return fmt.Errorf("creating JUnit output: %w", err)
	}
	defer f.Close()
	if err := WriteJUnitXML(f, result); err != nil {
		return fmt.Errorf("writing JUnit XML: %w", err)
	}
	fmt.Fprintf(w, "JUnit XML written to: %s\n", opts.JUnitOutput)
	return nil
}

// ListTests parses test files and prints the test names without executing.
func ListTests(files []string, w io.Writer) error {
	suite, err := parseTestFiles(files)
	if err != nil {
		return err
	}

	fmt.Fprintf(w, "Found %d test(s):\n", len(suite.Tests))
	for _, tc := range suite.Tests {
		fmt.Fprintf(w, "  %s: %s\n", tc.ID, tc.Name)
		for _, flow := range tc.Setups {
			fmt.Fprintf(w, "    @setup %s\n", flow)
		}
		for _, exp := range tc.Expects {
			fmt.Fprintf(w, "    @expect %s\n", exp.Raw)
		}
		for _, v := range tc.Verify {
			fmt.Fprintf(w, "    @verify %s\n", v.Raw)
		}
		if tc.expectsThrow() {
			if tc.Throws == "" {
				fmt.Fprintf(w, "    @throws\n")
			} else {
				fmt.Fprintf(w, "    @throws '%s'\n", tc.Throws)
			}
		}
		for _, e := range tc.AssertionErrors {
			fmt.Fprintf(w, "    ERROR: %s\n", e)
		}
		if tc.AssertionCount() == 0 && len(tc.AssertionErrors) == 0 {
			fmt.Fprintf(w, "    (no assertions — this test can only report that the body did not throw)\n")
		}
	}
	return reportFileErrors(suite.FileErrors, w)
}

// reportFileErrors prints the files that could not be parsed and returns an
// error when there were any, so listing a partly-readable directory exits
// non-zero instead of looking like a clean listing.
func reportFileErrors(errs []FileError, w io.Writer) error {
	if len(errs) == 0 {
		return nil
	}
	fmt.Fprintf(w, "\n%d file(s) could not be parsed:\n", len(errs))
	for _, fe := range errs {
		fmt.Fprintf(w, "  ERROR %s: %v\n", fe.Path, fe.Err)
	}
	return fmt.Errorf("%d test file(s) could not be parsed", len(errs))
}

// suiteFileErrorResults turns unparseable files into ERROR results so they are
// counted in the summary and in the JUnit report, and so the run's exit code is
// non-zero — the same fail-closed handling assertionErrorResult gives an
// @expect that cannot be compiled.
func suiteFileErrorResults(errs []FileError) []TestResult {
	out := make([]TestResult, 0, len(errs))
	for _, fe := range errs {
		out = append(out, TestResult{
			Name:       fmt.Sprintf("%s (file could not be parsed)", filepath.Base(fe.Path)),
			Status:     StatusError,
			Message:    fe.Err.Error(),
			SourceFile: fe.Path,
		})
	}
	return out
}

// parseTestFiles parses one or more test files or directories.
func parseTestFiles(paths []string) (*TestSuite, error) {
	combined := &TestSuite{
		Name: "mxtest",
	}

	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil {
			return nil, fmt.Errorf("stat %s: %w", path, err)
		}

		if info.IsDir() {
			dirSuite, err := ParseTestDir(path)
			if err != nil {
				return nil, err
			}
			if combined.Name == "mxtest" && dirSuite.Name != "" {
				combined.Name = dirSuite.Name
			}
			combined.Tests = append(combined.Tests, dirSuite.Tests...)
			combined.FileErrors = append(combined.FileErrors, dirSuite.FileErrors...)
			combined.FilesRead += dirSuite.FilesRead
		} else {
			combined.FilesRead++
			fileSuite, err := ParseTestFile(path)
			if err != nil {
				// Isolated rather than fatal, for the same reason as in a
				// directory (#903): naming three files should not cost the two
				// that parse. The run is still red — see suiteFileErrorResults.
				combined.FileErrors = append(combined.FileErrors, FileError{Path: path, Err: err})
				continue
			}
			if combined.Name == "mxtest" && fileSuite.Name != "" {
				combined.Name = fileSuite.Name
			}
			combined.Tests = append(combined.Tests, fileSuite.Tests...)
		}
	}

	// Re-number test IDs to be globally unique
	for i := range combined.Tests {
		combined.Tests[i].ID = fmt.Sprintf("test_%d", i+1)
	}

	// One suite is one generated script, so its tests share one language
	// version (ako/mxcli#847).
	if _, err := suiteLanguageVersion(combined.Tests); err != nil {
		return nil, err
	}

	return combined, nil
}

// projectState records what Run changed in the project, captured before the first
// mutation so cleanup can put things back exactly rather than guessing.
type projectState struct {
	// snapshot is the project file as it was before anything was injected,
	// restored on the way out so a run that changes nothing leaves it
	// byte-identical.
	snapshot projectSnapshot

	// afterStartup is the project's original after-startup microflow ("" = none).
	afterStartup string
	// createdMxTest reports whether Run created the MxTest module, i.e. it did not
	// already exist. A pre-existing MxTest module belongs to the user and must
	// survive cleanup with everything but the generated TestRunner intact.
	createdMxTest bool
}

// captureProjectState reads everything cleanup needs to restore, before anything
// is injected.
func captureProjectState(projectPath string) (projectState, error) {
	var st projectState
	af, err := getAfterStartup(projectPath)
	if err != nil {
		return st, fmt.Errorf("reading after-startup setting: %w", err)
	}
	st.afterStartup = af

	exists, err := moduleExists(projectPath, mxTestModule)
	if err != nil {
		return st, fmt.Errorf("listing modules: %w", err)
	}
	st.createdMxTest = !exists

	st.snapshot = takeProjectSnapshot(projectPath)
	return st, nil
}

// restoreProjectFile puts the .mpr back byte-for-byte once cleanup has genuinely
// undone every injection, so a test run is a no-op on disk. See projectSnapshot
// for why restoring the model's content is not enough on its own.
func restoreProjectFile(st projectState, cleanupErr error, w io.Writer) {
	if err := st.snapshot.restore(cleanupErr == nil); err != nil {
		fmt.Fprintf(w, "  note: could not restore the project file: %v\n", err)
	}
}

// getAfterStartup reads the current after-startup microflow setting.
func getAfterStartup(projectPath string) (string, error) {
	mxcliPath, err := findMxcli()
	if err != nil {
		return "", err
	}

	cmd := exec.Command(mxcliPath, "-p", projectPath, "-c", "DESCRIBE SETTINGS")
	cmd.Env = append(os.Environ(), "MXCLI_QUIET=1")
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}

	for _, line := range strings.Split(string(output), "\n") {
		if strings.Contains(line, "AfterStartupMicroflow") {
			return parseSettingValue(line), nil
		}
	}
	return "", nil
}

// parseSettingValue extracts the value from one DESCRIBE SETTINGS line, e.g.
//
//	AfterStartupMicroflow = 'Module.Name',
//
// DESCRIBE SETTINGS separates properties with commas and ends the statement with
// a semicolon, so the trailing punctuation must come off *before* the quotes:
// trimming quotes first stops at the comma and leaves `Module.Name',`, which was
// then re-interpolated into unparseable MDL and silently failed to restore the
// setting (mendixlabs/mxcli#803).
func parseSettingValue(line string) string {
	val := strings.TrimSpace(line)
	if _, after, found := strings.Cut(val, "="); found {
		val = after
	}
	val = strings.TrimSpace(val)
	val = strings.TrimRight(val, ",;")
	val = strings.TrimSpace(val)
	return strings.Trim(val, "'\"")
}

// quoteMDLString renders a value as an MDL single-quoted literal. Mendix escapes
// an embedded quote by doubling it; a qualified name should never contain one, but
// emitting a broken literal is how #803 turned a parse slip into a mutated project.
func quoteMDLString(v string) string {
	return "'" + strings.ReplaceAll(v, "'", "''") + "'"
}

// moduleExists reports whether the project has a module with the given name.
func moduleExists(projectPath, name string) (bool, error) {
	mxcliPath, err := findMxcli()
	if err != nil {
		return false, err
	}
	cmd := exec.Command(mxcliPath, "-p", projectPath, "-c", "SHOW MODULES", "--json")
	cmd.Env = append(os.Environ(), "MXCLI_QUIET=1")
	output, err := cmd.Output()
	if err != nil {
		return false, err
	}
	var modules []struct {
		Module string `json:"Module"`
	}
	if err := json.Unmarshal(output, &modules); err != nil {
		return false, fmt.Errorf("parsing module list: %w", err)
	}
	for _, m := range modules {
		if strings.EqualFold(m.Module, name) {
			return true, nil
		}
	}
	return false, nil
}

// setupCommands returns the MDL statements Run issues to put the project into its
// testing state, pointing after-startup at startupFlow. The project's Security
// Level is deliberately absent: the after-startup microflow runs in an
// administrative context and is not subject to it, so forcing it OFF bought
// nothing — while breaking any project with a published REST/OData service using
// custom authentication ("App security is off, but custom authentication is
// enabled for this service"), and the restore hardcoded PRODUCTION, silently
// changing projects that run at another level (mendixlabs/mxcli#802).
func setupCommands(startupFlow string) []string {
	return []string{
		"ALTER SETTINGS RUNTIME (AfterStartupMicroflow: " + quoteMDLString(startupFlow) + ")",
	}
}

// runMDLCommands executes each statement, attempting all of them even after one
// fails, and joins the failures. Restores must not stop at the first error: a
// half-restored project is worse than a fully failed one, because it looks fine
// (#803).
func runMDLCommands(projectPath string, cmds []string) error {
	var errs []error
	for _, cmd := range cmds {
		if err := execMxcliCmd(projectPath, cmd); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", cmd, err))
		}
	}
	return errors.Join(errs...)
}

// lowerModule maps a module name to the javasource/ directory Mendix generates
// for it, which is always lowercased.
func lowerModule(name string) string { return strings.ToLower(name) }

// execMDLScript writes MDL to a temp file and executes it against the project.
func execMDLScript(projectPath, mdl, namePattern string) error {
	f, err := os.CreateTemp("", namePattern)
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}
	path := f.Name()
	defer os.Remove(path)

	if _, err := f.WriteString(mdl); err != nil {
		f.Close()
		return fmt.Errorf("writing MDL: %w", err)
	}
	f.Close()

	return execMxcli(projectPath, "exec", path, "-p", projectPath)
}

// cleanupCommands returns the MDL statements that put the project back the way it
// was, in order. Kept separate from execution so the restore can be tested without
// a project. mxTestPresent says whether the generated module is still there —
// nothing is dropped when it is already gone, so a run that failed before the
// injection landed does not report a spurious cleanup failure.
func cleanupCommands(st projectState, mxTestPresent bool) []string {
	// Restore the original after-startup microflow, or clear it if there was none.
	restore := "ALTER SETTINGS RUNTIME (AfterStartupMicroflow: '')"
	if st.afterStartup != "" {
		restore = "ALTER SETTINGS RUNTIME (AfterStartupMicroflow: " + quoteMDLString(st.afterStartup) + ")"
	}
	cmds := []string{restore}
	if !mxTestPresent {
		return cmds
	}

	// Remove the generated runner. Drop the whole module only when Run created it;
	// a pre-existing MxTest module is the user's, so only the generated microflow
	// comes out of it.
	if st.createdMxTest {
		cmds = append(cmds, "DROP MODULE "+mxTestModule)
	} else {
		cmds = append(cmds, "DROP MICROFLOW "+mxTestRunner)
	}
	return cmds
}

// cleanup restores the project to the state captured before injection and removes
// the generated test runner.
//
// Every statement is attempted even if an earlier one fails, and the failures are
// returned rather than printed as warnings: a half-restored project is left with
// its after-startup pointing at a microflow this function is about to delete, and
// that has to be loud (#803).
func cleanup(projectPath string, st projectState, w io.Writer) error {
	// Re-check rather than assume: on failure fall back to attempting the drop, so
	// a genuine problem still surfaces instead of being skipped.
	mxTestPresent := true
	if exists, err := moduleExists(projectPath, mxTestModule); err == nil {
		mxTestPresent = exists
	}
	if mxTestPresent && !st.createdMxTest {
		fmt.Fprintf(w, "  %s module already existed; dropping only %s\n", mxTestModule, mxTestRunner)
	}

	return runMDLCommands(projectPath, cleanupCommands(st, mxTestPresent))
}

// reportCleanup prints a cleanup failure prominently. The project is left mutated,
// so this must not read as a passing run.
// left names the generated documents still in the project, so the reader does
// not have to find them by hand — which is the step mendixlabs/mxcli#1104 says
// cost them a debugging cycle. A surviving test microflow is not inert: it fails
// the build of every later run of any test file.
func reportCleanup(w io.Writer, err error, left []string) {
	if err == nil {
		return
	}
	fmt.Fprintf(w, "\nERROR: cleanup failed — the project has been left modified:\n%v\n", err)
	if len(left) > 0 {
		fmt.Fprintf(w, "\nStill in the project (generated by this run or an earlier one):\n")
		for _, name := range left {
			fmt.Fprintf(w, "  %s\n", name)
		}
		fmt.Fprintf(w, "\nRemove them with:\n")
		for _, name := range left {
			fmt.Fprintf(w, "  mxcli -p <project.mpr> -c \"DROP MICROFLOW %s\"\n", name)
		}
	}
	fmt.Fprintf(w, "Check the after-startup microflow and the %s module before committing.\n", mxTestModule)
}

// runDockerAndCapture builds the project, restarts the compose stack, and reads
// the test runner's output from the container log.
func runDockerAndCapture(opts RunOptions, timeout time.Duration, w io.Writer) (string, error) {
	dockerDir := filepath.Join(filepath.Dir(opts.ProjectPath), ".docker")
	if err := ensureDockerStack(opts.ProjectPath, dockerDir, w); err != nil {
		return "", fmt.Errorf("docker init: %w", err)
	}

	if !opts.SkipBuild {
		fmt.Fprintln(w, "Building project...")
		if err := execMxcli(opts.ProjectPath, "docker", "build", "-p", opts.ProjectPath, "--skip-check"); err != nil {
			return "", fmt.Errorf("docker build: %w", err)
		}
	}

	fmt.Fprintln(w, "Restarting runtime...")
	runCompose(dockerDir, "down")
	if err := runCompose(dockerDir, "up", "--detach", "--force-recreate"); err != nil {
		return "", fmt.Errorf("docker up: %w\n"+
			"  hint: this environment has no Docker daemon. Pass --local to run the tests "+
			"against mxcli's own runtime instead — no container needed.", err)
	}

	fmt.Fprintf(w, "Waiting for test execution (timeout: %s)...\n", timeout)
	logOutput, err := captureRuntimeLogs(dockerDir, timeout, w, opts.Verbose)
	if err != nil {
		return logOutput, fmt.Errorf("runtime execution: %w", err)
	}
	return logOutput, nil
}

// captureRuntimeLogs tails the docker compose logs, waiting for MXTEST:END or timeout.
// Returns the captured log output.
func captureRuntimeLogs(dockerDir string, timeout time.Duration, w io.Writer, verbose bool) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, docker.ContainerCLI(), "compose", "logs", "--follow", "--no-log-prefix", "--since", "1s", "mendix")
	cmd.Dir = dockerDir

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", fmt.Errorf("creating log pipe: %w", err)
	}
	cmd.Stderr = cmd.Stdout

	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("starting log follow: %w", err)
	}

	var logBuf bytes.Buffer
	scanner := bufio.NewScanner(stdout)
	done := false
	runtimeFailed := false
	var failMsg string

	for scanner.Scan() {
		line := scanner.Text()
		logBuf.WriteString(line)
		logBuf.WriteString("\n")

		if verbose {
			fmt.Fprintln(w, line)
		}

		// Check for test completion
		if strings.Contains(line, "MXTEST:END:") {
			done = true
			cancel()
			break
		}

		// Check for after-startup completion (tests ran)
		if strings.Contains(line, "Successfully ran after-startup-action") {
			done = true
			cancel()
			break
		}

		// Check for runtime failure
		if strings.Contains(line, "Error starting runtime") ||
			strings.Contains(line, "Critical error") ||
			strings.Contains(line, "After startup microflow should return a boolean") {
			runtimeFailed = true
			failMsg = line
			cancel()
			break
		}

		// Also catch after-startup failures
		if strings.Contains(line, "after-startup-action failed") {
			// The after-startup failed — tests may have partially run
			done = true
			cancel()
			break
		}
	}

	cmd.Process.Kill()
	cmd.Wait()

	if runtimeFailed {
		return logBuf.String(), fmt.Errorf("runtime failed: %s", failMsg)
	}

	if !done && ctx.Err() == context.DeadlineExceeded {
		return logBuf.String(), fmt.Errorf("timeout after %s waiting for test completion", timeout)
	}

	return logBuf.String(), nil
}

// execMxcli runs an mxcli subcommand.
func execMxcli(projectPath string, args ...string) error {
	mxcliPath, err := findMxcli()
	if err != nil {
		return err
	}

	cmd := exec.Command(mxcliPath, args...)
	cmd.Env = append(os.Environ(), "MXCLI_QUIET=1")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// execMxcliCmd runs an MDL command via mxcli -p ... -c "...".
func execMxcliCmd(projectPath, mdlCmd string) error {
	mxcliPath, err := findMxcli()
	if err != nil {
		return err
	}

	cmd := exec.Command(mxcliPath, "-p", projectPath, "-c", mdlCmd)
	cmd.Env = append(os.Environ(), "MXCLI_QUIET=1")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %s", err, string(output))
	}
	return nil
}

// findMxcli locates the mxcli binary.
func findMxcli() (string, error) {
	// First check if we're running as mxcli ourselves
	exe, err := os.Executable()
	if err == nil {
		return exe, nil
	}

	// Look in PATH
	path, err := exec.LookPath("mxcli")
	if err == nil {
		return path, nil
	}

	// Look in common locations
	for _, p := range []string{"./mxcli", "./bin/mxcli", "../bin/mxcli"} {
		if _, err := os.Stat(p); err == nil {
			abs, _ := filepath.Abs(p)
			return abs, nil
		}
	}

	return "", fmt.Errorf("mxcli binary not found (ensure it's in PATH or current directory)")
}

func ensureDockerStack(projectPath, dockerDir string, w io.Writer) error {
	composePath := filepath.Join(dockerDir, "docker-compose.yml")
	if _, err := os.Stat(composePath); err == nil {
		return nil
	}
	return docker.Init(docker.InitOptions{
		ProjectPath: projectPath,
		OutputDir:   dockerDir,
		Stdout:      w,
	})
}

// runCompose executes a docker compose command in the given directory.
func runCompose(dockerDir string, args ...string) error {
	cmd := exec.Command(docker.ContainerCLI(), append([]string{"compose"}, args...)...)
	cmd.Dir = dockerDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// emptySuiteError says WHY a run found nothing.
//
// "no tests found in the provided files" is true and useless. Discovery matches
// on the file NAME — a directory contributes only its .test.mdl and .test.md
// files — so a directory holding workflow.mdl is reported as empty with no hint
// that the name is the reason (ako/mxcli-maintenance §5). The same message also
// appears after "Found 0 test(s) in 1 file(s)", where the 1 is the directory
// itself, which reads as though a file had been read and found wanting.
//
// So name the skipped candidates. A .mdl file in a test directory is almost
// always the intended test under the wrong name, and printing it turns a dead end
// into a rename.
func emptySuiteError(paths []string) error {
	var skipped []string
	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil || !info.IsDir() {
			continue
		}
		entries, err := os.ReadDir(path)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() || isTestFile(e.Name()) {
				continue
			}
			if l := strings.ToLower(e.Name()); strings.HasSuffix(l, ".mdl") || strings.HasSuffix(l, ".md") {
				skipped = append(skipped, filepath.Join(path, e.Name()))
			}
		}
	}
	sort.Strings(skipped)

	if len(skipped) == 0 {
		return fmt.Errorf("no tests found: a test file is named *.test.mdl or *.test.md, " +
			"and holds one or more `/** @test ... */` blocks")
	}
	const maxNamed = 5
	named := skipped
	if len(named) > maxNamed {
		named = named[:maxNamed]
	}
	more := ""
	if len(skipped) > len(named) {
		more = fmt.Sprintf(" (and %d more)", len(skipped)-len(named))
	}
	return fmt.Errorf("no tests found: discovery matches on the file NAME, so these were not read%s:\n"+
		"  %s\n"+
		"Rename one to *.test.mdl (or *.test.md) to have it picked up",
		more, strings.Join(named, "\n  "))
}
