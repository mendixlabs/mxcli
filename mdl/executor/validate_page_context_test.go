// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
)

func TestValidatePageContextTree_ParameterDSValid(t *testing.T) {
	params := []ast.PageParameter{
		{Name: "Customer", EntityType: ast.QualifiedName{Module: "Mod", Name: "Customer"}},
	}
	widgets := []*ast.WidgetV3{
		{
			Type: "dataview", Name: "dvCustomer",
			Properties: map[string]any{
				"DataSource": &ast.DataSourceV3{Type: "parameter", Reference: "$Customer"},
			},
			Children: []*ast.WidgetV3{
				{Type: "textbox", Name: "txtName", Properties: map[string]any{"Attribute": "Name"}},
			},
		},
	}

	errors := validatePageContextTree(params, widgets)
	if len(errors) > 0 {
		t.Errorf("Expected no errors, got: %v", errors)
	}
}

func TestValidatePageContextTree_ParameterDSInvalid(t *testing.T) {
	params := []ast.PageParameter{
		{Name: "Order", EntityType: ast.QualifiedName{Module: "Mod", Name: "Order"}},
	}
	widgets := []*ast.WidgetV3{
		{
			Type: "dataview", Name: "dvCustomer",
			Properties: map[string]any{
				"DataSource": &ast.DataSourceV3{Type: "parameter", Reference: "$Customer"},
			},
		},
	}

	errors := validatePageContextTree(params, widgets)
	if len(errors) != 1 {
		t.Fatalf("Expected 1 error, got %d: %v", len(errors), errors)
	}
	if !strings.Contains(errors[0], "$Customer") || !strings.Contains(errors[0], "no such parameter") {
		t.Errorf("Unexpected error message: %s", errors[0])
	}
}

func TestValidatePageContextTree_SelectionDSValid(t *testing.T) {
	widgets := []*ast.WidgetV3{
		{
			Type: "datagrid", Name: "dgOrders",
			Properties: map[string]any{
				"DataSource": &ast.DataSourceV3{Type: "database", Reference: "Mod.Order"},
			},
		},
		{
			Type: "dataview", Name: "dvDetail",
			Properties: map[string]any{
				"DataSource": &ast.DataSourceV3{Type: "selection", Reference: "dgOrders"},
			},
			Children: []*ast.WidgetV3{
				{Type: "textbox", Name: "txtNum", Properties: map[string]any{"Attribute": "OrderNumber"}},
			},
		},
	}

	errors := validatePageContextTree(nil, widgets)
	if len(errors) > 0 {
		t.Errorf("Expected no errors, got: %v", errors)
	}
}

func TestValidatePageContextTree_SelectionDSInvalid(t *testing.T) {
	widgets := []*ast.WidgetV3{
		{
			Type: "dataview", Name: "dvDetail",
			Properties: map[string]any{
				"DataSource": &ast.DataSourceV3{Type: "selection", Reference: "nonExistentGrid"},
			},
		},
	}

	errors := validatePageContextTree(nil, widgets)
	if len(errors) != 1 {
		t.Fatalf("Expected 1 error, got %d: %v", len(errors), errors)
	}
	if !strings.Contains(errors[0], "nonExistentGrid") || !strings.Contains(errors[0], "no widget with that name") {
		t.Errorf("Unexpected error message: %s", errors[0])
	}
}

func TestValidatePageContextTree_AttributeWithoutContext(t *testing.T) {
	widgets := []*ast.WidgetV3{
		{Type: "textbox", Name: "txtName", Properties: map[string]any{"Attribute": "Name"}},
	}

	errors := validatePageContextTree(nil, widgets)
	if len(errors) != 1 {
		t.Fatalf("Expected 1 error, got %d: %v", len(errors), errors)
	}
	if !strings.Contains(errors[0], "no enclosing data container") {
		t.Errorf("Unexpected error message: %s", errors[0])
	}
}

func TestValidatePageContextTree_AttributeInsideDataView(t *testing.T) {
	widgets := []*ast.WidgetV3{
		{
			Type: "dataview", Name: "dv1",
			Properties: map[string]any{
				"DataSource": &ast.DataSourceV3{Type: "database", Reference: "Mod.Customer"},
			},
			Children: []*ast.WidgetV3{
				{Type: "textbox", Name: "txtName", Properties: map[string]any{"Attribute": "Name"}},
				{Type: "dropdown", Name: "ddCountry", Properties: map[string]any{"Attribute": "Country"}},
			},
		},
	}

	errors := validatePageContextTree(nil, widgets)
	if len(errors) > 0 {
		t.Errorf("Expected no errors, got: %v", errors)
	}
}

func TestValidatePageContextTree_NoErrors(t *testing.T) {
	errors := validatePageContextTree(nil, nil)
	if len(errors) > 0 {
		t.Errorf("Expected no errors for nil widgets, got: %v", errors)
	}
}
