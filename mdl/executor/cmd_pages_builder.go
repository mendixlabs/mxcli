// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/mdl/folderpath"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
	"github.com/mendixlabs/mxcli/sdk/microflows"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// ============================================================================
// Page Builder
// ============================================================================

// danglingRefOK reports whether a failed reference resolution may be kept by
// name: only for an excluded document, and only when the name resolved to
// nothing (a backend failure is never swallowed). See tolerateDanglingRefs.
func (pb *pageBuilder) danglingRefOK(err error) bool {
	var nf *mdlerrors.NotFoundError
	return pb.tolerateDanglingRefs && errors.As(err, &nf)
}

// pageBuilder constructs pages from AST.
type pageBuilder struct {
	ctx              *ExecContext // execution context (for building-block expansion, etc.)
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

	// dataViewVariables: enclosing data view name -> the Forms$PageVariable a
	// binding read through it (`$dataView1.Attr`) stores. Holds only the data
	// views around the widget being built (ako/mxcli#826).
	dataViewVariables map[string]pages.WidgetVariable

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
	// outerEntityContexts are the entities of the data containers enclosing the
	// current one, outermost first: a snippet parameter a call omits is taken
	// from any of them, not only the nearest (ako/mxcli#2055).
	outerEntityContexts []string

	// What this point of the walk knows about the object a SHOW_PAGE widget
	// action would bind its argument to: whether it is knowable at all, whether
	// there is one, and what it is called. Used to tell an argument that names
	// the context object from one that names something else — and from one
	// written where no context object exists at all, which mxcli cannot store
	// either. See cmd_pages_showpage_args.go.
	argCtx pageArgContext

	// Name of the widget currently being built, so a refusal names the control
	// the author has to go and fix. The check-time mirror gets it from the AST
	// node it is looking at; the builder's action path is several calls deep and
	// would otherwise say only "this widget".
	currentWidget string

	// tolerateDanglingRefs is set when the document being built is EXCLUDED
	// (by @excluded, or carried from the stored document). Mendix does not
	// validate an excluded document, and one may name flows, pages or snippets
	// the project does not contain — Feedback v4.0.2 ships such an example
	// page. The writer stores an ACTION's or snippet call's target BY NAME, so
	// an unresolved one is kept as written instead of failing the build.
	//
	// A data-source FLOW is kept by name too, but it is what puts an entity in
	// scope: without it a bare attribute binding inside the container cannot
	// be qualified, and one written bare made `mx check` fail to LOAD the
	// project (ArgumentNullException setting 'Attribute', Mendix 11.13.0).
	// DESCRIBE writes those bindings qualified there, the check refuses a bare
	// one (unscopedBindings), and the writer refuses any bare attribute
	// reference as a last line, ALTER included (canon.BareAttributeRefError).
	tolerateDanglingRefs bool

	// Local page/snippet variables (Variables: ( $name: Type = <default expression> )).
	// Used to distinguish a $localVar reference from a page parameter when
	// resolving TextTemplate parameters — local variables must be stored as
	// Forms$PageVariable.LocalVariable in BSON, not as a literal Expression.
	localVariables map[string]bool

	// primitiveParams are the page's primitive-typed parameters (String,
	// Integer, …). paramScope holds only the entity-typed ones, so without this
	// a bare `{1} = $IpAddress` template parameter was stored as the attribute
	// path "$IpAddress", bound nothing, and mxbuild reported CE0402 "No value
	// specified." (ako/mxcli#2132). Snippets cannot declare one (CE0046).
	primitiveParams map[string]bool

	// isDocumentWidget reports whether a name is a widget of the document being
	// built. A flow argument `$name` naming one passes that widget's selection,
	// which Studio Pro binds through the PageVariable's Widget slot
	// (ako/mxcli#1560). nil where the document's widgets are not known: nothing
	// is then a widget, and the argument binds as it always did.
	isDocumentWidget func(name string) bool

	// storedPluggables are the pluggable widgets of the stored document a
	// `create or replace`/`create or modify` rewrites; nil for a new document.
	// A widget the statement did not change keeps its stored Type and Object
	// (ako/mxcli#721 L4, see cmd_pages_pluggable_passthrough.go); one it did
	// change keeps the properties it did not (ako/mxcli#832,
	// cmd_pages_pluggable_patch.go).
	storedPluggables *pluggablePassthrough

	// shadowBuild is set while that patch builds the widget describe prints
	// for the stored one: its child widgets are stubbed (shadowChild).
	shadowBuild bool
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
	if pb.execCache == nil {
		modules, _ := pb.backend.ListModules()
		return modules
	}
	modules, _ := pb.execCache.cachedModules(pb.backend.ListModules)
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

// layoutIsNative reports whether the layout with this ID is a native one.
func (pb *pageBuilder) layoutIsNative(id model.ID) bool {
	layouts, err := pb.getLayouts()
	if err != nil {
		return false
	}
	for _, l := range layouts {
		if l.ID == id {
			return l.Native
		}
	}
	return false
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

	currentContainerID := pb.moduleID

	for _, part := range folderpath.Split(folderPath) {

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

// createFolder creates a new folder in the project through the executor's one
// folder creator, which also keeps the cached hierarchy in step (ako/mxcli#1486).
func (pb *pageBuilder) createFolder(name string, containerID model.ID) (model.ID, error) {
	return createFolder(pb.ctx, name, containerID)
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

	for _, p := range liveFirst(pages, pageExcluded) {
		modID := getModuleID(ctx, p.ContainerID)
		modName := getModuleName(ctx, modID)
		if modName == s.Name.Module && p.Name == s.Name.Name {
			if err := ctx.Backend.DeletePage(p.ID); err != nil {
				return mdlerrors.NewBackend("delete page", err)
			}
			fmt.Fprintf(ctx.Output, "Dropped page %s\n", s.Name.String())
			writeDroppedGrantsNote(ctx.Output, "page", "view", s.Name.String(), p.AllowedRoles, false)

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

// textLang is the language a bare caption/title/content string from this page's
// MDL is stored under (mendixlabs/mxcli#970). See authoringLanguage.
func (pb *pageBuilder) textLang() string { return authoringLanguage(pb.ctx) }
