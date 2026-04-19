// SPDX-License-Identifier: Apache-2.0

// Package mprbackend provides the MprBackend implementation of
// backend.FullBackend. The package name is "mprbackend" (not "mpr") to
// avoid collision with the sdk/mpr package in import blocks.
package mprbackend

import (
	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/agenteditor"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
	"github.com/mendixlabs/mxcli/sdk/javaactions"
	"github.com/mendixlabs/mxcli/sdk/microflows"
	"github.com/mendixlabs/mxcli/sdk/mpr"
	"github.com/mendixlabs/mxcli/sdk/pages"
	"github.com/mendixlabs/mxcli/sdk/security"
	"github.com/mendixlabs/mxcli/sdk/workflows"
)

var _ backend.FullBackend = (*MprBackend)(nil)

// MprBackend implements backend.FullBackend by delegating to mpr.Reader
// and mpr.Writer.
//
// Methods that access reader or writer assume Connect() has been called.
// Calling read/write methods before Connect() will panic with a nil
// pointer dereference. The executor enforces connection state via
// ConnectionBackend.IsConnected() before dispatching handlers.
type MprBackend struct {
	reader *mpr.Reader
	writer *mpr.Writer
	path   string
}

// Wrap creates an MprBackend that wraps an existing Writer (and its Reader).
// This is used during migration when the Executor already owns the Writer
// and we want to expose it through the Backend interface without opening
// a second connection.
func Wrap(writer *mpr.Writer, path string) *MprBackend {
	return &MprBackend{
		reader: writer.Reader(),
		writer: writer,
		path:   path,
	}
}

// ---------------------------------------------------------------------------
// ConnectionBackend
// ---------------------------------------------------------------------------

func (b *MprBackend) Connect(path string) error {
	w, err := mpr.NewWriter(path)
	if err != nil {
		return err
	}
	b.writer = w
	b.reader = w.Reader()
	b.path = path
	return nil
}

func (b *MprBackend) Disconnect() error {
	if b.writer == nil {
		return nil
	}
	err := b.writer.Close()
	b.writer = nil
	b.reader = nil
	b.path = ""
	return err
}

func (b *MprBackend) IsConnected() bool { return b.writer != nil }
func (b *MprBackend) Path() string      { return b.path }

func (b *MprBackend) Version() types.MPRVersion                 { return convertMPRVersion(b.reader.Version()) }
func (b *MprBackend) ProjectVersion() *types.ProjectVersion     { return convertProjectVersion(b.reader.ProjectVersion()) }
func (b *MprBackend) GetMendixVersion() (string, error)         { return b.reader.GetMendixVersion() }

// Commit is a no-op — the MPR writer auto-commits on each write operation.
func (b *MprBackend) Commit() error { return nil }

// ---------------------------------------------------------------------------
// ModuleBackend
// ---------------------------------------------------------------------------

func (b *MprBackend) ListModules() ([]*model.Module, error)        { return b.reader.ListModules() }
func (b *MprBackend) GetModule(id model.ID) (*model.Module, error) { return b.reader.GetModule(id) }
func (b *MprBackend) GetModuleByName(name string) (*model.Module, error) {
	return b.reader.GetModuleByName(name)
}
func (b *MprBackend) CreateModule(module *model.Module) error { return b.writer.CreateModule(module) }
func (b *MprBackend) UpdateModule(module *model.Module) error { return b.writer.UpdateModule(module) }
func (b *MprBackend) DeleteModule(id model.ID) error          { return b.writer.DeleteModule(id) }
func (b *MprBackend) DeleteModuleWithCleanup(id model.ID, moduleName string) error {
	return b.writer.DeleteModuleWithCleanup(id, moduleName)
}

// ---------------------------------------------------------------------------
// FolderBackend
// ---------------------------------------------------------------------------

func (b *MprBackend) ListFolders() ([]*types.FolderInfo, error) { return convertFolderInfoSlice(b.reader.ListFolders()) }
func (b *MprBackend) CreateFolder(folder *model.Folder) error { return b.writer.CreateFolder(folder) }
func (b *MprBackend) DeleteFolder(id model.ID) error          { return b.writer.DeleteFolder(id) }
func (b *MprBackend) MoveFolder(id model.ID, newContainerID model.ID) error {
	return b.writer.MoveFolder(id, newContainerID)
}

// ---------------------------------------------------------------------------
// DomainModelBackend
// ---------------------------------------------------------------------------

func (b *MprBackend) ListDomainModels() ([]*domainmodel.DomainModel, error) {
	return b.reader.ListDomainModels()
}
func (b *MprBackend) GetDomainModel(moduleID model.ID) (*domainmodel.DomainModel, error) {
	return b.reader.GetDomainModel(moduleID)
}
func (b *MprBackend) GetDomainModelByID(id model.ID) (*domainmodel.DomainModel, error) {
	return b.reader.GetDomainModelByID(id)
}
func (b *MprBackend) UpdateDomainModel(dm *domainmodel.DomainModel) error {
	return b.writer.UpdateDomainModel(dm)
}

func (b *MprBackend) CreateEntity(domainModelID model.ID, entity *domainmodel.Entity) error {
	return b.writer.CreateEntity(domainModelID, entity)
}
func (b *MprBackend) UpdateEntity(domainModelID model.ID, entity *domainmodel.Entity) error {
	return b.writer.UpdateEntity(domainModelID, entity)
}
func (b *MprBackend) DeleteEntity(domainModelID model.ID, entityID model.ID) error {
	return b.writer.DeleteEntity(domainModelID, entityID)
}
func (b *MprBackend) MoveEntity(entity *domainmodel.Entity, sourceDMID, targetDMID model.ID, sourceModuleName, targetModuleName string) ([]string, error) {
	return b.writer.MoveEntity(entity, sourceDMID, targetDMID, sourceModuleName, targetModuleName)
}

func (b *MprBackend) AddAttribute(domainModelID model.ID, entityID model.ID, attr *domainmodel.Attribute) error {
	return b.writer.AddAttribute(domainModelID, entityID, attr)
}
func (b *MprBackend) UpdateAttribute(domainModelID model.ID, entityID model.ID, attr *domainmodel.Attribute) error {
	return b.writer.UpdateAttribute(domainModelID, entityID, attr)
}
func (b *MprBackend) DeleteAttribute(domainModelID model.ID, entityID model.ID, attrID model.ID) error {
	return b.writer.DeleteAttribute(domainModelID, entityID, attrID)
}

func (b *MprBackend) CreateAssociation(domainModelID model.ID, assoc *domainmodel.Association) error {
	return b.writer.CreateAssociation(domainModelID, assoc)
}
func (b *MprBackend) CreateCrossAssociation(domainModelID model.ID, ca *domainmodel.CrossModuleAssociation) error {
	return b.writer.CreateCrossAssociation(domainModelID, ca)
}
func (b *MprBackend) DeleteAssociation(domainModelID model.ID, assocID model.ID) error {
	return b.writer.DeleteAssociation(domainModelID, assocID)
}
func (b *MprBackend) DeleteCrossAssociation(domainModelID model.ID, assocID model.ID) error {
	return b.writer.DeleteCrossAssociation(domainModelID, assocID)
}

func (b *MprBackend) CreateViewEntitySourceDocument(moduleID model.ID, moduleName, docName, oqlQuery, documentation string) (model.ID, error) {
	return b.writer.CreateViewEntitySourceDocument(moduleID, moduleName, docName, oqlQuery, documentation)
}
func (b *MprBackend) DeleteViewEntitySourceDocument(id model.ID) error {
	return b.writer.DeleteViewEntitySourceDocument(id)
}
func (b *MprBackend) DeleteViewEntitySourceDocumentByName(moduleName, docName string) error {
	return b.writer.DeleteViewEntitySourceDocumentByName(moduleName, docName)
}
func (b *MprBackend) FindViewEntitySourceDocumentID(moduleName, docName string) (model.ID, error) {
	return b.writer.FindViewEntitySourceDocumentID(moduleName, docName)
}
func (b *MprBackend) FindAllViewEntitySourceDocumentIDs(moduleName, docName string) ([]model.ID, error) {
	return b.writer.FindAllViewEntitySourceDocumentIDs(moduleName, docName)
}
func (b *MprBackend) MoveViewEntitySourceDocument(sourceModuleName string, targetModuleID model.ID, docName string) error {
	return b.writer.MoveViewEntitySourceDocument(sourceModuleName, targetModuleID, docName)
}
func (b *MprBackend) UpdateOqlQueriesForMovedEntity(oldQualifiedName, newQualifiedName string) (int, error) {
	return b.writer.UpdateOqlQueriesForMovedEntity(oldQualifiedName, newQualifiedName)
}
func (b *MprBackend) UpdateEnumerationRefsInAllDomainModels(oldQualifiedName, newQualifiedName string) error {
	return b.writer.UpdateEnumerationRefsInAllDomainModels(oldQualifiedName, newQualifiedName)
}

// ---------------------------------------------------------------------------
// MicroflowBackend
// ---------------------------------------------------------------------------

func (b *MprBackend) ListMicroflows() ([]*microflows.Microflow, error) {
	return b.reader.ListMicroflows()
}
func (b *MprBackend) GetMicroflow(id model.ID) (*microflows.Microflow, error) {
	return b.reader.GetMicroflow(id)
}
func (b *MprBackend) CreateMicroflow(mf *microflows.Microflow) error {
	return b.writer.CreateMicroflow(mf)
}
func (b *MprBackend) UpdateMicroflow(mf *microflows.Microflow) error {
	return b.writer.UpdateMicroflow(mf)
}
func (b *MprBackend) DeleteMicroflow(id model.ID) error { return b.writer.DeleteMicroflow(id) }
func (b *MprBackend) MoveMicroflow(mf *microflows.Microflow) error {
	return b.writer.MoveMicroflow(mf)
}

func (b *MprBackend) ListNanoflows() ([]*microflows.Nanoflow, error) {
	return b.reader.ListNanoflows()
}
func (b *MprBackend) GetNanoflow(id model.ID) (*microflows.Nanoflow, error) {
	return b.reader.GetNanoflow(id)
}
func (b *MprBackend) CreateNanoflow(nf *microflows.Nanoflow) error {
	return b.writer.CreateNanoflow(nf)
}
func (b *MprBackend) UpdateNanoflow(nf *microflows.Nanoflow) error {
	return b.writer.UpdateNanoflow(nf)
}
func (b *MprBackend) DeleteNanoflow(id model.ID) error { return b.writer.DeleteNanoflow(id) }
func (b *MprBackend) MoveNanoflow(nf *microflows.Nanoflow) error {
	return b.writer.MoveNanoflow(nf)
}

// ---------------------------------------------------------------------------
// PageBackend
// ---------------------------------------------------------------------------

func (b *MprBackend) ListPages() ([]*pages.Page, error)        { return b.reader.ListPages() }
func (b *MprBackend) GetPage(id model.ID) (*pages.Page, error) { return b.reader.GetPage(id) }
func (b *MprBackend) CreatePage(page *pages.Page) error        { return b.writer.CreatePage(page) }
func (b *MprBackend) UpdatePage(page *pages.Page) error        { return b.writer.UpdatePage(page) }
func (b *MprBackend) DeletePage(id model.ID) error             { return b.writer.DeletePage(id) }
func (b *MprBackend) MovePage(page *pages.Page) error          { return b.writer.MovePage(page) }

func (b *MprBackend) ListLayouts() ([]*pages.Layout, error)        { return b.reader.ListLayouts() }
func (b *MprBackend) GetLayout(id model.ID) (*pages.Layout, error) { return b.reader.GetLayout(id) }
func (b *MprBackend) CreateLayout(layout *pages.Layout) error      { return b.writer.CreateLayout(layout) }
func (b *MprBackend) UpdateLayout(layout *pages.Layout) error      { return b.writer.UpdateLayout(layout) }
func (b *MprBackend) DeleteLayout(id model.ID) error               { return b.writer.DeleteLayout(id) }

func (b *MprBackend) ListSnippets() ([]*pages.Snippet, error) { return b.reader.ListSnippets() }
func (b *MprBackend) CreateSnippet(snippet *pages.Snippet) error {
	return b.writer.CreateSnippet(snippet)
}
func (b *MprBackend) UpdateSnippet(snippet *pages.Snippet) error {
	return b.writer.UpdateSnippet(snippet)
}
func (b *MprBackend) DeleteSnippet(id model.ID) error          { return b.writer.DeleteSnippet(id) }
func (b *MprBackend) MoveSnippet(snippet *pages.Snippet) error { return b.writer.MoveSnippet(snippet) }

func (b *MprBackend) ListBuildingBlocks() ([]*pages.BuildingBlock, error) {
	return b.reader.ListBuildingBlocks()
}
func (b *MprBackend) ListPageTemplates() ([]*pages.PageTemplate, error) {
	return b.reader.ListPageTemplates()
}

// ---------------------------------------------------------------------------
// EnumerationBackend
// ---------------------------------------------------------------------------

func (b *MprBackend) ListEnumerations() ([]*model.Enumeration, error) {
	return b.reader.ListEnumerations()
}
func (b *MprBackend) GetEnumeration(id model.ID) (*model.Enumeration, error) {
	return b.reader.GetEnumeration(id)
}
func (b *MprBackend) CreateEnumeration(enum *model.Enumeration) error {
	return b.writer.CreateEnumeration(enum)
}
func (b *MprBackend) UpdateEnumeration(enum *model.Enumeration) error {
	return b.writer.UpdateEnumeration(enum)
}
func (b *MprBackend) MoveEnumeration(enum *model.Enumeration) error {
	return b.writer.MoveEnumeration(enum)
}
func (b *MprBackend) DeleteEnumeration(id model.ID) error { return b.writer.DeleteEnumeration(id) }

// ---------------------------------------------------------------------------
// ConstantBackend
// ---------------------------------------------------------------------------

func (b *MprBackend) ListConstants() ([]*model.Constant, error) { return b.reader.ListConstants() }
func (b *MprBackend) GetConstant(id model.ID) (*model.Constant, error) {
	return b.reader.GetConstant(id)
}
func (b *MprBackend) CreateConstant(constant *model.Constant) error {
	return b.writer.CreateConstant(constant)
}
func (b *MprBackend) UpdateConstant(constant *model.Constant) error {
	return b.writer.UpdateConstant(constant)
}
func (b *MprBackend) MoveConstant(constant *model.Constant) error {
	return b.writer.MoveConstant(constant)
}
func (b *MprBackend) DeleteConstant(id model.ID) error { return b.writer.DeleteConstant(id) }

// ---------------------------------------------------------------------------
// SecurityBackend (ProjectSecurityBackend + ModuleSecurityBackend + EntityAccessBackend)
// ---------------------------------------------------------------------------

func (b *MprBackend) GetProjectSecurity() (*security.ProjectSecurity, error) {
	return b.reader.GetProjectSecurity()
}
func (b *MprBackend) SetProjectSecurityLevel(unitID model.ID, level string) error {
	return b.writer.SetProjectSecurityLevel(unitID, level)
}
func (b *MprBackend) SetProjectDemoUsersEnabled(unitID model.ID, enabled bool) error {
	return b.writer.SetProjectDemoUsersEnabled(unitID, enabled)
}
func (b *MprBackend) AddUserRole(unitID model.ID, name string, moduleRoles []string, manageAllRoles bool) error {
	return b.writer.AddUserRole(unitID, name, moduleRoles, manageAllRoles)
}
func (b *MprBackend) AlterUserRoleModuleRoles(unitID model.ID, userRoleName string, add bool, moduleRoles []string) error {
	return b.writer.AlterUserRoleModuleRoles(unitID, userRoleName, add, moduleRoles)
}
func (b *MprBackend) RemoveUserRole(unitID model.ID, name string) error {
	return b.writer.RemoveUserRole(unitID, name)
}
func (b *MprBackend) AddDemoUser(unitID model.ID, userName, password, entity string, userRoles []string) error {
	return b.writer.AddDemoUser(unitID, userName, password, entity, userRoles)
}
func (b *MprBackend) RemoveDemoUser(unitID model.ID, userName string) error {
	return b.writer.RemoveDemoUser(unitID, userName)
}

func (b *MprBackend) ListModuleSecurity() ([]*security.ModuleSecurity, error) {
	return b.reader.ListModuleSecurity()
}
func (b *MprBackend) GetModuleSecurity(moduleID model.ID) (*security.ModuleSecurity, error) {
	return b.reader.GetModuleSecurity(moduleID)
}
func (b *MprBackend) AddModuleRole(unitID model.ID, roleName, description string) error {
	return b.writer.AddModuleRole(unitID, roleName, description)
}
func (b *MprBackend) RemoveModuleRole(unitID model.ID, roleName string) error {
	return b.writer.RemoveModuleRole(unitID, roleName)
}
func (b *MprBackend) RemoveModuleRoleFromAllUserRoles(unitID model.ID, qualifiedRole string) (int, error) {
	return b.writer.RemoveModuleRoleFromAllUserRoles(unitID, qualifiedRole)
}

func (b *MprBackend) UpdateAllowedRoles(unitID model.ID, roles []string) error {
	return b.writer.UpdateAllowedRoles(unitID, roles)
}
func (b *MprBackend) UpdatePublishedRestServiceRoles(unitID model.ID, roles []string) error {
	return b.writer.UpdatePublishedRestServiceRoles(unitID, roles)
}
func (b *MprBackend) RemoveFromAllowedRoles(unitID model.ID, roleName string) (bool, error) {
	return b.writer.RemoveFromAllowedRoles(unitID, roleName)
}
func (b *MprBackend) AddEntityAccessRule(params backend.EntityAccessRuleParams) error {
	return b.writer.AddEntityAccessRule(params.UnitID, params.EntityName, params.RoleNames, params.AllowCreate, params.AllowDelete, params.DefaultMemberAccess, params.XPathConstraint, unconvertEntityMemberAccessSlice(params.MemberAccesses))
}
func (b *MprBackend) RemoveEntityAccessRule(unitID model.ID, entityName string, roleNames []string) (int, error) {
	return b.writer.RemoveEntityAccessRule(unitID, entityName, roleNames)
}
func (b *MprBackend) RevokeEntityMemberAccess(unitID model.ID, entityName string, roleNames []string, revocation types.EntityAccessRevocation) (int, error) {
	return b.writer.RevokeEntityMemberAccess(unitID, entityName, roleNames, unconvertEntityAccessRevocation(revocation))
}
func (b *MprBackend) RemoveRoleFromAllEntities(unitID model.ID, roleName string) (int, error) {
	return b.writer.RemoveRoleFromAllEntities(unitID, roleName)
}
func (b *MprBackend) ReconcileMemberAccesses(unitID model.ID, moduleName string) (int, error) {
	return b.writer.ReconcileMemberAccesses(unitID, moduleName)
}

// ---------------------------------------------------------------------------
// NavigationBackend
// ---------------------------------------------------------------------------

func (b *MprBackend) ListNavigationDocuments() ([]*types.NavigationDocument, error) {
	return convertNavDocSlice(b.reader.ListNavigationDocuments())
}
func (b *MprBackend) GetNavigation() (*types.NavigationDocument, error) {
	return convertNavDocPtr(b.reader.GetNavigation())
}
func (b *MprBackend) UpdateNavigationProfile(navDocID model.ID, profileName string, spec types.NavigationProfileSpec) error {
	return b.writer.UpdateNavigationProfile(navDocID, profileName, unconvertNavProfileSpec(spec))
}

// ---------------------------------------------------------------------------
// ServiceBackend (OData + REST + BusinessEvent + DatabaseConnection + DataTransformer)
// ---------------------------------------------------------------------------

func (b *MprBackend) ListConsumedODataServices() ([]*model.ConsumedODataService, error) {
	return b.reader.ListConsumedODataServices()
}
func (b *MprBackend) ListPublishedODataServices() ([]*model.PublishedODataService, error) {
	return b.reader.ListPublishedODataServices()
}
func (b *MprBackend) CreateConsumedODataService(svc *model.ConsumedODataService) error {
	return b.writer.CreateConsumedODataService(svc)
}
func (b *MprBackend) UpdateConsumedODataService(svc *model.ConsumedODataService) error {
	return b.writer.UpdateConsumedODataService(svc)
}
func (b *MprBackend) DeleteConsumedODataService(id model.ID) error {
	return b.writer.DeleteConsumedODataService(id)
}
func (b *MprBackend) CreatePublishedODataService(svc *model.PublishedODataService) error {
	return b.writer.CreatePublishedODataService(svc)
}
func (b *MprBackend) UpdatePublishedODataService(svc *model.PublishedODataService) error {
	return b.writer.UpdatePublishedODataService(svc)
}
func (b *MprBackend) DeletePublishedODataService(id model.ID) error {
	return b.writer.DeletePublishedODataService(id)
}

func (b *MprBackend) ListConsumedRestServices() ([]*model.ConsumedRestService, error) {
	return b.reader.ListConsumedRestServices()
}
func (b *MprBackend) ListPublishedRestServices() ([]*model.PublishedRestService, error) {
	return b.reader.ListPublishedRestServices()
}
func (b *MprBackend) CreateConsumedRestService(svc *model.ConsumedRestService) error {
	return b.writer.CreateConsumedRestService(svc)
}
func (b *MprBackend) UpdateConsumedRestService(svc *model.ConsumedRestService) error {
	return b.writer.UpdateConsumedRestService(svc)
}
func (b *MprBackend) DeleteConsumedRestService(id model.ID) error {
	return b.writer.DeleteConsumedRestService(id)
}
func (b *MprBackend) CreatePublishedRestService(svc *model.PublishedRestService) error {
	return b.writer.CreatePublishedRestService(svc)
}
func (b *MprBackend) UpdatePublishedRestService(svc *model.PublishedRestService) error {
	return b.writer.UpdatePublishedRestService(svc)
}
func (b *MprBackend) DeletePublishedRestService(id model.ID) error {
	return b.writer.DeletePublishedRestService(id)
}

func (b *MprBackend) ListBusinessEventServices() ([]*model.BusinessEventService, error) {
	return b.reader.ListBusinessEventServices()
}
func (b *MprBackend) CreateBusinessEventService(svc *model.BusinessEventService) error {
	return b.writer.CreateBusinessEventService(svc)
}
func (b *MprBackend) UpdateBusinessEventService(svc *model.BusinessEventService) error {
	return b.writer.UpdateBusinessEventService(svc)
}
func (b *MprBackend) DeleteBusinessEventService(id model.ID) error {
	return b.writer.DeleteBusinessEventService(id)
}

func (b *MprBackend) ListDatabaseConnections() ([]*model.DatabaseConnection, error) {
	return b.reader.ListDatabaseConnections()
}
func (b *MprBackend) CreateDatabaseConnection(conn *model.DatabaseConnection) error {
	return b.writer.CreateDatabaseConnection(conn)
}
func (b *MprBackend) UpdateDatabaseConnection(conn *model.DatabaseConnection) error {
	return b.writer.UpdateDatabaseConnection(conn)
}
func (b *MprBackend) MoveDatabaseConnection(conn *model.DatabaseConnection) error {
	return b.writer.MoveDatabaseConnection(conn)
}
func (b *MprBackend) DeleteDatabaseConnection(id model.ID) error {
	return b.writer.DeleteDatabaseConnection(id)
}

func (b *MprBackend) ListDataTransformers() ([]*model.DataTransformer, error) {
	return b.reader.ListDataTransformers()
}
func (b *MprBackend) CreateDataTransformer(dt *model.DataTransformer) error {
	return b.writer.CreateDataTransformer(dt)
}
func (b *MprBackend) DeleteDataTransformer(id model.ID) error {
	return b.writer.DeleteDataTransformer(id)
}

// ---------------------------------------------------------------------------
// MappingBackend
// ---------------------------------------------------------------------------

func (b *MprBackend) ListImportMappings() ([]*model.ImportMapping, error) {
	return b.reader.ListImportMappings()
}
func (b *MprBackend) GetImportMappingByQualifiedName(moduleName, name string) (*model.ImportMapping, error) {
	return b.reader.GetImportMappingByQualifiedName(moduleName, name)
}
func (b *MprBackend) CreateImportMapping(im *model.ImportMapping) error {
	return b.writer.CreateImportMapping(im)
}
func (b *MprBackend) UpdateImportMapping(im *model.ImportMapping) error {
	return b.writer.UpdateImportMapping(im)
}
func (b *MprBackend) DeleteImportMapping(id model.ID) error {
	return b.writer.DeleteImportMapping(id)
}
func (b *MprBackend) MoveImportMapping(im *model.ImportMapping) error {
	return b.writer.MoveImportMapping(im)
}

func (b *MprBackend) ListExportMappings() ([]*model.ExportMapping, error) {
	return b.reader.ListExportMappings()
}
func (b *MprBackend) GetExportMappingByQualifiedName(moduleName, name string) (*model.ExportMapping, error) {
	return b.reader.GetExportMappingByQualifiedName(moduleName, name)
}
func (b *MprBackend) CreateExportMapping(em *model.ExportMapping) error {
	return b.writer.CreateExportMapping(em)
}
func (b *MprBackend) UpdateExportMapping(em *model.ExportMapping) error {
	return b.writer.UpdateExportMapping(em)
}
func (b *MprBackend) DeleteExportMapping(id model.ID) error {
	return b.writer.DeleteExportMapping(id)
}
func (b *MprBackend) MoveExportMapping(em *model.ExportMapping) error {
	return b.writer.MoveExportMapping(em)
}

func (b *MprBackend) ListJsonStructures() ([]*types.JsonStructure, error) {
	return convertJsonStructureSlice(b.reader.ListJsonStructures())
}
func (b *MprBackend) GetJsonStructureByQualifiedName(moduleName, name string) (*types.JsonStructure, error) {
	return convertJsonStructurePtr(b.reader.GetJsonStructureByQualifiedName(moduleName, name))
}
func (b *MprBackend) CreateJsonStructure(js *types.JsonStructure) error {
	return b.writer.CreateJsonStructure(unconvertJsonStructure(js))
}
func (b *MprBackend) DeleteJsonStructure(id string) error {
	return b.writer.DeleteJsonStructure(id)
}

// ---------------------------------------------------------------------------
// JavaBackend
// ---------------------------------------------------------------------------

func (b *MprBackend) ListJavaActions() ([]*types.JavaAction, error) {
	return convertJavaActionSlice(b.reader.ListJavaActions())
}
func (b *MprBackend) ListJavaActionsFull() ([]*javaactions.JavaAction, error) {
	return b.reader.ListJavaActionsFull()
}
func (b *MprBackend) ListJavaScriptActions() ([]*types.JavaScriptAction, error) {
	return convertJavaScriptActionSlice(b.reader.ListJavaScriptActions())
}
func (b *MprBackend) ReadJavaActionByName(qualifiedName string) (*javaactions.JavaAction, error) {
	return b.reader.ReadJavaActionByName(qualifiedName)
}
func (b *MprBackend) ReadJavaScriptActionByName(qualifiedName string) (*types.JavaScriptAction, error) {
	return convertJavaScriptActionPtr(b.reader.ReadJavaScriptActionByName(qualifiedName))
}
func (b *MprBackend) CreateJavaAction(ja *javaactions.JavaAction) error {
	return b.writer.CreateJavaAction(ja)
}
func (b *MprBackend) UpdateJavaAction(ja *javaactions.JavaAction) error {
	return b.writer.UpdateJavaAction(ja)
}
func (b *MprBackend) DeleteJavaAction(id model.ID) error {
	return b.writer.DeleteJavaAction(id)
}
func (b *MprBackend) WriteJavaSourceFile(moduleName, actionName string, javaCode string, params []*javaactions.JavaActionParameter, returnType javaactions.CodeActionReturnType) error {
	return b.writer.WriteJavaSourceFile(moduleName, actionName, javaCode, params, returnType)
}

// ReadJavaSourceFile delegates to writer because the mpr SDK places this
// read operation on Writer (it needs write-transaction access to the
// contents directory).
func (b *MprBackend) ReadJavaSourceFile(moduleName, actionName string) (string, error) {
	return b.writer.ReadJavaSourceFile(moduleName, actionName)
}

// ---------------------------------------------------------------------------
// WorkflowBackend
// ---------------------------------------------------------------------------

func (b *MprBackend) ListWorkflows() ([]*workflows.Workflow, error) {
	return b.reader.ListWorkflows()
}
func (b *MprBackend) GetWorkflow(id model.ID) (*workflows.Workflow, error) {
	return b.reader.GetWorkflow(id)
}
func (b *MprBackend) CreateWorkflow(wf *workflows.Workflow) error {
	return b.writer.CreateWorkflow(wf)
}
func (b *MprBackend) DeleteWorkflow(id model.ID) error { return b.writer.DeleteWorkflow(id) }

// ---------------------------------------------------------------------------
// SettingsBackend
// ---------------------------------------------------------------------------

func (b *MprBackend) GetProjectSettings() (*model.ProjectSettings, error) {
	return b.reader.GetProjectSettings()
}
func (b *MprBackend) UpdateProjectSettings(ps *model.ProjectSettings) error {
	return b.writer.UpdateProjectSettings(ps)
}

// ---------------------------------------------------------------------------
// ImageBackend
// ---------------------------------------------------------------------------

func (b *MprBackend) ListImageCollections() ([]*types.ImageCollection, error) {
	return convertImageCollectionSlice(b.reader.ListImageCollections())
}
func (b *MprBackend) CreateImageCollection(ic *types.ImageCollection) error {
	return b.writer.CreateImageCollection(unconvertImageCollection(ic))
}
func (b *MprBackend) DeleteImageCollection(id string) error {
	return b.writer.DeleteImageCollection(id)
}

// ---------------------------------------------------------------------------
// ScheduledEventBackend
// ---------------------------------------------------------------------------

func (b *MprBackend) ListScheduledEvents() ([]*model.ScheduledEvent, error) {
	return b.reader.ListScheduledEvents()
}
func (b *MprBackend) GetScheduledEvent(id model.ID) (*model.ScheduledEvent, error) {
	return b.reader.GetScheduledEvent(id)
}

// ---------------------------------------------------------------------------
// RenameBackend
// ---------------------------------------------------------------------------

func (b *MprBackend) UpdateQualifiedNameInAllUnits(oldName, newName string) (int, error) {
	return b.writer.UpdateQualifiedNameInAllUnits(oldName, newName)
}
func (b *MprBackend) RenameReferences(oldName, newName string, dryRun bool) ([]types.RenameHit, error) {
	return convertRenameHitSlice(b.writer.RenameReferences(oldName, newName, dryRun))
}
func (b *MprBackend) RenameDocumentByName(moduleName, oldName, newName string) error {
	return b.writer.RenameDocumentByName(moduleName, oldName, newName)
}

// ---------------------------------------------------------------------------
// RawUnitBackend
// ---------------------------------------------------------------------------

func (b *MprBackend) GetRawUnit(id model.ID) (map[string]any, error) {
	return b.reader.GetRawUnit(id)
}
func (b *MprBackend) GetRawUnitBytes(id model.ID) ([]byte, error) {
	return b.reader.GetRawUnitBytes(id)
}
func (b *MprBackend) ListRawUnitsByType(typePrefix string) ([]*types.RawUnit, error) {
	return convertRawUnitSlice(b.reader.ListRawUnitsByType(typePrefix))
}
func (b *MprBackend) ListRawUnits(objectType string) ([]*types.RawUnitInfo, error) {
	return convertRawUnitInfoSlice(b.reader.ListRawUnits(objectType))
}
func (b *MprBackend) GetRawUnitByName(objectType, qualifiedName string) (*types.RawUnitInfo, error) {
	return convertRawUnitInfoPtr(b.reader.GetRawUnitByName(objectType, qualifiedName))
}
func (b *MprBackend) GetRawMicroflowByName(qualifiedName string) ([]byte, error) {
	return b.reader.GetRawMicroflowByName(qualifiedName)
}
func (b *MprBackend) UpdateRawUnit(unitID string, contents []byte) error {
	return b.writer.UpdateRawUnit(unitID, contents)
}

// ---------------------------------------------------------------------------
// MetadataBackend
// ---------------------------------------------------------------------------

func (b *MprBackend) ListAllUnitIDs() ([]string, error)     { return b.reader.ListAllUnitIDs() }
func (b *MprBackend) ListUnits() ([]*types.UnitInfo, error)   { return convertUnitInfoSlice(b.reader.ListUnits()) }
func (b *MprBackend) GetUnitTypes() (map[string]int, error) { return b.reader.GetUnitTypes() }
func (b *MprBackend) GetProjectRootID() (string, error)     { return b.reader.GetProjectRootID() }
func (b *MprBackend) ContentsDir() string                   { return b.reader.ContentsDir() }
func (b *MprBackend) ExportJSON() ([]byte, error)           { return b.reader.ExportJSON() }
func (b *MprBackend) InvalidateCache()                      { b.reader.InvalidateCache() }

// ---------------------------------------------------------------------------
// WidgetBackend
// ---------------------------------------------------------------------------

func (b *MprBackend) FindCustomWidgetType(widgetID string) (*types.RawCustomWidgetType, error) {
	return convertRawCustomWidgetTypePtr(b.reader.FindCustomWidgetType(widgetID))
}
func (b *MprBackend) FindAllCustomWidgetTypes(widgetID string) ([]*types.RawCustomWidgetType, error) {
	return convertRawCustomWidgetTypeSlice(b.reader.FindAllCustomWidgetTypes(widgetID))
}

// ---------------------------------------------------------------------------
// AgentEditorBackend
// ---------------------------------------------------------------------------

func (b *MprBackend) ListAgentEditorModels() ([]*agenteditor.Model, error) {
	return b.reader.ListAgentEditorModels()
}
func (b *MprBackend) ListAgentEditorKnowledgeBases() ([]*agenteditor.KnowledgeBase, error) {
	return b.reader.ListAgentEditorKnowledgeBases()
}
func (b *MprBackend) ListAgentEditorConsumedMCPServices() ([]*agenteditor.ConsumedMCPService, error) {
	return b.reader.ListAgentEditorConsumedMCPServices()
}
func (b *MprBackend) ListAgentEditorAgents() ([]*agenteditor.Agent, error) {
	return b.reader.ListAgentEditorAgents()
}
func (b *MprBackend) CreateAgentEditorModel(m *agenteditor.Model) error {
	return b.writer.CreateAgentEditorModel(m)
}
func (b *MprBackend) DeleteAgentEditorModel(id string) error {
	return b.writer.DeleteAgentEditorModel(id)
}
func (b *MprBackend) CreateAgentEditorKnowledgeBase(k *agenteditor.KnowledgeBase) error {
	return b.writer.CreateAgentEditorKnowledgeBase(k)
}
func (b *MprBackend) DeleteAgentEditorKnowledgeBase(id string) error {
	return b.writer.DeleteAgentEditorKnowledgeBase(id)
}
func (b *MprBackend) CreateAgentEditorConsumedMCPService(c *agenteditor.ConsumedMCPService) error {
	return b.writer.CreateAgentEditorConsumedMCPService(c)
}
func (b *MprBackend) DeleteAgentEditorConsumedMCPService(id string) error {
	return b.writer.DeleteAgentEditorConsumedMCPService(id)
}
func (b *MprBackend) CreateAgentEditorAgent(a *agenteditor.Agent) error {
	return b.writer.CreateAgentEditorAgent(a)
}
func (b *MprBackend) DeleteAgentEditorAgent(id string) error {
	return b.writer.DeleteAgentEditorAgent(id)
}

// ---------------------------------------------------------------------------
// PageMutationBackend

func (b *MprBackend) OpenPageForMutation(unitID model.ID) (backend.PageMutator, error) {
	panic("MprBackend.OpenPageForMutation not yet implemented")
}

// ---------------------------------------------------------------------------
// WorkflowMutationBackend

func (b *MprBackend) OpenWorkflowForMutation(unitID model.ID) (backend.WorkflowMutator, error) {
	panic("MprBackend.OpenWorkflowForMutation not yet implemented")
}

// ---------------------------------------------------------------------------
// WidgetSerializationBackend

func (b *MprBackend) SerializeWidget(w pages.Widget) (any, error) {
	panic("MprBackend.SerializeWidget not yet implemented")
}

func (b *MprBackend) SerializeClientAction(a pages.ClientAction) (any, error) {
	panic("MprBackend.SerializeClientAction not yet implemented")
}

func (b *MprBackend) SerializeDataSource(ds pages.DataSource) (any, error) {
	panic("MprBackend.SerializeDataSource not yet implemented")
}

func (b *MprBackend) SerializeWorkflowActivity(a workflows.WorkflowActivity) (any, error) {
	panic("MprBackend.SerializeWorkflowActivity not yet implemented")
}
