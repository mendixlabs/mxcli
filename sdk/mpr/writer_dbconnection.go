// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"fmt"

	"github.com/JordtenBulte-OLC/mxcli/model"
	"go.mongodb.org/mongo-driver/bson"
)

// CreateDatabaseConnection creates a new DatabaseConnector$DatabaseConnection document.
func (w *Writer) CreateDatabaseConnection(conn *model.DatabaseConnection) error {
	if conn.ID == "" {
		conn.ID = model.ID(generateUUID())
	}
	conn.TypeName = "DatabaseConnector$DatabaseConnection"

	contents, err := w.serializeDatabaseConnection(conn)
	if err != nil {
		return fmt.Errorf("failed to serialize database connection: %w", err)
	}

	return w.insertUnit(string(conn.ID), string(conn.ContainerID),
		"Documents", "DatabaseConnector$DatabaseConnection", contents)
}

// UpdateDatabaseConnection updates an existing database connection.
func (w *Writer) UpdateDatabaseConnection(conn *model.DatabaseConnection) error {
	contents, err := w.serializeDatabaseConnection(conn)
	if err != nil {
		return fmt.Errorf("failed to serialize database connection: %w", err)
	}

	return w.updateUnit(string(conn.ID), contents)
}

// MoveDatabaseConnection moves a database connection to a new container (module or folder).
func (w *Writer) MoveDatabaseConnection(conn *model.DatabaseConnection) error {
	return w.moveUnitByID(string(conn.ID), string(conn.ContainerID))
}

// DeleteDatabaseConnection deletes a database connection by ID.
func (w *Writer) DeleteDatabaseConnection(id model.ID) error {
	return w.deleteUnit(string(id))
}

func (w *Writer) serializeDatabaseConnection(conn *model.DatabaseConnection) ([]byte, error) {
	// Build ConnectionInput — stores actual JDBC URL for Studio Pro development
	connInput := bson.D{
		{Key: "$ID", Value: idToBsonBinary(generateUUID())},
		{Key: "$Type", Value: "DatabaseConnector$ConnectionString"},
		{Key: "Value", Value: conn.ConnectionInputValue},
	}

	doc := bson.D{
		{Key: "$ID", Value: idToBsonBinary(string(conn.ID))},
		{Key: "$Type", Value: "DatabaseConnector$DatabaseConnection"},
		{Key: "Name", Value: conn.Name},
		{Key: "DatabaseType", Value: conn.DatabaseType},
		{Key: "ConnectionString", Value: conn.ConnectionString},
		{Key: "UserName", Value: conn.UserName},
		{Key: "Password", Value: conn.Password},
		{Key: "Documentation", Value: conn.Documentation},
		{Key: "Excluded", Value: conn.Excluded},
		{Key: "ExportLevel", Value: "Hidden"},
		{Key: "ConnectionInput", Value: connInput},
	}

	// Serialize Queries
	queries := bson.A{int32(2)} // versioned array prefix
	for _, q := range conn.Queries {
		queries = append(queries, serializeDBQuery(q))
	}
	doc = append(doc, bson.E{Key: "Queries", Value: queries})

	// AdditionalProperties (empty array)
	doc = append(doc, bson.E{Key: "AdditionalProperties", Value: bson.A{int32(2)}})

	// LastSelectedQuery (empty ref)
	doc = append(doc, bson.E{Key: "LastSelectedQuery", Value: ""})

	return marshalUnitIDFirst(doc)
}

func serializeDBQuery(q *model.DatabaseQuery) bson.D {
	id := string(q.ID)
	if id == "" {
		id = generateUUID()
	}

	// TableMappings
	mappings := bson.A{int32(2)}
	for _, m := range q.TableMappings {
		mappings = append(mappings, serializeDBTableMapping(m))
	}

	// Parameters
	params := bson.A{int32(2)}
	for _, p := range q.Parameters {
		params = append(params, serializeDBQueryParameter(p))
	}

	return bson.D{
		{Key: "$ID", Value: idToBsonBinary(id)},
		{Key: "$Type", Value: "DatabaseConnector$DatabaseQuery"},
		{Key: "Name", Value: q.Name},
		{Key: "Query", Value: q.SQL},
		{Key: "QueryType", Value: int64(q.QueryType)},
		{Key: "TableMappings", Value: mappings},
		{Key: "Parameters", Value: params},
	}
}

func serializeDBQueryParameter(p *model.DatabaseQueryParameter) bson.D {
	id := string(p.ID)
	if id == "" {
		id = generateUUID()
	}

	// DataType
	dataType := p.DataType
	if dataType == "" {
		dataType = "DataTypes$StringType"
	}

	return bson.D{
		{Key: "$ID", Value: idToBsonBinary(id)},
		{Key: "$Type", Value: "DatabaseConnector$QueryParameter"},
		{Key: "ParameterName", Value: p.ParameterName},
		{Key: "DatabaseParameterName", Value: ""},
		{Key: "DefaultValue", Value: p.DefaultValue},
		{Key: "EmptyValueBecomesNull", Value: p.EmptyValueBecomesNull},
		{Key: "Mode", Value: "Unknown"},
		{Key: "TableMapping", Value: nil},
		{Key: "DataType", Value: bson.D{
			{Key: "$ID", Value: idToBsonBinary(generateUUID())},
			{Key: "$Type", Value: dataType},
		}},
		{Key: "SqlDataType", Value: bson.D{
			{Key: "$ID", Value: idToBsonBinary(generateUUID())},
			{Key: "$Type", Value: "DatabaseConnector$SimpleSqlDataType"},
			{Key: "DataTypeName", Value: ""},
		}},
	}
}

func serializeDBTableMapping(m *model.DatabaseTableMapping) bson.D {
	id := string(m.ID)
	if id == "" {
		id = generateUUID()
	}

	// Columns
	columns := bson.A{int32(2)}
	for _, c := range m.Columns {
		columns = append(columns, serializeDBColumnMapping(c))
	}

	return bson.D{
		{Key: "$ID", Value: idToBsonBinary(id)},
		{Key: "$Type", Value: "DatabaseConnector$TableMapping"},
		{Key: "Entity", Value: m.Entity},
		{Key: "TableName", Value: m.TableName},
		{Key: "Columns", Value: columns},
	}
}

func serializeDBColumnMapping(c *model.DatabaseColumnMapping) bson.D {
	id := string(c.ID)
	if id == "" {
		id = generateUUID()
	}

	// SqlDataType — use SimpleSqlDataType as default
	cDoc := bson.D{
		{Key: "$ID", Value: idToBsonBinary(id)},
		{Key: "$Type", Value: "DatabaseConnector$ColumnMapping"},
		{Key: "Attribute", Value: c.Attribute},
		{Key: "ColumnName", Value: c.ColumnName},
	}

	cDoc = append(cDoc, bson.E{Key: "SqlDataType", Value: bson.D{
		{Key: "$ID", Value: idToBsonBinary(generateUUID())},
		{Key: "$Type", Value: "DatabaseConnector$SimpleSqlDataType"},
	}})

	return cDoc
}
