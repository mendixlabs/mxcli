// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// A quoted member in a `case` source was kept with its quotes: the split
// expression became `$Item/"Color"`, which mxbuild fails CE0117. Everywhere
// else (if, set, change, return) the quotes are stripped (ako/mxcli#2605,
// upstream mendixlabs/mxcli#1956).
func TestCaseSourceQuotedMember_2605(t *testing.T) {
	for src, want := range map[string]string{
		`$Item/"Color"`:                       `Item/Color`,
		`$Item/Color`:                         `Item/Color`,
		`$Item/"Repro"."Item_Parent"/"Color"`: `Item/Repro.Item_Parent/Color`,
	} {
		prog, errs := Build("create microflow Repro.MF ($Item: Repro.Item) returns Boolean\nbegin\n  case " + src +
			"\n    when Red then return true;\n    when Green, (empty) then return false;\n  end case;\nend;")
		if len(errs) > 0 {
			t.Fatalf("%s: parse: %v", src, errs[0])
		}
		body := prog.Statements[0].(*ast.CreateMicroflowStmt).Body
		cs, ok := body[0].(*ast.EnumSplitStmt)
		if !ok {
			t.Fatalf("%s: first statement %T, want a case", src, body[0])
		}
		if cs.Variable != want {
			t.Errorf("case %s: source %q, want %q", src, cs.Variable, want)
		}
	}
}
