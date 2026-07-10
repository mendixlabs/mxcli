// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
	"github.com/JordtenBulte-OLC/mxcli/sdk/javaactions"
	"github.com/JordtenBulte-OLC/mxcli/sdk/microflows"
)

// =============================================================================
// formatAction — Java Action Call with EntityTypeCodeActionParameterValue
// =============================================================================

func TestFormatAction_JavaActionCall_EntityTypeParam(t *testing.T) {
	e := newTestExecutor()
	action := &microflows.JavaActionCallAction{
		JavaAction:         "MyModule.Validate",
		ResultVariableName: "IsValid",
		UseReturnVariable:  true,
		ParameterMappings: []*microflows.JavaActionParameterMapping{
			{
				Parameter: "MyModule.Validate.InputObject",
				Value: &microflows.EntityTypeCodeActionParameterValue{
					Entity: "MyModule.Customer",
				},
			},
		},
	}
	got := e.formatAction(action, nil, nil)
	want := "$IsValid = call java action MyModule.Validate(InputObject = 'MyModule.Customer');"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestFormatAction_JavaActionCall_MixedParamTypes(t *testing.T) {
	e := newTestExecutor()
	action := &microflows.JavaActionCallAction{
		JavaAction:         "MyModule.ProcessEntity",
		ResultVariableName: "Result",
		UseReturnVariable:  true,
		ParameterMappings: []*microflows.JavaActionParameterMapping{
			{
				Parameter: "MyModule.ProcessEntity.InputObject",
				Value: &microflows.EntityTypeCodeActionParameterValue{
					Entity: "MyModule.Order",
				},
			},
			{
				Parameter: "MyModule.ProcessEntity.Label",
				Value: &microflows.ExpressionBasedCodeActionParameterValue{
					Expression: "'Process this'",
				},
			},
		},
	}
	got := e.formatAction(action, nil, nil)
	want := "$Result = call java action MyModule.ProcessEntity(InputObject = 'MyModule.Order', Label = 'Process this');"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestFormatAction_JavaActionCall_EntityTypeParam_EmptyEntity(t *testing.T) {
	e := newTestExecutor()
	action := &microflows.JavaActionCallAction{
		JavaAction:         "MyModule.Validate",
		ResultVariableName: "IsValid",
		UseReturnVariable:  true,
		ParameterMappings: []*microflows.JavaActionParameterMapping{
			{
				Parameter: "MyModule.Validate.InputObject",
				Value:     &microflows.EntityTypeCodeActionParameterValue{},
			},
		},
	}
	got := e.formatAction(action, nil, nil)
	// Empty entity param is omitted from argument list
	want := "$IsValid = call java action MyModule.Validate();"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// =============================================================================
// Java Action Type Helpers
// =============================================================================

func TestFormatJavaActionType_EntityTypeParameterType(t *testing.T) {
	typ := &javaactions.EntityTypeParameterType{
		TypeParameterName: "pEntity",
	}
	got := formatJavaActionType(typ)
	if got != "entity <pEntity>" {
		t.Errorf("got %q, want %q", got, "entity <pEntity>")
	}
}

func TestFormatJavaActionType_EntityTypeParameterType_NoName(t *testing.T) {
	typ := &javaactions.EntityTypeParameterType{}
	got := formatJavaActionType(typ)
	if got != "entity <>" {
		t.Errorf("got %q, want %q", got, "entity <>")
	}
}

func TestFormatJavaActionReturnType_TypeParameter(t *testing.T) {
	typ := &javaactions.TypeParameter{
		TypeParameter: "someId",
	}
	got := formatJavaActionReturnType(typ)
	if got != "someId" {
		t.Errorf("got %q, want %q", got, "someId")
	}
}

func TestFormatJavaActionReturnType_Nil(t *testing.T) {
	got := formatJavaActionReturnType(nil)
	if got != "Void" {
		t.Errorf("got %q, want %q", got, "Void")
	}
}

func TestFormatJavaActionType_Nil(t *testing.T) {
	got := formatJavaActionType(nil)
	if got != "Object" {
		t.Errorf("got %q, want %q", got, "Object")
	}
}

// =============================================================================
// isTypeParamRef & getTypeParamRefName
// =============================================================================

func TestIsTypeParamRef_BareName(t *testing.T) {
	typeParamNames := map[string]bool{"pEntity": true}

	// Bare name matching a type parameter (parsed as TypeEnumeration with no module)
	dt := ast.DataType{
		Kind:    ast.TypeEnumeration,
		EnumRef: &ast.QualifiedName{Name: "pEntity"},
	}
	if !isTypeParamRef(dt, typeParamNames) {
		t.Error("Expected bare name 'pEntity' to be a type parameter reference")
	}
}

func TestIsTypeParamRef_QualifiedName(t *testing.T) {
	typeParamNames := map[string]bool{"pEntity": true}

	// Qualified name should NOT be a type parameter ref
	dt := ast.DataType{
		Kind:    ast.TypeEnumeration,
		EnumRef: &ast.QualifiedName{Module: "MyModule", Name: "Customer"},
	}
	if isTypeParamRef(dt, typeParamNames) {
		t.Error("Expected qualified name to not be a type parameter reference")
	}
}

func TestIsTypeParamRef_NonMatchingName(t *testing.T) {
	typeParamNames := map[string]bool{"pEntity": true}

	// Bare name that doesn't match any type parameter
	dt := ast.DataType{
		Kind:    ast.TypeEnumeration,
		EnumRef: &ast.QualifiedName{Name: "SomeOtherName"},
	}
	if isTypeParamRef(dt, typeParamNames) {
		t.Error("Expected non-matching bare name to not be a type parameter reference")
	}
}

func TestIsTypeParamRef_PrimitiveType(t *testing.T) {
	typeParamNames := map[string]bool{"pEntity": true}

	// Primitive type should not be a type parameter ref
	dt := ast.DataType{Kind: ast.TypeString}
	if isTypeParamRef(dt, typeParamNames) {
		t.Error("Expected primitive type to not be a type parameter reference")
	}
}

func TestGetTypeParamRefName_EnumRef(t *testing.T) {
	dt := ast.DataType{
		Kind:    ast.TypeEnumeration,
		EnumRef: &ast.QualifiedName{Name: "pEntity"},
	}
	got := getTypeParamRefName(dt)
	if got != "pEntity" {
		t.Errorf("got %q, want %q", got, "pEntity")
	}
}

func TestGetTypeParamRefName_EntityRef(t *testing.T) {
	dt := ast.DataType{
		Kind:      ast.TypeEntity,
		EntityRef: &ast.QualifiedName{Name: "pEntity"},
	}
	got := getTypeParamRefName(dt)
	if got != "pEntity" {
		t.Errorf("got %q, want %q", got, "pEntity")
	}
}

func TestGetTypeParamRefName_PrimitiveType(t *testing.T) {
	dt := ast.DataType{Kind: ast.TypeBoolean}
	got := getTypeParamRefName(dt)
	if got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}

// =============================================================================
// Java Action Type TypeString() Methods
// =============================================================================

func TestEntityTypeParameterType_TypeString(t *testing.T) {
	tests := []struct {
		name string
		typ  javaactions.EntityTypeParameterType
		want string
	}{
		{"with name", javaactions.EntityTypeParameterType{TypeParameterName: "pEntity"}, "pEntity"},
		{"without name", javaactions.EntityTypeParameterType{}, "Object"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.typ.TypeString()
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestTypeParameter_TypeString(t *testing.T) {
	tests := []struct {
		name string
		typ  javaactions.TypeParameter
		want string
	}{
		{"with name", javaactions.TypeParameter{TypeParameter: "pEntity"}, "pEntity"},
		{"with name and ID", javaactions.TypeParameter{TypeParameter: "pEntity", TypeParameterID: "some-id"}, "pEntity"},
		{"without value", javaactions.TypeParameter{}, "T"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.typ.TypeString()
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

// TypeParameter used as ParameterizedEntityType should display like EntityTypeParameterType
func TestFormatJavaActionType_TypeParameterAsParameterizedEntity(t *testing.T) {
	typ := &javaactions.TypeParameter{
		TypeParameter:   "pEntity",
		TypeParameterID: "some-uuid",
	}
	got := formatJavaActionType(typ)
	if got != "pEntity" {
		t.Errorf("got %q, want %q", got, "pEntity")
	}
}

// =============================================================================
// First-occurrence rule: microflow builder treats EntityTypeParameterType as
// entity type selector and TypeParameter (ParameterizedEntityType) as regular
// =============================================================================

func TestFormatAction_JavaActionCall_EntityTypeAndParameterizedParams(t *testing.T) {
	// Simulates a Java action call with:
	// - EntityType param → EntityTypeCodeActionParameterValue (entity name)
	// - Source param → BasicCodeActionParameterValue (variable)
	// - Target param → BasicCodeActionParameterValue (variable)
	e := newTestExecutor()
	action := &microflows.JavaActionCallAction{
		JavaAction:         "MyModule.CopyAttributes",
		ResultVariableName: "Result",
		UseReturnVariable:  true,
		ParameterMappings: []*microflows.JavaActionParameterMapping{
			{
				Parameter: "MyModule.CopyAttributes.EntityType",
				Value: &microflows.EntityTypeCodeActionParameterValue{
					Entity: "MyModule.ProcessResult",
				},
			},
			{
				Parameter: "MyModule.CopyAttributes.Source",
				Value: &microflows.BasicCodeActionParameterValue{
					Argument: "$Source",
				},
			},
			{
				Parameter: "MyModule.CopyAttributes.Target",
				Value: &microflows.BasicCodeActionParameterValue{
					Argument: "$Target",
				},
			},
			{
				Parameter: "MyModule.CopyAttributes.AttributeNames",
				Value: &microflows.BasicCodeActionParameterValue{
					Argument: "'ProcessedCount,ResultMessage'",
				},
			},
		},
	}
	got := e.formatAction(action, nil, nil)
	want := "$Result = call java action MyModule.CopyAttributes(EntityType = 'MyModule.ProcessResult', Source = $Source, Target = $Target, AttributeNames = 'ProcessedCount,ResultMessage');"
	if got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

func TestTypeParameterDef_Names(t *testing.T) {
	ja := &javaactions.JavaAction{
		TypeParameters: []*javaactions.TypeParameterDef{
			{Name: "pEntity"},
			{Name: "pOther"},
		},
	}

	names := ja.TypeParameterNames()
	if len(names) != 2 {
		t.Fatalf("Expected 2 names, got %d", len(names))
	}
	if names[0] != "pEntity" || names[1] != "pOther" {
		t.Errorf("got %v", names)
	}
}

func TestMicroflowActionInfo_Fields(t *testing.T) {
	info := &javaactions.MicroflowActionInfo{
		Caption:  "My Action",
		Category: "My Category",
		IconData: []byte{0x1, 0x2, 0x3},
	}
	if info.Caption != "My Action" {
		t.Errorf("got %q", info.Caption)
	}
	if info.Category != "My Category" {
		t.Errorf("got %q", info.Category)
	}
	if len(info.IconData) != 3 {
		t.Errorf("IconData len = %d, want 3", len(info.IconData))
	}
}

func TestEntityTypeCodeActionParameterValue_Fields(t *testing.T) {
	v := &microflows.EntityTypeCodeActionParameterValue{
		Entity: "MyModule.Customer",
	}
	if v.Entity != "MyModule.Customer" {
		t.Errorf("got %q", v.Entity)
	}
}
