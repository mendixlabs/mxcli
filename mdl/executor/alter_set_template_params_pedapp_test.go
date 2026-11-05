// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"strings"
	"testing"
)

// ako/mxcli#1966 (mendixlabs/mxcli#1744): `alter page … set (CaptionParams:
// (…))` / `set (ContentParams: (…))` did not parse, so a text template's
// parameters could be written by create page and replace but not changed in
// place. The reporter's two statements now pass check and exec, and describe
// prints the parameters back, resolved against the data view's entity.
func TestAlterPageSet_TemplateParameters(t *testing.T) {
	exec, out, dir := openPedAppCopy(t)
	const setup = `mdl 1;
create persistent entity MyFirstModule.Draft1966 (Title: String(100), VersionNo: Integer);
create microflow MyFirstModule.ACT_Open1966 ($Draft: MyFirstModule.Draft1966) returns Boolean begin return true; end;
create page MyFirstModule.PageD1966 ( Title: 'D', Layout: Atlas_Core.PopupLayout, Params: ( $Draft: MyFirstModule.Draft1966 ) ) {
  dataview dvD (DataSource: $Draft) {
    actionbutton btnD (Caption: 'Open draft', Action: call microflow MyFirstModule.ACT_Open1966(Draft = $currentObject))
    dynamictext txtE (Content: 'Open draft (version {1})', ContentParams: ({1} = Title))
    textbox tbT (Label: 'Title', Attribute: Title)
  }
};`
	if err := agreeExec(t, exec, setup); err != nil {
		t.Fatalf("setup: %v\n%s", err, out.String())
	}
	assertAgree(t, exec, dir, "mdl 1;\nalter page MyFirstModule.PageD1966 { "+
		"set (Caption: 'Open draft (version {1})', CaptionParams: ({1} = VersionNo)) on btnD };", "")
	assertAgree(t, exec, dir, "mdl 1;\nalter page MyFirstModule.PageD1966 { "+
		"set (Content: 'Draft {1}', ContentParams: ({1} = VersionNo)) on txtE };", "")

	var buf bytes.Buffer
	saved := exec.output
	exec.output = &buf
	err := afRun(t, exec, "describe page MyFirstModule.PageD1966;")
	exec.output = saved
	if err != nil {
		t.Fatal(err)
	}
	d := buf.String()
	for _, want := range []string{
		"Caption: 'Open draft (version {1})',",
		"CaptionParams: ({1} = VersionNo)",
		"Content: 'Draft {1}', ContentParams: ({1} = VersionNo)",
	} {
		if !strings.Contains(d, want) {
			t.Errorf("describe does not show %q:\n%s", want, d)
		}
	}

	// Control: a widget with no text template refuses it, in both.
	assertAgree(t, exec, dir, "mdl 1;\nalter page MyFirstModule.PageD1966 { set (CaptionParams: ({1} = VersionNo)) on tbT };",
		"has no CaptionParams")
}
