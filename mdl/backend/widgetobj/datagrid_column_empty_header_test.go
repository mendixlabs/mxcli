// SPDX-License-Identifier: Apache-2.0

// ChipCoV8 #11: a Data grid 2 column with no caption, no attribute and no name
// had its `header` serialized as TextTemplate: null. `mx update-widgets` writes
// an empty Forms$ClientTemplate there, so `mx check` reported CE0463. The
// executor flags such a header EmptyTemplate; this test proves the flag
// reaches the encoded document through buildObjectListItemBSON.
package widgetobj

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/pages"
	"go.mongodb.org/mongo-driver/v2/bson"
)

const (
	dgColumnHeaderID = "00000000000000000000000000000011"
	dgColumnShowID   = "00000000000000000000000000000012"
)

func dgColumnEntry() pages.PropertyTypeIDEntry {
	return pages.PropertyTypeIDEntry{
		PropertyTypeID: "00000000000000000000000000000010",
		NestedKeyOrder: []string{"showContentAs", "header"},
		NestedPropertyIDs: map[string]pages.PropertyTypeIDEntry{
			"showContentAs": {PropertyTypeID: dgColumnShowID, ValueType: "Enumeration", DefaultValue: "attribute"},
			// The column header is an optional TextTemplate with no default.
			"header": {PropertyTypeID: dgColumnHeaderID, ValueType: "TextTemplate"},
		},
	}
}

func headerTemplate(t *testing.T, spec backend.ObjectListItemSpec) any {
	t.Helper()
	got := buildObjectListItemBSON("com.mendix.widget.web.datagrid.Datagrid", "columns", dgColumnEntry(), spec)
	return templateOf(t, got, dgColumnHeaderID)
}

func TestDataGridColumn_EmptyTemplateHeaderEncodesEmptyClientTemplate(t *testing.T) {
	spec := backend.ObjectListItemSpec{Properties: []backend.ObjectListItemProperty{
		{PropertyKey: "showContentAs", Operation: "primitive", PrimitiveVal: "customContent"},
		{PropertyKey: "header", Operation: "texttemplate", EmptyTemplate: true},
	}}
	tt, ok := headerTemplate(t, spec).(bson.D)
	if !ok {
		t.Fatalf("header TextTemplate = %v, want an empty Forms$ClientTemplate (null is CE0463)", headerTemplate(t, spec))
	}
	m := map[string]any{}
	for _, e := range tt {
		m[e.Key] = e.Value
	}
	if m["$Type"] != "Forms$ClientTemplate" {
		t.Fatalf("header TextTemplate $Type = %v, want Forms$ClientTemplate", m["$Type"])
	}
	// update-widgets' exact shape: no params, no translations in either text.
	if p, _ := m["Parameters"].(bson.A); len(p) != 1 {
		t.Errorf("Parameters = %v, want [2] only", m["Parameters"])
	}
	for _, k := range []string{"Template", "Fallback"} {
		var items bson.A
		for _, e := range m[k].(bson.D) {
			if e.Key == "Items" {
				items, _ = e.Value.(bson.A)
			}
		}
		if len(items) != 1 {
			t.Errorf("%s.Items = %v, want [3] only (no translations)", k, items)
		}
	}

	// Control: the same header without the flag stays null — the pre-fix
	// output, and the convention for an unflagged optional TextTemplate.
	spec.Properties[1].EmptyTemplate = false
	if tt := headerTemplate(t, spec); tt != nil {
		t.Errorf("unflagged empty header = %v, want null", tt)
	}
}

// A column header the statement leaves empty is written as ONE empty
// translation in the authoring language, not as a template with none. With none,
// `create or replace` carried the stored caption back (canon.CarryTranslations
// adds every language a rebuilt text did not write), so `Caption: ”` could
// never clear 'Triage'.
func TestDataGridColumn_EmptyTextHeaderWritesAnEmptyTranslation(t *testing.T) {
	spec := backend.ObjectListItemSpec{Properties: []backend.ObjectListItemProperty{
		{PropertyKey: "showContentAs", Operation: "primitive", PrimitiveVal: "customContent"},
		{PropertyKey: "header", Operation: "texttemplate", EmptyText: true},
	}}
	tt, ok := headerTemplate(t, spec).(bson.D)
	if !ok {
		t.Fatalf("header TextTemplate = %v, want a Forms$ClientTemplate", headerTemplate(t, spec))
	}
	var template bson.D
	for _, e := range tt {
		if e.Key == "Template" {
			template, _ = e.Value.(bson.D)
		}
	}
	var items bson.A
	for _, e := range template {
		if e.Key == "Items" {
			items, _ = e.Value.(bson.A)
		}
	}
	if len(items) != 2 {
		t.Fatalf("Template items = %v, want the list marker and one translation", items)
	}
	tr, _ := items[1].(bson.D)
	m := map[string]any{}
	for _, e := range tr {
		m[e.Key] = e.Value
	}
	if m["LanguageCode"] != model.AuthoringLanguage() || m["Text"] != "" {
		t.Errorf("translation = %v, want (%s, \"\")", m, model.AuthoringLanguage())
	}
}
