// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"fmt"

	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/microflows"
)

const microflowDocType = "Microflows$Microflow"

// CreateMicroflow creates a microflow via ped_create_document.
//
// The executor's object graph (Start/End/activities + sequence flows) is mapped
// onto the PED constructor: each object becomes a /objects entry and each
// SequenceFlow a /flows entry referencing objects by $id(/objects/N). Adding a
// new activity is one case in mapMicroflowAction. Object and action types that
// are not mapped yet are rejected with a clear error (the 130+ Microflows$*
// types are an iterative follow-on). See docs/03-development/PED_MCP_CAPABILITIES.md.
func (b *Backend) CreateMicroflow(mf *microflows.Microflow) error {
	moduleName, folderPath, err := b.resolveDocContainer(mf.ContainerID)
	if err != nil {
		return fmt.Errorf("resolve container for microflow %q: %w", mf.Name, err)
	}
	content, err := b.buildFlowDocContent("microflow", mf.Name, mf.Parameters, mf.ObjectCollection, mf.ReturnType)
	if err != nil {
		return err
	}
	content["returnVariableName"] = mf.ReturnVariableName
	if err := b.ensureSchema(microflowDocType); err != nil {
		return err
	}
	if err := b.pedCreateDocument(moduleName, microflowDocType, mf.Name, content, folderPath); err != nil {
		return err
	}
	if mf.ID == "" {
		mf.ID = model.ID("mcp~mf~" + moduleName + "~" + mf.Name)
	}
	b.sessionMicroflows = append(b.sessionMicroflows, mf)
	return b.pedCheckDocument(microflowDocType, moduleName+"."+mf.Name)
}

// buildFlowDocContent maps a microflow body — parameters as the leading canvas
// objects, the flow object tree, the sequence flows, and the return type — onto PED
// content. The caller adds doc-type-specific fields (returnVariableName) and picks
// the document type. kind labels errors. (Nanoflows share this body shape but can't
// be created over MCP — PED rejects the Microflows$Nanoflow doc type — so this has
// the single microflow caller today; see nanoflow.go.)
func (b *Backend) buildFlowDocContent(kind, name string, params []*microflows.MicroflowParameter, oc *microflows.MicroflowObjectCollection, returnType microflows.DataType) (map[string]any, error) {
	// Parameters occupy the first object slots (canvas objects that take part in no
	// flow); the executor's flow objects follow.
	objects := make([]any, 0, len(params)+2)
	for i, p := range params {
		typeName, entity, enumeration, err := mfDataType(p.Type)
		if err != nil {
			return nil, fmt.Errorf("%s %q parameter %q: %w", kind, name, p.Name, err)
		}
		po := map[string]any{
			"$Type":               "Microflows$MicroflowParameterObject",
			"name":                p.Name,
			"type":                typeName,
			"relativeMiddlePoint": map[string]int{"x": 200 + i*100, "y": 53},
		}
		if entity != "" {
			po["entity"] = entity
		}
		if enumeration != "" {
			po["enumeration"] = enumeration
		}
		objects = append(objects, po)
	}
	paramCount := len(objects)

	// Map the flow objects (Start/End/activities) and remember each object's index
	// so SequenceFlows can reference it by $id(/objects/N). idPath maps each
	// object's ID to its JSON-pointer path. Loop bodies nest, so a body object's
	// path is /objects/N/objects/M; flows (all at the top level, even loop-body
	// flows) reference objects by these paths.
	idPath := map[model.ID]string{}
	if oc != nil {
		for i, o := range oc.Objects {
			path := fmt.Sprintf("/objects/%d", paramCount+i)
			pedObj, err := b.mapObjectTree(o, path, idPath)
			if err != nil {
				return nil, fmt.Errorf("%s %q: %w", kind, name, err)
			}
			objects = append(objects, pedObj)
		}
	}

	flows := make([]any, 0)
	if oc != nil {
		for _, f := range oc.Flows {
			op, ok1 := idPath[f.OriginID]
			dp, ok2 := idPath[f.DestinationID]
			if !ok1 || !ok2 {
				return nil, fmt.Errorf("%s %q: a sequence flow references an object that is not supported yet", kind, name)
			}
			pf := map[string]any{
				"originId":      fmt.Sprintf("$id(%s)", op),
				"destinationId": fmt.Sprintf("$id(%s)", dp),
			}
			cv, err := mapCaseValue(f.CaseValue)
			if err != nil {
				return nil, fmt.Errorf("%s %q: %w", kind, name, err)
			}
			if cv != nil {
				pf["caseValue"] = cv
			}
			flows = append(flows, pf)
		}
	}

	returnTypeName, rtEntity, rtEnum, err := mfDataType(returnType)
	if err != nil {
		return nil, fmt.Errorf("%s %q return type: %w", kind, name, err)
	}
	rt := map[string]any{"type": returnTypeName}
	if rtEntity != "" {
		rt["entity"] = rtEntity
	}
	if rtEnum != "" {
		rt["enumeration"] = rtEnum
	}

	return map[string]any{"name": name, "objects": objects, "flows": flows, "returnType": rt}, nil
}

// ListMicroflows returns microflows from the local reader merged with those
// created over MCP this session (session entries take precedence by
// module+name) — for duplicate detection and create-then-reference in one run.
func (b *Backend) ListMicroflows() ([]*microflows.Microflow, error) {
	local, err := b.reader.ListMicroflows()
	if err != nil {
		return nil, err
	}
	if len(b.sessionMicroflows) == 0 {
		return local, nil
	}
	seen := make(map[string]bool, len(b.sessionMicroflows))
	out := make([]*microflows.Microflow, 0, len(local)+len(b.sessionMicroflows))
	for _, m := range b.sessionMicroflows {
		seen[mfKey(m)] = true
		out = append(out, m)
	}
	for _, m := range local {
		if !seen[mfKey(m)] {
			out = append(out, m)
		}
	}
	return out, nil
}

// DeleteMicroflow drops a microflow via Concord's delete_document (PED has no
// delete tool). Requires --mcp-concord; errors clearly otherwise.
func (b *Backend) DeleteMicroflow(id model.ID) error {
	mf, err := b.GetMicroflow(id)
	if err != nil {
		return fmt.Errorf("resolve microflow %s for DROP: %w", id, err)
	}
	modName, err := b.moduleNameForContainer(mf.ContainerID)
	if err != nil {
		return fmt.Errorf("resolve module for microflow %q: %w", mf.Name, err)
	}
	return b.concordDeleteDocument(modName, mf.Name)
}

// GetMicroflow resolves by ID, preferring session-created microflows.
func (b *Backend) GetMicroflow(id model.ID) (*microflows.Microflow, error) {
	for _, m := range b.sessionMicroflows {
		if m.ID == id {
			return m, nil
		}
	}
	return b.reader.GetMicroflow(id)
}

func mfKey(m *microflows.Microflow) string {
	return string(m.ContainerID) + "." + m.Name
}

// Nanoflow reads/writes live in nanoflow.go (they reuse buildFlowDocContent).
func (b *Backend) IsRule(qualifiedName string) (bool, error) { return b.reader.IsRule(qualifiedName) }

// pedMiddlePoint converts an executor object position to a PED relativeMiddlePoint.
// The executor's layout engine already computes these coordinates (the same
// value the MPR writer serializes as RelativeMiddlePoint), so reusing them makes
// the MCP-authored canvas match the file-written one. Loop-body positions are
// relative to their loop container, which is exactly what PED expects for
// nested objects.
func pedMiddlePoint(pt model.Point) map[string]int {
	return map[string]int{"x": pt.X, "y": pt.Y}
}

// mapObjectTree maps one executor microflow object onto a PED /objects entry,
// registering its path in idPath and recursing into loop bodies (whose objects
// nest at <path>/objects/M). Positions come from the executor's layout engine.
// Unmapped object types are rejected.
func (b *Backend) mapObjectTree(o microflows.MicroflowObject, path string, idPath map[model.ID]string) (map[string]any, error) {
	idPath[o.GetID()] = path
	pos := pedMiddlePoint(o.GetPosition())
	switch obj := o.(type) {
	case *microflows.StartEvent:
		return map[string]any{
			"$Type":               "Microflows$StartEvent",
			"relativeMiddlePoint": pos,
		}, nil
	case *microflows.EndEvent:
		m := map[string]any{
			"$Type":               "Microflows$EndEvent",
			"relativeMiddlePoint": pos,
		}
		if obj.ReturnValue != "" {
			m["returnValue"] = obj.ReturnValue
		}
		return m, nil
	case *microflows.ActionActivity:
		action, err := mapMicroflowAction(obj.Action)
		if err != nil {
			return nil, err
		}
		return map[string]any{
			"$Type":               "Microflows$ActionActivity",
			"relativeMiddlePoint": pos,
			"action":              action,
		}, nil
	case *microflows.ExclusiveSplit:
		m := map[string]any{
			"$Type":               "Microflows$ExclusiveSplit",
			"relativeMiddlePoint": pos,
		}
		switch c := obj.SplitCondition.(type) {
		case *microflows.ExpressionSplitCondition:
			m["expressionSplitCondition"] = c.Expression
		case microflows.ExpressionSplitCondition:
			m["expressionSplitCondition"] = c.Expression
		default:
			return nil, fmt.Errorf("exclusive split: only expression conditions are supported yet (got %T)", obj.SplitCondition)
		}
		if obj.Caption != "" {
			m["caption"] = obj.Caption
		}
		return m, nil
	case *microflows.ExclusiveMerge:
		return map[string]any{
			"$Type":               "Microflows$ExclusiveMerge",
			"relativeMiddlePoint": pos,
		}, nil
	case *microflows.BreakEvent:
		return map[string]any{
			"$Type":               "Microflows$BreakEvent",
			"relativeMiddlePoint": pos,
		}, nil
	case *microflows.ContinueEvent:
		return map[string]any{
			"$Type":               "Microflows$ContinueEvent",
			"relativeMiddlePoint": pos,
		}, nil
	case *microflows.LoopedActivity:
		m := map[string]any{
			"$Type":               "Microflows$LoopedActivity",
			"relativeMiddlePoint": pos,
		}
		// The loop container must be large enough to hold its (loop-relative)
		// body objects; reuse the executor's computed size.
		if obj.Size.Width > 0 && obj.Size.Height > 0 {
			m["size"] = map[string]int{"width": obj.Size.Width, "height": obj.Size.Height}
		}
		switch src := obj.LoopSource.(type) {
		case *microflows.IterableList:
			m["iterableListSource"] = map[string]any{
				"listVariableName":     src.ListVariableName,
				"iteratorVariableName": src.VariableName,
			}
		case *microflows.WhileLoopCondition:
			m["whileLoopSource"] = map[string]any{"condition": src.WhileExpression}
		default:
			return nil, fmt.Errorf("loop: unsupported loop source %T", obj.LoopSource)
		}
		// Body objects nest under <path>/objects/M. Their flows are already at
		// the microflow top level (the executor lifts them there), so we only
		// map objects here and register their nested paths.
		body := make([]any, 0)
		if obj.ObjectCollection != nil {
			for j, bo := range obj.ObjectCollection.Objects {
				bp := fmt.Sprintf("%s/objects/%d", path, j)
				pedBody, err := b.mapObjectTree(bo, bp, idPath)
				if err != nil {
					return nil, err
				}
				body = append(body, pedBody)
			}
		}
		m["objects"] = body
		return m, nil
	default:
		return nil, fmt.Errorf("microflow object type %T is not yet supported by the MCP backend", o)
	}
}

// mapMicroflowAction maps one microflow action onto its PED action element.
// Add a case here to support a new activity type.
func mapMicroflowAction(a microflows.MicroflowAction) (map[string]any, error) {
	switch act := a.(type) {
	case *microflows.CreateVariableAction:
		varType, err := mfVariableType(act.DataType)
		if err != nil {
			return nil, fmt.Errorf("create variable %q: %w", act.VariableName, err)
		}
		m := map[string]any{
			"$Type":        "Microflows$CreateVariableAction",
			"variableName": act.VariableName,
			"variableType": varType,
		}
		if act.InitialValue != "" {
			m["initialValue"] = act.InitialValue
		}
		return m, nil
	case *microflows.ChangeVariableAction:
		return map[string]any{
			"$Type":              "Microflows$ChangeVariableAction",
			"changeVariableName": act.VariableName,
			"value":              act.Value,
		}, nil
	case *microflows.ShowMessageAction:
		messageType := string(act.Type)
		if messageType == "" {
			messageType = "Information"
		}
		return map[string]any{
			"$Type": "Microflows$ShowMessageAction",
			"type":  messageType,
			// ShowMessage's template is an inline object (no $Type).
			"template": mfStringTemplate("", act.Template, act.TemplateParameters),
		}, nil
	case *microflows.CreateObjectAction:
		if act.EntityQualifiedName == "" {
			return nil, fmt.Errorf("create object: missing entity")
		}
		m := map[string]any{
			"$Type":  "Microflows$CreateObjectAction",
			"entity": act.EntityQualifiedName,
			"commit": mfCommitType(act.Commit),
		}
		if act.OutputVariable != "" {
			m["outputVariableName"] = act.OutputVariable
		}
		if len(act.InitialMembers) > 0 {
			m["items"] = mapMemberChanges(act.InitialMembers)
		}
		return m, nil
	case *microflows.ChangeObjectAction:
		m := map[string]any{
			"$Type":              "Microflows$ChangeObjectAction",
			"changeVariableName": act.ChangeVariable,
			"commit":             mfCommitType(act.Commit),
			"refreshInClient":    act.RefreshInClient,
		}
		if len(act.Changes) > 0 {
			m["items"] = mapMemberChanges(act.Changes)
		}
		return m, nil
	case *microflows.CommitObjectsAction:
		return map[string]any{
			"$Type":              "Microflows$CommitAction",
			"commitVariableName": act.CommitVariable,
			"withEvents":         act.WithEvents,
			"refreshInClient":    act.RefreshInClient,
		}, nil
	case *microflows.DeleteObjectAction:
		return map[string]any{
			"$Type":              "Microflows$DeleteAction",
			"deleteVariableName": act.DeleteVariable,
			"refreshInClient":    act.RefreshInClient,
		}, nil
	case *microflows.RollbackObjectAction:
		return map[string]any{
			"$Type":                "Microflows$RollbackAction",
			"rollbackVariableName": act.RollbackVariable,
			"refreshInClient":      act.RefreshInClient,
		}, nil
	case *microflows.CreateListAction:
		if act.EntityQualifiedName == "" {
			return nil, fmt.Errorf("create list: missing entity")
		}
		return map[string]any{
			"$Type":              "Microflows$CreateListAction",
			"entity":             act.EntityQualifiedName,
			"outputVariableName": act.OutputVariable,
		}, nil
	case *microflows.ChangeListAction:
		m := map[string]any{
			"$Type":              "Microflows$ChangeListAction",
			"changeVariableName": act.ChangeVariable,
			"type":               string(act.Type),
		}
		if act.Value != "" {
			m["value"] = act.Value
		}
		return m, nil
	case *microflows.AggregateListAction:
		m := map[string]any{
			"$Type":              "Microflows$AggregateListAction",
			"inputVariableName":  act.InputVariable,
			"outputVariableName": act.OutputVariable,
			"function":           string(act.Function),
		}
		// Count needs neither; Sum/Average/Min/Max need an attribute or an
		// expression to aggregate over.
		if act.UseExpression && act.Expression != "" {
			m["expression"] = act.Expression
		} else if act.AttributeQualifiedName != "" {
			m["attribute"] = act.AttributeQualifiedName
		}
		return m, nil
	case *microflows.ListOperationAction:
		op, err := mapListOperation(act.Operation)
		if err != nil {
			return nil, err
		}
		m := map[string]any{"$Type": "Microflows$ListOperationAction", "operation": op}
		if act.OutputVariable != "" {
			m["outputVariableName"] = act.OutputVariable
		}
		return m, nil
	case *microflows.DownloadFileAction:
		return map[string]any{
			"$Type":                    "Microflows$DownloadFileAction",
			"fileDocumentVariableName": act.FileDocument,
			"showFileInBrowser":        act.ShowInBrowser,
		}, nil
	case *microflows.ClosePageAction:
		n := act.NumberOfPages
		if n <= 0 {
			n = 1
		}
		return map[string]any{
			"$Type":                "Microflows$CloseFormAction",
			"numberOfPagesToClose": fmt.Sprintf("%d", n),
		}, nil
	case *microflows.ValidationFeedbackAction:
		m := map[string]any{
			"$Type":              "Microflows$ValidationFeedbackAction",
			"objectVariableName": act.ObjectVariable,
			"feedbackTemplate":   mfStringTemplate("Microflows$TextTemplate", act.Template, act.TemplateParameters),
		}
		if act.AttributeName != "" {
			m["attribute"] = act.AttributeName
		}
		if act.AssociationName != "" {
			m["association"] = act.AssociationName
		}
		return m, nil
	case *microflows.ShowPageAction:
		return nil, fmt.Errorf("show page is not supported by the MCP backend — PED's ShowPageAction constructor does not expose the target page (pages are handled by the pg_* tools, not PED)")
	case *microflows.ShowHomePageAction:
		return map[string]any{"$Type": "Microflows$ShowHomePageAction"}, nil
	case *microflows.NanoflowCallAction:
		if act.NanoflowCall == nil || act.NanoflowCall.Nanoflow == "" {
			return nil, fmt.Errorf("call nanoflow: missing target nanoflow")
		}
		mappings := make([]any, 0, len(act.NanoflowCall.ParameterMappings))
		for _, pm := range act.NanoflowCall.ParameterMappings {
			mappings = append(mappings, map[string]any{
				"$Type":     "Microflows$NanoflowCallParameterMapping",
				"parameter": pm.Parameter,
				"argument":  pm.Argument,
			})
		}
		call := map[string]any{"$Type": "Microflows$NanoflowCall", "nanoflow": act.NanoflowCall.Nanoflow}
		if len(mappings) > 0 {
			call["parameterMappings"] = mappings
		}
		m := map[string]any{
			"$Type":             "Microflows$NanoflowCallAction",
			"nanoflowCall":      call,
			"useReturnVariable": act.UseReturnVariable,
		}
		if act.OutputVariableName != "" {
			m["outputVariableName"] = act.OutputVariableName
		}
		return m, nil
	case *microflows.JavaActionCallAction:
		if act.JavaAction == "" {
			return nil, fmt.Errorf("call java action: missing target java action")
		}
		mappings := make([]any, 0, len(act.ParameterMappings))
		for _, pm := range act.ParameterMappings {
			pv, err := mapCodeActionParameterValue(pm.Value)
			if err != nil {
				return nil, err
			}
			mappings = append(mappings, map[string]any{
				"$Type":          "Microflows$JavaActionParameterMapping",
				"parameter":      pm.Parameter,
				"parameterValue": pv,
			})
		}
		m := map[string]any{
			"$Type":             "Microflows$JavaActionCallAction",
			"javaAction":        act.JavaAction,
			"useReturnVariable": act.UseReturnVariable,
		}
		if len(mappings) > 0 {
			m["parameterMappings"] = mappings
		}
		if act.ResultVariableName != "" {
			m["outputVariableName"] = act.ResultVariableName
		}
		return m, nil
	case *microflows.JavaScriptActionCallAction:
		if act.JavaScriptAction == "" {
			return nil, fmt.Errorf("call javascript action: missing target javascript action")
		}
		mappings := make([]any, 0, len(act.ParameterMappings))
		for _, pm := range act.ParameterMappings {
			pv, err := mapCodeActionParameterValue(pm.Value)
			if err != nil {
				return nil, err
			}
			mappings = append(mappings, map[string]any{
				"$Type":          "Microflows$JavaScriptActionParameterMapping",
				"parameter":      pm.Parameter,
				"parameterValue": pv,
			})
		}
		m := map[string]any{
			"$Type":             "Microflows$JavaScriptActionCallAction",
			"javaScriptAction":  act.JavaScriptAction,
			"useReturnVariable": act.UseReturnVariable,
		}
		if len(mappings) > 0 {
			m["parameterMappings"] = mappings
		}
		if act.OutputVariableName != "" {
			m["outputVariableName"] = act.OutputVariableName
		}
		return m, nil
	case *microflows.MicroflowCallAction:
		if act.MicroflowCall == nil || act.MicroflowCall.Microflow == "" {
			return nil, fmt.Errorf("call microflow: missing target microflow")
		}
		mappings := make([]any, 0, len(act.MicroflowCall.ParameterMappings))
		for _, pm := range act.MicroflowCall.ParameterMappings {
			mappings = append(mappings, map[string]any{
				"$Type":     "Microflows$MicroflowCallParameterMapping",
				"parameter": pm.Parameter,
				"argument":  pm.Argument,
			})
		}
		call := map[string]any{
			"$Type":     "Microflows$MicroflowCall",
			"microflow": act.MicroflowCall.Microflow,
		}
		if len(mappings) > 0 {
			call["parameterMappings"] = mappings
		}
		m := map[string]any{
			"$Type":             "Microflows$MicroflowCallAction",
			"microflowCall":     call,
			"useReturnVariable": act.UseReturnVariable,
		}
		if act.ResultVariableName != "" {
			m["outputVariableName"] = act.ResultVariableName
		}
		return m, nil
	case *microflows.RetrieveAction:
		m := map[string]any{"$Type": "Microflows$RetrieveAction"}
		if act.OutputVariable != "" {
			m["outputVariableName"] = act.OutputVariable
		}
		switch src := act.Source.(type) {
		case *microflows.AssociationRetrieveSource:
			m["byAssociation"] = map[string]any{
				"startVariableName": src.StartVariable,
				"association":       src.AssociationQualifiedName,
			}
		case *microflows.DatabaseRetrieveSource:
			if len(src.Sorting) > 0 {
				return nil, fmt.Errorf("retrieve: sorting is not yet supported by the MCP backend")
			}
			if src.Range != nil && src.Range.RangeType == microflows.RangeTypeCustom {
				return nil, fmt.Errorf("retrieve: custom range (offset/limit) is not yet supported by the MCP backend")
			}
			q := map[string]any{"entity": src.EntityQualifiedName}
			if src.XPathConstraint != "" {
				q["xPathConstraint"] = src.XPathConstraint
			}
			if src.Range != nil && src.Range.RangeType == microflows.RangeTypeFirst {
				q["takeOnlyFirst"] = true
			}
			m["byDatabaseQuery"] = q
		default:
			return nil, fmt.Errorf("retrieve: unsupported source %T", act.Source)
		}
		return m, nil
	case *microflows.LogMessageAction:
		level := string(act.LogLevel)
		if level == "" {
			level = "Info"
		}
		m := map[string]any{
			"$Type": "Microflows$LogMessageAction",
			"level": level,
			// LogMessage's messageTemplate is a StringTemplate element ($Type required).
			"messageTemplate": mfStringTemplate("Microflows$StringTemplate", act.MessageTemplate, act.TemplateParameters),
		}
		if act.LogNodeName != "" {
			m["node"] = act.LogNodeName
		}
		return m, nil
	default:
		return nil, fmt.Errorf("microflow action %T is not yet supported by the MCP backend", a)
	}
}

// mapCodeActionParameterValue maps a Java-action parameter value onto its PED
// element. Basic (expression) and entity-type values are supported; other code
// value kinds are rejected.
func mapCodeActionParameterValue(v microflows.CodeActionParameterValue) (map[string]any, error) {
	switch cv := v.(type) {
	case *microflows.BasicCodeActionParameterValue:
		return map[string]any{"$Type": "Microflows$BasicCodeActionParameterValue", "argument": cv.Argument}, nil
	case microflows.BasicCodeActionParameterValue:
		return map[string]any{"$Type": "Microflows$BasicCodeActionParameterValue", "argument": cv.Argument}, nil
	case *microflows.EntityTypeCodeActionParameterValue:
		return map[string]any{"$Type": "Microflows$EntityTypeCodeActionParameterValue", "entity": cv.Entity}, nil
	case microflows.EntityTypeCodeActionParameterValue:
		return map[string]any{"$Type": "Microflows$EntityTypeCodeActionParameterValue", "entity": cv.Entity}, nil
	default:
		return nil, fmt.Errorf("java action parameter value %T is not yet supported by the MCP backend", v)
	}
}

// mapListOperation maps a list operation onto its PED element. The common
// operations (head/tail, expression filter/find, set ops) are supported;
// attribute-based filter/find/sort and contains/equals/range are rejected.
func mapListOperation(op microflows.ListOperation) (map[string]any, error) {
	binary := func(t, a, b string) map[string]any {
		return map[string]any{"$Type": t, "listVariableName": a, "secondListOrObjectVariableName": b}
	}
	attributeListOp := func(t, list, attr, assoc, expr string) map[string]any {
		m := map[string]any{"$Type": t, "listVariableName": list, "expression": expr}
		if attr != "" {
			m["attribute"] = attr
		}
		if assoc != "" {
			m["association"] = assoc
		}
		return m
	}
	switch o := op.(type) {
	case *microflows.HeadOperation:
		return map[string]any{"$Type": "Microflows$Head", "listVariableName": o.ListVariable}, nil
	case *microflows.TailOperation:
		return map[string]any{"$Type": "Microflows$Tail", "listVariableName": o.ListVariable}, nil
	case *microflows.FilterOperation:
		return map[string]any{"$Type": "Microflows$FilterByExpression", "listVariableName": o.ListVariable, "expression": o.Expression}, nil
	case *microflows.FindOperation:
		return map[string]any{"$Type": "Microflows$FindByExpression", "listVariableName": o.ListVariable, "expression": o.Expression}, nil
	case *microflows.FilterByAttributeOperation:
		return attributeListOp("Microflows$Filter", o.ListVariable, o.Attribute, o.Association, o.Expression), nil
	case *microflows.FindByAttributeOperation:
		return attributeListOp("Microflows$Find", o.ListVariable, o.Attribute, o.Association, o.Expression), nil
	case *microflows.SortOperation:
		items := make([]any, 0, len(o.Sorting))
		for _, si := range o.Sorting {
			steps := make([]any, 0, len(si.EntityRefSteps))
			for _, st := range si.EntityRefSteps {
				step := map[string]any{"$Type": "DomainModels$EntityRefStep"}
				if st.Association != "" {
					step["association"] = st.Association
				}
				if st.DestinationEntity != "" {
					step["destinationEntity"] = st.DestinationEntity
				}
				steps = append(steps, step)
			}
			order := string(si.Direction)
			if order == "" {
				order = "Ascending"
			}
			items = append(items, map[string]any{
				"$Type": "Microflows$SortItem",
				"attributeRef": map[string]any{
					"$Type":     "DomainModels$AttributeRef",
					"attribute": si.AttributeQualifiedName,
					"entityRef": map[string]any{"$Type": "DomainModels$IndirectEntityRef", "steps": steps},
				},
				"sortOrder": order,
			})
		}
		return map[string]any{
			"$Type":            "Microflows$Sort",
			"listVariableName": o.ListVariable,
			"sortItemList":     map[string]any{"$Type": "Microflows$SortItemList", "items": items},
		}, nil
	case *microflows.UnionOperation:
		return binary("Microflows$Union", o.ListVariable1, o.ListVariable2), nil
	case *microflows.IntersectOperation:
		return binary("Microflows$Intersect", o.ListVariable1, o.ListVariable2), nil
	case *microflows.SubtractOperation:
		return binary("Microflows$Subtract", o.ListVariable1, o.ListVariable2), nil
	case *microflows.ContainsOperation:
		return map[string]any{
			"$Type":                          "Microflows$Contains",
			"listVariableName":               o.ListVariable,
			"secondListOrObjectVariableName": o.ObjectVariable,
		}, nil
	case *microflows.EqualsOperation:
		return map[string]any{
			"$Type":                          "Microflows$ListEquals",
			"listVariableName":               o.ListVariable1,
			"secondListOrObjectVariableName": o.ListVariable2,
		}, nil
	default:
		return nil, fmt.Errorf("list operation %T is not yet supported by the MCP backend", op)
	}
}

// mapCaseValue maps a sequence-flow case value onto the PED flow caseValue
// object ({enumerationCase} for boolean/enum splits, {inheritanceCase} for
// inheritance splits). A nil / NoCase value yields no caseValue (non-split
// flow). Boolean branches arrive as ExpressionCase{"true"|"false"}.
func mapCaseValue(cv microflows.CaseValue) (map[string]any, error) {
	switch c := cv.(type) {
	case nil:
		return nil, nil
	case *microflows.ExpressionCase:
		return map[string]any{"enumerationCase": c.Expression}, nil
	case microflows.ExpressionCase:
		return map[string]any{"enumerationCase": c.Expression}, nil
	case *microflows.EnumerationCase:
		return map[string]any{"enumerationCase": c.Value}, nil
	case microflows.EnumerationCase:
		return map[string]any{"enumerationCase": c.Value}, nil
	case *microflows.BooleanCase:
		return map[string]any{"enumerationCase": boolString(c.Value)}, nil
	case microflows.BooleanCase:
		return map[string]any{"enumerationCase": boolString(c.Value)}, nil
	case *microflows.InheritanceCase:
		return map[string]any{"inheritanceCase": c.EntityQualifiedName}, nil
	case microflows.InheritanceCase:
		return map[string]any{"inheritanceCase": c.EntityQualifiedName}, nil
	case *microflows.NoCase, microflows.NoCase:
		return nil, nil
	default:
		return nil, fmt.Errorf("unsupported flow case value %T", cv)
	}
}

func boolString(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

// mapMemberChanges maps a list of attribute/association member changes onto PED
// MemberChange elements ({attribute|association by-name, type, value}).
func mapMemberChanges(changes []*microflows.MemberChange) []any {
	items := make([]any, 0, len(changes))
	for _, c := range changes {
		m := map[string]any{
			"$Type": "Microflows$MemberChange",
			"type":  memberChangeType(c.Type),
			"value": c.Value,
		}
		if c.AttributeQualifiedName != "" {
			m["attribute"] = c.AttributeQualifiedName
		}
		if c.AssociationQualifiedName != "" {
			m["association"] = c.AssociationQualifiedName
		}
		items = append(items, m)
	}
	return items
}

func memberChangeType(t microflows.MemberChangeType) string {
	if t == "" {
		return "Set"
	}
	return string(t)
}

// mfCommitType maps a CommitType onto the PED commit enum (Yes / YesWithoutEvents / No).
func mfCommitType(c microflows.CommitType) string {
	switch c {
	case microflows.CommitTypeYes, microflows.CommitTypeYesWithEvents:
		return "Yes"
	case microflows.CommitTypeNoEvent:
		return "YesWithoutEvents"
	default:
		return "No"
	}
}

// mfStringTemplate builds a PED StringTemplate ({text, arguments}) from a
// localized text and its placeholder argument expressions. Shared by
// ShowMessage (template) and LogMessage (messageTemplate).
func mfStringTemplate(elementType string, text *model.Text, args []string) map[string]any {
	tmpl := map[string]any{"text": ""}
	if elementType != "" {
		tmpl["$Type"] = elementType
	}
	if text != nil {
		tmpl["text"] = text.Translations["en_US"]
	}
	if len(args) > 0 {
		tmpl["arguments"] = args
	}
	return tmpl
}

// mfVariableType maps a variable's DataType onto the PED CreateVariableAction
// variableType enum (primitives only).
func mfVariableType(dt microflows.DataType) (string, error) {
	if dt == nil {
		return "", fmt.Errorf("missing variable type")
	}
	switch dt.GetTypeName() {
	case "Boolean", "Decimal", "Integer", "String", "DateTime":
		return dt.GetTypeName(), nil
	case "Date":
		return "DateTime", nil
	default:
		return "", fmt.Errorf("variable type %q is not supported by the MCP backend (primitives only)", dt.GetTypeName())
	}
}

// mfDataType maps a microflow DataType onto the PED parameter/return type enum,
// returning (typeName, entityQualifiedName, enumerationQualifiedName). A nil
// DataType is Void (a microflow with no return value).
func mfDataType(dt microflows.DataType) (typeName, entity, enumeration string, err error) {
	if dt == nil {
		return "Void", "", "", nil
	}
	switch dt.GetTypeName() {
	case "Void":
		return "Void", "", "", nil
	case "Boolean", "Integer", "Decimal", "String", "DateTime":
		return dt.GetTypeName(), "", "", nil
	case "Date":
		return "DateTime", "", "", nil
	case "Object":
		return "Object", mfEntityName(dt), "", nil
	case "List":
		return "List", mfEntityName(dt), "", nil
	case "Enumeration":
		return "Enumeration", "", mfEnumName(dt), nil
	default:
		return "", "", "", fmt.Errorf("data type %q is not yet supported by the MCP backend", dt.GetTypeName())
	}
}

func mfEntityName(dt microflows.DataType) string {
	switch t := dt.(type) {
	case *microflows.ObjectType:
		return t.EntityQualifiedName
	case microflows.ObjectType:
		return t.EntityQualifiedName
	case *microflows.ListType:
		return t.EntityQualifiedName
	case microflows.ListType:
		return t.EntityQualifiedName
	}
	return ""
}

func mfEnumName(dt microflows.DataType) string {
	switch t := dt.(type) {
	case *microflows.EnumerationType:
		return t.EnumerationQualifiedName
	case microflows.EnumerationType:
		return t.EnumerationQualifiedName
	}
	return ""
}
