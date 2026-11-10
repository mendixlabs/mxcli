// SPDX-License-Identifier: Apache-2.0

package testrunner

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/mendixlabs/mxcli/cmd/mxcli/docker"
)

// testAppSession is a booted app plus a client for its test endpoint. It is what
// the warm loop keeps alive between runs.
type testAppSession struct {
	app     *docker.LocalApp
	client  *endpointClient
	logPath string
}

// bootForTests boots the app and waits for the test endpoint to register.
func bootForTests(opts RunOptions, token string, timeout time.Duration, w io.Writer) (*testAppSession, error) {
	logPath := filepath.Join(filepath.Dir(opts.ProjectPath), ".mxcli", "test-runtime.log")

	fmt.Fprintln(w, "Starting local runtime (no Docker)...")
	// The token reaches the runtime through its environment and is never written
	// to the project. See endpointTokenEnv.
	app, err := docker.StartLocalApp(
		localAppOptions(opts, logPath, []string{endpointTokenEnv + "=" + token}, w))
	if err != nil {
		// Unlike the after-startup path, a boot failure here is never a test
		// result — no test has run yet. It is always a real error.
		return nil, fmt.Errorf("local runtime: %w", err)
	}

	client := testEndpointClient(opts, token)
	if err := client.waitReady(endpointReadyTimeout(timeout)); err != nil {
		app.Stop()
		return nil, fmt.Errorf("%w\n  hint: check %s for a registration failure", err, logPath)
	}
	return &testAppSession{app: app, client: client, logPath: logPath}, nil
}

// testEndpointClient is the client for the test endpoint of the app a --local
// run boots, on the app port the options select.
func testEndpointClient(opts RunOptions, token string) *endpointClient {
	return newEndpointClient(opts.appPort(), token)
}

func (s *testAppSession) stop() {
	if s != nil && s.app != nil {
		s.app.Stop()
	}
}

// testTarget is an app the runner can run tests against and push model changes
// into. Two things satisfy it: a runtime the runner booted itself, and one
// already running that it attached to. The watch loop is written against this
// so it does not care which.
type testTarget interface {
	// endpoint is the client for the app's test endpoint.
	endpoint() *endpointClient
	// adminOptions reaches the app's M2EE admin API, which is where @verify's
	// OQL runs. It is a different plane and a different secret from the test
	// endpoint: the endpoint token invokes microflows, the admin password
	// queries the database.
	adminOptions() docker.M2EEOptions
	// applyModelChange rebuilds the project and applies it, returning a label for
	// what it took ("reload"/"restart"). It returns only once the endpoint is
	// reachable again, so the caller can invoke a test straight after.
	applyModelChange(projectPath string) (string, error)
}

func (s *testAppSession) endpoint() *endpointClient { return s.client }

func (s *testAppSession) adminOptions() docker.M2EEOptions {
	return s.app.Runtime.AdminOptions()
}

// applyModelChange rebuilds through the serve server this session owns.
//
// A restart is fine here — the session owns the runtime — but it re-runs
// after-startup, so the endpoint has to be waited for before the next test.
func (s *testAppSession) applyModelChange(projectPath string) (string, error) {
	action, _, err := s.app.Rebuild(projectPath)
	if err != nil {
		return action.String(), err
	}
	if action == docker.ActionRestart {
		if err := s.client.waitReady(endpointReadyTimeout(0)); err != nil {
			return action.String(), fmt.Errorf("the test endpoint did not come back after a restart: %w", err)
		}
	}
	return action.String(), nil
}

// runViaEndpoint boots the app once and drives the suite over HTTP.
//
// The contrast with the after-startup path is the whole point: there, tests run
// during boot, so every re-run is a restart and a result is something recovered
// from the log. Here boot only registers the endpoint, and each test is a
// request against a runtime that stays up.
func runViaEndpoint(opts RunOptions, suite *TestSuite, token string, timeout time.Duration, w io.Writer) (*SuiteResult, error) {
	sess, err := bootForTests(opts, token, timeout, w)
	if err != nil {
		// A build MxBuild rejected because of a generated test microflow is that
		// test's problem, not the run's. Reporting it as an ERROR row — and the
		// rest as SKIP — says which assertion broke, where the bare failure said
		// only that the project would not deploy (FINDINGS #46 follow-up).
		return resultsForBuildFailure(err, suite)
	}
	defer sess.stop()
	return runSuite(sess.client, sess.adminOptions(), suite, opts, w)
}

// runSuite invokes every test in the suite against a booted app and collects the
// verdicts. It never returns an error for a test-level problem — a missing
// microflow or a failed request is that test's result, so one bad test cannot
// hide every other.
func runSuite(client *endpointClient, admin docker.M2EEOptions, suite *TestSuite, opts RunOptions, w io.Writer) (*SuiteResult, error) {
	// Ask the app which test microflows it actually has. A test whose microflow
	// is missing is reported as an error against that test rather than failing
	// the run.
	// Tests without a compiled program run as generated microflows, so the app
	// is asked which of those it has whenever there is at least one.
	direct := 0
	for _, tc := range suite.Tests {
		if opts.execPrograms[tc.ID] != nil {
			direct++
		}
	}
	present := make(map[string]bool)
	if direct < len(suite.Tests) {
		names, err := client.list()
		if err != nil {
			return nil, fmt.Errorf("listing test microflows: %w", err)
		}
		for _, n := range names {
			present[n] = true
		}
	}

	result := &SuiteResult{Name: suite.Name, Started: time.Now()}
	switch {
	case direct == 0:
		fmt.Fprintf(w, "Running %d test(s) over the test endpoint...\n", len(suite.Tests))
	case direct == len(suite.Tests):
		fmt.Fprintf(w, "Running %d test(s) in the running app (no injection, no build)...\n", len(suite.Tests))
	default:
		fmt.Fprintf(w, "Running %d test(s): %d in the running app, %d as built test microflows...\n",
			len(suite.Tests), direct, len(suite.Tests)-direct)
	}

	// leaked counts tests whose requested rollback did not happen.
	leaked := 0

	for _, tc := range suite.Tests {
		if res, bad := preRunResult(tc, opts.RequireAssertions); bad {
			result.Tests = append(result.Tests, res)
			continue
		}

		rollback := rollsBack(tc)
		var rr *runResponse
		var err error
		if prog := opts.execPrograms[tc.ID]; prog != nil {
			rr, err = client.exec(prog)
		} else {
			flow := testFlowName(tc)
			if !present[flow] {
				res := newResult(tc)
				res.Status = StatusError
				res.Message = fmt.Sprintf("microflow %s was not created — the test body may not have compiled", flow)
				result.Tests = append(result.Tests, res)
				continue
			}
			rr, err = client.run(flow, rollback)
		}
		if err != nil {
			// A transport failure is not a verdict. Report it against this test
			// and keep going; if the runtime died the rest will say so too.
			res := newResult(tc)
			res.Status = StatusError
			res.Message = fmt.Sprintf("calling the test endpoint: %v", err)
			result.Tests = append(result.Tests, res)
			continue
		}

		// A rollback that was asked for and did not happen leaves the test's data
		// in the database while the verdict still says PASS. The test itself is
		// not wrong, so its verdict stands — but this must not pass in silence.
		if rollback && !rr.RolledBack {
			leaked++
			reportRollbackFailure(w, tc, rr)
		}

		res := toResult(tc, rr)
		// @verify runs only once the microflow has been and gone: it asserts on
		// what the app wrote, not on what it returned. A test that already
		// failed keeps its verdict — the first failure is the informative one.
		if res.Status == StatusPass {
			runVerifies(&res, tc, admin, opts.ProjectPath)
		}
		result.Tests = append(result.Tests, res)
		if opts.Verbose {
			fmt.Fprintf(w, "  %s %s (%s)%s\n", res.Status, res.Name,
				res.Duration.Round(time.Millisecond), rollbackNote(rollback, rr))
		}
	}

	if leaked > 0 {
		fmt.Fprintf(w, "\nWARNING: %d test(s) asked for @cleanup rollback and did not get it — "+
			"their writes are still in the database.\n", leaked)
	}

	result.Duration = time.Since(result.Started)
	return result, nil
}

// endpointReadyTimeout bounds the wait for the endpoint to register. It is
// capped well below the suite timeout: the handler is registered during the
// start action, so if it is not up shortly after the runtime reports started, it
// is not coming.
func endpointReadyTimeout(suiteTimeout time.Duration) time.Duration {
	const cap = 60 * time.Second
	if suiteTimeout > 0 && suiteTimeout < cap {
		return suiteTimeout
	}
	return cap
}

// endpointCleanupCommands returns the MDL that removes what the endpoint path
// injected, in order.
//
// It mirrors cleanupCommands but has more to take out: the registration Java
// action and startup microflow, plus one microflow per test. When Run created
// the MxTest module, dropping the module removes all of it in one statement;
// when the module was already the user's, each generated document is named
// explicitly so nothing of theirs is touched.
func endpointCleanupCommands(st projectState, flows []string, mxTestPresent bool) []string {
	restore := "ALTER SETTINGS RUNTIME (AfterStartupMicroflow: '')"
	if st.afterStartup != "" {
		restore = "ALTER SETTINGS RUNTIME (AfterStartupMicroflow: " + quoteMDLString(st.afterStartup) + ")"
	}
	cmds := []string{restore}
	if !mxTestPresent {
		return cmds
	}
	if st.createdMxTest {
		return append(cmds, "DROP MODULE "+mxTestModule)
	}
	// Every generated flow the project holds, not just this suite's — see
	// cleanup_leftovers.go.
	for _, name := range flows {
		cmds = append(cmds, "DROP MICROFLOW "+name)
	}
	return append(cmds,
		"DROP MICROFLOW "+endpointStartupFlow,
		"DROP JAVA ACTION "+endpointRegisterAction,
	)
}

// cleanupEndpoint restores the project after an endpoint run.
//
// As with cleanup, every statement is attempted even after one fails and the
// failures are returned rather than warned about: a half-restored project still
// carries a test endpoint and an after-startup pointing at it.
func cleanupEndpoint(projectPath string, st projectState, suite *TestSuite, w io.Writer) error {
	mxTestPresent := true
	if exists, err := moduleExists(projectPath, mxTestModule); err == nil {
		mxTestPresent = exists
	}
	if mxTestPresent && !st.createdMxTest {
		fmt.Fprintf(w, "  %s module already existed; dropping only the generated documents\n", mxTestModule)
	}
	return runMDLCommands(projectPath, endpointCleanupCommands(st, testFlowsToDrop(projectPath, suite), mxTestPresent))
}

// removeGeneratedJavaSource deletes the .java file the Java action generated.
//
// DROP JAVA ACTION removes the model document; the source file it wrote into
// javasource/ is not the model's to delete, so it is left behind. For a
// generated per-run artifact that is litter in the user's tree — and litter that
// still contains a request handler, which is exactly what should not be left
// lying around. Failure is not fatal: the file is inert without the model
// document, and a cleanup error here would mask the real ones.
func removeGeneratedJavaSource(projectPath string, w io.Writer) {
	dir := filepath.Join(filepath.Dir(projectPath), "javasource", lowerModule(mxTestModule))
	if _, err := os.Stat(dir); err != nil {
		return
	}
	if err := os.RemoveAll(dir); err != nil {
		fmt.Fprintf(w, "  note: could not remove generated Java source at %s: %v\n", dir, err)
	}
}
