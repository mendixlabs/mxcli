// Code generated from MDLParser.g4 by ANTLR 4.13.2. DO NOT EDIT.

package parser // MDLParser
import "github.com/antlr4-go/antlr/v4"

// MDLParserListener is a complete listener for a parse tree produced by MDLParser.
type MDLParserListener interface {
	antlr.ParseTreeListener

	// EnterProgram is called when entering the program production.
	EnterProgram(c *ProgramContext)

	// EnterStatement is called when entering the statement production.
	EnterStatement(c *StatementContext)

	// EnterDdlStatement is called when entering the ddlStatement production.
	EnterDdlStatement(c *DdlStatementContext)

	// EnterUpdateWidgetsStatement is called when entering the updateWidgetsStatement production.
	EnterUpdateWidgetsStatement(c *UpdateWidgetsStatementContext)

	// EnterCreateStatement is called when entering the createStatement production.
	EnterCreateStatement(c *CreateStatementContext)

	// EnterAlterStatement is called when entering the alterStatement production.
	EnterAlterStatement(c *AlterStatementContext)

	// EnterAlterStylingAction is called when entering the alterStylingAction production.
	EnterAlterStylingAction(c *AlterStylingActionContext)

	// EnterAlterStylingAssignment is called when entering the alterStylingAssignment production.
	EnterAlterStylingAssignment(c *AlterStylingAssignmentContext)

	// EnterAlterPageOperation is called when entering the alterPageOperation production.
	EnterAlterPageOperation(c *AlterPageOperationContext)

	// EnterAlterPageSet is called when entering the alterPageSet production.
	EnterAlterPageSet(c *AlterPageSetContext)

	// EnterAlterLayoutMapping is called when entering the alterLayoutMapping production.
	EnterAlterLayoutMapping(c *AlterLayoutMappingContext)

	// EnterAlterPageAssignment is called when entering the alterPageAssignment production.
	EnterAlterPageAssignment(c *AlterPageAssignmentContext)

	// EnterAlterPageInsert is called when entering the alterPageInsert production.
	EnterAlterPageInsert(c *AlterPageInsertContext)

	// EnterAlterPageDrop is called when entering the alterPageDrop production.
	EnterAlterPageDrop(c *AlterPageDropContext)

	// EnterAlterPageReplace is called when entering the alterPageReplace production.
	EnterAlterPageReplace(c *AlterPageReplaceContext)

	// EnterAlterPageAddVariable is called when entering the alterPageAddVariable production.
	EnterAlterPageAddVariable(c *AlterPageAddVariableContext)

	// EnterAlterPageDropVariable is called when entering the alterPageDropVariable production.
	EnterAlterPageDropVariable(c *AlterPageDropVariableContext)

	// EnterNavigationClause is called when entering the navigationClause production.
	EnterNavigationClause(c *NavigationClauseContext)

	// EnterNavMenuItemDef is called when entering the navMenuItemDef production.
	EnterNavMenuItemDef(c *NavMenuItemDefContext)

	// EnterDropStatement is called when entering the dropStatement production.
	EnterDropStatement(c *DropStatementContext)

	// EnterRenameStatement is called when entering the renameStatement production.
	EnterRenameStatement(c *RenameStatementContext)

	// EnterMoveStatement is called when entering the moveStatement production.
	EnterMoveStatement(c *MoveStatementContext)

	// EnterSecurityStatement is called when entering the securityStatement production.
	EnterSecurityStatement(c *SecurityStatementContext)

	// EnterCreateModuleRoleStatement is called when entering the createModuleRoleStatement production.
	EnterCreateModuleRoleStatement(c *CreateModuleRoleStatementContext)

	// EnterDropModuleRoleStatement is called when entering the dropModuleRoleStatement production.
	EnterDropModuleRoleStatement(c *DropModuleRoleStatementContext)

	// EnterCreateUserRoleStatement is called when entering the createUserRoleStatement production.
	EnterCreateUserRoleStatement(c *CreateUserRoleStatementContext)

	// EnterAlterUserRoleStatement is called when entering the alterUserRoleStatement production.
	EnterAlterUserRoleStatement(c *AlterUserRoleStatementContext)

	// EnterDropUserRoleStatement is called when entering the dropUserRoleStatement production.
	EnterDropUserRoleStatement(c *DropUserRoleStatementContext)

	// EnterGrantEntityAccessStatement is called when entering the grantEntityAccessStatement production.
	EnterGrantEntityAccessStatement(c *GrantEntityAccessStatementContext)

	// EnterRevokeEntityAccessStatement is called when entering the revokeEntityAccessStatement production.
	EnterRevokeEntityAccessStatement(c *RevokeEntityAccessStatementContext)

	// EnterGrantMicroflowAccessStatement is called when entering the grantMicroflowAccessStatement production.
	EnterGrantMicroflowAccessStatement(c *GrantMicroflowAccessStatementContext)

	// EnterRevokeMicroflowAccessStatement is called when entering the revokeMicroflowAccessStatement production.
	EnterRevokeMicroflowAccessStatement(c *RevokeMicroflowAccessStatementContext)

	// EnterGrantPageAccessStatement is called when entering the grantPageAccessStatement production.
	EnterGrantPageAccessStatement(c *GrantPageAccessStatementContext)

	// EnterRevokePageAccessStatement is called when entering the revokePageAccessStatement production.
	EnterRevokePageAccessStatement(c *RevokePageAccessStatementContext)

	// EnterGrantWorkflowAccessStatement is called when entering the grantWorkflowAccessStatement production.
	EnterGrantWorkflowAccessStatement(c *GrantWorkflowAccessStatementContext)

	// EnterRevokeWorkflowAccessStatement is called when entering the revokeWorkflowAccessStatement production.
	EnterRevokeWorkflowAccessStatement(c *RevokeWorkflowAccessStatementContext)

	// EnterGrantODataServiceAccessStatement is called when entering the grantODataServiceAccessStatement production.
	EnterGrantODataServiceAccessStatement(c *GrantODataServiceAccessStatementContext)

	// EnterRevokeODataServiceAccessStatement is called when entering the revokeODataServiceAccessStatement production.
	EnterRevokeODataServiceAccessStatement(c *RevokeODataServiceAccessStatementContext)

	// EnterAlterProjectSecurityStatement is called when entering the alterProjectSecurityStatement production.
	EnterAlterProjectSecurityStatement(c *AlterProjectSecurityStatementContext)

	// EnterCreateDemoUserStatement is called when entering the createDemoUserStatement production.
	EnterCreateDemoUserStatement(c *CreateDemoUserStatementContext)

	// EnterDropDemoUserStatement is called when entering the dropDemoUserStatement production.
	EnterDropDemoUserStatement(c *DropDemoUserStatementContext)

	// EnterUpdateSecurityStatement is called when entering the updateSecurityStatement production.
	EnterUpdateSecurityStatement(c *UpdateSecurityStatementContext)

	// EnterModuleRoleList is called when entering the moduleRoleList production.
	EnterModuleRoleList(c *ModuleRoleListContext)

	// EnterEntityAccessRightList is called when entering the entityAccessRightList production.
	EnterEntityAccessRightList(c *EntityAccessRightListContext)

	// EnterEntityAccessRight is called when entering the entityAccessRight production.
	EnterEntityAccessRight(c *EntityAccessRightContext)

	// EnterCreateEntityStatement is called when entering the createEntityStatement production.
	EnterCreateEntityStatement(c *CreateEntityStatementContext)

	// EnterGeneralizationClause is called when entering the generalizationClause production.
	EnterGeneralizationClause(c *GeneralizationClauseContext)

	// EnterEntityBody is called when entering the entityBody production.
	EnterEntityBody(c *EntityBodyContext)

	// EnterEntityOptions is called when entering the entityOptions production.
	EnterEntityOptions(c *EntityOptionsContext)

	// EnterEntityOption is called when entering the entityOption production.
	EnterEntityOption(c *EntityOptionContext)

	// EnterAttributeDefinitionList is called when entering the attributeDefinitionList production.
	EnterAttributeDefinitionList(c *AttributeDefinitionListContext)

	// EnterAttributeDefinition is called when entering the attributeDefinition production.
	EnterAttributeDefinition(c *AttributeDefinitionContext)

	// EnterAttributeName is called when entering the attributeName production.
	EnterAttributeName(c *AttributeNameContext)

	// EnterAttributeConstraint is called when entering the attributeConstraint production.
	EnterAttributeConstraint(c *AttributeConstraintContext)

	// EnterDataType is called when entering the dataType production.
	EnterDataType(c *DataTypeContext)

	// EnterTemplateContext is called when entering the templateContext production.
	EnterTemplateContext(c *TemplateContextContext)

	// EnterNonListDataType is called when entering the nonListDataType production.
	EnterNonListDataType(c *NonListDataTypeContext)

	// EnterIndexDefinition is called when entering the indexDefinition production.
	EnterIndexDefinition(c *IndexDefinitionContext)

	// EnterIndexAttributeList is called when entering the indexAttributeList production.
	EnterIndexAttributeList(c *IndexAttributeListContext)

	// EnterIndexAttribute is called when entering the indexAttribute production.
	EnterIndexAttribute(c *IndexAttributeContext)

	// EnterIndexColumnName is called when entering the indexColumnName production.
	EnterIndexColumnName(c *IndexColumnNameContext)

	// EnterCreateAssociationStatement is called when entering the createAssociationStatement production.
	EnterCreateAssociationStatement(c *CreateAssociationStatementContext)

	// EnterAssociationOptions is called when entering the associationOptions production.
	EnterAssociationOptions(c *AssociationOptionsContext)

	// EnterAssociationOption is called when entering the associationOption production.
	EnterAssociationOption(c *AssociationOptionContext)

	// EnterDeleteBehavior is called when entering the deleteBehavior production.
	EnterDeleteBehavior(c *DeleteBehaviorContext)

	// EnterAlterEntityAction is called when entering the alterEntityAction production.
	EnterAlterEntityAction(c *AlterEntityActionContext)

	// EnterAlterAssociationAction is called when entering the alterAssociationAction production.
	EnterAlterAssociationAction(c *AlterAssociationActionContext)

	// EnterAlterEnumerationAction is called when entering the alterEnumerationAction production.
	EnterAlterEnumerationAction(c *AlterEnumerationActionContext)

	// EnterAlterNotebookAction is called when entering the alterNotebookAction production.
	EnterAlterNotebookAction(c *AlterNotebookActionContext)

	// EnterCreateModuleStatement is called when entering the createModuleStatement production.
	EnterCreateModuleStatement(c *CreateModuleStatementContext)

	// EnterModuleOptions is called when entering the moduleOptions production.
	EnterModuleOptions(c *ModuleOptionsContext)

	// EnterModuleOption is called when entering the moduleOption production.
	EnterModuleOption(c *ModuleOptionContext)

	// EnterCreateEnumerationStatement is called when entering the createEnumerationStatement production.
	EnterCreateEnumerationStatement(c *CreateEnumerationStatementContext)

	// EnterEnumerationValueList is called when entering the enumerationValueList production.
	EnterEnumerationValueList(c *EnumerationValueListContext)

	// EnterEnumerationValue is called when entering the enumerationValue production.
	EnterEnumerationValue(c *EnumerationValueContext)

	// EnterEnumValueName is called when entering the enumValueName production.
	EnterEnumValueName(c *EnumValueNameContext)

	// EnterEnumerationOptions is called when entering the enumerationOptions production.
	EnterEnumerationOptions(c *EnumerationOptionsContext)

	// EnterEnumerationOption is called when entering the enumerationOption production.
	EnterEnumerationOption(c *EnumerationOptionContext)

	// EnterCreateImageCollectionStatement is called when entering the createImageCollectionStatement production.
	EnterCreateImageCollectionStatement(c *CreateImageCollectionStatementContext)

	// EnterImageCollectionOptions is called when entering the imageCollectionOptions production.
	EnterImageCollectionOptions(c *ImageCollectionOptionsContext)

	// EnterImageCollectionOption is called when entering the imageCollectionOption production.
	EnterImageCollectionOption(c *ImageCollectionOptionContext)

	// EnterImageCollectionBody is called when entering the imageCollectionBody production.
	EnterImageCollectionBody(c *ImageCollectionBodyContext)

	// EnterImageCollectionItem is called when entering the imageCollectionItem production.
	EnterImageCollectionItem(c *ImageCollectionItemContext)

	// EnterImageName is called when entering the imageName production.
	EnterImageName(c *ImageNameContext)

	// EnterCreateJsonStructureStatement is called when entering the createJsonStructureStatement production.
	EnterCreateJsonStructureStatement(c *CreateJsonStructureStatementContext)

	// EnterCustomNameMapping is called when entering the customNameMapping production.
	EnterCustomNameMapping(c *CustomNameMappingContext)

	// EnterCreateValidationRuleStatement is called when entering the createValidationRuleStatement production.
	EnterCreateValidationRuleStatement(c *CreateValidationRuleStatementContext)

	// EnterValidationRuleBody is called when entering the validationRuleBody production.
	EnterValidationRuleBody(c *ValidationRuleBodyContext)

	// EnterRangeConstraint is called when entering the rangeConstraint production.
	EnterRangeConstraint(c *RangeConstraintContext)

	// EnterAttributeReference is called when entering the attributeReference production.
	EnterAttributeReference(c *AttributeReferenceContext)

	// EnterAttributeReferenceList is called when entering the attributeReferenceList production.
	EnterAttributeReferenceList(c *AttributeReferenceListContext)

	// EnterCreateMicroflowStatement is called when entering the createMicroflowStatement production.
	EnterCreateMicroflowStatement(c *CreateMicroflowStatementContext)

	// EnterCreateJavaActionStatement is called when entering the createJavaActionStatement production.
	EnterCreateJavaActionStatement(c *CreateJavaActionStatementContext)

	// EnterJavaActionParameterList is called when entering the javaActionParameterList production.
	EnterJavaActionParameterList(c *JavaActionParameterListContext)

	// EnterJavaActionParameter is called when entering the javaActionParameter production.
	EnterJavaActionParameter(c *JavaActionParameterContext)

	// EnterJavaActionReturnType is called when entering the javaActionReturnType production.
	EnterJavaActionReturnType(c *JavaActionReturnTypeContext)

	// EnterJavaActionExposedClause is called when entering the javaActionExposedClause production.
	EnterJavaActionExposedClause(c *JavaActionExposedClauseContext)

	// EnterMicroflowParameterList is called when entering the microflowParameterList production.
	EnterMicroflowParameterList(c *MicroflowParameterListContext)

	// EnterMicroflowParameter is called when entering the microflowParameter production.
	EnterMicroflowParameter(c *MicroflowParameterContext)

	// EnterParameterName is called when entering the parameterName production.
	EnterParameterName(c *ParameterNameContext)

	// EnterMicroflowReturnType is called when entering the microflowReturnType production.
	EnterMicroflowReturnType(c *MicroflowReturnTypeContext)

	// EnterMicroflowOptions is called when entering the microflowOptions production.
	EnterMicroflowOptions(c *MicroflowOptionsContext)

	// EnterMicroflowOption is called when entering the microflowOption production.
	EnterMicroflowOption(c *MicroflowOptionContext)

	// EnterMicroflowBody is called when entering the microflowBody production.
	EnterMicroflowBody(c *MicroflowBodyContext)

	// EnterMicroflowStatement is called when entering the microflowStatement production.
	EnterMicroflowStatement(c *MicroflowStatementContext)

	// EnterDeclareStatement is called when entering the declareStatement production.
	EnterDeclareStatement(c *DeclareStatementContext)

	// EnterSetStatement is called when entering the setStatement production.
	EnterSetStatement(c *SetStatementContext)

	// EnterCreateObjectStatement is called when entering the createObjectStatement production.
	EnterCreateObjectStatement(c *CreateObjectStatementContext)

	// EnterChangeObjectStatement is called when entering the changeObjectStatement production.
	EnterChangeObjectStatement(c *ChangeObjectStatementContext)

	// EnterAttributePath is called when entering the attributePath production.
	EnterAttributePath(c *AttributePathContext)

	// EnterCommitStatement is called when entering the commitStatement production.
	EnterCommitStatement(c *CommitStatementContext)

	// EnterDeleteObjectStatement is called when entering the deleteObjectStatement production.
	EnterDeleteObjectStatement(c *DeleteObjectStatementContext)

	// EnterRollbackStatement is called when entering the rollbackStatement production.
	EnterRollbackStatement(c *RollbackStatementContext)

	// EnterRetrieveStatement is called when entering the retrieveStatement production.
	EnterRetrieveStatement(c *RetrieveStatementContext)

	// EnterRetrieveSource is called when entering the retrieveSource production.
	EnterRetrieveSource(c *RetrieveSourceContext)

	// EnterOnErrorClause is called when entering the onErrorClause production.
	EnterOnErrorClause(c *OnErrorClauseContext)

	// EnterIfStatement is called when entering the ifStatement production.
	EnterIfStatement(c *IfStatementContext)

	// EnterLoopStatement is called when entering the loopStatement production.
	EnterLoopStatement(c *LoopStatementContext)

	// EnterWhileStatement is called when entering the whileStatement production.
	EnterWhileStatement(c *WhileStatementContext)

	// EnterContinueStatement is called when entering the continueStatement production.
	EnterContinueStatement(c *ContinueStatementContext)

	// EnterBreakStatement is called when entering the breakStatement production.
	EnterBreakStatement(c *BreakStatementContext)

	// EnterReturnStatement is called when entering the returnStatement production.
	EnterReturnStatement(c *ReturnStatementContext)

	// EnterRaiseErrorStatement is called when entering the raiseErrorStatement production.
	EnterRaiseErrorStatement(c *RaiseErrorStatementContext)

	// EnterLogStatement is called when entering the logStatement production.
	EnterLogStatement(c *LogStatementContext)

	// EnterLogLevel is called when entering the logLevel production.
	EnterLogLevel(c *LogLevelContext)

	// EnterTemplateParams is called when entering the templateParams production.
	EnterTemplateParams(c *TemplateParamsContext)

	// EnterTemplateParam is called when entering the templateParam production.
	EnterTemplateParam(c *TemplateParamContext)

	// EnterLogTemplateParams is called when entering the logTemplateParams production.
	EnterLogTemplateParams(c *LogTemplateParamsContext)

	// EnterLogTemplateParam is called when entering the logTemplateParam production.
	EnterLogTemplateParam(c *LogTemplateParamContext)

	// EnterCallMicroflowStatement is called when entering the callMicroflowStatement production.
	EnterCallMicroflowStatement(c *CallMicroflowStatementContext)

	// EnterCallJavaActionStatement is called when entering the callJavaActionStatement production.
	EnterCallJavaActionStatement(c *CallJavaActionStatementContext)

	// EnterExecuteDatabaseQueryStatement is called when entering the executeDatabaseQueryStatement production.
	EnterExecuteDatabaseQueryStatement(c *ExecuteDatabaseQueryStatementContext)

	// EnterCallExternalActionStatement is called when entering the callExternalActionStatement production.
	EnterCallExternalActionStatement(c *CallExternalActionStatementContext)

	// EnterCallArgumentList is called when entering the callArgumentList production.
	EnterCallArgumentList(c *CallArgumentListContext)

	// EnterCallArgument is called when entering the callArgument production.
	EnterCallArgument(c *CallArgumentContext)

	// EnterShowPageStatement is called when entering the showPageStatement production.
	EnterShowPageStatement(c *ShowPageStatementContext)

	// EnterShowPageArgList is called when entering the showPageArgList production.
	EnterShowPageArgList(c *ShowPageArgListContext)

	// EnterShowPageArg is called when entering the showPageArg production.
	EnterShowPageArg(c *ShowPageArgContext)

	// EnterClosePageStatement is called when entering the closePageStatement production.
	EnterClosePageStatement(c *ClosePageStatementContext)

	// EnterShowHomePageStatement is called when entering the showHomePageStatement production.
	EnterShowHomePageStatement(c *ShowHomePageStatementContext)

	// EnterShowMessageStatement is called when entering the showMessageStatement production.
	EnterShowMessageStatement(c *ShowMessageStatementContext)

	// EnterThrowStatement is called when entering the throwStatement production.
	EnterThrowStatement(c *ThrowStatementContext)

	// EnterValidationFeedbackStatement is called when entering the validationFeedbackStatement production.
	EnterValidationFeedbackStatement(c *ValidationFeedbackStatementContext)

	// EnterRestCallStatement is called when entering the restCallStatement production.
	EnterRestCallStatement(c *RestCallStatementContext)

	// EnterHttpMethod is called when entering the httpMethod production.
	EnterHttpMethod(c *HttpMethodContext)

	// EnterRestCallUrl is called when entering the restCallUrl production.
	EnterRestCallUrl(c *RestCallUrlContext)

	// EnterRestCallUrlParams is called when entering the restCallUrlParams production.
	EnterRestCallUrlParams(c *RestCallUrlParamsContext)

	// EnterRestCallHeaderClause is called when entering the restCallHeaderClause production.
	EnterRestCallHeaderClause(c *RestCallHeaderClauseContext)

	// EnterRestCallAuthClause is called when entering the restCallAuthClause production.
	EnterRestCallAuthClause(c *RestCallAuthClauseContext)

	// EnterRestCallBodyClause is called when entering the restCallBodyClause production.
	EnterRestCallBodyClause(c *RestCallBodyClauseContext)

	// EnterRestCallTimeoutClause is called when entering the restCallTimeoutClause production.
	EnterRestCallTimeoutClause(c *RestCallTimeoutClauseContext)

	// EnterRestCallReturnsClause is called when entering the restCallReturnsClause production.
	EnterRestCallReturnsClause(c *RestCallReturnsClauseContext)

	// EnterSendRestRequestStatement is called when entering the sendRestRequestStatement production.
	EnterSendRestRequestStatement(c *SendRestRequestStatementContext)

	// EnterSendRestRequestBodyClause is called when entering the sendRestRequestBodyClause production.
	EnterSendRestRequestBodyClause(c *SendRestRequestBodyClauseContext)

	// EnterListOperationStatement is called when entering the listOperationStatement production.
	EnterListOperationStatement(c *ListOperationStatementContext)

	// EnterListOperation is called when entering the listOperation production.
	EnterListOperation(c *ListOperationContext)

	// EnterSortSpecList is called when entering the sortSpecList production.
	EnterSortSpecList(c *SortSpecListContext)

	// EnterSortSpec is called when entering the sortSpec production.
	EnterSortSpec(c *SortSpecContext)

	// EnterAggregateListStatement is called when entering the aggregateListStatement production.
	EnterAggregateListStatement(c *AggregateListStatementContext)

	// EnterListAggregateOperation is called when entering the listAggregateOperation production.
	EnterListAggregateOperation(c *ListAggregateOperationContext)

	// EnterCreateListStatement is called when entering the createListStatement production.
	EnterCreateListStatement(c *CreateListStatementContext)

	// EnterAddToListStatement is called when entering the addToListStatement production.
	EnterAddToListStatement(c *AddToListStatementContext)

	// EnterRemoveFromListStatement is called when entering the removeFromListStatement production.
	EnterRemoveFromListStatement(c *RemoveFromListStatementContext)

	// EnterMemberAssignmentList is called when entering the memberAssignmentList production.
	EnterMemberAssignmentList(c *MemberAssignmentListContext)

	// EnterMemberAssignment is called when entering the memberAssignment production.
	EnterMemberAssignment(c *MemberAssignmentContext)

	// EnterMemberAttributeName is called when entering the memberAttributeName production.
	EnterMemberAttributeName(c *MemberAttributeNameContext)

	// EnterChangeList is called when entering the changeList production.
	EnterChangeList(c *ChangeListContext)

	// EnterChangeItem is called when entering the changeItem production.
	EnterChangeItem(c *ChangeItemContext)

	// EnterCreatePageStatement is called when entering the createPageStatement production.
	EnterCreatePageStatement(c *CreatePageStatementContext)

	// EnterCreateSnippetStatement is called when entering the createSnippetStatement production.
	EnterCreateSnippetStatement(c *CreateSnippetStatementContext)

	// EnterSnippetOptions is called when entering the snippetOptions production.
	EnterSnippetOptions(c *SnippetOptionsContext)

	// EnterSnippetOption is called when entering the snippetOption production.
	EnterSnippetOption(c *SnippetOptionContext)

	// EnterPageParameterList is called when entering the pageParameterList production.
	EnterPageParameterList(c *PageParameterListContext)

	// EnterPageParameter is called when entering the pageParameter production.
	EnterPageParameter(c *PageParameterContext)

	// EnterSnippetParameterList is called when entering the snippetParameterList production.
	EnterSnippetParameterList(c *SnippetParameterListContext)

	// EnterSnippetParameter is called when entering the snippetParameter production.
	EnterSnippetParameter(c *SnippetParameterContext)

	// EnterVariableDeclarationList is called when entering the variableDeclarationList production.
	EnterVariableDeclarationList(c *VariableDeclarationListContext)

	// EnterVariableDeclaration is called when entering the variableDeclaration production.
	EnterVariableDeclaration(c *VariableDeclarationContext)

	// EnterSortColumn is called when entering the sortColumn production.
	EnterSortColumn(c *SortColumnContext)

	// EnterXpathConstraint is called when entering the xpathConstraint production.
	EnterXpathConstraint(c *XpathConstraintContext)

	// EnterAndOrXpath is called when entering the andOrXpath production.
	EnterAndOrXpath(c *AndOrXpathContext)

	// EnterXpathExpr is called when entering the xpathExpr production.
	EnterXpathExpr(c *XpathExprContext)

	// EnterXpathAndExpr is called when entering the xpathAndExpr production.
	EnterXpathAndExpr(c *XpathAndExprContext)

	// EnterXpathNotExpr is called when entering the xpathNotExpr production.
	EnterXpathNotExpr(c *XpathNotExprContext)

	// EnterXpathComparisonExpr is called when entering the xpathComparisonExpr production.
	EnterXpathComparisonExpr(c *XpathComparisonExprContext)

	// EnterXpathValueExpr is called when entering the xpathValueExpr production.
	EnterXpathValueExpr(c *XpathValueExprContext)

	// EnterXpathPath is called when entering the xpathPath production.
	EnterXpathPath(c *XpathPathContext)

	// EnterXpathStep is called when entering the xpathStep production.
	EnterXpathStep(c *XpathStepContext)

	// EnterXpathStepValue is called when entering the xpathStepValue production.
	EnterXpathStepValue(c *XpathStepValueContext)

	// EnterXpathQualifiedName is called when entering the xpathQualifiedName production.
	EnterXpathQualifiedName(c *XpathQualifiedNameContext)

	// EnterXpathWord is called when entering the xpathWord production.
	EnterXpathWord(c *XpathWordContext)

	// EnterXpathFunctionCall is called when entering the xpathFunctionCall production.
	EnterXpathFunctionCall(c *XpathFunctionCallContext)

	// EnterXpathFunctionName is called when entering the xpathFunctionName production.
	EnterXpathFunctionName(c *XpathFunctionNameContext)

	// EnterPageHeaderV3 is called when entering the pageHeaderV3 production.
	EnterPageHeaderV3(c *PageHeaderV3Context)

	// EnterPageHeaderPropertyV3 is called when entering the pageHeaderPropertyV3 production.
	EnterPageHeaderPropertyV3(c *PageHeaderPropertyV3Context)

	// EnterSnippetHeaderV3 is called when entering the snippetHeaderV3 production.
	EnterSnippetHeaderV3(c *SnippetHeaderV3Context)

	// EnterSnippetHeaderPropertyV3 is called when entering the snippetHeaderPropertyV3 production.
	EnterSnippetHeaderPropertyV3(c *SnippetHeaderPropertyV3Context)

	// EnterPageBodyV3 is called when entering the pageBodyV3 production.
	EnterPageBodyV3(c *PageBodyV3Context)

	// EnterUseFragmentRef is called when entering the useFragmentRef production.
	EnterUseFragmentRef(c *UseFragmentRefContext)

	// EnterWidgetV3 is called when entering the widgetV3 production.
	EnterWidgetV3(c *WidgetV3Context)

	// EnterWidgetTypeV3 is called when entering the widgetTypeV3 production.
	EnterWidgetTypeV3(c *WidgetTypeV3Context)

	// EnterWidgetPropertiesV3 is called when entering the widgetPropertiesV3 production.
	EnterWidgetPropertiesV3(c *WidgetPropertiesV3Context)

	// EnterWidgetPropertyV3 is called when entering the widgetPropertyV3 production.
	EnterWidgetPropertyV3(c *WidgetPropertyV3Context)

	// EnterFilterTypeValue is called when entering the filterTypeValue production.
	EnterFilterTypeValue(c *FilterTypeValueContext)

	// EnterAttributeListV3 is called when entering the attributeListV3 production.
	EnterAttributeListV3(c *AttributeListV3Context)

	// EnterDataSourceExprV3 is called when entering the dataSourceExprV3 production.
	EnterDataSourceExprV3(c *DataSourceExprV3Context)

	// EnterActionExprV3 is called when entering the actionExprV3 production.
	EnterActionExprV3(c *ActionExprV3Context)

	// EnterMicroflowArgsV3 is called when entering the microflowArgsV3 production.
	EnterMicroflowArgsV3(c *MicroflowArgsV3Context)

	// EnterMicroflowArgV3 is called when entering the microflowArgV3 production.
	EnterMicroflowArgV3(c *MicroflowArgV3Context)

	// EnterAttributePathV3 is called when entering the attributePathV3 production.
	EnterAttributePathV3(c *AttributePathV3Context)

	// EnterStringExprV3 is called when entering the stringExprV3 production.
	EnterStringExprV3(c *StringExprV3Context)

	// EnterParamListV3 is called when entering the paramListV3 production.
	EnterParamListV3(c *ParamListV3Context)

	// EnterParamAssignmentV3 is called when entering the paramAssignmentV3 production.
	EnterParamAssignmentV3(c *ParamAssignmentV3Context)

	// EnterRenderModeV3 is called when entering the renderModeV3 production.
	EnterRenderModeV3(c *RenderModeV3Context)

	// EnterButtonStyleV3 is called when entering the buttonStyleV3 production.
	EnterButtonStyleV3(c *ButtonStyleV3Context)

	// EnterDesktopWidthV3 is called when entering the desktopWidthV3 production.
	EnterDesktopWidthV3(c *DesktopWidthV3Context)

	// EnterSelectionModeV3 is called when entering the selectionModeV3 production.
	EnterSelectionModeV3(c *SelectionModeV3Context)

	// EnterPropertyValueV3 is called when entering the propertyValueV3 production.
	EnterPropertyValueV3(c *PropertyValueV3Context)

	// EnterDesignPropertyListV3 is called when entering the designPropertyListV3 production.
	EnterDesignPropertyListV3(c *DesignPropertyListV3Context)

	// EnterDesignPropertyEntryV3 is called when entering the designPropertyEntryV3 production.
	EnterDesignPropertyEntryV3(c *DesignPropertyEntryV3Context)

	// EnterWidgetBodyV3 is called when entering the widgetBodyV3 production.
	EnterWidgetBodyV3(c *WidgetBodyV3Context)

	// EnterCreateNotebookStatement is called when entering the createNotebookStatement production.
	EnterCreateNotebookStatement(c *CreateNotebookStatementContext)

	// EnterNotebookOptions is called when entering the notebookOptions production.
	EnterNotebookOptions(c *NotebookOptionsContext)

	// EnterNotebookOption is called when entering the notebookOption production.
	EnterNotebookOption(c *NotebookOptionContext)

	// EnterNotebookPage is called when entering the notebookPage production.
	EnterNotebookPage(c *NotebookPageContext)

	// EnterCreateDatabaseConnectionStatement is called when entering the createDatabaseConnectionStatement production.
	EnterCreateDatabaseConnectionStatement(c *CreateDatabaseConnectionStatementContext)

	// EnterDatabaseConnectionOption is called when entering the databaseConnectionOption production.
	EnterDatabaseConnectionOption(c *DatabaseConnectionOptionContext)

	// EnterDatabaseQuery is called when entering the databaseQuery production.
	EnterDatabaseQuery(c *DatabaseQueryContext)

	// EnterDatabaseQueryMapping is called when entering the databaseQueryMapping production.
	EnterDatabaseQueryMapping(c *DatabaseQueryMappingContext)

	// EnterCreateConstantStatement is called when entering the createConstantStatement production.
	EnterCreateConstantStatement(c *CreateConstantStatementContext)

	// EnterConstantOptions is called when entering the constantOptions production.
	EnterConstantOptions(c *ConstantOptionsContext)

	// EnterConstantOption is called when entering the constantOption production.
	EnterConstantOption(c *ConstantOptionContext)

	// EnterCreateConfigurationStatement is called when entering the createConfigurationStatement production.
	EnterCreateConfigurationStatement(c *CreateConfigurationStatementContext)

	// EnterCreateRestClientStatement is called when entering the createRestClientStatement production.
	EnterCreateRestClientStatement(c *CreateRestClientStatementContext)

	// EnterRestClientBaseUrl is called when entering the restClientBaseUrl production.
	EnterRestClientBaseUrl(c *RestClientBaseUrlContext)

	// EnterRestClientAuthentication is called when entering the restClientAuthentication production.
	EnterRestClientAuthentication(c *RestClientAuthenticationContext)

	// EnterRestAuthValue is called when entering the restAuthValue production.
	EnterRestAuthValue(c *RestAuthValueContext)

	// EnterRestOperationDef is called when entering the restOperationDef production.
	EnterRestOperationDef(c *RestOperationDefContext)

	// EnterRestHttpMethod is called when entering the restHttpMethod production.
	EnterRestHttpMethod(c *RestHttpMethodContext)

	// EnterRestOperationClause is called when entering the restOperationClause production.
	EnterRestOperationClause(c *RestOperationClauseContext)

	// EnterRestHeaderValue is called when entering the restHeaderValue production.
	EnterRestHeaderValue(c *RestHeaderValueContext)

	// EnterRestResponseSpec is called when entering the restResponseSpec production.
	EnterRestResponseSpec(c *RestResponseSpecContext)

	// EnterCreateIndexStatement is called when entering the createIndexStatement production.
	EnterCreateIndexStatement(c *CreateIndexStatementContext)

	// EnterCreateODataClientStatement is called when entering the createODataClientStatement production.
	EnterCreateODataClientStatement(c *CreateODataClientStatementContext)

	// EnterCreateODataServiceStatement is called when entering the createODataServiceStatement production.
	EnterCreateODataServiceStatement(c *CreateODataServiceStatementContext)

	// EnterOdataPropertyValue is called when entering the odataPropertyValue production.
	EnterOdataPropertyValue(c *OdataPropertyValueContext)

	// EnterOdataPropertyAssignment is called when entering the odataPropertyAssignment production.
	EnterOdataPropertyAssignment(c *OdataPropertyAssignmentContext)

	// EnterOdataAlterAssignment is called when entering the odataAlterAssignment production.
	EnterOdataAlterAssignment(c *OdataAlterAssignmentContext)

	// EnterOdataAuthenticationClause is called when entering the odataAuthenticationClause production.
	EnterOdataAuthenticationClause(c *OdataAuthenticationClauseContext)

	// EnterOdataAuthType is called when entering the odataAuthType production.
	EnterOdataAuthType(c *OdataAuthTypeContext)

	// EnterPublishEntityBlock is called when entering the publishEntityBlock production.
	EnterPublishEntityBlock(c *PublishEntityBlockContext)

	// EnterExposeClause is called when entering the exposeClause production.
	EnterExposeClause(c *ExposeClauseContext)

	// EnterExposeMember is called when entering the exposeMember production.
	EnterExposeMember(c *ExposeMemberContext)

	// EnterExposeMemberOptions is called when entering the exposeMemberOptions production.
	EnterExposeMemberOptions(c *ExposeMemberOptionsContext)

	// EnterCreateExternalEntityStatement is called when entering the createExternalEntityStatement production.
	EnterCreateExternalEntityStatement(c *CreateExternalEntityStatementContext)

	// EnterCreateNavigationStatement is called when entering the createNavigationStatement production.
	EnterCreateNavigationStatement(c *CreateNavigationStatementContext)

	// EnterOdataHeadersClause is called when entering the odataHeadersClause production.
	EnterOdataHeadersClause(c *OdataHeadersClauseContext)

	// EnterOdataHeaderEntry is called when entering the odataHeaderEntry production.
	EnterOdataHeaderEntry(c *OdataHeaderEntryContext)

	// EnterCreateBusinessEventServiceStatement is called when entering the createBusinessEventServiceStatement production.
	EnterCreateBusinessEventServiceStatement(c *CreateBusinessEventServiceStatementContext)

	// EnterBusinessEventMessageDef is called when entering the businessEventMessageDef production.
	EnterBusinessEventMessageDef(c *BusinessEventMessageDefContext)

	// EnterBusinessEventAttrDef is called when entering the businessEventAttrDef production.
	EnterBusinessEventAttrDef(c *BusinessEventAttrDefContext)

	// EnterCreateWorkflowStatement is called when entering the createWorkflowStatement production.
	EnterCreateWorkflowStatement(c *CreateWorkflowStatementContext)

	// EnterWorkflowBody is called when entering the workflowBody production.
	EnterWorkflowBody(c *WorkflowBodyContext)

	// EnterWorkflowActivityStmt is called when entering the workflowActivityStmt production.
	EnterWorkflowActivityStmt(c *WorkflowActivityStmtContext)

	// EnterWorkflowUserTaskStmt is called when entering the workflowUserTaskStmt production.
	EnterWorkflowUserTaskStmt(c *WorkflowUserTaskStmtContext)

	// EnterWorkflowBoundaryEventClause is called when entering the workflowBoundaryEventClause production.
	EnterWorkflowBoundaryEventClause(c *WorkflowBoundaryEventClauseContext)

	// EnterWorkflowUserTaskOutcome is called when entering the workflowUserTaskOutcome production.
	EnterWorkflowUserTaskOutcome(c *WorkflowUserTaskOutcomeContext)

	// EnterWorkflowCallMicroflowStmt is called when entering the workflowCallMicroflowStmt production.
	EnterWorkflowCallMicroflowStmt(c *WorkflowCallMicroflowStmtContext)

	// EnterWorkflowParameterMapping is called when entering the workflowParameterMapping production.
	EnterWorkflowParameterMapping(c *WorkflowParameterMappingContext)

	// EnterWorkflowCallWorkflowStmt is called when entering the workflowCallWorkflowStmt production.
	EnterWorkflowCallWorkflowStmt(c *WorkflowCallWorkflowStmtContext)

	// EnterWorkflowDecisionStmt is called when entering the workflowDecisionStmt production.
	EnterWorkflowDecisionStmt(c *WorkflowDecisionStmtContext)

	// EnterWorkflowConditionOutcome is called when entering the workflowConditionOutcome production.
	EnterWorkflowConditionOutcome(c *WorkflowConditionOutcomeContext)

	// EnterWorkflowParallelSplitStmt is called when entering the workflowParallelSplitStmt production.
	EnterWorkflowParallelSplitStmt(c *WorkflowParallelSplitStmtContext)

	// EnterWorkflowParallelPath is called when entering the workflowParallelPath production.
	EnterWorkflowParallelPath(c *WorkflowParallelPathContext)

	// EnterWorkflowJumpToStmt is called when entering the workflowJumpToStmt production.
	EnterWorkflowJumpToStmt(c *WorkflowJumpToStmtContext)

	// EnterWorkflowWaitForTimerStmt is called when entering the workflowWaitForTimerStmt production.
	EnterWorkflowWaitForTimerStmt(c *WorkflowWaitForTimerStmtContext)

	// EnterWorkflowWaitForNotificationStmt is called when entering the workflowWaitForNotificationStmt production.
	EnterWorkflowWaitForNotificationStmt(c *WorkflowWaitForNotificationStmtContext)

	// EnterWorkflowAnnotationStmt is called when entering the workflowAnnotationStmt production.
	EnterWorkflowAnnotationStmt(c *WorkflowAnnotationStmtContext)

	// EnterAlterSettingsClause is called when entering the alterSettingsClause production.
	EnterAlterSettingsClause(c *AlterSettingsClauseContext)

	// EnterSettingsSection is called when entering the settingsSection production.
	EnterSettingsSection(c *SettingsSectionContext)

	// EnterSettingsAssignment is called when entering the settingsAssignment production.
	EnterSettingsAssignment(c *SettingsAssignmentContext)

	// EnterSettingsValue is called when entering the settingsValue production.
	EnterSettingsValue(c *SettingsValueContext)

	// EnterDqlStatement is called when entering the dqlStatement production.
	EnterDqlStatement(c *DqlStatementContext)

	// EnterShowStatement is called when entering the showStatement production.
	EnterShowStatement(c *ShowStatementContext)

	// EnterShowWidgetsFilter is called when entering the showWidgetsFilter production.
	EnterShowWidgetsFilter(c *ShowWidgetsFilterContext)

	// EnterWidgetTypeKeyword is called when entering the widgetTypeKeyword production.
	EnterWidgetTypeKeyword(c *WidgetTypeKeywordContext)

	// EnterWidgetCondition is called when entering the widgetCondition production.
	EnterWidgetCondition(c *WidgetConditionContext)

	// EnterWidgetPropertyAssignment is called when entering the widgetPropertyAssignment production.
	EnterWidgetPropertyAssignment(c *WidgetPropertyAssignmentContext)

	// EnterWidgetPropertyValue is called when entering the widgetPropertyValue production.
	EnterWidgetPropertyValue(c *WidgetPropertyValueContext)

	// EnterDescribeStatement is called when entering the describeStatement production.
	EnterDescribeStatement(c *DescribeStatementContext)

	// EnterCatalogSelectQuery is called when entering the catalogSelectQuery production.
	EnterCatalogSelectQuery(c *CatalogSelectQueryContext)

	// EnterCatalogJoinClause is called when entering the catalogJoinClause production.
	EnterCatalogJoinClause(c *CatalogJoinClauseContext)

	// EnterCatalogTableName is called when entering the catalogTableName production.
	EnterCatalogTableName(c *CatalogTableNameContext)

	// EnterOqlQuery is called when entering the oqlQuery production.
	EnterOqlQuery(c *OqlQueryContext)

	// EnterOqlQueryTerm is called when entering the oqlQueryTerm production.
	EnterOqlQueryTerm(c *OqlQueryTermContext)

	// EnterSelectClause is called when entering the selectClause production.
	EnterSelectClause(c *SelectClauseContext)

	// EnterSelectList is called when entering the selectList production.
	EnterSelectList(c *SelectListContext)

	// EnterSelectItem is called when entering the selectItem production.
	EnterSelectItem(c *SelectItemContext)

	// EnterSelectAlias is called when entering the selectAlias production.
	EnterSelectAlias(c *SelectAliasContext)

	// EnterFromClause is called when entering the fromClause production.
	EnterFromClause(c *FromClauseContext)

	// EnterTableReference is called when entering the tableReference production.
	EnterTableReference(c *TableReferenceContext)

	// EnterJoinClause is called when entering the joinClause production.
	EnterJoinClause(c *JoinClauseContext)

	// EnterAssociationPath is called when entering the associationPath production.
	EnterAssociationPath(c *AssociationPathContext)

	// EnterJoinType is called when entering the joinType production.
	EnterJoinType(c *JoinTypeContext)

	// EnterWhereClause is called when entering the whereClause production.
	EnterWhereClause(c *WhereClauseContext)

	// EnterGroupByClause is called when entering the groupByClause production.
	EnterGroupByClause(c *GroupByClauseContext)

	// EnterHavingClause is called when entering the havingClause production.
	EnterHavingClause(c *HavingClauseContext)

	// EnterOrderByClause is called when entering the orderByClause production.
	EnterOrderByClause(c *OrderByClauseContext)

	// EnterOrderByList is called when entering the orderByList production.
	EnterOrderByList(c *OrderByListContext)

	// EnterOrderByItem is called when entering the orderByItem production.
	EnterOrderByItem(c *OrderByItemContext)

	// EnterGroupByList is called when entering the groupByList production.
	EnterGroupByList(c *GroupByListContext)

	// EnterLimitOffsetClause is called when entering the limitOffsetClause production.
	EnterLimitOffsetClause(c *LimitOffsetClauseContext)

	// EnterUtilityStatement is called when entering the utilityStatement production.
	EnterUtilityStatement(c *UtilityStatementContext)

	// EnterSearchStatement is called when entering the searchStatement production.
	EnterSearchStatement(c *SearchStatementContext)

	// EnterConnectStatement is called when entering the connectStatement production.
	EnterConnectStatement(c *ConnectStatementContext)

	// EnterDisconnectStatement is called when entering the disconnectStatement production.
	EnterDisconnectStatement(c *DisconnectStatementContext)

	// EnterUpdateStatement is called when entering the updateStatement production.
	EnterUpdateStatement(c *UpdateStatementContext)

	// EnterCheckStatement is called when entering the checkStatement production.
	EnterCheckStatement(c *CheckStatementContext)

	// EnterBuildStatement is called when entering the buildStatement production.
	EnterBuildStatement(c *BuildStatementContext)

	// EnterExecuteScriptStatement is called when entering the executeScriptStatement production.
	EnterExecuteScriptStatement(c *ExecuteScriptStatementContext)

	// EnterExecuteRuntimeStatement is called when entering the executeRuntimeStatement production.
	EnterExecuteRuntimeStatement(c *ExecuteRuntimeStatementContext)

	// EnterLintStatement is called when entering the lintStatement production.
	EnterLintStatement(c *LintStatementContext)

	// EnterLintTarget is called when entering the lintTarget production.
	EnterLintTarget(c *LintTargetContext)

	// EnterLintFormat is called when entering the lintFormat production.
	EnterLintFormat(c *LintFormatContext)

	// EnterUseSessionStatement is called when entering the useSessionStatement production.
	EnterUseSessionStatement(c *UseSessionStatementContext)

	// EnterSessionIdList is called when entering the sessionIdList production.
	EnterSessionIdList(c *SessionIdListContext)

	// EnterSessionId is called when entering the sessionId production.
	EnterSessionId(c *SessionIdContext)

	// EnterIntrospectApiStatement is called when entering the introspectApiStatement production.
	EnterIntrospectApiStatement(c *IntrospectApiStatementContext)

	// EnterDebugStatement is called when entering the debugStatement production.
	EnterDebugStatement(c *DebugStatementContext)

	// EnterSqlConnect is called when entering the sqlConnect production.
	EnterSqlConnect(c *SqlConnectContext)

	// EnterSqlConnectAlias is called when entering the sqlConnectAlias production.
	EnterSqlConnectAlias(c *SqlConnectAliasContext)

	// EnterSqlDisconnect is called when entering the sqlDisconnect production.
	EnterSqlDisconnect(c *SqlDisconnectContext)

	// EnterSqlConnections is called when entering the sqlConnections production.
	EnterSqlConnections(c *SqlConnectionsContext)

	// EnterSqlShowTables is called when entering the sqlShowTables production.
	EnterSqlShowTables(c *SqlShowTablesContext)

	// EnterSqlDescribeTable is called when entering the sqlDescribeTable production.
	EnterSqlDescribeTable(c *SqlDescribeTableContext)

	// EnterSqlGenerateConnector is called when entering the sqlGenerateConnector production.
	EnterSqlGenerateConnector(c *SqlGenerateConnectorContext)

	// EnterSqlQuery is called when entering the sqlQuery production.
	EnterSqlQuery(c *SqlQueryContext)

	// EnterSqlPassthrough is called when entering the sqlPassthrough production.
	EnterSqlPassthrough(c *SqlPassthroughContext)

	// EnterImportFromQuery is called when entering the importFromQuery production.
	EnterImportFromQuery(c *ImportFromQueryContext)

	// EnterImportMapping is called when entering the importMapping production.
	EnterImportMapping(c *ImportMappingContext)

	// EnterLinkLookup is called when entering the linkLookup production.
	EnterLinkLookup(c *LinkLookupContext)

	// EnterLinkDirect is called when entering the linkDirect production.
	EnterLinkDirect(c *LinkDirectContext)

	// EnterHelpStatement is called when entering the helpStatement production.
	EnterHelpStatement(c *HelpStatementContext)

	// EnterDefineFragmentStatement is called when entering the defineFragmentStatement production.
	EnterDefineFragmentStatement(c *DefineFragmentStatementContext)

	// EnterExpression is called when entering the expression production.
	EnterExpression(c *ExpressionContext)

	// EnterOrExpression is called when entering the orExpression production.
	EnterOrExpression(c *OrExpressionContext)

	// EnterAndExpression is called when entering the andExpression production.
	EnterAndExpression(c *AndExpressionContext)

	// EnterNotExpression is called when entering the notExpression production.
	EnterNotExpression(c *NotExpressionContext)

	// EnterComparisonExpression is called when entering the comparisonExpression production.
	EnterComparisonExpression(c *ComparisonExpressionContext)

	// EnterComparisonOperator is called when entering the comparisonOperator production.
	EnterComparisonOperator(c *ComparisonOperatorContext)

	// EnterAdditiveExpression is called when entering the additiveExpression production.
	EnterAdditiveExpression(c *AdditiveExpressionContext)

	// EnterMultiplicativeExpression is called when entering the multiplicativeExpression production.
	EnterMultiplicativeExpression(c *MultiplicativeExpressionContext)

	// EnterUnaryExpression is called when entering the unaryExpression production.
	EnterUnaryExpression(c *UnaryExpressionContext)

	// EnterPrimaryExpression is called when entering the primaryExpression production.
	EnterPrimaryExpression(c *PrimaryExpressionContext)

	// EnterCaseExpression is called when entering the caseExpression production.
	EnterCaseExpression(c *CaseExpressionContext)

	// EnterIfThenElseExpression is called when entering the ifThenElseExpression production.
	EnterIfThenElseExpression(c *IfThenElseExpressionContext)

	// EnterCastExpression is called when entering the castExpression production.
	EnterCastExpression(c *CastExpressionContext)

	// EnterCastDataType is called when entering the castDataType production.
	EnterCastDataType(c *CastDataTypeContext)

	// EnterAggregateFunction is called when entering the aggregateFunction production.
	EnterAggregateFunction(c *AggregateFunctionContext)

	// EnterFunctionCall is called when entering the functionCall production.
	EnterFunctionCall(c *FunctionCallContext)

	// EnterFunctionName is called when entering the functionName production.
	EnterFunctionName(c *FunctionNameContext)

	// EnterArgumentList is called when entering the argumentList production.
	EnterArgumentList(c *ArgumentListContext)

	// EnterAtomicExpression is called when entering the atomicExpression production.
	EnterAtomicExpression(c *AtomicExpressionContext)

	// EnterExpressionList is called when entering the expressionList production.
	EnterExpressionList(c *ExpressionListContext)

	// EnterQualifiedName is called when entering the qualifiedName production.
	EnterQualifiedName(c *QualifiedNameContext)

	// EnterIdentifierOrKeyword is called when entering the identifierOrKeyword production.
	EnterIdentifierOrKeyword(c *IdentifierOrKeywordContext)

	// EnterLiteral is called when entering the literal production.
	EnterLiteral(c *LiteralContext)

	// EnterArrayLiteral is called when entering the arrayLiteral production.
	EnterArrayLiteral(c *ArrayLiteralContext)

	// EnterBooleanLiteral is called when entering the booleanLiteral production.
	EnterBooleanLiteral(c *BooleanLiteralContext)

	// EnterDocComment is called when entering the docComment production.
	EnterDocComment(c *DocCommentContext)

	// EnterAnnotation is called when entering the annotation production.
	EnterAnnotation(c *AnnotationContext)

	// EnterAnnotationName is called when entering the annotationName production.
	EnterAnnotationName(c *AnnotationNameContext)

	// EnterAnnotationParams is called when entering the annotationParams production.
	EnterAnnotationParams(c *AnnotationParamsContext)

	// EnterAnnotationParam is called when entering the annotationParam production.
	EnterAnnotationParam(c *AnnotationParamContext)

	// EnterAnnotationValue is called when entering the annotationValue production.
	EnterAnnotationValue(c *AnnotationValueContext)

	// EnterCommonNameKeyword is called when entering the commonNameKeyword production.
	EnterCommonNameKeyword(c *CommonNameKeywordContext)

	// EnterKeyword is called when entering the keyword production.
	EnterKeyword(c *KeywordContext)

	// ExitProgram is called when exiting the program production.
	ExitProgram(c *ProgramContext)

	// ExitStatement is called when exiting the statement production.
	ExitStatement(c *StatementContext)

	// ExitDdlStatement is called when exiting the ddlStatement production.
	ExitDdlStatement(c *DdlStatementContext)

	// ExitUpdateWidgetsStatement is called when exiting the updateWidgetsStatement production.
	ExitUpdateWidgetsStatement(c *UpdateWidgetsStatementContext)

	// ExitCreateStatement is called when exiting the createStatement production.
	ExitCreateStatement(c *CreateStatementContext)

	// ExitAlterStatement is called when exiting the alterStatement production.
	ExitAlterStatement(c *AlterStatementContext)

	// ExitAlterStylingAction is called when exiting the alterStylingAction production.
	ExitAlterStylingAction(c *AlterStylingActionContext)

	// ExitAlterStylingAssignment is called when exiting the alterStylingAssignment production.
	ExitAlterStylingAssignment(c *AlterStylingAssignmentContext)

	// ExitAlterPageOperation is called when exiting the alterPageOperation production.
	ExitAlterPageOperation(c *AlterPageOperationContext)

	// ExitAlterPageSet is called when exiting the alterPageSet production.
	ExitAlterPageSet(c *AlterPageSetContext)

	// ExitAlterLayoutMapping is called when exiting the alterLayoutMapping production.
	ExitAlterLayoutMapping(c *AlterLayoutMappingContext)

	// ExitAlterPageAssignment is called when exiting the alterPageAssignment production.
	ExitAlterPageAssignment(c *AlterPageAssignmentContext)

	// ExitAlterPageInsert is called when exiting the alterPageInsert production.
	ExitAlterPageInsert(c *AlterPageInsertContext)

	// ExitAlterPageDrop is called when exiting the alterPageDrop production.
	ExitAlterPageDrop(c *AlterPageDropContext)

	// ExitAlterPageReplace is called when exiting the alterPageReplace production.
	ExitAlterPageReplace(c *AlterPageReplaceContext)

	// ExitAlterPageAddVariable is called when exiting the alterPageAddVariable production.
	ExitAlterPageAddVariable(c *AlterPageAddVariableContext)

	// ExitAlterPageDropVariable is called when exiting the alterPageDropVariable production.
	ExitAlterPageDropVariable(c *AlterPageDropVariableContext)

	// ExitNavigationClause is called when exiting the navigationClause production.
	ExitNavigationClause(c *NavigationClauseContext)

	// ExitNavMenuItemDef is called when exiting the navMenuItemDef production.
	ExitNavMenuItemDef(c *NavMenuItemDefContext)

	// ExitDropStatement is called when exiting the dropStatement production.
	ExitDropStatement(c *DropStatementContext)

	// ExitRenameStatement is called when exiting the renameStatement production.
	ExitRenameStatement(c *RenameStatementContext)

	// ExitMoveStatement is called when exiting the moveStatement production.
	ExitMoveStatement(c *MoveStatementContext)

	// ExitSecurityStatement is called when exiting the securityStatement production.
	ExitSecurityStatement(c *SecurityStatementContext)

	// ExitCreateModuleRoleStatement is called when exiting the createModuleRoleStatement production.
	ExitCreateModuleRoleStatement(c *CreateModuleRoleStatementContext)

	// ExitDropModuleRoleStatement is called when exiting the dropModuleRoleStatement production.
	ExitDropModuleRoleStatement(c *DropModuleRoleStatementContext)

	// ExitCreateUserRoleStatement is called when exiting the createUserRoleStatement production.
	ExitCreateUserRoleStatement(c *CreateUserRoleStatementContext)

	// ExitAlterUserRoleStatement is called when exiting the alterUserRoleStatement production.
	ExitAlterUserRoleStatement(c *AlterUserRoleStatementContext)

	// ExitDropUserRoleStatement is called when exiting the dropUserRoleStatement production.
	ExitDropUserRoleStatement(c *DropUserRoleStatementContext)

	// ExitGrantEntityAccessStatement is called when exiting the grantEntityAccessStatement production.
	ExitGrantEntityAccessStatement(c *GrantEntityAccessStatementContext)

	// ExitRevokeEntityAccessStatement is called when exiting the revokeEntityAccessStatement production.
	ExitRevokeEntityAccessStatement(c *RevokeEntityAccessStatementContext)

	// ExitGrantMicroflowAccessStatement is called when exiting the grantMicroflowAccessStatement production.
	ExitGrantMicroflowAccessStatement(c *GrantMicroflowAccessStatementContext)

	// ExitRevokeMicroflowAccessStatement is called when exiting the revokeMicroflowAccessStatement production.
	ExitRevokeMicroflowAccessStatement(c *RevokeMicroflowAccessStatementContext)

	// ExitGrantPageAccessStatement is called when exiting the grantPageAccessStatement production.
	ExitGrantPageAccessStatement(c *GrantPageAccessStatementContext)

	// ExitRevokePageAccessStatement is called when exiting the revokePageAccessStatement production.
	ExitRevokePageAccessStatement(c *RevokePageAccessStatementContext)

	// ExitGrantWorkflowAccessStatement is called when exiting the grantWorkflowAccessStatement production.
	ExitGrantWorkflowAccessStatement(c *GrantWorkflowAccessStatementContext)

	// ExitRevokeWorkflowAccessStatement is called when exiting the revokeWorkflowAccessStatement production.
	ExitRevokeWorkflowAccessStatement(c *RevokeWorkflowAccessStatementContext)

	// ExitGrantODataServiceAccessStatement is called when exiting the grantODataServiceAccessStatement production.
	ExitGrantODataServiceAccessStatement(c *GrantODataServiceAccessStatementContext)

	// ExitRevokeODataServiceAccessStatement is called when exiting the revokeODataServiceAccessStatement production.
	ExitRevokeODataServiceAccessStatement(c *RevokeODataServiceAccessStatementContext)

	// ExitAlterProjectSecurityStatement is called when exiting the alterProjectSecurityStatement production.
	ExitAlterProjectSecurityStatement(c *AlterProjectSecurityStatementContext)

	// ExitCreateDemoUserStatement is called when exiting the createDemoUserStatement production.
	ExitCreateDemoUserStatement(c *CreateDemoUserStatementContext)

	// ExitDropDemoUserStatement is called when exiting the dropDemoUserStatement production.
	ExitDropDemoUserStatement(c *DropDemoUserStatementContext)

	// ExitUpdateSecurityStatement is called when exiting the updateSecurityStatement production.
	ExitUpdateSecurityStatement(c *UpdateSecurityStatementContext)

	// ExitModuleRoleList is called when exiting the moduleRoleList production.
	ExitModuleRoleList(c *ModuleRoleListContext)

	// ExitEntityAccessRightList is called when exiting the entityAccessRightList production.
	ExitEntityAccessRightList(c *EntityAccessRightListContext)

	// ExitEntityAccessRight is called when exiting the entityAccessRight production.
	ExitEntityAccessRight(c *EntityAccessRightContext)

	// ExitCreateEntityStatement is called when exiting the createEntityStatement production.
	ExitCreateEntityStatement(c *CreateEntityStatementContext)

	// ExitGeneralizationClause is called when exiting the generalizationClause production.
	ExitGeneralizationClause(c *GeneralizationClauseContext)

	// ExitEntityBody is called when exiting the entityBody production.
	ExitEntityBody(c *EntityBodyContext)

	// ExitEntityOptions is called when exiting the entityOptions production.
	ExitEntityOptions(c *EntityOptionsContext)

	// ExitEntityOption is called when exiting the entityOption production.
	ExitEntityOption(c *EntityOptionContext)

	// ExitAttributeDefinitionList is called when exiting the attributeDefinitionList production.
	ExitAttributeDefinitionList(c *AttributeDefinitionListContext)

	// ExitAttributeDefinition is called when exiting the attributeDefinition production.
	ExitAttributeDefinition(c *AttributeDefinitionContext)

	// ExitAttributeName is called when exiting the attributeName production.
	ExitAttributeName(c *AttributeNameContext)

	// ExitAttributeConstraint is called when exiting the attributeConstraint production.
	ExitAttributeConstraint(c *AttributeConstraintContext)

	// ExitDataType is called when exiting the dataType production.
	ExitDataType(c *DataTypeContext)

	// ExitTemplateContext is called when exiting the templateContext production.
	ExitTemplateContext(c *TemplateContextContext)

	// ExitNonListDataType is called when exiting the nonListDataType production.
	ExitNonListDataType(c *NonListDataTypeContext)

	// ExitIndexDefinition is called when exiting the indexDefinition production.
	ExitIndexDefinition(c *IndexDefinitionContext)

	// ExitIndexAttributeList is called when exiting the indexAttributeList production.
	ExitIndexAttributeList(c *IndexAttributeListContext)

	// ExitIndexAttribute is called when exiting the indexAttribute production.
	ExitIndexAttribute(c *IndexAttributeContext)

	// ExitIndexColumnName is called when exiting the indexColumnName production.
	ExitIndexColumnName(c *IndexColumnNameContext)

	// ExitCreateAssociationStatement is called when exiting the createAssociationStatement production.
	ExitCreateAssociationStatement(c *CreateAssociationStatementContext)

	// ExitAssociationOptions is called when exiting the associationOptions production.
	ExitAssociationOptions(c *AssociationOptionsContext)

	// ExitAssociationOption is called when exiting the associationOption production.
	ExitAssociationOption(c *AssociationOptionContext)

	// ExitDeleteBehavior is called when exiting the deleteBehavior production.
	ExitDeleteBehavior(c *DeleteBehaviorContext)

	// ExitAlterEntityAction is called when exiting the alterEntityAction production.
	ExitAlterEntityAction(c *AlterEntityActionContext)

	// ExitAlterAssociationAction is called when exiting the alterAssociationAction production.
	ExitAlterAssociationAction(c *AlterAssociationActionContext)

	// ExitAlterEnumerationAction is called when exiting the alterEnumerationAction production.
	ExitAlterEnumerationAction(c *AlterEnumerationActionContext)

	// ExitAlterNotebookAction is called when exiting the alterNotebookAction production.
	ExitAlterNotebookAction(c *AlterNotebookActionContext)

	// ExitCreateModuleStatement is called when exiting the createModuleStatement production.
	ExitCreateModuleStatement(c *CreateModuleStatementContext)

	// ExitModuleOptions is called when exiting the moduleOptions production.
	ExitModuleOptions(c *ModuleOptionsContext)

	// ExitModuleOption is called when exiting the moduleOption production.
	ExitModuleOption(c *ModuleOptionContext)

	// ExitCreateEnumerationStatement is called when exiting the createEnumerationStatement production.
	ExitCreateEnumerationStatement(c *CreateEnumerationStatementContext)

	// ExitEnumerationValueList is called when exiting the enumerationValueList production.
	ExitEnumerationValueList(c *EnumerationValueListContext)

	// ExitEnumerationValue is called when exiting the enumerationValue production.
	ExitEnumerationValue(c *EnumerationValueContext)

	// ExitEnumValueName is called when exiting the enumValueName production.
	ExitEnumValueName(c *EnumValueNameContext)

	// ExitEnumerationOptions is called when exiting the enumerationOptions production.
	ExitEnumerationOptions(c *EnumerationOptionsContext)

	// ExitEnumerationOption is called when exiting the enumerationOption production.
	ExitEnumerationOption(c *EnumerationOptionContext)

	// ExitCreateImageCollectionStatement is called when exiting the createImageCollectionStatement production.
	ExitCreateImageCollectionStatement(c *CreateImageCollectionStatementContext)

	// ExitImageCollectionOptions is called when exiting the imageCollectionOptions production.
	ExitImageCollectionOptions(c *ImageCollectionOptionsContext)

	// ExitImageCollectionOption is called when exiting the imageCollectionOption production.
	ExitImageCollectionOption(c *ImageCollectionOptionContext)

	// ExitImageCollectionBody is called when exiting the imageCollectionBody production.
	ExitImageCollectionBody(c *ImageCollectionBodyContext)

	// ExitImageCollectionItem is called when exiting the imageCollectionItem production.
	ExitImageCollectionItem(c *ImageCollectionItemContext)

	// ExitImageName is called when exiting the imageName production.
	ExitImageName(c *ImageNameContext)

	// ExitCreateJsonStructureStatement is called when exiting the createJsonStructureStatement production.
	ExitCreateJsonStructureStatement(c *CreateJsonStructureStatementContext)

	// ExitCustomNameMapping is called when exiting the customNameMapping production.
	ExitCustomNameMapping(c *CustomNameMappingContext)

	// ExitCreateValidationRuleStatement is called when exiting the createValidationRuleStatement production.
	ExitCreateValidationRuleStatement(c *CreateValidationRuleStatementContext)

	// ExitValidationRuleBody is called when exiting the validationRuleBody production.
	ExitValidationRuleBody(c *ValidationRuleBodyContext)

	// ExitRangeConstraint is called when exiting the rangeConstraint production.
	ExitRangeConstraint(c *RangeConstraintContext)

	// ExitAttributeReference is called when exiting the attributeReference production.
	ExitAttributeReference(c *AttributeReferenceContext)

	// ExitAttributeReferenceList is called when exiting the attributeReferenceList production.
	ExitAttributeReferenceList(c *AttributeReferenceListContext)

	// ExitCreateMicroflowStatement is called when exiting the createMicroflowStatement production.
	ExitCreateMicroflowStatement(c *CreateMicroflowStatementContext)

	// ExitCreateJavaActionStatement is called when exiting the createJavaActionStatement production.
	ExitCreateJavaActionStatement(c *CreateJavaActionStatementContext)

	// ExitJavaActionParameterList is called when exiting the javaActionParameterList production.
	ExitJavaActionParameterList(c *JavaActionParameterListContext)

	// ExitJavaActionParameter is called when exiting the javaActionParameter production.
	ExitJavaActionParameter(c *JavaActionParameterContext)

	// ExitJavaActionReturnType is called when exiting the javaActionReturnType production.
	ExitJavaActionReturnType(c *JavaActionReturnTypeContext)

	// ExitJavaActionExposedClause is called when exiting the javaActionExposedClause production.
	ExitJavaActionExposedClause(c *JavaActionExposedClauseContext)

	// ExitMicroflowParameterList is called when exiting the microflowParameterList production.
	ExitMicroflowParameterList(c *MicroflowParameterListContext)

	// ExitMicroflowParameter is called when exiting the microflowParameter production.
	ExitMicroflowParameter(c *MicroflowParameterContext)

	// ExitParameterName is called when exiting the parameterName production.
	ExitParameterName(c *ParameterNameContext)

	// ExitMicroflowReturnType is called when exiting the microflowReturnType production.
	ExitMicroflowReturnType(c *MicroflowReturnTypeContext)

	// ExitMicroflowOptions is called when exiting the microflowOptions production.
	ExitMicroflowOptions(c *MicroflowOptionsContext)

	// ExitMicroflowOption is called when exiting the microflowOption production.
	ExitMicroflowOption(c *MicroflowOptionContext)

	// ExitMicroflowBody is called when exiting the microflowBody production.
	ExitMicroflowBody(c *MicroflowBodyContext)

	// ExitMicroflowStatement is called when exiting the microflowStatement production.
	ExitMicroflowStatement(c *MicroflowStatementContext)

	// ExitDeclareStatement is called when exiting the declareStatement production.
	ExitDeclareStatement(c *DeclareStatementContext)

	// ExitSetStatement is called when exiting the setStatement production.
	ExitSetStatement(c *SetStatementContext)

	// ExitCreateObjectStatement is called when exiting the createObjectStatement production.
	ExitCreateObjectStatement(c *CreateObjectStatementContext)

	// ExitChangeObjectStatement is called when exiting the changeObjectStatement production.
	ExitChangeObjectStatement(c *ChangeObjectStatementContext)

	// ExitAttributePath is called when exiting the attributePath production.
	ExitAttributePath(c *AttributePathContext)

	// ExitCommitStatement is called when exiting the commitStatement production.
	ExitCommitStatement(c *CommitStatementContext)

	// ExitDeleteObjectStatement is called when exiting the deleteObjectStatement production.
	ExitDeleteObjectStatement(c *DeleteObjectStatementContext)

	// ExitRollbackStatement is called when exiting the rollbackStatement production.
	ExitRollbackStatement(c *RollbackStatementContext)

	// ExitRetrieveStatement is called when exiting the retrieveStatement production.
	ExitRetrieveStatement(c *RetrieveStatementContext)

	// ExitRetrieveSource is called when exiting the retrieveSource production.
	ExitRetrieveSource(c *RetrieveSourceContext)

	// ExitOnErrorClause is called when exiting the onErrorClause production.
	ExitOnErrorClause(c *OnErrorClauseContext)

	// ExitIfStatement is called when exiting the ifStatement production.
	ExitIfStatement(c *IfStatementContext)

	// ExitLoopStatement is called when exiting the loopStatement production.
	ExitLoopStatement(c *LoopStatementContext)

	// ExitWhileStatement is called when exiting the whileStatement production.
	ExitWhileStatement(c *WhileStatementContext)

	// ExitContinueStatement is called when exiting the continueStatement production.
	ExitContinueStatement(c *ContinueStatementContext)

	// ExitBreakStatement is called when exiting the breakStatement production.
	ExitBreakStatement(c *BreakStatementContext)

	// ExitReturnStatement is called when exiting the returnStatement production.
	ExitReturnStatement(c *ReturnStatementContext)

	// ExitRaiseErrorStatement is called when exiting the raiseErrorStatement production.
	ExitRaiseErrorStatement(c *RaiseErrorStatementContext)

	// ExitLogStatement is called when exiting the logStatement production.
	ExitLogStatement(c *LogStatementContext)

	// ExitLogLevel is called when exiting the logLevel production.
	ExitLogLevel(c *LogLevelContext)

	// ExitTemplateParams is called when exiting the templateParams production.
	ExitTemplateParams(c *TemplateParamsContext)

	// ExitTemplateParam is called when exiting the templateParam production.
	ExitTemplateParam(c *TemplateParamContext)

	// ExitLogTemplateParams is called when exiting the logTemplateParams production.
	ExitLogTemplateParams(c *LogTemplateParamsContext)

	// ExitLogTemplateParam is called when exiting the logTemplateParam production.
	ExitLogTemplateParam(c *LogTemplateParamContext)

	// ExitCallMicroflowStatement is called when exiting the callMicroflowStatement production.
	ExitCallMicroflowStatement(c *CallMicroflowStatementContext)

	// ExitCallJavaActionStatement is called when exiting the callJavaActionStatement production.
	ExitCallJavaActionStatement(c *CallJavaActionStatementContext)

	// ExitExecuteDatabaseQueryStatement is called when exiting the executeDatabaseQueryStatement production.
	ExitExecuteDatabaseQueryStatement(c *ExecuteDatabaseQueryStatementContext)

	// ExitCallExternalActionStatement is called when exiting the callExternalActionStatement production.
	ExitCallExternalActionStatement(c *CallExternalActionStatementContext)

	// ExitCallArgumentList is called when exiting the callArgumentList production.
	ExitCallArgumentList(c *CallArgumentListContext)

	// ExitCallArgument is called when exiting the callArgument production.
	ExitCallArgument(c *CallArgumentContext)

	// ExitShowPageStatement is called when exiting the showPageStatement production.
	ExitShowPageStatement(c *ShowPageStatementContext)

	// ExitShowPageArgList is called when exiting the showPageArgList production.
	ExitShowPageArgList(c *ShowPageArgListContext)

	// ExitShowPageArg is called when exiting the showPageArg production.
	ExitShowPageArg(c *ShowPageArgContext)

	// ExitClosePageStatement is called when exiting the closePageStatement production.
	ExitClosePageStatement(c *ClosePageStatementContext)

	// ExitShowHomePageStatement is called when exiting the showHomePageStatement production.
	ExitShowHomePageStatement(c *ShowHomePageStatementContext)

	// ExitShowMessageStatement is called when exiting the showMessageStatement production.
	ExitShowMessageStatement(c *ShowMessageStatementContext)

	// ExitThrowStatement is called when exiting the throwStatement production.
	ExitThrowStatement(c *ThrowStatementContext)

	// ExitValidationFeedbackStatement is called when exiting the validationFeedbackStatement production.
	ExitValidationFeedbackStatement(c *ValidationFeedbackStatementContext)

	// ExitRestCallStatement is called when exiting the restCallStatement production.
	ExitRestCallStatement(c *RestCallStatementContext)

	// ExitHttpMethod is called when exiting the httpMethod production.
	ExitHttpMethod(c *HttpMethodContext)

	// ExitRestCallUrl is called when exiting the restCallUrl production.
	ExitRestCallUrl(c *RestCallUrlContext)

	// ExitRestCallUrlParams is called when exiting the restCallUrlParams production.
	ExitRestCallUrlParams(c *RestCallUrlParamsContext)

	// ExitRestCallHeaderClause is called when exiting the restCallHeaderClause production.
	ExitRestCallHeaderClause(c *RestCallHeaderClauseContext)

	// ExitRestCallAuthClause is called when exiting the restCallAuthClause production.
	ExitRestCallAuthClause(c *RestCallAuthClauseContext)

	// ExitRestCallBodyClause is called when exiting the restCallBodyClause production.
	ExitRestCallBodyClause(c *RestCallBodyClauseContext)

	// ExitRestCallTimeoutClause is called when exiting the restCallTimeoutClause production.
	ExitRestCallTimeoutClause(c *RestCallTimeoutClauseContext)

	// ExitRestCallReturnsClause is called when exiting the restCallReturnsClause production.
	ExitRestCallReturnsClause(c *RestCallReturnsClauseContext)

	// ExitSendRestRequestStatement is called when exiting the sendRestRequestStatement production.
	ExitSendRestRequestStatement(c *SendRestRequestStatementContext)

	// ExitSendRestRequestBodyClause is called when exiting the sendRestRequestBodyClause production.
	ExitSendRestRequestBodyClause(c *SendRestRequestBodyClauseContext)

	// ExitListOperationStatement is called when exiting the listOperationStatement production.
	ExitListOperationStatement(c *ListOperationStatementContext)

	// ExitListOperation is called when exiting the listOperation production.
	ExitListOperation(c *ListOperationContext)

	// ExitSortSpecList is called when exiting the sortSpecList production.
	ExitSortSpecList(c *SortSpecListContext)

	// ExitSortSpec is called when exiting the sortSpec production.
	ExitSortSpec(c *SortSpecContext)

	// ExitAggregateListStatement is called when exiting the aggregateListStatement production.
	ExitAggregateListStatement(c *AggregateListStatementContext)

	// ExitListAggregateOperation is called when exiting the listAggregateOperation production.
	ExitListAggregateOperation(c *ListAggregateOperationContext)

	// ExitCreateListStatement is called when exiting the createListStatement production.
	ExitCreateListStatement(c *CreateListStatementContext)

	// ExitAddToListStatement is called when exiting the addToListStatement production.
	ExitAddToListStatement(c *AddToListStatementContext)

	// ExitRemoveFromListStatement is called when exiting the removeFromListStatement production.
	ExitRemoveFromListStatement(c *RemoveFromListStatementContext)

	// ExitMemberAssignmentList is called when exiting the memberAssignmentList production.
	ExitMemberAssignmentList(c *MemberAssignmentListContext)

	// ExitMemberAssignment is called when exiting the memberAssignment production.
	ExitMemberAssignment(c *MemberAssignmentContext)

	// ExitMemberAttributeName is called when exiting the memberAttributeName production.
	ExitMemberAttributeName(c *MemberAttributeNameContext)

	// ExitChangeList is called when exiting the changeList production.
	ExitChangeList(c *ChangeListContext)

	// ExitChangeItem is called when exiting the changeItem production.
	ExitChangeItem(c *ChangeItemContext)

	// ExitCreatePageStatement is called when exiting the createPageStatement production.
	ExitCreatePageStatement(c *CreatePageStatementContext)

	// ExitCreateSnippetStatement is called when exiting the createSnippetStatement production.
	ExitCreateSnippetStatement(c *CreateSnippetStatementContext)

	// ExitSnippetOptions is called when exiting the snippetOptions production.
	ExitSnippetOptions(c *SnippetOptionsContext)

	// ExitSnippetOption is called when exiting the snippetOption production.
	ExitSnippetOption(c *SnippetOptionContext)

	// ExitPageParameterList is called when exiting the pageParameterList production.
	ExitPageParameterList(c *PageParameterListContext)

	// ExitPageParameter is called when exiting the pageParameter production.
	ExitPageParameter(c *PageParameterContext)

	// ExitSnippetParameterList is called when exiting the snippetParameterList production.
	ExitSnippetParameterList(c *SnippetParameterListContext)

	// ExitSnippetParameter is called when exiting the snippetParameter production.
	ExitSnippetParameter(c *SnippetParameterContext)

	// ExitVariableDeclarationList is called when exiting the variableDeclarationList production.
	ExitVariableDeclarationList(c *VariableDeclarationListContext)

	// ExitVariableDeclaration is called when exiting the variableDeclaration production.
	ExitVariableDeclaration(c *VariableDeclarationContext)

	// ExitSortColumn is called when exiting the sortColumn production.
	ExitSortColumn(c *SortColumnContext)

	// ExitXpathConstraint is called when exiting the xpathConstraint production.
	ExitXpathConstraint(c *XpathConstraintContext)

	// ExitAndOrXpath is called when exiting the andOrXpath production.
	ExitAndOrXpath(c *AndOrXpathContext)

	// ExitXpathExpr is called when exiting the xpathExpr production.
	ExitXpathExpr(c *XpathExprContext)

	// ExitXpathAndExpr is called when exiting the xpathAndExpr production.
	ExitXpathAndExpr(c *XpathAndExprContext)

	// ExitXpathNotExpr is called when exiting the xpathNotExpr production.
	ExitXpathNotExpr(c *XpathNotExprContext)

	// ExitXpathComparisonExpr is called when exiting the xpathComparisonExpr production.
	ExitXpathComparisonExpr(c *XpathComparisonExprContext)

	// ExitXpathValueExpr is called when exiting the xpathValueExpr production.
	ExitXpathValueExpr(c *XpathValueExprContext)

	// ExitXpathPath is called when exiting the xpathPath production.
	ExitXpathPath(c *XpathPathContext)

	// ExitXpathStep is called when exiting the xpathStep production.
	ExitXpathStep(c *XpathStepContext)

	// ExitXpathStepValue is called when exiting the xpathStepValue production.
	ExitXpathStepValue(c *XpathStepValueContext)

	// ExitXpathQualifiedName is called when exiting the xpathQualifiedName production.
	ExitXpathQualifiedName(c *XpathQualifiedNameContext)

	// ExitXpathWord is called when exiting the xpathWord production.
	ExitXpathWord(c *XpathWordContext)

	// ExitXpathFunctionCall is called when exiting the xpathFunctionCall production.
	ExitXpathFunctionCall(c *XpathFunctionCallContext)

	// ExitXpathFunctionName is called when exiting the xpathFunctionName production.
	ExitXpathFunctionName(c *XpathFunctionNameContext)

	// ExitPageHeaderV3 is called when exiting the pageHeaderV3 production.
	ExitPageHeaderV3(c *PageHeaderV3Context)

	// ExitPageHeaderPropertyV3 is called when exiting the pageHeaderPropertyV3 production.
	ExitPageHeaderPropertyV3(c *PageHeaderPropertyV3Context)

	// ExitSnippetHeaderV3 is called when exiting the snippetHeaderV3 production.
	ExitSnippetHeaderV3(c *SnippetHeaderV3Context)

	// ExitSnippetHeaderPropertyV3 is called when exiting the snippetHeaderPropertyV3 production.
	ExitSnippetHeaderPropertyV3(c *SnippetHeaderPropertyV3Context)

	// ExitPageBodyV3 is called when exiting the pageBodyV3 production.
	ExitPageBodyV3(c *PageBodyV3Context)

	// ExitUseFragmentRef is called when exiting the useFragmentRef production.
	ExitUseFragmentRef(c *UseFragmentRefContext)

	// ExitWidgetV3 is called when exiting the widgetV3 production.
	ExitWidgetV3(c *WidgetV3Context)

	// ExitWidgetTypeV3 is called when exiting the widgetTypeV3 production.
	ExitWidgetTypeV3(c *WidgetTypeV3Context)

	// ExitWidgetPropertiesV3 is called when exiting the widgetPropertiesV3 production.
	ExitWidgetPropertiesV3(c *WidgetPropertiesV3Context)

	// ExitWidgetPropertyV3 is called when exiting the widgetPropertyV3 production.
	ExitWidgetPropertyV3(c *WidgetPropertyV3Context)

	// ExitFilterTypeValue is called when exiting the filterTypeValue production.
	ExitFilterTypeValue(c *FilterTypeValueContext)

	// ExitAttributeListV3 is called when exiting the attributeListV3 production.
	ExitAttributeListV3(c *AttributeListV3Context)

	// ExitDataSourceExprV3 is called when exiting the dataSourceExprV3 production.
	ExitDataSourceExprV3(c *DataSourceExprV3Context)

	// ExitActionExprV3 is called when exiting the actionExprV3 production.
	ExitActionExprV3(c *ActionExprV3Context)

	// ExitMicroflowArgsV3 is called when exiting the microflowArgsV3 production.
	ExitMicroflowArgsV3(c *MicroflowArgsV3Context)

	// ExitMicroflowArgV3 is called when exiting the microflowArgV3 production.
	ExitMicroflowArgV3(c *MicroflowArgV3Context)

	// ExitAttributePathV3 is called when exiting the attributePathV3 production.
	ExitAttributePathV3(c *AttributePathV3Context)

	// ExitStringExprV3 is called when exiting the stringExprV3 production.
	ExitStringExprV3(c *StringExprV3Context)

	// ExitParamListV3 is called when exiting the paramListV3 production.
	ExitParamListV3(c *ParamListV3Context)

	// ExitParamAssignmentV3 is called when exiting the paramAssignmentV3 production.
	ExitParamAssignmentV3(c *ParamAssignmentV3Context)

	// ExitRenderModeV3 is called when exiting the renderModeV3 production.
	ExitRenderModeV3(c *RenderModeV3Context)

	// ExitButtonStyleV3 is called when exiting the buttonStyleV3 production.
	ExitButtonStyleV3(c *ButtonStyleV3Context)

	// ExitDesktopWidthV3 is called when exiting the desktopWidthV3 production.
	ExitDesktopWidthV3(c *DesktopWidthV3Context)

	// ExitSelectionModeV3 is called when exiting the selectionModeV3 production.
	ExitSelectionModeV3(c *SelectionModeV3Context)

	// ExitPropertyValueV3 is called when exiting the propertyValueV3 production.
	ExitPropertyValueV3(c *PropertyValueV3Context)

	// ExitDesignPropertyListV3 is called when exiting the designPropertyListV3 production.
	ExitDesignPropertyListV3(c *DesignPropertyListV3Context)

	// ExitDesignPropertyEntryV3 is called when exiting the designPropertyEntryV3 production.
	ExitDesignPropertyEntryV3(c *DesignPropertyEntryV3Context)

	// ExitWidgetBodyV3 is called when exiting the widgetBodyV3 production.
	ExitWidgetBodyV3(c *WidgetBodyV3Context)

	// ExitCreateNotebookStatement is called when exiting the createNotebookStatement production.
	ExitCreateNotebookStatement(c *CreateNotebookStatementContext)

	// ExitNotebookOptions is called when exiting the notebookOptions production.
	ExitNotebookOptions(c *NotebookOptionsContext)

	// ExitNotebookOption is called when exiting the notebookOption production.
	ExitNotebookOption(c *NotebookOptionContext)

	// ExitNotebookPage is called when exiting the notebookPage production.
	ExitNotebookPage(c *NotebookPageContext)

	// ExitCreateDatabaseConnectionStatement is called when exiting the createDatabaseConnectionStatement production.
	ExitCreateDatabaseConnectionStatement(c *CreateDatabaseConnectionStatementContext)

	// ExitDatabaseConnectionOption is called when exiting the databaseConnectionOption production.
	ExitDatabaseConnectionOption(c *DatabaseConnectionOptionContext)

	// ExitDatabaseQuery is called when exiting the databaseQuery production.
	ExitDatabaseQuery(c *DatabaseQueryContext)

	// ExitDatabaseQueryMapping is called when exiting the databaseQueryMapping production.
	ExitDatabaseQueryMapping(c *DatabaseQueryMappingContext)

	// ExitCreateConstantStatement is called when exiting the createConstantStatement production.
	ExitCreateConstantStatement(c *CreateConstantStatementContext)

	// ExitConstantOptions is called when exiting the constantOptions production.
	ExitConstantOptions(c *ConstantOptionsContext)

	// ExitConstantOption is called when exiting the constantOption production.
	ExitConstantOption(c *ConstantOptionContext)

	// ExitCreateConfigurationStatement is called when exiting the createConfigurationStatement production.
	ExitCreateConfigurationStatement(c *CreateConfigurationStatementContext)

	// ExitCreateRestClientStatement is called when exiting the createRestClientStatement production.
	ExitCreateRestClientStatement(c *CreateRestClientStatementContext)

	// ExitRestClientBaseUrl is called when exiting the restClientBaseUrl production.
	ExitRestClientBaseUrl(c *RestClientBaseUrlContext)

	// ExitRestClientAuthentication is called when exiting the restClientAuthentication production.
	ExitRestClientAuthentication(c *RestClientAuthenticationContext)

	// ExitRestAuthValue is called when exiting the restAuthValue production.
	ExitRestAuthValue(c *RestAuthValueContext)

	// ExitRestOperationDef is called when exiting the restOperationDef production.
	ExitRestOperationDef(c *RestOperationDefContext)

	// ExitRestHttpMethod is called when exiting the restHttpMethod production.
	ExitRestHttpMethod(c *RestHttpMethodContext)

	// ExitRestOperationClause is called when exiting the restOperationClause production.
	ExitRestOperationClause(c *RestOperationClauseContext)

	// ExitRestHeaderValue is called when exiting the restHeaderValue production.
	ExitRestHeaderValue(c *RestHeaderValueContext)

	// ExitRestResponseSpec is called when exiting the restResponseSpec production.
	ExitRestResponseSpec(c *RestResponseSpecContext)

	// ExitCreateIndexStatement is called when exiting the createIndexStatement production.
	ExitCreateIndexStatement(c *CreateIndexStatementContext)

	// ExitCreateODataClientStatement is called when exiting the createODataClientStatement production.
	ExitCreateODataClientStatement(c *CreateODataClientStatementContext)

	// ExitCreateODataServiceStatement is called when exiting the createODataServiceStatement production.
	ExitCreateODataServiceStatement(c *CreateODataServiceStatementContext)

	// ExitOdataPropertyValue is called when exiting the odataPropertyValue production.
	ExitOdataPropertyValue(c *OdataPropertyValueContext)

	// ExitOdataPropertyAssignment is called when exiting the odataPropertyAssignment production.
	ExitOdataPropertyAssignment(c *OdataPropertyAssignmentContext)

	// ExitOdataAlterAssignment is called when exiting the odataAlterAssignment production.
	ExitOdataAlterAssignment(c *OdataAlterAssignmentContext)

	// ExitOdataAuthenticationClause is called when exiting the odataAuthenticationClause production.
	ExitOdataAuthenticationClause(c *OdataAuthenticationClauseContext)

	// ExitOdataAuthType is called when exiting the odataAuthType production.
	ExitOdataAuthType(c *OdataAuthTypeContext)

	// ExitPublishEntityBlock is called when exiting the publishEntityBlock production.
	ExitPublishEntityBlock(c *PublishEntityBlockContext)

	// ExitExposeClause is called when exiting the exposeClause production.
	ExitExposeClause(c *ExposeClauseContext)

	// ExitExposeMember is called when exiting the exposeMember production.
	ExitExposeMember(c *ExposeMemberContext)

	// ExitExposeMemberOptions is called when exiting the exposeMemberOptions production.
	ExitExposeMemberOptions(c *ExposeMemberOptionsContext)

	// ExitCreateExternalEntityStatement is called when exiting the createExternalEntityStatement production.
	ExitCreateExternalEntityStatement(c *CreateExternalEntityStatementContext)

	// ExitCreateNavigationStatement is called when exiting the createNavigationStatement production.
	ExitCreateNavigationStatement(c *CreateNavigationStatementContext)

	// ExitOdataHeadersClause is called when exiting the odataHeadersClause production.
	ExitOdataHeadersClause(c *OdataHeadersClauseContext)

	// ExitOdataHeaderEntry is called when exiting the odataHeaderEntry production.
	ExitOdataHeaderEntry(c *OdataHeaderEntryContext)

	// ExitCreateBusinessEventServiceStatement is called when exiting the createBusinessEventServiceStatement production.
	ExitCreateBusinessEventServiceStatement(c *CreateBusinessEventServiceStatementContext)

	// ExitBusinessEventMessageDef is called when exiting the businessEventMessageDef production.
	ExitBusinessEventMessageDef(c *BusinessEventMessageDefContext)

	// ExitBusinessEventAttrDef is called when exiting the businessEventAttrDef production.
	ExitBusinessEventAttrDef(c *BusinessEventAttrDefContext)

	// ExitCreateWorkflowStatement is called when exiting the createWorkflowStatement production.
	ExitCreateWorkflowStatement(c *CreateWorkflowStatementContext)

	// ExitWorkflowBody is called when exiting the workflowBody production.
	ExitWorkflowBody(c *WorkflowBodyContext)

	// ExitWorkflowActivityStmt is called when exiting the workflowActivityStmt production.
	ExitWorkflowActivityStmt(c *WorkflowActivityStmtContext)

	// ExitWorkflowUserTaskStmt is called when exiting the workflowUserTaskStmt production.
	ExitWorkflowUserTaskStmt(c *WorkflowUserTaskStmtContext)

	// ExitWorkflowBoundaryEventClause is called when exiting the workflowBoundaryEventClause production.
	ExitWorkflowBoundaryEventClause(c *WorkflowBoundaryEventClauseContext)

	// ExitWorkflowUserTaskOutcome is called when exiting the workflowUserTaskOutcome production.
	ExitWorkflowUserTaskOutcome(c *WorkflowUserTaskOutcomeContext)

	// ExitWorkflowCallMicroflowStmt is called when exiting the workflowCallMicroflowStmt production.
	ExitWorkflowCallMicroflowStmt(c *WorkflowCallMicroflowStmtContext)

	// ExitWorkflowParameterMapping is called when exiting the workflowParameterMapping production.
	ExitWorkflowParameterMapping(c *WorkflowParameterMappingContext)

	// ExitWorkflowCallWorkflowStmt is called when exiting the workflowCallWorkflowStmt production.
	ExitWorkflowCallWorkflowStmt(c *WorkflowCallWorkflowStmtContext)

	// ExitWorkflowDecisionStmt is called when exiting the workflowDecisionStmt production.
	ExitWorkflowDecisionStmt(c *WorkflowDecisionStmtContext)

	// ExitWorkflowConditionOutcome is called when exiting the workflowConditionOutcome production.
	ExitWorkflowConditionOutcome(c *WorkflowConditionOutcomeContext)

	// ExitWorkflowParallelSplitStmt is called when exiting the workflowParallelSplitStmt production.
	ExitWorkflowParallelSplitStmt(c *WorkflowParallelSplitStmtContext)

	// ExitWorkflowParallelPath is called when exiting the workflowParallelPath production.
	ExitWorkflowParallelPath(c *WorkflowParallelPathContext)

	// ExitWorkflowJumpToStmt is called when exiting the workflowJumpToStmt production.
	ExitWorkflowJumpToStmt(c *WorkflowJumpToStmtContext)

	// ExitWorkflowWaitForTimerStmt is called when exiting the workflowWaitForTimerStmt production.
	ExitWorkflowWaitForTimerStmt(c *WorkflowWaitForTimerStmtContext)

	// ExitWorkflowWaitForNotificationStmt is called when exiting the workflowWaitForNotificationStmt production.
	ExitWorkflowWaitForNotificationStmt(c *WorkflowWaitForNotificationStmtContext)

	// ExitWorkflowAnnotationStmt is called when exiting the workflowAnnotationStmt production.
	ExitWorkflowAnnotationStmt(c *WorkflowAnnotationStmtContext)

	// ExitAlterSettingsClause is called when exiting the alterSettingsClause production.
	ExitAlterSettingsClause(c *AlterSettingsClauseContext)

	// ExitSettingsSection is called when exiting the settingsSection production.
	ExitSettingsSection(c *SettingsSectionContext)

	// ExitSettingsAssignment is called when exiting the settingsAssignment production.
	ExitSettingsAssignment(c *SettingsAssignmentContext)

	// ExitSettingsValue is called when exiting the settingsValue production.
	ExitSettingsValue(c *SettingsValueContext)

	// ExitDqlStatement is called when exiting the dqlStatement production.
	ExitDqlStatement(c *DqlStatementContext)

	// ExitShowStatement is called when exiting the showStatement production.
	ExitShowStatement(c *ShowStatementContext)

	// ExitShowWidgetsFilter is called when exiting the showWidgetsFilter production.
	ExitShowWidgetsFilter(c *ShowWidgetsFilterContext)

	// ExitWidgetTypeKeyword is called when exiting the widgetTypeKeyword production.
	ExitWidgetTypeKeyword(c *WidgetTypeKeywordContext)

	// ExitWidgetCondition is called when exiting the widgetCondition production.
	ExitWidgetCondition(c *WidgetConditionContext)

	// ExitWidgetPropertyAssignment is called when exiting the widgetPropertyAssignment production.
	ExitWidgetPropertyAssignment(c *WidgetPropertyAssignmentContext)

	// ExitWidgetPropertyValue is called when exiting the widgetPropertyValue production.
	ExitWidgetPropertyValue(c *WidgetPropertyValueContext)

	// ExitDescribeStatement is called when exiting the describeStatement production.
	ExitDescribeStatement(c *DescribeStatementContext)

	// ExitCatalogSelectQuery is called when exiting the catalogSelectQuery production.
	ExitCatalogSelectQuery(c *CatalogSelectQueryContext)

	// ExitCatalogJoinClause is called when exiting the catalogJoinClause production.
	ExitCatalogJoinClause(c *CatalogJoinClauseContext)

	// ExitCatalogTableName is called when exiting the catalogTableName production.
	ExitCatalogTableName(c *CatalogTableNameContext)

	// ExitOqlQuery is called when exiting the oqlQuery production.
	ExitOqlQuery(c *OqlQueryContext)

	// ExitOqlQueryTerm is called when exiting the oqlQueryTerm production.
	ExitOqlQueryTerm(c *OqlQueryTermContext)

	// ExitSelectClause is called when exiting the selectClause production.
	ExitSelectClause(c *SelectClauseContext)

	// ExitSelectList is called when exiting the selectList production.
	ExitSelectList(c *SelectListContext)

	// ExitSelectItem is called when exiting the selectItem production.
	ExitSelectItem(c *SelectItemContext)

	// ExitSelectAlias is called when exiting the selectAlias production.
	ExitSelectAlias(c *SelectAliasContext)

	// ExitFromClause is called when exiting the fromClause production.
	ExitFromClause(c *FromClauseContext)

	// ExitTableReference is called when exiting the tableReference production.
	ExitTableReference(c *TableReferenceContext)

	// ExitJoinClause is called when exiting the joinClause production.
	ExitJoinClause(c *JoinClauseContext)

	// ExitAssociationPath is called when exiting the associationPath production.
	ExitAssociationPath(c *AssociationPathContext)

	// ExitJoinType is called when exiting the joinType production.
	ExitJoinType(c *JoinTypeContext)

	// ExitWhereClause is called when exiting the whereClause production.
	ExitWhereClause(c *WhereClauseContext)

	// ExitGroupByClause is called when exiting the groupByClause production.
	ExitGroupByClause(c *GroupByClauseContext)

	// ExitHavingClause is called when exiting the havingClause production.
	ExitHavingClause(c *HavingClauseContext)

	// ExitOrderByClause is called when exiting the orderByClause production.
	ExitOrderByClause(c *OrderByClauseContext)

	// ExitOrderByList is called when exiting the orderByList production.
	ExitOrderByList(c *OrderByListContext)

	// ExitOrderByItem is called when exiting the orderByItem production.
	ExitOrderByItem(c *OrderByItemContext)

	// ExitGroupByList is called when exiting the groupByList production.
	ExitGroupByList(c *GroupByListContext)

	// ExitLimitOffsetClause is called when exiting the limitOffsetClause production.
	ExitLimitOffsetClause(c *LimitOffsetClauseContext)

	// ExitUtilityStatement is called when exiting the utilityStatement production.
	ExitUtilityStatement(c *UtilityStatementContext)

	// ExitSearchStatement is called when exiting the searchStatement production.
	ExitSearchStatement(c *SearchStatementContext)

	// ExitConnectStatement is called when exiting the connectStatement production.
	ExitConnectStatement(c *ConnectStatementContext)

	// ExitDisconnectStatement is called when exiting the disconnectStatement production.
	ExitDisconnectStatement(c *DisconnectStatementContext)

	// ExitUpdateStatement is called when exiting the updateStatement production.
	ExitUpdateStatement(c *UpdateStatementContext)

	// ExitCheckStatement is called when exiting the checkStatement production.
	ExitCheckStatement(c *CheckStatementContext)

	// ExitBuildStatement is called when exiting the buildStatement production.
	ExitBuildStatement(c *BuildStatementContext)

	// ExitExecuteScriptStatement is called when exiting the executeScriptStatement production.
	ExitExecuteScriptStatement(c *ExecuteScriptStatementContext)

	// ExitExecuteRuntimeStatement is called when exiting the executeRuntimeStatement production.
	ExitExecuteRuntimeStatement(c *ExecuteRuntimeStatementContext)

	// ExitLintStatement is called when exiting the lintStatement production.
	ExitLintStatement(c *LintStatementContext)

	// ExitLintTarget is called when exiting the lintTarget production.
	ExitLintTarget(c *LintTargetContext)

	// ExitLintFormat is called when exiting the lintFormat production.
	ExitLintFormat(c *LintFormatContext)

	// ExitUseSessionStatement is called when exiting the useSessionStatement production.
	ExitUseSessionStatement(c *UseSessionStatementContext)

	// ExitSessionIdList is called when exiting the sessionIdList production.
	ExitSessionIdList(c *SessionIdListContext)

	// ExitSessionId is called when exiting the sessionId production.
	ExitSessionId(c *SessionIdContext)

	// ExitIntrospectApiStatement is called when exiting the introspectApiStatement production.
	ExitIntrospectApiStatement(c *IntrospectApiStatementContext)

	// ExitDebugStatement is called when exiting the debugStatement production.
	ExitDebugStatement(c *DebugStatementContext)

	// ExitSqlConnect is called when exiting the sqlConnect production.
	ExitSqlConnect(c *SqlConnectContext)

	// ExitSqlConnectAlias is called when exiting the sqlConnectAlias production.
	ExitSqlConnectAlias(c *SqlConnectAliasContext)

	// ExitSqlDisconnect is called when exiting the sqlDisconnect production.
	ExitSqlDisconnect(c *SqlDisconnectContext)

	// ExitSqlConnections is called when exiting the sqlConnections production.
	ExitSqlConnections(c *SqlConnectionsContext)

	// ExitSqlShowTables is called when exiting the sqlShowTables production.
	ExitSqlShowTables(c *SqlShowTablesContext)

	// ExitSqlDescribeTable is called when exiting the sqlDescribeTable production.
	ExitSqlDescribeTable(c *SqlDescribeTableContext)

	// ExitSqlGenerateConnector is called when exiting the sqlGenerateConnector production.
	ExitSqlGenerateConnector(c *SqlGenerateConnectorContext)

	// ExitSqlQuery is called when exiting the sqlQuery production.
	ExitSqlQuery(c *SqlQueryContext)

	// ExitSqlPassthrough is called when exiting the sqlPassthrough production.
	ExitSqlPassthrough(c *SqlPassthroughContext)

	// ExitImportFromQuery is called when exiting the importFromQuery production.
	ExitImportFromQuery(c *ImportFromQueryContext)

	// ExitImportMapping is called when exiting the importMapping production.
	ExitImportMapping(c *ImportMappingContext)

	// ExitLinkLookup is called when exiting the linkLookup production.
	ExitLinkLookup(c *LinkLookupContext)

	// ExitLinkDirect is called when exiting the linkDirect production.
	ExitLinkDirect(c *LinkDirectContext)

	// ExitHelpStatement is called when exiting the helpStatement production.
	ExitHelpStatement(c *HelpStatementContext)

	// ExitDefineFragmentStatement is called when exiting the defineFragmentStatement production.
	ExitDefineFragmentStatement(c *DefineFragmentStatementContext)

	// ExitExpression is called when exiting the expression production.
	ExitExpression(c *ExpressionContext)

	// ExitOrExpression is called when exiting the orExpression production.
	ExitOrExpression(c *OrExpressionContext)

	// ExitAndExpression is called when exiting the andExpression production.
	ExitAndExpression(c *AndExpressionContext)

	// ExitNotExpression is called when exiting the notExpression production.
	ExitNotExpression(c *NotExpressionContext)

	// ExitComparisonExpression is called when exiting the comparisonExpression production.
	ExitComparisonExpression(c *ComparisonExpressionContext)

	// ExitComparisonOperator is called when exiting the comparisonOperator production.
	ExitComparisonOperator(c *ComparisonOperatorContext)

	// ExitAdditiveExpression is called when exiting the additiveExpression production.
	ExitAdditiveExpression(c *AdditiveExpressionContext)

	// ExitMultiplicativeExpression is called when exiting the multiplicativeExpression production.
	ExitMultiplicativeExpression(c *MultiplicativeExpressionContext)

	// ExitUnaryExpression is called when exiting the unaryExpression production.
	ExitUnaryExpression(c *UnaryExpressionContext)

	// ExitPrimaryExpression is called when exiting the primaryExpression production.
	ExitPrimaryExpression(c *PrimaryExpressionContext)

	// ExitCaseExpression is called when exiting the caseExpression production.
	ExitCaseExpression(c *CaseExpressionContext)

	// ExitIfThenElseExpression is called when exiting the ifThenElseExpression production.
	ExitIfThenElseExpression(c *IfThenElseExpressionContext)

	// ExitCastExpression is called when exiting the castExpression production.
	ExitCastExpression(c *CastExpressionContext)

	// ExitCastDataType is called when exiting the castDataType production.
	ExitCastDataType(c *CastDataTypeContext)

	// ExitAggregateFunction is called when exiting the aggregateFunction production.
	ExitAggregateFunction(c *AggregateFunctionContext)

	// ExitFunctionCall is called when exiting the functionCall production.
	ExitFunctionCall(c *FunctionCallContext)

	// ExitFunctionName is called when exiting the functionName production.
	ExitFunctionName(c *FunctionNameContext)

	// ExitArgumentList is called when exiting the argumentList production.
	ExitArgumentList(c *ArgumentListContext)

	// ExitAtomicExpression is called when exiting the atomicExpression production.
	ExitAtomicExpression(c *AtomicExpressionContext)

	// ExitExpressionList is called when exiting the expressionList production.
	ExitExpressionList(c *ExpressionListContext)

	// ExitQualifiedName is called when exiting the qualifiedName production.
	ExitQualifiedName(c *QualifiedNameContext)

	// ExitIdentifierOrKeyword is called when exiting the identifierOrKeyword production.
	ExitIdentifierOrKeyword(c *IdentifierOrKeywordContext)

	// ExitLiteral is called when exiting the literal production.
	ExitLiteral(c *LiteralContext)

	// ExitArrayLiteral is called when exiting the arrayLiteral production.
	ExitArrayLiteral(c *ArrayLiteralContext)

	// ExitBooleanLiteral is called when exiting the booleanLiteral production.
	ExitBooleanLiteral(c *BooleanLiteralContext)

	// ExitDocComment is called when exiting the docComment production.
	ExitDocComment(c *DocCommentContext)

	// ExitAnnotation is called when exiting the annotation production.
	ExitAnnotation(c *AnnotationContext)

	// ExitAnnotationName is called when exiting the annotationName production.
	ExitAnnotationName(c *AnnotationNameContext)

	// ExitAnnotationParams is called when exiting the annotationParams production.
	ExitAnnotationParams(c *AnnotationParamsContext)

	// ExitAnnotationParam is called when exiting the annotationParam production.
	ExitAnnotationParam(c *AnnotationParamContext)

	// ExitAnnotationValue is called when exiting the annotationValue production.
	ExitAnnotationValue(c *AnnotationValueContext)

	// ExitCommonNameKeyword is called when exiting the commonNameKeyword production.
	ExitCommonNameKeyword(c *CommonNameKeywordContext)

	// ExitKeyword is called when exiting the keyword production.
	ExitKeyword(c *KeywordContext)
}
