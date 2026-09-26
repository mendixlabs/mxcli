// SPDX-License-Identifier: Apache-2.0

package widgets

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/modelsdk/widgets/mpk"
)

// actionVariablesOf returns a ValueType's ActionVariables entries, without the
// leading list marker, failing when the marker is not the 2 Studio Pro writes.
func actionVariablesOf(t *testing.T, vt map[string]any) []map[string]any {
	t.Helper()
	arr, ok := vt["ActionVariables"].([]any)
	if !ok || len(arr) == 0 || arr[0] != float64(2) {
		t.Fatalf("ActionVariables = %#v, want a list led by marker 2", vt["ActionVariables"])
	}
	var out []map[string]any
	for _, e := range arr[1:] {
		out = append(out, e.(map[string]any))
	}
	return out
}

// mendixlabs/mxcli#1200: an action's declared variables were written as an
// empty list, so the widget Type disagreed with its package and mx check
// reported CE0463 — even with no action configured. The shape is the one in
// Studio Pro's own Combobox template (templates/mendix-11.6/combobox.json):
// CustomWidgets$WidgetActionVariable with Caption, Key and Type.
func TestValueTypeCarriesActionVariablesFromTheWidgetXML(t *testing.T) {
	p := mpk.PropertyDef{
		Key:  "onChangeFilterInputEvent",
		Type: "action",
		ActionVariables: []mpk.ActionVariable{
			{Key: "filterInput", Type: "String", Caption: "Filter Input"},
		},
	}
	got := actionVariablesOf(t, createDefaultValueType("vt-1", "Action", p))
	if len(got) != 1 {
		t.Fatalf("ActionVariables entries = %d, want 1: %#v", len(got), got)
	}
	e := got[0]
	for k, want := range map[string]any{
		"$Type":   "CustomWidgets$WidgetActionVariable",
		"Key":     "filterInput",
		"Type":    "String",
		"Caption": "Filter Input",
	} {
		if e[k] != want {
			t.Errorf("entry %s = %v, want %v", k, e[k], want)
		}
	}
	if id, _ := e["$ID"].(string); id == "" {
		t.Error("entry has no $ID")
	}

	// A property declaring none keeps the empty list.
	if n := len(actionVariablesOf(t, createDefaultValueType("vt-2", "Action", mpk.PropertyDef{Key: "a", Type: "action"}))); n != 0 {
		t.Errorf("no variables declared, got %d entries", n)
	}
}

// A widget WITH an embedded template goes through reconcile rather than
// generation. A stale empty list there — or one from an older widget version —
// must be brought in line with the installed package.
func TestReconcileFillsActionVariables(t *testing.T) {
	tmpl := &WidgetTemplate{
		Type: map[string]any{"ObjectType": map[string]any{"PropertyTypes": []any{float64(2),
			map[string]any{"$Type": "CustomWidgets$WidgetPropertyType", "$ID": "pt1", "PropertyKey": "onSign",
				"ValueType": map[string]any{"$Type": "CustomWidgets$WidgetValueType", "Type": "Action",
					"ActionVariables": []any{float64(2)}}},
		}}},
	}
	byKey := map[string]mpk.PropertyDef{"onSign": {Key: "onSign", Type: "action",
		ActionVariables: []mpk.ActionVariable{{Key: "signature", Type: "String", Caption: "Signature"}}}}
	reconcileValueTypesFromMPK(tmpl, byKey)

	vt := tmpl.Type["ObjectType"].(map[string]any)["PropertyTypes"].([]any)[1].(map[string]any)["ValueType"].(map[string]any)
	got := actionVariablesOf(t, vt)
	if len(got) != 1 || got[0]["Key"] != "signature" {
		t.Errorf("reconciled ActionVariables = %#v, want the package's one variable", got)
	}
}

// End to end on the real Combobox package, generated WITHOUT its embedded
// template — the path a widget like Signature or Calendar always takes.
func TestGenerateFromMPK_CarriesComboboxActionVariable(t *testing.T) {
	const path = "../../testdata/expr-checker/widgets/com.mendix.widget.web.Combobox.mpk"
	if _, err := os.Stat(path); err != nil {
		t.Skipf("fixture not present: %v", err)
	}
	def, err := mpk.ParseMPKForWidget(path, "com.mendix.widget.web.combobox.Combobox")
	if err != nil || def == nil {
		t.Fatalf("parse: %v", err)
	}
	tmpl := GenerateFromMPK(def)
	var found map[string]any
	var walk func(any)
	walk = func(n any) {
		switch v := n.(type) {
		case map[string]any:
			if v["PropertyKey"] == "onChangeFilterInputEvent" {
				found, _ = v["ValueType"].(map[string]any)
			}
			for _, x := range v {
				walk(x)
			}
		case []any:
			for _, x := range v {
				walk(x)
			}
		}
	}
	walk(tmpl.Type)
	if found == nil {
		t.Fatal("onChangeFilterInputEvent not generated")
	}
	got := actionVariablesOf(t, found)
	if len(got) != 1 || got[0]["Key"] != "filterInput" || got[0]["Type"] != "String" || got[0]["Caption"] != "Filter Input" {
		t.Errorf("generated ActionVariables = %#v, want [{filterInput String \"Filter Input\"}]", got)
	}
}

// Control: Studio Pro's own Combobox template already carries the package's
// action variables. Augmenting it with the matching .mpk must leave every
// ActionVariables list exactly as it was — same entries, same $IDs — or the fix
// would churn a template that was never wrong.
func TestAugment_AgreeingTemplateKeepsItsActionVariables(t *testing.T) {
	const path = "../../testdata/expr-checker/widgets/com.mendix.widget.web.Combobox.mpk"
	if _, err := os.Stat(path); err != nil {
		t.Skipf("fixture not present: %v", err)
	}
	const id = "com.mendix.widget.web.combobox.Combobox"
	def, err := mpk.ParseMPKForWidget(path, id)
	if err != nil || def == nil {
		t.Fatalf("parse: %v", err)
	}
	cached, err := GetTemplate(id)
	if err != nil || cached == nil {
		t.Fatalf("embedded template: %v", err)
	}
	tmpl, err := deepCloneTemplate(cached)
	if err != nil {
		t.Fatal(err)
	}
	collect := func(tm *WidgetTemplate) map[string]string {
		out := map[string]string{}
		var walk func(any)
		walk = func(n any) {
			switch v := n.(type) {
			case map[string]any:
				if key, _ := v["PropertyKey"].(string); key != "" {
					if vt, ok := v["ValueType"].(map[string]any); ok {
						b, _ := json.Marshal(vt["ActionVariables"])
						out[key] = string(b)
					}
				}
				for _, x := range v {
					walk(x)
				}
			case []any:
				for _, x := range v {
					walk(x)
				}
			}
		}
		walk(tm.Type)
		return out
	}
	before := collect(tmpl)
	if !strings.Contains(before["onChangeFilterInputEvent"], "filterInput") {
		t.Fatalf("precondition: embedded template should carry filterInput, has %s", before["onChangeFilterInputEvent"])
	}
	if err := AugmentTemplate(tmpl, def); err != nil {
		t.Fatalf("augment: %v", err)
	}
	after := collect(tmpl)
	for key, b := range before {
		if after[key] != b {
			t.Errorf("%s ActionVariables changed:\n before %s\n after  %s", key, b, after[key])
		}
	}
}
