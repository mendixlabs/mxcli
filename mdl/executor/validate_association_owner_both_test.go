// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

func ownerBothProg(owner ast.OwnerType, from, to string, extends map[string]string) *ast.Program {
	qn := func(s string) ast.QualifiedName {
		i := strings.Index(s, ".")
		return ast.QualifiedName{Module: s[:i], Name: s[i+1:]}
	}
	prog := &ast.Program{}
	for _, n := range []string{"M.A", "M.B", "M.C"} {
		e := &ast.CreateEntityStmt{Name: qn(n)}
		if p, ok := extends[n]; ok {
			g := qn(p)
			e.Generalization = &g
		}
		prog.Statements = append(prog.Statements, e)
	}
	a := assocStmt("M.Link", from, to)
	a.Owner = owner
	prog.Statements = append(prog.Statements, a)
	return prog
}

// Measured on MxBuild 11.16.1: CE0002 for owner Both when both ends are the same
// entity or one specializes the other, for Reference and ReferenceSet alike; an
// unrelated pair and any other owner are clean (controls).
func TestValidateAssociationOwnerBoth(t *testing.T) {
	ext := map[string]string{"M.B": "M.A"}
	cases := []struct {
		name    string
		owner   ast.OwnerType
		from    string
		to      string
		extends map[string]string
		want    bool
	}{
		{"self", ast.OwnerBoth, "M.A", "M.A", nil, true},
		{"self, other owner (control)", ast.OwnerDefault, "M.A", "M.A", nil, false},
		{"self, owner parent (control)", ast.OwnerParent, "M.A", "M.A", nil, false},
		{"child to parent", ast.OwnerBoth, "M.B", "M.A", ext, true},
		{"parent to child", ast.OwnerBoth, "M.A", "M.B", ext, true},
		{"unrelated (control)", ast.OwnerBoth, "M.A", "M.C", ext, false},
		{"grandchild chain", ast.OwnerBoth, "M.C", "M.A", map[string]string{"M.B": "M.A", "M.C": "M.B"}, true},
		{"case-insensitive", ast.OwnerBoth, "M.a", "M.A", nil, true},
		{"related but not declared (control)", ast.OwnerBoth, "M.B", "M.A", nil, false},
		{"cyclic generalization terminates", ast.OwnerBoth, "M.C", "M.A", map[string]string{"M.A": "M.B", "M.B": "M.A"}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			v := ValidateAssociationOwnerBoth(ownerBothProg(c.owner, c.from, c.to, c.extends))
			if got := len(v) > 0; got != c.want {
				t.Fatalf("violations=%v, want refusal=%v", v, c.want)
			}
			if c.want && (v[0].RuleID != associationOwnerBothRule || !strings.Contains(v[0].Message, "CE0002")) {
				t.Errorf("rule/message should name %s and CE0002: %+v", associationOwnerBothRule, v[0])
			}
		})
	}
}
