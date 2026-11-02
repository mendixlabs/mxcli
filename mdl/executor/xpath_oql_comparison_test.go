// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

func mdl091Messages(t *testing.T, src string) []string {
	t.Helper()
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse errors: %v", errs)
	}
	var msgs []string
	for _, vi := range ValidateMicroflow(prog.Statements[0].(*ast.CreateMicroflowStmt)) {
		if vi.RuleID == "MDL091" {
			msgs = append(msgs, vi.Message)
		}
	}
	return msgs
}

// mendixlabs/mxcli#1686: `like` in a retrieve constraint passed check and exec,
// then failed the build with CE0161 — the visitor had dropped the operator and
// stored `[Name]`. The siblings the shared OQL rule admits were dropped the same
// way (measured on mxbuild 11.12.2: `where N between 1 and 5` stored `[N]`).
func TestValidateMicroflow_XPathOQLComparison(t *testing.T) {
	cases := []struct {
		name  string
		where string
		want  string // the operator MDL091 must name, "" for none
	}{
		{"like", "Name like 'A-%'", "like"},
		{"like beside a comparison", "N = 1 and Name like 'A-%'", "like"},
		{"not like", "Name not like 'A-%'", "not like"},
		{"between", "N between 1 and 5", "between"},
		{"in", "N in (1, 2)", "in"},
		// Controls: the XPath spellings of the same tests, measured clean.
		{"starts-with", "starts-with(Name, 'A-')", ""},
		{"contains", "contains(Name, 'A')", ""},
		{"bounds", "N >= 1 and N <= 5", ""},
		{"like inside a literal is text", "Name = 'a like b'", ""},
		{"enum key", "Status = 'VALUE'", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			msgs := mdl091Messages(t, "create microflow M.F ()\nreturns list of M.Item\nbegin\n  retrieve $L from M.Item where "+tc.where+";\n  return $L;\nend;")
			if tc.want == "" {
				if len(msgs) > 0 {
					t.Fatalf("MDL091 fired on a valid constraint %q: %v", tc.where, msgs)
				}
				return
			}
			joined := strings.Join(msgs, "\n")
			if !strings.Contains(joined, "`"+tc.want+"`") || !strings.Contains(joined, "CE0161") {
				t.Fatalf("want MDL091 naming `%s` and CE0161 for %q, got %v", tc.want, tc.where, msgs)
			}
		})
	}
}

// The expression side of the same drop: `$s like 'A%'` was stored as `$s`.
func TestValidateMicroflow_ExpressionOQLComparison(t *testing.T) {
	bad := mdl091Messages(t, "create microflow M.F ($s: String) returns Boolean\nbegin\n  declare $b Boolean = $s like 'A%';\n  return $b;\nend;")
	if len(bad) != 1 || !strings.Contains(bad[0], "CE0117") || !strings.Contains(bad[0], "`like`") {
		t.Fatalf("want one MDL091 naming `like` and CE0117, got %v", bad)
	}
	if ok := mdl091Messages(t, "create microflow M.F ($s: String) returns Boolean\nbegin\n  declare $b Boolean = startsWith($s, 'A');\n  return $b;\nend;"); len(ok) > 0 {
		t.Fatalf("MDL091 fired on startsWith(): %v", ok)
	}
}

// The visitor keeps the comparison as written instead of dropping everything
// after the left operand.
func TestBuildExpression_KeepsOQLComparison(t *testing.T) {
	prog, errs := visitor.Build("create microflow M.F ()\nreturns list of M.Item\nbegin\n  retrieve $L from M.Item where Name like 'A-%';\n  return $L;\nend;")
	if len(errs) > 0 {
		t.Fatalf("parse errors: %v", errs)
	}
	mf := prog.Statements[0].(*ast.CreateMicroflowStmt)
	ret := mf.Body[0].(*ast.RetrieveStmt)
	if got := expressionToXPath(ret.Where); got != "Name like 'A-%'" {
		t.Fatalf("constraint serialized as %q, want the comparison as written", got)
	}
}

// check and exec must agree: the report had both accepting the constraint.
func TestCheckExecAgree_RetrieveConstraintLike(t *testing.T) {
	const head = `mdl 1;
create module ModG;
create persistent entity ModG.Item (Name: String(50));
create microflow ModG.SUB_Like ()
begin
  retrieve $L from ModG.Item where %s;
end;`
	exec, _, dir := openPedAppCopy(t)
	bad := strings.Replace(head, "%s", "Name like 'A-%'", 1)
	got := strings.Join(agreeCheck(t, exec, dir, bad), "\n")
	if !strings.Contains(got, "MDL091") || !strings.Contains(got, "`like`") {
		t.Errorf("check -p must report MDL091 for `like`, reported:\n%s", got)
	}
	if err := agreeExec(t, exec, bad); err == nil || !strings.Contains(err.Error(), "MDL091") {
		t.Errorf("exec must refuse `like` with MDL091, got: %v", err)
	}
	exec2, _, dir2 := openPedAppCopy(t)
	assertAgree(t, exec2, dir2, strings.Replace(head, "%s", "starts-with(Name, 'A-')", 1), "")
}
