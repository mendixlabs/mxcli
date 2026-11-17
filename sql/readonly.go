// SPDX-License-Identifier: Apache-2.0

package sql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"unicode"
)

// ErrWriteNotAllowed is the refusal of a statement that is not a read when
// writes were not allowed.
var ErrWriteNotAllowed = errors.New("statement is not a read, and writes are not allowed")

// readKeywords are the leading keywords of statements that read. A statement
// starting with one of them can still write (a data-modifying CTE, EXPLAIN
// ANALYZE of a DELETE), which is why a read also runs in a transaction that
// is read-only and rolled back.
var readKeywords = map[string]bool{
	"select": true, "with": true, "explain": true, "show": true,
	"values": true, "table": true, "describe": true, "desc": true,
}

// ReadStatement reports whether query starts, after comments and opening
// parentheses, with a keyword of a reading statement.
func ReadStatement(query string) bool {
	return readKeywords[leadingKeyword(query)]
}

func leadingKeyword(q string) string {
	for {
		q = strings.TrimLeftFunc(q, func(r rune) bool { return unicode.IsSpace(r) || r == '(' })
		switch {
		case strings.HasPrefix(q, "--"):
			if i := strings.IndexByte(q, '\n'); i >= 0 {
				q = q[i+1:]
				continue
			}
			return ""
		case strings.HasPrefix(q, "/*"):
			if i := strings.Index(q, "*/"); i >= 0 {
				q = q[i+2:]
				continue
			}
			return ""
		}
		end := strings.IndexFunc(q, func(r rune) bool { return !unicode.IsLetter(r) && r != '_' })
		if end < 0 {
			end = len(q)
		}
		return strings.ToLower(q[:end])
	}
}

// Run executes query. Without allowWrite (ako/mxcli#2362) a statement that is
// not a read is refused, and a read runs in a read-only transaction that is
// rolled back, so a write hiding in a read — `with d as (delete …) select …`
// — fails instead of changing data. With allowWrite a non-read statement runs
// as an exec and the result carries the rows it affected.
func Run(ctx context.Context, conn *Connection, query string, allowWrite bool) (*QueryResult, error) {
	read := ReadStatement(query)
	switch {
	case !read && !allowWrite:
		kw := leadingKeyword(query)
		if kw == "" {
			kw = "this statement"
		} else {
			kw = strings.ToUpper(kw)
		}
		return nil, fmt.Errorf("%w: %s can change the database, and sql runs read-only unless --write is given", ErrWriteNotAllowed, kw)
	case !read:
		res, err := conn.DB.ExecContext(ctx, query)
		if err != nil {
			return nil, fmt.Errorf("statement failed: %w", err)
		}
		out := &QueryResult{Rows: [][]any{}}
		if n, err := res.RowsAffected(); err == nil {
			out.RowsAffected = &n
		}
		return out, nil
	case allowWrite:
		return Execute(ctx, conn, query)
	}
	return executeReadOnly(ctx, conn, query)
}

func executeReadOnly(ctx context.Context, conn *Connection, query string) (*QueryResult, error) {
	// PostgreSQL honours a read-only transaction option; the SQL Server driver
	// rejects it, and Oracle is told with a statement. The rollback is the
	// guard that holds on every driver.
	opts := &sql.TxOptions{ReadOnly: conn.Driver == DriverPostgres}
	tx, err := conn.DB.BeginTx(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("begin read-only transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if conn.Driver == DriverOracle {
		if _, err := tx.ExecContext(ctx, "SET TRANSACTION READ ONLY"); err != nil {
			return nil, fmt.Errorf("set read-only transaction: %w", err)
		}
	}
	rows, err := tx.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("query failed: %w", err)
	}
	defer rows.Close()
	return scanRows(rows)
}
