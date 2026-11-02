// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

func ifOperandMessages(t *testing.T, src string) []string {
	t.Helper()
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse errors: %v", errs)
	}
	var msgs []string
	switch s := prog.Statements[0].(type) {
	case *ast.CreateMicroflowStmt:
		for _, vi := range ValidateMicroflow(s) {
			if vi.RuleID == ifOperandRule {
				msgs = append(msgs, vi.Message)
			}
		}
	case *ast.CreateNanoflowStmt:
		for _, vi := range ValidateNanoflow(s) {
			if vi.RuleID == ifOperandRule {
				msgs = append(msgs, vi.Message)
			}
		}
	}
	return msgs
}

// mendixlabs/mxcli#1710: `'x' + if $c then 'a' else 'b'` passed check and exec
// and failed the build with CE0117. Every row was measured on mxbuild 11.12.2.
func TestValidateMicroflow_IfOperand(t *testing.T) {
	cases := []struct {
		name string
		body string
		want bool
	}{
		{"right of +", "declare $L String = 'x' + if $c then 'a' else 'b';\n  return true;", true},
		{"right of and", "declare $L Boolean = $c and if $c then true else false;\n  return $L;", true},
		{"right of =", "declare $L Boolean = $c = if $c then true else false;\n  return $L;", true},
		{"unary minus", "declare $L Integer = -if $c then 1 else 2;\n  return true;", true},
		{"in a return", "return 'x' + if $c then 'a' else 'b' = 'xa';", true},
		// Controls, measured clean.
		{"parenthesised", "declare $L String = 'x' + (if $c then 'a' else 'b');\n  return true;", false},
		{"else takes the +", "declare $L String = if $c then 'a' else 'b' + 'x';\n  return true;", false},
		{"function argument", "declare $L String = toUpperCase(if $c then 'a' else 'b');\n  return true;", false},
		{"not() argument", "declare $L Boolean = not(if $c then true else false);\n  return $L;", false},
		{"if in the else branch", "declare $L String = if $c then 'a' else if $c then 'b' else 'c';\n  return true;", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			msgs := ifOperandMessages(t, "create microflow M.F ($c: Boolean) returns Boolean\nbegin\n  "+tc.body+"\nend;")
			if got := len(msgs) > 0; got != tc.want {
				t.Fatalf("%s fired=%v, want %v: %v", ifOperandRule, got, tc.want, msgs)
			}
			if tc.want && !strings.Contains(strings.Join(msgs, "\n"), "CE0117") {
				t.Errorf("message must name CE0117: %v", msgs)
			}
		})
	}
}

// Nanoflows share the per-expression hook.
func TestValidateNanoflow_IfOperand(t *testing.T) {
	msgs := ifOperandMessages(t, "create nanoflow M.N ($c: Boolean) returns String\nbegin\n  declare $L String = 'x' + if $c then 'a' else 'b';\n  return $L;\nend;")
	if len(msgs) != 1 {
		t.Fatalf("want one %s in a nanoflow, got %v", ifOperandRule, msgs)
	}
}

// check and exec must agree: the report had both accepting the expression.
func TestCheckExecAgree_IfOperand(t *testing.T) {
	const head = `mdl 1;
create module ModG;
create microflow ModG.SUB_IfOperand ($c: Boolean)
returns String
begin
  declare $L String = %s;
  return $L;
end;`
	exec, _, dir := openPedAppCopy(t)
	bad := strings.Replace(head, "%s", "'x' + if $c then 'a' else 'b'", 1)
	got := strings.Join(agreeCheck(t, exec, dir, bad), "\n")
	if !strings.Contains(got, ifOperandRule) {
		t.Errorf("check -p must report %s, reported:\n%s", ifOperandRule, got)
	}
	if err := agreeExec(t, exec, bad); err == nil || !strings.Contains(err.Error(), ifOperandRule) {
		t.Errorf("exec must refuse with %s, got: %v", ifOperandRule, err)
	}
	exec2, _, dir2 := openPedAppCopy(t)
	assertAgree(t, exec2, dir2, strings.Replace(head, "%s", "'x' + (if $c then 'a' else 'b')", 1), "")
}
