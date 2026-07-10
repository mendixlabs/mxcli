// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"strconv"
	"strings"

	"github.com/antlr4-go/antlr/v4"
	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
	"github.com/JordtenBulte-OLC/mxcli/mdl/grammar/parser"
)

// buildLogStatement converts LOG statement context to LogStmt.
// Grammar: LOG logLevel? (NODE expression)? expression logTemplateParams?
func buildLogStatement(ctx parser.ILogStatementContext) *ast.LogStmt {
	if ctx == nil {
		return nil
	}
	logCtx := ctx.(*parser.LogStatementContext)

	stmt := &ast.LogStmt{
		Level: ast.LogInfo, // Default level
	}

	// Get log level
	if level := logCtx.LogLevel(); level != nil {
		levelCtx := level.(*parser.LogLevelContext)
		if levelCtx.INFO() != nil {
			stmt.Level = ast.LogInfo
		} else if levelCtx.WARNING() != nil {
			stmt.Level = ast.LogWarning
		} else if levelCtx.ERROR() != nil {
			stmt.Level = ast.LogError
		} else if levelCtx.DEBUG() != nil {
			stmt.Level = ast.LogDebug
		} else if levelCtx.TRACE() != nil {
			stmt.Level = ast.LogTrace
		} else if levelCtx.CRITICAL() != nil {
			stmt.Level = ast.LogCritical
		}
	}

	var exprs []parser.IExpressionContext
	for _, child := range logCtx.GetChildren() {
		if expr, ok := child.(parser.IExpressionContext); ok {
			exprs = append(exprs, expr)
		}
	}

	// LOG always has a message expression; when NODE is present the first
	// expression is the node and the second is the message.
	if logCtx.NODE() != nil && len(exprs) > 1 {
		stmt.Node = buildSourceExpression(exprs[0])
		stmt.Node = appendLogNodeTrailingWhitespace(exprs[0], exprs[1], stmt.Node)
		stmt.Message = buildSourceExpression(exprs[1])
	} else if len(exprs) > 0 {
		stmt.Message = buildSourceExpression(exprs[0])
	}

	// Parse template parameters: WITH ({1} = expr, {2} = expr, ...)
	if params := logCtx.LogTemplateParams(); params != nil {
		logParamsCtx := params.(*parser.LogTemplateParamsContext)
		if tplParams := logParamsCtx.TemplateParams(); tplParams != nil {
			stmt.Template = buildTemplateParams(tplParams)
		}
	}

	return stmt
}

// buildTemplateParams converts templateParams context to []ast.TemplateParam.
// Handles both WITH ({1} = expr) syntax and deprecated PARAMETERS [array] syntax.
func buildTemplateParams(ctx parser.ITemplateParamsContext) []ast.TemplateParam {
	if ctx == nil {
		return nil
	}
	paramsCtx := ctx.(*parser.TemplateParamsContext)

	var result []ast.TemplateParam

	// Handle WITH ({1} = expr, {2} = expr, ...) syntax
	allParams := paramsCtx.AllTemplateParam()
	for i, param := range allParams {
		paramCtx := param.(*parser.TemplateParamContext)
		indexStr := paramCtx.NUMBER_LITERAL().GetText()
		index, _ := strconv.Atoi(indexStr)

		var tp ast.TemplateParam
		tp.Index = index

		// Parse the expression and check for data source attribute reference
		if exprCtx := paramCtx.Expression(); exprCtx != nil {
			expr := buildSourceExpression(exprCtx)
			expr = appendTemplateParamTrailingWhitespace(paramsCtx, allParams, i, exprCtx, expr)
			tp.Value = expr

			// Check if this is a $Widget.Attr pattern (AttributePathExpr with Path)
			if pathExpr, ok := expr.(*ast.AttributePathExpr); ok && len(pathExpr.Path) > 0 {
				// This is a data source attribute reference
				tp.DataSourceName = pathExpr.Variable
				tp.AttributeName = pathExpr.Path[len(pathExpr.Path)-1]
			}
		}

		result = append(result, tp)
	}

	// Handle deprecated PARAMETERS [array] syntax
	if arr := paramsCtx.ArrayLiteral(); arr != nil {
		arrCtx := arr.(*parser.ArrayLiteralContext)
		for i, lit := range arrCtx.AllLiteral() {
			var tp ast.TemplateParam
			tp.Index = i + 1 // 1-based index
			tp.Value = buildLiteralExpression(lit)
			result = append(result, tp)
		}
	}

	return result
}

func appendTemplateParamTrailingWhitespace(
	paramsCtx *parser.TemplateParamsContext,
	allParams []parser.ITemplateParamContext,
	index int,
	exprCtx parser.IExpressionContext,
	expr ast.Expression,
) ast.Expression {
	trailing := templateParamTrailingWhitespace(paramsCtx, allParams, index, exprCtx)
	if trailing == "" {
		return expr
	}

	return appendSourceExpressionSuffix(exprCtx, expr, trailing)
}

func templateParamTrailingWhitespace(
	paramsCtx *parser.TemplateParamsContext,
	allParams []parser.ITemplateParamContext,
	index int,
	exprCtx parser.IExpressionContext,
) string {
	exprRule, ok := exprCtx.(antlr.ParserRuleContext)
	if !ok || exprRule.GetStop() == nil {
		return ""
	}
	input := exprRule.GetStop().GetInputStream()
	if input == nil {
		return ""
	}

	start := exprRule.GetStop().GetStop() + 1
	end := -1
	delimiter := byte(')')
	if index+1 < len(allParams) {
		nextParam := allParams[index+1].(antlr.ParserRuleContext)
		end = nextParam.GetStart().GetStart() - 1
		delimiter = ','
	} else if paramsCtx.GetStop() != nil {
		end = paramsCtx.GetStop().GetStart() - 1
	}
	if start < 0 || end < start {
		return ""
	}

	gap := input.GetText(start, end)
	if delimiter == ',' {
		comma := strings.IndexByte(gap, ',')
		if comma == -1 {
			return ""
		}
		gap = gap[:comma]
	}
	if strings.TrimSpace(gap) != "" {
		return ""
	}
	return gap
}

func nextParserRuleContext[T any](items []T, index int) antlr.ParserRuleContext {
	if index+1 >= len(items) {
		return nil
	}
	next, _ := any(items[index+1]).(antlr.ParserRuleContext)
	return next
}

func appendExpressionListTrailingWhitespace(
	parent antlr.ParserRuleContext,
	next antlr.ParserRuleContext,
	exprCtx parser.IExpressionContext,
	expr ast.Expression,
) ast.Expression {
	trailing := expressionListTrailingWhitespace(parent, next, exprCtx)
	if trailing == "" {
		return expr
	}

	return appendSourceExpressionSuffix(exprCtx, expr, trailing)
}

func expressionListTrailingWhitespace(
	parent antlr.ParserRuleContext,
	next antlr.ParserRuleContext,
	exprCtx parser.IExpressionContext,
) string {
	exprRule, ok := exprCtx.(antlr.ParserRuleContext)
	if !ok || exprRule.GetStop() == nil {
		return ""
	}
	input := exprRule.GetStop().GetInputStream()
	if input == nil {
		return ""
	}

	start := exprRule.GetStop().GetStop() + 1
	end := -1
	if next != nil {
		end = next.GetStart().GetStart() - 1
	} else if parent != nil && parent.GetStop() != nil {
		end = parent.GetStop().GetStart() - 1
	}
	if start < 0 || end < start {
		if next == nil {
			return whitespaceUntilDelimiter(input, start, ")]")
		}
		return ""
	}

	gap := input.GetText(start, end)
	if next != nil {
		comma := strings.IndexByte(gap, ',')
		if comma == -1 {
			return ""
		}
		gap = gap[:comma]
	}
	if strings.TrimSpace(gap) != "" {
		return ""
	}
	return gap
}

func appendStatementExpressionTrailingWhitespace(
	exprCtx parser.IExpressionContext,
	expr ast.Expression,
) ast.Expression {
	trailing := statementExpressionTrailingWhitespace(exprCtx)
	if trailing == "" {
		return expr
	}
	return appendSourceExpressionSuffix(exprCtx, expr, trailing)
}

func statementExpressionTrailingWhitespace(exprCtx parser.IExpressionContext) string {
	exprRule, ok := exprCtx.(antlr.ParserRuleContext)
	if !ok || exprRule.GetStop() == nil {
		return ""
	}
	input := exprRule.GetStop().GetInputStream()
	if input == nil {
		return ""
	}

	start := exprRule.GetStop().GetStop() + 1
	if start < 0 || start >= input.Size() {
		return ""
	}

	return whitespaceUntilDelimiter(input, start, ";")
}

func appendLogNodeTrailingWhitespace(
	nodeCtx parser.IExpressionContext,
	messageCtx parser.IExpressionContext,
	expr ast.Expression,
) ast.Expression {
	trailing := expressionGapWhitespace(nodeCtx, messageCtx)
	if trailing == "" || !strings.ContainsAny(trailing, "\r\n") {
		return expr
	}
	// formatAction always writes one space between the node and message slots.
	// Preserve the source line break but avoid duplicating indentation spaces.
	trailing = strings.TrimRight(trailing, " \t")
	if trailing == "" {
		return expr
	}
	return appendSourceExpressionSuffix(nodeCtx, expr, trailing)
}

func expressionGapWhitespace(
	leftCtx parser.IExpressionContext,
	rightCtx parser.IExpressionContext,
) string {
	leftRule, ok := leftCtx.(antlr.ParserRuleContext)
	if !ok || leftRule.GetStop() == nil {
		return ""
	}
	rightRule, ok := rightCtx.(antlr.ParserRuleContext)
	if !ok || rightRule.GetStart() == nil {
		return ""
	}
	input := leftRule.GetStop().GetInputStream()
	if input == nil {
		return ""
	}
	start := leftRule.GetStop().GetStop() + 1
	end := rightRule.GetStart().GetStart() - 1
	if start < 0 || end < start {
		return ""
	}
	gap := input.GetText(start, end)
	if strings.TrimSpace(gap) != "" {
		return ""
	}
	return gap
}

func whitespaceUntilDelimiter(input antlr.CharStream, start int, delimiters string) string {
	if start < 0 || start >= input.Size() {
		return ""
	}
	end := start
	for end < input.Size() {
		ch := input.GetText(end, end)
		if strings.Contains(delimiters, ch) {
			break
		}
		if strings.TrimSpace(ch) != "" {
			return ""
		}
		end++
	}
	if end >= input.Size() || end == start {
		return ""
	}
	return input.GetText(start, end-1)
}

func appendSourceExpressionSuffix(
	exprCtx parser.IExpressionContext,
	expr ast.Expression,
	suffix string,
) ast.Expression {
	source := strings.TrimSpace(extractOriginalText(exprCtx.(antlr.ParserRuleContext)))
	innerExpr := expr
	if sourceExpr, ok := expr.(*ast.SourceExpr); ok {
		source = sourceExpr.Source
		innerExpr = sourceExpr.Expression
	}
	return &ast.SourceExpr{Expression: innerExpr, Source: source + suffix}
}

// buildCallMicroflowStatement converts CALL MICROFLOW statement context to CallMicroflowStmt.
// Grammar: (VARIABLE EQUALS)? CALL MICROFLOW qualifiedName LPAREN callArgumentList? RPAREN
func buildCallMicroflowStatement(ctx parser.ICallMicroflowStatementContext) *ast.CallMicroflowStmt {
	if ctx == nil {
		return nil
	}
	callCtx := ctx.(*parser.CallMicroflowStatementContext)

	stmt := &ast.CallMicroflowStmt{}

	// Get result variable if present
	if v := callCtx.VARIABLE(); v != nil {
		stmt.OutputVariable = strings.TrimPrefix(v.GetText(), "$")
	}

	// Get microflow name
	if qn := callCtx.QualifiedName(); qn != nil {
		stmt.MicroflowName = buildQualifiedName(qn)
	}

	// Get arguments from callArgumentList
	if argList := callCtx.CallArgumentList(); argList != nil {
		stmt.Arguments = buildCallArgumentList(argList)
	}

	// Check for ON ERROR clause
	if errClause := callCtx.OnErrorClause(); errClause != nil {
		stmt.ErrorHandling = buildOnErrorClause(errClause)
	}

	return stmt
}

// buildCallNanoflowStatement converts CALL NANOFLOW statement context to CallNanoflowStmt.
// Grammar: (VARIABLE EQUALS)? CALL NANOFLOW qualifiedName LPAREN callArgumentList? RPAREN onErrorClause?
func buildCallNanoflowStatement(ctx parser.ICallNanoflowStatementContext) *ast.CallNanoflowStmt {
	if ctx == nil {
		return nil
	}
	callCtx := ctx.(*parser.CallNanoflowStatementContext)

	stmt := &ast.CallNanoflowStmt{}

	// Get result variable if present
	if v := callCtx.VARIABLE(); v != nil {
		stmt.OutputVariable = strings.TrimPrefix(v.GetText(), "$")
	}

	// Get nanoflow name
	if qn := callCtx.QualifiedName(); qn != nil {
		stmt.NanoflowName = buildQualifiedName(qn)
	}

	// Get arguments from callArgumentList
	if argList := callCtx.CallArgumentList(); argList != nil {
		stmt.Arguments = buildCallArgumentList(argList)
	}

	// Check for ON ERROR clause
	if errClause := callCtx.OnErrorClause(); errClause != nil {
		stmt.ErrorHandling = buildOnErrorClause(errClause)
	}

	return stmt
}

// buildCallJavaActionStatement converts CALL JAVA ACTION statement context to CallJavaActionStmt.
// Grammar: (VARIABLE EQUALS)? CALL JAVA ACTION qualifiedName LPAREN callArgumentList? RPAREN
func buildCallJavaActionStatement(ctx parser.ICallJavaActionStatementContext) *ast.CallJavaActionStmt {
	if ctx == nil {
		return nil
	}
	callCtx := ctx.(*parser.CallJavaActionStatementContext)

	stmt := &ast.CallJavaActionStmt{}

	// Get result variable if present
	if v := callCtx.VARIABLE(); v != nil {
		stmt.OutputVariable = strings.TrimPrefix(v.GetText(), "$")
	}

	// Get java action name
	if qn := callCtx.QualifiedName(); qn != nil {
		stmt.ActionName = buildQualifiedName(qn)
	}

	// Get arguments from callArgumentList
	if argList := callCtx.CallArgumentList(); argList != nil {
		stmt.Arguments = buildCallArgumentList(argList)
	}

	// Check for ON ERROR clause
	if errClause := callCtx.OnErrorClause(); errClause != nil {
		stmt.ErrorHandling = buildOnErrorClause(errClause)
	}

	return stmt
}

// buildCallJavaScriptActionStatement converts CALL JAVASCRIPT ACTION statement context to CallJavaScriptActionStmt.
// Grammar: (VARIABLE EQUALS)? CALL JAVASCRIPT ACTION qualifiedName LPAREN callArgumentList? RPAREN
func buildCallJavaScriptActionStatement(ctx parser.ICallJavaScriptActionStatementContext) *ast.CallJavaScriptActionStmt {
	if ctx == nil {
		return nil
	}
	callCtx := ctx.(*parser.CallJavaScriptActionStatementContext)

	stmt := &ast.CallJavaScriptActionStmt{}

	// Get result variable if present
	if v := callCtx.VARIABLE(); v != nil {
		stmt.OutputVariable = strings.TrimPrefix(v.GetText(), "$")
	}

	// Get javascript action name
	if qn := callCtx.QualifiedName(); qn != nil {
		stmt.ActionName = buildQualifiedName(qn)
	}

	// Get arguments from callArgumentList
	if argList := callCtx.CallArgumentList(); argList != nil {
		stmt.Arguments = buildCallArgumentList(argList)
	}

	// Check for ON ERROR clause
	if errClause := callCtx.OnErrorClause(); errClause != nil {
		stmt.ErrorHandling = buildOnErrorClause(errClause)
	}

	return stmt
}

// buildCallWebServiceStatement converts CALL WEB SERVICE statement context to CallWebServiceStmt.
func buildCallWebServiceStatement(ctx parser.ICallWebServiceStatementContext) *ast.CallWebServiceStmt {
	if ctx == nil {
		return nil
	}
	callCtx := ctx.(*parser.CallWebServiceStatementContext)

	stmt := &ast.CallWebServiceStmt{}
	if v := callCtx.VARIABLE(); v != nil {
		stmt.OutputVariable = strings.TrimPrefix(v.GetText(), "$")
	}

	if callCtx.RAW() != nil {
		if lit := callCtx.STRING_LITERAL(); lit != nil {
			stmt.RawBSONBase64 = unquoteString(lit.GetText())
		}
		if errClause := callCtx.OnErrorClause(); errClause != nil {
			stmt.ErrorHandling = buildOnErrorClause(errClause)
		}
		return stmt
	}

	// The grammar fixes the structured CALL WEB SERVICE clause order as:
	// service, optional operation, optional send mapping, optional receive
	// mapping. Keep the positional reference walk in that same order.
	refs := callCtx.AllWebServiceReference()
	idx := 0
	if len(refs) > idx {
		stmt.ServiceID = webServiceReferenceText(refs[idx])
		idx++
	}
	if callCtx.OPERATION() != nil && len(refs) > idx {
		stmt.OperationName = webServiceReferenceText(refs[idx])
		idx++
	}
	if callCtx.SEND() != nil && len(refs) > idx {
		stmt.SendMappingID = webServiceReferenceText(refs[idx])
		idx++
	}
	if callCtx.RECEIVE() != nil && len(refs) > idx {
		stmt.ReceiveMappingID = webServiceReferenceText(refs[idx])
	}
	if expr := callCtx.Expression(); expr != nil {
		stmt.Timeout = buildExpression(expr)
	}
	if errClause := callCtx.OnErrorClause(); errClause != nil {
		stmt.ErrorHandling = buildOnErrorClause(errClause)
	}

	return stmt
}

func webServiceReferenceText(ctx parser.IWebServiceReferenceContext) string {
	if ctx == nil {
		return ""
	}
	refCtx := ctx.(*parser.WebServiceReferenceContext)
	if lit := refCtx.STRING_LITERAL(); lit != nil {
		return unquoteString(lit.GetText())
	}
	return getQualifiedNameText(refCtx.QualifiedName())
}

// buildExecuteDatabaseQueryStatement converts EXECUTE DATABASE QUERY context to ExecuteDatabaseQueryStmt.
func buildExecuteDatabaseQueryStatement(ctx parser.IExecuteDatabaseQueryStatementContext) *ast.ExecuteDatabaseQueryStmt {
	if ctx == nil {
		return nil
	}
	execCtx := ctx.(*parser.ExecuteDatabaseQueryStatementContext)

	stmt := &ast.ExecuteDatabaseQueryStmt{}

	// Get result variable if present
	if v := execCtx.VARIABLE(); v != nil {
		stmt.OutputVariable = strings.TrimPrefix(v.GetText(), "$")
	}

	// Get query name (Module.Connection.QueryName — 3-part identifier)
	if qn := execCtx.QualifiedName(); qn != nil {
		stmt.QueryName = getQualifiedNameText(qn)
	}

	// Get dynamic query if present
	if execCtx.DYNAMIC() != nil {
		if sl := execCtx.STRING_LITERAL(); sl != nil {
			stmt.DynamicQuery = unquoteString(sl.GetText())
		} else if ds := execCtx.DOLLAR_STRING(); ds != nil {
			stmt.DynamicQuery = unquoteDollarString(ds.GetText())
		} else if expr := execCtx.Expression(); expr != nil {
			stmt.DynamicQuery = expr.GetText()
		}
	}

	// Get query parameter arguments from first callArgumentList
	if argList := execCtx.CallArgumentList(0); argList != nil {
		stmt.Arguments = buildCallArgumentList(argList)
	}

	// Get connection parameter arguments from second callArgumentList (after CONNECTION keyword)
	if execCtx.CONNECTION() != nil {
		if argList := execCtx.CallArgumentList(1); argList != nil {
			stmt.ConnectionArguments = buildCallArgumentList(argList)
		}
	}

	// Check for ON ERROR clause
	if errClause := execCtx.OnErrorClause(); errClause != nil {
		stmt.ErrorHandling = buildOnErrorClause(errClause)
	}

	return stmt
}

// buildCallExternalActionStatement converts CALL EXTERNAL ACTION context to CallExternalActionStmt.
// Grammar: (VARIABLE EQUALS)? CALL EXTERNAL ACTION qualifiedName DOT IDENTIFIER LPAREN callArgumentList? RPAREN
func buildCallExternalActionStatement(ctx parser.ICallExternalActionStatementContext) *ast.CallExternalActionStmt {
	if ctx == nil {
		return nil
	}
	callCtx := ctx.(*parser.CallExternalActionStatementContext)

	stmt := &ast.CallExternalActionStmt{}

	// Get result variable if present
	if v := callCtx.VARIABLE(); v != nil {
		stmt.OutputVariable = strings.TrimPrefix(v.GetText(), "$")
	}

	// qualifiedName matches Module.ServiceName.ActionName (3+ parts)
	// Split off the last segment as the action name, rest is service qualified name
	if qn := callCtx.QualifiedName(); qn != nil {
		fullText := getQualifiedNameText(qn) // e.g. "Module.ServiceName.ActionName"
		if lastDot := strings.LastIndex(fullText, "."); lastDot >= 0 {
			servicePart := fullText[:lastDot]      // "Module.ServiceName"
			stmt.ActionName = fullText[lastDot+1:] // "ActionName"
			// Split service part into Module.Name
			if before, after, ok := strings.Cut(servicePart, "."); ok {
				stmt.ServiceName = ast.QualifiedName{
					Module: before,
					Name:   after,
				}
			} else {
				stmt.ServiceName = ast.QualifiedName{Name: servicePart}
			}
		} else {
			// Single identifier — treat as action name
			stmt.ActionName = fullText
		}
	}

	// Get arguments from callArgumentList
	if argList := callCtx.CallArgumentList(); argList != nil {
		stmt.Arguments = buildCallArgumentList(argList)
	}

	// Check for ON ERROR clause
	if errClause := callCtx.OnErrorClause(); errClause != nil {
		stmt.ErrorHandling = buildOnErrorClause(errClause)
	}

	return stmt
}

// buildCallArgumentList converts callArgumentList context to CallArgument slice.
func buildCallArgumentList(ctx parser.ICallArgumentListContext) []ast.CallArgument {
	if ctx == nil {
		return nil
	}
	listCtx := ctx.(*parser.CallArgumentListContext)
	var args []ast.CallArgument

	allArgs := listCtx.AllCallArgument()
	for i, argCtx := range allArgs {
		arg := argCtx.(*parser.CallArgumentContext)
		ca := ast.CallArgument{}

		// Name can be VARIABLE or parameterName
		if v := arg.VARIABLE(); v != nil {
			ca.Name = strings.TrimPrefix(v.GetText(), "$")
		} else if pn := arg.ParameterName(); pn != nil {
			ca.Name = parameterNameText(pn)
		}
		if expr := arg.Expression(); expr != nil {
			value := buildSourceExpression(expr)
			ca.Value = appendExpressionListTrailingWhitespace(listCtx, nextParserRuleContext(allArgs, i), expr, value)
		}

		args = append(args, ca)
	}

	return args
}

// buildMemberAssignmentList converts memberAssignmentList context to ChangeItem slice.
func buildMemberAssignmentList(ctx parser.IMemberAssignmentListContext) []ast.ChangeItem {
	if ctx == nil {
		return nil
	}
	listCtx := ctx.(*parser.MemberAssignmentListContext)
	var items []ast.ChangeItem

	allAssignments := listCtx.AllMemberAssignment()
	for i, assignCtx := range allAssignments {
		assign := assignCtx.(*parser.MemberAssignmentContext)
		ci := ast.ChangeItem{}

		// Get attribute name (can be IDENTIFIER, keyword, or quoted identifier)
		if name := assign.MemberAttributeName(); name != nil {
			ci.Attribute = memberAttributeNameText(name)
		}
		if expr := assign.Expression(); expr != nil {
			value := buildSourceExpression(expr)
			ci.Value = appendExpressionListTrailingWhitespace(listCtx, nextParserRuleContext(allAssignments, i), expr, value)
		}

		items = append(items, ci)
	}

	return items
}

// buildChangeList converts changeList context to ChangeItem slice.
func buildChangeList(ctx parser.IChangeListContext) []ast.ChangeItem {
	if ctx == nil {
		return nil
	}
	listCtx := ctx.(*parser.ChangeListContext)
	var items []ast.ChangeItem

	allItems := listCtx.AllChangeItem()
	for i, itemCtx := range allItems {
		item := itemCtx.(*parser.ChangeItemContext)
		ci := ast.ChangeItem{}

		if id := item.IDENTIFIER(); id != nil {
			ci.Attribute = id.GetText()
		}
		if expr := item.Expression(); expr != nil {
			value := buildSourceExpression(expr)
			ci.Value = appendExpressionListTrailingWhitespace(listCtx, nextParserRuleContext(allItems, i), expr, value)
		}

		items = append(items, ci)
	}

	return items
}

// ============================================================================
// List Operation Statements
// ============================================================================

// buildListOperationStatement converts list operation statement context to ListOperationStmt.
// Grammar: VARIABLE EQUALS listOperation
func buildListOperationStatement(ctx parser.IListOperationStatementContext) *ast.ListOperationStmt {
	if ctx == nil {
		return nil
	}
	listOpCtx := ctx.(*parser.ListOperationStatementContext)

	stmt := &ast.ListOperationStmt{}

	// Get output variable
	if v := listOpCtx.VARIABLE(); v != nil {
		stmt.OutputVariable = strings.TrimPrefix(v.GetText(), "$")
	}

	// Get the list operation
	if opCtx := listOpCtx.ListOperation(); opCtx != nil {
		op := opCtx.(*parser.ListOperationContext)

		// Get all variables from the operation
		vars := op.AllVARIABLE()

		// Determine operation type based on which token is present
		if op.HEAD() != nil {
			stmt.Operation = ast.ListOpHead
			if len(vars) >= 1 {
				stmt.InputVariable = strings.TrimPrefix(vars[0].GetText(), "$")
			}
		} else if op.TAIL() != nil {
			stmt.Operation = ast.ListOpTail
			if len(vars) >= 1 {
				stmt.InputVariable = strings.TrimPrefix(vars[0].GetText(), "$")
			}
		} else if op.FIND() != nil {
			stmt.Operation = ast.ListOpFind
			if len(vars) >= 1 {
				stmt.InputVariable = strings.TrimPrefix(vars[0].GetText(), "$")
			}
			if expr := op.Expression(0); expr != nil {
				stmt.Condition = buildSourceExpression(expr)
			}
		} else if op.FILTER() != nil {
			stmt.Operation = ast.ListOpFilter
			if len(vars) >= 1 {
				stmt.InputVariable = strings.TrimPrefix(vars[0].GetText(), "$")
			}
			if expr := op.Expression(0); expr != nil {
				stmt.Condition = buildSourceExpression(expr)
			}
		} else if op.SORT() != nil {
			stmt.Operation = ast.ListOpSort
			if len(vars) >= 1 {
				stmt.InputVariable = strings.TrimPrefix(vars[0].GetText(), "$")
			}
			if sortList := op.SortSpecList(); sortList != nil {
				stmt.SortSpecs = buildSortSpecList(sortList)
			}
		} else if op.UNION() != nil {
			stmt.Operation = ast.ListOpUnion
			if len(vars) >= 1 {
				stmt.InputVariable = strings.TrimPrefix(vars[0].GetText(), "$")
			}
			if len(vars) >= 2 {
				stmt.SecondVariable = strings.TrimPrefix(vars[1].GetText(), "$")
			}
		} else if op.INTERSECT() != nil {
			stmt.Operation = ast.ListOpIntersect
			if len(vars) >= 1 {
				stmt.InputVariable = strings.TrimPrefix(vars[0].GetText(), "$")
			}
			if len(vars) >= 2 {
				stmt.SecondVariable = strings.TrimPrefix(vars[1].GetText(), "$")
			}
		} else if op.SUBTRACT() != nil {
			stmt.Operation = ast.ListOpSubtract
			if len(vars) >= 1 {
				stmt.InputVariable = strings.TrimPrefix(vars[0].GetText(), "$")
			}
			if len(vars) >= 2 {
				stmt.SecondVariable = strings.TrimPrefix(vars[1].GetText(), "$")
			}
		} else if op.CONTAINS() != nil {
			stmt.Operation = ast.ListOpContains
			if len(vars) >= 1 {
				stmt.InputVariable = strings.TrimPrefix(vars[0].GetText(), "$")
			}
			if len(vars) >= 2 {
				stmt.SecondVariable = strings.TrimPrefix(vars[1].GetText(), "$")
			}
		} else if op.EQUALS_OP() != nil {
			stmt.Operation = ast.ListOpEquals
			if len(vars) >= 1 {
				stmt.InputVariable = strings.TrimPrefix(vars[0].GetText(), "$")
			}
			if len(vars) >= 2 {
				stmt.SecondVariable = strings.TrimPrefix(vars[1].GetText(), "$")
			}
		} else if op.RANGE() != nil {
			stmt.Operation = ast.ListOpRange
			if len(vars) >= 1 {
				stmt.InputVariable = strings.TrimPrefix(vars[0].GetText(), "$")
			}
			exprs := op.AllExpression()
			if len(exprs) >= 1 {
				stmt.OffsetExpr = buildSourceExpression(exprs[0])
			}
			if len(exprs) >= 2 {
				stmt.LimitExpr = buildSourceExpression(exprs[1])
			}
		}
	}

	return stmt
}

// buildSortSpecList converts sortSpecList context to SortSpec slice.
func buildSortSpecList(ctx parser.ISortSpecListContext) []ast.SortSpec {
	if ctx == nil {
		return nil
	}
	listCtx := ctx.(*parser.SortSpecListContext)
	var specs []ast.SortSpec

	for _, specCtx := range listCtx.AllSortSpec() {
		spec := specCtx.(*parser.SortSpecContext)
		ss := ast.SortSpec{
			Ascending: true, // Default to ascending
		}

		if id := spec.IDENTIFIER(); id != nil {
			ss.Attribute = id.GetText()
		} else if qid := spec.QUOTED_IDENTIFIER(); qid != nil {
			ss.Attribute = unquoteIdentifier(qid.GetText())
		}
		if spec.DESC() != nil {
			ss.Ascending = false
		}

		specs = append(specs, ss)
	}

	return specs
}

// buildAggregateListStatement converts aggregate list statement context to AggregateListStmt.
// Grammar: VARIABLE EQUALS listAggregateOperation
func buildAggregateListStatement(ctx parser.IAggregateListStatementContext) *ast.AggregateListStmt {
	if ctx == nil {
		return nil
	}
	aggrCtx := ctx.(*parser.AggregateListStatementContext)

	stmt := &ast.AggregateListStmt{}

	// Get output variable
	if v := aggrCtx.VARIABLE(); v != nil {
		stmt.OutputVariable = strings.TrimPrefix(v.GetText(), "$")
	}

	// Get the aggregate operation
	if opCtx := aggrCtx.ListAggregateOperation(); opCtx != nil {
		op := opCtx.(*parser.ListAggregateOperationContext)

		// Determine operation type
		if op.COUNT() != nil {
			stmt.Operation = ast.AggregateCount
			if v := op.VARIABLE(); v != nil {
				stmt.InputVariable = strings.TrimPrefix(v.GetText(), "$")
			}
		} else if op.SUM() != nil {
			stmt.Operation = ast.AggregateSum
			if exprCtx := op.Expression(); exprCtx != nil {
				stmt.IsExpression = true
				stmt.Expression = buildSourceExpression(exprCtx)
				if v := op.VARIABLE(); v != nil {
					stmt.InputVariable = strings.TrimPrefix(v.GetText(), "$")
				}
			} else if path := op.AttributePath(); path != nil {
				stmt.InputVariable, stmt.Attribute = parseAttributePath(path.GetText())
			}
		} else if op.AVERAGE() != nil {
			stmt.Operation = ast.AggregateAverage
			if exprCtx := op.Expression(); exprCtx != nil {
				stmt.IsExpression = true
				stmt.Expression = buildSourceExpression(exprCtx)
				if v := op.VARIABLE(); v != nil {
					stmt.InputVariable = strings.TrimPrefix(v.GetText(), "$")
				}
			} else if path := op.AttributePath(); path != nil {
				stmt.InputVariable, stmt.Attribute = parseAttributePath(path.GetText())
			}
		} else if op.MINIMUM() != nil {
			stmt.Operation = ast.AggregateMinimum
			if exprCtx := op.Expression(); exprCtx != nil {
				stmt.IsExpression = true
				stmt.Expression = buildSourceExpression(exprCtx)
				if v := op.VARIABLE(); v != nil {
					stmt.InputVariable = strings.TrimPrefix(v.GetText(), "$")
				}
			} else if path := op.AttributePath(); path != nil {
				stmt.InputVariable, stmt.Attribute = parseAttributePath(path.GetText())
			}
		} else if op.MAXIMUM() != nil {
			stmt.Operation = ast.AggregateMaximum
			if exprCtx := op.Expression(); exprCtx != nil {
				stmt.IsExpression = true
				stmt.Expression = buildSourceExpression(exprCtx)
				if v := op.VARIABLE(); v != nil {
					stmt.InputVariable = strings.TrimPrefix(v.GetText(), "$")
				}
			} else if path := op.AttributePath(); path != nil {
				stmt.InputVariable, stmt.Attribute = parseAttributePath(path.GetText())
			}
		}
	}

	return stmt
}

// parseAttributePath parses an attribute path like "$Products/Price" or "$Products.Price" into variable and attribute.
func parseAttributePath(path string) (variable string, attribute string) {
	// Remove $ prefix if present
	path = strings.TrimPrefix(path, "$")

	// Try splitting on / first (XPath style), then on . (object property style)
	var parts []string
	if strings.Contains(path, "/") {
		parts = strings.Split(path, "/")
	} else if strings.Contains(path, ".") {
		parts = strings.Split(path, ".")
	} else {
		// No separator found, entire path is variable name
		return path, ""
	}

	if len(parts) >= 1 {
		variable = parts[0]
	}
	if len(parts) >= 2 {
		attribute = parts[len(parts)-1]
	}

	return variable, attribute
}

// buildCreateListStatement converts create list statement context to CreateListStmt.
// Grammar: VARIABLE EQUALS CREATE LIST OF qualifiedName
func buildCreateListStatement(ctx parser.ICreateListStatementContext) *ast.CreateListStmt {
	if ctx == nil {
		return nil
	}
	createCtx := ctx.(*parser.CreateListStatementContext)

	stmt := &ast.CreateListStmt{}

	// Get variable name
	if v := createCtx.VARIABLE(); v != nil {
		stmt.Variable = strings.TrimPrefix(v.GetText(), "$")
	}

	// Get entity type
	if qn := createCtx.QualifiedName(); qn != nil {
		stmt.EntityType = buildQualifiedName(qn)
	}

	return stmt
}

// buildAddToListStatement converts add to list statement context to AddToListStmt.
// Grammar: ADD expression TO VARIABLE
func buildAddToListStatement(ctx parser.IAddToListStatementContext) *ast.AddToListStmt {
	if ctx == nil {
		return nil
	}
	addCtx := ctx.(*parser.AddToListStatementContext)

	stmt := &ast.AddToListStmt{}

	if expr := addCtx.Expression(); expr != nil {
		stmt.Value = buildExpression(expr)
		if varExpr, ok := stmt.Value.(*ast.VariableExpr); ok {
			stmt.Item = varExpr.Name
		}
	}
	if v := addCtx.VARIABLE(); v != nil {
		stmt.List = strings.TrimPrefix(v.GetText(), "$")
	}

	return stmt
}

// buildRemoveFromListStatement converts remove from list statement context to RemoveFromListStmt.
// Grammar: REMOVE VARIABLE FROM VARIABLE
func buildRemoveFromListStatement(ctx parser.IRemoveFromListStatementContext) *ast.RemoveFromListStmt {
	if ctx == nil {
		return nil
	}
	removeCtx := ctx.(*parser.RemoveFromListStatementContext)

	stmt := &ast.RemoveFromListStmt{}

	// Get both variables
	vars := removeCtx.AllVARIABLE()
	if len(vars) >= 1 {
		stmt.Item = strings.TrimPrefix(vars[0].GetText(), "$")
	}
	if len(vars) >= 2 {
		stmt.List = strings.TrimPrefix(vars[1].GetText(), "$")
	}

	return stmt
}

// ============================================================================
// Page Actions
// ============================================================================

// buildShowPageStatement converts show page statement context to ShowPageStmt.
// Grammar: SHOW PAGE qualifiedName (LPAREN showPageArgList? RPAREN)? (FOR VARIABLE)? (WITH memberAssignmentList)?
func buildShowPageStatement(ctx parser.IShowPageStatementContext) *ast.ShowPageStmt {
	if ctx == nil {
		return nil
	}
	showCtx := ctx.(*parser.ShowPageStatementContext)

	stmt := &ast.ShowPageStmt{
		Location: "Content", // Default location
	}

	// Get page name
	if qn := showCtx.QualifiedName(); qn != nil {
		stmt.PageName = buildQualifiedName(qn)
	}

	// Get page arguments
	if argList := showCtx.ShowPageArgList(); argList != nil {
		stmt.Arguments = buildShowPageArgList(argList)
	}

	// Get FOR variable (for data grid selection, etc.)
	if showCtx.FOR() != nil {
		if v := showCtx.VARIABLE(); v != nil {
			stmt.ForObject = strings.TrimPrefix(v.GetText(), "$")
		}
	}

	// Get WITH settings (title override, location, etc.)
	if showCtx.WITH() != nil {
		if memberList := showCtx.MemberAssignmentList(); memberList != nil {
			changes := buildMemberAssignmentList(memberList)
			for _, change := range changes {
				switch strings.ToLower(change.Attribute) {
				case "title":
					if litExpr, ok := change.Value.(*ast.LiteralExpr); ok && litExpr.Kind == ast.LiteralString {
						if s, ok := litExpr.Value.(string); ok {
							stmt.Title = s
						}
					}
				case "location":
					if litExpr, ok := change.Value.(*ast.LiteralExpr); ok && litExpr.Kind == ast.LiteralString {
						if s, ok := litExpr.Value.(string); ok {
							stmt.Location = s
						}
					} else if identExpr, ok := change.Value.(*ast.IdentifierExpr); ok {
						stmt.Location = identExpr.Name
					}
				case "modal", "modalform":
					if litExpr, ok := change.Value.(*ast.LiteralExpr); ok && litExpr.Kind == ast.LiteralBoolean {
						if b, ok := litExpr.Value.(bool); ok {
							stmt.ModalForm = b
						}
					}
				}
			}
		}
	}

	return stmt
}

// buildShowPageArgList converts showPageArgList context to ShowPageArg slice.
// Grammar: showPageArg (COMMA showPageArg)*
// showPageArg: VARIABLE EQUALS (VARIABLE | expression) | identifierOrKeyword COLON expression
func buildShowPageArgList(ctx parser.IShowPageArgListContext) []ast.ShowPageArg {
	if ctx == nil {
		return nil
	}
	listCtx := ctx.(*parser.ShowPageArgListContext)
	var args []ast.ShowPageArg

	for _, argCtx := range listCtx.AllShowPageArg() {
		arg := argCtx.(*parser.ShowPageArgContext)
		spa := ast.ShowPageArg{}

		if iok := arg.IdentifierOrKeyword(); iok != nil {
			// Widget-style: Param: $value
			spa.ParamName = identifierOrKeywordText(iok)
			if expr := arg.Expression(); expr != nil {
				spa.Value = buildSourceExpression(expr)
			}
		} else {
			// Canonical: $Param = $value
			vars := arg.AllVARIABLE()
			if len(vars) >= 1 {
				spa.ParamName = strings.TrimPrefix(vars[0].GetText(), "$")
			}
			if len(vars) >= 2 {
				spa.Value = &ast.VariableExpr{Name: strings.TrimPrefix(vars[1].GetText(), "$")}
			} else if expr := arg.Expression(); expr != nil {
				spa.Value = buildSourceExpression(expr)
			}
		}

		args = append(args, spa)
	}

	return args
}

// buildShowMessageStatement converts showMessageStatement context to ShowMessageStmt.
// Grammar: SHOW MESSAGE expression (TYPE identifierOrKeyword)? (OBJECTS LBRACKET expressionList RBRACKET)?
func buildShowMessageStatement(ctx parser.IShowMessageStatementContext) *ast.ShowMessageStmt {
	if ctx == nil {
		return nil
	}
	smCtx := ctx.(*parser.ShowMessageStatementContext)

	stmt := &ast.ShowMessageStmt{
		Type: "Information", // Default message type
	}

	if expr := smCtx.Expression(); expr != nil {
		stmt.Message = buildSourceExpression(expr)
	}

	if id := smCtx.IdentifierOrKeyword(); id != nil {
		stmt.Type = id.GetText()
	}

	// Build template arguments (optional)
	if exprList := smCtx.ExpressionList(); exprList != nil {
		listCtx := exprList.(*parser.ExpressionListContext)
		allExprs := listCtx.AllExpression()
		for i, expr := range allExprs {
			value := buildSourceExpression(expr)
			value = appendExpressionListTrailingWhitespace(listCtx, nextParserRuleContext(allExprs, i), expr, value)
			stmt.TemplateArgs = append(stmt.TemplateArgs, value)
		}
	}

	return stmt
}

// buildDownloadFileStatement converts downloadFileStatement context to DownloadFileStmt.
func buildDownloadFileStatement(ctx parser.IDownloadFileStatementContext) *ast.DownloadFileStmt {
	if ctx == nil {
		return nil
	}
	dlCtx := ctx.(*parser.DownloadFileStatementContext)
	stmt := &ast.DownloadFileStmt{}
	if variable := dlCtx.VARIABLE(); variable != nil {
		stmt.FileDocument = strings.TrimPrefix(variable.GetText(), "$")
	}
	stmt.ShowInBrowser = dlCtx.BROWSER() != nil
	if errClause := dlCtx.OnErrorClause(); errClause != nil {
		stmt.ErrorHandling = buildOnErrorClause(errClause)
	}
	return stmt
}

// buildValidationFeedbackStatement converts validationFeedbackStatement context to ValidationFeedbackStmt.
// Grammar: VALIDATION FEEDBACK attributePath MESSAGE expression (OBJECTS LBRACKET expressionList RBRACKET)?
func buildValidationFeedbackStatement(ctx parser.IValidationFeedbackStatementContext) *ast.ValidationFeedbackStmt {
	if ctx == nil {
		return nil
	}
	vfCtx := ctx.(*parser.ValidationFeedbackStatementContext)

	stmt := &ast.ValidationFeedbackStmt{}

	// Build attribute path
	if attrPath := vfCtx.AttributePath(); attrPath != nil {
		stmt.AttributePath = buildAttributePathFromContext(attrPath)
	} else if variable := vfCtx.VARIABLE(); variable != nil {
		stmt.AttributePath = &ast.AttributePathExpr{
			Variable: strings.TrimPrefix(variable.GetText(), "$"),
		}
	}

	// Build message expression
	if msgExpr := vfCtx.Expression(); msgExpr != nil {
		stmt.Message = buildSourceExpression(msgExpr)
	}

	// Build template arguments (optional)
	if exprList := vfCtx.ExpressionList(); exprList != nil {
		listCtx := exprList.(*parser.ExpressionListContext)
		allExprs := listCtx.AllExpression()
		for i, expr := range allExprs {
			value := buildSourceExpression(expr)
			value = appendExpressionListTrailingWhitespace(listCtx, nextParserRuleContext(allExprs, i), expr, value)
			stmt.TemplateArgs = append(stmt.TemplateArgs, value)
		}
	}

	return stmt
}

// attributePathTargetText reconstructs the normalized "$Variable<sep>Segment..."
// text of an attribute path with each qualified-name segment unquoted. Used for
// SET targets, whose string form is later split on '/' to derive the member
// (attribute or association) name; carrying quotes through would corrupt the
// serialized member identifier.
func attributePathTargetText(ctx parser.IAttributePathContext) string {
	ap := buildAttributePathFromContext(ctx)
	if ap == nil {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("$")
	sb.WriteString(ap.Variable)
	for _, seg := range ap.Segments {
		sb.WriteString(seg.Separator)
		sb.WriteString(seg.Name)
	}
	return sb.String()
}

// buildAttributePathFromContext builds an AttributePathExpr from attributePath context.
// Grammar: VARIABLE ((SLASH | DOT) qualifiedName)+
// attributePath is shared by SET, LOOP, aggregate expressions, and validation
// feedback targets, so keep separator capture generic rather than specializing
// it for validation feedback.
// Iterates children in order to preserve the separator (/ vs .) for each segment.
func buildAttributePathFromContext(ctx parser.IAttributePathContext) *ast.AttributePathExpr {
	if ctx == nil {
		return nil
	}
	apCtx := ctx.(*parser.AttributePathContext)

	result := &ast.AttributePathExpr{}

	// Get variable name (first element)
	if v := apCtx.VARIABLE(); v != nil {
		result.Variable = strings.TrimPrefix(v.GetText(), "$")
	}

	// Iterate children in order to capture separator-segment pairs.
	// Pattern: VARIABLE (separator segment)+ where separator is SLASH or DOT
	lastSep := "/"
	for _, child := range apCtx.GetChildren() {
		if tn, ok := child.(antlr.TerminalNode); ok {
			switch tn.GetSymbol().GetTokenType() {
			case parser.MDLParserSLASH:
				lastSep = "/"
			case parser.MDLParserDOT:
				lastSep = "."
			case parser.MDLParserIDENTIFIER:
				name := tn.GetText()
				result.Path = append(result.Path, name)
				result.Segments = append(result.Segments, ast.PathSegment{Name: name, Separator: lastSep})
			}
		} else if qn, ok := child.(parser.IQualifiedNameContext); ok {
			// Unquote each segment: a quoted member/association name like
			// BuildScheduling."SkillProfile_Resource" must resolve to the bare
			// identifier, otherwise the quotes leak into the serialized
			// Change/Set member and produce an invalid AttributeIdentifier that
			// passes `mxcli check` but fails to load in MxBuild/Studio Pro.
			name := getQualifiedNameText(qn)
			result.Path = append(result.Path, name)
			result.Segments = append(result.Segments, ast.PathSegment{Name: name, Separator: lastSep})
		}
	}

	return result
}

// ============================================================================
// REST Call Statements
// ============================================================================

// buildRestCallStatement converts REST CALL statement context to RestCallStmt.
// Grammar: (VARIABLE EQUALS)? REST CALL httpMethod restCallUrl restCallUrlParams?
//
//	restCallHeaderClause* restCallAuthClause? restCallBodyClause?
//	restCallTimeoutClause? restCallReturnsClause onErrorClause?
func buildRestCallStatement(ctx parser.IRestCallStatementContext) *ast.RestCallStmt {
	if ctx == nil {
		return nil
	}
	restCtx := ctx.(*parser.RestCallStatementContext)

	stmt := &ast.RestCallStmt{}

	// Get output variable if present
	if v := restCtx.VARIABLE(); v != nil {
		stmt.OutputVariable = strings.TrimPrefix(v.GetText(), "$")
	}

	// Get HTTP method
	if method := restCtx.HttpMethod(); method != nil {
		methodCtx := method.(*parser.HttpMethodContext)
		if methodCtx.GET() != nil {
			stmt.Method = ast.HttpMethodGet
		} else if methodCtx.POST() != nil {
			stmt.Method = ast.HttpMethodPost
		} else if methodCtx.PUT() != nil {
			stmt.Method = ast.HttpMethodPut
		} else if methodCtx.PATCH() != nil {
			stmt.Method = ast.HttpMethodPatch
		} else if methodCtx.DELETE() != nil {
			stmt.Method = ast.HttpMethodDelete
		}
	}

	// Get URL
	if urlCtx := restCtx.RestCallUrl(); urlCtx != nil {
		urlC := urlCtx.(*parser.RestCallUrlContext)
		if strLit := urlC.STRING_LITERAL(); strLit != nil {
			stmt.URL = &ast.LiteralExpr{
				Kind:  ast.LiteralString,
				Value: unquoteString(strLit.GetText()),
			}
		} else if expr := urlC.Expression(); expr != nil {
			stmt.URL = buildSourceExpression(expr)
		}
	}

	// Get URL template parameters
	if urlParams := restCtx.RestCallUrlParams(); urlParams != nil {
		paramsCtx := urlParams.(*parser.RestCallUrlParamsContext)
		if tplParams := paramsCtx.TemplateParams(); tplParams != nil {
			stmt.URLParams = buildTemplateParams(tplParams)
		}
	}

	// Get headers
	for _, headerClause := range restCtx.AllRestCallHeaderClause() {
		hdrCtx := headerClause.(*parser.RestCallHeaderClauseContext)
		header := ast.RestHeader{}
		if id := hdrCtx.IDENTIFIER(); id != nil {
			header.Name = id.GetText()
		} else if strLit := hdrCtx.STRING_LITERAL(); strLit != nil {
			// Handle quoted header names like 'Content-Type'
			header.Name = unquoteString(strLit.GetText())
		}
		if expr := hdrCtx.Expression(); expr != nil {
			header.Value = buildSourceExpression(expr)
		}
		stmt.Headers = append(stmt.Headers, header)
	}

	// Get auth clause
	if authClause := restCtx.RestCallAuthClause(); authClause != nil {
		authCtx := authClause.(*parser.RestCallAuthClauseContext)
		exprs := authCtx.AllExpression()
		if len(exprs) >= 2 {
			stmt.Auth = &ast.RestAuth{
				Username: buildSourceExpression(exprs[0]),
				Password: buildSourceExpression(exprs[1]),
			}
		}
	}

	// Get body clause
	if bodyClause := restCtx.RestCallBodyClause(); bodyClause != nil {
		bodyCtx := bodyClause.(*parser.RestCallBodyClauseContext)
		body := &ast.RestBody{}

		if bodyCtx.MAPPING() != nil {
			// Export mapping: BODY MAPPING QualifiedName FROM $Variable
			body.Type = ast.RestBodyMapping
			if qn := bodyCtx.QualifiedName(); qn != nil {
				body.MappingName = buildQualifiedName(qn)
			}
			if v := bodyCtx.VARIABLE(); v != nil {
				body.SourceVariable = strings.TrimPrefix(v.GetText(), "$")
			}
		} else {
			// Custom body template
			body.Type = ast.RestBodyCustom
			if strLit := bodyCtx.STRING_LITERAL(); strLit != nil {
				body.Template = &ast.LiteralExpr{
					Kind:  ast.LiteralString,
					Value: unquoteString(strLit.GetText()),
				}
			} else if expr := bodyCtx.Expression(); expr != nil {
				body.Template = buildSourceExpression(expr)
			}
			// Get template parameters
			if tplParams := bodyCtx.TemplateParams(); tplParams != nil {
				body.TemplateParams = buildTemplateParams(tplParams)
			}
		}

		stmt.Body = body
	}

	// Get timeout clause
	if timeoutClause := restCtx.RestCallTimeoutClause(); timeoutClause != nil {
		timeoutCtx := timeoutClause.(*parser.RestCallTimeoutClauseContext)
		if expr := timeoutCtx.Expression(); expr != nil {
			stmt.Timeout = buildSourceExpression(expr)
		}
	}

	// Get returns clause
	if returnsClause := restCtx.RestCallReturnsClause(); returnsClause != nil {
		returnsCtx := returnsClause.(*parser.RestCallReturnsClauseContext)
		result := ast.RestResult{}

		if returnsCtx.STRING_TYPE() != nil {
			result.Type = ast.RestResultString
		} else if returnsCtx.RESPONSE() != nil {
			result.Type = ast.RestResultResponse
		} else if returnsCtx.MAPPING() != nil {
			result.Type = ast.RestResultMapping
			qns := returnsCtx.AllQualifiedName()
			if len(qns) >= 1 {
				result.MappingName = buildQualifiedName(qns[0])
			}
			if len(qns) >= 2 {
				result.ResultEntity = buildQualifiedName(qns[1])
			}
			// `as list of Module.Entity` marks the mapping result as a list;
			// without LIST_OF the result is a single object.
			if returnsCtx.LIST_OF() != nil {
				result.IsList = true
			}
		} else if returnsCtx.NONE() != nil || returnsCtx.NOTHING() != nil {
			result.Type = ast.RestResultNone
		}

		stmt.Result = result
	}

	// Get error handling clause
	if errClause := restCtx.OnErrorClause(); errClause != nil {
		stmt.ErrorHandling = buildOnErrorClause(errClause)
	}

	return stmt
}

// buildSendRestRequestStatement builds a SendRestRequestStmt from the parser context.
// Syntax: [$Var =] SEND REST REQUEST Module.Service.Operation [BODY $var] [ON ERROR ...]
func buildSendRestRequestStatement(ctx parser.ISendRestRequestStatementContext) *ast.SendRestRequestStmt {
	if ctx == nil {
		return nil
	}
	sendCtx := ctx.(*parser.SendRestRequestStatementContext)

	stmt := &ast.SendRestRequestStmt{}

	// Output variable
	if v := sendCtx.VARIABLE(); v != nil {
		stmt.OutputVariable = strings.TrimPrefix(v.GetText(), "$")
	}

	// Operation qualified name (Module.Service.Operation)
	if qn := sendCtx.QualifiedName(); qn != nil {
		stmt.Operation = buildQualifiedName(qn)
	}

	// WITH clause (parameter bindings)
	if withClause := sendCtx.SendRestRequestWithClause(); withClause != nil {
		wc := withClause.(*parser.SendRestRequestWithClauseContext)
		for _, paramCtx := range wc.AllSendRestRequestParam() {
			pc := paramCtx.(*parser.SendRestRequestParamContext)
			param := ast.SendRestParamDef{}
			if v := pc.VARIABLE(); v != nil {
				param.Name = strings.TrimPrefix(v.GetText(), "$")
			}
			if expr := pc.Expression(); expr != nil {
				param.Expression = expr.GetText()
			}
			stmt.Parameters = append(stmt.Parameters, param)
		}
	}

	// Body clause
	if bodyClause := sendCtx.SendRestRequestBodyClause(); bodyClause != nil {
		bc := bodyClause.(*parser.SendRestRequestBodyClauseContext)
		if v := bc.VARIABLE(); v != nil {
			stmt.BodyVariable = strings.TrimPrefix(v.GetText(), "$")
		}
	}

	// Error handling
	if errClause := sendCtx.OnErrorClause(); errClause != nil {
		stmt.ErrorHandling = buildOnErrorClause(errClause)
	}

	return stmt
}
