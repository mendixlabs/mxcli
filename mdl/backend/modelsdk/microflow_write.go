// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"fmt"
	"go.mongodb.org/mongo-driver/v2/bson"
	"sort"
	"strconv"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/modelsdk/codec"
	"github.com/mendixlabs/mxcli/modelsdk/element"
	genDT "github.com/mendixlabs/mxcli/modelsdk/gen/datatypes"
	genDom "github.com/mendixlabs/mxcli/modelsdk/gen/domainmodels"
	genMf "github.com/mendixlabs/mxcli/modelsdk/gen/microflows"
	genTexts "github.com/mendixlabs/mxcli/modelsdk/gen/texts"
	mmpr "github.com/mendixlabs/mxcli/modelsdk/mpr"
	"github.com/mendixlabs/mxcli/modelsdk/property"
	"github.com/mendixlabs/mxcli/sdk/javaactions"
	"github.com/mendixlabs/mxcli/sdk/microflows"
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
	// A rule split's RuleCall follows the same shape: the ParameterMappings list
	// is always emitted, with marker 2 (legacy writer, verified vs mxbuild 11.13).
	codec.RegisterTypeDefaults("Microflows$RuleCall", codec.TypeDefaults{
		MandatoryListMarkers: map[string]int32{"ParameterMappings": 2},
	})
	codec.RegisterListMarker("Microflows$RuleCallParameterMapping", 2)
	codec.RegisterListMarker("Microflows$MicroflowCallParameterMapping", 2)
	codec.RegisterListMarker("Microflows$NanoflowCallParameterMapping", 2)
	// A JavaActionCallAction always serializes QueueSettings as null; its
	// ParameterMappings list empties as marker 2; a StringTemplateParameterValue's
	// TypedTemplate empties its Arguments list as marker 2 (legacy writer).
	codec.RegisterTypeDefaults("Microflows$JavaActionCallAction", codec.TypeDefaults{
		NullFields: []string{"QueueSettings"},
	})
	// A QueueSettings written by `IN QUEUE` names the queue and configures no
	// retry. Retry (Queues$QueueFixedRetry / Queues$QueueExponentialRetry) has no
	// MDL surface; serializing it as null mirrors how the call itself serializes
	// an absent QueueSettings. A stored retry is never overwritten by this —
	// checkNoQueuedCalls refuses the rewrite instead (guard-don't-drop).
	codec.RegisterTypeDefaults("Queues$QueueSettings", codec.TypeDefaults{
		NullFields: []string{"Retry"},
	})
	codec.RegisterListMarker("Microflows$JavaActionParameterMapping", 2)
	codec.RegisterTypeDefaults("Microflows$TypedTemplate", codec.TypeDefaults{
		MandatoryListMarkers: map[string]int32{"Arguments": 2},
	})
	codec.RegisterListMarker("Microflows$PrimitiveTypedTemplateArgument", 2)
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
	// Each PageParameterMapping carries a Variable (empty Forms$PageVariable), built
	// directly. Forms$FormSettings' own defaults — the marker-2 ParameterMappings list
	// and the null TitleOverride — are registered ONCE, in widget_write.go:
	// RegisterTypeDefaults overwrites rather than merges, so a second registration for
	// the same $Type silently wins by init order. A duplicate here is what swallowed
	// the TitleOverride null and kept #812 alive after the first attempt at fixing it.
}

// majorVersion returns the project's Mendix major version (for version-gated BSON).
func (b *Backend) majorVersion() int {
	if pv := b.ProjectVersion(); pv != nil {
		return pv.MajorVersion
	}
	return 11
}

// flowVersion is the Mendix version a flow document — microflow, nanoflow or
// rule — is written for. Flow properties changed shape within Mendix 10, so
// the major alone cannot gate them (mendixlabs/mxcli#1410: every flow write on
// a Mendix 9 project was refused for a StableId and a CaseValues list it
// cannot store, and 10.0–10.14 for the CaseValues list).
//
// The floors are the Model SDK's StructureVersionInfo (mendixmodelsdk
// 4.116.0), and the shapes are measured: a Studio Pro 9.24.30 document, and
// the same document after `mx convert` to 10.12.19.
type flowVersion struct{ major, minor int }

// newestFlowVersion is what an unreadable project version is written as —
// the newest shape, as majorVersion() has always defaulted to 11.
var newestFlowVersion = flowVersion{major: 11}

func (v flowVersion) atLeast(major, minor int) bool {
	return v.major > major || v.major == major && v.minor >= minor
}

// Microflow.stableId, introduced in 10.2.0.
func (v flowVersion) hasStableID() bool { return v.atLeast(10, 2) }

// SequenceFlow.caseValues, introduced in 10.15.0 when the single caseValue
// (stored as NewCaseValue) was deleted.
func (v flowVersion) hasCaseValueList() bool { return v.atLeast(10, 15) }

// Flow.line, introduced in 10.16.0 when Origin/DestinationBezierVector were
// deleted. Flow.lineType lived between 10.8.0 and 10.16.0.
func (v flowVersion) hasLine() bool     { return v.atLeast(10, 16) }
func (v flowVersion) hasLineType() bool { return v.atLeast(10, 8) && !v.hasLine() }

// MicroflowBase.returnVariableName, introduced in 10.12.0.
func (v flowVersion) hasReturnVariableName() bool { return v.atLeast(10, 12) }

// MicroflowParameterObject.isRequired / defaultValue, introduced in 10.17.0.
func (v flowVersion) hasParameterDefaults() bool { return v.atLeast(10, 17) }

// flowVersion returns the version this project's flow documents are written for.
func (b *Backend) flowVersion() flowVersion {
	pv := b.ProjectVersion()
	if pv == nil || pv.MajorVersion == 0 {
		return newestFlowVersion
	}
	return flowVersion{major: pv.MajorVersion, minor: pv.MinorVersion}
}

// microflowEncoder encodes a Microflows$Microflow for v. StableId comes from a
// registered Studio Pro default (a fresh GUID), not a gen property, so leaving
// it out below its floor is the encoder's job.
func microflowEncoder(v flowVersion) *codec.Encoder {
	if v.hasStableID() {
		return &codec.Encoder{}
	}
	return &codec.Encoder{OmitKeys: map[string]map[string]bool{"Microflows$Microflow": {"StableId": true}}}
}

// CreateMicroflow adds a new microflow document (a top-level unit).
func (b *Backend) CreateMicroflow(mf *microflows.Microflow) error {
	if mf == nil {
		return fmt.Errorf("CreateMicroflow: nil microflow")
	}
	if err := flowImportMappingEntityError(mf.ObjectCollection); err != nil {
		return fmt.Errorf("CreateMicroflow %s: %w", mf.Name, err)
	}
	if b.writer == nil {
		return fmt.Errorf("CreateMicroflow: not connected for writing")
	}
	if mf.ID == "" {
		mf.ID = model.ID(mmpr.GenerateID())
	}
	fv := b.flowVersion()
	gm := microflowToGen(mf, fv)
	gm.SetID(element.ID(mf.ID))
	assignMicroflowIDs(gm)
	contents, err := microflowEncoder(fv).Encode(gm)
	if err != nil {
		return fmt.Errorf("CreateMicroflow: encode: %w", err)
	}
	if contents, err = patchMicroflowToolboxEntries(contents, mf); err != nil {
		return fmt.Errorf("CreateMicroflow: %w", err)
	}
	return b.writer.InsertUnit(string(mf.ID), string(mf.ContainerID), "Documents", "Microflows$Microflow", contents)
}

// UpdateMicroflow rebuilds a microflow document (the CREATE OR REPLACE path).
func (b *Backend) UpdateMicroflow(mf *microflows.Microflow) error {
	if mf == nil {
		return fmt.Errorf("UpdateMicroflow: nil microflow")
	}
	if err := flowImportMappingEntityError(mf.ObjectCollection); err != nil {
		return fmt.Errorf("UpdateMicroflow %s: %w", mf.Name, err)
	}
	if b.writer == nil {
		return fmt.Errorf("UpdateMicroflow: not connected for writing")
	}
	fv := b.flowVersion()
	gm := microflowToGen(mf, fv)
	gm.SetID(element.ID(mf.ID))
	assignMicroflowIDs(gm)
	contents, err := microflowEncoder(fv).Encode(gm)
	if err != nil {
		return fmt.Errorf("UpdateMicroflow: encode: %w", err)
	}
	if contents, err = patchMicroflowToolboxEntries(contents, mf); err != nil {
		return fmt.Errorf("UpdateMicroflow: %w", err)
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
// serializer (writer_microflow.go). v10+ adds Url/UrlSearchParameters (the
// property floor drops them empty below 10.1/10.9), 10.12+ ReturnVariableName,
// and 10.2+ StableId (via microflowEncoder).
func microflowToGen(mf *microflows.Microflow, v flowVersion) *genMf.Microflow {
	out := genMf.NewMicroflow()
	out.SetName(mf.Name)
	out.SetDocumentation(mf.Documentation)
	out.SetExcluded(mf.Excluded)
	// Carried, not hardcoded — but defaulted, because "" is not a member of
	// MicroflowsExportLevel and an enum-valued property must never be written
	// with a value the metamodel does not declare. A fresh microflow and a
	// stored document that says nothing both get "Hidden", which is what this
	// line always wrote, so the default case is unchanged.
	exportLevel := mf.ExportLevel
	if exportLevel == "" {
		exportLevel = "Hidden"
	}
	out.SetExportLevel(exportLevel)
	out.SetAllowConcurrentExecution(mf.AllowConcurrentExecution)
	// Carried, not hardcoded. This was `false` unconditionally, which silently
	// turned a microflow's "apply entity access" OFF on every rewrite.
	out.SetApplyEntityAccess(mf.ApplyEntityAccess)
	out.SetMarkAsUsed(mf.MarkAsUsed)
	// Carried, not hardcoded. These two are what Mendix does to a second caller
	// when AllowConcurrentExecution is false, and Mendix requires one of them in
	// that case (CE4899) — writing both empty on every rewrite deleted the
	// answer along with the question. nil still yields the bare empty
	// Texts$Text this line always wrote (Items=[3] via the registered default),
	// so a microflow without a message is unchanged — semantically, which is
	// the level that matters: the element's $ID is minted fresh on every encode,
	// so it is canon.Equal that holds here, never bytes.Equal (ADR-0008).
	out.SetConcurrencyErrorMicroflowQualifiedName(mf.ConcurrencyErrorMicroflow)
	if mf.ConcurrencyErrorMessage != nil {
		out.SetConcurrencyErrorMessage(textToGen(mf.ConcurrencyErrorMessage))
	} else {
		out.SetConcurrencyErrorMessage(genTexts.NewText())
	}
	out.SetAllowedModuleRolesQualifiedNames(moduleRoleNames(mf.AllowedModuleRoles))
	out.SetMicroflowReturnType(microflowDataTypeToGen(mf.ReturnType))

	// Object collection (parameters merged first, then objects).
	oc := genMf.NewMicroflowObjectCollection()
	for i, p := range mf.Parameters {
		oc.AddObjects(microflowParameterToGen(p, i, mf.Parameters, v))
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
			out.AddFlows(sequenceFlowToGen(f, v))
		}
		for _, af := range mf.ObjectCollection.AnnotationFlows {
			out.AddFlows(annotationFlowToGen(af, v))
		}
	}

	if v.hasReturnVariableName() {
		out.SetReturnVariableName(mf.ReturnVariableName)
	}
	if v.major >= 10 {
		// Carried, not hardcoded. These two were `""` and `nil` unconditionally,
		// so every rewrite deleted the microflow's deep link (#1120). A fresh
		// microflow has neither, so the empty values still come out empty —
		// SetUrlSearchParametersQualifiedNames(nil) is the empty marker-1 list.
		out.SetUrl(mf.URL)
		// StableId is emitted as a fresh GUID binary via the registered default
		// (the gen mistypes it as a string), not set here; microflowEncoder
		// leaves it out below 10.2.
		out.SetUrlSearchParametersQualifiedNames(mf.URLSearchParameters)
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
	case *microflows.BreakEvent:
		// `break;` in a loop. Same hazard as ErrorEvent above: dropping it left the
		// branch's SequenceFlow pointing at a non-existent object, so Studio Pro
		// failed to open the project with KeyNotFoundException (#791).
		g := genMf.NewBreakEvent()
		g.SetID(element.ID(o.ID))
		g.SetRelativeMiddlePoint(pointStr(o.Position))
		g.SetSize(sizeStr(o.Size))
		return g
	case *microflows.ContinueEvent:
		// `continue;` in a loop — see BreakEvent above.
		g := genMf.NewContinueEvent()
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
	case *microflows.InheritanceSplit:
		// Without this the split hit `default: return nil` and was dropped, while
		// the sequence flows referencing its $ID were still written — a dangling
		// pointer that mxbuild cannot even load ("KeyNotFoundException ... at
		// StreamingBsonUnitReader.ResolvePostponedProperties"). Same shape as the
		// ErrorEvent/BreakEvent gap in #791. Fields mirror the legacy serializer
		// in sdk/mpr/writer_microflow.go.
		g := genMf.NewInheritanceSplit()
		g.SetID(element.ID(o.ID))
		g.SetCaption(o.Caption)
		g.SetDocumentation(o.Documentation)
		g.SetRelativeMiddlePoint(pointStr(o.Position))
		g.SetSize(sizeStr(o.Size))
		g.SetSplitVariableName(o.VariableName)
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
		// Always written, empty included: the reader keeps it, and a loop
		// without the key is one Mendix's merge engine cannot compare with
		// the Studio Pro revision that has it (mendixlabs/mxcli#1373).
		g.SetDocumentation(o.Documentation)
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
func annotationFlowToGen(af *microflows.AnnotationFlow, v flowVersion) element.Element {
	g := genMf.NewAnnotationFlow()
	g.SetID(element.ID(af.ID))
	g.SetOriginID(element.ID(af.OriginID))
	g.SetDestinationID(element.ID(af.DestinationID))
	// The builder picks the sides that face each other (orientAnnotationFlows);
	// a flow nothing oriented is drawn as for a note ABOVE its element — out of
	// the note's bottom edge, into the element's top. Both indexes were once 0
	// (top), which drew the line out of the top of the note and back down
	// around it.
	sides := af.SidesOrDefault()
	g.SetOriginConnectionIndex(int32(sides.Origin))
	g.SetDestinationConnectionIndex(int32(sides.Destination))
	// An annotation flow is a Flow like a sequence flow, with the same floors:
	// the Line was written from 10.0 on, six minors before it existed.
	if v.hasLine() {
		line := genMf.NewBezierCurve()
		line.SetOriginControlVector("0;0")
		line.SetDestinationControlVector("0;0")
		g.SetLine(line)
	} else {
		g.SetOriginBezierVector("0;0")
		g.SetDestinationBezierVector("0;0")
		if v.hasLineType() {
			g.SetLineType("BezierCurve")
		}
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
		g.SetRefreshInClient(a.RefreshInClient)
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
		// Studio Pro writes no ErrorHandlingType on a delete; set it only when the
		// author asked for one.
		if a.ErrorHandlingType != "" {
			g.SetErrorHandlingType(string(a.ErrorHandlingType))
		}
		g.SetDeleteVariableName(a.DeleteVariable)
		g.SetRefreshInClient(a.RefreshInClient)
		return g
	case *microflows.RollbackObjectAction:
		g := genMf.NewRollbackAction()
		g.SetID(element.ID(a.ID))
		g.SetRollbackVariableName(a.RollbackVariable)
		g.SetRefreshInClient(a.RefreshInClient)
		// Like delete: set only when the author wrote an `on error` clause, so a
		// rollback without one is written exactly as it always was (#1252).
		if a.ErrorHandlingType != "" {
			g.SetErrorHandlingType(string(a.ErrorHandlingType))
		}
		return g
	case *microflows.RetrieveAction:
		g := genMf.NewRetrieveAction()
		g.SetID(element.ID(a.ID))
		// Only an explicit ON ERROR clause moves this off the literal both engines
		// have always written (ako/CapTrackV3 FINDINGS §11).
		g.SetErrorHandlingType(orDefault(string(a.ErrorHandlingType), "Rollback"))
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
			if qs := queueSettingsToGen(a.MicroflowCall.QueueSettings); qs != nil {
				mc.SetQueueSettings(qs)
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
	case *microflows.SendEmailAction:
		return sendEmailActionToGen(a)
	case *microflows.SynchronizeAction:
		// Built directly rather than through genMf.NewSynchronizeAction: gen binds
		// VariableNames as a scalar Primitive[string] and exposes no setter for
		// it, so a Specific-mode action could not be written at all through the
		// generated type. An EnumList[string] encodes the BSON array of names the
		// platform actually stores.
		g := newElem("Microflows$SynchronizeAction", string(a.ID))
		addStr(g, "ErrorHandlingType", orDefault(string(a.ErrorHandlingType), "Rollback"))
		addStr(g, "Type", orDefault(string(a.SyncType), string(microflows.SynchronizationTypeAll)))
		addStrList(g, "VariableNames", a.VariableNames)
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
		if qs := queueSettingsToGen(a.QueueSettings); qs != nil {
			addPart(g, "QueueSettings", qs)
		}
		return g
	case *microflows.JavaScriptActionCallAction:
		// Built directly like JavaActionCallAction: the JS action call binds
		// distinct storage keys (JavaScriptAction/OutputVariableName, and the
		// parameter value goes under "ParameterValue", not "Value"). Mirrors the
		// legacy serializer (sdk/mpr/writer_microflow_actions.go). Without this
		// case the action fell through to `default: return nil`, so the
		// ActionActivity was written with no Action → CE0008 "No action defined."
		// and CE0109 on the (now-missing) output variable.
		g := newElem("Microflows$JavaScriptActionCallAction", string(a.ID))
		addStr(g, "ErrorHandlingType", orDefault(string(a.ErrorHandlingType), "Rollback"))
		addStr(g, "JavaScriptAction", a.JavaScriptAction)
		addStr(g, "OutputVariableName", a.OutputVariableName)
		addBool(g, "UseReturnVariable", a.UseReturnVariable)
		mappings := make([]element.Element, 0, len(a.ParameterMappings))
		for _, pm := range a.ParameterMappings {
			m := newElem("Microflows$JavaScriptActionParameterMapping", string(pm.ID))
			addStr(m, "Parameter", pm.Parameter)
			if pm.Value != nil {
				if v := codeActionParameterValueToGen(pm.Value); v != nil {
					addPart(m, "ParameterValue", v)
				}
			}
			mappings = append(mappings, m)
		}
		addPartList(g, "ParameterMappings", mappings)
		return g
	case *microflows.DownloadFileAction:
		// DOWNLOAD FILE. Without this case the action fell through to
		// `default: return nil` and the enclosing ActionActivity was written with
		// no Action at all — `mxcli exec` reported "Created microflow" and only
		// `mx check` noticed, as CE0008 "No action defined." (issue #850).
		//
		// The storage key is ShowFileInBrowser, not ShowInBrowser; the gen setter
		// binds the right one (legacy's parseDownloadFileAction reads the wrong
		// key — see TestActionFromGen_DownloadFile).
		g := genMf.NewDownloadFileAction()
		g.SetID(element.ID(a.ID))
		g.SetErrorHandlingType(orDefault(string(a.ErrorHandlingType), "Rollback"))
		g.SetFileDocumentVariableName(a.FileDocument)
		g.SetShowFileInBrowser(a.ShowInBrowser)
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
		g.SetErrorHandlingType(orDefault(string(a.ErrorHandlingType), "Rollback"))
		g.SetOutputVariableName(a.OutputVariable)
		return g
	case *microflows.AggregateListAction:
		// Storage $Type Microflows$AggregateAction. Input bound to
		// "AggregateVariableName", output to "VariableName"; Attribute is a
		// by-name ref. Expression mode is mutually exclusive with Attribute.
		g := genMf.NewAggregateListAction()
		g.SetID(element.ID(a.ID))
		g.SetErrorHandlingType(orDefault(string(a.ErrorHandlingType), "Rollback"))
		g.SetAggregateFunction(string(a.Function))
		g.SetInputListVariableName(a.InputVariable)
		if a.UseExpression {
			g.SetUseExpression(true)
			g.SetExpression(a.Expression)
		}
		// Written even when unused: every Studio Pro reference document carries
		// Attribute as "". Omitting it made a freshly described Studio Pro
		// aggregate rewrite on its first execution for no semantic reason.
		g.SetAttributeQualifiedName(a.AttributeQualifiedName)
		// Reduce's fold. Written for the functions a reference document shows
		// Mendix storing them on, and otherwise only to carry back what the
		// stored document already had (#1004).
		if a.Function.WritesReduceProperties() || a.ReduceInitialValue != "" || a.ReduceReturnType != nil {
			g.SetReduceInitialValueExpression(a.ReduceInitialValue)
			g.SetReduceReturnDataType(microflowDataTypeToGen(a.ReduceReturnType))
		}
		g.SetOutputVariableName(a.OutputVariable)
		return g
	case *microflows.CreateListAction:
		// Storage $Type Microflows$CreateListAction; output bound to "VariableName".
		g := genMf.NewCreateListAction()
		g.SetID(element.ID(a.ID))
		// The flow flavour's default, which the builder supplies: Rollback in a
		// microflow, Abort in a nanoflow — "Rollback" there is CE6035
		// (mendixlabs/mxcli#591). Same for the other list activities and cast.
		g.SetErrorHandlingType(orDefault(string(a.ErrorHandlingType), "Rollback"))
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
		g.SetErrorHandlingType(orDefault(string(a.ErrorHandlingType), "Rollback"))
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
		addStr(e, "ErrorHandlingType", orDefault(string(a.ErrorHandlingType), "Rollback"))
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
		// An expression; "" (the default) closes no page. It was always written
		// as "", whatever the statement said (mendixlabs/mxcli#1617).
		g.SetNumberOfPagesToClose(a.NumberOfPagesToClose)
		return g
	case *microflows.ClosePageAction:
		// Storage $Type Microflows$CloseFormAction. The count is the string
		// NumberOfPagesToClose ("" = one page), as Studio Pro stores it. The
		// int NumberOfPages was deleted in 8.11: writing it (as the legacy
		// serializer did) made `close page 2` close one page, and left a key
		// Mendix's merge engine does not have (mendixlabs/mxcli#1373).
		g := genMf.NewCloseFormAction()
		g.SetID(element.ID(a.ID))
		g.SetErrorHandlingType(orDefault(string(a.ErrorHandlingType), "Rollback"))
		g.SetNumberOfPagesToClose(numberOfPagesToClose(a.NumberOfPages))
		return g
	case *microflows.ShowHomePageAction:
		// Storage $Type Microflows$ShowHomePageAction. Its only property is
		// ErrorHandlingType: the authored clause, else the literal "Rollback" the
		// writer has always stored (#1252).
		g := genMf.NewShowHomePageAction()
		g.SetID(element.ID(a.ID))
		g.SetErrorHandlingType(orDefault(string(a.ErrorHandlingType), "Rollback"))
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
	case *microflows.TransformJsonAction:
		// "transform $In with Module.Transformer". Without this case the action
		// fell through to `default: return nil` and the enclosing ActionActivity
		// was written with NO action at all — `mxcli exec` reported success and
		// mxbuild then failed CE0008 "No action defined." This is the #850 shape,
		// and it means `transform` was unusable on the default engine while the
		// legacy writer handled it. Keys mirror sdk/mpr.serializeTransformJsonAction.
		g := newElem("Microflows$TransformJsonAction", string(a.ID))
		addStr(g, "ErrorHandlingType", orDefault(string(a.ErrorHandlingType), "Rollback"))
		addStr(g, "InputVariableName", a.InputVariableName)
		addStr(g, "OutputVariableName", a.OutputVariableName)
		addStr(g, "Transformation", a.Transformation)
		return g

	case *microflows.RestOperationCallAction:
		// "call rest operation" — Microflows$RestOperationCallAction. Mirrors
		// serializeRestOperationCallAction.
		return restOperationCallActionToGen(a)
	case *microflows.CallExternalAction:
		// "call external action" — Microflows$CallExternalAction. Without this
		// the activity serialized with no action → CE0008 "No action defined".
		return callExternalActionToGen(a)
	case *microflows.WebServiceCallAction:
		// "call web service" (legacy SOAP) — Microflows$CallWebServiceAction.
		// Same CE0008 shape as the two cases above, and the reason the legacy
		// engine was still the documented fallback for SOAP. See
		// microflow_webservice_write.go.
		return webServiceCallActionToGen(a)
	default:
		return nil // not yet supported (added in later groups)
	}
}

// importMappingRangeToGen builds the Range child of a Microflows$ImportMappingCall.
//
// Mendix has two variants and mxcli only ever wrote the first, so Studio Pro's
// "Custom" setting was unrepresentable rather than merely undescribed:
//
//	Microflows$ConstantRange{SingleObject}                     All (false) / First (true)
//	Microflows$CustomRange{LimitExpression, OffsetExpression}  Custom
//
// A limit or an offset selects CustomRange; SingleObject has no meaning there,
// because a bounded range is always a list.
//
// Shared because the ImportMappingCall is built at THREE sites in this engine —
// importXmlActionToGen (the `import from mapping` statement), the
// ResultHandlingMapping case (REST result handling), and the legacy writer's own
// copy. Fixing one and not the others is how a limit reached the model and was
// still written as a ConstantRange. (issue #881)
func importMappingRangeToGen(h *microflows.ResultHandlingMapping) *element.Base {
	if h.LimitExpression != "" || h.OffsetExpression != "" {
		rng := newElem("Microflows$CustomRange", "")
		addStr(rng, "LimitExpression", h.LimitExpression)
		addStr(rng, "OffsetExpression", h.OffsetExpression)
		return rng
	}
	rng := newElem("Microflows$ConstantRange", "")
	addBool(rng, "SingleObject", microflows.RangeSingleObjectOf(h))
	return rng
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
	// Both were constants, so a mapping with an input object could not be
	// called (mxbuild CE0244) and a non-default commit mode was unwritable
	// (ako/mxcli#1681).
	addStr(imc, "Commit", orDefault(rh.Commit, "YesWithoutEvents"))
	addStr(imc, "ContentType", orDefault(rh.ContentType, "Json"))
	addBool(imc, "ForceSingleOccurrence", forceSingle)
	addStr(imc, "ObjectHandlingBackup", "Create")
	addStr(imc, "ParameterVariableName", rh.ParameterVariable)
	addPart(imc, "Range", importMappingRangeToGen(rh))
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
		// A text-template (or SQL-template) Java action parameter: the text with
		// {n} placeholders plus one PrimitiveTypedTemplateArgument per
		// placeholder, each an expression and its result type
		// (ako/mxcli#1451; shape from the metamodel, accepted by mxbuild 11.13).
		g := newElem("Microflows$StringTemplateParameterValue", string(val.ID))
		if val.TypedTemplate != nil {
			tt := newElem("Microflows$TypedTemplate", string(val.TypedTemplate.ID))
			var args []element.Element
			for _, a := range val.TypedTemplate.Arguments {
				if a == nil {
					continue
				}
				ae := newElem("Microflows$PrimitiveTypedTemplateArgument", string(a.ID))
				addStr(ae, "Expression", a.Expression)
				addStr(ae, "Type", a.Type)
				args = append(args, ae)
			}
			addPartList(tt, "Arguments", args) // marker 2, registered in init
			addStr(tt, "Text", val.TypedTemplate.Text)
			addPart(g, "TypedTemplate", tt)
		}
		return g
	case *microflows.CustomBlobDocumentCodeActionParameterValue:
		// A document of a custom type (an agent-editor Agent), by name
		// (ako/mxcli#1451).
		g := newElem("Microflows$CustomBlobDocumentCodeActionParameterValue", string(val.ID))
		addStr(g, "CustomDocument", val.CustomDocument)
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

// queueSettingsToGen builds the Queues$QueueSettings child that binds a call
// activity to a task queue. Returns nil for an unqueued call, so the call's
// registered NullFields default writes QueueSettings: null as before.
//
// Only the QueueSettings element is written — NOT the call's sibling `Queue`
// string. gen carries a `Queue` ByNameRef on both call types, but
// generated/metamodel (the arbiter) has neither, and measured on 11.13 a call
// carrying only `Queue` is inert: mx check does not read it. Writing both would
// mean two places to keep in sync, one of which nothing consumes.
func queueSettingsToGen(qs *microflows.QueueSettings) element.Element {
	if qs == nil || qs.Queue == "" {
		return nil
	}
	g := newElem("Queues$QueueSettings", string(qs.ID))
	addStr(g, "Queue", qs.Queue)
	return g
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

// addInt32 adds a dirty Integer property (BSON key = name) to a bare element.
// It is stored as BSON int64, like every Integer property Studio Pro writes —
// the widening is property.Primitive's (ako/mxcli#1264) — so this and addInt64
// produce the same document; the int32 parameter is only the Go-side type.
func addInt32(b *element.Base, name string, val int32) {
	p := property.NewPrimitive[int32](name, property.DecodeInt32)
	b.AddProperty(p, uint(len(b.Properties())))
	p.Set(val)
}

// addStrList adds a dirty string-list property (BSON key = name).
//
// The leading int32 is Mendix's array version marker, and it is load-bearing:
// every array in a Mendix document carries one, and the reader treats element 0
// as the version rather than as data. Written without it, a one-element list
// reads back as an empty list — mxbuild reported CE2004 "No variables to
// synchronize have been selected." for a Synchronize action whose VariableNames
// array plainly held the variable (#863).
//
// property.EnumList is NOT used here for exactly that reason: its BSONValue
// returns a bare []string, so the codec's scalar path emits an unmarked array.
func addStrList(b *element.Base, name string, vals []string) {
	arr := bson.A{int32(1)}
	for _, v := range vals {
		arr = append(arr, v)
	}
	p := property.NewPrimitive[bson.A](name, func(raw bson.Raw, key string) bson.A { return nil })
	b.AddProperty(p, uint(len(b.Properties())))
	p.Set(arr)
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
		// The bounds are NOT properties of the ListRange. Mendix nests them:
		//
		//	Microflows$ListRange{ListName, CustomRange}
		//	  └── Microflows$CustomRange{LimitExpression, OffsetExpression}
		//
		// Writing them flat produced a range mxbuild reads as unbounded —
		// CE6520 "Amount and offset are not specified. Either amount or offset
		// or both must be specified." It survived review because the reader
		// beside it looked flat too, so mxcli round-tripped its own documents
		// perfectly and only Mendix disagreed. (issue #966)
		//
		// The nesting matches the ImportMappingCall's Range (importMappingRangeToGen
		// above) — same CustomRange element, a different parent.
		e := newElem("Microflows$ListRange", string(o.ID))
		addStr(e, "ListName", o.ListVariable)
		if o.LimitExpression != "" || o.OffsetExpression != "" {
			cr := newElem("Microflows$CustomRange", "")
			addStr(cr, "LimitExpression", o.LimitExpression)
			addStr(cr, "OffsetExpression", o.OffsetExpression)
			addPart(e, "CustomRange", cr)
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

// splitConditionToGen builds an exclusive-split condition — either an expression
// or a call into a rule. Inverse of splitConditionFromGen.
//
// The rule case is the one #939 reported: it used to fall through to nil and the
// caller skipped SetSplitCondition entirely, so a decision that reads
// `if Module.SomeRule(...) then` was stored with its Condition missing — mx check
// CE0080 "The 'Condition' property is required", with the Decision's caption
// still showing the call. The legacy engine had always written it
// (sdk/mpr/writer_microflow.go), which is why the same script passed there.
func splitConditionToGen(sc microflows.SplitCondition) element.Element {
	switch c := sc.(type) {
	case *microflows.ExpressionSplitCondition:
		g := genMf.NewExpressionSplitCondition()
		g.SetID(element.ID(c.ID))
		g.SetExpression(c.Expression)
		return g
	case *microflows.RuleSplitCondition:
		g := genMf.NewRuleSplitCondition()
		g.SetID(element.ID(c.ID))
		g.SetRuleCall(ruleCallToGen(c))
		return g
	default:
		return nil
	}
}

// ruleCallToGen builds the RuleCall sub-document a RuleSplitCondition wraps. The
// rule's qualified name goes to the "Microflow" storage key (patched in
// initRuleCall); the RuleCall itself carries no semantic ID, so it gets a fresh
// one like every other synthesized part.
func ruleCallToGen(c *microflows.RuleSplitCondition) element.Element {
	rc := genMf.NewRuleCall()
	assignID(rc)
	rc.SetRuleQualifiedName(c.RuleQualifiedName)
	for _, pm := range c.ParameterMappings {
		g := genMf.NewRuleCallParameterMapping()
		g.SetID(element.ID(pm.ID))
		g.SetParameterQualifiedName(pm.ParameterName)
		g.SetArgument(pm.Argument)
		rc.AddParameterMappings(g)
	}
	return rc
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
	case microflows.InheritanceCase:
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
	case *microflows.InheritanceCase:
		// A type-split branch selects on an entity. Without this it fell through
		// to NoCase, so every branch lost the entity it matches on — the second
		// half of the `split type` corruption.
		g := genMf.NewInheritanceCase()
		g.SetID(element.ID(c.ID))
		g.SetValueQualifiedName(c.EntityQualifiedName)
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

// microflowParameterToGen builds a gen MicroflowParameter. An authored position
// is written as given; without one the position derives from the index and the
// names of all the flow's parameters (params), which space the row.
func microflowParameterToGen(p *microflows.MicroflowParameter, idx int, params []*microflows.MicroflowParameter, v flowVersion) element.Element {
	g := genMf.NewMicroflowParameter()
	g.SetID(element.ID(p.ID))
	g.SetDocumentation(p.Documentation)
	g.SetHasVariableNameBeenChanged(false)
	g.SetName(p.Name)
	pos := microflows.DerivedParameterPosition(idx, params)
	if p.Position != nil {
		pos = *p.Position
	}
	g.SetRelativeMiddlePoint(fmt.Sprintf("%d;%d", pos.X, pos.Y))
	g.SetSize("30;30")
	if v.hasParameterDefaults() {
		g.SetDefaultValue("")
		g.SetIsRequired(true)
	}
	g.SetParameterType(microflowDataTypeToGen(p.Type))
	return g
}

// sequenceFlowToGen builds a gen SequenceFlow in the shape v stores; the case
// defaults to NoCase. Measured shapes (mendixlabs/mxcli#1410):
//
//	< 10.15   the case as one NewCaseValue document; 10.15+ a CaseValues list
//	< 10.16   Origin/DestinationBezierVector strings (with LineType "BezierCurve"
//	          from 10.8); 10.16+ a BezierCurve Line holding the control vectors
func sequenceFlowToGen(f *microflows.SequenceFlow, v flowVersion) element.Element {
	g := genMf.NewSequenceFlow()
	g.SetID(element.ID(f.ID))
	g.SetOriginID(element.ID(f.OriginID))
	g.SetDestinationID(element.ID(f.DestinationID))
	g.SetOriginConnectionIndex(int32(f.OriginConnectionIndex))
	g.SetDestinationConnectionIndex(int32(f.DestinationConnectionIndex))
	g.SetIsErrorHandler(f.IsErrorHandler)
	if v.hasCaseValueList() {
		g.AddCaseValues(caseValueToGen(f.CaseValue))
	} else {
		g.SetCaseValue(caseValueToGen(f.CaseValue))
	}

	originCV := orDefault(f.OriginControlVector, "0;0")
	destCV := orDefault(f.DestinationControlVector, "0;0")
	if v.hasLine() {
		line := genMf.NewBezierCurve()
		line.SetOriginControlVector(originCV)
		line.SetDestinationControlVector(destCV)
		g.SetLine(line)
	} else {
		g.SetOriginBezierVector(originCV)
		g.SetDestinationBezierVector(destCV)
		if v.hasLineType() {
			g.SetLineType("BezierCurve")
		}
	}
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
	// TitleOverride is nil unless the action actually overrides the page title. An
	// empty template is not "no override" — it overrides with the empty string, which
	// blanked the caption of every popup an mxcli-authored action opened (#812).
	if a.OverridePageTitle != nil {
		addPart(fs, "TitleOverride", textTemplateToGen(a.OverridePageTitle, nil))
	}
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
	addStr(e, "RequestHandlingType", requestHandlingTypeOf(a.RequestHandling))
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
		case *microflows.ResultHandlingFileDocument:
			resultHandlingType = "FileDocument"
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
// requestHandlingTypeOf is the action-level discriminator that must agree with
// the RequestHandling sub-element. It was hardcoded to "Custom" in both engines
// regardless of the handler, so an export-mapping body claimed to be a custom
// template.
//
// Measured against Studio Pro microflows (ako/TestApp, 11.13.0): Custom,
// Mapping, FormData and Binary each pair with the matching sub-element. Simple
// follows the same name rule but has no measured reference.
func requestHandlingTypeOf(rh microflows.RequestHandling) string {
	switch rh.(type) {
	case *microflows.MappingRequestHandling:
		return "Mapping"
	case *microflows.BinaryRequestHandling:
		return "Binary"
	case *microflows.FormDataRequestHandling:
		return "FormData"
	case *microflows.SimpleRequestHandling:
		return "Simple"
	default:
		return "Custom"
	}
}

func restRequestHandlingToGen(rh microflows.RequestHandling) element.Element {
	switch h := rh.(type) {
	case *microflows.CustomRequestHandling:
		e := newElem("Microflows$CustomRequestHandling", string(h.ID))
		addPart(e, "Template", stringTemplateElem(h.Template, h.TemplateParams))
		return e
	case *microflows.MappingRequestHandling:
		e := newElem("Microflows$MappingRequestHandling", string(h.ID))
		addStr(e, "MappingId", string(h.MappingID))
		// generated/metamodel gives this type exactly three properties:
		// contentType (Json|Xml), mappingId, mappingVariableName. mxcli wrote
		// "ParameterVariable" — a key the type does not own, which mxbuild
		// tolerates and Studio Pro refuses to open — and left ContentType empty,
		// which is not a member of the enum. Studio Pro writes
		// {ContentType: "Json", MappingVariableName: "<var>"}.
		addStr(e, "ContentType", orDefault(h.ContentType, "Json"))
		addStr(e, "MappingVariableName", h.ParameterVariable)
		return e
	case *microflows.BinaryRequestHandling:
		e := newElem("Microflows$BinaryRequestHandling", string(h.ID))
		addStr(e, "Expression", h.Expression)
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
	case *microflows.ResultHandlingFileDocument:
		e := newElem("Microflows$ResultHandling", string(h.ID))
		addBool(e, "Bind", outputVar != "")
		addStr(e, "ResultVariableName", outputVar)
		vt := newElem("DataTypes$ObjectType", "")
		addStr(vt, "Entity", h.EntityRef)
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
		addStr(imc, "ContentType", orDefault(h.ContentType, "Json"))
		addBool(imc, "ForceSingleOccurrence", forceSingle)
		addStr(imc, "ObjectHandlingBackup", "Create")
		addStr(imc, "ParameterVariableName", "")
		addPart(imc, "Range", importMappingRangeToGen(h))
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
			assignSequenceFlowIDs(sf)
		}
	}
}

// assignSequenceFlowIDs assigns IDs to a sequence flow's case and line, in
// whichever shape the flow was built (sequenceFlowToGen).
func assignSequenceFlowIDs(sf *genMf.SequenceFlow) {
	assignID(sf.CaseValue())
	for _, cv := range sf.CaseValuesItems() {
		assignID(cv)
	}
	assignID(sf.Line())
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

// patchMicroflowToolboxEntries writes the two "expose as … action" sub-documents
// into an encoded microflow unit.
//
// They are injected post-encode for the same reason the Java action's is (#656):
// the four icon/image fields are BSON *binary*, and gen declares them as string,
// so anything routed through the gen accessors would silently turn a real PNG
// into an empty value. Going to the document directly is the only lossless path.
//
// A nil entry writes BSON null — the absence of a toolbox entry — which is what
// gen already encodes, so the patch is a no-op in that case.
func patchMicroflowToolboxEntries(contents []byte, mf *microflows.Microflow) ([]byte, error) {
	var err error
	for _, e := range []struct {
		key  string
		info *javaactions.MicroflowActionInfo
	}{
		{"MicroflowActionInfo", mf.MicroflowActionInfo},
		{"WorkflowActionInfo", mf.WorkflowActionInfo},
	} {
		if e.info == nil {
			continue
		}
		if contents, err = codec.PatchBSONField(contents, e.key, microflowActionInfoBSON(e.info)); err != nil {
			return nil, fmt.Errorf("patch %s: %w", e.key, err)
		}
	}
	return contents, nil
}

// numberOfPagesToClose renders a close-page count as Studio Pro stores it: the
// empty string for the default single page, the number otherwise.
func numberOfPagesToClose(n int) string {
	if n <= 1 {
		return ""
	}
	return strconv.Itoa(n)
}
