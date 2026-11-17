// SPDX-License-Identifier: Apache-2.0

// Package projectlock serialises the mxcli processes that write one project.
//
// Four `mxcli exec` processes writing one project at once all failed
// (ako/mxcli#2326): each held the .mpr open, wrote units, and found the
// others' writes under it — "disk I/O error (5898)", "replaced on disk while
// unwritten changes were held", a temp-file rename that lost its race — and
// each rolled back, so none of the work landed and no message said to retry.
// A write run is not a single SQLite transaction that SQLite could serialise:
// it is a run of unit files and an index, and a failing run restores its
// backups, which would put back files a parallel run had just written.
//
// So a run that writes holds the project's lock from start to finish, and a
// second one waits for it. The lock is a file created with O_EXCL — the one
// primitive that means the same on Linux, macOS and Windows — under the
// project's .mxcli folder. It names its holder, so a refusal can say who, and
// is kept fresh by a heartbeat, so a lock left by a crashed run (killed -9, a
// container gone) is taken over instead of locking the project for good: at
// once when its holder is a process on this host that no longer exists, else
// once the heartbeat has stopped for staleAfter.
package projectlock

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/mendixlabs/mxcli/internal/procalive"
)

const (
	// heartbeat is how often a holder touches its lock.
	heartbeat = 2 * time.Second
	// staleAfter is how long a lock may go untouched before it is taken over
	// although its holder cannot be checked (another host, a container).
	staleAfter = 30 * time.Second
	poll       = 50 * time.Millisecond
	// DefaultWait is how long a writer waits for another to finish.
	DefaultWait = 60 * time.Second
)

// Options configure Acquire.
type Options struct {
	// Wait bounds the wait for a live holder; 0 is DefaultWait, negative is
	// no wait.
	Wait time.Duration
	// Purpose says what the run does ("exec script.mdl"), for the message a
	// waiting writer gets.
	Purpose string
}

// BusyError is the refusal when another process held the lock for the whole
// wait.
type BusyError struct {
	Project string
	Holder  string
	Waited  time.Duration
	Lock    string
}

func (e *BusyError) Error() string {
	return fmt.Sprintf("another mxcli process is writing %s (%s), and it still was after %s: "+
		"retry when it has finished — writes to one project run one at a time. "+
		"If no mxcli process is running, delete %s",
		filepath.Base(e.Project), e.Holder, e.Waited.Round(time.Second), e.Lock)
}

// Path is the lock file of the project at mprPath.
func Path(mprPath string) string {
	return filepath.Join(filepath.Dir(mprPath), ".mxcli", "write.lock")
}

// Acquire blocks until this process holds the project's write lock, and
// returns the function that releases it. Release is safe to call more than
// once, and never removes a lock another process has taken since.
func Acquire(mprPath string, opts Options) (release func(), err error) {
	path := Path(mprPath)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	wait := opts.Wait
	if wait == 0 {
		wait = DefaultWait
	}
	token := newToken()
	host, _ := os.Hostname()
	content := fmt.Sprintf("pid=%d\nhost=%s\npurpose=%s\nsince=%s\ntoken=%s\n",
		os.Getpid(), host, oneLine(opts.Purpose), time.Now().UTC().Format(time.RFC3339), token)

	start := time.Now()
	for {
		f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err == nil {
			_, werr := f.WriteString(content)
			cerr := f.Close()
			if werr != nil || cerr != nil {
				_ = os.Remove(path)
				return nil, fmt.Errorf("write the project lock %s: %v", path, firstErr(werr, cerr))
			}
			return holdLock(path, token), nil
		}
		if !os.IsExist(err) {
			return nil, err
		}
		holder, abandoned := inspect(path, host)
		if abandoned {
			_ = os.Remove(path) // a race to take it over is settled by O_EXCL
			continue
		}
		if wait < 0 || time.Since(start) >= wait {
			return nil, &BusyError{Project: mprPath, Holder: holder, Waited: time.Since(start), Lock: path}
		}
		time.Sleep(poll)
	}
}

// holdLock starts the heartbeat and returns the release.
func holdLock(path, token string) func() {
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		t := time.NewTicker(heartbeat)
		defer t.Stop()
		for {
			select {
			case <-stop:
				return
			case now := <-t.C:
				if owns(path, token) {
					_ = os.Chtimes(path, now, now)
				}
			}
		}
	}()
	released := false
	return func() {
		if released {
			return
		}
		released = true
		close(stop)
		<-done
		if owns(path, token) {
			_ = os.Remove(path)
		}
	}
}

// inspect describes the lock's holder and reports whether the lock is
// abandoned: its holder is a process on this host that is gone, or its
// heartbeat stopped staleAfter ago.
func inspect(path, host string) (holder string, abandoned bool) {
	raw, err := os.ReadFile(path)
	if err != nil {
		// Released between our create and this read: try again.
		return "", os.IsNotExist(err)
	}
	fields := map[string]string{}
	for _, line := range strings.Split(string(raw), "\n") {
		if k, v, ok := strings.Cut(line, "="); ok {
			fields[k] = v
		}
	}
	holder = "pid " + fields["pid"]
	if fields["host"] != "" && fields["host"] != host {
		holder += " on " + fields["host"]
	}
	if p := fields["purpose"]; p != "" {
		holder += ", " + p
	}
	if fields["host"] == host {
		if pid, err := strconv.Atoi(fields["pid"]); err == nil && !procalive.Alive(pid) {
			return holder, true
		}
	}
	if st, err := os.Stat(path); err == nil && time.Since(st.ModTime()) > staleAfter {
		return holder, true
	}
	return holder, false
}

// owns reports whether the lock file is still the one this holder wrote.
func owns(path, token string) bool {
	raw, err := os.ReadFile(path)
	return err == nil && strings.Contains(string(raw), "token="+token+"\n")
}

func newToken() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }

func firstErr(errs ...error) error {
	for _, e := range errs {
		if e != nil {
			return e
		}
	}
	return nil
}
