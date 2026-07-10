// SPDX-License-Identifier: Apache-2.0

package catalog

import (
	"crypto/sha256"
	"fmt"
	"strings"

	"github.com/JordtenBulte-OLC/mxcli/mdl/types"
)

// buildContractEntities parses cached $metadata from consumed OData services
// and populates the contract_entities and contract_actions catalog tables.
func (b *Builder) buildContractEntities() error {
	services, err := b.reader.ListConsumedODataServices()
	if err != nil {
		return err
	}

	entityStmt, err := b.tx.Prepare(`
		INSERT OR IGNORE INTO contract_entities_data (Id, ServiceId, ServiceQualifiedName,
			EntityName, EntitySetName, KeyProperties, PropertyCount, NavigationCount,
			Summary, Description, ModuleName,
			ProjectId, SnapshotId)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer entityStmt.Close()

	actionStmt, err := b.tx.Prepare(`
		INSERT OR IGNORE INTO contract_actions_data (Id, ServiceId, ServiceQualifiedName,
			ActionName, IsBound, ParameterCount, ReturnType, ModuleName,
			ProjectId, SnapshotId)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer actionStmt.Close()

	projectID, snapshotID := b.snapshotMeta()

	entityCount := 0
	actionCount := 0

	for _, svc := range services {
		if svc.Metadata == "" {
			continue
		}

		moduleID := b.hierarchy.findModuleID(svc.ContainerID)
		moduleName := b.hierarchy.getModuleName(moduleID)
		svcQN := moduleName + "." + svc.Name

		doc, err := types.ParseEdmx(svc.Metadata)
		if err != nil {
			continue // skip services with unparseable metadata
		}

		// Build entity set lookup
		esMap := make(map[string]string)
		for _, es := range doc.EntitySets {
			esMap[es.EntityType] = es.Name
		}

		for _, s := range doc.Schemas {
			for _, et := range s.EntityTypes {
				syntheticID := fmt.Sprintf("%x", sha256.Sum256([]byte(svcQN+"|entity|"+et.Name)))[:32]
				entitySetName := esMap[s.Namespace+"."+et.Name]
				keyProps := strings.Join(et.KeyProperties, ", ")

				_, err := entityStmt.Exec(
					syntheticID,
					string(svc.ID),
					svcQN,
					et.Name,
					entitySetName,
					keyProps,
					len(et.Properties),
					len(et.NavigationProperties),
					et.Summary,
					et.Description,
					moduleName,
					projectID, snapshotID,
				)
				if err != nil {
					return err
				}
				entityCount++
			}
		}

		for _, a := range doc.Actions {
			syntheticID := fmt.Sprintf("%x", sha256.Sum256([]byte(svcQN+"|action|"+a.Name)))[:32]
			isBound := 0
			if a.IsBound {
				isBound = 1
			}
			retType := a.ReturnType
			if retType == "" {
				retType = "(void)"
			}

			_, err := actionStmt.Exec(
				syntheticID,
				string(svc.ID),
				svcQN,
				a.Name,
				isBound,
				len(a.Parameters),
				retType,
				moduleName,
				projectID, snapshotID,
			)
			if err != nil {
				return err
			}
			actionCount++
		}
	}

	b.report("Contract Entities", entityCount)
	if actionCount > 0 {
		b.report("Contract Actions", actionCount)
	}
	return nil
}

// buildContractEntityUsage links contract_entities to the external_entities that
// consume them. Runs after both buildExternalEntities and buildContractEntities.
// Targets the underlying _data table because contract_entities is a view.
func (b *Builder) buildContractEntityUsage() error {
	_, err := b.tx.Exec(`
		UPDATE contract_entities_data
		SET UsedByExternalEntity = (
			SELECT ee.QualifiedName
			FROM external_entities ee
			JOIN odata_clients oc
				ON oc.QualifiedName = ee.ServiceName
			WHERE oc.Id = contract_entities_data.ServiceId
			  AND ee.RemoteName = contract_entities_data.EntityName
			LIMIT 1
		)
	`)
	return err
}

// buildContractMessages parses cached AsyncAPI documents from business event client
// services and populates the contract_messages catalog table.
func (b *Builder) buildContractMessages() error {
	services, err := b.cachedBusinessEventServices()
	if err != nil {
		return err
	}

	stmt, err := b.tx.Prepare(`
		INSERT OR IGNORE INTO contract_messages_data (Id, ServiceId, ServiceQualifiedName,
			ChannelName, OperationType, MessageName, Title, ContentType, PropertyCount,
			ModuleName, ProjectId, SnapshotId)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	projectID, snapshotID := b.snapshotMeta()

	count := 0

	for _, svc := range services {
		if svc.Document == "" {
			continue
		}

		moduleID := b.hierarchy.findModuleID(svc.ContainerID)
		moduleName := b.hierarchy.getModuleName(moduleID)
		svcQN := moduleName + "." + svc.Name

		doc, err := types.ParseAsyncAPI(svc.Document)
		if err != nil {
			continue
		}

		// Build message lookup for property counts
		msgProps := make(map[string]int)
		for _, msg := range doc.Messages {
			msgProps[msg.Name] = len(msg.Properties)
		}

		for _, ch := range doc.Channels {
			syntheticID := fmt.Sprintf("%x", sha256.Sum256([]byte(svcQN+"|channel|"+ch.Name+"|"+ch.MessageRef)))[:32]

			propCount := msgProps[ch.MessageRef]

			// Find message details
			title := ""
			contentType := ""
			if msg := doc.FindMessage(ch.MessageRef); msg != nil {
				title = msg.Title
				contentType = msg.ContentType
			}

			_, err := stmt.Exec(
				syntheticID,
				string(svc.ID),
				svcQN,
				ch.Name,
				ch.OperationType,
				ch.MessageRef,
				title,
				contentType,
				propCount,
				moduleName,
				projectID, snapshotID,
			)
			if err != nil {
				return err
			}
			count++
		}
	}

	b.report("Contract Messages", count)
	return nil
}
