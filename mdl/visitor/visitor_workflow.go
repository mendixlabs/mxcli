// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
	"github.com/JordtenBulte-OLC/mxcli/mdl/grammar/parser"
)

// ExitCreateWorkflowStatement handles CREATE WORKFLOW statements.
func (b *Builder) ExitCreateWorkflowStatement(ctx *parser.CreateWorkflowStatementContext) {
	names := ctx.AllQualifiedName()
	if len(names) == 0 {
		return
	}

	stmt := &ast.CreateWorkflowStmt{
		Name: buildQualifiedName(names[0]),
	}

	// Parse PARAMETER $Var: Entity
	if ctx.PARAMETER() != nil && ctx.VARIABLE() != nil {
		stmt.ParameterVar = ctx.VARIABLE().GetText()
		// The parameter entity is the second qualified name
		if len(names) > 1 {
			stmt.ParameterEntity = buildQualifiedName(names[1])
		}
	}

	// Parse DISPLAY, DESCRIPTION, EXPORT LEVEL, DUE DATE using positional STRING_LITERAL indexing
	allStrings := ctx.AllSTRING_LITERAL()
	stringIdx := 0

	// DISPLAY 'text'
	if ctx.DISPLAY() != nil && stringIdx < len(allStrings) {
		stmt.DisplayName = unquoteString(allStrings[stringIdx].GetText())
		stringIdx++
	}

	// DESCRIPTION 'text'
	if ctx.DESCRIPTION() != nil && stringIdx < len(allStrings) {
		stmt.Description = unquoteString(allStrings[stringIdx].GetText())
		stringIdx++
	}

	// EXPORT LEVEL (Identifier | API)
	if ctx.EXPORT() != nil && ctx.LEVEL() != nil {
		if ctx.IDENTIFIER() != nil {
			stmt.ExportLevel = ctx.IDENTIFIER().GetText()
		} else if ctx.API() != nil {
			stmt.ExportLevel = "API"
		}
	}

	// Parse OVERVIEW PAGE QualifiedName
	overviewPageIdx := -1
	if ctx.OVERVIEW() != nil && ctx.PAGE() != nil {
		// Find the overview page qualified name
		// It's either names[1] or names[2] depending on whether PARAMETER was present
		startIdx := 1
		if ctx.PARAMETER() != nil {
			startIdx = 2
		}
		if len(names) > startIdx {
			stmt.OverviewPage = buildQualifiedName(names[startIdx])
			overviewPageIdx = startIdx
		}
	}
	_ = overviewPageIdx

	// Parse DUE DATE 'expression'
	if ctx.DUE() != nil && stringIdx < len(allStrings) {
		stmt.DueDate = unquoteString(allStrings[stringIdx].GetText())
		stringIdx++
	}
	_ = stringIdx

	// Parse CREATE OR MODIFY
	createStmt := findParentCreateStatement(ctx)
	if createStmt != nil {
		if createStmt.OR() != nil && (createStmt.MODIFY() != nil || createStmt.REPLACE() != nil) {
			stmt.CreateOrModify = true
		}
	}
	stmt.Documentation = findDocCommentText(ctx)

	// Parse body
	if body := ctx.WorkflowBody(); body != nil {
		stmt.Activities = buildWorkflowBody(body)
	}

	b.statements = append(b.statements, stmt)
}

// exitAlterWorkflowStatement handles ALTER WORKFLOW Module.Name { actions }.
func (b *Builder) exitAlterWorkflowStatement(ctx *parser.AlterStatementContext) {
	qn := ctx.QualifiedName()
	if qn == nil {
		return
	}

	stmt := &ast.AlterWorkflowStmt{
		Name: buildQualifiedName(qn),
	}

	for _, actionCtx := range ctx.AllAlterWorkflowAction() {
		op := buildAlterWorkflowAction(actionCtx.(*parser.AlterWorkflowActionContext))
		if op != nil {
			stmt.Operations = append(stmt.Operations, op)
		}
	}

	b.statements = append(b.statements, stmt)
}

// buildAlterWorkflowAction converts a single ALTER WORKFLOW action to an AST operation.
func buildAlterWorkflowAction(ctx *parser.AlterWorkflowActionContext) ast.AlterWorkflowOp {
	// SET workflowSetProperty
	if ctx.SET() != nil && ctx.WorkflowSetProperty() != nil {
		return buildWorkflowSetPropertyOp(ctx.WorkflowSetProperty().(*parser.WorkflowSetPropertyContext))
	}

	// SET ACTIVITY alterActivityRef activitySetProperty
	if ctx.SET() != nil && ctx.ACTIVITY() != nil && ctx.ActivitySetProperty() != nil {
		if ctx.AlterActivityRef() == nil {
			return nil
		}
		ref, atPos := parseAlterActivityRef(ctx.AlterActivityRef().(*parser.AlterActivityRefContext))
		return buildActivitySetPropertyOp(ctx.ActivitySetProperty().(*parser.ActivitySetPropertyContext), ref, atPos)
	}

	// INSERT AFTER alterActivityRef workflowActivityStmt
	if ctx.INSERT() != nil && ctx.AFTER() != nil && ctx.WorkflowActivityStmt() != nil {
		if ctx.AlterActivityRef() == nil {
			return nil
		}
		ref, atPos := parseAlterActivityRef(ctx.AlterActivityRef().(*parser.AlterActivityRefContext))
		act := buildWorkflowActivityStmt(ctx.WorkflowActivityStmt())
		if act == nil {
			return nil
		}
		return &ast.InsertAfterOp{
			ActivityRef: ref,
			AtPosition:  atPos,
			NewActivity: act,
		}
	}

	// DROP ACTIVITY alterActivityRef
	if ctx.DROP() != nil && ctx.ACTIVITY() != nil {
		if ctx.AlterActivityRef() == nil {
			return nil
		}
		ref, atPos := parseAlterActivityRef(ctx.AlterActivityRef().(*parser.AlterActivityRefContext))
		return &ast.DropActivityOp{
			ActivityRef: ref,
			AtPosition:  atPos,
		}
	}

	// REPLACE ACTIVITY alterActivityRef WITH workflowActivityStmt
	if ctx.REPLACE() != nil && ctx.ACTIVITY() != nil && ctx.WorkflowActivityStmt() != nil {
		if ctx.AlterActivityRef() == nil {
			return nil
		}
		ref, atPos := parseAlterActivityRef(ctx.AlterActivityRef().(*parser.AlterActivityRefContext))
		act := buildWorkflowActivityStmt(ctx.WorkflowActivityStmt())
		if act == nil {
			return nil
		}
		return &ast.ReplaceActivityOp{
			ActivityRef: ref,
			AtPosition:  atPos,
			NewActivity: act,
		}
	}

	// INSERT OUTCOME 'name' ON alterActivityRef { workflowBody }
	if ctx.INSERT() != nil && ctx.OUTCOME() != nil {
		if ctx.AlterActivityRef() == nil || ctx.STRING_LITERAL() == nil {
			return nil
		}
		ref, atPos := parseAlterActivityRef(ctx.AlterActivityRef().(*parser.AlterActivityRefContext))
		outcomeName := unquoteString(ctx.STRING_LITERAL().GetText())
		var activities []ast.WorkflowActivityNode
		if body := ctx.WorkflowBody(); body != nil {
			activities = buildWorkflowBody(body)
		}
		return &ast.InsertOutcomeOp{
			OutcomeName: outcomeName,
			ActivityRef: ref,
			AtPosition:  atPos,
			Activities:  activities,
		}
	}

	// INSERT PATH ON alterActivityRef { workflowBody }
	if ctx.INSERT() != nil && ctx.PATH() != nil && ctx.BOUNDARY() == nil {
		if ctx.AlterActivityRef() == nil {
			return nil
		}
		ref, atPos := parseAlterActivityRef(ctx.AlterActivityRef().(*parser.AlterActivityRefContext))
		var activities []ast.WorkflowActivityNode
		if body := ctx.WorkflowBody(); body != nil {
			activities = buildWorkflowBody(body)
		}
		return &ast.InsertPathOp{
			ActivityRef: ref,
			AtPosition:  atPos,
			Activities:  activities,
		}
	}

	// DROP OUTCOME 'name' ON alterActivityRef
	if ctx.DROP() != nil && ctx.OUTCOME() != nil {
		if ctx.AlterActivityRef() == nil || ctx.STRING_LITERAL() == nil {
			return nil
		}
		ref, atPos := parseAlterActivityRef(ctx.AlterActivityRef().(*parser.AlterActivityRefContext))
		outcomeName := unquoteString(ctx.STRING_LITERAL().GetText())
		return &ast.DropOutcomeOp{
			OutcomeName: outcomeName,
			ActivityRef: ref,
			AtPosition:  atPos,
		}
	}

	// DROP PATH 'caption' ON alterActivityRef
	if ctx.DROP() != nil && ctx.PATH() != nil && ctx.BOUNDARY() == nil {
		if ctx.AlterActivityRef() == nil || ctx.STRING_LITERAL() == nil {
			return nil
		}
		ref, atPos := parseAlterActivityRef(ctx.AlterActivityRef().(*parser.AlterActivityRefContext))
		pathCaption := unquoteString(ctx.STRING_LITERAL().GetText())
		return &ast.DropPathOp{
			PathCaption: pathCaption,
			ActivityRef: ref,
			AtPosition:  atPos,
		}
	}

	// INSERT BOUNDARY EVENT workflowBoundaryEventClause ON alterActivityRef
	if ctx.INSERT() != nil && ctx.BOUNDARY() != nil {
		if ctx.AlterActivityRef() == nil {
			return nil
		}
		ref, atPos := parseAlterActivityRef(ctx.AlterActivityRef().(*parser.AlterActivityRefContext))
		be := buildBoundaryEventNode(ctx.WorkflowBoundaryEventClause())
		return &ast.InsertBoundaryEventOp{
			ActivityRef: ref,
			AtPosition:  atPos,
			EventType:   be.EventType,
			Delay:       be.Delay,
			Activities:  be.Activities,
		}
	}

	// DROP BOUNDARY EVENT ON alterActivityRef
	if ctx.DROP() != nil && ctx.BOUNDARY() != nil {
		if ctx.AlterActivityRef() == nil {
			return nil
		}
		ref, atPos := parseAlterActivityRef(ctx.AlterActivityRef().(*parser.AlterActivityRefContext))
		return &ast.DropBoundaryEventOp{
			ActivityRef: ref,
			AtPosition:  atPos,
		}
	}

	// INSERT CONDITION 'value' ON alterActivityRef { workflowBody }
	if ctx.INSERT() != nil && ctx.CONDITION() != nil {
		if ctx.AlterActivityRef() == nil || ctx.STRING_LITERAL() == nil {
			return nil
		}
		ref, atPos := parseAlterActivityRef(ctx.AlterActivityRef().(*parser.AlterActivityRefContext))
		condition := unquoteString(ctx.STRING_LITERAL().GetText())
		var activities []ast.WorkflowActivityNode
		if body := ctx.WorkflowBody(); body != nil {
			activities = buildWorkflowBody(body)
		}
		return &ast.InsertBranchOp{
			Condition:   condition,
			ActivityRef: ref,
			AtPosition:  atPos,
			Activities:  activities,
		}
	}

	// DROP CONDITION 'value' ON alterActivityRef
	if ctx.DROP() != nil && ctx.CONDITION() != nil {
		if ctx.AlterActivityRef() == nil || ctx.STRING_LITERAL() == nil {
			return nil
		}
		ref, atPos := parseAlterActivityRef(ctx.AlterActivityRef().(*parser.AlterActivityRefContext))
		branchName := unquoteString(ctx.STRING_LITERAL().GetText())
		return &ast.DropBranchOp{
			BranchName:  branchName,
			ActivityRef: ref,
			AtPosition:  atPos,
		}
	}

	return nil
}

// buildWorkflowSetPropertyOp converts a workflowSetProperty context to AST.
func buildWorkflowSetPropertyOp(ctx *parser.WorkflowSetPropertyContext) *ast.SetWorkflowPropertyOp {
	op := &ast.SetWorkflowPropertyOp{}

	if ctx.DISPLAY() != nil {
		op.Property = "display"
		op.Value = unquoteString(ctx.STRING_LITERAL().GetText())
	} else if ctx.DESCRIPTION() != nil {
		op.Property = "description"
		op.Value = unquoteString(ctx.STRING_LITERAL().GetText())
	} else if ctx.EXPORT() != nil {
		op.Property = "export_level"
		if ctx.API() != nil {
			op.Value = "API"
		} else if ctx.IDENTIFIER() != nil {
			op.Value = ctx.IDENTIFIER().GetText()
		}
	} else if ctx.DUE() != nil {
		op.Property = "due_date"
		op.Value = unquoteString(ctx.STRING_LITERAL().GetText())
	} else if ctx.OVERVIEW() != nil {
		op.Property = "overview_page"
		if qn := ctx.QualifiedName(); qn != nil {
			op.Entity = buildQualifiedName(qn)
		}
	} else if ctx.PARAMETER() != nil {
		op.Property = "parameter"
		op.Value = ctx.VARIABLE().GetText()
		if qn := ctx.QualifiedName(); qn != nil {
			op.Entity = buildQualifiedName(qn)
		}
	}

	return op
}

// buildActivitySetPropertyOp converts an activitySetProperty context to AST.
func buildActivitySetPropertyOp(ctx *parser.ActivitySetPropertyContext, ref string, atPos int) *ast.SetActivityPropertyOp {
	op := &ast.SetActivityPropertyOp{
		ActivityRef: ref,
		AtPosition:  atPos,
	}

	if ctx.PAGE() != nil {
		op.Property = "page"
		if qn := ctx.QualifiedName(); qn != nil {
			op.PageName = buildQualifiedName(qn)
		}
	} else if ctx.DESCRIPTION() != nil {
		op.Property = "description"
		op.Value = unquoteString(ctx.STRING_LITERAL().GetText())
	} else if ctx.TARGETING() != nil && ctx.MICROFLOW() != nil {
		op.Property = "targeting_microflow"
		if qn := ctx.QualifiedName(); qn != nil {
			op.Microflow = buildQualifiedName(qn)
		}
	} else if ctx.TARGETING() != nil && ctx.XPATH() != nil {
		op.Property = "targeting_xpath"
		op.Value = unquoteString(ctx.STRING_LITERAL().GetText())
	} else if ctx.DUE() != nil {
		op.Property = "due_date"
		op.Value = unquoteString(ctx.STRING_LITERAL().GetText())
	}

	return op
}

// parseAlterActivityRef extracts the activity reference name and optional position.
func parseAlterActivityRef(ctx *parser.AlterActivityRefContext) (string, int) {
	if ctx == nil {
		return "", 0
	}
	name := ""
	if iok := ctx.IdentifierOrKeyword(); iok != nil {
		name = identifierOrKeywordText(iok)
	} else if ctx.STRING_LITERAL() != nil {
		name = unquoteString(ctx.STRING_LITERAL().GetText())
	}

	atPos := 0
	if ctx.AT() != nil && ctx.NUMBER_LITERAL() != nil {
		atPos = parseInt(ctx.NUMBER_LITERAL().GetText())
	}

	return name, atPos
}

// buildWorkflowBody converts a workflow body context to activity nodes.
func buildWorkflowBody(ctx parser.IWorkflowBodyContext) []ast.WorkflowActivityNode {
	if ctx == nil {
		return nil
	}
	bodyCtx := ctx.(*parser.WorkflowBodyContext)
	var activities []ast.WorkflowActivityNode

	for _, actCtx := range bodyCtx.AllWorkflowActivityStmt() {
		act := buildWorkflowActivityStmt(actCtx)
		if act != nil {
			activities = append(activities, act)
		}
	}

	return activities
}

// buildWorkflowActivityStmt dispatches to the appropriate builder.
func buildWorkflowActivityStmt(ctx parser.IWorkflowActivityStmtContext) ast.WorkflowActivityNode {
	if ctx == nil {
		return nil
	}
	actCtx := ctx.(*parser.WorkflowActivityStmtContext)

	if ut := actCtx.WorkflowUserTaskStmt(); ut != nil {
		return buildWorkflowUserTask(ut)
	}
	if cm := actCtx.WorkflowCallMicroflowStmt(); cm != nil {
		return buildWorkflowCallMicroflow(cm)
	}
	if cw := actCtx.WorkflowCallWorkflowStmt(); cw != nil {
		return buildWorkflowCallWorkflow(cw)
	}
	if d := actCtx.WorkflowDecisionStmt(); d != nil {
		return buildWorkflowDecision(d)
	}
	if ps := actCtx.WorkflowParallelSplitStmt(); ps != nil {
		return buildWorkflowParallelSplit(ps)
	}
	if jt := actCtx.WorkflowJumpToStmt(); jt != nil {
		return buildWorkflowJumpTo(jt)
	}
	if wt := actCtx.WorkflowWaitForTimerStmt(); wt != nil {
		return buildWorkflowWaitForTimer(wt)
	}
	if wn := actCtx.WorkflowWaitForNotificationStmt(); wn != nil {
		return buildWorkflowWaitForNotification(wn)
	}
	if ann := actCtx.WorkflowAnnotationStmt(); ann != nil {
		return buildWorkflowAnnotation(ann)
	}
	return nil
}

// buildWorkflowUserTask builds a WorkflowUserTaskNode from the grammar context.
func buildWorkflowUserTask(ctx parser.IWorkflowUserTaskStmtContext) *ast.WorkflowUserTaskNode {
	utCtx := ctx.(*parser.WorkflowUserTaskStmtContext)

	// The task name may be a bare IDENTIFIER (ut1) or a QUOTED_IDENTIFIER ("ut1")
	// to allow reserved-word names. Always strip surrounding quotes.
	taskName := ""
	if id := utCtx.IDENTIFIER(); id != nil {
		taskName = id.GetText()
	} else if qid := utCtx.QUOTED_IDENTIFIER(); qid != nil {
		taskName = unquoteIdentifier(qid.GetText())
	}

	node := &ast.WorkflowUserTaskNode{
		Name:        taskName,
		IsMultiUser: utCtx.MULTI() != nil,
	}

	// Caption is the first STRING_LITERAL
	allStrings := utCtx.AllSTRING_LITERAL()
	if len(allStrings) > 0 {
		node.Caption = unquoteString(allStrings[0].GetText())
	}

	// Qualified names: PAGE, TARGETING MICROFLOW, ENTITY (in order)
	names := utCtx.AllQualifiedName()
	nameIdx := 0

	if utCtx.PAGE() != nil && nameIdx < len(names) {
		node.Page = buildQualifiedName(names[nameIdx])
		nameIdx++
	}

	// Determine if group targeting (TARGETING GROUPS vs TARGETING [USERS])
	isGroupTargeting := len(utCtx.AllGROUPS()) > 0

	if utCtx.MICROFLOW() != nil && nameIdx < len(names) {
		if isGroupTargeting {
			node.Targeting.Kind = "group_microflow"
		} else {
			node.Targeting.Kind = "microflow"
		}
		node.Targeting.Microflow = buildQualifiedName(names[nameIdx])
		nameIdx++
	}

	stringIdx := 1 // allStrings[0] is the caption
	if utCtx.XPATH() != nil && stringIdx < len(allStrings) {
		if isGroupTargeting {
			node.Targeting.Kind = "group_xpath"
		} else {
			node.Targeting.Kind = "xpath"
		}
		node.Targeting.XPath = unquoteString(allStrings[stringIdx].GetText())
		stringIdx++
	}

	if utCtx.ENTITY() != nil && nameIdx < len(names) {
		node.Entity = buildQualifiedName(names[nameIdx])
	}

	if utCtx.DUE() != nil && utCtx.DATE_TYPE() != nil && stringIdx < len(allStrings) {
		node.DueDate = unquoteString(allStrings[stringIdx].GetText())
		stringIdx++
	}

	if utCtx.DESCRIPTION() != nil && stringIdx < len(allStrings) {
		node.TaskDescription = unquoteString(allStrings[stringIdx].GetText())
		stringIdx++
	}

	// Outcomes
	for _, outcomeCtx := range utCtx.AllWorkflowUserTaskOutcome() {
		outcome := buildWorkflowUserTaskOutcome(outcomeCtx)
		node.Outcomes = append(node.Outcomes, outcome)
	}

	// BoundaryEvents (Issue #7)
	for _, beCtx := range utCtx.AllWorkflowBoundaryEventClause() {
		node.BoundaryEvents = append(node.BoundaryEvents, buildBoundaryEventNode(beCtx))
	}

	return node
}

// buildWorkflowUserTaskOutcome builds a WorkflowUserTaskOutcomeNode.
func buildWorkflowUserTaskOutcome(ctx parser.IWorkflowUserTaskOutcomeContext) ast.WorkflowUserTaskOutcomeNode {
	outCtx := ctx.(*parser.WorkflowUserTaskOutcomeContext)
	outcome := ast.WorkflowUserTaskOutcomeNode{
		Caption: unquoteString(outCtx.STRING_LITERAL().GetText()),
	}
	if body := outCtx.WorkflowBody(); body != nil {
		outcome.Activities = buildWorkflowBody(body)
	}
	return outcome
}

// buildWorkflowCallMicroflow builds a WorkflowCallMicroflowNode.
func buildWorkflowCallMicroflow(ctx parser.IWorkflowCallMicroflowStmtContext) *ast.WorkflowCallMicroflowNode {
	cmCtx := ctx.(*parser.WorkflowCallMicroflowStmtContext)
	node := &ast.WorkflowCallMicroflowNode{
		Microflow: buildQualifiedName(cmCtx.QualifiedName()),
	}

	if cmCtx.COMMENT() != nil && cmCtx.STRING_LITERAL() != nil {
		node.Caption = unquoteString(cmCtx.STRING_LITERAL().GetText())
	}

	for _, outcomeCtx := range cmCtx.AllWorkflowConditionOutcome() {
		outcome := buildWorkflowConditionOutcome(outcomeCtx)
		node.Outcomes = append(node.Outcomes, outcome)
	}

	// Parameter mappings (Issue #10)
	for _, pmCtx := range cmCtx.AllWorkflowParameterMapping() {
		pmCtx2 := pmCtx.(*parser.WorkflowParameterMappingContext)
		mapping := ast.WorkflowParameterMappingNode{
			Parameter:  pmCtx2.QualifiedName().GetText(),
			Expression: unquoteString(pmCtx2.STRING_LITERAL().GetText()),
		}
		node.ParameterMappings = append(node.ParameterMappings, mapping)
	}

	// BoundaryEvents (Issue #7)
	for _, beCtx := range cmCtx.AllWorkflowBoundaryEventClause() {
		node.BoundaryEvents = append(node.BoundaryEvents, buildBoundaryEventNode(beCtx))
	}

	return node
}

// buildWorkflowCallWorkflow builds a WorkflowCallWorkflowNode.
func buildWorkflowCallWorkflow(ctx parser.IWorkflowCallWorkflowStmtContext) *ast.WorkflowCallWorkflowNode {
	cwCtx := ctx.(*parser.WorkflowCallWorkflowStmtContext)
	node := &ast.WorkflowCallWorkflowNode{
		Workflow: buildQualifiedName(cwCtx.QualifiedName()),
	}

	if cwCtx.COMMENT() != nil && cwCtx.STRING_LITERAL() != nil {
		node.Caption = unquoteString(cwCtx.STRING_LITERAL().GetText())
	}

	// Parameter mappings
	for _, pmCtx := range cwCtx.AllWorkflowParameterMapping() {
		pmCtx2 := pmCtx.(*parser.WorkflowParameterMappingContext)
		mapping := ast.WorkflowParameterMappingNode{
			Parameter:  pmCtx2.QualifiedName().GetText(),
			Expression: unquoteString(pmCtx2.STRING_LITERAL().GetText()),
		}
		node.ParameterMappings = append(node.ParameterMappings, mapping)
	}

	return node
}

// buildWorkflowDecision builds a WorkflowDecisionNode.
func buildWorkflowDecision(ctx parser.IWorkflowDecisionStmtContext) *ast.WorkflowDecisionNode {
	dCtx := ctx.(*parser.WorkflowDecisionStmtContext)
	node := &ast.WorkflowDecisionNode{}

	allStrings := dCtx.AllSTRING_LITERAL()
	stringIdx := 0

	// First STRING_LITERAL is the expression (if present and COMMENT is not present or expression comes first)
	if len(allStrings) > 0 && dCtx.COMMENT() == nil {
		// All strings are expression
		node.Expression = unquoteString(allStrings[0].GetText())
		stringIdx = 1
	} else if len(allStrings) > 0 && dCtx.COMMENT() != nil {
		// Distinguish expression from comment
		if len(allStrings) >= 2 {
			node.Expression = unquoteString(allStrings[0].GetText())
			node.Caption = unquoteString(allStrings[1].GetText())
		} else {
			// Only one string with COMMENT - it's the caption
			node.Caption = unquoteString(allStrings[0].GetText())
		}
		stringIdx = len(allStrings)
	}
	_ = stringIdx

	for _, outcomeCtx := range dCtx.AllWorkflowConditionOutcome() {
		outcome := buildWorkflowConditionOutcome(outcomeCtx)
		node.Outcomes = append(node.Outcomes, outcome)
	}

	return node
}

// buildWorkflowConditionOutcome builds a WorkflowConditionOutcomeNode.
func buildWorkflowConditionOutcome(ctx parser.IWorkflowConditionOutcomeContext) ast.WorkflowConditionOutcomeNode {
	coCtx := ctx.(*parser.WorkflowConditionOutcomeContext)
	outcome := ast.WorkflowConditionOutcomeNode{}

	if coCtx.TRUE() != nil {
		outcome.Value = "True"
	} else if coCtx.FALSE() != nil {
		outcome.Value = "False"
	} else if coCtx.DEFAULT() != nil {
		outcome.Value = "Default"
	} else if coCtx.STRING_LITERAL() != nil {
		outcome.Value = unquoteString(coCtx.STRING_LITERAL().GetText())
	}

	if body := coCtx.WorkflowBody(); body != nil {
		outcome.Activities = buildWorkflowBody(body)
	}

	return outcome
}

// buildWorkflowParallelSplit builds a WorkflowParallelSplitNode.
func buildWorkflowParallelSplit(ctx parser.IWorkflowParallelSplitStmtContext) *ast.WorkflowParallelSplitNode {
	psCtx := ctx.(*parser.WorkflowParallelSplitStmtContext)
	node := &ast.WorkflowParallelSplitNode{}

	if psCtx.COMMENT() != nil && psCtx.STRING_LITERAL() != nil {
		node.Caption = unquoteString(psCtx.STRING_LITERAL().GetText())
	}

	for _, pathCtx := range psCtx.AllWorkflowParallelPath() {
		path := buildWorkflowParallelPath(pathCtx)
		node.Paths = append(node.Paths, path)
	}

	return node
}

// buildWorkflowParallelPath builds a WorkflowParallelPathNode.
func buildWorkflowParallelPath(ctx parser.IWorkflowParallelPathContext) ast.WorkflowParallelPathNode {
	ppCtx := ctx.(*parser.WorkflowParallelPathContext)
	path := ast.WorkflowParallelPathNode{}

	if ppCtx.NUMBER_LITERAL() != nil {
		path.PathNumber = parseInt(ppCtx.NUMBER_LITERAL().GetText())
	}

	if body := ppCtx.WorkflowBody(); body != nil {
		path.Activities = buildWorkflowBody(body)
	}

	return path
}

// buildWorkflowJumpTo builds a WorkflowJumpToNode.
func buildWorkflowJumpTo(ctx parser.IWorkflowJumpToStmtContext) *ast.WorkflowJumpToNode {
	jtCtx := ctx.(*parser.WorkflowJumpToStmtContext)

	target := ""
	if id := jtCtx.IDENTIFIER(); id != nil {
		target = id.GetText()
	} else if qid := jtCtx.QUOTED_IDENTIFIER(); qid != nil {
		target = unquoteIdentifier(qid.GetText())
	}

	node := &ast.WorkflowJumpToNode{
		Target: target,
	}

	if jtCtx.COMMENT() != nil && jtCtx.STRING_LITERAL() != nil {
		node.Caption = unquoteString(jtCtx.STRING_LITERAL().GetText())
	}

	return node
}

// buildWorkflowWaitForTimer builds a WorkflowWaitForTimerNode.
func buildWorkflowWaitForTimer(ctx parser.IWorkflowWaitForTimerStmtContext) *ast.WorkflowWaitForTimerNode {
	wtCtx := ctx.(*parser.WorkflowWaitForTimerStmtContext)
	node := &ast.WorkflowWaitForTimerNode{}

	allStrings := wtCtx.AllSTRING_LITERAL()
	if len(allStrings) > 0 && wtCtx.COMMENT() == nil {
		node.DelayExpression = unquoteString(allStrings[0].GetText())
	} else if len(allStrings) >= 2 && wtCtx.COMMENT() != nil {
		node.DelayExpression = unquoteString(allStrings[0].GetText())
		node.Caption = unquoteString(allStrings[1].GetText())
	} else if len(allStrings) == 1 && wtCtx.COMMENT() != nil {
		node.Caption = unquoteString(allStrings[0].GetText())
	}

	return node
}

// buildWorkflowWaitForNotification builds a WorkflowWaitForNotificationNode.
func buildWorkflowWaitForNotification(ctx parser.IWorkflowWaitForNotificationStmtContext) *ast.WorkflowWaitForNotificationNode {
	wnCtx := ctx.(*parser.WorkflowWaitForNotificationStmtContext)
	node := &ast.WorkflowWaitForNotificationNode{}

	if wnCtx.COMMENT() != nil && wnCtx.STRING_LITERAL() != nil {
		node.Caption = unquoteString(wnCtx.STRING_LITERAL().GetText())
	}

	// BoundaryEvents (Issue #7)
	for _, beCtx := range wnCtx.AllWorkflowBoundaryEventClause() {
		node.BoundaryEvents = append(node.BoundaryEvents, buildBoundaryEventNode(beCtx))
	}

	return node
}

// buildBoundaryEventNode builds a WorkflowBoundaryEventNode from a grammar context.
func buildBoundaryEventNode(beCtx parser.IWorkflowBoundaryEventClauseContext) ast.WorkflowBoundaryEventNode {
	beCtx2 := beCtx.(*parser.WorkflowBoundaryEventClauseContext)
	be := ast.WorkflowBoundaryEventNode{}
	if beCtx2.NON() != nil {
		be.EventType = "NonInterruptingTimer"
	} else if beCtx2.INTERRUPTING() != nil {
		be.EventType = "InterruptingTimer"
	} else {
		be.EventType = "Timer"
	}
	if beCtx2.STRING_LITERAL() != nil {
		be.Delay = unquoteString(beCtx2.STRING_LITERAL().GetText())
	}
	if body := beCtx2.WorkflowBody(); body != nil {
		be.Activities = buildWorkflowBody(body)
	}
	return be
}

// buildWorkflowAnnotation builds a WorkflowAnnotationActivityNode from the grammar context.
func buildWorkflowAnnotation(ctx parser.IWorkflowAnnotationStmtContext) *ast.WorkflowAnnotationActivityNode {
	annCtx := ctx.(*parser.WorkflowAnnotationStmtContext)
	node := &ast.WorkflowAnnotationActivityNode{}
	if annCtx.STRING_LITERAL() != nil {
		node.Text = unquoteString(annCtx.STRING_LITERAL().GetText())
	}
	return node
}

// parseInt parses a string as an integer, returning 0 on failure.
func parseInt(s string) int {
	n := 0
	for _, c := range s {
		if c >= '0' && c <= '9' {
			n = n*10 + int(c-'0')
		}
	}
	return n
}
