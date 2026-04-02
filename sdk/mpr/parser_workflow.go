// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/workflows"

	"go.mongodb.org/mongo-driver/bson"
)

func (r *Reader) parseWorkflow(unitID, containerID string, contents []byte) (*workflows.Workflow, error) {
	contents, err := r.resolveContents(unitID, contents)
	if err != nil {
		return nil, err
	}

	var raw map[string]any
	if err := bson.Unmarshal(contents, &raw); err != nil {
		return nil, fmt.Errorf("failed to unmarshal workflow BSON: %w", err)
	}

	w := &workflows.Workflow{}
	w.ID = model.ID(unitID)
	w.TypeName = "Workflows$Workflow"
	w.ContainerID = model.ID(containerID)

	if name, ok := raw["Name"].(string); ok {
		w.Name = name
	}
	if doc, ok := raw["Documentation"].(string); ok {
		w.Documentation = doc
	}
	if excluded, ok := raw["Excluded"].(bool); ok {
		w.Excluded = excluded
	}
	if exportLevel, ok := raw["ExportLevel"].(string); ok {
		w.ExportLevel = exportLevel
	}

	// Parse Annotation
	if annotRaw := raw["Annotation"]; annotRaw != nil {
		annotMap := toMap(annotRaw)
		if annotMap != nil {
			if desc, ok := annotMap["Description"].(string); ok {
				w.Annotation = desc
			}
		}
	}

	// Parse Parameter (PART — DomainModels$IndirectEntityRef or similar)
	if paramRaw := raw["Parameter"]; paramRaw != nil {
		w.Parameter = parseWorkflowParameter(toMap(paramRaw))
	}

	// Parse OverviewPage (BY_NAME reference to Pages$Page)
	if overviewPage, ok := raw["OverviewPage"].(string); ok {
		w.OverviewPage = overviewPage
	}

	// Parse AdminPage (BY_NAME reference)
	if adminPage, ok := raw["AdminPage"].(string); ok {
		w.AdminPage = adminPage
	}

	// Parse WorkflowName (StringTemplate — extract text)
	w.WorkflowName = extractStringTemplate(raw["WorkflowName"])

	// Parse WorkflowDescription (StringTemplate — extract text)
	w.WorkflowDescription = extractStringTemplate(raw["WorkflowDescription"])

	// Parse DueDate expression
	if dueDate, ok := raw["DueDate"].(string); ok {
		w.DueDate = dueDate
	}

	// Parse allowed module roles (BY_NAME references)
	allowedRoles := extractBsonArray(raw["AllowedModuleRoles"])
	for _, r := range allowedRoles {
		if name, ok := r.(string); ok {
			w.AllowedModuleRoles = append(w.AllowedModuleRoles, model.ID(name))
		}
	}

	// Parse Flow (PART — Workflows$Flow)
	if flowRaw := raw["Flow"]; flowRaw != nil {
		w.Flow = parseWorkflowFlow(toMap(flowRaw))
	}

	return w, nil
}

// extractStringTemplate extracts the text from a Mendix StringTemplate BSON structure.
// StringTemplates have a "Text" field with the template string.
func extractStringTemplate(v any) string {
	m := toMap(v)
	if m == nil {
		return ""
	}
	// Direct text field
	if text, ok := m["Text"].(string); ok {
		return text
	}
	// Try Translations for localized strings
	if translations := m["Translations"]; translations != nil {
		transMap := toMap(translations)
		if transMap != nil {
			// Look for "en_US" or first available
			for _, val := range transMap {
				if s, ok := val.(string); ok && s != "" {
					return s
				}
			}
		}
	}
	return ""
}

// parseWorkflowParameter parses the workflow context parameter.
func parseWorkflowParameter(raw map[string]any) *workflows.WorkflowParameter {
	if raw == nil {
		return nil
	}

	param := &workflows.WorkflowParameter{}
	param.ID = model.ID(extractBsonID(raw["$ID"]))

	// EntityRef is typically stored as an IndirectEntityRef with "EntityQualifiedName" or within an Entity field
	if entityRef := raw["EntityRef"]; entityRef != nil {
		entityMap := toMap(entityRef)
		if entityMap != nil {
			// Try EntityQualifiedName (new format)
			if eqn, ok := entityMap["EntityQualifiedName"].(string); ok {
				param.EntityRef = eqn
			}
			// Try QualifiedName
			if qn, ok := entityMap["QualifiedName"].(string); ok && param.EntityRef == "" {
				param.EntityRef = qn
			}
		}
	}

	// Also try Entity field directly (BY_NAME reference)
	if entity, ok := raw["Entity"].(string); ok && param.EntityRef == "" {
		param.EntityRef = entity
	}

	// Try EntityQualifiedName at parameter level
	if eqn, ok := raw["EntityQualifiedName"].(string); ok && param.EntityRef == "" {
		param.EntityRef = eqn
	}

	return param
}

// parseWorkflowFlow parses a Workflows$Flow from raw BSON data.
func parseWorkflowFlow(raw map[string]any) *workflows.Flow {
	if raw == nil {
		return nil
	}

	flow := &workflows.Flow{}
	flow.ID = model.ID(extractBsonID(raw["$ID"]))

	// Parse activities array
	activitiesRaw := extractBsonArray(raw["Activities"])
	for _, actRaw := range activitiesRaw {
		actMap := toMap(actRaw)
		if actMap == nil {
			continue
		}
		if activity := parseWorkflowActivity(actMap); activity != nil {
			flow.Activities = append(flow.Activities, activity)
		}
	}

	return flow
}

// workflowActivityParsers maps Mendix $Type strings to their workflow activity parser functions.
// Initialized in init() to avoid initialization cycle (parseParallelSplitActivity → parseWorkflowFlow → parseWorkflowActivity).
var workflowActivityParsers map[string]func(map[string]any) workflows.WorkflowActivity

func init() {
	workflowActivityParsers = map[string]func(map[string]any) workflows.WorkflowActivity{
		"Workflows$EndWorkflowActivity":         func(r map[string]any) workflows.WorkflowActivity { return parseEndWorkflowActivity(r) },
		"Workflows$UserTask":                    func(r map[string]any) workflows.WorkflowActivity { return parseUserTask(r) },
		"Workflows$SingleUserTaskActivity":      func(r map[string]any) workflows.WorkflowActivity { return parseUserTask(r) },
		"Workflows$MultiUserTaskActivity":       func(r map[string]any) workflows.WorkflowActivity { return parseMultiUserTask(r) },
		"Workflows$CallMicroflowTask":           func(r map[string]any) workflows.WorkflowActivity { return parseCallMicroflowTask(r) },
		"Workflows$CallWorkflowActivity":        func(r map[string]any) workflows.WorkflowActivity { return parseCallWorkflowActivity(r) },
		"Workflows$ExclusiveSplitActivity":      func(r map[string]any) workflows.WorkflowActivity { return parseExclusiveSplitActivity(r) },
		"Workflows$ParallelSplitActivity":       func(r map[string]any) workflows.WorkflowActivity { return parseParallelSplitActivity(r) },
		"Workflows$JumpToActivity":              func(r map[string]any) workflows.WorkflowActivity { return parseJumpToActivity(r) },
		"Workflows$WaitForTimerActivity":        func(r map[string]any) workflows.WorkflowActivity { return parseWaitForTimerActivity(r) },
		"Workflows$WaitForNotificationActivity": func(r map[string]any) workflows.WorkflowActivity { return parseWaitForNotificationActivity(r) },
		"Workflows$StartWorkflowActivity":       func(r map[string]any) workflows.WorkflowActivity { return parseStartWorkflowActivity(r) },
		"Workflows$EndOfParallelSplitPathActivity": func(r map[string]any) workflows.WorkflowActivity {
			a := &workflows.EndOfParallelSplitPathActivity{}
			parseBaseActivity(&a.BaseWorkflowActivity, r)
			return a
		},
		"Workflows$EndOfBoundaryEventPathActivity": func(r map[string]any) workflows.WorkflowActivity {
			a := &workflows.EndOfBoundaryEventPathActivity{}
			parseBaseActivity(&a.BaseWorkflowActivity, r)
			return a
		},
		"Workflows$Annotation": func(r map[string]any) workflows.WorkflowActivity {
			a := &workflows.WorkflowAnnotationActivity{}
			parseBaseActivity(&a.BaseWorkflowActivity, r)
			if desc, ok := r["Description"].(string); ok {
				a.Description = desc
			}
			return a
		},
		"Workflows$SystemTask": func(r map[string]any) workflows.WorkflowActivity { return parseSystemTask(r) },
	}
}

// parseWorkflowActivity dispatches activity parsing based on $Type.
func parseWorkflowActivity(raw map[string]any) workflows.WorkflowActivity {
	typeName := extractString(raw["$Type"])
	if fn, ok := workflowActivityParsers[typeName]; ok {
		return fn(raw)
	}
	if typeName != "" {
		return parseGenericWorkflowActivity(raw, typeName)
	}
	return nil
}

// parseEndWorkflowActivity parses an EndWorkflowActivity.
func parseStartWorkflowActivity(raw map[string]any) *workflows.StartWorkflowActivity {
	a := &workflows.StartWorkflowActivity{}
	parseBaseActivity(&a.BaseWorkflowActivity, raw)
	return a
}

func parseEndWorkflowActivity(raw map[string]any) *workflows.EndWorkflowActivity {
	a := &workflows.EndWorkflowActivity{}
	parseBaseActivity(&a.BaseWorkflowActivity, raw)
	return a
}

// parseUserTask parses a UserTask activity.
func parseUserTask(raw map[string]any) *workflows.UserTask {
	a := &workflows.UserTask{}
	parseBaseActivity(&a.BaseWorkflowActivity, raw)

	// Page (BY_NAME reference)
	if page, ok := raw["Page"].(string); ok {
		a.Page = page
	}
	// Also try TaskPage — may be a nested Workflows$PageReference object
	if a.Page == "" {
		if page, ok := raw["TaskPage"].(string); ok {
			a.Page = page
		} else if taskPageMap := toMap(raw["TaskPage"]); taskPageMap != nil {
			if page, ok := taskPageMap["Page"].(string); ok {
				a.Page = page
			}
		}
	}

	// TaskName (StringTemplate)
	a.TaskName = extractStringTemplate(raw["TaskName"])

	// TaskDescription (StringTemplate)
	a.TaskDescription = extractStringTemplate(raw["TaskDescription"])

	// DueDate
	if dueDate, ok := raw["DueDate"].(string); ok {
		a.DueDate = dueDate
	}

	// UserTaskEntity (BY_NAME reference)
	if ute, ok := raw["UserTaskEntity"].(string); ok {
		a.UserTaskEntity = ute
	}

	// OnCreated (BY_NAME reference to microflow)
	if onCreated, ok := raw["OnCreatedEvent"].(string); ok {
		a.OnCreated = onCreated
	}

	// UserSource (PART)
	if userSourceRaw := raw["UserSource"]; userSourceRaw != nil {
		a.UserSource = parseUserSource(toMap(userSourceRaw))
	}

	// Outcomes
	outcomesRaw := extractBsonArray(raw["Outcomes"])
	for _, outcomeRaw := range outcomesRaw {
		outcomeMap := toMap(outcomeRaw)
		if outcomeMap == nil {
			continue
		}
		outcome := parseUserTaskOutcome(outcomeMap)
		if outcome != nil {
			a.Outcomes = append(a.Outcomes, outcome)
		}
	}

	// BoundaryEvents
	a.BoundaryEvents = parseBoundaryEvents(raw["BoundaryEvents"])

	return a
}

// parseSystemTask parses a SystemTask (older type name for CallMicroflowTask).
func parseSystemTask(raw map[string]any) *workflows.SystemTask {
	a := &workflows.SystemTask{}
	parseBaseActivity(&a.BaseWorkflowActivity, raw)

	// Microflow (BY_NAME reference)
	if mf, ok := raw["Microflow"].(string); ok {
		a.Microflow = mf
	}
	if mf, ok := raw["MicroflowName"].(string); ok && a.Microflow == "" {
		a.Microflow = mf
	}

	// Outcomes
	a.Outcomes = parseConditionOutcomes(raw["Outcomes"])

	// ParameterMappings
	a.ParameterMappings = parseParameterMappings(raw["ParameterMappings"])

	return a
}

// parseCallMicroflowTask parses a CallMicroflowTask activity.
func parseCallMicroflowTask(raw map[string]any) *workflows.CallMicroflowTask {
	a := &workflows.CallMicroflowTask{}
	parseBaseActivity(&a.BaseWorkflowActivity, raw)

	// Microflow (BY_NAME reference)
	if mf, ok := raw["Microflow"].(string); ok {
		a.Microflow = mf
	}
	if mf, ok := raw["MicroflowName"].(string); ok && a.Microflow == "" {
		a.Microflow = mf
	}

	// Outcomes
	a.Outcomes = parseConditionOutcomes(raw["Outcomes"])

	// ParameterMappings
	a.ParameterMappings = parseParameterMappings(raw["ParameterMappings"])

	// BoundaryEvents
	a.BoundaryEvents = parseBoundaryEvents(raw["BoundaryEvents"])

	return a
}

// parseCallWorkflowActivity parses a CallWorkflowActivity.
func parseCallWorkflowActivity(raw map[string]any) *workflows.CallWorkflowActivity {
	a := &workflows.CallWorkflowActivity{}
	parseBaseActivity(&a.BaseWorkflowActivity, raw)

	// Workflow (BY_NAME reference)
	if wf, ok := raw["Workflow"].(string); ok {
		a.Workflow = wf
	}
	if wf, ok := raw["WorkflowName"].(string); ok && a.Workflow == "" {
		a.Workflow = wf
	}

	// ParameterExpression
	if expr, ok := raw["ParameterExpression"].(string); ok {
		a.ParameterExpression = expr
	}

	// BoundaryEvents
	a.BoundaryEvents = parseBoundaryEvents(raw["BoundaryEvents"])

	return a
}

// parseExclusiveSplitActivity parses an ExclusiveSplitActivity (decision).
func parseExclusiveSplitActivity(raw map[string]any) *workflows.ExclusiveSplitActivity {
	a := &workflows.ExclusiveSplitActivity{}
	parseBaseActivity(&a.BaseWorkflowActivity, raw)

	// Expression
	if expr, ok := raw["Expression"].(string); ok {
		a.Expression = expr
	}

	// Outcomes
	a.Outcomes = parseConditionOutcomes(raw["Outcomes"])

	return a
}

// parseParallelSplitActivity parses a ParallelSplitActivity.
func parseParallelSplitActivity(raw map[string]any) *workflows.ParallelSplitActivity {
	a := &workflows.ParallelSplitActivity{}
	parseBaseActivity(&a.BaseWorkflowActivity, raw)

	// Outcomes
	outcomesRaw := extractBsonArray(raw["Outcomes"])
	for _, outcomeRaw := range outcomesRaw {
		outcomeMap := toMap(outcomeRaw)
		if outcomeMap == nil {
			continue
		}
		outcome := &workflows.ParallelSplitOutcome{}
		outcome.ID = model.ID(extractBsonID(outcomeMap["$ID"]))
		if flowRaw := outcomeMap["Flow"]; flowRaw != nil {
			outcome.Flow = parseWorkflowFlow(toMap(flowRaw))
		}
		a.Outcomes = append(a.Outcomes, outcome)
	}

	return a
}

// parseJumpToActivity parses a JumpToActivity.
func parseJumpToActivity(raw map[string]any) *workflows.JumpToActivity {
	a := &workflows.JumpToActivity{}
	parseBaseActivity(&a.BaseWorkflowActivity, raw)

	// TargetActivity (LOCAL_BY_NAME reference)
	if target, ok := raw["TargetActivity"].(string); ok {
		a.TargetActivity = target
	}
	if target, ok := raw["TargetActivityName"].(string); ok && a.TargetActivity == "" {
		a.TargetActivity = target
	}

	return a
}

// parseWaitForTimerActivity parses a WaitForTimerActivity.
func parseWaitForTimerActivity(raw map[string]any) *workflows.WaitForTimerActivity {
	a := &workflows.WaitForTimerActivity{}
	parseBaseActivity(&a.BaseWorkflowActivity, raw)

	if expr, ok := raw["Delay"].(string); ok {
		a.DelayExpression = expr
	} else if expr, ok := raw["DelayExpression"].(string); ok {
		// Legacy fallback
		a.DelayExpression = expr
	}

	return a
}

// parseWaitForNotificationActivity parses a WaitForNotificationActivity.
func parseWaitForNotificationActivity(raw map[string]any) *workflows.WaitForNotificationActivity {
	a := &workflows.WaitForNotificationActivity{}
	parseBaseActivity(&a.BaseWorkflowActivity, raw)

	// BoundaryEvents
	a.BoundaryEvents = parseBoundaryEvents(raw["BoundaryEvents"])

	return a
}

// parseGenericWorkflowActivity creates a fallback for unknown activity types.
func parseGenericWorkflowActivity(raw map[string]any, typeName string) *workflows.GenericWorkflowActivity {
	a := &workflows.GenericWorkflowActivity{TypeString: typeName}
	parseBaseActivity(&a.BaseWorkflowActivity, raw)
	return a
}

// parseBaseActivity extracts common fields for all workflow activities.
func parseBaseActivity(a *workflows.BaseWorkflowActivity, raw map[string]any) {
	a.ID = model.ID(extractBsonID(raw["$ID"]))
	a.TypeName = extractString(raw["$Type"])

	if name, ok := raw["Name"].(string); ok {
		a.Name = name
	}
	if caption, ok := raw["Caption"].(string); ok {
		a.Caption = caption
	}

	// Annotation (PART — Workflows$Annotation)
	if annotRaw := raw["Annotation"]; annotRaw != nil {
		annotMap := toMap(annotRaw)
		if annotMap != nil {
			if desc, ok := annotMap["Description"].(string); ok {
				a.Annotation = desc
			}
		}
	}
}

// parseMultiUserTask parses a MultiUserTaskActivity, reusing parseUserTask with IsMulti flag.
func parseMultiUserTask(raw map[string]any) *workflows.UserTask {
	task := parseUserTask(raw)
	if task != nil {
		task.IsMulti = true
	}
	return task
}

// parseUserTaskOutcome parses a UserTaskOutcome.
func parseUserTaskOutcome(raw map[string]any) *workflows.UserTaskOutcome {
	outcome := &workflows.UserTaskOutcome{}
	outcome.ID = model.ID(extractBsonID(raw["$ID"]))

	if name, ok := raw["Name"].(string); ok {
		outcome.Name = name
	}
	if caption, ok := raw["Caption"].(string); ok {
		outcome.Caption = caption
	}
	if value, ok := raw["Value"].(string); ok {
		outcome.Value = value
	}

	if flowRaw := raw["Flow"]; flowRaw != nil {
		outcome.Flow = parseWorkflowFlow(toMap(flowRaw))
	}

	return outcome
}

// parseConditionOutcomes parses an array of condition outcomes.
func parseConditionOutcomes(v any) []workflows.ConditionOutcome {
	outcomesRaw := extractBsonArray(v)
	var outcomes []workflows.ConditionOutcome

	for _, outcomeRaw := range outcomesRaw {
		outcomeMap := toMap(outcomeRaw)
		if outcomeMap == nil {
			continue
		}

		typeName := extractString(outcomeMap["$Type"])
		switch typeName {
		case "Workflows$BooleanConditionOutcome":
			o := &workflows.BooleanConditionOutcome{}
			o.ID = model.ID(extractBsonID(outcomeMap["$ID"]))
			if v, ok := outcomeMap["Value"].(bool); ok {
				o.Value = v
			}
			if flowRaw := outcomeMap["Flow"]; flowRaw != nil {
				o.Flow = parseWorkflowFlow(toMap(flowRaw))
			}
			outcomes = append(outcomes, o)

		case "Workflows$EnumerationValueConditionOutcome":
			o := &workflows.EnumerationValueConditionOutcome{}
			o.ID = model.ID(extractBsonID(outcomeMap["$ID"]))
			if v, ok := outcomeMap["Value"].(string); ok {
				o.Value = v
			}
			if flowRaw := outcomeMap["Flow"]; flowRaw != nil {
				o.Flow = parseWorkflowFlow(toMap(flowRaw))
			}
			outcomes = append(outcomes, o)

		default:
			// VoidConditionOutcome or unknown
			o := &workflows.VoidConditionOutcome{}
			o.ID = model.ID(extractBsonID(outcomeMap["$ID"]))
			if flowRaw := outcomeMap["Flow"]; flowRaw != nil {
				o.Flow = parseWorkflowFlow(toMap(flowRaw))
			}
			outcomes = append(outcomes, o)
		}
	}

	return outcomes
}

// parseUserSource parses a UserSource from raw BSON data.
func parseUserSource(raw map[string]any) workflows.UserSource {
	if raw == nil {
		return &workflows.NoUserSource{}
	}

	typeName := extractString(raw["$Type"])
	switch typeName {
	case "Workflows$MicroflowBasedUserSource":
		source := &workflows.MicroflowBasedUserSource{}
		if mf, ok := raw["Microflow"].(string); ok {
			source.Microflow = mf
		}
		if mf, ok := raw["MicroflowName"].(string); ok && source.Microflow == "" {
			source.Microflow = mf
		}
		return source

	case "Workflows$XPathBasedUserSource":
		source := &workflows.XPathBasedUserSource{}
		if xpath, ok := raw["XPath"].(string); ok {
			source.XPath = xpath
		}
		if xpath, ok := raw["XPathConstraint"].(string); ok && source.XPath == "" {
			source.XPath = xpath
		}
		return source

	default:
		return &workflows.NoUserSource{}
	}
}

// parseBoundaryEvents parses boundary events from a BSON array.
func parseBoundaryEvents(v any) []*workflows.BoundaryEvent {
	eventsRaw := extractBsonArray(v)
	var events []*workflows.BoundaryEvent

	for _, eventRaw := range eventsRaw {
		eventMap := toMap(eventRaw)
		if eventMap == nil {
			continue
		}
		event := &workflows.BoundaryEvent{}
		event.ID = model.ID(extractBsonID(eventMap["$ID"]))
		event.TypeName = extractString(eventMap["$Type"])

		if caption, ok := eventMap["Caption"].(string); ok {
			event.Caption = caption
		}

		// Timer delay — BSON field is "FirstExecutionTime" for both boundary event types
		if delay, ok := eventMap["FirstExecutionTime"].(string); ok {
			event.TimerDelay = delay
		}
		// Legacy fallbacks
		if event.TimerDelay == "" {
			if delay, ok := eventMap["DelayExpression"].(string); ok {
				event.TimerDelay = delay
			}
		}
		if event.TimerDelay == "" {
			if delay, ok := eventMap["Delay"].(string); ok {
				event.TimerDelay = delay
			}
		}

		// Event type from $Type
		typeName := extractString(eventMap["$Type"])
		switch typeName {
		case "Workflows$InterruptingTimerBoundaryEvent":
			event.EventType = "InterruptingTimer"
		case "Workflows$NonInterruptingTimerBoundaryEvent":
			event.EventType = "NonInterruptingTimer"
		case "Workflows$TimerBoundaryEvent":
			event.EventType = "Timer"
		default:
			if typeName != "" {
				// Extract the event type from the type name
				event.EventType = strings.TrimPrefix(typeName, "Workflows$")
			}
		}

		// Flow
		if flowRaw := eventMap["Flow"]; flowRaw != nil {
			event.Flow = parseWorkflowFlow(toMap(flowRaw))
		}

		events = append(events, event)
	}

	return events
}

// parseParameterMappings parses parameter mappings from an array.
func parseParameterMappings(v any) []*workflows.ParameterMapping {
	mappingsRaw := extractBsonArray(v)
	var mappings []*workflows.ParameterMapping

	for _, mappingRaw := range mappingsRaw {
		mappingMap := toMap(mappingRaw)
		if mappingMap == nil {
			continue
		}
		mapping := &workflows.ParameterMapping{}
		mapping.ID = model.ID(extractBsonID(mappingMap["$ID"]))

		if param, ok := mappingMap["Parameter"].(string); ok {
			mapping.Parameter = param
		}
		if expr, ok := mappingMap["Expression"].(string); ok {
			mapping.Expression = expr
		}

		mappings = append(mappings, mapping)
	}

	return mappings
}
