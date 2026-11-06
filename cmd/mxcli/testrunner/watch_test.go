// SPDX-License-Identifier: Apache-2.0

package testrunner

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/mendixlabs/mxcli/cmd/mxcli/docker"
)

func TestValidateOptions(t *testing.T) {
	tests := []struct {
		name    string
		opts    RunOptions
		wantErr string
	}{
		{name: "no watch is always fine", opts: RunOptions{}},
		{name: "watch with local", opts: RunOptions{Watch: true, Local: true}},
		{
			name:    "watch without local",
			opts:    RunOptions{Watch: true},
			wantErr: "--watch requires --local",
		},
		{
			name:    "watch with the legacy runner",
			opts:    RunOptions{Watch: true, Local: true, LegacyRunner: true},
			wantErr: "--watch cannot be combined with --legacy-runner",
		},
		{
			name:    "watch with skip-build",
			opts:    RunOptions{Watch: true, Local: true, SkipBuild: true},
			wantErr: "--watch cannot be combined with --skip-build",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateOptions(tt.opts)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected an error containing %q", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error = %q, want it to contain %q", err, tt.wantErr)
			}
		})
	}
}

// writeTestFile writes a test file with a controlled mtime.
func writeTestFile(t *testing.T, path string, mtime time.Time) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte("/** @test x */\n"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := os.Chtimes(path, mtime, mtime); err != nil {
		t.Fatalf("chtimes: %v", err)
	}
}

// TestTestFilesMTimeSeesAnEditInsideADirectory pins the reason the directory is
// walked rather than stat'ed: on Linux a directory's own mtime does not move
// when an existing entry is edited in place, which is the common case.
func TestTestFilesMTimeSeesAnEditInsideADirectory(t *testing.T) {
	dir := t.TempDir()
	old := time.Now().Add(-2 * time.Hour)
	f := filepath.Join(dir, "a.test.mdl")
	writeTestFile(t, f, old)
	// Pin the directory itself to the old time, so only the file can move.
	if err := os.Chtimes(dir, old, old); err != nil {
		t.Fatalf("chtimes dir: %v", err)
	}

	before := testFilesMTime([]string{dir})

	edited := time.Now()
	writeTestFile(t, f, edited)
	if err := os.Chtimes(dir, old, old); err != nil {
		t.Fatalf("chtimes dir: %v", err)
	}

	after := testFilesMTime([]string{dir})
	if !after.After(before) {
		t.Errorf("editing a file in a watched directory produced no change signal (before=%v after=%v)", before, after)
	}
}

func TestTestFilesMTimeAcceptsAFilePath(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "a.test.mdl")
	want := time.Now().Add(-time.Hour).Truncate(time.Second)
	writeTestFile(t, f, want)

	if got := testFilesMTime([]string{f}); !got.Truncate(time.Second).Equal(want) {
		t.Errorf("mtime = %v, want %v", got, want)
	}
}

// TestTestFilesMTimeIgnoresNonTestFiles keeps an unrelated file in the tests
// directory — a README, an editor swap file — from re-triggering the loop.
func TestTestFilesMTimeIgnoresNonTestFiles(t *testing.T) {
	dir := t.TempDir()
	old := time.Now().Add(-2 * time.Hour)
	writeTestFile(t, filepath.Join(dir, "a.test.mdl"), old)
	if err := os.Chtimes(dir, old, old); err != nil {
		t.Fatalf("chtimes dir: %v", err)
	}
	before := testFilesMTime([]string{dir})

	noise := filepath.Join(dir, "notes.md")
	if err := os.WriteFile(noise, []byte("scratch"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	now := time.Now()
	if err := os.Chtimes(noise, now, now); err != nil {
		t.Fatalf("chtimes: %v", err)
	}
	// Hold the directory back so only the noise file could move the signal.
	if err := os.Chtimes(dir, old, old); err != nil {
		t.Fatalf("chtimes dir: %v", err)
	}

	if after := testFilesMTime([]string{dir}); after.After(before) {
		t.Error("a non-test file in the tests directory moved the change signal")
	}
}

// TestTestFilesMTimeSeesADeletion pins that removing a test is a change. The
// walk cannot see a file that is gone, so the directory's own mtime — which does
// move on a deletion — has to be folded in.
func TestTestFilesMTimeSeesADeletion(t *testing.T) {
	dir := t.TempDir()
	old := time.Now().Add(-2 * time.Hour)
	keep := filepath.Join(dir, "a.test.mdl")
	gone := filepath.Join(dir, "b.test.mdl")
	writeTestFile(t, keep, old)
	writeTestFile(t, gone, old)
	if err := os.Chtimes(dir, old, old); err != nil {
		t.Fatalf("chtimes dir: %v", err)
	}
	before := testFilesMTime([]string{dir})

	if err := os.Remove(gone); err != nil {
		t.Fatalf("remove: %v", err)
	}

	if after := testFilesMTime([]string{dir}); !after.After(before) {
		t.Errorf("deleting a test file produced no change signal (before=%v after=%v)", before, after)
	}
}

// TestStaleTestFlowsDropsRemovedTests pins the correctness hazard of re-running:
// CREATE OR REPLACE updates a test that changed but says nothing about one that
// was deleted, whose microflow would otherwise linger and keep reporting a pass.
func TestStaleTestFlowsDropsRemovedTests(t *testing.T) {
	old := &TestSuite{Tests: []TestCase{{ID: "test_1"}, {ID: "test_2"}, {ID: "test_3"}}}
	new := &TestSuite{Tests: []TestCase{{ID: "test_1"}, {ID: "test_2"}}}

	got := staleTestFlows(old, new)
	want := []string{"DROP MICROFLOW MxTest.Test_test_3"}
	if len(got) != len(want) || (len(got) > 0 && got[0] != want[0]) {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestStaleTestFlowsNoneWhenSuiteGrew(t *testing.T) {
	old := &TestSuite{Tests: []TestCase{{ID: "test_1"}}}
	new := &TestSuite{Tests: []TestCase{{ID: "test_1"}, {ID: "test_2"}}}

	if got := staleTestFlows(old, new); len(got) != 0 {
		t.Errorf("got %q, want no drops when tests were only added", got)
	}
}

// TestStaleTestFlowsDropsAllWhenEmptied covers deleting the last test in a file:
// every previously-injected flow has to come out.
func TestStaleTestFlowsDropsAllWhenEmptied(t *testing.T) {
	old := &TestSuite{Tests: []TestCase{{ID: "test_1"}, {ID: "test_2"}}}
	new := &TestSuite{}

	if got := staleTestFlows(old, new); len(got) != 2 {
		t.Errorf("got %d drops %q, want 2", len(got), got)
	}
}

func TestStaleTestFlowsHandlesNoPriorSuite(t *testing.T) {
	if got := staleTestFlows(nil, &TestSuite{Tests: []TestCase{{ID: "test_1"}}}); got != nil {
		t.Errorf("got %q, want nil for a first injection", got)
	}
}

// fakeWatchTarget is a testTarget whose rebuild outcomes are scripted.
type fakeWatchTarget struct {
	mu      sync.Mutex
	applyFn func(call int) error
	applies int
}

func (f *fakeWatchTarget) endpoint() *endpointClient        { return nil }
func (f *fakeWatchTarget) adminOptions() docker.M2EEOptions { return docker.M2EEOptions{} }
func (f *fakeWatchTarget) applyModelChange(string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.applies++
	return "reload", f.applyFn(f.applies)
}

type syncBuffer struct {
	mu sync.Mutex
	b  strings.Builder
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

// TestChangeFailedResultIsRed pins that a change that could not be applied is
// reported as an error row carrying the reason, not as an empty (green) result.
func TestChangeFailedResultIsRed(t *testing.T) {
	res := changeFailedResult(&TestSuite{Name: "mxtest"}, "rebuilding the app", errors.New("CE0117 broken"))
	if res.ErrorCount() != 1 || len(res.Tests) != 1 {
		t.Fatalf("want exactly one error row, got %+v", res.Tests)
	}
	row := res.Tests[0]
	if row.Status != StatusError || !strings.Contains(row.Message, "CE0117") ||
		!strings.Contains(row.Name, "rebuilding the app") {
		t.Errorf("row does not say what failed: %+v", row)
	}
	var out strings.Builder
	PrintResults(&out, res, false)
	if !strings.Contains(out.String(), "Errors: 1") {
		t.Errorf("the summary does not count it:\n%s", out.String())
	}
}

// TestWatchLoopDoesNotRerunAgainstTheOldModelAfterAFailedRebuild drives the real
// loop with a scripted target. A rebuild that fails must show red and wait; it
// used to fall through to the top of the loop and run the suite against the
// model from before the edit, so a broken microflow could print a green run.
func TestWatchLoopDoesNotRerunAgainstTheOldModelAfterAFailedRebuild(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("stops the loop with SIGINT")
	}
	dir := t.TempDir()
	tf := filepath.Join(dir, "a.test.mdl")
	writeTestFile(t, tf, time.Now().Add(-time.Hour))

	var suiteRuns atomic.Int32
	oldRun, oldReinject := runSuiteFn, reinjectTestsFn
	t.Cleanup(func() { runSuiteFn, reinjectTestsFn = oldRun, oldReinject })
	runSuiteFn = func(_ *endpointClient, _ docker.M2EEOptions, s *TestSuite, _ RunOptions, _ io.Writer) (*SuiteResult, error) {
		suiteRuns.Add(1)
		return &SuiteResult{Name: s.Name, Tests: []TestResult{{ID: "1", Name: "x", Status: StatusPass}}}, nil
	}
	reinjectTestsFn = func(RunOptions, *TestSuite, *TestSuite, io.Writer) error { return nil }

	target := &fakeWatchTarget{applyFn: func(call int) error {
		if call == 1 {
			return errors.New("build failed: CE0117 broken")
		}
		return nil
	}}
	opts := RunOptions{ProjectPath: filepath.Join(dir, "App.mpr"), TestFiles: []string{tf}}
	out := &syncBuffer{}
	var final *SuiteResult
	done := make(chan struct{})
	go func() {
		defer close(done)
		final, _ = watchLoop(opts, target, &TestSuite{Name: "mxtest"}, out, func(r *SuiteResult, err error) (*SuiteResult, error) {
			return r, err
		}, func(*TestSuite) {})
	}()

	waitFor := func(what string, cond func() bool) {
		t.Helper()
		deadline := time.Now().Add(15 * time.Second)
		for !cond() {
			if time.Now().After(deadline) {
				t.Fatalf("timed out waiting for %s; output:\n%s", what, out.String())
			}
			time.Sleep(20 * time.Millisecond)
		}
	}
	touch := func() {
		t.Helper()
		now := time.Now().Add(time.Duration(suiteRuns.Load()+1) * time.Minute)
		if err := os.Chtimes(tf, now, now); err != nil {
			t.Fatal(err)
		}
	}

	waitFor("the first run", func() bool { return suiteRuns.Load() == 1 && strings.Contains(out.String(), "run #1") })

	touch() // the edit whose rebuild fails
	waitFor("the failed rebuild", func() bool { return strings.Contains(out.String(), "the tests did not run") })
	// Nothing ran, so the run counter must not advance.
	waitFor("the loop to wait again", func() bool { return strings.Count(out.String(), "Watching tests + model") == 2 })
	if strings.Contains(out.String(), "run #2") {
		t.Errorf("the counter advanced for a round that ran nothing:\n%s", out.String())
	}
	// Two poll intervals: had the loop fallen through, the second run would be here.
	time.Sleep(2*watchPollInterval + 200*time.Millisecond)
	if got := suiteRuns.Load(); got != 1 {
		t.Fatalf("the suite ran %d time(s); after a failed rebuild it must not run against the old model", got)
	}
	if !strings.Contains(out.String(), "ERROR") || !strings.Contains(out.String(), "CE0117") {
		t.Errorf("the failed rebuild is not shown as red:\n%s", out.String())
	}

	touch() // the fix
	waitFor("the run after the fix", func() bool { return suiteRuns.Load() == 2 })

	waitFor("the loop to wait again", func() bool { return strings.Contains(out.String(), "run #2") })
	if err := syscall.Kill(os.Getpid(), syscall.SIGINT); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("the loop did not stop on SIGINT")
	}
	if final == nil || final.ErrorCount() != 0 {
		t.Errorf("the final result should be the green run after the fix, got %+v", final)
	}
}

// startWatch runs watchLoop on a scripted target and returns what a test needs
// to drive it. stop sends SIGINT and returns the loop's final result and error.
func startWatch(t *testing.T, target *fakeWatchTarget, reinject func() error) (tf string, out *syncBuffer, suiteRuns *atomic.Int32, stop func() (*SuiteResult, error)) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("stops the loop with SIGINT")
	}
	dir := t.TempDir()
	tf = filepath.Join(dir, "a.test.mdl")
	writeTestFile(t, tf, time.Now().Add(-time.Hour))
	suiteRuns = &atomic.Int32{}
	oldRun, oldReinject := runSuiteFn, reinjectTestsFn
	t.Cleanup(func() { runSuiteFn, reinjectTestsFn = oldRun, oldReinject })
	runSuiteFn = func(_ *endpointClient, _ docker.M2EEOptions, s *TestSuite, _ RunOptions, _ io.Writer) (*SuiteResult, error) {
		suiteRuns.Add(1)
		return &SuiteResult{Name: s.Name, Tests: []TestResult{{ID: "1", Name: "x", Status: StatusPass}}}, nil
	}
	reinjectTestsFn = func(RunOptions, *TestSuite, *TestSuite, io.Writer) error { return reinject() }
	opts := RunOptions{ProjectPath: filepath.Join(dir, "App.mpr"), TestFiles: []string{tf}}
	out = &syncBuffer{}
	var final *SuiteResult
	var finalErr error
	done := make(chan struct{})
	go func() {
		defer close(done)
		final, finalErr = watchLoop(opts, target, &TestSuite{Name: "mxtest"}, out, func(r *SuiteResult, err error) (*SuiteResult, error) {
			return r, err
		}, func(*TestSuite) {})
	}()
	stop = func() (*SuiteResult, error) {
		t.Helper()
		if err := syscall.Kill(os.Getpid(), syscall.SIGINT); err != nil {
			t.Fatal(err)
		}
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			t.Fatal("the loop did not stop on SIGINT")
		}
		return final, finalErr
	}
	return
}

func waitUntil(t *testing.T, out *syncBuffer, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s; output:\n%s", what, out.String())
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// A save made while the rebuild is failing must start a new round. The baseline
// used to be taken after the failed round, which absorbed that save.
func TestWatchLoopSeesASaveMadeDuringAFailingRebuild(t *testing.T) {
	inRebuild := make(chan struct{})
	release := make(chan struct{})
	target := &fakeWatchTarget{applyFn: func(call int) error {
		if call == 1 {
			close(inRebuild)
			<-release
			return errors.New("build failed: CE0117 broken")
		}
		return errors.New("build failed again")
	}}
	tf, out, suiteRuns, stop := startWatch(t, target, func() error { return nil })
	waitUntil(t, out, "the first run", func() bool { return suiteRuns.Load() == 1 && strings.Contains(out.String(), "Watching") })

	base := time.Now().Add(time.Hour)
	if err := os.Chtimes(tf, base, base); err != nil {
		t.Fatal(err)
	}
	select {
	case <-inRebuild:
	case <-time.After(15 * time.Second):
		t.Fatalf("no rebuild started:\n%s", out.String())
	}
	later := base.Add(time.Minute)
	if err := os.Chtimes(tf, later, later); err != nil { // the save during the rebuild
		t.Fatal(err)
	}
	close(release)
	waitUntil(t, out, "a second round for the save made during the rebuild", func() bool {
		target.mu.Lock()
		defer target.mu.Unlock()
		return target.applies >= 2
	})
	final, _ := stop()
	if final == nil || final.ErrorCount() == 0 {
		t.Errorf("a loop stopped after a failed change must end on the red result, got %+v", final)
	}
	if got := suiteRuns.Load(); got != 1 {
		t.Errorf("the suite ran %d times; it must not run against a model that failed to build", got)
	}
}

// A failed injection is a red round, the suite does not run, and nothing is rebuilt.
func TestWatchLoopFailedInjectionIsRedAndDoesNotRun(t *testing.T) {
	target := &fakeWatchTarget{applyFn: func(int) error { return nil }}
	tf, out, suiteRuns, stop := startWatch(t, target, func() error { return errors.New("mdl: CE1234 cannot create") })
	waitUntil(t, out, "the first run", func() bool { return suiteRuns.Load() == 1 && strings.Contains(out.String(), "Watching") })
	now := time.Now().Add(time.Hour)
	if err := os.Chtimes(tf, now, now); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, out, "the failed injection", func() bool { return strings.Contains(out.String(), "injecting the tests") })
	time.Sleep(2*watchPollInterval + 200*time.Millisecond)
	final, _ := stop()
	if final == nil || final.ErrorCount() != 1 || !strings.Contains(final.Tests[0].Message, "CE1234") {
		t.Errorf("final result is not the failed injection: %+v", final)
	}
	target.mu.Lock()
	defer target.mu.Unlock()
	if suiteRuns.Load() != 1 || target.applies != 0 {
		t.Errorf("suite runs %d, rebuilds %d; want 1 and 0", suiteRuns.Load(), target.applies)
	}
}
