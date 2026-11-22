// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// ako/mxcli#2523: MDL-WIDGET56 reported every other mode's required properties
// on a combo box (12 of them, `attributeEnumeration` among them although
// `attribute:` set it) and `refEntity`, `refSearchAttr` on a drop-down filter,
// and mxbuild builds both clean. Two causes: the hide rules were judged with
// only the statement's values, so `source` and `optionsSourceType` — which
// exec writes from the widget's template (context, enumeration) — were
// unknown and no rule could fire; and a property set through its MDL keyword
// (`attribute:` → attributeEnumeration) was not counted as set.

func shippedWidgetsRegistry2523(t *testing.T) *WidgetRegistry {
	t.Helper()
	proj, err := filepath.Abs(filepath.Join("..", "..", "testdata", "expr-checker", "minimal.mpr"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(proj); err != nil {
		t.Skipf("fixture not found: %v", err)
	}
	reg := LoadWidgetRegistry(proj)
	if reg == nil {
		t.Fatal("no registry")
	}
	return reg
}

func requiredWarnings2523(t *testing.T, reg *WidgetRegistry, src string, names ...string) []string {
	t.Helper()
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs)
	}
	// The walker check runs, so a filter gets its column's context as it does
	// there; only this rule's findings, for the named widgets.
	var out []string
	for _, v := range validateWidgetTree(prog.Statements[0].(*ast.CreatePageStmtV3).Widgets, reg, "page M.P") {
		if v.RuleID != widgetRequiredRule {
			continue
		}
		for _, n := range names {
			if strings.Contains(v.Message, "`"+n+"`") {
				i := strings.Index(v.Message, "`")
				out = append(out, v.Message[i:])
			}
		}
	}
	sort.Strings(out)
	return out
}

func TestRequiredPropertiesOfOtherModesAreNotReported_2523(t *testing.T) {
	reg := shippedWidgetsRegistry2523(t)
	got := requiredWarnings2523(t, reg, `create page M.P (title: 'x', layout: Atlas_Core.Atlas_Default) {
  dataview dv (datasource: $X) {
    combobox cbTech (label: 'Technology', attribute: Technology)
  }
  datagrid dg (datasource: database M.E) {
    column (attribute: Technology, caption: 'Technology') { dropdownfilter f1 }
  }
};`, "cbTech", "f1")
	if len(got) != 0 {
		t.Errorf("enumeration-mode combo box and attribute drop-down filter: %d warnings, want none:\n  %s",
			len(got), strings.Join(got, "\n  "))
	}
}

// Control: what the chosen mode needs and the statement leaves out is still
// reported — an association combo box with no caption attribute.
func TestRequiredPropertyOfTheChosenModeIsStillReported_2523(t *testing.T) {
	reg := shippedWidgetsRegistry2523(t)
	got := requiredWarnings2523(t, reg, `create page M.P (title: 'x', layout: Atlas_Core.Atlas_Default) {
  dataview dv (datasource: $X) {
    combobox cbOwner (label: 'Owner', association: M.E_Owner, datasource: database M.Owner)
  }
};`, "cbOwner")
	joined := strings.Join(got, "\n")
	if !strings.Contains(joined, "optionsSourceAssociationCaptionAttribute") {
		t.Errorf("the association mode's caption attribute is not reported:\n%s", joined)
	}
	for _, other := range []string{"attributeEnumeration", "attributeBoolean", "staticAttribute", "optionsSourceDatabaseValueAttribute"} {
		if strings.Contains(joined, "`"+other+"`") {
			t.Errorf("another mode's %s is reported:\n%s", other, joined)
		}
	}
}
