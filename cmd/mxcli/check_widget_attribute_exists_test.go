// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ako/mxcli#555: an attribute named inside a widget was not resolved by
// `check --references` — a textbox's Attribute, or a template parameter of a
// dynamic text — so a misspelt name passed check, exec wrote it, and mxbuild
// refused it with CE1613. Each binding is now resolved against the entity its
// data container supplies, and the controls bind what Account has: its own
// attribute, an inherited one, an audit member and an expression.
func TestCheck_WidgetAttributeMustExist(t *testing.T) {
	src := filepath.Join("..", "..", "testdata", "pedapp")
	if _, err := os.Stat(filepath.Join(src, "PedApp.mpr")); err != nil {
		t.Skipf("PedApp fixture not found: %v", err)
	}
	dir := t.TempDir()
	if err := copyTree(src, dir); err != nil {
		t.Fatal(err)
	}
	_ = checkCmd.InheritedFlags()
	_ = rootCmd.PersistentFlags().Set("project", filepath.Join(dir, "PedApp.mpr"))
	defer func() {
		_ = rootCmd.PersistentFlags().Set("project", "")
		rootCmd.PersistentFlags().Lookup("project").Changed = false
	}()
	if f := checkCmd.Flags().Lookup("format"); f != nil {
		_ = f.Value.Set(f.DefValue)
	}
	scripts := t.TempDir()
	page := func(file, widgets string) (int, string) {
		path := writeScript(t, scripts, file+".mdl", `mdl 1;
create page MyFirstModule.P555 ( Title: 'x', Layout: Atlas_Core.PopupLayout, Params: ( $Acc: Administration.Account ) )
{
  layoutgrid lg { row { column (desktopwidth: autofill) {
    dataview dv (DataSource: $Acc) {
`+widgets+`
    }
  } } }
};
`)
		var code int
		out := captureStd(t, func() { code = runCheckFiles(checkCmd, []string{path}) })
		return code, out
	}

	for _, tc := range []struct{ name, widgets, want string }{
		{"textbox attribute", `textbox tb (Label: 'x', Attribute: PlannedOn)`,
			"widget `tb` (textbox) binds `PlannedOn`, but Administration.Account has no attribute PlannedOn"},
		{"dynamic text content parameter", `dynamictext dt (Content: 'Hi {1}', ContentParams: ({1} = PlannedOn))`,
			"widget `dt` (dynamictext) ContentParams {1} binds `PlannedOn`, but Administration.Account has no attribute PlannedOn"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			code, out := page(strings.ReplaceAll(tc.name, " ", "_"), tc.widgets)
			if code == 0 || !strings.Contains(out, tc.want) || !strings.Contains(out, "CE1613") {
				t.Errorf("a missing attribute passed check (exit %d), want %q:\n%s", code, tc.want, out)
			}
		})
	}

	t.Run("control: attributes the entity has", func(t *testing.T) {
		code, out := page("control", `textbox tb1 (Label: 'x', Attribute: FullName)
      textbox tb2 (Label: 'x', Attribute: Name)
      datepicker dp (Label: 'x', Attribute: changedDate)
      dynamictext dt1 (Content: '{1} {2} {3}', ContentParams: ({1} = FullName, {2} = 'literal', {3} = $Acc/Email))`)
		if code != 0 || strings.Contains(out, "has no attribute") {
			t.Errorf("an attribute Account has was refused (exit %d):\n%s", code, out)
		}
	})
}
