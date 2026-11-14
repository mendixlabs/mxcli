// SPDX-License-Identifier: Apache-2.0

package pagemutator

import (
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/backend/bsonnav"
	"github.com/mendixlabs/mxcli/mdl/bsonutil"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// ako/mxcli#2173: the widgets inside a list view's specialization template
// were unreachable — `set … on`, `insert after` and `drop` all answered "widget
// not found" (Conversational UI's Snippet_Message_Content), though the
// alter-page skill documents them as ordinary named widgets. The control is a
// widget in the same list view's default body, which was always found.
func templateListView() bson.D {
	return bson.D{
		{Key: "$Type", Value: "Forms$ListView"},
		{Key: "Name", Value: "listView1"},
		{Key: "DataSource", Value: bson.D{
			{Key: "$Type", Value: "Forms$ListViewXPathSource"},
			{Key: "EntityRef", Value: bson.D{{Key: "$Type", Value: "DomainModels$DirectEntityRef"}, {Key: "Entity", Value: "Chat.Message"}}},
		}},
		{Key: "Widgets", Value: bson.A{int32(2), bson.D{{Key: "$Type", Value: "Forms$DynamicText"}, {Key: "Name", Value: "bodyText"}}}},
		{Key: "Templates", Value: bson.A{int32(2), bson.D{
			{Key: "$Type", Value: "Forms$ListViewTemplate"},
			{Key: "Entity", Value: "Chat.ToolMessage"},
			{Key: "Widgets", Value: bson.A{int32(2),
				bson.D{{Key: "$Type", Value: "Forms$DivContainer"}, {Key: "Name", Value: "container15"},
					{Key: "Widgets", Value: bson.A{int32(2), bson.D{{Key: "$ID", Value: bsonutil.NewIDBsonBinary()}, {Key: "$Type", Value: "Forms$DynamicText"}, {Key: "Name", Value: "toolText"}}}}},
			}},
		}}},
	}
}

func TestListViewTemplateWidgetsAreAddressable(t *testing.T) {
	m := New(makeRawPage(templateListView()), model.ID("u"), &stubWidgetDeps{})

	for _, name := range []string{"bodyText", "container15", "toolText"} {
		if !m.FindWidget(name) {
			t.Errorf("widget %q not found", name)
		}
	}
	if _, ok := m.WidgetScope()["toolText"]; !ok {
		t.Error("a template's widget is missing from the widget scope")
	}

	// A template's widgets bind to the specialization; the default body to
	// the list view's own entity.
	if got := m.EnclosingEntity("toolText"); got != "Chat.ToolMessage" {
		t.Errorf("EnclosingEntity(toolText) = %q, want Chat.ToolMessage", got)
	}
	if got := m.EnclosingEntity("container15"); got != "Chat.ToolMessage" {
		t.Errorf("EnclosingEntity(container15) = %q, want Chat.ToolMessage", got)
	}
	if got := m.EnclosingEntity("bodyText"); got != "Chat.Message" {
		t.Errorf("control: EnclosingEntity(bodyText) = %q, want Chat.Message", got)
	}

	// The reported statement: insert after a template's widget lands in that template.
	if err := m.InsertWidget("container15", "", backend.InsertAfter,
		[]pages.Widget{&pages.TextBox{BaseWidget: pages.BaseWidget{Name: "t"}}}); err != nil {
		t.Fatalf("insert after container15: %v", err)
	}
	lv := m.widgetFinder(m.rawData, "listView1")
	tmpl := bsonnav.DGetArrayElements(bsonnav.DGet(lv.widget, "Templates"))[0].(bson.D)
	if n := len(bsonnav.DGetArrayElements(bsonnav.DGet(tmpl, "Widgets"))); n != 2 {
		t.Errorf("template holds %d widgets after the insert, want 2", n)
	}
	if err := m.DropWidget([]backend.WidgetRef{{Widget: "toolText"}}); err != nil {
		t.Errorf("drop toolText: %v", err)
	}
}
