// SPDX-License-Identifier: Apache-2.0

package sql

import (
	"context"
	"strings"
	"testing"
)

// ako/mxcli#2422: an object whose own NAME contains a dot vanished from a
// generated connector with no warning. A DuckGraphLake lake names its raw
// topic tables after the Kafka topic, so four of its nine objects were missing:
//
//	main | local__cdc.CdcPublish.Customer.1.local | VIEW
//
// Three things had to hold for that object to come out right, and none did.
// The address was flattened to a string and split again on every dot, so
// splitTableName read schema `1`, name `local`; DescribeTable then found
// nothing and the reader's `continue // skip objects that fail` discarded the
// error. Even past that, the emitted SQL and the entity name would both have
// been invalid.
//
// Measured on the lake: `select count(*) from main."local__cdc.CdcPublish.Customer.1.local"`
// returns 5 rows, the unquoted form is `syntax error at or near ".1."`, and
// information_schema.columns has the object's 7 columns under that exact
// table_name — so nothing is wrong with the database's side of it.

func TestSplitTableName_QuotedNameKeepsItsDots_Issue2422(t *testing.T) {
	for in, want := range map[string][2]string{
		// The shape this issue is about.
		`main."local__cdc.CdcPublish.Customer.1.local"`: {"main", "local__cdc.CdcPublish.Customer.1.local"},
		`"main"."a.b"`: {"main", "a.b"},
		`"a.b"`:        {"", "a.b"},
		// CONTROLS: every shape that worked before still has to.
		"customers":           {"", "customers"},
		"sales.customers":     {"sales", "customers"},
		`"Sales"."Cust"`:      {"Sales", "Cust"},
		"db.sales.customers":  {"sales", "customers"},
		"[dbo].[Orders]":      {"dbo", "Orders"},
		" sales . customers ": {"sales", "customers"},
	} {
		schema, name := splitTableName(in)
		if schema != want[0] || name != want[1] {
			t.Errorf("splitTableName(%q) = (%q, %q), want (%q, %q)", in, schema, name, want[0], want[1])
		}
	}
}

// The address has to be re-quoted on the way out, or the generated query is a
// syntax error rather than a wrong table.
func TestTableSchemaQualifiedName_QuotesANameThatNeedsIt_Issue2422(t *testing.T) {
	for _, tc := range []struct{ schema, name, want string }{
		{"main", "local__cdc.CdcPublish.Customer.1.local", `main."local__cdc.CdcPublish.Customer.1.local"`},
		{"", "a.b", `"a.b"`},
		{"main", `odd"name`, `main."odd""name"`}, // an embedded quote is doubled
		{"main", "needs space", `main."needs space"`},
		{"main", "1startsdigit", `main."1startsdigit"`},
		// CONTROLS: a plain identifier is left alone, as #2414 established.
		{"cdcpublish", "customer", "cdcpublish.customer"},
		{"", "customer", "customer"},
		{"cdcpublish", "order_history", "cdcpublish.order_history"},
	} {
		ts := &TableSchema{Schema: tc.schema, Name: tc.name}
		if got := ts.QualifiedName(); got != tc.want {
			t.Errorf("QualifiedName(%q, %q) = %q, want %q", tc.schema, tc.name, got, tc.want)
		}
	}
}

// A dot is not a word character in a Mendix name. Left in, it produced
// `CREATE NON-PERSISTENT ENTITY LakeSql.LocalCdc.cdcpublish.customer.1.local`,
// which is not an entity name at all — so fixing only the addressing would
// have traded a silent drop for silently invalid MDL.
func TestTableToEntityName_DotsAreWordSeparators_Issue2422(t *testing.T) {
	for in, want := range map[string]string{
		"local__cdc.CdcPublish.Customer.1.local": "LocalCdcCdcpublishCustomer1Local",
		"my.table":                               "MyTable",
		// CONTROLS: the names everything else generates are unchanged.
		"customer":      "Customer",
		"order_history": "OrderHistory",
		"audit_log":     "AuditLog",
	} {
		if got := TableToEntityName(in); got != want {
			t.Errorf("TableToEntityName(%q) = %q, want %q", in, got, want)
		}
		if strings.ContainsAny(TableToEntityName(in), ". -") {
			t.Errorf("TableToEntityName(%q) = %q, which is not a usable Mendix name", in, TableToEntityName(in))
		}
	}
}

// dottedLake mirrors the lake: the per-app CDC views plus the raw topic table
// whose name is the topic, dots and all.
func dottedLake(t *testing.T) *Connection {
	conn := openSQLite(t, "dotted",
		`ATTACH DATABASE ':memory:' AS information_schema`,
		`CREATE TABLE information_schema.tables (table_schema TEXT, table_name TEXT, table_type TEXT)`,
		`CREATE TABLE information_schema.columns (table_schema TEXT, table_name TEXT,
			column_name TEXT, data_type TEXT, is_nullable TEXT, column_default TEXT, ordinal_position INTEGER)`,
		`INSERT INTO information_schema.tables VALUES
			('cdcpublish', 'customer', 'VIEW'),
			('main', 'local__cdc.CdcPublish.Customer.1.local', 'VIEW')`,
		`INSERT INTO information_schema.columns VALUES
			('cdcpublish', 'customer', 'id', 'bigint', 'NO', NULL, 1),
			('main', 'local__cdc.CdcPublish.Customer.1.local', 'offset', 'bigint', 'NO', NULL, 1),
			('main', 'local__cdc.CdcPublish.Customer.1.local', 'key', 'text', 'YES', NULL, 2)`)
	conn.Driver = DriverPostgres
	return conn
}

func TestGenerateConnector_DottedObjectIsGenerated_Issue2422(t *testing.T) {
	res, err := GenerateConnector(context.Background(), &GenerateConfig{
		Conn: dottedLake(t), Module: "LakeSql", Alias: "lake",
	})
	if err != nil {
		t.Fatal(err)
	}

	if n := strings.Count(res.MDL, "CREATE NON-PERSISTENT ENTITY"); n != 2 {
		t.Errorf("%d entities, want 2 — the dotted object was dropped:\n%s", n, res.MDL)
	}
	if want := `FROM main."local__cdc.CdcPublish.Customer.1.local"`; !strings.Contains(res.MDL, want) {
		t.Errorf("want %q in\n%s", want, res.MDL)
	}
	if !strings.Contains(res.MDL, "ENTITY LakeSql.LocalCdcCdcpublishCustomer1Local (") {
		t.Errorf("the dotted name did not become a usable entity name:\n%s", res.MDL)
	}
	// Its own columns, not another object's.
	if !strings.Contains(res.MDL, "Offset") || !strings.Contains(res.MDL, "Key") {
		t.Errorf("the dotted object's columns are missing:\n%s", res.MDL)
	}
	if res.ViewCount != 2 {
		t.Errorf("ViewCount = %d, want 2", res.ViewCount)
	}
}

// The other half of the defect, and the half worth having whatever is decided
// about dotted names: an object the listing names and the generator cannot read
// must be reported, not dropped. Nothing downstream can notice its absence.
func TestGenerateConnector_UnreadableObjectIsReported_Issue2422(t *testing.T) {
	conn := openSQLite(t, "unreadable",
		`ATTACH DATABASE ':memory:' AS information_schema`,
		`CREATE TABLE information_schema.tables (table_schema TEXT, table_name TEXT, table_type TEXT)`,
		`CREATE TABLE information_schema.columns (table_schema TEXT, table_name TEXT,
			column_name TEXT, data_type TEXT, is_nullable TEXT, column_default TEXT, ordinal_position INTEGER)`,
		`INSERT INTO information_schema.tables VALUES
			('cdcpublish', 'customer', 'VIEW'),
			('cdcpublish', 'vanished', 'VIEW')`,
		`INSERT INTO information_schema.columns VALUES
			('cdcpublish', 'customer', 'id', 'bigint', 'NO', NULL, 1)`)
	conn.Driver = DriverPostgres

	res, err := GenerateConnector(context.Background(), &GenerateConfig{
		Conn: conn, Module: "LakeSql", Alias: "lake",
	})
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(res.MDL, "CREATE NON-PERSISTENT ENTITY"); n != 1 {
		t.Errorf("%d entities, want 1", n)
	}
	if len(res.SkippedObjects) != 1 || !strings.Contains(res.SkippedObjects[0], "vanished") {
		t.Errorf("SkippedObjects = %v, want one naming cdcpublish.vanished", res.SkippedObjects)
	}
}
