// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
)

// Issue #642 — `where '<xpath>'` (quoted, with '' escapes) must un-escape to the
// same constraint as the inline `where [<xpath>]` form, not carry the raw token
// forward (which got bracket-wrapped into ['[Title=''abc'']'] and failed CE0161).

func retrieveWhere(t *testing.T, mfBody string) ast.Expression {
	t.Helper()
	prog, errs := Build("create microflow M.Mf () begin " + mfBody + " return; end;")
	if len(errs) > 0 {
		t.Fatalf("parse errors: %v", errs)
	}
	mf, ok := prog.Statements[0].(*ast.CreateMicroflowStmt)
	if !ok {
		t.Fatalf("expected CreateMicroflowStmt, got %T", prog.Statements[0])
	}
	for _, s := range mf.Body {
		if r, ok := s.(*ast.RetrieveStmt); ok {
			return r.Where
		}
	}
	t.Fatal("no RetrieveStmt in body")
	return nil
}

func sourceOf(t *testing.T, where ast.Expression) string {
	t.Helper()
	se, ok := where.(*ast.SourceExpr)
	if !ok {
		t.Fatalf("expected *ast.SourceExpr, got %T", where)
	}
	return se.Source
}

func TestRetrieveQuotedWhere_BracketedXPath(t *testing.T) {
	// Quoted form with internal '' escapes.
	got := sourceOf(t, retrieveWhere(t, `retrieve $L from M.E where '[Title=''abc'']';`))
	if got != "[Title='abc']" {
		t.Errorf("quoted where source = %q, want [Title='abc']", got)
	}
}

func TestRetrieveQuotedWhere_NoBrackets(t *testing.T) {
	got := sourceOf(t, retrieveWhere(t, `retrieve $L from M.E where 'Title = ''abc''';`))
	if got != "Title = 'abc'" {
		t.Errorf("quoted where source = %q, want Title = 'abc'", got)
	}
}

// The inline form still resolves to the inner constraint text (no regression).
func TestRetrieveInlineWhere_Unchanged(t *testing.T) {
	got := sourceOf(t, retrieveWhere(t, `retrieve $L from M.E where [Title='abc'];`))
	if got != "Title='abc'" {
		t.Errorf("inline where source = %q, want Title='abc'", got)
	}
}

// Page/datasource helper: a bare quoted string uses its unquoted value; other
// expressions serialize and bracket-wrap normally.
func TestBracketedXPathFromExpr(t *testing.T) {
	cases := []struct {
		name string
		expr ast.Expression
		want string
	}{
		{"bracketed literal", &ast.LiteralExpr{Value: "[Title='abc']", Kind: ast.LiteralString}, "[Title='abc']"},
		{"unbracketed literal", &ast.LiteralExpr{Value: "Title = 'abc'", Kind: ast.LiteralString}, "[Title = 'abc']"},
		{"binary expr", &ast.BinaryExpr{
			Left:     &ast.IdentifierExpr{Name: "Title"},
			Operator: "=",
			Right:    &ast.LiteralExpr{Value: "abc", Kind: ast.LiteralString},
		}, "[Title = 'abc']"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := bracketedXPathFromExpr(c.expr); got != c.want {
				t.Errorf("bracketedXPathFromExpr = %q, want %q", got, c.want)
			}
		})
	}
}
