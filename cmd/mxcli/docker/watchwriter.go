// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"os"
	"path/filepath"
	"strconv"
	"time"
)

// watchwriter.go lets an mxcli command that writes the model tell a
// `run --local --watch` dev loop when it starts and when it is done
// (ako/mxcli#2151).
//
// The watcher cannot see a write end, so it waits for the model source to stay
// unchanged for two polls before it builds (settleSource), on top of up to one
// poll to notice the change at all. Measured on a blank Mendix 11.16.0 app over
// eleven changes: 4.2-5.2 s from the end of `mxcli exec` to "applied", of which
// the build and the reload were 1.9-2.5 s. The rest was the watcher waiting for
// a writer that had already exited.
//
// `exec` knows both moments. While it runs it leaves a marker named after its
// pid; the watcher does not build while a live marker is there, which holds
// for a script of any length where the settle only guesses. When it is done it
// removes the marker and touches a stamp, and the watcher builds at once.
//
// Nothing changes for a writer that says nothing — an editor, Studio Pro, an
// older mxcli: its change is noticed and settled as before. A writer that dies
// without cleaning up is a marker whose pid is gone, which reads as done.

// watchWriterPoll is how often the watcher looks for a writer's signal. It is
// a ReadDir of a directory that is almost always empty.
const watchWriterPoll = 100 * time.Millisecond

const (
	watchWriterDirName = "watch-writers"
	watchWriterStamp   = "done"
)

func watchWriterDir(projectPath string) string {
	return filepath.Join(filepath.Dir(projectPath), ".mxcli", watchWriterDirName)
}

// watchRunLive reports whether a `run --local --watch` is serving the project.
// A seam for tests.
var watchRunLive = func(projectPath string) bool {
	st, err := ReadRunState(projectPath)
	return err == nil && st != nil && st.Watch && st.Ended.IsZero() && PidAlive(st.PID)
}

// BeginModelWrite tells a watching dev loop that this process is about to
// change the project, and returns the function that tells it the change is
// complete. Without a watching dev loop it does nothing and leaves no file.
//
// Call the returned function once the last write has landed. A process that
// exits without calling it is noticed by its pid, and its change is then built
// after the usual settle.
func BeginModelWrite(projectPath string) (done func()) {
	if projectPath == "" || !watchRunLive(projectPath) {
		return func() {}
	}
	dir := watchWriterDir(projectPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return func() {}
	}
	marker := filepath.Join(dir, strconv.Itoa(os.Getpid()))
	if err := os.WriteFile(marker, nil, 0o644); err != nil {
		return func() {}
	}
	return func() {
		// The stamp first: with the marker gone and no stamp yet, a poll in
		// between would see a writer that vanished without a word.
		stamp := filepath.Join(dir, watchWriterStamp)
		now := time.Now()
		if err := os.WriteFile(stamp, []byte(now.Format(time.RFC3339Nano)), 0o644); err == nil {
			_ = os.Chtimes(stamp, now, now)
		}
		_ = os.Remove(marker)
	}
}

// watchWriterGate is the watcher's side.
type watchWriterGate struct {
	projectPath string
	stamp       time.Time // the stamp as of the last poll with no writer
	sawWriter   bool      // a live writer was seen since then
}

// newWatchWriterGate starts from the stamp as it is, so a writer that finished
// before the loop began watching is not taken for a new one.
func newWatchWriterGate(projectPath string) *watchWriterGate {
	g := &watchWriterGate{projectPath: projectPath}
	g.stamp = g.readStamp()
	return g
}

func (g *watchWriterGate) readStamp() time.Time {
	fi, err := os.Stat(filepath.Join(watchWriterDir(g.projectPath), watchWriterStamp))
	if err != nil {
		return time.Time{}
	}
	return fi.ModTime()
}

// poll reports whether a writer is at work now, and whether one finished since
// the last poll that reported none at work. finished is reported once.
func (g *watchWriterGate) poll() (writing, finished bool) {
	dir := watchWriterDir(g.projectPath)
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		pid, err := strconv.Atoi(e.Name())
		if err != nil {
			continue // the stamp
		}
		if holdAlive(pid) {
			writing = true
			continue
		}
		// Its owner is gone without removing it: the write is over, however
		// it ended.
		_ = os.Remove(filepath.Join(dir, e.Name()))
		g.sawWriter = true
	}
	if writing {
		g.sawWriter = true
		return true, false
	}
	stamp := g.readStamp()
	finished = g.sawWriter || !stamp.Equal(g.stamp)
	g.stamp, g.sawWriter = stamp, false
	return false, finished
}
