// SPDX-License-Identifier: Apache-2.0

// Package executor - Module commands (SHOW/CREATE/DROP MODULES)
package executor

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/modelsdk/meta"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
	"github.com/mendixlabs/mxcli/sdk/security"
)

// execCreateModule handles CREATE MODULE statements.
func execCreateModule(ctx *ExecContext, s *ast.CreateModuleStmt) error {
	if !ctx.Connected() {
		return mdlerrors.NewNotConnected()
	}

	// Check if module already exists
	modules, err := ctx.Backend.ListModules()
	if err != nil {
		return mdlerrors.NewBackend("list modules", err)
	}

	for _, m := range modules {
		if m.Name == s.Name {
			// A plain create of an existing module stays the no-op it always
			// was; `or modify` applies the documentation the statement states.
			if s.CreateOrModify {
				if err := setModuleSettings(ctx, m, s.Settings); err != nil {
					return err
				}
				if s.DocumentationSet {
					return setModuleDocumentation(ctx, m, s.Documentation)
				}
				if !s.Settings.IsEmpty() {
					return nil
				}
			}
			if s.DocumentationSet || !s.Settings.IsEmpty() {
				fmt.Fprintf(ctx.Output, "Module '%s' already exists; documentation and settings unchanged "+
					"(create or modify module sets them)\n", s.Name)
				return nil
			}
			fmt.Fprintf(ctx.Output, "Module '%s' already exists\n", s.Name)
			return nil
		}
	}

	// Create the module
	module := &model.Module{
		Name: s.Name,
	}

	if err := ctx.Backend.CreateModule(module); err != nil {
		return mdlerrors.NewBackend("create module", err)
	}

	// Invalidate cache so new module is visible
	invalidateModuleCache(ctx)

	fmt.Fprintf(ctx.Output, "Created module: %s\n", s.Name)
	if !s.Settings.IsEmpty() {
		// The new module's settings unit holds the defaults; the stated ones
		// are written onto it, so its PackageId and the rest are kept.
		if err := setModuleSettings(ctx, module, s.Settings); err != nil {
			return err
		}
	}
	if s.DocumentationSet && s.Documentation != "" {
		return setModuleDocumentation(ctx, module, s.Documentation)
	}
	return nil
}

// setModuleDocumentation stores a module's doc comment as its domain model's
// documentation: a Mendix module has no documentation property, its
// DomainModels$DomainModel does, and that is what lint reads as
// modules().domain_model_documentation (mendixlabs/mxcli#1314).
func setModuleDocumentation(ctx *ExecContext, module *model.Module, doc string) error {
	dm, err := ctx.Backend.GetDomainModel(module.ID)
	if err != nil {
		return mdlerrors.NewBackend(fmt.Sprintf("read domain model of %s", module.Name), err)
	}
	if dm == nil {
		return mdlerrors.NewNotFound("domain model of module", module.Name)
	}
	if err := ctx.Backend.SetDomainModelDocumentation(dm.ID, doc); err != nil {
		return mdlerrors.NewBackend(fmt.Sprintf("set documentation of %s", module.Name), err)
	}
	fmt.Fprintf(ctx.Output, "Set documentation of module: %s\n", module.Name)
	return nil
}

// execDropModule handles DROP MODULE statements.
// This cascades to delete all objects inside the module:
// - Enumerations
// - Entities and Associations (in domain models)
// - Microflows
// - Nanoflows
// - Pages
// - Snippets
// - Constants
func execDropModule(ctx *ExecContext, s *ast.DropModuleStmt) error {
	if !ctx.Connected() {
		return mdlerrors.NewNotConnected()
	}

	// Find the module
	modules, err := ctx.Backend.ListModules()
	if err != nil {
		return mdlerrors.NewBackend("list modules", err)
	}

	var targetModule *model.Module
	for _, m := range modules {
		if m.Name == s.Name {
			targetModule = m
			break
		}
	}

	if targetModule == nil {
		return mdlerrors.NewNotFound("module", s.Name)
	}

	// The System module is virtual: it is synthesized from modelsdk/meta, not
	// stored, so there is nothing here to drop. The cascade below would walk its
	// synthesized documents and report a "unit not found" warning for each one
	// (15 of them once the enumerations became visible — #1102) while changing
	// nothing. Refusing says that in one line instead.
	if targetModule.Name == "System" {
		return mdlerrors.NewValidation(
			"cannot drop module System: it is owned by the Mendix platform and is not stored in the " +
				"project — its entities, associations, enumerations and Java actions are built in. " +
				"Every Mendix app has it and no app can remove it.")
	}

	// Build set of all container IDs belonging to this module (including nested folders)
	moduleContainers := getModuleContainers(ctx, targetModule.ID)

	// Counters for summary
	var nEnums, nEntities, nAssocs, nMicroflows, nNanoflows, nPages, nSnippets, nLayouts, nConstants, nJavaActions, nServices, nBizEvents, nDbConns int

	// Delete enumerations in this module
	if enums, err := ctx.Backend.ListEnumerations(); err == nil {
		for _, enum := range enums {
			if moduleContainers[enum.ContainerID] {
				if err := ctx.Backend.DeleteEnumeration(enum.ID); err != nil {
					fmt.Fprintf(ctx.Output, "Warning: failed to delete enumeration %s: %v\n", enum.Name, err)
				} else {
					nEnums++
				}
			}
		}
	}

	// Delete entities in domain models belonging to this module
	if dms, err := ctx.Backend.ListDomainModels(); err == nil {
		for _, dm := range dms {
			if moduleContainers[dm.ContainerID] {
				// Delete all associations in this domain model first (they reference entities)
				for _, assoc := range dm.Associations {
					if err := ctx.Backend.DeleteAssociation(dm.ID, assoc.ID); err != nil {
						fmt.Fprintf(ctx.Output, "Warning: failed to delete association %s: %v\n", assoc.Name, err)
					} else {
						nAssocs++
					}
				}
				// Delete all entities in this domain model
				for _, entity := range dm.Entities {
					if err := ctx.Backend.DeleteEntity(dm.ID, entity.ID); err != nil {
						fmt.Fprintf(ctx.Output, "Warning: failed to delete entity %s: %v\n", entity.Name, err)
					} else {
						nEntities++
					}
				}
			}
		}
	}

	// Delete microflows in this module
	if mfs, err := ctx.Backend.ListMicroflows(); err == nil {
		for _, mf := range mfs {
			if moduleContainers[mf.ContainerID] {
				if err := ctx.Backend.DeleteMicroflow(mf.ID); err != nil {
					fmt.Fprintf(ctx.Output, "Warning: failed to delete microflow %s: %v\n", mf.Name, err)
				} else {
					nMicroflows++
				}
			}
		}
	}

	// Delete nanoflows in this module
	if nfs, err := ctx.Backend.ListNanoflows(); err == nil {
		for _, nf := range nfs {
			if moduleContainers[nf.ContainerID] {
				if err := ctx.Backend.DeleteNanoflow(nf.ID); err != nil {
					fmt.Fprintf(ctx.Output, "Warning: failed to delete nanoflow %s: %v\n", nf.Name, err)
				} else {
					nNanoflows++
				}
			}
		}
	}

	// Delete pages in this module
	if pages, err := ctx.Backend.ListPages(); err == nil {
		for _, page := range pages {
			if moduleContainers[page.ContainerID] {
				if err := ctx.Backend.DeletePage(page.ID); err != nil {
					fmt.Fprintf(ctx.Output, "Warning: failed to delete page %s: %v\n", page.Name, err)
				} else {
					nPages++
				}
			}
		}
	}

	// Delete snippets in this module
	if snippets, err := ctx.Backend.ListSnippets(); err == nil {
		for _, snippet := range snippets {
			if moduleContainers[snippet.ContainerID] {
				if err := ctx.Backend.DeleteSnippet(snippet.ID); err != nil {
					fmt.Fprintf(ctx.Output, "Warning: failed to delete snippet %s: %v\n", snippet.Name, err)
				} else {
					nSnippets++
				}
			}
		}
	}

	// Delete constants in this module
	if constants, err := ctx.Backend.ListConstants(); err == nil {
		for _, c := range constants {
			if moduleContainers[c.ContainerID] {
				if err := ctx.Backend.DeleteConstant(c.ID); err != nil {
					fmt.Fprintf(ctx.Output, "Warning: failed to delete constant %s: %v\n", c.Name, err)
				} else {
					nConstants++
				}
			}
		}
	}

	// Delete layouts in this module
	if layouts, err := ctx.Backend.ListLayouts(); err == nil {
		for _, l := range layouts {
			if moduleContainers[l.ContainerID] {
				if err := ctx.Backend.DeleteLayout(l.ID); err != nil {
					fmt.Fprintf(ctx.Output, "Warning: failed to delete layout %s: %v\n", l.Name, err)
				} else {
					nLayouts++
				}
			}
		}
	}

	// Delete Java actions in this module
	if jas, err := ctx.Backend.ListJavaActions(); err == nil {
		for _, ja := range jas {
			if moduleContainers[ja.ContainerID] {
				if err := ctx.Backend.DeleteJavaAction(ja.ID); err != nil {
					fmt.Fprintf(ctx.Output, "Warning: failed to delete Java action %s: %v\n", ja.Name, err)
				} else {
					nJavaActions++
				}
			}
		}
	}

	// Delete business event services in this module
	if services, err := ctx.Backend.ListBusinessEventServices(); err == nil {
		for _, svc := range services {
			if moduleContainers[svc.ContainerID] {
				if err := ctx.Backend.DeleteBusinessEventService(svc.ID); err != nil {
					fmt.Fprintf(ctx.Output, "Warning: failed to delete business event service %s: %v\n", svc.Name, err)
				} else {
					nBizEvents++
				}
			}
		}
	}

	// Delete database connections in this module
	if conns, err := ctx.Backend.ListDatabaseConnections(); err == nil {
		for _, conn := range conns {
			if moduleContainers[conn.ContainerID] {
				if err := ctx.Backend.DeleteDatabaseConnection(conn.ID); err != nil {
					fmt.Fprintf(ctx.Output, "Warning: failed to delete database connection %s: %v\n", conn.Name, err)
				} else {
					nDbConns++
				}
			}
		}
	}

	// Delete consumed OData services (clients) in this module
	if services, err := ctx.Backend.ListConsumedODataServices(); err == nil {
		for _, svc := range services {
			if moduleContainers[svc.ContainerID] {
				if err := ctx.Backend.DeleteConsumedODataService(svc.ID); err != nil {
					fmt.Fprintf(ctx.Output, "Warning: failed to delete OData client %s: %v\n", svc.Name, err)
				} else {
					nServices++
				}
			}
		}
	}

	// Delete published OData services in this module
	if services, err := ctx.Backend.ListPublishedODataServices(); err == nil {
		for _, svc := range services {
			if moduleContainers[svc.ContainerID] {
				if err := ctx.Backend.DeletePublishedODataService(svc.ID); err != nil {
					fmt.Fprintf(ctx.Output, "Warning: failed to delete OData service %s: %v\n", svc.Name, err)
				} else {
					nServices++
				}
			}
		}
	}

	// Remove module roles from user roles in ProjectSecurity
	if ms, err := ctx.Backend.GetModuleSecurity(targetModule.ID); err == nil {
		if ps, err := ctx.Backend.GetProjectSecurity(); err == nil {
			for _, mr := range ms.ModuleRoles {
				qualifiedRole := s.Name + "." + mr.Name
				if n, err := ctx.Backend.RemoveModuleRoleFromAllUserRoles(ps.ID, qualifiedRole); err == nil && n > 0 {
					fmt.Fprintf(ctx.Output, "Removed %s from %d user role(s)\n", qualifiedRole, n)
				}
			}
			if err := pruneInvalidUserRoles(ctx, ps); err != nil {
				return mdlerrors.NewBackend("cleanup invalid user roles", err)
			}
		}
	}

	// Delete the module itself (and clean up themesource directory)
	if err := ctx.Backend.DeleteModuleWithCleanup(targetModule.ID, s.Name); err != nil {
		return mdlerrors.NewBackend("delete module", err)
	}

	// Build summary of what was removed
	var parts []string
	if nEntities > 0 {
		parts = append(parts, fmt.Sprintf("%d entities", nEntities))
	}
	if nAssocs > 0 {
		parts = append(parts, fmt.Sprintf("%d associations", nAssocs))
	}
	if nEnums > 0 {
		parts = append(parts, fmt.Sprintf("%d enumerations", nEnums))
	}
	if nMicroflows > 0 {
		parts = append(parts, fmt.Sprintf("%d microflows", nMicroflows))
	}
	if nNanoflows > 0 {
		parts = append(parts, fmt.Sprintf("%d nanoflows", nNanoflows))
	}
	if nPages > 0 {
		parts = append(parts, fmt.Sprintf("%d pages", nPages))
	}
	if nSnippets > 0 {
		parts = append(parts, fmt.Sprintf("%d snippets", nSnippets))
	}
	if nLayouts > 0 {
		parts = append(parts, fmt.Sprintf("%d layouts", nLayouts))
	}
	if nConstants > 0 {
		parts = append(parts, fmt.Sprintf("%d constants", nConstants))
	}
	if nJavaActions > 0 {
		parts = append(parts, fmt.Sprintf("%d java actions", nJavaActions))
	}
	if nServices > 0 {
		parts = append(parts, fmt.Sprintf("%d OData services", nServices))
	}
	if nBizEvents > 0 {
		parts = append(parts, fmt.Sprintf("%d business event services", nBizEvents))
	}
	if nDbConns > 0 {
		parts = append(parts, fmt.Sprintf("%d database connections", nDbConns))
	}

	if len(parts) > 0 {
		fmt.Fprintf(ctx.Output, "Dropped module: %s (%s)\n", s.Name, strings.Join(parts, ", "))
	} else {
		fmt.Fprintf(ctx.Output, "Dropped module: %s (empty)\n", s.Name)
	}
	return nil
}

// Executor method wrapper — kept during migration for callers not yet

// getModuleContainers returns a set of all container IDs that belong to a module
// (including nested folders).
func getModuleContainers(ctx *ExecContext, moduleID model.ID) map[model.ID]bool {
	containers := make(map[model.ID]bool)
	containers[moduleID] = true

	// Build parent -> children map from units
	units, err := ctx.Backend.ListUnits()
	if err != nil {
		return containers
	}

	childrenOf := make(map[model.ID][]model.ID)
	for _, u := range units {
		childrenOf[u.ContainerID] = append(childrenOf[u.ContainerID], u.ID)
	}

	// Also include folders
	folders, _ := ctx.Backend.ListFolders()
	for _, f := range folders {
		childrenOf[f.ContainerID] = append(childrenOf[f.ContainerID], f.ID)
	}

	// BFS to find all containers under this module
	queue := []model.ID{moduleID}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		for _, child := range childrenOf[current] {
			if !containers[child] {
				containers[child] = true
				queue = append(queue, child)
			}
		}
	}

	return containers
}

// listModules handles SHOW MODULES command.
func listModules(ctx *ExecContext) error {
	if !ctx.Connected() {
		return mdlerrors.NewNotConnected()
	}

	// Always get fresh module list and update cache
	invalidateModuleCache(ctx)
	modules, err := getModulesFromCache(ctx)
	if err != nil {
		return mdlerrors.NewBackend("list modules", err)
	}

	// Get hierarchy for module resolution
	h, err := getHierarchy(ctx)
	if err != nil {
		return mdlerrors.NewBackend("build hierarchy", err)
	}

	// Get units for type-based counting
	units, err := ctx.Backend.ListUnits()
	if err != nil {
		return mdlerrors.NewBackend("list units", err)
	}

	// Count elements per module using unit types
	entityCounts := make(map[model.ID]int)
	enumCounts := make(map[model.ID]int)
	pageCounts := make(map[model.ID]int)
	snippetCounts := make(map[model.ID]int)
	microflowCounts := make(map[model.ID]int)
	nanoflowCounts := make(map[model.ID]int)
	constantCounts := make(map[model.ID]int)
	javaActionCounts := make(map[model.ID]int)
	workflowCounts := make(map[model.ID]int)
	pubRestCounts := make(map[model.ID]int)
	pubODataCounts := make(map[model.ID]int)
	conODataCounts := make(map[model.ID]int)
	bizEventCounts := make(map[model.ID]int)
	extDbCounts := make(map[model.ID]int)

	// Count from the unit types in one pass. Every count but entities is a
	// count of units of one type, which the unit listing knows without opening
	// a document; listing each type through its decoder read every page,
	// snippet, flow, constant and Java action of the project (ako/mxcli#1791).
	for _, u := range units {
		modID := h.FindModuleID(u.ContainerID)
		switch u.Type {
		case "Enumerations$Enumeration":
			enumCounts[modID]++
		case "Forms$Page":
			pageCounts[modID]++
		case "Forms$Snippet":
			snippetCounts[modID]++
		case "Microflows$Microflow":
			microflowCounts[modID]++
		case "Microflows$Nanoflow":
			nanoflowCounts[modID]++
		case "Constants$Constant":
			constantCounts[modID]++
		case "JavaActions$JavaAction":
			javaActionCounts[modID]++
		}
		switch {
		case strings.HasPrefix(u.Type, "Rest$PublishedRestService"):
			pubRestCounts[modID]++
		case strings.HasPrefix(u.Type, "ODataPublish$PublishedODataService"):
			pubODataCounts[modID]++
		case strings.HasPrefix(u.Type, "Rest$ConsumedODataService"):
			conODataCounts[modID]++
		case strings.HasPrefix(u.Type, "BusinessEvents$"):
			bizEventCounts[modID]++
		case strings.HasPrefix(u.Type, "Workflows$Workflow"):
			workflowCounts[modID]++
		case strings.HasPrefix(u.Type, "DatabaseConnector$DatabaseConnection"):
			extDbCounts[modID]++
		}
	}

	// The System module's enumerations and Java actions are platform
	// built-ins with no unit; the backend's listings add them, so count them
	// the same way.
	for _, e := range meta.BuildSystemEnumerations() {
		enumCounts[h.FindModuleID(e.ContainerID)]++
	}
	for _, ja := range meta.BuildSystemJavaActions() {
		javaActionCounts[h.FindModuleID(ja.ContainerID)]++
	}

	// Count entities from domain models: one unit per module holds them all.
	if dms, err := ctx.Backend.ListDomainModels(); err == nil {
		for _, dm := range dms {
			modID := h.FindModuleID(dm.ContainerID)
			entityCounts[modID] += len(dm.Entities)
		}
	}

	// Sort modules alphabetically by name
	sort.Slice(modules, func(i, j int) bool {
		return strings.ToLower(modules[i].Name) < strings.ToLower(modules[j].Name)
	})

	// Collect rows and calculate column widths
	type row struct {
		name        string
		source      string
		version     string
		moduleType  string
		entities    int
		enums       int
		pages       int
		snippets    int
		microflows  int
		nanoflows   int
		workflows   int
		constants   int
		javaActions int
		pubRest     int
		pubOData    int
		conOData    int
		bizEvents   int
		extDb       int
	}
	var rows []row
	protections := moduleProtections(ctx)
	// Version and Type are the General tab of the Module settings dialog
	// (ako/mxcli#1921).
	settingsByModule := map[model.ID]*types.ModuleSettings{}
	if all, err := ctx.Backend.ListModuleSettings(); err == nil {
		for _, ms := range all {
			settingsByModule[ms.ContainerID] = ms
		}
	}

	for _, m := range modules {
		// Determine source (AppStore with version or local)
		source := ""
		if m.FromAppStore {
			if m.AppStoreVersion != "" {
				source = "Marketplace v" + m.AppStoreVersion
			} else {
				source = "Marketplace"
			}
		}
		if m.PackageFile != "" {
			// ako/mxcli#1780: held as modules/<Name>.mxmodule, not as units.
			if source != "" {
				source += ", "
			}
			source += "Packaged"
		}
		if kind := protections[string(m.ID)]; kind != "" {
			// mendixlabs/mxcli#1498: its Hidden elements cannot be referenced
			// from other modules, which nothing else printed showed.
			if source != "" {
				source += ", "
			}
			source += "Protected " + kind
		}

		version, moduleType := "", ""
		if ms := settingsByModule[m.ID]; ms != nil {
			version = ms.Version
			switch moduleTypeOf(ms) {
			case moduleTypeApp:
				moduleType = "App"
			case moduleTypeAddOn:
				moduleType = "Add-on"
			default:
				moduleType = ms.ProtectedModuleType
			}
		}
		r := row{
			name:        m.Name,
			source:      source,
			version:     version,
			moduleType:  moduleType,
			entities:    entityCounts[m.ID],
			enums:       enumCounts[m.ID],
			pages:       pageCounts[m.ID],
			snippets:    snippetCounts[m.ID],
			microflows:  microflowCounts[m.ID],
			nanoflows:   nanoflowCounts[m.ID],
			workflows:   workflowCounts[m.ID],
			constants:   constantCounts[m.ID],
			javaActions: javaActionCounts[m.ID],
			pubRest:     pubRestCounts[m.ID],
			pubOData:    pubODataCounts[m.ID],
			conOData:    conODataCounts[m.ID],
			bizEvents:   bizEventCounts[m.ID],
			extDb:       extDbCounts[m.ID],
		}
		rows = append(rows, r)
	}

	// Build TableResult
	result := &TableResult{
		Columns: []string{"Module", "Source", "Version", "Type", "Entities", "Enums", "Pages", "Snippets", "Microflows", "Nanoflows", "Workflows", "Constants", "JavaActions", "PubREST", "PubOData", "ConOData", "BizEvents", "ExtDB"},
		Summary: fmt.Sprintf("(%d modules)", len(modules)),
	}
	for _, r := range rows {
		result.Rows = append(result.Rows, []any{r.name, r.source, r.version, r.moduleType, r.entities, r.enums, r.pages, r.snippets, r.microflows, r.nanoflows, r.workflows, r.constants, r.javaActions, r.pubRest, r.pubOData, r.conOData, r.bizEvents, r.extDb})
	}
	return writeResult(ctx, result)
}

// describeModule handles DESCRIBE MODULE [WITH ALL] command.
func describeModule(ctx *ExecContext, moduleName string, withAll bool) error {
	if !ctx.Connected() {
		return mdlerrors.NewNotConnected()
	}

	// Find the module
	modules, err := ctx.Backend.ListModules()
	if err != nil {
		return mdlerrors.NewBackend("list modules", err)
	}

	var targetModule *model.Module
	for _, m := range modules {
		if m.Name == moduleName {
			targetModule = m
			break
		}
	}

	if targetModule == nil {
		return mdlerrors.NewNotFound("module", moduleName)
	}

	// The module's documentation is its domain model's; print it as the doc
	// comment that sets it, so the output re-run stores it (mendixlabs/mxcli#1314).
	if dm, err := ctx.Backend.GetDomainModel(targetModule.ID); err == nil && dm != nil && dm.Documentation != "" {
		fmt.Fprintf(ctx.Output, "/**\n * %s\n */\n", strings.ReplaceAll(dm.Documentation, "\n", "\n * "))
	}
	// Listed rather than got by module: the same read moduleProtection makes.
	var settings *types.ModuleSettings
	if all, err := ctx.Backend.ListModuleSettings(); err == nil {
		for _, ms := range all {
			if ms.ContainerID == targetModule.ID {
				settings = ms
			}
		}
	}
	fmt.Fprintf(ctx.Output, "create or modify module %s%s;\n", targetModule.Name, moduleSettingsMDL(ctx, settings))
	if targetModule.PackageFile != "" {
		fmt.Fprintf(ctx.Output, "-- packaged module: %s — read-only, the project file holds none of its documents\n", targetModule.PackageFile)
	}
	// Location has no key yet (PROPOSAL_module_settings.md, "Later: Location").
	if targetModule.FromAppStore {
		fmt.Fprintln(ctx.Output, "-- location: Marketplace modules (set in Studio Pro)")
	}
	// A Protected (add-on / solution) module hides every element not marked
	// Usable from the rest of the app (CE1406), which nothing else printed
	// says (mendixlabs/mxcli#1498). An add-on is `ModuleType: addon` above; a
	// solution module has no MDL spelling, so it is a comment only.
	switch kind := moduleProtection(ctx, string(targetModule.ID)); kind {
	case "":
	case "AddOn":
		fmt.Fprintln(ctx.Output, "-- add-on module: elements not marked Usable cannot be referenced from other modules")
	default:
		fmt.Fprintf(ctx.Output, "-- export level: Protected (%s module) — set in Studio Pro; elements not marked Usable cannot be referenced from other modules\n", kind)
	}

	// Module roles live in the module's own Security$ModuleSecurity unit rather
	// than in any document, so a sweep that describes every document in a module
	// still never reports them. Emitting them here is what lets a describe-based
	// comparison see a marketplace update that adds, removes or renames a role.
	describeModuleRoles(ctx, targetModule)

	if !withAll {
		return nil
	}

	// Get all containers belonging to this module (including nested folders)
	moduleContainers := getModuleContainers(ctx, targetModule.ID)

	// Output separator
	fmt.Fprintln(ctx.Output)

	// Every document of the module, in the registry's dependency order, with
	// the domain model's entities and associations after enumerations and
	// constants (mendixlabs/mxcli#1421).
	run := &moduleDescribeRun{ctx: ctx, module: moduleName, containers: moduleContainers, emitted: map[model.ID]bool{}}
	run.emitKinds(moduleDocKindsBeforeDomainModel)

	// Entities in dependency order (base entities before derived entities),
	// then associations once every entity is defined, then the entities'
	// access rules — a grant names the associations its entity owns, and exec
	// refuses one naming an association that does not exist yet
	// (mendixlabs/mxcli#1409).
	dm, err := ctx.Backend.GetDomainModel(targetModule.ID)
	if err != nil {
		run.fail("domain model "+moduleName, err)
	} else if dm != nil {
		entities := sortEntitiesByGeneralization(dm.Entities, moduleName)
		described := make([]*domainmodel.Entity, 0, len(entities))
		for _, entity := range entities {
			qn := ast.QualifiedName{Module: moduleName, Name: entity.Name}
			if err := describeEntityWith(ctx, qn, false); err != nil {
				run.fail("entity "+qn.String(), err)
				continue
			}
			described = append(described, entity)
			fmt.Fprintln(ctx.Output)
		}
		// Cross-module associations are stored in a separate collection but are
		// owned by this module (the FROM entity lives here); emit them too.
		//
		// A view entity's association is not emitted: its `<alias>.ID as <name>`
		// OQL column declares it, the view entity's description above carries
		// that column, and no statement can — a plain one is CE6771, refused by
		// check and exec alike (mendixlabs/mxcli#1627).
		var assocNames []string
		for _, a := range dm.Associations {
			if !isViewAssociationSource(a.Source) {
				assocNames = append(assocNames, a.Name)
			}
		}
		for _, ca := range dm.CrossAssociations {
			if !isViewAssociationSource(ca.Source) {
				assocNames = append(assocNames, ca.Name)
			}
		}
		for _, name := range assocNames {
			qn := ast.QualifiedName{Module: moduleName, Name: name}
			if err := describeAssociation(ctx, qn); err != nil {
				run.fail("association "+qn.String(), err)
				continue
			}
			fmt.Fprintln(ctx.Output)
		}
		for _, entity := range described {
			if len(entity.AccessRules) == 0 {
				continue
			}
			outputEntityAccessGrants(ctx, entity, moduleName, entity.Name)
			fmt.Fprintln(ctx.Output)
		}
	}

	run.emitKinds(moduleDocKindsAfterDomainModel)
	run.reportUnemitted()
	return run.err()
}

// sortEntitiesByGeneralization returns entities sorted so base entities come before derived entities.
// Uses topological sort based on GeneralizationRef (parent entity reference).
func sortEntitiesByGeneralization(entities []*domainmodel.Entity, moduleName string) []*domainmodel.Entity {
	if len(entities) <= 1 {
		return entities
	}

	// Build map of entity name to entity
	entityByName := make(map[string]*domainmodel.Entity)
	for _, ent := range entities {
		entityByName[ent.Name] = ent
		// Also index by qualified name
		entityByName[moduleName+"."+ent.Name] = ent
	}

	// Build adjacency: child -> parent (for entities within this module)
	// We need to output parent before child
	parentOf := make(map[string]string)
	for _, ent := range entities {
		if ent.GeneralizationRef != "" {
			// GeneralizationRef is like "ModuleName.EntityName"
			parentOf[ent.Name] = ent.GeneralizationRef
		}
	}

	// Kahn's algorithm for topological sort
	// Count incoming edges (how many entities extend this one)
	inDegree := make(map[string]int)
	for _, ent := range entities {
		inDegree[ent.Name] = 0
	}
	for child, parent := range parentOf {
		// Only count if parent is in the same module
		if _, inModule := entityByName[parent]; inModule {
			inDegree[child]++
		}
	}

	// Start with entities that have no parent in this module
	var queue []string
	for _, ent := range entities {
		if inDegree[ent.Name] == 0 {
			queue = append(queue, ent.Name)
		}
	}

	// Build children map for traversal
	childrenOf := make(map[string][]string)
	for child, parent := range parentOf {
		if _, inModule := entityByName[parent]; inModule {
			// Extract just the entity name if it's qualified
			parentName := parent
			if strings.Contains(parent, ".") {
				parts := strings.Split(parent, ".")
				parentName = parts[len(parts)-1]
			}
			childrenOf[parentName] = append(childrenOf[parentName], child)
		}
	}

	// Process queue
	var sorted []*domainmodel.Entity
	visited := make(map[string]bool)
	for len(queue) > 0 {
		name := queue[0]
		queue = queue[1:]

		if visited[name] {
			continue
		}
		visited[name] = true

		if ent, ok := entityByName[name]; ok {
			sorted = append(sorted, ent)
		}

		// Add children whose parents are now processed
		for _, child := range childrenOf[name] {
			inDegree[child]--
			if inDegree[child] == 0 {
				queue = append(queue, child)
			}
		}
	}

	// Add any remaining entities (handles cycles or external parents)
	for _, ent := range entities {
		if !visited[ent.Name] {
			sorted = append(sorted, ent)
		}
	}

	return sorted
}

// =============================================================================
// JAR DEPENDENCY COMMANDS
// =============================================================================

// execListJarDependencies implements: LIST JAR DEPENDENCIES [IN module]
func execListJarDependencies(ctx *ExecContext, inModule string) error {
	if !ctx.Connected() {
		return mdlerrors.NewNotConnected()
	}

	modules, err := ctx.Backend.ListModules()
	if err != nil {
		return mdlerrors.NewBackend("list modules", err)
	}

	type row struct {
		module   string
		group    string
		artifact string
		version  string
		included string
	}
	var rows []row

	for _, m := range modules {
		if m.Name == "System" {
			continue
		}
		if inModule != "" && m.Name != inModule {
			continue
		}
		ms, err := ctx.Backend.GetModuleSettings(m.ID)
		if err != nil {
			continue // module may have no settings unit yet
		}
		for _, dep := range ms.JarDependencies {
			inc := "yes"
			if !dep.IsIncluded {
				inc = "no"
			}
			rows = append(rows, row{m.Name, dep.GroupID, dep.ArtifactID, dep.Version, inc})
		}
	}

	if len(rows) == 0 {
		fmt.Fprintln(ctx.Output, "(no jar dependencies)")
		return nil
	}

	// Compute column widths
	colModule, colGroup, colArtifact, colVersion := 6, 5, 8, 7
	for _, r := range rows {
		if len(r.module) > colModule {
			colModule = len(r.module)
		}
		if len(r.group) > colGroup {
			colGroup = len(r.group)
		}
		if len(r.artifact) > colArtifact {
			colArtifact = len(r.artifact)
		}
		if len(r.version) > colVersion {
			colVersion = len(r.version)
		}
	}

	fmtLine := fmt.Sprintf("%%-%ds  %%-%ds  %%-%ds  %%-%ds  %%s\n",
		colModule, colGroup, colArtifact, colVersion)
	sep := func(n int) string { return strings.Repeat("-", n) }
	fmt.Fprintf(ctx.Output, fmtLine, "Module", "Group", "Artifact", "Version", "Included")
	fmt.Fprintf(ctx.Output, fmtLine,
		sep(colModule), sep(colGroup), sep(colArtifact), sep(colVersion), "--------")
	for _, r := range rows {
		fmt.Fprintf(ctx.Output, fmtLine, r.module, r.group, r.artifact, r.version, r.included)
	}
	fmt.Fprintf(ctx.Output, "(%d jar %s)\n", len(rows), pluralize(len(rows), "dependency", "dependencies"))
	return nil
}

// execDescribeJarDependency implements: DESCRIBE JAR DEPENDENCY ModuleName 'group:artifact'
func execDescribeJarDependency(ctx *ExecContext, moduleName, coordinate string) error {
	if !ctx.Connected() {
		return mdlerrors.NewNotConnected()
	}

	module, err := ctx.Backend.GetModuleByName(moduleName)
	if err != nil {
		return fmt.Errorf("module '%s' not found: %w", moduleName, err)
	}

	ms, err := ctx.Backend.GetModuleSettings(module.ID)
	if err != nil {
		return fmt.Errorf("failed to read module settings: %w", err)
	}

	dep := jarDepByCoord(ms.JarDependencies, coordinate)
	if dep == nil {
		return fmt.Errorf("jar dependency '%s' not found in module '%s'", coordinate, moduleName)
	}

	inc := "true"
	if !dep.IsIncluded {
		inc = "false"
	}
	fmt.Fprintf(ctx.Output, "alter module %s\n", moduleName)
	fmt.Fprintf(ctx.Output, "  add jar dependency (\n")
	fmt.Fprintf(ctx.Output, "    group    = '%s',\n", dep.GroupID)
	fmt.Fprintf(ctx.Output, "    artifact = '%s',\n", dep.ArtifactID)
	fmt.Fprintf(ctx.Output, "    version  = '%s',\n", dep.Version)
	fmt.Fprintf(ctx.Output, "    included = %s,\n", inc)
	fmt.Fprintf(ctx.Output, "  );\n")
	if len(dep.Exclusions) > 0 {
		for _, excl := range dep.Exclusions {
			fmt.Fprintf(ctx.Output, "alter module %s set jar dependency '%s:%s'\n",
				moduleName, dep.GroupID, dep.ArtifactID)
			fmt.Fprintf(ctx.Output, "  add exclusion '%s:%s';\n", excl.GroupID, excl.ArtifactID)
		}
	}
	return nil
}

// execAlterModuleJarDep implements ALTER MODULE name <jarDepAction>+
func execAlterModuleJarDep(ctx *ExecContext, s *ast.AlterModuleJarDepStmt) error {
	if !ctx.Connected() {
		return mdlerrors.NewNotConnected()
	}

	module, err := ctx.Backend.GetModuleByName(s.ModuleName)
	if err != nil {
		return fmt.Errorf("module '%s' not found: %w", s.ModuleName, err)
	}

	ms, err := ctx.Backend.GetModuleSettings(module.ID)
	if err != nil {
		return fmt.Errorf("failed to read module settings: %w", err)
	}

	skipped := 0
	for _, action := range s.Actions {
		skip, err := applyJarDepAction(ms, action, s.ModuleName)
		if err != nil {
			return err
		}
		if skip != "" {
			skipped++
			fmt.Fprintln(ctx.Output, skip)
		}
	}

	// Every action was an IF NOT EXISTS skip: the settings are exactly what is
	// stored, so there is nothing to offer storage (ako/mxcli#2239).
	if skipped == len(s.Actions) {
		reportUnchanged(ctx, fmt.Sprintf("jar dependencies for module '%s'", s.ModuleName))
		warnUnvendoredJarDependencies(ctx, ms)
		return nil
	}

	if err := ctx.Backend.UpdateModuleSettings(ms); err != nil {
		return mdlerrors.NewBackend("update module settings", err)
	}

	// ReportMutation says Unchanged when storage elided the write (ADR-0008),
	// e.g. a SET … VERSION restating the stored version.
	ctx.ReportMutation("Updated", "jar dependencies for module '%s'", s.ModuleName)
	warnUnvendoredJarDependencies(ctx, ms)
	return nil
}

// warnUnvendoredJarDependencies says so when a declared dependency has no jar in
// the project's vendorlib/.
//
// Declaring and resolving are separate steps: this writes the coordinate to the
// model, and `list jar dependencies` will report it, but nothing downloads the
// jar — and MxBuild does not resolve it either, so the build stays green and the
// first symptom is a runtime "no driver found" exception from code that looks
// correctly configured. Studio Pro runs the resolution when you edit Module
// Settings; headless it has to be asked for. (mxcli-formula1 findings #12.)
func warnUnvendoredJarDependencies(ctx *ExecContext, ms *types.ModuleSettings) {
	if ctx == nil || ctx.Backend == nil || ms == nil {
		return
	}
	projectPath := ctx.Backend.Path()
	if projectPath == "" {
		return
	}
	vendorlib := filepath.Join(filepath.Dir(projectPath), "vendorlib")

	var missing []string
	for _, d := range ms.JarDependencies {
		if d == nil || d.ArtifactID == "" || d.Version == "" {
			continue
		}
		jar := filepath.Join(vendorlib, fmt.Sprintf("%s-%s.jar", d.ArtifactID, d.Version))
		if _, err := os.Stat(jar); err != nil {
			missing = append(missing, fmt.Sprintf("%s:%s:%s", d.GroupID, d.ArtifactID, d.Version))
		}
	}
	if len(missing) == 0 {
		return
	}
	fmt.Fprintf(ctx.Output,
		"  Note: %s not in vendorlib/, so %s not on the classpath yet — the build will still succeed and fail at runtime.\n",
		strings.Join(missing, ", "), pluralItIsTheyAre(len(missing)))
	fmt.Fprintf(ctx.Output, "  Run: mxcli sync-java-deps -p %s\n", projectPath)
}

// pluralItIsTheyAre picks the pronoun+verb for a list of n coordinates.
func pluralItIsTheyAre(n int) string {
	if n == 1 {
		return "it is"
	}
	return "they are"
}

// applyJarDepAction applies one action to ms. A non-empty skip is the notice
// for an ADD … IF NOT EXISTS whose coordinate is already declared: ms is left
// as it was, and the caller decides whether to print it (exec) or not (check's
// replay, which calls this same function so the two cannot disagree).
func applyJarDepAction(ms *types.ModuleSettings, action ast.JarDepAction, moduleName string) (skip string, err error) {
	err = applyJarDepActionTo(ms, action, moduleName, &skip)
	return skip, err
}

func applyJarDepActionTo(ms *types.ModuleSettings, action ast.JarDepAction, moduleName string, skip *string) error {
	switch a := action.(type) {
	case *ast.AddJarDepAction:
		coord := a.Group + ":" + a.Artifact
		if existing := jarDepByCoord(ms.JarDependencies, coord); existing != nil {
			if a.IfNotExists {
				*skip = jarDepSkipNotice(existing, a, moduleName)
				return nil
			}
			return fmt.Errorf("jar dependency '%s' already exists in module '%s'; use SET to update version, "+
				"or 'add jar dependency if not exists' to make the script re-runnable", coord, moduleName)
		}
		ms.JarDependencies = append(ms.JarDependencies, &types.JarDependency{
			GroupID:    a.Group,
			ArtifactID: a.Artifact,
			Version:    a.Version,
			IsIncluded: a.Included,
		})

	case *ast.SetJarDepVersionAction:
		dep := jarDepByCoord(ms.JarDependencies, a.Coordinate)
		if dep == nil {
			return fmt.Errorf("jar dependency '%s' not found in module '%s'", a.Coordinate, moduleName)
		}
		dep.Version = a.Version

	case *ast.SetJarDepIncludedAction:
		dep := jarDepByCoord(ms.JarDependencies, a.Coordinate)
		if dep == nil {
			return fmt.Errorf("jar dependency '%s' not found in module '%s'", a.Coordinate, moduleName)
		}
		dep.IsIncluded = a.Included

	case *ast.DropJarDepAction:
		idx := jarDepIdxByCoord(ms.JarDependencies, a.Coordinate)
		if idx < 0 {
			return fmt.Errorf("jar dependency '%s' not found in module '%s'", a.Coordinate, moduleName)
		}
		ms.JarDependencies = append(ms.JarDependencies[:idx], ms.JarDependencies[idx+1:]...)

	case *ast.AddJarDepExclusionAction:
		dep := jarDepByCoord(ms.JarDependencies, a.Coordinate)
		if dep == nil {
			return fmt.Errorf("jar dependency '%s' not found in module '%s'", a.Coordinate, moduleName)
		}
		for _, e := range dep.Exclusions {
			if e.GroupID+":"+e.ArtifactID == a.Exclusion {
				return fmt.Errorf("exclusion '%s' already exists on '%s'", a.Exclusion, a.Coordinate)
			}
		}
		parts := strings.SplitN(a.Exclusion, ":", 2)
		if len(parts) != 2 {
			return fmt.Errorf("exclusion coordinate must be 'group:artifact', got: %s", a.Exclusion)
		}
		dep.Exclusions = append(dep.Exclusions, &types.JarDependencyExclusion{
			GroupID:    parts[0],
			ArtifactID: parts[1],
		})

	case *ast.DropJarDepExclusionAction:
		dep := jarDepByCoord(ms.JarDependencies, a.Coordinate)
		if dep == nil {
			return fmt.Errorf("jar dependency '%s' not found in module '%s'", a.Coordinate, moduleName)
		}
		excIdx := -1
		for i, e := range dep.Exclusions {
			if e.GroupID+":"+e.ArtifactID == a.Exclusion {
				excIdx = i
				break
			}
		}
		if excIdx < 0 {
			return fmt.Errorf("exclusion '%s' not found on '%s'", a.Exclusion, a.Coordinate)
		}
		dep.Exclusions = append(dep.Exclusions[:excIdx], dep.Exclusions[excIdx+1:]...)
	}
	return nil
}

// jarDepSkipNotice is the line an ADD … IF NOT EXISTS prints for a coordinate
// the module already declares. Like every IF NOT EXISTS form it leaves the
// stored element untouched; when the statement asks for a different version or
// included flag it says so and names the SET that would change it, because a
// silent skip would let a version bump in the script never reach the model.
func jarDepSkipNotice(existing *types.JarDependency, a *ast.AddJarDepAction, moduleName string) string {
	coord := a.Group + ":" + a.Artifact
	notice := fmt.Sprintf("Jar dependency '%s' already exists in module '%s' — skipped", coord, moduleName)
	var diffs, fixes []string
	if existing.Version != a.Version {
		diffs = append(diffs, fmt.Sprintf("stored version '%s', this statement says '%s'", existing.Version, a.Version))
		fixes = append(fixes, fmt.Sprintf("set jar dependency '%s' version '%s'", coord, a.Version))
	}
	if existing.IsIncluded != a.Included {
		diffs = append(diffs, fmt.Sprintf("stored included = %t, this statement says %t", existing.IsIncluded, a.Included))
		fixes = append(fixes, fmt.Sprintf("set jar dependency '%s' included %t", coord, a.Included))
	}
	if len(diffs) == 0 {
		return notice
	}
	return fmt.Sprintf("%s (%s; if not exists leaves it untouched — to change it: alter module %s %s)",
		notice, strings.Join(diffs, "; "), moduleName, strings.Join(fixes, " "))
}

func jarDepByCoord(deps []*types.JarDependency, coordinate string) *types.JarDependency {
	for _, d := range deps {
		if d.GroupID+":"+d.ArtifactID == coordinate {
			return d
		}
	}
	return nil
}

func jarDepIdxByCoord(deps []*types.JarDependency, coordinate string) int {
	for i, d := range deps {
		if d.GroupID+":"+d.ArtifactID == coordinate {
			return i
		}
	}
	return -1
}

// Executor method wrappers for callers in unmigrated files.

// describeModuleRoles emits the module's roles as re-executable statements.
//
// Failures are deliberately silent: DESCRIBE MODULE is useful without the roles,
// and a backend that cannot read module security (the MCP backend, for one)
// should not turn a working describe into an error.
func describeModuleRoles(ctx *ExecContext, mod *model.Module) {
	ms, err := ctx.Backend.GetModuleSecurity(mod.ID)
	if err != nil || ms == nil || len(ms.ModuleRoles) == 0 {
		return
	}
	// Sort so the output is stable: a differ comparing two describes must not see
	// a change because the reader returned roles in a different order.
	roles := append([]*security.ModuleRole(nil), ms.ModuleRoles...)
	sort.Slice(roles, func(i, j int) bool { return roles[i].Name < roles[j].Name })

	for _, r := range roles {
		// Same shape DESCRIBE MODULE ROLE emits, so the two agree.
		fmt.Fprintf(ctx.Output, "create or modify module role %s.%s", mod.Name, r.Name)
		if r.Description != "" {
			// Double embedded quotes; a description containing one would
			// otherwise terminate the literal and produce unparseable output.
			fmt.Fprintf(ctx.Output, " description '%s'", strings.ReplaceAll(r.Description, "'", "''"))
		}
		fmt.Fprintln(ctx.Output, ";")
	}
}
