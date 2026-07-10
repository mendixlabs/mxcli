// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"strings"

	"github.com/JordtenBulte-OLC/mxcli/model"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// ============================================================================
// Consumed OData Service — Rest$ConsumedODataService
// ============================================================================

// SerializeConsumedODataService returns BSON bytes for a consumed OData service unit.
func SerializeConsumedODataService(svc *model.ConsumedODataService) ([]byte, error) {
	doc := bson.D{
		{Key: "$ID", Value: idToBsonBinary(string(svc.ID))},
		{Key: "$Type", Value: "Rest$ConsumedODataService"},
		{Key: "Name", Value: svc.Name},
		{Key: "Documentation", Value: svc.Documentation},
		{Key: "Version", Value: svc.Version},
		{Key: "ServiceName", Value: svc.ServiceName},
		{Key: "ODataVersion", Value: svc.ODataVersion},
		{Key: "MetadataUrl", Value: svc.MetadataUrl},
		{Key: "TimeoutExpression", Value: svc.TimeoutExpression},
		{Key: "ProxyType", Value: svc.ProxyType},
		{Key: "Description", Value: svc.Description},
		{Key: "Validated", Value: svc.Validated},
		{Key: "Excluded", Value: svc.Excluded},
		{Key: "ExportLevel", Value: "Hidden"},
		{Key: "Metadata", Value: svc.Metadata},
		{Key: "MetadataHash", Value: svc.MetadataHash},
		{Key: "MetadataReferences", Value: bson.A{int32(0)}},
		{Key: "ValidatedEntities", Value: bson.A{int32(0)}},
		{Key: "LastUpdated", Value: ""},
		{Key: "UseQuerySegment", Value: false},
		{Key: "MinimumMxVersion", Value: ""},
		{Key: "RecommendedMxVersion", Value: ""},
	}

	if svc.ConfigurationMicroflow != "" {
		doc = append(doc, bson.E{Key: "ConfigurationMicroflow", Value: svc.ConfigurationMicroflow})
	}
	if svc.ErrorHandlingMicroflow != "" {
		doc = append(doc, bson.E{Key: "ErrorHandlingMicroflow", Value: svc.ErrorHandlingMicroflow})
	}
	if svc.ProxyHost != "" {
		doc = append(doc, bson.E{Key: "ProxyHost", Value: svc.ProxyHost})
	}
	if svc.ProxyPort != "" {
		doc = append(doc, bson.E{Key: "ProxyPort", Value: svc.ProxyPort})
	}
	if svc.ProxyUsername != "" {
		doc = append(doc, bson.E{Key: "ProxyUsername", Value: svc.ProxyUsername})
	}
	if svc.ProxyPassword != "" {
		doc = append(doc, bson.E{Key: "ProxyPassword", Value: svc.ProxyPassword})
	}
	if svc.ApplicationId != "" {
		doc = append(doc, bson.E{Key: "ApplicationId", Value: svc.ApplicationId})
	}
	if svc.EndpointId != "" {
		doc = append(doc, bson.E{Key: "EndpointId", Value: svc.EndpointId})
	}
	if svc.CatalogUrl != "" {
		doc = append(doc, bson.E{Key: "CatalogUrl", Value: svc.CatalogUrl})
	}
	if svc.EnvironmentType != "" {
		doc = append(doc, bson.E{Key: "EnvironmentType", Value: svc.EnvironmentType})
	}

	doc = append(doc, bson.E{Key: "HttpConfiguration", Value: serWebHttpConfiguration(svc.HttpConfiguration)})

	return bson.Marshal(doc)
}

// serWebHttpConfiguration converts an HttpConfiguration to a BSON map.
func serWebHttpConfiguration(cfg *model.HttpConfiguration) bson.D {
	cfgID := generateUUID()
	if cfg != nil && cfg.ID != "" {
		cfgID = string(cfg.ID)
	}

	useHTTPAuthentication := false
	httpAuthenticationUserName := ""
	httpAuthenticationPassword := ""
	httpMethod := "Post"
	overrideLocation := false
	customLocation := ""
	clientCertificate := ""
	var headerEntries bson.A

	if cfg != nil {
		useHTTPAuthentication = cfg.UseAuthentication
		httpAuthenticationUserName = cfg.Username
		httpAuthenticationPassword = cfg.Password
		if cfg.HttpMethod != "" {
			httpMethod = cfg.HttpMethod
		}
		overrideLocation = cfg.OverrideLocation
		customLocation = cfg.CustomLocation
		clientCertificate = cfg.ClientCertificate

		if len(cfg.HeaderEntries) > 0 {
			headers := bson.A{int32(3)}
			for _, h := range cfg.HeaderEntries {
				hID := string(h.ID)
				if hID == "" {
					hID = generateUUID()
				}
				headers = append(headers, bson.D{
					{Key: "$ID", Value: idToBsonBinary(hID)},
					{Key: "$Type", Value: "Microflows$HttpHeaderEntry"},
					{Key: "Key", Value: h.Key},
					{Key: "Value", Value: h.Value},
				})
			}
			headerEntries = headers
		}
	}

	doc := bson.D{
		{Key: "$ID", Value: idToBsonBinary(cfgID)},
		{Key: "$Type", Value: "Microflows$HttpConfiguration"},
		{Key: "UseHttpAuthentication", Value: useHTTPAuthentication},
		{Key: "HttpAuthenticationUserName", Value: httpAuthenticationUserName},
		{Key: "HttpAuthenticationPassword", Value: httpAuthenticationPassword},
		{Key: "HttpMethod", Value: httpMethod},
		{Key: "OverrideLocation", Value: overrideLocation},
		{Key: "CustomLocation", Value: customLocation},
		{Key: "ClientCertificate", Value: clientCertificate},
	}

	if headerEntries != nil {
		doc = append(doc, bson.E{Key: "HttpHeaderEntries", Value: headerEntries})
	}

	return doc
}

// ============================================================================
// Published OData Service — ODataPublish$PublishedODataService2
// ============================================================================

// SerializePublishedODataService returns BSON bytes for a published OData service unit.
func SerializePublishedODataService(svc *model.PublishedODataService) ([]byte, error) {
	// Authentication types array (versioned: starts with int32(3))
	authTypes := bson.A{int32(3)}
	for _, at := range svc.AuthenticationTypes {
		authTypes = append(authTypes, at)
	}

	// Serialize entity types and build ID map for entity set pointers
	entityTypeIDMap := make(map[string]string)
	entityTypes := bson.A{}
	for _, et := range svc.EntityTypes {
		etID := string(et.ID)
		if etID == "" {
			etID = generateUUID()
			et.ID = model.ID(etID)
		}
		entityTypeIDMap[et.ExposedName] = etID
		entityTypes = append(entityTypes, serWebPublishedEntityType(et))
	}

	entitySets := bson.A{}
	for _, es := range svc.EntitySets {
		esID := string(es.ID)
		if esID == "" {
			esID = generateUUID()
			es.ID = model.ID(esID)
		}
		entityTypeID := entityTypeIDMap[es.EntityTypeName]
		entitySets = append(entitySets, serWebPublishedEntitySet(es, entityTypeID))
	}

	doc := bson.D{
		{Key: "$ID", Value: idToBsonBinary(string(svc.ID))},
		{Key: "$Type", Value: "ODataPublish$PublishedODataService2"},
		{Key: "Name", Value: svc.Name},
		{Key: "Documentation", Value: svc.Documentation},
		{Key: "Path", Value: svc.Path},
		{Key: "Namespace", Value: svc.Namespace},
		{Key: "ServiceName", Value: svc.ServiceName},
		{Key: "Version", Value: svc.Version},
		{Key: "ODataVersion", Value: svc.ODataVersion},
		{Key: "Summary", Value: svc.Summary},
		{Key: "Description", Value: svc.Description},
		{Key: "PublishAssociations", Value: svc.PublishAssociations},
		{Key: "UseGeneralization", Value: svc.UseGeneralization},
		{Key: "AuthenticationMicroflow", Value: svc.AuthMicroflow},
		{Key: "AuthenticationTypes", Value: authTypes},
		{Key: "EntityTypes", Value: entityTypes},
		{Key: "EntitySets", Value: entitySets},
		{Key: "Excluded", Value: svc.Excluded},
	}
	return bson.Marshal(doc)
}

func serWebPublishedEntityType(et *model.PublishedEntityType) bson.D {
	members := bson.A{}
	for _, m := range et.Members {
		members = append(members, serWebPublishedMember(m))
	}
	return bson.D{
		{Key: "$ID", Value: idToBsonBinary(string(et.ID))},
		{Key: "$Type", Value: "ODataPublish$EntityType"},
		{Key: "Entity", Value: et.Entity},
		{Key: "ExposedName", Value: et.ExposedName},
		{Key: "Summary", Value: et.Summary},
		{Key: "Description", Value: et.Description},
		{Key: "ChildMembers", Value: members},
	}
}

func serWebPublishedEntitySet(es *model.PublishedEntitySet, entityTypeID string) bson.D {
	doc := bson.D{
		{Key: "$ID", Value: idToBsonBinary(string(es.ID))},
		{Key: "$Type", Value: "ODataPublish$EntitySet"},
		{Key: "ExposedName", Value: es.ExposedName},
		{Key: "UsePaging", Value: es.UsePaging},
		{Key: "PageSize", Value: int64(es.PageSize)},
	}

	if entityTypeID != "" {
		doc = append(doc, bson.E{Key: "EntityTypePointer", Value: idToBsonBinary(entityTypeID)})
	}
	if es.ReadMode != "" {
		doc = append(doc, bson.E{Key: "ReadMode", Value: serWebReadMode(es.ReadMode)})
	}
	if es.InsertMode != "" {
		doc = append(doc, bson.E{Key: "InsertMode", Value: serWebChangeMode(es.InsertMode)})
	}
	if es.UpdateMode != "" {
		doc = append(doc, bson.E{Key: "UpdateMode", Value: serWebChangeMode(es.UpdateMode)})
	}
	if es.DeleteMode != "" {
		doc = append(doc, bson.E{Key: "DeleteMode", Value: serWebChangeMode(es.DeleteMode)})
	}
	return doc
}

func serWebPublishedMember(m *model.PublishedMember) bson.D {
	memberID := string(m.ID)
	if memberID == "" {
		memberID = generateUUID()
	}

	doc := bson.D{
		{Key: "$ID", Value: idToBsonBinary(memberID)},
		{Key: "ExposedName", Value: m.ExposedName},
		{Key: "Filterable", Value: m.Filterable},
		{Key: "Sortable", Value: m.Sortable},
		{Key: "IsPartOfKey", Value: m.IsPartOfKey},
	}

	switch m.Kind {
	case "attribute":
		doc = append(doc, bson.E{Key: "$Type", Value: "ODataPublish$PublishedAttribute"})
		doc = append(doc, bson.E{Key: "Attribute", Value: m.Name})
	case "association":
		doc = append(doc, bson.E{Key: "$Type", Value: "ODataPublish$PublishedAssociationEnd"})
		doc = append(doc, bson.E{Key: "Association", Value: m.Name})
	case "id":
		doc = append(doc, bson.E{Key: "$Type", Value: "ODataPublish$PublishedId"})
		doc = append(doc, bson.E{Key: "Attribute", Value: m.Name})
	default:
		doc = append(doc, bson.E{Key: "$Type", Value: "ODataPublish$PublishedAttribute"})
		doc = append(doc, bson.E{Key: "Attribute", Value: m.Name})
	}
	return doc
}

func serWebReadMode(mode string) bson.D {
	modeID := idToBsonBinary(generateUUID())
	switch {
	case strings.EqualFold(mode, "ReadFromDatabase") || strings.EqualFold(mode, "SOURCE"):
		return bson.D{{Key: "$ID", Value: modeID}, {Key: "$Type", Value: "ODataPublish$ReadSource"}}
	case strings.HasPrefix(mode, "CallMicroflow:"):
		return bson.D{{Key: "$ID", Value: modeID}, {Key: "$Type", Value: "ODataPublish$CallMicroflowToRead"}, {Key: "Microflow", Value: strings.TrimPrefix(mode, "CallMicroflow:")}}
	case strings.HasPrefix(mode, "MICROFLOW "):
		return bson.D{{Key: "$ID", Value: modeID}, {Key: "$Type", Value: "ODataPublish$CallMicroflowToRead"}, {Key: "Microflow", Value: strings.TrimPrefix(mode, "MICROFLOW ")}}
	default:
		return bson.D{{Key: "$ID", Value: modeID}, {Key: "$Type", Value: "ODataPublish$ReadSource"}}
	}
}

func serWebChangeMode(mode string) bson.D {
	modeID := idToBsonBinary(generateUUID())
	switch {
	case strings.EqualFold(mode, "ChangeFromDatabase") || strings.EqualFold(mode, "SOURCE"):
		return bson.D{{Key: "$ID", Value: modeID}, {Key: "$Type", Value: "ODataPublish$ChangeSource"}}
	case strings.EqualFold(mode, "NotSupported") || strings.EqualFold(mode, "NOT_SUPPORTED"):
		return bson.D{{Key: "$ID", Value: modeID}, {Key: "$Type", Value: "ODataPublish$ChangeNotSupported"}}
	case strings.HasPrefix(mode, "CallMicroflow:"):
		return bson.D{{Key: "$ID", Value: modeID}, {Key: "$Type", Value: "ODataPublish$CallMicroflowToChange"}, {Key: "Microflow", Value: strings.TrimPrefix(mode, "CallMicroflow:")}}
	case strings.HasPrefix(mode, "MICROFLOW "):
		return bson.D{{Key: "$ID", Value: modeID}, {Key: "$Type", Value: "ODataPublish$CallMicroflowToChange"}, {Key: "Microflow", Value: strings.TrimPrefix(mode, "MICROFLOW ")}}
	default:
		return bson.D{{Key: "$ID", Value: modeID}, {Key: "$Type", Value: "ODataPublish$ChangeNotSupported"}}
	}
}

// ============================================================================
// Consumed REST Service — Rest$ConsumedRestService
// ============================================================================

// SerializeConsumedRestService returns BSON bytes for a consumed REST service unit.
func SerializeConsumedRestService(svc *model.ConsumedRestService) ([]byte, error) {
	doc := bson.D{
		{Key: "$ID", Value: idToBsonBinary(string(svc.ID))},
		{Key: "$Type", Value: "Rest$ConsumedRestService"},
		{Key: "Name", Value: svc.Name},
		{Key: "Documentation", Value: svc.Documentation},
		{Key: "Excluded", Value: svc.Excluded},
		{Key: "ExportLevel", Value: "Hidden"},
		{Key: "BaseUrlParameter", Value: nil},
	}

	if svc.OpenApiContent != "" {
		doc = append(doc, bson.E{Key: "OpenApiFile", Value: bson.D{
			{Key: "$ID", Value: idToBsonBinary(generateUUID())},
			{Key: "$Type", Value: "Rest$OpenApiFile"},
			{Key: "Content", Value: svc.OpenApiContent},
		}})
	}

	doc = append(doc, bson.E{Key: "BaseUrl", Value: serWebValueTemplate(svc.BaseUrl)})

	if svc.Authentication == nil {
		doc = append(doc, bson.E{Key: "AuthenticationScheme", Value: nil})
	} else {
		doc = append(doc, bson.E{Key: "AuthenticationScheme", Value: serWebRestAuthScheme(svc.Authentication)})
	}

	ops := bson.A{int32(2)}
	for _, op := range svc.Operations {
		ops = append(ops, serWebRestOperation(op))
	}
	doc = append(doc, bson.E{Key: "Operations", Value: ops})

	return bson.Marshal(doc)
}

func serWebValueTemplate(value string) bson.D {
	return bson.D{
		{Key: "$ID", Value: idToBsonBinary(generateUUID())},
		{Key: "$Type", Value: "Rest$ValueTemplate"},
		{Key: "Value", Value: value},
	}
}

func serWebRestAuthScheme(auth *model.RestAuthentication) bson.D {
	doc := bson.D{
		{Key: "$ID", Value: idToBsonBinary(generateUUID())},
		{Key: "$Type", Value: "Rest$BasicAuthenticationScheme"},
	}
	doc = append(doc, bson.E{Key: "Username", Value: serWebRestValue(auth.Username)})
	doc = append(doc, bson.E{Key: "Password", Value: serWebRestValue(auth.Password)})
	return doc
}

func serWebRestValue(value string) bson.D {
	if strings.HasPrefix(value, "$") {
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

func serWebRestOperation(op *model.RestClientOperation) bson.D {
	doc := bson.D{
		{Key: "$ID", Value: idToBsonBinary(generateUUID())},
		{Key: "$Type", Value: "Rest$RestOperation"},
		{Key: "Name", Value: op.Name},
	}

	timeout := int64(op.Timeout)
	if timeout <= 0 {
		timeout = 300
	}
	doc = append(doc, bson.E{Key: "Timeout", Value: timeout})

	tags := bson.A{int32(1)}
	for _, t := range op.Tags {
		tags = append(tags, t)
	}
	doc = append(doc, bson.E{Key: "Tags", Value: tags})

	doc = append(doc, bson.E{Key: "Method", Value: serWebRestMethod(op)})
	doc = append(doc, bson.E{Key: "Path", Value: serWebValueTemplate(op.Path)})

	headers := bson.A{int32(2)}
	hasAccept := false
	for _, h := range op.Headers {
		headers = append(headers, serWebRestHeader(h))
		if strings.EqualFold(h.Name, "Accept") {
			hasAccept = true
		}
	}
	if !hasAccept {
		headers = append(headers, serWebRestHeader(&model.RestClientHeader{Name: "Accept", Value: "*/*"}))
	}
	doc = append(doc, bson.E{Key: "Headers", Value: headers})

	params := bson.A{int32(2)}
	for _, p := range op.Parameters {
		params = append(params, serWebRestParameter(p))
	}
	doc = append(doc, bson.E{Key: "Parameters", Value: params})

	queryParams := bson.A{int32(2)}
	for _, q := range op.QueryParameters {
		queryParams = append(queryParams, serWebRestQueryParameter(q))
	}
	doc = append(doc, bson.E{Key: "QueryParameters", Value: queryParams})

	if op.ResponseType == "MAPPING" && op.ResponseEntity != "" && len(op.ResponseMappings) > 0 {
		doc = append(doc, bson.E{Key: "ResponseHandling", Value: serWebRestImplicitMappingResponse(op.ResponseEntity, op.ResponseMappings)})
	} else {
		doc = append(doc, bson.E{Key: "ResponseHandling", Value: serWebRestResponseHandling(op.ResponseType)})
	}

	return doc
}

func serWebRestMethod(op *model.RestClientOperation) bson.D {
	httpMethod := serWebHttpMethodToMendix(op.HttpMethod)

	if op.BodyType != "" {
		bodyDoc := bson.D{
			{Key: "$ID", Value: idToBsonBinary(generateUUID())},
			{Key: "$Type", Value: "Rest$RestOperationMethodWithBody"},
			{Key: "HttpMethod", Value: httpMethod},
		}
		if op.BodyType == "EXPORT_MAPPING" && len(op.BodyMappings) > 0 {
			bodyDoc = append(bodyDoc, bson.E{Key: "Body", Value: serWebRestImplicitMappingBody(op.BodyVariable, op.BodyMappings)})
		} else {
			bodyDoc = append(bodyDoc, bson.E{Key: "Body", Value: serWebRestBody(op.BodyType, op.BodyVariable)})
		}
		return bodyDoc
	}

	methodUpper := strings.ToUpper(op.HttpMethod)
	if methodUpper == "POST" || methodUpper == "PUT" || methodUpper == "PATCH" {
		bodyDoc := bson.D{
			{Key: "$ID", Value: idToBsonBinary(generateUUID())},
			{Key: "$Type", Value: "Rest$RestOperationMethodWithBody"},
			{Key: "HttpMethod", Value: httpMethod},
		}
		bodyDoc = append(bodyDoc, bson.E{Key: "Body", Value: serWebRestBody("JSON", op.BodyVariable)})
		return bodyDoc
	}

	return bson.D{
		{Key: "$ID", Value: idToBsonBinary(generateUUID())},
		{Key: "$Type", Value: "Rest$RestOperationMethodWithoutBody"},
		{Key: "HttpMethod", Value: httpMethod},
	}
}

func serWebRestBody(bodyType, bodyExpr string) bson.D {
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
			{Key: "ValueTemplate", Value: serWebValueTemplate(bodyExpr)},
		}
	default:
		return bson.D{
			{Key: "$ID", Value: idToBsonBinary(generateUUID())},
			{Key: "$Type", Value: "Rest$JsonBody"},
			{Key: "Value", Value: bodyExpr},
		}
	}
}

func serWebRestImplicitMappingBody(entity string, mappings []*model.RestResponseMapping) bson.D {
	rootElement := serWebInlineMappingElement(entity, "", "", "(Object)", mappings, "ExportMappings", "Parameter")
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

func serWebRestImplicitMappingResponse(entity string, mappings []*model.RestResponseMapping) bson.D {
	rootElement := serWebInlineMappingElement(entity, "", "", "(Object)", mappings, "ImportMappings", "Create")
	return bson.D{
		{Key: "$ID", Value: idToBsonBinary(generateUUID())},
		{Key: "$Type", Value: "Rest$ImplicitMappingResponseHandling"},
		{Key: "ContentType", Value: "application/json"},
		{Key: "RootMappingElement", Value: rootElement},
		{Key: "StatusCode", Value: int32(200)},
	}
}

func serWebInlineMappingElement(entity, association, exposedName, jsonPath string, mappings []*model.RestResponseMapping, namespace, objectHandling string) bson.D {
	children := bson.A{int32(2)}
	for _, m := range mappings {
		if m.Entity != "" {
			childJsonPath := jsonPath + "|" + m.ExposedName
			child := serWebInlineMappingElement(m.Entity, m.Association, m.ExposedName, childJsonPath, m.Children, namespace, "Create")
			children = append(children, child)
		} else {
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

func serWebRestHeader(h *model.RestClientHeader) bson.D {
	return bson.D{
		{Key: "$ID", Value: idToBsonBinary(generateUUID())},
		{Key: "$Type", Value: "Rest$HeaderWithValueTemplate"},
		{Key: "Name", Value: h.Name},
		{Key: "Value", Value: serWebValueTemplate(h.Value)},
	}
}

func serWebRestParameter(p *model.RestClientParameter) bson.D {
	return bson.D{
		{Key: "$ID", Value: idToBsonBinary(generateUUID())},
		{Key: "$Type", Value: "Rest$OperationParameter"},
		{Key: "Name", Value: p.Name},
		{Key: "DataType", Value: serWebRestDataType(p.DataType)},
	}
}

func serWebRestQueryParameter(p *model.RestClientParameter) bson.D {
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

func serWebRestResponseHandling(responseType string) bson.D {
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

func serWebRestDataType(typeName string) bson.D {
	bsonType := "DataTypes$StringType"
	switch typeName {
	case "Integer":
		bsonType = "DataTypes$IntegerType"
	case "Long":
		bsonType = "DataTypes$IntegerType"
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

// ============================================================================
// Published REST Service — Rest$PublishedRestService
// ============================================================================

// SerializePublishedRestService returns BSON bytes for a published REST service unit.
func SerializePublishedRestService(svc *model.PublishedRestService) ([]byte, error) {
	resources := bson.A{int32(2)}
	for _, res := range svc.Resources {
		ops := bson.A{int32(2)}
		for _, op := range res.Operations {
			opDoc := bson.D{
				{Key: "$ID", Value: idToBsonBinary(GenerateID())},
				{Key: "$Type", Value: "Rest$PublishedRestServiceOperation"},
				{Key: "HttpMethod", Value: serWebHttpMethodToMendix(op.HTTPMethod)},
				{Key: "Path", Value: op.Path},
				{Key: "Microflow", Value: op.Microflow},
				{Key: "Summary", Value: op.Summary},
				{Key: "Deprecated", Value: op.Deprecated},
				{Key: "Commit", Value: "Yes"},
				{Key: "Documentation", Value: ""},
				{Key: "ExportMapping", Value: ""},
				{Key: "ImportMapping", Value: ""},
				{Key: "ObjectHandlingBackup", Value: "Create"},
				{Key: "Parameters", Value: serWebPublishedRestParams(op.Path, op.Microflow, op.Parameters)},
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
		{Key: "AllowedRoles", Value: serWebMendixStringArray(svc.AllowedRoles)},
		{Key: "AuthenticationTypes", Value: bson.A{int32(2)}},
		{Key: "AuthenticationMicroflow", Value: ""},
		{Key: "CorsConfiguration", Value: nil},
		{Key: "Parameters", Value: bson.A{int32(2)}},
		{Key: "Resources", Value: resources},
	}

	return bson.Marshal(doc)
}

// serWebPublishedRestParams builds the Parameters array for a published REST operation.
func serWebPublishedRestParams(path string, microflowQN string, _ []string) bson.A {
	params := bson.A{int32(2)}
	for _, name := range serWebExtractPathParams(path) {
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

// serWebExtractPathParams returns parameter names from {param} placeholders in a path.
func serWebExtractPathParams(path string) []string {
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

// serWebHttpMethodToMendix converts uppercase HTTP method names to Mendix casing.
func serWebHttpMethodToMendix(method string) string {
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

// serWebMendixStringArray builds a Mendix-style versioned string array (int32(1) prefix).
func serWebMendixStringArray(items []string) bson.A {
	arr := bson.A{int32(1)}
	for _, s := range items {
		arr = append(arr, s)
	}
	return arr
}
