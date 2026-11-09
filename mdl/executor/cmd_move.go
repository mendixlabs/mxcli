// SPDX-License-Identifier: Apache-2.0

// Package executor - MOVE command
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

// execMove handles MOVE PAGE/MICROFLOW/SNIPPET/NANOFLOW/ENTITY/ENUMERATION statements.
func execMove(ctx *ExecContext, s *ast.MoveStmt) (moveErr error) {
	if !ctx.ConnectedForWrite() {
		return mdlerrors.NewNotConnected()
	}

	// Find the source module
	sourceModule, err := findModule(ctx, s.Name.Module)
	if err != nil {
		return mdlerrors.NewBackend("find source module", err)
	}

	// Determine target module
	var targetModule *model.Module
	isCrossModuleMove := false
	if s.TargetModule != "" {
		targetModule, err = findModule(ctx, s.TargetModule)
		if err != nil {
			return mdlerrors.NewBackend("find target module", err)
		}
		isCrossModuleMove = targetModule.ID != sourceModule.ID
	} else {
		targetModule = sourceModule
	}

	// Entity moves are handled specially (entities are embedded in domain models, not top-level units)
	if s.DocumentType == ast.DocumentTypeEntity {
		return moveEntity(ctx, s.Name, sourceModule, targetModule)
	}

	// Resolve target container (folder or module root)
	var targetContainerID model.ID
	if s.Folder != "" {
		targetContainerID, err = resolveFolder(ctx, targetModule.ID, s.Folder)
		if err != nil {
			return mdlerrors.NewBackend("resolve target folder", err)
		}
	} else {
		targetContainerID = targetModule.ID
	}

	// A document already where the statement puts it is not moved, and the
	// statement says so: an organise step re-run on a settled project
	// announced every document as moved although nothing was written
	// (ako/mxcli#890).
	if !isCrossModuleMove && alreadyPlaced(ctx, s, targetContainerID) {
		return nil
	}
	// Every mover changes a container the cached hierarchy indexes by, so a
	// later statement resolving the document through it would not find it
	// ("microflow not found" on a second move of the same document).
	defer invalidateHierarchy(ctx)

	// A Java or JavaScript action's source lives under its module's directory,
	// and mxbuild looks for it under the module the model now names: left
	// behind, the build succeeded with a stub that throws where the app calls
	// the action (measured on 11.13.0). The source moves first, so an occupied
	// destination refuses the statement before the model changes, and moves
	// back if the rest of the statement fails. exec names the move with the
	// other files beside the model a statement touched.
	if isCrossModuleMove {
		moved, err := moveActionSource(ctx, s.DocumentType, s.Name.Module, targetModule.Name, s.Name.Name)
		if err != nil {
			return err
		}
		if moved != "" {
			defer func() {
				if moveErr != nil {
					_, _ = moveActionSource(ctx, s.DocumentType, targetModule.Name, s.Name.Module, s.Name.Name)
				}
			}()
		}
	}

	// Execute move based on document type
	switch s.DocumentType {
	case ast.DocumentTypePage:
		if err := movePage(ctx, s.Name, targetContainerID, targetModule, isCrossModuleMove); err != nil {
			return err
		}
	case ast.DocumentTypeMicroflow:
		if err := moveMicroflow(ctx, s.Name, targetContainerID, targetModule, isCrossModuleMove); err != nil {
			return err
		}
	case ast.DocumentTypeSnippet:
		if err := moveSnippet(ctx, s.Name, targetContainerID); err != nil {
			return err
		}
	case ast.DocumentTypeNanoflow:
		if err := moveNanoflow(ctx, s.Name, targetContainerID, targetModule, isCrossModuleMove); err != nil {
			return err
		}
	case ast.DocumentTypeRule:
		if err := moveRule(ctx, s.Name, targetContainerID); err != nil {
			return err
		}
	case ast.DocumentTypeEnumeration:
		// Not a return: an enumeration's cross-module move needs the reference
		// sweep below like any other document's. Returning here left every
		// reference outside the domain models — parameter and variable types,
		// enumeration values — naming the old module (CE1613, ako/mxcli#1674).
		if err := moveEnumeration(ctx, s.Name, targetContainerID, targetModule.Name); err != nil {
			return err
		}
	case ast.DocumentTypeConstant:
		if err := moveConstant(ctx, s.Name, targetContainerID); err != nil {
			return err
		}
	case ast.DocumentTypeDatabaseConnection:
		if err := moveDatabaseConnection(ctx, s.Name, targetContainerID); err != nil {
			return err
		}
	case ast.DocumentTypeJavaAction:
		if err := moveJavaAction(ctx, s.Name, targetContainerID); err != nil {
			return err
		}
	case ast.DocumentTypeODataService:
		if err := movePublishedODataService(ctx, s.Name, targetContainerID); err != nil {
			return err
		}
	default:
		// Every remaining doctype is a plain top-level document, and a move is
		// one containment row — so they share a handler instead of each needing
		// a list call and a finder written for it. The typed cases above are the
		// ones whose move does extra work (access-role remapping, reference
		// rewriting); this is everything else, including doctypes added later.
		if err := moveDocumentUnit(ctx, s.DocumentType, s.Name, targetContainerID); err != nil {
			return err
		}
	}

	// For cross-module moves, update all BY_NAME references throughout the project
	if isCrossModuleMove {
		if err := updateQualifiedNameRefs(ctx, s.Name, targetModule.Name); err != nil {
			return err
		}
	}

	return nil
}

// moveActionSource moves a Java or JavaScript action's source file from one
// module's directory to another's and returns its new project-relative path, or
// "" when the document type has no source file or there was none to move.
func moveActionSource(ctx *ExecContext, docType ast.DocumentType, fromModule, toModule, name string) (string, error) {
	var (
		moved string
		err   error
	)
	switch docType {
	case ast.DocumentTypeJavaAction:
		moved, err = ctx.Backend.MoveJavaSourceFile(fromModule, toModule, name)
	case ast.DocumentTypeJavaScriptAction:
		moved, err = ctx.Backend.MoveJavaScriptSourceFile(fromModule, toModule, name)
	default:
		return "", nil
	}
	if err != nil {
		return "", mdlerrors.NewBackend("move "+docType.CanonicalSpelling()+" source", err)
	}
	return moved, nil
}

// alreadyPlaced reports, as Unchanged, a move whose document already sits in
// the target container. Anything it cannot establish — no such document, a
// different doctype under that name — is left to the mover, which reports it.
func alreadyPlaced(ctx *ExecContext, s *ast.MoveStmt, targetContainerID model.ID) bool {
	doc, err := ctx.Backend.FindDocumentUnit(s.Name.Module, s.Name.Name)
	if err != nil || doc == nil || doc.ContainerID != targetContainerID {
		return false
	}
	if checkMovedDocumentType(s.DocumentType.CanonicalSpelling(), doc.Kind, s.Name) != nil {
		return false
	}
	reportUnchanged(ctx, fmt.Sprintf("%s: %s", doc.Kind, s.Name))
	return true
}

// updateQualifiedNameRefs updates all BY_NAME references to an element after a cross-module move.
func updateQualifiedNameRefs(ctx *ExecContext, name ast.QualifiedName, newModule string) error {
	oldQN := name.String()               // "OldModule.ElementName"
	newQN := newModule + "." + name.Name // "NewModule.ElementName"
	updated, err := ctx.Backend.UpdateQualifiedNameInAllUnits(oldQN, newQN)
	if err != nil {
		return mdlerrors.NewBackend("update references", err)
	}
	if updated > 0 {
		fmt.Fprintf(ctx.Output, "Updated references in %d document(s): %s → %s\n", updated, oldQN, newQN)
	}
	return nil
}

// movePage moves a page to a new container.
func movePage(ctx *ExecContext, name ast.QualifiedName, targetContainerID model.ID, targetModule *model.Module, isCrossModuleMove bool) error {
	// Find the page
	pages, err := ctx.Backend.ListPages()
	if err != nil {
		return mdlerrors.NewBackend("list pages", err)
	}

	h, err := getHierarchy(ctx)
	if err != nil {
		return mdlerrors.NewBackend("build hierarchy", err)
	}

	for _, p := range pages {
		modID := h.FindModuleID(p.ContainerID)
		modName := h.GetModuleName(modID)
		if modName == name.Module && p.Name == name.Name {
			// Update container ID and move the unit
			p.ContainerID = targetContainerID
			if err := ctx.Backend.MovePage(p); err != nil {
				return mdlerrors.NewBackend("move page", err)
			}
			if isCrossModuleMove {
				p.AllowedRoles = remapDocumentAccessRoles(ctx, targetModule, p.AllowedRoles)
				if err := ctx.Backend.UpdateAllowedRoles(p.ID, documentRoleStrings(p.AllowedRoles)); err != nil {
					return mdlerrors.NewBackend("remap page access", err)
				}
			}
			fmt.Fprintf(ctx.Output, "Moved page %s to new location\n", name.String())
			return nil
		}
	}

	return mdlerrors.NewNotFound("page", name.String())
}

// moveMicroflow moves a microflow to a new container.
func moveMicroflow(ctx *ExecContext, name ast.QualifiedName, targetContainerID model.ID, targetModule *model.Module, isCrossModuleMove bool) error {
	// Find the microflow
	mfs, err := ctx.Backend.ListMicroflows()
	if err != nil {
		return mdlerrors.NewBackend("list microflows", err)
	}

	h, err := getHierarchy(ctx)
	if err != nil {
		return mdlerrors.NewBackend("build hierarchy", err)
	}

	for _, mf := range mfs {
		modID := h.FindModuleID(mf.ContainerID)
		modName := h.GetModuleName(modID)
		if modName == name.Module && mf.Name == name.Name {
			// Update container ID and move the unit
			mf.ContainerID = targetContainerID
			if err := ctx.Backend.MoveMicroflow(mf); err != nil {
				return mdlerrors.NewBackend("move microflow", err)
			}
			if isCrossModuleMove {
				mf.AllowedModuleRoles = remapDocumentAccessRoles(ctx, targetModule, mf.AllowedModuleRoles)
				if err := ctx.Backend.UpdateAllowedRoles(mf.ID, documentRoleStrings(mf.AllowedModuleRoles)); err != nil {
					return mdlerrors.NewBackend("remap microflow access", err)
				}
			}
			fmt.Fprintf(ctx.Output, "Moved microflow %s to new location\n", name.String())
			return nil
		}
	}

	return mdlerrors.NewNotFound("microflow", name.String())
}

// moveSnippet moves a snippet to a new container.
func moveSnippet(ctx *ExecContext, name ast.QualifiedName, targetContainerID model.ID) error {
	// Find the snippet
	snippets, err := ctx.Backend.ListSnippets()
	if err != nil {
		return mdlerrors.NewBackend("list snippets", err)
	}

	h, err := getHierarchy(ctx)
	if err != nil {
		return mdlerrors.NewBackend("build hierarchy", err)
	}

	for _, s := range snippets {
		modID := h.FindModuleID(s.ContainerID)
		modName := h.GetModuleName(modID)
		if modName == name.Module && s.Name == name.Name {
			// Update container ID and move the unit
			s.ContainerID = targetContainerID
			if err := ctx.Backend.MoveSnippet(s); err != nil {
				return mdlerrors.NewBackend("move snippet", err)
			}
			fmt.Fprintf(ctx.Output, "Moved snippet %s to new location\n", name.String())
			return nil
		}
	}

	return mdlerrors.NewNotFound("snippet", name.String())
}

// moveNanoflow moves a nanoflow to a new container.
// moveRule reparents a rule unit. A rule's placement is ordinary — the unit row
// is ContainmentName "Documents" with the folder as its container, identical to
// a microflow (measured on ako/TestApp, where both rules sit in a folder).
func moveRule(ctx *ExecContext, name ast.QualifiedName, targetContainerID model.ID) error {
	rules, err := ctx.Backend.ListRules()
	if err != nil {
		return mdlerrors.NewBackend("list rules", err)
	}

	h, err := getHierarchy(ctx)
	if err != nil {
		return mdlerrors.NewBackend("build hierarchy", err)
	}

	for _, rule := range rules {
		modID := h.FindModuleID(rule.ContainerID)
		if h.GetModuleName(modID) != name.Module || rule.Name != name.Name {
			continue
		}
		rule.ContainerID = targetContainerID
		if err := ctx.Backend.MoveRule(rule); err != nil {
			return mdlerrors.NewBackend("move rule", err)
		}
		fmt.Fprintf(ctx.Output, "Moved rule %s to new location\n", name.String())
		return nil
	}

	return mdlerrors.NewNotFound("rule", name.String())
}

func moveNanoflow(ctx *ExecContext, name ast.QualifiedName, targetContainerID model.ID, targetModule *model.Module, isCrossModuleMove bool) error {
	// Find the nanoflow
	nfs, err := ctx.Backend.ListNanoflows()
	if err != nil {
		return mdlerrors.NewBackend("list nanoflows", err)
	}

	h, err := getHierarchy(ctx)
	if err != nil {
		return mdlerrors.NewBackend("build hierarchy", err)
	}

	for _, nf := range nfs {
		modID := h.FindModuleID(nf.ContainerID)
		modName := h.GetModuleName(modID)
		if modName == name.Module && nf.Name == name.Name {
			// Update container ID and move the unit
			nf.ContainerID = targetContainerID
			if err := ctx.Backend.MoveNanoflow(nf); err != nil {
				return mdlerrors.NewBackend("move nanoflow", err)
			}
			// A nanoflow's grants map to the target's same-named roles, as a
			// microflow's do; left naming the old module they were CE0148
			// (ako/mxcli#1836).
			if isCrossModuleMove {
				nf.AllowedModuleRoles = remapDocumentAccessRoles(ctx, targetModule, nf.AllowedModuleRoles)
				if err := ctx.Backend.UpdateAllowedRoles(nf.ID, documentRoleStrings(nf.AllowedModuleRoles)); err != nil {
					return mdlerrors.NewBackend("remap nanoflow access", err)
				}
			}
			fmt.Fprintf(ctx.Output, "Moved nanoflow %s to new location\n", name.String())
			return nil
		}
	}

	return mdlerrors.NewNotFound("nanoflow", name.String())
}

// moveEntity moves an entity from one domain model to another.
// Entities are embedded inside DomainModel documents, so we must remove from source DM and add to target DM.
// Associations referencing the entity are converted to CrossAssociations.
// ViewEntitySourceDocuments for view entities are also moved.
func moveEntity(ctx *ExecContext, name ast.QualifiedName, sourceModule, targetModule *model.Module) error {
	// Get source domain model
	sourceDM, err := ctx.Backend.GetDomainModel(sourceModule.ID)
	if err != nil {
		return mdlerrors.NewBackend("get source domain model", err)
	}

	// Find the entity in the source domain model
	var entity *domainmodel.Entity
	for _, ent := range sourceDM.Entities {
		if ent.Name == name.Name {
			entity = ent
			break
		}
	}
	if entity == nil {
		return mdlerrors.NewNotFound("entity", name.String())
	}

	// Get target domain model
	targetDM, err := ctx.Backend.GetDomainModel(targetModule.ID)
	if err != nil {
		return mdlerrors.NewBackend("get target domain model", err)
	}

	// An entity's access rules may only name its own module's roles, so each
	// moves to the target's role of the same name; one the target lacks is kept,
	// and said, rather than dropped — a rule that silently lost its role would
	// hide the entity from it (ako/mxcli#1836).
	remapEntityAccessRoles(ctx, entity, sourceModule, targetModule)

	// Move entity via writer (converts associations to CrossAssociations, updates
	// validation rule and access rule refs)
	convertedAssocs, err := ctx.Backend.MoveEntity(entity, sourceDM.ID, targetDM.ID, sourceModule.Name, targetModule.Name)
	if err != nil {
		return mdlerrors.NewBackend("move entity", err)
	}

	// Move ViewEntitySourceDocument for view entities
	if entity.Source == "DomainModels$OqlViewEntitySource" && entity.SourceDocumentRef != "" {
		// The SourceDocumentRef was already updated by MoveEntity to use the new module name.
		// Extract the original doc name (before the module prefix was changed).
		docName := name.Name // ViewEntitySourceDocument name matches the entity name
		if err := ctx.Backend.MoveViewEntitySourceDocument(sourceModule.Name, targetModule.ID, docName); err != nil {
			fmt.Fprintf(ctx.Output, "Warning: Could not move ViewEntitySourceDocument: %v\n", err)
		}
	}

	// Update OQL queries in all ViewEntitySourceDocuments that reference the moved entity
	oldQualifiedName := name.String()                       // e.g., "DmTest.Customer"
	newQualifiedName := targetModule.Name + "." + name.Name // e.g., "DmTest2.Customer"
	if oqlUpdated, err := ctx.Backend.UpdateOqlQueriesForMovedEntity(oldQualifiedName, newQualifiedName); err != nil {
		fmt.Fprintf(ctx.Output, "Warning: Could not update OQL queries: %v\n", err)
	} else if oqlUpdated > 0 {
		fmt.Fprintf(ctx.Output, "Updated %d OQL query(ies) referencing %s\n", oqlUpdated, oldQualifiedName)
	}

	// Every OTHER cross-module move sweeps the project for BY_NAME references (the
	// isCrossModuleMove branch in execMove). An entity move returns early, before
	// both the doctype switch and that sweep — correctly, because an entity is not a
	// top-level unit, but the sweep is purely name-based and applies just the same.
	// Without it a move reported success and left every reference to the entity, its
	// attribute paths and the converted association naming the old module: 33 CE1613s
	// in a blank 11.13 app, in microflow activities, page widgets, page parameters and
	// access rules (#605).
	//
	// The association renames come from the backend rather than from the module names,
	// because only the direction that carried the cross-association to the target
	// changed its qualified name; the other is a no-op here by construction.
	sweep := func(oldQN, newQN string) error {
		if oldQN == newQN {
			return nil
		}
		updated, err := ctx.Backend.UpdateQualifiedNameInAllUnits(oldQN, newQN)
		if err != nil {
			return mdlerrors.NewBackend("update references", err)
		}
		if updated > 0 {
			fmt.Fprintf(ctx.Output, "Updated references in %d document(s): %s → %s\n", updated, oldQN, newQN)
		}
		return nil
	}
	// The entity itself, which also covers every `Module.Entity.Attribute` path
	// because the sweep matches a name that equals the old one or is prefixed by it.
	if err := sweep(oldQualifiedName, newQualifiedName); err != nil {
		return err
	}
	for _, assoc := range convertedAssocs {
		if err := sweep(assoc.OldQualifiedName, assoc.NewQualifiedName); err != nil {
			return err
		}
	}

	fmt.Fprintf(ctx.Output, "Moved entity %s to %s\n", name.String(), targetModule.Name)
	// A move that brings both endpoints of a cross-association into one module turns
	// it back into a plain association (ako/mxcli#628); report the two apart.
	var toCross, toPlain []types.MovedAssociation
	for _, a := range convertedAssocs {
		if a.SameModule {
			toPlain = append(toPlain, a)
		} else {
			toCross = append(toCross, a)
		}
	}
	if len(toCross) > 0 {
		fmt.Fprintf(ctx.Output, "Converted %d association(s) to cross-module associations:\n", len(toCross))
		for _, assocName := range types.MovedAssociationNames(toCross) {
			fmt.Fprintf(ctx.Output, "  - %s\n", assocName)
		}
	}
	if len(toPlain) > 0 {
		fmt.Fprintf(ctx.Output, "Converted %d cross-module association(s) to associations within %s:\n", len(toPlain), targetModule.Name)
		for _, assocName := range types.MovedAssociationNames(toPlain) {
			fmt.Fprintf(ctx.Output, "  - %s\n", assocName)
		}
	}
	return nil
}

// moveEnumeration moves an enumeration to a new container.
// For cross-module moves, updates all EnumerationAttributeType references across all domain models.
func moveEnumeration(ctx *ExecContext, name ast.QualifiedName, targetContainerID model.ID, targetModuleName string) error {
	// Neither end may be System: its enumerations are platform built-ins with no
	// stored unit, so there is nothing to move out and nowhere to move in (#1102).
	if err := refuseSystemEnumerationWrite("move enumeration", name); err != nil {
		return err
	}
	if targetModuleName == "System" {
		return refuseSystemEnumerationWrite("move enumeration into",
			ast.QualifiedName{Module: "System", Name: name.Name})
	}
	enum := findEnumeration(ctx, name.Module, name.Name)
	if enum == nil {
		return mdlerrors.NewNotFound("enumeration", name.String())
	}

	oldQualifiedName := name.String() // e.g., "DmTest.Country"
	enum.ContainerID = targetContainerID
	if err := ctx.Backend.MoveEnumeration(enum); err != nil {
		return mdlerrors.NewBackend("move enumeration", err)
	}

	// For cross-module moves, update enumeration references in all domain models
	if targetModuleName != "" && targetModuleName != name.Module {
		newQualifiedName := targetModuleName + "." + name.Name
		if err := ctx.Backend.UpdateEnumerationRefsInAllDomainModels(oldQualifiedName, newQualifiedName); err != nil {
			fmt.Fprintf(ctx.Output, "Warning: Could not update enumeration references: %v\n", err)
		} else {
			fmt.Fprintf(ctx.Output, "Updated enumeration references: %s -> %s\n", oldQualifiedName, newQualifiedName)
		}
	}

	fmt.Fprintf(ctx.Output, "Moved enumeration %s to new location\n", name.String())
	return nil
}

// moveConstant moves a constant to a new container.
func moveConstant(ctx *ExecContext, name ast.QualifiedName, targetContainerID model.ID) error {
	constants, err := ctx.Backend.ListConstants()
	if err != nil {
		return mdlerrors.NewBackend("list constants", err)
	}

	h, err := getHierarchy(ctx)
	if err != nil {
		return mdlerrors.NewBackend("build hierarchy", err)
	}

	for _, c := range constants {
		modID := h.FindModuleID(c.ContainerID)
		modName := h.GetModuleName(modID)
		if modName == name.Module && c.Name == name.Name {
			c.ContainerID = targetContainerID
			if err := ctx.Backend.MoveConstant(c); err != nil {
				return mdlerrors.NewBackend("move constant", err)
			}
			fmt.Fprintf(ctx.Output, "Moved constant %s to new location\n", name.String())
			return nil
		}
	}

	return mdlerrors.NewNotFound("constant", name.String())
}

// moveDatabaseConnection moves a database connection to a new container.
func moveDatabaseConnection(ctx *ExecContext, name ast.QualifiedName, targetContainerID model.ID) error {
	connections, err := ctx.Backend.ListDatabaseConnections()
	if err != nil {
		return mdlerrors.NewBackend("list database connections", err)
	}

	h, err := getHierarchy(ctx)
	if err != nil {
		return mdlerrors.NewBackend("build hierarchy", err)
	}

	for _, conn := range connections {
		modID := h.FindModuleID(conn.ContainerID)
		modName := h.GetModuleName(modID)
		if modName == name.Module && conn.Name == name.Name {
			conn.ContainerID = targetContainerID
			if err := ctx.Backend.MoveDatabaseConnection(conn); err != nil {
				return mdlerrors.NewBackend("move database connection", err)
			}
			fmt.Fprintf(ctx.Output, "Moved database connection %s to new location\n", name.String())
			return nil
		}
	}

	return mdlerrors.NewNotFound("database connection", name.String())
}

// moveJavaAction moves a Java action to a new container.
//
// Java actions and published OData services had no MOVE doctype at all, and
// neither CREATE form takes a folder clause — so those documents could never
// leave the module root from MDL (mxcli-formula1 #32).
func moveJavaAction(ctx *ExecContext, name ast.QualifiedName, targetContainerID model.ID) error {
	actions, err := ctx.Backend.ListJavaActionsFull()
	if err != nil {
		return mdlerrors.NewBackend("list java actions", err)
	}

	h, err := getHierarchy(ctx)
	if err != nil {
		return mdlerrors.NewBackend("build hierarchy", err)
	}

	for _, ja := range actions {
		modID := h.FindModuleID(ja.ContainerID)
		if h.GetModuleName(modID) == name.Module && ja.Name == name.Name {
			ja.ContainerID = targetContainerID
			if err := ctx.Backend.MoveJavaAction(ja); err != nil {
				return mdlerrors.NewBackend("move java action", err)
			}
			fmt.Fprintf(ctx.Output, "Moved java action %s to new location\n", name.String())
			return nil
		}
	}

	return mdlerrors.NewNotFound("java action", name.String())
}

// movePublishedODataService moves a published OData service to a new container.
func movePublishedODataService(ctx *ExecContext, name ast.QualifiedName, targetContainerID model.ID) error {
	services, err := ctx.Backend.ListPublishedODataServices()
	if err != nil {
		return mdlerrors.NewBackend("list published OData services", err)
	}

	h, err := getHierarchy(ctx)
	if err != nil {
		return mdlerrors.NewBackend("build hierarchy", err)
	}

	for _, svc := range services {
		modID := h.FindModuleID(svc.ContainerID)
		if h.GetModuleName(modID) == name.Module && svc.Name == name.Name {
			svc.ContainerID = targetContainerID
			if err := ctx.Backend.MovePublishedODataService(svc); err != nil {
				return mdlerrors.NewBackend("move published OData service", err)
			}
			fmt.Fprintf(ctx.Output, "Moved odata service %s to new location\n", name.String())
			return nil
		}
	}

	return mdlerrors.NewNotFound("odata service", name.String())
}

// remapEntityAccessRoles points each of the entity's access rules at the target
// module's role of the same name, and warns of a role the target lacks.
func remapEntityAccessRoles(ctx *ExecContext, entity *domainmodel.Entity, sourceModule, targetModule *model.Module) {
	if len(entity.AccessRules) == 0 {
		return
	}
	targetRoles := map[string]bool{}
	if ms, err := ctx.Backend.GetModuleSecurity(targetModule.ID); err == nil && ms != nil {
		for _, r := range ms.ModuleRoles {
			targetRoles[r.Name] = true
		}
	}
	oldPrefix := sourceModule.Name + "."
	var missing []string
	seen := map[string]bool{}
	for _, ar := range entity.AccessRules {
		names := append([]string(nil), ar.ModuleRoleNames...)
		changed := false
		for i, qn := range names {
			if !strings.HasPrefix(qn, oldPrefix) {
				continue
			}
			role := qn[len(oldPrefix):]
			if targetRoles[role] {
				names[i] = targetModule.Name + "." + role
				changed = true
			} else if !seen[qn] {
				seen[qn] = true
				missing = append(missing, qn)
			}
		}
		if changed {
			ar.ModuleRoleNames = names
		}
	}
	for _, qn := range missing {
		fmt.Fprintf(ctx.Output, "Warning: entity %s.%s keeps access-rule role %s: module %s has no role named %s. "+
			"An entity's access rules may only use its own module's roles — create %s.%s and grant again.\n",
			targetModule.Name, entity.Name, qn, targetModule.Name, qn[len(oldPrefix):], targetModule.Name, qn[len(oldPrefix):])
	}
}
