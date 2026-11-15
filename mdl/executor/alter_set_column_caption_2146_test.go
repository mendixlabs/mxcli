// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"
)

// ako/mxcli#2146 (mendixlabs/mxcli#1823): `set (Caption: 'New caption') on
// myGrid column('Old caption')` applied the rename, then the follow-up that
// re-finds the column looked for 'Old caption' and failed with "column … not
// found"; check and exec both refused, and exec rolled the script back. The
// #1644 sibling for an attribute address: the property the address reads is now
// written last.
func TestAlterPageSet_ColumnCaptionByCaption_Issue2146(t *testing.T) {
	exec, out := openPedAppWithWidgets(t)
	dir := filepath.Dir(exec.newExecContext(context.Background()).Backend.Path())
	const setup = `mdl 1;
create persistent entity MyFirstModule.Item2146 (Name: String(100), Code: String(20));
create page MyFirstModule.Grid2146 ( Title: 'Items', Layout: Atlas_Core.Atlas_Default ) {
  datagrid dgItems (DataSource: database from MyFirstModule.Item2146) {
    column colName (Attribute: Name, Caption: 'Name')
    column colCustom (Caption: 'Old caption') {
      dynamictext t1 (Content: 'x')
    }
    column colOther (Caption: 'Other') {
      dynamictext t2 (Content: 'y')
    }
  }
};`
	if err := agreeExec(t, exec, setup); err != nil {
		t.Fatalf("setup: %v\n%s", err, out.String())
	}
	alter := func(set string) string {
		return "mdl 1;\nalter page MyFirstModule.Grid2146 { " + set + " };"
	}
	// The reported statement.
	assertAgree(t, exec, dir, alter("set (Caption: 'New caption') on dgItems column('Old caption')"), "")
	// The caption together with another property: the rest is applied under
	// the address as written.
	assertAgree(t, exec, dir, alter("set (Caption: 'Renamed', Visible: false) on dgItems column('Other')"), "")
	// CONTROL: renaming a column addressed by its attribute always worked.
	assertAgree(t, exec, dir, alter("set (Caption: 'Full name') on dgItems column(Name)"), "")

	var buf bytes.Buffer
	saved := exec.output
	exec.output = &buf
	err := afRun(t, exec, "describe page MyFirstModule.Grid2146;")
	exec.output = saved
	if err != nil {
		t.Fatal(err)
	}
	d := buf.String()
	for _, want := range []string{"Caption: 'New caption'", "Caption: 'Renamed'", "Caption: 'Full name'"} {
		if !strings.Contains(d, want) {
			t.Errorf("describe does not show %q:\n%s", want, d)
		}
	}
	if strings.Contains(d, "Old caption") {
		t.Errorf("the old caption is still there:\n%s", d)
	}
}
