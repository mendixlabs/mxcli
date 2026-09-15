// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"strconv"
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
	} else if merge := mfCtx.MergeStatement(); merge != nil {
		stmt = &ast.MergeStmt{Label: mergeJoinLabel(merge.IDENTIFIER(), merge.QUOTED_IDENTIFIER())}
	} else if join := mfCtx.JoinStatement(); join != nil {
		stmt = &ast.JoinStmt{Label: mergeJoinLabel(join.IDENTIFIER(), join.QUOTED_IDENTIFIER())}
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
	} else if closePage := mfCtx.ClosePageStatement(); closePage != nil {
		close := &ast.ClosePageStmt{NumberOfPages: 1}
		if errClause := closePage.OnErrorClause(); errClause != nil {
			close.ErrorHandling = buildOnErrorClause(errClause)
		}
		stmt = close
	} else if mfCtx.ShowHomePageStatement() != nil {
		stmt = &ast.ShowHomePageStmt{}
	} else if showMsg := mfCtx.ShowMessageStatement(); showMsg != nil {
		stmt = buildShowMessageStatement(showMsg)
	} else if download := mfCtx.DownloadFileStatement(); download != nil {
		stmt = buildDownloadFileStatement(download)
	} else if sync := mfCtx.SynchronizeStatement(); sync != nil {
		stmt = buildSynchronizeStatement(sync)
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
			// Two forms. `@annotation 'text'` is the everyday one and is
			// unchanged. `@annotation(id: n1, text: '…', position: (x, y),
			// size: (w, h))` carries the note's identity and geometry, which the
			// bare form cannot express (#1077).
			//
			// The annotation rule already accepted parenthesised params, so that
			// form PARSED before this and was silently discarded. Two of the
			// four keys still needed a grammar change: `text` and `position` are
			// lexer keywords, and a keyword key does not fail the parse — it
			// falls through to annotationParam's positional alternative — so
			// they were being accepted and quietly ignored.
			note, ok := parseNoteAnnotation(ann, result)
			if ok {
				// Free-floating only when nothing has claimed this statement
				// yet AND an activity annotation follows: the note belongs to
				// the canvas, not to the statement below it.
				if !seenActivityMetadata && hasLaterActivityAnnotation(annotations, i+1) {
					result.FreeNotes = append(result.FreeNotes, note)
				} else {
					result.Notes = append(result.Notes, note)
				}
				hasAny = true
			} else if len(result.InvalidNotes) > 0 {
				hasAny = true
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

		case "merge":
			// @merge(x, y) — the implicit merge node that closes a split. Same
			// positional shape as @position, which belongs to the split. (#884)
			if params := ann.AnnotationParams(); params != nil {
				allParams := params.(*parser.AnnotationParamsContext).AllAnnotationParam()
				if len(allParams) >= 2 {
					result.Merge = &ast.Position{
						X: parseAnnotationParamInt(allParams[0]),
						Y: parseAnnotationParamInt(allParams[1]),
					}
					hasAny = true
				}
			}
			seenActivityMetadata = true

		case "start":
			// @start(x, y) — the StartEvent, the implicit node the flow begins
			// at. Written on the FIRST statement, the one the start flows into;
			// same positional shape and same reason as @merge, which positions
			// the other node that has no statement of its own. (#951)
			if params := ann.AnnotationParams(); params != nil {
				allParams := params.(*parser.AnnotationParamsContext).AllAnnotationParam()
				if len(allParams) >= 2 {
					result.Start = &ast.Position{
						X: parseAnnotationParamInt(allParams[0]),
						Y: parseAnnotationParamInt(allParams[1]),
					}
					hasAny = true
				}
			}
			seenActivityMetadata = true

		case "curve":
			// @curve(from: (40, -90), to: (-40, 90)) — the bezier control
			// vectors of the flow LEAVING this statement. Needed no grammar
			// change: `name: (x, y)` is already annotationParenValue, the same
			// shape the association anchors use. (#884)
			if params := ann.AnnotationParams(); params != nil {
				parseCurveAnnotation(params.(*parser.AnnotationParamsContext), result)
				hasAny = true
			}
			seenActivityMetadata = true

		default:
			// Record rather than drop. The grammar accepts any @name, so this
			// arm catches both an annotation mxcli does not implement (@size)
			// and — the reason it matters — a typo of one it does. (#884)
			result.UnknownNames = append(result.UnknownNames, ann.AnnotationName().GetText())
			hasAny = true
		}
	}

	if !hasAny {
		return nil
	}
	return result
}

// parseNoteAnnotation reads one `@annotation` into a MicroflowAnnotation.
//
// Anything it cannot use is recorded on result.InvalidNotes rather than
// dropped, and makes the note itself invalid: a typo'd parameter would
// otherwise cost the reader the whole note, or — worse for `id:` — turn a
// reference to an existing note into a second, textless one. Validation refuses
// them (MDL079); the visitor's job is only to not lose them.
func parseNoteAnnotation(ann *parser.AnnotationContext, result *ast.ActivityAnnotations) (ast.MicroflowAnnotation, bool) {
	var note ast.MicroflowAnnotation

	// @annotation 'text' — the bare form.
	if valCtx := ann.AnnotationValue(); valCtx != nil {
		note.Text = extractAnnotationValueString(valCtx)
		return note, note.Text != ""
	}

	params := ann.AnnotationParams()
	if params == nil {
		return note, false
	}

	for _, p := range params.(*parser.AnnotationParamsContext).AllAnnotationParam() {
		pCtx := p.(*parser.AnnotationParamContext)
		nameCtx := pCtx.AnnotationParamName()
		if nameCtx == nil {
			// Positional. Deliberately unsupported: `@annotation('a', 'b')`
			// has no reading that is obviously right, and guessing one would
			// silently mean something.
			result.InvalidNotes = append(result.InvalidNotes, strings.TrimSpace(pCtx.GetText()))
			continue
		}
		switch strings.ToLower(nameCtx.GetText()) {
		case "id":
			if v := pCtx.AnnotationValue(); v != nil {
				note.Label = extractAnnotationValueIdentifier(v)
			}
			if note.Label == "" {
				result.InvalidNotes = append(result.InvalidNotes, strings.TrimSpace(pCtx.GetText()))
			}
		case "text":
			if v := pCtx.AnnotationValue(); v != nil {
				note.Text = extractAnnotationValueString(v)
			}
			if note.Text == "" {
				result.InvalidNotes = append(result.InvalidNotes, strings.TrimSpace(pCtx.GetText()))
			}
		case "position":
			pt, ok := annotationPointValue(pCtx)
			if !ok {
				result.InvalidNotes = append(result.InvalidNotes, strings.TrimSpace(pCtx.GetText()))
				continue
			}
			note.Position = pt
		case "size":
			pt, ok := annotationPointValue(pCtx)
			if !ok {
				result.InvalidNotes = append(result.InvalidNotes, strings.TrimSpace(pCtx.GetText()))
				continue
			}
			note.Size = &ast.BoxSize{Width: pt.X, Height: pt.Y}
		default:
			result.InvalidNotes = append(result.InvalidNotes, strings.TrimSpace(pCtx.GetText()))
		}
	}

	// A note needs either text (a declaration) or a label (a reference to one).
	if note.Text == "" && note.Label == "" {
		result.InvalidNotes = append(result.InvalidNotes, strings.TrimSpace(ann.GetText()))
		return note, false
	}
	return note, true
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

// setStatementAnnotations sets the Annotations field on a microflow statement.
//
// Reflective (ast.SetStatementAnnotations), not a type switch: the switch this
// replaced had no case for any workflow or mapping statement, so their
// @position was parsed and silently dropped.
func setStatementAnnotations(stmt ast.MicroflowStatement, ann *ast.ActivityAnnotations) {
	ast.SetStatementAnnotations(stmt, ann)
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

	// Check for ON ERROR clause
	if errClause := declCtx.OnErrorClause(); errClause != nil {
		stmt.ErrorHandling = buildOnErrorClause(errClause)
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
		// `case X <body>` and `when X then <body>` are the same branch; only
		// the spelling differs, and it is recorded for MDL065.
		if c.CASE() != nil {
			stmt.LegacyCaseKeyword = true
		}
		stmt.Cases = append(stmt.Cases, ast.InheritanceSplitCase{
			Entity: buildQualifiedName(c.QualifiedName()),
			Body:   buildMicroflowBody(c.MicroflowBody()),
		})
	}
	if elseCtx := splitCtx.InheritanceSplitElse(); elseCtx != nil {
		e := elseCtx.(*parser.InheritanceSplitElseContext)
		if e.ELSE() != nil {
			stmt.LegacyElseKeyword = true
		}
		stmt.ElseBody = buildMicroflowBody(e.MicroflowBody())
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
//
// The ON ERROR clause is attached here rather than at each of the dozen return
// points below. Only the plain MfSetStmt form can honour it — Mendix gives
// ChangeVariableAction an ErrorHandlingType but gives ListOperationsAction and
// AggregateAction none — so the specialized nodes carry it only to be refused by
// MDL077, never to be executed.
func buildSetStatement(ctx parser.ISetStatementContext) ast.MicroflowStatement {
	stmt := buildSetStatementNode(ctx)
	if stmt == nil || ctx == nil {
		return stmt
	}
	errClause := ctx.(*parser.SetStatementContext).OnErrorClause()
	if errClause == nil {
		return stmt
	}
	eh := buildOnErrorClause(errClause)
	switch s := stmt.(type) {
	case *ast.MfSetStmt:
		s.ErrorHandling = eh
	case *ast.ListOperationStmt:
		s.ErrorHandling = eh
	case *ast.AggregateListStmt:
		s.ErrorHandling = eh
	}
	return stmt
}

func buildSetStatementNode(ctx parser.ISetStatementContext) ast.MicroflowStatement {
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
			// `find` is overloaded: the LIST operation find(list, condition) — which
			// filters a list by a boolean condition — and the STRING function
			// find(haystack, needle) → the index of a substring. A STRING-LITERAL
			// second argument is unambiguously the string function (you never filter
			// a list by a bare string literal); it must stay a value expression, not
			// a lossy List operation activity whose output variable collides
			// (CE0111). Ledger #63. When both arguments are plain variables the kind
			// is ambiguous here; the flow builder disambiguates String-typed inputs.
			if !isStringLiteralArg(funcCall.Arguments, 1) {
				return &ast.ListOperationStmt{
					OutputVariable: targetVar,
					Operation:      ast.ListOpFind,
					InputVariable:  extractVariableName(funcCall.Arguments, 0),
					Condition:      getArgumentExpression(funcCall.Arguments, 1),
				}
			}
			// Falls through to the default MfSetStmt (string find expression).
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
			// `contains` is overloaded: the LIST operation contains(list, object)
			// and the STRING function contains(haystack, needle). A List operation
			// activity requires two plain list/object variables; if either argument
			// is a literal or a computed expression it is unambiguously the string
			// function, which must stay a value expression (a Change Variable
			// action) — serializing it as a List operation fails the build
			// (CE0023/CE0097/CE0111). Ledger finding #53. When both arguments are
			// plain variables the kind is still ambiguous here (no type info); the
			// flow builder disambiguates String-typed inputs downstream.
			if isPlainVariableArg(funcCall.Arguments, 0) && isPlainVariableArg(funcCall.Arguments, 1) {
				return &ast.ListOperationStmt{
					OutputVariable: targetVar,
					Operation:      ast.ListOpContains,
					InputVariable:  extractVariableName(funcCall.Arguments, 0),
					SecondVariable: extractVariableName(funcCall.Arguments, 1),
				}
			}
			// Falls through to the default MfSetStmt (string contains expression).
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
			return buildSetAggregate(targetVar, ast.AggregateSum, funcCall.Arguments)
		case "AVERAGE":
			return buildSetAggregate(targetVar, ast.AggregateAverage, funcCall.Arguments)
		case "MINIMUM":
			return buildSetAggregate(targetVar, ast.AggregateMinimum, funcCall.Arguments)
		case "MAXIMUM":
			return buildSetAggregate(targetVar, ast.AggregateMaximum, funcCall.Arguments)
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

// isPlainVariableArg reports whether the argument at the given index is a bare
// variable reference (`$x` or an unquoted identifier) rather than a literal or a
// computed expression. Used to distinguish the list-operation form of an
// overloaded function (e.g. contains(list, object)) from its string form.
func isPlainVariableArg(args []ast.Expression, index int) bool {
	if index >= len(args) {
		return false
	}
	switch args[index].(type) {
	case *ast.VariableExpr, *ast.IdentifierExpr:
		return true
	}
	return false
}

// isStringLiteralArg reports whether the argument at the given index is a string
// literal — used to detect the string form of an overloaded function (e.g.
// find(haystack, 'needle')) that must not become a list operation.
func isStringLiteralArg(args []ast.Expression, index int) bool {
	if index >= len(args) {
		return false
	}
	lit, ok := args[index].(*ast.LiteralExpr)
	return ok && lit.Kind == ast.LiteralString
}

// getArgumentExpression returns the expression at the given index, or nil if not present.
func getArgumentExpression(args []ast.Expression, index int) ast.Expression {
	if index >= len(args) {
		return nil
	}
	return args[index]
}

// buildSetAggregate builds an aggregate activity from a SET whose value is a
// SUM/AVERAGE/MINIMUM/MAXIMUM call.
//
// It mirrors buildAggregateListStatement, which handles the same two spellings
// when they arrive through the aggregateListStatement rule: one argument is a
// list plus an attribute (`sum($List.Price)`), two arguments are a list plus an
// expression evaluated per item (`sum($List, $currentObject/Price * 0.21)`).
// Two conversions for one syntax is how the attribute went missing in the first
// place, so the two must agree.
func buildSetAggregate(targetVar string, op ast.AggregateListOperationType, args []ast.Expression) *ast.AggregateListStmt {
	stmt := &ast.AggregateListStmt{OutputVariable: targetVar, Operation: op}
	if len(args) == 0 {
		return stmt
	}

	switch arg := args[0].(type) {
	case *ast.AttributePathExpr:
		stmt.InputVariable = arg.Variable
		if len(arg.Path) > 0 {
			stmt.Attribute = arg.Path[len(arg.Path)-1]
		}
	case *ast.VariableExpr:
		// `sum($List.Price)` reaches the expression parser as one variable whose
		// name carries the dot, not as an attribute path. Left joined, mxbuild
		// reports the whole thing as an undefined variable (CE0109).
		stmt.InputVariable = arg.Name
		if list, attr, ok := strings.Cut(arg.Name, "."); ok {
			stmt.InputVariable, stmt.Attribute = list, attr
		}
	}

	// Any second argument is the per-item expression. Dropping it leaves an
	// aggregate with nothing to aggregate, which mxbuild rejects with CE0015.
	if len(args) > 1 {
		stmt.IsExpression = true
		stmt.Expression = args[1]
		stmt.Attribute = ""
	}
	return stmt
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

	stmt.Commit = buildCommitClause(createCtx.CommitClause())
	stmt.RefreshInClient = createCtx.REFRESH() != nil

	// Check for ON ERROR clause
	if errClause := createCtx.OnErrorClause(); errClause != nil {
		stmt.ErrorHandling = buildOnErrorClause(errClause)
	}

	return stmt
}

// buildCommitClause maps the optional COMMIT modifier on a create/change activity.
// Grammar: COMMIT (WITHOUT EVENTS)?  — absent means Mendix's default, No.
func buildCommitClause(ctx parser.ICommitClauseContext) ast.CommitFlag {
	if ctx == nil {
		return ast.CommitNo
	}
	if cc, ok := ctx.(*parser.CommitClauseContext); ok && cc.EVENTS() != nil {
		return ast.CommitYesWithoutEvents
	}
	return ast.CommitYes
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
	stmt.Commit = buildCommitClause(changeCtx.CommitClause())
	stmt.RefreshInClient = changeCtx.REFRESH() != nil

	// Check for ON ERROR clause
	if errClause := changeCtx.OnErrorClause(); errClause != nil {
		stmt.ErrorHandling = buildOnErrorClause(errClause)
	}

	return stmt
}

// buildCommitStatement converts COMMIT statement context to MfCommitStmt.
// Grammar: COMMIT VARIABLE ((WITH | WITHOUT) EVENTS)? REFRESH?
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

	// Check for WITHOUT EVENTS.
	//
	// Absent means events ON — Mendix's default for the Commit activity (#895).
	// A bare WITH EVENTS also leaves the flag clear: it says the same thing as
	// writing nothing, and every script written before the default was corrected
	// spells it out that way.
	if commitCtx.WITHOUT() != nil {
		stmt.WithoutEvents = true
	}
	stmt.ExplicitWithEvents = commitCtx.WITH() != nil

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
// Grammar: DELETE VARIABLE REFRESH?
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

	stmt.RefreshInClient = delCtx.REFRESH() != nil

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
						if source := strings.TrimSpace(extractExpressionText(prc)); source != "" {
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
		if source := strings.TrimSpace(extractExpressionText(prc)); source != "" {
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

	// Get attribute name from QualifiedName or IDENTIFIER. Strip quotes from each
	// segment: `sort by "Mod"."Entity"."Code"` must store the bare dotted form —
	// keeping the quotes produced a nonsense reference that only failed on write
	// ("attribute does not belong to entity"), unlike everywhere else where quoting
	// is safe (FINDINGS #13).
	if qn := colCtx.QualifiedName(); qn != nil {
		col.Attribute = unquoteQualifiedName(qn.GetText())
	} else if id := colCtx.IDENTIFIER(); id != nil {
		col.Attribute = unquoteIdentifier(id.GetText())
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
		if source := strings.TrimSpace(extractExpressionText(prc)); source != "" {
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
		if source := strings.TrimSpace(extractExpressionText(prc)); source != "" {
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
		if source := strings.TrimSpace(extractExpressionText(prc)); source != "" {
			if shouldPreserveExpressionSource(source) || strings.Contains(source, "/") {
				return &ast.SourceExpr{Expression: expr, Source: stripExpressionIdentifierQuotes(source)}
			}
		}
	}
	return expr
}

func isDigitByte(c byte) bool { return c >= '0' && c <= '9' }

func isIdentByte(c byte) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || isDigitByte(c)
}

// dotIsQualifiedNameSeparator reports whether the `.` at index i separates
// segments of a qualified name (`Module.Entity`, `L48.Transaction`) rather than
// being a decimal point. The token immediately before the `.` is a name segment
// (not a number) when it contains a letter or underscore — so a module/entity
// name that merely ends in a digit (`L48`, `Account2`) is not mistaken for a
// decimal. Symmetrically, a `.` that directly follows an identifier char and is
// followed by a name segment (a letter/underscore start) is a separator too.
func dotIsQualifiedNameSeparator(source string, i int) bool {
	// Walk back over the run of identifier chars ending at i-1; if any is a
	// letter or underscore, the preceding token is a name, not a number.
	s := i
	for s > 0 && isIdentByte(source[s-1]) {
		s--
		if source[s] == '_' || (source[s] >= 'a' && source[s] <= 'z') || (source[s] >= 'A' && source[s] <= 'Z') {
			return true
		}
	}
	return false
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
		// A `/` used as division with a variable right operand (`$a / $b`) parses
		// as a member-access path (the `$` on the RHS is stripped), so the AST
		// re-serializes to a bogus `$a/b`. Preserve the raw source ONLY for this
		// unambiguous misuse — a `/` immediately followed (ignoring spaces) by `$`.
		// A real association path never has `$` after `/`, so legitimate navigation
		// (`$Order/Assoc/Name`) is NOT source-frozen. MDL045 then rejects it at
		// check time using the preserved source. (#17, verification round)
		if source[i] == '/' {
			j := i + 1
			for j < len(source) && (source[j] == ' ' || source[j] == '\t') {
				j++
			}
			if j < len(source) && source[j] == '$' {
				return true
			}
		}
		// Otherwise `/` is the member-access separator (`$Order/Assoc/Name`), not
		// division (which is `div`), so it is NOT a preservation trigger — that
		// would wrongly source-freeze every association-navigation path.
		//
		// Decimal literal: the re-serializer truncates a zero fraction (`2.0` → `2`,
		// which breaks Mendix's Decimal typing) and emits small values in scientific
		// notation (`0.000001` → `1e-06`, which Mendix rejects). A `.` adjacent to a
		// digit marks a numeric literal; preserving the source keeps the exact form.
		// (#18, #19)
		//
		// BUT a `.` inside a QUALIFIED NAME (`L48.Transaction`, `Account2.Name`)
		// must NOT be mistaken for a decimal point just because a name segment ends
		// in a digit — modules/entities ending in a digit are common. Freezing such
		// an expression bypasses association-target-entity resolution
		// (`resolveAssociationPaths`), producing `$T/L48.Assoc/Attr` without the
		// required entity step, which Mendix rejects with CE0117 (ledger #48 root
		// cause). A decimal point's digit run is a standalone number — not preceded
		// by an identifier char; a name separator's `.` follows a token that
		// contains a letter or underscore.
		if source[i] == '.' {
			prevDigit := i > 0 && isDigitByte(source[i-1])
			nextDigit := i+1 < len(source) && isDigitByte(source[i+1])
			if (prevDigit || nextDigit) && !dotIsQualifiedNameSeparator(source, i) {
				return true
			}
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

// parseCurveAnnotation populates result.Curve from @curve(from: (x, y), to: (x, y)).
// Either end may be omitted, which leaves that end straight.
func parseCurveAnnotation(params *parser.AnnotationParamsContext, result *ast.ActivityAnnotations) {
	curve := &ast.FlowCurve{}
	for _, p := range params.AllAnnotationParam() {
		pCtx := p.(*parser.AnnotationParamContext)
		nameCtx := pCtx.AnnotationParamName()
		if nameCtx == nil {
			result.InvalidCurves = append(result.InvalidCurves, strings.TrimSpace(pCtx.GetText()))
			continue
		}
		pt, ok := annotationPointValue(pCtx)
		if !ok {
			result.InvalidCurves = append(result.InvalidCurves, strings.TrimSpace(pCtx.GetText()))
			continue
		}
		switch strings.ToLower(nameCtx.GetText()) {
		case "from":
			curve.From = pt
		case "to":
			curve.To = pt
		default:
			result.InvalidCurves = append(result.InvalidCurves, strings.TrimSpace(pCtx.GetText()))
		}
	}
	if curve.From != nil || curve.To != nil {
		result.Curve = curve
	}
}

// annotationPointValue reads a `(x, y)` parenthesised annotation value into a
// Position. Reports false for any other shape — including a non-integer
// coordinate, which the caller records so validation can refuse it.
//
// A free function rather than the Builder method the association anchors use
// (annotationParenPoint), because extractMicroflowAnnotations has no Builder to
// report through; the caller records the raw text on the AST instead.
func annotationPointValue(paramCtx *parser.AnnotationParamContext) (*ast.Position, bool) {
	paren := paramCtx.AnnotationParenValue()
	if paren == nil {
		return nil, false
	}
	inner := paren.(*parser.AnnotationParenValueContext).AnnotationParams()
	if inner == nil {
		return nil, false
	}
	coords := inner.(*parser.AnnotationParamsContext).AllAnnotationParam()
	if len(coords) != 2 {
		return nil, false
	}
	vals := make([]int, 0, 2)
	for _, c := range coords {
		cCtx := c.(*parser.AnnotationParamContext)
		if cCtx.AnnotationParamName() != nil {
			return nil, false // named, not a coordinate pair
		}
		v, err := strconv.Atoi(strings.TrimSpace(cCtx.GetText()))
		if err != nil {
			return nil, false
		}
		vals = append(vals, v)
	}
	return &ast.Position{X: vals[0], Y: vals[1]}, true
}

// mergeJoinLabel reads the label off a `merge`/`join` statement. Quoting is
// what lets a label collide with a keyword, so it is stripped here rather than
// carried into the AST — the label is matched by string when the merge is
// resolved, and `join "end"` must find `merge "end"`.
func mergeJoinLabel(plain, quoted antlr.TerminalNode) string {
	if quoted != nil {
		return unquoteIdentifier(quoted.GetText())
	}
	if plain != nil {
		return plain.GetText()
	}
	return ""
}
