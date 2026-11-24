// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"
)

// ako/mxcli#2560: a NativePhone home page whose layout shows the bottom bar
// must be one of the bar's items, or mx check fails with CE6108. check and exec
// accepted it whenever the script did not happen to list the page.
func TestNativeHomeBottomBar2560(t *testing.T) {
	exec, out, dir := openPedAppCopy(t)
	const pages = `mdl 1;
create page MyFirstModule.Home_Native (Title: 'Menu', Layout: Atlas_Core.NativePhone_Default) { dynamictext t (Content: 'x') };
create page MyFirstModule.Info_Native (Title: 'Info', Layout: Atlas_Core.NativePhone_Default) { dynamictext t (Content: 'y') };
create page MyFirstModule.Plain_Native (Title: 'Plain', Layout: Atlas_Core.NativePhone_TopBarOnly) { dynamictext t (Content: 'z') };`
	if err := agreeExec(t, exec, pages); err != nil {
		t.Fatalf("setup: %v\n%s", err, out.String())
	}
	nav := func(home, items string) string {
		return "mdl 1;\ncreate or modify navigation NativePhone\n  home page " + home + "\n  {\n" + items + "  };"
	}
	info := "    menu item 'Info' ( OnClick: show page MyFirstModule.Info_Native, Icon: glyph 57378 )\n"
	home := "    menu item 'Menu' ( OnClick: show page MyFirstModule.Home_Native, Icon: glyph 57377 )\n"

	t.Run("home page on a bottom-bar layout, not in the bar", func(t *testing.T) {
		assertAgree(t, exec, dir, nav("MyFirstModule.Home_Native", info), "CE6108")
	})
	t.Run("control: the home page is in the bar", func(t *testing.T) {
		assertAgree(t, exec, dir, nav("MyFirstModule.Home_Native", home+info), "")
	})
	t.Run("control: a layout without the bottom bar", func(t *testing.T) {
		assertAgree(t, exec, dir, nav("MyFirstModule.Plain_Native", info), "")
	})
	t.Run("a layout and page the same script creates", func(t *testing.T) {
		src := `mdl 1;
create or modify layout MyFirstModule.Native_Main (layouttype: 'Default', showbottombar: true) {
  placeholder Main
  region header { placeholder "Header" }
};
create page MyFirstModule.Scripted_Native (Title: 'S', Layout: MyFirstModule.Native_Main) { dynamictext t (Content: 's') };
` + strings.TrimPrefix(nav("MyFirstModule.Scripted_Native", info), "mdl 1;\n")
		got := strings.Join(agreeCheck(t, exec, dir, src), "\n")
		if !strings.Contains(got, "CE6108") {
			t.Errorf("check -p does not see the layout and page the script creates:\n%s", got)
		}
	})
}
