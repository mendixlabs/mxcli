// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/javaactions"
)

// Issue #680: a parameter/return typed with the explicit `ENUM`/`Enumeration(...)`
// syntax must serialize as an enumeration, not an entity reference.

func enumDataType(mod, name string) ast.DataType {
	qn := ast.QualifiedName{Module: mod, Name: name}
	return ast.DataType{Kind: ast.TypeEnumeration, EnumRef: &qn, ExplicitEnum: true}
}

func bareNameDataType(mod, name string) ast.DataType {
	qn := ast.QualifiedName{Module: mod, Name: name}
	// A bare Module.Name parses as TypeEnumeration WITHOUT ExplicitEnum.
	return ast.DataType{Kind: ast.TypeEnumeration, EnumRef: &qn}
}

func TestParamType_ExplicitEnum_IsEnumeration(t *testing.T) {
	pt := astDataTypeToJavaActionParamType(enumDataType("Barcode", "BarcodeFormat"))
	et, ok := pt.(*javaactions.EnumerationType)
	if !ok {
		t.Fatalf("expected *EnumerationType, got %T", pt)
	}
	if et.Enumeration != "Barcode.BarcodeFormat" {
		t.Errorf("Enumeration = %q", et.Enumeration)
	}
	if et.TypeName != "CodeActions$EnumerationType" {
		t.Errorf("TypeName = %q", et.TypeName)
	}
}

func TestReturnType_ExplicitEnum_IsEnumeration(t *testing.T) {
	rt := astDataTypeToJavaActionReturnType(enumDataType("M", "Status"))
	et, ok := rt.(*javaactions.EnumerationType)
	if !ok {
		t.Fatalf("expected *EnumerationType, got %T", rt)
	}
	if et.Enumeration != "M.Status" {
		t.Errorf("Enumeration = %q", et.Enumeration)
	}
}

// A bare Module.Name (not explicit ENUM/Enumeration()) is indistinguishable from
// an entity to the parser, so it stays an EntityType — unchanged behavior.
func TestParamType_BareName_StaysEntity(t *testing.T) {
	pt := astDataTypeToJavaActionParamType(bareNameDataType("Sales", "Customer"))
	if _, ok := pt.(*javaactions.EntityType); !ok {
		t.Fatalf("expected *EntityType for bare name, got %T", pt)
	}
}

// Sanity: the EnumerationType produced is a valid code-action parameter type.
func TestEnumerationType_ImplementsParamType(t *testing.T) {
	var _ javaactions.CodeActionParameterType = &javaactions.EnumerationType{
		BaseElement: model.BaseElement{ID: "x"},
		Enumeration: "M.E",
	}
}
