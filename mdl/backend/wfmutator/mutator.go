// SPDX-License-Identifier: Apache-2.0

// Package wfmutator holds the engine-agnostic workflow ALTER logic: fine-grained
// mutation of a single workflow unit's raw BSON tree (set properties, insert/
// drop/replace activities, outcomes, paths, branches, and boundary events). It
// mirrors the pagemutator package — the bson.D manipulation is identical across
// storage engines; the two engine-specific steps (serializing a new activity to
// bson.D, and persisting the re-marshaled unit) are supplied via Deps. Both the
// MPR (sdk/mpr) backend and the modelsdk (codec) backend construct a Mutator with
// their own Deps.
package wfmutator

import (
	"fmt"
	"strings"

	"go.mongodb.org/mongo-driver/bson"

	"github.com/JordtenBulte-OLC/mxcli/mdl/backend/bsonnav"
	"github.com/JordtenBulte-OLC/mxcli/mdl/bsonutil"
	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/workflows"
)

// bsonArrayMarker is the Mendix BSON array type marker (storageListType 3 =
// reference/association lists). Contrast with int32(2) used for object lists.
const bsonArrayMarker = int32(3)

// Deps supplies the two engine-specific operations the mutator needs: converting
// a domain activity to its raw bson.D form, and writing the re-marshaled unit
// back to storage. The MPR backend routes these through sdk/mpr; the modelsdk
// backend routes them through the codec converters.
type Deps interface {
	// SerializeWorkflowActivity converts a domain WorkflowActivity to its raw
	// bson.D form. Returns nil for unsupported activity types.
	SerializeWorkflowActivity(a workflows.WorkflowActivity) bson.D
	// SaveUnit writes the (re-marshaled) unit bytes back to storage.
	SaveUnit(unitID string, contents []byte) error
}

// Mutator applies fine-grained mutations to a single workflow unit's BSON tree.
// All methods operate on the in-memory rawData; call Save to persist.
type Mutator struct {
	deps    Deps
	unitID  model.ID
	rawData bson.D
}

// New returns a Mutator over the given decoded workflow unit.
func New(rawData bson.D, unitID model.ID, deps Deps) *Mutator {
	return &Mutator{deps: deps, unitID: unitID, rawData: rawData}
}

// ---------------------------------------------------------------------------
// Top-level properties
// ---------------------------------------------------------------------------

func (m *Mutator) SetProperty(prop string, value string) error {
	switch strings.ToLower(prop) {
	case "display":
		wfName := bsonnav.DGetDoc(m.rawData, "WorkflowName")
		if wfName == nil {
			newName := bson.D{
				{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
				{Key: "$Type", Value: "Texts$Text"},
				{Key: "Text", Value: value},
			}
			m.rawData = append(m.rawData, bson.E{Key: "WorkflowName", Value: newName})
		} else {
			bsonnav.DSet(wfName, "Text", value)
		}
		bsonnav.DSet(m.rawData, "Title", value)
		return nil

	case "description":
		wfDesc := bsonnav.DGetDoc(m.rawData, "WorkflowDescription")
		if wfDesc == nil {
			newDesc := bson.D{
				{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
				{Key: "$Type", Value: "Texts$Text"},
				{Key: "Text", Value: value},
			}
			m.rawData = append(m.rawData, bson.E{Key: "WorkflowDescription", Value: newDesc})
		} else {
			bsonnav.DSet(wfDesc, "Text", value)
		}
		return nil

	case "export_level":
		bsonnav.DSet(m.rawData, "ExportLevel", value)
		return nil

	case "due_date":
		bsonnav.DSet(m.rawData, "DueDate", value)
		return nil

	default:
		return fmt.Errorf("unsupported workflow property: %s", prop)
	}
}

func (m *Mutator) SetPropertyWithEntity(prop string, value string, entity string) error {
	switch prop {
	case "overview_page":
		if value == "" {
			bsonnav.DSet(m.rawData, "AdminPage", nil)
		} else {
			pageRef := bson.D{
				{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
				{Key: "$Type", Value: "Workflows$PageReference"},
				{Key: "Page", Value: value},
			}
			bsonnav.DSet(m.rawData, "AdminPage", pageRef)
		}
		return nil

	case "parameter":
		if value == "" {
			for i, elem := range m.rawData {
				if elem.Key == "Parameter" {
					m.rawData[i].Value = nil
					return nil
				}
			}
			return nil
		}
		param := bsonnav.DGetDoc(m.rawData, "Parameter")
		if param != nil {
			bsonnav.DSet(param, "Entity", entity)
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
// Activity operations
// ---------------------------------------------------------------------------

func (m *Mutator) SetActivityProperty(activityRef string, atPos int, prop string, value string) error {
	actDoc, err := m.findActivityByCaption(activityRef, atPos)
	if err != nil {
		return err
	}

	switch strings.ToLower(prop) {
	case "page":
		taskPage := bsonnav.DGetDoc(actDoc, "TaskPage")
		if taskPage != nil {
			bsonnav.DSet(taskPage, "Page", value)
			return nil
		}
		pageRef := bson.D{
			{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
			{Key: "$Type", Value: "Workflows$PageReference"},
			{Key: "Page", Value: value},
		}
		if !bsonnav.DSet(actDoc, "TaskPage", pageRef) {
			actDoc = append(actDoc, bson.E{Key: "TaskPage", Value: pageRef})
			m.replaceActivity(actDoc)
		}
		return nil

	case "description":
		taskDesc := bsonnav.DGetDoc(actDoc, "TaskDescription")
		if taskDesc != nil {
			bsonnav.DSet(taskDesc, "Text", value)
		}
		return nil

	case "targeting_microflow":
		userTargeting := bson.D{
			{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
			{Key: "$Type", Value: "Workflows$MicroflowUserTargeting"},
			{Key: "Microflow", Value: value},
		}
		bsonnav.DSet(actDoc, "UserTargeting", userTargeting)
		return nil

	case "targeting_xpath":
		userTargeting := bson.D{
			{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
			{Key: "$Type", Value: "Workflows$XPathUserTargeting"},
			{Key: "XPathConstraint", Value: value},
		}
		bsonnav.DSet(actDoc, "UserTargeting", userTargeting)
		return nil

	case "due_date":
		bsonnav.DSet(actDoc, "DueDate", value)
		return nil

	default:
		return fmt.Errorf("unsupported activity property: %s", prop)
	}
}

func (m *Mutator) InsertAfterActivity(activityRef string, atPos int, activities []workflows.WorkflowActivity) error {
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

	bsonnav.DSetArray(containingFlow, "Activities", newArr)
	return nil
}

func (m *Mutator) DropActivity(activityRef string, atPos int) error {
	idx, acts, containingFlow, err := m.findActivityIndex(activityRef, atPos)
	if err != nil {
		return err
	}

	newArr := make([]any, 0, len(acts)-1)
	newArr = append(newArr, acts[:idx]...)
	newArr = append(newArr, acts[idx+1:]...)

	bsonnav.DSetArray(containingFlow, "Activities", newArr)
	return nil
}

func (m *Mutator) ReplaceActivity(activityRef string, atPos int, activities []workflows.WorkflowActivity) error {
	idx, acts, containingFlow, err := m.findActivityIndex(activityRef, atPos)
	if err != nil {
		return err
	}

	newBsonActs := m.serializeAndDedup(activities)

	newArr := make([]any, 0, len(acts)-1+len(newBsonActs))
	newArr = append(newArr, acts[:idx]...)
	newArr = append(newArr, newBsonActs...)
	newArr = append(newArr, acts[idx+1:]...)

	bsonnav.DSetArray(containingFlow, "Activities", newArr)
	return nil
}

// ---------------------------------------------------------------------------
// Outcome operations
// ---------------------------------------------------------------------------

func (m *Mutator) InsertOutcome(activityRef string, atPos int, outcomeName string, activities []workflows.WorkflowActivity) error {
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

	outcomes := bsonnav.DGetArrayElements(bsonnav.DGet(actDoc, "Outcomes"))
	outcomes = append(outcomes, outcomeDoc)
	bsonnav.DSetArray(actDoc, "Outcomes", outcomes)
	return nil
}

func (m *Mutator) DropOutcome(activityRef string, atPos int, outcomeName string) error {
	actDoc, err := m.findActivityByCaption(activityRef, atPos)
	if err != nil {
		return err
	}

	outcomes := bsonnav.DGetArrayElements(bsonnav.DGet(actDoc, "Outcomes"))
	found := false
	var kept []any
	for _, elem := range outcomes {
		oDoc, ok := elem.(bson.D)
		if !ok {
			kept = append(kept, elem)
			continue
		}
		value := bsonnav.DGetString(oDoc, "Value")
		typeName := bsonnav.DGetString(oDoc, "$Type")
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
	bsonnav.DSetArray(actDoc, "Outcomes", kept)
	return nil
}

// ---------------------------------------------------------------------------
// Path operations (parallel split)
// ---------------------------------------------------------------------------

func (m *Mutator) InsertPath(activityRef string, atPos int, pathCaption string, activities []workflows.WorkflowActivity) error {
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

	outcomes := bsonnav.DGetArrayElements(bsonnav.DGet(actDoc, "Outcomes"))
	outcomes = append(outcomes, pathDoc)
	bsonnav.DSetArray(actDoc, "Outcomes", outcomes)
	return nil
}

func (m *Mutator) DropPath(activityRef string, atPos int, pathCaption string) error {
	actDoc, err := m.findActivityByCaption(activityRef, atPos)
	if err != nil {
		return err
	}

	outcomes := bsonnav.DGetArrayElements(bsonnav.DGet(actDoc, "Outcomes"))
	if pathCaption == "" && len(outcomes) > 0 {
		outcomes = outcomes[:len(outcomes)-1]
		bsonnav.DSetArray(actDoc, "Outcomes", outcomes)
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
	bsonnav.DSetArray(actDoc, "Outcomes", newOutcomes)
	return nil
}

// ---------------------------------------------------------------------------
// Branch operations (exclusive split)
// ---------------------------------------------------------------------------

func (m *Mutator) InsertBranch(activityRef string, atPos int, condition string, activities []workflows.WorkflowActivity) error {
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

	outcomes := bsonnav.DGetArrayElements(bsonnav.DGet(actDoc, "Outcomes"))
	outcomes = append(outcomes, outcomeDoc)
	bsonnav.DSetArray(actDoc, "Outcomes", outcomes)
	return nil
}

func (m *Mutator) DropBranch(activityRef string, atPos int, branchName string) error {
	actDoc, err := m.findActivityByCaption(activityRef, atPos)
	if err != nil {
		return err
	}

	outcomes := bsonnav.DGetArrayElements(bsonnav.DGet(actDoc, "Outcomes"))
	found := false
	var kept []any
	for _, elem := range outcomes {
		oDoc, ok := elem.(bson.D)
		if !ok {
			kept = append(kept, elem)
			continue
		}
		if !found {
			typeName := bsonnav.DGetString(oDoc, "$Type")
			switch strings.ToLower(branchName) {
			case "true":
				if typeName == "Workflows$BooleanConditionOutcome" {
					if v, ok := bsonnav.DGet(oDoc, "Value").(bool); ok && v {
						found = true
						continue
					}
				}
			case "false":
				if typeName == "Workflows$BooleanConditionOutcome" {
					if v, ok := bsonnav.DGet(oDoc, "Value").(bool); ok && !v {
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
				value := bsonnav.DGetString(oDoc, "Value")
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
	bsonnav.DSetArray(actDoc, "Outcomes", kept)
	return nil
}

// ---------------------------------------------------------------------------
// Boundary event operations
// ---------------------------------------------------------------------------

func (m *Mutator) InsertBoundaryEvent(activityRef string, atPos int, eventType string, delay string, activities []workflows.WorkflowActivity) error {
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

	events := bsonnav.DGetArrayElements(bsonnav.DGet(actDoc, "BoundaryEvents"))
	events = append(events, eventDoc)
	bsonnav.DSetArray(actDoc, "BoundaryEvents", events)
	return nil
}

func (m *Mutator) DropBoundaryEvent(activityRef string, atPos int) error {
	actDoc, err := m.findActivityByCaption(activityRef, atPos)
	if err != nil {
		return err
	}

	events := bsonnav.DGetArrayElements(bsonnav.DGet(actDoc, "BoundaryEvents"))
	if len(events) == 0 {
		return fmt.Errorf("activity %q has no boundary events", activityRef)
	}

	// Drop the first boundary event silently.
	bsonnav.DSetArray(actDoc, "BoundaryEvents", events[1:])
	return nil
}

// ---------------------------------------------------------------------------
// Save
// ---------------------------------------------------------------------------

func (m *Mutator) Save() error {
	outBytes, err := bson.Marshal(m.rawData)
	if err != nil {
		return fmt.Errorf("marshal modified workflow: %w", err)
	}
	return m.deps.SaveUnit(string(m.unitID), outBytes)
}

// ---------------------------------------------------------------------------
// Internal helpers — activity search
// ---------------------------------------------------------------------------

// replaceActivity replaces an activity document in the workflow's BSON tree by
// matching on $ID. Needed when appending new keys to an activity document,
// because the slice header returned by findActivityByCaption cannot propagate
// appends back to the parent bson.A.
func (m *Mutator) replaceActivity(updated bson.D) {
	actID := bsonnav.ExtractBinaryIDFromDoc(bsonnav.DGet(updated, "$ID"))
	if actID == "" {
		return
	}
	flow := bsonnav.DGetDoc(m.rawData, "Flow")
	if flow == nil {
		return
	}
	replaceActivityRecursive(flow, actID, updated)
}

func replaceActivityRecursive(flow bson.D, actID string, updated bson.D) bool {
	elements := bsonnav.DGetArrayElements(bsonnav.DGet(flow, "Activities"))
	for i, elem := range elements {
		actDoc, ok := elem.(bson.D)
		if !ok {
			continue
		}
		if bsonnav.ExtractBinaryIDFromDoc(bsonnav.DGet(actDoc, "$ID")) == actID {
			elements[i] = updated
			return true
		}
		for _, nestedFlow := range getNestedFlows(actDoc) {
			if replaceActivityRecursive(nestedFlow, actID, updated) {
				return true
			}
		}
	}
	return false
}

// findActivityByCaption searches the workflow for an activity matching caption.
func (m *Mutator) findActivityByCaption(caption string, atPosition int) (bson.D, error) {
	flow := bsonnav.DGetDoc(m.rawData, "Flow")
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
	activities := bsonnav.DGetArrayElements(bsonnav.DGet(flow, "Activities"))
	for _, elem := range activities {
		actDoc, ok := elem.(bson.D)
		if !ok {
			continue
		}
		actCaption := bsonnav.DGetString(actDoc, "Caption")
		actName := bsonnav.DGetString(actDoc, "Name")
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
	outcomes := bsonnav.DGetArrayElements(bsonnav.DGet(actDoc, "Outcomes"))
	for _, o := range outcomes {
		oDoc, ok := o.(bson.D)
		if !ok {
			continue
		}
		if f := bsonnav.DGetDoc(oDoc, "Flow"); f != nil {
			flows = append(flows, f)
		}
	}
	events := bsonnav.DGetArrayElements(bsonnav.DGet(actDoc, "BoundaryEvents"))
	for _, e := range events {
		eDoc, ok := e.(bson.D)
		if !ok {
			continue
		}
		if f := bsonnav.DGetDoc(eDoc, "Flow"); f != nil {
			flows = append(flows, f)
		}
	}
	return flows
}

// activityIndexMatch holds a search result for findActivityIndex.
type activityIndexMatch struct {
	idx        int
	activities []any
	flow       bson.D
}

// findActivityIndex returns the index, activities array, and containing flow of an activity.
func (m *Mutator) findActivityIndex(caption string, atPosition int) (int, []any, bson.D, error) {
	flow := bsonnav.DGetDoc(m.rawData, "Flow")
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
	activities := bsonnav.DGetArrayElements(bsonnav.DGet(flow, "Activities"))
	for i, elem := range activities {
		actDoc, ok := elem.(bson.D)
		if !ok {
			continue
		}
		actCaption := bsonnav.DGetString(actDoc, "Caption")
		actName := bsonnav.DGetString(actDoc, "Name")
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
func (m *Mutator) collectAllActivityNames() map[string]bool {
	names := make(map[string]bool)
	flow := bsonnav.DGetDoc(m.rawData, "Flow")
	if flow != nil {
		collectNamesRecursive(flow, names)
	}
	return names
}

func collectNamesRecursive(flow bson.D, names map[string]bool) {
	activities := bsonnav.DGetArrayElements(bsonnav.DGet(flow, "Activities"))
	for _, elem := range activities {
		actDoc, ok := elem.(bson.D)
		if !ok {
			continue
		}
		if name := bsonnav.DGetString(actDoc, "Name"); name != "" {
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
	if name == "" {
		return
	}
	if !existingNames[name] {
		existingNames[name] = true
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
// Internal helpers — serialization (via Deps)
// ---------------------------------------------------------------------------

// serializeAndDedup serializes workflow activities to BSON, deduplicating names.
func (m *Mutator) serializeAndDedup(activities []workflows.WorkflowActivity) []any {
	existingNames := m.collectAllActivityNames()
	for _, act := range activities {
		deduplicateNewActivityName(act, existingNames)
	}

	result := make([]any, 0, len(activities))
	for _, act := range activities {
		bsonDoc := m.deps.SerializeWorkflowActivity(act)
		if bsonDoc != nil {
			result = append(result, bsonDoc)
		}
	}
	return result
}

// buildSubFlowBson builds a Workflows$Flow BSON document from activities.
func (m *Mutator) buildSubFlowBson(activities []workflows.WorkflowActivity) bson.D {
	existingNames := m.collectAllActivityNames()
	for _, act := range activities {
		deduplicateNewActivityName(act, existingNames)
	}

	var subActsBson bson.A
	subActsBson = append(subActsBson, bsonArrayMarker)
	for _, act := range activities {
		bsonDoc := m.deps.SerializeWorkflowActivity(act)
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
