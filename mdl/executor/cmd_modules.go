// SPDX-License-Identifier: Apache-2.0

// Package executor - Module commands (SHOW/CREATE/DROP MODULES)
package executor

import (
	"fmt"
	"sort"
	"strings"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
	mdlerrors "github.com/JordtenBulte-OLC/mxcli/mdl/errors"
	"github.com/JordtenBulte-OLC/mxcli/mdl/types"
	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/domainmodel"
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

	// Count from units by type prefix (efficient single pass)
	for _, u := range units {
		modID := h.FindModuleID(u.ContainerID)
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

	// Count entities from domain models
	if dms, err := ctx.Backend.ListDomainModels(); err == nil {
		for _, dm := range dms {
			modID := h.FindModuleID(dm.ContainerID)
			entityCounts[modID] += len(dm.Entities)
		}
	}

	// Count enumerations
	if enums, err := ctx.Backend.ListEnumerations(); err == nil {
		for _, enum := range enums {
			modID := h.FindModuleID(enum.ContainerID)
			enumCounts[modID]++
		}
	}

	// Count pages
	if pages, err := ctx.Backend.ListPages(); err == nil {
		for _, p := range pages {
			modID := h.FindModuleID(p.ContainerID)
			pageCounts[modID]++
		}
	}

	// Count snippets
	if snippets, err := ctx.Backend.ListSnippets(); err == nil {
		for _, s := range snippets {
			modID := h.FindModuleID(s.ContainerID)
			snippetCounts[modID]++
		}
	}

	// Count microflows
	if mfs, err := ctx.Backend.ListMicroflows(); err == nil {
		for _, mf := range mfs {
			modID := h.FindModuleID(mf.ContainerID)
			microflowCounts[modID]++
		}
	}

	// Count nanoflows
	if nfs, err := ctx.Backend.ListNanoflows(); err == nil {
		for _, nf := range nfs {
			modID := h.FindModuleID(nf.ContainerID)
			nanoflowCounts[modID]++
		}
	}

	// Count constants
	if constants, err := ctx.Backend.ListConstants(); err == nil {
		for _, c := range constants {
			modID := h.FindModuleID(c.ContainerID)
			constantCounts[modID]++
		}
	}

	// Count Java actions
	if jas, err := ctx.Backend.ListJavaActions(); err == nil {
		for _, ja := range jas {
			modID := h.FindModuleID(ja.ContainerID)
			javaActionCounts[modID]++
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

		r := row{
			name:        m.Name,
			source:      source,
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
		Columns: []string{"Module", "Source", "Entities", "Enums", "Pages", "Snippets", "Microflows", "Nanoflows", "Workflows", "Constants", "JavaActions", "PubREST", "PubOData", "ConOData", "BizEvents", "ExtDB"},
		Summary: fmt.Sprintf("(%d modules)", len(modules)),
	}
	for _, r := range rows {
		result.Rows = append(result.Rows, []any{r.name, r.source, r.entities, r.enums, r.pages, r.snippets, r.microflows, r.nanoflows, r.workflows, r.constants, r.javaActions, r.pubRest, r.pubOData, r.conOData, r.bizEvents, r.extDb})
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

	// Output basic CREATE MODULE statement
	fmt.Fprintf(ctx.Output, "create module %s;\n", targetModule.Name)

	if !withAll {
		fmt.Fprintln(ctx.Output, "/")
		return nil
	}

	// Get all containers belonging to this module (including nested folders)
	moduleContainers := getModuleContainers(ctx, targetModule.ID)

	// Output separator
	fmt.Fprintln(ctx.Output)

	// Output enumerations in this module (no dependencies between enums)
	if enums, err := ctx.Backend.ListEnumerations(); err == nil {
		for _, enum := range enums {
			if moduleContainers[enum.ContainerID] {
				if err := describeEnumeration(ctx, ast.QualifiedName{Module: moduleName, Name: enum.Name}); err == nil {
					fmt.Fprintln(ctx.Output)
				}
			}
		}
	}

	// Output constants (may reference enumerations)
	if constants, err := ctx.Backend.ListConstants(); err == nil {
		for _, c := range constants {
			if moduleContainers[c.ContainerID] {
				if err := outputConstantMDL(ctx, c, moduleName); err == nil {
					fmt.Fprintln(ctx.Output)
				}
			}
		}
	}

	// Output entities in dependency order (base entities before derived entities)
	// and associations after all entities
	if dm, err := ctx.Backend.GetDomainModel(targetModule.ID); err == nil {
		// Topologically sort entities by generalization (inheritance)
		sortedEntities := sortEntitiesByGeneralization(dm.Entities, moduleName)

		// Output entities in sorted order
		for _, entity := range sortedEntities {
			if err := describeEntity(ctx, ast.QualifiedName{Module: moduleName, Name: entity.Name}); err == nil {
				fmt.Fprintln(ctx.Output)
			}
		}

		// Output associations (after all entities are defined)
		for _, assoc := range dm.Associations {
			if err := describeAssociation(ctx, ast.QualifiedName{Module: moduleName, Name: assoc.Name}); err == nil {
				fmt.Fprintln(ctx.Output)
			}
		}
		// Cross-module associations are stored in a separate collection but are
		// owned by this module (the FROM entity lives here); emit them too.
		for _, ca := range dm.CrossAssociations {
			if err := describeAssociation(ctx, ast.QualifiedName{Module: moduleName, Name: ca.Name}); err == nil {
				fmt.Fprintln(ctx.Output)
			}
		}
	}

	// Output microflows
	if mfs, err := ctx.Backend.ListMicroflows(); err == nil {
		for _, mf := range mfs {
			if moduleContainers[mf.ContainerID] {
				if err := describeMicroflow(ctx, ast.QualifiedName{Module: moduleName, Name: mf.Name}); err == nil {
					fmt.Fprintln(ctx.Output)
				}
			}
		}
	}

	// Output java actions
	if jaList, err := ctx.Backend.ListJavaActions(); err == nil {
		for _, ja := range jaList {
			if moduleContainers[ja.ContainerID] {
				if err := describeJavaAction(ctx, ast.QualifiedName{Module: moduleName, Name: ja.Name}); err == nil {
					fmt.Fprintln(ctx.Output)
				}
			}
		}
	}

	// Output pages
	if pageList, err := ctx.Backend.ListPages(); err == nil {
		for _, p := range pageList {
			if moduleContainers[p.ContainerID] {
				if err := describePage(ctx, ast.QualifiedName{Module: moduleName, Name: p.Name}); err == nil {
					fmt.Fprintln(ctx.Output)
				}
			}
		}
	}

	// Output snippets
	if snippets, err := ctx.Backend.ListSnippets(); err == nil {
		for _, s := range snippets {
			if moduleContainers[s.ContainerID] {
				if err := describeSnippet(ctx, ast.QualifiedName{Module: moduleName, Name: s.Name}); err == nil {
					fmt.Fprintln(ctx.Output)
				}
			}
		}
	}

	// Output layouts
	if layouts, err := ctx.Backend.ListLayouts(); err == nil {
		for _, l := range layouts {
			if moduleContainers[l.ContainerID] {
				if err := describeLayout(ctx, ast.QualifiedName{Module: moduleName, Name: l.Name}); err == nil {
					fmt.Fprintln(ctx.Output)
				}
			}
		}
	}

	// Output database connections
	if conns, err := ctx.Backend.ListDatabaseConnections(); err == nil {
		for _, conn := range conns {
			if moduleContainers[conn.ContainerID] {
				if err := outputDatabaseConnectionMDL(ctx, conn, moduleName); err == nil {
					fmt.Fprintln(ctx.Output)
				}
			}
		}
	}

	// Output business event services
	if services, err := ctx.Backend.ListBusinessEventServices(); err == nil {
		for _, svc := range services {
			if moduleContainers[svc.ContainerID] {
				if err := describeBusinessEventService(ctx, ast.QualifiedName{Module: moduleName, Name: svc.Name}); err == nil {
					fmt.Fprintln(ctx.Output)
				}
			}
		}
	}

	// Get hierarchy for folder path resolution (used by OData sections below)
	h, _ := getHierarchy(ctx)

	// Output consumed OData services (clients)
	if services, err := ctx.Backend.ListConsumedODataServices(); err == nil {
		for _, svc := range services {
			if moduleContainers[svc.ContainerID] {
				folderPath := ""
				if h != nil {
					folderPath = h.BuildFolderPath(svc.ContainerID)
				}
				if err := outputConsumedODataServiceMDL(ctx, svc, moduleName, folderPath); err == nil {
					fmt.Fprintln(ctx.Output)
				}
			}
		}
	}

	// Output published OData services
	if services, err := ctx.Backend.ListPublishedODataServices(); err == nil {
		for _, svc := range services {
			if moduleContainers[svc.ContainerID] {
				folderPath := ""
				if h != nil {
					folderPath = h.BuildFolderPath(svc.ContainerID)
				}
				if err := outputPublishedODataServiceMDL(ctx, svc, moduleName, folderPath); err == nil {
					fmt.Fprintln(ctx.Output)
				}
			}
		}
	}

	// Output agent-editor documents. Order matches referential dependencies:
	// Model / KnowledgeBase / ConsumedMCPService first (leaves), Agent last
	// (may reference the others via its TOOL / KNOWLEDGE BASE / MCP SERVICE
	// blocks).
	if models, err := ctx.Backend.ListAgentEditorModels(); err == nil {
		for _, m := range models {
			if moduleContainers[m.ContainerID] {
				if err := describeAgentEditorModel(ctx, ast.QualifiedName{Module: moduleName, Name: m.Name}); err == nil {
					fmt.Fprintln(ctx.Output)
				}
			}
		}
	}
	if kbs, err := ctx.Backend.ListAgentEditorKnowledgeBases(); err == nil {
		for _, kb := range kbs {
			if moduleContainers[kb.ContainerID] {
				if err := describeAgentEditorKnowledgeBase(ctx, ast.QualifiedName{Module: moduleName, Name: kb.Name}); err == nil {
					fmt.Fprintln(ctx.Output)
				}
			}
		}
	}
	if svcs, err := ctx.Backend.ListAgentEditorConsumedMCPServices(); err == nil {
		for _, svc := range svcs {
			if moduleContainers[svc.ContainerID] {
				if err := describeAgentEditorConsumedMCPService(ctx, ast.QualifiedName{Module: moduleName, Name: svc.Name}); err == nil {
					fmt.Fprintln(ctx.Output)
				}
			}
		}
	}
	if agents, err := ctx.Backend.ListAgentEditorAgents(); err == nil {
		for _, a := range agents {
			if moduleContainers[a.ContainerID] {
				if err := describeAgentEditorAgent(ctx, ast.QualifiedName{Module: moduleName, Name: a.Name}); err == nil {
					fmt.Fprintln(ctx.Output)
				}
			}
		}
	}

	fmt.Fprintln(ctx.Output, "/")
	return nil
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

	for _, action := range s.Actions {
		if err := applyJarDepAction(ms, action, s.ModuleName); err != nil {
			return err
		}
	}

	if err := ctx.Backend.UpdateModuleSettings(ms); err != nil {
		return mdlerrors.NewBackend("update module settings", err)
	}

	fmt.Fprintf(ctx.Output, "Updated jar dependencies for module '%s'\n", s.ModuleName)
	return nil
}

func applyJarDepAction(ms *types.ModuleSettings, action ast.JarDepAction, moduleName string) error {
	switch a := action.(type) {
	case *ast.AddJarDepAction:
		coord := a.Group + ":" + a.Artifact
		if jarDepByCoord(ms.JarDependencies, coord) != nil {
			return fmt.Errorf("jar dependency '%s' already exists in module '%s'; use SET to update version", coord, moduleName)
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
