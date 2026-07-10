// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
)

func TestValidateMicroflow_InheritanceSplitAllBranchesReturn(t *testing.T) {
	stmt := &ast.CreateMicroflowStmt{
		Name: ast.QualifiedName{Module: "Sample", Name: "Route"},
		ReturnType: &ast.MicroflowReturnType{
			Type: ast.DataType{Kind: ast.TypeBoolean},
		},
		Body: []ast.MicroflowStatement{
			&ast.InheritanceSplitStmt{
				Variable: "Input",
				Cases: []ast.InheritanceSplitCase{
					{
						Entity: ast.QualifiedName{Module: "Sample", Name: "SpecializedInput"},
						Body: []ast.MicroflowStatement{
							&ast.ReturnStmt{Value: &ast.LiteralExpr{Kind: ast.LiteralBoolean, Value: true}},
						},
					},
				},
				ElseBody: []ast.MicroflowStatement{
					&ast.ReturnStmt{Value: &ast.LiteralExpr{Kind: ast.LiteralBoolean, Value: false}},
				},
			},
		},
	}

	violations := ValidateMicroflow(stmt)
	for _, violation := range violations {
		if violation.RuleID == "MDL003" {
			t.Fatalf("inheritance split with exhaustive returning branches must satisfy return validation: %#v", violation)
		}
	}
}
