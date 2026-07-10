// SPDX-License-Identifier: Apache-2.0

package adapters

import (
	"bytes"
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
)

func TestExecAdapter_PrintsHintAndReturnsSerialised(t *testing.T) {
	var buf bytes.Buffer
	a := NewExecAdapter(&buf, nil)
	expr := &ast.SourceExpr{
		Source: "$x = true",
	}
	out := a.ExprToBSON("IfStmt.Condition", expr, "M.F")
	if out != "$x = true" {
		t.Errorf("returned source = %q", out)
	}
	if buf.Len() != 0 {
		t.Errorf("unexpected hint output: %s", buf.String())
	}
}
