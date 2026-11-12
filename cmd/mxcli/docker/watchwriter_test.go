// SPDX-License-Identifier: Apache-2.0

// The watcher waited ~2.7 s after `mxcli exec` had exited before it built,
// because it cannot see a write end and settles instead (ako/mxcli#2151).
package docker

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func writerProject(t *testing.T, watching bool) string {
	t.Helper()
	project := filepath.Join(t.TempDir(), "App.mpr")
	prev := watchRunLive
	watchRunLive = func(string) bool { return watching }
	t.Cleanup(func() { watchRunLive = prev })
	return project
}

// The signal the watcher builds on: nothing while the writer runs, "finished"
// exactly once when it is done.
func TestWatchWriter_FinishedIsReportedOnceAfterTheWrite(t *testing.T) {
	project := writerProject(t, true)
	g := newWatchWriterGate(project)

	if writing, finished := g.poll(); writing || finished {
		t.Fatalf("no writer yet: poll = writing %v, finished %v", writing, finished)
	}
	done := BeginModelWrite(project)
	if writing, finished := g.poll(); !writing || finished {
		t.Fatalf("while the writer runs the watcher must wait: poll = writing %v, finished %v", writing, finished)
	}
	done()
	if writing, finished := g.poll(); writing || !finished {
		t.Fatalf("after the write: poll = writing %v, finished %v, want finished", writing, finished)
	}
	if writing, finished := g.poll(); writing || finished {
		t.Fatalf("finished must be reported once: poll = writing %v, finished %v", writing, finished)
	}
}

// A write that begins and ends between two polls is still seen: the stamp
// outlives the marker.
func TestWatchWriter_WriteBetweenTwoPollsIsSeen(t *testing.T) {
	project := writerProject(t, true)
	g := newWatchWriterGate(project)
	g.poll()

	BeginModelWrite(project)()
	if writing, finished := g.poll(); writing || !finished {
		t.Fatalf("poll = writing %v, finished %v, want finished", writing, finished)
	}
}

// A writer that exited without saying so is over all the same; the watcher
// must not wait on it for ever.
func TestWatchWriter_DeadWriterCountsAsFinished(t *testing.T) {
	project := writerProject(t, true)
	g := newWatchWriterGate(project)
	dir := watchWriterDir(project)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(dir, strconv.Itoa(424242))
	if err := os.WriteFile(marker, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	prev := holdAlive
	holdAlive = func(int) bool { return false }
	t.Cleanup(func() { holdAlive = prev })

	if writing, finished := g.poll(); writing || !finished {
		t.Fatalf("poll = writing %v, finished %v, want finished", writing, finished)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("a dead writer's marker must be removed: %v", err)
	}
}

// A stamp left by an earlier run is not a new write.
func TestWatchWriter_StampFromBeforeTheLoopIsNotASignal(t *testing.T) {
	project := writerProject(t, true)
	BeginModelWrite(project)()

	g := newWatchWriterGate(project)
	if writing, finished := g.poll(); writing || finished {
		t.Fatalf("poll = writing %v, finished %v, want neither", writing, finished)
	}
}

// Without a watching dev loop, a write leaves nothing behind in .mxcli/.
func TestWatchWriter_NoWatchingLoopNoFiles(t *testing.T) {
	project := writerProject(t, false)
	BeginModelWrite(project)()
	if _, err := os.Stat(watchWriterDir(project)); !os.IsNotExist(err) {
		t.Fatalf("no dev loop is watching, so no marker directory should exist: %v", err)
	}
}
