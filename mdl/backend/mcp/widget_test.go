// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/backend"
	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/pages"
)

// buildCombobox drives the widget builder the way the pluggable engine does for
// a combobox, then maps the resulting CustomWidget to its pg form.
func buildCombobox(t *testing.T, b *Backend, drive func(w *mcpWidgetBuilder)) map[string]any {
	t.Helper()
	wb, err := b.LoadWidgetTemplate(comboboxWidgetID, "")
	if err != nil {
		t.Fatalf("LoadWidgetTemplate: %v", err)
	}
	w := wb.(*mcpWidgetBuilder)
	drive(w)
	cw := w.Finalize(model.ID("cw1"), "cmb", "Label", "Always")
	m, err := b.mapPageWidget(cw)
	if err != nil {
		t.Fatalf("mapPageWidget: %v", err)
	}
	return m
}

func TestMapCustomWidget_EnumCombobox(t *testing.T) {
	b := &Backend{}
	m := buildCombobox(t, b, func(w *mcpWidgetBuilder) {
		// Enum mode mapping: attributeEnumeration <- Attribute.
		w.SetAttribute("attributeEnumeration", "PgTest.Order.status")
	})
	if m["$Type"] != "CustomWidgets$CustomWidget" || m["widgetId"] != comboboxWidgetID {
		t.Fatalf("custom widget: %+v", m)
	}
	ob, _ := m["object"].(map[string]any)
	// optionsSourceType must be inferred so pg keeps attributeEnumeration.
	if ob["optionsSourceType"] != "enumeration" {
		t.Fatalf("optionsSourceType not inferred: %+v", ob)
	}
	ar, _ := ob["attributeEnumeration"].(map[string]any)
	if ar["$Type"] != "DomainModels$AttributeRef" || ar["attribute"] != "PgTest.Order.status" {
		t.Fatalf("attributeEnumeration: %+v", ob["attributeEnumeration"])
	}
}

func TestMapCustomWidget_AssociationCombobox(t *testing.T) {
	b := &Backend{}
	m := buildCombobox(t, b, func(w *mcpWidgetBuilder) {
		// Association mode mappings.
		w.SetPrimitive("optionsSourceType", "association")
		w.SetDataSource("optionsSourceAssociationDataSource", &pages.DatabaseSource{EntityName: "PgTest.Customer"})
		w.SetAssociation("attributeAssociation", "PgTest.Order_Customer", "PgTest.Customer")
		w.SetAttribute("optionsSourceAssociationCaptionAttribute", "PgTest.Customer.Name")
	})
	ob, _ := m["object"].(map[string]any)
	if ob["optionsSourceType"] != "association" {
		t.Fatalf("optionsSourceType: %+v", ob)
	}
	ds, _ := ob["optionsSourceAssociationDataSource"].(map[string]any)
	if ds["$Type"] != "CustomWidgets$CustomWidgetXPathSource" {
		t.Fatalf("dataSource: %+v", ds)
	}
	assoc, _ := ob["attributeAssociation"].(map[string]any)
	steps, _ := assoc["steps"].([]any)
	step0, _ := steps[0].(map[string]any)
	if step0["association"] != "PgTest.Order_Customer" || step0["destinationEntity"] != "PgTest.Customer" {
		t.Fatalf("attributeAssociation step: %+v", step0)
	}
	cap, _ := ob["optionsSourceAssociationCaptionAttribute"].(map[string]any)
	if cap["attribute"] != "PgTest.Customer.Name" {
		t.Fatalf("captionAttribute: %+v", cap)
	}
}

// TestMapCustomWidget_RegistryWidgetAccepted pins Phase 1: a widget outside the
// built-in widgets.def.json hint table (here BarcodeScanner, which the engine
// resolves from the shared embedded registry) is no longer rejected on a whitelist
// — it is accepted and emitted, using only operations the MCP builder supports.
// Studio Pro expands the rest over pg_patch_page.
func TestMapCustomWidget_RegistryWidgetAccepted(t *testing.T) {
	const barcodeWidgetID = "com.mendix.widget.web.barcodescanner.BarcodeScanner"
	b := &Backend{}
	wb, _ := b.LoadWidgetTemplate(barcodeWidgetID, "")
	w := wb.(*mcpWidgetBuilder)
	w.SetAttribute("valueAttribute", "PgTest.Order.Barcode") // a supported op
	cw := w.Finalize(model.ID("bc1"), "bc", "", "Always")
	m, err := b.mapPageWidget(cw)
	if err != nil {
		t.Fatalf("registry-resolved widget should be accepted, got: %v", err)
	}
	if m["widgetId"] != barcodeWidgetID {
		t.Fatalf("widgetId: %+v", m)
	}
	ob, _ := m["object"].(map[string]any)
	ar, _ := ob["valueAttribute"].(map[string]any)
	if ar["attribute"] != "PgTest.Order.Barcode" {
		t.Fatalf("valueAttribute not emitted: %+v", ob)
	}
}

func TestMapCustomWidget_UnsupportedPropertyRejected(t *testing.T) {
	b := &Backend{}
	wb, _ := b.LoadWidgetTemplate(comboboxWidgetID, "")
	w := wb.(*mcpWidgetBuilder)
	w.SetAttribute("attributeEnumeration", "M.E.Status")
	// A save action has no verified pg shape inside a custom-widget object, so
	// it must record an unsupported op and reject the widget loudly.
	w.SetAction("onChange", &pages.SaveChangesClientAction{})
	cw := w.Finalize(model.ID("cw2"), "cmb", "", "Always")
	if _, err := b.mapPageWidget(cw); err == nil {
		t.Error("combobox using an unsupported property op should be rejected")
	}
}

func TestMapCustomWidget_ActionWithParameterMappingsRejected(t *testing.T) {
	b := &Backend{}
	wb, _ := b.LoadWidgetTemplate(comboboxWidgetID, "")
	w := wb.(*mcpWidgetBuilder)
	w.SetAction("onChange", &pages.MicroflowClientAction{
		MicroflowName:     "M.MF",
		ParameterMappings: []*pages.MicroflowParameterMapping{{ParameterName: "P"}},
	})
	cw := w.Finalize(model.ID("cw4"), "cmb", "", "Always")
	if _, err := b.mapPageWidget(cw); err == nil {
		t.Error("action with parameter mappings should be rejected, not emitted with mappings dropped")
	}
}

func TestMapCustomWidget_Phase2Operations(t *testing.T) {
	// Pins the pg shapes verified live against Studio Pro 11.12 (Phase 2 of
	// PROPOSAL_mcp_pluggable_widget_authoring.md): texttemplate → ct:-prefixed
	// plain string, expression → plain string, microflow action → nested
	// microflowSettings (a flat `microflow` key is silently dropped by pg).
	const imageWidgetID = "com.mendix.widget.web.image.Image"
	b := &Backend{}
	wb, _ := b.LoadWidgetTemplate(imageWidgetID, "")
	w := wb.(*mcpWidgetBuilder)
	w.SetPrimitive("datasource", "imageUrl")
	w.SetTextTemplate("imageUrl", "https://example.com/x.png")
	w.SetTextTemplateWithParams("alternativeText", "Image of {Name}", "M.Thing")
	w.SetExpression("visibleExpression", "$currentObject/Name != empty")
	w.SetAction("onClick", &pages.MicroflowClientAction{MicroflowName: "M.MF"})
	cw := w.Finalize(model.ID("cw3"), "img", "", "Always")
	m, err := b.mapPageWidget(cw)
	if err != nil {
		t.Fatalf("phase 2 operations should be accepted, got: %v", err)
	}
	ob, _ := m["object"].(map[string]any)
	if ob["ct:imageUrl"] != "https://example.com/x.png" {
		t.Errorf("plain texttemplate should be a ct:-prefixed string: %+v", ob["ct:imageUrl"])
	}
	tmpl, _ := ob["ct:alternativeText"].(map[string]any)
	if tmpl["t:template"] != "Image of {1}" {
		t.Errorf("parameterised template text not rewritten: %+v", tmpl)
	}
	params, _ := tmpl["parameters"].([]any)
	if len(params) != 1 {
		t.Fatalf("expected 1 template parameter: %+v", tmpl)
	}
	p0, _ := params[0].(map[string]any)
	ar, _ := p0["attributeRef"].(map[string]any)
	if ar["attribute"] != "M.Thing.Name" {
		t.Errorf("template parameter attribute not resolved against entity context: %+v", p0)
	}
	if ob["visibleExpression"] != "$currentObject/Name != empty" {
		t.Errorf("expression should be a plain string: %+v", ob["visibleExpression"])
	}
	act, _ := ob["onClick"].(map[string]any)
	if act["$Type"] != "Pages$MicroflowClientAction" {
		t.Fatalf("onClick: %+v", act)
	}
	if _, flat := act["microflow"]; flat {
		t.Error("flat microflow key is silently dropped by pg; must nest in microflowSettings")
	}
	ms, _ := act["microflowSettings"].(map[string]any)
	if ms["microflow"] != "M.MF" {
		t.Errorf("microflow reference must nest in microflowSettings: %+v", act)
	}
}

func TestSetObjectList_DataGridColumns(t *testing.T) {
	b := &Backend{}
	wb, _ := b.LoadWidgetTemplate("com.mendix.widget.web.datagrid.Datagrid", "")
	w := wb.(*mcpWidgetBuilder)
	// The shared engine calls SetDataSource (via auto-datasource) and SetObjectList.
	w.SetDataSource("datasource", &pages.DatabaseSource{EntityName: "PgTest.Order"})
	w.SetObjectList("columns", []backend.ObjectListItemSpec{
		{Properties: []backend.ObjectListItemProperty{
			{PropertyKey: "attribute", Operation: "attribute", AttributePath: "PgTest.Order.OrderNumber"},
			{PropertyKey: "header", Operation: "texttemplate", TextTemplate: "Order #"},
			{PropertyKey: "showContentAs", Operation: "primitive", PrimitiveVal: "attribute"},
		}},
	})
	cw := w.Finalize(model.ID("dg1"), "dg", "", "Always")
	m, err := b.mapPageWidget(cw)
	if err != nil {
		t.Fatalf("mapPageWidget: %v", err)
	}
	ob, _ := m["object"].(map[string]any)
	ds, _ := ob["datasource"].(map[string]any)
	if ds["$Type"] != "CustomWidgets$CustomWidgetXPathSource" {
		t.Fatalf("datasource not set via PropertyTypeIDs/auto-datasource: %+v", ob["datasource"])
	}
	cols, _ := ob["columns"].([]any)
	if len(cols) != 1 {
		t.Fatalf("columns: %+v", cols)
	}
	col, _ := cols[0].(map[string]any)
	if col["$Type"] != "CustomWidgets$WidgetObject" || col["ct:header"] != "Order #" || col["showContentAs"] != "attribute" {
		t.Fatalf("column: %+v", col)
	}
	ar, _ := col["attribute"].(map[string]any)
	if ar["attribute"] != "PgTest.Order.OrderNumber" {
		t.Fatalf("column attribute: %+v", col["attribute"])
	}
}

func TestCustomWidgetXPathSource_Sort(t *testing.T) {
	src := customWidgetXPathSource(&pages.DatabaseSource{
		EntityName: "PgTest.Order",
		Sorting: []*pages.GridSort{
			{AttributePath: "PgTest.Order.OrderNumber", Direction: pages.SortDirectionAscending},
			{AttributePath: "PgTest.Order.OrderDate", Direction: pages.SortDirectionDescending},
		},
	})
	sb, _ := src["sortBar"].(map[string]any)
	if sb["$Type"] != "Pages$GridSortBar" {
		t.Fatalf("sortBar: %+v", src["sortBar"])
	}
	items, _ := sb["sortItems"].([]any)
	if len(items) != 2 {
		t.Fatalf("sortItems: %+v", items)
	}
	i0, _ := items[0].(map[string]any)
	if ar, _ := i0["attributeRef"].(map[string]any); ar["attribute"] != "PgTest.Order.OrderNumber" || i0["sortDirection"] != "Ascending" {
		t.Fatalf("sortItem[0]: %+v", i0)
	}
	i1, _ := items[1].(map[string]any)
	if i1["sortDirection"] != "Descending" {
		t.Fatalf("sortItem[1] direction: %+v", i1)
	}
	// No sort -> no sortBar key (pg fills an empty default).
	plain := customWidgetXPathSource(&pages.DatabaseSource{EntityName: "PgTest.Order"})
	if _, ok := plain["sortBar"]; ok {
		t.Fatalf("unexpected sortBar on unsorted source: %+v", plain)
	}
}

func TestCustomWidgetXPathSource_Constraint(t *testing.T) {
	src := customWidgetXPathSource(&pages.DatabaseSource{
		EntityName:      "PgTest.Order",
		XPathConstraint: "[OrderNumber > 100]",
	})
	if src["xPathConstraint"] != "[OrderNumber > 100]" {
		t.Fatalf("xPathConstraint: %+v", src["xPathConstraint"])
	}
	// No constraint -> no key.
	plain := customWidgetXPathSource(&pages.DatabaseSource{EntityName: "PgTest.Order"})
	if _, ok := plain["xPathConstraint"]; ok {
		t.Fatalf("unexpected xPathConstraint on unconstrained source: %+v", plain)
	}
}

func TestMapDataViewSource_ConstraintRejected(t *testing.T) {
	// A DataView has no XPath source type in the official metamodel (DataViewSource
	// is context-only), so a constraint on a data-view database source is rejected.
	_, err := mapDataViewSource(&pages.DatabaseSource{EntityName: "Sales.Order", XPathConstraint: "[Total > 0]"})
	if err == nil {
		t.Error("data-view database source with an XPath constraint should be rejected")
	}
}

func TestMapListViewSource_DatabaseConstraintAndSort(t *testing.T) {
	// A list-view database source uses Pages$ListViewXPathSource, which carries
	// both an xPathConstraint and a sortBar (per the official metamodel).
	src, err := mapListViewSource(&pages.DatabaseSource{
		EntityName:      "PgTest.Order",
		XPathConstraint: "[OrderNumber > 50]",
		Sorting:         []*pages.GridSort{{AttributePath: "PgTest.Order.OrderNumber", Direction: pages.SortDirectionDescending}},
	})
	if err != nil || src["$Type"] != "Pages$ListViewXPathSource" {
		t.Fatalf("list view source: %+v / %v", src, err)
	}
	if src["xPathConstraint"] != "[OrderNumber > 50]" {
		t.Fatalf("xPathConstraint: %+v", src["xPathConstraint"])
	}
	sb, _ := src["sortBar"].(map[string]any)
	if items, _ := sb["sortItems"].([]any); len(items) != 1 {
		t.Fatalf("sortBar: %+v", src["sortBar"])
	}
	// A microflow source on a list view falls through to the shared mapping.
	mf, err := mapListViewSource(&pages.MicroflowSource{Microflow: "M.DSO_X"})
	if err != nil || mf["$Type"] != "Pages$MicroflowSource" {
		t.Fatalf("microflow list source: %+v / %v", mf, err)
	}
}

func TestMapCustomWidget_Gallery(t *testing.T) {
	b := &Backend{}
	wb, _ := b.LoadWidgetTemplate("com.mendix.widget.web.gallery.Gallery", "")
	w := wb.(*mcpWidgetBuilder)
	// Gallery's def.json maps datasource explicitly and delivers the template
	// body via SetChildWidgets(content, ...).
	w.SetDataSource("datasource", &pages.DatabaseSource{EntityName: "PgTest.Order"})
	w.SetSelection("itemSelection", "Multi")
	row := &pages.DynamicText{Content: &pages.ClientTemplate{
		Template:   &model.Text{Translations: map[string]string{"en_US": "{1}"}},
		Parameters: []*pages.ClientTemplateParameter{{AttributeRef: "PgTest.Order.OrderNumber"}},
	}}
	row.Name = "dtNum"
	w.SetChildWidgets("content", []pages.Widget{row})
	cw := w.Finalize(model.ID("g1"), "gal", "", "Always")

	m, err := b.mapPageWidget(cw)
	if err != nil {
		t.Fatalf("mapPageWidget: %v", err)
	}
	ob, _ := m["object"].(map[string]any)
	if ob["itemSelection"] != "Multi" {
		t.Fatalf("itemSelection: %+v", ob["itemSelection"])
	}
	if ds, _ := ob["datasource"].(map[string]any); ds["$Type"] != "CustomWidgets$CustomWidgetXPathSource" {
		t.Fatalf("datasource: %+v", ob["datasource"])
	}
	content, _ := ob["content"].([]any)
	if len(content) != 1 {
		t.Fatalf("content widgets: %+v", content)
	}
	dt, _ := content[0].(map[string]any)
	if dt["$Type"] != "Pages$DynamicText" {
		t.Fatalf("content[0]: %+v", dt)
	}
	// The "{1}" binding must survive as a full ClientTemplate with a parameter.
	ct, _ := dt["ct:content"].(map[string]any)
	params, _ := ct["parameters"].([]any)
	if len(params) != 1 {
		t.Fatalf("template parameters dropped: %+v", dt["ct:content"])
	}
	p0, _ := params[0].(map[string]any)
	ar, _ := p0["attributeRef"].(map[string]any)
	if ar["attribute"] != "PgTest.Order.OrderNumber" {
		t.Fatalf("param attributeRef: %+v", p0)
	}
}

func TestSetObjectList_ColumnFilterAndCustomContent(t *testing.T) {
	b := &Backend{}
	wb, _ := b.LoadWidgetTemplate("com.mendix.widget.web.datagrid.Datagrid", "")
	w := wb.(*mcpWidgetBuilder)

	// A filter widget is itself a pluggable CustomWidget the engine builds and
	// registers; mapping the column's `filter` slot must recurse into it.
	filterWB, _ := b.LoadWidgetTemplate("com.mendix.widget.web.datagridtextfilter.DatagridTextFilter", "")
	filterW := filterWB.(*mcpWidgetBuilder)
	filterW.SetPrimitive("attrChoice", "auto")
	filter := filterW.Finalize(model.ID("tf1"), "tf", "", "Always")

	// A custom-content cell widget in the column's `content` slot.
	cell := &pages.DynamicText{Content: &pages.ClientTemplate{Template: &model.Text{Translations: map[string]string{"en_US": "x"}}}}
	cell.Name = "cell"

	w.SetObjectList("columns", []backend.ObjectListItemSpec{
		{
			Properties:   []backend.ObjectListItemProperty{{PropertyKey: "attribute", Operation: "attribute", AttributePath: "PgTest.Order.OrderNumber"}},
			ChildWidgets: map[string][]pages.Widget{"filter": {filter}, "content": {cell}},
		},
	})
	cw := w.Finalize(model.ID("dg2"), "dg", "", "Always")
	m, err := b.mapPageWidget(cw)
	if err != nil {
		t.Fatalf("mapPageWidget: %v", err)
	}
	cols, _ := m["object"].(map[string]any)["columns"].([]any)
	col, _ := cols[0].(map[string]any)
	fl, _ := col["filter"].([]any)
	if len(fl) != 1 {
		t.Fatalf("column filter slot: %+v", col["filter"])
	}
	f0, _ := fl[0].(map[string]any)
	if f0["widgetId"] != "com.mendix.widget.web.datagridtextfilter.DatagridTextFilter" {
		t.Fatalf("filter widget: %+v", f0)
	}
	if cc, _ := col["content"].([]any); len(cc) != 1 {
		t.Fatalf("column content slot: %+v", col["content"])
	}
}
