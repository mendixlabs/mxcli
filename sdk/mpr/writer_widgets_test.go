// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/pages"
	"go.mongodb.org/mongo-driver/bson"
)

func TestSerializeDataView(t *testing.T) {
	// Create a DataView with a DataViewSource (parameter reference)
	dataView := &pages.DataView{
		BaseWidget: pages.BaseWidget{
			BaseElement: model.BaseElement{
				ID:       "test-dataview-id",
				TypeName: "Forms$DataView",
			},
			Name: "customerForm",
		},
		DataSource: &pages.DataViewSource{
			BaseElement: model.BaseElement{
				ID:       "test-datasource-id",
				TypeName: "Forms$DataViewSource",
			},
			EntityID:      "test-entity-id",
			EntityName:    "TestModule.Customer",
			ParameterName: "Customer",
		},
		ShowFooter: true,
		Editable:   true,
	}

	result := serializeDataView(dataView)

	// Check that result is a BSON document
	if result == nil {
		t.Fatal("serializeDataView returned nil")
	}

	// Check $Type
	var foundType string
	for _, elem := range result {
		if elem.Key == "$Type" {
			foundType = elem.Value.(string)
		}
	}
	if foundType != "Forms$DataView" {
		t.Errorf("Expected $Type to be 'Forms$DataView', got '%s'", foundType)
	}

	// Check DataSource is present
	var foundDataSource any
	for _, elem := range result {
		if elem.Key == "DataSource" {
			foundDataSource = elem.Value
		}
	}
	if foundDataSource == nil {
		t.Error("DataSource is nil, expected it to be set")
	}

	// Check DataSource type
	if ds, ok := foundDataSource.(bson.D); ok {
		var dsType string
		for _, elem := range ds {
			if elem.Key == "$Type" {
				dsType = elem.Value.(string)
			}
		}
		if dsType != "Forms$DataViewSource" {
			t.Errorf("Expected DataSource.$Type to be 'Forms$DataViewSource', got '%s'", dsType)
		}

		// Check EntityRef is present
		var entityRef any
		for _, elem := range ds {
			if elem.Key == "EntityRef" {
				entityRef = elem.Value
			}
		}
		if entityRef == nil {
			t.Error("EntityRef is nil, expected it to be set")
		}

		// Check SourceVariable is present
		var sourceVar any
		for _, elem := range ds {
			if elem.Key == "SourceVariable" {
				sourceVar = elem.Value
			}
		}
		if sourceVar == nil {
			t.Error("SourceVariable is nil, expected it to be set")
		}

		// Check SourceVariable contains PageParameter
		if sv, ok := sourceVar.(bson.D); ok {
			var pageParam string
			var svType string
			for _, elem := range sv {
				if elem.Key == "PageParameter" {
					pageParam = elem.Value.(string)
				}
				if elem.Key == "$Type" {
					svType = elem.Value.(string)
				}
			}
			if svType != "Forms$PageVariable" {
				t.Errorf("Expected SourceVariable.$Type to be 'Forms$PageVariable', got '%s'", svType)
			}
			if pageParam != "Customer" {
				t.Errorf("Expected PageParameter to be 'Customer', got '%s'", pageParam)
			}
		} else {
			t.Error("SourceVariable is not a bson.D")
		}

		// Check EntityRef structure
		if er, ok := entityRef.(bson.D); ok {
			var erType string
			var entity string
			for _, elem := range er {
				if elem.Key == "$Type" {
					erType = elem.Value.(string)
				}
				if elem.Key == "Entity" {
					entity = elem.Value.(string)
				}
			}
			if erType != "DomainModels$DirectEntityRef" {
				t.Errorf("Expected EntityRef.$Type to be 'DomainModels$DirectEntityRef', got '%s'", erType)
			}
			if entity != "TestModule.Customer" {
				t.Errorf("Expected Entity to be 'TestModule.Customer', got '%s'", entity)
			}
		} else {
			t.Error("EntityRef is not a bson.D")
		}
	} else {
		t.Error("DataSource is not a bson.D")
	}
}

func TestSerializeDataViewDataSource(t *testing.T) {
	ds := &pages.DataViewSource{
		BaseElement: model.BaseElement{
			ID:       "test-ds-id",
			TypeName: "Forms$DataViewSource",
		},
		EntityID:      "entity-123",
		EntityName:    "MyModule.MyEntity",
		ParameterName: "MyParam",
	}

	result := serializeDataViewDataSource(ds)
	if result == nil {
		t.Fatal("serializeDataViewDataSource returned nil")
	}

	bsonResult, ok := result.(bson.D)
	if !ok {
		t.Fatalf("Expected bson.D, got %T", result)
	}

	// Check structure
	var foundType, foundEntityRef, foundSourceVar bool
	for _, elem := range bsonResult {
		switch elem.Key {
		case "$Type":
			if elem.Value.(string) != "Forms$DataViewSource" {
				t.Errorf("Expected $Type 'Forms$DataViewSource', got '%v'", elem.Value)
			}
			foundType = true
		case "EntityRef":
			if elem.Value != nil {
				foundEntityRef = true
			}
		case "SourceVariable":
			if elem.Value != nil {
				foundSourceVar = true
			}
		}
	}

	if !foundType {
		t.Error("$Type not found in result")
	}
	if !foundEntityRef {
		t.Error("EntityRef not found or is nil")
	}
	if !foundSourceVar {
		t.Error("SourceVariable not found or is nil")
	}
}

func TestSerializeTextBox(t *testing.T) {
	tb := &pages.TextBox{
		BaseWidget: pages.BaseWidget{
			BaseElement: model.BaseElement{
				ID:       "test-textbox-id",
				TypeName: "Forms$TextBox",
			},
			Name: "txtEmail",
		},
		AttributePath: "MyModule.Customer.Email",
	}

	result := serializeTextBox(tb)

	// Check $Type
	var foundType, foundAttrRef, foundName bool
	for _, elem := range result {
		switch elem.Key {
		case "$Type":
			if elem.Value.(string) != "Forms$TextBox" {
				t.Errorf("Expected $Type 'Forms$TextBox', got '%v'", elem.Value)
			}
			foundType = true
		case "AttributeRef":
			if elem.Value != nil {
				foundAttrRef = true
				// Check AttributeRef structure
				if ar, ok := elem.Value.(bson.D); ok {
					var attrType, attrValue string
					for _, arElem := range ar {
						if arElem.Key == "$Type" {
							attrType = arElem.Value.(string)
						}
						if arElem.Key == "Attribute" {
							attrValue = arElem.Value.(string)
						}
					}
					if attrType != "DomainModels$AttributeRef" {
						t.Errorf("Expected AttributeRef.$Type 'DomainModels$AttributeRef', got '%s'", attrType)
					}
					if attrValue != "MyModule.Customer.Email" {
						t.Errorf("Expected Attribute 'MyModule.Customer.Email', got '%s'", attrValue)
					}
				}
			}
		case "Name":
			if elem.Value.(string) == "txtEmail" {
				foundName = true
			}
		}
	}

	if !foundType {
		t.Error("$Type not found")
	}
	if !foundAttrRef {
		t.Error("AttributeRef not found or is nil")
	}
	if !foundName {
		t.Error("Name not found or incorrect")
	}
}

func TestSerializeDataViewLabelWidth(t *testing.T) {
	five := 5
	zero := 0
	cases := []struct {
		name string
		dv   *pages.DataView
		want int64
	}{
		{"default is Horizontal=3", &pages.DataView{}, 3},
		{"FormOrientation Vertical -> 0", &pages.DataView{FormOrientation: pages.FormOrientationVertical}, 0},
		{"FormOrientation Horizontal -> 3", &pages.DataView{FormOrientation: pages.FormOrientationHorizontal}, 3},
		{"explicit LabelWidth=5", &pages.DataView{LabelWidth: &five}, 5},
		{"explicit LabelWidth=0 wins over Horizontal", &pages.DataView{LabelWidth: &zero, FormOrientation: pages.FormOrientationHorizontal}, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := serializeDataView(tc.dv)
			var lw int64 = -1
			for _, elem := range got {
				if elem.Key == "LabelWidth" {
					lw = elem.Value.(int64)
				}
			}
			if lw != tc.want {
				t.Errorf("LabelWidth = %d, want %d", lw, tc.want)
			}
		})
	}
}

func TestSerializeRadioButtons(t *testing.T) {
	rb := &pages.RadioButtons{
		BaseWidget: pages.BaseWidget{
			BaseElement: model.BaseElement{
				ID:       "test-radio-id",
				TypeName: "Forms$RadioButtonGroup",
			},
			Name: "rbIsActive",
		},
		AttributePath: "MyModule.Customer.IsActive",
	}

	result := serializeRadioButtons(rb)

	// Check $Type
	var foundType string
	for _, elem := range result {
		if elem.Key == "$Type" {
			foundType = elem.Value.(string)
		}
	}
	if foundType != "Forms$RadioButtonGroup" {
		t.Errorf("Expected $Type 'Forms$RadioButtonGroup', got '%s'", foundType)
	}
}
