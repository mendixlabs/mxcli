// SPDX-License-Identifier: Apache-2.0

// Package executor executes MDL AST statements against a Mendix project.
package executor

import (
	"fmt"
	"io"
	"path/filepath"
	"time"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/catalog"
	"github.com/mendixlabs/mxcli/mdl/diaglog"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
	"github.com/mendixlabs/mxcli/sdk/mpr"
	sqllib "github.com/mendixlabs/mxcli/sql"
)

// executorCache holds cached data for performance across multiple operations.
type executorCache struct {
	modules      []*model.Module
	units        []*mpr.UnitInfo
	folders      []*mpr.FolderInfo
	domainModels []*domainmodel.DomainModel
	hierarchy    *ContainerHierarchy
	// pages, layouts, microflows are cached separately as they may change during execution

	// Track items created during this session (not yet visible via reader)
	createdMicroflows map[string]*createdMicroflowInfo // qualifiedName -> info
	createdPages      map[string]*createdPageInfo      // qualifiedName -> info
	createdSnippets   map[string]*createdSnippetInfo   // qualifiedName -> info

	// Track domain models modified during this session for finalization
	modifiedDomainModels map[model.ID]string // domain model unit ID -> module name
}

// createdMicroflowInfo tracks a microflow created during this session.
type createdMicroflowInfo struct {
	ID               model.ID
	Name             string
	ModuleName       string
	ContainerID      model.ID
	ReturnEntityName string // Qualified entity name from return type (e.g., "Module.Entity")
}

// createdPageInfo tracks a page created during this session.
type createdPageInfo struct {
	ID          model.ID
	Name        string
	ModuleName  string
	ContainerID model.ID
}

// createdSnippetInfo tracks a snippet created during this session.
type createdSnippetInfo struct {
	ID          model.ID
	Name        string
	ModuleName  string
	ContainerID model.ID
}

const (
	// maxOutputLines is the per-statement line limit. Statements that produce more
	// lines than this are aborted to prevent runaway output from infinite loops.
	maxOutputLines = 10_000
	// executeTimeout is the maximum wall-clock time allowed for a single statement.
	executeTimeout = 5 * time.Minute
)

// Executor executes MDL statements against a Mendix project.
type Executor struct {
	writer        *mpr.Writer
	reader        *mpr.Reader
	output        io.Writer
	guard         *outputGuard // line-limit wrapper around output
	mprPath       string
	settings      map[string]any
	cache         *executorCache
	catalog       *catalog.Catalog
	quiet         bool                               // suppress connection and status messages
	logger        *diaglog.Logger                    // session diagnostics logger (nil = no logging)
	fragments     map[string]*ast.DefineFragmentStmt // script-scoped fragment definitions
	sqlMgr        *sqllib.Manager                    // external SQL connection manager (lazy init)
	themeRegistry *ThemeRegistry                     // cached theme design property definitions (lazy init)
}

// New creates a new executor with the given output writer.
func New(output io.Writer) *Executor {
	guard := newOutputGuard(output, maxOutputLines)
	return &Executor{
		output:   guard,
		guard:    guard,
		settings: make(map[string]any),
	}
}

// getThemeRegistry returns the cached theme registry, loading it lazily from the project's theme sources.
func (e *Executor) getThemeRegistry() *ThemeRegistry {
	if e.themeRegistry != nil {
		return e.themeRegistry
	}
	if e.mprPath == "" {
		return nil
	}
	projectDir := filepath.Dir(e.mprPath)
	registry, err := loadThemeRegistry(projectDir)
	if err == nil {
		e.themeRegistry = registry
	}
	return e.themeRegistry
}

// SetQuiet enables or disables quiet mode (suppresses connection/status messages).
func (e *Executor) SetQuiet(quiet bool) {
	e.quiet = quiet
}

// SetLogger sets the diagnostics logger for session logging.
func (e *Executor) SetLogger(l *diaglog.Logger) {
	e.logger = l
}

// Execute runs a single MDL statement with output-line and wall-clock guards.
// Each statement gets a fresh line budget. If the statement exceeds maxOutputLines
// lines of output or runs longer than executeTimeout, it is aborted with an error.
func (e *Executor) Execute(stmt ast.Statement) error {
	start := time.Now()

	// Reset per-statement line counter.
	if e.guard != nil {
		e.guard.reset()
	}

	// Run statement in a goroutine so we can enforce a wall-clock timeout.
	// The outputGuard handles race-safe writes if the goroutine outlives the timeout.
	type result struct{ err error }
	ch := make(chan result, 1)
	go func() {
		ch <- result{e.executeInner(stmt)}
	}()

	var err error
	select {
	case r := <-ch:
		err = r.err
	case <-time.After(executeTimeout):
		err = fmt.Errorf("statement timed out after %v", executeTimeout)
	}

	if e.logger != nil {
		e.logger.Command(stmtTypeName(stmt), stmtSummary(stmt), time.Since(start), err)
	}
	return err
}

// executeInner dispatches a statement to its handler.
func (e *Executor) executeInner(stmt ast.Statement) error {
	switch s := stmt.(type) {
	// Connection statements
	case *ast.ConnectStmt:
		return e.execConnect(s)
	case *ast.DisconnectStmt:
		return e.execDisconnect()
	case *ast.StatusStmt:
		return e.execStatus()

	// Module statements
	case *ast.CreateModuleStmt:
		return e.execCreateModule(s)
	case *ast.DropModuleStmt:
		return e.execDropModule(s)

	// Enumeration statements
	case *ast.CreateEnumerationStmt:
		return e.execCreateEnumeration(s)
	case *ast.AlterEnumerationStmt:
		return e.execAlterEnumeration(s)
	case *ast.DropEnumerationStmt:
		return e.execDropEnumeration(s)

	// Constant statements
	case *ast.CreateConstantStmt:
		return e.createConstant(s)
	case *ast.DropConstantStmt:
		return e.dropConstant(s)

	// Database Connection statements
	case *ast.CreateDatabaseConnectionStmt:
		return e.createDatabaseConnection(s)

	// Entity statements
	case *ast.CreateEntityStmt:
		return e.execCreateEntity(s)
	case *ast.CreateViewEntityStmt:
		return e.execCreateViewEntity(s)
	case *ast.AlterEntityStmt:
		return e.execAlterEntity(s)
	case *ast.DropEntityStmt:
		return e.execDropEntity(s)

	// Association statements
	case *ast.CreateAssociationStmt:
		return e.execCreateAssociation(s)
	case *ast.AlterAssociationStmt:
		return e.execAlterAssociation(s)
	case *ast.DropAssociationStmt:
		return e.execDropAssociation(s)

	// Microflow statements
	case *ast.CreateMicroflowStmt:
		return e.execCreateMicroflow(s)
	case *ast.DropMicroflowStmt:
		return e.execDropMicroflow(s)

	// Page statements
	case *ast.CreatePageStmtV3:
		return e.execCreatePageV3(s)
	case *ast.DropPageStmt:
		return e.execDropPage(s)
	case *ast.CreateSnippetStmtV3:
		return e.execCreateSnippetV3(s)
	case *ast.DropSnippetStmt:
		return e.execDropSnippet(s)
	case *ast.DropJavaActionStmt:
		return e.execDropJavaAction(s)
	case *ast.CreateJavaActionStmt:
		return e.execCreateJavaAction(s)
	case *ast.DropFolderStmt:
		return e.execDropFolder(s)
	case *ast.MoveFolderStmt:
		return e.execMoveFolder(s)
	case *ast.MoveStmt:
		return e.execMove(s)

	// Security statements
	case *ast.CreateModuleRoleStmt:
		return e.execCreateModuleRole(s)
	case *ast.DropModuleRoleStmt:
		return e.execDropModuleRole(s)
	case *ast.CreateUserRoleStmt:
		return e.execCreateUserRole(s)
	case *ast.AlterUserRoleStmt:
		return e.execAlterUserRole(s)
	case *ast.DropUserRoleStmt:
		return e.execDropUserRole(s)
	case *ast.GrantEntityAccessStmt:
		return e.execGrantEntityAccess(s)
	case *ast.RevokeEntityAccessStmt:
		return e.execRevokeEntityAccess(s)
	case *ast.GrantMicroflowAccessStmt:
		return e.execGrantMicroflowAccess(s)
	case *ast.RevokeMicroflowAccessStmt:
		return e.execRevokeMicroflowAccess(s)
	case *ast.GrantPageAccessStmt:
		return e.execGrantPageAccess(s)
	case *ast.RevokePageAccessStmt:
		return e.execRevokePageAccess(s)
	case *ast.GrantWorkflowAccessStmt:
		return e.execGrantWorkflowAccess(s)
	case *ast.RevokeWorkflowAccessStmt:
		return e.execRevokeWorkflowAccess(s)
	case *ast.AlterProjectSecurityStmt:
		return e.execAlterProjectSecurity(s)
	case *ast.CreateDemoUserStmt:
		return e.execCreateDemoUser(s)
	case *ast.DropDemoUserStmt:
		return e.execDropDemoUser(s)
	case *ast.UpdateSecurityStmt:
		return e.execUpdateSecurity(s)

	// Navigation statements
	case *ast.AlterNavigationStmt:
		return e.execAlterNavigation(s)

	// Image collection statements
	case *ast.CreateImageCollectionStmt:
		return e.execCreateImageCollection(s)
	case *ast.DropImageCollectionStmt:
		return e.execDropImageCollection(s)

	// JSON structure statements
	case *ast.CreateJsonStructureStmt:
		return e.execCreateJsonStructure(s)
	case *ast.DropJsonStructureStmt:
		return e.execDropJsonStructure(s)

	// Workflow statements
	case *ast.CreateWorkflowStmt:
		return e.execCreateWorkflow(s)
	case *ast.DropWorkflowStmt:
		return e.execDropWorkflow(s)

	// Business Event statements
	case *ast.CreateBusinessEventServiceStmt:
		return e.createBusinessEventService(s)
	case *ast.DropBusinessEventServiceStmt:
		return e.dropBusinessEventService(s)

	// Settings statements
	case *ast.AlterSettingsStmt:
		return e.alterSettings(s)
	case *ast.CreateConfigurationStmt:
		return e.createConfiguration(s)
	case *ast.DropConfigurationStmt:
		return e.dropConfiguration(s)

	// OData statements
	case *ast.CreateODataClientStmt:
		return e.createODataClient(s)
	case *ast.AlterODataClientStmt:
		return e.alterODataClient(s)
	case *ast.DropODataClientStmt:
		return e.dropODataClient(s)
	case *ast.CreateODataServiceStmt:
		return e.createODataService(s)
	case *ast.AlterODataServiceStmt:
		return e.alterODataService(s)
	case *ast.DropODataServiceStmt:
		return e.dropODataService(s)

	// REST client statements
	case *ast.CreateRestClientStmt:
		return e.createRestClient(s)
	case *ast.DropRestClientStmt:
		return e.dropRestClient(s)
	case *ast.CreateExternalEntityStmt:
		return e.execCreateExternalEntity(s)
	case *ast.GrantODataServiceAccessStmt:
		return e.execGrantODataServiceAccess(s)
	case *ast.RevokeODataServiceAccessStmt:
		return e.execRevokeODataServiceAccess(s)

	// Query statements
	case *ast.ShowStmt:
		return e.execShow(s)
	case *ast.ShowFeaturesStmt:
		return e.execShowFeatures(s)
	case *ast.ShowWidgetsStmt:
		return e.execShowWidgets(s)
	case *ast.UpdateWidgetsStmt:
		return e.execUpdateWidgets(s)
	case *ast.SelectStmt:
		return e.execCatalogQuery(s.Query)
	case *ast.DescribeStmt:
		return e.execDescribe(s)
	case *ast.DescribeCatalogTableStmt:
		return e.execDescribeCatalogTable(s)

	// Styling statements
	case *ast.ShowDesignPropertiesStmt:
		return e.execShowDesignProperties(s)
	case *ast.DescribeStylingStmt:
		return e.execDescribeStyling(s)
	case *ast.AlterStylingStmt:
		return e.execAlterStyling(s)

	// Repository statements
	case *ast.UpdateStmt:
		return e.execUpdate()
	case *ast.RefreshStmt:
		return e.execRefresh()
	case *ast.RefreshCatalogStmt:
		return e.execRefreshCatalogStmt(s)
	case *ast.SearchStmt:
		return e.execSearch(s)

	// Session statements
	case *ast.SetStmt:
		return e.execSet(s)
	case *ast.HelpStmt:
		return e.execHelp()
	case *ast.ExitStmt:
		return e.execExit()
	case *ast.ExecuteScriptStmt:
		return e.execExecuteScript(s)

	// Lint statements
	case *ast.LintStmt:
		return e.execLint(s)

	// ALTER PAGE statements
	case *ast.AlterPageStmt:
		return e.execAlterPage(s)

	// Fragment statements
	case *ast.DefineFragmentStmt:
		return e.execDefineFragment(s)
	case *ast.DescribeFragmentFromStmt:
		return e.describeFragmentFrom(s)

	// SQL statements (external database connectivity)
	case *ast.SQLConnectStmt:
		return e.execSQLConnect(s)
	case *ast.SQLDisconnectStmt:
		return e.execSQLDisconnect(s)
	case *ast.SQLConnectionsStmt:
		return e.execSQLConnections()
	case *ast.SQLQueryStmt:
		return e.execSQLQuery(s)
	case *ast.SQLShowTablesStmt:
		return e.execSQLShowTables(s)
	case *ast.SQLShowViewsStmt:
		return e.execSQLShowViews(s)
	case *ast.SQLShowFunctionsStmt:
		return e.execSQLShowFunctions(s)
	case *ast.SQLDescribeTableStmt:
		return e.execSQLDescribeTable(s)
	case *ast.SQLGenerateConnectorStmt:
		return e.execSQLGenerateConnector(s)

	// Import statements
	case *ast.ImportStmt:
		return e.execImport(s)

	default:
		return fmt.Errorf("unknown statement type: %T", stmt)
	}
}

// ExecuteProgram runs all statements in a program.
func (e *Executor) ExecuteProgram(prog *ast.Program) error {
	// Collect all names defined in the script for forward-reference hints.
	allDefined := newScriptContext()
	allDefined.collectDefinitions(prog)

	// Track which names have been created so far.
	created := newScriptContext()

	for _, stmt := range prog.Statements {
		if err := e.Execute(stmt); err != nil {
			return annotateForwardRef(err, stmt, created, allDefined)
		}
		created.collectSingle(stmt)
	}
	return e.finalizeProgramExecution()
}

// trackModifiedDomainModel records a domain model that was modified during execution,
// so it can be reconciled at the end of the program.
func (e *Executor) trackModifiedDomainModel(moduleID model.ID, moduleName string) {
	if e.writer == nil {
		return
	}
	if e.cache == nil {
		e.cache = &executorCache{}
	}
	if e.cache.modifiedDomainModels == nil {
		e.cache.modifiedDomainModels = make(map[model.ID]string)
	}
	// We store the module ID as key temporarily; we'll resolve to DM ID during finalization
	e.cache.modifiedDomainModels[moduleID] = moduleName
}

// finalizeProgramExecution runs post-execution reconciliation on modified domain models.
func (e *Executor) finalizeProgramExecution() error {
	if e.writer == nil || e.cache == nil || len(e.cache.modifiedDomainModels) == 0 {
		return nil
	}

	for moduleID, moduleName := range e.cache.modifiedDomainModels {
		dm, err := e.reader.GetDomainModel(moduleID)
		if err != nil {
			continue // module may not have a domain model
		}

		count, err := e.writer.ReconcileMemberAccesses(dm.ID, moduleName)
		if err != nil {
			return fmt.Errorf("finalization: failed to reconcile security for module %s: %w", moduleName, err)
		}
		if count > 0 && !e.quiet {
			fmt.Fprintf(e.output, "Reconciled %d access rule(s) in module %s\n", count, moduleName)
		}
	}

	// Clear tracking
	e.cache.modifiedDomainModels = nil
	return nil
}

// Catalog returns the catalog, or nil if not built.
func (e *Executor) Catalog() *catalog.Catalog {
	return e.catalog
}

// Reader returns the MPR reader, or nil if not connected.
func (e *Executor) Reader() *mpr.Reader {
	return e.reader
}

// IsConnected returns true if connected to a project.
func (e *Executor) IsConnected() bool {
	return e.writer != nil
}

// Close closes the connection to the project and all SQL connections.
func (e *Executor) Close() error {
	if e.writer != nil {
		e.writer.Close()
		e.writer = nil
		e.reader = nil
	}
	if e.sqlMgr != nil {
		e.sqlMgr.CloseAll()
		e.sqlMgr = nil
	}
	return nil
}

// ----------------------------------------------------------------------------
// Cache and Tracking
// ----------------------------------------------------------------------------

// trackCreatedMicroflow registers a microflow that was created during this session.
// This allows subsequent page creations to resolve references to the microflow
// even though the reader cache hasn't been updated.
func (e *Executor) trackCreatedMicroflow(moduleName, mfName string, id, containerID model.ID, returnEntityName string) {
	e.ensureCache()
	if e.cache.createdMicroflows == nil {
		e.cache.createdMicroflows = make(map[string]*createdMicroflowInfo)
	}
	qualifiedName := moduleName + "." + mfName
	e.cache.createdMicroflows[qualifiedName] = &createdMicroflowInfo{
		ID:               id,
		Name:             mfName,
		ModuleName:       moduleName,
		ContainerID:      containerID,
		ReturnEntityName: returnEntityName,
	}
}

// trackCreatedPage registers a page that was created during this session.
// This allows subsequent page creations to resolve SHOW_PAGE references
// even though the reader cache hasn't been updated.
func (e *Executor) trackCreatedPage(moduleName, pageName string, id, containerID model.ID) {
	e.ensureCache()
	if e.cache.createdPages == nil {
		e.cache.createdPages = make(map[string]*createdPageInfo)
	}
	qualifiedName := moduleName + "." + pageName
	e.cache.createdPages[qualifiedName] = &createdPageInfo{
		ID:          id,
		Name:        pageName,
		ModuleName:  moduleName,
		ContainerID: containerID,
	}
}

// trackCreatedSnippet registers a snippet that was created during this session.
// This allows subsequent creations to resolve snippet references
// even though the reader cache hasn't been updated.
func (e *Executor) trackCreatedSnippet(moduleName, snippetName string, id, containerID model.ID) {
	e.ensureCache()
	if e.cache.createdSnippets == nil {
		e.cache.createdSnippets = make(map[string]*createdSnippetInfo)
	}
	qualifiedName := moduleName + "." + snippetName
	e.cache.createdSnippets[qualifiedName] = &createdSnippetInfo{
		ID:          id,
		Name:        snippetName,
		ModuleName:  moduleName,
		ContainerID: containerID,
	}
}

// getCreatedMicroflow returns info about a microflow created during this session,
// or nil if not found.
func (e *Executor) getCreatedMicroflow(qualifiedName string) *createdMicroflowInfo {
	if e.cache == nil || e.cache.createdMicroflows == nil {
		return nil
	}
	return e.cache.createdMicroflows[qualifiedName]
}

// getCreatedPage returns info about a page created during this session,
// or nil if not found.
func (e *Executor) getCreatedPage(qualifiedName string) *createdPageInfo {
	if e.cache == nil || e.cache.createdPages == nil {
		return nil
	}
	return e.cache.createdPages[qualifiedName]
}

// ensureCache initializes the executor cache if not already initialized.
func (e *Executor) ensureCache() {
	if e.cache == nil {
		e.cache = &executorCache{}
	}
}

// ----------------------------------------------------------------------------
// Connection Statements
// ----------------------------------------------------------------------------

func (e *Executor) execConnect(s *ast.ConnectStmt) error {
	if e.writer != nil {
		e.writer.Close()
	}

	writer, err := mpr.NewWriter(s.Path)
	if err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}

	e.writer = writer
	e.reader = writer.Reader()
	e.mprPath = s.Path
	e.cache = &executorCache{} // Initialize fresh cache

	// Display connection info with version
	pv := e.reader.ProjectVersion()
	if !e.quiet {
		fmt.Fprintf(e.output, "Connected to: %s (Mendix %s)\n", s.Path, pv.ProductVersion)
	}
	if e.logger != nil {
		e.logger.Connect(s.Path, pv.ProductVersion, pv.FormatVersion)
	}
	return nil
}

// reconnect closes the current connection and reopens it.
// This is needed when the project file has been modified externally.
func (e *Executor) reconnect() error {
	if e.mprPath == "" {
		return fmt.Errorf("no project path to reconnect to")
	}

	// Close existing connection
	if e.writer != nil {
		e.writer.Close()
	}

	// Reopen connection
	writer, err := mpr.NewWriter(e.mprPath)
	if err != nil {
		return fmt.Errorf("failed to reconnect: %w", err)
	}

	e.writer = writer
	e.reader = writer.Reader()
	e.cache = &executorCache{} // Reset cache
	return nil
}

func (e *Executor) execDisconnect() error {
	if e.writer == nil {
		fmt.Fprintln(e.output, "Not connected")
		return nil
	}

	// Reconcile any pending security changes before closing
	if err := e.finalizeProgramExecution(); err != nil {
		fmt.Fprintf(e.output, "Warning: finalization error: %v\n", err)
	}

	e.writer.Close()
	fmt.Fprintf(e.output, "Disconnected from: %s\n", e.mprPath)
	e.writer = nil
	e.reader = nil
	e.mprPath = ""
	e.cache = nil
	return nil
}

func (e *Executor) execStatus() error {
	if e.writer == nil {
		fmt.Fprintln(e.output, "Status: Not connected")
		return nil
	}

	pv := e.reader.ProjectVersion()
	fmt.Fprintf(e.output, "Status: Connected\n")
	fmt.Fprintf(e.output, "Project: %s\n", e.mprPath)
	fmt.Fprintf(e.output, "Mendix Version: %s\n", pv.ProductVersion)
	fmt.Fprintf(e.output, "MPR Format: v%d\n", pv.FormatVersion)

	// Show module count
	modules, err := e.reader.ListModules()
	if err == nil {
		fmt.Fprintf(e.output, "Modules: %d\n", len(modules))
	}

	return nil
}

// ----------------------------------------------------------------------------
// Query Dispatch Statements
// ----------------------------------------------------------------------------

func (e *Executor) execShow(s *ast.ShowStmt) error {
	if e.reader == nil && s.ObjectType != ast.ShowModules && s.ObjectType != ast.ShowFragments {
		return fmt.Errorf("not connected to a project")
	}

	switch s.ObjectType {
	case ast.ShowModules:
		return e.showModules()
	case ast.ShowEnumerations:
		return e.showEnumerations(s.InModule)
	case ast.ShowConstants:
		return e.showConstants(s.InModule)
	case ast.ShowConstantValues:
		return e.showConstantValues(s.InModule)
	case ast.ShowEntities:
		return e.showEntities(s.InModule)
	case ast.ShowEntity:
		return e.showEntity(s.Name)
	case ast.ShowAssociations:
		return e.showAssociations(s.InModule)
	case ast.ShowAssociation:
		return e.showAssociation(s.Name)
	case ast.ShowMicroflows:
		return e.showMicroflows(s.InModule)
	case ast.ShowNanoflows:
		return e.showNanoflows(s.InModule)
	case ast.ShowPages:
		return e.showPages(s.InModule)
	case ast.ShowSnippets:
		return e.showSnippets(s.InModule)
	case ast.ShowLayouts:
		return e.showLayouts(s.InModule)
	case ast.ShowJavaActions:
		return e.showJavaActions(s.InModule)
	case ast.ShowJavaScriptActions:
		return e.showJavaScriptActions(s.InModule)
	case ast.ShowVersion:
		return e.showVersion()
	case ast.ShowCatalogTables:
		return e.execShowCatalogTables()
	case ast.ShowCatalogStatus:
		return e.execShowCatalogStatus()
	case ast.ShowCallers:
		return e.execShowCallers(s)
	case ast.ShowCallees:
		return e.execShowCallees(s)
	case ast.ShowReferences:
		return e.execShowReferences(s)
	case ast.ShowImpact:
		return e.execShowImpact(s)
	case ast.ShowContext:
		return e.execShowContext(s)
	case ast.ShowProjectSecurity:
		return e.showProjectSecurity()
	case ast.ShowModuleRoles:
		return e.showModuleRoles(s.InModule)
	case ast.ShowUserRoles:
		return e.showUserRoles()
	case ast.ShowDemoUsers:
		return e.showDemoUsers()
	case ast.ShowAccessOn:
		return e.showAccessOnEntity(s.Name)
	case ast.ShowAccessOnMicroflow:
		return e.showAccessOnMicroflow(s.Name)
	case ast.ShowAccessOnPage:
		return e.showAccessOnPage(s.Name)
	case ast.ShowAccessOnWorkflow:
		return e.showAccessOnWorkflow(s.Name)
	case ast.ShowSecurityMatrix:
		return e.showSecurityMatrix(s.InModule)
	case ast.ShowODataClients:
		return e.showODataClients(s.InModule)
	case ast.ShowODataServices:
		return e.showODataServices(s.InModule)
	case ast.ShowExternalEntities:
		return e.showExternalEntities(s.InModule)
	case ast.ShowExternalActions:
		return e.showExternalActions(s.InModule)
	case ast.ShowNavigation:
		return e.showNavigation()
	case ast.ShowNavigationMenu:
		return e.showNavigationMenu(s.Name)
	case ast.ShowNavigationHomes:
		return e.showNavigationHomes()
	case ast.ShowStructure:
		return e.execShowStructure(s)
	case ast.ShowWorkflows:
		return e.showWorkflows(s.InModule)
	case ast.ShowBusinessEventServices:
		return e.showBusinessEventServices(s.InModule)
	case ast.ShowBusinessEventClients:
		return e.showBusinessEventClients(s.InModule)
	case ast.ShowBusinessEvents:
		return e.showBusinessEvents(s.InModule)
	case ast.ShowSettings:
		return e.showSettings()
	case ast.ShowFragments:
		return e.showFragments()
	case ast.ShowDatabaseConnections:
		return e.showDatabaseConnections(s.InModule)
	case ast.ShowImageCollections:
		return e.showImageCollections(s.InModule)
	case ast.ShowJsonStructures:
		return e.showJsonStructures(s.InModule)
	case ast.ShowRestClients:
		return e.showRestClients(s.InModule)
	case ast.ShowPublishedRestServices:
		return e.showPublishedRestServices(s.InModule)
	case ast.ShowLanguages:
		return e.showLanguages()
	case ast.ShowContractEntities:
		return e.showContractEntities(s.Name)
	case ast.ShowContractActions:
		return e.showContractActions(s.Name)
	case ast.ShowContractChannels:
		return e.showContractChannels(s.Name)
	case ast.ShowContractMessages:
		return e.showContractMessages(s.Name)
	default:
		return fmt.Errorf("unknown show object type")
	}
}

func (e *Executor) execDescribe(s *ast.DescribeStmt) error {
	if e.reader == nil && s.ObjectType != ast.DescribeFragment {
		return fmt.Errorf("not connected to a project")
	}

	switch s.ObjectType {
	case ast.DescribeEnumeration:
		return e.describeEnumeration(s.Name)
	case ast.DescribeEntity:
		return e.describeEntity(s.Name)
	case ast.DescribeAssociation:
		return e.describeAssociation(s.Name)
	case ast.DescribeMicroflow:
		return e.describeMicroflow(s.Name)
	case ast.DescribeModule:
		return e.describeModule(s.Name.Module, s.WithAll)
	case ast.DescribePage:
		return e.describePage(s.Name)
	case ast.DescribeSnippet:
		return e.describeSnippet(s.Name)
	case ast.DescribeLayout:
		return e.describeLayout(s.Name)
	case ast.DescribeConstant:
		return e.describeConstant(s.Name)
	case ast.DescribeJavaAction:
		return e.describeJavaAction(s.Name)
	case ast.DescribeJavaScriptAction:
		return e.describeJavaScriptAction(s.Name)
	case ast.DescribeModuleRole:
		return e.describeModuleRole(s.Name)
	case ast.DescribeUserRole:
		return e.describeUserRole(s.Name)
	case ast.DescribeDemoUser:
		return e.describeDemoUser(s.Name.Name)
	case ast.DescribeODataClient:
		return e.describeODataClient(s.Name)
	case ast.DescribeODataService:
		return e.describeODataService(s.Name)
	case ast.DescribeExternalEntity:
		return e.describeExternalEntity(s.Name)
	case ast.DescribeNavigation:
		return e.describeNavigation(s.Name)
	case ast.DescribeWorkflow:
		return e.describeWorkflow(s.Name)
	case ast.DescribeBusinessEventService:
		return e.describeBusinessEventService(s.Name)
	case ast.DescribeDatabaseConnection:
		return e.describeDatabaseConnection(s.Name)
	case ast.DescribeSettings:
		return e.describeSettings()
	case ast.DescribeFragment:
		return e.describeFragment(s.Name)
	case ast.DescribeImageCollection:
		return e.describeImageCollection(s.Name)
	case ast.DescribeJsonStructure:
		return e.describeJsonStructure(s.Name)
	case ast.DescribeRestClient:
		return e.describeRestClient(s.Name)
	case ast.DescribePublishedRestService:
		return e.describePublishedRestService(s.Name)
	case ast.DescribeContractEntity:
		return e.describeContractEntity(s.Name, s.Format)
	case ast.DescribeContractAction:
		return e.describeContractAction(s.Name, s.Format)
	case ast.DescribeContractMessage:
		return e.describeContractMessage(s.Name)
	default:
		return fmt.Errorf("unknown describe object type")
	}
}
