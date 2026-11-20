// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

// Reported (mendixlabs/mxcli#1810, mirrored as ako/mxcli#2129): under
// `run --local` with security on and the trial licence, every probe sign-in
// leaves a System.Session row; after six the runtime refuses sign-in with HTTP
// 560 "Maximum number of sessions exceeded! (You are currently using a trial
// license)". The rows survive a restart — 20 had piled up in the field — and
// the only way out was to stop the app, delete from "system$session", and start
// it again.

type fakeExec struct {
	stmts   []string
	results map[string]int64
	missing map[string]bool // statements that fail with undefined_table
}

type rowsResult int64

func (r rowsResult) LastInsertId() (int64, error) { return 0, nil }
func (r rowsResult) RowsAffected() (int64, error) { return int64(r), nil }

func (f *fakeExec) ExecContext(_ context.Context, q string, _ ...any) (sql.Result, error) {
	f.stmts = append(f.stmts, q)
	for k := range f.missing {
		if strings.Contains(q, k) {
			return nil, &pgconn.PgError{Code: "42P01", Message: "relation does not exist"}
		}
	}
	for k, n := range f.results {
		if strings.Contains(q, k) {
			return rowsResult(n), nil
		}
	}
	return rowsResult(0), nil
}

func TestClearSessionRows_DeletesSessionsAndTheirUserLinks(t *testing.T) {
	f := &fakeExec{results: map[string]int64{`"system$session"`: 6}}
	n, err := clearSessionRows(context.Background(), f)
	if err != nil {
		t.Fatal(err)
	}
	if n != 6 {
		t.Errorf("cleared = %d, want 6", n)
	}
	joined := strings.Join(f.stmts, "\n")
	for _, want := range []string{`delete from "system$session_user"`, `delete from "system$session"`} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing statement %q in:\n%s", want, joined)
		}
	}
}

// A fresh database has no System tables until the runtime's first start creates
// them; that is nothing to clear, not an error.
func TestClearSessionRows_FreshDatabaseIsNotAnError(t *testing.T) {
	f := &fakeExec{missing: map[string]bool{`"system$session`: true}}
	n, err := clearSessionRows(context.Background(), f)
	if err != nil || n != 0 {
		t.Fatalf("fresh database: n=%d err=%v, want 0, nil", n, err)
	}
}

func TestClearSessionRows_OtherErrorsAreReported(t *testing.T) {
	boom := errors.New("permission denied")
	n, err := clearSessionRows(context.Background(), execFunc(func(string) error { return boom }))
	if !errors.Is(err, boom) || n != 0 {
		t.Fatalf("n=%d err=%v, want the error", n, err)
	}
}

type execFunc func(string) error

func (e execFunc) ExecContext(_ context.Context, q string, _ ...any) (sql.Result, error) {
	return nil, e(q)
}

func TestClearSessions_RefusesTheFileDatabase(t *testing.T) {
	_, err := ClearSessions(DBConfig{Type: RuntimeDatabaseType(DBTypeHSQLDB)})
	if err == nil || !strings.Contains(err.Error(), "HSQLDB") {
		t.Fatalf("err = %v, want a refusal naming HSQLDB", err)
	}
}

// The sign-in hint names the way out that needs no restart.
func TestLoginFailureHint_NamesClearSessions(t *testing.T) {
	path := t.TempDir() + "/runtime.log"
	mustWrite(t, path, "LicenseRuntimeException: "+sessionCapMarker+" (You are currently using a trial license)\n")
	hint := loginFailureHint(path)
	for _, want := range []string{"mxcli run clear-sessions", "560", "6"} {
		if !strings.Contains(hint, want) {
			t.Errorf("hint missing %q:\n%s", want, hint)
		}
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
