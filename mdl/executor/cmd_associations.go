// SPDX-License-Identifier: Apache-2.0

// Package executor - Association commands (SHOW/DESCRIBE/CREATE/DROP ASSOCIATION)
package executor

import (
	"fmt"
	"sort"
	"strings"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
	mdlerrors "github.com/JordtenBulte-OLC/mxcli/mdl/errors"
	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/domainmodel"
)

// execCreateAssociation handles CREATE ASSOCIATION statements.
func execCreateAssociation(ctx *ExecContext, s *ast.CreateAssociationStmt) error {
	if !ctx.Connected() {
		return mdlerrors.NewNotConnected()
	}

	// Find or auto-create module
	module, err := findOrCreateModule(ctx, s.Name.Module)
	if err != nil {
		return err
	}

	dm, err := ctx.Backend.GetDomainModel(module.ID)
	if err != nil {
		return mdlerrors.NewBackend("get domain model", err)
	}

	// Find parent and child entities (supports cross-module associations)
	parentModule := s.Parent.Module
	if parentModule == "" {
		parentModule = s.Name.Module
	}
	parentEntity, err := findEntity(ctx, parentModule, s.Parent.Name)
	if err != nil {
		return mdlerrors.NewNotFound("parent entity", s.Parent.String())
	}
	parentID := parentEntity.ID

	childModule := s.Child.Module
	if childModule == "" {
		childModule = s.Name.Module
	}
	childEntity, err := findEntity(ctx, childModule, s.Child.Name)
	if err != nil {
		return mdlerrors.NewNotFound("child entity", s.Child.String())
	}
	childID := childEntity.ID

	// Convert types
	assocType := domainmodel.AssociationTypeReference
	if s.Type == ast.AssocReferenceSet {
		assocType = domainmodel.AssociationTypeReferenceSet
	}

	owner := domainmodel.AssociationOwnerDefault
	switch s.Owner {
	case ast.OwnerBoth:
		owner = domainmodel.AssociationOwnerBoth
	}

	// Convert delete behavior
	var deleteBehavior domainmodel.DeleteBehaviorType
	switch s.DeleteBehavior {
	case ast.DeleteKeepReferences:
		deleteBehavior = domainmodel.DeleteBehaviorTypeDeleteMeButKeepReferences
	case ast.DeleteCascade:
		deleteBehavior = domainmodel.DeleteBehaviorTypeDeleteMeAndReferences
	case ast.DeleteIfNoReferences:
		deleteBehavior = domainmodel.DeleteBehaviorTypeDeleteMeIfNoReferences
	default:
		deleteBehavior = domainmodel.DeleteBehaviorTypeDeleteMeButKeepReferences
	}

	// Convert storage type (default: Column = foreign key in parent table)
	storageFormat := domainmodel.StorageFormatColumn
	switch s.Storage {
	case ast.StorageColumn:
		storageFormat = domainmodel.StorageFormatColumn
	case ast.StorageTable:
		storageFormat = domainmodel.StorageFormatTable
	}

	// Create association
	// ParentID = FROM entity (the one with the FK)
	// ChildID = TO entity (the one being referenced)
	// Cross-module associations use BY_NAME for the child entity
	isCrossModule := parentModule != childModule

	// OR MODIFY: update existing association properties in place, preserving its UUID.
	if s.CreateOrModify {
		if !isCrossModule {
			for _, assoc := range dm.Associations {
				if assoc.Name == s.Name.Name {
					assoc.Type = assocType
					assoc.Owner = owner
					assoc.StorageFormat = storageFormat
					assoc.ChildDeleteBehavior = &domainmodel.DeleteBehavior{Type: deleteBehavior}
					if s.Comment != "" {
						assoc.Documentation = s.Comment
					}
					if err := ctx.Backend.UpdateDomainModel(dm); err != nil {
						return mdlerrors.NewBackend("update association", err)
					}
					invalidateHierarchy(ctx)
					invalidateDomainModelsCache(ctx)
					ctx.trackModifiedDomainModel(module.ID, module.Name)
					fmt.Fprintf(ctx.Output, "Modified association: %s\n", s.Name)
					return nil
				}
			}
		} else {
			childRef := childModule + "." + s.Child.Name
			for _, ca := range dm.CrossAssociations {
				if ca.Name == s.Name.Name {
					ca.Type = assocType
					ca.Owner = owner
					ca.StorageFormat = storageFormat
					ca.ChildDeleteBehavior = &domainmodel.DeleteBehavior{Type: deleteBehavior}
					ca.ChildRef = childRef
					if s.Comment != "" {
						ca.Documentation = s.Comment
					}
					if err := ctx.Backend.UpdateDomainModel(dm); err != nil {
						return mdlerrors.NewBackend("update cross-module association", err)
					}
					invalidateHierarchy(ctx)
					invalidateDomainModelsCache(ctx)
					ctx.trackModifiedDomainModel(module.ID, module.Name)
					fmt.Fprintf(ctx.Output, "Modified association: %s\n", s.Name)
					return nil
				}
			}
		}
		// Association not found — fall through to create it.
	}

	if isCrossModule {
		if !s.CreateOrModify {
			for _, ca := range dm.CrossAssociations {
				if ca.Name == s.Name.Name {
					return mdlerrors.NewAlreadyExists("association", s.Name.String())
				}
			}
			for _, assoc := range dm.Associations {
				if assoc.Name == s.Name.Name {
					return mdlerrors.NewAlreadyExists("association", s.Name.String())
				}
			}
		}
		childRef := childModule + "." + s.Child.Name
		ca := &domainmodel.CrossModuleAssociation{
			Name:          s.Name.Name,
			Type:          assocType,
			Owner:         owner,
			StorageFormat: storageFormat,
			ParentID:      parentID,
			ChildRef:      childRef,
			ChildDeleteBehavior: &domainmodel.DeleteBehavior{
				Type: deleteBehavior,
			},
		}
		if err := ctx.Backend.CreateCrossAssociation(dm.ID, ca); err != nil {
			return mdlerrors.NewBackend("create cross-module association", err)
		}
	} else {
		// Check for existing association when not using OR MODIFY.
		if !s.CreateOrModify {
			for _, assoc := range dm.Associations {
				if assoc.Name == s.Name.Name {
					return mdlerrors.NewAlreadyExists("association", s.Name.String())
				}
			}
			for _, ca := range dm.CrossAssociations {
				if ca.Name == s.Name.Name {
					return mdlerrors.NewAlreadyExists("association", s.Name.String())
				}
			}
		}
		assoc := &domainmodel.Association{
			Name:          s.Name.Name,
			Type:          assocType,
			Owner:         owner,
			StorageFormat: storageFormat,
			ParentID:      parentID,
			ChildID:       childID,
			ChildDeleteBehavior: &domainmodel.DeleteBehavior{
				Type: deleteBehavior,
			},
		}
		if err := ctx.Backend.CreateAssociation(dm.ID, assoc); err != nil {
			return mdlerrors.NewBackend("create association", err)
		}
	}

	// Invalidate hierarchy cache so the new association's container is visible
	invalidateHierarchy(ctx)
	invalidateDomainModelsCache(ctx)

	// Reconcile MemberAccesses immediately — existing access rules on entities
	// in this DM need MemberAccess entries for the new association (CE0066).
	if freshDM, err := ctx.Backend.GetDomainModel(module.ID); err == nil {
		if count, err := ctx.Backend.ReconcileMemberAccesses(freshDM.ID, module.Name); err == nil && count > 0 {
			fmt.Fprintf(ctx.Output, "Reconciled %d access rule(s) for new association\n", count)
		}
	}

	ctx.trackModifiedDomainModel(module.ID, module.Name)
	fmt.Fprintf(ctx.Output, "Created association: %s\n", s.Name)
	return nil
}

// execAlterAssociation handles ALTER ASSOCIATION statements.
func execAlterAssociation(ctx *ExecContext, s *ast.AlterAssociationStmt) error {
	if !ctx.Connected() {
		return mdlerrors.NewNotConnected()
	}

	module, err := findModule(ctx, s.Name.Module)
	if err != nil {
		return err
	}

	dm, err := ctx.Backend.GetDomainModel(module.ID)
	if err != nil {
		return mdlerrors.NewBackend("get domain model", err)
	}

	// Try intra-module associations first
	for _, assoc := range dm.Associations {
		if assoc.Name == s.Name.Name {
			switch s.Operation {
			case ast.AlterAssociationSetDeleteBehavior:
				assoc.ChildDeleteBehavior = &domainmodel.DeleteBehavior{
					Type: domainmodel.DeleteBehaviorType(s.DeleteBehavior.String()),
				}
			case ast.AlterAssociationSetOwner:
				assoc.Owner = domainmodel.AssociationOwner(s.Owner.String())
			case ast.AlterAssociationSetStorage:
				assoc.StorageFormat = domainmodel.AssociationStorageFormat(s.Storage.String())
			case ast.AlterAssociationSetComment:
				assoc.Documentation = s.Comment
			}
			if err := ctx.Backend.UpdateDomainModel(dm); err != nil {
				return mdlerrors.NewBackend("update association", err)
			}
			fmt.Fprintf(ctx.Output, "Altered association: %s\n", s.Name)
			return nil
		}
	}

	// Try cross-module associations
	for _, ca := range dm.CrossAssociations {
		if ca.Name == s.Name.Name {
			switch s.Operation {
			case ast.AlterAssociationSetDeleteBehavior:
				ca.ChildDeleteBehavior = &domainmodel.DeleteBehavior{
					Type: domainmodel.DeleteBehaviorType(s.DeleteBehavior.String()),
				}
			case ast.AlterAssociationSetOwner:
				ca.Owner = domainmodel.AssociationOwner(s.Owner.String())
			case ast.AlterAssociationSetStorage:
				ca.StorageFormat = domainmodel.AssociationStorageFormat(s.Storage.String())
			case ast.AlterAssociationSetComment:
				ca.Documentation = s.Comment
			}
			if err := ctx.Backend.UpdateDomainModel(dm); err != nil {
				return mdlerrors.NewBackend("update cross-module association", err)
			}
			fmt.Fprintf(ctx.Output, "Altered association: %s\n", s.Name)
			return nil
		}
	}

	return mdlerrors.NewNotFound("association", s.Name.String())
}

// execDropAssociation handles DROP ASSOCIATION statements.
func execDropAssociation(ctx *ExecContext, s *ast.DropAssociationStmt) error {
	if !ctx.Connected() {
		return mdlerrors.NewNotConnected()
	}

	// Find module
	module, err := findModule(ctx, s.Name.Module)
	if err != nil {
		return err
	}

	dm, err := ctx.Backend.GetDomainModel(module.ID)
	if err != nil {
		return mdlerrors.NewBackend("get domain model", err)
	}

	for _, assoc := range dm.Associations {
		if assoc.Name == s.Name.Name {
			if err := ctx.Backend.DeleteAssociation(dm.ID, assoc.ID); err != nil {
				return mdlerrors.NewBackend("delete association", err)
			}
			fmt.Fprintf(ctx.Output, "Dropped association: %s\n", s.Name)
			return nil
		}
	}
	for _, ca := range dm.CrossAssociations {
		if ca.Name == s.Name.Name {
			if err := ctx.Backend.DeleteCrossAssociation(dm.ID, ca.ID); err != nil {
				return mdlerrors.NewBackend("delete cross-module association", err)
			}
			fmt.Fprintf(ctx.Output, "Dropped cross-module association: %s\n", s.Name)
			return nil
		}
	}

	return mdlerrors.NewNotFound("association", s.Name.String())
}

// listAssociations handles SHOW ASSOCIATIONS command.
func listAssociations(ctx *ExecContext, moduleName string) error {
	// Build module ID -> name map (single query)
	modules, err := ctx.Backend.ListModules()
	if err != nil {
		return mdlerrors.NewBackend("list modules", err)
	}
	moduleNames := make(map[model.ID]string)
	for _, m := range modules {
		moduleNames[m.ID] = m.Name
	}

	// Get all domain models in a single query (avoids O(n²) behavior)
	domainModels, err := ctx.Backend.ListDomainModels()
	if err != nil {
		return mdlerrors.NewBackend("list domain models", err)
	}

	// Build entity ID -> qualified name map
	entityNames := make(map[model.ID]string)
	for _, dm := range domainModels {
		modName := moduleNames[dm.ContainerID]
		for _, entity := range dm.Entities {
			entityNames[entity.ID] = modName + "." + entity.Name
		}
	}

	// Collect rows
	type row struct {
		qualifiedName string
		module        string
		name          string
		parent        string
		child         string
		assocType     string
		owner         string
		storage       string
	}
	var rows []row

	for _, dm := range domainModels {
		modName := moduleNames[dm.ContainerID]
		// Filter by module name if specified
		if moduleName != "" && modName != moduleName {
			continue
		}
		// Intra-module associations
		for _, assoc := range dm.Associations {
			qualifiedName := modName + "." + assoc.Name
			parent := entityNames[assoc.ParentID]
			child := entityNames[assoc.ChildID]
			if parent == "" {
				parent = string(assoc.ParentID)
			}
			if child == "" {
				child = string(assoc.ChildID)
			}
			rows = append(rows, row{qualifiedName, modName, assoc.Name, parent, child, string(assoc.Type), string(assoc.Owner), string(assoc.StorageFormat)})
		}
		// Cross-module associations
		for _, ca := range dm.CrossAssociations {
			qualifiedName := modName + "." + ca.Name
			parent := entityNames[ca.ParentID]
			if parent == "" {
				parent = string(ca.ParentID)
			}
			rows = append(rows, row{qualifiedName, modName, ca.Name, parent, ca.ChildRef, string(ca.Type), string(ca.Owner), string(ca.StorageFormat)})
		}
	}

	// Sort by qualified name
	sort.Slice(rows, func(i, j int) bool {
		return strings.ToLower(rows[i].qualifiedName) < strings.ToLower(rows[j].qualifiedName)
	})

	// Build TableResult
	result := &TableResult{
		Columns: []string{"Qualified Name", "Module", "Name", "Parent", "Child", "Type", "Owner", "Storage"},
		Summary: fmt.Sprintf("(%d associations)", len(rows)),
	}
	for _, r := range rows {
		result.Rows = append(result.Rows, []any{r.qualifiedName, r.module, r.name, r.parent, r.child, r.assocType, r.owner, r.storage})
	}
	return writeResult(ctx, result)
}

// listAssociation handles SHOW ASSOCIATION command.
func listAssociation(ctx *ExecContext, name *ast.QualifiedName) error {
	if name == nil {
		return mdlerrors.NewValidation("association name required")
	}

	module, err := findModule(ctx, name.Module)
	if err != nil {
		return err
	}

	dm, err := ctx.Backend.GetDomainModel(module.ID)
	if err != nil {
		return mdlerrors.NewBackend("get domain model", err)
	}

	for _, assoc := range dm.Associations {
		if assoc.Name == name.Name {
			fmt.Fprintf(ctx.Output, "Association: %s.%s\n", module.Name, assoc.Name)
			fmt.Fprintf(ctx.Output, "  Type: %s\n", assoc.Type)
			fmt.Fprintf(ctx.Output, "  Owner: %s\n", assoc.Owner)
			fmt.Fprintf(ctx.Output, "  Storage: %s\n", assoc.StorageFormat)
			return nil
		}
	}
	for _, ca := range dm.CrossAssociations {
		if ca.Name == name.Name {
			fmt.Fprintf(ctx.Output, "Association: %s.%s (cross-module)\n", module.Name, ca.Name)
			fmt.Fprintf(ctx.Output, "  Type: %s\n", ca.Type)
			fmt.Fprintf(ctx.Output, "  Owner: %s\n", ca.Owner)
			fmt.Fprintf(ctx.Output, "  Storage: %s\n", ca.StorageFormat)
			fmt.Fprintf(ctx.Output, "  Child: %s\n", ca.ChildRef)
			return nil
		}
	}

	return mdlerrors.NewNotFound("association", name.String())
}

// describeAssociation handles DESCRIBE ASSOCIATION command.
func describeAssociation(ctx *ExecContext, name ast.QualifiedName) error {
	module, err := findModule(ctx, name.Module)
	if err != nil {
		return err
	}

	dm, err := ctx.Backend.GetDomainModel(module.ID)
	if err != nil {
		return mdlerrors.NewBackend("get domain model", err)
	}

	// Build entity ID -> qualified name map across all modules
	entityNames := make(map[model.ID]string)
	allDomainModels, err := ctx.Backend.ListDomainModels()
	if err != nil {
		return mdlerrors.NewBackend("list domain models", err)
	}
	h, err := getHierarchy(ctx)
	if err != nil {
		return mdlerrors.NewBackend("build hierarchy", err)
	}
	for _, otherDM := range allDomainModels {
		modName := h.GetModuleName(otherDM.ContainerID)
		for _, entity := range otherDM.Entities {
			entityNames[entity.ID] = modName + "." + entity.Name
		}
	}

	// Helper to format association type, owner, storage, and delete behavior
	formatAssocDetails := func(assocType domainmodel.AssociationType, assocOwner domainmodel.AssociationOwner, storageFormat domainmodel.AssociationStorageFormat, childDeleteBehavior *domainmodel.DeleteBehavior) {
		typeName := "Reference"
		if assocType == domainmodel.AssociationTypeReferenceSet {
			typeName = "ReferenceSet"
		}
		fmt.Fprintf(ctx.Output, "type %s\n", typeName)

		owner := "Default"
		if assocOwner == domainmodel.AssociationOwnerBoth {
			owner = "Both"
		}
		fmt.Fprintf(ctx.Output, "owner %s\n", owner)

		// Only output STORAGE when it's not the default (Table)
		if storageFormat == domainmodel.StorageFormatColumn {
			fmt.Fprintf(ctx.Output, "storage column\n")
		}

		deleteBehavior := "DELETE_BUT_KEEP_REFERENCES"
		if childDeleteBehavior != nil {
			switch childDeleteBehavior.Type {
			case domainmodel.DeleteBehaviorTypeDeleteMeAndReferences:
				deleteBehavior = "DELETE_CASCADE"
			case domainmodel.DeleteBehaviorTypeDeleteMeIfNoReferences:
				deleteBehavior = "DELETE_IF_NO_REFERENCES"
			case domainmodel.DeleteBehaviorTypeDeleteMeButKeepReferences:
				deleteBehavior = "DELETE_BUT_KEEP_REFERENCES"
			}
		}
		fmt.Fprintf(ctx.Output, "delete_behavior %s;\n", deleteBehavior)
	}

	for _, assoc := range dm.Associations {
		if assoc.Name == name.Name {
			fromEntity := entityNames[assoc.ParentID]
			toEntity := entityNames[assoc.ChildID]

			if assoc.Documentation != "" {
				fmt.Fprintf(ctx.Output, "/**\n * %s\n */\n", assoc.Documentation)
			}

			fmt.Fprintf(ctx.Output, "create association %s.%s\n", module.Name, assoc.Name)
			fmt.Fprintf(ctx.Output, "from %s to %s\n", fromEntity, toEntity)
			formatAssocDetails(assoc.Type, assoc.Owner, assoc.StorageFormat, assoc.ChildDeleteBehavior)
			fmt.Fprintln(ctx.Output, "/")
			return nil
		}
	}
	for _, ca := range dm.CrossAssociations {
		if ca.Name == name.Name {
			fromEntity := entityNames[ca.ParentID]
			if fromEntity == "" {
				fromEntity = string(ca.ParentID)
			}

			if ca.Documentation != "" {
				fmt.Fprintf(ctx.Output, "/**\n * %s\n */\n", ca.Documentation)
			}

			fmt.Fprintf(ctx.Output, "create association %s.%s\n", module.Name, ca.Name)
			fmt.Fprintf(ctx.Output, "from %s to %s\n", fromEntity, ca.ChildRef)
			formatAssocDetails(ca.Type, ca.Owner, ca.StorageFormat, ca.ChildDeleteBehavior)
			fmt.Fprintln(ctx.Output, "/")
			return nil
		}
	}

	return mdlerrors.NewNotFound("association", name.String())
}

// --- Executor method wrappers for callers not yet migrated ---
