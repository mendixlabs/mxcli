// SPDX-License-Identifier: Apache-2.0

package sql

import (
	"context"
	"strings"
	"testing"
)

// ako/mxcli#2415: `VIEWS (…)` without `TABLES (…)` generated every table as
// well, then appended the named views on top. On a database whose objects are
// views — a DuckGraphLake lake serving Mendix entity CDC — each named view came
// out twice, so one connector held two entities and two queries under the same
// name:
//
//	2 QUERY GetAllCustomers
//	2 ENTITY LakeSql.Customer
//
// Two causes compounded: `cfg.Tables == nil` meant "all tables" with nothing
// suppressing it when `Views` was set, and the table reader did not look at
// information_schema's table_type, so a view was already in the "tables" set
// before the view pass added it again.
//
// Same harness as the #2414 tests: modernc SQLite with an attached
// information_schema, DriverPostgres so the real query templates run.

// twoSchemaLake is three CDC views in two schemas plus one real table, so a
// test can tell a table pass from a view pass and a schema from a name.
func twoSchemaLake(t *testing.T) *Connection {
	conn := openSQLite(t, "clauses",
		`ATTACH DATABASE ':memory:' AS information_schema`,
		`CREATE TABLE information_schema.tables (table_schema TEXT, table_name TEXT, table_type TEXT)`,
		`CREATE TABLE information_schema.columns (table_schema TEXT, table_name TEXT,
			column_name TEXT, data_type TEXT, is_nullable TEXT, column_default TEXT, ordinal_position INTEGER)`,
		`INSERT INTO information_schema.tables VALUES
			('cdcpublish', 'customer', 'VIEW'),
			('cdcpublish', 'order', 'VIEW'),
			('otherapp', 'invoice', 'VIEW'),
			('public', 'audit_log', 'BASE TABLE')`,
		`INSERT INTO information_schema.columns VALUES
			('cdcpublish', 'customer', 'id', 'bigint', 'NO', NULL, 1),
			('cdcpublish', 'customer', 'Name', 'character varying', 'YES', NULL, 2),
			('cdcpublish', 'order', 'id', 'bigint', 'NO', NULL, 1),
			('otherapp', 'invoice', 'id', 'bigint', 'NO', NULL, 1),
			('public', 'audit_log', 'id', 'bigint', 'NO', NULL, 1)`)
	conn.Driver = DriverPostgres
	return conn
}

func gen(t *testing.T, conn *Connection, tables, views []string) *GenerateResult {
	t.Helper()
	res, err := GenerateConnector(context.Background(), &GenerateConfig{
		Conn: conn, Module: "LakeSql", Alias: "lake", Tables: tables, Views: views,
	})
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func queryNames(mdl string) []string {
	var out []string
	for _, line := range strings.Split(mdl, "\n") {
		if f := strings.Fields(strings.TrimSpace(line)); len(f) == 2 && f[0] == "QUERY" {
			out = append(out, f[1])
		}
	}
	return out
}

func duplicates(names []string) []string {
	seen, dup := map[string]bool{}, []string(nil)
	for _, n := range names {
		if seen[n] {
			dup = append(dup, n)
		}
		seen[n] = true
	}
	return dup
}

func TestGenerateConnector_ViewsClauseDoesNotPullInEveryTable_Issue2415(t *testing.T) {
	res := gen(t, twoSchemaLake(t), nil, []string{"cdcpublish.customer"})

	if got := queryNames(res.MDL); len(got) != 1 || got[0] != "GetAllCustomers" {
		t.Errorf("VIEWS (cdcpublish.customer) generated %v, want just [GetAllCustomers]", got)
	}
	for _, unwanted := range []string{"audit_log", "AuditLog", "invoice", "Invoice", "GetAllOrders"} {
		if strings.Contains(res.MDL, unwanted) {
			t.Errorf("an object nobody asked for reached the output: %q", unwanted)
		}
	}
	if res.TableCount != 0 || res.ViewCount != 1 {
		t.Errorf("counts = %d tables, %d views; want 0 and 1", res.TableCount, res.ViewCount)
	}
}

// Naming one object in both clauses must not emit it twice under one name —
// two entities called LakeSql.Customer in one connector is not a model Mendix
// can load.
func TestGenerateConnector_ObjectInBothClausesEmittedOnce_Issue2415(t *testing.T) {
	for _, tc := range []struct{ name, table, view string }{
		{"same spelling", "cdcpublish.customer", "cdcpublish.customer"},
		// Resolving a bare name to its schema is what makes the two spellings
		// one object; without it the dedupe key differs and the duplicate is
		// back.
		{"bare and qualified", "customer", "cdcpublish.customer"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			res := gen(t, twoSchemaLake(t), []string{tc.table}, []string{tc.view})
			if dup := duplicates(queryNames(res.MDL)); len(dup) > 0 {
				t.Errorf("duplicate query name(s) %v in\n%s", dup, res.MDL)
			}
			if n := strings.Count(res.MDL, "CREATE NON-PERSISTENT ENTITY"); n != 1 {
				t.Errorf("%d entities, want 1 — the same object was emitted twice:\n%s", n, res.MDL)
			}
		})
	}
}

// A bare name in a clause has to be addressed by its schema too, or the query
// fails exactly as ako/mxcli#2414 described. The #2414 fix reached the
// discovery path only.
func TestGenerateConnector_BareClauseNameIsQualified_Issue2415(t *testing.T) {
	res := gen(t, twoSchemaLake(t), []string{"customer"}, nil)
	if !strings.Contains(res.MDL, "FROM cdcpublish.customer") {
		t.Errorf("TABLES (customer) must still address the schema it resolved to:\n%s", res.MDL)
	}
}

// CONTROL: with neither clause, everything is still generated — tables and
// views alike. Filtering views out of the table reader without enumerating
// them would silently empty the output on a view-only database, which is what
// a CDC lake is.
func TestGenerateConnector_NoClauseGeneratesEverythingOnce_Issue2415(t *testing.T) {
	res := gen(t, twoSchemaLake(t), nil, nil)

	names := queryNames(res.MDL)
	if dup := duplicates(names); len(dup) > 0 {
		t.Errorf("duplicate query name(s) %v with no clause in\n%s", dup, res.MDL)
	}
	if len(names) != 4 {
		t.Errorf("%d queries (%v), want 4 — three views and one table", len(names), names)
	}
	for _, want := range []string{
		"FROM cdcpublish.customer", "FROM cdcpublish.order",
		"FROM otherapp.invoice", "FROM public.audit_log",
	} {
		if !strings.Contains(res.MDL, want) {
			t.Errorf("want %q in\n%s", want, res.MDL)
		}
	}
	// The counts have to say which is which, since that is the only report of
	// what the command decided.
	if res.TableCount != 1 || res.ViewCount != 3 {
		t.Errorf("counts = %d tables, %d views; want 1 and 3", res.TableCount, res.ViewCount)
	}
}

// ReadAllTableSchemas is exported and says tables. It must not hand back views
// — that conflation is half of this defect.
func TestReadAllTableSchemas_ExcludesViews_Issue2415(t *testing.T) {
	got, err := ReadAllTableSchemas(context.Background(), twoSchemaLake(t))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Name != "audit_log" || got[0].IsView {
		var names []string
		for _, ts := range got {
			names = append(names, ts.QualifiedName())
		}
		t.Errorf("ReadAllTableSchemas returned %v, want just the one BASE TABLE public.audit_log", names)
	}
}

// ReadAllSchemas is the one the no-clause path uses: every object, each marked
// with what it is.
func TestReadAllSchemas_MarksViewsAsViews_Issue2415(t *testing.T) {
	got, err := ReadAllSchemas(context.Background(), twoSchemaLake(t))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 4 {
		t.Fatalf("%d objects, want 4", len(got))
	}
	want := map[string]bool{ // address -> IsView
		"cdcpublish.customer": true, "cdcpublish.order": true,
		"otherapp.invoice": true, "public.audit_log": false,
	}
	for _, ts := range got {
		isView, known := want[ts.QualifiedName()]
		if !known {
			t.Errorf("unexpected object %q", ts.QualifiedName())
			continue
		}
		if ts.IsView != isView {
			t.Errorf("%s: IsView = %v, want %v", ts.QualifiedName(), ts.IsView, isView)
		}
	}
}
