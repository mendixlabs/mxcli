// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// DESCRIBE PAGE left out a pop-up page's width and height when they were 0,
// Studio Pro's auto-size, so a page set to auto-size read the same as one
// whose size was never looked at (mendixlabs/mxcli#1713). The pages are
// Studio Pro-authored: Administration.ChangePasswordForm sits on
// Atlas_Core.PopupLayout (ModalPopup) and stores PopupWidth/PopupHeight 0/0.
func TestDescribePage_PopupLayoutPrintsZeroDimensions(t *testing.T) {
	x, out := openTestAppCopy(t)
	describe := func(name string) string {
		t.Helper()
		out.Reset()
		if err := x.Execute(&ast.DescribeStmt{ObjectType: ast.DescribePage, Name: ast.QualifiedName{Module: "Administration", Name: name}}); err != nil {
			t.Fatalf("describe %s: %v", name, err)
		}
		return out.String()
	}

	popup := describe("ChangePasswordForm")
	if !strings.Contains(popup, "Layout: Atlas_Core.PopupLayout") {
		t.Fatalf("fixture moved: ChangePasswordForm is no longer on the pop-up layout:\n%s", popup)
	}
	for _, want := range []string{"PopupWidth: 0", "PopupHeight: 0"} {
		if !strings.Contains(popup, want) {
			t.Errorf("describe of a pop-up page does not print %q (#1713):\n%s", want, popup)
		}
	}

	// And they round-trip: executing the description changes nothing.
	if err := x.ExecuteProgram(reparse(t, popup)); err != nil {
		t.Fatalf("exec of the description: %v\n%s", err, popup)
	}
	if again := describe("ChangePasswordForm"); again != popup {
		t.Errorf("describe -> exec -> describe is not a fixed point:\n--- first ---\n%s\n--- second ---\n%s", popup, again)
	}

	// Control: on a page that does not open as a pop-up the dimensions do
	// nothing, and the default stays unprinted.
	if page := describe("Account_Overview"); strings.Contains(page, "PopupWidth") || strings.Contains(page, "PopupHeight") {
		t.Errorf("describe of a non-pop-up page prints its default pop-up size:\n%s", page)
	}
}

func TestPopupDimensionProps(t *testing.T) {
	for _, tc := range []struct {
		raw   map[string]any
		popup bool
		want  string
	}{
		{map[string]any{"PopupWidth": int32(0), "PopupHeight": int32(0)}, true, "PopupWidth: 0|PopupHeight: 0"},
		{map[string]any{"PopupWidth": int32(0), "PopupHeight": int32(0)}, false, ""},
		{map[string]any{"PopupWidth": int32(1000), "PopupHeight": int32(0)}, false, "PopupWidth: 1000"},
		{map[string]any{"PopupWidth": int32(1200), "PopupHeight": int32(480)}, true, "PopupWidth: 1200|PopupHeight: 480"},
	} {
		if got := strings.Join(popupDimensionProps(tc.raw, tc.popup), "|"); got != tc.want {
			t.Errorf("popupDimensionProps(%v, popup=%v) = %q, want %q", tc.raw, tc.popup, got, tc.want)
		}
	}
}
