// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"os"
	"path/filepath"
	"testing"
)

// ako/mxcli#2562: MDL-WIDGET15 says adjacent inline dynamic texts run together
// because each renders as an HTML <span>. On a native page each is its own
// React Native Text element, so the note's reasoning does not hold there, and
// it was raised anyway.
func TestConsecutiveDynamicTextNotOnNativePages2562(t *testing.T) {
	count := func(src, project string) int {
		n := 0
		for _, v := range ValidateProgram(parseMDL(t, src), project) {
			if v.RuleID == "MDL-WIDGET15" {
				n++
			}
		}
		return n
	}
	page := func(name, layout string) string {
		return "mdl 1;\ncreate page MyFirstModule." + name + " (Title: 'T', Layout: " + layout + ") {\n" +
			"  dynamictext a (Content: 'x')\n  dynamictext b (Content: 'y')\n};"
	}

	// No project: a layout the script creates says which platform it is.
	native := "mdl 1;\ncreate or modify layout MyFirstModule.N (layouttype: 'Default', showbottombar: false) {\n  placeholder Main\n};\n" +
		page("P", "MyFirstModule.N")[len("mdl 1;\n"):]
	if got := count(native, ""); got != 0 {
		t.Errorf("a page on a native layout the script creates: %d MDL-WIDGET15, want 0", got)
	}
	// The control: a page whose layout nothing resolves keeps the note.
	if got := count(page("W", "Atlas_Core.Atlas_Default"), ""); got != 1 {
		t.Errorf("control, unresolved layout: %d MDL-WIDGET15, want 1", got)
	}

	// With a project: its layouts say.
	mpr := filepath.Join("..", "..", "testdata", "pedapp", "PedApp.mpr")
	if _, err := os.Stat(mpr); err != nil {
		t.Skipf("PedApp fixture not found: %v", err)
	}
	if got := count(page("P2", "Atlas_Core.NativePhone_Default"), mpr); got != 0 {
		t.Errorf("a page on the project's native layout: %d MDL-WIDGET15, want 0", got)
	}
	if got := count(page("W2", "Atlas_Core.Atlas_Default"), mpr); got != 1 {
		t.Errorf("control, the project's web layout: %d MDL-WIDGET15, want 1", got)
	}
}
