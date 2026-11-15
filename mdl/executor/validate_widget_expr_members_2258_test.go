// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"context"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// ako/mxcli#2258: a widget expression naming a member the entity does not have
// passed check and exec, and the build failed with CE0117. Measured with mx
// check on 11.12.6, one widget per case:
//
//	Visible:  $currentObject/NoSuchAttr != empty                     CE0117
//	Editable: $currentObject/NoSuchAttr2 != empty  (text box)         CE0117
//	DynamicClasses: if $currentObject/NoSuchAttr3 = empty then …      CE0117
//	Visible:  $Product/NoSuchAttr4 != empty  (page parameter)         CE0117
//	Visible:  $currentObject/PartModel_Product/Sku != empty  (bare)   CE0117
//	Visible:  $currentObject/Mod.PartModel_Product/…  (qualified)     CE0117 (MDL-WIDGET13)
//	Visible:  $currentObject/Sku != empty                             clean
func TestWidgetExpressionMembers(t *testing.T) {
	exec := typeCheckFixture(t) // stored: MyFirstModule.Ticket (Title, Status), Ticket_Reporter
	ctx := exec.newExecContext(context.Background())

	judge := func(src string) []string {
		t.Helper()
		prog := parseMDL(t, src)
		s, ok := prog.Statements[0].(*ast.CreatePageStmtV3)
		if !ok {
			t.Fatalf("got %T", prog.Statements[0])
		}
		var out []string
		for _, e := range validatePluggableAttributeScopes(ctx, "", s.Parameters, s.Widgets, newScriptContext()) {
			if strings.Contains(e, "CE0117") {
				out = append(out, e)
			}
		}
		return out
	}
	page := func(body string) string {
		return `create page MyFirstModule.P (title: 'P', layout: Atlas_Core.Atlas_Default,
		  params: ($Ticket: MyFirstModule.Ticket)) { dataview dv (datasource: $Ticket) { ` + body + ` } };`
	}

	for _, tc := range []struct{ name, body, want string }{
		{"missing attribute in Visible", `container c (Visible: $currentObject/NoSuchAttr != empty) { }`, `has no attribute "NoSuchAttr"`},
		{"missing attribute in Editable", `textbox tb (attribute: Title, Editable: $currentObject/Nope != empty)`, `has no attribute "Nope"`},
		{"missing attribute in DynamicClasses", `container c (DynamicClasses: if $currentObject/Nope = empty then 'a' else 'b') { }`, `has no attribute "Nope"`},
		{"missing attribute on a page parameter", `container c (Visible: $Ticket/Nope != empty) { }`, `has no attribute "Nope"`},
		{"bare association", `container c (Visible: $currentObject/Ticket_Reporter/Email != empty) { }`, "cannot follow associations"},
	} {
		got := judge(page(tc.body))
		if len(got) != 1 || !strings.Contains(got[0], tc.want) {
			t.Errorf("%s: got %q, want one CE0117 error containing %q", tc.name, got, tc.want)
		}
	}
	// Controls: real members, the system members, and a qualified association
	// (MDL-WIDGET13's, not this check's).
	for _, body := range []string{
		`container c (Visible: $currentObject/Title != empty and $Ticket/Status != empty) { }`,
		`container c (Visible: $currentObject/MyFirstModule.Ticket_Reporter/MyFirstModule.Reporter/Email != empty) { }`,
		`container c (Visible: true) { }`,
	} {
		if got := judge(page(body)); len(got) != 0 {
			t.Errorf("%s: reported %q", body, got)
		}
	}
}

// A list view's specialization template renders its own entity. The walk
// judged a template's widgets against the list view's generalized source, so
// a specialization's attribute was "no attribute" — seven such errors on
// TestApp's Studio Pro-authored Pages.Vehicle templates, and the expression
// check would have added more.
func TestWidgetExpressionMembers_TemplateUsesItsSpecialization(t *testing.T) {
	exec := typeCheckFixture(t)
	ctx := exec.newExecContext(context.Background())
	prog := parseMDL(t, `create persistent entity MyFirstModule.Bug extends MyFirstModule.Ticket ( Severity: Integer );
create page MyFirstModule.P (title: 'P', layout: Atlas_Core.Atlas_Default) {
  listview lv (datasource: database MyFirstModule.Ticket) {
    dynamictext t (content: '{1}', contentparams: ({1} = Title))
    template for MyFirstModule.Bug {
      container cOk (Visible: $currentObject/Severity > 1) { }
      container cBad (Visible: $currentObject/Nope > 1) { }
    }
  }
};`)
	sc := newScriptContext()
	for _, st := range prog.Statements {
		sc.collectSingle(st)
	}
	s := prog.Statements[1].(*ast.CreatePageStmtV3)
	var got []string
	for _, e := range validatePluggableAttributeScopes(ctx, "", s.Parameters, s.Widgets, sc) {
		if strings.Contains(e, "no attribute") {
			got = append(got, e)
		}
	}
	if len(got) != 1 || !strings.Contains(got[0], "cBad") || !strings.Contains(got[0], "MyFirstModule.Bug") {
		t.Errorf("got %q, want only cBad's Nope, judged on MyFirstModule.Bug", got)
	}
}
