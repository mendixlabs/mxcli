// SPDX-License-Identifier: Apache-2.0

package sql

import (
	"context"
	"errors"
	"os"
	"testing"
)

// ako/mxcli#2362: `mxcli sql` ran any statement. `delete from sales.orders`
// printed "(0 rows)", which reads as an empty select, and the rows were gone.
// A statement now runs read-only unless writes are allowed.

func TestReadStatement(t *testing.T) {
	for q, want := range map[string]bool{
		"select 1":          true,
		"  SELECT * from t": true,
		"-- note\nselect 1": true,
		"/* c */ with x as (select 1) select * from x": true,
		"(select 1)":               true,
		"explain select 1":         true,
		"show search_path":         true,
		"values (1)":               true,
		"delete from sales.orders": false,
		"update t set a = 1":       false,
		"insert into t values (1)": false,
		"drop table t":             false,
		"truncate t":               false,
		"create table t (a int)":   false,
		"alter table t add b int":  false,
		"grant select on t to u":   false,
		"call p()":                 false,
		"exec p":                   false,
		"merge into t using s on (1=1) when matched then delete": false,
		"": false,
	} {
		if got := ReadStatement(q); got != want {
			t.Errorf("ReadStatement(%q) = %v, want %v", q, got, want)
		}
	}
}

// pgConn connects to the PostgreSQL in MXCLI_TEST_PG_DSN, with sales.orders
// holding rows; skipped where there is none.
func pgConn(t *testing.T) *Connection {
	t.Helper()
	dsn := os.Getenv("MXCLI_TEST_PG_DSN")
	if dsn == "" {
		t.Skip("MXCLI_TEST_PG_DSN not set")
	}
	m := NewManager()
	t.Cleanup(m.CloseAll)
	if err := m.Connect(DriverPostgres, dsn, "t"); err != nil {
		t.Fatal(err)
	}
	c, _ := m.Get("t")
	if _, err := c.DB.Exec(`drop table if exists ro_probe; create table ro_probe(id int); insert into ro_probe values (1),(2)`); err != nil {
		t.Fatal(err)
	}
	return c
}

func probeCount(t *testing.T, c *Connection) int {
	t.Helper()
	var n int
	if err := c.DB.QueryRow(`select count(*) from ro_probe`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestRun_ReadOnlyRefusesAWrite_Issue2362(t *testing.T) {
	c := pgConn(t)
	ctx := context.Background()

	_, err := Run(ctx, c, "delete from ro_probe", false)
	if !errors.Is(err, ErrWriteNotAllowed) {
		t.Fatalf("delete without write: err = %v, want ErrWriteNotAllowed", err)
	}
	if n := probeCount(t, c); n != 2 {
		t.Fatalf("rows after a refused delete = %d, want 2", n)
	}

	// A write that reads as a select — a data-modifying CTE — is stopped by the
	// read-only transaction, not by the keyword.
	_, err = Run(ctx, c, "with d as (delete from ro_probe returning id) select * from d", false)
	if err == nil {
		t.Errorf("a deleting CTE ran in read-only mode")
	}
	if n := probeCount(t, c); n != 2 {
		t.Fatalf("rows after a read-only CTE delete = %d, want 2", n)
	}

	// Reads still work.
	res, err := Run(ctx, c, "select id from ro_probe order by id", false)
	if err != nil || len(res.Rows) != 2 {
		t.Fatalf("select: %v, %+v", err, res)
	}
}

// CONTROL: with writes allowed the delete runs and says how many rows it hit.
func TestRun_WriteAllowedReportsAffectedRows_Issue2362(t *testing.T) {
	c := pgConn(t)
	res, err := Run(context.Background(), c, "delete from ro_probe where id = 1", true)
	if err != nil {
		t.Fatal(err)
	}
	if res.RowsAffected == nil || *res.RowsAffected != 1 {
		t.Errorf("RowsAffected = %v, want 1", res.RowsAffected)
	}
	if n := probeCount(t, c); n != 1 {
		t.Errorf("rows after an allowed delete = %d, want 1", n)
	}
}
