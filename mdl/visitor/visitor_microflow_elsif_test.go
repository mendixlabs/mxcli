// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// TestIfElsifArmsPreserved guards the "ELSIF arm silently dropped on write"
// bug: buildIfStatement only read the first condition/body pair and the
// trailing ELSE body, so every middle ELSIF arm of
// IF … THEN … (ELSIF … THEN …)* ELSE … END IF vanished from the AST — and
// therefore from the written .mpr — with no error. Mendix has no native
// elsif construct, so the visitor must lower each ELSIF arm into a nested
// IfStmt in the ELSE branch of the arm before it.
func TestIfElsifArmsPreserved(t *testing.T) {
	prog, errs := Build(`create microflow M.TestElsif ($In: Integer)
returns String as $Out
begin
  declare $Out String = 'none';
  if $In = 1 then
    set $Out = 'one';
  elsif $In = 2 then
    set $Out = 'two';
  elsif $In = 3 then
    set $Out = 'three';
  else
    set $Out = 'other';
  end if;
  return $Out;
end;`)
	if len(errs) > 0 {
		t.Fatalf("parse errors: %v", errs)
	}
	mf := prog.Statements[0].(*ast.CreateMicroflowStmt)

	var outer *ast.IfStmt
	for _, s := range mf.Body {
		if ifs, ok := s.(*ast.IfStmt); ok {
			outer = ifs
		}
	}
	if outer == nil {
		t.Fatal("no IfStmt found in microflow body")
	}

	// Expect a 3-level chain: if $In=1 → elsif $In=2 → elsif $In=3 → else.
	wantConds := []string{"$In = 1", "$In = 2", "$In = 3"}
	cur := outer
	for level, want := range wantConds {
		if cur == nil {
			t.Fatalf("level %d: expected a nested IfStmt for ELSIF arm %q, got none (arm dropped)", level, want)
		}
		if got := condSource(cur.Condition); got != "" && got != want {
			t.Errorf("level %d: condition = %q, want %q", level, got, want)
		}
		if len(cur.ThenBody) != 1 {
			t.Errorf("level %d: ThenBody has %d statements, want 1", level, len(cur.ThenBody))
		}
		if !cur.HasElse {
			t.Fatalf("level %d: HasElse = false, want true (chain must continue)", level)
		}
		if level == len(wantConds)-1 {
			// Innermost arm carries the original ELSE body.
			if len(cur.ElseBody) != 1 {
				t.Fatalf("innermost ElseBody has %d statements, want 1 (the ELSE arm)", len(cur.ElseBody))
			}
			if _, ok := cur.ElseBody[0].(*ast.IfStmt); ok {
				t.Fatal("innermost ElseBody is another IfStmt; want the plain ELSE statement")
			}
			return
		}
		if len(cur.ElseBody) != 1 {
			t.Fatalf("level %d: ElseBody has %d statements, want exactly the nested IfStmt", level, len(cur.ElseBody))
		}
		next, ok := cur.ElseBody[0].(*ast.IfStmt)
		if !ok {
			t.Fatalf("level %d: ElseBody[0] is %T, want *ast.IfStmt (lowered ELSIF arm)", level, cur.ElseBody[0])
		}
		cur = next
	}
}

// TestIfElsifWithoutElse — the chain must also work with no trailing ELSE:
// the innermost lowered arm then has no ELSE branch at all.
func TestIfElsifWithoutElse(t *testing.T) {
	prog, errs := Build(`create microflow M.TestElsifNoElse ($In: Integer)
begin
  if $In = 1 then
    log info 'one';
  elsif $In = 2 then
    log info 'two';
  end if;
end;`)
	if len(errs) > 0 {
		t.Fatalf("parse errors: %v", errs)
	}
	mf := prog.Statements[0].(*ast.CreateMicroflowStmt)
	var outer *ast.IfStmt
	for _, s := range mf.Body {
		if ifs, ok := s.(*ast.IfStmt); ok {
			outer = ifs
		}
	}
	if outer == nil {
		t.Fatal("no IfStmt found in microflow body")
	}
	if !outer.HasElse || len(outer.ElseBody) != 1 {
		t.Fatalf("outer arm: HasElse=%v ElseBody len=%d, want the lowered ELSIF in ELSE", outer.HasElse, len(outer.ElseBody))
	}
	inner, ok := outer.ElseBody[0].(*ast.IfStmt)
	if !ok {
		t.Fatalf("outer ElseBody[0] is %T, want *ast.IfStmt (the ELSIF arm)", outer.ElseBody[0])
	}
	if len(inner.ThenBody) != 1 {
		t.Errorf("inner ThenBody has %d statements, want 1", len(inner.ThenBody))
	}
	if inner.HasElse || len(inner.ElseBody) != 0 {
		t.Errorf("inner arm: HasElse=%v ElseBody len=%d, want no ELSE (source had none)", inner.HasElse, len(inner.ElseBody))
	}
}

// TestPlainIfElseUnchanged — regression guard: the common IF/ELSE (no ELSIF)
// shape must build exactly as before, with no gratuitous nesting.
func TestPlainIfElseUnchanged(t *testing.T) {
	prog, errs := Build(`create microflow M.TestPlain ($In: Integer)
begin
  if $In = 1 then
    log info 'one';
  else
    log info 'other';
  end if;
end;`)
	if len(errs) > 0 {
		t.Fatalf("parse errors: %v", errs)
	}
	mf := prog.Statements[0].(*ast.CreateMicroflowStmt)
	var outer *ast.IfStmt
	for _, s := range mf.Body {
		if ifs, ok := s.(*ast.IfStmt); ok {
			outer = ifs
		}
	}
	if outer == nil {
		t.Fatal("no IfStmt found in microflow body")
	}
	if len(outer.ThenBody) != 1 || !outer.HasElse || len(outer.ElseBody) != 1 {
		t.Fatalf("plain if/else shape changed: Then=%d HasElse=%v Else=%d", len(outer.ThenBody), outer.HasElse, len(outer.ElseBody))
	}
	if _, nested := outer.ElseBody[0].(*ast.IfStmt); nested {
		t.Fatal("plain ELSE body was wrapped in a nested IfStmt")
	}
}

// condSource extracts the preserved source text of a condition when the
// visitor kept it (SourceExpr); returns "" when it did not, in which case
// the caller skips the textual comparison (structure is asserted regardless).
func condSource(e ast.Expression) string {
	if se, ok := e.(*ast.SourceExpr); ok {
		return se.Source
	}
	return ""
}
