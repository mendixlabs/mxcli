// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"

	"github.com/JordtenBulte-OLC/mxcli/mdl/executor"
	"github.com/JordtenBulte-OLC/mxcli/mdl/types"
	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/mpr"
	"github.com/spf13/cobra"
)

// TreeNode represents a node in the project tree JSON output.
type TreeNode struct {
	Label         string      `json:"label"`
	Type          string      `json:"type"`
	QualifiedName string      `json:"qualifiedName,omitempty"`
	Children      []*TreeNode `json:"children,omitempty"`
}

// treeElement holds a name, type, and container ID for building the tree hierarchy.
type treeElement struct {
	Name        string
	Type        string
	ContainerID model.ID
	Children    []*TreeNode // optional pre-built children (for expandable documents)
}

var projectTreeCmd = &cobra.Command{
	Use:   "project-tree",
	Short: "Output the project structure as JSON",
	Long: `Output the full Mendix project structure as a JSON tree.

Each module contains categories (Domain Model, Microflows, Pages, etc.)
with their elements organized into folder hierarchies.

This command is designed for use by IDE integrations (e.g., VS Code TreeView).

Example:
  mxcli project-tree -p app.mpr
  mxcli project-tree -p app.mpr | python3 -m json.tool
`,
	Run: func(cmd *cobra.Command, args []string) {
		projectPath, _ := cmd.Flags().GetString("project")
		if projectPath == "" {
			fmt.Fprintln(os.Stderr, "Error: --project (-p) is required")
			os.Exit(1)
		}

		tree, err := buildProjectTree(projectPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(tree); err != nil {
			fmt.Fprintf(os.Stderr, "Error encoding JSON: %v\n", err)
			os.Exit(1)
		}
	},
}

func buildProjectTree(projectPath string) ([]*TreeNode, error) {
	reader, err := mpr.Open(projectPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open project: %w", err)
	}
	defer reader.Close()

	h, err := executor.NewContainerHierarchy(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to build hierarchy: %w", err)
	}

	modules, err := reader.ListModules()
	if err != nil {
		return nil, fmt.Errorf("failed to list modules: %w", err)
	}

	// Build per-module data
	// Domain model items (entities, associations, enumerations) go into Domain Model container
	// Other documents are organized by folder
	type moduleData struct {
		// Domain Model items (no folder hierarchy)
		entities     []treeElement
		associations []treeElement
		enumerations []treeElement
		// Security items
		moduleRoles []treeElement
		// Documents with folder hierarchy
		documents []treeElement
	}

	modData := make(map[model.ID]*moduleData)
	for _, m := range modules {
		modData[m.ID] = &moduleData{}
	}

	// Collect entities and associations from domain models
	dms, _ := reader.ListDomainModels()
	for _, dm := range dms {
		modID := h.FindModuleID(dm.ContainerID)
		md, ok := modData[modID]
		if !ok {
			continue
		}
		for _, ent := range dm.Entities {
			md.entities = append(md.entities, treeElement{Name: ent.Name, ContainerID: dm.ContainerID})
		}
		for _, assoc := range dm.Associations {
			md.associations = append(md.associations, treeElement{Name: assoc.Name, ContainerID: dm.ContainerID})
		}
	}

	// Collect enumerations (part of Domain Model)
	enums, _ := reader.ListEnumerations()
	for _, en := range enums {
		modID := h.FindModuleID(en.ContainerID)
		md, ok := modData[modID]
		if !ok {
			continue
		}
		md.enumerations = append(md.enumerations, treeElement{Name: en.Name, ContainerID: en.ContainerID})
	}

	// Collect module security (module roles)
	allMS, _ := reader.ListModuleSecurity()
	for _, ms := range allMS {
		md, ok := modData[ms.ContainerID]
		if !ok {
			continue
		}
		for _, mr := range ms.ModuleRoles {
			md.moduleRoles = append(md.moduleRoles, treeElement{Name: mr.Name, ContainerID: ms.ContainerID})
		}
	}

	// Collect microflows
	mfs, _ := reader.ListMicroflows()
	for _, mf := range mfs {
		modID := h.FindModuleID(mf.ContainerID)
		md, ok := modData[modID]
		if !ok {
			continue
		}
		md.documents = append(md.documents, treeElement{Name: mf.Name, ContainerID: mf.ContainerID, Type: "microflow"})
	}

	// Collect nanoflows
	nfs, _ := reader.ListNanoflows()
	for _, nf := range nfs {
		modID := h.FindModuleID(nf.ContainerID)
		md, ok := modData[modID]
		if !ok {
			continue
		}
		md.documents = append(md.documents, treeElement{Name: nf.Name, ContainerID: nf.ContainerID, Type: "nanoflow"})
	}

	// Collect pages
	pgs, _ := reader.ListPages()
	for _, pg := range pgs {
		modID := h.FindModuleID(pg.ContainerID)
		md, ok := modData[modID]
		if !ok {
			continue
		}
		md.documents = append(md.documents, treeElement{Name: pg.Name, ContainerID: pg.ContainerID, Type: "page"})
	}

	// Collect snippets
	sns, _ := reader.ListSnippets()
	for _, sn := range sns {
		modID := h.FindModuleID(sn.ContainerID)
		md, ok := modData[modID]
		if !ok {
			continue
		}
		md.documents = append(md.documents, treeElement{Name: sn.Name, ContainerID: sn.ContainerID, Type: "snippet"})
	}

	// Collect layouts
	lys, _ := reader.ListLayouts()
	for _, ly := range lys {
		modID := h.FindModuleID(ly.ContainerID)
		md, ok := modData[modID]
		if !ok {
			continue
		}
		md.documents = append(md.documents, treeElement{Name: ly.Name, ContainerID: ly.ContainerID, Type: "layout"})
	}

	// Collect constants
	consts, _ := reader.ListConstants()
	for _, c := range consts {
		modID := h.FindModuleID(c.ContainerID)
		md, ok := modData[modID]
		if !ok {
			continue
		}
		md.documents = append(md.documents, treeElement{Name: c.Name, ContainerID: c.ContainerID, Type: "constant"})
	}

	// Collect workflows
	wfs, _ := reader.ListWorkflows()
	for _, wf := range wfs {
		modID := h.FindModuleID(wf.ContainerID)
		md, ok := modData[modID]
		if !ok {
			continue
		}
		md.documents = append(md.documents, treeElement{Name: wf.Name, ContainerID: wf.ContainerID, Type: "workflow"})
	}

	// Collect java actions
	jas, _ := reader.ListJavaActions()
	for _, ja := range jas {
		modID := h.FindModuleID(ja.ContainerID)
		md, ok := modData[modID]
		if !ok {
			continue
		}
		md.documents = append(md.documents, treeElement{Name: ja.Name, ContainerID: ja.ContainerID, Type: "javaaction"})
	}

	// Collect scheduled events
	ses, _ := reader.ListScheduledEvents()
	for _, se := range ses {
		modID := h.FindModuleID(se.ContainerID)
		md, ok := modData[modID]
		if !ok {
			continue
		}
		md.documents = append(md.documents, treeElement{Name: se.Name, ContainerID: se.ContainerID, Type: "scheduledevent"})
	}

	// Collect JavaScript actions
	jsas, _ := reader.ListJavaScriptActions()
	for _, jsa := range jsas {
		modID := h.FindModuleID(jsa.ContainerID)
		md, ok := modData[modID]
		if !ok {
			continue
		}
		md.documents = append(md.documents, treeElement{Name: jsa.Name, ContainerID: jsa.ContainerID, Type: "javascriptaction"})
	}

	// Collect building blocks
	bbs, _ := reader.ListBuildingBlocks()
	for _, bb := range bbs {
		modID := h.FindModuleID(bb.ContainerID)
		md, ok := modData[modID]
		if !ok {
			continue
		}
		md.documents = append(md.documents, treeElement{Name: bb.Name, ContainerID: bb.ContainerID, Type: "buildingblock"})
	}

	// Collect page templates
	pts, _ := reader.ListPageTemplates()
	for _, pt := range pts {
		modID := h.FindModuleID(pt.ContainerID)
		md, ok := modData[modID]
		if !ok {
			continue
		}
		md.documents = append(md.documents, treeElement{Name: pt.Name, ContainerID: pt.ContainerID, Type: "pagetemplate"})
	}

	// Collect image collections
	ics, _ := reader.ListImageCollections()
	for _, ic := range ics {
		modID := h.FindModuleID(ic.ContainerID)
		md, ok := modData[modID]
		if !ok {
			continue
		}
		md.documents = append(md.documents, treeElement{Name: ic.Name, ContainerID: ic.ContainerID, Type: "imagecollection"})
	}

	// Collect consumed OData services (clients)
	odataClients, _ := reader.ListConsumedODataServices()
	for _, oc := range odataClients {
		modID := h.FindModuleID(oc.ContainerID)
		md, ok := modData[modID]
		if !ok {
			continue
		}
		md.documents = append(md.documents, treeElement{Name: oc.Name, ContainerID: oc.ContainerID, Type: "odataclient"})
	}

	// Collect published OData services (with entity sets as children)
	odataServices, _ := reader.ListPublishedODataServices()
	for _, svc := range odataServices {
		modID := h.FindModuleID(svc.ContainerID)
		md, ok := modData[modID]
		if !ok {
			continue
		}
		children := buildPublishedODataChildren(svc, modules, modID)
		md.documents = append(md.documents, treeElement{Name: svc.Name, ContainerID: svc.ContainerID, Type: "odataservice", Children: children})
	}

	// Collect published REST services (with resources/operations as children)
	restServices, _ := reader.ListPublishedRestServices()
	for _, svc := range restServices {
		modID := h.FindModuleID(svc.ContainerID)
		md, ok := modData[modID]
		if !ok {
			continue
		}
		children := buildPublishedRestChildren(svc)
		md.documents = append(md.documents, treeElement{Name: svc.Name, ContainerID: svc.ContainerID, Type: "publishedrestservice", Children: children})
	}

	// Collect business event services (with channels/messages as children)
	bess, _ := reader.ListBusinessEventServices()
	for _, bes := range bess {
		modID := h.FindModuleID(bes.ContainerID)
		md, ok := modData[modID]
		if !ok {
			continue
		}
		children := buildBusinessEventChildren(bes)
		md.documents = append(md.documents, treeElement{Name: bes.Name, ContainerID: bes.ContainerID, Type: "businesseventservice", Children: children})
	}

	// Collect JSON structures
	jss, _ := reader.ListJsonStructures()
	for _, js := range jss {
		modID := h.FindModuleID(js.ContainerID)
		md, ok := modData[modID]
		if !ok {
			continue
		}
		md.documents = append(md.documents, treeElement{Name: js.Name, ContainerID: js.ContainerID, Type: "jsonstructure"})
	}

	// Collect import mappings
	ims, _ := reader.ListImportMappings()
	for _, im := range ims {
		modID := h.FindModuleID(im.ContainerID)
		md, ok := modData[modID]
		if !ok {
			continue
		}
		md.documents = append(md.documents, treeElement{Name: im.Name, ContainerID: im.ContainerID, Type: "importmapping"})
	}

	// Collect export mappings
	ems, _ := reader.ListExportMappings()
	for _, em := range ems {
		modID := h.FindModuleID(em.ContainerID)
		md, ok := modData[modID]
		if !ok {
			continue
		}
		md.documents = append(md.documents, treeElement{Name: em.Name, ContainerID: em.ContainerID, Type: "exportmapping"})
	}

	// Collect consumed REST services (with operations as children)
	restClients, _ := reader.ListConsumedRestServices()
	for _, rc := range restClients {
		modID := h.FindModuleID(rc.ContainerID)
		md, ok := modData[modID]
		if !ok {
			continue
		}
		var children []*TreeNode
		for _, op := range rc.Operations {
			method := op.HttpMethod
			if method == "" {
				method = "GET"
			}
			label := method
			if op.Path != "" {
				label += " " + op.Path
			}
			children = append(children, &TreeNode{
				Label: label,
				Type:  "restoperation",
			})
		}
		md.documents = append(md.documents, treeElement{Name: rc.Name, ContainerID: rc.ContainerID, Type: "restclient", Children: children})
	}

	// Collect database connections (with queries as children)
	dbcs, _ := reader.ListDatabaseConnections()
	for _, dbc := range dbcs {
		modID := h.FindModuleID(dbc.ContainerID)
		md, ok := modData[modID]
		if !ok {
			continue
		}
		children := buildDatabaseConnectionChildren(dbc)
		md.documents = append(md.documents, treeElement{Name: dbc.Name, ContainerID: dbc.ContainerID, Type: "databaseconnection", Children: children})
	}

	// Collect data transformers
	dts, _ := reader.ListDataTransformers()
	for _, dt := range dts {
		modID := h.FindModuleID(dt.ContainerID)
		md, ok := modData[modID]
		if !ok {
			continue
		}
		md.documents = append(md.documents, treeElement{Name: dt.Name, ContainerID: dt.ContainerID, Type: "datatransformer"})
	}

	// Collect agent-editor documents (Mendix 11.9+; empty on older projects)
	agentModels, _ := reader.ListAgentEditorModels()
	for _, m := range agentModels {
		modID := h.FindModuleID(m.ContainerID)
		md, ok := modData[modID]
		if !ok {
			continue
		}
		md.documents = append(md.documents, treeElement{Name: m.Name, ContainerID: m.ContainerID, Type: "aimodel"})
	}

	kbs, _ := reader.ListAgentEditorKnowledgeBases()
	for _, kb := range kbs {
		modID := h.FindModuleID(kb.ContainerID)
		md, ok := modData[modID]
		if !ok {
			continue
		}
		md.documents = append(md.documents, treeElement{Name: kb.Name, ContainerID: kb.ContainerID, Type: "knowledgebase"})
	}

	mcpServices, _ := reader.ListAgentEditorConsumedMCPServices()
	for _, svc := range mcpServices {
		modID := h.FindModuleID(svc.ContainerID)
		md, ok := modData[modID]
		if !ok {
			continue
		}
		md.documents = append(md.documents, treeElement{Name: svc.Name, ContainerID: svc.ContainerID, Type: "consumedmcpservice"})
	}

	agents, _ := reader.ListAgentEditorAgents()
	for _, a := range agents {
		modID := h.FindModuleID(a.ContainerID)
		md, ok := modData[modID]
		if !ok {
			continue
		}
		md.documents = append(md.documents, treeElement{Name: a.Name, ContainerID: a.ContainerID, Type: "agent"})
	}

	// Mark external entities in domain model
	// External entities have Source == "Rest$ODataRemoteEntitySource"
	for _, dm := range dms {
		modID := h.FindModuleID(dm.ContainerID)
		md, ok := modData[modID]
		if !ok {
			continue
		}
		for i, ent := range dm.Entities {
			if ent.Source == "Rest$ODataRemoteEntitySource" {
				// Update the entity element to use externalentity type
				for j := range md.entities {
					if md.entities[j].Name == dm.Entities[i].Name {
						md.entities[j].Type = "externalentity"
						break
					}
				}
			}
		}
	}

	// Build tree nodes
	var tree []*TreeNode
	for _, m := range modules {
		md := modData[m.ID]
		modNode := &TreeNode{
			Label:         m.Name,
			Type:          "module",
			QualifiedName: m.Name,
		}

		// Domain Model container (entities, associations, enumerations)
		if len(md.entities) > 0 || len(md.associations) > 0 || len(md.enumerations) > 0 {
			dmNode := &TreeNode{
				Label:         "Domain Model",
				Type:          "domainmodel",
				QualifiedName: m.Name,
			}

			// Add entities (regular or external)
			for _, ent := range md.entities {
				entType := "entity"
				if ent.Type == "externalentity" {
					entType = "externalentity"
				}
				dmNode.Children = append(dmNode.Children, &TreeNode{
					Label:         ent.Name,
					Type:          entType,
					QualifiedName: m.Name + "." + ent.Name,
				})
			}

			// Add associations
			for _, assoc := range md.associations {
				dmNode.Children = append(dmNode.Children, &TreeNode{
					Label:         assoc.Name,
					Type:          "association",
					QualifiedName: m.Name + "." + assoc.Name,
				})
			}

			// Add enumerations
			for _, en := range md.enumerations {
				dmNode.Children = append(dmNode.Children, &TreeNode{
					Label:         en.Name,
					Type:          "enumeration",
					QualifiedName: m.Name + "." + en.Name,
				})
			}

			// Sort domain model children alphabetically
			sort.Slice(dmNode.Children, func(i, j int) bool {
				return dmNode.Children[i].Label < dmNode.Children[j].Label
			})

			modNode.Children = append(modNode.Children, dmNode)
		}

		// Security container (module roles)
		if len(md.moduleRoles) > 0 {
			secNode := &TreeNode{Label: "Security", Type: "security"}
			for _, mr := range md.moduleRoles {
				secNode.Children = append(secNode.Children, &TreeNode{
					Label:         mr.Name,
					Type:          "modulerole",
					QualifiedName: m.Name + "." + mr.Name,
				})
			}
			sort.Slice(secNode.Children, func(i, j int) bool {
				return secNode.Children[i].Label < secNode.Children[j].Label
			})
			modNode.Children = append(modNode.Children, secNode)
		}

		// Build folder hierarchy for documents (microflows, pages, etc.)
		if len(md.documents) > 0 {
			docChildren := buildDocumentHierarchy(h, md.documents, m.Name)
			modNode.Children = append(modNode.Children, docChildren...)
		}

		tree = append(tree, modNode)
	}

	// Project Security top-level node
	ps, err := reader.GetProjectSecurity()
	if err == nil {
		psNode := &TreeNode{Label: "Project Security", Type: "projectsecurity", QualifiedName: "ProjectSecurity"}

		// User Roles category
		if len(ps.UserRoles) > 0 {
			urNode := &TreeNode{Label: "User Roles", Type: "category"}
			for _, ur := range ps.UserRoles {
				urNode.Children = append(urNode.Children, &TreeNode{
					Label:         ur.Name,
					Type:          "userrole",
					QualifiedName: ur.Name,
				})
			}
			sort.Slice(urNode.Children, func(i, j int) bool {
				return urNode.Children[i].Label < urNode.Children[j].Label
			})
			psNode.Children = append(psNode.Children, urNode)
		}

		// Demo Users category
		if ps.EnableDemoUsers && len(ps.DemoUsers) > 0 {
			duNode := &TreeNode{Label: "Demo Users", Type: "category"}
			for _, du := range ps.DemoUsers {
				duNode.Children = append(duNode.Children, &TreeNode{
					Label:         du.UserName,
					Type:          "demouser",
					QualifiedName: du.UserName,
				})
			}
			sort.Slice(duNode.Children, func(i, j int) bool {
				return duNode.Children[i].Label < duNode.Children[j].Label
			})
			psNode.Children = append(psNode.Children, duNode)
		}

		tree = append([]*TreeNode{psNode}, tree...)
	}

	// Project Settings top-level node
	settings, settingsErr := reader.GetProjectSettings()
	if settingsErr == nil {
		settingsNode := &TreeNode{Label: "Settings", Type: "settings", QualifiedName: "Settings"}
		if settings.Model != nil {
			modelNode := &TreeNode{Label: "Model", Type: "settingscategory"}
			if settings.Model.AfterStartupMicroflow != "" {
				modelNode.Children = append(modelNode.Children, &TreeNode{
					Label: "After Startup: " + settings.Model.AfterStartupMicroflow,
					Type:  "settingsitem",
				})
			}
			if settings.Model.BeforeShutdownMicroflow != "" {
				modelNode.Children = append(modelNode.Children, &TreeNode{
					Label: "Before Shutdown: " + settings.Model.BeforeShutdownMicroflow,
					Type:  "settingsitem",
				})
			}
			if len(modelNode.Children) > 0 {
				settingsNode.Children = append(settingsNode.Children, modelNode)
			}
		}
		if settings.Language != nil && settings.Language.DefaultLanguageCode != "" {
			settingsNode.Children = append(settingsNode.Children, &TreeNode{
				Label: "Default Language: " + settings.Language.DefaultLanguageCode,
				Type:  "settingsitem",
			})
		}
		tree = append([]*TreeNode{settingsNode}, tree...)
	}

	// Navigation top-level node
	nav, navErr := reader.GetNavigation()
	if navErr == nil && len(nav.Profiles) > 0 {
		navNode := &TreeNode{Label: "Navigation", Type: "navigation", QualifiedName: "Navigation"}
		for _, profile := range nav.Profiles {
			profileNode := &TreeNode{
				Label:         profile.Kind,
				Type:          "navprofile",
				QualifiedName: profile.Kind,
			}

			// Home page
			if profile.HomePage != nil {
				target := profile.HomePage.Page
				if target == "" {
					target = profile.HomePage.Microflow
				}
				if target != "" {
					profileNode.Children = append(profileNode.Children, &TreeNode{
						Label: "Home: " + target,
						Type:  "navhome",
					})
				}
			}

			// Role-based home pages
			for _, rbh := range profile.RoleBasedHomePages {
				target := rbh.Page
				if target == "" {
					target = rbh.Microflow
				}
				if target != "" {
					profileNode.Children = append(profileNode.Children, &TreeNode{
						Label: "Home (" + rbh.UserRole + "): " + target,
						Type:  "navhome",
					})
				}
			}

			// Login page
			if profile.LoginPage != "" {
				profileNode.Children = append(profileNode.Children, &TreeNode{
					Label: "Login: " + profile.LoginPage,
					Type:  "navlogin",
				})
			}

			// Menu items
			if len(profile.MenuItems) > 0 {
				menuNode := &TreeNode{Label: "Menu", Type: "navmenu"}
				buildMenuTreeNodes(menuNode, profile.MenuItems)
				profileNode.Children = append(profileNode.Children, menuNode)
			}

			navNode.Children = append(navNode.Children, profileNode)
		}
		tree = append([]*TreeNode{navNode}, tree...)
	}

	// Add System Overview node at the top of the tree
	overviewNode := &TreeNode{
		Label:         "System Overview",
		Type:          "systemoverview",
		QualifiedName: "SystemOverview",
	}
	tree = append([]*TreeNode{overviewNode}, tree...)

	// Sort modules alphabetically (skip non-module nodes at front)
	startIdx := 0
	for startIdx < len(tree) && (tree[startIdx].Type == "systemoverview" || tree[startIdx].Type == "projectsecurity" || tree[startIdx].Type == "navigation") {
		startIdx++
	}
	moduleSlice := tree[startIdx:]
	sort.Slice(moduleSlice, func(i, j int) bool {
		return moduleSlice[i].Label < moduleSlice[j].Label
	})

	return tree, nil
}

// buildDocumentHierarchy organizes documents into folder trees based on their container hierarchy.
// Documents without folders appear at the top level (returned directly).
// Folders contain their documents regardless of document type.
func buildDocumentHierarchy(h *executor.ContainerHierarchy, elements []treeElement, moduleName string) []*TreeNode {
	// Group elements by folder path
	type pathElement struct {
		folderPath string
		name       string
		elemType   string
		children   []*TreeNode
	}

	var items []pathElement
	for _, el := range elements {
		fp := h.BuildFolderPath(el.ContainerID)
		items = append(items, pathElement{folderPath: fp, name: el.Name, elemType: el.Type, children: el.Children})
	}

	// Sort by folder path then name
	sort.Slice(items, func(i, j int) bool {
		if items[i].folderPath != items[j].folderPath {
			return items[i].folderPath < items[j].folderPath
		}
		return items[i].name < items[j].name
	})

	// Build folder tree
	root := &TreeNode{Type: "root"}
	folderNodes := make(map[string]*TreeNode)

	for _, item := range items {
		parent := root
		if item.folderPath != "" {
			parent = getOrCreateFolder(root, folderNodes, item.folderPath)
		}
		leaf := &TreeNode{
			Label:         item.name,
			Type:          item.elemType,
			QualifiedName: moduleName + "." + item.name,
			Children:      item.children,
		}
		parent.Children = append(parent.Children, leaf)
	}

	// Sort folders before documents, then alphabetically
	sortChildren(root)
	for _, folder := range folderNodes {
		sortChildren(folder)
	}

	return root.Children
}

// sortChildren sorts a node's children: folders first, then documents, alphabetically within each group.
func sortChildren(node *TreeNode) {
	if len(node.Children) == 0 {
		return
	}
	sort.Slice(node.Children, func(i, j int) bool {
		iIsFolder := node.Children[i].Type == "folder"
		jIsFolder := node.Children[j].Type == "folder"
		if iIsFolder != jIsFolder {
			return iIsFolder // folders come first
		}
		return node.Children[i].Label < node.Children[j].Label
	})
}

// getOrCreateFolder finds or creates a folder node hierarchy for the given path.
func getOrCreateFolder(root *TreeNode, cache map[string]*TreeNode, path string) *TreeNode {
	if node, ok := cache[path]; ok {
		return node
	}

	// Split path into parts
	parts := splitFolderPath(path)
	current := root
	builtPath := ""

	for _, part := range parts {
		if builtPath != "" {
			builtPath += "/"
		}
		builtPath += part

		if node, ok := cache[builtPath]; ok {
			current = node
			continue
		}

		folderNode := &TreeNode{
			Label: part,
			Type:  "folder",
		}
		current.Children = append(current.Children, folderNode)
		cache[builtPath] = folderNode
		current = folderNode
	}

	return current
}

// buildPublishedODataChildren builds child tree nodes for a published OData service.
// Shows entity sets with their exposed entities.
func buildPublishedODataChildren(svc *model.PublishedODataService, modules []*model.Module, modID model.ID) []*TreeNode {
	var children []*TreeNode

	// Find module name for qualified names
	moduleName := ""
	for _, m := range modules {
		if m.ID == modID {
			moduleName = m.Name
			break
		}
	}

	for _, es := range svc.EntitySets {
		label := es.ExposedName
		if es.EntityTypeName != "" {
			// Find the entity type to show the source entity
			for _, et := range svc.EntityTypes {
				if string(et.ID) == es.EntityTypeName {
					if et.Entity != "" {
						label += " → " + et.Entity
					}
					break
				}
			}
		}
		esNode := &TreeNode{
			Label: label,
			Type:  "odataentityset",
		}

		// Show members of the entity type
		for _, et := range svc.EntityTypes {
			if string(et.ID) == es.EntityTypeName {
				for _, mem := range et.Members {
					memLabel := mem.ExposedName
					if mem.Kind != "" {
						memLabel += " (" + mem.Kind + ")"
					}
					esNode.Children = append(esNode.Children, &TreeNode{
						Label: memLabel,
						Type:  "odatamember",
					})
				}
				break
			}
		}

		children = append(children, esNode)
	}

	// If no entity sets but entity types exist, show entity types directly
	if len(svc.EntitySets) == 0 {
		for _, et := range svc.EntityTypes {
			label := et.ExposedName
			if et.Entity != "" {
				label += " → " + et.Entity
			}
			etNode := &TreeNode{
				Label: label,
				Type:  "odataentityset",
			}
			children = append(children, etNode)
		}
	}

	_ = moduleName // reserved for future use
	return children
}

// buildPublishedRestChildren builds child tree nodes for a published REST service.
// Shows resources with their operations (like an OpenAPI contract).
func buildPublishedRestChildren(svc *model.PublishedRestService) []*TreeNode {
	var children []*TreeNode
	for _, res := range svc.Resources {
		resNode := &TreeNode{
			Label: res.Name,
			Type:  "restresource",
		}
		for _, op := range res.Operations {
			method := op.HTTPMethod
			if method == "" {
				method = "GET"
			}
			label := method
			if op.Path != "" {
				label += " " + op.Path
			}
			if op.Summary != "" {
				label += " — " + op.Summary
			}
			opNode := &TreeNode{
				Label: label,
				Type:  "restoperation",
			}
			resNode.Children = append(resNode.Children, opNode)
		}
		children = append(children, resNode)
	}
	return children
}

// buildBusinessEventChildren builds child tree nodes for a business event service.
// Shows channels with their messages.
func buildBusinessEventChildren(svc *model.BusinessEventService) []*TreeNode {
	var children []*TreeNode
	if svc.Definition == nil {
		return children
	}
	for _, ch := range svc.Definition.Channels {
		chNode := &TreeNode{
			Label: ch.ChannelName,
			Type:  "bechannel",
		}
		for _, msg := range ch.Messages {
			direction := ""
			if msg.CanPublish && msg.CanSubscribe {
				direction = " (pub/sub)"
			} else if msg.CanPublish {
				direction = " (publish)"
			} else if msg.CanSubscribe {
				direction = " (subscribe)"
			}
			msgNode := &TreeNode{
				Label: msg.MessageName + direction,
				Type:  "bemessage",
			}
			for _, attr := range msg.Attributes {
				attrLabel := attr.AttributeName
				if attr.AttributeType != "" {
					attrLabel += " : " + attr.AttributeType
				}
				msgNode.Children = append(msgNode.Children, &TreeNode{
					Label: attrLabel,
					Type:  "beattribute",
				})
			}
			chNode.Children = append(chNode.Children, msgNode)
		}
		children = append(children, chNode)
	}
	return children
}

// buildDatabaseConnectionChildren builds child tree nodes for a database connection.
// Shows queries with their parameters and table mappings.
func buildDatabaseConnectionChildren(dbc *model.DatabaseConnection) []*TreeNode {
	var children []*TreeNode
	for _, q := range dbc.Queries {
		qNode := &TreeNode{
			Label: q.Name,
			Type:  "dbquery",
		}
		// Show parameters
		for _, p := range q.Parameters {
			pLabel := p.ParameterName
			if p.DataType != "" {
				pLabel += " : " + p.DataType
			}
			qNode.Children = append(qNode.Children, &TreeNode{
				Label: pLabel,
				Type:  "dbqueryparam",
			})
		}
		// Show table mappings
		for _, tm := range q.TableMappings {
			tmLabel := tm.TableName
			if tm.Entity != "" {
				tmLabel += " → " + tm.Entity
			}
			tmNode := &TreeNode{
				Label: tmLabel,
				Type:  "dbtablemapping",
			}
			for _, col := range tm.Columns {
				colLabel := col.ColumnName
				if col.Attribute != "" {
					colLabel += " → " + col.Attribute
				}
				tmNode.Children = append(tmNode.Children, &TreeNode{
					Label: colLabel,
					Type:  "dbcolumnmapping",
				})
			}
			qNode.Children = append(qNode.Children, tmNode)
		}
		children = append(children, qNode)
	}
	return children
}

// buildMenuTreeNodes recursively builds tree nodes from navigation menu items.
func buildMenuTreeNodes(parent *TreeNode, items []*types.NavMenuItem) {
	for _, item := range items {
		label := item.Caption
		if label == "" {
			label = "(unnamed)"
		}
		if item.Page != "" {
			label += " → " + item.Page
		} else if item.Microflow != "" {
			label += " → " + item.Microflow
		}

		node := &TreeNode{
			Label: label,
			Type:  "navmenuitem",
		}

		if len(item.Items) > 0 {
			buildMenuTreeNodes(node, item.Items)
		}

		parent.Children = append(parent.Children, node)
	}
}

// splitFolderPath splits a folder path like "Parent/Child" into parts.
func splitFolderPath(path string) []string {
	if path == "" {
		return nil
	}
	var parts []string
	start := 0
	for i := 0; i < len(path); i++ {
		if path[i] == '/' {
			if i > start {
				parts = append(parts, path[start:i])
			}
			start = i + 1
		}
	}
	if start < len(path) {
		parts = append(parts, path[start:])
	}
	return parts
}
