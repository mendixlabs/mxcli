// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	_ "github.com/jackc/pgx/v5/stdlib" // registers the "pgx" driver
)

// A local run's runtime has no licence, and the trial licence caps concurrent
// sessions at six (mendixlabs/mxcli#1810). Every browser sign-in — a
// Playwright probe's included — leaves a System.Session row, and the licence
// check counts those ROWS: measured on 11.12.1, a seventh sign-in answers HTTP
// 560 with six rows present, and deleting them from the running app's database
// makes the next sign-in succeed at once, no restart. The login page reports
// the 560 as "Sign in failed", which reads as a defect in whatever was just
// changed.
//
// A restart frees the slots too — the runtime logs "Deleting old sessions from
// database..." at startup — but costs a boot. `mxcli run clear-sessions` is
// the cheap way: it deletes the rows the cap is counted from.

// sessionClearTimeout bounds the clear. It is housekeeping: a database that
// does not answer in this time is reported, not waited on.
const sessionClearTimeout = 10 * time.Second

// sqlExecer is the part of *sql.DB the clear uses, so it can be tested without
// a server.
type sqlExecer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// clearSessionRows deletes every System.Session row and the Session_User links
// that point at them, and returns how many sessions it deleted. A database the
// runtime has not initialised yet has neither table, which is nothing to clear.
func clearSessionRows(ctx context.Context, db sqlExecer) (int64, error) {
	// The association table first. Its foreign key cascades (measured on
	// 11.12.1: ON DELETE CASCADE; backgroundjob and changehash SET NULL), so this
	// is belt and braces for a schema that lacks the constraint.
	if _, err := db.ExecContext(ctx, `delete from "system$session_user"`); err != nil && !isUndefinedTable(err) {
		return 0, err
	}
	res, err := db.ExecContext(ctx, `delete from "system$session"`)
	if err != nil {
		if isUndefinedTable(err) {
			return 0, nil
		}
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}

// isUndefinedTable reports PostgreSQL's 42P01: the relation does not exist.
func isUndefinedTable(err error) bool {
	var pe *pgconn.PgError
	return errors.As(err, &pe) && pe.Code == "42P01"
}

// ClearSessions deletes every session in a local run's database and returns
// how many it deleted. Every signed-in browser has to sign in again.
func ClearSessions(db DBConfig) (int64, error) {
	if db.IsFileBased() {
		return 0, fmt.Errorf("the built-in HSQLDB database is held open by the runtime and cannot be reached from another process")
	}
	return clearLocalSessions(db)
}

func clearLocalSessions(db DBConfig) (int64, error) {
	host, port := splitHostPort(db.Host)
	u := url.URL{
		Scheme:   "postgres",
		User:     url.UserPassword(db.User, db.Password),
		Host:     host + ":" + port,
		Path:     "/" + db.Name,
		RawQuery: "sslmode=disable&connect_timeout=5",
	}
	conn, err := sql.Open("pgx", u.String())
	if err != nil {
		return 0, err
	}
	defer conn.Close()
	ctx, cancel := context.WithTimeout(context.Background(), sessionClearTimeout)
	defer cancel()
	return clearSessionRows(ctx, conn)
}
