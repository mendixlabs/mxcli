// SPDX-License-Identifier: Apache-2.0

//go:build !js

package catalog

import (
	"sync"
	"testing"
	"time"
)

// ako/mxcli#2417: an in-memory catalog's schema must be visible on every
// connection the pool hands out.
//
// sql.Open("sqlite", ":memory:") returns a POOL, and each connection to
// ":memory:" is its own separate, empty database. createTables() therefore
// populated whichever connection it happened to get, and any later query the
// pool served on a different connection saw an empty database and failed with
// "no such table: <whatever it asked for>".
//
// The symptom was not a clean failure. On a freshly built catalog
// LintContext.AttributesFor failed with "no such table: attributes" while
// ctx.Entities() on the same handle in the same run returned rows, because the
// two queries landed on different connections. Which query failed was down to
// pool timing, so it reproduced on one project and not another, and a lint run
// reported "No issues found." with the error printed after the findings.
//
// Forcing a second concurrent connection is what makes this deterministic: a
// single sequential query reuses the one connection and passes either way.
func TestInMemoryCatalogSchemaIsVisibleOnEveryPooledConnection(t *testing.T) {
	c, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	// Hold one query open so a second must use another connection, and read a
	// table on each. Two tables that the linter reads through different code
	// paths (entities via Entities, attributes via AttributesFor) are the pair
	// that actually diverged.
	var wg sync.WaitGroup
	errs := make(chan error, 16)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for _, table := range []string{"attributes", "entities", "activities", "widgets"} {
				rows, err := c.CatalogDB().Query(`SELECT count(*) FROM ` + table)
				if err != nil {
					errs <- err
					return
				}
				rows.Close()
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Errorf("query on a pooled connection failed: %v", err)
	}
}

// The pragma set at open time has the same problem: db.Exec runs it on ONE
// connection, so with a pool the others would not have it. Pinning the pool to
// a single connection is what makes the pragma hold for every query too.
func TestInMemoryCatalogForeignKeysPragmaHoldsForEveryQuery(t *testing.T) {
	c, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	var wg sync.WaitGroup
	bad := make(chan int, 16)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rows, err := c.CatalogDB().Query(`PRAGMA foreign_keys`)
			if err != nil {
				bad <- -1
				return
			}
			defer rows.Close()
			if rows.Next() {
				var on int
				if err := rows.Scan(&on); err == nil && on != 1 {
					bad <- on
				}
			}
		}()
	}
	wg.Wait()
	close(bad)
	for v := range bad {
		t.Errorf("foreign_keys = %d on a pooled connection, want 1", v)
	}
}

// A nested query while a cursor is open must not deadlock.
//
// This is the production pattern, not a contrived one: LintContext.Entities()
// yields rows from an open cursor and its consumers call AttributesFor() inside
// the loop, so a second query runs while the first still holds a connection.
//
// Regression guard for a fix that was tried and rejected. Pinning the pool to a
// single connection (db.SetMaxOpenConns(1)) also gives an in-memory catalog one
// database and looks like the obvious fix, but it makes this nested read wait
// forever for the connection the outer cursor holds — measured as a lint run
// hanging past 600s where it had taken seconds. The shared-cache DSN is what
// gives one database AND the connections these reads need.
//
// The test would hang rather than fail under that regression, so it carries its
// own deadline instead of relying on the package timeout to tell the story.
func TestNestedQueryWhileCursorOpenDoesNotDeadlock(t *testing.T) {
	c, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	done := make(chan error, 1)
	go func() {
		outer, err := c.CatalogDB().Query(`SELECT Id FROM entities`)
		if err != nil {
			done <- err
			return
		}
		defer outer.Close()
		// Query while `outer` is open, as the linter's iterators do. Reaching
		// this at all is the point; an empty catalog yields no rows, so the
		// nested read is issued before any Next().
		inner, err := c.CatalogDB().Query(`SELECT count(*) FROM attributes`)
		if err != nil {
			done <- err
			return
		}
		inner.Close()
		done <- nil
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("nested query failed: %v", err)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("nested query while a cursor is open deadlocked: the pool cannot serve a second connection")
	}
}
