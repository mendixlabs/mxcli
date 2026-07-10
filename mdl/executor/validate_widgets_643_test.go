// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
	"github.com/JordtenBulte-OLC/mxcli/mdl/linter"
)

// Issue #643: a datasource-typed combo property supplied as a named value
// (optionsSourceAssociationDataSource: Module.Entity) passed `check` but was
// silently dropped at exec (CE0642). It must now be flagged (MDL-WIDGET05), and
// a real-but-unmapped property (optionsSourceAssociationCaptionType) must be a
// warning (MDL-WIDGET06), not a false "unknown property" error (MDL-WIDGET01).
func combo(props map[string]any) *ast.WidgetV3 {
	p := map[string]any{"WidgetType": "com.mendix.widget.web.combobox.Combobox"}
	for k, v := range props {
		p[k] = v
	}
	return &ast.WidgetV3{Name: "cb", Type: "pluggablewidget", Properties: p}
}

func ruleIDs(vs []linter.Violation) map[string]string {
	out := map[string]string{}
	for _, v := range vs {
		out[v.RuleID] = v.Message
	}
	return out
}

func TestIssue643_DatasourceByName_Rejected(t *testing.T) {
	reg := LoadWidgetRegistry("")
	if reg == nil {
		t.Fatal("built-in widget registry not available")
	}

	// Datasource-typed property by name → MDL-WIDGET05 error.
	w := combo(map[string]any{
		"optionsSourceType":                   "association",
		"optionsSourceAssociationDataSource":  "Administration.Account",
		"optionsSourceAssociationCaptionType": "attribute",
	})
	got := ruleIDs(validatePluggableWidgetProperties(w, reg, "page P"))
	if _, ok := got["MDL-WIDGET05"]; !ok {
		t.Errorf("expected MDL-WIDGET05 for datasource-by-name, got rules: %v", keysOf(got))
	}
	if msg, ok := got["MDL-WIDGET06"]; !ok {
		t.Errorf("expected MDL-WIDGET06 warning for CaptionType, got rules: %v", keysOf(got))
	} else if !strings.Contains(msg, "optionsSourceAssociationCaptionType") {
		t.Errorf("WIDGET04 message should name the property, got: %q", msg)
	}
	if _, ok := got["MDL-WIDGET01"]; ok {
		t.Errorf("CaptionType must NOT be a false 'unknown property' (MDL-WIDGET01)")
	}
}

func TestIssue643_DatasourceClause_NotFlagged(t *testing.T) {
	reg := LoadWidgetRegistry("")
	if reg == nil {
		t.Fatal("built-in widget registry not available")
	}
	// The workaround: datasource provided via the widget DataSource clause (the
	// builtin "DataSource" key), not by name → no MDL-WIDGET05.
	w := combo(map[string]any{
		"optionsSourceType": "association",
		"DataSource":        &ast.DataSourceV3{Type: "database", Reference: "Administration.Account"},
	})
	for _, v := range validatePluggableWidgetProperties(w, reg, "page P") {
		if v.RuleID == "MDL-WIDGET05" {
			t.Errorf("datasource clause must not trigger MDL-WIDGET05: %s", v.Message)
		}
	}
}

func keysOf(m map[string]string) []string {
	var ks []string
	for k := range m {
		ks = append(ks, k)
	}
	return ks
}
