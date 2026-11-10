// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// heldProject is a v2-shaped project with a hold on it, owned by a process
// whose liveness the test sets.
func heldProject(t *testing.T) (mpr string, alive *bool) {
	t.Helper()
	dir := t.TempDir()
	mpr = filepath.Join(dir, "App.mpr")
	touch(t, mpr, time.Now().Add(-time.Hour))
	touch(t, filepath.Join(dir, "mprcontents", "ab", "cd", "unit.mxunit"), time.Now().Add(-time.Hour))
	live := true
	old := holdAlive
	holdAlive = func(int) bool { return live }
	t.Cleanup(func() { holdAlive = old })
	if err := HoldWatch(mpr, 4242); err != nil {
		t.Fatal(err)
	}
	return mpr, &live
}

// field test on ako/mxcli#2104: a `test --attach` that injected test
// microflows made the dev loop rebuild and restart the app. While the attach
// holds the project the watcher waits; when it lets go and the model is back as
// it was, there is nothing to build.
func TestWatchHoldGate_WaitsThenAbsorbsAModelPutBack(t *testing.T) {
	mpr, _ := heldProject(t)
	g := &watchHoldGate{projectPath: mpr}
	last := time.Now()

	// The attach injects: a new document appears while the hold is live.
	injected := filepath.Join(filepath.Dir(mpr), "mprcontents", "ef", "01", "test.mxunit")
	touch(t, injected, time.Now())
	if v, h := g.tick(last); v != holdWait || h == nil || h.PID != 4242 {
		t.Fatalf("live hold: verdict %v, hold %+v; want holdWait for pid 4242", v, h)
	}

	// It takes the injection out and puts the project back, then releases.
	os.Remove(injected)
	ReleaseWatch(mpr)
	if v, _ := g.tick(last); v != holdAbsorbed {
		t.Fatalf("released over an unchanged model: verdict %v, want holdAbsorbed", v)
	}
	if v, _ := g.tick(last); v != holdNone {
		t.Errorf("after the release was handled: verdict %v, want holdNone", v)
	}
}

// A model that did not come back as it was (the developer edited it meanwhile)
// is built as usual.
func TestWatchHoldGate_BuildsAModelThatChanged(t *testing.T) {
	mpr, _ := heldProject(t)
	g := &watchHoldGate{projectPath: mpr}
	last := time.Now()
	if v, _ := g.tick(last); v != holdWait {
		t.Fatalf("verdict %v, want holdWait", v)
	}
	touch(t, filepath.Join(filepath.Dir(mpr), "mprcontents", "ab", "cd", "edited.mxunit"), time.Now())
	ReleaseWatch(mpr)
	if v, _ := g.tick(last); v != holdReleased {
		t.Errorf("released over a changed model: verdict %v, want holdReleased", v)
	}
}

// An edit made before the hold that the watcher had not built yet is still
// built, though the model at release equals the model at the hold.
func TestWatchHoldGate_BuildsAnEditPendingBeforeTheHold(t *testing.T) {
	mpr, _ := heldProject(t)
	g := &watchHoldGate{projectPath: mpr}
	before := time.Now().Add(-2 * time.Hour) // the watcher's last build predates the project's edits
	if v, _ := g.tick(before); v != holdWait {
		t.Fatalf("verdict %v, want holdWait", v)
	}
	ReleaseWatch(mpr)
	if v, _ := g.tick(before); v != holdReleased {
		t.Errorf("a change pending before the hold was absorbed: verdict %v, want holdReleased", v)
	}
}

// A hold whose owner died is no hold: a crashed test run cannot freeze the loop.
func TestWatchHoldGate_IgnoresADeadOwner(t *testing.T) {
	mpr, alive := heldProject(t)
	*alive = false
	g := &watchHoldGate{projectPath: mpr}
	if v, _ := g.tick(time.Now()); v != holdNone {
		t.Errorf("a dead owner's hold: verdict %v, want holdNone", v)
	}
}

// The digest is the stored model: equal for the same bytes, different for a
// changed .mpr or document.
func TestModelDigest(t *testing.T) {
	dir := t.TempDir()
	mpr := filepath.Join(dir, "App.mpr")
	touch(t, mpr, time.Now())
	unit := filepath.Join(dir, "mprcontents", "u.mxunit")
	touch(t, unit, time.Now())
	a, err := ModelDigest(mpr)
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := ModelDigest(mpr); b != a {
		t.Error("the same model digests differently")
	}
	os.WriteFile(unit, []byte("changed"), 0o644)
	if c, _ := ModelDigest(mpr); c == a {
		t.Error("a changed document did not change the digest")
	}
	v1 := filepath.Join(t.TempDir(), "Old.mpr")
	touch(t, v1, time.Now())
	if _, err := ModelDigest(v1); err != nil {
		t.Errorf("an MPR v1 project (no mprcontents/) failed: %v", err)
	}
}

// After an absorbed hold the run state says the build covers the project as it
// is now, so `run status` shows no change pending and `run wait` returns.
func TestRunStateCovered(t *testing.T) {
	dir := t.TempDir()
	mpr := filepath.Join(dir, "App.mpr")
	touch(t, mpr, time.Now())
	rec := NewRunStateRecorder(mpr, os.Getpid(), false, true, nil, "")
	boot := time.Now().Add(-time.Minute)
	rec.BuildFinished(BuildOutcome{Gen: 1, OK: true, Action: "boot", Source: boot})
	later := time.Now()
	rec.Covered(later)
	if st := rec.State(); st.Last == nil || !st.Last.Source.Equal(later) {
		t.Errorf("Last.Source = %v, want %v", st.Last, later)
	}
	rec.Covered(boot) // never moves back
	if st := rec.State(); !st.Last.Source.Equal(later) {
		t.Errorf("Covered moved the source back to %v", st.Last.Source)
	}
}
