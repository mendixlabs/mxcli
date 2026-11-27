// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// ako/mxcli#2618: a `case` whose values move between its `when` branches, or
// that gains a new value on an existing branch, is the stored case restated —
// its branches are spliced, and the values put on them in place. It was
// compared with the values as part of the shell, so any value change replaced
// the whole split, which the splice refuses for more than two paths.
func caseWith(values [][]string, bodies ...string) *ast.EnumSplitStmt {
	c := &ast.EnumSplitStmt{Variable: "Kind"}
	for i, vs := range values {
		c.Cases = append(c.Cases, ast.EnumSplitCase{
			Value: vs[0], Values: vs,
			Body: []ast.MicroflowStatement{&ast.LogStmt{Level: ast.LogInfo, Message: &ast.LiteralExpr{Value: bodies[i], Kind: ast.LiteralString}}},
		})
	}
	return c
}

func TestCaseValuesMovedIsTheSameShell_2618(t *testing.T) {
	stored := caseWith([][]string{{"A"}, {"B", "D"}, {"C"}}, "a", "b", "c")
	moved := caseWith([][]string{{"A", "D"}, {"B"}, {"C"}}, "a", "b", "c")
	if _, _, ok := sameCaseShell(moved, stored); !ok {
		t.Fatal("a value moved to another branch is not the same case")
	}
	added := caseWith([][]string{{"A"}, {"B", "D"}, {"C", "E"}}, "a", "b", "c")
	if _, _, ok := sameCaseShell(added, stored); !ok {
		t.Error("a value added to a branch is not the same case")
	}

	// A branch taken out (B's, with B and D joining A) and one added are the
	// same case too: branches pair by a shared value, and the rest are new or
	// removed.
	fewer := caseWith([][]string{{"A", "B", "D"}, {"C"}}, "a", "c")
	if _, _, ok := sameCaseShell(fewer, stored); !ok {
		t.Fatal("a branch taken out is not the same case")
	}
	pairs, removed := caseBranchPairs(fewer, stored)
	if len(pairs) != 2 || pairs[0] != 0 || pairs[1] != 2 || len(removed) != 1 || removed[0] != 1 {
		t.Errorf("pairs %v removed %v, want [0 2] and [1]", pairs, removed)
	}
	more := caseWith([][]string{{"A"}, {"B", "D"}, {"C"}, {"E"}}, "a", "b", "c", "e")
	if pairs, removed := caseBranchPairs(more, stored); pairs[3] != -1 || len(removed) != 0 {
		t.Errorf("a new branch: pairs %v removed %v, want the last unpaired and none removed", pairs, removed)
	}
	// Control: a case sharing no value with the stored one is not it.
	other := caseWith([][]string{{"X"}}, "x")
	if _, _, ok := sameCaseShell(other, stored); ok {
		t.Error("a case with no branch in common counted as the same case")
	}
}

func TestSameCaseValueList_2618(t *testing.T) {
	for _, tc := range []struct {
		a, b []string
		want bool
	}{
		{[]string{"A", "B"}, []string{"B", "A"}, true},
		{[]string{"A"}, []string{"a"}, true},
		{[]string{"A"}, []string{"A", "D"}, false},
		{[]string{"A", "B"}, []string{"A", "C"}, false},
	} {
		if got := sameCaseValueList(tc.a, tc.b); got != tc.want {
			t.Errorf("%v vs %v = %v, want %v", tc.a, tc.b, got, tc.want)
		}
	}
}
