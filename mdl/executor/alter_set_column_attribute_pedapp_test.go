// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/sdk/pages"
)

// ako/mxcli#1644: `alter page … set (Attribute: …) on <grid> column(…)` was
// refused for every spelling ("holds a value of kind Attribute, which ALTER
// cannot set from a plain value"), and `Assoc/Attr` did not parse — the only way
// was to replace the column, dropping what the replacement did not restate. It
// now binds as CREATE binds a column's Attribute: against the grid's entity.
func TestAlterPageSet_ColumnAttribute(t *testing.T) {
	exec, out := openPedAppWithWidgets(t)
	dir := filepath.Dir(exec.newExecContext(context.Background()).Backend.Path())
	const setup = `mdl 1;
create persistent entity MyFirstModule.Customer1644 (Name: String(100), City: String(100));
create persistent entity MyFirstModule.Order1644 (Number: String(20), Note: String(200), Code: String(20), Ref: String(20));
create association MyFirstModule.Order1644_Customer1644 from MyFirstModule.Order1644 to MyFirstModule.Customer1644;
create page MyFirstModule.OrderGrid1644 ( Title: 'Orders', Layout: Atlas_Core.Atlas_Default, Params: ( $Cust: MyFirstModule.Customer1644 ) ) {
  datagrid dgOrders (DataSource: database from MyFirstModule.Order1644) {
    column colNumber (Attribute: Number, Caption: 'Number')
    column colCode (Attribute: Code, Caption: 'Code')
    column colRef (Attribute: Ref, Caption: 'Ref')
  }
};`
	if err := agreeExec(t, exec, setup); err != nil {
		t.Fatalf("setup: %v\n%s", err, out.String())
	}
	alter := func(set string) string {
		return "mdl 1;\nalter page MyFirstModule.OrderGrid1644 { " + set + " };"
	}
	assertAgree(t, exec, dir, alter("set (Attribute: Note) on dgOrders column(Number)"), "")
	assertAgree(t, exec, dir, alter("set (Attribute: MyFirstModule.Order1644.Number) on dgOrders column(Code)"), "")
	assertAgree(t, exec, dir, alter("set (Attribute: Order1644_Customer1644/City) on dgOrders column(Ref)"), "")

	var buf bytes.Buffer
	saved := exec.output
	exec.output = &buf
	err := afRun(t, exec, "describe page MyFirstModule.OrderGrid1644;")
	exec.output = saved
	if err != nil {
		t.Fatal(err)
	}
	d := buf.String()
	for _, want := range []string{"Attribute: Note", "Attribute: Number", "Attribute: Order1644_Customer1644/City"} {
		if !strings.Contains(d, want) {
			t.Errorf("describe does not show %q:\n%s", want, d)
		}
	}

	// Refusals, in check and in exec alike.
	assertAgree(t, exec, dir, alter("set (Attribute: $Cust.Name) on dgOrders column(Note)"),
		"a column's attribute is read from each of the grid's items")
	assertAgree(t, exec, dir, alter("set (Attribute: NoSuchAttr) on dgOrders column(Note)"), "NoSuchAttr")
	assertAgree(t, exec, dir, alter("set (Attribute: MyFirstModule.Customer1644.City) on dgOrders column(Note)"), "Customer1644")
}

// The reset of what the widget hides runs after a column SET and re-finds the
// column by its address — which is its attribute. Changing the attribute moved
// the column away from `column(Old)`, and the reset failed with "column(Old)
// not found" (measured on 11.14 with its Data grid 2 package; PedApp's ships no
// editorConfig, so its rules are installed from the expr-checker fixture here).
// The attribute is now written after the rest of the SET and its reset.
func TestAlterPageSet_ColumnAttributeWithHiddenPropertyReset(t *testing.T) {
	exec, out := openPedAppWithWidgets(t)
	path := exec.newExecContext(context.Background()).Backend.Path()
	dir := filepath.Dir(path)
	mpk := "com.mendix.widget.web.Datagrid.mpk"
	if err := copyPedAppFile(filepath.Join("..", "..", "testdata", "expr-checker", "widgets", mpk), filepath.Join(dir, "widgets", mpk)); err != nil {
		t.Fatal(err)
	}
	if rules := resolveWidgetVisibilityRules(path, pages.WidgetIDDataGrid2); len(rules) == 0 {
		t.Fatal("control: the installed Data grid 2 package states no visibility rules, so the reset would not run")
	}
	const setup = `mdl 1;
create persistent entity MyFirstModule.Order1644b (Number: String(20), Note: String(200));
create page MyFirstModule.OrderGrid1644b ( Title: 'Orders', Layout: Atlas_Core.Atlas_Default ) {
  datagrid dgOrders (DataSource: database from MyFirstModule.Order1644b) {
    column colNumber (Attribute: Number, Caption: 'Number')
  }
};`
	if err := agreeExec(t, exec, setup); err != nil {
		t.Fatalf("setup: %v\n%s", err, out.String())
	}
	for _, set := range []string{
		"set (Attribute: Note) on dgOrders column(Number)",
		"set (Attribute: Number, ColumnWidth: autoFill) on dgOrders column(Note)",
	} {
		assertAgree(t, exec, dir, "mdl 1;\nalter page MyFirstModule.OrderGrid1644b { "+set+" };", "")
	}
}
