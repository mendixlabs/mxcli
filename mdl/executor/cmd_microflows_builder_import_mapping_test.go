// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
	"github.com/JordtenBulte-OLC/mxcli/mdl/backend/mock"
	mdltypes "github.com/JordtenBulte-OLC/mxcli/mdl/types"
	"github.com/JordtenBulte-OLC/mxcli/model"
)

func TestAddImportFromMappingRegistersSingleResultType(t *testing.T) {
	fb := importMappingFlowBuilder(t, "Object")

	fb.addImportFromMappingAction(&ast.ImportFromMappingStmt{
		OutputVariable: "ImportedOrder",
		SourceVariable: "Payload",
		Mapping:        ast.QualifiedName{Module: "Integration", Name: "ImportOrder"},
	})

	if got := fb.varTypes["ImportedOrder"]; got != "Sales.Order" {
		t.Fatalf("ImportedOrder type = %q, want Sales.Order", got)
	}
}

func TestAddImportFromMappingRegistersListResultType(t *testing.T) {
	fb := importMappingFlowBuilder(t, "Array")

	fb.addImportFromMappingAction(&ast.ImportFromMappingStmt{
		OutputVariable: "ImportedOrders",
		SourceVariable: "Payload",
		Mapping:        ast.QualifiedName{Module: "Integration", Name: "ImportOrderList"},
	})

	if got := fb.varTypes["ImportedOrders"]; got != "List of Sales.Order" {
		t.Fatalf("ImportedOrders type = %q, want list of Sales.Order", got)
	}
}

// XML-schema and message-definition mappings have no JsonStructure;
// addImportFromMappingAction must then read the single-vs-list shape
// from the import mapping's own root element. MaxOccurs > 1 or
// unbounded (-1) signals a list — Studio Pro models a repeating Object
// root that way for these mappings.
func TestAddImportFromMappingFallsBackToImportMappingRootForListResult(t *testing.T) {
	fb := &flowBuilder{
		varTypes: map[string]string{},
		backend: &mock.MockBackend{
			GetImportMappingByQualifiedNameFunc: func(moduleName, name string) (*model.ImportMapping, error) {
				return &model.ImportMapping{
					JsonStructure: "",
					Elements: []*model.ImportMappingElement{
						{Kind: "Object", Entity: "Sales.Order", MaxOccurs: -1},
					},
				}, nil
			},
		},
	}

	fb.addImportFromMappingAction(&ast.ImportFromMappingStmt{
		OutputVariable: "ImportedOrders",
		SourceVariable: "Payload",
		Mapping:        ast.QualifiedName{Module: "Integration", Name: "ImportOrders"},
	})

	if got := fb.varTypes["ImportedOrders"]; got != "List of Sales.Order" {
		t.Fatalf("ImportedOrders type = %q, want list of Sales.Order (Object root with MaxOccurs=-1 must yield list)", got)
	}
}

// A non-repeating Object root (MaxOccurs ≤ 1) keeps the singleton type
// when the JSON structure is absent.
func TestAddImportFromMappingFallsBackToImportMappingRootForSingleObject(t *testing.T) {
	fb := &flowBuilder{
		varTypes: map[string]string{},
		backend: &mock.MockBackend{
			GetImportMappingByQualifiedNameFunc: func(moduleName, name string) (*model.ImportMapping, error) {
				return &model.ImportMapping{
					JsonStructure: "",
					Elements: []*model.ImportMappingElement{
						{Kind: "Object", Entity: "Sales.Order", MaxOccurs: 1, MinOccurs: 1},
					},
				}, nil
			},
		},
	}

	fb.addImportFromMappingAction(&ast.ImportFromMappingStmt{
		OutputVariable: "ImportedOrder",
		SourceVariable: "Payload",
		Mapping:        ast.QualifiedName{Module: "Integration", Name: "ImportOrder"},
	})

	if got := fb.varTypes["ImportedOrder"]; got != "Sales.Order" {
		t.Fatalf("ImportedOrder type = %q, want Sales.Order (Object root with MaxOccurs=1 must stay singleton)", got)
	}
}

func importMappingFlowBuilder(t *testing.T, rootElementType string) *flowBuilder {
	t.Helper()

	return &flowBuilder{
		varTypes: map[string]string{},
		backend: &mock.MockBackend{
			GetImportMappingByQualifiedNameFunc: func(moduleName, name string) (*model.ImportMapping, error) {
				if moduleName != "Integration" {
					return nil, fmt.Errorf("unexpected module %q", moduleName)
				}
				return &model.ImportMapping{
					JsonStructure: "Integration.OrderPayload",
					Elements: []*model.ImportMappingElement{
						{Entity: "Sales.Order"},
					},
				}, nil
			},
			GetJsonStructureByQualifiedNameFunc: func(moduleName, name string) (*mdltypes.JsonStructure, error) {
				if moduleName != "Integration" || name != "OrderPayload" {
					return nil, fmt.Errorf("unexpected json structure %s.%s", moduleName, name)
				}
				return &mdltypes.JsonStructure{
					Elements: []*mdltypes.JsonElement{
						{ElementType: rootElementType},
					},
				}, nil
			},
		},
	}
}
