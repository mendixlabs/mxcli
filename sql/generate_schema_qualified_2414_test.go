// SPDX-License-Identifier: Apache-2.0

package sql

import (
	"context"
	"strings"
	"testing"
)

// ako/mxcli#2414: the generated connector addressed every table by its bare
// name, so none of its queries could be run against the database they were
// generated from:
//
//	SELECT id, CustomerNumber, Name, Email, Active FROM customer
//	ERROR:  Table with name customer does not exist!
//	Did you mean "cdcpublish.customer"?
//
// SHOW TABLES returns (table_schema, table_name) and the reader kept only the
// name, so the schema never reached TableSchema and the FROM clause could not
// be qualified. Measured against a DuckGraphLake lake serving Mendix entity
// CDC, where each publishing app's views live in a schema of their own.
//
// This is tested on the emitted text rather than against a live database: the
// symptom is the query string, and a DSN-gated test would skip in CI, which is
// how ako/mxcli#808 shipped green.

func cdcSchemas() []*TableSchema {
	return []*TableSchema{
		{Schema: "cdcpublish", Name: "customer", Columns: []ColumnSchema{
			{Name: "id", DataType: "bigint"},
			{Name: "Name", DataType: "varchar"},
		}},
		{Schema: "cdcpublish", Name: "order", IsView: true, Columns: []ColumnSchema{
			{Name: "id", DataType: "bigint"},
			{Name: "OrderNumber", DataType: "bigint"},
		}},
	}
}

func TestRenderConnector_QualifiesTheTableWithItsSchema_Issue2414(t *testing.T) {
	cfg := &GenerateConfig{
		Conn:   &Connection{Driver: DriverPostgres, Alias: "lake"},
		Module: "LakeSql",
		Alias:  "lake",
	}
	got := renderConnector(cfg, cdcSchemas(), &GenerateResult{}).MDL

	for _, want := range []string{"FROM cdcpublish.customer", "FROM cdcpublish.order"} {
		if !strings.Contains(got, want) {
			t.Errorf("generated SQL does not address the table by schema: want %q in\n%s", want, got)
		}
	}
	// The bare form is the defect, not merely a missing improvement: a query
	// that reads FROM customer fails outright on the database it came from.
	for _, bare := range []string{"FROM customer", "FROM order"} {
		if strings.Contains(got, bare+"'") {
			t.Errorf("generated SQL still addresses a table unqualified: %q", bare)
		}
	}
	// The schema belongs in the address only. It must not leak into the Mendix
	// names, which are what a modeller and every expression refer to.
	for _, want := range []string{"ENTITY LakeSql.Customer (", "ENTITY LakeSql.Order (",
		"QUERY GetAllCustomers", "QUERY GetAllOrders"} {
		if !strings.Contains(got, want) {
			t.Errorf("want %q in\n%s", want, got)
		}
	}
	if strings.Contains(got, "LakeSql.CdcpublishCustomer") || strings.Contains(got, "Cdcpublish_") {
		t.Error("the schema reached an entity name")
	}
}

// CONTROL: a driver or database that reports no schema must still produce the
// unqualified name it did before, not a leading dot.
func TestRenderConnector_NoSchemaStaysUnqualified_Issue2414(t *testing.T) {
	cfg := &GenerateConfig{
		Conn:   &Connection{Driver: DriverPostgres, Alias: "hr"},
		Module: "HR",
		Alias:  "hr",
	}
	schemas := []*TableSchema{{Name: "employees", Columns: []ColumnSchema{{Name: "id", DataType: "bigint"}}}}
	got := renderConnector(cfg, schemas, &GenerateResult{}).MDL

	if !strings.Contains(got, "FROM employees'") {
		t.Errorf("want FROM employees in\n%s", got)
	}
	if strings.Contains(got, "FROM .employees") {
		t.Error("an empty schema produced a leading dot")
	}
}

// The reader's half of the same defect: the schema column of SHOW TABLES has to
// survive into the name the describe step is given, or TableSchema.Schema stays
// empty however the emitter is written.
func TestQualifiedTableName_KeepsTheSchemaColumn_Issue2414(t *testing.T) {
	for _, tc := range []struct {
		name string
		row  []any
		want string
	}{
		{"schema and name", []any{"cdcpublish", "customer"}, "cdcpublish.customer"},
		{"reserved word as a name", []any{"cdcpublish", "order"}, "cdcpublish.order"},
		{"no schema reported", []any{"", "employees"}, "employees"},
		{"name only", []any{"employees"}, "employees"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := qualifiedTableName(tc.row); got != tc.want {
				t.Errorf("qualifiedTableName(%v) = %q, want %q", tc.row, got, tc.want)
			}
		})
	}
}

// The readers must go through that helper, not read the rows themselves: the
// defect was one column dropped in a loop body, and a test on the helper alone
// passes while the loop ignores it. Stubbing this function is what reproduces
// the bug end to end.
func TestTableAddresses_EveryRowKeepsItsSchema_Issue2414(t *testing.T) {
	rows := [][]any{
		{"cdcpublish", "customer"},
		{"cdcpublish", "order"},
		{"main", "ro_probe"},
		{"", "unqualified"},
		{"too_short"},
		{},
	}
	got := tableAddresses(rows)
	want := []string{"cdcpublish.customer", "cdcpublish.order", "main.ro_probe", "unqualified", "too_short"}
	if len(got) != len(want) {
		t.Fatalf("tableAddresses = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("address %d = %q, want %q", i, got[i], want[i])
		}
	}
}

// ReadTableSchema is also reached with a name the user qualified themselves
// (TABLES (cdcpublish.customer)). The schema has to land in Schema and the bare
// name in Name, or the entity is called CdcpublishCustomer.
func TestTableSchemaQualifiedName_Issue2414(t *testing.T) {
	for _, tc := range []struct{ schema, name, want string }{
		{"cdcpublish", "customer", "cdcpublish.customer"},
		{"", "customer", "customer"},
	} {
		ts := &TableSchema{Schema: tc.schema, Name: tc.name}
		if got := ts.QualifiedName(); got != tc.want {
			t.Errorf("QualifiedName(%q, %q) = %q, want %q", tc.schema, tc.name, got, tc.want)
		}
	}
}

// End to end over the whole pipeline — ShowTables, DescribeTable, the readers
// and the emitter — with no PostgreSQL. The driver is DriverPostgres so the
// real query templates run; the database is modernc SQLite with an attached
// information_schema, the harness the IMPORT ... LINK fix established.
//
// This is the test that fails if either half of the fix is reverted: a reader
// that drops the schema column, or an emitter that addresses ts.Name.
func TestGenerateConnector_EndToEnd_QualifiesEverySchema_Issue2414(t *testing.T) {
	conn := openSQLite(t, "lake",
		`ATTACH DATABASE ':memory:' AS information_schema`,
		`CREATE TABLE information_schema.tables (table_schema TEXT, table_name TEXT, table_type TEXT)`,
		`CREATE TABLE information_schema.columns (table_schema TEXT, table_name TEXT,
			column_name TEXT, data_type TEXT, is_nullable TEXT, column_default TEXT, ordinal_position INTEGER)`,
		// Two apps' CDC views, as DuckGraphLake exposes them: a schema per
		// publishing app, and a name that repeats across them.
		`INSERT INTO information_schema.tables VALUES
			('cdcpublish', 'customer', 'VIEW'),
			('cdcpublish', 'order', 'VIEW'),
			('otherapp', 'customer', 'VIEW')`,
		`INSERT INTO information_schema.columns VALUES
			('cdcpublish', 'customer', 'id', 'bigint', 'NO', NULL, 1),
			('cdcpublish', 'customer', 'Name', 'character varying', 'YES', NULL, 2),
			('cdcpublish', 'order', 'id', 'bigint', 'NO', NULL, 1),
			('cdcpublish', 'order', 'OrderNumber', 'bigint', 'YES', NULL, 2),
			('otherapp', 'customer', 'id', 'bigint', 'NO', NULL, 1),
			('otherapp', 'customer', 'Email', 'text', 'YES', NULL, 2)`)
	conn.Driver = DriverPostgres

	res, err := GenerateConnector(context.Background(), &GenerateConfig{
		Conn: conn, Module: "LakeSql", Alias: "lake",
	})
	if err != nil {
		t.Fatal(err)
	}

	for _, want := range []string{
		"FROM cdcpublish.customer", "FROM cdcpublish.order", "FROM otherapp.customer",
	} {
		if !strings.Contains(res.MDL, want) {
			t.Errorf("want %q in the generated connector:\n%s", want, res.MDL)
		}
	}

	// Each schema's columns must be read as that schema's. Before the fix the
	// describe step was given a bare name, so a name two schemas share was
	// refused as ambiguous and the object was dropped from the output entirely
	// — three views in, fewer out, silently (ReadAllTableSchemas skips a table
	// whose describe fails).
	if n := strings.Count(res.MDL, "CREATE NON-PERSISTENT ENTITY"); n != 3 {
		t.Errorf("%d entities, want 3 — a view shared between schemas was dropped:\n%s", n, res.MDL)
	}
	if !strings.Contains(res.MDL, "Email") || !strings.Contains(res.MDL, "OrderNumber") {
		t.Errorf("a schema's own columns did not reach its entity:\n%s", res.MDL)
	}
}
