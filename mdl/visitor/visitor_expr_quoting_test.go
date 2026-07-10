// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
)

// Bug 1 — quoted attribute names must be stripped in expression contexts, the
// same way they are in binding/declaration contexts. Previously the raw source
// text (with quotes) leaked into the compiled expression and only mxbuild
// rejected it. See stripExpressionIdentifierQuotes.

func TestStripExpressionIdentifierQuotes(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"decision path", `$Expense/"Justification" != empty`, `$Expense/Justification != empty`},
		{"assoc path", `$Expense/"ExpenseApproval"."Expense_Employee" != empty`, `$Expense/ExpenseApproval.Expense_Employee != empty`},
		{"visibility", `$currentObject/"Amount" > 1000`, `$currentObject/Amount > 1000`},
		{"backtick", "$x/`Status` = empty", `$x/Status = empty`},
		{"no quotes", `$x/Status = empty`, `$x/Status = empty`},
		{"double-quote inside string literal is kept", `$x/Name = 'he said "hi"'`, `$x/Name = 'he said "hi"'`},
		{"escaped single quote kept", `$x/Name = 'it''s "here"'`, `$x/Name = 'it''s "here"'`},
		{"mixed", `$x/"Name" = 'a "b" c' and $x/"Age" > 1`, `$x/Name = 'a "b" c' and $x/Age > 1`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := stripExpressionIdentifierQuotes(tc.in); got != tc.want {
				t.Errorf("stripExpressionIdentifierQuotes(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// 1a — microflow decision / if condition.
func TestBug1_DecisionConditionStripsQuotes(t *testing.T) {
	prog, errs := Build(`create microflow M.Mf ($Expense: M.Expense) returns boolean as $R begin if $Expense/"Justification" != empty then return true; end if; return false; end`)
	if len(errs) > 0 {
		t.Fatalf("parse errors: %v", errs)
	}
	mf := prog.Statements[0].(*ast.CreateMicroflowStmt)
	var cond ast.Expression
	for _, s := range mf.Body {
		if ifs, ok := s.(*ast.IfStmt); ok {
			cond = ifs.Condition
		}
	}
	se, ok := cond.(*ast.SourceExpr)
	if !ok {
		t.Fatalf("expected *ast.SourceExpr, got %T", cond)
	}
	if want := `$Expense/Justification != empty`; se.Source != want {
		t.Errorf("decision Source = %q, want %q", se.Source, want)
	}
}

// 1b — widget contentparams expression.
func TestBug1_ContentParamsStripsQuotes(t *testing.T) {
	prog, errs := Build(`create page M.P (title: 'T', layout: Atlas_Core.Atlas_Default) {
  dataview dv (datasource: microflow, microflow: M.DS) {
    dynamictext qTitle (content: '{1}', contentparams: [{1} = "Title"])
  }
}`)
	if len(errs) > 0 {
		t.Fatalf("parse errors: %v", errs)
	}
	page := prog.Statements[0].(*ast.CreatePageStmtV3)
	w := findWidget(page.Widgets, "qTitle")
	if w == nil {
		t.Fatal("qTitle widget not found")
	}
	params, ok := w.Properties["ContentParams"].([]ast.ParamAssignmentV3)
	if !ok || len(params) != 1 {
		t.Fatalf("unexpected ContentParams: %#v", w.Properties["ContentParams"])
	}
	if want := "Title"; params[0].Value != want {
		t.Errorf("contentparam value = %q, want %q", params[0].Value, want)
	}
}

// 1c — widget visibility expression.
func TestBug1_VisibleExprStripsQuotes(t *testing.T) {
	prog, errs := Build(`create page M.P (title: 'T', layout: Atlas_Core.Atlas_Default) {
  dataview dv (datasource: microflow, microflow: M.DS) {
    dynamictext qReview (content: 'x', visible: [ "Amount" > 1000 ])
  }
}`)
	if len(errs) > 0 {
		t.Fatalf("parse errors: %v", errs)
	}
	page := prog.Statements[0].(*ast.CreatePageStmtV3)
	w := findWidget(page.Widgets, "qReview")
	if w == nil {
		t.Fatal("qReview widget not found")
	}
	if want := "$currentObject/Amount > 1000"; w.Properties["VisibleIf"] != want {
		t.Errorf("VisibleIf = %q, want %q", w.Properties["VisibleIf"], want)
	}
}

// 1d — XPath database datasource WHERE constraint. The worst variant: a quoted
// attribute compiled to the string literal 'Status', making the constraint
// always false — passes check + mxbuild, silently returns zero rows.
func TestBug1d_DatasourceWhereStripsQuotes(t *testing.T) {
	prog, errs := Build(`create page M.P (title: 'T', layout: Atlas_Core.Atlas_Default) {
  gallery g (datasource: database from M.Expense where [ "Status" = 'Submitted' ]) {
    dynamictext t (content: 'x')
  }
}`)
	if len(errs) > 0 {
		t.Fatalf("parse errors: %v", errs)
	}
	page := prog.Statements[0].(*ast.CreatePageStmtV3)
	w := findWidget(page.Widgets, "g")
	if w == nil {
		t.Fatal("gallery not found")
	}
	ds, ok := w.Properties["DataSource"].(*ast.DataSourceV3)
	if !ok || ds == nil {
		t.Fatalf("unexpected DataSource: %#v", w.Properties["DataSource"])
	}
	if want := "[Status = 'Submitted']"; ds.Where != want {
		t.Errorf("datasource Where = %q, want %q", ds.Where, want)
	}
}

// A quoted value that happens to be an XPath keyword ("empty"/"true"/"false")
// must stay an attribute name, not collapse to the literal.
func TestBug1d_QuotedKeywordAttributeStaysMember(t *testing.T) {
	prog, errs := Build(`create page M.P (title: 'T', layout: Atlas_Core.Atlas_Default) {
  gallery g (datasource: database from M.Expense where [ "empty" != empty ]) {
    dynamictext t (content: 'x')
  }
}`)
	if len(errs) > 0 {
		t.Fatalf("parse errors: %v", errs)
	}
	page := prog.Statements[0].(*ast.CreatePageStmtV3)
	ds := findWidget(page.Widgets, "g").Properties["DataSource"].(*ast.DataSourceV3)
	if want := "[empty != empty]"; ds.Where != want {
		t.Errorf("datasource Where = %q, want %q", ds.Where, want)
	}
}

func findWidget(ws []*ast.WidgetV3, name string) *ast.WidgetV3 {
	for _, w := range ws {
		if w.Name == name {
			return w
		}
		if got := findWidget(w.Children, name); got != nil {
			return got
		}
	}
	return nil
}
