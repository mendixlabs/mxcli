// SPDX-License-Identifier: Apache-2.0

package testrunner

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/mendixlabs/mxcli/cmd/mxcli/docker"
)

// watchPollInterval is how often the loop checks for a change. Polling rather
// than inotify for the same reason `run --local --watch` polls: container
// filesystems do not reliably deliver inotify events for host-mounted paths.
const watchPollInterval = 1 * time.Second

// runEndpointWatch keeps the runtime and the build server up across runs,
// re-running the suite on every change to a test file or to the app's model.
//
// This is what the endpoint was for. A cold boot is ~30s and the one-shot runner
// pays it on every invocation; here it is paid once and each subsequent run is a
// warm rebuild plus a few HTTP calls.
//
// The loop has one hazard the dev loop does not: **the runner writes to the
// project it is watching**. Injecting the test microflows changes model source,
// which is the very signal being polled, so every baseline is taken *after* the
// injection and rebuild have settled. Getting that wrong is an infinite rebuild
// loop, not a subtle bug.
func runEndpointWatch(opts RunOptions, suite *TestSuite, token string, timeout time.Duration, w io.Writer, finish finishFunc, onInject func(*TestSuite)) (*SuiteResult, error) {
	sess, err := bootForTests(opts, token, timeout, w)
	if err != nil {
		return finish(nil, err)
	}
	// Belt and braces: the exits below all stop the app explicitly, before
	// cleanup rewrites the project out from under it. This catches a panic.
	defer sess.stop()

	// shutdown stops the app and then restores the project, in that order —
	// nothing should still be serving a model that is about to have the test
	// endpoint removed from it.
	shutdown := func(result *SuiteResult, runErr error) (*SuiteResult, error) {
		fmt.Fprintln(w, "Stopping the runtime...")
		sess.stop()
		return finish(result, runErr)
	}
	return watchLoop(opts, sess, suite, w, shutdown, onInject)
}

// runAttachedWatch is the same loop against an app someone else owns. Nothing is
// stopped on the way out — only the injected test microflows are removed.
func runAttachedWatch(opts RunOptions, app *attachedApp, suite *TestSuite, timeout time.Duration, w io.Writer, finish finishFunc, onInject func(*TestSuite)) (*SuiteResult, error) {
	return watchLoop(opts, app, suite, w, finish, onInject)
}

// watchLoop re-runs the suite on every change until interrupted.
func watchLoop(opts RunOptions, target testTarget, suite *TestSuite, w io.Writer, shutdown finishFunc, onInject func(*TestSuite)) (*SuiteResult, error) {
	// Ctrl-C has to reach the cleanup, not kill the process with the project
	// still carrying the test endpoint and an after-startup pointing at it.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	ticker := time.NewTicker(watchPollInterval)
	defer ticker.Stop()

	injected := suite
	var last *SuiteResult
	gen := 0

	for {
		gen++
		result, err := runSuite(target.endpoint(), target.adminOptions(), injected, opts, w)
		if err != nil {
			// The endpoint stopped answering — the runtime is probably gone, and
			// nothing further will work. Bail rather than spin.
			return shutdown(last, err)
		}
		// A file that does not parse is carried into the results rather than
		// ending the loop, so the tests that do parse keep running while it is
		// being fixed — and the run stays red until it is (#903).
		result.Tests = append(result.Tests, suiteFileErrorResults(injected.FileErrors)...)
		last = result
		PrintResults(w, result, opts.Color)
		if err := writeJUnit(opts, result, w); err != nil {
			fmt.Fprintf(w, "  %v\n", err)
		}

		// Baseline AFTER the run, so an edit made while tests were executing is
		// still caught on the next tick.
		baseline := watchMTime(opts)
		fmt.Fprintf(w, "\nWatching tests + model for changes (run #%d; Ctrl-C to stop)...\n", gen)

		changed := false
		for !changed {
			select {
			case <-sigCh:
				fmt.Fprintln(w, "\nShutting down...")
				return shutdown(last, nil)
			case <-ticker.C:
				if now := watchMTime(opts); now.After(baseline) {
					changed = true
				}
			}
		}

		fmt.Fprintln(w, "Change detected, rebuilding...")
		start := time.Now()

		// Re-parse: a test may have been added, edited, or deleted.
		reparsed, err := parseTestFiles(opts.TestFiles)
		if err != nil {
			fmt.Fprintf(w, "  test files do not parse: %v\n", err)
			continue
		}
		for _, fe := range reparsed.FileErrors {
			fmt.Fprintf(w, "  ERROR %s: %v\n", fe.Path, fe.Err)
		}

		// Report the new set BEFORE injecting: if the injection fails partway,
		// cleanup must still know about the microflows that did land. Dropping a
		// microflow that was never created is harmless; leaving one behind is not.
		onInject(reparsed)
		if err := reinjectTests(opts, injected, reparsed, w); err != nil {
			fmt.Fprintf(w, "  injecting tests: %v\n", err)
			injected = reparsed
			continue
		}
		injected = reparsed

		action, err := target.applyModelChange(opts.ProjectPath)
		if err != nil {
			// Not fatal: a build error is usually the edit that just happened, and
			// the next save is likely to fix it. Report and keep watching.
			fmt.Fprintf(w, "  %v\n", err)
			continue
		}
		fmt.Fprintf(w, "  rebuilt and applied via %s in %s\n", action, time.Since(start).Round(time.Millisecond))
	}
}

// finishFunc restores the project and decides the final result. runEndpointWatch
// takes it rather than owning cleanup so that every exit — a clean Ctrl-C, a
// dead runtime, a boot failure — goes through the same restore as the one-shot
// path.
type finishFunc func(result *SuiteResult, runErr error) (*SuiteResult, error)

// watchMTime is the change signal: the newer of the test files' and the model's
// modification times.
//
// Both matter, and for different reasons. A test file changing means the
// assertions changed. The model changing means the code under test changed —
// which is the case a developer actually cares about, editing a microflow and
// wanting to know immediately whether it still passes.
func watchMTime(opts RunOptions) time.Time {
	newest := docker.ProjectSourceMTime(opts.ProjectPath)
	if t := testFilesMTime(opts.TestFiles); t.After(newest) {
		newest = t
	}
	return newest
}

// testFilesMTime is the newest modification time across the test paths, which
// may be individual files or directories.
//
// A directory is walked rather than stat'ed: on Linux a directory's own mtime
// changes when an entry is created or removed but *not* when an existing file is
// edited in place, which is the common case this loop exists to catch. It is
// walked by walkTestTree, the walk the parser uses, so the two cannot disagree
// about which files are tests (hidden and "_" folders below the root, and the
// root itself, are treated alike).
func testFilesMTime(paths []string) time.Time {
	var newest time.Time
	for _, p := range paths {
		info, err := os.Stat(p)
		if err != nil {
			continue
		}
		if !info.IsDir() {
			if info.ModTime().After(newest) {
				newest = info.ModTime()
			}
			continue
		}
		// An unreadable entry is not a change: no onErr, and the root's own error
		// leaves newest as it was.
		_, _ = walkTestTree(p, func(path string, d fs.DirEntry) {
			if !isTestFile(d.Name()) {
				return
			}
			if fi, err := d.Info(); err == nil && fi.ModTime().After(newest) {
				newest = fi.ModTime()
			}
		}, nil)
		// Catch a deletion too: the directory's own mtime moves when an entry
		// goes away, and the walk above cannot see a file that is no longer there.
		if info.ModTime().After(newest) {
			newest = info.ModTime()
		}
	}
	return newest
}

// reinjectTests updates the project's generated test microflows to match a
// re-parsed suite.
//
// CREATE OR REPLACE covers a test that was added or edited, but says nothing
// about one that was deleted: its microflow would linger and keep being invoked,
// reporting a stale pass for a test that no longer exists. So the flows for tests
// that are gone are dropped explicitly.
func reinjectTests(opts RunOptions, old, new *TestSuite, w io.Writer) error {
	if drops := staleTestFlows(old, new); len(drops) > 0 {
		fmt.Fprintf(w, "  dropping %d removed test microflow(s)\n", len(drops))
		if err := runMDLCommands(opts.ProjectPath, drops); err != nil {
			return err
		}
	}
	return execMDLScript(opts.ProjectPath, GenerateTestFlows(new), "mxtest-flows-*.mdl")
}

// staleTestFlows returns DROP statements for test microflows in old that no
// longer have a counterpart in new.
func staleTestFlows(old, new *TestSuite) []string {
	if old == nil {
		return nil
	}
	keep := make(map[string]bool, len(new.Tests))
	for _, tc := range new.Tests {
		keep[testFlowName(tc)] = true
	}
	var drops []string
	for _, tc := range old.Tests {
		if flow := testFlowName(tc); !keep[flow] {
			drops = append(drops, "DROP MICROFLOW "+flow)
		}
	}
	return drops
}
