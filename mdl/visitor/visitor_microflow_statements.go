// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"strings"

	"github.com/antlr4-go/antlr/v4"
	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/grammar/parser"
)

// buildMicroflowBody converts microflow body context to MicroflowStatement slice.
func buildMicroflowBody(ctx parser.IMicroflowBodyContext) []ast.MicroflowStatement {
	if ctx == nil {
		return nil
	}
	bodyCtx := ctx.(*parser.MicroflowBodyContext)
	var stmts []ast.MicroflowStatement

	for _, stmtCtx := range bodyCtx.AllMicroflowStatement() {
		stmt := buildMicroflowStatement(stmtCtx)
		if stmt != nil {
			stmts = append(stmts, stmt)
		}
	}

	return stmts
}

// buildMicroflowStatement converts a microflow statement context to an AST node.
func buildMicroflowStatement(ctx parser.IMicroflowStatementContext) ast.MicroflowStatement {
	if ctx == nil {
		return nil
	}
	mfCtx := ctx.(*parser.MicroflowStatementContext)

	// Extract annotations from the statement context
	ann := extractMicroflowAnnotations(mfCtx.AllAnnotation())

	var stmt ast.MicroflowStatement

	// Check each statement type
	if decl := mfCtx.DeclareStatement(); decl != nil {
		stmt = buildDeclareStatement(decl)
	} else if caseStmt := mfCtx.CaseStatement(); caseStmt != nil {
		stmt = buildCaseStatement(caseStmt)
	} else if split := mfCtx.InheritanceSplitStatement(); split != nil {
		stmt = buildInheritanceSplitStatement(split)
	} else if cast := mfCtx.CastObjectStatement(); cast != nil {
		stmt = buildCastObjectStatement(cast)
	} else if set := mfCtx.SetStatement(); set != nil {
		stmt = buildSetStatement(set)
	} else if createList := mfCtx.CreateListStatement(); createList != nil {
		// Check createListStatement before createObjectStatement to properly match "CREATE LIST OF"
		stmt = buildCreateListStatement(createList)
	} else if create := mfCtx.CreateObjectStatement(); create != nil {
		stmt = buildCreateObjectStatement(create)
	} else if change := mfCtx.ChangeObjectStatement(); change != nil {
		stmt = buildChangeObjectStatement(change)
	} else if commit := mfCtx.CommitStatement(); commit != nil {
		stmt = buildCommitStatement(commit)
	} else if del := mfCtx.DeleteObjectStatement(); del != nil {
		stmt = buildDeleteObjectStatement(del)
	} else if rollback := mfCtx.RollbackStatement(); rollback != nil {
		stmt = buildRollbackStatement(rollback)
	} else if retr := mfCtx.RetrieveStatement(); retr != nil {
		stmt = buildRetrieveStatement(retr)
	} else if ifStmt := mfCtx.IfStatement(); ifStmt != nil {
		stmt = buildIfStatement(ifStmt)
	} else if loop := mfCtx.LoopStatement(); loop != nil {
		stmt = buildLoopStatement(loop)
	} else if ws := mfCtx.WhileStatement(); ws != nil {
		stmt = buildWhileStatement(ws)
	} else if ret := mfCtx.ReturnStatement(); ret != nil {
		stmt = buildReturnStatement(ret)
	} else if mfCtx.RaiseErrorStatement() != nil {
		stmt = &ast.RaiseErrorStmt{}
	} else if log := mfCtx.LogStatement(); log != nil {
		stmt = buildLogStatement(log)
	} else if call := mfCtx.CallMicroflowStatement(); call != nil {
		stmt = buildCallMicroflowStatement(call)
	} else if call := mfCtx.CallNanoflowStatement(); call != nil {
		stmt = buildCallNanoflowStatement(call)
	} else if call := mfCtx.CallJavaActionStatement(); call != nil {
		stmt = buildCallJavaActionStatement(call)
	} else if call := mfCtx.CallJavaScriptActionStatement(); call != nil {
		stmt = buildCallJavaScriptActionStatement(call)
	} else if call := mfCtx.CallWebServiceStatement(); call != nil {
		stmt = buildCallWebServiceStatement(call)
	} else if call := mfCtx.ExecuteDatabaseQueryStatement(); call != nil {
		stmt = buildExecuteDatabaseQueryStatement(call)
	} else if call := mfCtx.CallExternalActionStatement(); call != nil {
		stmt = buildCallExternalActionStatement(call)
	} else if mfCtx.BreakStatement() != nil {
		stmt = &ast.BreakStmt{}
	} else if mfCtx.ContinueStatement() != nil {
		stmt = &ast.ContinueStmt{}
	} else if listOp := mfCtx.ListOperationStatement(); listOp != nil {
		stmt = buildListOperationStatement(listOp)
	} else if aggr := mfCtx.AggregateListStatement(); aggr != nil {
		stmt = buildAggregateListStatement(aggr)
	} else if addTo := mfCtx.AddToListStatement(); addTo != nil {
		stmt = buildAddToListStatement(addTo)
	} else if removeFrom := mfCtx.RemoveFromListStatement(); removeFrom != nil {
		stmt = buildRemoveFromListStatement(removeFrom)
	} else if showPage := mfCtx.ShowPageStatement(); showPage != nil {
		stmt = buildShowPageStatement(showPage)
	} else if mfCtx.ClosePageStatement() != nil {
		stmt = &ast.ClosePageStmt{NumberOfPages: 1}
	} else if mfCtx.ShowHomePageStatement() != nil {
		stmt = &ast.ShowHomePageStmt{}
	} else if showMsg := mfCtx.ShowMessageStatement(); showMsg != nil {
		stmt = buildShowMessageStatement(showMsg)
	} else if download := mfCtx.DownloadFileStatement(); download != nil {
		stmt = buildDownloadFileStatement(download)
	} else if valFeedback := mfCtx.ValidationFeedbackStatement(); valFeedback != nil {
		stmt = buildValidationFeedbackStatement(valFeedback)
	} else if restCall := mfCtx.RestCallStatement(); restCall != nil {
		stmt = buildRestCallStatement(restCall)
	} else if sendRest := mfCtx.SendRestRequestStatement(); sendRest != nil {
		stmt = buildSendRestRequestStatement(sendRest)
	} else if importMapping := mfCtx.ImportFromMappingStatement(); importMapping != nil {
		stmt = buildImportFromMappingStatement(importMapping)
	} else if exportMapping := mfCtx.ExportToMappingStatement(); exportMapping != nil {
		stmt = buildExportToMappingStatement(exportMapping)
	} else if transformJson := mfCtx.TransformJsonStatement(); transformJson != nil {
		stmt = buildTransformJsonStatement(transformJson)
	} else if callWf := mfCtx.CallWorkflowStatement(); callWf != nil {
		stmt = buildCallWorkflowStatement(callWf)
	} else if getWfData := mfCtx.GetWorkflowDataStatement(); getWfData != nil {
		stmt = buildGetWorkflowDataStatement(getWfData)
	} else if getWfs := mfCtx.GetWorkflowsStatement(); getWfs != nil {
		stmt = buildGetWorkflowsStatement(getWfs)
	} else if getWfRecords := mfCtx.GetWorkflowActivityRecordsStatement(); getWfRecords != nil {
		stmt = buildGetWorkflowActivityRecordsStatement(getWfRecords)
	} else if wfOp := mfCtx.WorkflowOperationStatement(); wfOp != nil {
		stmt = buildWorkflowOperationStatement(wfOp)
	} else if setOutcome := mfCtx.SetTaskOutcomeStatement(); setOutcome != nil {
		stmt = buildSetTaskOutcomeStatement(setOutcome)
	} else if openTask := mfCtx.OpenUserTaskStatement(); openTask != nil {
		stmt = buildOpenUserTaskStatement(openTask)
	} else if notifyWf := mfCtx.NotifyWorkflowStatement(); notifyWf != nil {
		stmt = buildNotifyWorkflowStatement(notifyWf)
	} else if openWf := mfCtx.OpenWorkflowStatement(); openWf != nil {
		stmt = buildOpenWorkflowStatement(openWf)
	} else if lockWf := mfCtx.LockWorkflowStatement(); lockWf != nil {
		stmt = buildLockWorkflowStatement(lockWf)
	} else if unlockWf := mfCtx.UnlockWorkflowStatement(); unlockWf != nil {
		stmt = buildUnlockWorkflowStatement(unlockWf)
	}

	// Attach annotations to the statement
	if stmt != nil && ann != nil {
		setStatementAnnotations(stmt, ann)
	}

	return stmt
}

func buildCaseStatement(ctx parser.ICaseStatementContext) *ast.EnumSplitStmt {
	if ctx == nil {
		return nil
	}
	caseCtx := ctx.(*parser.CaseStatementContext)

	stmt := &ast.EnumSplitStmt{}
	if source := caseCtx.EnumSplitSource(); source != nil {
		sourceCtx := source.(*parser.EnumSplitSourceContext)
		if attr := sourceCtx.AttributePath(); attr != nil {
			stmt.Variable = strings.TrimPrefix(attr.GetText(), "$")
		} else if variable := sourceCtx.VARIABLE(); variable != nil {
			stmt.Variable = strings.TrimPrefix(variable.GetText(), "$")
		}
	}

	// Reconstruct per-WHEN groups from the flat child list.
	// Grammar: (WHEN caseValue (, caseValue)* THEN microflowBody)+ (ELSE microflowBody)?
	// AllEnumSplitCaseValue() is flat across all WHEN clauses, so we walk children
	// and bucket values by their nearest preceding WHEN token.
	type whenGroup struct{ values []string }
	var groups []whenGroup
	var cur *whenGroup
	for _, child := range caseCtx.GetChildren() {
		switch c := child.(type) {
		case antlr.TerminalNode:
			if c.GetSymbol().GetTokenType() == parser.MDLParserWHEN {
				groups = append(groups, whenGroup{})
				cur = &groups[len(groups)-1]
			}
		case parser.IEnumSplitCaseValueContext:
			if cur != nil {
				cur.values = append(cur.values, enumSplitCaseValueText(c))
			}
		}
	}

	bodies := caseCtx.AllMicroflowBody()
	for i, g := range groups {
		if len(g.values) == 0 || i >= len(bodies) {
			continue
		}
		stmt.Cases = append(stmt.Cases, ast.EnumSplitCase{
			Value:  g.values[0],
			Values: g.values,
			Body:   buildMicroflowBody(bodies[i]),
		})
	}

	if caseCtx.ELSE() != nil && len(bodies) > len(groups) {
		stmt.ElseBody = buildMicroflowBody(bodies[len(bodies)-1])
	}

	return stmt
}

func enumSplitCaseValueText(ctx parser.IEnumSplitCaseValueContext) string {
	if ctx == nil {
		return ""
	}
	if strings.EqualFold(ctx.GetText(), "(empty)") {
		return "(empty)"
	}
	return ctx.GetText()
}

// extractMicroflowAnnotations extracts activity annotations from annotation contexts.
// Handles @position(x, y), @caption 'text', @color Green, @annotation 'text'.
func extractMicroflowAnnotations(annotations []parser.IAnnotationContext) *ast.ActivityAnnotations {
	if len(annotations) == 0 {
		return nil
	}

	result := &ast.ActivityAnnotations{}
	hasAny := false

	seenActivityMetadata := false
	for i, annCtx := range annotations {
		ann := annCtx.(*parser.AnnotationContext)
		annName := strings.ToLower(ann.AnnotationName().GetText())

		switch annName {
		case "position":
			// @position(x, y) — uses parenthesized params
			if params := ann.AnnotationParams(); params != nil {
				paramsCtx := params.(*parser.AnnotationParamsContext)
				allParams := paramsCtx.AllAnnotationParam()
				if len(allParams) >= 2 {
					x := parseAnnotationParamInt(allParams[0])
					y := parseAnnotationParamInt(allParams[1])
					result.Position = &ast.Position{X: x, Y: y}
					hasAny = true
				}
			}
			seenActivityMetadata = true

		case "caption":
			// @caption 'text' — bare annotationValue
			if valCtx := ann.AnnotationValue(); valCtx != nil {
				text := extractAnnotationValueString(valCtx)
				if text != "" {
					result.Caption = text
					hasAny = true
				}
			}
			seenActivityMetadata = true

		case "color":
			// @color Green — bare annotationValue (identifier)
			if valCtx := ann.AnnotationValue(); valCtx != nil {
				text := extractAnnotationValueIdentifier(valCtx)
				if text != "" {
					result.Color = text
					hasAny = true
				}
			}
			seenActivityMetadata = true

		case "annotation":
			// @annotation 'text' — bare annotationValue
			if valCtx := ann.AnnotationValue(); valCtx != nil {
				text := extractAnnotationValueString(valCtx)
				if text != "" {
					if !seenActivityMetadata && hasLaterActivityAnnotation(annotations, i+1) {
						result.FreeAnnotations = append(result.FreeAnnotations, text)
					} else {
						result.AnnotationText = text
					}
					hasAny = true
				}
			}

		case "excluded":
			// @excluded — no value needed
			result.Excluded = true
			hasAny = true
			seenActivityMetadata = true

		case "anchor":
			// @anchor(from: right, to: left) — simple form for the outgoing flow.
			// @anchor(true: (from: right, to: left), false: (from: bottom, to: left))
			//   — split form for IF statements.
			// @anchor(iterator: (from: ..., to: ...), tail: (from: ..., to: ...))
			//   — loop form for LOOP/WHILE body flows.
			if params := ann.AnnotationParams(); params != nil {
				parseAnchorAnnotation(params.(*parser.AnnotationParamsContext), result)
				hasAny = true
			}
			seenActivityMetadata = true
		}
	}

	if !hasAny {
		return nil
	}
	return result
}

func hasLaterActivityAnnotation(annotations []parser.IAnnotationContext, start int) bool {
	for _, annCtx := range annotations[start:] {
		ann := annCtx.(*parser.AnnotationContext)
		switch strings.ToLower(ann.AnnotationName().GetText()) {
		case "position", "caption", "color", "excluded", "anchor":
			return true
		}
	}
	return false
}

// parseAnchorAnnotation populates Anchor / TrueBranchAnchor / FalseBranchAnchor /
// IteratorAnchor / BodyTailAnchor fields on result from the @anchor(...) params.
func parseAnchorAnnotation(params *parser.AnnotationParamsContext, result *ast.ActivityAnnotations) {
	flat := &ast.FlowAnchors{From: ast.AnchorSideUnset, To: ast.AnchorSideUnset}
	flatSet := false

	for _, p := range params.AllAnnotationParam() {
		pCtx := p.(*parser.AnnotationParamContext)
		nameCtx := pCtx.AnnotationParamName()
		if nameCtx == nil {
			continue // positional form not supported for @anchor
		}
		key := strings.ToLower(nameCtx.GetText())

		switch key {
		case "from":
			if side, ok := parseAnchorSideFromValue(pCtx.AnnotationValue()); ok {
				flat.From = side
				flatSet = true
			}
		case "to":
			if side, ok := parseAnchorSideFromValue(pCtx.AnnotationValue()); ok {
				flat.To = side
				flatSet = true
			}
		case "true":
			if nested := pCtx.AnnotationParenValue(); nested != nil {
				result.TrueBranchAnchor = parseNestedFlowAnchors(nested.(*parser.AnnotationParenValueContext))
			}
		case "false":
			if nested := pCtx.AnnotationParenValue(); nested != nil {
				result.FalseBranchAnchor = parseNestedFlowAnchors(nested.(*parser.AnnotationParenValueContext))
			}
		case "iterator":
			if nested := pCtx.AnnotationParenValue(); nested != nil {
				result.IteratorAnchor = parseNestedFlowAnchors(nested.(*parser.AnnotationParenValueContext))
			}
		case "tail":
			if nested := pCtx.AnnotationParenValue(); nested != nil {
				result.BodyTailAnchor = parseNestedFlowAnchors(nested.(*parser.AnnotationParenValueContext))
			}
		}
	}

	if flatSet {
		result.Anchor = flat
	}
}

// parseNestedFlowAnchors parses a `(from: X, to: Y)` sub-expression into FlowAnchors.
func parseNestedFlowAnchors(p *parser.AnnotationParenValueContext) *ast.FlowAnchors {
	inner := p.AnnotationParams()
	if inner == nil {
		return nil
	}
	fa := &ast.FlowAnchors{From: ast.AnchorSideUnset, To: ast.AnchorSideUnset}
	set := false
	for _, pp := range inner.(*parser.AnnotationParamsContext).AllAnnotationParam() {
		ppCtx := pp.(*parser.AnnotationParamContext)
		nameCtx := ppCtx.AnnotationParamName()
		if nameCtx == nil {
			continue
		}
		key := strings.ToLower(nameCtx.GetText())
		side, ok := parseAnchorSideFromValue(ppCtx.AnnotationValue())
		if !ok {
			continue
		}
		switch key {
		case "from":
			fa.From = side
			set = true
		case "to":
			fa.To = side
			set = true
		}
	}
	if !set {
		return nil
	}
	return fa
}

// parseAnchorSideFromValue extracts a side keyword from an annotationValue.
// Accepts top | right | bottom | left.
func parseAnchorSideFromValue(val parser.IAnnotationValueContext) (ast.AnchorSide, bool) {
	if val == nil {
		return ast.AnchorSideUnset, false
	}
	valCtx := val.(*parser.AnnotationValueContext)
	if as := valCtx.AnchorSide(); as != nil {
		switch strings.ToLower(as.GetText()) {
		case "top":
			return ast.AnchorSideTop, true
		case "right":
			return ast.AnchorSideRight, true
		case "bottom":
			return ast.AnchorSideBottom, true
		case "left":
			return ast.AnchorSideLeft, true
		}
	}
	// Fallback — accept plain identifier via qualifiedName for user robustness.
	if qn := valCtx.QualifiedName(); qn != nil {
		switch strings.ToLower(qn.GetText()) {
		case "top":
			return ast.AnchorSideTop, true
		case "right":
			return ast.AnchorSideRight, true
		case "bottom":
			return ast.AnchorSideBottom, true
		case "left":
			return ast.AnchorSideLeft, true
		}
	}
	return ast.AnchorSideUnset, false
}

// extractAnnotationValueString extracts a string value from an annotationValue context.
func extractAnnotationValueString(ctx parser.IAnnotationValueContext) string {
	valCtx := ctx.(*parser.AnnotationValueContext)
	if lit := valCtx.Literal(); lit != nil {
		litCtx := lit.(*parser.LiteralContext)
		if litCtx.STRING_LITERAL() != nil {
			return unquoteString(litCtx.STRING_LITERAL().GetText())
		}
	}
	// Also try expression — it might be a string literal parsed as expression
	if expr := valCtx.Expression(); expr != nil {
		text := expr.GetText()
		if len(text) >= 2 && text[0] == '\'' && text[len(text)-1] == '\'' {
			return unquoteString(text)
		}
	}
	return ""
}

// extractAnnotationValueIdentifier extracts an identifier value from an annotationValue context.
func extractAnnotationValueIdentifier(ctx parser.IAnnotationValueContext) string {
	valCtx := ctx.(*parser.AnnotationValueContext)
	// Try qualifiedName first (handles plain identifiers like "Green")
	if qn := valCtx.QualifiedName(); qn != nil {
		return qn.GetText()
	}
	// Try expression (might be a plain identifier)
	if expr := valCtx.Expression(); expr != nil {
		return expr.GetText()
	}
	// Try literal
	if lit := valCtx.Literal(); lit != nil {
		return lit.GetText()
	}
	return ""
}

// setStatementAnnotations sets the Annotations field on a microflow statement via type switch.
func setStatementAnnotations(stmt ast.MicroflowStatement, ann *ast.ActivityAnnotations) {
	switch s := stmt.(type) {
	case *ast.DeclareStmt:
		s.Annotations = ann
	case *ast.EnumSplitStmt:
	case *ast.InheritanceSplitStmt:
		s.Annotations = ann
	case *ast.CastObjectStmt:
		s.Annotations = ann
	case *ast.MfSetStmt:
		s.Annotations = ann
	case *ast.ReturnStmt:
		s.Annotations = ann
	case *ast.RaiseErrorStmt:
		s.Annotations = ann
	case *ast.CreateObjectStmt:
		s.Annotations = ann
	case *ast.ChangeObjectStmt:
		s.Annotations = ann
	case *ast.MfCommitStmt:
		s.Annotations = ann
	case *ast.DeleteObjectStmt:
		s.Annotations = ann
	case *ast.RollbackStmt:
		s.Annotations = ann
	case *ast.RetrieveStmt:
		s.Annotations = ann
	case *ast.IfStmt:
		s.Annotations = ann
	case *ast.LoopStmt:
		s.Annotations = ann
	case *ast.WhileStmt:
		s.Annotations = ann
	case *ast.LogStmt:
		s.Annotations = ann
	case *ast.CallMicroflowStmt:
		s.Annotations = ann
	case *ast.CallNanoflowStmt:
		s.Annotations = ann
	case *ast.CallJavaActionStmt:
		s.Annotations = ann
	case *ast.CallJavaScriptActionStmt:
		s.Annotations = ann
	case *ast.CallWebServiceStmt:
		s.Annotations = ann
	case *ast.ExecuteDatabaseQueryStmt:
		s.Annotations = ann
	case *ast.CallExternalActionStmt:
		s.Annotations = ann
	case *ast.BreakStmt:
		s.Annotations = ann
	case *ast.ContinueStmt:
		s.Annotations = ann
	case *ast.ListOperationStmt:
		s.Annotations = ann
	case *ast.AggregateListStmt:
		s.Annotations = ann
	case *ast.CreateListStmt:
		s.Annotations = ann
	case *ast.AddToListStmt:
		s.Annotations = ann
	case *ast.RemoveFromListStmt:
		s.Annotations = ann
	case *ast.ShowPageStmt:
		s.Annotations = ann
	case *ast.ClosePageStmt:
		s.Annotations = ann
	case *ast.ShowHomePageStmt:
		s.Annotations = ann
	case *ast.ShowMessageStmt:
		s.Annotations = ann
	case *ast.DownloadFileStmt:
		s.Annotations = ann
	case *ast.ValidationFeedbackStmt:
		s.Annotations = ann
	case *ast.RestCallStmt:
		s.Annotations = ann
	case *ast.SendRestRequestStmt:
		s.Annotations = ann
	}
}

// buildOnErrorClause converts an OnErrorClauseContext to an ErrorHandlingClause.
func buildOnErrorClause(ctx parser.IOnErrorClauseContext) *ast.ErrorHandlingClause {
	if ctx == nil {
		return nil
	}
	errCtx := ctx.(*parser.OnErrorClauseContext)

	if errCtx.CONTINUE() != nil {
		return &ast.ErrorHandlingClause{Type: ast.ErrorHandlingContinue}
	}
	if errCtx.ROLLBACK() != nil && errCtx.LBRACE() == nil {
		return &ast.ErrorHandlingClause{Type: ast.ErrorHandlingRollback}
	}
	if errCtx.LBRACE() != nil {
		body := buildMicroflowBody(errCtx.MicroflowBody())
		if errCtx.WITHOUT() != nil {
			return &ast.ErrorHandlingClause{Type: ast.ErrorHandlingCustomWithoutRollback, Body: body}
		}
		return &ast.ErrorHandlingClause{Type: ast.ErrorHandlingCustom, Body: body}
	}
	return nil
}

// buildDeclareStatement converts DECLARE statement context to DeclareStmt.
func buildDeclareStatement(ctx parser.IDeclareStatementContext) *ast.DeclareStmt {
	if ctx == nil {
		return nil
	}
	declCtx := ctx.(*parser.DeclareStatementContext)

	stmt := &ast.DeclareStmt{}

	// Get variable name
	if v := declCtx.VARIABLE(); v != nil {
		stmt.Variable = strings.TrimPrefix(v.GetText(), "$")
	}

	// Get type
	if dt := declCtx.DataType(); dt != nil {
		stmt.Type = buildDataType(dt)
	}

	// Get optional initial value
	if expr := declCtx.Expression(); expr != nil {
		stmt.InitialValue = buildSourceExpression(expr)
		stmt.InitialValue = appendStatementExpressionTrailingWhitespace(expr, stmt.InitialValue)
	}

	return stmt
}

func buildInheritanceSplitStatement(ctx parser.IInheritanceSplitStatementContext) *ast.InheritanceSplitStmt {
	if ctx == nil {
		return nil
	}
	splitCtx := ctx.(*parser.InheritanceSplitStatementContext)
	stmt := &ast.InheritanceSplitStmt{}
	if v := splitCtx.VARIABLE(); v != nil {
		stmt.Variable = strings.TrimPrefix(v.GetText(), "$")
	}
	for _, caseCtx := range splitCtx.AllInheritanceSplitCase() {
		c := caseCtx.(*parser.InheritanceSplitCaseContext)
		stmt.Cases = append(stmt.Cases, ast.InheritanceSplitCase{
			Entity: buildQualifiedName(c.QualifiedName()),
			Body:   buildMicroflowBody(c.MicroflowBody()),
		})
	}
	if splitCtx.ELSE() != nil {
		stmt.ElseBody = buildMicroflowBody(splitCtx.MicroflowBody())
	}
	return stmt
}

func buildCastObjectStatement(ctx parser.ICastObjectStatementContext) *ast.CastObjectStmt {
	if ctx == nil {
		return nil
	}
	castCtx := ctx.(*parser.CastObjectStatementContext)
	stmt := &ast.CastObjectStmt{}
	vars := castCtx.AllVARIABLE()
	if len(vars) == 1 {
		stmt.OutputVariable = strings.TrimPrefix(vars[0].GetText(), "$")
		return stmt
	}
	if len(vars) > 0 {
		stmt.OutputVariable = strings.TrimPrefix(vars[0].GetText(), "$")
	}
	if len(vars) > 1 {
		stmt.ObjectVariable = strings.TrimPrefix(vars[1].GetText(), "$")
	}
	return stmt
}

// buildSetStatement converts SET statement context to MfSetStmt or specialized statement types.
// When the expression is a list operation (HEAD, TAIL, etc.) or aggregate (COUNT, SUM, etc.),
// this returns the specialized statement type instead of MfSetStmt.
func buildSetStatement(ctx parser.ISetStatementContext) ast.MicroflowStatement {
	if ctx == nil {
		return nil
	}
	setCtx := ctx.(*parser.SetStatementContext)

	// Get target variable name
	var targetVar string
	if v := setCtx.VARIABLE(); v != nil {
		targetVar = strings.TrimPrefix(v.GetText(), "$")
	} else if ap := setCtx.AttributePath(); ap != nil {
		// Rebuild the path from its structured segments (quotes stripped) rather
		// than ap.GetText(): a quoted member/association name would otherwise be
		// carried verbatim (with quotes) into the Change activity's member
		// identifier, corrupting the .mpr. See buildAttributePathFromContext.
		targetVar = attributePathTargetText(ap)
	}

	// Get value expression. Keep the structured expression for list/aggregate
	// detection, then preserve source text for plain SET statements.
	var valueExpr ast.Expression
	var valueExprCtx parser.IExpressionContext
	if expr := setCtx.Expression(); expr != nil {
		valueExprCtx = expr
		valueExpr = buildExpression(expr)
	}

	// Check if the expression is a list operation or aggregate function
	if funcCall, ok := valueExpr.(*ast.FunctionCallExpr); ok {
		funcName := strings.ToUpper(funcCall.Name)

		// Check for list operations: HEAD, TAIL, FIND, FILTER, SORT, UNION, INTERSECT, SUBTRACT, CONTAINS, EQUALS
		switch funcName {
		case "HEAD":
			return &ast.ListOperationStmt{
				OutputVariable: targetVar,
				Operation:      ast.ListOpHead,
				InputVariable:  extractVariableName(funcCall.Arguments, 0),
			}
		case "TAIL":
			return &ast.ListOperationStmt{
				OutputVariable: targetVar,
				Operation:      ast.ListOpTail,
				InputVariable:  extractVariableName(funcCall.Arguments, 0),
			}
		case "FIND":
			return &ast.ListOperationStmt{
				OutputVariable: targetVar,
				Operation:      ast.ListOpFind,
				InputVariable:  extractVariableName(funcCall.Arguments, 0),
				Condition:      getArgumentExpression(funcCall.Arguments, 1),
			}
		case "FILTER":
			return &ast.ListOperationStmt{
				OutputVariable: targetVar,
				Operation:      ast.ListOpFilter,
				InputVariable:  extractVariableName(funcCall.Arguments, 0),
				Condition:      getArgumentExpression(funcCall.Arguments, 1),
			}
		case "SORT":
			stmt := &ast.ListOperationStmt{
				OutputVariable: targetVar,
				Operation:      ast.ListOpSort,
				InputVariable:  extractVariableName(funcCall.Arguments, 0),
			}
			// Parse sort specifications from remaining arguments
			stmt.SortSpecs = extractSortSpecs(funcCall.Arguments[1:])
			return stmt
		case "UNION":
			return &ast.ListOperationStmt{
				OutputVariable: targetVar,
				Operation:      ast.ListOpUnion,
				InputVariable:  extractVariableName(funcCall.Arguments, 0),
				SecondVariable: extractVariableName(funcCall.Arguments, 1),
			}
		case "INTERSECT":
			return &ast.ListOperationStmt{
				OutputVariable: targetVar,
				Operation:      ast.ListOpIntersect,
				InputVariable:  extractVariableName(funcCall.Arguments, 0),
				SecondVariable: extractVariableName(funcCall.Arguments, 1),
			}
		case "SUBTRACT":
			return &ast.ListOperationStmt{
				OutputVariable: targetVar,
				Operation:      ast.ListOpSubtract,
				InputVariable:  extractVariableName(funcCall.Arguments, 0),
				SecondVariable: extractVariableName(funcCall.Arguments, 1),
			}
		case "CONTAINS":
			return &ast.ListOperationStmt{
				OutputVariable: targetVar,
				Operation:      ast.ListOpContains,
				InputVariable:  extractVariableName(funcCall.Arguments, 0),
				SecondVariable: extractVariableName(funcCall.Arguments, 1),
			}
		case "EQUALS":
			return &ast.ListOperationStmt{
				OutputVariable: targetVar,
				Operation:      ast.ListOpEquals,
				InputVariable:  extractVariableName(funcCall.Arguments, 0),
				SecondVariable: extractVariableName(funcCall.Arguments, 1),
			}
		case "RANGE":
			stmt := &ast.ListOperationStmt{
				OutputVariable: targetVar,
				Operation:      ast.ListOpRange,
				InputVariable:  extractVariableName(funcCall.Arguments, 0),
			}
			if len(funcCall.Arguments) > 1 {
				stmt.OffsetExpr = funcCall.Arguments[1]
			}
			if len(funcCall.Arguments) > 2 {
				stmt.LimitExpr = funcCall.Arguments[2]
			}
			return stmt
		// Check for aggregate operations: COUNT, SUM, AVERAGE, MINIMUM, MAXIMUM
		case "COUNT":
			return &ast.AggregateListStmt{
				OutputVariable: targetVar,
				Operation:      ast.AggregateCount,
				InputVariable:  extractVariableName(funcCall.Arguments, 0),
			}
		case "SUM":
			inputVar, attr := extractVariableAndAttribute(funcCall.Arguments, 0)
			return &ast.AggregateListStmt{
				OutputVariable: targetVar,
				Operation:      ast.AggregateSum,
				InputVariable:  inputVar,
				Attribute:      attr,
			}
		case "AVERAGE":
			inputVar, attr := extractVariableAndAttribute(funcCall.Arguments, 0)
			return &ast.AggregateListStmt{
				OutputVariable: targetVar,
				Operation:      ast.AggregateAverage,
				InputVariable:  inputVar,
				Attribute:      attr,
			}
		case "MINIMUM":
			inputVar, attr := extractVariableAndAttribute(funcCall.Arguments, 0)
			return &ast.AggregateListStmt{
				OutputVariable: targetVar,
				Operation:      ast.AggregateMinimum,
				InputVariable:  inputVar,
				Attribute:      attr,
			}
		case "MAXIMUM":
			inputVar, attr := extractVariableAndAttribute(funcCall.Arguments, 0)
			return &ast.AggregateListStmt{
				OutputVariable: targetVar,
				Operation:      ast.AggregateMaximum,
				InputVariable:  inputVar,
				Attribute:      attr,
			}
		}
	}

	if valueExprCtx != nil {
		valueExpr = buildSourceExpression(valueExprCtx)
		valueExpr = appendStatementExpressionTrailingWhitespace(valueExprCtx, valueExpr)
	}

	// Default: regular SET statement
	return &ast.MfSetStmt{
		Target: targetVar,
		Value:  valueExpr,
	}
}

// extractVariableName extracts a variable name from an argument at the given index.
func extractVariableName(args []ast.Expression, index int) string {
	if index >= len(args) {
		return ""
	}
	if varExpr, ok := args[index].(*ast.VariableExpr); ok {
		return varExpr.Name
	}
	// If it's an identifier (unquoted), treat it as a variable name
	if identExpr, ok := args[index].(*ast.IdentifierExpr); ok {
		return identExpr.Name
	}
	return ""
}

// getArgumentExpression returns the expression at the given index, or nil if not present.
func getArgumentExpression(args []ast.Expression, index int) ast.Expression {
	if index >= len(args) {
		return nil
	}
	return args[index]
}

// extractVariableAndAttribute extracts variable and attribute from $Var/Attr or $Var, Attr.
func extractVariableAndAttribute(args []ast.Expression, index int) (varName string, attrName string) {
	if index >= len(args) {
		return "", ""
	}
	// Check for attribute path like $Var/Attr
	if pathExpr, ok := args[index].(*ast.AttributePathExpr); ok {
		varName = pathExpr.Variable
		if len(pathExpr.Path) > 0 {
			attrName = pathExpr.Path[len(pathExpr.Path)-1]
		}
		return
	}
	// Check for simple variable
	if varExpr, ok := args[index].(*ast.VariableExpr); ok {
		varName = varExpr.Name
		// Look for attribute in next argument
		if index+1 < len(args) {
			if identExpr, ok := args[index+1].(*ast.IdentifierExpr); ok {
				attrName = identExpr.Name
			}
		}
		return
	}
	return "", ""
}

// extractSortSpecs extracts sort specifications from function arguments.
// Expected format: Attr ASC, Attr2 DESC or just Attr (defaults to ASC)
func extractSortSpecs(args []ast.Expression) []ast.SortSpec {
	var specs []ast.SortSpec
	for _, arg := range args {
		// Try to parse as "Attr ASC" or "Attr DESC" or just "Attr"
		if identExpr, ok := arg.(*ast.IdentifierExpr); ok {
			// Parse "Name ASC" or "Name DESC" format from expression visitor
			name := identExpr.Name
			ascending := true
			if before, ok0 := strings.CutSuffix(name, " DESC"); ok0 {
				name = before
				ascending = false
			} else if before, ok0 := strings.CutSuffix(name, " ASC"); ok0 {
				name = before
			}
			specs = append(specs, ast.SortSpec{
				Attribute: name,
				Ascending: ascending,
			})
		}
		// For more complex expressions, extract what we can
		if binExpr, ok := arg.(*ast.BinaryExpr); ok {
			// Handle "Attr ASC" parsed as binary expression
			if leftIdent, ok := binExpr.Left.(*ast.IdentifierExpr); ok {
				ascending := true
				if strings.ToUpper(binExpr.Operator) == "DESC" {
					ascending = false
				}
				specs = append(specs, ast.SortSpec{
					Attribute: leftIdent.Name,
					Ascending: ascending,
				})
			}
		}
	}
	return specs
}

// buildCreateObjectStatement converts CREATE OBJECT statement context to CreateObjectStmt.
// Grammar: (VARIABLE EQUALS)? CREATE nonListDataType (LPAREN memberAssignmentList? RPAREN)?
// Example: $NewProduct = CREATE MfTest.Product (Name = $Name, Code = $Code);
func buildCreateObjectStatement(ctx parser.ICreateObjectStatementContext) *ast.CreateObjectStmt {
	if ctx == nil {
		return nil
	}
	createCtx := ctx.(*parser.CreateObjectStatementContext)

	stmt := &ast.CreateObjectStmt{}

	// Get variable name
	if v := createCtx.VARIABLE(); v != nil {
		stmt.Variable = strings.TrimPrefix(v.GetText(), "$")
	}

	// Get entity type from nonListDataType - use microflow builder to get entity reference
	if dt := createCtx.NonListDataType(); dt != nil {
		dataType := buildNonListDataType(dt)
		if dataType.EntityRef != nil {
			stmt.EntityType = *dataType.EntityRef
		}
	}

	// Get SET member assignments
	if memberList := createCtx.MemberAssignmentList(); memberList != nil {
		stmt.Changes = buildMemberAssignmentList(memberList)
	}

	// Check for ON ERROR clause
	if errClause := createCtx.OnErrorClause(); errClause != nil {
		stmt.ErrorHandling = buildOnErrorClause(errClause)
	}

	return stmt
}

// buildChangeObjectStatement converts CHANGE statement context to ChangeObjectStmt.
// Grammar: CHANGE VARIABLE (LPAREN memberAssignmentList? RPAREN)?
// Example: CHANGE $Product (Name = $NewName, ModifiedDate = [%CurrentDateTime%]);
func buildChangeObjectStatement(ctx parser.IChangeObjectStatementContext) *ast.ChangeObjectStmt {
	if ctx == nil {
		return nil
	}
	changeCtx := ctx.(*parser.ChangeObjectStatementContext)

	stmt := &ast.ChangeObjectStmt{}

	// Get variable name
	if v := changeCtx.VARIABLE(); v != nil {
		stmt.Variable = strings.TrimPrefix(v.GetText(), "$")
	}

	// Get SET member assignments
	if memberList := changeCtx.MemberAssignmentList(); memberList != nil {
		stmt.Changes = buildMemberAssignmentList(memberList)
	}
	stmt.RefreshInClient = changeCtx.REFRESH() != nil

	return stmt
}

// buildCommitStatement converts COMMIT statement context to MfCommitStmt.
// Grammar: COMMIT VARIABLE (WITH EVENTS)? REFRESH?
func buildCommitStatement(ctx parser.ICommitStatementContext) *ast.MfCommitStmt {
	if ctx == nil {
		return nil
	}
	commitCtx := ctx.(*parser.CommitStatementContext)

	stmt := &ast.MfCommitStmt{}

	// Get variable name
	if v := commitCtx.VARIABLE(); v != nil {
		stmt.Variable = strings.TrimPrefix(v.GetText(), "$")
	}

	// Check for WITH EVENTS
	if commitCtx.EVENTS() != nil {
		stmt.WithEvents = true
	}

	// Check for REFRESH
	if commitCtx.REFRESH() != nil {
		stmt.RefreshInClient = true
	}

	// Check for ON ERROR clause
	if errClause := commitCtx.OnErrorClause(); errClause != nil {
		stmt.ErrorHandling = buildOnErrorClause(errClause)
	}

	return stmt
}

// buildDeleteObjectStatement converts DELETE statement context to DeleteObjectStmt.
func buildDeleteObjectStatement(ctx parser.IDeleteObjectStatementContext) *ast.DeleteObjectStmt {
	if ctx == nil {
		return nil
	}
	delCtx := ctx.(*parser.DeleteObjectStatementContext)

	stmt := &ast.DeleteObjectStmt{}

	// Get variable name
	if v := delCtx.VARIABLE(); v != nil {
		stmt.Variable = strings.TrimPrefix(v.GetText(), "$")
	}

	// Check for ON ERROR clause
	if errClause := delCtx.OnErrorClause(); errClause != nil {
		stmt.ErrorHandling = buildOnErrorClause(errClause)
	}

	return stmt
}

// buildRollbackStatement converts ROLLBACK statement context to RollbackStmt.
func buildRollbackStatement(ctx parser.IRollbackStatementContext) *ast.RollbackStmt {
	if ctx == nil {
		return nil
	}
	rollCtx := ctx.(*parser.RollbackStatementContext)

	stmt := &ast.RollbackStmt{}

	// Get variable name
	if v := rollCtx.VARIABLE(); v != nil {
		stmt.Variable = strings.TrimPrefix(v.GetText(), "$")
	}

	// Check for REFRESH keyword
	stmt.RefreshInClient = rollCtx.REFRESH() != nil

	return stmt
}

// buildRetrieveStatement converts RETRIEVE statement context to RetrieveStmt.
// Grammar: RETRIEVE VARIABLE FROM retrieveSource (WHERE expression)? (SORT_BY sortColumn+)? (OFFSET NUMBER_LITERAL)? (LIMIT NUMBER_LITERAL)?
func buildRetrieveStatement(ctx parser.IRetrieveStatementContext) *ast.RetrieveStmt {
	if ctx == nil {
		return nil
	}
	retrCtx := ctx.(*parser.RetrieveStatementContext)

	stmt := &ast.RetrieveStmt{}

	// Get variable name
	if v := retrCtx.VARIABLE(); v != nil {
		stmt.Variable = strings.TrimPrefix(v.GetText(), "$")
	}

	// Get source (database entity or association path)
	if src := retrCtx.RetrieveSource(); src != nil {
		srcCtx := src.(*parser.RetrieveSourceContext)
		if v := srcCtx.VARIABLE(); v != nil {
			// Association retrieve: $Parent/Module.AssociationName
			stmt.StartVariable = strings.TrimPrefix(v.GetText(), "$")
			if qn := srcCtx.QualifiedName(); qn != nil {
				stmt.Source = buildQualifiedName(qn)
			}
		} else if qn := srcCtx.QualifiedName(); qn != nil {
			// Database retrieve: Module.Entity
			stmt.Source = buildQualifiedName(qn)
		}
	}

	// Get WHERE condition (now at RETRIEVE level)
	// Supports both bare expression: WHERE expr
	// and bracket notation: WHERE [expr]
	if retrCtx.WHERE() != nil {
		xpathConstraints := retrCtx.AllXpathConstraint()
		if len(xpathConstraints) == 1 {
			xcCtx := xpathConstraints[0].(*parser.XpathConstraintContext)
			if xpathExpr := xcCtx.XpathExpr(); xpathExpr != nil {
				stmt.Where = buildXPathSourceExpression(xpathExpr)
			}
		} else if len(xpathConstraints) > 1 {
			// Multiple predicates [cond1][cond2] are semantically ANDed by
			// XPath, but their predicate boundaries matter when one predicate
			// contains OR. Preserve the bracketed source so the builder can
			// write the same XPathConstraint shape back to the MPR.
			var andExprs []ast.Expression
			var predicateSources []string
			for _, xc := range xpathConstraints {
				xcCtx := xc.(*parser.XpathConstraintContext)
				if xpathExpr := xcCtx.XpathExpr(); xpathExpr != nil {
					andExprs = append(andExprs, buildXPathSourceExpression(xpathExpr))
					if prc, ok := xpathExpr.(antlr.ParserRuleContext); ok {
						if source := strings.TrimSpace(extractOriginalText(prc)); source != "" {
							predicateSources = append(predicateSources, normalizeXPathTokens("["+source+"]"))
						}
					}
				}
			}
			if len(andExprs) == 1 {
				stmt.Where = andExprs[0]
			} else if len(andExprs) > 1 {
				// Build a chain of AND expressions
				result := andExprs[0]
				for _, expr := range andExprs[1:] {
					result = &ast.BinaryExpr{Left: result, Operator: "and", Right: expr}
				}
				if len(predicateSources) == len(andExprs) {
					result = &ast.SourceExpr{
						Expression: result,
						Source:     strings.Join(predicateSources, ""),
					}
				}
				stmt.Where = result
			}
		} else if expr := retrCtx.Expression(0); expr != nil {
			stmt.Where = buildRetrieveWhereExpression(expr)
		}
	}

	// Get SORT BY clause with multiple columns
	if retrCtx.SORT_BY() != nil {
		for _, sortColCtx := range retrCtx.AllSortColumn() {
			col := buildSortColumnMicroflow(sortColCtx)
			if col != nil {
				stmt.SortColumns = append(stmt.SortColumns, *col)
			}
		}
	}

	// Get LIMIT and OFFSET expressions
	if limitExpr := retrCtx.GetLimitExpr(); limitExpr != nil {
		stmt.Limit = retrieveRangeExpressionSource(limitExpr) + retrieveLimitTrailingWhitespace(retrCtx, limitExpr)
	}
	if offsetExpr := retrCtx.GetOffsetExpr(); offsetExpr != nil {
		stmt.Offset = retrieveRangeExpressionSource(offsetExpr) + retrieveRangeExpressionTrailingWhitespace(offsetExpr)
	}

	// Check for ON ERROR clause
	if errClause := retrCtx.OnErrorClause(); errClause != nil {
		stmt.ErrorHandling = buildOnErrorClause(errClause)
	}

	return stmt
}

func retrieveRangeExpressionSource(exprCtx parser.IExpressionContext) string {
	if exprCtx == nil {
		return ""
	}
	if prc, ok := exprCtx.(antlr.ParserRuleContext); ok {
		if source := strings.TrimSpace(extractOriginalText(prc)); source != "" {
			return source
		}
	}
	return exprCtx.GetText()
}

func retrieveLimitTrailingWhitespace(retrCtx *parser.RetrieveStatementContext, limitExpr parser.IExpressionContext) string {
	if retrCtx == nil || limitExpr == nil {
		return ""
	}
	exprRule, ok := limitExpr.(antlr.ParserRuleContext)
	if !ok || exprRule.GetStop() == nil {
		return ""
	}
	input := exprRule.GetStop().GetInputStream()
	if input == nil {
		return ""
	}

	start := exprRule.GetStop().GetStop() + 1
	if offset := retrCtx.OFFSET(); offset != nil && offset.GetSymbol() != nil {
		gap := whitespaceBetween(input, start, offset.GetSymbol().GetStart()-1)
		return retrieveInterClauseWhitespaceSuffix(gap)
	}
	return whitespaceUntilDelimiter(input, start, ";")
}

func retrieveRangeExpressionTrailingWhitespace(exprCtx parser.IExpressionContext) string {
	exprRule, ok := exprCtx.(antlr.ParserRuleContext)
	if !ok || exprRule.GetStop() == nil {
		return ""
	}
	input := exprRule.GetStop().GetInputStream()
	if input == nil {
		return ""
	}
	return whitespaceUntilDelimiter(input, exprRule.GetStop().GetStop()+1, ";")
}

func whitespaceBetween(input antlr.CharStream, start, end int) string {
	if start < 0 || end < start || start >= input.Size() {
		return ""
	}
	gap := input.GetText(start, end)
	if strings.TrimSpace(gap) != "" {
		return ""
	}
	return gap
}

// retrieveInterClauseWhitespaceSuffix returns the whitespace gap between a
// retrieve expression and the next clause keyword (LIMIT/OFFSET), with the
// trailing newline + indent that the formatter will re-emit stripped off.
//
// The formatter writes each subsequent clause on its own line indented by
// `formatRetrieveContinuationIndent` spaces, so the original source's trailing
// "\n<indent>" is structural and would duplicate after a roundtrip if kept.
// Anything before that final newline (blank lines, comments, additional
// indentation) is preserved as authored. When the gap does not end in a
// recognisable line-break-then-indent sequence we return "" — the formatter
// will lay out the clause normally.
func retrieveInterClauseWhitespaceSuffix(gap string) string {
	if gap == "" {
		return ""
	}
	// Trim the trailing newline + structural indentation the formatter will
	// re-emit. We strip whatever indent (spaces or tabs) follows the final
	// newline so this stays robust if the formatter changes its indent width.
	for i := len(gap) - 1; i >= 0; i-- {
		c := gap[i]
		if c == ' ' || c == '\t' {
			continue
		}
		if c == '\n' {
			// Include a preceding \r in the strip so CRLF line endings work.
			cut := i
			if cut > 0 && gap[cut-1] == '\r' {
				cut--
			}
			return gap[:cut]
		}
		break
	}
	return ""
}

// buildSortColumnMicroflow builds a sort column definition from a SortColumnContext.
// This is a duplicate of buildSortColumn in visitor_page_widgets.go but in a different file.
func buildSortColumnMicroflow(ctx parser.ISortColumnContext) *ast.SortColumnDef {
	if ctx == nil {
		return nil
	}
	colCtx := ctx.(*parser.SortColumnContext)

	col := &ast.SortColumnDef{
		Order: "ASC", // Default to ASC
	}

	// Get attribute name from QualifiedName or IDENTIFIER
	if qn := colCtx.QualifiedName(); qn != nil {
		col.Attribute = qn.GetText()
	} else if id := colCtx.IDENTIFIER(); id != nil {
		col.Attribute = id.GetText()
	}

	// Get sort order
	if colCtx.DESC() != nil {
		col.Order = "DESC"
	}

	return col
}

// buildIfStatement converts IF statement context to IfStmt.
func buildIfStatement(ctx parser.IIfStatementContext) *ast.IfStmt {
	if ctx == nil {
		return nil
	}
	ifCtx := ctx.(*parser.IfStatementContext)

	// Grammar: IF expression THEN microflowBody
	//          (ELSIF expression THEN microflowBody)*
	//          (ELSE microflowBody)? END IF
	// exprs[i] pairs with bodies[i]; one trailing extra body is the ELSE branch.
	exprs := ifCtx.AllExpression()
	bodies := ifCtx.AllMicroflowBody()

	if len(exprs) == 0 {
		// Defensive only — the grammar guarantees at least one condition.
		stmt := &ast.IfStmt{}
		if len(bodies) > 0 {
			stmt.ThenBody = buildMicroflowBody(bodies[0])
		}
		return stmt
	}

	hasElse := len(bodies) > len(exprs) || ifCtx.ELSE() != nil
	var elseBody []ast.MicroflowStatement
	if len(bodies) > len(exprs) {
		elseBody = buildMicroflowBody(bodies[len(bodies)-1])
	}

	// Mendix has no native elsif construct, so lower each ELSIF arm into a
	// nested IfStmt in the ELSE branch of the arm before it (built innermost
	// first). Previously only exprs[0]/bodies[0] and the trailing ELSE were
	// read, silently dropping every ELSIF arm from the written model.
	var stmt *ast.IfStmt
	for i := len(exprs) - 1; i >= 0; i-- {
		s := &ast.IfStmt{}
		s.Condition = buildSourceExpression(exprs[i])
		if i < len(bodies) {
			s.ThenBody = buildMicroflowBody(bodies[i])
		}
		if stmt != nil {
			s.HasElse = true
			s.ElseBody = []ast.MicroflowStatement{stmt}
		} else {
			s.HasElse = hasElse
			s.ElseBody = elseBody
		}
		stmt = s
	}

	return stmt
}

// buildLoopStatement converts LOOP statement context to LoopStmt.
func buildLoopStatement(ctx parser.ILoopStatementContext) *ast.LoopStmt {
	if ctx == nil {
		return nil
	}
	loopCtx := ctx.(*parser.LoopStatementContext)

	stmt := &ast.LoopStmt{}

	// Get variables (first is loop variable, second is list)
	vars := loopCtx.AllVARIABLE()
	if len(vars) >= 1 {
		stmt.LoopVariable = strings.TrimPrefix(vars[0].GetText(), "$")
	}
	if len(vars) >= 2 {
		stmt.ListVariable = strings.TrimPrefix(vars[1].GetText(), "$")
	}

	// Get body
	if body := loopCtx.MicroflowBody(); body != nil {
		stmt.Body = buildMicroflowBody(body)
	}

	return stmt
}

// buildWhileStatement converts WHILE statement context to WhileStmt.
func buildWhileStatement(ctx parser.IWhileStatementContext) *ast.WhileStmt {
	if ctx == nil {
		return nil
	}
	wsCtx := ctx.(*parser.WhileStatementContext)

	stmt := &ast.WhileStmt{}

	// Get condition expression
	if expr := wsCtx.Expression(); expr != nil {
		stmt.Condition = buildSourceExpression(expr)
	}

	// Get body
	if body := wsCtx.MicroflowBody(); body != nil {
		stmt.Body = buildMicroflowBody(body)
	}

	return stmt
}

func buildSourceExpression(ctx parser.IExpressionContext) ast.Expression {
	if ctx == nil {
		return nil
	}
	expr := buildExpression(ctx)
	if prc, ok := ctx.(antlr.ParserRuleContext); ok {
		if source := strings.TrimSpace(extractOriginalText(prc)); source != "" {
			if shouldPreserveExpressionSource(source) {
				return &ast.SourceExpr{Expression: expr, Source: stripExpressionIdentifierQuotes(source)}
			}
		}
	}
	return expr
}

func buildXPathSourceExpression(ctx parser.IXpathExprContext) ast.Expression {
	if ctx == nil {
		return nil
	}
	expr := buildXPathExpr(ctx)
	if prc, ok := ctx.(antlr.ParserRuleContext); ok {
		if source := strings.TrimSpace(extractOriginalText(prc)); source != "" {
			// Requote any bare [%token%] so the stored constraint passes mx check
			// (CE0161) — the original source preserves the unquoted form (#641).
			return &ast.SourceExpr{Expression: expr, Source: stripExpressionIdentifierQuotes(normalizeXPathTokens(source))}
		}
	}
	return expr
}

func buildRetrieveWhereExpression(ctx parser.IExpressionContext) ast.Expression {
	if ctx == nil {
		return nil
	}
	expr := buildExpression(ctx)
	// `where '<xpath>'` — the whole clause is a single quoted string carrying the
	// XPath constraint as a literal. Use the UNQUOTED value as the source, not the
	// raw token: preserving the raw text keeps the outer quotes and the doubled
	// '' escapes, which get bracket-wrapped into `['[Title=''abc'']']` and fail
	// CE0161. The inline `where [...]` form is unaffected (it takes the
	// xpathConstraint path, not this one). Issue #642.
	if lit, ok := expr.(*ast.LiteralExpr); ok && lit.Kind == ast.LiteralString {
		if s, ok := lit.Value.(string); ok {
			return &ast.SourceExpr{Expression: expr, Source: stripExpressionIdentifierQuotes(s)}
		}
	}
	if prc, ok := ctx.(antlr.ParserRuleContext); ok {
		if source := strings.TrimSpace(extractOriginalText(prc)); source != "" {
			if shouldPreserveExpressionSource(source) || strings.Contains(source, "/") {
				return &ast.SourceExpr{Expression: expr, Source: stripExpressionIdentifierQuotes(source)}
			}
		}
	}
	return expr
}

func shouldPreserveExpressionSource(source string) bool {
	if strings.ContainsAny(source, "\r\n") {
		return true
	}
	inString := false
	for i := 0; i < len(source); i++ {
		if source[i] == '\'' {
			if inString && i+1 < len(source) && source[i+1] == '\'' {
				i++
				continue
			}
			inString = !inString
			continue
		}
		if inString {
			continue
		}
		switch source[i] {
		case '=', '!', '<', '>', '+', '-', '*', ':', ',':
			if i > 0 && source[i-1] != ' ' && source[i-1] != '\t' {
				return true
			}
			if i+1 < len(source) && source[i+1] != ' ' && source[i+1] != '\t' && source[i+1] != '=' {
				return true
			}
		}
	}
	// Mendix's `not(<expr>)` function call has no surrounding spaces in
	// idiomatic source, but the parser would re-emit it as `not (<expr>)`
	// (function-call AST node loses the no-space affordance). Preserving the
	// original source keeps the compact form across describe → exec →
	// describe. The substring check is intentionally loose; false positives
	// (e.g. an attribute name containing "not(") only over-preserve and have
	// no semantic effect since the parsed expression is what runs.
	if strings.Contains(strings.ToLower(source), "not(") {
		return true
	}
	return false
}

// buildReturnStatement converts RETURN statement context to ReturnStmt.
func buildReturnStatement(ctx parser.IReturnStatementContext) *ast.ReturnStmt {
	if ctx == nil {
		return nil
	}
	retCtx := ctx.(*parser.ReturnStatementContext)

	stmt := &ast.ReturnStmt{}

	// Get optional return value
	if expr := retCtx.Expression(); expr != nil {
		stmt.Value = buildSourceExpression(expr)
	}

	return stmt
}
