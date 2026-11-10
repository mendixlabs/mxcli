// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// A watch hold is how another mxcli process tells a `run --local --watch` dev
// loop that it is changing the project for a while and will put it back.
//
// `mxcli test --attach` injects its test microflows into the same project the
// dev loop watches, builds them through a serve process of its own, runs the
// tests, and takes them out again. The watcher saw each of those writes as an
// edit: it rebuilt (the first build failing on a half-written model) and
// applied the injected model by restarting the runtime, which drops every
// browser session, including a --hub preview's (field test on ako/mxcli#2104,
// the same disruption as ako/mxcli#2063). So the attach holds the watcher while
// it owns the project, and when it lets go the watcher compares the model with
// the one the hold started from: unchanged means there is nothing to build.
//
// A hold whose owner is gone counts as released, so a crashed test run cannot
// freeze the dev loop.

const watchHoldName = "watch-hold.json"

// WatchHold is the content of the hold file.
type WatchHold struct {
	// PID is the process holding the watcher; the hold lasts while it lives.
	PID int `json:"pid"`
	// Digest is ModelDigest of the project when the hold began, before the
	// holder changed anything.
	Digest string `json:"digest"`
	// Source is the dev loop's change signal (SourceMTimeOf) when the hold
	// began. An edit the watcher had not built yet makes it newer than the
	// watcher's baseline, and then the release builds even if the digest matches.
	Source  time.Time `json:"source"`
	Started time.Time `json:"started"`
}

func watchHoldPath(projectPath string) string {
	return filepath.Join(filepath.Dir(projectPath), ".mxcli", watchHoldName)
}

// HoldWatch writes a hold for pid on the project as it is now. Call it before
// changing anything; ReleaseWatch ends it.
func HoldWatch(projectPath string, pid int) error {
	digest, err := ModelDigest(projectPath)
	if err != nil {
		return err
	}
	st, _ := ReadRunState(projectPath)
	h := WatchHold{PID: pid, Digest: digest, Source: SourceMTimeOf(projectPath, st), Started: time.Now()}
	body, err := json.Marshal(h)
	if err != nil {
		return err
	}
	path := watchHoldPath(projectPath)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, body, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// ReleaseWatch ends a hold. Releasing a project that has none is not an error.
func ReleaseWatch(projectPath string) { os.Remove(watchHoldPath(projectPath)) }

// readWatchHold returns the hold on the project, or nil when there is none.
func readWatchHold(projectPath string) *WatchHold {
	body, err := os.ReadFile(watchHoldPath(projectPath))
	if err != nil {
		return nil
	}
	var h WatchHold
	if json.Unmarshal(body, &h) != nil {
		return nil
	}
	return &h
}

// holdAlive is PidAlive, a seam for the tests.
var holdAlive = PidAlive

// activeWatchHold returns the hold on the project when its owner is alive.
func activeWatchHold(projectPath string) *WatchHold {
	h := readWatchHold(projectPath)
	if h == nil || !holdAlive(h.PID) {
		return nil
	}
	return h
}

// ModelDigest hashes the project's model as stored: the .mpr file and, for MPR
// v2, every file under mprcontents/ (path and content). Two equal digests are
// the same model byte for byte, which is what lets the watcher skip a build
// after a hold: a test run puts the .mpr back exactly (projectSnapshot), so a
// model that went out and came back reads as unchanged.
func ModelDigest(projectPath string) (string, error) {
	h := sha256.New()
	if err := hashFile(h, projectPath); err != nil {
		return "", err
	}
	dir := filepath.Join(filepath.Dir(projectPath), "mprcontents")
	var paths []string
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) && p == dir {
				return filepath.SkipDir
			}
			return err
		}
		if !d.IsDir() {
			paths = append(paths, p)
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	sort.Strings(paths)
	for _, p := range paths {
		rel, _ := filepath.Rel(dir, p)
		io.WriteString(h, filepath.ToSlash(rel)+"\x00")
		if err := hashFile(h, p); err != nil {
			return "", err
		}
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func hashFile(w io.Writer, path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(w, f)
	return err
}

// watchHoldGate is the watcher's side of a hold: it remembers the hold it saw,
// and decides on release whether the change it left behind needs a build.
type watchHoldGate struct {
	projectPath string
	held        *WatchHold
}

// holdVerdict is what the watcher does on a tick.
type holdVerdict int

const (
	holdNone     holdVerdict = iota // no hold, or one already settled: carry on as usual
	holdWait                        // a live hold: do not build
	holdAbsorbed                    // released, and the model is as before: move the baseline, do not build
	holdReleased                    // released, and something changed: build as usual
)

// tick reports what the watcher should do now. last is the watcher's baseline,
// the source time its latest build covered.
func (g *watchHoldGate) tick(last time.Time) (holdVerdict, *WatchHold) {
	if h := activeWatchHold(g.projectPath); h != nil {
		if g.held == nil {
			g.held = h
		}
		return holdWait, g.held
	}
	if g.held == nil {
		return holdNone, nil
	}
	h := g.held
	g.held = nil
	// An edit made before the hold and not yet built must still be built.
	if h.Source.After(last) {
		return holdReleased, h
	}
	if d, err := ModelDigest(g.projectPath); err == nil && d == h.Digest {
		return holdAbsorbed, h
	}
	return holdReleased, h
}
