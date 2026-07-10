// SPDX-License-Identifier: Apache-2.0

// Package executor - Microflow builder: CRUD & data actions
package executor

import (
	"fmt"
	"strings"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
	"github.com/JordtenBulte-OLC/mxcli/mdl/types"
	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/domainmodel"
	"github.com/JordtenBulte-OLC/mxcli/sdk/microflows"
)

// addCreateVariableAction creates a DECLARE statement as a CreateVariableAction.
func (fb *flowBuilder) addCreateVariableAction(s *ast.DeclareStmt) model.ID {
	// Resolve TypeEnumeration → TypeEntity ambiguity using the domain model
	declType := s.Type
	if declType.Kind == ast.TypeEnumeration && declType.EnumRef != nil && fb.backend != nil {
		if fb.isEntity(declType.EnumRef.Module, declType.EnumRef.Name) {
			declType = ast.DataType{Kind: ast.TypeEntity, EntityRef: declType.EnumRef}
		}
	}

	// Register the variable as declared
	typeName := declType.Kind.String()
	fb.declaredVars[s.Variable] = typeName

	action := &microflows.CreateVariableAction{
		BaseElement:       model.BaseElement{ID: model.ID(types.GenerateID())},
		ErrorHandlingType: fb.ehType(nil),
		VariableName:      s.Variable,
		DataType:          convertASTToMicroflowDataType(declType, nil),
		InitialValue:      fb.exprToString(s.InitialValue),
	}

	activity := &microflows.ActionActivity{
		BaseActivity: microflows.BaseActivity{
			BaseMicroflowObject: microflows.BaseMicroflowObject{
				BaseElement: model.BaseElement{ID: model.ID(types.GenerateID())},
				Position:    model.Point{X: fb.posX, Y: fb.posY},
				Size:        model.Size{Width: ActivityWidth, Height: ActivityHeight},
			},
			AutoGenerateCaption: true,
		},
		Action: action,
	}

	fb.objects = append(fb.objects, activity)
	fb.posX += fb.spacing
	return activity.ID
}

// addChangeVariableAction creates a SET statement as a ChangeVariableAction.
func (fb *flowBuilder) addChangeVariableAction(s *ast.MfSetStmt) model.ID {
	// Validate that the variable has been declared
	if !fb.isVariableDeclared(s.Target) {
		fb.addErrorWithExample(
			fmt.Sprintf("variable '%s' is not declared", s.Target),
			errorExampleDeclareVariable(s.Target))
	}

	action := &microflows.ChangeVariableAction{
		BaseElement:       model.BaseElement{ID: model.ID(types.GenerateID())},
		ErrorHandlingType: fb.ehType(nil),
		VariableName:      s.Target,
		Value:             fb.exprToString(s.Value),
	}

	activity := &microflows.ActionActivity{
		BaseActivity: microflows.BaseActivity{
			BaseMicroflowObject: microflows.BaseMicroflowObject{
				BaseElement: model.BaseElement{ID: model.ID(types.GenerateID())},
				Position:    model.Point{X: fb.posX, Y: fb.posY},
				Size:        model.Size{Width: ActivityWidth, Height: ActivityHeight},
			},
			AutoGenerateCaption: true,
		},
		Action: action,
	}

	fb.objects = append(fb.objects, activity)
	fb.posX += fb.spacing
	return activity.ID
}

// addCreateObjectAction creates a CREATE OBJECT statement.
func (fb *flowBuilder) addCreateObjectAction(s *ast.CreateObjectStmt) model.ID {
	action := &microflows.CreateObjectAction{
		BaseElement:       model.BaseElement{ID: model.ID(types.GenerateID())},
		ErrorHandlingType: fb.ehType(s.ErrorHandling),
		OutputVariable:    s.Variable,
		Commit:            microflows.CommitTypeNo,
	}
	// Set entity reference as qualified name (BY_NAME_REFERENCE)
	entityQN := ""
	if s.EntityType.Module != "" && s.EntityType.Name != "" {
		entityQN = s.EntityType.Module + "." + s.EntityType.Name
		action.EntityQualifiedName = entityQN
	}

	// Register variable type for CHANGE statements
	if fb.varTypes != nil && entityQN != "" {
		fb.varTypes[s.Variable] = entityQN
	}

	// Build InitialMembers for each SET assignment
	for _, change := range s.Changes {
		memberChange := &microflows.MemberChange{
			BaseElement: model.BaseElement{ID: model.ID(types.GenerateID())},
			Type:        microflows.MemberChangeTypeSet,
			Value:       fb.memberExpressionToString(change.Value, entityQN, change.Attribute),
		}
		fb.resolveMemberChange(memberChange, change.Attribute, entityQN)
		action.InitialMembers = append(action.InitialMembers, memberChange)
	}

	activityX := fb.posX
	activity := &microflows.ActionActivity{
		BaseActivity: microflows.BaseActivity{
			BaseMicroflowObject: microflows.BaseMicroflowObject{
				BaseElement: model.BaseElement{ID: model.ID(types.GenerateID())},
				Position:    model.Point{X: fb.posX, Y: fb.posY},
				Size:        model.Size{Width: ActivityWidth, Height: ActivityHeight},
			},
			AutoGenerateCaption: true,
			ErrorHandlingType:   fb.ehType(s.ErrorHandling),
		},
		Action: action,
	}

	fb.objects = append(fb.objects, activity)
	fb.posX += fb.spacing

	fb.finishCustomErrorHandler(activity.ID, activityX, s.ErrorHandling, s.Variable)

	return activity.ID
}

// addCommitAction creates a COMMIT statement.
func (fb *flowBuilder) addCommitAction(s *ast.MfCommitStmt) model.ID {
	action := &microflows.CommitObjectsAction{
		BaseElement:       model.BaseElement{ID: model.ID(types.GenerateID())},
		ErrorHandlingType: fb.ehType(s.ErrorHandling),
		CommitVariable:    s.Variable,
		WithEvents:        s.WithEvents,
		RefreshInClient:   s.RefreshInClient,
	}

	activityX := fb.posX
	activity := &microflows.ActionActivity{
		BaseActivity: microflows.BaseActivity{
			BaseMicroflowObject: microflows.BaseMicroflowObject{
				BaseElement: model.BaseElement{ID: model.ID(types.GenerateID())},
				Position:    model.Point{X: fb.posX, Y: fb.posY},
				Size:        model.Size{Width: ActivityWidth, Height: ActivityHeight},
			},
			AutoGenerateCaption: true,
		},
		Action: action,
	}

	fb.objects = append(fb.objects, activity)
	fb.posX += fb.spacing

	fb.finishCustomErrorHandler(activity.ID, activityX, s.ErrorHandling, "")

	return activity.ID
}

// addDeleteAction creates a DELETE statement.
func (fb *flowBuilder) addDeleteAction(s *ast.DeleteObjectStmt) model.ID {
	action := &microflows.DeleteObjectAction{
		BaseElement:    model.BaseElement{ID: model.ID(types.GenerateID())},
		DeleteVariable: s.Variable,
	}

	activityX := fb.posX
	activity := &microflows.ActionActivity{
		BaseActivity: microflows.BaseActivity{
			BaseMicroflowObject: microflows.BaseMicroflowObject{
				BaseElement: model.BaseElement{ID: model.ID(types.GenerateID())},
				Position:    model.Point{X: fb.posX, Y: fb.posY},
				Size:        model.Size{Width: ActivityWidth, Height: ActivityHeight},
			},
			AutoGenerateCaption: true,
			ErrorHandlingType:   fb.ehType(s.ErrorHandling),
		},
		Action: action,
	}

	fb.objects = append(fb.objects, activity)
	fb.posX += fb.spacing

	fb.finishCustomErrorHandler(activity.ID, activityX, s.ErrorHandling, "")

	return activity.ID
}

// addRollbackAction creates a ROLLBACK statement.
func (fb *flowBuilder) addRollbackAction(s *ast.RollbackStmt) model.ID {
	action := &microflows.RollbackObjectAction{
		BaseElement:      model.BaseElement{ID: model.ID(types.GenerateID())},
		RollbackVariable: s.Variable,
		RefreshInClient:  s.RefreshInClient,
	}

	activity := &microflows.ActionActivity{
		BaseActivity: microflows.BaseActivity{
			BaseMicroflowObject: microflows.BaseMicroflowObject{
				BaseElement: model.BaseElement{ID: model.ID(types.GenerateID())},
				Position:    model.Point{X: fb.posX, Y: fb.posY},
				Size:        model.Size{Width: ActivityWidth, Height: ActivityHeight},
			},
			AutoGenerateCaption: true,
		},
		Action: action,
	}

	fb.objects = append(fb.objects, activity)
	fb.posX += fb.spacing

	return activity.ID
}

// addChangeObjectAction creates a CHANGE statement.
func (fb *flowBuilder) addChangeObjectAction(s *ast.ChangeObjectStmt) model.ID {
	// CE0032 rejects change actions with no items that do not commit the
	// object. The published error text only mentions items/commit, but
	// `mx check` also accepts RefreshInClient=true as a third valid escape.
	// The builder auto-promotes empty changes to refresh-only so describe →
	// exec of such actions stays valid without requiring authored MDL to say
	// `refresh` explicitly; when the author wrote `refresh`, we keep the
	// same flag for non-empty changes too.
	action := &microflows.ChangeObjectAction{
		BaseElement:       model.BaseElement{ID: model.ID(types.GenerateID())},
		ErrorHandlingType: fb.ehType(nil),
		ChangeVariable:    s.Variable,
		Commit:            microflows.CommitTypeNo,
		RefreshInClient:   s.RefreshInClient || len(s.Changes) == 0,
	}

	// Look up entity type from variable scope
	entityQN := ""
	if fb.varTypes != nil {
		entityQN = fb.varTypes[s.Variable]
	}

	// Build MemberChange items for each SET assignment
	for _, change := range s.Changes {
		memberChange := &microflows.MemberChange{
			BaseElement: model.BaseElement{ID: model.ID(types.GenerateID())},
			Type:        microflows.MemberChangeTypeSet,
			Value:       fb.memberExpressionToString(change.Value, entityQN, change.Attribute),
		}
		fb.resolveMemberChange(memberChange, change.Attribute, entityQN)
		action.Changes = append(action.Changes, memberChange)
	}

	activity := &microflows.ActionActivity{
		BaseActivity: microflows.BaseActivity{
			BaseMicroflowObject: microflows.BaseMicroflowObject{
				BaseElement: model.BaseElement{ID: model.ID(types.GenerateID())},
				Position:    model.Point{X: fb.posX, Y: fb.posY},
				Size:        model.Size{Width: ActivityWidth, Height: ActivityHeight},
			},
			AutoGenerateCaption: true,
		},
		Action: action,
	}

	fb.objects = append(fb.objects, activity)
	fb.posX += fb.spacing
	return activity.ID
}

func (fb *flowBuilder) addEnumSplit(s *ast.EnumSplitStmt) model.ID {
	if count := enumSplitBranchCount(s); count > maxEnumSplitBranches {
		fb.addError("enum split has %d branches; at most %d branches are supported", count, maxEnumSplitBranches)
		return ""
	}

	if fb.measurer == nil {
		fb.measurer = &layoutMeasurer{varTypes: fb.varTypes}
	}

	splitX := fb.posX
	centerY := fb.posY
	split := &microflows.ExclusiveSplit{
		BaseMicroflowObject: microflows.BaseMicroflowObject{
			BaseElement: model.BaseElement{ID: model.ID(types.GenerateID())},
			Position:    model.Point{X: splitX, Y: centerY},
			Size:        model.Size{Width: SplitWidth, Height: SplitHeight},
		},
		Caption: "$" + s.Variable,
		SplitCondition: &microflows.ExpressionSplitCondition{
			BaseElement: model.BaseElement{ID: model.ID(types.GenerateID())},
			Expression:  "$" + s.Variable,
		},
		ErrorHandlingType: fb.ehType(nil),
	}
	fb.objects = append(fb.objects, split)
	splitID := split.ID
	if fb.pendingAnnotations != nil {
		fb.applyAnnotations(splitID, fb.pendingAnnotations)
		fb.pendingAnnotations = nil
	}

	type branch struct {
		values []string
		body   []ast.MicroflowStatement
	}
	branches := make([]branch, 0, len(s.Cases)+1)
	for _, c := range s.Cases {
		branches = append(branches, branch{values: enumSplitCaseValues(c), body: c.Body})
	}
	if len(s.ElseBody) > 0 {
		branches = append(branches, branch{body: s.ElseBody})
	}

	branchWidth := 0
	for _, br := range branches {
		w := fb.measurer.measureStatements(br.body).Width
		if w > branchWidth {
			branchWidth = w
		}
	}
	if branchWidth == 0 {
		branchWidth = HorizontalSpacing / 2
	}
	mergeX := splitX + SplitWidth + HorizontalSpacing/2 + branchWidth + HorizontalSpacing/2
	var merge *microflows.ExclusiveMerge
	ensureMerge := func() *microflows.ExclusiveMerge {
		if merge == nil {
			merge = &microflows.ExclusiveMerge{
				BaseMicroflowObject: microflows.BaseMicroflowObject{
					BaseElement: model.BaseElement{ID: model.ID(types.GenerateID())},
					Position:    model.Point{X: mergeX, Y: centerY},
					Size:        model.Size{Width: MergeSize, Height: MergeSize},
				},
			}
			fb.objects = append(fb.objects, merge)
		}
		return merge
	}

	// Precompute each branch's height for cumulative Y positioning.
	branchHeights := make([]int, len(branches))
	for i, br := range branches {
		h := fb.measurer.measureStatements(br.body).Height
		branchHeights[i] = max(h, ActivityHeight)
	}
	// First branch is centred on the happy-path line; subsequent branches
	// are placed so there is exactly BranchGap of empty space between them.
	branchYs := make([]int, len(branches))
	if len(branches) > 0 {
		// Centre the whole stack on centerY
		totalH := 0
		for _, h := range branchHeights {
			totalH += h
		}
		totalH += (len(branches) - 1) * BranchGap
		y := centerY - totalH/2 + branchHeights[0]/2
		for i := range branches {
			branchYs[i] = y
			if i < len(branches)-1 {
				y += branchHeights[i]/2 + BranchGap + branchHeights[i+1]/2
			}
		}
	}

	savedEndsWithReturn := fb.endsWithReturn
	allBranchesReturn := len(branches) > 0
	for i, br := range branches {
		branchY := branchYs[i]
		fb.posX = splitX + SplitWidth + HorizontalSpacing/2
		fb.posY = branchY
		fb.endsWithReturn = false

		lastID := model.ID("")
		pendingCase := ""
		var prevAnchor *ast.FlowAnchors
		for j, stmt := range br.body {
			thisAnchor := stmtOwnAnchor(stmt)
			actID := fb.addStatement(stmt)
			if actID == "" {
				continue
			}
			if fb.pendingAnnotations != nil {
				fb.applyAnnotations(actID, fb.pendingAnnotations)
				fb.pendingAnnotations = nil
			}
			if lastID == "" {
				fb.addGroupedEnumSplitFlows(splitID, actID, br.values, i, splitX+SplitWidth+HorizontalSpacing/4, branchY)
				// The first statement in a case can carry @anchor(from:…,
				// to:…) that should apply to the split→firstActivity flow.
				// addGroupedEnumSplitFlows appends one flow per case value;
				// anchor the last one so `@anchor(to: top)` etc. round-trips
				// through describe → exec without silently dropping.
				if thisAnchor != nil && len(fb.flows) > 0 {
					applyUserAnchors(fb.flows[len(fb.flows)-1], nil, thisAnchor)
				}
			} else {
				var flow *microflows.SequenceFlow
				if pendingCase != "" {
					flow = newHorizontalFlowWithCase(lastID, actID, pendingCase)
					pendingCase = ""
				} else {
					flow = newHorizontalFlow(lastID, actID)
				}
				applyUserAnchors(flow, prevAnchor, thisAnchor)
				fb.flows = append(fb.flows, flow)
				if fb.emptyErrorHandlerFrom == lastID {
					fb.addPendingErrorHandlerFlowForStatement(lastID, actID, stmt, statementsReferenceVar(br.body[j+1:], fb.errorHandlerSkipVar))
				}
			}
			prevAnchor = thisAnchor
			if fb.nextConnectionPoint != "" {
				lastID = fb.nextConnectionPoint
				fb.nextConnectionPoint = ""
				pendingCase = fb.nextFlowCase
				fb.nextFlowCase = ""
			} else {
				lastID = actID
			}
		}

		if lastStmtIsReturn(br.body) {
			continue
		}
		allBranchesReturn = false
		if lastID == "" {
			fb.addGroupedEnumSplitFlows(splitID, ensureMerge().ID, br.values, i, splitX+SplitWidth+HorizontalSpacing/4, branchY)
		} else {
			if pendingCase != "" {
				fb.flows = append(fb.flows, newHorizontalFlowWithCase(lastID, ensureMerge().ID, pendingCase))
			} else {
				fb.flows = append(fb.flows, newHorizontalFlow(lastID, ensureMerge().ID))
			}
		}
	}

	fb.posX = mergeX + HorizontalSpacing/2
	fb.posY = centerY
	fb.endsWithReturn = savedEndsWithReturn
	if allBranchesReturn {
		fb.endsWithReturn = true
	} else {
		fb.nextConnectionPoint = ensureMerge().ID
	}
	return splitID
}

func (fb *flowBuilder) addInheritanceSplit(s *ast.InheritanceSplitStmt) model.ID {
	if len(s.Cases) == 0 && len(s.ElseBody) == 0 {
		split := &microflows.InheritanceSplit{
			BaseMicroflowObject: microflows.BaseMicroflowObject{
				BaseElement: model.BaseElement{ID: model.ID(types.GenerateID())},
				Position:    model.Point{X: fb.posX, Y: fb.posY},
				Size:        model.Size{Width: ActivityWidth, Height: ActivityHeight},
			},
			ErrorHandlingType: microflows.ErrorHandlingTypeRollback,
			VariableName:      s.Variable,
		}
		fb.objects = append(fb.objects, split)
		fb.posX += fb.spacing
		return split.ID
	}
	return fb.addStructuredInheritanceSplit(s)
}

func (fb *flowBuilder) addStructuredInheritanceSplit(s *ast.InheritanceSplitStmt) model.ID {
	if fb.measurer == nil {
		fb.measurer = &layoutMeasurer{varTypes: fb.varTypes}
	}

	splitX := fb.posX
	centerY := fb.posY
	split := &microflows.InheritanceSplit{
		BaseMicroflowObject: microflows.BaseMicroflowObject{
			BaseElement: model.BaseElement{ID: model.ID(types.GenerateID())},
			Position:    model.Point{X: splitX, Y: centerY},
			Size:        model.Size{Width: ActivityWidth, Height: ActivityHeight},
		},
		ErrorHandlingType: microflows.ErrorHandlingTypeRollback,
		VariableName:      s.Variable,
	}
	fb.objects = append(fb.objects, split)
	splitID := split.ID
	if fb.pendingAnnotations != nil {
		fb.applyAnnotations(splitID, fb.pendingAnnotations)
		fb.pendingAnnotations = nil
	}

	branchWidth := fb.measurer.measureStatements(appendInheritanceBodies(s)).Width
	if branchWidth == 0 {
		branchWidth = HorizontalSpacing / 2
	}
	branchStartX := splitX + ActivityWidth + HorizontalSpacing/2
	mergeX := branchStartX + branchWidth + HorizontalSpacing/2

	type branchTail struct {
		id        model.ID
		caseValue string
		fromSplit bool
		order     int
		anchor    *ast.FlowAnchors
	}
	var branchTails []branchTail

	savedEndsWithReturn := fb.endsWithReturn
	allBranchesReturn := len(s.Cases) > 0 && len(s.ElseBody) > 0
	branchIndex := 0

	addBranch := func(caseValue string, body []ast.MicroflowStatement) {
		branchNumber := branchIndex
		branchY := centerY + branchIndex*VerticalSpacing
		branchIndex++
		if len(body) == 0 {
			allBranchesReturn = false
			branchTails = append(branchTails, branchTail{id: splitID, caseValue: caseValue, fromSplit: true, order: branchNumber})
			return
		}

		fb.posX = branchStartX
		fb.posY = branchY
		fb.endsWithReturn = false

		var lastID model.ID
		var prevAnchor *ast.FlowAnchors
		pendingCase := ""
		for _, stmt := range body {
			thisAnchor := stmtOwnAnchor(stmt)
			actID := fb.addStatement(stmt)
			if actID == "" {
				continue
			}
			if cast, ok := stmt.(*ast.CastObjectStmt); ok && cast.OutputVariable != "" && caseValue != "" && fb.varTypes != nil {
				fb.varTypes[cast.OutputVariable] = caseValue
			}
			if fb.pendingAnnotations != nil {
				fb.applyAnnotations(actID, fb.pendingAnnotations)
				fb.pendingAnnotations = nil
			}
			if lastID == "" {
				var flow *microflows.SequenceFlow
				if branchNumber == 0 {
					flow = newHorizontalFlowWithInheritanceCase(splitID, actID, caseValue)
				} else {
					flow = newDownwardFlowWithInheritanceCase(splitID, actID, caseValue)
				}
				applyUserAnchors(flow, nil, thisAnchor)
				fb.flows = append(fb.flows, flow)
			} else {
				if pendingCase != "" {
					flow := newHorizontalFlowWithCase(lastID, actID, pendingCase)
					applyUserAnchors(flow, prevAnchor, thisAnchor)
					fb.flows = append(fb.flows, flow)
					pendingCase = ""
				} else {
					flow := newHorizontalFlow(lastID, actID)
					applyUserAnchors(flow, prevAnchor, thisAnchor)
					fb.flows = append(fb.flows, flow)
				}
			}
			prevAnchor = thisAnchor
			if fb.nextConnectionPoint != "" {
				lastID = fb.nextConnectionPoint
				fb.nextConnectionPoint = ""
				pendingCase = fb.nextFlowCase
				fb.nextFlowCase = ""
			} else {
				lastID = actID
			}
		}

		if !lastStmtIsReturn(body) {
			allBranchesReturn = false
			if lastID != "" {
				branchTails = append(branchTails, branchTail{id: lastID, caseValue: pendingCase, anchor: prevAnchor})
			}
		}
	}

	for _, c := range s.Cases {
		addBranch(qualifiedNameString(c.Entity), c.Body)
	}
	addBranch("", s.ElseBody)

	fb.posX = mergeX
	fb.posY = centerY
	fb.endsWithReturn = savedEndsWithReturn
	if allBranchesReturn {
		fb.endsWithReturn = true
	} else if len(branchTails) > 0 {
		merge := &microflows.ExclusiveMerge{
			BaseMicroflowObject: microflows.BaseMicroflowObject{
				BaseElement: model.BaseElement{ID: model.ID(types.GenerateID())},
				Position:    model.Point{X: mergeX, Y: centerY},
				Size:        model.Size{Width: MergeSize, Height: MergeSize},
			},
		}
		fb.objects = append(fb.objects, merge)
		for _, tail := range branchTails {
			if tail.fromSplit {
				var flow *microflows.SequenceFlow
				if tail.order == 0 {
					flow = newHorizontalFlowWithInheritanceCase(splitID, merge.ID, tail.caseValue)
				} else {
					flow = newDownwardFlowWithInheritanceCase(splitID, merge.ID, tail.caseValue)
				}
				applyInheritanceSplitCaseOrder(flow, tail.order)
				fb.flows = append(fb.flows, flow)
			} else {
				if tail.caseValue != "" {
					flow := newHorizontalFlowWithCase(tail.id, merge.ID, tail.caseValue)
					applyUserAnchors(flow, tail.anchor, nil)
					fb.flows = append(fb.flows, flow)
				} else {
					flow := newHorizontalFlow(tail.id, merge.ID)
					applyUserAnchors(flow, tail.anchor, nil)
					fb.flows = append(fb.flows, flow)
				}
			}
		}
		fb.nextConnectionPoint = merge.ID
	}
	return splitID
}

func (fb *flowBuilder) addGroupedEnumSplitFlows(originID, destinationID model.ID, values []string, order int, mergeX, mergeY int) {
	if len(values) <= 1 {
		fb.addEnumSplitFlows(originID, destinationID, values, order)
		return
	}
	branchMerge := &microflows.ExclusiveMerge{
		BaseMicroflowObject: microflows.BaseMicroflowObject{
			BaseElement: model.BaseElement{ID: model.ID(types.GenerateID())},
			Position:    model.Point{X: mergeX, Y: mergeY},
			Size:        model.Size{Width: MergeSize, Height: MergeSize},
		},
	}
	fb.objects = append(fb.objects, branchMerge)
	fb.addEnumSplitFlows(originID, branchMerge.ID, values, order)
	fb.flows = append(fb.flows, newHorizontalFlow(branchMerge.ID, destinationID))
}

func (fb *flowBuilder) addEnumSplitFlows(originID, destinationID model.ID, values []string, order int) {
	if len(values) == 0 {
		flow := newHorizontalFlow(originID, destinationID)
		applySplitCaseOrder(flow, order)
		fb.flows = append(fb.flows, flow)
		return
	}
	for _, value := range values {
		flow := newHorizontalFlowWithEnumCase(originID, destinationID, value)
		applySplitCaseOrder(flow, order)
		fb.flows = append(fb.flows, flow)
	}
}

type splitCaseOrderAnchor struct {
	origin      int
	destination int
}

var splitCaseOrderAnchors = []splitCaseOrderAnchor{
	{AnchorTop, AnchorLeft},
	{AnchorRight, AnchorLeft},
	{AnchorBottom, AnchorLeft},
	{AnchorLeft, AnchorLeft},
	{AnchorTop, AnchorTop},
	{AnchorRight, AnchorTop},
	{AnchorBottom, AnchorTop},
	{AnchorLeft, AnchorTop},
	{AnchorTop, AnchorRight},
	{AnchorRight, AnchorRight},
	{AnchorBottom, AnchorRight},
	{AnchorLeft, AnchorRight},
	{AnchorTop, AnchorBottom},
	{AnchorRight, AnchorBottom},
	{AnchorBottom, AnchorBottom},
	{AnchorLeft, AnchorBottom},
}

var maxEnumSplitBranches = len(splitCaseOrderAnchors)

func applySplitCaseOrder(flow *microflows.SequenceFlow, order int) {
	if flow == nil || order < 0 || order >= len(splitCaseOrderAnchors) {
		return
	}
	pair := splitCaseOrderAnchors[order]
	flow.OriginConnectionIndex = pair.origin
	flow.DestinationConnectionIndex = pair.destination
}

func enumSplitCaseValues(c ast.EnumSplitCase) []string {
	if len(c.Values) > 0 {
		return append([]string(nil), c.Values...)
	}
	if c.Value != "" {
		return []string{c.Value}
	}
	return nil
}

func enumSplitBranchCount(s *ast.EnumSplitStmt) int {
	if s == nil {
		return 0
	}
	count := len(s.Cases)
	if len(s.ElseBody) > 0 {
		count++
	}
	return count
}

func appendEnumBodies(s *ast.EnumSplitStmt) []ast.MicroflowStatement {
	var stmts []ast.MicroflowStatement
	for _, c := range s.Cases {
		stmts = append(stmts, c.Body...)
	}
	stmts = append(stmts, s.ElseBody...)
	return stmts
}

func appendInheritanceBodies(s *ast.InheritanceSplitStmt) []ast.MicroflowStatement {
	var stmts []ast.MicroflowStatement
	for _, c := range s.Cases {
		stmts = append(stmts, c.Body...)
	}
	stmts = append(stmts, s.ElseBody...)
	return stmts
}

type inheritanceSplitCaseOrderAnchor struct {
	origin      int
	destination int
}

var inheritanceSplitCaseOrderAnchors = []inheritanceSplitCaseOrderAnchor{
	{AnchorTop, AnchorLeft},
	{AnchorRight, AnchorLeft},
	{AnchorBottom, AnchorLeft},
	{AnchorLeft, AnchorLeft},
	{AnchorTop, AnchorTop},
	{AnchorRight, AnchorTop},
	{AnchorBottom, AnchorTop},
	{AnchorLeft, AnchorTop},
	{AnchorTop, AnchorRight},
	{AnchorRight, AnchorRight},
	{AnchorBottom, AnchorRight},
	{AnchorLeft, AnchorRight},
	{AnchorTop, AnchorBottom},
	{AnchorRight, AnchorBottom},
	{AnchorBottom, AnchorBottom},
	{AnchorLeft, AnchorBottom},
}

func applyInheritanceSplitCaseOrder(flow *microflows.SequenceFlow, order int) {
	if flow == nil || order < 0 || order >= len(inheritanceSplitCaseOrderAnchors) {
		return
	}
	pair := inheritanceSplitCaseOrderAnchors[order]
	flow.OriginConnectionIndex = pair.origin
	flow.DestinationConnectionIndex = pair.destination
}

func qualifiedNameString(qn ast.QualifiedName) string {
	if qn.Module == "" {
		return qn.Name
	}
	return qn.Module + "." + qn.Name
}

func (fb *flowBuilder) addCastAction(s *ast.CastObjectStmt) model.ID {
	action := &microflows.CastAction{
		BaseElement:    model.BaseElement{ID: model.ID(types.GenerateID())},
		ObjectVariable: s.ObjectVariable,
		OutputVariable: s.OutputVariable,
	}

	activity := &microflows.ActionActivity{
		BaseActivity: microflows.BaseActivity{
			BaseMicroflowObject: microflows.BaseMicroflowObject{
				BaseElement: model.BaseElement{ID: model.ID(types.GenerateID())},
				Position:    model.Point{X: fb.posX, Y: fb.posY},
				Size:        model.Size{Width: ActivityWidth, Height: ActivityHeight},
			},
			AutoGenerateCaption: true,
		},
		Action: action,
	}

	fb.objects = append(fb.objects, activity)
	fb.posX += fb.spacing
	return activity.ID
}

// addRetrieveAction creates a RETRIEVE statement.
func (fb *flowBuilder) addRetrieveAction(s *ast.RetrieveStmt) model.ID {
	var source microflows.RetrieveSource

	if s.StartVariable != "" {
		// Association retrieve: RETRIEVE $List FROM $Parent/Module.AssocName
		// Always use AssociationRetrieveSource to preserve the original syntax.
		// The runtime resolves traversal direction from association metadata.
		assocQN := s.Source.Module + "." + s.Source.Name

		// Look up association to determine type and direction.
		// For Reference associations, AssociationRetrieveSource always returns a single
		// object (the entity on the other end). When the user navigates from the child
		// (non-owner) side, the intent is to get a list of parent entities — we must use
		// a DatabaseRetrieveSource with XPath constraint instead.
		assocInfo := fb.lookupAssociation(s.Source.Module, s.Source.Name)
		startVarType := ""
		if fb.varTypes != nil {
			startVarType = fb.varTypes[s.StartVariable]
		}

		outputUsedAsObject := fb.objectInputVariables != nil && fb.objectInputVariables[s.Variable]
		outputUsedAsList := fb.listInputVariables != nil && fb.listInputVariables[s.Variable]
		// startsFromChildSide is true when the retrieve's start variable is the
		// child side of the association (or a subclass of it). Inheritance has
		// to be honoured so traversals like `$httpRequest/System.HttpHeaders`
		// — where HttpRequest extends HttpMessage and HttpHeaders has child
		// HttpMessage — are still classified as reverse traversal (which returns
		// a list, so the output variable is typed accordingly below).
		startsFromChildSide := assocInfo != nil &&
			assocInfo.childEntityQN != "" &&
			fb.entityIsSubtypeOf(startVarType, assocInfo.childEntityQN)

		// The `$var/Module.Assoc` syntax is a retrieve-by-association (in-memory).
		// Keep it as an AssociationRetrieveSource — that is what preserves the
		// distinction between memory and database retrieves (issue #726), and for
		// a reverse Reference traversal (child → parent) Mendix returns a list and
		// mxbuild accepts it.
		//
		// The ONE exception is an owner="both" Reference consumed as a list: a
		// reverse AssociationRetrieveSource over such an association resolves to a
		// SINGLE object in Mendix (mxbuild reports CE0100 when it feeds a loop or
		// aggregate), so there is no in-memory way to obtain the list — fall back
		// to a DatabaseRetrieveSource. Object usage stays an association retrieve.
		expandReverseReference := assocInfo != nil &&
			assocInfo.Type == domainmodel.AssociationTypeReference &&
			assocInfo.Owner == domainmodel.AssociationOwnerBoth &&
			assocInfo.parentPersistable &&
			assocInfo.childEntityQN != "" &&
			startsFromChildSide &&
			outputUsedAsList && !outputUsedAsObject

		if expandReverseReference {
			source = &microflows.DatabaseRetrieveSource{
				BaseElement:         model.BaseElement{ID: model.ID(types.GenerateID())},
				EntityQualifiedName: assocInfo.parentEntityQN,
				XPathConstraint:     "[" + assocQN + " = $" + s.StartVariable + "]",
			}
			if fb.varTypes != nil {
				fb.varTypes[s.Variable] = "List of " + assocInfo.parentEntityQN
			}
		} else {
			source = &microflows.AssociationRetrieveSource{
				BaseElement:              model.BaseElement{ID: model.ID(types.GenerateID())},
				StartVariable:            s.StartVariable,
				AssociationQualifiedName: assocQN,
			}
			if fb.varTypes != nil {
				if assocInfo != nil && assocInfo.Type == domainmodel.AssociationTypeReference {
					// Reference: forward traversal (parent side) → single object;
					// reverse traversal (child side) → list of the other entity.
					otherEntity := assocInfo.childEntityQN
					if startsFromChildSide {
						otherEntity = assocInfo.parentEntityQN
					}
					if startsFromChildSide && !outputUsedAsObject {
						fb.varTypes[s.Variable] = "List of " + otherEntity
					} else {
						fb.varTypes[s.Variable] = otherEntity
					}
				} else if assocInfo != nil && assocInfo.Type == domainmodel.AssociationTypeReferenceSet {
					// ReferenceSet traversal returns a list of the entity on the other side,
					// not a list typed as the association itself.
					otherEntity := assocInfo.childEntityQN
					if startsFromChildSide {
						otherEntity = assocInfo.parentEntityQN
					}
					if otherEntity != "" {
						fb.varTypes[s.Variable] = "List of " + otherEntity
					} else {
						fb.varTypes[s.Variable] = "List of " + assocQN
					}
				} else {
					// ReferenceSet or unknown: returns a list
					fb.varTypes[s.Variable] = "List of " + assocQN
				}
			}
		}
	} else {
		// Database retrieve: RETRIEVE $List FROM Module.Entity WHERE ...
		entityQN := s.Source.Module + "." + s.Source.Name
		dbSource := &microflows.DatabaseRetrieveSource{
			BaseElement:         model.BaseElement{ID: model.ID(types.GenerateID())},
			EntityQualifiedName: entityQN,
		}

		// Set range if LIMIT is specified
		if s.Limit != "" {
			rangeType := microflows.RangeTypeCustom
			// LIMIT 1 with no offset uses RangeTypeFirst for single object retrieval
			if s.Limit == "1" && s.Offset == "" {
				rangeType = microflows.RangeTypeFirst
			}
			dbSource.Range = &microflows.Range{
				BaseElement: model.BaseElement{ID: model.ID(types.GenerateID())},
				RangeType:   rangeType,
				Limit:       s.Limit,
				Offset:      s.Offset,
			}
		}

		// Convert WHERE expression if present
		// XPath constraints are stored with square brackets in BSON: [expression]
		if s.Where != nil {
			dbSource.XPathConstraint = retrieveXPathConstraint(s.Where)
		}

		// Convert SORT BY columns if present
		if len(s.SortColumns) > 0 {
			for _, col := range s.SortColumns {
				// Resolve attribute path - if just a simple name, prefix with entity
				attrPath := col.Attribute
				var entityRefSteps []microflows.EntityRefStep
				if !strings.Contains(attrPath, ".") {
					attrPath = entityQN + "." + attrPath
				} else {
					// Validate that qualified attribute path belongs to the retrieved entity
					// Expected format: Module.Entity.Attribute
					parts := strings.Split(attrPath, ".")
					if len(parts) >= 3 {
						// Extract entity from attribute path (first two parts)
						attrEntityQN := parts[0] + "." + parts[1]
						if attrEntityQN != entityQN {
							entityRefSteps = fb.inferSortEntityRefSteps(entityQN, attrPath)
							if len(entityRefSteps) == 0 {
								fb.addError("sort by attribute '%s' does not belong to entity '%s'", col.Attribute, entityQN)
								continue // Skip this sort column but continue processing others
							}
						}
					}
				}

				direction := microflows.SortDirectionAscending
				if strings.EqualFold(col.Order, "desc") {
					direction = microflows.SortDirectionDescending
				}

				dbSource.Sorting = append(dbSource.Sorting, &microflows.SortItem{
					BaseElement:            model.BaseElement{ID: model.ID(types.GenerateID())},
					AttributeQualifiedName: attrPath,
					EntityRefSteps:         entityRefSteps,
					Direction:              direction,
				})
			}
		}

		source = dbSource

		// Register variable type for CHANGE statements
		// RETRIEVE with LIMIT 1 returns a single entity, otherwise returns a List
		if fb.varTypes != nil {
			if s.Limit == "1" {
				// LIMIT 1 returns a single entity
				fb.varTypes[s.Variable] = entityQN
			} else {
				// No LIMIT or LIMIT > 1 returns a list
				fb.varTypes[s.Variable] = "List of " + entityQN
			}
		}
	}

	action := &microflows.RetrieveAction{
		BaseElement:    model.BaseElement{ID: model.ID(types.GenerateID())},
		OutputVariable: s.Variable,
		Source:         source,
	}

	activityX := fb.posX
	activity := &microflows.ActionActivity{
		BaseActivity: microflows.BaseActivity{
			BaseMicroflowObject: microflows.BaseMicroflowObject{
				BaseElement: model.BaseElement{ID: model.ID(types.GenerateID())},
				Position:    model.Point{X: fb.posX, Y: fb.posY},
				Size:        model.Size{Width: ActivityWidth, Height: ActivityHeight},
			},
			AutoGenerateCaption: true,
			ErrorHandlingType:   fb.ehType(s.ErrorHandling),
		},
		Action: action,
	}

	fb.objects = append(fb.objects, activity)
	fb.posX += fb.spacing

	fb.finishCustomErrorHandler(activity.ID, activityX, s.ErrorHandling, s.Variable)

	return activity.ID
}

func retrieveXPathConstraint(expr ast.Expression) string {
	xpath := normalizeXPathEnumRefs(expressionToXPath(expr))
	if strings.HasPrefix(strings.TrimSpace(xpath), "[") && strings.HasSuffix(strings.TrimSpace(xpath), "]") {
		return strings.TrimSpace(xpath)
	}
	return "[" + xpath + "]"
}

func (fb *flowBuilder) inferSortEntityRefSteps(sourceEntityQN, attrPath string) []microflows.EntityRefStep {
	attrEntityQN := entityQualifiedNameFromAttribute(attrPath)
	if attrEntityQN == "" || attrEntityQN == sourceEntityQN {
		return nil
	}
	parts := strings.SplitN(sourceEntityQN, ".", 2)
	if len(parts) != 2 || parts[0] == "" {
		return nil
	}
	if fb.backend == nil {
		return nil
	}
	mod, err := fb.backend.GetModuleByName(parts[0])
	if err != nil || mod == nil {
		return nil
	}
	dm, err := fb.backend.GetDomainModel(mod.ID)
	if err != nil || dm == nil {
		return nil
	}
	entityNames := make(map[model.ID]string, len(dm.Entities))
	for _, e := range dm.Entities {
		entityNames[e.ID] = parts[0] + "." + e.Name
	}
	for _, assoc := range dm.Associations {
		parentQN := entityNames[assoc.ParentID]
		childQN := entityNames[assoc.ChildID]
		if parentQN == sourceEntityQN && childQN == attrEntityQN {
			return []microflows.EntityRefStep{{Association: parts[0] + "." + assoc.Name, DestinationEntity: childQN}}
		}
	}
	for _, assoc := range dm.CrossAssociations {
		parentQN := entityNames[assoc.ParentID]
		if parentQN == sourceEntityQN && assoc.ChildRef == attrEntityQN {
			return []microflows.EntityRefStep{{Association: parts[0] + "." + assoc.Name, DestinationEntity: assoc.ChildRef}}
		}
	}
	return nil
}

func entityQualifiedNameFromAttribute(attrPath string) string {
	parts := strings.Split(attrPath, ".")
	if len(parts) < 3 {
		return ""
	}
	return parts[0] + "." + parts[1]
}

// addListOperationAction creates list operations like HEAD, TAIL, FIND, etc.
func (fb *flowBuilder) addListOperationAction(s *ast.ListOperationStmt) model.ID {
	var operation microflows.ListOperation

	switch s.Operation {
	case ast.ListOpHead:
		operation = &microflows.HeadOperation{
			BaseElement:  model.BaseElement{ID: model.ID(types.GenerateID())},
			ListVariable: s.InputVariable,
		}
	case ast.ListOpTail:
		operation = &microflows.TailOperation{
			BaseElement:  model.BaseElement{ID: model.ID(types.GenerateID())},
			ListVariable: s.InputVariable,
		}
	case ast.ListOpFind:
		if op := fb.listAttributeOperation(s, false); op != nil {
			operation = op
		} else {
			operation = &microflows.FindOperation{
				BaseElement:  model.BaseElement{ID: model.ID(types.GenerateID())},
				ListVariable: s.InputVariable,
				Expression:   fb.exprToString(s.Condition),
			}
		}
	case ast.ListOpFilter:
		if op := fb.listAttributeOperation(s, true); op != nil {
			operation = op
		} else {
			operation = &microflows.FilterOperation{
				BaseElement:  model.BaseElement{ID: model.ID(types.GenerateID())},
				ListVariable: s.InputVariable,
				Expression:   fb.exprToString(s.Condition),
			}
		}
	case ast.ListOpSort:
		// Resolve entity type from input variable for qualified attribute names
		entityType := ""
		if fb.varTypes != nil {
			listType := fb.varTypes[s.InputVariable]
			if after, ok := strings.CutPrefix(listType, "List of "); ok {
				entityType = after
			}
		}

		// Build sort items from SortSpecs
		var sortItems []*microflows.SortItem
		for _, spec := range s.SortSpecs {
			direction := microflows.SortDirectionAscending
			if !spec.Ascending {
				direction = microflows.SortDirectionDescending
			}
			// Build fully qualified attribute name: Entity.Attribute
			attrQN := spec.Attribute
			if entityType != "" && !strings.Contains(spec.Attribute, ".") {
				attrQN = entityType + "." + spec.Attribute
			}
			sortItems = append(sortItems, &microflows.SortItem{
				BaseElement:            model.BaseElement{ID: model.ID(types.GenerateID())},
				AttributeQualifiedName: attrQN,
				Direction:              direction,
			})
		}
		operation = &microflows.SortOperation{
			BaseElement:  model.BaseElement{ID: model.ID(types.GenerateID())},
			ListVariable: s.InputVariable,
			Sorting:      sortItems,
		}
	case ast.ListOpUnion:
		operation = &microflows.UnionOperation{
			BaseElement:   model.BaseElement{ID: model.ID(types.GenerateID())},
			ListVariable1: s.InputVariable,
			ListVariable2: s.SecondVariable,
		}
	case ast.ListOpIntersect:
		operation = &microflows.IntersectOperation{
			BaseElement:   model.BaseElement{ID: model.ID(types.GenerateID())},
			ListVariable1: s.InputVariable,
			ListVariable2: s.SecondVariable,
		}
	case ast.ListOpSubtract:
		operation = &microflows.SubtractOperation{
			BaseElement:   model.BaseElement{ID: model.ID(types.GenerateID())},
			ListVariable1: s.InputVariable,
			ListVariable2: s.SecondVariable,
		}
	case ast.ListOpContains:
		operation = &microflows.ContainsOperation{
			BaseElement:    model.BaseElement{ID: model.ID(types.GenerateID())},
			ListVariable:   s.InputVariable,
			ObjectVariable: s.SecondVariable, // The item to check
		}
	case ast.ListOpEquals:
		operation = &microflows.EqualsOperation{
			BaseElement:   model.BaseElement{ID: model.ID(types.GenerateID())},
			ListVariable1: s.InputVariable,
			ListVariable2: s.SecondVariable,
		}
	case ast.ListOpRange:
		rangeOp := &microflows.ListRangeOperation{
			BaseElement:  model.BaseElement{ID: model.ID(types.GenerateID())},
			ListVariable: s.InputVariable,
		}
		if s.OffsetExpr != nil {
			rangeOp.OffsetExpression = fb.exprToString(s.OffsetExpr)
		}
		if s.LimitExpr != nil {
			rangeOp.LimitExpression = fb.exprToString(s.LimitExpr)
		}
		operation = rangeOp
	default:
		return ""
	}

	action := &microflows.ListOperationAction{
		BaseElement:    model.BaseElement{ID: model.ID(types.GenerateID())},
		Operation:      operation,
		OutputVariable: s.OutputVariable,
	}

	// Track output variable type for operations that preserve/produce list types
	if fb.varTypes != nil && s.OutputVariable != "" && s.InputVariable != "" {
		inputType := fb.varTypes[s.InputVariable]
		switch s.Operation {
		case ast.ListOpFilter, ast.ListOpSort, ast.ListOpTail, ast.ListOpUnion, ast.ListOpIntersect, ast.ListOpSubtract, ast.ListOpRange:
			// These operations preserve the list type
			if inputType != "" {
				fb.varTypes[s.OutputVariable] = inputType
			}
		case ast.ListOpHead, ast.ListOpFind:
			// These return a single element (remove "List of " prefix)
			if after, ok := strings.CutPrefix(inputType, "List of "); ok {
				fb.varTypes[s.OutputVariable] = after
			}
			// CONTAINS and EQUALS return Boolean, no need to track
		}
	}

	activity := &microflows.ActionActivity{
		BaseActivity: microflows.BaseActivity{
			BaseMicroflowObject: microflows.BaseMicroflowObject{
				BaseElement: model.BaseElement{ID: model.ID(types.GenerateID())},
				Position:    model.Point{X: fb.posX, Y: fb.posY},
				Size:        model.Size{Width: ActivityWidth, Height: ActivityHeight},
			},
			AutoGenerateCaption: true,
		},
		Action: action,
	}

	fb.objects = append(fb.objects, activity)
	fb.posX += fb.spacing
	return activity.ID
}

func (fb *flowBuilder) listAttributeOperation(s *ast.ListOperationStmt, filter bool) microflows.ListOperation {
	binary, ok := s.Condition.(*ast.BinaryExpr)
	if !ok || binary.Operator != "=" {
		return nil
	}
	fieldName, ok := listOperationFieldName(binary.Left)
	if !ok || fieldName == "" {
		return nil
	}
	expression := fb.exprToString(binary.Right)
	if expression == "" {
		return nil
	}

	attributeName, associationName := fb.resolveListOperationMember(s.InputVariable, fieldName)
	if associationName == "" && !strings.Contains(attributeName, ".") {
		return nil
	}
	if filter {
		return &microflows.FilterByAttributeOperation{
			BaseElement:  model.BaseElement{ID: model.ID(types.GenerateID())},
			ListVariable: s.InputVariable,
			Attribute:    attributeName,
			Association:  associationName,
			Expression:   expression,
		}
	}
	return &microflows.FindByAttributeOperation{
		BaseElement:  model.BaseElement{ID: model.ID(types.GenerateID())},
		ListVariable: s.InputVariable,
		Attribute:    attributeName,
		Association:  associationName,
		Expression:   expression,
	}
}

func listOperationFieldName(expr ast.Expression) (string, bool) {
	switch e := expr.(type) {
	case *ast.IdentifierExpr:
		return e.Name, true
	case *ast.QualifiedNameExpr:
		return e.QualifiedName.String(), true
	default:
		return "", false
	}
}

func (fb *flowBuilder) resolveListOperationMember(listVariable, memberName string) (attributeName, associationName string) {
	entityQN := ""
	if fb.varTypes != nil {
		if listType := fb.varTypes[listVariable]; strings.HasPrefix(listType, "List of ") {
			entityQN = strings.TrimPrefix(listType, "List of ")
		}
	}
	// Reuse the member-change resolver so list operations follow the same
	// attribute-vs-association qualification rules as change-object members.
	memberChange := &microflows.MemberChange{}
	fb.resolveMemberChange(memberChange, memberName, entityQN)
	return memberChange.AttributeQualifiedName, memberChange.AssociationQualifiedName
}

// addAggregateListAction creates aggregate operations like COUNT, SUM, AVERAGE, etc.
func (fb *flowBuilder) addAggregateListAction(s *ast.AggregateListStmt) model.ID {
	var function microflows.AggregateFunction
	switch s.Operation {
	case ast.AggregateCount:
		function = microflows.AggregateFunctionCount
	case ast.AggregateSum:
		function = microflows.AggregateFunctionSum
	case ast.AggregateAverage:
		function = microflows.AggregateFunctionAverage
	case ast.AggregateMinimum:
		function = microflows.AggregateFunctionMin
	case ast.AggregateMaximum:
		function = microflows.AggregateFunctionMax
	case ast.AggregateReduce:
		function = microflows.AggregateFunctionReduce
	default:
		return ""
	}

	action := &microflows.AggregateListAction{
		BaseElement:    model.BaseElement{ID: model.ID(types.GenerateID())},
		InputVariable:  s.InputVariable,
		OutputVariable: s.OutputVariable,
		Function:       function,
	}

	if s.IsExpression && s.Expression != nil {
		action.UseExpression = true
		action.Expression = expressionToString(s.Expression)
	} else if s.Attribute != "" {
		// For SUM/AVG/MIN/MAX, build qualified attribute name from variable type
		if fb.varTypes != nil {
			listType := fb.varTypes[s.InputVariable]
			if after, ok := strings.CutPrefix(listType, "List of "); ok {
				entityType := after
				action.AttributeQualifiedName = entityType + "." + s.Attribute
			}
		}
	}

	activity := &microflows.ActionActivity{
		BaseActivity: microflows.BaseActivity{
			BaseMicroflowObject: microflows.BaseMicroflowObject{
				BaseElement: model.BaseElement{ID: model.ID(types.GenerateID())},
				Position:    model.Point{X: fb.posX, Y: fb.posY},
				Size:        model.Size{Width: ActivityWidth, Height: ActivityHeight},
			},
			AutoGenerateCaption: true,
		},
		Action: action,
	}

	fb.objects = append(fb.objects, activity)
	fb.posX += fb.spacing
	return activity.ID
}

// addCreateListAction creates a CREATE LIST OF statement.
func (fb *flowBuilder) addCreateListAction(s *ast.CreateListStmt) model.ID {
	entityQN := ""
	if s.EntityType.Module != "" && s.EntityType.Name != "" {
		entityQN = s.EntityType.Module + "." + s.EntityType.Name
	}

	action := &microflows.CreateListAction{
		BaseElement:         model.BaseElement{ID: model.ID(types.GenerateID())},
		OutputVariable:      s.Variable,
		EntityQualifiedName: entityQN,
	}

	// Register variable type as list
	if fb.varTypes != nil && entityQN != "" {
		fb.varTypes[s.Variable] = "List of " + entityQN
	}

	activity := &microflows.ActionActivity{
		BaseActivity: microflows.BaseActivity{
			BaseMicroflowObject: microflows.BaseMicroflowObject{
				BaseElement: model.BaseElement{ID: model.ID(types.GenerateID())},
				Position:    model.Point{X: fb.posX, Y: fb.posY},
				Size:        model.Size{Width: ActivityWidth, Height: ActivityHeight},
			},
			AutoGenerateCaption: true,
		},
		Action: action,
	}

	fb.objects = append(fb.objects, activity)
	fb.posX += fb.spacing
	return activity.ID
}

// addAddToListAction creates an ADD TO list statement.
func (fb *flowBuilder) addAddToListAction(s *ast.AddToListStmt) model.ID {
	value := fb.exprToString(s.Value)
	if value == "" && s.Item != "" {
		value = "$" + s.Item
	}
	action := &microflows.ChangeListAction{
		BaseElement:    model.BaseElement{ID: model.ID(types.GenerateID())},
		Type:           microflows.ChangeListTypeAdd,
		ChangeVariable: s.List,
		Value:          value,
	}

	activity := &microflows.ActionActivity{
		BaseActivity: microflows.BaseActivity{
			BaseMicroflowObject: microflows.BaseMicroflowObject{
				BaseElement: model.BaseElement{ID: model.ID(types.GenerateID())},
				Position:    model.Point{X: fb.posX, Y: fb.posY},
				Size:        model.Size{Width: ActivityWidth, Height: ActivityHeight},
			},
			AutoGenerateCaption: true,
		},
		Action: action,
	}

	fb.objects = append(fb.objects, activity)
	fb.posX += fb.spacing
	return activity.ID
}

// addRemoveFromListAction creates a REMOVE FROM list statement.
func (fb *flowBuilder) addRemoveFromListAction(s *ast.RemoveFromListStmt) model.ID {
	action := &microflows.ChangeListAction{
		BaseElement:    model.BaseElement{ID: model.ID(types.GenerateID())},
		Type:           microflows.ChangeListTypeRemove,
		ChangeVariable: s.List,
		Value:          "$" + s.Item,
	}

	activity := &microflows.ActionActivity{
		BaseActivity: microflows.BaseActivity{
			BaseMicroflowObject: microflows.BaseMicroflowObject{
				BaseElement: model.BaseElement{ID: model.ID(types.GenerateID())},
				Position:    model.Point{X: fb.posX, Y: fb.posY},
				Size:        model.Size{Width: ActivityWidth, Height: ActivityHeight},
			},
			AutoGenerateCaption: true,
		},
		Action: action,
	}

	fb.objects = append(fb.objects, activity)
	fb.posX += fb.spacing
	return activity.ID
}

// isEntity checks whether a qualified name refers to an entity in the domain model.
func (fb *flowBuilder) isEntity(moduleName, entityName string) bool {
	if fb.backend == nil {
		return false
	}
	mod, err := fb.backend.GetModuleByName(moduleName)
	if err != nil || mod == nil {
		return false
	}
	dm, err := fb.backend.GetDomainModel(mod.ID)
	if err != nil || dm == nil {
		return false
	}
	for _, e := range dm.Entities {
		if e.Name == entityName {
			return true
		}
	}
	return false
}

// resolveMemberChange determines whether a member name is an association or attribute
// and sets the appropriate field on the MemberChange. It queries the domain model
// to check if the name matches an association on the entity; if no metadata is
// available, it falls back to a name-shape heuristic.
//
// memberName can be either bare ("Order_Customer") or qualified ("MfTest.Order_Customer").
// isValidMemberIdentifier reports whether name is a plain member identifier or a
// dot-qualified name (e.g. "Resource", "SkillProfile_Resource", or
// "BuildScheduling.SkillProfile_Resource"). Each dot-separated segment must be a
// valid Mendix identifier: a letter or underscore followed by letters, digits, or
// underscores. Quotes, spaces, and empty segments are rejected.
func isValidMemberIdentifier(name string) bool {
	if name == "" {
		return false
	}
	for _, seg := range strings.Split(name, ".") {
		if seg == "" {
			return false
		}
		for i, r := range seg {
			ok := r == '_' ||
				(r >= 'a' && r <= 'z') ||
				(r >= 'A' && r <= 'Z') ||
				(i > 0 && r >= '0' && r <= '9')
			if !ok {
				return false
			}
		}
	}
	return true
}

func (fb *flowBuilder) resolveMemberChange(mc *microflows.MemberChange, memberName string, entityQN string) {
	// Guard against a malformed member identifier reaching the writer, where it
	// would serialize as an invalid Attribute/Association value that passes
	// `mxcli check` but fails to load in MxBuild/Studio Pro (StorageLoadException:
	// "... is not a valid AttributeIdentifier"). The visitor normalizes quoted
	// members, so this only fires on genuinely invalid input. Empty names are
	// left to the existing graceful handling below.
	if memberName != "" && !isValidMemberIdentifier(memberName) {
		fb.addError("invalid member name %q — a Change/Set member must be a plain identifier or Module.Name; remove quotes and other illegal characters", memberName)
		return
	}
	if entityQN == "" {
		// Entity type of $variable is unknown (e.g., the variable comes from a
		// java action whose return type isn't registered, or from the iterator
		// of an untyped loop). Without the entity we cannot query the domain
		// model — but we must NOT silently drop the member name, otherwise
		// `change $x (Module.Assoc = $y)` would round-trip as `change $x ( = $y)`
		// which is invalid MDL. Fall back to a shape heuristic:
		//
		//   * no dot                 -> bare attribute name
		//   * exactly one dot        -> `Module.Assoc` (association)
		//   * two or more dots       -> `Module.Entity.Attribute` (qualified attribute)
		//
		// Two-dot names are never associations in MDL (association names carry a
		// single qualifier — the module), so they must stay on AttributeQualified-
		// Name even when the entity type is unknown. This avoids miscategorising
		// something like `change $x (MyModule.MyEntity.Offset = 1)` as an
		// association change.
		resolveMemberChangeFallback(mc, memberName, "")
		return
	}

	// Split entity qualified name into module and entity
	parts := strings.SplitN(entityQN, ".", 2)
	if len(parts) != 2 {
		mc.AttributeQualifiedName = entityQN + "." + memberName
		return
	}
	moduleName := parts[0]

	// If memberName is already qualified (e.g., "Module.Assoc"), the qualifier
	// is the module that OWNS the association, not the create/change target's
	// module. Associations can live in any module (see cross-association
	// lookups below), so prefer the authored module when present.
	bareName := memberName
	qualifiedName := memberName
	lookupModule := moduleName
	if dot := strings.Index(memberName, "."); dot >= 0 {
		lookupModule = memberName[:dot]
		bareName = memberName[dot+1:]
		// qualifiedName is already set to the full memberName
	} else {
		qualifiedName = moduleName + "." + memberName
	}

	// Query the authored (or target) module's domain model first. When the
	// association actually lives in a different module — the common case for
	// cross-module associations like `OtherModule.Assoc_Name` on a
	// `TargetModule.Entity` entity — keep the qualified name so the writer
	// serialises `Association` correctly instead of falling back to
	// `Attribute` and triggering Studio Pro CE1613 on re-open.
	if fb.backend != nil {
		if mod, err := fb.backend.GetModuleByName(lookupModule); err == nil && mod != nil {
			if dm, err := fb.backend.GetDomainModel(mod.ID); err == nil && dm != nil {
				for _, a := range dm.Associations {
					if a.Name == bareName {
						mc.AssociationQualifiedName = qualifiedName
						return
					}
				}
				for _, a := range dm.CrossAssociations {
					if a.Name == bareName {
						mc.AssociationQualifiedName = qualifiedName
						return
					}
				}
				// Not an association in the authored module — if the author
				// qualified it (e.g. `Module.Attr`) the qualification is an
				// error we must preserve rather than silently dropping; the
				// writer will surface it during mx check.
				if strings.Contains(memberName, ".") {
					mc.AttributeQualifiedName = memberName
				} else if attrQN, ok := fb.resolveAttributeInEntityHierarchy(entityQN, memberName); ok {
					mc.AttributeQualifiedName = attrQN
				} else {
					mc.AttributeQualifiedName = entityQN + "." + memberName
				}
				return
			}
		}
	}

	resolveMemberChangeFallback(mc, memberName, entityQN)
}

func (fb *flowBuilder) resolveAttributeInEntityHierarchy(entityQN, attrName string) (string, bool) {
	if fb == nil || fb.backend == nil || entityQN == "" || attrName == "" {
		return "", false
	}
	seen := make(map[string]bool)
	for currentQN := entityQN; currentQN != ""; {
		if seen[currentQN] {
			return "", false
		}
		seen[currentQN] = true

		parts := strings.SplitN(currentQN, ".", 2)
		if len(parts) != 2 {
			return "", false
		}
		mod, err := fb.backend.GetModuleByName(parts[0])
		if err != nil || mod == nil {
			return "", false
		}
		dm, err := fb.backend.GetDomainModel(mod.ID)
		if err != nil || dm == nil {
			return "", false
		}
		entity := dm.FindEntityByName(parts[1])
		if entity == nil {
			return "", false
		}
		for _, attr := range entity.Attributes {
			if attr != nil && attr.Name == attrName {
				return currentQN + "." + attrName, true
			}
		}
		currentQN = entity.GeneralizationRef
	}
	return "", false
}

// entityIsSubtypeOf reports whether candidateQN is the same as ancestorQN or
// inherits from it through the generalization chain. The walk consults the
// domain model the same way resolveAttributeInEntityHierarchy does.
func (fb *flowBuilder) entityIsSubtypeOf(candidateQN, ancestorQN string) bool {
	if candidateQN == "" || ancestorQN == "" {
		return false
	}
	if candidateQN == ancestorQN {
		return true
	}
	if fb == nil || fb.backend == nil {
		return false
	}
	seen := make(map[string]bool)
	for currentQN := candidateQN; currentQN != ""; {
		if seen[currentQN] {
			return false
		}
		seen[currentQN] = true
		if currentQN == ancestorQN {
			return true
		}
		parts := strings.SplitN(currentQN, ".", 2)
		if len(parts) != 2 {
			return false
		}
		mod, err := fb.backend.GetModuleByName(parts[0])
		if err != nil || mod == nil {
			return false
		}
		dm, err := fb.backend.GetDomainModel(mod.ID)
		if err != nil || dm == nil {
			return false
		}
		entity := dm.FindEntityByName(parts[1])
		if entity == nil {
			return false
		}
		currentQN = entity.GeneralizationRef
	}
	return false
}

// resolveMemberChangeFallback preserves the authored member name shape when the
// entity metadata is unavailable.
//
//   - 0 dots  => bare attribute name. If entityQN is known, qualify it as
//     `Module.Entity.Attribute`; otherwise preserve the bare attribute.
//   - 1 dot   => association qualified by module (`Module.Association`).
//   - >=2 dots => fully qualified attribute (`Module.Entity.Attribute`).
func resolveMemberChangeFallback(mc *microflows.MemberChange, memberName string, entityQN string) {
	if memberName == "" {
		return
	}
	switch strings.Count(memberName, ".") {
	case 0:
		if entityQN == "" {
			mc.AttributeQualifiedName = memberName
		} else {
			mc.AttributeQualifiedName = entityQN + "." + memberName
		}
	case 1:
		mc.AssociationQualifiedName = memberName
	default:
		mc.AttributeQualifiedName = memberName
	}
}

// assocLookupResult holds resolved association metadata.
type assocLookupResult struct {
	Type              domainmodel.AssociationType
	Owner             domainmodel.AssociationOwner
	parentEntityQN    string // Qualified name of the parent (FROM/owner) entity
	childEntityQN     string // Qualified name of the child (TO/referenced) entity
	parentPersistable bool
	childPersistable  bool
}

// lookupAssociation finds an association by module and name, returning its type
// and the qualified names of its parent and child entities. Returns nil if the
// association cannot be found (e.g., backend is nil or module doesn't exist).
func (fb *flowBuilder) lookupAssociation(moduleName, assocName string) *assocLookupResult {
	if fb.backend == nil {
		return nil
	}
	mod, err := fb.backend.GetModuleByName(moduleName)
	if err != nil || mod == nil {
		return nil
	}
	dm, err := fb.backend.GetDomainModel(mod.ID)
	if err != nil || dm == nil {
		return nil
	}

	// Build entity ID → qualified name map
	entityNames := make(map[model.ID]string, len(dm.Entities))
	entityPersistable := make(map[model.ID]bool, len(dm.Entities))
	for _, e := range dm.Entities {
		entityNames[e.ID] = moduleName + "." + e.Name
		entityPersistable[e.ID] = e.Persistable
	}

	for _, a := range dm.Associations {
		if a.Name == assocName {
			return &assocLookupResult{
				Type:              a.Type,
				Owner:             a.Owner,
				parentEntityQN:    entityNames[a.ParentID],
				childEntityQN:     entityNames[a.ChildID],
				parentPersistable: entityPersistable[a.ParentID],
				childPersistable:  entityPersistable[a.ChildID],
			}
		}
	}
	return nil
}
