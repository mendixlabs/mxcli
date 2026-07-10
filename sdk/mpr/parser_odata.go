// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"fmt"

	"github.com/JordtenBulte-OLC/mxcli/model"

	"go.mongodb.org/mongo-driver/bson"
)

// parseConsumedODataService parses a consumed OData service (OData client) from BSON.
func (r *Reader) parseConsumedODataService(unitID, containerID string, contents []byte) (*model.ConsumedODataService, error) {
	contents, err := r.resolveContents(unitID, contents)
	if err != nil {
		return nil, err
	}

	var raw map[string]any
	if err := bson.Unmarshal(contents, &raw); err != nil {
		return nil, fmt.Errorf("failed to unmarshal BSON: %w", err)
	}

	svc := &model.ConsumedODataService{}
	svc.ID = model.ID(unitID)
	svc.TypeName = "Rest$ConsumedODataService"
	svc.ContainerID = model.ID(containerID)

	svc.Name = extractString(raw["Name"])
	svc.Documentation = extractString(raw["Documentation"])
	svc.Version = extractString(raw["Version"])
	svc.ServiceName = extractString(raw["ServiceName"])
	svc.ODataVersion = extractString(raw["ODataVersion"])
	svc.MetadataUrl = extractString(raw["MetadataUrl"])
	svc.TimeoutExpression = extractString(raw["TimeoutExpression"])
	svc.ProxyType = extractString(raw["ProxyType"])
	svc.Description = extractString(raw["Description"])
	svc.Validated = extractBool(raw["Validated"], false)
	svc.Excluded = extractBool(raw["Excluded"], false)

	// Microflow references (BY_NAME). The microflow storage fields were renamed/
	// split across Mendix versions (see writer_odata.go / issue #728). Config
	// slot: ConfigurationEntityMicroflow (11.10+) or the pre-11.10 single slot
	// (ConfigurationMicroflow / ancient HeadersMicroflow). Headers slot is only
	// distinct on 11.10+ (HeaderListMicroflow).
	for _, key := range []string{"ConfigurationEntityMicroflow", "ConfigurationMicroflow", "HeadersMicroflow"} {
		if v := extractString(raw[key]); v != "" {
			svc.ConfigurationMicroflow = v
			break
		}
	}
	svc.HeadersMicroflow = extractString(raw["HeaderListMicroflow"])
	svc.ErrorHandlingMicroflow = extractString(raw["ErrorHandlingMicroflow"])

	// Proxy constant references (BY_NAME)
	svc.ProxyHost = extractString(raw["ProxyHost"])
	svc.ProxyPort = extractString(raw["ProxyPort"])
	svc.ProxyUsername = extractString(raw["ProxyUsername"])
	svc.ProxyPassword = extractString(raw["ProxyPassword"])

	// Cached contract metadata
	svc.Metadata = extractString(raw["Metadata"])
	svc.MetadataHash = extractString(raw["MetadataHash"])

	// Mendix Catalog integration
	svc.ApplicationId = extractString(raw["ApplicationId"])
	svc.EndpointId = extractString(raw["EndpointId"])
	svc.CatalogUrl = extractString(raw["CatalogUrl"])
	svc.EnvironmentType = extractString(raw["EnvironmentType"])

	// Parse HTTP configuration (nested part)
	if httpCfg, ok := raw["HttpConfiguration"].(map[string]any); ok {
		svc.HttpConfiguration = parseODataHttpConfiguration(httpCfg)
	}

	return svc, nil
}

// parseODataHttpConfiguration parses a Microflows$HttpConfiguration BSON map
// into the model.HttpConfiguration type used by consumed OData services.
func parseODataHttpConfiguration(raw map[string]any) *model.HttpConfiguration {
	cfg := &model.HttpConfiguration{}
	cfg.ID = model.ID(extractBsonID(raw["$ID"]))
	cfg.TypeName = extractString(raw["$Type"])
	cfg.UseAuthentication = extractBool(raw["UseHttpAuthentication"], false)
	cfg.Username = extractString(raw["HttpAuthenticationUserName"])
	cfg.Password = extractString(raw["HttpAuthenticationPassword"])
	cfg.HttpMethod = extractString(raw["HttpMethod"])
	cfg.OverrideLocation = extractBool(raw["OverrideLocation"], false)
	cfg.CustomLocation = extractString(raw["CustomLocation"])
	cfg.ClientCertificate = extractString(raw["ClientCertificate"])

	// Parse header entries
	headers := extractBsonArray(raw["HttpHeaderEntries"])
	for _, h := range headers {
		if hMap, ok := h.(map[string]any); ok {
			entry := &model.HttpHeaderEntry{}
			entry.ID = model.ID(extractBsonID(hMap["$ID"]))
			entry.TypeName = extractString(hMap["$Type"])
			entry.Key = extractString(hMap["Key"])
			entry.Value = extractString(hMap["Value"])
			cfg.HeaderEntries = append(cfg.HeaderEntries, entry)
		}
	}

	return cfg
}

// parsePublishedODataService parses a published OData service from BSON.
func (r *Reader) parsePublishedODataService(unitID, containerID string, contents []byte) (*model.PublishedODataService, error) {
	contents, err := r.resolveContents(unitID, contents)
	if err != nil {
		return nil, err
	}

	var raw map[string]any
	if err := bson.Unmarshal(contents, &raw); err != nil {
		return nil, fmt.Errorf("failed to unmarshal BSON: %w", err)
	}

	svc := &model.PublishedODataService{}
	svc.ID = model.ID(unitID)
	svc.TypeName = "ODataPublish$PublishedODataService2"
	svc.ContainerID = model.ID(containerID)

	svc.Name = extractString(raw["Name"])
	svc.Documentation = extractString(raw["Documentation"])
	svc.Path = extractString(raw["Path"])
	svc.Namespace = extractString(raw["Namespace"])
	svc.ServiceName = extractString(raw["ServiceName"])
	svc.Version = extractString(raw["Version"])
	svc.ODataVersion = extractString(raw["ODataVersion"])
	svc.Summary = extractString(raw["Summary"])
	svc.Description = extractString(raw["Description"])
	svc.PublishAssociations = extractBool(raw["PublishAssociations"], false)
	svc.UseGeneralization = extractBool(raw["UseGeneralization"], false)
	svc.Excluded = extractBool(raw["Excluded"], false)
	svc.AuthMicroflow = extractString(raw["AuthenticationMicroflow"])

	// Parse authentication types
	authTypes := extractBsonArray(raw["AuthenticationTypes"])
	for _, at := range authTypes {
		if s, ok := at.(string); ok {
			svc.AuthenticationTypes = append(svc.AuthenticationTypes, s)
		}
	}

	// Parse allowed module roles (BY_NAME references)
	allowedRoles := extractBsonArray(raw["AllowedModuleRoles"])
	for _, r := range allowedRoles {
		if name, ok := r.(string); ok {
			svc.AllowedModuleRoles = append(svc.AllowedModuleRoles, name)
		}
	}

	// Build map of entity type IDs for EntitySet -> EntityType resolution
	entityTypeMap := make(map[string]*model.PublishedEntityType) // ID -> EntityType

	// Parse entity types
	entityTypes := extractBsonArray(raw["EntityTypes"])
	for _, et := range entityTypes {
		if etMap, ok := et.(map[string]any); ok {
			entityType := parsePublishedEntityType(etMap)
			svc.EntityTypes = append(svc.EntityTypes, entityType)
			entityTypeMap[string(entityType.ID)] = entityType
		}
	}

	// Parse entity sets
	entitySets := extractBsonArray(raw["EntitySets"])
	for _, es := range entitySets {
		if esMap, ok := es.(map[string]any); ok {
			entitySet := parsePublishedEntitySet(esMap, entityTypeMap)
			svc.EntitySets = append(svc.EntitySets, entitySet)
		}
	}

	return svc, nil
}

// parsePublishedEntityType parses a published entity type from a BSON map.
func parsePublishedEntityType(raw map[string]any) *model.PublishedEntityType {
	et := &model.PublishedEntityType{}
	et.ID = model.ID(extractBsonID(raw["$ID"]))
	et.TypeName = extractString(raw["$Type"])
	et.Entity = extractString(raw["Entity"])
	et.ExposedName = extractString(raw["ExposedName"])
	et.Summary = extractString(raw["Summary"])
	et.Description = extractString(raw["Description"])

	// Parse members (attributes, associations, ids)
	members := extractBsonArray(raw["ChildMembers"])
	for _, m := range members {
		if mMap, ok := m.(map[string]any); ok {
			member := parsePublishedMember(mMap)
			et.Members = append(et.Members, member)
		}
	}

	return et
}

// parsePublishedEntitySet parses a published entity set from a BSON map.
func parsePublishedEntitySet(raw map[string]any, entityTypeMap map[string]*model.PublishedEntityType) *model.PublishedEntitySet {
	es := &model.PublishedEntitySet{}
	es.ID = model.ID(extractBsonID(raw["$ID"]))
	es.TypeName = extractString(raw["$Type"])
	es.ExposedName = extractString(raw["ExposedName"])
	es.UsePaging = extractBool(raw["UsePaging"], false)
	es.PageSize = extractInt(raw["PageSize"])

	// Resolve EntityType pointer (BY_ID reference)
	entityTypeID := extractBsonID(raw["EntityTypePointer"])
	if entityTypeID != "" {
		if et, ok := entityTypeMap[entityTypeID]; ok {
			es.EntityTypeName = et.Entity
		}
	}

	// Parse mode objects
	es.ReadMode = parseChangeMode(raw["ReadMode"])
	es.InsertMode = parseChangeMode(raw["InsertMode"])
	es.UpdateMode = parseChangeMode(raw["UpdateMode"])
	es.DeleteMode = parseChangeMode(raw["DeleteMode"])

	return es
}

// parsePublishedMember parses a published member from a BSON map.
func parsePublishedMember(raw map[string]any) *model.PublishedMember {
	m := &model.PublishedMember{}
	m.ID = model.ID(extractBsonID(raw["$ID"]))
	m.TypeName = extractString(raw["$Type"])
	m.ExposedName = extractString(raw["ExposedName"])
	m.Filterable = extractBool(raw["Filterable"], false)
	m.Sortable = extractBool(raw["Sortable"], false)
	m.IsPartOfKey = extractBool(raw["IsPartOfKey"], false)

	// Determine kind from $Type
	switch m.TypeName {
	case "ODataPublish$PublishedAttribute":
		m.Kind = "attribute"
		m.Name = extractString(raw["Attribute"])
		m.EdmType = extractString(raw["EdmType"])
	case "ODataPublish$PublishedAssociationEnd":
		m.Kind = "association"
		m.Name = extractString(raw["Association"])
		// Studio Pro stores the target entity and the bare association
		// name (separate from ExposedName, which is the navigation
		// property). They round-trip through these fields so that
		// ALTER ODATA SERVICE doesn't blank them out.
		m.AssociationTargetEntity = extractString(raw["Entity"])
		m.ExposedAssociationName = extractString(raw["ExposedAssociationName"])
		m.IsMany = extractBool(raw["IsMany"], false)
	case "ODataPublish$PublishedId":
		m.Kind = "id"
		m.Name = extractString(raw["Attribute"])
	default:
		m.Kind = "unknown"
	}

	return m
}

// parseChangeMode extracts the mode string from a change/read source BSON object.
func parseChangeMode(v any) string {
	if v == nil {
		return ""
	}
	modeMap, ok := v.(map[string]any)
	if !ok {
		return ""
	}

	typeName := extractString(modeMap["$Type"])
	switch typeName {
	case "ODataPublish$ReadSource":
		return "ReadFromDatabase"
	case "ODataPublish$CallMicroflowToRead":
		mfName := extractString(modeMap["Microflow"])
		if mfName != "" {
			return "CallMicroflow:" + mfName
		}
		return "CallMicroflow"
	case "ODataPublish$ChangeSource":
		return "ChangeFromDatabase"
	case "ODataPublish$ChangeNotSupported":
		return "NotSupported"
	case "ODataPublish$CallMicroflowToChange":
		mfName := extractString(modeMap["Microflow"])
		if mfName != "" {
			return "CallMicroflow:" + mfName
		}
		return "CallMicroflow"
	default:
		return typeName
	}
}
