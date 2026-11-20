// SPDX-License-Identifier: Apache-2.0

package catalog

import (
	"fmt"
	"strings"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/sdk/domainmodel"
	"github.com/mendixlabs/mxcli/sdk/javaactions"
)

func (b *Builder) buildModules() error {
	modules, err := b.reader.ListModules()
	if err != nil {
		return err
	}

	// The domain model's documentation, by module. A module has no
	// documentation of its own; its domain model does, and nothing read it.
	// A failed domain-model read costs the column, not the module list.
	// The modules are rebuilt every time, and an incremental build decodes
	// only the domain models it rebuilds: it reads every domain model's
	// stored Documentation instead.
	dmDocs := map[string]string{}
	if b.scope == nil {
		if dms, err := b.cachedDomainModels(); err == nil {
			for _, dm := range dms {
				if dm.Documentation != "" {
					dmDocs[string(b.hierarchy.findModuleID(dm.ContainerID))] = dm.Documentation
				}
			}
		}
	} else if units, err := b.reader.ListRawUnitsByType("DomainModels$DomainModel"); err == nil {
		for _, u := range units {
			if doc, ok := bson.Raw(u.Contents).Lookup("Documentation").StringValueOK(); ok && doc != "" {
				dmDocs[string(b.hierarchy.findModuleID(u.ContainerID))] = doc
			}
		}
	}

	stmt, err := b.tx.Prepare(`
		INSERT INTO modules_data (Id, Name, QualifiedName, ModuleName, Folder, Description,
			Source, AppStoreVersion, AppStoreGuid, DomainModelDocumentation,
			ProjectId, SnapshotId)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	projectID, snapshotID := b.snapshotMeta()

	for _, m := range modules {
		source := ""
		if m.FromAppStore {
			if m.AppStoreVersion != "" {
				source = "Marketplace v" + m.AppStoreVersion
			} else {
				source = "Marketplace"
			}
		}
		if m.PackageFile != "" {
			// ako/mxcli#1780: held as modules/<Name>.mxmodule. A non-empty
			// Source is also what keeps lint off a module the user cannot edit.
			if source != "" {
				source += ", "
			}
			source += "Packaged"
		}
		_, err := stmt.Exec(
			string(m.ID),
			m.Name,
			m.Name, // QualifiedName same as Name for modules
			"",     // ModuleName empty for modules
			"",     // Folder
			m.Documentation,
			source,
			m.AppStoreVersion,
			m.AppStoreGuid,
			dmDocs[string(m.ID)],
			projectID, snapshotID,
		)
		if err != nil {
			return err
		}
	}

	b.report("Modules", len(modules))
	return nil
}

func (b *Builder) buildEntities() error {
	// Get all domain models (cached — reused by buildReferences)
	domainModels, err := b.cachedDomainModels()
	if err != nil {
		return err
	}

	entityStmt, err := b.tx.Prepare(`
		INSERT INTO entities_data (Id, Name, QualifiedName, ModuleName, Folder, EntityType,
			Description, Generalization, AttributeCount, AssociationCount,
			AccessRuleCount, ValidationRuleCount, HasEventHandlers,
			HasCreatedDate, HasChangedDate, HasOwner, HasChangedBy,
			IsExternal, ExternalService,
			ProjectId, SnapshotId)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer entityStmt.Close()

	attrStmt, err := b.tx.Prepare(`
		INSERT INTO attributes_data (Id, Name, EntityId, EntityQualifiedName, ModuleName,
			DataType, EnumerationQualifiedName, Length, IsUnique, IsRequired, DefaultValue,
			IsCalculated, Description, ProjectId, SnapshotId)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer attrStmt.Close()

	projectID, snapshotID := b.snapshotMeta()

	entityCount := 0
	attrCount := 0
	for _, dm := range domainModels {
		if !b.own(string(dm.ID)) {
			continue
		}
		// Get module name using hierarchy
		moduleID := b.hierarchy.findModuleID(dm.ContainerID)
		moduleName := b.hierarchy.getModuleName(moduleID)

		// ListDomainModels already returns full domain models with entities
		for _, entity := range dm.Entities {
			entityType := "PERSISTENT"
			// Check entity type: view entities have OQL source, non-persistable have Persistable=false
			if strings.Contains(entity.Source, "OqlView") {
				entityType = "VIEW"
			} else if !entity.Persistable {
				entityType = "NON_PERSISTENT"
			}

			qualifiedName := moduleName + "." + entity.Name

			// Get generalization from GeneralizationRef
			generalization := entity.GeneralizationRef

			isExternal := 0
			externalService := ""
			if entity.Source == "Rest$ODataRemoteEntitySource" {
				isExternal = 1
				externalService = entity.RemoteServiceName
			}

			hasEventHandlers := 0
			if len(entity.EventHandlers) > 0 {
				hasEventHandlers = 1
			}

			_, err := entityStmt.Exec(
				string(entity.ID),
				entity.Name,
				qualifiedName,
				moduleName,
				"", // Folder - entities don't have folders
				entityType,
				entity.Documentation,
				generalization,
				len(entity.Attributes),
				0, // Association count - would need to count from associations
				len(entity.AccessRules),
				len(entity.ValidationRules),
				hasEventHandlers,
				boolToInt(entity.HasCreatedDate),
				boolToInt(entity.HasChangedDate),
				boolToInt(entity.HasOwner),
				boolToInt(entity.HasChangedBy),
				isExternal,
				externalService,
				projectID, snapshotID,
			)
			if err != nil {
				return err
			}
			entityCount++

			// Build maps for validation rules (attribute ID or name -> unique/required)
			// The AttributeID can be a UUID or a qualified name like "DmTest.Cars.CarId"
			uniqueByID := make(map[string]bool)
			requiredByID := make(map[string]bool)
			uniqueByName := make(map[string]bool)
			requiredByName := make(map[string]bool)
			for _, vr := range entity.ValidationRules {
				attrID := string(vr.AttributeID)
				// Extract attribute name from qualified name (e.g., "DmTest.Cars.CarId" -> "CarId")
				attrName := extractAttrName(attrID)
				switch vr.Type {
				case "Unique":
					uniqueByID[attrID] = true
					if attrName != "" {
						uniqueByName[attrName] = true
					}
				case "Required":
					requiredByID[attrID] = true
					if attrName != "" {
						requiredByName[attrName] = true
					}
				}
			}

			// Insert attributes
			for _, attr := range entity.Attributes {
				dataType := ""
				enumQN := ""
				length := 0
				if attr.Type != nil {
					dataType = attr.Type.GetTypeName()
					// Try to get length for string types
					if st, ok := attr.Type.(*domainmodel.StringAttributeType); ok {
						length = st.Length
					}
					// GetTypeName is the bare kind, so an enumeration attribute
					// reports only "Enumeration". Which enumeration is a separate
					// column; without it nothing downstream can check a value
					// against the cases.
					if et, ok := attr.Type.(*domainmodel.EnumerationAttributeType); ok {
						enumQN = et.EnumerationRef
					}
				}

				// Check for unique/required constraints by ID first, then by name
				isUnique := 0
				if uniqueByID[string(attr.ID)] || uniqueByName[attr.Name] {
					isUnique = 1
				}
				isRequired := 0
				if requiredByID[string(attr.ID)] || requiredByName[attr.Name] {
					isRequired = 1
				}

				defaultValue := ""
				isCalculated := 0
				if attr.Value != nil {
					defaultValue = attr.Value.DefaultValue
					if attr.Value.MicroflowName != "" || attr.Value.MicroflowID != "" {
						isCalculated = 1
					}
				}

				_, err := attrStmt.Exec(
					string(attr.ID),
					attr.Name,
					string(entity.ID),
					qualifiedName,
					moduleName,
					dataType,
					enumQN,
					length,
					isUnique,
					isRequired,
					defaultValue,
					isCalculated,
					attr.Documentation,
					projectID, snapshotID,
				)
				if err != nil {
					return err
				}
				attrCount++
			}
		}
	}
	b.disown()

	b.report("Entities", entityCount)
	b.report("Attributes", attrCount)
	return nil
}

// extractAttrName extracts the attribute name from a qualified name or ID.
// e.g., "DmTest.Cars.CarId" -> "CarId"
func extractAttrName(qualifiedName string) string {
	parts := strings.Split(qualifiedName, ".")
	if len(parts) >= 3 {
		return parts[len(parts)-1]
	}
	return ""
}

func (b *Builder) buildEnumerations() error {
	// Get all enumerations (cached — reused by buildStrings)
	enums, err := b.cachedEnumerations()
	if err != nil {
		return err
	}

	stmt, err := b.tx.Prepare(`
		INSERT INTO enumerations_data (Id, Name, QualifiedName, ModuleName, Folder, Description, ValueCount,
			ProjectId, SnapshotId)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	valueStmt, err := b.tx.Prepare(`
		INSERT INTO enumeration_values_data (Id, EnumerationId, EnumerationQualifiedName,
			ModuleName, Name, Caption, Ordinal, ProjectId, SnapshotId)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer valueStmt.Close()

	projectID, snapshotID := b.snapshotMeta()
	valueCount := 0
	lang := b.defaultLanguage()

	for _, enum := range enums {
		if !b.own(string(enum.ID)) {
			continue
		}
		// Get module name using hierarchy
		moduleID := b.hierarchy.findModuleID(enum.ContainerID)
		moduleName := b.hierarchy.getModuleName(moduleID)

		qualifiedName := moduleName + "." + enum.Name

		// Resolve the real folder path from the enum's container (matches pages /
		// simple docs). Hardcoding this to the module name made a MOVE ENUMERATION
		// … TO FOLDER look like a silent no-op in the catalog even though the move
		// persisted (Bug 12b).
		folderPath := b.hierarchy.buildFolderPath(enum.ContainerID)

		_, err := stmt.Exec(
			string(enum.ID),
			enum.Name,
			qualifiedName,
			moduleName,
			folderPath,
			enum.Documentation,
			len(enum.Values),
			projectID, snapshotID,
		)
		if err != nil {
			return err
		}

		for i, v := range enum.Values {
			// A value's Id is not always populated on read, and the pair
			// (enumeration, name) is what identifies it anyway — a synthetic key
			// keeps the row insertable either way.
			id := string(v.ID)
			if id == "" {
				id = string(enum.ID) + "/" + v.Name
			}
			// The project's own language first: a hardcoded "en_US" put the
			// English caption in the column of a Dutch project that has both, so
			// CATALOG.ENUMERATION_VALUES and DESCRIBE disagreed about the same
			// value (mendixlabs/mxcli#1113). Any translation still beats none —
			// nothing downstream keys off this, the checker matches on Name —
			// and the last resort is sorted, since a row that varies run to run
			// is indistinguishable from a model change.
			caption := ""
			if v.Caption != nil {
				caption = pickTranslation(v.Caption.Translations, lang)
			}
			if _, err := valueStmt.Exec(
				id,
				string(enum.ID),
				qualifiedName,
				moduleName,
				v.Name,
				caption,
				i,
				projectID, snapshotID,
			); err != nil {
				return err
			}
			valueCount++
		}
	}
	b.disown()

	b.report("Enumerations", len(enums))
	b.report("Enumeration values", valueCount)
	return nil
}

// catalogCodeActionType encodes a Java action return or parameter type for the
// catalog. A type-parameter reference is prefixed rather than written as its
// bare name: Studio Pro accepts any name for a type parameter, including a
// primitive's, and a bare `String` made an action returning its type parameter
// called String indistinguishable from one returning the primitive
// (mendixlabs/mxcli#1183). The prefix follows the `Kind:Name` shape
// microflows_data.ReturnType already uses; no primitive contains a colon.
//
//	TypeParameter:T          — an object of the entity bound to T
//	List of TypeParameter:T  — a list of them
//	EntityTypeParameter:T    — the entity-type selector that binds T
//
// DESCRIBE keeps the bare name, which is its MDL syntax; this is the catalog's
// encoding only.
func catalogCodeActionType(t interface{ TypeString() string }) string {
	switch tp := t.(type) {
	case *javaactions.TypeParameter:
		return "TypeParameter:" + tp.TypeParameter
	case *javaactions.EntityTypeParameterType:
		return "EntityTypeParameter:" + tp.TypeParameterName
	case *javaactions.ListType:
		if tp.TypeParameter != "" {
			return "List of TypeParameter:" + tp.TypeParameter
		}
	}
	return t.TypeString()
}

func (b *Builder) buildJavaActions() error {
	actions, err := b.lister().ListJavaActionsFull()
	if err != nil {
		return err
	}

	stmt, err := b.tx.Prepare(`
		INSERT INTO java_actions_data (Id, Name, QualifiedName, ModuleName, Folder,
			Documentation, ExportLevel, ReturnType, ParameterCount,
			ProjectId, SnapshotId)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	paramStmt, err := b.tx.Prepare(`
		INSERT INTO java_action_parameters_data (Id, JavaActionId, JavaActionName,
			QualifiedName, ModuleName, Name, Description, ParameterType, IsRequired,
			Ordinal, ProjectId, SnapshotId)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer paramStmt.Close()

	projectID, snapshotID := b.snapshotMeta()
	paramCount := 0

	for _, ja := range actions {
		if !b.own(string(ja.ID)) {
			continue
		}
		moduleID := b.hierarchy.findModuleID(ja.ContainerID)
		moduleName := b.hierarchy.getModuleName(moduleID)
		qualifiedName := moduleName + "." + ja.Name
		folder := b.hierarchy.buildFolderPath(ja.ContainerID)

		returnType := ""
		if ja.ReturnType != nil {
			returnType = catalogCodeActionType(ja.ReturnType)
		}

		_, err := stmt.Exec(
			string(ja.ID),
			ja.Name,
			qualifiedName,
			moduleName,
			folder,
			ja.Documentation,
			ja.ExportLevel,
			returnType,
			len(ja.Parameters),
			projectID, snapshotID,
		)
		if err != nil {
			return err
		}

		for i, p := range ja.Parameters {
			if p == nil {
				continue
			}
			paramType := ""
			if p.ParameterType != nil {
				paramType = catalogCodeActionType(p.ParameterType)
			}
			if _, err := paramStmt.Exec(
				string(p.ID),
				string(ja.ID),
				ja.Name,
				qualifiedName+"."+p.Name,
				moduleName,
				p.Name,
				p.Description,
				paramType,
				boolToInt(p.IsRequired),
				i,
				projectID, snapshotID,
			); err != nil {
				return err
			}
			paramCount++
		}
	}
	b.disown()

	b.report("Java Actions", len(actions))
	if paramCount > 0 {
		b.report("Java Action Parameters", paramCount)
	}
	return nil
}

func (b *Builder) buildODataClients() error {
	services, err := b.lister().ListConsumedODataServices()
	if err != nil {
		return err
	}

	stmt, err := b.tx.Prepare(`
		INSERT INTO odata_clients_data (Id, Name, QualifiedName, ModuleName,
			Version, ODataVersion, MetadataUrl, Validated,
			ProjectId, SnapshotId)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	projectID, snapshotID := b.snapshotMeta()

	for _, svc := range services {
		if !b.own(string(svc.ID)) {
			continue
		}
		moduleID := b.hierarchy.findModuleID(svc.ContainerID)
		moduleName := b.hierarchy.getModuleName(moduleID)

		qualifiedName := moduleName + "." + svc.Name

		// The runtime calls these on every request the service makes, so the
		// service is their caller.
		b.addServiceMicroflowRefs(RefObjectConsumedODataService, svc.ID, qualifiedName, moduleName,
			RefKindCall, svc.ConfigurationMicroflow, svc.HeadersMicroflow, svc.ErrorHandlingMicroflow)

		validated := 0
		if svc.Validated {
			validated = 1
		}

		_, err := stmt.Exec(
			string(svc.ID),
			svc.Name,
			qualifiedName,
			moduleName,
			svc.Version,
			svc.ODataVersion,
			svc.MetadataUrl,
			validated,
			projectID, snapshotID,
		)
		if err != nil {
			return err
		}
	}
	b.disown()

	b.report("OData Clients", len(services))
	return nil
}

func (b *Builder) buildODataServices() error {
	services, err := b.lister().ListPublishedODataServices()
	if err != nil {
		return err
	}

	stmt, err := b.tx.Prepare(`
		INSERT INTO odata_services_data (Id, Name, QualifiedName, ModuleName,
			Path, Version, ODataVersion, EntitySetCount, AuthenticationTypes,
			ProjectId, SnapshotId)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	projectID, snapshotID := b.snapshotMeta()

	for _, svc := range services {
		if !b.own(string(svc.ID)) {
			continue
		}
		moduleID := b.hierarchy.findModuleID(svc.ContainerID)
		moduleName := b.hierarchy.getModuleName(moduleID)

		qualifiedName := moduleName + "." + svc.Name
		authTypes := strings.Join(svc.AuthenticationTypes, ", ")

		// Every microflow the service runs is an entry point: a client's
		// request reaches it, nothing in the model calls it. Without the edge a
		// resource's read microflow and the service's authentication microflow
		// read as dead to `refs`, `impact` and QUAL004 (ako/mxcli#1639).
		b.addServiceMicroflowRefs(RefObjectPublishedODataService, svc.ID, qualifiedName, moduleName,
			RefKindPublish, publishedODataMicroflows(svc)...)

		_, err := stmt.Exec(
			string(svc.ID),
			svc.Name,
			qualifiedName,
			moduleName,
			svc.Path,
			svc.Version,
			svc.ODataVersion,
			len(svc.EntitySets),
			authTypes,
			projectID, snapshotID,
		)
		if err != nil {
			return err
		}
	}
	b.disown()

	b.report("OData Services", len(services))
	return nil
}

func (b *Builder) buildBusinessEventServices() error {
	services, err := b.cachedBusinessEventServices()
	if err != nil {
		return err
	}

	stmt, err := b.tx.Prepare(`
		INSERT INTO business_event_services_data (Id, Name, QualifiedName, ModuleName,
			Documentation, ServiceName, EventNamePrefix,
			MessageCount, PublishCount, SubscribeCount,
			ProjectId, SnapshotId)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	projectID, snapshotID := b.snapshotMeta()

	for _, svc := range services {
		if !b.own(string(svc.ID)) {
			continue
		}
		moduleID := b.hierarchy.findModuleID(svc.ContainerID)
		moduleName := b.hierarchy.getModuleName(moduleID)
		qualifiedName := moduleName + "." + svc.Name

		// Count messages and operations
		var msgCount, publishCount, subscribeCount int
		if svc.Definition != nil {
			for _, ch := range svc.Definition.Channels {
				msgCount += len(ch.Messages)
			}
		}
		handlers := make([]string, 0, len(svc.OperationImplementations))
		for _, op := range svc.OperationImplementations {
			switch op.Operation {
			case "publish":
				publishCount++
			case "subscribe":
				subscribeCount++
			}
			handlers = append(handlers, op.Microflow)
		}
		// An operation's microflow is the handler the platform runs when the
		// event arrives — an event handler, like an entity's, and reached the
		// same way: nothing in the model calls it.
		b.addServiceMicroflowRefs(RefObjectBusinessEventService, svc.ID, qualifiedName, moduleName,
			RefKindEvent, handlers...)

		serviceName := ""
		eventNamePrefix := ""
		if svc.Definition != nil {
			serviceName = svc.Definition.ServiceName
			eventNamePrefix = svc.Definition.EventNamePrefix
		}

		_, err := stmt.Exec(
			string(svc.ID),
			svc.Name,
			qualifiedName,
			moduleName,
			svc.Documentation,
			serviceName,
			eventNamePrefix,
			msgCount,
			publishCount,
			subscribeCount,
			projectID, snapshotID,
		)
		if err != nil {
			return err
		}
	}
	b.disown()

	b.report("Business Event Services", len(services))
	return nil
}

// buildBusinessEvents populates the business_events detail table with individual messages.
func (b *Builder) buildBusinessEvents() error {
	services, err := b.cachedBusinessEventServices()
	if err != nil {
		return err
	}

	stmt, err := b.tx.Prepare(`
		INSERT INTO business_events_data (Id, ServiceId, ServiceQualifiedName, ChannelName,
			MessageName, CanPublish, CanSubscribe, AttributeCount,
			Entity, PublishMicroflow, SubscribeMicroflow,
			ModuleName, ProjectId, SnapshotId)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	projectID, snapshotID := b.snapshotMeta()

	count := 0
	for _, svc := range services {
		if !b.own(string(svc.ID)) {
			continue
		}
		moduleID := b.hierarchy.findModuleID(svc.ContainerID)
		moduleName := b.hierarchy.getModuleName(moduleID)
		svcQN := moduleName + "." + svc.Name

		if svc.Definition == nil {
			continue
		}

		// Build operation lookup: messageName -> operation details
		type opInfo struct {
			entity    string
			microflow string
			operation string
		}
		opMap := make(map[string]*opInfo)
		for _, op := range svc.OperationImplementations {
			opMap[op.MessageName+"|"+op.Operation] = &opInfo{
				entity:    op.Entity,
				microflow: op.Microflow,
				operation: op.Operation,
			}
		}

		for _, ch := range svc.Definition.Channels {
			for _, msg := range ch.Messages {
				canPublish := 0
				canSubscribe := 0
				if msg.CanPublish {
					canPublish = 1
				}
				if msg.CanSubscribe {
					canSubscribe = 1
				}

				entity := ""
				publishMF := ""
				subscribeMF := ""
				if pub, ok := opMap[msg.MessageName+"|publish"]; ok {
					entity = pub.entity
					publishMF = pub.microflow
				}
				if sub, ok := opMap[msg.MessageName+"|subscribe"]; ok {
					if entity == "" {
						entity = sub.entity
					}
					subscribeMF = sub.microflow
				}

				syntheticID := fmt.Sprintf("%s|%s|%s", svc.ID, ch.ChannelName, msg.MessageName)

				_, err := stmt.Exec(
					syntheticID,
					string(svc.ID),
					svcQN,
					ch.ChannelName,
					msg.MessageName,
					canPublish,
					canSubscribe,
					len(msg.Attributes),
					entity,
					publishMF,
					subscribeMF,
					moduleName,
					projectID, snapshotID,
				)
				if err != nil {
					return err
				}
				count++
			}
		}
	}
	b.disown()

	b.report("Business Events", count)
	return nil
}

func (b *Builder) buildDatabaseConnections() error {
	connections, err := b.cachedDatabaseConnections()
	if err != nil {
		return err
	}

	stmt, err := b.tx.Prepare(`
		INSERT INTO database_connections_data (Id, Name, QualifiedName, ModuleName, Folder,
			DatabaseType, QueryCount,
			ProjectId, SnapshotId)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	projectID, snapshotID := b.snapshotMeta()

	for _, conn := range connections {
		if !b.own(string(conn.ID)) {
			continue
		}
		moduleID := b.hierarchy.findModuleID(conn.ContainerID)
		moduleName := b.hierarchy.getModuleName(moduleID)
		qualifiedName := moduleName + "." + conn.Name
		folderPath := b.hierarchy.buildFolderPath(conn.ContainerID)

		_, err := stmt.Exec(
			string(conn.ID),
			conn.Name,
			qualifiedName,
			moduleName,
			folderPath,
			conn.DatabaseType,
			len(conn.Queries),
			projectID, snapshotID,
		)
		if err != nil {
			return err
		}
	}
	b.disown()

	b.report("Database Connections", len(connections))
	return nil
}

func (b *Builder) buildJarDependencies() error {
	allSettings, err := b.cachedModuleSettings()
	if err != nil {
		return err
	}

	stmt, err := b.tx.Prepare(`
		INSERT INTO jar_dependencies_data (Id, ModuleName, GroupId, ArtifactId, Coordinate, Version, IsIncluded,
			ProjectId, SnapshotId)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	projectID, snapshotID := b.snapshotMeta()

	count := 0
	for _, ms := range allSettings {
		if !b.own(string(ms.ID)) {
			continue
		}
		moduleName := b.hierarchy.getModuleName(ms.ContainerID)
		for _, dep := range ms.JarDependencies {
			coordinate := dep.GroupID + ":" + dep.ArtifactID
			included := 0
			if dep.IsIncluded {
				included = 1
			}
			_, err := stmt.Exec(
				string(dep.ID),
				moduleName,
				dep.GroupID,
				dep.ArtifactID,
				coordinate,
				dep.Version,
				included,
				projectID, snapshotID,
			)
			if err != nil {
				return err
			}
			count++
		}
	}
	b.disown()

	b.report("JAR Dependencies", count)
	return nil
}

func (b *Builder) buildJsonStructures() error {
	structures, err := b.lister().ListJsonStructures()
	if err != nil {
		return err
	}

	stmt, err := b.tx.Prepare(`
		INSERT INTO json_structures_data (Name, QualifiedName, ModuleName,
			ElementCount, HasSnippet, Documentation, ExportLevel, Folder,
			ProjectId, SnapshotId)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	projectID, snapshotID := b.snapshotMeta()

	for _, js := range structures {
		if !b.own(string(js.ID)) {
			continue
		}
		moduleID := b.hierarchy.findModuleID(js.ContainerID)
		moduleName := b.hierarchy.getModuleName(moduleID)
		qualifiedName := moduleName + "." + js.Name
		folderPath := b.hierarchy.buildFolderPath(js.ContainerID)

		elemCount := 0
		if len(js.Elements) > 0 {
			elemCount = len(js.Elements[0].Children)
		}

		hasSnippet := 0
		if js.JsonSnippet != "" {
			hasSnippet = 1
		}

		_, err := stmt.Exec(
			js.Name,
			qualifiedName,
			moduleName,
			elemCount,
			hasSnippet,
			js.Documentation,
			js.ExportLevel,
			folderPath,
			projectID, snapshotID,
		)
		if err != nil {
			return err
		}
	}
	b.disown()

	b.report("JSON Structures", len(structures))
	return nil
}

func (b *Builder) buildImportMappings() error {
	mappings, err := b.lister().ListImportMappings()
	if err != nil {
		return err
	}

	stmt, err := b.tx.Prepare(`
		INSERT INTO import_mappings_data (Id, Name, QualifiedName, ModuleName,
			SchemaSource, ElementCount, Documentation, Folder, Excluded,
			ProjectId, SnapshotId)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	projectID, snapshotID := b.snapshotMeta()

	for _, im := range mappings {
		if !b.own(string(im.ID)) {
			continue
		}
		moduleID := b.hierarchy.findModuleID(im.ContainerID)
		moduleName := b.hierarchy.getModuleName(moduleID)
		qualifiedName := moduleName + "." + im.Name
		folderPath := b.hierarchy.buildFolderPath(im.ContainerID)

		src := im.JsonStructure
		if src == "" {
			src = im.XmlSchema
		}
		if src == "" {
			src = im.MessageDefinitionSource()
		}

		_, err := stmt.Exec(
			string(im.ID),
			im.Name,
			qualifiedName,
			moduleName,
			src,
			len(im.Elements),
			im.Documentation,
			folderPath,
			im.Excluded,
			projectID, snapshotID,
		)
		if err != nil {
			return err
		}
	}
	b.disown()

	b.report("Import Mappings", len(mappings))
	return nil
}

func (b *Builder) buildExportMappings() error {
	mappings, err := b.lister().ListExportMappings()
	if err != nil {
		return err
	}

	stmt, err := b.tx.Prepare(`
		INSERT INTO export_mappings_data (Id, Name, QualifiedName, ModuleName,
			SchemaSource, NullValueOption, ElementCount, Documentation, Folder, Excluded,
			ProjectId, SnapshotId)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	projectID, snapshotID := b.snapshotMeta()

	for _, em := range mappings {
		if !b.own(string(em.ID)) {
			continue
		}
		moduleID := b.hierarchy.findModuleID(em.ContainerID)
		moduleName := b.hierarchy.getModuleName(moduleID)
		qualifiedName := moduleName + "." + em.Name
		folderPath := b.hierarchy.buildFolderPath(em.ContainerID)

		src := em.JsonStructure
		if src == "" {
			src = em.XmlSchema
		}
		if src == "" {
			src = em.MessageDefinitionSource()
		}

		_, err := stmt.Exec(
			string(em.ID),
			em.Name,
			qualifiedName,
			moduleName,
			src,
			em.NullValueOption,
			len(em.Elements),
			em.Documentation,
			folderPath,
			em.Excluded,
			projectID, snapshotID,
		)
		if err != nil {
			return err
		}
	}
	b.disown()

	b.report("Export Mappings", len(mappings))
	return nil
}
