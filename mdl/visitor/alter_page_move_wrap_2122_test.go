// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// ako/mxcli#2122: `move` and `wrap` parse to their operations.
func TestAlterPage_MoveAndWrapParse_Issue2122(t *testing.T) {
	prog, errs := Build(`mdl 1;
alter page Shop.P {
  wrap dgLines in { dataview dvOrder (DataSource: $Order) { } }
  move txtTitle into dvOrder;
  move btnA after btnB;
};`)
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs)
	}
	ops := prog.Statements[0].(*ast.AlterPageStmt).Operations
	if len(ops) != 3 {
		t.Fatalf("got %d operations, want 3", len(ops))
	}
	wrap, ok := ops[0].(*ast.WrapWidgetOp)
	if !ok || wrap.Target.Widget != "dgLines" || len(wrap.Container) != 1 || wrap.Container[0].Name != "dvOrder" {
		t.Errorf("wrap: %#v", ops[0])
	}
	move, ok := ops[1].(*ast.MoveWidgetOp)
	if !ok || move.Target.Widget != "txtTitle" || move.Position != "INTO" || move.Destination.Widget != "dvOrder" {
		t.Errorf("move into: %#v", ops[1])
	}
	after, ok := ops[2].(*ast.MoveWidgetOp)
	if !ok || after.Position != "AFTER" || after.Destination.Widget != "btnB" {
		t.Errorf("move after: %#v", ops[2])
	}
	// `wrap` stays usable as a name.
	if _, errs := Build("mdl 1;\ncreate persistent entity M.E (Wrap: String(10));"); len(errs) > 0 {
		t.Errorf("wrap as an attribute name: %v", errs)
	}
}
