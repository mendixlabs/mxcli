// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
)

func execShow(ctx *ExecContext, s *ast.ShowStmt) error {
	// SHOW GLYPHS reads the Mendix glyph font, not the project, so a connection
	// would be an arbitrary requirement — and the rule that sends people here
	// (MDL078) runs in the project-free pass too.
	if !ctx.Connected() && s.ObjectType != ast.ShowModules && s.ObjectType != ast.ShowFragments &&
		s.ObjectType != ast.ShowGlyphs {
		return mdlerrors.NewNotConnected()
	}

	switch s.ObjectType {
	case ast.ShowModules:
		return listModules(ctx)
	case ast.ShowFolders:
		return listFolders(ctx, s)
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
	case ast.ShowAnnotations:
		return listAnnotations(ctx, s.InModule)
	case ast.ShowAssociations:
		return listAssociations(ctx, s.InModule)
	case ast.ShowAssociation:
		return listAssociation(ctx, s.Name)
	case ast.ShowMicroflows:
		return listMicroflows(ctx, s.InModule)
	case ast.ShowNanoflows:
		return listNanoflows(ctx, s.InModule)
	case ast.ShowRules:
		return listRules(ctx, s.InModule)
	case ast.ShowPages:
		return listPages(ctx, s.InModule)
	case ast.ShowSnippets:
		return listSnippets(ctx, s.InModule)
	case ast.ShowBuildingBlocks:
		return listBuildingBlocks(ctx, s.InModule)
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
	case ast.ShowWorkflowGroups:
		return listWorkflowGroups(ctx)
	case ast.ShowFragments:
		return listFragments(ctx)
	case ast.ShowDatabaseConnections:
		return listDatabaseConnections(ctx, s.InModule)
	case ast.ShowConnections:
		return listOpenSQLConnections(ctx)
	case ast.ShowImageCollections:
		return listImageCollections(ctx, s.InModule)
	case ast.ShowIconCollections:
		return listIconCollections(ctx, s.InModule)
	case ast.ShowGlyphs:
		return listGlyphs(ctx, s.Like)
	case ast.ShowModels:
		return listAgentEditorModels(ctx, s.InModule)
	case ast.ShowAgents:
		return listAgentEditorAgents(ctx, s.InModule)
	case ast.ShowKnowledgeBases:
		return listAgentEditorKnowledgeBases(ctx, s.InModule)
	case ast.ShowConsumedMCPServices:
		return listAgentEditorConsumedMCPServices(ctx, s.InModule)
	case ast.ShowPublishedMCPServices:
		return listAgentEditorPublishedMCPServices(ctx, s.InModule)
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
	case ast.ShowMessageDefinitionCollections:
		return listMessageDefinitionCollections(ctx, s.InModule)
	case ast.ShowMessageDefinitions:
		return listMessageDefinitions(ctx, s.InModule)
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
	// DESCRIBE WIDGET joins DESCRIBE FRAGMENT in not needing a project: a widget
	// definition is not a document in the model, and mxcli's embedded set can
	// answer for a built-in widget with nothing open. With a project the answer
	// is better — the installed .mpk is version-accurate and covers Marketplace
	// widgets — but requiring one would make the statement useless for exactly
	// the "what can I write here?" question it exists to answer.
	if !ctx.Connected() && s.ObjectType != ast.DescribeFragment && s.ObjectType != ast.DescribeWidget &&
		s.ObjectType != ast.DescribeGlyph {
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

	// An entity or association is only ever found by its module. Without one
	// the lookup failed with the create-path message "objects must be created
	// within a module", which reads as if describe were creating something
	// (mendixlabs/mxcli#1218).
	if (s.ObjectType == ast.DescribeEntity || s.ObjectType == ast.DescribeAssociation) && s.Name.Module == "" {
		label := describeObjectTypeLabel(s.ObjectType)
		return mdlerrors.NewValidation(fmt.Sprintf("%q is not a qualified %s name: write Module.%s", s.Name.Name, label, s.Name.Name))
	}

	// Determine the object type label and name for JSON wrapping.
	objectType := describeObjectTypeLabel(s.ObjectType)
	name := s.Name.String()

	// A definition or lookup is a report, not MDL: its first line says so, and
	// the JSON form marks it not executable (R6).
	report, isReport := definitionReportKind(s)
	if isReport && ctx.Format != FormatJSON {
		fmt.Fprintf(ctx.Output, "-- %s definition (not executable)\n", report)
	}

	describe := func() error { return describeByKind(ctx, s) }
	return writeDescribeJSONAs(ctx, name, objectType, !isReport, func() error {
		if isReport {
			return describe()
		}
		return withDescribeHeader(ctx, describe)
	})
}

// describeByKind runs the describe handler for one typed DESCRIBE statement:
// the dispatch execDescribe wraps in its header and JSON envelope, and that
// `describe module ... with all` reuses per document, so a module dump is the
// concatenation of exactly what each document's own describe prints.
func describeByKind(ctx *ExecContext, s *ast.DescribeStmt) error {
	switch s.ObjectType {
	case ast.DescribeEnumeration:
		return describeEnumeration(ctx, s.Name)
	case ast.DescribeEntity:
		return describeEntity(ctx, s.Name)
	case ast.DescribeAssociation:
		return describeAssociation(ctx, s.Name)
	case ast.DescribeMicroflow:
		return describeMicroflowMode(ctx, s.Name, describeMicroflowOptions{Normalized: s.Normalized, Handles: s.WithHandles})
	case ast.DescribeNanoflow:
		return describeNanoflow(ctx, s.Name)
	case ast.DescribeRule:
		return describeRule(ctx, s.Name)
	case ast.DescribeModule:
		return describeModule(ctx, s.Name.Module, s.WithAll)
	case ast.DescribePage:
		return describePage(ctx, s.Name)
	case ast.DescribeSnippet:
		return describeSnippet(ctx, s.Name)
	case ast.DescribeBuildingBlock:
		return describeBuildingBlock(ctx, s.Name)
	case ast.DescribeQueue:
		// Queues and scheduled events carry their own statement types rather
		// than a DescribeStmt kind, so bare `DESCRIBE Module.Name` reaches them
		// by synthesizing one. Without this they are indexed in the catalog's
		// objects view but unreachable without naming the type — the state
		// BUILDING_BLOCK and ICON_COLLECTION were in when 43 of 251 documents
		// in a marketplace project could not be described (see describe_auto.go).
		return execDescribeQueue(ctx, &ast.DescribeQueueStmt{Name: s.Name})
	case ast.DescribeScheduledEvent:
		return execDescribeScheduledEvent(ctx, &ast.DescribeScheduledEventStmt{Name: s.Name})
	case ast.DescribeRegularExpression:
		return execDescribeRegularExpression(ctx, &ast.DescribeRegularExpressionStmt{Name: s.Name})
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
	case ast.DescribeWidget:
		return describeWidgetStmt(ctx, s.Name.Name)
	case ast.DescribeWorkflow:
		return describeWorkflow(ctx, s.Name)
	case ast.DescribeBusinessEventService:
		return describeBusinessEventService(ctx, s.Name)
	case ast.DescribeDatabaseConnection:
		return describeDatabaseConnection(ctx, s.Name)
	case ast.DescribeSettings:
		return describeSettings(ctx, s.Qualifier)
	case ast.DescribeFragment:
		return describeFragment(ctx, s.Name)
	case ast.DescribeImageCollection:
		return describeImageCollection(ctx, s.Name)
	case ast.DescribeIconCollection:
		return describeIconCollection(ctx, s.Name)
	case ast.DescribeGlyph:
		return describeGlyph(ctx, s.Qualifier)
	case ast.DescribeModel:
		return describeAgentEditorModel(ctx, s.Name)
	case ast.DescribeAgent:
		return describeAgentEditorAgent(ctx, s.Name)
	case ast.DescribeKnowledgeBase:
		return describeAgentEditorKnowledgeBase(ctx, s.Name)
	case ast.DescribeConsumedMCPService:
		return describeAgentEditorConsumedMCPService(ctx, s.Name)
	case ast.DescribePublishedMCPService:
		return describeAgentEditorPublishedMCPService(ctx, s.Name)
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
	case ast.DescribeMessageDefinitionCollection:
		return execDescribeMessageDefinitionCollection(ctx, s.Name)
	case ast.DescribeMessageDefinition:
		return execDescribeMessageDefinition(ctx, s.Name)
	case ast.DescribeImportMapping:
		return describeImportMapping(ctx, s.Name)
	case ast.DescribeExportMapping:
		return describeExportMapping(ctx, s.Name)
	case ast.DescribeMenu:
		return describeMenu(ctx, s.Name)
	case ast.DescribeJarDependency:
		return execDescribeJarDependency(ctx, s.Name.String(), s.Qualifier)
	default:
		return mdlerrors.NewUnsupported("unknown describe object type")
	}
}

// definitionReportKind names the kind of definition a describe reports on, when
// its answer is a report rather than runnable MDL: a widget type, a glyph, an
// OData or AsyncAPI contract's entity, action or message (unless asked for as
// MDL with `format mdl`). A model element's describe is MDL and is not one.
func definitionReportKind(s *ast.DescribeStmt) (string, bool) {
	switch s.ObjectType {
	case ast.DescribeWidget:
		return "widget type", true
	case ast.DescribeGlyph:
		return "glyph", true
	case ast.DescribeContractEntity:
		return "contract entity", !strings.EqualFold(s.Format, "mdl")
	case ast.DescribeContractAction:
		return "contract action", !strings.EqualFold(s.Format, "mdl")
	case ast.DescribeContractMessage:
		return "contract message", true
	}
	return "", false
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
	case ast.DescribeRule:
		return "rule"
	case ast.DescribeModule:
		return "module"
	case ast.DescribePage:
		return "page"
	case ast.DescribeSnippet:
		return "snippet"
	case ast.DescribeBuildingBlock:
		return "buildingblock"
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
	case ast.DescribeWidget:
		return "widget"
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
	case ast.DescribeIconCollection:
		return "iconcollection"
	case ast.DescribeGlyph:
		return "glyph"
	case ast.DescribeModel:
		return "model"
	case ast.DescribeAgent:
		return "agent"
	case ast.DescribeKnowledgeBase:
		return "knowledgebase"
	case ast.DescribeConsumedMCPService:
		return "consumedmcpservice"
	case ast.DescribePublishedMCPService:
		return "publishedmcpservice"
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
	case ast.DescribeMessageDefinitionCollection:
		return "messagedefinitioncollection"
	case ast.DescribeMessageDefinition:
		return "messagedefinition"
	case ast.DescribeImportMapping:
		return "importmapping"
	case ast.DescribeExportMapping:
		return "exportmapping"
	case ast.DescribeMenu:
		return "menu"
	case ast.DescribeQueue:
		return "queue"
	case ast.DescribeScheduledEvent:
		return "scheduled event"
	case ast.DescribeRegularExpression:
		return "regular expression"
	default:
		return "unknown"
	}
}
