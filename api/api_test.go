// SPDX-License-Identifier: Apache-2.0

package api

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/model"
)

func TestParseQualifiedName(t *testing.T) {
	tests := []struct {
		input    string
		expected QualifiedName
	}{
		{"MyModule.Customer", QualifiedName{ModuleName: "MyModule", ElementName: "Customer"}},
		{"Customer", QualifiedName{ModuleName: "", ElementName: "Customer"}},
		{"A.B.C", QualifiedName{ModuleName: "A", ElementName: "B.C"}},
	}

	for _, test := range tests {
		result := ParseQualifiedName(test.input)
		if result.ModuleName != test.expected.ModuleName || result.ElementName != test.expected.ElementName {
			t.Errorf("ParseQualifiedName(%q) = %+v, want %+v", test.input, result, test.expected)
		}
	}
}

func TestParseAttributePath(t *testing.T) {
	tests := []struct {
		input    string
		expected AttributePath
	}{
		{"MyModule.Customer.Name", AttributePath{ModuleName: "MyModule", EntityName: "Customer", AttributeName: "Name"}},
		{"Customer.Name", AttributePath{ModuleName: "", EntityName: "Customer", AttributeName: "Name"}},
		{"Name", AttributePath{ModuleName: "", EntityName: "", AttributeName: "Name"}},
	}

	for _, test := range tests {
		result := ParseAttributePath(test.input)
		if result.ModuleName != test.expected.ModuleName ||
			result.EntityName != test.expected.EntityName ||
			result.AttributeName != test.expected.AttributeName {
			t.Errorf("ParseAttributePath(%q) = %+v, want %+v", test.input, result, test.expected)
		}
	}
}

func TestQualifiedNameString(t *testing.T) {
	tests := []struct {
		qn       QualifiedName
		expected string
	}{
		{QualifiedName{ModuleName: "MyModule", ElementName: "Customer"}, "MyModule.Customer"},
		{QualifiedName{ModuleName: "", ElementName: "Customer"}, "Customer"},
	}

	for _, test := range tests {
		result := test.qn.String()
		if result != test.expected {
			t.Errorf("QualifiedName{%q, %q}.String() = %q, want %q",
				test.qn.ModuleName, test.qn.ElementName, result, test.expected)
		}
	}
}

func TestGenerateID(t *testing.T) {
	id1 := generateID()
	id2 := generateID()

	if id1 == "" {
		t.Error("generateID() returned empty string")
	}

	if id1 == id2 {
		t.Error("generateID() returned same ID twice")
	}
}

func TestBuildQualifiedName(t *testing.T) {
	result := BuildQualifiedName("MyModule", "Customer")
	if result != "MyModule.Customer" {
		t.Errorf("BuildQualifiedName() = %q, want %q", result, "MyModule.Customer")
	}

	result = BuildQualifiedName("", "Customer")
	if result != "Customer" {
		t.Errorf("BuildQualifiedName() = %q, want %q", result, "Customer")
	}
}

// TestEntityBuilderFluent tests that the entity builder can be chained
func TestEntityBuilderFluent(t *testing.T) {
	// Create a mock DomainModelsAPI
	dm := &DomainModelsAPI{}

	// Test that the builder chain works
	builder := dm.CreateEntity("TestEntity")
	if builder == nil {
		t.Fatal("CreateEntity() returned nil")
	}

	// Test chaining
	builder = builder.
		Persistent().
		WithStringAttribute("Name", 100).
		WithIntegerAttribute("Age").
		WithBooleanAttribute("IsActive")

	if builder.entity.Name != "TestEntity" {
		t.Errorf("entity.Name = %q, want %q", builder.entity.Name, "TestEntity")
	}

	if len(builder.attributes) != 3 {
		t.Errorf("len(attributes) = %d, want %d", len(builder.attributes), 3)
	}
}

// TestEnumerationBuilderFluent tests that the enumeration builder can be chained
func TestEnumerationBuilderFluent(t *testing.T) {
	e := &EnumerationsAPI{}

	builder := e.CreateEnumeration("OrderStatus")
	if builder == nil {
		t.Fatal("CreateEnumeration() returned nil")
	}

	builder = builder.
		WithValue("New", "New").
		WithValue("Processing", "Processing").
		WithValue("Completed", "Completed")

	if builder.enum.Name != "OrderStatus" {
		t.Errorf("enum.Name = %q, want %q", builder.enum.Name, "OrderStatus")
	}

	if len(builder.enum.Values) != 3 {
		t.Errorf("len(values) = %d, want %d", len(builder.enum.Values), 3)
	}
}

// TestPageBuilderFluent tests that the page builder can be chained
func TestPageBuilderFluent(t *testing.T) {
	p := &PagesAPI{}

	builder := p.CreatePage("Customer_Edit")
	if builder == nil {
		t.Fatal("CreatePage() returned nil")
	}

	builder = builder.
		WithTitle("Edit Customer").
		WithURL("customer-edit/{Id}").
		WithParameter("Customer", "MyModule.Customer")

	if builder.page.Name != "Customer_Edit" {
		t.Errorf("page.Name = %q, want %q", builder.page.Name, "Customer_Edit")
	}

	if builder.page.URL != "customer-edit/{Id}" {
		t.Errorf("page.URL = %q, want %q", builder.page.URL, "customer-edit/{Id}")
	}

	if len(builder.page.Parameters) != 1 {
		t.Errorf("len(parameters) = %d, want %d", len(builder.page.Parameters), 1)
	}
}

// TestMicroflowBuilderFluent tests that the microflow builder can be chained
func TestMicroflowBuilderFluent(t *testing.T) {
	m := &MicroflowsAPI{}

	builder := m.CreateMicroflow("ACT_Customer_Save")
	if builder == nil {
		t.Fatal("CreateMicroflow() returned nil")
	}

	builder = builder.
		WithParameter("Customer", "MyModule.Customer").
		WithStringParameter("Message").
		ReturnsBoolean()

	if builder.microflow.Name != "ACT_Customer_Save" {
		t.Errorf("microflow.Name = %q, want %q", builder.microflow.Name, "ACT_Customer_Save")
	}

	if len(builder.microflow.Parameters) != 2 {
		t.Errorf("len(parameters) = %d, want %d", len(builder.microflow.Parameters), 2)
	}
}

// TestDataViewBuilder tests the DataView builder
func TestDataViewBuilder(t *testing.T) {
	p := &PagesAPI{}

	builder := p.CreateDataView()
	if builder == nil {
		t.Fatal("CreateDataView() returned nil")
	}

	builder = builder.
		WithName("customerDataView").
		WithEntity("MyModule.Customer").
		FromParameter("Customer")

	dataView := builder.Build()
	if dataView.Name != "customerDataView" {
		t.Errorf("dataView.Name = %q, want %q", dataView.Name, "customerDataView")
	}

	if builder.dataSource == nil {
		t.Fatal("dataSource is nil")
	}

	if builder.dataSource.EntityName != "MyModule.Customer" {
		t.Errorf("dataSource.EntityName = %q, want %q", builder.dataSource.EntityName, "MyModule.Customer")
	}

	if builder.dataSource.ParameterName != "Customer" {
		t.Errorf("dataSource.ParameterName = %q, want %q", builder.dataSource.ParameterName, "Customer")
	}
}

// TestWidgetBuilders tests various widget builders
func TestWidgetBuilders(t *testing.T) {
	p := &PagesAPI{}

	// TextBox
	textBox := p.CreateTextBox("nameTextBox").
		WithLabel("Name").
		WithAttribute("MyModule.Customer.Name").
		Build()

	if textBox.Name != "nameTextBox" {
		t.Errorf("textBox.Name = %q, want %q", textBox.Name, "nameTextBox")
	}
	if textBox.Label != "Name" {
		t.Errorf("textBox.Label = %q, want %q", textBox.Label, "Name")
	}

	// CheckBox
	checkBox := p.CreateCheckBox("activeCheckBox").
		WithLabel("Is Active").
		WithAttribute("MyModule.Customer.IsActive").
		Build()

	if checkBox.Name != "activeCheckBox" {
		t.Errorf("checkBox.Name = %q, want %q", checkBox.Name, "activeCheckBox")
	}
	if checkBox.Label != "Is Active" {
		t.Errorf("checkBox.Label = %q, want %q", checkBox.Label, "Is Active")
	}
}

// Ensure model import is used
var _ model.ID
