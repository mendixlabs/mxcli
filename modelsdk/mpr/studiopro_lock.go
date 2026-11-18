// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Studio Pro open-project guard (mendixlabs/mxcli#849).
//
// Studio Pro keeps the model in memory and does not reload it when another
// process changes the files. A write mxcli makes while the project is open is
// silently discarded the next time Studio Pro saves: the write succeeds, `mx
// check` passes, and the change is gone. So every write that would reach storage
// is refused while Studio Pro has the project open, unless the caller opted out
// (exec --force, or MXCLI_ALLOW_STUDIO_PRO_OPEN=1). Reads are never refused, and
// a write that reconciliation elides as a no-op never gets here, so re-running a
// script that is already applied keeps working.
//
// The signal is the lock file Studio Pro creates beside the .mpr while the
// project is open and removes on close: `<project>.mpr.lock`. Studio Pro's own
// generated .gitignore lists it (with the project name LOWER-cased — TestApp's
// says `testapp.mpr.lock` beside `TestApp.mpr`), so the name is matched without
// regard to case. A Studio Pro that crashed leaves the file behind; the message
// says so and how to proceed, because failing toward noise is the point — the
// failure this guards against is silent.

// AllowWritesWhileStudioProOpen disables the guard for this process. It is the
// `--force` of the CLI; nothing sets it implicitly.
var AllowWritesWhileStudioProOpen bool

// AllowStudioProOpenEnv is the environment variable that disables the guard,
// for commands that have no --force of their own (REPL, -c, other writers).
const AllowStudioProOpenEnv = "MXCLI_ALLOW_STUDIO_PRO_OPEN"

// StudioProOpenError reports a refused write.
type StudioProOpenError struct {
	MprPath  string
	LockPath string
	LockTime time.Time
}

func (e *StudioProOpenError) Error() string {
	return fmt.Sprintf("refusing to write %s: Studio Pro has this project open (lock file %s, last modified %s). "+
		"Studio Pro does not reload the model from disk, so its next save would silently discard this change. "+
		"Close the project in Studio Pro and retry, or write through Studio Pro with --mcp. "+
		"If Studio Pro is not running (a lock left behind by a crash), delete the lock file, "+
		"or re-run with --force (exec) or %s=1 to write anyway",
		filepath.Base(e.MprPath), e.LockPath, e.LockTime.Format(time.RFC3339), AllowStudioProOpenEnv)
}

// StudioProLockFile returns the path of the Studio Pro lock file beside mprPath,
// or "" when there is none.
func StudioProLockFile(mprPath string) (string, time.Time) {
	if mprPath == "" {
		return "", time.Time{}
	}
	dir, base := filepath.Split(mprPath)
	if dir == "" {
		dir = "."
	}
	want := base + ".lock"
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", time.Time{}
	}
	for _, e := range entries {
		if e.IsDir() || !strings.EqualFold(e.Name(), want) {
			continue
		}
		var mod time.Time
		if info, err := e.Info(); err == nil {
			mod = info.ModTime()
		}
		return filepath.Join(dir, e.Name()), mod
	}
	return "", time.Time{}
}

// CheckStudioProOpen is the guard for a command that writes many units in a
// run that cannot stop half-way cleanly: it asks once, before the run starts,
// so the refusal is one message and the project untouched (ako/mxcli#2366).
func CheckStudioProOpen(mprPath string) error { return studioProOpenGuard(mprPath) }

// studioProOpenGuard is called immediately before a write reaches storage.
func studioProOpenGuard(mprPath string) error {
	if AllowWritesWhileStudioProOpen || os.Getenv(AllowStudioProOpenEnv) == "1" {
		return nil
	}
	lock, mod := StudioProLockFile(mprPath)
	if lock == "" {
		return nil
	}
	return &StudioProOpenError{MprPath: mprPath, LockPath: lock, LockTime: mod}
}

// guardWrite applies the guard to this writer's project. Every write passes
// here before it changes anything, which is also what lets an open Journal copy
// the .mpr before the first change (journal.go).
func (w *Writer) guardWrite() error {
	if w.simulating {
		return ErrSimulatedWrite
	}
	if err := studioProOpenGuard(w.reader.path); err != nil {
		return err
	}
	return journalDB(w.reader.path)
}
