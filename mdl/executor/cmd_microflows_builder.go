// SPDX-License-Identifier: Apache-2.0

// Package executor - Microflow flow graph builder: core types and helpers
package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// flowBuilder helps construct the flow graph from AST statements.
type flowBuilder struct {
	// duplicateNamesOwnedElsewhere stops the check-time body validation
	// (validateFlowBody) from reporting a variable name created twice. For a
	// microflow or nanoflow that is MDL063's job, which knows the namespace is
	// flow-wide and that a void action call declares nothing (#953); this
	// validator scoped names per branch and counted every call output, so it
	// was wrong both ways. Rules keep it — MDL063 does not run on them.
	duplicateNamesOwnedElsewhere bool
	// checkAssocShapes is check --references' view of the project's and the
	// script's associations, so the validator types a forward Reference
	// retrieve as the object it yields (validateFlowBody). nil on the exec path,
	// which resolves associations through the backend instead.
	checkAssocShapes map[string]assocShape
	// assocObjectVars are the variables checkAssocShapes typed as one object,
	// the only objects whose `set` the validator reports itself (MDL-SET01
	// reports the rest without a project).
	assocObjectVars map[string]bool

	objects []microflows.MicroflowObject
	flows   []*microflows.SequenceFlow
	// statedIncludes are the external action parameter mappings whose
	// statement wrote `include associations ( … )`: the tree is the statement's,
	// so the stored one is not carried onto them (ako/mxcli#1914).
	statedIncludes map[*microflows.ExternalActionParameterMapping]bool
	// resolveKnownOnly is check's mode for include associations: an
	// association or entity the project does not hold yet may be one an
	// earlier statement creates, so only what the project decides is reported.
	resolveKnownOnly bool
	annotationFlows  []*microflows.AnnotationFlow

	// storedNotes resolves `@annotation(existing: '…')` to the note of the
	// stored flow with that text, keyed by the text as the script writes it.
	// Set only for an alter fragment, by resolveStoredNotes, which refuses a
	// text that names no note or several; nil means there is no stored flow
	// to attach to — a create body — and the form is refused
	// (ako/mxcli#1695).
	storedNotes map[string]*microflows.Annotation

	// annotationsByLabel resolves `@annotation(id: n1)` back to the Annotation
	// its first mention created, so a note wired to several activities is ONE
	// object with several flows — the shape Mendix stores and the shape MDL
	// could not previously express (#1077). Scoped to this flow build; labels
	// are not stored in the model.
	annotationsByLabel map[string]*microflows.Annotation
	posX               int
	posY               int
	baseY              int // Base Y position (for returning after ELSE branches)
	spacing            int
	returnValue        string // Return value expression for RETURN statement (used by buildFlowGraph final EndEvent)
	returnType         *ast.MicroflowReturnType
	endsWithReturn     bool     // True if the flow already ends with EndEvent(s) from RETURN statements
	lastReturnEndID    model.ID // Last explicit RETURN EndEvent, used as a fallback error-handler target
	// returnEndIDs are the end events RETURN statements drew, and
	// fallThroughEndID the one the builder adds where the body falls through
	// to its end. A spliced fragment keeps the first as new ends of the flow
	// and cuts the second out (ako/mxcli#888).
	returnEndIDs     map[model.ID]bool
	fallThroughEndID model.ID
	varTypes         map[string]string // Variable name -> entity qualified name (for CHANGE statements)
	// generatedVars holds the output-variable names minted for an unassigned
	// CREATE. Kept apart from varTypes so a generated name is never referenceable
	// from the script, while still reserving the name against a second create of
	// the same entity (see freshCreateVariable).
	generatedVars map[string]bool
	// qualifiedMembersOnly refuses an entity member the builder cannot
	// qualify — for want of the entity its variable holds — instead of writing
	// it bare. Set for a fragment spliced into a stored flow (ako/mxcli#885):
	// a bare change member makes the project unloadable ("not a valid
	// AttributeIdentifier"), a bare find member becomes a find by an invalid
	// expression, a bare aggregate attribute is dropped.
	qualifiedMembersOnly bool
	// textLang is the language a bare message/caption string is stored under
	// (mendixlabs/mxcli#970). Empty means en_US, which keeps a zero-value
	// flowBuilder — validateFlowBody builds one — behaving as before.
	textLang     string
	declaredVars map[string]string // Declared primitive variables: name -> type (e.g., "$IsValid" -> "Boolean")
	errors       []string          // Validation errors collected during build
	measurer     *layoutMeasurer   // For measuring statement dimensions
	// layout is the style the flow is laid out with (flowlayout.go); nil is the
	// default, which is the layout mxcli has always drawn.
	layout              *flowLayout
	nextConnectionPoint model.ID // For compound statements: the exit point differs from entry point
	// incomingRedirect, when set, instructs the next enclosing flow emission to
	// terminate at this merge/activity ID instead of the most recently emitted
	// activity. Used by retry-loop error handlers (where a merge is inserted
	// before the source activity so the normal inbound flow enters the merge
	// and the error-handler tail loops back to the same merge). Cleared after
	// the outer loop consumes it for the next flow.
	incomingRedirect model.ID
	nextFlowCase     string // If set, next connecting flow uses this case value (for merge-less splits)
	// nextFlowAnchor carries the branch-specific FlowAnchors that should be
	// applied to the flow created by the NEXT iteration of buildFlowGraph.
	// Used by guard-pattern IFs (where one branch returns and the other
	// continues) so the continuing branch's @anchor survives to the actual
	// splitID→nextActivity flow — which is emitted one iteration later by the
	// outer loop, not by addIfStatement.
	nextFlowAnchor *ast.FlowAnchors

	// atBodyTail is set while the LAST statement of a loop body is built, and
	// passed on to the last statement of each branch of an IF there. An IF/ELSE
	// at the tail joins nothing — both branches end the iteration — so it gets
	// no merge (mendixlabs/mxcli#884). Whoever reads it clears it.
	atBodyTail bool

	// statedNotes are the notes whose position the script states, which a
	// spliced fragment pins in place (see recordStatedNote).
	statedNotes map[model.ID]bool
	// curveByOrigin holds each statement's @curve keyed by the activity it was
	// written on, applied to that activity's outgoing flows by applyFlowCurves
	// once the graph is complete. (#884)
	curveByOrigin map[model.ID]*ast.FlowCurve
	// startPosition is the StartEvent position read off the microflow being
	// REPLACED, carried over so a hand-laid-out start survives the rebuild. The
	// start has no MDL statement to annotate and DESCRIBE cannot emit it, so
	// without this a describe→exec round-trip silently moved it (a Studio Pro
	// flow's 145;200 became 100;200). Nil on a fresh CREATE, where the position
	// is derived from the first annotated activity as before.
	startPosition *model.Point
	backend       backend.FullBackend // For looking up page/microflow references
	// assocCatalog is the project's associations with their ends, read once
	// per flow for the bare association names a retrieve resolves
	// (ako/mxcli#1690). nil until first needed.
	assocCatalog *associationCatalog
	// quiet silences the builder's own stderr warnings. Set for describe's
	// layout check (derivedFlowLayout), which rebuilds a flow it only reads and
	// must not repeat, once per round, a warning nobody asked for.
	quiet                bool
	hierarchy            *ContainerHierarchy          // For resolving container IDs to module names
	pendingAnnotations   *ast.ActivityAnnotations     // Pending annotations to attach to next activity
	restServices         []*model.ConsumedRestService // Cached REST services for parameter classification
	listInputVariables   map[string]bool              // Variables later consumed by list-only actions
	objectInputVariables map[string]bool              // Variables later consumed through object/attribute access
	// previousStmtAnchor holds the Anchor annotation of the statement that
	// just emitted an activity, so the next flow's OriginConnectionIndex can
	// be overridden by the user. Cleared after each flow is created.
	previousStmtAnchor *ast.FlowAnchors
	// Cached flow lists to avoid repeated backend calls during lookups.
	microflowsCache       []*microflows.Microflow
	microflowsCacheLoaded bool
	nanoflowsCache        []*microflows.Nanoflow
	nanoflowsCacheLoaded  bool
	manualLoopBackTarget  model.ID
	isNanoflow            bool // true when building a nanoflow — default error handling is "" not "Rollback"
	// self is the flow the statement creates. A call to it resolves to the
	// statement itself, not the project: on a first create the document does
	// not exist yet, so a recursive flow was refused "not found" (#843).
	self *selfFlow
	// scriptFlows are the flows earlier statements of the script declare
	// (ExecContext.scriptFlows): a call to one resolves to its declaration.
	// Set only while check predicts flow verdicts.
	scriptFlows map[string]*selfFlow
	// Pending custom error-handler routing uses two representations: the
	// currently active handler lives in the flat fields below, while handlers
	// postponed across branch boundaries are queued in pendingErrorHandlers.
	// Mutate this state through the helper methods in builder_flows.go so the
	// active/queued invariant stays synchronized.
	emptyErrorHandlerFrom    model.ID
	errorHandlerTailFrom     model.ID
	errorHandlerSource       model.ID
	errorHandlerSkipVar      string
	errorHandlerTailCase     string
	errorHandlerTailAnchor   *ast.FlowAnchors
	errorHandlerTailIsSource bool
	errorHandlerReturnValue  string
	pendingErrorHandlers     []pendingErrorHandlerState
	// rejoinMergeAt holds the @merge(x, y) written on an activity whose custom
	// error handler falls through: the position of the merge where the handler
	// rejoins the normal path, keyed by the activity. describe folds that merge
	// away into the fall-through form and carries its position here (#750).
	rejoinMergeAt map[model.ID]*ast.Position
	// rejoinPastRowX is the x of the merge an activity's falling-through
	// handler row rejoins at when the row is too long to rejoin before the next
	// statement (rejoinAfterRow). An @merge in rejoinMergeAt still wins.
	rejoinPastRowX map[model.ID]int
	// labelReg holds the `merge <label>` table and the pending `join` edges.
	// Shared with the error-handler sub-builder (same object collection) and
	// deliberately NOT with a loop's sub-builder, whose LoopedActivity owns a
	// collection a sequence flow may not leave. See builder_merge.go.
	labelReg *labelRegistry
	// pendingJoin is the `join` addStatement just saw, waiting for the enclosing
	// body loop to say which activity the path had reached.
	pendingJoin *ast.JoinStmt
	// lowerLane is how far right the lane under each main line (keyed by the line's
	// y) is occupied by a guard's branch. See layout_lanes.go.
	lowerLane map[int]int
	// allowWrap turns on row wrapping for this builder: the main line breaks onto a
	// new row past MaxRowWidth instead of running off the canvas (layout_rows.go).
	// Only the builders that lay out a whole microflow set it; a loop body builds in
	// its own coordinate space inside a box that is sized to fit, so wrapping there
	// would fight the box rather than help the reader.
	allowWrap bool
	row       rowTracker
	// joinScopes is the check-time validator's view of what is declared at
	// each `join <label>`, keyed by label, so a `merge <label>` statement sees
	// the variables of the paths that reach it (recordJoinScope). nil outside
	// validateFlowBody. A map, so the per-branch copies validateScopedStatements
	// makes all record into the same table.
	joinScopes map[string]flowBuilderVariableState
}

// nestedBuilder returns the builder for a body nested in this one — a loop's,
// a while loop's, an error handler's — whose first activity is centred at
// (x, y). The body has its own position, graph and pending state, and
// inherits what says which flow is being built and how its statements are
// written: the variable scope, the backend and project lookups, the language,
// the notes, and the guards. Every nested body is built here, so a field is
// inherited by all of them or by none — bar what the one site that needs it
// sets (an error handler's return type and label table) — and
// TestNestedBuilder_InheritsSharedState fails until a new field is put on one
// side or the other.
//
// ako/mxcli#1769: each site listed the fields it copied, and none listed
// qualifiedMembersOnly, so a fragment spliced into a stored flow refused an
// unqualifiable member on its main line and wrote it in every body nested in
// it — `find $L by Code = …` in a replaced loop became a find by the invalid
// expression `Code = …` (CE0117).
func (fb *flowBuilder) nestedBuilder(x, y int) *flowBuilder {
	// One registry for the whole flow, made here so a body's marks land in
	// it rather than in a map of the body's own (ako/mxcli#1914).
	if fb.statedIncludes == nil {
		fb.statedIncludes = map[*microflows.ExternalActionParameterMapping]bool{}
	}
	return &flowBuilder{
		posX:    x,
		posY:    y,
		baseY:   y,
		spacing: HorizontalSpacing,

		varTypes:     fb.varTypes,     // one variable scope
		declaredVars: fb.declaredVars, // (fixes a nil map panic)
		textLang:     fb.textLang,
		measurer:     fb.measurer,
		layout:       fb.layout, // a loop body is drawn in the flow's style
		backend:      fb.backend,
		quiet:        fb.quiet,
		hierarchy:    fb.hierarchy,
		restServices: fb.restServices, // for parameter classification
		isNanoflow:   fb.isNanoflow,
		self:         fb.self,
		scriptFlows:  fb.scriptFlows,
		// The note registry, so a note declared outside the body and
		// referenced on an activity in it attaches to the SAME Annotation
		// rather than being refused. The describer emits exactly that (its
		// label state is shared across the loop overlay), so a builder that
		// refused it would reject its own DESCRIBE output.
		//
		// Across a loop boundary the result — Annotation in the parent
		// collection, both AnnotationFlows in the parent collection, one
		// destination inside the loop — was measured at 0 errors on mxbuild
		// 11.13. mxbuild is the only oracle available here; Studio Pro is
		// stricter in general, though this is the same split mxcli already
		// ships for an ordinary loop-body note, whose flow is hoisted to the
		// parent. (#1077)
		annotationsByLabel: fb.annotationsByLabel,
		storedNotes:        fb.storedNotes,
		// A body of a spliced fragment is spliced too (ako/mxcli#1769).
		qualifiedMembersOnly: fb.qualifiedMembersOnly,
		// A call in a body that states `include associations` must reach the
		// carry, which reads the flow's registry (ako/mxcli#1914).
		statedIncludes:   fb.statedIncludes,
		resolveKnownOnly: fb.resolveKnownOnly,
	}
}

type flowBuilderVariableState struct {
	varTypes     map[string]string
	declaredVars map[string]string
}

func (fb *flowBuilder) snapshotVariableState() flowBuilderVariableState {
	return flowBuilderVariableState{
		varTypes:     cloneStringMap(fb.varTypes),
		declaredVars: cloneStringMap(fb.declaredVars),
	}
}

func (fb *flowBuilder) restoreVariableState(state flowBuilderVariableState) {
	fb.varTypes = state.varTypes
	fb.declaredVars = state.declaredVars
}

func cloneStringMap(in map[string]string) map[string]string {
	if in == nil {
		return nil
	}
	out := make(map[string]string, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

// addError records a validation error during flow building.
func (fb *flowBuilder) addError(format string, args ...any) {
	fb.errors = append(fb.errors, fmt.Sprintf(format, args...))
}

// addErrorWithExample records a validation error with a code example showing the fix.
func (fb *flowBuilder) addErrorWithExample(message, example string) {
	fb.errors = append(fb.errors, fmt.Sprintf("%s\n\n  Example:\n%s", message, example))
}

// GetErrors returns all validation errors collected during build.
func (fb *flowBuilder) GetErrors() []string {
	return fb.errors
}

func (fb *flowBuilder) hasDeclaredReturnValue() bool {
	return fb.returnType != nil && fb.returnType.Type.Kind != ast.TypeVoid
}

// addUndeclaredSetError reports `set $X = …` on a variable that is not
// declared. When $X is the flow's `returns … as $X` name the generic message
// reads as contradicting that clause, so it says what the clause does: it names
// the result for callers and declares nothing (mendixlabs/mxcli#1436).
func (fb *flowBuilder) addUndeclaredSetError(target string) {
	name := strings.TrimPrefix(target, "$")
	if fb.returnType != nil && fb.returnType.Variable != "" &&
		strings.TrimPrefix(fb.returnType.Variable, "$") == name {
		typ := fb.returnType.Type.Kind.String()
		if ref := fb.returnType.Type.EntityRef; ref != nil {
			typ = ref.Module + "." + ref.Name
		}
		fb.addErrorWithExample(
			fmt.Sprintf("variable '%s' is not declared: `returns … as $%s` names the result for callers "+
				"and does not declare $%s — declare it before the first `set`", name, name, name),
			fmt.Sprintf("    declare $%s %s = …;\n    ...\n    set $%s = …;", name, typ, name))
		return
	}
	fb.addErrorWithExample(
		fmt.Sprintf("variable '%s' is not declared", target),
		errorExampleDeclareVariable(target))
}

// errorExampleDeclareVariable returns an example for declaring a variable.
func errorExampleDeclareVariable(varName string) string {
	// Remove $ prefix if present for cleaner display
	cleanName := varName
	if len(varName) > 0 && varName[0] == '$' {
		cleanName = varName[1:]
	}
	return fmt.Sprintf(`    declare $%s Boolean = true;  -- or String, Integer, Decimal, DateTime
    ...
    set $%s = false;`, cleanName, cleanName)
}

// isVariableDeclared checks if a variable has been declared (either as primitive or entity).
func (fb *flowBuilder) isVariableDeclared(varName string) bool {
	name := strings.TrimPrefix(varName, "$")
	if idx := strings.IndexByte(name, '/'); idx >= 0 {
		name = name[:idx]
	}
	if _, ok := fb.varTypes[name]; ok {
		return true
	}
	if _, ok := fb.declaredVars[name]; ok {
		return true
	}
	return false
}

// registerResultVariableType records the output type of an action so later
// statements such as CHANGE, ADD TO, or attribute access can resolve members.
// When dt is nil (e.g. backend lookup failed), any stale entity/list typing is
// cleared but the variable remains declared as Unknown so downstream statements
// don't report it as undeclared.
func (fb *flowBuilder) registerResultVariableType(varName string, dt microflows.DataType) {
	if varName == "" {
		return
	}
	if dt == nil {
		if fb.varTypes != nil {
			delete(fb.varTypes, varName)
		}
		if fb.declaredVars != nil {
			fb.declaredVars[varName] = "Unknown"
		}
		return
	}

	switch t := dt.(type) {
	case *microflows.ObjectType:
		entityQName := t.EntityQualifiedName
		if entityQName == "" && t.EntityID != "" {
			entityQName = fb.resolveEntityQualifiedName(t.EntityID)
		}
		if fb.varTypes != nil && entityQName != "" {
			fb.varTypes[varName] = entityQName
			return
		}
	case *microflows.ListType:
		entityQName := t.EntityQualifiedName
		if entityQName == "" && t.EntityID != "" {
			entityQName = fb.resolveEntityQualifiedName(t.EntityID)
		}
		if fb.varTypes != nil && entityQName != "" {
			fb.varTypes[varName] = "List of " + entityQName
			return
		}
	}

	if fb.declaredVars != nil {
		fb.declaredVars[varName] = dt.GetTypeName()
	}
}

// lookupMicroflowReturnType resolves the return type of a called microflow by
// qualified name so downstream activities can infer variable types.
// selfFlow names the flow a create statement builds and what it returns.
type selfFlow struct {
	qualifiedName string
	nanoflow      bool
	returnType    microflows.DataType
}

// isSelf reports whether qualifiedName is the flow being built, of the kind
// the call names: a nanoflow never calls a microflow of its own name.
func (fb *flowBuilder) isSelf(qualifiedName string, nanoflow bool) bool {
	return fb.self != nil && fb.self.nanoflow == nanoflow && fb.self.qualifiedName == qualifiedName
}

// declaredFlow returns the declaration a call to qualifiedName resolves to
// without the project: the flow being built, or one an earlier statement of
// the script declares (scriptFlows). Nil means the project answers.
func (fb *flowBuilder) declaredFlow(qualifiedName string, nanoflow bool) *selfFlow {
	if fb.isSelf(qualifiedName, nanoflow) {
		return fb.self
	}
	return fb.scriptFlows[scriptFlowKey(nanoflow, qualifiedName)]
}

// scriptFlowKey keys ExecContext.scriptFlows.
func scriptFlowKey(nanoflow bool, qualifiedName string) string {
	if nanoflow {
		return "nanoflow:" + qualifiedName
	}
	return "microflow:" + qualifiedName
}

func (fb *flowBuilder) lookupMicroflowReturnType(qualifiedName string) microflows.DataType {
	if d := fb.declaredFlow(qualifiedName, false); d != nil {
		return d.returnType
	}
	if fb.backend == nil || qualifiedName == "" {
		return nil
	}

	// Fast path: direct lookup by qualified name avoids O(n) module walk.
	// Falls through to module walk on any error (not found, corrupt BSON, etc.).
	if rawUnit, err := fb.backend.GetRawUnitByName("microflow", qualifiedName); err == nil && rawUnit != nil && len(rawUnit.Contents) > 0 {
		if mf, err := fb.backend.ParseMicroflowBSON(rawUnit.Contents, model.ID(rawUnit.ID), ""); err == nil && mf != nil {
			return mf.ReturnType
		}
	}

	// Slow path: enumerate all microflows in the module and match by name.
	moduleName, microflowName, ok := strings.Cut(qualifiedName, ".")
	if !ok || moduleName == "" || microflowName == "" {
		return nil
	}

	module, err := fb.backend.GetModuleByName(moduleName)
	if err != nil || module == nil {
		return nil
	}
	if !fb.microflowsCacheLoaded {
		microflowList, err := fb.backend.ListMicroflows()
		if err != nil {
			return nil
		}
		fb.microflowsCache = microflowList
		fb.microflowsCacheLoaded = true
	}

	for _, mf := range fb.microflowsCache {
		if mf == nil {
			continue
		}
		containerModuleID := mf.ContainerID
		if fb.hierarchy != nil {
			containerModuleID = fb.hierarchy.FindModuleID(mf.ContainerID)
		}
		if containerModuleID == module.ID && mf.Name == microflowName {
			return mf.ReturnType
		}
	}

	return nil
}

func (fb *flowBuilder) lookupNanoflowReturnType(qualifiedName string) microflows.DataType {
	if d := fb.declaredFlow(qualifiedName, true); d != nil {
		return d.returnType
	}
	if fb.backend == nil || qualifiedName == "" {
		return nil
	}

	// Fast path: direct lookup by qualified name avoids O(n) module walk.
	// Falls through to module walk on any error (not found, corrupt BSON, etc.).
	if rawUnit, err := fb.backend.GetRawUnitByName("nanoflow", qualifiedName); err == nil && rawUnit != nil && len(rawUnit.Contents) > 0 {
		if nf, err := fb.backend.ParseMicroflowBSON(rawUnit.Contents, model.ID(rawUnit.ID), ""); err == nil && nf != nil {
			return nf.ReturnType
		}
	}

	// Slow path: enumerate all nanoflows in the module and match by name.
	moduleName, nanoflowName, ok := strings.Cut(qualifiedName, ".")
	if !ok || moduleName == "" || nanoflowName == "" {
		return nil
	}

	module, err := fb.backend.GetModuleByName(moduleName)
	if err != nil || module == nil {
		return nil
	}
	if !fb.nanoflowsCacheLoaded {
		nanoflowList, err := fb.backend.ListNanoflows()
		if err != nil {
			return nil
		}
		fb.nanoflowsCache = nanoflowList
		fb.nanoflowsCacheLoaded = true
	}

	for _, nf := range fb.nanoflowsCache {
		if nf == nil {
			continue
		}
		containerModuleID := nf.ContainerID
		if fb.hierarchy != nil {
			containerModuleID = fb.hierarchy.FindModuleID(nf.ContainerID)
		}
		if containerModuleID == module.ID && nf.Name == nanoflowName {
			return nf.ReturnType
		}
	}

	return nil
}

// microflowExists returns true if qualifiedName refers to a microflow present
// in the connected project. Returns true (no error) when no backend is available
// or when backend calls fail, so offline / syntax-check mode is unaffected.
func (fb *flowBuilder) microflowExists(qualifiedName string) bool {
	if fb.backend == nil || fb.declaredFlow(qualifiedName, false) != nil {
		return true
	}
	// Fast path: name-indexed lookup; succeeds without loading the full list.
	if rawUnit, err := fb.backend.GetRawUnitByName("microflow", qualifiedName); err == nil && rawUnit != nil {
		return true
	}
	// Slow path: enumerate microflows in the resolved module.
	moduleName, mfName, ok := strings.Cut(qualifiedName, ".")
	if !ok || moduleName == "" || mfName == "" {
		return true // malformed name — not our problem to report
	}
	module, err := fb.backend.GetModuleByName(moduleName)
	if err != nil || module == nil {
		return true // module not found — let other validation surface that
	}
	if !fb.microflowsCacheLoaded {
		list, err := fb.backend.ListMicroflows()
		if err != nil {
			return true // backend error — don't block
		}
		fb.microflowsCache = list
		fb.microflowsCacheLoaded = true
	}
	for _, mf := range fb.microflowsCache {
		if mf == nil {
			continue
		}
		containerModuleID := mf.ContainerID
		if fb.hierarchy != nil {
			containerModuleID = fb.hierarchy.FindModuleID(mf.ContainerID)
		}
		if containerModuleID == module.ID && mf.Name == mfName {
			return true
		}
	}
	return false
}

// nanoflowExists returns true if qualifiedName refers to a nanoflow present
// in the connected project. Same fallback-to-true semantics as microflowExists.
func (fb *flowBuilder) nanoflowExists(qualifiedName string) bool {
	if fb.backend == nil || fb.declaredFlow(qualifiedName, true) != nil {
		return true
	}
	if rawUnit, err := fb.backend.GetRawUnitByName("nanoflow", qualifiedName); err == nil && rawUnit != nil {
		return true
	}
	moduleName, nfName, ok := strings.Cut(qualifiedName, ".")
	if !ok || moduleName == "" || nfName == "" {
		return true
	}
	module, err := fb.backend.GetModuleByName(moduleName)
	if err != nil || module == nil {
		return true
	}
	if !fb.nanoflowsCacheLoaded {
		list, err := fb.backend.ListNanoflows()
		if err != nil {
			return true
		}
		fb.nanoflowsCache = list
		fb.nanoflowsCacheLoaded = true
	}
	for _, nf := range fb.nanoflowsCache {
		if nf == nil {
			continue
		}
		containerModuleID := nf.ContainerID
		if fb.hierarchy != nil {
			containerModuleID = fb.hierarchy.FindModuleID(nf.ContainerID)
		}
		if containerModuleID == module.ID && nf.Name == nfName {
			return true
		}
	}
	return false
}

func (fb *flowBuilder) resolveEntityQualifiedName(entityID model.ID) string {
	if fb.backend == nil || entityID == "" {
		return ""
	}

	domainModels, err := fb.backend.ListDomainModels()
	if err != nil {
		return ""
	}

	for _, dm := range domainModels {
		if dm == nil {
			continue
		}

		moduleName := ""
		if fb.hierarchy != nil {
			moduleName = fb.hierarchy.GetModuleName(dm.ContainerID)
		}
		if moduleName == "" {
			if mod, err := fb.backend.GetModule(dm.ContainerID); err == nil && mod != nil {
				moduleName = mod.Name
			}
		}
		if moduleName == "" {
			continue
		}

		for _, entity := range dm.Entities {
			if entity != nil && entity.ID == entityID {
				return moduleName + "." + entity.Name
			}
		}
	}

	return ""
}

// exprToString converts an AST Expression to a Mendix expression string,
// resolving association navigation paths to include the target entity qualifier.
// e.g. $Order/MyModule.Order_Customer/Name → $Order/MyModule.Order_Customer/MyModule.Customer/Name
func (fb *flowBuilder) exprToString(expr ast.Expression) string {
	resolved := fb.resolveAssociationPaths(expr)
	return expressionToString(resolved)
}

// resolveAssociationPaths walks an expression tree and, for any AttributePathExpr
// whose path contains an association (qualified name like Module.AssocName), inserts
// the association's target entity after the association segment.
func (fb *flowBuilder) resolveAssociationPaths(expr ast.Expression) ast.Expression {
	if expr == nil {
		return nil
	}

	switch e := expr.(type) {
	case *ast.AttributePathExpr:
		resolved := fb.resolvePathSegments(e.Path)
		return &ast.AttributePathExpr{
			Variable: e.Variable,
			Path:     resolved,
			Segments: e.Segments,
		}
	case *ast.BinaryExpr:
		return &ast.BinaryExpr{
			Left:     fb.resolveAssociationPaths(e.Left),
			Operator: e.Operator,
			Right:    fb.resolveAssociationPaths(e.Right),
		}
	case *ast.UnaryExpr:
		return &ast.UnaryExpr{
			Operator: e.Operator,
			Operand:  fb.resolveAssociationPaths(e.Operand),
		}
	case *ast.FunctionCallExpr:
		args := make([]ast.Expression, len(e.Arguments))
		for i, arg := range e.Arguments {
			args[i] = fb.resolveAssociationPaths(arg)
		}
		return &ast.FunctionCallExpr{
			Name:      e.Name,
			Arguments: args,
		}
	case *ast.ParenExpr:
		return &ast.ParenExpr{Inner: fb.resolveAssociationPaths(e.Inner)}
	case *ast.IfThenElseExpr:
		return &ast.IfThenElseExpr{
			Condition: fb.resolveAssociationPaths(e.Condition),
			ThenExpr:  fb.resolveAssociationPaths(e.ThenExpr),
			ElseExpr:  fb.resolveAssociationPaths(e.ElseExpr),
		}
	case *ast.SourceExpr:
		if e.Source != "" {
			// Non-empty Source is the exact expression text to write back (kept
			// for decimal/whitespace fidelity, #17-19). But an association
			// navigation inside that source still needs its target-entity step
			// inserted (`$T/Mod.Assoc/attr` → `$T/Mod.Assoc/Mod.Entity/attr`) or
			// Mendix rejects the whole expression with CE0117 — e.g. a decimal
			// literal co-occurring with an association nav (`$T/Mod.Assoc/Bal + 2.0`)
			// froze the source and skipped resolution (ledger #48). Rewrite each
			// association path within the source while preserving everything else.
			resolvedInner := fb.resolveAssociationPaths(e.Expression)
			fixed := e.Source
			for _, orig := range collectAttributePaths(e.Expression) {
				o := expressionToString(orig)
				r := expressionToString(fb.resolveAssociationPaths(orig))
				if o != r {
					fixed = strings.ReplaceAll(fixed, o, r)
				}
			}
			return &ast.SourceExpr{Expression: resolvedInner, Source: fixed}
		}
		return fb.resolveAssociationPaths(e.Expression)
	default:
		return expr
	}
}

// resolvePathSegments processes path segments in an attribute path expression.
// For each segment that is a qualified association name (Module.AssocName), it looks up
// the association's target entity and inserts it after the association.
func (fb *flowBuilder) resolvePathSegments(path []string) []string {
	if fb.backend == nil || len(path) == 0 {
		return path
	}

	var resolved []string
	skipNext := false
	for i, segment := range path {
		if skipNext {
			// Consumed as the association's target-entity step below.
			skipNext = false
			continue
		}
		resolved = append(resolved, segment)

		// A qualified name (contains ".") that isn't the last segment might be an association
		if !strings.Contains(segment, ".") {
			continue
		}
		// If the next segment is already a qualified name, the target entity is already present
		if i+1 < len(path) && strings.Contains(path[i+1], ".") {
			continue
		}
		// If this is the last segment, nothing to insert after
		if i == len(path)-1 {
			continue
		}

		// Look up association target entity
		parts := strings.SplitN(segment, ".", 2)
		if len(parts) != 2 {
			continue
		}
		result := fb.lookupAssociation(parts[0], parts[1])
		if result == nil || result.childEntityQN == "" {
			continue
		}
		// The author may have written the target entity by its BARE name
		// (`Mod.Assoc/Tag/Label`). That is not the form Mendix stores, but it is
		// the same step — qualify it in place. Appending unconditionally produced
		// `Mod.Assoc/Mod.Tag/Tag/Label`, a four-segment path as invalid as the
		// three-segment one it was meant to fix (#829).
		if i+1 < len(path) {
			if bare := path[i+1]; bare != "" && strings.HasSuffix(result.childEntityQN, "."+bare) {
				skipNext = true
			}
		}
		resolved = append(resolved, result.childEntityQN)
	}
	return resolved
}

// collectAttributePaths returns every AttributePathExpr in an expression tree,
// used to rewrite association navigations inside a source-preserved expression.
func collectAttributePaths(expr ast.Expression) []*ast.AttributePathExpr {
	var out []*ast.AttributePathExpr
	var walk func(ast.Expression)
	walk = func(e ast.Expression) {
		switch v := e.(type) {
		case *ast.AttributePathExpr:
			out = append(out, v)
		case *ast.BinaryExpr:
			walk(v.Left)
			walk(v.Right)
		case *ast.UnaryExpr:
			walk(v.Operand)
		case *ast.ParenExpr:
			walk(v.Inner)
		case *ast.FunctionCallExpr:
			for _, a := range v.Arguments {
				walk(a)
			}
		case *ast.IfThenElseExpr:
			walk(v.Condition)
			walk(v.ThenExpr)
			walk(v.ElseExpr)
		case *ast.SourceExpr:
			walk(v.Expression)
		}
	}
	walk(expr)
	return out
}

// buildSplitCondition constructs the right SplitCondition variant for an IF
// statement. When the condition is a qualified call into a rule, it emits a
// RuleSplitCondition (nested RuleCall with ParameterMappings). Everything else
// falls back to ExpressionSplitCondition.
//
// Studio Pro enforces this distinction: a rule reference stored as an
// expression fails validation with CE0117, which is the regression this
// helper prevents on describe → exec roundtrips.
func (fb *flowBuilder) buildSplitCondition(expr ast.Expression, fallbackExpression string) microflows.SplitCondition {
	if ruleCond := fb.tryBuildRuleSplitCondition(expr); ruleCond != nil {
		return ruleCond
	}
	// A qualified call that did NOT resolve to a rule cannot become a condition
	// either: Mendix expressions have no user-callable functions, so falling back
	// to an ExpressionSplitCondition here writes text the build rejects with
	// CE0117. MDL066 catches the same mistake in value positions without needing a
	// project; this is the decision position, where telling a rule from a
	// microflow needs the backend. Refuse rather than write it (#939).
	if call := unwrapParenCall(expr); call != nil && strings.Contains(call.Name, ".") {
		fb.addErrorWithExample(fmt.Sprintf(
			"if condition calls '%s(...)', which is not a rule in this project — a decision "+
				"can only call a rule, and a Mendix expression cannot call anything, so the "+
				"build fails CE0117 \"Error(s) in expression\"", call.Name),
			"    $Result = CALL MICROFLOW "+call.Name+" (...);\n"+
				"    if $Result then ... end if;")
	}
	return &microflows.ExpressionSplitCondition{
		BaseElement: model.BaseElement{ID: model.ID(types.GenerateID())},
		Expression:  fallbackExpression,
	}
}

// tryBuildRuleSplitCondition returns a RuleSplitCondition when the expression
// is a qualified function call that resolves to a rule via the backend.
// Returns nil if the expression isn't a qualified call, if the backend is
// unavailable, or if the name doesn't resolve to a rule.
func (fb *flowBuilder) tryBuildRuleSplitCondition(expr ast.Expression) *microflows.RuleSplitCondition {
	if fb.backend == nil {
		return nil
	}
	call := unwrapParenCall(expr)
	if call == nil {
		return nil
	}
	// Only qualified names (Module.Name) can refer to rules; bare identifiers
	// are built-ins (length, contains, etc.).
	if !strings.Contains(call.Name, ".") {
		return nil
	}
	isRule, err := fb.backend.IsRule(call.Name)
	if err != nil || !isRule {
		return nil
	}

	cond := &microflows.RuleSplitCondition{
		BaseElement:       model.BaseElement{ID: model.ID(types.GenerateID())},
		RuleQualifiedName: call.Name,
	}
	for _, arg := range call.Arguments {
		name, value := extractNamedArg(arg)
		if name == "" {
			// Positional arguments aren't representable in RuleCall — skip
			// rather than fabricate a parameter mapping that Studio Pro
			// would reject.
			continue
		}
		cond.ParameterMappings = append(cond.ParameterMappings, &microflows.RuleCallParameterMapping{
			BaseElement:   model.BaseElement{ID: model.ID(types.GenerateID())},
			ParameterName: call.Name + "." + name,
			Argument:      fb.exprToString(value),
		})
	}
	return cond
}

// unwrapParenCall peels outer ParenExprs and returns the inner FunctionCallExpr
// if present. Describer output wraps rule calls in parens when they sit inside
// boolean expressions, so we must see through them.
func unwrapParenCall(expr ast.Expression) *ast.FunctionCallExpr {
	for {
		switch e := expr.(type) {
		case *ast.FunctionCallExpr:
			return e
		case *ast.ParenExpr:
			expr = e.Inner
		case *ast.SourceExpr:
			expr = e.Expression
		default:
			return nil
		}
	}
}

// extractNamedArg recognises `Name = value` BinaryExprs and returns the
// parameter name + value. Anything else returns "", nil.
//
// The left side of a named-arg expression can surface as either an
// IdentifierExpr (bare parameter name) or an AttributePathExpr with an empty
// Variable — both forms come out of the visitor depending on surrounding
// context, so handle them both.
func extractNamedArg(expr ast.Expression) (string, ast.Expression) {
	bin, ok := expr.(*ast.BinaryExpr)
	if !ok || bin.Operator != "=" {
		return "", nil
	}
	switch left := bin.Left.(type) {
	case *ast.IdentifierExpr:
		return left.Name, bin.Right
	case *ast.AttributePathExpr:
		if left.Variable == "" && len(left.Path) == 1 {
			return left.Path[0], bin.Right
		}
	}
	return "", nil
}

// textLangOrDefault is the language this builder stores a bare message/caption
// string under. A zero-value flowBuilder (validateFlowBody builds one, and it
// never writes) reports en_US, so the fallback is identical to the pre-#970
// behaviour rather than an empty LanguageCode.
func (fb *flowBuilder) textLangOrDefault() string {
	if fb.textLang == "" {
		return fallbackLanguageCode
	}
	return fb.textLang
}
