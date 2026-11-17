// SPDX-License-Identifier: Apache-2.0

package projectlock

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ako/mxcli#2326: four `mxcli exec` processes writing one project at once
// all failed, with "disk I/O error (5898)", "replaced on disk while unwritten
// changes were held" and a temp-file rename error, and none said to retry. A
// writer now holds the project's lock for its run; a second one waits, and
// past its wait fails naming the holder.

func project(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	mpr := filepath.Join(dir, "App.mpr")
	if err := os.WriteFile(mpr, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	return mpr
}

func TestAcquire_SecondWriterWaitsThenFailsNamingTheHolder_Issue2326(t *testing.T) {
	mpr := project(t)
	release, err := Acquire(mpr, Options{Wait: time.Second, Purpose: "exec a.mdl"})
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	start := time.Now()
	_, err = Acquire(mpr, Options{Wait: 300 * time.Millisecond, Purpose: "exec b.mdl"})
	var busy *BusyError
	if !errors.As(err, &busy) {
		t.Fatalf("second Acquire = %v, want a BusyError", err)
	}
	if time.Since(start) < 250*time.Millisecond {
		t.Errorf("did not wait before failing")
	}
	for _, want := range []string{"another mxcli process", "exec a.mdl", "retry"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q lacks %q", err, want)
		}
	}
}

func TestAcquire_WaitingWriterGetsTheLockOnRelease_Issue2326(t *testing.T) {
	mpr := project(t)
	release, err := Acquire(mpr, Options{Wait: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		time.Sleep(150 * time.Millisecond)
		release()
	}()
	r2, err := Acquire(mpr, Options{Wait: 3 * time.Second})
	if err != nil {
		t.Fatalf("waiting writer did not get the lock after release: %v", err)
	}
	r2()
}

// A lock left by a process that is gone (crashed, killed -9) does not block
// the project: one whose holder is not alive on this host is taken over.
func TestAcquire_TakesOverADeadHoldersLock_Issue2326(t *testing.T) {
	mpr := project(t)
	path := Path(mpr)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	host, _ := os.Hostname()
	stale := "pid=999999999\nhost=" + host + "\npurpose=exec old.mdl\n"
	if err := os.WriteFile(path, []byte(stale), 0o600); err != nil {
		t.Fatal(err)
	}
	release, err := Acquire(mpr, Options{Wait: 200 * time.Millisecond})
	if err != nil {
		t.Fatalf("a dead holder's lock blocked the project: %v", err)
	}
	release()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("release left the lock file behind")
	}
}

// CONTROL: release is idempotent and does not remove a lock someone else has
// taken since.
func TestRelease_DoesNotRemoveAnotherHoldersLock_Issue2326(t *testing.T) {
	mpr := project(t)
	release, err := Acquire(mpr, Options{Wait: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	release()
	r2, err := Acquire(mpr, Options{Wait: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	release() // the first holder's release again
	if _, err := os.Stat(Path(mpr)); err != nil {
		t.Errorf("a second release removed the new holder's lock: %v", err)
	}
	r2()
}
