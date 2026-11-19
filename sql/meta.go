// SPDX-License-Identifier: Apache-2.0

package sql

import (
	"context"
	"fmt"
	"strings"
)

// tableQuery maps driver to the query that lists user tables.
var tableQuery = map[DriverName]string{
	DriverPostgres: `SELECT table_schema, table_name, table_type
FROM information_schema.tables
WHERE table_schema NOT IN ('pg_catalog', 'information_schema')
ORDER BY table_schema, table_name`,
	DriverOracle: `SELECT owner AS table_schema, table_name, 'BASE TABLE' AS table_type
FROM all_tables
WHERE owner NOT IN ('SYS','SYSTEM','XDB','CTXSYS','MDSYS','OLAPSYS','WMSYS','DBSNMP','APPQOSSYS','DBSFWUSER','OUTLN','LBACSYS','ORDDATA','ORDSYS','GSMADMIN_INTERNAL')
ORDER BY owner, table_name`,
	DriverSQLServer: `SELECT TABLE_SCHEMA AS table_schema, TABLE_NAME AS table_name, TABLE_TYPE AS table_type
FROM INFORMATION_SCHEMA.TABLES
WHERE TABLE_TYPE = 'BASE TABLE'
ORDER BY TABLE_SCHEMA, TABLE_NAME`,
}

// describeQuery maps driver to the query that describes a table's columns.
// The placeholder %s is replaced with the WHERE filter for the table.
var describeQuery = map[DriverName]string{
	DriverPostgres: `SELECT column_name, data_type,
  CASE WHEN is_nullable = 'YES' THEN 'YES' ELSE 'NO' END AS nullable,
  column_default
FROM information_schema.columns
WHERE %s
ORDER BY ordinal_position`,
	DriverOracle: `SELECT column_name, data_type, data_length, nullable
FROM all_tab_columns
WHERE %s
ORDER BY column_id`,
	DriverSQLServer: `SELECT COLUMN_NAME AS column_name, DATA_TYPE AS data_type,
  IS_NULLABLE AS nullable,
  COLUMN_DEFAULT AS column_default
FROM INFORMATION_SCHEMA.COLUMNS
WHERE %s
ORDER BY ORDINAL_POSITION`,
}

// viewQuery maps driver to the query that lists user views.
var viewQuery = map[DriverName]string{
	DriverPostgres: `SELECT schemaname AS view_schema, viewname AS view_name, definition
FROM pg_views
WHERE schemaname NOT IN ('pg_catalog', 'information_schema')
ORDER BY schemaname, viewname`,
	DriverOracle: `SELECT owner AS view_schema, view_name, text AS definition
FROM all_views
WHERE owner NOT IN ('SYS','SYSTEM','XDB','CTXSYS','MDSYS','OLAPSYS','WMSYS','DBSNMP','APPQOSSYS','DBSFWUSER','OUTLN','LBACSYS','ORDDATA','ORDSYS','GSMADMIN_INTERNAL')
ORDER BY owner, view_name`,
	DriverSQLServer: `SELECT s.name AS view_schema, v.name AS view_name, m.definition
FROM sys.views v
JOIN sys.schemas s ON s.schema_id = v.schema_id
LEFT JOIN sys.sql_modules m ON m.object_id = v.object_id
ORDER BY s.name, v.name`,
}

// functionQuery maps driver to the query that lists user functions and procedures.
var functionQuery = map[DriverName]string{
	DriverPostgres: `SELECT n.nspname AS schema, p.proname AS name,
  CASE p.prokind WHEN 'f' THEN 'function' WHEN 'p' THEN 'procedure' WHEN 'a' THEN 'aggregate' WHEN 'w' THEN 'window' END AS kind,
  pg_get_function_result(p.oid) AS return_type,
  pg_get_function_arguments(p.oid) AS arguments
FROM pg_proc p
JOIN pg_namespace n ON n.oid = p.pronamespace
WHERE n.nspname NOT IN ('pg_catalog', 'information_schema')
ORDER BY n.nspname, p.proname`,
	DriverOracle: `SELECT owner AS schema, object_name AS name, object_type AS kind
FROM all_procedures
WHERE owner NOT IN ('SYS','SYSTEM','XDB','CTXSYS','MDSYS','OLAPSYS','WMSYS','DBSNMP','APPQOSSYS','DBSFWUSER','OUTLN','LBACSYS','ORDDATA','ORDSYS','GSMADMIN_INTERNAL')
  AND object_type IN ('FUNCTION','PROCEDURE','PACKAGE')
ORDER BY owner, object_type, object_name`,
	DriverSQLServer: `SELECT s.name AS [schema], o.name, o.type_desc AS kind,
  m.definition
FROM sys.objects o
JOIN sys.schemas s ON s.schema_id = o.schema_id
LEFT JOIN sys.sql_modules m ON m.object_id = o.object_id
WHERE o.type IN ('FN','IF','TF','P','AF')
  AND o.is_ms_shipped = 0
ORDER BY s.name, o.type_desc, o.name`,
}

// ShowTables returns a list of tables for the given connection.
func ShowTables(ctx context.Context, conn *Connection) (*QueryResult, error) {
	query, ok := tableQuery[conn.Driver]
	if !ok {
		return nil, fmt.Errorf("SHOW TABLES not supported for driver %s", conn.Driver)
	}
	return Execute(ctx, conn, query)
}

// DescribeTable returns the column definitions for a table or view, named
// as `table` or `schema.table` — the two columns --tables prints
// (ako/mxcli#2361). A table that is not found is an error, not an empty
// column list.
func DescribeTable(ctx context.Context, conn *Connection, table string) (*QueryResult, error) {
	tmpl, ok := describeQuery[conn.Driver]
	if !ok {
		return nil, fmt.Errorf("DESCRIBE TABLE not supported for driver %s", conn.Driver)
	}

	if err := refuseAmbiguousTable(ctx, conn, table); err != nil {
		return nil, err
	}
	var filter string
	switch conn.Driver {
	case DriverOracle:
		filter = oracleTableFilter(table)
	case DriverSQLServer:
		filter = columnsFilter(table, "TABLE_SCHEMA", "TABLE_NAME")
	default:
		filter = columnsFilter(table, "table_schema", "table_name")
	}
	res, err := Execute(ctx, conn, fmt.Sprintf(tmpl, filter))
	if err != nil {
		return nil, err
	}
	if len(res.Rows) == 0 {
		return nil, fmt.Errorf("table or view %s not found: name it as --tables prints it, table_name or table_schema.table_name", table)
	}
	return res, nil
}

// refuseAmbiguousTable refuses an unqualified name that several schemas
// hold: their columns would be listed as one table's.
func refuseAmbiguousTable(ctx context.Context, conn *Connection, table string) error {
	schema, name := splitTableName(table)
	if schema != "" {
		return nil
	}
	holders := schemasHolding(ctx, conn, name)
	if len(holders) < 2 {
		return nil
	}
	var names []string
	for _, h := range holders {
		names = append(names, h+"."+name)
	}
	return fmt.Errorf("%s is in more than one schema: name one of %s", name, strings.Join(names, ", "))
}

// schemasHolding is every schema that has a table or view of this (bare) name,
// in order. Empty when the lookup is not possible or finds nothing, so a caller
// must treat "no answer" and "not there" alike: DescribeTable is the one that
// reports a missing object.
func schemasHolding(ctx context.Context, conn *Connection, name string) []string {
	var q string
	switch conn.Driver {
	case DriverOracle:
		q = "SELECT DISTINCT owner FROM all_tab_columns WHERE table_name = UPPER(" + sqlString(name) + ") ORDER BY 1"
	case DriverSQLServer:
		q = "SELECT DISTINCT TABLE_SCHEMA FROM INFORMATION_SCHEMA.COLUMNS WHERE TABLE_NAME = " + sqlString(name) + " ORDER BY 1"
	default:
		q = "SELECT DISTINCT table_schema FROM information_schema.columns WHERE table_name = " + sqlString(name) + " ORDER BY 1"
	}
	res, err := Execute(ctx, conn, q)
	if err != nil {
		return nil
	}
	var out []string
	for _, r := range res.Rows {
		if len(r) > 0 {
			out = append(out, fmt.Sprintf("%v", r[0]))
		}
	}
	return out
}

// columnsFilter is the information_schema filter for table, qualified by its
// schema when one is given.
func columnsFilter(table, schemaCol, nameCol string) string {
	schema, name := splitTableName(table)
	f := fmt.Sprintf("%s = %s", nameCol, sqlString(name))
	if schema != "" {
		f = fmt.Sprintf("%s = %s AND %s", schemaCol, sqlString(schema), f)
	}
	return f
}

// splitTableName splits `schema.table` (a leading database part is dropped)
// and removes identifier quotes.
func splitTableName(table string) (schema, name string) {
	parts := splitQualifiedIdent(table)
	if len(parts) == 1 {
		return "", parts[0]
	}
	return parts[len(parts)-2], parts[len(parts)-1]
}

// splitQualifiedIdent splits a dotted identifier on the dots that are not
// inside quotes, and unquotes each part. A quoted part may contain dots of its
// own -- a table named after a Kafka topic does, `main."local__cdc.App.E.1.ns"`
// -- and splitting those made the object unaddressable, so it was dropped from
// a generated connector without a word (ako/mxcli#2422).
func splitQualifiedIdent(s string) []string {
	var (
		parts  []string
		cur    strings.Builder
		closer rune // 0 when not inside quotes
	)
	runes := []rune(s)
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		if closer != 0 {
			if r != closer {
				cur.WriteRune(r)
				continue
			}
			// "" inside a quoted name is one literal quote, not the end of it.
			if r == '"' && i+1 < len(runes) && runes[i+1] == '"' {
				cur.WriteRune('"')
				i++
				continue
			}
			closer = 0
			continue
		}
		switch r {
		case '"':
			closer = '"'
		case '`':
			closer = '`'
		case '[':
			closer = ']'
		case '.':
			parts = append(parts, strings.TrimSpace(cur.String()))
			cur.Reset()
		default:
			cur.WriteRune(r)
		}
	}
	return append(parts, strings.TrimSpace(cur.String()))
}

// quoteIdent quotes an identifier that is not a bare word, doubling any quote
// inside it. A bare word is left alone, reserved or not: `cdcpublish.order`
// parses, which is what ako/mxcli#2414 measured, and quoting everything would
// change every query this tool has ever generated.
//
// The quote is the double quote, which PostgreSQL and Oracle take and SQL
// Server takes with QUOTED_IDENTIFIER on -- the default over JDBC, which is how
// a Database Connector reaches it.
func quoteIdent(ident string) string {
	if !needsQuoting(ident) {
		return ident
	}
	return `"` + strings.ReplaceAll(ident, `"`, `""`) + `"`
}

func needsQuoting(ident string) bool {
	for i, r := range ident {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r == '_':
		case r >= '0' && r <= '9' && i > 0:
		default:
			return true
		}
	}
	return false
}

func sqlString(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }

// oracleTableFilter builds a WHERE clause for Oracle's all_tab_columns.
// Supports both "TABLE" and "SCHEMA.TABLE" formats.
func oracleTableFilter(table string) string {
	schema, name := splitTableName(table)
	if schema != "" {
		return fmt.Sprintf("owner = %s AND table_name = %s", sqlString(strings.ToUpper(schema)), sqlString(strings.ToUpper(name)))
	}
	return fmt.Sprintf("table_name = UPPER(%s)", sqlString(name))
}

// ShowViews returns a list of views for the given connection.
func ShowViews(ctx context.Context, conn *Connection) (*QueryResult, error) {
	query, ok := viewQuery[conn.Driver]
	if !ok {
		return nil, fmt.Errorf("SHOW VIEWS not supported for driver %s", conn.Driver)
	}
	return Execute(ctx, conn, query)
}

// ShowFunctions returns a list of functions and procedures for the given connection.
func ShowFunctions(ctx context.Context, conn *Connection) (*QueryResult, error) {
	query, ok := functionQuery[conn.Driver]
	if !ok {
		return nil, fmt.Errorf("SHOW FUNCTIONS not supported for driver %s", conn.Driver)
	}
	return Execute(ctx, conn, query)
}
