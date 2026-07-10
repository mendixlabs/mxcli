// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"fmt"
	"sort"

	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/modelsdk/codec"
	"github.com/JordtenBulte-OLC/mxcli/modelsdk/element"
	genDT "github.com/JordtenBulte-OLC/mxcli/modelsdk/gen/datatypes"
	genDom "github.com/JordtenBulte-OLC/mxcli/modelsdk/gen/domainmodels"
	genMf "github.com/JordtenBulte-OLC/mxcli/modelsdk/gen/microflows"
	genTexts "github.com/JordtenBulte-OLC/mxcli/modelsdk/gen/texts"
	mmpr "github.com/JordtenBulte-OLC/mxcli/modelsdk/mpr"
	"github.com/JordtenBulte-OLC/mxcli/modelsdk/property"
	"github.com/JordtenBulte-OLC/mxcli/sdk/microflows"
)

func init() {
	// The microflow wrapper always serializes these two action-info slots as null.
	codec.RegisterTypeDefaults("Microflows$Microflow", codec.TypeDefaults{
		NullFields: []string{"MicroflowActionInfo", "WorkflowActionInfo"},
		// StableId is a GUID stored as binary; the gen mistypes it as a string, so
		// emit it via the fresh-GUID default instead (verified vs test7-app).
		FreshGUIDFields: []string{"StableId"},
	})
	// A SequenceFlow's CaseValues list uses typed-array marker 2 (like an index's
	// IndexedAttribute list), not the default 3. Keyed by the case child types.
	for _, t := range []string{"Microflows$NoCase", "Microflows$EnumerationCase", "Microflows$ExpressionCase", "Microflows$InheritanceCase"} {
		codec.RegisterListMarker(t, 2)
	}
	// Create/Change actions store their member-change Items list with marker 2.
	codec.RegisterListMarker("Microflows$ChangeActionItem", 2)
	// A retrieve's SortingsList always serializes its (possibly empty) Sortings
	// list with marker 2 (verified vs real BSON: test7 PopulateUserAttributes).
	codec.RegisterTypeDefaults("Microflows$SortingsList", codec.TypeDefaults{
		MandatoryListMarkers: map[string]int32{"Sortings": 2},
	})
	codec.RegisterListMarker("Microflows$RetrieveSorting", 2)
	// Call actions: ParameterMappings list always emitted (marker 2); a MicroflowCall
	// always serializes QueueSettings as null.
	codec.RegisterTypeDefaults("Microflows$MicroflowCall", codec.TypeDefaults{
		NullFields:           []string{"QueueSettings"},
		MandatoryListMarkers: map[string]int32{"ParameterMappings": 2},
	})
	codec.RegisterTypeDefaults("Microflows$NanoflowCall", codec.TypeDefaults{
		MandatoryListMarkers: map[string]int32{"ParameterMappings": 2},
	})
	codec.RegisterListMarker("Microflows$MicroflowCallParameterMapping", 2)
	codec.RegisterListMarker("Microflows$NanoflowCallParameterMapping", 2)
	// A JavaActionCallAction always serializes QueueSettings as null; its
	// ParameterMappings list empties as marker 2; a StringTemplateParameterValue's
	// TypedTemplate empties its Arguments list as marker 2 (legacy writer).
	codec.RegisterTypeDefaults("Microflows$JavaActionCallAction", codec.TypeDefaults{
		NullFields: []string{"QueueSettings"},
	})
	codec.RegisterListMarker("Microflows$JavaActionParameterMapping", 2)
	codec.RegisterTypeDefaults("Microflows$TypedTemplate", codec.TypeDefaults{
		MandatoryListMarkers: map[string]int32{"Arguments": 2},
	})
	// EXECUTE DATABASE QUERY action: both mapping lists empty as marker 2; their
	// child mapping $Types use marker 2 too.
	codec.RegisterListMarker("DatabaseConnector$ConnectionParameterMapping", 2)
	codec.RegisterListMarker("DatabaseConnector$QueryParameterMapping", 2)
	codec.RegisterTypeDefaults("DatabaseConnector$ExecuteDatabaseQueryAction", codec.TypeDefaults{
		MandatoryListMarkers: map[string]int32{"ConnectionParameterMappings": 2, "ParameterMappings": 2},
	})
	// A ListOperationAction's Sort operation reuses the same Microflows$SortingsList
	// envelope as a retrieve (registered above: empty "Sortings" = marker 2,
	// RetrieveSorting child marker = 2). Studio Pro tolerates the inner Sortings
	// marker (2 vs the legacy list-op writer's 3 — see the CE0463 tolerance note),
	// so no additional registration is needed here.
	// A DomainModels$IndirectEntityRef's "Steps" list uses marker 2.
	codec.RegisterListMarker("DomainModels$EntityRefStep", 2)
	// A ValidationFeedbackAction's FeedbackTemplate is a Microflows$TextTemplate
	// whose "Parameters" list empties as marker 2 (legacy serializeTextTemplate);
	// the nested template-parameter children themselves use marker 3 (the
	// encoder default) when present.
	codec.RegisterTypeDefaults("Microflows$TextTemplate", codec.TypeDefaults{
		MandatoryListMarkers: map[string]int32{"Parameters": 2},
	})
	// NOTE: do NOT globally register a marker for Texts$Text's "Items" list — an
	// empty Texts$Text (e.g. every microflow's ConcurrencyErrorMessage) serializes
	// with the default marker 3, and a global override breaks write-parity for all
	// microflows. The ShowMessage/ValidationFeedback TextTemplate's nested Texts$Text
	// marker is cosmetic (CE0463-tolerated) so we leave it at the default.
	// REST call sub-elements. The HttpConfiguration's HttpHeaderEntries list and a
	// StringTemplate's Parameters list both empty as marker 2 (legacy writer); their
	// child element $Types use marker 2 too. A ShowPage FormSettings' ParameterMappings
	// likewise empties as marker 2. These trees are built directly with the verified
	// storage keys (newElem/addStr/...), so register the per-$Type list markers here.
	codec.RegisterListMarker("Microflows$HttpHeaderEntry", 2)
	codec.RegisterListMarker("Microflows$TemplateParameter", 2)
	codec.RegisterListMarker("Forms$PageParameterMapping", 2)
	codec.RegisterTypeDefaults("Microflows$HttpConfiguration", codec.TypeDefaults{
		MandatoryListMarkers: map[string]int32{"HttpHeaderEntries": 2},
	})
	codec.RegisterTypeDefaults("Microflows$StringTemplate", codec.TypeDefaults{
		MandatoryListMarkers: map[string]int32{"Parameters": 2},
	})
	// A REST call always serializes ProxyConfiguration as null (legacy
	// serializeRestCallAction). A ResultHandling serializes ImportMappingCall as null
	// unless the result is mapped — for the mapped case the field is set explicitly,
	// so the NullField default only fires when it was not emitted.
	codec.RegisterTypeDefaults("Microflows$RestCallAction", codec.TypeDefaults{
		NullFields: []string{"ProxyConfiguration"},
	})
	codec.RegisterTypeDefaults("Microflows$ResultHandling", codec.TypeDefaults{
		NullFields: []string{"ImportMappingCall"},
	})
	// A ShowPage FormSettings always carries a TitleOverride (empty Microflows$TextTemplate)
	// and each PageParameterMapping a Variable (empty Forms$PageVariable); both are built
	// directly. FormSettings' ParameterMappings list empties as marker 2.
	codec.RegisterTypeDefaults("Forms$FormSettings", codec.TypeDefaults{
		MandatoryListMarkers: map[string]int32{"ParameterMappings": 2},
	})
}

// majorVersion returns the project's Mendix major version (for version-gated BSON).
func (b *Backend) majorVersion() int {
	if pv := b.ProjectVersion(); pv != nil {
		return pv.MajorVersion
	}
	return 11
}

// CreateMicroflow adds a new microflow document (a top-level unit).
func (b *Backend) CreateMicroflow(mf *microflows.Microflow) error {
	if mf == nil {
		return fmt.Errorf("CreateMicroflow: nil microflow")
	}
	if b.writer == nil {
		return fmt.Errorf("CreateMicroflow: not connected for writing")
	}
	if mf.ID == "" {
		mf.ID = model.ID(mmpr.GenerateID())
	}
	gm := microflowToGen(mf, b.majorVersion())
	gm.SetID(element.ID(mf.ID))
	assignMicroflowIDs(gm)
	contents, err := (&codec.Encoder{}).Encode(gm)
	if err != nil {
		return fmt.Errorf("CreateMicroflow: encode: %w", err)
	}
	return b.writer.InsertUnit(string(mf.ID), string(mf.ContainerID), "Documents", "Microflows$Microflow", contents)
}

// UpdateMicroflow rebuilds a microflow document (the CREATE OR REPLACE path).
func (b *Backend) UpdateMicroflow(mf *microflows.Microflow) error {
	if mf == nil {
		return fmt.Errorf("UpdateMicroflow: nil microflow")
	}
	if b.writer == nil {
		return fmt.Errorf("UpdateMicroflow: not connected for writing")
	}
	gm := microflowToGen(mf, b.majorVersion())
	gm.SetID(element.ID(mf.ID))
	assignMicroflowIDs(gm)
	contents, err := (&codec.Encoder{}).Encode(gm)
	if err != nil {
		return fmt.Errorf("UpdateMicroflow: encode: %w", err)
	}
	return b.writer.UpdateRawUnit(string(mf.ID), contents)
}

// DeleteMicroflow removes the microflow unit.
func (b *Backend) DeleteMicroflow(id model.ID) error {
	if b.writer == nil {
		return fmt.Errorf("DeleteMicroflow: not connected for writing")
	}
	return b.writer.DeleteUnit(string(id))
}

// microflowToGen builds a gen Microflow wrapper from the model. Mirrors the legacy
// serializer (writer_microflow.go). v10+ adds ReturnVariableName/StableId/Url/
// UrlSearchParameters.
func microflowToGen(mf *microflows.Microflow, major int) *genMf.Microflow {
	out := genMf.NewMicroflow()
	out.SetName(mf.Name)
	out.SetDocumentation(mf.Documentation)
	out.SetExcluded(mf.Excluded)
	out.SetExportLevel("Hidden")
	out.SetAllowConcurrentExecution(mf.AllowConcurrentExecution)
	out.SetApplyEntityAccess(false)
	out.SetMarkAsUsed(mf.MarkAsUsed)
	out.SetConcurrencyErrorMicroflowQualifiedName("")
	out.SetConcurrencyErrorMessage(genTexts.NewText()) // empty Texts$Text (Items=[3] via default)
	out.SetAllowedModuleRolesQualifiedNames(moduleRoleNames(mf.AllowedModuleRoles))
	out.SetMicroflowReturnType(microflowDataTypeToGen(mf.ReturnType))

	// Object collection (parameters merged first, then objects).
	oc := genMf.NewMicroflowObjectCollection()
	for i, p := range mf.Parameters {
		oc.AddObjects(microflowParameterToGen(p, i, major))
	}
	if mf.ObjectCollection != nil {
		for _, obj := range mf.ObjectCollection.Objects {
			if g := microflowObjectToGen(obj); g != nil {
				oc.AddObjects(g)
			}
		}
	}
	out.SetObjectCollection(oc)

	// Flows live on the microflow, not in the object collection. Sequence flows
	// and annotation flows share the same Flows list.
	if mf.ObjectCollection != nil {
		for _, f := range mf.ObjectCollection.Flows {
			out.AddFlows(sequenceFlowToGen(f, major))
		}
		for _, af := range mf.ObjectCollection.AnnotationFlows {
			out.AddFlows(annotationFlowToGen(af, major))
		}
	}

	if major >= 10 {
		out.SetReturnVariableName(mf.ReturnVariableName)
		out.SetUrl("")
		// StableId is emitted as a fresh GUID binary via the registered default
		// (the gen mistypes it as a string), not set here.
		out.SetUrlSearchParametersQualifiedNames(nil) // empty marker-1 list
	}
	return out
}

// moduleRoleNames renders allowed-module-role IDs as the by-name list the codec
// emits. Empty input yields an empty (marker-1) list, matching the legacy writer.
func moduleRoleNames(ids []model.ID) []string {
	if len(ids) == 0 {
		return nil
	}
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		out = append(out, string(id))
	}
	return out
}

// microflowObjectToGen dispatches a flow object to its gen element. Skeleton:
// start/end events; activities are added group by group.
func microflowObjectToGen(obj microflows.MicroflowObject) element.Element {
	switch o := obj.(type) {
	case *microflows.StartEvent:
		g := genMf.NewStartEvent()
		g.SetID(element.ID(o.ID))
		g.SetRelativeMiddlePoint(pointStr(o.Position))
		g.SetSize(sizeStr(o.Size))
		return g
	case *microflows.EndEvent:
		g := genMf.NewEndEvent()
		g.SetID(element.ID(o.ID))
		g.SetDocumentation("")
		g.SetRelativeMiddlePoint(pointStr(o.Position))
		g.SetReturnValue(o.ReturnValue)
		g.SetSize(sizeStr(o.Size))
		return g
	case *microflows.ErrorEvent:
		// `raise error` in a custom error handler. Dropping it (the old default)
		// left the error-handler SequenceFlow pointing at a non-existent object →
		// Studio Pro/mx crash "KeyNotFoundException" in LoadChildUnits.
		g := genMf.NewErrorEvent()
		g.SetID(element.ID(o.ID))
		g.SetRelativeMiddlePoint(pointStr(o.Position))
		g.SetSize(sizeStr(o.Size))
		return g
	case *microflows.ActionActivity:
		g := genMf.NewActionActivity()
		g.SetID(element.ID(o.ID))
		if a := microflowActionToGen(o.Action); a != nil {
			g.SetAction(a)
		}
		g.SetAutoGenerateCaption(o.AutoGenerateCaption)
		g.SetBackgroundColor(orDefault(o.BackgroundColor, "Default"))
		g.SetCaption(o.Caption)
		g.SetDisabled(o.Disabled)
		g.SetDocumentation(o.Documentation)
		g.SetRelativeMiddlePoint(pointStr(o.Position))
		g.SetSize(sizeStr(o.Size))
		return g
	case *microflows.ExclusiveSplit:
		g := genMf.NewExclusiveSplit()
		g.SetID(element.ID(o.ID))
		g.SetCaption(o.Caption)
		g.SetDocumentation(o.Documentation)
		g.SetErrorHandlingType(string(o.ErrorHandlingType))
		g.SetRelativeMiddlePoint(pointStr(o.Position))
		g.SetSize(sizeStr(o.Size))
		if sc := splitConditionToGen(o.SplitCondition); sc != nil {
			g.SetSplitCondition(sc)
		}
		return g
	case *microflows.ExclusiveMerge:
		g := genMf.NewExclusiveMerge()
		g.SetID(element.ID(o.ID))
		g.SetRelativeMiddlePoint(pointStr(o.Position))
		g.SetSize(sizeStr(o.Size))
		return g
	case *microflows.LoopedActivity:
		g := genMf.NewLoopedActivity()
		g.SetID(element.ID(o.ID))
		g.SetErrorHandlingType(string(o.ErrorHandlingType))
		if ls := loopSourceToGen(o.LoopSource); ls != nil {
			g.SetLoopSource(ls)
		}
		// Nested loop body: objects only (the gen/legacy ObjectCollection carries
		// no inline Flows — loop-external flows live on the top-level Microflow).
		if o.ObjectCollection != nil {
			noc := genMf.NewMicroflowObjectCollection()
			for _, obj := range o.ObjectCollection.Objects {
				if e := microflowObjectToGen(obj); e != nil {
					noc.AddObjects(e)
				}
			}
			g.SetObjectCollection(noc)
		}
		g.SetRelativeMiddlePoint(pointStr(o.Position))
		g.SetSize(sizeStr(o.Size))
		return g
	case *microflows.Annotation:
		// A microflow annotation (yellow note); its caption is the note text.
		// Connected to an activity by an AnnotationFlow (serialized in
		// microflowToGen). Without this, `@annotation` was silently dropped.
		g := genMf.NewAnnotation()
		g.SetID(element.ID(o.ID))
		g.SetCaption(o.Caption)
		g.SetRelativeMiddlePoint(pointStr(o.Position))
		g.SetSize(sizeStr(o.Size))
		return g
	default:
		return nil // unsupported object type (added in later activity groups)
	}
}

// annotationFlowToGen builds a Microflows$AnnotationFlow connecting an Annotation
// (origin) to the activity it documents (destination). The line shape is
// version-specific, mirroring the legacy serializeAnnotationFlow.
func annotationFlowToGen(af *microflows.AnnotationFlow, major int) element.Element {
	g := genMf.NewAnnotationFlow()
	g.SetID(element.ID(af.ID))
	g.SetOriginID(element.ID(af.OriginID))
	g.SetDestinationID(element.ID(af.DestinationID))
	g.SetOriginConnectionIndex(0)
	g.SetDestinationConnectionIndex(0)
	if major <= 9 {
		g.SetOriginBezierVector("0;0")
		g.SetDestinationBezierVector("0;0")
	} else {
		line := genMf.NewBezierCurve()
		line.SetOriginControlVector("0;0")
		line.SetDestinationControlVector("0;0")
		g.SetLine(line)
	}
	return g
}

// loopSourceToGen builds a loop's source (iterate-over-list or while-condition).
func loopSourceToGen(ls microflows.LoopSource) element.Element {
	switch s := ls.(type) {
	case *microflows.IterableList:
		g := genMf.NewIterableList()
		g.SetID(element.ID(s.ID))
		g.SetListVariableName(s.ListVariableName)
		g.SetVariableName(s.VariableName)
		return g
	case *microflows.WhileLoopCondition:
		g := genMf.NewWhileLoopCondition()
		g.SetID(element.ID(s.ID))
		g.SetWhileExpression(s.WhileExpression)
		return g
	default:
		return nil
	}
}

// microflowActionToGen dispatches a microflow action to its gen element. Object
// operations group: create/change/commit/delete/rollback. Uses the storage $Type
// (set by the gen constructors). Returns nil for not-yet-supported actions.
func microflowActionToGen(action microflows.MicroflowAction) element.Element {
	switch a := action.(type) {
	case *microflows.CreateObjectAction:
		g := genMf.NewCreateObjectAction()
		g.SetID(element.ID(a.ID))
		g.SetCommit(string(a.Commit))
		if a.EntityQualifiedName != "" {
			g.SetEntityQualifiedName(a.EntityQualifiedName)
		}
		g.SetErrorHandlingType(orDefault(string(a.ErrorHandlingType), "Rollback"))
		for _, m := range a.InitialMembers {
			g.AddItems(memberChangeToGen(m))
		}
		g.SetRefreshInClient(false)
		g.SetOutputVariableName(a.OutputVariable)
		return g
	case *microflows.ChangeObjectAction:
		g := genMf.NewChangeObjectAction()
		g.SetID(element.ID(a.ID))
		g.SetChangeVariableName(a.ChangeVariable)
		g.SetCommit(string(a.Commit))
		g.SetErrorHandlingType(orDefault(string(a.ErrorHandlingType), "Rollback"))
		for _, m := range a.Changes {
			g.AddItems(memberChangeToGen(m))
		}
		g.SetRefreshInClient(a.RefreshInClient)
		return g
	case *microflows.CommitObjectsAction:
		g := genMf.NewCommitAction()
		g.SetID(element.ID(a.ID))
		g.SetCommitVariableName(a.CommitVariable)
		g.SetErrorHandlingType(orDefault(string(a.ErrorHandlingType), "Rollback"))
		g.SetRefreshInClient(a.RefreshInClient)
		g.SetWithEvents(a.WithEvents)
		return g
	case *microflows.DeleteObjectAction:
		g := genMf.NewDeleteAction()
		g.SetID(element.ID(a.ID))
		g.SetDeleteVariableName(a.DeleteVariable)
		g.SetRefreshInClient(a.RefreshInClient)
		return g
	case *microflows.RollbackObjectAction:
		g := genMf.NewRollbackAction()
		g.SetID(element.ID(a.ID))
		g.SetRollbackVariableName(a.RollbackVariable)
		g.SetRefreshInClient(a.RefreshInClient)
		return g
	case *microflows.RetrieveAction:
		g := genMf.NewRetrieveAction()
		g.SetID(element.ID(a.ID))
		g.SetErrorHandlingType("Rollback")
		g.SetOutputVariableName(a.OutputVariable)
		if src := retrieveSourceToGen(a.Source); src != nil {
			g.SetRetrieveSource(src)
		}
		return g
	case *microflows.MicroflowCallAction:
		g := genMf.NewMicroflowCallAction()
		g.SetID(element.ID(a.ID))
		g.SetErrorHandlingType(orDefault(string(a.ErrorHandlingType), "Rollback"))
		if a.MicroflowCall != nil {
			mc := genMf.NewMicroflowCall()
			mc.SetID(element.ID(a.MicroflowCall.ID))
			mc.SetMicroflowQualifiedName(a.MicroflowCall.Microflow)
			for _, pm := range a.MicroflowCall.ParameterMappings {
				m := genMf.NewMicroflowCallParameterMapping()
				m.SetID(element.ID(pm.ID))
				m.SetParameterQualifiedName(pm.Parameter)
				m.SetArgument(pm.Argument)
				mc.AddParameterMappings(m)
			}
			g.SetMicroflowCall(mc)
		}
		g.SetOutputVariableName(a.ResultVariableName) // BSON key "ResultVariableName"
		g.SetUseReturnVariable(a.UseReturnVariable)
		return g
	case *microflows.ExecuteDatabaseQueryAction:
		// Storage $Type is DatabaseConnector$ExecuteDatabaseQueryAction (not the
		// Microflows$ prefix); built directly with the verified keys, mirroring the
		// legacy serializer (fields in alphabetical order, both mapping lists marker 2).
		g := newElem("DatabaseConnector$ExecuteDatabaseQueryAction", string(a.ID))
		conns := make([]element.Element, 0, len(a.ConnectionParameterMappings))
		for _, cm := range a.ConnectionParameterMappings {
			m := newElem("DatabaseConnector$ConnectionParameterMapping", string(cm.ID))
			addStr(m, "ParameterName", cm.ParameterName)
			addStr(m, "Value", cm.Value)
			conns = append(conns, m)
		}
		addPartList(g, "ConnectionParameterMappings", conns)
		addStr(g, "DynamicQuery", a.DynamicQuery)
		addStr(g, "ErrorHandlingType", orDefault(string(a.ErrorHandlingType), "Rollback"))
		addStr(g, "OutputVariableName", a.OutputVariableName)
		params := make([]element.Element, 0, len(a.ParameterMappings))
		for _, pm := range a.ParameterMappings {
			m := newElem("DatabaseConnector$QueryParameterMapping", string(pm.ID))
			addStr(m, "ParameterName", pm.ParameterName)
			addStr(m, "Value", pm.Value)
			params = append(params, m)
		}
		addPartList(g, "ParameterMappings", params)
		addStr(g, "Query", a.Query)
		return g
	case *microflows.JavaActionCallAction:
		// Built directly: the gen JavaActionCallAction binds the wrong BSON keys
		// (JavaActionQualifiedName/OutputVariableName) vs the verified storage keys
		// (JavaAction/ResultVariableName). Mirrors the legacy serializer.
		g := newElem("Microflows$JavaActionCallAction", string(a.ID))
		addStr(g, "ErrorHandlingType", orDefault(string(a.ErrorHandlingType), "Rollback"))
		addStr(g, "JavaAction", a.JavaAction)
		addStr(g, "ResultVariableName", a.ResultVariableName)
		addBool(g, "UseReturnVariable", a.UseReturnVariable)
		mappings := make([]element.Element, 0, len(a.ParameterMappings))
		for _, pm := range a.ParameterMappings {
			m := newElem("Microflows$JavaActionParameterMapping", string(pm.ID))
			addStr(m, "Parameter", pm.Parameter)
			if pm.Value != nil {
				if v := codeActionParameterValueToGen(pm.Value); v != nil {
					addPart(m, "Value", v)
				}
			}
			mappings = append(mappings, m)
		}
		addPartList(g, "ParameterMappings", mappings)
		return g
	case *microflows.LogMessageAction:
		g := genMf.NewLogMessageAction()
		g.SetID(element.ID(a.ID))
		g.SetErrorHandlingType(orDefault(string(a.ErrorHandlingType), "Rollback"))
		g.SetIncludeLatestStackTrace(a.IncludeLastStackTrace)
		g.SetLevel(string(a.LogLevel))
		g.SetNode(a.LogNodeName)
		if a.MessageTemplate != nil {
			g.SetMessageTemplate(stringTemplateToGen(a.MessageTemplate, a.TemplateParameters))
		}
		return g
	case *microflows.CreateVariableAction:
		g := genMf.NewCreateVariableAction()
		g.SetID(element.ID(a.ID))
		g.SetErrorHandlingType(orDefault(string(a.ErrorHandlingType), "Rollback"))
		g.SetVariableName(a.VariableName)
		g.SetInitialValue(a.InitialValue)
		if a.DataType != nil {
			g.SetVariableType(microflowDataTypeToGen(a.DataType))
		}
		return g
	case *microflows.ChangeVariableAction:
		g := genMf.NewChangeVariableAction()
		g.SetID(element.ID(a.ID))
		g.SetErrorHandlingType(orDefault(string(a.ErrorHandlingType), "Rollback"))
		g.SetChangeVariableName(a.VariableName)
		g.SetValue(a.Value)
		return g
	case *microflows.NanoflowCallAction:
		g := genMf.NewNanoflowCallAction()
		g.SetID(element.ID(a.ID))
		g.SetErrorHandlingType(orDefault(string(a.ErrorHandlingType), "Rollback"))
		g.SetOutputVariableName(a.OutputVariableName)
		g.SetUseReturnVariable(a.UseReturnVariable)
		if a.NanoflowCall != nil {
			nc := genMf.NewNanoflowCall()
			nc.SetID(element.ID(a.NanoflowCall.ID))
			nc.SetNanoflowQualifiedName(a.NanoflowCall.Nanoflow)
			for _, pm := range a.NanoflowCall.ParameterMappings {
				m := genMf.NewNanoflowCallParameterMapping()
				m.SetID(element.ID(pm.ID))
				m.SetParameterQualifiedName(pm.Parameter)
				m.SetArgument(pm.Argument)
				nc.AddParameterMappings(m)
			}
			g.SetNanoflowCall(nc)
		}
		return g
	case *microflows.CastAction:
		// Storage $Type Microflows$CastAction; output bound to "VariableName".
		g := genMf.NewCastAction()
		g.SetID(element.ID(a.ID))
		g.SetErrorHandlingType("Rollback")
		g.SetOutputVariableName(a.OutputVariable)
		return g
	case *microflows.AggregateListAction:
		// Storage $Type Microflows$AggregateAction. Input bound to
		// "AggregateVariableName", output to "VariableName"; Attribute is a
		// by-name ref. Expression mode is mutually exclusive with Attribute.
		g := genMf.NewAggregateListAction()
		g.SetID(element.ID(a.ID))
		g.SetErrorHandlingType("Rollback")
		g.SetAggregateFunction(string(a.Function))
		g.SetInputListVariableName(a.InputVariable)
		if a.UseExpression {
			g.SetUseExpression(true)
			g.SetExpression(a.Expression)
		} else if a.AttributeQualifiedName != "" {
			g.SetAttributeQualifiedName(a.AttributeQualifiedName)
		}
		g.SetOutputVariableName(a.OutputVariable)
		return g
	case *microflows.CreateListAction:
		// Storage $Type Microflows$CreateListAction; output bound to "VariableName".
		g := genMf.NewCreateListAction()
		g.SetID(element.ID(a.ID))
		g.SetErrorHandlingType("Rollback")
		if a.EntityQualifiedName != "" {
			g.SetEntityQualifiedName(a.EntityQualifiedName)
		}
		g.SetOutputVariableName(a.OutputVariable)
		return g
	case *microflows.ChangeListAction:
		// Storage $Type Microflows$ChangeListAction. Value is omitted by the
		// legacy writer when empty (e.g. Clear); the gen Set only marks dirty
		// when called, so guard it the same way.
		g := genMf.NewChangeListAction()
		g.SetID(element.ID(a.ID))
		g.SetErrorHandlingType("Rollback")
		g.SetChangeVariableName(a.ChangeVariable)
		g.SetType(string(a.Type))
		if a.Value != "" {
			g.SetValue(a.Value)
		}
		return g
	case *microflows.ValidationFeedbackAction:
		// Storage $Type Microflows$ValidationFeedbackAction. Object bound to
		// "ValidationVariableName"; FeedbackTemplate is a Microflows$TextTemplate
		// (NOT a StringTemplate) with a nested Texts$Text. Attribute/Association
		// are mutually exclusive by-name refs; the legacy writer emits both keys
		// (one empty) but only the populated one is required for validity.
		g := genMf.NewValidationFeedbackAction()
		g.SetID(element.ID(a.ID))
		g.SetErrorHandlingType(orDefault(string(a.ErrorHandlingType), "Rollback"))
		g.SetObjectVariableName(a.ObjectVariable)
		if a.AssociationName != "" {
			g.SetAssociationQualifiedName(a.AssociationName)
		} else if a.AttributeName != "" {
			g.SetAttributeQualifiedName(a.AttributeName)
		}
		if a.Template != nil {
			g.SetFeedbackTemplate(textTemplateToGen(a.Template, a.TemplateParameters))
		}
		return g
	case *microflows.ListOperationAction:
		// Storage $Type Microflows$ListOperationsAction. The operation is bound
		// to "NewOperation" and the output to "ResultVariableName" — the gen
		// ListOperationAction uses the wrong storage keys ("Operation"/
		// "VariableName"), so this action and its operation sub-elements are
		// built directly with the verified legacy BSON keys.
		e := newElem("Microflows$ListOperationsAction", string(a.ID))
		addStr(e, "ErrorHandlingType", "Rollback")
		if a.Operation != nil {
			addPart(e, "NewOperation", listOperationToGen(a.Operation))
		}
		addStr(e, "ResultVariableName", a.OutputVariable)
		return e
	case *microflows.ShowPageAction:
		// Storage $Type Microflows$ShowFormAction ("Form" = legacy term for "Page").
		// The gen ShowPageAction's FormSettings/PageParameterMapping/PageVariable
		// children are Forms$ types with no gen constructors, so the FormSettings tree
		// is built directly with the verified legacy storage keys.
		g := genMf.NewShowPageAction()
		g.SetID(element.ID(a.ID))
		g.SetErrorHandlingType(orDefault(string(a.ErrorHandlingType), "Rollback"))
		g.SetPageSettings(showPageFormSettingsToGen(a))
		g.SetNumberOfPagesToClose("")
		return g
	case *microflows.ClosePageAction:
		// Storage $Type Microflows$CloseFormAction. Legacy emits only
		// ErrorHandlingType + NumberOfPages (int32); the gen's extra
		// NumberOfPagesToClose string is left unset (not dirty → not emitted).
		g := genMf.NewCloseFormAction()
		g.SetID(element.ID(a.ID))
		g.SetErrorHandlingType(orDefault(string(a.ErrorHandlingType), "Rollback"))
		g.SetNumberOfPages(int32(a.NumberOfPages))
		return g
	case *microflows.ShowHomePageAction:
		// Storage $Type Microflows$ShowHomePageAction. Legacy emits only
		// ErrorHandlingType (always "Rollback").
		g := genMf.NewShowHomePageAction()
		g.SetID(element.ID(a.ID))
		g.SetErrorHandlingType("Rollback")
		return g
	case *microflows.ShowMessageAction:
		// Storage $Type Microflows$ShowMessageAction. Template is a
		// Microflows$TextTemplate (nested Texts$Text) — the same shape as
		// ValidationFeedback's FeedbackTemplate, so reuse textTemplateToGen.
		g := genMf.NewShowMessageAction()
		g.SetID(element.ID(a.ID))
		g.SetErrorHandlingType(orDefault(string(a.ErrorHandlingType), "Rollback"))
		g.SetType(string(a.Type))
		g.SetBlocking(a.Blocking)
		if a.Template != nil {
			g.SetTemplate(textTemplateToGen(a.Template, a.TemplateParameters))
		}
		return g
	case *microflows.RestCallAction:
		// Storage $Type Microflows$RestCallAction. The gen constructor's keys match
		// for the action level, but several sub-elements diverge from the verified
		// legacy storage names (HttpConfiguration's HttpHeaderEntries/
		// HttpAuthenticationPassword/UseHttpAuthentication; ImportMappingCall's
		// ReturnValueMapping; StringTemplate's Microflows$TemplateParameter children),
		// so the whole subtree is built directly. Mirrors serializeRestCallAction.
		return restCallActionToGen(a)
	case *microflows.ImportXmlAction:
		// "import from mapping" — Microflows$ImportXmlAction with a ResultHandling
		// whose ImportMappingCall references the mapping. Built directly because the
		// ImportMappingCall uses the verified key ReturnValueMapping (gen binds
		// "Mapping"). Mirrors serializeImportXmlAction.
		return importXmlActionToGen(a)
	case *microflows.ExportXmlAction:
		// "export to mapping" — Microflows$ExportXmlAction with an ExportXmlAction$
		// StringExport output method and a MappingRequestHandling. Mirrors
		// serializeExportXmlAction.
		return exportXmlActionToGen(a)
	case *microflows.WorkflowCallAction,
		*microflows.GetWorkflowDataAction,
		*microflows.GetWorkflowsAction,
		*microflows.GetWorkflowActivityRecordsAction,
		*microflows.WorkflowOperationAction,
		*microflows.SetTaskOutcomeAction,
		*microflows.OpenUserTaskAction,
		*microflows.NotifyWorkflowAction,
		*microflows.OpenWorkflowAction,
		*microflows.LockWorkflowAction,
		*microflows.UnlockWorkflowAction:
		// Workflow-related microflow actions (call workflow, get workflow data,
		// set task outcome, workflow operation pause/continue/abort/…, etc.).
		// Mirrors sdk/mpr/writer_microflow_workflow.go field-for-field.
		return workflowMicroflowActionToGen(a)
	case *microflows.RestOperationCallAction:
		// "call rest operation" — Microflows$RestOperationCallAction. Mirrors
		// serializeRestOperationCallAction.
		return restOperationCallActionToGen(a)
	case *microflows.CallExternalAction:
		// "call external action" — Microflows$CallExternalAction. Without this
		// the activity serialized with no action → CE0008 "No action defined".
		return callExternalActionToGen(a)
	default:
		return nil // not yet supported (added in later groups)
	}
}

// importXmlActionToGen builds a Microflows$ImportXmlAction ("import from mapping").
// Mirrors serializeImportXmlAction field-for-field, including the ImportMappingCall
// sub-element (ReturnValueMapping key) and the Object/List VariableType.
func importXmlActionToGen(a *microflows.ImportXmlAction) element.Element {
	g := newElem("Microflows$ImportXmlAction", string(a.ID))
	addStr(g, "ErrorHandlingType", orDefault(string(a.ErrorHandlingType), "Rollback"))
	addBool(g, "IsValidationRequired", a.IsValidationRequired)

	rh := a.ResultHandling
	if rh == nil {
		rh = &microflows.ResultHandlingMapping{}
	}
	forceSingle := rh.SingleObject
	if rh.ForceSingleOccurrence != nil {
		forceSingle = *rh.ForceSingleOccurrence
	}

	imc := newElem("Microflows$ImportMappingCall", "")
	addStr(imc, "Commit", "YesWithoutEvents")
	addStr(imc, "ContentType", "Json")
	addBool(imc, "ForceSingleOccurrence", forceSingle)
	addStr(imc, "ObjectHandlingBackup", "Create")
	addStr(imc, "ParameterVariableName", "")
	rng := newElem("Microflows$ConstantRange", "")
	addBool(rng, "SingleObject", rh.SingleObject)
	addPart(imc, "Range", rng)
	addStr(imc, "ReturnValueMapping", string(rh.MappingID))

	var vt *element.Base
	if rh.SingleObject {
		vt = newElem("DataTypes$ObjectType", "")
	} else {
		vt = newElem("DataTypes$ListType", "")
	}
	addStr(vt, "Entity", string(rh.ResultEntityID))

	resultHandling := newElem("Microflows$ResultHandling", string(rh.ID))
	addBool(resultHandling, "Bind", rh.ResultVariable != "")
	addPart(resultHandling, "ImportMappingCall", imc)
	addStr(resultHandling, "ResultVariableName", rh.ResultVariable)
	addPart(resultHandling, "VariableType", vt)

	addPart(g, "ResultHandling", resultHandling)
	addStr(g, "XmlDocumentVariableName", a.XmlDocumentVariable)
	return g
}

// exportXmlActionToGen builds a Microflows$ExportXmlAction ("export to mapping").
// Mirrors serializeExportXmlAction: an ExportXmlAction$StringExport output method
// plus a MappingRequestHandling referencing the export mapping.
func exportXmlActionToGen(a *microflows.ExportXmlAction) element.Element {
	g := newElem("Microflows$ExportXmlAction", string(a.ID))
	addStr(g, "ErrorHandlingType", orDefault(string(a.ErrorHandlingType), "Rollback"))
	addBool(g, "IsValidationRequired", a.IsValidationRequired)

	output := newElem("ExportXmlAction$StringExport", "")
	addStr(output, "OutputVariableName", a.OutputVariable)
	addPart(g, "OutputMethod", output)

	mappingID, paramVar := "", ""
	if a.RequestHandling != nil {
		mappingID = string(a.RequestHandling.MappingID)
		paramVar = a.RequestHandling.ParameterVariable
	}
	rh := newElem("Microflows$MappingRequestHandling", "")
	addStr(rh, "ContentType", "Json")
	addStr(rh, "MappingId", mappingID)
	addStr(rh, "MappingVariableName", paramVar)
	addPart(g, "ResultHandling", rh)
	return g
}

// newElem builds a bare codec element with the given storage $Type and ID,
// fresh-generating the ID when empty. Used where the gen type's property keys
// diverge from Mendix's verified storage names (the list-operation family),
// so the BSON keys are set explicitly via addStr/addPart/addPartList.
// codeActionParameterValueToGen builds a java/javascript-action parameter value
// (the Value child of a JavaActionParameterMapping). Mirrors the legacy
// serializeCodeActionParameterValue field-for-field.
func codeActionParameterValueToGen(v microflows.CodeActionParameterValue) element.Element {
	switch val := v.(type) {
	case *microflows.StringTemplateParameterValue:
		g := newElem("Microflows$StringTemplateParameterValue", string(val.ID))
		if val.TypedTemplate != nil {
			tt := newElem("Microflows$TypedTemplate", string(val.TypedTemplate.ID))
			addPartList(tt, "Arguments", nil) // empty (marker 2, registered in init)
			addStr(tt, "Text", val.TypedTemplate.Text)
			addPart(g, "TypedTemplate", tt)
		}
		return g
	case *microflows.ExpressionBasedCodeActionParameterValue:
		g := newElem("Microflows$ExpressionBasedCodeActionParameterValue", string(val.ID))
		addStr(g, "Expression", val.Expression)
		return g
	case *microflows.BasicCodeActionParameterValue:
		g := newElem("Microflows$BasicCodeActionParameterValue", string(val.ID))
		addStr(g, "Argument", val.Argument)
		return g
	case *microflows.MicroflowParameterValue:
		g := newElem("Microflows$MicroflowParameterValue", string(val.ID))
		addStr(g, "Microflow", val.Microflow)
		return g
	case *microflows.EntityTypeCodeActionParameterValue:
		g := newElem("Microflows$EntityTypeCodeActionParameterValue", string(val.ID))
		addStr(g, "Entity", val.Entity)
		return g
	}
	return nil
}

func newElem(typeName, id string) *element.Base {
	b := &element.Base{}
	b.SetTypeName(typeName)
	if id == "" {
		id = mmpr.GenerateID()
	}
	b.SetID(element.ID(id))
	return b
}

// addStr adds a dirty string property (BSON key = name) to a bare element.
func addStr(b *element.Base, name, val string) {
	p := property.NewPrimitive[string](name, property.DecodeString)
	b.AddProperty(p, uint(len(b.Properties())))
	p.Set(val)
}

// addBool adds a dirty bool property (BSON key = name) to a bare element.
func addBool(b *element.Base, name string, val bool) {
	p := property.NewPrimitive[bool](name, property.DecodeBool)
	b.AddProperty(p, uint(len(b.Properties())))
	p.Set(val)
}

// addInt32 adds a dirty int32 property (BSON key = name) to a bare element.
func addInt32(b *element.Base, name string, val int32) {
	p := property.NewPrimitive[int32](name, property.DecodeInt32)
	b.AddProperty(p, uint(len(b.Properties())))
	p.Set(val)
}

// addPart adds a dirty single-child property (BSON key = name).
func addPart(b *element.Base, name string, child element.Element) {
	p := property.NewPart[element.Element](name)
	b.AddProperty(p, uint(len(b.Properties())))
	p.Set(child)
}

// addPartList adds a dirty child-list property (BSON key = name). The encoder
// emits the typed-array marker (registered per child $Type) plus each child.
func addPartList(b *element.Base, name string, children []element.Element) {
	p := property.NewPartList[element.Element](name)
	b.AddProperty(p, uint(len(b.Properties())))
	for _, c := range children {
		p.Append(c)
	}
}

// listOperationToGen builds a ListOperation sub-element with the verified legacy
// storage names: ListName / SecondListOrObjectName (the gen types use the wrong
// keys ListVariableName / SecondListOrObjectVariableName). Mirrors
// sdk/mpr.serializeListOperation field-for-field.
func listOperationToGen(op microflows.ListOperation) element.Element {
	switch o := op.(type) {
	case *microflows.HeadOperation:
		e := newElem("Microflows$Head", string(o.ID))
		addStr(e, "ListName", o.ListVariable)
		return e
	case *microflows.TailOperation:
		e := newElem("Microflows$Tail", string(o.ID))
		addStr(e, "ListName", o.ListVariable)
		return e
	case *microflows.FindOperation:
		e := newElem("Microflows$FindByExpression", string(o.ID))
		addStr(e, "Expression", o.Expression)
		addStr(e, "ListName", o.ListVariable)
		return e
	case *microflows.FilterOperation:
		e := newElem("Microflows$FilterByExpression", string(o.ID))
		addStr(e, "Expression", o.Expression)
		addStr(e, "ListName", o.ListVariable)
		return e
	case *microflows.FindByAttributeOperation:
		e := newElem("Microflows$Find", string(o.ID))
		addStr(e, "Association", o.Association)
		addStr(e, "Attribute", o.Attribute)
		addStr(e, "Expression", o.Expression)
		addStr(e, "ListName", o.ListVariable)
		return e
	case *microflows.FilterByAttributeOperation:
		e := newElem("Microflows$Filter", string(o.ID))
		addStr(e, "Association", o.Association)
		addStr(e, "Attribute", o.Attribute)
		addStr(e, "Expression", o.Expression)
		addStr(e, "ListName", o.ListVariable)
		return e
	case *microflows.SortOperation:
		e := newElem("Microflows$Sort", string(o.ID))
		addStr(e, "ListName", o.ListVariable)
		// Sortings is a single Microflows$SortingsList whose own "Sortings" list
		// holds the RetrieveSorting items (marker 2, registered in init).
		sl := newElem("Microflows$SortingsList", "")
		items := make([]element.Element, 0, len(o.Sorting))
		for _, it := range o.Sorting {
			items = append(items, sortItemToGen(it))
		}
		addPartList(sl, "Sortings", items)
		addPart(e, "Sortings", sl)
		return e
	case *microflows.UnionOperation:
		e := newElem("Microflows$Union", string(o.ID))
		addStr(e, "ListName", o.ListVariable1)
		addStr(e, "SecondListOrObjectName", o.ListVariable2)
		return e
	case *microflows.IntersectOperation:
		e := newElem("Microflows$Intersect", string(o.ID))
		addStr(e, "ListName", o.ListVariable1)
		addStr(e, "SecondListOrObjectName", o.ListVariable2)
		return e
	case *microflows.SubtractOperation:
		e := newElem("Microflows$Subtract", string(o.ID))
		addStr(e, "ListName", o.ListVariable1)
		addStr(e, "SecondListOrObjectName", o.ListVariable2)
		return e
	case *microflows.ContainsOperation:
		e := newElem("Microflows$Contains", string(o.ID))
		addStr(e, "ListName", o.ListVariable)
		addStr(e, "SecondListOrObjectName", o.ObjectVariable)
		return e
	case *microflows.EqualsOperation:
		e := newElem("Microflows$Equals", string(o.ID))
		addStr(e, "ListName", o.ListVariable1)
		addStr(e, "SecondListOrObjectName", o.ListVariable2)
		return e
	case *microflows.ListRangeOperation:
		// Mirrors legacy parser key "Microflows$ListRange". No legacy writer
		// case existed; the example MDL does not exercise it, so emit the verified
		// storage name with ListName + range expressions.
		e := newElem("Microflows$ListRange", string(o.ID))
		addStr(e, "ListName", o.ListVariable)
		if o.LimitExpression != "" {
			addStr(e, "LimitExpression", o.LimitExpression)
		}
		if o.OffsetExpression != "" {
			addStr(e, "OffsetExpression", o.OffsetExpression)
		}
		return e
	default:
		return nil
	}
}

// sortItemToGen builds a Microflows$RetrieveSorting item with a nested
// DomainModels$AttributeRef. Mirrors the SortOperation branch of
// sdk/mpr.serializeListOperation.
func sortItemToGen(item *microflows.SortItem) element.Element {
	e := newElem("Microflows$RetrieveSorting", string(item.ID))
	addStr(e, "SortOrder", string(item.Direction))
	if item.AttributeQualifiedName != "" {
		ref := genDom.NewAttributeRef()
		assignID(ref)
		ref.SetAttributeQualifiedName(item.AttributeQualifiedName)
		if len(item.EntityRefSteps) > 0 {
			ref.SetEntityRef(entityRefToGen(item.EntityRefSteps))
		}
		addPart(e, "AttributeRef", ref)
	}
	return e
}

// entityRefToGen builds a DomainModels$IndirectEntityRef from association steps.
// Mirrors sdk/mpr.serializeIndirectEntityRef; the gen Steps list and EntityRefStep
// Association/DestinationEntity keys already match the verified storage names.
func entityRefToGen(steps []microflows.EntityRefStep) element.Element {
	ref := genDom.NewIndirectEntityRef()
	assignID(ref)
	for _, s := range steps {
		st := genDom.NewEntityRefStep()
		assignID(st)
		st.SetAssociationQualifiedName(s.Association)
		st.SetDestinationEntityQualifiedName(s.DestinationEntity)
		ref.AddSteps(st)
	}
	return ref
}

// splitConditionToGen builds an exclusive-split condition. Rule conditions are a
// later slice (RuleCall + parameter mappings).
func splitConditionToGen(sc microflows.SplitCondition) element.Element {
	switch c := sc.(type) {
	case *microflows.ExpressionSplitCondition:
		g := genMf.NewExpressionSplitCondition()
		g.SetID(element.ID(c.ID))
		g.SetExpression(c.Expression)
		return g
	default:
		return nil
	}
}

// caseValueToGen renders a sequence-flow case. ExpressionCase is serialized AS an
// EnumerationCase with Value = the expression ("true"/"false") — Studio Pro has
// never recognised Microflows$ExpressionCase (verified vs legacy). Default NoCase.
func caseValueToGen(cv microflows.CaseValue) element.Element {
	// The visitor sometimes yields value receivers; normalise to pointers so the
	// dispatch below handles each case once (mirrors the legacy writer). Without
	// this, a value-typed EnumerationCase fell through to NoCase — an enum `case`
	// split lost all its branch values (CE0079/CE0773 in Studio Pro).
	switch c := cv.(type) {
	case microflows.EnumerationCase:
		cv = &c
	case microflows.ExpressionCase:
		cv = &c
	case microflows.NoCase:
		cv = &c
	}
	switch c := cv.(type) {
	case *microflows.EnumerationCase:
		g := genMf.NewEnumerationCase()
		g.SetID(element.ID(c.ID))
		g.SetValue(c.Value)
		return g
	case *microflows.ExpressionCase:
		g := genMf.NewEnumerationCase()
		g.SetID(element.ID(c.ID))
		g.SetValue(c.Expression)
		return g
	default:
		return genMf.NewNoCase()
	}
}

// retrieveSourceToGen builds a gen retrieve source. Verified against real BSON
// (test7): a DatabaseRetrieveSource always carries a Range (default ConstantRange,
// SingleObject=false) and a NewSortings SortingsList (empty Sortings=[2]).
func retrieveSourceToGen(src microflows.RetrieveSource) element.Element {
	switch s := src.(type) {
	case *microflows.DatabaseRetrieveSource:
		g := genMf.NewDatabaseRetrieveSource()
		g.SetID(element.ID(s.ID))
		if s.EntityQualifiedName != "" {
			g.SetEntityQualifiedName(s.EntityQualifiedName)
		}
		g.SetRange(rangeToGen(s.Range))
		if s.XPathConstraint != "" {
			g.SetXPathConstraint(s.XPathConstraint)
		}
		// Build the SortingsList (stored under "NewSortings") with the actual sort
		// columns. The generated SortItemList type serializes its items under
		// "Items", but Mendix stores retrieve/sort columns under "Sortings" — so
		// build the envelope manually, mirroring the Sort list-operation branch.
		// Without this, "retrieve … sort by …" columns are dropped on write.
		sl := newElem("Microflows$SortingsList", "")
		items := make([]element.Element, 0, len(s.Sorting))
		for _, it := range s.Sorting {
			items = append(items, sortItemToGen(it))
		}
		addPartList(sl, "Sortings", items)
		g.SetSortItemList(sl)
		return g
	case *microflows.AssociationRetrieveSource:
		g := genMf.NewAssociationRetrieveSource()
		g.SetID(element.ID(s.ID))
		if s.StartVariable != "" {
			g.SetStartVariableName(s.StartVariable)
		}
		if s.AssociationQualifiedName != "" {
			g.SetAssociationQualifiedName(s.AssociationQualifiedName)
		}
		return g
	default:
		return nil
	}
}

// rangeToGen builds a gen retrieve Range; nil → default ConstantRange (retrieve all).
func rangeToGen(r *microflows.Range) element.Element {
	if r == nil {
		g := genMf.NewConstantRange()
		g.SetSingleObject(false)
		return g
	}
	if r.RangeType == microflows.RangeTypeCustom {
		g := genMf.NewCustomRange()
		if r.Limit != "" {
			g.SetLimitExpression(r.Limit)
		}
		if r.Offset != "" {
			g.SetOffsetExpression(r.Offset)
		}
		return g
	}
	g := genMf.NewConstantRange()
	g.SetSingleObject(r.RangeType == microflows.RangeTypeFirst)
	return g
}

// memberChangeToGen builds a gen MemberChange (ChangeActionItem). Mirrors legacy:
// Association is emitted as "" for attribute-targeted changes.
func memberChangeToGen(m *microflows.MemberChange) element.Element {
	g := genMf.NewMemberChange()
	g.SetID(element.ID(m.ID))
	if m.AssociationQualifiedName != "" {
		g.SetAssociationQualifiedName(m.AssociationQualifiedName)
	} else {
		g.SetAssociationQualifiedName("")
		if m.AttributeQualifiedName != "" {
			g.SetAttributeQualifiedName(m.AttributeQualifiedName)
		}
	}
	g.SetType(string(m.Type))
	g.SetValue(m.Value)
	return g
}

// microflowParameterToGen builds a gen MicroflowParameter (position derives from
// index, matching the legacy serializer).
func microflowParameterToGen(p *microflows.MicroflowParameter, idx, major int) element.Element {
	g := genMf.NewMicroflowParameter()
	g.SetID(element.ID(p.ID))
	g.SetDocumentation(p.Documentation)
	g.SetHasVariableNameBeenChanged(false)
	g.SetName(p.Name)
	g.SetRelativeMiddlePoint(fmt.Sprintf("%d;53", 200+idx*100))
	g.SetSize("30;30")
	if major >= 10 {
		g.SetDefaultValue("")
		g.SetIsRequired(true)
	}
	g.SetParameterType(microflowDataTypeToGen(p.Type))
	return g
}

// sequenceFlowToGen builds a gen SequenceFlow. v10+ uses CaseValues + a BezierCurve
// Line; the case defaults to NoCase.
func sequenceFlowToGen(f *microflows.SequenceFlow, major int) element.Element {
	g := genMf.NewSequenceFlow()
	g.SetID(element.ID(f.ID))
	g.SetOriginID(element.ID(f.OriginID))
	g.SetDestinationID(element.ID(f.DestinationID))
	g.SetOriginConnectionIndex(int32(f.OriginConnectionIndex))
	g.SetDestinationConnectionIndex(int32(f.DestinationConnectionIndex))
	g.SetIsErrorHandler(f.IsErrorHandler)
	g.AddCaseValues(caseValueToGen(f.CaseValue))

	originCV := orDefault(f.OriginControlVector, "0;0")
	destCV := orDefault(f.DestinationControlVector, "0;0")
	line := genMf.NewBezierCurve()
	line.SetOriginControlVector(originCV)
	line.SetDestinationControlVector(destCV)
	g.SetLine(line)
	return g
}

// stringTemplateToGen builds a Microflows$StringTemplate (the message body of a
// LogMessageAction): a Text plus one TemplateParameter per {1},{2},… expression.
// The text is taken from the (sorted, for determinism) first translation.
func stringTemplateToGen(text *model.Text, params []string) *genMf.StringTemplate {
	st := genMf.NewStringTemplate()
	assignID(st)
	st.SetText(firstTranslation(text))
	for _, expr := range params {
		tp := genMf.NewTemplateArgument()
		assignID(tp)
		tp.SetExpression(expr)
		st.AddArguments(tp)
	}
	return st
}

// textTemplateToGen builds a Microflows$TextTemplate (a ValidationFeedbackAction's
// FeedbackTemplate): a nested Texts$Text holding the translations, plus one
// TemplateParameter per {1},{2},… expression. Distinct from StringTemplate, whose
// Text is a scalar — here Text is a nested Texts$Text element. Mirrors
// sdk/mpr.serializeTextTemplate. Empty Parameters emits as marker 2 via the
// registered TextTemplate default.
func textTemplateToGen(text *model.Text, params []string) *genMf.TextTemplate {
	tt := genMf.NewTextTemplate()
	assignID(tt)
	txt := genTexts.NewText()
	assignID(txt)
	if text != nil && len(text.Translations) > 0 {
		langs := make([]string, 0, len(text.Translations))
		for lang := range text.Translations {
			langs = append(langs, lang)
		}
		sort.Strings(langs)
		for _, lang := range langs {
			tr := genTexts.NewTranslation()
			assignID(tr)
			tr.SetLanguageCode(lang)
			tr.SetText(text.Translations[lang])
			txt.AddTranslations(tr)
		}
	}
	tt.SetText(txt)
	for _, expr := range params {
		tp := genMf.NewTemplateArgument()
		assignID(tp)
		tp.SetExpression(expr)
		tt.AddArguments(tp)
	}
	return tt
}

// showPageFormSettingsToGen builds a ShowPage's Forms$FormSettings subtree directly:
// Form (page by-name ref), ParameterMappings (Forms$PageParameterMapping list,
// marker 2) and a TitleOverride (empty Microflows$TextTemplate, reusing the widget
// helper). Mirrors serializeMicroflowAction's ShowFormAction case; the Forms$ types
// have no gen constructors.
func showPageFormSettingsToGen(a *microflows.ShowPageAction) element.Element {
	fs := newElem("Forms$FormSettings", string(a.FormSettingsID))
	addStr(fs, "Form", a.PageName) // BY_NAME_REFERENCE: page qualified name
	mappings := make([]element.Element, 0, len(a.PageParameterMappings))
	for _, pm := range a.PageParameterMappings {
		m := newElem("Forms$PageParameterMapping", string(pm.ID))
		addStr(m, "Argument", pm.Argument)
		addStr(m, "Parameter", pm.Parameter) // BY_NAME_REFERENCE
		addPart(m, "Variable", emptyPageVariable())
		mappings = append(mappings, m)
	}
	addPartList(fs, "ParameterMappings", mappings)
	addPart(fs, "TitleOverride", emptyTextTemplateToGen())
	return fs
}

// emptyPageVariable builds an empty Forms$PageVariable (the Variable on a
// PageParameterMapping). Mirrors the legacy emptyPageVariable().
func emptyPageVariable() element.Element {
	v := newElem("Forms$PageVariable", "")
	addStr(v, "PageParameter", "")
	addStr(v, "SnippetParameter", "")
	addBool(v, "UseAllPages", false)
	addStr(v, "Widget", "")
	return v
}

// restCallActionToGen builds a Microflows$RestCallAction subtree directly, mirroring
// sdk/mpr.serializeRestCallAction field-for-field. The gen sub-element types diverge
// from the verified legacy storage keys, so the whole tree uses newElem/addStr/etc.
func restCallActionToGen(a *microflows.RestCallAction) element.Element {
	e := newElem("Microflows$RestCallAction", string(a.ID))
	addStr(e, "ErrorHandlingType", orDefault(string(a.ErrorHandlingType), "Rollback"))
	addStr(e, "ErrorResultHandlingType", "HttpResponse")
	if a.HttpConfiguration != nil {
		addPart(e, "HttpConfiguration", httpConfigToGen(a.HttpConfiguration))
	}
	// ProxyConfiguration is emitted as null via the registered NullField default.
	if a.RequestHandling != nil {
		if rh := restRequestHandlingToGen(a.RequestHandling); rh != nil {
			addPart(e, "RequestHandling", rh)
		}
	}
	addStr(e, "RequestHandlingType", "Custom")
	addStr(e, "RequestProxyType", "DefaultProxy")
	resultHandlingType := "String"
	if a.ResultHandling != nil {
		addPart(e, "ResultHandling", restResultHandlingToGen(a.ResultHandling, a.OutputVariable))
		switch a.ResultHandling.(type) {
		case *microflows.ResultHandlingString:
			resultHandlingType = "String"
		case *microflows.ResultHandlingHttpResponse:
			resultHandlingType = "HttpResponse"
		case *microflows.ResultHandlingMapping:
			resultHandlingType = "Mapping"
		case *microflows.ResultHandlingNone:
			resultHandlingType = "None"
		}
	}
	addStr(e, "ResultHandlingType", resultHandlingType)
	addStr(e, "TimeOutExpression", orDefault(a.TimeoutExpression, "300"))
	addBool(e, "UseRequestTimeOut", true)
	return e
}

// httpConfigToGen builds a Microflows$HttpConfiguration with the verified legacy
// storage keys (HttpHeaderEntries / HttpAuthenticationPassword / UseHttpAuthentication
// — all of which diverge from the gen constructor's keys). Empty HttpHeaderEntries
// emits as marker 2 via the registered default. Mirrors serializeRestCallAction's
// HttpConfiguration block.
func httpConfigToGen(c *microflows.HttpConfiguration) element.Element {
	hc := newElem("Microflows$HttpConfiguration", string(c.ID))
	addStr(hc, "ClientCertificate", "")
	addStr(hc, "CustomLocation", "")
	if c.LocationTemplate != "" {
		addPart(hc, "CustomLocationTemplate", stringTemplateElem(c.LocationTemplate, c.LocationParams))
	}
	addStr(hc, "HttpAuthenticationPassword", c.Password)
	addStr(hc, "HttpAuthenticationUserName", c.Username)
	headers := make([]element.Element, 0, len(c.CustomHeaders))
	for _, h := range c.CustomHeaders {
		he := newElem("Microflows$HttpHeaderEntry", string(h.ID))
		addStr(he, "Key", h.Name)
		addStr(he, "Value", h.Value)
		headers = append(headers, he)
	}
	addPartList(hc, "HttpHeaderEntries", headers)
	addStr(hc, "HttpMethod", string(c.HttpMethod))
	addBool(hc, "OverrideLocation", true)
	addBool(hc, "UseHttpAuthentication", c.UseAuthentication)
	return hc
}

// stringTemplateElem builds a Microflows$StringTemplate with a scalar Text plus a
// Parameters list of Microflows$TemplateParameter (Expression) children — the shape
// the REST writer uses for the location/custom-request templates (distinct from the
// StringTemplate gen type which uses TemplateArgument children). Empty Parameters
// emits as marker 2 via the registered default.
func stringTemplateElem(text string, params []string) element.Element {
	st := newElem("Microflows$StringTemplate", "")
	addStr(st, "Text", text)
	if len(params) > 0 {
		items := make([]element.Element, 0, len(params))
		for _, p := range params {
			tp := newElem("Microflows$TemplateParameter", "")
			addStr(tp, "Expression", p)
			items = append(items, tp)
		}
		addPartList(st, "Parameters", items)
	}
	return st
}

// restRequestHandlingToGen builds a REST RequestHandling sub-element. Mirrors
// serializeRestRequestHandling (Custom/Mapping/Simple).
func restRequestHandlingToGen(rh microflows.RequestHandling) element.Element {
	switch h := rh.(type) {
	case *microflows.CustomRequestHandling:
		e := newElem("Microflows$CustomRequestHandling", string(h.ID))
		addPart(e, "Template", stringTemplateElem(h.Template, h.TemplateParams))
		return e
	case *microflows.MappingRequestHandling:
		e := newElem("Microflows$MappingRequestHandling", string(h.ID))
		addStr(e, "MappingId", string(h.MappingID))
		addStr(e, "ContentType", h.ContentType)
		addStr(e, "ParameterVariable", h.ParameterVariable)
		return e
	case *microflows.SimpleRequestHandling:
		return newElem("Microflows$SimpleRequestHandling", string(h.ID))
	default:
		return nil
	}
}

// restResultHandlingToGen builds a REST ResultHandling sub-element. Mirrors
// serializeRestResultHandling. The Mapping case's ImportMappingCall uses the verified
// legacy key ReturnValueMapping (the gen ImportMappingCall uses "Mapping"); for the
// non-mapping cases ImportMappingCall is emitted as null via the registered default.
func restResultHandlingToGen(rh microflows.ResultHandling, outputVar string) element.Element {
	switch h := rh.(type) {
	case *microflows.ResultHandlingString:
		e := newElem("Microflows$ResultHandling", string(h.ID))
		addBool(e, "Bind", outputVar != "")
		addStr(e, "ResultVariableName", outputVar)
		addPart(e, "VariableType", newElem("DataTypes$StringType", ""))
		return e
	case *microflows.ResultHandlingHttpResponse:
		e := newElem("Microflows$ResultHandling", string(h.ID))
		addBool(e, "Bind", outputVar != "")
		addStr(e, "ResultVariableName", outputVar)
		vt := newElem("DataTypes$ObjectType", "")
		addStr(vt, "Entity", "System.HttpResponse")
		addPart(e, "VariableType", vt)
		return e
	case *microflows.ResultHandlingNone:
		e := newElem("Microflows$ResultHandling", string(h.ID))
		addBool(e, "Bind", false)
		addStr(e, "ResultVariableName", "")
		addPart(e, "VariableType", newElem("DataTypes$VoidType", ""))
		return e
	case *microflows.ResultHandlingMapping:
		e := newElem("Microflows$ResultHandling", string(h.ID))
		addBool(e, "Bind", true)
		forceSingle := h.SingleObject
		if h.ForceSingleOccurrence != nil {
			forceSingle = *h.ForceSingleOccurrence
		}
		imc := newElem("Microflows$ImportMappingCall", "")
		addStr(imc, "Commit", "YesWithoutEvents")
		addStr(imc, "ContentType", "Json")
		addBool(imc, "ForceSingleOccurrence", forceSingle)
		addStr(imc, "ObjectHandlingBackup", "Create")
		addStr(imc, "ParameterVariableName", "")
		rng := newElem("Microflows$ConstantRange", "")
		addBool(rng, "SingleObject", h.SingleObject)
		addPart(imc, "Range", rng)
		addStr(imc, "ReturnValueMapping", string(h.MappingID))
		addPart(e, "ImportMappingCall", imc)
		var vt *element.Base
		if h.SingleObject {
			vt = newElem("DataTypes$ObjectType", "")
		} else {
			vt = newElem("DataTypes$ListType", "")
		}
		if h.ResultEntityID != "" {
			addStr(vt, "Entity", string(h.ResultEntityID))
		}
		addStr(e, "ResultVariableName", h.ResultVariable)
		addPart(e, "VariableType", vt)
		return e
	default:
		e := newElem("Microflows$ResultHandling", string(mmpr.GenerateID()))
		addBool(e, "Bind", outputVar != "")
		addStr(e, "ResultVariableName", outputVar)
		addPart(e, "VariableType", newElem("DataTypes$StringType", ""))
		return e
	}
}

// firstTranslation returns a translation value deterministically (lowest language
// code), or "" when there are none.
func firstTranslation(text *model.Text) string {
	if text == nil || len(text.Translations) == 0 {
		return ""
	}
	keys := make([]string, 0, len(text.Translations))
	for k := range text.Translations {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return text.Translations[keys[0]]
}

// microflowDataTypeToGen maps a microflow DataType to a gen DataTypes$* element
// (nil → VoidType). Long maps to IntegerType (per the legacy serializer).
func microflowDataTypeToGen(dt microflows.DataType) element.Element {
	if dt == nil {
		return genDT.NewVoidType()
	}
	switch a := dt.(type) {
	case *microflows.BooleanType:
		return genDT.NewBooleanType()
	case *microflows.IntegerType, *microflows.LongType:
		return genDT.NewIntegerType()
	case *microflows.DecimalType:
		return genDT.NewDecimalType()
	case *microflows.StringType:
		return genDT.NewStringType()
	case *microflows.DateTimeType, *microflows.DateType: // Date maps to DateTime in BSON
		return genDT.NewDateTimeType()
	case *microflows.BinaryType:
		return genDT.NewBinaryType()
	case *microflows.EnumerationType:
		t := genDT.NewEnumerationType()
		t.SetEnumerationQualifiedName(a.EnumerationQualifiedName)
		return t
	case *microflows.ObjectType:
		t := genDT.NewObjectType()
		t.SetEntityQualifiedName(a.EntityQualifiedName)
		return t
	case *microflows.ListType:
		t := genDT.NewListType()
		t.SetEntityQualifiedName(a.EntityQualifiedName)
		return t
	default:
		return genDT.NewVoidType()
	}
}

// assignMicroflowIDs assigns fresh IDs to wrapper sub-elements that lack one
// (return type, object collection + its objects, flows + their cases/lines).
func assignMicroflowIDs(m *genMf.Microflow) {
	assignID(m.MicroflowReturnType())
	assignID(m.ConcurrencyErrorMessage())
	if oc, ok := m.ObjectCollection().(*genMf.MicroflowObjectCollection); ok {
		assignObjectCollectionIDs(oc)
	}
	for _, el := range m.FlowsItems() {
		assignID(el)
		if sf, ok := el.(*genMf.SequenceFlow); ok {
			for _, cv := range sf.CaseValuesItems() {
				assignID(cv)
			}
			assignID(sf.Line())
		}
	}
}

// assignObjectCollectionIDs assigns IDs to a collection's objects and their
// sub-elements, recursing into loop bodies.
func assignObjectCollectionIDs(oc *genMf.MicroflowObjectCollection) {
	assignID(oc)
	for _, el := range oc.ObjectsItems() {
		assignFlowObjectIDs(el)
	}
}

// assignFlowObjectIDs assigns the element's ID and walks its sub-elements
// (parameter type, split condition, action items/sources, loop source + nested body).
func assignFlowObjectIDs(el element.Element) {
	assignID(el)
	switch o := el.(type) {
	case *genMf.MicroflowParameter:
		assignID(o.ParameterType())
	case *genMf.ExclusiveSplit:
		assignID(o.SplitCondition())
	case *genMf.LoopedActivity:
		assignID(o.LoopSource())
		if noc, ok := o.ObjectCollection().(*genMf.MicroflowObjectCollection); ok {
			assignObjectCollectionIDs(noc)
		}
	case *genMf.ActionActivity:
		act := o.Action()
		assignID(act)
		switch a := act.(type) {
		case *genMf.CreateObjectAction:
			for _, it := range a.ItemsItems() {
				assignID(it)
			}
		case *genMf.ChangeObjectAction:
			for _, it := range a.ItemsItems() {
				assignID(it)
			}
		case *genMf.RetrieveAction:
			src := a.RetrieveSource()
			assignID(src)
			if db, ok := src.(*genMf.DatabaseRetrieveSource); ok {
				assignID(db.Range())
				if sl, ok := db.SortItemList().(*genMf.SortItemList); ok {
					assignID(sl)
					for _, it := range sl.ItemsItems() {
						assignID(it)
					}
				}
			}
		case *genMf.MicroflowCallAction:
			if mc, ok := a.MicroflowCall().(*genMf.MicroflowCall); ok {
				assignID(mc)
				for _, pm := range mc.ParameterMappingsItems() {
					assignID(pm)
				}
			}
		case *genMf.NanoflowCallAction:
			if nc, ok := a.NanoflowCall().(*genMf.NanoflowCall); ok {
				assignID(nc)
				for _, pm := range nc.ParameterMappingsItems() {
					assignID(pm)
				}
			}
		}
	}
}

func pointStr(p model.Point) string { return fmt.Sprintf("%d;%d", p.X, p.Y) }
func sizeStr(s model.Size) string {
	if s.Width == 0 && s.Height == 0 {
		return "0;0"
	}
	return fmt.Sprintf("%d;%d", s.Width, s.Height)
}
func orDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}
