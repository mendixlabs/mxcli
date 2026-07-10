// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/microflows"
)

// --- lookupRestOperation helper tests ---

func TestLookupRestOperation_Found(t *testing.T) {
	svc := &model.ConsumedRestService{
		BaseElement: model.BaseElement{ID: "svc-1"},
		Name:        "MyAPI",
		Operations: []*model.RestClientOperation{
			{Name: "PostData", BodyType: "json"},
		},
	}
	op := lookupRestOperation([]*model.ConsumedRestService{svc}, "MyAPI", "PostData")
	if op == nil {
		t.Fatal("expected operation to be found, got nil")
	}
	if op.Name != "PostData" {
		t.Errorf("got op.Name=%q, want PostData", op.Name)
	}
}

func TestLookupRestOperation_NotFound(t *testing.T) {
	svc := &model.ConsumedRestService{
		BaseElement: model.BaseElement{ID: "svc-1"},
		Name:        "MyAPI",
		Operations: []*model.RestClientOperation{
			{Name: "GetItems", BodyType: ""},
		},
	}
	op := lookupRestOperation([]*model.ConsumedRestService{svc}, "MyAPI", "PostData")
	if op != nil {
		t.Errorf("expected nil for unknown operation, got %+v", op)
	}
}

// --- buildRestParameterMappings helper tests ---

// Test: operation has path param $userId and query param $status.
// With clause binds both. Expect path in ParameterMappings, query in QueryParameterMappings.
func TestBuildRestParameterMappings_PathAndQuery(t *testing.T) {
	op := &model.RestClientOperation{
		Name: "GetUser",
		Parameters: []*model.RestClientParameter{
			{Name: "userId", DataType: "Integer"},
		},
		QueryParameters: []*model.RestClientParameter{
			{Name: "status", DataType: "String"},
		},
	}
	opQN := "Test.MyAPI.GetUser"

	params := []ast.SendRestParamDef{
		{Name: "userId", Expression: "$UserId"},
		{Name: "status", Expression: "'active'"},
	}

	pathMappings, queryMappings := buildRestParameterMappings(params, op, opQN)

	if len(pathMappings) != 1 {
		t.Fatalf("expected 1 path mapping, got %d", len(pathMappings))
	}
	if pathMappings[0].Parameter != "Test.MyAPI.GetUser.userId" {
		t.Errorf("got Parameter=%q, want Test.MyAPI.GetUser.userId", pathMappings[0].Parameter)
	}
	if pathMappings[0].Value != "$UserId" {
		t.Errorf("got Value=%q, want $UserId", pathMappings[0].Value)
	}

	if len(queryMappings) != 1 {
		t.Fatalf("expected 1 query mapping, got %d", len(queryMappings))
	}
	if queryMappings[0].Parameter != "Test.MyAPI.GetUser.status" {
		t.Errorf("got Parameter=%q, want Test.MyAPI.GetUser.status", queryMappings[0].Parameter)
	}
	if queryMappings[0].Value != "'active'" {
		t.Errorf("got Value=%q, want 'active'", queryMappings[0].Value)
	}
	if queryMappings[0].Included != "Yes" {
		t.Errorf("got Included=%q, want Yes", queryMappings[0].Included)
	}
}

// Test: no operation info available (nil op) → all params go to QueryParameterMappings.
func TestBuildRestParameterMappings_NilOp_FallbackToQuery(t *testing.T) {
	params := []ast.SendRestParamDef{
		{Name: "name", Expression: "$Name"},
		{Name: "email", Expression: "$Email"},
	}
	pathMappings, queryMappings := buildRestParameterMappings(params, nil, "Test.MyAPI.PostData")

	if len(pathMappings) != 0 {
		t.Errorf("expected 0 path mappings with nil op, got %d", len(pathMappings))
	}
	if len(queryMappings) != 2 {
		t.Errorf("expected 2 query mappings with nil op fallback, got %d", len(queryMappings))
	}
}

// --- shouldSetBodyVariable tests ---

// Test: JSON body → should NOT set BodyVariable.
func TestShouldSetBodyVariable_JsonBody_False(t *testing.T) {
	op := &model.RestClientOperation{BodyType: "json"}
	if shouldSetBodyVariable(op) {
		t.Error("expected shouldSetBodyVariable=false for json body, got true")
	}
}

// Test: TEMPLATE/STRING body → should NOT set BodyVariable.
func TestShouldSetBodyVariable_TemplateBody_False(t *testing.T) {
	op := &model.RestClientOperation{BodyType: "template"}
	if shouldSetBodyVariable(op) {
		t.Error("expected shouldSetBodyVariable=false for template body, got true")
	}
}

// Test: EXPORT_MAPPING body → should set BodyVariable.
func TestShouldSetBodyVariable_ExportMappingBody_True(t *testing.T) {
	op := &model.RestClientOperation{BodyType: "EXPORT_MAPPING"}
	if !shouldSetBodyVariable(op) {
		t.Error("expected shouldSetBodyVariable=true for EXPORT_MAPPING body, got false")
	}
}

// Test: nil op (operation not found) → should set BodyVariable (preserve old behavior).
func TestShouldSetBodyVariable_NilOp_True(t *testing.T) {
	if !shouldSetBodyVariable(nil) {
		t.Error("expected shouldSetBodyVariable=true for nil op (fallback), got false")
	}
}

// Test: empty BodyType (no body) → should NOT set BodyVariable.
func TestShouldSetBodyVariable_NoBody_False(t *testing.T) {
	op := &model.RestClientOperation{BodyType: ""}
	if shouldSetBodyVariable(op) {
		t.Error("expected shouldSetBodyVariable=false for empty BodyType, got true")
	}
}

// --- addSendRestRequestAction integration (via flowBuilder) ---

// Test: SEND REST REQUEST with JSON body — BodyVariable must be nil.
func TestAddSendRestRequest_JsonBody_NoBodyVariable(t *testing.T) {
	op := &model.RestClientOperation{
		Name:     "PostJsonTemplate",
		BodyType: "json",
		Parameters: []*model.RestClientParameter{
			{Name: "Name", DataType: "String"},
			{Name: "Email", DataType: "String"},
		},
	}
	svc := &model.ConsumedRestService{
		BaseElement: model.BaseElement{ID: "svc-1"},
		Name:        "RC_TestApi",
		Operations:  []*model.RestClientOperation{op},
	}

	fb := &flowBuilder{
		objects:      nil,
		flows:        nil,
		posX:         100,
		posY:         100,
		spacing:      200,
		varTypes:     map[string]string{},
		declaredVars: map[string]string{},
		restServices: []*model.ConsumedRestService{svc},
	}

	stmt := &ast.SendRestRequestStmt{
		Operation: ast.QualifiedName{Module: "MfTest", Name: "RC_TestApi.PostJsonTemplate"},
		Parameters: []ast.SendRestParamDef{
			{Name: "Name", Expression: "$Name"},
			{Name: "Email", Expression: "$Email"},
		},
		BodyVariable: "JsonBody",
	}

	fb.addSendRestRequestAction(stmt)

	if len(fb.objects) == 0 {
		t.Fatal("expected at least one object in flowBuilder after addSendRestRequestAction")
	}

	activity, ok := fb.objects[0].(*microflows.ActionActivity)
	if !ok {
		t.Fatalf("expected ActionActivity, got %T", fb.objects[0])
	}

	action, ok := activity.Action.(*microflows.RestOperationCallAction)
	if !ok {
		t.Fatalf("expected RestOperationCallAction, got %T", activity.Action)
	}

	// For JSON body, BodyVariable must be nil
	if action.BodyVariable != nil {
		t.Errorf("expected BodyVariable=nil for json body, got %+v", action.BodyVariable)
	}

	// Both params should be classified as path params (both are in op.Parameters)
	if len(action.ParameterMappings) != 2 {
		t.Errorf("expected 2 path parameter mappings, got %d", len(action.ParameterMappings))
	}
	if len(action.QueryParameterMappings) != 0 {
		t.Errorf("expected 0 query parameter mappings, got %d", len(action.QueryParameterMappings))
	}
}

// Test: SEND REST REQUEST with EXPORT_MAPPING body — BodyVariable must be set.
func TestAddSendRestRequest_ExportMappingBody_HasBodyVariable(t *testing.T) {
	op := &model.RestClientOperation{
		Name:     "PostEntity",
		BodyType: "EXPORT_MAPPING",
	}
	svc := &model.ConsumedRestService{
		BaseElement: model.BaseElement{ID: "svc-2"},
		Name:        "EntityAPI",
		Operations:  []*model.RestClientOperation{op},
	}

	fb := &flowBuilder{
		objects:      nil,
		flows:        nil,
		posX:         100,
		posY:         100,
		spacing:      200,
		varTypes:     map[string]string{},
		declaredVars: map[string]string{},
		restServices: []*model.ConsumedRestService{svc},
	}

	stmt := &ast.SendRestRequestStmt{
		Operation:    ast.QualifiedName{Module: "MfTest", Name: "EntityAPI.PostEntity"},
		BodyVariable: "MyEntity",
	}

	fb.addSendRestRequestAction(stmt)

	activity, ok := fb.objects[0].(*microflows.ActionActivity)
	if !ok {
		t.Fatalf("expected ActionActivity, got %T", fb.objects[0])
	}
	action, ok := activity.Action.(*microflows.RestOperationCallAction)
	if !ok {
		t.Fatalf("expected RestOperationCallAction, got %T", activity.Action)
	}

	if action.BodyVariable == nil {
		t.Error("expected BodyVariable to be set for EXPORT_MAPPING body, got nil")
	} else if action.BodyVariable.VariableName != "MyEntity" {
		t.Errorf("got BodyVariable.VariableName=%q, want MyEntity", action.BodyVariable.VariableName)
	}
}
