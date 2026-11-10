// SPDX-License-Identifier: Apache-2.0

package testrunner

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// withRestoreSeams replaces the liveness check and the removal for one test.
func withRestoreSeams(t *testing.T, alive bool, remove func(string, projectState, io.Writer) error) {
	t.Helper()
	oldAlive, oldRemove := pidAlive, removeInstalled
	pidAlive = func(int) bool { return alive }
	removeInstalled = remove
	t.Cleanup(func() { pidAlive, removeInstalled = oldAlive, oldRemove })
}

func recordedProject(t *testing.T) string {
	t.Helper()
	mpr := filepath.Join(t.TempDir(), "App.mpr")
	if err := os.WriteFile(mpr, []byte("before"), 0o644); err != nil {
		t.Fatal(err)
	}
	st := projectState{
		afterStartup:  "MyFirstModule.ASU",
		createdMxTest: true,
		snapshot:      projectSnapshot{path: mpr, contents: []byte("before"), treeDigest: "d1"},
	}
	if err := writeRestoreRecord(mpr, 4242, st); err != nil {
		t.Fatal(err)
	}
	return mpr
}

// The run that installed the endpoint is gone (taskkill /F, SIGKILL): the
// removal runs with the state it recorded, and the record goes with it.
func TestRestoreAbandonedEndpoint_RemovesWhatADeadRunLeft(t *testing.T) {
	mpr := recordedProject(t)
	var got projectState
	withRestoreSeams(t, false, func(_ string, st projectState, _ io.Writer) error {
		got = st
		return nil
	})

	restored, err := RestoreAbandonedEndpoint(mpr, io.Discard)
	if err != nil || !restored {
		t.Fatalf("restored=%v err=%v, want true, nil", restored, err)
	}
	if got.afterStartup != "MyFirstModule.ASU" || !got.createdMxTest {
		t.Errorf("removal got %+v, want the recorded after-startup and createdMxTest", got)
	}
	if string(got.snapshot.contents) != "before" || got.snapshot.treeDigest != "d1" || got.snapshot.path != mpr {
		t.Errorf("removal got snapshot %q/%q/%q, want the saved project file", got.snapshot.contents, got.snapshot.treeDigest, got.snapshot.path)
	}
	for _, p := range []string{restoreRecordPath(mpr), restoreMprPath(mpr)} {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Errorf("%s survives a successful removal", p)
		}
	}
}

// Control: an endpoint whose run is still serving is that run's to remove.
func TestRestoreAbandonedEndpoint_LeavesALiveRunsEndpoint(t *testing.T) {
	mpr := recordedProject(t)
	withRestoreSeams(t, true, func(string, projectState, io.Writer) error {
		t.Fatal("removed the endpoint of a run that is still alive")
		return nil
	})
	if restored, err := RestoreAbandonedEndpoint(mpr, io.Discard); restored || err != nil {
		t.Fatalf("restored=%v err=%v, want false, nil", restored, err)
	}
	if _, err := os.Stat(restoreRecordPath(mpr)); err != nil {
		t.Errorf("the live run's record was removed: %v", err)
	}
}

// A failed removal keeps the record, so the next stop or run tries again.
func TestRestoreAbandonedEndpoint_KeepsTheRecordWhenRemovalFails(t *testing.T) {
	mpr := recordedProject(t)
	withRestoreSeams(t, false, func(string, projectState, io.Writer) error { return errors.New("drop failed") })
	if _, err := RestoreAbandonedEndpoint(mpr, io.Discard); err == nil {
		t.Fatal("a failed removal reported success")
	}
	if _, err := os.Stat(restoreRecordPath(mpr)); err != nil {
		t.Errorf("the record was removed although the endpoint is still in the project: %v", err)
	}
}

func TestRestoreAbandonedEndpoint_NoRecordIsNoOp(t *testing.T) {
	mpr := filepath.Join(t.TempDir(), "App.mpr")
	withRestoreSeams(t, false, func(string, projectState, io.Writer) error {
		t.Fatal("removal ran without a record")
		return nil
	})
	if restored, err := RestoreAbandonedEndpoint(mpr, io.Discard); restored || err != nil {
		t.Fatalf("restored=%v err=%v, want false, nil", restored, err)
	}
}

func withAfterStartup(t *testing.T, af string) {
	t.Helper()
	old := afterStartupOf
	afterStartupOf = func(string) (string, error) { return af, nil }
	t.Cleanup(func() { afterStartupOf = old })
}

// A run killed with its container leaves the after-startup microflow pointing
// at the endpoint, and nothing said so until a `run stop` — so the swap could be
// committed. With the restore record still there, the fix is `run stop`.
func TestLeftoverEndpointWarning_DeadRunWithRecord(t *testing.T) {
	mpr := recordedProject(t)
	withRestoreSeams(t, false, nil)
	withAfterStartup(t, endpointStartupFlow)
	got := LeftoverEndpointWarning(mpr)
	if !strings.Contains(got, endpointStartupFlow) || !strings.Contains(got, "mxcli run stop -p "+mpr) || !strings.Contains(got, "4242") {
		t.Errorf("warning = %q", got)
	}
}

// Without a record (the .mxcli/ folder went too) only the user knows what the
// setting was, so the warning says how to set it rather than promising a restore.
func TestLeftoverEndpointWarning_NoRecord(t *testing.T) {
	mpr := filepath.Join(t.TempDir(), "App.mpr")
	withAfterStartup(t, endpointStartupFlow)
	got := LeftoverEndpointWarning(mpr)
	if !strings.Contains(got, "nothing records") || !strings.Contains(got, "alter settings runtime") || strings.Contains(got, "run stop") {
		t.Errorf("warning = %q", got)
	}
}

// Controls: a live run's endpoint is in use, and a project whose after-startup
// is its own needs nothing said.
func TestLeftoverEndpointWarning_Controls(t *testing.T) {
	mpr := recordedProject(t)
	withRestoreSeams(t, true, nil)
	withAfterStartup(t, endpointStartupFlow)
	if got := LeftoverEndpointWarning(mpr); got != "" {
		t.Errorf("a live run's endpoint was reported: %q", got)
	}
	withRestoreSeams(t, false, nil)
	withAfterStartup(t, "MyFirstModule.ASU")
	if got := LeftoverEndpointWarning(mpr); got != "" {
		t.Errorf("a project's own after-startup was reported: %q", got)
	}
}
