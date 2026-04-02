// SPDX-License-Identifier: Apache-2.0

package catalog

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

func (b *Builder) buildModules() error {
	modules, err := b.reader.ListModules()
	if err != nil {
		return err
	}

	stmt, err := b.tx.Prepare(`
		INSERT INTO modules (Id, Name, QualifiedName, ModuleName, Folder, Description,
			Source, AppStoreVersion, AppStoreGuid,
			ProjectId, ProjectName, SnapshotId, SnapshotDate, SnapshotSource,
			SourceId, SourceBranch, SourceRevision)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	projectID, projectName, snapshotID, snapshotDate, snapshotSource, sourceID, sourceBranch, sourceRevision := b.snapshotMeta()

	for _, m := range modules {
		source := ""
		if m.FromAppStore {
			if m.AppStoreVersion != "" {
				source = "Marketplace v" + m.AppStoreVersion
			} else {
				source = "Marketplace"
			}
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
			projectID, projectName, snapshotID, snapshotDate, snapshotSource,
			sourceID, sourceBranch, sourceRevision,
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
		INSERT INTO entities (Id, Name, QualifiedName, ModuleName, Folder, EntityType,
			Description, Generalization, AttributeCount, AssociationCount,
			AccessRuleCount, ValidationRuleCount, HasEventHandlers,
			IsExternal, ExternalService,
			ProjectId, ProjectName, SnapshotId, SnapshotDate, SnapshotSource,
			SourceId, SourceBranch, SourceRevision)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer entityStmt.Close()

	attrStmt, err := b.tx.Prepare(`
		INSERT INTO attributes (Id, Name, EntityId, EntityQualifiedName, ModuleName,
			DataType, Length, IsUnique, IsRequired, DefaultValue, IsCalculated, Description,
			ProjectId, ProjectName, SnapshotId, SnapshotDate, SnapshotSource,
			SourceId, SourceBranch, SourceRevision)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer attrStmt.Close()

	projectID, projectName, snapshotID, snapshotDate, snapshotSource, sourceID, sourceBranch, sourceRevision := b.snapshotMeta()

	entityCount := 0
	attrCount := 0
	for _, dm := range domainModels {
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
				isExternal,
				externalService,
				projectID, projectName, snapshotID, snapshotDate, snapshotSource,
				sourceID, sourceBranch, sourceRevision,
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
				length := 0
				if attr.Type != nil {
					dataType = attr.Type.GetTypeName()
					// Try to get length for string types
					if st, ok := attr.Type.(*domainmodel.StringAttributeType); ok {
						length = st.Length
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
					length,
					isUnique,
					isRequired,
					defaultValue,
					isCalculated,
					attr.Documentation,
					projectID, projectName, snapshotID, snapshotDate, snapshotSource,
					sourceID, sourceBranch, sourceRevision,
				)
				if err != nil {
					return err
				}
				attrCount++
			}
		}
	}

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
		INSERT INTO enumerations (Id, Name, QualifiedName, ModuleName, Folder, Description, ValueCount,
			ProjectId, ProjectName, SnapshotId, SnapshotDate, SnapshotSource,
			SourceId, SourceBranch, SourceRevision)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	projectID, projectName, snapshotID, snapshotDate, snapshotSource, sourceID, sourceBranch, sourceRevision := b.snapshotMeta()

	for _, enum := range enums {
		// Get module name using hierarchy
		moduleID := b.hierarchy.findModuleID(enum.ContainerID)
		moduleName := b.hierarchy.getModuleName(moduleID)

		qualifiedName := moduleName + "." + enum.Name

		_, err := stmt.Exec(
			string(enum.ID),
			enum.Name,
			qualifiedName,
			moduleName,
			moduleName, // Folder as module name for now
			enum.Documentation,
			len(enum.Values),
			projectID, projectName, snapshotID, snapshotDate, snapshotSource,
			sourceID, sourceBranch, sourceRevision,
		)
		if err != nil {
			return err
		}
	}

	b.report("Enumerations", len(enums))
	return nil
}

func (b *Builder) buildJavaActions() error {
	actions, err := b.reader.ListJavaActionsFull()
	if err != nil {
		return err
	}

	stmt, err := b.tx.Prepare(`
		INSERT INTO java_actions (Id, Name, QualifiedName, ModuleName, Folder,
			Documentation, ExportLevel, ReturnType, ParameterCount,
			ProjectId, ProjectName, SnapshotId, SnapshotDate, SnapshotSource,
			SourceId, SourceBranch, SourceRevision)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	projectID, projectName, snapshotID, snapshotDate, snapshotSource, sourceID, sourceBranch, sourceRevision := b.snapshotMeta()

	for _, ja := range actions {
		moduleID := b.hierarchy.findModuleID(ja.ContainerID)
		moduleName := b.hierarchy.getModuleName(moduleID)
		qualifiedName := moduleName + "." + ja.Name
		folder := b.hierarchy.buildFolderPath(ja.ContainerID)

		returnType := ""
		if ja.ReturnType != nil {
			returnType = ja.ReturnType.TypeString()
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
			projectID, projectName, snapshotID, snapshotDate, snapshotSource,
			sourceID, sourceBranch, sourceRevision,
		)
		if err != nil {
			return err
		}
	}

	b.report("Java Actions", len(actions))
	return nil
}

func (b *Builder) buildODataClients() error {
	services, err := b.reader.ListConsumedODataServices()
	if err != nil {
		return err
	}

	stmt, err := b.tx.Prepare(`
		INSERT INTO odata_clients (Id, Name, QualifiedName, ModuleName,
			Version, ODataVersion, MetadataUrl, Validated,
			ProjectId, ProjectName, SnapshotId, SnapshotDate, SnapshotSource,
			SourceId, SourceBranch, SourceRevision)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	projectID, projectName, snapshotID, snapshotDate, snapshotSource, sourceID, sourceBranch, sourceRevision := b.snapshotMeta()

	for _, svc := range services {
		moduleID := b.hierarchy.findModuleID(svc.ContainerID)
		moduleName := b.hierarchy.getModuleName(moduleID)

		qualifiedName := moduleName + "." + svc.Name

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
			projectID, projectName, snapshotID, snapshotDate, snapshotSource,
			sourceID, sourceBranch, sourceRevision,
		)
		if err != nil {
			return err
		}
	}

	b.report("OData Clients", len(services))
	return nil
}

func (b *Builder) buildODataServices() error {
	services, err := b.reader.ListPublishedODataServices()
	if err != nil {
		return err
	}

	stmt, err := b.tx.Prepare(`
		INSERT INTO odata_services (Id, Name, QualifiedName, ModuleName,
			Path, Version, ODataVersion, EntitySetCount, AuthenticationTypes,
			ProjectId, ProjectName, SnapshotId, SnapshotDate, SnapshotSource,
			SourceId, SourceBranch, SourceRevision)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	projectID, projectName, snapshotID, snapshotDate, snapshotSource, sourceID, sourceBranch, sourceRevision := b.snapshotMeta()

	for _, svc := range services {
		moduleID := b.hierarchy.findModuleID(svc.ContainerID)
		moduleName := b.hierarchy.getModuleName(moduleID)

		qualifiedName := moduleName + "." + svc.Name
		authTypes := strings.Join(svc.AuthenticationTypes, ", ")

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
			projectID, projectName, snapshotID, snapshotDate, snapshotSource,
			sourceID, sourceBranch, sourceRevision,
		)
		if err != nil {
			return err
		}
	}

	b.report("OData Services", len(services))
	return nil
}

func (b *Builder) buildBusinessEventServices() error {
	services, err := b.cachedBusinessEventServices()
	if err != nil {
		return err
	}

	stmt, err := b.tx.Prepare(`
		INSERT INTO business_event_services (Id, Name, QualifiedName, ModuleName,
			Documentation, ServiceName, EventNamePrefix,
			MessageCount, PublishCount, SubscribeCount,
			ProjectId, ProjectName, SnapshotId, SnapshotDate, SnapshotSource,
			SourceId, SourceBranch, SourceRevision)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	projectID, projectName, snapshotID, snapshotDate, snapshotSource, sourceID, sourceBranch, sourceRevision := b.snapshotMeta()

	for _, svc := range services {
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
		for _, op := range svc.OperationImplementations {
			switch op.Operation {
			case "publish":
				publishCount++
			case "subscribe":
				subscribeCount++
			}
		}

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
			projectID, projectName, snapshotID, snapshotDate, snapshotSource,
			sourceID, sourceBranch, sourceRevision,
		)
		if err != nil {
			return err
		}
	}

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
		INSERT INTO business_events (Id, ServiceId, ServiceQualifiedName, ChannelName,
			MessageName, CanPublish, CanSubscribe, AttributeCount,
			Entity, PublishMicroflow, SubscribeMicroflow,
			ModuleName, ProjectId, SnapshotId, SnapshotDate, SnapshotSource)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	projectID, _, snapshotID, snapshotDate, snapshotSource, _, _, _ := b.snapshotMeta()

	count := 0
	for _, svc := range services {
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
					projectID, snapshotID, snapshotDate, snapshotSource,
				)
				if err != nil {
					return err
				}
				count++
			}
		}
	}

	b.report("Business Events", count)
	return nil
}

func (b *Builder) buildDatabaseConnections() error {
	connections, err := b.cachedDatabaseConnections()
	if err != nil {
		return err
	}

	stmt, err := b.tx.Prepare(`
		INSERT INTO database_connections (Id, Name, QualifiedName, ModuleName, Folder,
			DatabaseType, QueryCount,
			ProjectId, ProjectName, SnapshotId, SnapshotDate, SnapshotSource,
			SourceId, SourceBranch, SourceRevision)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	projectID, projectName, snapshotID, snapshotDate, snapshotSource, sourceID, sourceBranch, sourceRevision := b.snapshotMeta()

	for _, conn := range connections {
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
			projectID, projectName, snapshotID, snapshotDate, snapshotSource,
			sourceID, sourceBranch, sourceRevision,
		)
		if err != nil {
			return err
		}
	}

	b.report("Database Connections", len(connections))
	return nil
}

func (b *Builder) buildJsonStructures() error {
	structures, err := b.reader.ListJsonStructures()
	if err != nil {
		return err
	}

	stmt, err := b.tx.Prepare(`
		INSERT INTO json_structures (Name, QualifiedName, ModuleName,
			ElementCount, HasSnippet, Documentation, ExportLevel, Folder,
			ProjectId, ProjectName, SnapshotId, SnapshotDate, SnapshotSource)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	projectID, projectName, snapshotID, snapshotDate, snapshotSource, _, _, _ := b.snapshotMeta()

	for _, js := range structures {
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
			projectID, projectName, snapshotID, snapshotDate, snapshotSource,
		)
		if err != nil {
			return err
		}
	}

	b.report("JSON Structures", len(structures))
	return nil
}
