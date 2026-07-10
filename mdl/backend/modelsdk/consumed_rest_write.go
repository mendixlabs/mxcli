// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"fmt"
	"strings"

	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/modelsdk/codec"
	"github.com/JordtenBulte-OLC/mxcli/modelsdk/element"
	mmpr "github.com/JordtenBulte-OLC/mxcli/modelsdk/mpr"
)

func init() {
	// A consumed REST service's Operations list and each operation's Headers /
	// Parameters / QueryParameters use the typed-array marker 2; Tags is a marker-1
	// string array. BaseUrlParameter is always null; AuthenticationScheme is null
	// when there is no authentication.
	codec.RegisterListMarker("Rest$RestOperation", 2)
	codec.RegisterListMarker("Rest$HeaderWithValueTemplate", 2)
	codec.RegisterListMarker("Rest$OperationParameter", 2)
	codec.RegisterListMarker("Rest$QueryParameter", 2)
	codec.RegisterTypeDefaults("Rest$ConsumedRestService", codec.TypeDefaults{
		MandatoryListMarkers: map[string]int32{"Operations": 2},
		NullFields:           []string{"BaseUrlParameter", "AuthenticationScheme"},
	})
	codec.RegisterTypeDefaults("Rest$RestOperation", codec.TypeDefaults{
		MandatoryListMarkers: map[string]int32{"Tags": 1, "Headers": 2, "Parameters": 2, "QueryParameters": 2},
	})
}

// CreateConsumedRestService inserts a new Rest$ConsumedRestService document.
// Mirrors the legacy serializer field-for-field.
func (b *Backend) CreateConsumedRestService(svc *model.ConsumedRestService) error {
	if svc == nil {
		return fmt.Errorf("CreateConsumedRestService: nil service")
	}
	if b.writer == nil {
		return fmt.Errorf("CreateConsumedRestService: not connected for writing")
	}
	if svc.ID == "" {
		svc.ID = model.ID(mmpr.GenerateID())
	}
	svc.TypeName = "Rest$ConsumedRestService"
	contents, err := (&codec.Encoder{}).Encode(consumedRestServiceToGen(svc))
	if err != nil {
		return fmt.Errorf("CreateConsumedRestService: encode: %w", err)
	}
	return b.writer.InsertUnit(string(svc.ID), string(svc.ContainerID), "Documents", "Rest$ConsumedRestService", contents)
}

// UpdateConsumedRestService rewrites an existing consumed REST service in place.
func (b *Backend) UpdateConsumedRestService(svc *model.ConsumedRestService) error {
	if svc == nil {
		return fmt.Errorf("UpdateConsumedRestService: nil service")
	}
	if b.writer == nil {
		return fmt.Errorf("UpdateConsumedRestService: not connected for writing")
	}
	contents, err := (&codec.Encoder{}).Encode(consumedRestServiceToGen(svc))
	if err != nil {
		return fmt.Errorf("UpdateConsumedRestService: encode: %w", err)
	}
	return b.writer.UpdateRawUnit(string(svc.ID), contents)
}

// DeleteConsumedRestService removes a consumed REST service unit by ID.
func (b *Backend) DeleteConsumedRestService(id model.ID) error {
	if b.writer == nil {
		return fmt.Errorf("DeleteConsumedRestService: not connected for writing")
	}
	return b.writer.DeleteUnit(string(id))
}

func consumedRestServiceToGen(svc *model.ConsumedRestService) element.Element {
	g := newElem("Rest$ConsumedRestService", string(svc.ID))
	addStr(g, "Name", svc.Name)
	addStr(g, "Documentation", svc.Documentation)
	addBool(g, "Excluded", svc.Excluded)
	addStr(g, "ExportLevel", "Hidden")
	// BaseUrlParameter: null (via NullFields).
	if svc.OpenApiContent != "" {
		f := newElem("Rest$OpenApiFile", "")
		addStr(f, "Content", svc.OpenApiContent)
		addPart(g, "OpenApiFile", f)
	}
	addPart(g, "BaseUrl", valueTemplateElem(svc.BaseUrl))
	// AuthenticationScheme: null (via NullFields) when absent.
	if svc.Authentication != nil {
		addPart(g, "AuthenticationScheme", restAuthSchemeToGen(svc.Authentication))
	}
	ops := make([]element.Element, 0, len(svc.Operations))
	for _, op := range svc.Operations {
		ops = append(ops, restOperationToGen(op))
	}
	if len(ops) > 0 {
		addPartList(g, "Operations", ops)
	}
	return g
}

// valueTemplateElem builds a Rest$ValueTemplate (BaseUrl / Path / header value).
func valueTemplateElem(value string) element.Element {
	g := newElem("Rest$ValueTemplate", "")
	addStr(g, "Value", value)
	return g
}

func restAuthSchemeToGen(auth *model.RestAuthentication) element.Element {
	g := newElem("Rest$BasicAuthenticationScheme", "")
	addPart(g, "Username", restValueElem(auth.Username))
	addPart(g, "Password", restValueElem(auth.Password))
	return g
}

// restValueElem builds a polymorphic Rest$Value: a "$"-prefixed value is a
// constant reference (Rest$ConstantValue), otherwise a string literal.
func restValueElem(value string) element.Element {
	if strings.HasPrefix(value, "$") {
		g := newElem("Rest$ConstantValue", "")
		addStr(g, "Value", strings.TrimPrefix(value, "$"))
		return g
	}
	g := newElem("Rest$StringValue", "")
	addStr(g, "Value", value)
	return g
}

func restOperationToGen(op *model.RestClientOperation) element.Element {
	g := newElem("Rest$RestOperation", "")
	addStr(g, "Name", op.Name)
	timeout := int64(op.Timeout)
	if timeout <= 0 {
		timeout = 300
	}
	addInt64(g, "Timeout", timeout)
	if len(op.Tags) > 0 {
		addByNameRefList(g, "Tags", "", op.Tags)
	}
	addPart(g, "Method", restMethodToGen(op))
	addPart(g, "Path", valueTemplateElem(op.Path))

	headers := make([]element.Element, 0, len(op.Headers)+1)
	hasAccept := false
	for _, h := range op.Headers {
		headers = append(headers, restHeaderToGen(h))
		if strings.EqualFold(h.Name, "Accept") {
			hasAccept = true
		}
	}
	// Mendix requires an Accept header on every consumed REST operation (CE7062).
	if !hasAccept {
		headers = append(headers, restHeaderToGen(&model.RestClientHeader{Name: "Accept", Value: "*/*"}))
	}
	addPartList(g, "Headers", headers)

	params := make([]element.Element, 0, len(op.Parameters))
	for _, p := range op.Parameters {
		pe := newElem("Rest$OperationParameter", "")
		addStr(pe, "Name", p.Name)
		addPart(pe, "DataType", restDataTypeElem(p.DataType))
		params = append(params, pe)
	}
	if len(params) > 0 {
		addPartList(g, "Parameters", params)
	}

	queryParams := make([]element.Element, 0, len(op.QueryParameters))
	for _, q := range op.QueryParameters {
		qe := newElem("Rest$QueryParameter", "")
		addStr(qe, "Name", q.Name)
		addPart(qe, "ParameterUsage", newElem("Rest$RequiredQueryParameterUsage", ""))
		queryParams = append(queryParams, qe)
	}
	if len(queryParams) > 0 {
		addPartList(g, "QueryParameters", queryParams)
	}

	if op.ResponseType == "MAPPING" && op.ResponseEntity != "" && len(op.ResponseMappings) > 0 {
		addPart(g, "ResponseHandling", restImplicitMappingResponseToGen(op.ResponseEntity, op.ResponseMappings))
	} else {
		addPart(g, "ResponseHandling", restResponseHandlingToGen(op.ResponseType))
	}
	return g
}

// restMethodToGen builds the polymorphic Method field. POST/PUT/PATCH always
// carry a body (CE7064); GET/DELETE/etc. use the without-body form.
func restMethodToGen(op *model.RestClientOperation) element.Element {
	httpMethod := httpMethodToMendix(op.HttpMethod)
	upper := strings.ToUpper(op.HttpMethod)
	withBody := op.BodyType != "" || upper == "POST" || upper == "PUT" || upper == "PATCH"
	if !withBody {
		g := newElem("Rest$RestOperationMethodWithoutBody", "")
		addStr(g, "HttpMethod", httpMethod)
		return g
	}
	g := newElem("Rest$RestOperationMethodWithBody", "")
	addStr(g, "HttpMethod", httpMethod)
	if op.BodyType == "EXPORT_MAPPING" && len(op.BodyMappings) > 0 {
		addPart(g, "Body", restImplicitMappingBodyToGen(op.BodyVariable, op.BodyMappings))
	} else {
		bodyType := op.BodyType
		if bodyType == "" {
			bodyType = "JSON"
		}
		addPart(g, "Body", restBodyToGen(bodyType, op.BodyVariable))
	}
	return g
}

func restBodyToGen(bodyType, bodyExpr string) element.Element {
	switch strings.ToUpper(bodyType) {
	case "FILE", "TEMPLATE":
		g := newElem("Rest$StringBody", "")
		addPart(g, "ValueTemplate", valueTemplateElem(bodyExpr))
		return g
	default: // JSON
		g := newElem("Rest$JsonBody", "")
		addStr(g, "Value", bodyExpr)
		return g
	}
}

func restHeaderToGen(h *model.RestClientHeader) element.Element {
	g := newElem("Rest$HeaderWithValueTemplate", "")
	addStr(g, "Name", h.Name)
	addPart(g, "Value", valueTemplateElem(h.Value))
	return g
}

// restResponseHandlingToGen builds a Rest$NoResponseHandling with the ContentType
// matching the declared response type (so a round-trip preserves it).
func restResponseHandlingToGen(responseType string) element.Element {
	g := newElem("Rest$NoResponseHandling", "")
	switch strings.ToUpper(responseType) {
	case "JSON":
		addStr(g, "ContentType", "application/json")
	case "STRING":
		addStr(g, "ContentType", "text/plain")
	case "FILE":
		addStr(g, "ContentType", "application/octet-stream")
	}
	return g
}

func restDataTypeElem(typeName string) element.Element {
	bsonType := "DataTypes$StringType"
	switch typeName {
	case "Integer", "Long":
		bsonType = "DataTypes$IntegerType"
	case "Decimal":
		bsonType = "DataTypes$DecimalType"
	case "Boolean":
		bsonType = "DataTypes$BooleanType"
	}
	return newElem(bsonType, "")
}

// ---------------------------------------------------------------------------
// Implicit mapping trees (Body: MAPPING / Response: MAPPING) — mirror the legacy
// inline Export/Import mapping element construction.
// ---------------------------------------------------------------------------

func restImplicitMappingBodyToGen(entity string, mappings []*model.RestResponseMapping) element.Element {
	g := newElem("Rest$ImplicitMappingBody", "")
	addPart(g, "RootMappingElement", restInlineMappingElementToGen(entity, "", "", "(Object)", mappings, "ExportMappings", "Parameter"))
	tv := newElem("Rest$StringValue", "")
	addStr(tv, "Value", "")
	addPart(g, "TestValue", tv)
	return g
}

func restImplicitMappingResponseToGen(entity string, mappings []*model.RestResponseMapping) element.Element {
	g := newElem("Rest$ImplicitMappingResponseHandling", "")
	addStr(g, "ContentType", "application/json")
	addPart(g, "RootMappingElement", restInlineMappingElementToGen(entity, "", "", "(Object)", mappings, "ImportMappings", "Create"))
	addInt32(g, "StatusCode", 200)
	return g
}

func restInlineMappingElementToGen(entity, association, exposedName, jsonPath string, mappings []*model.RestResponseMapping, namespace, objectHandling string) element.Element {
	children := make([]element.Element, 0, len(mappings))
	for _, m := range mappings {
		if m.Entity != "" {
			childJSONPath := jsonPath + "|" + m.ExposedName
			children = append(children, restInlineMappingElementToGen(m.Entity, m.Association, m.ExposedName, childJSONPath, m.Children, namespace, "Create"))
			continue
		}
		valueJSONPath := m.JsonPath
		if valueJSONPath == "" {
			valueJSONPath = jsonPath + "|" + m.ExposedName
		}
		v := newElem(namespace+"$ValueMappingElement", "")
		addStr(v, "Attribute", entity+"."+m.Attribute)
		addStr(v, "ExposedName", m.ExposedName)
		addStr(v, "JsonPath", valueJSONPath)
		addStr(v, "XmlPath", "")
		addBool(v, "IsKey", false)
		addPart(v, "Type", newElem("DataTypes$StringType", ""))
		addInt32(v, "MinOccurs", 0)
		addInt32(v, "MaxOccurs", 1)
		addBool(v, "Nillable", true)
		addBool(v, "IsDefaultType", false)
		addStr(v, "ElementType", "Value")
		addStr(v, "Documentation", "")
		addStr(v, "Converter", "")
		addInt32(v, "FractionDigits", -1)
		addInt32(v, "TotalDigits", -1)
		addInt32(v, "MaxLength", 0)
		addBool(v, "IsContent", false)
		addBool(v, "IsXmlAttribute", false)
		addStr(v, "OriginalValue", "")
		addStr(v, "XmlPrimitiveType", "String")
		children = append(children, v)
	}

	minOccurs := int32(1)
	if association != "" {
		minOccurs = 0
	}
	g := newElem(namespace+"$ObjectMappingElement", "")
	addStr(g, "Entity", entity)
	addStr(g, "ExposedName", exposedName)
	addStr(g, "JsonPath", jsonPath)
	addStr(g, "XmlPath", "")
	addStr(g, "ObjectHandling", objectHandling)
	addStr(g, "ObjectHandlingBackup", "Create")
	addBool(g, "ObjectHandlingBackupAllowOverride", false)
	addStr(g, "Association", association)
	if len(children) > 0 {
		addPartList(g, "Children", children)
	}
	addInt32(g, "MinOccurs", minOccurs)
	addInt32(g, "MaxOccurs", 1)
	addBool(g, "Nillable", true)
	addBool(g, "IsDefaultType", false)
	addStr(g, "ElementType", "Object")
	addStr(g, "Documentation", "")
	// CustomHandlerCall: null (via the ObjectMappingElement TypeDefaults from
	// mapping_write.go init()).
	return g
}
