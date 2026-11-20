// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// writeUnits simulates what an `mxcli exec` does to the model on disk: many
// files rewritten over a stretch of time, not one atomic change.
func writeUnits(t *testing.T, dir string, n int, gap time.Duration) {
	t.Helper()
	for i := 0; i < n; i++ {
		path := filepath.Join(dir, "unit"+string(rune('a'+i))+".mxunit")
		if err := os.WriteFile(path, []byte{byte(i)}, 0o644); err != nil {
			t.Error(err)
			return
		}
		time.Sleep(gap)
	}
}

// TestSettleSourceWaitsForAMultiFileWrite is the regression test for the stale
// build. The watcher used to rebuild on the first mtime bump, so a multi-second
// exec was deployed half-applied — and once exec became byte-idempotent,
// re-running it wrote nothing and there was no way to re-trigger a build.
func TestSettleSourceWaitsForAMultiFileWrite(t *testing.T) {
	dir := t.TempDir()
	mpr := filepath.Join(dir, "App.mpr")
	if err := os.WriteFile(mpr, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	contents := filepath.Join(dir, "mprcontents")
	if err := os.MkdirAll(contents, 0o755); err != nil {
		t.Fatal(err)
	}

	// The gap between unit writes is well inside one poll, which is what an exec
	// actually does — it rewrites units as fast as the disk takes them, over a
	// span of seconds. A writer that paused longer than the settle window between
	// files would be indistinguishable from a finished one, and no timeout-based
	// watcher can tell those apart.
	const poll = 20 * time.Millisecond
	var done atomic.Bool
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		writeUnits(t, contents, 12, poll/4)
		done.Store(true)
	}()

	settled := settleSource(func() time.Time { return sourceMTime(mpr) }, sourceMTime(mpr), poll, nil)
	// Read the writer's state at the moment settleSource returned — after
	// wg.Wait() every unit is on disk regardless, so that assertion would hold
	// against a settleSource that returned immediately.
	finishedFirst := done.Load()
	wg.Wait()

	if settled.IsZero() {
		t.Fatal("settleSource reported an interrupt that never happened")
	}
	if !finishedFirst {
		t.Error("settleSource returned while the model was still being written — the build would be half-applied")
	}
	if final := sourceMTime(mpr); settled.Before(final) {
		t.Errorf("settled at %v but the source is newer (%v)", settled, final)
	}
}

// countingTick stands in for the poll timer: every call fires immediately and
// records that a poll happened, so a test can count polls instead of timing them.
func countingTick(polls *int) func() <-chan time.Time {
	return func() <-chan time.Time {
		*polls++
		ch := make(chan time.Time, 1)
		ch <- time.Now()
		return ch
	}
}

// TestSettleSourceReturnsPromptlyForOneChange guards the other direction: a
// single editor save must not pay a long wait.
//
// "Promptly" is a POLL COUNT, not a duration. This test used to assert
// `elapsed <= poll * (sourceSettleWindow + 3)` — 100ms against a nominal 40ms —
// and failed on a loaded CI runner at 196ms with nothing wrong: time.After
// guarantees at least its duration and nothing about the upper bound, so
// bounding wall-clock as a multiple of it is not a property the scheduler
// offers. Counting polls asserts the same guarantee and cannot flake.
func TestSettleSourceReturnsPromptlyForOneChange(t *testing.T) {
	dir := t.TempDir()
	mpr := filepath.Join(dir, "App.mpr")
	if err := os.WriteFile(mpr, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	polls := 0
	settled := settleSourceWith(func() time.Time { return sourceMTime(mpr) }, sourceMTime(mpr), nil, countingTick(&polls))

	if settled.IsZero() {
		t.Fatal("settleSource reported an interrupt that never happened")
	}
	if polls != sourceSettleWindow {
		t.Errorf("a quiet source cost %d polls, want exactly %d", polls, sourceSettleWindow)
	}
}

// TestSettleSourceNeedsConsecutiveQuietPolls pins the half of the window the
// timing test could never see: the source must be quiet for sourceSettleWindow
// polls IN A ROW, so a write part-way through the window restarts the count
// rather than being tolerated. Without it, `quiet = 0` could be dropped from the
// change branch and every test here would still pass.
//
// Only expressible now that polls are countable — with a real timer this needed
// a write timed against a wall clock, which is the flake this file just removed.
func TestSettleSourceNeedsConsecutiveQuietPolls(t *testing.T) {
	dir := t.TempDir()
	mpr := filepath.Join(dir, "App.mpr")
	if err := os.WriteFile(mpr, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	start := sourceMTime(mpr)

	// Touch the source once, on the poll right before the window would close.
	polls, touched := 0, false
	tick := func() <-chan time.Time {
		polls++
		if polls == sourceSettleWindow && !touched {
			touched = true
			if err := os.Chtimes(mpr, start.Add(time.Second), start.Add(time.Second)); err != nil {
				t.Error(err)
			}
		}
		ch := make(chan time.Time, 1)
		ch <- time.Now()
		return ch
	}

	settled := settleSourceWith(func() time.Time { return sourceMTime(mpr) }, start, nil, tick)

	if !touched {
		t.Fatal("the write never happened — the test proved nothing")
	}
	// The touch lands on the last poll of the first window; that poll observes
	// the change and resets instead of closing, so a full window runs again.
	if want := sourceSettleWindow * 2; polls != want {
		t.Errorf("a source touched mid-window cost %d polls, want %d "+
			"(the quiet counter did not restart)", polls, want)
	}
	if settled.Before(start.Add(time.Second)) {
		t.Errorf("settled at %v, before the last write — the build would miss it", settled)
	}
}

// TestSettleSourceHonoursInterrupt — Ctrl-C during a long exec must still stop
// the loop rather than wait the write out.
func TestSettleSourceHonoursInterrupt(t *testing.T) {
	dir := t.TempDir()
	mpr := filepath.Join(dir, "App.mpr")
	if err := os.WriteFile(mpr, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	sigCh := make(chan os.Signal, 1)
	sigCh <- os.Interrupt

	if settled := settleSource(func() time.Time { return sourceMTime(mpr) }, sourceMTime(mpr), 20*time.Millisecond, sigCh); !settled.IsZero() {
		t.Errorf("settleSource ignored the interrupt, returned %v", settled)
	}
}
