// SPDX-License-Identifier: Apache-2.0

package widgetobj

import (
	"testing"

	"go.mongodb.org/mongo-driver/bson"

	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// TestSetAttributeRefField locks in the BSON shape for the
// pluggable-widget engine's `attribute` operation (object-list item
// sub-properties — e.g. `column colName (Attribute: NId)` inside a
// pluggablewidget 'com.mendix.widget.web.datagrid.Datagrid' block).
//
// Regression for #64: an earlier implementation wrote `$Type:
// "CustomWidgets$AttributeRef"` and only the bare attribute name. Mendix's
// type cache has no such type, so `mx update-widgets` / `mx check` failed
// with TypeCacheUnknownTypeException and Studio Pro flagged CE0463.
func TestSetAttributeRefField(t *testing.T) {
	t.Run("fully-qualified path -> DomainModels$AttributeRef", func(t *testing.T) {
		value := bson.D{
			{Key: "AttributeRef", Value: nil},
			{Key: "PrimitiveValue", Value: ""},
		}
		got := setAttributeRefField(value, "ReproUoM.UoM.NId", nil)

		ref := findField(t, got, "AttributeRef")
		refDoc, ok := ref.(bson.D)
		if !ok {
			t.Fatalf("AttributeRef value is %T, want bson.D", ref)
		}

		gotType := findField(t, refDoc, "$Type")
		if gotType != "DomainModels$AttributeRef" {
			t.Errorf("$Type = %q, want DomainModels$AttributeRef (CustomWidgets$AttributeRef is not registered in Mendix's type cache)", gotType)
		}

		gotAttr := findField(t, refDoc, "Attribute")
		if gotAttr != "ReproUoM.UoM.NId" {
			t.Errorf("Attribute = %q, want fully-qualified ReproUoM.UoM.NId", gotAttr)
		}

		gotEntityRef := findField(t, refDoc, "EntityRef")
		if gotEntityRef != nil {
			t.Errorf("EntityRef = %v, want nil", gotEntityRef)
		}
	})

	t.Run("unqualified path -> AttributeRef nil", func(t *testing.T) {
		value := bson.D{{Key: "AttributeRef", Value: bson.D{{Key: "stale", Value: 1}}}}
		got := setAttributeRefField(value, "NId", nil)
		if findField(t, got, "AttributeRef") != nil {
			t.Errorf("AttributeRef must be nil for unqualified path; got %v", findField(t, got, "AttributeRef"))
		}
	})
}

func findField(t *testing.T, doc bson.D, key string) any {
	t.Helper()
	for _, e := range doc {
		if e.Key == key {
			return e.Value
		}
	}
	t.Fatalf("field %q not found in BSON doc", key)
	return nil
}

// TestDetectObjectListItemKind covers the heuristic used to classify an
// object-list item as attribute-bound, custom-content, or default. Mirrors
// the keyword path's hasCustomContent logic.
func TestDetectObjectListItemKind(t *testing.T) {
	tests := []struct {
		name         string
		spec         map[string]backend.ObjectListItemProperty
		childWidgets map[string][]pages.Widget
		want         objectListItemKind
	}{
		{
			name: "attribute set, no children → attribute kind",
			spec: map[string]backend.ObjectListItemProperty{
				"attribute": {AttributePath: "Mod.Ent.Attr"},
			},
			childWidgets: nil,
			want:         itemKindAttribute,
		},
		{
			name:         "content slot widgets present → customcontent kind",
			spec:         nil,
			childWidgets: map[string][]pages.Widget{"content": {nil}},
			want:         itemKindCustomContent,
		},
		{
			name: "filter widget present + attribute set → still attribute kind",
			// Filter widgets are sidecars to attribute columns; they don't
			// make the column custom-content.
			spec: map[string]backend.ObjectListItemProperty{
				"attribute": {AttributePath: "Mod.Ent.Attr"},
			},
			childWidgets: map[string][]pages.Widget{"filter": {nil}},
			want:         itemKindAttribute,
		},
		{
			name:         "neither → default",
			spec:         nil,
			childWidgets: nil,
			want:         itemKindDefault,
		},
		{
			name: "empty AttributePath → default",
			spec: map[string]backend.ObjectListItemProperty{
				"attribute": {AttributePath: ""},
			},
			want: itemKindDefault,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := detectObjectListItemKind(tc.spec, tc.childWidgets)
			if got != tc.want {
				t.Errorf("detectObjectListItemKind() = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestShouldEmitEmptyClientTemplate locks in the per-widget Studio Pro
// convention for unset TextTemplate properties on DataGrid columns
// (verified against Cars_Overview in c3d61af1).
func TestShouldEmitEmptyClientTemplate(t *testing.T) {
	const datagrid = "com.mendix.widget.web.datagrid.Datagrid"
	tests := []struct {
		name     string
		widgetID string
		listKey  string
		propKey  string
		kind     objectListItemKind
		want     bool
	}{
		{"DataGrid attribute col tooltip → empty", datagrid, "columns", "tooltip", itemKindAttribute, true},
		{"DataGrid attribute col exportValue → null", datagrid, "columns", "exportValue", itemKindAttribute, false},
		{"DataGrid attribute col dynamicText → null", datagrid, "columns", "dynamicText", itemKindAttribute, false},
		{"DataGrid custom-content col tooltip → null", datagrid, "columns", "tooltip", itemKindCustomContent, false},
		{"DataGrid custom-content col exportValue → empty", datagrid, "columns", "exportValue", itemKindCustomContent, true},
		{"DataGrid custom-content col dynamicText → null", datagrid, "columns", "dynamicText", itemKindCustomContent, false},
		{"Unknown widget → null", "com.example.Other", "columns", "tooltip", itemKindAttribute, false},
		{"Unknown list → null", datagrid, "rows", "tooltip", itemKindAttribute, false},
		{"Unknown kind → null", datagrid, "columns", "tooltip", itemKindDefault, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := shouldEmitEmptyClientTemplate(tc.widgetID, tc.listKey, tc.propKey, tc.kind)
			if got != tc.want {
				t.Errorf("shouldEmitEmptyClientTemplate(%q, %q, %q, %q) = %v, want %v",
					tc.widgetID, tc.listKey, tc.propKey, tc.kind, got, tc.want)
			}
		})
	}
}

// (TestApplyColumnHeaderFallback covers the helper in widget_engine.go; the
// unit test lives in the executor package — see widget_defs_test.go.)

// Bug 7 — a column bound to an ASSOCIATED attribute (Assoc/Attr) must serialize
// its AttributeRef with an EntityRef (IndirectEntityRef of association steps),
// matching the DynamicText contentparam / Studio Pro structure. Without it the
// column has a flat attribute path and mxbuild fails CE1613.
func TestSetAttributeRefField_AssociationSteps(t *testing.T) {
	value := bson.D{{Key: "AttributeRef", Value: nil}}
	steps := []pages.AttributeRefStep{
		{Association: "Sales.Order_Customer", DestinationEntity: "Sales.Customer"},
	}
	got := setAttributeRefField(value, "Sales.Customer.Name", steps)

	refDoc, ok := findField(t, got, "AttributeRef").(bson.D)
	if !ok {
		t.Fatalf("AttributeRef not a document")
	}
	if a := findField(t, refDoc, "Attribute"); a != "Sales.Customer.Name" {
		t.Errorf("Attribute = %q, want Sales.Customer.Name", a)
	}
	entityRef, ok := findField(t, refDoc, "EntityRef").(bson.D)
	if !ok {
		t.Fatalf("EntityRef not a document (got %T) — association steps dropped", findField(t, refDoc, "EntityRef"))
	}
	if tp := findField(t, entityRef, "$Type"); tp != "DomainModels$IndirectEntityRef" {
		t.Errorf("EntityRef.$Type = %q, want DomainModels$IndirectEntityRef", tp)
	}
	stepsArr, ok := findField(t, entityRef, "Steps").(bson.A)
	if !ok || len(stepsArr) < 2 {
		t.Fatalf("Steps not a marker-prefixed array: %#v", findField(t, entityRef, "Steps"))
	}
	step, ok := stepsArr[1].(bson.D)
	if !ok {
		t.Fatalf("step[0] not a document")
	}
	if a := findField(t, step, "Association"); a != "Sales.Order_Customer" {
		t.Errorf("step.Association = %q, want Sales.Order_Customer", a)
	}
	if de := findField(t, step, "DestinationEntity"); de != "Sales.Customer" {
		t.Errorf("step.DestinationEntity = %q, want Sales.Customer", de)
	}
}
