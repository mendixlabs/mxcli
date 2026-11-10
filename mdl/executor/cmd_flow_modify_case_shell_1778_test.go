// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// ako/mxcli#2035 (mendixlabs/mxcli#1778): a `create or modify` that changed
// one activity in one branch of a `case` — or that describe and the script
// spelled differently somewhere inside it — was refused: the diff had no shell
// match for a case, as it has for an `if`, so the whole split was replaced,
// and the splice replaces a decision of two paths only ("it has 9 paths, not
// two"). sameCaseShell is the match that lets each branch be spliced instead.

func caseStmt(subject string, branches map[string]string, order ...string) *ast.EnumSplitStmt {
	c := &ast.EnumSplitStmt{Variable: subject}
	for _, v := range order {
		c.Cases = append(c.Cases, ast.EnumSplitCase{
			Value: v, Values: []string{v},
			Body: []ast.MicroflowStatement{&ast.LogStmt{Level: ast.LogInfo, Message: &ast.LiteralExpr{Value: branches[v], Kind: ast.LiteralString}}},
		})
	}
	return c
}

func TestSameCaseShell(t *testing.T) {
	base := map[string]string{"A": "a", "B": "b", "C": "c"}
	changed := map[string]string{"A": "a", "B": "CHANGED", "C": "c"}
	for _, tc := range []struct {
		name     string
		declared *ast.EnumSplitStmt
		want     bool
	}{
		{"a change inside one branch is the same shell", caseStmt("Root/Src", changed, "A", "B", "C"), true},
		{"control: another subject", caseStmt("Root/Other", changed, "A", "B", "C"), false},
		{"control: a branch fewer", caseStmt("Root/Src", changed, "A", "B"), false},
		{"control: branches reordered", caseStmt("Root/Src", changed, "B", "A", "C"), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stored := caseStmt("Root/Src", base, "A", "B", "C")
			if _, _, got := sameCaseShell(tc.declared, stored); got != tc.want {
				t.Errorf("sameCaseShell = %v, want %v", got, tc.want)
			}
		})
	}
	t.Run("control: an else on one side only", func(t *testing.T) {
		d, s := caseStmt("Root/Src", base, "A", "B"), caseStmt("Root/Src", base, "A", "B")
		d.ElseBody = []ast.MicroflowStatement{&ast.LogStmt{Level: ast.LogInfo}}
		if _, _, got := sameCaseShell(d, s); got {
			t.Error("an else added counted as the same shell")
		}
	})
	t.Run("control: an if is not a case", func(t *testing.T) {
		if _, _, got := sameCaseShell(&ast.IfStmt{}, caseStmt("Root/Src", base, "A")); got {
			t.Error("an if matched a case")
		}
	})
}

// The reported statement changed two activities before the case as well:
// the case then sits in one run with them, which was replaced first-and-drop-
// the-rest, dropping the split. casePair finds the case in the run so it is
// diffed on its own.
func TestCasePairFindsTheRestatedCaseInARun(t *testing.T) {
	base := map[string]string{"A": "a", "B": "b"}
	changed := map[string]string{"A": "a", "B": "CHANGED"}
	log := func(m string) ast.MicroflowStatement {
		return &ast.LogStmt{Level: ast.LogInfo, Message: &ast.LiteralExpr{Value: m, Kind: ast.LiteralString}}
	}
	ins := []ast.MicroflowStatement{log("new1"), log("new2"), caseStmt("Root/Src", changed, "A", "B")}
	del := []ast.MicroflowStatement{log("old1"), log("old2"), caseStmt("Root/Src", base, "A", "B")}
	if i, j, ok := casePair(ins, del); !ok || i != 2 || j != 2 {
		t.Fatalf("casePair = (%d, %d, %v), want (2, 2, true)", i, j, ok)
	}
	// Control: a case on another subject is not the stored one restated.
	ins[2] = caseStmt("Root/Other", changed, "A", "B")
	if _, _, ok := casePair(ins, del); ok {
		t.Error("a case on another subject was paired")
	}
}
