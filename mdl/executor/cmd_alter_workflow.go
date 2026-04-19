// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"io"
	"strings"

	"go.mongodb.org/mongo-driver/bson"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/bsonutil"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/mpr"
	"github.com/mendixlabs/mxcli/sdk/workflows"
)

// bsonArrayMarker is the Mendix BSON array type marker (storageListType 3)
// that prefixes versioned arrays in serialized documents.
const bsonArrayMarker = int32(3)

// execAlterWorkflow handles ALTER WORKFLOW Module.Name { operations }.
func execAlterWorkflow(ctx *ExecContext, s *ast.AlterWorkflowStmt) error {
	if !ctx.Connected() {
		return mdlerrors.NewNotConnected()
	}
	if !ctx.ConnectedForWrite() {
		return mdlerrors.NewNotConnectedWrite()
	}

	// Version pre-check: workflows require Mendix 9.12+
	if err := checkFeature(ctx, "workflows", "basic",
		"ALTER WORKFLOW",
		"upgrade your project to Mendix 9.12+ to use workflows"); err != nil {
		return err
	}

	h, err := getHierarchy(ctx)
	if err != nil {
		return mdlerrors.NewBackend("build hierarchy", err)
	}

	// Find workflow by qualified name
	allWorkflows, err := ctx.Backend.ListWorkflows()
	if err != nil {
		return mdlerrors.NewBackend("list workflows", err)
	}

	var wfID model.ID
	for _, wf := range allWorkflows {
		modID := h.FindModuleID(wf.ContainerID)
		modName := h.GetModuleName(modID)
		if modName == s.Name.Module && wf.Name == s.Name.Name {
			wfID = wf.ID
			break
		}
	}
	if wfID == "" {
		return mdlerrors.NewNotFound("workflow", s.Name.Module+"."+s.Name.Name)
	}

	// Load raw BSON as ordered document
	rawBytes, err := ctx.Backend.GetRawUnitBytes(wfID)
	if err != nil {
		return mdlerrors.NewBackend("load raw workflow data", err)
	}
	var rawData bson.D
	if err := bson.Unmarshal(rawBytes, &rawData); err != nil {
		return mdlerrors.NewBackend("unmarshal workflow BSON", err)
	}

	// Apply operations sequentially
	for _, op := range s.Operations {
		switch o := op.(type) {
		case *ast.SetWorkflowPropertyOp:
			if err := applySetWorkflowProperty(&rawData, o); err != nil {
				return mdlerrors.NewBackend("SET "+o.Property, err)
			}
		case *ast.SetActivityPropertyOp:
			if err := applySetActivityProperty(rawData, o); err != nil {
				return mdlerrors.NewBackend("SET ACTIVITY", err)
			}
		case *ast.InsertAfterOp:
			if err := applyInsertAfterActivity(ctx, rawData, o); err != nil {
				return mdlerrors.NewBackend("INSERT AFTER", err)
			}
		case *ast.DropActivityOp:
			if err := applyDropActivity(rawData, o); err != nil {
				return mdlerrors.NewBackend("DROP ACTIVITY", err)
			}
		case *ast.ReplaceActivityOp:
			if err := applyReplaceActivity(ctx, rawData, o); err != nil {
				return mdlerrors.NewBackend("REPLACE ACTIVITY", err)
			}
		case *ast.InsertOutcomeOp:
			if err := applyInsertOutcome(ctx, rawData, o); err != nil {
				return mdlerrors.NewBackend("INSERT OUTCOME", err)
			}
		case *ast.DropOutcomeOp:
			if err := applyDropOutcome(rawData, o); err != nil {
				return mdlerrors.NewBackend("DROP OUTCOME", err)
			}
		case *ast.InsertPathOp:
			if err := applyInsertPath(ctx, rawData, o); err != nil {
				return mdlerrors.NewBackend("INSERT PATH", err)
			}
		case *ast.DropPathOp:
			if err := applyDropPath(rawData, o); err != nil {
				return mdlerrors.NewBackend("DROP PATH", err)
			}
		case *ast.InsertBranchOp:
			if err := applyInsertBranch(ctx, rawData, o); err != nil {
				return mdlerrors.NewBackend("INSERT BRANCH", err)
			}
		case *ast.DropBranchOp:
			if err := applyDropBranch(rawData, o); err != nil {
				return mdlerrors.NewBackend("DROP BRANCH", err)
			}
		case *ast.InsertBoundaryEventOp:
			if err := applyInsertBoundaryEvent(ctx, rawData, o); err != nil {
				return mdlerrors.NewBackend("INSERT BOUNDARY EVENT", err)
			}
		case *ast.DropBoundaryEventOp:
			if err := applyDropBoundaryEvent(ctx.Output, rawData, o); err != nil {
				return mdlerrors.NewBackend("DROP BOUNDARY EVENT", err)
			}
		default:
			return mdlerrors.NewUnsupported(fmt.Sprintf("unknown ALTER WORKFLOW operation type: %T", op))
		}
	}

	// Marshal back to BSON bytes
	outBytes, err := bson.Marshal(rawData)
	if err != nil {
		return mdlerrors.NewBackend("marshal modified workflow", err)
	}

	// Save
	if err := ctx.Backend.UpdateRawUnit(string(wfID), outBytes); err != nil {
		return mdlerrors.NewBackend("save modified workflow", err)
	}

	invalidateHierarchy(ctx)
	fmt.Fprintf(ctx.Output, "Altered workflow %s.%s\n", s.Name.Module, s.Name.Name)
	return nil
}

// applySetWorkflowProperty sets a workflow-level property in raw BSON.
func applySetWorkflowProperty(doc *bson.D, op *ast.SetWorkflowPropertyOp) error {
	switch op.Property {
	case "DISPLAY":
		// WorkflowName is a StringTemplate with Text field
		wfName := dGetDoc(*doc, "WorkflowName")
		if wfName == nil {
			// Auto-create the WorkflowName sub-document
			newName := bson.D{
				{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
				{Key: "$Type", Value: "Texts$Text"},
				{Key: "Text", Value: op.Value},
			}
			*doc = append(*doc, bson.E{Key: "WorkflowName", Value: newName})
		} else {
			dSet(wfName, "Text", op.Value)
		}
		// Also update Title (top-level string)
		dSet(*doc, "Title", op.Value)
		return nil

	case "DESCRIPTION":
		// WorkflowDescription is a StringTemplate with Text field
		wfDesc := dGetDoc(*doc, "WorkflowDescription")
		if wfDesc == nil {
			// Auto-create the WorkflowDescription sub-document
			newDesc := bson.D{
				{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
				{Key: "$Type", Value: "Texts$Text"},
				{Key: "Text", Value: op.Value},
			}
			*doc = append(*doc, bson.E{Key: "WorkflowDescription", Value: newDesc})
		} else {
			dSet(wfDesc, "Text", op.Value)
		}
		return nil

	case "EXPORT_LEVEL":
		dSet(*doc, "ExportLevel", op.Value)
		return nil

	case "DUE_DATE":
		dSet(*doc, "DueDate", op.Value)
		return nil

	case "OVERVIEW_PAGE":
		qn := op.Entity.Module + "." + op.Entity.Name
		if qn == "." {
			// Clear overview page
			dSet(*doc, "AdminPage", nil)
		} else {
			pageRef := bson.D{
				{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
				{Key: "$Type", Value: "Workflows$PageReference"},
				{Key: "Page", Value: qn},
			}
			dSet(*doc, "AdminPage", pageRef)
		}
		return nil

	case "PARAMETER":
		qn := op.Entity.Module + "." + op.Entity.Name
		if qn == "." {
			// Clear parameter — remove it
			for i, elem := range *doc {
				if elem.Key == "Parameter" {
					(*doc)[i].Value = nil
					return nil
				}
			}
			return nil
		}
		// Check if Parameter already exists
		param := dGetDoc(*doc, "Parameter")
		if param != nil {
			dSet(param, "Entity", qn)
		} else {
			// Create new Parameter
			newParam := bson.D{
				{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
				{Key: "$Type", Value: "Workflows$Parameter"},
				{Key: "Entity", Value: qn},
				{Key: "Name", Value: "WorkflowContext"},
			}
			// Check if field exists with nil value
			for i, elem := range *doc {
				if elem.Key == "Parameter" {
					(*doc)[i].Value = newParam
					return nil
				}
			}
			// Field doesn't exist — append it
			*doc = append(*doc, bson.E{Key: "Parameter", Value: newParam})
		}
		return nil

	default:
		return mdlerrors.NewUnsupported("unsupported workflow property: " + op.Property)
	}
}

// applySetActivityProperty sets a property on a named workflow activity in raw BSON.
func applySetActivityProperty(doc bson.D, op *ast.SetActivityPropertyOp) error {
	actDoc, err := findActivityByCaption(doc, op.ActivityRef, op.AtPosition)
	if err != nil {
		return err
	}

	switch op.Property {
	case "PAGE":
		qn := op.PageName.Module + "." + op.PageName.Name
		// TaskPage is a PageReference object
		taskPage := dGetDoc(actDoc, "TaskPage")
		if taskPage != nil {
			dSet(taskPage, "Page", qn)
		} else {
			// Create TaskPage
			pageRef := bson.D{
				{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
				{Key: "$Type", Value: "Workflows$PageReference"},
				{Key: "Page", Value: qn},
			}
			dSet(actDoc, "TaskPage", pageRef)
		}
		return nil

	case "DESCRIPTION":
		// TaskDescription is a StringTemplate
		taskDesc := dGetDoc(actDoc, "TaskDescription")
		if taskDesc != nil {
			dSet(taskDesc, "Text", op.Value)
		}
		return nil

	case "TARGETING_MICROFLOW":
		qn := op.Microflow.Module + "." + op.Microflow.Name
		userTargeting := bson.D{
			{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
			{Key: "$Type", Value: "Workflows$MicroflowUserTargeting"},
			{Key: "Microflow", Value: qn},
		}
		dSet(actDoc, "UserTargeting", userTargeting)
		return nil

	case "TARGETING_XPATH":
		userTargeting := bson.D{
			{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
			{Key: "$Type", Value: "Workflows$XPathUserTargeting"},
			{Key: "XPathConstraint", Value: op.Value},
		}
		dSet(actDoc, "UserTargeting", userTargeting)
		return nil

	case "DUE_DATE":
		dSet(actDoc, "DueDate", op.Value)
		return nil

	default:
		return mdlerrors.NewUnsupported("unsupported activity property: " + op.Property)
	}
}

// findActivityByCaption searches the workflow for an activity matching the given caption.
// Searches recursively through nested flows (Decision outcomes, ParallelSplit paths, UserTask outcomes, BoundaryEvents).
// atPosition provides positional disambiguation when multiple activities share the same caption (1-based).
func findActivityByCaption(doc bson.D, caption string, atPosition int) (bson.D, error) {
	flow := dGetDoc(doc, "Flow")
	if flow == nil {
		return nil, mdlerrors.NewValidation("workflow has no Flow")
	}

	var matches []bson.D
	findActivitiesRecursive(flow, caption, &matches)

	if len(matches) == 0 {
		return nil, mdlerrors.NewNotFound("activity", caption)
	}
	if len(matches) == 1 || atPosition == 0 {
		if atPosition > 0 && atPosition > len(matches) {
			return nil, mdlerrors.NewNotFoundMsg("activity", caption, fmt.Sprintf("activity %q at position %d not found (found %d matches)", caption, atPosition, len(matches)))
		}
		if atPosition > 0 {
			return matches[atPosition-1], nil
		}
		if len(matches) > 1 {
			return nil, mdlerrors.NewValidation(fmt.Sprintf("ambiguous activity %q — %d matches. Use @N to disambiguate", caption, len(matches)))
		}
		return matches[0], nil
	}
	if atPosition > len(matches) {
		return nil, mdlerrors.NewNotFoundMsg("activity", caption, fmt.Sprintf("activity %q at position %d not found (found %d matches)", caption, atPosition, len(matches)))
	}
	return matches[atPosition-1], nil
}

// findActivitiesRecursive collects all activities matching caption in a flow and its nested sub-flows.
func findActivitiesRecursive(flow bson.D, caption string, matches *[]bson.D) {
	activities := dGetArrayElements(dGet(flow, "Activities"))
	for _, elem := range activities {
		actDoc, ok := elem.(bson.D)
		if !ok {
			continue
		}
		actCaption := dGetString(actDoc, "Caption")
		actName := dGetString(actDoc, "Name")
		if actCaption == caption || actName == caption {
			*matches = append(*matches, actDoc)
		}
		// Recurse into nested flows: Outcomes (Decision, UserTask, CallMicroflow), Paths (ParallelSplit), BoundaryEvents
		for _, nestedFlow := range getNestedFlows(actDoc) {
			findActivitiesRecursive(nestedFlow, caption, matches)
		}
	}
}

// getNestedFlows returns all sub-flows within an activity (outcomes, paths, boundary events).
func getNestedFlows(actDoc bson.D) []bson.D {
	var flows []bson.D
	// Outcomes (UserTask, Decision, CallMicroflow)
	outcomes := dGetArrayElements(dGet(actDoc, "Outcomes"))
	for _, o := range outcomes {
		oDoc, ok := o.(bson.D)
		if !ok {
			continue
		}
		if f := dGetDoc(oDoc, "Flow"); f != nil {
			flows = append(flows, f)
		}
	}
	// BoundaryEvents
	events := dGetArrayElements(dGet(actDoc, "BoundaryEvents"))
	for _, e := range events {
		eDoc, ok := e.(bson.D)
		if !ok {
			continue
		}
		if f := dGetDoc(eDoc, "Flow"); f != nil {
			flows = append(flows, f)
		}
	}
	return flows
}

// findActivityIndex returns the index, activities array, and containing flow of an activity.
// Searches recursively through nested flows.
func findActivityIndex(doc bson.D, caption string, atPosition int) (int, []any, bson.D, error) {
	flow := dGetDoc(doc, "Flow")
	if flow == nil {
		return -1, nil, nil, mdlerrors.NewValidation("workflow has no Flow")
	}

	var matches []activityMatch
	findActivityIndexRecursive(flow, caption, &matches)

	if len(matches) == 0 {
		return -1, nil, nil, mdlerrors.NewNotFound("activity", caption)
	}
	pos := 0
	if atPosition > 0 {
		pos = atPosition - 1
	} else if len(matches) > 1 {
		return -1, nil, nil, mdlerrors.NewValidation(fmt.Sprintf("ambiguous activity %q — %d matches. Use @N to disambiguate", caption, len(matches)))
	}
	if pos >= len(matches) {
		return -1, nil, nil, mdlerrors.NewNotFoundMsg("activity", caption, fmt.Sprintf("activity %q at position %d not found (found %d matches)", caption, atPosition, len(matches)))
	}
	m := matches[pos]
	return m.idx, m.activities, m.flow, nil
}

type activityMatch struct {
	idx        int
	activities []any
	flow       bson.D
}

func findActivityIndexRecursive(flow bson.D, caption string, matches *[]activityMatch) {
	activities := dGetArrayElements(dGet(flow, "Activities"))
	for i, elem := range activities {
		actDoc, ok := elem.(bson.D)
		if !ok {
			continue
		}
		actCaption := dGetString(actDoc, "Caption")
		actName := dGetString(actDoc, "Name")
		if actCaption == caption || actName == caption {
			*matches = append(*matches, activityMatch{idx: i, activities: activities, flow: flow})
		}
		for _, nestedFlow := range getNestedFlows(actDoc) {
			findActivityIndexRecursive(nestedFlow, caption, matches)
		}
	}
}

// collectAllActivityNames collects all activity names from the entire workflow BSON (recursively).
func collectAllActivityNames(doc bson.D) map[string]bool {
	names := make(map[string]bool)
	flow := dGetDoc(doc, "Flow")
	if flow != nil {
		collectNamesRecursive(flow, names)
	}
	return names
}

func collectNamesRecursive(flow bson.D, names map[string]bool) {
	activities := dGetArrayElements(dGet(flow, "Activities"))
	for _, elem := range activities {
		actDoc, ok := elem.(bson.D)
		if !ok {
			continue
		}
		if name := dGetString(actDoc, "Name"); name != "" {
			names[name] = true
		}
		for _, nested := range getNestedFlows(actDoc) {
			collectNamesRecursive(nested, names)
		}
	}
}

// deduplicateNewActivityName ensures a new activity name doesn't conflict with existing names.
func deduplicateNewActivityName(act workflows.WorkflowActivity, existingNames map[string]bool) {
	name := act.GetName()
	if name == "" || !existingNames[name] {
		return
	}
	for i := 2; i < 1000; i++ {
		candidate := fmt.Sprintf("%s_%d", name, i)
		if !existingNames[candidate] {
			act.SetName(candidate)
			existingNames[candidate] = true
			return
		}
	}
}

// buildSubFlowBson builds a Workflows$Flow BSON document from AST activity nodes,
// with auto-binding and name deduplication against existing workflow activities.
func buildSubFlowBson(ctx *ExecContext, doc bson.D, activities []ast.WorkflowActivityNode) bson.D {
	subActs := buildWorkflowActivities(activities)
	autoBindActivitiesInFlow(ctx, subActs)
	existingNames := collectAllActivityNames(doc)
	for _, act := range subActs {
		deduplicateNewActivityName(act, existingNames)
	}
	var subActsBson bson.A
	subActsBson = append(subActsBson, bsonArrayMarker)
	for _, act := range subActs {
		bsonDoc := mpr.SerializeWorkflowActivity(act)
		if bsonDoc != nil {
			subActsBson = append(subActsBson, bsonDoc)
		}
	}
	return bson.D{
		{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
		{Key: "$Type", Value: "Workflows$Flow"},
		{Key: "Activities", Value: subActsBson},
	}
}

// applyInsertAfterActivity inserts a new activity after a named activity.
func applyInsertAfterActivity(ctx *ExecContext, doc bson.D, op *ast.InsertAfterOp) error {
	idx, activities, containingFlow, err := findActivityIndex(doc, op.ActivityRef, op.AtPosition)
	if err != nil {
		return err
	}

	newActs := buildWorkflowActivities([]ast.WorkflowActivityNode{op.NewActivity})
	if len(newActs) == 0 {
		return mdlerrors.NewValidation("failed to build new activity")
	}

	// Auto-bind parameters and deduplicate against existing workflow names
	autoBindActivitiesInFlow(ctx, newActs)
	existingNames := collectAllActivityNames(doc)
	for _, act := range newActs {
		deduplicateNewActivityName(act, existingNames)
	}

	newBsonActs := make([]any, 0, len(newActs))
	for _, act := range newActs {
		bsonDoc := mpr.SerializeWorkflowActivity(act)
		if bsonDoc != nil {
			newBsonActs = append(newBsonActs, bsonDoc)
		}
	}

	insertIdx := idx + 1
	newArr := make([]any, 0, len(activities)+len(newBsonActs))
	newArr = append(newArr, activities[:insertIdx]...)
	newArr = append(newArr, newBsonActs...)
	newArr = append(newArr, activities[insertIdx:]...)

	dSetArray(containingFlow, "Activities", newArr)
	return nil
}

// applyDropActivity removes an activity from the flow.
func applyDropActivity(doc bson.D, op *ast.DropActivityOp) error {
	idx, activities, containingFlow, err := findActivityIndex(doc, op.ActivityRef, op.AtPosition)
	if err != nil {
		return err
	}

	newArr := make([]any, 0, len(activities)-1)
	newArr = append(newArr, activities[:idx]...)
	newArr = append(newArr, activities[idx+1:]...)

	dSetArray(containingFlow, "Activities", newArr)
	return nil
}

// applyReplaceActivity replaces an activity in place.
func applyReplaceActivity(ctx *ExecContext, doc bson.D, op *ast.ReplaceActivityOp) error {
	idx, activities, containingFlow, err := findActivityIndex(doc, op.ActivityRef, op.AtPosition)
	if err != nil {
		return err
	}

	newActs := buildWorkflowActivities([]ast.WorkflowActivityNode{op.NewActivity})
	if len(newActs) == 0 {
		return mdlerrors.NewValidation("failed to build replacement activity")
	}

	autoBindActivitiesInFlow(ctx, newActs)
	existingNames := collectAllActivityNames(doc)
	for _, act := range newActs {
		deduplicateNewActivityName(act, existingNames)
	}

	newBsonActs := make([]any, 0, len(newActs))
	for _, act := range newActs {
		bsonDoc := mpr.SerializeWorkflowActivity(act)
		if bsonDoc != nil {
			newBsonActs = append(newBsonActs, bsonDoc)
		}
	}

	newArr := make([]any, 0, len(activities)-1+len(newBsonActs))
	newArr = append(newArr, activities[:idx]...)
	newArr = append(newArr, newBsonActs...)
	newArr = append(newArr, activities[idx+1:]...)

	dSetArray(containingFlow, "Activities", newArr)
	return nil
}

// applyInsertOutcome adds a new outcome to a user task.
func applyInsertOutcome(ctx *ExecContext, doc bson.D, op *ast.InsertOutcomeOp) error {
	actDoc, err := findActivityByCaption(doc, op.ActivityRef, op.AtPosition)
	if err != nil {
		return err
	}

	// Build outcome BSON
	outcomeDoc := bson.D{
		{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
		{Key: "$Type", Value: "Workflows$UserTaskOutcome"},
	}

	// Build sub-flow if activities provided
	if len(op.Activities) > 0 {
		outcomeDoc = append(outcomeDoc, bson.E{Key: "Flow", Value: buildSubFlowBson(ctx, doc, op.Activities)})
	}

	outcomeDoc = append(outcomeDoc,
		bson.E{Key: "PersistentId", Value: bsonutil.NewIDBsonBinary()},
		bson.E{Key: "Value", Value: op.OutcomeName},
	)

	// Append to Outcomes array
	outcomes := dGetArrayElements(dGet(actDoc, "Outcomes"))
	outcomes = append(outcomes, outcomeDoc)
	dSetArray(actDoc, "Outcomes", outcomes)
	return nil
}

// applyDropOutcome removes an outcome from a user task.
func applyDropOutcome(doc bson.D, op *ast.DropOutcomeOp) error {
	actDoc, err := findActivityByCaption(doc, op.ActivityRef, op.AtPosition)
	if err != nil {
		return err
	}

	outcomes := dGetArrayElements(dGet(actDoc, "Outcomes"))
	found := false
	var kept []any
	for _, elem := range outcomes {
		oDoc, ok := elem.(bson.D)
		if !ok {
			kept = append(kept, elem)
			continue
		}
		value := dGetString(oDoc, "Value")
		typeName := dGetString(oDoc, "$Type")
		// Match by Value, or by $Type for VoidConditionOutcome ("Default")
		matched := value == op.OutcomeName
		if !matched && strings.EqualFold(op.OutcomeName, "Default") && typeName == "Workflows$VoidConditionOutcome" {
			matched = true
		}
		if matched && !found {
			found = true
			continue
		}
		kept = append(kept, elem)
	}
	if !found {
		return mdlerrors.NewNotFoundMsg("outcome", op.OutcomeName, fmt.Sprintf("outcome %q not found on activity %q", op.OutcomeName, op.ActivityRef))
	}
	dSetArray(actDoc, "Outcomes", kept)
	return nil
}

// applyInsertPath adds a new path to a parallel split.
func applyInsertPath(ctx *ExecContext, doc bson.D, op *ast.InsertPathOp) error {
	actDoc, err := findActivityByCaption(doc, op.ActivityRef, op.AtPosition)
	if err != nil {
		return err
	}

	pathDoc := bson.D{
		{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
		{Key: "$Type", Value: "Workflows$ParallelSplitOutcome"},
	}

	if len(op.Activities) > 0 {
		pathDoc = append(pathDoc, bson.E{Key: "Flow", Value: buildSubFlowBson(ctx, doc, op.Activities)})
	}

	pathDoc = append(pathDoc, bson.E{Key: "PersistentId", Value: bsonutil.NewIDBsonBinary()})

	outcomes := dGetArrayElements(dGet(actDoc, "Outcomes"))
	outcomes = append(outcomes, pathDoc)
	dSetArray(actDoc, "Outcomes", outcomes)
	return nil
}

// applyDropPath removes a path from a parallel split by caption.
func applyDropPath(doc bson.D, op *ast.DropPathOp) error {
	actDoc, err := findActivityByCaption(doc, op.ActivityRef, op.AtPosition)
	if err != nil {
		return err
	}

	outcomes := dGetArrayElements(dGet(actDoc, "Outcomes"))
	if op.PathCaption == "" && len(outcomes) > 0 {
		// Drop last path
		outcomes = outcomes[:len(outcomes)-1]
		dSetArray(actDoc, "Outcomes", outcomes)
		return nil
	}

	// Find by index (paths are numbered 1-based in MDL)
	pathIdx := -1
	for i := range outcomes {
		// Path captions are typically "Path 1", "Path 2" etc.
		if fmt.Sprintf("Path %d", i+1) == op.PathCaption {
			pathIdx = i
			break
		}
	}
	if pathIdx < 0 {
		return mdlerrors.NewNotFoundMsg("path", op.PathCaption, fmt.Sprintf("path %q not found on parallel split %q", op.PathCaption, op.ActivityRef))
	}

	newOutcomes := make([]any, 0, len(outcomes)-1)
	newOutcomes = append(newOutcomes, outcomes[:pathIdx]...)
	newOutcomes = append(newOutcomes, outcomes[pathIdx+1:]...)
	dSetArray(actDoc, "Outcomes", newOutcomes)
	return nil
}

// applyInsertBranch adds a new branch to a decision.
func applyInsertBranch(ctx *ExecContext, doc bson.D, op *ast.InsertBranchOp) error {
	actDoc, err := findActivityByCaption(doc, op.ActivityRef, op.AtPosition)
	if err != nil {
		return err
	}

	// Build the condition outcome BSON
	var outcomeDoc bson.D
	switch strings.ToLower(op.Condition) {
	case "true":
		outcomeDoc = bson.D{
			{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
			{Key: "$Type", Value: "Workflows$BooleanConditionOutcome"},
			{Key: "Value", Value: true},
		}
	case "false":
		outcomeDoc = bson.D{
			{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
			{Key: "$Type", Value: "Workflows$BooleanConditionOutcome"},
			{Key: "Value", Value: false},
		}
	case "default":
		outcomeDoc = bson.D{
			{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
			{Key: "$Type", Value: "Workflows$VoidConditionOutcome"},
		}
	default:
		outcomeDoc = bson.D{
			{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
			{Key: "$Type", Value: "Workflows$EnumerationValueConditionOutcome"},
			{Key: "Value", Value: op.Condition},
		}
	}

	if len(op.Activities) > 0 {
		outcomeDoc = append(outcomeDoc, bson.E{Key: "Flow", Value: buildSubFlowBson(ctx, doc, op.Activities)})
	}

	outcomes := dGetArrayElements(dGet(actDoc, "Outcomes"))
	outcomes = append(outcomes, outcomeDoc)
	dSetArray(actDoc, "Outcomes", outcomes)
	return nil
}

// applyDropBranch removes a branch from a decision.
func applyDropBranch(doc bson.D, op *ast.DropBranchOp) error {
	actDoc, err := findActivityByCaption(doc, op.ActivityRef, op.AtPosition)
	if err != nil {
		return err
	}

	outcomes := dGetArrayElements(dGet(actDoc, "Outcomes"))
	found := false
	var kept []any
	for _, elem := range outcomes {
		oDoc, ok := elem.(bson.D)
		if !ok {
			kept = append(kept, elem)
			continue
		}
		if !found {
			// Match by Value or $Type for void outcomes
			typeName := dGetString(oDoc, "$Type")
			switch strings.ToLower(op.BranchName) {
			case "true":
				if typeName == "Workflows$BooleanConditionOutcome" {
					if v, ok := dGet(oDoc, "Value").(bool); ok && v {
						found = true
						continue
					}
				}
			case "false":
				if typeName == "Workflows$BooleanConditionOutcome" {
					if v, ok := dGet(oDoc, "Value").(bool); ok && !v {
						found = true
						continue
					}
				}
			case "default":
				if typeName == "Workflows$VoidConditionOutcome" {
					found = true
					continue
				}
			default:
				value := dGetString(oDoc, "Value")
				if value == op.BranchName {
					found = true
					continue
				}
			}
		}
		kept = append(kept, elem)
	}
	if !found {
		return mdlerrors.NewNotFoundMsg("branch", op.BranchName, fmt.Sprintf("branch %q not found on activity %q", op.BranchName, op.ActivityRef))
	}
	dSetArray(actDoc, "Outcomes", kept)
	return nil
}

// applyInsertBoundaryEvent adds a boundary event to an activity.
func applyInsertBoundaryEvent(ctx *ExecContext, doc bson.D, op *ast.InsertBoundaryEventOp) error {
	actDoc, err := findActivityByCaption(doc, op.ActivityRef, op.AtPosition)
	if err != nil {
		return err
	}

	typeName := "Workflows$InterruptingTimerBoundaryEvent"
	switch op.EventType {
	case "NonInterruptingTimer":
		typeName = "Workflows$NonInterruptingTimerBoundaryEvent"
	case "Timer":
		typeName = "Workflows$TimerBoundaryEvent"
	}

	eventDoc := bson.D{
		{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
		{Key: "$Type", Value: typeName},
		{Key: "Caption", Value: ""},
	}

	if op.Delay != "" {
		eventDoc = append(eventDoc, bson.E{Key: "FirstExecutionTime", Value: op.Delay})
	}

	if len(op.Activities) > 0 {
		eventDoc = append(eventDoc, bson.E{Key: "Flow", Value: buildSubFlowBson(ctx, doc, op.Activities)})
	}

	eventDoc = append(eventDoc, bson.E{Key: "PersistentId", Value: bsonutil.NewIDBsonBinary()})

	if typeName == "Workflows$NonInterruptingTimerBoundaryEvent" {
		eventDoc = append(eventDoc, bson.E{Key: "Recurrence", Value: nil})
	}

	// Append to BoundaryEvents array
	events := dGetArrayElements(dGet(actDoc, "BoundaryEvents"))
	events = append(events, eventDoc)
	dSetArray(actDoc, "BoundaryEvents", events)
	return nil
}

// applyDropBoundaryEvent removes the first boundary event from an activity.
//
// Limitation: this always removes events[0]. There is currently no syntax to
// target a specific boundary event by name or type when multiple exist.
func applyDropBoundaryEvent(w io.Writer, doc bson.D, op *ast.DropBoundaryEventOp) error {
	actDoc, err := findActivityByCaption(doc, op.ActivityRef, op.AtPosition)
	if err != nil {
		return err
	}

	events := dGetArrayElements(dGet(actDoc, "BoundaryEvents"))
	if len(events) == 0 {
		return mdlerrors.NewValidation(fmt.Sprintf("activity %q has no boundary events", op.ActivityRef))
	}

	if len(events) > 1 {
		fmt.Fprintf(w, "warning: activity %q has %d boundary events; dropping the first one\n", op.ActivityRef, len(events))
	}

	// Drop the first boundary event
	dSetArray(actDoc, "BoundaryEvents", events[1:])
	return nil
}
