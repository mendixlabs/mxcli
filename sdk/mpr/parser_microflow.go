// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func (r *Reader) parseMicroflow(unitID, containerID string, contents []byte) (*microflows.Microflow, error) {
	contents, err := r.resolveContents(unitID, contents)
	if err != nil {
		return nil, err
	}
	return ParseMicroflowBSON(contents, model.ID(unitID), model.ID(containerID))
}

// ParseMicroflowBSON parses raw microflow BSON bytes into a Microflow.
// Unlike (*Reader).parseMicroflow it does not require a Reader, so it can
// parse arbitrary blobs (e.g. historical versions read via `git show`).
func ParseMicroflowBSON(contents []byte, unitID, containerID model.ID) (*microflows.Microflow, error) {
	var raw map[string]any
	if err := bson.Unmarshal(contents, &raw); err != nil {
		return nil, fmt.Errorf("failed to unmarshal BSON: %w", err)
	}
	return ParseMicroflowFromRaw(raw, unitID, containerID), nil
}

// ParseMicroflowFromRaw builds a Microflow from an already-unmarshalled BSON map.
// Useful when the caller already has the decoded map (e.g. diff-local).
func ParseMicroflowFromRaw(raw map[string]any, unitID, containerID model.ID) *microflows.Microflow {
	mf := &microflows.Microflow{}
	mf.ID = unitID
	mf.TypeName = "Microflows$Microflow"
	mf.ContainerID = containerID

	if name, ok := raw["Name"].(string); ok {
		mf.Name = name
	}
	if doc, ok := raw["Documentation"].(string); ok {
		mf.Documentation = doc
	}
	if concurrent, ok := raw["AllowConcurrentExecution"].(bool); ok {
		mf.AllowConcurrentExecution = concurrent
	}
	if markAsUsed, ok := raw["MarkAsUsed"].(bool); ok {
		mf.MarkAsUsed = markAsUsed
	}
	if excluded, ok := raw["Excluded"].(bool); ok {
		mf.Excluded = excluded
	}

	// Parse allowed module roles (BY_NAME references)
	allowedRoles := extractBsonArray(raw["AllowedModuleRoles"])
	for _, r := range allowedRoles {
		if name, ok := r.(string); ok {
			mf.AllowedModuleRoles = append(mf.AllowedModuleRoles, model.ID(name))
		}
	}

	// Parse parameters from MicroflowParameterCollection (new format) or MicroflowParameters/Parameters (old format)
	var paramsArray any
	if mpc, ok := raw["MicroflowParameterCollection"]; ok {
		// New format: MicroflowParameterCollection contains Parameters array
		if mpcMap := extractBsonMap(mpc); mpcMap != nil {
			paramsArray = mpcMap["Parameters"]
		}
	} else {
		// Old format: direct MicroflowParameters or Parameters field
		paramKey := "MicroflowParameters"
		if _, ok := raw[paramKey]; !ok {
			paramKey = "Parameters"
		}
		paramsArray = raw[paramKey]
	}
	for _, p := range extractBsonSlice(paramsArray) {
		if paramMap := extractBsonMap(p); paramMap != nil {
			param := parseMicroflowParameter(paramMap)
			mf.Parameters = append(mf.Parameters, param)
		}
	}

	// Parse return type (Mendix uses "MicroflowReturnType")
	if rt, ok := raw["MicroflowReturnType"].(map[string]any); ok {
		mf.ReturnType = parseMicroflowDataType(rt)
	}

	// Parse return variable name
	if rvn, ok := raw["ReturnVariableName"].(string); ok {
		mf.ReturnVariableName = rvn
	}

	// Parse object collection (flow elements)
	if oc := extractBsonMap(raw["ObjectCollection"]); oc != nil {
		mf.ObjectCollection = parseMicroflowObjectCollection(oc)
	}

	// Also extract parameters from ObjectCollection.Objects (modern format)
	// Parameters are stored as Microflows$MicroflowParameter in ObjectCollection
	if len(mf.Parameters) == 0 {
		if ocRaw := extractBsonMap(raw["ObjectCollection"]); ocRaw != nil {
			for _, obj := range extractBsonSlice(ocRaw["Objects"]) {
				if objMap := extractBsonMap(obj); objMap != nil {
					if typeName, _ := objMap["$Type"].(string); typeName == "Microflows$MicroflowParameter" {
						param := parseMicroflowParameter(objMap)
						mf.Parameters = append(mf.Parameters, param)
					}
				}
			}
		}
	}

	// Parse Flows array (SequenceFlows and AnnotationFlows are at root level, not in ObjectCollection)
	if flowsRaw := raw["Flows"]; flowsRaw != nil {
		if mf.ObjectCollection == nil {
			mf.ObjectCollection = &microflows.MicroflowObjectCollection{}
		}
		for _, f := range extractBsonSlice(flowsRaw) {
			if flowMap := extractBsonMap(f); flowMap != nil {
				typeName, _ := flowMap["$Type"].(string)
				switch typeName {
				case "Microflows$AnnotationFlow":
					if af := parseAnnotationFlow(flowMap); af != nil {
						mf.ObjectCollection.AnnotationFlows = append(mf.ObjectCollection.AnnotationFlows, af)
					}
				default:
					if flow := parseSequenceFlow(flowMap); flow != nil {
						mf.ObjectCollection.Flows = append(mf.ObjectCollection.Flows, flow)
					}
				}
			}
		}
	}

	return mf
}

// parseSequenceFlow parses a SequenceFlow from raw BSON data.
func parseSequenceFlow(raw map[string]any) *microflows.SequenceFlow {
	flow := &microflows.SequenceFlow{}
	flow.ID = model.ID(extractBsonID(raw["$ID"]))

	// OriginPointer and DestinationPointer are binary IDs
	flow.OriginID = model.ID(extractBsonID(raw["OriginPointer"]))
	flow.DestinationID = model.ID(extractBsonID(raw["DestinationPointer"]))

	flow.OriginConnectionIndex = extractInt(raw["OriginConnectionIndex"])
	flow.DestinationConnectionIndex = extractInt(raw["DestinationConnectionIndex"])
	if isErr, ok := raw["IsErrorHandler"].(bool); ok {
		flow.IsErrorHandler = isErr
	}

	// Parse CaseValues if present (note: plural, stored as array [count, case_object, ...])
	if caseVals := raw["CaseValues"]; caseVals != nil {
		flow.CaseValue = parseCaseValues(caseVals)
	}

	// Parse BezierCurve control vectors from Line
	if lineMap := extractBsonMap(raw["Line"]); lineMap != nil {
		if v, ok := lineMap["OriginControlVector"].(string); ok {
			flow.OriginControlVector = v
		}
		if v, ok := lineMap["DestinationControlVector"].(string); ok {
			flow.DestinationControlVector = v
		}
	}

	return flow
}

// parseCaseValues parses CaseValues from raw BSON data.
// CaseValues is stored as an array: [count_marker, case_object, ...]
// Usually [2] for empty, or [2, {case}] for a single case value.
func parseCaseValues(raw any) microflows.CaseValue {
	arr := extractBsonSlice(raw)
	if arr == nil {
		return nil
	}

	// Skip the count marker (first element), process actual case values
	if len(arr) < 2 {
		return nil // Empty array or just count marker
	}

	// Parse the first case value (element at index 1)
	return parseCaseValue(arr[1])
}

// parseCaseValue parses a single CaseValue from raw BSON data.
func parseCaseValue(raw any) microflows.CaseValue {
	caseMap := extractBsonMap(raw)
	if caseMap == nil {
		return nil
	}

	typeName, _ := caseMap["$Type"].(string)
	id := model.ID(extractBsonID(caseMap["$ID"]))
	switch typeName {
	case "Microflows$NoCase":
		return &microflows.NoCase{BaseElement: model.BaseElement{ID: id}}
	case "Microflows$ExpressionCase":
		if expr, ok := caseMap["Expression"].(string); ok {
			return &microflows.ExpressionCase{
				BaseElement: model.BaseElement{ID: id},
				Expression:  expr,
			}
		}
	case "Microflows$EnumerationCase":
		if val, ok := caseMap["Value"].(string); ok {
			return &microflows.EnumerationCase{
				BaseElement: model.BaseElement{ID: id},
				Value:       val,
			}
		}
	}
	return nil
}

func parseMicroflowParameter(raw map[string]any) *microflows.MicroflowParameter {
	param := &microflows.MicroflowParameter{}

	// Use extractBsonID to handle binary IDs
	param.ID = model.ID(extractBsonID(raw["$ID"]))
	if name, ok := raw["Name"].(string); ok {
		param.Name = name
	}
	if doc, ok := raw["Documentation"].(string); ok {
		param.Documentation = doc
	}
	// Parse parameter type - Mendix uses "VariableType" in ObjectCollection.Objects format
	// and "ParameterType" in older formats
	if pt := extractBsonMap(raw["VariableType"]); pt != nil {
		param.Type = parseMicroflowDataType(pt)
	} else if pt := extractBsonMap(raw["ParameterType"]); pt != nil {
		param.Type = parseMicroflowDataType(pt)
	}

	return param
}

func parseMicroflowObjectCollection(raw map[string]any) *microflows.MicroflowObjectCollection {
	collection := &microflows.MicroflowObjectCollection{}

	// Handle various ID formats (string, binary, etc.)
	collection.ID = model.ID(extractBsonID(raw["$ID"]))

	// Parse objects array (int32/int64 version markers are skipped by extractBsonMap returning nil)
	for _, obj := range extractBsonSlice(raw["Objects"]) {
		// Prefer primitive.D path to preserve field ordering for unknown types
		if rawD, ok := obj.(primitive.D); ok {
			typeName, _ := rawD.Map()["$Type"].(string)
			if typeName == "" || typeName == "Microflows$MicroflowParameter" {
				// Parameters are handled separately via mf.Parameters
				continue
			}
			if fn, ok := microflowObjectParsers[typeName]; ok {
				if mfObj := fn(rawD.Map()); mfObj != nil {
					collection.Objects = append(collection.Objects, mfObj)
				}
			} else {
				collection.Objects = append(collection.Objects, newUnknownObjectFromD(typeName, bson.D(rawD)))
			}
			continue
		}
		// Fallback for map[string]any
		if objMap := extractBsonMap(obj); objMap != nil {
			if typeName, _ := objMap["$Type"].(string); typeName == "Microflows$MicroflowParameter" {
				continue // Parameters are handled separately
			}
			if mfObj := parseMicroflowObject(objMap); mfObj != nil {
				collection.Objects = append(collection.Objects, mfObj)
			}
		}
	}

	return collection
}

// microflowObjectParsers maps Mendix $Type strings to their parser functions.
// Adding support for a new type requires only one new entry here.
// Declared as a nil var and populated in init() so that the map literal can
// reference parseLoopedActivity, which itself calls parseMicroflowObjectCollection,
// keeping the package-level initialization order unambiguous.
var microflowObjectParsers map[string]func(map[string]any) microflows.MicroflowObject

func init() {
	microflowObjectParsers = map[string]func(map[string]any) microflows.MicroflowObject{
		"Microflows$StartEvent":       func(r map[string]any) microflows.MicroflowObject { return parseStartEvent(r) },
		"Microflows$EndEvent":         func(r map[string]any) microflows.MicroflowObject { return parseEndEvent(r) },
		"Microflows$ErrorEvent":       func(r map[string]any) microflows.MicroflowObject { return parseErrorEvent(r) },
		"Microflows$ActionActivity":   func(r map[string]any) microflows.MicroflowObject { return parseActionActivity(r) },
		"Microflows$ExclusiveSplit":   func(r map[string]any) microflows.MicroflowObject { return parseExclusiveSplit(r) },
		"Microflows$ExclusiveMerge":   func(r map[string]any) microflows.MicroflowObject { return parseExclusiveMerge(r) },
		"Microflows$InheritanceSplit": func(r map[string]any) microflows.MicroflowObject { return parseInheritanceSplit(r) },
		"Microflows$LoopedActivity":   func(r map[string]any) microflows.MicroflowObject { return parseLoopedActivity(r) },
		"Microflows$BreakEvent":       func(r map[string]any) microflows.MicroflowObject { return parseBreakEvent(r) },
		"Microflows$ContinueEvent":    func(r map[string]any) microflows.MicroflowObject { return parseContinueEvent(r) },
		"Microflows$Annotation":       func(r map[string]any) microflows.MicroflowObject { return parseMicroflowAnnotation(r) },
	}
}

// parseMicroflowObject parses a single microflow object based on its $Type.
// Returns nil for elements with an empty $Type (corrupt or placeholder records).
func parseMicroflowObject(raw map[string]any) microflows.MicroflowObject {
	typeName, _ := raw["$Type"].(string)
	if typeName == "" {
		return nil
	}
	if fn, ok := microflowObjectParsers[typeName]; ok {
		return fn(raw)
	}
	return newUnknownObject(typeName, raw)
}

func parseStartEvent(raw map[string]any) *microflows.StartEvent {
	event := &microflows.StartEvent{}
	event.ID = model.ID(extractBsonID(raw["$ID"]))
	event.Position = parsePoint(raw["RelativeMiddlePoint"])
	event.Size = parseSize(raw["Size"])
	return event
}

func parseEndEvent(raw map[string]any) *microflows.EndEvent {
	event := &microflows.EndEvent{}
	event.ID = model.ID(extractBsonID(raw["$ID"]))
	event.Position = parsePoint(raw["RelativeMiddlePoint"])
	event.Size = parseSize(raw["Size"])
	event.ReturnValue = extractString(raw["ReturnValue"])
	return event
}

func parseErrorEvent(raw map[string]any) *microflows.ErrorEvent {
	event := &microflows.ErrorEvent{}
	event.ID = model.ID(extractBsonID(raw["$ID"]))
	event.Position = parsePoint(raw["RelativeMiddlePoint"])
	event.Size = parseSize(raw["Size"])
	return event
}

func parseBreakEvent(raw map[string]any) *microflows.BreakEvent {
	event := &microflows.BreakEvent{}
	event.ID = model.ID(extractBsonID(raw["$ID"]))
	event.Position = parsePoint(raw["RelativeMiddlePoint"])
	event.Size = parseSize(raw["Size"])
	return event
}

func parseContinueEvent(raw map[string]any) *microflows.ContinueEvent {
	event := &microflows.ContinueEvent{}
	event.ID = model.ID(extractBsonID(raw["$ID"]))
	event.Position = parsePoint(raw["RelativeMiddlePoint"])
	event.Size = parseSize(raw["Size"])
	return event
}

func parseExclusiveSplit(raw map[string]any) *microflows.ExclusiveSplit {
	split := &microflows.ExclusiveSplit{}
	split.ID = model.ID(extractBsonID(raw["$ID"]))
	split.Position = parsePoint(raw["RelativeMiddlePoint"])
	split.Size = parseSize(raw["Size"])
	split.Caption = extractString(raw["Caption"])
	split.Documentation = extractString(raw["Documentation"])

	// Parse split condition
	if condition, ok := raw["SplitCondition"].(map[string]any); ok {
		split.SplitCondition = parseSplitCondition(condition)
	}

	return split
}

func parseExclusiveMerge(raw map[string]any) *microflows.ExclusiveMerge {
	merge := &microflows.ExclusiveMerge{}
	merge.ID = model.ID(extractBsonID(raw["$ID"]))
	merge.Position = parsePoint(raw["RelativeMiddlePoint"])
	merge.Size = parseSize(raw["Size"])
	return merge
}

func parseInheritanceSplit(raw map[string]any) *microflows.InheritanceSplit {
	split := &microflows.InheritanceSplit{}
	split.ID = model.ID(extractBsonID(raw["$ID"]))
	split.Position = parsePoint(raw["RelativeMiddlePoint"])
	split.Size = parseSize(raw["Size"])
	split.Caption = extractString(raw["Caption"])
	split.Documentation = extractString(raw["Documentation"])
	split.VariableName = extractString(raw["SplitVariableName"])
	return split
}

func parseLoopedActivity(raw map[string]any) *microflows.LoopedActivity {
	loop := &microflows.LoopedActivity{}
	loop.ID = model.ID(extractBsonID(raw["$ID"]))
	loop.Position = parsePoint(raw["RelativeMiddlePoint"])
	loop.Size = parseSize(raw["Size"])
	loop.Caption = extractString(raw["Caption"])
	loop.Documentation = extractString(raw["Documentation"])

	// Parse LoopSource (IterableList or WhileLoopCondition)
	if loopSourceMap := extractBsonMap(raw["LoopSource"]); loopSourceMap != nil {
		typeName := extractString(loopSourceMap["$Type"])
		switch typeName {
		case "Microflows$WhileLoopCondition":
			loop.LoopSource = &microflows.WhileLoopCondition{
				BaseElement:     model.BaseElement{ID: model.ID(extractBsonID(loopSourceMap["$ID"]))},
				WhileExpression: extractString(loopSourceMap["WhileExpression"]),
			}
		default: // Microflows$IterableList
			loop.LoopSource = &microflows.IterableList{
				BaseElement:      model.BaseElement{ID: model.ID(extractBsonID(loopSourceMap["$ID"]))},
				ListVariableName: extractString(loopSourceMap["ListVariableName"]),
				VariableName:     extractString(loopSourceMap["VariableName"]),
			}
		}
	}

	// Parse nested object collection
	if oc := extractBsonMap(raw["ObjectCollection"]); oc != nil {
		loop.ObjectCollection = parseMicroflowObjectCollection(oc)
	}

	return loop
}

func parseMicroflowAnnotation(raw map[string]any) *microflows.Annotation {
	annot := &microflows.Annotation{}
	annot.ID = model.ID(extractBsonID(raw["$ID"]))
	annot.Position = parsePoint(raw["RelativeMiddlePoint"])
	annot.Size = parseSize(raw["Size"])
	annot.Caption = extractString(raw["Caption"])
	return annot
}

// parseAnnotationFlow parses an AnnotationFlow from raw BSON data.
func parseAnnotationFlow(raw map[string]any) *microflows.AnnotationFlow {
	flow := &microflows.AnnotationFlow{}
	flow.ID = model.ID(extractBsonID(raw["$ID"]))
	flow.OriginID = model.ID(extractBsonID(raw["OriginPointer"]))
	flow.DestinationID = model.ID(extractBsonID(raw["DestinationPointer"]))
	return flow
}

func parseSplitCondition(raw map[string]any) microflows.SplitCondition {
	typeName, _ := raw["$Type"].(string)

	switch typeName {
	case "Microflows$ExpressionSplitCondition":
		return &microflows.ExpressionSplitCondition{
			Expression: extractString(raw["Expression"]),
		}
	case "Microflows$RuleSplitCondition":
		cond := &microflows.RuleSplitCondition{
			RuleID: model.ID(extractBsonID(raw["Rule"])),
		}
		// Parse parameter mappings if present
		if mappings, ok := raw["ParameterMappings"].([]any); ok {
			for _, m := range mappings {
				if mMap, ok := m.(map[string]any); ok {
					mapping := &microflows.RuleCallParameterMapping{
						ParameterID: model.ID(extractBsonID(mMap["Parameter"])),
						Argument:    extractString(mMap["Argument"]),
					}
					cond.ParameterMappings = append(cond.ParameterMappings, mapping)
				}
			}
		}
		return cond
	default:
		return nil
	}
}

func parseActionActivity(raw map[string]any) *microflows.ActionActivity {
	activity := &microflows.ActionActivity{}
	activity.ID = model.ID(extractBsonID(raw["$ID"]))
	activity.Position = parsePoint(raw["RelativeMiddlePoint"])
	activity.Size = parseSize(raw["Size"])
	activity.Caption = extractString(raw["Caption"])
	activity.Documentation = extractString(raw["Documentation"])
	activity.AutoGenerateCaption = extractBool(raw["AutoGenerateCaption"], false)
	activity.BackgroundColor = extractString(raw["BackgroundColor"])
	activity.Disabled = extractBool(raw["Disabled"], false)

	if errorHandling, ok := raw["ErrorHandlingType"].(string); ok {
		activity.ErrorHandlingType = microflows.ErrorHandlingType(errorHandling)
	}

	// Parse the action
	if action, ok := raw["Action"].(map[string]any); ok {
		activity.Action = parseMicroflowAction(action)
	}

	return activity
}

// microflowActionParsers maps Mendix $Type strings to their action parser functions.
// Storage names (e.g. CreateChangeAction) and qualified names (e.g. CreateObjectAction)
// both map to the same parser to handle BSON format variations.
var microflowActionParsers = map[string]func(map[string]any) microflows.MicroflowAction{
	// Variable actions
	"Microflows$CreateVariableAction": func(r map[string]any) microflows.MicroflowAction { return parseCreateVariableAction(r) },
	"Microflows$ChangeVariableAction": func(r map[string]any) microflows.MicroflowAction { return parseChangeVariableAction(r) },

	// Object actions (storageName may differ from qualifiedName)
	"Microflows$CreateObjectAction": func(r map[string]any) microflows.MicroflowAction { return parseCreateObjectAction(r) },
	"Microflows$CreateChangeAction": func(r map[string]any) microflows.MicroflowAction { return parseCreateObjectAction(r) },
	"Microflows$ChangeObjectAction": func(r map[string]any) microflows.MicroflowAction { return parseChangeObjectAction(r) },
	"Microflows$ChangeAction":       func(r map[string]any) microflows.MicroflowAction { return parseChangeObjectAction(r) },
	"Microflows$DeleteAction":       func(r map[string]any) microflows.MicroflowAction { return parseDeleteAction(r) },
	"Microflows$CommitAction":       func(r map[string]any) microflows.MicroflowAction { return parseCommitAction(r) },
	"Microflows$RollbackAction":     func(r map[string]any) microflows.MicroflowAction { return parseRollbackAction(r) },

	// Retrieve actions
	"Microflows$RetrieveAction":      func(r map[string]any) microflows.MicroflowAction { return parseRetrieveAction(r) },
	"Microflows$AggregateListAction": func(r map[string]any) microflows.MicroflowAction { return parseAggregateListAction(r) },
	"Microflows$AggregateAction":     func(r map[string]any) microflows.MicroflowAction { return parseAggregateListAction(r) },

	// List actions
	"Microflows$CreateListAction":     func(r map[string]any) microflows.MicroflowAction { return parseCreateListAction(r) },
	"Microflows$ChangeListAction":     func(r map[string]any) microflows.MicroflowAction { return parseChangeListAction(r) },
	"Microflows$ListOperationAction":  func(r map[string]any) microflows.MicroflowAction { return parseListOperationAction(r) },
	"Microflows$ListOperationsAction": func(r map[string]any) microflows.MicroflowAction { return parseListOperationAction(r) },

	// Integration actions
	"Microflows$MicroflowCallAction":  func(r map[string]any) microflows.MicroflowAction { return parseMicroflowCallAction(r) },
	"Microflows$JavaActionCallAction": func(r map[string]any) microflows.MicroflowAction { return parseJavaActionCallAction(r) },
	"Microflows$CallExternalAction":   func(r map[string]any) microflows.MicroflowAction { return parseCallExternalAction(r) },

	// Client actions (ShowFormAction is storageName for ShowPageAction)
	"Microflows$ShowFormAction":           func(r map[string]any) microflows.MicroflowAction { return parseShowPageAction(r) },
	"Microflows$ShowPageAction":           func(r map[string]any) microflows.MicroflowAction { return parseShowPageAction(r) },
	"Microflows$ShowHomePageAction":       func(r map[string]any) microflows.MicroflowAction { return parseShowHomePageAction(r) },
	"Microflows$CloseFormAction":          func(r map[string]any) microflows.MicroflowAction { return parseClosePageAction(r) },
	"Microflows$ShowMessageAction":        func(r map[string]any) microflows.MicroflowAction { return parseShowMessageAction(r) },
	"Microflows$ValidationFeedbackAction": func(r map[string]any) microflows.MicroflowAction { return parseValidationFeedbackAction(r) },
	"Microflows$DownloadFileAction":       func(r map[string]any) microflows.MicroflowAction { return parseDownloadFileAction(r) },

	// Log action
	"Microflows$LogMessageAction": func(r map[string]any) microflows.MicroflowAction { return parseLogMessageAction(r) },

	// Cast action
	"Microflows$CastAction": func(r map[string]any) microflows.MicroflowAction { return parseCastAction(r) },

	// REST call action (inline HTTP)
	"Microflows$RestCallAction": func(r map[string]any) microflows.MicroflowAction { return parseRestCallAction(r) },

	// REST operation call action (consumed REST service)
	"Microflows$RestOperationCallAction": func(r map[string]any) microflows.MicroflowAction {
		return parseRestOperationCallAction(r)
	},

	// Import/Export mapping actions
	"Microflows$ImportXmlAction": func(r map[string]any) microflows.MicroflowAction { return parseImportXmlAction(r) },
	"Microflows$ExportXmlAction": func(r map[string]any) microflows.MicroflowAction { return parseExportXmlAction(r) },

	// Data transformer action
	"Microflows$TransformJsonAction": func(r map[string]any) microflows.MicroflowAction { return parseTransformJsonAction(r) },

	// Database Connector action
	"DatabaseConnector$ExecuteDatabaseQueryAction": func(r map[string]any) microflows.MicroflowAction { return parseExecuteDatabaseQueryAction(r) },

	// Workflow actions
	"Microflows$WorkflowCallAction":               func(r map[string]any) microflows.MicroflowAction { return parseWorkflowCallAction(r) },
	"Microflows$GetWorkflowDataAction":            func(r map[string]any) microflows.MicroflowAction { return parseGetWorkflowDataAction(r) },
	"Microflows$GetWorkflowsAction":               func(r map[string]any) microflows.MicroflowAction { return parseGetWorkflowsAction(r) },
	"Microflows$GetWorkflowActivityRecordsAction": func(r map[string]any) microflows.MicroflowAction { return parseGetWorkflowActivityRecordsAction(r) },
	"Microflows$WorkflowOperationAction":          func(r map[string]any) microflows.MicroflowAction { return parseWorkflowOperationAction(r) },
	"Microflows$SetTaskOutcomeAction":             func(r map[string]any) microflows.MicroflowAction { return parseSetTaskOutcomeAction(r) },
	"Microflows$OpenUserTaskAction":               func(r map[string]any) microflows.MicroflowAction { return parseOpenUserTaskAction(r) },
	"Microflows$NotifyWorkflowAction":             func(r map[string]any) microflows.MicroflowAction { return parseNotifyWorkflowAction(r) },
	"Microflows$OpenWorkflowAction":               func(r map[string]any) microflows.MicroflowAction { return parseOpenWorkflowAction(r) },
	"Microflows$LockWorkflowAction":               func(r map[string]any) microflows.MicroflowAction { return parseLockWorkflowAction(r) },
	"Microflows$UnlockWorkflowAction":             func(r map[string]any) microflows.MicroflowAction { return parseUnlockWorkflowAction(r) },
}

// parseMicroflowAction parses a microflow action based on its $Type.
func parseMicroflowAction(raw map[string]any) microflows.MicroflowAction {
	typeName, _ := raw["$Type"].(string)
	if fn, ok := microflowActionParsers[typeName]; ok {
		return fn(raw)
	}
	return &microflows.UnknownAction{TypeName: typeName}
}

func parseCreateVariableAction(raw map[string]any) *microflows.CreateVariableAction {
	action := &microflows.CreateVariableAction{}
	action.ID = model.ID(extractBsonID(raw["$ID"]))
	action.VariableName = extractString(raw["VariableName"])
	action.InitialValue = extractString(raw["InitialValue"])

	if dt, ok := raw["VariableType"].(map[string]any); ok {
		action.DataType = parseMicroflowDataType(dt)
	}

	return action
}

func parseChangeVariableAction(raw map[string]any) *microflows.ChangeVariableAction {
	action := &microflows.ChangeVariableAction{}
	action.ID = model.ID(extractBsonID(raw["$ID"]))
	action.VariableName = extractString(raw["ChangeVariableName"])
	action.Value = extractString(raw["Value"])
	return action
}

func parseCreateObjectAction(raw map[string]any) *microflows.CreateObjectAction {
	action := &microflows.CreateObjectAction{}
	action.ID = model.ID(extractBsonID(raw["$ID"]))
	// Entity is BY_NAME_REFERENCE - can be string (qualified name) or binary (legacy)
	if entityStr, ok := raw["Entity"].(string); ok {
		action.EntityQualifiedName = entityStr
	} else {
		action.EntityID = model.ID(extractBsonID(raw["Entity"]))
	}
	// OutputVariable has storageName "VariableName" but qualifiedName "OutputVariableName"
	action.OutputVariable = extractString(raw["VariableName"])
	if action.OutputVariable == "" {
		action.OutputVariable = extractString(raw["OutputVariableName"])
	}

	if commit, ok := raw["Commit"].(string); ok {
		action.Commit = microflows.CommitType(commit)
	}

	// Parse initial member values
	for _, item := range extractBsonSlice(raw["Items"]) {
		if itemMap := extractBsonMap(item); itemMap != nil {
			if change := parseMemberChange(itemMap); change != nil {
				action.InitialMembers = append(action.InitialMembers, change)
			}
		}
	}

	return action
}

func parseChangeObjectAction(raw map[string]any) *microflows.ChangeObjectAction {
	action := &microflows.ChangeObjectAction{}
	action.ID = model.ID(extractBsonID(raw["$ID"]))
	action.ChangeVariable = extractString(raw["ChangeVariableName"])
	action.RefreshInClient = extractBool(raw["RefreshInClient"], false)

	if commit, ok := raw["Commit"].(string); ok {
		action.Commit = microflows.CommitType(commit)
	}

	// Parse member changes
	for _, item := range extractBsonSlice(raw["Items"]) {
		if itemMap := extractBsonMap(item); itemMap != nil {
			if change := parseMemberChange(itemMap); change != nil {
				action.Changes = append(action.Changes, change)
			}
		}
	}

	return action
}

func parseMemberChange(raw map[string]any) *microflows.MemberChange {
	change := &microflows.MemberChange{}
	change.ID = model.ID(extractBsonID(raw["$ID"]))

	// Attribute can be BY_NAME_REFERENCE (string) or BY_ID (binary)
	if attrStr, ok := raw["Attribute"].(string); ok {
		change.AttributeQualifiedName = attrStr
	} else {
		change.AttributeID = model.ID(extractBsonID(raw["Attribute"]))
	}

	// Association can be BY_NAME_REFERENCE (string) or BY_ID (binary)
	if assocStr, ok := raw["Association"].(string); ok {
		change.AssociationQualifiedName = assocStr
	} else {
		change.AssociationID = model.ID(extractBsonID(raw["Association"]))
	}

	change.Value = extractString(raw["Value"])

	if changeType, ok := raw["Type"].(string); ok {
		change.Type = microflows.MemberChangeType(changeType)
	}

	return change
}

func parseDeleteAction(raw map[string]any) *microflows.DeleteObjectAction {
	action := &microflows.DeleteObjectAction{}
	action.ID = model.ID(extractBsonID(raw["$ID"]))
	action.DeleteVariable = extractString(raw["DeleteVariableName"])
	action.RefreshInClient = extractBool(raw["RefreshInClient"], false)
	return action
}

func parseCommitAction(raw map[string]any) *microflows.CommitObjectsAction {
	action := &microflows.CommitObjectsAction{}
	action.ID = model.ID(extractBsonID(raw["$ID"]))
	action.CommitVariable = extractString(raw["CommitVariableName"])
	action.WithEvents = extractBool(raw["WithEvents"], false)
	action.RefreshInClient = extractBool(raw["RefreshInClient"], false)
	if errType, ok := raw["ErrorHandlingType"].(string); ok {
		action.ErrorHandlingType = microflows.ErrorHandlingType(errType)
	}
	return action
}

func parseRollbackAction(raw map[string]any) *microflows.RollbackObjectAction {
	action := &microflows.RollbackObjectAction{}
	action.ID = model.ID(extractBsonID(raw["$ID"]))
	action.RollbackVariable = extractString(raw["RollbackVariableName"])
	action.RefreshInClient = extractBool(raw["RefreshInClient"], false)
	return action
}

func parseRetrieveAction(raw map[string]any) *microflows.RetrieveAction {
	action := &microflows.RetrieveAction{}
	action.ID = model.ID(extractBsonID(raw["$ID"]))
	// Writer uses "ResultVariableName" as the storage name
	action.OutputVariable = extractString(raw["ResultVariableName"])

	// Parse retrieve source
	if source, ok := raw["RetrieveSource"].(map[string]any); ok {
		action.Source = parseRetrieveSource(source)
	}

	return action
}

// parseSortItems parses sort items from a BSON map that wraps a SortItemList.
// It tries multiple field-name conventions (modern and legacy storage names).
func parseSortItems(raw map[string]any) []*microflows.SortItem {
	// Try field names: "sortItemList", "NewSortings", "Sortings", "SortItemList"
	var listMap map[string]any
	for _, key := range []string{"sortItemList", "NewSortings", "Sortings", "SortItemList"} {
		if m := extractBsonMap(raw[key]); m != nil {
			listMap = m
			break
		}
	}
	if listMap == nil {
		return nil
	}

	// Extract items array — try "items", "Sortings", "Items"
	var items []any
	for _, key := range []string{"items", "Sortings", "Items"} {
		if s := extractBsonSlice(listMap[key]); s != nil {
			items = s
			break
		}
	}

	var result []*microflows.SortItem
	for _, item := range items {
		itemMap := extractBsonMap(item)
		if itemMap == nil {
			continue
		}
		sortItem := &microflows.SortItem{}
		sortItem.ID = model.ID(extractBsonID(itemMap["$ID"]))

		// Try AttributeRef (modern: DomainModels$AttributeRef with BY_NAME_REFERENCE)
		if attrRefMap := extractBsonMap(itemMap["AttributeRef"]); attrRefMap != nil {
			if attrStr, ok := attrRefMap["Attribute"].(string); ok {
				sortItem.AttributeQualifiedName = attrStr
			} else {
				sortItem.AttributeID = model.ID(extractBsonID(attrRefMap["Attribute"]))
			}
		}

		// Fall back to AttributePath (legacy)
		if sortItem.AttributeQualifiedName == "" && sortItem.AttributeID == "" {
			if attrStr, ok := itemMap["AttributePath"].(string); ok {
				sortItem.AttributeQualifiedName = attrStr
			} else {
				sortItem.AttributeID = model.ID(extractBsonID(itemMap["AttributePath"]))
			}
		}

		if dir, ok := itemMap["SortOrder"].(string); ok {
			sortItem.Direction = microflows.SortDirection(dir)
		}
		result = append(result, sortItem)
	}
	return result
}

func parseRetrieveSource(raw map[string]any) microflows.RetrieveSource {
	typeName, _ := raw["$Type"].(string)

	switch typeName {
	case "Microflows$DatabaseRetrieveSource":
		source := &microflows.DatabaseRetrieveSource{}
		source.ID = model.ID(extractBsonID(raw["$ID"]))
		// Entity can be stored as string (BY_NAME_REFERENCE) or binary ID
		if entityStr, ok := raw["Entity"].(string); ok {
			source.EntityQualifiedName = entityStr
		} else {
			source.EntityID = model.ID(extractBsonID(raw["Entity"]))
		}
		// XPath constraint - Studio Pro uses lowercase 'p' (XpathConstraint), but we also support uppercase for backwards compatibility
		source.XPathConstraint = extractString(raw["XpathConstraint"])
		if source.XPathConstraint == "" {
			source.XPathConstraint = extractString(raw["XPathConstraint"])
		}

		// Parse range
		if rangeMap, ok := raw["Range"].(map[string]any); ok {
			source.Range = parseRange(rangeMap)
		}

		// Parse sorting
		source.Sorting = parseSortItems(raw)

		return source

	case "Microflows$AssociationRetrieveSource":
		source := &microflows.AssociationRetrieveSource{}
		source.ID = model.ID(extractBsonID(raw["$ID"]))
		source.StartVariable = extractString(raw["StartVariableName"])
		source.AssociationID = model.ID(extractBsonID(raw["Association"]))
		// AssociationId contains BY_NAME_REFERENCE (qualified name)
		source.AssociationQualifiedName = extractString(raw["AssociationId"])
		return source

	default:
		return nil
	}
}

func parseRange(raw map[string]any) *microflows.Range {
	typeName, _ := raw["$Type"].(string)

	r := &microflows.Range{}
	r.ID = model.ID(extractBsonID(raw["$ID"]))

	switch typeName {
	case "Microflows$ConstantRange":
		r.Limit = extractString(raw["LimitExpression"])
		r.Offset = extractString(raw["OffsetExpression"])
		if singleObject := extractBool(raw["SingleObject"], false); singleObject {
			r.RangeType = microflows.RangeTypeFirst
		} else if r.Limit != "" || r.Offset != "" {
			// Studio Pro stores custom ranges as ConstantRange with LimitExpression/OffsetExpression
			r.RangeType = microflows.RangeTypeCustom
		} else {
			r.RangeType = microflows.RangeTypeAll
		}
	case "Microflows$CustomRange":
		r.RangeType = microflows.RangeTypeCustom
		r.Limit = extractString(raw["LimitExpression"])
		r.Offset = extractString(raw["OffsetExpression"])
	default:
		r.RangeType = microflows.RangeTypeAll
	}

	return r
}

func parseAggregateListAction(raw map[string]any) *microflows.AggregateListAction {
	action := &microflows.AggregateListAction{}
	action.ID = model.ID(extractBsonID(raw["$ID"]))
	// Storage name is AggregateVariableName, qualified name is inputListVariableName
	action.InputVariable = extractString(raw["AggregateVariableName"])
	if action.InputVariable == "" {
		action.InputVariable = extractString(raw["InputListVariableName"])
	}
	// Storage name is VariableName, qualified name is outputVariableName
	action.OutputVariable = extractString(raw["VariableName"])
	if action.OutputVariable == "" {
		action.OutputVariable = extractString(raw["OutputVariableName"])
	}

	// Attribute is BY_NAME_REFERENCE - can be string (qualified name) or binary (legacy ID)
	if attrStr, ok := raw["Attribute"].(string); ok {
		action.AttributeQualifiedName = attrStr
	} else {
		action.AttributeID = model.ID(extractBsonID(raw["Attribute"]))
	}

	if fn, ok := raw["AggregateFunction"].(string); ok {
		action.Function = microflows.AggregateFunction(fn)
	}

	return action
}

func parseCreateListAction(raw map[string]any) *microflows.CreateListAction {
	action := &microflows.CreateListAction{}
	action.ID = model.ID(extractBsonID(raw["$ID"]))
	// Entity is BY_NAME_REFERENCE - can be string (qualified name) or binary (legacy)
	if entityStr, ok := raw["Entity"].(string); ok {
		action.EntityQualifiedName = entityStr
	} else {
		action.EntityID = model.ID(extractBsonID(raw["Entity"]))
	}
	action.OutputVariable = extractString(raw["VariableName"])
	return action
}

func parseChangeListAction(raw map[string]any) *microflows.ChangeListAction {
	action := &microflows.ChangeListAction{}
	action.ID = model.ID(extractBsonID(raw["$ID"]))
	action.ChangeVariable = extractString(raw["ChangeVariableName"])
	action.Value = extractString(raw["Value"])
	if t, ok := raw["Type"].(string); ok {
		action.Type = microflows.ChangeListType(t)
	}
	return action
}

func parseListOperationAction(raw map[string]any) *microflows.ListOperationAction {
	action := &microflows.ListOperationAction{}
	action.ID = model.ID(extractBsonID(raw["$ID"]))
	action.OutputVariable = extractString(raw["ResultVariableName"])

	// Parse the operation from NewOperation (storage name for operation)
	if opRaw, ok := raw["NewOperation"].(map[string]any); ok {
		action.Operation = parseListOperation(opRaw)
	}

	return action
}

func parseListOperation(raw map[string]any) microflows.ListOperation {
	typeName, _ := raw["$Type"].(string)
	listVar := extractString(raw["ListName"])
	id := model.ID(extractBsonID(raw["$ID"]))

	switch typeName {
	case "Microflows$Head":
		return &microflows.HeadOperation{
			BaseElement:  model.BaseElement{ID: id},
			ListVariable: listVar,
		}
	case "Microflows$Tail":
		return &microflows.TailOperation{
			BaseElement:  model.BaseElement{ID: id},
			ListVariable: listVar,
		}
	case "Microflows$FindByExpression":
		return &microflows.FindOperation{
			BaseElement:  model.BaseElement{ID: id},
			ListVariable: listVar,
			Expression:   extractString(raw["Expression"]),
		}
	case "Microflows$FilterByExpression":
		return &microflows.FilterOperation{
			BaseElement:  model.BaseElement{ID: id},
			ListVariable: listVar,
			Expression:   extractString(raw["Expression"]),
		}
	case "Microflows$Sort":
		sortOp := &microflows.SortOperation{
			BaseElement:  model.BaseElement{ID: id},
			ListVariable: listVar,
		}
		sortOp.Sorting = parseSortItems(raw)
		return sortOp
	case "Microflows$Union":
		return &microflows.UnionOperation{
			BaseElement:   model.BaseElement{ID: id},
			ListVariable1: listVar,
			ListVariable2: extractString(raw["SecondListOrObjectName"]),
		}
	case "Microflows$Intersect":
		return &microflows.IntersectOperation{
			BaseElement:   model.BaseElement{ID: id},
			ListVariable1: listVar,
			ListVariable2: extractString(raw["SecondListOrObjectName"]),
		}
	case "Microflows$Subtract":
		return &microflows.SubtractOperation{
			BaseElement:   model.BaseElement{ID: id},
			ListVariable1: listVar,
			ListVariable2: extractString(raw["SecondListOrObjectName"]),
		}
	case "Microflows$Contains":
		return &microflows.ContainsOperation{
			BaseElement:    model.BaseElement{ID: id},
			ListVariable:   listVar,
			ObjectVariable: extractString(raw["SecondListOrObjectName"]),
		}
	case "Microflows$ListEquals":
		return &microflows.EqualsOperation{
			BaseElement:   model.BaseElement{ID: id},
			ListVariable1: listVar,
			ListVariable2: extractString(raw["SecondListOrObjectName"]),
		}
	case "Microflows$Find":
		return &microflows.FindByAttributeOperation{
			BaseElement:  model.BaseElement{ID: id},
			ListVariable: listVar,
			Attribute:    extractString(raw["Attribute"]),
			Association:  extractString(raw["Association"]),
			Expression:   extractString(raw["Expression"]),
		}
	case "Microflows$Filter":
		return &microflows.FilterByAttributeOperation{
			BaseElement:  model.BaseElement{ID: id},
			ListVariable: listVar,
			Attribute:    extractString(raw["Attribute"]),
			Association:  extractString(raw["Association"]),
			Expression:   extractString(raw["Expression"]),
		}
	case "Microflows$ListRange":
		offset, limit := "", ""
		if cr, ok := raw["CustomRange"].(map[string]any); ok {
			offset = extractString(cr["OffsetExpression"])
			limit = extractString(cr["LimitExpression"])
		}
		return &microflows.RangeOperation{
			BaseElement:      model.BaseElement{ID: id},
			ListVariable:     listVar,
			OffsetExpression: offset,
			LimitExpression:  limit,
		}
	default:
		return nil
	}
}

func parseMicroflowDataType(raw map[string]any) microflows.DataType {
	typeName, _ := raw["$Type"].(string)

	switch typeName {
	case "DataTypes$BooleanType":
		return &microflows.BooleanType{}
	case "DataTypes$IntegerType":
		return &microflows.IntegerType{}
	case "DataTypes$LongType":
		return &microflows.LongType{}
	case "DataTypes$DecimalType":
		return &microflows.DecimalType{}
	case "DataTypes$StringType":
		return &microflows.StringType{}
	case "DataTypes$DateTimeType":
		return &microflows.DateTimeType{}
	case "DataTypes$BinaryType":
		return &microflows.BinaryType{}
	case "DataTypes$VoidType":
		return &microflows.VoidType{}
	case "DataTypes$ObjectType":
		objType := &microflows.ObjectType{}
		// Entity can be BY_NAME_REFERENCE (string) or binary ID (legacy)
		if entityStr, ok := raw["Entity"].(string); ok {
			objType.EntityQualifiedName = entityStr
		} else {
			objType.EntityID = model.ID(extractBsonID(raw["Entity"]))
		}
		return objType
	case "DataTypes$ListType":
		listType := &microflows.ListType{}
		// Entity can be BY_NAME_REFERENCE (string) or binary ID (legacy)
		if entityStr, ok := raw["Entity"].(string); ok {
			listType.EntityQualifiedName = entityStr
		} else {
			listType.EntityID = model.ID(extractBsonID(raw["Entity"]))
		}
		return listType
	case "DataTypes$EnumerationType":
		enumType := &microflows.EnumerationType{}
		// Enumeration can be BY_NAME_REFERENCE (string) or binary ID (legacy)
		if enumStr, ok := raw["Enumeration"].(string); ok {
			enumType.EnumerationQualifiedName = enumStr
		} else {
			enumType.EnumerationID = model.ID(extractBsonID(raw["Enumeration"]))
		}
		return enumType
	default:
		return nil
	}
}

func parsePoint(raw any) model.Point {
	switch v := raw.(type) {
	case map[string]any:
		return model.Point{
			X: extractInt(v["X"]),
			Y: extractInt(v["Y"]),
		}
	case string:
		// MPR v2 stores positions as "X;Y" strings, e.g. "570;297"
		parts := strings.SplitN(v, ";", 2)
		if len(parts) == 2 {
			x, _ := strconv.Atoi(strings.TrimSpace(parts[0]))
			y, _ := strconv.Atoi(strings.TrimSpace(parts[1]))
			return model.Point{X: x, Y: y}
		}
	}
	return model.Point{}
}

// parseSize parses a Size from raw BSON data (stored as "W;H" string).
func parseSize(raw any) model.Size {
	if s, ok := raw.(string); ok {
		parts := strings.SplitN(s, ";", 2)
		if len(parts) == 2 {
			w, _ := strconv.Atoi(strings.TrimSpace(parts[0]))
			h, _ := strconv.Atoi(strings.TrimSpace(parts[1]))
			return model.Size{Width: w, Height: h}
		}
	}
	return model.Size{}
}

// parseNanoflow parses nanoflow contents from BSON.
