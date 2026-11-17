// SPDX-License-Identifier: Apache-2.0

package sql

import (
	"context"
	"strings"
	"testing"
)

// ako/mxcli#2361: `--tables` prints schema and table apart, so the next step
// is `--describe sales.customers` — which returned zero rows and no error, as
// if the table had no columns.
func TestDescribeTable_SchemaQualified_Issue2361(t *testing.T) {
	c := pgConn(t)
	if _, err := c.DB.Exec(`create schema if not exists dq; drop table if exists dq.customers;
		create table dq.customers (id serial primary key, name varchar(50) not null, email text)`); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	res, err := DescribeTable(ctx, c, "dq.customers")
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Rows) != 3 {
		t.Errorf("dq.customers: %d columns, want 3", len(res.Rows))
	}

	// CONTROL: the unqualified form, which worked before, still does for a
	// name one schema holds.
	if _, err := c.DB.Exec(`drop table if exists dq.only_here; create table dq.only_here (a int, b int)`); err != nil {
		t.Fatal(err)
	}
	if res, err := DescribeTable(ctx, c, "only_here"); err != nil || len(res.Rows) != 2 {
		t.Errorf("only_here: %v, %+v", err, res)
	}

	// A name in two schemas is refused, naming both, rather than listing both
	// tables' columns as one.
	if _, err := c.DB.Exec(`create schema if not exists dq2; drop table if exists dq2.customers; create table dq2.customers (x int)`); err != nil {
		t.Fatal(err)
	}
	if _, err := DescribeTable(ctx, c, "customers"); err == nil ||
		!strings.Contains(err.Error(), "dq.customers") || !strings.Contains(err.Error(), "dq2.customers") {
		t.Errorf("ambiguous customers: err = %v, want one naming dq.customers and dq2.customers", err)
	}

	// A table that does not exist is an error naming it, not an empty list.
	_, err = DescribeTable(ctx, c, "dq.nosuch")
	if err == nil || !strings.Contains(err.Error(), "dq.nosuch") {
		t.Errorf("missing table: err = %v, want one naming dq.nosuch", err)
	}
}

func TestSplitTableName(t *testing.T) {
	for in, want := range map[string][2]string{
		"customers":          {"", "customers"},
		"sales.customers":    {"sales", "customers"},
		`"Sales"."Cust"`:     {"Sales", "Cust"},
		"db.sales.customers": {"sales", "customers"},
	} {
		s, n := splitTableName(in)
		if s != want[0] || n != want[1] {
			t.Errorf("splitTableName(%q) = %q, %q; want %q, %q", in, s, n, want[0], want[1])
		}
	}
}
