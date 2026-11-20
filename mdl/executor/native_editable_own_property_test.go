// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// The native Slider, Range slider, Rating and Toggle buttons declare a property
// of their own keyed `editable` — an enumeration, `default` | `never` — and not
// the Editability system property. exec writes `editable:` to that property
// (measured on PedApp 11.13.0 with Native Mobile Resources 12.6.0: the
// CustomWidget's own Editable stays Always), and describe prints it back as
// `editable: 'default'`. check read the name as the core Editable and refused
// the value: "`Editable: default` is not an Editable value [MDL-WIDGET19]" — so
// a native page with a slider could not be described and re-run.
//
// The value is the widget's own enumeration, judged against the keys its
// package declares, as a PDS Button's own `buttonStyle` is (ako/mxcli#2180).
func TestCheckNativeOwnEditable_JudgedByTheWidgetsOwnEnumeration(t *testing.T) {
	def := &WidgetDefinition{
		WidgetID:   "com.mendix.widget.native.slider.Slider",
		MDLName:    "nativeslider",
		WidgetKind: "pluggable",
		PropertyMappings: []PropertyMapping{
			{PropertyKey: "valueAttribute", Source: "Attribute", Operation: "attribute"},
			{PropertyKey: "editable", Value: "default", Operation: "primitive"},
		},
		enumKeys: map[string][]string{"editable": {"default", "never"}},
	}
	reg := &WidgetRegistry{
		byMDLName:  map[string]*WidgetDefinition{"NATIVESLIDER": def},
		byWidgetID: map[string]*WidgetDefinition{def.WidgetID: def},
	}

	for _, value := range []string{"'default'", "'never'", "never"} {
		got := checkRules(t, reg, "nativeslider s1 (editable: "+value+")")
		if hasNativeRule(got, "MDL-WIDGET19") {
			t.Errorf("editable: %s is the native slider's own enumeration key, but check said %v", value, got)
		}
	}
	// A key the widget does not declare is still refused — by its own list.
	got := checkRules(t, reg, "nativeslider s1 (editable: 'sometimes')")
	if hasNativeRule(got, "MDL-WIDGET19") || !hasNativeRule(got, widgetEnumRefused.Code) {
		t.Errorf("editable: 'sometimes' on a nativeslider: want %s naming its own keys, got %v", widgetEnumRefused.Code, got)
	}
	// Control: a core input keeps MDL-WIDGET19.
	got = checkRules(t, reg, "textbox tb (Attribute: Name, Editable: 'default')")
	if !hasNativeRule(got, "MDL-WIDGET19") {
		t.Errorf("a textbox with `Editable: default` must keep MDL-WIDGET19, got %v", got)
	}
}

// checkRules runs the widget checks over one widget and returns "RULE: message"
// for each violation.
func checkRules(t *testing.T, reg *WidgetRegistry, widget string) []string {
	t.Helper()
	prog, errs := visitor.Build("create page M.P (Title: 'P', Layout: Atlas_Core.NativePhone_Default) {\n  " + widget + "\n};")
	if len(errs) > 0 {
		t.Fatalf("parse %q: %v", widget, errs[0])
	}
	stmt, ok := prog.Statements[0].(*ast.CreatePageStmtV3)
	if !ok {
		t.Fatalf("parsed %T, want a page", prog.Statements[0])
	}
	var out []string
	for _, v := range validateWidgetTree(stmt.Widgets, reg, "page M.P") {
		out = append(out, v.RuleID+": "+v.Message)
	}
	return out
}

func hasNativeRule(got []string, rule string) bool {
	for _, g := range got {
		if strings.HasPrefix(g, rule) {
			return true
		}
	}
	return false
}
