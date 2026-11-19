// SPDX-License-Identifier: Apache-2.0

package sql

import (
	"context"
	"fmt"
	"strings"
)

// ColumnSchema describes a single column in a table or view.
type ColumnSchema struct {
	Name     string
	DataType string
	Nullable bool
	IsPK     bool
}

// TableSchema describes a table or view with its columns.
type TableSchema struct {
	Schema  string
	Name    string
	Columns []ColumnSchema
	IsView  bool
}

// QualifiedName is how the table must be addressed in a generated query: the
// schema it was discovered in, when one is known. A database reports no schema
// for some drivers, and there the bare name is the address (ako/mxcli#2414).
func (t *TableSchema) QualifiedName() string {
	if t.Schema == "" {
		return t.Name
	}
	return t.Schema + "." + t.Name
}

// tableAddresses is the address of every object a SHOW TABLES / SHOW VIEWS
// result names. The readers go through it rather than reading the rows
// themselves, so a schema cannot be dropped again without changing a function
// a test covers (ako/mxcli#2414).
func tableAddresses(rows [][]any) []string {
	var out []string
	for _, o := range discoveredObjects(rows) {
		out = append(out, o.address)
	}
	return out
}

// discoveredObject is one row of a SHOW TABLES listing: where the object is and
// what it is. Every driver's listing returns table_type as its third column,
// and reading it is what keeps a view out of the set of tables -- the
// conflation that made one view generate two entities (ako/mxcli#2415).
type discoveredObject struct {
	address string
	isView  bool
}

func discoveredObjects(rows [][]any) []discoveredObject {
	var out []discoveredObject
	for _, row := range rows {
		addr := qualifiedTableName(row)
		if addr == "" {
			continue
		}
		isView := false
		if len(row) > 2 {
			isView = strings.EqualFold(strings.TrimSpace(fmt.Sprintf("%v", row[2])), "VIEW")
		}
		out = append(out, discoveredObject{address: addr, isView: isView})
	}
	return out
}

// qualifiedTableName is the address in a SHOW TABLES / SHOW VIEWS row, whose
// columns are (table_schema, table_name). The schema is part of the address,
// so dropping it leaves a name that cannot be selected from (ako/mxcli#2414).
func qualifiedTableName(row []any) string {
	if len(row) < 2 {
		if len(row) == 1 {
			return fmt.Sprintf("%v", row[0])
		}
		return ""
	}
	name := fmt.Sprintf("%v", row[1])
	schema := fmt.Sprintf("%v", row[0])
	if schema == "" {
		return name
	}
	return schema + "." + name
}

// pkQuery returns the query to find primary key columns for a given table.
var pkQuery = map[DriverName]string{
	DriverPostgres: `SELECT a.attname
FROM pg_index i
JOIN pg_attribute a ON a.attrelid = i.indrelid AND a.attnum = ANY(i.indkey)
WHERE i.indrelid = '%s'::regclass AND i.indisprimary`,
	DriverOracle: `SELECT cols.column_name
FROM all_constraints c
JOIN all_cons_columns cols ON c.constraint_name = cols.constraint_name AND c.owner = cols.owner
WHERE c.constraint_type = 'P' AND cols.table_name = UPPER('%s')`,
	DriverSQLServer: `SELECT c.COLUMN_NAME
FROM INFORMATION_SCHEMA.TABLE_CONSTRAINTS tc
JOIN INFORMATION_SCHEMA.KEY_COLUMN_USAGE c ON c.CONSTRAINT_NAME = tc.CONSTRAINT_NAME
WHERE tc.CONSTRAINT_TYPE = 'PRIMARY KEY' AND tc.TABLE_NAME = '%s'`,
}

// ReadTableSchema reads the column metadata and primary key info for a single table.
func ReadTableSchema(ctx context.Context, conn *Connection, tableName string, isView bool) (*TableSchema, error) {
	// Get columns
	colResult, err := DescribeTable(ctx, conn, tableName)
	if err != nil {
		return nil, fmt.Errorf("describe %s: %w", tableName, err)
	}

	// Get primary keys
	pkCols := make(map[string]bool)
	if !isView {
		if tmpl, ok := pkQuery[conn.Driver]; ok {
			q := fmt.Sprintf(tmpl, tableName)
			pkResult, err := Execute(ctx, conn, q)
			if err == nil {
				for _, row := range pkResult.Rows {
					if len(row) > 0 {
						pkCols[fmt.Sprintf("%v", row[0])] = true
					}
				}
			}
			// Ignore PK query errors — some tables may not have PKs
		}
	}

	// tableName may be qualified, either because discovery qualified it or
	// because the user wrote TABLES (sales.customers). The schema belongs in
	// Schema and the bare name in Name: the name is what every Mendix entity
	// and query is called after (ako/mxcli#2414).
	schemaName, bareName := splitTableName(tableName)
	if schemaName == "" {
		// A name the user wrote bare still has a schema, and the generated
		// query has to address it. DescribeTable has already refused a name
		// more than one schema holds, so there is at most one here. Resolving
		// it is also what makes `TABLES (customer)` and `VIEWS (sales.customer)`
		// the same object rather than two entities of one name (ako/mxcli#2415).
		if holders := schemasHolding(ctx, conn, bareName); len(holders) == 1 {
			schemaName = holders[0]
		}
	}
	schema := &TableSchema{
		Schema: schemaName,
		Name:   bareName,
		IsView: isView,
	}

	// colResult columns: column_name, data_type, nullable, column_default
	for _, row := range colResult.Rows {
		if len(row) < 3 {
			continue
		}
		name := fmt.Sprintf("%v", row[0])
		dataType := fmt.Sprintf("%v", row[1])
		nullable := strings.EqualFold(fmt.Sprintf("%v", row[2]), "YES")

		schema.Columns = append(schema.Columns, ColumnSchema{
			Name:     name,
			DataType: dataType,
			Nullable: nullable,
			IsPK:     pkCols[name],
		})
	}

	return schema, nil
}

// ReadAllSchemas reads every object the connection's table listing names --
// base tables and views alike -- each marked with what it is. This is what a
// GENERATE CONNECTOR with no TABLES/VIEWS clause enumerates: filtering views
// out here would empty the output on a database whose objects are all views,
// which is what a CDC lake is.
//
// On SQL Server the listing itself is restricted to base tables, so no views
// come back; name them in a VIEWS clause there.
func ReadAllSchemas(ctx context.Context, conn *Connection) ([]*TableSchema, error) {
	return readListed(ctx, conn, false)
}

// readListed reads the listed objects, optionally only the base tables.
func readListed(ctx context.Context, conn *Connection, tablesOnly bool) ([]*TableSchema, error) {
	result, err := ShowTables(ctx, conn)
	if err != nil {
		return nil, err
	}
	var schemas []*TableSchema
	for _, o := range discoveredObjects(result.Rows) {
		if tablesOnly && o.isView {
			continue
		}
		ts, err := ReadTableSchema(ctx, conn, o.address, o.isView)
		if err != nil {
			continue // skip objects that fail
		}
		ts.IsView = o.isView
		schemas = append(schemas, ts)
	}
	return schemas, nil
}

// ReadAllTableSchemas reads schemas for the connection's base tables. A view is
// not a table: handing one back here is what let a view named in a VIEWS clause
// be generated twice (ako/mxcli#2415). Use ReadAllSchemas for everything.
func ReadAllTableSchemas(ctx context.Context, conn *Connection) ([]*TableSchema, error) {
	return readListed(ctx, conn, true)
}

// ReadAllViewSchemas reads schemas for all user views from the connection.
func ReadAllViewSchemas(ctx context.Context, conn *Connection) ([]*TableSchema, error) {
	result, err := ShowViews(ctx, conn)
	if err != nil {
		return nil, err
	}

	var schemas []*TableSchema
	for _, addr := range tableAddresses(result.Rows) {
		ts, err := ReadTableSchema(ctx, conn, addr, true)
		if err != nil {
			continue // skip views that fail
		}
		ts.IsView = true
		schemas = append(schemas, ts)
	}
	return schemas, nil
}
