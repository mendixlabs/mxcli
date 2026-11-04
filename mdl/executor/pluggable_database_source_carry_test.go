// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// ako/mxcli#1208: on a 9.x project, rewriting a pluggable widget whose stored
// source is Studio Pro 9's "Database" source (CustomWidgetDatabaseSource) wrote
// it back as an XPath source, though describe prints both as `database from E
// where …` and the statement asked for no change of type. The rewrite now keeps
// the stored type; this pins the detection it rests on. The end-to-end run (a
// 9.24 project, a grid edited by its sort only) is in the PR: the base build
// writes CustomWidgetXPathSource, the fix keeps the DatabaseSource and its
// constraint rows.
func TestStoredUsesDatabaseSource(t *testing.T) {
	db := bson.D{{Key: "Properties", Value: bson.A{int32(2), bson.D{
		{Key: "Value", Value: bson.D{{Key: "DataSource", Value: bson.D{{Key: "$Type", Value: "CustomWidgets$CustomWidgetDatabaseSource"}}}}},
	}}}}
	xp := bson.D{{Key: "Properties", Value: bson.A{int32(2), bson.D{
		{Key: "Value", Value: bson.D{{Key: "DataSource", Value: bson.D{{Key: "$Type", Value: "CustomWidgets$CustomWidgetXPathSource"}}}}},
	}}}}
	pt := &pluggablePassthrough{byName: map[string]*storedPluggable{
		"dg":  {obj: db},
		"gal": {obj: xp},
	}}
	if !pt.storedUsesDatabaseSource("dg") {
		t.Error("a stored CustomWidgetDatabaseSource must be detected, nested in the Object")
	}
	if pt.storedUsesDatabaseSource("gal") {
		t.Error("an XPath source is not a database source")
	}
	if pt.storedUsesDatabaseSource("missing") || (*pluggablePassthrough)(nil).storedUsesDatabaseSource("dg") {
		t.Error("an unknown widget or no stored page is not a database source")
	}
}
