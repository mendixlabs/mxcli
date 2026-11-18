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
	for _, row := range rows {
		if addr := qualifiedTableName(row); addr != "" {
			out = append(out, addr)
		}
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

// ReadAllTableSchemas reads schemas for all user tables from the connection.
func ReadAllTableSchemas(ctx context.Context, conn *Connection) ([]*TableSchema, error) {
	result, err := ShowTables(ctx, conn)
	if err != nil {
		return nil, err
	}

	var schemas []*TableSchema
	for _, addr := range tableAddresses(result.Rows) {
		ts, err := ReadTableSchema(ctx, conn, addr, false)
		if err != nil {
			continue // skip tables that fail
		}
		schemas = append(schemas, ts)
	}
	return schemas, nil
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
