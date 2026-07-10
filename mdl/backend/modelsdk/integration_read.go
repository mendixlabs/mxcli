// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/modelsdk/element"
	genBe "github.com/JordtenBulte-OLC/mxcli/modelsdk/gen/businessevents"
	genDb "github.com/JordtenBulte-OLC/mxcli/modelsdk/gen/databaseconnector"
	genOp "github.com/JordtenBulte-OLC/mxcli/modelsdk/gen/odatapublish"
	genRest "github.com/JordtenBulte-OLC/mxcli/modelsdk/gen/rest"
	"github.com/JordtenBulte-OLC/mxcli/modelsdk/mprread"
)

// ListConsumedODataServices reads every Rest$ConsumedODataService unit and
// converts it to the semantic model. Mirrors the legacy
// (*mpr.Reader).ListConsumedODataServices field-for-field for the top-level
// service fields (the catalog consumes Name/Version/ODataVersion/MetadataUrl/
// Validated/Metadata). HTTP-configuration and proxy detail are stored on the
// gen element as nested parts/by-name refs and are not surfaced here because
// no catalog/describe path reads them through this method.
func (b *Backend) ListConsumedODataServices() ([]*model.ConsumedODataService, error) {
	units, err := mprread.ListUnitsWithContainer[*genRest.ConsumedODataService](b.reader)
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
	units, err := mprread.ListUnitsWithContainer[*genOp.PublishedODataService2](b.reader)
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
			UseGeneralization:   g.UseGeneralization(),
			Excluded:            g.Excluded(),
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
	units, err := mprread.ListUnitsWithContainer[*genRest.ConsumedRestService](b.reader)
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
		if vt, ok := g.BaseUrl().(*genRest.ValueTemplate); ok && vt != nil {
			svc.BaseUrl = vt.Value()
		}
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
	case *genRest.RestOperationMethodWithoutBody:
		out.HttpMethod = httpMethodUpper(m.HttpMethod())
	}
	if pt, ok := op.Path().(*genRest.ValueTemplate); ok && pt != nil {
		out.Path = pt.Value()
	}
	for _, pEl := range op.ParametersItems() {
		if p, ok := pEl.(*genRest.RestParameter); ok {
			out.Parameters = append(out.Parameters, &model.RestClientParameter{Name: p.Name()})
		}
	}
	for _, qEl := range op.QueryParametersItems() {
		if q, ok := qEl.(*genRest.RestParameter); ok {
			out.QueryParameters = append(out.QueryParameters, &model.RestClientParameter{Name: q.Name()})
		}
	}
	return out
}

// restValueOf extracts a string from a polymorphic Rest$Value (StringValue or
// ConstantValue), mirroring the legacy extractRestValue.
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
	units, err := mprread.ListUnitsWithContainer[*genRest.PublishedRestService](b.reader)
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
		}
		svc.ID = model.ID(g.ID())
		svc.TypeName = "Rest$PublishedRestService"
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
					Path:       op.Path(),
					HTTPMethod: op.HttpMethod(),
					Summary:    op.Summary(),
					Microflow:  op.MicroflowQualifiedName(),
					Deprecated: op.Deprecated(),
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
	units, err := mprread.ListUnitsWithContainer[*genBe.BusinessEventService](b.reader)
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
	units, err := mprread.ListUnitsWithContainer[*genDb.DatabaseConnection](b.reader)
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
			dq := &model.DatabaseQuery{
				Name:      q.Name(),
				SQL:       q.Query(),
				QueryType: int(q.QueryType()),
			}
			dq.ID = model.ID(q.ID())
			dq.TypeName = "DatabaseConnector$DatabaseQuery"
			conn.Queries = append(conn.Queries, dq)
		}
		out = append(out, conn)
	}
	return out, nil
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
