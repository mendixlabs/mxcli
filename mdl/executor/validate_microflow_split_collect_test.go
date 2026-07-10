// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
	"github.com/JordtenBulte-OLC/mxcli/mdl/backend/mock"
	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/microflows"
)

// flowRefCollector.collectFromStatements must descend into EnumSplitStmt
// case bodies and the else body. A regression in PR #475's first revision
// left the EnumSplitStmt branch with an empty case body in a Go type
// switch, so the loop walking case bodies was silently stolen by the
// next case (InheritanceSplitStmt). The result was that microflow calls
// inside any `case ... when ... then ...` branch escaped reference
// validation.
func TestValidateMicroflowReferences_DescendsIntoEnumSplitCases(t *testing.T) {
	moduleID := model.ID("module-1")
	backend := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) {
			return []*model.Module{{
				BaseElement: model.BaseElement{ID: moduleID},
				Name:        "SyntheticAudit",
			}}, nil
		},
		ListMicroflowsFunc: func() ([]*microflows.Microflow, error) {
			return nil, nil
		},
	}
	ctx, _ := newMockCtx(t, withBackend(backend))

	stmt := &ast.CreateMicroflowStmt{
		Name: ast.QualifiedName{Module: "SyntheticAudit", Name: "RouteByStatus"},
		Body: []ast.MicroflowStatement{
			&ast.EnumSplitStmt{
				Variable: "Status",
				Cases: []ast.EnumSplitCase{
					{
						Values: []string{"Open"},
						Body: []ast.MicroflowStatement{
							&ast.CallMicroflowStmt{
								MicroflowName: ast.QualifiedName{Module: "SyntheticAudit", Name: "MissingHandler"},
							},
						},
					},
				},
			},
		},
	}

	err := validate(ctx, stmt)
	if err == nil {
		t.Fatal("expected reference error for microflow inside enum split case body")
	}
	if !strings.Contains(err.Error(), "microflow not found: SyntheticAudit.MissingHandler") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// And the else body of an EnumSplitStmt must also be walked.
func TestValidateMicroflowReferences_DescendsIntoEnumSplitElse(t *testing.T) {
	moduleID := model.ID("module-1")
	backend := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) {
			return []*model.Module{{
				BaseElement: model.BaseElement{ID: moduleID},
				Name:        "SyntheticAudit",
			}}, nil
		},
		ListMicroflowsFunc: func() ([]*microflows.Microflow, error) {
			return nil, nil
		},
	}
	ctx, _ := newMockCtx(t, withBackend(backend))

	stmt := &ast.CreateMicroflowStmt{
		Name: ast.QualifiedName{Module: "SyntheticAudit", Name: "RouteByStatus"},
		Body: []ast.MicroflowStatement{
			&ast.EnumSplitStmt{
				Variable: "Status",
				ElseBody: []ast.MicroflowStatement{
					&ast.CallMicroflowStmt{
						MicroflowName: ast.QualifiedName{Module: "SyntheticAudit", Name: "MissingFallback"},
					},
				},
			},
		},
	}

	err := validate(ctx, stmt)
	if err == nil {
		t.Fatal("expected reference error for microflow inside enum split else body")
	}
	if !strings.Contains(err.Error(), "microflow not found: SyntheticAudit.MissingFallback") {
		t.Fatalf("unexpected error: %v", err)
	}
}
