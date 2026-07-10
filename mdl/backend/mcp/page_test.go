// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/pages"
)

func TestMapPageWidget_ActionButton(t *testing.T) {
	b := &Backend{}
	btn := &pages.ActionButton{
		Caption:     &model.Text{Translations: map[string]string{"en_US": "Click Me"}},
		ButtonStyle: pages.ButtonStylePrimary,
		Action:      &pages.NoClientAction{},
	}
	btn.Name = "btn1"
	m, err := b.mapPageWidget(btn)
	if err != nil || m["$Type"] != "Pages$ActionButton" || m["name"] != "btn1" ||
		m["ct:caption"] != "Click Me" || m["buttonStyle"] != "Primary" {
		t.Fatalf("action button: %+v / %v", m, err)
	}
	if act, _ := m["action"].(map[string]any); act["$Type"] != "Pages$NoClientAction" {
		t.Fatalf("action: %+v", m["action"])
	}
	if ap, _ := m["appearance"].(map[string]any); ap["$Type"] != "Pages$Appearance" {
		t.Fatalf("appearance: %+v", m["appearance"])
	}
}

func TestMapPageWidget_ActionButton_CaptionTemplate(t *testing.T) {
	// The executor stores a button's caption in CaptionTemplate, not Caption.
	// Reading the wrong field produced captionless (invisible) buttons.
	b := &Backend{}
	btn := &pages.ActionButton{
		CaptionTemplate: &pages.ClientTemplate{Template: &model.Text{Translations: map[string]string{"en_US": "New Order"}}},
		Action:          &pages.NoClientAction{},
	}
	btn.Name = "btnNew"
	m, err := b.mapPageWidget(btn)
	if err != nil || m["ct:caption"] != "New Order" {
		t.Fatalf("caption from CaptionTemplate: %+v / %v", m["ct:caption"], err)
	}
}

func TestMapPageWidget_Container(t *testing.T) {
	b := &Backend{}
	inner := &pages.ActionButton{Action: &pages.NoClientAction{}}
	inner.Name = "btn"
	c := &pages.Container{Widgets: []pages.Widget{inner}}
	c.Name = "box1"
	m, err := b.mapPageWidget(c)
	if err != nil || m["$Type"] != "Pages$DivContainer" || m["name"] != "box1" {
		t.Fatalf("container: %+v / %v", m, err)
	}
	kids, _ := m["widgets"].([]any)
	if len(kids) != 1 {
		t.Fatalf("expected 1 child widget: %+v", kids)
	}
}

func TestMapClientAction(t *testing.T) {
	none, _ := mapClientAction(nil)
	if none["$Type"] != "Pages$NoClientAction" {
		t.Errorf("nil action -> %+v", none)
	}
	mf, err := mapClientAction(&pages.MicroflowClientAction{MicroflowName: "M.ACT_Do"})
	if err != nil || mf["$Type"] != "Pages$MicroflowClientAction" || mf["microflow"] != "M.ACT_Do" {
		t.Errorf("microflow action: %+v / %v", mf, err)
	}
	pg, err := mapClientAction(&pages.PageClientAction{PageName: "M.Detail"})
	if err != nil || pg["$Type"] != "Pages$PageClientAction" {
		t.Errorf("page action: %+v / %v", pg, err)
	}
	ps, _ := pg["pageSettings"].(map[string]any)
	if ps["page"] != "M.Detail" {
		t.Errorf("pageSettings: %+v", ps)
	}
	co, err := mapClientAction(&pages.CreateObjectClientAction{EntityName: "M.Order", PageName: "M.Order_Edit"})
	if err != nil || co["$Type"] != "Pages$CreateObjectClientAction" {
		t.Errorf("create-object action: %+v / %v", co, err)
	}
	if er, _ := co["entityRef"].(map[string]any); er["entity"] != "M.Order" {
		t.Errorf("create-object entityRef: %+v", co["entityRef"])
	}
	if cps, _ := co["pageSettings"].(map[string]any); cps["page"] != "M.Order_Edit" {
		t.Errorf("create-object pageSettings: %+v", co["pageSettings"])
	}
}

func TestButtonStyle(t *testing.T) {
	cases := map[string]string{"primary": "Primary", "PRIMARY": "Primary", "danger": "Danger", "": "Default", "bogus": "Default"}
	for in, want := range cases {
		if got := buttonStyle(in); got != want {
			t.Errorf("buttonStyle(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestMapPageWidget_DynamicText(t *testing.T) {
	b := &Backend{}
	dt := &pages.DynamicText{
		Content:    &pages.ClientTemplate{Template: &model.Text{Translations: map[string]string{"en_US": "$Loc/Name"}}},
		RenderMode: pages.TextRenderModeH2,
	}
	dt.Name = "txt1"
	m, err := b.mapPageWidget(dt)
	if err != nil || m["$Type"] != "Pages$DynamicText" || m["ct:content"] != "$Loc/Name" || m["renderMode"] != "H2" {
		t.Fatalf("dynamic text: %+v / %v", m, err)
	}
}

func TestMapPageWidget_DataView(t *testing.T) {
	b := &Backend{}
	inner := &pages.DynamicText{AttributePath: "$Loc/Name"}
	inner.Name = "t"
	dv := &pages.DataView{
		DataSource: &pages.DataViewSource{ParameterName: "Loc"},
		Widgets:    []pages.Widget{inner},
	}
	dv.Name = "dv1"
	m, err := b.mapPageWidget(dv)
	if err != nil || m["$Type"] != "Pages$DataView" {
		t.Fatalf("data view: %+v / %v", m, err)
	}
	src, _ := m["dataSource"].(map[string]any)
	sv, _ := src["sourceVariable"].(map[string]any)
	if sv["pageParameter"] != "Loc" {
		t.Fatalf("data view source: %+v", src)
	}
	if kids, _ := m["widgets"].([]any); len(kids) != 1 {
		t.Fatalf("data view children: %+v", m["widgets"])
	}
}

func TestMapDataViewSource(t *testing.T) {
	// page parameter -> sourceVariable
	pv, err := mapDataViewSource(&pages.DataViewSource{ParameterName: "Account"})
	if err != nil || pv["sourceVariable"] == nil {
		t.Fatalf("param source: %+v / %v", pv, err)
	}
	// context (parameter-bound) source with a resolved entity must carry BOTH the
	// sourceVariable binding AND entityRef — otherwise the data view has no entity
	// configured on its source (CE0488). Regression for the MCP page-authoring gap.
	ctx, err := mapDataViewSource(&pages.DataViewSource{ParameterName: "Product", EntityName: "MES.Product"})
	if err != nil {
		t.Fatalf("context source: %v", err)
	}
	if sv, _ := ctx["sourceVariable"].(map[string]any); sv["pageParameter"] != "Product" {
		t.Fatalf("context source missing/var wrong sourceVariable: %+v", ctx)
	}
	if ref, _ := ctx["entityRef"].(map[string]any); ref["entity"] != "MES.Product" {
		t.Fatalf("context source missing entityRef (CE0488): %+v", ctx)
	}
	// direct entity -> entityRef
	er, err := mapDataViewSource(&pages.DataViewSource{EntityName: "Sales.Order"})
	if err != nil {
		t.Fatalf("entity source: %v", err)
	}
	ref, _ := er["entityRef"].(map[string]any)
	if ref["entity"] != "Sales.Order" {
		t.Fatalf("entityRef: %+v", er)
	}
	// microflow source -> Pages$MicroflowSource with microflowSettings
	mf, err := mapDataViewSource(&pages.MicroflowSource{Microflow: "M.DSO_GetX"})
	if err != nil || mf["$Type"] != "Pages$MicroflowSource" {
		t.Fatalf("microflow source: %+v / %v", mf, err)
	}
	ms, _ := mf["microflowSettings"].(map[string]any)
	if ms["$Type"] != "Pages$MicroflowSettings" || ms["microflow"] != "M.DSO_GetX" {
		t.Fatalf("microflowSettings: %+v", ms)
	}
	// microflow source with no microflow -> error
	if _, err := mapDataViewSource(&pages.MicroflowSource{}); err == nil {
		t.Error("microflow source with no microflow should error")
	}
}

func TestMapPageWidget_Inputs(t *testing.T) {
	b := &Backend{}
	tb := &pages.TextBox{Label: "Name", AttributePath: "M.E.Name"}
	tb.Name = "tb1"
	m, err := b.mapPageWidget(tb)
	if err != nil || m["$Type"] != "Pages$TextBox" || m["ct:labelTemplate"] != "Name" {
		t.Fatalf("textbox: %+v / %v", m, err)
	}
	if ar, _ := m["attributeRef"].(map[string]any); ar["$Type"] != "DomainModels$AttributeRef" || ar["attribute"] != "M.E.Name" {
		t.Fatalf("textbox attributeRef: %+v", m["attributeRef"])
	}

	cb := &pages.CheckBox{Label: "Active", AttributePath: "M.E.Active"}
	cb.Name = "cb1"
	if m, err := b.mapPageWidget(cb); err != nil || m["$Type"] != "Pages$CheckBox" {
		t.Fatalf("checkbox: %+v / %v", m, err)
	}

	dp := &pages.DatePicker{Label: "When", AttributePath: "M.E.When"}
	dp.Name = "dp1"
	m, err = b.mapPageWidget(dp)
	if err != nil || m["$Type"] != "Pages$DatePicker" {
		t.Fatalf("datepicker: %+v / %v", m, err)
	}
	if ar, _ := m["attributeRef"].(map[string]any); ar["attribute"] != "M.E.When" {
		t.Fatalf("datepicker attributeRef: %+v", m["attributeRef"])
	}

	ta := &pages.TextArea{Label: "Notes", AttributePath: "M.E.Notes"}
	ta.Name = "ta1"
	if m, err := b.mapPageWidget(ta); err != nil || m["$Type"] != "Pages$TextArea" {
		t.Fatalf("textarea: %+v / %v", m, err)
	}

	rb := &pages.RadioButtons{Label: "Status", AttributePath: "M.E.Status"}
	rb.Name = "rb1"
	m, err = b.mapPageWidget(rb)
	if err != nil || m["$Type"] != "Pages$RadioButtonGroup" {
		t.Fatalf("radiobuttons: %+v / %v", m, err)
	}
	if ar, _ := m["attributeRef"].(map[string]any); ar["attribute"] != "M.E.Status" {
		t.Fatalf("radiobuttons attributeRef: %+v", m["attributeRef"])
	}
}

func TestMapPageWidget_LayoutGrid(t *testing.T) {
	b := &Backend{}
	btn := &pages.ActionButton{Action: &pages.NoClientAction{}}
	btn.Name = "btn"
	grid := &pages.LayoutGrid{
		Rows: []*pages.LayoutGridRow{{
			Columns: []*pages.LayoutGridColumn{
				{Weight: 6, Widgets: []pages.Widget{btn}},
				{}, // empty column -> defaults to full width (12)
			},
		}},
	}
	grid.Name = "lg"
	m, err := b.mapPageWidget(grid)
	if err != nil || m["$Type"] != "Pages$LayoutGrid" || m["width"] != "FullWidth" {
		t.Fatalf("layout grid: %+v / %v", m, err)
	}
	rows, _ := m["rows"].([]any)
	if len(rows) != 1 {
		t.Fatalf("rows: %+v", rows)
	}
	row, _ := rows[0].(map[string]any)
	cols, _ := row["columns"].([]any)
	if len(cols) != 2 {
		t.Fatalf("columns: %+v", cols)
	}
	c0, _ := cols[0].(map[string]any)
	c1, _ := cols[1].(map[string]any)
	if c0["weight"] != 6 || c1["weight"] != 12 {
		t.Fatalf("weights: %v / %v", c0["weight"], c1["weight"])
	}
	if kids, _ := c0["widgets"].([]any); len(kids) != 1 {
		t.Fatalf("column widgets: %+v", c0["widgets"])
	}
}

func TestMapPageWidget_ListView(t *testing.T) {
	b := &Backend{}
	row := &pages.DynamicText{AttributePath: "$currentObject/Name"}
	row.Name = "t"
	lv := &pages.ListView{
		DataSource: &pages.DatabaseSource{EntityName: "ObjListV10.Location"},
		Widgets:    []pages.Widget{row},
	}
	lv.Name = "lv1"
	m, err := b.mapPageWidget(lv)
	if err != nil || m["$Type"] != "Pages$ListView" {
		t.Fatalf("listview: %+v / %v", m, err)
	}
	src, _ := m["dataSource"].(map[string]any)
	ref, _ := src["entityRef"].(map[string]any)
	if ref["entity"] != "ObjListV10.Location" {
		t.Fatalf("listview source: %+v", src)
	}
	if kids, _ := m["widgets"].([]any); len(kids) != 1 {
		t.Fatalf("listview row widgets: %+v", m["widgets"])
	}
}

func TestMapPageWidget_DataGridRejected(t *testing.T) {
	b := &Backend{}
	dg := &pages.DataGrid{}
	dg.Name = "dg"
	if _, err := b.mapPageWidget(dg); err == nil {
		t.Error("legacy DataGrid should be rejected (no Pages$DataGrid in pg)")
	}
}

func TestMapDataViewSource_Database(t *testing.T) {
	ok, err := mapDataViewSource(&pages.DatabaseSource{EntityName: "Sales.Order"})
	if err != nil {
		t.Fatalf("database source: %v", err)
	}
	if ref, _ := ok["entityRef"].(map[string]any); ref["entity"] != "Sales.Order" {
		t.Fatalf("entityRef: %+v", ok)
	}
	// xpath constraint not supported yet
	if _, err := mapDataViewSource(&pages.DatabaseSource{EntityName: "Sales.Order", XPathConstraint: "[Total > 0]"}); err == nil {
		t.Error("database source with xpath should be rejected for now")
	}
}

func TestMapPageWidget_TabContainer(t *testing.T) {
	b := &Backend{}
	inner := &pages.DynamicText{AttributePath: "$currentObject/Name"}
	inner.Name = "t"
	tab := &pages.TabContainer{
		TabPages: []*pages.TabPage{
			{Name: "tab1", Caption: &model.Text{Translations: map[string]string{"en_US": "First"}}, Widgets: []pages.Widget{inner}},
			{Name: "tab2"}, // no caption -> defaults to the tab name
		},
	}
	tab.Name = "tabs1"
	m, err := b.mapPageWidget(tab)
	if err != nil || m["$Type"] != "Pages$TabContainer" || m["name"] != "tabs1" {
		t.Fatalf("tab container: %+v / %v", m, err)
	}
	tabs, _ := m["tabPages"].([]any)
	if len(tabs) != 2 {
		t.Fatalf("tab pages: %+v", tabs)
	}
	tp0, _ := tabs[0].(map[string]any)
	if tp0["$Type"] != "Pages$TabPage" || tp0["t:caption"] != "First" {
		t.Fatalf("tab page 0: %+v", tp0)
	}
	if kids, _ := tp0["widgets"].([]any); len(kids) != 1 {
		t.Fatalf("tab page 0 widgets: %+v", tp0["widgets"])
	}
	tp1, _ := tabs[1].(map[string]any)
	if tp1["t:caption"] != "tab2" {
		t.Fatalf("tab page 1 caption should default to name: %+v", tp1)
	}
}

func TestMapPageWidget_ConditionalVisibility(t *testing.T) {
	b := &Backend{}
	dt := &pages.DynamicText{
		Content: &pages.ClientTemplate{Template: &model.Text{Translations: map[string]string{"en_US": "hi"}}},
	}
	dt.Name = "t1"
	dt.ConditionalVisibility = &pages.ConditionalVisibilitySettings{Expression: "$currentObject/Active"}
	m, err := b.mapPageWidget(dt)
	if err != nil {
		t.Fatalf("dynamic text: %v", err)
	}
	cv, _ := m["conditionalVisibilitySettings"].(map[string]any)
	if cv == nil || cv["$Type"] != "Pages$ConditionalVisibilitySettings" || cv["expression"] != "$currentObject/Active" {
		t.Fatalf("conditionalVisibilitySettings: %+v", m["conditionalVisibilitySettings"])
	}

	// No VISIBLE IF -> no conditionalVisibilitySettings key emitted.
	dt2 := &pages.DynamicText{Content: &pages.ClientTemplate{Template: &model.Text{}}}
	dt2.Name = "t2"
	m2, _ := b.mapPageWidget(dt2)
	if _, ok := m2["conditionalVisibilitySettings"]; ok {
		t.Fatalf("unexpected conditionalVisibilitySettings on plain widget: %+v", m2)
	}
}

func TestMapPageWidget_Unsupported(t *testing.T) {
	b := &Backend{}
	sc := &pages.ScrollContainer{}
	sc.Name = "scroll"
	if _, err := b.mapPageWidget(sc); err == nil {
		t.Error("an unmapped widget type should error")
	}
}
