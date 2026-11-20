// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

func flowBody(t *testing.T, body string) []ast.MicroflowStatement {
	t.Helper()
	prog, errs := visitor.Build("mdl 1;\ncreate microflow M.F ($A: Boolean, $B: Boolean, $N: Integer) returns String\nbegin\n" + body + "\nend;\n")
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs[0])
	}
	return prog.Statements[0].(*ast.CreateMicroflowStmt).Body
}

// Two spellings of one graph are one canonical form (ako/mxcli#859); the
// authored spelling is on the left, describe's on the right.
func TestCanonicalFlow_SpellingsOfOneGraphMatch(t *testing.T) {
	for _, c := range []struct{ name, authored, described string }{
		{"guard clause before the final return",
			"if $N < 0 then return 'n'; end if; return 'p';",
			"if $N < 0 then return 'n'; else return 'p'; end if;"},
		{"guard clause before activities",
			"if $N < 0 then return 'n'; end if; declare $S String = 'p'; return $S;",
			"if $N < 0 then return 'n'; else declare $S String = 'p'; return $S; end if;"},
		{"guards in sequence",
			"if $A then return 'a'; end if; if $B then return 'b'; end if; return '';",
			"if $A then return 'a'; else if $B then return 'b'; else return ''; end if; end if;"},
		{"nested guard sharing the outer merge",
			"if $A then if $B then return 'b'; end if; end if; return '';",
			"if $A then if $B then return 'b'; end if; end if; join shared1; merge shared1; return '';"},
		// ako/mxcli#2259: describe prints the empty else inside a handler too.
		{"an if without else in an error handler",
			"$R = call microflow M.G () on error without rollback begin if $A then log info 'a'; end if; return 'e'; end error; return $R;",
			"$R = call microflow M.G () on error without rollback begin if $A then log info 'a'; else end if; return 'e'; end error; return $R;"},
	} {
		t.Run(c.name, func(t *testing.T) {
			a, d := canonicalFlow(flowBody(t, c.authored)), canonicalFlow(flowBody(t, c.described))
			if !declaredMatches(a, d) {
				t.Errorf("the two spellings do not match after canonicalFlow:\n  authored:  %#v\n  described: %#v", a, d)
			}
		})
	}
}

// Controls: what the canonical form must not merge.
func TestCanonicalFlow_DifferentGraphsStayDifferent(t *testing.T) {
	for _, c := range []struct{ name, a, b string }{
		// The then-branch goes on to the merge: not a guard.
		{"an if whose then-branch continues",
			"if $A then declare $S String = 'a'; end if; return '';",
			"if $A then declare $S String = 'a'; else return ''; end if;"},
		// A merge another path joins is a join point, and stays.
		{"a merge two paths join",
			"if $A then join m; else join m; end if; merge m; return '';",
			"if $A then declare $S String = 'a'; else declare $T String = 'b'; end if; return '';"},
		{"another return value",
			"if $N < 0 then return 'n'; end if; return 'p';",
			"if $N < 0 then return 'n'; else return 'q'; end if;"},
		{"an else that does something, in an error handler",
			"$R = call microflow M.G () on error without rollback begin if $A then log info 'a'; end if; return 'e'; end error; return $R;",
			"$R = call microflow M.G () on error without rollback begin if $A then log info 'a'; else log info 'b'; end if; return 'e'; end error; return $R;"},
	} {
		t.Run(c.name, func(t *testing.T) {
			if declaredMatches(canonicalFlow(flowBody(t, c.a)), canonicalFlow(flowBody(t, c.b))) {
				t.Error("two different graphs match after canonicalFlow")
			}
		})
	}
}

// The script's own statements are not rewritten: the mdl 0 rebuild may still
// build from them.
func TestCanonicalFlow_LeavesTheInputAlone(t *testing.T) {
	body := flowBody(t, "if $A then return 'a'; else return 'b'; end if;")
	_ = canonicalFlow(body)
	if s := body[0].(*ast.IfStmt); len(s.ElseBody) != 1 || !s.HasElse {
		t.Fatal("canonicalFlow rewrote the statement it was given")
	}
}
