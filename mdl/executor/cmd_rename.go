// SPDX-License-Identifier: Apache-2.0

// Package executor — RENAME commands (entity, module)
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

// execRename handles RENAME statements for all document types.
func execRename(ctx *ExecContext, s *ast.RenameStmt) error {
	if !ctx.Connected() {
		return mdlerrors.NewNotConnected()
	}

	switch s.ObjectType {
	case "entity":
		return execRenameEntity(ctx, s)
	case "microflow":
		return execRenameDocument(ctx, s, "microflow")
	case "nanoflow":
		return execRenameDocument(ctx, s, "nanoflow")
	case "page":
		return execRenameDocument(ctx, s, "page")
	case "snippet":
		return execRenameDocument(ctx, s, "snippet")
	case "enumeration":
		return execRenameEnumeration(ctx, s)
	case "association":
		return execRenameAssociation(ctx, s)
	case "constant":
		return execRenameDocument(ctx, s, "constant")
	case "workflow":
		return execRenameDocument(ctx, s, "workflow")
	case "javaaction":
		return execRenameJavaAction(ctx, s)
	case "module":
		return execRenameModule(ctx, s)
	default:
		return mdlerrors.NewUnsupported(fmt.Sprintf("rename not supported for %s", s.ObjectType))
	}
}

// execRenameEntity renames an entity and updates all BY_NAME references.
func execRenameEntity(ctx *ExecContext, s *ast.RenameStmt) error {
	// Find the entity
	module, err := findModule(ctx, s.Name.Module)
	if err != nil {
		return err
	}

	dm, err := ctx.Backend.GetDomainModel(module.ID)
	if err != nil {
		return mdlerrors.NewBackend("get domain model", err)
	}

	found := false
	collision := false
	for _, ent := range dm.Entities {
		if ent.Name == s.Name.Name {
			found = true
		} else if ent.Name == s.NewName {
			collision = true
		}
	}
	if !found {
		return mdlerrors.NewNotFound("entity", s.Name.String())
	}
	if collision {
		return mdlerrors.NewValidationf("entity %s already exists in module %s", s.NewName, s.Name.Module)
	}

	oldQualifiedName := s.Name.Module + "." + s.Name.Name
	newQualifiedName := s.Name.Module + "." + s.NewName

	// Scan for references
	hits, err := ctx.Backend.RenameReferences(oldQualifiedName, newQualifiedName, s.DryRun)
	if err != nil {
		return mdlerrors.NewBackend("scan references", err)
	}

	if s.DryRun {
		printRenameReport(ctx, oldQualifiedName, newQualifiedName, hits)
		return nil
	}

	// Re-read the domain model: the sweep above rewrote the raw unit, and the
	// semantic model read before it is stale wherever the sweep reached into this
	// module — a same-module `extends Module.Old`, for one. Persisting the stale
	// copy put every one of those names back (ako/mxcli#817).
	dm, err = ctx.Backend.GetDomainModel(module.ID)
	if err != nil {
		return mdlerrors.NewBackend("re-read domain model", err)
	}

	// Update the entity name in the domain model
	isView := false
	for _, ent := range dm.Entities {
		if ent.Name == s.Name.Name {
			ent.Name = s.NewName
			repointEntitySelfRefs(ent, oldQualifiedName, newQualifiedName)
			// A view entity's OQL source document carries the entity's name,
			// and the entity points at it by qualified name. The sweep above
			// rewrites the pointer in the raw unit, but this persist would put
			// the stale one back — the same clobber repointEntitySelfRefs undoes
			// for member names — and the document itself kept the old name:
			// CE6784 "View Entity name is out of sync with the OQL query name".
			if ent.Source == "DomainModels$OqlViewEntitySource" {
				isView = true
				if ent.SourceDocumentRef == oldQualifiedName {
					ent.SourceDocumentRef = newQualifiedName
				}
			}
			break
		}
	}
	if err := ctx.Backend.UpdateDomainModel(dm); err != nil {
		return mdlerrors.NewBackend("update entity name", err)
	}
	if isView {
		if err := ctx.Backend.RenameViewEntitySourceDocument(s.Name.Module, s.Name.Name, s.NewName); err != nil {
			return mdlerrors.NewBackend("rename view entity source document", err)
		}
	}

	invalidateHierarchy(ctx)
	invalidateDomainModelsCache(ctx)

	fmt.Fprintf(ctx.Output, "Renamed entity: %s → %s\n", oldQualifiedName, newQualifiedName)
	if len(hits) > 0 {
		fmt.Fprintf(ctx.Output, "Updated %d reference(s) in %d document(s)\n", totalRefCount(hits), len(hits))
	}
	return nil
}

// repointEntitySelfRefs rewrites the qualified names the entity uses to refer to
// its OWN members, which embed the entity name that has just changed.
//
// It is needed because of a clobber, not because the sweep missed them: the
// project-wide RenameReferences pass above does rewrite these names in the raw
// unit, and then UpdateDomainModel persists the semantic model read BEFORE it and
// puts the stale ones back. Measured on a real 11.13 app, the report and the errors
// line up exactly — "Updated 3 reference(s) in 1 document(s)" followed by three
// CE1613s at "Access rule of entity" and "Validation rule of entity" for the entity
// just renamed. Re-pointing here makes the persist agree with the sweep instead of
// undoing it.
//
// Since ako/mxcli#817 the handler re-reads the domain model after the sweep, which
// covers every name the sweep reaches in the unit (a same-module generalization was
// the one this helper missed). The repoint stays: it is idempotent there, and it is
// what keeps the rename correct against a backend whose sweep is a no-op.
//
// Leaving them stale is worse than a dangling string: entityToGen's
// syncMemberAccesses matches existing entries BY qualified name, so a stale
// `Module.Old.Attr` never equals the rebuilt `Module.New.Attr` and it appends the
// new entry while keeping the old — the entity ends up carrying every member twice,
// half of the entries dangling. DESCRIBE renders members bare, so the duplication is
// the only visible trace. This is the RENAME sibling of the MOVE ENTITY defect in
// ako/mxcli#605, where the same names went stale on the module prefix instead.
//
// Only names qualified by the ENTITY are rewritten. An association member is named
// `Module.Association` — it carries no entity name — so a blanket prefix swap would
// corrupt a reference that is still correct.
func repointEntitySelfRefs(ent *domainmodel.Entity, oldQualifiedName, newQualifiedName string) {
	oldPrefix, newPrefix := oldQualifiedName+".", newQualifiedName+"."
	repoint := func(name string) string {
		if strings.HasPrefix(name, oldPrefix) {
			return newPrefix + name[len(oldPrefix):]
		}
		return name
	}
	for _, ar := range ent.AccessRules {
		for _, ma := range ar.MemberAccesses {
			ma.AttributeName = repoint(ma.AttributeName)
		}
	}
	for _, vr := range ent.ValidationRules {
		vr.AttributeID = model.ID(repoint(string(vr.AttributeID)))
	}
}

// execRenameModule renames a module and updates all BY_NAME references with the module prefix.
func execRenameModule(ctx *ExecContext, s *ast.RenameStmt) error {
	oldModuleName := s.Name.Module
	newModuleName := s.NewName

	module, err := findModule(ctx, oldModuleName)
	if err != nil {
		return err
	}

	// Scan for all references with the old module prefix
	// Module rename replaces "OldModule." with "NewModule." in all qualified names
	hits, err := ctx.Backend.RenameReferences(oldModuleName+".", newModuleName+".", s.DryRun)
	if err != nil {
		return mdlerrors.NewBackend("scan references", err)
	}

	// Also scan for exact module name matches (e.g., in navigation, security role refs)
	exactHits, err := ctx.Backend.RenameReferences(oldModuleName, newModuleName, s.DryRun)
	if err != nil {
		return mdlerrors.NewBackend("scan exact module references", err)
	}

	// Merge hit lists (deduplicate by unit ID)
	allHits := mergeHits(hits, exactHits)

	if s.DryRun {
		printRenameReport(ctx, oldModuleName, newModuleName, allHits)
		return nil
	}

	// Update the module name
	module.Name = newModuleName
	if err := ctx.Backend.UpdateModule(module); err != nil {
		return mdlerrors.NewBackend("update module name", err)
	}

	invalidateHierarchy(ctx)
	invalidateDomainModelsCache(ctx)

	fmt.Fprintf(ctx.Output, "Renamed module: %s → %s\n", oldModuleName, newModuleName)
	if len(allHits) > 0 {
		fmt.Fprintf(ctx.Output, "Updated %d reference(s) in %d document(s)\n", totalRefCount(allHits), len(allHits))
	}
	return nil
}

// execRenameDocument handles RENAME MICROFLOW/NANOFLOW/PAGE/CONSTANT.
// These are standalone documents where the Name field is in the document BSON itself.
// The reference scanner handles updating all BY_NAME references, and then we update
// the document's own Name field via a raw BSON rewrite.
func execRenameDocument(ctx *ExecContext, s *ast.RenameStmt, docType string) error {
	oldQualifiedName := s.Name.Module + "." + s.Name.Name
	newQualifiedName := s.Name.Module + "." + s.NewName

	// Verify the document exists
	h, err := getHierarchy(ctx)
	if err != nil {
		return err
	}

	// docID is the document the rename acts on: the one found here, among the
	// documents of the requested kind. The rename used to look it up again by
	// bare name among all the module's units and take the first, and a folder of
	// the same name — inserted before the document a FOLDER clause puts in it —
	// was renamed instead, after the sweep had already pointed every reference
	// at the new name (ako/mxcli#1443).
	var docID model.ID
	collision := false
	switch docType {
	case "microflow":
		mfs, _ := ctx.Backend.ListMicroflows()
		for _, mf := range mfs {
			modID := h.FindModuleID(mf.ContainerID)
			if h.GetModuleName(modID) != s.Name.Module {
				continue
			}
			if mf.Name == s.Name.Name {
				docID = mf.ID
			} else if mf.Name == s.NewName {
				collision = true
			}
		}
	case "nanoflow":
		nfs, _ := ctx.Backend.ListNanoflows()
		for _, nf := range nfs {
			modID := h.FindModuleID(nf.ContainerID)
			if h.GetModuleName(modID) != s.Name.Module {
				continue
			}
			if nf.Name == s.Name.Name {
				docID = nf.ID
			} else if nf.Name == s.NewName {
				collision = true
			}
		}
	case "page":
		pgs, _ := ctx.Backend.ListPages()
		for _, pg := range pgs {
			modID := h.FindModuleID(pg.ContainerID)
			if h.GetModuleName(modID) != s.Name.Module {
				continue
			}
			if pg.Name == s.Name.Name {
				docID = pg.ID
			} else if pg.Name == s.NewName {
				collision = true
			}
		}
	case "snippet":
		// A snippet call names its snippet by qualified name, so the reference
		// sweep below rewrites the calls in pages, layouts and other snippets
		// (ako/mxcli#2130).
		sns, _ := ctx.Backend.ListSnippets()
		for _, sn := range sns {
			modID := h.FindModuleID(sn.ContainerID)
			if h.GetModuleName(modID) != s.Name.Module {
				continue
			}
			if sn.Name == s.Name.Name {
				docID = sn.ID
			} else if sn.Name == s.NewName {
				collision = true
			}
		}
	case "constant":
		cs, _ := ctx.Backend.ListConstants()
		for _, c := range cs {
			modID := h.FindModuleID(c.ContainerID)
			if h.GetModuleName(modID) != s.Name.Module {
				continue
			}
			if c.Name == s.Name.Name {
				docID = c.ID
			} else if c.Name == s.NewName {
				collision = true
			}
		}
	case "workflow":
		wfs, _ := ctx.Backend.ListWorkflows()
		for _, wf := range wfs {
			modID := h.FindModuleID(wf.ContainerID)
			if h.GetModuleName(modID) != s.Name.Module {
				continue
			}
			if wf.Name == s.Name.Name {
				docID = wf.ID
			} else if wf.Name == s.NewName {
				collision = true
			}
		}
	}

	if docID == "" {
		return mdlerrors.NewNotFound(s.ObjectType, oldQualifiedName)
	}
	if collision {
		return mdlerrors.NewValidationf("%s %s already exists in module %s", docType, s.NewName, s.Name.Module)
	}

	// The document's own Name is a simple name, which the reference sweep below
	// does not match, so it is rewritten separately — and first, so that a
	// rename the backend refuses leaves no reference pointing at a name nothing
	// carries.
	if !s.DryRun {
		if err := ctx.Backend.RenameDocument(docID, s.Name.Name, s.NewName); err != nil {
			return mdlerrors.NewBackend(fmt.Sprintf("rename %s", docType), err)
		}
	}

	hits, err := ctx.Backend.RenameReferences(oldQualifiedName, newQualifiedName, s.DryRun)
	if err != nil {
		return mdlerrors.NewBackend("scan references", err)
	}

	if s.DryRun {
		printRenameReport(ctx, oldQualifiedName, newQualifiedName, hits)
		return nil
	}

	invalidateHierarchy(ctx)

	fmt.Fprintf(ctx.Output, "Renamed %s: %s → %s\n", docType, oldQualifiedName, newQualifiedName)
	if len(hits) > 0 {
		fmt.Fprintf(ctx.Output, "Updated %d reference(s) in %d document(s)\n", totalRefCount(hits), len(hits))
	}
	return nil
}

// execRenameEnumeration renames an enumeration and updates all references.
func execRenameEnumeration(ctx *ExecContext, s *ast.RenameStmt) error {
	// Platform built-in, no stored unit to rename (#1102).
	if err := refuseSystemEnumerationWrite("rename enumeration", s.Name); err != nil {
		return err
	}

	oldQualifiedName := s.Name.Module + "." + s.Name.Name
	newQualifiedName := s.Name.Module + "." + s.NewName

	// Verify it exists
	enums, err := ctx.Backend.ListEnumerations()
	if err != nil {
		return mdlerrors.NewBackend("list enumerations", err)
	}
	h, err := getHierarchy(ctx)
	if err != nil {
		return err
	}
	// Rename the enumeration found here, by ID — not by looking its name up
	// again among units of any type (ako/mxcli#1443).
	var docID model.ID
	collision := false
	for _, en := range enums {
		modID := h.FindModuleID(en.ContainerID)
		if h.GetModuleName(modID) != s.Name.Module {
			continue
		}
		if en.Name == s.Name.Name {
			docID = en.ID
		} else if en.Name == s.NewName {
			collision = true
		}
	}
	if docID == "" {
		return mdlerrors.NewNotFound("enumeration", oldQualifiedName)
	}
	if collision {
		return mdlerrors.NewValidationf("enumeration %s already exists in module %s", s.NewName, s.Name.Module)
	}

	// The document first, as in execRenameDocument: a refused rename then
	// leaves the references alone.
	if !s.DryRun {
		if err := ctx.Backend.RenameDocument(docID, s.Name.Name, s.NewName); err != nil {
			return mdlerrors.NewBackend("rename enumeration", err)
		}
	}

	hits, err := ctx.Backend.RenameReferences(oldQualifiedName, newQualifiedName, s.DryRun)
	if err != nil {
		return mdlerrors.NewBackend("scan references", err)
	}

	if s.DryRun {
		printRenameReport(ctx, oldQualifiedName, newQualifiedName, hits)
		return nil
	}

	// Also update enumeration refs in domain models (attribute types store qualified enum names)
	if err := ctx.Backend.UpdateEnumerationRefsInAllDomainModels(oldQualifiedName, newQualifiedName); err != nil {
		fmt.Fprintf(ctx.Output, "Warning: failed to update enumeration references in domain models: %v\n", err)
	}

	invalidateHierarchy(ctx)
	invalidateDomainModelsCache(ctx)

	fmt.Fprintf(ctx.Output, "Renamed enumeration: %s → %s\n", oldQualifiedName, newQualifiedName)
	if len(hits) > 0 {
		fmt.Fprintf(ctx.Output, "Updated %d reference(s) in %d document(s)\n", totalRefCount(hits), len(hits))
	}
	return nil
}

// execRenameAssociation renames an association and updates all references.
func execRenameAssociation(ctx *ExecContext, s *ast.RenameStmt) error {
	oldQualifiedName := s.Name.Module + "." + s.Name.Name
	newQualifiedName := s.Name.Module + "." + s.NewName

	module, err := findModule(ctx, s.Name.Module)
	if err != nil {
		return err
	}

	dm, err := ctx.Backend.GetDomainModel(module.ID)
	if err != nil {
		return mdlerrors.NewBackend("get domain model", err)
	}

	// A cross-module association is stored in the FROM module's CrossAssociations,
	// not its Associations; looking only at the latter reported one as "not
	// found" (ako/mxcli#803). Both lists share one namespace for the collision
	// check.
	found := false
	collision := false
	for _, assoc := range dm.Associations {
		if assoc.Name == s.Name.Name {
			found = true
		} else if assoc.Name == s.NewName {
			collision = true
		}
	}
	for _, ca := range dm.CrossAssociations {
		if ca.Name == s.Name.Name {
			found = true
		} else if ca.Name == s.NewName {
			collision = true
		}
	}
	if !found {
		return mdlerrors.NewNotFound("association", oldQualifiedName)
	}
	if collision {
		return mdlerrors.NewValidationf("association %s already exists in module %s", s.NewName, s.Name.Module)
	}
	// The new name must also be free on its owners' generalization chains
	// (CE0069, ako/mxcli#1645).
	if err := refuseMemberNameClash(ctx, s); err != nil {
		return err
	}

	hits, err := ctx.Backend.RenameReferences(oldQualifiedName, newQualifiedName, s.DryRun)
	if err != nil {
		return mdlerrors.NewBackend("scan references", err)
	}

	if s.DryRun {
		printRenameReport(ctx, oldQualifiedName, newQualifiedName, hits)
		return nil
	}

	// Re-read after the sweep, for the same reason as execRenameEntity (#817).
	dm, err = ctx.Backend.GetDomainModel(module.ID)
	if err != nil {
		return mdlerrors.NewBackend("re-read domain model", err)
	}

	// Update association name in domain model. The association keeps its $ID and,
	// through UpdateDomainModel's carry (#1169) and the in-place cross-association
	// patch (#792), its storage GUID.
	renamed := false
	for _, assoc := range dm.Associations {
		if assoc.Name == s.Name.Name {
			assoc.Name = s.NewName
			renamed = true
			break
		}
	}
	if !renamed {
		for _, ca := range dm.CrossAssociations {
			if ca.Name == s.Name.Name {
				ca.Name = s.NewName
				break
			}
		}
	}
	repointAssociationMemberRefs(dm, oldQualifiedName, newQualifiedName)
	if err := ctx.Backend.UpdateDomainModel(dm); err != nil {
		return mdlerrors.NewBackend("update association name", err)
	}

	invalidateHierarchy(ctx)
	invalidateDomainModelsCache(ctx)

	fmt.Fprintf(ctx.Output, "Renamed association: %s → %s\n", oldQualifiedName, newQualifiedName)
	if len(hits) > 0 {
		fmt.Fprintf(ctx.Output, "Updated %d reference(s) in %d document(s)\n", totalRefCount(hits), len(hits))
	}
	return nil
}

// repointAssociationMemberRefs rewrites the entity access rules in this unit that
// name the renamed association, which they do by qualified name.
//
// Same clobber as repointEntitySelfRefs, one document type over: RenameReferences
// rewrites these in the raw unit and the UpdateDomainModel that follows puts the
// stale name back. It was the last of the four CE1613s a RENAME ASSOCIATION followed
// by a RENAME ENTITY left on a real 11.13 app, and the one that survives longest,
// because the stale name is re-read by every later statement that loads the unit.
//
// The match is EXACT rather than a prefix: an association member is named
// `Module.Association` with nothing after it, and a prefix match would also rewrite
// a differently-named association that happens to start with the same text.
//
// Every entity is walked, not just the association's FROM side. Mendix stores a
// MemberAccess for an association only on the FROM entity (adding one to the TO
// entity is CE0066), so in a well-formed model this finds them all on one entity —
// but a sweep that assumed the storage rule would silently skip anything a model
// carries in spite of it.
func repointAssociationMemberRefs(dm *domainmodel.DomainModel, oldQualifiedName, newQualifiedName string) {
	if dm == nil {
		return
	}
	for _, ent := range dm.Entities {
		for _, ar := range ent.AccessRules {
			for _, ma := range ar.MemberAccesses {
				if ma.AssociationName == oldQualifiedName {
					ma.AssociationName = newQualifiedName
				}
			}
		}
	}
}

// execRenameJavaAction renames a Java action and its .java source file.
func execRenameJavaAction(ctx *ExecContext, s *ast.RenameStmt) error {
	oldQualifiedName := s.Name.Module + "." + s.Name.Name
	newQualifiedName := s.Name.Module + "." + s.NewName

	// Verify the Java action exists
	jas, err := ctx.Backend.ListJavaActions()
	if err != nil {
		return mdlerrors.NewBackend("list java actions", err)
	}
	h, err := getHierarchy(ctx)
	if err != nil {
		return err
	}
	// Rename the Java action found here, by ID — not by looking its name up
	// again among units of any type (ako/mxcli#1443).
	var docID model.ID
	collision := false
	for _, ja := range jas {
		modID := h.FindModuleID(ja.ContainerID)
		if h.GetModuleName(modID) != s.Name.Module {
			continue
		}
		if ja.Name == s.Name.Name {
			docID = ja.ID
		} else if ja.Name == s.NewName {
			collision = true
		}
	}
	if docID == "" {
		return mdlerrors.NewNotFound("java action", oldQualifiedName)
	}
	if collision {
		return mdlerrors.NewValidationf("java action %s already exists in module %s", s.NewName, s.Name.Module)
	}

	// The document first, as in execRenameDocument: a refused rename then
	// leaves the references alone.
	if !s.DryRun {
		if err := ctx.Backend.RenameDocument(docID, s.Name.Name, s.NewName); err != nil {
			return mdlerrors.NewBackend("rename java action", err)
		}
	}

	hits, err := ctx.Backend.RenameReferences(oldQualifiedName, newQualifiedName, s.DryRun)
	if err != nil {
		return mdlerrors.NewBackend("scan references", err)
	}

	if s.DryRun {
		printRenameReport(ctx, oldQualifiedName, newQualifiedName, hits)
		return nil
	}

	if err := ctx.Backend.RenameJavaSourceFile(s.Name.Module, s.Name.Name, s.NewName); err != nil {
		return mdlerrors.NewBackend("rename java source file", err)
	}

	invalidateHierarchy(ctx)

	fmt.Fprintf(ctx.Output, "Renamed java action: %s → %s\n", oldQualifiedName, newQualifiedName)
	if len(hits) > 0 {
		fmt.Fprintf(ctx.Output, "Updated %d reference(s) in %d document(s)\n", totalRefCount(hits), len(hits))
	}
	return nil
}

// printRenameReport outputs a dry-run report of what would change.
func printRenameReport(ctx *ExecContext, oldName, newName string, hits []types.RenameHit) {
	fmt.Fprintf(ctx.Output, "Would rename: %s → %s\n", oldName, newName)
	fmt.Fprintf(ctx.Output, "References found: %d in %d document(s)\n", totalRefCount(hits), len(hits))

	for _, h := range hits {
		label := h.Name
		if label == "" {
			label = h.UnitID
		}
		typeName := h.UnitType
		if idx := strings.Index(typeName, "$"); idx >= 0 {
			typeName = typeName[idx+1:]
		}
		fmt.Fprintf(ctx.Output, "  %s (%s) — %d reference(s)\n", label, typeName, h.Count)
	}
}

func totalRefCount(hits []types.RenameHit) int {
	total := 0
	for _, h := range hits {
		total += h.Count
	}
	return total
}

func mergeHits(a, b []types.RenameHit) []types.RenameHit {
	seen := make(map[string]int) // unitID → index in result
	result := make([]types.RenameHit, len(a))
	copy(result, a)
	for i := range result {
		seen[result[i].UnitID] = i
	}
	for _, h := range b {
		if idx, ok := seen[h.UnitID]; ok {
			result[idx].Count += h.Count
		} else {
			seen[h.UnitID] = len(result)
			result = append(result, h)
		}
	}
	return result
}
