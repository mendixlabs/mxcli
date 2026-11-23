// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// ako/mxcli#2522: Combo box `customEditability` is hidden while `source` is not
// "database" — a rule `widget describe combobox` lists as recognised — yet a
// script writing it on a combo box whose source exec leaves at "context" (an
// association combo box: its options come from optionsSourceAssociation…, not
// from `source`) passed check, and mxbuild failed it with CE0463. The hide rule
// was never judged: `source` came from the widget's template, not from the
// statement, so the condition read as undeterminable and MDL-WIDGET10 stayed
// silent — the same gap as ako/mxcli#2523 in the required-property check.
func hiddenWarnings2522(t *testing.T, widget string) []string {
	t.Helper()
	reg := shippedWidgetsRegistry2523(t)
	src := "create page M.P (Title: 'P', Layout: Atlas_Core.Atlas_Default, Params: ($O: M.Order)) {\n" +
		"  dataview dv (datasource: $O) {\n    " + widget + "\n  }\n};"
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs)
	}
	var out []string
	for _, v := range validateWidgetTree(prog.Statements[0].(*ast.CreatePageStmtV3).Widgets, reg, "page M.P") {
		if v.RuleID == "MDL-WIDGET10" {
			out = append(out, v.Message)
		}
	}
	return out
}

func TestHiddenPropertyJudgedWithTemplateValues_2522(t *testing.T) {
	got := hiddenWarnings2522(t, "combobox cb (label: 'Customer', association: M.Order_Customer, datasource: database M.Customer, customEditability: never)")
	if len(got) == 0 || !strings.Contains(strings.Join(got, "\n"), "customEditability") {
		t.Errorf("customEditability is hidden while source is context (CE0463), but MDL-WIDGET10 said %v", got)
	}
}

// Controls: in database mode (chosen by its own datasource key — a bare
// `datasource:` selects the association mode) the property is shown; not
// writing it is fine.
func TestHiddenPropertyControls_2522(t *testing.T) {
	for _, widget := range []string{
		"combobox cb (label: 'Customer', optionsSourceDatabaseDataSource: database M.Customer, customEditability: never)",
		"combobox cb (label: 'Customer', association: M.Order_Customer, datasource: database M.Customer)",
	} {
		if got := hiddenWarnings2522(t, widget); strings.Contains(strings.Join(got, "\n"), "customEditability") {
			t.Errorf("%s\n  reported %v", widget, got)
		}
	}
}
