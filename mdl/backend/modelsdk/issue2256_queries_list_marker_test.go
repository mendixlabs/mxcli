// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/mdl/dbconnector"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/modelsdk/canon"
)

// ako/mxcli#2256: Studio Pro writes a DatabaseConnection's Queries list with
// marker 3, empty or not, and every list below it (a query's Parameters and
// TableMappings, a mapping's Columns) and the connection's AdditionalProperties
// with marker 2. Measured twice: Studio Pro 11.16.1 saving ChatV2's
// VectorDatabase (commit 5a85760, two queries: Queries 2 -> 3, nothing else),
// and `mx convert` 11.16.0 rewriting one connection with no queries and one
// with a query (Queries 2 -> 3 in both, nothing else). mxcli wrote 2, so every
// exec of the defining script flipped Studio Pro's 3 back.

// listMarkers returns "Owner.Key" -> marker for every list in a stored unit.
func listMarkers(t *testing.T, raw []byte) map[string][]int32 {
	t.Helper()
	var doc bson.D
	if err := bson.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("unmarshal unit: %v", err)
	}
	out := map[string][]int32{}
	var walk func(any)
	walk = func(v any) {
		switch x := v.(type) {
		case bson.D:
			owner := ""
			for _, e := range x {
				if e.Key == "$Type" {
					owner, _ = e.Value.(string)
				}
			}
			for _, e := range x {
				if a, ok := e.Value.(bson.A); ok && len(a) > 0 {
					if m, ok := a[0].(int32); ok {
						out[owner+"."+e.Key] = append(out[owner+"."+e.Key], m)
					}
				}
				walk(e.Value)
			}
		case bson.A:
			for _, e := range x {
				walk(e)
			}
		}
	}
	walk(doc)
	return out
}

func TestIssue2256_DatabaseConnectionListMarkersAreStudioPros(t *testing.T) {
	withQuery := func() []*model.DatabaseQuery {
		return []*model.DatabaseQuery{{
			Name:      "ChunksForDocument",
			SQL:       "select a from t where b = {p}",
			QueryType: dbconnector.CustomSQLQueryType,
			Parameters: []*model.DatabaseQueryParameter{{
				ParameterName: "p", DataType: "DataTypes$StringType",
			}},
			TableMappings: []*model.DatabaseTableMapping{{
				Entity: "MyFirstModule.Row",
				Columns: []*model.DatabaseColumnMapping{{
					Attribute: "MyFirstModule.Row.A", ColumnName: "a",
				}},
			}},
		}}
	}
	// What Studio Pro stores, per list, in the measured units.
	want := map[string]int32{
		"DatabaseConnector$DatabaseConnection.Queries":              3,
		"DatabaseConnector$DatabaseConnection.AdditionalProperties": 2,
		"DatabaseConnector$DatabaseQuery.Parameters":                2,
		"DatabaseConnector$DatabaseQuery.TableMappings":             2,
		"DatabaseConnector$TableMapping.Columns":                    2,
	}
	for _, tc := range []struct {
		name    string
		queries func() []*model.DatabaseQuery
	}{
		{"no_queries", func() []*model.DatabaseQuery { return nil }},
		{"one_query", withQuery},
	} {
		t.Run(tc.name, func(t *testing.T) {
			proj := copyFixture(t)
			b := New()
			if err := b.Connect(proj); err != nil {
				t.Fatalf("connect: %v", err)
			}
			mod, err := b.GetModuleByName("MyFirstModule")
			if err != nil || mod == nil {
				t.Fatalf("GetModuleByName: %v", err)
			}
			conn := &model.DatabaseConnection{
				ContainerID: mod.ID, Name: "VectorDatabase", DatabaseType: "PostgreSQL",
				Queries: tc.queries(),
			}
			if err := b.CreateDatabaseConnection(conn); err != nil {
				t.Fatalf("CreateDatabaseConnection: %v", err)
			}
			check := func(stage string) {
				got := listMarkers(t, readUnitBytes(t, proj, string(conn.ID)))
				for key, m := range want {
					ms, ok := got[key]
					if !ok {
						if len(tc.queries()) == 0 && key != "DatabaseConnector$DatabaseConnection.Queries" &&
							key != "DatabaseConnector$DatabaseConnection.AdditionalProperties" {
							continue // no query, so no query-level lists
						}
						t.Errorf("%s: %s not written", stage, key)
						continue
					}
					for _, gm := range ms {
						if gm != m {
							t.Errorf("%s: %s marker = %d, Studio Pro writes %d", stage, key, gm, m)
						}
					}
				}
				// Control: the marker is the only thing at stake, the queries
				// themselves are written as before.
				if len(tc.queries()) > 0 {
					q := readFirstQuery(t, proj, string(conn.ID))
					if q["Name"] != "ChunksForDocument" || q["Query"] != "select a from t where b = {p}" {
						t.Errorf("%s: query = %v/%v, want the query as declared", stage, q["Name"], q["Query"])
					}
				}
			}
			check("create")
			if err := b.UpdateDatabaseConnection(conn); err != nil {
				t.Fatalf("UpdateDatabaseConnection: %v", err)
			}
			check("update")
			if err := b.Disconnect(); err != nil {
				t.Fatalf("disconnect: %v", err)
			}
		})
	}
}

// A connection an older mxcli stored with Queries marker 2 gets Studio Pro's 3
// on its next write, whoever builds the list (canon.NormalizeListMarkers).
func TestIssue2256_StoredQueriesMarker2IsNormalizedTo3(t *testing.T) {
	query := bson.D{
		{Key: "$ID", Value: bson.Binary{Subtype: 0, Data: make([]byte, 16)}},
		{Key: "$Type", Value: "DatabaseConnector$DatabaseQuery"},
		{Key: "Name", Value: "Q"},
		{Key: "Parameters", Value: bson.A{int32(2)}},
		{Key: "TableMappings", Value: bson.A{int32(2)}},
	}
	stored, err := bson.Marshal(bson.D{
		{Key: "$ID", Value: bson.Binary{Subtype: 0, Data: make([]byte, 16)}},
		{Key: "$Type", Value: "DatabaseConnector$DatabaseConnection"},
		{Key: "Name", Value: "VectorDatabase"},
		{Key: "Queries", Value: bson.A{int32(2), query}},
		{Key: "AdditionalProperties", Value: bson.A{int32(2)}},
	})
	if err != nil {
		t.Fatal(err)
	}
	got := listMarkers(t, canon.NormalizeListMarkers(stored))
	if m := got["DatabaseConnector$DatabaseConnection.Queries"]; len(m) != 1 || m[0] != 3 {
		t.Errorf("Queries marker after normalization = %v, want [3]", m)
	}
	for _, key := range []string{
		"DatabaseConnector$DatabaseConnection.AdditionalProperties",
		"DatabaseConnector$DatabaseQuery.Parameters",
		"DatabaseConnector$DatabaseQuery.TableMappings",
	} {
		if m := got[key]; len(m) != 1 || m[0] != 2 {
			t.Errorf("%s marker after normalization = %v, want [2] (Studio Pro's, untouched)", key, m)
		}
	}
}
