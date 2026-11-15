// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// ako/mxcli#1943 (mendixlabs/mxcli#1740): `find $L by Number = <value>` was
// written as the find-by-EXPRESSION operation whenever the value held an
// operator without single spaces around it — `$N-1`, `-1`, Studio Pro's own
// `$Item/Number-1` — and describe then printed `find … where
// $currentObject/Number = …`. The spaced spelling was a find by attribute.
func TestFindByUnspacedValue_IsAFindByAttribute_Issue1943(t *testing.T) {
	for _, value := range []string{
		"$N - 1", // control: always worked
		"$N-1", "$N+1", "$N*2", "$N -1", "$N- 1", "-1", "$Item/Number-1",
	} {
		src := "create microflow M.F ($N: Integer) begin $Found = find $Items by Number = " + value + "; end;"
		prog, errs := visitor.Build(src)
		if len(errs) > 0 {
			t.Fatalf("%s: does not parse: %v", value, errs)
		}
		stmt := prog.Statements[0].(*ast.CreateMicroflowStmt).Body[0].(*ast.ListOperationStmt)

		fb := &flowBuilder{posX: 100, posY: 100, spacing: HorizontalSpacing,
			varTypes: map[string]string{"Items": "List of M.Item"}}
		fb.addListOperationAction(stmt)
		if len(fb.objects) != 1 {
			t.Fatalf("%s: got %d objects, want 1", value, len(fb.objects))
		}
		action := fb.objects[0].(*microflows.ActionActivity).Action.(*microflows.ListOperationAction)
		op, ok := action.Operation.(*microflows.FindByAttributeOperation)
		if !ok {
			t.Errorf("%s: written as %T, want a find by attribute", value, action.Operation)
			continue
		}
		if op.Attribute != "M.Item.Number" {
			t.Errorf("%s: Attribute = %q, want M.Item.Number", value, op.Attribute)
		}
		// The value is stored as the author spelled it, so a Studio Pro
		// `$Item/Number-1` round-trips unchanged.
		if op.Expression != value {
			t.Errorf("%s: Expression = %q, want %q", value, op.Expression, value)
		}
	}
}

// `where` keeps meaning the by-expression operation, however the condition is
// spelled.
func TestFindWhereUnspaced_StaysByExpression_Issue1943(t *testing.T) {
	prog, errs := visitor.Build("create microflow M.F ($N: Integer) begin $Found = find $Items where Number = $N-1; end;")
	if len(errs) > 0 {
		t.Fatalf("does not parse: %v", errs)
	}
	stmt := prog.Statements[0].(*ast.CreateMicroflowStmt).Body[0].(*ast.ListOperationStmt)
	fb := &flowBuilder{posX: 100, posY: 100, spacing: HorizontalSpacing,
		varTypes: map[string]string{"Items": "List of M.Item"}}
	fb.addListOperationAction(stmt)
	action := fb.objects[0].(*microflows.ActionActivity).Action.(*microflows.ListOperationAction)
	if _, ok := action.Operation.(*microflows.FindByAttributeOperation); ok {
		t.Errorf("`where` was written as a find by attribute")
	}
}
