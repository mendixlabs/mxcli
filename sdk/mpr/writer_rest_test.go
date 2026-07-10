// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/model"
	"go.mongodb.org/mongo-driver/bson"
)

func TestSerializeConsumedRestServiceBasic(t *testing.T) {
	w := &Writer{}
	svc := &model.ConsumedRestService{
		BaseElement: model.BaseElement{
			ID:       "test-rest-id",
			TypeName: "Rest$ConsumedRestService",
		},
		ContainerID: "test-module-id",
		Name:        "PetStoreAPI",
		BaseUrl:     "https://petstore.swagger.io/v2",
	}

	data, err := w.serializeConsumedRestService(svc)
	if err != nil {
		t.Fatalf("serialize failed: %v", err)
	}

	var raw map[string]any
	if err := bson.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	assertField(t, raw, "$Type", "Rest$ConsumedRestService")
	assertField(t, raw, "Name", "PetStoreAPI")

	// BaseUrl should be a ValueTemplate
	baseUrl, ok := raw["BaseUrl"].(map[string]any)
	if !ok {
		t.Fatalf("BaseUrl: expected map, got %T", raw["BaseUrl"])
	}
	assertField(t, baseUrl, "$Type", "Rest$ValueTemplate")
	assertField(t, baseUrl, "Value", "https://petstore.swagger.io/v2")

	// AuthenticationScheme should be nil
	if raw["AuthenticationScheme"] != nil {
		t.Errorf("AuthenticationScheme: expected nil, got %v", raw["AuthenticationScheme"])
	}
}

func TestSerializeConsumedRestServiceWithAuth(t *testing.T) {
	w := &Writer{}
	svc := &model.ConsumedRestService{
		BaseElement: model.BaseElement{
			ID: "test-rest-auth-id",
		},
		ContainerID: "test-module-id",
		Name:        "SecureAPI",
		BaseUrl:     "https://api.example.com",
		Authentication: &model.RestAuthentication{
			Scheme:   "Basic",
			Username: "admin",
			Password: "secret",
		},
	}

	data, err := w.serializeConsumedRestService(svc)
	if err != nil {
		t.Fatalf("serialize failed: %v", err)
	}

	var raw map[string]any
	if err := bson.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	// AuthenticationScheme should be BasicAuthenticationScheme
	authScheme, ok := raw["AuthenticationScheme"].(map[string]any)
	if !ok {
		t.Fatalf("AuthenticationScheme: expected map, got %T", raw["AuthenticationScheme"])
	}
	assertField(t, authScheme, "$Type", "Rest$BasicAuthenticationScheme")

	// Username should be StringValue (literal)
	username, ok := authScheme["Username"].(map[string]any)
	if !ok {
		t.Fatalf("Username: expected map, got %T", authScheme["Username"])
	}
	assertField(t, username, "$Type", "Rest$StringValue")
	assertField(t, username, "Value", "admin")

	// Password should be StringValue (literal)
	password, ok := authScheme["Password"].(map[string]any)
	if !ok {
		t.Fatalf("Password: expected map, got %T", authScheme["Password"])
	}
	assertField(t, password, "$Type", "Rest$StringValue")
	assertField(t, password, "Value", "secret")
}

func TestSerializeConsumedRestServiceWithConstantAuth(t *testing.T) {
	w := &Writer{}
	svc := &model.ConsumedRestService{
		BaseElement: model.BaseElement{
			ID: "test-rest-const-auth",
		},
		ContainerID: "test-module-id",
		Name:        "ConstAuthAPI",
		BaseUrl:     "https://api.example.com",
		Authentication: &model.RestAuthentication{
			Scheme:   "Basic",
			Username: "$MyModule.ApiUser",
			Password: "$MyModule.ApiPass",
		},
	}

	data, err := w.serializeConsumedRestService(svc)
	if err != nil {
		t.Fatalf("serialize failed: %v", err)
	}

	var raw map[string]any
	if err := bson.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	authScheme, ok := raw["AuthenticationScheme"].(map[string]any)
	if !ok {
		t.Fatalf("AuthenticationScheme: expected map, got %T", raw["AuthenticationScheme"])
	}

	// Username should be ConstantValue
	username, ok := authScheme["Username"].(map[string]any)
	if !ok {
		t.Fatalf("Username: expected map, got %T", authScheme["Username"])
	}
	assertField(t, username, "$Type", "Rest$ConstantValue")
	assertField(t, username, "Value", "MyModule.ApiUser")
}

func TestSerializeRestOperationGetWithParams(t *testing.T) {
	op := &model.RestClientOperation{
		Name:       "GetPet",
		HttpMethod: "GET",
		Path:       "/pet/{petId}",
		Parameters: []*model.RestClientParameter{
			{Name: "petId", DataType: "Integer"},
		},
		Headers: []*model.RestClientHeader{
			{Name: "Accept", Value: "application/json"},
		},
		ResponseType: "JSON",
		Timeout:      30,
	}

	result := dToM(serializeRestOperation(op))

	assertField(t, result, "$Type", "Rest$RestOperation")
	assertField(t, result, "Name", "GetPet")

	// Timeout
	if v, ok := result["Timeout"].(int64); !ok || v != 30 {
		t.Errorf("Timeout: expected 30, got %v", result["Timeout"])
	}

	// Method should be WithoutBody (GET)
	method, ok := result["Method"].(bson.M)
	if !ok {
		t.Fatalf("Method: expected bson.M, got %T", result["Method"])
	}
	if method["$Type"] != "Rest$RestOperationMethodWithoutBody" {
		t.Errorf("Method.$Type: expected WithoutBody, got %v", method["$Type"])
	}
	if method["HttpMethod"] != "Get" {
		t.Errorf("Method.HttpMethod: expected Get, got %v", method["HttpMethod"])
	}

	// Path should be ValueTemplate
	path, ok := result["Path"].(bson.M)
	if !ok {
		t.Fatalf("Path: expected bson.M, got %T", result["Path"])
	}
	if path["Value"] != "/pet/{petId}" {
		t.Errorf("Path.Value: expected /pet/{petId}, got %v", path["Value"])
	}

	// Parameters
	params := extractBsonArray(result["Parameters"])
	if len(params) != 1 {
		t.Fatalf("Parameters: expected 1, got %d", len(params))
	}
	p0, ok := params[0].(bson.M)
	if !ok {
		t.Fatalf("Parameters[0]: expected bson.M, got %T", params[0])
	}
	if p0["Name"] != "petId" {
		t.Errorf("Parameter Name: expected petId, got %v", p0["Name"])
	}
	dataType, ok := p0["DataType"].(bson.M)
	if !ok {
		t.Fatalf("Parameter DataType: expected bson.M, got %T", p0["DataType"])
	}
	if dataType["$Type"] != "DataTypes$IntegerType" {
		t.Errorf("Parameter DataType.$Type: expected IntegerAttributeType, got %v", dataType["$Type"])
	}

	// Headers
	headers := extractBsonArray(result["Headers"])
	if len(headers) != 1 {
		t.Fatalf("Headers: expected 1, got %d", len(headers))
	}

	// ResponseHandling (JSON uses NoResponseHandling with ContentType for compatibility)
	respHandling, ok := result["ResponseHandling"].(bson.M)
	if !ok {
		t.Fatalf("ResponseHandling: expected bson.M, got %T", result["ResponseHandling"])
	}
	if respHandling["$Type"] != "Rest$NoResponseHandling" {
		t.Errorf("ResponseHandling.$Type: expected NoResponseHandling, got %v", respHandling["$Type"])
	}
	if respHandling["ContentType"] != "application/json" {
		t.Errorf("ResponseHandling.ContentType: expected application/json, got %v", respHandling["ContentType"])
	}
}

func TestSerializeRestOperationPostWithBody(t *testing.T) {
	op := &model.RestClientOperation{
		Name:         "AddPet",
		HttpMethod:   "POST",
		Path:         "/pet",
		BodyType:     "JSON",
		ResponseType: "JSON",
	}

	result := dToM(serializeRestOperation(op))

	// Method should be WithBody (POST)
	method, ok := result["Method"].(bson.M)
	if !ok {
		t.Fatalf("Method: expected bson.M, got %T", result["Method"])
	}
	if method["$Type"] != "Rest$RestOperationMethodWithBody" {
		t.Errorf("Method.$Type: expected WithBody, got %v", method["$Type"])
	}
	if method["HttpMethod"] != "Post" {
		t.Errorf("Method.HttpMethod: expected Post, got %v", method["HttpMethod"])
	}

	// Body should be JsonBody (used instead of ImplicitMappingBody to avoid CE7247/CE0061)
	body, ok := method["Body"].(bson.M)
	if !ok {
		t.Fatalf("Body: expected bson.M, got %T", method["Body"])
	}
	if body["$Type"] != "Rest$JsonBody" {
		t.Errorf("Body.$Type: expected JsonBody, got %v", body["$Type"])
	}
}

func TestSerializeRestOperationNoResponse(t *testing.T) {
	op := &model.RestClientOperation{
		Name:         "DeletePet",
		HttpMethod:   "DELETE",
		Path:         "/pet/{petId}",
		ResponseType: "NONE",
	}

	result := dToM(serializeRestOperation(op))

	respHandling, ok := result["ResponseHandling"].(bson.M)
	if !ok {
		t.Fatalf("ResponseHandling: expected bson.M, got %T", result["ResponseHandling"])
	}
	if respHandling["$Type"] != "Rest$NoResponseHandling" {
		t.Errorf("ResponseHandling.$Type: expected NoResponseHandling, got %v", respHandling["$Type"])
	}
}

func TestSerializeRestOperationQueryParams(t *testing.T) {
	op := &model.RestClientOperation{
		Name:       "SearchPets",
		HttpMethod: "GET",
		Path:       "/pet/findByStatus",
		QueryParameters: []*model.RestClientParameter{
			{Name: "status", DataType: "String"},
		},
		ResponseType: "JSON",
	}

	result := dToM(serializeRestOperation(op))

	queryParams := extractBsonArray(result["QueryParameters"])
	if len(queryParams) != 1 {
		t.Fatalf("QueryParameters: expected 1, got %d", len(queryParams))
	}
	q0, ok := queryParams[0].(bson.M)
	if !ok {
		t.Fatalf("QueryParameters[0]: expected bson.M, got %T", queryParams[0])
	}
	if q0["Name"] != "status" {
		t.Errorf("QueryParam Name: expected status, got %v", q0["Name"])
	}
	if q0["$Type"] != "Rest$QueryParameter" {
		t.Errorf("QueryParam $Type: expected Rest$QueryParameter, got %v", q0["$Type"])
	}

	// ParameterUsage
	usage, ok := q0["ParameterUsage"].(bson.M)
	if !ok {
		t.Fatalf("ParameterUsage: expected bson.M, got %T", q0["ParameterUsage"])
	}
	if usage["$Type"] != "Rest$RequiredQueryParameterUsage" {
		t.Errorf("ParameterUsage.$Type: expected RequiredQueryParameterUsage, got %v", usage["$Type"])
	}
}

func TestHttpMethodToMendix(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"GET", "Get"},
		{"POST", "Post"},
		{"PUT", "Put"},
		{"PATCH", "Patch"},
		{"DELETE", "Delete"},
		{"HEAD", "Head"},
		{"OPTIONS", "Options"},
	}
	for _, tc := range tests {
		result := httpMethodToMendix(tc.input)
		if result != tc.expected {
			t.Errorf("httpMethodToMendix(%q): expected %q, got %q", tc.input, tc.expected, result)
		}
	}
}

func TestSerializeConsumedRestServiceFullRoundtrip(t *testing.T) {
	w := &Writer{}
	svc := &model.ConsumedRestService{
		BaseElement: model.BaseElement{
			ID: "test-roundtrip-id",
		},
		ContainerID:   "test-module-id",
		Name:          "PetStoreAPI",
		Documentation: "Swagger Pet Store API",
		BaseUrl:       "https://petstore.swagger.io/v2",
		Operations: []*model.RestClientOperation{
			{
				Name:       "ListPets",
				HttpMethod: "GET",
				Path:       "/pet/findByStatus",
				QueryParameters: []*model.RestClientParameter{
					{Name: "status", DataType: "String"},
				},
				Headers: []*model.RestClientHeader{
					{Name: "Accept", Value: "application/json"},
				},
				ResponseType: "JSON",
				Timeout:      30,
			},
			{
				Name:       "GetPet",
				HttpMethod: "GET",
				Path:       "/pet/{petId}",
				Parameters: []*model.RestClientParameter{
					{Name: "petId", DataType: "Integer"},
				},
				ResponseType: "JSON",
			},
			{
				Name:         "AddPet",
				HttpMethod:   "POST",
				Path:         "/pet",
				BodyType:     "JSON",
				ResponseType: "JSON",
			},
			{
				Name:         "RemovePet",
				HttpMethod:   "DELETE",
				Path:         "/pet/{petId}",
				ResponseType: "NONE",
				Parameters: []*model.RestClientParameter{
					{Name: "petId", DataType: "Integer"},
				},
			},
		},
	}

	data, err := w.serializeConsumedRestService(svc)
	if err != nil {
		t.Fatalf("serialize failed: %v", err)
	}

	// Verify the BSON can be deserialized
	var raw map[string]any
	if err := bson.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	// Verify top-level structure
	assertField(t, raw, "$Type", "Rest$ConsumedRestService")
	assertField(t, raw, "Name", "PetStoreAPI")
	assertField(t, raw, "Documentation", "Swagger Pet Store API")

	// Verify operations count
	ops := extractBsonArray(raw["Operations"])
	if len(ops) != 4 {
		t.Fatalf("Operations: expected 4, got %d", len(ops))
	}

	// Verify first operation
	op0, ok := ops[0].(map[string]any)
	if !ok {
		t.Fatalf("Operations[0]: expected map, got %T", ops[0])
	}
	assertField(t, op0, "Name", "ListPets")

	// Verify POST operation has WithBody method
	op2, ok := ops[2].(map[string]any)
	if !ok {
		t.Fatalf("Operations[2]: expected map, got %T", ops[2])
	}
	assertField(t, op2, "Name", "AddPet")
	method2, ok := op2["Method"].(map[string]any)
	if !ok {
		t.Fatalf("Operations[2].Method: expected map, got %T", op2["Method"])
	}
	assertField(t, method2, "$Type", "Rest$RestOperationMethodWithBody")

	// Verify Body is JsonBody
	body2, ok := method2["Body"].(map[string]any)
	if !ok {
		t.Fatalf("Operations[2].Method.Body: expected map, got %T", method2["Body"])
	}
	assertField(t, body2, "$Type", "Rest$JsonBody")

	// Verify DELETE operation has WithoutBody method
	op3, ok := ops[3].(map[string]any)
	if !ok {
		t.Fatalf("Operations[3]: expected map, got %T", ops[3])
	}
	method3, ok := op3["Method"].(map[string]any)
	if !ok {
		t.Fatalf("Operations[3].Method: expected map, got %T", op3["Method"])
	}
	assertField(t, method3, "$Type", "Rest$RestOperationMethodWithoutBody")
}
