// SPDX-License-Identifier: Apache-2.0

package mprbackend

import (
	"fmt"
	"strings"

	"go.mongodb.org/mongo-driver/bson"

	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/bsonutil"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/mpr"
	"github.com/mendixlabs/mxcli/sdk/workflows"
)

// bsonArrayMarker is the Mendix BSON array type marker (storageListType 3).
const bsonArrayMarker = int32(3)

// Compile-time check.
var _ backend.WorkflowMutator = (*mprWorkflowMutator)(nil)

// mprWorkflowMutator implements backend.WorkflowMutator for the MPR backend.
type mprWorkflowMutator struct {
	backend *MprBackend
	unitID  model.ID
	rawData bson.D
}

// ---------------------------------------------------------------------------
// OpenWorkflowForMutation
// ---------------------------------------------------------------------------

// OpenWorkflowForMutation loads a workflow unit and returns a WorkflowMutator.
func (b *MprBackend) openWorkflowForMutation(unitID model.ID) (backend.WorkflowMutator, error) {
	rawBytes, err := b.reader.GetRawUnitBytes(unitID)
	if err != nil {
		return nil, fmt.Errorf("load raw unit bytes: %w", err)
	}
	var rawData bson.D
	if err := bson.Unmarshal(rawBytes, &rawData); err != nil {
		return nil, fmt.Errorf("unmarshal workflow BSON: %w", err)
	}
	return &mprWorkflowMutator{
		backend: b,
		unitID:  unitID,
		rawData: rawData,
	}, nil
}

// ---------------------------------------------------------------------------
// WorkflowMutator interface — top-level properties
// ---------------------------------------------------------------------------

func (m *mprWorkflowMutator) SetProperty(prop string, value string) error {
	switch prop {
	case "DISPLAY":
		wfName := dGetDoc(m.rawData, "WorkflowName")
		if wfName == nil {
			newName := bson.D{
				{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
				{Key: "$Type", Value: "Texts$Text"},
				{Key: "Text", Value: value},
			}
			m.rawData = append(m.rawData, bson.E{Key: "WorkflowName", Value: newName})
		} else {
			dSet(wfName, "Text", value)
		}
		dSet(m.rawData, "Title", value)
		return nil

	case "DESCRIPTION":
		wfDesc := dGetDoc(m.rawData, "WorkflowDescription")
		if wfDesc == nil {
			newDesc := bson.D{
				{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
				{Key: "$Type", Value: "Texts$Text"},
				{Key: "Text", Value: value},
			}
			m.rawData = append(m.rawData, bson.E{Key: "WorkflowDescription", Value: newDesc})
		} else {
			dSet(wfDesc, "Text", value)
		}
		return nil

	case "EXPORT_LEVEL":
		dSet(m.rawData, "ExportLevel", value)
		return nil

	case "DUE_DATE":
		dSet(m.rawData, "DueDate", value)
		return nil

	default:
		return fmt.Errorf("unsupported workflow property: %s", prop)
	}
}

func (m *mprWorkflowMutator) SetPropertyWithEntity(prop string, value string, entity string) error {
	switch prop {
	case "OVERVIEW_PAGE":
		if value == "" {
			dSet(m.rawData, "AdminPage", nil)
		} else {
			pageRef := bson.D{
				{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
				{Key: "$Type", Value: "Workflows$PageReference"},
				{Key: "Page", Value: value},
			}
			dSet(m.rawData, "AdminPage", pageRef)
		}
		return nil

	case "PARAMETER":
		if value == "" {
			for i, elem := range m.rawData {
				if elem.Key == "Parameter" {
					m.rawData[i].Value = nil
					return nil
				}
			}
			return nil
		}
		param := dGetDoc(m.rawData, "Parameter")
		if param != nil {
			dSet(param, "Entity", entity)
		} else {
			newParam := bson.D{
				{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
				{Key: "$Type", Value: "Workflows$Parameter"},
				{Key: "Entity", Value: entity},
				{Key: "Name", Value: "WorkflowContext"},
			}
			for i, elem := range m.rawData {
				if elem.Key == "Parameter" {
					m.rawData[i].Value = newParam
					return nil
				}
			}
			m.rawData = append(m.rawData, bson.E{Key: "Parameter", Value: newParam})
		}
		return nil

	default:
		return fmt.Errorf("unsupported workflow property with entity: %s", prop)
	}
}

// ---------------------------------------------------------------------------
// WorkflowMutator interface — activity operations
// ---------------------------------------------------------------------------

func (m *mprWorkflowMutator) SetActivityProperty(activityRef string, atPos int, prop string, value string) error {
	actDoc, err := m.findActivityByCaption(activityRef, atPos)
	if err != nil {
		return err
	}

	switch prop {
	case "PAGE":
		taskPage := dGetDoc(actDoc, "TaskPage")
		if taskPage != nil {
			dSet(taskPage, "Page", value)
		} else {
			pageRef := bson.D{
				{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
				{Key: "$Type", Value: "Workflows$PageReference"},
				{Key: "Page", Value: value},
			}
			dSet(actDoc, "TaskPage", pageRef)
		}
		return nil

	case "DESCRIPTION":
		taskDesc := dGetDoc(actDoc, "TaskDescription")
		if taskDesc != nil {
			dSet(taskDesc, "Text", value)
		}
		return nil

	case "TARGETING_MICROFLOW":
		userTargeting := bson.D{
			{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
			{Key: "$Type", Value: "Workflows$MicroflowUserTargeting"},
			{Key: "Microflow", Value: value},
		}
		dSet(actDoc, "UserTargeting", userTargeting)
		return nil

	case "TARGETING_XPATH":
		userTargeting := bson.D{
			{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
			{Key: "$Type", Value: "Workflows$XPathUserTargeting"},
			{Key: "XPathConstraint", Value: value},
		}
		dSet(actDoc, "UserTargeting", userTargeting)
		return nil

	case "DUE_DATE":
		dSet(actDoc, "DueDate", value)
		return nil

	default:
		return fmt.Errorf("unsupported activity property: %s", prop)
	}
}

func (m *mprWorkflowMutator) InsertAfterActivity(activityRef string, atPos int, activities []workflows.WorkflowActivity) error {
	idx, acts, containingFlow, err := m.findActivityIndex(activityRef, atPos)
	if err != nil {
		return err
	}

	newBsonActs := m.serializeAndDedup(activities)

	insertIdx := idx + 1
	newArr := make([]any, 0, len(acts)+len(newBsonActs))
	newArr = append(newArr, acts[:insertIdx]...)
	newArr = append(newArr, newBsonActs...)
	newArr = append(newArr, acts[insertIdx:]...)

	dSetArray(containingFlow, "Activities", newArr)
	return nil
}

func (m *mprWorkflowMutator) DropActivity(activityRef string, atPos int) error {
	idx, acts, containingFlow, err := m.findActivityIndex(activityRef, atPos)
	if err != nil {
		return err
	}

	newArr := make([]any, 0, len(acts)-1)
	newArr = append(newArr, acts[:idx]...)
	newArr = append(newArr, acts[idx+1:]...)

	dSetArray(containingFlow, "Activities", newArr)
	return nil
}

func (m *mprWorkflowMutator) ReplaceActivity(activityRef string, atPos int, activities []workflows.WorkflowActivity) error {
	idx, acts, containingFlow, err := m.findActivityIndex(activityRef, atPos)
	if err != nil {
		return err
	}

	newBsonActs := m.serializeAndDedup(activities)

	newArr := make([]any, 0, len(acts)-1+len(newBsonActs))
	newArr = append(newArr, acts[:idx]...)
	newArr = append(newArr, newBsonActs...)
	newArr = append(newArr, acts[idx+1:]...)

	dSetArray(containingFlow, "Activities", newArr)
	return nil
}

// ---------------------------------------------------------------------------
// WorkflowMutator interface — outcome operations
// ---------------------------------------------------------------------------

func (m *mprWorkflowMutator) InsertOutcome(activityRef string, atPos int, outcomeName string, activities []workflows.WorkflowActivity) error {
	actDoc, err := m.findActivityByCaption(activityRef, atPos)
	if err != nil {
		return err
	}

	outcomeDoc := bson.D{
		{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
		{Key: "$Type", Value: "Workflows$UserTaskOutcome"},
	}

	if len(activities) > 0 {
		outcomeDoc = append(outcomeDoc, bson.E{Key: "Flow", Value: m.buildSubFlowBson(activities)})
	}

	outcomeDoc = append(outcomeDoc,
		bson.E{Key: "PersistentId", Value: bsonutil.NewIDBsonBinary()},
		bson.E{Key: "Value", Value: outcomeName},
	)

	outcomes := dGetArrayElements(dGet(actDoc, "Outcomes"))
	outcomes = append(outcomes, outcomeDoc)
	dSetArray(actDoc, "Outcomes", outcomes)
	return nil
}

func (m *mprWorkflowMutator) DropOutcome(activityRef string, atPos int, outcomeName string) error {
	actDoc, err := m.findActivityByCaption(activityRef, atPos)
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
		matched := value == outcomeName
		if !matched && strings.EqualFold(outcomeName, "Default") && typeName == "Workflows$VoidConditionOutcome" {
			matched = true
		}
		if matched && !found {
			found = true
			continue
		}
		kept = append(kept, elem)
	}
	if !found {
		return fmt.Errorf("outcome %q not found on activity %q", outcomeName, activityRef)
	}
	dSetArray(actDoc, "Outcomes", kept)
	return nil
}

// ---------------------------------------------------------------------------
// WorkflowMutator interface — path operations (parallel split)
// ---------------------------------------------------------------------------

func (m *mprWorkflowMutator) InsertPath(activityRef string, atPos int, pathCaption string, activities []workflows.WorkflowActivity) error {
	actDoc, err := m.findActivityByCaption(activityRef, atPos)
	if err != nil {
		return err
	}

	pathDoc := bson.D{
		{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
		{Key: "$Type", Value: "Workflows$ParallelSplitOutcome"},
	}

	if len(activities) > 0 {
		pathDoc = append(pathDoc, bson.E{Key: "Flow", Value: m.buildSubFlowBson(activities)})
	}

	pathDoc = append(pathDoc, bson.E{Key: "PersistentId", Value: bsonutil.NewIDBsonBinary()})

	outcomes := dGetArrayElements(dGet(actDoc, "Outcomes"))
	outcomes = append(outcomes, pathDoc)
	dSetArray(actDoc, "Outcomes", outcomes)
	return nil
}

func (m *mprWorkflowMutator) DropPath(activityRef string, atPos int, pathCaption string) error {
	actDoc, err := m.findActivityByCaption(activityRef, atPos)
	if err != nil {
		return err
	}

	outcomes := dGetArrayElements(dGet(actDoc, "Outcomes"))
	if pathCaption == "" && len(outcomes) > 0 {
		outcomes = outcomes[:len(outcomes)-1]
		dSetArray(actDoc, "Outcomes", outcomes)
		return nil
	}

	pathIdx := -1
	for i := range outcomes {
		if fmt.Sprintf("Path %d", i+1) == pathCaption {
			pathIdx = i
			break
		}
	}
	if pathIdx < 0 {
		return fmt.Errorf("path %q not found on parallel split %q", pathCaption, activityRef)
	}

	newOutcomes := make([]any, 0, len(outcomes)-1)
	newOutcomes = append(newOutcomes, outcomes[:pathIdx]...)
	newOutcomes = append(newOutcomes, outcomes[pathIdx+1:]...)
	dSetArray(actDoc, "Outcomes", newOutcomes)
	return nil
}

// ---------------------------------------------------------------------------
// WorkflowMutator interface — branch operations (exclusive split)
// ---------------------------------------------------------------------------

func (m *mprWorkflowMutator) InsertBranch(activityRef string, atPos int, condition string, activities []workflows.WorkflowActivity) error {
	actDoc, err := m.findActivityByCaption(activityRef, atPos)
	if err != nil {
		return err
	}

	var outcomeDoc bson.D
	switch strings.ToLower(condition) {
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
			{Key: "Value", Value: condition},
		}
	}

	if len(activities) > 0 {
		outcomeDoc = append(outcomeDoc, bson.E{Key: "Flow", Value: m.buildSubFlowBson(activities)})
	}

	outcomes := dGetArrayElements(dGet(actDoc, "Outcomes"))
	outcomes = append(outcomes, outcomeDoc)
	dSetArray(actDoc, "Outcomes", outcomes)
	return nil
}

func (m *mprWorkflowMutator) DropBranch(activityRef string, atPos int, branchName string) error {
	actDoc, err := m.findActivityByCaption(activityRef, atPos)
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
			typeName := dGetString(oDoc, "$Type")
			switch strings.ToLower(branchName) {
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
				if value == branchName {
					found = true
					continue
				}
			}
		}
		kept = append(kept, elem)
	}
	if !found {
		return fmt.Errorf("branch %q not found on activity %q", branchName, activityRef)
	}
	dSetArray(actDoc, "Outcomes", kept)
	return nil
}

// ---------------------------------------------------------------------------
// WorkflowMutator interface — boundary event operations
// ---------------------------------------------------------------------------

func (m *mprWorkflowMutator) InsertBoundaryEvent(activityRef string, atPos int, eventType string, delay string, activities []workflows.WorkflowActivity) error {
	actDoc, err := m.findActivityByCaption(activityRef, atPos)
	if err != nil {
		return err
	}

	typeName := "Workflows$InterruptingTimerBoundaryEvent"
	switch eventType {
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

	if delay != "" {
		eventDoc = append(eventDoc, bson.E{Key: "FirstExecutionTime", Value: delay})
	}

	if len(activities) > 0 {
		eventDoc = append(eventDoc, bson.E{Key: "Flow", Value: m.buildSubFlowBson(activities)})
	}

	eventDoc = append(eventDoc, bson.E{Key: "PersistentId", Value: bsonutil.NewIDBsonBinary()})

	if typeName == "Workflows$NonInterruptingTimerBoundaryEvent" {
		eventDoc = append(eventDoc, bson.E{Key: "Recurrence", Value: nil})
	}

	events := dGetArrayElements(dGet(actDoc, "BoundaryEvents"))
	events = append(events, eventDoc)
	dSetArray(actDoc, "BoundaryEvents", events)
	return nil
}

func (m *mprWorkflowMutator) DropBoundaryEvent(activityRef string, atPos int) error {
	actDoc, err := m.findActivityByCaption(activityRef, atPos)
	if err != nil {
		return err
	}

	events := dGetArrayElements(dGet(actDoc, "BoundaryEvents"))
	if len(events) == 0 {
		return fmt.Errorf("activity %q has no boundary events", activityRef)
	}

	// Drop the first boundary event silently.
	dSetArray(actDoc, "BoundaryEvents", events[1:])
	return nil
}

// ---------------------------------------------------------------------------
// Save
// ---------------------------------------------------------------------------

func (m *mprWorkflowMutator) Save() error {
	outBytes, err := bson.Marshal(m.rawData)
	if err != nil {
		return fmt.Errorf("marshal modified workflow: %w", err)
	}
	return m.backend.writer.UpdateRawUnit(string(m.unitID), outBytes)
}

// ---------------------------------------------------------------------------
// Internal helpers — activity search
// ---------------------------------------------------------------------------

// findActivityByCaption searches the workflow for an activity matching caption.
func (m *mprWorkflowMutator) findActivityByCaption(caption string, atPosition int) (bson.D, error) {
	flow := dGetDoc(m.rawData, "Flow")
	if flow == nil {
		return nil, fmt.Errorf("workflow has no Flow")
	}

	var matches []bson.D
	findActivitiesRecursive(flow, caption, &matches)

	if len(matches) == 0 {
		return nil, fmt.Errorf("activity %q not found", caption)
	}
	if len(matches) == 1 || atPosition == 0 {
		if atPosition > 0 && atPosition > len(matches) {
			return nil, fmt.Errorf("activity %q at position %d not found (found %d matches)", caption, atPosition, len(matches))
		}
		if atPosition > 0 {
			return matches[atPosition-1], nil
		}
		if len(matches) > 1 {
			return nil, fmt.Errorf("ambiguous activity %q — %d matches. Use @N to disambiguate", caption, len(matches))
		}
		return matches[0], nil
	}
	if atPosition > len(matches) {
		return nil, fmt.Errorf("activity %q at position %d not found (found %d matches)", caption, atPosition, len(matches))
	}
	return matches[atPosition-1], nil
}

// findActivitiesRecursive collects all activities matching caption in a flow and nested sub-flows.
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
		for _, nestedFlow := range getNestedFlows(actDoc) {
			findActivitiesRecursive(nestedFlow, caption, matches)
		}
	}
}

// getNestedFlows returns all sub-flows within an activity.
func getNestedFlows(actDoc bson.D) []bson.D {
	var flows []bson.D
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

// activityIndexMatch holds search result for findActivityIndex.
type activityIndexMatch struct {
	idx        int
	activities []any
	flow       bson.D
}

// findActivityIndex returns the index, activities array, and containing flow of an activity.
func (m *mprWorkflowMutator) findActivityIndex(caption string, atPosition int) (int, []any, bson.D, error) {
	flow := dGetDoc(m.rawData, "Flow")
	if flow == nil {
		return -1, nil, nil, fmt.Errorf("workflow has no Flow")
	}

	var matches []activityIndexMatch
	findActivityIndexRecursive(flow, caption, &matches)

	if len(matches) == 0 {
		return -1, nil, nil, fmt.Errorf("activity %q not found", caption)
	}
	pos := 0
	if atPosition > 0 {
		pos = atPosition - 1
	} else if len(matches) > 1 {
		return -1, nil, nil, fmt.Errorf("ambiguous activity %q — %d matches. Use @N to disambiguate", caption, len(matches))
	}
	if pos >= len(matches) {
		return -1, nil, nil, fmt.Errorf("activity %q at position %d not found (found %d matches)", caption, atPosition, len(matches))
	}
	am := matches[pos]
	return am.idx, am.activities, am.flow, nil
}

func findActivityIndexRecursive(flow bson.D, caption string, matches *[]activityIndexMatch) {
	activities := dGetArrayElements(dGet(flow, "Activities"))
	for i, elem := range activities {
		actDoc, ok := elem.(bson.D)
		if !ok {
			continue
		}
		actCaption := dGetString(actDoc, "Caption")
		actName := dGetString(actDoc, "Name")
		if actCaption == caption || actName == caption {
			*matches = append(*matches, activityIndexMatch{idx: i, activities: activities, flow: flow})
		}
		for _, nestedFlow := range getNestedFlows(actDoc) {
			findActivityIndexRecursive(nestedFlow, caption, matches)
		}
	}
}

// ---------------------------------------------------------------------------
// Internal helpers — name collection & deduplication
// ---------------------------------------------------------------------------

// collectAllActivityNames collects all activity names from the entire workflow BSON.
func (m *mprWorkflowMutator) collectAllActivityNames() map[string]bool {
	names := make(map[string]bool)
	flow := dGetDoc(m.rawData, "Flow")
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

// deduplicateNewActivityName ensures a new activity name doesn't conflict.
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

// ---------------------------------------------------------------------------
// Internal helpers — serialization
// ---------------------------------------------------------------------------

// serializeAndDedup serializes workflow activities to BSON, deduplicating names.
func (m *mprWorkflowMutator) serializeAndDedup(activities []workflows.WorkflowActivity) []any {
	existingNames := m.collectAllActivityNames()
	for _, act := range activities {
		deduplicateNewActivityName(act, existingNames)
	}

	result := make([]any, 0, len(activities))
	for _, act := range activities {
		bsonDoc := mpr.SerializeWorkflowActivity(act)
		if bsonDoc != nil {
			result = append(result, bsonDoc)
		}
	}
	return result
}

// buildSubFlowBson builds a Workflows$Flow BSON document from activities.
func (m *mprWorkflowMutator) buildSubFlowBson(activities []workflows.WorkflowActivity) bson.D {
	existingNames := m.collectAllActivityNames()
	for _, act := range activities {
		deduplicateNewActivityName(act, existingNames)
	}

	var subActsBson bson.A
	subActsBson = append(subActsBson, bsonArrayMarker)
	for _, act := range activities {
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
