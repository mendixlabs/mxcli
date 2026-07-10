// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"fmt"

	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/modelsdk/codec"
	"github.com/JordtenBulte-OLC/mxcli/modelsdk/element"
	mmpr "github.com/JordtenBulte-OLC/mxcli/modelsdk/mpr"
	"github.com/JordtenBulte-OLC/mxcli/modelsdk/property"
	"go.mongodb.org/mongo-driver/v2/bson"
)

func init() {
	// Every list in a DatabaseConnection tree uses the typed-array marker 2 (the
	// legacy serializer prefixes each bson.A with int32(2)), both empty and
	// populated. Register the per-child-$Type marker for the populated case and
	// MandatoryListMarkers on the parents for the empty case. Mirrors the legacy
	// serializer in sdk/mpr/writer_dbconnection.go field-for-field.
	codec.RegisterListMarker("DatabaseConnector$DatabaseQuery", 2)
	codec.RegisterListMarker("DatabaseConnector$QueryParameter", 2)
	codec.RegisterListMarker("DatabaseConnector$TableMapping", 2)
	codec.RegisterListMarker("DatabaseConnector$ColumnMapping", 2)
	codec.RegisterTypeDefaults("DatabaseConnector$DatabaseConnection", codec.TypeDefaults{
		MandatoryListMarkers: map[string]int32{"Queries": 2, "AdditionalProperties": 2},
	})
	codec.RegisterTypeDefaults("DatabaseConnector$DatabaseQuery", codec.TypeDefaults{
		MandatoryListMarkers: map[string]int32{"TableMappings": 2, "Parameters": 2},
	})
	codec.RegisterTypeDefaults("DatabaseConnector$TableMapping", codec.TypeDefaults{
		MandatoryListMarkers: map[string]int32{"Columns": 2},
	})
	// A QueryParameter always serializes its TableMapping slot as BSON null.
	codec.RegisterTypeDefaults("DatabaseConnector$QueryParameter", codec.TypeDefaults{
		NullFields: []string{"TableMapping"},
	})
}

// CreateDatabaseConnection inserts a new DatabaseConnector$DatabaseConnection unit.
func (b *Backend) CreateDatabaseConnection(conn *model.DatabaseConnection) error {
	if conn == nil {
		return fmt.Errorf("CreateDatabaseConnection: nil connection")
	}
	if b.writer == nil {
		return fmt.Errorf("CreateDatabaseConnection: not connected for writing")
	}
	if conn.ID == "" {
		conn.ID = model.ID(mmpr.GenerateID())
	}
	g := databaseConnectionToGen(conn)
	contents, err := (&codec.Encoder{}).Encode(g)
	if err != nil {
		return fmt.Errorf("CreateDatabaseConnection: encode: %w", err)
	}
	return b.writer.InsertUnit(string(conn.ID), string(conn.ContainerID),
		"Documents", "DatabaseConnector$DatabaseConnection", contents)
}

// UpdateDatabaseConnection rewrites an existing database connection in place,
// preserving its ID.
func (b *Backend) UpdateDatabaseConnection(conn *model.DatabaseConnection) error {
	if conn == nil {
		return fmt.Errorf("UpdateDatabaseConnection: nil connection")
	}
	if b.writer == nil {
		return fmt.Errorf("UpdateDatabaseConnection: not connected for writing")
	}
	g := databaseConnectionToGen(conn)
	contents, err := (&codec.Encoder{}).Encode(g)
	if err != nil {
		return fmt.Errorf("UpdateDatabaseConnection: encode: %w", err)
	}
	return b.writer.UpdateRawUnit(string(conn.ID), contents)
}

// MoveDatabaseConnection is implemented in move_documents_write.go via the shared
// moveUnit helper.

// DeleteDatabaseConnection removes a database connection unit by ID.
func (b *Backend) DeleteDatabaseConnection(id model.ID) error {
	if b.writer == nil {
		return fmt.Errorf("DeleteDatabaseConnection: not connected for writing")
	}
	return b.writer.DeleteUnit(string(id))
}

// databaseConnectionToGen builds the DatabaseConnection element tree directly with
// the verified storage keys. The gen/databaseconnector setters bind different
// property keys, so this mirrors sdk/mpr.serializeDatabaseConnection field-for-field.
func databaseConnectionToGen(conn *model.DatabaseConnection) element.Element {
	e := newElem("DatabaseConnector$DatabaseConnection", string(conn.ID))
	addStr(e, "Name", conn.Name)
	addStr(e, "DatabaseType", conn.DatabaseType)
	addStr(e, "ConnectionString", conn.ConnectionString)
	addStr(e, "UserName", conn.UserName)
	addStr(e, "Password", conn.Password)
	addStr(e, "Documentation", conn.Documentation)
	addBool(e, "Excluded", conn.Excluded)
	addStr(e, "ExportLevel", "Hidden")

	// ConnectionInput — stores the actual JDBC URL for Studio Pro development.
	connInput := newElem("DatabaseConnector$ConnectionString", "")
	addStr(connInput, "Value", conn.ConnectionInputValue)
	addPart(e, "ConnectionInput", connInput)

	queries := make([]element.Element, 0, len(conn.Queries))
	for _, q := range conn.Queries {
		queries = append(queries, databaseQueryToGen(q))
	}
	addPartList(e, "Queries", queries)

	// AdditionalProperties always serializes as an empty marker-2 list.
	addPartList(e, "AdditionalProperties", nil)
	addStr(e, "LastSelectedQuery", "")
	return e
}

// databaseQueryToGen builds a DatabaseConnector$DatabaseQuery sub-element.
func databaseQueryToGen(q *model.DatabaseQuery) element.Element {
	e := newElem("DatabaseConnector$DatabaseQuery", string(q.ID))
	addStr(e, "Name", q.Name)
	addStr(e, "Query", q.SQL)
	addInt64(e, "QueryType", int64(q.QueryType))

	mappings := make([]element.Element, 0, len(q.TableMappings))
	for _, m := range q.TableMappings {
		mappings = append(mappings, databaseTableMappingToGen(m))
	}
	addPartList(e, "TableMappings", mappings)

	params := make([]element.Element, 0, len(q.Parameters))
	for _, p := range q.Parameters {
		params = append(params, databaseQueryParameterToGen(p))
	}
	addPartList(e, "Parameters", params)
	return e
}

// databaseQueryParameterToGen builds a DatabaseConnector$QueryParameter sub-element.
// The TableMapping slot is emitted as BSON null via the registered NullField default.
func databaseQueryParameterToGen(p *model.DatabaseQueryParameter) element.Element {
	e := newElem("DatabaseConnector$QueryParameter", string(p.ID))
	addStr(e, "ParameterName", p.ParameterName)
	addStr(e, "DatabaseParameterName", "")
	addStr(e, "DefaultValue", p.DefaultValue)
	addBool(e, "EmptyValueBecomesNull", p.EmptyValueBecomesNull)
	addStr(e, "Mode", "Unknown")
	// TableMapping: emitted as null via the registered DatabaseConnector$QueryParameter
	// NullField default (no value set here).

	dataType := p.DataType
	if dataType == "" {
		dataType = "DataTypes$StringType"
	}
	addPart(e, "DataType", newElem(dataType, ""))

	sqlDataType := newElem("DatabaseConnector$SimpleSqlDataType", "")
	addStr(sqlDataType, "DataTypeName", "")
	addPart(e, "SqlDataType", sqlDataType)
	return e
}

// databaseTableMappingToGen builds a DatabaseConnector$TableMapping sub-element.
func databaseTableMappingToGen(m *model.DatabaseTableMapping) element.Element {
	e := newElem("DatabaseConnector$TableMapping", string(m.ID))
	addStr(e, "Entity", m.Entity)
	addStr(e, "TableName", m.TableName)
	columns := make([]element.Element, 0, len(m.Columns))
	for _, c := range m.Columns {
		columns = append(columns, databaseColumnMappingToGen(c))
	}
	addPartList(e, "Columns", columns)
	return e
}

// databaseColumnMappingToGen builds a DatabaseConnector$ColumnMapping sub-element.
func databaseColumnMappingToGen(c *model.DatabaseColumnMapping) element.Element {
	e := newElem("DatabaseConnector$ColumnMapping", string(c.ID))
	addStr(e, "Attribute", c.Attribute)
	addStr(e, "ColumnName", c.ColumnName)
	addPart(e, "SqlDataType", newElem("DatabaseConnector$SimpleSqlDataType", ""))
	return e
}

// addInt64 adds a dirty int64 property (BSON key = name) to a bare element. The
// codec emits the Go int64 as a BSON int64 (matching the legacy writer's
// int64(QueryType)).
func addInt64(b *element.Base, name string, val int64) {
	p := property.NewPrimitive[int64](name, decodeInt64)
	b.AddProperty(p, uint(len(b.Properties())))
	p.Set(val)
}

func decodeInt64(raw bson.Raw, key string) int64 {
	v, err := raw.LookupErr(key)
	if err != nil {
		return 0
	}
	i, _ := v.Int64OK()
	return i
}
