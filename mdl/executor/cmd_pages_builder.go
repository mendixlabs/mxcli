// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
	"github.com/JordtenBulte-OLC/mxcli/mdl/backend"
	mdlerrors "github.com/JordtenBulte-OLC/mxcli/mdl/errors"
	"github.com/JordtenBulte-OLC/mxcli/mdl/types"
	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/domainmodel"
	"github.com/JordtenBulte-OLC/mxcli/sdk/microflows"
	"github.com/JordtenBulte-OLC/mxcli/sdk/pages"
)

// ============================================================================
// Page Builder
// ============================================================================

// pageBuilder constructs pages from AST.
type pageBuilder struct {
	backend          backend.FullBackend
	moduleID         model.ID
	moduleName       string
	widgetScope      map[string]model.ID                // widget name -> widget ID
	paramScope       map[string]model.ID                // param name -> entity ID
	paramEntityNames map[string]string                  // param name -> qualified entity name
	execCache        *executorCache                     // Shared cache from executor
	isSnippet        bool                               // True if building a snippet (affects parameter datasource)
	fragments        map[string]*ast.DefineFragmentStmt // Fragment registry from executor
	themeRegistry    *ThemeRegistry                     // Theme design property definitions (may be nil)
	widgetBackend    backend.WidgetBuilderBackend       // Backend for pluggable widget construction

	// Pluggable widget engine (lazily initialized)
	widgetRegistry     *WidgetRegistry
	pluggableEngine    *PluggableWidgetEngine
	pluggableEngineErr error // stores init failure reason for better error messages

	// Per-operation caches (may change during execution)
	layoutsCache    []*pages.Layout
	pagesCache      []*pages.Page
	microflowsCache []*microflows.Microflow
	foldersCache    []*types.FolderInfo

	// Entity context for resolving short attribute names inside DataViews
	entityContext string // Qualified entity name (e.g., "Module.Entity")

	// Local page/snippet variables (Variables: { $name: Type = 'default' }).
	// Used to distinguish a $localVar reference from a page parameter when
	// resolving TextTemplate parameters — local variables must be stored as
	// Forms$PageVariable.LocalVariable in BSON, not as a literal Expression.
	localVariables map[string]bool
}

// initPluggableEngine lazily initializes the pluggable widget engine.
func (pb *pageBuilder) initPluggableEngine() {
	if pb.pluggableEngine != nil || pb.pluggableEngineErr != nil {
		return
	}
	registry, err := NewWidgetRegistry()
	if err != nil {
		pb.pluggableEngineErr = mdlerrors.NewBackend("widget registry init", err)
		log.Printf("warning: %v", pb.pluggableEngineErr)
		return
	}
	if pb.backend != nil {
		// Ensure project-local defs exist and are current before the engine
		// reads them: generate them from installed .mpk when missing (project
		// never `widget init`-ed), and refresh any whose generatorVersion is
		// behind the current build (an mxcli upgrade would otherwise emit stale
		// BSON from defs the unchanged .mpk generated under an older version).
		if refreshed, refreshErr := RefreshStaleWidgetDefinitions(pb.backend.Path()); refreshErr != nil {
			log.Printf("warning: updating widget definitions: %v", refreshErr)
		} else if refreshed {
			log.Printf("info: updated widget definitions for %s", pb.backend.Path())
		}
		if loadErr := registry.LoadUserDefinitions(pb.backend.Path()); loadErr != nil {
			log.Printf("warning: loading user widget definitions: %v", loadErr)
		}
	}
	pb.widgetRegistry = registry
	pb.pluggableEngine = NewPluggableWidgetEngine(pb.widgetBackend, pb)
}

// registerWidgetName registers a widget name and returns an error if it's already used.
// Widget names must be unique within a page/snippet.

// getProjectPath returns the project directory path from the backend.
func (pb *pageBuilder) getProjectPath() string {
	if pb.backend != nil {
		return pb.backend.Path()
	}
	return ""
}
func (pb *pageBuilder) registerWidgetName(name string, id model.ID) error {
	if name == "" {
		return nil // Anonymous widgets are allowed
	}
	if existingID, exists := pb.widgetScope[name]; exists {
		return mdlerrors.NewAlreadyExistsMsg("widget", name, fmt.Sprintf("duplicate widget name '%s': widget names must be unique within a page (existing ID: %s)", name, existingID))
	}
	pb.widgetScope[name] = id
	return nil
}

// getModules returns cached modules or loads them.
func (pb *pageBuilder) getModules() []*model.Module {
	if pb.execCache != nil && pb.execCache.modules != nil {
		return pb.execCache.modules
	}
	modules, _ := pb.backend.ListModules()
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
	h, err := NewContainerHierarchyFromBackend(pb.backend)
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
		pb.layoutsCache, err = pb.backend.ListLayouts()
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
	domainModels, err := pb.backend.ListDomainModels()
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
		pb.pagesCache, err = pb.backend.ListPages()
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
		pb.microflowsCache, err = pb.backend.ListMicroflows()
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
		return "", mdlerrors.NewBackend("list layouts", err)
	}

	h, err := pb.getHierarchy()
	if err != nil {
		return "", mdlerrors.NewBackend("build hierarchy", err)
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

	return "", mdlerrors.NewNotFound("layout", layoutName)
}

// resolveEntity finds an entity by qualified name.
func (pb *pageBuilder) resolveEntity(entityRef ast.QualifiedName) (model.ID, error) {
	// Get domain models which contain entities
	domainModels, err := pb.getDomainModels()
	if err != nil {
		return "", mdlerrors.NewBackend("list domain models", err)
	}

	h, err := pb.getHierarchy()
	if err != nil {
		return "", mdlerrors.NewBackend("build hierarchy", err)
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

	return "", mdlerrors.NewNotFound("entity", entityRef.String())
}

// getModuleID returns the module ID for any container by using the hierarchy.
// Deprecated: prefer using getHierarchy().FindModuleID() directly.
func getModuleID(ctx *ExecContext, containerID model.ID) model.ID {
	h, err := getHierarchy(ctx)
	if err != nil {
		return containerID
	}
	return h.FindModuleID(containerID)
}

// getModuleName returns the module name for a module ID.
// Deprecated: prefer using getHierarchy().GetModuleName() directly.
func getModuleName(ctx *ExecContext, moduleID model.ID) string {
	h, err := getHierarchy(ctx)
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
func (pb *pageBuilder) getFolders() ([]*types.FolderInfo, error) {
	if pb.foldersCache == nil {
		var err error
		pb.foldersCache, err = pb.backend.ListFolders()
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
		return "", mdlerrors.NewBackend("list folders", err)
	}

	// Split path into parts
	parts := strings.Split(folderPath, "/")
	currentContainerID := pb.moduleID

	for _, part := range parts {
		if part == "" {
			continue
		}

		// Find folder with this name under current container
		var foundFolder *types.FolderInfo
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
				return "", mdlerrors.NewBackend(fmt.Sprintf("create folder %s", part), err)
			}
			parentContainerID := currentContainerID
			currentContainerID = newFolderID

			// Add to cache
			pb.foldersCache = append(pb.foldersCache, &types.FolderInfo{
				ID:          newFolderID,
				ContainerID: parentContainerID,
				Name:        part,
			})
			currentContainerID = newFolderID
		}
	}

	return currentContainerID, nil
}

// createFolder creates a new folder in the project.
func (pb *pageBuilder) createFolder(name string, containerID model.ID) (model.ID, error) {
	folder := &model.Folder{
		BaseElement: model.BaseElement{
			ID:       model.ID(types.GenerateID()),
			TypeName: "Projects$Folder",
		},
		ContainerID: containerID,
		Name:        name,
	}

	if err := pb.backend.CreateFolder(folder); err != nil {
		return "", err
	}

	return folder.ID, nil
}

// execDropPage handles DROP PAGE statement.
func execDropPage(ctx *ExecContext, s *ast.DropPageStmt) error {
	if !ctx.ConnectedForWrite() {
		return mdlerrors.NewNotConnectedWrite()
	}

	pages, err := ctx.Backend.ListPages()
	if err != nil {
		return mdlerrors.NewBackend("list pages", err)
	}

	for _, p := range pages {
		modID := getModuleID(ctx, p.ContainerID)
		modName := getModuleName(ctx, modID)
		if modName == s.Name.Module && p.Name == s.Name.Name {
			if err := ctx.Backend.DeletePage(p.ID); err != nil {
				return mdlerrors.NewBackend("delete page", err)
			}
			fmt.Fprintf(ctx.Output, "Dropped page %s\n", s.Name.String())
			return nil
		}
	}

	return mdlerrors.NewNotFound("page", s.Name.String())
}

// execDropSnippet handles DROP SNIPPET statement.
func execDropSnippet(ctx *ExecContext, s *ast.DropSnippetStmt) error {
	if !ctx.ConnectedForWrite() {
		return mdlerrors.NewNotConnectedWrite()
	}

	snippets, err := ctx.Backend.ListSnippets()
	if err != nil {
		return mdlerrors.NewBackend("list snippets", err)
	}

	for _, snip := range snippets {
		modID := getModuleID(ctx, snip.ContainerID)
		modName := getModuleName(ctx, modID)
		if modName == s.Name.Module && snip.Name == s.Name.Name {
			if err := ctx.Backend.DeleteSnippet(snip.ID); err != nil {
				return mdlerrors.NewBackend("delete snippet", err)
			}
			fmt.Fprintf(ctx.Output, "Dropped snippet %s\n", s.Name.String())
			return nil
		}
	}

	return mdlerrors.NewNotFound("snippet", s.Name.String())
}

func (e *Executor) getModuleName(moduleID model.ID) string {
	return getModuleName(e.newExecContext(context.Background()), moduleID)
}
