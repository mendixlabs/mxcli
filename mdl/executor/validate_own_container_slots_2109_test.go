// SPDX-License-Identifier: Apache-2.0

// ako/mxcli#2109: check refused its own describe output on four FactoryManagement
// pages (Mendix 11.16), which Studio Pro authored and which build clean. The
// stored shapes, measured:
//
//   - MxModelReflection.MxObjectType_Select, OIDC.OIDC_Client_NewEdit,
//     UserCommons.ClaimsForClaimEntityAttribute: a button in a Data grid 2
//     column's content, in a grid with `Selection: Single`, whose show page
//     mapping stores Variable {Widget: grid1} — the grid's SELECTION, the shape
//     a control bar button stores for `$grid` (mendixlabs/mxcli#122).
//   - AgentCommons.Version_NewEdit: container5 in a combo box's
//     optionsSourceAssociationCustomContent, inside dataView2, with
//     `Visible: $currentObject = $dataView2`. The option content is the combo
//     box's own item context, so $dataView2 is an OUTER container there.
//
// check said: "`actionButton1` reads `$grid1` …, but `grid1` is the data
// container the widget sits in directly — its name is a variable only inside
// a data container nested below it" (MDL-BUTTON02), and refused the argument
// as an explicit mapping (MDL-PAGEARG01, "an explicit one is rejected as CE0115").
package executor

import (
	"strings"
	"testing"
)

func ownContainerRules(t *testing.T, src string) []string {
	t.Helper()
	var out []string
	for _, v := range ValidateProgram(parseMDL(t, src), "") {
		if v.RuleID == "MDL-BUTTON02" || v.RuleID == "MDL-PAGEARG01" {
			out = append(out, v.RuleID+": "+v.Message)
		}
	}
	return out
}

func gridColumnButtonPage(selection string) string {
	return `mdl 1;
create page M.Select ( Title: 'Select', Layout: Atlas_Core.PopupLayout ) {
  datagrid grid1 ( DataSource: database from M.Item` + selection + ` ) {
    column ( Attribute: Name, ShowContentAs: customContent ) {
      actionbutton actionButton1 ( Caption: 'View', Action: show page M.Item_Details(Item = $grid1) )
    }
  }
};`
}

func TestCheck_GridSelectionFromColumnContent(t *testing.T) {
	if got := ownContainerRules(t, gridColumnButtonPage(", Selection: Single")); len(got) != 0 {
		t.Errorf("a selectable grid's selection refused from its column content:\n%s", strings.Join(got, "\n"))
	}
	// Control: a grid with no selection has no `$grid1` to pass.
	if got := ownContainerRules(t, gridColumnButtonPage("")); len(got) == 0 {
		t.Error("$grid1 accepted in a grid with no selection")
	}
}

func comboBoxOptionContentPage(visible string) string {
	return `mdl 1;
create page M.Edit ( Title: 'Edit', Layout: Atlas_Core.PopupLayout, Params: ( $Agent: M.Agent ) ) {
  dataview dataView2 ( DataSource: $Agent ) {
    combobox comboBox3 ( Attribute: Agent_Version_InUse, DataSource: database from M.Version, optionsSourceAssociationCustomContentType: 'yes' ) {
      optionssourceassociationcustomcontent {
        container container5 ( Visible: ` + visible + ` )
      }
    }
    container container9 ( Visible: $currentObject = $dataView2 )
  }
};`
}

func TestCheck_ComboBoxOptionContentIsItsOwnContext(t *testing.T) {
	got := ownContainerRules(t, comboBoxOptionContentPage("$currentObject = $dataView2"))
	// container9 sits directly in dataView2 and is still refused (the control);
	// container5, in the combo box's option content, is not.
	if len(got) != 1 || !strings.Contains(got[0], "container9") {
		t.Errorf("want only container9 refused, got:\n%s", strings.Join(got, "\n"))
	}
}
