// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"strings"

	"github.com/mendixlabs/mxcli/mdl/dbconnector"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/modelsdk/element"
	genBe "github.com/mendixlabs/mxcli/modelsdk/gen/businessevents"
	genDb "github.com/mendixlabs/mxcli/modelsdk/gen/databaseconnector"
	genDT "github.com/mendixlabs/mxcli/modelsdk/gen/datatypes"
	genExportMappings "github.com/mendixlabs/mxcli/modelsdk/gen/exportmappings"
	genImportMappings "github.com/mendixlabs/mxcli/modelsdk/gen/importmappings"
	genOp "github.com/mendixlabs/mxcli/modelsdk/gen/odatapublish"
	genRest "github.com/mendixlabs/mxcli/modelsdk/gen/rest"
)

// ListConsumedODataServices reads every Rest$ConsumedODataService unit and
// converts it to the semantic model. Mirrors the legacy
// (*mpr.Reader).ListConsumedODataServices field-for-field for the top-level
// service fields (the catalog consumes Name/Version/ODataVersion/MetadataUrl/
// Validated/Metadata). HTTP-configuration and proxy detail are stored on the
// gen element as nested parts/by-name refs and are not surfaced here because
// no catalog/describe path reads them through this method.
func (b *Backend) ListConsumedODataServices() ([]*model.ConsumedODataService, error) {
	units, err := listUnits[*genRest.ConsumedODataService](b)
	if err != nil {
		return nil, err
	}
	out := make([]*model.ConsumedODataService, 0, len(units))
	for _, u := range units {
		g := u.Element
		svc := &model.ConsumedODataService{
			ContainerID:       model.ID(u.ContainerID),
			Name:              g.Name(),
			Documentation:     g.Documentation(),
			Version:           g.Version(),
			ServiceName:       g.ServiceName(),
			ODataVersion:      g.ODataVersion(),
			MetadataUrl:       g.MetadataUrl(),
			TimeoutExpression: g.TimeoutExpression(),
			ProxyType:         g.ProxyType(),
			Description:       g.Description(),
			Validated:         g.Validated(),
			Excluded:          g.Excluded(),
			Metadata:          g.Metadata(),
			MetadataHash:      g.MetadataHash(),
			ApplicationId:     g.ApplicationId(),
			EndpointId:        g.EndpointId(),
			CatalogUrl:        g.CatalogUrl(),
			EnvironmentType:   g.EnvironmentType(),
			// The microflow storage fields were renamed/split across versions
			// (issue #728). Config slot: ConfigurationEntityMicroflow (11.10+) or
			// the pre-11.10 single slot (ConfigurationMicroflow / ancient
			// HeadersMicroflow). Headers slot is only distinct on 11.10+
			// (HeaderListMicroflow). Coalesce so DESCRIBE round-trips any version.
			ConfigurationMicroflow: firstNonEmpty(
				g.ConfigurationEntityMicroflowQualifiedName(), // >= 11.10 config
				g.ConfigurationMicroflowQualifiedName(),       // 10.12 – 11.10 (single slot)
				g.HeadersMicroflowQualifiedName(),             // < 10.12 (single slot)
			),
			HeadersMicroflow:       g.HeaderListMicroflowQualifiedName(), // >= 11.10 headers
			ErrorHandlingMicroflow: g.ErrorHandlingMicroflowQualifiedName(),
			ProxyHost:              g.ProxyHostQualifiedName(),
			ProxyPort:              g.ProxyPortQualifiedName(),
			ProxyUsername:          g.ProxyUsernameQualifiedName(),
			ProxyPassword:          g.ProxyPasswordQualifiedName(),
		}
		svc.ID = model.ID(g.ID())
		svc.TypeName = "Rest$ConsumedODataService"
		// HttpConfiguration (ServiceUrl/auth/headers) is read from the raw unit —
		// the gen accessors bind mismatched storage keys. Without it a CREATE OR
		// MODIFY drops the ServiceUrl (CE5111).
		if raw, rerr := b.GetRawUnit(svc.ID); rerr == nil {
			if hc := jsToMap(raw["HttpConfiguration"]); hc != nil {
				svc.HttpConfiguration = odataHttpConfigFromRaw(hc)
			}
		}
		out = append(out, svc)
	}
	return out, nil
}

// ListPublishedODataServices reads every ODataPublish$PublishedODataService2
// unit and converts it to the semantic model. Mirrors the legacy reader for
// the fields the catalog consumes (Name/Path/Version/ODataVersion/EntitySets/
// AuthenticationTypes/AllowedModuleRoles). The per-entity-type/entity-set
// member detail that the legacy parser builds is not surfaced here: no path
// reaching this method walks svc.EntityTypes/EntitySets members, and the gen
// element stores those as nested parts.
func (b *Backend) ListPublishedODataServices() ([]*model.PublishedODataService, error) {
	units, err := listUnits[*genOp.PublishedODataService2](b)
	if err != nil {
		return nil, err
	}
	out := make([]*model.PublishedODataService, 0, len(units))
	for _, u := range units {
		g := u.Element
		svc := &model.PublishedODataService{
			ContainerID:         model.ID(u.ContainerID),
			Name:                g.Name(),
			Documentation:       g.Documentation(),
			Path:                g.Path(),
			Namespace:           g.Namespace(),
			ServiceName:         g.ServiceName(),
			Version:             g.Version(),
			ODataVersion:        g.ODataVersion(),
			Summary:             g.Summary(),
			Description:         g.Description(),
			PublishAssociations: g.PublishAssociations(),
			SupportsGraphQL:     g.SupportsGraphQL(),
			UseGeneralization:   g.UseGeneralization(),
			Excluded:            g.Excluded(),
			ExportLevel:         g.ExportLevel(),
			AuthMicroflow:       g.AuthenticationMicroflowQualifiedName(),
			AuthenticationTypes: append([]string(nil), g.AuthenticationTypesItems()...),
			AllowedModuleRoles:  append([]string(nil), g.AllowedModuleRolesQualifiedNames()...),
		}
		svc.ID = model.ID(g.ID())
		svc.TypeName = "ODataPublish$PublishedODataService2"
		// EntityTypes/EntitySets (with their members + modes) are read from the
		// raw unit; the gen surfaces only counts. Without the full tree an ALTER
		// rewrites the service stripped of its entity sets, which crashes Studio
		// Pro on load (NullReferenceException).
		if raw, rerr := b.GetRawUnit(svc.ID); rerr == nil {
			svc.EntityTypes, svc.EntitySets = publishedEntityTreeFromRaw(raw)
			svc.Microflows = publishedMicroflowsFromRaw(raw)
		}
		out = append(out, svc)
	}
	return out, nil
}

// ListConsumedRestServices reads every Rest$ConsumedRestService unit and
// converts it to the semantic model, mirroring the legacy reader for the
// fields the catalog/REST builder consumes (Name, BaseUrl, Authentication
// scheme, Operations with Name/HttpMethod/Path/Timeout/ResponseType/BodyType
// and parameter counts).
func (b *Backend) ListConsumedRestServices() ([]*model.ConsumedRestService, error) {
	units, err := listUnits[*genRest.ConsumedRestService](b)
	if err != nil {
		return nil, err
	}
	out := make([]*model.ConsumedRestService, 0, len(units))
	for _, u := range units {
		g := u.Element
		svc := &model.ConsumedRestService{
			ContainerID:   model.ID(u.ContainerID),
			Name:          g.Name(),
			Documentation: g.Documentation(),
			Excluded:      g.Excluded(),
		}
		svc.ID = model.ID(g.ID())
		svc.TypeName = "Rest$ConsumedRestService"
		svc.BaseUrl, svc.BaseUrlConstant = restBaseURL(g.BaseUrl())
		if auth, ok := g.AuthenticationScheme().(*genRest.BasicAuthenticationScheme); ok && auth != nil {
			a := &model.RestAuthentication{Scheme: "Basic"}
			a.Username = restValueOf(auth.Username())
			a.Password = restValueOf(auth.Password())
			svc.Authentication = a
		}
		for _, opEl := range g.OperationsItems() {
			op, ok := opEl.(*genRest.RestOperation)
			if !ok {
				continue
			}
			svc.Operations = append(svc.Operations, restOperationFromGen(op))
		}
		out = append(out, svc)
	}
	return out, nil
}

// restOperationFromGen converts a gen RestOperation to the semantic client op.
func restOperationFromGen(op *genRest.RestOperation) *model.RestClientOperation {
	out := &model.RestClientOperation{
		Name:    op.Name(),
		Timeout: int(op.Timeout()),
	}
	switch m := op.Method().(type) {
	case *genRest.RestOperationMethodWithBody:
		out.HttpMethod = httpMethodUpper(m.HttpMethod())
		restBodyFromGen(m.Body(), out)
	case *genRest.RestOperationMethodWithoutBody:
		out.HttpMethod = httpMethodUpper(m.HttpMethod())
	}
	if pt, ok := op.Path().(*genRest.ValueTemplate); ok && pt != nil {
		out.Path = pt.Value()
	}
	// Path and query parameters are *different* gen types — Rest$OperationParameter
	// and Rest$QueryParameter, which is what the writer emits. Asserting
	// Rest$RestParameter (a third type, used by REST call activities) matched
	// neither, so both lists read back empty and `describe rest client` printed
	// no Parameters/Query line at all (#843).
	for _, pEl := range op.ParametersItems() {
		if p, ok := pEl.(*genRest.OperationParameter); ok {
			out.Parameters = append(out.Parameters, &model.RestClientParameter{
				Name:     p.Name(),
				DataType: restDataTypeName(p.DataType()),
			})
		}
	}
	for _, qEl := range op.QueryParametersItems() {
		if q, ok := qEl.(*genRest.QueryParameter); ok {
			// Rest$QueryParameter carries no DataType — Mendix does not model one
			// for query parameters, so there is nothing to read back here.
			out.QueryParameters = append(out.QueryParameters, &model.RestClientParameter{Name: q.Name()})
		}
	}
	for _, hEl := range op.HeadersItems() {
		h, ok := hEl.(*genRest.HeaderWithValueTemplate)
		if !ok {
			continue
		}
		header := &model.RestClientHeader{Name: h.Name()}
		if vt, ok := h.Value().(*genRest.ValueTemplate); ok && vt != nil {
			header.Value = vt.Value()
		}
		out.Headers = append(out.Headers, header)
	}
	restResponseFromGen(op.ResponseHandling(), out)
	return out
}

// restResponseFromGen fills in the response half of the operation, mirroring the
// legacy parseRestOperation. Without it `describe rest client` reported every
// operation as `Response: none` regardless of what was stored (#843).
func restResponseFromGen(handling element.Element, out *model.RestClientOperation) {
	switch r := handling.(type) {
	case *genRest.NoResponseHandling:
		// The writer encodes the declared response type in ContentType so a
		// no-handling response still round-trips as json/string/file.
		switch r.ContentType() {
		case "application/json":
			out.ResponseType = "JSON"
		case "text/plain":
			out.ResponseType = "STRING"
		case "application/octet-stream":
			out.ResponseType = "FILE"
		default:
			out.ResponseType = "NONE"
		}
	case *genRest.ImplicitMappingResponseHandling:
		out.ResponseType = "MAPPING"
		if root, ok := r.RootMappingElement().(*genImportMappings.ImportObjectMappingElement); ok && root != nil {
			out.ResponseEntity = root.EntityQualifiedName()
			out.ResponseMappings = importMappingChildrenFromGen(root)
		}
	}
}

// restBodyFromGen fills in the body half of the operation. Same read-back gap as
// the response: a body authored as an implicit export mapping did not describe.
func restBodyFromGen(body element.Element, out *model.RestClientOperation) {
	switch b := body.(type) {
	case *genRest.JsonBody:
		out.BodyType = "JSON"
		out.BodyVariable = b.Value()
	case *genRest.StringBody:
		out.BodyType = "TEMPLATE"
		if vt, ok := b.ValueTemplate().(*genRest.ValueTemplate); ok && vt != nil {
			out.BodyVariable = vt.Value()
		}
	case *genRest.ImplicitMappingBody:
		out.BodyType = "EXPORT_MAPPING"
		if root, ok := b.RootMappingElement().(*genExportMappings.ExportObjectMappingElement); ok && root != nil {
			out.BodyVariable = root.EntityQualifiedName()
			out.BodyMappings = exportMappingChildrenFromGen(root)
		}
	}
}

// importMappingChildrenFromGen walks an ImportMappings object element tree into
// the semantic mapping entries describe re-emits as `Attr = jsonField`.
func importMappingChildrenFromGen(parent *genImportMappings.ImportObjectMappingElement) []*model.RestResponseMapping {
	entityPrefix := parent.EntityQualifiedName() + "."
	var out []*model.RestResponseMapping
	for _, childEl := range parent.ChildrenItems() {
		switch c := childEl.(type) {
		case *genImportMappings.ImportValueMappingElement:
			attr, exposed := c.AttributeQualifiedName(), c.ExposedName()
			if attr == "" || exposed == "" {
				continue
			}
			out = append(out, &model.RestResponseMapping{
				Attribute:   strings.TrimPrefix(attr, entityPrefix),
				ExposedName: exposed,
				JsonPath:    c.JsonPath(),
			})
		case *genImportMappings.ImportObjectMappingElement:
			out = append(out, &model.RestResponseMapping{
				Entity:      c.EntityQualifiedName(),
				Association: c.AssociationQualifiedName(),
				ExposedName: c.ExposedName(),
				JsonPath:    c.JsonPath(),
				Children:    importMappingChildrenFromGen(c),
			})
		}
	}
	return out
}

// exportMappingChildrenFromGen is the export-direction twin of
// importMappingChildrenFromGen (ExportMappings$* elements).
func exportMappingChildrenFromGen(parent *genExportMappings.ExportObjectMappingElement) []*model.RestResponseMapping {
	entityPrefix := parent.EntityQualifiedName() + "."
	var out []*model.RestResponseMapping
	for _, childEl := range parent.ChildrenItems() {
		switch c := childEl.(type) {
		case *genExportMappings.ExportValueMappingElement:
			attr, exposed := c.AttributeQualifiedName(), c.ExposedName()
			if attr == "" || exposed == "" {
				continue
			}
			out = append(out, &model.RestResponseMapping{
				Attribute:   strings.TrimPrefix(attr, entityPrefix),
				ExposedName: exposed,
				JsonPath:    c.JsonPath(),
			})
		case *genExportMappings.ExportObjectMappingElement:
			out = append(out, &model.RestResponseMapping{
				Entity:      c.EntityQualifiedName(),
				Association: c.AssociationQualifiedName(),
				ExposedName: c.ExposedName(),
				JsonPath:    c.JsonPath(),
				Children:    exportMappingChildrenFromGen(c),
			})
		}
	}
	return out
}

// restDataTypeName is the inverse of restDataTypeElem: DataTypes$* element back
// to the MDL type name describe prints.
func restDataTypeName(dt element.Element) string {
	if dt == nil {
		return ""
	}
	switch dt.TypeName() {
	case "DataTypes$IntegerType":
		return "Integer"
	case "DataTypes$DecimalType":
		return "Decimal"
	case "DataTypes$BooleanType":
		return "Boolean"
	case "DataTypes$StringType":
		return "String"
	}
	return ""
}

// restValueOf extracts a string from a polymorphic Rest$Value (StringValue or
// ConstantValue), mirroring the legacy extractRestValue.
// restBaseURL reads a consumed REST service's base URL: a literal
// (Rest$ValueTemplate) or the constant it is read from (Rest$ConstantValue).
// Only the literal was read, so a service configured per environment had no
// base URL at all (ako/mxcli#2426).
func restBaseURL(el element.Element) (literal, constant string) {
	switch v := el.(type) {
	case *genRest.ValueTemplate:
		if v != nil {
			return v.Value(), ""
		}
	case *genRest.ConstantValue:
		if v != nil {
			return "", v.ValueQualifiedName()
		}
	}
	return "", ""
}

func restValueOf(el element.Element) string {
	switch v := el.(type) {
	case *genRest.StringValue:
		return v.Value()
	case *genRest.ConstantValue:
		if q := v.ValueQualifiedName(); q != "" {
			return "$" + q
		}
	}
	return ""
}

// httpMethodUpper converts a Mendix HTTP method name to uppercase.
func httpMethodUpper(method string) string {
	switch method {
	case "Get":
		return "GET"
	case "Post":
		return "POST"
	case "Put":
		return "PUT"
	case "Patch":
		return "PATCH"
	case "Delete":
		return "DELETE"
	case "Head":
		return "HEAD"
	case "Options":
		return "OPTIONS"
	default:
		return method
	}
}

// ListPublishedRestServices reads every Rest$PublishedRestService unit and
// converts it to the semantic model, mirroring the legacy reader for the
// fields the catalog/strings builders consume (Name/Path/Version/ServiceName,
// Resources with Name and Operations with HTTPMethod/Path/Summary/Microflow/
// Deprecated).
func (b *Backend) ListPublishedRestServices() ([]*model.PublishedRestService, error) {
	units, err := listUnits[*genRest.PublishedRestService](b)
	if err != nil {
		return nil, err
	}
	out := make([]*model.PublishedRestService, 0, len(units))
	for _, u := range units {
		g := u.Element
		svc := &model.PublishedRestService{
			ContainerID:  model.ID(u.ContainerID),
			Name:         g.Name(),
			Path:         g.Path(),
			Version:      g.Version(),
			ServiceName:  g.ServiceName(),
			Excluded:     g.Excluded(),
			AllowedRoles: append([]string(nil), g.AllowedRolesQualifiedNames()...),
			// mendixlabs/mxcli#1510
			Documentation: g.Documentation(),

			AuthenticationTypes:     append([]string(nil), g.AuthenticationTypesItems()...),
			AuthenticationMicroflow: g.AuthenticationMicroflowQualifiedName(),
		}
		svc.ID = model.ID(g.ID())
		svc.TypeName = "Rest$PublishedRestService"
		for _, pEl := range g.ParametersItems() {
			if p, ok := pEl.(*genRest.RestOperationParameter); ok {
				param := publishedRestParameterFromGen(p)
				param.ID = model.ID(p.ID())
				svc.Parameters = append(svc.Parameters, param)
			}
		}
		for _, resEl := range g.ResourcesItems() {
			res, ok := resEl.(*genRest.PublishedRestServiceResource)
			if !ok {
				continue
			}
			resource := &model.PublishedRestResource{Name: res.Name()}
			resource.ID = model.ID(res.ID())
			resource.TypeName = "Rest$PublishedRestServiceResource"
			for _, opEl := range res.OperationsItems() {
				op, ok := opEl.(*genRest.PublishedRestServiceOperation)
				if !ok {
					continue
				}
				operation := &model.PublishedRestOperation{
					Path:                 op.Path(),
					HTTPMethod:           op.HttpMethod(),
					Summary:              op.Summary(),
					Microflow:            op.MicroflowQualifiedName(),
					Deprecated:           op.Deprecated(),
					Documentation:        op.Documentation(),
					ImportMapping:        op.ImportMappingQualifiedName(),
					ExportMapping:        op.ExportMappingQualifiedName(),
					Commit:               op.Commit(),
					ObjectHandlingBackup: op.ObjectHandlingBackup(),
				}
				for _, pEl := range op.ParametersItems() {
					if p, ok := pEl.(*genRest.RestOperationParameter); ok {
						operation.OperationParameters = append(operation.OperationParameters, publishedRestParameterFromGen(p))
					}
				}
				operation.ID = model.ID(op.ID())
				operation.TypeName = "Rest$PublishedRestServiceOperation"
				resource.Operations = append(resource.Operations, operation)
			}
			svc.Resources = append(svc.Resources, resource)
		}
		out = append(out, svc)
	}
	return out, nil
}

// ListBusinessEventServices reads every BusinessEvents$BusinessEventService
// unit and converts it to the semantic model, mirroring the legacy reader:
// top-level fields, the cached AsyncAPI Document, the Definition (ServiceName,
// EventNamePrefix, Channels with Messages/Attributes) and the
// OperationImplementations the catalog/contract builders consume.
func (b *Backend) ListBusinessEventServices() ([]*model.BusinessEventService, error) {
	units, err := listUnits[*genBe.BusinessEventService](b)
	if err != nil {
		return nil, err
	}
	out := make([]*model.BusinessEventService, 0, len(units))
	for _, u := range units {
		g := u.Element
		svc := &model.BusinessEventService{
			ContainerID:   model.ID(u.ContainerID),
			Name:          g.Name(),
			Documentation: g.Documentation(),
			Excluded:      g.Excluded(),
			ExportLevel:   g.ExportLevel(),
			Document:      g.Document(),
		}
		svc.ID = model.ID(g.ID())
		svc.TypeName = "BusinessEvents$BusinessEventService"
		if def, ok := g.Definition().(*genBe.BusinessEventDefinition); ok && def != nil {
			svc.Definition = businessEventDefinitionFromGen(def)
		}
		for _, oiEl := range g.OperationImplementationsItems() {
			op, ok := oiEl.(*genBe.ServiceOperation)
			if !ok {
				continue
			}
			so := &model.ServiceOperation{
				MessageName: op.MessageName(),
				Operation:   op.Operation(),
				Entity:      op.EntityQualifiedName(),
				Microflow:   op.MicroflowQualifiedName(),
			}
			so.ID = model.ID(op.ID())
			so.TypeName = "BusinessEvents$ServiceOperation"
			svc.OperationImplementations = append(svc.OperationImplementations, so)
		}
		out = append(out, svc)
	}
	return out, nil
}

func businessEventDefinitionFromGen(def *genBe.BusinessEventDefinition) *model.BusinessEventDefinition {
	d := &model.BusinessEventDefinition{
		ServiceName:     def.ServiceName(),
		EventNamePrefix: def.EventNamePrefix(),
		Description:     def.Description(),
		Summary:         def.Summary(),
	}
	d.ID = model.ID(def.ID())
	d.TypeName = "BusinessEvents$BusinessEventDefinition"
	for _, chEl := range def.ChannelsItems() {
		ch, ok := chEl.(*genBe.Channel)
		if !ok {
			continue
		}
		channel := &model.BusinessEventChannel{
			ChannelName: ch.ChannelName(),
			Description: ch.Description(),
		}
		channel.ID = model.ID(ch.ID())
		channel.TypeName = "BusinessEvents$Channel"
		for _, msgEl := range ch.MessagesItems() {
			msg, ok := msgEl.(*genBe.Message)
			if !ok {
				continue
			}
			message := &model.BusinessEventMessage{
				MessageName:  msg.MessageName(),
				Description:  msg.Description(),
				CanPublish:   msg.CanPublish(),
				CanSubscribe: msg.CanSubscribe(),
			}
			message.ID = model.ID(msg.ID())
			message.TypeName = "BusinessEvents$Message"
			for _, aEl := range msg.AttributesItems() {
				attr, ok := aEl.(*genBe.MessageAttribute)
				if !ok {
					continue
				}
				ma := &model.BusinessEventAttribute{
					AttributeName: attr.AttributeName(),
					Description:   attr.Description(),
					AttributeType: businessEventAttrType(attr.AttributeType()),
				}
				ma.ID = model.ID(attr.ID())
				ma.TypeName = "BusinessEvents$MessageAttribute"
				message.Attributes = append(message.Attributes, ma)
			}
			channel.Messages = append(channel.Messages, message)
		}
		d.Channels = append(d.Channels, channel)
	}
	return d
}

// businessEventAttrType maps a gen attribute-type element's $Type to the short
// type name the semantic model uses, mirroring attributeTypeFromBsonType.
func businessEventAttrType(el element.Element) string {
	if el == nil {
		return ""
	}
	switch bsonType := el.TypeName(); bsonType {
	case "DomainModels$LongAttributeType":
		return "Long"
	case "DomainModels$StringAttributeType":
		return "String"
	case "DomainModels$IntegerAttributeType":
		return "Integer"
	case "DomainModels$BooleanAttributeType":
		return "Boolean"
	case "DomainModels$DateTimeAttributeType":
		return "DateTime"
	case "DomainModels$DecimalAttributeType":
		return "Decimal"
	case "DomainModels$AutoNumberAttributeType":
		return "AutoNumber"
	case "DomainModels$BinaryAttributeType":
		return "Binary"
	default:
		return bsonType
	}
}

// ListDatabaseConnections reads every DatabaseConnector$DatabaseConnection unit
// and converts it to the semantic model, mirroring the legacy reader for
// top-level fields and the Queries (Name/SQL/QueryType) the catalog consumes.
func (b *Backend) ListDatabaseConnections() ([]*model.DatabaseConnection, error) {
	units, err := listUnits[*genDb.DatabaseConnection](b)
	if err != nil {
		return nil, err
	}
	out := make([]*model.DatabaseConnection, 0, len(units))
	for _, u := range units {
		g := u.Element
		conn := &model.DatabaseConnection{
			ContainerID:      model.ID(u.ContainerID),
			Name:             g.Name(),
			DatabaseType:     g.DatabaseType(),
			ConnectionString: g.ConnectionStringQualifiedName(),
			UserName:         g.UserNameQualifiedName(),
			Password:         g.PasswordQualifiedName(),
			Documentation:    g.Documentation(),
			Excluded:         g.Excluded(),
			ExportLevel:      g.ExportLevel(),
		}
		conn.ID = model.ID(g.ID())
		conn.TypeName = "DatabaseConnector$DatabaseConnection"
		for _, qEl := range g.QueriesItems() {
			q, ok := qEl.(*genDb.DatabaseQuery)
			if !ok {
				continue
			}
			// Mendix 11.13 replaced the integer QueryType with the `Type` string
			// enum, which the gen accessor does not know; read it off the raw
			// document so a round-trip preserves the stored value.
			dq := &model.DatabaseQuery{
				Name:          q.Name(),
				SQL:           q.Query(),
				QueryTypeName: rawQueryTypeName(q),
			}
			if dq.QueryTypeName != "" {
				dq.QueryType = dbconnector.LegacyQueryTypeFor(dq.QueryTypeName)
			} else {
				dq.QueryType = int(q.QueryType())
			}
			dq.ID = model.ID(q.ID())
			dq.TypeName = "DatabaseConnector$DatabaseQuery"
			dq.TableMappings = tableMappingsFromGen(q)
			dq.Parameters = queryParametersFromGen(q)
			conn.Queries = append(conn.Queries, dq)
		}
		out = append(out, conn)
	}
	return out, nil
}

// queryParametersFromGen reads a query's Parameters back into the semantic
// model. Sibling of tableMappingsFromGen and missed for the same reason: the
// DESCRIBE renderer emits `parameter <name>: <Type> [default …]` from this
// slice, so leaving it empty made `describe database connection` drop every
// parameter on this engine while the legacy reader showed them. A describe →
// exec round trip then produced a query with no parameters at all.
func queryParametersFromGen(q *genDb.DatabaseQuery) []*model.DatabaseQueryParameter {
	var out []*model.DatabaseQueryParameter
	for _, el := range q.ParametersItems() {
		p, ok := el.(*genDb.QueryParameter)
		if !ok {
			continue
		}
		param := &model.DatabaseQueryParameter{
			ParameterName:         p.ParameterName(),
			DefaultValue:          p.DefaultValue(),
			EmptyValueBecomesNull: p.EmptyValueBecomesNull(),
		}
		param.ID = model.ID(p.ID())
		param.TypeName = "DatabaseConnector$QueryParameter"
		// DataType is a child element whose $Type carries the type
		// (DataTypes$StringType, …) — the same string the legacy reader lifts.
		if dt := p.DataType(); dt != nil {
			param.DataType = dt.TypeName()
		}
		out = append(out, param)
	}
	return out
}

// tableMappingsFromGen reads a query's TableMappings back into the semantic
// model — the entity a query returns and its column→attribute mapping.
//
// Without this the DESCRIBE renderer's `returns`/`map (…)` block was dead code
// on this engine (it only ever saw an empty slice), so `describe database
// connection` emitted a query with no return entity and no mapping, and a
// describe → exec round trip dropped both. The legacy reader has always
// populated it, which is why the round-trip test — pinned to legacy — stayed
// green.
func tableMappingsFromGen(q *genDb.DatabaseQuery) []*model.DatabaseTableMapping {
	var out []*model.DatabaseTableMapping
	for _, el := range q.TableMappingsItems() {
		tm, ok := el.(*genDb.TableMapping)
		if !ok {
			continue
		}
		m := &model.DatabaseTableMapping{
			Entity:    tm.EntityQualifiedName(),
			TableName: tm.TableName(),
		}
		m.ID = model.ID(tm.ID())
		m.TypeName = "DatabaseConnector$TableMapping"
		for _, cEl := range tm.ColumnsItems() {
			c, ok := cEl.(*genDb.ColumnMapping)
			if !ok {
				continue
			}
			cm := &model.DatabaseColumnMapping{
				Attribute:  c.AttributeQualifiedName(),
				ColumnName: c.ColumnName(),
			}
			cm.ID = model.ID(c.ID())
			cm.TypeName = "DatabaseConnector$ColumnMapping"
			m.Columns = append(m.Columns, cm)
		}
		out = append(out, m)
	}
	return out
}

// firstNonEmpty returns the first non-empty string in vals, or "".
func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// rawQueryTypeName reads a query's Mendix 11.13+ `Type` enum member off the raw
// document. The gen accessor binds only the legacy integer QueryType, so a
// project written by 11.13 has no typed accessor for what it actually stores.
func rawQueryTypeName(q *genDb.DatabaseQuery) string {
	v, _ := q.Raw().Lookup(dbconnector.TypeKey).StringValueOK()
	return v
}

// publishedRestParameterFromGen reads one Rest$RestOperationParameter.
func publishedRestParameterFromGen(p *genRest.RestOperationParameter) *model.PublishedRestOperationParameter {
	param := &model.PublishedRestOperationParameter{
		Name:               p.Name(),
		ParameterType:      p.ParameterType(),
		MicroflowParameter: p.MicroflowParameterQualifiedName(),
		Description:        p.Description(),
	}
	if t := p.Type(); t != nil {
		param.DataType = strings.TrimSuffix(strings.TrimPrefix(t.TypeName(), "DataTypes$"), "Type")
		switch g := t.(type) {
		case *genDT.ObjectType:
			param.QualifiedName = g.EntityQualifiedName()
		case *genDT.ListType:
			param.QualifiedName = g.EntityQualifiedName()
		case *genDT.EnumerationType:
			param.QualifiedName = g.EnumerationQualifiedName()
		}
	}
	return param
}
