// SPDX-License-Identifier: Apache-2.0

// Package mock provides a configurable MockBackend for testing that
// implements backend.FullBackend. Each method delegates to an optional
// function field; when the field is nil the method returns zero values.
package mock

import (
	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/agenteditor"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
	"github.com/mendixlabs/mxcli/sdk/javaactions"
	"github.com/mendixlabs/mxcli/sdk/microflows"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/sdk/pages"
	"github.com/mendixlabs/mxcli/sdk/security"
	"github.com/mendixlabs/mxcli/sdk/workflows"
)

var _ backend.FullBackend = (*MockBackend)(nil)

// MockBackend implements backend.FullBackend. Every interface method is
// backed by a public function field. If the field is nil the method
// returns zero values / nil error (never panics).
type MockBackend struct {
	// ConnectionBackend
	ConnectFunc          func(path string) error
	DisconnectFunc       func() error
	CommitFunc           func() error
	IsConnectedFunc      func() bool
	PathFunc             func() string
	VersionFunc          func() types.MPRVersion
	ProjectVersionFunc   func() *types.ProjectVersion
	GetMendixVersionFunc func() (string, error)

	// ModuleBackend
	ListModulesFunc             func() ([]*model.Module, error)
	GetModuleFunc               func(id model.ID) (*model.Module, error)
	GetModuleByNameFunc         func(name string) (*model.Module, error)
	CreateModuleFunc            func(module *model.Module) error
	UpdateModuleFunc            func(module *model.Module) error
	DeleteModuleFunc            func(id model.ID) error
	DeleteModuleWithCleanupFunc func(id model.ID, moduleName string) error

	// FolderBackend
	ListFoldersFunc  func() ([]*types.FolderInfo, error)
	CreateFolderFunc func(folder *model.Folder) error
	DeleteFolderFunc func(id model.ID) error
	MoveFolderFunc   func(id model.ID, newContainerID model.ID) error

	// DomainModelBackend
	ListDomainModelsFunc                       func() ([]*domainmodel.DomainModel, error)
	GetDomainModelFunc                         func(moduleID model.ID) (*domainmodel.DomainModel, error)
	GetDomainModelByIDFunc                     func(id model.ID) (*domainmodel.DomainModel, error)
	UpdateDomainModelFunc                      func(dm *domainmodel.DomainModel) error
	CreateEntityFunc                           func(domainModelID model.ID, entity *domainmodel.Entity) error
	UpdateEntityFunc                           func(domainModelID model.ID, entity *domainmodel.Entity) error
	DeleteEntityFunc                           func(domainModelID model.ID, entityID model.ID) error
	MoveEntityFunc                             func(entity *domainmodel.Entity, sourceDMID, targetDMID model.ID, sourceModuleName, targetModuleName string) ([]string, error)
	AddAttributeFunc                           func(domainModelID model.ID, entityID model.ID, attr *domainmodel.Attribute) error
	UpdateAttributeFunc                        func(domainModelID model.ID, entityID model.ID, attr *domainmodel.Attribute) error
	DeleteAttributeFunc                        func(domainModelID model.ID, entityID model.ID, attrID model.ID) error
	CreateAssociationFunc                      func(domainModelID model.ID, assoc *domainmodel.Association) error
	CreateCrossAssociationFunc                 func(domainModelID model.ID, ca *domainmodel.CrossModuleAssociation) error
	DeleteAssociationFunc                      func(domainModelID model.ID, assocID model.ID) error
	DeleteCrossAssociationFunc                 func(domainModelID model.ID, assocID model.ID) error
	CreateViewEntitySourceDocumentFunc         func(moduleID model.ID, moduleName, docName, oqlQuery, documentation string) (model.ID, error)
	DeleteViewEntitySourceDocumentFunc         func(id model.ID) error
	DeleteViewEntitySourceDocumentByNameFunc   func(moduleName, docName string) error
	FindViewEntitySourceDocumentIDFunc         func(moduleName, docName string) (model.ID, error)
	FindAllViewEntitySourceDocumentIDsFunc     func(moduleName, docName string) ([]model.ID, error)
	MoveViewEntitySourceDocumentFunc           func(sourceModuleName string, targetModuleID model.ID, docName string) error
	UpdateOqlQueriesForMovedEntityFunc         func(oldQualifiedName, newQualifiedName string) (int, error)
	UpdateEnumerationRefsInAllDomainModelsFunc func(oldQualifiedName, newQualifiedName string) error

	// MicroflowBackend
	ListMicroflowsFunc  func() ([]*microflows.Microflow, error)
	GetMicroflowFunc    func(id model.ID) (*microflows.Microflow, error)
	CreateMicroflowFunc func(mf *microflows.Microflow) error
	UpdateMicroflowFunc func(mf *microflows.Microflow) error
	DeleteMicroflowFunc func(id model.ID) error
	MoveMicroflowFunc          func(mf *microflows.Microflow) error
	ParseMicroflowFromRawFunc  func(raw map[string]any, unitID, containerID model.ID) *microflows.Microflow
	ListNanoflowsFunc   func() ([]*microflows.Nanoflow, error)
	GetNanoflowFunc     func(id model.ID) (*microflows.Nanoflow, error)
	CreateNanoflowFunc  func(nf *microflows.Nanoflow) error
	UpdateNanoflowFunc  func(nf *microflows.Nanoflow) error
	DeleteNanoflowFunc  func(id model.ID) error
	MoveNanoflowFunc    func(nf *microflows.Nanoflow) error

	// PageBackend
	ListPagesFunc          func() ([]*pages.Page, error)
	GetPageFunc            func(id model.ID) (*pages.Page, error)
	CreatePageFunc         func(page *pages.Page) error
	UpdatePageFunc         func(page *pages.Page) error
	DeletePageFunc         func(id model.ID) error
	MovePageFunc           func(page *pages.Page) error
	ListLayoutsFunc        func() ([]*pages.Layout, error)
	GetLayoutFunc          func(id model.ID) (*pages.Layout, error)
	CreateLayoutFunc       func(layout *pages.Layout) error
	UpdateLayoutFunc       func(layout *pages.Layout) error
	DeleteLayoutFunc       func(id model.ID) error
	ListSnippetsFunc       func() ([]*pages.Snippet, error)
	CreateSnippetFunc      func(snippet *pages.Snippet) error
	UpdateSnippetFunc      func(snippet *pages.Snippet) error
	DeleteSnippetFunc      func(id model.ID) error
	MoveSnippetFunc        func(snippet *pages.Snippet) error
	ListBuildingBlocksFunc func() ([]*pages.BuildingBlock, error)
	ListPageTemplatesFunc  func() ([]*pages.PageTemplate, error)

	// EnumerationBackend
	ListEnumerationsFunc  func() ([]*model.Enumeration, error)
	GetEnumerationFunc    func(id model.ID) (*model.Enumeration, error)
	CreateEnumerationFunc func(enum *model.Enumeration) error
	UpdateEnumerationFunc func(enum *model.Enumeration) error
	MoveEnumerationFunc   func(enum *model.Enumeration) error
	DeleteEnumerationFunc func(id model.ID) error

	// ConstantBackend
	ListConstantsFunc  func() ([]*model.Constant, error)
	GetConstantFunc    func(id model.ID) (*model.Constant, error)
	CreateConstantFunc func(constant *model.Constant) error
	UpdateConstantFunc func(constant *model.Constant) error
	MoveConstantFunc   func(constant *model.Constant) error
	DeleteConstantFunc func(id model.ID) error

	// SecurityBackend
	GetProjectSecurityFunc               func() (*security.ProjectSecurity, error)
	SetProjectSecurityLevelFunc          func(unitID model.ID, level string) error
	SetProjectDemoUsersEnabledFunc       func(unitID model.ID, enabled bool) error
	AddUserRoleFunc                      func(unitID model.ID, name string, moduleRoles []string, manageAllRoles bool) error
	AlterUserRoleModuleRolesFunc         func(unitID model.ID, userRoleName string, add bool, moduleRoles []string) error
	RemoveUserRoleFunc                   func(unitID model.ID, name string) error
	AddDemoUserFunc                      func(unitID model.ID, userName, password, entity string, userRoles []string) error
	RemoveDemoUserFunc                   func(unitID model.ID, userName string) error
	ListModuleSecurityFunc               func() ([]*security.ModuleSecurity, error)
	GetModuleSecurityFunc                func(moduleID model.ID) (*security.ModuleSecurity, error)
	AddModuleRoleFunc                    func(unitID model.ID, roleName, description string) error
	RemoveModuleRoleFunc                 func(unitID model.ID, roleName string) error
	RemoveModuleRoleFromAllUserRolesFunc func(unitID model.ID, qualifiedRole string) (int, error)
	UpdateAllowedRolesFunc               func(unitID model.ID, roles []string) error
	UpdatePublishedRestServiceRolesFunc  func(unitID model.ID, roles []string) error
	RemoveFromAllowedRolesFunc           func(unitID model.ID, roleName string) (bool, error)
	AddEntityAccessRuleFunc              func(params backend.EntityAccessRuleParams) error
	RemoveEntityAccessRuleFunc           func(unitID model.ID, entityName string, roleNames []string) (int, error)
	RevokeEntityMemberAccessFunc         func(unitID model.ID, entityName string, roleNames []string, revocation types.EntityAccessRevocation) (int, error)
	RemoveRoleFromAllEntitiesFunc        func(unitID model.ID, roleName string) (int, error)
	ReconcileMemberAccessesFunc          func(unitID model.ID, moduleName string) (int, error)

	// NavigationBackend
	ListNavigationDocumentsFunc func() ([]*types.NavigationDocument, error)
	GetNavigationFunc           func() (*types.NavigationDocument, error)
	UpdateNavigationProfileFunc func(navDocID model.ID, profileName string, spec types.NavigationProfileSpec) error

	// ServiceBackend
	ListConsumedODataServicesFunc   func() ([]*model.ConsumedODataService, error)
	ListPublishedODataServicesFunc  func() ([]*model.PublishedODataService, error)
	CreateConsumedODataServiceFunc  func(svc *model.ConsumedODataService) error
	UpdateConsumedODataServiceFunc  func(svc *model.ConsumedODataService) error
	DeleteConsumedODataServiceFunc  func(id model.ID) error
	CreatePublishedODataServiceFunc func(svc *model.PublishedODataService) error
	UpdatePublishedODataServiceFunc func(svc *model.PublishedODataService) error
	DeletePublishedODataServiceFunc func(id model.ID) error
	ListConsumedRestServicesFunc    func() ([]*model.ConsumedRestService, error)
	ListPublishedRestServicesFunc   func() ([]*model.PublishedRestService, error)
	CreateConsumedRestServiceFunc   func(svc *model.ConsumedRestService) error
	UpdateConsumedRestServiceFunc   func(svc *model.ConsumedRestService) error
	DeleteConsumedRestServiceFunc   func(id model.ID) error
	CreatePublishedRestServiceFunc  func(svc *model.PublishedRestService) error
	UpdatePublishedRestServiceFunc  func(svc *model.PublishedRestService) error
	DeletePublishedRestServiceFunc  func(id model.ID) error
	ListBusinessEventServicesFunc   func() ([]*model.BusinessEventService, error)
	CreateBusinessEventServiceFunc  func(svc *model.BusinessEventService) error
	UpdateBusinessEventServiceFunc  func(svc *model.BusinessEventService) error
	DeleteBusinessEventServiceFunc  func(id model.ID) error
	ListDatabaseConnectionsFunc     func() ([]*model.DatabaseConnection, error)
	CreateDatabaseConnectionFunc    func(conn *model.DatabaseConnection) error
	UpdateDatabaseConnectionFunc    func(conn *model.DatabaseConnection) error
	MoveDatabaseConnectionFunc      func(conn *model.DatabaseConnection) error
	DeleteDatabaseConnectionFunc    func(id model.ID) error
	ListDataTransformersFunc        func() ([]*model.DataTransformer, error)
	CreateDataTransformerFunc       func(dt *model.DataTransformer) error
	DeleteDataTransformerFunc       func(id model.ID) error

	// MappingBackend
	ListImportMappingsFunc              func() ([]*model.ImportMapping, error)
	GetImportMappingByQualifiedNameFunc func(moduleName, name string) (*model.ImportMapping, error)
	CreateImportMappingFunc             func(im *model.ImportMapping) error
	UpdateImportMappingFunc             func(im *model.ImportMapping) error
	DeleteImportMappingFunc             func(id model.ID) error
	MoveImportMappingFunc               func(im *model.ImportMapping) error
	ListExportMappingsFunc              func() ([]*model.ExportMapping, error)
	GetExportMappingByQualifiedNameFunc func(moduleName, name string) (*model.ExportMapping, error)
	CreateExportMappingFunc             func(em *model.ExportMapping) error
	UpdateExportMappingFunc             func(em *model.ExportMapping) error
	DeleteExportMappingFunc             func(id model.ID) error
	MoveExportMappingFunc               func(em *model.ExportMapping) error
	ListJsonStructuresFunc              func() ([]*types.JsonStructure, error)
	GetJsonStructureByQualifiedNameFunc func(moduleName, name string) (*types.JsonStructure, error)
	CreateJsonStructureFunc             func(js *types.JsonStructure) error
	DeleteJsonStructureFunc             func(id string) error

	// JavaBackend
	ListJavaActionsFunc            func() ([]*types.JavaAction, error)
	ListJavaActionsFullFunc        func() ([]*javaactions.JavaAction, error)
	ListJavaScriptActionsFunc      func() ([]*types.JavaScriptAction, error)
	ReadJavaActionByNameFunc       func(qualifiedName string) (*javaactions.JavaAction, error)
	ReadJavaScriptActionByNameFunc func(qualifiedName string) (*types.JavaScriptAction, error)
	CreateJavaActionFunc           func(ja *javaactions.JavaAction) error
	UpdateJavaActionFunc           func(ja *javaactions.JavaAction) error
	DeleteJavaActionFunc           func(id model.ID) error
	WriteJavaSourceFileFunc        func(moduleName, actionName string, javaCode string, params []*javaactions.JavaActionParameter, returnType javaactions.CodeActionReturnType) error
	ReadJavaSourceFileFunc         func(moduleName, actionName string) (string, error)

	// WorkflowBackend
	ListWorkflowsFunc  func() ([]*workflows.Workflow, error)
	GetWorkflowFunc    func(id model.ID) (*workflows.Workflow, error)
	CreateWorkflowFunc func(wf *workflows.Workflow) error
	DeleteWorkflowFunc func(id model.ID) error

	// SettingsBackend
	GetProjectSettingsFunc    func() (*model.ProjectSettings, error)
	UpdateProjectSettingsFunc func(ps *model.ProjectSettings) error

	// ImageBackend
	ListImageCollectionsFunc  func() ([]*types.ImageCollection, error)
	CreateImageCollectionFunc func(ic *types.ImageCollection) error
	DeleteImageCollectionFunc func(id string) error

	// ScheduledEventBackend
	ListScheduledEventsFunc func() ([]*model.ScheduledEvent, error)
	GetScheduledEventFunc   func(id model.ID) (*model.ScheduledEvent, error)

	// RenameBackend
	UpdateQualifiedNameInAllUnitsFunc func(oldName, newName string) (int, error)
	RenameReferencesFunc              func(oldName, newName string, dryRun bool) ([]types.RenameHit, error)
	RenameDocumentByNameFunc          func(moduleName, oldName, newName string) error

	// RawUnitBackend
	GetRawUnitFunc            func(id model.ID) (map[string]any, error)
	GetRawUnitBytesFunc       func(id model.ID) ([]byte, error)
	ListRawUnitsByTypeFunc    func(typePrefix string) ([]*types.RawUnit, error)
	ListRawUnitsFunc          func(objectType string) ([]*types.RawUnitInfo, error)
	GetRawUnitByNameFunc      func(objectType, qualifiedName string) (*types.RawUnitInfo, error)
	GetRawMicroflowByNameFunc func(qualifiedName string) ([]byte, error)
	UpdateRawUnitFunc         func(unitID string, contents []byte) error

	// MetadataBackend
	ListAllUnitIDsFunc   func() ([]string, error)
	ListUnitsFunc        func() ([]*types.UnitInfo, error)
	GetUnitTypesFunc     func() (map[string]int, error)
	GetProjectRootIDFunc func() (string, error)
	ContentsDirFunc      func() string
	ExportJSONFunc       func() ([]byte, error)
	InvalidateCacheFunc  func()

	// WidgetBackend
	FindCustomWidgetTypeFunc     func(widgetID string) (*types.RawCustomWidgetType, error)
	FindAllCustomWidgetTypesFunc func(widgetID string) ([]*types.RawCustomWidgetType, error)

	// PageMutationBackend
	OpenPageForMutationFunc func(unitID model.ID) (backend.PageMutator, error)

	// WorkflowMutationBackend
	OpenWorkflowForMutationFunc func(unitID model.ID) (backend.WorkflowMutator, error)

	// WidgetSerializationBackend
	SerializeWidgetFunc           func(w pages.Widget) (any, error)
	SerializeClientActionFunc     func(a pages.ClientAction) (any, error)
	SerializeDataSourceFunc       func(ds pages.DataSource) (any, error)
	SerializeWorkflowActivityFunc func(a workflows.WorkflowActivity) (any, error)

	// WidgetBuilderBackend
	LoadWidgetTemplateFunc          func(widgetID string, projectPath string) (backend.WidgetObjectBuilder, error)
	SerializeWidgetToOpaqueFunc     func(w pages.Widget) any
	SerializeDataSourceToOpaqueFunc func(ds pages.DataSource) any
	BuildCreateAttributeObjectFunc  func(attributePath string, objectTypeID, propertyTypeID, valueTypeID string) (any, error)
	BuildDataGrid2WidgetFunc        func(id model.ID, name string, spec backend.DataGridSpec, projectPath string) (*pages.CustomWidget, error)
	BuildFilterWidgetFunc           func(spec backend.FilterWidgetSpec, projectPath string) (pages.Widget, error)

	// AgentEditorBackend
	ListAgentEditorModelsFunc               func() ([]*agenteditor.Model, error)
	ListAgentEditorKnowledgeBasesFunc       func() ([]*agenteditor.KnowledgeBase, error)
	ListAgentEditorConsumedMCPServicesFunc  func() ([]*agenteditor.ConsumedMCPService, error)
	ListAgentEditorAgentsFunc               func() ([]*agenteditor.Agent, error)
	CreateAgentEditorModelFunc              func(m *agenteditor.Model) error
	DeleteAgentEditorModelFunc              func(id string) error
	CreateAgentEditorKnowledgeBaseFunc      func(kb *agenteditor.KnowledgeBase) error
	DeleteAgentEditorKnowledgeBaseFunc      func(id string) error
	CreateAgentEditorConsumedMCPServiceFunc func(svc *agenteditor.ConsumedMCPService) error
	DeleteAgentEditorConsumedMCPServiceFunc func(id string) error
	CreateAgentEditorAgentFunc              func(a *agenteditor.Agent) error
	DeleteAgentEditorAgentFunc              func(id string) error
}
