// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
)

func TestValidateMicroflow_WhileTrueWithoutBreakDoesNotNeedFallthroughReturn(t *testing.T) {
	stmt := &ast.CreateMicroflowStmt{
		Name: ast.QualifiedName{Module: "Sample", Name: "PollUntilDone"},
		ReturnType: &ast.MicroflowReturnType{
			Type: ast.DataType{Kind: ast.TypeBoolean},
		},
		Body: []ast.MicroflowStatement{
			&ast.WhileStmt{
				Condition: &ast.LiteralExpr{Kind: ast.LiteralBoolean, Value: true},
				Body: []ast.MicroflowStatement{
					&ast.IfStmt{
						Condition: &ast.VariableExpr{Name: "Done"},
						ThenBody: []ast.MicroflowStatement{
							&ast.ReturnStmt{Value: &ast.LiteralExpr{Kind: ast.LiteralBoolean, Value: true}},
						},
					},
					&ast.LogStmt{Level: ast.LogInfo, Message: &ast.LiteralExpr{Kind: ast.LiteralString, Value: "retry"}},
				},
			},
		},
	}

	violations := ValidateMicroflow(stmt)
	for _, violation := range violations {
		if violation.RuleID == "MDL003" {
			t.Fatalf("while true without break must not require a fallthrough return: %#v", violation)
		}
	}
}

func TestValidateMicroflow_WhileTrueWithBreakStillNeedsFallthroughReturn(t *testing.T) {
	stmt := &ast.CreateMicroflowStmt{
		Name: ast.QualifiedName{Module: "Sample", Name: "MaybeBreak"},
		ReturnType: &ast.MicroflowReturnType{
			Type: ast.DataType{Kind: ast.TypeBoolean},
		},
		Body: []ast.MicroflowStatement{
			&ast.WhileStmt{
				Condition: &ast.LiteralExpr{Kind: ast.LiteralBoolean, Value: true},
				Body: []ast.MicroflowStatement{
					&ast.BreakStmt{},
				},
			},
		},
	}

	violations := ValidateMicroflow(stmt)
	for _, violation := range violations {
		if violation.RuleID == "MDL003" {
			return
		}
	}
	t.Fatalf("expected MDL003 for while true that can break, got %#v", violations)
}
