// SPDX-License-Identifier: Apache-2.0

package catalog

import (
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/modelsdk/bsoncompat"
)

func guidBin(b byte) bson.Binary {
	data := make([]byte, 16)
	for i := range data {
		data[i] = b + byte(i)
	}
	return bson.Binary{Subtype: 0, Data: data}
}

// ako/mxcli#1879: a page data source's ComponentId was its binary $ID formatted
// with %v — `{0 [57 221 206 …]}` — so no row could be joined to anything, and
// the row named no widget at all (ComponentName is the source's $Type). Lint
// rule SEC011 could only name a widget when the page had exactly one data
// widget over that entity.
//
// The XPath is stored under the widget's data source; the row now carries the
// source's id in the catalog's own GUID form and the widget that owns it, by
// the same id the widgets table records for it.
func TestXPathRowsCarryIDsAndTheirWidget(t *testing.T) {
	page := bson.D{
		{Key: "$ID", Value: guidBin(0x10)},
		{Key: "$Type", Value: "Forms$Page"},
		{Key: "Name", Value: "Portal"},
		{Key: "FormCall", Value: bson.D{
			{Key: "$ID", Value: guidBin(0x20)},
			{Key: "Arguments", Value: bson.A{int32(2), bson.D{
				{Key: "$ID", Value: guidBin(0x30)},
				{Key: "Widgets", Value: bson.A{int32(2), bson.D{
					{Key: "$ID", Value: guidBin(0x40)},
					{Key: "$Type", Value: "CustomWidgets$CustomWidget"},
					{Key: "Name", Value: "dgOrders"},
					{Key: "Object", Value: bson.D{
						{Key: "$ID", Value: guidBin(0x50)},
						{Key: "Properties", Value: bson.A{int32(2), bson.D{
							{Key: "$ID", Value: guidBin(0x60)},
							{Key: "Value", Value: bson.D{
								{Key: "$ID", Value: guidBin(0x70)},
								{Key: "DataSource", Value: bson.D{
									{Key: "$ID", Value: guidBin(0x80)},
									{Key: "$Type", Value: "CustomWidgets$CustomWidgetXPathSource"},
									{Key: "XPathConstraint", Value: "[IsActive = true()]"},
									{Key: "EntityRef", Value: bson.D{
										{Key: "$Type", Value: "DomainModels$DirectEntityRef"},
										{Key: "Entity", Value: "Shop.Account"},
									}},
								}},
							}},
						}}},
					}},
				}}},
			}}},
		}},
	}
	raw, err := bson.Marshal(page)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := bsoncompat.UnmarshalDocMap(raw, &doc); err != nil {
		t.Fatal(err)
	}

	cat, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer cat.Close()
	tx, err := cat.db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	stmt, err := wrapTx(tx).Prepare(xpathInsertSQL)
	if err != nil {
		t.Fatal(err)
	}
	if n := scanBSONForXPath(stmt, doc, "PAGE", "page-1", "Shop.Portal", "Shop", "p", "s"); n != 1 {
		t.Fatalf("rows = %d, want 1", n)
	}
	stmt.Close()
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	var componentID, widgetID, widgetName string
	if err := cat.db.QueryRow(`SELECT ComponentId, COALESCE(WidgetId, ''), COALESCE(WidgetName, '') FROM xpath_expressions_data`).
		Scan(&componentID, &widgetID, &widgetName); err != nil {
		t.Fatal(err)
	}
	if want := formatGUID(guidBin(0x80).Data); componentID != want {
		t.Errorf("ComponentId = %q, want %q (the data source's $ID as a GUID)", componentID, want)
	}
	if want := extractBsonID(guidBin(0x40)); widgetID != want || widgetName != "dgOrders" {
		t.Errorf("widget = %q %q, want %q dgOrders — the id the widgets table records for it", widgetID, widgetName, want)
	}
}
