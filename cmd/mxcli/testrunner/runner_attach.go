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

// attachedApp is a running app someone else owns, reached through its handshake.
//
// It satisfies the same two needs bootForTests does — a client for the endpoint,
// and a way to apply a model change — but owns neither the runtime nor the build
// server, so it must never stop them.
type attachedApp struct {
	client *endpointClient
	// builds go through the dev loop's serve server, falling back to a serve
	// process of the attach's own when that one would leave the client bundle
	// without its pages (ako/mxcli#1917). close stops only the attach's own.
	builds *docker.AttachedServe
	ctrl   *docker.RuntimeController
	hs     *Handshake
}

// close stops what the attach started; the dev loop's processes are not its to stop.
func (a *attachedApp) close() { a.builds.Close() }

// attach connects to an app already hosting the test endpoint.
func attach(opts RunOptions, w io.Writer) (*attachedApp, error) {
	hs, err := ReadHandshake(opts.ProjectPath)
	if err != nil {
		return nil, err
	}

	client := newEndpointClient(hs.AppPort, hs.Token)
	if err := client.ping(); err != nil {
		return nil, fmt.Errorf("the app on port %d is not answering the test endpoint: %w\n"+
			"  The handshake looks live, so the app may still be starting. Retry in a moment.", hs.AppPort, err)
	}

	fmt.Fprintf(w, "Attached to the app on port %d (pid %d) — no boot needed.\n", hs.AppPort, hs.PID)
	// The dev app's database is the developer's, and these tests are about to
	// write to it. That is the whole trade --attach makes, so say it every time
	// rather than burying it in the docs.
	fmt.Fprintln(w, "  NOTE: tests run against the running app's database, not a scratch one.")

	return &attachedApp{
		client: client,
		builds: docker.NewAttachedServe(&docker.ServeServer{Host: "127.0.0.1", Port: hs.ServePort}, opts.ProjectPath, w),
		// The admin password, not the endpoint token — different secrets.
		ctrl: docker.NewRuntimeController(docker.M2EEOptions{
			Host:  "127.0.0.1",
			Port:  hs.AdminPort,
			Token: hs.AdminPass,
		}),
		hs: hs,
	}, nil
}

// applyModelChange rebuilds the project through the dev loop's serve server and
// applies the result through its admin API.
//
// Driving the other process's services rather than requiring it to be in
// --watch: both are plain loopback APIs, so an attach can apply its own
// injections deterministically instead of waiting to see whether someone else's
// watcher noticed. A dev loop that *is* watching may also rebuild — harmless,
// since both produce the same deployment from the same source.
func (a *attachedApp) endpoint() *endpointClient { return a.client }

// adminOptions reaches the attached app's admin API — the same plane
// applyModelChange drives, and where @verify's OQL runs.
func (a *attachedApp) adminOptions() docker.M2EEOptions {
	return docker.M2EEOptions{
		Host:   "127.0.0.1",
		Port:   a.hs.AdminPort,
		Token:  a.hs.AdminPass,
		Direct: true,
	}
}

func (a *attachedApp) applyModelChange(projectPath string) (string, error) {
	build, err := a.builds.Build(docker.BuildRequest{Target: docker.TargetDeploy, ProjectFilePath: projectPath})
	if err != nil {
		return "", fmt.Errorf("rebuilding through the attached app's build server on port %d: %w", a.hs.ServePort, err)
	}
	if !build.OK() {
		// Carries the parsed problems; the caller turns them into per-test rows or
		// names the document they were found in. Returning only build.Message here
		// is what made an --attach run say nothing but "the project cannot be
		// deployed" (mendixlabs/mxcli#1104).
		return "", buildFailure(build)
	}
	// No restart callback: the runtime belongs to the other process. A structural
	// change is refused rather than half-applied — see attachRestartRefusal.
	action, err := a.ctrl.ApplyBuild(build, nil)
	if err != nil {
		return action.String(), err
	}
	if action == docker.ActionRestart {
		return action.String(), attachRestartRefusal(build)
	}
	return action.String(), nil
}

// attachRestartRefusal is why an attach cannot apply a build that asks for a
// runtime restart. The runtime belongs to the other process, so a structural
// change is refused rather than half-applied.
func attachRestartRefusal(build *docker.BuildResult) error {
	if build.ColdRestart {
		// Nothing says an entity changed: the serve process could not build
		// incrementally (Mendix 11.14.0 fails every later build), and a first
		// build asks for a restart whatever changed. Restarting the dev loop does
		// not help either — the attach's build is again a later one in its serve.
		return fmt.Errorf("mxbuild --serve could not build this change incrementally, and the full build it " +
			"fell back to asks for a runtime restart whatever changed (it has no earlier build to compare with), " +
			"which --attach cannot do — the runtime belongs to the 'mxcli run --local' process.\n" +
			"  Drop --attach to run against a scratch runtime.")
	}
	return fmt.Errorf("this change needs a runtime restart (an entity or association changed), " +
		"which --attach cannot do — the runtime belongs to the 'mxcli run --local' process.\n" +
		"  Restart that process, or drop --attach to run against a scratch runtime.")
}

// runAttached injects the test microflows into the already-running app, runs the
// suite, and removes them again.
//
// It deliberately does not touch the endpoint or the after-startup setting: the
// dev loop installed those and will remove them when it exits. An attach only
// owns the test microflows it adds.
func runAttached(opts RunOptions, suite *TestSuite, timeout time.Duration, w io.Writer) (*SuiteResult, error) {
	app, err := attach(opts, w)
	if err != nil {
		return nil, err
	}
	defer app.close()

	// Only the generated test microflows are ours to remove.
	injected := suite
	// snapshot is the project file before the injection, put back after it so
	// the dev loop sees the model it had (see holdDevLoop).
	var snapshot projectSnapshot
	finish := func(result *SuiteResult, runErr error) (*SuiteResult, error) {
		fmt.Fprintln(w, "Cleaning up...")
		cleanupErr := runMDLCommands(opts.ProjectPath, dropTestFlows(opts.ProjectPath, injected))
		if err := snapshot.restore(cleanupErr == nil); err != nil {
			fmt.Fprintf(w, "  note: %v\n", err)
		}
		if cleanupErr == nil {
			// Leave the app serving a model that matches the project on disk;
			// otherwise the developer's next page load still runs the test flows.
			if _, err := app.applyModelChange(opts.ProjectPath); err != nil {
				fmt.Fprintf(w, "  note: the app is still serving the test microflows until its next rebuild: %v\n", err)
			}
			fmt.Fprintln(w, "  test microflows removed")
		} else {
			reportCleanup(w, cleanupErr, survivingTestFlows(opts.ProjectPath))
		}
		if runErr != nil {
			return nil, runErr
		}
		if cleanupErr != nil {
			return result, fmt.Errorf("cleanup failed, project left modified: %w", cleanupErr)
		}
		return result, nil
	}

	// Printing is shared because a build MxBuild rejected now produces results
	// too: the failing test gets an ERROR row instead of the run getting a bare
	// "the project cannot be deployed".
	report := func(result *SuiteResult, err error) (*SuiteResult, error) {
		if result != nil {
			PrintResults(w, result, opts.Color)
			if jerr := writeJUnit(opts, result, w); jerr != nil && err == nil {
				err = jerr
			}
		}
		return result, err
	}

	// An app started with --allow-call runs a compiled test itself (interp.go):
	// those tests are neither injected nor built. Only the tests it cannot run
	// are, so one unsupported statement no longer costs the whole suite a build.
	toInject := suite
	if app.hs.AllowExec && !opts.Watch {
		programs, needBuild := compileSuiteForExec(suite)
		if len(programs) > 0 {
			if err := refuseStaleModel(opts.ProjectPath); err != nil {
				return nil, err
			}
			opts.execPrograms = programs
		}
		if len(needBuild) == 0 {
			result, err := runSuite(app.client, app.adminOptions(), suite, opts, w)
			if err != nil {
				return nil, err
			}
			return report(result, nil)
		}
		for _, tc := range suite.Tests {
			if reason, ok := needBuild[tc.ID]; ok {
				fmt.Fprintf(w, "  injecting and building %q: %s\n", tc.Name, reason)
			}
		}
		toInject = suiteWith(suite, func(tc TestCase) bool { return programs[tc.ID] == nil })
		injected = toInject
	}

	// The dev loop watches the project this run is about to write to. Held, it
	// does not rebuild (and restart the app) for the injection, and on release
	// it finds the model as it was. Released when this function returns, after
	// finish has taken the injection out and put the project file back.
	snapshot = takeProjectSnapshot(opts.ProjectPath)
	if err := docker.HoldWatch(opts.ProjectPath, os.Getpid()); err != nil {
		fmt.Fprintf(w, "  note: could not hold the dev loop's watcher (%v); it may rebuild for the test microflows\n", err)
	} else {
		defer docker.ReleaseWatch(opts.ProjectPath)
		fmt.Fprintln(w, "  the dev loop's watcher is held for this run; the app is not rebuilt for the test microflows")
	}

	fmt.Fprintln(w, "Injecting test microflows...")
	if err := execMDLScript(opts.ProjectPath, GenerateTestFlows(toInject), "mxtest-flows-*.mdl"); err != nil {
		return finish(nil, fmt.Errorf("injecting test microflows: %w", err))
	}
	if _, err := app.applyModelChange(opts.ProjectPath); err != nil {
		// Same treatment the --local boot gives a rejected build: attribute each
		// error to the generated microflow it was found in, and name the document
		// when it belongs to the project instead (mendixlabs/mxcli#1104).
		return report(finish(resultsForBuildFailure(err, toInject)))
	}

	if opts.Watch {
		return runAttachedWatch(opts, app, suite, timeout, w, finish, func(s *TestSuite) { injected = s })
	}

	result, err := runSuite(app.client, app.adminOptions(), suite, opts, w)
	if err != nil {
		return finish(nil, err)
	}
	return report(finish(result, nil))
}

// dropTestFlows returns the DROP statements for a suite's generated microflows.
func dropTestFlows(projectPath string, suite *TestSuite) []string {
	// Keyed on what the project holds, not on this suite: an attach always runs
	// against a project whose MxTest module pre-exists, so nothing else ever
	// removes a flow an earlier run left behind (mendixlabs/mxcli#1104).
	flows := testFlowsToDrop(projectPath, suite)
	cmds := make([]string, 0, len(flows))
	for _, name := range flows {
		cmds = append(cmds, "DROP MICROFLOW "+name)
	}
	return cmds
}

// refuseStaleModel is staleModelError for the project's own build: the model on
// disk against deployment/model/model.mdp, which every build rewrites.
func refuseStaleModel(projectPath string) error {
	built, err := os.Stat(filepath.Join(filepath.Dir(projectPath), "deployment", "model", "model.mdp"))
	if err != nil {
		return fmt.Errorf("cannot tell which model the running app was built from (%v); restart the dev loop", err)
	}
	watching := false
	if st, err := docker.ReadRunState(projectPath); err == nil && st != nil {
		watching = st.Watch
	}
	return staleModelError(projectPath, docker.ProjectSourceMTime(projectPath), built.ModTime(), watching)
}
