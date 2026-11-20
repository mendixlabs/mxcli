// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"
	"time"
)

// ako/mxcli#2043 (mendixlabs/mxcli#1780): `exec` panicked with a nil pointer
// in GetRawUnitByName, and the project was left half-written — no rollback.
// The statement had outlasted the per-statement limit. Execute returned the
// timeout but left the statement's goroutine running; exec then closed the
// project (Disconnect sets the reader to nil) to roll back, the goroutine read
// through the nil reader, and its panic killed the process before the rollback
// and before the timeout was even printed.
//
// The fixture holds a statement inside an implicit catalog build for longer
// than the limit: the build reads the project once the hook returns, which is
// the abandoned goroutine touching the backend after the timeout.

// Close waits for a statement the guard abandoned before it disconnects, so
// the statement neither reads a closed project nor runs alongside a rollback.
func TestCloseWaitsForAnAbandonedStatement(t *testing.T) {
	exec, _ := sourceCachedFixture(t)
	t.Setenv("MXCLI_EXEC_TIMEOUT", "300ms")
	rebuilt := slowImplicitRebuilds(t, 1500*time.Millisecond, 300*time.Millisecond)
	touchProject(t, exec)

	err := execStatement(t, exec, "SHOW REFERENCES TO MyFirstModule.Ticket")
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("want the timeout, got %v", err)
	}
	// What exec does next: close the project, then roll back.
	if err := exec.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	select {
	case <-rebuilt:
	default:
		t.Fatal("Close returned while the abandoned statement was still running")
	}
}

// A statement that outlasts even the wait at Close may then fail on the closed
// project, but as an error nobody reads, not as a panic that ends the process
// before the caller rolls back.
func TestAStatementStillRunningAfterCloseDoesNotCrash(t *testing.T) {
	exec, _ := sourceCachedFixture(t)
	t.Setenv("MXCLI_EXEC_TIMEOUT", "200ms")
	rebuilt := slowImplicitRebuilds(t, 1500*time.Millisecond, 200*time.Millisecond)
	prev := abandonedStatementWait
	abandonedStatementWait = 100 * time.Millisecond
	t.Cleanup(func() { abandonedStatementWait = prev })
	touchProject(t, exec)

	if err := execStatement(t, exec, "SHOW REFERENCES TO MyFirstModule.Ticket"); err == nil {
		t.Fatal("want the timeout")
	}
	err := exec.Close()
	if err == nil || !strings.Contains(err.Error(), "still running") {
		t.Errorf("Close = %v, want it to say a statement is still running", err)
	}
	// The abandoned build now reads through the closed project. Before the fix
	// this panicked in its goroutine and ended the test binary.
	select {
	case <-rebuilt:
	case <-time.After(time.Minute):
		t.Fatal("the abandoned statement did not finish")
	}
	exec.WaitForAbandonedStatements(time.Minute)
}

// A panic inside a statement is the statement's error, so the caller's
// rollback still runs.
func TestAPanickingStatementReturnsAnError(t *testing.T) {
	exec, _ := sourceCachedFixture(t)
	prevHook := implicitCatalogBuildHook
	implicitCatalogBuildHook = func() func() { panic("boom in a handler") }
	t.Cleanup(func() { implicitCatalogBuildHook = prevHook })
	touchProject(t, exec)

	t.Setenv("MXCLI_EXEC_TIMEOUT", "1m")
	err := execStatement(t, exec, "SHOW REFERENCES TO MyFirstModule.Ticket")
	if err == nil || !strings.Contains(err.Error(), "boom in a handler") || !strings.Contains(err.Error(), "internal error") {
		t.Errorf("err = %v, want the panic as an internal error", err)
	}
}
