// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"fmt"
	"sort"
	"strings"

	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/domainmodel"
	"github.com/JordtenBulte-OLC/mxcli/sdk/mpr/version"

	"go.mongodb.org/mongo-driver/bson"
)

// CreateEntity creates a new entity in a domain model.
// domainModelID is the ID of the domain model itself (not the module ID).
func (w *Writer) CreateEntity(domainModelID model.ID, entity *domainmodel.Entity) error {
	// Load the domain model by its ID
	dm, err := w.reader.GetDomainModelByID(domainModelID)
	if err != nil {
		return err
	}

	// Assign ID if not set
	if entity.ID == "" {
		entity.ID = model.ID(generateUUID())
	}
	entity.TypeName = "DomainModels$Entity"
	entity.ContainerID = domainModelID

	// Assign IDs to attributes if not set
	for _, attr := range entity.Attributes {
		if attr.ID == "" {
			attr.ID = model.ID(generateUUID())
		}
		attr.TypeName = "DomainModels$Attribute"
		attr.ContainerID = entity.ID
	}

	// Add entity to domain model
	dm.Entities = append(dm.Entities, entity)

	// Serialize and update
	return w.updateDomainModel(dm)
}

// UpdateEntity updates an existing entity.
// domainModelID is the ID of the domain model itself (not the module ID).
func (w *Writer) UpdateEntity(domainModelID model.ID, entity *domainmodel.Entity) error {
	dm, err := w.reader.GetDomainModelByID(domainModelID)
	if err != nil {
		return err
	}

	// Find and replace the entity
	for i, e := range dm.Entities {
		if e.ID == entity.ID {
			dm.Entities[i] = entity
			return w.updateDomainModel(dm)
		}
	}

	return fmt.Errorf("entity not found: %s", entity.ID)
}

// DeleteEntity deletes an entity from a domain model.
// domainModelID is the ID of the domain model itself (not the module ID).
// Cascade: any association in any module whose ParentID or ChildID matches
// entityID is also removed, preventing dangling unit-pointer errors.
func (w *Writer) DeleteEntity(domainModelID model.ID, entityID model.ID) error {
	dm, err := w.reader.GetDomainModelByID(domainModelID)
	if err != nil {
		return err
	}

	// Find and remove the entity
	found := false
	for i, e := range dm.Entities {
		if e.ID == entityID {
			dm.Entities = append(dm.Entities[:i], dm.Entities[i+1:]...)
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("entity not found: %s", entityID)
	}

	// Remove associations referencing this entity from the same DM
	var keptAssocs []*domainmodel.Association
	for _, a := range dm.Associations {
		if a.ParentID != entityID && a.ChildID != entityID {
			keptAssocs = append(keptAssocs, a)
		}
	}
	dm.Associations = keptAssocs

	if err := w.updateDomainModel(dm); err != nil {
		return err
	}

	// Cascade: remove associations referencing this entity from all other DMs
	allDMs, err := w.reader.ListDomainModels()
	if err != nil {
		return fmt.Errorf("cascade cleanup: list domain models: %w", err)
	}
	for _, other := range allDMs {
		if other.ID == domainModelID {
			continue
		}
		changed := false
		var kept []*domainmodel.Association
		for _, a := range other.Associations {
			if a.ParentID == entityID || a.ChildID == entityID {
				changed = true
			} else {
				kept = append(kept, a)
			}
		}
		if changed {
			other.Associations = kept
			if err := w.updateDomainModel(other); err != nil {
				return fmt.Errorf("cascade cleanup: update domain model %s: %w", other.ID, err)
			}
		}
	}

	return nil
}

// MoveEntity moves an entity from one domain model to another.
// Associations referencing the moved entity are converted to CrossAssociations
// (cross-module associations with BY_NAME references to the remote entity).
// Validation rule attribute references are updated to reflect the new module name.
// Returns the names of converted associations (for caller to inform about).
func (w *Writer) MoveEntity(entity *domainmodel.Entity, sourceDMID, targetDMID model.ID, sourceModuleName, targetModuleName string) ([]string, error) {
	// Load source domain model and remove the entity
	sourceDM, err := w.reader.GetDomainModelByID(sourceDMID)
	if err != nil {
		return nil, fmt.Errorf("failed to load source domain model: %w", err)
	}

	found := false
	for i, e := range sourceDM.Entities {
		if e.ID == entity.ID {
			sourceDM.Entities = append(sourceDM.Entities[:i], sourceDM.Entities[i+1:]...)
			found = true
			break
		}
	}
	if !found {
		return nil, fmt.Errorf("entity not found in source domain model: %s", entity.ID)
	}

	// Load target domain model
	targetDM, err := w.reader.GetDomainModelByID(targetDMID)
	if err != nil {
		return nil, fmt.Errorf("failed to load target domain model: %w", err)
	}

	// Convert associations referencing the moved entity to CrossAssociations.
	// - If moved entity is the child: CrossAssoc stays in source DM (parent is local)
	// - If moved entity is the parent: CrossAssoc goes to target DM (parent moves with entity)
	var convertedAssocs []string
	var keptAssocs []*domainmodel.Association
	for _, a := range sourceDM.Associations {
		if a.ChildID == entity.ID {
			// Child is being moved → CrossAssoc stays in source DM
			// ParentPointer = parent entity (stays local), Child = remote qualified name
			ca := &domainmodel.CrossModuleAssociation{}
			ca.ID = a.ID
			ca.TypeName = "DomainModels$CrossAssociation"
			ca.ContainerID = sourceDMID
			ca.Name = a.Name
			ca.Documentation = a.Documentation
			ca.ParentID = a.ParentID
			ca.ChildRef = targetModuleName + "." + entity.Name
			ca.Type = a.Type
			ca.Owner = a.Owner
			ca.StorageFormat = a.StorageFormat
			ca.ParentDeleteBehavior = a.ParentDeleteBehavior
			ca.ChildDeleteBehavior = a.ChildDeleteBehavior
			sourceDM.CrossAssociations = append(sourceDM.CrossAssociations, ca)
			convertedAssocs = append(convertedAssocs, a.Name)
		} else if a.ParentID == entity.ID {
			// Parent is being moved → CrossAssoc goes to target DM
			// ParentPointer = moved entity (will be local in target), Child = remote entity in source
			var childEntityName string
			for _, e := range sourceDM.Entities {
				if e.ID == a.ChildID {
					childEntityName = e.Name
					break
				}
			}
			ca := &domainmodel.CrossModuleAssociation{}
			ca.ID = a.ID
			ca.TypeName = "DomainModels$CrossAssociation"
			ca.ContainerID = targetDMID
			ca.Name = a.Name
			ca.Documentation = a.Documentation
			ca.ParentID = a.ParentID // parent entity ID (same, just moving to target DM)
			ca.ChildRef = sourceModuleName + "." + childEntityName
			ca.Type = a.Type
			ca.Owner = a.Owner
			ca.StorageFormat = a.StorageFormat
			ca.ParentDeleteBehavior = a.ParentDeleteBehavior
			ca.ChildDeleteBehavior = a.ChildDeleteBehavior
			targetDM.CrossAssociations = append(targetDM.CrossAssociations, ca)
			convertedAssocs = append(convertedAssocs, a.Name)
		} else {
			keptAssocs = append(keptAssocs, a)
		}
	}
	sourceDM.Associations = keptAssocs

	// Update validation rule attribute references in the moved entity.
	// These are BY_NAME qualified names like "OldModule.Entity.Attribute" that need
	// to be updated to "NewModule.Entity.Attribute".
	oldPrefix := sourceModuleName + "."
	newPrefix := targetModuleName + "."
	for _, vr := range entity.ValidationRules {
		attrIDStr := string(vr.AttributeID)
		if strings.HasPrefix(attrIDStr, oldPrefix) {
			vr.AttributeID = model.ID(newPrefix + attrIDStr[len(oldPrefix):])
		}
	}

	// Update SourceDocumentRef for view entities
	if entity.Source == "DomainModels$OqlViewEntitySource" && entity.SourceDocumentRef != "" {
		if strings.HasPrefix(entity.SourceDocumentRef, oldPrefix) {
			entity.SourceDocumentRef = newPrefix + entity.SourceDocumentRef[len(oldPrefix):]
		}
	}

	// Save source domain model
	if err := w.updateDomainModel(sourceDM); err != nil {
		return nil, fmt.Errorf("failed to update source domain model: %w", err)
	}

	// Add entity to target domain model and save
	entity.ContainerID = targetDMID
	targetDM.Entities = append(targetDM.Entities, entity)

	if err := w.updateDomainModel(targetDM); err != nil {
		return nil, fmt.Errorf("failed to update target domain model: %w", err)
	}

	return convertedAssocs, nil
}

// UpdateEnumerationRefsInAllDomainModels updates enumeration references across all domain models.
// When an enumeration is moved to a different module, its qualified name changes and all
// EnumerationAttributeType references need to be updated.
func (w *Writer) UpdateEnumerationRefsInAllDomainModels(oldQualifiedName, newQualifiedName string) error {
	dms, err := w.reader.ListDomainModels()
	if err != nil {
		return fmt.Errorf("failed to list domain models: %w", err)
	}

	for _, dm := range dms {
		changed := false
		for _, entity := range dm.Entities {
			for _, attr := range entity.Attributes {
				if enumType, ok := attr.Type.(*domainmodel.EnumerationAttributeType); ok {
					if enumType.EnumerationRef == oldQualifiedName {
						enumType.EnumerationRef = newQualifiedName
						enumType.EnumerationID = model.ID(newQualifiedName)
						changed = true
					}
				}
			}
		}
		if changed {
			if err := w.updateDomainModel(dm); err != nil {
				return fmt.Errorf("failed to update domain model %s: %w", dm.ID, err)
			}
		}
	}
	return nil
}

// MoveViewEntitySourceDocument moves a ViewEntitySourceDocument to a new module.
func (w *Writer) MoveViewEntitySourceDocument(sourceModuleName string, targetModuleID model.ID, docName string) error {
	docID, err := w.FindViewEntitySourceDocumentID(sourceModuleName, docName)
	if err != nil {
		return err
	}
	if docID == "" {
		return nil // No document to move
	}

	// Update ContainerID in database
	return w.moveUnitByID(string(docID), string(targetModuleID))
}

// UpdateOqlQueriesForMovedEntity updates OQL queries in all ViewEntitySourceDocuments
// to reflect a moved entity's new qualified name. For example, when DmTest.Customer moves
// to DmTest2.Customer, all OQL references like "DmTest.Customer" are updated.
func (w *Writer) UpdateOqlQueriesForMovedEntity(oldQualifiedName, newQualifiedName string) (int, error) {
	units, err := w.reader.listUnitsByType("DomainModels$ViewEntitySourceDocument")
	if err != nil {
		return 0, err
	}

	updated := 0
	for _, u := range units {
		var raw map[string]any
		if err := bson.Unmarshal(u.Contents, &raw); err != nil {
			continue
		}

		oql, _ := raw["Oql"].(string)
		if oql == "" || !strings.Contains(oql, oldQualifiedName) {
			continue
		}

		// Replace entity references in OQL
		newOql := strings.ReplaceAll(oql, oldQualifiedName, newQualifiedName)
		raw["Oql"] = newOql

		// Re-serialize and update
		contents, err := marshalUnitIDFirst(raw)
		if err != nil {
			continue
		}
		if err := w.updateUnit(u.ID, contents); err != nil {
			return updated, fmt.Errorf("failed to update ViewEntitySourceDocument %s: %w", u.ID, err)
		}
		updated++
	}
	return updated, nil
}

// moveUnitByID changes a unit's ContainerID without modifying its contents.
func (w *Writer) moveUnitByID(unitID string, newContainerID string) error {
	unitIDBlob := uuidToBlob(unitID)
	containerIDBlob := uuidToBlob(newContainerID)

	_, err := w.reader.db.Exec(`UPDATE Unit SET ContainerID = ? WHERE UnitID = ?`, containerIDBlob, unitIDBlob)
	if err == nil {
		w.reader.InvalidateCache()
	}
	return err
}

// AddAttribute adds an attribute to an entity.
// domainModelID is the ID of the domain model itself (not the module ID).
func (w *Writer) AddAttribute(domainModelID model.ID, entityID model.ID, attr *domainmodel.Attribute) error {
	dm, err := w.reader.GetDomainModelByID(domainModelID)
	if err != nil {
		return err
	}

	// Find the entity
	for _, e := range dm.Entities {
		if e.ID == entityID {
			if attr.ID == "" {
				attr.ID = model.ID(generateUUID())
			}
			attr.TypeName = "DomainModels$Attribute"
			attr.ContainerID = entityID
			e.Attributes = append(e.Attributes, attr)
			return w.updateDomainModel(dm)
		}
	}

	return fmt.Errorf("entity not found: %s", entityID)
}

// UpdateAttribute updates an existing attribute in an entity.
// domainModelID is the ID of the domain model itself (not the module ID).
func (w *Writer) UpdateAttribute(domainModelID model.ID, entityID model.ID, attr *domainmodel.Attribute) error {
	dm, err := w.reader.GetDomainModelByID(domainModelID)
	if err != nil {
		return err
	}

	// Find the entity
	for _, e := range dm.Entities {
		if e.ID == entityID {
			// Find and update the attribute
			for i, a := range e.Attributes {
				if a.ID == attr.ID {
					e.Attributes[i] = attr
					return w.updateDomainModel(dm)
				}
			}
			return fmt.Errorf("attribute not found: %s", attr.ID)
		}
	}

	return fmt.Errorf("entity not found: %s", entityID)
}

// DeleteAttribute deletes an attribute from an entity.
// domainModelID is the ID of the domain model itself (not the module ID).
func (w *Writer) DeleteAttribute(domainModelID model.ID, entityID model.ID, attrID model.ID) error {
	dm, err := w.reader.GetDomainModelByID(domainModelID)
	if err != nil {
		return err
	}

	// Find the entity
	for _, e := range dm.Entities {
		if e.ID == entityID {
			// Find and remove the attribute
			for i, a := range e.Attributes {
				if a.ID == attrID {
					e.Attributes = append(e.Attributes[:i], e.Attributes[i+1:]...)
					return w.updateDomainModel(dm)
				}
			}
			return fmt.Errorf("attribute not found: %s", attrID)
		}
	}

	return fmt.Errorf("entity not found: %s", entityID)
}

// CreateAssociation creates a new association between entities.
// domainModelID is the ID of the domain model itself (not the module ID).
func (w *Writer) CreateAssociation(domainModelID model.ID, assoc *domainmodel.Association) error {
	dm, err := w.reader.GetDomainModelByID(domainModelID)
	if err != nil {
		return err
	}

	if assoc.ID == "" {
		assoc.ID = model.ID(generateUUID())
	}
	assoc.TypeName = "DomainModels$Association"
	assoc.ContainerID = domainModelID

	dm.Associations = append(dm.Associations, assoc)
	return w.updateDomainModel(dm)
}

// CreateCrossAssociation creates a cross-module association in a domain model.
// The parent entity must be local to this domain model; the child entity is
// referenced by qualified name (BY_NAME) since it lives in another module.
func (w *Writer) CreateCrossAssociation(domainModelID model.ID, ca *domainmodel.CrossModuleAssociation) error {
	dm, err := w.reader.GetDomainModelByID(domainModelID)
	if err != nil {
		return err
	}

	if ca.ID == "" {
		ca.ID = model.ID(generateUUID())
	}
	ca.TypeName = "DomainModels$CrossAssociation"
	ca.ContainerID = domainModelID

	dm.CrossAssociations = append(dm.CrossAssociations, ca)
	return w.updateDomainModel(dm)
}

// DeleteAssociation deletes an association.
// domainModelID is the ID of the domain model itself (not the module ID).
func (w *Writer) DeleteAssociation(domainModelID model.ID, assocID model.ID) error {
	dm, err := w.reader.GetDomainModelByID(domainModelID)
	if err != nil {
		return err
	}

	for i, a := range dm.Associations {
		if a.ID == assocID {
			dm.Associations = append(dm.Associations[:i], dm.Associations[i+1:]...)
			return w.updateDomainModel(dm)
		}
	}

	return fmt.Errorf("association not found: %s", assocID)
}

// DeleteCrossAssociation removes a cross-module association from a domain model.
func (w *Writer) DeleteCrossAssociation(domainModelID model.ID, assocID model.ID) error {
	dm, err := w.reader.GetDomainModelByID(domainModelID)
	if err != nil {
		return err
	}

	for i, ca := range dm.CrossAssociations {
		if ca.ID == assocID {
			dm.CrossAssociations = append(dm.CrossAssociations[:i], dm.CrossAssociations[i+1:]...)
			return w.updateDomainModel(dm)
		}
	}

	return fmt.Errorf("cross-module association not found: %s", assocID)
}

// CreateViewEntitySourceDocument creates a ViewEntitySourceDocument for a view entity.
// This is a separate document that contains the OQL query for the view entity.
func (w *Writer) CreateViewEntitySourceDocument(moduleID model.ID, moduleName, docName, oqlQuery, documentation string) (model.ID, error) {
	docID := model.ID(generateUUID())

	doc := bson.D{
		{Key: "$ID", Value: idToBsonBinary(string(docID))},
		{Key: "$Type", Value: "DomainModels$ViewEntitySourceDocument"},
		{Key: "Documentation", Value: documentation},
		{Key: "Excluded", Value: false},
		{Key: "ExportLevel", Value: "Hidden"},
		{Key: "Name", Value: docName},
		{Key: "Oql", Value: oqlQuery},
	}

	contents, err := marshalUnitIDFirst(doc)
	if err != nil {
		return "", fmt.Errorf("failed to serialize ViewEntitySourceDocument: %w", err)
	}

	if err := w.insertUnit(string(docID), string(moduleID), "Documents", "DomainModels$ViewEntitySourceDocument", contents); err != nil {
		return "", fmt.Errorf("failed to insert ViewEntitySourceDocument: %w", err)
	}

	return docID, nil
}

// DeleteViewEntitySourceDocument deletes a ViewEntitySourceDocument.
func (w *Writer) DeleteViewEntitySourceDocument(id model.ID) error {
	return w.deleteUnit(string(id))
}

// FindViewEntitySourceDocumentID finds a ViewEntitySourceDocument by module and document name.
// Returns the document ID if found, empty string if not found.
func (w *Writer) FindViewEntitySourceDocumentID(moduleName, docName string) (model.ID, error) {
	units, err := w.reader.listUnitsByType("DomainModels$ViewEntitySourceDocument")
	if err != nil {
		return "", err
	}

	// Build module ID -> name map
	modules, err := w.reader.ListModules()
	if err != nil {
		return "", err
	}
	moduleNames := make(map[string]string)
	for _, m := range modules {
		moduleNames[string(m.ID)] = m.Name
	}

	for _, u := range units {
		var raw map[string]any
		if err := bson.Unmarshal(u.Contents, &raw); err != nil {
			continue
		}

		name, _ := raw["Name"].(string)
		modName := moduleNames[u.ContainerID]

		if modName == moduleName && name == docName {
			return model.ID(u.ID), nil
		}
	}

	return "", nil // Not found
}

// DeleteViewEntitySourceDocumentByName deletes ALL ViewEntitySourceDocuments matching the
// given module and document name. This handles cleanup of duplicate documents that may
// have accumulated from previous script runs or incomplete deletions.
// Returns nil if documents were deleted or none existed.
func (w *Writer) DeleteViewEntitySourceDocumentByName(moduleName, docName string) error {
	docIDs, err := w.FindAllViewEntitySourceDocumentIDs(moduleName, docName)
	if err != nil {
		return err
	}
	for _, docID := range docIDs {
		if err := w.deleteUnit(string(docID)); err != nil {
			return err
		}
	}
	return nil
}

// FindAllViewEntitySourceDocumentIDs finds ALL ViewEntitySourceDocuments matching the
// given module and document name. Returns all matching IDs (not just the first).
func (w *Writer) FindAllViewEntitySourceDocumentIDs(moduleName, docName string) ([]model.ID, error) {
	units, err := w.reader.listUnitsByType("DomainModels$ViewEntitySourceDocument")
	if err != nil {
		return nil, err
	}

	// Build module ID -> name map
	modules, err := w.reader.ListModules()
	if err != nil {
		return nil, err
	}
	moduleNames := make(map[string]string)
	for _, m := range modules {
		moduleNames[string(m.ID)] = m.Name
	}

	var ids []model.ID
	for _, u := range units {
		var raw map[string]any
		if err := bson.Unmarshal(u.Contents, &raw); err != nil {
			continue
		}

		name, _ := raw["Name"].(string)
		modName := moduleNames[u.ContainerID]

		if modName == moduleName && name == docName {
			ids = append(ids, model.ID(u.ID))
		}
	}

	return ids, nil
}
func (w *Writer) serializeDomainModel(dm *domainmodel.DomainModel) ([]byte, error) {
	// Look up module name for qualified names in validation rules
	moduleName := ""
	if dm.ContainerID != "" {
		module, err := w.reader.GetModule(dm.ContainerID)
		if err == nil && module != nil {
			moduleName = module.Name
		}
	}

	// Entities array with version prefix 3
	pv := w.reader.ProjectVersion()
	entities := bson.A{int32(3)}
	for _, e := range dm.Entities {
		entities = append(entities, serializeEntity(e, moduleName, pv))
	}

	// Associations array with version prefix 3
	associations := bson.A{int32(3)}
	for _, a := range dm.Associations {
		associations = append(associations, serializeAssociation(a))
	}

	// CrossAssociations array with version prefix 3
	crossAssociations := bson.A{int32(3)}
	for _, ca := range dm.CrossAssociations {
		crossAssociations = append(crossAssociations, serializeCrossAssociation(ca))
	}

	// Use bson.D (ordered) so $Type appears early — Mendix requires this for correct parsing
	doc := bson.D{
		{Key: "$ID", Value: idToBsonBinary(string(dm.ID))},
		{Key: "$Type", Value: "DomainModels$DomainModel"},
		{Key: "Documentation", Value: ""},
		{Key: "Annotations", Value: bson.A{int32(3)}},
		{Key: "Entities", Value: entities},
		{Key: "Associations", Value: associations},
		{Key: "CrossAssociations", Value: crossAssociations},
	}
	return marshalUnitIDFirst(doc)
}

func serializeEntity(e *domainmodel.Entity, moduleName string, pv *version.ProjectVersion) bson.D {
	// Any of the three OData source types means the attributes need OData
	// mapped value serialization (Rest$ODataMappedValue or its primitive
	// collection variant), not the regular DomainModels$StoredValue.
	isExternal := e.Source == "Rest$ODataRemoteEntitySource" ||
		e.Source == "Rest$ODataEntityTypeSource" ||
		e.Source == "Rest$ODataPrimitiveCollectionEntitySource"

	// Attributes array with version prefix 3
	attrs := bson.A{int32(3)}
	for _, a := range e.Attributes {
		attrs = append(attrs, serializeAttribute(a, isExternal))
	}

	// Indexes array with version prefix 3
	indexes := bson.A{int32(3)}
	for _, idx := range e.Indexes {
		indexes = append(indexes, serializeIndex(idx))
	}

	// ValidationRules array with version prefix 3
	validationRules := bson.A{int32(3)}
	for _, vr := range e.ValidationRules {
		validationRules = append(validationRules, serializeValidationRule(vr, moduleName, e))
	}

	// Generate a GUID for the entity if not present (used for qualified name)
	entityGUID := idToBsonBinary(string(e.ID))

	// Location is stored as "x;y" string format
	location := fmt.Sprintf("%d;%d", e.Location.X, e.Location.Y)

	// Serialize generalization: either a parent entity reference or NoGeneralization
	var maybeGeneralization bson.D
	if e.GeneralizationRef != "" {
		maybeGeneralization = serializeGeneralization(e.GeneralizationRef)
	} else {
		maybeGeneralization = serializeNoGeneralization(e, pv)
	}

	// AccessRules array with version prefix 3
	accessRules := bson.A{int32(3)}
	for _, ar := range e.AccessRules {
		accessRules = append(accessRules, serializeAccessRule(ar))
	}

	// Use bson.D (ordered document) to match Studio Pro field order
	// Mendix 11.12 requires "$ID" to be the first property of every storage object
	// ("$Type" conventionally second); it rejects the unit otherwise. Remaining
	// fields keep Studio Pro's order.
	// CRITICAL: Attributes MUST come before ValidationRules for attribute lookup to work
	doc := bson.D{
		{Key: "$ID", Value: idToBsonBinary(string(e.ID))},
		{Key: "$Type", Value: "DomainModels$EntityImpl"},
		{Key: "Name", Value: e.Name},
		{Key: "Documentation", Value: e.Documentation},
		{Key: "MaybeGeneralization", Value: maybeGeneralization},
		{Key: "Attributes", Value: attrs}, // Must come before ValidationRules!
		{Key: "AccessRules", Value: accessRules},
		{Key: "ValidationRules", Value: validationRules}, // After Attributes
		{Key: "ExportLevel", Value: "Hidden"},
		{Key: "GUID", Value: entityGUID},
		{Key: "Location", Value: location},
		{Key: "Indexes", Value: indexes},
		{Key: "Events", Value: serializeEventHandlers(e.EventHandlers)},
	}

	// Add Source for view entities (references a ViewEntitySourceDocument)
	if e.Source == "DomainModels$OqlViewEntitySource" && e.SourceDocumentRef != "" {
		doc = append(doc, bson.E{Key: "Source", Value: serializeOqlViewEntitySource(e.SourceObjectID, e.SourceDocumentRef, e.OqlQuery, pv)})
	}

	// Add Source for external entities (OData remote entity source)
	if e.Source == "Rest$ODataRemoteEntitySource" && e.RemoteServiceName != "" {
		doc = append(doc, bson.E{Key: "Source", Value: serializeODataRemoteEntitySource(e)})
	}

	// Source for entity-type-only external entities (derived/abstract/contained types
	// that have no entity set, e.g. PlanItem, Flight, Trip)
	if e.Source == "Rest$ODataEntityTypeSource" && e.RemoteServiceName != "" {
		doc = append(doc, bson.E{Key: "Source", Value: serializeODataEntityTypeSource(e)})
	}

	// Source for primitive collection NPEs (e.g. TripTag for Trip.Tags = Collection(Edm.String))
	if e.Source == "Rest$ODataPrimitiveCollectionEntitySource" && e.RemoteServiceName != "" {
		doc = append(doc, bson.E{Key: "Source", Value: bson.D{
			{Key: "$ID", Value: idToBsonBinary(generateUUID())},
			{Key: "$Type", Value: "Rest$ODataPrimitiveCollectionEntitySource"},
			{Key: "SourceDocument", Value: e.RemoteServiceName},
		}})
	}

	return doc
}

// serializeODataEntityTypeSource emits Rest$ODataEntityTypeSource for an entity
// that maps to an OData entity type but has no entity set (e.g. derived,
// abstract, or contained nav target). It carries only the type name, key, and
// SourceDocument — no CRUD or paging fields.
func serializeODataEntityTypeSource(e *domainmodel.Entity) bson.D {
	doc := bson.D{
		{Key: "$ID", Value: idToBsonBinary(generateUUID())},
		{Key: "$Type", Value: "Rest$ODataEntityTypeSource"},
		{Key: "EntityTypeName", Value: e.RemoteEntityName},
		{Key: "IsOpen", Value: e.IsOpen},
	}

	if len(e.RemoteKeyParts) > 0 {
		parts := bson.A{int32(2)}
		for _, kp := range e.RemoteKeyParts {
			parts = append(parts, serializeODataKeyPart(kp))
		}
		doc = append(doc, bson.E{Key: "Key", Value: bson.D{
			{Key: "$ID", Value: idToBsonBinary(generateUUID())},
			{Key: "$Type", Value: "Rest$ODataKey"},
			{Key: "Parts", Value: parts},
		}})
	}

	doc = append(doc, bson.E{Key: "SourceDocument", Value: e.RemoteServiceName})
	return doc
}

// serializeEventHandlers serializes a list of EventHandlers to a BSON array.
// Returns [int32(3)] for empty (storageListType 3 = stored object list).
func serializeEventHandlers(handlers []*domainmodel.EventHandler) bson.A {
	arr := bson.A{int32(3)}
	for _, eh := range handlers {
		arr = append(arr, serializeEventHandler(eh))
	}
	return arr
}

// serializeEventHandler serializes a single EventHandler to BSON.
// $Type is "DomainModels$EntityEvent". Microflow uses BY_NAME (string) reference.
func serializeEventHandler(eh *domainmodel.EventHandler) bson.D {
	ehID := string(eh.ID)
	if ehID == "" {
		ehID = generateUUID()
	}
	moment := string(eh.Moment)
	if moment == "" {
		moment = "Before"
	}
	event := string(eh.Event)
	if event == "" {
		event = "Commit"
	}
	// BY_NAME reference for the microflow
	var microflowRef interface{}
	if eh.MicroflowName != "" {
		microflowRef = eh.MicroflowName
	} else if eh.MicroflowID != "" {
		microflowRef = idToBsonBinary(string(eh.MicroflowID))
	} else {
		microflowRef = ""
	}
	return bson.D{
		{Key: "$ID", Value: idToBsonBinary(ehID)},
		{Key: "$Type", Value: "DomainModels$EntityEvent"},
		{Key: "Microflow", Value: microflowRef},
		{Key: "Moment", Value: moment},
		{Key: "RaiseErrorOnFalse", Value: eh.RaiseErrorOnFalse},
		{Key: "SendInputParameter", Value: eh.PassEventObject},
		{Key: "Type", Value: event},
	}
}

func serializeAccessRule(ar *domainmodel.AccessRule) bson.D {
	// AllowedModuleRoles: storageListType 1 (BY_NAME references)
	roles := bson.A{int32(1)}
	for _, name := range ar.ModuleRoleNames {
		roles = append(roles, name)
	}

	// MemberAccesses: storageListType 3
	memberAccesses := bson.A{int32(3)}
	for _, ma := range ar.MemberAccesses {
		memberAccesses = append(memberAccesses, serializeMemberAccess(ma))
	}

	ruleID := string(ar.ID)
	if ruleID == "" {
		ruleID = generateUUID()
	}

	defaultMemberAccess := string(ar.DefaultMemberAccessRights)
	if defaultMemberAccess == "" {
		defaultMemberAccess = "None"
	}

	return bson.D{
		{Key: "$ID", Value: idToBsonBinary(ruleID)},
		{Key: "$Type", Value: "DomainModels$AccessRule"},
		{Key: "AllowedModuleRoles", Value: roles},
		{Key: "AllowCreate", Value: ar.AllowCreate},
		{Key: "AllowDelete", Value: ar.AllowDelete},
		{Key: "DefaultMemberAccessRights", Value: defaultMemberAccess},
		{Key: "XPathConstraint", Value: ar.XPathConstraint},
		{Key: "XPathConstraintCaption", Value: ""},
		{Key: "Documentation", Value: ""},
		{Key: "MemberAccesses", Value: memberAccesses},
	}
}

func serializeMemberAccess(ma *domainmodel.MemberAccess) bson.D {
	maID := string(ma.ID)
	if maID == "" {
		maID = generateUUID()
	}

	doc := bson.D{
		{Key: "$ID", Value: idToBsonBinary(maID)},
		{Key: "$Type", Value: "DomainModels$MemberAccess"},
		{Key: "AccessRights", Value: string(ma.AccessRights)},
	}

	// Attribute reference (BY_NAME)
	if ma.AttributeName != "" {
		doc = append(doc, bson.E{Key: "Attribute", Value: ma.AttributeName})
	}

	// Association reference (BY_NAME)
	if ma.AssociationName != "" {
		doc = append(doc, bson.E{Key: "Association", Value: ma.AssociationName})
	}

	return doc
}

func serializeNoGeneralization(e *domainmodel.Entity, pv *version.ProjectVersion) bson.D {
	// Persistability rules for external entities, verified against Studio Pro
	// reference projects:
	//   Rest$ODataRemoteEntitySource              → Persistable=true
	//   Rest$ODataEntityTypeSource                → Persistable=false
	//   Rest$ODataPrimitiveCollectionEntitySource → Persistable=false
	persistable := e.Persistable
	switch e.Source {
	case "Rest$ODataRemoteEntitySource":
		persistable = true
	case "Rest$ODataEntityTypeSource", "Rest$ODataPrimitiveCollectionEntitySource":
		persistable = false
	}
	doc := bson.D{
		{Key: "$ID", Value: idToBsonBinary(generateUUID())},
		{Key: "$Type", Value: "DomainModels$NoGeneralization"},
		{Key: "Persistable", Value: persistable},
	}
	// Mendix >= 11.9 renamed HasOwner → HasOwnerAttr, etc.
	useAttrSuffix := pv != nil && pv.IsAtLeast(11, 9)
	ownerKey, changedByKey, changedDateKey, createdDateKey := "HasOwner", "HasChangedBy", "HasChangedDate", "HasCreatedDate"
	if useAttrSuffix {
		ownerKey, changedByKey, changedDateKey, createdDateKey = "HasOwnerAttr", "HasChangedByAttr", "HasChangedDateAttr", "HasCreatedDateAttr"
	}
	if e.HasOwner {
		doc = append(doc, bson.E{Key: ownerKey, Value: true})
	}
	if e.HasChangedBy {
		doc = append(doc, bson.E{Key: changedByKey, Value: true})
	}
	if e.HasChangedDate {
		doc = append(doc, bson.E{Key: changedDateKey, Value: true})
	}
	if e.HasCreatedDate {
		doc = append(doc, bson.E{Key: createdDateKey, Value: true})
	}
	return doc
}

func serializeGeneralization(parentRef string) bson.D {
	// Generalization stores the parent entity as a BY_NAME qualified name string
	return bson.D{
		{Key: "$ID", Value: idToBsonBinary(generateUUID())},
		{Key: "$Type", Value: "DomainModels$Generalization"},
		{Key: "Generalization", Value: parentRef},
	}
}

func serializeOqlViewEntitySource(sourceObjectID model.ID, sourceDocumentRef, oqlQuery string, pv *version.ProjectVersion) bson.D {
	id := string(sourceObjectID)
	if id == "" {
		id = generateUUID()
	}
	doc := bson.D{
		{Key: "$ID", Value: idToBsonBinary(id)},
		{Key: "$Type", Value: "DomainModels$OqlViewEntitySource"},
	}
	// Mendix 10.x stores the OQL query inline on the source object (reflection data: 10.21 has "Oql" property).
	// Mendix 11.0+ removed this field; only the ViewEntitySourceDocument stores the OQL.
	if !pv.IsAtLeast(11, 0) {
		doc = append(doc, bson.E{Key: "Oql", Value: oqlQuery})
	}
	doc = append(doc, bson.E{Key: "SourceDocument", Value: sourceDocumentRef})
	return doc
}

func serializeODataRemoteEntitySource(e *domainmodel.Entity) bson.D {
	doc := bson.D{
		{Key: "$ID", Value: idToBsonBinary(generateUUID())},
		{Key: "$Type", Value: "Rest$ODataRemoteEntitySource"},
		{Key: "Countable", Value: e.Countable},
		{Key: "Creatable", Value: e.Creatable},
		{Key: "CreateChangeLocally", Value: e.CreateChangeLocally},
		{Key: "Deletable", Value: e.Deletable},
		{Key: "EntitySet", Value: e.RemoteEntitySet},
	}

	// Key with KeyParts (storageListType 2)
	if len(e.RemoteKeyParts) > 0 {
		parts := bson.A{int32(2)}
		for _, kp := range e.RemoteKeyParts {
			parts = append(parts, serializeODataKeyPart(kp))
		}
		key := bson.D{
			{Key: "$ID", Value: idToBsonBinary(generateUUID())},
			{Key: "$Type", Value: "Rest$ODataKey"},
			{Key: "Parts", Value: parts},
		}
		doc = append(doc, bson.E{Key: "Key", Value: key})
	}

	doc = append(doc,
		bson.E{Key: "RemoteName", Value: e.RemoteEntityName},
		bson.E{Key: "SkipSupported", Value: e.SkipSupported},
		bson.E{Key: "SourceDocument", Value: e.RemoteServiceName},
		bson.E{Key: "TopSupported", Value: e.TopSupported},
	)
	return doc
}

func serializeODataKeyPart(kp *domainmodel.RemoteKeyPart) bson.D {
	// Build the type sub-document, similar to serializeAttribute's NewType
	typeName := "DomainModels$StringAttributeType"
	if kp.Type != nil {
		typeName = "DomainModels$" + kp.Type.GetTypeName() + "AttributeType"
	}
	typeDoc := bson.D{
		{Key: "$ID", Value: idToBsonBinary(generateUUID())},
		{Key: "$Type", Value: typeName},
	}
	if t, ok := kp.Type.(*domainmodel.StringAttributeType); ok {
		typeDoc = append(typeDoc, bson.E{Key: "Length", Value: t.Length})
	}

	return bson.D{
		{Key: "$ID", Value: idToBsonBinary(generateUUID())},
		{Key: "$Type", Value: "Rest$ODataKeyPart"},
		{Key: "EntityKeyPartName", Value: kp.Name},
		{Key: "Filterable", Value: true},
		{Key: "Name", Value: kp.RemoteName},
		{Key: "RemoteType", Value: kp.RemoteType},
		{Key: "Type", Value: typeDoc},
	}
}

func serializeAttribute(a *domainmodel.Attribute, isExternalEntity bool) bson.D {
	// Attribute type with its own ID - use bson.D for ordered fields
	typeName := "DomainModels$StringAttributeType"
	if a.Type != nil {
		switch a.Type.(type) {
		case *domainmodel.DateAttributeType:
			// Date is stored as DateTimeAttributeType with LocalizeDate=false
			typeName = "DomainModels$DateTimeAttributeType"
		default:
			typeName = "DomainModels$" + a.Type.GetTypeName() + "AttributeType"
		}
	}

	attrTypeID := generateUUID()
	if a.Type != nil {
		if elem, ok := a.Type.(model.Element); ok && elem.GetID() != "" {
			attrTypeID = string(elem.GetID())
		}
	}
	attrType := bson.D{
		{Key: "$ID", Value: idToBsonBinary(attrTypeID)},
		{Key: "$Type", Value: typeName},
	}
	// Add type-specific properties
	if a.Type != nil {
		switch t := a.Type.(type) {
		case *domainmodel.StringAttributeType:
			attrType = append(attrType, bson.E{Key: "Length", Value: t.Length})
		case *domainmodel.DateTimeAttributeType:
			attrType = append(attrType, bson.E{Key: "LocalizeDate", Value: t.LocalizeDate})
		case *domainmodel.DateAttributeType:
			attrType = append(attrType, bson.E{Key: "LocalizeDate", Value: false})
		case *domainmodel.EnumerationAttributeType:
			// Enumeration uses BY_NAME_REFERENCE - store as qualified name string
			enumRef := t.EnumerationRef
			if enumRef == "" && t.EnumerationID != "" {
				// Fall back to ID if no ref (though this shouldn't happen for new entities)
				enumRef = string(t.EnumerationID)
			}
			attrType = append(attrType, bson.E{Key: "Enumeration", Value: enumRef})
		}
	}

	// Determine value type: OqlViewValue, CalculatedValue, ODataMappedValue, or StoredValue
	var valueDoc bson.D
	valueID := ""
	if a.Value != nil && a.Value.ID != "" {
		valueID = string(a.Value.ID)
	}
	if valueID == "" {
		valueID = generateUUID()
	}
	if a.Value != nil && a.Value.ViewReference != "" {
		// View entity attribute - use OqlViewValue
		valueDoc = bson.D{
			{Key: "$ID", Value: idToBsonBinary(valueID)},
			{Key: "$Type", Value: "DomainModels$OqlViewValue"},
			{Key: "Reference", Value: a.Value.ViewReference},
		}
	} else if a.Value != nil && a.Value.Type == "CalculatedValue" {
		// Calculated attribute - use CalculatedValue (Microflow is ByNameReference → string)
		microflowRef := a.Value.MicroflowName
		valueDoc = bson.D{
			{Key: "$ID", Value: idToBsonBinary(generateUUID())},
			{Key: "$Type", Value: "DomainModels$CalculatedValue"},
			{Key: "Microflow", Value: microflowRef},
			{Key: "PassEntity", Value: microflowRef != ""},
		}
	} else if isExternalEntity && a.IsPrimitiveCollection {
		// Single attribute of a primitive collection NPE (e.g. TripTag.Tag)
		defaultValue := ""
		if a.Value != nil && a.Value.DefaultValue != "" {
			defaultValue = a.Value.DefaultValue
		}
		valueDoc = bson.D{
			{Key: "$ID", Value: idToBsonBinary(valueID)},
			{Key: "$Type", Value: "Rest$ODataMappedPrimitiveCollectionValue"},
			{Key: "DefaultValueDesignTime", Value: defaultValue},
			{Key: "RemoteName", Value: a.RemoteName},
			{Key: "RemoteType", Value: a.RemoteType},
		}
	} else if isExternalEntity && a.RemoteName != "" {
		// External entity attribute backed by an OData property - use ODataMappedValue
		defaultValue := ""
		if a.Value != nil && a.Value.DefaultValue != "" {
			defaultValue = a.Value.DefaultValue
		}
		valueDoc = bson.D{
			{Key: "$ID", Value: idToBsonBinary(valueID)},
			{Key: "$Type", Value: "Rest$ODataMappedValue"},
			{Key: "Creatable", Value: a.Creatable},
			{Key: "DefaultValueDesignTime", Value: defaultValue},
			{Key: "Filterable", Value: a.Filterable},
			{Key: "RemoteName", Value: a.RemoteName},
			{Key: "RemoteType", Value: a.RemoteType},
			{Key: "RepresentsStream", Value: false},
			{Key: "Sortable", Value: a.Sortable},
			{Key: "Updatable", Value: a.Updatable},
		}
	} else {
		// Regular entity attribute - use StoredValue
		defaultValue := ""
		if a.Value != nil && a.Value.DefaultValue != "" {
			defaultValue = a.Value.DefaultValue
		}
		valueDoc = bson.D{
			{Key: "$ID", Value: idToBsonBinary(valueID)},
			{Key: "$Type", Value: "DomainModels$StoredValue"},
			{Key: "DefaultValue", Value: defaultValue},
		}
	}

	// Mendix 11.12 requires "$ID" first, "$Type" second; remaining fields keep
	// Studio Pro's order (Name, Documentation, ExportLevel, GUID, NewType, Value).
	return bson.D{
		{Key: "$ID", Value: idToBsonBinary(string(a.ID))},
		{Key: "$Type", Value: "DomainModels$Attribute"},
		{Key: "Name", Value: a.Name},
		{Key: "Documentation", Value: a.Documentation},
		{Key: "ExportLevel", Value: "Hidden"},
		{Key: "GUID", Value: idToBsonBinary(string(a.ID))},
		{Key: "NewType", Value: attrType},
		{Key: "Value", Value: valueDoc},
	}
}

func serializeAssociation(a *domainmodel.Association) bson.D {
	storageFormat := string(a.StorageFormat)
	if storageFormat == "" {
		storageFormat = "Column"
	}

	var source any
	switch a.Source {
	case "Rest$ODataRemoteAssociationSource":
		nav := a.Navigability2
		if nav == "" {
			nav = "ParentToChild"
		}
		source = bson.D{
			{Key: "$ID", Value: idToBsonBinary(generateUUID())},
			{Key: "$Type", Value: "Rest$ODataRemoteAssociationSource"},
			{Key: "CreatableFromChild", Value: a.CreatableFromChild},
			{Key: "CreatableFromParent", Value: a.CreatableFromParent},
			{Key: "Navigability2", Value: nav},
			{Key: "RemoteChildNavigationProperty", Value: a.RemoteChildNavigationProperty},
			{Key: "RemoteParentNavigationProperty", Value: a.RemoteParentNavigationProperty},
			{Key: "UpdatableFromChild", Value: a.UpdatableFromChild},
			{Key: "UpdatableFromParent", Value: a.UpdatableFromParent},
		}
	case "Rest$ODataPrimitiveCollectionAssociationSource":
		// Studio Pro emits this with no extra fields — it's a marker that
		// pairs with Rest$ODataPrimitiveCollectionEntitySource on the child.
		source = bson.D{
			{Key: "$ID", Value: idToBsonBinary(generateUUID())},
			{Key: "$Type", Value: "Rest$ODataPrimitiveCollectionAssociationSource"},
		}
	default:
		source = nil
	}

	return bson.D{
		{Key: "$ID", Value: idToBsonBinary(string(a.ID))},
		{Key: "$Type", Value: "DomainModels$Association"},
		{Key: "Name", Value: a.Name},
		{Key: "Documentation", Value: a.Documentation},
		{Key: "ExportLevel", Value: "Hidden"},
		{Key: "GUID", Value: idToBsonBinary(string(a.ID))},
		{Key: "ParentPointer", Value: idToBsonBinary(string(a.ParentID))},
		{Key: "ChildPointer", Value: idToBsonBinary(string(a.ChildID))},
		{Key: "Type", Value: string(a.Type)},
		{Key: "Owner", Value: string(a.Owner)},
		{Key: "ParentConnection", Value: "0;50"},
		{Key: "ChildConnection", Value: "100;50"},
		{Key: "StorageFormat", Value: storageFormat},
		{Key: "DeleteBehavior", Value: serializeDeleteBehavior(a.ParentDeleteBehavior, a.ChildDeleteBehavior)},
		{Key: "Source", Value: source},
	}
}

func serializeCrossAssociation(ca *domainmodel.CrossModuleAssociation) bson.D {
	storageFormat := string(ca.StorageFormat)
	if storageFormat == "" {
		storageFormat = "Column"
	}
	// CrossAssociation does NOT have ParentConnection/ChildConnection properties
	// (unlike Association). Writing them causes Studio Pro to crash with
	// InvalidOperationException in MprProperty..ctor.
	return bson.D{
		{Key: "$ID", Value: idToBsonBinary(string(ca.ID))},
		{Key: "$Type", Value: "DomainModels$CrossAssociation"},
		{Key: "Name", Value: ca.Name},
		{Key: "Documentation", Value: ca.Documentation},
		{Key: "ExportLevel", Value: "Hidden"},
		{Key: "GUID", Value: idToBsonBinary(string(ca.ID))},
		{Key: "ParentPointer", Value: idToBsonBinary(string(ca.ParentID))},
		{Key: "Child", Value: ca.ChildRef},
		{Key: "Type", Value: string(ca.Type)},
		{Key: "Owner", Value: string(ca.Owner)},
		{Key: "StorageFormat", Value: storageFormat},
		{Key: "Source", Value: nil},
		{Key: "DeleteBehavior", Value: serializeDeleteBehavior(ca.ParentDeleteBehavior, ca.ChildDeleteBehavior)},
	}
}

func serializeDeleteBehavior(parentBehavior, childBehavior *domainmodel.DeleteBehavior) bson.D {
	parentType := "DeleteMeButKeepReferences"
	childType := "DeleteMeButKeepReferences"

	if parentBehavior != nil && parentBehavior.Type != "" {
		parentType = string(parentBehavior.Type)
	}
	if childBehavior != nil && childBehavior.Type != "" {
		childType = string(childBehavior.Type)
	}

	return bson.D{
		{Key: "$ID", Value: idToBsonBinary(generateUUID())},
		{Key: "$Type", Value: "DomainModels$DeleteBehavior"},
		{Key: "ChildDeleteBehavior", Value: childType},
		{Key: "ChildErrorMessage", Value: nil},
		{Key: "ParentDeleteBehavior", Value: parentType},
		{Key: "ParentErrorMessage", Value: nil},
	}
}

// zeroGUID is the all-zero UUID Studio Pro writes for an unset GUID reference.
const zeroGUID = "00000000-0000-0000-0000-000000000000"

func serializeIndex(idx *domainmodel.Index) bson.D {
	// IndexedAttribute lists use typed-array marker 2 (NOT the domain-model
	// default of 3) — verified against real Studio-Pro 11.x BSON
	// (mx-test-projects/test7-app: IdxProbe).
	attrs := bson.A{int32(2)}
	for _, ia := range idx.Attributes {
		attrs = append(attrs, serializeIndexAttribute(ia))
	}

	return bson.D{
		{Key: "$ID", Value: idToBsonBinary(string(idx.ID))},
		{Key: "$Type", Value: "DomainModels$EntityIndex"},
		{Key: "Attributes", Value: attrs},
		{Key: "GUID", Value: idToBsonBinary(string(idx.ID))},
		{Key: "IncludeInOffline", Value: false},
	}
}

// serializeIndexAttribute emits the Studio-Pro 11.x index-segment shape:
// Ascending(bool)+Type("Normal")+AttributePointer, plus an all-zero
// AssociationPointer for an attribute-based segment. This replaces the stale
// "SortOrder" string the writer previously emitted — the legacy parser already
// reads Ascending (with a SortOrder fallback), so writer and parser are now
// aligned, and the output matches what Studio Pro produces.
func serializeIndexAttribute(ia *domainmodel.IndexAttribute) bson.D {
	return bson.D{
		{Key: "$ID", Value: idToBsonBinary(string(ia.ID))},
		{Key: "$Type", Value: "DomainModels$IndexedAttribute"},
		{Key: "AttributePointer", Value: idToBsonBinary(string(ia.AttributeID))}, // BSON Binary like $ID
		{Key: "AssociationPointer", Value: idToBsonBinary(zeroGUID)},             // zero GUID: attribute-based segment
		{Key: "Ascending", Value: ia.Ascending},
		{Key: "Type", Value: "Normal"},
	}
}

func serializeValidationRule(vr *domainmodel.ValidationRule, moduleName string, entity *domainmodel.Entity) bson.D {
	// Look up attribute name from the entity's attributes using AttributeID
	// The Attribute field uses BY_NAME_REFERENCE, so it must be a qualified name STRING
	// Format: "ModuleName.EntityName.AttributeName"
	//
	// NOTE: AttributeID can be either:
	// 1. A UUID (when entity was just created) - compare with attr.ID
	// 2. A qualified name string (when entity was read from disk) - extract attr name and compare
	attributeQualifiedName := ""
	attrIDStr := string(vr.AttributeID)

	// Check if AttributeID is already a qualified name (contains dots)
	if strings.Contains(attrIDStr, ".") {
		// It's already a qualified name - use it directly
		attributeQualifiedName = attrIDStr
	} else {
		// It's a UUID - look up the attribute name
		for _, attr := range entity.Attributes {
			if attr.ID == vr.AttributeID {
				attributeQualifiedName = fmt.Sprintf("%s.%s.%s", moduleName, entity.Name, attr.Name)
				break
			}
		}
	}

	// Use bson.D (ordered document) to match Studio Pro's field order:
	// $ID, $Type, Attribute, Message, RuleInfo
	doc := bson.D{
		{Key: "$ID", Value: idToBsonBinary(string(vr.ID))},
		{Key: "$Type", Value: "DomainModels$ValidationRule"},
		{Key: "Attribute", Value: attributeQualifiedName}, // BY_NAME_REFERENCE: qualified name STRING
	}

	// Message comes before RuleInfo in Studio Pro's format
	if vr.ErrorMessage != nil && len(vr.ErrorMessage.Translations) > 0 {
		doc = append(doc, bson.E{Key: "Message", Value: serializeText(vr.ErrorMessage)})
	}

	// RuleInfo comes last
	doc = append(doc, bson.E{Key: "RuleInfo", Value: serializeRuleInfo(vr.Type)})

	return doc
}

func serializeRuleInfo(ruleType string) bson.D {
	// Use bson.D (ordered document) - Studio Pro uses $ID first, then $Type
	switch ruleType {
	case "Required":
		return bson.D{
			{Key: "$ID", Value: idToBsonBinary(generateUUID())},
			{Key: "$Type", Value: "DomainModels$RequiredRuleInfo"},
		}
	case "Unique":
		return bson.D{
			{Key: "$ID", Value: idToBsonBinary(generateUUID())},
			{Key: "$Type", Value: "DomainModels$UniqueRuleInfo"},
		}
	default:
		// Fallback to required
		return bson.D{
			{Key: "$ID", Value: idToBsonBinary(generateUUID())},
			{Key: "$Type", Value: "DomainModels$RequiredRuleInfo"},
		}
	}
}

func serializeText(text *model.Text) bson.D {
	// Translations as Items array with version prefix 3
	// Use bson.D for ordered documents to match Studio Pro format
	items := bson.A{int32(3)}
	// Sort language keys for deterministic output
	langs := make([]string, 0, len(text.Translations))
	for lang := range text.Translations {
		langs = append(langs, lang)
	}
	sort.Strings(langs)
	for _, lang := range langs {
		value := text.Translations[lang]
		items = append(items, bson.D{
			{Key: "$ID", Value: idToBsonBinary(generateUUID())},
			{Key: "$Type", Value: "Texts$Translation"},
			{Key: "LanguageCode", Value: lang},
			{Key: "Text", Value: value},
		})
	}

	// Studio Pro order: $ID, $Type, Items
	return bson.D{
		{Key: "$ID", Value: idToBsonBinary(string(text.ID))},
		{Key: "$Type", Value: "Texts$Text"},
		{Key: "Items", Value: items},
	}
}
