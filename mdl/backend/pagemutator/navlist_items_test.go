// SPDX-License-Identifier: Apache-2.0

package pagemutator

import (
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/backend/bsonnav"
	"github.com/mendixlabs/mxcli/mdl/bsonutil"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// navStubDeps serializes a navigation list the way the codec does: a
// Forms$NavigationList whose Items are Forms$NavigationListItem with no Name.
type navStubDeps struct{ stubWidgetDeps }

func (d *navStubDeps) SerializeWidget(w pages.Widget) bson.D {
	nl, ok := w.(*pages.NavigationList)
	if !ok {
		return d.stubWidgetDeps.SerializeWidget(w)
	}
	arr := bson.A{int32(2)}
	for _, it := range nl.Items {
		widgets := bson.A{int32(2)}
		for _, c := range it.Widgets {
			widgets = append(widgets, d.stubWidgetDeps.SerializeWidget(c))
		}
		arr = append(arr, bson.D{
			{Key: "$ID", Value: bsonutil.IDToBsonBinary(string(it.ID))},
			{Key: "$Type", Value: "Forms$NavigationListItem"},
			{Key: "Widgets", Value: widgets},
		})
	}
	return bson.D{{Key: "$Type", Value: "Forms$NavigationList"}, {Key: "Items", Value: arr}}
}

// navItemDoc is a stored item the way Studio Pro writes it: no Name.
func navItemDoc(textName string) bson.D {
	return bson.D{
		{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
		{Key: "$Type", Value: "Forms$NavigationListItem"},
		{Key: "Widgets", Value: bson.A{int32(2), bson.D{
			{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
			{Key: "$Type", Value: "Forms$DynamicText"},
			{Key: "Name", Value: textName},
		}}},
	}
}

func makeNavListPage() bson.D {
	return makeRawPage(bson.D{
		{Key: "$Type", Value: "Forms$NavigationList"},
		{Key: "Name", Value: "navList1"},
		{Key: "Items", Value: bson.A{int32(2), navItemDoc("t1")}},
	})
}

func newNavItem(textName string) *pages.NavigationListItem {
	return &pages.NavigationListItem{
		BaseElement: model.BaseElement{ID: model.ID(types.GenerateID()), TypeName: "Forms$NavigationListItem"},
		Widgets:     []pages.Widget{&pages.DynamicText{BaseWidget: pages.BaseWidget{Name: textName}}},
	}
}

// navItemTexts lists, per stored item, the name of its first widget.
func storedItemTexts(t *testing.T, raw bson.D) []string {
	t.Helper()
	nl := findBsonWidget(raw, "navList1")
	if nl == nil {
		t.Fatal("navList1 no longer resolves")
	}
	items := bsonnav.ToBsonA(bsonnav.DGet(nl.widget, "Items"))
	if len(items) == 0 || !isListMarker(items[0]) {
		t.Fatalf("Items lost its list marker: %v", items)
	}
	var out []string
	for _, el := range items[1:] {
		ws := bsonnav.DGetArrayElements(bsonnav.DGet(el.(bson.D), "Widgets"))
		out = append(out, bsonnav.DGetString(ws[0].(bson.D), "Name"))
	}
	return out
}

// mendixlabs/mxcli#1388: a widget inside a navigation list item could not be
// addressed — `insert after t1 { … }` failed with `widget "t1" not found` —
// because the lookup never descended into the list's Items.
func TestNavigationListItemWidgets_AreAddressable(t *testing.T) {
	raw := makeNavListPage()
	m := New(raw, model.ID("page-1"), &navStubDeps{})
	if findBsonWidget(raw, "t1") == nil {
		t.Fatal(`findBsonWidget("t1") = nil; a widget inside a navigation list item must resolve`)
	}
	if _, ok := m.WidgetScope()["t1"]; !ok {
		t.Error("WidgetScope lacks t1: a duplicate name inside an item would not be caught")
	}
	if _, found := findNearestDataSourceDoc(raw, "t1"); !found {
		t.Error("findNearestDataSourceDoc does not find t1")
	}
	// A sibling inserted next to t1 lands in the item's own Widgets.
	if err := m.InsertWidget("t1", "", toPos("after"), []pages.Widget{&pages.DynamicText{BaseWidget: pages.BaseWidget{Name: "t1b"}}}); err != nil {
		t.Fatalf("insert after t1: %v", err)
	}
	ws := bsonnav.ToBsonA(bsonnav.DGet(findBsonWidget(m.rawData, "t1").parentDoc, "Widgets"))
	if len(ws) != 3 || bsonnav.DGetString(ws[2].(bson.D), "Name") != "t1b" {
		t.Errorf("item widgets = %v, want [marker t1 t1b]", ws)
	}
}

// mendixlabs/mxcli#1388: `insert into navList1 { item … }` appends the item to
// the list's Items and leaves the existing item untouched.
func TestInsertNavigationListItems_IntoAppends(t *testing.T) {
	m := New(makeNavListPage(), model.ID("page-1"), &navStubDeps{})
	firstBefore := bsonnav.DGet(bsonnav.ToBsonA(bsonnav.DGet(findBsonWidget(m.rawData, "navList1").widget, "Items"))[1].(bson.D), "$ID")
	if err := m.InsertNavigationListItems("navList1", toPos("into"), []*pages.NavigationListItem{newNavItem("t2")}); err != nil {
		t.Fatalf("insert into navList1 { item }: %v", err)
	}
	if got := strings.Join(storedItemTexts(t, m.rawData), ","); got != "t1,t2" {
		t.Errorf("items = %s, want t1,t2", got)
	}
	firstAfter := bsonnav.DGet(bsonnav.ToBsonA(bsonnav.DGet(findBsonWidget(m.rawData, "navList1").widget, "Items"))[1].(bson.D), "$ID")
	if !equalIDs(firstBefore, firstAfter) {
		t.Error("the existing item's $ID changed")
	}
}

// An empty navigation list gains an Items list with Studio Pro's marker.
func TestInsertNavigationListItems_IntoEmptyList(t *testing.T) {
	raw := makeRawPage(bson.D{{Key: "$Type", Value: "Forms$NavigationList"}, {Key: "Name", Value: "navList1"}})
	m := New(raw, model.ID("page-1"), &navStubDeps{})
	if err := m.InsertNavigationListItems("navList1", toPos("into"), []*pages.NavigationListItem{newNavItem("t2")}); err != nil {
		t.Fatalf("insert into empty navList1: %v", err)
	}
	if got := strings.Join(storedItemTexts(t, m.rawData), ","); got != "t2" {
		t.Errorf("items = %s, want t2", got)
	}
}

// Items have no Name, so BEFORE/AFTER has nothing to name; a widget inside an
// item is not an item's sibling. Refuse rather than put an item into a widget
// list, and say which form works.
func TestInsertNavigationListItems_RefusesWrongTargets(t *testing.T) {
	for _, tc := range []struct{ target, pos, want string }{
		{"t1", "after", "insert after navList1 item(t1) { item … }"},
		{"t1", "into", "only be added to a navigation list"},
	} {
		m := New(makeNavListPage(), model.ID("page-1"), &navStubDeps{})
		err := m.InsertNavigationListItems(tc.target, toPos(tc.pos), []*pages.NavigationListItem{newNavItem("t2")})
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("insert %s %s { item }: err = %v, want one naming %q", tc.pos, tc.target, err, tc.want)
		}
		if got := strings.Join(storedItemTexts(t, m.rawData), ","); got != "t1" {
			t.Errorf("a refused insert changed the items: %s", got)
		}
	}
}

// navItemWithText is a stored item holding one dynamic text, its name and its
// shown text given, and the Action every item carries.
func navItemWithText(textName, text string) bson.D {
	return bson.D{
		{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
		{Key: "$Type", Value: "Forms$NavigationListItem"},
		{Key: "Action", Value: bson.D{{Key: "$Type", Value: "Forms$NoAction"}}},
		{Key: "Widgets", Value: bson.A{int32(2), bson.D{
			{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
			{Key: "$Type", Value: "Forms$DynamicText"},
			{Key: "Name", Value: textName},
			{Key: "Content", Value: bson.D{
				{Key: "$Type", Value: "Forms$ClientTemplate"},
				{Key: "Template", Value: textsText(translation("en_US", text))},
			}},
		}}},
	}
}

// Three items: t1 'Home', t2 'Orders', t3 'Home' — two share a text.
func makeThreeItemNavList() *Mutator {
	raw := makeRawPage(bson.D{
		{Key: "$Type", Value: "Forms$NavigationList"},
		{Key: "Name", Value: "navList1"},
		{Key: "Items", Value: bson.A{int32(2),
			navItemWithText("t1", "Home"), navItemWithText("t2", "Orders"), navItemWithText("t3", "Home")}},
	})
	return New(raw, model.ID("page-1"), &navStubDeps{})
}

func navItemIndex(t *testing.T, m *Mutator, ref string) int {
	t.Helper()
	r := m.widgetFinder(m.rawData, ref)
	if r == nil {
		t.Fatalf("%s did not resolve: %v", ref, m.widgetNotFoundError(ref))
	}
	if typ := widgetTypeName(r.widget); typ != navListItemType {
		t.Fatalf("%s resolved to %s, want an item", ref, typ)
	}
	return r.index
}

// mendixlabs/mxcli#1388: an item has no name, so it is addressed by a widget it
// holds, a text it shows, or its position — as a DataGrid 2 column is.
func TestNavItemAddress_Resolves(t *testing.T) {
	m := makeThreeItemNavList()
	for ref, want := range map[string]int{
		"navList1 item(t2)":       1,
		"navList1 item('Orders')": 1,
		"navList1 item@3":         2,
		"navList1 item('Home')@2": 2,
		"navList1 item(t1)@1":     0,
	} {
		if got := navItemIndex(t, m, ref); got != want {
			t.Errorf("%s = item %d, want %d", ref, got+1, want+1)
		}
	}
	if match, err := m.ResolveAlterTarget(backend.AlterTarget{Path: []string{"navList1 item@2"}}); err != nil || match.Kind != "item" {
		t.Errorf("ResolveAlterTarget(navList1 item@2) = %+v, %v; want kind item", match, err)
	}
}

// An address that matches several items, none, or a widget that is not a
// navigation list is refused with what would work — never a guess.
func TestNavItemAddress_RefusesMissesAndAmbiguity(t *testing.T) {
	m := makeThreeItemNavList()
	for ref, want := range map[string]string{
		"navList1 item('Home')":   "is ambiguous: 2 items match — add @n",
		"navList1 item('Nope')":   "no item matches",
		"navList1 item@4":         "the list has 3 items, so there is no @4",
		"navList1 item('Home')@3": "2 items match, so there is no @3",
		"navList1 item(zz)":       "item@2 = item(t2) = item('Orders')",
		"t1 item@1":               "is a Forms$DynamicText, not a navigation list",
		"nope item@1":             `widget "nope" not found`,
	} {
		if m.widgetFinder(m.rawData, ref) != nil {
			t.Errorf("%s resolved; want a refusal", ref)
			continue
		}
		if err := m.widgetNotFoundError(ref); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: err = %v, want one containing %q", ref, err, want)
		}
	}
}

func itemTexts(t *testing.T, m *Mutator) string {
	t.Helper()
	return strings.Join(storedItemTexts(t, m.rawData), ",")
}

// Every operation reaches an item through its address.
func TestNavItemAddress_Operations(t *testing.T) {
	t.Run("drop", func(t *testing.T) {
		m := makeThreeItemNavList()
		if err := m.DropWidget([]backend.WidgetRef{{Widget: "navList1 item('Orders')"}}); err != nil {
			t.Fatal(err)
		}
		if got := itemTexts(t, m); got != "t1,t3" {
			t.Errorf("items = %s, want t1,t3", got)
		}
	})
	t.Run("insert item after", func(t *testing.T) {
		m := makeThreeItemNavList()
		if err := m.InsertNavigationListItems("navList1 item(t1)", toPos("after"), []*pages.NavigationListItem{newNavItem("tNew")}); err != nil {
			t.Fatal(err)
		}
		if got := itemTexts(t, m); got != "t1,tNew,t2,t3" {
			t.Errorf("items = %s, want t1,tNew,t2,t3", got)
		}
	})
	t.Run("insert item before", func(t *testing.T) {
		m := makeThreeItemNavList()
		if err := m.InsertNavigationListItems("navList1 item@1", toPos("before"), []*pages.NavigationListItem{newNavItem("tNew")}); err != nil {
			t.Fatal(err)
		}
		if got := itemTexts(t, m); got != "tNew,t1,t2,t3" {
			t.Errorf("items = %s, want tNew,t1,t2,t3", got)
		}
	})
	t.Run("replace item", func(t *testing.T) {
		m := makeThreeItemNavList()
		if err := m.ReplaceNavigationListItem("navList1 item@2", []*pages.NavigationListItem{newNavItem("tNew")}); err != nil {
			t.Fatal(err)
		}
		if got := itemTexts(t, m); got != "t1,tNew,t3" {
			t.Errorf("items = %s, want t1,tNew,t3", got)
		}
	})
	t.Run("insert widgets into item", func(t *testing.T) {
		m := makeThreeItemNavList()
		if err := m.InsertWidget("navList1 item@2", "", toPos("into"), []pages.Widget{&pages.DynamicText{BaseWidget: pages.BaseWidget{Name: "t2b"}}}); err != nil {
			t.Fatal(err)
		}
		ws := bsonnav.DGetArrayElements(bsonnav.DGet(m.widgetFinder(m.rawData, "navList1 item@2").widget, "Widgets"))
		if len(ws) != 2 || bsonnav.DGetString(ws[1].(bson.D), "Name") != "t2b" {
			t.Errorf("item 2 widgets = %v, want t2, t2b", ws)
		}
	})
	t.Run("set action", func(t *testing.T) {
		m := makeThreeItemNavList()
		var called bool
		m.deps = &actionStubDeps{navStubDeps: navStubDeps{}, onAction: func() { called = true }}
		if err := m.SetWidgetAction("navList1 item@3", &pages.NoClientAction{}); err != nil {
			t.Fatal(err)
		}
		if !called {
			t.Error("the action was not serialized")
		}
		if typ := bsonnav.DGetString(bsonnav.DGetDoc(m.widgetFinder(m.rawData, "navList1 item@3").widget, "Action"), "$Type"); typ != "Forms$ShowPageAction" {
			t.Errorf("item 3 action = %s, want the new one", typ)
		}
	})
}

type actionStubDeps struct {
	navStubDeps
	onAction func()
}

func (d *actionStubDeps) SerializeClientAction(pages.ClientAction) bson.D {
	d.onAction()
	return bson.D{{Key: "$Type", Value: "Forms$ShowPageAction"}}
}

// Widgets next to an item, or in its place, would land in the list's Items.
func TestNavItemAddress_RefusesWidgetsAmongItems(t *testing.T) {
	w := []pages.Widget{&pages.DynamicText{BaseWidget: pages.BaseWidget{Name: "x"}}}
	m := makeThreeItemNavList()
	if err := m.InsertWidget("navList1 item@1", "", toPos("after"), w); err == nil || !strings.Contains(err.Error(), "only items belong") {
		t.Errorf("insert after an item {widget}: err = %v", err)
	}
	if err := m.ReplaceWidget("navList1 item@1", "", w); err == nil || !strings.Contains(err.Error(), "only items belong") {
		t.Errorf("replace an item with a widget: err = %v", err)
	}
	if got := itemTexts(t, m); got != "t1,t2,t3" {
		t.Errorf("a refused edit changed the items: %s", got)
	}
}
