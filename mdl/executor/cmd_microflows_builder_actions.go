// SPDX-License-Identifier: Apache-2.0

// Package executor - Microflow builder: CRUD & data actions
package executor

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
	"github.com/mendixlabs/mxcli/sdk/microflows"
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

	activityX := fb.posX

	action := &microflows.CreateVariableAction{
		BaseElement: model.BaseElement{ID: model.ID(types.GenerateID())},
		// fb.ehType, not explicitErrorHandling — see ehType's doc comment.
		ErrorHandlingType: fb.ehType(s.ErrorHandling),
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
			ErrorHandlingType:   fb.ehType(s.ErrorHandling),
		},
		Action: action,
	}

	fb.objects = append(fb.objects, activity)
	fb.posX += fb.spacing

	fb.finishCustomErrorHandler(activity.ID, activityX, s.ErrorHandling, s.Variable)

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

	activityX := fb.posX

	action := &microflows.ChangeVariableAction{
		BaseElement: model.BaseElement{ID: model.ID(types.GenerateID())},
		// fb.ehType, not explicitErrorHandling — see ehType's doc comment.
		ErrorHandlingType: fb.ehType(s.ErrorHandling),
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
			ErrorHandlingType:   fb.ehType(s.ErrorHandling),
		},
		Action: action,
	}

	fb.objects = append(fb.objects, activity)
	fb.posX += fb.spacing

	fb.finishCustomErrorHandler(activity.ID, activityX, s.ErrorHandling, s.Target)

	return activity.ID
}

// commitTypeOf maps the AST's Commit modifier onto the stored Mendix enum. Both
// create and change previously hardcoded CommitTypeNo, so any project authored or
// round-tripped through MDL had its commit flags silently cleared (#779).
func commitTypeOf(f ast.CommitFlag) microflows.CommitType {
	switch f {
	case ast.CommitYes:
		return microflows.CommitTypeYes
	case ast.CommitYesWithoutEvents:
		return microflows.CommitTypeYesWithoutEvents
	default:
		return microflows.CommitTypeNo
	}
}

// addCreateObjectAction creates a CREATE OBJECT statement.
func (fb *flowBuilder) addCreateObjectAction(s *ast.CreateObjectStmt) model.ID {
	// Set entity reference as qualified name (BY_NAME_REFERENCE)
	entityQN := ""
	if s.EntityType.Module != "" && s.EntityType.Name != "" {
		entityQN = s.EntityType.Module + "." + s.EntityType.Name
	}

	// A Mendix Create activity ALWAYS names its output, and Studio Pro supplies
	// the name itself when you drop one. mxcli wrote an empty VariableName for
	// `CREATE Mod.Thing (…);` with no `$Var =`, and mxbuild rejects that as
	// CE6005 "The 'Entity' property is required." — an error that names the wrong
	// property, since Entity IS stored. Measured on 11.14.0: the assigned and
	// unassigned documents differ in exactly one field, and setting that one
	// field takes the project from 1 error to 0 (ako/CapTrackV3 FINDINGS §9).
	//
	// Supplied rather than refused, because nothing is being guessed: the author
	// said create this entity and did not ask for a handle, which is precisely
	// what an auto-named output is. Studio Pro's own convention is New<Entity>.
	outputVar := s.Variable
	if outputVar == "" {
		outputVar = fb.freshCreateVariable(s.EntityType.Name)
		if fb.generatedVars == nil {
			fb.generatedVars = map[string]bool{}
		}
		fb.generatedVars[outputVar] = true
	}

	action := &microflows.CreateObjectAction{
		BaseElement:       model.BaseElement{ID: model.ID(types.GenerateID())},
		ErrorHandlingType: fb.ehType(s.ErrorHandling),
		OutputVariable:    outputVar,
		Commit:            commitTypeOf(s.Commit),
		RefreshInClient:   s.RefreshInClient,
	}
	if entityQN != "" {
		action.EntityQualifiedName = entityQN
	}

	// Register variable type for CHANGE statements. The AUTHOR's name is
	// registered, never the generated one: an auto-named output is deliberately
	// not referenceable, so a later `change $NewThing` stays the error it is.
	if fb.varTypes != nil && entityQN != "" && s.Variable != "" {
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

// freshCreateVariable returns the output-variable name for a CREATE the script
// did not assign, following Studio Pro's New<Entity> convention.
//
// It must not collide with a name already in the flow: two activities sharing an
// output variable is CE0119, so a bare "New"+entity would turn one silent defect
// into another the moment a microflow creates two of the same entity. The suffix
// walks up until the name is free, and the search is bounded by the number of
// variables that exist, so it always terminates.
func (fb *flowBuilder) freshCreateVariable(entityName string) string {
	base := "New" + entityName
	if base == "New" {
		base = "NewObject"
	}
	if !fb.variableNameTaken(base) {
		return base
	}
	for i := 2; ; i++ {
		candidate := base + strconv.Itoa(i)
		if !fb.variableNameTaken(candidate) {
			return candidate
		}
	}
}

// variableNameTaken reports whether a name is already used in this flow. varTypes
// carries the ones the script named; generatedVars carries the ones this function
// has handed out, which varTypes deliberately does not record.
func (fb *flowBuilder) variableNameTaken(name string) bool {
	if fb.varTypes != nil {
		if _, ok := fb.varTypes[name]; ok {
			return true
		}
	}
	return fb.generatedVars[name]
}

// addCommitAction creates a COMMIT statement.
func (fb *flowBuilder) addCommitAction(s *ast.MfCommitStmt) model.ID {
	action := &microflows.CommitObjectsAction{
		BaseElement:       model.BaseElement{ID: model.ID(types.GenerateID())},
		ErrorHandlingType: fb.ehType(s.ErrorHandling),
		CommitVariable:    s.Variable,
		// Absent WITHOUT EVENTS means events ON — Mendix's default (#895).
		WithEvents:      !s.WithoutEvents,
		RefreshInClient: s.RefreshInClient,
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
		BaseElement:       model.BaseElement{ID: model.ID(types.GenerateID())},
		ErrorHandlingType: explicitErrorHandling(fb, s.ErrorHandling),
		DeleteVariable:    s.Variable,
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
	activityX := fb.posX

	action := &microflows.ChangeObjectAction{
		BaseElement: model.BaseElement{ID: model.ID(types.GenerateID())},
		// fb.ehType, not explicitErrorHandling — see ehType's doc comment.
		ErrorHandlingType: fb.ehType(s.ErrorHandling),
		ChangeVariable:    s.Variable,
		Commit:            commitTypeOf(s.Commit),
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
			ErrorHandlingType:   fb.ehType(s.ErrorHandling),
		},
		Action: action,
	}

	fb.objects = append(fb.objects, activity)
	fb.posX += fb.spacing

	fb.finishCustomErrorHandler(activity.ID, activityX, s.ErrorHandling, s.Variable)

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
		w := fb.measurer.measureBranch(br.body).Width
		if w > branchWidth {
			branchWidth = w
		}
	}
	if branchWidth == 0 {
		branchWidth = HorizontalSpacing / 2
	}
	mergeX := splitX + SplitWidth + HorizontalSpacing/2 + branchWidth
	mergeX, mergeY := mergePosition(s.Annotations, mergeX, centerY)
	var merge *microflows.ExclusiveMerge
	ensureMerge := func() *microflows.ExclusiveMerge {
		if merge == nil {
			merge = &microflows.ExclusiveMerge{
				BaseMicroflowObject: microflows.BaseMicroflowObject{
					BaseElement: model.BaseElement{ID: model.ID(types.GenerateID())},
					Position:    model.Point{X: mergeX, Y: mergeY},
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

	// The branch above's own objects, so a lane can be placed clear of what it really
	// occupies. Bounded at both ends: the merge that closes the split is appended
	// while the branches are being built, and it sits on the centre line, so an open
	// range would measure it as part of whichever branch came before it.
	prevBranchStart, prevBranchEnd := 0, 0
	origins := enumSplitOriginAnchors(branchYs, centerY)
	slot := func(i int) splitCaseSlot {
		if origins == nil {
			return splitCaseSlot{order: i, origin: -1}
		}
		return splitCaseSlot{order: i, origin: origins[i]}
	}

	savedEndsWithReturn := fb.endsWithReturn
	allBranchesReturn := len(branches) > 0
	for i, br := range branches {
		if i > 0 {
			// Clear of what the branch above actually occupies, not of half its
			// measured height: a branch holding a nested IF hangs below its own line.
			fallback := branchYs[i-1] + ActivityHeight/2
			if below := fb.lowestBetween(prevBranchStart, prevBranchEnd, fallback) + BranchGap + branchHeights[i]/2; below > branchYs[i] {
				branchYs[i] = below
			}
		}
		prevBranchStart = len(fb.objects)
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
			if fb.pendingJoin != nil {
				// `join L` as the FIRST statement of a case body means the split
				// itself goes to the merge, so the case flows are emitted with the
				// merge as destination rather than an activity.
				if lastID == "" {
					label := fb.pendingJoin.Label
					fb.pendingJoin = nil
					fb.labels().handled++
					m := fb.mergeForLabel(label)
					fb.addGroupedEnumSplitFlows(splitID, m.ID, br.values, slot(i), splitX+SplitWidth+HorizontalSpacing/4, branchY)
				} else {
					fb.takePendingJoin(lastID, pendingCase, prevAnchor)
				}
				pendingCase = ""
				continue
			}
			if actID == "" {
				continue
			}
			if fb.pendingAnnotations != nil {
				fb.applyAnnotations(actID, fb.pendingAnnotations)
				fb.pendingAnnotations = nil
			}
			if lastID == "" {
				fb.addGroupedEnumSplitFlows(splitID, actID, br.values, slot(i), splitX+SplitWidth+HorizontalSpacing/4, branchY)
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
		prevBranchEnd = len(fb.objects)

		if lastStmtIsReturn(br.body) {
			continue
		}
		allBranchesReturn = false
		if lastID == "" {
			fb.addGroupedEnumSplitFlows(splitID, ensureMerge().ID, br.values, slot(i), splitX+SplitWidth+HorizontalSpacing/4, branchY)
		} else {
			var tail *microflows.SequenceFlow
			if pendingCase != "" {
				tail = newHorizontalFlowWithCase(lastID, ensureMerge().ID, pendingCase)
			} else {
				tail = newHorizontalFlow(lastID, ensureMerge().ID)
			}
			if origins != nil {
				tail.DestinationConnectionIndex = mergeSideFor(fb.objectPosition(lastID), fb.objectPosition(ensureMerge().ID))
			}
			fb.flows = append(fb.flows, tail)
		}
	}

	fb.posX = mergeX + MergeSize + HorizontalSpacing/2
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

	// The branches are laid out one BELOW another, all starting at branchStartX,
	// so the room they need is the WIDEST branch — not the total. This used to
	// measure every body concatenated into one list, which made the merge slide
	// right by a whole activity-plus-spacing per extra branch and left it far
	// past the branches it joins (mendixlabs/mxcli#953: three one-activity
	// branches starting at x=720 put the merge at x=1480 instead of x=920).
	// `layout.go`'s measureInheritanceSplitStatement always took the max, so the
	// builder disagreed with its own measurer.
	branchWidth := 0
	for _, body := range inheritanceBranchBodies(s) {
		if w := fb.measurer.measureStatements(body).Width; w > branchWidth {
			branchWidth = w
		}
	}
	if branchWidth == 0 {
		branchWidth = HorizontalSpacing / 2
	}
	branchStartX := splitX + ActivityWidth + HorizontalSpacing/2
	mergeX := branchStartX + branchWidth + HorizontalSpacing/2
	mergeX, mergeY := mergePosition(s.Annotations, mergeX, centerY)

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
			if fb.takeBranchJoin(lastID, splitID, caseValue, pendingCase, prevAnchor, nil) {
				pendingCase = ""
				continue
			}
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
	// The empty-entity branch is NOT optional, and is not a "default" case: on an
	// object-type decision it is the `(empty)` flow, for a null object. Dropping
	// it when no `else` is written fails the build with
	//
	//	CE0089 "The '(empty)' value should be configured for an outgoing flow."
	//
	// (verified on mxbuild 11.6.6). So it is emitted unconditionally, and MDL's
	// `else` on an inheritance split IS that `(empty)` case — which is also why
	// an `else` cannot substitute for the base entity's own case (CE0090): the
	// two cover different things.
	//
	// DESCRIBE suppresses this flow when its body is empty, so a describe→exec
	// roundtrip does not accumulate `else` blocks; exec re-creates it here.
	addBranch("", s.ElseBody)

	// Where the next element goes. This was `mergeX`, i.e. the merge's own centre,
	// so whatever followed `end split` was drawn ON TOP of the merge, joined by a
	// zero-length sequence flow (#953). The model is valid either way, so nothing
	// below this line — not `mx check`, not the build — can notice.
	//
	// The spacing constants are centre-to-centre and tuned for a 40px edge gap, so
	// clearing a 40-wide merge before a 120-wide activity needs MergeSize + half a
	// pitch. That is what addIfStatement uses. addEnumSplit uses only
	// HorizontalSpacing/2, which leaves an activity's left edge exactly touching
	// the merge (measured: merge 890, activity 970, both edges at 910) — legible,
	// but not the house gap, and not worth re-laying out every existing enum split
	// to change.
	fb.posX = mergeX + MergeSize + HorizontalSpacing/2
	fb.posY = centerY
	fb.endsWithReturn = savedEndsWithReturn
	if allBranchesReturn {
		fb.endsWithReturn = true
	} else if len(branchTails) > 0 {
		merge := &microflows.ExclusiveMerge{
			BaseMicroflowObject: microflows.BaseMicroflowObject{
				BaseElement: model.BaseElement{ID: model.ID(types.GenerateID())},
				Position:    model.Point{X: mergeX, Y: mergeY},
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

func (fb *flowBuilder) addGroupedEnumSplitFlows(originID, destinationID model.ID, values []string, slot splitCaseSlot, mergeX, mergeY int) {
	if len(values) <= 1 {
		fb.addEnumSplitFlows(originID, destinationID, values, slot)
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
	fb.addEnumSplitFlows(originID, branchMerge.ID, values, slot)
	fb.flows = append(fb.flows, newHorizontalFlow(branchMerge.ID, destinationID))
}

func (fb *flowBuilder) addEnumSplitFlows(originID, destinationID model.ID, values []string, slot splitCaseSlot) {
	split, target := fb.objectPosition(originID), fb.objectPosition(destinationID)
	if len(values) == 0 {
		flow := newHorizontalFlow(originID, destinationID)
		slot.apply(flow, split, target)
		fb.flows = append(fb.flows, flow)
		return
	}
	for _, value := range values {
		flow := newHorizontalFlowWithEnumCase(originID, destinationID, value)
		slot.apply(flow, split, target)
		fb.flows = append(fb.flows, flow)
	}
}

// splitCaseSlot says how one case's flow leaves the split. origin is the side of
// the split it leaves from, or -1 for a split of up to three cases, which keeps the
// pair table below (top, right, bottom — the same sides, as it happens).
type splitCaseSlot struct {
	order  int
	origin int
}

// apply sets the flow's sides. split and target are where the two ends actually are,
// which is not where the stack was planned once a branch carries @position: a line is
// only sent out of the top corner towards something above the split, and out of the
// bottom towards something below, whatever third the case was counted into.
func (slot splitCaseSlot) apply(flow *microflows.SequenceFlow, split, target model.Point) {
	if flow == nil {
		return
	}
	if slot.origin < 0 {
		applySplitCaseOrder(flow, slot.order)
		return
	}
	origin := slot.origin
	if origin == AnchorTop && target.Y >= split.Y || origin == AnchorBottom && target.Y <= split.Y {
		origin = AnchorRight
	}
	flow.OriginConnectionIndex = origin
	flow.DestinationConnectionIndex = AnchorLeft
}

// objectPosition returns where an already-built object stands.
func (fb *flowBuilder) objectPosition(id model.ID) model.Point {
	for i := len(fb.objects) - 1; i >= 0; i-- {
		if o := fb.objects[i]; o != nil && o.GetID() == id {
			return o.GetPosition()
		}
	}
	return model.Point{}
}

// enumSplitOriginAnchors picks the side of the split each case leaves from, for a
// split of four or more cases: the upper third from the top corner, the middle
// third from the right, the lower third from the bottom. It returns nil for three
// cases or fewer, which the pair table already draws that way.
//
// The pair table was written to store the case ORDER (see splitCaseOrder), and past
// the third case it does so with sides no drawing would choose: the fourth case
// leaves the split's LEFT corner, the fifth to eighth arrive on top of their
// activity, the ninth onwards on its far side. Seven cases drawn that way cross each
// other and the activities they pass. Grouped, no two lines cross: the branches are
// stacked top to bottom in case order, so a line from the top corner only ever goes
// up and one from the bottom only down.
//
// A third is by count, the remainder going to the middle (7 cases: 2/3/2) or, when
// it is two, one each to top and bottom (8 cases: 3/2/3). A branch is only given the
// top corner if it really is above the split, and the bottom only if below, so a
// stack made lopsided by one tall branch never gets a line that leaves upwards to
// reach something underneath.
func enumSplitOriginAnchors(branchYs []int, centerY int) []int {
	n := len(branchYs)
	if n <= 3 {
		return nil
	}
	top, bottom := n/3, n/3
	if n%3 == 2 {
		top++
		bottom++
	}
	origins := make([]int, n)
	for i, y := range branchYs {
		switch {
		case i < top && y < centerY:
			origins[i] = AnchorTop
		case i >= n-bottom && y > centerY:
			origins[i] = AnchorBottom
		default:
			origins[i] = AnchorRight
		}
	}
	return origins
}

// mergeSideFor is the side of the closing merge a branch arrives on: an upper branch
// comes down onto its top corner and a lower one up onto its bottom, instead of all of
// them converging on the left corner. It goes by where the branch's last element
// actually stands, so a branch moved with @position still arrives from its own side.
func mergeSideFor(last, merge model.Point) int {
	switch {
	case last.Y < merge.Y:
		return AnchorTop
	case last.Y > merge.Y:
		return AnchorBottom
	default:
		return AnchorLeft
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

// inheritanceBranchBodies returns each branch of a type split as its own body,
// including the `(empty)` branch.
//
// It deliberately does NOT flatten them into one list. The predecessor did, and
// every caller then measured the branches as if they ran end to end when they
// are in fact stacked vertically (#953).
func inheritanceBranchBodies(s *ast.InheritanceSplitStmt) [][]ast.MicroflowStatement {
	bodies := make([][]ast.MicroflowStatement, 0, len(s.Cases)+1)
	for _, c := range s.Cases {
		bodies = append(bodies, c.Body)
	}
	return append(bodies, s.ElseBody)
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
				if len(col.Associations) > 0 {
					// The script SPELLS the hops. Take them as written rather than
					// inferring: inference cannot tell two associations reaching the
					// same entity apart, and picking the wrong one is a model that
					// builds cleanly and sorts by the wrong thing
					// (mendixlabs/mxcli#1152).
					resolved, finalQN, err := fb.resolveSortAssociationPath(entityQN, col.Associations, col.Attribute)
					if err != nil {
						fb.addError("sort by %s: %s", sortColumnText(col), err.Error())
						continue // Skip this sort column but continue processing others
					}
					entityRefSteps, attrPath = resolved, finalQN
				} else if !strings.Contains(attrPath, ".") {
					// Qualify with the entity that DECLARES the attribute, which is
					// not always the one being retrieved. Mendix resolves a sort
					// reference against the declaring entity, so qualifying an
					// INHERITED name with the list's own entity produced CE1613 "The
					// selected attribute … no longer exists" — after `mxcli check`
					// and `exec` both passed (CapTrackV2 §13). Reading the same
					// attribute already worked, because the page builder walks the
					// chain; this path did not, though flowBuilder has carried
					// resolveAttributeInEntityHierarchy all along.
					//
					// Falling back to the plain qualification keeps the behaviour
					// for a unit test with no backend, and for an attribute the
					// model does not know about — which is the checker's business to
					// report, not this function's to guess at.
					if declared, ok := fb.resolveAttributeInEntityHierarchy(entityQN, attrPath); ok {
						attrPath = declared
					} else {
						attrPath = entityQN + "." + attrPath
					}
				} else {
					// Validate that qualified attribute path belongs to the retrieved entity
					// Expected format: Module.Entity.Attribute
					parts := strings.Split(attrPath, ".")
					// A TWO-part path is never one of those, and the guard below used
					// to skip it entirely: `System.createdDate` reads as
					// Module.Attribute, named no entity, and was written straight
					// through as the sort's AttributeQualifiedName. Mendix then
					// resolves it to nothing and stores a null AttributeId, which
					// makes the project UNLOADABLE — not a CE code but an unhandled
					// System.ArgumentNullException out of AttributeRef.set_AttributeId,
					// so Studio Pro cannot open it either (ako/CapTrackV3 FINDINGS §46).
					//
					// Refused rather than repaired. `Module.Attribute` has no reading
					// that names an entity, and the two candidate repairs — treat it
					// as a bare member of the retrieved entity, or as a member of the
					// module's like-named entity — mean different things and only the
					// author knows which.
					//
					// A system member is spelled BARE (`sort by createdDate`), which
					// works whenever the entity stores it: measured on 11.14.0, that
					// builds at 0 errors, and the same sort on an entity that does not
					// store it is CE1613 — Mendix reporting the truth, not a defect.
					if len(parts) == 2 {
						fb.addError("sort by '%s' is not a valid attribute reference — a qualified sort "+
							"attribute is Module.Entity.Attribute, and '%s' names no entity. Mendix stores "+
							"an empty attribute reference for it, which makes the project impossible to "+
							"open. Write a system member bare (`sort by %s`), or qualify it fully",
							col.Attribute, attrPath, parts[1])
						continue // Skip this sort column but continue processing others
					}
					if len(parts) >= 3 {
						// Extract entity from attribute path (first two parts)
						attrEntityQN := parts[0] + "." + parts[1]
						// An ANCESTOR is not a foreign entity: naming the declaring
						// entity outright is the reference Mendix wants, and it was
						// refused as not belonging (CapTrackV2 §13). It takes no
						// EntityRefSteps either — inheritance is not a traversal,
						// the attribute is on the object already.
						if attrEntityQN != entityQN && !fb.entityIsSubtypeOf(entityQN, attrEntityQN) {
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
		BaseElement: model.BaseElement{ID: model.ID(types.GenerateID())},
		// ErrorHandlingType lives on the ACTION in the metamodel, not on the
		// activity — the activity-level assignment below is read by nothing and
		// serialized by neither engine, which is why `ON ERROR CONTINUE` on a
		// retrieve parsed, executed without complaint, and arrived in the model as
		// the writer's hardcoded "Rollback" (ako/CapTrackV3 FINDINGS §11).
		//
		// Only an EXPLICIT clause is carried. Left empty the writer keeps the
		// literal it has always written, so a retrieve nobody annotated is
		// byte-identical to before — which matters because a NANOFLOW's default is
		// Abort, and Abort is CE6035 on most activity types.
		ErrorHandlingType: explicitErrorHandling(fb, s.ErrorHandling),
		OutputVariable:    s.Variable,
		Source:            source,
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
		return visitor.FormatXPathConstraint(strings.TrimSpace(xpath))
	}
	// A constraint too long to read on one line is broken at its boolean joints;
	// one that already fits comes back unchanged (upstream #979).
	return visitor.FormatXPathConstraint("[" + xpath + "]")
}

// inferSortEntityRefSteps finds the one association hop that reaches the entity
// DECLARING the sort attribute, for a `sort by Module.Entity.Attribute` whose
// entity is neither the retrieved one nor an ancestor of it.
//
// Mendix stores such a sort as an AttributeRef whose AttributeQualifiedName
// names the far entity plus an EntityRef carrying one EntityRefStep per hop.
// DESCRIBE emits only the attribute's qualified name — MDL has no spelling for
// the hop — so replaying a described retrieve has to re-derive the step, and
// the whole round trip rests on that derivation.
//
// The association is not necessarily declared on the retrieved entity, nor in
// its module. `Administration.Account` reaches `System.Language` through
// `System.User_Language`, which is declared on the ANCESTOR `System.User` and
// stored in the SYSTEM module's domain model. Searching only the retrieved
// entity's own module for associations whose parent is the retrieved entity
// itself found nothing, so a retrieve `mxcli describe` had just emitted was
// refused by `exec` with "does not belong to entity" while `mxcli check` passed
// — mendixlabs/mxcli#1152. Same shape as the inherited-attribute defect
// (CapTrackV2 §13): the resolver that walks the generalization chain existed,
// and this path did not call it.
//
// So the walk is over the generalization chain, and each ancestor is looked up
// in ITS OWN module — which is also where the association's qualified name
// comes from. Qualifying with the retrieved entity's module is what the
// same-module case made look right, and it is wrong exactly in the case that
// was broken.
//
// The destination end is matched with entityIsSubtypeOf rather than by equality,
// because an association may point at a SPECIALIZATION of the entity that
// declares the attribute; Mendix stores the declaring entity in the path either
// way.
//
// Limitation, stated because the round trip depends on it: where several
// associations reach the same entity, MDL cannot say which one was stored, and
// the nearest entity's first association wins. The order is deterministic (both
// the chain walk and dm.Associations are ordered), so a replay is stable — but
// a model with two hops to one entity can still round-trip to the other one.
// Spelling the hop would need grammar, and is a language change, not a fix.
func (fb *flowBuilder) inferSortEntityRefSteps(sourceEntityQN, attrPath string) []microflows.EntityRefStep {
	attrEntityQN := entityQualifiedNameFromAttribute(attrPath)
	if attrEntityQN == "" || attrEntityQN == sourceEntityQN {
		return nil
	}
	if fb == nil || fb.backend == nil {
		return nil
	}
	seen := make(map[string]bool)
	for currentQN := sourceEntityQN; currentQN != ""; {
		if seen[currentQN] {
			return nil
		}
		seen[currentQN] = true

		parts := strings.SplitN(currentQN, ".", 2)
		if len(parts) != 2 || parts[0] == "" {
			return nil
		}
		moduleName := parts[0]
		mod, err := fb.backend.GetModuleByName(moduleName)
		if err != nil || mod == nil {
			return nil
		}
		dm, err := fb.backend.GetDomainModel(mod.ID)
		if err != nil || dm == nil {
			return nil
		}
		entityNames := make(map[model.ID]string, len(dm.Entities))
		for _, e := range dm.Entities {
			entityNames[e.ID] = moduleName + "." + e.Name
		}
		for _, assoc := range dm.Associations {
			if entityNames[assoc.ParentID] != currentQN {
				continue
			}
			childQN := entityNames[assoc.ChildID]
			if childQN != "" && fb.entityIsSubtypeOf(childQN, attrEntityQN) {
				return []microflows.EntityRefStep{{Association: moduleName + "." + assoc.Name, DestinationEntity: childQN}}
			}
		}
		for _, assoc := range dm.CrossAssociations {
			if entityNames[assoc.ParentID] != currentQN {
				continue
			}
			if assoc.ChildRef != "" && fb.entityIsSubtypeOf(assoc.ChildRef, attrEntityQN) {
				return []microflows.EntityRefStep{{Association: moduleName + "." + assoc.Name, DestinationEntity: assoc.ChildRef}}
			}
		}

		entity := dm.FindEntityByName(parts[1])
		if entity == nil {
			return nil
		}
		currentQN = entity.GeneralizationRef
	}
	return nil
}

// sortColumnText renders a sort column the way it was authored, for messages.
func sortColumnText(col ast.SortColumnDef) string {
	if len(col.Associations) == 0 {
		return col.Attribute
	}
	return strings.Join(append(append([]string{}, col.Associations...), col.Attribute), "/")
}

// resolveSortAssociationPath turns an authored `Assoc/…/Attribute` sort column
// into the EntityRefSteps Mendix stores alongside the attribute, plus the
// attribute's fully-qualified name.
//
// This is the spelling that exists because inference cannot be made correct:
// where two associations reach the same entity — `Order_ShipTo` and
// `Order_BillTo`, both `Order → Address`, an ordinary shape — the stored hop is
// not recoverable from the attribute name alone, and DESCRIBE emitted nothing
// else. Measured on 11.12.3: a microflow sorting by the billing address came
// back from `describe → exec` sorting by the shipping one, at 0 errors on both
// sides (mendixlabs/mxcli#1152).
//
// Everything here is refused rather than guessed. A hop that does not resolve,
// one that starts nowhere near the entity in hand, or a final attribute on an
// entity the last hop does not reach are each an error — a step written with an
// empty DestinationEntity is the one outcome worse than a refusal, since it
// makes the project unopenable (System.ArgumentNullException at
// EntityRefStep.set_DestinationEntityId) rather than merely wrong.
func (fb *flowBuilder) resolveSortAssociationPath(sourceEntityQN string, hops []string, attrName string) ([]microflows.EntityRefStep, string, error) {
	if sourceEntityQN == "" {
		return nil, "", fmt.Errorf("the retrieved entity is unknown, so the association path cannot be resolved")
	}
	if fb == nil || fb.backend == nil {
		return nil, "", fmt.Errorf("no project is open, so the association path cannot be resolved")
	}

	steps := make([]microflows.EntityRefStep, 0, len(hops))
	current := sourceEntityQN
	for _, hop := range hops {
		assocQN, info := fb.lookupSortHop(current, hop)
		if info == nil {
			return nil, "", fmt.Errorf("association '%s' was not found", hop)
		}
		var dest string
		switch {
		case fb.entityIsSubtypeOf(current, info.parentEntityQN):
			dest = info.childEntityQN
		case fb.entityIsSubtypeOf(current, info.childEntityQN):
			dest = info.parentEntityQN
		default:
			return nil, "", fmt.Errorf("association '%s' connects %s and %s, neither of which is %s",
				assocQN, info.parentEntityQN, info.childEntityQN, current)
		}
		if dest == "" {
			return nil, "", fmt.Errorf("association '%s' has an unresolved end, so the entity it reaches is unknown", assocQN)
		}
		steps = append(steps, microflows.EntityRefStep{Association: assocQN, DestinationEntity: dest})
		current = dest
	}

	// The final attribute is qualified with the entity that DECLARES it, which
	// for an inherited attribute is an ancestor of the last hop's destination —
	// the same rule (and the same CE1613 when broken) as a sort with no hops.
	if strings.Count(attrName, ".") >= 2 {
		owner := attrName[:strings.LastIndex(attrName, ".")]
		if !fb.entityIsSubtypeOf(current, owner) {
			return nil, "", fmt.Errorf("attribute '%s' does not belong to %s, which is where the association path ends",
				attrName, current)
		}
		return steps, attrName, nil
	}
	if declared, ok := fb.resolveAttributeInEntityHierarchy(current, attrName); ok {
		return steps, declared, nil
	}
	return nil, "", fmt.Errorf("entity %s has no attribute '%s'", current, attrName)
}

// lookupSortHop resolves one segment of a sort column's association path. A
// qualified segment names its module outright; a bare one is looked for in the
// modules of the entity in hand and of its ancestors, because an association is
// stored in the module of the entity that DECLARES it — which for an inherited
// one is not the module of the entity being sorted (mendixlabs/mxcli#1152).
func (fb *flowBuilder) lookupSortHop(currentEntityQN, hop string) (string, *assocLookupResult) {
	if i := strings.LastIndex(hop, "."); i > 0 {
		if info := fb.lookupAssociation(hop[:i], hop[i+1:]); info != nil {
			return hop, info
		}
		return hop, nil
	}
	for _, moduleName := range fb.entityChainModules(currentEntityQN) {
		if info := fb.lookupAssociation(moduleName, hop); info != nil {
			return moduleName + "." + hop, info
		}
	}
	return hop, nil
}

// entityChainModules lists the modules of an entity and of its ancestors,
// nearest first and without repeats.
func (fb *flowBuilder) entityChainModules(entityQN string) []string {
	var out []string
	seenModule := make(map[string]bool)
	seenEntity := make(map[string]bool)
	for currentQN := entityQN; currentQN != ""; {
		if seenEntity[currentQN] {
			break
		}
		seenEntity[currentQN] = true
		parts := strings.SplitN(currentQN, ".", 2)
		if len(parts) != 2 || parts[0] == "" {
			break
		}
		if !seenModule[parts[0]] {
			seenModule[parts[0]] = true
			out = append(out, parts[0])
		}
		if fb.backend == nil {
			break
		}
		mod, err := fb.backend.GetModuleByName(parts[0])
		if err != nil || mod == nil {
			break
		}
		dm, err := fb.backend.GetDomainModel(mod.ID)
		if err != nil || dm == nil {
			break
		}
		entity := dm.FindEntityByName(parts[1])
		if entity == nil {
			break
		}
		currentQN = entity.GeneralizationRef
	}
	return out
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
	// `contains` and `find` are overloaded: contains(list, object) / find(list,
	// condition) are list operations, but contains(haystack, needle) and
	// find(haystack, needle) over strings are String functions. When the input is
	// a declared String variable, Mendix requires a Change Variable action
	// carrying the string expression — a List operation activity on strings fails
	// the build (CE0023/CE0097/CE0111). Ledger findings #53 (contains) and #63 (find).
	//
	// The operation test is shared with MDL063, which must not report the
	// CE0111 this rewrite exists to avoid — see stringOverloadedListOp.
	if fb.declaredVars != nil && fb.declaredVars[s.InputVariable] == "String" &&
		stringOverloadedListOp(s.Operation) {
		switch s.Operation {
		case ast.ListOpContains:
			return fb.addChangeVariableAction(&ast.MfSetStmt{
				Target: s.OutputVariable,
				Value: &ast.FunctionCallExpr{
					Name: "contains",
					Arguments: []ast.Expression{
						&ast.VariableExpr{Name: s.InputVariable},
						&ast.VariableExpr{Name: s.SecondVariable},
					},
				},
			})
		case ast.ListOpFind:
			// The string find's second argument is carried as Condition (the
			// visitor stores arg1 there); rebuild `find($in, <arg1>)`.
			second := s.Condition
			if second == nil {
				second = &ast.VariableExpr{Name: s.SecondVariable}
			}
			return fb.addChangeVariableAction(&ast.MfSetStmt{
				Target: s.OutputVariable,
				Value: &ast.FunctionCallExpr{
					Name:      "find",
					Arguments: []ast.Expression{&ast.VariableExpr{Name: s.InputVariable}, second},
				},
			})
		}
	}

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
				Expression:   fb.exprToString(fb.qualifyIteratorAttributes(s.Condition, s.InputVariable, "find")),
			}
		}
	case ast.ListOpFilter:
		if op := fb.listAttributeOperation(s, true); op != nil {
			operation = op
		} else {
			operation = &microflows.FilterOperation{
				BaseElement:  model.BaseElement{ID: model.ID(types.GenerateID())},
				ListVariable: s.InputVariable,
				Expression:   fb.exprToString(fb.qualifyIteratorAttributes(s.Condition, s.InputVariable, "filter")),
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
	case ast.AggregateAll:
		function = microflows.AggregateFunctionAll
	case ast.AggregateAny:
		function = microflows.AggregateFunctionAny
	default:
		return ""
	}

	action := &microflows.AggregateListAction{
		BaseElement:    model.BaseElement{ID: model.ID(types.GenerateID())},
		InputVariable:  s.InputVariable,
		OutputVariable: s.OutputVariable,
		Function:       function,
	}

	// The fold Mendix stores beside the expression. REDUCE names both in MDL;
	// ALL and ANY always fold a Boolean and never take a seed, which is what
	// Studio Pro writes for them (empty initial value, Boolean return type).
	switch s.Operation {
	case ast.AggregateReduce:
		if s.InitialValue != nil {
			action.ReduceInitialValue = expressionToString(s.InitialValue)
		}
		if s.ReturnType != nil {
			action.ReduceReturnType = convertASTToMicroflowDataType(*s.ReturnType, nil)
		}
	case ast.AggregateAll, ast.AggregateAny:
		action.ReduceReturnType = &microflows.BooleanType{}
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
				// Not an association in the authored module. A single-qualifier
				// name (`Module.Name`) can ONLY be an association — attributes are
				// bare or `Module.Entity.Attribute` (two qualifiers). So a one-dot
				// member that isn't a known association is a reference to an
				// association that does not exist. Writing it as an Attribute
				// produces an *unloadable* .mpr (StorageLoadException "... is not a
				// valid AttributeIdentifier") rather than a clean CE error — reject
				// it here instead (FINDINGS #51). Associations created earlier in the
				// same script are visible via GetDomainModel, so this does not
				// false-positive on same-script associations.
				if strings.Count(memberName, ".") == 1 {
					fb.addError("member %q on entity %s is not a known association (and a one-qualifier name cannot be an attribute) — create the association first (`create or modify association %s from ... to ...`) or fix the name", memberName, entityQN, memberName)
					return
				}
				if strings.Contains(memberName, ".") {
					// Two-or-more-dot qualified attribute (Module.Entity.Attribute):
					// preserve the authored qualification; mx check surfaces a wrong one.
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
	// A domain model keeps associations in TWO lists: an association whose target
	// lives in another module (or in System) is a DomainModels$CrossAssociation,
	// where only the local end is BY_ID and the remote end is the BY_NAME
	// ChildRef. Searching only the first list left every cross-module hop
	// unresolvable, so an expression navigating one was written without its
	// target-entity step and mxbuild failed CE0117 (#829) — the same two-list
	// trap as #854 and issuetracker #19.
	//
	// The remote entity is not in this domain model, so its persistability is
	// unknown here; callers that need it must resolve the entity themselves
	// rather than read `false` as "non-persistable".
	for _, ca := range dm.CrossAssociations {
		if ca.Name == assocName {
			return &assocLookupResult{
				Type:              ca.Type,
				Owner:             ca.Owner,
				parentEntityQN:    entityNames[ca.ParentID],
				childEntityQN:     ca.ChildRef,
				parentPersistable: entityPersistable[ca.ParentID],
				childPersistable:  true,
			}
		}
	}
	return nil
}

// qualifyIteratorAttributes rewrites a bare attribute name in a FILTER/FIND
// predicate into `$currentObject/<Attr>`.
//
// Mendix's "filter by expression" / "find by expression" evaluates the predicate
// once per item with the item bound to `$currentObject`; a bare attribute name is
// not a valid expression there and the build fails with CE0117. mxcli used to
// store the authored text verbatim, so `filter($L, Amount > 0)` wrote
// `"Amount > 0"` and produced a model that `mxcli check` accepted and mxbuild
// rejected — measured identically on 11.11.0 and 11.13.0, so this is mxcli's
// behaviour and not a Mendix version change (issue #1002).
//
// This is the other half of bug #343. That fix rerouted `filter($L, attr = value)`
// to `Microflows$Filter` (filter BY ATTRIBUTE), which takes a member name rather
// than an expression and so accepts the bare form. Everything else — a different
// operator, a compound predicate — still fell through to the expression shape.
// The split was on the operator and invisible to the author: `Status = 'x'` built
// and `Status != 'x'` did not.
//
// Only a name that provably resolves to a member of the list's element entity is
// qualified. A name that does NOT resolve, when the entity IS known, is a real
// mistake and is reported here rather than left to surface as CE0117 at build
// time. When the element entity cannot be determined nothing is proven either
// way, so the expression is passed through unchanged.
func (fb *flowBuilder) qualifyIteratorAttributes(cond ast.Expression, listVariable, opLabel string) ast.Expression {
	if cond == nil {
		return nil
	}
	entityQN := fb.listElementEntity(listVariable)
	if entityQN == "" {
		return cond
	}

	// name → the path segment to write after `$currentObject/`. An attribute is
	// referenced by its bare name; an association carries its module qualifier.
	renamed := map[string]string{}
	var rewrite func(ast.Expression) ast.Expression
	rewrite = func(e ast.Expression) ast.Expression {
		switch n := e.(type) {
		case nil:
			return nil
		case *ast.IdentifierExpr:
			segment, ok := fb.iteratorMemberPath(entityQN, n.Name)
			if !ok {
				if !isMendixExpressionKeyword(n.Name) {
					fb.addError("%s($%s, …): %q is not an attribute or association of %s — a %s predicate is evaluated once per item, so a bare name must be a member of the list's entity (mxbuild reports CE0117 otherwise)",
						opLabel, listVariable, n.Name, entityQN, opLabel)
				}
				return n
			}
			renamed[n.Name] = segment
			return &ast.AttributePathExpr{
				Variable: "currentObject",
				Path:     []string{segment},
				Segments: []ast.PathSegment{{Name: segment, Separator: "/"}},
			}
		case *ast.BinaryExpr:
			return &ast.BinaryExpr{Left: rewrite(n.Left), Operator: n.Operator, Right: rewrite(n.Right)}
		case *ast.UnaryExpr:
			return &ast.UnaryExpr{Operator: n.Operator, Operand: rewrite(n.Operand)}
		case *ast.ParenExpr:
			return &ast.ParenExpr{Inner: rewrite(n.Inner)}
		case *ast.FunctionCallExpr:
			args := make([]ast.Expression, len(n.Arguments))
			for i, a := range n.Arguments {
				args[i] = rewrite(a)
			}
			return &ast.FunctionCallExpr{Name: n.Name, Arguments: args}
		case *ast.IfThenElseExpr:
			return &ast.IfThenElseExpr{
				Condition: rewrite(n.Condition),
				ThenExpr:  rewrite(n.ThenExpr),
				ElseExpr:  rewrite(n.ElseExpr),
			}
		case *ast.SourceExpr:
			inner := rewrite(n.Expression)
			if n.Source == "" {
				return inner
			}
			// A non-empty Source is the exact text to write back (decimal and
			// whitespace fidelity, #17-19), so patch the names inside it rather
			// than re-rendering the tree — the same treatment association paths
			// get in resolveAssociationPaths.
			return &ast.SourceExpr{Expression: inner, Source: qualifyNamesInSource(n.Source, renamed)}
		default:
			// LiteralExpr, VariableExpr, AttributePathExpr, QualifiedNameExpr,
			// ConstantRefExpr, TokenExpr, XPathPathExpr: nothing to qualify. An
			// AttributePathExpr is already anchored to some variable, and naming
			// the wrong one is MDL-LISTOP01's business, not this function's.
			return e
		}
	}

	return rewrite(cond)
}

// listElementEntity returns the qualified entity name of a list variable's
// elements, or "" when the variable's type was never tracked.
func (fb *flowBuilder) listElementEntity(listVariable string) string {
	if fb == nil || fb.varTypes == nil || listVariable == "" {
		return ""
	}
	entityQN, _ := strings.CutPrefix(fb.varTypes[listVariable], "List of ")
	if entityQN == fb.varTypes[listVariable] {
		return "" // not a list type
	}
	return entityQN
}

// iteratorMemberPath resolves a bare name against entityQN and returns the path
// segment that references it from `$currentObject`. An attribute keeps its bare
// name; an association is returned module-qualified, which is how Mendix spells
// one in a path (`$currentObject/Sales.Order_Customer`).
func (fb *flowBuilder) iteratorMemberPath(entityQN, name string) (string, bool) {
	if name == "" || strings.Contains(name, ".") {
		return "", false
	}
	if _, ok := fb.resolveAttributeInEntityHierarchy(entityQN, name); ok {
		return name, true
	}
	if fb.backend == nil {
		return "", false
	}
	module, _, found := strings.Cut(entityQN, ".")
	if !found {
		return "", false
	}
	mod, err := fb.backend.GetModuleByName(module)
	if err != nil || mod == nil {
		return "", false
	}
	dm, err := fb.backend.GetDomainModel(mod.ID)
	if err != nil || dm == nil {
		return "", false
	}
	for _, a := range dm.Associations {
		if a.Name == name {
			return module + "." + name, true
		}
	}
	for _, a := range dm.CrossAssociations {
		if a.Name == name {
			return module + "." + name, true
		}
	}
	return "", false
}

// isMendixExpressionKeyword reports whether name is a bare word Mendix defines
// itself, so an unresolved one is not reported as a missing attribute.
func isMendixExpressionKeyword(name string) bool {
	switch strings.ToLower(name) {
	case "true", "false", "empty", "nil", "null":
		return true
	}
	return false
}

// bareNameInSourceRe matches a whole-word occurrence of a name that is not
// already part of a path (`$currentObject/Amount`) or a qualified name
// (`Module.Amount`). Go's regexp has no lookbehind, so the preceding character
// is captured and put back.
var bareNameInSourceRe = regexp.MustCompile(`(^|[^\w./$])([A-Za-z_]\w*)(\b)`)

// qualifyNamesInSource rewrites each name in names to `$currentObject/<segment>`
// within expression source text, leaving single-quoted string literals alone.
func qualifyNamesInSource(source string, names map[string]string) string {
	if len(names) == 0 {
		return source
	}
	var out strings.Builder
	for i := 0; i < len(source); {
		if source[i] == '\'' {
			// Copy the whole literal verbatim, including doubled-quote escapes.
			j := i + 1
			for j < len(source) {
				if source[j] == '\'' {
					if j+1 < len(source) && source[j+1] == '\'' {
						j += 2
						continue
					}
					j++
					break
				}
				j++
			}
			out.WriteString(source[i:j])
			i = j
			continue
		}
		next := strings.IndexByte(source[i:], '\'')
		end := len(source)
		if next >= 0 {
			end = i + next
		}
		segment := source[i:end]
		out.WriteString(bareNameInSourceRe.ReplaceAllStringFunc(segment, func(m string) string {
			sub := bareNameInSourceRe.FindStringSubmatch(m)
			if len(sub) == 4 {
				if segment, ok := names[sub[2]]; ok {
					return sub[1] + "$currentObject/" + segment
				}
			}
			return m
		}))
		i = end
	}
	return out.String()
}
