// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"log"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
	"github.com/mendixlabs/mxcli/sdk/microflows"
	"github.com/mendixlabs/mxcli/sdk/mpr"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// ============================================================================
// Page Builder
// ============================================================================

// pageBuilder constructs pages from AST.
type pageBuilder struct {
	writer           *mpr.Writer
	reader           *mpr.Reader
	moduleID         model.ID
	moduleName       string
	widgetScope      map[string]model.ID                // widget name -> widget ID
	paramScope       map[string]model.ID                // param name -> entity ID
	paramEntityNames map[string]string                  // param name -> qualified entity name
	execCache        *executorCache                     // Shared cache from executor
	isSnippet        bool                               // True if building a snippet (affects parameter datasource)
	fragments        map[string]*ast.DefineFragmentStmt // Fragment registry from executor
	themeRegistry    *ThemeRegistry                     // Theme design property definitions (may be nil)

	// Pluggable widget engine (lazily initialized)
	widgetRegistry     *WidgetRegistry
	pluggableEngine    *PluggableWidgetEngine
	pluggableEngineErr error // stores init failure reason for better error messages

	// Per-operation caches (may change during execution)
	layoutsCache    []*pages.Layout
	pagesCache      []*pages.Page
	microflowsCache []*microflows.Microflow
	foldersCache    []*mpr.FolderInfo

	// Entity context for resolving short attribute names inside DataViews
	entityContext string // Qualified entity name (e.g., "Module.Entity")
}

// initPluggableEngine lazily initializes the pluggable widget engine.
func (pb *pageBuilder) initPluggableEngine() {
	if pb.pluggableEngine != nil || pb.pluggableEngineErr != nil {
		return
	}
	registry, err := NewWidgetRegistry()
	if err != nil {
		pb.pluggableEngineErr = fmt.Errorf("widget registry init failed: %w", err)
		log.Printf("warning: %v", pb.pluggableEngineErr)
		return
	}
	if pb.reader != nil {
		if loadErr := registry.LoadUserDefinitions(pb.reader.Path()); loadErr != nil {
			log.Printf("warning: loading user widget definitions: %v", loadErr)
		}
	}
	pb.widgetRegistry = registry
	pb.pluggableEngine = NewPluggableWidgetEngine(NewOperationRegistry(), pb)
}

// registerWidgetName registers a widget name and returns an error if it's already used.
// Widget names must be unique within a page/snippet.
func (pb *pageBuilder) registerWidgetName(name string, id model.ID) error {
	if name == "" {
		return nil // Anonymous widgets are allowed
	}
	if existingID, exists := pb.widgetScope[name]; exists {
		return fmt.Errorf("duplicate widget name '%s': widget names must be unique within a page (existing ID: %s)", name, existingID)
	}
	pb.widgetScope[name] = id
	return nil
}

// getModules returns cached modules or loads them.
func (pb *pageBuilder) getModules() []*model.Module {
	if pb.execCache != nil && pb.execCache.modules != nil {
		return pb.execCache.modules
	}
	modules, _ := pb.reader.ListModules()
	if pb.execCache != nil {
		pb.execCache.modules = modules
	}
	return modules
}

// getHierarchy returns cached hierarchy or creates one.
func (pb *pageBuilder) getHierarchy() (*ContainerHierarchy, error) {
	if pb.execCache != nil && pb.execCache.hierarchy != nil {
		return pb.execCache.hierarchy, nil
	}
	h, err := NewContainerHierarchy(pb.reader)
	if err != nil {
		return nil, err
	}
	if pb.execCache != nil {
		pb.execCache.hierarchy = h
	}
	return h, nil
}

// getLayouts returns cached layouts or loads them.
func (pb *pageBuilder) getLayouts() ([]*pages.Layout, error) {
	if pb.layoutsCache == nil {
		var err error
		pb.layoutsCache, err = pb.reader.ListLayouts()
		if err != nil {
			return nil, err
		}
	}
	return pb.layoutsCache, nil
}

// getDomainModels returns cached domain models or loads them.
func (pb *pageBuilder) getDomainModels() ([]*domainmodel.DomainModel, error) {
	if pb.execCache != nil && pb.execCache.domainModels != nil {
		return pb.execCache.domainModels, nil
	}
	domainModels, err := pb.reader.ListDomainModels()
	if err != nil {
		return nil, err
	}
	if pb.execCache != nil {
		pb.execCache.domainModels = domainModels
	}
	return domainModels, nil
}

// getPages returns cached pages or loads them.
func (pb *pageBuilder) getPages() ([]*pages.Page, error) {
	if pb.pagesCache == nil {
		var err error
		pb.pagesCache, err = pb.reader.ListPages()
		if err != nil {
			return nil, err
		}
	}
	return pb.pagesCache, nil
}

// getMicroflows returns cached microflows or loads them.
func (pb *pageBuilder) getMicroflows() ([]*microflows.Microflow, error) {
	if pb.microflowsCache == nil {
		var err error
		pb.microflowsCache, err = pb.reader.ListMicroflows()
		if err != nil {
			return nil, err
		}
	}
	return pb.microflowsCache, nil
}

// resolveLayout finds a layout by qualified name.
func (pb *pageBuilder) resolveLayout(layoutName string) (model.ID, error) {
	layouts, err := pb.getLayouts()
	if err != nil {
		return "", fmt.Errorf("failed to list layouts: %w", err)
	}

	h, err := pb.getHierarchy()
	if err != nil {
		return "", fmt.Errorf("failed to build hierarchy: %w", err)
	}

	// Parse qualified name
	parts := strings.Split(layoutName, ".")
	var moduleName, name string
	if len(parts) >= 2 {
		moduleName = parts[0]
		name = parts[len(parts)-1]
	} else {
		name = layoutName
	}

	// Find matching layout
	for _, l := range layouts {
		modID := h.FindModuleID(l.ContainerID)
		modName := h.GetModuleName(modID)
		if l.Name == name && (moduleName == "" || modName == moduleName) {
			return l.ID, nil
		}
	}

	return "", fmt.Errorf("layout %s not found", layoutName)
}

// resolveEntity finds an entity by qualified name.
func (pb *pageBuilder) resolveEntity(entityRef ast.QualifiedName) (model.ID, error) {
	// Get domain models which contain entities
	domainModels, err := pb.getDomainModels()
	if err != nil {
		return "", fmt.Errorf("failed to list domain models: %w", err)
	}

	h, err := pb.getHierarchy()
	if err != nil {
		return "", fmt.Errorf("failed to build hierarchy: %w", err)
	}

	// Search for entity in domain models
	for _, dm := range domainModels {
		modName := h.GetModuleName(dm.ContainerID)
		for _, e := range dm.Entities {
			if e.Name == entityRef.Name && (entityRef.Module == "" || modName == entityRef.Module) {
				return e.ID, nil
			}
		}
	}

	return "", fmt.Errorf("entity %s not found", entityRef.String())
}

// getModuleID returns the module ID for any container by using the hierarchy.
// Deprecated: prefer using getHierarchy().FindModuleID() directly.
func (e *Executor) getModuleID(containerID model.ID) model.ID {
	h, err := e.getHierarchy()
	if err != nil {
		return containerID
	}
	return h.FindModuleID(containerID)
}

// getModuleName returns the module name for a module ID.
// Deprecated: prefer using getHierarchy().GetModuleName() directly.
func (e *Executor) getModuleName(moduleID model.ID) string {
	h, err := e.getHierarchy()
	if err != nil {
		return ""
	}
	return h.GetModuleName(moduleID)
}

// getMainPlaceholderRef returns the qualified name reference for the main placeholder.
// The format is "Module.Layout.Main" (e.g., "Atlas_Core.Atlas_TopBar.Main").
func (pb *pageBuilder) getMainPlaceholderRef(layoutName string) string {
	// Standard convention: the main placeholder is named "Main"
	// So the reference is "LayoutQualifiedName.Main"
	if layoutName == "" {
		return ""
	}
	return layoutName + ".Main"
}

// getFolders returns cached folders or loads them.
func (pb *pageBuilder) getFolders() ([]*mpr.FolderInfo, error) {
	if pb.foldersCache == nil {
		var err error
		pb.foldersCache, err = pb.reader.ListFolders()
		if err != nil {
			return nil, err
		}
	}
	return pb.foldersCache, nil
}

// resolveFolder resolves a folder path (e.g., "Resources/Images") to a folder ID.
// The path is relative to the current module. If the folder doesn't exist, it creates it.
func (pb *pageBuilder) resolveFolder(folderPath string) (model.ID, error) {
	if folderPath == "" {
		return pb.moduleID, nil
	}

	folders, err := pb.getFolders()
	if err != nil {
		return "", fmt.Errorf("failed to list folders: %w", err)
	}

	// Split path into parts
	parts := strings.Split(folderPath, "/")
	currentContainerID := pb.moduleID

	for _, part := range parts {
		if part == "" {
			continue
		}

		// Find folder with this name under current container
		var foundFolder *mpr.FolderInfo
		for _, f := range folders {
			if f.ContainerID == currentContainerID && f.Name == part {
				foundFolder = f
				break
			}
		}

		if foundFolder != nil {
			currentContainerID = foundFolder.ID
		} else {
			// Create the folder
			newFolderID, err := pb.createFolder(part, currentContainerID)
			if err != nil {
				return "", fmt.Errorf("failed to create folder %s: %w", part, err)
			}
			currentContainerID = newFolderID

			// Add to cache
			pb.foldersCache = append(pb.foldersCache, &mpr.FolderInfo{
				ID:          newFolderID,
				ContainerID: currentContainerID,
				Name:        part,
			})
		}
	}

	return currentContainerID, nil
}

// createFolder creates a new folder in the project.
func (pb *pageBuilder) createFolder(name string, containerID model.ID) (model.ID, error) {
	folder := &model.Folder{
		BaseElement: model.BaseElement{
			ID:       model.ID(mpr.GenerateID()),
			TypeName: "Projects$Folder",
		},
		ContainerID: containerID,
		Name:        name,
	}

	if err := pb.writer.CreateFolder(folder); err != nil {
		return "", err
	}

	return folder.ID, nil
}

// execDropPage handles DROP PAGE statement.
func (e *Executor) execDropPage(s *ast.DropPageStmt) error {
	if e.writer == nil {
		return fmt.Errorf("not connected to a project")
	}

	pages, err := e.reader.ListPages()
	if err != nil {
		return fmt.Errorf("failed to list pages: %w", err)
	}

	for _, p := range pages {
		modID := e.getModuleID(p.ContainerID)
		modName := e.getModuleName(modID)
		if modName == s.Name.Module && p.Name == s.Name.Name {
			if err := e.writer.DeletePage(p.ID); err != nil {
				return fmt.Errorf("failed to delete page: %w", err)
			}
			fmt.Fprintf(e.output, "Dropped page %s\n", s.Name.String())
			return nil
		}
	}

	return fmt.Errorf("page %s not found", s.Name.String())
}

// execDropSnippet handles DROP SNIPPET statement.
func (e *Executor) execDropSnippet(s *ast.DropSnippetStmt) error {
	if e.writer == nil {
		return fmt.Errorf("not connected to a project")
	}

	snippets, err := e.reader.ListSnippets()
	if err != nil {
		return fmt.Errorf("failed to list snippets: %w", err)
	}

	for _, snip := range snippets {
		modID := e.getModuleID(snip.ContainerID)
		modName := e.getModuleName(modID)
		if modName == s.Name.Module && snip.Name == s.Name.Name {
			if err := e.writer.DeleteSnippet(snip.ID); err != nil {
				return fmt.Errorf("failed to delete snippet: %w", err)
			}
			fmt.Fprintf(e.output, "Dropped snippet %s\n", s.Name.String())
			return nil
		}
	}

	return fmt.Errorf("snippet %s not found", s.Name.String())
}
