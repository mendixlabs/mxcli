// SPDX-License-Identifier: Apache-2.0

package pagemutator

import (
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/mdl/backend/bsonnav"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// cwSourceDeps serializes a pluggable data source to a marker naming its flow,
// so the test asserts where the source lands without an engine's serializer.
type cwSourceDeps struct{ Deps }

func (cwSourceDeps) SerializeFormDataSource(pages.DataSource, bool) bson.D { return nil }

func (cwSourceDeps) SerializeCustomWidgetDataSource(ds pages.DataSource) bson.D {
	switch d := ds.(type) {
	case *pages.MicroflowSource:
		return bson.D{{Key: "$Type", Value: "Forms$MicroflowSource"}, {Key: "Marker", Value: d.Microflow}}
	case *pages.NanoflowSource:
		return bson.D{{Key: "$Type", Value: "Forms$NanoflowSource"}, {Key: "Marker", Value: d.Nanoflow}}
	}
	return nil
}

// galleryDoc is a CustomWidget whose schema keys one property "datasource",
// stored with a database (XPath) source — the reported starting point.
func galleryDoc(name string) bson.D {
	typeID := bson.Binary{Subtype: 0x04, Data: []byte{
		0x41, 0x42, 0x43, 0x44, 0x45, 0x46, 0x47, 0x48,
		0x49, 0x4a, 0x4b, 0x4c, 0x4d, 0x4e, 0x4f, 0x50,
	}}
	return bson.D{
		{Key: "$Type", Value: "CustomWidgets$CustomWidget"},
		{Key: "Name", Value: name},
		{Key: "Type", Value: bson.D{
			{Key: "$Type", Value: "CustomWidgets$CustomWidgetType"},
			{Key: "WidgetId", Value: "com.mendix.widget.web.gallery.Gallery"},
			{Key: "ObjectType", Value: bson.D{
				{Key: "PropertyTypes", Value: bson.A{int32(2), bson.D{
					{Key: "$ID", Value: typeID},
					{Key: "PropertyKey", Value: "datasource"},
					{Key: "ValueType", Value: bson.D{{Key: "Type", Value: "DataSource"}}},
				}}},
			}},
		}},
		{Key: "Object", Value: bson.D{
			{Key: "Properties", Value: bson.A{int32(2), bson.D{
				{Key: "TypePointer", Value: typeID},
				{Key: "Value", Value: bson.D{
					{Key: "DataSource", Value: bson.D{
						{Key: "$Type", Value: "CustomWidgets$CustomWidgetXPathSource"},
					}},
				}},
			}}},
		}},
	}
}

// mendixlabs/mxcli#1548: `set (DataSource: microflow M.F) on galOrders` passed
// check, printed "Altered page" and left the unit byte-identical. A
// CustomWidget keeps its source in the datasource property's Value, not in a
// top-level DataSource key, and the setter's DSet onto that missing key failed
// silently.
func TestSetWidgetDataSource_PluggableMicroflow_1548(t *testing.T) {
	m := New(makeRawPage(galleryDoc("galOrders")), model.ID("unit-1"), cwSourceDeps{})
	if err := m.SetWidgetDataSource("galOrders", &pages.MicroflowSource{Microflow: "Shop.DS_Order_Open"}); err != nil {
		t.Fatalf("SetWidgetDataSource: %v", err)
	}
	w := findWidgetForTest(t, m.rawData, "galOrders")
	ds := pluggableDataSourceDoc(w)
	if got := bsonnav.DGetString(ds, "Marker"); got != "Shop.DS_Order_Open" {
		t.Fatalf("pluggable datasource = %v, want the microflow source", ds)
	}
	if bsonnav.DGet(w, "DataSource") != nil {
		t.Error("a CustomWidget must not get a top-level DataSource key")
	}
}

func TestSetWidgetDataSource_PluggableNanoflow_1548(t *testing.T) {
	m := New(makeRawPage(galleryDoc("galOrders")), model.ID("unit-1"), cwSourceDeps{})
	if err := m.SetWidgetDataSource("galOrders", &pages.NanoflowSource{Nanoflow: "Shop.NF_Orders"}); err != nil {
		t.Fatalf("SetWidgetDataSource: %v", err)
	}
	if got := bsonnav.DGetString(pluggableDataSourceDoc(findWidgetForTest(t, m.rawData, "galOrders")), "Marker"); got != "Shop.NF_Orders" {
		t.Fatalf("pluggable datasource marker = %q, want the nanoflow", got)
	}
}

// A source the pluggable serializer has no shape for is refused, not dropped.
func TestSetWidgetDataSource_PluggableUnsupportedIsRefused_1548(t *testing.T) {
	m := New(makeRawPage(galleryDoc("galOrders")), model.ID("unit-1"), cwSourceDeps{})
	err := m.SetWidgetDataSource("galOrders", &pages.ListenToWidgetSource{WidgetName: "dg1"})
	if err == nil {
		t.Fatal("expected a refusal for a selection source on a gallery")
	}
	if !strings.Contains(err.Error(), "galOrders") {
		t.Errorf("refusal should name the widget: %v", err)
	}
}

// No setter may report success without writing: a widget with no DataSource
// slot at all is refused.
func TestSetWidgetDataSource_NoSlotIsRefused_1548(t *testing.T) {
	txt := bson.D{{Key: "$Type", Value: "Forms$DynamicText"}, {Key: "Name", Value: "txt1"}}
	m := New(makeRawPage(txt), model.ID("unit-1"), cwSourceDeps{})
	if err := m.SetWidgetDataSource("txt1", &pages.MicroflowSource{Microflow: "Shop.DS"}); err == nil {
		t.Fatal("expected a refusal: a dynamic text has no data source")
	}
}
