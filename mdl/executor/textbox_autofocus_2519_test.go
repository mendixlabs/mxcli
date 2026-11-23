// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
)

// ako/mxcli#2519 (mendixlabs/mxcli#1921): a text box's Auto focus
// (Forms$TextBox.AutoFocus) could not be written — check refused it
// (MDL-WIDGET52) and the writer hard-coded false — so a describe → exec round
// trip reset a Studio Pro text box's Auto focus. On native it is what opens
// the keyboard when the page opens.

func TestBuildTextBox_AutoFocus_2519(t *testing.T) {
	for _, tc := range []struct {
		value any
		want  bool
	}{{nil, false}, {true, true}, {"true", true}, {false, false}} {
		pb := &pageBuilder{widgetScope: map[string]model.ID{}}
		w := &ast.WidgetV3{Name: "tb", Type: "textbox", Properties: map[string]any{}}
		if tc.value != nil {
			w.Properties["AutoFocus"] = tc.value
		}
		tb, err := pb.buildTextBoxV3(w)
		if err != nil {
			t.Fatalf("%v: %v", tc.value, err)
		}
		if tb.AutoFocus != tc.want {
			t.Errorf("AutoFocus %v built as %v, want %v", tc.value, tb.AutoFocus, tc.want)
		}
	}
	pb := &pageBuilder{widgetScope: map[string]model.ID{}}
	if _, err := pb.buildTextBoxV3(&ast.WidgetV3{Name: "tb", Type: "textbox",
		Properties: map[string]any{"AutoFocus": "maybe"}}); err == nil {
		t.Error("AutoFocus: maybe was accepted")
	}
}

func TestDescribeTextBox_AutoFocus_2519(t *testing.T) {
	ctx, buf := newMockCtx(t)
	for _, rw := range parseRawWidget(ctx, map[string]any{
		"$Type": "Forms$TextBox", "Name": "tbCode", "AutoFocus": true,
	}) {
		outputWidgetMDLV3(ctx, rw, 0)
	}
	out := buf.String()
	if !strings.Contains(out, "AutoFocus: true") {
		t.Errorf("describe dropped Auto focus, so a rewrite clears it:\n%s", out)
	}
	if _, errs := visitor.Build("create page M.P (Title: 'x', Layout: A.L) {\n" + out + "}\n"); len(errs) > 0 {
		t.Fatalf("describe output does not parse: %v\n%s", errs, out)
	}
	// Control: false is every ordinary text box and is not printed.
	ctx, buf = newMockCtx(t)
	for _, rw := range parseRawWidget(ctx, map[string]any{
		"$Type": "Forms$TextBox", "Name": "tbName", "AutoFocus": false,
	}) {
		outputWidgetMDLV3(ctx, rw, 0)
	}
	if strings.Contains(buf.String(), "AutoFocus") {
		t.Errorf("describe printed AutoFocus false: %s", buf.String())
	}
}

func TestCheckTextBox_AutoFocusIsWritten_2519(t *testing.T) {
	w := &ast.WidgetV3{Name: "tbCode", Type: "textbox", Properties: map[string]any{"AutoFocus": true}}
	for _, v := range validateNativeWidgetProps("textbox", w, "page M.P") {
		if strings.Contains(v.Message, "AutoFocus") {
			t.Errorf("AutoFocus still refused: %s", v.Message)
		}
	}
}
