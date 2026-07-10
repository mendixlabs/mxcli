// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
	mdlerrors "github.com/JordtenBulte-OLC/mxcli/mdl/errors"
)

func execShow(ctx *ExecContext, s *ast.ShowStmt) error {
	if !ctx.Connected() && s.ObjectType != ast.ShowModules && s.ObjectType != ast.ShowFragments {
		return mdlerrors.NewNotConnected()
	}

	switch s.ObjectType {
	case ast.ShowModules:
		return listModules(ctx)
	case ast.ShowEnumerations:
		return listEnumerations(ctx, s.InModule)
	case ast.ShowConstants:
		return listConstants(ctx, s.InModule)
	case ast.ShowConstantValues:
		return listConstantValues(ctx, s.InModule)
	case ast.ShowEntities:
		return listEntities(ctx, s.InModule)
	case ast.ShowEntity:
		return listEntity(ctx, s.Name)
	case ast.ShowAssociations:
		return listAssociations(ctx, s.InModule)
	case ast.ShowAssociation:
		return listAssociation(ctx, s.Name)
	case ast.ShowMicroflows:
		return listMicroflows(ctx, s.InModule)
	case ast.ShowNanoflows:
		return listNanoflows(ctx, s.InModule)
	case ast.ShowPages:
		return listPages(ctx, s.InModule)
	case ast.ShowSnippets:
		return listSnippets(ctx, s.InModule)
	case ast.ShowLayouts:
		return listLayouts(ctx, s.InModule)
	case ast.ShowJavaActions:
		return listJavaActions(ctx, s.InModule)
	case ast.ShowJavaScriptActions:
		return listJavaScriptActions(ctx, s.InModule)
	case ast.ShowVersion:
		return listVersion(ctx)
	case ast.ShowCatalogTables:
		return execShowCatalogTables(ctx)
	case ast.ShowCatalogStatus:
		return execShowCatalogStatus(ctx)
	case ast.ShowCallers:
		return execShowCallers(ctx, s)
	case ast.ShowCallees:
		return execShowCallees(ctx, s)
	case ast.ShowReferences:
		return execShowReferences(ctx, s)
	case ast.ShowImpact:
		return execShowImpact(ctx, s)
	case ast.ShowContext:
		return execShowContext(ctx, s)
	case ast.ShowCommunities:
		return execShowCommunities(ctx)
	case ast.ShowCommunity:
		return execShowCommunity(ctx, s, false)
	case ast.ShowCommunityMembers:
		return execShowCommunity(ctx, s, true)
	case ast.ShowProjectSecurity:
		return listProjectSecurity(ctx)
	case ast.ShowModuleRoles:
		return listModuleRoles(ctx, s.InModule)
	case ast.ShowUserRoles:
		return listUserRoles(ctx)
	case ast.ShowDemoUsers:
		return listDemoUsers(ctx)
	case ast.ShowAccessOn:
		return listAccessOnEntity(ctx, s.Name)
	case ast.ShowAccessOnMicroflow:
		return listAccessOnMicroflow(ctx, s.Name)
	case ast.ShowAccessOnPage:
		return listAccessOnPage(ctx, s.Name)
	case ast.ShowAccessOnWorkflow:
		return listAccessOnWorkflow(ctx, s.Name)
	case ast.ShowAccessOnNanoflow:
		return listAccessOnNanoflow(ctx, s.Name)
	case ast.ShowSecurityMatrix:
		return listSecurityMatrix(ctx, s.InModule)
	case ast.ShowODataClients:
		return listODataClients(ctx, s.InModule)
	case ast.ShowODataServices:
		return listODataServices(ctx, s.InModule)
	case ast.ShowExternalEntities:
		return listExternalEntities(ctx, s.InModule)
	case ast.ShowExternalActions:
		return listExternalActions(ctx, s.InModule)
	case ast.ShowNavigation:
		return listNavigation(ctx)
	case ast.ShowNavigationMenu:
		return listNavigationMenu(ctx, s.Name)
	case ast.ShowNavigationHomes:
		return listNavigationHomes(ctx)
	case ast.ShowStructure:
		return execShowStructure(ctx, s)
	case ast.ShowWorkflows:
		return listWorkflows(ctx, s.InModule)
	case ast.ShowBusinessEventServices:
		return listBusinessEventServices(ctx, s.InModule)
	case ast.ShowBusinessEventClients:
		return listBusinessEventClients(ctx, s.InModule)
	case ast.ShowBusinessEvents:
		return listBusinessEvents(ctx, s.InModule)
	case ast.ShowSettings:
		return listSettings(ctx)
	case ast.ShowLanguages:
		return listLanguages(ctx)
	case ast.ShowFragments:
		return listFragments(ctx)
	case ast.ShowDatabaseConnections:
		return listDatabaseConnections(ctx, s.InModule)
	case ast.ShowImageCollections:
		return listImageCollections(ctx, s.InModule)
	case ast.ShowModels:
		return listAgentEditorModels(ctx, s.InModule)
	case ast.ShowAgents:
		return listAgentEditorAgents(ctx, s.InModule)
	case ast.ShowKnowledgeBases:
		return listAgentEditorKnowledgeBases(ctx, s.InModule)
	case ast.ShowConsumedMCPServices:
		return listAgentEditorConsumedMCPServices(ctx, s.InModule)
	case ast.ShowRestClients:
		return listRestClients(ctx, s.InModule)
	case ast.ShowPublishedRestServices:
		return listPublishedRestServices(ctx, s.InModule)
	case ast.ShowDataTransformers:
		return listDataTransformers(ctx, s.InModule)
	case ast.ShowContractEntities:
		return listContractEntities(ctx, s.Name)
	case ast.ShowContractActions:
		return listContractActions(ctx, s.Name)
	case ast.ShowContractChannels:
		return listContractChannels(ctx, s.Name)
	case ast.ShowContractMessages:
		return listContractMessages(ctx, s.Name)
	case ast.ShowJsonStructures:
		return listJsonStructures(ctx, s.InModule)
	case ast.ShowImportMappings:
		return listImportMappings(ctx, s.InModule)
	case ast.ShowExportMappings:
		return listExportMappings(ctx, s.InModule)
	case ast.ShowJarDependencies:
		return execListJarDependencies(ctx, s.InModule)
	default:
		return mdlerrors.NewUnsupported("unknown show object type")
	}
}

func execDescribe(ctx *ExecContext, s *ast.DescribeStmt) error {
	if !ctx.Connected() && s.ObjectType != ast.DescribeFragment {
		return mdlerrors.NewNotConnected()
	}

	// Bare `DESCRIBE Module.Name` — resolve the concrete type from the project,
	// then fall through to the normal typed dispatch.
	if s.ObjectType == ast.DescribeAuto {
		resolved, err := resolveDescribeAuto(ctx, s.Name.String())
		if err != nil {
			return err
		}
		s.ObjectType = resolved
	}

	// Determine the object type label and name for JSON wrapping.
	objectType := describeObjectTypeLabel(s.ObjectType)
	name := s.Name.String()

	return writeDescribeJSON(ctx, name, objectType, func() error {
		switch s.ObjectType {
		case ast.DescribeEnumeration:
			return describeEnumeration(ctx, s.Name)
		case ast.DescribeEntity:
			return describeEntity(ctx, s.Name)
		case ast.DescribeAssociation:
			return describeAssociation(ctx, s.Name)
		case ast.DescribeMicroflow:
			return describeMicroflow(ctx, s.Name)
		case ast.DescribeNanoflow:
			return describeNanoflow(ctx, s.Name)
		case ast.DescribeModule:
			return describeModule(ctx, s.Name.Module, s.WithAll)
		case ast.DescribePage:
			return describePage(ctx, s.Name)
		case ast.DescribeSnippet:
			return describeSnippet(ctx, s.Name)
		case ast.DescribeLayout:
			return describeLayout(ctx, s.Name)
		case ast.DescribeConstant:
			return describeConstant(ctx, s.Name)
		case ast.DescribeJavaAction:
			return describeJavaAction(ctx, s.Name)
		case ast.DescribeJavaScriptAction:
			return describeJavaScriptAction(ctx, s.Name)
		case ast.DescribeModuleRole:
			return describeModuleRole(ctx, s.Name)
		case ast.DescribeUserRole:
			return describeUserRole(ctx, s.Name)
		case ast.DescribeDemoUser:
			return describeDemoUser(ctx, s.Name.Name)
		case ast.DescribeODataClient:
			return describeODataClient(ctx, s.Name)
		case ast.DescribeODataService:
			return describeODataService(ctx, s.Name)
		case ast.DescribeExternalEntity:
			return describeExternalEntity(ctx, s.Name)
		case ast.DescribeNavigation:
			return describeNavigation(ctx, s.Name)
		case ast.DescribeWorkflow:
			return describeWorkflow(ctx, s.Name)
		case ast.DescribeBusinessEventService:
			return describeBusinessEventService(ctx, s.Name)
		case ast.DescribeDatabaseConnection:
			return describeDatabaseConnection(ctx, s.Name)
		case ast.DescribeSettings:
			return describeSettings(ctx)
		case ast.DescribeFragment:
			return describeFragment(ctx, s.Name)
		case ast.DescribeImageCollection:
			return describeImageCollection(ctx, s.Name)
		case ast.DescribeModel:
			return describeAgentEditorModel(ctx, s.Name)
		case ast.DescribeAgent:
			return describeAgentEditorAgent(ctx, s.Name)
		case ast.DescribeKnowledgeBase:
			return describeAgentEditorKnowledgeBase(ctx, s.Name)
		case ast.DescribeConsumedMCPService:
			return describeAgentEditorConsumedMCPService(ctx, s.Name)
		case ast.DescribeRestClient:
			return describeRestClient(ctx, s.Name)
		case ast.DescribePublishedRestService:
			return describePublishedRestService(ctx, s.Name)
		case ast.DescribeDataTransformer:
			return describeDataTransformer(ctx, s.Name)
		case ast.DescribeContractEntity:
			return describeContractEntity(ctx, s.Name, s.Format)
		case ast.DescribeContractAction:
			return describeContractAction(ctx, s.Name, s.Format)
		case ast.DescribeContractMessage:
			return describeContractMessage(ctx, s.Name)
		case ast.DescribeJsonStructure:
			return describeJsonStructure(ctx, s.Name)
		case ast.DescribeImportMapping:
			return describeImportMapping(ctx, s.Name)
		case ast.DescribeExportMapping:
			return describeExportMapping(ctx, s.Name)
		case ast.DescribeJarDependency:
			return execDescribeJarDependency(ctx, s.Name.String(), s.Qualifier)
		default:
			return mdlerrors.NewUnsupported("unknown describe object type")
		}
	})
}

// describeObjectTypeLabel returns a human-readable label for a describe object type.
func describeObjectTypeLabel(t ast.DescribeObjectType) string {
	switch t {
	case ast.DescribeEnumeration:
		return "enumeration"
	case ast.DescribeEntity:
		return "entity"
	case ast.DescribeAssociation:
		return "association"
	case ast.DescribeMicroflow:
		return "microflow"
	case ast.DescribeNanoflow:
		return "nanoflow"
	case ast.DescribeModule:
		return "module"
	case ast.DescribePage:
		return "page"
	case ast.DescribeSnippet:
		return "snippet"
	case ast.DescribeLayout:
		return "layout"
	case ast.DescribeConstant:
		return "constant"
	case ast.DescribeJavaAction:
		return "javaaction"
	case ast.DescribeJavaScriptAction:
		return "javascriptaction"
	case ast.DescribeModuleRole:
		return "modulerole"
	case ast.DescribeUserRole:
		return "userrole"
	case ast.DescribeDemoUser:
		return "demouser"
	case ast.DescribeODataClient:
		return "odataclient"
	case ast.DescribeODataService:
		return "odataservice"
	case ast.DescribeExternalEntity:
		return "externalentity"
	case ast.DescribeNavigation:
		return "navigation"
	case ast.DescribeWorkflow:
		return "workflow"
	case ast.DescribeBusinessEventService:
		return "businesseventservice"
	case ast.DescribeDatabaseConnection:
		return "databaseconnection"
	case ast.DescribeSettings:
		return "settings"
	case ast.DescribeFragment:
		return "fragment"
	case ast.DescribeImageCollection:
		return "imagecollection"
	case ast.DescribeModel:
		return "model"
	case ast.DescribeAgent:
		return "agent"
	case ast.DescribeKnowledgeBase:
		return "knowledgebase"
	case ast.DescribeConsumedMCPService:
		return "consumedmcpservice"
	case ast.DescribeRestClient:
		return "restclient"
	case ast.DescribePublishedRestService:
		return "publishedrestservice"
	case ast.DescribeDataTransformer:
		return "datatransformer"
	case ast.DescribeContractEntity:
		return "contractentity"
	case ast.DescribeContractAction:
		return "contractaction"
	case ast.DescribeContractMessage:
		return "contractmessage"
	case ast.DescribeJsonStructure:
		return "jsonstructure"
	case ast.DescribeImportMapping:
		return "importmapping"
	case ast.DescribeExportMapping:
		return "exportmapping"
	default:
		return "unknown"
	}
}
