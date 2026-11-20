// SPDX-License-Identifier: Apache-2.0

package sql

import (
	"context"
	"fmt"
	"strings"
)

// GenerateConfig configures the connector generation.
type GenerateConfig struct {
	Conn   *Connection
	Module string   // target Mendix module
	Alias  string   // connection alias (for naming constants/connection)
	Tables []string // nil = all tables
	Views  []string // nil = no views
	DSN    string   // original DSN for JDBC conversion
}

// GenerateResult holds the generated MDL and statistics.
type GenerateResult struct {
	MDL           string // complete MDL (constants + entities + database connection)
	ExecutableMDL string // constants + entities only (parseable/executable by mxcli)
	ConnectionMDL string // DATABASE CONNECTION definition (reference output)
	TableCount    int
	ViewCount     int
	SkippedCols   []string // "table.column: type" for unmappable columns
	// SkippedObjects is every listed object the generator could not read, as
	// "address: reason". An object the listing names and the output omits is
	// invisible downstream, so it is reported rather than dropped
	// (ako/mxcli#2422).
	SkippedObjects []string
}

// GenerateConnector discovers schema and generates complete MDL for a Mendix Database Connector.
func GenerateConnector(ctx context.Context, cfg *GenerateConfig) (*GenerateResult, error) {
	var schemas []*TableSchema
	result := &GenerateResult{}

	// Either clause scopes the whole command. Treating an absent TABLES as
	// "every table" while VIEWS named a few generated the lot and then the
	// named views on top, so a view came out twice under one entity name
	// (ako/mxcli#2415).
	if cfg.Tables == nil && cfg.Views == nil {
		var err error
		schemas, result.SkippedObjects, err = readListed(ctx, cfg.Conn, false)
		if err != nil {
			return nil, fmt.Errorf("reading tables: %w", err)
		}
	} else {
		for _, t := range cfg.Tables {
			ts, err := ReadTableSchema(ctx, cfg.Conn, t, false)
			if err != nil {
				return nil, fmt.Errorf("table %s: %w", t, err)
			}
			schemas = append(schemas, ts)
		}
		for _, v := range cfg.Views {
			ts, err := ReadTableSchema(ctx, cfg.Conn, v, true)
			if err != nil {
				return nil, fmt.Errorf("view %s: %w", v, err)
			}
			ts.IsView = true
			schemas = append(schemas, ts)
		}
	}

	// One object, one entity. Naming it in both clauses, or under two
	// spellings of its address, is a request for it once -- two entities of one
	// name is not a model Mendix can load.
	schemas = dedupeByAddress(schemas)
	for _, ts := range schemas {
		if ts.IsView {
			result.ViewCount++
		} else {
			result.TableCount++
		}
	}

	if len(schemas) == 0 {
		return nil, fmt.Errorf("no tables or views found")
	}

	renderConnector(cfg, schemas, result)
	return result, nil
}

// renderConnector writes the MDL for schemas that have already been discovered.
// It is separate from discovery so the emitted text — in particular how a table
// is addressed in a generated query (ako/mxcli#2414) — can be tested without a
// database.
func renderConnector(cfg *GenerateConfig, schemas []*TableSchema, result *GenerateResult) *GenerateResult {
	// Generate MDL in two parts: executable (constants + entities) and connection reference
	var exec strings.Builder // constants + entities (parseable by mxcli)
	var conn strings.Builder // DATABASE CONNECTION definition (reference)
	aliasTitle := titleCase(cfg.Alias)

	// Connection name: e.g. "F1Database" from alias "f1"
	connName := aliasTitle + "Database"

	// Constants use Mendix Database Connector naming convention:
	// {ConnectionName}_DBSource, {ConnectionName}_DBUsername, {ConnectionName}_DBPassword
	constSource := connName + "_DBSource"
	constUsername := connName + "_DBUsername"
	constPassword := connName + "_DBPassword"

	// JDBC URL for the connection string constant
	jdbcURL, _ := GoDriverDSNToJDBC(cfg.Conn.Driver, cfg.DSN)
	if jdbcURL == "" {
		jdbcURL = cfg.DSN
	}

	dbType := driverToMendixType(cfg.Conn.Driver)

	fmt.Fprintf(&exec, "-- Constants for %s connection (names must match Database Connector convention)\n", connName)
	fmt.Fprintf(&exec, "/** JDBC connection string for %s */\n", connName)
	fmt.Fprintf(&exec, "CREATE CONSTANT %s.%s ( Type: String, DefaultValue: '%s' );\n\n", cfg.Module, constSource, escapeString(jdbcURL))

	fmt.Fprintf(&exec, "/** Database username for %s */\n", connName)
	fmt.Fprintf(&exec, "CREATE CONSTANT %s.%s ( Type: String, DefaultValue: '' );\n\n", cfg.Module, constUsername)

	fmt.Fprintf(&exec, "/** Database password for %s */\n", connName)
	fmt.Fprintf(&exec, "CREATE CONSTANT %s.%s ( Type: String, DefaultValue: '' );\n\n", cfg.Module, constPassword)

	// Non-persistent entities
	type entityInfo struct {
		entityName string
		columns    []ColumnSchema
		mappings   []columnMapping
		tableName  string
	}
	var entities []entityInfo

	for _, ts := range schemas {
		entityName := TableToEntityName(ts.Name)
		var mappings []columnMapping

		fmt.Fprintf(&exec, "-- Non-persistent entity for %s\n", ts.Name)
		fmt.Fprintf(&exec, "CREATE NON-PERSISTENT ENTITY %s.%s (\n", cfg.Module, entityName)

		first := true
		for _, col := range ts.Columns {
			mt := MapSQLType(cfg.Conn.Driver, col.DataType, col.IsPK)
			if mt == nil {
				result.SkippedCols = append(result.SkippedCols,
					fmt.Sprintf("%s.%s: %s", ts.Name, col.Name, col.DataType))
				continue
			}
			attrName := ColumnToAttributeName(col.Name)
			if !first {
				fmt.Fprintf(&exec, ",\n")
			}
			fmt.Fprintf(&exec, "  %s: %s", attrName, mt.String())
			first = false

			mappings = append(mappings, columnMapping{
				colName:  col.Name,
				attrName: attrName,
			})
		}
		fmt.Fprintf(&exec, "\n);\n\n")

		entities = append(entities, entityInfo{
			entityName: entityName,
			columns:    ts.Columns,
			mappings:   mappings,
			// The address, not the Mendix name: a query that selects FROM the
			// bare name fails on any database that keeps the object outside the
			// connection's search path (ako/mxcli#2414).
			tableName: ts.QualifiedName(),
		})
	}

	// Database Connection with queries (reference output — not yet executable by mxcli)
	fmt.Fprintf(&conn, "CREATE DATABASE CONNECTION %s.%s\n", cfg.Module, connName)
	fmt.Fprintf(&conn, "TYPE '%s'\n", dbType)
	fmt.Fprintf(&conn, "CONNECTION STRING @%s.%s\n", cfg.Module, constSource)
	fmt.Fprintf(&conn, "USERNAME @%s.%s\n", cfg.Module, constUsername)
	fmt.Fprintf(&conn, "PASSWORD @%s.%s\n", cfg.Module, constPassword)
	fmt.Fprintf(&conn, "BEGIN\n")

	for _, ei := range entities {
		queryName := "GetAll" + ei.entityName + "s"

		// Build SELECT with original column names
		var cols []string
		for _, m := range ei.mappings {
			cols = append(cols, m.colName)
		}
		selectSQL := fmt.Sprintf("SELECT %s FROM %s", strings.Join(cols, ", "), ei.tableName)

		fmt.Fprintf(&conn, "  QUERY %s\n", queryName)
		fmt.Fprintf(&conn, "    SQL '%s'\n", escapeString(selectSQL))
		fmt.Fprintf(&conn, "    RETURNS %s.%s\n", cfg.Module, ei.entityName)

		// MAP clause if any column names differ from attribute names
		needsMap := false
		for _, m := range ei.mappings {
			if !strings.EqualFold(m.colName, m.attrName) {
				needsMap = true
				break
			}
		}
		if needsMap {
			fmt.Fprintf(&conn, "    MAP (\n")
			for i, m := range ei.mappings {
				sep := ","
				if i == len(ei.mappings)-1 {
					sep = ""
				}
				fmt.Fprintf(&conn, "      %s AS %s%s\n", m.colName, m.attrName, sep)
			}
			fmt.Fprintf(&conn, "    )")
		}
		fmt.Fprintf(&conn, ";\n\n")
	}

	fmt.Fprintf(&conn, "END;\n")

	result.ExecutableMDL = exec.String()
	result.ConnectionMDL = conn.String()
	result.MDL = exec.String() + "-- Database Connection (create in Studio Pro or configure via Database Connector module)\n" + conn.String()
	return result
}

type columnMapping struct {
	colName  string
	attrName string
}

// driverToMendixType maps our driver to the Mendix Database Connector TYPE string.
// dedupeByAddress keeps the first reading of each object, comparing addresses
// without case: the clause spells a name as the user typed it, the listing as
// the database stores it.
func dedupeByAddress(schemas []*TableSchema) []*TableSchema {
	seen := make(map[string]bool, len(schemas))
	out := schemas[:0:0]
	for _, ts := range schemas {
		key := strings.ToLower(ts.QualifiedName())
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, ts)
	}
	return out
}

func driverToMendixType(driver DriverName) string {
	switch driver {
	case DriverPostgres:
		return "PostgreSQL"
	case DriverOracle:
		return "Oracle"
	case DriverSQLServer:
		return "MSSQL"
	default:
		return string(driver)
	}
}

// titleCase capitalizes the first letter.
func titleCase(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// escapeString escapes single quotes for MDL string literals.
func escapeString(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}
