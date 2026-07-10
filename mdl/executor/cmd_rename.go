// SPDX-License-Identifier: Apache-2.0

// Package executor — RENAME commands (entity, module)
package executor

import (
	"fmt"
	"strings"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
	mdlerrors "github.com/JordtenBulte-OLC/mxcli/mdl/errors"
	"github.com/JordtenBulte-OLC/mxcli/mdl/types"
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

	// Update the entity name in the domain model
	for _, ent := range dm.Entities {
		if ent.Name == s.Name.Name {
			ent.Name = s.NewName
			break
		}
	}
	if err := ctx.Backend.UpdateDomainModel(dm); err != nil {
		return mdlerrors.NewBackend("update entity name", err)
	}

	invalidateHierarchy(ctx)
	invalidateDomainModelsCache(ctx)

	fmt.Fprintf(ctx.Output, "Renamed entity: %s → %s\n", oldQualifiedName, newQualifiedName)
	if len(hits) > 0 {
		fmt.Fprintf(ctx.Output, "Updated %d reference(s) in %d document(s)\n", totalRefCount(hits), len(hits))
	}
	return nil
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

	found := false
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
				found = true
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
				found = true
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
				found = true
			} else if pg.Name == s.NewName {
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
				found = true
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
				found = true
			} else if wf.Name == s.NewName {
				collision = true
			}
		}
	}

	if !found {
		return mdlerrors.NewNotFound(s.ObjectType, oldQualifiedName)
	}
	if collision {
		return mdlerrors.NewValidationf("%s %s already exists in module %s", docType, s.NewName, s.Name.Module)
	}

	// The reference scanner will also update the document's own Name field
	// when it matches the old qualified name. But the Name field is just the
	// simple name (e.g., "OldName"), not the qualified name. So we need to
	// handle it separately — the scanner updates cross-references, and we
	// update the Name field directly.
	hits, err := ctx.Backend.RenameReferences(oldQualifiedName, newQualifiedName, s.DryRun)
	if err != nil {
		return mdlerrors.NewBackend("scan references", err)
	}

	if s.DryRun {
		printRenameReport(ctx, oldQualifiedName, newQualifiedName, hits)
		return nil
	}

	// Update the document's own Name field via the raw BSON name updater
	if err := ctx.Backend.RenameDocumentByName(s.Name.Module, s.Name.Name, s.NewName); err != nil {
		return mdlerrors.NewBackend(fmt.Sprintf("rename %s", docType), err)
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
	found := false
	collision := false
	for _, en := range enums {
		modID := h.FindModuleID(en.ContainerID)
		if h.GetModuleName(modID) != s.Name.Module {
			continue
		}
		if en.Name == s.Name.Name {
			found = true
		} else if en.Name == s.NewName {
			collision = true
		}
	}
	if !found {
		return mdlerrors.NewNotFound("enumeration", oldQualifiedName)
	}
	if collision {
		return mdlerrors.NewValidationf("enumeration %s already exists in module %s", s.NewName, s.Name.Module)
	}

	hits, err := ctx.Backend.RenameReferences(oldQualifiedName, newQualifiedName, s.DryRun)
	if err != nil {
		return mdlerrors.NewBackend("scan references", err)
	}

	if s.DryRun {
		printRenameReport(ctx, oldQualifiedName, newQualifiedName, hits)
		return nil
	}

	// Update enumeration name via raw BSON
	if err := ctx.Backend.RenameDocumentByName(s.Name.Module, s.Name.Name, s.NewName); err != nil {
		return mdlerrors.NewBackend("rename enumeration", err)
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

	found := false
	collision := false
	for _, assoc := range dm.Associations {
		if assoc.Name == s.Name.Name {
			found = true
		} else if assoc.Name == s.NewName {
			collision = true
		}
	}
	if !found {
		return mdlerrors.NewNotFound("association", oldQualifiedName)
	}
	if collision {
		return mdlerrors.NewValidationf("association %s already exists in module %s", s.NewName, s.Name.Module)
	}

	hits, err := ctx.Backend.RenameReferences(oldQualifiedName, newQualifiedName, s.DryRun)
	if err != nil {
		return mdlerrors.NewBackend("scan references", err)
	}

	if s.DryRun {
		printRenameReport(ctx, oldQualifiedName, newQualifiedName, hits)
		return nil
	}

	// Update association name in domain model
	for _, assoc := range dm.Associations {
		if assoc.Name == s.Name.Name {
			assoc.Name = s.NewName
			break
		}
	}
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
	found := false
	collision := false
	for _, ja := range jas {
		modID := h.FindModuleID(ja.ContainerID)
		if h.GetModuleName(modID) != s.Name.Module {
			continue
		}
		if ja.Name == s.Name.Name {
			found = true
		} else if ja.Name == s.NewName {
			collision = true
		}
	}
	if !found {
		return mdlerrors.NewNotFound("java action", oldQualifiedName)
	}
	if collision {
		return mdlerrors.NewValidationf("java action %s already exists in module %s", s.NewName, s.Name.Module)
	}

	hits, err := ctx.Backend.RenameReferences(oldQualifiedName, newQualifiedName, s.DryRun)
	if err != nil {
		return mdlerrors.NewBackend("scan references", err)
	}

	if s.DryRun {
		printRenameReport(ctx, oldQualifiedName, newQualifiedName, hits)
		return nil
	}

	if err := ctx.Backend.RenameDocumentByName(s.Name.Module, s.Name.Name, s.NewName); err != nil {
		return mdlerrors.NewBackend("rename java action", err)
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
