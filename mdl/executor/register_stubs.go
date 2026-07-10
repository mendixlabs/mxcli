// SPDX-License-Identifier: Apache-2.0

package executor

import "github.com/JordtenBulte-OLC/mxcli/mdl/ast"

// Handler registration functions — each registers handlers for its domain.
// Handlers are thin wrappers around existing Executor methods. Once handlers
// are migrated to *ExecContext signatures, these wrappers will be replaced
// by direct function references.

func registerConnectionHandlers(r *Registry) {
	r.Register(&ast.ConnectStmt{}, func(ctx *ExecContext, stmt ast.Statement) error {
		return execConnect(ctx, stmt.(*ast.ConnectStmt))
	})
	r.Register(&ast.DisconnectStmt{}, func(ctx *ExecContext, stmt ast.Statement) error {
		return execDisconnect(ctx)
	})
	r.Register(&ast.StatusStmt{}, func(ctx *ExecContext, stmt ast.Statement) error {
		return execStatus(ctx)
	})
}

func registerModuleHandlers(r *Registry) {
	r.Register(&ast.CreateModuleStmt{}, func(ctx *ExecContext, stmt ast.Statement) error {
		return execCreateModule(ctx, stmt.(*ast.CreateModuleStmt))
	})
	r.Register(&ast.DropModuleStmt{}, func(ctx *ExecContext, stmt ast.Statement) error {
		return execDropModule(ctx, stmt.(*ast.DropModuleStmt))
	})
	r.Register(&ast.AlterModuleJarDepStmt{}, func(ctx *ExecContext, stmt ast.Statement) error {
		return execAlterModuleJarDep(ctx, stmt.(*ast.AlterModuleJarDepStmt))
	})
}

func registerEnumerationHandlers(r *Registry) {
	r.Register(&ast.CreateEnumerationStmt{}, func(ctx *ExecContext, stmt ast.Statement) error {
		return execCreateEnumeration(ctx, stmt.(*ast.CreateEnumerationStmt))
	})
	r.Register(&ast.AlterEnumerationStmt{}, func(ctx *ExecContext, stmt ast.Statement) error {
		return execAlterEnumeration(ctx, stmt.(*ast.AlterEnumerationStmt))
	})
	r.Register(&ast.DropEnumerationStmt{}, func(ctx *ExecContext, stmt ast.Statement) error {
		return execDropEnumeration(ctx, stmt.(*ast.DropEnumerationStmt))
	})
}

func registerConstantHandlers(r *Registry) {
	r.Register(&ast.CreateConstantStmt{}, func(ctx *ExecContext, stmt ast.Statement) error {
		return createConstant(ctx, stmt.(*ast.CreateConstantStmt))
	})
	r.Register(&ast.DropConstantStmt{}, func(ctx *ExecContext, stmt ast.Statement) error {
		return dropConstant(ctx, stmt.(*ast.DropConstantStmt))
	})
}

func registerDatabaseConnectionHandlers(r *Registry) {
	r.Register(&ast.CreateDatabaseConnectionStmt{}, func(ctx *ExecContext, stmt ast.Statement) error {
		return createDatabaseConnection(ctx, stmt.(*ast.CreateDatabaseConnectionStmt))
	})
}

func registerEntityHandlers(r *Registry) {
	r.Register(&ast.CreateEntityStmt{}, func(ctx *ExecContext, stmt ast.Statement) error {
		return execCreateEntity(ctx, stmt.(*ast.CreateEntityStmt))
	})
	r.Register(&ast.CreateViewEntityStmt{}, func(ctx *ExecContext, stmt ast.Statement) error {
		return execCreateViewEntity(ctx, stmt.(*ast.CreateViewEntityStmt))
	})
	r.Register(&ast.AlterEntityStmt{}, func(ctx *ExecContext, stmt ast.Statement) error {
		return execAlterEntity(ctx, stmt.(*ast.AlterEntityStmt))
	})
	r.Register(&ast.DropEntityStmt{}, func(ctx *ExecContext, stmt ast.Statement) error {
		return execDropEntity(ctx, stmt.(*ast.DropEntityStmt))
	})
}

func registerAssociationHandlers(r *Registry) {
	r.Register(&ast.CreateAssociationStmt{}, func(ctx *ExecContext, stmt ast.Statement) error {
		return execCreateAssociation(ctx, stmt.(*ast.CreateAssociationStmt))
	})
	r.Register(&ast.AlterAssociationStmt{}, func(ctx *ExecContext, stmt ast.Statement) error {
		return execAlterAssociation(ctx, stmt.(*ast.AlterAssociationStmt))
	})
	r.Register(&ast.DropAssociationStmt{}, func(ctx *ExecContext, stmt ast.Statement) error {
		return execDropAssociation(ctx, stmt.(*ast.DropAssociationStmt))
	})
}

func registerMicroflowAndNanoflowHandlers(r *Registry) {
	r.Register(&ast.CreateMicroflowStmt{}, func(ctx *ExecContext, stmt ast.Statement) error {
		return execCreateMicroflow(ctx, stmt.(*ast.CreateMicroflowStmt))
	})
	r.Register(&ast.DropMicroflowStmt{}, func(ctx *ExecContext, stmt ast.Statement) error {
		return execDropMicroflow(ctx, stmt.(*ast.DropMicroflowStmt))
	})
	r.Register(&ast.CreateNanoflowStmt{}, func(ctx *ExecContext, stmt ast.Statement) error {
		return execCreateNanoflow(ctx, stmt.(*ast.CreateNanoflowStmt))
	})
	r.Register(&ast.DropNanoflowStmt{}, func(ctx *ExecContext, stmt ast.Statement) error {
		return execDropNanoflow(ctx, stmt.(*ast.DropNanoflowStmt))
	})
}

func registerPageHandlers(r *Registry) {
	r.Register(&ast.CreatePageStmtV3{}, func(ctx *ExecContext, stmt ast.Statement) error {
		return execCreatePageV3(ctx, stmt.(*ast.CreatePageStmtV3))
	})
	r.Register(&ast.DropPageStmt{}, func(ctx *ExecContext, stmt ast.Statement) error {
		return execDropPage(ctx, stmt.(*ast.DropPageStmt))
	})
	r.Register(&ast.CreateSnippetStmtV3{}, func(ctx *ExecContext, stmt ast.Statement) error {
		return execCreateSnippetV3(ctx, stmt.(*ast.CreateSnippetStmtV3))
	})
	r.Register(&ast.DropSnippetStmt{}, func(ctx *ExecContext, stmt ast.Statement) error {
		return execDropSnippet(ctx, stmt.(*ast.DropSnippetStmt))
	})
	r.Register(&ast.DropJavaActionStmt{}, func(ctx *ExecContext, stmt ast.Statement) error {
		return execDropJavaAction(ctx, stmt.(*ast.DropJavaActionStmt))
	})
	r.Register(&ast.CreateJavaActionStmt{}, func(ctx *ExecContext, stmt ast.Statement) error {
		return execCreateJavaAction(ctx, stmt.(*ast.CreateJavaActionStmt))
	})
	r.Register(&ast.DropJavaScriptActionStmt{}, func(ctx *ExecContext, stmt ast.Statement) error {
		return execDropJavaScriptAction(ctx, stmt.(*ast.DropJavaScriptActionStmt))
	})
	r.Register(&ast.CreateJavaScriptActionStmt{}, func(ctx *ExecContext, stmt ast.Statement) error {
		return execCreateJavaScriptAction(ctx, stmt.(*ast.CreateJavaScriptActionStmt))
	})
	r.Register(&ast.DropFolderStmt{}, func(ctx *ExecContext, stmt ast.Statement) error {
		return execDropFolder(ctx, stmt.(*ast.DropFolderStmt))
	})
	r.Register(&ast.MoveFolderStmt{}, func(ctx *ExecContext, stmt ast.Statement) error {
		return execMoveFolder(ctx, stmt.(*ast.MoveFolderStmt))
	})
	r.Register(&ast.MoveStmt{}, func(ctx *ExecContext, stmt ast.Statement) error {
		return execMove(ctx, stmt.(*ast.MoveStmt))
	})
	r.Register(&ast.RenameStmt{}, func(ctx *ExecContext, stmt ast.Statement) error {
		return execRename(ctx, stmt.(*ast.RenameStmt))
	})
}

func registerSecurityHandlers(r *Registry) {
	r.Register(&ast.CreateModuleRoleStmt{}, func(ctx *ExecContext, stmt ast.Statement) error {
		return execCreateModuleRole(ctx, stmt.(*ast.CreateModuleRoleStmt))
	})
	r.Register(&ast.DropModuleRoleStmt{}, func(ctx *ExecContext, stmt ast.Statement) error {
		return execDropModuleRole(ctx, stmt.(*ast.DropModuleRoleStmt))
	})
	r.Register(&ast.CreateUserRoleStmt{}, func(ctx *ExecContext, stmt ast.Statement) error {
		return execCreateUserRole(ctx, stmt.(*ast.CreateUserRoleStmt))
	})
	r.Register(&ast.AlterUserRoleStmt{}, func(ctx *ExecContext, stmt ast.Statement) error {
		return execAlterUserRole(ctx, stmt.(*ast.AlterUserRoleStmt))
	})
	r.Register(&ast.DropUserRoleStmt{}, func(ctx *ExecContext, stmt ast.Statement) error {
		return execDropUserRole(ctx, stmt.(*ast.DropUserRoleStmt))
	})
	r.Register(&ast.GrantEntityAccessStmt{}, func(ctx *ExecContext, stmt ast.Statement) error {
		return execGrantEntityAccess(ctx, stmt.(*ast.GrantEntityAccessStmt))
	})
	r.Register(&ast.RevokeEntityAccessStmt{}, func(ctx *ExecContext, stmt ast.Statement) error {
		return execRevokeEntityAccess(ctx, stmt.(*ast.RevokeEntityAccessStmt))
	})
	r.Register(&ast.GrantMicroflowAccessStmt{}, func(ctx *ExecContext, stmt ast.Statement) error {
		return execGrantMicroflowAccess(ctx, stmt.(*ast.GrantMicroflowAccessStmt))
	})
	r.Register(&ast.RevokeMicroflowAccessStmt{}, func(ctx *ExecContext, stmt ast.Statement) error {
		return execRevokeMicroflowAccess(ctx, stmt.(*ast.RevokeMicroflowAccessStmt))
	})
	r.Register(&ast.GrantNanoflowAccessStmt{}, func(ctx *ExecContext, stmt ast.Statement) error {
		return execGrantNanoflowAccess(ctx, stmt.(*ast.GrantNanoflowAccessStmt))
	})
	r.Register(&ast.RevokeNanoflowAccessStmt{}, func(ctx *ExecContext, stmt ast.Statement) error {
		return execRevokeNanoflowAccess(ctx, stmt.(*ast.RevokeNanoflowAccessStmt))
	})
	r.Register(&ast.GrantPageAccessStmt{}, func(ctx *ExecContext, stmt ast.Statement) error {
		return execGrantPageAccess(ctx, stmt.(*ast.GrantPageAccessStmt))
	})
	r.Register(&ast.RevokePageAccessStmt{}, func(ctx *ExecContext, stmt ast.Statement) error {
		return execRevokePageAccess(ctx, stmt.(*ast.RevokePageAccessStmt))
	})
	r.Register(&ast.GrantWorkflowAccessStmt{}, func(ctx *ExecContext, stmt ast.Statement) error {
		return execGrantWorkflowAccess(ctx, stmt.(*ast.GrantWorkflowAccessStmt))
	})
	r.Register(&ast.RevokeWorkflowAccessStmt{}, func(ctx *ExecContext, stmt ast.Statement) error {
		return execRevokeWorkflowAccess(ctx, stmt.(*ast.RevokeWorkflowAccessStmt))
	})
	r.Register(&ast.AlterProjectSecurityStmt{}, func(ctx *ExecContext, stmt ast.Statement) error {
		return execAlterProjectSecurity(ctx, stmt.(*ast.AlterProjectSecurityStmt))
	})
	r.Register(&ast.CreateDemoUserStmt{}, func(ctx *ExecContext, stmt ast.Statement) error {
		return execCreateDemoUser(ctx, stmt.(*ast.CreateDemoUserStmt))
	})
	r.Register(&ast.DropDemoUserStmt{}, func(ctx *ExecContext, stmt ast.Statement) error {
		return execDropDemoUser(ctx, stmt.(*ast.DropDemoUserStmt))
	})
	r.Register(&ast.UpdateSecurityStmt{}, func(ctx *ExecContext, stmt ast.Statement) error {
		return execUpdateSecurity(ctx, stmt.(*ast.UpdateSecurityStmt))
	})
}

func registerNavigationHandlers(r *Registry) {
	r.Register(&ast.AlterNavigationStmt{}, func(ctx *ExecContext, stmt ast.Statement) error {
		return execAlterNavigation(ctx, stmt.(*ast.AlterNavigationStmt))
	})
}

func registerImageHandlers(r *Registry) {
	r.Register(&ast.CreateImageCollectionStmt{}, func(ctx *ExecContext, stmt ast.Statement) error {
		return execCreateImageCollection(ctx, stmt.(*ast.CreateImageCollectionStmt))
	})
	r.Register(&ast.DropImageCollectionStmt{}, func(ctx *ExecContext, stmt ast.Statement) error {
		return execDropImageCollection(ctx, stmt.(*ast.DropImageCollectionStmt))
	})
}

func registerWorkflowHandlers(r *Registry) {
	r.Register(&ast.CreateWorkflowStmt{}, func(ctx *ExecContext, stmt ast.Statement) error {
		return execCreateWorkflow(ctx, stmt.(*ast.CreateWorkflowStmt))
	})
	r.Register(&ast.DropWorkflowStmt{}, func(ctx *ExecContext, stmt ast.Statement) error {
		return execDropWorkflow(ctx, stmt.(*ast.DropWorkflowStmt))
	})
	r.Register(&ast.AlterWorkflowStmt{}, func(ctx *ExecContext, stmt ast.Statement) error {
		return execAlterWorkflow(ctx, stmt.(*ast.AlterWorkflowStmt))
	})
}

func registerBusinessEventHandlers(r *Registry) {
	r.Register(&ast.CreateBusinessEventServiceStmt{}, func(ctx *ExecContext, stmt ast.Statement) error {
		return createBusinessEventService(ctx, stmt.(*ast.CreateBusinessEventServiceStmt))
	})
	r.Register(&ast.DropBusinessEventServiceStmt{}, func(ctx *ExecContext, stmt ast.Statement) error {
		return dropBusinessEventService(ctx, stmt.(*ast.DropBusinessEventServiceStmt))
	})
}

func registerSettingsHandlers(r *Registry) {
	r.Register(&ast.AlterSettingsStmt{}, func(ctx *ExecContext, stmt ast.Statement) error {
		return alterSettings(ctx, stmt.(*ast.AlterSettingsStmt))
	})
	r.Register(&ast.CreateConfigurationStmt{}, func(ctx *ExecContext, stmt ast.Statement) error {
		return createConfiguration(ctx, stmt.(*ast.CreateConfigurationStmt))
	})
	r.Register(&ast.DropConfigurationStmt{}, func(ctx *ExecContext, stmt ast.Statement) error {
		return dropConfiguration(ctx, stmt.(*ast.DropConfigurationStmt))
	})
}

func registerODataHandlers(r *Registry) {
	r.Register(&ast.CreateODataClientStmt{}, func(ctx *ExecContext, stmt ast.Statement) error {
		return createODataClient(ctx, stmt.(*ast.CreateODataClientStmt))
	})
	r.Register(&ast.AlterODataClientStmt{}, func(ctx *ExecContext, stmt ast.Statement) error {
		return alterODataClient(ctx, stmt.(*ast.AlterODataClientStmt))
	})
	r.Register(&ast.DropODataClientStmt{}, func(ctx *ExecContext, stmt ast.Statement) error {
		return dropODataClient(ctx, stmt.(*ast.DropODataClientStmt))
	})
	r.Register(&ast.CreateODataServiceStmt{}, func(ctx *ExecContext, stmt ast.Statement) error {
		return createODataService(ctx, stmt.(*ast.CreateODataServiceStmt))
	})
	r.Register(&ast.AlterODataServiceStmt{}, func(ctx *ExecContext, stmt ast.Statement) error {
		return alterODataService(ctx, stmt.(*ast.AlterODataServiceStmt))
	})
	r.Register(&ast.DropODataServiceStmt{}, func(ctx *ExecContext, stmt ast.Statement) error {
		return dropODataService(ctx, stmt.(*ast.DropODataServiceStmt))
	})
}

func registerJSONStructureHandlers(r *Registry) {
	r.Register(&ast.CreateJsonStructureStmt{}, func(ctx *ExecContext, stmt ast.Statement) error {
		return execCreateJsonStructure(ctx, stmt.(*ast.CreateJsonStructureStmt))
	})
	r.Register(&ast.DropJsonStructureStmt{}, func(ctx *ExecContext, stmt ast.Statement) error {
		return execDropJsonStructure(ctx, stmt.(*ast.DropJsonStructureStmt))
	})
}

func registerMappingHandlers(r *Registry) {
	r.Register(&ast.CreateImportMappingStmt{}, func(ctx *ExecContext, stmt ast.Statement) error {
		return execCreateImportMapping(ctx, stmt.(*ast.CreateImportMappingStmt))
	})
	r.Register(&ast.DropImportMappingStmt{}, func(ctx *ExecContext, stmt ast.Statement) error {
		return execDropImportMapping(ctx, stmt.(*ast.DropImportMappingStmt))
	})
	r.Register(&ast.CreateExportMappingStmt{}, func(ctx *ExecContext, stmt ast.Statement) error {
		return execCreateExportMapping(ctx, stmt.(*ast.CreateExportMappingStmt))
	})
	r.Register(&ast.DropExportMappingStmt{}, func(ctx *ExecContext, stmt ast.Statement) error {
		return execDropExportMapping(ctx, stmt.(*ast.DropExportMappingStmt))
	})
}

func registerRESTHandlers(r *Registry) {
	r.Register(&ast.CreateRestClientStmt{}, func(ctx *ExecContext, stmt ast.Statement) error {
		return createRestClient(ctx, stmt.(*ast.CreateRestClientStmt))
	})
	r.Register(&ast.DropRestClientStmt{}, func(ctx *ExecContext, stmt ast.Statement) error {
		return dropRestClient(ctx, stmt.(*ast.DropRestClientStmt))
	})
	r.Register(&ast.DescribeContractFromOpenAPIStmt{}, func(ctx *ExecContext, stmt ast.Statement) error {
		return describeContractFromOpenAPI(ctx, stmt.(*ast.DescribeContractFromOpenAPIStmt))
	})
	r.Register(&ast.CreatePublishedRestServiceStmt{}, func(ctx *ExecContext, stmt ast.Statement) error {
		return execCreatePublishedRestService(ctx, stmt.(*ast.CreatePublishedRestServiceStmt))
	})
	r.Register(&ast.DropPublishedRestServiceStmt{}, func(ctx *ExecContext, stmt ast.Statement) error {
		return execDropPublishedRestService(ctx, stmt.(*ast.DropPublishedRestServiceStmt))
	})
	r.Register(&ast.AlterPublishedRestServiceStmt{}, func(ctx *ExecContext, stmt ast.Statement) error {
		return execAlterPublishedRestService(ctx, stmt.(*ast.AlterPublishedRestServiceStmt))
	})
	r.Register(&ast.CreateExternalEntityStmt{}, func(ctx *ExecContext, stmt ast.Statement) error {
		return execCreateExternalEntity(ctx, stmt.(*ast.CreateExternalEntityStmt))
	})
	r.Register(&ast.CreateExternalEntitiesStmt{}, func(ctx *ExecContext, stmt ast.Statement) error {
		return createExternalEntities(ctx, stmt.(*ast.CreateExternalEntitiesStmt))
	})
	r.Register(&ast.GrantODataServiceAccessStmt{}, func(ctx *ExecContext, stmt ast.Statement) error {
		return execGrantODataServiceAccess(ctx, stmt.(*ast.GrantODataServiceAccessStmt))
	})
	r.Register(&ast.RevokeODataServiceAccessStmt{}, func(ctx *ExecContext, stmt ast.Statement) error {
		return execRevokeODataServiceAccess(ctx, stmt.(*ast.RevokeODataServiceAccessStmt))
	})
	r.Register(&ast.GrantPublishedRestServiceAccessStmt{}, func(ctx *ExecContext, stmt ast.Statement) error {
		return execGrantPublishedRestServiceAccess(ctx, stmt.(*ast.GrantPublishedRestServiceAccessStmt))
	})
	r.Register(&ast.RevokePublishedRestServiceAccessStmt{}, func(ctx *ExecContext, stmt ast.Statement) error {
		return execRevokePublishedRestServiceAccess(ctx, stmt.(*ast.RevokePublishedRestServiceAccessStmt))
	})
}

func registerDataTransformerHandlers(r *Registry) {
	r.Register(&ast.CreateDataTransformerStmt{}, func(ctx *ExecContext, stmt ast.Statement) error {
		return execCreateDataTransformer(ctx, stmt.(*ast.CreateDataTransformerStmt))
	})
	r.Register(&ast.DropDataTransformerStmt{}, func(ctx *ExecContext, stmt ast.Statement) error {
		return execDropDataTransformer(ctx, stmt.(*ast.DropDataTransformerStmt))
	})
}

func registerQueryHandlers(r *Registry) {
	r.Register(&ast.ShowStmt{}, func(ctx *ExecContext, stmt ast.Statement) error {
		return execShow(ctx, stmt.(*ast.ShowStmt))
	})
	r.Register(&ast.ShowWidgetsStmt{}, func(ctx *ExecContext, stmt ast.Statement) error {
		return execShowWidgets(ctx, stmt.(*ast.ShowWidgetsStmt))
	})
	r.Register(&ast.UpdateWidgetsStmt{}, func(ctx *ExecContext, stmt ast.Statement) error {
		return execUpdateWidgets(ctx, stmt.(*ast.UpdateWidgetsStmt))
	})
	r.Register(&ast.SelectStmt{}, func(ctx *ExecContext, stmt ast.Statement) error {
		return execCatalogQuery(ctx, stmt.(*ast.SelectStmt).Query)
	})
	r.Register(&ast.DescribeStmt{}, func(ctx *ExecContext, stmt ast.Statement) error {
		return execDescribe(ctx, stmt.(*ast.DescribeStmt))
	})
	r.Register(&ast.DescribeCatalogTableStmt{}, func(ctx *ExecContext, stmt ast.Statement) error {
		return execDescribeCatalogTable(ctx, stmt.(*ast.DescribeCatalogTableStmt))
	})
	// NOTE: ShowFeaturesStmt was missing from the original type-switch
	// (pre-existing bug). Adding it here to fix the dead-code path.
	r.Register(&ast.ShowFeaturesStmt{}, func(ctx *ExecContext, stmt ast.Statement) error {
		return execShowFeatures(ctx, stmt.(*ast.ShowFeaturesStmt))
	})
}

func registerStylingHandlers(r *Registry) {
	r.Register(&ast.ShowDesignPropertiesStmt{}, func(ctx *ExecContext, stmt ast.Statement) error {
		return execShowDesignProperties(ctx, stmt.(*ast.ShowDesignPropertiesStmt))
	})
	r.Register(&ast.DescribeStylingStmt{}, func(ctx *ExecContext, stmt ast.Statement) error {
		return execDescribeStyling(ctx, stmt.(*ast.DescribeStylingStmt))
	})
	r.Register(&ast.AlterStylingStmt{}, func(ctx *ExecContext, stmt ast.Statement) error {
		return execAlterStyling(ctx, stmt.(*ast.AlterStylingStmt))
	})
}

func registerRepositoryHandlers(r *Registry) {
	r.Register(&ast.UpdateStmt{}, func(ctx *ExecContext, stmt ast.Statement) error {
		return execUpdate(ctx)
	})
	r.Register(&ast.RefreshStmt{}, func(ctx *ExecContext, stmt ast.Statement) error {
		return execRefresh(ctx)
	})
	r.Register(&ast.RefreshCatalogStmt{}, func(ctx *ExecContext, stmt ast.Statement) error {
		return execRefreshCatalogStmt(ctx, stmt.(*ast.RefreshCatalogStmt))
	})
	r.Register(&ast.SearchStmt{}, func(ctx *ExecContext, stmt ast.Statement) error {
		return execSearch(ctx, stmt.(*ast.SearchStmt))
	})
}

func registerSessionHandlers(r *Registry) {
	r.Register(&ast.SetStmt{}, func(ctx *ExecContext, stmt ast.Statement) error {
		return execSet(ctx, stmt.(*ast.SetStmt))
	})
	r.Register(&ast.HelpStmt{}, func(ctx *ExecContext, stmt ast.Statement) error {
		return execHelp(ctx, stmt.(*ast.HelpStmt))
	})
	r.Register(&ast.ExitStmt{}, func(ctx *ExecContext, stmt ast.Statement) error {
		return execExit(ctx)
	})
	r.Register(&ast.ExecuteScriptStmt{}, func(ctx *ExecContext, stmt ast.Statement) error {
		return execExecuteScript(ctx, stmt.(*ast.ExecuteScriptStmt))
	})
}

func registerLintHandlers(r *Registry) {
	r.Register(&ast.LintStmt{}, func(ctx *ExecContext, stmt ast.Statement) error {
		return execLint(ctx, stmt.(*ast.LintStmt))
	})
}

func registerAlterPageHandlers(r *Registry) {
	r.Register(&ast.AlterPageStmt{}, func(ctx *ExecContext, stmt ast.Statement) error {
		return execAlterPage(ctx, stmt.(*ast.AlterPageStmt))
	})
}

func registerFragmentHandlers(r *Registry) {
	r.Register(&ast.DefineFragmentStmt{}, func(ctx *ExecContext, stmt ast.Statement) error {
		return execDefineFragment(ctx, stmt.(*ast.DefineFragmentStmt))
	})
	r.Register(&ast.DescribeFragmentFromStmt{}, func(ctx *ExecContext, stmt ast.Statement) error {
		return describeFragmentFrom(ctx, stmt.(*ast.DescribeFragmentFromStmt))
	})
}

func registerSQLHandlers(r *Registry) {
	r.Register(&ast.SQLConnectStmt{}, func(ctx *ExecContext, stmt ast.Statement) error {
		return execSQLConnect(ctx, stmt.(*ast.SQLConnectStmt))
	})
	r.Register(&ast.SQLDisconnectStmt{}, func(ctx *ExecContext, stmt ast.Statement) error {
		return execSQLDisconnect(ctx, stmt.(*ast.SQLDisconnectStmt))
	})
	r.Register(&ast.SQLConnectionsStmt{}, func(ctx *ExecContext, stmt ast.Statement) error {
		return execSQLConnections(ctx)
	})
	r.Register(&ast.SQLQueryStmt{}, func(ctx *ExecContext, stmt ast.Statement) error {
		return execSQLQuery(ctx, stmt.(*ast.SQLQueryStmt))
	})
	r.Register(&ast.SQLShowTablesStmt{}, func(ctx *ExecContext, stmt ast.Statement) error {
		return execSQLShowTables(ctx, stmt.(*ast.SQLShowTablesStmt))
	})
	r.Register(&ast.SQLShowViewsStmt{}, func(ctx *ExecContext, stmt ast.Statement) error {
		return execSQLShowViews(ctx, stmt.(*ast.SQLShowViewsStmt))
	})
	r.Register(&ast.SQLShowFunctionsStmt{}, func(ctx *ExecContext, stmt ast.Statement) error {
		return execSQLShowFunctions(ctx, stmt.(*ast.SQLShowFunctionsStmt))
	})
	r.Register(&ast.SQLDescribeTableStmt{}, func(ctx *ExecContext, stmt ast.Statement) error {
		return execSQLDescribeTable(ctx, stmt.(*ast.SQLDescribeTableStmt))
	})
	r.Register(&ast.SQLGenerateConnectorStmt{}, func(ctx *ExecContext, stmt ast.Statement) error {
		return execSQLGenerateConnector(ctx, stmt.(*ast.SQLGenerateConnectorStmt))
	})
}

func registerImportHandlers(r *Registry) {
	r.Register(&ast.ImportStmt{}, func(ctx *ExecContext, stmt ast.Statement) error {
		return execImport(ctx, stmt.(*ast.ImportStmt))
	})
}

func registerAgentEditorHandlers(r *Registry) {
	r.Register(&ast.CreateModelStmt{}, func(ctx *ExecContext, stmt ast.Statement) error {
		return execCreateAgentEditorModel(ctx, stmt.(*ast.CreateModelStmt))
	})
	r.Register(&ast.DropModelStmt{}, func(ctx *ExecContext, stmt ast.Statement) error {
		return execDropAgentEditorModel(ctx, stmt.(*ast.DropModelStmt))
	})
	r.Register(&ast.CreateConsumedMCPServiceStmt{}, func(ctx *ExecContext, stmt ast.Statement) error {
		return execCreateConsumedMCPService(ctx, stmt.(*ast.CreateConsumedMCPServiceStmt))
	})
	r.Register(&ast.DropConsumedMCPServiceStmt{}, func(ctx *ExecContext, stmt ast.Statement) error {
		return execDropConsumedMCPService(ctx, stmt.(*ast.DropConsumedMCPServiceStmt))
	})
	r.Register(&ast.CreateKnowledgeBaseStmt{}, func(ctx *ExecContext, stmt ast.Statement) error {
		return execCreateKnowledgeBase(ctx, stmt.(*ast.CreateKnowledgeBaseStmt))
	})
	r.Register(&ast.DropKnowledgeBaseStmt{}, func(ctx *ExecContext, stmt ast.Statement) error {
		return execDropKnowledgeBase(ctx, stmt.(*ast.DropKnowledgeBaseStmt))
	})
	r.Register(&ast.CreateAgentStmt{}, func(ctx *ExecContext, stmt ast.Statement) error {
		return execCreateAgent(ctx, stmt.(*ast.CreateAgentStmt))
	})
	r.Register(&ast.DropAgentStmt{}, func(ctx *ExecContext, stmt ast.Statement) error {
		return execDropAgent(ctx, stmt.(*ast.DropAgentStmt))
	})
	r.Register(&ast.AlterModelStmt{}, func(ctx *ExecContext, stmt ast.Statement) error {
		return execAlterModel(ctx, stmt.(*ast.AlterModelStmt))
	})
	r.Register(&ast.AlterKnowledgeBaseStmt{}, func(ctx *ExecContext, stmt ast.Statement) error {
		return execAlterKnowledgeBase(ctx, stmt.(*ast.AlterKnowledgeBaseStmt))
	})
	r.Register(&ast.AlterConsumedMCPServiceStmt{}, func(ctx *ExecContext, stmt ast.Statement) error {
		return execAlterConsumedMCPService(ctx, stmt.(*ast.AlterConsumedMCPServiceStmt))
	})
	r.Register(&ast.AlterAgentStmt{}, func(ctx *ExecContext, stmt ast.Statement) error {
		return execAlterAgent(ctx, stmt.(*ast.AlterAgentStmt))
	})
}
