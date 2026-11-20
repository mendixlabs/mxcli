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

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/backend/bsonnav"
	"github.com/mendixlabs/mxcli/mdl/backend/wfnames"
	"github.com/mendixlabs/mxcli/mdl/bsonutil"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/workflows"
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

	case "annotation":
		return m.setStartNote(value)

	default:
		return fmt.Errorf("unsupported workflow property: %s", prop)
	}
}

// setStartNote sets the note on the workflow's start, the header's
// `annotation '…'` (ako/mxcli#1717): the start activity's, where Studio Pro
// stores and shows it. The workflow's own Annotation — shown nowhere, and where
// mxcli wrote the note before — is cleared, or describe would still print the
// note the statement replaced or removed.
//
// A workflow whose main flow has no start activity is one stored below Mendix
// 11.3, which keeps the start implicit and the note as the workflow's own
// Annotation (ako/mxcli#1754); the note is set there.
func (m *Mutator) setStartNote(text string) error {
	var start bson.D
	for _, el := range bsonnav.DGetArrayElements(bsonnav.DGet(bsonnav.DGetDoc(m.rawData, "Flow"), "Activities")) {
		if d, ok := el.(bson.D); ok && bsonnav.DGetString(d, "$Type") == "Workflows$StartWorkflowActivity" {
			start = d
			break
		}
	}
	if start == nil {
		if setNote(m.rawData, "the workflow", text) == nil {
			return nil
		}
		return fmt.Errorf("the workflow has no start activity to hold its note")
	}
	if err := setNote(start, "the workflow's start activity", text); err != nil {
		return err
	}
	bsonnav.DSet(m.rawData, "Annotation", nil)
	return nil
}

// setNote writes the note Studio Pro attaches to an activity (ako/mxcli#1717):
// a Workflows$Annotation holding Description — the shape CREATE writes for
// `@annotation '…'` — or null for "".
// An existing note keeps its $ID. A document without the Annotation slot is
// refused: every type with a note stores the slot, null when empty (11.15.0),
// so a missing one is a type or version that has no note to write.
func setNote(owner bson.D, what, text string) error {
	for i := range owner {
		if owner[i].Key != "Annotation" {
			continue
		}
		note, _ := owner[i].Value.(bson.D)
		switch {
		case text == "":
			owner[i].Value = nil
		case note != nil && bsonnav.DSet(note, "Description", text):
			// Edited in place, keeping the note's $ID.
		default:
			owner[i].Value = bson.D{
				{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
				{Key: "$Type", Value: "Workflows$Annotation"},
				{Key: "Description", Value: text},
			}
		}
		return nil
	}
	return fmt.Errorf("%s has no annotation to set", what)
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

// templateKeys maps SetTemplateParameters' property to the key of the
// Microflows$StringTemplate it fills, on the workflow and on a user task.
var templateKeys = map[string][2]string{
	"display":     {"WorkflowName", ""},
	"description": {"WorkflowDescription", "TaskDescription"},
	"task_name":   {"", "TaskName"},
}

// SetTemplateParameters replaces a text's parameters with one
// Microflows$TemplateParameter per expression — the shape the create path
// writes (ako/mxcli#1714).
func (m *Mutator) SetTemplateParameters(activityRef string, atPos int, prop string, params []string) error {
	keys, ok := templateKeys[strings.ToLower(prop)]
	if !ok {
		return fmt.Errorf("%s is not a text with parameters", prop)
	}
	owner, key := m.rawData, keys[0]
	if activityRef != "" {
		actDoc, err := m.findActivityByCaption(activityRef, atPos)
		if err != nil {
			return err
		}
		owner, key = actDoc, keys[1]
	}
	if key == "" {
		return fmt.Errorf("%s has no parameters here", prop)
	}
	tmpl := bsonnav.DGetDoc(owner, key)
	if tmpl == nil {
		if len(params) == 0 {
			return nil
		}
		return fmt.Errorf("%s has no %s to give parameters", orWorkflow(activityRef), key)
	}
	list := bson.A{int32(2)}
	for _, p := range params {
		list = append(list, bson.D{
			{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
			{Key: "$Type", Value: "Microflows$TemplateParameter"},
			{Key: "Expression", Value: p},
		})
	}
	if !bsonnav.DSet(tmpl, "Parameters", list) {
		return fmt.Errorf("%s's %s has no parameter list", orWorkflow(activityRef), key)
	}
	return nil
}

// orWorkflow names what SetTemplateParameters edits, for its errors.
func orWorkflow(activityRef string) string {
	if activityRef == "" {
		return "the workflow"
	}
	return "activity " + activityRef
}

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

	case "task_name":
		taskName := bsonnav.DGetDoc(actDoc, "TaskName")
		if taskName == nil {
			return fmt.Errorf("activity %q has no task name", activityRef)
		}
		bsonnav.DSet(taskName, "Text", value)
		return nil

	case "targeting_microflow":
		m.setUserTargeting(actDoc, bson.D{
			{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
			{Key: "$Type", Value: "Workflows$MicroflowUserTargeting"},
			{Key: "Microflow", Value: value},
		})
		return nil

	case "targeting_xpath":
		m.setUserTargeting(actDoc, bson.D{
			{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
			{Key: "$Type", Value: "Workflows$XPathUserTargeting"},
			{Key: "XPathConstraint", Value: value},
		})
		return nil

	case "due_date":
		bsonnav.DSet(actDoc, "DueDate", value)
		return nil

	case "annotation":
		switch bsonnav.DGetString(actDoc, "$Type") {
		case "Workflows$StartWorkflowActivity":
			// The workflow's note, which describe prints in the header.
			return m.setStartNote(value)
		case "Workflows$EndWorkflowActivity", "Workflows$EndOfParallelSplitPathActivity", "Workflows$EndOfBoundaryEventPathActivity":
			// Studio Pro's workflow editor draws no note on an end (11.15.0),
			// and describe prints none: a note there is one nothing shows.
			return fmt.Errorf("activity %q is an end of the flow, which shows no note", activityRef)
		}
		return setNote(actDoc, fmt.Sprintf("activity %q", activityRef), value)

	default:
		return fmt.Errorf("unsupported activity property: %s", prop)
	}
}

func (m *Mutator) InsertAfterActivity(activityRef string, atPos int, activities []workflows.WorkflowActivity) error {
	idx, acts, containingFlow, err := m.findActivityIndex(activityRef, atPos)
	if err != nil {
		return err
	}

	newBsonActs := m.serializeAndDedup(activities, "")

	insertIdx := idx + 1
	newArr := make([]any, 0, len(acts)+len(newBsonActs))
	newArr = append(newArr, acts[:insertIdx]...)
	newArr = append(newArr, newBsonActs...)
	newArr = append(newArr, acts[insertIdx:]...)

	bsonnav.DSetArray(containingFlow, "Activities", newArr)
	return nil
}

// InsertBeforeActivity inserts activities just before the referenced one, in
// the same flow: the generic ALTER's `insert before X { … }`.
func (m *Mutator) InsertBeforeActivity(activityRef string, atPos int, activities []workflows.WorkflowActivity) error {
	idx, acts, containingFlow, err := m.findActivityIndex(activityRef, atPos)
	if err != nil {
		return err
	}

	newBsonActs := m.serializeAndDedup(activities, "")

	newArr := make([]any, 0, len(acts)+len(newBsonActs))
	newArr = append(newArr, acts[:idx]...)
	newArr = append(newArr, newBsonActs...)
	newArr = append(newArr, acts[idx:]...)

	bsonnav.DSetArray(containingFlow, "Activities", newArr)
	return nil
}

// ResolveAlterTarget is the workflow's backend.AlterTargetResolver: the
// activities whose name or caption the target names, in describe order, under
// the rule backend.ResolveWorkflowActivityTarget shares with every backend.
func (m *Mutator) ResolveAlterTarget(t backend.AlterTarget) (backend.AlterTargetMatch, error) {
	if err := backend.CheckWorkflowAlterTarget(t); err != nil {
		return backend.AlterTargetMatch{}, err
	}
	flow := bsonnav.DGetDoc(m.rawData, "Flow")
	if flow == nil {
		return backend.AlterTargetMatch{}, fmt.Errorf("workflow has no Flow")
	}
	var found []bson.D
	findActivitiesRecursive(flow, backend.WorkflowTargetText(t), &found)
	candidates := make([]backend.WorkflowActivityCandidate, 0, len(found))
	for _, d := range found {
		candidates = append(candidates, backend.WorkflowActivityCandidate{
			Name:        bsonnav.DGetString(d, "Name"),
			Caption:     bsonnav.DGetString(d, "Caption"),
			StorageType: bsonnav.DGetString(d, "$Type"),
		})
	}
	return backend.ResolveWorkflowActivityTarget(t, candidates)
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

	// Free the outgoing activity's name for the replacement. It is read off the
	// resolved element rather than from activityRef, which may be a caption.
	var outgoing string
	if old, ok := acts[idx].(bson.D); ok {
		outgoing = bsonnav.DGetString(old, "Name")
	}
	newBsonActs := m.serializeAndDedup(activities, outgoing)

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
	actDoc, err := m.findMemberTarget(activityRef, atPos, "insert outcome", backend.WorkflowUserTaskOutcomes)
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
	kept, err := dropOneOutcome(outcomes, "outcome", outcomeName, activityRef, func(oDoc bson.D) bool {
		// Only an outcome that stores a string value can be addressed by one:
		// a path, a void or a boolean outcome reads back as "" and would match
		// an empty value (found reviewing ako/mxcli#791).
		value, hasValue := bsonnav.DGet(oDoc, "Value").(string)
		if hasValue && value == outcomeName {
			return true
		}
		return strings.EqualFold(outcomeName, "Default") && bsonnav.DGetString(oDoc, "$Type") == "Workflows$VoidConditionOutcome"
	})
	if err != nil {
		return err
	}
	bsonnav.DSetArray(actDoc, "Outcomes", kept)
	return nil
}

// dropOneOutcome removes the single outcome that matches. A value is the only
// address an outcome has, and Mendix does not keep it unique: with two outcomes
// of the same value, dropping the first was a guess at which one was meant, so
// it is refused instead (ako/mxcli#808).
func dropOneOutcome(outcomes []any, kind, value, activityRef string, matches func(bson.D) bool) ([]any, error) {
	at, n := -1, 0
	for i, elem := range outcomes {
		if oDoc, ok := elem.(bson.D); ok && matches(oDoc) {
			if at < 0 {
				at = i
			}
			n++
		}
	}
	if n == 0 {
		return nil, fmt.Errorf("%s %q not found on activity %q", kind, value, activityRef)
	}
	if n > 1 {
		return nil, fmt.Errorf("%s %q on activity %q is ambiguous: %d outcomes have that value. Nothing was written", kind, value, activityRef, n)
	}
	kept := make([]any, 0, len(outcomes)-1)
	kept = append(kept, outcomes[:at]...)
	return append(kept, outcomes[at+1:]...), nil
}

// ---------------------------------------------------------------------------
// Path operations (parallel split)
// ---------------------------------------------------------------------------

func (m *Mutator) InsertPath(activityRef string, atPos int, pathCaption string, activities []workflows.WorkflowActivity) error {
	actDoc, err := m.findMemberTarget(activityRef, atPos, "insert path", backend.WorkflowParallelPaths)
	if err != nil {
		return err
	}

	pathDoc := bson.D{
		{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
		{Key: "$Type", Value: "Workflows$ParallelSplitOutcome"},
	}

	// A path must end with Mendix's end-of-path marker or the engine skips its
	// contents at runtime (workflows.EndParallelSplitPath) — an empty path too.
	ended := workflows.EndParallelSplitPath(&workflows.Flow{Activities: activities}, func() model.ID {
		return model.ID(bsonutil.BsonBinaryToID(bsonutil.NewIDBsonBinary()))
	})
	pathDoc = append(pathDoc, bson.E{Key: "Flow", Value: m.buildSubFlowBson(ended.Activities)})

	pathDoc = append(pathDoc, bson.E{Key: "PersistentId", Value: bsonutil.NewIDBsonBinary()})

	outcomes := bsonnav.DGetArrayElements(bsonnav.DGet(actDoc, "Outcomes"))
	outcomes = append(outcomes, pathDoc)
	bsonnav.DSetArray(actDoc, "Outcomes", outcomes)
	return nil
}

// DropPath removes a parallel split's path by its number ("Path N"). Only a
// parallel split has paths: on any other activity the number would index that
// activity's outcomes instead — `drop userTask1 path 1` deleted the user task's
// first outcome (ako/mxcli#791) — so the target's kind is checked first, and an
// address that names no single path is refused rather than resolved.
func (m *Mutator) DropPath(activityRef string, atPos int, pathCaption string) error {
	actDoc, err := m.findMemberTarget(activityRef, atPos, "drop path", backend.WorkflowParallelPaths)
	if err != nil {
		return err
	}

	outcomes := bsonnav.DGetArrayElements(bsonnav.DGet(actDoc, "Outcomes"))
	pathIdx, err := backend.ParallelPathIndex(pathCaption, activityRef, len(outcomes))
	if err != nil {
		return err
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
	actDoc, err := m.findMemberTarget(activityRef, atPos, "insert outcome", backend.WorkflowConditionOutcomes)
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

	// Every outcome carries a Flow (empty when the branch is) and a
	// PersistentId: Studio Pro stores both on all of them (ako/TestApp 11.14.0),
	// and an outcome without them is one Mendix's merge engine cannot compare
	// with Studio Pro's next save of the workflow (mendixlabs/mxcli#1373).
	outcomeDoc = append(outcomeDoc,
		bson.E{Key: "Flow", Value: m.buildSubFlowBson(activities)},
		bson.E{Key: "PersistentId", Value: bsonutil.NewIDBsonBinary()})

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
	kept, err := dropOneOutcome(outcomes, "branch", branchName, activityRef, func(oDoc bson.D) bool {
		typeName := bsonnav.DGetString(oDoc, "$Type")
		switch strings.ToLower(branchName) {
		case "true", "false":
			v, ok := bsonnav.DGet(oDoc, "Value").(bool)
			return typeName == "Workflows$BooleanConditionOutcome" && ok && v == strings.EqualFold(branchName, "true")
		case "default":
			return typeName == "Workflows$VoidConditionOutcome"
		default:
			value, ok := bsonnav.DGet(oDoc, "Value").(string)
			return ok && value == branchName
		}
	})
	if err != nil {
		return err
	}
	bsonnav.DSetArray(actDoc, "Outcomes", kept)
	return nil
}

// ---------------------------------------------------------------------------
// Boundary event operations
// ---------------------------------------------------------------------------

func (m *Mutator) InsertBoundaryEvent(activityRef string, atPos int, eventType string, delay string, caption string, recurrence *workflows.Recurrence, activities []workflows.WorkflowActivity) error {
	actDoc, err := m.findMemberTarget(activityRef, atPos, "insert boundary event", backend.WorkflowBoundaryEvents)
	if err != nil {
		return err
	}

	// Unknown kinds used to fall through to an interrupting timer. A notification
	// event needs a name this op does not carry, so it is refused too.
	typeName, ok := workflows.BoundaryEventStorageType(eventType)
	if !ok || (&workflows.BoundaryEvent{EventType: eventType}).IsNotification() {
		return fmt.Errorf("insert boundary event: %q boundary events cannot be inserted by ALTER WORKFLOW", eventType)
	}

	eventDoc := bson.D{
		{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
		{Key: "$Type", Value: typeName},
		{Key: "Caption", Value: caption},
	}

	if delay != "" {
		eventDoc = append(eventDoc, bson.E{Key: "FirstExecutionTime", Value: delay})
	}

	// End the path with Mendix's marker (workflows.EndBoundaryEventPath) — an
	// unterminated non-interrupting path builds, then stops the runtime starting.
	ended := workflows.EndBoundaryEventPath(&workflows.Flow{Activities: activities}, func() model.ID {
		return model.ID(bsonutil.BsonBinaryToID(bsonutil.NewIDBsonBinary()))
	})
	eventDoc = append(eventDoc, bson.E{Key: "Flow", Value: m.buildSubFlowBson(ended.Activities)})

	eventDoc = append(eventDoc, bson.E{Key: "PersistentId", Value: bsonutil.NewIDBsonBinary()})

	if typeName == "Workflows$NonInterruptingTimerBoundaryEvent" {
		var rec any // null: fires once
		if recurrence != nil {
			rec = bson.D{
				{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
				{Key: "$Type", Value: "Workflows$LinearRecurrence"},
				{Key: "Interval", Value: int32(recurrence.Interval)},
				{Key: "IntervalType", Value: recurrence.IntervalType},
				{Key: "MaxExecutions", Value: int32(recurrence.MaxExecutions)},
			}
		}
		eventDoc = append(eventDoc, bson.E{Key: "Recurrence", Value: rec})
	} else if recurrence != nil {
		return fmt.Errorf("insert boundary event: only a non-interrupting timer repeats")
	}

	events := bsonnav.DGetArrayElements(bsonnav.DGet(actDoc, "BoundaryEvents"))
	// mx check reports CE6696 on a sixth boundary event (ako/mxcli#2242). Only
	// here is the stored count known, so the static MDL-WF23 check cannot
	// catch an insert onto an activity that already has five.
	if len(events) >= workflows.MaxBoundaryEventsPerActivity {
		return fmt.Errorf("insert boundary event: activity '%s' already has %d boundary events, the most an activity may have (mx check CE6696) [MDL-WF23]",
			activityRef, len(events))
	}
	events = append(events, eventDoc)
	bsonnav.DSetArray(actDoc, "BoundaryEvents", events)
	return nil
}

func (m *Mutator) DropBoundaryEvent(activityRef string, atPos int, index int) error {
	actDoc, err := m.findActivityByCaption(activityRef, atPos)
	if err != nil {
		return err
	}

	events := bsonnav.DGetArrayElements(bsonnav.DGet(actDoc, "BoundaryEvents"))
	if len(events) == 0 {
		return fmt.Errorf("activity %q has no boundary events", activityRef)
	}
	at, err := backend.BoundaryEventIndex(activityRef, index, len(events))
	if err != nil {
		return err
	}
	bsonnav.DSetArray(actDoc, "BoundaryEvents", append(append([]any{}, events[:at]...), events[at+1:]...))
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
// setUserTargeting replaces a user task's targeting with targeting, in the
// current (11.2) shape and in the place of whichever key stores it: below 11.2
// that is UserSource (ako/mxcli#1771), which the write lowers it back to
// (canon.LowerToProjectShape). Setting UserTargeting only where it already
// existed changed nothing on such a task, and the statement still reported
// success.
func (m *Mutator) setUserTargeting(actDoc bson.D, targeting bson.D) {
	for i := range actDoc {
		if actDoc[i].Key == "UserTargeting" || actDoc[i].Key == "UserSource" {
			actDoc[i] = bson.E{Key: "UserTargeting", Value: targeting}
			return
		}
	}
	m.replaceActivity(append(actDoc, bson.E{Key: "UserTargeting", Value: targeting}))
}

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

// findMemberTarget resolves the activity a member op addresses and refuses it
// when its document does not declare the list the op reads or writes
// (backend.CheckWorkflowMemberList).
func (m *Mutator) findMemberTarget(ref string, atPos int, op string, list backend.WorkflowMemberList) (bson.D, error) {
	actDoc, err := m.findActivityByCaption(ref, atPos)
	if err != nil {
		return nil, err
	}
	if err := backend.CheckWorkflowMemberList(bsonnav.DGetString(actDoc, "$Type"), ref, op, list); err != nil {
		return nil, err
	}
	return actDoc, nil
}

// findActivityByCaption searches the workflow for an activity matching caption.
func (m *Mutator) findActivityByCaption(caption string, atPosition int) (bson.D, error) {
	flow := bsonnav.DGetDoc(m.rawData, "Flow")
	if flow == nil {
		return nil, fmt.Errorf("workflow has no Flow")
	}

	var all []bson.D
	findActivitiesRecursive(flow, caption, &all)
	matches := preferNameMatches(all, atPosition, func(d bson.D) bool {
		return bsonnav.DGetString(d, "Name") == caption
	})

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

// preferNameMatches narrows the activities a reference matched. Without @N, the
// ones whose Name equals the reference win when there are any: a name is an
// activity's identity (deduplicated within the workflow), while a caption is a
// label that may repeat another activity's name — buildJumpTo defaults a jump's
// caption to its target's name — so pooling the two made every jump target
// ambiguous. With @N every match counts, in the depth-first order DESCRIBE
// walks, so an existing `ACT_Process@2` keeps addressing what it always did.
func preferNameMatches[T any](all []T, atPosition int, isName func(T) bool) []T {
	if atPosition > 0 {
		return all
	}
	var named []T
	for _, m := range all {
		if isName(m) {
			named = append(named, m)
		}
	}
	if len(named) > 0 {
		return named
	}
	return all
}

// findActivitiesRecursive collects all activities whose name or caption equals
// ref in a flow and nested sub-flows, in document order.
func findActivitiesRecursive(flow bson.D, ref string, matches *[]bson.D) {
	activities := bsonnav.DGetArrayElements(bsonnav.DGet(flow, "Activities"))
	for _, elem := range activities {
		actDoc, ok := elem.(bson.D)
		if !ok {
			continue
		}
		if bsonnav.DGetString(actDoc, "Name") == ref || bsonnav.DGetString(actDoc, "Caption") == ref {
			*matches = append(*matches, actDoc)
		}
		for _, nestedFlow := range getNestedFlows(actDoc) {
			findActivitiesRecursive(nestedFlow, ref, matches)
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

	var all []activityIndexMatch
	findActivityIndexRecursive(flow, caption, &all)
	matches := preferNameMatches(all, atPosition, func(am activityIndexMatch) bool {
		actDoc, _ := am.activities[am.idx].(bson.D)
		return bsonnav.DGetString(actDoc, "Name") == caption
	})

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

func findActivityIndexRecursive(flow bson.D, ref string, matches *[]activityIndexMatch) {
	activities := bsonnav.DGetArrayElements(bsonnav.DGet(flow, "Activities"))
	for i, elem := range activities {
		actDoc, ok := elem.(bson.D)
		if !ok {
			continue
		}
		if bsonnav.DGetString(actDoc, "Name") == ref || bsonnav.DGetString(actDoc, "Caption") == ref {
			*matches = append(*matches, activityIndexMatch{idx: i, activities: activities, flow: flow})
		}
		for _, nestedFlow := range getNestedFlows(actDoc) {
			findActivityIndexRecursive(nestedFlow, ref, matches)
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

// ---------------------------------------------------------------------------
// Internal helpers — serialization (via Deps)
// ---------------------------------------------------------------------------

// serializeAndDedup serializes workflow activities to BSON, deduplicating their
// names against the ones already in the workflow.
//
// freeName, when non-empty, names an activity that this same operation is
// removing, so it is not really taken. REPLACE ACTIVITY needs it: the
// replacement reusing the original's name is the ordinary in-place edit, not a
// collision, and treating it as one renamed every same-name replace to Name_2 —
// compounding on each re-run (Name_2_2, Name_2_2_2, ...) so an ALTER that
// changed nothing still mutated the model (issue #944). INSERT passes "": there
// the incoming activity is genuinely additional, and every existing name stands.
func (m *Mutator) serializeAndDedup(activities []workflows.WorkflowActivity, freeName string) []any {
	taken := m.collectAllActivityNames()
	delete(taken, freeName) // no-op for ""
	wfnames.Dedup(activities, taken)

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
	// An outcome/path/branch/boundary-event sub-flow is always new content, so
	// every existing name stands (unlike ReplaceActivity — see serializeAndDedup).
	wfnames.Dedup(activities, m.collectAllActivityNames())

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
