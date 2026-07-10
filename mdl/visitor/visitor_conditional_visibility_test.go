// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
)

// Issue #627 — conditional-visibility/editability expressions must root bare
// attribute references in the widget data context ($currentObject), or Studio
// Pro rejects them with CE0117.
func TestConditionalVisibility_PrefixesContext(t *testing.T) {
	cases := []struct {
		name string
		expr string // what goes inside Visible: [ ... ]
		want string
	}{
		{"bare attr not-empty", "Name != ''", "$currentObject/Name != ''"},
		{"bare boolean attr", "Active", "$currentObject/Active"},
		{"bare attr empty keyword", "Name != empty", "$currentObject/Name != empty"},
		{"already qualified currentObject", "$currentObject/Name != ''", "$currentObject/Name != ''"},
		{"already qualified param", "$Customer/Name != ''", "$Customer/Name != ''"},
		{"and of two attrs", "Active and Name != ''", "$currentObject/Active and $currentObject/Name != ''"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			input := "CREATE PAGE M.P (Title: 'P') { CONTAINER ctn (Visible: [" + c.expr + "]) { DYNAMICTEXT t (Content: 'x') } };"
			prog, errs := Build(input)
			if len(errs) > 0 {
				t.Fatalf("parse errors: %v", errs)
			}
			ctn := findWidgetV3(prog.Statements[0].(*ast.CreatePageStmtV3).Widgets, "ctn")
			if ctn == nil {
				t.Fatal("container ctn not found")
			}
			got, _ := ctn.Properties["VisibleIf"].(string)
			if got != c.want {
				t.Errorf("VisibleIf = %q, want %q", got, c.want)
			}
		})
	}
}

// Editable uses the same transform.
func TestConditionalEditability_PrefixesContext(t *testing.T) {
	input := "CREATE PAGE M.P (Title: 'P') { TEXTBOX tb (Label: 'N', Attribute: Name, Editable: [Status = 'Open']) };"
	prog, errs := Build(input)
	if len(errs) > 0 {
		t.Fatalf("parse errors: %v", errs)
	}
	tb := findWidgetV3(prog.Statements[0].(*ast.CreatePageStmtV3).Widgets, "tb")
	if tb == nil {
		t.Fatal("textbox tb not found")
	}
	if got, _ := tb.Properties["EditableIf"].(string); got != "$currentObject/Status = 'Open'" {
		t.Errorf("EditableIf = %q, want %q", got, "$currentObject/Status = 'Open'")
	}
}

// findWidgetV3 finds a widget by name anywhere in the V3 widget tree.
func findWidgetV3(widgets []*ast.WidgetV3, name string) *ast.WidgetV3 {
	for _, w := range widgets {
		if w.Name == name {
			return w
		}
		if found := findWidgetV3(w.Children, name); found != nil {
			return found
		}
	}
	return nil
}

// Regression: a qualified enum value in a conditional-visibility expression must
// be preserved as the qualified literal (Module.Enum.Value), NOT stringified to
// 'Value' like an XPath datasource constraint — that produced CE0117 in v0.13.0.
func TestConditionalVisibility_EnumLiteralPreserved(t *testing.T) {
	input := "CREATE PAGE M.P (Title: 'P') { CONTAINER ctn (Visible: [$currentObject/Status = MES.EquipmentStatus.Running]) { DYNAMICTEXT t (Content: 'x') } };"
	prog, errs := Build(input)
	if len(errs) > 0 {
		t.Fatalf("parse errors: %v", errs)
	}
	ctn := findWidgetV3(prog.Statements[0].(*ast.CreatePageStmtV3).Widgets, "ctn")
	got, _ := ctn.Properties["VisibleIf"].(string)
	want := "$currentObject/Status = MES.EquipmentStatus.Running"
	if got != want {
		t.Errorf("VisibleIf = %q, want %q", got, want)
	}
}
