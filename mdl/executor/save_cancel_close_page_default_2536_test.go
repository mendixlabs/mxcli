// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// ako/mxcli#2536 (mendixlabs/mxcli#1927): a bare `cancel changes` stored
// ClosePage false, so a pop-up's Cancel left the pop-up open. Studio Pro's
// default is to close — every save and cancel button in ako/TestApp stores
// ClosePage true (31 of 31) — and the skills' pop-up pages wrote the bare
// form. The bare form now closes the page; `with (ClosePage: false)` keeps it.
func TestSaveCancelClosePageDefault_2536(t *testing.T) {
	for _, tc := range []struct {
		action string
		want   bool
	}{
		{"cancel changes", true},
		{"save changes", true},
		{"cancel changes close page", true},
		{"save changes close page", true},
		{"cancel changes with (ClosePage: false)", false},
		{"save changes with (ClosePage: false)", false},
	} {
		prog, errs := visitor.Build("create page M.P (Title: 'T', Layout: A.L) {\n" +
			"  actionbutton b1 (Caption: 'Go', Action: " + tc.action + ")\n}")
		if len(errs) > 0 {
			t.Fatalf("%q does not parse: %v", tc.action, errs)
		}
		a := prog.Statements[0].(*ast.CreatePageStmtV3).Widgets[0].Properties["Action"].(*ast.ActionV3)
		built, err := (&pageBuilder{}).buildClientActionV3(a)
		if err != nil {
			t.Fatalf("build %q: %v", tc.action, err)
		}
		var got bool
		switch x := built.(type) {
		case *pages.SaveChangesClientAction:
			got = x.ClosePage
		case *pages.CancelChangesClientAction:
			got = x.ClosePage
		default:
			t.Fatalf("%q built %T", tc.action, built)
		}
		if got != tc.want {
			t.Errorf("%q stored ClosePage %v, want %v", tc.action, got, tc.want)
		}
	}
}
