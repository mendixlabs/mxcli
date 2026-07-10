// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"fmt"
	"strings"

	"github.com/JordtenBulte-OLC/mxcli/model"

	"go.mongodb.org/mongo-driver/bson"
)

// ============================================================================
// Consumed OData Service (OData Client) — Rest$ConsumedODataService
// ============================================================================

// CreateConsumedODataService creates a new consumed OData service (client) document.
func (w *Writer) CreateConsumedODataService(svc *model.ConsumedODataService) error {
	if svc.ID == "" {
		svc.ID = model.ID(generateUUID())
	}
	svc.TypeName = "Rest$ConsumedODataService"

	contents, err := w.serializeConsumedODataService(svc)
	if err != nil {
		return fmt.Errorf("failed to serialize consumed OData service: %w", err)
	}

	return w.insertUnit(string(svc.ID), string(svc.ContainerID), "Documents", "Rest$ConsumedODataService", contents)
}

// UpdateConsumedODataService updates an existing consumed OData service.
func (w *Writer) UpdateConsumedODataService(svc *model.ConsumedODataService) error {
	contents, err := w.serializeConsumedODataService(svc)
	if err != nil {
		return fmt.Errorf("failed to serialize consumed OData service: %w", err)
	}

	return w.updateUnit(string(svc.ID), contents)
}

// DeleteConsumedODataService deletes a consumed OData service by ID.
func (w *Writer) DeleteConsumedODataService(id model.ID) error {
	return w.deleteUnit(string(id))
}

// serializeConsumedODataService converts a ConsumedODataService to BSON bytes.
func (w *Writer) serializeConsumedODataService(svc *model.ConsumedODataService) ([]byte, error) {
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
		{Key: "MetadataReferences", Value: bson.A{int32(0)}}, // empty BSON array marker
		{Key: "ValidatedEntities", Value: bson.A{int32(0)}},  // empty BSON array marker
		{Key: "LastUpdated", Value: ""},
		{Key: "UseQuerySegment", Value: false},
		{Key: "MinimumMxVersion", Value: ""},
		{Key: "RecommendedMxVersion", Value: ""},
	}

	// Microflow reference (BY_NAME). Mendix renamed this storage field across
	// versions: `ConfigurationMicroflow` (10.12–11.10) → `ConfigurationEntity-
	// Microflow` (11.10+). Writing the wrong key makes Studio Pro ignore it and
	// fall back to "Constants only" (issue #728), so gate on the project version.
	configKey, headersKey := "ConfigurationMicroflow", "ConfigurationMicroflow"
	if w.reader != nil {
		if pv := w.reader.ProjectVersion(); pv != nil {
			configKey = model.ODataConfigMicroflowBSONKey(pv.MajorVersion, pv.MinorVersion)
			headersKey = model.ODataHeadersMicroflowBSONKey(pv.MajorVersion, pv.MinorVersion)
		}
	}
	if svc.ConfigurationMicroflow != "" {
		doc = append(doc, bson.E{Key: configKey, Value: svc.ConfigurationMicroflow})
	}
	if svc.HeadersMicroflow != "" {
		doc = append(doc, bson.E{Key: headersKey, Value: svc.HeadersMicroflow})
	}
	if svc.ErrorHandlingMicroflow != "" {
		doc = append(doc, bson.E{Key: "ErrorHandlingMicroflow", Value: svc.ErrorHandlingMicroflow})
	}

	// Proxy constant references (BY_NAME)
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

	// Mendix Catalog integration (optional)
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

	// HTTP configuration (required nested part)
	doc = append(doc, bson.E{Key: "HttpConfiguration", Value: serializeHttpConfiguration(svc.HttpConfiguration)})

	return marshalUnitIDFirst(doc)
}

// serializeHttpConfiguration converts an HttpConfiguration to a BSON map.
// If cfg is nil, a minimal default configuration is created.
func serializeHttpConfiguration(cfg *model.HttpConfiguration) bson.D {
	cfgID := generateUUID()
	if cfg != nil && cfg.ID != "" {
		cfgID = string(cfg.ID)
	}

	// Field defaults; overridden below when cfg is provided. These are
	// resolved before building the ordered document so that providing a
	// cfg replaces (rather than duplicates) the default entries.
	useHttpAuthentication := false
	httpAuthenticationUserName := ""
	httpAuthenticationPassword := ""
	httpMethod := "Post"
	overrideLocation := false
	customLocation := ""
	clientCertificate := ""
	var httpHeaderEntries bson.A

	if cfg != nil {
		useHttpAuthentication = cfg.UseAuthentication
		httpAuthenticationUserName = cfg.Username
		httpAuthenticationPassword = cfg.Password
		if cfg.HttpMethod != "" {
			httpMethod = cfg.HttpMethod
		}
		overrideLocation = cfg.OverrideLocation
		customLocation = cfg.CustomLocation
		clientCertificate = cfg.ClientCertificate

		// Serialize header entries
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
			httpHeaderEntries = headers
		}
	}

	doc := bson.D{
		{Key: "$ID", Value: idToBsonBinary(cfgID)},
		{Key: "$Type", Value: "Microflows$HttpConfiguration"},
		{Key: "UseHttpAuthentication", Value: useHttpAuthentication},
		{Key: "HttpAuthenticationUserName", Value: httpAuthenticationUserName},
		{Key: "HttpAuthenticationPassword", Value: httpAuthenticationPassword},
		{Key: "HttpMethod", Value: httpMethod},
		{Key: "OverrideLocation", Value: overrideLocation},
		{Key: "CustomLocation", Value: customLocation},
		{Key: "ClientCertificate", Value: clientCertificate},
	}

	if httpHeaderEntries != nil {
		doc = append(doc, bson.E{Key: "HttpHeaderEntries", Value: httpHeaderEntries})
	}

	return doc
}

// ============================================================================
// Published OData Service — ODataPublish$PublishedODataService2
// ============================================================================

// CreatePublishedODataService creates a new published OData service document.
func (w *Writer) CreatePublishedODataService(svc *model.PublishedODataService) error {
	if svc.ID == "" {
		svc.ID = model.ID(generateUUID())
	}
	svc.TypeName = "ODataPublish$PublishedODataService2"

	contents, err := w.serializePublishedODataService(svc)
	if err != nil {
		return fmt.Errorf("failed to serialize published OData service: %w", err)
	}

	return w.insertUnit(string(svc.ID), string(svc.ContainerID), "Documents", "ODataPublish$PublishedODataService2", contents)
}

// UpdatePublishedODataService updates an existing published OData service.
func (w *Writer) UpdatePublishedODataService(svc *model.PublishedODataService) error {
	contents, err := w.serializePublishedODataService(svc)
	if err != nil {
		return fmt.Errorf("failed to serialize published OData service: %w", err)
	}

	return w.updateUnit(string(svc.ID), contents)
}

// DeletePublishedODataService deletes a published OData service by ID.
func (w *Writer) DeletePublishedODataService(id model.ID) error {
	return w.deleteUnit(string(id))
}

// serializePublishedODataService converts a PublishedODataService to BSON bytes.
func (w *Writer) serializePublishedODataService(svc *model.PublishedODataService) ([]byte, error) {
	// Authentication types array (versioned: starts with int32(3))
	authTypes := bson.A{int32(3)}
	for _, at := range svc.AuthenticationTypes {
		authTypes = append(authTypes, at)
	}

	// Serialize entity types and build ID map for entity set pointers.
	// Issue #595: key by qualified entity name (et.Entity), not ExposedName.
	// PublishedEntitySet.EntityTypeName holds the qualified name, so keying
	// by ExposedName made the lookup return "" and EntityTypePointer was
	// never written. Studio Pro's EntitySet.Check then NREs dereferencing
	// the missing pointer and aborts the whole project checker.
	//
	// Versioned BSON arrays in Mendix start with an int32 storage marker
	// (typically 3). Without it Mendix treats the array as malformed and
	// silently drops elements after the first — observed in CE6585 firing
	// on the second entity in a multi-entity service.
	entityTypeIDMap := make(map[string]string) // qualified entity name -> entity type ID
	entityTypes := bson.A{int32(3)}
	for _, et := range svc.EntityTypes {
		etID := string(et.ID)
		if etID == "" {
			etID = generateUUID()
			et.ID = model.ID(etID)
		}
		entityTypeIDMap[et.Entity] = etID
		entityTypes = append(entityTypes, serializePublishedEntityType(et))
	}

	// Serialize entity sets with BY_ID pointers to entity types
	entitySets := bson.A{int32(3)}
	for _, es := range svc.EntitySets {
		esID := string(es.ID)
		if esID == "" {
			esID = generateUUID()
			es.ID = model.ID(esID)
		}
		// Resolve EntityTypeName to EntityType ID
		entityTypeID := entityTypeIDMap[es.EntityTypeName]
		entitySets = append(entitySets, serializePublishedEntitySet(es, entityTypeID))
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
		// Empty collection markers required by Studio Pro 11.10. Without
		// these fields Mendix can resolve the first entity's key but fails
		// to resolve the second's (CE6585) — observed when comparing a
		// Studio Pro-authored multi-entity service against ours.
		{Key: "Enumerations", Value: bson.A{int32(3)}},
		{Key: "Microflows", Value: bson.A{int32(3)}},
		{Key: "IncludeMetadataByDefault", Value: true},
		{Key: "ReplaceIllegalChars", Value: false},
		{Key: "SupportsGraphQL", Value: false},
	}
	return marshalUnitIDFirst(doc)
}

// serializePublishedEntityType converts a PublishedEntityType to a BSON map.
func serializePublishedEntityType(et *model.PublishedEntityType) bson.D {
	// Serialize child members. Pass the owning entity's qualified name so
	// the writer can emit fully-qualified Attribute / Association BSON
	// references (Module.Entity.AttributeName) — Studio Pro and mx check
	// require these to be qualified, and using bare names made the second
	// entity's members silently fail to link in a multi-entity service.
	// Like EntityTypes / EntitySets, ChildMembers is a Mendix versioned
	// array and must start with the int32(3) storage marker.
	members := bson.A{int32(3)}
	for _, m := range et.Members {
		members = append(members, serializePublishedMember(m, et.Entity))
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

// serializePublishedEntitySet converts a PublishedEntitySet to a BSON map.
func serializePublishedEntitySet(es *model.PublishedEntitySet, entityTypeID string) bson.D {
	doc := bson.D{
		{Key: "$ID", Value: idToBsonBinary(string(es.ID))},
		{Key: "$Type", Value: "ODataPublish$EntitySet"},
		{Key: "ExposedName", Value: es.ExposedName},
		{Key: "AlternativeExposedName", Value: ""},
		{Key: "UsePaging", Value: es.UsePaging},
		{Key: "PageSize", Value: int64(es.PageSize)},
		// QueryOptions is required by Studio Pro's BSON shape for the
		// entity set to be considered valid. Without it the second
		// published entity in a multi-entity service fails to resolve
		// its key (CE6585) — see Studio Pro reference dump.
		{Key: "QueryOptions", Value: bson.D{
			{Key: "$ID", Value: idToBsonBinary(generateUUID())},
			{Key: "$Type", Value: "ODataPublish$QueryOptions"},
			{Key: "Countable", Value: true},
			{Key: "SkipSupported", Value: true},
			{Key: "TopSupported", Value: true},
		}},
	}

	// EntityTypePointer is a BY_ID reference
	if entityTypeID != "" {
		doc = append(doc, bson.E{Key: "EntityTypePointer", Value: idToBsonBinary(entityTypeID)})
	}

	// Serialize mode objects
	if es.ReadMode != "" {
		doc = append(doc, bson.E{Key: "ReadMode", Value: serializeReadMode(es.ReadMode)})
	}
	if es.InsertMode != "" {
		doc = append(doc, bson.E{Key: "InsertMode", Value: serializeChangeMode(es.InsertMode)})
	}
	if es.UpdateMode != "" {
		doc = append(doc, bson.E{Key: "UpdateMode", Value: serializeChangeMode(es.UpdateMode)})
	}
	if es.DeleteMode != "" {
		doc = append(doc, bson.E{Key: "DeleteMode", Value: serializeChangeMode(es.DeleteMode)})
	}

	return doc
}

// serializePublishedMember converts a PublishedMember to a BSON map.
// `ownerQN` is the qualified name (Module.Entity) of the EntityType this
// member belongs to. Mendix expects PublishedAttribute.Attribute and
// PublishedAssociationEnd.Association BSON values to be fully qualified —
// "Module.Entity.AttributeName" for attributes and "Module.AssociationName"
// for associations. If the AST already supplied a qualified name (contains
// a dot), it's used as-is; otherwise the owner is prepended.
func serializePublishedMember(m *model.PublishedMember, ownerQN string) bson.D {
	memberID := string(m.ID)
	if memberID == "" {
		memberID = generateUUID()
	}

	// Base fields written by Studio Pro for both attribute and association
	// members. Description/Summary stay empty in this writer; CanBeEmpty
	// defaults to !IsPartOfKey (keys are required to have a value) which
	// matches Studio Pro's convention and is required for Mendix to
	// recognise the attribute as a valid OData key (CE6585).
	doc := bson.D{
		{Key: "$ID", Value: idToBsonBinary(memberID)},
		{Key: "ExposedName", Value: m.ExposedName},
		{Key: "CanBeEmpty", Value: !m.IsPartOfKey},
		{Key: "Description", Value: ""},
		{Key: "Summary", Value: ""},
	}

	switch m.Kind {
	case "attribute":
		doc = append(doc, bson.E{Key: "$Type", Value: "ODataPublish$PublishedAttribute"})
		doc = append(doc, bson.E{Key: "Attribute", Value: qualifyMemberName(m.Name, ownerQN)})
		// EdmType is the published OData type; without it Studio Pro reports
		// CE5016 ("published as ."). Verified against Studio Pro's corrected BSON.
		doc = append(doc, bson.E{Key: "EdmType", Value: m.EdmType})
		doc = append(doc, bson.E{Key: "Filterable", Value: m.Filterable})
		doc = append(doc, bson.E{Key: "Sortable", Value: m.Sortable})
		doc = append(doc, bson.E{Key: "IsPartOfKey", Value: m.IsPartOfKey})
		doc = append(doc, bson.E{Key: "EnumerationAsString", Value: false})
		doc = append(doc, bson.E{Key: "StringAsGuid", Value: false})
	case "association":
		doc = append(doc, bson.E{Key: "$Type", Value: "ODataPublish$PublishedAssociationEnd"})
		// Associations live at module scope (Module.AssocName), so prepend
		// only the module portion of the owner.
		doc = append(doc, bson.E{Key: "Association", Value: qualifyAssociationName(m.Name, ownerQN)})
		// AssociationEnd carries the target entity and a separate
		// ExposedAssociationName (typically the bare assoc name). Both
		// are required by Studio Pro's BSON shape.
		doc = append(doc, bson.E{Key: "Entity", Value: m.AssociationTargetEntity})
		// IsMany is the exposed navigation's multiplicity; without it Studio Pro
		// reports CE5022 ("changed multiplicity"). Verified against Studio Pro BSON.
		doc = append(doc, bson.E{Key: "IsMany", Value: m.IsMany})
		doc = append(doc, bson.E{Key: "ExposedAssociationName", Value: m.ExposedAssociationName})
	case "id":
		doc = append(doc, bson.E{Key: "$Type", Value: "ODataPublish$PublishedId"})
		doc = append(doc, bson.E{Key: "Attribute", Value: qualifyMemberName(m.Name, ownerQN)})
		doc = append(doc, bson.E{Key: "Filterable", Value: m.Filterable})
		doc = append(doc, bson.E{Key: "Sortable", Value: m.Sortable})
		doc = append(doc, bson.E{Key: "IsPartOfKey", Value: m.IsPartOfKey})
	default:
		// Default to attribute for unknown kinds
		doc = append(doc, bson.E{Key: "$Type", Value: "ODataPublish$PublishedAttribute"})
		doc = append(doc, bson.E{Key: "Attribute", Value: qualifyMemberName(m.Name, ownerQN)})
		doc = append(doc, bson.E{Key: "Filterable", Value: m.Filterable})
		doc = append(doc, bson.E{Key: "Sortable", Value: m.Sortable})
		doc = append(doc, bson.E{Key: "IsPartOfKey", Value: m.IsPartOfKey})
		doc = append(doc, bson.E{Key: "EnumerationAsString", Value: false})
		doc = append(doc, bson.E{Key: "StringAsGuid", Value: false})
	}

	return doc
}

// qualifyMemberName prepends the owning entity's qualified name (Module.Entity)
// to a bare attribute name. If `name` already contains a dot (already qualified)
// or `ownerQN` is empty, the original is returned unchanged.
func qualifyMemberName(name, ownerQN string) string {
	if name == "" || ownerQN == "" || strings.Contains(name, ".") {
		return name
	}
	return ownerQN + "." + name
}

// qualifyAssociationName prepends the owning entity's module to a bare
// association name. Associations live at module scope, so only the module
// portion of `ownerQN` is used.
func qualifyAssociationName(name, ownerQN string) string {
	if name == "" || ownerQN == "" || strings.Contains(name, ".") {
		return name
	}
	if idx := strings.IndexByte(ownerQN, '.'); idx > 0 {
		return ownerQN[:idx] + "." + name
	}
	return name
}

// serializeReadMode converts a read mode string to a BSON mode object.
// Accepts both parsed format ("ReadFromDatabase") and MDL format ("SOURCE").
func serializeReadMode(mode string) bson.D {
	modeID := idToBsonBinary(generateUUID())

	switch {
	case strings.EqualFold(mode, "ReadFromDatabase") || strings.EqualFold(mode, "SOURCE"):
		return bson.D{
			{Key: "$ID", Value: modeID},
			{Key: "$Type", Value: "ODataPublish$ReadSource"},
		}
	case strings.HasPrefix(mode, "CallMicroflow:"):
		return bson.D{
			{Key: "$ID", Value: modeID},
			{Key: "$Type", Value: "ODataPublish$CallMicroflowToRead"},
			{Key: "Microflow", Value: strings.TrimPrefix(mode, "CallMicroflow:")},
		}
	case strings.HasPrefix(mode, "MICROFLOW "):
		return bson.D{
			{Key: "$ID", Value: modeID},
			{Key: "$Type", Value: "ODataPublish$CallMicroflowToRead"},
			{Key: "Microflow", Value: strings.TrimPrefix(mode, "MICROFLOW ")},
		}
	default:
		// Unknown mode — store as ReadSource
		return bson.D{
			{Key: "$ID", Value: modeID},
			{Key: "$Type", Value: "ODataPublish$ReadSource"},
		}
	}
}

// serializeChangeMode converts a change mode string to a BSON mode object.
// Accepts both parsed format ("ChangeFromDatabase", "NotSupported") and MDL format ("SOURCE", "NOT_SUPPORTED").
func serializeChangeMode(mode string) bson.D {
	modeID := idToBsonBinary(generateUUID())

	switch {
	case strings.EqualFold(mode, "ChangeFromDatabase") || strings.EqualFold(mode, "SOURCE"):
		return bson.D{
			{Key: "$ID", Value: modeID},
			{Key: "$Type", Value: "ODataPublish$ChangeSource"},
		}
	case strings.EqualFold(mode, "NotSupported") || strings.EqualFold(mode, "NOT_SUPPORTED"):
		return bson.D{
			{Key: "$ID", Value: modeID},
			{Key: "$Type", Value: "ODataPublish$ChangeNotSupported"},
		}
	case strings.HasPrefix(mode, "CallMicroflow:"):
		return bson.D{
			{Key: "$ID", Value: modeID},
			{Key: "$Type", Value: "ODataPublish$CallMicroflowToChange"},
			{Key: "Microflow", Value: strings.TrimPrefix(mode, "CallMicroflow:")},
		}
	case strings.HasPrefix(mode, "MICROFLOW "):
		return bson.D{
			{Key: "$ID", Value: modeID},
			{Key: "$Type", Value: "ODataPublish$CallMicroflowToChange"},
			{Key: "Microflow", Value: strings.TrimPrefix(mode, "MICROFLOW ")},
		}
	default:
		// Unknown mode — store as ChangeNotSupported
		return bson.D{
			{Key: "$ID", Value: modeID},
			{Key: "$Type", Value: "ODataPublish$ChangeNotSupported"},
		}
	}
}
