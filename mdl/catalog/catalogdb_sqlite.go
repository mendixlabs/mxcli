// SPDX-License-Identifier: Apache-2.0

//go:build !js

package catalog

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"sync/atomic"

	_ "modernc.org/sqlite"
)

// SqliteCatalogDB wraps *sql.DB from modernc.org/sqlite.
type SqliteCatalogDB struct {
	db *sql.DB
	// keepAlive pins one connection open for an in-memory catalog's lifetime.
	// A shared-cache in-memory database is destroyed when its LAST connection
	// closes, and the pool may close idle ones, so without this a quiet moment
	// could discard the catalog. nil for a file-backed catalog.
	keepAlive *sql.Conn
}

// memCatalogSeq names each in-memory catalog apart from the others.
var memCatalogSeq atomic.Uint64

// NewSqliteCatalogDB opens an in-memory SQLite database.
//
// The DSN names a SHARED-CACHE database rather than a bare ":memory:"
// (ako/mxcli#2417). Every connection to ":memory:" is its own separate, empty
// database and sql.Open hands back a POOL, so the schema createTables wrote
// landed on one connection while a query the pool served on another saw
// nothing. That surfaced as LintContext.AttributesFor failing with "no such
// table: attributes" on a freshly built catalog while ctx.Entities() succeeded
// in the same run on the same handle. Which query broke came down to pool
// timing, so it reproduced on one project and not another, and lint printed
// "No issues found." with the error after the findings.
//
// Pinning the pool to one connection would also give one database and is NOT
// the fix: the readers query while a cursor is open — ctx.Entities() yields
// rows and the loop body calls ctx.AttributesFor() — so one connection
// deadlocks, the inner read waiting for the connection the outer cursor holds.
// Measured: a lint run that took seconds hung past 600s. A shared cache gives
// one database AND keeps the connections those nested reads need.
//
// The name is unique per catalog so two in one process stay separate, and the
// pragma rides in the DSN because a pragma set through Exec reaches only the
// one pooled connection it ran on.
func NewSqliteCatalogDB() (*SqliteCatalogDB, error) {
	dsn := fmt.Sprintf("file:mxcli-catalog-%d-%d?mode=memory&cache=shared&_pragma=foreign_keys(1)",
		os.Getpid(), memCatalogSeq.Add(1))
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	conn, err := db.Conn(context.Background())
	if err != nil {
		db.Close()
		return nil, err
	}
	return &SqliteCatalogDB{db: db, keepAlive: conn}, nil
}

// fileBusyTimeoutMs is how long a connection to an on-disk catalog waits for a
// lock held by another process before failing with SQLITE_BUSY. Opening a cache
// writes to it (schema upgrade, schema version), so parallel mxcli processes on
// one project contend for the write lock for a moment (ako/mxcli#951).
const fileBusyTimeoutMs = 10000

// NewSqliteCatalogDBFromFile opens a file-based SQLite database.
func NewSqliteCatalogDBFromFile(path string) (*SqliteCatalogDB, error) {
	dsn := path
	// The busy timeout goes in the DSN so it applies to every pooled connection,
	// not only the one a PRAGMA statement happens to run on. A '?' in the path
	// would be read as the start of the query string; keep the bare path then.
	if !strings.Contains(path, "?") {
		dsn = fmt.Sprintf("%s?_pragma=busy_timeout(%d)", path, fileBusyTimeoutMs)
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		db.Close()
		return nil, err
	}
	return &SqliteCatalogDB{db: db}, nil
}

func (s *SqliteCatalogDB) Query(query string, args ...any) (*sql.Rows, error) {
	return s.db.Query(query, args...)
}

func (s *SqliteCatalogDB) QueryRow(query string, args ...any) *sql.Row {
	return s.db.QueryRow(query, args...)
}

func (s *SqliteCatalogDB) Exec(query string, args ...any) (sql.Result, error) {
	return s.db.Exec(query, args...)
}

func (s *SqliteCatalogDB) Begin() (CatalogTx, error) {
	return s.db.Begin()
}

func (s *SqliteCatalogDB) Close() error {
	// The pinned connection goes first: it is what keeps an in-memory
	// catalog's shared cache alive.
	if s.keepAlive != nil {
		_ = s.keepAlive.Close()
		s.keepAlive = nil
	}
	return s.db.Close()
}

// RawDB returns the underlying *sql.DB. Used only for SQLite-specific
// operations like VACUUM INTO in SaveToFile.
func (s *SqliteCatalogDB) RawDB() *sql.DB {
	return s.db
}

// WrapSqlDB wraps an existing *sql.DB as a CatalogDB.
// Used by tests that create their own in-memory databases.
func WrapSqlDB(db *sql.DB) *SqliteCatalogDB {
	return &SqliteCatalogDB{db: db}
}
