// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/linter"
)

// ako/mxcli#2561: a NativePhone bottom bar that breaks its limits got no error
// from check — only MDL077 warnings about a collapsed web sidebar, suggesting
// the collection icon exec then refused — and exec named one problem at a time.
func TestValidateNativeBottomBar2561(t *testing.T) {
	const src = `mdl 1;
create or modify navigation NativePhone
  home page NativeApp.Home_Native
  {
    menu item 'Menu' ( OnClick: show page NativeApp.Home_Native, Icon: Atlas_Core.Atlas.home )
    menu 'Group' { menu item 'X' ( OnClick: show page NativeApp.Info_Native ) }
    menu item 'Out' ( OnClick: sign out )
    menu item 'Fine' ( OnClick: show page NativeApp.Info_Native, Icon: glyph 57378 )
    menu item 'Bare' ( OnClick: show page NativeApp.Info_Native )
  };`
	prog := parseMDL(t, src)
	vs := ValidateProgram(prog, "")
	var errs, sidebar []string
	for _, v := range vs {
		switch v.RuleID {
		case "MDL-NATIVEBAR01":
			if v.Severity != linter.SeverityError {
				t.Errorf("%s is %v, want an error: exec refuses it", v.Message, v.Severity)
			}
			errs = append(errs, v.Message)
		case "MDL077":
			sidebar = append(sidebar, v.Message)
		}
	}
	got := strings.Join(errs, "\n")
	for _, want := range []string{`"Menu" has a collection or image icon`, `"Group" is a menu with sub-items`, `"Out" signs out`} {
		if !strings.Contains(got, want) {
			t.Errorf("check does not report %s; got:\n%s", want, got)
		}
	}
	if len(errs) != 3 {
		t.Errorf("want exactly the three problems (not the glyph item, not the item with no icon), got %d:\n%s", len(errs), got)
	}
	if len(sidebar) > 0 {
		t.Errorf("MDL077 is about a web sidebar a native profile does not have:\n%s", strings.Join(sidebar, "\n"))
	}

	// The control: the same menu on a web profile is not a bottom bar.
	web := parseMDL(t, strings.Replace(src, "NativePhone", "Responsive", 1))
	for _, v := range ValidateProgram(web, "") {
		if v.RuleID == "MDL-NATIVEBAR01" {
			t.Errorf("a web profile is reported as a bottom bar: %s", v.Message)
		}
	}
}
