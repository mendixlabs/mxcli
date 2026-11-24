// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// ako/mxcli#2565: mxbuild gives each bottom bar item the roles that may view
// its page, and the native client drops an item no role of the session has —
// with no items left, no bottom bar at all. A bar whose pages no user role may
// view (new pages, no grant) passed check, exec and mx check, and the app just
// showed none.
func TestNativeBottomBarRoles2565(t *testing.T) {
	exec, out, _ := openPedAppCopy(t)
	const setup = `mdl 1;
alter project security level prototype;
create page MyFirstModule.Home_Native (Title: 'Menu', Layout: Atlas_Core.NativePhone_Default) { dynamictext t (Content: 'x') };
create page MyFirstModule.Info_Native (Title: 'Info', Layout: Atlas_Core.NativePhone_Default) { dynamictext t (Content: 'y') };`
	if err := agreeExec(t, exec, setup); err != nil {
		t.Fatalf("setup: %v\n%s", err, out.String())
	}
	const nav = `
create or modify navigation NativePhone
  home page MyFirstModule.Home_Native
  {
    menu item 'Menu' ( OnClick: show page MyFirstModule.Home_Native, Icon: glyph 57377 )
    menu item 'Info' ( OnClick: show page MyFirstModule.Info_Native, Icon: glyph 57378 )
  };`
	warnings := func(src string) []string {
		prog, errs := visitor.Build(src)
		if len(errs) > 0 {
			t.Fatal(errs[0])
		}
		var got []string
		for _, v := range exec.CheckNativeBottomBarRoles(prog) {
			got = append(got, v.RuleID+" "+v.Message)
		}
		return got
	}

	// Every item hidden: one warning, for the bar, which is not shown.
	got := warnings("mdl 1;" + nav)
	if len(got) != 1 || !strings.HasPrefix(got[0], "MDL-NATIVEBAR02") || !strings.Contains(got[0], "no user role may view") ||
		!strings.Contains(got[0], "bottom bar is not shown") || !strings.Contains(got[0], "Info_Native") {
		t.Errorf("want one MDL-NATIVEBAR02 for the bar naming its pages; got:\n%s", strings.Join(got, "\n"))
	}

	// One page granted to a role a user role has: that item shows, and so
	// does the bar; only the other item is reported.
	got = warnings("mdl 1;\ngrant view on page MyFirstModule.Home_Native to MyFirstModule.User;" + nav)
	if len(got) != 1 || !strings.Contains(got[0], "Info_Native") || strings.Contains(got[0], "bottom bar is not shown") {
		t.Errorf("with Home_Native granted, want only Info_Native reported; got:\n%s", strings.Join(got, "\n"))
	}

	// A module role no user role has is no viewer.
	got = warnings("mdl 1;\ncreate module role MyFirstModule.Nobody;\ngrant view on page MyFirstModule.Home_Native to MyFirstModule.Nobody;\n" +
		"grant view on page MyFirstModule.Info_Native to MyFirstModule.Nobody;" + nav)
	if len(got) != 1 || !strings.Contains(got[0], "bottom bar is not shown") {
		t.Errorf("a module role no user role holds must not count as a viewer; got:\n%s", strings.Join(got, "\n"))
	}

	// At security level Off nothing is checked.
	if got := warnings("mdl 1;\nalter project security level off;" + nav); len(got) != 0 {
		t.Errorf("security Off: want no warning, got:\n%s", strings.Join(got, "\n"))
	}
}
