// SPDX-License-Identifier: Apache-2.0

// ako/mxcli#2106: three widget settings Studio Pro stores had no MDL spelling,
// so `describe page` → `exec` of FactoryManagement's pages changed them
// (Mendix 11.16, value-only diff of the unit before and after):
//
//	TraceFiltering_Edit       checkBox1.LabelPosition                AfterControl → Default
//	TeamcenterConfiguration…  radioButtons1.RenderHorizontal         false → true
//	ConnectionDetails_…Step2  actionButton1.Action.NumberOfPagesToClose2  "1" → ""
//
// Each now reads back from the stored widget, describes, builds and writes.
package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

func TestDescribeCheckBox_LabelPosition(t *testing.T) {
	got := describeStoredWidget(t, map[string]any{"$Type": "Forms$CheckBox", "Name": "checkBox1", "LabelPosition": "AfterControl"})
	if !strings.Contains(got, "LabelPosition: AfterControl") {
		t.Errorf("describe dropped the label position:\n%s", got)
	}
	// Control: Studio Pro's default is not printed.
	got = describeStoredWidget(t, map[string]any{"$Type": "Forms$CheckBox", "Name": "checkBox1", "LabelPosition": "Default"})
	if strings.Contains(got, "LabelPosition") {
		t.Errorf("describe printed the default label position:\n%s", got)
	}
}

func TestDescribeRadioButtons_RenderHorizontal(t *testing.T) {
	got := describeStoredWidget(t, map[string]any{"$Type": "Forms$RadioButtonGroup", "Name": "rb", "RenderHorizontal": false})
	if !strings.Contains(got, "RenderHorizontal: false") {
		t.Errorf("describe dropped a vertical radio button group:\n%s", got)
	}
	// Control: horizontal is what the writer stores by default.
	got = describeStoredWidget(t, map[string]any{"$Type": "Forms$RadioButtonGroup", "Name": "rb", "RenderHorizontal": true})
	if strings.Contains(got, "RenderHorizontal") {
		t.Errorf("describe printed the default orientation:\n%s", got)
	}
}

func TestDescribeShowPageAction_NumberOfPagesToClose(t *testing.T) {
	action := map[string]any{"$Type": "Forms$FormAction", "DisabledDuringExecution": true, "NumberOfPagesToClose2": "1"}
	if got := renderClientActionSettingsMDL(nil, action); got != " with (NumberOfPagesToClose: 1)" {
		t.Errorf("settings = %q, want the close count", got)
	}
	action["NumberOfPagesToClose2"] = ""
	if got := renderClientActionSettingsMDL(nil, action); got != "" {
		t.Errorf("settings = %q, want none for the default", got)
	}
}

func TestBuildCheckBox_LabelPosition(t *testing.T) {
	pb := newPropsBuilder()
	cb, err := pb.buildCheckBoxV3(&ast.WidgetV3{Name: "cb", Type: "checkbox", Properties: map[string]any{"LabelPosition": "aftercontrol"}})
	if err != nil {
		t.Fatal(err)
	}
	if cb.LabelPosition != "AfterControl" {
		t.Errorf("LabelPosition = %q, want AfterControl", cb.LabelPosition)
	}
	if _, err := newPropsBuilder().buildCheckBoxV3(&ast.WidgetV3{Name: "cb", Type: "checkbox", Properties: map[string]any{"LabelPosition": "Left"}}); err == nil {
		t.Error("LabelPosition Left accepted, want a refusal naming the values")
	}
}

func TestBuildRadioButtons_RenderHorizontal(t *testing.T) {
	for _, tc := range []struct {
		value any
		want  pages.RenderDirection
	}{{nil, ""}, {false, pages.RenderDirectionVertical}, {"false", pages.RenderDirectionVertical}, {true, pages.RenderDirectionHorizontal}} {
		w := &ast.WidgetV3{Name: "rb", Type: "radiobuttons", Properties: map[string]any{}}
		if tc.value != nil {
			w.Properties["RenderHorizontal"] = tc.value
		}
		rb, err := newPropsBuilder().buildRadioButtonsV3(w)
		if err != nil {
			t.Fatal(err)
		}
		if rb.RenderDirection != tc.want {
			t.Errorf("RenderHorizontal %v: RenderDirection = %q, want %q", tc.value, rb.RenderDirection, tc.want)
		}
	}
}

func TestApplyActionSettings_NumberOfPagesToClose(t *testing.T) {
	a := &pages.PageClientAction{PageName: "M.Next"}
	if err := applyActionSettings(a, &ast.ActionSettingsV3{NumberOfPagesToClose: "2"}); err != nil {
		t.Fatal(err)
	}
	if a.NumberOfPagesToClose != "2" {
		t.Errorf("NumberOfPagesToClose = %q, want 2", a.NumberOfPagesToClose)
	}
}
