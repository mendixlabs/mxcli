// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// mendixlabs/mxcli#1735 (ako/mxcli#1939): Studio Pro's "Send external object"
// and "Delete external object" activities had no MDL. A flow that saves an
// external (OData) object could not be written — a plain commit on one is a
// Studio Pro error — and describe printed `-- Unsupported action`, so a drop +
// create of such a flow from its description silently lost the activity.

// externalObjectActionOf builds the one statement of a microflow body and
// returns the action the builder makes of it.
func externalObjectActionOf(t *testing.T, stmt string) microflows.MicroflowAction {
	t.Helper()
	src := "create microflow M.F ($District: M.District) begin\n  " + stmt + "\nend;"
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("%q does not parse: %v", stmt, errs)
	}
	mf := prog.Statements[0].(*ast.CreateMicroflowStmt)
	fb := &flowBuilder{varTypes: map[string]string{"District": "M.District"}}
	fb.addExternalObjectAction(mf.Body[0].(*ast.ExternalObjectStmt))
	if len(fb.errors) > 0 {
		t.Fatalf("%q: builder errors: %v", stmt, fb.errors)
	}
	return fb.objects[0].(*microflows.ActionActivity).Action
}

func TestExternalObject_SendAndDeleteBuildTheirActions(t *testing.T) {
	cases := []struct {
		stmt     string
		describe string
		check    func(t *testing.T, a microflows.MicroflowAction)
	}{
		{
			"send external object $District;", "send external object $District;",
			func(t *testing.T, a microflows.MicroflowAction) {
				s, ok := a.(*microflows.SendExternalObjectAction)
				if !ok || s.Variable != "District" || s.RefreshInClient {
					t.Fatalf("action = %#v, want a send of $District without refresh", a)
				}
			},
		},
		{
			"send external object $District refresh;", "send external object $District refresh;",
			func(t *testing.T, a microflows.MicroflowAction) {
				s, ok := a.(*microflows.SendExternalObjectAction)
				if !ok || !s.RefreshInClient {
					t.Fatalf("action = %#v, want a send with refresh", a)
				}
			},
		},
		{
			"delete external object $District;", "delete external object $District;",
			func(t *testing.T, a microflows.MicroflowAction) {
				d, ok := a.(*microflows.DeleteExternalObjectAction)
				if !ok || d.Variable != "District" || d.RefreshInClient {
					t.Fatalf("action = %#v, want a delete of $District without refresh", a)
				}
			},
		},
		{
			// The `on error` clause is printed by the flow walk, not by the
			// action's own line (as for every action).
			"delete external object $District refresh on error continue;", "delete external object $District refresh;",
			func(t *testing.T, a microflows.MicroflowAction) {
				d, ok := a.(*microflows.DeleteExternalObjectAction)
				if !ok || !d.RefreshInClient || d.ErrorHandlingType != microflows.ErrorHandlingTypeContinue {
					t.Fatalf("action = %#v, want a delete with refresh and on error continue", a)
				}
			},
		},
	}
	e := newTestExecutor()
	for _, tc := range cases {
		t.Run(tc.stmt, func(t *testing.T) {
			a := externalObjectActionOf(t, tc.stmt)
			tc.check(t, a)
			// describe prints what parses back to the same action.
			got := e.formatAction(a, nil, nil)
			if got != tc.describe {
				t.Fatalf("describe = %q, want %q", got, tc.describe)
			}
			if strings.HasPrefix(got, "--") {
				t.Fatalf("describe still prints the action as unsupported: %q", got)
			}
		})
	}
}

// A plain `delete $X` is still the Delete object activity: `external object`
// is what selects the external one.
func TestExternalObject_PlainDeleteIsUnchanged(t *testing.T) {
	src := "create microflow M.F ($Order: M.Order) begin\n  delete $Order;\nend;"
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatal(errs)
	}
	if _, ok := prog.Statements[0].(*ast.CreateMicroflowStmt).Body[0].(*ast.DeleteObjectStmt); !ok {
		t.Fatalf("`delete $Order` parsed as %T", prog.Statements[0].(*ast.CreateMicroflowStmt).Body[0])
	}
}

// `on error continue` on either activity is CE6035 (measured on 11.12.6, one
// microflow per project; the default and a custom handler, with or without
// rollback, are accepted). check refuses it before mxbuild does.
func TestExternalObject_ContinueIsRefused(t *testing.T) {
	for _, verb := range []string{"send", "delete"} {
		t.Run(verb, func(t *testing.T) {
			stmt := &ast.ExternalObjectStmt{Delete: verb == "delete", Variable: "D",
				ErrorHandling: &ast.ErrorHandlingClause{Type: ast.ErrorHandlingContinue}}
			keyword, activity := continueUnsupportedStatement(stmt)
			if keyword != verb+" external object" || activity == "" {
				t.Fatalf("continueUnsupportedStatement = %q, %q; want %q refused", keyword, activity, verb+" external object")
			}
		})
	}
}
