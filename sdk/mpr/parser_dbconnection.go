// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"fmt"

	"github.com/JordtenBulte-OLC/mxcli/model"
	"go.mongodb.org/mongo-driver/bson"
)

// parseDBConnection parses a DatabaseConnector$DatabaseConnection from BSON.
func (r *Reader) parseDBConnection(unitID, containerID string, contents []byte) (*model.DatabaseConnection, error) {
	contents, err := r.resolveContents(unitID, contents)
	if err != nil {
		return nil, err
	}

	var raw map[string]any
	if err := bson.Unmarshal(contents, &raw); err != nil {
		return nil, fmt.Errorf("failed to unmarshal BSON: %w", err)
	}

	conn := &model.DatabaseConnection{}
	conn.ID = model.ID(unitID)
	conn.TypeName = "DatabaseConnector$DatabaseConnection"
	conn.ContainerID = model.ID(containerID)

	conn.Name = extractString(raw["Name"])
	conn.DatabaseType = extractString(raw["DatabaseType"])
	conn.ConnectionString = extractString(raw["ConnectionString"])
	conn.UserName = extractString(raw["UserName"])
	conn.Password = extractString(raw["Password"])
	conn.Documentation = extractString(raw["Documentation"])
	conn.Excluded = extractBool(raw["Excluded"], false)
	conn.ExportLevel = extractString(raw["ExportLevel"])

	// Parse ConnectionInput.Value (actual JDBC URL for Studio Pro)
	if ci := extractBsonMap(raw["ConnectionInput"]); ci != nil {
		conn.ConnectionInputValue = extractString(ci["Value"])
	}

	// Parse Queries
	queries := extractBsonArray(raw["Queries"])
	for _, q := range queries {
		if qMap := extractBsonMap(q); qMap != nil {
			conn.Queries = append(conn.Queries, parseDBQuery(qMap))
		}
	}

	return conn, nil
}

func parseDBQuery(raw map[string]any) *model.DatabaseQuery {
	q := &model.DatabaseQuery{}
	q.ID = model.ID(extractBsonID(raw["$ID"]))
	q.TypeName = extractString(raw["$Type"])
	q.Name = extractString(raw["Name"])
	q.SQL = extractString(raw["Query"])
	q.QueryType = extractInt(raw["QueryType"])

	// Parse TableMappings
	mappings := extractBsonArray(raw["TableMappings"])
	for _, m := range mappings {
		if mMap := extractBsonMap(m); mMap != nil {
			q.TableMappings = append(q.TableMappings, parseDBTableMapping(mMap))
		}
	}

	// Parse Parameters
	params := extractBsonArray(raw["Parameters"])
	for _, p := range params {
		if pMap := extractBsonMap(p); pMap != nil {
			q.Parameters = append(q.Parameters, parseDBQueryParameter(pMap))
		}
	}

	return q
}

func parseDBQueryParameter(raw map[string]any) *model.DatabaseQueryParameter {
	p := &model.DatabaseQueryParameter{}
	p.ID = model.ID(extractBsonID(raw["$ID"]))
	p.TypeName = extractString(raw["$Type"])
	p.ParameterName = extractString(raw["ParameterName"])
	p.DefaultValue = extractString(raw["DefaultValue"])
	p.EmptyValueBecomesNull = extractBool(raw["EmptyValueBecomesNull"], false)

	// DataType is a nested object like {"$Type": "DataTypes$IntegerType", "$ID": "..."}
	if dt := extractBsonMap(raw["DataType"]); dt != nil {
		p.DataType = extractString(dt["$Type"])
	}

	return p
}

func parseDBTableMapping(raw map[string]any) *model.DatabaseTableMapping {
	m := &model.DatabaseTableMapping{}
	m.ID = model.ID(extractBsonID(raw["$ID"]))
	m.TypeName = extractString(raw["$Type"])
	m.Entity = extractString(raw["Entity"])
	m.TableName = extractString(raw["TableName"])

	// Parse Columns
	columns := extractBsonArray(raw["Columns"])
	for _, c := range columns {
		if cMap := extractBsonMap(c); cMap != nil {
			m.Columns = append(m.Columns, parseDBColumnMapping(cMap))
		}
	}

	return m
}

func parseDBColumnMapping(raw map[string]any) *model.DatabaseColumnMapping {
	c := &model.DatabaseColumnMapping{}
	c.ID = model.ID(extractBsonID(raw["$ID"]))
	c.TypeName = extractString(raw["$Type"])
	c.Attribute = extractString(raw["Attribute"])
	c.ColumnName = extractString(raw["ColumnName"])

	// SqlDataType is polymorphic: SimpleSqlDataType or LimitedLengthSqlDataType
	if dt := extractBsonMap(raw["SqlDataType"]); dt != nil {
		c.SqlDataType = extractString(dt["$Type"])
	}

	return c
}
