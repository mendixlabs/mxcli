// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/model"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func TestSerializeConsumedODataService(t *testing.T) {
	w := &Writer{}
	svc := &model.ConsumedODataService{
		BaseElement: model.BaseElement{
			ID:       "test-consumed-id",
			TypeName: "Rest$ConsumedODataService",
		},
		ContainerID:       "test-module-id",
		Name:              "SalesforceAPI",
		Documentation:     "Connects to Salesforce",
		Version:           "1.0",
		ODataVersion:      "OData4",
		MetadataUrl:       "https://api.salesforce.com/odata/v4/$metadata",
		TimeoutExpression: "300",
		ProxyType:         "DefaultProxy",
		Description:       "Salesforce OData API",
		Validated:         true,
	}

	data, err := w.serializeConsumedODataService(svc)
	if err != nil {
		t.Fatalf("serialize failed: %v", err)
	}

	// Deserialize and verify fields
	var raw map[string]any
	if err := bson.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	assertField(t, raw, "$Type", "Rest$ConsumedODataService")
	assertField(t, raw, "Name", "SalesforceAPI")
	assertField(t, raw, "Documentation", "Connects to Salesforce")
	assertField(t, raw, "Version", "1.0")
	assertField(t, raw, "ODataVersion", "OData4")
	assertField(t, raw, "MetadataUrl", "https://api.salesforce.com/odata/v4/$metadata")
	assertField(t, raw, "TimeoutExpression", "300")
	assertField(t, raw, "ProxyType", "DefaultProxy")
	assertField(t, raw, "Description", "Salesforce OData API")

	if v, ok := raw["Validated"].(bool); !ok || !v {
		t.Errorf("Validated: expected true, got %v", raw["Validated"])
	}
}

func TestSerializeConsumedODataServiceWithHttpConfig(t *testing.T) {
	w := &Writer{}
	svc := &model.ConsumedODataService{
		BaseElement: model.BaseElement{
			ID:       "test-consumed-full-id",
			TypeName: "Rest$ConsumedODataService",
		},
		ContainerID:            "test-module-id",
		Name:                   "FullAPI",
		ODataVersion:           "OData4",
		MetadataUrl:            "https://api.example.com/odata/$metadata",
		TimeoutExpression:      "300",
		ConfigurationMicroflow: "MyModule.ConfigureMF",
		ErrorHandlingMicroflow: "MyModule.HandleErrorMF",
		ProxyHost:              "MyModule.ProxyHostConst",
		HttpConfiguration: &model.HttpConfiguration{
			BaseElement: model.BaseElement{
				ID:       "test-http-cfg-id",
				TypeName: "Microflows$HttpConfiguration",
			},
			UseAuthentication: true,
			Username:          "'admin'",
			Password:          "'secret'",
			HttpMethod:        "Get",
			OverrideLocation:  true,
			CustomLocation:    "'https://api.example.com/odata'",
			ClientCertificate: "my-cert",
			HeaderEntries: []*model.HttpHeaderEntry{
				{
					BaseElement: model.BaseElement{ID: "header-1"},
					Key:         "X-Api-Key",
					Value:       "'abc123'",
				},
			},
		},
	}

	data, err := w.serializeConsumedODataService(svc)
	if err != nil {
		t.Fatalf("serialize failed: %v", err)
	}

	var raw map[string]any
	if err := bson.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	// Microflow reference. Studio Pro stores both the "Configuration
	// microflow" and "Headers microflow" dropdown options in the single
	// `ConfigurationMicroflow` BSON field — it picks the dropdown label
	// from the microflow's return type, not from which field carries the
	// reference. Older mxcli fixes tried `ConfigurationEntityMicroflow` /
	// `HeaderListMicroflow` / a separate `HeadersMicroflow`; Studio Pro
	// silently ignores all three, leaving the dropdown stuck on
	// "Constants only".
	assertField(t, raw, "ConfigurationMicroflow", "MyModule.ConfigureMF")
	assertField(t, raw, "ErrorHandlingMicroflow", "MyModule.HandleErrorMF")
	assertField(t, raw, "ProxyHost", "MyModule.ProxyHostConst")
	if _, exists := raw["HeadersMicroflow"]; exists {
		t.Errorf("HeadersMicroflow leaked into BSON — Studio Pro stores both microflow dropdown options under ConfigurationMicroflow")
	}

	// HTTP Configuration
	httpCfg, ok := raw["HttpConfiguration"].(map[string]any)
	if !ok {
		t.Fatalf("HttpConfiguration: expected map, got %T", raw["HttpConfiguration"])
	}
	assertField(t, httpCfg, "$Type", "Microflows$HttpConfiguration")

	if v, ok := httpCfg["UseHttpAuthentication"].(bool); !ok || !v {
		t.Errorf("UseHttpAuthentication: expected true, got %v", httpCfg["UseHttpAuthentication"])
	}
	assertField(t, httpCfg, "HttpAuthenticationUserName", "'admin'")
	assertField(t, httpCfg, "HttpAuthenticationPassword", "'secret'")
	assertField(t, httpCfg, "HttpMethod", "Get")
	assertField(t, httpCfg, "CustomLocation", "'https://api.example.com/odata'")
	assertField(t, httpCfg, "ClientCertificate", "my-cert")

	if v, ok := httpCfg["OverrideLocation"].(bool); !ok || !v {
		t.Errorf("OverrideLocation: expected true, got %v", httpCfg["OverrideLocation"])
	}

	// Header entries
	headers := extractBsonArray(httpCfg["HttpHeaderEntries"])
	if len(headers) != 1 {
		t.Fatalf("HttpHeaderEntries: expected 1, got %d", len(headers))
	}
	h0, ok := headers[0].(map[string]any)
	if !ok {
		t.Fatalf("HttpHeaderEntries[0]: expected map, got %T", headers[0])
	}
	assertField(t, h0, "Key", "X-Api-Key")
	assertField(t, h0, "Value", "'abc123'")
}

func TestSerializePublishedODataService(t *testing.T) {
	w := &Writer{}
	svc := &model.PublishedODataService{
		BaseElement: model.BaseElement{
			ID:       "test-published-id",
			TypeName: "ODataPublish$PublishedODataService2",
		},
		ContainerID:         "test-module-id",
		Name:                "CustomerAPI",
		Path:                "/odata/customers",
		Version:             "1.0.0",
		ODataVersion:        "OData4",
		Namespace:           "MyApp.Customers",
		ServiceName:         "Customer Service",
		Summary:             "API for customers",
		PublishAssociations: true,
		AuthenticationTypes: []string{"Basic", "Session"},
		EntityTypes: []*model.PublishedEntityType{
			{
				BaseElement: model.BaseElement{ID: "11111111-1111-1111-1111-111111111111"},
				Entity:      "MyModule.Customer",
				ExposedName: "Customers",
				Members: []*model.PublishedMember{
					{
						BaseElement: model.BaseElement{ID: "m-1"},
						Kind:        "attribute",
						Name:        "Name",
						ExposedName: "CustomerName",
						Filterable:  true,
						Sortable:    true,
					},
					{
						BaseElement: model.BaseElement{ID: "m-2"},
						Kind:        "id",
						Name:        "ID",
						ExposedName: "Id",
						IsPartOfKey: true,
					},
				},
			},
		},
		EntitySets: []*model.PublishedEntitySet{
			{
				BaseElement:    model.BaseElement{ID: "es-1"},
				ExposedName:    "Customers",
				EntityTypeName: "MyModule.Customer",
				ReadMode:       "ReadFromDatabase",
				InsertMode:     "ChangeFromDatabase",
				UpdateMode:     "ChangeFromDatabase",
				DeleteMode:     "NotSupported",
				UsePaging:      true,
				PageSize:       100,
			},
		},
	}

	data, err := w.serializePublishedODataService(svc)
	if err != nil {
		t.Fatalf("serialize failed: %v", err)
	}

	var raw map[string]any
	if err := bson.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	// Top-level fields
	assertField(t, raw, "$Type", "ODataPublish$PublishedODataService2")
	assertField(t, raw, "Name", "CustomerAPI")
	assertField(t, raw, "Path", "/odata/customers")
	assertField(t, raw, "Version", "1.0.0")
	assertField(t, raw, "ODataVersion", "OData4")
	assertField(t, raw, "Namespace", "MyApp.Customers")
	assertField(t, raw, "ServiceName", "Customer Service")

	if v, ok := raw["PublishAssociations"].(bool); !ok || !v {
		t.Errorf("PublishAssociations: expected true, got %v", raw["PublishAssociations"])
	}

	// Authentication types (versioned array: [int32(3), "Basic", "Session"])
	authArr := extractBsonArray(raw["AuthenticationTypes"])
	if len(authArr) != 2 {
		t.Errorf("AuthenticationTypes: expected 2 items, got %d", len(authArr))
	}
	if len(authArr) >= 2 {
		if authArr[0] != "Basic" {
			t.Errorf("AuthenticationTypes[0]: expected Basic, got %v", authArr[0])
		}
		if authArr[1] != "Session" {
			t.Errorf("AuthenticationTypes[1]: expected Session, got %v", authArr[1])
		}
	}

	// Entity types array
	entityTypes := extractBsonArray(raw["EntityTypes"])
	if len(entityTypes) != 1 {
		t.Fatalf("EntityTypes: expected 1, got %d", len(entityTypes))
	}
	etMap, ok := entityTypes[0].(map[string]any)
	if !ok {
		t.Fatalf("EntityTypes[0]: expected map, got %T", entityTypes[0])
	}
	assertField(t, etMap, "$Type", "ODataPublish$EntityType")
	assertField(t, etMap, "Entity", "MyModule.Customer")
	assertField(t, etMap, "ExposedName", "Customers")

	// Child members
	members := extractBsonArray(etMap["ChildMembers"])
	if len(members) != 2 {
		t.Fatalf("ChildMembers: expected 2, got %d", len(members))
	}
	m0, ok := members[0].(map[string]any)
	if !ok {
		t.Fatalf("ChildMembers[0]: expected map, got %T", members[0])
	}
	assertField(t, m0, "$Type", "ODataPublish$PublishedAttribute")
	// Attribute is the fully-qualified Module.Entity.AttributeName — Studio
	// Pro requires qualified references, and using bare names made the
	// second entity in a multi-entity service silently fail to resolve.
	assertField(t, m0, "Attribute", "MyModule.Customer.Name")
	assertField(t, m0, "ExposedName", "CustomerName")
	if v, ok := m0["Filterable"].(bool); !ok || !v {
		t.Errorf("Member Filterable: expected true, got %v", m0["Filterable"])
	}

	m1, ok := members[1].(map[string]any)
	if !ok {
		t.Fatalf("ChildMembers[1]: expected map, got %T", members[1])
	}
	assertField(t, m1, "$Type", "ODataPublish$PublishedId")
	if v, ok := m1["IsPartOfKey"].(bool); !ok || !v {
		t.Errorf("Member IsPartOfKey: expected true, got %v", m1["IsPartOfKey"])
	}

	// Entity sets
	entitySets := extractBsonArray(raw["EntitySets"])
	if len(entitySets) != 1 {
		t.Fatalf("EntitySets: expected 1, got %d", len(entitySets))
	}
	esMap, ok := entitySets[0].(map[string]any)
	if !ok {
		t.Fatalf("EntitySets[0]: expected map, got %T", entitySets[0])
	}
	assertField(t, esMap, "$Type", "ODataPublish$EntitySet")
	assertField(t, esMap, "ExposedName", "Customers")

	// Issue #595: EntityTypePointer must reference the owning EntityType.
	// Without it, Studio Pro's EntitySet.Check NREs (it can't navigate from
	// the set to its type). The map lookup in serializePublishedODataService
	// was previously keyed by ExposedName instead of the qualified entity
	// name, so the resolved ID was always empty and the pointer was omitted.
	etID := etMap["$ID"].(primitive.Binary)
	esPointer, ok := esMap["EntityTypePointer"].(primitive.Binary)
	if !ok {
		t.Fatalf("EntityTypePointer: expected primitive.Binary, got %T (%v)", esMap["EntityTypePointer"], esMap["EntityTypePointer"])
	}
	if string(esPointer.Data) != string(etID.Data) {
		t.Errorf("EntityTypePointer = %x, want %x (entity type $ID)", esPointer.Data, etID.Data)
	}

	if v, ok := esMap["UsePaging"].(bool); !ok || !v {
		t.Errorf("UsePaging: expected true, got %v", esMap["UsePaging"])
	}

	// Mode objects
	readMode, ok := esMap["ReadMode"].(map[string]any)
	if !ok {
		t.Fatalf("ReadMode: expected map, got %T", esMap["ReadMode"])
	}
	assertField(t, readMode, "$Type", "ODataPublish$ReadSource")

	deleteMode, ok := esMap["DeleteMode"].(map[string]any)
	if !ok {
		t.Fatalf("DeleteMode: expected map, got %T", esMap["DeleteMode"])
	}
	assertField(t, deleteMode, "$Type", "ODataPublish$ChangeNotSupported")
}

func TestSerializeModeRoundTrip(t *testing.T) {
	tests := []struct {
		name     string
		mode     string
		isRead   bool
		expected string
	}{
		{"ReadSource", "ReadFromDatabase", true, "ODataPublish$ReadSource"},
		{"ReadSourceMDL", "SOURCE", true, "ODataPublish$ReadSource"},
		{"ChangeSource", "ChangeFromDatabase", false, "ODataPublish$ChangeSource"},
		{"ChangeSourceMDL", "SOURCE", false, "ODataPublish$ChangeSource"},
		{"NotSupported", "NotSupported", false, "ODataPublish$ChangeNotSupported"},
		{"NotSupportedMDL", "NOT_SUPPORTED", false, "ODataPublish$ChangeNotSupported"},
		{"CallMicroflowRead", "CallMicroflow:MyModule.ReadMF", true, "ODataPublish$CallMicroflowToRead"},
		{"CallMicroflowChange", "CallMicroflow:MyModule.WriteMF", false, "ODataPublish$CallMicroflowToChange"},
		{"MicroflowMDLRead", "MICROFLOW MyModule.ReadMF", true, "ODataPublish$CallMicroflowToRead"},
		{"MicroflowMDLChange", "MICROFLOW MyModule.WriteMF", false, "ODataPublish$CallMicroflowToChange"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var result bson.M
			if tc.isRead {
				result = dToM(serializeReadMode(tc.mode))
			} else {
				result = dToM(serializeChangeMode(tc.mode))
			}

			typeName, ok := result["$Type"].(string)
			if !ok {
				t.Fatalf("$Type: expected string, got %T", result["$Type"])
			}
			if typeName != tc.expected {
				t.Errorf("$Type: expected %s, got %s", tc.expected, typeName)
			}

			// Verify $ID is present
			if _, ok := result["$ID"]; !ok {
				t.Error("$ID: expected to be present")
			}
		})
	}
}

// assertField checks a string field in a BSON map.
func assertField(t *testing.T, m map[string]any, key, expected string) {
	t.Helper()
	val, ok := m[key]
	if !ok {
		t.Errorf("field %q: missing", key)
		return
	}
	s, ok := val.(string)
	if !ok {
		t.Errorf("field %q: expected string, got %T", key, val)
		return
	}
	if s != expected {
		t.Errorf("field %q: expected %q, got %q", key, expected, s)
	}
}
