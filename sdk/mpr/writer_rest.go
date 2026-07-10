// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"fmt"
	"strings"

	"github.com/JordtenBulte-OLC/mxcli/model"

	"go.mongodb.org/mongo-driver/bson"
)

// CreateConsumedRestService creates a new consumed REST service document.
func (w *Writer) CreateConsumedRestService(svc *model.ConsumedRestService) error {
	if svc.ID == "" {
		svc.ID = model.ID(generateUUID())
	}
	svc.TypeName = "Rest$ConsumedRestService"

	contents, err := w.serializeConsumedRestService(svc)
	if err != nil {
		return fmt.Errorf("failed to serialize consumed REST service: %w", err)
	}

	return w.insertUnit(string(svc.ID), string(svc.ContainerID), "Documents", "Rest$ConsumedRestService", contents)
}

// UpdateConsumedRestService updates an existing consumed REST service.
func (w *Writer) UpdateConsumedRestService(svc *model.ConsumedRestService) error {
	contents, err := w.serializeConsumedRestService(svc)
	if err != nil {
		return fmt.Errorf("failed to serialize consumed REST service: %w", err)
	}

	return w.updateUnit(string(svc.ID), contents)
}

// DeleteConsumedRestService deletes a consumed REST service by ID.
func (w *Writer) DeleteConsumedRestService(id model.ID) error {
	return w.deleteUnit(string(id))
}

// serializeConsumedRestService converts a ConsumedRestService to BSON bytes.
func (w *Writer) serializeConsumedRestService(svc *model.ConsumedRestService) ([]byte, error) {
	doc := bson.D{
		{Key: "$ID", Value: idToBsonBinary(string(svc.ID))},
		{Key: "$Type", Value: "Rest$ConsumedRestService"},
		{Key: "Name", Value: svc.Name},
		{Key: "Documentation", Value: svc.Documentation},
		{Key: "Excluded", Value: svc.Excluded},
		// ExportLevel: whether the document is exposed to other modules/projects.
		// Studio Pro defaults to "Hidden". Missing this field has been observed
		// to cause runtime auth issues (#200).
		{Key: "ExportLevel", Value: "Hidden"},
		{Key: "BaseUrlParameter", Value: nil},
	}

	// OpenApiFile: only present when the service was created from an OpenAPI spec.
	// Field name and subfield are PascalCase to match Studio Pro serialization.
	// Do NOT write a null entry for manually-created services — Studio Pro omits this field entirely.
	if svc.OpenApiContent != "" {
		doc = append(doc, bson.E{Key: "OpenApiFile", Value: bson.D{
			{Key: "$ID", Value: idToBsonBinary(generateUUID())},
			{Key: "$Type", Value: "Rest$OpenApiFile"},
			{Key: "Content", Value: svc.OpenApiContent},
		}})
	}

	// BaseUrl as Rest$ValueTemplate
	doc = append(doc, bson.E{Key: "BaseUrl", Value: serializeValueTemplate(svc.BaseUrl)})

	// AuthenticationScheme: polymorphic (null or Rest$BasicAuthenticationScheme)
	if svc.Authentication == nil {
		doc = append(doc, bson.E{Key: "AuthenticationScheme", Value: nil})
	} else {
		doc = append(doc, bson.E{Key: "AuthenticationScheme", Value: serializeRestAuthScheme(svc.Authentication)})
	}

	// Operations: versioned array
	ops := bson.A{int32(2)}
	for _, op := range svc.Operations {
		ops = append(ops, serializeRestOperation(op))
	}
	doc = append(doc, bson.E{Key: "Operations", Value: ops})

	return marshalUnitIDFirst(doc)
}

// serializeValueTemplate creates a Rest$ValueTemplate BSON object.
func serializeValueTemplate(value string) bson.D {
	return bson.D{
		{Key: "$ID", Value: idToBsonBinary(generateUUID())},
		{Key: "$Type", Value: "Rest$ValueTemplate"},
		{Key: "Value", Value: value},
	}
}

// serializeRestAuthScheme converts authentication config to a BSON map.
func serializeRestAuthScheme(auth *model.RestAuthentication) bson.D {
	doc := bson.D{
		{Key: "$ID", Value: idToBsonBinary(generateUUID())},
		{Key: "$Type", Value: "Rest$BasicAuthenticationScheme"},
	}

	doc = append(doc, bson.E{Key: "Username", Value: serializeRestValue(auth.Username)})
	doc = append(doc, bson.E{Key: "Password", Value: serializeRestValue(auth.Password)})

	return doc
}

// serializeRestValue creates a polymorphic Rest$Value (StringValue or ConstantValue).
// Values starting with "$" are treated as constant references; others as string literals.
func serializeRestValue(value string) bson.D {
	if strings.HasPrefix(value, "$") {
		// Constant reference — the BSON field is "Value" (QualifiedName of the constant).
		constRef := strings.TrimPrefix(value, "$")
		return bson.D{
			{Key: "$ID", Value: idToBsonBinary(generateUUID())},
			{Key: "$Type", Value: "Rest$ConstantValue"},
			{Key: "Value", Value: constRef},
		}
	}
	return bson.D{
		{Key: "$ID", Value: idToBsonBinary(generateUUID())},
		{Key: "$Type", Value: "Rest$StringValue"},
		{Key: "Value", Value: value},
	}
}

// serializeRestOperation converts a RestClientOperation to a BSON map.
func serializeRestOperation(op *model.RestClientOperation) bson.D {
	doc := bson.D{
		{Key: "$ID", Value: idToBsonBinary(generateUUID())},
		{Key: "$Type", Value: "Rest$RestOperation"},
		{Key: "Name", Value: op.Name},
	}

	// Timeout: Studio Pro always writes this field; default is 300 seconds.
	timeout := int64(op.Timeout)
	if timeout <= 0 {
		timeout = 300
	}
	doc = append(doc, bson.E{Key: "Timeout", Value: timeout})

	// Tags: versioned string array; used by Studio Pro as resource group labels.
	tags := bson.A{int32(1)}
	for _, t := range op.Tags {
		tags = append(tags, t)
	}
	doc = append(doc, bson.E{Key: "Tags", Value: tags})

	// Method: polymorphic (WithBody or WithoutBody)
	doc = append(doc, bson.E{Key: "Method", Value: serializeRestMethod(op)})

	// Path as Rest$ValueTemplate
	doc = append(doc, bson.E{Key: "Path", Value: serializeValueTemplate(op.Path)})

	// Headers: versioned array of Rest$HeaderWithValueTemplate
	headers := bson.A{int32(2)}
	hasAccept := false
	for _, h := range op.Headers {
		headers = append(headers, serializeRestHeader(h))
		if strings.EqualFold(h.Name, "Accept") {
			hasAccept = true
		}
	}
	// Mendix requires an Accept header on every consumed REST operation (CE7062)
	if !hasAccept {
		headers = append(headers, serializeRestHeader(&model.RestClientHeader{Name: "Accept", Value: "*/*"}))
	}
	doc = append(doc, bson.E{Key: "Headers", Value: headers})

	// Parameters: versioned array of Rest$RestOperationParameter (path params)
	params := bson.A{int32(2)}
	for _, p := range op.Parameters {
		params = append(params, serializeRestParameter(p))
	}
	doc = append(doc, bson.E{Key: "Parameters", Value: params})

	// QueryParameters: versioned array of Rest$QueryParameter
	queryParams := bson.A{int32(2)}
	for _, q := range op.QueryParameters {
		queryParams = append(queryParams, serializeRestQueryParameter(q))
	}
	doc = append(doc, bson.E{Key: "QueryParameters", Value: queryParams})

	// ResponseHandling: polymorphic
	if op.ResponseType == "MAPPING" && op.ResponseEntity != "" && len(op.ResponseMappings) > 0 {
		doc = append(doc, bson.E{Key: "ResponseHandling", Value: serializeRestImplicitMappingResponse(op.ResponseEntity, op.ResponseMappings)})
	} else {
		doc = append(doc, bson.E{Key: "ResponseHandling", Value: serializeRestResponseHandling(op.ResponseType)})
	}

	return doc
}

// serializeRestMethod creates the polymorphic Method field.
// Methods with bodies (POST, PUT, PATCH) use Rest$RestOperationMethodWithBody;
// others use Rest$RestOperationMethodWithoutBody.
func serializeRestMethod(op *model.RestClientOperation) bson.D {
	httpMethod := httpMethodToMendix(op.HttpMethod)

	if op.BodyType != "" {
		// Method with explicit body
		doc := bson.D{
			{Key: "$ID", Value: idToBsonBinary(generateUUID())},
			{Key: "$Type", Value: "Rest$RestOperationMethodWithBody"},
			{Key: "HttpMethod", Value: httpMethod},
		}
		if op.BodyType == "EXPORT_MAPPING" && len(op.BodyMappings) > 0 {
			doc = append(doc, bson.E{Key: "Body", Value: serializeRestImplicitMappingBody(op.BodyVariable, op.BodyMappings)})
		} else {
			doc = append(doc, bson.E{Key: "Body", Value: serializeRestBody(op.BodyType, op.BodyVariable)})
		}
		return doc
	}

	// POST, PUT, PATCH must include a body even if not explicitly specified (CE7064)
	methodUpper := strings.ToUpper(op.HttpMethod)
	if methodUpper == "POST" || methodUpper == "PUT" || methodUpper == "PATCH" {
		doc := bson.D{
			{Key: "$ID", Value: idToBsonBinary(generateUUID())},
			{Key: "$Type", Value: "Rest$RestOperationMethodWithBody"},
			{Key: "HttpMethod", Value: httpMethod},
		}
		doc = append(doc, bson.E{Key: "Body", Value: serializeRestBody("JSON", op.BodyVariable)})
		return doc
	}

	// Method without body
	return bson.D{
		{Key: "$ID", Value: idToBsonBinary(generateUUID())},
		{Key: "$Type", Value: "Rest$RestOperationMethodWithoutBody"},
		{Key: "HttpMethod", Value: httpMethod},
	}
}

// serializeRestBody creates a polymorphic Body field.
// Uses Rest$JsonBody instead of Rest$ImplicitMappingBody to avoid CE7247/CE0061
// (ImplicitMappingBody requires entity mapping which isn't supported yet).
//
// bodyExpr is a Mendix expression (typically "$variableName") that produces
// the JSON or file body at call time. It is stored verbatim in the BSON Value
// field so a roundtrip preserves it.
func serializeRestBody(bodyType, bodyExpr string) bson.D {
	switch strings.ToUpper(bodyType) {
	case "JSON":
		return bson.D{
			{Key: "$ID", Value: idToBsonBinary(generateUUID())},
			{Key: "$Type", Value: "Rest$JsonBody"},
			{Key: "Value", Value: bodyExpr},
		}
	case "FILE", "TEMPLATE":
		return bson.D{
			{Key: "$ID", Value: idToBsonBinary(generateUUID())},
			{Key: "$Type", Value: "Rest$StringBody"},
			{Key: "ValueTemplate", Value: serializeValueTemplate(bodyExpr)},
		}
	default:
		return bson.D{
			{Key: "$ID", Value: idToBsonBinary(generateUUID())},
			{Key: "$Type", Value: "Rest$JsonBody"},
			{Key: "Value", Value: bodyExpr},
		}
	}
}

// serializeRestImplicitMappingBody creates a Rest$ImplicitMappingBody with an inline
// export mapping tree (ExportMappings$ObjectMappingElement). Used for Body: MAPPING Entity { ... }.
func serializeRestImplicitMappingBody(entity string, mappings []*model.RestResponseMapping) bson.D {
	rootElement := serializeInlineMappingElement(entity, "", "", "(Object)", mappings, "ExportMappings", "Parameter")

	return bson.D{
		{Key: "$ID", Value: idToBsonBinary(generateUUID())},
		{Key: "$Type", Value: "Rest$ImplicitMappingBody"},
		{Key: "RootMappingElement", Value: rootElement},
		{Key: "TestValue", Value: bson.D{
			{Key: "$ID", Value: idToBsonBinary(generateUUID())},
			{Key: "$Type", Value: "Rest$StringValue"},
			{Key: "Value", Value: ""},
		}},
	}
}

// serializeRestImplicitMappingResponse creates a Rest$ImplicitMappingResponseHandling with an
// inline import mapping tree (ImportMappings$ObjectMappingElement). Used for Response: MAPPING Entity { ... }.
func serializeRestImplicitMappingResponse(entity string, mappings []*model.RestResponseMapping) bson.D {
	rootElement := serializeInlineMappingElement(entity, "", "", "(Object)", mappings, "ImportMappings", "Create")

	return bson.D{
		{Key: "$ID", Value: idToBsonBinary(generateUUID())},
		{Key: "$Type", Value: "Rest$ImplicitMappingResponseHandling"},
		{Key: "ContentType", Value: "application/json"},
		{Key: "RootMappingElement", Value: rootElement},
		{Key: "StatusCode", Value: int32(200)},
	}
}

// serializeInlineMappingElement creates a single ObjectMappingElement with children for inline REST mappings.
// namespace is "ImportMappings" or "ExportMappings". objectHandling is "Create" or "Parameter".
func serializeInlineMappingElement(entity, association, exposedName, jsonPath string, mappings []*model.RestResponseMapping, namespace, objectHandling string) bson.D {
	children := bson.A{int32(2)}
	for _, m := range mappings {
		if m.Entity != "" {
			// Nested object mapping
			childJsonPath := jsonPath + "|" + m.ExposedName
			child := serializeInlineMappingElement(m.Entity, m.Association, m.ExposedName, childJsonPath, m.Children, namespace, "Create")
			children = append(children, child)
		} else {
			// Value mapping
			valueJsonPath := m.JsonPath
			if valueJsonPath == "" {
				valueJsonPath = jsonPath + "|" + m.ExposedName
			}
			attrQN := entity + "." + m.Attribute
			children = append(children, bson.D{
				{Key: "$ID", Value: idToBsonBinary(generateUUID())},
				{Key: "$Type", Value: namespace + "$ValueMappingElement"},
				{Key: "Attribute", Value: attrQN},
				{Key: "ExposedName", Value: m.ExposedName},
				{Key: "JsonPath", Value: valueJsonPath},
				{Key: "XmlPath", Value: ""},
				{Key: "IsKey", Value: false},
				{Key: "Type", Value: bson.D{{Key: "$ID", Value: idToBsonBinary(generateUUID())}, {Key: "$Type", Value: "DataTypes$StringType"}}},
				{Key: "MinOccurs", Value: int32(0)},
				{Key: "MaxOccurs", Value: int32(1)},
				{Key: "Nillable", Value: true},
				{Key: "IsDefaultType", Value: false},
				{Key: "ElementType", Value: "Value"},
				{Key: "Documentation", Value: ""},
				{Key: "Converter", Value: ""},
				{Key: "FractionDigits", Value: int32(-1)},
				{Key: "TotalDigits", Value: int32(-1)},
				{Key: "MaxLength", Value: int32(0)},
				{Key: "IsContent", Value: false},
				{Key: "IsXmlAttribute", Value: false},
				{Key: "OriginalValue", Value: ""},
				{Key: "XmlPrimitiveType", Value: "String"},
			})
		}
	}

	minOccurs := int32(1)
	if association != "" {
		minOccurs = 0
	}

	return bson.D{
		{Key: "$ID", Value: idToBsonBinary(generateUUID())},
		{Key: "$Type", Value: namespace + "$ObjectMappingElement"},
		{Key: "Entity", Value: entity},
		{Key: "ExposedName", Value: exposedName},
		{Key: "JsonPath", Value: jsonPath},
		{Key: "XmlPath", Value: ""},
		{Key: "ObjectHandling", Value: objectHandling},
		{Key: "ObjectHandlingBackup", Value: "Create"},
		{Key: "ObjectHandlingBackupAllowOverride", Value: false},
		{Key: "Association", Value: association},
		{Key: "Children", Value: children},
		{Key: "MinOccurs", Value: minOccurs},
		{Key: "MaxOccurs", Value: int32(1)},
		{Key: "Nillable", Value: true},
		{Key: "IsDefaultType", Value: false},
		{Key: "ElementType", Value: "Object"},
		{Key: "Documentation", Value: ""},
		{Key: "CustomHandlerCall", Value: nil},
	}
}

// serializeRestHeader creates a Rest$HeaderWithValueTemplate BSON object.
func serializeRestHeader(h *model.RestClientHeader) bson.D {
	return bson.D{
		{Key: "$ID", Value: idToBsonBinary(generateUUID())},
		{Key: "$Type", Value: "Rest$HeaderWithValueTemplate"},
		{Key: "Name", Value: h.Name},
		{Key: "Value", Value: serializeValueTemplate(h.Value)},
	}
}

// serializeRestParameter creates a Rest$OperationParameter BSON object.
// This is the correct type for consumed REST operation parameters
// (distinct from Rest$RestOperationParameter used in published REST services).
func serializeRestParameter(p *model.RestClientParameter) bson.D {
	return bson.D{
		{Key: "$ID", Value: idToBsonBinary(generateUUID())},
		{Key: "$Type", Value: "Rest$OperationParameter"},
		{Key: "Name", Value: p.Name},
		{Key: "DataType", Value: serializeRestDataType(p.DataType)},
	}
}

// serializeRestQueryParameter creates a Rest$QueryParameter BSON object.
func serializeRestQueryParameter(p *model.RestClientParameter) bson.D {
	return bson.D{
		{Key: "$ID", Value: idToBsonBinary(generateUUID())},
		{Key: "$Type", Value: "Rest$QueryParameter"},
		{Key: "Name", Value: p.Name},
		{Key: "ParameterUsage", Value: bson.D{
			{Key: "$ID", Value: idToBsonBinary(generateUUID())},
			{Key: "$Type", Value: "Rest$RequiredQueryParameterUsage"},
		}},
	}
}

// serializeRestResponseHandling creates a polymorphic ResponseHandling BSON object.
// Uses Rest$NoResponseHandling for all types to avoid CE0061 (ImplicitMappingResponseHandling
// requires entity mapping which isn't supported yet). ContentType is set to enable roundtripping.
func serializeRestResponseHandling(responseType string) bson.D {
	doc := bson.D{
		{Key: "$ID", Value: idToBsonBinary(generateUUID())},
		{Key: "$Type", Value: "Rest$NoResponseHandling"},
	}
	switch strings.ToUpper(responseType) {
	case "JSON":
		doc = append(doc, bson.E{Key: "ContentType", Value: "application/json"})
	case "STRING":
		doc = append(doc, bson.E{Key: "ContentType", Value: "text/plain"})
	case "FILE":
		doc = append(doc, bson.E{Key: "ContentType", Value: "application/octet-stream"})
	}
	return doc
}

// serializeRestDataType converts a simple type name to a BSON DataType object.
// REST operation parameters use the DataTypes$ namespace with simple type names
// (e.g., DataTypes$IntegerType, not DataTypes$IntegerAttributeType).
func serializeRestDataType(typeName string) bson.D {
	bsonType := "DataTypes$StringType"
	switch typeName {
	case "Integer":
		bsonType = "DataTypes$IntegerType"
	case "Long":
		bsonType = "DataTypes$IntegerType" // Long maps to IntegerType in DataTypes
	case "Decimal":
		bsonType = "DataTypes$DecimalType"
	case "Boolean":
		bsonType = "DataTypes$BooleanType"
	case "String":
		bsonType = "DataTypes$StringType"
	}
	return bson.D{
		{Key: "$ID", Value: idToBsonBinary(generateUUID())},
		{Key: "$Type", Value: bsonType},
	}
}

// CreatePublishedRestService creates a new published REST service document.
func (w *Writer) CreatePublishedRestService(svc *model.PublishedRestService) error {
	if svc.ID == "" {
		svc.ID = model.ID(generateUUID())
	}
	svc.TypeName = "Rest$PublishedRestService"

	contents, err := w.serializePublishedRestService(svc)
	if err != nil {
		return fmt.Errorf("failed to serialize published REST service: %w", err)
	}

	return w.insertUnit(string(svc.ID), string(svc.ContainerID), "Documents", "Rest$PublishedRestService", contents)
}

// DeletePublishedRestService deletes a published REST service by ID.
func (w *Writer) DeletePublishedRestService(id model.ID) error {
	return w.deleteUnit(string(id))
}

// UpdatePublishedRestService re-serializes an existing published REST
// service. Used by ALTER PUBLISHED REST SERVICE.
func (w *Writer) UpdatePublishedRestService(svc *model.PublishedRestService) error {
	contents, err := w.serializePublishedRestService(svc)
	if err != nil {
		return fmt.Errorf("failed to serialize published REST service: %w", err)
	}
	return w.updateUnit(string(svc.ID), contents)
}

func (w *Writer) serializePublishedRestService(svc *model.PublishedRestService) ([]byte, error) {
	resources := bson.A{int32(2)}
	for _, res := range svc.Resources {
		ops := bson.A{int32(2)}
		for _, op := range res.Operations {
			opDoc := bson.D{
				{Key: "$ID", Value: idToBsonBinary(GenerateID())},
				{Key: "$Type", Value: "Rest$PublishedRestServiceOperation"},
				{Key: "HttpMethod", Value: httpMethodToMendix(op.HTTPMethod)},
				{Key: "Path", Value: op.Path},
				{Key: "Microflow", Value: op.Microflow},
				{Key: "Summary", Value: op.Summary},
				{Key: "Deprecated", Value: op.Deprecated},
				{Key: "Commit", Value: "Yes"},
				{Key: "Documentation", Value: ""},
				{Key: "ExportMapping", Value: ""},
				{Key: "ImportMapping", Value: ""},
				{Key: "ObjectHandlingBackup", Value: "Create"},
				{Key: "Parameters", Value: serializePublishedRestParams(op.Path, op.Microflow, op.Parameters)},
			}
			ops = append(ops, opDoc)
		}
		resDoc := bson.D{
			{Key: "$ID", Value: idToBsonBinary(GenerateID())},
			{Key: "$Type", Value: "Rest$PublishedRestServiceResource"},
			{Key: "Name", Value: res.Name},
			{Key: "Documentation", Value: ""},
			{Key: "Operations", Value: ops},
		}
		resources = append(resources, resDoc)
	}

	doc := bson.D{
		{Key: "$ID", Value: idToBsonBinary(string(svc.ID))},
		{Key: "$Type", Value: "Rest$PublishedRestService"},
		{Key: "Name", Value: svc.Name},
		{Key: "Documentation", Value: ""},
		{Key: "Excluded", Value: svc.Excluded},
		{Key: "ExportLevel", Value: "Hidden"},
		{Key: "Path", Value: svc.Path},
		{Key: "Version", Value: svc.Version},
		{Key: "ServiceName", Value: svc.ServiceName},
		{Key: "AllowedRoles", Value: makeMendixStringArray(svc.AllowedRoles)},
		{Key: "AuthenticationTypes", Value: bson.A{int32(2)}},
		{Key: "AuthenticationMicroflow", Value: ""},
		{Key: "CorsConfiguration", Value: nil},
		{Key: "Parameters", Value: bson.A{int32(2)}},
		{Key: "Resources", Value: resources},
	}

	return marshalUnitIDFirst(doc)
}

// serializePublishedRestParams builds the Parameters array for a published REST operation.
// It auto-extracts path parameters from {paramName} placeholders in the path string,
// then appends any explicitly declared parameters.
//
// Each parameter must include:
//   - Type: a structured DataTypes$StringType object (not a bare string)
//   - ParameterType: "Path" (vs Query/Header/Body)
//   - MicroflowParameter: qualified name of the matching microflow parameter,
//     so Mendix wires the path value to the handler. Without this, mx check
//     reports CE6538 "Parameter is not passed to a microflow parameter" and
//     CE0350 "Microflow has a parameter that is not a parameter of the operation".
func serializePublishedRestParams(path string, microflowQN string, _ []string) bson.A {
	params := bson.A{int32(2)}
	// Extract {paramName} from path
	for _, name := range extractPathParams(path) {
		// MicroflowParameter format: "Module.Microflow.parameterName"
		mfParam := ""
		if microflowQN != "" {
			mfParam = microflowQN + "." + name
		}
		params = append(params, bson.D{
			{Key: "$ID", Value: idToBsonBinary(generateUUID())},
			{Key: "$Type", Value: "Rest$RestOperationParameter"},
			{Key: "Name", Value: name},
			{Key: "Type", Value: bson.D{
				{Key: "$ID", Value: idToBsonBinary(generateUUID())},
				{Key: "$Type", Value: "DataTypes$StringType"},
			}},
			{Key: "ParameterType", Value: "Path"},
			{Key: "MicroflowParameter", Value: mfParam},
			{Key: "Description", Value: ""},
		})
	}
	return params
}

// extractPathParams returns parameter names from {param} placeholders in a path.
func extractPathParams(path string) []string {
	var names []string
	for {
		start := strings.Index(path, "{")
		if start < 0 {
			break
		}
		end := strings.Index(path[start:], "}")
		if end < 0 {
			break
		}
		names = append(names, path[start+1:start+end])
		path = path[start+end+1:]
	}
	return names
}

// httpMethodToMendix converts uppercase HTTP method names to Mendix casing.
func httpMethodToMendix(method string) string {
	switch strings.ToUpper(method) {
	case "GET":
		return "Get"
	case "POST":
		return "Post"
	case "PUT":
		return "Put"
	case "PATCH":
		return "Patch"
	case "DELETE":
		return "Delete"
	case "HEAD":
		return "Head"
	case "OPTIONS":
		return "Options"
	default:
		return method
	}
}
