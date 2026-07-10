// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
	"github.com/JordtenBulte-OLC/mxcli/mdl/backend/mock"
	"github.com/JordtenBulte-OLC/mxcli/sdk/javaactions"
)

func TestAddJavaAction_FileDocumentReturnRegistersSystemFileDocument(t *testing.T) {
	backend := &mock.MockBackend{
		ReadJavaActionByNameFunc: func(qualifiedName string) (*javaactions.JavaAction, error) {
			if qualifiedName != "Spreadsheet.ExportRows" {
				return nil, nil
			}
			return &javaactions.JavaAction{
				ReturnType: &javaactions.FileDocumentType{},
			}, nil
		},
	}

	fb := &flowBuilder{
		backend:      backend,
		varTypes:     map[string]string{},
		declaredVars: map[string]string{},
	}

	fb.addCallJavaActionAction(&ast.CallJavaActionStmt{
		OutputVariable: "GeneratedDocument",
		ActionName:     ast.QualifiedName{Module: "Spreadsheet", Name: "ExportRows"},
	})

	if got := fb.varTypes["GeneratedDocument"]; got != "System.FileDocument" {
		t.Fatalf("GeneratedDocument type = %q, want System.FileDocument", got)
	}
}

func TestAddJavaAction_ConcreteListReturnRegistersListType(t *testing.T) {
	backend := &mock.MockBackend{
		ReadJavaActionByNameFunc: func(qualifiedName string) (*javaactions.JavaAction, error) {
			return &javaactions.JavaAction{
				ReturnType: &javaactions.ListType{Entity: "Orders.Order"},
			}, nil
		},
	}

	fb := &flowBuilder{
		backend:      backend,
		varTypes:     map[string]string{},
		declaredVars: map[string]string{},
	}

	fb.addCallJavaActionAction(&ast.CallJavaActionStmt{
		OutputVariable: "FilteredOrders",
		ActionName:     ast.QualifiedName{Module: "Lists", Name: "FilterOrders"},
	})

	if got := fb.varTypes["FilteredOrders"]; got != "List of Orders.Order" {
		t.Fatalf("FilteredOrders type = %q, want list type", got)
	}
}

// TestAddJavaAction_EntityReturnRegistersEntityType pins ako's review follow-up
// for PR #357: the most common Java action return shape — a bare
// `*javaactions.EntityType{Entity: "Mod.Ent"}` — must register the output
// variable as the entity's qualified name so downstream attribute access
// resolves against the right domain-model entry.
func TestAddJavaAction_EntityReturnRegistersEntityType(t *testing.T) {
	backend := &mock.MockBackend{
		ReadJavaActionByNameFunc: func(qualifiedName string) (*javaactions.JavaAction, error) {
			return &javaactions.JavaAction{
				ReturnType: &javaactions.EntityType{Entity: "Orders.Order"},
			}, nil
		},
	}

	fb := &flowBuilder{
		backend:      backend,
		varTypes:     map[string]string{},
		declaredVars: map[string]string{},
		measurer:     &layoutMeasurer{},
	}

	fb.addCallJavaActionAction(&ast.CallJavaActionStmt{
		OutputVariable: "CreatedOrder",
		ActionName:     ast.QualifiedName{Module: "Orders", Name: "CreateFromPayload"},
	})

	if got := fb.varTypes["CreatedOrder"]; got != "Orders.Order" {
		t.Fatalf("CreatedOrder type = %q, want Orders.Order", got)
	}
}

func TestAddJavaAction_GenericListReturnInheritsInputListType(t *testing.T) {
	backend := &mock.MockBackend{
		ReadJavaActionByNameFunc: func(qualifiedName string) (*javaactions.JavaAction, error) {
			return &javaactions.JavaAction{
				ReturnType: &javaactions.ListType{},
			}, nil
		},
	}

	fb := &flowBuilder{
		varTypes: map[string]string{
			"InputOrders": "List of Orders.Order",
		},
		declaredVars: map[string]string{},
		backend:      backend,
		measurer:     &layoutMeasurer{},
	}

	fb.addCallJavaActionAction(&ast.CallJavaActionStmt{
		OutputVariable: "FilteredOrders",
		ActionName:     ast.QualifiedName{Module: "Lists", Name: "FilterGeneric"},
		Arguments: []ast.CallArgument{
			{Name: "InputList", Value: &ast.VariableExpr{Name: "InputOrders"}},
		},
	})

	if got := fb.varTypes["FilteredOrders"]; got != "List of Orders.Order" {
		t.Fatalf("generic java list result type = %q, want input list type", got)
	}
}
