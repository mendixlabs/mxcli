// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
	"github.com/JordtenBulte-OLC/mxcli/mdl/backend/mock"
	mdltypes "github.com/JordtenBulte-OLC/mxcli/mdl/types"
	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/microflows"
)

// TestAddRestCallAction_ReturnsResponseUsesHttpResponseHandling pins the
// builder behavior for issue #377: when MDL authors `rest call ... returns
// response`, the builder must construct a ResultHandlingHttpResponse so the
// writer's matching branch (PR #376) emits a DataTypes$ObjectType bound to
// System.HttpResponse, instead of falling back to a string variable.
func TestAddRestCallAction_ReturnsResponseUsesHttpResponseHandling(t *testing.T) {
	fb := &flowBuilder{
		posX:         100,
		posY:         100,
		spacing:      HorizontalSpacing,
		varTypes:     map[string]string{},
		declaredVars: map[string]string{},
		measurer:     &layoutMeasurer{},
	}

	stmt := &ast.RestCallStmt{
		OutputVariable: "Response",
		Method:         ast.HttpMethodGet,
		URL:            &ast.LiteralExpr{Kind: ast.LiteralString, Value: "https://example.com"},
		Result:         ast.RestResult{Type: ast.RestResultResponse},
	}
	fb.addRestCallAction(stmt)

	if len(fb.objects) == 0 {
		t.Fatalf("expected one activity, got %d", len(fb.objects))
	}

	activity, ok := fb.objects[0].(*microflows.ActionActivity)
	if !ok {
		t.Fatalf("first object is %T, want *microflows.ActionActivity", fb.objects[0])
	}
	action, ok := activity.Action.(*microflows.RestCallAction)
	if !ok {
		t.Fatalf("activity.Action is %T, want *microflows.RestCallAction", activity.Action)
	}

	httpResponse, ok := action.ResultHandling.(*microflows.ResultHandlingHttpResponse)
	if !ok {
		t.Fatalf("ResultHandling is %T, want *microflows.ResultHandlingHttpResponse", action.ResultHandling)
	}
	if httpResponse.VariableName != "Response" {
		t.Errorf("VariableName = %q, want %q", httpResponse.VariableName, "Response")
	}
}

// `as Module.Entity` (no `list of`) marks the REST result as a single
// object regardless of whether the underlying import mapping is
// list-typed. Studio Pro stores this on the microflow's
// ImportMappingCall (Range.SingleObject + ForceSingleOccurrence), so the
// builder must trust the explicit MDL syntax — using mapping shape as a
// proxy collides with cases like PCD's REST_GetEnvironmentByUUID, where
// the mapping has MaxOccurs=-1 but the call site binds a single Object.
func TestAddRestCallAction_MappingAsEntityProducesSingleObject(t *testing.T) {
	fb := &flowBuilder{
		posX:         100,
		posY:         100,
		spacing:      HorizontalSpacing,
		varTypes:     map[string]string{},
		declaredVars: map[string]string{},
		measurer:     &layoutMeasurer{},
	}

	stmt := &ast.RestCallStmt{
		OutputVariable: "Item",
		Method:         ast.HttpMethodGet,
		URL:            &ast.LiteralExpr{Kind: ast.LiteralString, Value: "https://example.com"},
		Result: ast.RestResult{
			Type:         ast.RestResultMapping,
			MappingName:  ast.QualifiedName{Module: "Synthetic", Name: "MsgDefMapping"},
			ResultEntity: ast.QualifiedName{Module: "Synthetic", Name: "Item"},
			IsList:       false,
		},
	}
	fb.addRestCallAction(stmt)

	activity := fb.objects[0].(*microflows.ActionActivity)
	action := activity.Action.(*microflows.RestCallAction)
	mapping := action.ResultHandling.(*microflows.ResultHandlingMapping)
	if !mapping.SingleObject {
		t.Errorf("SingleObject = false, want true (no `list of` => single object)")
	}
	if mapping.ForceSingleOccurrence == nil || !*mapping.ForceSingleOccurrence {
		t.Errorf("ForceSingleOccurrence = %v, want explicit true to mirror SingleObject", mapping.ForceSingleOccurrence)
	}
}

// `as list of Module.Entity` produces a list-typed result regardless of
// the import mapping's underlying shape. ForceSingleOccurrence mirrors
// SingleObject so the writer reproduces the BSON layout Studio Pro emits.
func TestAddRestCallAction_MappingAsListOfEntityProducesListResult(t *testing.T) {
	fb := &flowBuilder{
		posX:         100,
		posY:         100,
		spacing:      HorizontalSpacing,
		varTypes:     map[string]string{},
		declaredVars: map[string]string{},
		measurer:     &layoutMeasurer{},
	}

	stmt := &ast.RestCallStmt{
		OutputVariable: "Items",
		Method:         ast.HttpMethodGet,
		URL:            &ast.LiteralExpr{Kind: ast.LiteralString, Value: "https://example.com"},
		Result: ast.RestResult{
			Type:         ast.RestResultMapping,
			MappingName:  ast.QualifiedName{Module: "Synthetic", Name: "RepeatingObjectMapping"},
			ResultEntity: ast.QualifiedName{Module: "Synthetic", Name: "Item"},
			IsList:       true,
		},
	}
	fb.addRestCallAction(stmt)

	activity := fb.objects[0].(*microflows.ActionActivity)
	action := activity.Action.(*microflows.RestCallAction)
	mapping := action.ResultHandling.(*microflows.ResultHandlingMapping)
	if mapping.SingleObject {
		t.Errorf("SingleObject = true, want false (`list of` => list result)")
	}
	if mapping.ForceSingleOccurrence == nil || *mapping.ForceSingleOccurrence {
		t.Errorf("ForceSingleOccurrence = %v, want explicit false to mirror SingleObject", mapping.ForceSingleOccurrence)
	}
}

func TestAddRestCallAction_MappingResultPreservesExplicitOutputVariable(t *testing.T) {
	fb := &flowBuilder{
		posX:         100,
		posY:         100,
		spacing:      HorizontalSpacing,
		varTypes:     map[string]string{},
		declaredVars: map[string]string{},
		measurer:     &layoutMeasurer{},
		backend: &mock.MockBackend{
			GetImportMappingByQualifiedNameFunc: func(moduleName, name string) (*model.ImportMapping, error) {
				if moduleName != "Synthetic" || name != "ImportItems" {
					return nil, fmt.Errorf("unexpected import mapping %s.%s", moduleName, name)
				}
				return &model.ImportMapping{JsonStructure: "Synthetic.ItemsPayload"}, nil
			},
			GetJsonStructureByQualifiedNameFunc: func(moduleName, name string) (*mdltypes.JsonStructure, error) {
				if moduleName != "Synthetic" || name != "ItemsPayload" {
					return nil, fmt.Errorf("unexpected json structure %s.%s", moduleName, name)
				}
				return &mdltypes.JsonStructure{
					Elements: []*mdltypes.JsonElement{{ElementType: "Array"}},
				}, nil
			},
		},
	}

	stmt := &ast.RestCallStmt{
		OutputVariable: "Items",
		Method:         ast.HttpMethodGet,
		URL:            &ast.LiteralExpr{Kind: ast.LiteralString, Value: "https://example.com"},
		Result: ast.RestResult{
			Type:         ast.RestResultMapping,
			MappingName:  ast.QualifiedName{Module: "Synthetic", Name: "ImportItems"},
			ResultEntity: ast.QualifiedName{Module: "Synthetic", Name: "Item"},
		},
	}
	fb.addRestCallAction(stmt)

	activity, ok := fb.objects[0].(*microflows.ActionActivity)
	if !ok {
		t.Fatalf("first object is %T, want *microflows.ActionActivity", fb.objects[0])
	}
	action, ok := activity.Action.(*microflows.RestCallAction)
	if !ok {
		t.Fatalf("activity.Action is %T, want *microflows.RestCallAction", activity.Action)
	}
	mapping, ok := action.ResultHandling.(*microflows.ResultHandlingMapping)
	if !ok {
		t.Fatalf("ResultHandling is %T, want *microflows.ResultHandlingMapping", action.ResultHandling)
	}
	if action.OutputVariable != "Items" {
		t.Fatalf("OutputVariable = %q, want Items", action.OutputVariable)
	}
	if mapping.ResultVariable != "Items" {
		t.Fatalf("ResultVariable = %q, want Items", mapping.ResultVariable)
	}
}
