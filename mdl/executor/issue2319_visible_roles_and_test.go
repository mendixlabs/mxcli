// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"reflect"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// ako/mxcli#2319: `Visible: roles (Shop.Admin) and $currentObject/Secret =
// 'x'` passed check and exec, and the whole text — `roles (…)` is not Mendix
// expression syntax — was stored as the visibility expression, with
// ModuleRoles empty. The role restriction was lost and the expression invalid.
//
// Studio Pro keeps "visible for module roles" and the condition on one
// settings element, both applying; that is what the leading `roles (…) and`
// means, so it is stored that way. A `roles (…)` anywhere else in an
// expression has no such reading and is refused.

func visibleWidget(t *testing.T, visible string) *ast.WidgetV3 {
	t.Helper()
	prog, errs := visitor.Build("create page Shop.P (Title: 'x', Layout: A.L) {\n  container c1 (Visible: " + visible + ") { }\n}\n")
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs)
	}
	return prog.Statements[0].(*ast.CreatePageStmtV3).Widgets[0]
}

func TestVisibleRolesAndExpression_SplitIntoRolesAndCondition_Issue2319(t *testing.T) {
	w := visibleWidget(t, "roles (Shop.Admin, Shop.User) and $currentObject/Secret = 'x'")
	if roles, _ := w.Properties["VisibleRoles"].([]string); !reflect.DeepEqual(roles, []string{"Shop.Admin", "Shop.User"}) {
		t.Errorf("VisibleRoles = %#v, want [Shop.Admin Shop.User]", w.Properties["VisibleRoles"])
	}
	if got := w.GetStringProp("VisibleIf"); got != "$currentObject/Secret = 'x'" {
		t.Errorf("VisibleIf = %q, want the condition alone", got)
	}
}

// CONTROL: a plain expression stays an expression, roles untouched.
func TestVisibleExpressionWithoutRoles_Unchanged_Issue2319(t *testing.T) {
	w := visibleWidget(t, "$currentObject/Secret = 'roles (x) and y'")
	if _, has := w.Properties["VisibleRoles"]; has {
		t.Errorf("VisibleRoles set from a string literal: %#v", w.Properties["VisibleRoles"])
	}
	if got := w.GetStringProp("VisibleIf"); got != "$currentObject/Secret = 'roles (x) and y'" {
		t.Errorf("VisibleIf = %q", got)
	}
}

func TestVisibleRolesElsewhereInExpression_Refused_Issue2319(t *testing.T) {
	for _, visible := range []string{
		"$currentObject/Secret = 'x' and roles (Shop.Admin)",
		"roles (Shop.Admin) or $currentObject/Secret = 'x'",
	} {
		w := visibleWidget(t, visible)
		vs := validateWidgetExpressionRoles(w, "page Shop.P")
		if len(vs) != 1 || vs[0].RuleID != "MDL-WIDGET54" || !strings.Contains(vs[0].Message, "Visible: roles (") {
			t.Errorf("%s: violations = %v, want one MDL-WIDGET54 naming the supported spelling", visible, vs)
		}
	}
	// CONTROL: the split form and a plain expression are clean.
	for _, visible := range []string{"roles (Shop.Admin) and $currentObject/Secret = 'x'", "$currentObject/Secret = 'x'"} {
		if vs := validateWidgetExpressionRoles(visibleWidget(t, visible), "page Shop.P"); len(vs) != 0 {
			t.Errorf("%s: unexpected %v", visible, vs)
		}
	}
}

// exec refuses it too, for a script run past check (--no-check).
func TestVisibleRolesElsewhereInExpression_ExecRefuses_Issue2319(t *testing.T) {
	w := visibleWidget(t, "$currentObject/Secret = 'x' and roles (Shop.Admin)")
	if err := refuseRolesInExpressions(w); err == nil || !strings.Contains(err.Error(), "roles (") {
		t.Errorf("exec guard = %v, want a refusal", err)
	}
	if err := refuseRolesInExpressions(visibleWidget(t, "roles (Shop.Admin) and $currentObject/Secret = 'x'")); err != nil {
		t.Errorf("split form refused: %v", err)
	}
}
