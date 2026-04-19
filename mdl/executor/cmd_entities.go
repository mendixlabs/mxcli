// SPDX-License-Identifier: Apache-2.0

// Package executor - Entity commands (SHOW/DESCRIBE/CREATE/DROP ENTITY)
package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// execCreateEntity handles CREATE ENTITY statements.
// buildEventHandlers converts a list of AST EventHandlerDef to domain model EventHandler.
// Resolves the microflow qualified name to a microflow ID, but stores the qualified name
// for BY_NAME serialization.
func buildEventHandlers(ctx *ExecContext, defs []ast.EventHandlerDef) ([]*domainmodel.EventHandler, error) {
	if len(defs) == 0 {
		return nil, nil
	}
	var handlers []*domainmodel.EventHandler
	for _, d := range defs {
		mfQN := d.Microflow.String()
		mfID, err := resolveMicroflowByName(ctx, mfQN)
		if err != nil {
			return nil, mdlerrors.NewNotFound("microflow", mfQN)
		}
		handlers = append(handlers, &domainmodel.EventHandler{
			BaseElement: model.BaseElement{
				ID:       model.ID(types.GenerateID()),
				TypeName: "DomainModels$EventHandler",
			},
			Moment:            domainmodel.EventMoment(d.Moment),
			Event:             domainmodel.EventType(d.Event),
			MicroflowID:       mfID,
			MicroflowName:     mfQN,
			RaiseErrorOnFalse: d.RaiseErrorOnFalse,
			PassEventObject:   d.PassEventObject,
		})
	}
	return handlers, nil
}

func execCreateEntity(ctx *ExecContext, s *ast.CreateEntityStmt) error {
	e := ctx.executor
	if !ctx.Connected() {
		return mdlerrors.NewNotConnected()
	}

	// Find or auto-create module
	module, err := findOrCreateModule(ctx, s.Name.Module)
	if err != nil {
		return err
	}

	// Get domain model
	dm, err := ctx.Backend.GetDomainModel(module.ID)
	if err != nil {
		return mdlerrors.NewBackend("get domain model", err)
	}

	// Check if entity already exists
	var existingEntity *domainmodel.Entity
	for _, entity := range dm.Entities {
		if entity.Name == s.Name.Name {
			existingEntity = entity
			break
		}
	}

	// If entity exists and not using CREATE OR MODIFY, return error
	if existingEntity != nil && !s.CreateOrModify {
		return mdlerrors.NewAlreadyExistsMsg("entity", s.Name.Module+"."+s.Name.Name, fmt.Sprintf("entity already exists: %s.%s (use CREATE OR MODIFY to update)", s.Name.Module, s.Name.Name))
	}

	// Calculate position
	var location model.Point
	if s.Position != nil {
		location = model.Point{X: s.Position.X, Y: s.Position.Y}
	} else if existingEntity != nil {
		location = existingEntity.Location
	} else {
		// Auto-position based on existing entities
		location = model.Point{X: 100 + len(dm.Entities)*150, Y: 100}
	}

	// Determine persistable based on entity kind
	persistable := s.Kind != ast.EntityNonPersistent

	// Auto-default Boolean attributes to false if no DEFAULT specified
	for i := range s.Attributes {
		if s.Attributes[i].Type.Kind == ast.TypeBoolean && !s.Attributes[i].HasDefault {
			s.Attributes[i].HasDefault = true
			s.Attributes[i].DefaultValue = false
		}
	}

	// Create attributes and build name-to-ID map for validation rules and indexes
	// Also detect pseudo-types (AutoOwner, AutoChangedBy, etc.) and set entity flags
	var storeOwner, storeChangedBy, storeCreatedDate, storeChangedDate bool

	var attrs []*domainmodel.Attribute
	attrNameToID := make(map[string]model.ID)
	for _, a := range s.Attributes {
		// Pseudo-types: set entity flags instead of creating attributes
		switch a.Type.Kind {
		case ast.TypeAutoOwner:
			storeOwner = true
			continue
		case ast.TypeAutoChangedBy:
			storeChangedBy = true
			continue
		case ast.TypeAutoCreatedDate:
			storeCreatedDate = true
			continue
		case ast.TypeAutoChangedDate:
			storeChangedDate = true
			continue
		}

		// CALCULATED attributes are only supported on persistent entities
		if a.Calculated && !persistable {
			return mdlerrors.NewValidationf("attribute '%s': CALCULATED attributes are only supported on persistent entities", a.Name)
		}

		// Use Documentation if available, fall back to Comment
		doc := a.Documentation
		if doc == "" {
			doc = a.Comment
		}

		// Generate ID for the attribute so we can reference it in validation rules/indexes
		attrID := model.ID(types.GenerateID())
		attrNameToID[a.Name] = attrID

		attr := &domainmodel.Attribute{
			Name:          a.Name,
			Documentation: doc,
			Type:          convertDataType(a.Type),
		}
		attr.ID = attrID

		// Value type: CALCULATED or DEFAULT
		if a.Calculated {
			attrValue := &domainmodel.AttributeValue{
				Type: "CalculatedValue",
			}
			if a.CalculatedMicroflow != nil {
				mfID, err := resolveMicroflowByName(ctx, a.CalculatedMicroflow.String())
				if err != nil {
					return mdlerrors.NewBackend(fmt.Sprintf("attribute '%s'", a.Name), err)
				}
				attrValue.MicroflowID = mfID
				attrValue.MicroflowName = a.CalculatedMicroflow.String()
			}
			attr.Value = attrValue
		} else if a.HasDefault {
			defaultStr := fmt.Sprintf("%v", a.DefaultValue)
			// For enum attributes, Mendix stores just the value name (e.g., "Open"),
			// not the fully qualified name. The EnumerationRef already provides context.
			// Strip the enum prefix if the default is fully qualified.
			if a.Type.Kind == ast.TypeEnumeration && a.Type.EnumRef != nil {
				enumPrefix := a.Type.EnumRef.String() + "."
				if strings.HasPrefix(defaultStr, enumPrefix) {
					defaultStr = strings.TrimPrefix(defaultStr, enumPrefix)
				}
			}
			attr.Value = &domainmodel.AttributeValue{
				DefaultValue: defaultStr,
			}
		}

		attrs = append(attrs, attr)
	}

	// Create validation rules for NOT NULL and UNIQUE constraints
	var validationRules []*domainmodel.ValidationRule
	for _, a := range s.Attributes {
		attrID := attrNameToID[a.Name]

		// NOT NULL -> Required validation rule
		if a.NotNull {
			vr := &domainmodel.ValidationRule{
				AttributeID: attrID,
				Type:        "Required",
			}
			vr.ID = model.ID(types.GenerateID())
			if a.NotNullError != "" {
				vr.ErrorMessage = &model.Text{
					Translations: map[string]string{"en_US": a.NotNullError},
				}
				vr.ErrorMessage.ID = model.ID(types.GenerateID())
			}
			validationRules = append(validationRules, vr)
		}

		// UNIQUE -> Unique validation rule
		if a.Unique {
			vr := &domainmodel.ValidationRule{
				AttributeID: attrID,
				Type:        "Unique",
			}
			vr.ID = model.ID(types.GenerateID())
			if a.UniqueError != "" {
				vr.ErrorMessage = &model.Text{
					Translations: map[string]string{"en_US": a.UniqueError},
				}
				vr.ErrorMessage.ID = model.ID(types.GenerateID())
			}
			validationRules = append(validationRules, vr)
		}
	}

	// Create indexes
	var indexes []*domainmodel.Index
	for _, idx := range s.Indexes {
		idxID := model.ID(types.GenerateID())
		var indexAttrs []*domainmodel.IndexAttribute
		for _, col := range idx.Columns {
			if attrID, ok := attrNameToID[col.Name]; ok {
				iaID := model.ID(types.GenerateID())
				ia := &domainmodel.IndexAttribute{
					AttributeID: attrID,
					Ascending:   !col.Descending,
				}
				ia.ID = iaID
				indexAttrs = append(indexAttrs, ia)
			}
		}
		if len(indexAttrs) > 0 {
			index := &domainmodel.Index{
				Attributes: indexAttrs,
			}
			index.ID = idxID
			indexes = append(indexes, index)
		}
	}

	// Create entity
	// Build event handlers
	eventHandlers, err := buildEventHandlers(ctx, s.EventHandlers)
	if err != nil {
		return err
	}

	entity := &domainmodel.Entity{
		Name:            s.Name.Name,
		Documentation:   s.Documentation,
		Location:        location,
		Persistable:     persistable,
		Attributes:      attrs,
		ValidationRules: validationRules,
		Indexes:         indexes,
		EventHandlers:   eventHandlers,
		HasOwner:        storeOwner,
		HasChangedBy:    storeChangedBy,
		HasCreatedDate:  storeCreatedDate,
		HasChangedDate:  storeChangedDate,
	}

	// Set generalization (inheritance) if specified
	if s.Generalization != nil {
		entity.GeneralizationRef = s.Generalization.String()
	}

	if s.CreateOrModify && existingEntity != nil {
		// Update existing entity
		entity.ID = existingEntity.ID
		if err := ctx.Backend.UpdateEntity(dm.ID, entity); err != nil {
			return mdlerrors.NewBackend("update entity", err)
		}
		// Invalidate caches so updated entity is visible
		invalidateHierarchy(ctx)
		invalidateDomainModelsCache(ctx)
		fmt.Fprintf(ctx.Output, "Modified entity: %s\n", s.Name)
	} else {
		// Create new entity
		if err := ctx.Backend.CreateEntity(dm.ID, entity); err != nil {
			return mdlerrors.NewBackend("create entity", err)
		}
		// Invalidate caches so new entity is visible
		invalidateHierarchy(ctx)
		invalidateDomainModelsCache(ctx)
		fmt.Fprintf(ctx.Output, "Created entity: %s\n", s.Name)
	}

	e.trackModifiedDomainModel(module.ID, module.Name)
	return nil
}

// execCreateViewEntity handles CREATE VIEW ENTITY statements.
func execCreateViewEntity(ctx *ExecContext, s *ast.CreateViewEntityStmt) error {
	if !ctx.Connected() {
		return mdlerrors.NewNotConnected()
	}

	// Version pre-check
	if err := checkFeature(ctx, "domain_model", "view_entities",
		"CREATE VIEW ENTITY",
		"upgrade your project to 10.18+ or use a regular entity with a microflow data source"); err != nil {
		return err
	}

	// Validate OQL syntax before creating the view entity
	// This prevents creating view entities that will crash Studio Pro
	if s.Query.RawQuery != "" {
		if oqlViolations := ValidateOQLSyntax(s.Query.RawQuery); len(oqlViolations) > 0 {
			var msgs []string
			for _, v := range oqlViolations {
				msgs = append(msgs, v.Message)
			}
			return mdlerrors.NewValidationf("invalid OQL in view entity '%s':\n  - %s",
				s.Name.String(), strings.Join(msgs, "\n  - "))
		}
	}

	// Find module
	module, err := findModule(ctx, s.Name.Module)
	if err != nil {
		return err
	}

	// Get domain model
	dm, err := ctx.Backend.GetDomainModel(module.ID)
	if err != nil {
		return mdlerrors.NewBackend("get domain model", err)
	}

	// Check if entity already exists
	var existingEntity *domainmodel.Entity
	for _, entity := range dm.Entities {
		if entity.Name == s.Name.Name {
			existingEntity = entity
			break
		}
	}

	// If entity exists: REPLACE drops and recreates, MODIFY updates in place
	if existingEntity != nil && !s.CreateOrModify && !s.CreateOrReplace {
		return mdlerrors.NewAlreadyExistsMsg("entity", s.Name.Module+"."+s.Name.Name, fmt.Sprintf("entity already exists: %s.%s (use CREATE OR MODIFY to update, or CREATE OR REPLACE to drop and recreate)", s.Name.Module, s.Name.Name))
	}

	// CREATE OR REPLACE: delete existing entity and source doc, then recreate
	if existingEntity != nil && s.CreateOrReplace {
		// Preserve location from old entity if no explicit position
		if s.Position == nil {
			s.Position = &ast.Position{X: existingEntity.Location.X, Y: existingEntity.Location.Y}
		}
		// Delete ViewEntitySourceDocument
		if err := ctx.Backend.DeleteViewEntitySourceDocumentByName(s.Name.Module, s.Name.Name); err != nil {
			return mdlerrors.NewBackend("delete existing ViewEntitySourceDocument", err)
		}
		// Delete the entity itself
		if err := ctx.Backend.DeleteEntity(dm.ID, existingEntity.ID); err != nil {
			return mdlerrors.NewBackend("delete existing entity for replace", err)
		}
		existingEntity = nil
		// Re-fetch domain model after deletion so entity count is correct for positioning
		dm, err = ctx.Backend.GetDomainModel(module.ID)
		if err != nil {
			return mdlerrors.NewBackend("get domain model after delete", err)
		}
	}

	// Calculate position
	var location model.Point
	if s.Position != nil {
		location = model.Point{X: s.Position.X, Y: s.Position.Y}
	} else if existingEntity != nil {
		location = existingEntity.Location
	} else {
		location = model.Point{X: 100 + len(dm.Entities)*150, Y: 100}
	}

	// Create or update ViewEntitySourceDocument (separate document for OQL query)
	sourceDocRef := s.Name.Module + "." + s.Name.Name
	// Always delete any existing ViewEntitySourceDocument before creating a new one.
	// This prevents duplicate OQL documents from accumulating (e.g., from re-running
	// scripts or after a previous DROP that didn't clean up properly).
	if err := ctx.Backend.DeleteViewEntitySourceDocumentByName(s.Name.Module, s.Name.Name); err != nil {
		return mdlerrors.NewBackend("delete existing ViewEntitySourceDocument", err)
	}
	_, err = ctx.Backend.CreateViewEntitySourceDocument(
		module.ID,
		s.Name.Module,
		s.Name.Name,
		s.Query.RawQuery,
		s.Documentation,
	)
	if err != nil {
		return mdlerrors.NewBackend("create ViewEntitySourceDocument", err)
	}

	// Create view attributes with OqlViewValue references.
	// Preserve existing attribute and value IDs on modify to avoid CE-6770.
	existingAttrsByName := make(map[string]*domainmodel.Attribute)
	if existingEntity != nil {
		for _, ea := range existingEntity.Attributes {
			existingAttrsByName[ea.Name] = ea
		}
	}
	var attrs []*domainmodel.Attribute
	for _, a := range s.Attributes {
		attr := &domainmodel.Attribute{
			Name: a.Name,
			Type: convertDataType(a.Type),
			Value: &domainmodel.AttributeValue{
				ViewReference: a.Name, // OQL column alias matches attribute name
			},
		}
		// Preserve IDs from existing attribute with same name
		if ea, ok := existingAttrsByName[a.Name]; ok {
			attr.ID = ea.ID
			if ea.Value != nil {
				attr.Value.ID = ea.Value.ID
			}
		}
		attrs = append(attrs, attr)
	}

	// Create view entity with source document reference.
	// View entities use Persistable=true because they are retrievable from database (via OQL).
	// Studio Pro treats view entities as persistable for database retrieval purposes.
	entity := &domainmodel.Entity{
		Name:              s.Name.Name,
		Documentation:     s.Documentation,
		Location:          location,
		Persistable:       true,
		Attributes:        attrs,
		Source:            "DomainModels$OqlViewEntitySource",
		SourceDocumentRef: sourceDocRef,
		OqlQuery:          s.Query.RawQuery,
	}

	if s.CreateOrModify && existingEntity != nil {
		// Update existing entity — preserve Source object ID to avoid CE-6770
		entity.ID = existingEntity.ID
		entity.SourceObjectID = existingEntity.SourceObjectID
		if err := ctx.Backend.UpdateEntity(dm.ID, entity); err != nil {
			return mdlerrors.NewBackend("update view entity", err)
		}
		// Invalidate caches so updated entity is visible
		invalidateHierarchy(ctx)
		invalidateDomainModelsCache(ctx)
		fmt.Fprintf(ctx.Output, "Modified view entity: %s\n", s.Name)
	} else {
		// Create new entity
		if err := ctx.Backend.CreateEntity(dm.ID, entity); err != nil {
			return mdlerrors.NewBackend("create view entity", err)
		}
		// Invalidate caches so new entity is visible
		invalidateHierarchy(ctx)
		invalidateDomainModelsCache(ctx)
		fmt.Fprintf(ctx.Output, "Created view entity: %s\n", s.Name)
	}

	return nil
}

// execAlterEntity handles ALTER ENTITY statements.
func execAlterEntity(ctx *ExecContext, s *ast.AlterEntityStmt) error {
	e := ctx.executor
	if !ctx.Connected() {
		return mdlerrors.NewNotConnected()
	}

	// Find module
	module, err := findModule(ctx, s.Name.Module)
	if err != nil {
		return err
	}

	// Get domain model
	dm, err := ctx.Backend.GetDomainModel(module.ID)
	if err != nil {
		return mdlerrors.NewBackend("get domain model", err)
	}

	// Find entity
	var entity *domainmodel.Entity
	for _, ent := range dm.Entities {
		if ent.Name == s.Name.Name {
			entity = ent
			break
		}
	}
	if entity == nil {
		return mdlerrors.NewNotFoundMsg("entity", fmt.Sprint(s.Name), fmt.Sprintf("entity not found: %s", s.Name))
	}

	switch s.Operation {
	case ast.AlterEntityAddAttribute:
		a := s.Attribute
		if a == nil {
			return mdlerrors.NewValidation("no attribute definition provided")
		}
		// Pseudo-types: set entity flags instead of adding real attributes
		switch a.Type.Kind {
		case ast.TypeAutoOwner:
			entity.HasOwner = true
			return ctx.Backend.UpdateEntity(dm.ID, entity)
		case ast.TypeAutoChangedBy:
			entity.HasChangedBy = true
			return ctx.Backend.UpdateEntity(dm.ID, entity)
		case ast.TypeAutoCreatedDate:
			entity.HasCreatedDate = true
			return ctx.Backend.UpdateEntity(dm.ID, entity)
		case ast.TypeAutoChangedDate:
			entity.HasChangedDate = true
			return ctx.Backend.UpdateEntity(dm.ID, entity)
		}
		// CALCULATED attributes are only supported on persistent entities
		if a.Calculated && !entity.Persistable {
			return mdlerrors.NewValidationf("attribute '%s': CALCULATED attributes are only supported on persistent entities", a.Name)
		}
		// Auto-default Boolean attributes to false if no DEFAULT specified
		if a.Type.Kind == ast.TypeBoolean && !a.HasDefault {
			a.HasDefault = true
			a.DefaultValue = false
		}
		// Check for duplicate attribute name
		for _, existing := range entity.Attributes {
			if existing.Name == a.Name {
				return mdlerrors.NewAlreadyExistsMsg("attribute", a.Name, fmt.Sprintf("attribute '%s' already exists on entity %s", a.Name, s.Name))
			}
		}

		attrID := model.ID(types.GenerateID())
		attr := &domainmodel.Attribute{
			Name:          a.Name,
			Documentation: a.Documentation,
			Type:          convertDataType(a.Type),
		}
		attr.ID = attrID
		if a.Calculated {
			attrValue := &domainmodel.AttributeValue{
				Type: "CalculatedValue",
			}
			if a.CalculatedMicroflow != nil {
				mfID, err := resolveMicroflowByName(ctx, a.CalculatedMicroflow.String())
				if err != nil {
					return mdlerrors.NewBackend(fmt.Sprintf("attribute '%s'", a.Name), err)
				}
				attrValue.MicroflowID = mfID
				attrValue.MicroflowName = a.CalculatedMicroflow.String()
			}
			attr.Value = attrValue
		} else if a.HasDefault {
			defaultStr := fmt.Sprintf("%v", a.DefaultValue)
			if a.Type.Kind == ast.TypeEnumeration && a.Type.EnumRef != nil {
				enumPrefix := a.Type.EnumRef.String() + "."
				if strings.HasPrefix(defaultStr, enumPrefix) {
					defaultStr = strings.TrimPrefix(defaultStr, enumPrefix)
				}
			}
			attr.Value = &domainmodel.AttributeValue{
				DefaultValue: defaultStr,
			}
		}
		entity.Attributes = append(entity.Attributes, attr)

		// Add validation rules for NOT NULL and UNIQUE
		if a.NotNull {
			vr := &domainmodel.ValidationRule{
				AttributeID: attrID,
				Type:        "Required",
			}
			vr.ID = model.ID(types.GenerateID())
			if a.NotNullError != "" {
				vr.ErrorMessage = &model.Text{
					Translations: map[string]string{"en_US": a.NotNullError},
				}
				vr.ErrorMessage.ID = model.ID(types.GenerateID())
			}
			entity.ValidationRules = append(entity.ValidationRules, vr)
		}
		if a.Unique {
			vr := &domainmodel.ValidationRule{
				AttributeID: attrID,
				Type:        "Unique",
			}
			vr.ID = model.ID(types.GenerateID())
			if a.UniqueError != "" {
				vr.ErrorMessage = &model.Text{
					Translations: map[string]string{"en_US": a.UniqueError},
				}
				vr.ErrorMessage.ID = model.ID(types.GenerateID())
			}
			entity.ValidationRules = append(entity.ValidationRules, vr)
		}

		if err := ctx.Backend.UpdateEntity(dm.ID, entity); err != nil {
			return mdlerrors.NewBackend("add attribute", err)
		}
		invalidateHierarchy(ctx)
		invalidateDomainModelsCache(ctx)
		fmt.Fprintf(ctx.Output, "Added attribute '%s' to entity %s\n", a.Name, s.Name)

	case ast.AlterEntityRenameAttribute:
		found := false
		for _, attr := range entity.Attributes {
			if attr.Name == s.AttributeName {
				attr.Name = s.NewName
				found = true
				break
			}
		}
		if !found {
			return mdlerrors.NewNotFoundMsg("attribute", s.AttributeName, fmt.Sprintf("attribute '%s' not found on entity %s", s.AttributeName, s.Name))
		}
		if err := ctx.Backend.UpdateEntity(dm.ID, entity); err != nil {
			return mdlerrors.NewBackend("rename attribute", err)
		}
		invalidateHierarchy(ctx)
		invalidateDomainModelsCache(ctx)
		fmt.Fprintf(ctx.Output, "Renamed attribute '%s' to '%s' on entity %s\n", s.AttributeName, s.NewName, s.Name)

	case ast.AlterEntityModifyAttribute:
		// CALCULATED attributes are only supported on persistent entities
		if s.Calculated && !entity.Persistable {
			return mdlerrors.NewValidationf("attribute '%s': CALCULATED attributes are only supported on persistent entities", s.AttributeName)
		}
		found := false
		for _, attr := range entity.Attributes {
			if attr.Name == s.AttributeName {
				attr.Type = convertDataType(s.DataType)
				if s.Calculated {
					attrValue := &domainmodel.AttributeValue{
						Type: "CalculatedValue",
					}
					if s.CalculatedMicroflow != nil {
						mfID, err := resolveMicroflowByName(ctx, s.CalculatedMicroflow.String())
						if err != nil {
							return mdlerrors.NewBackend(fmt.Sprintf("attribute '%s'", s.AttributeName), err)
						}
						attrValue.MicroflowID = mfID
						attrValue.MicroflowName = s.CalculatedMicroflow.String()
					}
					attr.Value = attrValue
				}
				found = true
				break
			}
		}
		if !found {
			return mdlerrors.NewNotFoundMsg("attribute", s.AttributeName, fmt.Sprintf("attribute '%s' not found on entity %s", s.AttributeName, s.Name))
		}
		if err := ctx.Backend.UpdateEntity(dm.ID, entity); err != nil {
			return mdlerrors.NewBackend("modify attribute", err)
		}
		invalidateHierarchy(ctx)
		invalidateDomainModelsCache(ctx)
		fmt.Fprintf(ctx.Output, "Modified attribute '%s' on entity %s\n", s.AttributeName, s.Name)

	case ast.AlterEntityDropAttribute:
		// System attribute pseudo-names: drop by clearing entity flags
		switch strings.ToLower(s.AttributeName) {
		case "owner":
			if entity.HasOwner {
				entity.HasOwner = false
				return ctx.Backend.UpdateEntity(dm.ID, entity)
			}
		case "changedby":
			if entity.HasChangedBy {
				entity.HasChangedBy = false
				return ctx.Backend.UpdateEntity(dm.ID, entity)
			}
		case "createddate":
			if entity.HasCreatedDate {
				entity.HasCreatedDate = false
				return ctx.Backend.UpdateEntity(dm.ID, entity)
			}
		case "changeddate":
			if entity.HasChangedDate {
				entity.HasChangedDate = false
				return ctx.Backend.UpdateEntity(dm.ID, entity)
			}
		}

		idx := -1
		for i, attr := range entity.Attributes {
			if attr.Name == s.AttributeName {
				idx = i
				break
			}
		}
		if idx < 0 {
			return mdlerrors.NewNotFoundMsg("attribute", s.AttributeName, fmt.Sprintf("attribute '%s' not found on entity %s", s.AttributeName, s.Name))
		}
		// Clean up entity-level references to the dropped attribute
		droppedID := entity.Attributes[idx].ID

		// Track what gets cleaned up for reporting
		origValidationCount := len(entity.ValidationRules)
		origIndexCount := len(entity.Indexes)

		// Remove validation rules that reference this attribute
		var keepRules []*domainmodel.ValidationRule
		for _, vr := range entity.ValidationRules {
			if vr.AttributeID != droppedID {
				keepRules = append(keepRules, vr)
			}
		}
		entity.ValidationRules = keepRules

		// Remove MemberAccess entries from access rules that reference this attribute
		removedMemberAccess := 0
		for _, rule := range entity.AccessRules {
			var keepMembers []*domainmodel.MemberAccess
			for _, ma := range rule.MemberAccesses {
				if ma.AttributeID != droppedID {
					keepMembers = append(keepMembers, ma)
				} else {
					removedMemberAccess++
				}
			}
			rule.MemberAccesses = keepMembers
		}

		// Remove index attributes that reference this attribute, and drop empty indexes
		var keepIndexes []*domainmodel.Index
		for _, idx := range entity.Indexes {
			var keepAttrs []*domainmodel.IndexAttribute
			for _, ia := range idx.Attributes {
				if ia.AttributeID != droppedID {
					keepAttrs = append(keepAttrs, ia)
				}
			}
			idx.Attributes = keepAttrs

			var keepIDs []model.ID
			for _, id := range idx.AttributeIDs {
				if id != droppedID {
					keepIDs = append(keepIDs, id)
				}
			}
			idx.AttributeIDs = keepIDs

			if len(idx.Attributes) > 0 || len(idx.AttributeIDs) > 0 {
				keepIndexes = append(keepIndexes, idx)
			}
		}
		entity.Indexes = keepIndexes

		// Remove the attribute
		entity.Attributes = append(entity.Attributes[:idx], entity.Attributes[idx+1:]...)
		if err := ctx.Backend.UpdateEntity(dm.ID, entity); err != nil {
			return mdlerrors.NewBackend("drop attribute", err)
		}
		invalidateHierarchy(ctx)
		invalidateDomainModelsCache(ctx)
		fmt.Fprintf(ctx.Output, "Dropped attribute '%s' from entity %s\n", s.AttributeName, s.Name)

		// Report what was cleaned up on the entity itself
		if n := origValidationCount - len(keepRules); n > 0 {
			fmt.Fprintf(ctx.Output, "  Removed %d validation rule(s)\n", n)
		}
		if removedMemberAccess > 0 {
			fmt.Fprintf(ctx.Output, "  Removed %d access rule member reference(s)\n", removedMemberAccess)
		}
		if n := origIndexCount - len(keepIndexes); n > 0 {
			fmt.Fprintf(ctx.Output, "  Removed %d index(es)\n", n)
		}

		// Warn about references in other documents that are NOT auto-cleaned
		entityQName := s.Name.String()
		fmt.Fprintf(ctx.Output, "  Warning: pages, microflows, and other documents may still reference '%s'. Update them manually.\n", s.AttributeName)
		fmt.Fprintf(ctx.Output, "  Use SHOW REFERENCES TO %s to find usages (requires REFRESH CATALOG FULL).\n", entityQName)

	case ast.AlterEntitySetDocumentation:
		entity.Documentation = s.Documentation
		if err := ctx.Backend.UpdateEntity(dm.ID, entity); err != nil {
			return mdlerrors.NewBackend("set documentation", err)
		}
		invalidateDomainModelsCache(ctx)
		fmt.Fprintf(ctx.Output, "Set documentation on entity %s\n", s.Name)

	case ast.AlterEntitySetComment:
		// Comments are stored as documentation in the Mendix model
		entity.Documentation = s.Comment
		if err := ctx.Backend.UpdateEntity(dm.ID, entity); err != nil {
			return mdlerrors.NewBackend("set comment", err)
		}
		invalidateDomainModelsCache(ctx)
		fmt.Fprintf(ctx.Output, "Set comment on entity %s\n", s.Name)

	case ast.AlterEntitySetPosition:
		if s.Position == nil {
			return mdlerrors.NewValidation("no position provided")
		}
		entity.Location = model.Point{X: s.Position.X, Y: s.Position.Y}
		if err := ctx.Backend.UpdateEntity(dm.ID, entity); err != nil {
			return mdlerrors.NewBackend("set position", err)
		}
		invalidateDomainModelsCache(ctx)
		fmt.Fprintf(ctx.Output, "Set position of entity %s to (%d, %d)\n", s.Name, s.Position.X, s.Position.Y)

	case ast.AlterEntityAddIndex:
		if s.Index == nil {
			return mdlerrors.NewValidation("no index definition provided")
		}
		// Build name-to-ID map for attribute lookup
		attrNameToID := make(map[string]model.ID)
		for _, attr := range entity.Attributes {
			attrNameToID[attr.Name] = attr.ID
		}
		idxID := model.ID(types.GenerateID())
		var indexAttrs []*domainmodel.IndexAttribute
		for _, col := range s.Index.Columns {
			if attrID, ok := attrNameToID[col.Name]; ok {
				ia := &domainmodel.IndexAttribute{
					AttributeID: attrID,
					Ascending:   !col.Descending,
				}
				ia.ID = model.ID(types.GenerateID())
				indexAttrs = append(indexAttrs, ia)
			} else {
				return mdlerrors.NewNotFoundMsg("attribute", col.Name, fmt.Sprintf("attribute '%s' not found for index on entity %s", col.Name, s.Name))
			}
		}
		index := &domainmodel.Index{
			Attributes: indexAttrs,
		}
		index.ID = idxID
		entity.Indexes = append(entity.Indexes, index)
		if err := ctx.Backend.UpdateEntity(dm.ID, entity); err != nil {
			return mdlerrors.NewBackend("add index", err)
		}
		invalidateDomainModelsCache(ctx)
		fmt.Fprintf(ctx.Output, "Added index to entity %s\n", s.Name)

	case ast.AlterEntityDropIndex:
		// Find and remove the index by position (Mendix indexes don't have user-visible names)
		if len(entity.Indexes) == 0 {
			return mdlerrors.NewValidationf("no indexes on entity %s", s.Name)
		}
		// For now, drop by ordinal name ("idx1", "idx2", etc.) or drop all
		idx := -1
		for i := range entity.Indexes {
			indexName := fmt.Sprintf("idx%d", i+1)
			if indexName == s.IndexName {
				idx = i
				break
			}
		}
		if idx < 0 {
			return mdlerrors.NewNotFoundMsg("index", s.IndexName, fmt.Sprintf("index '%s' not found on entity %s", s.IndexName, s.Name))
		}
		entity.Indexes = append(entity.Indexes[:idx], entity.Indexes[idx+1:]...)
		if err := ctx.Backend.UpdateEntity(dm.ID, entity); err != nil {
			return mdlerrors.NewBackend("drop index", err)
		}
		invalidateDomainModelsCache(ctx)
		fmt.Fprintf(ctx.Output, "Dropped index '%s' from entity %s\n", s.IndexName, s.Name)

	case ast.AlterEntityAddEventHandler:
		if s.EventHandler == nil {
			return mdlerrors.NewValidation("missing event handler definition")
		}
		ehs, err := buildEventHandlers(ctx, []ast.EventHandlerDef{*s.EventHandler})
		if err != nil {
			return err
		}
		// Reject duplicate (same Moment + Event)
		for _, existing := range entity.EventHandlers {
			if existing.Moment == ehs[0].Moment && existing.Event == ehs[0].Event {
				return mdlerrors.NewAlreadyExistsMsg("event handler",
					fmt.Sprintf("%s %s", s.EventHandler.Moment, s.EventHandler.Event),
					fmt.Sprintf("event handler already exists for %s %s on %s", s.EventHandler.Moment, s.EventHandler.Event, s.Name))
			}
		}
		entity.EventHandlers = append(entity.EventHandlers, ehs[0])
		if err := ctx.Backend.UpdateEntity(dm.ID, entity); err != nil {
			return mdlerrors.NewBackend("add event handler", err)
		}
		invalidateDomainModelsCache(ctx)
		fmt.Fprintf(ctx.Output, "Added event handler %s %s on %s\n",
			s.EventHandler.Moment, s.EventHandler.Event, s.Name)

	case ast.AlterEntityDropEventHandler:
		if s.EventHandler == nil {
			return mdlerrors.NewValidation("missing event handler reference")
		}
		moment := domainmodel.EventMoment(s.EventHandler.Moment)
		event := domainmodel.EventType(s.EventHandler.Event)
		idx := -1
		for i, eh := range entity.EventHandlers {
			if eh.Moment == moment && eh.Event == event {
				idx = i
				break
			}
		}
		if idx < 0 {
			return mdlerrors.NewNotFoundMsg("event handler",
				fmt.Sprintf("%s %s", s.EventHandler.Moment, s.EventHandler.Event),
				fmt.Sprintf("event handler %s %s not found on %s", s.EventHandler.Moment, s.EventHandler.Event, s.Name))
		}
		entity.EventHandlers = append(entity.EventHandlers[:idx], entity.EventHandlers[idx+1:]...)
		if err := ctx.Backend.UpdateEntity(dm.ID, entity); err != nil {
			return mdlerrors.NewBackend("drop event handler", err)
		}
		invalidateDomainModelsCache(ctx)
		fmt.Fprintf(ctx.Output, "Dropped event handler %s %s from %s\n",
			s.EventHandler.Moment, s.EventHandler.Event, s.Name)

	default:
		return mdlerrors.NewUnsupported("unsupported ALTER ENTITY operation")
	}

	e.trackModifiedDomainModel(module.ID, module.Name)
	return nil
}

// execDropEntity handles DROP ENTITY statements.
func execDropEntity(ctx *ExecContext, s *ast.DropEntityStmt) error {
	if !ctx.Connected() {
		return mdlerrors.NewNotConnected()
	}

	// Find module and entity
	module, err := findModule(ctx, s.Name.Module)
	if err != nil {
		return err
	}

	dm, err := ctx.Backend.GetDomainModel(module.ID)
	if err != nil {
		return mdlerrors.NewBackend("get domain model", err)
	}

	for _, entity := range dm.Entities {
		if entity.Name == s.Name.Name {
			// Warn about references before deleting (best-effort)
			warnEntityReferences(ctx, s.Name.String())

			// If this is a view entity, also delete the associated ViewEntitySourceDocument
			if entity.Source == "DomainModels$OqlViewEntitySource" {
				if err := ctx.Backend.DeleteViewEntitySourceDocumentByName(s.Name.Module, s.Name.Name); err != nil {
					return mdlerrors.NewBackend("delete view entity source document", err)
				}
			}
			if err := ctx.Backend.DeleteEntity(dm.ID, entity.ID); err != nil {
				return mdlerrors.NewBackend("delete entity", err)
			}
			invalidateDomainModelsCache(ctx)
			fmt.Fprintf(ctx.Output, "Dropped entity: %s\n", s.Name)
			return nil
		}
	}

	return mdlerrors.NewNotFoundMsg("entity", fmt.Sprint(s.Name), fmt.Sprintf("entity not found: %s", s.Name))
}

// warnEntityReferences prints a warning if the entity is referenced by other elements.
// Uses the catalog if available; silently skips if catalog is not built.
func warnEntityReferences(ctx *ExecContext, entityName string) {
	if ctx.Catalog == nil || !ctx.Catalog.IsBuilt() {
		return
	}

	query := fmt.Sprintf(
		"SELECT SourceType, SourceName, RefKind FROM refs WHERE TargetName = '%s'",
		strings.ReplaceAll(entityName, "'", "''"),
	)
	result, err := ctx.Catalog.Query(query)
	if err != nil || result.Count == 0 {
		return
	}

	fmt.Fprintf(ctx.Output, "WARNING: %s is referenced by %d element(s):\n", entityName, result.Count)
	for _, row := range result.Rows {
		sourceType, _ := row[0].(string)
		sourceName, _ := row[1].(string)
		refKind, _ := row[2].(string)
		fmt.Fprintf(ctx.Output, "  - %s %s (%s)\n", sourceType, sourceName, refKind)
	}
}

// --- Executor method wrappers for callers not yet migrated ---
