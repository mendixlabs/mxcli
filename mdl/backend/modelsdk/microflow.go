// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"strconv"
	"strings"

	"github.com/JordtenBulte-OLC/mxcli/modelsdk/element"
	genDT "github.com/JordtenBulte-OLC/mxcli/modelsdk/gen/datatypes"
	genMf "github.com/JordtenBulte-OLC/mxcli/modelsdk/gen/microflows"
	"github.com/JordtenBulte-OLC/mxcli/modelsdk/mprread"

	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/microflows"
)

// gen→microflows read adapter. Covers the breadth SHOW MICROFLOWS needs: name,
// module, excluded, return type, parameter count, and a flow-object collection
// faithful enough for the activity count and McCabe complexity (which key off
// the concrete object types: Start/End/Merge are skipped, splits/loops/error
// events drive complexity). Full per-activity detail (DESCRIBE) is a later phase.

func (b *Backend) ListMicroflows() ([]*microflows.Microflow, error) {
	units, err := mprread.ListUnitsWithContainer[*genMf.Microflow](b.reader)
	if err != nil {
		return nil, err
	}
	out := make([]*microflows.Microflow, 0, len(units))
	for _, u := range units {
		out = append(out, microflowFromGen(u.Element, u.ContainerID))
	}
	return out, nil
}

func (b *Backend) GetMicroflow(id model.ID) (*microflows.Microflow, error) {
	units, err := mprread.ListUnitsWithContainer[*genMf.Microflow](b.reader)
	if err != nil {
		return nil, err
	}
	for _, u := range units {
		if model.ID(u.Element.ID()) == id {
			return microflowFromGen(u.Element, u.ContainerID), nil
		}
	}
	return nil, nil
}

// IsRule reports whether qualifiedName refers to a rule (Microflows$Rule).
// Rules share the microflow namespace but are stored under a distinct BSON
// $Type, so the flow-builder needs the distinction to emit a RuleSplitCondition
// (not an ExpressionSplitCondition) for a rule-based `if Module.SomeRule(...)`.
//
// Without this the modelsdk backend fell back to the embedded `unimplemented`
// stub (which errors); the builder's `if err != nil || !isRule` guard then
// treated the rule call as a plain expression and emitted an invalid
// ExpressionSplitCondition → mx check CE0117 "Error in expression" (issue #723
// §A). Mirrors the legacy reader's IsRule.
func (b *Backend) IsRule(qualifiedName string) (bool, error) {
	if qualifiedName == "" {
		return false, nil
	}
	units, err := mprread.ListUnitsWithContainer[*genMf.Rule](b.reader)
	if err != nil {
		return false, err
	}
	for _, u := range units {
		name := u.Element.Name()
		if name == "" {
			continue
		}
		fullName := name
		if mod := b.moduleNameFor(model.ID(u.Element.ID())); mod != "" {
			fullName = mod + "." + name
		}
		if fullName == qualifiedName {
			return true, nil
		}
	}
	return false, nil
}

func (b *Backend) ListNanoflows() ([]*microflows.Nanoflow, error) {
	units, err := mprread.ListUnitsWithContainer[*genMf.Nanoflow](b.reader)
	if err != nil {
		return nil, err
	}
	out := make([]*microflows.Nanoflow, 0, len(units))
	for _, u := range units {
		out = append(out, nanoflowFromGen(u.Element, u.ContainerID))
	}
	return out, nil
}

func (b *Backend) GetNanoflow(id model.ID) (*microflows.Nanoflow, error) {
	units, err := mprread.ListUnitsWithContainer[*genMf.Nanoflow](b.reader)
	if err != nil {
		return nil, err
	}
	for _, u := range units {
		if model.ID(u.Element.ID()) == id {
			return nanoflowFromGen(u.Element, u.ContainerID), nil
		}
	}
	return nil, nil
}

// nanoflowFromGen mirrors microflowFromGen — nanoflows share the same parameter,
// flow-object, and return-type structures, so the same helpers apply.
func nanoflowFromGen(nf *genMf.Nanoflow, containerID model.ID) *microflows.Nanoflow {
	out := &microflows.Nanoflow{
		ContainerID:   containerID,
		Name:          nf.Name(),
		Documentation: nf.Documentation(),
		Excluded:      nf.Excluded(),
		ReturnType:    dataTypeFromGen(nf.MicroflowReturnType()),
	}
	out.ID = model.ID(nf.ID())
	for _, qn := range nf.AllowedModuleRolesQualifiedNames() {
		out.AllowedModuleRoles = append(out.AllowedModuleRoles, model.ID(qn))
	}
	params, objs := splitFlowObjects(nf.ObjectCollection())
	out.Parameters = params
	flows := flowsFromGen(nf.FlowsItems())
	annotFlows := annotationFlowsFromGen(nf.FlowsItems())
	if objs != nil || flows != nil || annotFlows != nil {
		out.ObjectCollection = &microflows.MicroflowObjectCollection{Objects: objs, Flows: flows, AnnotationFlows: annotFlows}
	}
	return out
}

func microflowFromGen(mf *genMf.Microflow, containerID model.ID) *microflows.Microflow {
	out := &microflows.Microflow{
		ContainerID:        containerID,
		Name:               mf.Name(),
		Documentation:      mf.Documentation(),
		Excluded:           mf.Excluded(),
		ReturnVariableName: mf.ReturnVariableName(),
		ReturnType:         dataTypeFromGen(mf.MicroflowReturnType()),
		// Read back the execution flags (issue #723 §A). Without these, an
		// UpdateMicroflow round-trip reset them to false: a microflow that
		// allowed concurrent execution came back as "disallow" with no error
		// message configured → mx check CE4899.
		AllowConcurrentExecution: mf.AllowConcurrentExecution(),
		MarkAsUsed:               mf.MarkAsUsed(),
	}
	out.ID = model.ID(mf.ID())
	// AllowedModuleRoles (BY_NAME role references) — without these DESCRIBE omits
	// the "grant execute on microflow … to …" line that legacy emits.
	for _, qn := range mf.AllowedModuleRolesQualifiedNames() {
		out.AllowedModuleRoles = append(out.AllowedModuleRoles, model.ID(qn))
	}
	params, objs := splitFlowObjects(mf.ObjectCollection())
	out.Parameters = params
	// Flows live on the gen Microflow, but the model keeps them in the object
	// collection (where the DESCRIBE flow-graph traversal reads them). Without
	// the edges, traversal from the start event has nowhere to go and the body
	// renders empty.
	flows := flowsFromGen(mf.FlowsItems())
	annotFlows := annotationFlowsFromGen(mf.FlowsItems())
	if objs != nil || flows != nil || annotFlows != nil {
		out.ObjectCollection = &microflows.MicroflowObjectCollection{Objects: objs, Flows: flows, AnnotationFlows: annotFlows}
	}
	return out
}

// flowsFromGen reconstructs the sequence-flow edges (origin/destination + branch
// case) from a gen flow list, so DESCRIBE can order activities by the flow graph.
func flowsFromGen(items []element.Element) []*microflows.SequenceFlow {
	var flows []*microflows.SequenceFlow
	for _, el := range items {
		g, ok := el.(*genMf.SequenceFlow)
		if !ok {
			continue
		}
		f := &microflows.SequenceFlow{
			OriginID:                   model.ID(g.OriginRefID()),
			DestinationID:              model.ID(g.DestinationRefID()),
			OriginConnectionIndex:      int(g.OriginConnectionIndex()),
			DestinationConnectionIndex: int(g.DestinationConnectionIndex()),
			IsErrorHandler:             g.IsErrorHandler(),
			CaseValue:                  caseValueFromGen(g),
		}
		f.ID = model.ID(g.ID())
		flows = append(flows, f)
	}
	return flows
}

// annotationFlowsFromGen reconstructs the annotation→activity connections. They
// live in the same gen flow list as the sequence flows (FlowsItems), so this
// filters out the AnnotationFlow entries; buildAnnotationsByTarget joins each
// flow's origin (the annotation) to its destination (the activity).
func annotationFlowsFromGen(items []element.Element) []*microflows.AnnotationFlow {
	var flows []*microflows.AnnotationFlow
	for _, el := range items {
		g, ok := el.(*genMf.AnnotationFlow)
		if !ok {
			continue
		}
		f := &microflows.AnnotationFlow{
			OriginID:      model.ID(g.OriginRefID()),
			DestinationID: model.ID(g.DestinationRefID()),
		}
		f.ID = model.ID(g.ID())
		flows = append(flows, f)
	}
	return flows
}

// loopSourceFromGen reconstructs a loop's source: iterate-over-list (iterator +
// list variable) or a while-condition. Inverse of loopSourceToGen.
func loopSourceFromGen(el element.Element) microflows.LoopSource {
	switch g := el.(type) {
	case *genMf.IterableList:
		return &microflows.IterableList{ListVariableName: g.ListVariableName(), VariableName: g.VariableName()}
	case *genMf.WhileLoopCondition:
		return &microflows.WhileLoopCondition{WhileExpression: g.WhileExpression()}
	default:
		return nil
	}
}

// splitConditionFromGen reconstructs an exclusive split's condition (the `if <expr>`
// expression). Inverse of splitConditionToGen.
func splitConditionFromGen(el element.Element) microflows.SplitCondition {
	switch g := el.(type) {
	case *genMf.ExpressionSplitCondition:
		return &microflows.ExpressionSplitCondition{Expression: g.Expression()}
	case *genMf.RuleSplitCondition:
		// A rule-based split. Without this the condition reads as nil and the
		// renderer falls back to "if true then …", losing the real rule call.
		rc, ok := g.RuleCall().(*genMf.RuleCall)
		if !ok || rc == nil {
			return nil
		}
		// The rule reference is stored under the "Microflow" key (rules share the
		// microflow namespace — see sdk/mpr/parser_microflow.go:479). Gen decodes
		// the property as "Rule", so RuleQualifiedName() is empty here; read the
		// storage key off the raw BSON, as the action readers do for other
		// storage-name fields.
		name := rc.RuleQualifiedName()
		if name == "" {
			if s, ok := rc.Raw().Lookup("Microflow").StringValueOK(); ok {
				name = s
			}
		}
		cond := &microflows.RuleSplitCondition{RuleQualifiedName: name}
		for _, el := range rc.ParameterMappingsItems() {
			pm, ok := el.(*genMf.RuleCallParameterMapping)
			if !ok {
				continue
			}
			cond.ParameterMappings = append(cond.ParameterMappings, &microflows.RuleCallParameterMapping{
				ParameterName: pm.ParameterQualifiedName(),
				Argument:      pm.Argument(),
			})
		}
		return cond
	}
	return nil
}

// caseValueFromGen reconstructs a sequence flow's branch case (the true/false/enum
// label that findBranchFlows and the enum/inheritance-split renderers key on).
//
// The case can live in either of two places: the singular `caseValue`/`NewCaseValue`
// child (the form Studio Pro still writes for boolean and enumeration splits — see
// the codec fieldAliases "CaseValue"→"NewCaseValue") or the plural `caseValues`
// list (newer format). Read the singular first, then the list. Both expression and
// enum cases are stored as a gen EnumerationCase (Value carries the label);
// inheritance cases as a gen InheritanceCase; a normal flow's NoCase yields nil.
func caseValueFromGen(g *genMf.SequenceFlow) microflows.CaseValue {
	if cv := mapGenCaseValue(g.CaseValue()); cv != nil {
		return cv
	}
	for _, el := range g.CaseValuesItems() {
		if cv := mapGenCaseValue(el); cv != nil {
			return cv
		}
	}
	return nil
}

// mapGenCaseValue converts a single gen case element to its model form, or nil
// when the element is absent or a NoCase (no branch label).
func mapGenCaseValue(el element.Element) microflows.CaseValue {
	switch c := el.(type) {
	case *genMf.EnumerationCase:
		return &microflows.EnumerationCase{Value: c.Value()}
	case *genMf.InheritanceCase:
		return &microflows.InheritanceCase{EntityQualifiedName: c.ValueQualifiedName()}
	}
	return nil
}

// splitFlowObjects separates parameter objects (which our model keeps in
// Microflow.Parameters) from flow activities (kept in ObjectCollection.Objects),
// mirroring the legacy parser so parameter and activity counts both match.
func splitFlowObjects(coll element.Element) ([]*microflows.MicroflowParameter, []microflows.MicroflowObject) {
	mc, ok := coll.(*genMf.MicroflowObjectCollection)
	if !ok || mc == nil {
		return nil, nil
	}
	var params []*microflows.MicroflowParameter
	var objs []microflows.MicroflowObject
	for _, el := range mc.ObjectsItems() {
		if po, ok := el.(*genMf.MicroflowParameter); ok {
			p := &microflows.MicroflowParameter{Name: po.Name(), Type: dataTypeFromGen(po.ParameterType())}
			p.ID = model.ID(el.ID())
			params = append(params, p)
			continue
		}
		o := flowObjectFromGen(el)
		// Carry the object's box size (@size) alongside its position, so a
		// write → read → write round-trip keeps the real box dimensions
		// instead of resetting them to 0;0 (issue #723 §A: 1-px slivers in
		// Studio Pro). Nested loop-body objects go through this same recursive
		// call, so they are covered too.
		if s, ok := o.(interface{ SetSize(model.Size) }); ok {
			s.SetSize(sizeFromGen(el))
		}
		objs = append(objs, o)
	}
	return params, objs
}

// flowObjectFromGen maps a gen flow object to the concrete our-model type the
// activity/complexity counters discriminate on. Non-control objects collapse to
// ActionActivity (sufficient for counting); LoopedActivity recurses so decision
// points inside loop bodies are counted.
// pointFromGen reads a flow object's canvas position from its RelativeMiddlePoint
// accessor (a "X;Y" string, e.g. "570;297"); mirrors the legacy parsePoint. All
// gen flow objects expose RelativeMiddlePoint() via their embedded base.
func pointFromGen(el element.Element) model.Point {
	rm, ok := el.(interface{ RelativeMiddlePoint() string })
	if !ok {
		return model.Point{}
	}
	parts := strings.SplitN(rm.RelativeMiddlePoint(), ";", 2)
	if len(parts) != 2 {
		return model.Point{}
	}
	x, _ := strconv.Atoi(strings.TrimSpace(parts[0]))
	y, _ := strconv.Atoi(strings.TrimSpace(parts[1]))
	return model.Point{X: x, Y: y}
}

// sizeFromGen reads a flow object's box size from its Size accessor (a "W;H"
// string, e.g. "120;60"); mirrors pointFromGen. Without it the read path left
// every node at size 0;0, so a round-trip (write → read → write) shrank each
// activity/decision to a 1-px sliver in Studio Pro (issue #723 §A). All gen flow
// objects expose Size() via their embedded base.
func sizeFromGen(el element.Element) model.Size {
	sz, ok := el.(interface{ Size() string })
	if !ok {
		return model.Size{}
	}
	parts := strings.SplitN(sz.Size(), ";", 2)
	if len(parts) != 2 {
		return model.Size{}
	}
	w, _ := strconv.Atoi(strings.TrimSpace(parts[0]))
	h, _ := strconv.Atoi(strings.TrimSpace(parts[1]))
	return model.Size{Width: w, Height: h}
}

func flowObjectFromGen(el element.Element) microflows.MicroflowObject {
	// Carry the real object ID: the catalog keys activities_data on the activity Id,
	// so bare objects with an empty ID collide (UNIQUE constraint on
	// activities_data.Id) on the second activity of any microflow (full-mode catalog
	// / REFRESH CATALOG FULL). ID is the promoted public field from BaseElement.
	id := model.ID(el.ID())
	// Canvas position (@position) lives on every flow object as a "X;Y"
	// RelativeMiddlePoint string; reconstruct it so the diagram layout round-trips.
	pos := pointFromGen(el)
	switch el.TypeName() {
	case "Microflows$StartEvent":
		o := &microflows.StartEvent{}
		o.ID = id
		o.Position = pos
		return o
	case "Microflows$EndEvent":
		o := &microflows.EndEvent{}
		o.ID = id
		o.Position = pos
		if g, ok := el.(*genMf.EndEvent); ok {
			o.ReturnValue = g.ReturnValue()
		}
		return o
	case "Microflows$ExclusiveMerge":
		o := &microflows.ExclusiveMerge{}
		o.ID = id
		o.Position = pos
		return o
	case "Microflows$ExclusiveSplit":
		o := &microflows.ExclusiveSplit{}
		o.ID = id
		o.Position = pos
		if g, ok := el.(*genMf.ExclusiveSplit); ok {
			o.Caption = g.Caption()
			o.SplitCondition = splitConditionFromGen(g.SplitCondition())
		}
		return o
	case "Microflows$InheritanceSplit":
		o := &microflows.InheritanceSplit{}
		o.ID = id
		o.Position = pos
		if g, ok := el.(*genMf.InheritanceSplit); ok {
			// Without the split variable the loop header renders "split type $"
			// (empty), and without the caption the @caption line is dropped.
			o.VariableName = g.SplitVariableName()
			o.Caption = g.Caption()
			o.Documentation = g.Documentation()
		}
		return o
	case "Microflows$Annotation":
		// Sticky-note annotation. Not reached by the sequence-flow traversal (it is
		// attached via an AnnotationFlow), so it never renders as a statement, but
		// collectAnnotationCaptions needs it in Objects with its caption to emit the
		// @annotation line on the activity the AnnotationFlow points at.
		o := &microflows.Annotation{}
		o.ID = id
		o.Position = pos
		if g, ok := el.(*genMf.Annotation); ok {
			o.Caption = g.Caption()
		}
		return o
	case "Microflows$ErrorEvent":
		o := &microflows.ErrorEvent{}
		o.ID = id
		o.Position = pos
		return o
	case "Microflows$BreakEvent":
		// Loop break — without this case it falls through to ActionActivity and
		// renders "-- Empty action" instead of "break;".
		o := &microflows.BreakEvent{}
		o.ID = id
		o.Position = pos
		return o
	case "Microflows$ContinueEvent":
		// Loop continue — see BreakEvent above ("continue;").
		o := &microflows.ContinueEvent{}
		o.ID = id
		o.Position = pos
		return o
	case "Microflows$LoopedActivity":
		la := &microflows.LoopedActivity{}
		la.ID = id
		la.Position = pos
		if g, ok := el.(*genMf.LoopedActivity); ok {
			la.LoopSource = loopSourceFromGen(g.LoopSource())
			if _, objs := splitFlowObjects(g.ObjectCollection()); objs != nil {
				la.ObjectCollection = &microflows.MicroflowObjectCollection{Objects: objs}
			}
		}
		return la
	default:
		o := &microflows.ActionActivity{}
		o.ID = id
		o.Position = pos
		// Reconstruct the action body so DESCRIBE/SHOW can render it. Unhandled
		// action types leave Action nil (renders empty), so coverage grows
		// incrementally without regressing.
		if g, ok := el.(*genMf.ActionActivity); ok {
			// Activity-level annotations (@caption / @color / @excluded /
			// documentation) live on the activity, not the action, so reconstruct
			// them too — emitObjectAnnotations renders them around the action body.
			o.Caption = g.Caption()
			o.AutoGenerateCaption = g.AutoGenerateCaption()
			o.BackgroundColor = g.BackgroundColor()
			o.Disabled = g.Disabled()
			o.Documentation = g.Documentation()
			if act := g.Action(); act != nil {
				o.Action = actionFromGen(act)
			}
		}
		return o
	}
}

// dataTypeFromGen maps a gen DataTypes$* return-type element to our DataType.
// Only GetTypeName() fidelity is needed for SHOW; nil means no declared type.
func dataTypeFromGen(el element.Element) microflows.DataType {
	if el == nil {
		return nil
	}
	// Return pointer types: the describe formatter (formatMicroflowDataType) and
	// microflowHasReturnValue both type-switch on *microflows.X, matching the
	// legacy reader — value types would silently fall through (showing "Unknown",
	// and misdetecting Void so the void `return;` is dropped). Object/List carry
	// the entity name and Enumeration the enum name so "List of Module.Entity"
	// renders instead of a bare "List".
	// Long has no distinct gen type; match it by storage name before the switch.
	if el.TypeName() == "DataTypes$LongType" {
		return &microflows.LongType{}
	}
	switch g := el.(type) {
	case *genDT.BooleanType:
		return &microflows.BooleanType{}
	case *genDT.IntegerType:
		return &microflows.IntegerType{}
	case *genDT.DecimalType:
		return &microflows.DecimalType{}
	case *genDT.StringType:
		return &microflows.StringType{}
	case *genDT.DateTimeType:
		return &microflows.DateTimeType{}
	case *genDT.VoidType:
		return &microflows.VoidType{}
	case *genDT.ObjectType:
		return &microflows.ObjectType{EntityQualifiedName: g.EntityQualifiedName()}
	case *genDT.ListType:
		return &microflows.ListType{EntityQualifiedName: g.EntityQualifiedName()}
	case *genDT.EnumerationType:
		return &microflows.EnumerationType{EnumerationQualifiedName: g.EnumerationQualifiedName()}
	default:
		return nil
	}
}
