// SPDX-License-Identifier: Apache-2.0

package widgets

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/modelsdk/widgets/mpk"
)

// TestReconcileEnumValues verifies that augment rebuilds an enum property's
// option set from the .mpk, replacing stale members carried by an embedded
// template extracted from a different widget version. Confirmed empirically that
// a stale enum option triggers CE0463 (Gallery pagingPosition {top,bottom,both}
// vs installed 3.0.1 {above,below}); this locks in the fix.
func TestReconcileEnumValues(t *testing.T) {
	enumVals := func(vt map[string]any) []string {
		var out []string
		for _, e := range vt["EnumerationValues"].([]any) {
			if m, ok := e.(map[string]any); ok {
				out = append(out, m["_Key"].(string))
			}
		}
		return out
	}
	mkType := func() map[string]any {
		return map[string]any{
			"ObjectType": map[string]any{
				"PropertyTypes": []any{float64(2),
					map[string]any{
						"$Type":       "CustomWidgets$WidgetPropertyType",
						"PropertyKey": "pagingPosition",
						"ValueType": map[string]any{
							"Type": "Enumeration",
							"EnumerationValues": []any{float64(2),
								map[string]any{"$Type": "CustomWidgets$WidgetEnumerationValue", "_Key": "top", "Caption": "Top"},
								map[string]any{"$Type": "CustomWidgets$WidgetEnumerationValue", "_Key": "both", "Caption": "Both"},
							},
						},
					},
				},
			},
		}
	}

	def := &mpk.WidgetDefinition{Properties: []mpk.PropertyDef{
		{Key: "pagingPosition", Type: "enumeration", EnumValues: []mpk.EnumValue{
			{Key: "below", Caption: "Below grid"},
			{Key: "above", Caption: "Above grid"},
		}},
	}}

	typ := mkType()
	reconcileEnumValues(typ, mpkEnumValuesByKey(def))

	vt := typ["ObjectType"].(map[string]any)["PropertyTypes"].([]any)[1].(map[string]any)["ValueType"].(map[string]any)
	got := enumVals(vt)
	want := []string{"below", "above"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("reconciled enum = %v, want %v (stale top/both should be replaced by the .mpk options)", got, want)
	}
}
